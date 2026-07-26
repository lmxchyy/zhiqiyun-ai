package operationcenter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/payment"
)

var manualVoucherHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type RefundManagementService struct {
	db    *sql.DB
	store *PostgresStore
}

func NewRefundManagementService(db *sql.DB) (*RefundManagementService, error) {
	store, err := NewPostgresStore(db)
	if err != nil {
		return nil, err
	}
	return &RefundManagementService{db: db, store: store}, nil
}

func (service *RefundManagementService) RequestActiveRefund(ctx context.Context, command RefundRequestCommand) (result RefundManagementResult, err error) {
	if service == nil || service.db == nil || strings.TrimSpace(command.ServiceOrderID) == "" ||
		strings.TrimSpace(command.IdempotencyKey) == "" || strings.TrimSpace(command.RequestedBy) == "" ||
		strings.TrimSpace(command.Reason) == "" {
		return result, ErrConstraintViolation
	}
	if command.ExpectedServiceStatus != OperationCenterServiceActive {
		return result, ErrExpectedServiceStatus
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	boundValue, err := service.store.BindTx(tx)
	if err != nil {
		return result, err
	}
	bound := boundValue.(Store)
	item, err := bound.GetServiceOrderForUpdate(ctx, command.ServiceOrderID)
	if err != nil {
		return result, err
	}
	if existingEvent, eventErr := bound.GetRefundRequestEventByIdempotencyKey(ctx, item.TenantID, command.IdempotencyKey); eventErr == nil {
		if existingEvent.ServiceOrderID != item.ID {
			return result, ErrIdempotencyConflict
		}
		task, taskErr := bound.GetRefundTaskForUpdate(ctx, existingEvent.RefundTaskID)
		if taskErr != nil {
			return result, taskErr
		}
		if err := tx.Commit(); err != nil {
			return result, err
		}
		return RefundManagementResult{RefundTask: task, IdempotentReplay: true}, nil
	} else if !errors.Is(eventErr, ErrNotFound) {
		return result, eventErr
	}
	if item.CurrentRefundTaskID != nil || item.RefundStatus != OperationCenterRefundNone {
		return result, ErrRefundAlreadyRequested
	}
	if item.Status != command.ExpectedServiceStatus {
		return result, ErrExpectedServiceStatus
	}
	if !refundPolicyIsFullOnly(item.RefundPolicySnapshot) {
		return result, ErrRefundPolicyNotFullOnly
	}
	paymentID, err := latestSuccessfulPaymentIDForUpdate(ctx, tx, item.OrderID)
	if err != nil {
		return result, err
	}
	snapshot, err := lockPaidOrderSnapshot(ctx, tx, PaymentSucceededCommand{OrderID: item.OrderID, PaymentRecordID: paymentID})
	if err != nil {
		return result, err
	}
	if err := validatePaymentAmounts(snapshot); err != nil {
		return result, err
	}
	if !isSuccessfulPaymentStatus(snapshot.PaymentStatus) || snapshot.PaymentAmountCents != item.TechnicalServiceFeeCents ||
		item.CommercialRuleSetID == nil || *item.CommercialRuleSetID != snapshot.RuleSetID ||
		item.PlanVersionID == nil || *item.PlanVersionID != snapshot.PlanVersionID {
		return result, ErrPaymentAmountMismatch
	}
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return result, err
	}
	task := &OperationCenterRefundTask{
		ID: stableWorkflowID("operation_center_refund_task", item.ID, "full"), TenantID: item.TenantID,
		ServiceOrderID: item.ID, OrderID: item.OrderID, PaymentRecordID: optionalString(paymentID),
		CommercialRuleSetID: snapshot.RuleSetID, Origin: RefundOriginActiveRevocation, Scope: RefundScopeFull,
		AmountCents: item.TechnicalServiceFeeCents, Currency: item.Currency,
		PaymentChannel: snapshot.PaymentChannel, ProviderPaymentNo: optionalString(snapshot.ProviderPaymentNo),
		Status: OperationCenterRefundPending, FailureDetail: JSONSnapshot{},
		IdempotencyKey: strings.TrimSpace(command.IdempotencyKey), CreatedAt: now, UpdatedAt: now,
	}
	if err := bound.CreateRefundTask(ctx, task); err != nil {
		return result, err
	}
	event := &RefundRequestEvent{
		ID: stableWorkflowID("operation_center_refund_request", item.ID, command.IdempotencyKey), TenantID: item.TenantID,
		ServiceOrderID: item.ID, RefundTaskID: task.ID, RequestedBy: command.RequestedBy,
		RequestID: strings.TrimSpace(command.RequestID), IdempotencyKey: task.IdempotencyKey,
		Reason: strings.TrimSpace(command.Reason), ExpectedServiceStatus: command.ExpectedServiceStatus,
		Snapshot: JSONSnapshot{"amountCents": task.AmountCents, "currency": task.Currency, "scope": string(task.Scope), "origin": "ACTIVE_REFUND"}, CreatedAt: now,
	}
	if err := bound.CreateRefundRequestEvent(ctx, event); err != nil {
		return result, err
	}
	transition := newRefundTaskTransition(task, nil, OperationCenterRefundPending, "ACTIVE_REFUND_REQUESTED", command.RequestedBy, command.RequestID, command.IdempotencyKey, command.Reason, now)
	if err := bound.AppendStateTransition(ctx, &transition); err != nil {
		return result, err
	}
	item.RefundStatus = OperationCenterRefundPending
	item.RefundOrderID = optionalString(task.ID)
	item.RefundIdempotencyKey = optionalString(task.IdempotencyKey)
	item.PaymentChannel = optionalString(task.PaymentChannel)
	item.CurrentRefundTaskID = optionalString(task.ID)
	item.StateVersion++
	item.UpdatedAt = now
	if err := bound.UpdateServiceOrderRefundProjection(ctx, item); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return RefundManagementResult{RefundTask: task}, nil
}

