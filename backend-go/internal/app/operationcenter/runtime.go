package operationcenter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"xianzhi-ai/backend-go/internal/app/payment"
)

type RefundProviderBinding struct {
	PaymentChannel string
	Provider       payment.RefundProvider
}

type ReferralRewardReleaseSchedulerRunResult struct {
	WorkerName                      string
	Disabled, DryRun                bool
	Due, Claimed, Succeeded, Failed int
	TaskIDs                         []string
}

type OperationCenterRuntime struct {
	db                           *sql.DB
	config                       OperationCenterRuntimeConfig
	providers                    map[string]payment.RefundProvider
	RefundOrchestrator           *RefundOrchestrator
	RefundRetryScheduler         *RefundSchedulerService
	RefundVerificationScheduler  *RefundSchedulerService
	ReferralRewardReleaseService *ReferralRewardReleaseService
}

func NewOperationCenterRuntime(db *sql.DB, config OperationCenterRuntimeConfig, logger *slog.Logger, bindings ...RefundProviderBinding) (*OperationCenterRuntime, error) {
	if db == nil {
		return nil, ErrConstraintViolation
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	providers := make(map[string]payment.RefundProvider, len(bindings))
	for _, binding := range bindings {
		channel := normalizePaymentChannel(binding.PaymentChannel)
		if channel == "" || binding.Provider == nil {
			return nil, ErrConstraintViolation
		}
		if _, exists := providers[channel]; exists {
			return nil, fmt.Errorf("%w: duplicate payment channel %s", ErrUniqueConflict, channel)
		}
		providers[channel] = binding.Provider
	}
	store, err := NewPostgresStore(db)
	if err != nil {
		return nil, err
	}
	orchestrator, err := NewRefundOrchestrator(db, store, NewReferralRewardReversalService(store), providers, RefundOrchestratorOptions{
		ProviderLeaseDuration: config.ProviderLeaseDuration, TemporaryRetryDelay: config.TemporaryRetryDelay,
		UnknownSafetyWait: config.UnknownSafetyWait, VerificationInterval: config.VerificationInterval,
	})
	if err != nil {
		return nil, err
	}
	base := DefaultRefundSchedulerOptions()
	base.BatchLimit, base.MaxRetryAttempts, base.MaxVerificationAttempts = config.BatchLimit, config.MaxRetryAttempts, config.MaxVerificationAttempts
	base.LeaseDuration, base.DryRun, base.ManualAutoApproval = config.ProviderLeaseDuration, config.DryRun, false
	retryOptions := base
	retryOptions.RetryEnabled, retryOptions.VerificationEnabled = config.RefundRetrySchedulerEnabled, false
	retryOptions.WorkerName = strings.TrimSpace(config.WorkerName) + "-refund-retry"
	retryScheduler, err := NewRefundSchedulerService(db, orchestrator, retryOptions, logger)
	if err != nil {
		return nil, err
	}
	verificationOptions := base
	verificationOptions.RetryEnabled, verificationOptions.VerificationEnabled = false, config.RefundVerificationEnabled
	verificationOptions.WorkerName = strings.TrimSpace(config.WorkerName) + "-refund-verification"
	verificationScheduler, err := NewRefundSchedulerService(db, orchestrator, verificationOptions, logger)
	if err != nil {
		return nil, err
	}
	releaseService, err := NewReferralRewardReleaseService(db, ReferralRewardReleaseOptions{LeaseDuration: config.RewardReleaseLeaseDuration, RetryDelay: config.RewardReleaseRetryDelay})
	if err != nil {
		return nil, err
	}
	return &OperationCenterRuntime{
		db: db, config: config, providers: providers, RefundOrchestrator: orchestrator,
		RefundRetryScheduler: retryScheduler, RefundVerificationScheduler: verificationScheduler,
		ReferralRewardReleaseService: releaseService,
	}, nil
}

func (runtime *OperationCenterRuntime) Execute(ctx context.Context, command RefundSagaCommand) (RefundSagaResult, error) {
	if runtime == nil || runtime.RefundOrchestrator == nil {
		return RefundSagaResult{}, ErrWorkflowUnavailable
	}
	if err := runtime.requireProviderForTask(ctx, command.RefundTaskID); err != nil {
		return RefundSagaResult{}, err
	}
	return runtime.RefundOrchestrator.Execute(ctx, command)
}

func (runtime *OperationCenterRuntime) RunRefundRetryOnce(ctx context.Context) (RefundSchedulerRunResult, error) {
	if runtime == nil || runtime.RefundRetryScheduler == nil {
		return RefundSchedulerRunResult{}, ErrWorkflowUnavailable
	}
	if runtime.config.RefundRetrySchedulerEnabled && !runtime.config.DryRun {
		if err := runtime.requireProvidersForDueTasks(ctx, OperationCenterRefundRetryable, runtime.config.MaxRetryAttempts, false); err != nil {
			return RefundSchedulerRunResult{Scheduler: "refund_retry", WorkerName: runtime.config.WorkerName + "-refund-retry"}, err
		}
	}
	return runtime.RefundRetryScheduler.RunRetryOnce(ctx)
}

func (runtime *OperationCenterRuntime) RunRefundVerificationOnce(ctx context.Context) (RefundSchedulerRunResult, error) {
	if runtime == nil || runtime.RefundVerificationScheduler == nil {
		return RefundSchedulerRunResult{}, ErrWorkflowUnavailable
	}
	if runtime.config.RefundVerificationEnabled && !runtime.config.DryRun {
		if err := runtime.requireProvidersForDueTasks(ctx, OperationCenterRefundUnknownVerifying, runtime.config.MaxVerificationAttempts, true); err != nil {
			return RefundSchedulerRunResult{Scheduler: "refund_verification", WorkerName: runtime.config.WorkerName + "-refund-verification"}, err
		}
	}
	return runtime.RefundVerificationScheduler.RunVerificationOnce(ctx)
}

func (runtime *OperationCenterRuntime) RunReferralRewardReleaseOnce(ctx context.Context) (ReferralRewardReleaseSchedulerRunResult, error) {
	if runtime == nil || runtime.ReferralRewardReleaseService == nil {
		return ReferralRewardReleaseSchedulerRunResult{}, ErrWorkflowUnavailable
	}
	result := ReferralRewardReleaseSchedulerRunResult{WorkerName: runtime.config.WorkerName + "-reward-release", Disabled: !runtime.config.RewardReleaseSchedulerEnabled, DryRun: runtime.config.DryRun}
	if !runtime.config.RewardReleaseSchedulerEnabled {
		return result, ErrRewardReleaseSchedulerDisabled
	}
	if runtime.config.DryRun {
		err := runtime.db.QueryRowContext(ctx, `
			SELECT count(*) FROM xz_referral_reward_release_tasks
			WHERE release_status IN ('PENDING','FAILED') AND execute_at<=clock_timestamp()
			  AND (next_retry_at IS NULL OR next_retry_at<=clock_timestamp())
			  AND (lease_expires_at IS NULL OR lease_expires_at<=clock_timestamp())
		`).Scan(&result.Due)
		return result, mapPostgresStoreError("count due referral reward releases", err)
	}
	batch, err := runtime.ReferralRewardReleaseService.ClaimAndReleaseDueRewards(ctx, result.WorkerName, runtime.config.BatchLimit)
	result.Claimed, result.Succeeded, result.Failed = batch.Claimed, batch.Succeeded, batch.Failed
	for _, item := range batch.Results {
		result.TaskIDs = append(result.TaskIDs, item.TaskID)
	}
	return result, err
}

func (runtime *OperationCenterRuntime) HasRefundProvider(paymentChannel string) bool {
	if runtime == nil {
		return false
	}
	return runtime.providers[normalizePaymentChannel(paymentChannel)] != nil
}

func (runtime *OperationCenterRuntime) requireProviderForTask(ctx context.Context, taskID string) error {
	if strings.TrimSpace(taskID) == "" {
		return ErrConstraintViolation
	}
	var status, channel string
	err := runtime.db.QueryRowContext(ctx, `SELECT refund_status,payment_channel FROM xz_operation_center_refund_tasks WHERE id=$1`, taskID).Scan(&status, &channel)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return mapPostgresStoreError("load refund task provider", err)
	}
	switch OperationCenterRefundStatus(status) {
	case OperationCenterRefundSucceeded, OperationCenterRefundUnknownVerifying, OperationCenterRefundManualRequired, OperationCenterRefundManualSubmitted:
		return nil
	default:
		return runtime.requireProvider(channel)
	}
}

