package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"
)

type lockedVirtualOrder struct {
	ID                          string
	OrderNo                     string
	TenantID                    string
	UserID                      string
	PlanID                      string
	ProductCode                 string
	ProductName                 string
	ProductType                 string
	AmountCents                 int64
	Status                      string
	EntitlementStatus           string
	WeChatOpenIDHash            string
	StoredSnapshotVersion       int
	StoredPlanVersionID         string
	StoredPricePlanID           string
	StoredTransactionPriceCents int64
	StoredWeChatProductID       string
	StoredWeChatGoodsPriceCents int64
	StoredCurrency              string
	StoredPaymentChannel        string
	StoredPaymentEnvironment    string
	StoredRights                map[string]any
	StoredCommissionRuleVersion string
	StoredCommissionSnapshot    map[string]any
	Snapshot                    virtualOrderSnapshot
}

func (order lockedVirtualOrder) isV2() bool {
	return order.StoredSnapshotVersion == 2
}

func validateLockedVirtualOrderSnapshot(order lockedVirtualOrder) error {
	if !order.isV2() {
		if order.Snapshot.SnapshotVersion == 2 {
			return fmt.Errorf("%w: JSON snapshotVersion does not match the database order version", errVirtualPaymentMismatch)
		}
		return nil
	}
	if order.Snapshot.SnapshotVersion != 2 {
		return fmt.Errorf("%w: database V2 order is missing its V2 JSON snapshot", errVirtualPaymentMismatch)
	}
	if err := validateV2MemberAgentSnapshot(order.Snapshot, order.AmountCents); err != nil {
		return err
	}
	if order.PlanID != order.Snapshot.PlanID || order.StoredPlanVersionID != order.Snapshot.PlanVersionID ||
		order.StoredPricePlanID != order.Snapshot.PricePlanID || order.StoredTransactionPriceCents != order.Snapshot.TransactionPriceCents ||
		order.StoredWeChatProductID != order.Snapshot.WeChatProductID || order.StoredWeChatGoodsPriceCents != order.Snapshot.WeChatGoodsPriceCents ||
		!strings.EqualFold(order.StoredCurrency, order.Snapshot.Currency) ||
		!strings.EqualFold(order.StoredPaymentChannel, order.Snapshot.PaymentChannel) ||
		!strings.EqualFold(order.StoredPaymentEnvironment, order.Snapshot.PaymentEnvironment) ||
		!reflect.DeepEqual(order.StoredRights, order.Snapshot.Rights) ||
		order.StoredCommissionRuleVersion != order.Snapshot.CommissionRuleVersion ||
		!reflect.DeepEqual(order.StoredCommissionSnapshot, order.Snapshot.CommissionSnapshotV2) {
		return fmt.Errorf("%w: V2 JSON snapshot does not match normalized order columns", errVirtualPaymentMismatch)
	}
	return nil
}

