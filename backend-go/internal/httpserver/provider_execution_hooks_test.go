package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"xianzhi-ai/backend-go/internal/app/generation"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

type countingImageProvider struct{ calls atomic.Int32 }

func (p *countingImageProvider) DefaultModel() string { return "counting-image" }
func (p *countingImageProvider) Generate(context.Context, generation.CreateRequest) ([]generation.GeneratedImage, error) {
	p.calls.Add(1)
	return []generation.GeneratedImage{{URL: "https://artifact.example/result.png", ContentType: "image/png", ProviderTaskID: "image-result-1"}}, nil
}

type failedRecoveryVideoProvider struct {
	createCalls atomic.Int32
	getCalls    atomic.Int32
}

func (p *failedRecoveryVideoProvider) DefaultModel() string { return "queryable-video" }

func (p *failedRecoveryVideoProvider) Create(context.Context, generation.CreateRequest) (any, error) {
	p.createCalls.Add(1)
	return nil, errors.New("Create must not run during failed recovery")
}

func (p *failedRecoveryVideoProvider) Get(context.Context, string) (any, error) {
	p.getCalls.Add(1)
	return map[string]any{
		"providerTaskId": "provider-failed",
		"status":         "FAILED",
	}, nil
}

func openProviderExecutionHookTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "database", "migrations", "114-provider-execution-safety.sql"))
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), string(raw)); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}

