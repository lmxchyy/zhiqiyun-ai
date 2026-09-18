package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func openTestPostgresDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured; skipping Postgres integration test")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("ping postgres: %v", err)
	}
	return db
}

// -----------------------------------------------------------------------------
// Part 1: Recovery Starvation Elimination Tests
// -----------------------------------------------------------------------------

// TestRecoveryStarvation_100AmbiguousPlus1Recoverable proves that 100 older ambiguous
// tasks in DISPATCHING cannot starve downstream recoverable virgin tasks.
func TestRecoveryStarvation_100AmbiguousPlus1Recoverable(t *testing.T) {
	db := openTestPostgresDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	userID := "user_starve_100_" + suffix

	// Seed user
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_users (id, email, name, role, status)
		VALUES ($1, $1 || '@test.local', 'Starvation Test User', 'MEMBER', 'ACTIVE')
		ON CONFLICT (id) DO NOTHING
	`, userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	staleTime := time.Now().UTC().Add(-180 * time.Second).Format(time.RFC3339Nano)
	staleTimeRecoverable := time.Now().UTC().Add(-120 * time.Second).Format(time.RFC3339Nano)

	// Clean up afterward
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id LIKE '%"+suffix+"%'")
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE user_id = $1", userID)
	}()

	// 1. Seed 100 ambiguous tasks in DISPATCHING (e.g. outbox event already published)
	for i := 1; i <= 100; i++ {
		taskID := fmt.Sprintf("rec_t_ambig_%03d_%s", i, suffix)
		eventID := fmt.Sprintf("event_ambig_%03d_%s", i, suffix)

		if _, err := db.ExecContext(ctx, `
			INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, execution_generation, created_at, updated_at)
			VALUES ($1, $2, 'TEXT_TO_IMAGE', 'PROCESSING', 'DISPATCHING', 1, $3, $3)
		`, taskID, userID, staleTime); err != nil {
			t.Fatalf("seed ambiguous task %d: %v", i, err)
		}

		// Insert outbox event with status='published' (ambiguous - must NOT be reaped by virgin recovery)
		if _, err := db.ExecContext(ctx, `
			INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, event_type, payload, status, attempt_count, published_at, created_at, updated_at)
			VALUES ($1, 'generation_task', $2, 'x.ai.generation.image.canary.requested', ('{"task_id":"' || $2 || '"}')::jsonb, 'published', 1, now(), now(), now())
		`, eventID, taskID); err != nil {
			t.Fatalf("seed published outbox event %d: %v", i, err)
		}
	}

	// 2. Seed 1 recoverable virgin dispatch task (updated_at is NEWER than the ambiguous ones)
	recoverableTaskID := fmt.Sprintf("rec_t_virgin_001_%s", suffix)
	recoverableEventID := fmt.Sprintf("event_virgin_001_%s", suffix)

	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, execution_generation, created_at, updated_at)
		VALUES ($1, $2, 'TEXT_TO_IMAGE', 'PROCESSING', 'DISPATCHING', 1, $3, $3)
	`, recoverableTaskID, userID, staleTimeRecoverable); err != nil {
		t.Fatalf("seed recoverable task: %v", err)
	}

	// Virgin outbox event: status='pending', attempt_count=0, never claimed/published
	if _, err := db.ExecContext(ctx, `
		INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, event_type, payload, status, attempt_count, created_at, updated_at)
		VALUES ($1, 'generation_task', $2, 'x.ai.generation.image.canary.requested', ('{"task_id":"' || $2 || '"}')::jsonb, 'pending', 0, now(), now())
	`, recoverableEventID, recoverableTaskID); err != nil {
		t.Fatalf("seed virgin outbox event: %v", err)
	}

	// 3. Run RecoverStaleDispatches with BatchSize = 50 (smaller than the 100 ambiguous tasks)
	scheduler := NewGenerationScheduler(db, GenerationSchedulerOptions{
		BatchUsers:           50,
		StaleDispatchTimeout: 60 * time.Second,
	})

	recovered, err := scheduler.RecoverStaleDispatches(ctx)
	if err != nil {
		t.Fatalf("RecoverStaleDispatches failed: %v", err)
	}

	// The 1 recoverable task MUST be recovered on the first tick despite 100 older ambiguous rows!
	if recovered < 1 {
		t.Fatalf("recovered = %d, want >= 1 (recoverable task was starved!)", recovered)
	}

	// 4. Verify recoverable task was reset to QUEUED, generation bumped to 2
	var taskStatus string
	var gen int64
	if err := db.QueryRowContext(ctx, `
		SELECT task_status, execution_generation
		FROM xz_generation_tasks WHERE id = $1
	`, recoverableTaskID).Scan(&taskStatus, &gen); err != nil {
		t.Fatalf("query recoverable task: %v", err)
	}
	if taskStatus != "QUEUED" {
		t.Errorf("recoverable task status = %s, want QUEUED", taskStatus)
	}
	if gen != 2 {
		t.Errorf("recoverable task generation = %d, want 2", gen)
	}

	// Verify the virgin outbox event was deleted
	var outboxCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM outbox_events WHERE event_id = $1`, recoverableEventID).Scan(&outboxCount); err != nil {
		t.Fatalf("query outbox count: %v", err)
	}
	if outboxCount != 0 {
		t.Errorf("virgin outbox event still exists (count = %d), want 0", outboxCount)
	}

	// 5. Verify the 100 ambiguous tasks were NOT modified
	var ambigDispatchingCount int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM xz_generation_tasks
		WHERE user_id = $1 AND id LIKE 'rec_t_ambig_%' AND task_status = 'DISPATCHING'
	`, userID).Scan(&ambigDispatchingCount); err != nil {
		t.Fatalf("query ambiguous count: %v", err)
	}
	if ambigDispatchingCount != 100 {
		t.Errorf("ambiguous dispatching count = %d, want 100", ambigDispatchingCount)
	}
}

