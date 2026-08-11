package operationcenter

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

func TestOperationCenterReferralEligibilityScenariosAndHistoryPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	t.Run("operation center recommends operation center", func(t *testing.T) {
		prefix := fmt.Sprintf("elig_oc_%d", time.Now().UnixNano())
		referrerID := prefix + "_referrer_center"
		fixture := createWorkflowFixtureWithRelationship(t, ctx, db, prefix, fmt.Sprintf(`{"referrerType":"OPERATION_CENTER","referrerUserId":"%s","referrerOperationCenterUserId":"%s"}`, referrerID, referrerID))
		seedEligibilityUser(t, ctx, db, referrerID)
		ruleID := seedEligibilityRule(t, ctx, db, fixture, "OC_REFERS_OC", 1, ReferralReferrerOperationCenter, ReferralBeneficiaryOperationCenter, ReferralRelationReferrer)
		service := mustWorkflowService(t, db, WorkflowOptions{ReferralRewardGrantHook: NoopReferralRewardGrantHook{}})
		paid, err := service.RecordPaymentSucceeded(ctx, PaymentSucceededCommand{OrderID: fixture.orderID, PaymentRecordID: fixture.paymentID})
		if err != nil {
			t.Fatal(err)
		}
		command := ReviewCommand{ServiceOrderID: paid.ServiceOrder.ID, Decision: ReviewApproved, ExpectedStatus: OperationCenterServiceReviewRequired, IdempotencyKey: fixture.id("review-key"), ReviewedBy: fixture.reviewerID}
		if _, err := service.Review(ctx, command); err != nil {
			t.Fatal(err)
		}
		assertEligibilityResult(t, db, fixture, 1, 1, 0, 0)
		assertEligibilityReference(t, db, fixture, ruleID, referrerID, ReferralBeneficiaryOperationCenter)
		if _, err := service.Review(ctx, command); err != nil {
			t.Fatal(err)
		}
		assertEligibilityResult(t, db, fixture, 1, 1, 0, 0)

		newRuleSetID := fixture.id("rules-v2")
		if _, err := db.ExecContext(ctx, `INSERT INTO xz_commercial_rule_sets(id,tenant_id,rule_code,version,name,status,effective_start_at,published_by,published_at) VALUES($1,$2,$3,2,'Eligibility v2','PUBLISHED',now(),$4,now())`, newRuleSetID, fixture.tenantID, fixture.prefix, fixture.reviewerID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := db.ExecContext(ctx, `DELETE FROM xz_referral_reward_rule_versions WHERE rule_set_id=$1`, newRuleSetID); err != nil {
				t.Errorf("clean eligibility v2 referral rules: %v", err)
				return
			}
			if _, err := db.ExecContext(ctx, `DELETE FROM xz_commercial_rule_sets WHERE id=$1`, newRuleSetID); err != nil {
				t.Errorf("clean eligibility v2 rule set: %v", err)
			}
		})
		if _, err := db.ExecContext(ctx, `INSERT INTO xz_referral_reward_rule_versions(id,tenant_id,rule_set_id,rule_code,version,referrer_type,beneficiary_type,beneficiary_relation,amount_cents,status) VALUES($1,$2,$3,$4,2,'OPERATION_CENTER','OPERATION_CENTER','REFERRER',400000,'PUBLISHED')`, fixture.id("new-rule"), fixture.tenantID, newRuleSetID, fixture.id("OC_REFERS_OC")); err != nil {
			t.Fatal(err)
		}
		assertEligibilityReference(t, db, fixture, ruleID, referrerID, ReferralBeneficiaryOperationCenter)
	})

	t.Run("agent recommends operation center", func(t *testing.T) {
		prefix := fmt.Sprintf("elig_agent_%d", time.Now().UnixNano())
		agentID := prefix + "_agent"
		centerID := prefix + "_agent_center"
		fixture := createWorkflowFixtureWithRelationship(t, ctx, db, prefix, fmt.Sprintf(`{"referrerType":"AGENT","directAgentUserId":"%s","referrerOperationCenterUserId":"%s","parentAgentUserId":"%s"}`, agentID, centerID, prefix+"_ancestor_must_not_receive"))
		seedEligibilityUser(t, ctx, db, agentID)
		seedEligibilityUser(t, ctx, db, centerID)
		agentRuleID := seedEligibilityRule(t, ctx, db, fixture, "AGENT_REFERS_OC_AGENT", 1, ReferralReferrerAgent, ReferralBeneficiaryAgent, ReferralRelationReferrer)
		centerRuleID := seedEligibilityRule(t, ctx, db, fixture, "AGENT_REFERS_OC_CENTER", 1, ReferralReferrerAgent, ReferralBeneficiaryOperationCenter, ReferralRelationReferrerOperationCenter)
		service := mustWorkflowService(t, db, WorkflowOptions{ReferralRewardGrantHook: NoopReferralRewardGrantHook{}})
		paid, err := service.RecordPaymentSucceeded(ctx, PaymentSucceededCommand{OrderID: fixture.orderID, PaymentRecordID: fixture.paymentID})
		if err != nil {
			t.Fatal(err)
		}
		command := ReviewCommand{ServiceOrderID: paid.ServiceOrder.ID, Decision: ReviewApproved, ExpectedStatus: OperationCenterServiceReviewRequired, IdempotencyKey: fixture.id("review-key"), ReviewedBy: fixture.reviewerID}
		if _, err := service.Review(ctx, command); err != nil {
			t.Fatal(err)
		}
		assertEligibilityResult(t, db, fixture, 1, 2, 0, 0)
		assertEligibilityReference(t, db, fixture, agentRuleID, agentID, ReferralBeneficiaryAgent)
		assertEligibilityReference(t, db, fixture, centerRuleID, centerID, ReferralBeneficiaryOperationCenter)
		var ancestorEligibility int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_referral_eligibilities WHERE beneficiary_user_id=$1`, prefix+"_ancestor_must_not_receive").Scan(&ancestorEligibility); err != nil || ancestorEligibility != 0 {
			t.Fatalf("ancestor eligibility=%d err=%v", ancestorEligibility, err)
		}
		if _, err := service.Review(ctx, command); err != nil {
			t.Fatal(err)
		}
		assertEligibilityResult(t, db, fixture, 1, 2, 0, 0)
	})
}

func seedEligibilityUser(t *testing.T, ctx context.Context, db *sql.DB, userID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users(id,email,name,role,status,operation_center_status,created_at,updated_at,raw) VALUES($1,$2,$1,'USER','ACTIVE','NONE',$3,$3,'{}'::jsonb)`, userID, userID+"@example.test", now); err != nil {
		t.Fatal(err)
	}
}

