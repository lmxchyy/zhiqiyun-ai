package commission

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestEngineCalculates996CashSplitWithoutTokenRights(t *testing.T) {
	input := baseEngineInput()
	input.Rules = []CommissionRule{
		activeRule("agent", BeneficiaryAgent, CalculationFixedAmount, 10, 30_000, 0, 1),
		activeRule("operation", BeneficiaryOperationCenter, CalculationFixedAmount, 20, 20_000, 0, 1),
		activeRule("platform", BeneficiaryPlatform, CalculationRemainderToPlatform, 1, 0, 0, 0),
	}
	result, err := NewEngine().Calculate(input)
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if len(result.Records) != 3 || result.CashCommissionCents != 99_600 || result.PlatformIncomeCents != 49_600 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Records[0].AmountCents != 30_000 || result.Records[1].AmountCents != 20_000 || result.Records[2].AmountCents != 49_600 {
		t.Fatalf("unexpected records: %+v", result.Records)
	}
	for _, record := range result.Records {
		if record.Status != CommissionExpected || record.IdempotencyKey == "" || record.FreezeUntil == nil {
			t.Fatalf("invalid generated record: %+v", record)
		}
	}

	repeated, err := NewEngine().Calculate(input)
	if err != nil {
		t.Fatalf("repeat calculate: %v", err)
	}
	for i := range result.Records {
		if result.Records[i].ID != repeated.Records[i].ID || result.Records[i].IdempotencyKey != repeated.Records[i].IdempotencyKey {
			t.Fatal("engine output must be deterministic")
		}
	}
}

func TestEngineAppliesExplicitlySelectedCommissionTemplateAcrossProducts(t *testing.T) {
	for _, productType := range []string{"MEMBER_PACKAGE", "AGENT_JOIN_PACKAGE"} {
		t.Run(productType, func(t *testing.T) {
			input := baseEngineInput()
			input.ProductType = productType
			input.ProductID = "product-for-" + productType
			input.Rules = []CommissionRule{
				activeRule("agent", BeneficiaryAgent, CalculationFixedAmount, 10, 30_000, 0, 1),
				activeRule("operation", BeneficiaryOperationCenter, CalculationFixedAmount, 20, 20_000, 0, 1),
				activeRule("platform", BeneficiaryPlatform, CalculationRemainderToPlatform, 1000, 0, 0, 0),
			}
			for index := range input.Rules {
				input.Rules[index].ProductType = "COMMISSION_TEMPLATE"
				input.Rules[index].ProductID = "COMMISSION_996_STANDARD"
			}
			result, err := NewEngine().Calculate(input)
			if err != nil {
				t.Fatalf("calculate template: %v", err)
			}
			if len(result.Records) != 3 || result.PlatformIncomeCents != 49_600 {
				t.Fatalf("unexpected result: %+v", result)
			}
		})
	}
}

