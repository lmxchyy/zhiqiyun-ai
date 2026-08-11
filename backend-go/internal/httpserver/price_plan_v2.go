package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	pricePlanEntryPublic = "PUBLIC"
	pricePlanEntryTest   = "TEST"
	quoteStatusAvailable = "AVAILABLE"
	quoteStatusConsumed  = "CONSUMED"
)

var (
	errPricePlanUnavailable                      = errors.New("当前套餐没有可用价格方案")
	errPricePlanWhitelistRequired                = errors.New("测试价格方案仅对白名单账号开放")
	errPricePlanNotEligible                      = errors.New("the user is not eligible for this test price plan")
	errPricePlanGiftPointsFulfillmentUnavailable = errors.New("gift points fulfillment is unavailable for V2 member or agent price-plan orders")
	errPricePlanPriceMismatch                    = errors.New("价格方案与微信道具价格不一致，已禁止下单")
	errPricePlanEnvironmentMismatch              = errors.New("价格方案与微信道具支付环境不一致，已禁止下单")
	errPriceQuoteExpired                         = errors.New("报价已过期，请重新获取")
	errPriceQuoteForbidden                       = errors.New("报价不属于当前用户")
	errPriceQuoteConsumed                        = errors.New("报价已被使用，请重新获取")
	errPriceQuoteRequired                        = errors.New("创建订单必须提供服务端签发的 quoteId")
	errPriceQuoteConfigurationChanged            = errors.New("报价关联的支付配置已发生变化，请重新获取报价")
	errWechatGoodVerificationExpired             = errors.New("微信道具人工发布确认已过期，请重新确认后再支付")
)

func validatePricePlanRuntimeFulfillment(quote resolvedPriceQuoteV2) error {
	if quote.BonusPoints > 0 {
		return fmt.Errorf("%w: giftPoints=%d", errPricePlanGiftPointsFulfillmentUnavailable, quote.BonusPoints)
	}
	return nil
}

type pricePlanV2 struct {
	ID                 string
	PlanID             string
	PlanVersionID      string
	PriceType          string
	Channel            string
	Environment        string
	SalePriceCents     int64
	OriginalPriceCents int64
	IsDefault          bool
	IsVisible          bool
	Enabled            bool
	Status             string
	EffectiveAt        *time.Time
	ExpiresAt          *time.Time
}

func (p pricePlanV2) activeAt(now time.Time) bool {
	if !p.Enabled || !strings.EqualFold(p.Status, "ACTIVE") {
		return false
	}
	if p.EffectiveAt != nil && now.Before(p.EffectiveAt.UTC()) {
		return false
	}
	return p.ExpiresAt == nil || now.Before(p.ExpiresAt.UTC())
}

func resolvePricePlanForEntry(plans []pricePlanV2, entry string, whitelisted bool, now time.Time) (pricePlanV2, error) {
	if strings.EqualFold(entry, pricePlanEntryTest) && !whitelisted {
		return pricePlanV2{}, errPricePlanWhitelistRequired
	}
	for _, plan := range plans {
		if !plan.activeAt(now) {
			continue
		}
		if strings.EqualFold(entry, pricePlanEntryTest) {
			if strings.EqualFold(plan.PriceType, "TEST") && !plan.IsDefault && !plan.IsVisible {
				return plan, nil
			}
			continue
		}
		if !strings.EqualFold(plan.PriceType, "TEST") && plan.IsDefault && plan.IsVisible {
			return plan, nil
		}
	}
	return pricePlanV2{}, errPricePlanUnavailable
}

type pricePaymentChain struct {
	QuotePriceCents       int64
	PlanPriceCents        int64
	BindingPriceCents     int64
	WeChatGoodsPriceCents int64
	PlanChannel           string
	BindingChannel        string
	GoodsChannel          string
	PlanEnvironment       string
	BindingEnvironment    string
	GoodsEnvironment      string
}

