package providerexecution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"
)

// deliberately separate from provider execution state: losing a lease never
// deliberately separate from provider execution state: losing a lease never
// implies that a provider request is safe to recreate.
type TaskLease struct {
	TaskID       string
	WorkerID     string
	LeaseUntil   time.Time
	TaskStatus   string
	AttemptCount int
}

var ErrLeaseNotOwned = errors.New("generation task lease is not owned")
var ErrNoLeaseAvailable = sql.ErrNoRows

// ClaimTask atomically claims one queued (or expired) task. SKIP LOCKED makes
// this safe for multiple worker instances and keeps the transaction short.
func (s *Store) ClaimTask(ctx context.Context, workerID string, lease time.Duration) (TaskLease, error) {
	if s == nil || s.DB == nil || workerID == "" || lease <= 0 {
		return TaskLease{}, fmt.Errorf("worker id, database and positive lease are required")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return TaskLease{}, err
	}
	defer tx.Rollback()
	var out TaskLease
	err = tx.QueryRowContext(ctx, `
		SELECT id, attempt_count FROM xz_generation_tasks
		WHERE upper(coalesce(nullif(task_status,''),status)) IN ('QUEUED','PENDING','RUNNING','PROCESSING')
		  AND (lease_until IS NULL OR lease_until <= now())
		ORDER BY created_at, id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&out.TaskID, &out.AttemptCount)
	if err != nil {
		return TaskLease{}, err
	}
	if err = tx.QueryRowContext(ctx, `UPDATE xz_generation_tasks
		SET worker_id=$1, lease_until=now()+$2::interval, last_heartbeat_at=now(),
			task_status=CASE WHEN upper(coalesce(nullif(task_status,''),status)) IN ('QUEUED','PENDING') THEN 'RUNNING' ELSE task_status END,
			started_at=coalesce(started_at,now()), updated_at=now()
		WHERE id=$3 RETURNING lease_until,task_status`, workerID, lease.String(), out.TaskID).Scan(&out.LeaseUntil, &out.TaskStatus); err != nil {
		return TaskLease{}, err
	}
	out.WorkerID = workerID
	return out, tx.Commit()
}

// AcquireTask atomically takes ownership of a specific delivery. It is used by
// the real image/video consumers so re-delivery cannot execute concurrently.
func (s *Store) AcquireTask(ctx context.Context, taskID, workerID string, lease time.Duration) (TaskLease, error) {
	if s == nil || s.DB == nil || taskID == "" || workerID == "" || lease <= 0 {
		return TaskLease{}, fmt.Errorf("task id, worker id, database and positive lease are required")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return TaskLease{}, err
	}
	defer tx.Rollback()
	var out TaskLease
	err = tx.QueryRowContext(ctx, `UPDATE xz_generation_tasks SET worker_id=$1,lease_until=now()+$2::interval,last_heartbeat_at=now(),task_status=CASE WHEN upper(coalesce(nullif(task_status,''),status)) IN ('QUEUED','PENDING') THEN 'RUNNING' ELSE task_status END,started_at=coalesce(started_at,now()),updated_at=now() WHERE id=$3 AND upper(coalesce(nullif(task_status,''),status)) IN ('QUEUED','PENDING','RUNNING','PROCESSING') AND (lease_until IS NULL OR lease_until<=now()) RETURNING id,coalesce(attempt_count,0),lease_until,task_status`, workerID, lease.String(), taskID).Scan(&out.TaskID, &out.AttemptCount, &out.LeaseUntil, &out.TaskStatus)
	if err != nil {
		return TaskLease{}, err
	}
	out.WorkerID = workerID
	return out, tx.Commit()
}

// RenewTaskLease only extends the caller's lease. A stale worker can never
// resurrect ownership after another worker has reclaimed the row.
func (s *Store) RenewTaskLease(ctx context.Context, taskID, workerID string, lease time.Duration) error {
	if lease <= 0 {
		return fmt.Errorf("positive lease is required")
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE xz_generation_tasks SET lease_until=now()+$1::interval,last_heartbeat_at=now(),updated_at=now() WHERE id=$2 AND worker_id=$3 AND lease_until>now()`, lease.String(), taskID, workerID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrLeaseNotOwned
	}
	return nil
}

