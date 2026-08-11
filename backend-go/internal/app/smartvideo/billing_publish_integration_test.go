package smartvideo

import (
	"context"
	"errors"
	"testing"
)

func seedUploadingExport(t *testing.T) (*MemoryRepository, *MemoryPointsLifecycle, *MemoryWorkPublisher, *SettleService, Access, RenderTask) {
	t.Helper()
	repo, points, exports, access, project, version := seedConfirmedExport(t)
	task, err := exports.CreateExport(context.Background(), access, project.ID, ExportCreateInput{
		VersionID: version.ID, IdempotencyKey: "billing-" + t.Name(),
	})
	if err != nil {
		t.Fatalf("create export: %v", err)
	}
	task.Status = RenderStatusUploading
	task.Stage = "uploading"
	task.Progress = 80
	repo.mu.Lock()
	repo.tasks[task.ID] = task
	repo.mu.Unlock()

	works := NewMemoryWorkPublisher()
	settle := NewSettleService(repo, points, works)
	return repo, points, works, settle, access, task
}

func TestBillingPublishCapturesPointsAndCreatesWork(t *testing.T) {
	repo, points, works, settle, access, task := seedUploadingExport(t)
	output := RenderOutput{VideoFileID: "out_video", CoverFileID: "out_cover", DurationMS: 5000, Width: 1080, Height: 1920, FrameRate: 30}

	done, err := settle.SettleSuccess(context.Background(), task, output)
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if done.Status != RenderStatusSucceeded || done.WorkID == "" || done.CapturedPoints != done.ReservedPoints {
		t.Fatalf("unexpected settled task: %+v", done)
	}
	if !points.IsCaptured(task.ID) {
		t.Fatal("ledger did not capture points")
	}
	work, ok := works.Get(done.WorkID)
	if !ok || work.ProjectID != task.ProjectID || work.VersionID != task.VersionID || work.RenderTaskID != task.ID {
		t.Fatalf("work metadata missing: %+v", work)
	}
	if work.VideoFileID != "out_video" {
		t.Fatalf("work video file = %s", work.VideoFileID)
	}

	again, err := settle.SettleSuccess(context.Background(), task, output)
	if err != nil {
		t.Fatalf("idempotent settle: %v", err)
	}
	if again.ID != done.ID || again.WorkID != done.WorkID || works.Count() != 1 {
		t.Fatalf("duplicate settle created extra work: again=%+v count=%d", again, works.Count())
	}
	_ = access
	_ = repo
}

func TestBillingPublishReleaseOnFinalFailure(t *testing.T) {
	_, points, _, settle, _, task := seedUploadingExport(t)
	if err := settle.SettleFinalFailure(context.Background(), task); err != nil {
		t.Fatalf("release: %v", err)
	}
	if !points.IsReleased(task.ID) {
		t.Fatal("expected points released on final failure")
	}
	stored, err := settle.repo.GetRenderTask(context.Background(), Access{TenantID: task.TenantID, UserID: task.UserID}, task.ProjectID, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.ReleasedPoints != stored.ReservedPoints || stored.CapturedPoints != 0 {
		t.Fatalf("released points not recorded: %+v", stored)
	}
	// second release is no-op
	if err := settle.SettleFinalFailure(context.Background(), task); err != nil {
		t.Fatalf("second release: %v", err)
	}
}

func TestBillingPublishRetriesAfterWorkFailureWithoutDoubleCharge(t *testing.T) {
	repo, points, works, settle, _, task := seedUploadingExport(t)
	output := RenderOutput{VideoFileID: "out_video", CoverFileID: "out_cover", DurationMS: 3000}
	works.SetFail(errors.New("work center down"))

	if _, err := settle.SettleSuccess(context.Background(), task, output); err == nil {
		t.Fatal("expected publish failure")
	}
	mid, err := repo.GetRenderTask(context.Background(), Access{TenantID: task.TenantID, UserID: task.UserID}, task.ProjectID, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if mid.CapturedPoints != 0 {
		t.Fatalf("must not capture before publish succeeds: %+v", mid)
	}
	if mid.OutputFileID == "" {
		t.Fatal("output file ids must persist for publish retry")
	}
	if points.IsReleased(task.ID) {
		t.Fatal("reservation must remain frozen while publish retries")
	}

	works.SetFail(nil)
	done, err := settle.SettleSuccess(context.Background(), mid, output)
	if err != nil {
		t.Fatalf("retry settle: %v", err)
	}
	if done.Status != RenderStatusSucceeded || works.Count() != 1 || done.CapturedPoints == 0 {
		t.Fatalf("retry settle incomplete: %+v works=%d", done, works.Count())
	}
}

func TestBillingPublishCancelStillReleasesOnce(t *testing.T) {
	repo, points, exports, access, project, version := seedConfirmedExport(t)
	task, err := exports.CreateExport(context.Background(), access, project.ID, ExportCreateInput{
		VersionID: version.ID, IdempotencyKey: "cancel-billing",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	cancelled, err := exports.CancelExport(context.Background(), access, project.ID, task.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled.ReleasedPoints != cancelled.ReservedPoints || !points.IsReleased(task.ID) {
		t.Fatalf("cancel did not release: %+v", cancelled)
	}
	_ = repo
}
