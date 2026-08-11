package operationcenter

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type paymentSucceededTransactionContextKey struct{}

// RecordPaymentSucceededForTx applies payment fulfillment inside the caller's
// transaction. The caller owns commit and rollback.
func (service *WorkflowService) RecordPaymentSucceededForTx(ctx context.Context, tx *sql.Tx, command PaymentSucceededCommand) (WorkflowResult, error) {
	if tx == nil {
		return WorkflowResult{}, ErrTransactionRequired
	}
	return service.RecordPaymentSucceeded(context.WithValue(ctx, paymentSucceededTransactionContextKey{}, tx), command)
}

func (service *WorkflowService) RecordPaymentSucceeded(ctx context.Context, command PaymentSucceededCommand) (result WorkflowResult, err error) {
	if service == nil || service.db == nil {
		return result, ErrWorkflowUnavailable
	}
	if strings.TrimSpace(command.OrderID) == "" || strings.TrimSpace(command.PaymentRecordID) == "" {
		return result, fmt.Errorf("record operation center payment: %w", ErrConstraintViolation)
	}
	tx, _ := ctx.Value(paymentSucceededTransactionContextKey{}).(*sql.Tx)
	ownsTransaction := tx == nil
	if ownsTransaction {
		tx, err = service.db.BeginTx(ctx, nil)
		if err != nil {
			return result, err
		}
		defer func() { _ = tx.Rollback() }()
	}
	txStoreValue, err := service.store.BindTx(tx)
	if err != nil {
		return result, err
	}
	txStore := txStoreValue.(Store)
	snapshot, err := lockPaidOrderSnapshot(ctx, tx, command)
	if err != nil {
		return result, err
	}
	if !isSuccessfulPaymentStatus(snapshot.PaymentStatus) || !isPaidOrderStatus(snapshot.OrderStatus) || !snapshot.PaidAt.Valid {
		return result, ErrPaymentNotSuccessful
	}
	if err := validatePaymentAmounts(snapshot); err != nil {
		return result, err
	}
	if snapshot.RefundPolicySnapshot == nil || snapshot.RelationshipSnapshot == nil || snapshot.RuleSetID == "" || snapshot.PlanVersionID == "" || snapshot.CommercialSnapshotID == "" {
		return result, ErrFrozenSnapshotMissing
	}

	existing, lookupErr := txStore.FindServiceOrderByCommercialOrderID(ctx, snapshot.OrderID)
	if lookupErr == nil {
		existing, err = txStore.GetServiceOrderForUpdate(ctx, existing.ID)
		if err != nil {
			return result, err
		}
		switch existing.Status {
		case OperationCenterServiceReviewRequired, OperationCenterServiceActive,
			OperationCenterServiceRejected, OperationCenterServiceRevoking, OperationCenterServiceRevoked:
			if ownsTransaction {
				if err := tx.Commit(); err != nil {
					return result, err
				}
			}
			return WorkflowResult{ServiceOrder: existing, IdempotentReplay: true}, nil
		case OperationCenterServicePendingPayment:
		default:
			return result, fmt.Errorf("payment fulfillment found unsupported service status %s: %w", existing.Status, ErrInvalidServiceTransition)
		}
	} else if !errors.Is(lookupErr, ErrNotFound) {
		return result, lookupErr
	}

	now, err := databaseNow(ctx, tx)
	if err != nil {
		return result, err
	}
	item := existing
	if item == nil {
		ruleVersion := snapshot.RuleSetVersion
		ruleSetID := snapshot.RuleSetID
		planVersionID := snapshot.PlanVersionID
		commercialSnapshotID := snapshot.CommercialSnapshotID
		paymentChannel := snapshot.PaymentChannel
		item = &OperationCenterServiceOrder{
			ID: stableWorkflowID("operation_center_service", snapshot.OrderID), TenantID: snapshot.TenantID,
			OrderID: snapshot.OrderID, OrderNo: snapshot.OrderNo, ApplicantUserID: snapshot.UserID,
			TechnicalServiceFeeCents: snapshot.PlanPriceCents, Currency: snapshot.Currency,
			Status: OperationCenterServicePendingPayment, RefundStatus: OperationCenterRefundNone,
			CommercialRuleSetID: &ruleSetID, CommercialRuleSetVersion: &ruleVersion,
			PlanVersionID: &planVersionID, CommercialOrderSnapshotID: &commercialSnapshotID,
			RelationshipSnapshot: snapshot.RelationshipSnapshot, RefundPolicySnapshot: snapshot.RefundPolicySnapshot,
			PaymentChannel: &paymentChannel, RefundFailureDetail: JSONSnapshot{},
			Metadata: paymentMetadata(snapshot), CreatedAt: now, UpdatedAt: now,
		}
		if err := txStore.CreateServiceOrder(ctx, item); err != nil {
			return result, err
		}
	}
	paidAt := snapshot.PaidAt.Time.UTC()
	item.Status = OperationCenterServiceReviewRequired
	item.PaidAt = &paidAt
	item.RefundStatus = OperationCenterRefundNone
	item.StateVersion++
	item.UpdatedAt = now
	transition := OperationCenterStateTransition{
		ID: stableWorkflowID("operation_center_transition", item.ID, "payment_review_required"), TenantID: item.TenantID,
		EntityType: StateEntityServiceOrder, EntityID: item.ID,
		FromStatus: stringPointerValue(string(OperationCenterServicePendingPayment)), ToStatus: string(OperationCenterServiceReviewRequired),
		TransitionReason: "PAYMENT_SUCCEEDED_REVIEW_REQUIRED", TransactionGroupID: stableWorkflowID("operation_center_tx", item.ID, "payment"),
		IdempotencyKey: stableWorkflowID("operation_center_audit", item.ID, "payment_review_required"),
		Metadata:       JSONSnapshot{"orderId": snapshot.OrderID, "paymentRecordId": snapshot.PaymentRecordID}, CreatedAt: now,
	}
	if err := txStore.UpdateServiceOrder(ctx, item, &transition); err != nil {
		return result, err
	}
	if ownsTransaction {
		if err := tx.Commit(); err != nil {
			return result, err
		}
	}
	return WorkflowResult{ServiceOrder: item}, nil
}

