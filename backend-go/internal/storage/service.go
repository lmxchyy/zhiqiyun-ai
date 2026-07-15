package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMaxUploadBytes   int64 = 2 << 30
	defaultTenantQuotaBytes int64 = 10 << 30
)

var safeSegmentPattern = regexp.MustCompile(`[^a-z0-9_-]+`)

type Service struct {
	repo    Repository
	factory ProviderFactory
	options Options
	cipher  *SecretCipher
}

func NewService(repo Repository, factory ProviderFactory, options Options) *Service {
	if options.DefaultQuotaBytes <= 0 {
		options.DefaultQuotaBytes = defaultTenantQuotaBytes
	}
	if options.MaxUploadBytes <= 0 {
		options.MaxUploadBytes = defaultMaxUploadBytes
	}
	if options.UploadURLTTL <= 0 {
		options.UploadURLTTL = 10 * time.Minute
	}
	if options.AccessURLTTL <= 0 {
		options.AccessURLTTL = 15 * time.Minute
	}
	if options.RecycleRetention <= 0 {
		options.RecycleRetention = 30 * 24 * time.Hour
	}
	if strings.TrimSpace(options.DefaultProvider) == "" {
		options.DefaultProvider = "minio"
	}
	cipher, _ := NewSecretCipher(options.MasterKey)
	return &Service{repo: repo, factory: factory, options: options, cipher: cipher}
}

func (s *Service) InitUpload(ctx context.Context, input UploadInitInput) (UploadTicket, error) {
	file, provider, err := s.preparePendingUpload(ctx, input)
	if err != nil {
		return UploadTicket{}, err
	}
	uploadURL, err := provider.CreatePresignedUploadURL(ctx, file.ObjectKey, s.options.UploadURLTTL)
	if err != nil {
		_ = s.repo.MarkUploadFailed(ctx, file.TenantID, file.FileID, err.Error())
		return UploadTicket{}, fmt.Errorf("STORAGE_UPLOAD_INIT_FAILED: %w", err)
	}
	return UploadTicket{
		File: file, UploadMethod: "PUT", UploadURL: uploadURL, ExpiresIn: int64(s.options.UploadURLTTL.Seconds()),
		Headers: map[string]string{"Content-Type": file.MIMEType},
	}, nil
}

// StoreObject writes a server-owned stream through the same validation, quota,
// tenancy and lifecycle path used by client uploads. It is intended for
// generated artifacts and other backend-produced files.
func (s *Service) StoreObject(ctx context.Context, input UploadInitInput, source io.Reader) (FileObject, error) {
	if source == nil {
		return FileObject{}, ErrInvalidFileSize
	}
	file, provider, err := s.preparePendingUpload(ctx, input)
	if err != nil {
		return FileObject{}, err
	}
	metadata, err := provider.PutObject(ctx, file.ObjectKey, source, file.ReservedSize, file.MIMEType)
	if err != nil {
		_ = provider.DeleteObject(ctx, file.ObjectKey)
		_ = s.repo.MarkUploadFailed(ctx, file.TenantID, file.FileID, err.Error())
		return FileObject{}, fmt.Errorf("STORAGE_UPLOAD_FAILED: %w", err)
	}
	if metadata.Size != file.ReservedSize {
		_ = provider.DeleteObject(ctx, file.ObjectKey)
		_ = s.repo.MarkUploadFailed(ctx, file.TenantID, file.FileID, "stored object size does not match reservation")
		return FileObject{}, fmt.Errorf("%w: stored object size does not match reservation", ErrUploadConfirmFailed)
	}
	if strings.TrimSpace(metadata.ContentType) == "" {
		metadata.ContentType = file.MIMEType
	}
	completed, err := s.repo.CompleteUpload(ctx, file.TenantID, file.FileID, metadata)
	if err != nil {
		_ = provider.DeleteObject(ctx, file.ObjectKey)
		_ = s.repo.MarkUploadFailed(ctx, file.TenantID, file.FileID, err.Error())
		return FileObject{}, err
	}
	return completed, nil
}

