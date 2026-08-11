package storage

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

type Repository interface {
	ListConfigs(context.Context, string, bool) ([]Config, error)
	GetConfig(context.Context, string) (Config, error)
	SaveConfig(context.Context, Config) (Config, error)
	DeleteConfig(context.Context, string) error
	UpdateConfigTest(context.Context, string, string, string, time.Time) error
	CreatePending(context.Context, FileObject, int64) error
	CompleteUpload(context.Context, string, string, ObjectMetadata) (FileObject, error)
	MarkUploadFailed(context.Context, string, string, string) error
	GetFile(context.Context, string, string) (FileObject, error)
	ListFiles(context.Context, FileFilter) ([]FileObject, int64, error)
	MarkDeletePending(context.Context, string, string, time.Time) (FileObject, error)
	RestoreFile(context.Context, string, string) (FileObject, error)
	MarkDeleted(context.Context, string, string) error
	GetQuota(context.Context, string, int64) (Quota, error)
	UpdateQuota(context.Context, Quota) (Quota, error)
	Overview(context.Context, string) (Overview, error)
	CreateMultipartSession(context.Context, MultipartUploadRecord) error
	GetMultipartSession(context.Context, string, string, string) (MultipartUploadRecord, error)
	GetMultipartSessionByIdempotency(context.Context, string, string, string) (MultipartUploadRecord, error)
	SaveMultipartPart(context.Context, string, CompletedPart) error
	UpdateMultipartState(context.Context, string, string, string, *time.Time) error
}

type MemoryRepository struct {
	mu         sync.Mutex
	configs    map[string]Config
	files      map[string]FileObject
	quotas     map[string]Quota
	multiparts map[string]MultipartUploadRecord
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		configs: map[string]Config{}, files: map[string]FileObject{}, quotas: map[string]Quota{},
		multiparts: map[string]MultipartUploadRecord{},
	}
}

