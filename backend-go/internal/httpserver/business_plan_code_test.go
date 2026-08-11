package httpserver

import (
	"path/filepath"
	"testing"
)

func TestValidateNewBusinessPlanCodeSeparatesPriceSemanticsFromNumericIdentifiers(t *testing.T) {
	domain := businessPlanDomainService{}
	for _, code := range []string{"plan_member_v10", "plan_2026", "agent_level_02", "plan_member", "plan_agent"} {
		if err := domain.ValidateNewCode(code); err != nil {
			t.Errorf("valid code %q was rejected: %v", code, err)
		}
	}

	for _, code := range []string{
		"plan_member_996",
		"plan_agent_199",
		"member_1yuan",
		"agent_price_996",
		"plan_member_rmb_996",
		"plan_agent_amount_199",
		"plan_member_yuan_offer",
	} {
		if err := domain.ValidateNewCode(code); err == nil {
			t.Errorf("price-semantic code %q was accepted", code)
		}
	}
}

func TestLegacyPlanWithoutV2ProjectionRemainsOnV1CompatibilityPath(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	updated, err := store.UpdateAdminPlan("plan_ai_creator_996", adminPlanMutation{Name: "legacy compatible"})
	if err != nil {
		t.Fatalf("legacy V1 plan was incorrectly treated as V2 managed: %v", err)
	}
	if updated.Name != "legacy compatible" {
		t.Fatalf("legacy V1 update did not persist: %+v", updated)
	}
}
