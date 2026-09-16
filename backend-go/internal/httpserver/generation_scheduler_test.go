package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestGenerationSchedulerOptionsDefaults(t *testing.T) {
	opts := DefaultGenerationSchedulerOptions()
	if opts.PollInterval != 500*time.Millisecond {
		t.Errorf("default PollInterval = %v, want 500ms", opts.PollInterval)
	}
	if opts.BatchUsers != 50 {
		t.Errorf("default BatchUsers = %d, want 50", opts.BatchUsers)
	}
	if opts.BatchTasksPerUser != 10 {
		t.Errorf("default BatchTasksPerUser = %d, want 10", opts.BatchTasksPerUser)
	}
	if opts.StaleDispatchTimeout != 60*time.Second {
		t.Errorf("default StaleDispatchTimeout = %v, want 60s", opts.StaleDispatchTimeout)
	}
	if opts.Owner != "generation-fair-scheduler" {
		t.Errorf("default Owner = %q, want generation-fair-scheduler", opts.Owner)
	}
}

func TestGenerationSchedulerLifecycle(t *testing.T) {
	scheduler := NewGenerationScheduler(nil, GenerationSchedulerOptions{
		PollInterval: 10 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- scheduler.Run(ctx)
	}()

	time.Sleep(20 * time.Millisecond)

	if err := scheduler.Run(ctx); err == nil {
		t.Errorf("expected error when starting already running scheduler")
	}

	cancel()
	err := <-errCh
	if err != context.Canceled {
		t.Errorf("expected context.Canceled on stop, got %v", err)
	}
}

func TestFairScheduler_PostgresIntegration(t *testing.T) {
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	planID := "plan_sched_" + suffix
	userA := "user_sched_a_" + suffix
	userB := "user_sched_b_" + suffix

	// Seed plan with concurrency 2
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_plans (id, code, name, concurrency, active)
		VALUES ($1, $1, 'Scheduler Test Plan', 2, true)
		ON CONFLICT (id) DO UPDATE SET concurrency = 2
	`, planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	// Seed User A and User B
	for _, u := range []string{userA, userB} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO xz_users (id, email, name, role, status, plan_id)
			VALUES ($1, $1 || '@test.local', 'Sched User', 'MEMBER', 'ACTIVE', $2)
			ON CONFLICT (id) DO UPDATE SET plan_id = $2
		`, u, planID); err != nil {
			t.Fatalf("seed user %s: %v", u, err)
		}
	}

	// Seed 5 QUEUED tasks for User A
	now := time.Now().UTC()
	for i := 1; i <= 5; i++ {
		taskID := fmt.Sprintf("task_sched_a_%d_%s", i, suffix)
		createdAt := now.Add(time.Duration(i) * time.Millisecond).Format(time.RFC3339Nano)
		if _, err := db.ExecContext(ctx, `
			INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, created_at, updated_at)
			VALUES ($1, $2, 'TEXT_TO_IMAGE', 'PROCESSING', 'QUEUED', $3, $3)
		`, taskID, userA, createdAt); err != nil {
			t.Fatalf("seed user A task: %v", err)
		}
	}

	// Seed 1 QUEUED task for User B
	taskB := fmt.Sprintf("task_sched_b_1_%s", suffix)
	createdAtB := now.Add(100 * time.Millisecond).Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, created_at, updated_at)
		VALUES ($1, $2, 'TEXT_TO_IMAGE', 'PROCESSING', 'QUEUED', $3, $3)
	`, taskB, userB, createdAtB); err != nil {
		t.Fatalf("seed user B task: %v", err)
	}

	scheduler := NewGenerationScheduler(db, GenerationSchedulerOptions{
		BatchUsers:        10,
		BatchTasksPerUser: 5,
	})

	// Run DispatchOnce
	dispatched, err := scheduler.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("DispatchOnce failed: %v", err)
	}

	// Expect 3 tasks total dispatched: User A gets 2 (capped at concurrency 2), User B gets 1
	if dispatched != 3 {
		t.Fatalf("dispatched = %d, want 3 (User A 2, User B 1)", dispatched)
	}

	// Check User A task statuses: 2 DISPATCHING, 3 QUEUED
	var userADispatching, userAQueued int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FILTER (WHERE task_status = 'DISPATCHING'),
		       count(*) FILTER (WHERE task_status = 'QUEUED')
		FROM xz_generation_tasks WHERE user_id = $1
	`, userA).Scan(&userADispatching, &userAQueued); err != nil {
		t.Fatalf("query user A counts: %v", err)
	}
	if userADispatching != 2 || userAQueued != 3 {
		t.Fatalf("user A counts: dispatching=%d queued=%d, want 2/3", userADispatching, userAQueued)
	}

	// Check User B task status: 1 DISPATCHING, 0 QUEUED (proves User B was not starved)
	var userBDispatching, userBQueued int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FILTER (WHERE task_status = 'DISPATCHING'),
		       count(*) FILTER (WHERE task_status = 'QUEUED')
		FROM xz_generation_tasks WHERE user_id = $1
	`, userB).Scan(&userBDispatching, &userBQueued); err != nil {
		t.Fatalf("query user B counts: %v", err)
	}
	if userBDispatching != 1 || userBQueued != 0 {
		t.Fatalf("user B counts: dispatching=%d queued=%d, want 1/0", userBDispatching, userBQueued)
	}

	// Test Stale Recovery: set one task to DISPATCHING with old updated_at
	staleTaskID := fmt.Sprintf("task_sched_stale_%s", suffix)
	staleTime := time.Now().UTC().Add(-120 * time.Second).Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, created_at, updated_at)
		VALUES ($1, $2, 'TEXT_TO_IMAGE', 'PROCESSING', 'DISPATCHING', $3, $3)
	`, staleTaskID, userA, staleTime); err != nil {
		t.Fatalf("seed stale task: %v", err)
	}

	recovered, err := scheduler.RecoverStaleDispatches(ctx)
	if err != nil {
		t.Fatalf("RecoverStaleDispatches failed: %v", err)
	}
	if recovered < 1 {
		t.Fatalf("recovered = %d, want at least 1", recovered)
	}

	var staleStatus string
	if err := db.QueryRowContext(ctx, `SELECT task_status FROM xz_generation_tasks WHERE id = $1`, staleTaskID).Scan(&staleStatus); err != nil {
		t.Fatalf("query stale task status: %v", err)
	}
	if staleStatus != "QUEUED" {
		t.Fatalf("stale task status after recovery = %s, want QUEUED", staleStatus)
	}
}
