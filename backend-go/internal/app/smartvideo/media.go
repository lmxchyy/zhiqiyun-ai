package smartvideo

import (
	"context"
	"errors"
	"time"
)

const (
	AnalysisStatusPending   = "PENDING"
	AnalysisStatusQueued    = "QUEUED"
	AnalysisStatusRunning   = "RUNNING"
	AnalysisStatusSucceeded = "SUCCEEDED"
	AnalysisStatusFailed    = "FAILED"

	MediaErrorToolMissing      = "SMARTVIDEO_MEDIA_TOOL_MISSING"
	MediaErrorTimeout          = "SMARTVIDEO_MEDIA_TIMEOUT"
	MediaErrorProbeFailed      = "SMARTVIDEO_MEDIA_PROBE_FAILED"
	MediaErrorInvalidJSON      = "SMARTVIDEO_MEDIA_INVALID_PROBE_JSON"
	MediaErrorNoVideoStream    = "SMARTVIDEO_MEDIA_NO_VIDEO_STREAM"
	MediaErrorUnsupported      = "SMARTVIDEO_MEDIA_UNSUPPORTED"
	MediaErrorFileTooLarge     = "SMARTVIDEO_MEDIA_FILE_TOO_LARGE"
	MediaErrorDurationExceeded = "SMARTVIDEO_MEDIA_DURATION_EXCEEDED"
	MediaErrorPixelsExceeded   = "SMARTVIDEO_MEDIA_PIXELS_EXCEEDED"
	MediaErrorDownloadFailed   = "SMARTVIDEO_MEDIA_DOWNLOAD_FAILED"
	MediaErrorPreprocessFailed = "SMARTVIDEO_MEDIA_PREPROCESS_FAILED"
	MediaErrorStorageFailed    = "SMARTVIDEO_MEDIA_STORAGE_FAILED"
	MediaErrorLeaseLost        = "SMARTVIDEO_ANALYSIS_LEASE_LOST"
	MediaErrorLeaseExpired     = "SMARTVIDEO_ANALYSIS_LEASE_EXPIRED"
)

var (
	ErrAnalysisDisabled  = errors.New("SMART_VIDEO_ANALYSIS_DISABLED")
	ErrAnalysisNotReady  = errors.New("SMART_VIDEO_ANALYSIS_NOT_READY")
	ErrAnalysisNotFailed = errors.New("SMART_VIDEO_ANALYSIS_NOT_FAILED")
	ErrAnalysisLeaseLost = errors.New(MediaErrorLeaseLost)
)

func ValidateAnalysisTransition(from, to string) error {
	allowed := map[string]map[string]bool{
		AnalysisStatusPending: {
			AnalysisStatusQueued:  true,
			AnalysisStatusRunning: true,
		},
		AnalysisStatusQueued: {
			AnalysisStatusRunning: true,
		},
		AnalysisStatusRunning: {
			AnalysisStatusQueued:    true,
			AnalysisStatusSucceeded: true,
			AnalysisStatusFailed:    true,
		},
		AnalysisStatusFailed: {
			AnalysisStatusPending: true,
		},
	}
	if !allowed[from][to] {
		return ErrInvalidStateTransition
	}
	return nil
}

type ContainerMetadata struct {
	Title        string `json:"title,omitempty"`
	Encoder      string `json:"encoder,omitempty"`
	CreationTime string `json:"creationTime,omitempty"`
	MajorBrand   string `json:"majorBrand,omitempty"`
	Compatible   string `json:"compatibleBrands,omitempty"`
}

type VideoMetadata struct {
	Format          string            `json:"format"`
	MIMEType        string            `json:"mimeType"`
	DurationMS      int64             `json:"durationMs"`
	Width           int               `json:"width"`
	Height          int               `json:"height"`
	DisplayWidth    int               `json:"displayWidth"`
	DisplayHeight   int               `json:"displayHeight"`
	Rotation        int               `json:"rotation"`
	FPSNumerator    int64             `json:"fpsNumerator"`
	FPSDenominator  int64             `json:"fpsDenominator"`
	VideoCodec      string            `json:"videoCodec"`
	PixelFormat     string            `json:"pixelFormat"`
	Bitrate         int64             `json:"bitrate"`
	HasAudio        bool              `json:"hasAudio"`
	AudioCodec      string            `json:"audioCodec,omitempty"`
	AudioSampleRate int               `json:"audioSampleRate,omitempty"`
	AudioChannels   int               `json:"audioChannels,omitempty"`
	Container       ContainerMetadata `json:"containerMetadata"`
	ProbeVersion    string            `json:"probeVersion"`
}

type ImageMetadata struct {
	Format       string `json:"format"`
	MIMEType     string `json:"mimeType"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Orientation  int    `json:"orientation"`
	Animated     bool   `json:"animated"`
	FrameCount   int    `json:"frameCount"`
	ColorSpace   string `json:"colorSpace"`
	ProbeVersion string `json:"probeVersion"`
}

type NormalizedMediaMetadata struct {
	Kind  string         `json:"kind"`
	Video *VideoMetadata `json:"video,omitempty"`
	Image *ImageMetadata `json:"image,omitempty"`
}

type FilteredProbeResult struct {
	FormatName     string   `json:"formatName,omitempty"`
	FormatLongName string   `json:"formatLongName,omitempty"`
	StreamCodecs   []string `json:"streamCodecs,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
}

