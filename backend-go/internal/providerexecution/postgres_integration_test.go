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
	for _, name := range []string{"114-provider-execution-safety.sql", "120-provider-execution-correlation.sql"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "database", "migrations", name))
		if err != nil {
			db.Close()
			t.Fatal(err)
		}
		if _, err := db.ExecContext(context.Background(), string(raw)); err != nil {
			db.Close()
			t.Fatal(err)
		}
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
	var correlationTableExists bool
	if err := db.QueryRowContext(context.Background(), `SELECT to_regclass('provider_execution_correlations') IS NOT NULL`).Scan(&correlationTableExists); err != nil || !correlationTableExists {
		t.Fatalf("provider_execution_correlations exists=%v err=%v", correlationTableExists, err)
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
	for _, event := range []CorrelationEvent{
		{ExecutionID: got.ID, Kind: "create_response", ProviderCode: "channel-a", Host: "provider.example", Path: "/v1/videos", JobID: "submit-id", JobRole: "submit", State: "processing", HTTPStatus: 200},
		{ExecutionID: got.ID, Kind: "poll_response", ProviderCode: "channel-a", Host: "provider.example", Path: "/v1/videos/terminal-id", JobID: "terminal-id", JobRole: "poll", State: "failed", HTTPStatus: 200, ErrorCode: "generation_failed", ErrorHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	} {
		if _, err := s.RecordCorrelation(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	correlations, err := s.ListCorrelations(ctx, got.ID)
	if err != nil || len(correlations) != 2 || correlations[0].JobID != "submit-id" || correlations[1].JobID != "terminal-id" || correlations[1].ErrorHash != "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("correlations=%+v err=%v", correlations, err)
	}
	if _, err := s.RecordCorrelation(ctx, CorrelationEvent{ExecutionID: got.ID, Kind: "unsafe", Host: "https://provider.example/v1", ErrorHash: "secret"}); err == nil {
		t.Fatal("unsafe correlation fields were accepted")
	}
	if _, err := s.RecordCorrelation(ctx, CorrelationEvent{ExecutionID: got.ID, Kind: "unsafe", ErrorCode: "raw provider error text"}); err == nil {
		t.Fatal("raw error text was accepted as a correlation code")
	}
	attemptTwo, err := s.CreatePrepared(ctx, Execution{TaskID: task, Provider: "mock", ProviderModel: "m", Capability: "video", Attempt: 2, RequestFingerprint: e.RequestFingerprint})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordCorrelation(ctx, CorrelationEvent{ExecutionID: attemptTwo.ID, Kind: "replay_poll", ProviderCode: "channel-a", Host: "provider.example", Path: "/v1/videos/submit-id", JobID: "submit-id", JobRole: "poll", State: "processing", HTTPStatus: 200}); err != nil {
		t.Fatal(err)
	}
	attemptTwoCorrelations, err := s.ListCorrelations(ctx, attemptTwo.ID)
	if err != nil || len(attemptTwoCorrelations) != 1 || attemptTwoCorrelations[0].ExecutionID == got.ID {
		t.Fatalf("attempt correlations=%+v err=%v", attemptTwoCorrelations, err)
	}
	_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id LIKE $1", task+"%")
}
