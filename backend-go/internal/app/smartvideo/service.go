package smartvideo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	repository  Repository
	files       FileResolver
	renderQueue RenderQueue
}

func NewService(repository Repository, files FileResolver) *Service {
	return &Service{repository: repository, files: files}
}

func (s *Service) SetRenderQueue(queue RenderQueue) *Service { s.renderQueue = queue; return s }

func (s *Service) CreateProject(ctx context.Context, access Access, input CreateProjectInput) (Project, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Requirement = strings.TrimSpace(input.Requirement)
	if access.TenantID == "" || access.UserID == "" || input.Title == "" {
		return Project{}, ErrInvalidInput
	}
	now := time.Now().UTC()
	return s.repository.CreateProject(ctx, Project{
		ID: newID("vp"), TenantID: access.TenantID, UserID: access.UserID,
		Title: input.Title, Requirement: input.Requirement, Status: ProjectStatusDraft,
		CurrentVersion: 0, CreatedAt: now, UpdatedAt: now,
	})
}

func (s *Service) GetProject(ctx context.Context, access Access, id string) (Project, error) {
	return s.repository.GetProject(ctx, access, strings.TrimSpace(id))
}

func (s *Service) ListProjects(ctx context.Context, access Access) ([]Project, error) {
	return s.repository.ListProjects(ctx, access)
}

func (s *Service) UpdateProject(ctx context.Context, access Access, id string, input UpdateProjectInput) (Project, error) {
	project, err := s.repository.GetProject(ctx, access, strings.TrimSpace(id))
	if err != nil {
		return Project{}, err
	}
	if project.Status != ProjectStatusDraft && project.Status != ProjectStatusFailed {
		return Project{}, ErrInvalidStateTransition
	}
	if input.Title != nil {
		project.Title = strings.TrimSpace(*input.Title)
		if project.Title == "" {
			return Project{}, ErrInvalidInput
		}
	}
	if input.Requirement != nil {
		project.Requirement = strings.TrimSpace(*input.Requirement)
	}
	project.UpdatedAt = time.Now().UTC()
	return s.repository.UpdateProject(ctx, project)
}