func seedFailedRecoveryExecution(t *testing.T, store *pe.Store, taskID string, status pe.Status) {
	t.Helper()
	ctx := context.Background()
	fingerprint, err := videoRequestFingerprint(taskID, "queryable-video", "video", "queryable-video", map[string]any{"provider": "queryable-video"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.CreatePrepared(ctx, pe.Execution{
		TaskID:             taskID,
		Provider:           "queryable-video",
		ProviderModel:      "queryable-video",
		Capability:         "video",
		RequestFingerprint: fingerprint,
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err = store.ClaimPrepared(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(ctx, claimed.ID, pe.Submitted, stringPtr("provider-failed"), nil, nil); err != nil {
		t.Fatal(err)
	}
	if status == pe.Unknown {
		if err := store.Transition(ctx, claimed.ID, pe.Unknown, nil, stringPtr(string(pe.ProviderUnknown)), stringPtr("process restarted")); err != nil {
			t.Fatal(err)
		}
	}
	if status == pe.Succeeded {
		if err := store.Transition(ctx, claimed.ID, pe.Succeeded, nil, stringPtr(string(pe.ProviderSucceeded)), nil); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGuardedImageDurableSuccessReplayDoesNotGenerateAgain(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "hook-image-crash-" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", taskID)
	}()
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_generation_tasks (id,user_id,type,status,created_at,updated_at) VALUES ($1,'hook-user','image','PENDING',now()::text,now()::text)`, taskID); err != nil {
		t.Fatal(err)
	}
	provider := &countingImageProvider{}
	req := generation.CreateRequest{Model: "counting-image", Params: map[string]any{providerExecutionTaskParam: taskID, "provider": "counting-image"}}
	first, err := guardedImage(ctx, req, provider, pe.NewStore(db))
	if err != nil {
		t.Fatal(err)
	}
	// Simulate process loss after durable provider success and before local task completion.
	second, err := guardedImage(ctx, req, provider, pe.NewStore(db))
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls.Load() != 1 || len(first) != 1 || len(second) != 1 || second[0].ProviderTaskID != "image-result-1" {
		t.Fatalf("calls=%d first=%#v second=%#v", provider.calls.Load(), first, second)
	}
}

func TestGuardedVideoFailedGetReturnsFailureWithoutCreate(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "hook-failed-recovery-" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskID)
	}()

	store := pe.NewStore(db)
	seedFailedRecoveryExecution(t, store, taskID, pe.Unknown)
	provider := &failedRecoveryVideoProvider{}
	result, err := guardedVideo(ctx, generation.CreateRequest{
		Model: "queryable-video",
		Params: map[string]any{
			providerExecutionTaskParam: taskID,
			"provider":                 "queryable-video",
		},
	}, provider, store, nil)
	if !errors.Is(err, pe.ErrProviderExecutionFailed) {
		t.Fatalf("FAILED_PROVIDER_RECOVERY=FAIL: err=%v", err)
	}
	if result != nil || provider.createCalls.Load() != 0 || provider.getCalls.Load() != 1 {
		t.Fatalf("failed recovery result=%v creates=%d gets=%d", result, provider.createCalls.Load(), provider.getCalls.Load())
	}
	latest, err := store.GetLatestByTask(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Status != pe.Failed {
		t.Fatalf("execution status=%s, want failed", latest.Status)
	}
}

func TestGuardedVideoFailedGetForSucceededExecutionReturnsFailure(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "hook-failed-succeeded-" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskID)
	}()

	store := pe.NewStore(db)
	seedFailedRecoveryExecution(t, store, taskID, pe.Succeeded)
	provider := &failedRecoveryVideoProvider{}
	result, err := guardedVideo(ctx, generation.CreateRequest{
		Model: "queryable-video",
		Params: map[string]any{
			providerExecutionTaskParam: taskID,
			"provider":                 "queryable-video",
		},
	}, provider, store, nil)
	if !errors.Is(err, pe.ErrProviderExecutionFailed) {
		t.Fatalf("FAILED_PROVIDER_RECOVERY=FAIL for succeeded local state: err=%v", err)
	}
	if result != nil || provider.createCalls.Load() != 0 || provider.getCalls.Load() != 1 {
		t.Fatalf("succeeded-state recovery result=%v creates=%d gets=%d", result, provider.createCalls.Load(), provider.getCalls.Load())
	}
}

func stringPtr(value string) *string { return &value }

type mockVideoProvider struct {
	createCalls atomic.Int32
	getCalls    atomic.Int32
	createFn    func(ctx context.Context, req generation.CreateRequest) (any, error)
	getFn       func(ctx context.Context, id string) (any, error)
}

func (p *mockVideoProvider) DefaultModel() string { return "mock-video" }
func (p *mockVideoProvider) Create(ctx context.Context, req generation.CreateRequest) (any, error) {
	p.createCalls.Add(1)
	if p.createFn != nil {
		return p.createFn(ctx, req)
	}
	return map[string]any{
		"provider":       "mock-video",
		"providerTaskId": "v-1",
		"status":         "SUCCEEDED",
		"videoUrl":       "https://cdn.example.com/video.mp4",
	}, nil
}
func (p *mockVideoProvider) Get(ctx context.Context, id string) (any, error) {
	p.getCalls.Add(1)
	if p.getFn != nil {
		return p.getFn(ctx, id)
	}
	return map[string]any{
		"provider":       "mock-video",
		"providerTaskId": id,
		"status":         "SUCCEEDED",
		"videoUrl":       "https://cdn.example.com/video.mp4",
	}, nil
}

func TestTEST_A_CrashBeforeSubmitting_RetryMaySubmitOnce(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "test-a-crash-prepared-" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", taskID)
	}()
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_generation_tasks (id,user_id,type,status,created_at,updated_at) VALUES ($1,'test-user','video','PENDING',now()::text,now()::text)`, taskID); err != nil {
		t.Fatal(err)
	}
	store := pe.NewStore(db)
	fp, _ := videoRequestFingerprint(taskID, "mock-video", "video", "mock-video", map[string]any{"provider": "mock-video"})
	_, err := store.CreatePrepared(ctx, pe.Execution{
		TaskID:             taskID,
		Provider:           "mock-video",
		ProviderModel:      "mock-video",
		Capability:         "video",
		RequestFingerprint: fp,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &mockVideoProvider{}
	req := generation.CreateRequest{
		Model: "mock-video",
		Params: map[string]any{
			providerExecutionTaskParam: taskID,
			"provider":                 "mock-video",
		},
	}
	res, err := guardedVideo(ctx, req, provider, store, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || provider.createCalls.Load() != 1 {
		t.Fatalf("expected 1 create call, got %d", provider.createCalls.Load())
	}
	latest, err := store.GetLatestByTask(ctx, taskID)
	if err != nil || latest.Status != pe.Succeeded {
		t.Fatalf("expected status Succeeded, got %v (%v)", latest.Status, err)
	}
}

func TestTEST_B_SubmittingCommitted_CrashBeforeIDPersisted_RedeliveryNeverCallsCreateAgain(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "test-b-submitting-crash-" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", taskID)
	}()
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_generation_tasks (id,user_id,type,status,created_at,updated_at) VALUES ($1,'test-user','video','PENDING',now()::text,now()::text)`, taskID); err != nil {
		t.Fatal(err)
	}
	store := pe.NewStore(db)
	fp, _ := videoRequestFingerprint(taskID, "mock-video", "video", "mock-video", map[string]any{"provider": "mock-video"})
	_, err := store.CreatePrepared(ctx, pe.Execution{
		TaskID:             taskID,
		Provider:           "mock-video",
		ProviderModel:      "mock-video",
		Capability:         "video",
		RequestFingerprint: fp,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimPrepared(ctx, taskID); err != nil {
		t.Fatal(err)
	}

	provider := &mockVideoProvider{}
	req := generation.CreateRequest{
		Model: "mock-video",
		Params: map[string]any{
			providerExecutionTaskParam: taskID,
			"provider":                 "mock-video",
		},
	}
	_, err = guardedVideo(ctx, req, provider, store, nil)
	if !errors.Is(err, pe.ErrUnknownResubmitBlocked) {
		t.Fatalf("expected ErrUnknownResubmitBlocked, got: %v", err)
	}
	if provider.createCalls.Load() != 0 {
		t.Fatalf("BLIND_RESUBMIT: provider Create called %d times, want 0", provider.createCalls.Load())
	}
	latest, _ := store.GetLatestByTask(ctx, taskID)
	if latest.Status != pe.Unknown {
		t.Fatalf("expected status Unknown, got %v", latest.Status)
	}

	_, err = guardedVideo(ctx, req, provider, store, nil)
	if !errors.Is(err, pe.ErrUnknownResubmitBlocked) {
		t.Fatalf("expected ErrUnknownResubmitBlocked on second redelivery, got: %v", err)
	}
	if provider.createCalls.Load() != 0 {
		t.Fatalf("BLIND_RESUBMIT: provider Create called on 2nd redelivery, want 0")
	}
}

func TestTEST_C_SubmittingWithDurableID_RedeliveryCallsGetNotCreate(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "test-c-submitted-durable-id-" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", taskID)
	}()
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_generation_tasks (id,user_id,type,status,created_at,updated_at) VALUES ($1,'test-user','video','PENDING',now()::text,now()::text)`, taskID); err != nil {
		t.Fatal(err)
	}
	store := pe.NewStore(db)
	fp, _ := videoRequestFingerprint(taskID, "queryable-video", "video", "queryable-video", map[string]any{"provider": "queryable-video"})
	claimed, err := store.CreatePrepared(ctx, pe.Execution{
		TaskID:             taskID,
		Provider:           "queryable-video",
		ProviderModel:      "queryable-video",
		Capability:         "video",
		RequestFingerprint: fp,
	})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err = store.ClaimPrepared(ctx, taskID); err != nil {
		t.Fatal(err)
	}
	providerTaskID := "prov-task-c-999"
	if err := store.Transition(ctx, claimed.ID, pe.Submitted, &providerTaskID, nil, nil); err != nil {
		t.Fatal(err)
	}

	provider := &mockVideoProvider{
		getFn: func(ctx context.Context, id string) (any, error) {
			if id != providerTaskID {
				t.Fatalf("unexpected provider task ID: %s", id)
			}
			return map[string]any{
				"provider":       "queryable-video",
				"providerTaskId": id,
				"status":         "SUCCEEDED",
				"videoUrl":       "https://cdn.example.com/recovered.mp4",
			}, nil
		},
	}
	req := generation.CreateRequest{
		Model: "queryable-video",
		Params: map[string]any{
			providerExecutionTaskParam: taskID,
			"provider":                 "queryable-video",
		},
	}
	res, err := guardedVideo(ctx, req, provider, store, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected result from recovery")
	}
	if provider.createCalls.Load() != 0 {
		t.Fatalf("provider Create called %d times, want 0", provider.createCalls.Load())
	}
	if provider.getCalls.Load() != 1 {
		t.Fatalf("provider Get called %d times, want 1", provider.getCalls.Load())
	}
	latest, _ := store.GetLatestByTask(ctx, taskID)
	if latest.Status != pe.Succeeded {
		t.Fatalf("expected status Succeeded, got %v", latest.Status)
	}
}

func TestTEST_D_ProviderCreateDefinitivePreSubmitFailure_SafeRetry(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "test-d-definitive-failure-" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", taskID)
	}()
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_generation_tasks (id,user_id,type,status,created_at,updated_at) VALUES ($1,'test-user','video','PENDING',now()::text,now()::text)`, taskID); err != nil {
		t.Fatal(err)
	}
	store := pe.NewStore(db)
	shouldFail := true
	provider := &mockVideoProvider{
		createFn: func(ctx context.Context, req generation.CreateRequest) (any, error) {
			if shouldFail {
				return nil, errors.New("video model is required")
			}
			return map[string]any{
				"provider":       "mock-video",
				"providerTaskId": "d-success",
				"status":         "SUCCEEDED",
				"videoUrl":       "https://cdn.example.com/d.mp4",
			}, nil
		},
	}
	req := generation.CreateRequest{
		Model: "mock-video",
		Params: map[string]any{
			providerExecutionTaskParam: taskID,
			"provider":                 "mock-video",
		},
	}
	_, err := guardedVideo(ctx, req, provider, store, nil)
	if err == nil {
		t.Fatal("expected error on first attempt")
	}
	latest, _ := store.GetLatestByTask(ctx, taskID)
	if latest.Status != pe.Failed {
		t.Fatalf("expected status Failed, got %v", latest.Status)
	}
	if latest.ErrorClass == nil || *latest.ErrorClass != string(pe.DefinitiveNotSubmitted) {
		t.Fatalf("expected error class DefinitiveNotSubmitted, got %v", latest.ErrorClass)
	}

	shouldFail = false
	res, err := guardedVideo(ctx, req, provider, store, nil)
	if err != nil {
		t.Fatalf("attempt 2 should succeed, got: %v", err)
	}
	if res == nil {
		t.Fatal("expected result from attempt 2")
	}
	latest2, _ := store.GetLatestByTask(ctx, taskID)
	if latest2.Attempt != 2 || latest2.Status != pe.Succeeded {
		t.Fatalf("attempt 2: got attempt=%d status=%v", latest2.Attempt, latest2.Status)
	}
}

func TestTEST_E_ProviderCreateNetworkAmbiguousError_NoBlindResubmit(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "test-e-ambiguous-network-" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", taskID)
	}()
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_generation_tasks (id,user_id,type,status,created_at,updated_at) VALUES ($1,'test-user','video','PENDING',now()::text,now()::text)`, taskID); err != nil {
		t.Fatal(err)
	}
	store := pe.NewStore(db)
	provider := &mockVideoProvider{
		createFn: func(ctx context.Context, req generation.CreateRequest) (any, error) {
			return nil, errors.New("connection reset by peer")
		},
	}
	req := generation.CreateRequest{
		Model: "mock-video",
		Params: map[string]any{
			providerExecutionTaskParam: taskID,
			"provider":                 "mock-video",
		},
	}
	_, err := guardedVideo(ctx, req, provider, store, nil)
	if err == nil || !errors.Is(err, pe.ErrUnknownResubmitBlocked) {
		t.Fatalf("expected ErrUnknownResubmitBlocked, got: %v", err)
	}
	latest, _ := store.GetLatestByTask(ctx, taskID)
	if latest.Status != pe.Unknown {
		t.Fatalf("expected status Unknown, got %v", latest.Status)
	}

	_, err = guardedVideo(ctx, req, provider, store, nil)
	if !errors.Is(err, pe.ErrUnknownResubmitBlocked) {
		t.Fatalf("expected ErrUnknownResubmitBlocked, got: %v", err)
	}
	if provider.createCalls.Load() != 1 {
		t.Fatalf("BLIND_RESUBMIT: createCalls=%d, want 1", provider.createCalls.Load())
	}
}

func TestTEST_F_ProviderReturnsProviderRequestID_DurablePersistenceSucceedsExactlyOnce(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "test-f-early-id-persistence-" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", taskID)
	}()
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_generation_tasks (id,user_id,type,status,created_at,updated_at) VALUES ($1,'test-user','video','PENDING',now()::text,now()::text)`, taskID); err != nil {
		t.Fatal(err)
	}
	store := pe.NewStore(db)
	provider := &mockVideoProvider{
		createFn: func(ctx context.Context, req generation.CreateRequest) (any, error) {
			generation.NotifyProviderSubmission(ctx, "early-task-f-999")
			return map[string]any{
				"provider":       "mock-video",
				"providerTaskId": "early-task-f-999",
				"status":         "SUCCEEDED",
				"videoUrl":       "https://cdn.example.com/f.mp4",
			}, nil
		},
	}
	req := generation.CreateRequest{
		Model: "mock-video",
		Params: map[string]any{
			providerExecutionTaskParam: taskID,
			"provider":                 "mock-video",
		},
	}
	res, err := guardedVideo(ctx, req, provider, store, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected result")
	}
	latest, _ := store.GetLatestByTask(ctx, taskID)
	if latest.ProviderRequestID == nil || *latest.ProviderRequestID != "early-task-f-999" {
		t.Fatalf("providerRequestID=%v, want early-task-f-999", latest.ProviderRequestID)
	}
	if latest.Status != pe.Succeeded {
		t.Fatalf("status=%v, want Succeeded", latest.Status)
	}
}

func TestTEST_G_DuplicateInvocationSimulation_SameTaskAttemptNoSecondCreate(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "test-g-concurrent-invocation-" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", taskID)
	}()
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_generation_tasks (id,user_id,type,status,created_at,updated_at) VALUES ($1,'test-user','video','PENDING',now()::text,now()::text)`, taskID); err != nil {
		t.Fatal(err)
	}
	store := pe.NewStore(db)
	provider := &mockVideoProvider{
		createFn: func(ctx context.Context, req generation.CreateRequest) (any, error) {
			time.Sleep(50 * time.Millisecond)
			return map[string]any{
				"provider":       "mock-video",
				"providerTaskId": "g-task",
				"status":         "SUCCEEDED",
				"videoUrl":       "https://cdn.example.com/g.mp4",
			}, nil
		},
	}
	req := generation.CreateRequest{
		Model: "mock-video",
		Params: map[string]any{
			providerExecutionTaskParam: taskID,
			"provider":                 "mock-video",
		},
	}

	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			_, _ = guardedVideo(ctx, req, provider, store, nil)
		}()
	}
	wg.Wait()

	if provider.createCalls.Load() != 1 {
		t.Fatalf("CONCURRENT_SUBMISSION: createCalls=%d, want exactly 1", provider.createCalls.Load())
	}
}

