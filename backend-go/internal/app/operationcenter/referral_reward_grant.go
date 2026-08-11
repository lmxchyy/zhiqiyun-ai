package operationcenter

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrReferralEventNotReady       = errors.New("referral event is not ready")
	ErrEligibilityAlreadyConsumed  = errors.New("referral eligibility is already consumed inconsistently")
	ErrReferralRuleSnapshotMissing = errors.New("referral reward rule snapshot is missing")
	ErrRewardAmountInvalid         = errors.New("referral reward amount is invalid")
	ErrRewardGrantConflict         = errors.New("referral reward grant conflicts with an existing grant")
	ErrWalletCreditConflict        = errors.New("referral reward wallet credit conflicts with an existing credit")
	ErrRewardReleaseTaskConflict   = errors.New("referral reward release task conflicts with an existing task")
	ErrReferralWalletTenantInvalid = errors.New("referral reward wallet tenant is invalid")
)

const (
	ReferralEventReady    = "READY"
	ReferralEventRewarded = "REWARDED"
)

type ReferralRewardStatus string

const (
	ReferralRewardFrozen ReferralRewardStatus = "FROZEN"
)

type ReferralReward struct {
	ID, TenantID, ReferralEventID, ReferralEligibilityID string
	ReferralRuleID, CommercialRuleSetID                  string
	ReferralRuleVersion                                  int
	BeneficiaryType                                      ReferralBeneficiaryType
	BeneficiaryUserID                                    string
	BeneficiaryRelation                                  ReferralBeneficiaryRelation
	AmountCents                                          int64
	Status                                               ReferralRewardStatus
	FreezeUntil                                          time.Time
	RelationshipSnapshot                                 JSONSnapshot
	IdempotencyKey                                       string
	GrantWalletLedgerID, ReleaseWalletLedgerID           string
	CurrentReleaseTaskID                                 string
	RefundTaskID, ReversalWalletLedgerID                 string
	CreatedAt, UpdatedAt                                 time.Time
}

type ReferralRewardRuleSnapshot struct {
	ID, TenantID, RuleSetID string
	RuleSetVersion, Version int
	BeneficiaryType         ReferralBeneficiaryType
	BeneficiaryRelation     ReferralBeneficiaryRelation
	AmountCents             int64
	FreezeDays              int
}

type referralRewardGrantPlan struct {
	Eligibility ReferralEligibility
	Rule        ReferralRewardRuleSnapshot
	Reward      ReferralReward
}

func buildReferralRewardGrantPlan(event ReferralEvent, eligibility ReferralEligibility, rule ReferralRewardRuleSnapshot, now time.Time) (referralRewardGrantPlan, error) {
	if event.ID == "" || event.TenantID == "" || eligibility.ReferralEventID != event.ID || eligibility.TenantID != event.TenantID {
		return referralRewardGrantPlan{}, ErrReferralWalletTenantInvalid
	}
	if eligibility.Status != ReferralEligibilityEligible {
		return referralRewardGrantPlan{}, ErrEligibilityAlreadyConsumed
	}
	if rule.ID == "" || rule.RuleSetID == "" || rule.Version <= 0 || rule.RuleSetVersion <= 0 ||
		rule.ID != eligibility.ReferralRuleVersionID || rule.RuleSetID != eligibility.CommercialRuleSetID ||
		rule.Version != eligibility.ReferralRuleVersion || rule.RuleSetVersion != eligibility.CommercialRuleSetVersion ||
		rule.TenantID != eligibility.TenantID || rule.BeneficiaryType != eligibility.BeneficiaryType ||
		rule.BeneficiaryRelation != eligibility.BeneficiaryRelation {
		return referralRewardGrantPlan{}, ErrReferralRuleSnapshotMissing
	}
	if rule.AmountCents <= 0 || rule.FreezeDays < 0 {
		return referralRewardGrantPlan{}, ErrRewardAmountInvalid
	}
	rewardKey := "operation-center-referral-reward:" + eligibility.ID
	reward := ReferralReward{
		ID: stableWorkflowID("referral_reward", eligibility.ID), TenantID: eligibility.TenantID,
		ReferralEventID: event.ID, ReferralEligibilityID: eligibility.ID,
		ReferralRuleID: rule.ID, ReferralRuleVersion: rule.Version, CommercialRuleSetID: rule.RuleSetID,
		BeneficiaryType: eligibility.BeneficiaryType, BeneficiaryUserID: eligibility.BeneficiaryUserID,
		BeneficiaryRelation: eligibility.BeneficiaryRelation, AmountCents: rule.AmountCents,
		Status: ReferralRewardFrozen, FreezeUntil: now.AddDate(0, 0, rule.FreezeDays),
		RelationshipSnapshot: eligibility.RelationshipSnapshot, IdempotencyKey: rewardKey,
		CreatedAt: now, UpdatedAt: now,
	}
	if reward.BeneficiaryUserID == "" {
		return referralRewardGrantPlan{}, fmt.Errorf("%w: beneficiary is empty", ErrReferralRuleSnapshotMissing)
	}
	return referralRewardGrantPlan{Eligibility: eligibility, Rule: rule, Reward: reward}, nil
}
