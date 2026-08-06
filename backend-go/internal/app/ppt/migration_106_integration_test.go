package ppt

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const migration106DatabaseURLEnv = "PPT_MIGRATION_TEST_DATABASE_URL"

var migration106FixtureTables = []string{
	"migration_106_sentinel",
	"xz_billing_events",
	"xz_generation_tasks",
	"xz_ppt_tasks",
	"xz_tenants",
}

type migration106Scripts struct {
	up   string
	down string
}

type migration106LegacyTask struct {
	taskID          string
	userID          string
	clientRequestID string
	status          string
	raw             string
	createdAt       time.Time
	updatedAt       time.Time
}

func TestPPTAgentPhase1Migration106Contract(t *testing.T) {
	scripts := readMigration106Scripts(t)
	up := normalizeMigration106SQL(scripts.up)
	down := normalizeMigration106SQL(scripts.down)

	for name, script := range map[string]string{"up": up, "down": down} {
		for _, required := range []string{
			"begin;",
			"set local lock_timeout",
			"set local statement_timeout",
			"pg_advisory_xact_lock",
			"lock table public.xz_ppt_tasks in access exclusive mode",
			"commit;",
		} {
			if !strings.Contains(script, required) {
				t.Errorf("%s migration is missing %q", name, required)
			}
		}
	}

	for _, forbidden := range []string{
		"add column if not exists",
		"create index if not exists",
		"tenant_default",
		"xz_tenant_members",
		"raw->'slides'",
		"raw -> 'slides'",
	} {
		if strings.Contains(up, forbidden) {
			t.Errorf("up migration contains forbidden fallback/inference %q", forbidden)
		}
	}

	for _, required := range []string{
		"xz_billing_events",
		"metric_code = 'ppt.generations'",
		"xz_generation_tasks",
		"module_code = 'ppt_generation'",
		"type = 'ppt_generation'",
		"params->>'source_type' = 'feishu'",
		"params->>'source_task_id'",
		"idx_xz_ppt_tasks_tenant_user_client_request",
		"idx_xz_ppt_tasks_tenant_user_session",
		"idx_xz_ppt_tasks_tenant_user_stage_updated",
		"ck_xz_ppt_tasks_tenant_nonblank",
		"ck_xz_ppt_tasks_session_nonblank",
		"ck_xz_ppt_tasks_source_file_ids_array",
		"ck_xz_ppt_tasks_stage_status",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("up migration is missing contract token %q", required)
		}
	}

	for _, required := range []string{
		"uk_xz_ppt_tasks_user_client_request",
		"idx_xz_ppt_tasks_user_created",
		"idx_xz_ppt_tasks_user_status",
		"drop column tenant_id",
		"drop column session_id",
		"drop column skill_code",
		"drop column stage",
		"drop column source_file_ids",
	} {
		if !strings.Contains(down, required) {
			t.Errorf("down migration is missing legacy restoration token %q", required)
		}
	}
}

