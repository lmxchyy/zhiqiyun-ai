package smartvideo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// PostgresWorkPublisher writes private works into xz_assets without going
// through the generation_tasks path. Metadata carries project/version/render ids.
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
		return existing, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	id := newID("asset")
	name := fmt.Sprintf("AI自动混剪-%s.mp4", input.RenderTaskID)
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
		"id": id, "userId": input.Access.UserID, "tenantId": input.Access.TenantID,
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
		id, input.Access.UserID, input.Access.TenantID, input.RenderTaskID, name,
		"storage://"+input.VideoFileID, coverRef(input.CoverFileID), metaRaw, nowText, raw)
	if err != nil {
		return "", err
	}
	return id, nil
}

func coverRef(coverFileID string) string {
	if coverFileID == "" {
		return ""
	}
	return "storage://" + coverFileID
}
