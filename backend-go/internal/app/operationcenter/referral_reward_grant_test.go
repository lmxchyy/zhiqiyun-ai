package operationcenter

import (
	"errors"
	"testing"
	"time"
)

func TestBuildReferralRewardGrantPlanUsesPinnedRuleTerms(t *testing.T) {
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	event := ReferralEvent{ID: "event", TenantID: "tenant"}
	eligibility := ReferralEligibility{
		ID: "eligibility", TenantID: "tenant", ReferralEventID: "event",
		CommercialRuleSetID: "rules-v1", CommercialRuleSetVersion: 1,
		ReferralRuleVersionID: "referral-rule-v1", ReferralRuleVersion: 3,
		BeneficiaryType: ReferralBeneficiaryOperationCenter, BeneficiaryUserID: "center",
		BeneficiaryRelation: ReferralRelationReferrer, RelationshipSnapshot: JSONSnapshot{"referrerUserId": "center"},
		Status: ReferralEligibilityEligible,
	}
	rule := ReferralRewardRuleSnapshot{
		ID: "referral-rule-v1", TenantID: "tenant", RuleSetID: "rules-v1", RuleSetVersion: 1, Version: 3,
		BeneficiaryType: ReferralBeneficiaryOperationCenter, BeneficiaryRelation: ReferralRelationReferrer,
		AmountCents: 310000, FreezeDays: 9,
	}
	plan, err := buildReferralRewardGrantPlan(event, eligibility, rule, now)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Reward.AmountCents != rule.AmountCents || !plan.Reward.FreezeUntil.Equal(now.AddDate(0, 0, rule.FreezeDays)) {
		t.Fatalf("reward did not use pinned terms: %+v", plan.Reward)
	}
	if plan.Reward.ReferralRuleVersion != 3 || plan.Reward.CommercialRuleSetID != "rules-v1" {
		t.Fatalf("reward lost pinned rule identity: %+v", plan.Reward)
	}
}

func TestBuildReferralRewardGrantPlanRejectsInvalidOrUnknownRule(t *testing.T) {
	now := time.Now().UTC()
	event := ReferralEvent{ID: "event", TenantID: "tenant"}
	eligibility := ReferralEligibility{
		ID: "eligibility", TenantID: "tenant", ReferralEventID: "event",
		CommercialRuleSetID: "rules", CommercialRuleSetVersion: 1,
		ReferralRuleVersionID: "rule", ReferralRuleVersion: 1,
		BeneficiaryType: ReferralBeneficiaryAgent, BeneficiaryUserID: "agent",
		BeneficiaryRelation: ReferralRelationReferrer, Status: ReferralEligibilityEligible,
	}
	rule := ReferralRewardRuleSnapshot{ID: "unknown", TenantID: "tenant", RuleSetID: "rules", RuleSetVersion: 1, Version: 1, BeneficiaryType: ReferralBeneficiaryAgent, BeneficiaryRelation: ReferralRelationReferrer, AmountCents: 1}
	if _, err := buildReferralRewardGrantPlan(event, eligibility, rule, now); !errors.Is(err, ErrReferralRuleSnapshotMissing) {
		t.Fatalf("unknown rule error=%v", err)
	}
	rule.ID = "rule"
	rule.AmountCents = 0
	if _, err := buildReferralRewardGrantPlan(event, eligibility, rule, now); !errors.Is(err, ErrRewardAmountInvalid) {
		t.Fatalf("invalid amount error=%v", err)
	}
	eligibility.Status = ReferralEligibilityConsumed
	rule.AmountCents = 1
	if _, err := buildReferralRewardGrantPlan(event, eligibility, rule, now); !errors.Is(err, ErrEligibilityAlreadyConsumed) {
		t.Fatalf("consumed eligibility error=%v", err)
	}
}