func (s *virtualPaymentService) confirmPaidAndGrant(ctx context.Context, notification virtualPayNotification) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	duplicateComplete, err := upsertVirtualPaymentEventTx(ctx, tx, notification, "PROCESSING", "")
	if err != nil {
		return err
	}
	if duplicateComplete {
		return tx.Commit()
	}
	order, err := virtualOrderForUpdate(ctx, tx, notification.OrderNo)
	if err != nil {
		_ = tx.Rollback()
		_ = s.recordFailedNotification(ctx, notification, err)
		return err
	}
	if err := validateVirtualPaymentConfirmation(order, notification); err != nil {
		_ = tx.Rollback()
		_ = s.recordFailedNotification(ctx, notification, err)
		return err
	}
	paidAt := notification.PaidAt.UTC()
	if paidAt.IsZero() || paidAt.Year() < 2020 {
		paidAt = time.Now().UTC()
	}
	_, err = tx.ExecContext(ctx, `
		update xz_orders
		set status = $2, paid_at = $3, wechat_order_id = nullif($4, ''),
		    wechat_transaction_id = nullif($5, ''), entitlement_status = $6,
		    entitlement_error = '', entitlement_started_at = coalesce(entitlement_started_at, now()),
		    updated_at = now()
		where order_no = $1
	`, order.OrderNo, virtualOrderPaid, paidAt.Format(time.RFC3339Nano), notification.WeChatOrderID,
		notification.TransactionID, entitlementProcessing)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(notification.Payload)
	if len(payload) == 0 || string(payload) == "null" {
		payload = []byte("{}")
	}
	_, err = tx.ExecContext(ctx, `
		update xz_payment_records
		set prepay_status = 'PAID', wechat_order_id = nullif($2, ''),
		    wechat_transaction_id = nullif($3, ''), callback_payload = $4::jsonb,
		    paid_at = $5, failure_reason = '', updated_at = now()
		where order_no = $1
	`, order.OrderNo, notification.WeChatOrderID, notification.TransactionID, payload, paidAt)
	if err != nil {
		return err
	}
	order.Status = virtualOrderPaid
	order.EntitlementStatus = entitlementProcessing
	if err := s.grantOrderEntitlementsTx(ctx, tx, order, paidAt); err != nil {
		_ = tx.Rollback()
		_ = s.persistPaidEntitlementFailure(ctx, notification, paidAt, err)
		_ = s.recordFailedNotification(ctx, notification, err)
		return err
	}
	if err := markVirtualPaymentEventProcessedTx(ctx, tx, notification, "SUCCESS", ""); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *virtualPaymentService) persistPaidEntitlementFailure(ctx context.Context, notification virtualPayNotification, paidAt time.Time, cause error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	order, err := virtualOrderForUpdate(ctx, tx, notification.OrderNo)
	if err != nil {
		return err
	}
	if order.EntitlementStatus == entitlementSuccess {
		return tx.Commit()
	}
	if err := validateVirtualPaymentConfirmation(order, notification); err != nil {
		return err
	}
	message := truncateVirtualPaymentError(cause)
	_, err = tx.ExecContext(ctx, `
		update xz_orders
		set status = $2, paid_at = $3, wechat_order_id = nullif($4, ''),
		    wechat_transaction_id = nullif($5, ''), entitlement_status = $6,
		    entitlement_error = $7, entitlement_started_at = coalesce(entitlement_started_at, now()),
		    updated_at = now()
		where order_no = $1
	`, order.OrderNo, virtualOrderPaid, paidAt, notification.WeChatOrderID,
		notification.TransactionID, entitlementFailed, message)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(notification.Payload)
	if len(payload) == 0 || string(payload) == "null" {
		payload = []byte("{}")
	}
	_, err = tx.ExecContext(ctx, `
		update xz_payment_records
		set prepay_status = 'PAID', wechat_order_id = nullif($2, ''),
		    wechat_transaction_id = nullif($3, ''), callback_payload = $4::jsonb,
		    paid_at = $5, failure_reason = $6, updated_at = now()
		where order_no = $1
	`, order.OrderNo, notification.WeChatOrderID, notification.TransactionID, payload, paidAt, message)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func validateVirtualPaymentConfirmation(order lockedVirtualOrder, notification virtualPayNotification) error {
	if order.OrderNo == "" || !strings.EqualFold(order.ProductCode, order.Snapshot.ProductCode) || order.AmountCents != order.Snapshot.AmountCents {
		return fmt.Errorf("%w: immutable product snapshot mismatch", errVirtualPaymentMismatch)
	}
	if notification.AmountCents <= 0 || notification.AmountCents != order.AmountCents {
		return fmt.Errorf("%w: amount mismatch", errVirtualPaymentMismatch)
	}
	if notification.ProductID != "" && !strings.EqualFold(notification.ProductID, order.Snapshot.WeChatProductID) {
		return fmt.Errorf("%w: wechat product mismatch", errVirtualPaymentMismatch)
	}
	expectedQuantity := order.Snapshot.BuyQuantity
	if expectedQuantity <= 0 {
		expectedQuantity = 1
	}
	if notification.Quantity != 0 && notification.Quantity != expectedQuantity {
		return fmt.Errorf("%w: purchase quantity mismatch", errVirtualPaymentMismatch)
	}
	if notification.HasEnv && notification.Env != order.Snapshot.Env {
		return fmt.Errorf("%w: payment environment mismatch", errVirtualPaymentMismatch)
	}
	if notification.OpenID != "" && order.WeChatOpenIDHash != "" && hashSensitiveIdentifier(notification.OpenID) != order.WeChatOpenIDHash {
		return fmt.Errorf("%w: payer identity mismatch", errVirtualPaymentMismatch)
	}
	switch order.Status {
	case virtualOrderPending, virtualOrderPaid:
		return nil
	default:
		return fmt.Errorf("%w: order status %s cannot be paid", errVirtualPaymentMismatch, order.Status)
	}
}

func virtualOrderForUpdate(ctx context.Context, tx *sql.Tx, orderNo string) (lockedVirtualOrder, error) {
	var order lockedVirtualOrder
	var snapshot, storedRights, storedCommission []byte
	err := tx.QueryRowContext(ctx, `
		select id, order_no, tenant_id, user_id, plan_id, product_code, product_name, product_type,
		       amount_cents, status, entitlement_status, coalesce(wechat_openid_hash, ''), price_snapshot,
		       coalesce(snapshot_version,0),coalesce(plan_version_id,''),coalesce(price_plan_id,''),
		       coalesce(transaction_price_cents,0),coalesce(wechat_product_id_snapshot,''),
		       coalesce(wechat_goods_price_cents,0),coalesce(currency,'CNY'),coalesce(payment_channel,''),
		       coalesce(payment_environment,''),coalesce(rights_snapshot,'{}'::jsonb),
		       coalesce(commission_rule_version_snapshot,''),coalesce(commission_snapshot_v2,'{}'::jsonb)
		from xz_orders
		where order_no = $1 and payment_channel = $2
		for update
	`, strings.TrimSpace(orderNo), virtualPaymentChannel).Scan(
		&order.ID, &order.OrderNo, &order.TenantID, &order.UserID, &order.PlanID, &order.ProductCode,
		&order.ProductName, &order.ProductType, &order.AmountCents, &order.Status,
		&order.EntitlementStatus, &order.WeChatOpenIDHash, &snapshot, &order.StoredSnapshotVersion,
		&order.StoredPlanVersionID, &order.StoredPricePlanID, &order.StoredTransactionPriceCents,
		&order.StoredWeChatProductID, &order.StoredWeChatGoodsPriceCents, &order.StoredCurrency,
		&order.StoredPaymentChannel, &order.StoredPaymentEnvironment, &storedRights,
		&order.StoredCommissionRuleVersion, &storedCommission,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return lockedVirtualOrder{}, errVirtualOrderNotFound
	}
	if err != nil {
		return lockedVirtualOrder{}, err
	}
	if err := json.Unmarshal(snapshot, &order.Snapshot); err != nil {
		return lockedVirtualOrder{}, err
	}
	if err := json.Unmarshal(storedRights, &order.StoredRights); err != nil {
		return lockedVirtualOrder{}, err
	}
	if err := json.Unmarshal(storedCommission, &order.StoredCommissionSnapshot); err != nil {
		return lockedVirtualOrder{}, err
	}
	if err := validateLockedVirtualOrderSnapshot(order); err != nil {
		return lockedVirtualOrder{}, err
	}
	return order, nil
}

func (s *virtualPaymentService) GrantOrderEntitlements(ctx context.Context, orderNo string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	order, err := virtualOrderForUpdate(ctx, tx, orderNo)
	if err != nil {
		return err
	}
	if order.Status != virtualOrderPaid {
		return fmt.Errorf("order %s is not paid", order.OrderNo)
	}
	paidAt := time.Now().UTC()
	var paidAtText sql.NullString
	if err := tx.QueryRowContext(ctx, `select paid_at from xz_orders where order_no = $1`, orderNo).Scan(&paidAtText); err == nil && paidAtText.Valid {
		if parsed, parseErr := time.Parse(time.RFC3339Nano, paidAtText.String); parseErr == nil {
			paidAt = parsed.UTC()
		}
	}
	if err := s.grantOrderEntitlementsTx(ctx, tx, order, paidAt); err != nil {
		_ = tx.Rollback()
		_ = s.markEntitlementFailed(ctx, order.OrderNo, err)
		return err
	}
	return tx.Commit()
}

func (s *virtualPaymentService) grantOrderEntitlementsTx(ctx context.Context, tx *sql.Tx, order lockedVirtualOrder, paidAt time.Time) error {
	if order.EntitlementStatus == entitlementSuccess {
		return markCommercialOrderPaidTx(ctx, tx, order, paidAt)
	}
	if order.Snapshot.AmountCents != order.AmountCents || !strings.EqualFold(order.Snapshot.ProductCode, order.ProductCode) {
		return fmt.Errorf("%w: immutable entitlement snapshot mismatch", errVirtualPaymentMismatch)
	}
	switch strings.ToUpper(strings.TrimSpace(order.Snapshot.ProductType)) {
	case "TOKEN_ONLY", "TOKEN_UPGRADE", "MEMBERSHIP", "IDENTITY":
		if order.Snapshot.CreditUnits <= 0 {
			return errors.New("token entitlement snapshot is invalid")
		}
		if err := s.grantCommerceVirtualProductTx(ctx, tx, order, paidAt); err != nil {
			return err
		}
	case "IMAGE_QUOTA_PACK":
		if order.Snapshot.ImageQuota <= 0 {
			return errors.New("image quota snapshot is invalid")
		}
		if err := grantImageQuotaTx(ctx, tx, order, order.Snapshot.ImageQuota); err != nil {
			return err
		}
	case "MEMBER_PACKAGE":
		if order.Snapshot.MemberDays <= 0 || order.Snapshot.MemberLevel == "" || order.Snapshot.CreditUnits <= 0 {
			return errors.New("membership entitlement snapshot is invalid")
		}
		if err := grantMembershipTx(ctx, tx, order, paidAt); err != nil {
			return err
		}
		if err := grantCreationCreditsTx(ctx, tx, order, order.Snapshot.CreditUnits, paidAt); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported virtual product type: %s", order.Snapshot.ProductType)
	}
	if err := applyVirtualCouponBonusTx(ctx, tx, order, paidAt); err != nil {
		return err
	}
	if err := markCommercialOrderPaidTx(ctx, tx, order, paidAt); err != nil {
		return err
	}
	grantedAt := time.Now().UTC()
	_, err := tx.ExecContext(ctx, `
		update xz_orders
		set fulfillment_status = 'FULFILLED', fulfilled_at = $2, entitlement_status = $3,
		    entitlement_error = '', entitlement_granted_at = $4, updated_at = now()
		where order_no = $1
	`, order.OrderNo, grantedAt.Format(time.RFC3339Nano), entitlementSuccess, grantedAt)
	return err
}

func (s *virtualPaymentService) grantCommerceVirtualProductTx(ctx context.Context, tx *sql.Tx, order lockedVirtualOrder, paidAt time.Time) error {
	if order.AmountCents <= 0 || order.AmountCents > math.MaxInt || order.Snapshot.CreditUnits <= 0 || order.Snapshot.CreditUnits > math.MaxInt {
		return errors.New("virtual commerce amount or token grant is invalid")
	}
	isV2 := order.isV2()
	var plan adminPlan
	if isV2 {
		if !s.cfg.SnapshotV2FulfillmentEnabled {
			return errors.New("V2 member/agent fulfillment is disabled")
		}
		var err error
		plan, err = planForV2Snapshot(order.Snapshot)
		if err != nil {
			return err
		}
	} else {
		var ok bool
		plan, ok = planCatalogByID(order.PlanID)
		if !ok {
			return fmt.Errorf("virtual commerce plan not found: %s", order.PlanID)
		}
	}
	planType := planBusinessType(plan)
	productType := strings.ToUpper(strings.TrimSpace(order.Snapshot.ProductType))
	if normalizePlanTypeString(order.Snapshot.PlanType) != planType {
		return fmt.Errorf("%w: virtual commerce plan type changed after order creation", errVirtualPaymentMismatch)
	}
	if productType == "TOKEN_ONLY" && planType != planTypeTokenRecharge {
		return fmt.Errorf("%w: TOKEN_ONLY must use a recharge plan", errVirtualPaymentMismatch)
	}
	if productType == "TOKEN_UPGRADE" && planType != planTypeMemberPackage && planType != planTypeAgentJoinPackage {
		return fmt.Errorf("%w: TOKEN_UPGRADE plan type is invalid", errVirtualPaymentMismatch)
	}
	if productType == "MEMBERSHIP" && planType != planTypeMemberPackage {
		return fmt.Errorf("%w: MEMBERSHIP must use a member package", errVirtualPaymentMismatch)
	}
	if productType == "IDENTITY" && planType != planTypeAgentJoinPackage {
		return fmt.Errorf("%w: IDENTITY must use an agent join package", errVirtualPaymentMismatch)
	}
	quantity := order.Snapshot.BuyQuantity
	if quantity <= 0 {
		quantity = 1
	}
	customQuantity := boolValue(plan.Entitlements["customQuantity"])
	if quantity != 1 && !customQuantity {
		return fmt.Errorf("%w: fixed virtual commerce plan has invalid quantity", errVirtualPaymentMismatch)
	}
	unitPrice := int64(planPrice(plan))
	unitTokens := int64(planTokenGrantAmount(plan))
	if unitPrice <= 0 || unitTokens <= 0 {
		return fmt.Errorf("%w: virtual commerce unit configuration is invalid", errVirtualPaymentMismatch)
	}
	if quantity > math.MaxInt64/unitPrice || quantity > math.MaxInt64/unitTokens {
		return fmt.Errorf("%w: virtual commerce quantity overflow", errVirtualPaymentMismatch)
	}
	if order.Snapshot.UnitPriceCents > 0 && order.Snapshot.UnitPriceCents != unitPrice {
		return fmt.Errorf("%w: virtual commerce unit price changed after order creation", errVirtualPaymentMismatch)
	}
	if order.Snapshot.UnitCreditUnits > 0 && order.Snapshot.UnitCreditUnits != unitTokens {
		return fmt.Errorf("%w: virtual commerce unit grant changed after order creation", errVirtualPaymentMismatch)
	}
	if unitPrice*quantity != order.AmountCents || unitTokens*quantity != order.Snapshot.CreditUnits {
		return fmt.Errorf("%w: virtual commerce plan changed after order creation", errVirtualPaymentMismatch)
	}
	if int64(plan.DurationDays) != order.Snapshot.MemberDays {
		return fmt.Errorf("%w: virtual commerce validity changed after order creation", errVirtualPaymentMismatch)
	}
	if planType == planTypeMemberPackage && !strings.EqualFold(planMemberLevel(plan), order.Snapshot.MemberLevel) {
		return fmt.Errorf("%w: member level changed after order creation", errVirtualPaymentMismatch)
	}
	if planType == planTypeAgentJoinPackage && !strings.EqualFold(plan.AgentLevel, order.Snapshot.AgentLevel) {
		return fmt.Errorf("%w: agent level changed after order creation", errVirtualPaymentMismatch)
	}

	paidAt = paidAt.UTC()
	if paidAt.IsZero() {
		paidAt = time.Now().UTC()
	}
	priceSnapshot := map[string]any{
		"productCode":                order.Snapshot.ProductCode,
		"productName":                order.Snapshot.ProductName,
		"productType":                productType,
		"planType":                   planType,
		"amountCents":                order.Snapshot.AmountCents,
		"memberLevel":                order.Snapshot.MemberLevel,
		"agentLevel":                 order.Snapshot.AgentLevel,
		"durationDays":               order.Snapshot.MemberDays,
		"creditUnits":                order.Snapshot.CreditUnits,
		"buyQuantity":                quantity,
		"unitPriceCents":             unitPrice,
		"unitCreditUnits":            unitTokens,
		"tokenGrantAmount":           order.Snapshot.CreditUnits,
		"tokenAmount":                order.Snapshot.CreditUnits,
		"offerId":                    order.Snapshot.OfferID,
		"wechatProductId":            order.Snapshot.WeChatProductID,
		"mode":                       order.Snapshot.Mode,
		"env":                        order.Snapshot.Env,
		"nonWithdrawable":            true,
		"nonTransferable":            true,
		"commissionTemplateCode":     order.Snapshot.CommissionTemplateCode,
		"commissionSnapshotCaptured": order.Snapshot.CommissionSnapshotCaptured,
		"directAgentId":              order.Snapshot.DirectAgentID,
		"parentAgentId":              order.Snapshot.ParentAgentID,
		"operationCenterId":          order.Snapshot.OperationCenterID,
		"commissionRules":            order.Snapshot.CommissionRules,
	}
	if isV2 {
		priceSnapshot["snapshotVersion"] = 2
	}
	if planType == planTypeTokenRecharge {
		priceSnapshot["orderType"] = "COMPUTE_RECHARGE"
		priceSnapshot["rechargePoints"] = order.Snapshot.CreditUnits
	}
	commerceOrder := adminOrder{
		ID: order.ID, OrderNo: order.OrderNo, TenantID: order.TenantID, UserID: order.UserID,
		BuyerUserID: order.UserID, PlanID: order.PlanID, Amount: int(order.AmountCents), AmountCents: int(order.AmountCents),
		TokenGrantAmount: int(order.Snapshot.CreditUnits), TokenAmount: int(order.Snapshot.CreditUnits),
		Status: virtualOrderPaid, PaidAt: paidAt.Format(time.RFC3339Nano), CreatedAt: paidAt.Format(time.RFC3339Nano),
		PriceSnapshot: priceSnapshot,
		DirectAgentID: order.Snapshot.DirectAgentID, ParentAgentID: order.Snapshot.ParentAgentID,
		OperationCenterID:          order.Snapshot.OperationCenterID,
		CommissionTemplateCode:     order.Snapshot.CommissionTemplateCode,
		CommissionSnapshotCaptured: order.Snapshot.CommissionSnapshotCaptured,
		CommissionRuleSnapshot:     append([]commissionRuleSnapshot(nil), order.Snapshot.CommissionRules...),
	}
	var fulfillmentErr error
	if isV2 {
		fulfillmentErr = applyCommerceOrderFulfillmentWithPlanForTx(ctx, tx, s.db, &commerceOrder, plan)
	} else {
		fulfillmentErr = applyCommerceOrderFulfillmentForTx(ctx, tx, s.db, &commerceOrder)
	}
	if fulfillmentErr != nil {
		return fulfillmentErr
	}
	if commerceOrder.FulfillmentStatus != "FULFILLED" {
		return errors.New("virtual commerce entitlement was not fulfilled")
	}
	if planType == planTypeMemberPackage {
		if err := recordVirtualMembershipEntitlementTx(ctx, tx, order, paidAt); err != nil {
			return err
		}
	}
	priceJSON, err := json.Marshal(commerceOrder.PriceSnapshot)
	if err != nil {
		return err
	}
	rewardJSON, err := json.Marshal(commerceOrder.RewardSnapshot)
	if err != nil {
		return err
	}
	if isV2 {
		_, err = tx.ExecContext(ctx, `
			update xz_orders
			set buyer_user_id = $2, order_type = $3, business_order_type = $4,
			    direct_agent_id = nullif($5, ''), parent_agent_id = nullif($6, ''),
			    operation_center_id = nullif($7, ''), token_amount = $8, token_grant_amount = $8,
			    token_grant_value_cents = $9, platform_income_cents = $10,
			    reward_snapshot = $11::jsonb,
			    fulfillment_status = $12, fulfilled_at = $13, updated_at = now()
			where order_no = $1
		`, order.OrderNo, order.UserID, commerceOrder.OrderType, businessOrderTypeFromOrder(commerceOrder),
			commerceOrder.DirectAgentID, commerceOrder.ParentAgentID, commerceOrder.OperationCenterID,
			commerceOrder.TokenGrantAmount, intValue(commerceOrder.PriceSnapshot["tokenGrantValueCents"]),
			commerceOrder.PlatformIncomeCents, rewardJSON, commerceOrder.FulfillmentStatus, commerceOrder.FulfilledAt)
		return err
	}
	_, err = tx.ExecContext(ctx, `
		update xz_orders
		set buyer_user_id = $2, order_type = $3, business_order_type = $4,
		    direct_agent_id = nullif($5, ''), parent_agent_id = nullif($6, ''),
		    operation_center_id = nullif($7, ''), token_amount = $8, token_grant_amount = $8,
		    token_grant_value_cents = $9, platform_income_cents = $10,
		    reward_snapshot = $11::jsonb, price_snapshot = $12::jsonb,
		    fulfillment_status = $13, fulfilled_at = $14, updated_at = now()
		where order_no = $1
	`, order.OrderNo, order.UserID, commerceOrder.OrderType, businessOrderTypeFromOrder(commerceOrder),
		commerceOrder.DirectAgentID, commerceOrder.ParentAgentID, commerceOrder.OperationCenterID,
		commerceOrder.TokenGrantAmount, intValue(commerceOrder.PriceSnapshot["tokenGrantValueCents"]),
		commerceOrder.PlatformIncomeCents, rewardJSON, priceJSON, commerceOrder.FulfillmentStatus, commerceOrder.FulfilledAt)
	return err
}

func recordVirtualMembershipEntitlementTx(ctx context.Context, tx *sql.Tx, order lockedVirtualOrder, paidAt time.Time) error {
	idempotencyKey := "virtual-payment:" + order.OrderNo + ":membership"
	var exists bool
	if err := tx.QueryRowContext(ctx, `select exists(select 1 from xz_membership_entitlement_records where idempotency_key = $1)`, idempotencyKey).Scan(&exists); err != nil || exists {
		return err
	}
	var expiresText string
	if err := tx.QueryRowContext(ctx, `select coalesce(subscription_expires_at, '') from xz_users where id = $1`, order.UserID).Scan(&expiresText); err != nil {
		return err
	}
	expiresAt := paidAt.UTC().AddDate(0, 0, int(order.Snapshot.MemberDays))
	if parsed, err := time.Parse(time.RFC3339Nano, expiresText); err == nil {
		expiresAt = parsed.UTC()
	}
	effectiveAt := expiresAt.AddDate(0, 0, -int(order.Snapshot.MemberDays))
	metadata, _ := json.Marshal(map[string]any{"productCode": order.ProductCode, "days": order.Snapshot.MemberDays, "productType": order.Snapshot.ProductType})
	_, err := tx.ExecContext(ctx, `
		insert into xz_membership_entitlement_records(
			id, tenant_id, user_id, member_level, effective_at, expires_at,
			source_order_no, idempotency_key, metadata
		) values ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)
		on conflict (idempotency_key) do nothing
	`, virtualPaymentResourceID("membership", idempotencyKey), order.TenantID, order.UserID,
		strings.ToUpper(order.Snapshot.MemberLevel), effectiveAt, expiresAt, order.OrderNo, idempotencyKey, metadata)
	return err
}

func grantCreationCreditsTx(ctx context.Context, tx *sql.Tx, order lockedVirtualOrder, amount int64, now time.Time) error {
	if amount <= 0 || amount > math.MaxInt {
		return errors.New("credit entitlement amount is invalid")
	}
	idempotencyKey := "virtual-payment:" + order.OrderNo + ":credits"
	var exists bool
	if err := tx.QueryRowContext(ctx, `select exists(select 1 from xz_token_records where idempotency_key = $1)`, idempotencyKey).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	account, err := pointAccountForUpdate(ctx, tx, order.UserID)
	if err != nil {
		return err
	}
	if account.Available < 0 || amount > int64(math.MaxInt-account.Available) {
		return errors.New("credit entitlement would overflow wallet balance")
	}
	before := account.Available
	account.Available += int(amount)
	account.TotalGranted += int(amount)
	if err := insertPointAccount(ctx, tx, account); err != nil {
		return err
	}
	if err := insertAccountBalanceLedgerV1(ctx, tx, account, "GRANT", int(amount), before, account.Available, "WECHAT_VIRTUAL_ORDER", order.OrderNo, "WeChat virtual payment credit grant"); err != nil {
		return err
	}
	recordID := virtualPaymentResourceID("token", idempotencyKey)
	record := adminTokenRecord{
		ID: recordID, UserID: order.UserID, OrderID: order.OrderNo, ChangeType: "WECHAT_VIRTUAL_MEMBER_BONUS",
		Amount: int(amount), BalanceAfter: account.Available, Remark: "wechat_virtual_payment_entitlement",
		CreatedAt: now.Format(time.RFC3339Nano),
	}
	_, err = tx.ExecContext(ctx, `
		insert into xz_token_records(
			id, user_id, order_id, change_type, amount, balance_before, balance_after, remark,
			created_at, tenant_id, idempotency_key, source_order_no, raw
		) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$3,$12::jsonb)
		on conflict do nothing
	`, record.ID, record.UserID, order.OrderNo, record.ChangeType, amount, before, record.BalanceAfter,
		record.Remark, record.CreatedAt, order.TenantID, idempotencyKey, jsonProjection(record))
	if err != nil {
		return err
	}
	exists, err = billingEventExistsTx(ctx, tx, order.OrderNo, "wechat_virtual.credit_grant")
	if err != nil || exists {
		return err
	}
	return insertBillingEvent(ctx, tx, adminBillingEvent{
		ID: virtualPaymentResourceID("evt", idempotencyKey), TransactionID: virtualPaymentResourceID("txn", idempotencyKey),
		UserID: order.UserID, TaskID: order.OrderNo, MetricCode: "wechat_virtual.credit_grant",
		Quantity: int(amount), AmountCents: 0, PointCost: -int(amount), BalanceBefore: before,
		BalanceAfter: account.Available, Model: "wechat_virtual_payment", Status: "SUCCEEDED",
		OccurredAt: now.Format(time.RFC3339Nano), Metadata: map[string]any{
			"source": "wechat_virtual_payment", "orderNo": order.OrderNo, "productCode": order.ProductCode,
			"nonWithdrawable": true, "nonTransferable": true,
		},
	})
}

func grantImageQuotaTx(ctx context.Context, tx *sql.Tx, order lockedVirtualOrder, amount int64) error {
	idempotencyKey := "virtual-payment:" + order.OrderNo + ":image-quota"
	var exists bool
	if err := tx.QueryRowContext(ctx, `select exists(select 1 from xz_image_quota_ledger where idempotency_key = $1)`, idempotencyKey).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	accountID := virtualPaymentResourceID("image_account", order.TenantID+":"+order.UserID)
	_, err := tx.ExecContext(ctx, `
		insert into xz_image_quota_accounts(id, tenant_id, user_id)
		values ($1,$2,$3)
		on conflict (tenant_id, user_id) do nothing
	`, accountID, order.TenantID, order.UserID)
	if err != nil {
		return err
	}
	var storedAccountID string
	var before int64
	err = tx.QueryRowContext(ctx, `
		select id, remaining_images from xz_image_quota_accounts
		where tenant_id = $1 and user_id = $2 for update
	`, order.TenantID, order.UserID).Scan(&storedAccountID, &before)
	if err != nil {
		return err
	}
	after := before + amount
	_, err = tx.ExecContext(ctx, `
		update xz_image_quota_accounts
		set remaining_images = $3, total_granted = total_granted + $4,
		    version = version + 1, updated_at = now()
		where tenant_id = $1 and user_id = $2
	`, order.TenantID, order.UserID, after, amount)
	if err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]any{"productCode": order.ProductCode, "validity": "PERMANENT", "nonWithdrawable": true, "nonTransferable": true})
	_, err = tx.ExecContext(ctx, `
		insert into xz_image_quota_ledger(
			id, tenant_id, user_id, account_id, image_delta, balance_before, balance_after,
			source_order_no, business_type, idempotency_key, metadata
		) values ($1,$2,$3,$4,$5,$6,$7,$8,'WECHAT_VIRTUAL_PURCHASE',$9,$10::jsonb)
		on conflict (idempotency_key) do nothing
	`, virtualPaymentResourceID("image_ledger", idempotencyKey), order.TenantID, order.UserID, storedAccountID,
		amount, before, after, order.OrderNo, idempotencyKey, metadata)
	return err
}

