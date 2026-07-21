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

func TestCommissionEnginePersists996RecordsIdempotently(t *testing.T) {
	databaseURL := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	userID := "commission_user_" + suffix
	orderID := "commission_order_" + suffix
	orderNo := "COMMISSION" + suffix
	paidAt := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_users (id,email,name,role,status,created_at,updated_at,raw)
		VALUES ($1,$2,'Commission Test','MEMBER','ACTIVE',$3,$3,'{}'::jsonb)
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

	plan, ok := planCatalogByID("plan_ai_creator_996")
	if !ok {
		t.Fatal("996 membership plan is missing")
	}
	order := adminOrder{
		ID: orderID, OrderNo: orderNo, TenantID: "tenant_default", UserID: userID,
		PlanID: plan.ID, AmountCents: 99600, Status: "PAID", PaidAt: paidAt.Format(time.RFC3339Nano),
		PriceSnapshot: map[string]any{},
	}
	commerceCtx := commissionOrderContext{
		OrderID: orderID, OrderType: orderTypeUserRechargeDirect, PlanType: planTypeMemberPackage,
		AmountCents: 99600, BuyerUserID: userID, DirectAgentID: "agent-1",
		OperationCenterID: "operation-center-1", TokenGrantAmount: 40000, TokenGrantValueCents: 40000,
	}
	for attempt := 0; attempt < 2; attempt++ {
		result, err := generateCommissionRecordsForCommerceOrderTx(ctx, tx, order, plan, commerceCtx)
		if err != nil {
			t.Fatal(err)
		}
		if result.PlatformIncomeCents != 49600 || len(result.Records) != 3 {
			t.Fatalf("unexpected 996 result: %+v", result)
		}
	}

	var count int
	var total int64
	var platform int64
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*), coalesce(sum(amount_cents),0),
		       coalesce(sum(amount_cents) FILTER (WHERE beneficiary_type='PLATFORM'),0)
		FROM xz_commission_records WHERE order_id=$1
	`, orderID).Scan(&count, &total, &platform); err != nil {
		t.Fatal(err)
	}
	if count != 3 || total != 99600 || platform != 49600 {
		t.Fatalf("persisted records count=%d total=%d platform=%d", count, total, platform)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := reverseCommissionRecordsForOrderTx(ctx, tx, orderID, orderNo, paidAt.Add(time.Hour)); err != nil {
			t.Fatalf("reverse pending commission: %v", err)
		}
	}
	var cancelled int
	var reversals int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*) FILTER (WHERE record_type='EARNING' AND status='CANCELLED'),
		       count(*) FILTER (WHERE record_type='REVERSAL')
		FROM xz_commission_records WHERE order_id=$1
	`, orderID).Scan(&cancelled, &reversals); err != nil {
		t.Fatal(err)
	}
	if cancelled != 3 || reversals != 0 {
		t.Fatalf("pending refund cancellation mismatch: cancelled=%d reversals=%d", cancelled, reversals)
	}
}
