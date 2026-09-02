package providerexecution

import (
	"context"
	"database/sql"
	"errors"
	_ "github.com/jackc/pgx/v5/stdlib"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func openProviderExecutionTestDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "database", "migrations", "114-provider-execution-safety.sql"))
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), string(raw)); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}

func TestProviderExecutionMigrationFreshAndReplayCompatible(t *testing.T) {
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured")
	}
	db := openProviderExecutionTestDB(t, dsn)
	defer db.Close()
	migrationPath := filepath.Join("..", "..", "..", "database", "migrations", "114-provider-execution-safety.sql")
	raw, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatal(err)
	}
	for replay := 0; replay < 2; replay++ {
		if _, err := db.ExecContext(context.Background(), string(raw)); err != nil {
			t.Fatalf("migration replay %d: %v", replay+1, err)
		}
	}
	for _, column := range []string{"provider_operation_key", "result_metadata"} {
		var exists bool
		if err := db.QueryRowContext(context.Background(), `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='provider_executions' AND column_name=$1)`, column).Scan(&exists); err != nil || !exists {
			t.Fatalf("column %s exists=%v err=%v", column, exists, err)
		}
	}
}

func TestPostgresProviderExecutionStore(t *testing.T) {
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured")
	}
	db := openProviderExecutionTestDB(t, dsn)
	defer db.Close()
	ctx := context.Background()
	task := "integration-" + time.Now().UTC().Format("20060102150405.000000000")
	s := NewStore(db)
	e, err := s.CreatePrepared(ctx, Execution{TaskID: task, Provider: "mock", ProviderModel: "m", Capability: "video", RequestFingerprint: "0123456789012345678901234567890123456789012345678901234567890123"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreatePrepared(ctx, Execution{TaskID: task, Provider: "mock", ProviderModel: "m", Capability: "video", Attempt: 1, RequestFingerprint: e.RequestFingerprint}); err == nil {
		t.Fatal("duplicate task/attempt accepted")
	}
	claimed, err := s.ClaimPrepared(ctx, task)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Transition(ctx, claimed.ID, Submitted, ptr("provider-task"), nil, nil); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetLatestByTask(ctx, task)
	if err != nil || got.ProviderRequestID == nil || *got.ProviderRequestID != "provider-task" || got.RequestFingerprint != e.RequestFingerprint {
		t.Fatalf("persisted fields lost: %+v %v", got, err)
	}
	if err = s.Transition(ctx, got.ID, Succeeded, nil, ptr(string(ProviderSucceeded)), nil); err != nil {
		t.Fatal(err)
	}
	if err = s.Transition(ctx, got.ID, Submitting, nil, nil, nil); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("illegal transition accepted: %v", err)
	}
	concurrentTask := task + "-concurrent"
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, x := s.CreatePrepared(ctx, Execution{TaskID: concurrentTask, Provider: "mock", Capability: "image", RequestFingerprint: e.RequestFingerprint})
			results <- x
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for x := range results {
		if x == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent inserts=%d, want 1", success)
	}
	_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id LIKE $1", task+"%")
}
