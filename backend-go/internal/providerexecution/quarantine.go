package providerexecution

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"reflect"
	"strings"
	"time"
)

// ErrQuarantined rejects a mutating operation. Callers must not invoke a
// provider or write execution, task, asset, or billing state after this error.
var ErrQuarantined = errors.New("EXECUTION_QUARANTINED_READONLY")

// ErrQuarantineBarrierUnavailable means the barrier could not prove that the
// operation is safe. It is also fail-closed and must not be treated as "not
// quarantined". PostgreSQL operations require the quarantine relation even
// when there are no active approvals.
var ErrQuarantineBarrierUnavailable = errors.New("quarantine barrier unavailable")

// QuarantineRecord is the durable identity bound to one execution. It never
// contains prompts, payloads, credentials, or media URLs.
type QuarantineRecord struct {
	ExecutionID    int64
	TaskID         string
	Attempt        int
	Generation     *int64
	SnapshotSHA256 string
	EvidenceSHA256 string
	ApprovalID     string
	ReleaseSHA     string
	NotBefore      time.Time
	ExpiresAt      time.Time
}

// QuarantineQuery is the identity observed inside the caller's write fence.
type QuarantineQuery struct {
	ExecutionID    int64
	TaskID         string
	Attempt        int
	Generation     *int64
	ReleaseSHA     string
	EvidenceSHA256 string
	Now            time.Time
}

type quarantineQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// DecideQuarantine is the pure fail-closed policy. Any related row blocks,
// including expired, mismatched, or duplicate rows. No row allows the normal
// path. A failed read never becomes "not quarantined".
func DecideQuarantine(query QuarantineQuery, records []QuarantineRecord, queryFailed bool) (bool, string) {
	if queryFailed || strings.TrimSpace(query.TaskID) == "" && query.ExecutionID <= 0 {
		return true, "query_failed"
	}
	if len(records) == 0 {
		return false, "none"
	}
	if len(records) != 1 {
		return true, "duplicate"
	}
	record := records[0]
	if record.ExecutionID <= 0 || strings.TrimSpace(record.TaskID) == "" || record.Attempt < 1 || record.ApprovalID == "" {
		return true, "malformed"
	}
	if query.ExecutionID > 0 && record.ExecutionID != query.ExecutionID {
		return true, "identity_mismatch"
	}
	if query.TaskID != "" && record.TaskID != query.TaskID {
		return true, "identity_mismatch"
	}
	if query.Attempt > 0 && record.Attempt != query.Attempt {
		return true, "identity_mismatch"
	}
	if query.Generation != nil && (record.Generation == nil || *record.Generation != *query.Generation) {
		return true, "generation_mismatch"
	}
	if query.ReleaseSHA != "" && record.ReleaseSHA != query.ReleaseSHA {
		return true, "release_mismatch"
	}
	if query.EvidenceSHA256 != "" && record.EvidenceSHA256 != query.EvidenceSHA256 {
		return true, "evidence_mismatch"
	}
	now := query.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if !record.ExpiresAt.After(now) || now.Before(record.NotBefore) {
		return true, "expired"
	}
	return true, "quarantined"
}

// RejectTask blocks when a quarantine row exists for the task or its lookup
// fails, including an absent required relation.
func RejectTask(ctx context.Context, q quarantineQuerier, taskID, operation string) error {
	return reject(ctx, q, 0, taskID, operation)
}

// RejectExecution blocks inside a write transaction after the execution row
// lock. It re-reads quarantine rather than trusting an earlier check.
func RejectExecution(ctx context.Context, q quarantineQuerier, executionID int64, taskID, operation string) error {
	return reject(ctx, q, executionID, taskID, operation)
}