// TestRecoveryStarvation_1000AmbiguousPlusMultipleRecoverable tests scale resilience
// with 1000 ambiguous rows and multiple recoverable tasks behind them.
func TestRecoveryStarvation_1000AmbiguousPlusMultipleRecoverable(t *testing.T) {
	db := openTestPostgresDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	userID := "user_starve_1k_" + suffix

	// Seed user
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_users (id, email, name, role, status)
		VALUES ($1, $1 || '@test.local', '1k Starvation User', 'MEMBER', 'ACTIVE')
		ON CONFLICT (id) DO NOTHING
	`, userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id LIKE '%"+suffix+"%'")
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE user_id = $1", userID)
	}()

	staleTime := time.Now().UTC().Add(-300 * time.Second).Format(time.RFC3339Nano)

	// Batch insert 1000 ambiguous tasks using generate_series
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, execution_generation, created_at, updated_at)
		SELECT 'rec_t_1k_ambig_' || lpad(i::text, 4, '0') || '_' || $1,
		       $2, 'TEXT_TO_IMAGE', 'PROCESSING', 'DISPATCHING', 1, $3::text, $3::text
		FROM generate_series(1, 1000) i
	`, suffix, userID, staleTime); err != nil {
		t.Fatalf("bulk insert 1000 ambiguous tasks: %v", err)
	}

	// Insert outbox events with attempt_count = 1 (making them ambiguous / non-virgin)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, event_type, payload, status, attempt_count, created_at, updated_at)
		SELECT 'event_1k_ambig_' || lpad(i::text, 4, '0') || '_' || $1,
		       'generation_task',
		       'rec_t_1k_ambig_' || lpad(i::text, 4, '0') || '_' || $1,
		       'x.ai.generation.image.canary.requested',
		       '{}'::jsonb, 'pending', 1, now(), now()
		FROM generate_series(1, 1000) i
	`, suffix); err != nil {
		t.Fatalf("bulk insert 1000 ambiguous outbox events: %v", err)
	}

	scheduler := NewGenerationScheduler(db, GenerationSchedulerOptions{
		BatchUsers:           50,
		StaleDispatchTimeout: 60 * time.Second,
	})

	// Drain any pre-existing stale dispatches so count is deterministic
	_, _ = scheduler.RecoverStaleDispatches(ctx)

	// Now insert 5 recoverable virgin tasks
	for i := 1; i <= 5; i++ {
		tID := fmt.Sprintf("rec_t_1k_rec_%d_%s", i, suffix)
		eID := fmt.Sprintf("event_1k_rec_%d_%s", i, suffix)
		if _, err := db.ExecContext(ctx, `
			INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, execution_generation, created_at, updated_at)
			VALUES ($1, $2, 'TEXT_TO_IMAGE', 'PROCESSING', 'DISPATCHING', 1, $3, $3)
		`, tID, userID, staleTime); err != nil {
			t.Fatalf("insert recoverable task %d: %v", i, err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, event_type, payload, status, attempt_count, created_at, updated_at)
			VALUES ($1, 'generation_task', $2, 'x.ai.generation.image.canary.requested', '{}'::jsonb, 'pending', 0, now(), now())
		`, eID, tID); err != nil {
			t.Fatalf("insert virgin outbox event %d: %v", i, err)
		}
	}

	recovered, err := scheduler.RecoverStaleDispatches(ctx)
	if err != nil {
		t.Fatalf("RecoverStaleDispatches failed: %v", err)
	}

	// All 5 recoverable tasks must be recovered despite 1000 ambiguous tasks
	if recovered < 5 {
		t.Fatalf("recovered = %d, want at least 5", recovered)
	}

	// Verify all 5 virgin tasks for this user were recovered to QUEUED with generation 2
	var userRecoveredCount int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM xz_generation_tasks
		WHERE user_id = $1 AND id LIKE 'rec_t_1k_rec_%' AND task_status = 'QUEUED' AND execution_generation = 2
	`, userID).Scan(&userRecoveredCount); err != nil {
		t.Fatalf("query user recovered count: %v", err)
	}
	if userRecoveredCount != 5 {
		t.Errorf("user recovered count = %d, want 5", userRecoveredCount)
	}

	// Verify ambiguous tasks remain untouched
	var ambigLeft int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM xz_generation_tasks
		WHERE user_id = $1 AND id LIKE 'rec_t_1k_ambig_%' AND task_status = 'DISPATCHING'
	`, userID).Scan(&ambigLeft); err != nil {
		t.Fatalf("query remaining ambiguous: %v", err)
	}
	if ambigLeft != 1000 {
		t.Errorf("ambiguous count = %d, want 1000", ambigLeft)
	}
}

