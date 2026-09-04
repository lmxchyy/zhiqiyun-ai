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

const generationVideoCanaryConsumer = "generation-video-canary-worker"

// RunGenerationVideoCanaryWorker consumes only the opt-in video canary queue.
// Provider calls remain behind the same API ProviderExecution hook and local
// completion is performed by runVideoGenerationTask.
func RunGenerationVideoCanaryWorker(ctx context.Context, cfg config.Config, db *sql.DB, manager *messaging.ConnectionManager) error {
	if db == nil || manager == nil {
		return fmt.Errorf("generation video worker dependencies are required")
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
		messaging.WithRetryPolicy(messaging.ExchangeRetry, messaging.GenerationVideoCanaryRetryKey, 30),
		messaging.WithOnMessage(func(messageCtx context.Context, envelope *messaging.Envelope) error {
			return a.processGenerationVideoCanaryMessage(messageCtx, inbox, envelope)
		}),
	)
	if err := consumer.Start(ctx, messaging.GenerationVideoCanaryQueue); err != nil {
		return err
	}
	<-ctx.Done()
	consumer.Stop()
	return ctx.Err()
}

func (a api) processGenerationVideoCanaryMessage(ctx context.Context, inbox *messaging.InboxStore, envelope *messaging.Envelope) error {
	if envelope == nil || envelope.EventType != messaging.GenerationVideoCanaryRoutingKey || envelope.AggregateType != "generation_task" || envelope.AggregateID == "" {
		return messaging.Permanent(fmt.Errorf("invalid generation video canary envelope"))
	}
	taskID := envelope.AggregateID
	if value, ok := envelope.Data["task_id"].(string); ok && strings.TrimSpace(value) != taskID {
		return messaging.Permanent(fmt.Errorf("generation video canary task mismatch"))
	}
	shortCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := a.pgDB().BeginTx(shortCtx, nil)
	if err != nil {
		return err
	}
	duplicate, err := inbox.ClaimTx(shortCtx, tx, generationVideoCanaryConsumer, envelope.EventID)
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
	if !isVideoGenerationRequest(task.Type) || !canaryTaskMarker(task.Params) {
		_ = tx.Rollback()
		return messaging.Permanent(fmt.Errorf("generation task %s is not a video canary", taskID))
	}
	if !isRunningGenerationTaskStatus(task.Status) {
		if err := inbox.CompleteTx(shortCtx, tx, generationVideoCanaryConsumer, envelope.EventID, "completed", map[string]any{"task_id": taskID, "terminal": true}); err != nil {
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
	req.Params[providerExecutionTaskParam] = taskID
	service, err := a.retryGenerationService(adminUser{ID: task.UserID}, req)
	if err != nil {
		return err
	}
	if execErr := checkProviderExecutionState(a.pgDB(), taskID); execErr != nil {
		return execErr
	}
	if err := a.runVideoGenerationTask(taskID, service, req); err != nil {
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
	if err := inbox.CompleteTx(finishCtx, finishTx, generationVideoCanaryConsumer, envelope.EventID, "completed", map[string]any{"task_id": taskID}); err != nil {
		_ = finishTx.Rollback()
		return err
	}
	if err := finishTx.Commit(); err != nil {
		return err
	}
	generationCanaryMetrics.completed.Add(1)
	return nil
}