func (r *MemoryRepository) ListConfigs(_ context.Context, tenantID string, includePlatform bool) ([]Config, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := []Config{}
	for _, item := range r.configs {
		if item.TenantID == tenantID || (includePlatform && item.TenantID == PlatformTenantID) {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDefault != items[j].IsDefault {
			return items[i].IsDefault
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, nil
}

func (r *MemoryRepository) GetConfig(_ context.Context, id string) (Config, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.configs[id]
	if !ok {
		return Config{}, ErrConfigNotFound
	}
	return item, nil
}

func (r *MemoryRepository) SaveConfig(_ context.Context, item Config) (Config, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	if item.IsDefault {
		for id, existing := range r.configs {
			if id != item.ID && existing.TenantID == item.TenantID {
				existing.IsDefault = false
				r.configs[id] = existing
			}
		}
	}
	r.configs[item.ID] = item
	return item, nil
}

func (r *MemoryRepository) DeleteConfig(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, file := range r.files {
		if file.StorageConfigID == id && file.Status != StatusDeleted {
			return ErrDeleteFailed
		}
	}
	if _, ok := r.configs[id]; !ok {
		return ErrConfigNotFound
	}
	delete(r.configs, id)
	return nil
}

func (r *MemoryRepository) UpdateConfigTest(_ context.Context, id string, status string, message string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.configs[id]
	if !ok {
		return ErrConfigNotFound
	}
	item.LastTestStatus = status
	item.LastTestMessage = message
	item.LastTestAt = &at
	item.UpdatedAt = at
	r.configs[id] = item
	return nil
}

func (r *MemoryRepository) ensureQuota(tenantID string, defaultQuota int64) Quota {
	quota, ok := r.quotas[tenantID]
	if !ok {
		quota = Quota{TenantID: tenantID, QuotaBytes: defaultQuota, WarningPercent: 80, CriticalPercent: 95, UpdatedAt: time.Now().UTC()}
		r.quotas[tenantID] = quota
	}
	return quota
}

func (r *MemoryRepository) CreatePending(_ context.Context, file FileObject, defaultQuota int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.files[file.FileID]; exists {
		return ErrUploadConfirmFailed
	}
	quota := r.ensureQuota(file.TenantID, defaultQuota)
	if quota.QuotaBytes > 0 && quota.UsedBytes+quota.ReservedBytes+file.ReservedSize > quota.QuotaBytes {
		return ErrQuotaExceeded
	}
	quota.ReservedBytes += file.ReservedSize
	quota.UpdatedAt = time.Now().UTC()
	r.quotas[file.TenantID] = quota
	r.files[file.FileID] = cloneFile(file)
	return nil
}

func (r *MemoryRepository) CompleteUpload(_ context.Context, tenantID string, fileID string, metadata ObjectMetadata) (FileObject, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	file, ok := r.files[fileID]
	if !ok || file.TenantID != tenantID {
		return FileObject{}, ErrFileNotFound
	}
	if file.Status == StatusActive {
		return cloneFile(file), nil
	}
	if file.Status != StatusPendingUpload {
		return FileObject{}, ErrUploadConfirmFailed
	}
	quota := r.ensureQuota(tenantID, 0)
	projected := quota.UsedBytes + quota.ReservedBytes - file.ReservedSize + metadata.Size
	if quota.QuotaBytes > 0 && projected > quota.QuotaBytes {
		return FileObject{}, ErrQuotaExceeded
	}
	quota.ReservedBytes = maxInt64(0, quota.ReservedBytes-file.ReservedSize)
	quota.UsedBytes += metadata.Size
	quota.FileCount++
	quota.UpdatedAt = time.Now().UTC()
	r.quotas[tenantID] = quota
	file.FileSize = metadata.Size
	file.ReservedSize = 0
	file.ETag = metadata.ETag
	if metadata.ContentType != "" {
		file.MIMEType = metadata.ContentType
	}
	file.Status = StatusActive
	file.Metadata = stringMapToAny(metadata.Metadata)
	file.UpdatedAt = quota.UpdatedAt
	r.files[fileID] = cloneFile(file)
	return cloneFile(file), nil
}

func (r *MemoryRepository) MarkUploadFailed(_ context.Context, tenantID string, fileID string, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	file, ok := r.files[fileID]
	if !ok || file.TenantID != tenantID {
		return ErrFileNotFound
	}
	if file.Status == StatusPendingUpload {
		quota := r.ensureQuota(tenantID, 0)
		quota.ReservedBytes = maxInt64(0, quota.ReservedBytes-file.ReservedSize)
		quota.UpdatedAt = time.Now().UTC()
		r.quotas[tenantID] = quota
		file.ReservedSize = 0
		file.Status = StatusUploadFailed
		if file.Metadata == nil {
			file.Metadata = map[string]any{}
		}
		file.Metadata["uploadError"] = reason
		file.UpdatedAt = quota.UpdatedAt
		r.files[fileID] = cloneFile(file)
	}
	return nil
}

func (r *MemoryRepository) GetFile(_ context.Context, tenantID string, fileID string) (FileObject, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	file, ok := r.files[fileID]
	if !ok || (tenantID != "" && file.TenantID != tenantID) {
		return FileObject{}, ErrFileNotFound
	}
	return cloneFile(file), nil
}

func (r *MemoryRepository) ListFiles(_ context.Context, filter FileFilter) ([]FileObject, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := []FileObject{}
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	for _, file := range r.files {
		if filter.TenantID != "" && file.TenantID != filter.TenantID {
			continue
		}
		if filter.UserID != "" && file.UserID != filter.UserID {
			continue
		}
		if filter.Status != "" && !strings.EqualFold(file.Status, filter.Status) {
			continue
		}
		if filter.BusinessType != "" && !strings.EqualFold(file.BusinessType, filter.BusinessType) {
			continue
		}
		if filter.Provider != "" && !strings.EqualFold(file.Provider, filter.Provider) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(file.FileID+" "+file.OriginalName), query) {
			continue
		}
		items = append(items, cloneFile(file))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	total := int64(len(items))
	start := filter.Offset
	if start < 0 {
		start = 0
	}
	if start > len(items) {
		start = len(items)
	}
	end := len(items)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	return items[start:end], total, nil
}

func (r *MemoryRepository) MarkDeletePending(_ context.Context, tenantID string, fileID string, recycleExpiresAt time.Time) (FileObject, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	file, ok := r.files[fileID]
	if !ok || file.TenantID != tenantID {
		return FileObject{}, ErrFileNotFound
	}
	if file.Status == StatusDeletePending {
		return cloneFile(file), nil
	}
	if file.Status != StatusActive || file.ReferenceCount > 0 {
		return FileObject{}, ErrDeleteFailed
	}
	now := time.Now().UTC()
	file.Status = StatusDeletePending
	file.DeletedAt = &now
	file.RecycleExpiresAt = &recycleExpiresAt
	file.UpdatedAt = now
	r.files[fileID] = cloneFile(file)
	return cloneFile(file), nil
}

func (r *MemoryRepository) RestoreFile(_ context.Context, tenantID string, fileID string) (FileObject, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	file, ok := r.files[fileID]
	if !ok || file.TenantID != tenantID {
		return FileObject{}, ErrFileNotFound
	}
	if file.Status != StatusDeletePending {
		return FileObject{}, ErrDeleteFailed
	}
	file.Status = StatusActive
	file.DeletedAt = nil
	file.RecycleExpiresAt = nil
	file.UpdatedAt = time.Now().UTC()
	r.files[fileID] = cloneFile(file)
	return cloneFile(file), nil
}

func (r *MemoryRepository) MarkDeleted(_ context.Context, tenantID string, fileID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	file, ok := r.files[fileID]
	if !ok || file.TenantID != tenantID {
		return ErrFileNotFound
	}
	if file.Status == StatusDeleted {
		return nil
	}
	quota := r.ensureQuota(tenantID, 0)
	quota.UsedBytes = maxInt64(0, quota.UsedBytes-file.FileSize)
	quota.FileCount = maxInt64(0, quota.FileCount-1)
	quota.UpdatedAt = time.Now().UTC()
	r.quotas[tenantID] = quota
	file.Status = StatusDeleted
	file.UpdatedAt = quota.UpdatedAt
	r.files[fileID] = cloneFile(file)
	return nil
}

func (r *MemoryRepository) GetQuota(_ context.Context, tenantID string, defaultQuota int64) (Quota, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ensureQuota(tenantID, defaultQuota), nil
}

func (r *MemoryRepository) UpdateQuota(_ context.Context, quota Quota) (Quota, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing := r.ensureQuota(quota.TenantID, quota.QuotaBytes)
	existing.QuotaBytes = quota.QuotaBytes
	if quota.WarningPercent > 0 {
		existing.WarningPercent = quota.WarningPercent
	}
	if quota.CriticalPercent > 0 {
		existing.CriticalPercent = quota.CriticalPercent
	}
	existing.UpdatedAt = time.Now().UTC()
	r.quotas[quota.TenantID] = existing
	return existing, nil
}

func (r *MemoryRepository) Overview(_ context.Context, tenantID string) (Overview, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	overview := Overview{ProviderBytes: map[string]int64{}, Quota: r.ensureQuota(tenantID, 0)}
	for _, file := range r.files {
		if tenantID != "" && file.TenantID != tenantID {
			continue
		}
		switch file.Status {
		case StatusActive:
			overview.TotalFiles++
			overview.TotalBytes += file.FileSize
			overview.ProviderBytes[file.Provider] += file.FileSize
			if file.IsTemporary {
				overview.TemporaryBytes += file.FileSize
			}
		case StatusPendingUpload:
			overview.PendingFiles++
		case StatusDeletePending:
			overview.RecycleFiles++
		case StatusUploadFailed, StatusQuarantined:
			overview.AbnormalFiles++
		}
	}
	return overview, nil
}

func (r *MemoryRepository) CreateMultipartSession(_ context.Context, session MultipartUploadRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.multiparts[session.ID]; exists {
		return ErrUploadConfirmFailed
	}
	if session.Parts == nil {
		session.Parts = map[int]CompletedPart{}
	}
	r.multiparts[session.ID] = cloneMultipart(session)
	return nil
}

func (r *MemoryRepository) GetMultipartSession(_ context.Context, tenantID, userID, uploadID string) (MultipartUploadRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.multiparts[uploadID]
	if !ok || session.TenantID != tenantID || session.OwnerUserID != userID {
		return MultipartUploadRecord{}, ErrMultipartNotFound
	}
	return cloneMultipart(session), nil
}

func (r *MemoryRepository) GetMultipartSessionByIdempotency(_ context.Context, tenantID, userID, key string) (MultipartUploadRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key = strings.TrimSpace(key)
	if key == "" {
		return MultipartUploadRecord{}, ErrMultipartNotFound
	}
	for _, session := range r.multiparts {
		if session.TenantID == tenantID && session.OwnerUserID == userID && session.IdempotencyKey == key {
			return cloneMultipart(session), nil
		}
	}
	return MultipartUploadRecord{}, ErrMultipartNotFound
}

func (r *MemoryRepository) SaveMultipartPart(_ context.Context, uploadID string, part CompletedPart) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.multiparts[uploadID]
	if !ok {
		return ErrMultipartNotFound
	}
	if session.Parts == nil {
		session.Parts = map[int]CompletedPart{}
	}
	session.Parts[part.PartNumber] = part
	if session.State == MultipartStateInitialized {
		session.State = MultipartStateUploading
	}
	r.multiparts[uploadID] = session
	return nil
}

func (r *MemoryRepository) UpdateMultipartState(_ context.Context, tenantID, uploadID, state string, completedAt *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.multiparts[uploadID]
	if !ok || (tenantID != "" && session.TenantID != tenantID) {
		return ErrMultipartNotFound
	}
	session.State = state
	if completedAt != nil {
		session.CompletedAt = completedAt
	}
	r.multiparts[uploadID] = session
	return nil
}

func cloneMultipart(session MultipartUploadRecord) MultipartUploadRecord {
	copy := session
	if session.Parts != nil {
		copy.Parts = make(map[int]CompletedPart, len(session.Parts))
		for key, value := range session.Parts {
			copy.Parts[key] = value
		}
	}
	return copy
}

func cloneFile(file FileObject) FileObject {
	copy := file
	if file.Metadata != nil {
		copy.Metadata = map[string]any{}
		for key, value := range file.Metadata {
			copy.Metadata[key] = value
		}
	}
	return copy
}

func stringMapToAny(input map[string]string) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
