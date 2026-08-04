package httpserver

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type personalPointExpiryWorker struct {
	service  *PersonalPointService
	interval time.Duration
	batch    int
	logger   *slog.Logger

	startOnce sync.Once
	stopOnce  sync.Once
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

func newPersonalPointExpiryWorker(service *PersonalPointService, interval time.Duration, batch int, logger *slog.Logger) *personalPointExpiryWorker {
	if interval <= 0 {
		interval = time.Minute
	}
	if batch <= 0 {
		batch = 100
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &personalPointExpiryWorker{service: service, interval: interval, batch: batch, logger: logger}
}

func (w *personalPointExpiryWorker) RunOnce(ctx context.Context, now time.Time) (PersonalPointExpiryBatchResult, error) {
	if w == nil || w.service == nil {
		return PersonalPointExpiryBatchResult{}, ErrInvalidPointCommand
	}
	result, err := w.service.ExpireDue(ctx, now, w.batch)
	if err != nil {
		return result, err
	}
	if result.AccountsProcessed > 0 {
		w.logger.Info("personal point expiry batch completed", "accountsProcessed", result.AccountsProcessed, "pointsExpired", result.PointsExpired)
	}
	return result, nil
}

func (w *personalPointExpiryWorker) Start() {
	if w == nil || w.service == nil {
		return
	}
	w.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		w.cancel = cancel
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			ticker := time.NewTicker(w.interval)
			defer ticker.Stop()
			w.logger.Info("personal point expiry worker started", "interval", w.interval.String(), "batchSize", w.batch)
			for {
				select {
				case <-ctx.Done():
					w.logger.Info("personal point expiry worker stopped")
					return
				case now := <-ticker.C:
					if _, err := w.RunOnce(ctx, now.UTC()); err != nil && ctx.Err() == nil {
						w.logger.Error("personal point expiry batch failed", "error", err)
					}
				}
			}
		}()
	})
}

func (w *personalPointExpiryWorker) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
		w.wg.Wait()
	})
}

func personalPointExpiryWorkerEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_ENABLED")))
	return value != "0" && value != "false" && value != "off" && value != "disabled"
}

func personalPointExpiryWorkerOptions() (time.Duration, int) {
	interval := time.Minute
	if raw := strings.TrimSpace(os.Getenv("XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_INTERVAL")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			interval = parsed
		}
	}
	batch := 100
	if raw := strings.TrimSpace(os.Getenv("XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_BATCH")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 1000 {
			batch = parsed
		}
	}
	return interval, batch
}