// TestRecovery_MultiSchedulerConcurrentRecovery proves that multiple scheduler
// instances running RecoverStaleDispatches concurrently do not duplicate recovery.
func TestRecovery_MultiSchedulerConcurrentRecovery(t *testing.T) {
	db := openTestPostgresDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	userID := "user_multi_rec_" + suffix

	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_users (id, email, name, role, status)
		VALUES ($1, $1 || '@test.local', 'Multi Scheduler Recovery User', 'MEMBER', 'ACTIVE')
		ON CONFLICT (id) DO NOTHING
	`, userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id LIKE '%"+suffix+"%'")
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE user_id = $1", userID)
	}()

	staleTime := time.Now().UTC().Add(-120 * time.Second).Format(time.RFC3339Nano)
	taskCount := 20

	// Seed 20 virgin dispatch tasks
	for i := 1; i <= taskCount; i++ {
		tID := fmt.Sprintf("rec_t_multi_%02d_%s", i, suffix)
		eID := fmt.Sprintf("event_multi_%02d_%s", i, suffix)
		if _, err := db.ExecContext(ctx, `
			INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, execution_generation, created_at, updated_at)
			VALUES ($1, $2, 'TEXT_TO_IMAGE', 'PROCESSING', 'DISPATCHING', 1, $3, $3)
		`, tID, userID, staleTime); err != nil {
			t.Fatalf("insert task %d: %v", i, err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, event_type, payload, status, attempt_count, created_at, updated_at)
			VALUES ($1, 'generation_task', $2, 'x.ai.generation.image.canary.requested', '{}'::jsonb, 'pending', 0, now(), now())
		`, eID, tID); err != nil {
			t.Fatalf("insert outbox %d: %v", i, err)
		}
	}

	// Launch 4 concurrent schedulers
	numSchedulers := 4
	var wg sync.WaitGroup
	recoveredCounts := make([]int, numSchedulers)
	errs := make([]error, numSchedulers)

	for i := 0; i < numSchedulers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			s := NewGenerationScheduler(db, GenerationSchedulerOptions{
				BatchUsers:           10,
				StaleDispatchTimeout: 60 * time.Second,
				Owner:                fmt.Sprintf("scheduler-replica-%d", idx),
			})
			recoveredCounts[idx], errs[idx] = s.RecoverStaleDispatches(ctx)
		}(i)
	}
	wg.Wait()

	totalRecovered := 0
	for i := 0; i < numSchedulers; i++ {
		if errs[i] != nil {
			t.Fatalf("scheduler %d error: %v", i, errs[i])
		}
		totalRecovered += recoveredCounts[i]
	}

	// Exactly 20 tasks recovered across all schedulers, no duplicates
	if totalRecovered != taskCount {
		t.Fatalf("total recovered = %d, want %d", totalRecovered, taskCount)
	}

	// Verify all 20 are now QUEUED with generation = 2
	var queuedCount int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM xz_generation_tasks
		WHERE user_id = $1 AND task_status = 'QUEUED' AND execution_generation = 2
	`, userID).Scan(&queuedCount); err != nil {
		t.Fatalf("query queued count: %v", err)
	}
	if queuedCount != taskCount {
		t.Fatalf("queued count with generation 2 = %d, want %d", queuedCount, taskCount)
	}
}

// -----------------------------------------------------------------------------
// Part 2: PPT Routing & Fail-Fast Tests
// -----------------------------------------------------------------------------

// TestPPT_FailFastBeforeFreezeWhenCapacityFull proves that PPT requests fail fast
// with 429 / errGenerationConcurrencyLimit BEFORE any points are frozen or orphan rows created.
func TestPPT_FailFastBeforeFreezeWhenCapacityFull(t *testing.T) {
	db := openTestPostgresDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	userID := "user_ppt_failfast_" + suffix
	planID := "plan_ppt_1slot_" + suffix

	// 1. Seed plan with concurrency = 1
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_plans (id, code, name, concurrency, active)
		VALUES ($1, $1, '1-Slot Plan', 1, true)
		ON CONFLICT (id) DO UPDATE SET concurrency = 1
	`, planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	// 2. Seed user with 5000 available points
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_users (id, email, name, role, status, plan_id)
		VALUES ($1, $1 || '@test.local', 'PPT Test User', 'MEMBER', 'ACTIVE', $2)
		ON CONFLICT (id) DO UPDATE SET plan_id = $2
	`, userID, planID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	pointStore := NewPostgresPersonalPointStore(db)
	accountID := "acc-" + userID
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{
		AccountID:      accountID,
		UserID:         userID,
		Source:         PointSourceRecharge,
		Points:         5000,
		IdempotencyKey: "grant-" + suffix,
	}); err != nil {
		t.Fatalf("grant points: %v", err)
	}

	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id LIKE '%"+suffix+"%'")
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_personal_point_lot_movements WHERE user_id = $1", userID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_personal_point_reservations WHERE user_id = $1", userID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_ppt_tasks WHERE user_id = $1", userID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE user_id = $1", userID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_point_accounts WHERE user_id = $1", userID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_users WHERE id = $1", userID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_plans WHERE id = $1", planID)
	}()

	// 3. Occupy the 1 running slot with an active image task
	activeTaskID := "rec_t_active_img_" + suffix
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, created_at, updated_at)
		VALUES ($1, $2, 'TEXT_TO_IMAGE', 'PROCESSING', 'RUNNING', now(), now())
	`, activeTaskID, userID); err != nil {
		t.Fatalf("seed active running task: %v", err)
	}

	store := newPostgresPrimaryStore(db, "")

	// 4. Submit a PPT canary request
	clientReqID := "ppt-req-failfast-" + suffix
	capReq, pptReq := pptAcceptanceRequest(clientReqID)
	capReq.UserID = userID
	pptReq.UserID = userID

	_, err := store.CreatePendingGenerationTaskWithPPTCanaryOutbox(capReq, pptReq)
	if err == nil {
		t.Fatalf("expected errGenerationConcurrencyLimit, got nil!")
	}
	if !errors.Is(err, errGenerationConcurrencyLimit) {
		t.Fatalf("expected errGenerationConcurrencyLimit, got: %v", err)
	}

	// 5. PROVE FAIL-FAST INVARIANTS:
	// - 0 points reserved
	var resCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_reservations WHERE user_id = $1`, userID).Scan(&resCount); err != nil {
		t.Fatalf("query reservations: %v", err)
	}
	if resCount != 0 {
		t.Errorf("orphan reservation created! count = %d, want 0", resCount)
	}

	// - User available balance unchanged (still 5000), frozen = 0
	var available, frozen int64
	if err := db.QueryRowContext(ctx, `SELECT available, frozen FROM xz_point_accounts WHERE user_id = $1`, userID).Scan(&available, &frozen); err != nil {
		t.Fatalf("query point account: %v", err)
	}
	if available != 5000 || frozen != 0 {
		t.Errorf("point balance changed! available=%d (want 5000), frozen=%d (want 0)", available, frozen)
	}

	// - 0 PPT tasks created
	var pptCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_ppt_tasks WHERE user_id = $1`, userID).Scan(&pptCount); err != nil {
		t.Fatalf("query ppt tasks: %v", err)
	}
	if pptCount != 0 {
		t.Errorf("orphan ppt task created! count = %d, want 0", pptCount)
	}

	// - 0 generation tasks for this client_request_id (only the initial active task exists)
	var genCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_generation_tasks WHERE client_request_id = $1`, clientReqID).Scan(&genCount); err != nil {
		t.Fatalf("query gen tasks: %v", err)
	}
	if genCount != 0 {
		t.Errorf("orphan generation task created! count = %d, want 0", genCount)
	}

	// 6. Test Idempotency: replaying the same request also fails cleanly without side-effects
	_, err2 := store.CreatePendingGenerationTaskWithPPTCanaryOutbox(capReq, pptReq)
	if !errors.Is(err2, errGenerationConcurrencyLimit) {
		t.Fatalf("replayed call expected errGenerationConcurrencyLimit, got: %v", err2)
	}

	// Recheck point balance
	_ = db.QueryRowContext(ctx, `SELECT available, frozen FROM xz_point_accounts WHERE user_id = $1`, userID).Scan(&available, &frozen)
	if available != 5000 || frozen != 0 {
		t.Errorf("replayed call altered point balance! available=%d, frozen=%d", available, frozen)
	}
}

// TestPPT_ExcludedFromFairScheduler proves that PPT tasks are never dispatched by the FairScheduler.
func TestPPT_ExcludedFromFairScheduler(t *testing.T) {
	db := openTestPostgresDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	userID := "user_ppt_nosched_" + suffix
	planID := "plan_ppt_sched_" + suffix

	// Seed plan with concurrency = 2
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_plans (id, code, name, concurrency, active)
		VALUES ($1, $1, '2-Slot Plan', 2, true)
		ON CONFLICT (id) DO UPDATE SET concurrency = 2
	`, planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_users (id, email, name, role, status, plan_id)
		VALUES ($1, $1 || '@test.local', 'PPT Sched User', 'MEMBER', 'ACTIVE', $2)
		ON CONFLICT (id) DO UPDATE SET plan_id = $2
	`, userID, planID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id LIKE '%"+suffix+"%'")
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE user_id = $1", userID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_users WHERE id = $1", userID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_plans WHERE id = $1", planID)
	}()

	now := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano)

	// Seed 1 QUEUED PPT task
	pptTaskID := "rec_t_ppt_queued_" + suffix
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, created_at, updated_at)
		VALUES ($1, $2, 'PPT_GENERATION', 'PROCESSING', 'QUEUED', $3, $3)
	`, pptTaskID, userID, now); err != nil {
		t.Fatalf("seed queued ppt task: %v", err)
	}

	// Seed 1 QUEUED Image task
	imgTaskID := "rec_t_img_queued_" + suffix
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, created_at, updated_at)
		VALUES ($1, $2, 'TEXT_TO_IMAGE', 'PROCESSING', 'QUEUED', $3, $3)
	`, imgTaskID, userID, now); err != nil {
		t.Fatalf("seed queued image task: %v", err)
	}

	scheduler := NewGenerationScheduler(db, GenerationSchedulerOptions{
		BatchUsers:        10,
		BatchTasksPerUser: 10,
	})

	_, err := scheduler.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("DispatchOnce failed: %v", err)
	}

	// Image task status should be DISPATCHING
	var imgStatus string
	if err := db.QueryRowContext(ctx, `SELECT task_status FROM xz_generation_tasks WHERE id = $1`, imgTaskID).Scan(&imgStatus); err != nil {
		t.Fatalf("query img task: %v", err)
	}
	if imgStatus != "DISPATCHING" {
		t.Errorf("image task status = %s, want DISPATCHING", imgStatus)
	}

	// PPT task status must STILL be QUEUED (untouched by scheduler)
	var pptStatus string
	if err := db.QueryRowContext(ctx, `SELECT task_status FROM xz_generation_tasks WHERE id = $1`, pptTaskID).Scan(&pptStatus); err != nil {
		t.Fatalf("query ppt task: %v", err)
	}
	if pptStatus != "QUEUED" {
		t.Errorf("ppt task status = %s, want QUEUED (scheduler must not touch PPT)", pptStatus)
	}
}

// TestRecovery_KeysetCursorContinuationAndRestart proves that keyset cursor
// advances across batches and successfully recovers all tasks without infinite loops.
func TestRecovery_KeysetCursorContinuationAndRestart(t *testing.T) {
	db := openTestPostgresDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	userID := "user_cursor_rec_" + suffix

	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_users (id, email, name, role, status)
		VALUES ($1, $1 || '@test.local', 'Cursor Test User', 'MEMBER', 'ACTIVE')
		ON CONFLICT (id) DO NOTHING
	`, userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id LIKE '%"+suffix+"%'")
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE user_id = $1", userID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_users WHERE id = $1", userID)
	}()

	staleTime := time.Now().UTC().Add(-120 * time.Second).Format(time.RFC3339Nano)
	totalTasks := 120
	batchSize := 50

	for i := 1; i <= totalTasks; i++ {
		tID := fmt.Sprintf("rec_t_cur_%03d_%s", i, suffix)
		eID := fmt.Sprintf("event_cur_%03d_%s", i, suffix)
		if _, err := db.ExecContext(ctx, `
			INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, execution_generation, created_at, updated_at)
			VALUES ($1, $2, 'TEXT_TO_IMAGE', 'PROCESSING', 'DISPATCHING', 1, $3, $3)
		`, tID, userID, staleTime); err != nil {
			t.Fatalf("insert task %d: %v", i, err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, event_type, payload, status, attempt_count, created_at, updated_at)
			VALUES ($1, 'generation_task', $2, 'x.ai.generation.image.canary.requested', ('{"task_id":"' || $2 || '"}')::jsonb, 'pending', 0, now(), now())
		`, eID, tID); err != nil {
			t.Fatalf("insert outbox %d: %v", i, err)
		}
	}

	scheduler := NewGenerationScheduler(db, GenerationSchedulerOptions{
		BatchUsers:           batchSize,
		StaleDispatchTimeout: 60 * time.Second,
	})

	totalRecovered := 0
	for round := 1; round <= 5; round++ {
		n, err := scheduler.RecoverStaleDispatches(ctx)
		if err != nil {
			t.Fatalf("round %d RecoverStaleDispatches failed: %v", round, err)
		}
		totalRecovered += n
		if totalRecovered >= totalTasks {
			break
		}
	}

	if totalRecovered < totalTasks {
		t.Fatalf("total recovered = %d, want at least %d", totalRecovered, totalTasks)
	}

	// Verify all tasks are QUEUED with gen = 2
	var queuedCount int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM xz_generation_tasks
		WHERE user_id = $1 AND task_status = 'QUEUED' AND execution_generation = 2
	`, userID).Scan(&queuedCount); err != nil {
		t.Fatalf("query queued count: %v", err)
	}
	if queuedCount != totalTasks {
		t.Fatalf("queued count = %d, want %d", queuedCount, totalTasks)
	}
}

