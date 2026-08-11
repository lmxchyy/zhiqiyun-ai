package smartvideo

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type RevisePlanInput struct {
	Plan       EditPlanV1
	ChangeNote string
}

func (s *PlanService) ListVersions(ctx context.Context, access Access, projectID string) ([]ProjectVersion, error) {
	if s == nil || s.versions == nil || s.projects == nil {
		return nil, ErrPlanNotReady
	}
	if _, err := s.projects.GetProject(ctx, access, strings.TrimSpace(projectID)); err != nil {
		return nil, err
	}
	return s.versions.ListVersions(ctx, access, strings.TrimSpace(projectID))
}

func (s *PlanService) GetVersion(ctx context.Context, access Access, projectID, versionID string) (ProjectVersion, error) {
	if s == nil || s.versions == nil {
		return ProjectVersion{}, ErrPlanNotReady
	}
	return s.versions.GetVersion(ctx, access, strings.TrimSpace(projectID), strings.TrimSpace(versionID))
}

func (s *PlanService) RevisePlan(ctx context.Context, access Access, projectID, parentVersionID string, input RevisePlanInput) (ProjectVersion, error) {
	if s == nil || s.projects == nil || s.versions == nil {
		return ProjectVersion{}, ErrPlanNotReady
	}
	projectID = strings.TrimSpace(projectID)
	parentVersionID = strings.TrimSpace(parentVersionID)
	project, err := s.projects.GetProject(ctx, access, projectID)
	if err != nil {
		return ProjectVersion{}, err
	}
	switch project.Status {
	case ProjectStatusStoryboardReady, ProjectStatusConfirmed:
	default:
		return ProjectVersion{}, fmt.Errorf("%w: project status %s cannot revise", ErrInvalidStateTransition, project.Status)
	}
	parent, err := s.versions.GetVersion(ctx, access, projectID, parentVersionID)
	if err != nil {
		return ProjectVersion{}, err
	}
	assets, err := s.projects.ListAssets(ctx, access, projectID)
	if err != nil {
		return ProjectVersion{}, err
	}
	plan := input.Plan
	if err := requireOwnedPlan(&plan, assets); err != nil {
		return ProjectVersion{}, err
	}
	if err := ValidateChangeNote(input.ChangeNote); err != nil {
		return ProjectVersion{}, err
	}

	now := s.now()
	version := ProjectVersion{
		ID: newID("svv"), ProjectID: project.ID, TenantID: project.TenantID,
		VersionNumber: project.CurrentVersion + 1, Source: VersionSourceUser,
		ParentVersionID: parent.ID, PlanSchemaVersion: EditPlanSchemaVersion,
		PlanSnapshot: plan, ChangeNote: strings.TrimSpace(input.ChangeNote),
		Requirement: project.Requirement, Status: "GENERATED",
		CreatedBy: access.UserID, CreatedAt: now,
	}
	created, err := s.versions.CreateImmutableVersion(ctx, version)
	if err != nil {
		return ProjectVersion{}, err
	}

	if project.Status == ProjectStatusConfirmed {
		if err := ValidateProjectTransition(project.Status, ProjectStatusStoryboardReady); err != nil {
			return ProjectVersion{}, err
		}
	}
	project.Status = ProjectStatusStoryboardReady
	project.CurrentVersion = created.VersionNumber
	project.CurrentVersionID = created.ID
	project.UpdatedAt = now
	if _, err := s.projects.UpdateProject(ctx, project); err != nil {
		return ProjectVersion{}, err
	}
	return created, nil
}

func (s *PlanService) ConfirmPlan(ctx context.Context, access Access, projectID, versionID string) (Project, ProjectVersion, error) {
	if s == nil || s.projects == nil || s.versions == nil {
		return Project{}, ProjectVersion{}, ErrPlanNotReady
	}
	projectID = strings.TrimSpace(projectID)
	versionID = strings.TrimSpace(versionID)
	project, err := s.projects.GetProject(ctx, access, projectID)
	if err != nil {
		return Project{}, ProjectVersion{}, err
	}
	switch project.Status {
	case ProjectStatusStoryboardReady, ProjectStatusConfirmed:
	case ProjectStatusRendering, ProjectStatusCompleted:
		// Confirm is idempotent once the version is already locked.
		if strings.TrimSpace(project.ConfirmedVersionID) == versionID {
			version, err := s.versions.GetVersion(ctx, access, projectID, versionID)
			if err != nil {
				return Project{}, ProjectVersion{}, err
			}
			return project, version, nil
		}
		return Project{}, ProjectVersion{}, fmt.Errorf("%w: project status %s cannot confirm", ErrInvalidStateTransition, project.Status)
	default:
		return Project{}, ProjectVersion{}, fmt.Errorf("%w: project status %s cannot confirm", ErrInvalidStateTransition, project.Status)
	}
	version, err := s.versions.GetVersion(ctx, access, projectID, versionID)
	if err != nil {
		return Project{}, ProjectVersion{}, err
	}
	assets, err := s.projects.ListAssets(ctx, access, projectID)
	if err != nil {
		return Project{}, ProjectVersion{}, err
	}
	plan := version.PlanSnapshot
	if err := requireOwnedPlan(&plan, assets); err != nil {
		return Project{}, ProjectVersion{}, err
	}
	version.PlanSnapshot = plan
	manifest, err := CompileRenderManifest(RenderManifestInput{Version: version, Assets: assets})
	if err != nil {
		return Project{}, ProjectVersion{}, err
	}
	if err := ensureManifestStable(manifest); err != nil {
		return Project{}, ProjectVersion{}, err
	}
	confirmed, err := s.versions.AttachRenderManifest(ctx, access, projectID, versionID, manifest, manifest.ManifestHash)
	if err != nil {
		return Project{}, ProjectVersion{}, err
	}
	project, err = s.projects.GetProject(ctx, access, projectID)
	if err != nil {
		return Project{}, ProjectVersion{}, err
	}
	return project, confirmed, nil
}

func (s *PlanService) EstimateRender(ctx context.Context, access Access, projectID, versionID string) (RenderQuote, error) {
	if s == nil || s.versions == nil || s.projects == nil {
		return RenderQuote{}, ErrPlanNotReady
	}
	if _, err := s.projects.GetProject(ctx, access, strings.TrimSpace(projectID)); err != nil {
		return RenderQuote{}, err
	}
	version, err := s.versions.GetVersion(ctx, access, strings.TrimSpace(projectID), strings.TrimSpace(versionID))
	if err != nil {
		return RenderQuote{}, err
	}
	plan := version.PlanSnapshot
	if plan.SchemaVersion == 0 {
		return RenderQuote{}, &EditPlanValidationError{Code: "missing_plan", Message: "version has no plan snapshot"}
	}
	quote := EstimateRenderQuote(RenderQuoteInput{
		AspectRatio: plan.Target.AspectRatio,
		Resolution:  plan.Target.Resolution,
		DurationMs:  plan.Target.DurationMs,
		Voice:       plan.Voice.Enabled,
	}, time.Now().UTC())
	return quote, nil
}