func TestPPTAgentPhase1Migration106Postgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv(migration106DatabaseURLEnv))
	if dsn == "" {
		t.Skip(migration106DatabaseURLEnv + " is not configured")
	}
	scripts := readMigration106Scripts(t)
	db := openMigration106TestDB(t, dsn)
	assertMigration106FixtureDatabaseSafe(t, db)
	dropMigration106FixtureTables(t, db)
	t.Cleanup(func() { dropMigration106FixtureTables(t, db) })

	t.Run("exact legacy to final and rerun is a no-op", func(t *testing.T) {
		resetMigration106LegacyFixture(t, db)
		insertMigration106Tenant(t, db, "tenant_a")
		insertMigration106Tenant(t, db, "tenant_b")

		success := insertMigration106LegacyTask(t, db, "ppt_legacy_success", "shared_user", "same-request", "success")
		failed := insertMigration106LegacyTask(t, db, "ppt_legacy_failed", "failed_user", "failed-request", "failed")
		cancelled := insertMigration106LegacyTask(t, db, "ppt_legacy_cancelled", "cancelled_user", "cancelled-request", "cancelled")
		insertMigration106BillingEvidence(t, db, "billing_success_1", success, "tenant_a")
		insertMigration106BillingEvidence(t, db, "billing_success_2", success, "tenant_a")
		insertMigration106FeishuEvidence(t, db, "generation_success", success, "tenant_a")
		insertMigration106BillingEvidence(t, db, "billing_failed", failed, "tenant_b")
		insertMigration106FeishuEvidence(t, db, "generation_cancelled", cancelled, "tenant_b")

		nonPPTBefore := snapshotMigration106NonPPT(t, db)
		executeMigration106SQL(t, db, scripts.up, false)
		assertMigration106FinalSchema(t, db)
		assertMigration106Projection(t, db, success, "tenant_a", StageReady)
		assertMigration106Projection(t, db, failed, "tenant_b", StageFailed)
		assertMigration106Projection(t, db, cancelled, "tenant_b", StageCancelled)
		if after := snapshotMigration106NonPPT(t, db); after != nonPPTBefore {
			t.Fatalf("up migration changed non-PPT schema/data\nbefore=%s\nafter=%s", nonPPTBefore, after)
		}

		if err := NewPostgresService(db).ensurePostgresReady(context.Background()); err != nil {
			t.Fatalf("ensurePostgresReady() on migration 106 schema error = %v", err)
		}

		beforeRerun := snapshotMigration106Database(t, db)
		executeMigration106SQL(t, db, scripts.up, false)
		if afterRerun := snapshotMigration106Database(t, db); afterRerun != beforeRerun {
			t.Fatalf("exact-final rerun was not a no-op\nbefore=%s\nafter=%s", beforeRerun, afterRerun)
		}

		assertMigration106Checks(t, db)
		assertMigration106TenantAwareUniqueness(t, db, success)
	})

	t.Run("partial 102-like schema fails atomically", func(t *testing.T) {
		resetMigration106LegacyFixture(t, db)
		if _, err := db.Exec(`
			ALTER TABLE public.xz_ppt_tasks
			  ADD COLUMN session_id VARCHAR(128),
			  ADD COLUMN skill_code VARCHAR(64) NOT NULL DEFAULT '',
			  ADD COLUMN stage VARCHAR(32) NOT NULL DEFAULT 'DRAFT',
			  ADD COLUMN source_file_ids JSONB NOT NULL DEFAULT '[]'::JSONB;
			CREATE INDEX idx_xz_ppt_tasks_user_session
			  ON public.xz_ppt_tasks(user_id, session_id) WHERE session_id IS NOT NULL;
			CREATE INDEX idx_xz_ppt_tasks_user_stage_updated
			  ON public.xz_ppt_tasks(user_id, stage, updated_at DESC);
		`); err != nil {
			t.Fatal(err)
		}
		before := snapshotMigration106Database(t, db)
		executeMigration106SQL(t, db, scripts.up, true)
		if after := snapshotMigration106Database(t, db); after != before {
			t.Fatalf("partial-schema rejection changed database\nbefore=%s\nafter=%s", before, after)
		}
	})

	for _, test := range []struct {
		name    string
		prepare func(*testing.T, *sql.DB)
	}{
		{
			name: "missing tenant evidence",
			prepare: func(t *testing.T, db *sql.DB) {
				insertMigration106Tenant(t, db, "tenant_a")
				insertMigration106LegacyTask(t, db, "ppt_missing_evidence", "missing_user", "missing-request", "success")
			},
		},
		{
			name: "conflicting tenant evidence",
			prepare: func(t *testing.T, db *sql.DB) {
				insertMigration106Tenant(t, db, "tenant_a")
				insertMigration106Tenant(t, db, "tenant_b")
				task := insertMigration106LegacyTask(t, db, "ppt_conflicting_evidence", "conflict_user", "conflict-request", "success")
				insertMigration106BillingEvidence(t, db, "billing_conflict", task, "tenant_a")
				insertMigration106FeishuEvidence(t, db, "generation_conflict", task, "tenant_b")
			},
		},
		{
			name: "pending status",
			prepare: func(t *testing.T, db *sql.DB) {
				insertMigration106Tenant(t, db, "tenant_a")
				task := insertMigration106LegacyTask(t, db, "ppt_pending", "pending_user", "pending-request", "pending")
				insertMigration106BillingEvidence(t, db, "billing_pending", task, "tenant_a")
			},
		},
		{
			name: "processing status",
			prepare: func(t *testing.T, db *sql.DB) {
				insertMigration106Tenant(t, db, "tenant_a")
				task := insertMigration106LegacyTask(t, db, "ppt_processing", "processing_user", "processing-request", "processing")
				insertMigration106BillingEvidence(t, db, "billing_processing", task, "tenant_a")
			},
		},
		{
			name: "unknown status",
			prepare: func(t *testing.T, db *sql.DB) {
				insertMigration106Tenant(t, db, "tenant_a")
				task := insertMigration106LegacyTask(t, db, "ppt_unknown", "unknown_user", "unknown-request", "completed")
				insertMigration106BillingEvidence(t, db, "billing_unknown", task, "tenant_a")
			},
		},
		{
			name: "malformed raw",
			prepare: func(t *testing.T, db *sql.DB) {
				insertMigration106Tenant(t, db, "tenant_a")
				task := insertMigration106LegacyTask(t, db, "ppt_malformed", "malformed_user", "malformed-request", "success")
				if _, err := db.Exec(`UPDATE public.xz_ppt_tasks SET raw='[]'::jsonb WHERE task_id=$1`, task.taskID); err != nil {
					t.Fatal(err)
				}
				insertMigration106BillingEvidence(t, db, "billing_malformed", task, "tenant_a")
			},
		},
		{
			name: "tenant does not exist",
			prepare: func(t *testing.T, db *sql.DB) {
				task := insertMigration106LegacyTask(t, db, "ppt_missing_tenant", "missing_tenant_user", "missing-tenant-request", "success")
				insertMigration106BillingEvidence(t, db, "billing_missing_tenant", task, "tenant_missing")
			},
		},
		{
			name: "tenant id exceeds target length",
			prepare: func(t *testing.T, db *sql.DB) {
				tenantID := strings.Repeat("t", 129)
				insertMigration106Tenant(t, db, tenantID)
				task := insertMigration106LegacyTask(t, db, "ppt_long_tenant", "long_tenant_user", "long-tenant-request", "success")
				insertMigration106BillingEvidence(t, db, "billing_long_tenant", task, tenantID)
			},
		},
	} {
		t.Run(test.name+" fails atomically", func(t *testing.T) {
			resetMigration106LegacyFixture(t, db)
			test.prepare(t, db)
			before := snapshotMigration106Database(t, db)
			executeMigration106SQL(t, db, scripts.up, true)
			if after := snapshotMigration106Database(t, db); after != before {
				t.Fatalf("rejected migration changed database\nbefore=%s\nafter=%s", before, after)
			}
		})
	}

	t.Run("blank Feishu chain is never up evidence", func(t *testing.T) {
		resetMigration106LegacyFixture(t, db)
		insertMigration106Tenant(t, db, "tenant_a")
		task := insertMigration106LegacyTask(t, db, "ppt_blank_feishu_up", "blank_feishu_user", "", "success")
		insertMigration106FeishuEvidence(t, db, "generation_blank_feishu_up", task, "tenant_a")
		before := snapshotMigration106Database(t, db)
		executeMigration106SQL(t, db, scripts.up, true)
		if after := snapshotMigration106Database(t, db); after != before {
			t.Fatalf("blank Feishu up-chain rejection changed database\nbefore=%s\nafter=%s", before, after)
		}
	})

	t.Run("blank Feishu chain cannot reprove tenant during down", func(t *testing.T) {
		resetMigration106LegacyFixture(t, db)
		insertMigration106Tenant(t, db, "tenant_a")
		task := insertMigration106LegacyTask(t, db, "ppt_blank_feishu_down", "blank_feishu_user", "", "success")
		insertMigration106BillingEvidence(t, db, "billing_blank_feishu_down", task, "tenant_a")
		executeMigration106SQL(t, db, scripts.up, false)
		if _, err := db.Exec(`DELETE FROM public.xz_billing_events WHERE id='billing_blank_feishu_down'`); err != nil {
			t.Fatal(err)
		}
		insertMigration106FeishuEvidence(t, db, "generation_blank_feishu_down", task, "tenant_a")
		before := snapshotMigration106Database(t, db)
		executeMigration106SQL(t, db, scripts.down, true)
		if after := snapshotMigration106Database(t, db); after != before {
			t.Fatalf("blank Feishu down-chain rejection changed database\nbefore=%s\nafter=%s", before, after)
		}
	})

	t.Run("down restores exact legacy before new writes", func(t *testing.T) {
		resetMigration106LegacyFixture(t, db)
		insertMigration106Tenant(t, db, "tenant_a")
		task := insertMigration106LegacyTask(t, db, "ppt_down_safe", "down_user", "down-request", "success")
		insertMigration106BillingEvidence(t, db, "billing_down_safe", task, "tenant_a")
		nonPPTBefore := snapshotMigration106NonPPT(t, db)
		executeMigration106SQL(t, db, scripts.up, false)
		executeMigration106SQL(t, db, scripts.down, false)
		assertMigration106LegacySchema(t, db)
		assertMigration106LegacyRow(t, db, task)
		if after := snapshotMigration106NonPPT(t, db); after != nonPPTBefore {
			t.Fatalf("up/down changed non-PPT schema/data\nbefore=%s\nafter=%s", nonPPTBefore, after)
		}
	})

	t.Run("down rejects after a Phase 1 write", func(t *testing.T) {
		resetMigration106LegacyFixture(t, db)
		insertMigration106Tenant(t, db, "tenant_a")
		legacy := insertMigration106LegacyTask(t, db, "ppt_down_refuse_legacy", "down_refuse_user", "down-refuse-request", "failed")
		insertMigration106BillingEvidence(t, db, "billing_down_refuse", legacy, "tenant_a")
		executeMigration106SQL(t, db, scripts.up, false)
		if _, err := db.Exec(`
			INSERT INTO public.xz_ppt_tasks(
			  task_id,tenant_id,user_id,client_request_id,status,session_id,skill_code,stage,
			  source_file_ids,created_at,updated_at,raw
			) VALUES (
			  'ppt_phase1_write','tenant_a','down_refuse_user','phase1-request','pending',
			  'ppt_phase1_write','general','DRAFT','[]'::jsonb,now(),now(),
			  '{"taskId":"ppt_phase1_write","tenantId":"tenant_a","sessionId":"ppt_phase1_write","skillCode":"general","stage":"DRAFT","status":"pending"}'::jsonb
			)
		`); err != nil {
			t.Fatal(err)
		}
		before := snapshotMigration106Database(t, db)
		executeMigration106SQL(t, db, scripts.down, true)
		if after := snapshotMigration106Database(t, db); after != before {
			t.Fatalf("rejected down migration changed database\nbefore=%s\nafter=%s", before, after)
		}
		assertMigration106FinalSchema(t, db)
	})
}

