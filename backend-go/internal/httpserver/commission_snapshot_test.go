package httpserver

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	commissionapp "xianzhi-ai/backend-go/internal/app/commission"
)

func TestRebuildCommissionRulesFromSnapshotDoesNotRequireCurrentRuleRecords(t *testing.T) {
	paidAt := time.Date(2026, time.July, 28, 8, 0, 0, 0, time.UTC)
	snapshots := []commissionRuleSnapshot{
		{
			ID: "retired-agent-rule", Code: "SNAPSHOT_AGENT", Name: "snapshotted agent commission", Version: 3,
			BeneficiaryRole: commissionapp.BeneficiaryAgent, RelationshipLevel: 1,
			CalculationType: commissionapp.CalculationFixedAmount, FixedAmountCents: 30_000,
			FreezeDays: 7, RefundPolicy: "REVERSE_OR_RECOVER",
		},
		{
			ID: "replaced-platform-rule", Code: "SNAPSHOT_PLATFORM", Name: "snapshotted platform remainder", Version: 4,
			BeneficiaryRole: commissionapp.BeneficiaryPlatform,
			CalculationType: commissionapp.CalculationRemainderToPlatform,
			RefundPolicy:    "REVERSE_OR_RECOVER",
		},
	}

	rules, err := rebuildCommissionRulesFromSnapshot(commissionRuleSnapshotContext{
		TenantID: "tenant_default", ProductType: "AGENT_JOIN_PACKAGE", ProductID: "plan_agent", PaidAt: paidAt,
	}, snapshots)
	if err != nil {
		t.Fatalf("rebuild rules from immutable order snapshot: %v", err)
	}
	if len(rules) != 2 || rules[0].ID != "retired-agent-rule" || rules[1].ID != "replaced-platform-rule" {
		t.Fatalf("unexpected rebuilt rules: %+v", rules)
	}
	result, err := commissionapp.NewEngine().Calculate(commissionapp.CalculationInput{
		TenantID: "tenant_default", OrderID: "order-v2-snapshot", OrderNo: "ORDER-V2-SNAPSHOT",
		ProductType: "AGENT_JOIN_PACKAGE", ProductID: "plan_agent", SourceUserID: "buyer",
		OrderAmountCents: 99_600, PaidAmountCents: 99_600, Quantity: 1, PaidAt: paidAt,
		Relationships: commissionapp.RelationshipSnapshot{
			AgentIDsByLevel: map[int]string{1: "agent-1"}, PlatformID: "platform:tenant_default",
		},
		Rules: rules,
	})
	if err != nil {
		t.Fatalf("calculate using rebuilt snapshot rules: %v", err)
	}
	if len(result.Records) != 2 || result.PlatformIncomeCents != 69_600 {
		t.Fatalf("snapshot economics changed: %+v", result)
	}
}

func TestRebuildCommissionRulesFromSnapshotFailsClosedWhenTierConfigIsMissing(t *testing.T) {
	_, err := rebuildCommissionRulesFromSnapshot(commissionRuleSnapshotContext{
		TenantID: "tenant_default", ProductType: "MEMBER_PACKAGE", ProductID: "plan_member",
		PaidAt: time.Date(2026, time.July, 28, 8, 0, 0, 0, time.UTC),
	}, []commissionRuleSnapshot{{
		ID: "legacy-tiered-rule", Code: "LEGACY_TIERED", Name: "legacy tiered", Version: 1,
		BeneficiaryRole: commissionapp.BeneficiaryAgent, RelationshipLevel: 1,
		CalculationType: commissionapp.CalculationTiered, RefundPolicy: "REVERSE_OR_RECOVER",
	}})
	if !errors.Is(err, errCommissionRuleSnapshotIncomplete) {
		t.Fatalf("missing tier config must fail closed, got %v", err)
	}
	if !strings.Contains(err.Error(), "calculationConfig") {
		t.Fatalf("failure must identify missing calculationConfig, got %v", err)
	}
}

func TestEmptyCommissionRuleSnapshotExplicitlyMeansNoCommission(t *testing.T) {
	paidAt := time.Date(2026, time.July, 28, 8, 0, 0, 0, time.UTC)
	snapshotContext := commissionRuleSnapshotContext{
		TenantID: "tenant_default", ProductType: "MEMBER_PACKAGE", ProductID: "plan_member", PaidAt: paidAt,
	}
	rules, err := rebuildCommissionRulesFromSnapshot(snapshotContext, []commissionRuleSnapshot{})
	if err != nil {
		t.Fatalf("explicit empty commission snapshot must be accepted: %v", err)
	}
	if rules == nil || len(rules) != 0 {
		t.Fatalf("explicit empty commission snapshot must rebuild to a non-nil empty slice: %#v", rules)
	}

	order := adminOrder{
		ID: "order-without-commission", OrderNo: "ORDER-WITHOUT-COMMISSION", TenantID: "tenant_default",
		UserID: "buyer", PlanID: "plan_member", AmountCents: 100, PaidAt: paidAt.Format(time.RFC3339Nano),
		PriceSnapshot:              map[string]any{"snapshotVersion": 2},
		CommissionSnapshotCaptured: true, CommissionRuleSnapshot: []commissionRuleSnapshot{},
	}
	plan := adminPlan{ID: "plan_member", PlanType: planTypeMemberPackage}
	commerceContext := commissionOrderContext{
		OrderID: order.ID, OrderType: orderTypeUserRechargeDirect, PlanType: planTypeMemberPackage,
		AmountCents: 100, BuyerUserID: order.UserID,
	}
	result, err := generateCommissionRecordsForCommerceOrderTx(t.Context(), nil, order, plan, commerceContext)
	if err != nil {
		t.Fatalf("explicit no-commission snapshot must not query current rules or block fulfillment: %v", err)
	}
	if len(result.Records) != 0 || result.PlatformIncomeCents != 100 {
		t.Fatalf("unexpected no-commission settlement: %+v", result)
	}
	legacyResult, err := compatibilitySettlementResult(commerceContext, result)
	if err != nil {
		t.Fatalf("no-commission settlement must still balance to platform income: %v", err)
	}
	if legacyResult.PlatformIncomeCents != 100 {
		t.Fatalf("platform income=%d, want 100", legacyResult.PlatformIncomeCents)
	}
}

func TestSnapshotCommissionRulesCapturesTierConfigAndPriority(t *testing.T) {
	rule := commissionapp.CommissionRule{
		ID: "tiered-v2", TenantID: "tenant_default", Code: "TIERED_V2", Name: "tiered v2",
		ProductType: "MEMBER_PACKAGE", BeneficiaryRole: commissionapp.BeneficiaryAgent, RelationshipLevel: 1,
		CalculationType:   commissionapp.CalculationTiered,
		CalculationConfig: json.RawMessage(`{"basis":"PAID_AMOUNT","tiers":[{"minAmountCents":1,"calculationType":"FIXED_AMOUNT","fixedAmountCents":100}]}`),
		Priority:          23, FreezeDays: 7, RefundPolicy: "REVERSE_OR_RECOVER",
		EffectiveStartAt: time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC), Version: 2, Status: "ACTIVE",
	}
	snapshots := snapshotCommissionRules([]commissionapp.CommissionRule{rule})
	encoded, err := json.Marshal(snapshots[0])
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if intValue(payload["priority"]) != 23 {
		t.Fatalf("priority is missing from snapshot: %s", encoded)
	}
	config, ok := payload["calculationConfig"].(map[string]any)
	if !ok || stringValue(config["basis"]) != "PAID_AMOUNT" {
		t.Fatalf("calculation config is missing from snapshot: %s", encoded)
	}
}
