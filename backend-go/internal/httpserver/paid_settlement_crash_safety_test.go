package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

var paidTestSequence atomic.Int64

func setupPaidTestUser(t *testing.T, db *sql.DB, prefix string, initialPoints int64) (string, string) {
	t.Helper()
	ctx := context.Background()
	seq := paidTestSequence.Add(1)
	userID := fmt.Sprintf("paid_user_%s_%d_%d", prefix, time.Now().UTC().UnixNano(), seq)
	accountID := "acc-" + userID

	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users (id, name, role) VALUES ($1, 'Paid Test User', 'MEMBER') ON CONFLICT (id) DO NOTHING`, userID); err != nil {
		t.Fatalf("setupPaidTestUser: insert user: %v", err)
	}

	pointStore := NewPostgresPersonalPointStore(db)
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{
		AccountID:      accountID,
		UserID:         userID,
		Source:         PointSourceRecharge,
		Points:         initialPoints,
		IdempotencyKey: "grant-" + userID,
	}); err != nil {
		t.Fatalf("setupPaidTestUser: grant initial points: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM xz_personal_point_lot_movements WHERE account_id=$1`, accountID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM xz_personal_point_reservation_allocations WHERE account_id=$1`, accountID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM xz_personal_point_reservations WHERE account_id=$1`, accountID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM xz_personal_point_lots WHERE account_id=$1`, accountID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM xz_point_accounts WHERE id=$1`, accountID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM xz_user_wallets WHERE user_id=$1`, userID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM xz_users WHERE id=$1`, userID)
	})

	return userID, accountID
}

func createPaidPendingTask(t *testing.T, store *postgresStore, userID, prefix string) generationTask {
	t.Helper()
	seq := paidTestSequence.Add(1)
	req := createGenerationTaskRequest{
		ClientRequestID: fmt.Sprintf("paid_req_%s_%d_%d", prefix, time.Now().UTC().UnixNano(), seq),
		UserID:          userID,
		Type:            "TEXT_TO_VIDEO",
		ModuleCode:      moduleVideoGeneration,
		Prompt:          "paid settlement crash safety test video",
		Model:           "seedance-fast-2.0",
		Params: map[string]any{
			"duration":   5,
			"resolution": "720p",
		},
	}

	task, err := store.CreatePendingGenerationTask(req)
	if err != nil {
		t.Fatalf("createPaidPendingTask: %v", err)
	}

	if task.BillingEngine != personalLotBillingEngine {
		t.Fatalf("task billing engine = %q, want %q", task.BillingEngine, personalLotBillingEngine)
	}
	if task.PersonalPointAccountID == "" || task.PersonalPointReservationID == "" {
		t.Fatalf("task missing personal point markers: account=%q reservation=%q", task.PersonalPointAccountID, task.PersonalPointReservationID)
	}
	if task.BillingStatus != billingStatusReserved {
		t.Fatalf("task billing status = %q, want RESERVED", task.BillingStatus)
	}
	if task.PointCost <= 0 || task.ReservedPoints <= 0 {
		t.Fatalf("task point cost/reserved = %d/%.0f, want > 0", task.PointCost, task.ReservedPoints)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = store.db.ExecContext(cleanupCtx, `DELETE FROM xz_generation_tasks WHERE id=$1`, task.ID)
		_, _ = store.db.ExecContext(cleanupCtx, `DELETE FROM provider_executions WHERE task_id=$1`, task.ID)
		_, _ = store.db.ExecContext(cleanupCtx, `DELETE FROM xz_assets WHERE task_id=$1`, task.ID)
		_, _ = store.db.ExecContext(cleanupCtx, `DELETE FROM xz_billing_lifecycle_events WHERE task_id=$1`, task.ID)
		_, _ = store.db.ExecContext(cleanupCtx, `DELETE FROM outbox_events WHERE aggregate_id=$1`, task.ID)
	})

	return task
}

func getPointAccountBalances(t *testing.T, db *sql.DB, accountID, userID string) (available int64, frozen int64) {
	t.Helper()
	err := db.QueryRowContext(context.Background(), `
		SELECT available, frozen FROM xz_point_accounts WHERE id=$1 AND user_id=$2
	`, accountID, userID).Scan(&available, &frozen)
	if err != nil {
		t.Fatalf("getPointAccountBalances: %v", err)
	}
	return available, frozen
}

func getReservationState(t *testing.T, db *sql.DB, reservationID string) (status string, reserved int64, captured int64, released int64) {
	t.Helper()
	err := db.QueryRowContext(context.Background(), `
		SELECT status, reserved_points, captured_points, released_points
		FROM xz_personal_point_reservations WHERE id=$1
	`, reservationID).Scan(&status, &reserved, &captured, &released)
	if err != nil {
		t.Fatalf("getReservationState: %v", err)
	}
	return status, reserved, captured, released
}