func readMigration106Scripts(t *testing.T) migration106Scripts {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve migration 106 test path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", ".."))
	read := func(path string) string {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		return string(raw)
	}
	return migration106Scripts{
		up:   read(filepath.Join(repoRoot, "database", "migrations", "106-ppt-agent-phase1.sql")),
		down: read(filepath.Join(repoRoot, "database", "rollbacks", "106-ppt-agent-phase1.down.sql")),
	}
}

func normalizeMigration106SQL(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func openMigration106TestDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	var databaseName string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if !isPPTAgentPhase1IntegrationTestDatabaseName(databaseName) {
		t.Fatalf("%s must target a dedicated PPT Agent Phase 1 database, got %q", migration106DatabaseURLEnv, databaseName)
	}
	return db
}

func assertMigration106FixtureDatabaseSafe(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`
		SELECT c.relname
		FROM pg_catalog.pg_class c
		JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname='public' AND c.relkind IN ('r','p','v','m','f')
		ORDER BY c.relname
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	allowed := make(map[string]bool, len(migration106FixtureTables))
	for _, name := range migration106FixtureTables {
		allowed[name] = true
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		if !allowed[name] {
			t.Fatalf("dedicated migration database contains non-fixture relation %q", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func dropMigration106FixtureTables(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, name := range []string{
		"xz_ppt_tasks",
		"xz_billing_events",
		"xz_generation_tasks",
		"migration_106_sentinel",
		"xz_tenants",
	} {
		if _, err := db.Exec(`DROP TABLE IF EXISTS public.` + name); err != nil {
			t.Fatalf("drop fixture table %s: %v", name, err)
		}
	}
}

func resetMigration106LegacyFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	dropMigration106FixtureTables(t, db)
	if _, err := db.Exec(`
		CREATE TABLE public.xz_tenants (
		  id TEXT PRIMARY KEY
		);
		CREATE TABLE public.xz_billing_events (
		  id TEXT PRIMARY KEY,
		  tenant_id TEXT,
		  user_id TEXT,
		  task_id TEXT,
		  metric_code TEXT
		);
		CREATE TABLE public.xz_generation_tasks (
		  id TEXT PRIMARY KEY,
		  tenant_id TEXT,
		  user_id TEXT,
		  client_request_id TEXT,
		  module_code TEXT,
		  type TEXT,
		  params JSONB NOT NULL DEFAULT '{}'::jsonb
		);
		CREATE TABLE public.migration_106_sentinel (
		  id TEXT PRIMARY KEY,
		  payload JSONB NOT NULL
		);
		INSERT INTO public.migration_106_sentinel(id,payload)
		VALUES ('sentinel','{"must":"remain unchanged"}'::jsonb);
		CREATE TABLE public.xz_ppt_tasks (
		  task_id VARCHAR(128) PRIMARY KEY,
		  user_id VARCHAR(128) NOT NULL,
		  client_request_id VARCHAR(256) NOT NULL DEFAULT '',
		  status VARCHAR(32) NOT NULL,
		  created_at TIMESTAMPTZ NOT NULL,
		  updated_at TIMESTAMPTZ NOT NULL,
		  raw JSONB NOT NULL
		);
		CREATE INDEX idx_xz_ppt_tasks_user_created
		  ON public.xz_ppt_tasks(user_id,created_at DESC);
		CREATE INDEX idx_xz_ppt_tasks_user_status
		  ON public.xz_ppt_tasks(user_id,status);
		CREATE UNIQUE INDEX uk_xz_ppt_tasks_user_client_request
		  ON public.xz_ppt_tasks(user_id,client_request_id)
		  WHERE client_request_id<>'';
	`); err != nil {
		t.Fatalf("reset migration 106 fixture: %v", err)
	}
	assertMigration106LegacySchema(t, db)
}

func insertMigration106Tenant(t *testing.T, db *sql.DB, tenantID string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO public.xz_tenants(id) VALUES($1)`, tenantID); err != nil {
		t.Fatal(err)
	}
}

