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

const priceQuoteTTL = 5 * time.Minute

var errPricePlanCreationDisabled = errors.New("会员和代理商价格方案下单功能未启用")
var errPricePlanTestEntryDisabled = errors.New("测试价格入口未启用")

type createPriceQuoteResponse struct {
	QuoteID        string         `json:"quoteId"`
	PlanID         string         `json:"planId"`
	PlanVersionID  string         `json:"planVersionId"`
	PricePlanID    string         `json:"pricePlanId"`
	Name           string         `json:"name"`
	AmountCent     int64          `json:"amountCent"`
	OriginalAmount int64          `json:"originalAmountCent"`
	Currency       string         `json:"currency"`
	GiftPoints     int64          `json:"giftPoints"`
	GiftTokens     int64          `json:"giftTokens"`
	Environment    string         `json:"environment"`
	ExpiresAt      time.Time      `json:"expiresAt"`
	Entitlements   map[string]any `json:"entitlements"`
	TestOnly       bool           `json:"testOnly"`
}

type resolvedPriceQuoteV2 struct {
	DBID                        string
	TokenHash                   string
	TenantID                    string
	UserID                      string
	PlanID                      string
	PlanCode                    string
	PlanName                    string
	BusinessType                string
	PlanVersionID               string
	PricePlanID                 string
	PaymentBindingID            string
	WeChatGoodID                string
	PricePlanName               string
	PriceType                   string
	EntryType                   string
	Currency                    string
	QuotedCurrency              string
	BonusPoints                 int64
	QuotedBonusPoints           int64
	BonusTokens                 int64
	QuotedBonusTokens           int64
	QuotedRights                map[string]any
	QuotedCommissionRuleVersion string
	QuotedCommissionSnapshot    map[string]any
	TransactionPriceCents       int64
	PlanPriceCents              int64
	OriginalPriceCents          int64
	BindingPriceCents           int64
	QuotedBindingPriceCents     int64
	WeChatGoodsPriceCents       int64
	QuotedGoodsPriceCents       int64
	PlanChannel                 string
	BindingChannel              string
	GoodsChannel                string
	PlanEnvironment             string
	BindingEnvironment          string
	GoodsEnvironment            string
	QuotedChannel               string
	QuotedEnvironment           string
	QuotedOfferID               string
	QuotedWeChatProductID       string
	QuotedMode                  string
	OfferID                     string
	WeChatProductID             string
	Mode                        string
	BoundPricePlanID            string
	BoundWeChatGoodID           string
	Rights                      map[string]any
	CommissionRuleVersion       string
	CommissionSnapshot          map[string]any
	WhitelistEntryID            *string
	WhitelistRevision           *int64
	WhitelistCheckedAt          *time.Time
	ExpiresAt                   time.Time
}

func (a virtualPaymentAPI) createPublicPriceQuote(w http.ResponseWriter, r *http.Request) {
	a.createPriceQuote(w, r, pricePlanEntryPublic)
}

func (a virtualPaymentAPI) createTestPriceQuote(w http.ResponseWriter, r *http.Request) {
	a.createPriceQuote(w, r, pricePlanEntryTest)
}

