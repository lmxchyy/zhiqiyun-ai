package operationcenter

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type RefundSchedulerService struct {
	db           *sql.DB
	store        *PostgresStore
	orchestrator *RefundOrchestrator
	options      RefundSchedulerOptions
	logger       *slog.Logger
}

func NewRefundSchedulerService(db *sql.DB, orchestrator *RefundOrchestrator, options RefundSchedulerOptions, logger *slog.Logger) (*RefundSchedulerService, error) {
	if db == nil || orchestrator == nil {
		return nil, ErrConstraintViolation
	}
	if err := validateRefundSchedulerOptions(options); err != nil {
		return nil, err
	}
	store, err := NewPostgresStore(db)
	if err != nil {
		return nil, err
	}
	return &RefundSchedulerService{db: db, store: store, orchestrator: orchestrator, options: options, logger: schedulerLogger(logger)}, nil
}

func (scheduler *RefundSchedulerService) RunRetryOnce(ctx context.Context) (RefundSchedulerRunResult, error) {
	result := RefundSchedulerRunResult{Scheduler: "refund_retry", WorkerName: scheduler.options.WorkerName, Disabled: !scheduler.options.RetryEnabled, DryRun: scheduler.options.DryRun}
	if !scheduler.options.RetryEnabled {
		return result, ErrRefundSchedulerDisabled
	}
	now, err := scheduler.databaseTime(ctx)
	if err != nil {
		return result, err
	}
	if scheduler.options.DryRun {
		result.Due, err = scheduler.countDue(ctx, OperationCenterRefundRetryable, now)
		return result, err
	}
	claimed, err := scheduler.claim(ctx, OperationCenterRefundRetryable, now)
	if err != nil {
		return result, err
	}
	result.Claimed = len(claimed)
	var firstErr error
	for _, task := range claimed {
		result.TaskIDs = append(result.TaskIDs, task.ID)
		if task.AttemptCount >= scheduler.options.MaxRetryAttempts {
			if err := scheduler.markManualRequired(ctx, task, "MAX_REFUND_RETRY_ATTEMPTS"); err != nil {
				result.Failed++
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			result.ManualRequired++
			continue
		}
		command := schedulerCommand(task, scheduler.options.WorkerName, "REFUND_RETRY_SCHEDULER")
		execution, executeErr := scheduler.orchestrator.Execute(ctx, command)
		if executeErr != nil {
			result.Failed++
			if firstErr == nil {
				firstErr = executeErr
			}
			scheduler.logger.WarnContext(ctx, "operation center refund retry failed", "task_id", task.ID, "status", task.Status, "worker", scheduler.options.WorkerName)
			continue
		}
		switch execution.RefundStatus {
		case OperationCenterRefundSucceeded:
			result.Succeeded++
		case OperationCenterRefundManualRequired:
			result.ManualRequired++
		default:
			result.Retried++
		}
	}
	return result, firstErr
}

func (scheduler *RefundSchedulerService) RunVerificationOnce(ctx context.Context) (RefundSchedulerRunResult, error) {
	result := RefundSchedulerRunResult{Scheduler: "refund_verification", WorkerName: scheduler.options.WorkerName, Disabled: !scheduler.options.VerificationEnabled, DryRun: scheduler.options.DryRun}
	if !scheduler.options.VerificationEnabled {
		return result, ErrRefundSchedulerDisabled
	}
	now, err := scheduler.databaseTime(ctx)
	if err != nil {
		return result, err
	}
	if scheduler.options.DryRun {
		result.Due, err = scheduler.countDue(ctx, OperationCenterRefundUnknownVerifying, now)
		return result, err
	}
	claimed, err := scheduler.claim(ctx, OperationCenterRefundUnknownVerifying, now)
	if err != nil {
		return result, err
	}
	result.Claimed = len(claimed)
	var firstErr error
	for _, task := range claimed {
		result.TaskIDs = append(result.TaskIDs, task.ID)
		if task.VerificationAttemptCount >= scheduler.options.MaxVerificationAttempts {
			if err := scheduler.markManualRequired(ctx, task, "MAX_REFUND_VERIFICATION_ATTEMPTS"); err != nil {
				result.Failed++
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			result.ManualRequired++
			continue
		}
		command := schedulerCommand(task, scheduler.options.WorkerName, "REFUND_VERIFICATION_SCHEDULER")
		verification, verifyErr := scheduler.orchestrator.VerifyUnknownRefund(ctx, command)
		if verifyErr != nil {
			result.Failed++
			if firstErr == nil {
				firstErr = verifyErr
			}
			scheduler.logger.WarnContext(ctx, "operation center refund verification failed", "task_id", task.ID, "status", task.Status, "worker", scheduler.options.WorkerName)
			continue
		}
		if verification.RefundStatus == OperationCenterRefundUnknownVerifying {
			current, getErr := scheduler.store.GetRefundTask(ctx, task.ID)
			if getErr != nil {
				result.Failed++
				if firstErr == nil {
					firstErr = getErr
				}
				continue
			}
			if current.VerificationAttemptCount >= scheduler.options.MaxVerificationAttempts {
				if err := scheduler.markManualRequired(ctx, *current, "MAX_REFUND_VERIFICATION_ATTEMPTS"); err != nil {
					result.Failed++
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
				result.ManualRequired++
				continue
			}
		}
		switch verification.RefundStatus {
		case OperationCenterRefundSucceeded:
			result.Succeeded++
		case OperationCenterRefundManualRequired:
			result.ManualRequired++
		default:
			result.Retried++
		}
	}
	return result, firstErr
}

func (scheduler *RefundSchedulerService) claim(ctx context.Context, status OperationCenterRefundStatus, now time.Time) ([]OperationCenterRefundTask, error) {
	tx, err := scheduler.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	boundValue, err := scheduler.store.BindTx(tx)
	if err != nil {
		return nil, err
	}
	bound := boundValue.(Store)
	leaseUntil := now.Add(scheduler.options.LeaseDuration)
	var tasks []OperationCenterRefundTask
	if status == OperationCenterRefundRetryable {
		tasks, err = bound.ClaimRetryableRefundTasks(ctx, now, scheduler.options.WorkerName, leaseUntil, scheduler.options.BatchLimit)
	} else {
		tasks, err = bound.ClaimVerificationRefundTasks(ctx, now, scheduler.options.WorkerName, leaseUntil, scheduler.options.BatchLimit)
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (scheduler *RefundSchedulerService) markManualRequired(ctx context.Context, claimed OperationCenterRefundTask, reason string) error {
	tx, err := scheduler.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	boundValue, err := scheduler.store.BindTx(tx)
	if err != nil {
		return err
	}
	bound := boundValue.(Store)
	service, err := bound.GetServiceOrderForUpdate(ctx, claimed.ServiceOrderID)
	if err != nil {
		return err
	}
	task, err := bound.GetRefundTaskForUpdate(ctx, claimed.ID)
	if err != nil {
		return err
	}
	if task.LeaseOwner == nil || *task.LeaseOwner != scheduler.options.WorkerName {
		return ErrRefundSagaInProgress
	}
	if task.Status != OperationCenterRefundRetryable && task.Status != OperationCenterRefundUnknownVerifying {
		return nil
	}
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return err
	}
	failure := RefundFailureManualRequired
	task.FailureClass = &failure
	task.FailureDetail = JSONSnapshot{"reason": reason}
	task.LeaseOwner, task.LeaseExpiresAt, task.NextRetryAt = nil, nil, nil
	key := stableWorkflowID("operation_center_scheduler_limit", task.ID, reason, fmt.Sprint(task.StateVersion))
	if err := advanceManagedRefundTask(ctx, bound, task, OperationCenterRefundManualRequired, reason, scheduler.options.WorkerName, key, key, reason, now); err != nil {
		return err
	}
	service.RefundStatus = task.Status
	service.RefundFailureClass = task.FailureClass
	service.RefundFailureDetail = task.FailureDetail
	service.NextRefundRetryAt = nil
	service.StateVersion++
	service.UpdatedAt = now
	if err := bound.UpdateServiceOrderRefundProjection(ctx, service); err != nil {
		return err
	}
	return tx.Commit()
}

func (scheduler *RefundSchedulerService) databaseTime(ctx context.Context) (time.Time, error) {
	var now time.Time
	err := scheduler.db.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now)
	return now.UTC(), err
}

func (scheduler *RefundSchedulerService) countDue(ctx context.Context, status OperationCenterRefundStatus, now time.Time) (int, error) {
	var count int
	err := scheduler.db.QueryRowContext(ctx, `SELECT count(*) FROM xz_operation_center_refund_tasks WHERE refund_status=$1 AND (next_retry_at IS NULL OR next_retry_at<=$2) AND (lease_expires_at IS NULL OR lease_expires_at<=$2)`, status, now).Scan(&count)
	return count, err
}

func schedulerCommand(task OperationCenterRefundTask, worker, reason string) RefundSagaCommand {
	group := strings.TrimSpace(worker)
	return RefundSagaCommand{
		ServiceOrderID: task.ServiceOrderID, RefundTaskID: task.ID, OperatorID: worker,
		RequestID:          stableWorkflowID("operation_center_scheduler_request", task.ID, fmt.Sprint(task.StateVersion)),
		TransactionGroupID: group, Reason: reason,
	}
}
