package smartvideo

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedStoryboardProject(t *testing.T, repo *MemoryRepository) (Access, Project, ProjectVersion) {
	t.Helper()
	access := Access{TenantID: "tenant_a", UserID: "user_a"}
	now := time.Now().UTC()
	project := Project{
		ID: "vp_1", TenantID: access.TenantID, UserID: access.UserID,
		Title: "混剪", Requirement: "做一条宣传片", Status: ProjectStatusStoryboardReady,
		CurrentVersion: 1, CurrentVersionID: "svv_1", CreatedAt: now, UpdatedAt: now,
	}
	if _, err := repo.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	assets := ownedAssets()
	for _, asset := range assets {
		asset.ProjectID = project.ID
		asset.TenantID = access.TenantID
		asset.UserID = access.UserID
		asset.FileID = "file_" + asset.ID
		asset.StorageKey = "obj/" + asset.ID
		if _, err := repo.CreateAsset(context.Background(), asset); err != nil {
			t.Fatalf("create asset: %v", err)
		}
	}
	version := ProjectVersion{
		ID: "svv_1", ProjectID: project.ID, TenantID: access.TenantID, VersionNumber: 1,
		Source: VersionSourceAI, PlanSchemaVersion: EditPlanSchemaVersion,
		PlanSnapshot: makeValidEditPlanV1(), CreatedBy: access.UserID, CreatedAt: now,
	}
	if _, err := repo.CreateImmutableVersion(context.Background(), version); err != nil {
		t.Fatalf("create version: %v", err)
	}
	return access, project, version
}

func TestRevisePlanCreatesImmutableChildVersion(t *testing.T) {
	repo := NewMemoryRepository()
	access, project, parent := seedStoryboardProject(t, repo)
	service := NewPlanService(repo, nil, repo, nil)

	plan := makeValidEditPlanV1()
	plan.Title = "用户修订版"
	plan.Scenes[0].Narration = "欢迎观看修订版"

	child, err := service.RevisePlan(context.Background(), access, project.ID, parent.ID, RevisePlanInput{
		Plan: plan, ChangeNote: "调整开场旁白",
	})
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if child.ID == parent.ID || child.ParentVersionID != parent.ID {
		t.Fatalf("child version linkage broken: %+v", child)
	}
	if child.VersionNumber != 2 || child.Source != VersionSourceUser {
		t.Fatalf("unexpected child metadata: %+v", child)
	}
	if child.PlanSnapshot.Title != "用户修订版" {
		t.Fatalf("plan not revised: %s", child.PlanSnapshot.Title)
	}

	updated, err := repo.GetProject(context.Background(), access, project.ID)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if updated.CurrentVersionID != child.ID || updated.CurrentVersion != 2 {
		t.Fatalf("project pointer not updated: %+v", updated)
	}

	// Parent remains immutable.
	loadedParent, err := repo.GetVersion(context.Background(), access, project.ID, parent.ID)
	if err != nil {
		t.Fatalf("get parent: %v", err)
	}
	if loadedParent.PlanSnapshot.Title != "测试混剪方案" {
		t.Fatalf("parent plan mutated: %s", loadedParent.PlanSnapshot.Title)
	}
}

func TestRevisePlanRejectsUnsafeContent(t *testing.T) {
	repo := NewMemoryRepository()
	access, project, parent := seedStoryboardProject(t, repo)
	service := NewPlanService(repo, nil, repo, nil)

	plan := makeValidEditPlanV1()
	plan.Title = "contains BLOCKED_CONTENT marker"
	_, err := service.RevisePlan(context.Background(), access, project.ID, parent.ID, RevisePlanInput{Plan: plan})
	if !errors.Is(err, ErrContentSafetyRejected) {
		t.Fatalf("error = %v, want ErrContentSafetyRejected", err)
	}
}

func TestConfirmPlanCompilesManifestAndLocksVersion(t *testing.T) {
	repo := NewMemoryRepository()
	access, project, version := seedStoryboardProject(t, repo)
	service := NewPlanService(repo, nil, repo, nil)

	confirmedProject, confirmedVersion, err := service.ConfirmPlan(context.Background(), access, project.ID, version.ID)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if confirmedProject.Status != ProjectStatusConfirmed || confirmedProject.ConfirmedVersionID != version.ID {
		t.Fatalf("unexpected project after confirm: %+v", confirmedProject)
	}
	if confirmedVersion.ManifestHash == "" || confirmedVersion.RenderManifest == nil {
		t.Fatalf("manifest missing: %+v", confirmedVersion)
	}
	if confirmedVersion.RenderManifest.ManifestHash != confirmedVersion.ManifestHash {
		t.Fatalf("hash mismatch on version")
	}

	// Re-confirm with same hash is allowed; different hash is rejected.
	again, err := repo.AttachRenderManifest(context.Background(), access, project.ID, version.ID, *confirmedVersion.RenderManifest, confirmedVersion.ManifestHash)
	if err != nil {
		t.Fatalf("idempotent confirm: %v", err)
	}
	if again.ManifestHash != confirmedVersion.ManifestHash {
		t.Fatalf("hash changed on idempotent confirm")
	}
	mutated := *confirmedVersion.RenderManifest
	mutated.Output.Bitrate = "9999k"
	if _, err := repo.AttachRenderManifest(context.Background(), access, project.ID, version.ID, mutated, "different"); !errors.Is(err, ErrVersionImmutable) {
		t.Fatalf("error = %v, want ErrVersionImmutable", err)
	}
}

