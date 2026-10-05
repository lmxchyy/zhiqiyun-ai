package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

func setupHTTPServerConcurrencyTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		t.Fatal(err)
	}

	ddl := `
	CREATE TABLE IF NOT EXISTS xz_generation_tasks (
		id text PRIMARY KEY,
		status text NOT NULL DEFAULT 'PROCESSING',
		task_status text NOT NULL DEFAULT 'RUNNING',
		execution_generation bigint DEFAULT 1,
		worker_id text,
		lease_until timestamptz,
		last_heartbeat_at timestamptz,
		type text DEFAULT 'TEXT_TO_IMAGE',
		params jsonb DEFAULT '{}',
		created_at timestamptz DEFAULT now(),
		updated_at timestamptz DEFAULT now()
	);
	ALTER TABLE xz_generation_tasks ADD COLUMN IF NOT EXISTS task_status text NOT NULL DEFAULT 'RUNNING';
	ALTER TABLE xz_generation_tasks ADD COLUMN IF NOT EXISTS execution_generation bigint DEFAULT 1;
	ALTER TABLE xz_generation_tasks ADD COLUMN IF NOT EXISTS worker_id text;
	ALTER TABLE xz_generation_tasks ADD COLUMN IF NOT EXISTS lease_until timestamptz;
	ALTER TABLE xz_generation_tasks ADD COLUMN IF NOT EXISTS last_heartbeat_at timestamptz;
	ALTER TABLE xz_generation_tasks ADD COLUMN IF NOT EXISTS params jsonb DEFAULT '{}';

	CREATE TABLE IF NOT EXISTS xz_point_accounts (
		id text PRIMARY KEY,
		user_id text NOT NULL,
		available bigint NOT NULL DEFAULT 0,
		frozen bigint NOT NULL DEFAULT 0,
		total_granted bigint NOT NULL DEFAULT 0,
		total_consumed bigint NOT NULL DEFAULT 0,
		status text NOT NULL DEFAULT 'active',
		created_at timestamptz DEFAULT now(),
		updated_at timestamptz DEFAULT now()
	);
	ALTER TABLE xz_point_accounts ADD COLUMN IF NOT EXISTS total_granted bigint NOT NULL DEFAULT 0;
	ALTER TABLE xz_point_accounts ADD COLUMN IF NOT EXISTS total_consumed bigint NOT NULL DEFAULT 0;
	ALTER TABLE xz_point_accounts ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'active';

	CREATE TABLE IF NOT EXISTS xz_wallet_ledger (
		id bigserial PRIMARY KEY,
		account_id text NOT NULL,
		wallet_key text NOT NULL UNIQUE,
		command_fingerprint text NOT NULL,
		created_at timestamptz DEFAULT now()
	);

	CREATE TABLE IF NOT EXISTS xz_personal_point_reservations (
		id text PRIMARY KEY,
		account_id text NOT NULL,
		user_id text NOT NULL,
		business_type text NOT NULL,
		business_id text NOT NULL,
		idempotency_key text NOT NULL,
		requested_points bigint NOT NULL,
		reserved_points bigint NOT NULL,
		status text NOT NULL DEFAULT 'RESERVED',
		reserved_at timestamptz DEFAULT now(),
		expires_at timestamptz,
		settled_at timestamptz
	);
	`
	if _, err := db.ExecContext(context.Background(), ddl); err != nil {
		db.Close()
		t.Fatalf("setup httpserver concurrency tables: %v", err)
	}
	return db
}

