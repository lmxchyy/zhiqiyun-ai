package operationcenter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestReferralRewardGrantApprovalAndConservationPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Run("operation center reward is frozen from pinned terms", func(t *testing.T) {
		prefix := fmt.Sprintf("grant_oc_%d", time.Now().UnixNano())
		beneficiaryID := prefix + "_center"
		fixture := createWorkflowFixtureWithRelationship(t, ctx, db, prefix, fmt.Sprintf(`{"referrerType":"OPERATION_CENTER","referrerUserId":"%s","referrerOperationCenterUserId":"%s"}`, beneficiaryID, beneficiaryID))
		seedEligibilityUser(t, ctx, db, beneficiaryID)
		ruleID := seedRewardGrantRule(t, ctx, db, fixture, "OC_GRANT", ReferralReferrerOperationCenter, ReferralBeneficiaryOperationCenter, ReferralRelationReferrer, 310000, 9)
		service := mustWorkflowService(t, db, WorkflowOptions{})
		paid, err := service.RecordPaymentSucceeded(ctx, PaymentSucceededCommand{OrderID: fixture.orderID, PaymentRecordID: fixture.paymentID})
		if err != nil {
			t.Fatal(err)
		}
		command := ReviewCommand{ServiceOrderID: paid.ServiceOrder.ID, Decision: ReviewApproved, ExpectedStatus: OperationCenterServiceReviewRequired, IdempotencyKey: fixture.id("review"), ReviewedBy: fixture.reviewerID}
		if _, err := service.Review(ctx, command); err != nil {
			t.Fatal(err)
		}
		assertRewardGrantConservation(t, ctx, db, fixture, 1, 310000)
		assertRewardGrantReference(t, ctx, db, fixture.orderID, ruleID, beneficiaryID, 310000, 9)
		if _, err := service.Review(ctx, command); err != nil {
			t.Fatal(err)
		}
		assertRewardGrantConservation(t, ctx, db, fixture, 1, 310000)
	})

	t.Run("agent referral creates direct agent and center rewards only", func(t *testing.T) {
		prefix := fmt.Sprintf("grant_agent_%d", time.Now().UnixNano())
		agentID, centerID, ancestorID := prefix+"_agent", prefix+"_center", prefix+"_ancestor"
		fixture := createWorkflowFixtureWithRelationship(t, ctx, db, prefix, fmt.Sprintf(`{"referrerType":"AGENT","directAgentUserId":"%s","referrerOperationCenterUserId":"%s","parentAgentUserId":"%s"}`, agentID, centerID, ancestorID))
		seedEligibilityUser(t, ctx, db, agentID)
		seedEligibilityUser(t, ctx, db, centerID)
		seedRewardGrantRule(t, ctx, db, fixture, "AGENT_GRANT", ReferralReferrerAgent, ReferralBeneficiaryAgent, ReferralRelationReferrer, 100000, 7)
		seedRewardGrantRule(t, ctx, db, fixture, "CENTER_GRANT", ReferralReferrerAgent, ReferralBeneficiaryOperationCenter, ReferralRelationReferrerOperationCenter, 200000, 7)
		service := mustWorkflowService(t, db, WorkflowOptions{})
		paid, err := service.RecordPaymentSucceeded(ctx, PaymentSucceededCommand{OrderID: fixture.orderID, PaymentRecordID: fixture.paymentID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Review(ctx, ReviewCommand{ServiceOrderID: paid.ServiceOrder.ID, Decision: ReviewApproved, ExpectedStatus: OperationCenterServiceReviewRequired, IdempotencyKey: fixture.id("review"), ReviewedBy: fixture.reviewerID}); err != nil {
			t.Fatal(err)
		}
		assertRewardGrantConservation(t, ctx, db, fixture, 2, 300000)
		var ancestorRewards int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_referral_rewards WHERE beneficiary_user_id=$1`, ancestorID).Scan(&ancestorRewards); err != nil || ancestorRewards != 0 {
			t.Fatalf("ancestor rewards=%d err=%v", ancestorRewards, err)
		}
	})
}

func TestReferralRewardGrantPinnedHistoryConcurrencyAndRollbackPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	t.Run("new rule version does not change historical eligibility", func(t *testing.T) {
		fixture, eventID, beneficiaryID, oldRuleID := prepareEligibilityOnlyGrantFixture(t, ctx, db, "grant_history", 125000, 5)
		newRuleSetID := fixture.id("rules_v2")
		if _, err := db.ExecContext(ctx, `INSERT INTO xz_commercial_rule_sets(id,tenant_id,rule_code,version,name,status,effective_start_at,published_by,published_at) VALUES($1,$2,$3,2,'Grant v2','PUBLISHED',now(),$4,now())`, newRuleSetID, fixture.tenantID, fixture.prefix, fixture.reviewerID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM xz_referral_reward_rule_versions WHERE rule_set_id=$1`, newRuleSetID)
			_, _ = db.ExecContext(context.Background(), `DELETE FROM xz_commercial_rule_sets WHERE id=$1`, newRuleSetID)
		})
		if _, err := db.ExecContext(ctx, `INSERT INTO xz_referral_reward_rule_versions(id,tenant_id,rule_set_id,rule_code,version,referrer_type,beneficiary_type,beneficiary_relation,amount_cents,freeze_days,status) VALUES($1,$2,$3,$4,2,'OPERATION_CENTER','OPERATION_CENTER','REFERRER',999999,30,'PUBLISHED')`, fixture.id("new_rule"), fixture.tenantID, newRuleSetID, fixture.id("new_code")); err != nil {
			t.Fatal(err)
		}
		grantReferralEvent(t, ctx, db, eventID)
		assertRewardGrantReference(t, ctx, db, fixture.orderID, oldRuleID, beneficiaryID, 125000, 5)
	})

	t.Run("concurrent grants credit once", func(t *testing.T) {
		fixture, eventID, _, _ := prepareEligibilityOnlyGrantFixture(t, ctx, db, "grant_concurrent", 88000, 4)
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tx, err := db.BeginTx(ctx, nil)
				if err != nil {
					errs <- err
					return
				}
				store, err := NewPostgresStore(db)
				if err == nil {
					_, err = NewPostgresReferralRewardGrantService(store).CreateRewardsForReferralEvent(ctx, tx, eventID)
				}
				if err == nil {
					err = tx.Commit()
				} else {
					_ = tx.Rollback()
				}
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		assertRewardGrantConservation(t, ctx, db, fixture, 1, 88000)
	})

	t.Run("wallet ledger conflict rolls back reward and eligibility", func(t *testing.T) {
		fixture, eventID, beneficiaryID, _ := prepareEligibilityOnlyGrantFixture(t, ctx, db, "grant_rollback", 77000, 3)
		var eligibilityID string
		if err := db.QueryRowContext(ctx, `SELECT id FROM xz_referral_eligibilities WHERE referral_event_id=$1`, eventID).Scan(&eligibilityID); err != nil {
			t.Fatal(err)
		}
		rewardKey := "operation-center-referral-reward:" + eligibilityID
		ledgerKey := "referral-wallet-frozen:" + rewardKey
		accountID := referralWalletAccountID(fixture.tenantID, ReferralBeneficiaryOperationCenter, beneficiaryID)
		if _, err := db.ExecContext(ctx, `INSERT INTO xz_commission_wallet_accounts(id,tenant_id,beneficiary_type,beneficiary_id,frozen_cents) VALUES($1,$2,'OPERATION_CENTER',$3,1)`, accountID, fixture.tenantID, beneficiaryID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO xz_commission_wallet_ledger(id,tenant_id,account_id,beneficiary_type,beneficiary_id,business_type,business_id,direction,frozen_delta_cents,balances_before,balances_after,idempotency_key) VALUES($1,$2,$3,'OPERATION_CENTER',$4,'TEST_CONFLICT',$5,'CREDIT',1,'{}','{"Frozen":1}',$6)`, fixture.id("conflict_ledger"), fixture.tenantID, accountID, beneficiaryID, fixture.orderID, ledgerKey); err != nil {
			t.Fatal(err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		store, _ := NewPostgresStore(db)
		_, err = NewPostgresReferralRewardGrantService(store).CreateRewardsForReferralEvent(ctx, tx, eventID)
		if !errors.Is(err, ErrWalletCreditConflict) {
			_ = tx.Rollback()
			t.Fatalf("wallet conflict error=%v", err)
		}
		_ = tx.Rollback()
		assertGrantRollbackState(t, ctx, db, eventID, eligibilityID, accountID, 1)
	})

	t.Run("invalid wallet tenant fails without partial data", func(t *testing.T) {
		fixture, eventID, beneficiaryID, _ := prepareEligibilityOnlyGrantFixture(t, ctx, db, "grant_bad_tenant", 66000, 2)
		otherTenant := fixture.id("other_tenant")
		if _, err := db.ExecContext(ctx, `INSERT INTO xz_tenants(id,tenant_type,name,status) SELECT $1,tenant_type,$1,'ACTIVE' FROM xz_tenants WHERE id=$2`, otherTenant, fixture.tenantID); err != nil {
			t.Fatal(err)
		}
		accountID := referralWalletAccountID(fixture.tenantID, ReferralBeneficiaryOperationCenter, beneficiaryID)
		if _, err := db.ExecContext(ctx, `INSERT INTO xz_commission_wallet_accounts(id,tenant_id,beneficiary_type,beneficiary_id) VALUES($1,$2,'OPERATION_CENTER',$3)`, accountID, otherTenant, fixture.id("foreign_beneficiary")); err != nil {
			t.Fatal(err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		store, _ := NewPostgresStore(db)
		_, err = NewPostgresReferralRewardGrantService(store).CreateRewardsForReferralEvent(ctx, tx, eventID)
		if !errors.Is(err, ErrWalletCreditConflict) {
			_ = tx.Rollback()
			t.Fatalf("invalid tenant wallet error=%v", err)
		}
		_ = tx.Rollback()
		var rewards int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_referral_rewards WHERE referral_event_id=$1`, eventID).Scan(&rewards); err != nil || rewards != 0 {
			t.Fatalf("invalid wallet left rewards=%d err=%v", rewards, err)
		}
	})
}

