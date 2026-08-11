package operationcenter

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type RuntimeSchedulerGroup struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once
}

func (runtime *OperationCenterRuntime) StartSchedulers(parent context.Context, logger *slog.Logger) (*RuntimeSchedulerGroup, error) {
	if runtime == nil {
		return nil, ErrWorkflowUnavailable
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := runtime.config.Validate(); err != nil {
		return nil, err
	}
	if err := runtime.validateSchedulerProviders(parent); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	group := &RuntimeSchedulerGroup{cancel: cancel}
	specs := []struct {
		name     string
		enabled  bool
		interval time.Duration
		run      func(context.Context) error
	}{
		{name: "refund_retry", enabled: runtime.config.RefundRetrySchedulerEnabled, interval: runtime.config.RefundRetrySchedulerInterval, run: func(ctx context.Context) error {
			_, err := runtime.RunRefundRetryOnce(ctx)
			return err
		}},
		{name: "refund_verification", enabled: runtime.config.RefundVerificationEnabled, interval: runtime.config.RefundVerificationInterval, run: func(ctx context.Context) error {
			_, err := runtime.RunRefundVerificationOnce(ctx)
			return err
		}},
		{name: "referral_reward_release", enabled: runtime.config.RewardReleaseSchedulerEnabled, interval: runtime.config.RewardReleaseSchedulerInterval, run: func(ctx context.Context) error {
			_, err := runtime.RunReferralRewardReleaseOnce(ctx)
			return err
		}},
	}
	for _, spec := range specs {
		logger.Info("operation center scheduler configured",
			"scheduler", spec.name, "enabled", spec.enabled, "dry_run", runtime.config.DryRun,
			"worker", runtime.config.WorkerName, "batch_limit", runtime.config.BatchLimit,
			"interval", spec.interval.String(), "provider_lease", runtime.config.ProviderLeaseDuration.String())
		if !spec.enabled {
			continue
		}
		group.wg.Add(1)
		go runtimeSchedulerLoop(ctx, &group.wg, logger, spec.name, spec.interval, spec.run)
	}
	return group, nil
}

func (runtime *OperationCenterRuntime) validateSchedulerProviders(ctx context.Context) error {
	if runtime.config.DryRun {
		return nil
	}
	if (runtime.config.RefundRetrySchedulerEnabled || runtime.config.RefundVerificationEnabled) && len(runtime.providers) == 0 {
		return ErrRefundProviderUnavailable
	}
	if runtime.config.RefundRetrySchedulerEnabled {
		if err := runtime.requireProvidersForDueTasks(ctx, OperationCenterRefundRetryable, runtime.config.MaxRetryAttempts, false); err != nil {
			return err
		}
	}
	if runtime.config.RefundVerificationEnabled {
		if err := runtime.requireProvidersForDueTasks(ctx, OperationCenterRefundUnknownVerifying, runtime.config.MaxVerificationAttempts, true); err != nil {
			return err
		}
	}
	return nil
}

func runtimeSchedulerLoop(ctx context.Context, wg *sync.WaitGroup, logger *slog.Logger, name string, interval time.Duration, run func(context.Context) error) {
	defer wg.Done()
	logger.Info("operation center scheduler started", "scheduler", name)
	defer logger.Info("operation center scheduler stopped", "scheduler", name)
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if err := runRuntimeSchedulerRound(ctx, run); err != nil && ctx.Err() == nil {
				logger.Error("operation center scheduler round failed", "scheduler", name, "error", err)
			}
			timer.Reset(interval)
		}
	}
}

func runRuntimeSchedulerRound(ctx context.Context, run func(context.Context) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("scheduler panic recovered: %v", recovered)
		}
	}()
	if run == nil {
		return ErrConstraintViolation
	}
	return run(ctx)
}

func (group *RuntimeSchedulerGroup) Stop() {
	if group == nil {
		return
	}
	group.once.Do(func() {
		group.cancel()
		group.wg.Wait()
	})
}