func lockPaidOrderSnapshot(ctx context.Context, tx *sql.Tx, command PaymentSucceededCommand) (paidOrderSnapshot, error) {
	var snapshot paidOrderSnapshot
	var paymentStatus, provider, providerPaymentNo sql.NullString
	var commercialRelationship, commercialRules JSONSnapshot
	var planIdentity string
	err := tx.QueryRowContext(ctx, `
		SELECT orders.id,orders.tenant_id,orders.user_id,orders.order_no,orders.plan_id,
		       orders.amount_cents,orders.payable_amount_cents,orders.currency,
		       coalesce(orders.order_status,orders.status,''),orders.price_snapshot,
		       payment.id,payment.amount_cents,coalesce(payment.payment_status,payment.prepay_status),
		       coalesce(payment.provider,payment.payment_channel),
		       coalesce(payment.provider_trade_no,payment.provider_transaction_id,payment.payment_no),payment.paid_at,
		       snapshot.id,snapshot.plan_version_id,snapshot.rule_set_id,snapshot.rule_set_version,
		       snapshot.paid_amount_cents,snapshot.relationship_snapshot,snapshot.commission_rule_snapshot,
		       plan.price_cents,plan.identity_type
		FROM xz_orders orders
		JOIN xz_payment_records payment ON payment.id=$2 AND payment.order_id=orders.id
		JOIN xz_commercial_order_rule_snapshots snapshot ON snapshot.order_id=orders.id
		JOIN xz_commercial_plan_versions plan ON plan.id=snapshot.plan_version_id AND plan.rule_set_id=snapshot.rule_set_id AND plan.plan_id=snapshot.plan_id
		JOIN xz_commercial_rule_sets rules ON rules.id=snapshot.rule_set_id
			AND rules.version=snapshot.rule_set_version AND rules.status='PUBLISHED'
		WHERE orders.id=$1 AND snapshot.scenario_code='OPERATION_CENTER_SERVICE'
		  AND snapshot.plan_id=orders.plan_id
		FOR UPDATE OF orders,payment
	`, command.OrderID, command.PaymentRecordID).Scan(
		&snapshot.OrderID, &snapshot.TenantID, &snapshot.UserID, &snapshot.OrderNo, &snapshot.PlanID,
		&snapshot.OrderAmountCents, &snapshot.PayableAmountCents, &snapshot.Currency,
		&snapshot.OrderStatus, &snapshot.PriceSnapshot, &snapshot.PaymentRecordID,
		&snapshot.PaymentAmountCents, &paymentStatus, &provider, &providerPaymentNo, &snapshot.PaidAt,
		&snapshot.CommercialSnapshotID, &snapshot.PlanVersionID, &snapshot.RuleSetID,
		&snapshot.RuleSetVersion, &snapshot.CommercialSnapshotPaidCents, &commercialRelationship,
		&commercialRules, &snapshot.PlanPriceCents, &planIdentity,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return snapshot, ErrFrozenSnapshotMissing
	}
	if err != nil {
		return snapshot, err
	}
	if planIdentity != "OPERATION_CENTER" {
		return snapshot, ErrFrozenSnapshotMissing
	}
	snapshot.PaymentStatus = paymentStatus.String
	snapshot.PaymentChannel = provider.String
	snapshot.ProviderPaymentNo = providerPaymentNo.String
	snapshot.RelationshipSnapshot = commercialRelationship
	if refundPolicy, ok := snapshotObject(snapshot.PriceSnapshot, "refundPolicySnapshot"); ok {
		snapshot.RefundPolicySnapshot = refundPolicy
	} else if commercialRules != nil {
		snapshot.RefundPolicySnapshot = JSONSnapshot{"commissionRuleSnapshot": map[string]any(commercialRules)}
	}
	return snapshot, nil
}

