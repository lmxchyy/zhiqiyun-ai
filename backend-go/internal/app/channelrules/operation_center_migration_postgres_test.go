package channelrules

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestOperationCenterMigration089StaticContract(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "database", "migrations", "089-operation-center-lifecycle-refund-saga.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(content)

	required := []string{
		"CREATE TABLE IF NOT EXISTS xz_operation_center_refund_tasks",
		"CREATE TABLE IF NOT EXISTS xz_operation_center_manual_refunds",
		"CREATE TABLE IF NOT EXISTS xz_operation_center_review_events",
		"CREATE TABLE IF NOT EXISTS xz_referral_reward_release_tasks",
		"CREATE TABLE IF NOT EXISTS xz_operation_center_state_transitions",
		"ADD COLUMN IF NOT EXISTS refund_status",
		"ADD COLUMN IF NOT EXISTS commercial_rule_set_id",
		"ADD COLUMN IF NOT EXISTS original_wallet_ledger_id",
		"ADD COLUMN IF NOT EXISTS recoverable_cents",
		"ADD COLUMN IF NOT EXISTS recoverable_cents_delta",
		"UNIQUE (idempotency_key)",
		"UNIQUE (service_order_id, refund_scope)",
		"UNIQUE (referral_reward_id)",
		"ck_xz_oc_refund_tasks_success_evidence_089",
		"provider_outcome IN ('SUCCESS', 'TEMPORARY_FAILURE', 'UNSUPPORTED', 'UNKNOWN')",
		"refund_status IN (",
		"voucher_file_hash ~ '^[0-9a-f]{64}$'",
		"attempt_count INT NOT NULL DEFAULT 0",
		"next_retry_at TIMESTAMPTZ",
		"review_idempotency_key TEXT",
		"transition_group_key TEXT NOT NULL",
		"FOREIGN KEY (current_refund_task_id)",
		"operation center refund identity is immutable",
	}
	for _, token := range required {
		if !strings.Contains(sqlText, token) {
			t.Fatalf("089 migration is missing contract token %q", token)
		}
	}

	forbidden := []string{
		"UPDATE xz_channel_rollout_configs",
		"INSERT INTO xz_channel_rollout_whitelists",
		"UPDATE xz_order_settlement_engine_decisions",
		"applyCommerceOrderFulfillmentForTx",
	}
	for _, token := range forbidden {
		if strings.Contains(sqlText, token) {
			t.Fatalf("089 migration must not change rollout or fulfillment: found %q", token)
		}
	}
}

