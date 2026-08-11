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

func TestReferralRewardReversalFrozenAgentAtomicityPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	prefix := fmt.Sprintf("reverse_agent_%d", time.Now().UnixNano())
	agentID, centerID, ancestorID := prefix+"_agent", prefix+"_center", prefix+"_ancestor"
	fixture := createWorkflowFixtureWithRelationship(t, ctx, db, prefix, fmt.Sprintf(`{"referrerType":"AGENT","directAgentUserId":"%s","referrerOperationCenterUserId":"%s","parentAgentUserId":"%s"}`, agentID, centerID, ancestorID))
	seedEligibilityUser(t, ctx, db, agentID)
	seedEligibilityUser(t, ctx, db, centerID)
	seedRewardGrantRule(t, ctx, db, fixture, "REVERSE_AGENT", ReferralReferrerAgent, ReferralBeneficiaryAgent, ReferralRelationReferrer, 100000, 7)
	seedRewardGrantRule(t, ctx, db, fixture, "REVERSE_CENTER", ReferralReferrerAgent, ReferralBeneficiaryOperationCenter, ReferralRelationReferrerOperationCenter, 200000, 7)
	workflow := mustWorkflowService(t, db, WorkflowOptions{})
	paid, err := workflow.RecordPaymentSucceeded(ctx, PaymentSucceededCommand{OrderID: fixture.orderID, PaymentRecordID: fixture.paymentID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.Review(ctx, ReviewCommand{ServiceOrderID: paid.ServiceOrder.ID, Decision: ReviewApproved, ExpectedStatus: OperationCenterServiceReviewRequired, IdempotencyKey: fixture.id("review"), ReviewedBy: fixture.reviewerID}); err != nil {
		t.Fatal(err)
	}
	var eventID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM xz_referral_events WHERE source_order_id=$1`, fixture.orderID).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	command := createReferralReversalCommand(t, ctx, db, fixture, eventID)
	var legacyBefore int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_commissions`).Scan(&legacyBefore); err != nil {
		t.Fatal(err)
	}
	if _, err := executeReferralReversal(t, ctx, db, command); err != nil {
		t.Fatal(err)
	}

	assertReferralReversalConservation(t, ctx, db, eventID, command.RefundTaskID, 2, 300000, 300000, 0)
	var cancelled, ancestorRecords, legacyAfter int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_referral_reward_release_tasks task JOIN xz_referral_rewards reward ON reward.current_release_task_id=task.id WHERE reward.referral_event_id=$1 AND reward.reversal_of_id IS NULL AND task.release_status='CANCELLED' AND task.cancellation_reason<>'' AND task.cancelled_at IS NOT NULL`, eventID).Scan(&cancelled); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_referral_rewards WHERE referral_event_id=$1 AND beneficiary_user_id=$2`, eventID, ancestorID).Scan(&ancestorRecords); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_commissions`).Scan(&legacyAfter); err != nil {
		t.Fatal(err)
	}
	if cancelled != 2 || ancestorRecords != 0 || legacyAfter != legacyBefore {
		t.Fatalf("frozen reversal cancelled=%d ancestor=%d legacy=%d/%d", cancelled, ancestorRecords, legacyBefore, legacyAfter)
	}
	claimed, err := mustReferralReleaseService(t, db).ClaimDueRewards(ctx, fixture.id("post_reverse_worker"), 10)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("reversed release task was claimable: tasks=%+v err=%v", claimed, err)
	}
	if _, err := executeReferralReversal(t, ctx, db, command); err != nil {
		t.Fatalf("idempotent replay failed: %v", err)
	}
	assertReferralReversalConservation(t, ctx, db, eventID, command.RefundTaskID, 2, 300000, 300000, 0)
}

func TestReferralRewardReversalAvailableAndSettledPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	t.Run("available sufficient", func(t *testing.T) {
		fixture, _, rewardID, accountID, _, amount := prepareFrozenReleaseFixture(t, ctx, db, "reverse_available_full", 120000, 0)
		releaseFrozenRewardForReversal(t, ctx, db, fixture)
		eventID := referralEventIDForReward(t, ctx, db, rewardID)
		command := createReferralReversalCommand(t, ctx, db, fixture, eventID)
		if _, err := executeReferralReversal(t, ctx, db, command); err != nil {
			t.Fatal(err)
		}
		assertWalletBuckets(t, ctx, db, accountID, 0, 0, 0, 0)
		assertReferralReversalConservation(t, ctx, db, eventID, command.RefundTaskID, 1, amount, amount, 0)
	})

	t.Run("available insufficient creates recoverable", func(t *testing.T) {
		fixture, _, rewardID, accountID, _, amount := prepareFrozenReleaseFixture(t, ctx, db, "reverse_available_partial", 150000, 0)
		releaseFrozenRewardForReversal(t, ctx, db, fixture)
		if _, err := db.ExecContext(ctx, `UPDATE xz_commission_wallet_accounts SET available_cents=40000 WHERE id=$1`, accountID); err != nil {
			t.Fatal(err)
		}
		eventID := referralEventIDForReward(t, ctx, db, rewardID)
		command := createReferralReversalCommand(t, ctx, db, fixture, eventID)
		if _, err := executeReferralReversal(t, ctx, db, command); err != nil {
			t.Fatal(err)
		}
		assertWalletBuckets(t, ctx, db, accountID, 0, 0, 0, amount-40000)
		assertReferralReversalConservation(t, ctx, db, eventID, command.RefundTaskID, 1, amount, 40000, amount-40000)
	})

	t.Run("settled history stays immutable and becomes recoverable", func(t *testing.T) {
		fixture, _, rewardID, accountID, _, amount := prepareFrozenReleaseFixture(t, ctx, db, "reverse_settled", 180000, 0)
		releaseFrozenRewardForReversal(t, ctx, db, fixture)
		if _, err := db.ExecContext(ctx, `UPDATE xz_referral_rewards SET status='SETTLED' WHERE id=$1`, rewardID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE xz_commission_wallet_accounts SET available_cents=0,settled_cents=$1 WHERE id=$2`, amount, accountID); err != nil {
			t.Fatal(err)
		}
		eventID := referralEventIDForReward(t, ctx, db, rewardID)
		command := createReferralReversalCommand(t, ctx, db, fixture, eventID)
		if _, err := executeReferralReversal(t, ctx, db, command); err != nil {
			t.Fatal(err)
		}
		assertWalletBuckets(t, ctx, db, accountID, 0, 0, amount, amount)
		assertReferralReversalConservation(t, ctx, db, eventID, command.RefundTaskID, 1, amount, 0, amount)
	})
}

