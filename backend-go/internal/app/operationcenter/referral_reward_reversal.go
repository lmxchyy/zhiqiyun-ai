package operationcenter

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrRewardAlreadyReversed            = errors.New("referral reward is already reversed")
	ErrRewardReversalConflict           = errors.New("referral reward was reversed by another refund")
	ErrRewardStateNotReversible         = errors.New("referral reward state is not reversible")
	ErrRewardWalletMismatch             = errors.New("referral reward wallet does not match")
	ErrReversalLedgerConflict           = errors.New("referral reward reversal ledger conflicts with existing data")
	ErrReleaseTaskStillExecutable       = errors.New("referral reward release task remains executable")
	ErrRewardReversalInvariantViolation = errors.New("referral reward reversal invariant is violated")
)

type ReferralRewardReversalCommand struct {
	RefundTaskID, OperationCenterServiceOrderID, ReferralEventID string
	RefundAmountCents                                            int64
	ReversalReason, OperatorID, TransactionGroupID               string
}

type ReferralRewardReversalType string

const (
	ReferralReversalFrozenDebit          ReferralRewardReversalType = "FROZEN_DEBIT"
	ReferralReversalAvailableDebit       ReferralRewardReversalType = "AVAILABLE_DEBIT"
	ReferralReversalAvailableRecoverable ReferralRewardReversalType = "AVAILABLE_RECOVERABLE"
	ReferralReversalSettledRecoverable   ReferralRewardReversalType = "SETTLED_RECOVERABLE"
)

type ReferralRewardReversalPlan struct {
	OriginalReward      ReferralReward
	ReversalType        ReferralRewardReversalType
	FrozenDebitCents    int64
	AvailableDebitCents int64
	RecoverableCents    int64
	SourceStatus        ReferralRewardStatus
}

type ReferralRewardReversalItem struct {
	OriginalRewardID, ReversalRewardID, WalletLedgerID string
	SourceStatus                                       ReferralRewardStatus
	AmountCents, DirectDebitCents, RecoverableCents    int64
}

type ReferralRewardReversalResult struct {
	RefundTaskID, ReferralEventID string
	Items                         []ReferralRewardReversalItem
	OriginalAmountCents           int64
	DirectDebitCents              int64
	RecoverableCents              int64
	IdempotentReplay              bool
}

func buildReferralRewardReversalPlan(reward ReferralReward, wallet referralWalletState) (ReferralRewardReversalPlan, error) {
	if reward.AmountCents <= 0 {
		return ReferralRewardReversalPlan{}, ErrRewardReversalInvariantViolation
	}
	if wallet.TenantID != reward.TenantID || wallet.BeneficiaryType != string(reward.BeneficiaryType) || wallet.BeneficiaryID != reward.BeneficiaryUserID {
		return ReferralRewardReversalPlan{}, ErrRewardWalletMismatch
	}
	plan := ReferralRewardReversalPlan{OriginalReward: reward, SourceStatus: reward.Status}
	switch reward.Status {
	case ReferralRewardFrozen:
		if wallet.Frozen < reward.AmountCents {
			return plan, ErrFrozenBalanceInsufficient
		}
		plan.ReversalType = ReferralReversalFrozenDebit
		plan.FrozenDebitCents = reward.AmountCents
	case ReferralRewardAvailable:
		plan.AvailableDebitCents = reward.AmountCents
		if wallet.Available < plan.AvailableDebitCents {
			plan.AvailableDebitCents = wallet.Available
		}
		plan.RecoverableCents = reward.AmountCents - plan.AvailableDebitCents
		if plan.RecoverableCents > 0 {
			plan.ReversalType = ReferralReversalAvailableRecoverable
		} else {
			plan.ReversalType = ReferralReversalAvailableDebit
		}
	case ReferralRewardStatus("SETTLED"):
		plan.ReversalType = ReferralReversalSettledRecoverable
		plan.RecoverableCents = reward.AmountCents
	default:
		return plan, fmt.Errorf("%w: %s", ErrRewardStateNotReversible, reward.Status)
	}
	if plan.FrozenDebitCents+plan.AvailableDebitCents+plan.RecoverableCents != reward.AmountCents {
		return plan, ErrRewardReversalInvariantViolation
	}
	return plan, nil
}

func applyReferralRewardReversalBalances(before referralWalletBalances, plan ReferralRewardReversalPlan) (referralWalletBalances, error) {
	if before.Frozen < plan.FrozenDebitCents || before.Available < plan.AvailableDebitCents {
		return before, ErrRewardReversalInvariantViolation
	}
	after := before
	after.Frozen -= plan.FrozenDebitCents
	after.Available -= plan.AvailableDebitCents
	after.Recoverable += plan.RecoverableCents
	beforeEquity := before.Frozen + before.Available + before.Settled - before.Recoverable
	afterEquity := after.Frozen + after.Available + after.Settled - after.Recoverable
	if beforeEquity-afterEquity != plan.OriginalReward.AmountCents || before.Settled != after.Settled {
		return before, ErrRewardReversalInvariantViolation
	}
	return after, nil
}

func referralRewardReversalKey(refundTaskID, originalRewardID string) string {
	return "referral-reward-reversal:" + refundTaskID + ":" + originalRewardID
}

func referralRewardReversalLedgerKey(refundTaskID, originalRewardID string) string {
	return "referral-wallet-reversal:" + refundTaskID + ":" + originalRewardID
}

func reversalTimestampMetadata(now time.Time) JSONSnapshot {
	return JSONSnapshot{"recordedAt": now.UTC().Format(time.RFC3339Nano)}
}
