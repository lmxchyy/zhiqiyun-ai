package ppt

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type PostgresGenerationJobStore struct {
	db *sql.DB
}

func NewPostgresGenerationJobStore(db *sql.DB) (*PostgresGenerationJobStore, error) {
	if db == nil {
		return nil, ErrGenerationJobInvalid
	}
	return &PostgresGenerationJobStore{db: db}, nil
}

const generationJobColumns = `
  id,tenant_id,user_id,organization_id,existing_task_id,client_request_id,idempotency_key,
  status,stage,attempt_count,max_attempts,run_after,coalesce(lease_owner,''),lease_expires_at,
  fencing_token,completed_work_units,total_work_units,deck_job_id,input_snapshot,
  coalesce(deck_id,''),coalesce(revision,0),slide_count,coalesce(render_sha256,''),render_bytes,
  coalesce(file_id,''),coalesce(asset_id,''),error,created_at,updated_at,started_at,finished_at,cancel_requested_at`

type generationJobScanner interface {
	Scan(...any) error
}

func scanPostgresGenerationJob(row generationJobScanner) (GenerationJob, error) {
	var job GenerationJob
	var leaseExpiresAt, startedAt, finishedAt, cancelRequestedAt sql.NullTime
	var inputSnapshot, renderBytes, rawError []byte
	err := row.Scan(
		&job.ID, &job.TenantID, &job.UserID, &job.OrganizationID, &job.ExistingTaskID, &job.ClientRequestID, &job.IdempotencyKey,
		&job.Status, &job.Stage, &job.AttemptCount, &job.MaxAttempts, &job.RunAfter, &job.LeaseOwner, &leaseExpiresAt,
		&job.FencingToken, &job.CompletedWorkUnits, &job.TotalWorkUnits, &job.DeckJobID, &inputSnapshot,
		&job.DeckID, &job.Revision, &job.SlideCount, &job.RenderSHA256, &renderBytes,
		&job.FileID, &job.AssetID, &rawError, &job.CreatedAt, &job.UpdatedAt, &startedAt, &finishedAt, &cancelRequestedAt,
	)
	if err != nil {
		return GenerationJob{}, err
	}
	job.InputSnapshot = append([]byte(nil), inputSnapshot...)
	job.RenderBytes = append([]byte(nil), renderBytes...)
	if len(rawError) > 0 {
		var failure GenerationJobError
		if err := json.Unmarshal(rawError, &failure); err != nil {
			return GenerationJob{}, err
		}
		job.LastError = &failure
	}
	if leaseExpiresAt.Valid {
		job.LeaseExpiresAt = leaseExpiresAt.Time
	}
	if startedAt.Valid {
		job.StartedAt = startedAt.Time
	}
	if finishedAt.Valid {
		job.FinishedAt = finishedAt.Time
	}
	if cancelRequestedAt.Valid {
		job.CancelRequestedAt = cancelRequestedAt.Time
	}
	return job, nil
}

