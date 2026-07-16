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
	AIImageEnabled      bool   `json:"aiImageEnabled"`
	DefaultImageModel   string `json:"defaultImageModel"`
	DefaultSize         string `json:"defaultSize"`
	DefaultImageCount   int    `json:"defaultImageCount"`
	MemberDailyQuota    int    `json:"memberDailyQuota"`
	AllowGroupChat      bool   `json:"allowGroupChat"`
	GroupRequireMention bool   `json:"groupRequireMention"`
}

func defaultConnectorConfig() connectorConfig {
	return connectorConfig{AIImageEnabled: true, DefaultSize: "1024x1024", DefaultImageCount: 1, MemberDailyQuota: 20, AllowGroupChat: true, GroupRequireMention: true}
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
	return value
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
	Progress          int            `json:"progress"`
	Result            map[string]any `json:"result"`
	TokenCost         int64          `json:"tokenCost"`
	PointsCost        int64          `json:"pointsCost"`
	ErrorCode         string         `json:"errorCode,omitempty"`
	ErrorMessage      string         `json:"errorMessage,omitempty"`
	StartedAt         string         `json:"startedAt,omitempty"`
	CompletedAt       string         `json:"completedAt,omitempty"`
	CreatedAt         string         `json:"createdAt"`
	UpdatedAt         string         `json:"updatedAt"`
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
