package smartvideo

import (
	"context"
	"fmt"
	"strings"
)

// SettleService coordinates work-center publish and points capture/release.
// Capture is idempotent via task.CapturedPoints; publish is idempotent via task.WorkID.
type SettleService struct {
	repo   RenderRepository
	points PointsLifecycle
	works  WorkCenterPublisher
}

func NewSettleService(repo RenderRepository, points PointsLifecycle, works WorkCenterPublisher) *SettleService {
	return &SettleService{repo: repo, points: points, works: works}
}

func (s *SettleService) SettleSuccess(ctx context.Context, task RenderTask, output RenderOutput) (RenderTask, error) {
	if s == nil || s.repo == nil {
		return RenderTask{}, ErrSettleNotReady
	}
	access := Access{TenantID: task.TenantID, UserID: task.UserID}
	current, err := s.repo.GetRenderTask(ctx, access, task.ProjectID, task.ID)
	if err != nil {
		return RenderTask{}, err
	}
	if current.Status == RenderStatusSucceeded {
		return current, nil
	}

	if current.OutputFileID == "" {
		persisted, err := s.repo.PersistRenderOutput(ctx, current.ID, "", output)
		if err != nil {
			return RenderTask{}, err
		}
		current = persisted
	} else if current.Status == RenderStatusUploading {
		if err := s.repo.AdvanceRenderTask(ctx, current.ID, "", RenderStatusUploading, RenderStatusPublishing, "publishing", 95); err != nil {
			return RenderTask{}, err
		}
		current.Status = RenderStatusPublishing
	}

	if strings.TrimSpace(current.WorkID) == "" {
		if s.works == nil {
			return RenderTask{}, ErrSettleNotReady
		}
		workID, err := s.works.PublishPrivateWork(ctx, WorkPublishInput{
			Access: access,
			VideoFileID: firstNonEmpty(current.OutputFileID, output.VideoFileID),
			CoverFileID: firstNonEmpty(current.CoverFileID, output.CoverFileID),
			ProjectID: current.ProjectID, VersionID: current.VersionID, RenderTaskID: current.ID,
			DurationMs: output.DurationMS, Width: output.Width, Height: output.Height,
			FrameRate: output.FrameRate, FileSize: output.FileSize,
			VideoCodec: output.VideoCodec, AudioCodec: output.AudioCodec, PixelFormat: output.PixelFormat,
		})
		if err != nil {
			return RenderTask{}, fmt.Errorf("work publish: %w", err)
		}
		if err := s.repo.MarkRenderWorkPublished(ctx, current.ID, "", workID); err != nil {
			return RenderTask{}, err
		}
		current.WorkID = workID
		current.OutputAssetID = workID
	}

	if current.CapturedPoints == 0 && current.ReservedPoints > 0 {
		if s.points == nil {
			return RenderTask{}, ErrSettleNotReady
		}
		if err := s.points.Capture(ctx, access, current.ID); err != nil {
			return RenderTask{}, fmt.Errorf("points capture: %w", err)
		}
		if err := s.repo.MarkPointsCaptured(ctx, current.ID, current.ReservedPoints); err != nil {
			return RenderTask{}, err
		}
		current.CapturedPoints = current.ReservedPoints
	}

	output.VideoFileID = firstNonEmpty(output.VideoFileID, current.OutputFileID)
	output.CoverFileID = firstNonEmpty(output.CoverFileID, current.CoverFileID)
	return s.repo.CompleteRenderTask(ctx, current.ID, "", output)
}

func (s *SettleService) SettleFinalFailure(ctx context.Context, task RenderTask) error {
	if s == nil || s.repo == nil || s.points == nil {
		return nil
	}
	access := Access{TenantID: task.TenantID, UserID: task.UserID}
	current, err := s.repo.GetRenderTask(ctx, access, task.ProjectID, task.ID)
	if err != nil {
		return err
	}
	if current.CapturedPoints > 0 {
		return nil
	}
	releasable := current.ReservedPoints - current.ReleasedPoints
	if releasable <= 0 {
		return nil
	}
	if err := s.points.Release(ctx, access, current.ID, "failed"); err != nil {
		return err
	}
	return s.repo.MarkPointsReleased(ctx, current.ID, releasable)
}
