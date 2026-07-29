package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const phase2ETestDSN = "postgres://codex:codex@127.0.0.1:55441/xianzhi_test?sslmode=disable"

func TestPhase2ETestDSNGuardIsExact(t *testing.T) {
	if err := validatePhase2ETestDSN(phase2ETestDSN); err != nil {
		t.Fatalf("approved isolated DSN rejected: %v", err)
	}
	for _, dsn := range []string{
		"postgresql://codex:codex@127.0.0.1:55441/xianzhi_test?sslmode=disable",
		"postgres://other:codex@127.0.0.1:55441/xianzhi_test?sslmode=disable",
		"postgres://codex:other@127.0.0.1:55441/xianzhi_test?sslmode=disable",
		"postgres://codex:codex@localhost:55441/xianzhi_test?sslmode=disable",
		"postgres://codex:codex@127.0.0.1:54321/xianzhi_test?sslmode=disable",
		"postgres://codex:codex@127.0.0.1:55441/postgres?sslmode=disable",
		"postgres://codex:codex@127.0.0.1:55441/xianzhi_test?sslmode=require",
		"postgres://codex:codex@127.0.0.1:55441/xianzhi_test?sslmode=disable&application_name=extra",
		"postgres://codex:codex@127.0.0.1:55441/xianzhi_test?sslmode=disable&sslmode=disable",
		"postgres://codex:codex@127.0.0.1:55441/xianzhi_test?sslmode=disable#unexpected-fragment",
	} {
		if err := validatePhase2ETestDSN(dsn); err == nil {
			t.Fatalf("unsafe DSN accepted: %s", dsn)
		}
	}
}

func validatePhase2ETestDSN(dsn string) error {
	parsed, err := url.Parse(strings.TrimSpace(dsn))
	if err != nil {
		return err
	}
	password, hasPassword := parsed.User.Password()
	if parsed.Scheme != "postgres" || parsed.User.Username() != "codex" || !hasPassword || password != "codex" ||
		parsed.Hostname() != "127.0.0.1" || parsed.Port() != "55441" || parsed.Path != "/xianzhi_test" ||
		parsed.RawQuery != "sslmode=disable" || parsed.Fragment != "" {
		return fmt.Errorf("Phase 2E PostgreSQL tests refuse non-isolated DSN %q", dsn)
	}
	return nil
}

func TestPricePlanPhase2EMigration100NumberIsUnique(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	matches, err := filepath.Glob(filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "database", "migrations", "100-*.sql")))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || filepath.Base(matches[0]) != "100-price-plan-test-whitelist-audit.sql" {
		t.Fatalf("migration 100 must be unique and named 100-price-plan-test-whitelist-audit.sql, got %v", matches)
	}
}

