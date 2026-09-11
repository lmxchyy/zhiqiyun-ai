package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/messaging"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

// RunGenerationDLQWorker drains one dedicated image, video, or PPT generation
// DLQ. It intentionally has no retry policy: a recovery failure is rejected
// for operator inspection rather than sent through the business retry loop.
func RunGenerationDLQWorker(ctx context.Context, cfg config.Config, db *sql.DB, manager *messaging.ConnectionManager, queue string, expired bool) error {
	if db == nil || manager == nil || !generationDLQQueue(queue) {
		return fmt.Errorf("generation dlq worker dependencies or queue are invalid")
	}
	inbox := messaging.NewInboxStore(db)
	consumerName := "generation-dlq-" + strings.ReplaceAll(queue, ".", "-")
	consumer := messaging.NewConsumer(manager, messaging.WithPrefetch(1), messaging.WithMaxConcurrency(1), messaging.WithAutoAck(false), messaging.WithOnMessage(func(messageCtx context.Context, envelope *messaging.Envelope) error {
		return ProcessGenerationDLQMessage(messageCtx, db, inbox, envelope, consumerName, expired)
	}))
	if err := consumer.Start(ctx, queue); err != nil {
		return err
	}
	<-ctx.Done()
	consumer.Stop()
	return ctx.Err()
}

func generationDLQQueue(queue string) bool {
	switch queue {
	case messaging.GenerationCanaryDLQ, messaging.GenerationVideoCanaryDLQ, messaging.GenerationPPTCanaryDLQ:
		return true
	default:
		return false
	}
}

// ProcessGenerationDLQMessage converges one of the dedicated generation DLQs
// without invoking a provider or billing operation. The event identity is the
// inbox idempotency key; replaying a DLQ delivery therefore cannot create a
// task or charge points twice.
func ProcessGenerationDLQMessage(ctx context.Context, db *sql.DB, inbox *messaging.InboxStore, envelope *messaging.Envelope, consumerName string, expired bool) error {
	if db == nil || inbox == nil || envelope == nil || strings.TrimSpace(consumerName) == "" {
		return fmt.Errorf("generation dlq recovery dependencies are required")
	}
	if envelope.AggregateType != "generation_task" || envelope.AggregateID == "" || !isGenerationDLQEvent(envelope.EventType) {
		return messaging.Permanent(fmt.Errorf("invalid generation dlq envelope"))
	}
	if v, ok := envelope.Data["task_id"].(string); ok && strings.TrimSpace(v) != envelope.AggregateID {
		return messaging.Permanent(fmt.Errorf("generation dlq task mismatch"))
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	duplicate, err := inbox.ClaimTx(ctx, tx, consumerName, envelope.EventID)
	if err != nil {
		return err
	}
	if duplicate {
		return tx.Rollback()
	}
	task, err := generationTaskForUpdate(ctx, tx, envelope.AggregateID)
	if err != nil {
		if err == sql.ErrNoRows {
			return messaging.Permanent(fmt.Errorf("generation task %s not found", envelope.AggregateID))
		}
		return err
	}
	if generationTaskStateTerminal(canonicalGenerationTaskState(task)) {
		if err := inbox.CompleteTx(ctx, tx, consumerName, envelope.EventID, "completed", map[string]any{"task_id": envelope.AggregateID, "terminal": true}); err != nil {
			return err
		}
		return tx.Commit()
	}

	action := pe.DLQAction(pe.DecideDLQAction(pe.Prepared, expired))
	latest, latestErr := pe.NewStore(db).GetLatestByTask(ctx, envelope.AggregateID)
	if latestErr == nil {
		action = pe.DecideDLQAction(latest.Status, expired)
	} else if latestErr != sql.ErrNoRows {
		return latestErr
	}
	// Only the three generation terminal projections are written here. In
	// particular, MANUAL_REVIEW never clears billing reservations or releases a
	// lease for a provider execution that may still be active.
	if _, err := tx.ExecContext(ctx, `UPDATE xz_generation_tasks SET task_status=$1,status=$1,worker_id=CASE WHEN $1='MANUAL_REVIEW' THEN worker_id ELSE NULL END,lease_until=CASE WHEN $1='MANUAL_REVIEW' THEN lease_until ELSE NULL END,updated_at=now() WHERE id=$2 AND upper(coalesce(nullif(task_status,''),status)) NOT IN ('SUCCEEDED','FAILED','EXPIRED','CANCELLED','MANUAL_REVIEW')`, string(action), envelope.AggregateID); err != nil {
		return err
	}
	if err := inbox.CompleteTx(ctx, tx, consumerName, envelope.EventID, "converged", map[string]any{"task_id": envelope.AggregateID, "action": string(action), "provider_execution": latestErr == nil}); err != nil {
		return err
	}
	return tx.Commit()
}

func isGenerationDLQEvent(eventType string) bool {
	switch eventType {
	case messaging.GenerationCanaryRoutingKey, messaging.GenerationVideoCanaryRoutingKey, messaging.GenerationPPTCanaryRoutingKey:
		return true
	default:
		return false
	}
}
