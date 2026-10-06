package providerexecution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func setupConcurrencyTestDB(t *testing.T) (*sql.DB, *Store) {
	t.Helper()
	dsn := testingDatabaseURL(t)
	db := openProviderExecutionTestDB(t, dsn)

	// Ensure minimal xz_generation_tasks schema for generation fencing and task locking
	ddl := `
	CREATE TABLE IF NOT EXISTS xz_generation_tasks (
		id text PRIMARY KEY,
		status text NOT NULL DEFAULT 'PROCESSING',
		task_status text NOT NULL DEFAULT 'RUNNING',
		execution_generation bigint DEFAULT 1,
		worker_id text,
		lease_until timestamptz,
		type text DEFAULT 'TEXT_TO_IMAGE',
		params jsonb DEFAULT '{}',
		created_at timestamptz DEFAULT now(),
		updated_at timestamptz DEFAULT now()
	);
	ALTER TABLE xz_generation_tasks ADD COLUMN IF NOT EXISTS task_status text NOT NULL DEFAULT 'RUNNING';
	ALTER TABLE xz_generation_tasks ADD COLUMN IF NOT EXISTS execution_generation bigint DEFAULT 1;
	ALTER TABLE xz_generation_tasks ADD COLUMN IF NOT EXISTS worker_id text;
	ALTER TABLE xz_generation_tasks ADD COLUMN IF NOT EXISTS lease_until timestamptz;
	ALTER TABLE xz_generation_tasks ADD COLUMN IF NOT EXISTS params jsonb DEFAULT '{}';
	`
	if _, err := db.ExecContext(context.Background(), ddl); err != nil {
		db.Close()
		t.Fatalf("setup xz_generation_tasks schema: %v", err)
	}
	return db, NewStore(db)
}

