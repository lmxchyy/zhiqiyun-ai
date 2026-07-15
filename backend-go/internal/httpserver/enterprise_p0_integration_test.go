package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var enterpriseP0FixtureSequence atomic.Int64

type enterpriseP0Fixture struct {
	prefix          string
	userID          string
	tenantIDs       []string
	organizationIDs []string
}

func enterpriseP0TestStore(t *testing.T) (*postgresStore, *sql.DB) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("XIANZHI_P0_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("XIANZHI_P0_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open P0 test database: %v", err)
	}
	db.SetMaxOpenConns(32)
	db.SetMaxIdleConns(8)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatalf("ping P0 test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &postgresStore{db: db, ready: true}, db
}

func seedEnterpriseP0Fixture(t *testing.T, db *sql.DB, tenantCount int, balance int64) enterpriseP0Fixture {
	t.Helper()
	if tenantCount <= 0 {
		tenantCount = 1
	}
	prefix := fmt.Sprintf("p0_%d_%d", time.Now().UTC().UnixNano(), enterpriseP0FixtureSequence.Add(1))
	fixture := enterpriseP0Fixture{prefix: prefix, userID: prefix + "_user"}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustEnterpriseP0Exec(t, db, `
		INSERT INTO xz_users(id,email,name,role,status,created_at,updated_at,raw)
		VALUES($1,$2,$3,'USER','ACTIVE',$4,$4,'{}'::jsonb)
	`, fixture.userID, fixture.userID+"@example.test", "P0 Test User", now)
	for index := 0; index < tenantCount; index++ {
		tenantID := fmt.Sprintf("%s_tenant_%d", prefix, index+1)
		organizationID := fmt.Sprintf("%s_org_%d", prefix, index+1)
		memberID := fmt.Sprintf("%s_member_%d", prefix, index+1)
		fixture.tenantIDs = append(fixture.tenantIDs, tenantID)
		fixture.organizationIDs = append(fixture.organizationIDs, organizationID)
		mustEnterpriseP0Exec(t, db, `
			INSERT INTO xz_tenants(id,tenant_type,owner_user_id,name,status,config)
			VALUES($1,'ENTERPRISE',$2,$3,'ACTIVE','{}'::jsonb)
		`, tenantID, fixture.userID, "P0 Enterprise")
		mustEnterpriseP0Exec(t, db, `
			INSERT INTO xz_organizations(id,tenant_id,organization_type,name,status,metadata)
			VALUES($1,$2,'DEPARTMENT',$3,'ACTIVE','{}'::jsonb)
		`, organizationID, tenantID, "P0 Organization")
		mustEnterpriseP0Exec(t, db, `
			INSERT INTO xz_tenant_members(
				id,tenant_id,user_id,role,status,primary_organization_id,member_status,
				certification_status,data_scope,joined_at,metadata
			) VALUES($1,$2,$3,'MEMBER','ACTIVE',$4,'ACTIVE','VERIFIED','SELF',now(),'{}'::jsonb)
		`, memberID, tenantID, fixture.userID, organizationID)
		mustEnterpriseP0Exec(t, db, `
			INSERT INTO xz_user_roles(user_id,tenant_id,organization_id,role,status)
			VALUES($1,$2,$3,'ENTERPRISE_MEMBER','ACTIVE')
		`, fixture.userID, tenantID, organizationID)
		mustEnterpriseP0Exec(t, db, `
			INSERT INTO xz_tenant_subscriptions(id,tenant_id,plan_code,status,entitlements)
			VALUES($1,$2,'p0_enterprise','ACTIVE','{}'::jsonb)
		`, prefix+fmt.Sprintf("_subscription_%d", index+1), tenantID)
		mustEnterpriseP0Exec(t, db, `
			INSERT INTO xz_tenant_wallets(tenant_id,point_balance,status,metadata)
			VALUES($1,$2,'ACTIVE','{}'::jsonb)
		`, tenantID, balance)
		if balance > 0 {
			mustEnterpriseP0Exec(t, db, `
				INSERT INTO xz_compute_credit_lots(
					id,tenant_id,account_id,source_type,original_units,remaining_units,
					reference_type,reference_id,idempotency_key,status,metadata
				) VALUES($1,$2,$2,'RECHARGE',$3,$3,'TEST',$4,$5,'ACTIVE','{}'::jsonb)
			`, prefix+fmt.Sprintf("_lot_%d", index+1), tenantID, balance,
				prefix+fmt.Sprintf("_seed_%d", index+1), prefix+fmt.Sprintf("_seed_lot_%d", index+1))
		}
		mustEnterpriseP0Exec(t, db, `
			UPDATE xz_tenant_service_states
			SET lifecycle_state='ACTIVE',status='ACTIVE',reason='',updated_at=now()
			WHERE tenant_id=$1
		`, tenantID)
	}
	mustEnterpriseP0Exec(t, db, `
		INSERT INTO xz_user_role_context(user_id,tenant_id,organization_id,current_role_code,context_type)
		VALUES($1,$2,$3,'ENTERPRISE_MEMBER','ENTERPRISE')
	`, fixture.userID, fixture.tenantIDs[0], fixture.organizationIDs[0])
	return fixture
}

