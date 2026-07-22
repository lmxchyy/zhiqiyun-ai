package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type storedIdentityPreview struct {
	id, userID, actorID, actorRole, action, method, targetIdentity, status, sourceMembershipOrderID string
	highRisk                                                                                        bool
	expiresAt                                                                                       time.Time
	request                                                                                         identityChangePreviewRequest
	result                                                                                          identityChangePreviewResult
}

func (s *postgresStore) ReviewAdminIdentityChange(actorID, actorRole, userID string, request identityChangeReviewRequest) (identityChangePreviewResult, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if actorID == "" || strings.TrimSpace(request.PreviewToken) == "" || strings.TrimSpace(request.Reason) == "" {
		return identityChangePreviewResult{}, fmt.Errorf("%w: preview token and review reason are required", errIdentityChangeInvalid)
	}
	if !identityChangeAdminRoleAllowed(actorRole) {
		return identityChangePreviewResult{}, errIdentityPermission
	}
	decision := strings.ToUpper(strings.TrimSpace(request.Decision))
	if decision != "APPROVED" && decision != "REJECTED" {
		return identityChangePreviewResult{}, fmt.Errorf("%w: review decision must be APPROVED or REJECTED", errIdentityChangeInvalid)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return identityChangePreviewResult{}, err
	}
	defer tx.Rollback()
	preview, err := loadStoredIdentityPreviewTx(ctx, tx, request.PreviewToken, true)
	if err != nil {
		return identityChangePreviewResult{}, err
	}
	if preview.userID != userID {
		return identityChangePreviewResult{}, errIdentityPreviewNotFound
	}
	if !preview.result.ReviewRequired || preview.status != "REVIEW_REQUIRED" {
		return identityChangePreviewResult{}, errIdentityReviewRequired
	}
	if preview.actorID == actorID {
		return identityChangePreviewResult{}, fmt.Errorf("%w: reviewer must be different from preview operator", errIdentityPermission)
	}
	if time.Now().UTC().After(preview.expiresAt) {
		return identityChangePreviewResult{}, errIdentityPreviewExpired
	}
	approvalID := "identity_approval_" + shortID(preview.id+actorID)
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_identity_change_approvals(id,preview_id,reviewer_id,reviewer_role,decision,reason) VALUES($1,$2,$3,$4,$5,$6)`, approvalID, preview.id, actorID, actorRole, decision, strings.TrimSpace(request.Reason)); err != nil {
		return identityChangePreviewResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE xz_identity_change_previews SET status=$2,updated_at=now() WHERE id=$1`, preview.id, decision); err != nil {
		return identityChangePreviewResult{}, err
	}
	if err := insertAuditLog(ctx, tx, actorID, actorRole, "admin.identity_change.review", "user_identity", userID, "POST", "/api/v1/admin/users/"+userID+"/identity-change/review", http.StatusOK, map[string]any{"previewId": preview.id, "decision": decision, "reason": strings.TrimSpace(request.Reason)}); err != nil {
		return identityChangePreviewResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return identityChangePreviewResult{}, err
	}
	preview.result.Status = decision
	preview.result.PreviewToken = ""
	return preview.result, nil
}

