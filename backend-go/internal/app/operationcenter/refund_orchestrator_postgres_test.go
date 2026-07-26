package operationcenter

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/payment"
)

type sagaMockProvider struct {
	mu            sync.Mutex
	outcomes      []RefundProviderResult
	queryOutcomes []payment.QueryRefundOutcome
	queryErrors   []error
	calls         int
	queryCalls    int
	refundNo      []string
	queryRefundNo []string
	started       chan struct{}
	release       chan struct{}
	afterQuery    func()
	once          sync.Once
}

func newSagaMockProvider(outcomes ...RefundProviderResult) *sagaMockProvider {
	return &sagaMockProvider{outcomes: outcomes}
}

func (provider *sagaMockProvider) GetProviderName() string { return "saga-mock" }

func (provider *sagaMockProvider) RefundPayment(ctx context.Context, request payment.RefundPaymentRequest) (payment.RefundPaymentResult, error) {
	provider.mu.Lock()
	index := provider.calls
	provider.calls++
	provider.refundNo = append(provider.refundNo, request.RefundNo)
	outcome := RefundProviderSuccess
	if len(provider.outcomes) > 0 {
		if index >= len(provider.outcomes) {
			index = len(provider.outcomes) - 1
		}
		outcome = provider.outcomes[index]
	}
	started, release := provider.started, provider.release
	provider.mu.Unlock()
	if started != nil {
		provider.once.Do(func() { close(started) })
	}
	if release != nil {
		select {
		case <-ctx.Done():
			return payment.RefundPaymentResult{}, ctx.Err()
		case <-release:
		}
	}
	switch outcome {
	case RefundProviderSuccess:
		return payment.RefundPaymentResult{Outcome: payment.RefundSuccess, ProviderRefundID: "provider_" + request.RefundNo, Status: payment.PaymentRefunded, ResponseSummary: payment.ProviderResponseSummary{"result": "SUCCESS"}}, nil
	case RefundProviderTemporaryFailure:
		return payment.RefundPaymentResult{Outcome: payment.RefundTemporaryFailure, ResponseSummary: payment.ProviderResponseSummary{"result": "TEMPORARY_FAILURE"}}, nil
	case RefundProviderUnsupported:
		return payment.RefundPaymentResult{Outcome: payment.RefundUnsupported, ResponseSummary: payment.ProviderResponseSummary{"result": "UNSUPPORTED"}}, nil
	case RefundProviderUnknown:
		return payment.RefundPaymentResult{Outcome: payment.RefundUnknown, ResponseSummary: payment.ProviderResponseSummary{"result": "UNKNOWN"}}, nil
	default:
		return payment.RefundPaymentResult{}, errors.New("invalid mock outcome")
	}
}

func (provider *sagaMockProvider) QueryRefund(_ context.Context, request payment.QueryRefundRequest) (payment.QueryRefundResult, error) {
	provider.mu.Lock()
	index := provider.queryCalls
	provider.queryCalls++
	provider.queryRefundNo = append(provider.queryRefundNo, request.RefundNo)
	outcome := payment.QueryRefundUnknown
	if len(provider.queryOutcomes) > 0 {
		if index >= len(provider.queryOutcomes) {
			index = len(provider.queryOutcomes) - 1
		}
		outcome = provider.queryOutcomes[index]
	}
	var queryErr error
	if len(provider.queryErrors) > 0 {
		errorIndex := provider.queryCalls - 1
		if errorIndex >= len(provider.queryErrors) {
			errorIndex = len(provider.queryErrors) - 1
		}
		queryErr = provider.queryErrors[errorIndex]
	}
	afterQuery := provider.afterQuery
	provider.mu.Unlock()
	result := payment.QueryRefundResult{Outcome: outcome, ResponseSummary: payment.ProviderResponseSummary{"result": string(outcome)}}
	if outcome == payment.QueryRefundSucceeded {
		result.ProviderRefundID = "provider_" + request.RefundNo
		completedAt := time.Now().UTC()
		result.CompletedAt = &completedAt
	}
	if afterQuery != nil {
		afterQuery()
	}
	return result, queryErr
}

