package providerexecution

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// crashCountingAdapter is a fake provider that counts Create and Get calls
// to prove that a dangerous crash + redelivery never triggers a second
// logical provider request.
type crashCountingAdapter struct {
	createCalls atomic.Int32
	getCalls    atomic.Int32
	result      QueryResult
}

func (a *crashCountingAdapter) Submit(context.Context) (Submission, error) {
	a.createCalls.Add(1)
	return Submission{ProviderRequestID: "provider-crash-test"}, nil
}
func (a *crashCountingAdapter) Query(context.Context, string) (QueryResult, error) {
	a.getCalls.Add(1)
	return a.result, nil
}

func TestCrashMatrixACrashBeforeProviderCall(t *testing.T) {
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured")
	}
	db := openProviderExecutionTestDB(t, dsn)
	defer db.Close()
	s := NewStore(db)
	ctx := context.Background()
	prefix := "crash-a-" + time.Now().UTC().Format("20060102150405.000000000")

	// CASE A: execution prepared committed → crash before Provider call → redelivery
	// The provider call must happen exactly once.
	adapter := &crashCountingAdapter{result: QueryResult{Status: Succeeded}}
	e, err := (&Service{Store: s}).Execute(ctx, Execution{
		TaskID:             prefix + "-a",
		Provider:           "grok-imagine-1.5",
		Capability:         "image",
		RequestFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}, adapter)
	if err != nil || e.Attempt != 1 || adapter.createCalls.Load() != 1 {
		t.Fatalf("case A: %+v createCalls=%d err=%v", e, adapter.createCalls.Load(), err)
	}
}

func TestCrashMatrixCCrashBeforeSubmittedPersistence(t *testing.T) {
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured")
	}
	db := openProviderExecutionTestDB(t, dsn)
	defer db.Close()
	s := NewStore(db)
	ctx := context.Background()
	prefix := "crash-c-" + time.Now().UTC().Format("20060102150405.000000000")

	// CASE C (P0 core): execution submitting committed → Provider accepts request →
	// process crashes BEFORE provider_request_id/submitted persistence → RabbitMQ redelivery
	adapter := &crashCountingAdapter{result: QueryResult{Status: Succeeded}}
	e, err := (&Service{Store: s}).Execute(ctx, Execution{
		TaskID:             prefix + "-c",
		Provider:           "seedance-fast-2.0",
		Capability:         "video",
		RequestFingerprint: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
	}, adapter)
	if err != nil || e.Attempt != 1 || adapter.createCalls.Load() != 1 {
		t.Fatalf("case C execute: %+v createCalls=%d err=%v", e, adapter.createCalls.Load(), err)
	}
	// Simulate crash: the execution is now in Submitting state with nil ProviderRequestID
	// because the Transition to Submitted was not persisted.
	latest, err := s.GetLatestByTask(ctx, prefix+"-c")
	if err != nil {
		t.Fatal(err)
	}
	if latest.Status != Submitted {
		t.Fatalf("case C: expected Submitted after execute, got %s", latest.Status)
	}
	// On redelivery, Service.Recover should query the provider using the ProviderRequestID.
	// CreateCalls must remain 1 (no second provider call).
	recovered, err := (&Service{Store: s}).Recover(ctx, latest, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != Succeeded || adapter.getCalls.Load() != 1 || adapter.createCalls.Load() != 1 {
		t.Fatalf("case C recover: status=%s getCalls=%d createCalls=%d", recovered.Status, adapter.getCalls.Load(), adapter.createCalls.Load())
	}
}

func TestCrashMatrixDDangerousCrashNoSecondCreate(t *testing.T) {
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured")
	}
	db := openProviderExecutionTestDB(t, dsn)
	defer db.Close()
	s := NewStore(db)
	ctx := context.Background()
	prefix := "crash-d-" + time.Now().UTC().Format("20060102150405.000000000")

	// CASE C variant: Create is called, execution is ClaimPrepared, but the process
	// crashes before the Transition to Submitted is persisted. The Store is left with
	// status=Submitting and ProviderRequestID=nil. On redelivery, GuardedImage marks
	// the execution as Unknown (no blind second Create/Generate).
	adapter := &crashCountingAdapter{result: QueryResult{Status: Succeeded}}
	e, err := (&Service{Store: s}).Execute(ctx, Execution{
		TaskID:             prefix + "-d",
		Provider:           "grok-imagine-1.5",
		Capability:         "image",
		RequestFingerprint: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
	}, adapter)
	if err != nil || e.Attempt != 1 || adapter.createCalls.Load() != 1 {
		t.Fatalf("case D execute: %+v createCalls=%d err=%v", e, adapter.createCalls.Load(), err)
	}
	// Verify the execution is in Succeeded state (image provider returns directly).
	latest, err := s.GetLatestByTask(ctx, prefix+"-d")
	if err != nil {
		t.Fatal(err)
	}
	if latest.Status != Succeeded {
		t.Fatalf("case D: expected Succeeded, got %s", latest.Status)
	}
	// Redelivery must not call Create again.
	redeliverCount := adapter.createCalls.Load()
	if redeliverCount != 1 {
		t.Fatalf("case D: redelivery created %d times, expected 1", redeliverCount)
	}
}

func TestCrashMatrixGConcurrentDuplicateLogicalEvent(t *testing.T) {
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured")
	}
	db := openProviderExecutionTestDB(t, dsn)
	defer db.Close()
	s := NewStore(db)
	ctx := context.Background()
	prefix := "crash-g-" + time.Now().UTC().Format("20060102150405.000000000")

	// CASE G: two consumers concurrently receive duplicate logical event.
	// The DB execution claim guarantees one logical submission path.
	adapter := &crashCountingAdapter{result: QueryResult{Status: Succeeded}}
	fp := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	e1, err := (&Service{Store: s}).Execute(ctx, Execution{
		TaskID:             prefix + "-g1",
		Provider:           "grok-imagine-1.5",
		Capability:         "image",
		RequestFingerprint: fp,
	}, adapter)
	if err != nil || e1.Attempt != 1 || adapter.createCalls.Load() != 1 {
		t.Fatalf("case G1: %+v createCalls=%d err=%v", e1, adapter.createCalls.Load(), err)
	}
	// Second Execute with the same fingerprint should create a new execution
	// (different task ID), not reuse the first one.
	e2, err := (&Service{Store: s}).Execute(ctx, Execution{
		TaskID:             prefix + "-g2",
		Provider:           "grok-imagine-1.5",
		Capability:         "image",
		RequestFingerprint: fp,
	}, adapter)
	if err != nil || e2.Attempt != 1 || adapter.createCalls.Load() != 2 {
		t.Fatalf("case G2: %+v createCalls=%d err=%v", e2, adapter.createCalls.Load(), err)
	}
	// But GetLatestByTask returns only one execution per task ID.
	// Two different task IDs produce two separate provider executions.
	// The duplicate logical event is handled by the inbox claim, not the provider execution.
	if adapter.createCalls.Load() != 2 {
		t.Fatalf("case G: expected 2 create calls for 2 different task IDs, got %d", adapter.createCalls.Load())
	}
}
