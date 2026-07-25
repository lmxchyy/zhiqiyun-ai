package smartvideo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

const analysisTaskColumns = `id,project_id,asset_id,tenant_id,user_id,source_file_id,source_fingerprint,
	client_request_id,status,attempt_count,max_attempts,run_after,coalesce(lease_owner,''),lease_expires_at,
	heartbeat_at,coalesce(analyzer_version,''),coalesce(error_code,''),coalesce(sanitized_error_message,''),
	created_at,updated_at,started_at,finished_at`

func scanAnalysisTask(scanner interface{ Scan(...any) error }) (AnalysisTask, error) {
	var task AnalysisTask
	err := scanner.Scan(
		&task.ID, &task.ProjectID, &task.AssetID, &task.TenantID, &task.UserID, &task.SourceFileID,
		&task.SourceFingerprint, &task.ClientRequestID, &task.Status, &task.AttemptCount, &task.MaxAttempts,
		&task.RunAfter, &task.LeaseOwner, &task.LeaseExpiresAt, &task.HeartbeatAt, &task.AnalyzerVersion,
		&task.ErrorCode, &task.SanitizedErrorMessage, &task.CreatedAt, &task.UpdatedAt, &task.StartedAt, &task.FinishedAt,
	)
	return task, err
}

func (r *PostgresRepository) EnsureAnalysisTask(ctx context.Context, access Access, asset ProjectAsset, fingerprint, clientRequestID string, maxAttempts int) (AnalysisTask, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return AnalysisTask{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var owned bool
	if err := tx.QueryRowContext(ctx, `select exists(
		select 1 from video_project_assets where id=$1 and project_id=$2 and tenant_id=$3 and user_id=$4
	)`, asset.ID, asset.ProjectID, access.TenantID, access.UserID).Scan(&owned); err != nil {
		return AnalysisTask{}, err
	}
	if !owned {
		return AnalysisTask{}, ErrNotFound
	}
	now := time.Now().UTC()
	taskID := newID("vat")
	var storedID string
	err = tx.QueryRowContext(ctx, `insert into video_asset_analysis_tasks
		(id,project_id,asset_id,tenant_id,user_id,source_file_id,source_fingerprint,client_request_id,
		status,attempt_count,max_attempts,run_after,created_at,updated_at)
		values($1,$2,$3,$4,$5,$6,$7,$8,'PENDING',0,$9,$10,$10,$10)
		on conflict(asset_id,source_fingerprint) do update
		set client_request_id=case when video_asset_analysis_tasks.client_request_id='' then excluded.client_request_id else video_asset_analysis_tasks.client_request_id end
		returning id`,
		taskID, asset.ProjectID, asset.ID, access.TenantID, access.UserID, asset.FileID, fingerprint,
		clientRequestID, maxAttempts, now).Scan(&storedID)
	if err != nil {
		return AnalysisTask{}, err
	}
	if storedID == taskID {
		if _, err := tx.ExecContext(ctx, `update video_project_assets set
			analysis_status='PENDING',source_fingerprint=$1,normalized_metadata=null,filtered_probe_result=null,
			thumbnail_file_id=null,proxy_file_id=null,attempt_count=0,error_code=null,sanitized_error_message=null,
			analyzer_version=null,analysis_started_at=null,analysis_finished_at=null,updated_at=$2
			where id=$3 and tenant_id=$4 and user_id=$5`,
			fingerprint, now, asset.ID, access.TenantID, access.UserID); err != nil {
			return AnalysisTask{}, err
		}
	}
	task, err := scanAnalysisTask(tx.QueryRowContext(ctx, `select `+analysisTaskColumns+`
		from video_asset_analysis_tasks where id=$1`, storedID))
	if err != nil {
		return AnalysisTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return AnalysisTask{}, err
	}
	return task, nil
}

func (r *PostgresRepository) GetAnalysisTask(ctx context.Context, id string) (AnalysisTask, error) {
	task, err := scanAnalysisTask(r.db.QueryRowContext(ctx, `select `+analysisTaskColumns+`
		from video_asset_analysis_tasks where id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return AnalysisTask{}, ErrNotFound
	}
	return task, err
}

func (r *PostgresRepository) ListAnalysisTasks(ctx context.Context, access Access, projectID string) ([]AnalysisTask, error) {
	rows, err := r.db.QueryContext(ctx, `select `+analysisTaskColumns+` from video_asset_analysis_tasks
		where project_id=$1 and tenant_id=$2 and user_id=$3 order by created_at`,
		projectID, access.TenantID, access.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AnalysisTask{}
	for rows.Next() {
		item, err := scanAnalysisTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) MarkAnalysisQueued(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `with changed as (
		update video_asset_analysis_tasks set status='QUEUED',updated_at=now()
		where id=$1 and status='PENDING' returning asset_id
	)
	update video_project_assets set analysis_status='QUEUED',updated_at=now()
	where id in (select asset_id from changed)`, id)
	return err
}

func (r *PostgresRepository) AcquireAnalysisTask(ctx context.Context, id, workerID string, lease time.Duration) (AnalysisTask, ProjectAsset, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return AnalysisTask{}, ProjectAsset{}, err
	}
	defer func() { _ = tx.Rollback() }()
	leaseSeconds := int64(lease.Seconds())
	var exhaustedAssetID string
	err = tx.QueryRowContext(ctx, `update video_asset_analysis_tasks set
		status='FAILED',error_code=$2,sanitized_error_message=$3,lease_owner=null,
		lease_expires_at=null,heartbeat_at=null,finished_at=now(),updated_at=now()
		where id=$1 and status='RUNNING' and lease_expires_at < now()
		and attempt_count >= max_attempts returning asset_id`,
		id, MediaErrorLeaseExpired, "分析任务租约已过期，且重试次数已用尽").Scan(&exhaustedAssetID)
	if err == nil {
		if _, err := tx.ExecContext(ctx, `update video_project_assets set
			analysis_status='FAILED',error_code=$1,sanitized_error_message=$2,
			analysis_finished_at=now(),updated_at=now() where id=$3`,
			MediaErrorLeaseExpired, "分析任务租约已过期，且重试次数已用尽", exhaustedAssetID); err != nil {
			return AnalysisTask{}, ProjectAsset{}, err
		}
		if err := tx.Commit(); err != nil {
			return AnalysisTask{}, ProjectAsset{}, err
		}
		return AnalysisTask{}, ProjectAsset{}, ErrAnalysisNotReady
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return AnalysisTask{}, ProjectAsset{}, err
	}
	task, err := scanAnalysisTask(tx.QueryRowContext(ctx, `update video_asset_analysis_tasks set
		status='RUNNING',attempt_count=attempt_count+1,lease_owner=$2,
		lease_expires_at=now()+($3 * interval '1 second'),heartbeat_at=now(),
		started_at=coalesce(started_at,now()),updated_at=now()
		where id=$1 and attempt_count < max_attempts and run_after <= now()
		and (status in ('PENDING','QUEUED') or (status='RUNNING' and lease_expires_at < now()))
		returning `+analysisTaskColumns, id, workerID, leaseSeconds))
	if errors.Is(err, sql.ErrNoRows) {
		return AnalysisTask{}, ProjectAsset{}, ErrAnalysisNotReady
	}
	if err != nil {
		return AnalysisTask{}, ProjectAsset{}, err
	}
	if _, err := tx.ExecContext(ctx, `update video_project_assets set analysis_status='RUNNING',
		attempt_count=$1,analysis_started_at=coalesce(analysis_started_at,now()),updated_at=now()
		where id=$2 and tenant_id=$3 and user_id=$4`, task.AttemptCount, task.AssetID, task.TenantID, task.UserID); err != nil {
		return AnalysisTask{}, ProjectAsset{}, err
	}
	asset, err := scanAsset(tx.QueryRowContext(ctx, `select `+assetColumns+` from video_project_assets where id=$1`, task.AssetID))
	if err != nil {
		return AnalysisTask{}, ProjectAsset{}, err
	}
	if err := tx.Commit(); err != nil {
		return AnalysisTask{}, ProjectAsset{}, err
	}
	return task, asset, nil
}

func (r *PostgresRepository) HeartbeatAnalysisTask(ctx context.Context, id, workerID string, lease time.Duration) error {
	result, err := r.db.ExecContext(ctx, `update video_asset_analysis_tasks set
		heartbeat_at=now(),lease_expires_at=now()+($3 * interval '1 second'),updated_at=now()
		where id=$1 and status='RUNNING' and lease_owner=$2`, id, workerID, int64(lease.Seconds()))
	if err == nil {
		if affected, _ := result.RowsAffected(); affected == 0 {
			err = ErrAnalysisLeaseLost
		}
	}
	return err
}

func (r *PostgresRepository) CompleteAnalysisTask(ctx context.Context, id, workerID string, result AnalysisResult) error {
	metadata, err := json.Marshal(result.Metadata)
	if err != nil {
		return err
	}
	filtered, err := json.Marshal(result.FilteredProbeResult)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var assetID string
	err = tx.QueryRowContext(ctx, `update video_asset_analysis_tasks set status='SUCCEEDED',
		analyzer_version=$3,error_code=null,sanitized_error_message=null,lease_owner=null,
		lease_expires_at=null,heartbeat_at=null,finished_at=now(),updated_at=now()
		where id=$1 and status='RUNNING' and lease_owner=$2 returning asset_id`,
		id, workerID, result.AnalyzerVersion).Scan(&assetID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAnalysisLeaseLost
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `update video_project_assets set analysis_status='SUCCEEDED',
		normalized_metadata=$1,filtered_probe_result=$2,thumbnail_file_id=nullif($3,''),
		proxy_file_id=nullif($4,''),analyzer_version=$5,error_code=null,sanitized_error_message=null,
		analysis_finished_at=now(),updated_at=now() where id=$6`,
		metadata, filtered, result.ThumbnailFileID, result.ProxyFileID, result.AnalyzerVersion, assetID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresRepository) FailAnalysisTask(ctx context.Context, id, workerID, code, message string, retryAt time.Time, final bool) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var assetID, status string
	err = tx.QueryRowContext(ctx, `update video_asset_analysis_tasks set
		status=case when $6 or attempt_count>=max_attempts then 'FAILED' else 'QUEUED' end,
		error_code=$3,sanitized_error_message=$4,run_after=$5,lease_owner=null,lease_expires_at=null,
		heartbeat_at=null,finished_at=case when $6 or attempt_count>=max_attempts then now() else null end,
		updated_at=now()
		where id=$1 and status='RUNNING' and lease_owner=$2 returning asset_id,status`,
		id, workerID, code, message, retryAt, final).Scan(&assetID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAnalysisLeaseLost
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `update video_project_assets set analysis_status=$1,
		attempt_count=(select attempt_count from video_asset_analysis_tasks where id=$2),
		error_code=$3,sanitized_error_message=$4,
		analysis_finished_at=case when $1='FAILED' then now() else null end,updated_at=now()
		where id=$5`, status, id, code, message, assetID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresRepository) RetryAnalysisTask(ctx context.Context, access Access, projectID, assetID string) (AnalysisTask, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return AnalysisTask{}, err
	}
	defer func() { _ = tx.Rollback() }()
	task, err := scanAnalysisTask(tx.QueryRowContext(ctx, `update video_asset_analysis_tasks set
		status='PENDING',attempt_count=0,run_after=now(),lease_owner=null,lease_expires_at=null,
		heartbeat_at=null,error_code=null,sanitized_error_message=null,started_at=null,finished_at=null,updated_at=now()
		where project_id=$1 and asset_id=$2 and tenant_id=$3 and user_id=$4 and status='FAILED'
		returning `+analysisTaskColumns, projectID, assetID, access.TenantID, access.UserID))
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		if queryErr := tx.QueryRowContext(ctx, `select exists(select 1 from video_asset_analysis_tasks
			where project_id=$1 and asset_id=$2 and tenant_id=$3 and user_id=$4)`,
			projectID, assetID, access.TenantID, access.UserID).Scan(&exists); queryErr != nil {
			return AnalysisTask{}, queryErr
		}
		if exists {
			return AnalysisTask{}, ErrAnalysisNotFailed
		}
		return AnalysisTask{}, ErrNotFound
	}
	if err != nil {
		return AnalysisTask{}, err
	}
	if _, err := tx.ExecContext(ctx, `update video_project_assets set analysis_status='PENDING',
		attempt_count=0,error_code=null,sanitized_error_message=null,analysis_started_at=null,
		analysis_finished_at=null,updated_at=now()
		where id=$1 and tenant_id=$2 and user_id=$3`, assetID, access.TenantID, access.UserID); err != nil {
		return AnalysisTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return AnalysisTask{}, err
	}
	return task, nil
}
