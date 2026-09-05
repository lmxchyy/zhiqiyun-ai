package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/httpserver"
	"xianzhi-ai/backend-go/internal/messaging"
)

const (
	asyncStatusDisabled   = "DISABLED"
	asyncStatusConnecting = "CONNECTING"
	asyncStatusReady      = "READY"
	asyncStatusDegraded   = "DEGRADED"
	asyncStatusStopped    = "STOPPED"
)

// asyncMessagingRuntime owns startup and shutdown order for the opt-in business
// messaging workers. It never blocks API startup on RabbitMQ availability.
type asyncMessagingRuntime struct {
	enabled   bool
	manager   *messaging.ConnectionManager
	publisher *messaging.Publisher

	mu         sync.Mutex
	started    bool
	stopped    bool
	everReady  bool
	runtimeErr error
	cancel     context.CancelFunc
	wg         sync.WaitGroup

	db     *sql.DB
	cfg    config.Config
	worker func(context.Context, config.Config, *sql.DB, *messaging.ConnectionManager) error
}

func newAsyncMessagingRuntime(cfg config.Config, db *sql.DB, manager *messaging.ConnectionManager) *asyncMessagingRuntime {
	return &asyncMessagingRuntime{
		enabled: cfg.AsyncMessagingEnabled, manager: manager, db: db, cfg: cfg,
		worker: httpserver.RunGenerationImageCanaryWorker,
	}
}

func (r *asyncMessagingRuntime) Start(parent context.Context) {
	if r == nil || !r.enabled {
		return
	}
	r.mu.Lock()
	if r.started || r.stopped {
		r.mu.Unlock()
		return
	}
	r.started = true
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	if r.manager == nil || r.db == nil {
		r.runtimeErr = fmt.Errorf("async messaging requires PostgreSQL and RabbitMQ manager")
		r.mu.Unlock()
		return
	}
	r.publisher = messaging.NewPublisher(r.manager)
	publisher := &messaging.OutboxPublisher{
		Store: messaging.NewOutboxStore(r.db), Publisher: r.publisher,
		BatchSize: 25, PollInterval: time.Second, Owner: "api-generation-outbox",
	}
	r.mu.Unlock()

	r.manager.Start()
	r.run(ctx, "outbox publisher", publisher.Run)
	r.run(ctx, "generation canary consumer", func(ctx context.Context) error {
		return r.worker(ctx, r.cfg, r.db, r.manager)
	})
	r.run(ctx, "generation video canary consumer", func(ctx context.Context) error {
		return httpserver.RunGenerationVideoCanaryWorker(ctx, r.cfg, r.db, r.manager)
	})
	r.run(ctx, "generation ppt canary consumer", func(ctx context.Context) error {
		return httpserver.RunGenerationPPTCanaryWorker(ctx, r.cfg, r.db, r.manager)
	})
}

func (r *asyncMessagingRuntime) run(ctx context.Context, name string, fn func(context.Context) error) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		if err := fn(ctx); err != nil && !errors.Is(err, context.Canceled) && ctx.Err() == nil {
			r.mu.Lock()
			r.runtimeErr = fmt.Errorf("%s stopped: %w", name, err)
			r.mu.Unlock()
		}
	}()
}

func (r *asyncMessagingRuntime) Status() string {
	if r == nil || !r.enabled {
		return asyncStatusDisabled
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return asyncStatusStopped
	}
	if r.runtimeErr != nil {
		return asyncStatusDegraded
	}
	if r.manager == nil {
		return asyncStatusDegraded
	}
	if r.manager.IsConnected() {
		r.everReady = true
		return asyncStatusReady
	}
	if r.everReady {
		return asyncStatusDegraded
	}
	return asyncStatusConnecting
}

// Stop first stops claiming/consuming, then closes publisher channels, then the
// connection owner. The caller controls the graceful bound.
func (r *asyncMessagingRuntime) Stop(ctx context.Context) error {
	if r == nil || !r.enabled {
		return nil
	}
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return nil
	}
	r.stopped = true
	cancel, publisher, manager := r.cancel, r.publisher, r.manager
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	var waitErr error
	select {
	case <-done:
	case <-ctx.Done():
		waitErr = fmt.Errorf("async workers shutdown: %w", ctx.Err())
	}
	var closeErrs []error
	if publisher != nil {
		if err := publisher.Close(); err != nil {
			closeErrs = append(closeErrs, err)
		}
	}
	if manager != nil {
		if err := manager.Close(); err != nil {
			closeErrs = append(closeErrs, err)
		}
	}
	closeErrs = append(closeErrs, waitErr)
	return errors.Join(closeErrs...)
}
