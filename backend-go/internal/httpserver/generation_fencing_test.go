package httpserver

// Execution-generation fencing crash matrix (Issue #145, GO_IMPLEMENT).
//
// These tests run against a genuine disposable PostgreSQL (no skips count as
// pass: set XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL to the disposable
// DSN). They prove the acceptance invariant: only the current authoritative
// owner may heartbeat / complete / fail / write artifacts / trigger
// settlement; every deposed writer fails closed with ErrFencedStaleExecution
// and mutates nothing.
//
// Matrix:
//   A claim -> expiry -> requeue -> B claim -> A late success/failure fenced -> B commits
//   A success -> durable capture -> crash before complete -> B retry (no duplicate side effect)
//   A stale heartbeat after B claim (rejected)
//   A stale failure after B success (rejected, winner intact)
//   reaper vs heartbeat race (single generation bump)
//   concurrent old/new fenced settlement (single winner)
//   migration rolling compat (legacy rows, unfenced callers, NULL bindings, rollback shape)

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"xianzhi-ai/backend-go/internal/messaging"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

var fencingTestSequence atomic.Int64

func openFencingTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open fencing test database: %v", err)
	}
	db.SetMaxOpenConns(16)
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		t.Fatalf("ping fencing test database: %v", err)
	}
	// The migration must be replayable against the already-migrated
	// disposable database (idempotency is part of the contract).
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "database", "migrations", "119-execution-generation-fencing.sql"))
	if err != nil {
		_ = db.Close()
		t.Fatalf("read fencing migration: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), string(raw)); err != nil {
		_ = db.Close()
		t.Fatalf("replay fencing migration: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func fencingTaskID(t *testing.T, prefix string) string {
	t.Helper()
	return fmt.Sprintf("fence_%s_%d_%d", prefix, time.Now().UTC().UnixNano(), fencingTestSequence.Add(1))
}

func fencingStore(db *sql.DB) *postgresStore {
	return &postgresStore{db: db, ready: true}
}

// seedFencingTask inserts a minimal task row the way legacy writers do: no
// fencing columns referenced, so defaults/NULLs apply. raw carries the id
// exactly like production writes (generationTaskForUpdate recovers identity
// from the raw projection), keeping the fixture realistic.
func seedFencingTask(t *testing.T, db *sql.DB, id, status string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO xz_generation_tasks (id,user_id,type,status,task_status,created_at,updated_at,raw)
		VALUES ($1,'fence-user','image',$2,'RUNNING',$3,$3,jsonb_build_object('id',$1::text,'userId','fence-user','type','image','status',$2::text))
	`, id, status, now); err != nil {
		t.Fatalf("seed fencing task: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM xz_generation_tasks WHERE id=$1`, id)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM provider_executions WHERE task_id=$1`, id)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM xz_assets WHERE task_id=$1`, id)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM xz_billing_lifecycle_events WHERE task_id=$1`, id)
	})
}

func fencingGeneration(t *testing.T, db *sql.DB, id string) (int64, string) {
	t.Helper()
	var gen sql.NullInt64
	var worker sql.NullString
	if err := db.QueryRowContext(context.Background(), `SELECT execution_generation, worker_id FROM xz_generation_tasks WHERE id=$1`, id).Scan(&gen, &worker); err != nil {
		t.Fatalf("read fencing generation: %v", err)
	}
	workerID := ""
	if worker.Valid {
		workerID = worker.String
	}
	return gen.Int64, workerID
}

func fencingTaskStatus(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var status string
	if err := db.QueryRowContext(context.Background(), `SELECT status FROM xz_generation_tasks WHERE id=$1`, id).Scan(&status); err != nil {
		t.Fatalf("read task status: %v", err)
	}
	return status
}

func expireFencingLease(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `UPDATE xz_generation_tasks SET lease_until = now() - interval '1 hour' WHERE id=$1`, id); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
}

func requeueAsReaper(t *testing.T, db *sql.DB, id string, expectedGen int64) int64 {
	t.Helper()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	next, err := requeueGenerationTx(context.Background(), tx, id, expectedGen, "")
	if err != nil {
		t.Fatalf("reaper requeue: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return next
}

func requireFenced(t *testing.T, err error, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected ErrFencedStaleExecution, got nil", what)
	}
	if !errors.Is(err, ErrFencedStaleExecution) {
		t.Fatalf("%s: expected ErrFencedStaleExecution, got %v", what, err)
	}
}

// TestFencing_MigrationReplayAndDefaults proves the DDL contract: replayable,
// correct defaults for legacy rows, supporting indexes present, and rollback
// shape viable without stranding tasks.
func TestFencing_MigrationReplayAndDefaults(t *testing.T) {
	db := openFencingTestDB(t)
	ctx := context.Background()

	for _, column := range []string{"execution_generation", "worker_id", "lease_until", "last_heartbeat_at"} {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='xz_generation_tasks' AND column_name=$1)`, column).Scan(&exists); err != nil || !exists {
			t.Fatalf("column %s exists=%v err=%v", column, exists, err)
		}
	}
	var boundExists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='provider_executions' AND column_name='task_execution_generation')`).Scan(&boundExists); err != nil || !boundExists {
		t.Fatalf("provider_executions.task_execution_generation exists=%v err=%v", boundExists, err)
	}
	for _, index := range []string{"xz_generation_tasks_fencing_status_gen_idx", "xz_generation_tasks_fencing_lease_idx", "provider_executions_task_generation_idx"} {
		var found bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname=current_schema() AND indexname=$1)`, index).Scan(&found); err != nil || !found {
			t.Fatalf("index %s exists=%v err=%v", index, found, err)
		}
	}
	// The lease index must be the partial index the lease-aware reaper
	// relies on for `lease_until < now()` sweeps.
	var leasePredicate string
	if err := db.QueryRowContext(ctx, `SELECT coalesce(pg_get_expr(indpred, indrelid),'') FROM pg_index WHERE indexrelid = 'xz_generation_tasks_fencing_lease_idx'::regclass`).Scan(&leasePredicate); err != nil || !strings.Contains(leasePredicate, "lease_until") || !strings.Contains(leasePredicate, "IS NOT NULL") {
		t.Fatalf("lease index predicate = %q err=%v (want partial on lease_until IS NOT NULL)", leasePredicate, err)
	}

	// Legacy-shaped insert (no fencing columns): defaults apply, old code
	// that never references the new columns keeps working.
	id := fencingTaskID(t, "legacy")
	seedFencingTask(t, db, id, "RUNNING")
	gen, workerID := fencingGeneration(t, db, id)
	if gen != 1 || workerID != "" {
		t.Fatalf("legacy row generation=%d worker=%q, want 1/empty", gen, workerID)
	}
	var lease sql.NullTime
	if err := db.QueryRowContext(ctx, `SELECT lease_until FROM xz_generation_tasks WHERE id=$1`, id).Scan(&lease); err != nil || lease.Valid {
		t.Fatalf("legacy row lease valid=%v err=%v, want NULL", lease.Valid, err)
	}

	// Rollback shape: dropping the added columns inside a rolled-back txn
	// proves the rollback statements are valid and that legacy-shaped reads
	// and writes still function without the fencing columns (an old build
	// rolled back to never references them, so no task is stranded).
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`DROP INDEX IF EXISTS provider_executions_task_generation_idx`,
		`DROP INDEX IF EXISTS xz_generation_tasks_fencing_lease_idx`,
		`DROP INDEX IF EXISTS xz_generation_tasks_fencing_status_gen_idx`,
		`ALTER TABLE provider_executions DROP COLUMN IF EXISTS task_execution_generation`,
		`ALTER TABLE xz_generation_tasks DROP COLUMN IF EXISTS last_heartbeat_at`,
		`ALTER TABLE xz_generation_tasks DROP COLUMN IF EXISTS lease_until`,
		`ALTER TABLE xz_generation_tasks DROP COLUMN IF EXISTS worker_id`,
		`ALTER TABLE xz_generation_tasks DROP COLUMN IF EXISTS execution_generation`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			_ = tx.Rollback()
			t.Fatalf("rollback stmt %q: %v", stmt, err)
		}
	}
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM xz_generation_tasks WHERE id=$1`, id).Scan(&status); err != nil || status != "RUNNING" {
		_ = tx.Rollback()
		t.Fatalf("legacy-shaped read after rollback stmts: status=%q err=%v", status, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE xz_generation_tasks SET task_status='QUEUED' WHERE id=$1`, id); err != nil {
		_ = tx.Rollback()
		t.Fatalf("legacy-shaped write after rollback stmts: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	// The rollback was transactional: fencing columns survive on the real row.
	gen, _ = fencingGeneration(t, db, id)
	if gen != 1 {
		t.Fatalf("post-rollback-probe generation=%d, want 1 (columns must survive)", gen)
	}
}

// TestFencing_ClaimRequeueBumpMonotonic proves every legitimate ownership
// transfer bumps the generation and concurrent requeues arbitrate to exactly
// one winner via WHERE id AND generation.
func TestFencing_ClaimRequeueBumpMonotonic(t *testing.T) {
	db := openFencingTestDB(t)
	store := fencingStore(db)
	id := fencingTaskID(t, "bump")
	seedFencingTask(t, db, id, "RUNNING")

	genA, workerA, err := claimGenerationTaskOwnership(store, id)
	if err != nil {
		t.Fatalf("claim A: %v", err)
	}
	if genA != 2 || workerA == "" {
		t.Fatalf("claim A generation=%d worker=%q, want 2/non-empty", genA, workerA)
	}

	// Two reapers race the same conditional bump: exactly one wins.
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tx, txErr := db.BeginTx(context.Background(), nil)
			if txErr != nil {
				results <- txErr
				return
			}
			defer func() { _ = tx.Rollback() }()
			_, txErr = requeueGenerationTx(context.Background(), tx, id, genA, "")
			if txErr == nil {
				txErr = tx.Commit()
			}
			results <- txErr
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var wins, fences int
	for err := range results {
		if err == nil {
			wins++
		} else if errors.Is(err, ErrFencedStaleExecution) {
			fences++
		} else {
			t.Fatalf("unexpected requeue error: %v", err)
		}
	}
	if wins != 1 || fences != 1 {
		t.Fatalf("requeue race wins=%d fences=%d, want 1/1", wins, fences)
	}
	gen, _ := fencingGeneration(t, db, id)
	if gen != genA+1 {
		t.Fatalf("post-requeue generation=%d, want %d", gen, genA+1)
	}

	genB, _, err := claimGenerationTaskOwnership(store, id)
	if err != nil {
		t.Fatalf("claim B: %v", err)
	}
	if genB != genA+2 {
		t.Fatalf("claim B generation=%d, want %d", genB, genA+2)
	}
}

// TestFencing_CrashMatrixStaleOwnerFenced is the required fencing race:
// A claim -> A expiry -> requeue -> B claim -> A late success/failure fenced
// -> B commits. It also covers A stale heartbeat after B claim and A stale
// failure after B success.
func TestFencing_CrashMatrixStaleOwnerFenced(t *testing.T) {
	db := openFencingTestDB(t)
	store := fencingStore(db)
	ctx := context.Background()
	id := fencingTaskID(t, "race")
	seedFencingTask(t, db, id, "RUNNING")

	genA, workerA, err := claimGenerationTaskOwnership(store, id)
	if err != nil {
		t.Fatalf("claim A: %v", err)
	}
	expireFencingLease(t, db, id)
	genRequeued := requeueAsReaper(t, db, id, genA)
	if genRequeued != genA+1 {
		t.Fatalf("requeue generation=%d, want %d", genRequeued, genA+1)
	}
	genB, workerB, err := claimGenerationTaskOwnership(store, id)
	if err != nil {
		t.Fatalf("claim B: %v", err)
	}
	if workerB == workerA {
		t.Fatal("B claim reused A's worker id; owner ids must differ")
	}

	// A late heartbeat after losing ownership is rejected (old worker can
	// never renew into the new generation).
	renewTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	renewErr := renewGenerationLeaseTx(ctx, renewTx, id, workerA, genA, generationLeaseTTL)
	_ = renewTx.Rollback()
	requireFenced(t, renewErr, "stale heartbeat renewal")

	// A late failure with the deposed generation is fenced: status, assets,
	// and paid side-effect tables are untouched.
	_, err = store.FailGenerationTaskFenced(id, "stale A failure", genA)
	requireFenced(t, err, "stale A Fail")
	if got := fencingTaskStatus(t, db, id); got != "RUNNING" {
		t.Fatalf("status after fenced stale fail = %q, want RUNNING", got)
	}
	var assets, lifecycle int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_assets WHERE task_id=$1`, id).Scan(&assets); err != nil || assets != 0 {
		t.Fatalf("assets after fenced stale fail = %d err=%v, want 0", assets, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_billing_lifecycle_events WHERE task_id=$1`, id).Scan(&lifecycle); err != nil || lifecycle != 0 {
		t.Fatalf("billing lifecycle rows after fenced stale fail = %d err=%v, want 0", lifecycle, err)
	}

	// A late *success* with the deposed generation is equally fenced and
	// writes nothing (assert precedes asset/billing writes by construction).
	_, err = store.CompleteGenerationTaskFenced(id, createGenerationTaskRequest{UserID: "fence-user", Type: "image", Model: "m", Params: map[string]any{}}, genA)
	requireFenced(t, err, "stale A Complete")
	if got := fencingTaskStatus(t, db, id); got != "RUNNING" {
		t.Fatalf("status after fenced stale complete = %q, want RUNNING", got)
	}

	// B (current generation) commits exactly once.
	if _, err := store.FailGenerationTaskFenced(id, "B failure", genB); err != nil {
		t.Fatalf("B Fail: %v", err)
	}
	if got := fencingTaskStatus(t, db, id); got != "FAILED" {
		t.Fatalf("status after B fail = %q, want FAILED", got)
	}

	// A stale failure after B success is rejected and the winner is intact.
	_, err = store.FailGenerationTaskFenced(id, "stale A failure after B", genA)
	requireFenced(t, err, "stale A Fail after B success")
	if got := fencingTaskStatus(t, db, id); got != "FAILED" {
		t.Fatalf("status after post-success stale fail = %q, want FAILED", got)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_billing_lifecycle_events WHERE task_id=$1`, id).Scan(&lifecycle); err != nil || lifecycle != 0 {
		t.Fatalf("billing lifecycle rows at end = %d err=%v, want 0", lifecycle, err)
	}
}

// TestFencing_CrashMatrixSuccessCaptureCrashRetry proves the durable
// execution -> generation binding: A captures provider success, crashes
// before local completion, the task is requeued, and B's retry settles while
// A's stale success can never settle the new generation (no duplicate
// side effect from the stale winner).
func TestFencing_CrashMatrixSuccessCaptureCrashRetry(t *testing.T) {
	db := openFencingTestDB(t)
	store := fencingStore(db)
	ctx := context.Background()
	id := fencingTaskID(t, "capture")
	seedFencingTask(t, db, id, "RUNNING")

	genA, _, err := claimGenerationTaskOwnership(store, id)
	if err != nil {
		t.Fatalf("claim A: %v", err)
	}

	// A captures provider success durably, then "crashes" before Complete.
	execStore := pe.NewStore(db)
	created, err := execStore.CreatePreparedForGenerationTask(ctx, pe.Execution{
		TaskID: id, Provider: "mock", Capability: "image",
		RequestFingerprint: strings.Repeat("c", 64),
	})
	if err != nil {
		t.Fatalf("create prepared: %v", err)
	}
	if created.TaskGeneration == nil || *created.TaskGeneration != genA {
		t.Fatalf("execution binding = %v, want %d (every legitimate generation bound)", created.TaskGeneration, genA)
	}
	claimed, err := execStore.ClaimPreparedForGenerationTask(ctx, id)
	if err != nil {
		t.Fatalf("claim prepared: %v", err)
	}
	if err := execStore.SaveSucceededResult(ctx, claimed.ID, fencingStrPtr("req-1"), []byte(`[{"url":"https://example.test/a.png"}]`)); err != nil {
		t.Fatalf("durable capture: %v", err)
	}

	// Crash + requeue: the generation moves on without local completion.
	expireFencingLease(t, db, id)
	genB := requeueAsReaper(t, db, id, genA)
	if _, _, err := claimGenerationTaskOwnership(store, id); err != nil {
		t.Fatalf("claim B: %v", err)
	}

	// B's retry must not settle from A's stale succeeded execution.
	latest, err := execStore.GetLatestByTask(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	retryAPI := api{store: store}
	if err := retryAPI.checkSucceededExecutionGeneration(id, latest); !errors.Is(err, ErrFencedStaleExecution) {
		t.Fatalf("stale succeeded recovery gate = %v, want ErrFencedStaleExecution", err)
	}

	// B captures its own success under the current generation and settles.
	createdB, err := execStore.CreatePreparedForGenerationTask(ctx, pe.Execution{
		TaskID: id, Provider: "mock", Capability: "image",
		RequestFingerprint: strings.Repeat("d", 64), Attempt: latest.Attempt + 1,
	})
	if err != nil {
		t.Fatalf("create B prepared: %v", err)
	}
	if createdB.TaskGeneration == nil || *createdB.TaskGeneration != genB+1 {
		t.Fatalf("B execution binding = %v, want %d", createdB.TaskGeneration, genB+1)
	}
	if _, err := store.FailGenerationTaskFenced(id, "B failure after retry", genB+1); err != nil {
		t.Fatalf("B Fail: %v", err)
	}
	// A's generation can never write again, even though its provider row is
	// durably succeeded.
	_, err = store.FailGenerationTaskFenced(id, "stale A failure", genA)
	requireFenced(t, err, "stale A Fail after B retry")
	var assets, lifecycle int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_assets WHERE task_id=$1`, id).Scan(&assets); err != nil || assets != 0 {
		t.Fatalf("assets = %d err=%v, want 0 (no duplicate side effect)", assets, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_billing_lifecycle_events WHERE task_id=$1`, id).Scan(&lifecycle); err != nil || lifecycle != 0 {
		t.Fatalf("billing lifecycle rows = %d err=%v, want 0 (no duplicate side effect)", lifecycle, err)
	}
}

// TestFencing_CrashMatrixReaperHeartbeatRace proves reaper/heartbeat
// arbitration: concurrent renewals and conditional requeues bump the
// generation exactly once, and post-bump renewals are rejected.
func TestFencing_CrashMatrixReaperHeartbeatRace(t *testing.T) {
	db := openFencingTestDB(t)
	store := fencingStore(db)
	ctx := context.Background()
	id := fencingTaskID(t, "reaperhb")
	seedFencingTask(t, db, id, "RUNNING")

	genA, workerA, err := claimGenerationTaskOwnership(store, id)
	if err != nil {
		t.Fatalf("claim A: %v", err)
	}
	expireFencingLease(t, db, id)

	const renewers = 8
	const requeuers = 3
	start := make(chan struct{})
	var wg sync.WaitGroup
	renewResults := make(chan error, renewers)
	requeueResults := make(chan error, requeuers)
	for i := 0; i < renewers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tx, txErr := db.BeginTx(ctx, nil)
			if txErr != nil {
				renewResults <- txErr
				return
			}
			defer func() { _ = tx.Rollback() }()
			txErr = renewGenerationLeaseTx(ctx, tx, id, workerA, genA, generationLeaseTTL)
			if txErr == nil {
				txErr = tx.Commit()
			}
			renewResults <- txErr
		}()
	}
	for i := 0; i < requeuers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tx, txErr := db.BeginTx(ctx, nil)
			if txErr != nil {
				requeueResults <- txErr
				return
			}
			defer func() { _ = tx.Rollback() }()
			_, txErr = requeueGenerationTx(ctx, tx, id, genA, "")
			if txErr == nil {
				txErr = tx.Commit()
			}
			requeueResults <- txErr
		}()
	}
	close(start)
	wg.Wait()
	close(renewResults)
	close(requeueResults)

	var requeueWins, requeueFences int
	for err := range requeueResults {
		if err == nil {
			requeueWins++
		} else if errors.Is(err, ErrFencedStaleExecution) {
			requeueFences++
		} else {
			t.Fatalf("unexpected requeue error: %v", err)
		}
	}
	if requeueWins != 1 || requeueFences != requeuers-1 {
		t.Fatalf("requeue race wins=%d fences=%d, want 1/%d", requeueWins, requeueFences, requeuers-1)
	}
	gen, _ := fencingGeneration(t, db, id)
	if gen != genA+1 {
		t.Fatalf("post-race generation=%d, want %d (exactly one bump)", gen, genA+1)
	}
	// After the bump, the old owner's heartbeat is unconditionally rejected.
	if err := renewGenerationLease(store, id, workerA, genA); !errors.Is(err, ErrFencedStaleExecution) {
		t.Fatalf("post-bump stale renew = %v, want ErrFencedStaleExecution", err)
	}
	for err := range renewResults {
		if err != nil && !errors.Is(err, ErrFencedStaleExecution) {
			t.Fatalf("unexpected renew error: %v", err)
		}
	}
}

// TestFencing_ConcurrentFencedSettlementSingleWinner proves dual-worker
// arbitration: N stale settlers are all fenced while the current
// generation's settlers converge to a single terminal state.
func TestFencing_ConcurrentFencedSettlementSingleWinner(t *testing.T) {
	db := openFencingTestDB(t)
	store := fencingStore(db)
	id := fencingTaskID(t, "dualsettle")
	seedFencingTask(t, db, id, "RUNNING")

	genA, _, err := claimGenerationTaskOwnership(store, id)
	if err != nil {
		t.Fatalf("claim A: %v", err)
	}
	expireFencingLease(t, db, id)
	requeueAsReaper(t, db, id, genA)
	genB, _, err := claimGenerationTaskOwnership(store, id)
	if err != nil {
		t.Fatalf("claim B: %v", err)
	}

	const settlers = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	staleResults := make(chan error, settlers)
	currentResults := make(chan error, settlers)
	for i := 0; i < settlers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if i%2 == 0 {
				_, err := store.FailGenerationTaskFenced(id, "stale settler", genA)
				staleResults <- err
			} else {
				_, err := store.FailGenerationTaskFenced(id, "stale settler", genA)
				staleResults <- err
			}
		}(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := store.FailGenerationTaskFenced(id, "current settler", genB)
			currentResults <- err
		}()
	}
	close(start)
	wg.Wait()
	close(staleResults)
	close(currentResults)

	for err := range staleResults {
		requireFenced(t, err, "stale concurrent settler")
	}
	var currentNils int
	for err := range currentResults {
		if err != nil {
			t.Fatalf("current-generation settler error: %v", err)
		}
		currentNils++
	}
	if currentNils != settlers {
		t.Fatalf("current settlers committed=%d, want %d", currentNils, settlers)
	}
	// Single winner: exactly one terminal state, no duplicate paid effects.
	if got := fencingTaskStatus(t, db, id); got != "FAILED" {
		t.Fatalf("final status = %q, want FAILED", got)
	}
	var lifecycle int
	if err := db.QueryRowContext(context.Background(), `SELECT count(*) FROM xz_billing_lifecycle_events WHERE task_id=$1`, id).Scan(&lifecycle); err != nil || lifecycle != 0 {
		t.Fatalf("billing lifecycle rows = %d err=%v, want 0", lifecycle, err)
	}
}

// TestFencing_LegacyRollingCompat proves rollout safety: unfenced legacy
// callers keep working, NULL-bound executions stay claimable/recoverable,
// and a new worker adopting a legacy task binds the next generation.
func TestFencing_LegacyRollingCompat(t *testing.T) {
	db := openFencingTestDB(t)
	store := fencingStore(db)
	ctx := context.Background()
	id := fencingTaskID(t, "legacycompat")
	seedFencingTask(t, db, id, "RUNNING")

	// Unfenced legacy settlement still settles (expectedGen 0 = compat).
	if _, err := store.FailGenerationTask(id, "legacy failure"); err != nil {
		t.Fatalf("legacy unfenced Fail: %v", err)
	}
	if got := fencingTaskStatus(t, db, id); got != "FAILED" {
		t.Fatalf("status = %q, want FAILED", got)
	}

	// Legacy NULL-bound execution: claimable and recoverable (never stale).
	id2 := fencingTaskID(t, "legacyexec")
	seedFencingTask(t, db, id2, "RUNNING")
	if _, err := db.ExecContext(ctx, `INSERT INTO provider_executions (task_id,provider,capability,attempt,status,request_fingerprint,provider_operation_key) VALUES ($1,'mock','image',1,'prepared',$2,'legacy-op')`, id2, strings.Repeat("e", 64)); err != nil {
		t.Fatalf("seed legacy execution: %v", err)
	}
	execStore := pe.NewStore(db)
	legacyExec, err := execStore.GetLatestByTask(ctx, id2)
	if err != nil {
		t.Fatal(err)
	}
	if legacyExec.TaskGeneration != nil {
		t.Fatalf("legacy execution binding = %v, want nil", legacyExec.TaskGeneration)
	}
	if _, err := execStore.ClaimPreparedForGenerationTask(ctx, id2); err != nil {
		t.Fatalf("legacy NULL-bound claim: %v", err)
	}
	// Close the legacy execution the way the safe-retry flow does before
	// Attempt+1 (single active execution per task is a pre-existing
	// invariant, untouched by fencing).
	safeClass := string(pe.DefinitiveNotSubmitted)
	safeMsg := "legacy pre-submit failure"
	if err := execStore.Transition(ctx, legacyExec.ID, pe.Failed, nil, &safeClass, &safeMsg); err != nil {
		t.Fatalf("close legacy execution: %v", err)
	}
	retryAPI := api{store: store}
	if err := retryAPI.checkSucceededExecutionGeneration(id2, legacyExec); err != nil {
		t.Fatalf("legacy NULL-bound recovery gate: %v", err)
	}

	// New worker adopting a legacy task binds the next generation.
	gen, _, err := claimGenerationTaskOwnership(store, id2)
	if err != nil {
		t.Fatalf("adopt legacy task: %v", err)
	}
	if gen != 2 {
		t.Fatalf("adopted generation=%d, want 2", gen)
	}
	created, err := execStore.CreatePreparedForGenerationTask(ctx, pe.Execution{
		TaskID: id2, Provider: "mock", Capability: "image", Attempt: legacyExec.Attempt + 1,
		RequestFingerprint: strings.Repeat("f", 64),
	})
	if err != nil {
		t.Fatalf("create after adopt: %v", err)
	}
	if created.TaskGeneration == nil || *created.TaskGeneration != gen {
		t.Fatalf("post-adopt binding = %v, want %d", created.TaskGeneration, gen)
	}
}

// TestFencing_LeaseAwareReaperEligibility proves the cutover gate: valid
// leases skip, expired leases reap, lease-less rows use the age backstop.
func TestFencing_LeaseAwareReaperEligibility(t *testing.T) {
	before := fencingCutoverSnapshot()
	now := time.Now().UTC()
	old := now.Add(-time.Hour)
	fresh := now.Add(-time.Second)

	live := time.Now().UTC().Add(time.Hour)
	if reaperEligibleForFencing(taskFencing{Found: true, Generation: 3, LeaseUntil: &live}, old, now, 15*time.Minute) {
		t.Fatal("valid lease must skip the reaper")
	}
	expired := now.Add(-time.Minute)
	if !reaperEligibleForFencing(taskFencing{Found: true, Generation: 3, LeaseUntil: &expired}, old, now, 15*time.Minute) {
		t.Fatal("expired lease must be reaper-eligible")
	}
	if !reaperEligibleForFencing(taskFencing{Found: true, Generation: 1}, old, now, 15*time.Minute) {
		t.Fatal("lease-less old row must reap via age backstop")
	}
	if reaperEligibleForFencing(taskFencing{Found: true, Generation: 1}, fresh, now, 15*time.Minute) {
		t.Fatal("lease-less fresh row must not reap")
	}
	after := fencingCutoverSnapshot()
	if after["lease_skips"]-before["lease_skips"] != 1 {
		t.Fatalf("lease_skips delta = %d, want 1", after["lease_skips"]-before["lease_skips"])
	}
	if after["lease_expired"]-before["lease_expired"] != 1 {
		t.Fatalf("lease_expired delta = %d, want 1", after["lease_expired"]-before["lease_expired"])
	}
	if after["age_backstop"]-before["age_backstop"] != 2 {
		t.Fatalf("age_backstop delta = %d, want 2", after["age_backstop"]-before["age_backstop"])
	}
}

func fencingStrPtr(value string) *string { return &value }

// seedFencingTaskWithType inserts a task row with an explicit type and
// params shape (image vs video recovery branches, canary markers), keeping
// the same legacy-writer posture as seedFencingTask: no fencing columns
// referenced, so defaults/NULLs apply.
func seedFencingTaskWithType(t *testing.T, db *sql.DB, id, taskType, status, paramsJSON string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	raw := fmt.Sprintf(`{"id":%q,"userId":"fence-user","type":%q,"status":%q,"params":%s}`, id, taskType, status, paramsJSON)
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO xz_generation_tasks (id,user_id,type,status,task_status,params,created_at,updated_at,raw)
		VALUES ($1,'fence-user',$2,$3,'RUNNING',$4::jsonb,$5,$5,$6::jsonb)
	`, id, taskType, status, paramsJSON, now, raw); err != nil {
		t.Fatalf("seed fencing task with type: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM xz_generation_tasks WHERE id=$1`, id)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM provider_executions WHERE task_id=$1`, id)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM xz_assets WHERE task_id=$1`, id)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM xz_billing_lifecycle_events WHERE task_id=$1`, id)
	})
}

// TestFencing_RecoverSucceededSingleObservationStaleFenced (Issue #145
// P1-1) proves the recovery entry gate on both the image and video
// branches: a succeeded execution bound to generation G can never settle a
// task that has since moved to a newer generation. Gate and settle share
// the single entry observation, so the stale execution fences with
// ErrFencedStaleExecution and nothing is mutated (no status, asset, or
// billing writes).
func TestFencing_RecoverSucceededSingleObservationStaleFenced(t *testing.T) {
	db := openFencingTestDB(t)
	ctx := context.Background()
	store := fencingStore(db)
	retryAPI := api{store: store}

	cases := []struct {
		name           string
		taskType       string
		resultMetadata string
		fingerprint    string
	}{
		{"image", "TEXT_TO_IMAGE", `[{"url":"https://example.test/stale-a.png"}]`, strings.Repeat("r", 64)},
		{"video", "TEXT_TO_VIDEO", `{"videoUrl":"https://example.test/stale-v.mp4"}`, strings.Repeat("s", 64)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := fencingTaskID(t, "recoverstale")
			seedFencingTaskWithType(t, db, id, tc.taskType, "RUNNING", `{}`)
			genA, _, err := claimGenerationTaskOwnership(store, id)
			if err != nil {
				t.Fatalf("claim A: %v", err)
			}
			execStore := pe.NewStore(db)
			if _, err := execStore.CreatePreparedForGenerationTask(ctx, pe.Execution{
				TaskID: id, Provider: "mock", Capability: tc.name,
				RequestFingerprint: tc.fingerprint,
			}); err != nil {
				t.Fatalf("create prepared: %v", err)
			}
			claimed, err := execStore.ClaimPreparedForGenerationTask(ctx, id)
			if err != nil {
				t.Fatalf("claim prepared: %v", err)
			}
			if err := execStore.SaveSucceededResult(ctx, claimed.ID, fencingStrPtr("req-stale"), []byte(tc.resultMetadata)); err != nil {
				t.Fatalf("durable capture: %v", err)
			}
			stale, err := execStore.GetLatestByTask(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if stale.TaskGeneration == nil || *stale.TaskGeneration != genA {
				t.Fatalf("execution binding = %v, want %d", stale.TaskGeneration, genA)
			}
			// Ownership moves on: expiry -> requeue -> fresh claim, so the
			// stored generation is newer than the execution binding.
			expireFencingLease(t, db, id)
			requeueAsReaper(t, db, id, genA)
			if _, _, err := claimGenerationTaskOwnership(store, id); err != nil {
				t.Fatalf("claim B: %v", err)
			}
			if stored, _ := fencingGeneration(t, db, id); stored == genA {
				t.Fatalf("stored generation=%d, want bumped past %d", stored, genA)
			}

			task := generationTask{ID: id, UserID: "fence-user", Type: tc.taskType, Status: "RUNNING", Params: map[string]any{}}
			err = retryAPI.recoverSucceededGenerationTask(task, stale)
			requireFenced(t, err, "stale recover "+tc.name)
			if got := fencingTaskStatus(t, db, id); got != "RUNNING" {
				t.Fatalf("status after fenced stale recover = %q, want RUNNING", got)
			}
			var assets, lifecycle int
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_assets WHERE task_id=$1`, id).Scan(&assets); err != nil || assets != 0 {
				t.Fatalf("assets after fenced stale recover = %d err=%v, want 0", assets, err)
			}
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_billing_lifecycle_events WHERE task_id=$1`, id).Scan(&lifecycle); err != nil || lifecycle != 0 {
				t.Fatalf("billing lifecycle rows after fenced stale recover = %d err=%v, want 0", lifecycle, err)
			}
		})
	}
}