// StorageAvailable reports whether a tenant can resolve an enabled default
// storage configuration. A missing configuration is a normal disabled state;
// malformed or undecryptable configurations remain visible as errors.
func (s *Service) StorageAvailable(ctx context.Context, tenantID string) (bool, error) {
	_, err := s.resolveConfig(ctx, strings.TrimSpace(tenantID), "")
	if errors.Is(err, ErrConfigNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) preparePendingUpload(ctx context.Context, input UploadInitInput) (FileObject, Provider, error) {
	input.TenantID = strings.TrimSpace(input.TenantID)
	input.UserID = strings.TrimSpace(input.UserID)
	if input.TenantID == "" || input.UserID == "" {
		return FileObject{}, nil, ErrFileForbidden
	}
	fileName, extension, err := s.validateUpload(input.FileName, input.FileSize, input.MIMEType)
	if err != nil {
		return FileObject{}, nil, err
	}
	config, err := s.resolveConfig(ctx, input.TenantID, input.StorageConfigID)
	if err != nil {
		return FileObject{}, nil, err
	}
	provider, err := s.factory.Build(config)
	if err != nil {
		return FileObject{}, nil, err
	}
	fileID := newID("file")
	objectID := newID("")
	businessType := safeSegment(firstNonEmpty(input.BusinessType, "uploads"))
	now := time.Now().UTC()
	objectKey := fmt.Sprintf("tenants/%s/%s/%04d/%02d/%02d/%s.%s", safeSegment(input.TenantID), businessType, now.Year(), int(now.Month()), now.Day(), objectID, extension)
	visibility := strings.ToUpper(strings.TrimSpace(input.Visibility))
	if visibility == "" {
		visibility = "PRIVATE"
	}
	if !contains([]string{"PRIVATE", "TENANT", "SHARED", "PUBLIC", "SYSTEM"}, visibility) {
		return FileObject{}, nil, ErrFileForbidden
	}
	file := FileObject{
		FileID: fileID, TenantID: input.TenantID, UserID: input.UserID, StorageConfigID: config.ID,
		Provider: config.Provider, Bucket: config.Bucket, ObjectKey: objectKey,
		OriginalName: fileName, StoredName: path.Base(objectKey), Extension: extension,
		MIMEType: normalizeMIME(input.MIMEType), ReservedSize: input.FileSize,
		BusinessType: businessType, BusinessID: strings.TrimSpace(input.BusinessID), Visibility: visibility,
		Status: StatusPendingUpload, IsTemporary: input.IsTemporary, ExpiresAt: input.ExpiresAt,
		Metadata:  map[string]any{"declaredSize": input.FileSize, "declaredMimeType": normalizeMIME(input.MIMEType)},
		CreatedAt: now, UpdatedAt: now,
	}
	if err = s.repo.CreatePending(ctx, file, s.options.DefaultQuotaBytes); err != nil {
		return FileObject{}, nil, err
	}
	return file, provider, nil
}

func (s *Service) CompleteUpload(ctx context.Context, access AccessContext, fileID string) (FileObject, error) {
	file, err := s.repo.GetFile(ctx, access.TenantID, fileID)
	if err != nil {
		return FileObject{}, err
	}
	if !access.IsAdmin && file.UserID != access.UserID {
		return FileObject{}, ErrFileForbidden
	}
	if file.Status == StatusActive {
		return file, nil
	}
	if file.Status != StatusPendingUpload {
		return FileObject{}, ErrUploadConfirmFailed
	}
	provider, err := s.providerForFile(ctx, file)
	if err != nil {
		return FileObject{}, err
	}
	metadata, err := provider.HeadObject(ctx, file.ObjectKey)
	if err != nil {
		_ = s.repo.MarkUploadFailed(ctx, file.TenantID, file.FileID, err.Error())
		return FileObject{}, fmt.Errorf("%w: %v", ErrUploadConfirmFailed, err)
	}
	if metadata.Size <= 0 || metadata.Size > s.options.MaxUploadBytes || metadata.Size != file.ReservedSize {
		_ = s.repo.MarkUploadFailed(ctx, file.TenantID, file.FileID, "uploaded object size does not match reservation")
		return FileObject{}, fmt.Errorf("%w: uploaded object size does not match reservation", ErrUploadConfirmFailed)
	}
	actualMIME := normalizeMIME(metadata.ContentType)
	declaredMIME := normalizeMIME(file.MIMEType)
	if actualMIME != "" && actualMIME != "application/octet-stream" && declaredMIME != "" && actualMIME != declaredMIME {
		_ = s.repo.MarkUploadFailed(ctx, file.TenantID, file.FileID, "uploaded object MIME type does not match reservation")
		return FileObject{}, fmt.Errorf("%w: uploaded object MIME type does not match reservation", ErrUploadConfirmFailed)
	}
	completed, err := s.repo.CompleteUpload(ctx, file.TenantID, file.FileID, metadata)
	if err != nil {
		if errors.Is(err, ErrQuotaExceeded) {
			_ = s.repo.MarkUploadFailed(ctx, file.TenantID, file.FileID, err.Error())
		}
		return FileObject{}, err
	}
	return completed, nil
}

func (s *Service) GetFile(ctx context.Context, access AccessContext, fileID string) (FileObject, error) {
	file, err := s.repo.GetFile(ctx, access.TenantID, fileID)
	if err != nil {
		return FileObject{}, err
	}
	if err = authorizeFile(access, file); err != nil {
		return FileObject{}, err
	}
	return file, nil
}

func (s *Service) AccessURL(ctx context.Context, access AccessContext, fileID string, download bool) (AccessTicket, error) {
	file, err := s.GetFile(ctx, access, fileID)
	if err != nil {
		return AccessTicket{}, err
	}
	if file.Status == StatusQuarantined {
		return AccessTicket{}, ErrFileQuarantined
	}
	if file.Status == StatusExpired || (file.ExpiresAt != nil && time.Now().After(*file.ExpiresAt)) {
		return AccessTicket{}, ErrFileExpired
	}
	if file.Status != StatusActive {
		return AccessTicket{}, ErrFileNotFound
	}
	provider, err := s.providerForFile(ctx, file)
	if err != nil {
		return AccessTicket{}, err
	}
	ttl := s.options.AccessURLTTL
	if download && ttl > 10*time.Minute {
		ttl = 10 * time.Minute
	}
	url, err := provider.CreatePresignedDownloadURL(ctx, file.ObjectKey, ttl)
	if err != nil {
		return AccessTicket{}, err
	}
	return AccessTicket{File: file, URL: url, ExpiresIn: int64(ttl.Seconds())}, nil
}

func (s *Service) Delete(ctx context.Context, access AccessContext, fileID string) (FileObject, error) {
	file, err := s.repo.GetFile(ctx, access.TenantID, fileID)
	if err != nil {
		return FileObject{}, err
	}
	if !access.IsAdmin && file.UserID != access.UserID {
		return FileObject{}, ErrFileForbidden
	}
	return s.repo.MarkDeletePending(ctx, file.TenantID, file.FileID, time.Now().UTC().Add(s.options.RecycleRetention))
}

func (s *Service) Restore(ctx context.Context, access AccessContext, fileID string) (FileObject, error) {
	file, err := s.repo.GetFile(ctx, access.TenantID, fileID)
	if err != nil {
		return FileObject{}, err
	}
	if !access.IsAdmin && file.UserID != access.UserID {
		return FileObject{}, ErrFileForbidden
	}
	return s.repo.RestoreFile(ctx, file.TenantID, file.FileID)
}

func (s *Service) PermanentDelete(ctx context.Context, access AccessContext, fileID string) error {
	file, err := s.repo.GetFile(ctx, access.TenantID, fileID)
	if err != nil {
		return err
	}
	if !access.IsAdmin && file.UserID != access.UserID {
		return ErrFileForbidden
	}
	if file.Status != StatusDeletePending && !access.IsAdmin {
		return ErrDeleteFailed
	}
	provider, err := s.providerForFile(ctx, file)
	if err != nil {
		return err
	}
	if err = provider.DeleteObject(ctx, file.ObjectKey); err != nil {
		return fmt.Errorf("%w: %v", ErrDeleteFailed, err)
	}
	return s.repo.MarkDeleted(ctx, file.TenantID, file.FileID)
}

func (s *Service) ListFiles(ctx context.Context, filter FileFilter) ([]FileObject, int64, error) {
	return s.repo.ListFiles(ctx, filter)
}

func (s *Service) Overview(ctx context.Context, tenantID string) (Overview, error) {
	return s.repo.Overview(ctx, tenantID)
}

func (s *Service) Quota(ctx context.Context, tenantID string) (Quota, error) {
	return s.repo.GetQuota(ctx, tenantID, s.options.DefaultQuotaBytes)
}

func (s *Service) UpdateQuota(ctx context.Context, quota Quota) (Quota, error) {
	if quota.TenantID == "" || quota.QuotaBytes < 0 {
		return Quota{}, ErrInvalidFileSize
	}
	if quota.WarningPercent <= 0 {
		quota.WarningPercent = 80
	}
	if quota.CriticalPercent <= 0 {
		quota.CriticalPercent = 95
	}
	return s.repo.UpdateQuota(ctx, quota)
}

func (s *Service) ListConfigs(ctx context.Context, tenantID string) ([]Config, error) {
	items, err := s.repo.ListConfigs(ctx, tenantID, true)
	if err != nil {
		return nil, err
	}
	if s.hasEnvConfig() && !hasConfigID(items, EnvConfigID) {
		items = append(items, s.envConfig())
	}
	return items, nil
}

func (s *Service) SaveConfig(ctx context.Context, item Config, accessKey string, secretKey string, sessionToken string) (Config, error) {
	item.ID = strings.TrimSpace(item.ID)
	if item.ID == "" {
		item.ID = newID("storage")
		item.CreatedAt = time.Now().UTC()
	}
	if item.ID == EnvConfigID {
		return Config{}, ErrFileForbidden
	}
	item.TenantID = firstNonEmpty(strings.TrimSpace(item.TenantID), PlatformTenantID)
	item.Name = strings.TrimSpace(item.Name)
	item.Provider = strings.ToLower(strings.TrimSpace(item.Provider))
	item.Endpoint = strings.TrimSpace(item.Endpoint)
	item.SigningEndpoint = strings.TrimSpace(item.SigningEndpoint)
	item.Bucket = strings.TrimSpace(item.Bucket)
	item.Status = strings.ToUpper(firstNonEmpty(item.Status, "ENABLED"))
	if item.Name == "" || item.Endpoint == "" || item.Bucket == "" || !contains([]string{"ENABLED", "DISABLED"}, item.Status) {
		return Config{}, ErrConfigNotFound
	}
	if _, err := (S3ProviderFactory{AutoCreateBucket: s.options.AutoCreateBucket}).Build(Config{Provider: item.Provider, Endpoint: item.Endpoint, SigningEndpoint: item.SigningEndpoint, Region: item.Region, Bucket: item.Bucket, AccessKey: firstNonEmpty(accessKey, "validate"), SecretKey: firstNonEmpty(secretKey, "validate"), UseSSL: item.UseSSL, ForcePathStyle: item.ForcePathStyle}); err != nil {
		return Config{}, err
	}
	existing, existingErr := s.repo.GetConfig(ctx, item.ID)
	if existingErr == nil {
		item.AccessKeyEncrypted = existing.AccessKeyEncrypted
		item.SecretKeyEncrypted = existing.SecretKeyEncrypted
		item.SessionTokenEncrypted = existing.SessionTokenEncrypted
		item.CreatedAt = existing.CreatedAt
		item.CreatedBy = firstNonEmpty(item.CreatedBy, existing.CreatedBy)
	} else if !errors.Is(existingErr, ErrConfigNotFound) {
		return Config{}, existingErr
	}
	if accessKey != "" || secretKey != "" || sessionToken != "" {
		if s.cipher == nil {
			return Config{}, ErrSecretCipherRequired
		}
		var err error
		if accessKey != "" {
			item.AccessKeyEncrypted, err = s.cipher.Encrypt(accessKey, item.ID)
			if err != nil {
				return Config{}, err
			}
		}
		if secretKey != "" {
			item.SecretKeyEncrypted, err = s.cipher.Encrypt(secretKey, item.ID)
			if err != nil {
				return Config{}, err
			}
		}
		if sessionToken != "" {
			item.SessionTokenEncrypted, err = s.cipher.Encrypt(sessionToken, item.ID)
			if err != nil {
				return Config{}, err
			}
		}
	}
	if item.AccessKeyEncrypted == "" || item.SecretKeyEncrypted == "" {
		return Config{}, ErrSecretCipherRequired
	}
	return s.repo.SaveConfig(ctx, item)
}

func (s *Service) DeleteConfig(ctx context.Context, id string) error {
	if id == EnvConfigID {
		return ErrFileForbidden
	}
	return s.repo.DeleteConfig(ctx, id)
}

func (s *Service) TestConfig(ctx context.Context, id string) error {
	config, err := s.configByID(ctx, id)
	if err != nil {
		return err
	}
	provider, err := s.factory.Build(config)
	if err == nil {
		err = provider.TestConnection(ctx)
	}
	status := "SUCCESS"
	message := "connection succeeded"
	if err != nil {
		status = "FAILED"
		message = sanitizeError(err)
	}
	if id != EnvConfigID {
		_ = s.repo.UpdateConfigTest(ctx, id, status, message, time.Now().UTC())
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}
	return nil
}

func (s *Service) resolveConfig(ctx context.Context, tenantID string, specificID string) (Config, error) {
	if strings.TrimSpace(specificID) != "" {
		config, err := s.configByID(ctx, specificID)
		if err != nil {
			return Config{}, err
		}
		if config.TenantID != PlatformTenantID && config.TenantID != tenantID {
			return Config{}, ErrFileForbidden
		}
		if !strings.EqualFold(config.Status, "ENABLED") {
			return Config{}, ErrConfigDisabled
		}
		return config, nil
	}
	items, err := s.repo.ListConfigs(ctx, tenantID, true)
	if err != nil {
		return Config{}, err
	}
	for _, tenantDefault := range []bool{true, false} {
		for _, item := range items {
			if !strings.EqualFold(item.Status, "ENABLED") || !item.IsDefault {
				continue
			}
			if tenantDefault && item.TenantID == tenantID {
				return s.hydrateConfig(item)
			}
			if !tenantDefault && item.TenantID == PlatformTenantID {
				return s.hydrateConfig(item)
			}
		}
	}
	if s.hasEnvConfig() {
		return s.envConfig(), nil
	}
	return Config{}, ErrConfigNotFound
}

func (s *Service) providerForFile(ctx context.Context, file FileObject) (Provider, error) {
	config, err := s.configByID(ctx, file.StorageConfigID)
	if err != nil {
		return nil, err
	}
	return s.factory.Build(config)
}

func (s *Service) configByID(ctx context.Context, id string) (Config, error) {
	if id == EnvConfigID {
		if !s.hasEnvConfig() {
			return Config{}, ErrConfigNotFound
		}
		return s.envConfig(), nil
	}
	item, err := s.repo.GetConfig(ctx, id)
	if err != nil {
		return Config{}, err
	}
	return s.hydrateConfig(item)
}

func (s *Service) hydrateConfig(item Config) (Config, error) {
	if item.AccessKeyEncrypted != "" {
		if s.cipher == nil {
			return Config{}, ErrSecretCipherRequired
		}
		var err error
		item.AccessKey, err = s.cipher.Decrypt(item.AccessKeyEncrypted, item.ID)
		if err != nil {
			return Config{}, err
		}
		item.SecretKey, err = s.cipher.Decrypt(item.SecretKeyEncrypted, item.ID)
		if err != nil {
			return Config{}, err
		}
		item.SessionToken, err = s.cipher.Decrypt(item.SessionTokenEncrypted, item.ID)
		if err != nil {
			return Config{}, err
		}
	}
	return item, nil
}

func (s *Service) envConfig() Config {
	return Config{
		ID: EnvConfigID, TenantID: PlatformTenantID, Name: "Environment default", Provider: strings.ToLower(s.options.DefaultProvider),
		Endpoint: s.options.Endpoint, SigningEndpoint: firstNonEmpty(s.options.PublicEndpoint, s.options.Endpoint), Region: s.options.Region, Bucket: s.options.Bucket,
		AccessKey: s.options.AccessKey, SecretKey: s.options.SecretKey,
		PublicDomain: s.options.PublicDomain, CDNDomain: s.options.CDNDomain,
		UseSSL: strings.HasPrefix(strings.ToLower(s.options.Endpoint), "https://"), ForcePathStyle: s.options.ForcePathStyle,
		IsDefault: true, IsSystem: true, Status: "ENABLED", HasAccessKey: s.options.AccessKey != "", HasSecretKey: s.options.SecretKey != "",
	}
}

func (s *Service) hasEnvConfig() bool {
	return strings.TrimSpace(s.options.Endpoint) != "" && strings.TrimSpace(s.options.AccessKey) != "" && strings.TrimSpace(s.options.SecretKey) != "" && strings.TrimSpace(s.options.Bucket) != ""
}

func (s *Service) validateUpload(name string, size int64, mimeType string) (string, string, error) {
	name = path.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	if name == "" || name == "." || name == "/" || strings.ContainsAny(name, "\x00\r\n") {
		return "", "", ErrInvalidFileType
	}
	if size <= 0 || size > s.options.MaxUploadBytes {
		return "", "", ErrInvalidFileSize
	}
	extension := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
	if extension == "" || !contains(s.allowedExtensions(), extension) {
		return "", "", ErrInvalidFileType
	}
	actualMIME := normalizeMIME(mimeType)
	if actualMIME == "" || !contains(s.allowedMIMETypes(), actualMIME) || !extensionMatchesMIME(extension, actualMIME) {
		return "", "", ErrInvalidFileType
	}
	return name, extension, nil
}

func (s *Service) allowedExtensions() []string {
	if len(s.options.AllowedExtensions) > 0 {
		return lowerSlice(s.options.AllowedExtensions)
	}
	return []string{"jpg", "jpeg", "png", "webp", "avif", "gif", "pdf", "txt", "csv", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "zip", "mp4", "webm", "mov", "m4v", "mp3", "wav"}
}

func (s *Service) allowedMIMETypes() []string {
	if len(s.options.AllowedMIMETypes) > 0 {
		return lowerSlice(s.options.AllowedMIMETypes)
	}
	return []string{
		"image/jpeg", "image/png", "image/webp", "image/avif", "image/gif", "application/pdf", "text/plain", "text/csv",
		"application/msword", "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.ms-excel", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.ms-powerpoint", "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"application/zip", "application/x-zip-compressed", "video/mp4", "video/webm", "video/quicktime", "audio/mpeg", "audio/wav", "audio/x-wav",
	}
}

func extensionMatchesMIME(extension string, mimeType string) bool {
	allowed := map[string][]string{
		"jpg": {"image/jpeg"}, "jpeg": {"image/jpeg"}, "png": {"image/png"}, "webp": {"image/webp"}, "avif": {"image/avif"}, "gif": {"image/gif"},
		"pdf": {"application/pdf"}, "txt": {"text/plain"}, "csv": {"text/csv", "text/plain"}, "doc": {"application/msword"},
		"docx": {"application/vnd.openxmlformats-officedocument.wordprocessingml.document"}, "xls": {"application/vnd.ms-excel"},
		"xlsx": {"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"}, "ppt": {"application/vnd.ms-powerpoint"},
		"pptx": {"application/vnd.openxmlformats-officedocument.presentationml.presentation"}, "zip": {"application/zip", "application/x-zip-compressed"},
		"mp4": {"video/mp4"}, "m4v": {"video/mp4"}, "webm": {"video/webm"}, "mov": {"video/quicktime"},
		"mp3": {"audio/mpeg"}, "wav": {"audio/wav", "audio/x-wav"},
	}
	return contains(allowed[extension], mimeType)
}

func authorizeFile(access AccessContext, file FileObject) error {
	if file.TenantID != access.TenantID {
		return ErrFileForbidden
	}
	if access.IsAdmin || file.UserID == access.UserID || contains([]string{"TENANT", "SHARED", "PUBLIC", "SYSTEM"}, file.Visibility) {
		return nil
	}
	return ErrFileForbidden
}

func normalizeMIME(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if parsed, _, err := mime.ParseMediaType(value); err == nil {
		return parsed
	}
	if index := strings.Index(value, ";"); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(value)
}

func safeSegment(value string) string {
	value = safeSegmentPattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "default"
	}
	return value
}

func newID(prefix string) string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err == nil {
		if prefix == "" {
			return hex.EncodeToString(raw)
		}
		return prefix + "_" + hex.EncodeToString(raw)
	}
	value := strconv.FormatInt(time.Now().UnixNano(), 36)
	if prefix == "" {
		return value
	}
	return prefix + "_" + value
}

func sanitizeError(err error) string {
	message := err.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}

func hasConfigID(items []Config, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func contains(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}

func lowerSlice(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item = strings.ToLower(strings.TrimSpace(item)); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
