package smartvideo

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound               = errors.New("SMART_VIDEO_NOT_FOUND")
	ErrForbidden              = errors.New("SMART_VIDEO_FORBIDDEN")
	ErrInvalidInput           = errors.New("SMART_VIDEO_INVALID_INPUT")
	ErrInvalidStateTransition = errors.New("SMART_VIDEO_INVALID_STATE_TRANSITION")
	ErrProjectNotConfirmed    = errors.New("SMART_VIDEO_PROJECT_NOT_CONFIRMED")
	ErrFileNotReady           = errors.New("SMART_VIDEO_FILE_NOT_READY")
	ErrIdempotencyKeyRequired = errors.New("SMART_VIDEO_IDEMPOTENCY_KEY_REQUIRED")
)

type Repository interface {
	CreateProject(context.Context, Project) (Project, error)
	GetProject(context.Context, Access, string) (Project, error)
	ListProjects(context.Context, Access) ([]Project, error)
	UpdateProject(context.Context, Project) (Project, error)
	SoftDeleteProject(context.Context, Access, string) error

	CreateAsset(context.Context, ProjectAsset) (ProjectAsset, error)
	GetAsset(context.Context, Access, string, string) (ProjectAsset, error)
	ListAssets(context.Context, Access, string) ([]ProjectAsset, error)
	ReorderAssets(context.Context, Access, string, []string) ([]ProjectAsset, error)
	DeleteAsset(context.Context, Access, string, string) error

	CreateRenderTask(context.Context, RenderTask) (RenderTask, error)
	GetRenderTaskByClientRequestID(context.Context, Access, string) (RenderTask, error)

	EnsureAnalysisTask(context.Context, Access, ProjectAsset, string, string, int) (AnalysisTask, error)
	GetAnalysisTask(context.Context, string) (AnalysisTask, error)
	ListAnalysisTasks(context.Context, Access, string) ([]AnalysisTask, error)
	MarkAnalysisQueued(context.Context, string) error
	AcquireAnalysisTask(context.Context, string, string, time.Duration) (AnalysisTask, ProjectAsset, error)
	HeartbeatAnalysisTask(context.Context, string, string, time.Duration) error
	CompleteAnalysisTask(context.Context, string, string, AnalysisResult) error
	FailAnalysisTask(context.Context, string, string, string, string, time.Time, bool) error
	RetryAnalysisTask(context.Context, Access, string, string) (AnalysisTask, error)
}

type FileResolver interface {
	ResolveFile(context.Context, Access, string) (FileReference, error)
}

type MaterialAnalyzer interface {
	Analyze(context.Context, Project, []ProjectAsset) (ScriptSnapshot, []StoryboardScene, error)
}

type VideoRenderer interface {
	Render(context.Context, Project, ProjectVersion, []ProjectAsset, RenderSpecification) (outputFileID string, err error)
}

type WorkCenterPublisher interface {
	PublishVideo(context.Context, Access, string, string, string) (assetID string, err error)
}

type TokenLifecycle interface {
	Estimate(context.Context, Access, ProjectVersion, RenderSpecification) (int64, error)
	Reserve(context.Context, Access, string, int64, string) error
	Capture(context.Context, Access, string, int64, string) error
	Release(context.Context, Access, string, int64, string) error
}

type RenderJob struct {
	TaskID string `json:"taskId"`
}
type RenderRepository interface {
	GetRenderTask(context.Context, Access, string, string) (RenderTask, error)
	MarkRenderQueued(context.Context, string) error
	AcquireRenderTask(context.Context, string, string, time.Duration) (RenderTask, error)
	HeartbeatRenderTask(context.Context, string, string, time.Duration) error
	AdvanceRenderTask(context.Context, string, string, string, string, string, int) error
	CompleteRenderTask(context.Context, string, string, RenderOutput) (RenderTask, error)
	FailRenderTask(context.Context, string, string, string, string, time.Time, bool) error
	RetryRenderTask(context.Context, Access, string, string) (RenderTask, error)
}
type RenderQueue interface {
	Enqueue(context.Context, RenderJob, time.Duration) error
}
type RenderWorkerQueue interface {
	RenderQueue
	Run(context.Context, func(context.Context, RenderJob) QueueDecision) error
}
type RenderProcessor interface {
	Render(context.Context, RenderTask, string) (RenderArtifact, error)
}
type RenderOutputPublisher interface {
	Publish(context.Context, RenderTask, RenderArtifact) (RenderOutput, error)
}
