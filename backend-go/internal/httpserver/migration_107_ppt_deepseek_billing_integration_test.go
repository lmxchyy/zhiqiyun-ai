package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const pptDeepseekBillingTestDSNEnv = "PPT_DEEPSEEK_BILLING_TEST_DATABASE_URL"

func TestPPTDeepseekBillingMigrationFilesAndSafetyContract(t *testing.T) {
	for _, name := range []string{
		"107-ppt-deepseek-v4-flash-billing.sql",
		"../rollbacks/107-ppt-deepseek-v4-flash-billing.down.sql",
	} {
		path := pptDeepseekBillingMigrationPath(t, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration artifact %s: %v", name, err)
		}
		text := string(raw)
		for _, required := range []string{"BEGIN;", "COMMIT;", "pg_advisory_xact_lock", "lock_timeout", "statement_timeout", "IN SHARE ROW EXCLUSIVE MODE"} {
			if !strings.Contains(text, required) {
				t.Fatalf("%s must contain %q", name, required)
			}
		}
	}

	up, err := os.ReadFile(pptDeepseekBillingMigrationPath(t, "107-ppt-deepseek-v4-flash-billing.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"migration_107_ppt_deepseek_billing_backup",
		"deepseek-v4-flash", "channel_api_1785315792271635355",
		"PPT_DEEPSEEK_BILLING_PARTIAL_OR_DRIFTED_STATE",
		"PPT_DEEPSEEK_BILLING_ACTIVE_PPT_TASKS",
		"xz_billing_rule_versions", "xz_provider_costs",
	} {
		if !strings.Contains(string(up), required) {
			t.Fatalf("up migration missing %q", required)
		}
	}

	down, err := os.ReadFile(pptDeepseekBillingMigrationPath(t, "../rollbacks/107-ppt-deepseek-v4-flash-billing.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"PPT_DEEPSEEK_BILLING_DOWN_REQUIRES_EXACT_CUTOVER",
		"PPT_DEEPSEEK_BILLING_DOWN_POSTCUTOVER_USAGE",
		"PPT_DEEPSEEK_BILLING_DOWN_ACTIVE_PPT_TASKS",
	} {
		if !strings.Contains(string(down), required) {
			t.Fatalf("down migration missing %q", required)
		}
	}
}

func TestPPTDeepseekBillingTestDSNGuard(t *testing.T) {
	if value := strings.TrimSpace(os.Getenv(pptDeepseekBillingTestDSNEnv)); value == "" {
		t.Skip(pptDeepseekBillingTestDSNEnv + " is not configured")
	} else if err := validatePPTDeepseekBillingTestDSN(value); err != nil {
		t.Fatal(err)
	}
}

func TestPPTDeepseekBillingMigrationPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv(pptDeepseekBillingTestDSNEnv))
	if dsn == "" {
		t.Skip(pptDeepseekBillingTestDSNEnv + " is not configured")
	}
	if err := validatePPTDeepseekBillingTestDSN(dsn); err != nil {
		t.Fatal(err)
	}

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
	if !strings.Contains(strings.ToLower(databaseName), "ppt_deepseek_billing") {
		t.Fatalf("%s resolved to unsafe database %q", pptDeepseekBillingTestDSNEnv, databaseName)
	}

	up := readPPTDeepseekBillingMigration(t, "107-ppt-deepseek-v4-flash-billing.sql")
	down := readPPTDeepseekBillingMigration(t, "../rollbacks/107-ppt-deepseek-v4-flash-billing.down.sql")
	assertPPTDeepseekBillingFixtureDatabaseSafe(t, db)
	dropPPTDeepseekBillingFixtureTables(t, db)
	t.Cleanup(func() { dropPPTDeepseekBillingFixtureTables(t, db) })

	t.Run("up is exact idempotent and down restores preimage", func(t *testing.T) {
		resetPPTDeepseekBillingFixture(t, db)
		capabilityBefore := queryMigration107String(t, db, `SELECT raw::text || '|' || coalesce(updated_at, '<null>') FROM public.xz_system_settings WHERE id='ai_capability_config'`)
		channelBefore := queryMigration107String(t, db, `SELECT raw::text FROM public.xz_api_channels WHERE id='channel_api_1785315792271635355'`)
		kimiBefore := queryMigration107String(t, db, `SELECT to_jsonb(rule)::text FROM public.xz_billing_rule_versions rule WHERE id='brv_billing_rule_ppt_kimi_v1'`)
		nonPPTBefore := snapshotPPTDeepseekBillingNonPPT(t, db)

		executePPTDeepseekBillingMigration(t, db, up, "")
		assertPPTDeepseekBillingTarget(t, db)
		if after := snapshotPPTDeepseekBillingNonPPT(t, db); after != nonPPTBefore {
			t.Fatalf("up changed non-PPT state\nbefore=%s\nafter=%s", nonPPTBefore, after)
		}

		beforeSecondUp := snapshotPPTDeepseekBillingDatabase(t, db)
		executePPTDeepseekBillingMigration(t, db, up, "")
		if afterSecondUp := snapshotPPTDeepseekBillingDatabase(t, db); afterSecondUp != beforeSecondUp {
			t.Fatalf("second up was not a no-op\nbefore=%s\nafter=%s", beforeSecondUp, afterSecondUp)
		}

		executePPTDeepseekBillingMigration(t, db, down, "")
		if got := queryMigration107String(t, db, `SELECT raw::text || '|' || coalesce(updated_at, '<null>') FROM public.xz_system_settings WHERE id='ai_capability_config'`); got != capabilityBefore {
			t.Fatalf("capability preimage was not restored\nwant=%s\ngot=%s", capabilityBefore, got)
		}
		if got := queryMigration107String(t, db, `SELECT raw::text FROM public.xz_api_channels WHERE id='channel_api_1785315792271635355'`); got != channelBefore {
			t.Fatalf("channel preimage was not restored\nwant=%s\ngot=%s", channelBefore, got)
		}
		if got := queryMigration107String(t, db, `SELECT to_jsonb(rule)::text FROM public.xz_billing_rule_versions rule WHERE id='brv_billing_rule_ppt_kimi_v1'`); got != kimiBefore {
			t.Fatalf("Kimi V1 preimage was not restored\nwant=%s\ngot=%s", kimiBefore, got)
		}
		if after := snapshotPPTDeepseekBillingNonPPT(t, db); after != nonPPTBefore {
			t.Fatalf("up/down changed non-PPT state\nbefore=%s\nafter=%s", nonPPTBefore, after)
		}
		var targetRows int
		if err := db.QueryRow(`
			SELECT
			  (SELECT count(*) FROM public.xz_billing_rule_versions WHERE id='brv_billing_rule_ppt_deepseek_v4_flash_v1') +
			  (SELECT count(*) FROM public.xz_provider_costs WHERE id='pcost_newapi_deepseek_v4_flash_per_page_v1')
		`).Scan(&targetRows); err != nil {
			t.Fatal(err)
		}
		if targetRows != 0 {
			t.Fatalf("down left %d DeepSeek billing rows", targetRows)
		}
		var backupExists bool
		if err := db.QueryRow(`SELECT to_regclass('public.xz_migration_107_ppt_deepseek_billing_backup') IS NOT NULL`).Scan(&backupExists); err != nil {
			t.Fatal(err)
		}
		if backupExists {
			t.Fatal("down left migration 107 backup table")
		}
	})

	t.Run("active PPT task rejects up atomically", func(t *testing.T) {
		resetPPTDeepseekBillingFixture(t, db)
		if _, err := db.Exec(`INSERT INTO public.xz_ppt_tasks(task_id,status,stage,created_at) VALUES('ppt-active','pending','DRAFT',now())`); err != nil {
			t.Fatal(err)
		}
		before := snapshotPPTDeepseekBillingDatabase(t, db)
		executePPTDeepseekBillingMigration(t, db, up, "PPT_DEEPSEEK_BILLING_ACTIVE_PPT_TASKS")
		if after := snapshotPPTDeepseekBillingDatabase(t, db); after != before {
			t.Fatalf("rejected up changed database\nbefore=%s\nafter=%s", before, after)
		}
	})

	t.Run("post-cutover billing event rejects down atomically", func(t *testing.T) {
		resetPPTDeepseekBillingFixture(t, db)
		executePPTDeepseekBillingMigration(t, db, up, "")
		if _, err := db.Exec(`
			INSERT INTO public.xz_billing_events(id,model,metric_code,occurred_at)
			SELECT 'billing-after-cutover','deepseek-v4-flash','ppt.generations',(cutover_at + interval '1 second')::text
			FROM public.xz_migration_107_ppt_deepseek_billing_backup WHERE id='cutover'
		`); err != nil {
			t.Fatal(err)
		}
		before := snapshotPPTDeepseekBillingDatabase(t, db)
		executePPTDeepseekBillingMigration(t, db, down, "PPT_DEEPSEEK_BILLING_DOWN_POSTCUTOVER_USAGE")
		if after := snapshotPPTDeepseekBillingDatabase(t, db); after != before {
			t.Fatalf("rejected down changed database\nbefore=%s\nafter=%s", before, after)
		}
	})

	t.Run("published Kimi drift rejects second up atomically", func(t *testing.T) {
		resetPPTDeepseekBillingFixture(t, db)
		executePPTDeepseekBillingMigration(t, db, up, "")
		if _, err := db.Exec(`
			UPDATE public.xz_billing_rule_versions
			SET status='PUBLISHED'
			WHERE id='brv_billing_rule_ppt_kimi_v1'
		`); err != nil {
			t.Fatal(err)
		}
		before := snapshotPPTDeepseekBillingDatabase(t, db)
		executePPTDeepseekBillingMigration(t, db, up, "PPT_DEEPSEEK_BILLING_PARTIAL_OR_DRIFTED_STATE")
		if after := snapshotPPTDeepseekBillingDatabase(t, db); after != before {
			t.Fatalf("rejected second up changed database\nbefore=%s\nafter=%s", before, after)
		}
	})

	t.Run("missing PPT limit allowed models rejects up atomically", func(t *testing.T) {
		resetPPTDeepseekBillingFixture(t, db)
		if _, err := db.Exec(`
			UPDATE public.xz_system_settings
			SET raw = raw #- '{tenantModuleLimits,1,limit_json,models,allowed}'
			WHERE id='ai_capability_config'
		`); err != nil {
			t.Fatal(err)
		}
		before := snapshotPPTDeepseekBillingDatabase(t, db)
		executePPTDeepseekBillingMigration(t, db, up, "PPT_DEEPSEEK_BILLING_UNEXPECTED_PPT_LIMITS")
		if after := snapshotPPTDeepseekBillingDatabase(t, db); after != before {
			t.Fatalf("rejected up changed database\nbefore=%s\nafter=%s", before, after)
		}
	})

	for _, testCase := range []struct {
		name      string
		mutation  string
		wantError string
	}{
		{
			name: "duplicate DeepSeek model",
			mutation: `
				UPDATE public.xz_system_settings settings
				SET raw = jsonb_set(
				  settings.raw,
				  '{aiModels}',
				  settings.raw->'aiModels' || (
				    SELECT item FROM jsonb_array_elements(settings.raw->'aiModels') item
				    WHERE item->>'model_name'='deepseek-v4-flash' LIMIT 1
				  )
				)
				WHERE id='ai_capability_config'
			`,
			wantError: "PPT_DEEPSEEK_BILLING_UNEXPECTED_PPT_MODELS",
		},
		{
			name: "duplicate DeepSeek schema",
			mutation: `
				UPDATE public.xz_system_settings settings
				SET raw = jsonb_set(
				  settings.raw,
				  '{aiParameterSchemas}',
				  settings.raw->'aiParameterSchemas' || (
				    SELECT item FROM jsonb_array_elements(settings.raw->'aiParameterSchemas') item
				    WHERE item->>'model_name'='deepseek-v4-flash' LIMIT 1
				  )
				)
				WHERE id='ai_capability_config'
			`,
			wantError: "PPT_DEEPSEEK_BILLING_UNEXPECTED_PPT_SCHEMAS",
		},
	} {
		t.Run(testCase.name+" rejects up atomically", func(t *testing.T) {
			resetPPTDeepseekBillingFixture(t, db)
			if _, err := db.Exec(testCase.mutation); err != nil {
				t.Fatal(err)
			}
			before := snapshotPPTDeepseekBillingDatabase(t, db)
			executePPTDeepseekBillingMigration(t, db, up, testCase.wantError)
			if after := snapshotPPTDeepseekBillingDatabase(t, db); after != before {
				t.Fatalf("rejected up changed database\nbefore=%s\nafter=%s", before, after)
			}
		})
	}

	t.Run("active PPT task rejects down atomically", func(t *testing.T) {
		resetPPTDeepseekBillingFixture(t, db)
		executePPTDeepseekBillingMigration(t, db, up, "")
		if _, err := db.Exec(`INSERT INTO public.xz_ppt_tasks(task_id,status,stage,created_at) VALUES('ppt-active-after-cutover','processing','GENERATING',now())`); err != nil {
			t.Fatal(err)
		}
		before := snapshotPPTDeepseekBillingDatabase(t, db)
		executePPTDeepseekBillingMigration(t, db, down, "PPT_DEEPSEEK_BILLING_DOWN_ACTIVE_PPT_TASKS")
		if after := snapshotPPTDeepseekBillingDatabase(t, db); after != before {
			t.Fatalf("rejected down changed database\nbefore=%s\nafter=%s", before, after)
		}
	})

	t.Run("post-cutover DeepSeek generation task rejects down atomically", func(t *testing.T) {
		resetPPTDeepseekBillingFixture(t, db)
		executePPTDeepseekBillingMigration(t, db, up, "")
		if _, err := db.Exec(`
			INSERT INTO public.xz_generation_tasks(id,model,created_at)
			SELECT 'generation-after-cutover','deepseek-v4-flash',(cutover_at + interval '1 second')::text
			FROM public.xz_migration_107_ppt_deepseek_billing_backup WHERE id='cutover'
		`); err != nil {
			t.Fatal(err)
		}
		before := snapshotPPTDeepseekBillingDatabase(t, db)
		executePPTDeepseekBillingMigration(t, db, down, "PPT_DEEPSEEK_BILLING_DOWN_POSTCUTOVER_USAGE")
		if after := snapshotPPTDeepseekBillingDatabase(t, db); after != before {
			t.Fatalf("rejected down changed database\nbefore=%s\nafter=%s", before, after)
		}
	})

	t.Run("missing legacy PPT default schema rejects up atomically", func(t *testing.T) {
		resetPPTDeepseekBillingFixture(t, db)
		if _, err := db.Exec(`
			UPDATE public.xz_system_settings
			SET raw = raw #- '{aiModules,1,default_schema_id}'
			WHERE id='ai_capability_config'
		`); err != nil {
			t.Fatal(err)
		}
		before := snapshotPPTDeepseekBillingDatabase(t, db)
		executePPTDeepseekBillingMigration(t, db, up, "PPT_DEEPSEEK_BILLING_UNEXPECTED_PPT_MODULE")
		if after := snapshotPPTDeepseekBillingDatabase(t, db); after != before {
			t.Fatalf("rejected up changed database\nbefore=%s\nafter=%s", before, after)
		}
	})

	t.Run("Kimi immutable tenant scope drift rejects second up and down atomically", func(t *testing.T) {
		resetPPTDeepseekBillingFixture(t, db)
		executePPTDeepseekBillingMigration(t, db, up, "")
		if _, err := db.Exec(`
			UPDATE public.xz_billing_rule_versions
			SET tenant_id='tenant_drift'
			WHERE id='brv_billing_rule_ppt_kimi_v1'
		`); err != nil {
			t.Fatal(err)
		}

		beforeSecondUp := snapshotPPTDeepseekBillingDatabase(t, db)
		executePPTDeepseekBillingMigration(t, db, up, "PPT_DEEPSEEK_BILLING_PARTIAL_OR_DRIFTED_STATE")
		if afterSecondUp := snapshotPPTDeepseekBillingDatabase(t, db); afterSecondUp != beforeSecondUp {
			t.Fatalf("rejected second up changed database\nbefore=%s\nafter=%s", beforeSecondUp, afterSecondUp)
		}

		beforeDown := snapshotPPTDeepseekBillingDatabase(t, db)
		executePPTDeepseekBillingMigration(t, db, down, "PPT_DEEPSEEK_BILLING_DOWN_REQUIRES_EXACT_CUTOVER")
		if afterDown := snapshotPPTDeepseekBillingDatabase(t, db); afterDown != beforeDown {
			t.Fatalf("rejected down changed database\nbefore=%s\nafter=%s", beforeDown, afterDown)
		}
	})

	for _, testCase := range []struct {
		name     string
		mutation string
	}{
		{
			name: "extra DeepSeek billing rule",
			mutation: `
				INSERT INTO public.xz_billing_rule_versions(
				 id,rule_key,legacy_rule_id,model_name,model_code,module_code,billing_unit,
				 base_price_points,minimum_charge_points,parameter_rules,rule_source,version,status,
				 effective_from,effective_to,validation_result,created_by,created_at,updated_at,published_at
				)
				SELECT 'brv_extra_deepseek','billing_rule_extra_deepseek','billing_rule_extra_deepseek',
				 model_name,model_code,module_code,billing_unit,base_price_points,minimum_charge_points,
				 parameter_rules,rule_source,1,'ARCHIVED',effective_from,effective_to,validation_result,
				 'drift',created_at,updated_at,published_at
				FROM public.xz_billing_rule_versions
				WHERE id='brv_billing_rule_ppt_deepseek_v4_flash_v1'
			`,
		},
		{
			name: "extra DeepSeek provider cost",
			mutation: `
				INSERT INTO public.xz_provider_costs(
				 id,provider,channel,platform_model_code,upstream_model_name,billing_unit,
				 parameter_range,unit_cost,currency,effective_from,effective_to,status,created_at,updated_at
				)
				SELECT 'pcost_extra_deepseek',provider,channel,platform_model_code,upstream_model_name,
				 billing_unit,parameter_range,unit_cost,currency,effective_from,effective_to,'INACTIVE',created_at,updated_at
				FROM public.xz_provider_costs
				WHERE id='pcost_newapi_deepseek_v4_flash_per_page_v1'
			`,
		},
	} {
		t.Run(testCase.name+" rejects second up and down atomically", func(t *testing.T) {
			resetPPTDeepseekBillingFixture(t, db)
			executePPTDeepseekBillingMigration(t, db, up, "")
			if _, err := db.Exec(testCase.mutation); err != nil {
				t.Fatal(err)
			}

			beforeSecondUp := snapshotPPTDeepseekBillingDatabase(t, db)
			executePPTDeepseekBillingMigration(t, db, up, "PPT_DEEPSEEK_BILLING_PARTIAL_OR_DRIFTED_STATE")
			if afterSecondUp := snapshotPPTDeepseekBillingDatabase(t, db); afterSecondUp != beforeSecondUp {
				t.Fatalf("rejected second up changed database\nbefore=%s\nafter=%s", beforeSecondUp, afterSecondUp)
			}

			beforeDown := snapshotPPTDeepseekBillingDatabase(t, db)
			executePPTDeepseekBillingMigration(t, db, down, "PPT_DEEPSEEK_BILLING_DOWN_REQUIRES_EXACT_CUTOVER")
			if afterDown := snapshotPPTDeepseekBillingDatabase(t, db); afterDown != beforeDown {
				t.Fatalf("rejected down changed database\nbefore=%s\nafter=%s", beforeDown, afterDown)
			}
		})
	}

	t.Run("migration table lock mode blocks concurrent billing inserts", func(t *testing.T) {
		resetPPTDeepseekBillingFixture(t, db)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		locker, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer locker.Close()
		lockerTx, err := locker.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = lockerTx.Rollback() }()
		if _, err := lockerTx.ExecContext(ctx, `
			LOCK TABLE public.xz_billing_rule_versions, public.xz_provider_costs
			IN SHARE ROW EXCLUSIVE MODE
		`); err != nil {
			t.Fatal(err)
		}

		for _, insert := range []struct {
			name  string
			query string
		}{
			{
				name: "billing rule insert",
				query: `
					INSERT INTO public.xz_billing_rule_versions(
					 id,rule_key,model_name,model_code,module_code,billing_unit,base_price_points,
					 minimum_charge_points,parameter_rules,rule_source,version,status,
					 validation_result,created_at,updated_at
					) VALUES (
					 'blocked_rule','blocked_rule','Blocked','blocked','ppt_generation','PER_PAGE',1,
					 1,'{}','DATABASE',1,'DRAFT','{"valid":false,"issues":[]}',now(),now()
					)
				`,
			},
			{
				name: "provider cost insert",
				query: `
					INSERT INTO public.xz_provider_costs(
					 id,provider,channel,platform_model_code,upstream_model_name,billing_unit,
					 parameter_range,unit_cost,currency,effective_from,status,created_at,updated_at
					) VALUES (
					 'blocked_cost','NEWAPI','channel_other','blocked','blocked','PER_PAGE',
					 '{}',0.01,'CNY',now(),'ACTIVE',now(),now()
					)
				`,
			},
		} {
			t.Run(insert.name, func(t *testing.T) {
				writer, err := db.Conn(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer writer.Close()
				writerTx, err := writer.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := writerTx.ExecContext(ctx, `SET LOCAL lock_timeout='250ms'`); err != nil {
					_ = writerTx.Rollback()
					t.Fatal(err)
				}
				_, insertErr := writerTx.ExecContext(ctx, insert.query)
				_ = writerTx.Rollback()
				if insertErr == nil {
					t.Fatal("concurrent insert succeeded while SHARE ROW EXCLUSIVE lock was held")
				}
				if message := strings.ToLower(insertErr.Error()); !strings.Contains(message, "lock timeout") && !strings.Contains(message, "55p03") {
					t.Fatalf("concurrent insert error = %v, want lock timeout", insertErr)
				}
			})
		}
		if err := lockerTx.Rollback(); err != nil {
			t.Fatal(err)
		}
	})
}

