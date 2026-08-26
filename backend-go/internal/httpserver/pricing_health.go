package httpserver

import "time"

const (
	pricingHealthStatusHealthy  = "HEALTHY"
	pricingHealthStatusDegraded = "DEGRADED"
	pricingHealthStatusBlocked  = "BLOCKED"

	pricingHealthSeverityWarning  = "WARNING"
	pricingHealthSeverityBlocking = "BLOCKING"

	pricingHealthIssueEntitlementVersionMissing  = "ENTITLEMENT_VERSION_MISSING"
	pricingHealthIssuePricePlanMissing           = "PRICE_PLAN_MISSING"
	pricingHealthIssueDefaultMissing             = "DEFAULT_PRICE_PLAN_MISSING"
	pricingHealthIssueGoodNotConfirmed           = "WECHAT_GOOD_NOT_CONFIRMED"
	pricingHealthIssueGoodVerificationExpired    = "WECHAT_GOOD_VERIFICATION_EXPIRED"
	pricingHealthIssueBindingMissing             = "PAYMENT_BINDING_MISSING"
	pricingHealthIssuePriceMismatch              = "PRICE_PLAN_WECHAT_PRICE_MISMATCH"
	pricingHealthIssuePaymentEnvironmentMismatch = "PRICE_PLAN_PAYMENT_ENV_MISMATCH"
	pricingHealthIssueWhitelistMissing           = "TEST_WHITELIST_MISSING"
	pricingHealthIssueV132Blocked                = "V132_BLOCKED"
	pricingHealthIssueGiftPointsUnavailable      = "PRICE_PLAN_GIFT_POINTS_FULFILLMENT_UNAVAILABLE"
	pricingHealthIssueDisabled                   = "DISABLED"
	pricingHealthIssueProviderCostMissing        = providerCostIssueMissing
	pricingHealthIssueProviderCostExpired        = providerCostIssueExpired
	pricingHealthIssueProviderCostAmbiguous      = providerCostIssueAmbiguous
	pricingHealthIssueProviderCostInvalid        = providerCostIssueInvalid
	pricingHealthIssueMarginInvalid              = marginIssueInvalid
	pricingHealthIssueNegativeMargin             = marginIssueNegative
	pricingHealthIssueMarginBelowTarget          = marginIssueBelowTarget
)

type pricingHealthView struct {
	CheckedAt     time.Time                   `json:"checkedAt"`
	Status        string                      `json:"status"`
	Summary       pricingHealthSummary        `json:"summary"`
	Issues        []pricingHealthIssue        `json:"issues"`
	BusinessPlans []pricingHealthBusinessPlan `json:"businessPlans"`
	PricePlans    []pricingHealthPricePlan    `json:"pricePlans"`
	WeChatGoods   []pricingHealthWeChatGood   `json:"wechatGoods"`
	Runtime       pricingHealthRuntime        `json:"runtime"`
}

type pricingHealthSummary struct {
	BusinessPlanCount    int `json:"businessPlanCount"`
	PricePlanCount       int `json:"pricePlanCount"`
	WeChatGoodCount      int `json:"wechatGoodCount"`
	IssueCount           int `json:"issueCount"`
	BlockedIssueCount    int `json:"blockedIssueCount"`
	DegradedIssueCount   int `json:"degradedIssueCount"`
	HealthyResourceCount int `json:"healthyResourceCount"`
}

type pricingHealthIssue struct {
	Code             string `json:"code"`
	Severity         string `json:"severity"`
	Scope            string `json:"scope"`
	PlanID           string `json:"planId,omitempty"`
	PricePlanID      string `json:"pricePlanId,omitempty"`
	PaymentBindingID string `json:"paymentBindingId,omitempty"`
	WeChatGoodID     string `json:"wechatGoodId,omitempty"`
	Environment      string `json:"environment,omitempty"`
	Message          string `json:"message"`
}

type pricingHealthDefaultSummary struct {
	PricePlanID     string `json:"pricePlanId"`
	SalePriceCents  int64  `json:"salePriceCents"`
	Currency        string `json:"currency"`
	WeChatGoodID    string `json:"wechatGoodId,omitempty"`
	WeChatProductID string `json:"wechatProductId,omitempty"`
}

type pricingHealthBusinessPlanDefaults struct {
	Production *pricingHealthDefaultSummary `json:"production"`
	Sandbox    *pricingHealthDefaultSummary `json:"sandbox"`
}

type pricingHealthBusinessPlan struct {
	PlanID          string                            `json:"planId"`
	Name            string                            `json:"name"`
	Status          string                            `json:"status"`
	IssueCodes      []string                          `json:"issueCodes"`
	ActiveVersionID string                            `json:"activeVersionId"`
	PricePlanCount  int                               `json:"pricePlanCount"`
	Defaults        pricingHealthBusinessPlanDefaults `json:"defaults"`
	active          bool
	channels        map[string]bool
}

