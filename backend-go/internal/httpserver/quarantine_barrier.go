package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

// rejectQuarantinedGeneration requires the PostgreSQL barrier. Only the
// explicitly constructed JSON adapter below has non-PostgreSQL behavior.
func rejectQuarantinedGeneration(ctx context.Context, db *sql.DB, taskID, operation string) error {
	if db == nil {
		return pe.ErrQuarantineBarrierUnavailable
	}
	return pe.RejectTask(ctx, db, strings.TrimSpace(taskID), operation)
}

func rejectQuarantinedTaskTx(ctx context.Context, tx *sql.Tx, taskID, operation string) error {
	taskID = strings.TrimSpace(taskID)
	if tx == nil || taskID == "" {
		return pe.ErrQuarantineBarrierUnavailable
	}
	var dummy string
	err := tx.QueryRowContext(ctx, `SELECT id FROM xz_generation_tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&dummy)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return pe.RejectTask(ctx, tx, taskID, operation)
}

// rejectQuarantinedGeneration permits JSON-only operation by concrete store
// selection, never by a missing PostgreSQL dependency or environment flag.
func (a api) rejectQuarantinedGeneration(ctx context.Context, taskID, operation string) error {
	if store, ok := a.store.(*jsonStore); ok && store != nil {
		return nil
	}
	return rejectQuarantinedGeneration(ctx, a.pgDB(), taskID, operation)
}
