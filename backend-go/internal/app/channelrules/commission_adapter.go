package channelrules

import (
	"fmt"
	"strings"

	commissionapp "xianzhi-ai/backend-go/internal/app/commission"
)

type CommissionEngineAdapter struct {
	engine commissionapp.Engine
}

func NewCommissionEngineAdapter(engine commissionapp.Engine) CommissionEngineAdapter {
	return CommissionEngineAdapter{engine: engine}
}

func (a CommissionEngineAdapter) Calculate(request ResolveOrderRequest, resolved ResolvedOrderRules) (OrderCalculation, error) {
	if strings.TrimSpace(request.TenantID) == "" || strings.TrimSpace(request.OrderID) == "" || strings.TrimSpace(request.OrderNo) == "" || strings.TrimSpace(request.PlanID) == "" || strings.TrimSpace(request.SourceUserID) == "" || request.BusinessTime.IsZero() {
		return OrderCalculation{}, fmt.Errorf("%s: complete order context is required", ErrCodeRuleValidationFailed)
	}
	if request.PaidAmountCents <= 0 || resolved.Plan.PriceCents <= 0 || request.PaidAmountCents > resolved.Plan.PriceCents {
		return OrderCalculation{}, fmt.Errorf("%s: invalid paid amount", ErrCodeRuleValidationFailed)
	}
	if resolved.RuleSet.TenantID != request.TenantID || resolved.Plan.TenantID != request.TenantID || resolved.Plan.RuleSetID != resolved.RuleSet.ID || resolved.Plan.PlanID != request.PlanID {
		return OrderCalculation{}, fmt.Errorf("%s: resolved rule, plan and order context do not match", ErrCodeRuleValidationFailed)
	}
	for _, rule := range resolved.CommissionRules {
		if rule.BeneficiaryRole == commissionapp.BeneficiaryAgent && rule.RelationshipLevel != 1 {
			return OrderCalculation{}, fmt.Errorf("%s: agent rule %s must target relationship level 1", ErrCodeAncestorCommissionForbidden, rule.ID)
		}
	}

	distributableCents := request.PaidAmountCents - resolved.Plan.TokenRightsValueCents
	if distributableCents <= 0 {
		return OrderCalculation{}, fmt.Errorf("%s: token rights leave no distributable cash", ErrCodeRuleValidationFailed)
	}
	relationships := commissionapp.RelationshipSnapshot{
		AgentIDsByLevel:   map[int]string{},
		OperationCenterID: strings.TrimSpace(resolved.Relationship.OperationCenterID),
		PlatformID:        "platform:" + request.TenantID,
	}
	if directAgentID := strings.TrimSpace(resolved.Relationship.DirectAgentID); directAgentID != "" {
		relationships.AgentIDsByLevel[1] = directAgentID
	}

	result, err := a.engine.Calculate(commissionapp.CalculationInput{
		TenantID:         request.TenantID,
		OrderID:          request.OrderID,
		OrderNo:          request.OrderNo,
		ProductType:      string(resolved.Scenario),
		ProductID:        request.PlanID,
		SourceUserID:     request.SourceUserID,
		OrderAmountCents: commissionapp.AmountCents(distributableCents),
		PaidAmountCents:  commissionapp.AmountCents(distributableCents),
		Quantity:         1,
		PaidAt:           request.BusinessTime,
		Relationships:    relationships,
		Rules:            resolved.CommissionRules,
	})
	if err != nil {
		return OrderCalculation{}, err
	}

	calculation := OrderCalculation{
		RuleSetID:              resolved.RuleSet.ID,
		RuleSetVersion:         resolved.RuleSet.Version,
		Scenario:               resolved.Scenario,
		TokenRightsValueCents:  resolved.Plan.TokenRightsValueCents,
		TokenGrantAmount:       resolved.Plan.TokenGrantAmount,
		Allocations:            make([]Allocation, 0, len(result.Records)),
	}
	for _, record := range result.Records {
		amount := int64(record.AmountCents)
		calculation.Allocations = append(calculation.Allocations, Allocation{
			BusinessType:     "ORDER_COMMISSION",
			BeneficiaryType: string(record.BeneficiaryType),
			BeneficiaryID:   record.BeneficiaryID,
			AmountCents:     amount,
			RuleID:          record.RuleID,
			RuleVersion:     record.RuleVersion,
		})
		switch record.BeneficiaryType {
		case commissionapp.BeneficiaryAgent:
			calculation.DirectAgentAmountCents += amount
		case commissionapp.BeneficiaryOperationCenter:
			calculation.OperationCenterAmountCents += amount
		case commissionapp.BeneficiaryPlatform:
			calculation.PlatformAmountCents += amount
		default:
			return OrderCalculation{}, fmt.Errorf("%s: unsupported calculation beneficiary %s", ErrCodeRuleValidationFailed, record.BeneficiaryType)
		}
	}
	total := calculation.TokenRightsValueCents + calculation.DirectAgentAmountCents + calculation.OperationCenterAmountCents + calculation.PlatformAmountCents
	if total != request.PaidAmountCents {
		return OrderCalculation{}, fmt.Errorf("%s: allocation total %d does not match paid amount %d", ErrCodeRuleValidationFailed, total, request.PaidAmountCents)
	}
	return calculation, nil
}
