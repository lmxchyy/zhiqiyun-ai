package httpserver

import (
	"encoding/json"
	"time"
)

type enterpriseConnector struct {
	ID                         string
	EnterpriseID               string
	ConnectorType              string
	ConnectorName              string
	ConnectorKey               string
	AppID                      string
	AppSecretEncrypted         string
	VerificationTokenEncrypted string
	EncryptKeyEncrypted        string
	ExternalTenantKey          string
	BotOpenID                  string
	Status                     string
	Config                     connectorConfig
	LastConnectedAt            *time.Time
	LastErrorMessage           string
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

type connectorConfig struct {
	AIImageEnabled            bool   `json:"aiImageEnabled"`
	AIImageEditEnabled        bool   `json:"aiImageEditEnabled"`
	AIVideoEnabled            bool   `json:"aiVideoEnabled"`
	PPTEnabled                bool   `json:"pptEnabled"`
	DefaultImageModel         string `json:"defaultImageModel"`
	DefaultSize               string `json:"defaultSize"`
	DefaultImageCount         int    `json:"defaultImageCount"`
	MemberDailyQuota          int    `json:"memberDailyQuota"`
	DefaultVideoModel         string `json:"defaultVideoModel"`
	DefaultVideoDuration      int    `json:"defaultVideoDuration"`
	DefaultVideoAspectRatio   string `json:"defaultVideoAspectRatio"`
	DefaultVideoResolution    string `json:"defaultVideoResolution"`
	AllowImageToVideo         bool   `json:"allowImageToVideo"`
	VideoMaxDuration          int    `json:"videoMaxDuration"`
	VideoMaxResolution        string `json:"videoMaxResolution"`
	VideoPerRequestPointLimit int    `json:"videoPerRequestPointLimit"`
	VideoDailyPointLimit      int    `json:"videoDailyPointLimit"`
	VideoMonthlyPointLimit    int    `json:"videoMonthlyPointLimit"`
	VideoPermissionMode       string `json:"videoPermissionMode"`
	DefaultPPTTemplate        string `json:"defaultPptTemplate"`
	DefaultPPTPageCount       int    `json:"defaultPptPageCount"`
	PPTMaxPageCount           int    `json:"pptMaxPageCount"`
	PPTUseEnterpriseLogo      bool   `json:"pptUseEnterpriseLogo"`
	PPTUseEnterpriseKnowledge bool   `json:"pptUseEnterpriseKnowledge"`
	PPTPerRequestPointLimit   int    `json:"pptPerRequestPointLimit"`
	PPTDailyPointLimit        int    `json:"pptDailyPointLimit"`
	PPTMonthlyPointLimit      int    `json:"pptMonthlyPointLimit"`
	PPTPermissionMode         string `json:"pptPermissionMode"`
	AllowGroupChat            bool   `json:"allowGroupChat"`
	GroupRequireMention       bool   `json:"groupRequireMention"`
}

func defaultConnectorConfig() connectorConfig {
	return connectorConfig{
		AIImageEnabled: true, AIImageEditEnabled: true, AIVideoEnabled: false, PPTEnabled: false,
		DefaultSize: "1024x1024", DefaultImageCount: 1, MemberDailyQuota: 20,
		DefaultVideoDuration: 5, DefaultVideoAspectRatio: "16:9", DefaultVideoResolution: "720p",
		AllowImageToVideo: true, VideoMaxDuration: 15, VideoMaxResolution: "1080p",
		VideoPerRequestPointLimit: 1000, VideoDailyPointLimit: 3000, VideoMonthlyPointLimit: 30000, VideoPermissionMode: "allow",
		DefaultPPTTemplate: "business", DefaultPPTPageCount: 8, PPTMaxPageCount: 30,
		PPTPerRequestPointLimit: 1000, PPTDailyPointLimit: 3000, PPTMonthlyPointLimit: 30000, PPTPermissionMode: "allow",
		AllowGroupChat: true, GroupRequireMention: true,
	}
}

func normalizeConnectorConfig(value connectorConfig) connectorConfig {
	defaults := defaultConnectorConfig()
	if value.DefaultSize == "" {
		value.DefaultSize = defaults.DefaultSize
	}
	if value.DefaultImageCount <= 0 || value.DefaultImageCount > 4 {
		value.DefaultImageCount = defaults.DefaultImageCount
	}
	if value.MemberDailyQuota <= 0 || value.MemberDailyQuota > 10000 {
		value.MemberDailyQuota = defaults.MemberDailyQuota
	}
	if value.DefaultVideoDuration <= 0 {
		value.DefaultVideoDuration = defaults.DefaultVideoDuration
	}
	if value.DefaultVideoAspectRatio == "" {
		value.DefaultVideoAspectRatio = defaults.DefaultVideoAspectRatio
	}
	if value.DefaultVideoResolution == "" {
		value.DefaultVideoResolution = defaults.DefaultVideoResolution
	}
	if value.VideoMaxDuration <= 0 {
		value.VideoMaxDuration = defaults.VideoMaxDuration
	}
	if value.VideoMaxResolution == "" {
		value.VideoMaxResolution = defaults.VideoMaxResolution
	}
	if value.VideoPerRequestPointLimit <= 0 {
		value.VideoPerRequestPointLimit = defaults.VideoPerRequestPointLimit
	}
	if value.VideoDailyPointLimit <= 0 {
		value.VideoDailyPointLimit = defaults.VideoDailyPointLimit
	}
	if value.VideoMonthlyPointLimit <= 0 {
		value.VideoMonthlyPointLimit = defaults.VideoMonthlyPointLimit
	}
	value.VideoPermissionMode = normalizeConnectorPermissionMode(value.VideoPermissionMode, defaults.VideoPermissionMode)
	if value.DefaultPPTTemplate == "" {
		value.DefaultPPTTemplate = defaults.DefaultPPTTemplate
	}
	if value.DefaultPPTPageCount <= 0 {
		value.DefaultPPTPageCount = defaults.DefaultPPTPageCount
	}
	if value.PPTMaxPageCount <= 0 {
		value.PPTMaxPageCount = defaults.PPTMaxPageCount
	}
	if value.DefaultPPTPageCount > value.PPTMaxPageCount {
		value.DefaultPPTPageCount = value.PPTMaxPageCount
	}
	if value.PPTPerRequestPointLimit <= 0 {
		value.PPTPerRequestPointLimit = defaults.PPTPerRequestPointLimit
	}
	if value.PPTDailyPointLimit <= 0 {
		value.PPTDailyPointLimit = defaults.PPTDailyPointLimit
	}
	if value.PPTMonthlyPointLimit <= 0 {
		value.PPTMonthlyPointLimit = defaults.PPTMonthlyPointLimit
	}
	value.PPTPermissionMode = normalizeConnectorPermissionMode(value.PPTPermissionMode, defaults.PPTPermissionMode)
	return value
}

func normalizeConnectorPermissionMode(value, fallback string) string {
	switch value {
	case "deny", "allow", "approval":
		return value
	default:
		return fallback
	}
}

type connectorSecretState struct {
	AppSecret         bool `json:"appSecret"`
	VerificationToken bool `json:"verificationToken"`
	EncryptKey        bool `json:"encryptKey"`
}

type connectorView struct {
	ID                string               `json:"id"`
	EnterpriseID      string               `json:"enterpriseId"`
	ConnectorType     string               `json:"connectorType"`
	ConnectorName     string               `json:"connectorName"`
	ConnectorKey      string               `json:"connectorKey"`
	AppID             string               `json:"appId"`
	ExternalTenantKey string               `json:"externalTenantKey,omitempty"`
	BotOpenID         string               `json:"botOpenId,omitempty"`
	Status            string               `json:"status"`
	Config            connectorConfig      `json:"config"`
	SecretsConfigured connectorSecretState `json:"secretsConfigured"`
	CallbackURL       string               `json:"callbackUrl"`
	LastConnectedAt   string               `json:"lastConnectedAt,omitempty"`
	LastErrorMessage  string               `json:"lastErrorMessage,omitempty"`
	CreatedAt         string               `json:"createdAt"`
	UpdatedAt         string               `json:"updatedAt"`
}

type connectorSaveRequest struct {
	ConnectorName     string          `json:"connectorName"`
	AppID             string          `json:"appId"`
	AppSecret         string          `json:"appSecret"`
	VerificationToken string          `json:"verificationToken"`
	EncryptKey        string          `json:"encryptKey"`
	Config            connectorConfig `json:"config"`
}

type connectorUserBinding struct {
	ID               string         `json:"id"`
	EnterpriseID     string         `json:"enterpriseId"`
	ConnectorID      string         `json:"connectorId"`
	Platform         string         `json:"platform"`
	ExternalUserID   string         `json:"externalUserId"`
	ExternalUnionID  string         `json:"externalUnionId,omitempty"`
	ExternalName     string         `json:"externalName"`
	ExternalAvatar   string         `json:"externalAvatar,omitempty"`
	InternalUserID   string         `json:"internalUserId,omitempty"`
	InternalUserName string         `json:"internalUserName,omitempty"`
	OrganizationName string         `json:"organizationName,omitempty"`
	Permission       map[string]any `json:"permission"`
	Status           string         `json:"status"`
	DailyUsage       int            `json:"dailyUsage"`
	DailyQuota       int            `json:"dailyQuota"`
	LastActiveAt     string         `json:"lastActiveAt,omitempty"`
	CreatedAt        string         `json:"createdAt"`
	UpdatedAt        string         `json:"updatedAt"`
}

type connectorBindingUpdateRequest struct {
	InternalUserID string         `json:"internalUserId"`
	Permission     map[string]any `json:"permission"`
	Status         string         `json:"status"`
}

type connectorMessageRecord struct {
	ID                string
	EnterpriseID      string
	ConnectorID       string
	ExternalMessageID string
	ExternalChatID    string
	ExternalUserID    string
	ChatType          string
	MessageType       string
	Content           map[string]any
	ProcessingStatus  string
}

type connectorMessageView struct {
	ID                string         `json:"id"`
	Platform          string         `json:"platform"`
	ExternalMessageID string         `json:"externalMessageId"`
	ExternalChatID    string         `json:"externalChatId"`
	ExternalUserID    string         `json:"externalUserId"`
	ChatType          string         `json:"chatType"`
	MessageType       string         `json:"messageType"`
	Direction         string         `json:"direction"`
	Content           map[string]any `json:"content"`
	ProcessingStatus  string         `json:"processingStatus"`
	ErrorMessage      string         `json:"errorMessage,omitempty"`
	CreatedAt         string         `json:"createdAt"`
}

type connectorTaskRecord struct {
	ID                string         `json:"id"`
	EnterpriseID      string         `json:"enterpriseId"`
	ConnectorID       string         `json:"connectorId"`
	BindingID         string         `json:"bindingId,omitempty"`
	ExternalUserID    string         `json:"externalUserId,omitempty"`
	ExternalUserName  string         `json:"externalUserName,omitempty"`
	ExternalChatID    string         `json:"externalChatId"`
	ExternalMessageID string         `json:"externalMessageId"`
	TaskType          string         `json:"taskType"`
	Intent            string         `json:"intent"`
	OriginalText      string         `json:"originalText"`
	OptimizedPrompt   string         `json:"optimizedPrompt"`
	ModelID           string         `json:"modelId"`
	PlatformTaskID    string         `json:"platformTaskId,omitempty"`
	Status            string         `json:"status"`
	UnifiedStatus     string         `json:"unifiedStatus,omitempty"`
	InternalStage     string         `json:"internalStage,omitempty"`
	DeliveryStatus    string         `json:"deliveryStatus,omitempty"`
	DeliveryAttempts  int            `json:"deliveryAttempts,omitempty"`
	Progress          int            `json:"progress"`
	Result            map[string]any `json:"result"`
	TokenCost         int64          `json:"tokenCost"`
	PointsCost        int64          `json:"pointsCost"`
	EstimatedPoints   int64          `json:"estimatedPoints,omitempty"`
	ErrorCode         string         `json:"errorCode,omitempty"`
	ErrorMessage      string         `json:"errorMessage,omitempty"`
	StartedAt         string         `json:"startedAt,omitempty"`
	CompletedAt       string         `json:"completedAt,omitempty"`
	CreatedAt         string         `json:"createdAt"`
	UpdatedAt         string         `json:"updatedAt"`
}

type connectorSessionContext struct {
	EnterpriseID   string
	ConnectorID    string
	ExternalChatID string
	ExternalUserID string
	LastIntent     string
	LastTaskType   string
	LastTaskID     string
	LastAssetIDs   []string
	LastTopic      string
	LastParameters map[string]any
	LastPrompt     string
	ExpiresAt      time.Time
}

type connectorReferenceImage struct {
	AssetID          string
	GenerationTaskID string
	Name             string
	URL              string
}

type connectorJob struct {
	MessageID string `json:"messageId"`
}

type connectorAuthorizationSession struct {
	ID                string
	EnterpriseID      string
	Platform          string
	StateHash         string
	Status            string
	CreatedByUserID   string
	CreatedByRole     string
	OrganizationID    string
	ConnectorID       string
	ExternalTenantKey string
	ExternalUserID    string
	ExternalUserName  string
	Result            map[string]any
	ErrorCode         string
	ErrorMessage      string
	ExpiresAt         time.Time
	ConsumedAt        *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type connectorAuthorizationView struct {
	ID               string         `json:"id"`
	Platform         string         `json:"platform"`
	Status           string         `json:"status"`
	AuthorizationURL string         `json:"authorizationUrl,omitempty"`
	QRCodeDataURL    string         `json:"qrCodeDataUrl,omitempty"`
	ConnectorID      string         `json:"connectorId,omitempty"`
	ExternalUserName string         `json:"externalUserName,omitempty"`
	Result           map[string]any `json:"result,omitempty"`
	ErrorCode        string         `json:"errorCode,omitempty"`
	ErrorMessage     string         `json:"errorMessage,omitempty"`
	ExpiresAt        string         `json:"expiresAt"`
	CreatedAt        string         `json:"createdAt"`
}

type connectorAuthorizationCreateRequest struct {
	Platform string `json:"platform"`
}

type connectorPlatformView struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	Available    bool   `json:"available"`
	Configured   bool   `json:"configured"`
	Connected    bool   `json:"connected"`
	Mode         string `json:"mode"`
	Description  string `json:"description"`
	Prerequisite string `json:"prerequisite,omitempty"`
}

type connectorExternalIdentity struct {
	Platform          string
	ExternalTenantKey string
	ExternalUserID    string
	ExternalUnionID   string
	ExternalName      string
	ExternalAvatar    string
}

func connectorJSON(value any) []byte {
	raw, _ := json.Marshal(value)
	return raw
}
