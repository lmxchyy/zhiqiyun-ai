package operationcenter

import (
	"errors"
	"testing"
)

func TestReferralRewardReversalPlansAndConservation(t *testing.T) {
	cases := []struct {
		name        string
		status      ReferralRewardStatus
		wallet      referralWalletBalances
		direct      int64
		recoverable int64
	}{
		{"frozen", ReferralRewardFrozen, referralWalletBalances{Frozen: 100}, 100, 0},
		{"available sufficient", ReferralRewardAvailable, referralWalletBalances{Available: 100}, 100, 0},
		{"available insufficient", ReferralRewardAvailable, referralWalletBalances{Available: 35}, 35, 65},
		{"settled", ReferralRewardStatus("SETTLED"), referralWalletBalances{Settled: 100}, 0, 100},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			reward := ReferralReward{ID: "reward", TenantID: "tenant", BeneficiaryType: ReferralBeneficiaryAgent, BeneficiaryUserID: "agent", AmountCents: 100, Status: item.status}
			wallet := referralWalletState{TenantID: "tenant", BeneficiaryType: "AGENT", BeneficiaryID: "agent", referralWalletBalances: item.wallet}
			plan, err := buildReferralRewardReversalPlan(reward, wallet)
			if err != nil {
				t.Fatal(err)
			}
			after, err := applyReferralRewardReversalBalances(item.wallet, plan)
			if err != nil {
				t.Fatal(err)
			}
			if plan.FrozenDebitCents+plan.AvailableDebitCents != item.direct || plan.RecoverableCents != item.recoverable || item.direct+item.recoverable != reward.AmountCents {
				t.Fatalf("plan=%+v", plan)
			}
			if after.Settled != item.wallet.Settled || after.Recoverable != item.wallet.Recoverable+item.recoverable {
				t.Fatalf("after=%+v", after)
			}
		})
	}
}

func TestReferralRewardReversalRejectsTerminalAndStableKeys(t *testing.T) {
	reward := ReferralReward{ID: "reward", TenantID: "tenant", BeneficiaryType: ReferralBeneficiaryAgent, BeneficiaryUserID: "agent", AmountCents: 100, Status: ReferralRewardStatus("REVERSED")}
	wallet := referralWalletState{TenantID: "tenant", BeneficiaryType: "AGENT", BeneficiaryID: "agent"}
	if _, err := buildReferralRewardReversalPlan(reward, wallet); !errors.Is(err, ErrRewardStateNotReversible) {
		t.Fatalf("terminal reward error=%v", err)
	}
	if referralRewardReversalKey("refund", "reward") != referralRewardReversalKey("refund", "reward") || referralRewardReversalLedgerKey("refund", "reward") != referralRewardReversalLedgerKey("refund", "reward") {
		t.Fatal("reversal keys are not stable")
	}
}
