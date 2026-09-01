package providerexecution

import (
	"context"
	"database/sql"
	"fmt"
)

type Store struct{ DB *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{DB: db} }

func (s *Store) CreatePrepared(ctx context.Context, e Execution) (Execution, error) {
	if s == nil || s.DB == nil {
		return Execution{}, fmt.Errorf("provider execution database is required")
	}
	if e.Attempt <= 0 {
		e.Attempt = 1
	}
	e.Status = Prepared
	row := s.DB.QueryRowContext(ctx, `INSERT INTO provider_executions (task_id,provider,provider_channel,provider_model,capability,attempt,status,request_fingerprint) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id,created_at,updated_at`, e.TaskID, e.Provider, e.ProviderChannel, e.ProviderModel, e.Capability, e.Attempt, e.Status, e.RequestFingerprint)
	if err := row.Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return Execution{}, err
	}
	return e, nil
}

func (s *Store) GetByID(ctx context.Context, id int64) (Execution, error) {
	return s.get(ctx, `SELECT id,task_id,provider,provider_channel,provider_model,capability,attempt,status,request_fingerprint,provider_request_id,submitted_at,processing_at,succeeded_at,failed_at,unknown_at,last_checked_at,next_check_at,error_code,error_class,last_error,created_at,updated_at FROM provider_executions WHERE id=$1`, id)
}
func (s *Store) GetActiveByTask(ctx context.Context, taskID string) (Execution, error) {
	return s.get(ctx, `SELECT id,task_id,provider,provider_channel,provider_model,capability,attempt,status,request_fingerprint,provider_request_id,submitted_at,processing_at,succeeded_at,failed_at,unknown_at,last_checked_at,next_check_at,error_code,error_class,last_error,created_at,updated_at FROM provider_executions WHERE task_id=$1 AND status NOT IN ('succeeded','failed') ORDER BY attempt DESC LIMIT 1`, taskID)
}

func (s *Store) GetLatestByTask(ctx context.Context, taskID string) (Execution, error) {
	return s.get(ctx, `SELECT id,task_id,provider,provider_channel,provider_model,capability,attempt,status,request_fingerprint,provider_request_id,submitted_at,processing_at,succeeded_at,failed_at,unknown_at,last_checked_at,next_check_at,error_code,error_class,last_error,created_at,updated_at FROM provider_executions WHERE task_id=$1 ORDER BY attempt DESC LIMIT 1`, taskID)
}
func (s *Store) get(ctx context.Context, q string, arg any) (Execution, error) {
	var e Execution
	err := s.DB.QueryRowContext(ctx, q, arg).Scan(&e.ID, &e.TaskID, &e.Provider, &e.ProviderChannel, &e.ProviderModel, &e.Capability, &e.Attempt, &e.Status, &e.RequestFingerprint, &e.ProviderRequestID, &e.SubmittedAt, &e.ProcessingAt, &e.SucceededAt, &e.FailedAt, &e.UnknownAt, &e.LastCheckedAt, &e.NextCheckAt, &e.ErrorCode, &e.ErrorClass, &e.LastError, &e.CreatedAt, &e.UpdatedAt)
	return e, err
}

func (s *Store) Transition(ctx context.Context, id int64, to Status, providerRequestID *string, errorClass, lastError *string) error {
	e, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err = ValidateTransition(e.Status, to); err != nil {
		return err
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE provider_executions SET status=$1,provider_request_id=COALESCE($2,provider_request_id),error_class=$3,last_error=$4,submitted_at=CASE WHEN $1='submitted' THEN now() ELSE submitted_at END,processing_at=CASE WHEN $1='processing' THEN now() ELSE processing_at END,succeeded_at=CASE WHEN $1='succeeded' THEN now() ELSE succeeded_at END,failed_at=CASE WHEN $1='failed' THEN now() ELSE failed_at END,unknown_at=CASE WHEN $1='unknown' THEN now() ELSE unknown_at END,updated_at=now() WHERE id=$5 AND status=$6`, to, providerRequestID, errorClass, lastError, id, e.Status)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrTransitionConflict
	}
	return nil
}

// ClaimPrepared changes exactly one prepared execution to submitting under a
// row lock. A crash in this window is intentionally recoverable as unknown.
func (s *Store) ClaimPrepared(ctx context.Context, taskID string) (Execution, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Execution{}, err
	}
	defer tx.Rollback()
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
	return s.GetByID(ctx, id)
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
