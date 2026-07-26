package operationcenter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrRewardNotFrozen                 = errors.New("referral reward is not frozen")
	ErrRewardNotDue                    = errors.New("referral reward is not due")
	ErrReleaseTaskNotExecutable        = errors.New("referral reward release task is not executable")
	ErrReleaseTaskMismatch             = errors.New("referral reward release task does not match reward")
	ErrFrozenBalanceInsufficient       = errors.New("referral reward frozen balance is insufficient")
	ErrReleaseLedgerConflict           = errors.New("referral reward release ledger conflicts with existing data")
	ErrRewardReleaseInvariantViolation = errors.New("referral reward release invariant is violated")
)

const ReferralRewardAvailable ReferralRewardStatus = "AVAILABLE"

type ReferralRewardReleaseOptions struct {
	LeaseDuration time.Duration
	RetryDelay    time.Duration
}

type ReferralRewardReleaseResult struct {
	TaskID, RewardID, WalletLedgerID string
	AmountCents                      int64
	IdempotentReplay                 bool
}

type ReferralRewardReleaseBatchResult struct {
	Claimed, Succeeded, Failed int
	Results                    []ReferralRewardReleaseResult
	Errors                     []error
}

type referralGrantLedgerSnapshot struct {
	ID, TenantID, AccountID, ReferralRewardID string
	BusinessType                              string
	FrozenDelta                               int64
}

func validateReferralRewardRelease(task ReferralRewardReleaseTask, reward ReferralReward, wallet referralWalletState, grant referralGrantLedgerSnapshot, now time.Time) error {
	if task.Status != ReferralRewardReleaseProcessing {
		return ErrReleaseTaskNotExecutable
	}
	if task.ReferralRewardID != reward.ID || reward.CurrentReleaseTaskID != task.ID {
		return ErrReleaseTaskMismatch
	}
	if task.ExecuteAt.After(now) || reward.FreezeUntil.After(now) {
		return ErrRewardNotDue
	}
	if reward.Status != ReferralRewardFrozen {
		return ErrRewardNotFrozen
	}
	if reward.AmountCents <= 0 {
		return ErrRewardReleaseInvariantViolation
	}
	if wallet.TenantID != reward.TenantID || wallet.BeneficiaryType != string(reward.BeneficiaryType) || wallet.BeneficiaryID != reward.BeneficiaryUserID {
		return ErrRewardReleaseInvariantViolation
	}
	if wallet.Frozen < reward.AmountCents {
		return ErrFrozenBalanceInsufficient
	}
	if grant.ID == "" || grant.ID != reward.GrantWalletLedgerID || grant.TenantID != reward.TenantID || grant.AccountID != wallet.ID ||
		grant.ReferralRewardID != reward.ID || grant.BusinessType != "REFERRAL_REWARD_GRANT" || grant.FrozenDelta != reward.AmountCents {
		return ErrRewardReleaseInvariantViolation
	}
	return nil
}

func applyReferralRewardReleaseBalances(before referralWalletBalances, amount int64) (referralWalletBalances, error) {
	if amount <= 0 || before.Frozen < amount {
		return before, ErrFrozenBalanceInsufficient
	}
	after := before
	after.Frozen -= amount
	after.Available += amount
	beforeEquity := before.Frozen + before.Available + before.Settled - before.Recoverable
	afterEquity := after.Frozen + after.Available + after.Settled - after.Recoverable
	if beforeEquity != afterEquity || before.Settled != after.Settled || before.Recoverable != after.Recoverable {
		return before, ErrRewardReleaseInvariantViolation
	}
	return after, nil
}

func referralRewardReleaseLedgerKey(taskID, rewardID string) string {
	return "referral-wallet-release:" + taskID + ":" + rewardID
}

func classifyReferralRewardReleaseFailure(err error) (RefundFailureClass, bool) {
	if errors.Is(err, ErrRewardNotDue) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return RefundFailureClass("TEMPORARY_FAILURE"), true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01" || pgErr.Code == "55P03") {
		return RefundFailureClass("TEMPORARY_FAILURE"), true
	}
	if errors.Is(err, ErrRewardNotFrozen) || errors.Is(err, ErrReleaseTaskNotExecutable) ||
		errors.Is(err, ErrReleaseTaskMismatch) || errors.Is(err, ErrFrozenBalanceInsufficient) ||
		errors.Is(err, ErrReleaseLedgerConflict) || errors.Is(err, ErrRewardReleaseInvariantViolation) {
		return RefundFailureClass("VALIDATION_FAILURE"), false
	}
	return RefundFailureClass("UNKNOWN"), false
}

func validateReferralReleaseLease(task ReferralRewardReleaseTask, owner string, now time.Time) error {
	if task.Status != ReferralRewardReleaseProcessing || task.LeaseOwner == nil || *task.LeaseOwner != owner || task.LeaseExpiresAt == nil || !task.LeaseExpiresAt.After(now) {
		return fmt.Errorf("%w: owner=%s", ErrReleaseTaskNotExecutable, owner)
	}
	return nil
}
