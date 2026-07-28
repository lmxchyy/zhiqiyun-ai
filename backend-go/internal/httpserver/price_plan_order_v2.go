package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

var errPricePlanV132SnapshotIncompatible = errors.New("V132 settlement is not enabled for V2 member or agent price-plan orders")

func validateV132PricePlanSettlementDecision(decision orderSettlementDecision) error {
	if decision.SettlementEngine != settlementEngineV132 {
		return nil
	}
	return fmt.Errorf("%w: use LEGACY settlement until V132 token-value conservation is snapshot-safe", errPricePlanV132SnapshotIncompatible)
}

func (s *virtualPaymentService) createOrderFromPriceQuote(ctx context.Context, user adminUser, requestedTenant, quoteToken string, paymentSession *wechatMiniProgramSession) (createVirtualOrderResponse, error) {
	if !s.cfg.PricePlanCreationEnabled {
		return createVirtualOrderResponse{}, errPricePlanCreationDisabled
	}
	if !s.cfg.SnapshotV2FulfillmentEnabled {
		return createVirtualOrderResponse{}, errors.New("V2 快照履约未启用，禁止创建新订单")
	}
	if strings.TrimSpace(quoteToken) == "" {
		return createVirtualOrderResponse{}, errPriceQuoteRequired
	}
	if !s.cfg.ready() {
		return createVirtualOrderResponse{}, errVirtualPaymentUnavailable
	}
	wechatSession := wechatMiniProgramSession{}
	if paymentSession != nil {
		wechatSession = *paymentSession
	} else {
		var ok bool
		var err error
		wechatSession, ok, err = s.sessions.WeChatSession(ctx, user.ID)
		if err != nil {
			return createVirtualOrderResponse{}, err
		}
		if !ok {
			return createVirtualOrderResponse{}, errVirtualPaymentRelogin
		}
	}
	if strings.TrimSpace(wechatSession.OpenID) == "" || strings.TrimSpace(wechatSession.SessionKey) == "" {
		return createVirtualOrderResponse{}, errVirtualPaymentRelogin
	}
	tenantID, err := s.resolveTenant(ctx, user, requestedTenant)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	defer func() { _ = tx.Rollback() }()
	quote, status, err := loadPriceQuoteRowForUpdate(ctx, tx, quoteToken)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	if quote.UserID != user.ID || quote.TenantID != tenantID {
		return createVirtualOrderResponse{}, errPriceQuoteForbidden
	}
	now := time.Now().UTC()
	if !now.Before(quote.ExpiresAt) {
		return createVirtualOrderResponse{}, errPriceQuoteExpired
	}
	if status != quoteStatusAvailable {
		return createVirtualOrderResponse{}, errPriceQuoteConsumed
	}
	if err := lockPriceQuoteConfiguration(ctx, tx, &quote, now, false); err != nil {
		return createVirtualOrderResponse{}, err
	}
	if err := validatePricePlanRuntimeFulfillment(quote); err != nil {
		return createVirtualOrderResponse{}, err
	}
	if err := validatePriceQuoteConfiguration(quote); err != nil {
		return createVirtualOrderResponse{}, err
	}
	if err := validatePricePlanPaymentChain(quote.paymentChain()); err != nil {
		return createVirtualOrderResponse{}, err
	}
	if err := validateV2QuoteEnvironment(quote, s.cfg.Env); err != nil {
		return createVirtualOrderResponse{}, err
	}
	if err := revalidatePinnedTestWhitelistForOrder(ctx, tx, quote); err != nil {
		return createVirtualOrderResponse{}, err
	}
	snapshot, err := snapshotForResolvedPriceQuote(quote)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	plan, err := planForV2Snapshot(snapshot)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	orderNo := newVirtualBusinessNo("ZQY")
	paymentNo := newVirtualBusinessNo("PAY")
	commerceOrder := adminOrder{
		ID: orderNo, OrderNo: orderNo, TenantID: tenantID, UserID: user.ID, BuyerUserID: user.ID,
		PlanID: snapshot.PlanID, Amount: int(snapshot.AmountCents), AmountCents: int(snapshot.AmountCents),
		CreatedAt: now.Format(time.RFC3339Nano), PriceSnapshot: map[string]any{},
	}
	commerceCtx, err := commerceContextForOrderTx(ctx, tx, commerceOrder, plan)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	settlementDecision, err := resolveOrderSettlementDecisionTx(ctx, tx, &commerceOrder, plan)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	if err := validateV132PricePlanSettlementDecision(settlementDecision); err != nil {
		return createVirtualOrderResponse{}, err
	}
	snapshot.DirectAgentID = commerceCtx.DirectAgentID
	snapshot.ParentAgentID = commerceCtx.ParentAgentID
	snapshot.OperationCenterID = commerceCtx.OperationCenterID

	signData := virtualPaySignData{
		OfferID: snapshot.OfferID, BuyQuantity: 1, Env: snapshot.Env, CurrencyType: snapshot.Currency,
		ProductID: snapshot.WeChatProductID, GoodsPrice: snapshot.TransactionPriceCents,
		OutTradeNo: orderNo, Attach: orderNo,
	}
	signDataJSON, err := json.Marshal(signData)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	paySig := calcVirtualPaySig(requestVirtualPayURI, signDataJSON, s.cfg.AppKey)
	signature := calcVirtualSignature(signDataJSON, wechatSession.SessionKey)
	expiresAt := now.Add(30 * time.Minute)
	responseAudit, _ := json.Marshal(map[string]any{"mode": snapshot.Mode, "signDataHash": hashSensitiveIdentifier(string(signDataJSON)), "snapshotVersion": 2})
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	rightsJSON, _ := json.Marshal(snapshot.Rights)
	commissionJSON, _ := json.Marshal(snapshot.CommissionSnapshotV2)
	rewardSnapshot := map[string]any{
		"commissionRuleVersion": snapshot.CommissionRuleVersion, "commissionSnapshot": snapshot.CommissionSnapshotV2,
		"referral": map[string]any{"directAgentId": snapshot.DirectAgentID, "parentAgentId": snapshot.ParentAgentID, "operationCenterId": snapshot.OperationCenterID},
	}
	rewardJSON, _ := json.Marshal(rewardSnapshot)
	rawOrder, _ := json.Marshal(map[string]any{
		"id": orderNo, "orderNo": orderNo, "tenantId": tenantID, "userId": user.ID,
		"planId": snapshot.PlanID, "planVersionId": snapshot.PlanVersionID, "pricePlanId": snapshot.PricePlanID,
		"amountCents": snapshot.AmountCents, "snapshotVersion": 2, "priceSnapshot": snapshot,
		"status": virtualOrderPending, "createdAt": now.Format(time.RFC3339Nano),
	})
	_, err = tx.ExecContext(ctx, `
		insert into xz_orders(
			id,order_no,tenant_id,user_id,buyer_user_id,plan_id,plan_version_id,price_plan_id,price_quote_id,
			snapshot_version,transaction_price_cents,wechat_product_id_snapshot,wechat_goods_price_cents,
			currency,payment_environment,rights_snapshot,commission_rule_version_snapshot,commission_snapshot_v2,
			order_type,business_order_type,amount_cents,status,fulfillment_status,entitlement_status,
			product_code,product_name,product_type,payment_channel,payment_scene,payment_mode,wechat_openid_hash,
			payment_expires_at,created_at,updated_at,price_snapshot,reward_snapshot,raw
		) values(
			$1,$1,$2,$3,$3,$4,$5,$6,$7,2,$8,$9,$10,$11,$12,$13::jsonb,$14,$15::jsonb,
			'VIRTUAL_PRODUCT','VIRTUAL_PRODUCT',$8,$16,$17,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28::jsonb,$29::jsonb,$30::jsonb
		)
	`, orderNo, tenantID, user.ID, snapshot.PlanID, snapshot.PlanVersionID, snapshot.PricePlanID, quote.DBID,
		snapshot.AmountCents, snapshot.WeChatProductID, snapshot.WeChatGoodsPriceCents, snapshot.Currency,
		snapshot.PaymentEnvironment, rightsJSON, snapshot.CommissionRuleVersion, commissionJSON, virtualOrderPending, entitlementPending,
		snapshot.ProductCode, snapshot.ProductName, snapshot.ProductType, snapshot.PaymentChannel, virtualPaymentScene,
		snapshot.Mode, hashSensitiveIdentifier(wechatSession.OpenID), expiresAt, now.Format(time.RFC3339Nano), now,
		snapshotJSON, rewardJSON, rawOrder)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	_, err = tx.ExecContext(ctx, `
		insert into xz_payment_records(
			id,payment_no,order_id,order_no,tenant_id,user_id,payment_channel,payment_scene,
			amount_cents,prepay_status,request_payload,response_payload
		) values($1,$1,$2,$2,$3,$4,$5,$6,$7,'SIGNED',$8::jsonb,$9::jsonb)
	`, paymentNo, orderNo, tenantID, user.ID, snapshot.PaymentChannel, virtualPaymentScene, snapshot.AmountCents, signDataJSON, responseAudit)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	if err := insertCommercialBillingOrderTx(ctx, tx, orderNo, tenantID, user.ID, snapshot.AmountCents, now, expiresAt); err != nil {
		return createVirtualOrderResponse{}, err
	}
	result, err := tx.ExecContext(ctx, `
		update xz_order_price_quotes set status='CONSUMED',consumed_at=$2,consumed_order_no=$3
		where id=$1 and status='AVAILABLE' and expires_at>$2 and user_id=$4
	`, quote.DBID, now, orderNo, user.ID)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return createVirtualOrderResponse{}, errPriceQuoteConsumed
	}
	if err := tx.Commit(); err != nil {
		return createVirtualOrderResponse{}, err
	}
	return createVirtualOrderResponse{
		OrderNo: orderNo, AmountCent: snapshot.AmountCents, SignData: string(signDataJSON),
		PaySig: paySig, Signature: signature, Mode: snapshot.Mode,
	}, nil
}

