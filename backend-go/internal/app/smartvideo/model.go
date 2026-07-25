package smartvideo

import "time"

const (
	ProjectStatusDraft           = "DRAFT"
	ProjectStatusAnalyzing       = "ANALYZING"
	ProjectStatusStoryboardReady = "STORYBOARD_READY"
	ProjectStatusConfirmed       = "CONFIRMED"
	ProjectStatusRendering       = "RENDERING"
	ProjectStatusCompleted       = "COMPLETED"
	ProjectStatusFailed          = "FAILED"

	RenderStatusCreated    = "CREATED"
	RenderStatusQueued     = "QUEUED"
	RenderStatusProcessing = "PROCESSING"
	RenderStatusRendering  = "RENDERING"
	RenderStatusUploading  = "UPLOADING"
	RenderStatusSucceeded  = "SUCCEEDED"
	RenderStatusFailed     = "FAILED"
	RenderStatusCancelled  = "CANCELLED"

	AssetTypeVideo = "VIDEO"
	AssetTypeImage = "IMAGE"
)

type Access struct {
	TenantID string
	UserID   string
}

type Project struct {
	ID                 string     `json:"id"`
	TenantID           string     `json:"tenantId"`
	UserID             string     `json:"userId"`
	Title              string     `json:"title"`
	Requirement        string     `json:"requirement"`
	Status             string     `json:"status"`
	CurrentVersion     int        `json:"currentVersion"`
	OutputAssetID      string     `json:"outputAssetId,omitempty"`
	ActiveRenderTaskID string     `json:"activeRenderTaskId,omitempty"`
	ErrorCode          string     `json:"errorCode,omitempty"`
	ErrorMessage       string     `json:"errorMessage,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	DeletedAt          *time.Time `json:"deletedAt,omitempty"`
}

type AssetMetadata struct {
	OriginalName string `json:"originalName,omitempty"`
	MIMEType     string `json:"mimeType,omitempty"`
	FileSize     int64  `json:"fileSize,omitempty"`
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
	DurationMS   int64  `json:"durationMs,omitempty"`
	FileHash     string `json:"fileHash,omitempty"`
}

type ProjectAsset struct {
	ID                    string                   `json:"id"`
	ProjectID             string                   `json:"projectId"`
	TenantID              string                   `json:"tenantId"`
	UserID                string                   `json:"userId"`
	FileID                string                   `json:"fileId"`
	StorageKey            string                   `json:"storageKey"`
	AssetType             string                   `json:"assetType"`
	SortOrder             int                      `json:"sortOrder"`
	Metadata              AssetMetadata            `json:"metadata"`
	AnalysisStatus        string                   `json:"analysisStatus"`
	SourceFingerprint     string                   `json:"-"`
	NormalizedMetadata    *NormalizedMediaMetadata `json:"normalizedMetadata,omitempty"`
	FilteredProbeResult   *FilteredProbeResult     `json:"-"`
	ThumbnailFileID       string                   `json:"thumbnailFileId,omitempty"`
	ProxyFileID           string                   `json:"proxyFileId,omitempty"`
	AttemptCount          int                      `json:"attemptCount"`
	ErrorCode             string                   `json:"errorCode,omitempty"`
	SanitizedErrorMessage string                   `json:"errorMessage,omitempty"`
	AnalyzerVersion       string                   `json:"analyzerVersion,omitempty"`
	AnalysisStartedAt     *time.Time               `json:"analysisStartedAt,omitempty"`
	AnalysisFinishedAt    *time.Time               `json:"analysisFinishedAt,omitempty"`
	CreatedAt             time.Time                `json:"createdAt"`
	UpdatedAt             time.Time                `json:"updatedAt"`
}

type SceneTransition struct {
	Type       string `json:"type"`
	DurationMS int64  `json:"durationMs"`
}

type StoryboardScene struct {
	ID             string          `json:"id"`
	ProjectID      string          `json:"projectId"`
	VersionID      string          `json:"versionId"`
	SceneIndex     int             `json:"sceneIndex"`
	Title          string          `json:"title"`
	Narration      string          `json:"narration"`
	VisualPrompt   string          `json:"visualPrompt"`
	DurationMS     int64           `json:"durationMs"`
	SourceAssetIDs []string        `json:"sourceAssetIds"`
	Transition     SceneTransition `json:"transition"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

type ScriptSection struct {
	Title     string `json:"title"`
	Narration string `json:"narration"`
}

type ScriptSnapshot struct {
	Title           string          `json:"title"`
	Summary         string          `json:"summary"`
	Language        string          `json:"language"`
	EstimatedLength int64           `json:"estimatedLengthMs"`
	Sections        []ScriptSection `json:"sections"`
}

type ProjectVersion struct {
	ID                 string            `json:"id"`
	ProjectID          string            `json:"projectId"`
	TenantID           string            `json:"tenantId"`
	VersionNumber      int               `json:"versionNumber"`
	Status             string            `json:"status"`
	Requirement        string            `json:"requirement"`
	Script             ScriptSnapshot    `json:"script"`
	StoryboardSnapshot []StoryboardScene `json:"storyboardSnapshot"`
	CreatedBy          string            `json:"createdBy"`
	CreatedAt          time.Time         `json:"createdAt"`
}

type RenderSpecification struct {
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	FrameRate  int    `json:"frameRate"`
	Format     string `json:"format"`
	VideoCodec string `json:"videoCodec"`
	AudioCodec string `json:"audioCodec"`
	DurationMS int64  `json:"durationMs"`
}

type RenderTask struct {
	ID              string              `json:"id"`
	ProjectID       string              `json:"projectId"`
	VersionID       string              `json:"versionId"`
	TenantID        string              `json:"tenantId"`
	UserID          string              `json:"userId"`
	ClientRequestID string              `json:"clientRequestId"`
	Status          string              `json:"status"`
	Progress        int                 `json:"progress"`
	Step            string              `json:"step"`
	AttemptCount    int                 `json:"attemptCount"`
	MaxAttempts     int                 `json:"maxAttempts"`
	RunAfter        time.Time           `json:"runAfter"`
	Specification   RenderSpecification `json:"specification"`
	QuotedTokens    int64               `json:"quotedTokens"`
	ReservedTokens  int64               `json:"reservedTokens"`
	CapturedTokens  int64               `json:"capturedTokens"`
	ReleasedTokens  int64               `json:"releasedTokens"`
	OutputFileID    string              `json:"outputFileId,omitempty"`
	CoverFileID     string              `json:"coverFileId,omitempty"`
	OutputAssetID   string              `json:"outputAssetId,omitempty"`
	Output          RenderOutput        `json:"output"`
	ErrorCode       string              `json:"errorCode,omitempty"`
	ErrorMessage    string              `json:"errorMessage,omitempty"`
	CreatedAt       time.Time           `json:"createdAt"`
	UpdatedAt       time.Time           `json:"updatedAt"`
	StartedAt       *time.Time          `json:"startedAt,omitempty"`
	FinishedAt      *time.Time          `json:"finishedAt,omitempty"`
}

type RenderArtifact struct {
	VideoPath, CoverPath                string
	DurationMS, FileSize                int64
	Width, Height, FrameRate            int
	VideoCodec, AudioCodec, PixelFormat string
}

type RenderOutput struct {
	VideoFileID string `json:"videoFileId,omitempty"`
	CoverFileID string `json:"coverFileId,omitempty"`
	VideoURL    string `json:"videoUrl,omitempty"`
	CoverURL    string `json:"coverUrl,omitempty"`
	DurationMS  int64  `json:"durationMs,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	FrameRate   int    `json:"frameRate,omitempty"`
	FileSize    int64  `json:"fileSize,omitempty"`
	VideoCodec  string `json:"videoCodec,omitempty"`
	AudioCodec  string `json:"audioCodec,omitempty"`
	PixelFormat string `json:"pixelFormat,omitempty"`
}

type CreateProjectInput struct {
	Title       string `json:"title"`
	Requirement string `json:"requirement"`
}

type UpdateProjectInput struct {
	Title       *string `json:"title"`
	Requirement *string `json:"requirement"`
}

type CreateAssetInput struct {
	FileID    string `json:"fileId"`
	AssetType string `json:"assetType"`
	SortOrder int    `json:"sortOrder"`
}

type ReorderAssetsInput struct {
	AssetIDs []string `json:"assetIds"`
}

type CreateRenderTaskInput struct {
	VersionID       string              `json:"versionId"`
	ClientRequestID string              `json:"clientRequestId"`
	Specification   RenderSpecification `json:"specification"`
}

type FileReference struct {
	FileID    string
	TenantID  string
	UserID    string
	ObjectKey string
	Status    string
	Metadata  AssetMetadata
}
