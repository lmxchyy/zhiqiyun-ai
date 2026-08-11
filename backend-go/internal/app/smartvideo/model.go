package smartvideo

import "time"

const (
	ProjectStatusDraft           = "DRAFT"
	ProjectStatusAnalyzing       = "ANALYZING"
	ProjectStatusMaterialReady   = "MATERIAL_READY"
	ProjectStatusPlanning        = "PLANNING"
	ProjectStatusStoryboardReady = "STORYBOARD_READY"
	ProjectStatusConfirmed       = "CONFIRMED"
	ProjectStatusRendering       = "RENDERING"
	ProjectStatusCompleted       = "COMPLETED"
	ProjectStatusFailed          = "FAILED"

	RenderStatusCreated      = "CREATED"
	RenderStatusQueued       = "QUEUED"
	RenderStatusProcessing   = "PROCESSING"
	RenderStatusSynthesizing = "SYNTHESIZING"
	RenderStatusRendering    = "RENDERING"
	RenderStatusUploading    = "UPLOADING"
	RenderStatusPublishing   = "PUBLISHING"
	RenderStatusSucceeded    = "SUCCEEDED"
	RenderStatusFailed       = "FAILED"
	RenderStatusCancelled    = "CANCELLED"

	AssetTypeVideo = "VIDEO"
	AssetTypeImage = "IMAGE"

	VersionSourceAI   = "ai"
	VersionSourceUser = "user"

	PlanDailyLimit = 20
)

type Access struct {
	TenantID string
	UserID   string
}

type Project struct {
	ID                    string     `json:"id"`
	TenantID              string     `json:"tenantId"`
	UserID                string     `json:"userId"`
	Title                 string     `json:"title"`
	Requirement           string     `json:"requirement"`
	Status                string     `json:"status"`
	TargetSpec            TargetSpec `json:"targetSpec"`
	CurrentVersion        int        `json:"currentVersion"`
	CurrentVersionID      string     `json:"currentVersionId,omitempty"`
	ConfirmedVersionID    string     `json:"confirmedVersionId,omitempty"`
	ActiveAnalysisTaskID  string     `json:"activeAnalysisTaskId,omitempty"`
	ActivePlanTaskID      string     `json:"activePlanTaskId,omitempty"`
	OutputAssetID         string     `json:"outputAssetId,omitempty"`
	ActiveRenderTaskID    string     `json:"activeRenderTaskId,omitempty"`
	ErrorStage            string     `json:"errorStage,omitempty"`
	ErrorCode             string     `json:"errorCode,omitempty"`
	ErrorMessage          string     `json:"errorMessage,omitempty"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
	DeletedAt             *time.Time `json:"deletedAt,omitempty"`
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
	OrderIndex            int                      `json:"orderIndex"`
	DurationMS            int64                    `json:"durationMs"`
	Metadata              AssetMetadata            `json:"metadata"`
	ContentAuditStatus    string                   `json:"contentAuditStatus"`
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
	ID                 string             `json:"id"`
	ProjectID          string             `json:"projectId"`
	TenantID           string             `json:"tenantId"`
	VersionNumber      int                `json:"versionNumber"`
	Source             string             `json:"source"`
	ParentVersionID    string             `json:"parentVersionId,omitempty"`
	PlanSchemaVersion  int                `json:"planSchemaVersion"`
	PlanSnapshot       EditPlanV1         `json:"planSnapshot"`
	RenderManifest     *RenderManifestV1  `json:"renderManifest,omitempty"`
	ManifestHash       string             `json:"manifestHash,omitempty"`
	PlannerModelKey    string             `json:"plannerModelKey,omitempty"`
	PlannerRequestID   string             `json:"plannerRequestId,omitempty"`
	ChangeNote         string             `json:"changeNote,omitempty"`
	Status             string             `json:"status,omitempty"`
	Requirement        string             `json:"requirement,omitempty"`
	Script             ScriptSnapshot     `json:"script,omitempty"`
	StoryboardSnapshot []StoryboardScene  `json:"storyboardSnapshot,omitempty"`
	CreatedBy          string             `json:"createdBy"`
	CreatedAt          time.Time          `json:"createdAt"`
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
	Stage           string              `json:"stage"`
	Attempt         int                 `json:"attempt"`
	AttemptCount    int                 `json:"attemptCount"`
	MaxAttempts     int                 `json:"maxAttempts"`
	RunAfter        time.Time           `json:"runAfter"`
	Specification   RenderSpecification `json:"specification"`
	QuotedTokens    int64               `json:"quotedTokens"`
	ReservedTokens  int64               `json:"reservedTokens"`
	CapturedTokens  int64               `json:"capturedTokens"`
	ReleasedTokens  int64               `json:"releasedTokens"`
	QuotedPoints    int64               `json:"quotedPoints"`
	ReservedPoints  int64               `json:"reservedPoints"`
	CapturedPoints  int64               `json:"capturedPoints"`
	ReleasedPoints  int64               `json:"releasedPoints"`
	BillingTransactionID string           `json:"billingTransactionId,omitempty"`
	OutputFileID    string              `json:"outputFileId,omitempty"`
	CoverFileID     string              `json:"coverFileId,omitempty"`
	OutputAssetID   string              `json:"outputAssetId,omitempty"`
	VoiceFileID     string              `json:"voiceFileId,omitempty"`
	CaptionFileID   string              `json:"captionFileId,omitempty"`
	WorkID          string              `json:"workId,omitempty"`
	ManifestHash    string              `json:"manifestHash,omitempty"`
	RetryOfTaskID   string              `json:"retryOfTaskId,omitempty"`
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

type PlanTask struct {
	ID                string     `json:"id"`
	TenantID          string     `json:"tenantId"`
	ProjectID         string     `json:"projectId"`
	UserID            string     `json:"userId"`
	State             string     `json:"state"`
	Instruction       string     `json:"instruction"`
	SourceVersionID   string     `json:"sourceVersionId,omitempty"`
	OutputVersionID   string     `json:"outputVersionId,omitempty"`
	ModelKey          string     `json:"modelKey"`
	ProviderRequestID string     `json:"providerRequestId,omitempty"`
	Attempt           int        `json:"attempt"`
	Progress          int        `json:"progress"`
	PlanSnapshot      EditPlanV1 `json:"planSnapshot,omitempty"`
	ErrorCode         string     `json:"errorCode,omitempty"`
	ErrorMessage      string     `json:"errorMessage,omitempty"`
	LeaseOwner        string     `json:"-"`
	LeaseExpiresAt    *time.Time `json:"-"`
	HeartbeatAt       *time.Time `json:"-"`
	IdempotencyKey    string     `json:"idempotencyKey"`
	CreatedAt         time.Time  `json:"createdAt"`
	StartedAt         *time.Time `json:"startedAt,omitempty"`
	FinishedAt        *time.Time `json:"finishedAt,omitempty"`
}

type PlanRequest struct {
	ProjectID       string
	Requirement     string
	TargetSpec      TargetSpec
	Assets          []ProjectAsset
	Instruction     string
	SourceVersionID string
}

type SpeechRequest struct {
	Text     string
	ModelKey string
	VoiceKey string
	Speed    float64
}

type SpeechResult struct {
	AudioFileID string
	DurationMs  int64
	Format      string
	SampleRate  int
	Channels    int
}

type RenderManifestV1 struct {
	SchemaVersion int                `json:"schemaVersion"`
	Output        ManifestOutputSpec `json:"output"`
	Inputs        []ManifestInput    `json:"inputs"`
	Scenes        []ManifestScene    `json:"scenes"`
	AudioMix      ManifestAudioMix   `json:"audioMix"`
	VoiceFileID   string             `json:"voiceFileId,omitempty"`
	CaptionFileID string             `json:"captionFileId,omitempty"`
	ManifestHash  string             `json:"manifestHash"`
}

type ManifestAudioMix struct {
	SourceGain float64 `json:"sourceGain"`
	VoiceGain  float64 `json:"voiceGain"`
}

type ManifestOutputSpec struct {
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	FrameRate   int    `json:"frameRate"`
	VideoCodec  string `json:"videoCodec"`
	AudioCodec  string `json:"audioCodec"`
	PixelFormat string `json:"pixelFormat"`
	Format      string `json:"format"`
	Bitrate     string `json:"bitrate"`
}

type ManifestInput struct {
	FileID     string `json:"fileId"`
	ObjectKey  string `json:"objectKey"`
	Checksum   string `json:"checksum"`
	DurationMs int64  `json:"durationMs"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	AssetType  string `json:"assetType"`
}

