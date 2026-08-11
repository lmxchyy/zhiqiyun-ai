package smartvideo

import (
	"context"
	"errors"
	"testing"
)

type stubFileResolver struct{ files map[string]FileReference }

func (r stubFileResolver) ResolveFile(_ context.Context, _ Access, id string) (FileReference, error) {
	file, ok := r.files[id]
	if !ok {
		return FileReference{}, ErrNotFound
	}
	return file, nil
}

func newTestService() (*Service, *MemoryRepository, Access) {
	access := Access{TenantID: "tenant_a", UserID: "user_a"}
	repository := NewMemoryRepository()
	files := stubFileResolver{files: map[string]FileReference{
		"file_1": {
			FileID: "file_1", TenantID: access.TenantID, UserID: access.UserID,
			ObjectKey: "tenant_a/user_a/video.mp4", Status: "ACTIVE",
			Metadata: AssetMetadata{OriginalName: "video.mp4", MIMEType: "video/mp4", FileSize: 1024, DurationMS: 1000},
		},
		"file_2": {
			FileID: "file_2", TenantID: access.TenantID, UserID: access.UserID,
			ObjectKey: "tenant_a/user_a/image.png", Status: "ACTIVE",
			Metadata: AssetMetadata{OriginalName: "image.png", MIMEType: "image/png", FileSize: 512},
		},
		"file_other_tenant": {
			FileID: "file_other_tenant", TenantID: "tenant_b", UserID: access.UserID,
			ObjectKey: "tenant_b/user_a/video.mp4", Status: "ACTIVE",
		},
		"file_other_user": {
			FileID: "file_other_user", TenantID: access.TenantID, UserID: "user_b",
			ObjectKey: "tenant_a/user_b/video.mp4", Status: "ACTIVE",
		},
		"file_inactive": {
			FileID: "file_inactive", TenantID: access.TenantID, UserID: access.UserID,
			ObjectKey: "tenant_a/user_a/inactive.mp4", Status: "DELETED",
		},
	}}
	return NewService(repository, files), repository, access
}

func TestAssetRejectsUnauthorizedAndInactiveFiles(t *testing.T) {
	service, _, access := newTestService()
	project, err := service.CreateProject(context.Background(), access, CreateProjectInput{Title: "Project"})
	if err != nil {
		t.Fatal(err)
	}
	for _, fileID := range []string{"file_other_tenant", "file_other_user", "file_inactive"} {
		_, err := service.CreateAsset(context.Background(), access, project.ID, CreateAssetInput{
			FileID: fileID, AssetType: AssetTypeVideo,
		})
		if !errors.Is(err, ErrFileNotReady) {
			t.Fatalf("file %s error = %v, want ErrFileNotReady", fileID, err)
		}
	}
}

func TestProjectTenantAndUserIsolation(t *testing.T) {
	service, _, owner := newTestService()
	project, err := service.CreateProject(context.Background(), owner, CreateProjectInput{Title: "Project"})
	if err != nil {
		t.Fatal(err)
	}
	for _, access := range []Access{
		{TenantID: "tenant_b", UserID: owner.UserID},
		{TenantID: owner.TenantID, UserID: "user_b"},
	} {
		if _, err := service.GetProject(context.Background(), access, project.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("cross-scope access error = %v, want ErrNotFound", err)
		}
	}
}

func TestAssetUsesOwnedActiveFileAndStoresObjectKey(t *testing.T) {
	service, _, access := newTestService()
	project, err := service.CreateProject(context.Background(), access, CreateProjectInput{Title: "Project"})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := service.CreateAsset(context.Background(), access, project.ID, CreateAssetInput{
		FileID: "file_1", AssetType: AssetTypeVideo,
	})
	if err != nil {
		t.Fatal(err)
	}
	if asset.StorageKey != "tenant_a/user_a/video.mp4" || asset.Metadata.DurationMS != 1000 {
		t.Fatalf("unexpected asset: %+v", asset)
	}
}

func TestInvalidRenderStateTransitionRejected(t *testing.T) {
	for _, transition := range [][2]string{
		{RenderStatusCreated, RenderStatusSucceeded},
		{RenderStatusQueued, RenderStatusUploading},
		{RenderStatusSucceeded, RenderStatusProcessing},
		{RenderStatusFailed, RenderStatusQueued},
	} {
		if err := ValidateRenderTransition(transition[0], transition[1]); !errors.Is(err, ErrInvalidStateTransition) {
			t.Fatalf("%s -> %s error = %v", transition[0], transition[1], err)
		}
	}
	if err := ValidateRenderTransition(RenderStatusCreated, RenderStatusQueued); err != nil {
		t.Fatalf("valid transition rejected: %v", err)
	}
}

func TestInvalidProjectStateTransitionRejected(t *testing.T) {
	if err := ValidateProjectTransition(ProjectStatusDraft, ProjectStatusCompleted); !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("invalid project transition error = %v", err)
	}
	if err := ValidateProjectTransition(ProjectStatusDraft, ProjectStatusAnalyzing); err != nil {
		t.Fatalf("valid project transition rejected: %v", err)
	}
}

func TestLegacyCreateRenderTaskIsDisabled(t *testing.T) {
	service, repository, access := newTestService()
	project, err := service.CreateProject(context.Background(), access, CreateProjectInput{Title: "Project"})
	if err != nil {
		t.Fatal(err)
	}
	project.Status = ProjectStatusConfirmed
	if _, err := repository.UpdateProject(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	_, err = service.CreateRenderTask(context.Background(), access, project.ID, CreateRenderTaskInput{
		ClientRequestID: "request_1",
		Specification: RenderSpecification{
			Width: 1920, Height: 1080, FrameRate: 30, Format: "mp4",
		},
	})
	if !errors.Is(err, ErrExportNotReady) {
		t.Fatalf("error = %v, want ErrExportNotReady", err)
	}
}

func TestLegacyRetryRenderTaskIsDisabled(t *testing.T) {
	service, _, access := newTestService()
	_, err := service.RetryRenderTask(context.Background(), access, "proj", "task")
	if !errors.Is(err, ErrExportNotReady) {
		t.Fatalf("error = %v, want ErrExportNotReady", err)
	}
}