func mustEnterpriseP0Exec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("seed P0 fixture: %v", err)
	}
}

func TestEnterpriseP0MultiTenantIsolation(t *testing.T) {
	store, db := enterpriseP0TestStore(t)
	fixture := seedEnterpriseP0Fixture(t, db, 2, 100)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for index := range fixture.tenantIDs {
		taskID := fmt.Sprintf("%s_task_%d", fixture.prefix, index+1)
		assetID := fmt.Sprintf("%s_asset_%d", fixture.prefix, index+1)
		mustEnterpriseP0Exec(t, db, `
			INSERT INTO xz_generation_tasks(
				id,user_id,tenant_id,organization_id,billing_account_type,billing_account_id,
				type,model,status,created_at,updated_at,params,result_ids,error,raw
			) VALUES($1,$2,$3,$4,'ENTERPRISE',$3,'IMAGE','p0-model','COMPLETED',$5,$5,'{}','[]','null','{}')
		`, taskID, fixture.userID, fixture.tenantIDs[index], fixture.organizationIDs[index], now)
		mustEnterpriseP0Exec(t, db, `
			INSERT INTO xz_assets(
				id,user_id,tenant_id,organization_id,task_id,name,media_type,url,metadata,created_at,updated_at,raw
			) VALUES($1,$2,$3,$4,$5,$6,'image','https://example.test/p0.png','{}',$7,$7,'{}')
		`, assetID, fixture.userID, fixture.tenantIDs[index], fixture.organizationIDs[index], taskID, assetID, now)
	}

	tasks, err := store.ListGenerationTasksForUser(fixture.userID, 20)
	if err != nil || len(tasks) != 1 || tasks[0].TenantID != fixture.tenantIDs[0] {
		t.Fatalf("tenant A task scope mismatch: tasks=%+v err=%v", tasks, err)
	}
	assets, total, err := store.ListAssetsForCenter(fixture.userID, assetCenterListQuery{Limit: 20})
	if err != nil || total != 1 || len(assets) != 1 || assets[0].TenantID != fixture.tenantIDs[0] {
		t.Fatalf("tenant A asset scope mismatch: assets=%+v total=%d err=%v", assets, total, err)
	}
	summary, err := store.AssetListSummaryForUser(fixture.userID, time.Now().UTC().Format("2006-01"))
	if err != nil || summary.Total != 1 {
		t.Fatalf("tenant A asset summary leaked: summary=%+v err=%v", summary, err)
	}
	name := "must-not-cross-tenant"
	if _, err := store.MutateAssetForUser(fixture.userID, fixture.prefix+"_asset_2", assetCenterMutation{Name: &name}); !errors.Is(err, errAssetNotFound) {
		t.Fatalf("cross-tenant asset mutation was not blocked: %v", err)
	}

	mustEnterpriseP0Exec(t, db, `
		UPDATE xz_user_role_context
		SET tenant_id=$2,organization_id=$3,current_role_code='ENTERPRISE_MEMBER',context_type='ENTERPRISE',updated_at=now()
		WHERE user_id=$1
	`, fixture.userID, fixture.tenantIDs[1], fixture.organizationIDs[1])
	tasks, err = store.ListGenerationTasksForUser(fixture.userID, 20)
	if err != nil || len(tasks) != 1 || tasks[0].TenantID != fixture.tenantIDs[1] {
		t.Fatalf("tenant B task scope mismatch after switch: tasks=%+v err=%v", tasks, err)
	}
	assets, err = store.ListAssetsForUser(fixture.userID, 20)
	if err != nil || len(assets) != 1 || assets[0].TenantID != fixture.tenantIDs[1] {
		t.Fatalf("tenant B asset scope mismatch after switch: assets=%+v err=%v", assets, err)
	}
}

