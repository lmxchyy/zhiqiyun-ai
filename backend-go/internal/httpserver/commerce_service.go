package httpserver

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	commissionapp "xianzhi-ai/backend-go/internal/app/commission"
)

const (
	planTypeMemberPackage          = "MEMBER_PACKAGE"
	planTypeTokenRecharge          = "TOKEN_RECHARGE"
	planTypeAgentJoinPackage       = "AGENT_JOIN_PACKAGE"
	planTypeOperationCenterPackage = "OPERATION_CENTER_PACKAGE"

	orderTypeUserRechargeDirect      = "USER_RECHARGE_DIRECT"
	orderTypeUserRechargeSecondLevel = "USER_RECHARGE_SECOND_LEVEL"
	orderTypePlatformDirectRecharge  = "PLATFORM_DIRECT_USER_RECHARGE"
	orderTypeAgentJoin               = "AGENT_JOIN"
	orderTypeOperationCenterJoin     = "OPERATION_CENTER_JOIN"

	memberLevelFree       = "FREE"
	memberLevelBasic      = "BASIC"
	memberLevelPro        = "PRO"
	memberLevelEnterprise = "ENTERPRISE"

	agentStatusNone       = "NONE"
	agentStatusActive     = "ACTIVE"
	operationStatusNone   = "NONE"
	operationStatusActive = "ACTIVE"

	receiverTypeAgent           = "AGENT"
	receiverTypeOperationCenter = "OPERATION_CENTER"
	receiverTypePlatform        = "PLATFORM"

	commissionTypeDirectAgentReward     = "DIRECT_AGENT_REWARD"
	commissionTypeParentAgentReward     = "PARENT_AGENT_REWARD"
	commissionTypeOperationCenterReward = "OPERATION_CENTER_REWARD"
	commissionTypePlatformIncome        = "PLATFORM_INCOME"
)

type commissionOrderContext struct {
	OrderID              string
	OrderType            string
	PlanType             string
	AmountCents          int
	BuyerUserID          string
	DirectAgentID        string
	ParentAgentID        string
	OperationCenterID    string
	TokenGrantAmount     int
	TokenGrantValueCents int
}

type commissionSettlementResult struct {
	OrderType                  string
	DirectAgentRewardCents     int
	ParentAgentRewardCents     int
	OperationCenterRewardCents int
	TokenGrantAmount           int
	TokenGrantValueCents       int
	PlatformIncomeCents        int
}

//go:embed commission_compat_rules.json
var commissionCompatibilityRulesJSON []byte

var (
	commissionCompatibilityRulesOnce sync.Once
	commissionCompatibilityRulesData []commissionapp.CommissionRule
	commissionCompatibilityRulesErr  error
)

func calculateCommissionSettlement(ctx commissionOrderContext) (commissionSettlementResult, error) {
	ctx.OrderType = normalizeCommerceCode(ctx.OrderType)
	ctx.PlanType = normalizePlanTypeString(ctx.PlanType)
	if ctx.AmountCents <= 0 {
		return commissionSettlementResult{}, fmt.Errorf("order amount must be positive")
	}
	switch ctx.OrderType {
	case orderTypeUserRechargeDirect, orderTypeUserRechargeSecondLevel, orderTypeAgentJoin,
		orderTypeOperationCenterJoin, orderTypePlatformDirectRecharge:
	default:
		return commissionSettlementResult{}, fmt.Errorf("unsupported commerce order type: %s", ctx.OrderType)
	}
	if ctx.PlanType == "" {
		switch ctx.OrderType {
		case orderTypeAgentJoin:
			ctx.PlanType = planTypeAgentJoinPackage
		case orderTypeOperationCenterJoin:
			ctx.PlanType = planTypeOperationCenterPackage
		case orderTypePlatformDirectRecharge:
			ctx.PlanType = planTypeTokenRecharge
		default:
			ctx.PlanType = planTypeMemberPackage
		}
	}
	rules, err := loadCommissionCompatibilityRules()
	if err != nil {
		return commissionSettlementResult{}, err
	}
	agentIDs := map[int]string{}
	if ctx.DirectAgentID != "" {
		agentIDs[1] = ctx.DirectAgentID
	}
	if ctx.ParentAgentID != "" {
		agentIDs[2] = ctx.ParentAgentID
	}
	orderID := firstNonEmptyString(ctx.OrderID, "compat:"+ctx.OrderType)
	sourceUserID := firstNonEmptyString(ctx.BuyerUserID, "compat-user")
	engineResult, err := commissionapp.NewEngine().Calculate(commissionapp.CalculationInput{
		TenantID: "tenant_default", OrderID: orderID, OrderNo: orderID,
		ProductType: ctx.PlanType, ProductID: ctx.PlanType, SourceUserID: sourceUserID,
		OrderAmountCents: commissionapp.AmountCents(ctx.AmountCents),
		PaidAmountCents:  commissionapp.AmountCents(ctx.AmountCents),
		Quantity:         1,
		PaidAt:           time.Now().UTC(),
		Rules:            rules,
		Relationships: commissionapp.RelationshipSnapshot{
			AgentIDsByLevel: agentIDs, OperationCenterID: ctx.OperationCenterID, PlatformID: "platform",
		},
	})
	if err != nil {
		return commissionSettlementResult{}, err
	}
	return compatibilitySettlementResult(ctx, engineResult)
}

