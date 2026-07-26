package operationcenter

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/payment"
)

func TestRefundManagementActiveRequestIdempotencyAndConcurrencyPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	management, err := NewRefundManagementService(db)
	if err != nil {
		t.Fatal(err)
	}
	fixture, _, _, _, _, _ := prepareFrozenReleaseFixture(t, ctx, db, "refund_management_request", 500000, 7)
	var serviceID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM xz_operation_center_service_orders WHERE order_id=$1`, fixture.orderID).Scan(&serviceID); err != nil {
		t.Fatal(err)
	}
	command := RefundRequestCommand{ServiceOrderID: serviceID, IdempotencyKey: fixture.id("active_refund_request"), ExpectedServiceStatus: OperationCenterServiceActive, Reason: "customer requested full refund", RequestedBy: fixture.reviewerID, RequestID: fixture.id("request")}
	created, err := management.RequestActiveRefund(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if created.RefundTask == nil || created.RefundTask.Status != OperationCenterRefundPending || created.RefundTask.Origin != RefundOriginActiveRevocation {
		t.Fatalf("active refund result=%+v", created)
	}
	replay, err := management.RequestActiveRefund(ctx, command)
	if err != nil || !replay.IdempotentReplay || replay.RefundTask.ID != created.RefundTask.ID {
		t.Fatalf("request replay=%+v err=%v", replay, err)
	}
	conflict := command
	conflict.IdempotencyKey = fixture.id("different_active_refund_request")
	if _, err := management.RequestActiveRefund(ctx, conflict); !errors.Is(err, ErrRefundAlreadyRequested) {
		t.Fatalf("different request error=%v", err)
	}
	var serviceStatus, refundStatus string
	if err := db.QueryRowContext(ctx, `SELECT status,refund_status FROM xz_operation_center_service_orders WHERE id=$1`, serviceID).Scan(&serviceStatus, &refundStatus); err != nil {
		t.Fatal(err)
	}
	if serviceStatus != "ACTIVE" || refundStatus != "PENDING" {
		t.Fatalf("request changed service too early %s/%s", serviceStatus, refundStatus)
	}

	fixture2, _, _, _, _, _ := prepareFrozenReleaseFixture(t, ctx, db, "refund_management_concurrent", 500000, 7)
	var serviceID2 string
	if err := db.QueryRowContext(ctx, `SELECT id FROM xz_operation_center_service_orders WHERE order_id=$1`, fixture2.orderID).Scan(&serviceID2); err != nil {
		t.Fatal(err)
	}
	commands := []RefundRequestCommand{
		{ServiceOrderID: serviceID2, IdempotencyKey: fixture2.id("concurrent_a"), ExpectedServiceStatus: OperationCenterServiceActive, Reason: "a", RequestedBy: fixture2.reviewerID},
		{ServiceOrderID: serviceID2, IdempotencyKey: fixture2.id("concurrent_b"), ExpectedServiceStatus: OperationCenterServiceActive, Reason: "b", RequestedBy: fixture2.reviewerID},
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, item := range commands {
		wg.Add(1)
		go func(cmd RefundRequestCommand) {
			defer wg.Done()
			_, callErr := management.RequestActiveRefund(ctx, cmd)
			errs <- callErr
		}(item)
	}
	wg.Wait()
	close(errs)
	var successes int
	for callErr := range errs {
		if callErr == nil {
			successes++
		} else if !errors.Is(callErr, ErrRefundAlreadyRequested) && !errors.Is(callErr, ErrUniqueConflict) {
			t.Fatalf("concurrent error=%v", callErr)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent successes=%d", successes)
	}
}

func TestRefundSchedulersIsolationLimitsAndRecoveryPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Run("retry scheduler", func(t *testing.T) {
		fixture, serviceID, taskID := createRejectedRefundSagaFixture(t, ctx, db, "refund_scheduler_retry")
		provider := newSagaMockProvider(RefundProviderTemporaryFailure, RefundProviderSuccess)
		orchestrator := mustRefundOrchestrator(t, db, provider)
		if result, err := orchestrator.Execute(ctx, refundSagaCommand(fixture, serviceID, taskID)); err != nil || result.RefundStatus != OperationCenterRefundRetryable {
			t.Fatalf("initial retry=%+v err=%v", result, err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE xz_operation_center_refund_tasks SET next_retry_at=now()-interval '1 second' WHERE id=$1`, taskID); err != nil {
			t.Fatal(err)
		}
		options := DefaultRefundSchedulerOptions()
		options.RetryEnabled = true
		options.WorkerName = fixture.id("retry_worker")
		scheduler, err := NewRefundSchedulerService(db, orchestrator, options, nil)
		if err != nil {
			t.Fatal(err)
		}
		run, err := scheduler.RunRetryOnce(ctx)
		if err != nil || run.Claimed != 1 || run.Succeeded != 1 {
			t.Fatalf("retry run=%+v err=%v", run, err)
		}
		if calls, numbers := provider.snapshot(); calls != 2 || numbers[0] != numbers[1] {
			t.Fatalf("retry refund calls=%d numbers=%v", calls, numbers)
		}
	})

	t.Run("verification scheduler and status isolation", func(t *testing.T) {
		fixture, serviceID, taskID := createRejectedRefundSagaFixture(t, ctx, db, "refund_scheduler_verify")
		provider := newSagaMockProvider(RefundProviderUnknown)
		provider.queryOutcomes = []payment.QueryRefundOutcome{payment.QueryRefundSucceeded}
		orchestrator := mustRefundOrchestrator(t, db, provider)
		if result, err := orchestrator.Execute(ctx, refundSagaCommand(fixture, serviceID, taskID)); err != nil || result.RefundStatus != OperationCenterRefundUnknownVerifying {
			t.Fatalf("initial unknown=%+v err=%v", result, err)
		}
		options := DefaultRefundSchedulerOptions()
		options.VerificationEnabled = true
		options.WorkerName = fixture.id("verify_worker")
		scheduler, err := NewRefundSchedulerService(db, orchestrator, options, nil)
		if err != nil {
			t.Fatal(err)
		}
		run, err := scheduler.RunVerificationOnce(ctx)
		if err != nil || run.Claimed != 1 || run.Succeeded != 1 {
			t.Fatalf("verify run=%+v err=%v", run, err)
		}
		if calls, _ := provider.snapshot(); calls != 1 {
			t.Fatalf("verification called RefundPayment %d times", calls)
		}
		if queryCalls, _ := provider.querySnapshot(); queryCalls != 1 {
			t.Fatalf("query calls=%d", queryCalls)
		}
	})

	t.Run("retry maximum becomes manual", func(t *testing.T) {
		fixture, serviceID, taskID := createRejectedRefundSagaFixture(t, ctx, db, "refund_scheduler_limit")
		provider := newSagaMockProvider(RefundProviderTemporaryFailure)
		orchestrator := mustRefundOrchestrator(t, db, provider)
		if _, err := orchestrator.Execute(ctx, refundSagaCommand(fixture, serviceID, taskID)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE xz_operation_center_refund_tasks SET attempt_count=3,next_retry_at=now()-interval '1 second' WHERE id=$1`, taskID); err != nil {
			t.Fatal(err)
		}
		options := DefaultRefundSchedulerOptions()
		options.RetryEnabled = true
		options.MaxRetryAttempts = 3
		options.WorkerName = fixture.id("limit_worker")
		scheduler, err := NewRefundSchedulerService(db, orchestrator, options, nil)
		if err != nil {
			t.Fatal(err)
		}
		run, err := scheduler.RunRetryOnce(ctx)
		if err != nil || run.ManualRequired != 1 {
			t.Fatalf("limit run=%+v err=%v", run, err)
		}
		assertRefundSagaTaskResult(t, ctx, db, taskID, OperationCenterRefundManualRequired, RefundProviderTemporaryFailure, false)
	})

	t.Run("verification maximum becomes manual without query", func(t *testing.T) {
		fixture, serviceID, taskID := createRejectedRefundSagaFixture(t, ctx, db, "refund_verification_limit")
		provider := newSagaMockProvider(RefundProviderUnknown)
		provider.queryOutcomes = []payment.QueryRefundOutcome{payment.QueryRefundSucceeded}
		orchestrator := mustRefundOrchestrator(t, db, provider)
		if _, err := orchestrator.Execute(ctx, refundSagaCommand(fixture, serviceID, taskID)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE xz_operation_center_refund_tasks SET verification_attempt_count=2,next_retry_at=now()-interval '1 second' WHERE id=$1`, taskID); err != nil {
			t.Fatal(err)
		}
		options := DefaultRefundSchedulerOptions()
		options.VerificationEnabled = true
		options.MaxVerificationAttempts = 2
		options.WorkerName = fixture.id("verification_limit_worker")
		scheduler, err := NewRefundSchedulerService(db, orchestrator, options, nil)
		if err != nil {
			t.Fatal(err)
		}
		run, err := scheduler.RunVerificationOnce(ctx)
		if err != nil || run.ManualRequired != 1 {
			t.Fatalf("verification limit run=%+v err=%v", run, err)
		}
		if queryCalls, _ := provider.querySnapshot(); queryCalls != 0 {
			t.Fatalf("verification limit queried provider %d times", queryCalls)
		}
	})
}