func grantMembershipTx(ctx context.Context, tx *sql.Tx, order lockedVirtualOrder, paidAt time.Time) error {
	idempotencyKey := "virtual-payment:" + order.OrderNo + ":membership"
	var exists bool
	if err := tx.QueryRowContext(ctx, `select exists(select 1 from xz_membership_entitlement_records where idempotency_key = $1)`, idempotencyKey).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	user, err := userByIDForUpdateTx(ctx, tx, order.UserID)
	if err != nil {
		return err
	}
	effectiveAt, expiresAt := membershipExtensionWindow(user.SubscriptionExpiresAt, paidAt, order.Snapshot.MemberDays)
	user.PlanID = order.PlanID
	user.MemberLevel = strings.ToUpper(order.Snapshot.MemberLevel)
	user.SubscriptionExpiresAt = expiresAt.Format(time.RFC3339Nano)
	user.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := insertUser(ctx, tx, user); err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]any{"productCode": order.ProductCode, "days": order.Snapshot.MemberDays})
	_, err = tx.ExecContext(ctx, `
		insert into xz_membership_entitlement_records(
			id, tenant_id, user_id, member_level, effective_at, expires_at,
			source_order_no, idempotency_key, metadata
		) values ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)
		on conflict (idempotency_key) do nothing
	`, virtualPaymentResourceID("membership", idempotencyKey), order.TenantID, order.UserID,
		strings.ToUpper(order.Snapshot.MemberLevel), effectiveAt, expiresAt, order.OrderNo, idempotencyKey, metadata)
	return err
}

