package providerexecution

import (
	"context"
	"database/sql"
	"fmt"
)

type generationOwnershipKey struct{}
type GenerationOwnership struct {
	TaskID, WorkerID string
	Generation       int64
}

// WithGenerationOwnership carries the caller's claim, never a freshly observed
// database generation. The value is internal context, not user request params.
func WithGenerationOwnership(ctx context.Context, taskID, workerID string, generation int64) context.Context {
	return context.WithValue(ctx, generationOwnershipKey{}, GenerationOwnership{taskID, workerID, generation})
}

type ownershipQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func verifyGenerationOwnership(ctx context.Context, q ownershipQuerier, taskID string) error {
	owner, ok := ctx.Value(generationOwnershipKey{}).(GenerationOwnership)
	if !ok {
		return nil
	} // legacy unclaimed library callers; workers always carry a claim
	var status, taskStatus, worker string
	var generation int64
	var live bool
	err := q.QueryRowContext(ctx, `SELECT status,coalesce(task_status,''),execution_generation,coalesce(worker_id,''),coalesce(lease_until>now(),false) FROM xz_generation_tasks WHERE id=$1`, taskID).Scan(&status, &taskStatus, &generation, &worker, &live)
	if err != nil {
		return err
	}
	if owner.TaskID != taskID || owner.Generation <= 0 || owner.WorkerID == "" || generation != owner.Generation || worker != owner.WorkerID || !live || taskStatus != "RUNNING" || (status != "PROCESSING" && status != "RUNNING") {
		return fmt.Errorf("%w: task %s provider ownership lost (expected generation=%d owner=%s, current generation=%d owner=%s state=%s/%s lease_valid=%t)", ErrFencedStaleExecution, taskID, owner.Generation, owner.WorkerID, generation, worker, status, taskStatus, live)
	}
	return nil
}

// ValidateGenerationOwnership is also called immediately before the network
// submission. Transactional prepared/claim barriers use the same predicate.
func (s *Store) ValidateGenerationOwnership(ctx context.Context, taskID string) error {
	return verifyGenerationOwnership(ctx, s.DB, taskID)
}
func (s *Store) ValidateGenerationOwnershipTx(ctx context.Context, tx *sql.Tx, taskID string) error {
	return verifyGenerationOwnership(ctx, tx, taskID)
}

// ValidateGenerationExecution must not query or adopt an old operation after a
// new claim. NULL bindings retain the explicit pre-fencing compatibility.
func (s *Store) ValidateGenerationExecution(ctx context.Context, taskID string, e Execution) error {
	if e.TaskGeneration == nil {
		return s.ValidateGenerationOwnership(ctx, taskID)
	}
	var generation int64
	if err := s.DB.QueryRowContext(ctx, `SELECT execution_generation FROM xz_generation_tasks WHERE id=$1`, taskID).Scan(&generation); err != nil {
		return err
	}
	if generation != *e.TaskGeneration {
		return fmt.Errorf("%w: task %s provider execution generation=%d current=%d", ErrFencedStaleExecution, taskID, *e.TaskGeneration, generation)
	}
	return s.ValidateGenerationOwnership(ctx, taskID)
}