func TestConfirmThenReviseCreatesChildWithoutOverwritingConfirmed(t *testing.T) {
	repo := NewMemoryRepository()
	access, project, version := seedStoryboardProject(t, repo)
	service := NewPlanService(repo, nil, repo, nil)

	if _, _, err := service.ConfirmPlan(context.Background(), access, project.ID, version.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	plan := makeValidEditPlanV1()
	plan.Title = "二次修订"
	child, err := service.RevisePlan(context.Background(), access, project.ID, version.ID, RevisePlanInput{
		Plan: plan, ChangeNote: "确认后再改",
	})
	if err != nil {
		t.Fatalf("revise after confirm: %v", err)
	}
	updated, err := repo.GetProject(context.Background(), access, project.ID)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if updated.Status != ProjectStatusStoryboardReady {
		t.Fatalf("status = %s, want STORYBOARD_READY", updated.Status)
	}
	if updated.ConfirmedVersionID != version.ID {
		t.Fatalf("confirmed version overwritten: %s", updated.ConfirmedVersionID)
	}
	if updated.CurrentVersionID != child.ID {
		t.Fatalf("current version not pointing to child: %s", updated.CurrentVersionID)
	}
	parent, err := repo.GetVersion(context.Background(), access, project.ID, version.ID)
	if err != nil {
		t.Fatalf("get confirmed version: %v", err)
	}
	if parent.ManifestHash == "" {
		t.Fatal("confirmed version lost manifest")
	}
}

func TestEstimateRenderUsesServerSideRules(t *testing.T) {
	repo := NewMemoryRepository()
	access, project, version := seedStoryboardProject(t, repo)
	service := NewPlanService(repo, nil, repo, nil)

	quote, err := service.EstimateRender(context.Background(), access, project.ID, version.ID)
	if err != nil {
		t.Fatalf("estimate: %v", err)
	}
	// 30s * 2 * 1.5(1080p) * 1.2(voice) = 108
	if quote.Points != 108 {
		t.Fatalf("points = %d, want 108", quote.Points)
	}
	if quote.ExpiresAt.Before(time.Now().UTC()) {
		t.Fatalf("quote already expired: %s", quote.ExpiresAt)
	}
	direct := EstimateRenderQuote(RenderQuoteInput{
		AspectRatio: "16:9", Resolution: "720p", DurationMs: 15000, Voice: false,
	}, time.Unix(1_700_000_000, 0).UTC())
	if direct.Points != 30 { // ceil(15)*2*1
		t.Fatalf("720p silent quote = %d, want 30", direct.Points)
	}
}

func TestCompileRenderManifestIsDeterministic(t *testing.T) {
	version := ProjectVersion{PlanSnapshot: makeValidEditPlanV1()}
	assets := []ProjectAsset{}
	for id, asset := range ownedAssets() {
		asset.ID = id
		asset.FileID = "file_" + id
		asset.StorageKey = "obj/" + id
		assets = append(assets, asset)
	}
	first, err := CompileRenderManifest(RenderManifestInput{Version: version, Assets: assets})
	if err != nil {
		t.Fatalf("compile first: %v", err)
	}
	second, err := CompileRenderManifest(RenderManifestInput{Version: version, Assets: assets})
	if err != nil {
		t.Fatalf("compile second: %v", err)
	}
	if first.ManifestHash == "" || first.ManifestHash != second.ManifestHash {
		t.Fatalf("hash not stable: %s vs %s", first.ManifestHash, second.ManifestHash)
	}
	if len(first.Scenes) != 2 || len(first.Inputs) != 2 {
		t.Fatalf("unexpected manifest shape: %+v", first)
	}
}

func TestValidateEditPlanContentRejectsBlockedMarker(t *testing.T) {
	plan := makeValidEditPlanV1()
	plan.Scenes[0].Narration = "please allow BLOCKED_CONTENT"
	if err := ValidateEditPlanContent(plan); !errors.Is(err, ErrContentSafetyRejected) {
		t.Fatalf("error = %v, want ErrContentSafetyRejected", err)
	}
}
