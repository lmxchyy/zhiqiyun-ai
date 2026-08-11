package smartvideo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const renderTaskColumns = `id,project_id,coalesce(version_id,''),tenant_id,user_id,client_request_id,
 status,progress,coalesce(stage,''),step,attempt_count,max_attempts,run_after,specification,
 quoted_tokens,reserved_tokens,captured_tokens,released_tokens,
 coalesce(quoted_points,0),coalesce(reserved_points,0),coalesce(captured_points,0),coalesce(released_points,0),
 coalesce(output_file_id,''),coalesce(cover_file_id,''),coalesce(output_asset_id,''),
 coalesce(voice_file_id,''),coalesce(caption_file_id,''),coalesce(work_id,''),coalesce(manifest_hash,''),
 attempt,coalesce(retry_of_task_id,''),coalesce(billing_transaction_id,''),
 output_metadata,coalesce(error_code,''),coalesce(error_message,''),
 created_at,updated_at,started_at,finished_at`

func scanExtendedRenderTask(scanner interface{ Scan(...any) error }) (RenderTask, error) {
	var task RenderTask
	var specification, output []byte
	var stage, voiceFileID, captionFileID, workID, manifestHash, retryOfTaskID, billingTx sql.NullString
	err := scanner.Scan(
		&task.ID, &task.ProjectID, &task.VersionID, &task.TenantID, &task.UserID, &task.ClientRequestID,
		&task.Status, &task.Progress, &stage, &task.Step, &task.AttemptCount, &task.MaxAttempts, &task.RunAfter, &specification,
		&task.QuotedTokens, &task.ReservedTokens, &task.CapturedTokens, &task.ReleasedTokens,
		&task.QuotedPoints, &task.ReservedPoints, &task.CapturedPoints, &task.ReleasedPoints,
		&task.OutputFileID, &task.CoverFileID, &task.OutputAssetID,
		&voiceFileID, &captionFileID, &workID, &manifestHash,
		&task.Attempt, &retryOfTaskID, &billingTx,
		&output, &task.ErrorCode, &task.ErrorMessage,
		&task.CreatedAt, &task.UpdatedAt, &task.StartedAt, &task.FinishedAt,
	)
	if err == nil {
		err = json.Unmarshal(specification, &task.Specification)
	}
	if err == nil && len(output) > 0 {
		err = json.Unmarshal(output, &task.Output)
	}
	if stage.Valid {
		task.Stage = stage.String
	}
	if voiceFileID.Valid {
		task.VoiceFileID = voiceFileID.String
	}
	if captionFileID.Valid {
		task.CaptionFileID = captionFileID.String
	}
	if workID.Valid {
		task.WorkID = workID.String
	}
	if manifestHash.Valid {
		task.ManifestHash = manifestHash.String
	}
	if retryOfTaskID.Valid {
		task.RetryOfTaskID = retryOfTaskID.String
	}
	if billingTx.Valid {
		task.BillingTransactionID = billingTx.String
	}
	return task, err
}

func (r *PostgresRepository) GetRenderTask(ctx context.Context, access Access, projectID, taskID string) (RenderTask, error) {
	task, err := scanExtendedRenderTask(r.db.QueryRowContext(ctx, `select `+renderTaskColumns+`
 from video_render_tasks where id=$1 and project_id=$2 and tenant_id=$3 and user_id=$4`,
		taskID, projectID, access.TenantID, access.UserID))
	if errors.Is(err, sql.ErrNoRows) {
		return RenderTask{}, ErrNotFound
	}
	return task, err
}

func (r *PostgresRepository) MarkRenderQueued(ctx context.Context, taskID string) error {
	result, err := r.db.ExecContext(ctx, `update video_render_tasks set status='QUEUED',stage='queued',step='queued',progress=5,run_after=now(),updated_at=now()
 where id=$1 and status='CREATED'`, taskID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		var status string
		if err := r.db.QueryRowContext(ctx, `select status from video_render_tasks where id=$1`, taskID).Scan(&status); err != nil {
			return err
		}
		if status != RenderStatusQueued {
			return ErrInvalidStateTransition
		}
	}
	return nil
}