func countLedgerMovements(t *testing.T, db *sql.DB, accountID, reservationID, movementType string) int {
	t.Helper()
	var count int
	query := `SELECT count(*) FROM xz_personal_point_lot_movements WHERE account_id=$1 AND movement_type=$2`
	args := []any{accountID, movementType}
	if reservationID != "" {
		query += ` AND reservation_id=$3`
		args = append(args, reservationID)
	}
	if err := db.QueryRowContext(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("countLedgerMovements: %v", err)
	}
	return count
}

// Case 1: Provider success -> Worker crash before Capture -> Retry/recovery executes -> Exactly 1 Capture occurs, zero duplicate Capture.
func TestPaidSettlement_Case1_ProviderSuccessWorkerCrashBeforeCapture(t *testing.T) {
	db := openFencingTestDB(t)
	store := newPostgresPrimaryStore(db, "")
	ctx := context.Background()

	const initialPoints = int64(1000)
	userID, accountID := setupPaidTestUser(t, db, "case1", initialPoints)
	task := createPaidPendingTask(t, store, userID, "case1")
	cost := int64(task.PointCost)

	// Precondition: points are frozen in account, reservation is active
	availAfterReserve, frozenAfterReserve := getPointAccountBalances(t, db, accountID, userID)
	if availAfterReserve != initialPoints-cost || frozenAfterReserve != cost {
		t.Fatalf("balance after reserve = available %d frozen %d, want %d/%d", availAfterReserve, frozenAfterReserve, initialPoints-cost, cost)
	}
	resStatus, resReserved, resCaptured, resReleased := getReservationState(t, db, task.PersonalPointReservationID)
	if resStatus != "RESERVED" || resReserved != cost || resCaptured != 0 || resReleased != 0 {
		t.Fatalf("reservation state after reserve = %s reserved=%d captured=%d released=%d", resStatus, resReserved, resCaptured, resReleased)
	}

	// 1. Worker A claims generation ownership
	genA, workerA, err := claimGenerationTaskOwnership(store, task.ID)
	if err != nil {
		t.Fatalf("claim A: %v", err)
	}

	// 2. Worker A provider execution completes with durable success
	execStore := pe.NewStore(db)
	createdA, err := execStore.CreatePreparedForGenerationTask(ctx, pe.Execution{
		TaskID:             task.ID,
		Provider:           "seedance-mock",
		Capability:         "video",
		RequestFingerprint: strings.Repeat("1", 64),
	})
	if err != nil {
		t.Fatalf("create prepared A: %v", err)
	}
	if createdA.TaskGeneration == nil || *createdA.TaskGeneration != genA {
		t.Fatalf("created A generation = %v, want %d", createdA.TaskGeneration, genA)
	}
	claimedA, err := execStore.ClaimPreparedForGenerationTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("claim prepared A: %v", err)
	}
	if err := execStore.SaveSucceededResult(ctx, claimedA.ID, fencingStrPtr("req-case1-A"), []byte(`[{"url":"https://example.test/video1.mp4"}]`)); err != nil {
		t.Fatalf("save succeeded result A: %v", err)
	}

	// 3. FAULT INJECTION: Worker A "crashes" before Capture!
	// No capture has occurred; points remain frozen.
	if count := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "CAPTURE"); count != 0 {
		t.Fatalf("capture count before recovery = %d, want 0", count)
	}

	// 4. Watchdog/reaper detects lease expiry, requeues task, worker B claims and recovers
	expireFencingLease(t, db, task.ID)
	genRequeued := requeueAsReaper(t, db, task.ID, genA)
	if genRequeued != genA+1 {
		t.Fatalf("requeued gen = %d, want %d", genRequeued, genA+1)
	}
	genB, workerB, err := claimGenerationTaskOwnership(store, task.ID)
	if err != nil {
		t.Fatalf("claim B: %v", err)
	}
	if workerB == workerA {
		t.Fatal("worker B reused worker A ID")
	}

	// Worker B (or recovery worker) executes Complete under generation B
	reqB := createGenerationTaskRequest{
		UserID: userID,
		Type:   task.Type,
		Model:  task.Model,
		Params: task.Params,
	}
	completedTask, err := store.CompleteGenerationTaskFenced(task.ID, reqB, genB)
	if err != nil {
		t.Fatalf("Worker B Complete: %v", err)
	}
	if completedTask.Status != "SUCCEEDED" || completedTask.BillingStatus != billingStatusCaptured {
		t.Fatalf("completed task status = %s billingStatus = %s, want SUCCEEDED/CAPTURED", completedTask.Status, completedTask.BillingStatus)
	}

	// 5. Crashed Worker A attempts late completion with stale generation A -> strictly fenced!
	reqA := createGenerationTaskRequest{
		UserID: userID,
		Type:   task.Type,
		Model:  task.Model,
		Params: task.Params,
	}
	_, errStale := store.CompleteGenerationTaskFenced(task.ID, reqA, genA)
	requireFenced(t, errStale, "stale Worker A Complete")

	// Crashed Worker A attempts heartbeat renewal -> strictly fenced!
	renewErr := renewGenerationLease(store, task.ID, workerA, genA)
	requireFenced(t, renewErr, "stale Worker A heartbeat")

	// Crashed Worker A attempts late fail -> strictly fenced!
	_, failErr := store.FailGenerationTaskFenced(task.ID, "stale fail", genA)
	requireFenced(t, failErr, "stale Worker A Fail")

	// 6. End-state invariants
	// Exactly 1 Capture in ledger; zero duplicate capture
	captures := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "CAPTURE")
	if captures != 1 {
		t.Fatalf("ledger CAPTURE movements = %d, want exactly 1", captures)
	}
	releases := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "RELEASE")
	if releases != 0 {
		t.Fatalf("ledger RELEASE movements = %d, want 0", releases)
	}

	// Reservation must be CAPTURED with exact points
	resStatus, resReserved, resCaptured, resReleased = getReservationState(t, db, task.PersonalPointReservationID)
	if resStatus != "CAPTURED" || resReserved != 0 || resCaptured != cost || resReleased != 0 {
		t.Fatalf("final reservation = %s reserved=%d captured=%d released=%d, want CAPTURED/0/%d/0", resStatus, resReserved, resCaptured, resReleased, cost)
	}

	// Frozen points MUST be 0 after definitive terminal settlement
	finalAvail, finalFrozen := getPointAccountBalances(t, db, accountID, userID)
	if finalAvail != initialPoints-cost || finalFrozen != 0 {
		t.Fatalf("final balance = available %d frozen %d, want %d/0", finalAvail, finalFrozen, initialPoints-cost)
	}
}

