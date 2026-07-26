package channelrules

import "testing"

func TestCanaryWhitelistNeverIncludesOperationCenterPackage(t *testing.T) {
	config := RolloutConfig{
		TenantID: "tenant_default", ConfigVersion: 1, Mode: RolloutModeV132Canary, Enabled: true,
		PinnedRuleSetID: "rules_v132", PinnedRuleSetVersion: 1,
		AllowOrderIDs: []string{"order_operation_center"}, RealSwitchEnabled: true,
	}
	decision, err := EvaluateRollout(config, RolloutSubject{
		OrderID: "order_operation_center", UserID: "user_1", PlanID: "operation_center_package",
		OperationCenterPackage: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.UseV132Settlement || decision.Reason != "OPERATION_CENTER_PACKAGE_EXCLUDED" {
		t.Fatalf("operation center package must remain Legacy: %+v", decision)
	}
}

func TestCanaryPackageWhitelistAndNonWhitelist(t *testing.T) {
	config := RolloutConfig{
		TenantID: "tenant_default", ConfigVersion: 1, Mode: RolloutModeV132Canary, Enabled: true,
		PinnedRuleSetID: "rules_v132", PinnedRuleSetVersion: 1,
		AllowPlanIDs: []string{"member_package"}, RealSwitchEnabled: true,
	}
	selected, err := EvaluateRollout(config, RolloutSubject{OrderID: "order_1", UserID: "user_1", PlanID: "member_package"})
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := EvaluateRollout(config, RolloutSubject{OrderID: "order_2", UserID: "user_2", PlanID: "other_package"})
	if err != nil {
		t.Fatal(err)
	}
	if !selected.UseV132Settlement || legacy.UseV132Settlement {
		t.Fatalf("package whitelist mismatch selected=%+v legacy=%+v", selected, legacy)
	}
}

func TestCanaryTenantWhitelist(t *testing.T) {
	config := RolloutConfig{
		TenantID: "tenant_default", ConfigVersion: 1, Mode: RolloutModeV132Canary, Enabled: true,
		PinnedRuleSetID: "rules_v132", PinnedRuleSetVersion: 1,
		AllowTenantIDs: []string{"tenant_canary"}, RealSwitchEnabled: true,
	}
	selected, err := EvaluateRollout(config, RolloutSubject{TenantID: "tenant_canary", OrderID: "order_1"})
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := EvaluateRollout(config, RolloutSubject{TenantID: "tenant_legacy", OrderID: "order_2"})
	if err != nil {
		t.Fatal(err)
	}
	if !selected.UseV132Settlement || legacy.UseV132Settlement {
		t.Fatalf("tenant whitelist mismatch selected=%+v legacy=%+v", selected, legacy)
	}
}
