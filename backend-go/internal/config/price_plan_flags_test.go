package config

import (
	"strings"
	"testing"
)

func TestLoadPricePlanPhase1Flags(t *testing.T) {
	t.Setenv("PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED", "true")
	t.Setenv("PRICE_PLAN_TEST_ENTRY_ENABLED", "1")
	t.Setenv("SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED", "yes")
	cfg := Load()
	if !cfg.PricePlanCreationEnabled || !cfg.PricePlanTestEntryEnabled || !cfg.SnapshotV2FulfillmentEnabled {
		t.Fatalf("phase-one flags were not loaded: %+v", cfg)
	}
}

func TestCreationFlagRequiresSnapshotV2Fulfillment(t *testing.T) {
	cfg := Config{PricePlanCreationEnabled: true}
	err := cfg.ValidateProduction()
	if err == nil || !strings.Contains(err.Error(), "SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED") {
		t.Fatalf("unsafe creation/fulfillment flag combination was accepted: %v", err)
	}
}