// Case 2: Capture succeeds -> Worker crash before task complete commits -> Redelivery/recovery executes -> No duplicate Capture, no wrong Release, points balance and frozen points consistent.
func TestPaidSettlement_Case2_CaptureSucceedsCrashBeforeTaskCompleteCommit(t *testing.T) {
	db := openFencingTestDB(t)
	store := newPostgresPrimaryStore(db, "")
	ctx := context.Background()

	const initialPoints = int64(1000)
	userID, accountID := setupPaidTestUser(t, db, "case2", initialPoints)
	task := createPaidPendingTask(t, store, userID, "case2")
	cost := int64(task.PointCost)

	// 1. Worker A claims generation ownership
	genA, _, err := claimGenerationTaskOwnership(store, task.ID)
	if err != nil {
		t.Fatalf("claim A: %v", err)
	}

	// 2. FAULT INJECTION:
	// Capture succeeds in the points store (committed in PostgreSQL), but worker crashes
	// BEFORE CompleteGenerationTaskFenced updates the task status and commits!
	pointStore := NewPostgresPersonalPointStore(db)
	capResult, err := pointStore.capture(ctx, PersonalPointCaptureCommand{
		AccountID:      task.PersonalPointAccountID,
		UserID:         userID,
		ReservationID:  task.PersonalPointReservationID,
		Points:         cost,
		IdempotencyKey: "generation:capture:" + task.ID,
	})
	if err != nil {
		t.Fatalf("fault injection: pointStore.capture: %v", err)
	}
	if capResult.Reservation.Status != "CAPTURED" {
		t.Fatalf("capture result status = %s, want CAPTURED", capResult.Reservation.Status)
	}

	// Verify state right after crash:
	// Points reservation is CAPTURED, account frozen is 0, but task is still non-terminal in xz_generation_tasks
	midAvail, midFrozen := getPointAccountBalances(t, db, accountID, userID)
	if midAvail != initialPoints-cost || midFrozen != 0 {
		t.Fatalf("balance right after capture crash = available %d frozen %d, want %d/0", midAvail, midFrozen, initialPoints-cost)
	}
	if count := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "CAPTURE"); count != 1 {
		t.Fatalf("capture movements right after crash = %d, want 1", count)
	}

	// 3. Verify NO WRONG RELEASE:
	// If failure recovery or error path is accidentally triggered, it MUST NOT release the already-captured reservation!
	_, failErr := store.FailGenerationTaskFenced(task.ID, "erroneous fail after capture", genA)
	if failErr == nil {
		t.Fatal("FailGenerationTaskFenced on already-captured reservation should fail, got nil")
	}
	if !errors.Is(failErr, ErrPersonalPointImportConflict) {
		t.Fatalf("FailGenerationTaskFenced error = %v, want ErrPersonalPointImportConflict", failErr)
	}
	if count := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "RELEASE"); count != 0 {
		t.Fatalf("RELEASE movement count after failed release = %d, want 0 (NO WRONG RELEASE)", count)
	}

	// 4. Redelivery / recovery executes: CompleteGenerationTaskFenced
	// Idempotent recovery path: recognizes the already-captured reservation, captures idempotently,
	// writes assets, and moves task to SUCCEEDED.
	req := createGenerationTaskRequest{
		UserID: userID,
		Type:   task.Type,
		Model:  task.Model,
		Params: task.Params,
	}
	completedTask, err := store.CompleteGenerationTaskFenced(task.ID, req, genA)
	if err != nil {
		t.Fatalf("redelivery Complete: %v", err)
	}
	if completedTask.Status != "SUCCEEDED" || completedTask.BillingStatus != billingStatusCaptured {
		t.Fatalf("completed task status = %s billingStatus = %s, want SUCCEEDED/CAPTURED", completedTask.Status, completedTask.BillingStatus)
	}

	// 5. Second idempotent replay of CompleteGenerationTaskFenced must also succeed with zero changes
	replayedTask, err := store.CompleteGenerationTaskFenced(task.ID, req, genA)
	if err != nil {
		t.Fatalf("replayed Complete: %v", err)
	}
	if replayedTask.Status != "SUCCEEDED" {
		t.Fatalf("replayed task status = %s, want SUCCEEDED", replayedTask.Status)
	}

	// 6. End-state invariants:
	// EXACTLY 1 capture movement in ledger (ZERO duplicate capture)
	captures := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "CAPTURE")
	if captures != 1 {
		t.Fatalf("final CAPTURE movements = %d, want exactly 1 (NO duplicate capture)", captures)
	}
	// ZERO release movement in ledger (ZERO wrong release)
	releases := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "RELEASE")
	if releases != 0 {
		t.Fatalf("final RELEASE movements = %d, want 0 (NO wrong release)", releases)
	}
	// Points balance and frozen points consistent
	finalAvail, finalFrozen := getPointAccountBalances(t, db, accountID, userID)
	if finalAvail != initialPoints-cost || finalFrozen != 0 {
		t.Fatalf("final balance = available %d frozen %d, want %d/0", finalAvail, finalFrozen, initialPoints-cost)
	}
	// Reservation state consistent
	resStatus, resReserved, resCaptured, resReleased := getReservationState(t, db, task.PersonalPointReservationID)
	if resStatus != "CAPTURED" || resReserved != 0 || resCaptured != cost || resReleased != 0 {
		t.Fatalf("final reservation state = %s reserved=%d captured=%d released=%d", resStatus, resReserved, resCaptured, resReleased)
	}
}