type LocalMedia struct {
	Path      string
	AssetType string
}

type ThumbnailOptions struct {
	MaxWidth  int
	MaxHeight int
	Quality   int
}

type ProxyOptions struct {
	MaxWidth     int
	VideoBitrate string
	AudioBitrate string
}

type MediaProbe interface {
	ProbeVideo(context.Context, LocalMedia) (VideoMetadata, FilteredProbeResult, error)
	ProbeImage(context.Context, LocalMedia) (ImageMetadata, FilteredProbeResult, error)
	GenerateThumbnail(context.Context, LocalMedia, string, ThumbnailOptions) error
	GenerateProxy(context.Context, LocalMedia, string, ProxyOptions) error
	GetToolVersion(context.Context) (string, string, error)
}

type AnalysisTask struct {
	ID                    string     `json:"id"`
	ProjectID             string     `json:"projectId"`
	AssetID               string     `json:"assetId"`
	TenantID              string     `json:"tenantId"`
	UserID                string     `json:"userId"`
	SourceFileID          string     `json:"sourceFileId"`
	SourceFingerprint     string     `json:"sourceFingerprint"`
	ClientRequestID       string     `json:"clientRequestId,omitempty"`
	Status                string     `json:"status"`
	AttemptCount          int        `json:"attemptCount"`
	MaxAttempts           int        `json:"maxAttempts"`
	RunAfter              time.Time  `json:"runAfter"`
	LeaseOwner            string     `json:"-"`
	LeaseExpiresAt        *time.Time `json:"-"`
	HeartbeatAt           *time.Time `json:"-"`
	AnalyzerVersion       string     `json:"analyzerVersion,omitempty"`
	ErrorCode             string     `json:"errorCode,omitempty"`
	SanitizedErrorMessage string     `json:"errorMessage,omitempty"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
	StartedAt             *time.Time `json:"startedAt,omitempty"`
	FinishedAt            *time.Time `json:"finishedAt,omitempty"`
}

type AnalysisResult struct {
	Metadata            NormalizedMediaMetadata `json:"metadata"`
	FilteredProbeResult FilteredProbeResult     `json:"filteredProbeResult"`
	ThumbnailFileID     string                  `json:"thumbnailFileId,omitempty"`
	ProxyFileID         string                  `json:"proxyFileId,omitempty"`
	AnalyzerVersion     string                  `json:"analyzerVersion"`
}

type AnalysisAssetStatus struct {
	AssetID         string                   `json:"assetId"`
	FileID          string                   `json:"fileId"`
	AssetType       string                   `json:"assetType"`
	Status          string                   `json:"status"`
	AttemptCount    int                      `json:"attemptCount"`
	Metadata        *NormalizedMediaMetadata `json:"metadata,omitempty"`
	ThumbnailFileID string                   `json:"thumbnailFileId,omitempty"`
	ProxyFileID     string                   `json:"proxyFileId,omitempty"`
	ErrorCode       string                   `json:"errorCode,omitempty"`
	ErrorMessage    string                   `json:"errorMessage,omitempty"`
	AnalyzerVersion string                   `json:"analyzerVersion,omitempty"`
	StartedAt       *time.Time               `json:"startedAt,omitempty"`
	FinishedAt      *time.Time               `json:"finishedAt,omitempty"`
}

type AnalysisSummary struct {
	ProjectID      string                `json:"projectId"`
	OverallStatus  string                `json:"overallStatus"`
	TotalAssets    int                   `json:"totalAssets"`
	PendingCount   int                   `json:"pendingCount"`
	RunningCount   int                   `json:"runningCount"`
	SucceededCount int                   `json:"succeededCount"`
	FailedCount    int                   `json:"failedCount"`
	Assets         []AnalysisAssetStatus `json:"assets"`
}

type AnalysisJob struct {
	TaskID    string `json:"taskId"`
	ProjectID string `json:"projectId"`
	AssetID   string `json:"assetId"`
}

type AnalysisQueue interface {
	Enqueue(context.Context, AnalysisJob, time.Duration) error
}

type QueueDecision struct {
	RetryAfter time.Duration
	Dead       bool
}

type AnalysisWorkerQueue interface {
	AnalysisQueue
	Run(context.Context, func(context.Context, AnalysisJob) QueueDecision) error
}

type AnalysisTaskProcessor interface {
	Process(context.Context, AnalysisTask, ProjectAsset) (AnalysisResult, error)
}

type MediaError struct {
	Code    string
	Message string
	Cause   error
}

func (e *MediaError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code + ": " + e.Message
}

func (e *MediaError) Unwrap() error { return e.Cause }
