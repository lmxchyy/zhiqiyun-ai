package httpserver

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestV132CanaryDecisionIdempotencyWriteGuardRollbackAndConservationPostgres(t *testing.T) {
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
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	userID := "v132_canary_user_" + suffix
	orderID := "v132_canary_order_" + suffix
	orderNo := "V132CANARY" + suffix
	paidAt := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_users (id,email,name,role,status,created_at,updated_at,raw)
		VALUES ($1,$2,'V132 Canary','MEMBER','ACTIVE',$3,$3,'{}'::jsonb)
	`, userID, userID+"@example.test", paidAt.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_orders (
		  id,order_no,tenant_id,user_id,plan_id,amount_cents,status,paid_at,created_at,price_snapshot,raw
		) VALUES ($1,$2,'tenant_default',$3,'plan_ai_creator_996',99600,'PAID',$4,$4,'{}'::jsonb,'{}'::jsonb)
	`, orderID, orderNo, userID, paidAt.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE xz_channel_rollout_configs
		SET mode='CANARY',real_switch_enabled=TRUE,percentage_rollout_enabled=FALSE,
		    canary_basis_points=0,allow_order_ids=jsonb_build_array($1::text),
		    change_reason='integration whitelist',updated_by='test'
		WHERE tenant_id='tenant_default'
	`, orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE xz_commercial_rule_sets
		SET status='PUBLISHED',published_by='test',published_at=$1,updated_at=$1
		WHERE id='channel_rules_v132_default_v1'
	`, paidAt); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE xz_commission_rules SET status='ACTIVE',updated_at=$1
		WHERE commercial_rule_set_id='channel_rules_v132_default_v1'
	`, paidAt); err != nil {
		t.Fatal(err)
	}

	plan, ok := planCatalogByID("plan_ai_creator_996")
	if !ok {
		t.Fatal("member plan missing")
	}
	order := adminOrder{
		ID: orderID, OrderNo: orderNo, TenantID: "tenant_default", UserID: userID,
		PlanID: plan.ID, AmountCents: 99600, Status: "PAID", PaidAt: paidAt.Format(time.RFC3339Nano),
		PriceSnapshot: map[string]any{},
	}
	decision, err := resolveOrderSettlementDecisionTx(ctx, tx, &order, plan)
	if err != nil {
		t.Fatal(err)
	}
	if decision.SettlementEngine != settlementEngineV132 {
		t.Fatalf("whitelisted order decision=%+v", decision)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := claimSettlementWriteSourceTx(ctx, tx, decision); err != nil {
			t.Fatalf("same-source claim attempt %d: %v", attempt, err)
		}
	}
	conflict := decision
	conflict.SettlementEngine = settlementEngineLegacy
	if err := claimSettlementWriteSourceTx(ctx, tx, conflict); err == nil {
		t.Fatal("legacy source was accepted after V1.3.2 claim")
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE xz_channel_rollout_configs
		SET mode='SHADOW',real_switch_enabled=FALSE,allow_order_ids='[]'::jsonb,
		    change_reason='integration rollback',updated_by='test'
		WHERE tenant_id='tenant_default'
	`); err != nil {
		t.Fatal(err)
	}
	replayedOrder := order
	replayedOrder.PriceSnapshot = map[string]any{}
	replayed, err := resolveOrderSettlementDecisionTx(ctx, tx, &replayedOrder, plan)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.SettlementEngine != settlementEngineV132 || replayed.RuleSetID != decision.RuleSetID {
		t.Fatalf("rollback changed historical decision: %+v", replayed)
	}

	commerceCtx := commissionOrderContext{
		OrderID: orderID, OrderType: orderTypePlatformDirectRecharge, PlanType: planTypeMemberPackage,
		AmountCents: 99600, BuyerUserID: userID, TokenGrantAmount: 40000, TokenGrantValueCents: 40000,
	}
	for attempt := 0; attempt < 2; attempt++ {
		result, err := generateV132CommissionRecordsForCommerceOrderTx(ctx, tx, &order, plan, commerceCtx, decision)
		if err != nil {
			t.Fatalf("V1.3.2 generation attempt %d: %v", attempt, err)
		}
		if int64(result.PlatformIncomeCents) != 59600 {
			t.Fatalf("platform income=%d", result.PlatformIncomeCents)
		}
	}
	var count int
	var cashTotal, platformTotal int64
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*),coalesce(sum(amount_cents),0),
		       coalesce(sum(amount_cents) FILTER (WHERE beneficiary_type='PLATFORM'),0)
		FROM xz_commission_records WHERE order_id=$1
	`, orderID).Scan(&count, &cashTotal, &platformTotal); err != nil {
		t.Fatal(err)
	}
	if count != 1 || cashTotal != 59600 || platformTotal != 59600 || cashTotal+40000 != 99600 {
		t.Fatalf("conservation count=%d cash=%d platform=%d token=%d", count, cashTotal, platformTotal, 40000)
	}
}
