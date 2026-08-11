package operationcenter

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/payment"
)

func TestOperationCenterSystemE2EGateLifecyclePostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Run("review activation eligibility grant release available", func(t *testing.T) {
		fixture, releaseTaskID, rewardID, accountID, _, amount := prepareFrozenReleaseFixture(t, ctx, db, "system_gate_release", 88000, 0)
		eventID := referralEventIDForReward(t, ctx, db, rewardID)
		dryConfig := DefaultOperationCenterRuntimeConfig("test")
		dryConfig.RewardReleaseSchedulerEnabled = true
		dryConfig.DryRun = true
		dryResult, err := mustOperationCenterRuntime(t, db, dryConfig, nil).RunReferralRewardReleaseOnce(ctx)
		if err != nil || !dryResult.DryRun || dryResult.Due < 1 || dryResult.Claimed != 0 {
			t.Fatalf("reward release dry-run result=%+v err=%v", dryResult, err)
		}
		assertWalletBuckets(t, ctx, db, accountID, amount, 0, 0, 0)
		config := DefaultOperationCenterRuntimeConfig("test")
		config.RewardReleaseSchedulerEnabled = true
		runtime := mustOperationCenterRuntime(t, db, config, nil)
		result, err := runtime.RunReferralRewardReleaseOnce(ctx)
		if err != nil || result.Claimed != 1 || result.Succeeded != 1 || result.Failed != 0 {
			t.Fatalf("release gate result=%+v err=%v", result, err)
		}
		var serviceStatus, eventStatus, eligibilityStatus, rewardStatus, taskStatus string
		var eligibilityCount, rewardCount, ledgerCount int
		if err := db.QueryRowContext(ctx, `SELECT status FROM xz_operation_center_service_orders WHERE order_id=$1`, fixture.orderID).Scan(&serviceStatus); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT status FROM xz_referral_events WHERE id=$1`, eventID).Scan(&eventStatus); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*),min(eligibility_status::text) FROM xz_referral_eligibilities WHERE referral_event_id=$1`, eventID).Scan(&eligibilityCount, &eligibilityStatus); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*),min(status::text) FROM xz_referral_rewards WHERE referral_event_id=$1 AND reversal_of_id IS NULL`, eventID).Scan(&rewardCount, &rewardStatus); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT release_status FROM xz_referral_reward_release_tasks WHERE id=$1`, releaseTaskID).Scan(&taskStatus); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_commission_wallet_ledger WHERE referral_reward_id=$1`, rewardID).Scan(&ledgerCount); err != nil {
			t.Fatal(err)
		}
		if serviceStatus != "ACTIVE" || eventStatus != "REWARDED" || eligibilityCount != 1 || eligibilityStatus != "CONSUMED" || rewardCount != 1 || rewardStatus != "AVAILABLE" || taskStatus != "SUCCEEDED" || ledgerCount != 2 {
			t.Fatalf("closed loop service=%s event=%s eligibility=%d/%s reward=%d/%s task=%s ledgers=%d", serviceStatus, eventStatus, eligibilityCount, eligibilityStatus, rewardCount, rewardStatus, taskStatus, ledgerCount)
		}
		assertWalletBuckets(t, ctx, db, accountID, 0, amount, 0, 0)
	})

	t.Run("provider missing fails before refund saga mutation", func(t *testing.T) {
		fixture, serviceID, taskID := createRejectedRefundSagaFixture(t, ctx, db, "system_gate_provider_missing")
		runtime := mustOperationCenterRuntime(t, db, DefaultOperationCenterRuntimeConfig("test"), nil)
		_, err := runtime.Execute(ctx, refundSagaCommand(fixture, serviceID, taskID))
		if !errors.Is(err, ErrRefundProviderUnavailable) {
			t.Fatalf("expected unavailable provider, got %v", err)
		}
		var serviceStatus, refundStatus string
		if err := db.QueryRowContext(ctx, `SELECT status,refund_status FROM xz_operation_center_service_orders WHERE id=$1`, serviceID).Scan(&serviceStatus, &refundStatus); err != nil {
			t.Fatal(err)
		}
		if serviceStatus != "REJECTED" || refundStatus != "PENDING" {
			t.Fatalf("missing provider mutated saga state %s/%s", serviceStatus, refundStatus)
		}
	})

	t.Run("review reject automatic and manual refund evidence", func(t *testing.T) {
		fixture, serviceID, taskID := createRejectedRefundSagaFixture(t, ctx, db, "system_gate_reject_success")
		provider := newSagaMockProvider(RefundProviderSuccess)
		runtime := mustOperationCenterRuntime(t, db, DefaultOperationCenterRuntimeConfig("test"), provider)
		command := refundSagaCommand(fixture, serviceID, taskID)
		first, err := runtime.Execute(ctx, command)
		if err != nil || first.RefundStatus != OperationCenterRefundSucceeded || !first.ProviderCalled {
			t.Fatalf("automatic reject refund result=%+v err=%v", first, err)
		}
		replay, err := runtime.Execute(ctx, command)
		if err != nil || !replay.IdempotentReplay {
			t.Fatalf("automatic refund replay=%+v err=%v", replay, err)
		}
		if calls, numbers := provider.snapshot(); calls != 1 || len(numbers) != 1 {
			t.Fatalf("automatic refund calls=%d numbers=%v", calls, numbers)
		}

		manualFixture, manualServiceID, manualTaskID := createRejectedRefundSagaFixture(t, ctx, db, "system_gate_reject_manual")
		unsupported := newSagaMockProvider(RefundProviderUnsupported)
		manualRuntime := mustOperationCenterRuntime(t, db, DefaultOperationCenterRuntimeConfig("test"), unsupported)
		manualResult, err := manualRuntime.Execute(ctx, refundSagaCommand(manualFixture, manualServiceID, manualTaskID))
		if err != nil || manualResult.RefundStatus != OperationCenterRefundManualRequired {
			t.Fatalf("unsupported result=%+v err=%v", manualResult, err)
		}
		management, err := NewRefundManagementService(db)
		if err != nil {
			t.Fatal(err)
		}
		financialSubmitter := manualFixture.reviewerID
		financialApprover := manualFixture.userID
		view, err := management.GetRefund(ctx, manualTaskID)
		if err != nil {
			t.Fatal(err)
		}
		submitted, err := management.SubmitManualRefund(ctx, ManualRefundSubmitCommand{
			RefundTaskID: manualTaskID, IdempotencyKey: manualFixture.id("financial_submitter_submit"),
			ChannelRefundNo: manualFixture.id("manual_channel_refund"), VoucherReference: "finance-voucher-001",
			VoucherFileHash: strings.Repeat("a", 64), Reason: "provider unsupported manual refund",
			RefundAmountCents: view.AmountCents, SubmittedBy: financialSubmitter, RequestID: manualFixture.id("manual_submit_request"),
		})
		if err != nil || submitted.ManualRefund == nil || submitted.RefundTask.Status != OperationCenterRefundManualSubmitted {
			t.Fatalf("manual submit=%+v err=%v", submitted, err)
		}
		_, err = management.ReviewManualRefund(ctx, ManualRefundReviewCommand{
			RefundTaskID: manualTaskID, IdempotencyKey: manualFixture.id("self_approval"), Decision: "APPROVED",
			ExpectedStatus: ManualRefundSubmitted, ReviewedBy: financialSubmitter, Reason: "must be rejected",
		})
		if !errors.Is(err, ErrManualRefundSelfApproval) {
			t.Fatalf("self approval err=%v", err)
		}
		approved, err := management.ReviewManualRefund(ctx, ManualRefundReviewCommand{
			RefundTaskID: manualTaskID, IdempotencyKey: manualFixture.id("financial_approver_approve"), Decision: "APPROVED",
			ExpectedStatus: ManualRefundSubmitted, ReviewedBy: financialApprover, RequestID: manualFixture.id("manual_approve_request"), Reason: "voucher verified",
		})
		if err != nil || approved.ManualRefund == nil || approved.RefundTask.Status != OperationCenterRefundSucceeded || approved.ManualRefund.ApprovedBy == nil || *approved.ManualRefund.ApprovedBy != financialApprover {
			t.Fatalf("manual approve=%+v err=%v", approved, err)
		}
		auditView, err := management.GetRefund(ctx, manualTaskID)
		if err != nil {
			t.Fatal(err)
		}
		actors := map[string]bool{}
		for _, audit := range auditView.Audit {
			if audit.OperatorID != nil {
				actors[*audit.OperatorID] = true
			}
		}
		if !actors[financialSubmitter] || !actors[financialApprover] || financialSubmitter == financialApprover || approved.RefundTask.ProviderRefundNo == nil {
			t.Fatalf("manual evidence actors=%v task=%+v", actors, approved.RefundTask)
		}
	})
}

func TestOperationCenterSystemE2EGateRewardReversalStatesPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cases := []struct {
		name                         string
		prepare                      func(*testing.T, context.Context, workflowFixture, string, string, int64)
		wantFrozen, wantAvailable    int64
		wantSettled, wantRecoverable int64
	}{
		{name: "frozen"},
		{name: "available", prepare: func(t *testing.T, ctx context.Context, fixture workflowFixture, _, _ string, _ int64) {
			releaseFrozenRewardForReversal(t, ctx, db, fixture)
		}},
		{name: "settled", prepare: func(t *testing.T, ctx context.Context, fixture workflowFixture, rewardID, accountID string, amount int64) {
			releaseFrozenRewardForReversal(t, ctx, db, fixture)
			if _, err := db.ExecContext(ctx, `UPDATE xz_referral_rewards SET status='SETTLED' WHERE id=$1`, rewardID); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `UPDATE xz_commission_wallet_accounts SET available_cents=0,settled_cents=$1 WHERE id=$2`, amount, accountID); err != nil {
				t.Fatal(err)
			}
		}, wantSettled: -1, wantRecoverable: -1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture, _, rewardID, accountID, _, amount := prepareFrozenReleaseFixture(t, ctx, db, "system_gate_reverse_"+testCase.name, 97000, 0)
			if testCase.prepare != nil {
				testCase.prepare(t, ctx, fixture, rewardID, accountID, amount)
			}
			serviceID, taskID := createActiveRefundSagaTask(t, ctx, db, fixture)
			provider := newSagaMockProvider(RefundProviderSuccess)
			runtime := mustOperationCenterRuntime(t, db, DefaultOperationCenterRuntimeConfig("test"), provider)
			result, err := runtime.Execute(ctx, refundSagaCommand(fixture, serviceID, taskID))
			if err != nil || result.ServiceStatus != OperationCenterServiceRevoked || result.RefundStatus != OperationCenterRefundSucceeded {
				t.Fatalf("%s reversal result=%+v err=%v", testCase.name, result, err)
			}
			settled, recoverable := testCase.wantSettled, testCase.wantRecoverable
			if settled < 0 {
				settled = amount
			}
			if recoverable < 0 {
				recoverable = amount
			}
			assertWalletBuckets(t, ctx, db, accountID, testCase.wantFrozen, testCase.wantAvailable, settled, recoverable)
			assertRevokedOperationCenterResources(t, ctx, db, fixture, serviceID)
			assertReferralReversalConservation(t, ctx, db, referralEventIDForReward(t, ctx, db, rewardID), taskID, 1, amount, amount-recoverable, recoverable)
		})
	}
}

func TestOperationCenterSystemE2EGateSchedulersPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	fixture, serviceID, taskID := createRejectedRefundSagaFixture(t, ctx, db, "system_gate_scheduler_retry")
	provider := newSagaMockProvider(RefundProviderTemporaryFailure, RefundProviderSuccess)
	config := DefaultOperationCenterRuntimeConfig("test")
	config.RefundRetrySchedulerEnabled = true
	config.BatchLimit = 1
	runtime := mustOperationCenterRuntime(t, db, config, provider)
	if result, err := runtime.Execute(ctx, refundSagaCommand(fixture, serviceID, taskID)); err != nil || result.RefundStatus != OperationCenterRefundRetryable {
		t.Fatalf("temporary result=%+v err=%v", result, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE xz_operation_center_refund_tasks SET next_retry_at='2000-01-01T00:00:00Z' WHERE id=$1`, taskID); err != nil {
		t.Fatal(err)
	}
	dryConfig := config
	dryConfig.DryRun = true
	dryRuntime := mustOperationCenterRuntime(t, db, dryConfig, provider)
	dry, err := dryRuntime.RunRefundRetryOnce(ctx)
	if err != nil || !dry.DryRun || dry.Due != 1 || dry.Claimed != 0 {
		t.Fatalf("dry run=%+v err=%v", dry, err)
	}
	if calls, _ := provider.snapshot(); calls != 1 {
		t.Fatalf("dry run called provider %d times", calls)
	}
	disabled := mustOperationCenterRuntime(t, db, DefaultOperationCenterRuntimeConfig("test"), provider)
	if _, err := disabled.RunRefundRetryOnce(ctx); !errors.Is(err, ErrRefundSchedulerDisabled) {
		t.Fatalf("disabled retry err=%v", err)
	}
	retried, err := runtime.RunRefundRetryOnce(ctx)
	if err != nil || retried.Claimed != 1 || retried.Succeeded != 1 {
		t.Fatalf("retry result=%+v err=%v", retried, err)
	}
	if calls, numbers := provider.snapshot(); calls != 2 || len(numbers) != 2 || numbers[0] != numbers[1] {
		t.Fatalf("stable refund number calls=%d numbers=%v", calls, numbers)
	}

	unknownFixture, unknownServiceID, unknownTaskID := createRejectedRefundSagaFixture(t, ctx, db, "system_gate_scheduler_unknown")
	unknownProvider := newSagaMockProvider(RefundProviderUnknown)
	unknownProvider.queryOutcomes = []payment.QueryRefundOutcome{payment.QueryRefundProcessing, payment.QueryRefundSucceeded}
	verifyConfig := DefaultOperationCenterRuntimeConfig("test")
	verifyConfig.RefundVerificationEnabled = true
	verifyConfig.BatchLimit = 1
	verifyRuntime := mustOperationCenterRuntime(t, db, verifyConfig, unknownProvider)
	if result, err := verifyRuntime.Execute(ctx, refundSagaCommand(unknownFixture, unknownServiceID, unknownTaskID)); err != nil || result.RefundStatus != OperationCenterRefundUnknownVerifying {
		t.Fatalf("unknown result=%+v err=%v", result, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE xz_operation_center_refund_tasks SET next_retry_at='2000-01-01T00:00:00Z' WHERE id=$1`, unknownTaskID); err != nil {
		t.Fatal(err)
	}
	verifyDryConfig := verifyConfig
	verifyDryConfig.DryRun = true
	verifyDry, err := mustOperationCenterRuntime(t, db, verifyDryConfig, unknownProvider).RunRefundVerificationOnce(ctx)
	if err != nil || !verifyDry.DryRun || verifyDry.Due < 1 || verifyDry.Claimed != 0 {
		t.Fatalf("verification dry-run=%+v err=%v", verifyDry, err)
	}
	if queryCalls, _ := unknownProvider.querySnapshot(); queryCalls != 0 {
		t.Fatalf("verification dry-run queried provider %d times", queryCalls)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := db.ExecContext(ctx, `UPDATE xz_operation_center_refund_tasks SET next_retry_at='2000-01-01T00:00:00Z' WHERE id=$1`, unknownTaskID); err != nil {
			t.Fatal(err)
		}
		verified, err := verifyRuntime.RunRefundVerificationOnce(ctx)
		if err != nil || verified.Claimed != 1 {
			t.Fatalf("verification attempt=%d result=%+v err=%v", attempt, verified, err)
		}
	}
	assertRefundSagaTaskResult(t, ctx, db, unknownTaskID, OperationCenterRefundSucceeded, RefundProviderSuccess, true)
	if calls, _ := unknownProvider.snapshot(); calls != 1 {
		t.Fatalf("UNKNOWN verification retried RefundPayment calls=%d", calls)
	}
	if queryCalls, numbers := unknownProvider.querySnapshot(); queryCalls != 2 || len(numbers) != 2 || numbers[0] != numbers[1] {
		t.Fatalf("query stability calls=%d numbers=%v", queryCalls, numbers)
	}
}

func mustOperationCenterRuntime(t *testing.T, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, config OperationCenterRuntimeConfig, provider payment.RefundProvider) *OperationCenterRuntime {
	t.Helper()
	sqlDB, ok := db.(*sql.DB)
	if !ok {
		t.Fatal("operation center runtime test requires *sql.DB")
	}
	bindings := []RefundProviderBinding{}
	if provider != nil {
		bindings = append(bindings, RefundProviderBinding{PaymentChannel: "MOCK", Provider: provider})
	}
	runtime, err := NewOperationCenterRuntime(sqlDB, config, nil, bindings...)
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}