func TestReferralRewardReversalConcurrencyAndRollbackPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	t.Run("concurrent reversal writes once", func(t *testing.T) {
		fixture, _, rewardID, _, _, amount := prepareFrozenReleaseFixture(t, ctx, db, "reverse_concurrent", 99000, 2)
		eventID := referralEventIDForReward(t, ctx, db, rewardID)
		command := createReferralReversalCommand(t, ctx, db, fixture, eventID)
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := executeReferralReversal(t, ctx, db, command)
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("concurrent reversal: %v", err)
			}
		}
		assertReferralReversalConservation(t, ctx, db, eventID, command.RefundTaskID, 1, amount, amount, 0)
	})

	t.Run("insufficient frozen balance rolls back everything", func(t *testing.T) {
		fixture, taskID, rewardID, accountID, _, amount := prepareFrozenReleaseFixture(t, ctx, db, "reverse_rollback", 88000, 2)
		if _, err := db.ExecContext(ctx, `UPDATE xz_commission_wallet_accounts SET frozen_cents=$1 WHERE id=$2`, amount-1, accountID); err != nil {
			t.Fatal(err)
		}
		eventID := referralEventIDForReward(t, ctx, db, rewardID)
		command := createReferralReversalCommand(t, ctx, db, fixture, eventID)
		if _, err := executeReferralReversal(t, ctx, db, command); !errors.Is(err, ErrFrozenBalanceInsufficient) {
			t.Fatalf("expected insufficient frozen balance, got %v", err)
		}
		var rewardStatus, taskStatus, eventStatus string
		var negativeRewards, reversalLedgers int
		if err := db.QueryRowContext(ctx, `SELECT status FROM xz_referral_rewards WHERE id=$1`, rewardID).Scan(&rewardStatus); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT release_status FROM xz_referral_reward_release_tasks WHERE id=$1`, taskID).Scan(&taskStatus); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT status FROM xz_referral_events WHERE id=$1`, eventID).Scan(&eventStatus); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_referral_rewards WHERE reversal_of_id=$1`, rewardID).Scan(&negativeRewards); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_commission_wallet_ledger WHERE original_referral_reward_id=$1`, rewardID).Scan(&reversalLedgers); err != nil {
			t.Fatal(err)
		}
		if rewardStatus != "FROZEN" || taskStatus != "PENDING" || eventStatus != "REWARDED" || negativeRewards != 0 || reversalLedgers != 0 {
			t.Fatalf("partial rollback reward=%s task=%s event=%s adjustments=%d ledgers=%d", rewardStatus, taskStatus, eventStatus, negativeRewards, reversalLedgers)
		}
	})

	t.Run("wallet ledger conflict rolls back everything", func(t *testing.T) {
		fixture, taskID, rewardID, accountID, _, amount := prepareFrozenReleaseFixture(t, ctx, db, "reverse_ledger_conflict", 77000, 2)
		eventID := referralEventIDForReward(t, ctx, db, rewardID)
		command := createReferralReversalCommand(t, ctx, db, fixture, eventID)
		var beneficiaryType, beneficiaryID string
		if err := db.QueryRowContext(ctx, `SELECT beneficiary_type,beneficiary_user_id FROM xz_referral_rewards WHERE id=$1`, rewardID).Scan(&beneficiaryType, &beneficiaryID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO xz_commission_wallet_ledger(
				id,tenant_id,account_id,beneficiary_type,beneficiary_id,business_type,business_id,
				direction,available_delta_cents,balances_before,balances_after,idempotency_key,metadata
			) VALUES($1,$2,$3,$4,$5,'TEST_REVERSAL_CONFLICT',$6,'DEBIT',-1,'{}'::jsonb,'{}'::jsonb,$7,'{}'::jsonb)
		`, fixture.id("occupied_reversal_ledger"), fixture.tenantID, accountID, beneficiaryType, beneficiaryID, rewardID, referralRewardReversalLedgerKey(command.RefundTaskID, rewardID)); err != nil {
			t.Fatal(err)
		}
		if _, err := executeReferralReversal(t, ctx, db, command); !errors.Is(err, ErrReversalLedgerConflict) {
			t.Fatalf("expected reversal ledger conflict, got %v", err)
		}
		var rewardStatus, taskStatus string
		var frozen int64
		var negativeRewards, reversalLedgers int
		if err := db.QueryRowContext(ctx, `SELECT status FROM xz_referral_rewards WHERE id=$1`, rewardID).Scan(&rewardStatus); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT release_status FROM xz_referral_reward_release_tasks WHERE id=$1`, taskID).Scan(&taskStatus); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT frozen_cents FROM xz_commission_wallet_accounts WHERE id=$1`, accountID).Scan(&frozen); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_referral_rewards WHERE reversal_of_id=$1`, rewardID).Scan(&negativeRewards); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_commission_wallet_ledger WHERE original_referral_reward_id=$1`, rewardID).Scan(&reversalLedgers); err != nil {
			t.Fatal(err)
		}
		if rewardStatus != "FROZEN" || taskStatus != "PENDING" || frozen != amount || negativeRewards != 0 || reversalLedgers != 0 {
			t.Fatalf("ledger conflict partial state reward=%s task=%s frozen=%d adjustments=%d reversalLedgers=%d", rewardStatus, taskStatus, frozen, negativeRewards, reversalLedgers)
		}
	})
}

func createReferralReversalCommand(t *testing.T, ctx context.Context, db *sql.DB, fixture workflowFixture, eventID string) ReferralRewardReversalCommand {
	t.Helper()
	var serviceOrderID string
	var refundAmount int64
	refundTaskID := fixture.id("reward_reversal_refund")
	if err := db.QueryRowContext(ctx, `
		INSERT INTO xz_operation_center_refund_tasks(
			id,tenant_id,service_order_id,order_id,payment_record_id,commercial_rule_set_id,
			origin_type,refund_scope,amount_cents,currency,payment_channel,refund_status,idempotency_key
		)
		SELECT $1,service.tenant_id,service.id,service.order_id,$2,service.commercial_rule_set_id,
		       'ACTIVE_REVOCATION','FULL',service.technical_service_fee_cents,service.currency,
		       COALESCE(service.payment_channel,'MOCK'),'PENDING',$3
		FROM xz_operation_center_service_orders service WHERE service.order_id=$4
		RETURNING service_order_id,amount_cents
	`, refundTaskID, fixture.paymentID, fixture.id("reward_reversal_refund_key"), fixture.orderID).Scan(&serviceOrderID, &refundAmount); err != nil {
		t.Fatal(err)
	}
	return ReferralRewardReversalCommand{
		RefundTaskID: refundTaskID, OperationCenterServiceOrderID: serviceOrderID,
		ReferralEventID: eventID, RefundAmountCents: refundAmount,
		ReversalReason: "OPERATION_CENTER_FULL_REFUND", OperatorID: fixture.reviewerID,
		TransactionGroupID: fixture.id("reward_reversal_transaction"),
	}
}

func executeReferralReversal(t *testing.T, ctx context.Context, db *sql.DB, command ReferralRewardReversalCommand) (ReferralRewardReversalResult, error) {
	t.Helper()
	var result ReferralRewardReversalResult
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	store, err := NewPostgresStore(db)
	if err == nil {
		result, err = NewReferralRewardReversalService(store).ReverseReferralRewardsForRefund(ctx, tx, command)
	}
	if err != nil {
		_ = tx.Rollback()
		return result, err
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func releaseFrozenRewardForReversal(t *testing.T, ctx context.Context, db *sql.DB, fixture workflowFixture) {
	t.Helper()
	batch, err := mustReferralReleaseService(t, db).ClaimAndReleaseDueRewards(ctx, fixture.id("release_before_reversal"), 1)
	if err != nil || batch.Claimed != 1 || batch.Succeeded != 1 {
		t.Fatalf("release before reversal batch=%+v err=%v", batch, err)
	}
}

func referralEventIDForReward(t *testing.T, ctx context.Context, db *sql.DB, rewardID string) string {
	t.Helper()
	var eventID string
	if err := db.QueryRowContext(ctx, `SELECT referral_event_id FROM xz_referral_rewards WHERE id=$1`, rewardID).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	return eventID
}

func assertWalletBuckets(t *testing.T, ctx context.Context, db *sql.DB, accountID string, frozen, available, settled, recoverable int64) {
	t.Helper()
	var actualFrozen, actualAvailable, actualSettled, actualRecoverable int64
	if err := db.QueryRowContext(ctx, `SELECT frozen_cents,available_cents,settled_cents,recoverable_cents FROM xz_commission_wallet_accounts WHERE id=$1`, accountID).Scan(&actualFrozen, &actualAvailable, &actualSettled, &actualRecoverable); err != nil {
		t.Fatal(err)
	}
	if actualFrozen != frozen || actualAvailable != available || actualSettled != settled || actualRecoverable != recoverable {
		t.Fatalf("wallet buckets got=%d/%d/%d/%d want=%d/%d/%d/%d", actualFrozen, actualAvailable, actualSettled, actualRecoverable, frozen, available, settled, recoverable)
	}
}

func assertReferralReversalConservation(t *testing.T, ctx context.Context, db *sql.DB, eventID, refundTaskID string, count int, rewardTotal, directDebit, recoverable int64) {
	t.Helper()
	var originals, adjustments, ledgers int
	var originalSum, adjustmentSum, frozenDelta, availableDelta, recoverableDelta int64
	if err := db.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(amount_cents),0) FROM xz_referral_rewards WHERE referral_event_id=$1 AND reversal_of_id IS NULL AND status='REVERSED'`, eventID).Scan(&originals, &originalSum); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(reversal_amount_cents),0) FROM xz_referral_rewards WHERE referral_event_id=$1 AND reversal_of_id IS NOT NULL AND refund_task_id=$2`, eventID, refundTaskID).Scan(&adjustments, &adjustmentSum); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(frozen_delta_cents),0),COALESCE(sum(available_delta_cents),0),COALESCE(sum(recoverable_cents_delta),0) FROM xz_commission_wallet_ledger WHERE referral_event_id=$1 AND refund_task_id=$2 AND business_type='REFERRAL_REWARD_REVERSAL'`, eventID, refundTaskID).Scan(&ledgers, &frozenDelta, &availableDelta, &recoverableDelta); err != nil {
		t.Fatal(err)
	}
	actualDirectDebit := -frozenDelta - availableDelta
	if originals != count || adjustments != count || ledgers != count || originalSum != rewardTotal || adjustmentSum != rewardTotal || actualDirectDebit != directDebit || recoverableDelta != recoverable || rewardTotal != directDebit+recoverable {
		t.Fatalf("reversal conservation originals=%d/%d adjustments=%d/%d ledgers=%d sum=%d/%d debit=%d/%d recoverable=%d/%d", originals, count, adjustments, count, ledgers, originalSum, adjustmentSum, actualDirectDebit, directDebit, recoverableDelta, recoverable)
	}
}
