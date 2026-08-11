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

func TestPricePlanPhase2DGovernancePostgres(t *testing.T) {
	dsn := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var migrated bool
	if err := db.QueryRowContext(ctx, `
		select to_regprocedure('xz_guard_price_plan_099()') is not null
		  and exists(select 1 from information_schema.columns where table_name='xz_order_price_quotes' and column_name='bonus_tokens')
	`).Scan(&migrated); err != nil || !migrated {
		t.Skip("migration 099 is not applied to the test database")
	}

	type fixture struct {
		planID, versionID, userID, priceID, goodID, bindingID string
	}
	runTx := func(t *testing.T, fn func(*sql.Tx, fixture)) {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
		item := fixture{
			planID: "plan_guard_" + suffix, versionID: "version_guard_" + suffix,
			userID: "user_guard_" + suffix, priceID: "price_guard_" + suffix,
			goodID: "good_guard_" + suffix, bindingID: "binding_guard_" + suffix,
		}
		if _, err := tx.ExecContext(ctx, `insert into xz_users(id,email,name,role,status) values($1,$1||'@example.test',$1,'MEMBER','ACTIVE')`, item.userID); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `insert into xz_plans(id,code,name,plan_type,active) values($1,$1,'guard plan','MEMBER_PACKAGE',true)`, item.planID); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `
			insert into xz_plan_versions(id,plan_id,version_no,business_type,rights_snapshot,member_level,token_amount,duration_days,status)
			values($1,$2,1,'MEMBER','{"memberLevel":"PRO","tokenAmount":100,"durationDays":30}'::jsonb,'PRO',100,30,'ACTIVE')
		`, item.versionID, item.planID); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `
			insert into xz_price_plans(
				id,plan_id,plan_version_id,code,name,price_type,channel,environment,currency,
				sale_price_cents,original_price_cents,audience_type,audience_rule,is_visible,is_default,enabled,status
			) values($1,$2,$3,$1,'guard price','NORMAL','WECHAT_VIRTUAL','SANDBOX','CNY',100,100,'PUBLIC','{}'::jsonb,true,false,false,'DRAFT')
		`, item.priceID, item.planID, item.versionID); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `
			insert into xz_wechat_virtual_goods(
				id,channel,environment,offer_id,product_id,goods_name,platform_price_cents,mode
			) values($1,'WECHAT_VIRTUAL','SANDBOX','offer',$1,'guard good',100,'short_series_goods')
		`, item.goodID); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `
			insert into xz_price_plan_payment_bindings(
				id,price_plan_id,wechat_good_id,channel,environment,provider_price_snapshot_cents,enabled,status
			) values($1,$2,$3,'WECHAT_VIRTUAL','SANDBOX',100,false,'DRAFT')
		`, item.bindingID, item.priceID, item.goodID); err != nil {
			t.Fatal(err)
		}
		fn(tx, item)
	}
	requireCode := func(t *testing.T, err error, code string) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), code) {
			t.Fatalf("expected %s, got %v", code, err)
		}
	}
	insertV2Quote := func(t *testing.T, tx *sql.Tx, item fixture) string {
		t.Helper()
		quoteID := "quote_v2_" + item.priceID
		if _, err := tx.ExecContext(ctx, `
			insert into xz_order_price_quotes(
				id,quote_token_hash,tenant_id,user_id,plan_id,plan_version_id,price_plan_id,payment_binding_id,wechat_good_id,
				entry_type,transaction_price_cents,provider_price_snapshot_cents,wechat_goods_price_cents,channel,environment,
				offer_id,wechat_product_id,payment_mode,rights_snapshot,commission_snapshot,expires_at
			) values($1,$1,'tenant_default',$2,$3,$4,$5,$6,$7,'PUBLIC',100,100,100,'WECHAT_VIRTUAL','SANDBOX',
				'offer',$7,'short_series_goods',
				'{"memberLevel":"PRO","tokenAmount":100,"pointsAmount":0,"durationDays":30}'::jsonb,
				'{}'::jsonb,now()+interval '5 minutes')
		`, quoteID, item.userID, item.planID, item.versionID, item.priceID, item.bindingID, item.goodID); err != nil {
			t.Fatal(err)
		}
		return quoteID
	}
	insertV2Order := func(t *testing.T, tx *sql.Tx, item fixture) string {
		t.Helper()
		quoteID := insertV2Quote(t, tx, item)
		orderID := "order_v2_" + item.priceID
		if _, err := tx.ExecContext(ctx, `
			insert into xz_orders(
				id,order_no,tenant_id,user_id,plan_id,plan_version_id,price_plan_id,price_quote_id,
				snapshot_version,transaction_price_cents,wechat_product_id_snapshot,wechat_goods_price_cents,
				currency,payment_environment,rights_snapshot,commission_rule_version_snapshot,commission_snapshot_v2,
				amount_cents,status,fulfillment_status,entitlement_status,
				product_code,product_name,product_type,payment_channel,payment_scene,payment_mode,wechat_openid_hash,
				created_at,updated_at,price_snapshot,reward_snapshot,raw
			) values(
				$1,$1,'tenant_default',$2,$3,$4,$5,$6,
				2,100,$7,100,'CNY','SANDBOX',
				'{"memberLevel":"PRO","tokenAmount":100,"pointsAmount":0,"durationDays":30}'::jsonb,
				'','{}'::jsonb,100,'PENDING','PENDING','PENDING',
				$3,'guard plan','MEMBERSHIP','WECHAT_VIRTUAL','MINI_PROGRAM','short_series_goods','openid-hash',
				now()::text,now(),
				jsonb_build_object(
					'snapshotVersion',2,'planId',$3::text,'planVersionId',$4::text,'pricePlanId',$5::text,
					'currency','CNY','transactionPriceCents',100,'wechatGoodsPriceCents',100,
					'paymentChannel','WECHAT_VIRTUAL','paymentEnvironment','SANDBOX',
					'rights','{"memberLevel":"PRO","tokenAmount":100,"pointsAmount":0,"durationDays":30}'::jsonb,
					'commissionRuleVersion','','commissionSnapshotV2','{}'::jsonb,
					'productCode',$3::text,'productName','guard plan','productType','MEMBERSHIP','planType','MEMBER_PACKAGE',
					'amountCents',100,'memberLevel','PRO','memberDays',30,'creditUnits',100,'pointUnits',0,
					'buyQuantity',1,'unitPriceCents',100,'unitCreditUnits',100,
					'offerId','offer','wechatProductId',$7::text,'mode','short_series_goods','env',1
				),'{}'::jsonb,'{}'::jsonb
			)
		`, orderID, item.userID, item.planID, item.versionID, item.priceID, quoteID, item.goodID); err != nil {
			t.Fatal(err)
		}
		return orderID
	}

	t.Run("draft economics remain editable before history", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, item fixture) {
			if _, err := tx.ExecContext(ctx, `update xz_price_plans set sale_price_cents=101,original_price_cents=101 where id=$1`, item.priceID); err != nil {
				t.Fatal(err)
			}
			var price, revision int64
			if err := tx.QueryRowContext(ctx, `select sale_price_cents,revision from xz_price_plans where id=$1`, item.priceID).Scan(&price, &revision); err != nil || price != 101 || revision != 2 {
				t.Fatalf("price=%d revision=%d err=%v", price, revision, err)
			}
		})
	})

	t.Run("quote history freezes draft economics", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, item fixture) {
			if _, err := tx.ExecContext(ctx, `
				insert into xz_order_price_quotes(
					id,quote_token_hash,tenant_id,user_id,plan_id,plan_version_id,price_plan_id,payment_binding_id,wechat_good_id,
					entry_type,transaction_price_cents,provider_price_snapshot_cents,wechat_goods_price_cents,channel,environment,
					offer_id,wechat_product_id,payment_mode,rights_snapshot,expires_at
				) values($1,$1,'tenant_default',$2,$3,$4,$5,$6,$7,'PUBLIC',100,100,100,'WECHAT_VIRTUAL','SANDBOX',
					'offer',$7,'short_series_goods','{}'::jsonb,now()+interval '5 minutes')
			`, "quote_guard_"+item.priceID, item.userID, item.planID, item.versionID, item.priceID, item.bindingID, item.goodID); err != nil {
				t.Fatal(err)
			}
			_, err := tx.ExecContext(ctx, `update xz_price_plans set sale_price_cents=101,original_price_cents=101 where id=$1`, item.priceID)
			requireCode(t, err, "PRICE_PLAN_CLONE_REQUIRED")
		})
	})

	t.Run("order history freezes draft economics", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, item fixture) {
			orderID := "order_guard_" + item.priceID
			if _, err := tx.ExecContext(ctx, `
				insert into xz_orders(
					id,order_no,user_id,plan_id,price_plan_id,snapshot_version,
					amount_cents,status,created_at,price_snapshot,raw
				) values($1,$1,$2,$3,$4,1,100,'PENDING',$5,'{}'::jsonb,'{}'::jsonb)
			`, orderID, item.userID, item.planID, item.priceID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				t.Fatal(err)
			}
			_, err := tx.ExecContext(ctx, `update xz_price_plans set sale_price_cents=101,original_price_cents=101 where id=$1`, item.priceID)
			requireCode(t, err, "PRICE_PLAN_CLONE_REQUIRED")
		})
	})

	t.Run("status regression is blocked", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, item fixture) {
			if _, err := tx.ExecContext(ctx, `update xz_price_plans set status='ACTIVE',enabled=true,enabled_at=now() where id=$1`, item.priceID); err != nil {
				t.Fatal(err)
			}
			_, err := tx.ExecContext(ctx, `update xz_price_plans set status='DRAFT' where id=$1`, item.priceID)
			requireCode(t, err, "PRICE_PLAN_STATUS_REGRESSION_FORBIDDEN")
		})
	})

	t.Run("enabled history is immutable", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, item fixture) {
			if _, err := tx.ExecContext(ctx, `update xz_price_plans set status='ACTIVE',enabled=true,enabled_at=now() where id=$1`, item.priceID); err != nil {
				t.Fatal(err)
			}
			_, err := tx.ExecContext(ctx, `update xz_price_plans set enabled_at=null where id=$1`, item.priceID)
			requireCode(t, err, "PRICE_PLAN_ENABLED_HISTORY_IMMUTABLE")
		})
	})

	t.Run("price plans cannot be deleted", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, item fixture) {
			_, err := tx.ExecContext(ctx, `delete from xz_price_plans where id=$1`, item.priceID)
			requireCode(t, err, "PRICE_PLAN_DELETE_FORBIDDEN")
		})
	})

	t.Run("database keeps one valid default as final defense", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, item fixture) {
			if _, err := tx.ExecContext(ctx, `update xz_price_plans set status='ACTIVE',enabled=true,is_default=true where id=$1`, item.priceID); err != nil {
				t.Fatal(err)
			}
			secondID := "price_second_" + item.priceID
			_, err := tx.ExecContext(ctx, `
				insert into xz_price_plans(
					id,plan_id,plan_version_id,code,name,price_type,channel,environment,currency,
					sale_price_cents,original_price_cents,audience_type,audience_rule,is_visible,is_default,enabled,status
				) values($1,$2,$3,$1,'second','NORMAL','WECHAT_VIRTUAL','SANDBOX','CNY',100,100,'PUBLIC','{}'::jsonb,true,true,true,'ACTIVE')
			`, secondID, item.planID, item.versionID)
			if err == nil {
				t.Fatal("database accepted two defaults")
			}
		})
	})

	t.Run("invalid TEST scope is rejected", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, item fixture) {
			_, err := tx.ExecContext(ctx, `update xz_price_plans set price_type='TEST',audience_type='TEST',is_visible=true where id=$1`, item.priceID)
			if err == nil {
				t.Fatal("visible TEST price plan was accepted")
			}
		})
	})

	t.Run("inactive default is rejected", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, item fixture) {
			_, err := tx.ExecContext(ctx, `update xz_price_plans set is_default=true where id=$1`, item.priceID)
			if err == nil {
				t.Fatal("inactive DRAFT default was accepted")
			}
		})
	})

	for _, tc := range []struct {
		name string
		sql  string
	}{
		{name: "normalized price", sql: `update xz_orders set amount_cents=101 where id=$1`},
		{name: "buyer identity", sql: `update xz_orders set buyer_user_id='different-buyer' where id=$1`},
		{name: "json price snapshot", sql: `update xz_orders set price_snapshot=jsonb_set(price_snapshot,'{amountCents}','101'::jsonb) where id=$1`},
		{name: "rights snapshot", sql: `update xz_orders set rights_snapshot=jsonb_set(rights_snapshot,'{tokenAmount}','101'::jsonb) where id=$1`},
		{name: "payment identity", sql: `update xz_orders set wechat_product_id_snapshot='different-product' where id=$1`},
		{name: "commission snapshot", sql: `update xz_orders set commission_snapshot_v2='{"changed":true}'::jsonb where id=$1`},
		{name: "snapshot version downgrade", sql: `update xz_orders set snapshot_version=1 where id=$1`},
	} {
		t.Run("V2 snapshot rejects "+tc.name+" mutation", func(t *testing.T) {
			runTx(t, func(tx *sql.Tx, item fixture) {
				orderID := insertV2Order(t, tx, item)
				_, err := tx.ExecContext(ctx, tc.sql, orderID)
				requireCode(t, err, "ORDER_V2_SNAPSHOT_IMMUTABLE")
			})
		})
	}

	for _, tc := range []struct {
		name           string
		initialVersion any
	}{
		{name: "V1", initialVersion: int16(1)},
		{name: "unversioned V1", initialVersion: nil},
	} {
		t.Run(tc.name+" order cannot be upgraded to V2 by update", func(t *testing.T) {
			runTx(t, func(tx *sql.Tx, item fixture) {
				quoteID := insertV2Quote(t, tx, item)
				orderID := "legacy_upgrade_" + item.priceID
				if _, err := tx.ExecContext(ctx, `
					insert into xz_orders(
						id,order_no,tenant_id,user_id,plan_id,snapshot_version,
						amount_cents,status,created_at,price_snapshot,raw
					) values($1,$1,'tenant_default',$2,$3,$4,100,'PENDING',now()::text,'{}'::jsonb,'{}'::jsonb)
				`, orderID, item.userID, item.planID, tc.initialVersion); err != nil {
					t.Fatal(err)
				}
				_, err := tx.ExecContext(ctx, `
					update xz_orders
					set snapshot_version=2,plan_version_id=$2,price_plan_id=$3,price_quote_id=$4,
					    transaction_price_cents=100,wechat_product_id_snapshot=$5,wechat_goods_price_cents=100,
					    payment_channel='WECHAT_VIRTUAL',payment_environment='SANDBOX',
					    rights_snapshot='{}'::jsonb,commission_rule_version_snapshot='',commission_snapshot_v2='{}'::jsonb,
					    price_snapshot='{"snapshotVersion":2}'::jsonb
					where id=$1
				`, orderID, item.versionID, item.priceID, quoteID, item.goodID)
				requireCode(t, err, "ORDER_V2_SNAPSHOT_IMMUTABLE")
			})
		})
	}

	t.Run("V2 order lifecycle fields remain mutable", func(t *testing.T) {
		runTx(t, func(tx *sql.Tx, item fixture) {
			orderID := insertV2Order(t, tx, item)
			if _, err := tx.ExecContext(ctx, `
				update xz_orders
				set status='PAID',paid_at=now()::text,
				    entitlement_status='GRANTED',entitlement_error='',
				    entitlement_started_at=now(),entitlement_granted_at=now(),
				    fulfillment_status='FULFILLED',fulfilled_at=now()::text,
				    wechat_order_id='wechat-order',wechat_transaction_id='wechat-transaction',
				    compensation_locked_until=now()+interval '1 minute',updated_at=now()
				where id=$1
			`, orderID); err != nil {
				t.Fatalf("lifecycle update was blocked: %v", err)
			}
			var status, entitlementStatus, fulfillmentStatus string
			if err := tx.QueryRowContext(ctx, `
				select status,entitlement_status,fulfillment_status from xz_orders where id=$1
			`, orderID).Scan(&status, &entitlementStatus, &fulfillmentStatus); err != nil {
				t.Fatal(err)
			}
			if status != "PAID" || entitlementStatus != "GRANTED" || fulfillmentStatus != "FULFILLED" {
				t.Fatalf("lifecycle update was not persisted: status=%s entitlement=%s fulfillment=%s", status, entitlementStatus, fulfillmentStatus)
			}
		})
	})
}
