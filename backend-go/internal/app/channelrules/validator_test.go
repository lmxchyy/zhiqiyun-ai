package channelrules

import (
	"strings"
	"testing"
	"time"

	commissionapp "xianzhi-ai/backend-go/internal/app/commission"
)

func TestValidateRuleSetRejectsAncestorAgentCommission(t *testing.T) {
	now := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	rules := defaultRules(now, ScenarioMemberPurchase)
	rules[0].RelationshipLevel = 2

	err := ValidateRuleSet(RuleSetValidationInput{
		RuleSet: CommercialRuleSet{ID: "rule_set_v132", TenantID: "tenant_default", Code: "CHANNEL_ECOSYSTEM_V132", Version: 1, Status: RuleSetDraft},
		Plans: []PlanConfigVersion{{ID: "plan_version_member", TenantID: "tenant_default", RuleSetID: "rule_set_v132", PlanID: "plan_ai_creator_996", PriceCents: 99600, TokenRightsValueCents: 40000, TokenGrantAmount: 40000, IdentityType: "MEMBER"}},
		CommissionRules: rules,
	})
	if err == nil || !strings.Contains(err.Error(), ErrCodeAncestorCommissionForbidden) {
		t.Fatalf("expected %s, got %v", ErrCodeAncestorCommissionForbidden, err)
	}
}

func TestCommissionEngineAdapterCalculatesMemberScenarioWithTokenPreallocation(t *testing.T) {
	calculation := calculateDefaultScenario(t, ScenarioMemberPurchase, 40000)

	if calculation.TokenRightsValueCents != 40000 {
		t.Fatalf("token rights = %d, want 40000", calculation.TokenRightsValueCents)
	}
	if calculation.DirectAgentAmountCents != 30000 {
		t.Fatalf("direct agent = %d, want 30000", calculation.DirectAgentAmountCents)
	}
	if calculation.OperationCenterAmountCents != 20000 {
		t.Fatalf("operation center = %d, want 20000", calculation.OperationCenterAmountCents)
	}
	if calculation.PlatformAmountCents != 9600 {
		t.Fatalf("platform = %d, want 9600", calculation.PlatformAmountCents)
	}
	assertAmountConservation(t, calculation, 99600)
}

func TestCommissionEngineAdapterCalculatesAgentScenarioWithTokenPreallocation(t *testing.T) {
	calculation := calculateDefaultScenario(t, ScenarioAgentJoin, 20000)

	if calculation.TokenRightsValueCents != 20000 {
		t.Fatalf("token rights = %d, want 20000", calculation.TokenRightsValueCents)
	}
	if calculation.DirectAgentAmountCents != 30000 {
		t.Fatalf("direct agent = %d, want 30000", calculation.DirectAgentAmountCents)
	}
	if calculation.OperationCenterAmountCents != 20000 {
		t.Fatalf("operation center = %d, want 20000", calculation.OperationCenterAmountCents)
	}
	if calculation.PlatformAmountCents != 29600 {
		t.Fatalf("platform = %d, want 29600", calculation.PlatformAmountCents)
	}
	assertAmountConservation(t, calculation, 99600)
}

func calculateDefaultScenario(t *testing.T, scenario ScenarioCode, tokenRights int64) OrderCalculation {
	t.Helper()
	now := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	planID := "plan_ai_creator_996"
	identityType := "MEMBER"
	if scenario == ScenarioAgentJoin {
		planID = "plan_agent_join_996"
		identityType = "AGENT"
	}
	resolved := ResolvedOrderRules{
		RuleSet: CommercialRuleSet{ID: "rule_set_v132", TenantID: "tenant_default", Code: "CHANNEL_ECOSYSTEM_V132", Version: 1, Status: RuleSetPublished},
		Plan: PlanConfigVersion{ID: "plan_version", TenantID: "tenant_default", RuleSetID: "rule_set_v132", PlanID: planID, PriceCents: 99600, TokenRightsValueCents: tokenRights, TokenGrantAmount: tokenRights, IdentityType: identityType},
		Scenario: scenario,
		Relationship: RelationshipSnapshot{SourceUserID: "user_buyer", DirectAgentID: "agent_direct", OperationCenterID: "operation_center_direct", EffectiveAt: now},
		CommissionRules: defaultRules(now, scenario),
	}
	request := ResolveOrderRequest{TenantID: "tenant_default", OrderID: "order_1", OrderNo: "ORDER-1", PlanID: planID, SourceUserID: "user_buyer", PaidAmountCents: 99600, BusinessTime: now}

	calculation, err := NewCommissionEngineAdapter(commissionapp.NewEngine()).Calculate(request, resolved)
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	return calculation
}

func defaultRules(now time.Time, scenario ScenarioCode) []commissionapp.CommissionRule {
	productType := string(scenario)
	return []commissionapp.CommissionRule{
		{ID: "rule_agent", TenantID: "tenant_default", Code: "DIRECT_AGENT", Name: "direct agent", ProductType: productType, BeneficiaryRole: commissionapp.BeneficiaryAgent, RelationshipLevel: 1, CalculationType: commissionapp.CalculationFixedAmount, FixedAmountCents: 30000, Priority: 10, FreezeDays: 7, RefundPolicy: "PRO_RATA", EffectiveStartAt: now.Add(-time.Hour), Version: 1, Status: "ACTIVE"},
		{ID: "rule_operation_center", TenantID: "tenant_default", Code: "OPERATION_CENTER", Name: "operation center", ProductType: productType, BeneficiaryRole: commissionapp.BeneficiaryOperationCenter, RelationshipLevel: 1, CalculationType: commissionapp.CalculationFixedAmount, FixedAmountCents: 20000, Priority: 20, FreezeDays: 7, RefundPolicy: "PRO_RATA", EffectiveStartAt: now.Add(-time.Hour), Version: 1, Status: "ACTIVE"},
		{ID: "rule_platform", TenantID: "tenant_default", Code: "PLATFORM", Name: "platform", ProductType: productType, BeneficiaryRole: commissionapp.BeneficiaryPlatform, RelationshipLevel: 0, CalculationType: commissionapp.CalculationRemainderToPlatform, Priority: 1000, EffectiveStartAt: now.Add(-time.Hour), Version: 1, Status: "ACTIVE"},
	}
}

func assertAmountConservation(t *testing.T, calculation OrderCalculation, paidAmount int64) {
	t.Helper()
	total := calculation.TokenRightsValueCents + calculation.DirectAgentAmountCents + calculation.OperationCenterAmountCents + calculation.PlatformAmountCents
	if total != paidAmount {
		t.Fatalf("allocation total = %d, want paid amount %d", total, paidAmount)
	}
}
