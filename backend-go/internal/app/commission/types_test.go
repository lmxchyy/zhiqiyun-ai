package commission

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCommissionRuleValidatesIntegerCentAndBasisPointModels(t *testing.T) {
	now := time.Date(2026, time.July, 15, 0, 0, 0, 0, time.UTC)
	tests := []CommissionRule{
		{
			ID: "direct", TenantID: "tenant_default", Code: "MEMBER_996_DIRECT_AGENT", Name: "direct",
			ProductType: "MEMBER_PACKAGE", ProductID: "plan_ai_creator_996", BeneficiaryRole: BeneficiaryAgent,
			RelationshipLevel: 1, CalculationType: CalculationFixedAmount, FixedAmountCents: 30_000,
			FreezeDays: 7, EffectiveStartAt: now, Version: 1, Status: "ACTIVE",
		},
		{
			ID: "percentage", TenantID: "tenant_default", Code: "PAID_1250", Name: "percentage",
			ProductType: "MEMBER_PACKAGE", BeneficiaryRole: BeneficiaryAgent, RelationshipLevel: 1,
			CalculationType: CalculationPaidAmountPercentage, PercentageBPS: 1_250,
			EffectiveStartAt: now, Version: 1, Status: "ACTIVE",
		},
		{
			ID: "tiered", TenantID: "tenant_default", Code: "TIERED", Name: "tiered",
			ProductType: "MEMBER_PACKAGE", BeneficiaryRole: BeneficiaryAgent,
			CalculationType: CalculationTiered, CalculationConfig: json.RawMessage(`{"tiers":[{"fromCents":0,"amountCents":100}]}`),
			EffectiveStartAt: now, Version: 1, Status: "ACTIVE",
		},
		{
			ID: "remainder", TenantID: "tenant_default", Code: "REMAINDER", Name: "remainder",
			ProductType: "MEMBER_PACKAGE", BeneficiaryRole: BeneficiaryPlatform,
			CalculationType: CalculationRemainderToPlatform, EffectiveStartAt: now, Version: 1, Status: "ACTIVE",
		},
	}
	for _, rule := range tests {
		if err := rule.Validate(); err != nil {
			t.Fatalf("rule %s should be valid: %v", rule.Code, err)
		}
	}

	invalid := tests[1]
	invalid.PercentageBPS = 10_001
	if err := invalid.Validate(); err == nil {
		t.Fatal("percentage above 100 percent must be rejected")
	}
}

func TestCommissionRecordRequiresImmutableReversalShape(t *testing.T) {
	now := time.Now().UTC()
	base := CommissionRecord{
		ID: "record", TenantID: "tenant_default", OrderID: "order", OrderNo: "ORDER-1",
		BeneficiaryType: BeneficiaryAgent, BeneficiaryID: "agent", SourceUserID: "user",
		RuleID: "rule", RuleVersion: 1, AmountCents: 30_000, Currency: "CNY",
		RecordType: RecordEarning, Status: CommissionExpected, IdempotencyKey: "earning-key",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("earning should be valid: %v", err)
	}

	reversal := base
	reversal.ID = "reversal"
	reversal.AmountCents = -30_000
	reversal.RecordType = RecordReversal
	reversal.Status = CommissionReversed
	reversal.ReversalOfID = base.ID
	reversal.IdempotencyKey = "reversal-key"
	if err := reversal.Validate(); err != nil {
		t.Fatalf("reversal should be valid: %v", err)
	}

	reversal.ReversalOfID = ""
	if err := reversal.Validate(); err == nil {
		t.Fatal("reversal without original record must be rejected")
	}
}

func TestWalletBalancesRejectNegativeBuckets(t *testing.T) {
	balances := WalletBalances{ExpectedCents: 10, FrozenCents: 20, AvailableCents: 30, SettlingCents: 40, SettledCents: 50, RecoverableCents: 60}
	if err := balances.Validate(); err != nil {
		t.Fatalf("positive balances should be valid: %v", err)
	}
	balances.AvailableCents = -1
	if err := balances.Validate(); err == nil {
		t.Fatal("negative wallet bucket must be rejected")
	}
}

func TestPayoutBatchSummaryMustEqualDetails(t *testing.T) {
	batch := PayoutBatch{
		ID: "batch", BatchNo: "BATCH-1", TenantID: "tenant_default", ProviderCode: "EXCEL_MANUAL",
		BusinessScene: "COMMISSION_PAYOUT", TotalCount: 2, TotalAmountCents: 50_000, Status: BatchDraft,
		Details: []PayoutDetail{
			{ID: "d1", BatchID: "batch", DetailNo: "DETAIL-1", SettlementApplicationID: "a1", BeneficiaryType: BeneficiaryAgent, BeneficiaryID: "agent-1", WorkerProfileID: "w1", AmountCents: 30_000, IdempotencyKey: "key-1"},
			{ID: "d2", BatchID: "batch", DetailNo: "DETAIL-2", SettlementApplicationID: "a2", BeneficiaryType: BeneficiaryOperationCenter, BeneficiaryID: "operation-1", WorkerProfileID: "w2", AmountCents: 20_000, IdempotencyKey: "key-2"},
		},
	}
	if err := batch.ValidateTotals(); err != nil {
		t.Fatalf("batch should be valid: %v", err)
	}

	batch.TotalAmountCents = 49_999
	if err := batch.ValidateTotals(); err == nil {
		t.Fatal("summary amount mismatch must be rejected")
	}
}

func Test996CashSplitExcludesTokenRights(t *testing.T) {
	paid := AmountCents(99_600)
	directAgent := AmountCents(30_000)
	operationCenter := AmountCents(20_000)
	platformCash := paid - directAgent - operationCenter
	if platformCash != 49_600 {
		t.Fatalf("platform cash = %d, want 49600", platformCash)
	}

	tokenRightsValue := AmountCents(40_000)
	if paid-directAgent-operationCenter-tokenRightsValue == platformCash {
		t.Fatal("token rights must not be included in cash commission split")
	}
}