func TestTEST_H_Points_AmbiguousProviderState_NoCaptureNoRelease(t *testing.T) {
	store := newBillingAcceptanceStore(t)
	req := videoAcceptanceRequest("ambiguous-points-safety")
	req.Model = "grok-imagine-1.5-video"
	req.Params["duration"] = 6
	req.Params["resolution"] = "480p"
	pending, err := store.CreatePendingGenerationTask(req)
	if err != nil {
		t.Fatalf("create pending task: %v", err)
	}
	if pending.BillingStatus != "RESERVED" {
		t.Fatalf("initial state unexpected: billing=%s", pending.BillingStatus)
	}

	service := generation.NewServiceWithOptions(generation.ServiceOptions{
		VideoProvider: &mockVideoProvider{
			createFn: func(ctx context.Context, req generation.CreateRequest) (any, error) {
				return nil, errors.Join(errors.New("upstream gateway timeout"), pe.ErrUnknownResubmitBlocked)
			},
		},
	})
	api{store: store}.runVideoGenerationTask(pending.ID, service, req)

	tasks, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}
	task := generationBillingTaskByID(t, tasks, pending.ID)
	// Points must remain RESERVED, not CAPTURED and not RELEASED!
	if task.Status == "FAILED" || task.Status == "SUCCEEDED" {
		t.Fatalf("ambiguous state must not mark task terminal, got: %s", task.Status)
	}
	if task.BillingStatus != "RESERVED" {
		t.Fatalf("ambiguous state must leave points RESERVED, got: %s", task.BillingStatus)
	}
	if task.CapturedPoints != 0 {
		t.Fatalf("CapturedPoints=%v, want 0", task.CapturedPoints)
	}
	if task.ReleasedPoints != 0 {
		t.Fatalf("ReleasedPoints=%v, want 0", task.ReleasedPoints)
	}
}