func TestManualRefundRejectResubmitApproveAndEvidencePostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	fixture, serviceID, taskID := createRejectedRefundSagaFixture(t, ctx, db, "manual_refund_management")
	provider := newSagaMockProvider(RefundProviderUnsupported)
	if result, err := mustRefundOrchestrator(t, db, provider).Execute(ctx, refundSagaCommand(fixture, serviceID, taskID)); err != nil || result.RefundStatus != OperationCenterRefundManualRequired {
		t.Fatalf("unsupported=%+v err=%v", result, err)
	}
	management, err := NewRefundManagementService(db)
	if err != nil {
		t.Fatal(err)
	}
	var approverID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM xz_users WHERE id<>$1 ORDER BY id LIMIT 1`, fixture.reviewerID).Scan(&approverID); err != nil {
		t.Fatal(err)
	}
	beforeLegacy := countLegacyCommissionsForOrder(t, ctx, db, fixture.orderID)
	submit := ManualRefundSubmitCommand{RefundTaskID: taskID, IdempotencyKey: fixture.id("manual_submit_1"), ChannelRefundNo: fixture.id("manual_channel_refund"), RefundAmountCents: 500000, VoucherReference: fixture.id("voucher_1"), VoucherFileHash: strings.Repeat("a", 64), Reason: "finance submitted", SubmittedBy: fixture.reviewerID, RequestID: fixture.id("manual_request_1")}
	submitted, err := management.SubmitManualRefund(ctx, submit)
	if err != nil || submitted.RefundTask.Status != OperationCenterRefundManualSubmitted {
		t.Fatalf("submitted=%+v err=%v", submitted, err)
	}
	if _, err := management.SubmitManualRefund(ctx, submit); err != nil {
		t.Fatalf("submit replay=%v", err)
	}
	selfReview := ManualRefundReviewCommand{RefundTaskID: taskID, IdempotencyKey: fixture.id("self_review"), ExpectedStatus: ManualRefundSubmitted, Decision: "APPROVED", Reason: "self", ReviewedBy: fixture.reviewerID}
	if _, err := management.ReviewManualRefund(ctx, selfReview); !errors.Is(err, ErrManualRefundSelfApproval) {
		t.Fatalf("self approval=%v", err)
	}
	reject := ManualRefundReviewCommand{RefundTaskID: taskID, IdempotencyKey: fixture.id("manual_reject"), ExpectedStatus: ManualRefundSubmitted, Decision: "REJECTED", Reason: "voucher unclear", ReviewedBy: approverID}
	rejected, err := management.ReviewManualRefund(ctx, reject)
	if err != nil || rejected.RefundTask.Status != OperationCenterRefundManualRequired {
		t.Fatalf("rejected=%+v err=%v", rejected, err)
	}
	submit.IdempotencyKey = fixture.id("manual_submit_2")
	submit.VoucherReference = fixture.id("voucher_2")
	submit.VoucherFileHash = strings.Repeat("b", 64)
	resubmitted, err := management.SubmitManualRefund(ctx, submit)
	if err != nil || resubmitted.RefundTask.Status != OperationCenterRefundManualSubmitted {
		t.Fatalf("resubmitted=%+v err=%v", resubmitted, err)
	}
	approve := ManualRefundReviewCommand{RefundTaskID: taskID, IdempotencyKey: fixture.id("manual_approve"), ExpectedStatus: ManualRefundSubmitted, Decision: "APPROVED", Reason: "verified", ReviewedBy: approverID, RequestID: fixture.id("approve_request")}
	approved, err := management.ReviewManualRefund(ctx, approve)
	if err != nil || approved.RefundTask.Status != OperationCenterRefundSucceeded {
		t.Fatalf("approved=%+v err=%v", approved, err)
	}
	replay, err := management.ReviewManualRefund(ctx, approve)
	if err != nil || !replay.IdempotentReplay {
		t.Fatalf("approve replay=%+v err=%v", replay, err)
	}
	var status string
	var providerNo sql.NullString
	var refundedAt sql.NullTime
	var submittedBy, approvedBy sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT refund_status,provider_refund_no,provider_refunded_at,manual_submitted_by,manual_approved_by FROM xz_operation_center_refund_tasks WHERE id=$1`, taskID).Scan(&status, &providerNo, &refundedAt, &submittedBy, &approvedBy); err != nil {
		t.Fatal(err)
	}
	if status != "SUCCEEDED" || !providerNo.Valid || !refundedAt.Valid || !submittedBy.Valid || !approvedBy.Valid || submittedBy.String == approvedBy.String {
		t.Fatalf("manual evidence status=%s provider=%v refunded=%v users=%v/%v", status, providerNo, refundedAt, submittedBy, approvedBy)
	}
	if afterLegacy := countLegacyCommissionsForOrder(t, ctx, db, fixture.orderID); afterLegacy != beforeLegacy {
		t.Fatalf("manual refund wrote Legacy projection before=%d after=%d", beforeLegacy, afterLegacy)
	}
	view, err := management.GetRefund(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if view.ServiceStatus == OperationCenterServiceActive || view.RefundStatus != OperationCenterRefundSucceeded || len(view.Audit) < 3 {
		t.Fatalf("refund view=%+v", view)
	}
}

func countLegacyCommissionsForOrder(t *testing.T, ctx context.Context, db *sql.DB, orderID string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_commissions WHERE order_id=$1`, orderID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
