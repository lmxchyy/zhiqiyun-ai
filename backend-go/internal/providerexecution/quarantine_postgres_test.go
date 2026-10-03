package providerexecution

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestPostgresQuarantineBlocksExecutionWrites(t *testing.T) {
	dsn := testingDatabaseURL(t)
	db := openProviderExecutionTestDB(t, dsn)
	defer db.Close()
	ctx := context.Background()
	store := NewStore(db)
	if _, err := db.ExecContext(ctx, `ALTER TABLE provider_executions ADD COLUMN IF NOT EXISTS task_execution_generation BIGINT`); err != nil {
		t.Fatal(err)
	}
	taskID := "quarantine-" + time.Now().UTC().Format("20060102150405.000000000")
	execution, err := store.CreatePrepared(ctx, Execution{TaskID: taskID, Provider: "mock", ProviderModel: "m", Capability: "image", RequestFingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO provider_execution_quarantine
		(execution_id,task_id,attempt,generation,snapshot_sha256,evidence_sha256,approval_id,release_sha,not_before,expires_at)
		VALUES ($1,$2,1,NULL,$3,$3,'approval-test',$4,now()-interval '1 minute',now()+interval '1 hour')`,
		execution.ID, taskID, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "0123456789abcdef0123456789abcdef01234567"); err != nil {
		t.Fatal(err)
	}
	before := execution.UpdatedAt
	if err := store.Transition(ctx, execution.ID, Submitted, nil, nil, nil); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("transition err=%v", err)
	}
	if err := store.ScheduleNextCheck(ctx, execution.ID, time.Second); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("schedule err=%v", err)
	}
	if err := store.FailPreparedIfUnclaimed(ctx, execution.ID, nil, nil); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("fail prepared err=%v", err)
	}
	if err := store.SaveSucceededResult(ctx, execution.ID, nil, []byte(`{"url":"https://example.com"}`)); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("save succeeded err=%v", err)
	}
	if _, err := store.ClaimPrepared(ctx, taskID); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("claim prepared err=%v", err)
	}
	if _, err := store.RecordCorrelation(ctx, CorrelationEvent{ExecutionID: execution.ID, Kind: "test", ProviderCode: "mock", State: "submitting"}); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("record correlation err=%v", err)
	}
	if _, _, err := store.RecordTerminalCorrelationOnce(ctx, CorrelationEvent{ExecutionID: execution.ID, ProviderCode: "mock", State: "success"}); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("record terminal correlation err=%v", err)
	}
	var correlationCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM provider_execution_correlations WHERE execution_id=$1`, execution.ID).Scan(&correlationCount); err != nil || correlationCount != 0 {
		t.Fatalf("correlations must be 0, got %d err=%v", correlationCount, err)
	}
	// CreatePrepared for a quarantined task must also block with zero rows inserted.
	var execCountBefore int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM provider_executions WHERE task_id=$1`, taskID).Scan(&execCountBefore)
	if _, err := store.CreatePrepared(ctx, Execution{TaskID: taskID, Provider: "mock", ProviderModel: "m", Capability: "image", Attempt: 2, RequestFingerprint: "1111111111111111111111111111111111111111111111111111111111111111"}); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("create prepared after quarantine err=%v", err)
	}
	var execCountAfter int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM provider_executions WHERE task_id=$1`, taskID).Scan(&execCountAfter)
	if execCountAfter != execCountBefore {
		t.Fatalf("exec count mutated: before=%d after=%d", execCountBefore, execCountAfter)
	}
	var status string
	var updated time.Time
	if err := db.QueryRowContext(ctx, `SELECT status, updated_at FROM provider_executions WHERE id=$1`, execution.ID).Scan(&status, &updated); err != nil {
		t.Fatal(err)
	}
	if status != string(Prepared) || !updated.Equal(before) {
		t.Fatalf("execution mutated status=%s updated=%s before=%s", status, updated, before)
	}
	if _, err := db.ExecContext(ctx, `UPDATE provider_execution_quarantine SET approval_id='changed' WHERE execution_id=$1`, execution.ID); err == nil {
		t.Fatal("quarantine row must be immutable (UPDATE blocked)")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM provider_execution_quarantine WHERE execution_id=$1`, execution.ID); err == nil {
		t.Fatal("quarantine row must be immutable (DELETE blocked)")
	}
}

func testingDatabaseURL(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured")
	}
	return dsn
}