// TestPPT_SucceedsWithOutboxWhenCapacityAvailable proves that when user concurrency
// is available, PPT canary requests succeed, reserve points, and write the PPT outbox event.
func TestPPT_SucceedsWithOutboxWhenCapacityAvailable(t *testing.T) {
	db := openTestPostgresDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	userID := "user_ppt_avail_" + suffix
	planID := "plan_ppt_avail_" + suffix

	// Seed plan with concurrency = 3
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_plans (id, code, name, concurrency, active)
		VALUES ($1, $1, '3-Slot Plan', 3, true)
		ON CONFLICT (id) DO UPDATE SET concurrency = 3
	`, planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_users (id, email, name, role, status, plan_id)
		VALUES ($1, $1 || '@test.local', 'PPT Avail User', 'MEMBER', 'ACTIVE', $2)
		ON CONFLICT (id) DO UPDATE SET plan_id = $2
	`, userID, planID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	pointStore := NewPostgresPersonalPointStore(db)
	accountID := "acc-" + userID
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{
		AccountID:      accountID,
		UserID:         userID,
		Source:         PointSourceRecharge,
		Points:         10000,
		IdempotencyKey: "grant-" + suffix,
	}); err != nil {
		t.Fatalf("grant points: %v", err)
	}

	var createdTaskID string
	defer func() {
		if createdTaskID != "" {
			_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id = $1", createdTaskID)
			_, _ = db.ExecContext(ctx, "DELETE FROM xz_personal_point_lot_movements WHERE user_id = $1", userID)
			_, _ = db.ExecContext(ctx, "DELETE FROM xz_personal_point_reservations WHERE business_id = $1 AND user_id = $2", createdTaskID, userID)
			_, _ = db.ExecContext(ctx, "DELETE FROM xz_ppt_tasks WHERE task_id = $1", createdTaskID)
			_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id = $1", createdTaskID)
		}
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_point_accounts WHERE user_id = $1", userID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_users WHERE id = $1", userID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_plans WHERE id = $1", planID)
	}()

	store := newPostgresPrimaryStore(db, "")

	clientReqID := "ppt-req-avail-" + suffix
	capReq, pptReq := pptAcceptanceRequest(clientReqID)
	capReq.UserID = userID
	pptReq.UserID = userID

	task, err := store.CreatePendingGenerationTaskWithPPTCanaryOutbox(capReq, pptReq)
	if err != nil {
		t.Fatalf("CreatePendingGenerationTaskWithPPTCanaryOutbox failed: %v", err)
	}
	createdTaskID = task.ID

	if task.ID == "" {
		t.Fatalf("returned task.ID is empty")
	}

	// Verify points were reserved
	var resCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_reservations WHERE business_id = $1 AND user_id = $2`, task.ID, userID).Scan(&resCount); err != nil {
		t.Fatalf("query reservations: %v", err)
	}
	if resCount != 1 {
		t.Errorf("reservation count = %d, want 1", resCount)
	}

	// Verify PPT task created
	var pptCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_ppt_tasks WHERE task_id = $1`, task.ID).Scan(&pptCount); err != nil {
		t.Fatalf("query ppt tasks: %v", err)
	}
	if pptCount != 1 {
		t.Errorf("ppt task count = %d, want 1", pptCount)
	}

	// Verify outbox event created with PPT routing key
	var outboxEventType, outboxEventID string
	if err := db.QueryRowContext(ctx, `SELECT event_type, event_id FROM outbox_events WHERE aggregate_id = $1`, task.ID).Scan(&outboxEventType, &outboxEventID); err != nil {
		t.Fatalf("query outbox event: %v", err)
	}
	if outboxEventType != "x.ai.generation.ppt.canary.requested" {
		t.Errorf("outbox event_type = %s, want x.ai.generation.ppt.canary.requested", outboxEventType)
	}
	expectedEventID := "generation.ppt.requested:" + task.ID
	if outboxEventID != expectedEventID {
		t.Errorf("outbox event_id = %s, want %s", outboxEventID, expectedEventID)
	}
}