func paymentMetadata(snapshot paidOrderSnapshot) JSONSnapshot {
	return JSONSnapshot{
		"paymentRecordId": snapshot.PaymentRecordID, "paymentStatus": snapshot.PaymentStatus,
		"providerPaymentNo": snapshot.ProviderPaymentNo, "paidAmountCents": snapshot.PaymentAmountCents,
		"commercialOrderSnapshotId": snapshot.CommercialSnapshotID,
	}
}

func (service *WorkflowService) Review(ctx context.Context, command ReviewCommand) (result WorkflowResult, err error) {
	if service == nil || service.db == nil {
		return result, ErrWorkflowUnavailable
	}
	if strings.TrimSpace(command.ServiceOrderID) == "" || strings.TrimSpace(command.IdempotencyKey) == "" || strings.TrimSpace(command.ReviewedBy) == "" {
		return result, ErrConstraintViolation
	}
	if command.ExpectedStatus != OperationCenterServiceReviewRequired {
		return result, ErrExpectedServiceStatus
	}
	if command.Decision != ReviewApproved && command.Decision != ReviewRejected {
		return result, ErrConstraintViolation
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	txStoreValue, err := service.store.BindTx(tx)
	if err != nil {
		return result, err
	}
	txStore := txStoreValue.(Store)
	item, err := txStore.GetServiceOrderForUpdate(ctx, command.ServiceOrderID)
	if err != nil {
		return result, err
	}
	existingEvent, eventErr := txStore.GetReviewEventByIdempotencyKey(ctx, item.TenantID, command.IdempotencyKey)
	if eventErr == nil {
		if existingEvent.ServiceOrderID != item.ID || existingEvent.Decision != command.Decision {
			return result, ErrReviewDecisionConflict
		}
		if err := tx.Commit(); err != nil {
			return result, err
		}
		return WorkflowResult{ServiceOrder: item, ReviewEvent: existingEvent, IdempotentReplay: true}, nil
	}
	if !errors.Is(eventErr, ErrNotFound) {
		return result, eventErr
	}
	if item.Status != command.ExpectedStatus {
		return result, fmt.Errorf("%w: expected=%s actual=%s", ErrExpectedServiceStatus, command.ExpectedStatus, item.Status)
	}
	if err := ValidateOperationCenterServiceTransition(item.Status, serviceStatusForDecision(command.Decision)); err != nil {
		return result, err
	}
	if item.RefundStatus != OperationCenterRefundNone {
		return result, fmt.Errorf("review requires refund NONE: %w", ErrInvalidRefundTransition)
	}
	payment, err := lockReviewPaymentAndSnapshots(ctx, tx, item)
	if err != nil {
		return result, err
	}
	if !isSuccessfulPaymentStatus(payment.Status) {
		return result, ErrPaymentNotSuccessful
	}
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return result, err
	}
	event := &OperationCenterReviewEvent{
		ID:       stableWorkflowID("operation_center_review", item.TenantID, command.IdempotencyKey),
		TenantID: item.TenantID, ServiceOrderID: item.ID, Decision: command.Decision,
		Status: ReviewEventPending, ReviewedBy: command.ReviewedBy,
		RequestID: optionalString(command.RequestID), IdempotencyKey: command.IdempotencyKey,
		FailureDetail: JSONSnapshot{}, EventSnapshot: JSONSnapshot{"reason": command.Reason, "expectedStatus": string(command.ExpectedStatus)},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := txStore.CreateReviewEvent(ctx, event); err != nil {
		return result, err
	}
	transactionGroupID := stableWorkflowID("operation_center_tx", item.ID, command.IdempotencyKey)
	switch command.Decision {
	case ReviewApproved:
		if err := activateOperationCenterResourcesTx(ctx, tx, item, command, payment, now); err != nil {
			return result, err
		}
		item.Status = OperationCenterServiceActive
		item.ActivatedAt = timePointer(now)
		item.ReviewedAt = timePointer(now)
		item.ReviewedBy = optionalString(command.ReviewedBy)
		item.ReviewIdempotencyKey = optionalString(command.IdempotencyKey)
		item.StateVersion++
		item.UpdatedAt = now
		transition := reviewServiceTransition(item, command, OperationCenterServiceReviewRequired, OperationCenterServiceActive, transactionGroupID, now)
		if err := txStore.UpdateServiceOrder(ctx, item, &transition); err != nil {
			return result, err
		}
		if err := service.activationHook.AfterActivated(ctx, tx, item); err != nil {
			return result, err
		}
		if err := service.referralEligibilityTrigger.MarkEligible(ctx, tx, item); err != nil {
			return result, err
		}
		if _, err := service.referralRewardGrantHook.GrantForServiceOrder(ctx, tx, item); err != nil {
			return result, err
		}
	case ReviewRejected:
		refundTask, rejectErr := prepareRejectedRefundTx(ctx, txStore, item, command, payment, transactionGroupID, now)
		if rejectErr != nil {
			return result, rejectErr
		}
		result.RefundTask = refundTask
	}
	event.Status = ReviewEventApplied
	event.AppliedAt = timePointer(now)
	event.UpdatedAt = now
	if err := txStore.UpdateReviewEvent(ctx, event); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	result.ServiceOrder = item
	result.ReviewEvent = event
	return result, nil
}

type reviewPaymentSnapshot struct {
	PaymentRecordID, Status, Channel, ProviderPaymentNo string
}

func lockReviewPaymentAndSnapshots(ctx context.Context, tx *sql.Tx, item *OperationCenterServiceOrder) (reviewPaymentSnapshot, error) {
	var result reviewPaymentSnapshot
	paymentRecordID, ok := item.Metadata["paymentRecordId"].(string)
	if !ok || paymentRecordID == "" || item.CommercialRuleSetID == nil || item.CommercialRuleSetVersion == nil || item.PlanVersionID == nil || item.CommercialOrderSnapshotID == nil {
		return result, ErrFrozenSnapshotMissing
	}
	err := tx.QueryRowContext(ctx, `
		SELECT payment.id,coalesce(payment.payment_status,payment.prepay_status),
		       coalesce(payment.provider,payment.payment_channel),
		       coalesce(payment.provider_trade_no,payment.provider_transaction_id,payment.payment_no)
		FROM xz_payment_records payment
		JOIN xz_orders orders ON orders.id=payment.order_id
		JOIN xz_commercial_order_rule_snapshots snapshot ON snapshot.id=$4 AND snapshot.order_id=orders.id
		JOIN xz_commercial_plan_versions plan ON plan.id=$5 AND plan.rule_set_id=$6
		JOIN xz_commercial_rule_sets rules ON rules.id=$6 AND rules.version=$7 AND rules.status='PUBLISHED'
		WHERE payment.id=$1 AND orders.id=$2 AND payment.amount_cents=$3
		FOR UPDATE OF payment,orders
	`, paymentRecordID, item.OrderID, item.TechnicalServiceFeeCents, *item.CommercialOrderSnapshotID,
		*item.PlanVersionID, *item.CommercialRuleSetID, *item.CommercialRuleSetVersion).Scan(
		&result.PaymentRecordID, &result.Status, &result.Channel, &result.ProviderPaymentNo)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrFrozenSnapshotMissing
	}
	return result, err
}