func (service *RefundManagementService) GetRefund(ctx context.Context, taskID string) (RefundManagementView, error) {
	if service == nil || strings.TrimSpace(taskID) == "" {
		return RefundManagementView{}, ErrConstraintViolation
	}
	task, err := service.store.GetRefundTask(ctx, taskID)
	if err != nil {
		return RefundManagementView{}, err
	}
	item, err := service.store.GetServiceOrder(ctx, task.ServiceOrderID)
	if err != nil {
		return RefundManagementView{}, err
	}
	view := refundManagementView(*item, *task)
	manual, manualErr := scanManualRefund(service.db.QueryRowContext(ctx, `SELECT `+manualRefundColumns+` FROM xz_operation_center_manual_refunds WHERE refund_task_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1`, task.ID))
	if manualErr == nil {
		view.ManualRefundStatus = &manual.Status
		view.ManualRefundID = optionalString(manual.ID)
		view.ManualVoucherReference = optionalString(manual.VoucherReference)
	} else if !errors.Is(manualErr, ErrNotFound) {
		return RefundManagementView{}, manualErr
	}
	rows, err := service.db.QueryContext(ctx, `SELECT from_status,to_status,action,actor_id,request_id,created_at FROM xz_operation_center_state_transitions WHERE entity_type='REFUND_TASK' AND entity_id=$1 ORDER BY created_at DESC,id DESC LIMIT 20`, task.ID)
	if err != nil {
		return RefundManagementView{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var audit RefundAuditSummary
		var from, actor, request sql.NullString
		if err := rows.Scan(&from, &audit.ToStatus, &audit.Reason, &actor, &request, &audit.CreatedAt); err != nil {
			return RefundManagementView{}, err
		}
		if from.Valid {
			audit.FromStatus = from.String
		}
		audit.OperatorID = nullableStringPointer(actor)
		audit.RequestID = nullableStringPointer(request)
		view.Audit = append(view.Audit, audit)
	}
	return view, rows.Err()
}

func (service *RefundManagementService) ListRefunds(ctx context.Context, filter RefundListFilter) ([]RefundManagementView, error) {
	if service == nil {
		return nil, ErrConstraintViolation
	}
	ids, err := service.store.listRefundTaskIDs(ctx, filter)
	if err != nil {
		return nil, err
	}
	result := make([]RefundManagementView, 0, len(ids))
	for _, id := range ids {
		view, viewErr := service.GetRefund(ctx, id)
		if viewErr != nil {
			return nil, viewErr
		}
		result = append(result, view)
	}
	return result, nil
}

func (service *RefundManagementService) SubmitManualRefund(ctx context.Context, command ManualRefundSubmitCommand) (result RefundManagementResult, err error) {
	if service == nil || service.db == nil || !validManualSubmitCommand(command) {
		return result, ErrConstraintViolation
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	boundValue, err := service.store.BindTx(tx)
	if err != nil {
		return result, err
	}
	bound := boundValue.(Store)
	serviceID, err := refundTaskServiceID(ctx, tx, command.RefundTaskID)
	if err != nil {
		return result, err
	}
	serviceOrder, err := bound.GetServiceOrderForUpdate(ctx, serviceID)
	if err != nil {
		return result, err
	}
	task, err := bound.GetRefundTaskForUpdate(ctx, command.RefundTaskID)
	if err != nil {
		return result, err
	}
	if event, eventErr := bound.GetManualRefundEventByIdempotencyKey(ctx, task.TenantID, command.IdempotencyKey); eventErr == nil {
		if event.RefundTaskID != task.ID || event.EventType != "SUBMITTED" {
			return result, ErrManualRefundConflict
		}
		manual, manualErr := bound.GetManualRefundByID(ctx, event.ManualRefundID)
		if manualErr != nil {
			return result, manualErr
		}
		if err := tx.Commit(); err != nil {
			return result, err
		}
		return RefundManagementResult{RefundTask: task, ManualRefund: manual, IdempotentReplay: true}, nil
	} else if !errors.Is(eventErr, ErrNotFound) {
		return result, eventErr
	}
	if task.Status != OperationCenterRefundManualRequired || serviceOrder.Status == OperationCenterServiceActive {
		return result, ErrRefundSagaNotReady
	}
	if command.RefundAmountCents != task.AmountCents {
		return result, ErrPaymentAmountMismatch
	}
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return result, err
	}
	manual, lookupErr := bound.GetManualRefundByProviderTransactionForUpdate(ctx, task.ID, command.ChannelRefundNo)
	if errors.Is(lookupErr, ErrNotFound) {
		manual = &OperationCenterManualRefund{
			ID: stableWorkflowID("operation_center_manual_refund", task.ID, command.ChannelRefundNo), TenantID: task.TenantID,
			RefundTaskID: task.ID, PaymentChannel: task.PaymentChannel, AmountCents: task.AmountCents,
			Currency: task.Currency, ProviderTransactionID: command.ChannelRefundNo, ProviderRefundNo: optionalString(command.ChannelRefundNo),
			VoucherReference: command.VoucherReference, VoucherFileHash: strings.ToLower(command.VoucherFileHash),
			Status: ManualRefundSubmitted, SubmittedBy: command.SubmittedBy, SubmittedAt: now,
			Remark: optionalString(command.Reason), CreatedAt: now, UpdatedAt: now,
		}
		if err := bound.SubmitManualRefund(ctx, manual); err != nil {
			return result, err
		}
	} else if lookupErr != nil {
		return result, lookupErr
	} else {
		if manual.Status != ManualRefundRejected {
			return result, ErrManualRefundConflict
		}
		manual.Status = ManualRefundSubmitted
		manual.VoucherReference = command.VoucherReference
		manual.VoucherFileHash = strings.ToLower(command.VoucherFileHash)
		manual.SubmittedBy = command.SubmittedBy
		manual.SubmittedAt = now
		manual.ApprovedBy, manual.ApprovedAt, manual.RejectionReason = nil, nil, nil
		manual.Remark, manual.UpdatedAt = optionalString(command.Reason), now
		if err := bound.UpdateManualRefundRecord(ctx, manual); err != nil {
			return result, err
		}
	}
	from := task.Status
	task.ManualProviderTransactionID = optionalString(command.ChannelRefundNo)
	task.ManualVoucherReference = optionalString(command.VoucherReference)
	task.ManualVoucherFileHash = optionalString(strings.ToLower(command.VoucherFileHash))
	task.ManualSubmittedBy = optionalString(command.SubmittedBy)
	task.ManualApprovedBy = nil
	task.LeaseOwner, task.LeaseExpiresAt, task.NextRetryAt = nil, nil, nil
	if err := advanceManagedRefundTask(ctx, bound, task, OperationCenterRefundManualSubmitted, "MANUAL_REFUND_SUBMITTED", command.SubmittedBy, command.RequestID, command.IdempotencyKey, command.Reason, now); err != nil {
		return result, err
	}
	serviceOrder.RefundStatus = task.Status
	serviceOrder.ManualRefundVoucherReference = task.ManualVoucherReference
	serviceOrder.ManualRefundVoucherFileHash = task.ManualVoucherFileHash
	serviceOrder.ManualRefundSubmittedBy = task.ManualSubmittedBy
	serviceOrder.ManualRefundApprovedBy = nil
	serviceOrder.StateVersion++
	serviceOrder.UpdatedAt = now
	if err := bound.UpdateServiceOrderRefundProjection(ctx, serviceOrder); err != nil {
		return result, err
	}
	event := manualRefundEvent(task, manual, "SUBMITTED", from, task.Status, command.SubmittedBy, command.RequestID, command.IdempotencyKey, command.Reason, now)
	if err := bound.CreateManualRefundEvent(ctx, &event); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return RefundManagementResult{RefundTask: task, ManualRefund: manual}, nil
}

func (service *RefundManagementService) ReviewManualRefund(ctx context.Context, command ManualRefundReviewCommand) (result RefundManagementResult, err error) {
	decision := strings.ToUpper(strings.TrimSpace(command.Decision))
	if service == nil || service.db == nil || strings.TrimSpace(command.RefundTaskID) == "" || strings.TrimSpace(command.IdempotencyKey) == "" || strings.TrimSpace(command.ReviewedBy) == "" || strings.TrimSpace(command.Reason) == "" || command.ExpectedStatus != ManualRefundSubmitted || (decision != "APPROVED" && decision != "REJECTED") {
		return result, ErrConstraintViolation
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	boundValue, err := service.store.BindTx(tx)
	if err != nil {
		return result, err
	}
	bound := boundValue.(Store)
	serviceID, err := refundTaskServiceID(ctx, tx, command.RefundTaskID)
	if err != nil {
		return result, err
	}
	serviceOrder, err := bound.GetServiceOrderForUpdate(ctx, serviceID)
	if err != nil {
		return result, err
	}
	task, err := bound.GetRefundTaskForUpdate(ctx, command.RefundTaskID)
	if err != nil {
		return result, err
	}
	if event, eventErr := bound.GetManualRefundEventByIdempotencyKey(ctx, task.TenantID, command.IdempotencyKey); eventErr == nil {
		if event.RefundTaskID != task.ID || event.EventType != decision {
			return result, ErrManualRefundConflict
		}
		manual, manualErr := bound.GetManualRefundByID(ctx, event.ManualRefundID)
		if manualErr != nil {
			return result, manualErr
		}
		if err := tx.Commit(); err != nil {
			return result, err
		}
		return RefundManagementResult{RefundTask: task, ManualRefund: manual, IdempotentReplay: true}, nil
	} else if !errors.Is(eventErr, ErrNotFound) {
		return result, eventErr
	}
	if task.Status != OperationCenterRefundManualSubmitted || serviceOrder.Status == OperationCenterServiceActive {
		return result, ErrManualRefundNotSubmitted
	}
	manual, err := bound.GetLatestManualRefundForUpdate(ctx, task.ID)
	if err != nil {
		return result, err
	}
	if manual.Status != command.ExpectedStatus {
		return result, ErrManualRefundNotSubmitted
	}
	if manual.SubmittedBy == command.ReviewedBy {
		return result, ErrManualRefundSelfApproval
	}
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return result, err
	}
	from := task.Status
	to := OperationCenterRefundManualRequired
	if decision == "APPROVED" {
		manual.Status = ManualRefundApproved
		manual.ApprovedBy = optionalString(command.ReviewedBy)
		manual.ApprovedAt = timePointer(now)
		manual.RejectionReason = nil
		to = OperationCenterRefundSucceeded
		task.ProviderRefundNo = optionalString(manual.ProviderTransactionID)
		task.ProviderRefundedAt = timePointer(now)
		task.CompletedAt = timePointer(now)
		task.ManualApprovedBy = optionalString(command.ReviewedBy)
	} else {
		manual.Status = ManualRefundRejected
		manual.ApprovedBy, manual.ApprovedAt = nil, nil
		manual.RejectionReason = optionalString(command.Reason)
		task.ManualProviderTransactionID, task.ManualVoucherReference, task.ManualVoucherFileHash = nil, nil, nil
		task.ManualSubmittedBy, task.ManualApprovedBy = nil, nil
	}
	manual.UpdatedAt = now
	if err := bound.UpdateManualRefundRecord(ctx, manual); err != nil {
		return result, err
	}
	if err := advanceManagedRefundTask(ctx, bound, task, to, "MANUAL_REFUND_"+decision, command.ReviewedBy, command.RequestID, command.IdempotencyKey, command.Reason, now); err != nil {
		return result, err
	}
	serviceOrder.RefundStatus = task.Status
	serviceOrder.ProviderRefundNo = task.ProviderRefundNo
	serviceOrder.ManualRefundVoucherReference = task.ManualVoucherReference
	serviceOrder.ManualRefundVoucherFileHash = task.ManualVoucherFileHash
	serviceOrder.ManualRefundSubmittedBy = task.ManualSubmittedBy
	serviceOrder.ManualRefundApprovedBy = task.ManualApprovedBy
	serviceOrder.StateVersion++
	serviceOrder.UpdatedAt = now
	if err := bound.UpdateServiceOrderRefundProjection(ctx, serviceOrder); err != nil {
		return result, err
	}
	event := manualRefundEvent(task, manual, decision, from, to, command.ReviewedBy, command.RequestID, command.IdempotencyKey, command.Reason, now)
	if err := bound.CreateManualRefundEvent(ctx, &event); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return RefundManagementResult{RefundTask: task, ManualRefund: manual}, nil
}

func latestSuccessfulPaymentIDForUpdate(ctx context.Context, tx *sql.Tx, orderID string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, `SELECT id FROM xz_payment_records WHERE order_id=$1 AND upper(coalesce(payment_status,prepay_status,'')) IN ('PAID','SUCCESS','SUCCEEDED','COMPLETED') ORDER BY paid_at DESC NULLS LAST,created_at DESC,id DESC LIMIT 1 FOR UPDATE`, orderID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrPaymentNotSuccessful
	}
	return id, err
}

