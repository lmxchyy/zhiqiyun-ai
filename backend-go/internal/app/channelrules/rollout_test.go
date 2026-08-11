package channelrules

import "testing"

func TestRolloutConfigRequiresPinnedRuleVersion(t *testing.T) {
	config := RolloutConfig{TenantID: "tenant_default", Mode: RolloutModeShadow, Enabled: true}
	if err := config.Validate(); err == nil {
		t.Fatal("shadow rollout must require an exact pinned rule set and version")
	}
}

func TestEvaluateRolloutCanaryIsStableAndDoesNotEnableUnselectedOrders(t *testing.T) {
	config := RolloutConfig{
		TenantID: "tenant_default", Mode: RolloutModeV132Canary, Enabled: true,
		PinnedRuleSetID: "rules-v1", PinnedRuleSetVersion: 1,
		CanaryBasisPoints: 500, PercentageRolloutEnabled: true, RealSwitchEnabled: true,
	}
	first, err := EvaluateRollout(config, RolloutSubject{OrderID: "order-stable", UserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := EvaluateRollout(config, RolloutSubject{OrderID: "order-stable", UserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("canary decision drifted: first=%+v second=%+v", first, second)
	}
	if !first.CalculateShadow {
		t.Fatal("canary mode must retain shadow calculation")
	}
}

func TestRolloutModesUseConfirmedNames(t *testing.T) {
	if RolloutModeLegacy != "LEGACY" || RolloutModeShadow != "SHADOW" || RolloutModeV132Canary != "CANARY" || RolloutModeV132Full != "V132" {
		t.Fatalf("unexpected rollout mode names: %s %s %s %s", RolloutModeLegacy, RolloutModeShadow, RolloutModeV132Canary, RolloutModeV132Full)
	}
}

func TestCanaryWhitelistSupportsAccountPlanAndOrderWithoutGlobalPercentage(t *testing.T) {
	config := RolloutConfig{
		TenantID: "tenant_default", Mode: RolloutModeV132Canary, Enabled: true,
		PinnedRuleSetID: "rules-v1", PinnedRuleSetVersion: 1, RealSwitchEnabled: true,
		AllowUserIDs: []string{"user-allowed"}, AllowPlanIDs: []string{"plan-allowed"}, AllowOrderIDs: []string{"order-allowed"},
	}
	for _, subject := range []RolloutSubject{
		{OrderID: "order-1", UserID: "user-allowed", PlanID: "other"},
		{OrderID: "order-2", UserID: "other", PlanID: "plan-allowed"},
		{OrderID: "order-allowed", UserID: "other", PlanID: "other"},
	} {
		decision, err := EvaluateRollout(config, subject)
		if err != nil || !decision.UseV132Settlement {
			t.Fatalf("whitelist subject %+v was not selected: %+v %v", subject, decision, err)
		}
	}
	decision, err := EvaluateRollout(config, RolloutSubject{OrderID: "order-none", UserID: "user-none", PlanID: "plan-none"})
	if err != nil || decision.UseV132Settlement {
		t.Fatalf("non-whitelisted subject selected: %+v %v", decision, err)
	}
}

func TestRolloutBucketUsesOnlyTenantAndOrder(t *testing.T) {
	config := RolloutConfig{
		TenantID: "tenant_default", Mode: RolloutModeV132Canary, Enabled: true,
		PinnedRuleSetID: "rules-v1", PinnedRuleSetVersion: 1,
		CanaryBasisPoints: 5000, PercentageRolloutEnabled: true, RealSwitchEnabled: true,
	}
	first, err := EvaluateRollout(config, RolloutSubject{OrderID: "order-stable", UserID: "user-a", PlanID: "plan-a"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := EvaluateRollout(config, RolloutSubject{OrderID: "order-stable", UserID: "user-b", PlanID: "plan-b"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Bucket != second.Bucket {
		t.Fatalf("bucket must depend only on tenant and order: %d != %d", first.Bucket, second.Bucket)
	}
}

func TestEvaluateRolloutShadowNeverUsesV132Settlement(t *testing.T) {
	decision, err := EvaluateRollout(RolloutConfig{
		TenantID: "tenant_default", Mode: RolloutModeShadow, Enabled: true,
		PinnedRuleSetID: "rules-v1", PinnedRuleSetVersion: 1,
	}, RolloutSubject{OrderID: "order-1", UserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.CalculateShadow || decision.UseV132Settlement {
		t.Fatalf("unexpected shadow decision: %+v", decision)
	}
}