func (s *Service) DeleteProject(ctx context.Context, access Access, id string) error {
	project, err := s.repository.GetProject(ctx, access, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if project.Status == ProjectStatusRendering {
		return ErrInvalidStateTransition
	}
	return s.repository.SoftDeleteProject(ctx, access, project.ID)
}

func (s *Service) CreateAsset(ctx context.Context, access Access, projectID string, input CreateAssetInput) (ProjectAsset, error) {
	project, err := s.repository.GetProject(ctx, access, strings.TrimSpace(projectID))
	if err != nil {
		return ProjectAsset{}, err
	}
	if project.Status != ProjectStatusDraft && project.Status != ProjectStatusFailed {
		return ProjectAsset{}, ErrInvalidStateTransition
	}
	input.FileID = strings.TrimSpace(input.FileID)
	input.AssetType = strings.ToUpper(strings.TrimSpace(input.AssetType))
	if input.FileID == "" || (input.AssetType != AssetTypeVideo && input.AssetType != AssetTypeImage) {
		return ProjectAsset{}, ErrInvalidInput
	}
	if s.files == nil {
		return ProjectAsset{}, ErrFileNotReady
	}
	file, err := s.files.ResolveFile(ctx, access, input.FileID)
	if err != nil {
		return ProjectAsset{}, err
	}
	if file.TenantID != access.TenantID || file.UserID != access.UserID || strings.TrimSpace(file.ObjectKey) == "" || strings.ToUpper(file.Status) != "ACTIVE" {
		return ProjectAsset{}, ErrFileNotReady
	}
	now := time.Now().UTC()
	return s.repository.CreateAsset(ctx, ProjectAsset{
		ID: newID("vpa"), ProjectID: project.ID, TenantID: access.TenantID, UserID: access.UserID,
		FileID: file.FileID, StorageKey: file.ObjectKey, AssetType: input.AssetType,
		SortOrder: input.SortOrder, Metadata: file.Metadata, AnalysisStatus: AnalysisStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
}

func (s *Service) ListAssets(ctx context.Context, access Access, projectID string) ([]ProjectAsset, error) {
	if _, err := s.repository.GetProject(ctx, access, strings.TrimSpace(projectID)); err != nil {
		return nil, err
	}
	return s.repository.ListAssets(ctx, access, strings.TrimSpace(projectID))
}

func (s *Service) ReorderAssets(ctx context.Context, access Access, projectID string, input ReorderAssetsInput) ([]ProjectAsset, error) {
	project, err := s.repository.GetProject(ctx, access, strings.TrimSpace(projectID))
	if err != nil {
		return nil, err
	}
	if project.Status != ProjectStatusDraft && project.Status != ProjectStatusFailed {
		return nil, ErrInvalidStateTransition
	}
	if len(input.AssetIDs) == 0 {
		return nil, ErrInvalidInput
	}
	return s.repository.ReorderAssets(ctx, access, project.ID, input.AssetIDs)
}

func (s *Service) DeleteAsset(ctx context.Context, access Access, projectID, assetID string) error {
	project, err := s.repository.GetProject(ctx, access, strings.TrimSpace(projectID))
	if err != nil {
		return err
	}
	if project.Status != ProjectStatusDraft && project.Status != ProjectStatusFailed {
		return ErrInvalidStateTransition
	}
	return s.repository.DeleteAsset(ctx, access, project.ID, strings.TrimSpace(assetID))
}

func (s *Service) CreateRenderTask(ctx context.Context, access Access, projectID string, input CreateRenderTaskInput) (RenderTask, error) {
	input.ClientRequestID = strings.TrimSpace(input.ClientRequestID)
	if input.ClientRequestID == "" {
		return RenderTask{}, ErrIdempotencyKeyRequired
	}
	if existing, err := s.repository.GetRenderTaskByClientRequestID(ctx, access, input.ClientRequestID); err == nil {
		if existing.ProjectID != strings.TrimSpace(projectID) ||
			existing.VersionID != strings.TrimSpace(input.VersionID) ||
			existing.Specification != input.Specification {
			return RenderTask{}, ErrInvalidInput
		}
		if existing.Status == RenderStatusCreated && s.renderQueue != nil {
			if err := s.renderQueue.Enqueue(ctx, RenderJob{TaskID: existing.ID}, 0); err != nil {
				return RenderTask{}, err
			}
			if repository, ok := s.repository.(RenderRepository); ok {
				_ = repository.MarkRenderQueued(ctx, existing.ID)
			}
		}
		return existing, nil
	} else if err != ErrNotFound {
		return RenderTask{}, err
	}
	project, err := s.repository.GetProject(ctx, access, strings.TrimSpace(projectID))
	if err != nil {
		return RenderTask{}, err
	}
	if project.Status != ProjectStatusConfirmed {
		return RenderTask{}, ErrProjectNotConfirmed
	}
	if input.Specification.Width <= 0 || input.Specification.Height <= 0 || input.Specification.FrameRate <= 0 {
		return RenderTask{}, ErrInvalidInput
	}
	if s.renderQueue != nil && (input.Specification.Width != 1080 || input.Specification.Height != 1920 ||
		input.Specification.FrameRate != 30 || input.Specification.DurationMS != 5000 ||
		strings.ToLower(input.Specification.Format) != "mp4" ||
		strings.ToLower(input.Specification.VideoCodec) != "h264" ||
		strings.ToLower(input.Specification.AudioCodec) != "aac") {
		return RenderTask{}, ErrInvalidInput
	}
	now := time.Now().UTC()
	task, err := s.repository.CreateRenderTask(ctx, RenderTask{
		ID: newID("vrt"), ProjectID: project.ID, VersionID: strings.TrimSpace(input.VersionID),
		TenantID: access.TenantID, UserID: access.UserID, ClientRequestID: input.ClientRequestID,
		Status: RenderStatusCreated, Step: "created", MaxAttempts: 3, RunAfter: now,
		Specification: input.Specification, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return RenderTask{}, err
	}
	if s.renderQueue == nil {
		return task, nil
	}
	if err = s.renderQueue.Enqueue(ctx, RenderJob{TaskID: task.ID}, 0); err != nil {
		return RenderTask{}, err
	}
	if repository, ok := s.repository.(RenderRepository); ok {
		if err = repository.MarkRenderQueued(ctx, task.ID); err != nil {
			return RenderTask{}, err
		}
		return repository.GetRenderTask(ctx, access, project.ID, task.ID)
	}
	return task, nil
}

func (s *Service) GetRenderTask(ctx context.Context, access Access, projectID, taskID string) (RenderTask, error) {
	repository, ok := s.repository.(RenderRepository)
	if !ok {
		return RenderTask{}, ErrNotFound
	}
	return repository.GetRenderTask(ctx, access, strings.TrimSpace(projectID), strings.TrimSpace(taskID))
}

func (s *Service) RetryRenderTask(ctx context.Context, access Access, projectID, taskID string) (RenderTask, error) {
	repository, ok := s.repository.(RenderRepository)
	if !ok || s.renderQueue == nil {
		return RenderTask{}, ErrAnalysisNotReady
	}
	task, err := repository.RetryRenderTask(ctx, access, strings.TrimSpace(projectID), strings.TrimSpace(taskID))
	if err != nil {
		return RenderTask{}, err
	}
	if err = s.renderQueue.Enqueue(ctx, RenderJob{TaskID: task.ID}, 0); err != nil {
		return RenderTask{}, err
	}
	return task, nil
}

func ValidateRenderTransition(from, to string) error {
	from, to = strings.ToUpper(strings.TrimSpace(from)), strings.ToUpper(strings.TrimSpace(to))
	allowed := map[string]map[string]bool{
		RenderStatusCreated:    {RenderStatusQueued: true, RenderStatusCancelled: true, RenderStatusFailed: true},
		RenderStatusQueued:     {RenderStatusProcessing: true, RenderStatusCancelled: true, RenderStatusFailed: true},
		RenderStatusProcessing: {RenderStatusRendering: true, RenderStatusCancelled: true, RenderStatusFailed: true},
		RenderStatusRendering:  {RenderStatusUploading: true, RenderStatusCancelled: true, RenderStatusFailed: true},
		RenderStatusUploading:  {RenderStatusSucceeded: true, RenderStatusFailed: true},
	}
	if !allowed[from][to] {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidStateTransition, from, to)
	}
	return nil
}

func ValidateProjectTransition(from, to string) error {
	from, to = strings.ToUpper(strings.TrimSpace(from)), strings.ToUpper(strings.TrimSpace(to))
	allowed := map[string]map[string]bool{
		ProjectStatusDraft:           {ProjectStatusAnalyzing: true},
		ProjectStatusAnalyzing:       {ProjectStatusStoryboardReady: true, ProjectStatusFailed: true},
		ProjectStatusStoryboardReady: {ProjectStatusConfirmed: true, ProjectStatusDraft: true},
		ProjectStatusConfirmed:       {ProjectStatusRendering: true, ProjectStatusDraft: true},
		ProjectStatusRendering:       {ProjectStatusCompleted: true, ProjectStatusFailed: true},
		ProjectStatusFailed:          {ProjectStatusDraft: true, ProjectStatusAnalyzing: true, ProjectStatusRendering: true},
	}
	if !allowed[from][to] {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidStateTransition, from, to)
	}
	return nil
}

func newID(prefix string) string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(raw[:])
}