func TestPricePlanPhase2EMigration100PostgresGovernance(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)

	t.Run("composite ownership catalog definition is exact", func(t *testing.T) {
		var indexTable, indexMethod, indexColumns string
		var indexUnique, indexValid, indexReady, indexHasPredicate, indexHasExpressions bool
		var indexKeyAttributes, indexAttributes int
		if err := db.QueryRowContext(ctx, `
			select indexed_table.relname,index_method.amname,index_item.indisunique,index_item.indisvalid,index_item.indisready,
			       index_item.indnkeyatts,index_item.indnatts,index_item.indpred is not null,index_item.indexprs is not null,
			       coalesce((
			         select string_agg(attribute.attname,',' order by key_item.ordinality)
			         from unnest(index_item.indkey) with ordinality as key_item(attnum,ordinality)
			         join pg_attribute attribute
			           on attribute.attrelid=index_item.indrelid and attribute.attnum=key_item.attnum
			       ),'')
			from pg_class index_relation
			join pg_namespace index_namespace on index_namespace.oid=index_relation.relnamespace
			join pg_index index_item on index_item.indexrelid=index_relation.oid
			join pg_class indexed_table on indexed_table.oid=index_item.indrelid
			join pg_am index_method on index_method.oid=index_relation.relam
			where index_namespace.nspname=current_schema()
			  and index_relation.relname='ux_xz_price_plan_whitelist_identity_100'
		`).Scan(&indexTable, &indexMethod, &indexUnique, &indexValid, &indexReady,
			&indexKeyAttributes, &indexAttributes, &indexHasPredicate, &indexHasExpressions, &indexColumns); err != nil {
			t.Fatal(err)
		}
		if indexTable != "xz_price_plan_user_whitelist" || indexMethod != "btree" || !indexUnique || !indexValid || !indexReady ||
			indexKeyAttributes != 3 || indexAttributes != 3 || indexHasPredicate || indexHasExpressions ||
			indexColumns != "id,price_plan_id,user_id" {
			t.Fatalf("ownership index definition table=%s method=%s unique=%t valid=%t ready=%t keys=%d attrs=%d predicate=%t expressions=%t columns=%s",
				indexTable, indexMethod, indexUnique, indexValid, indexReady, indexKeyAttributes, indexAttributes,
				indexHasPredicate, indexHasExpressions, indexColumns)
		}

		var foreignKeyValidated bool
		var foreignKeyUpdateAction, foreignKeyDeleteAction, foreignKeyColumns, referencedColumns string
		if err := db.QueryRowContext(ctx, `
			select constraint_item.convalidated,constraint_item.confupdtype::text,constraint_item.confdeltype::text,
			       (select string_agg(attribute.attname,',' order by key_item.ordinality)
			        from unnest(constraint_item.conkey) with ordinality as key_item(attnum,ordinality)
			        join pg_attribute attribute
			          on attribute.attrelid=constraint_item.conrelid and attribute.attnum=key_item.attnum),
			       (select string_agg(attribute.attname,',' order by key_item.ordinality)
			        from unnest(constraint_item.confkey) with ordinality as key_item(attnum,ordinality)
			        join pg_attribute attribute
			          on attribute.attrelid=constraint_item.confrelid and attribute.attnum=key_item.attnum)
			from pg_constraint constraint_item
			where constraint_item.conrelid='xz_order_price_quotes'::regclass
			  and constraint_item.conname='fk_xz_order_price_quotes_whitelist_100'
			  and constraint_item.contype='f'
		`).Scan(&foreignKeyValidated, &foreignKeyUpdateAction, &foreignKeyDeleteAction, &foreignKeyColumns, &referencedColumns); err != nil {
			t.Fatal(err)
		}
		if foreignKeyValidated || foreignKeyUpdateAction != "a" || foreignKeyDeleteAction != "a" ||
			foreignKeyColumns != "whitelist_entry_id,price_plan_id,user_id" || referencedColumns != "id,price_plan_id,user_id" {
			t.Fatalf("ownership FK definition validated=%t update=%s delete=%s columns=%s references=%s",
				foreignKeyValidated, foreignKeyUpdateAction, foreignKeyDeleteAction, foreignKeyColumns, referencedColumns)
		}
	})

	t.Run("migration rejects a drifted same-name ownership index", func(t *testing.T) {
		drifts := []struct {
			name      string
			createSQL string
		}{
			{
				name:      "non-unique index",
				createSQL: `create index ux_xz_price_plan_whitelist_identity_100 on xz_price_plan_user_whitelist(id,price_plan_id,user_id)`,
			},
			{
				name:      "wrong column order",
				createSQL: `create unique index ux_xz_price_plan_whitelist_identity_100 on xz_price_plan_user_whitelist(price_plan_id,id,user_id)`,
			},
		}
		for _, drift := range drifts {
			t.Run(drift.name, func(t *testing.T) {
				tx, err := db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				if _, err := tx.ExecContext(ctx, `alter table xz_order_price_quotes drop constraint fk_xz_order_price_quotes_whitelist_100`); err != nil {
					t.Fatal(err)
				}
				if _, err := tx.ExecContext(ctx, `drop index ux_xz_price_plan_whitelist_identity_100`); err != nil {
					t.Fatal(err)
				}
				if _, err := tx.ExecContext(ctx, drift.createSQL); err != nil {
					t.Fatal(err)
				}
				_, err = tx.ExecContext(ctx, phase2EMigration100TransactionBody(t))
				if err == nil || !strings.Contains(err.Error(), "MIGRATION_100_WHITELIST_IDENTITY_INDEX_DRIFT") {
					t.Fatalf("migration drift error=%v", err)
				}
			})
		}
	})

	var columns int
	if err := db.QueryRowContext(ctx, `
		select count(*) from information_schema.columns
		where (table_name='xz_price_plan_user_whitelist' and column_name='lifecycle_status')
		   or (table_name='xz_order_price_quotes' and column_name in('whitelist_entry_id','whitelist_revision','whitelist_checked_at'))
		   or (table_name='xz_audit_logs' and column_name in(
		      'request_id','domain','result','error_code','change_reason','before_snapshot','after_snapshot',
		      'revision_before','revision_after','plan_id','plan_version_id','price_plan_id','wechat_good_id',
		      'payment_binding_id','whitelist_entry_id','environment'))
	`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 20 {
		t.Fatalf("migration 100 governance columns=%d want=20", columns)
	}

	permissions := []string{
		"pricing:plan:view", "pricing:entitlement:manage", "pricing:price-plan:manage",
		"pricing:price-plan:default", "pricing:wechat-good:manage",
		"pricing:test-whitelist:manage", "pricing:audit:view",
	}
	var superAdminCount, ordinaryCount int
	if err := db.QueryRowContext(ctx, `select count(*) from xz_role_permissions where role='SUPER_ADMIN' and permission=any($1)`, permissions).Scan(&superAdminCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `select count(*) from xz_role_permissions where role<>'SUPER_ADMIN' and permission=any($1)`, permissions).Scan(&ordinaryCount); err != nil {
		t.Fatal(err)
	}
	if superAdminCount != len(permissions) || ordinaryCount != 0 {
		t.Fatalf("pricing permissions superAdmin=%d ordinary=%d", superAdminCount, ordinaryCount)
	}

	fixture := insertPhase2EPricePlanFixture(t, ctx, db, "TEST")
	runSQLTx := func(t *testing.T, fn func(*sql.Tx)) {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		fn(tx)
	}

	t.Run("stored lifecycle remains compatible and only one ACTIVE row exists", func(t *testing.T) {
		runSQLTx(t, func(tx *sql.Tx) {
			id := "whitelist_legacy_insert_" + fixture.suffix
			if _, err := tx.ExecContext(ctx, `
				insert into xz_price_plan_user_whitelist(
					id,price_plan_id,user_id,enabled,reason,created_by,updated_by
				) values($1,$2,$3,false,'legacy compatibility',$4,$4)
			`, id, fixture.pricePlanID, fixture.userID, fixture.actorID); err != nil {
				t.Fatalf("legacy insert without lifecycle failed: %v", err)
			}
			var lifecycle string
			if err := tx.QueryRowContext(ctx, `select lifecycle_status from xz_price_plan_user_whitelist where id=$1`, id).Scan(&lifecycle); err != nil {
				t.Fatal(err)
			}
			if lifecycle != "DISABLED" {
				t.Fatalf("legacy lifecycle=%q want DISABLED", lifecycle)
			}
		})

		for _, legacy := range []struct {
			name, wantLifecycle string
			enabled             bool
		}{
			{name: "active", enabled: true, wantLifecycle: "ACTIVE"},
			{name: "disabled", enabled: false, wantLifecycle: "DISABLED"},
		} {
			t.Run("legacy NULL update normalizes "+legacy.name, func(t *testing.T) {
				runSQLTx(t, func(tx *sql.Tx) {
					id := "whitelist_legacy_update_" + legacy.name + "_" + fixture.suffix
					if _, err := tx.ExecContext(ctx, `alter table xz_price_plan_user_whitelist disable trigger trg_xz_price_plan_whitelist_guard_100`); err != nil {
						t.Fatal(err)
					}
					if _, err := tx.ExecContext(ctx, `
						insert into xz_price_plan_user_whitelist(
							id,price_plan_id,user_id,enabled,lifecycle_status,reason,created_by,updated_by
						) values($1,$2,$3,$4,null,'pre-100 row',$5,$5)
					`, id, fixture.pricePlanID, fixture.userID, legacy.enabled, fixture.actorID); err != nil {
						t.Fatal(err)
					}
					if _, err := tx.ExecContext(ctx, `alter table xz_price_plan_user_whitelist enable trigger trg_xz_price_plan_whitelist_guard_100`); err != nil {
						t.Fatal(err)
					}
					if _, err := tx.ExecContext(ctx, `update xz_price_plan_user_whitelist set reason='legacy maintenance' where id=$1`, id); err != nil {
						t.Fatalf("legacy NULL update failed: %v", err)
					}
					var lifecycle string
					if err := tx.QueryRowContext(ctx, `select lifecycle_status from xz_price_plan_user_whitelist where id=$1`, id).Scan(&lifecycle); err != nil {
						t.Fatal(err)
					}
					if lifecycle != legacy.wantLifecycle {
						t.Fatalf("legacy update lifecycle=%q want=%q", lifecycle, legacy.wantLifecycle)
					}
					if !legacy.enabled {
						if _, err := tx.ExecContext(ctx, `update xz_price_plan_user_whitelist set enabled=true,lifecycle_status='ACTIVE' where id=$1`, id); err == nil || !strings.Contains(err.Error(), "WHITELIST_TERMINAL_IMMUTABLE") {
							t.Fatalf("normalized terminal restore error=%v", err)
						}
					}
				})
			})
		}

		runSQLTx(t, func(tx *sql.Tx) {
			firstID := "whitelist_first_" + fixture.suffix
			if _, err := tx.ExecContext(ctx, `
				insert into xz_price_plan_user_whitelist(
					id,price_plan_id,user_id,enabled,lifecycle_status,effective_at,expires_at,reason,created_by,updated_by
				) values($1,$2,$3,true,'ACTIVE',now()-interval '1 minute',now()+interval '1 hour','first test',$4,$4)
			`, firstID, fixture.pricePlanID, fixture.userID, fixture.actorID); err != nil {
				t.Fatal(err)
			}
			_, err := tx.ExecContext(ctx, `
				insert into xz_price_plan_user_whitelist(
					id,price_plan_id,user_id,enabled,lifecycle_status,reason,created_by,updated_by
				) values($1,$2,$3,true,'ACTIVE','duplicate test',$4,$4)
			`, "whitelist_duplicate_"+fixture.suffix, fixture.pricePlanID, fixture.userID, fixture.actorID)
			requirePostgresCode(t, err, "23505")
		})

		runSQLTx(t, func(tx *sql.Tx) {
			id := "whitelist_terminal_" + fixture.suffix
			if _, err := tx.ExecContext(ctx, `
				insert into xz_price_plan_user_whitelist(
					id,price_plan_id,user_id,enabled,lifecycle_status,reason,created_by,updated_by
				) values($1,$2,$3,false,'DISABLED','terminal test',$4,$4)
			`, id, fixture.pricePlanID, fixture.userID, fixture.actorID); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(ctx, `update xz_price_plan_user_whitelist set enabled=true,lifecycle_status='ACTIVE' where id=$1`, id); err == nil || !strings.Contains(err.Error(), "WHITELIST_TERMINAL_IMMUTABLE") {
				t.Fatalf("terminal restore error=%v", err)
			}
		})

		runSQLTx(t, func(tx *sql.Tx) {
			id := "whitelist_delete_" + fixture.suffix
			if _, err := tx.ExecContext(ctx, `
				insert into xz_price_plan_user_whitelist(
					id,price_plan_id,user_id,enabled,lifecycle_status,reason,created_by,updated_by
				) values($1,$2,$3,false,'EXPIRED','delete guard',$4,$4)
			`, id, fixture.pricePlanID, fixture.userID, fixture.actorID); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(ctx, `delete from xz_price_plan_user_whitelist where id=$1`, id); err == nil || !strings.Contains(err.Error(), "WHITELIST_DELETE_FORBIDDEN") {
				t.Fatalf("delete error=%v", err)
			}
		})

		runSQLTx(t, func(tx *sql.Tx) {
			_, err := tx.ExecContext(ctx, `
				insert into xz_price_plan_user_whitelist(
					id,price_plan_id,user_id,enabled,lifecycle_status,reason,created_by,updated_by
				) values($1,$2,$3,false,'ACTIVE','inconsistent test',$4,$4)
			`, "whitelist_inconsistent_"+fixture.suffix, fixture.pricePlanID, fixture.userID, fixture.actorID)
			requirePostgresCode(t, err, "23514")
		})
	})

	t.Run("quote whitelist pins are complete immutable and insert-gated", func(t *testing.T) {
		runSQLTx(t, func(tx *sql.Tx) {
			whitelistID := "whitelist_quote_pin_" + fixture.suffix
			if _, err := tx.ExecContext(ctx, `
				insert into xz_price_plan_user_whitelist(
					id,price_plan_id,user_id,enabled,lifecycle_status,effective_at,expires_at,reason,created_by,updated_by
				) values($1,$2,$3,true,'ACTIVE',now()-interval '1 minute',now()+interval '1 hour','quote pin',$4,$4)
			`, whitelistID, fixture.pricePlanID, fixture.userID, fixture.actorID); err != nil {
				t.Fatal(err)
			}

			insertQuote := func(id, entryType string, whitelistEntry any, whitelistRevision any, whitelistCheckedAt any) error {
				_, err := tx.ExecContext(ctx, `
				insert into xz_order_price_quotes(
					id,quote_token_hash,tenant_id,user_id,plan_id,plan_version_id,price_plan_id,payment_binding_id,wechat_good_id,
					entry_type,transaction_price_cents,provider_price_snapshot_cents,wechat_goods_price_cents,channel,environment,
					offer_id,wechat_product_id,payment_mode,rights_snapshot,expires_at,
					whitelist_entry_id,whitelist_revision,whitelist_checked_at
				) values($1,$1,'tenant_default',$2,$3,$4,$5,$6,$7,$8,100,100,100,'WECHAT_VIRTUAL','SANDBOX',
					'offer',$7,'short_series_goods','{}'::jsonb,now()+interval '5 minutes',$9,$10,$11)
			`, id, fixture.userID, fixture.planID, fixture.versionID, fixture.pricePlanID, fixture.bindingID, fixture.goodID,
					entryType, whitelistEntry, whitelistRevision, whitelistCheckedAt)
				return err
			}
			expectRejected := func(name string, operation func() error, message string) {
				t.Helper()
				if _, err := tx.ExecContext(ctx, `savepoint quote_pin_rejection`); err != nil {
					t.Fatal(err)
				}
				err := operation()
				if err == nil || (message != "" && !strings.Contains(err.Error(), message)) {
					t.Fatalf("%s error=%v", name, err)
				}
				if _, rollbackErr := tx.ExecContext(ctx, `rollback to savepoint quote_pin_rejection`); rollbackErr != nil {
					t.Fatal(rollbackErr)
				}
				if _, err := tx.ExecContext(ctx, `release savepoint quote_pin_rejection`); err != nil {
					t.Fatal(err)
				}
			}

			expectRejected("partial whitelist pin", func() error {
				return insertQuote("quote_partial_pin_"+fixture.suffix, "TEST", whitelistID, nil, time.Now().UTC())
			}, "")
			expectRejected("new unpinned TEST quote", func() error {
				return insertQuote("quote_test_unpinned_"+fixture.suffix, "TEST", nil, nil, nil)
			}, "PRICE_QUOTE_TEST_WHITELIST_PIN_REQUIRED")
			expectRejected("pinned non-TEST quote", func() error {
				return insertQuote("quote_public_pinned_"+fixture.suffix, "PUBLIC", whitelistID, int64(1), time.Now().UTC())
			}, "PRICE_QUOTE_NON_TEST_WHITELIST_PIN_FORBIDDEN")

			validID := "quote_test_pinned_" + fixture.suffix
			if err := insertQuote(validID, "TEST", whitelistID, int64(1), time.Now().UTC()); err != nil {
				t.Fatalf("complete TEST quote pin was rejected: %v", err)
			}
			expectRejected("quote whitelist pin mutation", func() error {
				_, err := tx.ExecContext(ctx, `update xz_order_price_quotes set whitelist_revision=2 where id=$1`, validID)
				return err
			}, "PRICE_QUOTE_WHITELIST_PIN_IMMUTABLE")

			legacyID := "quote_legacy_unpinned_" + fixture.suffix
			if _, err := tx.ExecContext(ctx, `alter table xz_order_price_quotes disable trigger trg_xz_order_price_quotes_whitelist_pin_100`); err != nil {
				t.Fatal(err)
			}
			if err := insertQuote(legacyID, "TEST", nil, nil, nil); err != nil {
				t.Fatalf("historical unpinned TEST fixture insert failed: %v", err)
			}
			if _, err := tx.ExecContext(ctx, `alter table xz_order_price_quotes enable trigger trg_xz_order_price_quotes_whitelist_pin_100`); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(ctx, `update xz_order_price_quotes set status='EXPIRED' where id=$1`, legacyID); err != nil {
				t.Fatalf("legacy unpinned TEST lifecycle update failed: %v", err)
			}
		})
	})

	t.Run("quote whitelist pins enforce composite ownership", func(t *testing.T) {
		otherFixture := insertPhase2EPricePlanFixture(t, ctx, db, "TEST")
		tests := []struct {
			name         string
			idSuffix     string
			quoteOwnerID string
			quotePlan    phase2EFixture
		}{
			{
				name:         "rejects a whitelist entry owned by another user",
				idSuffix:     "wrong_user",
				quoteOwnerID: otherFixture.userID,
				quotePlan:    fixture,
			},
			{
				name:         "rejects a whitelist entry owned by another price plan",
				idSuffix:     "wrong_price_plan",
				quoteOwnerID: fixture.userID,
				quotePlan:    otherFixture,
			},
		}
		for _, test := range tests {
			t.Run(test.name+" on insert", func(t *testing.T) {
				runSQLTx(t, func(tx *sql.Tx) {
					whitelistID := "whitelist_quote_owner_insert_" + test.idSuffix + "_" + fixture.suffix
					if _, err := tx.ExecContext(ctx, `
						insert into xz_price_plan_user_whitelist(
							id,price_plan_id,user_id,enabled,lifecycle_status,effective_at,expires_at,reason,created_by,updated_by
						) values($1,$2,$3,true,'ACTIVE',now()-interval '1 minute',now()+interval '1 hour','ownership gate',$4,$4)
					`, whitelistID, fixture.pricePlanID, fixture.userID, fixture.actorID); err != nil {
						t.Fatal(err)
					}

					_, err := tx.ExecContext(ctx, `
						insert into xz_order_price_quotes(
							id,quote_token_hash,tenant_id,user_id,plan_id,plan_version_id,price_plan_id,payment_binding_id,wechat_good_id,
							entry_type,transaction_price_cents,provider_price_snapshot_cents,wechat_goods_price_cents,channel,environment,
							offer_id,wechat_product_id,payment_mode,rights_snapshot,expires_at,
							whitelist_entry_id,whitelist_revision,whitelist_checked_at
						) values($1,$1,'tenant_default',$2,$3,$4,$5,$6,$7,'TEST',100,100,100,'WECHAT_VIRTUAL','SANDBOX',
							'offer',$7,'short_series_goods','{}'::jsonb,now()+interval '5 minutes',$8,1,now())
					`, "quote_wrong_whitelist_owner_insert_"+test.idSuffix+"_"+fixture.suffix,
						test.quoteOwnerID, test.quotePlan.planID, test.quotePlan.versionID, test.quotePlan.pricePlanID,
						test.quotePlan.bindingID, test.quotePlan.goodID, whitelistID)
					requirePostgresCode(t, err, "23503")
				})
			})

			t.Run(test.name+" on update", func(t *testing.T) {
				runSQLTx(t, func(tx *sql.Tx) {
					whitelistID := "whitelist_quote_owner_update_" + test.idSuffix + "_" + fixture.suffix
					if _, err := tx.ExecContext(ctx, `
						insert into xz_price_plan_user_whitelist(
							id,price_plan_id,user_id,enabled,lifecycle_status,effective_at,expires_at,reason,created_by,updated_by
						) values($1,$2,$3,true,'ACTIVE',now()-interval '1 minute',now()+interval '1 hour','ownership gate',$4,$4)
					`, whitelistID, fixture.pricePlanID, fixture.userID, fixture.actorID); err != nil {
						t.Fatal(err)
					}

					quoteID := "quote_wrong_whitelist_owner_update_" + test.idSuffix + "_" + fixture.suffix
					if _, err := tx.ExecContext(ctx, `
						insert into xz_order_price_quotes(
							id,quote_token_hash,tenant_id,user_id,plan_id,plan_version_id,price_plan_id,payment_binding_id,wechat_good_id,
							entry_type,transaction_price_cents,provider_price_snapshot_cents,wechat_goods_price_cents,channel,environment,
							offer_id,wechat_product_id,payment_mode,rights_snapshot,expires_at,
							whitelist_entry_id,whitelist_revision,whitelist_checked_at
						) values($1,$1,'tenant_default',$2,$3,$4,$5,$6,$7,'TEST',100,100,100,'WECHAT_VIRTUAL','SANDBOX',
							'offer',$7,'short_series_goods','{}'::jsonb,now()+interval '5 minutes',$8,1,now())
					`, quoteID, fixture.userID, fixture.planID, fixture.versionID, fixture.pricePlanID,
						fixture.bindingID, fixture.goodID, whitelistID); err != nil {
						t.Fatal(err)
					}

					var err error
					if test.quoteOwnerID != fixture.userID {
						_, err = tx.ExecContext(ctx, `update xz_order_price_quotes set user_id=$2 where id=$1`, quoteID, test.quoteOwnerID)
					} else {
						_, err = tx.ExecContext(ctx, `update xz_order_price_quotes set price_plan_id=$2 where id=$1`, quoteID, test.quotePlan.pricePlanID)
					}
					requirePostgresCode(t, err, "23503")
				})
			})
		}
	})

	t.Run("structured pricing audits are immutable while legacy audits remain compatible", func(t *testing.T) {
		runSQLTx(t, func(tx *sql.Tx) {
			id := "audit_pricing_guard_" + fixture.suffix
			if _, err := tx.ExecContext(ctx, `
				insert into xz_audit_logs(
					id,actor_id,actor_role,action,resource,resource_id,status,metadata,
					request_id,domain,result,change_reason,before_snapshot,after_snapshot,revision_before,revision_after,
					plan_id,price_plan_id,whitelist_entry_id,environment
				) values($1,$2,'SUPER_ADMIN','price_plan.test_whitelist.create','price_plan_test_whitelist',$3,201,'{}'::jsonb,
					$1,'PRICING_TEST_WHITELIST','SUCCEEDED','controlled test','{}'::jsonb,'{}'::jsonb,0,1,$4,$5,$3,'SANDBOX')
			`, id, fixture.actorID, "whitelist_audit_"+fixture.suffix, fixture.planID, fixture.pricePlanID); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(ctx, `update xz_audit_logs set result='FAILED' where id=$1`, id); err == nil || !strings.Contains(err.Error(), "PRICING_AUDIT_IMMUTABLE") {
				t.Fatalf("pricing audit update error=%v", err)
			}
		})
		runSQLTx(t, func(tx *sql.Tx) {
			id := "audit_pricing_delete_" + fixture.suffix
			if _, err := tx.ExecContext(ctx, `
				insert into xz_audit_logs(
					id,actor_id,actor_role,action,resource,resource_id,metadata,request_id,domain,result,change_reason,
					after_snapshot,revision_before,revision_after
				) values($1,$2,'SUPER_ADMIN','price_plan.test.create','test',$1,'{}'::jsonb,$1,
					'PRICING_TEST_WHITELIST','SUCCEEDED','immutability delete test','{}'::jsonb,0,1)
			`, id, fixture.actorID); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(ctx, `delete from xz_audit_logs where id=$1`, id); err == nil || !strings.Contains(err.Error(), "PRICING_AUDIT_IMMUTABLE") {
				t.Fatalf("pricing audit delete error=%v", err)
			}
		})
		runSQLTx(t, func(tx *sql.Tx) {
			id := "audit_legacy_promotion_" + fixture.suffix
			if _, err := tx.ExecContext(ctx, `
				insert into xz_audit_logs(
					id,actor_id,actor_role,action,resource,resource_id,metadata,request_id,domain,result,
					change_reason,before_snapshot,after_snapshot,revision_before,revision_after
				) values($1,$2,'SUPER_ADMIN','legacy.test','test',$1,'{}'::jsonb,$1,'LEGACY','SUCCEEDED',
					'legacy row with complete structured fields','{}'::jsonb,'{}'::jsonb,0,1)
			`, id, fixture.actorID); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(ctx, `update xz_audit_logs set domain='PRICING_PRICE_PLAN' where id=$1`, id); err == nil || !strings.Contains(err.Error(), "PRICING_AUDIT_IMMUTABLE") {
				t.Fatalf("legacy audit promotion error=%v", err)
			}
		})
		runSQLTx(t, func(tx *sql.Tx) {
			id := "audit_legacy_mutable_" + fixture.suffix
			if _, err := tx.ExecContext(ctx, `insert into xz_audit_logs(id,action,resource,metadata) values($1,'legacy.test','test','{}'::jsonb)`, id); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(ctx, `update xz_audit_logs set status=200 where id=$1`, id); err != nil {
				t.Fatalf("legacy audit update failed: %v", err)
			}
			var status int
			if err := tx.QueryRowContext(ctx, `select status from xz_audit_logs where id=$1`, id).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != 200 {
				t.Fatalf("legacy audit status=%d want=200", status)
			}
			result, err := tx.ExecContext(ctx, `delete from xz_audit_logs where id=$1`, id)
			if err != nil {
				t.Fatalf("legacy audit delete failed: %v", err)
			}
			if affected, _ := result.RowsAffected(); affected != 1 {
				t.Fatalf("legacy audit delete affected=%d want=1", affected)
			}
		})
	})

	t.Run("new pricing audits require governed identity and query indexes", func(t *testing.T) {
		runSQLTx(t, func(tx *sql.Tx) {
			_, err := tx.ExecContext(ctx, `
				insert into xz_audit_logs(
					id,actor_id,actor_role,action,resource,resource_id,metadata,domain,result,change_reason
				) values($1,$2,'SUPER_ADMIN','price_plan.update','price_plan',$3,'{}'::jsonb,
					'PRICING_PRICE_PLAN','SUCCEEDED','missing request id must fail')
			`, "audit_missing_request_"+fixture.suffix, fixture.actorID, fixture.pricePlanID)
			requirePostgresCode(t, err, "23514")
		})
		runSQLTx(t, func(tx *sql.Tx) {
			_, err := tx.ExecContext(ctx, `
				insert into xz_audit_logs(
					id,actor_id,actor_role,action,resource,resource_id,metadata,request_id,domain,result,change_reason
				) values($1,$2,'SUPER_ADMIN','price_plan.update','price_plan',$3,'{}'::jsonb,$1,
					'PRICING_PRICE_PLAN','SUCCEEDED','missing structured snapshots must fail')
			`, "audit_missing_snapshot_"+fixture.suffix, fixture.actorID, fixture.pricePlanID)
			requirePostgresCode(t, err, "23514")
		})

		indexNames := []string{
			"idx_xz_audit_logs_pricing_action_100",
			"idx_xz_audit_logs_pricing_operator_100",
			"idx_xz_audit_logs_pricing_operator_role_100",
			"idx_xz_audit_logs_pricing_plan_100",
			"idx_xz_audit_logs_pricing_plan_version_100",
			"idx_xz_audit_logs_pricing_wechat_good_100",
			"idx_xz_audit_logs_pricing_binding_100",
			"idx_xz_audit_logs_pricing_result_100",
		}
		var count int
		if err := db.QueryRowContext(ctx, `
			select count(*) from pg_indexes
			where schemaname=current_schema() and tablename='xz_audit_logs' and indexname=any($1)
		`, indexNames).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != len(indexNames) {
			t.Fatalf("pricing audit query indexes=%d want=%d", count, len(indexNames))
		}
	})
}

