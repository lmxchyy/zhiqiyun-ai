package httpserver

import "testing"

func TestTrialPlanGrantsTenPoints(t *testing.T) {
	plan, ok := planCatalogByID("plan_free")
	if !ok {
		t.Fatal("trial plan not found")
	}
	if got := planPoints(plan); got != 10 {
		t.Fatalf("trial plan points = %d, want 10", got)
	}
	if got := planGrantPoints(plan); got != 10 {
		t.Fatalf("trial plan grant points = %d, want 10", got)
	}
}

func TestMineRechargePackagesMatchProductContract(t *testing.T) {
	tests := []struct {
		id          string
		amountCents int
		points      int
	}{
		{id: "recharge_100", amountCents: 10000, points: 10000},
		{id: "recharge_400", amountCents: 40000, points: 40000},
	}

	for _, test := range tests {
		plan, ok := planCatalogByID(test.id)
		if !ok {
			t.Fatalf("plan %s not found", test.id)
		}
		if got := planPrice(plan); got != test.amountCents {
			t.Fatalf("plan %s price = %d, want %d", test.id, got, test.amountCents)
		}
		if got := planPoints(plan); got != test.points {
			t.Fatalf("plan %s points = %d, want %d", test.id, got, test.points)
		}
		if got := rechargePackageIDForAmount(test.amountCents); got != test.id {
			t.Fatalf("amount %d maps to %s, want %s", test.amountCents, got, test.id)
		}
	}
}

func TestOneCentVirtualPaymentPlanIsGrantable(t *testing.T) {
	plan, ok := planCatalogByID("recharge_test_1fen")
	if !ok {
		t.Fatal("one-cent virtual payment plan not found")
	}
	if got := planPrice(plan); got != 1 {
		t.Fatalf("one-cent plan price = %d, want 1", got)
	}
	if got := planTokenGrantAmount(plan); got != 1 {
		t.Fatalf("one-cent plan token grant = %d, want 1", got)
	}
	if got := planBusinessType(plan); got != planTypeTokenRecharge {
		t.Fatalf("one-cent plan type = %s, want %s", got, planTypeTokenRecharge)
	}
	if productType := stringValue(plan.Entitlements["productType"]); productType != "TOKEN_ONLY" {
		t.Fatalf("one-cent product type = %s, want TOKEN_ONLY", productType)
	}
}

func TestMergeCanonicalPlansPreservesConfiguredValues(t *testing.T) {
	existing := []adminPlan{{
		ID: "plan_free", Code: "trial", Name: "自定义新人体验", PriceCents: 0,
		GrantPoints: 588, DurationDays: 21, Concurrency: 3, Active: true,
	}}

	merged := mergeCanonicalPlans(existing)
	plan := configuredNewcomerPlan(merged)
	if plan.Name != existing[0].Name || planPoints(plan) != 588 || plan.DurationDays != 21 || plan.Concurrency != 3 {
		t.Fatalf("configured plan was overwritten: %+v", plan)
	}
	if len(merged) <= len(existing) {
		t.Fatalf("missing canonical plans were not appended: got %d plans", len(merged))
	}
}