func prepareEligibilityOnlyGrantFixture(t *testing.T, ctx context.Context, db *sql.DB, name string, amount int64, freezeDays int) (workflowFixture, string, string, string) {
	t.Helper()
	prefix := fmt.Sprintf("%s_%d", name, time.Now().UnixNano())
	beneficiaryID := prefix + "_center"
	fixture := createWorkflowFixtureWithRelationship(t, ctx, db, prefix, fmt.Sprintf(`{"referrerType":"OPERATION_CENTER","referrerUserId":"%s","referrerOperationCenterUserId":"%s"}`, beneficiaryID, beneficiaryID))
	seedEligibilityUser(t, ctx, db, beneficiaryID)
	ruleID := seedRewardGrantRule(t, ctx, db, fixture, "PINNED_GRANT", ReferralReferrerOperationCenter, ReferralBeneficiaryOperationCenter, ReferralRelationReferrer, amount, freezeDays)
	service := mustWorkflowService(t, db, WorkflowOptions{ReferralRewardGrantHook: NoopReferralRewardGrantHook{}})
	paid, err := service.RecordPaymentSucceeded(ctx, PaymentSucceededCommand{OrderID: fixture.orderID, PaymentRecordID: fixture.paymentID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Review(ctx, ReviewCommand{ServiceOrderID: paid.ServiceOrder.ID, Decision: ReviewApproved, ExpectedStatus: OperationCenterServiceReviewRequired, IdempotencyKey: fixture.id("review"), ReviewedBy: fixture.reviewerID}); err != nil {
		t.Fatal(err)
	}
	var eventID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM xz_referral_events WHERE source_order_id=$1 AND status='READY'`, fixture.orderID).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	return fixture, eventID, beneficiaryID, ruleID
}

func seedRewardGrantRule(t *testing.T, ctx context.Context, db *sql.DB, fixture workflowFixture, code string, referrer ReferralReferrerType, beneficiary ReferralBeneficiaryType, relation ReferralBeneficiaryRelation, amount int64, freezeDays int) string {
	t.Helper()
	id := fixture.id("grant_rule_" + code)
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_referral_reward_rule_versions(id,tenant_id,rule_set_id,rule_code,version,referrer_type,beneficiary_type,beneficiary_relation,amount_cents,freeze_days,status) VALUES($1,$2,$3,$4,1,$5,$6,$7,$8,$9,'PUBLISHED')`, id, fixture.tenantID, fixture.ruleSetID, fixture.id(code), referrer, beneficiary, relation, amount, freezeDays); err != nil {
		t.Fatal(err)
	}
	return id
}

func grantReferralEvent(t *testing.T, ctx context.Context, db *sql.DB, eventID string) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewPostgresStore(db)
	if err == nil {
		_, err = NewPostgresReferralRewardGrantService(store).CreateRewardsForReferralEvent(ctx, tx, eventID)
	}
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func assertRewardGrantConservation(t *testing.T, ctx context.Context, db *sql.DB, fixture workflowFixture, rewardCount int, amount int64) {
	t.Helper()
	var eventStatus string
	var eligibilityCount, consumedCount, rewards, ledgers, tasks, legacyCount int
	var rewardSum, ledgerSum, walletSum, availableSum, settledSum, recoverableSum int64
	if err := db.QueryRowContext(ctx, `SELECT status FROM xz_referral_events WHERE source_order_id=$1`, fixture.orderID).Scan(&eventStatus); err != nil {
		t.Fatal(err)
	}
	query := `
		SELECT count(*),count(*) FILTER(WHERE eligibility_status='CONSUMED')
		FROM xz_referral_eligibilities WHERE referral_event_id=(SELECT id FROM xz_referral_events WHERE source_order_id=$1)
	`
	if err := db.QueryRowContext(ctx, query, fixture.orderID).Scan(&eligibilityCount, &consumedCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(amount_cents),0) FROM xz_referral_rewards WHERE referral_event_id=(SELECT id FROM xz_referral_events WHERE source_order_id=$1) AND record_type='REWARD'`, fixture.orderID).Scan(&rewards, &rewardSum); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(frozen_delta_cents),0) FROM xz_commission_wallet_ledger WHERE referral_event_id=(SELECT id FROM xz_referral_events WHERE source_order_id=$1) AND business_type='REFERRAL_REWARD_GRANT'`, fixture.orderID).Scan(&ledgers, &ledgerSum); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT coalesce(sum(account.frozen_cents),0),coalesce(sum(account.available_cents),0),coalesce(sum(account.settled_cents),0),coalesce(sum(account.recoverable_cents),0) FROM xz_commission_wallet_accounts account WHERE (account.tenant_id,account.beneficiary_type,account.beneficiary_id) IN (SELECT tenant_id,beneficiary_type,beneficiary_user_id FROM xz_referral_eligibilities WHERE referral_event_id=(SELECT id FROM xz_referral_events WHERE source_order_id=$1))`, fixture.orderID).Scan(&walletSum, &availableSum, &settledSum, &recoverableSum); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_referral_reward_release_tasks WHERE referral_reward_id IN (SELECT id FROM xz_referral_rewards WHERE referral_event_id=(SELECT id FROM xz_referral_events WHERE source_order_id=$1))`, fixture.orderID).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_commissions`).Scan(&legacyCount); err != nil {
		t.Fatal(err)
	}
	if eventStatus != ReferralEventRewarded || eligibilityCount != rewardCount || consumedCount != rewardCount || rewards != rewardCount || ledgers != rewardCount || tasks != rewardCount {
		t.Fatalf("grant counts status=%s elig=%d consumed=%d rewards=%d ledgers=%d tasks=%d", eventStatus, eligibilityCount, consumedCount, rewards, ledgers, tasks)
	}
	if rewardSum != amount || ledgerSum != amount || walletSum != amount || availableSum != 0 || settledSum != 0 || recoverableSum != 0 {
		t.Fatalf("conservation reward=%d ledger=%d wallet=%d available=%d settled=%d recoverable=%d legacy=%d", rewardSum, ledgerSum, walletSum, availableSum, settledSum, recoverableSum, legacyCount)
	}
}

func assertRewardGrantReference(t *testing.T, ctx context.Context, db *sql.DB, orderID, ruleID, beneficiaryID string, amount int64, freezeDays int) {
	t.Helper()
	var gotRule, gotBeneficiary, status, eligibilityStatus, releaseStatus string
	var gotAmount int64
	var freezeUntil, createdAt, executeAt time.Time
	err := db.QueryRowContext(ctx, `
		SELECT reward.reward_rule_id,reward.beneficiary_user_id,reward.amount_cents,reward.status,
		       reward.created_at,reward.freeze_until,eligibility.eligibility_status,task.release_status,task.execute_at
		FROM xz_referral_rewards reward
		JOIN xz_referral_eligibilities eligibility ON eligibility.id=reward.referral_eligibility_id AND eligibility.reward_id=reward.id
		JOIN xz_referral_reward_release_tasks task ON task.id=reward.current_release_task_id
		WHERE reward.referral_event_id=(SELECT id FROM xz_referral_events WHERE source_order_id=$1)
		  AND reward.reward_rule_id=$2
	`, orderID, ruleID).Scan(&gotRule, &gotBeneficiary, &gotAmount, &status, &createdAt, &freezeUntil, &eligibilityStatus, &releaseStatus, &executeAt)
	if err != nil {
		t.Fatal(err)
	}
	if gotRule != ruleID || gotBeneficiary != beneficiaryID || gotAmount != amount || status != "FROZEN" || eligibilityStatus != "CONSUMED" || releaseStatus != "PENDING" || !executeAt.Equal(freezeUntil) || !freezeUntil.Equal(createdAt.AddDate(0, 0, freezeDays)) {
		t.Fatalf("reward reference rule=%s beneficiary=%s amount=%d status=%s eligibility=%s release=%s created=%s freeze=%s execute=%s", gotRule, gotBeneficiary, gotAmount, status, eligibilityStatus, releaseStatus, createdAt, freezeUntil, executeAt)
	}
}

func assertGrantRollbackState(t *testing.T, ctx context.Context, db *sql.DB, eventID, eligibilityID, accountID string, frozen int64) {
	t.Helper()
	var eventStatus, eligibilityStatus string
	var rewardID sql.NullString
	var rewardCount, taskCount int
	var frozenCents int64
	if err := db.QueryRowContext(ctx, `SELECT status FROM xz_referral_events WHERE id=$1`, eventID).Scan(&eventStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT eligibility_status,reward_id FROM xz_referral_eligibilities WHERE id=$1`, eligibilityID).Scan(&eligibilityStatus, &rewardID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_referral_rewards WHERE referral_event_id=$1`, eventID).Scan(&rewardCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_referral_reward_release_tasks WHERE referral_reward_id IN (SELECT id FROM xz_referral_rewards WHERE referral_event_id=$1)`, eventID).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT frozen_cents FROM xz_commission_wallet_accounts WHERE id=$1`, accountID).Scan(&frozenCents); err != nil {
		t.Fatal(err)
	}
	if eventStatus != "READY" || eligibilityStatus != "ELIGIBLE" || rewardID.Valid || rewardCount != 0 || taskCount != 0 || frozenCents != frozen {
		t.Fatalf("rollback event=%s eligibility=%s rewardID=%v rewards=%d tasks=%d frozen=%d", eventStatus, eligibilityStatus, rewardID, rewardCount, taskCount, frozenCents)
	}
}
