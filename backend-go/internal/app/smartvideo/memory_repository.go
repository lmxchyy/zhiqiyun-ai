package smartvideo

import (
	"context"
	"sort"
	"sync"
	"time"
)

type MemoryRepository struct {
	mu       sync.RWMutex
	projects map[string]Project
	assets   map[string]ProjectAsset
	tasks    map[string]RenderTask
	analysis map[string]AnalysisTask
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		projects: map[string]Project{},
		assets:   map[string]ProjectAsset{},
		tasks:    map[string]RenderTask{},
		analysis: map[string]AnalysisTask{},
	}
}

func (r *MemoryRepository) CreateProject(_ context.Context, project Project) (Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.projects[project.ID] = project
	return project, nil
}

func (r *MemoryRepository) GetProject(_ context.Context, access Access, id string) (Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	project, ok := r.projects[id]
	if !ok || project.DeletedAt != nil || project.TenantID != access.TenantID || project.UserID != access.UserID {
		return Project{}, ErrNotFound
	}
	return project, nil
}

func (r *MemoryRepository) ListProjects(_ context.Context, access Access) ([]Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := []Project{}
	for _, item := range r.projects {
		if item.DeletedAt == nil && item.TenantID == access.TenantID && item.UserID == access.UserID {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items, nil
}

func (r *MemoryRepository) UpdateProject(_ context.Context, project Project) (Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.projects[project.ID]
	if !ok || current.DeletedAt != nil || current.TenantID != project.TenantID || current.UserID != project.UserID {
		return Project{}, ErrNotFound
	}
	r.projects[project.ID] = project
	return project, nil
}

func (r *MemoryRepository) SoftDeleteProject(_ context.Context, access Access, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	project, ok := r.projects[id]
	if !ok || project.DeletedAt != nil || project.TenantID != access.TenantID || project.UserID != access.UserID {
		return ErrNotFound
	}
	now := time.Now().UTC()
	project.DeletedAt, project.UpdatedAt = &now, now
	r.projects[id] = project
	return nil
}

func (r *MemoryRepository) CreateAsset(_ context.Context, asset ProjectAsset) (ProjectAsset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.assets {
		if item.ProjectID == asset.ProjectID && item.FileID == asset.FileID {
			return item, nil
		}
	}
	r.assets[asset.ID] = asset
	return asset, nil
}

func (r *MemoryRepository) GetAsset(_ context.Context, access Access, projectID, assetID string) (ProjectAsset, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.assets[assetID]
	if !ok || item.ProjectID != projectID || item.TenantID != access.TenantID || item.UserID != access.UserID {
		return ProjectAsset{}, ErrNotFound
	}
	return item, nil
}

func (r *MemoryRepository) ListAssets(_ context.Context, access Access, projectID string) ([]ProjectAsset, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := []ProjectAsset{}
	for _, item := range r.assets {
		if item.ProjectID == projectID && item.TenantID == access.TenantID && item.UserID == access.UserID {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].SortOrder == items[j].SortOrder {
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		}
		return items[i].SortOrder < items[j].SortOrder
	})
	return items, nil
}

func (r *MemoryRepository) ReorderAssets(ctx context.Context, access Access, projectID string, ids []string) ([]ProjectAsset, error) {
	r.mu.Lock()
	seen := map[string]bool{}
	for index, id := range ids {
		item, ok := r.assets[id]
		if !ok || item.ProjectID != projectID || item.TenantID != access.TenantID || item.UserID != access.UserID || seen[id] {
			r.mu.Unlock()
			return nil, ErrInvalidInput
		}
		seen[id] = true
		item.SortOrder, item.UpdatedAt = index, time.Now().UTC()
		r.assets[id] = item
	}
	r.mu.Unlock()
	return r.ListAssets(ctx, access, projectID)
}

func (r *MemoryRepository) DeleteAsset(_ context.Context, access Access, projectID, assetID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.assets[assetID]
	if !ok || item.ProjectID != projectID || item.TenantID != access.TenantID || item.UserID != access.UserID {
		return ErrNotFound
	}
	delete(r.assets, assetID)
	return nil
}

func (r *MemoryRepository) CreateRenderTask(_ context.Context, task RenderTask) (RenderTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.tasks {
		if item.TenantID == task.TenantID && item.UserID == task.UserID && item.ClientRequestID == task.ClientRequestID {
			return item, nil
		}
	}
	r.tasks[task.ID] = task
	return task, nil
}

func (r *MemoryRepository) GetRenderTaskByClientRequestID(_ context.Context, access Access, key string) (RenderTask, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, item := range r.tasks {
		if item.TenantID == access.TenantID && item.UserID == access.UserID && item.ClientRequestID == key {
			return item, nil
		}
	}
	return RenderTask{}, ErrNotFound
}

func (r *MemoryRepository) EnsureAnalysisTask(_ context.Context, access Access, asset ProjectAsset, fingerprint, clientRequestID string, maxAttempts int) (AnalysisTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if asset.TenantID != access.TenantID || asset.UserID != access.UserID {
		return AnalysisTask{}, ErrNotFound
	}
	for _, task := range r.analysis {
		if task.AssetID == asset.ID && task.SourceFingerprint == fingerprint {
			return task, nil
		}
	}
	now := time.Now().UTC()
	task := AnalysisTask{
		ID: newID("vat"), ProjectID: asset.ProjectID, AssetID: asset.ID,
		TenantID: asset.TenantID, UserID: asset.UserID, SourceFileID: asset.FileID,
		SourceFingerprint: fingerprint, ClientRequestID: clientRequestID,
		Status: AnalysisStatusPending, MaxAttempts: maxAttempts, RunAfter: now,
		CreatedAt: now, UpdatedAt: now,
	}
	r.analysis[task.ID] = task
	asset.AnalysisStatus = AnalysisStatusPending
	asset.SourceFingerprint = fingerprint
	asset.NormalizedMetadata = nil
	asset.FilteredProbeResult = nil
	asset.ThumbnailFileID = ""
	asset.ProxyFileID = ""
	asset.ErrorCode = ""
	asset.SanitizedErrorMessage = ""
	asset.UpdatedAt = now
	r.assets[asset.ID] = asset
	return task, nil
}

func (r *MemoryRepository) GetAnalysisTask(_ context.Context, id string) (AnalysisTask, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, ok := r.analysis[id]
	if !ok {
		return AnalysisTask{}, ErrNotFound
	}
	return task, nil
}

func (r *MemoryRepository) ListAnalysisTasks(_ context.Context, access Access, projectID string) ([]AnalysisTask, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := []AnalysisTask{}
	for _, task := range r.analysis {
		if task.ProjectID == projectID && task.TenantID == access.TenantID && task.UserID == access.UserID {
			items = append(items, task)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	return items, nil
}

func (r *MemoryRepository) MarkAnalysisQueued(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.analysis[id]
	if !ok {
		return ErrNotFound
	}
	if task.Status == AnalysisStatusPending {
		task.Status = AnalysisStatusQueued
		task.UpdatedAt = time.Now().UTC()
		r.analysis[id] = task
		asset := r.assets[task.AssetID]
		asset.AnalysisStatus, asset.UpdatedAt = AnalysisStatusQueued, task.UpdatedAt
		r.assets[asset.ID] = asset
	}
	return nil
}

func (r *MemoryRepository) AcquireAnalysisTask(_ context.Context, id, workerID string, lease time.Duration) (AnalysisTask, ProjectAsset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.analysis[id]
	if !ok {
		return AnalysisTask{}, ProjectAsset{}, ErrNotFound
	}
	now := time.Now().UTC()
	eligible := (task.Status == AnalysisStatusPending || task.Status == AnalysisStatusQueued) && !task.RunAfter.After(now)
	if task.Status == AnalysisStatusRunning && task.LeaseExpiresAt != nil && !task.LeaseExpiresAt.After(now) {
		eligible = true
	}
	if eligible && task.AttemptCount >= task.MaxAttempts {
		task.Status = AnalysisStatusFailed
		task.ErrorCode = MediaErrorLeaseExpired
		task.SanitizedErrorMessage = "分析任务租约已过期，且重试次数已用尽"
		task.FinishedAt = &now
		task.UpdatedAt = now
		r.analysis[id] = task
		asset := r.assets[task.AssetID]
		asset.AnalysisStatus = AnalysisStatusFailed
		asset.ErrorCode = task.ErrorCode
		asset.SanitizedErrorMessage = task.SanitizedErrorMessage
		asset.AnalysisFinishedAt = &now
		asset.UpdatedAt = now
		r.assets[asset.ID] = asset
		return AnalysisTask{}, ProjectAsset{}, ErrAnalysisNotReady
	}
	if !eligible {
		return AnalysisTask{}, ProjectAsset{}, ErrAnalysisNotReady
	}
	expires := now.Add(lease)
	task.Status, task.LeaseOwner, task.LeaseExpiresAt, task.HeartbeatAt = AnalysisStatusRunning, workerID, &expires, &now
	task.AttemptCount++
	task.StartedAt = &now
	task.UpdatedAt = now
	r.analysis[id] = task
	asset := r.assets[task.AssetID]
	asset.AnalysisStatus, asset.AttemptCount, asset.AnalysisStartedAt, asset.UpdatedAt = AnalysisStatusRunning, task.AttemptCount, &now, now
	r.assets[asset.ID] = asset
	return task, asset, nil
}

func (r *MemoryRepository) HeartbeatAnalysisTask(_ context.Context, id, workerID string, lease time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.analysis[id]
	if !ok {
		return ErrNotFound
	}
	if task.Status != AnalysisStatusRunning || task.LeaseOwner != workerID {
		return ErrAnalysisLeaseLost
	}
	now, expires := time.Now().UTC(), time.Now().UTC().Add(lease)
	task.HeartbeatAt, task.LeaseExpiresAt, task.UpdatedAt = &now, &expires, now
	r.analysis[id] = task
	return nil
}

func (r *MemoryRepository) CompleteAnalysisTask(_ context.Context, id, workerID string, result AnalysisResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.analysis[id]
	if !ok {
		return ErrNotFound
	}
	if task.Status != AnalysisStatusRunning || task.LeaseOwner != workerID {
		return ErrAnalysisLeaseLost
	}
	now := time.Now().UTC()
	task.Status, task.AnalyzerVersion = AnalysisStatusSucceeded, result.AnalyzerVersion
	task.ErrorCode, task.SanitizedErrorMessage = "", ""
	task.FinishedAt, task.UpdatedAt = &now, now
	task.LeaseOwner, task.LeaseExpiresAt, task.HeartbeatAt = "", nil, nil
	r.analysis[id] = task
	asset := r.assets[task.AssetID]
	asset.AnalysisStatus = AnalysisStatusSucceeded
	asset.NormalizedMetadata = &result.Metadata
	asset.FilteredProbeResult = &result.FilteredProbeResult
	asset.ThumbnailFileID, asset.ProxyFileID = result.ThumbnailFileID, result.ProxyFileID
	asset.AnalyzerVersion, asset.ErrorCode, asset.SanitizedErrorMessage = result.AnalyzerVersion, "", ""
	asset.AnalysisFinishedAt, asset.UpdatedAt = &now, now
	r.assets[asset.ID] = asset
	return nil
}

func (r *MemoryRepository) FailAnalysisTask(_ context.Context, id, workerID, code, message string, retryAt time.Time, final bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.analysis[id]
	if !ok {
		return ErrNotFound
	}
	if task.Status != AnalysisStatusRunning || task.LeaseOwner != workerID {
		return ErrAnalysisLeaseLost
	}
	now := time.Now().UTC()
	task.Status = AnalysisStatusQueued
	if final || task.AttemptCount >= task.MaxAttempts {
		task.Status, task.FinishedAt = AnalysisStatusFailed, &now
	}
	task.ErrorCode, task.SanitizedErrorMessage = code, message
	task.RunAfter, task.UpdatedAt = retryAt, now
	task.LeaseOwner, task.LeaseExpiresAt, task.HeartbeatAt = "", nil, nil
	r.analysis[id] = task
	asset := r.assets[task.AssetID]
	asset.AnalysisStatus, asset.AttemptCount = task.Status, task.AttemptCount
	asset.ErrorCode, asset.SanitizedErrorMessage = code, message
	if task.Status == AnalysisStatusFailed {
		asset.AnalysisFinishedAt = &now
	}
	asset.UpdatedAt = now
	r.assets[asset.ID] = asset
	return nil
}

func (r *MemoryRepository) RetryAnalysisTask(_ context.Context, access Access, projectID, assetID string) (AnalysisTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, task := range r.analysis {
		if task.ProjectID != projectID || task.AssetID != assetID || task.TenantID != access.TenantID || task.UserID != access.UserID {
			continue
		}
		if task.Status != AnalysisStatusFailed {
			return AnalysisTask{}, ErrAnalysisNotFailed
		}
		now := time.Now().UTC()
		task.Status, task.AttemptCount, task.RunAfter = AnalysisStatusPending, 0, now
		task.ErrorCode, task.SanitizedErrorMessage = "", ""
		task.StartedAt, task.FinishedAt, task.UpdatedAt = nil, nil, now
		r.analysis[id] = task
		asset := r.assets[assetID]
		asset.AnalysisStatus, asset.AttemptCount = AnalysisStatusPending, 0
		asset.ErrorCode, asset.SanitizedErrorMessage = "", ""
		asset.AnalysisStartedAt, asset.AnalysisFinishedAt, asset.UpdatedAt = nil, nil, now
		r.assets[assetID] = asset
		return task, nil
	}
	return AnalysisTask{}, ErrNotFound
}
