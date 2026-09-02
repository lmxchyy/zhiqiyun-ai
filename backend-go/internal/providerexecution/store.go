package providerexecution

import (
	"context"
	"database/sql"
	"fmt"
)

type Store struct{ DB *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{DB: db} }

func (s *Store) CreatePrepared(ctx context.Context, e Execution) (Execution, error) {
	return s.createPrepared(ctx, e, false)
}

// CreatePreparedForGenerationTask establishes the task-row barrier before a
// provider execution claim. Cancellation/stale repair use the same row lock;
// therefore a terminal task can never acquire a new provider execution.
func (s *Store) CreatePreparedForGenerationTask(ctx context.Context, e Execution) (Execution, error) {
	return s.createPrepared(ctx, e, true)
}

func (s *Store) createPrepared(ctx context.Context, e Execution, lockTask bool) (Execution, error) {
	if s == nil || s.DB == nil {
		return Execution{}, fmt.Errorf("provider execution database is required")
	}
	if e.Attempt <= 0 {
		e.Attempt = 1
	}
	e.Status = Prepared
	if e.ProviderOperationKey == "" {
		e.ProviderOperationKey = fmt.Sprintf("generation:%s:%d", e.TaskID, e.Attempt)
	}
	if lockTask {
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return Execution{}, err
		}
		defer tx.Rollback()
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM xz_generation_tasks WHERE id=$1 FOR UPDATE`, e.TaskID).Scan(&status); err != nil {
			return Execution{}, err
		}
		switch status {
		case "PENDING", "PROCESSING", "RUNNING", "QUEUED":
		default:
			return Execution{}, fmt.Errorf("generation task %s is terminal (%s)", e.TaskID, status)
		}
		if err := tx.QueryRowContext(ctx, `INSERT INTO provider_executions (task_id,provider,provider_channel,provider_model,capability,attempt,status,request_fingerprint,provider_operation_key) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id,created_at,updated_at`, e.TaskID, e.Provider, e.ProviderChannel, e.ProviderModel, e.Capability, e.Attempt, e.Status, e.RequestFingerprint, e.ProviderOperationKey).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return Execution{}, err
		}
		return e, tx.Commit()
	}
	row := s.DB.QueryRowContext(ctx, `INSERT INTO provider_executions (task_id,provider,provider_channel,provider_model,capability,attempt,status,request_fingerprint,provider_operation_key) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id,created_at,updated_at`, e.TaskID, e.Provider, e.ProviderChannel, e.ProviderModel, e.Capability, e.Attempt, e.Status, e.RequestFingerprint, e.ProviderOperationKey)
	if err := row.Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return Execution{}, err
	}
	return e, nil
}

func (s *Store) GetByID(ctx context.Context, id int64) (Execution, error) {
	return s.get(ctx, `SELECT id,task_id,provider,provider_channel,provider_model,capability,attempt,status,request_fingerprint,provider_operation_key,provider_request_id,result_metadata,submitted_at,processing_at,succeeded_at,failed_at,unknown_at,last_checked_at,next_check_at,error_code,error_class,last_error,created_at,updated_at FROM provider_executions WHERE id=$1`, id)
}
func (s *Store) GetActiveByTask(ctx context.Context, taskID string) (Execution, error) {
	return s.get(ctx, `SELECT id,task_id,provider,provider_channel,provider_model,capability,attempt,status,request_fingerprint,provider_operation_key,provider_request_id,result_metadata,submitted_at,processing_at,succeeded_at,failed_at,unknown_at,last_checked_at,next_check_at,error_code,error_class,last_error,created_at,updated_at FROM provider_executions WHERE task_id=$1 AND status NOT IN ('succeeded','failed') ORDER BY attempt DESC LIMIT 1`, taskID)
}

func (s *Store) GetLatestByTask(ctx context.Context, taskID string) (Execution, error) {
	return s.get(ctx, `SELECT id,task_id,provider,provider_channel,provider_model,capability,attempt,status,request_fingerprint,provider_operation_key,provider_request_id,result_metadata,submitted_at,processing_at,succeeded_at,failed_at,unknown_at,last_checked_at,next_check_at,error_code,error_class,last_error,created_at,updated_at FROM provider_executions WHERE task_id=$1 ORDER BY attempt DESC LIMIT 1`, taskID)
}
func (s *Store) get(ctx context.Context, q string, arg any) (Execution, error) {
	var e Execution
	// result_metadata is nullable. Scanning SQL NULL directly into
	// *json.RawMessage is rejected by database/sql, so scan into []byte and
	// preserve NULL as a nil RawMessage.
	var resultMetadata []byte
	err := s.DB.QueryRowContext(ctx, q, arg).Scan(&e.ID, &e.TaskID, &e.Provider, &e.ProviderChannel, &e.ProviderModel, &e.Capability, &e.Attempt, &e.Status, &e.RequestFingerprint, &e.ProviderOperationKey, &e.ProviderRequestID, &resultMetadata, &e.SubmittedAt, &e.ProcessingAt, &e.SucceededAt, &e.FailedAt, &e.UnknownAt, &e.LastCheckedAt, &e.NextCheckAt, &e.ErrorCode, &e.ErrorClass, &e.LastError, &e.CreatedAt, &e.UpdatedAt)
	if err == nil && resultMetadata != nil {
		e.ResultMetadata = append(e.ResultMetadata[:0], resultMetadata...)
	}
	return e, err
}

// SaveSucceededResult durably records the minimum provider result before local
// asset/task completion. A replay can rebuild local completion without Submit.
func (s *Store) SaveSucceededResult(ctx context.Context, id int64, providerRequestID *string, metadata []byte) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status Status
	if err = tx.QueryRowContext(ctx, `SELECT status FROM provider_executions WHERE id=$1 FOR UPDATE`, id).Scan(&status); err != nil {
		return err
	}
	if err = ValidateTransition(status, Succeeded); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE provider_executions SET status='succeeded',provider_request_id=COALESCE($1,provider_request_id),result_metadata=$2::jsonb,error_class=$3,last_error=NULL,succeeded_at=now(),updated_at=now() WHERE id=$4`, providerRequestID, string(metadata), string(ProviderSucceeded), id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Transition(ctx context.Context, id int64, to Status, providerRequestID *string, errorClass, lastError *string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var from Status
	if err = tx.QueryRowContext(ctx, `SELECT status FROM provider_executions WHERE id=$1 FOR UPDATE`, id).Scan(&from); err != nil {
		return err
	}
	if err = ValidateTransition(from, to); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE provider_executions SET status=$1,provider_request_id=COALESCE($2,provider_request_id),error_class=$3,last_error=$4,submitted_at=CASE WHEN $1='submitted' THEN now() ELSE submitted_at END,processing_at=CASE WHEN $1='processing' THEN now() ELSE processing_at END,succeeded_at=CASE WHEN $1='succeeded' THEN now() ELSE succeeded_at END,failed_at=CASE WHEN $1='failed' THEN now() ELSE failed_at END,unknown_at=CASE WHEN $1='unknown' THEN now() ELSE unknown_at END,updated_at=now() WHERE id=$5`, to, providerRequestID, errorClass, lastError, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ClaimPrepared changes exactly one prepared execution to submitting under a
// row lock. A crash in this window is intentionally recoverable as unknown.
func (s *Store) ClaimPrepared(ctx context.Context, taskID string) (Execution, error) {
	return s.claimPrepared(ctx, taskID, false)
}

// ClaimPreparedForGenerationTask locks the generation task and execution in a
// single transaction. The lock is retained until submitting is durable.
func (s *Store) ClaimPreparedForGenerationTask(ctx context.Context, taskID string) (Execution, error) {
	return s.claimPrepared(ctx, taskID, true)
}

func (s *Store) claimPrepared(ctx context.Context, taskID string, lockTask bool) (Execution, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Execution{}, err
	}
	defer tx.Rollback()
	if lockTask {
		var status string
		if err = tx.QueryRowContext(ctx, `SELECT status FROM xz_generation_tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&status); err != nil {
			return Execution{}, err
		}
		switch status {
		case "PENDING", "PROCESSING", "RUNNING", "QUEUED":
		default:
			return Execution{}, fmt.Errorf("generation task %s is terminal (%s)", taskID, status)
		}
	}
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM provider_executions WHERE task_id=$1 AND status='prepared' ORDER BY attempt FOR UPDATE SKIP LOCKED LIMIT 1`, taskID).Scan(&id)
	if err != nil {
		return Execution{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE provider_executions SET status='submitting',updated_at=now() WHERE id=$1`, id); err != nil {
		return Execution{}, err
	}
	if err = tx.Commit(); err != nil {
		return Execution{}, err
	}
	e, err := s.GetByID(ctx, id)
	if err != nil {
		return Execution{}, err
	}
	if e.ProviderOperationKey == "" {
		e.ProviderOperationKey = fmt.Sprintf("generation:%s:%d", e.TaskID, e.Attempt)
		if _, err := s.DB.ExecContext(ctx, `UPDATE provider_executions SET provider_operation_key=$1,updated_at=now() WHERE id=$2 AND provider_operation_key=''`, e.ProviderOperationKey, e.ID); err != nil {
			return Execution{}, err
		}
	}
	return e, nil
}
func (s *Store) MarkUnknown(ctx context.Context, id int64, class ErrorClass, msg string) error {
	e, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if e.Status == Unknown {
		return nil
	}
	return s.Transition(ctx, id, Unknown, nil, ptr(string(class)), ptr(msg))
}
func ptr(v string) *string { return &v }
