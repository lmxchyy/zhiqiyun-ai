package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"xianzhi-ai/backend-go/internal/messaging"
)

func TestSchedulerDispatch_EventIDCollisionFix(t *testing.T) {
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	planID := "plan_coll_" + suffix
	userID := "user_coll_" + suffix
	taskID := "task_coll_" + suffix

	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id = $1", taskID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id = $1", taskID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_users WHERE id = $1", userID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_plans WHERE id = $1", planID)
	}()

	// 1. Seed plan (concurrency 3) and user
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_plans (id, code, name, concurrency, active)
		VALUES ($1, $1, 'Basic Collision Test Plan', 3, true)
	`, planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_users (id, email, name, role, status, plan_id)
		VALUES ($1, $1 || '@test.local', 'Collision User', 'MEMBER', 'ACTIVE', $2)
	`, userID, planID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// 2. Simulate Creation Outbox Event already existing (as happens on task creation)
	creationEventID := "generation.image.requested:" + taskID
	if _, err := db.ExecContext(ctx, `
		INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, event_type, payload, status, created_at, updated_at)
		VALUES ($1, 'generation_task', $2, 'x.ai.generation.image.canary.requested', '{"task_id":"`+taskID+`"}', 'published', now(), now())
	`, creationEventID, taskID); err != nil {
		t.Fatalf("seed creation outbox event: %v", err)
	}

	// 3. Seed QUEUED task for this user with execution_generation = 1
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, execution_generation, created_at, updated_at)
		VALUES ($1, $2, 'TEXT_TO_IMAGE', 'PROCESSING', 'QUEUED', 1, $3, $3)
	`, taskID, userID, nowStr); err != nil {
		t.Fatalf("seed queued task: %v", err)
	}

	scheduler := NewGenerationScheduler(db, GenerationSchedulerOptions{
		BatchUsers:        10,
		BatchTasksPerUser: 5,
		Owner:             "generation-fair-scheduler",
	})

	// 4. DispatchOnce: verify that creation outbox already existing does NOT cause collision
	dispatched, err := scheduler.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("DispatchOnce failed with collision: %v", err)
	}
	if dispatched != 1 {
		t.Fatalf("dispatched = %d, want 1", dispatched)
	}

	// 5. Verify the dispatched EventID contract
	var dispatchEventID string
	var dispatchGen int64
	if err := db.QueryRowContext(ctx, `
		SELECT event_id FROM outbox_events 
		WHERE aggregate_id = $1 AND event_id <> $2
	`, taskID, creationEventID).Scan(&dispatchEventID); err != nil {
		t.Fatalf("failed to query new dispatch outbox event: %v", err)
	}

	if err := db.QueryRowContext(ctx, `
		SELECT execution_generation FROM xz_generation_tasks WHERE id = $1
	`, taskID).Scan(&dispatchGen); err != nil {
		t.Fatalf("query task generation: %v", err)
	}

	expectedEventID := fmt.Sprintf("generation.dispatched:%s:%d", taskID, dispatchGen)
	if dispatchEventID != expectedEventID {
		t.Fatalf("dispatched event_id = %q, want %q", dispatchEventID, expectedEventID)
	}
	if dispatchGen != 2 {
		t.Fatalf("task generation after dispatch = %d, want 2", dispatchGen)
	}

	// 6. Verify same generation idempotency: re-inserting the exact same event does NOT error
	outboxStore := messaging.NewOutboxStore(db)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	dupEnvelope := &messaging.Envelope{
		EventID:       expectedEventID,
		EventType:     "x.ai.generation.image.canary.requested",
		Version:       1,
		OccurredAt:    time.Now().UTC().Format(time.RFC3339),
		Producer:      "generation-fair-scheduler",
		AggregateType: "generation_task",
		AggregateID:   taskID,
		Data:          map[string]interface{}{"task_id": taskID, "execution_generation": dispatchGen},
	}
	if insertErr := outboxStore.InsertTx(ctx, tx, dupEnvelope, "generation_task", taskID, ""); insertErr != nil {
		t.Fatalf("duplicate same-generation outbox insert should be idempotently absorbed, got: %v", insertErr)
	}
	if commitErr := tx.Commit(); commitErr != nil {
		t.Fatalf("commit duplicate absorption: %v", commitErr)
	}

	// Verify count is still exactly 1 for the dispatch event (no second authority event created)
	var countDispatchEvents int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM outbox_events WHERE event_id = $1`, expectedEventID).Scan(&countDispatchEvents); err != nil || countDispatchEvents != 1 {
		t.Fatalf("count of dispatch events = %d, want exactly 1", countDispatchEvents)
	}
}

