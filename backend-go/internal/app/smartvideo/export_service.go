package smartvideo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInsufficientPoints   = errors.New("SMART_VIDEO_INSUFFICIENT_POINTS")
	ErrQuoteExpired         = errors.New("SMART_VIDEO_QUOTE_EXPIRED")
	ErrRenderNotCancellable = errors.New("SMART_VIDEO_RENDER_NOT_CANCELLABLE")
	ErrExportNotReady       = errors.New("SMART_VIDEO_EXPORT_NOT_READY")
	ErrSettleNotReady       = errors.New("SMART_VIDEO_SETTLE_NOT_READY")
)

type ExportCreateInput struct {
	VersionID      string `json:"versionId"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type ExportRepository interface {
	CreateRenderTaskWithOutbox(context.Context, RenderTask, OutboxEvent) error
	GetRenderTask(context.Context, Access, string, string) (RenderTask, error)
	GetRenderTaskByClientRequestID(context.Context, Access, string) (RenderTask, error)
	CancelRenderTask(context.Context, Access, string, string) (RenderTask, error)
	MarkPointsReleased(context.Context, string, int64) error
	CreateRetryRenderTaskWithOutbox(context.Context, RenderTask, OutboxEvent) error
}

type ExportService struct {
	projects Repository
	versions VersionRepository
	exports  ExportRepository
	points   PointsLifecycle
	now      func() time.Time
}

func NewExportService(projects Repository, versions VersionRepository, exports ExportRepository, points PointsLifecycle) *ExportService {
	return &ExportService{
		projects: projects, versions: versions, exports: exports, points: points,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (s *ExportService) CreateExport(ctx context.Context, access Access, projectID string, input ExportCreateInput) (RenderTask, error) {
	if s == nil || s.projects == nil || s.versions == nil || s.exports == nil || s.points == nil {
		return RenderTask{}, ErrExportNotReady
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.VersionID = strings.TrimSpace(input.VersionID)
	projectID = strings.TrimSpace(projectID)
	if input.IdempotencyKey == "" {
		return RenderTask{}, ErrIdempotencyKeyRequired
	}
	if input.VersionID == "" {
		return RenderTask{}, ErrInvalidInput
	}
	if existing, err := s.exports.GetRenderTaskByClientRequestID(ctx, access, input.IdempotencyKey); err == nil {
		if existing.ProjectID != projectID || existing.VersionID != input.VersionID {
			return RenderTask{}, ErrInvalidInput
		}
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return RenderTask{}, err
	}

	project, err := s.projects.GetProject(ctx, access, projectID)
	if err != nil {
		return RenderTask{}, err
	}
	if project.Status != ProjectStatusConfirmed {
		return RenderTask{}, ErrProjectNotConfirmed
	}
	version, err := s.versions.GetVersion(ctx, access, projectID, input.VersionID)
	if err != nil {
		return RenderTask{}, err
	}
	if version.RenderManifest == nil || strings.TrimSpace(version.ManifestHash) == "" {
		return RenderTask{}, ErrProjectNotConfirmed
	}
	if project.ConfirmedVersionID != "" && project.ConfirmedVersionID != version.ID {
		return RenderTask{}, fmt.Errorf("%w: version is not the confirmed export target", ErrInvalidInput)
	}

	plan := version.PlanSnapshot
	quoteInput := RenderQuoteInput{
		AspectRatio: plan.Target.AspectRatio,
		Resolution:  plan.Target.Resolution,
		DurationMs:  plan.Target.DurationMs,
		Voice:       plan.Voice.Enabled,
	}
	quote, err := s.points.Quote(ctx, quoteInput)
	if err != nil {
		return RenderTask{}, err
	}
	now := s.now()
	if quote.ExpiresAt.Before(now) {
		return RenderTask{}, ErrQuoteExpired
	}

	spec := specificationFromManifest(*version.RenderManifest, plan.Target.DurationMs)
	task := RenderTask{
		ID: newID("svrender"), ProjectID: project.ID, VersionID: version.ID,
		TenantID: access.TenantID, UserID: access.UserID, ClientRequestID: input.IdempotencyKey,
		Status: RenderStatusCreated, Progress: 0, Step: "created", Stage: "created",
		Attempt: 1, MaxAttempts: 3, RunAfter: now, Specification: spec,
		QuotedPoints: quote.Points, ReservedPoints: quote.Points,
		ManifestHash: version.ManifestHash,
		CreatedAt: now, UpdatedAt: now,
	}
	txID, err := s.points.Reserve(ctx, access, task.ID, quote)
	if err != nil {
		if errors.Is(err, ErrInsufficientPoints) {
			return RenderTask{}, err
		}
		return RenderTask{}, err
	}
	task.BillingTransactionID = txID

	payload, _ := json.Marshal(map[string]string{"taskId": task.ID})
	err = s.exports.CreateRenderTaskWithOutbox(ctx, task, OutboxEvent{
		TenantID: access.TenantID, AggregateType: "render", AggregateID: task.ID,
		EventType: "enqueue_requested", Payload: payload,
	})
	if err != nil {
		_ = s.points.Release(ctx, access, task.ID, "create_failed")
		if errors.Is(err, ErrIdempotencyConflict) {
			return s.exports.GetRenderTaskByClientRequestID(ctx, access, input.IdempotencyKey)
		}
		return RenderTask{}, err
	}
	return task, nil
}

func (s *ExportService) CancelExport(ctx context.Context, access Access, projectID, taskID string) (RenderTask, error) {
	if s == nil || s.exports == nil || s.points == nil {
		return RenderTask{}, ErrExportNotReady
	}
	task, err := s.exports.GetRenderTask(ctx, access, strings.TrimSpace(projectID), strings.TrimSpace(taskID))
	if err != nil {
		return RenderTask{}, err
	}
	if !canCancelRender(task.Status) {
		return RenderTask{}, ErrRenderNotCancellable
	}
	cancelled, err := s.exports.CancelRenderTask(ctx, access, task.ProjectID, task.ID)
	if err != nil {
		return RenderTask{}, err
	}
	releasable := cancelled.ReservedPoints - cancelled.ReleasedPoints
	if releasable < 0 {
		releasable = 0
	}
	if releasable > 0 && cancelled.CapturedPoints == 0 {
		if err := s.points.Release(ctx, access, cancelled.ID, "cancelled"); err != nil {
			return RenderTask{}, err
		}
		if err := s.exports.MarkPointsReleased(ctx, cancelled.ID, releasable); err != nil {
			return RenderTask{}, err
		}
		cancelled.ReleasedPoints += releasable
	}
	return cancelled, nil
}

func (s *ExportService) RetryExport(ctx context.Context, access Access, projectID, taskID string) (RenderTask, error) {
	if s == nil || s.projects == nil || s.versions == nil || s.exports == nil || s.points == nil {
		return RenderTask{}, ErrExportNotReady
	}
	projectID = strings.TrimSpace(projectID)
	taskID = strings.TrimSpace(taskID)
	failed, err := s.exports.GetRenderTask(ctx, access, projectID, taskID)
	if err != nil {
		return RenderTask{}, err
	}
	if failed.Status != RenderStatusFailed {
		return RenderTask{}, fmt.Errorf("%w: only failed render tasks can be retried", ErrInvalidStateTransition)
	}
	project, err := s.projects.GetProject(ctx, access, projectID)
	if err != nil {
		return RenderTask{}, err
	}
	if project.Status != ProjectStatusConfirmed && project.Status != ProjectStatusFailed {
		// allow retry when project still confirmed or marked failed after render
		if project.Status != ProjectStatusRendering {
			return RenderTask{}, ErrProjectNotConfirmed
		}
	}
	version, err := s.versions.GetVersion(ctx, access, projectID, failed.VersionID)
	if err != nil {
		return RenderTask{}, err
	}
	if version.RenderManifest == nil || version.ManifestHash == "" {
		return RenderTask{}, ErrProjectNotConfirmed
	}

	plan := version.PlanSnapshot
	quote, err := s.points.Quote(ctx, RenderQuoteInput{
		AspectRatio: plan.Target.AspectRatio, Resolution: plan.Target.Resolution,
		DurationMs: plan.Target.DurationMs, Voice: plan.Voice.Enabled,
	})
	if err != nil {
		return RenderTask{}, err
	}
	now := s.now()
	retry := RenderTask{
		ID: newID("svrender"), ProjectID: failed.ProjectID, VersionID: failed.VersionID,
		TenantID: access.TenantID, UserID: access.UserID,
		ClientRequestID: failed.ClientRequestID + ":retry:" + now.Format("20060102150405.000"),
		Status: RenderStatusCreated, Progress: 0, Step: "created", Stage: "created",
		Attempt: 1, MaxAttempts: 3, RunAfter: now,
		Specification: specificationFromManifest(*version.RenderManifest, plan.Target.DurationMs),
		QuotedPoints: quote.Points, ReservedPoints: quote.Points,
		VoiceFileID: failed.VoiceFileID, CaptionFileID: failed.CaptionFileID,
		ManifestHash: version.ManifestHash, RetryOfTaskID: failed.ID,
		CreatedAt: now, UpdatedAt: now,
	}
	if txID, err := s.points.Reserve(ctx, access, retry.ID, quote); err != nil {
		return RenderTask{}, err
	} else {
		retry.BillingTransactionID = txID
	}
	payload, _ := json.Marshal(map[string]string{"taskId": retry.ID})
	if err := s.exports.CreateRetryRenderTaskWithOutbox(ctx, retry, OutboxEvent{
		TenantID: access.TenantID, AggregateType: "render", AggregateID: retry.ID,
		EventType: "enqueue_requested", Payload: payload,
	}); err != nil {
		_ = s.points.Release(ctx, access, retry.ID, "retry_create_failed")
		return RenderTask{}, err
	}
	return retry, nil
}

func canCancelRender(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case RenderStatusCreated, RenderStatusQueued, RenderStatusProcessing, RenderStatusSynthesizing:
		return true
	default:
		return false
	}
}

func specificationFromManifest(manifest RenderManifestV1, fallbackDurationMs int64) RenderSpecification {
	duration := manifestTimelineDurationMsLocal(manifest)
	if duration <= 0 {
		duration = fallbackDurationMs
	}
	return RenderSpecification{
		Width: manifest.Output.Width, Height: manifest.Output.Height,
		FrameRate: manifest.Output.FrameRate, Format: "mp4",
		VideoCodec: "h264", AudioCodec: "aac", DurationMS: duration,
	}
}

func manifestTimelineDurationMsLocal(manifest RenderManifestV1) int64 {
	var total int64
	for i, scene := range manifest.Scenes {
		total += scene.DurationMs
		if i < len(manifest.Scenes)-1 && scene.Transition.DurationMs > 0 &&
			scene.Transition.Type != "" && scene.Transition.Type != TransitionTypeCut {
			total -= scene.Transition.DurationMs
		}
	}
	return total
}
