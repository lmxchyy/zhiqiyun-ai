package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	channelrules "xianzhi-ai/backend-go/internal/app/channelrules"
	commissionapp "xianzhi-ai/backend-go/internal/app/commission"
)

type pinnedRuleBundleStore struct {
	bundle channelrules.RuleBundle
}

func (s pinnedRuleBundleStore) LoadEffectiveRuleBundle(context.Context, channelrules.RuleBundleQuery) (channelrules.RuleBundle, error) {
	return channelrules.RuleBundle{}, errors.New("pinned V1.3.2 store does not load dynamic rules")
}

func (s pinnedRuleBundleStore) ResolveRelationshipSnapshot(context.Context, channelrules.RelationshipQuery) (channelrules.RelationshipSnapshot, error) {
	return channelrules.RelationshipSnapshot{}, errors.New("pinned V1.3.2 relationship is supplied by the order context")
}

func (s pinnedRuleBundleStore) SaveOrderRuleSnapshot(context.Context, channelrules.OrderRuleSnapshot) error {
	return errors.New("pinned V1.3.2 calculation cannot replace the immutable decision snapshot")
}

func (s pinnedRuleBundleStore) LoadShadowRuleBundle(context.Context, channelrules.RuleBundleQuery) (channelrules.RuleBundle, error) {
	return s.bundle, nil
}