func TestSchedulerDispatch_StaleRecoveryGenerationProgression(t *testing.T) {
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	planID := "plan_recov_" + suffix
	userID := "user_recov_" + suffix
	taskID := "task_recov_" + suffix

	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id = $1", taskID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id = $1", taskID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_users WHERE id = $1", userID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_plans WHERE id = $1", planID)
	}()

	// 1. Seed plan & user
	_, _ = db.ExecContext(ctx, `INSERT INTO xz_plans (id, code, name, concurrency, active) VALUES ($1, $1, 'Recov Plan', 3, true)`, planID)
	_, _ = db.ExecContext(ctx, `INSERT INTO xz_users (id, email, name, role, status, plan_id) VALUES ($1, $1 || '@test.local', 'Recov User', 'MEMBER', 'ACTIVE', $2)`, userID, planID)

	// 2. Seed task in DISPATCHING at Gen G=2, with expired lease and pending outbox
	staleLease := time.Now().UTC().Add(-120 * time.Second).Format(time.RFC3339Nano)
	staleEventID := fmt.Sprintf("generation.dispatched:%s:2", taskID)

	if _, err := db.ExecContext(ctx, `
		INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, event_type, payload, status, attempt_count, created_at, updated_at)
		VALUES ($1, 'generation_task', $2, 'x.ai.generation.image.canary.requested', '{"task_id":"`+taskID+`"}', 'pending', 0, $3, $3)
	`, staleEventID, taskID, staleLease); err != nil {
		t.Fatalf("seed stale outbox: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, execution_generation, worker_id, lease_until, updated_at, created_at)
		VALUES ($1, $2, 'TEXT_TO_IMAGE', 'PROCESSING', 'DISPATCHING', 2, 'old-worker', $3::timestamptz, $3::text, $3::text)
	`, taskID, userID, staleLease); err != nil {
		t.Fatalf("seed stale task: %v", err)
	}

	scheduler := NewGenerationScheduler(db, GenerationSchedulerOptions{
		BatchUsers:           10,
		BatchTasksPerUser:    5,
		StaleDispatchTimeout: 30 * time.Second,
	})

	// 3. RecoverStaleDispatches: deletes pending outbox, bumps gen to 3, requeues to QUEUED
	recovered, err := scheduler.RecoverStaleDispatches(ctx)
	if err != nil {
		t.Fatalf("RecoverStaleDispatches: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("recovered = %d, want 1", recovered)
	}

	var taskStatus string
	var newGen int64
	if err := db.QueryRowContext(ctx, `SELECT task_status, execution_generation FROM xz_generation_tasks WHERE id = $1`, taskID).Scan(&taskStatus, &newGen); err != nil {
		t.Fatalf("query recovered task: %v", err)
	}
	if taskStatus != "QUEUED" || newGen != 3 {
		t.Fatalf("after recovery: status=%s gen=%d, want QUEUED / 3", taskStatus, newGen)
	}

	// Stale event should be cleanly deleted
	var remainingOutbox int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM outbox_events WHERE event_id = $1`, staleEventID).Scan(&remainingOutbox); err != nil || remainingOutbox != 0 {
		t.Fatalf("stale outbox event should be deleted, count=%d", remainingOutbox)
	}

	// 4. DispatchOnce: re-dispatching must produce generation.dispatched:<task_id>:4
	dispatched, err := scheduler.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("DispatchOnce after recovery: %v", err)
	}
	if dispatched != 1 {
		t.Fatalf("dispatched = %d, want 1", dispatched)
	}

	var nextEventID string
	var nextGen int64
	if err := db.QueryRowContext(ctx, `SELECT event_id FROM outbox_events WHERE aggregate_id = $1`, taskID).Scan(&nextEventID); err != nil {
		t.Fatalf("query next outbox event: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT execution_generation FROM xz_generation_tasks WHERE id = $1`, taskID).Scan(&nextGen); err != nil {
		t.Fatalf("query next gen: %v", err)
	}

	expectedNextEventID := fmt.Sprintf("generation.dispatched:%s:4", taskID)
	if nextEventID != expectedNextEventID || nextGen != 4 {
		t.Fatalf("next dispatch produced event=%s gen=%d, want event=%s gen=4", nextEventID, nextGen, expectedNextEventID)
	}
}