func (s *postgresStore) ConfirmAdminIdentityChange(actorID, actorRole, userID string, request identityChangeConfirmRequest) (identityChangeConfirmResult, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if actorID == "" || strings.TrimSpace(request.PreviewToken) == "" {
		return identityChangeConfirmResult{}, fmt.Errorf("%w: preview token is required", errIdentityChangeInvalid)
	}
	if !identityChangeAdminRoleAllowed(actorRole) {
		return identityChangeConfirmResult{}, errIdentityPermission
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return identityChangeConfirmResult{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, identityPreviewTokenHash(request.PreviewToken)); err != nil {
		return identityChangeConfirmResult{}, err
	}
	preview, err := loadStoredIdentityPreviewTx(ctx, tx, request.PreviewToken, true)
	if err != nil {
		return identityChangeConfirmResult{}, err
	}
	if preview.userID != userID {
		return identityChangeConfirmResult{}, errIdentityPreviewNotFound
	}
	if preview.status == "CONSUMED" {
		result, err := loadIdentityExecutionResultTx(ctx, tx, preview.id)
		if err != nil {
			return identityChangeConfirmResult{}, err
		}
		result.Idempotent = true
		if err := tx.Commit(); err != nil {
			return identityChangeConfirmResult{}, err
		}
		return result, nil
	}
	if time.Now().UTC().After(preview.expiresAt) {
		return identityChangeConfirmResult{}, errIdentityPreviewExpired
	}
	if preview.status == "BLOCKED" || len(preview.result.Blockers) > 0 || preview.status == "REJECTED" {
		return identityChangeConfirmResult{}, errIdentityChangeBlocked
	}
	if preview.result.ReviewRequired && preview.status != "APPROVED" {
		return identityChangeConfirmResult{}, errIdentityReviewRequired
	}
	if preview.highRisk && !request.HighRiskConfirmed {
		return identityChangeConfirmResult{}, errIdentityHighRiskConfirm
	}
	if preview.targetIdentity == "OPERATION_CENTER" && preview.action == "UPGRADE" && !strings.EqualFold(actorRole, "SUPER_ADMIN") {
		return identityChangeConfirmResult{}, errIdentityPermission
	}
	if err := lockIdentityCommandUserTx(ctx, tx, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identityChangeConfirmResult{}, errIdentityUserNotFound
		}
		return identityChangeConfirmResult{}, err
	}
	recomputed, err := s.computeIdentityChangePreviewTx(ctx, tx, preview.actorID, preview.actorRole, userID, preview.request)
	if err != nil {
		return identityChangeConfirmResult{}, err
	}
	if len(recomputed.result.Blockers) > 0 || recomputed.result.OldIdentity != preview.result.OldIdentity || recomputed.result.TargetIdentity != preview.result.TargetIdentity || recomputed.result.PaidAmountCents != preview.result.PaidAmountCents || recomputed.result.TokenDelta != preview.result.TokenDelta || recomputed.result.SourceMembershipOrderID != preview.result.SourceMembershipOrderID {
		return identityChangeConfirmResult{}, fmt.Errorf("%w: critical state changed after preview", errIdentityChangeBlocked)
	}
	currentRelation, err := currentIdentityRelationshipTx(ctx, tx, userID)
	if err != nil {
		return identityChangeConfirmResult{}, err
	}
	currentRelationSnapshot := mapStringSnapshot(identityRelationshipSnapshot(currentRelation))
	previewRelationSnapshot := mapStringSnapshot(preview.result.RelationshipBefore)
	if currentRelationSnapshot["parentAgentId"] != previewRelationSnapshot["parentAgentId"] || currentRelationSnapshot["operationCenterId"] != previewRelationSnapshot["operationCenterId"] {
		return identityChangeConfirmResult{}, fmt.Errorf("%w: relationship changed after preview", errIdentityChangeBlocked)
	}
	if preview.method == identityMethodPackageConversion {
		var sourceStatus string
		if err := tx.QueryRowContext(ctx, `SELECT upper(coalesce(status,'')) FROM xz_orders WHERE id=$1 AND user_id=$2 FOR UPDATE`, preview.sourceMembershipOrderID, userID).Scan(&sourceStatus); err != nil {
			return identityChangeConfirmResult{}, fmt.Errorf("%w: membership source order is unavailable", errIdentityChangeBlocked)
		}
		if sourceStatus != "PAID" && sourceStatus != "SUCCEEDED" {
			return identityChangeConfirmResult{}, fmt.Errorf("%w: membership source order is no longer paid", errIdentityChangeBlocked)
		}
	}
	executionID := "identity_execution_" + shortID(preview.id)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_identity_change_executions(id,preview_id,user_id,actor_id,actor_role,change_action,change_method,source_membership_order_id,status)
		VALUES($1,$2,$3,$4,$5,$6,$7,nullif($8,''),'PROCESSING')
	`, executionID, preview.id, userID, actorID, actorRole, preview.action, preview.method, preview.sourceMembershipOrderID); err != nil {
		return identityChangeConfirmResult{}, err
	}

	orderID := ""
	if preview.result.PaidAmountCents > 0 && (preview.method == identityMethodOfflineOrder || preview.method == identityMethodPackageConversion) {
		orderID, err = createIdentityOfflineOrderTx(ctx, tx, executionID, preview)
		if err != nil {
			return identityChangeConfirmResult{}, err
		}
	}
	if preview.result.TokenDelta != 0 {
		if err := applyIdentityTokenDeltaTx(ctx, tx, preview, executionID, orderID); err != nil {
			return identityChangeConfirmResult{}, err
		}
	}
	commissionTotal := int64(0)
	if preview.result.CommissionGenerated {
		if orderID == "" {
			return identityChangeConfirmResult{}, errors.New("commission requires a persisted business order")
		}
		commissionTotal, err = createIdentityCommissionsTx(ctx, tx, preview, orderID)
		if err != nil {
			return identityChangeConfirmResult{}, err
		}
	}
	mutationResult, err := applyIdentityMutationTx(ctx, tx, preview, actorID, orderID, executionID)
	if err != nil {
		return identityChangeConfirmResult{}, err
	}
	mutationResult["tokenDelta"] = preview.result.TokenDelta
	mutationResult["paidAmountCents"] = preview.result.PaidAmountCents
	mutationResult["commissionTotalCents"] = commissionTotal
	mutationResult["orderId"] = orderID
	resultPayload := jsonProjection(mutationResult)
	if _, err := tx.ExecContext(ctx, `UPDATE xz_identity_change_executions SET order_id=nullif($2,''),status='SUCCEEDED',result_snapshot=$3::jsonb,completed_at=now() WHERE id=$1`, executionID, orderID, resultPayload); err != nil {
		return identityChangeConfirmResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE xz_identity_change_previews SET status='CONSUMED',consumed_at=now(),execution_id=$2,updated_at=now() WHERE id=$1`, preview.id, executionID); err != nil {
		return identityChangeConfirmResult{}, err
	}
	if err := insertAuditLog(ctx, tx, actorID, actorRole, "admin.identity_change.confirm", "user_identity", userID, "POST", "/api/v1/admin/users/"+userID+"/identity-change/confirm", http.StatusOK, map[string]any{
		"previewId": preview.id, "executionId": executionID, "action": preview.action, "method": preview.method, "reason": preview.request.Reason, "orderId": orderID,
		"rolesBefore": mutationResult["rolesBefore"], "rolesAfter": mutationResult["rolesAfter"],
	}); err != nil {
		return identityChangeConfirmResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_operation_logs(id,actor_id,operation,target,target_id,before_state,after_state) VALUES($1,$2,'IDENTITY_CHANGE','user_identity',$3,$4::jsonb,$5::jsonb)`, "operation_"+shortID(executionID), actorID, userID, jsonProjection(map[string]any{"identity": preview.result.OldIdentity, "relationship": preview.result.RelationshipBefore}), resultPayload); err != nil {
		return identityChangeConfirmResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return identityChangeConfirmResult{}, err
	}
	return identityChangeConfirmResult{ExecutionID: executionID, PreviewID: preview.id, UserID: userID, Status: "SUCCEEDED", OrderID: orderID, Result: mutationResult}, nil
}

func loadStoredIdentityPreviewTx(ctx context.Context, tx *sql.Tx, token string, lock bool) (storedIdentityPreview, error) {
	query := `SELECT id,user_id,actor_id,actor_role,change_action,change_method,coalesce(target_identity,''),request_snapshot,result_snapshot,status,high_risk,coalesce(source_membership_order_id,''),expires_at FROM xz_identity_change_previews WHERE token_hash=$1`
	if lock {
		query += " FOR UPDATE"
	}
	var item storedIdentityPreview
	var requestRaw, resultRaw []byte
	err := tx.QueryRowContext(ctx, query, identityPreviewTokenHash(token)).Scan(&item.id, &item.userID, &item.actorID, &item.actorRole, &item.action, &item.method, &item.targetIdentity, &requestRaw, &resultRaw, &item.status, &item.highRisk, &item.sourceMembershipOrderID, &item.expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return storedIdentityPreview{}, errIdentityPreviewNotFound
	}
	if err != nil {
		return storedIdentityPreview{}, err
	}
	if json.Unmarshal(requestRaw, &item.request) != nil || json.Unmarshal(resultRaw, &item.result) != nil {
		return storedIdentityPreview{}, errors.New("invalid stored identity preview snapshot")
	}
	return item, nil
}

func loadIdentityExecutionResultTx(ctx context.Context, tx *sql.Tx, previewID string) (identityChangeConfirmResult, error) {
	var item identityChangeConfirmResult
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT id,preview_id,user_id,status,coalesce(order_id,''),result_snapshot FROM xz_identity_change_executions WHERE preview_id=$1 AND status='SUCCEEDED'`, previewID).Scan(&item.ExecutionID, &item.PreviewID, &item.UserID, &item.Status, &item.OrderID, &raw)
	if err != nil {
		return identityChangeConfirmResult{}, err
	}
	if err := json.Unmarshal(raw, &item.Result); err != nil {
		return identityChangeConfirmResult{}, err
	}
	return item, nil
}