func (provider *sagaMockProvider) CreatePayment(context.Context, payment.CreatePaymentRequest) (payment.CreatePaymentResult, error) {
	return payment.CreatePaymentResult{}, errors.New("not implemented")
}
func (provider *sagaMockProvider) QueryPayment(context.Context, payment.QueryPaymentRequest) (payment.PaymentStatus, error) {
	return "", errors.New("not implemented")
}
func (provider *sagaMockProvider) ClosePayment(context.Context, payment.QueryPaymentRequest) error {
	return errors.New("not implemented")
}
func (provider *sagaMockProvider) VerifyNotification(context.Context, []byte, map[string]string) (payment.PaymentNotification, error) {
	return payment.PaymentNotification{}, errors.New("not implemented")
}

func (provider *sagaMockProvider) snapshot() (int, []string) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.calls, append([]string(nil), provider.refundNo...)
}

func (provider *sagaMockProvider) querySnapshot() (int, []string) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.queryCalls, append([]string(nil), provider.queryRefundNo...)
}

func TestRefundOrchestratorActiveSuccessConservesFundsAndPermissionsPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture, _, rewardID, accountID, _, amount := prepareFrozenReleaseFixture(t, ctx, db, "refund_saga_active", 125000, 7)
	serviceID, taskID := createActiveRefundSagaTask(t, ctx, db, fixture)
	provider := newSagaMockProvider(RefundProviderSuccess)
	orchestrator := mustRefundOrchestrator(t, db, provider)
	command := refundSagaCommand(fixture, serviceID, taskID)

	result, err := orchestrator.Execute(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if result.ServiceStatus != OperationCenterServiceRevoked || result.RefundStatus != OperationCenterRefundSucceeded || result.ProviderOutcome == nil || *result.ProviderOutcome != RefundProviderSuccess || !result.ProviderCalled {
		t.Fatalf("active refund result=%+v", result)
	}
	assertRefundSagaTaskResult(t, ctx, db, taskID, OperationCenterRefundSucceeded, RefundProviderSuccess, true)
	assertRevokedOperationCenterResources(t, ctx, db, fixture, serviceID)
	assertWalletBuckets(t, ctx, db, accountID, 0, 0, 0, 0)
	var rewardStatus, releaseStatus string
	var reversals, ledgers int
	var reversalAmount, frozenDelta int64
	if err := db.QueryRowContext(ctx, `SELECT status FROM xz_referral_rewards WHERE id=$1`, rewardID).Scan(&rewardStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT release_status FROM xz_referral_reward_release_tasks WHERE referral_reward_id=$1`, rewardID).Scan(&releaseStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(reversal_amount_cents),0) FROM xz_referral_rewards WHERE reversal_of_id=$1`, rewardID).Scan(&reversals, &reversalAmount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(frozen_delta_cents),0) FROM xz_commission_wallet_ledger WHERE original_referral_reward_id=$1 AND business_type='REFERRAL_REWARD_REVERSAL'`, rewardID).Scan(&ledgers, &frozenDelta); err != nil {
		t.Fatal(err)
	}
	if rewardStatus != "REVERSED" || releaseStatus != "CANCELLED" || reversals != 1 || ledgers != 1 || reversalAmount != amount || frozenDelta != -amount {
		t.Fatalf("fund invariant reward=%s release=%s adjustments=%d ledgers=%d amounts=%d/%d", rewardStatus, releaseStatus, reversals, ledgers, reversalAmount, frozenDelta)
	}
	replay, err := orchestrator.Execute(ctx, command)
	if err != nil || !replay.IdempotentReplay {
		t.Fatalf("success replay=%+v err=%v", replay, err)
	}
	if calls, _ := provider.snapshot(); calls != 1 {
		t.Fatalf("duplicate provider refund calls=%d", calls)
	}
}

func TestRefundOrchestratorReviewRejectedAndProviderOutcomesPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	t.Run("review rejection uses same saga without reward reversal", func(t *testing.T) {
		fixture, serviceID, taskID := createRejectedRefundSagaFixture(t, ctx, db, "refund_saga_rejected_success")
		provider := newSagaMockProvider(RefundProviderSuccess)
		result, err := mustRefundOrchestrator(t, db, provider).Execute(ctx, refundSagaCommand(fixture, serviceID, taskID))
		if err != nil || result.ServiceStatus != OperationCenterServiceRejected || result.RefundStatus != OperationCenterRefundSucceeded {
			t.Fatalf("rejected success result=%+v err=%v", result, err)
		}
		var rewards, reversalLedgers int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_referral_rewards reward JOIN xz_referral_events event ON event.id=reward.referral_event_id WHERE event.source_order_id=$1`, fixture.orderID).Scan(&rewards); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_commission_wallet_ledger WHERE refund_task_id=$1`, taskID).Scan(&reversalLedgers); err != nil {
			t.Fatal(err)
		}
		if rewards != 0 || reversalLedgers != 0 {
			t.Fatalf("review rejection created funds rewards=%d ledgers=%d", rewards, reversalLedgers)
		}
	})

	t.Run("temporary failure waits then retries with same refund number", func(t *testing.T) {
		fixture, serviceID, taskID := createRejectedRefundSagaFixture(t, ctx, db, "refund_saga_temporary")
		provider := newSagaMockProvider(RefundProviderTemporaryFailure, RefundProviderSuccess)
		orchestrator := mustRefundOrchestrator(t, db, provider)
		command := refundSagaCommand(fixture, serviceID, taskID)
		result, err := orchestrator.Execute(ctx, command)
		if err != nil || result.RefundStatus != OperationCenterRefundRetryable {
			t.Fatalf("temporary result=%+v err=%v", result, err)
		}
		beforeRetry, err := orchestrator.Execute(ctx, command)
		if err != nil || !beforeRetry.ProviderCallSkipped {
			t.Fatalf("early retry=%+v err=%v", beforeRetry, err)
		}
		if calls, _ := provider.snapshot(); calls != 1 {
			t.Fatalf("early retry provider calls=%d", calls)
		}
		if _, err := db.ExecContext(ctx, `UPDATE xz_operation_center_refund_tasks SET next_retry_at=now()-interval '1 second' WHERE id=$1`, taskID); err != nil {
			t.Fatal(err)
		}
		result, err = orchestrator.Execute(ctx, command)
		if err != nil || result.RefundStatus != OperationCenterRefundSucceeded {
			t.Fatalf("retry success result=%+v err=%v", result, err)
		}
		calls, refundNumbers := provider.snapshot()
		if calls != 2 || len(refundNumbers) != 2 || refundNumbers[0] != refundNumbers[1] {
			t.Fatalf("unstable retry calls=%d refundNumbers=%v", calls, refundNumbers)
		}
	})

	t.Run("unsupported requires manual refund", func(t *testing.T) {
		fixture, serviceID, taskID := createRejectedRefundSagaFixture(t, ctx, db, "refund_saga_unsupported")
		provider := newSagaMockProvider(RefundProviderUnsupported)
		result, err := mustRefundOrchestrator(t, db, provider).Execute(ctx, refundSagaCommand(fixture, serviceID, taskID))
		if err != nil || result.RefundStatus != OperationCenterRefundManualRequired {
			t.Fatalf("unsupported result=%+v err=%v", result, err)
		}
		assertRefundSagaTaskResult(t, ctx, db, taskID, OperationCenterRefundManualRequired, RefundProviderUnsupported, false)
	})

	t.Run("unknown waits for query and never blindly retries", func(t *testing.T) {
		fixture, serviceID, taskID := createRejectedRefundSagaFixture(t, ctx, db, "refund_saga_unknown")
		provider := newSagaMockProvider(RefundProviderUnknown)
		orchestrator := mustRefundOrchestrator(t, db, provider)
		command := refundSagaCommand(fixture, serviceID, taskID)
		result, err := orchestrator.Execute(ctx, command)
		if err != nil || result.RefundStatus != OperationCenterRefundUnknownVerifying {
			t.Fatalf("unknown result=%+v err=%v", result, err)
		}
		replay, err := orchestrator.Execute(ctx, command)
		if err != nil || !replay.ProviderCallSkipped || !replay.IdempotentReplay {
			t.Fatalf("unknown replay=%+v err=%v", replay, err)
		}
		if calls, _ := provider.snapshot(); calls != 1 {
			t.Fatalf("unknown was blindly retried calls=%d", calls)
		}
		var unknownSince, nextRetry sql.NullTime
		if err := db.QueryRowContext(ctx, `SELECT unknown_since,next_retry_at FROM xz_operation_center_refund_tasks WHERE id=$1`, taskID).Scan(&unknownSince, &nextRetry); err != nil {
			t.Fatal(err)
		}
		if !unknownSince.Valid || nextRetry.Valid {
			t.Fatalf("unknown scheduling unknownSince=%v nextRetry=%v", unknownSince, nextRetry)
		}
	})
}

func TestRefundOrchestratorConcurrentInvocationCallsProviderOncePostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture, serviceID, taskID := createRejectedRefundSagaFixture(t, ctx, db, "refund_saga_concurrent")
	provider := newSagaMockProvider(RefundProviderSuccess)
	provider.started = make(chan struct{})
	provider.release = make(chan struct{})
	orchestrator := mustRefundOrchestrator(t, db, provider)
	command := refundSagaCommand(fixture, serviceID, taskID)
	results := make(chan RefundSagaResult, 1)
	errs := make(chan error, 1)
	go func() {
		result, err := orchestrator.Execute(ctx, command)
		results <- result
		errs <- err
	}()
	select {
	case <-provider.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	concurrent, err := orchestrator.Execute(ctx, command)
	if err != nil || !concurrent.InProgress || !concurrent.ProviderCallSkipped {
		t.Fatalf("concurrent result=%+v err=%v", concurrent, err)
	}
	close(provider.release)
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if result := <-results; result.RefundStatus != OperationCenterRefundSucceeded {
		t.Fatalf("first concurrent result=%+v", result)
	}
	if calls, _ := provider.snapshot(); calls != 1 {
		t.Fatalf("concurrent provider calls=%d", calls)
	}
}

func TestRefundOrchestratorActiveProviderFailuresRemainRevokedPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cases := []struct {
		name    string
		outcome RefundProviderResult
		status  OperationCenterRefundStatus
	}{
		{name: "temporary", outcome: RefundProviderTemporaryFailure, status: OperationCenterRefundRetryable},
		{name: "unsupported", outcome: RefundProviderUnsupported, status: OperationCenterRefundManualRequired},
		{name: "unknown", outcome: RefundProviderUnknown, status: OperationCenterRefundUnknownVerifying},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture, _, rewardID, _, _, _ := prepareFrozenReleaseFixture(t, ctx, db, "refund_saga_active_"+testCase.name, 33000, 7)
			serviceID, taskID := createActiveRefundSagaTask(t, ctx, db, fixture)
			provider := newSagaMockProvider(testCase.outcome)
			result, err := mustRefundOrchestrator(t, db, provider).Execute(ctx, refundSagaCommand(fixture, serviceID, taskID))
			if err != nil || result.ServiceStatus != OperationCenterServiceRevoked || result.RefundStatus != testCase.status {
				t.Fatalf("active %s result=%+v err=%v", testCase.outcome, result, err)
			}
			assertRevokedOperationCenterResources(t, ctx, db, fixture, serviceID)
			var rewardStatus string
			if err := db.QueryRowContext(ctx, `SELECT status FROM xz_referral_rewards WHERE id=$1`, rewardID).Scan(&rewardStatus); err != nil {
				t.Fatal(err)
			}
			if rewardStatus != "REVERSED" {
				t.Fatalf("active %s reward status=%s", testCase.outcome, rewardStatus)
			}
		})
	}
}

func TestRefundOrchestratorFirstTransactionFailureRollsBackAndSkipsProviderPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture, _, rewardID, accountID, _, amount := prepareFrozenReleaseFixture(t, ctx, db, "refund_saga_rollback", 44000, 7)
	serviceID, refundTaskID := createActiveRefundSagaTask(t, ctx, db, fixture)
	if _, err := db.ExecContext(ctx, `UPDATE xz_commission_wallet_accounts SET frozen_cents=$1 WHERE id=$2`, amount-1, accountID); err != nil {
		t.Fatal(err)
	}
	provider := newSagaMockProvider(RefundProviderSuccess)
	_, err := mustRefundOrchestrator(t, db, provider).Execute(ctx, refundSagaCommand(fixture, serviceID, refundTaskID))
	if !errors.Is(err, ErrFrozenBalanceInsufficient) {
		t.Fatalf("expected reversal failure, got %v", err)
	}
	if calls, _ := provider.snapshot(); calls != 0 {
		t.Fatalf("provider called before first transaction committed: %d", calls)
	}
	var serviceStatus, refundStatus, identityStatus, roleStatus, rewardStatus, releaseStatus string
	if err := db.QueryRowContext(ctx, `SELECT status,refund_status FROM xz_operation_center_service_orders WHERE id=$1`, serviceID).Scan(&serviceStatus, &refundStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT identity_status FROM xz_user_business_identities WHERE tenant_id=$1 AND user_id=$2 AND identity_type='OPERATION_CENTER' ORDER BY identity_version DESC LIMIT 1`, fixture.tenantID, fixture.userID).Scan(&identityStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT status FROM xz_user_roles WHERE user_id=$1 AND tenant_id=$2 AND role='OPERATION'`, fixture.userID, fixture.tenantID).Scan(&roleStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT status FROM xz_referral_rewards WHERE id=$1`, rewardID).Scan(&rewardStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT release_status FROM xz_referral_reward_release_tasks WHERE referral_reward_id=$1`, rewardID).Scan(&releaseStatus); err != nil {
		t.Fatal(err)
	}
	var taskStatus string
	if err := db.QueryRowContext(ctx, `SELECT refund_status FROM xz_operation_center_refund_tasks WHERE id=$1`, refundTaskID).Scan(&taskStatus); err != nil {
		t.Fatal(err)
	}
	if serviceStatus != "ACTIVE" || refundStatus != "PENDING" || taskStatus != "PENDING" || identityStatus != "ACTIVE" || roleStatus != "ACTIVE" || rewardStatus != "FROZEN" || releaseStatus != "PENDING" {
		t.Fatalf("rollback invariant service=%s/%s task=%s identity=%s role=%s reward=%s release=%s", serviceStatus, refundStatus, taskStatus, identityStatus, roleStatus, rewardStatus, releaseStatus)
	}
}

func TestRefundVerifierQueryOutcomesPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tests := []struct {
		name          string
		queryOutcome  payment.QueryRefundOutcome
		expected      OperationCenterRefundStatus
		requireNumber bool
	}{
		{name: "succeeded", queryOutcome: payment.QueryRefundSucceeded, expected: OperationCenterRefundSucceeded, requireNumber: true},
		{name: "processing", queryOutcome: payment.QueryRefundProcessing, expected: OperationCenterRefundUnknownVerifying},
		{name: "failed", queryOutcome: payment.QueryRefundFailed, expected: OperationCenterRefundRetryable},
		{name: "unsupported", queryOutcome: payment.QueryRefundUnsupported, expected: OperationCenterRefundManualRequired},
		{name: "unknown", queryOutcome: payment.QueryRefundUnknown, expected: OperationCenterRefundUnknownVerifying},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			fixture, serviceID, taskID := createRejectedRefundSagaFixture(t, ctx, db, "refund_verify_"+testCase.name)
			provider := newSagaMockProvider(RefundProviderUnknown)
			provider.queryOutcomes = []payment.QueryRefundOutcome{testCase.queryOutcome}
			orchestrator := mustRefundOrchestrator(t, db, provider)
			command := refundSagaCommand(fixture, serviceID, taskID)
			initial, err := orchestrator.Execute(ctx, command)
			if err != nil || initial.RefundStatus != OperationCenterRefundUnknownVerifying {
				t.Fatalf("initial unknown result=%+v err=%v", initial, err)
			}
			verified, err := orchestrator.VerifyUnknownRefund(ctx, command)
			if err != nil || verified.RefundStatus != testCase.expected {
				t.Fatalf("verify %s result=%+v err=%v", testCase.queryOutcome, verified, err)
			}
			calls, refundNumbers := provider.querySnapshot()
			_, paymentRefundNumbers := provider.snapshot()
			if calls != 1 || len(refundNumbers) != 1 || len(paymentRefundNumbers) != 1 || refundNumbers[0] != paymentRefundNumbers[0] {
				t.Fatalf("query stable refund number calls=%d query=%v refund=%v", calls, refundNumbers, paymentRefundNumbers)
			}
			var status, outcome string
			var providerRefundNo sql.NullString
			var querySummary []byte
			if err := db.QueryRowContext(ctx, `SELECT refund_status,provider_query_outcome,provider_refund_no,provider_query_response_summary FROM xz_operation_center_refund_tasks WHERE id=$1`, taskID).Scan(&status, &outcome, &providerRefundNo, &querySummary); err != nil {
				t.Fatal(err)
			}
			if status != string(testCase.expected) || outcome != string(testCase.queryOutcome) || providerRefundNo.Valid != testCase.requireNumber || len(querySummary) == 0 {
				t.Fatalf("persisted query result status=%s outcome=%s refundNo=%v summary=%s", status, outcome, providerRefundNo, querySummary)
			}
			if testCase.expected == OperationCenterRefundUnknownVerifying {
				replay, err := orchestrator.VerifyUnknownRefund(ctx, command)
				if err != nil || replay.RefundStatus != OperationCenterRefundUnknownVerifying {
					t.Fatalf("early verification replay=%+v err=%v", replay, err)
				}
				if queryCalls, _ := provider.querySnapshot(); queryCalls != 1 {
					t.Fatalf("early verification queried provider %d times", queryCalls)
				}
			}
		})
	}
}

func TestRefundVerifierNotFoundSafetyWaitAndStableRetryPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture, serviceID, taskID := createRejectedRefundSagaFixture(t, ctx, db, "refund_verify_not_found")
	provider := newSagaMockProvider(RefundProviderUnknown, RefundProviderSuccess)
	provider.queryOutcomes = []payment.QueryRefundOutcome{payment.QueryRefundNotFound, payment.QueryRefundNotFound}
	orchestrator := mustRefundOrchestrator(t, db, provider)
	command := refundSagaCommand(fixture, serviceID, taskID)
	if result, err := orchestrator.Execute(ctx, command); err != nil || result.RefundStatus != OperationCenterRefundUnknownVerifying {
		t.Fatalf("initial unknown result=%+v err=%v", result, err)
	}
	beforeSafety, err := orchestrator.VerifyUnknownRefund(ctx, command)
	if err != nil || beforeSafety.RefundStatus != OperationCenterRefundUnknownVerifying {
		t.Fatalf("not found before safety result=%+v err=%v", beforeSafety, err)
	}
	if calls, _ := provider.snapshot(); calls != 1 {
		t.Fatalf("NOT_FOUND retried RefundPayment before safety wait calls=%d", calls)
	}
	if _, err := db.ExecContext(ctx, `UPDATE xz_operation_center_refund_tasks SET unknown_since=now()-interval '31 minutes',next_retry_at=now()-interval '1 second' WHERE id=$1`, taskID); err != nil {
		t.Fatal(err)
	}
	afterSafety, err := orchestrator.VerifyUnknownRefund(ctx, command)
	if err != nil || afterSafety.RefundStatus != OperationCenterRefundProviderPending {
		t.Fatalf("not found after safety result=%+v err=%v", afterSafety, err)
	}
	result, err := orchestrator.Execute(ctx, command)
	if err != nil || result.RefundStatus != OperationCenterRefundSucceeded {
		t.Fatalf("stable retry result=%+v err=%v", result, err)
	}
	refundCalls, refundNumbers := provider.snapshot()
	queryCalls, queryRefundNumbers := provider.querySnapshot()
	if refundCalls != 2 || queryCalls != 2 || refundNumbers[0] != refundNumbers[1] || queryRefundNumbers[0] != refundNumbers[0] || queryRefundNumbers[1] != refundNumbers[0] {
		t.Fatalf("unstable safe retry refundCalls=%d queryCalls=%d refund=%v query=%v", refundCalls, queryCalls, refundNumbers, queryRefundNumbers)
	}
}

func TestRefundVerifierTimeoutAndSecondTransactionRecoveryPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture, serviceID, taskID := createRejectedRefundSagaFixture(t, ctx, db, "refund_verify_recovery")
	provider := newSagaMockProvider(RefundProviderUnknown)
	provider.queryOutcomes = []payment.QueryRefundOutcome{payment.QueryRefundUnknown, payment.QueryRefundSucceeded}
	provider.queryErrors = []error{context.DeadlineExceeded, nil}
	orchestrator := mustRefundOrchestrator(t, db, provider)
	command := refundSagaCommand(fixture, serviceID, taskID)
	if _, err := orchestrator.Execute(ctx, command); err != nil {
		t.Fatal(err)
	}
	result, err := orchestrator.VerifyUnknownRefund(ctx, command)
	if err != nil || result.RefundStatus != OperationCenterRefundUnknownVerifying {
		t.Fatalf("provider timeout result=%+v err=%v", result, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE xz_operation_center_refund_tasks SET next_retry_at=now()-interval '1 second' WHERE id=$1`, taskID); err != nil {
		t.Fatal(err)
	}
	verificationCtx, cancelVerification := context.WithCancel(ctx)
	provider.mu.Lock()
	provider.afterQuery = cancelVerification
	provider.mu.Unlock()
	if _, err := orchestrator.VerifyUnknownRefund(verificationCtx, command); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancelled second transaction, got %v", err)
	}
	recoveryCtx, cancelRecovery := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelRecovery()
	provider.mu.Lock()
	provider.afterQuery = nil
	provider.queryErrors = nil
	provider.mu.Unlock()
	if _, err := db.ExecContext(recoveryCtx, `UPDATE xz_operation_center_refund_tasks SET lease_expires_at=now()-interval '1 second',next_retry_at=now()-interval '1 second' WHERE id=$1`, taskID); err != nil {
		t.Fatal(err)
	}
	recovered, err := orchestrator.VerifyUnknownRefund(recoveryCtx, command)
	if err != nil || recovered.RefundStatus != OperationCenterRefundSucceeded {
		t.Fatalf("verification recovery result=%+v err=%v", recovered, err)
	}
	queryCalls, refundNumbers := provider.querySnapshot()
	if queryCalls != 3 || refundNumbers[0] != refundNumbers[1] || refundNumbers[1] != refundNumbers[2] {
		t.Fatalf("verification recovery was not stable calls=%d refundNumbers=%v", queryCalls, refundNumbers)
	}
}