func loadPriceQuoteForUpdate(ctx context.Context, tx *sql.Tx, token string) (resolvedPriceQuoteV2, string, error) {
	quote, status, err := loadPriceQuoteRowForUpdate(ctx, tx, token)
	if err != nil {
		return resolvedPriceQuoteV2{}, "", err
	}
	if err := lockPriceQuoteConfiguration(ctx, tx, &quote, time.Now().UTC(), false); err != nil {
		return resolvedPriceQuoteV2{}, "", err
	}
	return quote, status, nil
}

func loadPriceQuoteRowForUpdate(ctx context.Context, tx *sql.Tx, token string) (resolvedPriceQuoteV2, string, error) {
	var quote resolvedPriceQuoteV2
	var status string
	var rightsJSON, commissionJSON []byte
	var whitelistEntryID sql.NullString
	var whitelistRevision sql.NullInt64
	var whitelistCheckedAt sql.NullTime
	err := tx.QueryRowContext(ctx, `
		select id,tenant_id,user_id,plan_id,plan_version_id,price_plan_id,entry_type,
		       transaction_price_cents,provider_price_snapshot_cents,wechat_goods_price_cents,
		       channel,environment,offer_id,wechat_product_id,payment_mode,currency,bonus_points,bonus_tokens,
		       rights_snapshot,commission_rule_version,commission_snapshot,expires_at,status,
		       payment_binding_id,wechat_good_id,
		       whitelist_entry_id,whitelist_revision,whitelist_checked_at
		from xz_order_price_quotes
		where quote_token_hash=$1
		for update
	`, hashSensitiveIdentifier(token)).Scan(
		&quote.DBID, &quote.TenantID, &quote.UserID, &quote.PlanID, &quote.PlanVersionID, &quote.PricePlanID,
		&quote.EntryType, &quote.TransactionPriceCents, &quote.QuotedBindingPriceCents,
		&quote.QuotedGoodsPriceCents, &quote.QuotedChannel, &quote.QuotedEnvironment, &quote.QuotedOfferID,
		&quote.QuotedWeChatProductID, &quote.QuotedMode, &quote.QuotedCurrency, &quote.QuotedBonusPoints,
		&quote.QuotedBonusTokens, &rightsJSON, &quote.CommissionRuleVersion,
		&commissionJSON, &quote.ExpiresAt, &status, &quote.PaymentBindingID, &quote.WeChatGoodID,
		&whitelistEntryID, &whitelistRevision, &whitelistCheckedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return resolvedPriceQuoteV2{}, "", errPriceQuoteForbidden
	}
	if err != nil {
		return resolvedPriceQuoteV2{}, "", err
	}
	quote.Rights = nil
	if err := json.Unmarshal(rightsJSON, &quote.Rights); err != nil {
		return resolvedPriceQuoteV2{}, "", err
	}
	quote.QuotedRights = cloneSnapshotMap(quote.Rights)
	quote.CommissionSnapshot = nil
	_ = json.Unmarshal(commissionJSON, &quote.CommissionSnapshot)
	quote.QuotedCommissionRuleVersion = quote.CommissionRuleVersion
	quote.QuotedCommissionSnapshot = cloneSnapshotMap(quote.CommissionSnapshot)
	if whitelistEntryID.Valid {
		quote.WhitelistEntryID = &whitelistEntryID.String
	}
	if whitelistRevision.Valid {
		quote.WhitelistRevision = &whitelistRevision.Int64
	}
	if whitelistCheckedAt.Valid {
		checkedAt := whitelistCheckedAt.Time.UTC()
		quote.WhitelistCheckedAt = &checkedAt
	}
	return quote, status, nil
}