// StartTaskHeartbeat renews a lease until ctx is cancelled. A lost lease
// stops renewal rather than extending another worker's ownership.
func (s *Store) StartTaskHeartbeat(ctx context.Context, taskID, workerID string, interval, lease time.Duration) {
	if interval <= 0 || lease <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.RenewTaskLease(ctx, taskID, workerID, lease); err != nil {
					return
				}
			}
		}
	}()
}

// ReleaseTaskLease clears ownership without changing task status. Completion
// and failure paths remain responsible for their existing state/billing guards.
func (s *Store) ReleaseTaskLease(ctx context.Context, taskID, workerID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT task_status FROM xz_generation_tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&status); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE xz_generation_tasks SET worker_id=NULL,lease_until=NULL,updated_at=now() WHERE id=$1 AND worker_id=$2`, taskID, workerID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrLeaseNotOwned
	}
	return tx.Commit()
}

// ReapExpired atomically requeues expired tasks only when the original durable
// generation event exists. Tasks without a replayable event, exhausted retries,
// or provider executions are never made QUEUED: they become MANUAL_REVIEW.
func (s *Store) ReapExpired(ctx context.Context, limit int, maxAttempts int) ([]TaskLease, []string, error) {
	if limit <= 0 {
		limit = 100
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT t.id, EXISTS (SELECT 1 FROM provider_executions pe WHERE pe.task_id=t.id), coalesce(t.attempt_count,0), EXISTS (SELECT 1 FROM outbox_events oe WHERE oe.aggregate_type='generation_task' AND oe.aggregate_id=t.id AND oe.event_type IN ('x.ai.generation.image.canary.requested','x.ai.generation.video.canary.requested','x.ai.generation.ppt.canary.requested'))
		FROM xz_generation_tasks t
		WHERE upper(coalesce(nullif(t.task_status,''),t.status)) IN ('QUEUED','RUNNING','PROCESSING') AND t.lease_until IS NOT NULL AND t.lease_until<=now()
		ORDER BY t.lease_until FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var requeued []TaskLease
	var recoveries []string
	for rows.Next() {
		var id string
		var hasExec, hasEvent bool
		var attempts int
		if err = rows.Scan(&id, &hasExec, &attempts, &hasEvent); err != nil {
			return nil, nil, err
		}
		if hasExec {
			if _, err = tx.ExecContext(ctx, `UPDATE xz_generation_tasks SET task_status='MANUAL_REVIEW',worker_id=NULL,lease_until=NULL,updated_at=now() WHERE id=$1`, id); err != nil {
				return nil, nil, err
			}
			recoveries = append(recoveries, id)
			log.Printf("generation_reaper task=%s action=manual_review reason=provider_execution_exists", id)
			continue
		}
		if attempts >= maxAttempts || !hasEvent {
			if _, err = tx.ExecContext(ctx, `UPDATE xz_generation_tasks SET task_status='MANUAL_REVIEW',worker_id=NULL,lease_until=NULL,updated_at=now() WHERE id=$1`, id); err != nil {
				return nil, nil, err
			}
			log.Printf("generation_reaper task=%s action=manual_review attempts=%d has_outbox=%t", id, attempts, hasEvent)
			continue
		}
		result, updateErr := tx.ExecContext(ctx, `UPDATE outbox_events SET status='pending',next_attempt_at=now(),claimed_at=NULL,claim_owner=NULL,last_error=NULL,updated_at=now() WHERE aggregate_type='generation_task' AND aggregate_id=$1 AND event_type IN ('x.ai.generation.image.canary.requested','x.ai.generation.video.canary.requested','x.ai.generation.ppt.canary.requested')`, id)
		if updateErr != nil {
			return nil, nil, updateErr
		}
		if n, _ := result.RowsAffected(); n != 1 {
			return nil, nil, fmt.Errorf("task %s has no unique generation outbox event", id)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE xz_generation_tasks SET task_status='QUEUED',worker_id=NULL,lease_until=NULL,attempt_count=coalesce(attempt_count,0)+1,updated_at=now() WHERE id=$1`, id); err != nil {
			return nil, nil, err
		}
		requeued = append(requeued, TaskLease{TaskID: id, TaskStatus: "QUEUED", AttemptCount: attempts + 1})
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}
	return requeued, recoveries, tx.Commit()
}