func TestOperationCenterMigration089PostgresContract(t *testing.T) {
	db := openOperationCenterMigration089DB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	tables := []string{
		"xz_operation_center_refund_tasks",
		"xz_operation_center_manual_refunds",
		"xz_operation_center_review_events",
		"xz_referral_reward_release_tasks",
		"xz_operation_center_state_transitions",
	}
	for _, table := range tables {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT to_regclass(current_schema() || '.' || $1) IS NOT NULL`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("missing 089 table %s", table)
		}
	}

	columns := map[string][]string{
		"xz_operation_center_service_orders": {
			"refund_status", "commercial_rule_set_id", "commercial_rule_set_version",
			"review_idempotency_key", "refund_idempotency_key", "payment_channel",
			"provider_refund_no", "refund_failure_class", "refund_failure_detail",
			"refund_attempt_count", "next_refund_retry_at", "current_refund_task_id",
		},
		"xz_referral_rewards": {
			"commercial_rule_set_id", "grant_wallet_ledger_id", "release_wallet_ledger_id",
			"original_wallet_ledger_id", "reversal_wallet_ledger_id", "refund_task_id",
			"current_release_task_id", "recoverable_cents",
		},
		"xz_commission_wallet_ledger": {
			"referral_reward_id", "original_ledger_id", "refund_task_id", "recoverable_cents_delta",
		},
	}
	for table, names := range columns {
		for _, column := range names {
			var exists bool
			if err := db.QueryRowContext(ctx, `
				SELECT EXISTS (
				  SELECT 1 FROM information_schema.columns
				  WHERE table_schema=current_schema() AND table_name=$1 AND column_name=$2
				)
			`, table, column).Scan(&exists); err != nil {
				t.Fatal(err)
			}
			if !exists {
				t.Fatalf("missing 089 column %s.%s", table, column)
			}
		}
	}

	constraints := []struct {
		table string
		name  string
	}{
		{"xz_operation_center_service_orders", "ck_xz_oc_service_orders_service_status_089"},
		{"xz_operation_center_service_orders", "ck_xz_oc_service_orders_refund_status_089"},
		{"xz_operation_center_service_orders", "ck_xz_oc_service_orders_refund_success_evidence_089"},
		{"xz_operation_center_refund_tasks", "ck_xz_oc_refund_tasks_success_evidence_089"},
		{"xz_operation_center_service_orders", "fk_xz_oc_service_orders_current_refund_task_089"},
		{"xz_referral_rewards", "ck_xz_referral_rewards_recoverable_089"},
	}
	for _, item := range constraints {
		var exists bool
		if err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
			  SELECT 1 FROM pg_constraint
			  WHERE conrelid=to_regclass(current_schema() || '.' || $1) AND conname=$2
			)
		`, item.table, item.name).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("missing 089 constraint %s on %s", item.name, item.table)
		}
	}

	foreignKeys := []struct {
		table       string
		column      string
		parentTable string
	}{
		{"xz_operation_center_refund_tasks", "service_order_id", "xz_operation_center_service_orders"},
		{"xz_operation_center_refund_tasks", "payment_record_id", "xz_payment_records"},
		{"xz_operation_center_manual_refunds", "refund_task_id", "xz_operation_center_refund_tasks"},
		{"xz_operation_center_review_events", "service_order_id", "xz_operation_center_service_orders"},
		{"xz_referral_reward_release_tasks", "referral_reward_id", "xz_referral_rewards"},
		{"xz_referral_rewards", "original_wallet_ledger_id", "xz_commission_wallet_ledger"},
		{"xz_commission_wallet_ledger", "refund_task_id", "xz_operation_center_refund_tasks"},
	}
	for _, item := range foreignKeys {
		var exists bool
		if err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
			  SELECT 1
			  FROM pg_constraint c
			  JOIN unnest(c.conkey) WITH ORDINALITY key(attnum, ordinality) ON TRUE
			  JOIN pg_attribute a ON a.attrelid=c.conrelid AND a.attnum=key.attnum
			  WHERE c.contype='f'
			    AND c.conrelid=to_regclass(current_schema() || '.' || $1)
			    AND c.confrelid=to_regclass(current_schema() || '.' || $3)
			    AND a.attname=$2
			)
		`, item.table, item.column, item.parentTable).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("missing FK %s.%s -> %s", item.table, item.column, item.parentTable)
		}
	}

	indexes := []string{
		"ux_xz_oc_service_orders_review_idempotency_089",
		"ux_xz_oc_service_orders_refund_idempotency_089",
		"ux_xz_oc_refund_tasks_provider_refund_089",
		"ux_xz_oc_manual_refunds_approved_089",
		"ux_xz_oc_review_events_applied_089",
		"ux_xz_commission_wallet_ledger_refund_original_089",
	}
	for _, index := range indexes {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT to_regclass(current_schema() || '.' || $1) IS NOT NULL`, index).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("missing 089 index %s", index)
		}
	}

	var incompatibleHistoricalRows int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM xz_operation_center_service_orders
		WHERE refund_status IS NULL
		   OR refund_attempt_count < 0
		   OR relationship_snapshot IS NULL
		   OR refund_policy_snapshot IS NULL
		   OR refund_failure_detail IS NULL
	`).Scan(&incompatibleHistoricalRows); err != nil {
		t.Fatal(err)
	}
	if incompatibleHistoricalRows != 0 {
		t.Fatalf("089 left %d historical service rows without safe defaults", incompatibleHistoricalRows)
	}
}

func TestOperationCenterMigration089ConstraintBehaviorPostgres(t *testing.T) {
	db := openOperationCenterMigration089DB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		CREATE TEMP TABLE tmp_oc_refund_tasks_089
		(LIKE xz_operation_center_refund_tasks INCLUDING ALL)
		ON COMMIT DROP
	`); err != nil {
		t.Fatal(err)
	}
	insertRefund := `
		INSERT INTO tmp_oc_refund_tasks_089(
		  id,tenant_id,service_order_id,order_id,commercial_rule_set_id,
		  origin_type,refund_scope,amount_cents,currency,payment_channel,
		  refund_status,idempotency_key
		) VALUES($1,$2,$3,$4,$5,'REVIEW_REJECTION','FULL',1,'CNY','MOCK','PENDING',$6)
	`
	if _, err := tx.ExecContext(ctx, insertRefund, "refund_1", "tenant_1", "service_1", "order_1", "rule_1", "stable-key"); err != nil {
		t.Fatal(err)
	}
	assertOperationCenterMigration089ConstraintError(
		t, ctx, tx, "sp_refund_idempotency", "23505", insertRefund,
		"refund_2", "tenant_2", "service_2", "order_2", "rule_2", "stable-key",
	)

	assertOperationCenterMigration089ConstraintError(t, ctx, tx, "sp_refund_status", "23514", `
		INSERT INTO tmp_oc_refund_tasks_089(
		  id,tenant_id,service_order_id,order_id,commercial_rule_set_id,
		  origin_type,refund_scope,amount_cents,currency,payment_channel,
		  refund_status,idempotency_key
		) VALUES('refund_bad_status','tenant','service','order','rule','REVIEW_REJECTION','FULL',1,'CNY','MOCK','INVALID','bad-status-key')
	`)

	assertOperationCenterMigration089ConstraintError(t, ctx, tx, "sp_refund_success_evidence", "23514", `
		INSERT INTO tmp_oc_refund_tasks_089(
		  id,tenant_id,service_order_id,order_id,commercial_rule_set_id,
		  origin_type,refund_scope,amount_cents,currency,payment_channel,
		  refund_status,idempotency_key,prepared_at,completed_at
		) VALUES('refund_no_evidence','tenant','service','order','rule','REVIEW_REJECTION','FULL',1,'CNY','MOCK','SUCCEEDED','success-without-evidence',now(),now())
	`)

	if _, err := tx.ExecContext(ctx, `
		CREATE TEMP TABLE tmp_oc_review_events_089
		(LIKE xz_operation_center_review_events INCLUDING ALL)
		ON COMMIT DROP;
		CREATE TEMP TABLE tmp_referral_release_tasks_089
		(LIKE xz_referral_reward_release_tasks INCLUDING ALL)
		ON COMMIT DROP;
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tmp_oc_review_events_089(id,tenant_id,service_order_id,decision,reviewed_by,idempotency_key)
		VALUES('review_1','tenant','service','APPROVED','reviewer','review-key')
	`); err != nil {
		t.Fatal(err)
	}
	assertOperationCenterMigration089ConstraintError(t, ctx, tx, "sp_review_idempotency", "23505", `
		INSERT INTO tmp_oc_review_events_089(id,tenant_id,service_order_id,decision,reviewed_by,idempotency_key)
		VALUES('review_2','tenant','service_2','APPROVED','reviewer','review-key')
	`)

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tmp_referral_release_tasks_089(id,tenant_id,referral_reward_id,idempotency_key,execute_at)
		VALUES('release_1','tenant','reward','release-key',now())
	`); err != nil {
		t.Fatal(err)
	}
	assertOperationCenterMigration089ConstraintError(t, ctx, tx, "sp_release_idempotency", "23505", `
		INSERT INTO tmp_referral_release_tasks_089(id,tenant_id,referral_reward_id,idempotency_key,execute_at)
		VALUES('release_2','tenant','reward_2','release-key',now())
	`)

	assertOperationCenterMigration089ConstraintError(t, ctx, tx, "sp_refund_foreign_key", "23503", `
		INSERT INTO xz_operation_center_refund_tasks(
		  id,tenant_id,service_order_id,order_id,commercial_rule_set_id,
		  origin_type,refund_scope,amount_cents,currency,payment_channel,
		  refund_status,idempotency_key
		) VALUES('refund_fk_probe_089','missing_tenant_089','missing_service_089','missing_order_089','missing_rule_089','REVIEW_REJECTION','FULL',1,'CNY','MOCK','PENDING','fk-probe-089')
	`)
}

func openOperationCenterMigration089DB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("XIANZHI_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

func assertOperationCenterMigration089ConstraintError(
	t *testing.T,
	ctx context.Context,
	tx *sql.Tx,
	savepoint string,
	expectedSQLState string,
	statement string,
	args ...any,
) {
	t.Helper()
	if !validOperationCenterMigration089Savepoint(savepoint) {
		t.Fatalf("invalid savepoint name %q", savepoint)
	}
	if _, err := tx.ExecContext(ctx, "SAVEPOINT "+savepoint); err != nil {
		t.Fatalf("create savepoint %s: %v", savepoint, err)
	}

	_, execErr := tx.ExecContext(ctx, statement, args...)
	validationErr := validateOperationCenterMigration089SQLState(execErr, expectedSQLState)

	if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+savepoint); err != nil {
		t.Fatalf("rollback to savepoint %s after expected SQLSTATE %s: %v", savepoint, expectedSQLState, err)
	}
	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+savepoint); err != nil {
		t.Fatalf("release savepoint %s after expected SQLSTATE %s: %v", savepoint, expectedSQLState, err)
	}
	if validationErr != nil {
		t.Fatal(validationErr)
	}
}

func validateOperationCenterMigration089SQLState(err error, expected string) error {
	if err == nil {
		return fmt.Errorf("expected PostgreSQL SQLSTATE %s", expected)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return fmt.Errorf("expected PostgreSQL error %s, got %T: %v", expected, err, err)
	}
	if pgErr.Code != expected {
		return fmt.Errorf("expected PostgreSQL SQLSTATE %s, got %s: %v", expected, pgErr.Code, err)
	}
	return nil
}

func validOperationCenterMigration089Savepoint(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' {
			return false
		}
	}
	return true
}