func lockPriceQuoteConfiguration(ctx context.Context, tx *sql.Tx, quote *resolvedPriceQuoteV2, now time.Time, requireCurrentSelection bool) error {
	var planType string
	var planActive bool
	if err := tx.QueryRowContext(ctx, `
		select code,name,coalesce(plan_type,''),active from xz_plans where id=$1 for share
	`, quote.PlanID).Scan(&quote.PlanCode, &quote.PlanName, &planType, &planActive); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: business plan is missing", errPriceQuoteConfigurationChanged)
		}
		return err
	}

	var versionPlanID, versionStatus string
	var versionEffectiveAt, versionExpiresAt sql.NullTime
	var rightsJSON, commissionJSON []byte
	if err := tx.QueryRowContext(ctx, `
		select plan_id,business_type,rights_snapshot,commission_rule_version,commission_snapshot,
		       status,effective_at,expires_at
		from xz_plan_versions where id=$1 for share
	`, quote.PlanVersionID).Scan(&versionPlanID, &quote.BusinessType, &rightsJSON, &quote.CommissionRuleVersion,
		&commissionJSON, &versionStatus, &versionEffectiveAt, &versionExpiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: entitlement version is missing", errPriceQuoteConfigurationChanged)
		}
		return err
	}

	var pricePlanPlanID, pricePlanVersionID, priceStatus string
	var priceEnabled, isDefault, isVisible bool
	var priceEffectiveAt, priceExpiresAt sql.NullTime
	if err := tx.QueryRowContext(ctx, `
		select plan_id,plan_version_id,name,price_type,currency,bonus_points,bonus_tokens,sale_price_cents,original_price_cents,
		       channel,environment,enabled,status,is_default,is_visible,effective_at,expires_at
		from xz_price_plans where id=$1 for share
	`, quote.PricePlanID).Scan(&pricePlanPlanID, &pricePlanVersionID, &quote.PricePlanName, &quote.PriceType,
		&quote.Currency, &quote.BonusPoints, &quote.BonusTokens, &quote.PlanPriceCents, &quote.OriginalPriceCents,
		&quote.PlanChannel, &quote.PlanEnvironment,
		&priceEnabled, &priceStatus, &isDefault, &isVisible, &priceEffectiveAt, &priceExpiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: price plan is missing", errPriceQuoteConfigurationChanged)
		}
		return err
	}

	var goodEnabled, goodPublished bool
	var goodStatus, verificationStatus string
	var verificationExpiresAt sql.NullTime
	if err := tx.QueryRowContext(ctx, `
		select platform_price_cents,channel,environment,offer_id,product_id,mode,
		       enabled,published,status,verification_status,verification_expires_at
		from xz_wechat_virtual_goods where id=$1 for share
	`, quote.WeChatGoodID).Scan(&quote.WeChatGoodsPriceCents, &quote.GoodsChannel, &quote.GoodsEnvironment,
		&quote.OfferID, &quote.WeChatProductID, &quote.Mode, &goodEnabled, &goodPublished,
		&goodStatus, &verificationStatus, &verificationExpiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: WeChat good is missing", errPriceQuoteConfigurationChanged)
		}
		return err
	}

	var bindingEnabled bool
	var bindingStatus string
	if err := tx.QueryRowContext(ctx, `
		select price_plan_id,wechat_good_id,provider_price_snapshot_cents,channel,environment,enabled,status
		from xz_price_plan_payment_bindings where id=$1 for share
	`, quote.PaymentBindingID).Scan(&quote.BoundPricePlanID, &quote.BoundWeChatGoodID,
		&quote.BindingPriceCents, &quote.BindingChannel, &quote.BindingEnvironment,
		&bindingEnabled, &bindingStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: payment binding is missing", errPriceQuoteConfigurationChanged)
		}
		return err
	}

	now = time.Now().UTC()
	managedType := (planType == planTypeMemberPackage && quote.BusinessType == "MEMBER") ||
		(planType == planTypeAgentJoinPackage && quote.BusinessType == "AGENT")
	versionActive := versionStatus == "ACTIVE" && (!versionEffectiveAt.Valid || !now.Before(versionEffectiveAt.Time)) &&
		(!versionExpiresAt.Valid || now.Before(versionExpiresAt.Time))
	priceActive := priceEnabled && priceStatus == "ACTIVE" && (!priceEffectiveAt.Valid || !now.Before(priceEffectiveAt.Time)) &&
		(!priceExpiresAt.Valid || now.Before(priceExpiresAt.Time))
	if !planActive || !managedType || versionPlanID != quote.PlanID || pricePlanPlanID != quote.PlanID ||
		pricePlanVersionID != quote.PlanVersionID || !versionActive || !priceActive ||
		!bindingEnabled || bindingStatus != "ACTIVE" {
		return fmt.Errorf("%w: managed price configuration is no longer active", errPriceQuoteConfigurationChanged)
	}
	if verificationStatus == wechatGoodVerificationManual && verificationExpiresAt.Valid && !now.Before(verificationExpiresAt.Time) {
		return errWechatGoodVerificationExpired
	}
	if !goodEnabled || !goodPublished || goodStatus != "PUBLISHED" || verificationStatus != wechatGoodVerificationManual {
		return fmt.Errorf("%w: WeChat good is no longer manually confirmed and available", errPriceQuoteConfigurationChanged)
	}
	if requireCurrentSelection {
		if strings.EqualFold(quote.EntryType, pricePlanEntryTest) {
			if !strings.EqualFold(quote.PriceType, "TEST") || isDefault || isVisible {
				return fmt.Errorf("%w: test price selection changed", errPriceQuoteConfigurationChanged)
			}
		} else if strings.EqualFold(quote.PriceType, "TEST") || !isDefault || !isVisible {
			return fmt.Errorf("%w: public default price selection changed", errPriceQuoteConfigurationChanged)
		}
	}
	if err := validatePriceQuoteEntryType(*quote); err != nil {
		return err
	}
	quote.Rights = nil
	if err := json.Unmarshal(rightsJSON, &quote.Rights); err != nil {
		return err
	}
	if quote.Rights == nil {
		quote.Rights = map[string]any{}
	}
	if err := mergePricePlanBonusRights(quote.Rights, quote.BonusPoints, quote.BonusTokens); err != nil {
		return fmt.Errorf("%w: %v", errPriceQuoteConfigurationChanged, err)
	}
	quote.CommissionSnapshot = nil
	_ = json.Unmarshal(commissionJSON, &quote.CommissionSnapshot)
	if quote.CommissionSnapshot == nil {
		quote.CommissionSnapshot = map[string]any{}
	}
	return nil
}

