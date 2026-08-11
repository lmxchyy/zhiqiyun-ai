package smartvideo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type PlanRepository interface {
	CreatePlanTaskWithOutbox(ctx context.Context, task PlanTask, outbox OutboxEvent) error
	GetPlanTask(ctx context.Context, access Access, taskID string) (PlanTask, error)
	GetPlanTaskByIdempotencyKey(ctx context.Context, access Access, key string) (PlanTask, error)
	ClaimPlanTask(ctx context.Context, taskID, workerID string, lease time.Duration) (PlanTask, error)
	HeartbeatPlanTask(ctx context.Context, taskID, workerID string, lease time.Duration) error
	CompletePlanTask(ctx context.Context, taskID, workerID string, version ProjectVersion) error
	FailPlanTask(ctx context.Context, taskID, workerID, code, message string) error
}

type OutboxEvent struct {
	ID            int64
	TenantID      string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       []byte
	Attempts      int
}

func (r *PostgresRepository) CreatePlanTaskWithOutbox(ctx context.Context, task PlanTask, outbox OutboxEvent) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `insert into video_plan_tasks
		(id,tenant_id,project_id,user_id,state,instruction,source_version_id,model_key,
		attempt,progress,idempotency_key,created_at)
		values($1,$2,$3,$4,$5,$6,nullif($7,''),$8,$9,$10,$11,$12)
		on conflict(tenant_id,user_id,idempotency_key) do nothing`,
		task.ID, task.TenantID, task.ProjectID, task.UserID, task.State,
		task.Instruction, task.SourceVersionID, task.ModelKey,
		task.Attempt, task.Progress, task.IdempotencyKey, task.CreatedAt)
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
		status='PLANNING',active_plan_task_id=$2,error_stage=null,error_code=null,error_message=null,updated_at=now()
		where id=$1 and tenant_id=$3`, task.ProjectID, task.ID, task.TenantID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *PostgresRepository) GetPlanTask(ctx context.Context, access Access, taskID string) (PlanTask, error) {
	return scanPlanTask(r.db.QueryRowContext(ctx,
		`select id,tenant_id,project_id,user_id,state,instruction,
		coalesce(source_version_id,''),coalesce(output_version_id,''),coalesce(model_key,''),
		coalesce(provider_request_id,''),attempt,progress,plan_snapshot,
		coalesce(error_code,''),coalesce(error_message,''),idempotency_key,
		created_at,started_at,finished_at
		from video_plan_tasks where id=$1 and tenant_id=$2 and user_id=$3`,
		taskID, access.TenantID, access.UserID))
}

func (r *PostgresRepository) GetPlanTaskByIdempotencyKey(ctx context.Context, access Access, key string) (PlanTask, error) {
	return scanPlanTask(r.db.QueryRowContext(ctx,
		`select id,tenant_id,project_id,user_id,state,instruction,
		coalesce(source_version_id,''),coalesce(output_version_id,''),coalesce(model_key,''),
		coalesce(provider_request_id,''),attempt,progress,plan_snapshot,
		coalesce(error_code,''),coalesce(error_message,''),idempotency_key,
		created_at,started_at,finished_at
		from video_plan_tasks where tenant_id=$1 and user_id=$2 and idempotency_key=$3`,
		access.TenantID, access.UserID, key))
}

func (r *PostgresRepository) CountSuccessfulPlansToday(ctx context.Context, access Access) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `select count(*) from video_plan_tasks
		where tenant_id=$1 and user_id=$2 and state='SUCCEEDED'
		  and created_at >= date_trunc('day', now() at time zone 'utc')`,
		access.TenantID, access.UserID).Scan(&count)
	return count, err
}

func scanPlanTask(scanner interface{ Scan(...any) error }) (PlanTask, error) {
	var t PlanTask
	var planSnapshot []byte
	err := scanner.Scan(&t.ID, &t.TenantID, &t.ProjectID, &t.UserID, &t.State, &t.Instruction,
		&t.SourceVersionID, &t.OutputVersionID, &t.ModelKey, &t.ProviderRequestID,
		&t.Attempt, &t.Progress, &planSnapshot, &t.ErrorCode, &t.ErrorMessage, &t.IdempotencyKey,
		&t.CreatedAt, &t.StartedAt, &t.FinishedAt)
	if err == nil && len(planSnapshot) > 0 {
		err = json.Unmarshal(planSnapshot, &t.PlanSnapshot)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return PlanTask{}, ErrNotFound
	}
	return t, err
}

func (r *PostgresRepository) ClaimPlanTask(ctx context.Context, taskID, workerID string, lease time.Duration) (PlanTask, error) {
	leaseMs := lease.Milliseconds()
	task, err := scanPlanTask(r.db.QueryRowContext(ctx, `update video_plan_tasks set
		state='PROCESSING',attempt=attempt+1,lease_owner=$2,lease_expires_at=now()+($3 * interval '1 millisecond'),
		heartbeat_at=now(),started_at=coalesce(started_at,now()),error_code=null,error_message=null
		where id=$1 and attempt < 10 and
		(state in ('CREATED','QUEUED') or (state='PROCESSING' and lease_expires_at < now()))
		returning id,tenant_id,project_id,user_id,state,instruction,
		coalesce(source_version_id,''),coalesce(output_version_id,''),coalesce(model_key,''),
		coalesce(provider_request_id,''),attempt,progress,plan_snapshot,
		coalesce(error_code,''),coalesce(error_message,''),idempotency_key,
		created_at,started_at,finished_at`, taskID, workerID, leaseMs))
	if errors.Is(err, sql.ErrNoRows) {
		return PlanTask{}, ErrNotFound
	}
	return task, err
}

func (r *PostgresRepository) HeartbeatPlanTask(ctx context.Context, taskID, workerID string, lease time.Duration) error {
	leaseMs := lease.Milliseconds()
	result, err := r.db.ExecContext(ctx, `update video_plan_tasks set
		heartbeat_at=now(),lease_expires_at=now()+($3 * interval '1 millisecond')
		where id=$1 and lease_owner=$2 and state='PROCESSING'`, taskID, workerID, leaseMs)
	if err == nil {
		if affected, _ := result.RowsAffected(); affected == 0 {
			err = ErrAnalysisLeaseLost
		}
	}
	return err
}

func (r *PostgresRepository) CompletePlanTask(ctx context.Context, taskID, workerID string, version ProjectVersion) error {
	if version.Source == "" {
		version.Source = VersionSourceAI
	}
	if version.PlanSchemaVersion == 0 {
		version.PlanSchemaVersion = EditPlanSchemaVersion
	}
	planData, err := json.Marshal(version.PlanSnapshot)
	if err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `insert into video_project_versions
		(id,project_id,tenant_id,version_number,status,requirement,script,storyboard_snapshot,
		 source,parent_version_id,plan_schema_version,plan_snapshot,planner_model_key,planner_request_id,
		 change_note,created_by,created_at)
		values($1,$2,$3,$4,'GENERATED',coalesce($5,''),'{}'::jsonb,'[]'::jsonb,
		 $6,nullif($7,''),$8,$9::jsonb,nullif($10,''),nullif($11,''),
		 nullif($12,''),$13,$14)`,
		version.ID, version.ProjectID, version.TenantID, version.VersionNumber, version.Requirement,
		version.Source, version.ParentVersionID, version.PlanSchemaVersion, planData,
		version.PlannerModelKey, version.PlannerRequestID, version.ChangeNote,
		version.CreatedBy, version.CreatedAt)
	if err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx, `update video_plan_tasks set
		state='SUCCEEDED',output_version_id=$3,plan_snapshot=$4::jsonb,progress=100,
		lease_owner=null,lease_expires_at=null,finished_at=now()
		where id=$1 and lease_owner=$2 and state='PROCESSING'`,
		taskID, workerID, version.ID, planData)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrInvalidStateTransition
	}

	_, err = tx.ExecContext(ctx, `update video_projects set
		status='STORYBOARD_READY',current_version=$2,current_version_id=$3,
		active_plan_task_id=null,error_stage=null,error_code=null,error_message=null,updated_at=now()
		where id=$1 and tenant_id=$4`,
		version.ProjectID, version.VersionNumber, version.ID, version.TenantID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *PostgresRepository) FailPlanTask(ctx context.Context, taskID, workerID, code, message string) error {
	if len(message) > 500 {
		message = message[:500]
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var projectID, tenantID string
	err = tx.QueryRowContext(ctx, `update video_plan_tasks set
		state='FAILED',error_code=$3,error_message=$4,
		lease_owner=null,lease_expires_at=null,finished_at=now()
		where id=$1 and ($2='' or lease_owner=$2) and state not in ('SUCCEEDED')
		returning project_id,tenant_id`, taskID, workerID, code, message).Scan(&projectID, &tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidStateTransition
	}
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `update video_projects set
		status=case when coalesce(current_version_id,'')<>'' then 'STORYBOARD_READY' else 'MATERIAL_READY' end,
		active_plan_task_id=null,error_stage='planning',error_code=$2,error_message=$3,updated_at=now()
		where id=$1 and tenant_id=$4`, projectID, code, message, tenantID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresRepository) CreateImmutableVersion(ctx context.Context, version ProjectVersion) (ProjectVersion, error) {
	if version.Source == "" {
		version.Source = VersionSourceUser
	}
	if version.PlanSchemaVersion == 0 {
		version.PlanSchemaVersion = EditPlanSchemaVersion
	}
	planData, err := json.Marshal(version.PlanSnapshot)
	if err != nil {
		return ProjectVersion{}, err
	}
	_, err = r.db.ExecContext(ctx, `insert into video_project_versions
		(id,project_id,tenant_id,version_number,status,requirement,script,storyboard_snapshot,
		 source,parent_version_id,plan_schema_version,plan_snapshot,planner_model_key,planner_request_id,
		 change_note,created_by,created_at)
		values($1,$2,$3,$4,'GENERATED',coalesce($5,''),'{}'::jsonb,'[]'::jsonb,
		 $6,nullif($7,''),$8,$9::jsonb,nullif($10,''),nullif($11,''),
		 nullif($12,''),$13,$14)`,
		version.ID, version.ProjectID, version.TenantID, version.VersionNumber, version.Requirement,
		version.Source, version.ParentVersionID, version.PlanSchemaVersion, planData,
		version.PlannerModelKey, version.PlannerRequestID, version.ChangeNote,
		version.CreatedBy, version.CreatedAt)
	if err != nil {
		return ProjectVersion{}, err
	}
	return version, nil
}

func (r *PostgresRepository) GetVersion(ctx context.Context, access Access, projectID, versionID string) (ProjectVersion, error) {
	version, err := scanProjectVersion(r.db.QueryRowContext(ctx, `select
		id,project_id,tenant_id,version_number,coalesce(source,'ai'),coalesce(parent_version_id,''),
		coalesce(plan_schema_version,0),plan_snapshot,render_manifest,coalesce(manifest_hash,''),
		coalesce(planner_model_key,''),coalesce(planner_request_id,''),coalesce(change_note,''),
		coalesce(status,''),coalesce(requirement,''),created_by,created_at
		from video_project_versions
		where id=$1 and project_id=$2 and tenant_id=$3`, versionID, projectID, access.TenantID))
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectVersion{}, ErrNotFound
	}
	return version, err
}

func (r *PostgresRepository) ListVersions(ctx context.Context, access Access, projectID string) ([]ProjectVersion, error) {
	rows, err := r.db.QueryContext(ctx, `select
		id,project_id,tenant_id,version_number,coalesce(source,'ai'),coalesce(parent_version_id,''),
		coalesce(plan_schema_version,0),plan_snapshot,render_manifest,coalesce(manifest_hash,''),
		coalesce(planner_model_key,''),coalesce(planner_request_id,''),coalesce(change_note,''),
		coalesce(status,''),coalesce(requirement,''),created_by,created_at
		from video_project_versions
		where project_id=$1 and tenant_id=$2
		order by version_number desc`, projectID, access.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ProjectVersion{}
	for rows.Next() {
		item, err := scanProjectVersion(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) AttachRenderManifest(ctx context.Context, access Access, projectID, versionID string, manifest RenderManifestV1, hash string) (ProjectVersion, error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return ProjectVersion{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ProjectVersion{}, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `update video_project_versions set
		render_manifest=$4::jsonb,manifest_hash=$5,status='CONFIRMED'
		where id=$1 and project_id=$2 and tenant_id=$3
		  and (manifest_hash is null or manifest_hash='' or manifest_hash=$5)`,
		versionID, projectID, access.TenantID, raw, hash)
	if err != nil {
		return ProjectVersion{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		var existingHash sql.NullString
		_ = tx.QueryRowContext(ctx, `select manifest_hash from video_project_versions
			where id=$1 and project_id=$2 and tenant_id=$3`, versionID, projectID, access.TenantID).Scan(&existingHash)
		if existingHash.Valid && existingHash.String != "" && existingHash.String != hash {
			return ProjectVersion{}, ErrVersionImmutable
		}
		return ProjectVersion{}, ErrNotFound
	}

	_, err = tx.ExecContext(ctx, `update video_projects set
		status='CONFIRMED',confirmed_version_id=$2,updated_at=now()
		where id=$1 and tenant_id=$3`, projectID, versionID, access.TenantID)
	if err != nil {
		return ProjectVersion{}, err
	}
	if err = tx.Commit(); err != nil {
		return ProjectVersion{}, err
	}
	return r.GetVersion(ctx, access, projectID, versionID)
}

func scanProjectVersion(scanner interface{ Scan(...any) error }) (ProjectVersion, error) {
	var v ProjectVersion
	var planRaw, manifestRaw []byte
	err := scanner.Scan(
		&v.ID, &v.ProjectID, &v.TenantID, &v.VersionNumber, &v.Source, &v.ParentVersionID,
		&v.PlanSchemaVersion, &planRaw, &manifestRaw, &v.ManifestHash,
		&v.PlannerModelKey, &v.PlannerRequestID, &v.ChangeNote,
		&v.Status, &v.Requirement, &v.CreatedBy, &v.CreatedAt,
	)
	if err != nil {
		return ProjectVersion{}, err
	}
	if len(planRaw) > 0 {
		if err = json.Unmarshal(planRaw, &v.PlanSnapshot); err != nil {
			return ProjectVersion{}, err
		}
	}
	if len(manifestRaw) > 0 {
		var manifest RenderManifestV1
		if err = json.Unmarshal(manifestRaw, &manifest); err != nil {
			return ProjectVersion{}, err
		}
		v.RenderManifest = &manifest
	}
	return v, nil
}

func (r *PostgresRepository) PublishOutbox(ctx context.Context, limit int) ([]OutboxEvent, error) {
	rows, err := r.db.QueryContext(ctx, `update video_task_outbox set
		state='published',published_at=now(),attempts=attempts+1
		where id in (
			select id from video_task_outbox
			where state='pending' and available_at <= now()
			order by id limit $1
			for update skip locked
		)
		returning id,tenant_id,aggregate_type,aggregate_id,event_type,payload,attempts`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []OutboxEvent
	for rows.Next() {
		var e OutboxEvent
		var payload []byte
		if err := rows.Scan(&e.ID, &e.TenantID, &e.AggregateType, &e.AggregateID, &e.EventType, &payload, &e.Attempts); err != nil {
			return nil, err
		}
		e.Payload = payload
		events = append(events, e)
	}
	return events, rows.Err()
}

func (r *PostgresRepository) MarkOutboxFailed(ctx context.Context, eventID int64, errMsg string) error {
	if len(errMsg) > 500 {
		errMsg = errMsg[:500]
	}
	_, err := r.db.ExecContext(ctx, `update video_task_outbox set
		state='failed',last_error=$2 where id=$1`, eventID, errMsg)
	return err
}

func (r *PostgresRepository) RequeueOutbox(ctx context.Context, eventID int64, delay time.Duration, errMsg string) error {
	if len(errMsg) > 500 {
		errMsg = errMsg[:500]
	}
	_, err := r.db.ExecContext(ctx, `update video_task_outbox set
		state='pending',available_at=now()+($2 * interval '1 millisecond'),last_error=$3
		where id=$1`, eventID, delay.Milliseconds(), errMsg)
	return err
}
