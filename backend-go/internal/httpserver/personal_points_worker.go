package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
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

func personalPointExpiryWorkerEnabled() (bool, error) {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_ENABLED")))
	switch value {
	case "":
		return false, nil
	case "1", "true", "on", "enabled":
		return true, nil
	case "0", "false", "off", "disabled":
		return false, nil
	default:
		return false, errors.New("invalid XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_ENABLED")
	}
}

func personalPointExpiryWorkerOptions() (time.Duration, int, error) {
	interval := time.Minute
	if raw := strings.TrimSpace(os.Getenv("XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_INTERVAL")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return 0, 0, errors.New("invalid XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_INTERVAL")
		}
		interval = parsed
	}
	batch := 100
	if raw := strings.TrimSpace(os.Getenv("XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_BATCH")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > 1000 {
			return 0, 0, errors.New("invalid XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_BATCH")
		}
		batch = parsed
	}
	return interval, batch, nil
}

func configurePersonalPointExpiryWorker(server *http.Server, store platformStore, logger *slog.Logger) (*personalPointExpiryWorker, error) {
	pgStore, ok := store.(*postgresStore)
	if !ok || pgStore == nil || server == nil {
		return nil, nil
	}
	enabled, err := personalPointExpiryWorkerEnabled()
	if err != nil || !enabled {
		return nil, err
	}
	interval, batch, err := personalPointExpiryWorkerOptions()
	if err != nil {
		return nil, err
	}
	worker := newPersonalPointExpiryWorker(pgStore.PersonalPointService(), interval, batch, logger)
	worker.Start()
	server.RegisterOnShutdown(worker.Stop)
	return worker, nil
}