// Case 3: Concurrent old worker (generation G) and new worker (generation G+1) completion:
// old worker fenced with ErrFencedStaleExecution, only generation G+1 captures points and writes task/artifact.
// Ledger records exactly 1 authoritative capture.
func TestPaidSettlement_Case3_ConcurrentOldAndNewWorkerCompletion(t *testing.T) {
	db := openFencingTestDB(t)
	store := newPostgresPrimaryStore(db, "")

	const initialPoints = int64(1000)
	userID, accountID := setupPaidTestUser(t, db, "case3", initialPoints)
	task := createPaidPendingTask(t, store, userID, "case3")
	cost := int64(task.PointCost)

	// 1. Worker A claims generation G
	genG, workerA, err := claimGenerationTaskOwnership(store, task.ID)
	if err != nil {
		t.Fatalf("claim A: %v", err)
	}

	// 2. Lease expires, reaper requeues, Worker B claims generation G+1
	expireFencingLease(t, db, task.ID)
	_ = requeueAsReaper(t, db, task.ID, genG)
	genNext, workerB, err := claimGenerationTaskOwnership(store, task.ID)
	if err != nil {
		t.Fatalf("claim B: %v", err)
	}
	if genNext <= genG {
		t.Fatalf("genNext = %d, want > genG (%d)", genNext, genG)
	}
	if workerB == workerA {
		t.Fatal("worker B reused worker A ID")
	}

	// 3. Concurrent completion race between old worker (genG) and new worker (genNext)
	reqOld := createGenerationTaskRequest{
		UserID: userID,
		Type:   task.Type,
		Prompt: "old worker generation result",
		Model:  task.Model,
		Params: task.Params,
	}
	reqNew := createGenerationTaskRequest{
		UserID: userID,
		Type:   task.Type,
		Prompt: "new worker generation result",
		Model:  task.Model,
		Params: task.Params,
	}

	var wg sync.WaitGroup
	wg.Add(2)
	var errOld, errNew error
	var taskOld, taskNew generationTask

	go func() {
		defer wg.Done()
		taskOld, errOld = store.CompleteGenerationTaskFenced(task.ID, reqOld, genG)
	}()
	go func() {
		defer wg.Done()
		taskNew, errNew = store.CompleteGenerationTaskFenced(task.ID, reqNew, genNext)
	}()
	wg.Wait()

	// 4. Assert race outcome:
	// Old worker must be strictly fenced with ErrFencedStaleExecution
	if errOld == nil {
		t.Fatal("old worker Complete should have failed, got nil")
	}
	if !errors.Is(errOld, ErrFencedStaleExecution) {
		t.Fatalf("old worker Complete error = %v, want ErrFencedStaleExecution", errOld)
	}

	// New worker must succeed
	if errNew != nil {
		t.Fatalf("new worker Complete error = %v, want nil", errNew)
	}
	if taskNew.Status != "SUCCEEDED" || taskNew.BillingStatus != billingStatusCaptured {
		t.Fatalf("new worker task status = %s billingStatus = %s, want SUCCEEDED/CAPTURED", taskNew.Status, taskNew.BillingStatus)
	}
	_ = taskOld

	// 5. Late retry by old worker after new worker succeeded must still be rejected
	_, retryErr := store.CompleteGenerationTaskFenced(task.ID, reqOld, genG)
	requireFenced(t, retryErr, "late retry by old worker")

	// 6. End-state invariants:
	// Ledger records EXACTLY 1 authoritative capture
	captures := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "CAPTURE")
	if captures != 1 {
		t.Fatalf("ledger CAPTURE movements = %d, want exactly 1", captures)
	}
	releases := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "RELEASE")
	if releases != 0 {
		t.Fatalf("ledger RELEASE movements = %d, want 0", releases)
	}

	// Frozen points = 0
	finalAvail, finalFrozen := getPointAccountBalances(t, db, accountID, userID)
	if finalAvail != initialPoints-cost || finalFrozen != 0 {
		t.Fatalf("final balance = available %d frozen %d, want %d/0", finalAvail, finalFrozen, initialPoints-cost)
	}

	// Asset was written by the winning worker (genNext), not the stale worker
	var assetPrompt string
	var assetCount int
	if err := db.QueryRowContext(context.Background(), `SELECT count(*), coalesce(min(metadata->>'prompt'),'') FROM xz_assets WHERE task_id=$1`, task.ID).Scan(&assetCount, &assetPrompt); err != nil {
		t.Fatal(err)
	}
	if assetCount != 1 {
		t.Fatalf("asset count = %d, want exactly 1 (only winner writes artifact)", assetCount)
	}
	if assetPrompt != reqNew.Prompt {
		t.Fatalf("asset prompt = %q, want %q (winner prompt)", assetPrompt, reqNew.Prompt)
	}
}

