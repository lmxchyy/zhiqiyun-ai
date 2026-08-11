package httpserver

import "testing"

func TestExistingV132SettlementDecisionSurvivesRollbackToShadow(t *testing.T) {
	existing := orderSettlementDecision{OrderID: "order-1", SettlementEngine: settlementEngineV132, RuleSetID: "rules-v1", RuleSetVersion: 1}
	proposed := orderSettlementDecision{OrderID: "order-1", SettlementEngine: settlementEngineLegacy}
	resolved, err := preserveExistingSettlementDecision(existing, proposed)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.SettlementEngine != settlementEngineV132 || resolved.RuleSetID != "rules-v1" {
		t.Fatalf("historical V1.3.2 decision was migrated backward: %+v", resolved)
	}
}

func TestSettlementDecisionRejectsEngineChangeForSameOrder(t *testing.T) {
	existing := orderSettlementDecision{OrderID: "order-1", SettlementEngine: settlementEngineLegacy}
	if err := validateSettlementWriteSource(existing.SettlementEngine, settlementEngineV132); err == nil {
		t.Fatal("same order must not accept a second settlement write source")
	}
}

func TestV132SettlementConservesWalletAndPlatformIncome(t *testing.T) {
	if err := validateV132SettlementConservation(99600, 40000, 30000, 20000, 9600); err != nil {
		t.Fatal(err)
	}
	if err := validateV132SettlementConservation(99600, 40000, 30000, 20000, 9500); err == nil {
		t.Fatal("platform income mismatch must fail conservation")
	}
}
