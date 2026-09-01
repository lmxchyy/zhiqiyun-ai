package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"xianzhi-ai/backend-go/internal/app/generation"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

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
	return db
}

func seedFailedRecoveryExecution(t *testing.T, store *pe.Store, taskID string, status pe.Status) {
	t.Helper()
	ctx := context.Background()
	fingerprint, err := pe.Fingerprint(taskID, "queryable-video", "queryable-video", "video", map[string]any{"provider": "queryable-video"})
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
	}, provider, store)
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
	}, provider, store)
	if !errors.Is(err, pe.ErrProviderExecutionFailed) {
		t.Fatalf("FAILED_PROVIDER_RECOVERY=FAIL for succeeded local state: err=%v", err)
	}
	if result != nil || provider.createCalls.Load() != 0 || provider.getCalls.Load() != 1 {
		t.Fatalf("succeeded-state recovery result=%v creates=%d gets=%d", result, provider.createCalls.Load(), provider.getCalls.Load())
	}
}

func stringPtr(value string) *string { return &value }