type phase2EFixture struct {
	suffix, actorID, userID, planID, versionID, pricePlanID, goodID, bindingID string
}

func openPhase2ETestPostgres(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("XIANZHI_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	if err := validatePhase2ETestDSN(dsn); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	return db, ctx
}

func applyPhase2EMigrationForTest(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	var migrated bool
	if err := db.QueryRowContext(ctx, `select exists(
		select 1 from information_schema.columns
		where table_name='xz_price_plan_user_whitelist' and column_name='lifecycle_status'
	)`).Scan(&migrated); err != nil {
		t.Fatal(err)
	}
	apply := os.Getenv("XIANZHI_APPLY_TEST_MIGRATION_100") == "true"
	if migrated && !apply {
		return
	}
	if !migrated && !apply {
		t.Skip("migration 100 is not applied to the isolated test database")
	}
	if _, err := db.ExecContext(ctx, phase2EMigration100SQL(t)); err != nil {
		t.Fatalf("apply migration 100 to %s: %v", phase2ETestDSN, err)
	}
}

func phase2EMigration100SQL(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "database", "migrations", "100-price-plan-test-whitelist-audit.sql"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func phase2EMigration100TransactionBody(t *testing.T) string {
	t.Helper()
	raw := phase2EMigration100SQL(t)
	beginAt := strings.Index(raw, "BEGIN;")
	commitAt := strings.LastIndex(raw, "COMMIT;")
	if beginAt < 0 || commitAt <= beginAt {
		t.Fatal("migration 100 must retain its explicit BEGIN/COMMIT transaction")
	}
	return strings.TrimSpace(raw[beginAt+len("BEGIN;") : commitAt])
}

func insertPhase2EPricePlanFixture(t *testing.T, ctx context.Context, db *sql.DB, priceType string) phase2EFixture {
	t.Helper()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	item := phase2EFixture{
		suffix: suffix, actorID: "admin_whitelist_" + suffix, userID: "user_whitelist_" + suffix,
		planID: "plan_whitelist_" + suffix, versionID: "version_whitelist_" + suffix,
		pricePlanID: "price_whitelist_" + suffix, goodID: "good_whitelist_" + suffix,
		bindingID: "binding_whitelist_" + suffix,
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_users(id,email,name,role,status) values
		($1,$1||'@example.test',$1,'SUPER_ADMIN','ACTIVE'),
		($2,$2||'@example.test',$2,'MEMBER','ACTIVE')
	`, item.actorID, item.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into xz_plans(id,code,name,plan_type,active) values($1,$1,'Phase 2E plan','MEMBER_PACKAGE',true)`, item.planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plan_versions(id,plan_id,version_no,business_type,rights_snapshot,member_level,token_amount,duration_days,status)
		values($1,$2,1,'MEMBER','{"memberLevel":"PRO","tokenAmount":100,"durationDays":30}'::jsonb,'PRO',100,30,'ACTIVE')
	`, item.versionID, item.planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plans(
			id,plan_id,plan_version_id,code,name,price_type,channel,environment,currency,
			sale_price_cents,original_price_cents,audience_type,audience_rule,is_visible,is_default,enabled,status
		) values($1,$2,$3,$1,'Phase 2E TEST price',$4,'WECHAT_VIRTUAL','SANDBOX','CNY',100,100,'TEST','{}'::jsonb,false,false,false,'DRAFT')
	`, item.pricePlanID, item.planID, item.versionID, priceType); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_wechat_virtual_goods(
			id,channel,environment,offer_id,product_id,goods_name,platform_price_cents,mode
		) values($1,'WECHAT_VIRTUAL','SANDBOX','offer',$1,'Phase 2E good',100,'short_series_goods')
	`, item.goodID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plan_payment_bindings(
			id,price_plan_id,wechat_good_id,channel,environment,provider_price_snapshot_cents,enabled,status
		) values($1,$2,$3,'WECHAT_VIRTUAL','SANDBOX',100,false,'DRAFT')
	`, item.bindingID, item.pricePlanID, item.goodID); err != nil {
		t.Fatal(err)
	}
	return item
}

func requirePostgresCode(t *testing.T, err error, code string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		t.Fatalf("PostgreSQL code=%v err=%v want=%s", func() string {
			if pgErr != nil {
				return pgErr.Code
			}
			return ""
		}(), err, code)
	}
}
