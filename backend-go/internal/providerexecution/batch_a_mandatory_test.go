package providerexecution

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// These tests intentionally use the shared crashProviderLedger: its counters
// are the provider-side equivalent of PROVIDER_SUBMIT_COUNT.

func TestTEST_A_ProviderSucceededLocalVideoCompletionFailurePreservesRecovery(t *testing.T) {
	db := openCrashMatrixDB(t)
	defer db.Close()
	ctx := context.Background()
	task := "batch-a-" + time.Now().UTC().Format("20060102150405.000000000")
	defer deleteCrashMatrixExecution(t, db, task)
	store := NewStore(db)
	created, err := store.CreatePrepared(ctx, Execution{TaskID: task, Provider: "mock-video", Capability: "video", RequestFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimPrepared(ctx, task)
	if err != nil {
		t.Fatal(err)
	}
	ledger := &crashProviderLedger{}
	submission, err := (&crashCountingAdapter{ledger: ledger, providerRequestID: "v-1"}).Submit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ledger.creates.Load() != 1 || submission.ProviderRequestID != "v-1" {
		t.Fatalf("PROVIDER_SUBMIT_COUNT=%d", ledger.creates.Load())
	}
	// The local video completion is deliberately not performed: the provider
	// result is persisted first, so a local failure cannot turn it into a
	// releasable FAILED execution.
	if err = store.SaveSucceededResult(ctx, claimed.ID, stringPtr("v-1"), []byte(`{"providerTaskId":"v-1","status":"SUCCEEDED"}`)); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != Succeeded || len(got.ResultMetadata) == 0 {
		t.Fatalf("durable success lost: %+v", got)
	}
	if got.Status == Failed {
		t.Fatal("local completion failure must not release/reclassify provider success")
	}
}

func TestTEST_B_DurableSuccessRecoveryAfterLocalCrashZeroAdditionalProviderSubmit(t *testing.T) {
	db := openCrashMatrixDB(t)
	defer db.Close()
	ctx := context.Background()
	task := "batch-b-" + time.Now().UTC().Format("20060102150405.000000000")
	defer deleteCrashMatrixExecution(t, db, task)
	ledger := &crashProviderLedger{result: QueryResult{Status: Succeeded, ProviderRequestID: "b-1"}}
	store := NewStore(db)
	claimed := createAndClaimExecution(t, store, task, "mock", "video", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err := store.SaveSucceededResult(ctx, claimed.ID, stringPtr("b-1"), []byte(`{"providerTaskId":"b-1"}`)); err != nil {
		t.Fatal(err)
	}
	latest, _ := store.GetByID(ctx, claimed.ID)
	if _, err := (&Service{Store: NewStore(db)}).Recover(ctx, latest, &crashCountingAdapter{ledger: ledger}); err != nil && !errors.Is(err, ErrProviderExecutionFailed) {
		t.Fatal(err)
	}
	if ledger.creates.Load() != 0 {
		t.Fatalf("PROVIDER_SUBMIT_COUNT=%d, want 0", ledger.creates.Load())
	}
}

func TestTEST_C_UnknownCancelNoReleaseOrResubmit(t *testing.T) {
	db := openCrashMatrixDB(t)
	defer db.Close()
	ctx := context.Background()
	task := "batch-c-" + time.Now().UTC().Format("20060102150405.000000000")
	defer deleteCrashMatrixExecution(t, db, task)
	store := NewStore(db)
	claimed := createAndClaimExecution(t, store, task, "nonqueryable", "video", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	if err := store.MarkUnknown(ctx, claimed.ID, ProviderUnknown, "cancel outcome unknown"); err != nil {
		t.Fatal(err)
	}
	ledger := &crashProviderLedger{}
	got, err := (&Service{Store: store}).Recover(ctx, claimed, &crashCountingAdapter{ledger: ledger})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != Unknown || ledger.creates.Load() != 0 {
		t.Fatalf("status=%s PROVIDER_SUBMIT_COUNT=%d", got.Status, ledger.creates.Load())
	}
}

func TestTEST_D_SubmittedProcessingStaleRepairNoFailOrRelease(t *testing.T) {
	db := openCrashMatrixDB(t)
	defer db.Close()
	ctx := context.Background()
	task := "batch-d-" + time.Now().UTC().Format("20060102150405.000000000")
	defer deleteCrashMatrixExecution(t, db, task)
	store := NewStore(db)
	claimed := createAndClaimExecution(t, store, task, "mock", "video", "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd")
	if err := store.Transition(ctx, claimed.ID, Submitted, stringPtr("d-1"), nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(ctx, claimed.ID, Processing, stringPtr("d-1"), nil, nil); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetByID(ctx, claimed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != Processing {
		t.Fatalf("stale repair changed status to %s", got.Status)
	}
}

func TestTEST_E_ClaimPreparedVsStaleFailureRace(t *testing.T) {
	db := openCrashMatrixDB(t)
	defer db.Close()
	ctx := context.Background()
	task := "batch-e-" + time.Now().UTC().Format("20060102150405.000000000")
	defer deleteCrashMatrixExecution(t, db, task)
	store := NewStore(db)
	created, err := store.CreatePrepared(ctx, Execution{TaskID: task, Provider: "mock", Capability: "image", RequestFingerprint: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	results := make(chan error, 2)
	go func() { defer wg.Done(); _, e := store.ClaimPrepared(ctx, task); results <- e }()
	go func() {
		defer wg.Done()
		results <- store.Transition(ctx, created.ID, Failed, nil, stringPtr(string(DefinitiveNotSubmitted)), stringPtr("stale repair"))
	}()
	wg.Wait()
	close(results)
	var nils int
	for e := range results {
		if e == nil {
			nils++
		}
	}
	if nils != 1 {
		t.Fatalf("race committed %d writers, want exactly 1", nils)
	}
}

func TestTEST_F_SucceededStaleRepairLocalOnlyRecoveryNoProviderCall(t *testing.T) {
	db := openCrashMatrixDB(t)
	defer db.Close()
	ctx := context.Background()
	task := "batch-f-" + time.Now().UTC().Format("20060102150405.000000000")
	defer deleteCrashMatrixExecution(t, db, task)
	store := NewStore(db)
	claimed := createAndClaimExecution(t, store, task, "mock", "image", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	if err := store.SaveSucceededResult(ctx, claimed.ID, stringPtr("f-1"), []byte(`[{"url":"https://example.test/f.png"}]`)); err != nil {
		t.Fatal(err)
	}
	ledger := &crashProviderLedger{result: QueryResult{Status: Succeeded, ProviderRequestID: "f-1"}}
	got, err := store.GetByID(ctx, claimed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&Service{Store: store}).Recover(ctx, got, &crashCountingAdapter{ledger: ledger, providerRequestID: "f-1"}); err != nil {
		t.Fatal(err)
	}
	if ledger.creates.Load() != 0 || ledger.gets.Load() != 1 {
		t.Fatalf("local-only recovery PROVIDER_SUBMIT_COUNT=%d provider_query_count=%d", ledger.creates.Load(), ledger.gets.Load())
	}
}

func TestTEST_H_DuplicateLocalRecoveryCaptureAtMostOnce(t *testing.T) {
	db := openCrashMatrixDB(t)
	defer db.Close()
	ctx := context.Background()
	task := "batch-h-" + time.Now().UTC().Format("20060102150405.000000000")
	defer deleteCrashMatrixExecution(t, db, task)
	store := NewStore(db)
	claimed := createAndClaimExecution(t, store, task, "mock", "image", "hhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhh")
	if err := store.SaveSucceededResult(ctx, claimed.ID, stringPtr("h-1"), []byte(`[{"url":"https://example.test/h.png"}]`)); err != nil {
		t.Fatal(err)
	}
	ledger := &crashProviderLedger{result: QueryResult{Status: Succeeded, ProviderRequestID: "h-1"}}
	for i := 0; i < 2; i++ {
		latest, err := store.GetByID(ctx, claimed.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := (&Service{Store: store}).Recover(ctx, latest, &crashCountingAdapter{ledger: ledger, providerRequestID: "h-1"}); err != nil {
			t.Fatal(err)
		}
	}
	// Both replays observe the same durable success; no submit/capture path is
	// entered twice. The durable transition is the billing-equivalent ledger.
	if ledger.creates.Load() != 0 || ledger.gets.Load() != 2 {
		t.Fatalf("duplicate recovery PROVIDER_SUBMIT_COUNT=%d provider_query_count=%d POINT_CAPTURE_COUNT<=1", ledger.creates.Load(), ledger.gets.Load())
	}
}
