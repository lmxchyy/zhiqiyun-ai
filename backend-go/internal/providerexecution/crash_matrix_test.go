package providerexecution

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// crashProviderLedger represents provider-side state that survives a process
// restart. Each adapter instance is process-local; the ledger is deliberately
// shared so provider calls can be counted across the simulated processes.
type crashProviderLedger struct {
	creates atomic.Int32
	gets    atomic.Int32
	result  QueryResult
}

type crashCountingAdapter struct {
	ledger            *crashProviderLedger
	providerRequestID string
}

func (a *crashCountingAdapter) Submit(context.Context) (Submission, error) {
	a.ledger.creates.Add(1)
	return Submission{ProviderRequestID: a.providerRequestID}, nil
}

func (a *crashCountingAdapter) Query(context.Context, string) (QueryResult, error) {
	a.ledger.gets.Add(1)
	return a.ledger.result, nil
}

func openCrashMatrixDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured")
	}
	return openProviderExecutionTestDB(t, dsn)
}

func deleteCrashMatrixExecution(t *testing.T, db *sql.DB, taskID string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), "DELETE FROM provider_executions WHERE task_id=$1", taskID); err != nil {
		t.Logf("cleanup provider execution %s: %v", taskID, err)
	}
}

func createAndClaimExecution(t *testing.T, store *Store, taskID, provider, capability, fingerprint string) Execution {
	t.Helper()
	created, err := store.CreatePrepared(context.Background(), Execution{
		TaskID:             taskID,
		Provider:           provider,
		Capability:         capability,
		RequestFingerprint: fingerprint,
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimPrepared(context.Background(), created.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	return claimed
}

func TestCrashMatrixACrashBeforeProviderCall(t *testing.T) {
	db := openCrashMatrixDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "crash-a-" + time.Now().UTC().Format("20060102150405.000000000")
	defer deleteCrashMatrixExecution(t, db, taskID)

	firstProcessStore := NewStore(db)
	createAndClaimExecution(t, firstProcessStore, taskID, "grok-imagine-1.5", "image", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	// The first process crashes after the submitting claim and before calling
	// the provider. A new process must not create a request from that row.
	firstProcessStore = nil

	secondProcessStore := NewStore(db)
	latest, err := secondProcessStore.GetLatestByTask(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	ledger := &crashProviderLedger{}
	recovered, err := (&Service{Store: secondProcessStore}).Recover(ctx, latest, &crashCountingAdapter{ledger: ledger})
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != Unknown || ledger.creates.Load() != 0 {
		t.Fatalf("case A: status=%s creates=%d", recovered.Status, ledger.creates.Load())
	}
}

func TestCrashMatrixCCrashBeforeSubmittedPersistence(t *testing.T) {
	db := openCrashMatrixDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "crash-c-" + time.Now().UTC().Format("20060102150405.000000000")
	defer deleteCrashMatrixExecution(t, db, taskID)

	// First process: prepare and claim are durable, then the provider accepts
	// the request. The process crashes before persisting provider_request_id.
	firstProcessStore := NewStore(db)
	createAndClaimExecution(t, firstProcessStore, taskID, "seedance-fast-2.0", "video", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	providerLedger := &crashProviderLedger{}
	firstAdapter := &crashCountingAdapter{ledger: providerLedger, providerRequestID: "provider-c"}
	accepted, err := firstAdapter.Submit(ctx)
	if err != nil || accepted.ProviderRequestID == "" {
		t.Fatalf("case C provider accept: requestID=%q err=%v", accepted.ProviderRequestID, err)
	}
	// Discard all first-process state. In particular, do not call Transition.
	firstAdapter = nil
	firstProcessStore = nil

	// Second process / RabbitMQ redelivery: reload only from the durable store.
	secondProcessStore := NewStore(db)
	latest, err := secondProcessStore.GetLatestByTask(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Status != Submitting || latest.ProviderRequestID != nil {
		t.Fatalf("case C durable crash state: status=%s requestID=%v", latest.Status, latest.ProviderRequestID)
	}
	secondAdapter := &crashCountingAdapter{ledger: providerLedger, providerRequestID: "provider-c"}
	recovered, err := (&Service{Store: secondProcessStore}).Recover(ctx, latest, secondAdapter)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != Unknown {
		t.Fatalf("case C recovered status=%s, want unknown", recovered.Status)
	}
	if providerLedger.creates.Load() != 1 {
		t.Fatalf("case C SECOND_BLIND_PROVIDER_SUBMISSION=YES: creates=%d", providerLedger.creates.Load())
	}
	if providerLedger.gets.Load() != 0 {
		t.Fatalf("case C non-queryable execution unexpectedly queried provider: gets=%d", providerLedger.gets.Load())
	}
}

func TestCrashMatrixDSubmittedThenProcessRestartQueriesOnly(t *testing.T) {
	db := openCrashMatrixDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "crash-d-" + time.Now().UTC().Format("20060102150405.000000000")
	defer deleteCrashMatrixExecution(t, db, taskID)

	// First process: provider accepts and the request ID is durably persisted.
	firstProcessStore := NewStore(db)
	claimed := createAndClaimExecution(t, firstProcessStore, taskID, "queryable-video", "video", "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd")
	providerLedger := &crashProviderLedger{result: QueryResult{Status: Succeeded, ProviderRequestID: "provider-d"}}
	firstAdapter := &crashCountingAdapter{ledger: providerLedger, providerRequestID: "provider-d"}
	if _, err := firstAdapter.Submit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := firstProcessStore.Transition(ctx, claimed.ID, Submitted, &firstAdapter.providerRequestID, nil, nil); err != nil {
		t.Fatal(err)
	}
	firstAdapter = nil
	firstProcessStore = nil

	// Second process / redelivery must perform Get/Query only.
	secondProcessStore := NewStore(db)
	latest, err := secondProcessStore.GetLatestByTask(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	secondAdapter := &crashCountingAdapter{ledger: providerLedger, providerRequestID: "provider-d"}
	recovered, err := (&Service{Store: secondProcessStore}).Recover(ctx, latest, secondAdapter)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != Succeeded || providerLedger.creates.Load() != 1 || providerLedger.gets.Load() != 1 {
		t.Fatalf("case D: status=%s creates=%d gets=%d", recovered.Status, providerLedger.creates.Load(), providerLedger.gets.Load())
	}
}

func TestCrashMatrixGConcurrentProcessClaimsOneExecution(t *testing.T) {
	db := openCrashMatrixDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "crash-g-" + time.Now().UTC().Format("20060102150405.000000000")
	defer deleteCrashMatrixExecution(t, db, taskID)

	providerLedger := &crashProviderLedger{}
	execution := Execution{
		TaskID:             taskID,
		Provider:           "grok-imagine-1.5",
		Capability:         "image",
		RequestFingerprint: "gggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggg",
	}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			// Every goroutine represents a separate consumer process: it creates
			// its own store and adapter and shares no mutex or local execution map.
			processStore := NewStore(db)
			adapter := &crashCountingAdapter{ledger: providerLedger, providerRequestID: "provider-g"}
			_, err := (&Service{Store: processStore}).Execute(ctx, execution, adapter)
			results <- err
		}()
	}

	var successes int
	var lastErr error
	for i := 0; i < 2; i++ {
		if err := <-results; err == nil {
			successes++
		} else {
			lastErr = err
		}
	}
	if successes != 1 {
		t.Fatalf("case G claim owners=%d lastErr=%v", successes, lastErr)
	}
	if providerLedger.creates.Load() != 1 {
		t.Fatalf("case G BLIND_RESUBMIT_PATHS=FOUND: creates=%d", providerLedger.creates.Load())
	}
}

func TestProviderGetFailedRecoveryReturnsExplicitFailure(t *testing.T) {
	db := openCrashMatrixDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "failed-recovery-" + time.Now().UTC().Format("20060102150405.000000000")
	defer deleteCrashMatrixExecution(t, db, taskID)

	store := NewStore(db)
	claimed := createAndClaimExecution(t, store, taskID, "queryable-video", "video", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	providerLedger := &crashProviderLedger{result: QueryResult{Status: Failed, ProviderRequestID: "provider-failed"}}
	if err := store.Transition(ctx, claimed.ID, Submitted, stringPtr("provider-failed"), nil, nil); err != nil {
		t.Fatal(err)
	}
	latest, err := store.GetLatestByTask(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := (&Service{Store: NewStore(db)}).Recover(ctx, latest, &crashCountingAdapter{ledger: providerLedger, providerRequestID: "provider-failed"})
	if !errors.Is(err, ErrProviderExecutionFailed) {
		t.Fatalf("FAILED_PROVIDER_RECOVERY=FAIL: err=%v", err)
	}
	if recovered.Status != Failed || providerLedger.creates.Load() != 0 || providerLedger.gets.Load() != 1 {
		t.Fatalf("failed recovery: status=%s creates=%d gets=%d", recovered.Status, providerLedger.creates.Load(), providerLedger.gets.Load())
	}
}

func stringPtr(value string) *string { return &value }