func (a virtualPaymentAPI) createPriceQuote(w http.ResponseWriter, r *http.Request, entry string) {
	if !a.available(w) {
		return
	}
	user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var request struct {
		PlanID      string `json:"planId"`
		ProductCode string `json:"productCode"`
		PricePlanID string `json:"pricePlanId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	planRef := strings.TrimSpace(request.PlanID)
	if planRef == "" {
		planRef = strings.TrimSpace(request.ProductCode)
	}
	// pricePlanId is intentionally honored only by the dedicated test endpoint.
	requestedPricePlanID := ""
	if strings.EqualFold(entry, pricePlanEntryTest) {
		requestedPricePlanID = strings.TrimSpace(request.PricePlanID)
	}
	quote, err := a.service.issuePriceQuote(r.Context(), user, strings.TrimSpace(r.Header.Get("X-Tenant-Id")), planRef, requestedPricePlanID, entry)
	if err != nil {
		writePricePlanError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, quote)
}

func writePricePlanError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	code := "PRICE_PLAN_INVALID"
	switch {
	case errors.Is(err, errPricePlanNotEligible):
		status, code = http.StatusForbidden, "PRICE_PLAN_NOT_ELIGIBLE"
	case errors.Is(err, errPricePlanWhitelistRequired):
		status, code = http.StatusForbidden, "PRICE_PLAN_TEST_FORBIDDEN"
	case errors.Is(err, errPriceQuoteForbidden):
		status, code = http.StatusForbidden, "PRICE_QUOTE_FORBIDDEN"
	case errors.Is(err, errPricePlanCreationDisabled), errors.Is(err, errPricePlanTestEntryDisabled):
		status, code = http.StatusServiceUnavailable, "PRICE_PLAN_FEATURE_DISABLED"
	case errors.Is(err, errPricePlanUnavailable):
		status, code = http.StatusNotFound, "PRICE_PLAN_NOT_FOUND"
	case errors.Is(err, errPricePlanPriceMismatch):
		status, code = http.StatusConflict, "PRICE_PLAN_WECHAT_PRICE_MISMATCH"
	case errors.Is(err, errPricePlanEnvironmentMismatch):
		status, code = http.StatusConflict, "PRICE_PLAN_PAYMENT_ENV_MISMATCH"
	case errors.Is(err, errPriceQuoteExpired):
		status, code = http.StatusGone, "PRICE_QUOTE_EXPIRED"
	case errors.Is(err, errPriceQuoteConsumed):
		status, code = http.StatusConflict, "PRICE_QUOTE_ALREADY_CONSUMED"
	case errors.Is(err, errPriceQuoteConfigurationChanged):
		status, code = http.StatusConflict, "PRICE_QUOTE_CONFIGURATION_CHANGED"
	case errors.Is(err, errPricePlanV132SnapshotIncompatible):
		status, code = http.StatusUnprocessableEntity, "PRICE_PLAN_SETTLEMENT_CONFIGURATION_MISMATCH"
	case errors.Is(err, errPricePlanGiftPointsFulfillmentUnavailable):
		status, code = http.StatusUnprocessableEntity, "PRICE_PLAN_GIFT_POINTS_FULFILLMENT_UNAVAILABLE"
	case errors.Is(err, errCommissionRuleSnapshotIncomplete):
		status, code = http.StatusUnprocessableEntity, "PRICE_PLAN_COMMISSION_SNAPSHOT_INVALID"
	case errors.Is(err, errWechatGoodVerificationExpired):
		status, code = http.StatusConflict, "WECHAT_GOOD_VERIFICATION_EXPIRED"
	case errors.Is(err, errPriceQuoteRequired):
		status, code = http.StatusBadRequest, "PRICE_QUOTE_REQUIRED"
	case errors.Is(err, errVirtualPaymentRelogin):
		status, code = http.StatusUnauthorized, "WECHAT_SESSION_EXPIRED"
	case errors.Is(err, errVirtualPaymentUnavailable):
		status, code = http.StatusServiceUnavailable, "WECHAT_VIRTUAL_PAYMENT_UNAVAILABLE"
	case errors.Is(err, errForbidden):
		status, code = http.StatusForbidden, "PRICE_PLAN_FORBIDDEN"
	default:
		status, code = http.StatusInternalServerError, "PRICE_PLAN_INTERNAL_ERROR"
	}
	writeJSONWithStatus(w, status, map[string]any{"code": code, "error": err.Error()})
}

func (s *virtualPaymentService) issuePriceQuote(ctx context.Context, user adminUser, requestedTenant, planRef, requestedPricePlanID, entry string) (createPriceQuoteResponse, error) {
	if !s.cfg.PricePlanCreationEnabled {
		return createPriceQuoteResponse{}, errPricePlanCreationDisabled
	}
	if strings.EqualFold(entry, pricePlanEntryTest) && !s.cfg.PricePlanTestEntryEnabled {
		return createPriceQuoteResponse{}, errPricePlanTestEntryDisabled
	}
	if strings.TrimSpace(planRef) == "" {
		return createPriceQuoteResponse{}, errPricePlanUnavailable
	}
	tenantID, err := s.resolveTenant(ctx, user, requestedTenant)
	if err != nil {
		return createPriceQuoteResponse{}, err
	}
	now := time.Now().UTC()
	resolved, err := s.resolvePriceQuoteSource(ctx, tenantID, user.ID, planRef, requestedPricePlanID, entry, now)
	if err != nil {
		return createPriceQuoteResponse{}, err
	}
	resolved.TenantID = tenantID
	resolved.UserID = user.ID
	resolved.QuotedBindingPriceCents = resolved.BindingPriceCents
	resolved.QuotedGoodsPriceCents = resolved.WeChatGoodsPriceCents
	resolved.QuotedChannel = resolved.PlanChannel
	resolved.QuotedEnvironment = resolved.PlanEnvironment
	resolved.QuotedOfferID = resolved.OfferID
	resolved.QuotedWeChatProductID = resolved.WeChatProductID
	resolved.QuotedMode = resolved.Mode
	resolved.QuotedCurrency = resolved.Currency
	resolved.QuotedBonusPoints = resolved.BonusPoints
	resolved.QuotedBonusTokens = resolved.BonusTokens
	resolved.QuotedRights = cloneSnapshotMap(resolved.Rights)
	resolved.QuotedCommissionRuleVersion = resolved.CommissionRuleVersion
	resolved.QuotedCommissionSnapshot = cloneSnapshotMap(resolved.CommissionSnapshot)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return createPriceQuoteResponse{}, err
	}
	defer tx.Rollback()
	if err := lockPriceQuoteConfiguration(ctx, tx, &resolved, now, true); err != nil {
		return createPriceQuoteResponse{}, err
	}
	if err := validatePricePlanRuntimeFulfillment(resolved); err != nil {
		return createPriceQuoteResponse{}, err
	}
	if err := lockEligibleTestWhitelistForQuote(ctx, tx, &resolved, user.ID); err != nil {
		return createPriceQuoteResponse{}, err
	}
	now = time.Now().UTC()
	if err := validatePriceQuoteConfiguration(resolved); err != nil {
		return createPriceQuoteResponse{}, err
	}
	if err := validatePricePlanPaymentChain(resolved.paymentChain()); err != nil {
		return createPriceQuoteResponse{}, err
	}
	token := newVirtualBusinessNo("QUOTE")
	resolved.DBID = virtualPaymentResourceID("quote", token)
	resolved.TokenHash = hashSensitiveIdentifier(token)
	resolved.ExpiresAt = now.Add(priceQuoteTTL)
	rightsJSON, _ := json.Marshal(resolved.QuotedRights)
	commissionJSON, _ := json.Marshal(resolved.QuotedCommissionSnapshot)
	_, err = tx.ExecContext(ctx, `
		insert into xz_order_price_quotes(
			id,quote_token_hash,tenant_id,user_id,plan_id,plan_version_id,price_plan_id,
			payment_binding_id,wechat_good_id,entry_type,transaction_price_cents,
			provider_price_snapshot_cents,wechat_goods_price_cents,channel,environment,
			offer_id,wechat_product_id,payment_mode,currency,bonus_points,bonus_tokens,rights_snapshot,commission_rule_version,
			commission_snapshot,snapshot_version,status,expires_at,
			whitelist_entry_id,whitelist_revision,whitelist_checked_at
		) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22::jsonb,$23,$24::jsonb,2,'AVAILABLE',$25,$26,$27,$28)
	`, resolved.DBID, resolved.TokenHash, tenantID, user.ID, resolved.PlanID, resolved.PlanVersionID,
		resolved.PricePlanID, resolved.PaymentBindingID, resolved.WeChatGoodID, resolved.EntryType,
		resolved.TransactionPriceCents, resolved.BindingPriceCents, resolved.WeChatGoodsPriceCents,
		resolved.PlanChannel, resolved.PlanEnvironment, resolved.OfferID, resolved.WeChatProductID,
		resolved.Mode, resolved.Currency, resolved.BonusPoints, resolved.BonusTokens, rightsJSON,
		resolved.QuotedCommissionRuleVersion, commissionJSON, resolved.ExpiresAt,
		resolved.WhitelistEntryID, resolved.WhitelistRevision, resolved.WhitelistCheckedAt)
	if err != nil {
		return createPriceQuoteResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return createPriceQuoteResponse{}, err
	}
	return createPriceQuoteResponse{
		QuoteID: token, PlanID: resolved.PlanID, PlanVersionID: resolved.PlanVersionID,
		PricePlanID: resolved.PricePlanID, Name: resolved.PricePlanName,
		AmountCent: resolved.TransactionPriceCents, OriginalAmount: resolved.OriginalPriceCents,
		Currency: resolved.Currency, GiftPoints: resolved.BonusPoints, GiftTokens: resolved.BonusTokens,
		Environment: resolved.PlanEnvironment, ExpiresAt: resolved.ExpiresAt,
		Entitlements: cloneSnapshotMap(resolved.QuotedRights), TestOnly: strings.EqualFold(resolved.PriceType, "TEST"),
	}, nil
}

func (s *virtualPaymentService) isManagedMemberAgentPlanRef(ctx context.Context, planRef string) (bool, error) {
	planRef = strings.TrimSpace(planRef)
	if planRef == "" {
		return false, nil
	}
	var managed bool
	environment := map[int]string{0: "PRODUCTION", 1: "SANDBOX"}[s.cfg.Env]
	err := s.db.QueryRowContext(ctx, `
		select exists(
			select 1
			from xz_plans p
			join xz_plan_versions pv on pv.plan_id=p.id
			join xz_price_plans pp on pp.plan_id=p.id and pp.plan_version_id=pv.id
			join xz_price_plan_payment_bindings b on b.price_plan_id=pp.id
			where (p.id=$1 or lower(p.code)=lower($1) or lower(coalesce(p.payment_product_code,''))=lower($1))
			  and ((p.plan_type='MEMBER_PACKAGE' and pv.business_type='MEMBER')
			    or (p.plan_type='AGENT_JOIN_PACKAGE' and pv.business_type='AGENT'))
			  and pp.channel='WECHAT_VIRTUAL' and pp.environment=$2
			  and pp.price_type<>'TEST' and pp.is_visible=true
			  and (
				(pp.is_default=true and (
					(pp.enabled_at is not null and b.enabled_at is not null)
					or (pp.enabled=true and pp.status='ACTIVE' and b.enabled=true and b.status='ACTIVE')
				))
				or exists(select 1 from xz_order_price_quotes q where q.payment_binding_id=b.id)
				or exists(select 1 from xz_orders o where o.price_plan_id=pp.id and o.snapshot_version=2)
			  )
		)
	`, planRef, environment).Scan(&managed)
	return managed, err
}

func legacyOrderManagedV2Tx(ctx context.Context, tx *sql.Tx, planID, environment string) (bool, error) {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return false, nil
	}
	var lockedPlanID string
	if err := tx.QueryRowContext(ctx, `select id from xz_plans where id=$1 for share`, planID).Scan(&lockedPlanID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	var managed bool
	err := tx.QueryRowContext(ctx, `
		select exists(
			select 1
			from xz_plans p
			join xz_plan_versions pv on pv.plan_id=p.id
			join xz_price_plans pp on pp.plan_id=p.id and pp.plan_version_id=pv.id
			join xz_price_plan_payment_bindings b on b.price_plan_id=pp.id
			where p.id=$1
			  and ((p.plan_type='MEMBER_PACKAGE' and pv.business_type='MEMBER')
			    or (p.plan_type='AGENT_JOIN_PACKAGE' and pv.business_type='AGENT'))
			  and pp.channel='WECHAT_VIRTUAL' and ($2='' or pp.environment=$2)
			  and pp.price_type<>'TEST' and pp.is_visible=true
			  and (
				(pp.is_default=true and (
					(pp.enabled_at is not null and b.enabled_at is not null)
					or (pp.enabled=true and pp.status='ACTIVE' and b.enabled=true and b.status='ACTIVE')
				))
				or exists(select 1 from xz_order_price_quotes q where q.payment_binding_id=b.id)
				or exists(select 1 from xz_orders o where o.price_plan_id=pp.id and o.snapshot_version=2)
			  )
		)
	`, lockedPlanID, strings.ToUpper(strings.TrimSpace(environment))).Scan(&managed)
	return managed, err
}

func (r resolvedPriceQuoteV2) paymentChain() pricePaymentChain {
	return pricePaymentChain{
		QuotePriceCents: r.TransactionPriceCents, PlanPriceCents: r.PlanPriceCents,
		BindingPriceCents: r.BindingPriceCents, WeChatGoodsPriceCents: r.WeChatGoodsPriceCents,
		PlanChannel: r.PlanChannel, BindingChannel: r.BindingChannel, GoodsChannel: r.GoodsChannel,
		PlanEnvironment: r.PlanEnvironment, BindingEnvironment: r.BindingEnvironment, GoodsEnvironment: r.GoodsEnvironment,
	}
}

func (s *virtualPaymentService) resolvePriceQuoteSource(ctx context.Context, tenantID, userID, planRef, requestedPricePlanID, entry string, now time.Time) (resolvedPriceQuoteV2, error) {
	environment := map[int]string{0: "PRODUCTION", 1: "SANDBOX"}[s.cfg.Env]
	pricePredicate := "pp.price_type <> 'TEST' and pp.is_default=true and pp.is_visible=true and pp.audience_type='PUBLIC' and pp.currency='CNY'"
	whitelistPredicate := "($5::text=$5::text)"
	entryType := pricePlanEntryPublic
	if strings.EqualFold(entry, "LEGACY_PRODUCT_CODE") {
		entryType = "LEGACY_PRODUCT_CODE"
	}
	if strings.EqualFold(entry, pricePlanEntryTest) {
		pricePredicate = "pp.price_type='TEST' and pp.is_default=false and pp.is_visible=false and (pp.audience_type<>'PUBLIC' or pp.created_by is null) and pp.currency='CNY'"
		whitelistPredicate = `exists(
			select 1 from xz_price_plan_user_whitelist selected_whitelist
			where selected_whitelist.price_plan_id=pp.id and selected_whitelist.user_id=$5
			  and selected_whitelist.enabled=true
			  and coalesce(selected_whitelist.lifecycle_status,case when selected_whitelist.enabled then 'ACTIVE' else 'DISABLED' end)='ACTIVE'
			  and (selected_whitelist.effective_at is null or selected_whitelist.effective_at <= $4)
			  and (selected_whitelist.expires_at is null or selected_whitelist.expires_at > $4)
		)`
		entryType = pricePlanEntryTest
	}
	query := fmt.Sprintf(`
		select p.id,p.code,p.name,pv.business_type,pv.id,pp.id,pp.name,pp.price_type,
		       pp.currency,pp.bonus_points,pp.bonus_tokens,
		       pp.sale_price_cents,pp.original_price_cents,pp.channel,pp.environment,
		       b.id,b.provider_price_snapshot_cents,b.channel,b.environment,
		       g.id,g.platform_price_cents,g.channel,g.environment,g.offer_id,g.product_id,g.mode,
		       pv.rights_snapshot,pv.commission_rule_version,pv.commission_snapshot
		from xz_plans p
		join xz_plan_versions pv on pv.plan_id=p.id
		join xz_price_plans pp on pp.plan_id=p.id and pp.plan_version_id=pv.id
		join xz_price_plan_payment_bindings b on b.price_plan_id=pp.id
		join xz_wechat_virtual_goods g on g.id=b.wechat_good_id
		where (p.id=$1 or p.code=$1 or p.payment_product_code=$1)
		  and p.active=true
		  and ((p.plan_type='MEMBER_PACKAGE' and pv.business_type='MEMBER')
		    or (p.plan_type='AGENT_JOIN_PACKAGE' and pv.business_type='AGENT'))
		  and pv.status='ACTIVE'
		  and (pv.effective_at is null or pv.effective_at <= $4)
		  and (pv.expires_at is null or pv.expires_at > $4)
		  and pp.channel='WECHAT_VIRTUAL' and pp.environment=$2
		  and pp.enabled=true and pp.status='ACTIVE' and %s
		  and ($3='' or pp.id=$3)
		  and %s
		  and (pp.effective_at is null or pp.effective_at <= $4)
		  and (pp.expires_at is null or pp.expires_at > $4)
		  and b.enabled=true and b.status='ACTIVE'
		  and g.enabled=true and g.published=true and g.status='PUBLISHED'
		  and g.verification_status='MANUALLY_CONFIRMED_PUBLISHED'
		  and (g.verification_expires_at is null or g.verification_expires_at>$4)
		order by pp.created_at desc limit 1
	`, pricePredicate, whitelistPredicate)
	var r resolvedPriceQuoteV2
	var rightsJSON, commissionJSON []byte
	err := s.db.QueryRowContext(ctx, query, planRef, environment, requestedPricePlanID, now, userID).Scan(
		&r.PlanID, &r.PlanCode, &r.PlanName, &r.BusinessType, &r.PlanVersionID,
		&r.PricePlanID, &r.PricePlanName, &r.PriceType, &r.Currency, &r.BonusPoints, &r.BonusTokens,
		&r.PlanPriceCents, &r.OriginalPriceCents,
		&r.PlanChannel, &r.PlanEnvironment, &r.PaymentBindingID, &r.BindingPriceCents, &r.BindingChannel,
		&r.BindingEnvironment, &r.WeChatGoodID, &r.WeChatGoodsPriceCents, &r.GoodsChannel,
		&r.GoodsEnvironment, &r.OfferID, &r.WeChatProductID, &r.Mode, &rightsJSON,
		&r.CommissionRuleVersion, &commissionJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		if strings.EqualFold(entry, pricePlanEntryTest) {
			return resolvedPriceQuoteV2{}, errPricePlanNotEligible
		}
		return resolvedPriceQuoteV2{}, errPricePlanUnavailable
	}
	if err != nil {
		return resolvedPriceQuoteV2{}, err
	}
	r.EntryType = entryType
	r.TransactionPriceCents = r.PlanPriceCents
	if err := json.Unmarshal(rightsJSON, &r.Rights); err != nil {
		return resolvedPriceQuoteV2{}, err
	}
	if r.Rights == nil {
		r.Rights = map[string]any{}
	}
	if err := mergePricePlanBonusRights(r.Rights, r.BonusPoints, r.BonusTokens); err != nil {
		return resolvedPriceQuoteV2{}, err
	}
	_ = json.Unmarshal(commissionJSON, &r.CommissionSnapshot)
	if r.CommissionSnapshot == nil {
		r.CommissionSnapshot = map[string]any{}
	}
	return r, nil
}

func lockEligibleTestWhitelistForQuote(ctx context.Context, tx *sql.Tx, quote *resolvedPriceQuoteV2, userID string) error {
	if err := validatePriceQuoteEntryType(*quote); err != nil {
		return err
	}
	if !strings.EqualFold(quote.EntryType, pricePlanEntryTest) {
		quote.WhitelistEntryID = nil
		quote.WhitelistRevision = nil
		quote.WhitelistCheckedAt = nil
		return nil
	}

	var entryID, lifecycle string
	var revision int64
	var enabled bool
	var effectiveAt, expiresAt sql.NullTime
	err := tx.QueryRowContext(ctx, `
		select id,revision,enabled,
		       coalesce(lifecycle_status,case when enabled then 'ACTIVE' else 'DISABLED' end),
		       effective_at,expires_at
		from xz_price_plan_user_whitelist
		where price_plan_id=$1 and user_id=$2
		  and coalesce(lifecycle_status,case when enabled then 'ACTIVE' else 'DISABLED' end)='ACTIVE'
		order by created_at desc,id
		limit 1
		for share
	`, quote.PricePlanID, strings.TrimSpace(userID)).Scan(
		&entryID, &revision, &enabled, &lifecycle, &effectiveAt, &expiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return errPricePlanNotEligible
	}
	if err != nil {
		return err
	}
	checkedAt := time.Now().UTC()
	if revision <= 0 || !enabled || lifecycle != pricePlanWhitelistLifecycleActive ||
		(effectiveAt.Valid && checkedAt.Before(effectiveAt.Time)) ||
		(expiresAt.Valid && !checkedAt.Before(expiresAt.Time)) {
		return errPricePlanNotEligible
	}
	quote.WhitelistEntryID = &entryID
	quote.WhitelistRevision = &revision
	quote.WhitelistCheckedAt = &checkedAt
	return nil
}
