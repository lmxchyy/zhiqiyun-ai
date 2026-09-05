package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"errors"
	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/messaging"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

const generationImageCanaryConsumer = "generation-image-canary-worker"

// generationCanaryDrainEnabled is intentionally independent of the new-submit
// canary flag. Operators can stop selection while existing durable work drains.
func generationCanaryDrainEnabled(cfg config.Config) bool {
	return cfg.AsyncMessagingEnabled && cfg.ProviderExecutionSafetyEnabled
}

// RunGenerationImageCanaryWorker consumes only the opt-in image canary queue.
// Provider calls remain behind the same API ProviderExecution hook and local
// completion is performed by runGenerationTask.
func RunGenerationImageCanaryWorker(ctx context.Context, cfg config.Config, db *sql.DB, manager *messaging.ConnectionManager) error {
	if db == nil || manager == nil {
		return fmt.Errorf("generation worker dependencies are required")
	}
	if !generationCanaryDrainEnabled(cfg) {
		return fmt.Errorf("ASYNC_MESSAGING_ENABLED and PROVIDER_EXECUTION_SAFETY_ENABLED must be true")
	}
	store := newPostgresPrimaryStore(db, cfg.DataPath)
	a := newAPI(store, cfg, nil, nil)
	inbox := messaging.NewInboxStore(db)
	consumer := messaging.NewConsumer(manager,
		messaging.WithPrefetch(1),
		messaging.WithMaxConcurrency(1),
		messaging.WithAutoAck(false),
		messaging.WithRetryPolicy(messaging.ExchangeRetry, messaging.GenerationCanaryRetryKey, messaging.DefaultConsumerMaxRetries),
		messaging.WithOnMessage(func(messageCtx context.Context, envelope *messaging.Envelope) error {
			return a.processGenerationCanaryMessage(messageCtx, inbox, envelope)
		}),
	)
	if err := consumer.Start(ctx, messaging.GenerationCanaryQueue); err != nil {
		return err
	}
	<-ctx.Done()
	consumer.Stop()
	return ctx.Err()
}