func activateOperationCenterResourcesTx(ctx context.Context, tx *sql.Tx, item *OperationCenterServiceOrder, command ReviewCommand, payment reviewPaymentSnapshot, now time.Time) error {
	var userName, userStatus string
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(name,''),upper(coalesce(status,'')) FROM xz_users WHERE id=$1 FOR UPDATE`, item.ApplicantUserID).Scan(&userName, &userStatus); err != nil {
		return err
	}
	if userStatus != "ACTIVE" {
		return fmt.Errorf("applicant user is not active: %w", ErrConstraintViolation)
	}
	var existingIdentityID, existingIdentityType, existingIdentityStatus, existingSourceOrderID string
	identityErr := tx.QueryRowContext(ctx, `SELECT id,identity_type,identity_status,coalesce(source_order_id,'') FROM xz_user_business_identities WHERE tenant_id=$1 AND user_id=$2 AND identity_type IN ('AGENT','OPERATION_CENTER') AND identity_status IN ('PENDING','ACTIVE','FROZEN') FOR UPDATE`, item.TenantID, item.ApplicantUserID).Scan(&existingIdentityID, &existingIdentityType, &existingIdentityStatus, &existingSourceOrderID)
	if identityErr != nil && !errors.Is(identityErr, sql.ErrNoRows) {
		return identityErr
	}
	if identityErr == nil {
		if existingIdentityType != "OPERATION_CENTER" || existingSourceOrderID != item.OrderID || existingIdentityStatus == "FROZEN" {
			return fmt.Errorf("applicant already has current channel identity: %w", ErrConstraintViolation)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE xz_user_business_identities SET identity_status='ACTIVE',commission_enabled=true,effective_at=$2,status_reason='',updated_at=$2 WHERE id=$1`, existingIdentityID, now); err != nil {
			return err
		}
	} else {
		var version int64
		if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(identity_version),0)+1 FROM xz_user_business_identities WHERE tenant_id=$1 AND user_id=$2 AND identity_type='OPERATION_CENTER'`, item.TenantID, item.ApplicantUserID).Scan(&version); err != nil {
			return err
		}
		identityID := stableWorkflowID("operation_center_identity", item.TenantID, item.ApplicantUserID, fmt.Sprint(version))
		if _, err := tx.ExecContext(ctx, `INSERT INTO xz_user_business_identities(id,tenant_id,user_id,identity_type,identity_status,commission_enabled,source_type,source_order_id,effective_at,status_reason,identity_version,created_by,created_at,updated_at) VALUES($1,$2,$3,'OPERATION_CENTER','ACTIVE',true,'OPERATION_CENTER_REVIEW',$4,$5,'',$6,$7,$5,$5)`, identityID, item.TenantID, item.ApplicantUserID, item.OrderID, now, version, command.ReviewedBy); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE xz_users SET operation_center_status='ACTIVE',updated_at=$2,raw=jsonb_set(coalesce(raw,'{}'::jsonb),'{operationCenterStatus}','"ACTIVE"'::jsonb,true) WHERE id=$1`, item.ApplicantUserID, now.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	profileID := stableWorkflowID("operation_center_profile", item.TenantID, item.ApplicantUserID)
	inviteCode := strings.ToUpper("OC" + strings.TrimPrefix(stableWorkflowID("invite", profileID), "invite_")[:12])
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_operation_centers(id,user_id,name,invite_code,status,join_order_id,join_fee_cents,approved_at,created_at,updated_at,raw)
		VALUES($1,$2,$3,$4,'ACTIVE',$5,$6,$7,$8,$8,'{}'::jsonb)
		ON CONFLICT(user_id) DO UPDATE SET status='ACTIVE',join_order_id=excluded.join_order_id,
		  join_fee_cents=excluded.join_fee_cents,approved_at=excluded.approved_at,updated_at=excluded.updated_at
	`, profileID, item.ApplicantUserID, userName+"运营中心", inviteCode, item.OrderID,
		item.TechnicalServiceFeeCents, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	var role string
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(nullif(config->>'rbacRole',''),'OPERATION') FROM xz_commercial_plan_versions WHERE id=$1 AND identity_type='OPERATION_CENTER'`, *item.PlanVersionID).Scan(&role); err != nil {
		return err
	}
	var permissionConfigured bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM xz_role_permissions WHERE role=$1)`, role).Scan(&permissionConfigured); err != nil {
		return err
	}
	if !permissionConfigured {
		return fmt.Errorf("operation center RBAC role %s has no configured permissions: %w", role, ErrConstraintViolation)
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO xz_user_roles(user_id,tenant_id,organization_id,role,status,assigned_at,updated_at)
		SELECT $1,scope.tenant_id,scope.organization_id,$2,'ACTIVE',$3,$3
		FROM xz_user_roles scope
		WHERE scope.user_id=$1 AND scope.tenant_id=$4 AND scope.role='USER' AND upper(scope.status)='ACTIVE'
		ON CONFLICT(user_id,tenant_id,organization_id,role) DO UPDATE SET status='ACTIVE',updated_at=excluded.updated_at
	`, item.ApplicantUserID, role, now, item.TenantID)
	if err != nil {
		return err
	}
	if count, countErr := result.RowsAffected(); countErr != nil || count == 0 {
		if countErr != nil {
			return countErr
		}
		return fmt.Errorf("applicant has no active USER RBAC scope: %w", ErrConstraintViolation)
	}
	result, err = tx.ExecContext(ctx, `UPDATE xz_orders SET status='COMPLETED',order_status='COMPLETED',fulfillment_status='FULFILLED',fulfilled_at=$2,price_snapshot=jsonb_set(coalesce(price_snapshot,'{}'::jsonb),'{fulfillmentStatus}','"FULFILLED"'::jsonb,true) WHERE id=$1 AND coalesce(order_status,status) IN ('PAID','COMPLETED')`, item.OrderID, now.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if count, countErr := result.RowsAffected(); countErr != nil || count != 1 {
		if countErr != nil {
			return countErr
		}
		return fmt.Errorf("commercial order was not paid: %w", ErrPaymentNotSuccessful)
	}
	_ = payment
	return nil
}