func validatePriceQuoteEntryType(quote resolvedPriceQuoteV2) error {
	entryIsTest := strings.EqualFold(quote.EntryType, pricePlanEntryTest)
	priceIsTest := strings.EqualFold(quote.PriceType, "TEST")
	if entryIsTest != priceIsTest {
		return fmt.Errorf("%w: quote entry type %s does not match price type %s", errPriceQuoteConfigurationChanged, quote.EntryType, quote.PriceType)
	}
	return nil
}

func revalidatePinnedTestWhitelistForOrder(ctx context.Context, tx *sql.Tx, quote resolvedPriceQuoteV2) error {
	if err := validatePriceQuoteEntryType(quote); err != nil {
		return err
	}
	if !strings.EqualFold(quote.EntryType, pricePlanEntryTest) {
		if quote.WhitelistEntryID != nil || quote.WhitelistRevision != nil || quote.WhitelistCheckedAt != nil {
			return fmt.Errorf("%w: non-TEST quote contains a whitelist pin", errPriceQuoteConfigurationChanged)
		}
		return nil
	}
	if quote.WhitelistEntryID == nil || strings.TrimSpace(*quote.WhitelistEntryID) == "" ||
		quote.WhitelistRevision == nil || *quote.WhitelistRevision <= 0 || quote.WhitelistCheckedAt == nil {
		return errPricePlanNotEligible
	}

	var revision int64
	var enabled bool
	var lifecycle string
	var effectiveAt, expiresAt sql.NullTime
	err := tx.QueryRowContext(ctx, `
		select revision,enabled,
		       coalesce(lifecycle_status,case when enabled then 'ACTIVE' else 'DISABLED' end),
		       effective_at,expires_at
		from xz_price_plan_user_whitelist
		where id=$1 and price_plan_id=$2 and user_id=$3
		for share
	`, *quote.WhitelistEntryID, quote.PricePlanID, quote.UserID).Scan(
		&revision, &enabled, &lifecycle, &effectiveAt, &expiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return errPricePlanNotEligible
	}
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if revision < *quote.WhitelistRevision || !enabled || lifecycle != pricePlanWhitelistLifecycleActive ||
		(effectiveAt.Valid && now.Before(effectiveAt.Time)) ||
		(expiresAt.Valid && !now.Before(expiresAt.Time)) {
		return errPricePlanNotEligible
	}
	return nil
}