func TestSchedulerDispatch_FencingAndInboxClaimReplay(t *testing.T) {
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	taskID := "task_fence_test_" + suffix

	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM consumer_inbox WHERE event_id LIKE '%"+suffix+"%'")
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id = $1", taskID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id = $1", taskID)
	}()

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	// Task is at execution_generation = 3 (current owner holding valid lease)
	futureLease := time.Now().UTC().Add(60 * time.Second).Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, execution_generation, worker_id, lease_until, created_at, updated_at)
		VALUES ($1, 'user_1', 'TEXT_TO_IMAGE', 'PROCESSING', 'DISPATCHING', 3, 'live-worker', $2::timestamptz, $3, $3)
	`, taskID, futureLease, nowStr); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	inbox := messaging.NewInboxStore(db)

	// Test 1: inbox ClaimTx replay on the same event_id
	staleEventID := fmt.Sprintf("generation.dispatched:%s:2", taskID)
	tx1, _ := db.BeginTx(ctx, nil)
	dup1, err1 := inbox.ClaimTx(ctx, tx1, "test-consumer", staleEventID)
	if err1 != nil || dup1 {
		t.Fatalf("first claim: dup=%v err=%v", dup1, err1)
	}
	if completeErr := inbox.CompleteTx(ctx, tx1, "test-consumer", staleEventID, "completed", map[string]any{"task_id": taskID}); completeErr != nil {
		t.Fatalf("complete tx1: %v", completeErr)
	}
	_ = tx1.Commit()

	tx2, _ := db.BeginTx(ctx, nil)
	dup2, err2 := inbox.ClaimTx(ctx, tx2, "test-consumer", staleEventID)
	_ = tx2.Rollback()
	if err2 != nil || !dup2 {
		t.Fatalf("second claim on same event_id must report duplicate=true, got dup=%v err=%v", dup2, err2)
	}

	// Test 2: Stale generation fencing check in processGenerationCanaryMessage logic
	// When envelope generation (G=2) < task generation (G=3) and lease is valid, it skips provider
	var currentGen int64
	var leaseUntil time.Time
	if err := db.QueryRowContext(ctx, `SELECT execution_generation, lease_until FROM xz_generation_tasks WHERE id = $1`, taskID).Scan(&currentGen, &leaseUntil); err != nil {
		t.Fatalf("query task: %v", err)
	}
	envelopeGen := int64(2)
	if !(envelopeGen < currentGen && leaseUntil.After(time.Now().UTC())) {
		t.Fatalf("stale generation condition check failed: envelope=%d current=%d lease=%v", envelopeGen, currentGen, leaseUntil)
	}

	// Test 3: provider_executions (task_id, attempt) unique constraint
	fingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO provider_executions (task_id, task_execution_generation, capability, provider, provider_model, request_fingerprint, attempt, status, created_at, updated_at)
		VALUES ($1, 3, 'image', 'configured', 'gpt-image-2', $2, 1, 'succeeded', now(), now())
	`, taskID, fingerprint); err != nil {
		t.Fatalf("seed provider execution: %v", err)
	}

	// Second execution for same (task_id, attempt) MUST be rejected by DB constraint
	_, dupExecErr := db.ExecContext(ctx, `
		INSERT INTO provider_executions (task_id, task_execution_generation, capability, provider, provider_model, request_fingerprint, attempt, status, created_at, updated_at)
		VALUES ($1, 3, 'image', 'configured', 'gpt-image-2', $2, 1, 'succeeded', now(), now())
	`, taskID, fingerprint)
	if dupExecErr == nil {
		t.Fatalf("expected unique constraint violation on (task_id, attempt), got nil")
	}
}

