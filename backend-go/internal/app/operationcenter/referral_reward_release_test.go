package operationcenter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestReferralRewardReleaseValidationAndConservation(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	owner := "worker"
	lease := now.Add(time.Minute)
	task := ReferralRewardReleaseTask{ID: "task", TenantID: "tenant", ReferralRewardID: "reward", Status: ReferralRewardReleaseProcessing, ExecuteAt: now, LeaseOwner: &owner, LeaseExpiresAt: &lease}
	reward := ReferralReward{ID: "reward", TenantID: "tenant", BeneficiaryType: ReferralBeneficiaryAgent, BeneficiaryUserID: "agent", AmountCents: 300, Status: ReferralRewardFrozen, FreezeUntil: now, GrantWalletLedgerID: "grant", CurrentReleaseTaskID: "task"}
	wallet := referralWalletState{ID: "wallet", TenantID: "tenant", BeneficiaryType: "AGENT", BeneficiaryID: "agent", Status: "ACTIVE", referralWalletBalances: referralWalletBalances{Frozen: 300, Available: 20, Settled: 10, Recoverable: 5}}
	grant := referralGrantLedgerSnapshot{ID: "grant", TenantID: "tenant", AccountID: "wallet", ReferralRewardID: "reward", BusinessType: "REFERRAL_REWARD_GRANT", FrozenDelta: 300}
	if err := validateReferralRewardRelease(task, reward, wallet, grant, now); err != nil {
		t.Fatal(err)
	}
	after, err := applyReferralRewardReleaseBalances(wallet.referralWalletBalances, reward.AmountCents)
	if err != nil {
		t.Fatal(err)
	}
	if after.Frozen != 0 || after.Available != 320 || after.Settled != 10 || after.Recoverable != 5 {
		t.Fatalf("unexpected release balances: %+v", after)
	}
	beforeEquity := wallet.Frozen + wallet.Available + wallet.Settled - wallet.Recoverable
	afterEquity := after.Frozen + after.Available + after.Settled - after.Recoverable
	if beforeEquity != afterEquity {
		t.Fatalf("equity changed before=%d after=%d", beforeEquity, afterEquity)
	}
}

func TestReferralRewardReleaseRejectsNotDueCancelledAndInsufficient(t *testing.T) {
	now := time.Now().UTC()
	task := ReferralRewardReleaseTask{ID: "task", ReferralRewardID: "reward", Status: ReferralRewardReleaseProcessing, ExecuteAt: now.Add(time.Minute)}
	reward := ReferralReward{ID: "reward", CurrentReleaseTaskID: "task", Status: ReferralRewardFrozen, FreezeUntil: now, AmountCents: 10}
	if err := validateReferralRewardRelease(task, reward, referralWalletState{}, referralGrantLedgerSnapshot{}, now); !errors.Is(err, ErrRewardNotDue) {
		t.Fatalf("not due error=%v", err)
	}
	task.ExecuteAt = now
	reward.Status = ReferralRewardStatus("CANCELLED")
	if err := validateReferralRewardRelease(task, reward, referralWalletState{}, referralGrantLedgerSnapshot{}, now); !errors.Is(err, ErrRewardNotFrozen) {
		t.Fatalf("cancelled error=%v", err)
	}
	if _, err := applyReferralRewardReleaseBalances(referralWalletBalances{Frozen: 9}, 10); !errors.Is(err, ErrFrozenBalanceInsufficient) {
		t.Fatalf("insufficient error=%v", err)
	}
}

func TestReferralRewardReleaseIdempotencyAndFailureClassification(t *testing.T) {
	key1 := referralRewardReleaseLedgerKey("task", "reward")
	key2 := referralRewardReleaseLedgerKey("task", "reward")
	if key1 != key2 {
		t.Fatalf("release key changed: %s %s", key1, key2)
	}
	cases := []struct {
		err       error
		class     RefundFailureClass
		retryable bool
	}{
		{ErrRewardNotDue, RefundFailureClass("TEMPORARY_FAILURE"), true},
		{&pgconn.PgError{Code: "40001"}, RefundFailureClass("TEMPORARY_FAILURE"), true},
		{ErrFrozenBalanceInsufficient, RefundFailureClass("VALIDATION_FAILURE"), false},
		{errors.New("unknown"), RefundFailureClass("UNKNOWN"), false},
		{context.DeadlineExceeded, RefundFailureClass("TEMPORARY_FAILURE"), true},
	}
	for _, item := range cases {
		class, retryable := classifyReferralRewardReleaseFailure(item.err)
		if class != item.class || retryable != item.retryable {
			t.Fatalf("classification err=%v class=%s retryable=%v", item.err, class, retryable)
		}
	}
}