func planForV2Snapshot(snapshot virtualOrderSnapshot) (adminPlan, error) {
	if err := validateV2MemberAgentSnapshot(snapshot, snapshot.AmountCents); err != nil {
		return adminPlan{}, err
	}
	if snapshot.AmountCents > math.MaxInt || snapshot.CreditUnits > math.MaxInt || snapshot.PointUnits > math.MaxInt || snapshot.MemberDays > math.MaxInt {
		return adminPlan{}, errors.New("V2 snapshot exceeds local integer range")
	}
	entitlements := make(map[string]any, len(snapshot.Rights)+4)
	for key, value := range snapshot.Rights {
		entitlements[key] = value
	}
	// V2 fulfillment must never fall back to the historical member/agent token
	// constants. These canonical values come from the immutable order snapshot.
	entitlements["tokenAmount"] = snapshot.CreditUnits
	entitlements["tokenGrantAmount"] = snapshot.CreditUnits
	entitlements["tokenRightsValueCents"] = snapshot.CreditUnits
	entitlements["pointsAmount"] = snapshot.PointUnits
	return adminPlan{
		ID: snapshot.PlanID, Code: snapshot.ProductCode, Name: snapshot.ProductName,
		PlanType: snapshot.PlanType, Price: int(snapshot.AmountCents), PriceCents: int(snapshot.AmountCents),
		Points: int(snapshot.PointUnits), GrantPoints: int(snapshot.PointUnits), TokenAmount: int(snapshot.CreditUnits),
		MemberLevel: snapshot.MemberLevel, AgentLevel: snapshot.AgentLevel, DurationDays: int(snapshot.MemberDays),
		CommissionTemplateCode: snapshot.CommissionRuleVersion, Entitlements: entitlements, Active: true,
	}, nil
}

func validateV2QuoteEnvironment(quote resolvedPriceQuoteV2, configuredEnv int) error {
	expected := map[int]string{0: "PRODUCTION", 1: "SANDBOX"}[configuredEnv]
	if !strings.EqualFold(quote.PlanEnvironment, expected) {
		return fmt.Errorf("%w: quote=%s runtime=%s", errPricePlanEnvironmentMismatch, quote.PlanEnvironment, expected)
	}
	return nil
}
