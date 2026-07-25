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
 status,progress,step,attempt_count,max_attempts,run_after,specification,
 quoted_tokens,reserved_tokens,captured_tokens,released_tokens,
 coalesce(output_file_id,''),coalesce(cover_file_id,''),coalesce(output_asset_id,''),
 output_metadata,coalesce(error_code,''),coalesce(error_message,''),
 created_at,updated_at,started_at,finished_at`

func scanExtendedRenderTask(scanner interface{ Scan(...any) error }) (RenderTask, error) {
	var task RenderTask
	var specification, output []byte
	err := scanner.Scan(
		&task.ID, &task.ProjectID, &task.VersionID, &task.TenantID, &task.UserID, &task.ClientRequestID,
		&task.Status, &task.Progress, &task.Step, &task.AttemptCount, &task.MaxAttempts, &task.RunAfter, &specification,
		&task.QuotedTokens, &task.ReservedTokens, &task.CapturedTokens, &task.ReleasedTokens,
		&task.OutputFileID, &task.CoverFileID, &task.OutputAssetID, &output, &task.ErrorCode, &task.ErrorMessage,
		&task.CreatedAt, &task.UpdatedAt, &task.StartedAt, &task.FinishedAt,
	)
	if err == nil {
		err = json.Unmarshal(specification, &task.Specification)
	}
	if err == nil && len(output) > 0 {
		err = json.Unmarshal(output, &task.Output)
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
	result, err := r.db.ExecContext(ctx, `update video_render_tasks set status='QUEUED',step='queued',progress=5,run_after=now(),updated_at=now()
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
 status='PROCESSING',step='preparing',progress=10,attempt_count=attempt_count+1,
 lease_owner=$2,lease_expires_at=now()+($3 * interval '1 millisecond'),heartbeat_at=now(),
 started_at=coalesce(started_at,now()),updated_at=now(),error_code=null,error_message=null
 where id=$1 and attempt_count < max_attempts and run_after <= now()
 and (status='QUEUED' or (status in ('PROCESSING','RENDERING','UPLOADING') and lease_expires_at < now()))
 returning `+renderTaskColumns, taskID, workerID, lease.Milliseconds()))
	if errors.Is(err, sql.ErrNoRows) {
		return RenderTask{}, ErrNotFound
	}
	return task, err
}

func (r *PostgresRepository) HeartbeatRenderTask(ctx context.Context, taskID, workerID string, lease time.Duration) error {
	result, err := r.db.ExecContext(ctx, `update video_render_tasks set heartbeat_at=now(),
 lease_expires_at=now()+($3 * interval '1 millisecond'),updated_at=now()
 where id=$1 and lease_owner=$2 and status in ('PROCESSING','RENDERING','UPLOADING')`,
		taskID, workerID, lease.Milliseconds())
	if err == nil {
		if affected, _ := result.RowsAffected(); affected == 0 {
			err = ErrAnalysisLeaseLost
		}
	}
	return err
}

func (r *PostgresRepository) AdvanceRenderTask(ctx context.Context, taskID, workerID, from, to, step string, progress int) error {
	if err := ValidateRenderTransition(from, to); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `update video_render_tasks set status=$4,step=$5,progress=$6,updated_at=now()
 where id=$1 and lease_owner=$2 and status=$3`, taskID, workerID, from, to, step, progress)
	if err == nil {
		if affected, _ := result.RowsAffected(); affected == 0 {
			err = ErrInvalidStateTransition
		}
	}
	return err
}

func (r *PostgresRepository) CompleteRenderTask(ctx context.Context, taskID, workerID string, output RenderOutput) (RenderTask, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return RenderTask{}, err
	}
	defer tx.Rollback()
	var projectID, tenantID, userID, status string
	if err = tx.QueryRowContext(ctx, `select project_id,tenant_id,user_id,status from video_render_tasks where id=$1 for update`, taskID).
		Scan(&projectID, &tenantID, &userID, &status); err != nil {
		return RenderTask{}, err
	}
	if status == RenderStatusSucceeded {
		if err = tx.Commit(); err != nil {
			return RenderTask{}, err
		}
		return r.GetRenderTask(ctx, Access{TenantID: tenantID, UserID: userID}, projectID, taskID)
	}
	if status != RenderStatusUploading {
		return RenderTask{}, ErrInvalidStateTransition
	}
	assetID := "asset_" + taskID
	metadata := map[string]any{
		"source": "smart_video", "videoProjectId": projectID, "videoRenderTaskId": taskID,
		"fileId": output.VideoFileID, "thumbnailFileId": output.CoverFileID,
		"durationMs": output.DurationMS, "width": output.Width, "height": output.Height,
		"frameRate": output.FrameRate, "fileSize": output.FileSize, "videoCodec": output.VideoCodec,
		"audioCodec": output.AudioCodec, "pixelFormat": output.PixelFormat,
	}
	raw, _ := json.Marshal(metadata)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `insert into xz_assets
 (id,user_id,tenant_id,task_id,name,media_type,url,thumbnail_url,favorite,metadata,created_at,updated_at,raw)
 values($1,$2,$3,$4,'ZhiqiyunSmartVideoSmoke','video',$5,$6,false,$7::jsonb,$8,$8,$7::jsonb)
 on conflict(id) do nothing`, assetID, userID, tenantID, taskID,
		"storage://"+output.VideoFileID, "storage://"+output.CoverFileID, raw, now)
	if err != nil {
		return RenderTask{}, err
	}
	outputRaw, _ := json.Marshal(output)
	result, err := tx.ExecContext(ctx, `update video_render_tasks set status='SUCCEEDED',step='completed',progress=100,
 output_file_id=$3,cover_file_id=$4,output_asset_id=$5,output_metadata=$6::jsonb,
 lease_owner=null,lease_expires_at=null,heartbeat_at=now(),finished_at=now(),updated_at=now()
 where id=$1 and lease_owner=$2 and status='UPLOADING'`,
		taskID, workerID, output.VideoFileID, output.CoverFileID, assetID, outputRaw)
	if err != nil {
		return RenderTask{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return RenderTask{}, ErrAnalysisLeaseLost
	}
	_, err = tx.ExecContext(ctx, `update video_projects set status='COMPLETED',output_asset_id=$2,
 active_render_task_id=null,error_code=null,error_message=null,updated_at=now() where id=$1`, projectID, assetID)
	if err != nil {
		return RenderTask{}, err
	}
	if err = tx.Commit(); err != nil {
		return RenderTask{}, err
	}
	return r.GetRenderTask(ctx, Access{TenantID: tenantID, UserID: userID}, projectID, taskID)
}

func (r *PostgresRepository) FailRenderTask(ctx context.Context, taskID, workerID, code, message string, runAfter time.Time, retry bool) error {
	message = strings.TrimSpace(message)
	if len(message) > 500 {
		message = message[:500]
	}
	if retry {
		result, err := r.db.ExecContext(ctx, `update video_render_tasks set status='QUEUED',step='retry_wait',
 run_after=$3,lease_owner=null,lease_expires_at=null,error_code=$4,error_message=$5,updated_at=now()
 where id=$1 and lease_owner=$2 and attempt_count < max_attempts`, taskID, workerID, runAfter, code, message)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected > 0 {
			return nil
		}
	}
	_, err := r.db.ExecContext(ctx, `update video_render_tasks set status='FAILED',step='failed',
 lease_owner=null,lease_expires_at=null,error_code=$3,error_message=$4,finished_at=now(),updated_at=now()
 where id=$1 and ($2='' or lease_owner=$2) and status not in ('SUCCEEDED','CANCELLED')`, taskID, workerID, code, message)
	return err
}

func (r *PostgresRepository) RetryRenderTask(ctx context.Context, access Access, projectID, taskID string) (RenderTask, error) {
	result, err := r.db.ExecContext(ctx, `update video_render_tasks set status='QUEUED',step='queued',progress=5,
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
