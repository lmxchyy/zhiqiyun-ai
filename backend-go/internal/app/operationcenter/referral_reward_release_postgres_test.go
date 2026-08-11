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

func TestReferralRewardReleaseSuccessIdempotencyAndLeasePostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	service := mustReferralReleaseService(t, db)

	t.Run("single reward release preserves equity and references", func(t *testing.T) {
		fixture, taskID, rewardID, accountID, grantLedgerID, amount := prepareFrozenReleaseFixture(t, ctx, db, "release_success", 321000, 0)
		var legacyBefore int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_commissions`).Scan(&legacyBefore); err != nil {
			t.Fatal(err)
		}
		batch, err := service.ClaimAndReleaseDueRewards(ctx, fixture.id("worker"), 10)
		if err != nil || batch.Claimed != 1 || batch.Succeeded != 1 || batch.Failed != 0 {
			t.Fatalf("release batch=%+v err=%v", batch, err)
		}
		assertReleasedRewardState(t, ctx, db, taskID, rewardID, accountID, grantLedgerID, amount)
		replay, err := service.ReleaseReferralReward(ctx, taskID, fixture.id("replay_worker"))
		if err != nil || !replay.IdempotentReplay {
			t.Fatalf("release replay=%+v err=%v", replay, err)
		}
		assertReleasedRewardState(t, ctx, db, taskID, rewardID, accountID, grantLedgerID, amount)
		var legacyAfter int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_commissions`).Scan(&legacyAfter); err != nil || legacyAfter != legacyBefore {
			t.Fatalf("legacy projection changed before=%d after=%d err=%v", legacyBefore, legacyAfter, err)
		}
	})

	t.Run("lease excludes another worker and expired lease is reclaimed", func(t *testing.T) {
		_, taskID, _, _, _, _ := prepareFrozenReleaseFixture(t, ctx, db, "release_lease", 9000, 0)
		claimed, err := service.ClaimDueRewards(ctx, "lease-worker-1", 1)
		if err != nil || len(claimed) != 1 || claimed[0].ID != taskID {
			t.Fatalf("first claim=%+v err=%v", claimed, err)
		}
		second, err := service.ClaimDueRewards(ctx, "lease-worker-2", 1)
		if err != nil || len(second) != 0 {
			t.Fatalf("active lease claim=%+v err=%v", second, err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE xz_referral_reward_release_tasks SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, taskID); err != nil {
			t.Fatal(err)
		}
		reclaimed, err := service.ClaimDueRewards(ctx, "lease-worker-2", 1)
		if err != nil || len(reclaimed) != 1 || reclaimed[0].ID != taskID {
			t.Fatalf("expired lease claim=%+v err=%v", reclaimed, err)
		}
	})

	t.Run("future task is not claimed", func(t *testing.T) {
		_, taskID, _, _, _, _ := prepareFrozenReleaseFixture(t, ctx, db, "release_future", 10000, 1)
		claimed, err := service.ClaimDueRewards(ctx, "future-worker", 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, task := range claimed {
			if task.ID == taskID {
				t.Fatalf("future task was claimed: %+v", task)
			}
		}
	})
}

func TestReferralRewardReleaseConcurrencyAndSharedWalletPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	service := mustReferralReleaseService(t, db)

	t.Run("two workers release once", func(t *testing.T) {
		_, taskID, rewardID, accountID, grantLedgerID, amount := prepareFrozenReleaseFixture(t, ctx, db, "release_concurrent", 45000, 0)
		var wg sync.WaitGroup
		results := make(chan ReferralRewardReleaseBatchResult, 2)
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				result, err := service.ClaimAndReleaseDueRewards(ctx, fmt.Sprintf("concurrent-worker-%d", worker), 1)
				results <- result
				errs <- err
			}(i)
		}
		wg.Wait()
		close(results)
		close(errs)
		totalSucceeded := 0
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		for result := range results {
			totalSucceeded += result.Succeeded
		}
		if totalSucceeded != 1 {
			t.Fatalf("concurrent success count=%d", totalSucceeded)
		}
		assertReleasedRewardState(t, ctx, db, taskID, rewardID, accountID, grantLedgerID, amount)
	})

	t.Run("multiple rewards sharing wallet serialize and conserve", func(t *testing.T) {
		prefix := fmt.Sprintf("release_shared_%d", time.Now().UnixNano())
		beneficiaryID := prefix + "_center"
		var accountID string
		var total int64
		for i, amount := range []int64{12000, 18000} {
			fixture := createWorkflowFixtureWithRelationship(t, ctx, db, fmt.Sprintf("%s_%d", prefix, i), fmt.Sprintf(`{"referrerType":"OPERATION_CENTER","referrerUserId":"%s","referrerOperationCenterUserId":"%s"}`, beneficiaryID, beneficiaryID))
			if i == 0 {
				seedEligibilityUser(t, ctx, db, beneficiaryID)
			}
			seedRewardGrantRule(t, ctx, db, fixture, fmt.Sprintf("SHARED_%d", i), ReferralReferrerOperationCenter, ReferralBeneficiaryOperationCenter, ReferralRelationReferrer, amount, 0)
			workflow := mustWorkflowService(t, db, WorkflowOptions{})
			paid, err := workflow.RecordPaymentSucceeded(ctx, PaymentSucceededCommand{OrderID: fixture.orderID, PaymentRecordID: fixture.paymentID})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := workflow.Review(ctx, ReviewCommand{ServiceOrderID: paid.ServiceOrder.ID, Decision: ReviewApproved, ExpectedStatus: OperationCenterServiceReviewRequired, IdempotencyKey: fixture.id("review"), ReviewedBy: fixture.reviewerID}); err != nil {
				t.Fatal(err)
			}
			total += amount
			accountID = referralWalletAccountID(fixture.tenantID, ReferralBeneficiaryOperationCenter, beneficiaryID)
		}
		batch, err := service.ClaimAndReleaseDueRewards(ctx, prefix+"_worker", 10)
		if err != nil || batch.Succeeded != 2 {
			t.Fatalf("shared wallet batch=%+v err=%v", batch, err)
		}
		var frozen, available int64
		if err := db.QueryRowContext(ctx, `SELECT frozen_cents,available_cents FROM xz_commission_wallet_accounts WHERE id=$1`, accountID).Scan(&frozen, &available); err != nil || frozen != 0 || available != total {
			t.Fatalf("shared wallet frozen=%d available=%d total=%d err=%v", frozen, available, total, err)
		}
	})
}