func refundPolicyIsFullOnly(snapshot JSONSnapshot) bool {
	for _, key := range []string{"mode", "refundMode", "policy"} {
		if value, ok := snapshot[key].(string); ok && strings.EqualFold(strings.TrimSpace(value), "FULL_ONLY") {
			return true
		}
	}
	for _, value := range snapshot {
		if nested, ok := value.(map[string]any); ok && refundPolicyIsFullOnly(JSONSnapshot(nested)) {
			return true
		}
	}
	return false
}

func validManualSubmitCommand(command ManualRefundSubmitCommand) bool {
	return strings.TrimSpace(command.RefundTaskID) != "" && strings.TrimSpace(command.IdempotencyKey) != "" &&
		strings.TrimSpace(command.ChannelRefundNo) != "" && command.RefundAmountCents > 0 &&
		strings.TrimSpace(command.VoucherReference) != "" && manualVoucherHashPattern.MatchString(strings.ToLower(strings.TrimSpace(command.VoucherFileHash))) &&
		strings.TrimSpace(command.Reason) != "" && strings.TrimSpace(command.SubmittedBy) != ""
}

func refundTaskServiceID(ctx context.Context, tx *sql.Tx, taskID string) (string, error) {
	var serviceID string
	err := tx.QueryRowContext(ctx, `SELECT service_order_id FROM xz_operation_center_refund_tasks WHERE id=$1`, taskID).Scan(&serviceID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return serviceID, err
}

func advanceManagedRefundTask(ctx context.Context, store Store, task *OperationCenterRefundTask, to OperationCenterRefundStatus, reason, actor, requestID, key, detail string, now time.Time) error {
	from := task.Status
	if err := ValidateOperationCenterRefundTransition(from, to); err != nil {
		return err
	}
	task.Status = to
	task.StateVersion++
	task.UpdatedAt = now
	transition := newRefundTaskTransition(task, &from, to, reason, actor, requestID, key, detail, now)
	return store.UpdateRefundTask(ctx, task, &transition)
}

func newRefundTaskTransition(task *OperationCenterRefundTask, from *OperationCenterRefundStatus, to OperationCenterRefundStatus, reason, actor, requestID, key, detail string, now time.Time) OperationCenterStateTransition {
	var fromValue *string
	if from != nil {
		value := string(*from)
		fromValue = &value
	}
	return OperationCenterStateTransition{
		ID: stableWorkflowID("operation_center_transition", task.ID, key, string(to)), TenantID: task.TenantID,
		EntityType: StateEntityRefundTask, EntityID: task.ID, FromStatus: fromValue, ToStatus: string(to),
		TransitionReason: reason, TransactionGroupID: stableWorkflowID("operation_center_management_tx", task.ID, key),
		OperatorID: optionalString(actor), RequestID: optionalString(requestID),
		IdempotencyKey: stableWorkflowID("operation_center_management_audit", task.ID, key, string(to)),
		Metadata:       JSONSnapshot{"reason": detail}, CreatedAt: now,
	}
}

func manualRefundEvent(task *OperationCenterRefundTask, manual *OperationCenterManualRefund, eventType string, before, after OperationCenterRefundStatus, actor, requestID, key, reason string, now time.Time) ManualRefundEvent {
	return ManualRefundEvent{
		ID: stableWorkflowID("operation_center_manual_event", task.ID, key), TenantID: task.TenantID,
		RefundTaskID: task.ID, ManualRefundID: manual.ID, EventType: eventType, ActorID: actor,
		RequestID: strings.TrimSpace(requestID), IdempotencyKey: key, Reason: reason,
		BeforeStatus: before, AfterStatus: after,
		Snapshot: JSONSnapshot{"amountCents": manual.AmountCents, "channelRefundNo": manual.ProviderTransactionID, "voucherReference": manual.VoucherReference}, CreatedAt: now,
	}
}

func refundManagementView(service OperationCenterServiceOrder, task OperationCenterRefundTask) RefundManagementView {
	view := RefundManagementView{
		RefundTaskID: task.ID, TenantID: task.TenantID, ServiceOrderID: task.ServiceOrderID, OrderID: task.OrderID,
		ServiceStatus: service.Status, RefundStatus: task.Status, AmountCents: task.AmountCents,
		Currency: task.Currency, PaymentChannel: task.PaymentChannel, ProviderResult: task.ProviderOutcome,
		ProviderRefundNo: task.ProviderRefundNo, AttemptCount: task.AttemptCount,
		VerificationAttemptCount: task.VerificationAttemptCount, NextRetryAt: task.NextRetryAt,
		LastVerificationAt: task.LastVerificationAt, CreatedAt: timePointer(task.CreatedAt),
		ProviderResponseSummary:      snapshotProviderSummary(payment.ProviderResponseSummary(task.ProviderResponseSummary)),
		ProviderQueryResponseSummary: snapshotProviderSummary(payment.ProviderResponseSummary(task.ProviderQueryResponseSummary)),
		RequiresManualAction:         task.Status == OperationCenterRefundManualRequired || task.Status == OperationCenterRefundManualSubmitted,
	}
	if task.FailureClass != nil {
		value := string(*task.FailureClass)
		view.FailureClass = &value
	}
	return view
}

var _ = fmt.Sprintf