func seedEligibilityRule(t *testing.T, ctx context.Context, db *sql.DB, fixture workflowFixture, code string, version int, referrer ReferralReferrerType, beneficiary ReferralBeneficiaryType, relation ReferralBeneficiaryRelation) string {
	t.Helper()
	id := fixture.id("rule_" + code)
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_referral_reward_rule_versions(id,tenant_id,rule_set_id,rule_code,version,referrer_type,beneficiary_type,beneficiary_relation,amount_cents,freeze_days,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,100,7,'PUBLISHED')`, id, fixture.tenantID, fixture.ruleSetID, fixture.id(code), version, referrer, beneficiary, relation); err != nil {
		t.Fatal(err)
	}
	return id
}

func assertEligibilityResult(t *testing.T, db *sql.DB, fixture workflowFixture, events, eligibilities, rewards, wallet int) {
	t.Helper()
	checks := []struct {
		query string
		want  int
	}{
		{`SELECT count(*) FROM xz_referral_events WHERE source_order_id=$1`, events},
		{`SELECT count(*) FROM xz_referral_eligibilities WHERE referral_event_id IN (SELECT id FROM xz_referral_events WHERE source_order_id=$1)`, eligibilities},
		{`SELECT count(*) FROM xz_referral_rewards WHERE referral_event_id IN (SELECT id FROM xz_referral_events WHERE source_order_id=$1)`, rewards},
		{`SELECT count(*) FROM xz_commission_wallet_ledger WHERE business_id=$1`, wallet},
		{`SELECT count(*) FROM xz_referral_reward_release_tasks WHERE referral_reward_id IN (SELECT id FROM xz_referral_rewards WHERE referral_event_id IN (SELECT id FROM xz_referral_events WHERE source_order_id=$1))`, 0},
	}
	for _, check := range checks {
		var got int
		if err := db.QueryRow(check.query, fixture.orderID).Scan(&got); err != nil || got != check.want {
			t.Fatalf("query=%s got=%d want=%d err=%v", check.query, got, check.want, err)
		}
	}
}

func assertEligibilityReference(t *testing.T, db *sql.DB, fixture workflowFixture, ruleID, beneficiaryID string, beneficiaryType ReferralBeneficiaryType) {
	t.Helper()
	var gotRuleID, gotRuleSetID, gotBeneficiaryID, gotBeneficiaryType string
	var gotRuleSetVersion, gotRuleVersion int
	var relationAgent string
	err := db.QueryRow(`
		SELECT eligibility.referral_rule_version_id,eligibility.commercial_rule_set_id,
		       eligibility.commercial_rule_set_version,eligibility.referral_rule_version,
		       eligibility.beneficiary_user_id,eligibility.beneficiary_type,
		       coalesce(eligibility.relationship_snapshot->>'directAgentUserId','')
		FROM xz_referral_eligibilities eligibility
		JOIN xz_referral_events event ON event.id=eligibility.referral_event_id
		WHERE event.source_order_id=$1 AND eligibility.referral_rule_version_id=$2
	`, fixture.orderID, ruleID).Scan(&gotRuleID, &gotRuleSetID, &gotRuleSetVersion, &gotRuleVersion, &gotBeneficiaryID, &gotBeneficiaryType, &relationAgent)
	if err != nil {
		t.Fatal(err)
	}
	if gotRuleID != ruleID || gotRuleSetID != fixture.ruleSetID || gotRuleSetVersion != 1 || gotRuleVersion != 1 || gotBeneficiaryID != beneficiaryID || gotBeneficiaryType != string(beneficiaryType) {
		t.Fatalf("eligibility reference rule=%s set=%s/%d ruleVersion=%d beneficiary=%s/%s relationAgent=%s", gotRuleID, gotRuleSetID, gotRuleSetVersion, gotRuleVersion, gotBeneficiaryType, gotBeneficiaryID, relationAgent)
	}
}