func insertMigration106LegacyTask(t *testing.T, db *sql.DB, taskID, userID, clientRequestID, status string) migration106LegacyTask {
	t.Helper()
	createdAt := time.Date(2026, time.August, 1, 10, 0, 0, 123456000, time.UTC)
	updatedAt := createdAt.Add(5 * time.Minute)
	raw := fmt.Sprintf(`{"taskId":%q,"clientRequestId":%q,"status":%q,"title":"legacy"}`, taskID, clientRequestID, status)
	if _, err := db.Exec(`
		INSERT INTO public.xz_ppt_tasks(task_id,user_id,client_request_id,status,created_at,updated_at,raw)
		VALUES($1,$2,$3,$4,$5,$6,$7::jsonb)
	`, taskID, userID, clientRequestID, status, createdAt, updatedAt, raw); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT raw::text FROM public.xz_ppt_tasks WHERE task_id = $1`, taskID).Scan(&raw); err != nil {
		t.Fatalf("read canonical legacy raw for %s: %v", taskID, err)
	}
	return migration106LegacyTask{
		taskID: taskID, userID: userID, clientRequestID: clientRequestID,
		status: status, raw: raw, createdAt: createdAt, updatedAt: updatedAt,
	}
}

func insertMigration106BillingEvidence(t *testing.T, db *sql.DB, id string, task migration106LegacyTask, tenantID string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO public.xz_billing_events(id,tenant_id,user_id,task_id,metric_code)
		VALUES($1,$2,$3,$4,'ppt.generations')
	`, id, tenantID, task.userID, task.taskID); err != nil {
		t.Fatal(err)
	}
}