func validatePricePlanPaymentChain(chain pricePaymentChain) error {
	if chain.QuotePriceCents <= 0 || chain.QuotePriceCents != chain.PlanPriceCents ||
		chain.QuotePriceCents != chain.BindingPriceCents || chain.QuotePriceCents != chain.WeChatGoodsPriceCents {
		return fmt.Errorf("%w: quote=%d pricePlan=%d binding=%d wechatGood=%d", errPricePlanPriceMismatch,
			chain.QuotePriceCents, chain.PlanPriceCents, chain.BindingPriceCents, chain.WeChatGoodsPriceCents)
	}
	if !strings.EqualFold(chain.PlanChannel, chain.BindingChannel) || !strings.EqualFold(chain.PlanChannel, chain.GoodsChannel) ||
		!strings.EqualFold(chain.PlanEnvironment, chain.BindingEnvironment) || !strings.EqualFold(chain.PlanEnvironment, chain.GoodsEnvironment) {
		return fmt.Errorf("%w: plan=%s/%s binding=%s/%s wechatGood=%s/%s", errPricePlanEnvironmentMismatch,
			chain.PlanChannel, chain.PlanEnvironment, chain.BindingChannel, chain.BindingEnvironment, chain.GoodsChannel, chain.GoodsEnvironment)
	}
	return nil
}

type priceQuoteV2 struct {
	mu        sync.Mutex
	ID        string
	UserID    string
	Status    string
	ExpiresAt time.Time
}

func validatePriceQuoteConfiguration(quote resolvedPriceQuoteV2) error {
	if quote.BoundPricePlanID != quote.PricePlanID || quote.BoundWeChatGoodID != quote.WeChatGoodID ||
		quote.QuotedBindingPriceCents != quote.BindingPriceCents ||
		quote.QuotedGoodsPriceCents != quote.WeChatGoodsPriceCents ||
		!strings.EqualFold(quote.QuotedChannel, quote.PlanChannel) ||
		!strings.EqualFold(quote.QuotedChannel, quote.BindingChannel) ||
		!strings.EqualFold(quote.QuotedChannel, quote.GoodsChannel) ||
		!strings.EqualFold(quote.QuotedEnvironment, quote.PlanEnvironment) ||
		!strings.EqualFold(quote.QuotedEnvironment, quote.BindingEnvironment) ||
		!strings.EqualFold(quote.QuotedEnvironment, quote.GoodsEnvironment) ||
		quote.QuotedOfferID != quote.OfferID || quote.QuotedWeChatProductID != quote.WeChatProductID ||
		quote.QuotedMode != quote.Mode || !strings.EqualFold(quote.QuotedCurrency, quote.Currency) ||
		quote.QuotedBonusPoints != quote.BonusPoints || quote.QuotedBonusTokens != quote.BonusTokens ||
		!snapshotMapsEqual(quote.QuotedRights, quote.Rights) ||
		quote.QuotedCommissionRuleVersion != quote.CommissionRuleVersion ||
		!snapshotMapsEqual(quote.QuotedCommissionSnapshot, quote.CommissionSnapshot) {
		return fmt.Errorf("%w: quote=%s pricePlan=%s binding=%s good=%s", errPriceQuoteConfigurationChanged,
			quote.DBID, quote.PricePlanID, quote.PaymentBindingID, quote.WeChatGoodID)
	}
	return nil
}