func createIdentityOfflineOrderTx(ctx context.Context, tx *sql.Tx, executionID string, preview storedIdentityPreview) (string, error) {
	plan, err := loadIdentityChangePlanTx(ctx, tx, preview.request.PlanID)
	if err != nil {
		return "", err
	}
	orderID := "order_identity_" + shortID(executionID)
	orderNo := "IDOFF-" + strings.ToUpper(shortID(executionID))
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if parsed, parseErr := time.Parse(time.RFC3339, preview.request.PaymentProof.PaidAt); parseErr == nil {
		now = parsed.UTC().Format(time.RFC3339Nano)
	}
	order := adminOrder{
		ID: orderID, OrderNo: orderNo, TenantID: "tenant_default", UserID: preview.userID, BuyerUserID: preview.userID,
		PlanID: plan.ID, OrderType: "IDENTITY_UPGRADE", BusinessOrderType: businessOrderTypeForPlanType(planBusinessType(plan)),
		AmountCents: int(preview.result.PaidAmountCents), Amount: int(preview.result.PaidAmountCents), Status: "PAID", PaidAt: now, CreatedAt: now,
		FulfillmentStatus: "FULFILLED", FulfilledAt: now, PriceSnapshot: map[string]any{
			"identityChangeMethod": preview.method, "previewId": preview.id, "executionId": executionID,
			"actualPaidAmountCents": preview.result.PaidAmountCents, "paymentProof": preview.request.PaymentProof,
			"originalPlanPriceCents": plan.PriceCents, "discountAmountCents": preview.result.DiscountAmountCents,
			"discountReason": preview.request.DiscountReason, "specialPrice": preview.result.SpecialPrice, "tokenDelta": preview.result.TokenDelta,
		},
	}
	if err := insertOrder(ctx, tx, order); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE xz_orders SET order_no=$2,product_code=$3,product_name=$4,product_type='IDENTITY',payment_channel='OFFLINE_ADMIN',payment_scene='ADMIN_IDENTITY_CHANGE',payment_mode='OFFLINE_PROOF',entitlement_status='GRANTED',original_amount_cents=$5,payable_amount_cents=$6,order_status='PAID',channel='OFFLINE_ADMIN',platform='ADMIN',idempotency_key=$7,updated_at=now() WHERE id=$1`, orderID, orderNo, plan.Code, plan.Name, plan.PriceCents, preview.result.PaidAmountCents, preview.id); err != nil {
		return "", err
	}
	paymentID := "payment_identity_" + shortID(executionID)
	proof := jsonProjection(preview.request.PaymentProof)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_payment_records(id,payment_no,order_id,order_no,tenant_id,user_id,payment_channel,payment_scene,amount_cents,prepay_status,request_payload,response_payload,callback_payload,paid_at,provider,currency,payment_status)
		VALUES($1,$2,$3,$4,'tenant_default',$5,'OFFLINE_ADMIN','ADMIN_IDENTITY_CHANGE',$6,'SUCCEEDED',$7::jsonb,'{}','{}',now(),'OFFLINE','CNY','SUCCEEDED')
	`, paymentID, "PAY-"+strings.ToUpper(shortID(paymentID)), orderID, orderNo, preview.userID, preview.result.PaidAmountCents, proof); err != nil {
		return "", err
	}
	return orderID, nil
}