// Case 4A: Timeout vs late provider success:
// Timeout definitive state cleanly releases frozen points; late provider success on stale generation is rejected,
// preventing simultaneous Release + Capture.
func TestPaidSettlement_Case4A_TimeoutVsLateProviderSuccess(t *testing.T) {
	db := openFencingTestDB(t)
	store := newPostgresPrimaryStore(db, "")
	ctx := context.Background()

	const initialPoints = int64(1000)
	userID, accountID := setupPaidTestUser(t, db, "case4a", initialPoints)
	task := createPaidPendingTask(t, store, userID, "case4a")
	_ = task.PointCost

	// 1. Worker A claims generation G
	genA, workerA, err := claimGenerationTaskOwnership(store, task.ID)
	if err != nil {
		t.Fatalf("claim A: %v", err)
	}

	// Worker A creates provider execution bound to generation A
	execStore := pe.NewStore(db)
	_, err = execStore.CreatePreparedForGenerationTask(ctx, pe.Execution{
		TaskID:             task.ID,
		Provider:           "seedance-mock",
		Capability:         "video",
		RequestFingerprint: strings.Repeat("4", 64),
	})
	if err != nil {
		t.Fatalf("create prepared A: %v", err)
	}
	claimedA, err := execStore.ClaimPreparedForGenerationTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("claim prepared A: %v", err)
	}

	// 2. TIMEOUT OCCURS:
	// Worker A takes too long. Watchdog/reaper notices lease expiry.
	// Reaper requeues, bumping generation to G+1.
	expireFencingLease(t, db, task.ID)
	genTimeout := requeueAsReaper(t, db, task.ID, genA)
	if genTimeout != genA+1 {
		t.Fatalf("requeue generation = %d, want %d", genTimeout, genA+1)
	}

	// Transition provider execution to failed/timeout so durable failure can release
	if err := execStore.Transition(ctx, claimedA.ID, pe.Failed, nil, fencingStrPtr(string(pe.RetryableBeforeSubmit)), fencingStrPtr("timeout")); err != nil {
		t.Fatalf("transition execution: %v", err)
	}

	// Definitive timeout failure settlement on generation G+1
	failedTask, err := store.FailGenerationTaskDurableFenced(task.ID, "context deadline exceeded", genTimeout)
	if err != nil {
		t.Fatalf("timeout FailGenerationTaskDurableFenced: %v", err)
	}
	if failedTask.Status != "FAILED" || failedTask.BillingStatus != billingStatusReleased {
		t.Fatalf("failed task status = %s billingStatus = %s, want FAILED/RELEASED", failedTask.Status, failedTask.BillingStatus)
	}

	// Frozen points are cleanly released
	timeoutAvail, timeoutFrozen := getPointAccountBalances(t, db, accountID, userID)
	if timeoutAvail != initialPoints || timeoutFrozen != 0 {
		t.Fatalf("balance after timeout release = available %d frozen %d, want %d/0", timeoutAvail, timeoutFrozen, initialPoints)
	}
	if relCount := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "RELEASE"); relCount != 1 {
		t.Fatalf("RELEASE movements after timeout = %d, want 1", relCount)
	}

	// 3. LATE PROVIDER SUCCESS ARRIVES on stale Worker A (generation genA):
	// Worker A attempts to call CompleteGenerationTaskFenced with stale generation genA
	reqA := createGenerationTaskRequest{
		UserID: userID,
		Type:   task.Type,
		Model:  task.Model,
		Params: task.Params,
	}
	_, lateCompleteErr := store.CompleteGenerationTaskFenced(task.ID, reqA, genA)
	if lateCompleteErr == nil {
		t.Fatal("late provider success on stale generation should fail, got nil")
	}
	if !errors.Is(lateCompleteErr, ErrFencedStaleExecution) {
		t.Fatalf("late complete error = %v, want ErrFencedStaleExecution", lateCompleteErr)
	}

	// Late heartbeat renewal also rejected
	renewErr := renewGenerationLease(store, task.ID, workerA, genA)
	requireFenced(t, renewErr, "stale heartbeat after timeout")

	// 4. Succeeded execution gate also rejects stale generation late success
	apiInst := api{store: store}
	lateExec := pe.Execution{
		TaskGeneration: &genA,
	}
	if gateErr := apiInst.checkSucceededExecutionGeneration(task.ID, lateExec); !errors.Is(gateErr, ErrFencedStaleExecution) {
		t.Fatalf("recovery gate error = %v, want ErrFencedStaleExecution", gateErr)
	}

	// 5. End-state invariants:
	// Exactly 1 RELEASE in ledger
	releases := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "RELEASE")
	if releases != 1 {
		t.Fatalf("final RELEASE movements = %d, want exactly 1", releases)
	}
	// EXACTLY 0 CAPTURE in ledger (preventing simultaneous Release + Capture!)
	captures := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "CAPTURE")
	if captures != 0 {
		t.Fatalf("final CAPTURE movements = %d, want 0 (SIMULTANEOUS RELEASE + CAPTURE OCCURRED!)", captures)
	}

	// Frozen points MUST be 0
	finalAvail, finalFrozen := getPointAccountBalances(t, db, accountID, userID)
	if finalAvail != initialPoints || finalFrozen != 0 {
		t.Fatalf("final balance = available %d frozen %d, want %d/0", finalAvail, finalFrozen, initialPoints)
	}
}

