package channelrules

import (
	"errors"
	"fmt"
	"strings"

	commissionapp "xianzhi-ai/backend-go/internal/app/commission"
)

const (
	ErrCodeAncestorCommissionForbidden = "CHANNEL_ANCESTOR_COMMISSION_FORBIDDEN"
	ErrCodeRuleValidationFailed        = "CHANNEL_RULE_VALIDATION_FAILED"
)

func ValidateRuleSet(input RuleSetValidationInput) error {
	if strings.TrimSpace(input.RuleSet.ID) == "" || strings.TrimSpace(input.RuleSet.TenantID) == "" || strings.TrimSpace(input.RuleSet.Code) == "" || input.RuleSet.Version <= 0 {
		return fmt.Errorf("%s: rule set identity, tenant, code and positive version are required", ErrCodeRuleValidationFailed)
	}
	if len(input.Plans) == 0 {
		return fmt.Errorf("%s: at least one plan configuration is required", ErrCodeRuleValidationFailed)
	}
	if len(input.CommissionRules) == 0 {
		return fmt.Errorf("%s: at least one commission rule is required", ErrCodeRuleValidationFailed)
	}

	plansByScenario := make(map[ScenarioCode]PlanConfigVersion, len(input.Plans))
	for _, plan := range input.Plans {
		if err := validatePlan(input.RuleSet, plan); err != nil {
			return err
		}
		scenario, err := scenarioForIdentity(plan.IdentityType)
		if err != nil {
			return err
		}
		plansByScenario[scenario] = plan
	}

	type totals struct {
		fixedAmount int64
		remainders  int
	}
	totalsByScenario := make(map[ScenarioCode]*totals)
	for _, rule := range input.CommissionRules {
		if rule.BeneficiaryRole == commissionapp.BeneficiaryAgent && rule.RelationshipLevel != 1 {
			return fmt.Errorf("%s: agent rule %s must target relationship level 1", ErrCodeAncestorCommissionForbidden, rule.ID)
		}
		if rule.BeneficiaryRole == commissionapp.BeneficiaryOperationCenter && rule.RelationshipLevel != 1 {
			return fmt.Errorf("%s: operation center rule %s must be resolved from the direct agent", ErrCodeRuleValidationFailed, rule.ID)
		}
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("%s: invalid commission rule %s: %w", ErrCodeRuleValidationFailed, rule.ID, err)
		}
		scenario := ScenarioCode(strings.ToUpper(strings.TrimSpace(rule.ProductType)))
		if _, ok := plansByScenario[scenario]; !ok {
			return fmt.Errorf("%s: no plan configuration for scenario %s", ErrCodeRuleValidationFailed, scenario)
		}
		group := totalsByScenario[scenario]
		if group == nil {
			group = &totals{}
			totalsByScenario[scenario] = group
		}
		switch rule.CalculationType {
		case commissionapp.CalculationFixedAmount:
			group.fixedAmount += int64(rule.FixedAmountCents)
		case commissionapp.CalculationRemainderToPlatform:
			if rule.BeneficiaryRole != commissionapp.BeneficiaryPlatform {
				return fmt.Errorf("%s: remainder rule must target platform", ErrCodeRuleValidationFailed)
			}
			group.remainders++
		}
	}

	for scenario, plan := range plansByScenario {
		group := totalsByScenario[scenario]
		if group == nil || group.remainders != 1 {
			return fmt.Errorf("%s: scenario %s requires exactly one platform remainder rule", ErrCodeRuleValidationFailed, scenario)
		}
		if plan.TokenRightsValueCents+group.fixedAmount > plan.PriceCents {
			return fmt.Errorf("%s: scenario %s allocations exceed plan price", ErrCodeRuleValidationFailed, scenario)
		}
	}
	return nil
}

func ValidateReferralRewardRules(ruleSet CommercialRuleSet, rules []ReferralRewardRule) error {
	if len(rules) == 0 {
		return fmt.Errorf("%s: at least one referral reward rule is required", ErrCodeRuleValidationFailed)
	}
	seen := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		code := strings.ToUpper(strings.TrimSpace(rule.Code))
		referrerType := strings.ToUpper(strings.TrimSpace(rule.ReferrerType))
		beneficiaryType := strings.ToUpper(strings.TrimSpace(rule.BeneficiaryType))
		beneficiaryRelation := strings.ToUpper(strings.TrimSpace(rule.BeneficiaryRelation))
		if strings.TrimSpace(rule.ID) == "" || code == "" || rule.Version <= 0 || rule.TenantID != ruleSet.TenantID || rule.RuleSetID != ruleSet.ID || rule.AmountCents <= 0 || rule.FreezeDays < 0 {
			return fmt.Errorf("%s: invalid referral reward rule %s", ErrCodeRuleValidationFailed, rule.ID)
		}
		if _, exists := seen[code]; exists {
			return fmt.Errorf("%s: duplicate referral reward rule code %s", ErrCodeRuleValidationFailed, code)
		}
		seen[code] = struct{}{}
		valid := (referrerType == "OPERATION_CENTER" && beneficiaryType == "OPERATION_CENTER" && beneficiaryRelation == "REFERRER") ||
			(referrerType == "AGENT" && beneficiaryType == "AGENT" && beneficiaryRelation == "REFERRER") ||
			(referrerType == "AGENT" && beneficiaryType == "OPERATION_CENTER" && beneficiaryRelation == "REFERRER_OPERATION_CENTER")
		if !valid {
			return fmt.Errorf("%s: unsupported referral reward relationship for %s", ErrCodeRuleValidationFailed, code)
		}
	}
	return nil
}

func validatePlan(ruleSet CommercialRuleSet, plan PlanConfigVersion) error {
	if strings.TrimSpace(plan.ID) == "" || strings.TrimSpace(plan.PlanID) == "" || plan.RuleSetID != ruleSet.ID || plan.TenantID != ruleSet.TenantID {
		return fmt.Errorf("%s: plan identity, tenant and rule set must match", ErrCodeRuleValidationFailed)
	}
	if plan.PriceCents <= 0 || plan.TokenRightsValueCents < 0 || plan.TokenGrantAmount < 0 || plan.TokenRightsValueCents > plan.PriceCents || plan.DurationDays < 0 {
		return fmt.Errorf("%s: invalid plan amounts or duration", ErrCodeRuleValidationFailed)
	}
	return nil
}

func scenarioForIdentity(identityType string) (ScenarioCode, error) {
	switch strings.ToUpper(strings.TrimSpace(identityType)) {
	case "MEMBER":
		return ScenarioMemberPurchase, nil
	case "AGENT":
		return ScenarioAgentJoin, nil
	case "OPERATION_CENTER":
		return ScenarioOCService, nil
	default:
		return "", errors.New(ErrCodeRuleValidationFailed + ": unsupported plan identity type")
	}
}