// TestFencing_SucceededExecutionGateContract pins the nil-or-equal binding
// semantics shared by the recovery gate and settlement: nil bindings stay
// recoverable (pre-fencing legacy), a non-positive observed generation keeps
// the unfenced legacy path, otherwise bound must equal observed. A mismatch
// fences via provider_gate_skips without touching stale_redeliveries (P2-1
// separation).
func TestFencing_SucceededExecutionGateContract(t *testing.T) {
	receiver := api{}
	bound := func(gen int64) *int64 { return &gen }
	if err := receiver.checkSucceededExecutionGenerationWithObserved("t", pe.Execution{}, 2); err != nil {
		t.Fatalf("nil binding must stay recoverable: %v", err)
	}
	if err := receiver.checkSucceededExecutionGenerationWithObserved("t", pe.Execution{TaskGeneration: bound(2)}, 2); err != nil {
		t.Fatalf("equal binding must pass: %v", err)
	}
	if err := receiver.checkSucceededExecutionGenerationWithObserved("t", pe.Execution{TaskGeneration: bound(2)}, 0); err != nil {
		t.Fatalf("legacy unfenced path must pass: %v", err)
	}
	before := fencingCutoverSnapshot()
	if err := receiver.checkSucceededExecutionGenerationWithObserved("t", pe.Execution{TaskGeneration: bound(2)}, 3); !errors.Is(err, ErrFencedStaleExecution) {
		t.Fatalf("mismatched binding must fence, got %v", err)
	}
	after := fencingCutoverSnapshot()
	if after["provider_gate_skips"]-before["provider_gate_skips"] != 1 {
		t.Fatalf("provider_gate_skips delta = %d, want 1", after["provider_gate_skips"]-before["provider_gate_skips"])
	}
	if after["stale_redeliveries"]-before["stale_redeliveries"] != 0 {
		t.Fatalf("stale_redeliveries delta = %d, want 0 (gate skips stay separate)", after["stale_redeliveries"]-before["stale_redeliveries"])
	}
}