func applyIdentityTokenDeltaTx(ctx context.Context, tx *sql.Tx, preview storedIdentityPreview, executionID, orderID string) error {
	delta := int(preview.result.TokenDelta)
	account, err := pointAccountForUpdate(ctx, tx, preview.userID)
	if err != nil {
		return err
	}
	before := account.Available
	after := before + delta
	if after < 0 {
		return errors.New("insufficient token balance for identity change")
	}
	account.Available = after
	if delta > 0 {
		account.TotalGranted += delta
	}
	if err := insertPointAccount(ctx, tx, account); err != nil {
		return err
	}
	absDelta := delta
	if absDelta < 0 {
		absDelta = -absDelta
	}
	if err := insertAccountBalanceLedgerV1(ctx, tx, account, "ADJUSTMENT", absDelta, before, after, "IDENTITY_CHANGE", executionID, preview.request.Reason); err != nil {
		return err
	}
	idempotencyKey := "identity-token:" + executionID
	_, err = tx.ExecContext(ctx, `
		INSERT INTO xz_token_records(id,tenant_id,user_id,order_id,change_type,amount,balance_before,balance_after,remark,created_at,idempotency_key,source_order_no,raw)
		VALUES($1,'tenant_default',$2,nullif($3,''),$4,$5,$6,$7,$8,$9,$10,nullif($11,''),$12::jsonb)
		ON CONFLICT DO NOTHING
	`, "token_identity_"+shortID(executionID), preview.userID, orderID, preview.result.TokenChangeType, delta, before, after, preview.request.Reason, time.Now().UTC().Format(time.RFC3339Nano), idempotencyKey, orderID, jsonProjection(map[string]any{"executionId": executionID, "method": preview.method, "independentGift": preview.method == identityMethodSpecialGrant}))
	return err
}