func snapshotMapsEqual(left, right map[string]any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func cloneSnapshotMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func mergePricePlanBonusRights(rights map[string]any, bonusPoints, bonusTokens int64) error {
	if rights == nil {
		return errors.New("entitlement rights are missing")
	}
	if bonusPoints < 0 || bonusTokens < 0 {
		return errors.New("price plan gifts cannot be negative")
	}
	baseTokens := int64Value(rights["tokenAmount"])
	if baseTokens == 0 {
		baseTokens = int64Value(rights["tokenGrantAmount"])
	}
	basePoints := int64Value(rights["pointsAmount"])
	if baseTokens < 0 || basePoints < 0 || baseTokens > math.MaxInt64-bonusTokens || basePoints > math.MaxInt64-bonusPoints {
		return errors.New("price plan gift entitlement overflow")
	}
	rights["baseTokenAmount"] = baseTokens
	rights["basePointsAmount"] = basePoints
	rights["pricePlanGiftTokens"] = bonusTokens
	rights["pricePlanGiftPoints"] = bonusPoints
	rights["tokenAmount"] = baseTokens + bonusTokens
	rights["tokenGrantAmount"] = baseTokens + bonusTokens
	rights["pointsAmount"] = basePoints + bonusPoints
	return nil
}

func consumePriceQuote(quote *priceQuoteV2, userID string, now time.Time) (*priceQuoteV2, error) {
	quote.mu.Lock()
	defer quote.mu.Unlock()
	if quote.UserID != userID {
		return nil, errPriceQuoteForbidden
	}
	if !now.Before(quote.ExpiresAt) {
		return nil, errPriceQuoteExpired
	}
	if quote.Status != quoteStatusAvailable {
		return nil, errPriceQuoteConsumed
	}
	quote.Status = quoteStatusConsumed
	return quote, nil
}

func snapshotForResolvedPriceQuote(quote resolvedPriceQuoteV2) (virtualOrderSnapshot, error) {
	businessType := strings.ToUpper(strings.TrimSpace(quote.BusinessType))
	productType := ""
	planType := ""
	switch businessType {
	case "MEMBER":
		productType = "MEMBERSHIP"
		planType = planTypeMemberPackage
	case "AGENT":
		productType = "IDENTITY"
		planType = planTypeAgentJoinPackage
	default:
		return virtualOrderSnapshot{}, fmt.Errorf("unsupported V2 business type: %s", quote.BusinessType)
	}
	rights := quote.Rights
	if quote.QuotedRights != nil {
		rights = quote.QuotedRights
	}
	commissionRuleVersion := quote.CommissionRuleVersion
	if quote.QuotedCommissionRuleVersion != "" {
		commissionRuleVersion = quote.QuotedCommissionRuleVersion
	}
	commissionSnapshot := quote.CommissionSnapshot
	if quote.QuotedCommissionSnapshot != nil {
		commissionSnapshot = quote.QuotedCommissionSnapshot
	}
	rules, err := decodeCommissionRuleSnapshots(commissionSnapshot)
	if err != nil {
		return virtualOrderSnapshot{}, err
	}
	tokens := int64Value(rights["tokenAmount"])
	if tokens == 0 {
		tokens = int64Value(rights["tokenGrantAmount"])
	}
	snapshot := virtualOrderSnapshot{
		SnapshotVersion: 2, PlanID: quote.PlanID, PlanVersionID: quote.PlanVersionID, PricePlanID: quote.PricePlanID,
		Currency: quote.Currency, PricePlanGiftPoints: quote.BonusPoints, PricePlanGiftTokens: quote.BonusTokens,
		ProductCode: quote.PlanCode, ProductName: quote.PlanName, ProductType: productType, PlanType: planType,
		AmountCents: quote.TransactionPriceCents, TransactionPriceCents: quote.TransactionPriceCents,
		UnitPriceCents: quote.TransactionPriceCents, BuyQuantity: 1,
		MemberLevel: stringValue(rights["memberLevel"]), AgentLevel: stringValue(rights["agentLevel"]),
		MemberDays: int64Value(rights["durationDays"]), CreditUnits: tokens,
		PointUnits: int64Value(rights["pointsAmount"]), UnitCreditUnits: tokens,
		OfferID: quote.OfferID, WeChatProductID: quote.WeChatProductID, WeChatGoodsPriceCents: quote.WeChatGoodsPriceCents,
		Mode: quote.Mode, Env: map[string]int{"PRODUCTION": 0, "SANDBOX": 1}[strings.ToUpper(quote.PlanEnvironment)],
		PaymentChannel: quote.PlanChannel, PaymentEnvironment: strings.ToUpper(quote.PlanEnvironment), Rights: cloneSnapshotMap(rights),
		CommissionRuleVersion: commissionRuleVersion, CommissionSnapshotV2: cloneSnapshotMap(commissionSnapshot),
		CommissionTemplateCode: commissionRuleVersion, CommissionSnapshotCaptured: true, CommissionRules: rules,
	}
	if snapshot.Currency == "" {
		snapshot.Currency = "CNY"
	}
	return snapshot, validateV2MemberAgentSnapshot(snapshot, quote.TransactionPriceCents)
}

func validateV2MemberAgentSnapshot(snapshot virtualOrderSnapshot, orderAmount int64) error {
	if snapshot.SnapshotVersion != 2 || snapshot.AmountCents <= 0 || snapshot.AmountCents != orderAmount ||
		snapshot.TransactionPriceCents != orderAmount || snapshot.UnitPriceCents != orderAmount ||
		snapshot.WeChatGoodsPriceCents != orderAmount {
		return fmt.Errorf("%w: V2 immutable price snapshot mismatch", errVirtualPaymentMismatch)
	}
	if snapshot.Currency != "" && !strings.EqualFold(snapshot.Currency, "CNY") {
		return errors.New("V2 WeChat virtual payment currency must be CNY")
	}
	if snapshot.PricePlanGiftPoints < 0 || snapshot.PricePlanGiftTokens < 0 ||
		snapshot.PointUnits < snapshot.PricePlanGiftPoints || snapshot.CreditUnits < snapshot.PricePlanGiftTokens {
		return errors.New("V2 price plan gift snapshot is invalid")
	}
	rightsTokens, valid := v2RightsTokenAmount(snapshot.Rights)
	if !valid || snapshot.CreditUnits != rightsTokens {
		return fmt.Errorf("%w: V2 token rights projection mismatch", errVirtualPaymentMismatch)
	}
	rightsPoints, valid := v2RightsInteger(snapshot.Rights, "pointsAmount")
	if !valid || snapshot.PointUnits != rightsPoints {
		return fmt.Errorf("%w: V2 point rights projection mismatch", errVirtualPaymentMismatch)
	}
	rightsDays, valid := v2RightsInteger(snapshot.Rights, "durationDays")
	if !valid || snapshot.MemberDays != rightsDays {
		return fmt.Errorf("%w: V2 duration rights projection mismatch", errVirtualPaymentMismatch)
	}
	switch normalizePlanTypeString(snapshot.PlanType) {
	case planTypeMemberPackage:
		if snapshot.MemberLevel == "" || snapshot.MemberDays <= 0 || snapshot.CreditUnits <= 0 {
			return errors.New("V2 membership rights snapshot is invalid")
		}
		if snapshot.MemberLevel != stringValue(snapshot.Rights["memberLevel"]) {
			return fmt.Errorf("%w: V2 member level rights projection mismatch", errVirtualPaymentMismatch)
		}
	case planTypeAgentJoinPackage:
		if snapshot.AgentLevel == "" || snapshot.CreditUnits <= 0 {
			return errors.New("V2 agent rights snapshot is invalid")
		}
		if snapshot.AgentLevel != stringValue(snapshot.Rights["agentLevel"]) {
			return fmt.Errorf("%w: V2 agent level rights projection mismatch", errVirtualPaymentMismatch)
		}
	default:
		return errors.New("V2 snapshot is not a member or agent order")
	}
	if snapshot.PlanID == "" || snapshot.PlanVersionID == "" || snapshot.PricePlanID == "" ||
		snapshot.WeChatProductID == "" || snapshot.PaymentChannel == "" || snapshot.PaymentEnvironment == "" {
		return errors.New("V2 order snapshot identity is incomplete")
	}
	if err := validateV2CommissionSnapshot(snapshot); err != nil {
		return err
	}
	return nil
}

func decodeCommissionRuleSnapshots(snapshot map[string]any) ([]commissionRuleSnapshot, error) {
	items := make([]commissionRuleSnapshot, 0)
	if snapshot == nil {
		return items, nil
	}
	rawRules, exists := snapshot["rules"]
	if !exists || rawRules == nil {
		return items, nil
	}
	encoded, err := json.Marshal(rawRules)
	if err != nil {
		return nil, fmt.Errorf("%w: rules cannot be encoded: %v", errCommissionRuleSnapshotIncomplete, err)
	}
	if err := json.Unmarshal(encoded, &items); err != nil {
		return nil, fmt.Errorf("%w: rules must be an array of valid rule snapshots: %v", errCommissionRuleSnapshotIncomplete, err)
	}
	return items, nil
}

func validateV2CommissionSnapshot(snapshot virtualOrderSnapshot) error {
	if !snapshot.CommissionSnapshotCaptured && snapshot.CommissionSnapshotV2 == nil && len(snapshot.CommissionRules) == 0 {
		return nil
	}
	normalized, err := decodeCommissionRuleSnapshots(snapshot.CommissionSnapshotV2)
	if err != nil {
		return err
	}
	normalizedJSON, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("%w: normalized rules cannot be encoded: %v", errCommissionRuleSnapshotIncomplete, err)
	}
	topLevel := snapshot.CommissionRules
	if topLevel == nil {
		topLevel = make([]commissionRuleSnapshot, 0)
	}
	topLevelJSON, err := json.Marshal(topLevel)
	if err != nil {
		return fmt.Errorf("%w: top-level rules cannot be encoded: %v", errCommissionRuleSnapshotIncomplete, err)
	}
	if !bytes.Equal(normalizedJSON, topLevelJSON) {
		return fmt.Errorf("%w: top-level rules do not match commissionSnapshotV2", errCommissionRuleSnapshotIncomplete)
	}
	_, err = rebuildCommissionRulesFromSnapshot(commissionRuleSnapshotContext{
		TenantID: "snapshot-validation", ProductType: normalizePlanTypeString(snapshot.PlanType),
		ProductID: snapshot.PlanID, PaidAt: time.Unix(1, 0).UTC(),
	}, normalized)
	return err
}

