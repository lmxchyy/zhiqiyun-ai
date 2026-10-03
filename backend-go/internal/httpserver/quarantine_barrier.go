package httpserver

import (
	"context"
	"database/sql"
	"strings"

	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

// rejectQuarantinedGeneration is the shared runtime barrier. A nil database
// means this process has no PostgreSQL quarantine relation; callers that are
// about to mutate through PostgreSQL must not pass nil. JSON-store tests pass
// nil and keep their existing non-PostgreSQL behavior.
func rejectQuarantinedGeneration(ctx context.Context, db *sql.DB, taskID, operation string) error {
	if db == nil {
		return nil
	}
	return pe.RejectTask(ctx, db, strings.TrimSpace(taskID), operation)
}

func rejectQuarantinedTaskTx(ctx context.Context, tx *sql.Tx, taskID, operation string) error {
	if tx == nil || strings.TrimSpace(taskID) == "" {
		return nil
	}
	return pe.RejectTask(ctx, tx, strings.TrimSpace(taskID), operation)
}

func rejectQuarantinedPointMutation(ctx context.Context, tx *sql.Tx, idempotencyKey, operation string) error {
	taskID := generationTaskIDFromPointKey(idempotencyKey)
	if taskID == "" || tx == nil {
		return nil
	}
	return pe.RejectTask(ctx, tx, taskID, "billing_"+operation)
}

func generationTaskIDFromPointKey(key string) string {
	for _, prefix := range []string{"generation:capture:", "generation:release:", "generation:reserve:", "generation:durable-release:"} {
		if strings.HasPrefix(key, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(key, prefix))
		}
	}
	return ""
}
