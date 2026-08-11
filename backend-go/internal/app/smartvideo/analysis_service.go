package smartvideo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

type AnalysisOptions struct {
	Enabled     bool
	MaxAttempts int
}

type AnalysisService struct {
	repository Repository
	queue      AnalysisQueue
	options    AnalysisOptions
}

func NewAnalysisService(repository Repository, queue AnalysisQueue, options AnalysisOptions) *AnalysisService {
	if options.MaxAttempts <= 0 {
		options.MaxAttempts = 3
	}
	return &AnalysisService{repository: repository, queue: queue, options: options}
}

func (s *AnalysisService) RequestProjectAnalysis(ctx context.Context, access Access, projectID, clientRequestID string) (AnalysisSummary, error) {
	if !s.options.Enabled {
		return AnalysisSummary{}, ErrAnalysisDisabled
	}
	if s.repository == nil {
		return AnalysisSummary{}, ErrAnalysisNotReady
	}
	projectID = strings.TrimSpace(projectID)
	clientRequestID = strings.TrimSpace(clientRequestID)
	if clientRequestID == "" {
		return AnalysisSummary{}, ErrIdempotencyKeyRequired
	}
	project, err := s.repository.GetProject(ctx, access, projectID)
	if err != nil {
		return AnalysisSummary{}, err
	}
	assets, err := s.repository.ListAssets(ctx, access, projectID)
	if err != nil {
		return AnalysisSummary{}, err
	}
	if len(assets) < MinProjectAssets {
		return AnalysisSummary{}, fmt.Errorf("%w: need at least %d assets", ErrInvalidInput, MinProjectAssets)
	}
	if project.Status == ProjectStatusDraft || project.Status == ProjectStatusFailed || project.Status == ProjectStatusMaterialReady {
		project.Status = ProjectStatusAnalyzing
		project.UpdatedAt = time.Now().UTC()
		if _, err := s.repository.UpdateProject(ctx, project); err != nil {
			return AnalysisSummary{}, err
		}
	}
	for _, asset := range assets {
		fingerprint := SourceFingerprint(asset)
		if asset.AnalysisStatus == AnalysisStatusSucceeded && asset.SourceFingerprint == fingerprint {
			continue
		}
		task, err := s.repository.EnsureAnalysisTask(ctx, access, asset, fingerprint, clientRequestID, s.options.MaxAttempts)
		if err != nil {
			return AnalysisSummary{}, err
		}
		if task.Status == AnalysisStatusQueued || task.Status == AnalysisStatusRunning || task.Status == AnalysisStatusSucceeded || task.Status == AnalysisStatusFailed {
			continue
		}
		payload, _ := json.Marshal(map[string]string{
			"taskId": task.ID, "projectId": task.ProjectID, "assetId": task.AssetID,
		})
		if err := s.repository.EnqueueAnalysisTaskWithOutbox(ctx, task, OutboxEvent{
			TenantID: task.TenantID, AggregateType: "analysis", AggregateID: task.ID,
			EventType: "enqueue_requested", Payload: payload,
		}); err != nil {
			return AnalysisSummary{}, err
		}
	}
	return s.GetProjectAnalysis(ctx, access, projectID)
}

