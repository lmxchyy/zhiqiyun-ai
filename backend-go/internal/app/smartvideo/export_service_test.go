package smartvideo

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func seedConfirmedExport(t *testing.T) (*MemoryRepository, *MemoryPointsLifecycle, *ExportService, Access, Project, ProjectVersion) {
	t.Helper()
	repo := NewMemoryRepository()
	access, project, version := seedStoryboardProject(t, repo)
	plans := NewPlanService(repo, nil, repo, nil)
	confirmedProject, confirmedVersion, err := plans.ConfirmPlan(context.Background(), access, project.ID, version.ID)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	points := NewMemoryPointsLifecycle(10_000)
	exports := NewExportService(repo, repo, repo, points)
	return repo, points, exports, access, confirmedProject, confirmedVersion
}

func TestCreateExportReservesPointsAndIsIdempotent(t *testing.T) {
	_, points, exports, access, project, version := seedConfirmedExport(t)

	first, err := exports.CreateExport(context.Background(), access, project.ID, ExportCreateInput{
		VersionID: version.ID, IdempotencyKey: "export-key-1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if first.QuotedPoints <= 0 || first.ReservedPoints != first.QuotedPoints {
		t.Fatalf("points not reserved: %+v", first)
	}
	if points.Reserved(first.ID) != first.ReservedPoints {
		t.Fatalf("ledger reserve mismatch")
	}
	if first.ManifestHash == "" || first.VersionID != version.ID {
		t.Fatalf("manifest/version missing: %+v", first)
	}

	second, err := exports.CreateExport(context.Background(), access, project.ID, ExportCreateInput{
		VersionID: version.ID, IdempotencyKey: "export-key-1",
	})
	if err != nil {
		t.Fatalf("idempotent create: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("idempotency broken: %s vs %s", first.ID, second.ID)
	}
}

func TestCreateExportRejectsInsufficientPoints(t *testing.T) {
	repo := NewMemoryRepository()
	access, project, version := seedStoryboardProject(t, repo)
	plans := NewPlanService(repo, nil, repo, nil)
	if _, _, err := plans.ConfirmPlan(context.Background(), access, project.ID, version.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	points := NewMemoryPointsLifecycle(0)
	exports := NewExportService(repo, repo, repo, points)
	_, err := exports.CreateExport(context.Background(), access, project.ID, ExportCreateInput{
		VersionID: version.ID, IdempotencyKey: "poor",
	})
	if !errors.Is(err, ErrInsufficientPoints) {
		t.Fatalf("error = %v, want ErrInsufficientPoints", err)
	}
}

func TestCancelExportReleasesPointsOnce(t *testing.T) {
	repo, points, exports, access, project, version := seedConfirmedExport(t)
	task, err := exports.CreateExport(context.Background(), access, project.ID, ExportCreateInput{
		VersionID: version.ID, IdempotencyKey: "cancel-me",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	cancelled, err := exports.CancelExport(context.Background(), access, project.ID, task.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled.Status != RenderStatusCancelled {
		t.Fatalf("status = %s", cancelled.Status)
	}
	if cancelled.ReleasedPoints != cancelled.ReservedPoints {
		t.Fatalf("released not recorded: %+v", cancelled)
	}
	if !points.IsReleased(task.ID) {
		t.Fatal("points ledger not released")
	}

	again, err := exports.CancelExport(context.Background(), access, project.ID, task.ID)
	if !errors.Is(err, ErrRenderNotCancellable) {
		t.Fatalf("second cancel error = %v, want ErrRenderNotCancellable", err)
	}
	_ = again

	stored, err := repo.GetRenderTask(context.Background(), access, project.ID, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.ReleasedPoints != stored.ReservedPoints {
		t.Fatalf("released points mutated on second cancel attempt: %+v", stored)
	}
}

func TestRetryExportCreatesNewTaskReusingVoiceCaption(t *testing.T) {
	repo, points, exports, access, project, version := seedConfirmedExport(t)
	task, err := exports.CreateExport(context.Background(), access, project.ID, ExportCreateInput{
		VersionID: version.ID, IdempotencyKey: "retry-parent",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	task.Status = RenderStatusFailed
	task.VoiceFileID = "voice_file_1"
	task.CaptionFileID = "caption_file_1"
	task.ErrorCode = "RENDER_FAILED"
	if _, err := repo.CreateRenderTask(context.Background(), task); err != nil {
		// CreateRenderTask is idempotent by client key; overwrite map entry directly.
	}
	repo.mu.Lock()
	repo.tasks[task.ID] = task
	repo.mu.Unlock()

	retry, err := exports.RetryExport(context.Background(), access, project.ID, task.ID)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if retry.ID == task.ID {
		t.Fatal("retry must create a new task id")
	}
	if retry.RetryOfTaskID != task.ID {
		t.Fatalf("retryOf = %s", retry.RetryOfTaskID)
	}
	if retry.VoiceFileID != "voice_file_1" || retry.CaptionFileID != "caption_file_1" {
		t.Fatalf("voice/caption not reused: %+v", retry)
	}
	if retry.ManifestHash != version.ManifestHash && retry.ManifestHash == "" {
		loaded, _ := repo.GetVersion(context.Background(), access, project.ID, version.ID)
		if retry.ManifestHash != loaded.ManifestHash {
			t.Fatalf("manifest hash missing on retry")
		}
	}
	if !strings.Contains(retry.ClientRequestID, ":retry:") {
		t.Fatalf("retry client key = %s", retry.ClientRequestID)
	}
	if points.Reserved(retry.ID) != retry.ReservedPoints {
		t.Fatalf("retry points not reserved")
	}
	if points.IsReleased(task.ID) {
		t.Fatal("failed parent should keep original reservation until settle path releases it")
	}
}