// Case 4B: Cancel vs late provider success:
// Cancel definitive state cleanly releases frozen points; late provider success on stale generation is rejected,
// preventing simultaneous Release + Capture.
func TestPaidSettlement_Case4B_CancelVsLateProviderSuccess(t *testing.T) {
	db := openFencingTestDB(t)
	store := newPostgresPrimaryStore(db, "")

	const initialPoints = int64(1000)
	userID, accountID := setupPaidTestUser(t, db, "case4b", initialPoints)
	task := createPaidPendingTask(t, store, userID, "case4b")
	_ = task.PointCost

	// 1. Worker A claims generation G
	genA, workerA, err := claimGenerationTaskOwnership(store, task.ID)
	if err != nil {
		t.Fatalf("claim A: %v", err)
	}

	// 2. USER CANCELS THE TASK:
	// Cancel definitive state bumps generation to G+1 and cleanly releases frozen points
	cancelledTask, err := store.CancelGenerationTaskForUser(userID, task.ID)
	if err != nil {
		t.Fatalf("CancelGenerationTaskForUser: %v", err)
	}
	if cancelledTask.Status != "CANCELLED" || cancelledTask.BillingStatus != billingStatusReleased {
		t.Fatalf("cancelled task status = %s billingStatus = %s, want CANCELLED/RELEASED", cancelledTask.Status, cancelledTask.BillingStatus)
	}
	if cancelledTask.ExecutionGeneration <= genA {
		t.Fatalf("cancelled generation = %d, want > genA (%d)", cancelledTask.ExecutionGeneration, genA)
	}

	// Frozen points are cleanly released back to user
	cancelAvail, cancelFrozen := getPointAccountBalances(t, db, accountID, userID)
	if cancelAvail != initialPoints || cancelFrozen != 0 {
		t.Fatalf("balance after cancel = available %d frozen %d, want %d/0", cancelAvail, cancelFrozen, initialPoints)
	}
	if relCount := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "RELEASE"); relCount != 1 {
		t.Fatalf("RELEASE movements after cancel = %d, want 1", relCount)
	}

	// 3. LATE PROVIDER SUCCESS ARRIVES on Worker A (stale generation genA):
	// Worker A calls CompleteGenerationTaskFenced with stale generation genA
	reqA := createGenerationTaskRequest{
		UserID: userID,
		Type:   task.Type,
		Model:  task.Model,
		Params: task.Params,
	}
	_, lateCompleteErr := store.CompleteGenerationTaskFenced(task.ID, reqA, genA)
	if lateCompleteErr == nil {
		t.Fatal("late provider success on cancelled task should fail, got nil")
	}
	if !errors.Is(lateCompleteErr, ErrFencedStaleExecution) {
		t.Fatalf("late complete error = %v, want ErrFencedStaleExecution", lateCompleteErr)
	}

	// Late heartbeat renewal also rejected
	renewErr := renewGenerationLease(store, task.ID, workerA, genA)
	requireFenced(t, renewErr, "stale heartbeat after cancel")

	// Late failure settlement also rejected
	_, lateFailErr := store.FailGenerationTaskFenced(task.ID, "late fail after cancel", genA)
	requireFenced(t, lateFailErr, "stale fail after cancel")

	// 4. End-state invariants:
	// Exactly 1 RELEASE in ledger
	releases := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "RELEASE")
	if releases != 1 {
		t.Fatalf("final RELEASE movements = %d, want exactly 1", releases)
	}
	// EXACTLY 0 CAPTURE in ledger (preventing simultaneous Release + Capture!)
	captures := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "CAPTURE")
	if captures != 0 {
		t.Fatalf("final CAPTURE movements = %d, want 0 (SIMULTANEOUS RELEASE + CAPTURE OCCURRED!)", captures)
	}

	// Frozen points MUST be 0
	finalAvail, finalFrozen := getPointAccountBalances(t, db, accountID, userID)
	if finalAvail != initialPoints || finalFrozen != 0 {
		t.Fatalf("final balance = available %d frozen %d, want %d/0", finalAvail, finalFrozen, initialPoints)
	}

	// Stale worker cannot mutate artifacts
	var assetCount int
	if err := db.QueryRowContext(context.Background(), `SELECT count(*) FROM xz_assets WHERE task_id=$1`, task.ID).Scan(&assetCount); err != nil {
		t.Fatal(err)
	}
	if assetCount != 0 {
		t.Fatalf("asset count = %d, want 0 (stale worker wrote artifacts!)", assetCount)
	}
}

