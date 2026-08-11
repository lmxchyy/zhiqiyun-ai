package operationcenter

import (
	"errors"
	"testing"
)

func TestResolveReferralBeneficiariesSupportsOnlyConfirmedRelations(t *testing.T) {
	agentSnapshot := referralRelationshipSnapshot{
		ReferrerType: ReferralReferrerAgent, ReferrerUserID: "agent-1",
		ReferrerOperationCenterUserID: "center-1",
	}
	agentRules := []ReferralEligibilityRule{
		{ID: "rule-agent", ReferrerType: ReferralReferrerAgent, BeneficiaryType: ReferralBeneficiaryAgent, BeneficiaryRelation: ReferralRelationReferrer},
		{ID: "rule-center", ReferrerType: ReferralReferrerAgent, BeneficiaryType: ReferralBeneficiaryOperationCenter, BeneficiaryRelation: ReferralRelationReferrerOperationCenter},
	}
	resolved, err := resolveReferralBeneficiaries(agentSnapshot, agentRules)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 2 || resolved[0].BeneficiaryUserID != "agent-1" || resolved[1].BeneficiaryUserID != "center-1" {
		t.Fatalf("agent referral beneficiaries=%+v", resolved)
	}

	centerSnapshot := referralRelationshipSnapshot{ReferrerType: ReferralReferrerOperationCenter, ReferrerUserID: "center-2", ReferrerOperationCenterUserID: "center-2"}
	centerRules := []ReferralEligibilityRule{{ID: "rule-center-center", ReferrerType: ReferralReferrerOperationCenter, BeneficiaryType: ReferralBeneficiaryOperationCenter, BeneficiaryRelation: ReferralRelationReferrer}}
	resolved, err = resolveReferralBeneficiaries(centerSnapshot, centerRules)
	if err != nil || len(resolved) != 1 || resolved[0].BeneficiaryUserID != "center-2" {
		t.Fatalf("operation center referral beneficiaries=%+v err=%v", resolved, err)
	}
}

func TestResolveReferralBeneficiariesRejectsAncestorOrMissingSnapshot(t *testing.T) {
	snapshot := referralRelationshipSnapshot{ReferrerType: ReferralReferrerAgent, ReferrerUserID: "agent-1"}
	_, err := resolveReferralBeneficiaries(snapshot, []ReferralEligibilityRule{{
		ID: "rule-invalid", ReferrerType: ReferralReferrerAgent,
		BeneficiaryType:     ReferralBeneficiaryOperationCenter,
		BeneficiaryRelation: ReferralRelationReferrerOperationCenter,
	}})
	if !errors.Is(err, ErrReferralRelationshipSnapshotInvalid) {
		t.Fatalf("missing operation center error=%v", err)
	}
	_, err = resolveReferralBeneficiaries(snapshot, []ReferralEligibilityRule{{
		ID: "rule-ancestor", ReferrerType: ReferralReferrerAgent,
		BeneficiaryType:     ReferralBeneficiaryAgent,
		BeneficiaryRelation: ReferralBeneficiaryRelation("ANCESTOR_AGENT"),
	}})
	if !errors.Is(err, ErrReferralRuleUnsupported) {
		t.Fatalf("ancestor rule error=%v", err)
	}
}

func TestReferralRelationshipSnapshotUsesPaymentSnapshotKeys(t *testing.T) {
	snapshot, err := parseReferralRelationshipSnapshot(JSONSnapshot{
		"referrerType": "AGENT", "directAgentUserId": "agent-1",
		"referrerOperationCenterUserId": "center-1", "parentAgentUserId": "must-not-be-used",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ReferrerUserID != "agent-1" || snapshot.ReferrerOperationCenterUserID != "center-1" {
		t.Fatalf("snapshot=%+v", snapshot)
	}
}