func membershipExtensionWindow(currentExpiry string, paidAt time.Time, days int64) (time.Time, time.Time) {
	paidAt = paidAt.UTC()
	if paidAt.IsZero() {
		paidAt = time.Now().UTC()
	}
	base := paidAt
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(currentExpiry)); err == nil && parsed.After(base) {
			base = parsed.UTC()
			break
		}
	}
	return base, base.AddDate(0, 0, int(days))
}

func (s *virtualPaymentService) markEntitlementFailed(ctx context.Context, orderNo string, cause error) error {
	message := truncateVirtualPaymentError(cause)
	_, err := s.db.ExecContext(ctx, `
		update xz_orders set entitlement_status = $2, entitlement_error = $3, updated_at = now()
		where order_no = $1 and entitlement_status <> $4
	`, orderNo, entitlementFailed, message, entitlementSuccess)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		update xz_payment_records set failure_reason = $2, updated_at = now() where order_no = $1
	`, orderNo, message)
	return err
}

func truncateVirtualPaymentError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}

func upsertVirtualPaymentEventTx(ctx context.Context, tx *sql.Tx, notification virtualPayNotification, status string, errorMessage string) (bool, error) {
	idempotencyKey := "WECHAT_VIRTUAL:" + notification.ID
	var existingStatus string
	err := tx.QueryRowContext(ctx, `
		select processing_status from xz_payment_events
		where idempotency_key = $1
		   or ($2 <> '' and provider = $3 and transaction_id = $2)
		for update
	`, idempotencyKey, notification.TransactionID, virtualPaymentChannel).Scan(&existingStatus)
	if err == nil {
		_, updateErr := tx.ExecContext(ctx, `
			update xz_payment_events
			set process_attempts = process_attempts + 1,
			    processing_status = case when processing_status = 'SUCCESS' then processing_status else $2 end,
			    error_message = case when processing_status = 'SUCCESS' then error_message else $3 end
			where idempotency_key = $1 or ($4 <> '' and provider = $5 and transaction_id = $4)
		`, idempotencyKey, status, errorMessage, notification.TransactionID, virtualPaymentChannel)
		return strings.EqualFold(existingStatus, "SUCCESS"), updateErr
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	rawPayload := notification.Payload
	if rawPayload == nil {
		rawPayload = map[string]any{"event": notification.Event, "orderNo": notification.OrderNo}
	}
	rawJSON, _ := json.Marshal(rawPayload)
	eventID := virtualPaymentResourceID("payment_event", idempotencyKey)
	result, err := tx.ExecContext(ctx, `
		insert into xz_payment_events(
			id, provider, event_id, event_type, order_id, transaction_id, amount_cents,
			raw, raw_body, verified, idempotency_key, status, processing_status,
			process_attempts, error_message
		) values ($1,$2,$3,$4,$5,nullif($6,''),$7,$8::jsonb,$9,true,$10,'RECEIVED',$11,1,$12)
		on conflict do nothing
	`, eventID, virtualPaymentChannel, notification.ID, notification.Event, notification.OrderNo,
		notification.TransactionID, notification.AmountCents, rawJSON, string(notification.Raw),
		idempotencyKey, status, errorMessage)
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil || inserted > 0 {
		return false, err
	}
	err = tx.QueryRowContext(ctx, `
		select processing_status from xz_payment_events
		where idempotency_key = $1
		   or ($2 <> '' and provider = $3 and transaction_id = $2)
		for update
	`, idempotencyKey, notification.TransactionID, virtualPaymentChannel).Scan(&existingStatus)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(existingStatus, "SUCCESS"), nil
}

func markVirtualPaymentEventProcessedTx(ctx context.Context, tx *sql.Tx, notification virtualPayNotification, status string, errorMessage string) error {
	idempotencyKey := "WECHAT_VIRTUAL:" + notification.ID
	_, err := tx.ExecContext(ctx, `
		update xz_payment_events
		set processing_status = $2, status = $2, error_message = $3,
		    processed_at = case when $2 = 'SUCCESS' then now() else processed_at end
		where idempotency_key = $1
		   or ($4 <> '' and provider = $5 and transaction_id = $4)
	`, idempotencyKey, status, errorMessage, notification.TransactionID, virtualPaymentChannel)
	return err
}

func (s *virtualPaymentService) recordFailedNotification(ctx context.Context, notification virtualPayNotification, cause error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = upsertVirtualPaymentEventTx(ctx, tx, notification, "FAILED", truncateVirtualPaymentError(cause))
	if err != nil {
		return err
	}
	if err := markVirtualPaymentEventProcessedTx(ctx, tx, notification, "FAILED", truncateVirtualPaymentError(cause)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *virtualPaymentService) recordIgnoredNotification(ctx context.Context, notification virtualPayNotification) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = upsertVirtualPaymentEventTx(ctx, tx, notification, "IGNORED", "unsupported event")
	if err != nil {
		return err
	}
	if err := markVirtualPaymentEventProcessedTx(ctx, tx, notification, "IGNORED", "unsupported event"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *virtualPaymentService) recordRefundNotification(ctx context.Context, notification virtualPayNotification) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	duplicate, err := upsertVirtualPaymentEventTx(ctx, tx, notification, "PROCESSING", "")
	if err != nil {
		return err
	}
	if duplicate {
		return tx.Commit()
	}
	order, err := virtualOrderForUpdate(ctx, tx, notification.OrderNo)
	if err != nil {
		return err
	}
	if notification.OpenID != "" && order.WeChatOpenIDHash != "" && hashSensitiveIdentifier(notification.OpenID) != order.WeChatOpenIDHash {
		return fmt.Errorf("%w: refund payer identity mismatch", errVirtualPaymentMismatch)
	}
	idempotencyKey := "virtual-payment:" + order.OrderNo + ":refund:" + notification.ID
	refundID := virtualPaymentResourceID("refund", idempotencyKey)
	raw, _ := json.Marshal(notification.Payload)
	status := virtualOrderRefunded
	if notification.ResultCode != 0 {
		status = "REFUND_FAILED"
	}
	_, err = tx.ExecContext(ctx, `
		insert into xz_refund_records(
			id, order_id, order_no, tenant_id, user_id, provider, provider_refund_id,
			amount_cents, status, idempotency_key, raw
		) values ($1,$2,$2,$3,$4,$5,nullif($6,''),nullif($7,0),$8,$9,$10::jsonb)
		on conflict (idempotency_key) do nothing
	`, refundID, order.OrderNo, order.TenantID, order.UserID,
		virtualPaymentChannel, notification.TransactionID, notification.AmountCents, status,
		idempotencyKey, raw)
	if err != nil {
		return err
	}
	if notification.ResultCode == 0 {
		_, err = tx.ExecContext(ctx, `
			update xz_orders set status = $2, updated_at = now() where order_no = $1
		`, order.OrderNo, status)
		if err != nil {
			return err
		}
		if err := reverseCommissionRecordsForOrderTx(ctx, tx, order.ID, order.OrderNo, time.Now().UTC()); err != nil {
			return err
		}
	}
	if err := markCommercialOrderRefundedTx(ctx, tx, order, refundID, notification.AmountCents, notification.ResultCode == 0); err != nil {
		return err
	}
	processMessage := "commission cancelled or reversed; entitlement reversal requires review"
	if notification.ResultCode != 0 {
		processMessage = firstNonEmptyString(notification.ResultMessage, "wechat refund failed")
	}
	if err := markVirtualPaymentEventProcessedTx(ctx, tx, notification, "SUCCESS", processMessage); err != nil {
		return err
	}
	return tx.Commit()
}