func v2RightsTokenAmount(rights map[string]any) (int64, bool) {
	tokens, tokenPresent, valid := v2RightsIntegerValue(rights, "tokenAmount")
	if !valid {
		return 0, false
	}
	legacyTokens, legacyPresent, valid := v2RightsIntegerValue(rights, "tokenGrantAmount")
	if !valid {
		return 0, false
	}
	if tokenPresent && tokens != 0 {
		if legacyPresent && legacyTokens != tokens {
			return 0, false
		}
		return tokens, true
	}
	if legacyPresent {
		return legacyTokens, true
	}
	return tokens, tokenPresent
}

func v2RightsInteger(rights map[string]any, key string) (int64, bool) {
	value, _, valid := v2RightsIntegerValue(rights, key)
	return value, valid
}

func v2RightsIntegerValue(rights map[string]any, key string) (int64, bool, bool) {
	if rights == nil {
		return 0, false, false
	}
	raw, present := rights[key]
	if !present {
		return 0, false, true
	}
	value, valid := exactInt64Value(raw)
	return value, true, valid
}

func exactInt64Value(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		if uint64(typed) > uint64(math.MaxInt64) {
			return 0, false
		}
		return int64(typed), true
	case uint8:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		if typed > uint64(math.MaxInt64) {
			return 0, false
		}
		return int64(typed), true
	case float32:
		return exactFloat64Int64(float64(typed))
	case float64:
		return exactFloat64Int64(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return parsed, true
		}
		parsed, err := strconv.ParseFloat(string(typed), 64)
		if err != nil {
			return 0, false
		}
		return exactFloat64Int64(parsed)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func exactFloat64Int64(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return 0, false
	}
	parsed, err := strconv.ParseInt(strconv.FormatFloat(value, 'f', -1, 64), 10, 64)
	return parsed, err == nil
}