func insertMigration106FeishuEvidence(t *testing.T, db *sql.DB, id string, task migration106LegacyTask, tenantID string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO public.xz_generation_tasks(
		  id,tenant_id,user_id,client_request_id,module_code,type,params
		) VALUES(
		  $1,$2,$3,$4,'ppt_generation','PPT_GENERATION',
		  jsonb_build_object('source_type','feishu','source_task_id',$4::text)
		)
	`, id, tenantID, task.userID, task.clientRequestID); err != nil {
		t.Fatal(err)
	}
}

func executeMigration106SQL(t *testing.T, db *sql.DB, script string, wantError bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, execErr := conn.ExecContext(ctx, script)
	if execErr != nil {
		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, rollbackErr := conn.ExecContext(rollbackCtx, `ROLLBACK`)
		rollbackCancel()
		if rollbackErr != nil && !strings.Contains(strings.ToLower(rollbackErr.Error()), "no transaction") {
			t.Fatalf("migration failed with %v and rollback failed with %v", execErr, rollbackErr)
		}
	}
	if wantError && execErr == nil {
		t.Fatal("migration succeeded, want fail-closed error")
	}
	if !wantError && execErr != nil {
		t.Fatalf("migration error = %v", execErr)
	}
}

func assertMigration106LegacySchema(t *testing.T, db *sql.DB) {
	t.Helper()
	columns, err := readPostgresSchemaColumns(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]postgresExpectedColumn{
		"task_id":           {typeName: "character varying(128)", notNull: true},
		"user_id":           {typeName: "character varying(128)", notNull: true},
		"client_request_id": {typeName: "character varying(256)", notNull: true, requiresDefault: true, defaultExpr: "''"},
		"status":            {typeName: "character varying(32)", notNull: true},
		"created_at":        {typeName: "timestamp with time zone", notNull: true},
		"updated_at":        {typeName: "timestamp with time zone", notNull: true},
		"raw":               {typeName: "jsonb", notNull: true},
	}
	assertMigration106Columns(t, columns, expected)
	indexes, err := readPostgresSchemaIndexes(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 4 {
		t.Fatalf("legacy indexes = %#v, want exactly four", indexes)
	}
	assertMigration106Index(t, indexes, "xz_ppt_tasks_pkey", []postgresSchemaIndexKey{{Column: "task_id"}}, "", true, true)
	assertMigration106Index(t, indexes, "idx_xz_ppt_tasks_user_created", []postgresSchemaIndexKey{{Column: "user_id"}, {Column: "created_at", Descending: true}}, "", false, false)
	assertMigration106Index(t, indexes, "idx_xz_ppt_tasks_user_status", []postgresSchemaIndexKey{{Column: "user_id"}, {Column: "status"}}, "", false, false)
	assertMigration106Index(t, indexes, "uk_xz_ppt_tasks_user_client_request", []postgresSchemaIndexKey{{Column: "user_id"}, {Column: "client_request_id"}}, "client_request_id <> ''", true, true)
	assertMigration106ConstraintNames(t, db, []string{"xz_ppt_tasks_pkey"})
}

func assertMigration106FinalSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	columns, err := readPostgresSchemaColumns(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	assertMigration106Columns(t, columns, postgresReadinessColumns())
	indexes, err := readPostgresSchemaIndexes(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 4 {
		t.Fatalf("final indexes = %#v, want exactly four", indexes)
	}
	assertMigration106Index(t, indexes, "xz_ppt_tasks_pkey", []postgresSchemaIndexKey{{Column: "task_id"}}, "", true, true)
	assertMigration106Index(t, indexes, "idx_xz_ppt_tasks_tenant_user_client_request", []postgresSchemaIndexKey{{Column: "tenant_id"}, {Column: "user_id"}, {Column: "client_request_id"}}, "client_request_id <> ''", true, true)
	assertMigration106Index(t, indexes, "idx_xz_ppt_tasks_tenant_user_session", []postgresSchemaIndexKey{{Column: "tenant_id"}, {Column: "user_id"}, {Column: "session_id"}}, "session_id IS NOT NULL", false, false)
	assertMigration106Index(t, indexes, "idx_xz_ppt_tasks_tenant_user_stage_updated", []postgresSchemaIndexKey{{Column: "tenant_id"}, {Column: "user_id"}, {Column: "stage"}, {Column: "updated_at", Descending: true}}, "", false, false)
	assertMigration106ConstraintNames(t, db, []string{
		"ck_xz_ppt_tasks_session_nonblank",
		"ck_xz_ppt_tasks_source_file_ids_array",
		"ck_xz_ppt_tasks_stage_status",
		"ck_xz_ppt_tasks_tenant_nonblank",
		"xz_ppt_tasks_pkey",
	})
}

func assertMigration106Columns(t *testing.T, got map[string]postgresSchemaColumn, expected map[string]postgresExpectedColumn) {
	t.Helper()
	if len(got) != len(expected) {
		t.Fatalf("columns = %#v, want exact set %#v", got, expected)
	}
	for name, want := range expected {
		if !postgresColumnMatches(got[name], want) {
			t.Errorf("column %s = %#v, want %#v", name, got[name], want)
		}
	}
}

func assertMigration106Index(t *testing.T, indexes map[string]postgresSchemaIndex, name string, keys []postgresSchemaIndexKey, predicate string, unique, requireUnique bool) {
	t.Helper()
	index, ok := indexes[name]
	if !ok {
		t.Errorf("index %s is missing", name)
		return
	}
	if index.unique != unique || !postgresIndexMatches(index, keys, predicate, requireUnique) {
		t.Errorf("index %s = %#v, want keys=%#v predicate=%q unique=%v", name, index, keys, predicate, unique)
	}
}

func assertMigration106ConstraintNames(t *testing.T, db *sql.DB, expected []string) {
	t.Helper()
	rows, err := db.Query(`
		SELECT con.conname
		FROM pg_catalog.pg_constraint con
		WHERE con.conrelid='public.xz_ppt_tasks'::regclass
		  AND con.contype IN ('p','u','c','f','x')
		  AND con.convalidated
		ORDER BY con.conname
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	actual := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		actual = append(actual, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(expected)
	if strings.Join(actual, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("constraints = %#v, want %#v", actual, expected)
	}
}

func assertMigration106Projection(t *testing.T, db *sql.DB, legacy migration106LegacyTask, tenantID string, stage Stage) {
	t.Helper()
	var gotTenant, sessionID, skillCode, gotStage, status, raw string
	var sourceFileIDs string
	var createdAt, updatedAt time.Time
	if err := db.QueryRow(`
		SELECT tenant_id,coalesce(session_id,''),skill_code,stage,status,source_file_ids::text,
		       raw::text,created_at,updated_at
		FROM public.xz_ppt_tasks WHERE task_id=$1
	`, legacy.taskID).Scan(&gotTenant, &sessionID, &skillCode, &gotStage, &status, &sourceFileIDs, &raw, &createdAt, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if gotTenant != tenantID || sessionID != "" || skillCode != "general" || gotStage != string(stage) || status != legacy.status || sourceFileIDs != "[]" {
		t.Fatalf("projection for %s = tenant=%q session=%q skill=%q stage=%q status=%q sources=%s", legacy.taskID, gotTenant, sessionID, skillCode, gotStage, status, sourceFileIDs)
	}
	if raw != legacy.raw || !createdAt.Equal(legacy.createdAt) || !updatedAt.Equal(legacy.updatedAt) {
		t.Fatalf("migration rewrote immutable legacy fields for %s: raw=%s created=%s updated=%s", legacy.taskID, raw, createdAt, updatedAt)
	}
}

func assertMigration106LegacyRow(t *testing.T, db *sql.DB, legacy migration106LegacyTask) {
	t.Helper()
	var taskID, userID, clientRequestID, status, raw string
	var createdAt, updatedAt time.Time
	if err := db.QueryRow(`
		SELECT task_id,user_id,client_request_id,status,raw::text,created_at,updated_at
		FROM public.xz_ppt_tasks WHERE task_id=$1
	`, legacy.taskID).Scan(&taskID, &userID, &clientRequestID, &status, &raw, &createdAt, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if taskID != legacy.taskID || userID != legacy.userID || clientRequestID != legacy.clientRequestID || status != legacy.status || raw != legacy.raw || !createdAt.Equal(legacy.createdAt) || !updatedAt.Equal(legacy.updatedAt) {
		t.Fatalf("legacy row changed after up/down: task=%q user=%q request=%q status=%q raw=%s created=%s updated=%s", taskID, userID, clientRequestID, status, raw, createdAt, updatedAt)
	}
}

func assertMigration106Checks(t *testing.T, db *sql.DB) {
	t.Helper()
	valid := []struct {
		stage  string
		status string
	}{
		{stage: "DRAFT", status: "pending"},
		{stage: "OUTLINE_READY", status: "pending"},
		{stage: "GENERATING", status: "processing"},
		{stage: "READY", status: "success"},
		{stage: "FAILED", status: "failed"},
		{stage: "CANCELLED", status: "cancelled"},
	}
	for index, pair := range valid {
		taskID := fmt.Sprintf("ppt_check_valid_%d", index)
		if _, err := db.Exec(`
			INSERT INTO public.xz_ppt_tasks(
			  task_id,tenant_id,user_id,client_request_id,status,session_id,skill_code,stage,
			  source_file_ids,created_at,updated_at,raw
			) VALUES($1::varchar(128),'tenant_a','check_user','',$2::varchar(32),NULL,'general',$3::varchar(32),'[]'::jsonb,now(),now(),jsonb_build_object('taskId',$1::varchar(128),'status',$2::varchar(32)))
		`, taskID, pair.status, pair.stage); err != nil {
			t.Errorf("valid stage/status %s/%s rejected: %v", pair.stage, pair.status, err)
		}
	}

	invalidStatements := []string{
		`INSERT INTO public.xz_ppt_tasks(task_id,tenant_id,user_id,client_request_id,status,skill_code,stage,source_file_ids,created_at,updated_at,raw) VALUES('ppt_blank_tenant',' ','check_user','','success','general','READY','[]',now(),now(),'{}')`,
		`INSERT INTO public.xz_ppt_tasks(task_id,tenant_id,user_id,client_request_id,status,session_id,skill_code,stage,source_file_ids,created_at,updated_at,raw) VALUES('ppt_blank_session','tenant_a','check_user','','success',' ','general','READY','[]',now(),now(),'{}')`,
		`INSERT INTO public.xz_ppt_tasks(task_id,tenant_id,user_id,client_request_id,status,skill_code,stage,source_file_ids,created_at,updated_at,raw) VALUES('ppt_bad_sources','tenant_a','check_user','','success','general','READY','{}',now(),now(),'{}')`,
		`INSERT INTO public.xz_ppt_tasks(task_id,tenant_id,user_id,client_request_id,status,skill_code,stage,source_file_ids,created_at,updated_at,raw) VALUES('ppt_bad_mapping','tenant_a','check_user','','pending','general','READY','[]',now(),now(),'{}')`,
		`INSERT INTO public.xz_ppt_tasks(task_id,tenant_id,user_id,client_request_id,status,skill_code,stage,source_file_ids,created_at,updated_at,raw) VALUES('ppt_unknown_stage','tenant_a','check_user','','success','general','UNKNOWN','[]',now(),now(),'{}')`,
	}
	for _, statement := range invalidStatements {
		if _, err := db.Exec(statement); err == nil {
			t.Errorf("final schema accepted invalid write: %s", statement)
		}
	}
	if _, err := db.Exec(`DELETE FROM public.xz_ppt_tasks WHERE task_id LIKE 'ppt_check_valid_%'`); err != nil {
		t.Fatal(err)
	}
}

func assertMigration106TenantAwareUniqueness(t *testing.T, db *sql.DB, legacy migration106LegacyTask) {
	t.Helper()
	insert := func(taskID, tenantID string) error {
		_, err := db.Exec(`
			INSERT INTO public.xz_ppt_tasks(
			  task_id,tenant_id,user_id,client_request_id,status,skill_code,stage,
			  source_file_ids,created_at,updated_at,raw
			) VALUES($1::varchar(128),$2::varchar(128),$3::varchar(128),$4::varchar(256),'success','general','READY','[]',now(),now(),jsonb_build_object('taskId',$1::varchar(128),'status','success'))
		`, taskID, tenantID, legacy.userID, legacy.clientRequestID)
		return err
	}
	if err := insert("ppt_other_tenant_same_request", "tenant_b"); err != nil {
		t.Fatalf("tenant-aware unique index rejected distinct tenant: %v", err)
	}
	if err := insert("ppt_same_tenant_duplicate_request", "tenant_a"); err == nil {
		t.Fatal("tenant-aware unique index accepted duplicate within tenant/user")
	}
	if _, err := db.Exec(`DELETE FROM public.xz_ppt_tasks WHERE task_id='ppt_other_tenant_same_request'`); err != nil {
		t.Fatal(err)
	}
}

func snapshotMigration106Database(t *testing.T, db *sql.DB) string {
	t.Helper()
	parts := []string{snapshotMigration106Catalog(t, db, false)}
	for _, name := range migration106FixtureTables {
		parts = append(parts, name+"="+snapshotMigration106Table(t, db, name))
	}
	return strings.Join(parts, "|")
}

func snapshotMigration106NonPPT(t *testing.T, db *sql.DB) string {
	t.Helper()
	parts := []string{snapshotMigration106Catalog(t, db, true)}
	for _, name := range migration106FixtureTables {
		if name == "xz_ppt_tasks" {
			continue
		}
		parts = append(parts, name+"="+snapshotMigration106Table(t, db, name))
	}
	return strings.Join(parts, "|")
}

func snapshotMigration106Catalog(t *testing.T, db *sql.DB, excludePPT bool) string {
	t.Helper()
	filter := ""
	if excludePPT {
		filter = " AND c.relname <> 'xz_ppt_tasks'"
	}
	query := `
		WITH objects AS (
			  SELECT 'table|' || c.relname || '|' || c.relkind::text || '|' || c.relfilenode::text AS value
		  FROM pg_catalog.pg_class c
		  JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
		  WHERE n.nspname='public' AND c.relkind IN ('r','p')` + filter + `
		  UNION ALL
		  SELECT 'column|' || c.relname || '|' || a.attnum::text || '|' || a.attname || '|' ||
		         format_type(a.atttypid,a.atttypmod) || '|' || a.attnotnull::text || '|' ||
		         coalesce(pg_get_expr(d.adbin,d.adrelid),'')
		  FROM pg_catalog.pg_attribute a
		  JOIN pg_catalog.pg_class c ON c.oid=a.attrelid
		  JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
		  LEFT JOIN pg_catalog.pg_attrdef d ON d.adrelid=a.attrelid AND d.adnum=a.attnum
		  WHERE n.nspname='public' AND c.relkind IN ('r','p') AND a.attnum>0 AND NOT a.attisdropped` + filter + `
		  UNION ALL
			  SELECT 'constraint|' || c.relname || '|' || con.conname || '|' || con.contype::text || '|' ||
		         con.convalidated::text || '|' || pg_get_constraintdef(con.oid,TRUE)
		  FROM pg_catalog.pg_constraint con
		  JOIN pg_catalog.pg_class c ON c.oid=con.conrelid
		  JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
		  WHERE n.nspname='public'` + filter + `
		  UNION ALL
		  SELECT 'index|' || table_rel.relname || '|' || index_rel.relname || '|' || index_rel.relfilenode::text || '|' ||
		         i.indisvalid::text || '|' || i.indisready::text || '|' || i.indisunique::text || '|' ||
		         pg_get_indexdef(index_rel.oid)
		  FROM pg_catalog.pg_index i
		  JOIN pg_catalog.pg_class table_rel ON table_rel.oid=i.indrelid
		  JOIN pg_catalog.pg_class index_rel ON index_rel.oid=i.indexrelid
		  JOIN pg_catalog.pg_namespace n ON n.oid=table_rel.relnamespace
		  JOIN pg_catalog.pg_class c ON c.oid=table_rel.oid
		  WHERE n.nspname='public'` + filter + `
		)
		SELECT coalesce(md5(string_agg(value,E'\n' ORDER BY value)),md5('')) FROM objects
	`
	var fingerprint string
	if err := db.QueryRow(query).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

func snapshotMigration106Table(t *testing.T, db *sql.DB, table string) string {
	t.Helper()
	allowed := false
	for _, name := range migration106FixtureTables {
		if table == name {
			allowed = true
			break
		}
	}
	if !allowed {
		t.Fatalf("refuse fingerprint of unexpected table %q", table)
	}
	query := fmt.Sprintf(`
		SELECT coalesce(
		  md5(string_agg(to_jsonb(t)::text || '|xmin=' || t.xmin::text || '|ctid=' || t.ctid::text, E'\n' ORDER BY to_jsonb(t)::text)),
		  md5('')
		)
		FROM public.%s t
	`, table)
	var fingerprint string
	if err := db.QueryRow(query).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	return fingerprint
}