func TestReferralRewardReleaseFailureRollbackAndRetryPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	service := mustReferralReleaseService(t, db)

	t.Run("insufficient frozen balance rolls back and becomes permanent failure", func(t *testing.T) {
		_, taskID, rewardID, accountID, _, amount := prepareFrozenReleaseFixture(t, ctx, db, "release_insufficient", 50000, 0)
		if _, err := db.ExecContext(ctx, `UPDATE xz_commission_wallet_accounts SET frozen_cents=$2 WHERE id=$1`, accountID, amount-1); err != nil {
			t.Fatal(err)
		}
		claimed, err := service.ClaimDueRewards(ctx, "insufficient-worker", 1)
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim=%+v err=%v", claimed, err)
		}
		_, err = service.ReleaseReferralReward(ctx, taskID, "insufficient-worker")
		if !errors.Is(err, ErrFrozenBalanceInsufficient) {
			t.Fatalf("insufficient error=%v", err)
		}
		assertFailedReleaseRollback(t, ctx, db, taskID, rewardID, accountID, amount-1, "VALIDATION_FAILURE", false)
	})

	t.Run("release ledger conflict rolls back wallet and reward", func(t *testing.T) {
		fixture, taskID, rewardID, accountID, _, amount := prepareFrozenReleaseFixture(t, ctx, db, "release_conflict", 42000, 0)
		ledgerKey := referralRewardReleaseLedgerKey(taskID, rewardID)
		if _, err := db.ExecContext(ctx, `INSERT INTO xz_commission_wallet_ledger(id,tenant_id,account_id,beneficiary_type,beneficiary_id,business_type,business_id,direction,frozen_delta_cents,balances_before,balances_after,idempotency_key) SELECT $1,tenant_id,id,beneficiary_type,beneficiary_id,'TEST_RELEASE_CONFLICT',$2,'CREDIT',1,'{}','{}',$3 FROM xz_commission_wallet_accounts WHERE id=$4`, fixture.id("conflict"), rewardID, ledgerKey, accountID); err != nil {
			t.Fatal(err)
		}
		claimed, err := service.ClaimDueRewards(ctx, "conflict-worker", 1)
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim=%+v err=%v", claimed, err)
		}
		_, err = service.ReleaseReferralReward(ctx, taskID, "conflict-worker")
		if !errors.Is(err, ErrReleaseLedgerConflict) {
			t.Fatalf("ledger conflict error=%v", err)
		}
		assertFailedReleaseRollback(t, ctx, db, taskID, rewardID, accountID, amount, "VALIDATION_FAILURE", false)
	})

	t.Run("not due business condition schedules retry", func(t *testing.T) {
		_, taskID, rewardID, accountID, _, amount := prepareFrozenReleaseFixture(t, ctx, db, "release_retry", 33000, 1)
		owner := "retry-worker"
		if _, err := db.ExecContext(ctx, `UPDATE xz_referral_reward_release_tasks SET release_status='PROCESSING',lease_owner=$2,lease_expires_at=now()+interval '5 minutes',started_at=now() WHERE id=$1`, taskID, owner); err != nil {
			t.Fatal(err)
		}
		_, err := service.ReleaseReferralReward(ctx, taskID, owner)
		if !errors.Is(err, ErrRewardNotDue) {
			t.Fatalf("not due error=%v", err)
		}
		assertFailedReleaseRollback(t, ctx, db, taskID, rewardID, accountID, amount, "TEMPORARY_FAILURE", true)
	})

	t.Run("available reward without successful ledger fails safely", func(t *testing.T) {
		_, taskID, rewardID, _, _, _ := prepareFrozenReleaseFixture(t, ctx, db, "release_invariant", 22000, 0)
		if _, err := db.ExecContext(ctx, `UPDATE xz_referral_rewards SET status='AVAILABLE' WHERE id=$1`, rewardID); err != nil {
			t.Fatal(err)
		}
		claimed, err := service.ClaimDueRewards(ctx, "invariant-worker", 1)
		if err != nil || len(claimed) != 0 {
			t.Fatalf("inconsistent reward should not be claimed: %+v err=%v", claimed, err)
		}
		var ledgerCount int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_commission_wallet_ledger WHERE referral_release_task_id=$1`, taskID).Scan(&ledgerCount); err != nil || ledgerCount != 0 {
			t.Fatalf("inconsistent reward ledger=%d err=%v", ledgerCount, err)
		}
	})
}

func mustReferralReleaseService(t *testing.T, db *sql.DB) *ReferralRewardReleaseService {
	t.Helper()
	service, err := NewReferralRewardReleaseService(db, ReferralRewardReleaseOptions{LeaseDuration: 2 * time.Minute, RetryDelay: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func prepareFrozenReleaseFixture(t *testing.T, ctx context.Context, db *sql.DB, name string, amount int64, freezeDays int) (workflowFixture, string, string, string, string, int64) {
	t.Helper()
	fixture, eventID, _, _ := prepareEligibilityOnlyGrantFixture(t, ctx, db, name, amount, freezeDays)
	grantReferralEvent(t, ctx, db, eventID)
	var taskID, rewardID, accountID, grantLedgerID string
	err := db.QueryRowContext(ctx, `
		SELECT task.id,reward.id,ledger.account_id,reward.grant_wallet_ledger_id
		FROM xz_referral_rewards reward
		JOIN xz_referral_reward_release_tasks task ON task.id=reward.current_release_task_id
		JOIN xz_commission_wallet_ledger ledger ON ledger.id=reward.grant_wallet_ledger_id
		WHERE reward.referral_event_id=$1
	`, eventID).Scan(&taskID, &rewardID, &accountID, &grantLedgerID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `
			UPDATE xz_referral_reward_release_tasks
			SET release_status='CANCELLED',lease_owner=NULL,lease_expires_at=NULL,next_retry_at=NULL,
			    cancellation_reason='TEST_CLEANUP',cancelled_at=now(),completed_at=now()
			WHERE id=$1 AND release_status<>'SUCCEEDED'
		`, taskID)
	})
	return fixture, taskID, rewardID, accountID, grantLedgerID, amount
}

func assertReleasedRewardState(t *testing.T, ctx context.Context, db *sql.DB, taskID, rewardID, accountID, grantLedgerID string, amount int64) {
	t.Helper()
	var rewardStatus, taskStatus, releaseLedgerID, originalLedgerID, ledgerRewardID, ledgerTaskID string
	var frozen, available, settled, recoverable, frozenDelta, availableDelta int64
	var leaseOwner sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT status,release_wallet_ledger_id FROM xz_referral_rewards WHERE id=$1`, rewardID).Scan(&rewardStatus, &releaseLedgerID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT release_status,lease_owner FROM xz_referral_reward_release_tasks WHERE id=$1`, taskID).Scan(&taskStatus, &leaseOwner); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT frozen_cents,available_cents,settled_cents,recoverable_cents FROM xz_commission_wallet_accounts WHERE id=$1`, accountID).Scan(&frozen, &available, &settled, &recoverable); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT original_ledger_id,referral_reward_id,referral_release_task_id,frozen_delta_cents,available_delta_cents FROM xz_commission_wallet_ledger WHERE id=$1`, releaseLedgerID).Scan(&originalLedgerID, &ledgerRewardID, &ledgerTaskID, &frozenDelta, &availableDelta); err != nil {
		t.Fatal(err)
	}
	if rewardStatus != "AVAILABLE" || taskStatus != "SUCCEEDED" || leaseOwner.Valid || originalLedgerID != grantLedgerID || ledgerRewardID != rewardID || ledgerTaskID != taskID {
		t.Fatalf("release references reward=%s task=%s lease=%v original=%s rewardRef=%s taskRef=%s", rewardStatus, taskStatus, leaseOwner, originalLedgerID, ledgerRewardID, ledgerTaskID)
	}
	if frozen != 0 || available != amount || settled != 0 || recoverable != 0 || frozenDelta != -amount || availableDelta != amount {
		t.Fatalf("release amounts frozen=%d available=%d settled=%d recoverable=%d deltas=%d/%d", frozen, available, settled, recoverable, frozenDelta, availableDelta)
	}
	var ledgerCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_commission_wallet_ledger WHERE referral_release_task_id=$1`, taskID).Scan(&ledgerCount); err != nil || ledgerCount != 1 {
		t.Fatalf("release ledger count=%d err=%v", ledgerCount, err)
	}
}