func generateV132CommissionRecordsForCommerceOrderTx(ctx context.Context, tx *sql.Tx, order *adminOrder, plan adminPlan, commerceCtx commissionOrderContext, decision orderSettlementDecision) (commissionapp.CalculationResult, error) {
	if order == nil {
		return commissionapp.CalculationResult{}, errors.New("V1.3.2 order is required")
	}
	businessTime, err := time.Parse(time.RFC3339Nano, nowForOrder(*order))
	if err != nil {
		return commissionapp.CalculationResult{}, err
	}
	transactionStore, err := channelrules.NewTransactionStore(tx)
	if err != nil {
		return commissionapp.CalculationResult{}, err
	}
	request := channelrules.ResolveOrderRequest{
		TenantID: decision.TenantID, OrderID: order.ID, OrderNo: firstNonEmptyString(order.OrderNo, order.ID),
		PlanID: plan.ID, SourceUserID: firstNonEmptyString(commerceCtx.BuyerUserID, order.UserID),
		PaidAmountCents: int64(commerceCtx.AmountCents), BusinessTime: businessTime,
	}
	bundle, err := transactionStore.LoadPinnedRuleBundle(ctx, channelrules.RuleBundleQuery{
		TenantID: request.TenantID, PlanID: request.PlanID, BusinessTime: request.BusinessTime,
	}, decision.RuleSetID, decision.RuleSetVersion)
	if err != nil {
		return commissionapp.CalculationResult{}, err
	}
	if bundle.RuleSet.Status != channelrules.RuleSetPublished {
		return commissionapp.CalculationResult{}, errors.New("V1.3.2 real settlement requires a published rule set")
	}
	directAgentID := commerceCtx.DirectAgentID
	if directAgentID != "" {
		eligible, eligibilityErr := commissionBeneficiaryEligibleTx(ctx, tx, "AGENT", directAgentID)
		if eligibilityErr != nil {
			return commissionapp.CalculationResult{}, eligibilityErr
		}
		if !eligible {
			directAgentID = ""
		}
	}
	operationCenterID := commerceCtx.OperationCenterID
	if operationCenterID != "" {
		eligible, eligibilityErr := commissionBeneficiaryEligibleTx(ctx, tx, "OPERATION_CENTER", operationCenterID)
		if eligibilityErr != nil {
			return commissionapp.CalculationResult{}, eligibilityErr
		}
		if !eligible {
			operationCenterID = ""
		}
	}
	relationship := channelrules.RelationshipSnapshot{
		SourceUserID: request.SourceUserID, DirectAgentID: directAgentID,
		OperationCenterID: operationCenterID, EffectiveAt: businessTime,
		SourceType: "ORDER_SETTLEMENT_DECISION", SourceID: order.ID,
	}
	resolved, err := channelrules.NewChannelRuleService(pinnedRuleBundleStore{bundle: bundle}).ResolveShadowOrder(ctx, request, relationship)
	if err != nil {
		return commissionapp.CalculationResult{}, err
	}
	calculation, err := channelrules.NewCommissionEngineAdapter(commissionapp.NewEngine()).Calculate(request, resolved)
	if err != nil {
		return commissionapp.CalculationResult{}, err
	}
	if err := validateV132SettlementConservation(
		request.PaidAmountCents, calculation.TokenRightsValueCents,
		calculation.DirectAgentAmountCents, calculation.OperationCenterAmountCents,
		calculation.PlatformAmountCents,
	); err != nil {
		return commissionapp.CalculationResult{}, err
	}

	rulesByID := make(map[string]commissionapp.CommissionRule, len(resolved.CommissionRules))
	for _, rule := range resolved.CommissionRules {
		rulesByID[rule.ID] = rule
	}
	result := commissionapp.CalculationResult{
		CashCommissionCents: commissionapp.AmountCents(calculation.DirectAgentAmountCents + calculation.OperationCenterAmountCents),
		PlatformIncomeCents: commissionapp.AmountCents(calculation.PlatformAmountCents),
	}
	for _, allocation := range calculation.Allocations {
		if allocation.AmountCents <= 0 {
			continue
		}
		rule, ok := rulesByID[allocation.RuleID]
		if !ok {
			return commissionapp.CalculationResult{}, fmt.Errorf("V1.3.2 allocation references unknown rule %s", allocation.RuleID)
		}
		status := commissionapp.CommissionAvailable
		availableAt := businessTime
		var freezeUntil *time.Time
		if rule.FreezeDays > 0 {
			status = commissionapp.CommissionFrozen
			value := businessTime.AddDate(0, 0, rule.FreezeDays)
			freezeUntil = &value
			availableAt = value
		}
		key := fmt.Sprintf("commission:v132:%s:%s:%s", order.ID, allocation.RuleID, allocation.BeneficiaryID)
		record := commissionapp.CommissionRecord{
			ID: "commission_v132_" + shortID(key), TenantID: decision.TenantID,
			OrderID: order.ID, OrderNo: request.OrderNo,
			BeneficiaryType: commissionapp.BeneficiaryType(allocation.BeneficiaryType), BeneficiaryID: allocation.BeneficiaryID,
			SourceUserID: request.SourceUserID, RuleID: allocation.RuleID, RuleVersion: allocation.RuleVersion,
			AmountCents: commissionapp.AmountCents(allocation.AmountCents), Currency: "CNY",
			RecordType: commissionapp.RecordEarning, Status: status, FreezeUntil: freezeUntil,
			AvailableAt: &availableAt, IdempotencyKey: key, CreatedAt: businessTime, UpdatedAt: businessTime,
		}
		if err := insertImmutableCommissionRecordTx(ctx, tx, record, rule, plan); err != nil {
			return commissionapp.CalculationResult{}, err
		}
		if err := postV132CommissionRecordToWalletTx(ctx, tx, record); err != nil {
			return commissionapp.CalculationResult{}, err
		}
		result.Records = append(result.Records, record)
	}
	if order.PriceSnapshot == nil {
		order.PriceSnapshot = map[string]any{}
	}
	order.PriceSnapshot["v132RuleSetId"] = resolved.RuleSet.ID
	order.PriceSnapshot["v132RuleSetVersion"] = resolved.RuleSet.Version
	order.PriceSnapshot["v132PlanVersionId"] = resolved.Plan.ID
	order.PriceSnapshot["v132Calculation"] = calculation
	return result, nil
}