func createIdentityCommissionsTx(ctx context.Context, tx *sql.Tx, preview storedIdentityPreview, orderID string) (int64, error) {
	plan, err := loadIdentityChangePlanTx(ctx, tx, preview.request.PlanID)
	if err != nil {
		return 0, err
	}
	relation := adminUserRelationship{ParentAgentID: stringValue(preview.result.RelationshipAfter["parentAgentId"]), OperationCenterID: stringValue(preview.result.RelationshipAfter["operationCenterId"])}
	order := adminOrder{ID: orderID, OrderNo: orderID, TenantID: "tenant_default", UserID: preview.userID, PlanID: plan.ID, AmountCents: int(preview.result.PaidAmountCents), Amount: int(preview.result.PaidAmountCents), PaidAt: time.Now().UTC().Format(time.RFC3339Nano), CommissionSnapshotCaptured: true, CommissionRuleSnapshot: preview.result.CommissionRuleSnapshot}
	commerceCtx := commissionOrderContext{OrderID: orderID, PlanType: planBusinessType(plan), AmountCents: int(preview.result.PaidAmountCents), BuyerUserID: preview.userID, DirectAgentID: relation.ParentAgentID, OperationCenterID: relation.OperationCenterID}
	engineResult, err := generateCommissionRecordsForCommerceOrderTx(ctx, tx, order, plan, commerceCtx)
	if err != nil {
		return 0, err
	}
	actual := map[string]int64{}
	for _, record := range engineResult.Records {
		actual[string(record.BeneficiaryType)+"|"+record.BeneficiaryID+"|"+record.RuleID] = int64(record.AmountCents)
	}
	for _, expected := range preview.result.EstimatedCommissions {
		if actual[expected.BeneficiaryType+"|"+expected.BeneficiaryID+"|"+expected.RuleID] != expected.AmountCents {
			return 0, errors.New("commission result no longer matches preview snapshot")
		}
	}
	if len(actual) != len(preview.result.EstimatedCommissions) {
		return 0, errors.New("commission record count no longer matches preview snapshot")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE xz_orders SET platform_income_cents=$2,reward_snapshot=$3::jsonb,updated_at=now() WHERE id=$1`, orderID, engineResult.PlatformIncomeCents, jsonProjection(map[string]any{"commissionRecords": preview.result.EstimatedCommissions})); err != nil {
		return 0, err
	}
	return int64(engineResult.CashCommissionCents), nil
}

func identityChangeAdminRoleAllowed(role string) bool {
	return strings.EqualFold(strings.TrimSpace(role), "ADMIN") || strings.EqualFold(strings.TrimSpace(role), "SUPER_ADMIN")
}