type pricingHealthPricePlan struct {
	PricePlanID      string   `json:"pricePlanId"`
	PlanID           string   `json:"planId"`
	PlanVersionID    string   `json:"planVersionId"`
	Name             string   `json:"name"`
	PriceType        string   `json:"priceType"`
	Channel          string   `json:"channel"`
	Environment      string   `json:"environment"`
	Status           string   `json:"status"`
	IssueCodes       []string `json:"issueCodes"`
	SalePriceCents   int64    `json:"salePriceCents"`
	Currency         string   `json:"currency"`
	PaymentBindingID string   `json:"paymentBindingId,omitempty"`
	WeChatGoodID     string   `json:"wechatGoodId,omitempty"`
	WeChatProductID  string   `json:"wechatProductId,omitempty"`
	QuoteCount       int      `json:"quoteCount"`
	OrderCount       int      `json:"orderCount"`
	isDefault        bool
	enabled          bool
	bindingEnabled   bool
	bindingStatus    string
	bindingPrice     int64
	goodEnvironment  string
	goodChannel      string
	goodPrice        int64
	goodVerification string
	goodExpiry       *time.Time
	whitelistCount   int
	giftPoints       int64
	publicEligible   bool
	testEligible     bool
}

type pricingHealthWeChatGood struct {
	WeChatGoodID    string `json:"wechatGoodId"`
	WeChatProductID string `json:"wechatProductId"`
	Environment     string `json:"environment"`
	ReferenceCount  int    `json:"referenceCount"`
}

type pricingHealthRuntime struct {
	PricePlanCreationEnabled     bool     `json:"pricePlanCreationEnabled"`
	PricePlanTestEntryEnabled    bool     `json:"pricePlanTestEntryEnabled"`
	SnapshotV2FulfillmentEnabled bool     `json:"snapshotV2FulfillmentEnabled"`
	V132Blocked                  bool     `json:"v132Blocked"`
	V132Scope                    string   `json:"v132Scope"`
	V132AffectedTenantCount      int      `json:"v132AffectedTenantCount"`
	V132AffectedTenantIDs        []string `json:"v132AffectedTenantIds"`
}

func appendPricingHealthIssue(view *pricingHealthView, issue pricingHealthIssue) {
	view.Issues = append(view.Issues, issue)
	for i := range view.BusinessPlans {
		if issue.PlanID != "" && view.BusinessPlans[i].PlanID == issue.PlanID {
			view.BusinessPlans[i].IssueCodes = appendPricingHealthUniqueString(view.BusinessPlans[i].IssueCodes, issue.Code)
		}
	}
	for i := range view.PricePlans {
		if issue.PricePlanID != "" && view.PricePlans[i].PricePlanID == issue.PricePlanID {
			view.PricePlans[i].IssueCodes = appendPricingHealthUniqueString(view.PricePlans[i].IssueCodes, issue.Code)
		}
	}
}

func appendPricingHealthUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func finalizePricingHealth(view *pricingHealthView) {
	view.Summary.BusinessPlanCount = len(view.BusinessPlans)
	view.Summary.PricePlanCount = len(view.PricePlans)
	view.Summary.WeChatGoodCount = len(view.WeChatGoods)
	view.Summary.IssueCount = len(view.Issues)
	view.Status = pricingHealthStatusHealthy
	for _, issue := range view.Issues {
		if issue.Severity == pricingHealthSeverityBlocking {
			view.Summary.BlockedIssueCount++
			view.Status = pricingHealthStatusBlocked
		} else {
			view.Summary.DegradedIssueCount++
			if view.Status == pricingHealthStatusHealthy {
				view.Status = pricingHealthStatusDegraded
			}
		}
	}
	for i := range view.BusinessPlans {
		if len(view.BusinessPlans[i].IssueCodes) == 0 {
			view.BusinessPlans[i].Status = pricingHealthStatusHealthy
			view.Summary.HealthyResourceCount++
		} else {
			view.BusinessPlans[i].Status = pricingHealthStatusDegraded
		}
		for _, issue := range view.Issues {
			if issue.PlanID == view.BusinessPlans[i].PlanID && issue.Severity == pricingHealthSeverityBlocking {
				view.BusinessPlans[i].Status = pricingHealthStatusBlocked
				break
			}
		}
	}
	for i := range view.PricePlans {
		if len(view.PricePlans[i].IssueCodes) == 0 {
			view.PricePlans[i].Status = pricingHealthStatusHealthy
			view.Summary.HealthyResourceCount++
		} else {
			view.PricePlans[i].Status = pricingHealthStatusDegraded
		}
		for _, code := range view.PricePlans[i].IssueCodes {
			for _, issue := range view.Issues {
				if issue.Code == code && issue.PricePlanID == view.PricePlans[i].PricePlanID && issue.Severity == pricingHealthSeverityBlocking {
					view.PricePlans[i].Status = pricingHealthStatusBlocked
				}
			}
		}
	}
}