func seedQuarantinedTaskForFencing(t *testing.T, db *sql.DB, taskID, workerID string, gen int64) int64 {
	t.Helper()
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
		INSERT INTO xz_generation_tasks (id, status, task_status, execution_generation, worker_id, lease_until, type)
		VALUES ($1, 'PROCESSING', 'RUNNING', $2, $3, now() + interval '30 seconds', 'TEXT_TO_IMAGE')
		ON CONFLICT (id) DO UPDATE SET status='PROCESSING', task_status='RUNNING', execution_generation=$2, worker_id=$3
	`, taskID, gen, workerID)
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}

	var execID int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO provider_executions (task_id, provider, provider_model, capability, attempt, status, request_fingerprint, task_execution_generation)
		VALUES ($1, 'mock', 'model', 'image', 1, 'prepared', 'd1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1', $2)
		RETURNING id
	`, taskID, gen).Scan(&execID)
	if err != nil {
		t.Fatalf("seed execution: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO provider_execution_quarantine (
			execution_id, task_id, attempt, generation,
			snapshot_sha256, evidence_sha256, approval_id, release_sha,
			not_before, expires_at
		) VALUES (
			$1, $2, 1, $3,
			'0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
			'0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
			'appr-fencing', '0123456789abcdef0123456789abcdef01234567',
			now() - interval '5m', now() + interval '1h'
		)`, execID, taskID, gen)
	if err != nil {
		t.Fatalf("seed quarantine: %v", err)
	}

	return execID
}

// TestRenewGenerationLease_TimingA_QuarantinedBlocksWithErrQuarantined verifies:
// 1. renewGenerationLeaseTx locks xz_generation_tasks FOR UPDATE first.
// 2. Evaluates RejectTask under the row lock.
// 3. Since task is quarantined, returns ErrQuarantined.
// 4. Zero lease updates occur.
func TestRenewGenerationLease_TimingA_QuarantinedBlocksWithErrQuarantined(t *testing.T) {
	db := setupHTTPServerConcurrencyTestDB(t)
	defer db.Close()
	ctx := context.Background()

	taskID := fmt.Sprintf("fence-renew-%d", time.Now().UnixNano())
	workerID := "worker-fence-a"
	execID := seedQuarantinedTaskForFencing(t, db, taskID, workerID, 1)
	defer func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM provider_execution_quarantine WHERE execution_id=$1`, execID)
		_, _ = db.ExecContext(ctx, `DELETE FROM provider_executions WHERE id=$1`, execID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_generation_tasks WHERE id=$1`, taskID)
	}()

	var leaseBefore time.Time
	if err := db.QueryRowContext(ctx, `SELECT lease_until FROM xz_generation_tasks WHERE id=$1`, taskID).Scan(&leaseBefore); err != nil {
		t.Fatalf("query lease before: %v", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	err = renewGenerationLeaseTx(ctx, tx, taskID, workerID, 1, 60*time.Second)
	if !errors.Is(err, pe.ErrQuarantined) {
		t.Fatalf("expected ErrQuarantined, got %v", err)
	}
	_ = tx.Rollback()

	// Verify ZERO lease mutations
	var leaseAfter time.Time
	if err := db.QueryRowContext(ctx, `SELECT lease_until FROM xz_generation_tasks WHERE id=$1`, taskID).Scan(&leaseAfter); err != nil {
		t.Fatalf("query lease after: %v", err)
	}
	if !leaseAfter.Equal(leaseBefore) {
		t.Fatalf("lease mutated after quarantine rejection! before=%v, after=%v", leaseBefore, leaseAfter)
	}
}

// TestPersonalPointsBilling_TimingA_QuarantinedBlocksWithErrQuarantined verifies:
// reserveTx, captureTx, and releaseTx lock xz_generation_tasks FOR UPDATE first before RejectTask,
// and return ErrQuarantined with strictly 0 balance or ledger mutations.
func TestPersonalPointsBilling_TimingA_QuarantinedBlocksWithErrQuarantined(t *testing.T) {
	db := setupHTTPServerConcurrencyTestDB(t)
	defer db.Close()
	ctx := context.Background()

	taskID := fmt.Sprintf("billing-quarantine-%d", time.Now().UnixNano())
	userID := "user-" + taskID
	accountID := "acc-" + taskID
	workerID := "worker-bill"
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users (id,name,role,status) VALUES ($1,$1,'MEMBER','ACTIVE') ON CONFLICT (id) DO NOTHING`, userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	pointStore := NewPostgresPersonalPointStore(db)
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{
		AccountID: accountID, UserID: userID, Source: PointSourceRecharge,
		Points: 100, ReferenceType: "TEST_INIT", ReferenceID: "init-" + taskID,
		IdempotencyKey: "grant-init-" + taskID,
	}); err != nil {
		t.Fatalf("grant initial points: %v", err)
	}
	reserved, err := pointStore.reserve(ctx, PersonalPointReserveCommand{
		AccountID: accountID, UserID: userID, BusinessType: "IMAGE_GENERATION",
		BusinessID: taskID, QuarantineTaskID: taskID, RequestedPoints: 10,
		IdempotencyKey: "pre-quarantine-reserve:" + taskID,
	})
	if err != nil {
		t.Fatalf("seed normal reservation before quarantine: %v", err)
	}
	execID := seedQuarantinedTaskForFencing(t, db, taskID, workerID, 1)
	defer func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM provider_execution_quarantine WHERE execution_id=$1`, execID)
		_, _ = db.ExecContext(ctx, `DELETE FROM provider_executions WHERE id=$1`, execID)
		_, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE aggregate_id=$1`, taskID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_assets WHERE task_id=$1`, taskID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_generation_tasks WHERE id=$1`, taskID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_personal_point_reservation_allocations WHERE account_id=$1`, accountID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_personal_point_reservations WHERE account_id=$1`, accountID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_personal_point_lot_movements WHERE account_id=$1`, accountID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_personal_point_lots WHERE account_id=$1`, accountID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_wallet_ledger WHERE account_id=$1`, accountID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_point_accounts WHERE id=$1`, accountID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_users WHERE id=$1`, userID)
	}()

	// 1. reserveTx on quarantined task must lock task and return ErrQuarantined
	tx1, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	defer tx1.Rollback()

	_, reserveErr := pointStore.reserveTx(ctx, tx1, PersonalPointReserveCommand{
		AccountID:        accountID,
		UserID:           userID,
		BusinessType:     "IMAGE_GENERATION",
		BusinessID:       taskID,
		QuarantineTaskID: taskID,
		RequestedPoints:  10,
		IdempotencyKey:   "generation:reserve:" + taskID,
	})
	if !errors.Is(reserveErr, pe.ErrQuarantined) {
		t.Fatalf("reserveTx expected ErrQuarantined, got %v", reserveErr)
	}
	_ = tx1.Rollback()

	// 2. captureTx on quarantined task must lock task and return ErrQuarantined
	tx2, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx2: %v", err)
	}
	defer tx2.Rollback()

	_, captureErr := pointStore.captureTx(ctx, tx2, PersonalPointCaptureCommand{
		AccountID:      accountID,
		UserID:         userID,
		ReservationID:  reserved.Reservation.ID,
		Points:         10,
		IdempotencyKey: "generation:capture:" + taskID,
	})
	if !errors.Is(captureErr, pe.ErrQuarantined) {
		t.Fatalf("captureTx expected ErrQuarantined, got %v", captureErr)
	}
	_ = tx2.Rollback()

	// 3. releaseTx on quarantined task must lock task and return ErrQuarantined
	tx3, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx3: %v", err)
	}
	defer tx3.Rollback()

	_, releaseErr := pointStore.releaseTx(ctx, tx3, PersonalPointReleaseCommand{
		AccountID:      accountID,
		UserID:         userID,
		ReservationID:  reserved.Reservation.ID,
		Points:         10,
		IdempotencyKey: "generation:release:" + taskID,
	})
	if !errors.Is(releaseErr, pe.ErrQuarantined) {
		t.Fatalf("releaseTx expected ErrQuarantined, got %v", releaseErr)
	}
	_ = tx3.Rollback()

	// Assertions: points balance strictly untouched (100 available, 0 frozen)
	var avail, frozen int64
	if err := db.QueryRowContext(ctx, `SELECT available, frozen FROM xz_point_accounts WHERE id=$1`, accountID).Scan(&avail, &frozen); err != nil {
		t.Fatalf("query balance: %v", err)
	}
	if avail != 90 || frozen != 10 {
		t.Fatalf("balance mutated! available=%d frozen=%d, want 90/10", avail, frozen)
	}

	var ledgerCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_wallet_ledger WHERE account_id=$1`, accountID).Scan(&ledgerCount); err != nil {
		t.Fatalf("query ledger count: %v", err)
	}
	if ledgerCount != 2 {
		t.Fatalf("wallet ledger mutated! count=%d, want initial grant+reserve only", ledgerCount)
	}
	var reservationCount, allocationCount, movementCount int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_reservations WHERE account_id=$1`, accountID).Scan(&reservationCount)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_reservation_allocations WHERE account_id=$1`, accountID).Scan(&allocationCount)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_lot_movements WHERE account_id=$1`, accountID).Scan(&movementCount)
	if reservationCount != 1 || allocationCount != 1 || movementCount != 2 {
		t.Fatalf("ledger side effects changed reservation/allocation/movement counts=%d/%d/%d; want 1/1/2", reservationCount, allocationCount, movementCount)
	}
}
