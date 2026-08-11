package httpserver

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPricePlanPhase2AGovernancePostgres(t *testing.T) {
	dsn := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var migrated bool
	if err := db.QueryRowContext(ctx, `
		select exists (
			select 1 from information_schema.columns
			where table_schema='public'
			  and table_name='xz_wechat_virtual_goods'
			  and column_name='verification_status'
		)
	`).Scan(&migrated); err != nil || !migrated {
		t.Skip("migration 098 is not applied to the test database")
	}

	runTx := func(t *testing.T, test func(*sql.Tx, string)) {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
		test(tx, suffix)
	}
	requireGovernanceError := func(t *testing.T, err error, code string) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), code) {
			t.Fatalf("expected %s, got %v", code, err)
		}
	}
	insertPlan := func(t *testing.T, tx *sql.Tx, id string) {
		t.Helper()
		if _, err := tx.ExecContext(ctx, `
			insert into xz_plans(id,code,name,plan_type,active)
			values($1,$1,'phase 2a plan','MEMBER_PACKAGE',true)
		`, id); err != nil {
			t.Fatal(err)
		}
	}
	insertVersion := func(t *testing.T, tx *sql.Tx, id, planID, status string, version int) {
		t.Helper()
		if _, err := tx.ExecContext(ctx, `
			insert into xz_plan_versions(
				id,plan_id,version_no,business_type,rights_snapshot,
				member_level,token_amount,duration_days,status
			) values($1,$2,$3,'MEMBER','{"memberLevel":"PRO","tokenAmount":100}'::jsonb,'PRO',100,30,$4)
		`, id, planID, version, status); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("new plan code is immutable", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, suffix string) {
			planID := "plan_member_" + suffix
			insertPlan(t, tx, planID)
			_, err := tx.ExecContext(ctx, `update xz_plans set code=$2 where id=$1`, planID, "plan_agent_"+suffix)
			requireGovernanceError(t, err, "PLAN_CODE_IMMUTABLE")
		})
	})

	t.Run("numeric business identifiers are allowed", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, _ string) {
			for _, code := range []string{"plan_member_v10", "plan_2026", "agent_level_02"} {
				if _, err := tx.ExecContext(ctx, `
					insert into xz_plans(id,code,name,plan_type,active)
					values($1,$1,'valid stable code','MEMBER_PACKAGE',true)
				`, code); err != nil {
					t.Fatalf("valid code %q was rejected: %v", code, err)
				}
			}
		})
	})

	t.Run("plan code rejects characters outside the base format", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, _ string) {
			_, err := tx.ExecContext(ctx, `
				insert into xz_plans(id,code,name,plan_type,active)
				values('plan_invalid_format','plan-member','invalid','MEMBER_PACKAGE',true)
			`)
			requireGovernanceError(t, err, "PLAN_CODE_FORMAT_INVALID")
		})
	})

	t.Run("plan code is unique", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, suffix string) {
			code := "plan_duplicate_" + suffix
			if _, err := tx.ExecContext(ctx, `insert into xz_plans(id,code,name,active) values($1,$3,'one',true),($2,$3,'two',true)`, "plan_one_"+suffix, "plan_two_"+suffix, code); err == nil {
				t.Fatal("duplicate plan code was accepted")
			}
		})
	})

	t.Run("only one active entitlement version exists", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, suffix string) {
			planID := "plan_member_" + suffix
			insertPlan(t, tx, planID)
			insertVersion(t, tx, "version_one_"+suffix, planID, "ACTIVE", 1)
			_, err := tx.ExecContext(ctx, `
				insert into xz_plan_versions(id,plan_id,version_no,business_type,rights_snapshot,member_level,status)
				values($1,$2,2,'MEMBER','{}'::jsonb,'PRO','ACTIVE')
			`, "version_two_"+suffix, planID)
			if err == nil {
				t.Fatal("second ACTIVE entitlement version was accepted")
			}
		})
	})

	t.Run("active entitlement rights are immutable", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, suffix string) {
			planID := "plan_member_" + suffix
			versionID := "version_active_" + suffix
			insertPlan(t, tx, planID)
			insertVersion(t, tx, versionID, planID, "ACTIVE", 1)
			_, err := tx.ExecContext(ctx, `update xz_plan_versions set token_amount=101 where id=$1`, versionID)
			requireGovernanceError(t, err, "PLAN_VERSION_ACTIVE_RIGHTS_IMMUTABLE")
		})
	})

	t.Run("draft entitlement update increments revision", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, suffix string) {
			planID := "plan_member_" + suffix
			versionID := "version_draft_" + suffix
			insertPlan(t, tx, planID)
			insertVersion(t, tx, versionID, planID, "DRAFT", 1)
			if _, err := tx.ExecContext(ctx, `update xz_plan_versions set token_amount=101,updated_by='operator' where id=$1`, versionID); err != nil {
				t.Fatal(err)
			}
			var revision int64
			if err := tx.QueryRowContext(ctx, `select revision from xz_plan_versions where id=$1`, versionID).Scan(&revision); err != nil || revision != 2 {
				t.Fatalf("revision=%d err=%v", revision, err)
			}
		})
	})

	t.Run("active price change requires clone", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, suffix string) {
			planID := "plan_member_" + suffix
			versionID := "version_active_" + suffix
			priceID := "price_active_" + suffix
			insertPlan(t, tx, planID)
			insertVersion(t, tx, versionID, planID, "ACTIVE", 1)
			if _, err := tx.ExecContext(ctx, `
				insert into xz_price_plans(
					id,plan_id,plan_version_id,code,name,price_type,channel,environment,
					sale_price_cents,original_price_cents,is_default,is_visible,enabled,status
				) values($1,$2,$3,$1,'normal','NORMAL','WECHAT_VIRTUAL','SANDBOX',100,100,true,true,true,'ACTIVE')
			`, priceID, planID, versionID); err != nil {
				t.Fatal(err)
			}
			_, err := tx.ExecContext(ctx, `update xz_price_plans set sale_price_cents=101,original_price_cents=101 where id=$1`, priceID)
			requireGovernanceError(t, err, "PRICE_PLAN_CLONE_REQUIRED")
		})
	})

	t.Run("operational price-plan update increments revision", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, suffix string) {
			planID := "plan_member_" + suffix
			versionID := "version_active_" + suffix
			priceID := "price_active_" + suffix
			insertPlan(t, tx, planID)
			insertVersion(t, tx, versionID, planID, "ACTIVE", 1)
			if _, err := tx.ExecContext(ctx, `
				insert into xz_price_plans(
					id,plan_id,plan_version_id,code,name,price_type,channel,environment,
					sale_price_cents,original_price_cents,is_visible,enabled,status
				) values($1,$2,$3,$1,'normal','NORMAL','WECHAT_VIRTUAL','SANDBOX',100,100,true,false,'ACTIVE')
			`, priceID, planID, versionID); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(ctx, `update xz_price_plans set enabled=true,enabled_by='operator' where id=$1`, priceID); err != nil {
				t.Fatal(err)
			}
			var revision int64
			if err := tx.QueryRowContext(ctx, `select revision from xz_price_plans where id=$1`, priceID).Scan(&revision); err != nil || revision != 2 {
				t.Fatalf("revision=%d err=%v", revision, err)
			}
		})
	})

	t.Run("manual WeChat confirmation requires actor reason and exact snapshot", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, suffix string) {
			goodID := "good_invalid_" + suffix
			if _, err := tx.ExecContext(ctx, `
				insert into xz_wechat_virtual_goods(
					id,environment,offer_id,product_id,goods_name,platform_price_cents
				) values($1,'SANDBOX','offer','product','test',100)
			`, goodID); err != nil {
				t.Fatal(err)
			}
			_, err := tx.ExecContext(ctx, `update xz_wechat_virtual_goods set verification_status='MANUALLY_CONFIRMED_PUBLISHED' where id=$1`, goodID)
			if err == nil {
				t.Fatal("manual confirmation without audit snapshot was accepted")
			}
		})
	})

	t.Run("manual WeChat confirmation stores local snapshot", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, suffix string) {
			goodID := "good_valid_" + suffix
			if _, err := tx.ExecContext(ctx, `
				insert into xz_wechat_virtual_goods(
					id,environment,offer_id,product_id,goods_name,platform_price_cents
				) values($1,'SANDBOX','offer','product','test',100)
			`, goodID); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(ctx, `
				update xz_wechat_virtual_goods set
					verification_status='MANUALLY_CONFIRMED_PUBLISHED',
					verified_by='operator', verified_at=now(),
					verification_reason='checked in WeChat console',
					verification_evidence='ticket-123',
					verification_snapshot=jsonb_build_object(
						'productId',product_id,'offerId',offer_id,
						'environment',environment,'platformPriceCents',platform_price_cents
					)
				where id=$1
			`, goodID); err != nil {
				t.Fatal(err)
			}
			var revision int64
			if err := tx.QueryRowContext(ctx, `select revision from xz_wechat_virtual_goods where id=$1`, goodID).Scan(&revision); err != nil || revision != 2 {
				t.Fatalf("revision=%d err=%v", revision, err)
			}
		})
	})

	t.Run("pricing permissions are registered for super admin only", func(t *testing.T) {
		var superAdminCount, otherRoleCount int
		if err := db.QueryRowContext(ctx, `select count(*) from xz_role_permissions where role='SUPER_ADMIN' and permission like 'pricing:%'`).Scan(&superAdminCount); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `select count(*) from xz_role_permissions where role<>'SUPER_ADMIN' and permission like 'pricing:%'`).Scan(&otherRoleCount); err != nil {
			t.Fatal(err)
		}
		if superAdminCount != 7 || otherRoleCount != 0 {
			t.Fatalf("superAdmin=%d otherRoles=%d", superAdminCount, otherRoleCount)
		}
	})
}