func TestEnterpriseP0ConcurrentDebit(t *testing.T) {
	store, db := enterpriseP0TestStore(t)
	fixture := seedEnterpriseP0Fixture(t, db, 1, 100)
	authorization := modelCallAuthorization{
		ContextType: contextEnterprise, TenantID: fixture.tenantIDs[0], OrganizationID: fixture.organizationIDs[0],
		UserID: fixture.userID, Role: roleEnterpriseMember, BillingScope: contextEnterprise, BillingAccountID: fixture.tenantIDs[0],
	}
	var successes atomic.Int64
	var waitGroup sync.WaitGroup
	errCh := make(chan error, 20)
	for index := 0; index < 20; index++ {
		index := index
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			tx, err := db.BeginTx(context.Background(), nil)
			if err != nil {
				errCh <- err
				return
			}
			_, err = store.reserveEnterpriseComputeTx(context.Background(), tx, authorization, 7, "P0_CONCURRENCY", fmt.Sprintf("%s_request_%d", fixture.prefix, index))
			if err != nil {
				_ = tx.Rollback()
				if !strings.Contains(err.Error(), "insufficient enterprise compute units") {
					errCh <- err
				}
				return
			}
			if err := tx.Commit(); err != nil {
				errCh <- err
				return
			}
			successes.Add(1)
		}()
	}
	waitGroup.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("unexpected concurrent debit error: %v", err)
	}
	if successes.Load() != 14 {
		t.Fatalf("expected 14 successful 7-unit debits from 100 units, got %d", successes.Load())
	}
	var balance int64
	if err := db.QueryRow(`SELECT point_balance FROM xz_tenant_wallets WHERE tenant_id=$1`, fixture.tenantIDs[0]).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != 2 {
		t.Fatalf("concurrent debit balance mismatch: got %d want 2", balance)
	}
	var ledgerCount int
	if err := db.QueryRow(`SELECT count(*) FROM xz_compute_ledger_entries WHERE tenant_id=$1 AND entry_type='DEBIT'`, fixture.tenantIDs[0]).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 14 {
		t.Fatalf("concurrent debit ledger count mismatch: got %d want 14", ledgerCount)
	}
}

func TestEnterpriseP0RepeatedPaymentCallback(t *testing.T) {
	store, db := enterpriseP0TestStore(t)
	fixture := seedEnterpriseP0Fixture(t, db, 1, 0)
	orderID := fixture.prefix + "_order"
	mustEnterpriseP0Exec(t, db, `
		INSERT INTO xz_orders(id,tenant_id,user_id,buyer_user_id,amount_cents,status,created_at,price_snapshot,reward_snapshot,raw)
		VALUES($1,$2,$3,$3,99600,'PENDING',$4,'{}','{}','{}')
	`, orderID, fixture.tenantIDs[0], fixture.userID, time.Now().UTC().Format(time.RFC3339Nano))
	event := adminPaymentEvent{
		Provider: "wechat", EventID: fixture.prefix + "_event", OrderID: orderID,
		TransactionID: fixture.prefix + "_transaction", AmountCents: 99600, Verified: true,
	}
	var inserted atomic.Int64
	var duplicates atomic.Int64
	var waitGroup sync.WaitGroup
	errCh := make(chan error, 8)
	for index := 0; index < 8; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			duplicate, err := store.RegisterPaymentCallbackEvent(event)
			if err != nil {
				errCh <- err
				return
			}
			if duplicate {
				duplicates.Add(1)
			} else {
				inserted.Add(1)
			}
		}()
	}
	waitGroup.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("repeated payment callback failed: %v", err)
	}
	if inserted.Load() != 1 || duplicates.Load() != 7 {
		t.Fatalf("callback idempotency mismatch: inserted=%d duplicates=%d", inserted.Load(), duplicates.Load())
	}
	var count int
	var idempotencyKey string
	if err := db.QueryRow(`
		SELECT count(*),max(idempotency_key) FROM xz_payment_events
		WHERE provider=$1 AND event_id=$2
	`, normalizePaymentMethod(event.Provider), event.EventID).Scan(&count, &idempotencyKey); err != nil {
		t.Fatal(err)
	}
	if count != 1 || idempotencyKey == "" {
		t.Fatalf("expected one payment event with an idempotency key, count=%d key=%q", count, idempotencyKey)
	}
}

func TestEnterpriseP0RechargeAndBonusSplit(t *testing.T) {
	store, db := enterpriseP0TestStore(t)
	fixture := seedEnterpriseP0Fixture(t, db, 1, 0)
	requestID := fixture.prefix + "_recharge"
	_, err := store.MutateAdminEnterprise("", "SUPER_ADMIN", fixture.tenantIDs[0], adminEnterpriseMutationRequest{
		Action: "recharge", RequestID: requestID, Reason: "P0 recharge split test",
		AmountCents: 99600, RechargeUnits: 9960, BonusUnits: 4000,
	})
	if err != nil {
		t.Fatalf("enterprise recharge mutation failed: %v", err)
	}
	var balance, rechargeTotal, bonusTotal int64
	if err := db.QueryRow(`
		SELECT point_balance,total_recharge_units,total_bonus_units
		FROM xz_tenant_wallets WHERE tenant_id=$1
	`, fixture.tenantIDs[0]).Scan(&balance, &rechargeTotal, &bonusTotal); err != nil {
		t.Fatal(err)
	}
	if balance != 13960 || rechargeTotal != 9960 || bonusTotal != 4000 {
		t.Fatalf("recharge projection mismatch: balance=%d recharge=%d bonus=%d", balance, rechargeTotal, bonusTotal)
	}
	var rechargeLots, bonusLots int
	var rechargeAmountCents int64
	if err := db.QueryRow(`
		SELECT
			count(*) FILTER (WHERE source_type='RECHARGE'),
			count(*) FILTER (WHERE source_type='BONUS'),
			coalesce(sum(amount_cents) FILTER (WHERE source_type='RECHARGE'),0)
		FROM xz_compute_credit_lots
		WHERE tenant_id=$1 AND reference_id=$2
	`, fixture.tenantIDs[0], requestID).Scan(&rechargeLots, &bonusLots, &rechargeAmountCents); err != nil {
		t.Fatal(err)
	}
	if rechargeLots != 1 || bonusLots != 1 || rechargeAmountCents != 99600 {
		t.Fatalf("recharge/bonus lots were not split: recharge=%d bonus=%d cents=%d", rechargeLots, bonusLots, rechargeAmountCents)
	}
}

