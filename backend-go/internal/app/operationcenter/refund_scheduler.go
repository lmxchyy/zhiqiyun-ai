package operationcenter

import (
	"context"
	"log/slog"
	"time"
)

type RefundSchedulerOptions struct {
	RetryEnabled, VerificationEnabled, ManualAutoApproval bool
	BatchLimit, MaxRetryAttempts, MaxVerificationAttempts int
	LeaseDuration                                         time.Duration
	WorkerName                                            string
	DryRun                                                bool
}

type RefundSchedulerRunResult struct {
	Scheduler, WorkerName  string
	Disabled, DryRun       bool
	Due, Claimed           int
	Succeeded, Retried     int
	ManualRequired, Failed int
	TaskIDs                []string
}

type RefundScheduler interface {
	RunRetryOnce(context.Context) (RefundSchedulerRunResult, error)
	RunVerificationOnce(context.Context) (RefundSchedulerRunResult, error)
}

func DefaultRefundSchedulerOptions() RefundSchedulerOptions {
	return RefundSchedulerOptions{
		RetryEnabled: false, VerificationEnabled: false, ManualAutoApproval: false,
		BatchLimit: 20, MaxRetryAttempts: 5, MaxVerificationAttempts: 12,
		LeaseDuration: 2 * time.Minute, WorkerName: "operation-center-refund-worker",
	}
}

func validateRefundSchedulerOptions(options RefundSchedulerOptions) error {
	if options.BatchLimit <= 0 || options.MaxRetryAttempts <= 0 || options.MaxVerificationAttempts <= 0 || options.LeaseDuration <= 0 || options.WorkerName == "" {
		return ErrConstraintViolation
	}
	return nil
}

func schedulerLogger(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.Default()
}
