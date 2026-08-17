package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type durableTestRenderer struct {
	calls     int
	failures  int
	lastInput pptV2LegacyInput
}

func (r *durableTestRenderer) Render(_ context.Context, input pptV2LegacyInput) (pptV2RenderOutput, error) {
	r.calls++
	r.lastInput = input
	if r.calls <= r.failures {
		return pptV2RenderOutput{}, errors.New("temporary renderer outage")
	}
	payload := []byte("PK-durable-pptx")
	return pptV2RenderOutput{DeckID: "deck_durable", Revision: 1, SlideCount: 2, Bytes: len(payload), PPTX: payload}, nil
}

func durablePPTTask(t *testing.T, service *pptapp.Service, user adminUser) pptapp.Task {
	t.Helper()
	response, err := service.Generate(pptapp.GenerateRequest{
		UserID: user.ID, TenantID: effectiveTenantID(user), OrganizationID: user.OrganizationID,
		ClientRequestID: "durable-existing-task", Prompt: "Durable generation", SlideCount: 2,
		Outline: &pptapp.Outline{Title: "Durable generation", Slides: []pptapp.OutlineSlide{
			{Page: 1, Title: "Durable generation", Summary: "Cover", Layout: "cover", SlideType: "cover"},
			{Page: 2, Title: "State machine", Summary: "Durable work", BulletPoints: []string{"Lease", "Fencing"}, Layout: "content", SlideType: "text_image"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.GetTask(user.ID, response.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func durableTestAPI(t *testing.T, user adminUser) (api, *jsonStore, *pptapp.Service, *generatedStorageTestProvider) {
	t.Helper()
	pptService := pptapp.NewService()
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	fileService, provider := phase1StorageService()
	return api{store: store, pptService: pptService, fileService: fileService}, store, pptService, provider
}

func durableOptions(key string) pptV2DurableGenerationOptions {
	return pptV2DurableGenerationOptions{IdempotencyKey: key, ClientRequestID: key, WorkerID: "worker_phase2", MaxAttempts: 3, LeaseDuration: time.Minute}
}

func TestPPTV2DurableGenerationIsEffectivelyOnceAndUsesRealWorkProgress(t *testing.T) {
	user := adminUser{ID: "user_phase2", TenantID: "tenant_phase2", OrganizationID: "org_phase2", Role: "USER"}
	a, store, pptService, provider := durableTestAPI(t, user)
	task := durablePPTTask(t, pptService, user)
	jobs := pptapp.NewMemoryGenerationJobStore()
	renderer := &durableTestRenderer{}
	options := durableOptions("durable-effectively-once")

	result, job, err := a.runPPTV2DurableGeneration(t.Context(), user, task.TaskID, jobs, renderer, options)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != pptapp.GenerationJobSucceeded || job.Stage != pptapp.GenerationStageCompleted || job.Progress() != 100 || job.CompletedWorkUnits != 5 {
		t.Fatalf("unexpected durable terminal job: %+v", job)
	}
	if renderer.calls != 1 || result.DeckID != "deck_durable" || result.Task.PPTXAssetID == "" || result.Task.PPTXAssetID != result.Asset.ID {
		t.Fatalf("unexpected durable result: calls=%d result=%+v", renderer.calls, result)
	}
	if result.File.Visibility != "PRIVATE" || result.File.TenantID != user.TenantID || result.File.UserID != user.ID || result.File.BusinessID != job.ID {
		t.Fatalf("private durable file scope mismatch: %+v", result.File)
	}
	if len(provider.objects) != 1 {
		t.Fatalf("physical object count=%d", len(provider.objects))
	}
	assets, err := store.ListAssets()
	if err != nil || len(assets) != 1 || stringValue(assets[0].Metadata["pptV2GenerationJobId"]) != job.ID {
		t.Fatalf("effective asset count: assets=%+v err=%v", assets, err)
	}
	bundle, err := jobs.Get(t.Context(), pptapp.GenerationJobScope{TenantID: user.TenantID, UserID: user.ID}, job.ID)
	if err != nil || len(bundle.Slides) != 2 || len(bundle.Attempts) != 1 || len(bundle.History) != 7 {
		t.Fatalf("durable identity/history missing: bundle=%+v err=%v", bundle, err)
	}
	for _, slide := range bundle.Slides {
		if slide.Status != pptapp.GenerationChildSucceeded || slide.CompletedWorkUnits != 1 {
			t.Fatalf("slide work was not completed: %+v", slide)
		}
	}

	replayed, replayedJob, err := a.runPPTV2DurableGeneration(t.Context(), user, task.TaskID, jobs, renderer, options)
	if err != nil {
		t.Fatal(err)
	}
	if replayedJob.ID != job.ID || replayed.Asset.ID != result.Asset.ID || replayed.File.FileID != result.File.FileID || renderer.calls != 1 || len(provider.objects) != 1 {
		t.Fatalf("idempotent replay duplicated work: first=%+v replay=%+v calls=%d objects=%d", result, replayed, renderer.calls, len(provider.objects))
	}
	assets, _ = store.ListAssets()
	if len(assets) != 1 {
		t.Fatalf("replay duplicated Work Center artifact: %+v", assets)
	}
}

func TestPPTV2DurableGenerationRetriesWithoutDuplicatingArtifactOrTaskRelation(t *testing.T) {
	user := adminUser{ID: "user_retry", TenantID: "tenant_retry", OrganizationID: "org_retry", Role: "USER"}
	a, store, pptService, provider := durableTestAPI(t, user)
	task := durablePPTTask(t, pptService, user)
	jobs := pptapp.NewMemoryGenerationJobStore()
	renderer := &durableTestRenderer{failures: 1}
	options := durableOptions("durable-retry")
	options.RetryDelay = 0

	_, first, err := a.runPPTV2DurableGeneration(t.Context(), user, task.TaskID, jobs, renderer, options)
	if err == nil || first.Status != pptapp.GenerationJobRetryWait || first.AttemptCount != 1 {
		t.Fatalf("first retryable attempt: job=%+v err=%v", first, err)
	}
	result, second, err := a.runPPTV2DurableGeneration(t.Context(), user, task.TaskID, jobs, renderer, options)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != pptapp.GenerationJobSucceeded || second.AttemptCount != 2 || renderer.calls != 2 {
		t.Fatalf("retry did not resume: job=%+v calls=%d", second, renderer.calls)
	}
	assets, _ := store.ListAssets()
	files, total, fileErr := a.fileService.ListFiles(t.Context(), storagecenter.FileFilter{TenantID: user.TenantID, UserID: user.ID, BusinessType: "ppt_v2_generation", Limit: 10})
	if fileErr != nil || total != 1 || len(files) != 1 || len(assets) != 1 || len(provider.objects) != 1 {
		t.Fatalf("retry duplicated persistence: files=%+v total=%d assets=%+v objects=%d err=%v", files, total, assets, len(provider.objects), fileErr)
	}
	persisted, err := pptService.GetTask(user.ID, task.TaskID)
	if err != nil || persisted.PPTXAssetID != result.Asset.ID || persisted.V2DeckID != second.DeckID {
		t.Fatalf("task relation is not idempotent: task=%+v err=%v", persisted, err)
	}
}

func TestPPTV2DurableGenerationResumesRenderedCheckpointAfterRestart(t *testing.T) {
	user := adminUser{ID: "user_restart", TenantID: "tenant_restart", OrganizationID: "org_restart", Role: "USER"}
	a, _, pptService, _ := durableTestAPI(t, user)
	task := durablePPTTask(t, pptService, user)
	jobs := pptapp.NewMemoryGenerationJobStore()
	options := durableOptions("durable-restart")
	now := time.Now().UTC().Add(-2 * time.Minute)
	job, _, err := jobs.Create(t.Context(), pptapp.CreateGenerationJobInput{
		JobID: "pptv2_job_restart", TenantID: user.TenantID, UserID: user.ID, OrganizationID: user.OrganizationID,
		ExistingTaskID: task.TaskID, ClientRequestID: options.ClientRequestID, IdempotencyKey: options.IdempotencyKey,
		MaxAttempts: 3, SlideCount: 2, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := jobs.Claim(t.Context(), pptapp.GenerationJobScope{TenantID: user.TenantID, UserID: user.ID}, job.ID, "process_before_restart", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := json.Marshal(legacyPPTV2Input(task))
	job, err = jobs.Checkpoint(t.Context(), lease, pptapp.GenerationCheckpoint{NextStage: pptapp.GenerationStageTaskLoaded, InputSnapshot: snapshot, SourceSlideIDs: []string{task.Slides[0].ID, task.Slides[1].ID}, Now: now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("PK-checkpointed-before-restart")
	digest := sha256.Sum256(payload)
	_, err = jobs.Checkpoint(t.Context(), lease, pptapp.GenerationCheckpoint{
		NextStage: pptapp.GenerationStageRendered, DeckID: "deck_restart", Revision: 1, SlideCount: 2,
		RenderSHA256: hex.EncodeToString(digest[:]), RenderBytes: payload, Now: now.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer := &durableTestRenderer{}
	result, resumed, err := a.runPPTV2DurableGeneration(t.Context(), user, task.TaskID, jobs, renderer, options)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != pptapp.GenerationJobSucceeded || resumed.AttemptCount != 2 || renderer.calls != 0 || result.DeckID != "deck_restart" {
		t.Fatalf("restart reran completed work: job=%+v calls=%d result=%+v", resumed, renderer.calls, result)
	}
}

func TestPPTV2DurableGenerationCancellationAndTenantIsolationStopWork(t *testing.T) {
	user := adminUser{ID: "user_scope", TenantID: "tenant_a", OrganizationID: "org_a", Role: "USER"}
	a, store, pptService, provider := durableTestAPI(t, user)
	task := durablePPTTask(t, pptService, user)
	jobs := pptapp.NewMemoryGenerationJobStore()
	options := durableOptions("durable-cancel")
	job, _, err := jobs.Create(t.Context(), pptapp.CreateGenerationJobInput{
		TenantID: user.TenantID, UserID: user.ID, OrganizationID: user.OrganizationID, ExistingTaskID: task.TaskID,
		IdempotencyKey: options.IdempotencyKey, ClientRequestID: options.ClientRequestID, MaxAttempts: 3, SlideCount: 2, Now: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.Cancel(t.Context(), pptapp.GenerationJobScope{TenantID: user.TenantID, UserID: user.ID}, job.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	renderer := &durableTestRenderer{}
	if _, cancelled, err := a.runPPTV2DurableGeneration(t.Context(), user, task.TaskID, jobs, renderer, options); !errors.Is(err, pptapp.ErrGenerationJobCancelled) || cancelled.Status != pptapp.GenerationJobCancelled {
		t.Fatalf("cancelled run: job=%+v err=%v", cancelled, err)
	}

	wrongTenant := user
	wrongTenant.TenantID = "tenant_b"
	wrongTenant.OrganizationID = "org_b"
	otherJobs := pptapp.NewMemoryGenerationJobStore()
	if _, failed, err := a.runPPTV2DurableGeneration(t.Context(), wrongTenant, task.TaskID, otherJobs, renderer, durableOptions("durable-wrong-tenant")); err == nil || failed.Status != pptapp.GenerationJobFailed || failed.LastError == nil || failed.LastError.Code != "TASK_SCOPE_MISMATCH" {
		t.Fatalf("cross-tenant run: job=%+v err=%v", failed, err)
	}
	assets, _ := store.ListAssets()
	if renderer.calls != 0 || len(provider.objects) != 0 || len(assets) != 0 {
		t.Fatalf("cancel/scope check crossed side-effect boundary: calls=%d objects=%d assets=%d", renderer.calls, len(provider.objects), len(assets))
	}
}