func (s *AnalysisService) GetProjectAnalysis(ctx context.Context, access Access, projectID string) (AnalysisSummary, error) {
	projectID = strings.TrimSpace(projectID)
	if _, err := s.repository.GetProject(ctx, access, projectID); err != nil {
		return AnalysisSummary{}, err
	}
	assets, err := s.repository.ListAssets(ctx, access, projectID)
	if err != nil {
		return AnalysisSummary{}, err
	}
	summary := AnalysisSummary{ProjectID: projectID, TotalAssets: len(assets), Assets: make([]AnalysisAssetStatus, 0, len(assets))}
	for _, asset := range assets {
		status := asset.AnalysisStatus
		if status == "" {
			status = AnalysisStatusPending
		}
		item := AnalysisAssetStatus{
			AssetID: asset.ID, FileID: asset.FileID, AssetType: asset.AssetType, Status: status,
			AttemptCount: asset.AttemptCount, Metadata: asset.NormalizedMetadata,
			ThumbnailFileID: asset.ThumbnailFileID, ProxyFileID: asset.ProxyFileID,
			ErrorCode: asset.ErrorCode, ErrorMessage: asset.SanitizedErrorMessage,
			AnalyzerVersion: asset.AnalyzerVersion, StartedAt: asset.AnalysisStartedAt, FinishedAt: asset.AnalysisFinishedAt,
		}
		summary.Assets = append(summary.Assets, item)
		switch status {
		case AnalysisStatusSucceeded:
			summary.SucceededCount++
		case AnalysisStatusFailed:
			summary.FailedCount++
		case AnalysisStatusRunning:
			summary.RunningCount++
		case AnalysisStatusQueued:
			summary.PendingCount++
		default:
			summary.PendingCount++
		}
	}
	summary.OverallStatus = overallAnalysisStatus(summary)
	return summary, nil
}

func (s *AnalysisService) RetryAsset(ctx context.Context, access Access, projectID, assetID string) (AnalysisSummary, error) {
	if !s.options.Enabled {
		return AnalysisSummary{}, ErrAnalysisDisabled
	}
	if s.repository == nil {
		return AnalysisSummary{}, ErrAnalysisNotReady
	}
	task, err := s.repository.RetryAnalysisTask(ctx, access, strings.TrimSpace(projectID), strings.TrimSpace(assetID))
	if err != nil {
		return AnalysisSummary{}, err
	}
	payload, _ := json.Marshal(map[string]string{
		"taskId": task.ID, "projectId": task.ProjectID, "assetId": task.AssetID,
	})
	if err := s.repository.EnqueueAnalysisTaskWithOutbox(ctx, task, OutboxEvent{
		TenantID: task.TenantID, AggregateType: "analysis", AggregateID: task.ID,
		EventType: "enqueue_requested", Payload: payload,
	}); err != nil {
		return AnalysisSummary{}, err
	}
	return s.GetProjectAnalysis(ctx, access, projectID)
}

func SourceFingerprint(asset ProjectAsset) string {
	raw := strings.Join([]string{
		strings.TrimSpace(asset.FileID), strings.TrimSpace(asset.StorageKey),
		strings.TrimSpace(asset.Metadata.FileHash), strings.TrimSpace(asset.Metadata.MIMEType),
		formatInt64(asset.Metadata.FileSize),
	}, "\x00")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func overallAnalysisStatus(summary AnalysisSummary) string {
	if summary.TotalAssets == 0 || summary.PendingCount > 0 {
		return AnalysisStatusPending
	}
	if summary.RunningCount > 0 {
		return AnalysisStatusRunning
	}
	if summary.FailedCount > 0 {
		return AnalysisStatusFailed
	}
	return AnalysisStatusSucceeded
}

func formatInt64(value int64) string {
	if value == 0 {
		return "0"
	}
	const digits = "0123456789"
	negative := value < 0
	if negative {
		value = -value
	}
	var raw [20]byte
	index := len(raw)
	for value > 0 {
		index--
		raw[index] = digits[value%10]
		value /= 10
	}
	if negative {
		index--
		raw[index] = '-'
	}
	return string(raw[index:])
}

type MemoryAnalysisQueue struct {
	mu   sync.Mutex
	jobs []AnalysisJob
}

func NewMemoryAnalysisQueue() *MemoryAnalysisQueue { return &MemoryAnalysisQueue{} }

func (q *MemoryAnalysisQueue) Enqueue(_ context.Context, job AnalysisJob, _ time.Duration) error {
	if strings.TrimSpace(job.TaskID) == "" {
		return ErrInvalidInput
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, existing := range q.jobs {
		if existing.TaskID == job.TaskID {
			return nil
		}
	}
	q.jobs = append(q.jobs, job)
	return nil
}

func (q *MemoryAnalysisQueue) Jobs() []AnalysisJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]AnalysisJob{}, q.jobs...)
}
