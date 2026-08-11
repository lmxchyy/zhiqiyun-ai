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
	if !CanEditAssets(project.Status) {
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
	existing, err := s.repository.ListAssets(ctx, access, project.ID)
	if err != nil {
		return ProjectAsset{}, err
	}
	if err := ValidateAssetQuota(existing, file, input.AssetType); err != nil {
		return ProjectAsset{}, err
	}
	now := time.Now().UTC()
	return s.repository.CreateAsset(ctx, ProjectAsset{
		ID: newID("vpa"), ProjectID: project.ID, TenantID: access.TenantID, UserID: access.UserID,
		FileID: file.FileID, StorageKey: file.ObjectKey, AssetType: input.AssetType,
		SortOrder: input.SortOrder, OrderIndex: input.SortOrder, Metadata: file.Metadata,
		AnalysisStatus: AnalysisStatusPending, ContentAuditStatus: "pending",
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
	if !CanEditAssets(project.Status) {
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
	if !CanEditAssets(project.Status) {
		return ErrInvalidStateTransition
	}
	return s.repository.DeleteAsset(ctx, access, project.ID, strings.TrimSpace(assetID))
}

func (s *Service) CreateRenderTask(ctx context.Context, access Access, projectID string, input CreateRenderTaskInput) (RenderTask, error) {
	_ = ctx
	_ = access
	_ = projectID
	_ = input
	// Legacy smoke enqueue path is retired. Use ExportService.CreateExport (outbox + points).
	return RenderTask{}, fmt.Errorf("%w: use ExportService.CreateExport", ErrExportNotReady)
}

func (s *Service) GetRenderTask(ctx context.Context, access Access, projectID, taskID string) (RenderTask, error) {
	repository, ok := s.repository.(RenderRepository)
	if !ok {
		return RenderTask{}, ErrNotFound
	}
	return repository.GetRenderTask(ctx, access, strings.TrimSpace(projectID), strings.TrimSpace(taskID))
}

func (s *Service) RetryRenderTask(ctx context.Context, access Access, projectID, taskID string) (RenderTask, error) {
	_ = ctx
	_ = access
	_ = projectID
	_ = taskID
	return RenderTask{}, fmt.Errorf("%w: use ExportService.RetryExport", ErrExportNotReady)
}

func ValidateRenderTransition(from, to string) error {
	from, to = strings.ToUpper(strings.TrimSpace(from)), strings.ToUpper(strings.TrimSpace(to))
	allowed := map[string]map[string]bool{
		RenderStatusCreated:      {RenderStatusQueued: true, RenderStatusCancelled: true, RenderStatusFailed: true},
		RenderStatusQueued:       {RenderStatusProcessing: true, RenderStatusCancelled: true, RenderStatusFailed: true},
		RenderStatusProcessing:   {RenderStatusSynthesizing: true, RenderStatusCancelled: true, RenderStatusFailed: true},
		RenderStatusSynthesizing: {RenderStatusRendering: true, RenderStatusCancelled: true, RenderStatusFailed: true},
		RenderStatusRendering:    {RenderStatusUploading: true, RenderStatusFailed: true},
		RenderStatusUploading:    {RenderStatusPublishing: true, RenderStatusFailed: true},
		RenderStatusPublishing:   {RenderStatusSucceeded: true, RenderStatusFailed: true},
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
		ProjectStatusAnalyzing:       {ProjectStatusMaterialReady: true, ProjectStatusFailed: true},
		ProjectStatusMaterialReady:   {ProjectStatusPlanning: true, ProjectStatusDraft: true},
		ProjectStatusPlanning:        {ProjectStatusStoryboardReady: true, ProjectStatusFailed: true},
		ProjectStatusStoryboardReady: {ProjectStatusConfirmed: true, ProjectStatusPlanning: true, ProjectStatusDraft: true},
		ProjectStatusConfirmed:       {ProjectStatusRendering: true, ProjectStatusStoryboardReady: true, ProjectStatusDraft: true},
		ProjectStatusRendering:       {ProjectStatusCompleted: true, ProjectStatusFailed: true},
		ProjectStatusCompleted:       {ProjectStatusDraft: true},
		ProjectStatusFailed:          {ProjectStatusDraft: true, ProjectStatusAnalyzing: true, ProjectStatusRendering: true, ProjectStatusPlanning: true},
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