func prepareRejectedRefundTx(ctx context.Context, store Store, item *OperationCenterServiceOrder, command ReviewCommand, payment reviewPaymentSnapshot, transactionGroupID string, now time.Time) (*OperationCenterRefundTask, error) {
	if err := ValidateOperationCenterRefundTransition(item.RefundStatus, OperationCenterRefundPending); err != nil {
		return nil, err
	}
	refundKey := stableWorkflowID("operation_center_refund", item.ID, "review_rejection_full")
	taskID := stableWorkflowID("operation_center_refund_task", item.ID, "full")
	ruleSetID := ""
	if item.CommercialRuleSetID != nil {
		ruleSetID = *item.CommercialRuleSetID
	}
	paymentRecordID := payment.PaymentRecordID
	providerPaymentNo := payment.ProviderPaymentNo
	task := &OperationCenterRefundTask{
		ID: taskID, TenantID: item.TenantID, ServiceOrderID: item.ID, OrderID: item.OrderID,
		PaymentRecordID: &paymentRecordID, CommercialRuleSetID: ruleSetID,
		Origin: RefundOriginReviewRejection, Scope: RefundScopeFull,
		AmountCents: item.TechnicalServiceFeeCents, Currency: item.Currency,
		PaymentChannel: payment.Channel, ProviderPaymentNo: &providerPaymentNo,
		Status: OperationCenterRefundPending, FailureDetail: JSONSnapshot{},
		IdempotencyKey: refundKey, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateRefundTask(ctx, task); err != nil {
		return nil, err
	}
	refundTransition := OperationCenterStateTransition{
		ID: stableWorkflowID("operation_center_transition", task.ID, "created_pending"), TenantID: task.TenantID,
		EntityType: StateEntityRefundTask, EntityID: task.ID, ToStatus: string(OperationCenterRefundPending),
		TransitionReason: "REVIEW_REJECTED_REFUND_PENDING", TransactionGroupID: transactionGroupID,
		OperatorID: optionalString(command.ReviewedBy), RequestID: optionalString(command.RequestID),
		IdempotencyKey: stableWorkflowID("operation_center_audit", task.ID, "created_pending"),
		Metadata:       JSONSnapshot{"serviceOrderId": item.ID, "decision": string(command.Decision)}, CreatedAt: now,
	}
	if err := store.AppendStateTransition(ctx, &refundTransition); err != nil {
		return nil, err
	}
	item.Status = OperationCenterServiceRejected
	item.RefundStatus = OperationCenterRefundPending
	item.ReviewedAt = timePointer(now)
	item.ReviewedBy = optionalString(command.ReviewedBy)
	item.ReviewIdempotencyKey = optionalString(command.IdempotencyKey)
	item.RefundIdempotencyKey = optionalString(refundKey)
	item.PaymentChannel = optionalString(payment.Channel)
	item.CurrentRefundTaskID = optionalString(task.ID)
	item.StateVersion++
	item.UpdatedAt = now
	transition := reviewServiceTransition(item, command, OperationCenterServiceReviewRequired, OperationCenterServiceRejected, transactionGroupID, now)
	if err := store.UpdateServiceOrder(ctx, item, &transition); err != nil {
		return nil, err
	}
	return task, nil
}

func reviewServiceTransition(item *OperationCenterServiceOrder, command ReviewCommand, from, to OperationCenterServiceStatus, transactionGroupID string, now time.Time) OperationCenterStateTransition {
	return OperationCenterStateTransition{
		ID:       stableWorkflowID("operation_center_transition", item.ID, command.IdempotencyKey, string(to)),
		TenantID: item.TenantID, EntityType: StateEntityServiceOrder, EntityID: item.ID,
		FromStatus: stringPointerValue(string(from)), ToStatus: string(to),
		TransitionReason: "REVIEW_" + string(command.Decision), TransactionGroupID: transactionGroupID,
		OperatorID: optionalString(command.ReviewedBy), RequestID: optionalString(command.RequestID),
		IdempotencyKey: stableWorkflowID("operation_center_audit", item.ID, command.IdempotencyKey, string(to)),
		Metadata:       JSONSnapshot{"decision": string(command.Decision), "reason": command.Reason}, CreatedAt: now,
	}
}

func (service *WorkflowService) GetReviewStatus(ctx context.Context, serviceOrderID string) (ReviewStatusView, error) {
	if service == nil || service.db == nil {
		return ReviewStatusView{}, ErrWorkflowUnavailable
	}
	var view ReviewStatusView
	var decision, eventStatus, reviewKey sql.NullString
	err := service.db.QueryRowContext(ctx, `
		SELECT service.id,service.order_id,service.order_no,service.applicant_user_id,
		       service.status,service.refund_status,service.state_version,
		       review.decision,review.event_status,review.idempotency_key
		FROM xz_operation_center_service_orders service
		LEFT JOIN LATERAL (
		  SELECT decision,event_status,idempotency_key
		  FROM xz_operation_center_review_events
		  WHERE service_order_id=service.id ORDER BY created_at DESC,id DESC LIMIT 1
		) review ON true
		WHERE service.id=$1
	`, serviceOrderID).Scan(&view.ServiceOrderID, &view.OrderID, &view.OrderNo, &view.ApplicantUserID,
		&view.Status, &view.RefundStatus, &view.StateVersion, &decision, &eventStatus, &reviewKey)
	if errors.Is(err, sql.ErrNoRows) {
		return view, ErrNotFound
	}
	if err != nil {
		return view, err
	}
	if decision.Valid {
		value := ReviewDecision(decision.String)
		view.ReviewDecision = &value
	}
	if eventStatus.Valid {
		value := ReviewEventStatus(eventStatus.String)
		view.ReviewEventStatus = &value
	}
	view.ReviewIdempotencyKey = nullableStringPointer(reviewKey)
	return view, nil
}

func databaseNow(ctx context.Context, tx *sql.Tx) (time.Time, error) {
	var now time.Time
	err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now)
	return now.UTC(), err
}

func stableWorkflowID(prefix string, parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return prefix + "_" + hex.EncodeToString(digest[:12])
}

func serviceStatusForDecision(decision ReviewDecision) OperationCenterServiceStatus {
	if decision == ReviewApproved {
		return OperationCenterServiceActive
	}
	return OperationCenterServiceRejected
}

func isSuccessfulPaymentStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "SUCCESS", "SUCCEEDED", "PAID", "COMPLETED":
		return true
	default:
		return false
	}
}

func isPaidOrderStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "PAID", "COMPLETED":
		return true
	default:
		return false
	}
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func stringPointerValue(value string) *string { return &value }

func timePointer(value time.Time) *time.Time { return &value }