func TestEnterpriseP0AttributionHistory(t *testing.T) {
	store, db := enterpriseP0TestStore(t)
	fixture := seedEnterpriseP0Fixture(t, db, 1, 0)
	for index := 1; index <= 2; index++ {
		_, err := store.MutateAdminEnterprise("", "SUPER_ADMIN", fixture.tenantIDs[0], adminEnterpriseMutationRequest{
			Action: "attribution-change", RequestID: fmt.Sprintf("%s_attribution_%d", fixture.prefix, index),
			Reason: fmt.Sprintf("P0 attribution history %d", index),
		})
		if err != nil {
			t.Fatalf("attribution change %d failed: %v", index, err)
		}
	}
	var total, active, superseded int
	if err := db.QueryRow(`
		SELECT count(*),count(*) FILTER (WHERE status='ACTIVE'),count(*) FILTER (WHERE status='SUPERSEDED')
		FROM xz_customer_attribution_history WHERE tenant_id=$1
	`, fixture.tenantIDs[0]).Scan(&total, &active, &superseded); err != nil {
		t.Fatal(err)
	}
	if total != 2 || active != 1 || superseded != 1 {
		t.Fatalf("attribution history mismatch: total=%d active=%d superseded=%d", total, active, superseded)
	}
}

func TestEnterpriseP0DisabledTenantBlocksModelCall(t *testing.T) {
	store, db := enterpriseP0TestStore(t)
	fixture := seedEnterpriseP0Fixture(t, db, 1, 100)
	if _, err := store.AuthorizeModelCall(fixture.userID, "IMAGE_GENERATION"); err != nil {
		t.Fatalf("active enterprise should be authorized: %v", err)
	}
	mustEnterpriseP0Exec(t, db, `
		UPDATE xz_tenant_service_states
		SET lifecycle_state='PAUSED',reason='P0 test',state_version=state_version+1,updated_at=now()
		WHERE tenant_id=$1
	`, fixture.tenantIDs[0])
	if _, err := store.AuthorizeModelCall(fixture.userID, "IMAGE_GENERATION"); !errors.Is(err, errEnterpriseServiceUnavailable) {
		t.Fatalf("paused enterprise model call was not blocked: %v", err)
	}
}

func TestEnterpriseP0AuditAndImmutableLedger(t *testing.T) {
	store, db := enterpriseP0TestStore(t)
	fixture := seedEnterpriseP0Fixture(t, db, 1, 30)
	authorization := modelCallAuthorization{
		ContextType: contextEnterprise, TenantID: fixture.tenantIDs[0], OrganizationID: fixture.organizationIDs[0],
		UserID: fixture.userID, Role: roleEnterpriseMember, BillingScope: contextEnterprise, BillingAccountID: fixture.tenantIDs[0],
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := store.reserveEnterpriseComputeTx(context.Background(), tx, authorization, 10, "P0_AUDIT", fixture.prefix+"_audit_request")
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var auditCount int
	if err := db.QueryRow(`
		SELECT count(*) FROM xz_tenant_audit_logs
		WHERE tenant_id=$1 AND resource_id=$2 AND before_value <> '{}'::jsonb AND after_value <> '{}'::jsonb
	`, fixture.tenantIDs[0], reservation.LedgerID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one before/after audit record, got %d", auditCount)
	}
	if _, err := db.Exec(`UPDATE xz_compute_ledger_entries SET status='VOID' WHERE id=$1`, reservation.LedgerID); err == nil {
		t.Fatal("append-only compute ledger accepted UPDATE")
	}
	if _, err := db.Exec(`DELETE FROM xz_compute_ledger_entries WHERE id=$1`, reservation.LedgerID); err == nil {
		t.Fatal("append-only compute ledger accepted DELETE")
	}
}