func TestEngineMissingRelationshipsMoveCashToPlatform(t *testing.T) {
	tests := []struct {
		name          string
		relationships RelationshipSnapshot
		wantRecords   int
		wantPlatform  AmountCents
	}{
		{name: "no agent", relationships: RelationshipSnapshot{OperationCenterID: "operation-1", PlatformID: "platform"}, wantRecords: 2, wantPlatform: 79_600},
		{name: "no operation center", relationships: RelationshipSnapshot{AgentIDsByLevel: map[int]string{1: "agent-1"}, PlatformID: "platform"}, wantRecords: 2, wantPlatform: 69_600},
		{name: "no relationships", relationships: RelationshipSnapshot{PlatformID: "platform"}, wantRecords: 1, wantPlatform: 99_600},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := baseEngineInput()
			input.Relationships = test.relationships
			input.Rules = []CommissionRule{
				activeRule("agent", BeneficiaryAgent, CalculationFixedAmount, 10, 30_000, 0, 1),
				activeRule("operation", BeneficiaryOperationCenter, CalculationFixedAmount, 20, 20_000, 0, 1),
				activeRule("platform", BeneficiaryPlatform, CalculationRemainderToPlatform, 1000, 0, 0, 0),
			}
			result, err := NewEngine().Calculate(input)
			if err != nil {
				t.Fatalf("calculate: %v", err)
			}
			if len(result.Records) != test.wantRecords || result.PlatformIncomeCents != test.wantPlatform {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

func TestEngineSupportsPercentageQuantityAndTieredRules(t *testing.T) {
	input := baseEngineInput()
	input.OrderAmountCents = 120_000
	input.PaidAmountCents = 100_000
	input.Quantity = 3
	tierConfig := json.RawMessage(`{"basis":"PAID_AMOUNT","tiers":[{"minAmountCents":50000,"maxAmountCents":100000,"calculationType":"FIXED_AMOUNT","fixedAmountCents":7000}]}`)
	tier := activeRule("tier", BeneficiaryAgent, CalculationTiered, 40, 0, 0, 2)
	tier.CalculationConfig = tierConfig
	input.Relationships.AgentIDsByLevel[2] = "agent-2"
	input.Rules = []CommissionRule{
		activeRule("order-percent", BeneficiaryAgent, CalculationOrderPercentage, 10, 0, 1_000, 1),
		activeRule("paid-percent", BeneficiaryOperationCenter, CalculationPaidAmountPercentage, 20, 0, 500, 1),
		activeRule("quantity", BeneficiaryAgent, CalculationQuantity, 30, 2_000, 0, 2),
		tier,
		activeRule("platform", BeneficiaryPlatform, CalculationRemainderToPlatform, 1000, 0, 0, 0),
	}
	result, err := NewEngine().Calculate(input)
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	wants := []AmountCents{12_000, 5_000, 6_000, 7_000, 70_000}
	for i, want := range wants {
		if result.Records[i].AmountCents != want {
			t.Fatalf("record %d amount = %d, want %d", i, result.Records[i].AmountCents, want)
		}
	}
}

func TestEngineRejectsOverAllocationDuplicateAndOverflow(t *testing.T) {
	input := baseEngineInput()
	input.Rules = []CommissionRule{
		activeRule("too-much", BeneficiaryAgent, CalculationFixedAmount, 10, 100_000, 0, 1),
		activeRule("platform", BeneficiaryPlatform, CalculationRemainderToPlatform, 1000, 0, 0, 0),
	}
	if _, err := NewEngine().Calculate(input); err == nil {
		t.Fatal("over allocation must fail")
	}

	duplicate := activeRule("duplicate", BeneficiaryAgent, CalculationFixedAmount, 10, 1, 0, 1)
	input.Rules = []CommissionRule{duplicate, duplicate, activeRule("platform", BeneficiaryPlatform, CalculationRemainderToPlatform, 1000, 0, 0, 0)}
	if _, err := NewEngine().Calculate(input); err == nil {
		t.Fatal("duplicate rule output must fail")
	}

	input = baseEngineInput()
	input.OrderAmountCents = AmountCents(math.MaxInt64)
	input.PaidAmountCents = AmountCents(math.MaxInt64)
	input.Quantity = 2
	input.Rules = []CommissionRule{
		activeRule("quantity", BeneficiaryAgent, CalculationQuantity, 10, AmountCents(math.MaxInt64), 0, 1),
		activeRule("platform", BeneficiaryPlatform, CalculationRemainderToPlatform, 1000, 0, 0, 0),
	}
	if _, err := NewEngine().Calculate(input); err == nil {
		t.Fatal("integer overflow must fail")
	}
}

func baseEngineInput() CalculationInput {
	paidAt := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	return CalculationInput{
		TenantID: "tenant_default", OrderID: "order-996", OrderNo: "ORDER-996",
		ProductType: "MEMBER_PACKAGE", ProductID: "plan_ai_creator_996", SourceUserID: "user-1",
		OrderAmountCents: 99_600, PaidAmountCents: 99_600, Quantity: 1, PaidAt: paidAt,
		Relationships: RelationshipSnapshot{AgentIDsByLevel: map[int]string{1: "agent-1"}, OperationCenterID: "operation-1", PlatformID: "platform"},
	}
}

func activeRule(id string, beneficiary BeneficiaryType, calculation CalculationType, priority int, fixed AmountCents, bps PercentageBPS, level int) CommissionRule {
	return CommissionRule{
		ID: id, TenantID: "tenant_default", Code: id, Name: id, ProductType: "MEMBER_PACKAGE",
		ProductID: "plan_ai_creator_996", BeneficiaryRole: beneficiary, RelationshipLevel: level,
		CalculationType: calculation, FixedAmountCents: fixed, PercentageBPS: bps,
		Priority: priority, FreezeDays: 7, RefundPolicy: "REVERSE_OR_RECOVER",
		EffectiveStartAt: time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC), Version: 1, Status: "ACTIVE",
	}
}