func validatePPTDeepseekBillingTestDSN(dsn string) error {
	parsed, err := url.Parse(strings.TrimSpace(dsn))
	if err != nil {
		return err
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return fmt.Errorf("PPT DeepSeek billing tests require a PostgreSQL DSN")
	}
	if !strings.Contains(strings.ToLower(strings.Trim(parsed.Path, "/")), "ppt_deepseek_billing") {
		return fmt.Errorf("PPT DeepSeek billing tests refuse database %q: database name must contain ppt_deepseek_billing", parsed.Path)
	}
	return nil
}

func pptDeepseekBillingMigrationPath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "database", "migrations", name))
}

func readPPTDeepseekBillingMigration(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(pptDeepseekBillingMigrationPath(t, name))
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	return string(raw)
}

var pptDeepseekBillingFixtureTables = []string{
	"xz_migration_107_ppt_deepseek_billing_backup",
	"xz_billing_events",
	"xz_generation_tasks",
	"xz_ppt_tasks",
	"xz_provider_costs",
	"xz_billing_rule_versions",
	"xz_api_channels",
	"xz_system_settings",
}

func assertPPTDeepseekBillingFixtureDatabaseSafe(t *testing.T, db *sql.DB) {
	t.Helper()
	allowed := make(map[string]bool, len(pptDeepseekBillingFixtureTables))
	for _, name := range pptDeepseekBillingFixtureTables {
		allowed[name] = true
	}
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

func dropPPTDeepseekBillingFixtureTables(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, name := range pptDeepseekBillingFixtureTables {
		if _, err := db.Exec(`DROP TABLE IF EXISTS public.` + name); err != nil {
			t.Fatalf("drop fixture table %s: %v", name, err)
		}
	}
}

const pptDeepseekBillingCapabilityFixture = `{
  "aiModules": [
    {"id":"ai_module_image_generation","module_code":"image_generation","name":"image sentinel","bound_models":["gpt-image-2"],"default_schema_id":"schema_image","config":{"must":"remain"}},
    {"id":"ai_module_ppt_generation","module_code":"ppt_generation","name":"PPT","bound_models":["deepseek-v4-flash","kimi-k2.6","ppt-text-model"],"default_schema_id":"schema_ppt_kimi","config":{"preserve":"module"}}
  ],
  "aiModels": [
    {"id":"ai_model_gpt_image_2","model_name":"gpt-image-2","module_code":"image_generation","provider":"NewAPI","status":"ACTIVE","sentinel":{"must":"remain"}},
    {"id":"ai_model_kimi_k26","model_name":"kimi-k2.6","module_code":"ppt_generation","provider":"Moonshot","status":"ACTIVE","fallback_model":"ppt-text-model","allow_fallback_switch":true},
    {"id":"ai_model_ppt_text","model_name":"ppt-text-model","module_code":"ppt_generation","provider":"Local","status":"ACTIVE","fallback_model":"","allow_fallback_switch":false},
    {"id":"ai_model_deepseek_old","model_name":"deepseek-v4-flash","module_code":"ppt_generation","provider":"NewAPI","channel_id":"channel_runtime_env","status":"ACTIVE","fallback_model":"kimi-k2.6","allow_fallback_switch":true,"capability_code":["ppt_outline","ppt_content"]}
  ],
  "aiParameterSchemas": [
    {"id":"schema_image","module_code":"image_generation","model_name":"gpt-image-2","schema_json":{"fields":[{"key":"prompt","required":true}]},"status":"ACTIVE","sentinel":"keep"},
    {"id":"schema_ppt_kimi","module_code":"ppt_generation","model_name":"kimi-k2.6","schema_json":{"fields":[{"key":"topic","required":true},{"key":"page_count","required":true,"max":20}]},"status":"ACTIVE"},
    {"id":"schema_ppt_deepseek_old","module_code":"ppt_generation","model_name":"deepseek-v4-flash","schema_json":{"fields":[{"key":"topic","required":true},{"key":"page_count","required":true,"max":20},{"key":"with_images"},{"key":"uploaded_file"}]},"status":"ACTIVE","sentinel":"deepseek-schema-content"}
  ],
  "tenantModuleLimits": [
    {"id":"limit_image","tenant_id":"default","module_code":"image_generation","limit_json":{"models":{"allowed":["gpt-image-2"]},"n":{"max":4}},"status":"ACTIVE","sentinel":"keep"},
    {"id":"limit_ppt_default","tenant_id":"default","module_code":"ppt_generation","limit_json":{"models":{"allowed":["deepseek-v4-flash","kimi-k2.6","ppt-text-model"]},"page_count":{"max":20}},"status":"ACTIVE"},
    {"id":"limit_ppt_free","tenant_id":"default","package_id":"plan_free","module_code":"ppt_generation","limit_json":{"models":{"allowed":["kimi-k2.6","ppt-text-model","deepseek-v4-flash"]},"page_count":{"max":5}},"status":"ACTIVE"},
    {"id":"limit_ppt_month","tenant_id":"default","package_id":"plan_month","module_code":"ppt_generation","limit_json":{"models":{"allowed":["ppt-text-model","deepseek-v4-flash","kimi-k2.6"]},"page_count":{"max":10}},"status":"ACTIVE"},
    {"id":"limit_ppt_pro","tenant_id":"default","package_id":"plan_pro","module_code":"ppt_generation","limit_json":{"models":{"allowed":["deepseek-v4-flash","kimi-k2.6","ppt-text-model"]},"page_count":{"max":20}},"status":"ACTIVE"},
    {"id":"limit_ppt_year","tenant_id":"default","package_id":"plan_year","module_code":"ppt_generation","limit_json":{"models":{"allowed":["deepseek-v4-flash","ppt-text-model","kimi-k2.6"]},"page_count":{"max":20}},"status":"ACTIVE"}
  ],
  "billingRules": [
    {"id":"billing_rule_image_gpt","module_code":"image_generation","model_name":"gpt-image-2","billing_type":"per_image","base_price":10,"cost_price":6,"currency_type":"credit","parameter_multiplier":{"quality":{"standard":1}},"status":"ACTIVE","sentinel":"keep"},
    {"id":"billing_rule_ppt_kimi","module_code":"ppt_generation","model_name":"kimi-k2.6","billing_type":"per_page","base_price":1,"cost_price":0,"currency_type":"credit","parameter_multiplier":{"with_images":{"true":1,"false":1},"uploaded_file":{"true":1,"false":1}},"status":"ACTIVE","sentinel":"preserve-on-rewrite"}
  ],
  "unrelatedRoot": {"must":"remain unchanged"}
}`

func resetPPTDeepseekBillingFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	dropPPTDeepseekBillingFixtureTables(t, db)
	if _, err := db.Exec(`
		CREATE TABLE public.xz_system_settings (
		  id TEXT PRIMARY KEY, raw JSONB NOT NULL, updated_at TEXT
		);
		CREATE TABLE public.xz_api_channels (
		  id TEXT PRIMARY KEY, name TEXT, base_url TEXT, protocol TEXT, status TEXT,
		  priority INT NOT NULL DEFAULT 0, raw JSONB NOT NULL
		);
		CREATE TABLE public.xz_billing_rule_versions (
		  id TEXT PRIMARY KEY, rule_key TEXT NOT NULL, legacy_rule_id TEXT,
		  model_name TEXT NOT NULL, model_code TEXT NOT NULL, module_code TEXT NOT NULL,
		  billing_unit TEXT NOT NULL, base_price_points NUMERIC(18,6) NOT NULL,
		  minimum_charge_points NUMERIC(18,6) NOT NULL DEFAULT 0,
		  parameter_rules JSONB NOT NULL DEFAULT '{}'::jsonb, rule_source TEXT NOT NULL,
		  tenant_id TEXT, plan_id TEXT, version INT NOT NULL, status TEXT NOT NULL,
		  effective_from TIMESTAMPTZ, effective_to TIMESTAMPTZ,
		  validation_result JSONB NOT NULL, created_by TEXT,
		  created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL,
		  published_at TIMESTAMPTZ, UNIQUE(rule_key,version)
		);
		CREATE TABLE public.xz_provider_costs (
		  id TEXT PRIMARY KEY, provider TEXT NOT NULL, channel TEXT NOT NULL,
		  platform_model_code TEXT NOT NULL, upstream_model_name TEXT NOT NULL,
		  billing_unit TEXT NOT NULL, parameter_range JSONB NOT NULL,
		  unit_cost NUMERIC(18,6) NOT NULL, currency TEXT NOT NULL,
		  effective_from TIMESTAMPTZ NOT NULL, effective_to TIMESTAMPTZ,
		  status TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL
		);
		CREATE TABLE public.xz_ppt_tasks (
		  task_id TEXT PRIMARY KEY, status TEXT, stage TEXT, created_at TIMESTAMPTZ NOT NULL
		);
		CREATE TABLE public.xz_generation_tasks (
		  id TEXT PRIMARY KEY, model TEXT, created_at TEXT
		);
		CREATE TABLE public.xz_billing_events (
		  id TEXT PRIMARY KEY, model TEXT, metric_code TEXT, occurred_at TEXT
		);
	`); err != nil {
		t.Fatalf("create migration 107 fixture tables: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO public.xz_system_settings(id,raw,updated_at) VALUES('ai_capability_config',$1::jsonb,'2026-08-01T00:00:00Z')`, pptDeepseekBillingCapabilityFixture); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO public.xz_api_channels(id,name,base_url,protocol,status,priority,raw) VALUES
		('channel_api_1785315792271635355','canonical NewAPI','https://newapi.zs-kjhn.cn','openai','ACTIVE',10,
		 '{"id":"channel_api_1785315792271635355","name":"canonical NewAPI","baseUrl":"https://newapi.zs-kjhn.cn","protocol":"openai","status":"ACTIVE","models":["gpt-image-2","kimi-k2.6"],"apiKeyEnv":"NEWAPI_API_KEY","sentinel":{"must":"remain"}}'),
		('channel_other','other','https://other.invalid','openai','ACTIVE',20,
		 '{"id":"channel_other","baseUrl":"https://other.invalid","protocol":"openai","status":"ACTIVE","models":["other-model"],"sentinel":"keep"}');

		INSERT INTO public.xz_billing_rule_versions(
		 id,rule_key,legacy_rule_id,model_name,model_code,module_code,billing_unit,
		 base_price_points,minimum_charge_points,parameter_rules,rule_source,version,status,
		 effective_from,effective_to,validation_result,created_by,created_at,updated_at,published_at
		) VALUES
		('brv_billing_rule_ppt_kimi_v1','billing_rule_ppt_kimi','billing_rule_ppt_kimi','Kimi K2.6','kimi-k2.6','ppt_generation','PER_PAGE',1,1,
		 '{"with_images":{"true":1,"false":1},"uploaded_file":{"true":1,"false":1}}','CODE_DEFAULT',1,'PUBLISHED',
		 '2026-01-01T00:00:00Z',NULL,'{"valid":true,"issues":[]}',NULL,'2026-01-01T00:00:00Z','2026-01-02T00:00:00Z','2026-01-02T00:00:00Z'),
		('brv_image_sentinel_v1','image_sentinel','image_sentinel','Image Sentinel','gpt-image-2','image_generation','PER_IMAGE',10,1,
		 '{"quality":{"standard":1}}','DATABASE',1,'PUBLISHED','2026-01-01T00:00:00Z',NULL,'{"valid":true,"issues":[]}',
		 'fixture','2026-01-01T00:00:00Z','2026-01-03T00:00:00Z','2026-01-03T00:00:00Z');

		INSERT INTO public.xz_provider_costs(
		 id,provider,channel,platform_model_code,upstream_model_name,billing_unit,
		 parameter_range,unit_cost,currency,effective_from,effective_to,status,created_at,updated_at
		) VALUES ('pcost_image_sentinel','OPENAI','channel_other','gpt-image-2','gpt-image-2','PER_IMAGE',
		 '{"quality":["standard"]}',0.60,'CNY','2026-01-01T00:00:00Z',NULL,'ACTIVE','2026-01-01T00:00:00Z','2026-01-03T00:00:00Z');
	`); err != nil {
		t.Fatalf("seed migration 107 fixture: %v", err)
	}
}

func executePPTDeepseekBillingMigration(t *testing.T, db *sql.DB, script, wantError string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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
		if rollbackErr != nil {
			t.Fatalf("migration error %v; rollback error: %v", execErr, rollbackErr)
		}
	}
	if wantError == "" {
		if execErr != nil {
			t.Fatalf("migration failed: %v", execErr)
		}
		return
	}
	if execErr == nil {
		t.Fatalf("migration succeeded, want error containing %q", wantError)
	}
	if !strings.Contains(execErr.Error(), wantError) {
		t.Fatalf("migration error = %v, want %q", execErr, wantError)
	}
}

func queryMigration107String(t *testing.T, db *sql.DB, query string) string {
	t.Helper()
	var value string
	if err := db.QueryRow(query).Scan(&value); err != nil {
		t.Fatalf("query migration 107 snapshot: %v", err)
	}
	return value
}

func snapshotPPTDeepseekBillingNonPPT(t *testing.T, db *sql.DB) string {
	t.Helper()
	capability := queryMigration107String(t, db, `
		WITH cfg AS (
		  SELECT raw FROM public.xz_system_settings WHERE id='ai_capability_config'
		)
		SELECT jsonb_build_object(
		  'root', raw - ARRAY['aiModules','aiModels','aiParameterSchemas','tenantModuleLimits','billingRules'],
		  'modules', (SELECT coalesce(jsonb_agg(item ORDER BY ord),'[]'::jsonb) FROM jsonb_array_elements(raw->'aiModules') WITH ORDINALITY source(item,ord) WHERE coalesce(item->>'module_code',item->>'moduleCode')<>'ppt_generation'),
		  'models', (SELECT coalesce(jsonb_agg(item ORDER BY ord),'[]'::jsonb) FROM jsonb_array_elements(raw->'aiModels') WITH ORDINALITY source(item,ord) WHERE coalesce(item->>'module_code',item->>'moduleCode')<>'ppt_generation'),
		  'schemas', (SELECT coalesce(jsonb_agg(item ORDER BY ord),'[]'::jsonb) FROM jsonb_array_elements(raw->'aiParameterSchemas') WITH ORDINALITY source(item,ord) WHERE coalesce(item->>'module_code',item->>'moduleCode')<>'ppt_generation'),
		  'limits', (SELECT coalesce(jsonb_agg(item ORDER BY ord),'[]'::jsonb) FROM jsonb_array_elements(raw->'tenantModuleLimits') WITH ORDINALITY source(item,ord) WHERE coalesce(item->>'module_code',item->>'moduleCode')<>'ppt_generation'),
		  'rules', (SELECT coalesce(jsonb_agg(item ORDER BY ord),'[]'::jsonb) FROM jsonb_array_elements(raw->'billingRules') WITH ORDINALITY source(item,ord) WHERE coalesce(item->>'module_code',item->>'moduleCode')<>'ppt_generation')
		)::text FROM cfg
	`)
	channels := queryMigration107String(t, db, `SELECT coalesce(jsonb_agg(to_jsonb(channel) ORDER BY id),'[]'::jsonb)::text FROM public.xz_api_channels channel WHERE id<>'channel_api_1785315792271635355'`)
	rules := queryMigration107String(t, db, `SELECT coalesce(jsonb_agg(to_jsonb(rule) ORDER BY id),'[]'::jsonb)::text FROM public.xz_billing_rule_versions rule WHERE module_code<>'ppt_generation'`)
	costs := queryMigration107String(t, db, `SELECT coalesce(jsonb_agg(to_jsonb(cost) ORDER BY id),'[]'::jsonb)::text FROM public.xz_provider_costs cost WHERE platform_model_code<>'deepseek-v4-flash'`)
	return strings.Join([]string{capability, channels, rules, costs}, "|")
}

func snapshotPPTDeepseekBillingDatabase(t *testing.T, db *sql.DB) string {
	t.Helper()
	queries := []string{
		`SELECT coalesce(jsonb_agg(to_jsonb(row) ORDER BY id),'[]'::jsonb)::text FROM public.xz_system_settings row`,
		`SELECT coalesce(jsonb_agg(to_jsonb(row) ORDER BY id),'[]'::jsonb)::text FROM public.xz_api_channels row`,
		`SELECT coalesce(jsonb_agg(to_jsonb(row) ORDER BY id),'[]'::jsonb)::text FROM public.xz_billing_rule_versions row`,
		`SELECT coalesce(jsonb_agg(to_jsonb(row) ORDER BY id),'[]'::jsonb)::text FROM public.xz_provider_costs row`,
		`SELECT coalesce(jsonb_agg(to_jsonb(row) ORDER BY task_id),'[]'::jsonb)::text FROM public.xz_ppt_tasks row`,
		`SELECT coalesce(jsonb_agg(to_jsonb(row) ORDER BY id),'[]'::jsonb)::text FROM public.xz_generation_tasks row`,
		`SELECT coalesce(jsonb_agg(to_jsonb(row) ORDER BY id),'[]'::jsonb)::text FROM public.xz_billing_events row`,
	}
	parts := make([]string, 0, len(queries)+1)
	for _, query := range queries {
		parts = append(parts, queryMigration107String(t, db, query))
	}
	var backupExists bool
	if err := db.QueryRow(`SELECT to_regclass('public.xz_migration_107_ppt_deepseek_billing_backup') IS NOT NULL`).Scan(&backupExists); err != nil {
		t.Fatal(err)
	}
	if backupExists {
		parts = append(parts, queryMigration107String(t, db, `SELECT coalesce(jsonb_agg(to_jsonb(row) ORDER BY id),'[]'::jsonb)::text FROM public.xz_migration_107_ppt_deepseek_billing_backup row`))
	} else {
		parts = append(parts, "<no-backup>")
	}
	return strings.Join(parts, "|")
}

func assertPPTDeepseekBillingTarget(t *testing.T, db *sql.DB) {
	t.Helper()
	var capabilityOK bool
	if err := db.QueryRow(`
		WITH cfg AS (SELECT raw FROM public.xz_system_settings WHERE id='ai_capability_config')
		SELECT
		  (SELECT count(*)=1 AND bool_and(item->'bound_models'='["deepseek-v4-flash"]'::jsonb AND item->>'default_schema_id'='schema_ppt_generation_default')
		   FROM cfg, jsonb_array_elements(raw->'aiModules') item WHERE coalesce(item->>'module_code',item->>'moduleCode')='ppt_generation')
		  AND
		  (SELECT count(*)=1 AND bool_and(item->>'id'='ai_model_deepseek_v4_flash' AND item->>'model_name'='deepseek-v4-flash' AND item->>'provider'='NewAPI' AND item->>'channel_id'='channel_api_1785315792271635355' AND item->>'fallback_model'='' AND item->'allow_fallback_switch'='false'::jsonb)
		   FROM cfg, jsonb_array_elements(raw->'aiModels') item WHERE coalesce(item->>'module_code',item->>'moduleCode')='ppt_generation')
		  AND
		  (SELECT count(*)=1 AND bool_and(item->>'id'='schema_ppt_generation_default' AND item->>'model_name'='deepseek-v4-flash' AND EXISTS(SELECT 1 FROM jsonb_array_elements(item->'schema_json'->'fields') field WHERE field->>'key'='topic') AND EXISTS(SELECT 1 FROM jsonb_array_elements(item->'schema_json'->'fields') field WHERE field->>'key'='page_count'))
		   FROM cfg, jsonb_array_elements(raw->'aiParameterSchemas') item WHERE coalesce(item->>'module_code',item->>'moduleCode')='ppt_generation')
		  AND
		  (SELECT count(*)=5 AND bool_and(item->'limit_json'->'models'->'allowed'='["deepseek-v4-flash"]'::jsonb)
		   FROM cfg, jsonb_array_elements(raw->'tenantModuleLimits') item WHERE coalesce(item->>'module_code',item->>'moduleCode')='ppt_generation')
		  AND
		  (SELECT count(*)=1 AND bool_and(item->>'id'='billing_rule_ppt_deepseek_v4_flash' AND item->>'model_name'='deepseek-v4-flash' AND item->>'billing_type'='per_page' AND (item->>'base_price')::numeric=3 AND (item->>'minimum_charge')::numeric=3 AND (item->>'cost_price')::numeric=2 AND item->>'status'='ACTIVE')
		   FROM cfg, jsonb_array_elements(raw->'billingRules') item WHERE coalesce(item->>'module_code',item->>'moduleCode')='ppt_generation')
	`).Scan(&capabilityOK); err != nil {
		t.Fatal(err)
	}
	if !capabilityOK {
		t.Fatal("capability JSON did not reach the exact DeepSeek PPT target")
	}

	var ruleOK, costOK, channelOK, kimiArchived bool
	if err := db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM public.xz_billing_rule_versions
		 WHERE id='brv_billing_rule_ppt_deepseek_v4_flash_v1' AND rule_source='DATABASE' AND version=1
		   AND model_code='deepseek-v4-flash' AND module_code='ppt_generation' AND billing_unit='PER_PAGE'
		   AND base_price_points=3 AND minimum_charge_points=3 AND status='PUBLISHED'
		   AND validation_result='{"valid":true,"issues":[]}'::jsonb),
		 EXISTS(SELECT 1 FROM public.xz_provider_costs
		 WHERE id='pcost_newapi_deepseek_v4_flash_per_page_v1' AND provider='NEWAPI'
		   AND channel='channel_api_1785315792271635355' AND platform_model_code='deepseek-v4-flash'
		   AND upstream_model_name='deepseek-v4-flash-free' AND billing_unit='PER_PAGE'
		   AND parameter_range='{}'::jsonb AND unit_cost=0.02 AND currency='CNY' AND status='ACTIVE'),
		 EXISTS(SELECT 1 FROM public.xz_api_channels
		 WHERE id='channel_api_1785315792271635355'
		   AND (SELECT count(*) FROM jsonb_array_elements_text(raw->'models') model WHERE model='deepseek-v4-flash')=1),
		 EXISTS(SELECT 1 FROM public.xz_billing_rule_versions
		 WHERE id='brv_billing_rule_ppt_kimi_v1' AND status='ARCHIVED' AND effective_to IS NOT NULL)
	`).Scan(&ruleOK, &costOK, &channelOK, &kimiArchived); err != nil {
		t.Fatal(err)
	}
	if !ruleOK || !costOK || !channelOK || !kimiArchived {
		t.Fatalf("target rows invalid: rule=%t cost=%t channel=%t kimiArchived=%t", ruleOK, costOK, channelOK, kimiArchived)
	}
	var backupCount int
	if err := db.QueryRow(`SELECT count(*) FROM public.xz_migration_107_ppt_deepseek_billing_backup`).Scan(&backupCount); err != nil {
		t.Fatal(err)
	}
	if backupCount != 1 {
		t.Fatalf("migration backup rows = %d, want 1", backupCount)
	}
}
