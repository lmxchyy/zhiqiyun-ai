package smartvideo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// PostgresWorkPublisher writes private works into xz_assets and a matching
// xz_generation_tasks row so works center can discover montage exports both
// from assets and from recentTasks.
type PostgresWorkPublisher struct {
	DB *sql.DB
}

func (p *PostgresWorkPublisher) PublishVideo(context.Context, Access, string, string, string) (string, error) {
	return "", ErrSettleNotReady
}

func (p *PostgresWorkPublisher) PublishPrivateWork(ctx context.Context, input WorkPublishInput) (string, error) {
	if p == nil || p.DB == nil {
		return "", ErrSettleNotReady
	}
	if input.RenderTaskID == "" || input.VideoFileID == "" {
		return "", ErrInvalidInput
	}
	var existing string
	err := p.DB.QueryRowContext(ctx, `
		select id from xz_assets
		 where user_id=$1 and deleted_at is null
		   and coalesce(metadata->>'renderTaskId','')=$2
		 order by created_at desc limit 1`,
		input.Access.UserID, input.RenderTaskID).Scan(&existing)
	if err == nil && existing != "" {
		_ = p.ensureGenerationTask(ctx, input, existing)
		return existing, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	id := newID("asset")
	name := fmt.Sprintf("AI自动混剪-%s.mp4", input.RenderTaskID)
	tenantID := normalizeWorksTenantID(input.Access.TenantID)
	metadata := map[string]any{
		"type": "SMART_VIDEO_MONTAGE", "mediaType": "video",
		"fileId": input.VideoFileID, "storageFileId": input.VideoFileID,
		"coverFileId": input.CoverFileID, "projectId": input.ProjectID,
		"versionId": input.VersionID, "renderTaskId": input.RenderTaskID,
		"durationMs": input.DurationMs, "width": input.Width, "height": input.Height,
		"frameRate": input.FrameRate, "fileSize": input.FileSize,
		"videoCodec": input.VideoCodec, "audioCodec": input.AudioCodec,
		"pixelFormat": input.PixelFormat, "status": "COMPLETED",
	}
	metaRaw, _ := json.Marshal(metadata)
	raw, _ := json.Marshal(map[string]any{
		"id": id, "userId": input.Access.UserID, "tenantId": tenantID,
		"taskId": input.RenderTaskID, "name": name, "mediaType": "video",
		"url": "storage://" + input.VideoFileID, "thumbnailUrl": coverRef(input.CoverFileID),
		"favorite": false, "metadata": metadata,
		"createdAt": nowText, "updatedAt": nowText,
	})
	_, err = p.DB.ExecContext(ctx, `
		insert into xz_assets
		 (id,user_id,tenant_id,organization_id,task_id,name,media_type,url,thumbnail_url,favorite,metadata,deleted_at,created_at,updated_at,raw)
		 values ($1,$2,nullif($3,''),null,$4,$5,'video',$6,$7,false,$8::jsonb,null,$9,$9,$10::jsonb)
		 on conflict (id) do nothing`,
		id, input.Access.UserID, tenantID, input.RenderTaskID, name,
		"storage://"+input.VideoFileID, coverRef(input.CoverFileID), metaRaw, nowText, raw)
	if err != nil {
		return "", err
	}
	if err := p.ensureGenerationTask(ctx, input, id); err != nil {
		return "", err
	}
	return id, nil
}

func (p *PostgresWorkPublisher) ensureGenerationTask(ctx context.Context, input WorkPublishInput, assetID string) error {
	if p == nil || p.DB == nil || assetID == "" || input.RenderTaskID == "" {
		return nil
	}
	var existing string
	err := p.DB.QueryRowContext(ctx, `
		select id from xz_generation_tasks
		 where user_id=$1 and coalesce(client_request_id,'')=$2
		 limit 1`, input.Access.UserID, input.RenderTaskID).Scan(&existing)
	if err == nil && existing != "" {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	tenantID := normalizeWorksTenantID(input.Access.TenantID)
	taskID := "task_" + input.RenderTaskID
	name := fmt.Sprintf("AI自动混剪-%s.mp4", input.RenderTaskID)
	videoURL := "storage://" + input.VideoFileID
	thumbURL := coverRef(input.CoverFileID)
	params := map[string]any{
		"moduleCode": "smart_video_montage",
		"projectId":  input.ProjectID,
		"versionId":  input.VersionID,
		"renderTaskId": input.RenderTaskID,
		"mediaType":  "video",
		"width":      input.Width,
		"height":     input.Height,
		"durationMs": input.DurationMs,
		"fileId":     input.VideoFileID,
		"coverFileId": input.CoverFileID,
	}
	raw := map[string]any{
		"id": taskID, "userId": input.Access.UserID, "tenantId": tenantID,
		"clientRequestId": input.RenderTaskID, "moduleCode": "smart_video_montage",
		"type": "SMART_VIDEO_MONTAGE", "prompt": name, "model": "AI自动混剪",
		"status": "SUCCEEDED", "progress": 100, "pointCost": 0,
		"resultIds": []string{assetID},
		"imageUrl": videoURL, "outputUrl": videoURL, "resultUrl": videoURL, "thumbnailUrl": thumbURL,
		"mediaType": "video", "name": name, "params": params,
		"createdAt": now, "updatedAt": now, "workerFinishedAt": now,
	}
	paramsRaw, _ := json.Marshal(params)
	resultRaw, _ := json.Marshal([]string{assetID})
	rawJSON, _ := json.Marshal(raw)
	_, err = p.DB.ExecContext(ctx, `
		insert into xz_generation_tasks (
			id,user_id,tenant_id,organization_id,billing_account_type,billing_account_id,module_code,type,model,billing_type,
			status,progress,point_cost,prompt,params,result_ids,error,created_at,updated_at,worker_finished_at,
			client_request_id,task_status,billing_status,raw
		) values (
			$1,$2,nullif($3,''),null,'PERSONAL',null,'smart_video_montage','SMART_VIDEO_MONTAGE','AI自动混剪','POINTS',
			'SUCCEEDED',100,0,$4,$5::jsonb,$6::jsonb,'null'::jsonb,$7,$7,$7,
			$8,'SUCCEEDED','CAPTURED',$9::jsonb
		)
		on conflict (id) do nothing`,
		taskID, input.Access.UserID, tenantID, name, paramsRaw, resultRaw, now, input.RenderTaskID, rawJSON)
	return err
}

func normalizeWorksTenantID(tenantID string) string {
	if tenantID == "" {
		return "tenant_default"
	}
	return tenantID
}

func coverRef(coverFileID string) string {
	if coverFileID == "" {
		return ""
	}
	return "storage://" + coverFileID
}