func (runtime *OperationCenterRuntime) requireProvidersForDueTasks(ctx context.Context, status OperationCenterRefundStatus, maxAttempts int, verification bool) error {
	attemptColumn := "attempt_count"
	if verification {
		attemptColumn = "verification_attempt_count"
	}
	query := fmt.Sprintf(`
		SELECT DISTINCT payment_channel FROM xz_operation_center_refund_tasks
		WHERE refund_status=$1 AND (next_retry_at IS NULL OR next_retry_at<=clock_timestamp())
		  AND (lease_expires_at IS NULL OR lease_expires_at<=clock_timestamp()) AND %s<$2
		ORDER BY payment_channel LIMIT $3
	`, attemptColumn)
	rows, err := runtime.db.QueryContext(ctx, query, status, maxAttempts, runtime.config.BatchLimit)
	if err != nil {
		return mapPostgresStoreError("list due refund task providers", err)
	}
	defer rows.Close()
	for rows.Next() {
		var channel string
		if err := rows.Scan(&channel); err != nil {
			return mapPostgresStoreError("scan due refund task provider", err)
		}
		if err := runtime.requireProvider(channel); err != nil {
			return err
		}
	}
	return mapPostgresStoreError("iterate due refund task providers", rows.Err())
}

func (runtime *OperationCenterRuntime) requireProvider(paymentChannel string) error {
	channel := normalizePaymentChannel(paymentChannel)
	if channel == "" || runtime.providers[channel] == nil {
		return fmt.Errorf("%w: payment_channel=%s", ErrRefundProviderUnavailable, strings.TrimSpace(paymentChannel))
	}
	return nil
}

func normalizePaymentChannel(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
