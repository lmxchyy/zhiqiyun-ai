package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminCommissionRulePermissions(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodGet, "/api/v1/admin/commission-rules", "finance:commission-rule:view"},
		{http.MethodPost, "/api/v1/admin/commission-rules", "finance:commission-rule:manage"},
		{http.MethodPut, "/api/v1/admin/commission-rules/rule-1", "finance:commission-rule:manage"},
	}
	for _, test := range tests {
		req := httptest.NewRequest(test.method, test.path, nil)
		if got := adminPermissionForRequest(req); got != test.want {
			t.Fatalf("%s %s permission = %s, want %s", test.method, test.path, got, test.want)
		}
	}
}

func TestCommissionRuleMutationUsesIntegerCentsAndBasisPoints(t *testing.T) {
	rule, err := commissionRuleFromMutation(commissionRuleMutation{
		RuleCode: "MEMBER_PERCENT", RuleName: "会员分润", ProductType: "MEMBER_PACKAGE",
		ProductID: "plan_ai_creator_996", BeneficiaryRole: "AGENT", RelationshipLevel: 1,
		CalculationType: "PAID_AMOUNT_PERCENTAGE", PercentageBPS: 1250,
		Priority: 10, FreezeDays: 7, EffectiveStartAt: "2026-01-01T00:00:00Z", Status: "ACTIVE",
	}, "tenant_default")
	if err != nil {
		t.Fatal(err)
	}
	if rule.PercentageBPS != 1250 || rule.FixedAmountCents != 0 {
		t.Fatalf("unexpected rule money model: %+v", rule)
	}
}
