package storage

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrConfigNotFound       = errors.New("STORAGE_CONFIG_NOT_FOUND")
	ErrConfigDisabled       = errors.New("STORAGE_CONFIG_DISABLED")
	ErrProviderUnsupported  = errors.New("STORAGE_PROVIDER_UNSUPPORTED")
	ErrConnectionFailed     = errors.New("STORAGE_CONNECTION_FAILED")
	ErrFileNotFound         = errors.New("STORAGE_FILE_NOT_FOUND")
	ErrFileForbidden        = errors.New("STORAGE_FILE_FORBIDDEN")
	ErrFileExpired          = errors.New("STORAGE_FILE_EXPIRED")
	ErrFileQuarantined      = errors.New("STORAGE_FILE_QUARANTINED")
	ErrQuotaExceeded        = errors.New("STORAGE_QUOTA_EXCEEDED")
	ErrInvalidFileType      = errors.New("STORAGE_INVALID_FILE_TYPE")
	ErrInvalidFileSize      = errors.New("STORAGE_INVALID_FILE_SIZE")
	ErrUploadConfirmFailed  = errors.New("STORAGE_UPLOAD_CONFIRM_FAILED")
	ErrArtifactAlreadyClaimed = errors.New("STORAGE_ARTIFACT_ALREADY_CLAIMED")
	ErrDeleteFailed         = errors.New("STORAGE_DELETE_FAILED")
	ErrSecretCipherRequired = errors.New("STORAGE_MASTER_KEY is required to manage storage credentials")
	ErrBackupConfigNotFound = errors.New("BACKUP_STORAGE_CONFIG_NOT_FOUND")
	ErrConfigAlreadyExists  = errors.New("STORAGE_CONFIG_ALREADY_EXISTS")
	ErrInvalidMultipartPart = errors.New("STORAGE_INVALID_MULTIPART_PART")
	ErrMultipartNotFound    = errors.New("STORAGE_MULTIPART_NOT_FOUND")
	ErrMultipartExpired     = errors.New("STORAGE_MULTIPART_EXPIRED")
	ErrMultipartState       = errors.New("STORAGE_MULTIPART_INVALID_STATE")
)

const (
	PlatformTenantID = "platform"
	EnvConfigID      = "env_default"

	StatusPendingUpload = "PENDING_UPLOAD"
	StatusActive        = "ACTIVE"
	StatusUploadFailed  = "UPLOAD_FAILED"
	StatusQuarantined   = "QUARANTINED"
	StatusDeletePending = "DELETE_PENDING"
	StatusDeleted       = "DELETED"
	StatusExpired       = "EXPIRED"

	MultipartStateInitialized = "initialized"
	MultipartStateUploading   = "uploading"
	MultipartStateCompleting  = "completing"
	MultipartStateCompleted   = "completed"
	MultipartStateAborted     = "aborted"
	MultipartStateExpired     = "expired"

	DefaultMultipartPartSize = 8 << 20
	MinMultipartPartSize     = 5 << 20
	MaxMultipartPartSize     = 64 << 20
	MaxMultipartParts        = 10000
	MultipartSessionTTL      = 24 * time.Hour
)

const BackupObjectPrefix = "backups/postgres/"

func ValidateBackupConfig(item Config) error {
	if strings.ToLower(strings.TrimSpace(item.Purpose)) != "backup" {
		return errors.New("backup purpose is required")
	}
	if !strings.EqualFold(strings.TrimSpace(item.Provider), "huawei_obs") {
		return errors.New("backup provider must be huawei_obs")
	}
	if item.IsDefault {
		return errors.New("backup config cannot be default")
	}
	if !strings.EqualFold(strings.TrimSpace(item.Status), "ENABLED") {
		return errors.New("backup config must be enabled")
	}
	if strings.TrimSpace(item.ObjectPrefix) != BackupObjectPrefix {
		return errors.New("backup prefix must be backups/postgres/")
	}
	return nil
}

type Options struct {
	Environment       string
	DefaultProvider   string
	Endpoint          string
	PublicEndpoint    string
	Region            string
	AccessKey         string
	SecretKey         string
	Bucket            string
	PublicDomain      string
	CDNDomain         string
	MasterKey         string
	DefaultQuotaBytes int64
	MaxUploadBytes    int64
	UploadURLTTL      time.Duration
	AccessURLTTL      time.Duration
	RecycleRetention  time.Duration
	AutoCreateBucket  bool
	ForcePathStyle    bool
	AllowedExtensions []string
	AllowedMIMETypes  []string
}

