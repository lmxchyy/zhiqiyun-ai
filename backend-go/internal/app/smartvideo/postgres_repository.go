package smartvideo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type PostgresRepository struct{ db *sql.DB }

func NewPostgresRepository(db *sql.DB) *PostgresRepository { return &PostgresRepository{db: db} }

func (r *PostgresRepository) CreateProject(ctx context.Context, p Project) (Project, error) {
	_, err := r.db.ExecContext(ctx, `insert into video_projects
		(id,tenant_id,user_id,title,requirement,status,current_version,created_at,updated_at)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		p.ID, p.TenantID, p.UserID, p.Title, p.Requirement, p.Status, p.CurrentVersion, p.CreatedAt, p.UpdatedAt)
	return p, err
}

func scanProject(scanner interface{ Scan(...any) error }) (Project, error) {
	var p Project
	var targetSpec []byte
	var currentVersionID, confirmedVersionID, activeAnalysisTaskID, activePlanTaskID, errorStage sql.NullString
	err := scanner.Scan(&p.ID, &p.TenantID, &p.UserID, &p.Title, &p.Requirement, &p.Status, &p.CurrentVersion,
		&targetSpec, &currentVersionID, &confirmedVersionID, &activeAnalysisTaskID, &activePlanTaskID,
		&errorStage, &p.OutputAssetID, &p.ActiveRenderTaskID, &p.ErrorCode, &p.ErrorMessage,
		&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt)
	if err == nil && len(targetSpec) > 0 {
		_ = json.Unmarshal(targetSpec, &p.TargetSpec)
	}
	if currentVersionID.Valid {
		p.CurrentVersionID = currentVersionID.String
	}
	if confirmedVersionID.Valid {
		p.ConfirmedVersionID = confirmedVersionID.String
	}
	if activeAnalysisTaskID.Valid {
		p.ActiveAnalysisTaskID = activeAnalysisTaskID.String
	}
	if activePlanTaskID.Valid {
		p.ActivePlanTaskID = activePlanTaskID.String
	}
	if errorStage.Valid {
		p.ErrorStage = errorStage.String
	}
	return p, err
}

const projectColumns = `id,tenant_id,user_id,title,requirement,status,current_version,
	target_spec,current_version_id,confirmed_version_id,active_analysis_task_id,active_plan_task_id,
	error_stage,coalesce(output_asset_id,''),coalesce(active_render_task_id,''),coalesce(error_code,''),coalesce(error_message,''),
	created_at,updated_at,deleted_at`

func (r *PostgresRepository) GetProject(ctx context.Context, access Access, id string) (Project, error) {
	p, err := scanProject(r.db.QueryRowContext(ctx, `select `+projectColumns+` from video_projects
		where id=$1 and tenant_id=$2 and user_id=$3 and deleted_at is null`, id, access.TenantID, access.UserID))
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return p, err
}

func (r *PostgresRepository) ListProjects(ctx context.Context, access Access) ([]Project, error) {
	rows, err := r.db.QueryContext(ctx, `select `+projectColumns+` from video_projects
		where tenant_id=$1 and user_id=$2 and deleted_at is null order by updated_at desc`, access.TenantID, access.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Project{}
	for rows.Next() {
		item, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) UpdateProject(ctx context.Context, p Project) (Project, error) {
	result, err := r.db.ExecContext(ctx, `update video_projects set title=$1,requirement=$2,status=$3,current_version=$4,
		output_asset_id=nullif($5,''),active_render_task_id=nullif($6,''),
		active_analysis_task_id=nullif($7,''),active_plan_task_id=nullif($8,''),
		error_stage=nullif($9,''),error_code=nullif($10,''),error_message=nullif($11,''),updated_at=$12
		where id=$13 and tenant_id=$14 and user_id=$15 and deleted_at is null`,
		p.Title, p.Requirement, p.Status, p.CurrentVersion, p.OutputAssetID, p.ActiveRenderTaskID,
		p.ActiveAnalysisTaskID, p.ActivePlanTaskID, p.ErrorStage, p.ErrorCode, p.ErrorMessage, p.UpdatedAt,
		p.ID, p.TenantID, p.UserID)
	if err == nil {
		if affected, _ := result.RowsAffected(); affected == 0 {
			err = ErrNotFound
		}
	}
	return p, err
}

func (r *PostgresRepository) SoftDeleteProject(ctx context.Context, access Access, id string) error {
	result, err := r.db.ExecContext(ctx, `update video_projects set deleted_at=now(),updated_at=now()
		where id=$1 and tenant_id=$2 and user_id=$3 and deleted_at is null`, id, access.TenantID, access.UserID)
	if err == nil {
		if affected, _ := result.RowsAffected(); affected == 0 {
			err = ErrNotFound
		}
	}
	return err
}

func (r *PostgresRepository) CreateAsset(ctx context.Context, a ProjectAsset) (ProjectAsset, error) {
	metadata, _ := json.Marshal(a.Metadata)
	err := r.db.QueryRowContext(ctx, `insert into video_project_assets
		(id,project_id,tenant_id,user_id,file_id,storage_key,asset_type,kind,sort_order,order_index,metadata,analysis_status,duration_ms,created_at,updated_at)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		on conflict(project_id,file_id) do update set sort_order=excluded.sort_order,order_index=excluded.order_index,updated_at=excluded.updated_at
		returning id`, a.ID, a.ProjectID, a.TenantID, a.UserID, a.FileID, a.StorageKey, a.AssetType,
		toAssetKind(a.AssetType), a.SortOrder, a.SortOrder, metadata, AnalysisStatusPending, int64(0), a.CreatedAt, a.UpdatedAt).Scan(&a.ID)
	return a, err
}

func toAssetKind(assetType string) string {
	if assetType == AssetTypeVideo {
		return "video"
	}
	return "image"
}

const assetColumns = `id,project_id,tenant_id,user_id,file_id,storage_key,asset_type,
	sort_order,order_index,kind,duration_ms,metadata,
	analysis_status,source_fingerprint,coalesce(normalized_metadata,'null'::jsonb),
	coalesce(filtered_probe_result,'null'::jsonb),coalesce(analysis_summary,'null'::jsonb),
	coalesce(representative_frame_file_ids,'null'::jsonb),content_audit_status,
	coalesce(thumbnail_file_id,''),coalesce(proxy_file_id,''),
	attempt_count,coalesce(error_code,''),coalesce(sanitized_error_message,''),coalesce(analyzer_version,''),
	analysis_started_at,analysis_finished_at,created_at,updated_at`

func scanAsset(scanner interface{ Scan(...any) error }) (ProjectAsset, error) {
	var a ProjectAsset
	var metadata, normalized, filtered, analysisSummary, repFrames []byte
	var kind string
	var durationMs sql.NullInt64
	// Must match assetColumns field count/order exactly (29 columns).
	err := scanner.Scan(
		&a.ID, &a.ProjectID, &a.TenantID, &a.UserID, &a.FileID, &a.StorageKey,
		&a.AssetType, &a.SortOrder, &a.OrderIndex, &kind, &durationMs, &metadata,
		&a.AnalysisStatus, &a.SourceFingerprint, &normalized, &filtered, &analysisSummary, &repFrames,
		&a.ContentAuditStatus,
		&a.ThumbnailFileID, &a.ProxyFileID,
		&a.AttemptCount, &a.ErrorCode, &a.SanitizedErrorMessage, &a.AnalyzerVersion,
		&a.AnalysisStartedAt, &a.AnalysisFinishedAt, &a.CreatedAt, &a.UpdatedAt,
	)
	_ = kind
	_ = analysisSummary
	_ = repFrames
	if err == nil {
		err = json.Unmarshal(metadata, &a.Metadata)
	}
	if err == nil && string(normalized) != "null" {
		a.NormalizedMetadata = &NormalizedMediaMetadata{}
		err = json.Unmarshal(normalized, a.NormalizedMetadata)
	}
	if err == nil && string(filtered) != "null" {
		a.FilteredProbeResult = &FilteredProbeResult{}
		err = json.Unmarshal(filtered, a.FilteredProbeResult)
	}
	if durationMs.Valid {
		a.DurationMS = durationMs.Int64
	}
	return a, err
}

func (r *PostgresRepository) GetAsset(ctx context.Context, access Access, projectID, assetID string) (ProjectAsset, error) {
	item, err := scanAsset(r.db.QueryRowContext(ctx, `select `+assetColumns+`
		from video_project_assets where id=$1 and project_id=$2 and tenant_id=$3 and user_id=$4`,
		assetID, projectID, access.TenantID, access.UserID))
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectAsset{}, ErrNotFound
	}
	return item, err
}

func (r *PostgresRepository) ListAssets(ctx context.Context, access Access, projectID string) ([]ProjectAsset, error) {
	rows, err := r.db.QueryContext(ctx, `select `+assetColumns+`
		from video_project_assets where project_id=$1 and tenant_id=$2 and user_id=$3 order by order_index,created_at`,
		projectID, access.TenantID, access.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ProjectAsset{}
	for rows.Next() {
		item, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) ReorderAssets(ctx context.Context, access Access, projectID string, ids []string) ([]ProjectAsset, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	for index, id := range ids {
		result, err := tx.ExecContext(ctx, `update video_project_assets set sort_order=$1,order_index=$1,updated_at=now()
			where id=$2 and project_id=$3 and tenant_id=$4 and user_id=$5`, index, id, projectID, access.TenantID, access.UserID)
		if err != nil {
			return nil, err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return nil, ErrInvalidInput
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.ListAssets(ctx, access, projectID)
}

func (r *PostgresRepository) DeleteAsset(ctx context.Context, access Access, projectID, assetID string) error {
	result, err := r.db.ExecContext(ctx, `delete from video_project_assets
		where id=$1 and project_id=$2 and tenant_id=$3 and user_id=$4`, assetID, projectID, access.TenantID, access.UserID)
	if err == nil {
		if affected, _ := result.RowsAffected(); affected == 0 {
			err = ErrNotFound
		}
	}
	return err
}

func (r *PostgresRepository) CreateRenderTask(ctx context.Context, t RenderTask) (RenderTask, error) {
	spec, _ := json.Marshal(t.Specification)
	err := r.db.QueryRowContext(ctx, `insert into video_render_tasks
		(id,project_id,version_id,tenant_id,user_id,client_request_id,status,progress,specification,
		quoted_tokens,reserved_tokens,captured_tokens,released_tokens,
		quoted_points,reserved_points,captured_points,released_points,
		attempt,created_at,updated_at)
		values($1,$2,nullif($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,1,$18,$19)
		on conflict(tenant_id,user_id,client_request_id) do update set updated_at=video_render_tasks.updated_at
		returning id`, t.ID, t.ProjectID, t.VersionID, t.TenantID, t.UserID, t.ClientRequestID, t.Status, t.Progress,
		spec, t.QuotedTokens, t.ReservedTokens, t.CapturedTokens, t.ReleasedTokens,
		t.QuotedPoints, t.ReservedPoints, t.CapturedPoints, t.ReleasedPoints,
		t.CreatedAt, t.UpdatedAt).Scan(&t.ID)
	if err != nil {
		return RenderTask{}, err
	}
	return r.GetRenderTaskByClientRequestID(ctx, Access{TenantID: t.TenantID, UserID: t.UserID}, t.ClientRequestID)
}

func (r *PostgresRepository) GetRenderTaskByClientRequestID(ctx context.Context, access Access, key string) (RenderTask, error) {
	t, err := scanExtendedRenderTask(r.db.QueryRowContext(ctx, `select `+renderTaskColumns+`
 from video_render_tasks where tenant_id=$1 and user_id=$2 and client_request_id=$3`,
		access.TenantID, access.UserID, key))
	if errors.Is(err, sql.ErrNoRows) {
		return RenderTask{}, ErrNotFound
	}
	return t, err
}