type ManifestScene struct {
	Index        int               `json:"index"`
	DurationMs   int64             `json:"durationMs"`
	Cuts         []ManifestCut     `json:"cuts"`
	VoiceSegment *ManifestVoiceSeg `json:"voiceSegment,omitempty"`
	Transition   ManifestTrans     `json:"transition"`
	Subtitles    []ManifestCue     `json:"subtitles,omitempty"`
}

type ManifestCut struct {
	InputIndex   int     `json:"inputIndex"`
	SourceInMs   int64   `json:"sourceInMs"`
	SourceOutMs  int64   `json:"sourceOutMs"`
	FitMode      string  `json:"fitMode"`
	Motion       string  `json:"motion"`
	AudioGain    float64 `json:"audioGain"`
	TargetWidth  int     `json:"targetWidth"`
	TargetHeight int     `json:"targetHeight"`
}

type ManifestVoiceSeg struct {
	AudioFileID string `json:"audioFileId"`
	StartMs     int64  `json:"startMs"`
	DurationMs  int64  `json:"durationMs"`
}

type ManifestTrans struct {
	Type       string `json:"type"`
	DurationMs int64  `json:"durationMs"`
}

type ManifestCue struct {
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Text    string `json:"text"`
}

type RenderQuoteInput struct {
	AspectRatio string
	Resolution  string
	DurationMs  int64
	Voice       bool
}

type RenderQuote struct {
	Points    int64     `json:"points"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type RenderManifestInput struct {
	Version       ProjectVersion
	Assets        []ProjectAsset
	VoiceFileID   string
	CaptionFileID string
}

type RenderedOutput struct {
	VideoFileID string
	CoverFileID string
	DurationMs  int64
	Width       int
	Height      int
	FrameRate   int
	FileSize    int64
	VideoCodec  string
	AudioCodec  string
	PixelFormat string
}

type WorkPublishInput struct {
	Access       Access
	VideoFileID  string
	CoverFileID  string
	ProjectID    string
	VersionID    string
	RenderTaskID string
	DurationMs   int64
	Width        int
	Height       int
	FrameRate    int
	FileSize     int64
	VideoCodec   string
	AudioCodec   string
	PixelFormat  string
}

type CompletedPart struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"etag"`
}

type ObjectInfo struct {
	Key          string
	Size         int64
	ETag         string
	ContentType  string
	LastModified time.Time
}