func (a api) processGenerationCanaryMessage(ctx context.Context, inbox *messaging.InboxStore, envelope *messaging.Envelope) error {
	if envelope == nil || envelope.EventType != messaging.GenerationCanaryRoutingKey || envelope.AggregateType != "generation_task" || envelope.AggregateID == "" {
		return messaging.Permanent(fmt.Errorf("invalid generation canary envelope"))
	}
	taskID := envelope.AggregateID
	if value, ok := envelope.Data["task_id"].(string); ok && strings.TrimSpace(value) != taskID {
		return messaging.Permanent(fmt.Errorf("generation canary task mismatch"))
	}
	shortCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := a.pgDB().BeginTx(shortCtx, nil)
	if err != nil {
		return err
	}
	duplicate, err := inbox.ClaimTx(shortCtx, tx, generationImageCanaryConsumer, envelope.EventID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if duplicate {
		_ = tx.Rollback()
		return nil
	}
	task, err := generationTaskForUpdate(shortCtx, tx, taskID)
	if err != nil {
		_ = tx.Rollback()
		if err == sql.ErrNoRows {
			return messaging.Permanent(fmt.Errorf("generation task %s not found", taskID))
		}
		return err
	}
	if !isImageGenerationRequest(task.Type) || !canaryTaskMarker(task.Params) {
		_ = tx.Rollback()
		return messaging.Permanent(fmt.Errorf("generation task %s is not an image canary", taskID))
	}
	if !isRunningGenerationTaskStatus(task.Status) {
		if err := inbox.CompleteTx(shortCtx, tx, generationImageCanaryConsumer, envelope.EventID, "completed", map[string]any{"task_id": taskID, "terminal": true}); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	req := generation.CreateRequest{UserID: task.UserID, Type: task.Type, Prompt: task.Prompt, Model: task.Model, Params: cloneAnyMap(task.Params), ModuleCode: stringValue(task.Params["moduleCode"])}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	// The internal execution identity is deliberately not persisted in the
	// user-facing generation task params. Rebind it when reconstructing a
	// canary request after a process restart.
	req.Params[providerExecutionTaskParam] = taskID
	service, err := a.retryGenerationService(adminUser{ID: task.UserID}, req)
	if err != nil {
		return err
	}
	recovery := false
	if latest, latestErr := pe.NewStore(a.pgDB()).GetLatestByTask(context.Background(), taskID); latestErr == nil {
		recovery = latest.Status == pe.Succeeded || latest.Status == pe.Unknown || latest.Status == pe.Submitted || latest.Status == pe.Processing
	}
	if execErr := checkProviderExecutionState(a.pgDB(), taskID); execErr != nil {
		return execErr
	}
	// Keep the local orchestration path running for a durable execution. The
	// provider hook performs Get-only recovery (or fails closed) without a
	// second Create/Generate call; returning here would acknowledge a
	// succeeded provider row while the generation task is still pending.
	if err := a.runGenerationTask(taskID, service, req); err != nil {
		// A definitive failure settles/releases the task. Ack it so broker
		// redelivery cannot create another pre-submit provider attempt against a
		// terminal task. Ambiguous/deferred states remain active and retryable.
		terminal, checkErr := a.completeCanaryInboxIfTerminal(inbox, envelope.EventID, taskID)
		if checkErr != nil {
			return checkErr
		}
		if terminal {
			generationCanaryMetrics.failed.Add(1)
			return nil
		}
		return err
	}

	finishCtx, finishCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finishCancel()
	finishTx, err := a.pgDB().BeginTx(finishCtx, nil)
	if err != nil {
		return err
	}
	if err := inbox.CompleteTx(finishCtx, finishTx, generationImageCanaryConsumer, envelope.EventID, "completed", map[string]any{"task_id": taskID}); err != nil {
		_ = finishTx.Rollback()
		return err
	}
	if err := finishTx.Commit(); err != nil {
		return err
	}
	generationCanaryMetrics.completed.Add(1)
	if recovery {
		generationCanaryMetrics.recovered.Add(1)
	}
	return nil
}

func (a api) completeCanaryInboxIfTerminal(inbox *messaging.InboxStore, eventID, taskID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := a.pgDB().BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	task, err := generationTaskForUpdate(ctx, tx, taskID)
	if err != nil {
		return false, err
	}
	if isRunningGenerationTaskStatus(task.Status) {
		return false, nil
	}
	if err := inbox.CompleteTx(ctx, tx, generationImageCanaryConsumer, eventID, "completed", map[string]any{"task_id": taskID, "terminal": true}); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (a api) pgDB() *sql.DB {
	if store, ok := a.store.(*postgresStore); ok {
		return store.db
	}
	return nil
}

func canaryTaskMarker(params map[string]any) bool {
	value, ok := params["generation_async_canary"]
	marked, okBool := value.(bool)
	return ok && okBool && marked
}

func checkProviderExecutionState(db *sql.DB, taskID string) error {
	store := pe.NewStore(db)
	latest, err := store.GetLatestByTask(context.Background(), taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	switch latest.Status {
	case pe.Succeeded, pe.Unknown, pe.Submitted, pe.Processing:
		// Let runGenerationTask invoke the guarded hook so a persisted provider
		// request can be queried and local completion can be retried. A durable
		// Succeeded row without local completion must not be silently acked.
		return nil
	case pe.Submitting:
		if latest.ProviderRequestID != nil {
			return nil
		}
		_ = store.MarkUnknown(context.Background(), latest.ID, pe.ProviderUnknown, "submission outcome unknown after crash before transition")
		return pe.ErrUnknownResubmitBlocked
	case pe.Failed:
		if latest.ErrorClass == nil || (*latest.ErrorClass != string(pe.DefinitiveNotSubmitted) && *latest.ErrorClass != string(pe.RetryableBeforeSubmit)) {
			return pe.ErrUnknownResubmitBlocked
		}
	}
	return nil
}