func (r *PostgresRepository) AcquireRenderTask(ctx context.Context, taskID, workerID string, lease time.Duration) (RenderTask, error) {
	task, err := scanExtendedRenderTask(r.db.QueryRowContext(ctx, `update video_render_tasks set
 status='PROCESSING',stage='preparing',step='preparing',progress=10,attempt=attempt+1,attempt_count=attempt_count+1,
 lease_owner=$2,lease_expires_at=now()+($3 * interval '1 millisecond'),heartbeat_at=now(),
 started_at=coalesce(started_at,now()),updated_at=now(),error_code=null,error_message=null
 where id=$1 and attempt_count < max_attempts and run_after <= now()
 and (
   status in ('CREATED','QUEUED')
   or (status in ('PROCESSING','SYNTHESIZING','RENDERING','UPLOADING','PUBLISHING') and lease_expires_at < now())
   or (status in ('PROCESSING','SYNTHESIZING','RENDERING','UPLOADING','PUBLISHING') and lease_owner=$2)
 )
 returning `+renderTaskColumns, taskID, workerID, lease.Milliseconds()))
	if errors.Is(err, sql.ErrNoRows) {
		return RenderTask{}, ErrNotFound
	}
	return task, err
}

// RecoverExpiredRenderTasks resets abandoned in-flight renders so workers can reclaim them.
func (r *PostgresRepository) RecoverExpiredRenderTasks(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
		with stuck as (
			select id from video_render_tasks
			where finished_at is null
			  and status in ('CREATED','QUEUED','PROCESSING','SYNTHESIZING','RENDERING','UPLOADING')
			  and (
			    status in ('CREATED','QUEUED')
			    or lease_expires_at is null
			    or lease_expires_at < now()
			  )
			  and updated_at < now() - interval '30 seconds'
			order by updated_at asc
			limit $1
			for update skip locked
		)
		update video_render_tasks t set
			status='QUEUED',
			stage='queued',
			step='queued',
			progress=case when t.progress < 5 then 5 else t.progress end,
			lease_owner=null,
			lease_expires_at=null,
			run_after=now(),
			updated_at=now()
		from stuck
		where t.id=stuck.id
		returning t.id`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *PostgresRepository) HeartbeatRenderTask(ctx context.Context, taskID, workerID string, lease time.Duration) error {
	result, err := r.db.ExecContext(ctx, `update video_render_tasks set heartbeat_at=now(),
 lease_expires_at=now()+($3 * interval '1 millisecond'),updated_at=now()
 where id=$1 and lease_owner=$2 and status in ('PROCESSING','SYNTHESIZING','RENDERING','UPLOADING','PUBLISHING')`,
		taskID, workerID, lease.Milliseconds())
	if err == nil {
		if affected, _ := result.RowsAffected(); affected == 0 {
			err = ErrAnalysisLeaseLost
		}
	}
	return err
}

func (r *PostgresRepository) AdvanceRenderTask(ctx context.Context, taskID, workerID, from, to, stage string, progress int) error {
	if err := ValidateRenderTransition(from, to); err != nil {
		return err
	}
	// stage is varchar, step is text — reuse of one $N param causes PG "inconsistent types".
	result, err := r.db.ExecContext(ctx, `update video_render_tasks set status=$4,stage=$5,step=$6,progress=$7,updated_at=now()
 where id=$1 and ($2='' or lease_owner=$2) and status=$3`, taskID, workerID, from, to, stage, stage, progress)
	if err == nil {
		if affected, _ := result.RowsAffected(); affected == 0 {
			err = ErrInvalidStateTransition
		}
	}
	return err
}

func (r *PostgresRepository) AttachVoiceCaptionArtifacts(ctx context.Context, taskID, workerID, voiceFileID, captionFileID string) error {
	voiceFileID = strings.TrimSpace(voiceFileID)
	captionFileID = strings.TrimSpace(captionFileID)
	result, err := r.db.ExecContext(ctx, `update video_render_tasks set
 voice_file_id=case when $3='' then voice_file_id else $3 end,
 caption_file_id=case when $4='' then caption_file_id else $4 end,
 updated_at=now()
 where id=$1 and lease_owner=$2 and status in ('SYNTHESIZING','PROCESSING','RENDERING')`,
		taskID, workerID, voiceFileID, captionFileID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrInvalidStateTransition
	}
	return nil
}

func (r *PostgresRepository) CompleteRenderTask(ctx context.Context, taskID, workerID string, output RenderOutput) (RenderTask, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return RenderTask{}, err
	}
	defer tx.Rollback()
	var projectID, tenantID, userID, status, workID string
	if err = tx.QueryRowContext(ctx, `select project_id,tenant_id,user_id,status,coalesce(work_id,'')
 from video_render_tasks where id=$1 for update`, taskID).
		Scan(&projectID, &tenantID, &userID, &status, &workID); err != nil {
		return RenderTask{}, err
	}
	if status == RenderStatusSucceeded {
		if err = tx.Commit(); err != nil {
			return RenderTask{}, err
		}
		return r.GetRenderTask(ctx, Access{TenantID: tenantID, UserID: userID}, projectID, taskID)
	}
	// Work center publishing is owned by WorkCenterPublisher; repository only persists render output.
	if status != RenderStatusUploading && status != RenderStatusPublishing {
		return RenderTask{}, ErrInvalidStateTransition
	}
	if status == RenderStatusUploading {
		if _, err = tx.ExecContext(ctx, `update video_render_tasks set status='PUBLISHING',stage='publishing',step='publishing',progress=95,updated_at=now()
 where id=$1 and ($2='' or lease_owner=$2) and status='UPLOADING'`, taskID, workerID); err != nil {
			return RenderTask{}, err
		}
	}
	outputRaw, _ := json.Marshal(output)
	result, err := tx.ExecContext(ctx, `update video_render_tasks set status='SUCCEEDED',stage='completed',step='completed',progress=100,
 output_file_id=$3,cover_file_id=$4,output_metadata=$5::jsonb,
 lease_owner=null,lease_expires_at=null,heartbeat_at=now(),finished_at=now(),updated_at=now()
 where id=$1 and ($2='' or lease_owner=$2) and status='PUBLISHING'`,
		taskID, workerID, output.VideoFileID, output.CoverFileID, outputRaw)
	if err != nil {
		return RenderTask{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return RenderTask{}, ErrAnalysisLeaseLost
	}
	_, err = tx.ExecContext(ctx, `update video_projects set status='COMPLETED',
 active_render_task_id=null,error_stage=null,error_code=null,error_message=null,updated_at=now() where id=$1`, projectID)
	if err != nil {
		return RenderTask{}, err
	}
	if err = tx.Commit(); err != nil {
		return RenderTask{}, err
	}
	return r.GetRenderTask(ctx, Access{TenantID: tenantID, UserID: userID}, projectID, taskID)
}

func (r *PostgresRepository) MarkRenderWorkPublished(ctx context.Context, taskID, workerID, workID string) error {
	result, err := r.db.ExecContext(ctx, `update video_render_tasks set work_id=$3,output_asset_id=$3,updated_at=now()
 where id=$1 and ($2='' or lease_owner=$2) and status in ('PUBLISHING','SUCCEEDED') and coalesce(work_id,'')=''`,
		taskID, workerID, workID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		var existing string
		_ = r.db.QueryRowContext(ctx, `select coalesce(work_id,'') from video_render_tasks where id=$1`, taskID).Scan(&existing)
		if existing != "" && existing != workID {
			return ErrInvalidStateTransition
		}
		if existing == workID {
			return nil
		}
		return ErrNotFound
	}
	_, err = r.db.ExecContext(ctx, `update video_projects set output_asset_id=$2,updated_at=now()
 where id=(select project_id from video_render_tasks where id=$1)`, taskID, workID)
	return err
}

func (r *PostgresRepository) PersistRenderOutput(ctx context.Context, taskID, workerID string, output RenderOutput) (RenderTask, error) {
	outputRaw, _ := json.Marshal(output)
	result, err := r.db.ExecContext(ctx, `update video_render_tasks set
 status='PUBLISHING',stage='publishing',step='publishing',progress=95,
 output_file_id=$3,cover_file_id=$4,output_metadata=$5::jsonb,updated_at=now()
 where id=$1 and ($2='' or lease_owner=$2)
   and status in ('UPLOADING','PUBLISHING')`,
		taskID, workerID, output.VideoFileID, output.CoverFileID, outputRaw)
	if err != nil {
		return RenderTask{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return RenderTask{}, ErrInvalidStateTransition
	}
	var projectID, tenantID, userID string
	if err := r.db.QueryRowContext(ctx, `select project_id,tenant_id,user_id from video_render_tasks where id=$1`, taskID).
		Scan(&projectID, &tenantID, &userID); err != nil {
		return RenderTask{}, err
	}
	return r.GetRenderTask(ctx, Access{TenantID: tenantID, UserID: userID}, projectID, taskID)
}

func (r *PostgresRepository) MarkPointsCaptured(ctx context.Context, taskID string, points int64) error {
	if points < 0 {
		points = 0
	}
	result, err := r.db.ExecContext(ctx, `update video_render_tasks set
 captured_points=$2,updated_at=now()
 where id=$1 and captured_points=0 and released_points=0 and reserved_points>=$2`, taskID, points)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		var captured int64
		_ = r.db.QueryRowContext(ctx, `select captured_points from video_render_tasks where id=$1`, taskID).Scan(&captured)
		if captured > 0 {
			return nil
		}
		return ErrInvalidStateTransition
	}
	return nil
}

func (r *PostgresRepository) FailRenderTask(ctx context.Context, taskID, workerID, code, message string, runAfter time.Time, retry bool) error {
	message = strings.TrimSpace(message)
	if len(message) > 500 {
		message = message[:500]
	}
	if retry {
		result, err := r.db.ExecContext(ctx, `update video_render_tasks set status='QUEUED',stage='retry_wait',step='retry_wait',
 run_after=$3,lease_owner=null,lease_expires_at=null,error_code=$4,error_message=$5,updated_at=now()
 where id=$1 and lease_owner=$2 and attempt_count < max_attempts`, taskID, workerID, runAfter, code, message)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected > 0 {
			return nil
		}
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var projectID string
	err = tx.QueryRowContext(ctx, `update video_render_tasks set status='FAILED',stage='failed',step='failed',
 lease_owner=null,lease_expires_at=null,error_code=$3,error_message=$4,finished_at=now(),updated_at=now()
 where id=$1 and ($2='' or lease_owner=$2) and status not in ('SUCCEEDED','CANCELLED')
 returning project_id`, taskID, workerID, code, message).Scan(&projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `update video_projects set status='FAILED',active_render_task_id=null,
 error_stage='render',error_code=$2,error_message=$3,updated_at=now() where id=$1`, projectID, code, message)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresRepository) RetryRenderTask(ctx context.Context, access Access, projectID, taskID string) (RenderTask, error) {
	result, err := r.db.ExecContext(ctx, `update video_render_tasks set status='QUEUED',stage='queued',step='queued',progress=5,
 attempt_count=0,run_after=now(),error_code=null,error_message=null,finished_at=null,updated_at=now()
 where id=$1 and project_id=$2 and tenant_id=$3 and user_id=$4 and status='FAILED'`,
		taskID, projectID, access.TenantID, access.UserID)
	if err != nil {
		return RenderTask{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return RenderTask{}, fmt.Errorf("%w: render task is not failed", ErrInvalidStateTransition)
	}
	return r.GetRenderTask(ctx, access, projectID, taskID)
}

func (r *PostgresRepository) CreateRenderTaskWithOutbox(ctx context.Context, task RenderTask, outbox OutboxEvent) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	spec, _ := json.Marshal(task.Specification)
	result, err := tx.ExecContext(ctx, `insert into video_render_tasks
		(id,project_id,version_id,tenant_id,user_id,client_request_id,status,progress,stage,step,specification,
		 quoted_points,reserved_points,captured_points,released_points,
		 voice_file_id,caption_file_id,manifest_hash,retry_of_task_id,billing_transaction_id,
		 attempt,attempt_count,max_attempts,run_after,created_at,updated_at)
		values($1,$2,nullif($3,''),$4,$5,$6,$7,$8,$9,$10,$11::jsonb,
		 $12,$13,$14,$15,nullif($16,''),nullif($17,''),nullif($18,''),nullif($19,''),nullif($20,''),
		 $21,$22,$23,$24,$25,$26)
		on conflict(tenant_id,user_id,client_request_id) do nothing`,
		task.ID, task.ProjectID, task.VersionID, task.TenantID, task.UserID, task.ClientRequestID,
		task.Status, task.Progress, firstNonEmptyLocal(task.Stage, "created"), firstNonEmptyLocal(task.Step, "created"), spec,
		task.QuotedPoints, task.ReservedPoints, task.CapturedPoints, task.ReleasedPoints,
		task.VoiceFileID, task.CaptionFileID, task.ManifestHash, task.RetryOfTaskID, task.BillingTransactionID,
		task.Attempt, task.AttemptCount, maxInt(task.MaxAttempts, 3), task.RunAfter, task.CreatedAt, task.UpdatedAt)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrIdempotencyConflict
	}

	payload := []byte("{}")
	if len(outbox.Payload) > 0 {
		payload = outbox.Payload
	}
	_, err = tx.ExecContext(ctx, `insert into video_task_outbox
		(tenant_id,aggregate_type,aggregate_id,event_type,payload,state)
		values($1,$2,$3,$4,$5,'pending')
		on conflict(aggregate_type,aggregate_id,event_type) do nothing`,
		outbox.TenantID, outbox.AggregateType, outbox.AggregateID, outbox.EventType, payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `update video_projects set
		status='RENDERING',active_render_task_id=$2,error_stage=null,error_code=null,error_message=null,updated_at=now()
		where id=$1 and tenant_id=$3`, task.ProjectID, task.ID, task.TenantID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresRepository) CreateRetryRenderTaskWithOutbox(ctx context.Context, task RenderTask, outbox OutboxEvent) error {
	return r.CreateRenderTaskWithOutbox(ctx, task, outbox)
}

func (r *PostgresRepository) CancelRenderTask(ctx context.Context, access Access, projectID, taskID string) (RenderTask, error) {
	result, err := r.db.ExecContext(ctx, `update video_render_tasks set
		status='CANCELLED',stage='cancelled',step='cancelled',
		lease_owner=null,lease_expires_at=null,finished_at=now(),updated_at=now()
		where id=$1 and project_id=$2 and tenant_id=$3 and user_id=$4
		  and status in ('CREATED','QUEUED','PROCESSING','SYNTHESIZING')`,
		taskID, projectID, access.TenantID, access.UserID)
	if err != nil {
		return RenderTask{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return RenderTask{}, ErrRenderNotCancellable
	}
	_, _ = r.db.ExecContext(ctx, `update video_projects set
		active_render_task_id=null,
		status=case when coalesce(confirmed_version_id,'')<>'' then 'CONFIRMED' else status end,
		updated_at=now()
		where id=$1 and tenant_id=$2 and active_render_task_id=$3`, projectID, access.TenantID, taskID)
	return r.GetRenderTask(ctx, access, projectID, taskID)
}

func (r *PostgresRepository) MarkPointsReleased(ctx context.Context, taskID string, points int64) error {
	if points < 0 {
		points = 0
	}
	result, err := r.db.ExecContext(ctx, `update video_render_tasks set
		released_points=released_points+$2,updated_at=now()
		where id=$1 and captured_points=0 and released_points+ $2 <= reserved_points`, taskID, points)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		// already released or captured; treat as idempotent success
		return nil
	}
	return nil
}

func firstNonEmptyLocal(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func maxInt(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
}