// End-state invariant test:
// frozen_points = 0 only after definitive terminal settlement; stale worker cannot mutate ledger or artifact.
func TestPaidSettlement_EndStateInvariant_FrozenPointsZeroOnlyAfterTerminal(t *testing.T) {
	db := openFencingTestDB(t)
	store := newPostgresPrimaryStore(db, "")

	const initialPoints = int64(1000)
	userID, accountID := setupPaidTestUser(t, db, "invariant", initialPoints)
	task := createPaidPendingTask(t, store, userID, "invariant")
	cost := int64(task.PointCost)

	// INVARIANT 1: While task is non-terminal, frozen_points > 0
	avail, frozen := getPointAccountBalances(t, db, accountID, userID)
	if frozen != cost || avail != initialPoints-cost {
		t.Fatalf("pre-settlement frozen=%d avail=%d, want %d/%d", frozen, avail, cost, initialPoints-cost)
	}

	// Worker 1 claims generation G
	gen1, worker1, err := claimGenerationTaskOwnership(store, task.ID)
	if err != nil {
		t.Fatalf("claim 1: %v", err)
	}

	// Task lease expires, reaper requeues, Worker 2 claims generation G+1
	expireFencingLease(t, db, task.ID)
	_ = requeueAsReaper(t, db, task.ID, gen1)
	gen2, worker2, err := claimGenerationTaskOwnership(store, task.ID)
	if err != nil {
		t.Fatalf("claim 2: %v", err)
	}
	_ = worker1
	_ = worker2

	// INVARIANT 2: While task is still non-terminal, stale worker (gen1) cannot settle or mutate
	reqStale := createGenerationTaskRequest{
		UserID: userID,
		Type:   task.Type,
		Model:  task.Model,
		Params: task.Params,
	}
	_, errStaleComplete := store.CompleteGenerationTaskFenced(task.ID, reqStale, gen1)
	requireFenced(t, errStaleComplete, "stale Complete before terminal")

	_, errStaleFail := store.FailGenerationTaskFenced(task.ID, "stale fail before terminal", gen1)
	requireFenced(t, errStaleFail, "stale Fail before terminal")

	_, errStaleDurable := store.FailGenerationTaskDurableFenced(task.ID, "stale durable fail", gen1)
	requireFenced(t, errStaleDurable, "stale DurableFail before terminal")

	// Verify frozen_points is STILL > 0 (stale worker could not release or settle!)
	availAfterStale, frozenAfterStale := getPointAccountBalances(t, db, accountID, userID)
	if frozenAfterStale != cost || availAfterStale != initialPoints-cost {
		t.Fatalf("frozen after stale attempts = %d avail=%d, want %d/%d", frozenAfterStale, availAfterStale, cost, initialPoints-cost)
	}
	if captures := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "CAPTURE"); captures != 0 {
		t.Fatalf("captures after stale attempts = %d, want 0", captures)
	}
	if releases := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "RELEASE"); releases != 0 {
		t.Fatalf("releases after stale attempts = %d, want 0", releases)
	}

	// INVARIANT 3: Only authoritative current generation terminal settlement transitions frozen_points to 0
	reqAuth := createGenerationTaskRequest{
		UserID: userID,
		Type:   task.Type,
		Model:  task.Model,
		Params: task.Params,
	}
	completed, err := store.CompleteGenerationTaskFenced(task.ID, reqAuth, gen2)
	if err != nil {
		t.Fatalf("authoritative complete: %v", err)
	}
	if completed.Status != "SUCCEEDED" || completed.BillingStatus != billingStatusCaptured {
		t.Fatalf("completed status=%s billing=%s, want SUCCEEDED/CAPTURED", completed.Status, completed.BillingStatus)
	}

	// Now frozen_points MUST be 0
	availFinal, frozenFinal := getPointAccountBalances(t, db, accountID, userID)
	if frozenFinal != 0 || availFinal != initialPoints-cost {
		t.Fatalf("final frozen=%d avail=%d, want 0/%d", frozenFinal, availFinal, initialPoints-cost)
	}

	// INVARIANT 4: Post-settlement stale operations are strictly rejected and cannot mutate anything
	_, postFailErr := store.FailGenerationTaskFenced(task.ID, "post-terminal stale fail", gen1)
	requireFenced(t, postFailErr, "post-terminal stale fail")

	_, postCompleteErr := store.CompleteGenerationTaskFenced(task.ID, reqStale, gen1)
	requireFenced(t, postCompleteErr, "post-terminal stale complete")

	// Final verification: exactly 1 CAPTURE, 0 RELEASE, frozen = 0
	if captures := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "CAPTURE"); captures != 1 {
		t.Fatalf("final captures = %d, want exactly 1", captures)
	}
	if releases := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "RELEASE"); releases != 0 {
		t.Fatalf("final releases = %d, want 0", releases)
	}
}