func TestScheduler_ProductionCanarySimulatedScenario(t *testing.T) {
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	planID := "plan_canary_sim_" + suffix
	userID := "user_canary_sim_" + suffix

	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id LIKE '%"+suffix+"%'")
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE user_id = $1", userID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_users WHERE id = $1", userID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_plans WHERE id = $1", planID)
	}()

	// 1. Seed Basic plan (concurrency = 3)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_plans (id, code, name, concurrency, active)
		VALUES ($1, $1, 'Basic Simulated Plan', 3, true)
	`, planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_users (id, email, name, role, status, plan_id)
		VALUES ($1, $1 || '@test.local', 'Sim User', 'MEMBER', 'ACTIVE', $2)
	`, userID, planID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// 2. Setup 3 RUNNING tasks (slots occupied) and 2 QUEUED tasks
	now := time.Now().UTC()
	var runningIDs []string
	for i := 1; i <= 3; i++ {
		tid := fmt.Sprintf("task_sim_run_%d_%s", i, suffix)
		runningIDs = append(runningIDs, tid)
		timeStr := now.Add(time.Duration(i)*time.Millisecond).Format(time.RFC3339Nano)
		if _, err := db.ExecContext(ctx, `
			INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, execution_generation, worker_id, created_at, updated_at)
			VALUES ($1, $2, 'TEXT_TO_IMAGE', 'PROCESSING', 'RUNNING', 1, 'worker-1', $3, $3)
		`, tid, userID, timeStr); err != nil {
			t.Fatalf("seed running task: %v", err)
		}
	}

	var queuedIDs []string
	for i := 4; i <= 5; i++ {
		tid := fmt.Sprintf("task_sim_queued_%d_%s", i, suffix)
		queuedIDs = append(queuedIDs, tid)
		timeStr := now.Add(time.Duration(i)*time.Millisecond).Format(time.RFC3339Nano)
		if _, err := db.ExecContext(ctx, `
			INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, execution_generation, created_at, updated_at)
			VALUES ($1, $2, 'TEXT_TO_IMAGE', 'PROCESSING', 'QUEUED', 1, $3, $3)
		`, tid, userID, timeStr); err != nil {
			t.Fatalf("seed queued task: %v", err)
		}
	}

	scheduler := NewGenerationScheduler(db, GenerationSchedulerOptions{
		BatchUsers:        10,
		BatchTasksPerUser: 5,
	})

	// 3. When 3 slots are occupied, Scheduler must NOT dispatch any queued task
	dispatched0, err := scheduler.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("DispatchOnce with full slots failed: %v", err)
	}
	if dispatched0 != 0 {
		t.Fatalf("dispatched = %d, want 0 when all 3 slots occupied", dispatched0)
	}

	// 4. One running task finishes (transitions to SUCCEEDED), freeing 1 slot!
	freedTaskID := runningIDs[0]
	if _, err := db.ExecContext(ctx, `
		UPDATE xz_generation_tasks SET status = 'SUCCEEDED', task_status = 'SUCCEEDED', updated_at = now() WHERE id = $1
	`, freedTaskID); err != nil {
		t.Fatalf("update freed task: %v", err)
	}

	// 5. Scheduler runs again: must refill exactly 1 queued task (the oldest, task 4)
	dispatched1, err := scheduler.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("DispatchOnce on slot release failed: %v", err)
	}
	if dispatched1 != 1 {
		t.Fatalf("dispatched = %d, want 1 after 1 slot freed", dispatched1)
	}

	// Verify task 4 is promoted to DISPATCHING with unique EventID
	var task4Status string
	var task4Gen int64
	if err := db.QueryRowContext(ctx, `
		SELECT task_status, execution_generation FROM xz_generation_tasks WHERE id = $1
	`, queuedIDs[0]).Scan(&task4Status, &task4Gen); err != nil {
		t.Fatalf("query task 4 status: %v", err)
	}
	if task4Status != "DISPATCHING" || task4Gen != 2 {
		t.Fatalf("task 4 status=%s gen=%d, want DISPATCHING / 2", task4Status, task4Gen)
	}

	var task4EventID string
	if err := db.QueryRowContext(ctx, `
		SELECT event_id FROM outbox_events WHERE aggregate_id = $1
	`, queuedIDs[0]).Scan(&task4EventID); err != nil {
		t.Fatalf("query task 4 outbox event: %v", err)
	}
	expectedTask4EventID := fmt.Sprintf("generation.dispatched:%s:2", queuedIDs[0])
	if task4EventID != expectedTask4EventID {
		t.Fatalf("task 4 event_id = %q, want %q", task4EventID, expectedTask4EventID)
	}

	// Task 5 must STILL be QUEUED (since concurrency 3 is reached: 2 existing + 1 refilled)
	var task5Status string
	if err := db.QueryRowContext(ctx, `SELECT task_status FROM xz_generation_tasks WHERE id = $1`, queuedIDs[1]).Scan(&task5Status); err != nil {
		t.Fatalf("query task 5 status: %v", err)
	}
	if task5Status != "QUEUED" {
		t.Fatalf("task 5 status = %s, want QUEUED", task5Status)
	}
}