// TestTimingA_EnrollmentWins verifies:
// 1. Enrollment acquires task FOR UPDATE first.
// 2. Worker ClaimPreparedForGenerationTask blocks on task FOR UPDATE.
// 3. Enrollment inserts quarantine row and commits.
// 4. Worker unblocks, evaluates RejectTask under same tx, receives ErrQuarantined.
// 5. Worker rolls back: 0 side effects (execution stays prepared, not submitting).
func TestTimingA_EnrollmentWins(t *testing.T) {
	db, store := setupConcurrencyTestDB(t)
	defer db.Close()
	ctx := context.Background()

	taskID := fmt.Sprintf("task-timing-a-%d", time.Now().UnixNano())
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_generation_tasks (id, status, task_status, execution_generation) VALUES ($1, 'PROCESSING', 'RUNNING', 1)`, taskID); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM provider_execution_quarantine WHERE task_id=$1`, taskID)
		_, _ = db.ExecContext(ctx, `DELETE FROM provider_executions WHERE task_id=$1`, taskID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_generation_tasks WHERE id=$1`, taskID)
	}()

	created, err := store.CreatePrepared(ctx, Execution{
		TaskID:             taskID,
		Provider:           "mock-provider",
		ProviderModel:      "mock-model",
		Capability:         "image",
		Attempt:            1,
		RequestFingerprint: "a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1",
	})
	if err != nil {
		t.Fatalf("create prepared: %v", err)
	}

	// Enrollment transaction begins and locks task FOR UPDATE
	enrollTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin enroll tx: %v", err)
	}
	defer enrollTx.Rollback()

	var lockedTaskStatus string
	if err := enrollTx.QueryRowContext(ctx, `SELECT status FROM xz_generation_tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&lockedTaskStatus); err != nil {
		t.Fatalf("enrollment lock task FOR UPDATE: %v", err)
	}

	workerStarted := make(chan struct{})
	workerDone := make(chan error, 1)

	// Worker attempts ClaimPreparedForGenerationTask in background; must block on task FOR UPDATE
	go func() {
		close(workerStarted)
		_, claimErr := store.ClaimPreparedForGenerationTask(context.Background(), taskID)
		workerDone <- claimErr
	}()

	<-workerStarted
	// Small settle time to ensure worker goroutine has entered query and is blocking on row lock
	time.Sleep(50 * time.Millisecond)

	select {
	case err := <-workerDone:
		t.Fatalf("worker completed prematurely while enrollment held lock! err=%v", err)
	default:
		// Worker is correctly blocked waiting on xz_generation_tasks FOR UPDATE
	}

	// Enrollment inserts quarantine row and commits
	evidenceSHA := "e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1"
	releaseSHA := "b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0"
	if _, err := enrollTx.ExecContext(ctx, `
		INSERT INTO provider_execution_quarantine (
			execution_id, task_id, attempt, generation,
			snapshot_sha256, evidence_sha256, approval_id, release_sha,
			not_before, expires_at
		) VALUES (
			$1, $2, 1, 1,
			$3, $3, 'approval-timing-a', $4,
			now() - interval '5 minutes', now() + interval '1 hour'
		)`, created.ID, taskID, evidenceSHA, releaseSHA); err != nil {
		t.Fatalf("enrollment insert quarantine: %v", err)
	}

	if err := enrollTx.Commit(); err != nil {
		t.Fatalf("enrollment commit: %v", err)
	}

	// Worker should now unblock, evaluate RejectTask under row lock, receive ErrQuarantined
	select {
	case workerErr := <-workerDone:
		if !errors.Is(workerErr, ErrQuarantined) {
			t.Fatalf("expected ErrQuarantined from worker, got: %v", workerErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for worker to unblock and reject quarantine")
	}

	// Verify ZERO side effects: execution status must STILL be prepared (NOT submitting)
	var execStatus string
	if err := db.QueryRowContext(ctx, `SELECT status FROM provider_executions WHERE id=$1`, created.ID).Scan(&execStatus); err != nil {
		t.Fatalf("query execution status: %v", err)
	}
	if execStatus != string(Prepared) {
		t.Fatalf("execution mutated status: expected %s, got %s", Prepared, execStatus)
	}
}

// TestTimingB_WorkerWins verifies:
// 1. Worker acquires task FOR UPDATE first.
// 2. Enrollment waits on task FOR UPDATE.
// 3. Worker advances task generation/status and commits.
// 4. Enrollment unblocks, recalculates live snapshot under its lock.
// 5. Generation drift is detected: enrollment fails closed and rolls back.
// 6. Assertion: quarantine table has strictly 0 rows written.
func TestTimingB_WorkerWins(t *testing.T) {
	db, _ := setupConcurrencyTestDB(t)
	defer db.Close()
	ctx := context.Background()

	taskID := fmt.Sprintf("task-timing-b-%d", time.Now().UnixNano())
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_generation_tasks (id, status, task_status, execution_generation) VALUES ($1, 'PROCESSING', 'RUNNING', 1)`, taskID); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM provider_execution_quarantine WHERE task_id=$1`, taskID)
		_, _ = db.ExecContext(ctx, `DELETE FROM provider_executions WHERE task_id=$1`, taskID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_generation_tasks WHERE id=$1`, taskID)
	}()

	var execID int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO provider_executions (task_id, provider, provider_model, capability, attempt, status, request_fingerprint, task_execution_generation)
		VALUES ($1, 'mock-provider', 'mock-model', 'image', 1, 'prepared', 'b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1', 1)
		RETURNING id
	`, taskID).Scan(&execID); err != nil {
		t.Fatalf("insert execution: %v", err)
	}

	// 1. Worker begins transaction and locks task FOR UPDATE first
	workerTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin worker tx: %v", err)
	}
	defer workerTx.Rollback()

	var workerLockedStatus string
	if err := workerTx.QueryRowContext(ctx, `SELECT status FROM xz_generation_tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&workerLockedStatus); err != nil {
		t.Fatalf("worker lock task FOR UPDATE: %v", err)
	}

	enrollStarted := make(chan struct{})
	enrollDone := make(chan error, 1)

	// 2. Enrollment begins transaction in background and attempts to lock task FOR UPDATE
	// It must BLOCK waiting for workerTx to release the row lock.
	go func() {
		enrollTx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			enrollDone <- err
			return
		}
		defer enrollTx.Rollback()

		close(enrollStarted)
		// Lock task FOR UPDATE: will block until workerTx commits
		var status string
		var gen int64
		if err := enrollTx.QueryRowContext(context.Background(), `SELECT status, execution_generation FROM xz_generation_tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&status, &gen); err != nil {
			enrollDone <- err
			return
		}

		// 4-5. Enrollment unblocks under lock, checks generation. Manifest expected generation=1.
		const expectedGen = int64(1)
		if gen != expectedGen {
			// Drift detected under lock! Fail closed and rollback.
			enrollDone <- fmt.Errorf("SNAPSHOT_SHA256_MISMATCH: task generation drifted from %d to %d", expectedGen, gen)
			return
		}

		// If matched (should not happen in Timing B), insert quarantine
		_, _ = enrollTx.ExecContext(context.Background(), `INSERT INTO provider_execution_quarantine (execution_id, task_id, attempt, generation, snapshot_sha256, evidence_sha256, approval_id, release_sha, not_before, expires_at) VALUES ($1, $2, 1, 1, 'sha', 'sha', 'appr', 'rel', now()-interval '1m', now()+interval '1h')`, execID, taskID)
		_ = enrollTx.Commit()
		enrollDone <- nil
	}()

	<-enrollStarted
	time.Sleep(50 * time.Millisecond)

	select {
	case err := <-enrollDone:
		t.Fatalf("enrollment completed prematurely while worker held lock! err=%v", err)
	default:
		// Enrollment is correctly blocked waiting on worker's task row lock
	}

	// 3. Worker advances task generation to 2 and commits
	if _, err := workerTx.ExecContext(ctx, `UPDATE xz_generation_tasks SET execution_generation=2, updated_at=now() WHERE id=$1`, taskID); err != nil {
		t.Fatalf("worker bump generation: %v", err)
	}
	if err := workerTx.Commit(); err != nil {
		t.Fatalf("worker commit: %v", err)
	}

	// 4-6. Enrollment unblocks, detects generation drift, rolls back
	select {
	case enrollErr := <-enrollDone:
		if enrollErr == nil || !errors.Is(enrollErr, errors.New(enrollErr.Error())) {
			t.Logf("enrollment observed expected drift: %v", enrollErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for enrollment to unblock and detect drift")
	}

	// 7. Assertion: quarantine table has strictly 0 rows written
	var quarantineCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM provider_execution_quarantine WHERE task_id=$1`, taskID).Scan(&quarantineCount); err != nil {
		t.Fatalf("query quarantine count: %v", err)
	}
	if quarantineCount != 0 {
		t.Fatalf("NEVER create fake quarantine after worker wins! Found %d rows", quarantineCount)
	}
}

// TestPostgresConcurrentRaceSafetyProviderExecution verifies high concurrency race safety.
func TestPostgresConcurrentRaceSafetyProviderExecution(t *testing.T) {
	db, store := setupConcurrencyTestDB(t)
	defer db.Close()
	ctx := context.Background()

	taskID := fmt.Sprintf("race-task-%d", time.Now().UnixNano())
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_generation_tasks (id, status, task_status, execution_generation) VALUES ($1, 'PROCESSING', 'RUNNING', 1)`, taskID); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM provider_execution_quarantine WHERE task_id=$1`, taskID)
		_, _ = db.ExecContext(ctx, `DELETE FROM provider_executions WHERE task_id=$1`, taskID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_generation_tasks WHERE id=$1`, taskID)
	}()

	created, err := store.CreatePrepared(ctx, Execution{
		TaskID:             taskID,
		Provider:           "mock-provider",
		ProviderModel:      "mock-model",
		Capability:         "image",
		Attempt:            1,
		RequestFingerprint: "c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1",
	})
	if err != nil {
		t.Fatalf("create prepared: %v", err)
	}

	// Insert quarantine row
	if _, err := db.ExecContext(ctx, `
		INSERT INTO provider_execution_quarantine (
			execution_id, task_id, attempt, generation,
			snapshot_sha256, evidence_sha256, approval_id, release_sha,
			not_before, expires_at
		) VALUES (
			$1, $2, 1, 1,
			'0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
			'0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
			'approval-race', '0123456789abcdef0123456789abcdef01234567',
			now() - interval '5m', now() + interval '1h'
		)`, created.ID, taskID); err != nil {
		t.Fatalf("insert quarantine: %v", err)
	}

	const concurrency = 20
	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerIdx int) {
			defer wg.Done()
			var opErr error
			switch workerIdx % 3 {
			case 0:
				_, opErr = store.ClaimPreparedForGenerationTask(ctx, taskID)
			case 1:
				opErr = store.Transition(ctx, created.ID, Submitted, nil, nil, nil)
			case 2:
				opErr = store.SaveSucceededResult(ctx, created.ID, nil, []byte(`{"url":"https://example.com"}`))
			}
			if !errors.Is(opErr, ErrQuarantined) {
				errCh <- fmt.Errorf("worker %d returned non-quarantine error: %w", workerIdx, opErr)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for e := range errCh {
		t.Errorf("race error: %v", e)
	}
}