type Config struct {
	ID                    string     `json:"id"`
	TenantID              string     `json:"tenantId"`
	Name                  string     `json:"name"`
	Purpose               string     `json:"purpose,omitempty"`
	ObjectPrefix          string     `json:"objectPrefix,omitempty"`
	Provider              string     `json:"provider"`
	Endpoint              string     `json:"endpoint"`
	SigningEndpoint       string     `json:"signingEndpoint,omitempty"`
	Region                string     `json:"region,omitempty"`
	Bucket                string     `json:"bucket"`
	AccessKey             string     `json:"-"`
	SecretKey             string     `json:"-"`
	SessionToken          string     `json:"-"`
	AccessKeyEncrypted    string     `json:"-"`
	SecretKeyEncrypted    string     `json:"-"`
	SessionTokenEncrypted string     `json:"-"`
	PublicDomain          string     `json:"publicDomain,omitempty"`
	CDNDomain             string     `json:"cdnDomain,omitempty"`
	UseSSL                bool       `json:"useSSL"`
	ForcePathStyle        bool       `json:"forcePathStyle"`
	IsDefault             bool       `json:"isDefault"`
	IsSystem              bool       `json:"isSystem"`
	Status                string     `json:"status"`
	LastTestStatus        string     `json:"lastTestStatus,omitempty"`
	LastTestMessage       string     `json:"lastTestMessage,omitempty"`
	LastTestAt            *time.Time `json:"lastTestAt,omitempty"`
	CreatedBy             string     `json:"createdBy,omitempty"`
	UpdatedBy             string     `json:"updatedBy,omitempty"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
	HasAccessKey          bool       `json:"hasAccessKey"`
	HasSecretKey          bool       `json:"hasSecretKey"`
}

type FileObject struct {
	FileID           string         `json:"fileId"`
	TenantID         string         `json:"tenantId"`
	UserID           string         `json:"userId"`
	StorageConfigID  string         `json:"storageConfigId"`
	Provider         string         `json:"provider"`
	Bucket           string         `json:"bucket"`
	ObjectKey        string         `json:"objectKey"`
	OriginalName     string         `json:"originalName"`
	StoredName       string         `json:"storedName"`
	Extension        string         `json:"extension,omitempty"`
	MIMEType         string         `json:"mimeType,omitempty"`
	FileSize         int64          `json:"fileSize"`
	ReservedSize     int64          `json:"reservedSize"`
	FileHash         string         `json:"fileHash,omitempty"`
	HashAlgorithm    string         `json:"hashAlgorithm,omitempty"`
	ETag             string         `json:"etag,omitempty"`
	BusinessType     string         `json:"businessType"`
	BusinessID       string         `json:"businessId,omitempty"`
	Visibility       string         `json:"visibility"`
	Status           string         `json:"status"`
	IsTemporary      bool           `json:"isTemporary"`
	ExpiresAt        *time.Time     `json:"expiresAt,omitempty"`
	RecycleExpiresAt *time.Time     `json:"recycleExpiresAt,omitempty"`
	ReferenceCount   int            `json:"referenceCount"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	DeletedAt        *time.Time     `json:"deletedAt,omitempty"`
}

type Quota struct {
	TenantID        string    `json:"tenantId"`
	QuotaBytes      int64     `json:"quotaBytes"`
	UsedBytes       int64     `json:"usedBytes"`
	ReservedBytes   int64     `json:"reservedBytes"`
	FileCount       int64     `json:"fileCount"`
	WarningPercent  int       `json:"warningPercent"`
	CriticalPercent int       `json:"criticalPercent"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type UploadInitInput struct {
	TenantID        string
	UserID          string
	FileName        string
	FileSize        int64
	MIMEType        string
	BusinessType    string
	BusinessID      string
	Visibility      string
	IsTemporary     bool
	ExpiresAt       *time.Time
	StorageConfigID string
}

type UploadTicket struct {
	File         FileObject        `json:"file"`
	UploadMethod string            `json:"uploadMethod"`
	UploadURL    string            `json:"uploadUrl"`
	ExpiresIn    int64             `json:"expiresIn"`
	Headers      map[string]string `json:"headers"`
}

type AccessContext struct {
	TenantID string
	UserID   string
	IsAdmin  bool
}

type AccessTicket struct {
	File      FileObject `json:"file"`
	URL       string     `json:"url"`
	ExpiresIn int64      `json:"expiresIn"`
}

type ObjectMetadata struct {
	Size         int64
	ETag         string
	ContentType  string
	LastModified time.Time
	Metadata     map[string]string
}

type FileFilter struct {
	TenantID     string
	UserID       string
	Query        string
	BusinessType string
	Status       string
	Provider     string
	Limit        int
	Offset       int
}

type Overview struct {
	TotalFiles     int64            `json:"totalFiles"`
	TotalBytes     int64            `json:"totalBytes"`
	PendingFiles   int64            `json:"pendingFiles"`
	RecycleFiles   int64            `json:"recycleFiles"`
	AbnormalFiles  int64            `json:"abnormalFiles"`
	TemporaryBytes int64            `json:"temporaryBytes"`
	ProviderBytes  map[string]int64 `json:"providerBytes"`
	Quota          Quota            `json:"quota"`
}

type CompletedPart struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"etag"`
	SizeBytes  int64  `json:"sizeBytes,omitempty"`
}

type MultipartInitInput struct {
	UploadInitInput
	PartSize       int64
	IdempotencyKey string
}

type MultipartSession struct {
	UploadID   string    `json:"uploadId"`
	FileID     string    `json:"fileId"`
	ObjectKey  string    `json:"objectKey"`
	PartSize   int64     `json:"partSize"`
	TotalParts int       `json:"totalParts"`
	ExpiresIn  int64     `json:"expiresIn,omitempty"`
	ExpiresAt  time.Time `json:"expiresAt,omitempty"`
}

type MultipartPartTicket struct {
	PartNumber int               `json:"partNumber"`
	UploadURL  string            `json:"uploadUrl"`
	Headers    map[string]string `json:"headers"`
	ExpiresIn  int64             `json:"expiresIn,omitempty"`
}

type MultipartUploadRecord struct {
	ID               string
	TenantID         string
	OwnerUserID      string
	FileID           string
	ProviderUploadID string
	ObjectKey        string
	FileName         string
	ContentType      string
	TotalSize        int64
	PartSize         int64
	TotalParts       int
	State            string
	IdempotencyKey   string
	ExpiresAt        time.Time
	CreatedAt        time.Time
	CompletedAt      *time.Time
	Parts            map[int]CompletedPart
}