func assertFailedReleaseRollback(t *testing.T, ctx context.Context, db *sql.DB, taskID, rewardID, accountID string, frozen int64, failureClass string, retryable bool) {
	t.Helper()
	var taskStatus, gotFailure, rewardStatus string
	var nextRetry sql.NullTime
	var frozenCents, availableCents int64
	var ledgerCount int
	if err := db.QueryRowContext(ctx, `SELECT release_status,failure_class,next_retry_at FROM xz_referral_reward_release_tasks WHERE id=$1`, taskID).Scan(&taskStatus, &gotFailure, &nextRetry); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT status FROM xz_referral_rewards WHERE id=$1`, rewardID).Scan(&rewardStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT frozen_cents,available_cents FROM xz_commission_wallet_accounts WHERE id=$1`, accountID).Scan(&frozenCents, &availableCents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_commission_wallet_ledger WHERE referral_release_task_id=$1`, taskID).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if taskStatus != "FAILED" || gotFailure != failureClass || nextRetry.Valid != retryable || rewardStatus != "FROZEN" || frozenCents != frozen || availableCents != 0 || ledgerCount != 0 {
		t.Fatalf("failed release task=%s class=%s retry=%v reward=%s frozen=%d available=%d ledgers=%d", taskStatus, gotFailure, nextRetry.Valid, rewardStatus, frozenCents, availableCents, ledgerCount)
	}
}
