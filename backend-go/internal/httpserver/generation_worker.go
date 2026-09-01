package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/messaging"
)

const generationImageCanaryConsumer = "generation-image-canary-worker"

// RunGenerationImageCanaryWorker consumes only the opt-in image canary queue.
// Provider calls remain behind the same API ProviderExecution hook and local
// completion is performed by runGenerationTask.
func RunGenerationImageCanaryWorker(ctx context.Context, cfg config.Config, db *sql.DB, manager *messaging.ConnectionManager) error {
	if db == nil || manager == nil {
		return fmt.Errorf("generation worker dependencies are required")
	}
	if !cfg.ProviderExecutionSafetyEnabled {
		return fmt.Errorf("PROVIDER_EXECUTION_SAFETY_ENABLED must be true")
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
	if err := tx.Commit(); err != nil {
		return err
	}

	req := generation.CreateRequest{UserID: task.UserID, Type: task.Type, Prompt: task.Prompt, Model: task.Model, Params: cloneAnyMap(task.Params), ModuleCode: stringValue(task.Params["moduleCode"])}
	service, err := a.retryGenerationService(adminUser{ID: task.UserID}, req)
	if err != nil {
		return err
	}
	a.runGenerationTask(taskID, service, req)

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
	return finishTx.Commit()
}

func (a api) pgDB() *sql.DB { return a.store.(*postgresStore).db }

func canaryTaskMarker(params map[string]any) bool {
	value, ok := params["generation_async_canary"]
	marked, okBool := value.(bool)
	return ok && okBool && marked
}