func (s *PostgresGenerationJobStore) Create(ctx context.Context, input CreateGenerationJobInput) (GenerationJob, bool, error) {
	job, deck, slides, err := NormalizeCreateGenerationJob(input)
	if err != nil {
		return GenerationJob{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GenerationJob{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	existing, err := scanPostgresGenerationJob(tx.QueryRowContext(ctx, `select `+generationJobColumns+` from xz_ppt_v2_generation_jobs where tenant_id=$1 and user_id=$2 and idempotency_key=$3 for update`, job.TenantID, job.UserID, job.IdempotencyKey))
	if err == nil {
		if err := validateGenerationIdempotentReplay(existing, job); err != nil {
			return GenerationJob{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return GenerationJob{}, false, err
		}
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return GenerationJob{}, false, err
	}
	_, err = tx.ExecContext(ctx, `
insert into xz_ppt_v2_generation_jobs(
  id,tenant_id,user_id,organization_id,existing_task_id,client_request_id,idempotency_key,status,stage,
  attempt_count,max_attempts,run_after,fencing_token,completed_work_units,total_work_units,deck_job_id,slide_count,
  created_at,updated_at
) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$18)
`, job.ID, job.TenantID, job.UserID, job.OrganizationID, job.ExistingTaskID, job.ClientRequestID, job.IdempotencyKey,
		job.Status, job.Stage, job.AttemptCount, job.MaxAttempts, job.RunAfter, job.FencingToken, job.CompletedWorkUnits,
		job.TotalWorkUnits, job.DeckJobID, job.SlideCount, job.CreatedAt)
	if err != nil {
		_ = tx.Rollback()
		existing, getErr := s.getByIdempotency(ctx, job.TenantID, job.UserID, job.IdempotencyKey)
		if getErr == nil {
			if replayErr := validateGenerationIdempotentReplay(existing, job); replayErr != nil {
				return GenerationJob{}, false, replayErr
			}
			return existing, false, nil
		}
		var existingTaskJobID string
		if taskErr := s.db.QueryRowContext(ctx, `select id from xz_ppt_v2_generation_jobs where existing_task_id=$1`, job.ExistingTaskID).Scan(&existingTaskJobID); taskErr == nil {
			return GenerationJob{}, false, ErrGenerationJobIdempotencyConflict
		}
		return GenerationJob{}, false, fmt.Errorf("create ppt v2 generation job: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `insert into xz_ppt_v2_deck_jobs(id,generation_job_id,status,created_at,updated_at) values($1,$2,$3,$4,$4)`, deck.ID, deck.GenerationJobID, deck.Status, deck.CreatedAt); err != nil {
		return GenerationJob{}, false, err
	}
	for _, slide := range slides {
		if _, err := tx.ExecContext(ctx, `insert into xz_ppt_v2_slide_jobs(id,generation_job_id,deck_job_id,slide_index,status,completed_work_units,created_at,updated_at) values($1,$2,$3,$4,$5,0,$6,$6)`, slide.ID, slide.GenerationJobID, slide.DeckJobID, slide.SlideIndex, slide.Status, slide.CreatedAt); err != nil {
			return GenerationJob{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `insert into xz_ppt_v2_generation_transitions(generation_job_id,from_stage,to_stage,fencing_token,checkpoint,created_at) values($1,null,$2,0,'{"completedWorkUnits":0}'::jsonb,$3)`, job.ID, GenerationStageCreated, job.CreatedAt); err != nil {
		return GenerationJob{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return GenerationJob{}, false, err
	}
	return job, true, nil
}

func validateGenerationIdempotentReplay(existing, requested GenerationJob) error {
	if existing.ExistingTaskID != requested.ExistingTaskID || existing.OrganizationID != requested.OrganizationID || existing.SlideCount != requested.SlideCount {
		return ErrGenerationJobIdempotencyConflict
	}
	return nil
}

func (s *PostgresGenerationJobStore) getByIdempotency(ctx context.Context, tenantID, userID, key string) (GenerationJob, error) {
	job, err := scanPostgresGenerationJob(s.db.QueryRowContext(ctx, `select `+generationJobColumns+` from xz_ppt_v2_generation_jobs where tenant_id=$1 and user_id=$2 and idempotency_key=$3`, tenantID, userID, key))
	if errors.Is(err, sql.ErrNoRows) {
		return GenerationJob{}, ErrGenerationJobNotFound
	}
	return job, err
}

func (s *PostgresGenerationJobStore) Get(ctx context.Context, scope GenerationJobScope, jobID string) (GenerationJobBundle, error) {
	job, err := scanPostgresGenerationJob(s.db.QueryRowContext(ctx, `select `+generationJobColumns+` from xz_ppt_v2_generation_jobs where id=$1 and tenant_id=$2 and user_id=$3`, strings.TrimSpace(jobID), strings.TrimSpace(scope.TenantID), strings.TrimSpace(scope.UserID)))
	if errors.Is(err, sql.ErrNoRows) {
		return GenerationJobBundle{}, ErrGenerationJobNotFound
	}
	if err != nil {
		return GenerationJobBundle{}, err
	}
	bundle := GenerationJobBundle{Job: job}
	if err := s.db.QueryRowContext(ctx, `select id,generation_job_id,coalesce(deck_id,''),coalesce(revision,0),status,created_at,updated_at from xz_ppt_v2_deck_jobs where generation_job_id=$1`, job.ID).Scan(
		&bundle.Deck.ID, &bundle.Deck.GenerationJobID, &bundle.Deck.DeckID, &bundle.Deck.Revision, &bundle.Deck.Status, &bundle.Deck.CreatedAt, &bundle.Deck.UpdatedAt,
	); err != nil {
		return GenerationJobBundle{}, err
	}
	rows, err := s.db.QueryContext(ctx, `select id,generation_job_id,deck_job_id,slide_index,coalesce(source_slide_id,''),status,completed_work_units,created_at,updated_at from xz_ppt_v2_slide_jobs where generation_job_id=$1 order by slide_index`, job.ID)
	if err != nil {
		return GenerationJobBundle{}, err
	}
	for rows.Next() {
		var slide SlideJob
		if err := rows.Scan(&slide.ID, &slide.GenerationJobID, &slide.DeckJobID, &slide.SlideIndex, &slide.SourceSlideID, &slide.Status, &slide.CompletedWorkUnits, &slide.CreatedAt, &slide.UpdatedAt); err != nil {
			_ = rows.Close()
			return GenerationJobBundle{}, err
		}
		bundle.Slides = append(bundle.Slides, slide)
	}
	if err := rows.Close(); err != nil {
		return GenerationJobBundle{}, err
	}
	if err := rows.Err(); err != nil {
		return GenerationJobBundle{}, err
	}
	attemptRows, err := s.db.QueryContext(ctx, `select id,generation_job_id,attempt_number,worker_id,fencing_token,status,coalesce(usage_identity,''),error,started_at,finished_at from xz_ppt_v2_generation_attempts where generation_job_id=$1 order by attempt_number`, job.ID)
	if err != nil {
		return GenerationJobBundle{}, err
	}
	for attemptRows.Next() {
		var attempt GenerationAttempt
		var rawError []byte
		var finishedAt sql.NullTime
		if err := attemptRows.Scan(&attempt.ID, &attempt.JobID, &attempt.AttemptNumber, &attempt.WorkerID, &attempt.FencingToken, &attempt.Status, &attempt.UsageIdentity, &rawError, &attempt.StartedAt, &finishedAt); err != nil {
			_ = attemptRows.Close()
			return GenerationJobBundle{}, err
		}
		if len(rawError) > 0 {
			var failure GenerationJobError
			if err := json.Unmarshal(rawError, &failure); err != nil {
				_ = attemptRows.Close()
				return GenerationJobBundle{}, err
			}
			attempt.Error = &failure
		}
		if finishedAt.Valid {
			attempt.FinishedAt = finishedAt.Time
		}
		bundle.Attempts = append(bundle.Attempts, attempt)
	}
	if err := attemptRows.Close(); err != nil {
		return GenerationJobBundle{}, err
	}
	if err := attemptRows.Err(); err != nil {
		return GenerationJobBundle{}, err
	}
	historyRows, err := s.db.QueryContext(ctx, `select generation_job_id,coalesce(attempt_id,''),coalesce(from_stage,''),to_stage,fencing_token,checkpoint,created_at from xz_ppt_v2_generation_transitions where generation_job_id=$1 order by id`, job.ID)
	if err != nil {
		return GenerationJobBundle{}, err
	}
	for historyRows.Next() {
		var transition GenerationTransition
		var checkpoint []byte
		if err := historyRows.Scan(&transition.JobID, &transition.AttemptID, &transition.FromStage, &transition.ToStage, &transition.FencingToken, &checkpoint, &transition.CreatedAt); err != nil {
			_ = historyRows.Close()
			return GenerationJobBundle{}, err
		}
		transition.Checkpoint = map[string]any{}
		if err := json.Unmarshal(checkpoint, &transition.Checkpoint); err != nil {
			_ = historyRows.Close()
			return GenerationJobBundle{}, err
		}
		bundle.History = append(bundle.History, transition)
	}
	if err := historyRows.Close(); err != nil {
		return GenerationJobBundle{}, err
	}
	return bundle, historyRows.Err()
}

func (s *PostgresGenerationJobStore) Claim(ctx context.Context, scope GenerationJobScope, jobID, workerID string, now time.Time, duration time.Duration) (GenerationLease, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" || duration <= 0 {
		return GenerationLease{}, ErrGenerationJobInvalid
	}
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GenerationLease{}, err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := loadGenerationJobForUpdate(ctx, tx, scope, jobID)
	if err != nil {
		return GenerationLease{}, err
	}
	if job.Terminal() {
		if job.Status == GenerationJobCancelled {
			return GenerationLease{}, ErrGenerationJobCancelled
		}
		return GenerationLease{}, ErrGenerationJobTerminal
	}
	if job.Status == GenerationJobRunning && job.LeaseExpiresAt.After(now) {
		return GenerationLease{}, ErrGenerationJobLeaseHeld
	}
	if job.Status == GenerationJobRetryWait && job.RunAfter.After(now) {
		return GenerationLease{}, ErrGenerationJobNotReady
	}
	if job.AttemptCount >= job.MaxAttempts {
		failure := GenerationJobError{Code: "LEASE_EXPIRED", Message: "generation worker lease expired", Stage: job.Stage, Retryable: false, AttemptID: generationAttemptID(job.ID, job.AttemptCount)}
		job.LastError = &failure
		job.Status = GenerationJobFailed
		job.FinishedAt = now
		job.UpdatedAt = now
		if err := persistGenerationJob(ctx, tx, job); err != nil {
			return GenerationLease{}, err
		}
		if job.AttemptCount > 0 {
			if err := finishPostgresGenerationAttempt(ctx, tx, failure.AttemptID, GenerationAttemptFailed, &failure, now); err != nil {
				return GenerationLease{}, err
			}
		}
		if err := tx.Commit(); err != nil {
			return GenerationLease{}, err
		}
		return GenerationLease{}, ErrGenerationJobTerminal
	}
	if job.Status == GenerationJobRunning && !job.LeaseExpiresAt.After(now) && job.AttemptCount > 0 {
		failure := GenerationJobError{Code: "LEASE_EXPIRED", Message: "generation worker lease expired", Stage: job.Stage, Retryable: true, AttemptID: generationAttemptID(job.ID, job.AttemptCount)}
		if err := finishPostgresGenerationAttempt(ctx, tx, failure.AttemptID, GenerationAttemptRetryWait, &failure, now); err != nil {
			return GenerationLease{}, err
		}
	}
	job.Status = GenerationJobRunning
	job.AttemptCount++
	job.FencingToken++
	job.LeaseOwner = workerID
	job.LeaseExpiresAt = now.Add(duration)
	job.UpdatedAt = now
	if job.StartedAt.IsZero() {
		job.StartedAt = now
	}
	if err := persistGenerationJob(ctx, tx, job); err != nil {
		return GenerationLease{}, err
	}
	attempt := GenerationAttempt{ID: generationAttemptID(job.ID, job.AttemptCount), JobID: job.ID, AttemptNumber: job.AttemptCount, WorkerID: workerID, FencingToken: job.FencingToken, Status: GenerationAttemptRunning, StartedAt: now}
	if _, err := tx.ExecContext(ctx, `insert into xz_ppt_v2_generation_attempts(id,generation_job_id,attempt_number,worker_id,fencing_token,status,started_at) values($1,$2,$3,$4,$5,$6,$7)`, attempt.ID, attempt.JobID, attempt.AttemptNumber, attempt.WorkerID, attempt.FencingToken, attempt.Status, attempt.StartedAt); err != nil {
		return GenerationLease{}, err
	}
	if _, err := tx.ExecContext(ctx, `update xz_ppt_v2_deck_jobs set status=$2,updated_at=$3 where generation_job_id=$1 and status='PENDING'`, job.ID, GenerationChildRunning, now); err != nil {
		return GenerationLease{}, err
	}
	if err := tx.Commit(); err != nil {
		return GenerationLease{}, err
	}
	return GenerationLease{JobID: job.ID, TenantID: job.TenantID, UserID: job.UserID, WorkerID: workerID, AttemptID: attempt.ID, FencingToken: job.FencingToken, LeaseExpiresAt: job.LeaseExpiresAt, Job: job}, nil
}

func (s *PostgresGenerationJobStore) Renew(ctx context.Context, lease GenerationLease, now time.Time, duration time.Duration) (GenerationLease, error) {
	if duration <= 0 {
		return GenerationLease{}, ErrGenerationJobInvalid
	}
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GenerationLease{}, err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := loadGenerationJobForLease(ctx, tx, lease)
	if err != nil {
		return GenerationLease{}, err
	}
	if err := validatePostgresGenerationLease(job, lease, now); err != nil {
		return GenerationLease{}, err
	}
	job.LeaseExpiresAt = now.Add(duration)
	job.UpdatedAt = now
	if err := persistGenerationJob(ctx, tx, job); err != nil {
		return GenerationLease{}, err
	}
	if err := tx.Commit(); err != nil {
		return GenerationLease{}, err
	}
	lease.LeaseExpiresAt = job.LeaseExpiresAt
	lease.Job = job
	return lease, nil
}

func (s *PostgresGenerationJobStore) Checkpoint(ctx context.Context, lease GenerationLease, checkpoint GenerationCheckpoint) (GenerationJob, error) {
	if checkpoint.Now.IsZero() {
		checkpoint.Now = time.Now().UTC()
	} else {
		checkpoint.Now = checkpoint.Now.UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GenerationJob{}, err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := loadGenerationJobForLease(ctx, tx, lease)
	if err != nil {
		return GenerationJob{}, err
	}
	if err := validatePostgresGenerationLease(job, lease, checkpoint.Now); err != nil {
		return GenerationJob{}, err
	}
	if err := validateGenerationCheckpoint(job, checkpoint); err != nil {
		return GenerationJob{}, err
	}
	fromStage := job.Stage
	deck, slides, err := loadGenerationChildrenForUpdate(ctx, tx, job.ID)
	if err != nil {
		return GenerationJob{}, err
	}
	applyGenerationCheckpoint(&job, &deck, slides, checkpoint)
	if err := persistGenerationJob(ctx, tx, job); err != nil {
		return GenerationJob{}, err
	}
	if _, err := tx.ExecContext(ctx, `update xz_ppt_v2_deck_jobs set deck_id=nullif($2,''),revision=nullif($3,0),status=$4,updated_at=$5 where generation_job_id=$1`, job.ID, deck.DeckID, deck.Revision, deck.Status, deck.UpdatedAt); err != nil {
		return GenerationJob{}, err
	}
	for _, slide := range slides {
		if _, err := tx.ExecContext(ctx, `update xz_ppt_v2_slide_jobs set source_slide_id=nullif($3,''),status=$4,completed_work_units=$5,updated_at=$6 where generation_job_id=$1 and slide_index=$2`, job.ID, slide.SlideIndex, slide.SourceSlideID, slide.Status, slide.CompletedWorkUnits, slide.UpdatedAt); err != nil {
			return GenerationJob{}, err
		}
	}
	checkpointJSON, _ := json.Marshal(map[string]any{"completedWorkUnits": job.CompletedWorkUnits, "fileId": job.FileID, "assetId": job.AssetID, "renderSha256": job.RenderSHA256})
	if _, err := tx.ExecContext(ctx, `insert into xz_ppt_v2_generation_transitions(generation_job_id,attempt_id,from_stage,to_stage,fencing_token,checkpoint,created_at) values($1,$2,$3,$4,$5,$6::jsonb,$7)`, job.ID, lease.AttemptID, fromStage, checkpoint.NextStage, lease.FencingToken, string(checkpointJSON), checkpoint.Now); err != nil {
		return GenerationJob{}, err
	}
	if checkpoint.NextStage == GenerationStageCompleted {
		if err := finishPostgresGenerationAttempt(ctx, tx, lease.AttemptID, GenerationAttemptSucceeded, nil, checkpoint.Now); err != nil {
			return GenerationJob{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return GenerationJob{}, err
	}
	return job, nil
}

func (s *PostgresGenerationJobStore) Fail(ctx context.Context, lease GenerationLease, failure GenerationJobError, now time.Time, retryDelay time.Duration) (GenerationJob, error) {
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GenerationJob{}, err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := loadGenerationJobForLease(ctx, tx, lease)
	if err != nil {
		return GenerationJob{}, err
	}
	if err := validatePostgresGenerationLease(job, lease, now); err != nil {
		return GenerationJob{}, err
	}
	failure.Code = strings.TrimSpace(failure.Code)
	failure.Message = strings.TrimSpace(failure.Message)
	failure.Stage = job.Stage
	failure.AttemptID = lease.AttemptID
	if failure.Code == "" || failure.Message == "" {
		return GenerationJob{}, ErrGenerationJobInvalid
	}
	job.LastError = &failure
	job.LeaseOwner = ""
	job.LeaseExpiresAt = time.Time{}
	job.UpdatedAt = now
	attemptStatus := GenerationAttemptFailed
	if failure.Retryable && job.AttemptCount < job.MaxAttempts {
		job.Status = GenerationJobRetryWait
		job.RunAfter = now.Add(retryDelay)
		attemptStatus = GenerationAttemptRetryWait
	} else {
		job.Status = GenerationJobFailed
		job.FinishedAt = now
		if _, err := tx.ExecContext(ctx, `update xz_ppt_v2_deck_jobs set status=$2,updated_at=$3 where generation_job_id=$1 and status<>'SUCCEEDED'`, job.ID, GenerationChildFailed, now); err != nil {
			return GenerationJob{}, err
		}
	}
	if err := persistGenerationJob(ctx, tx, job); err != nil {
		return GenerationJob{}, err
	}
	if err := finishPostgresGenerationAttempt(ctx, tx, lease.AttemptID, attemptStatus, &failure, now); err != nil {
		return GenerationJob{}, err
	}
	if err := tx.Commit(); err != nil {
		return GenerationJob{}, err
	}
	return job, nil
}

func (s *PostgresGenerationJobStore) Cancel(ctx context.Context, scope GenerationJobScope, jobID string, now time.Time) (GenerationJob, error) {
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GenerationJob{}, err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := loadGenerationJobForUpdate(ctx, tx, scope, jobID)
	if err != nil {
		return GenerationJob{}, err
	}
	if job.Terminal() {
		if err := tx.Commit(); err != nil {
			return GenerationJob{}, err
		}
		return job, nil
	}
	job.Status = GenerationJobCancelled
	job.CancelRequestedAt = now
	job.FinishedAt = now
	job.UpdatedAt = now
	job.LeaseOwner = ""
	job.LeaseExpiresAt = time.Time{}
	if err := persistGenerationJob(ctx, tx, job); err != nil {
		return GenerationJob{}, err
	}
	if _, err := tx.ExecContext(ctx, `update xz_ppt_v2_deck_jobs set status=$2,updated_at=$3 where generation_job_id=$1 and status<>'SUCCEEDED'`, job.ID, GenerationChildCancelled, now); err != nil {
		return GenerationJob{}, err
	}
	if _, err := tx.ExecContext(ctx, `update xz_ppt_v2_slide_jobs set status=$2,updated_at=$3 where generation_job_id=$1 and status<>'SUCCEEDED'`, job.ID, GenerationChildCancelled, now); err != nil {
		return GenerationJob{}, err
	}
	if job.AttemptCount > 0 {
		if err := finishPostgresGenerationAttempt(ctx, tx, generationAttemptID(job.ID, job.AttemptCount), GenerationAttemptCancelled, nil, now); err != nil {
			return GenerationJob{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return GenerationJob{}, err
	}
	return job, nil
}

// RelateTaskArtifact commits the legacy Task relation and the durable
// TASK_RELATED checkpoint in one PostgreSQL transaction. The lease and fencing
// token are checked while the job row is locked, so cancellation or a newer
// worker prevents the relation write.
func (s *PostgresGenerationJobStore) RelateTaskArtifact(ctx context.Context, lease GenerationLease, relation V2ArtifactRelation, now time.Time) (GenerationJob, error) {
	now = now.UTC()
	relation.DeckID = strings.TrimSpace(relation.DeckID)
	relation.PPTXAssetID = strings.TrimSpace(relation.PPTXAssetID)
	if relation.DeckID == "" || relation.Revision <= 0 || relation.PPTXAssetID == "" {
		return GenerationJob{}, ErrInvalidV2ArtifactRelation
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GenerationJob{}, err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := loadGenerationJobForLease(ctx, tx, lease)
	if err != nil {
		return GenerationJob{}, err
	}
	if err := validatePostgresGenerationLease(job, lease, now); err != nil {
		return GenerationJob{}, err
	}
	if job.Stage != GenerationStageAssetCreated {
		return GenerationJob{}, ErrGenerationJobTransition
	}
	if relation.DeckID != job.DeckID || relation.Revision != job.Revision || relation.PPTXAssetID != job.AssetID {
		return GenerationJob{}, ErrGenerationJobIdempotencyConflict
	}
	var raw []byte
	var taskTenantID, taskOrganizationID string
	err = tx.QueryRowContext(ctx, `select raw,tenant_id,organization_id from xz_ppt_tasks where task_id=$1 and user_id=$2 for update`, job.ExistingTaskID, job.UserID).Scan(&raw, &taskTenantID, &taskOrganizationID)
	if errors.Is(err, sql.ErrNoRows) {
		return GenerationJob{}, ErrTaskNotFound
	}
	if err != nil {
		return GenerationJob{}, err
	}
	if taskTenantID != job.TenantID || (job.OrganizationID != "" && taskOrganizationID != job.OrganizationID) {
		return GenerationJob{}, ErrTaskNotFound
	}
	task, err := taskFromPostgresRaw(raw, job.UserID)
	if err != nil {
		return GenerationJob{}, err
	}
	task.TenantID = taskTenantID
	task.OrganizationID = taskOrganizationID
	if task.V2DeckID != "" || task.V2Revision != 0 || task.PPTXAssetID != "" {
		if task.V2DeckID != relation.DeckID || task.V2Revision != relation.Revision || task.PPTXAssetID != relation.PPTXAssetID {
			return GenerationJob{}, ErrV2ArtifactRelationConflict
		}
	} else {
		task.V2DeckID = relation.DeckID
		task.V2Revision = relation.Revision
		task.PPTXAssetID = relation.PPTXAssetID
		task.UpdatedAt = now.Format(time.RFC3339Nano)
		if err := persistPostgresTask(ctx, tx, task); err != nil {
			return GenerationJob{}, err
		}
	}
	fromStage := job.Stage
	job.Stage = GenerationStageTaskRelated
	job.CompletedWorkUnits = generationStageWorkUnits(GenerationStageTaskRelated)
	job.UpdatedAt = now
	if err := persistGenerationJob(ctx, tx, job); err != nil {
		return GenerationJob{}, err
	}
	checkpointJSON, _ := json.Marshal(map[string]any{"completedWorkUnits": job.CompletedWorkUnits, "assetId": job.AssetID, "taskId": job.ExistingTaskID})
	if _, err := tx.ExecContext(ctx, `insert into xz_ppt_v2_generation_transitions(generation_job_id,attempt_id,from_stage,to_stage,fencing_token,checkpoint,created_at) values($1,$2,$3,$4,$5,$6::jsonb,$7)`, job.ID, lease.AttemptID, fromStage, GenerationStageTaskRelated, lease.FencingToken, string(checkpointJSON), now); err != nil {
		return GenerationJob{}, err
	}
	if err := tx.Commit(); err != nil {
		return GenerationJob{}, err
	}
	return job, nil
}

func loadGenerationJobForUpdate(ctx context.Context, tx *sql.Tx, scope GenerationJobScope, jobID string) (GenerationJob, error) {
	job, err := scanPostgresGenerationJob(tx.QueryRowContext(ctx, `select `+generationJobColumns+` from xz_ppt_v2_generation_jobs where id=$1 and tenant_id=$2 and user_id=$3 for update`, strings.TrimSpace(jobID), strings.TrimSpace(scope.TenantID), strings.TrimSpace(scope.UserID)))
	if errors.Is(err, sql.ErrNoRows) {
		return GenerationJob{}, ErrGenerationJobNotFound
	}
	return job, err
}

func loadGenerationJobForLease(ctx context.Context, tx *sql.Tx, lease GenerationLease) (GenerationJob, error) {
	return loadGenerationJobForUpdate(ctx, tx, GenerationJobScope{TenantID: lease.TenantID, UserID: lease.UserID}, lease.JobID)
}

func validatePostgresGenerationLease(job GenerationJob, lease GenerationLease, now time.Time) error {
	if job.Terminal() {
		if job.Status == GenerationJobCancelled {
			return ErrGenerationJobCancelled
		}
		return ErrGenerationJobTerminal
	}
	if job.Status != GenerationJobRunning || job.LeaseOwner != lease.WorkerID || job.FencingToken != lease.FencingToken || !job.LeaseExpiresAt.After(now.UTC()) {
		return ErrGenerationJobLeaseLost
	}
	return nil
}

func persistGenerationJob(ctx context.Context, tx *sql.Tx, job GenerationJob) error {
	inputSnapshot := nullableJSON(job.InputSnapshot)
	rawError, err := nullableGenerationError(job.LastError)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
update xz_ppt_v2_generation_jobs set
  status=$2,stage=$3,attempt_count=$4,max_attempts=$5,run_after=$6,lease_owner=nullif($7,''),lease_expires_at=$8,
  fencing_token=$9,completed_work_units=$10,total_work_units=$11,input_snapshot=$12::jsonb,deck_id=nullif($13,''),
  revision=nullif($14,0),slide_count=$15,render_sha256=nullif($16,''),render_bytes=$17,file_id=nullif($18,''),
  asset_id=nullif($19,''),error=$20::jsonb,updated_at=$21,started_at=$22,finished_at=$23,cancel_requested_at=$24
where id=$1
`, job.ID, job.Status, job.Stage, job.AttemptCount, job.MaxAttempts, job.RunAfter, job.LeaseOwner, nullableTime(job.LeaseExpiresAt),
		job.FencingToken, job.CompletedWorkUnits, job.TotalWorkUnits, inputSnapshot, job.DeckID, job.Revision, job.SlideCount,
		job.RenderSHA256, nullableBytes(job.RenderBytes), job.FileID, job.AssetID, rawError, job.UpdatedAt,
		nullableTime(job.StartedAt), nullableTime(job.FinishedAt), nullableTime(job.CancelRequestedAt))
	return err
}

func loadGenerationChildrenForUpdate(ctx context.Context, tx *sql.Tx, jobID string) (DeckJob, []SlideJob, error) {
	var deck DeckJob
	if err := tx.QueryRowContext(ctx, `select id,generation_job_id,coalesce(deck_id,''),coalesce(revision,0),status,created_at,updated_at from xz_ppt_v2_deck_jobs where generation_job_id=$1 for update`, jobID).Scan(
		&deck.ID, &deck.GenerationJobID, &deck.DeckID, &deck.Revision, &deck.Status, &deck.CreatedAt, &deck.UpdatedAt,
	); err != nil {
		return DeckJob{}, nil, err
	}
	rows, err := tx.QueryContext(ctx, `select id,generation_job_id,deck_job_id,slide_index,coalesce(source_slide_id,''),status,completed_work_units,created_at,updated_at from xz_ppt_v2_slide_jobs where generation_job_id=$1 order by slide_index for update`, jobID)
	if err != nil {
		return DeckJob{}, nil, err
	}
	defer rows.Close()
	slides := []SlideJob{}
	for rows.Next() {
		var slide SlideJob
		if err := rows.Scan(&slide.ID, &slide.GenerationJobID, &slide.DeckJobID, &slide.SlideIndex, &slide.SourceSlideID, &slide.Status, &slide.CompletedWorkUnits, &slide.CreatedAt, &slide.UpdatedAt); err != nil {
			return DeckJob{}, nil, err
		}
		slides = append(slides, slide)
	}
	return deck, slides, rows.Err()
}

func finishPostgresGenerationAttempt(ctx context.Context, tx *sql.Tx, attemptID, status string, failure *GenerationJobError, now time.Time) error {
	rawError, err := nullableGenerationError(failure)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `update xz_ppt_v2_generation_attempts set status=$2,error=$3::jsonb,finished_at=$4 where id=$1 and status='RUNNING'`, attemptID, status, rawError, now)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		var current string
		if err := tx.QueryRowContext(ctx, `select status from xz_ppt_v2_generation_attempts where id=$1`, attemptID).Scan(&current); err != nil {
			return err
		}
		if current != status {
			return ErrGenerationJobLeaseLost
		}
	}
	return nil
}

func nullableGenerationError(failure *GenerationJobError) (any, error) {
	if failure == nil {
		return nil, nil
	}
	raw, err := json.Marshal(failure)
	if err != nil {
		return nil, err
	}
	return string(raw), nil
}

func nullableJSON(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}

func nullableBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

var _ GenerationJobStore = (*PostgresGenerationJobStore)(nil)
var _ GenerationTaskRelationStore = (*PostgresGenerationJobStore)(nil)