func loadCommissionCompatibilityRules() ([]commissionapp.CommissionRule, error) {
	commissionCompatibilityRulesOnce.Do(func() {
		commissionCompatibilityRulesErr = json.Unmarshal(commissionCompatibilityRulesJSON, &commissionCompatibilityRulesData)
	})
	if commissionCompatibilityRulesErr != nil {
		return nil, fmt.Errorf("decode compatibility commission rules: %w", commissionCompatibilityRulesErr)
	}
	return append([]commissionapp.CommissionRule(nil), commissionCompatibilityRulesData...), nil
}

func validateSettlementAmount(amountCents int, result commissionSettlementResult) error {
	total := result.DirectAgentRewardCents +
		result.ParentAgentRewardCents +
		result.OperationCenterRewardCents +
		result.PlatformIncomeCents
	if total != amountCents {
		return fmt.Errorf("settlement amount mismatch: order=%d settlement=%d", amountCents, total)
	}
	return nil
}

func settlementCommissionRecords(ctx commissionOrderContext, result commissionSettlementResult, now string) []adminCommission {
	records := []adminCommission{}
	appendRecord := func(receiverType, receiverID, commissionType string, amount int, status string) {
		if amount <= 0 {
			return
		}
		id := "commission_" + shortID(ctx.OrderID+"_"+commissionType+"_"+receiverID)
		records = append(records, adminCommission{
			ID:             id,
			OrderID:        ctx.OrderID,
			AgentID:        receiverAgentID(receiverType, receiverID),
			ReceiverType:   receiverType,
			ReceiverID:     receiverID,
			AmountCents:    amount,
			CommissionType: commissionType,
			Rate:           0,
			Status:         status,
			SettleStatus:   settlementStatusForReceiver(receiverType),
			RuleSnapshot: map[string]any{
				"settlementMode":       "FIXED_COMMERCE_RULE",
				"orderType":            result.OrderType,
				"planType":             ctx.PlanType,
				"amountCents":          ctx.AmountCents,
				"tokenGrantAmount":     result.TokenGrantAmount,
				"tokenGrantValueCents": result.TokenGrantValueCents,
				"commissionType":       commissionType,
				"receiverType":         receiverType,
				"receiverId":           receiverID,
			},
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	appendRecord(receiverTypeAgent, ctx.DirectAgentID, commissionTypeDirectAgentReward, result.DirectAgentRewardCents, "PENDING")
	appendRecord(receiverTypeAgent, ctx.ParentAgentID, commissionTypeParentAgentReward, result.ParentAgentRewardCents, "PENDING")
	appendRecord(receiverTypeOperationCenter, ctx.OperationCenterID, commissionTypeOperationCenterReward, result.OperationCenterRewardCents, "PENDING")
	appendRecord(receiverTypePlatform, "platform", commissionTypePlatformIncome, result.PlatformIncomeCents, "RECORDED")
	return records
}

func rewardSnapshotForSettlement(ctx commissionOrderContext, result commissionSettlementResult, planType string) map[string]any {
	return map[string]any{
		"settlementMode":               "FIXED_COMMERCE_RULE",
		"orderType":                    result.OrderType,
		"businessOrderType":            businessOrderTypeForPlanType(planType),
		"planType":                     normalizePlanTypeString(planType),
		"buyerUserId":                  ctx.BuyerUserID,
		"amountCents":                  ctx.AmountCents,
		"tokenAmount":                  result.TokenGrantAmount,
		"tokenGrantAmount":             result.TokenGrantAmount,
		"tokenGrantValueCents":         result.TokenGrantValueCents,
		"directAgentId":                ctx.DirectAgentID,
		"directAgentRewardCents":       result.DirectAgentRewardCents,
		"parentAgentId":                ctx.ParentAgentID,
		"parentAgentRewardCents":       result.ParentAgentRewardCents,
		"operationCenterId":            ctx.OperationCenterID,
		"operationCenterRewardCents":   result.OperationCenterRewardCents,
		"platformIncomeCents":          result.PlatformIncomeCents,
		"agentWalletPolicy":            "AGENT_COMMISSIONS_ONLY",
		"userWalletPolicy":             "AI_TOKEN_AND_CASH_BALANCE",
		"agentSelfAIUsageBillingScope": "USER_IDENTITY",
	}
}

func businessOrderTypeForPlanType(planType string) string {
	switch normalizePlanTypeString(planType) {
	case planTypeMemberPackage:
		return "USER_PACKAGE"
	case planTypeAgentJoinPackage:
		return "AGENT_JOIN"
	case planTypeTokenRecharge:
		return "TOKEN_RECHARGE"
	case planTypeOperationCenterPackage:
		return "OPERATION_CENTER_JOIN"
	default:
		return normalizeCommerceCode(planType)
	}
}

func businessOrderTypeForPlanID(planID string) string {
	if plan, ok := planCatalogByID(planID); ok {
		return businessOrderTypeForPlanType(planBusinessType(plan))
	}
	return ""
}

func businessOrderTypeFromOrder(order adminOrder) string {
	if value := strings.TrimSpace(order.BusinessOrderType); value != "" {
		return value
	}
	if value := stringValue(order.PriceSnapshot["businessOrderType"]); value != "" {
		return value
	}
	if planType := stringValue(order.PriceSnapshot["planType"]); planType != "" {
		return businessOrderTypeForPlanType(planType)
	}
	if plan, ok := planCatalogByID(order.PlanID); ok {
		return businessOrderTypeForPlanType(planBusinessType(plan))
	}
	return ""
}

func receiverAgentID(receiverType string, receiverID string) string {
	if receiverType == receiverTypeAgent {
		return receiverID
	}
	return ""
}

func settlementStatusForReceiver(receiverType string) string {
	if receiverType == receiverTypePlatform {
		return "SETTLED"
	}
	return "UNSETTLED"
}

func normalizeCommerceCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizePlanTypeString(value string) string {
	value = normalizeCommerceCode(value)
	switch value {
	case "SUBSCRIPTION", "PLAN_ORDER", "USER_MEMBER_PACKAGE":
		return planTypeMemberPackage
	case "RECHARGE", "COMPUTE_RECHARGE", "USER_RECHARGE":
		return planTypeTokenRecharge
	case "AGENT_JOIN":
		return planTypeAgentJoinPackage
	case "OPERATION_CENTER_JOIN":
		return planTypeOperationCenterPackage
	default:
		return value
	}
}

func planBusinessType(plan adminPlan) string {
	if strings.TrimSpace(plan.PlanType) != "" {
		return normalizePlanTypeString(plan.PlanType)
	}
	if plan.Entitlements != nil {
		if value := stringValue(plan.Entitlements["productType"]); value != "" {
			return normalizePlanTypeString(value)
		}
		if value := stringValue(plan.Entitlements["planType"]); value != "" {
			return normalizePlanTypeString(value)
		}
	}
	return ""
}

func planTokenGrantAmount(plan adminPlan) int {
	if rightsValueCents := planTokenRightsValueCents(plan); rightsValueCents > 0 && commerceTokenGrantFollowsRightsValue(plan) {
		return rightsValueCents
	}
	if plan.TokenAmount > 0 {
		return plan.TokenAmount
	}
	if plan.Entitlements != nil {
		if amount := intValue(plan.Entitlements["tokenGrantAmount"]); amount > 0 {
			return amount
		}
	}
	return planPoints(plan)
}

func commerceTokenGrantFollowsRightsValue(plan adminPlan) bool {
	switch planBusinessType(plan) {
	case planTypeMemberPackage, planTypeAgentJoinPackage:
		return true
	default:
		return false
	}
}

func planEntitlementInt(plan adminPlan, key string) (int, bool) {
	if plan.Entitlements == nil {
		return 0, false
	}
	value, ok := plan.Entitlements[key]
	if !ok {
		return 0, false
	}
	return intValue(value), true
}

func planTokenRightsValueCents(plan adminPlan) int {
	if amount, ok := planEntitlementInt(plan, "tokenRightsValueCents"); ok {
		if amount >= 0 {
			return amount
		}
	}
	switch planBusinessType(plan) {
	case planTypeMemberPackage:
		if plan.ID == "plan_ai_creator_996" {
			return 40000
		}
	case planTypeAgentJoinPackage:
		return 20000
	case planTypeOperationCenterPackage:
		return 0
	}
	return 0
}

func planMemberLevel(plan adminPlan) string {
	if strings.TrimSpace(plan.MemberLevel) != "" {
		return normalizeCommerceCode(plan.MemberLevel)
	}
	if plan.Entitlements != nil {
		if level := stringValue(plan.Entitlements["memberLevel"]); level != "" {
			return normalizeCommerceCode(level)
		}
	}
	return memberLevelFree
}