func reject(ctx context.Context, q quarantineQuerier, executionID int64, taskID, operation string) error {
	if q == nil || (reflect.ValueOf(q).Kind() == reflect.Ptr && reflect.ValueOf(q).IsNil()) {
		return logQuarantine(operation, executionID, taskID, QuarantineRecord{}, "query_failed", ErrQuarantineBarrierUnavailable)
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" && executionID <= 0 {
		return logQuarantine(operation, executionID, taskID, QuarantineRecord{}, "query_failed", ErrQuarantineBarrierUnavailable)
	}
	if executionID > 0 && taskID == "" {
		if err := q.QueryRowContext(ctx, `SELECT task_id FROM provider_executions WHERE id=$1`, executionID).Scan(&taskID); err != nil {
			return logQuarantine(operation, executionID, taskID, QuarantineRecord{}, "query_failed", ErrQuarantineBarrierUnavailable)
		}
		taskID = strings.TrimSpace(taskID)
	}
	var installed sql.NullBool
	if err := q.QueryRowContext(ctx, `SELECT to_regclass('public.provider_execution_quarantine') IS NOT NULL`).Scan(&installed); err != nil {
		return logQuarantine(operation, executionID, taskID, QuarantineRecord{}, "query_failed", ErrQuarantineBarrierUnavailable)
	}
	if !installed.Valid || !installed.Bool {
		return logQuarantine(operation, executionID, taskID, QuarantineRecord{}, "relation_missing", ErrQuarantineBarrierUnavailable)
	}
	rows, err := q.QueryContext(ctx, `
		SELECT execution_id, task_id, attempt, generation, snapshot_sha256, evidence_sha256,
		       approval_id, release_sha, not_before, expires_at
		FROM provider_execution_quarantine
		WHERE ($1 > 0 AND execution_id = $1) OR ($2 <> '' AND task_id = $2)
		FOR SHARE`, executionID, taskID)
	if err != nil {
		return logQuarantine(operation, executionID, taskID, QuarantineRecord{}, "query_failed", ErrQuarantineBarrierUnavailable)
	}
	defer rows.Close()
	var records []QuarantineRecord
	for rows.Next() {
		var record QuarantineRecord
		var generation sql.NullInt64
		if err := rows.Scan(&record.ExecutionID, &record.TaskID, &record.Attempt, &generation, &record.SnapshotSHA256, &record.EvidenceSHA256, &record.ApprovalID, &record.ReleaseSHA, &record.NotBefore, &record.ExpiresAt); err != nil {
			return logQuarantine(operation, executionID, taskID, QuarantineRecord{}, "query_failed", ErrQuarantineBarrierUnavailable)
		}
		if generation.Valid {
			value := generation.Int64
			record.Generation = &value
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return logQuarantine(operation, executionID, taskID, QuarantineRecord{}, "query_failed", ErrQuarantineBarrierUnavailable)
	}
	query := QuarantineQuery{ExecutionID: executionID, TaskID: taskID, Now: time.Now().UTC()}
	if executionID > 0 && taskID != "" {
		var attempt int
		var generation sql.NullInt64
		err := q.QueryRowContext(ctx, `SELECT attempt, task_execution_generation FROM provider_executions WHERE id=$1`, executionID).Scan(&attempt, &generation)
		if err != nil {
			return logQuarantine(operation, executionID, taskID, QuarantineRecord{}, "query_failed", ErrQuarantineBarrierUnavailable)
		}
		query.Attempt = attempt
		if generation.Valid {
			value := generation.Int64
			query.Generation = &value
		}
	}
	block, reason := DecideQuarantine(query, records, false)
	if !block {
		return nil
	}
	var record QuarantineRecord
	if len(records) == 1 {
		record = records[0]
	}
	return logQuarantine(operation, executionID, taskID, record, reason, ErrQuarantined)
}

func logQuarantine(operation string, executionID int64, taskID string, record QuarantineRecord, reason string, err error) error {
	log.Printf("quarantine_barrier decision=block reason=%s operation=%s execution_id=%d task_id=%s approval_id=%s evidence_sha256=%s release_sha=%s",
		reason, operation, executionID, taskID, record.ApprovalID, record.EvidenceSHA256, record.ReleaseSHA)
	return err
}