func mustRefundOrchestrator(t *testing.T, db *sql.DB, provider payment.RefundProvider) *RefundOrchestrator {
	t.Helper()
	store, err := NewPostgresStore(db)
	if err != nil {
		t.Fatal(err)
	}
	orchestrator, err := NewRefundOrchestrator(db, store, NewReferralRewardReversalService(store), map[string]payment.RefundProvider{"mock": provider}, RefundOrchestratorOptions{ProviderLeaseDuration: 2 * time.Minute, TemporaryRetryDelay: 10 * time.Minute, UnknownSafetyWait: 30 * time.Minute, VerificationInterval: 5 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return orchestrator
}

func refundSagaCommand(fixture workflowFixture, serviceID, taskID string) RefundSagaCommand {
	return RefundSagaCommand{ServiceOrderID: serviceID, RefundTaskID: taskID, OperatorID: fixture.reviewerID, RequestID: fixture.id("refund_request"), TransactionGroupID: fixture.id("refund_saga_tx"), Reason: "OPERATION_CENTER_FULL_REFUND"}
}

func createRejectedRefundSagaFixture(t *testing.T, ctx context.Context, db *sql.DB, name string) (workflowFixture, string, string) {
	t.Helper()
	fixture := createWorkflowFixture(t, ctx, db, name)
	service := mustWorkflowService(t, db, WorkflowOptions{})
	paid, err := service.RecordPaymentSucceeded(ctx, PaymentSucceededCommand{OrderID: fixture.orderID, PaymentRecordID: fixture.paymentID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Review(ctx, ReviewCommand{ServiceOrderID: paid.ServiceOrder.ID, Decision: ReviewRejected, ExpectedStatus: OperationCenterServiceReviewRequired, IdempotencyKey: fixture.id("reject_review"), ReviewedBy: fixture.reviewerID, Reason: "REJECTED_FOR_TEST"}); err != nil {
		t.Fatal(err)
	}
	var taskID string
	if err := db.QueryRowContext(ctx, `SELECT current_refund_task_id FROM xz_operation_center_service_orders WHERE id=$1`, paid.ServiceOrder.ID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	return fixture, paid.ServiceOrder.ID, taskID
}

func createActiveRefundSagaTask(t *testing.T, ctx context.Context, db *sql.DB, fixture workflowFixture) (string, string) {
	t.Helper()
	store, err := NewPostgresStore(db)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := store.BindTx(tx)
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	var serviceID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM xz_operation_center_service_orders WHERE order_id=$1`, fixture.orderID).Scan(&serviceID); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	service, err := bound.GetServiceOrderForUpdate(ctx, serviceID)
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	now, err := databaseNow(ctx, tx)
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	taskID := fixture.id("active_refund_task")
	ruleSetID := ""
	if service.CommercialRuleSetID != nil {
		ruleSetID = *service.CommercialRuleSetID
	}
	task := &OperationCenterRefundTask{
		ID: taskID, TenantID: service.TenantID, ServiceOrderID: service.ID, OrderID: service.OrderID,
		PaymentRecordID: optionalString(fixture.paymentID), CommercialRuleSetID: ruleSetID,
		Origin: RefundOriginActiveRevocation, Scope: RefundScopeFull, AmountCents: service.TechnicalServiceFeeCents,
		Currency: service.Currency, PaymentChannel: "MOCK", ProviderPaymentNo: optionalString(fixture.id("provider_payment")),
		Status: OperationCenterRefundPending, FailureDetail: JSONSnapshot{}, IdempotencyKey: fixture.id("active_refund_key"), CreatedAt: now, UpdatedAt: now,
	}
	if err := bound.CreateRefundTask(ctx, task); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	service.RefundStatus = OperationCenterRefundPending
	service.CurrentRefundTaskID = optionalString(taskID)
	service.RefundIdempotencyKey = optionalString(task.IdempotencyKey)
	service.RefundOrderID = optionalString(taskID)
	service.StateVersion++
	service.UpdatedAt = now
	if err := bound.UpdateServiceOrderRefundProjection(ctx, service); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return service.ID, taskID
}

func assertRefundSagaTaskResult(t *testing.T, ctx context.Context, db *sql.DB, taskID string, status OperationCenterRefundStatus, outcome RefundProviderResult, requireRefundNo bool) {
	t.Helper()
	var actualStatus, actualOutcome string
	var providerRefundNo sql.NullString
	var refundedAt sql.NullTime
	if err := db.QueryRowContext(ctx, `SELECT refund_status,provider_outcome,provider_refund_no,provider_refunded_at FROM xz_operation_center_refund_tasks WHERE id=$1`, taskID).Scan(&actualStatus, &actualOutcome, &providerRefundNo, &refundedAt); err != nil {
		t.Fatal(err)
	}
	if actualStatus != string(status) || actualOutcome != string(outcome) || providerRefundNo.Valid != requireRefundNo || refundedAt.Valid != requireRefundNo {
		t.Fatalf("refund task status=%s outcome=%s refundNo=%v refundedAt=%v", actualStatus, actualOutcome, providerRefundNo, refundedAt)
	}
}

func assertRevokedOperationCenterResources(t *testing.T, ctx context.Context, db *sql.DB, fixture workflowFixture, serviceID string) {
	t.Helper()
	var serviceStatus, identityStatus, userStatus, profileStatus, roleStatus string
	var commissionEnabled bool
	if err := db.QueryRowContext(ctx, `SELECT status FROM xz_operation_center_service_orders WHERE id=$1`, serviceID).Scan(&serviceStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT identity_status,commission_enabled FROM xz_user_business_identities WHERE tenant_id=$1 AND user_id=$2 AND identity_type='OPERATION_CENTER' ORDER BY identity_version DESC LIMIT 1`, fixture.tenantID, fixture.userID).Scan(&identityStatus, &commissionEnabled); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT operation_center_status FROM xz_users WHERE id=$1`, fixture.userID).Scan(&userStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT status FROM xz_operation_centers WHERE user_id=$1`, fixture.userID).Scan(&profileStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT status FROM xz_user_roles WHERE user_id=$1 AND tenant_id=$2 AND role='OPERATION'`, fixture.userID, fixture.tenantID).Scan(&roleStatus); err != nil {
		t.Fatal(err)
	}
	if serviceStatus != "REVOKED" || identityStatus != "TERMINATED" || commissionEnabled || userStatus != "REVOKED" || profileStatus != "REVOKED" || roleStatus != "INACTIVE" {
		t.Fatalf("permission invariant service=%s identity=%s commission=%v user=%s profile=%s role=%s", serviceStatus, identityStatus, commissionEnabled, userStatus, profileStatus, roleStatus)
	}
}
