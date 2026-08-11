package smartvideo

import (
	"context"
	"errors"
	"testing"
	"time"
)

func preparedAnalysisService(t *testing.T) (*AnalysisService, *MemoryRepository, *MemoryAnalysisQueue, Access, Project, ProjectAsset) {
	t.Helper()
	service, repository, access := newTestService()
	project, err := service.CreateProject(context.Background(), access, CreateProjectInput{Title: "Analysis"})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := service.CreateAsset(context.Background(), access, project.ID, CreateAssetInput{FileID: "file_1", AssetType: AssetTypeVideo})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateAsset(context.Background(), access, project.ID, CreateAssetInput{FileID: "file_2", AssetType: AssetTypeImage, SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	queue := NewMemoryAnalysisQueue()
	repository.SetAnalysisQueue(queue)
	return NewAnalysisService(repository, queue, AnalysisOptions{Enabled: true, MaxAttempts: 2}), repository, queue, access, project, asset
}

func TestAnalysisRequestIsIdempotent(t *testing.T) {
	service, repository, queue, access, project, _ := preparedAnalysisService(t)
	for range 2 {
		if _, err := service.RequestProjectAnalysis(context.Background(), access, project.ID, "request_1"); err != nil {
			t.Fatal(err)
		}
	}
	tasks, err := repository.ListAnalysisTasks(context.Background(), access, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 || len(queue.Jobs()) != 2 {
		t.Fatalf("tasks=%d jobs=%d, want two each", len(tasks), len(queue.Jobs()))
	}
}

func TestAnalysisRequestRequiresIdempotencyKey(t *testing.T) {
	service, _, _, access, project, _ := preparedAnalysisService(t)
	if _, err := service.RequestProjectAnalysis(context.Background(), access, project.ID, " "); !errors.Is(err, ErrIdempotencyKeyRequired) {
		t.Fatalf("error = %v, want ErrIdempotencyKeyRequired", err)
	}
}

func TestRunningAnalysisCannotBeAcquiredOrSubmittedTwice(t *testing.T) {
	service, repository, queue, access, project, _ := preparedAnalysisService(t)
	if _, err := service.RequestProjectAnalysis(context.Background(), access, project.ID, "request_1"); err != nil {
		t.Fatal(err)
	}
	task := queue.Jobs()[0]
	if _, _, err := repository.AcquireAnalysisTask(context.Background(), task.TaskID, "worker_1", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.AcquireAnalysisTask(context.Background(), task.TaskID, "worker_2", time.Minute); !errors.Is(err, ErrAnalysisNotReady) {
		t.Fatalf("second acquire error = %v", err)
	}
	if _, err := service.RequestProjectAnalysis(context.Background(), access, project.ID, "request_2"); err != nil {
		t.Fatal(err)
	}
	tasks, _ := repository.ListAnalysisTasks(context.Background(), access, project.ID)
	if len(tasks) != 2 {
		t.Fatalf("running resubmit created %d tasks", len(tasks))
	}
}

func TestSucceededUnchangedAssetIsNotAnalyzedAgain(t *testing.T) {
	service, repository, queue, access, project, _ := preparedAnalysisService(t)
	if _, err := service.RequestProjectAnalysis(context.Background(), access, project.ID, "request_1"); err != nil {
		t.Fatal(err)
	}
	for _, job := range queue.Jobs() {
		task, asset, err := repository.AcquireAnalysisTask(context.Background(), job.TaskID, "worker_1", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		meta := NormalizedMediaMetadata{Kind: "IMAGE", Image: &ImageMetadata{Format: "png", Width: 100, Height: 100}}
		if asset.AssetType == AssetTypeVideo {
			meta = NormalizedMediaMetadata{Kind: "VIDEO", Video: &VideoMetadata{Format: "mp4", DurationMS: 1000}}
		}
		if err := repository.CompleteAnalysisTask(context.Background(), task.ID, "worker_1", AnalysisResult{
			Metadata: meta, AnalyzerVersion: "test",
		}); err != nil {
			t.Fatal(err)
		}
	}
	before, err := repository.ListAnalysisTasks(context.Background(), access, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RequestProjectAnalysis(context.Background(), access, project.ID, "request_2"); err != nil {
		t.Fatal(err)
	}
	tasks, err := repository.ListAnalysisTasks(context.Background(), access, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != len(before) {
		t.Fatalf("unchanged successful assets created %d tasks, before=%d", len(tasks), len(before))
	}
}

func TestFailedAnalysisAllowsExplicitRetry(t *testing.T) {
	service, repository, queue, access, project, asset := preparedAnalysisService(t)
	if _, err := service.RequestProjectAnalysis(context.Background(), access, project.ID, "request_1"); err != nil {
		t.Fatal(err)
	}
	task := queue.Jobs()[0]
	acquired, _, err := repository.AcquireAnalysisTask(context.Background(), task.TaskID, "worker_1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.FailAnalysisTask(context.Background(), acquired.ID, "worker_1", MediaErrorUnsupported, "不支持的媒体", time.Now(), true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RetryAsset(context.Background(), access, project.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	retried, err := repository.GetAnalysisTask(context.Background(), acquired.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status != AnalysisStatusQueued || retried.AttemptCount != 0 {
		t.Fatalf("unexpected retried task: %+v", retried)
	}
}

func TestCrossTenantAnalysisIsHidden(t *testing.T) {
	service, _, _, _, project, _ := preparedAnalysisService(t)
	_, err := service.GetProjectAnalysis(context.Background(), Access{TenantID: "tenant_b", UserID: "user_a"}, project.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant analysis error = %v", err)
	}
}

func TestAnalysisStateTransitionValidation(t *testing.T) {
	for _, transition := range [][2]string{
		{AnalysisStatusPending, AnalysisStatusSucceeded},
		{AnalysisStatusQueued, AnalysisStatusFailed},
		{AnalysisStatusSucceeded, AnalysisStatusRunning},
		{AnalysisStatusFailed, AnalysisStatusQueued},
	} {
		if err := ValidateAnalysisTransition(transition[0], transition[1]); !errors.Is(err, ErrInvalidStateTransition) {
			t.Fatalf("%s -> %s error = %v, want ErrInvalidStateTransition", transition[0], transition[1], err)
		}
	}
	for _, transition := range [][2]string{
		{AnalysisStatusPending, AnalysisStatusQueued},
		{AnalysisStatusQueued, AnalysisStatusRunning},
		{AnalysisStatusRunning, AnalysisStatusSucceeded},
		{AnalysisStatusRunning, AnalysisStatusFailed},
		{AnalysisStatusFailed, AnalysisStatusPending},
	} {
		if err := ValidateAnalysisTransition(transition[0], transition[1]); err != nil {
			t.Fatalf("%s -> %s unexpectedly rejected: %v", transition[0], transition[1], err)
		}
	}
}