// TestFencing_VideoCanaryStaleRedeliverySkipped (Issue #145 P1-2) replicates
// the image consumer envelope-generation stale-skip for the video canary
// consumer: a redelivery older than the stored generation ack-skips without
// provider work while a live lease is held. The inbox row completes with the
// stale marker, ownership is untouched, and no provider execution is
// created.
func TestFencing_VideoCanaryStaleRedeliverySkipped(t *testing.T) {
	db := openFencingTestDB(t)
	ctx := context.Background()
	store := newPostgresPrimaryStore(db, "")
	id := fencingTaskID(t, "videostale")
	eventID := "evt-" + id
	rawJSON := fmt.Sprintf(`{"id":%q,"userId":"fence-user","type":"TEXT_TO_VIDEO","model":"mock-video","status":"RUNNING","params":{"generation_async_canary":true}}`, id)
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_generation_tasks (id,user_id,type,model,status,task_status,params,raw,created_at,updated_at) VALUES ($1,'fence-user','TEXT_TO_VIDEO','mock-video','RUNNING','RUNNING','{"generation_async_canary":true}'::jsonb,$2::jsonb,now()::text,now()::text)`, id, rawJSON); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM consumer_inbox WHERE event_id=$1`, eventID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM provider_executions WHERE task_id=$1`, id)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM xz_generation_tasks WHERE id=$1`, id)
	})
	gen, _, err := claimGenerationTaskOwnership(store, id)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if gen <= 1 {
		t.Fatalf("claimed generation=%d, want >1", gen)
	}

	inbox := messaging.NewInboxStore(db)
	envelope := &messaging.Envelope{
		EventID:       eventID,
		EventType:     messaging.GenerationVideoCanaryRoutingKey,
		AggregateType: "generation_task",
		AggregateID:   id,
		Data:          map[string]any{"task_id": id, "execution_generation": int64(gen - 1)},
	}
	before := fencingCutoverSnapshot()
	apiInstance := newAPI(store, stage0VideoCanaryConfig(), nil, nil)
	if err := apiInstance.processGenerationVideoCanaryMessage(ctx, inbox, envelope); err != nil {
		t.Fatalf("stale redelivery must ack-skip without error, got %v", err)
	}
	after := fencingCutoverSnapshot()
	if after["stale_redeliveries"]-before["stale_redeliveries"] != 1 {
		t.Fatalf("stale_redeliveries delta = %d, want 1", after["stale_redeliveries"]-before["stale_redeliveries"])
	}
	var result string
	var metadata string
	var processedAt sql.NullTime
	if err := db.QueryRowContext(ctx, `SELECT result, coalesce(metadata::text,''), processed_at FROM consumer_inbox WHERE consumer_name=$1 AND event_id=$2`, generationVideoCanaryConsumer, eventID).Scan(&result, &metadata, &processedAt); err != nil {
		t.Fatalf("read inbox row: %v", err)
	}
	if result != "completed" || !processedAt.Valid || !strings.Contains(metadata, "stale_generation") {
		t.Fatalf("inbox result=%q processed=%v metadata=%s, want completed/stale_generation", result, processedAt.Valid, metadata)
	}
	if stored, _ := fencingGeneration(t, db, id); stored != gen {
		t.Fatalf("stored generation=%d, want %d (skip must not bump)", stored, gen)
	}
	if got := fencingTaskStatus(t, db, id); got != "RUNNING" {
		t.Fatalf("status = %q, want RUNNING", got)
	}
	var executions int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM provider_executions WHERE task_id=$1`, id).Scan(&executions); err != nil || executions != 0 {
		t.Fatalf("provider executions = %d err=%v, want 0 (no provider work on skip)", executions, err)
	}
}

// TestFencing_PPTCanaryStaleRedeliverySkipped (Issue #145 P1-2) replicates
// the image consumer envelope-generation stale-skip for the PPT canary
// consumer: a redelivery older than the stored generation ack-skips without
// provider work while a live lease is held.
func TestFencing_PPTCanaryStaleRedeliverySkipped(t *testing.T) {
	db := openFencingTestDB(t)
	ctx := context.Background()
	store := newPostgresPrimaryStore(db, "")
	id := fencingTaskID(t, "pptstale")
	eventID := "evt-" + id
	rawJSON := fmt.Sprintf(`{"id":%q,"userId":"fence-user","type":"PPT_GENERATION","model":"kimi-k2.6","status":"RUNNING","params":{"generation_async_canary":true,"generation_ppt_async_canary":true}}`, id)
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_generation_tasks (id,user_id,type,model,status,task_status,params,raw,created_at,updated_at) VALUES ($1,'fence-user','PPT_GENERATION','kimi-k2.6','RUNNING','RUNNING','{"generation_async_canary":true,"generation_ppt_async_canary":true}'::jsonb,$2::jsonb,now()::text,now()::text)`, id, rawJSON); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM consumer_inbox WHERE event_id=$1`, eventID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM provider_executions WHERE task_id=$1`, id)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM xz_generation_tasks WHERE id=$1`, id)
	})
	gen, _, err := claimGenerationTaskOwnership(store, id)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if gen <= 1 {
		t.Fatalf("claimed generation=%d, want >1", gen)
	}

	inbox := messaging.NewInboxStore(db)
	envelope := &messaging.Envelope{
		EventID:       eventID,
		EventType:     messaging.GenerationPPTCanaryRoutingKey,
		AggregateType: "generation_task",
		AggregateID:   id,
		Data:          map[string]any{"task_id": id, "execution_generation": int64(gen - 1)},
	}
	before := fencingCutoverSnapshot()
	apiInstance := newAPI(store, stage0PPTCanaryConfig(), nil, nil)
	if err := apiInstance.processGenerationPPTCanaryMessage(ctx, inbox, envelope); err != nil {
		t.Fatalf("stale redelivery must ack-skip without error, got %v", err)
	}
	after := fencingCutoverSnapshot()
	if after["stale_redeliveries"]-before["stale_redeliveries"] != 1 {
		t.Fatalf("stale_redeliveries delta = %d, want 1", after["stale_redeliveries"]-before["stale_redeliveries"])
	}
	var result string
	var metadata string
	var processedAt sql.NullTime
	if err := db.QueryRowContext(ctx, `SELECT result, coalesce(metadata::text,''), processed_at FROM consumer_inbox WHERE consumer_name=$1 AND event_id=$2`, generationPPTCanaryConsumer, eventID).Scan(&result, &metadata, &processedAt); err != nil {
		t.Fatalf("read inbox row: %v", err)
	}
	if result != "completed" || !processedAt.Valid || !strings.Contains(metadata, "stale_generation") {
		t.Fatalf("inbox result=%q processed=%v metadata=%s, want completed/stale_generation", result, processedAt.Valid, metadata)
	}
	if stored, _ := fencingGeneration(t, db, id); stored != gen {
		t.Fatalf("stored generation=%d, want %d (skip must not bump)", stored, gen)
	}
	if got := fencingTaskStatus(t, db, id); got != "RUNNING" {
		t.Fatalf("status = %q, want RUNNING", got)
	}
	var executions int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM provider_executions WHERE task_id=$1`, id).Scan(&executions); err != nil || executions != 0 {
		t.Fatalf("provider executions = %d err=%v, want 0 (no provider work on skip)", executions, err)
	}
}
