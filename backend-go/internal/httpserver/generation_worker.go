package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"errors"
	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/messaging"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

const generationImageCanaryConsumer = "generation-image-canary-worker"
const generationImageNormalConsumer = "generation-image-normal-worker"

// generationCanaryDrainEnabled is intentionally independent of the new-submit
// canary flag. Operators can stop selection while existing durable work drains.
func generationCanaryDrainEnabled(cfg config.Config) bool {
	return cfg.AsyncMessagingEnabled && cfg.ProviderExecutionSafetyEnabled
}

// RunGenerationImageCanaryWorker consumes only the opt-in image canary queue.
// Provider calls remain behind the same API ProviderExecution hook and local
// completion is performed by runGenerationTask.
func RunGenerationImageCanaryWorker(ctx context.Context, cfg config.Config, db *sql.DB, manager *messaging.ConnectionManager) error {
	return runGenerationImageWorker(ctx, cfg, db, manager, imageDispatchCanary)
}
func RunGenerationImageNormalWorker(ctx context.Context, cfg config.Config, db *sql.DB, manager *messaging.ConnectionManager) error {
	return runGenerationImageWorker(ctx, cfg, db, manager, imageDispatchNormal)
}
func runGenerationImageWorker(ctx context.Context, cfg config.Config, db *sql.DB, manager *messaging.ConnectionManager, mode string) error {
	if db == nil || manager == nil {
		return fmt.Errorf("generation worker dependencies are required")
	}
	if !generationCanaryDrainEnabled(cfg) {
		return fmt.Errorf("ASYNC_MESSAGING_ENABLED and PROVIDER_EXECUTION_SAFETY_ENABLED must be true")
	}
	store := newPostgresPrimaryStore(db, cfg.DataPath)
	a := newAPI(store, cfg, nil, nil)
	inbox := messaging.NewInboxStore(db)
	queue, retryKey := messaging.GenerationCanaryQueue, messaging.GenerationCanaryRetryKey
	if mode == imageDispatchNormal {
		queue, retryKey = messaging.GenerationImageNormalQueue, messaging.GenerationImageNormalRetryKey
	}
	consumer := messaging.NewConsumer(manager,
		messaging.WithPrefetch(1),
		messaging.WithMaxConcurrency(1),
		messaging.WithAutoAck(false),
		messaging.WithRetryPolicy(messaging.ExchangeRetry, retryKey, messaging.DefaultConsumerMaxRetries),
		messaging.WithOnMessage(func(messageCtx context.Context, envelope *messaging.Envelope) error {
			return a.processGenerationImageMessage(messageCtx, inbox, envelope, mode)
		}),
	)
	if err := consumer.Start(ctx, queue); err != nil {
		return err
	}
	<-ctx.Done()
	consumer.Stop()
	return ctx.Err()
}

func (a api) processGenerationCanaryMessage(ctx context.Context, inbox *messaging.InboxStore, envelope *messaging.Envelope) error {
	return a.processGenerationImageMessage(ctx, inbox, envelope, imageDispatchCanary)
}
func (a *api) processGenerationNormalMessage(ctx context.Context, inbox *messaging.InboxStore, envelope *messaging.Envelope) error {
	return a.processGenerationImageMessage(ctx, inbox, envelope, imageDispatchNormal)
}
func (a *api) processGenerationImageMessage(ctx context.Context, inbox *messaging.InboxStore, envelope *messaging.Envelope, mode string) error {
	if err := validateImageDispatchEnvelope(envelope, mode); err != nil {
		return messaging.Permanent(err)
	}
	consumerName := generationImageCanaryConsumer
	if mode == imageDispatchNormal {
		consumerName = generationImageNormalConsumer
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
	duplicate, err := inbox.ClaimTx(shortCtx, tx, consumerName, envelope.EventID)
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
	taskMode, modeErr := imageDispatchMode(task.Params)
	if !isImageGenerationRequest(task.Type) || modeErr != nil || taskMode != mode {
		_ = tx.Rollback()
		return messaging.Permanent(fmt.Errorf("generation task %s dispatch protocol mismatch expected=%s actual=%s", taskID, mode, taskMode))
	}
	if !isRunningGenerationTaskStatus(task.Status) {
		if err := inbox.CompleteTx(shortCtx, tx, consumerName, envelope.EventID, "completed", map[string]any{"task_id": taskID, "terminal": true}); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	// A pending inbox is not a lease. Never complete the original unfinished
	// event just because its Worker is live; it remains a recovery trigger.
	if task.TaskStatus == taskStatusRunning && generationLeaseValid(task.LeaseUntil, time.Now().UTC()) && task.WorkerID != "" {
		_ = tx.Rollback()
		fencingCutover.staleRedeliveries.Add(1)
		log.Printf("generation image duplicate deferred task_id=%s mode=%s generation=%d owner=%s", taskID, mode, task.fencingGeneration(), task.WorkerID)
		return errGenerationOwnershipBusy
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
	// Keep the local orchestration path running for a durable execution. The
	// provider hook performs Get-only recovery (or fails closed) without a
	// second Create/Generate call; returning here would acknowledge a
	// succeeded provider row while the generation task is still pending.
	if err := a.runGenerationTaskForDispatch(taskID, service, req, envelopeExecutionGeneration(envelope.Data)); err != nil {
		// A definitive failure settles/releases the task. Ack it so broker
		// redelivery cannot create another pre-submit provider attempt against a
		// terminal task. Ambiguous/deferred states remain active and retryable.
		terminal, checkErr := a.completeImageInboxIfTerminal(inbox, envelope.EventID, taskID, consumerName)
		if checkErr != nil {
			return checkErr
		}
		if terminal {
			if mode == imageDispatchCanary {
				generationCanaryMetrics.failed.Add(1)
			}
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
	if err := inbox.CompleteTx(finishCtx, finishTx, consumerName, envelope.EventID, "completed", map[string]any{"task_id": taskID}); err != nil {
		_ = finishTx.Rollback()
		return err
	}
	if err := finishTx.Commit(); err != nil {
		return err
	}
	if mode == imageDispatchCanary {
		generationCanaryMetrics.completed.Add(1)
	}
	if recovery && mode == imageDispatchCanary {
		generationCanaryMetrics.recovered.Add(1)
	}
	return nil
}

func (a api) completeCanaryInboxIfTerminal(inbox *messaging.InboxStore, eventID, taskID string) (bool, error) {
	return a.completeImageInboxIfTerminal(inbox, eventID, taskID, generationImageCanaryConsumer)
}
func (a *api) completeImageInboxIfTerminal(inbox *messaging.InboxStore, eventID, taskID, consumerName string) (bool, error) {
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
	if err := inbox.CompleteTx(ctx, tx, consumerName, eventID, "completed", map[string]any{"task_id": taskID, "terminal": true}); err != nil {
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

// envelopeExecutionGeneration reads the threaded execution identity from an
// outbox envelope (scheduler dispatch writes it). 0 means absent/legacy.
func envelopeExecutionGeneration(data map[string]any) int64 {
	if data == nil {
		return 0
	}
	switch value := data["execution_generation"].(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	case string:
		var parsed int64
		_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed)
		return parsed
	default:
		return 0
	}
}

// generationLeaseValid reports whether the RFC3339Nano lease timestamp is
// still in the future against the given clock.
func generationLeaseValid(leaseUntil string, now time.Time) bool {
	trimmed := strings.TrimSpace(leaseUntil)
	if trimmed == "" {
		return false
	}
	expiry, err := time.Parse(time.RFC3339Nano, trimmed)
	if err != nil {
		return false
	}
	return expiry.After(now)
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
