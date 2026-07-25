package media

import (
	"context"
	"os"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type RenderPublisher struct {
	Storage        *storagecenter.Service
	MaxOutputBytes int64
}

func (p *RenderPublisher) Publish(ctx context.Context, task smartvideo.RenderTask, artifact smartvideo.RenderArtifact) (smartvideo.RenderOutput, error) {
	if p.Storage == nil {
		return smartvideo.RenderOutput{}, smartvideo.ErrFileNotReady
	}
	video, err := os.Open(artifact.VideoPath)
	if err != nil {
		return smartvideo.RenderOutput{}, err
	}
	defer video.Close()
	info, err := video.Stat()
	if err != nil {
		return smartvideo.RenderOutput{}, err
	}
	if info.Size() <= 0 || (p.MaxOutputBytes > 0 && info.Size() > p.MaxOutputBytes) {
		return smartvideo.RenderOutput{}, smartvideo.ErrInvalidInput
	}
	videoFile, err := p.Storage.StoreObject(ctx, storagecenter.UploadInitInput{
		TenantID: task.TenantID, UserID: task.UserID, FileName: "ZhiqiyunSmartVideoSmoke.mp4",
		FileSize: info.Size(), MIMEType: "video/mp4", BusinessType: "smart_video_result",
		BusinessID: task.ID, Visibility: "PRIVATE",
	}, video)
	if err != nil {
		return smartvideo.RenderOutput{}, err
	}
	cover, err := os.Open(artifact.CoverPath)
	if err != nil {
		return smartvideo.RenderOutput{}, err
	}
	defer cover.Close()
	coverInfo, err := cover.Stat()
	if err != nil {
		return smartvideo.RenderOutput{}, err
	}
	coverFile, err := p.Storage.StoreObject(ctx, storagecenter.UploadInitInput{
		TenantID: task.TenantID, UserID: task.UserID, FileName: "ZhiqiyunSmartVideoSmoke.jpg",
		FileSize: coverInfo.Size(), MIMEType: "image/jpeg", BusinessType: "smart_video_cover",
		BusinessID: task.ID, Visibility: "PRIVATE",
	}, cover)
	if err != nil {
		_ = p.Storage.PermanentDelete(ctx, storagecenter.AccessContext{TenantID: task.TenantID, UserID: task.UserID, IsAdmin: true}, videoFile.FileID)
		return smartvideo.RenderOutput{}, err
	}
	return smartvideo.RenderOutput{
		VideoFileID: videoFile.FileID, CoverFileID: coverFile.FileID,
		DurationMS: artifact.DurationMS, Width: artifact.Width, Height: artifact.Height,
		FrameRate: artifact.FrameRate, FileSize: artifact.FileSize, VideoCodec: artifact.VideoCodec,
		AudioCodec: artifact.AudioCodec, PixelFormat: artifact.PixelFormat,
	}, nil
}