// Stale generations CANNOT perform Capture, Release, or Refund on points ledger
func TestPaidSettlement_StaleGenerationsCannotMutateLedgerOrArtifact(t *testing.T) {
	db := openFencingTestDB(t)
	store := newPostgresPrimaryStore(db, "")

	const initialPoints = int64(1000)
	userID, accountID := setupPaidTestUser(t, db, "mutateguard", initialPoints)
	task := createPaidPendingTask(t, store, userID, "mutateguard")

	// 1. Initial generation is 1. Bump to 2 via claim.
	gen1, worker1, err := claimGenerationTaskOwnership(store, task.ID)
	if err != nil {
		t.Fatalf("claim 1: %v", err)
	}

	// Bump to 3 via requeue
	expireFencingLease(t, db, task.ID)
	gen2 := requeueAsReaper(t, db, task.ID, gen1)

	// Bump to 4 via new claim
	gen3, worker2, err := claimGenerationTaskOwnership(store, task.ID)
	if err != nil {
		t.Fatalf("claim 2: %v", err)
	}
	_ = worker1
	_ = worker2
	_ = gen2

	// Any stale generation (< gen3) must fail every ledger-mutating operation
	staleGens := []int64{gen1, gen2}
	for _, stale := range staleGens {
		// 1. Stale Complete (attempted Capture)
		_, err := store.CompleteGenerationTaskFenced(task.ID, createGenerationTaskRequest{UserID: userID, Type: task.Type}, stale)
		requireFenced(t, err, fmt.Sprintf("stale gen %d Complete", stale))

		// 2. Stale Fail (attempted Release)
		_, err = store.FailGenerationTaskFenced(task.ID, "stale failure", stale)
		requireFenced(t, err, fmt.Sprintf("stale gen %d Fail", stale))

		// 3. Stale Durable Fail (attempted Release)
		_, err = store.FailGenerationTaskDurableFenced(task.ID, "stale durable", stale)
		requireFenced(t, err, fmt.Sprintf("stale gen %d DurableFail", stale))

		// 4. Stale Unknown Grace Fail (attempted Release)
		_, err = store.FailGenerationTaskUnknownGraceFenced(task.ID, "stale unknown grace", 5*time.Minute, stale)
		requireFenced(t, err, fmt.Sprintf("stale gen %d UnknownGraceFail", stale))
	}

	// Assert: zero ledger movements were created by any of the stale attempts
	captures := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "CAPTURE")
	releases := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "RELEASE")
	if captures != 0 || releases != 0 {
		t.Fatalf("stale attempts produced ledger movements: captures=%d releases=%d, want 0/0", captures, releases)
	}

	// Assert: zero assets were created by any of the stale attempts
	var assetCount int
	if err := db.QueryRowContext(context.Background(), `SELECT count(*) FROM xz_assets WHERE task_id=$1`, task.ID).Scan(&assetCount); err != nil {
		t.Fatal(err)
	}
	if assetCount != 0 {
		t.Fatalf("stale attempts produced assets: count=%d, want 0", assetCount)
	}

	// Clean authoritative release by current generation (gen3) succeeds cleanly
	cancelled, err := store.FailGenerationTaskFenced(task.ID, "authoritative failure", gen3)
	if err != nil {
		t.Fatalf("authoritative fail: %v", err)
	}
	if cancelled.Status != "FAILED" || cancelled.BillingStatus != billingStatusReleased {
		t.Fatalf("cancelled status=%s billing=%s, want FAILED/RELEASED", cancelled.Status, cancelled.BillingStatus)
	}

	// Assert: exactly 1 release occurred, frozen points = 0
	if count := countLedgerMovements(t, db, accountID, task.PersonalPointReservationID, "RELEASE"); count != 1 {
		t.Fatalf("releases after authoritative fail = %d, want 1", count)
	}
	finalAvail, finalFrozen := getPointAccountBalances(t, db, accountID, userID)
	if finalAvail != initialPoints || finalFrozen != 0 {
		t.Fatalf("final balance = %d/%d, want %d/0", finalAvail, finalFrozen, initialPoints)
	}
}
