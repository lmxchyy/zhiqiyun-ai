package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

const authMergeTestDSN = "postgres://xianzhi_test:gift_points_auth_103_only@127.0.0.1:55443/xianzhi_auth_merge_test?sslmode=disable"

func TestPostgresAuthMergeActiveReservationRollsBackIdentityAndPoints(t *testing.T) {
	db, ctx := openAuthMergeTestPostgres(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	targetID, sourceID := "auth_merge_target_"+suffix, "auth_merge_source_"+suffix
	mergeID, sourceAccountID := "auth_merge_reservation_"+suffix, "auth_merge_points_"+suffix
	now := time.Now().UTC().Format(time.RFC3339Nano)
	target := adminUser{ID: targetID, Email: targetID + "@example.test", Name: "Merge target", Role: "MEMBER", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now}
	source := adminUser{ID: sourceID, Email: sourceID + "@example.test", Name: "Merge source", Role: "MEMBER", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now}
	request := adminAuthMergeRequest{ID: mergeID, PrimaryUserID: targetID, SecondaryUserID: sourceID, ConflictCode: "AUTH_ACCOUNT_MERGE_REQUIRED", Source: "postgres_atomic_test", Reason: "active reservation", Status: "PENDING", CreatedAt: now, UpdatedAt: now}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range []adminUser{target, source} {
		if err := insertUser(ctx, tx, user); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := insertAuthMergeRequest(ctx, tx, request); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	pointStore := NewPostgresPersonalPointStore(db)
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{AccountID: sourceAccountID, UserID: sourceID, Source: PointSourceRecharge, Points: 3, ReferenceType: "ORDER", ReferenceID: mergeID, IdempotencyKey: "grant"}); err != nil {
		t.Fatal(err)
	}
	if _, err := pointStore.reserve(ctx, PersonalPointReserveCommand{AccountID: sourceAccountID, UserID: sourceID, BusinessType: "GENERATION_TASK", BusinessID: mergeID, RequestedPoints: 1, IdempotencyKey: "reserve"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := contextWithPostgresTestTimeout()
		defer cancel()
		cleanupTx, cleanupErr := db.BeginTx(cleanupCtx, nil)
		if cleanupErr != nil {
			t.Errorf("begin cleanup: %v", cleanupErr)
			return
		}
		defer func() { _ = cleanupTx.Rollback() }()
		if _, err := cleanupTx.ExecContext(cleanupCtx, `ALTER TABLE xz_personal_point_lot_movements DISABLE TRIGGER trg_xz_personal_point_lot_movements_immutable`); err != nil {
			t.Errorf("disable movement cleanup guard: %v", err)
			return
		}
		statements := []struct {
			query string
			args  []any
		}{
			{`DELETE FROM xz_audit_logs WHERE resource_id=$1`, []any{mergeID}},
			{`DELETE FROM xz_personal_point_lot_movements WHERE account_id=$1`, []any{sourceAccountID}},
			{`DELETE FROM xz_personal_point_reservation_allocations WHERE account_id=$1`, []any{sourceAccountID}},
			{`DELETE FROM xz_personal_point_reservations WHERE account_id=$1`, []any{sourceAccountID}},
			{`DELETE FROM xz_personal_point_lots WHERE account_id=$1`, []any{sourceAccountID}},
			{`DELETE FROM xz_wallet_ledger WHERE account_id=$1`, []any{sourceAccountID}},
			{`DELETE FROM xz_user_wallets WHERE user_id IN($1,$2)`, []any{targetID, sourceID}},
			{`DELETE FROM xz_point_accounts WHERE id=$1`, []any{sourceAccountID}},
			{`DELETE FROM xz_auth_account_merge_requests WHERE id=$1`, []any{mergeID}},
			{`DELETE FROM xz_users WHERE id IN($1,$2)`, []any{targetID, sourceID}},
		}
		for _, statement := range statements {
			if _, err := cleanupTx.ExecContext(cleanupCtx, statement.query, statement.args...); err != nil {
				t.Errorf("cleanup fixture: %v", err)
				return
			}
		}
		if _, err := cleanupTx.ExecContext(cleanupCtx, `ALTER TABLE xz_personal_point_lot_movements ENABLE TRIGGER trg_xz_personal_point_lot_movements_immutable`); err != nil {
			t.Errorf("enable movement cleanup guard: %v", err)
			return
		}
		if err := cleanupTx.Commit(); err != nil {
			t.Errorf("commit cleanup: %v", err)
		}
	})
	var targetRawBefore, sourceRawBefore, requestRawBefore string
	if err := db.QueryRowContext(ctx, `SELECT raw::text FROM xz_users WHERE id=$1`, targetID).Scan(&targetRawBefore); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT raw::text FROM xz_users WHERE id=$1`, sourceID).Scan(&sourceRawBefore); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT raw::text FROM xz_auth_account_merge_requests WHERE id=$1`, mergeID).Scan(&requestRawBefore); err != nil {
		t.Fatal(err)
	}
	store := &postgresStore{db: db, ready: true}
	_, _, err = store.ExecuteAdminAuthMergeRequest(mergeID, adminAuthMergeExecuteRequest{TargetUserID: targetID, Confirm: true, ResolvedBy: "test"})
	if !errors.Is(err, ErrPersonalPointMergeActiveReservation) {
		t.Fatalf("execute error=%v, want active reservation conflict", err)
	}
	var targetRawAfter, sourceRawAfter, requestRawAfter string
	if err := db.QueryRowContext(ctx, `SELECT raw::text FROM xz_users WHERE id=$1`, targetID).Scan(&targetRawAfter); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT raw::text FROM xz_users WHERE id=$1`, sourceID).Scan(&sourceRawAfter); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT raw::text FROM xz_auth_account_merge_requests WHERE id=$1`, mergeID).Scan(&requestRawAfter); err != nil {
		t.Fatal(err)
	}
	if targetRawAfter != targetRawBefore || sourceRawAfter != sourceRawBefore || requestRawAfter != requestRawBefore {
		t.Fatal("rejected PostgreSQL auth merge changed user or request state")
	}
	var available, frozen int64
	if err := db.QueryRowContext(ctx, `SELECT available,frozen FROM xz_point_accounts WHERE id=$1`, sourceAccountID).Scan(&available, &frozen); err != nil {
		t.Fatal(err)
	}
	if available != 2 || frozen != 1 {
		t.Fatalf("rejected PostgreSQL auth merge changed points available=%d frozen=%d", available, frozen)
	}
}

func TestPostgresAuthMergeDoesNotRewriteExistingTargetV2Orders(t *testing.T) {
	db, ctx := openAuthMergeTestPostgres(t)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	targetID := "user_auth_merge_target_" + suffix
	sourceID := "user_auth_merge_source_" + suffix
	mergeID := "auth_merge_test_" + suffix
	targetV2OrderID := "order_auth_merge_v2_" + suffix
	priceQuoteID := "quote_auth_merge_v2_" + suffix
	legacyOrderID := "order_auth_merge_legacy_" + suffix
	now := time.Now().UTC().Format(time.RFC3339Nano)
	pricing := insertPhase2EPricePlanFixture(t, ctx, db, "TEST")
	target := adminUser{
		ID:        targetID,
		Email:     targetID + "@example.test",
		Name:      "Auth merge target",
		Role:      "MEMBER",
		Status:    "ACTIVE",
		CreatedAt: now,
		UpdatedAt: now,
	}
	source := adminUser{
		ID:        sourceID,
		Email:     sourceID + "@example.test",
		Mobile:    fmt.Sprintf("139%08d", time.Now().UnixNano()%100000000),
		Name:      "Auth merge source",
		Role:      "MEMBER",
		Status:    "ACTIVE",
		CreatedAt: now,
		UpdatedAt: now,
	}
	request := adminAuthMergeRequest{
		ID:              mergeID,
		PrimaryUserID:   sourceID,
		SecondaryUserID: targetID,
		Mobile:          source.Mobile,
		ConflictCode:    "AUTH_ACCOUNT_MERGE_REQUIRED",
		Source:          "postgres_regression_test",
		Reason:          "verify target V2 orders remain untouched",
		Status:          "PENDING",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	legacyOrder := adminOrder{
		ID:            legacyOrderID,
		UserID:        sourceID,
		BuyerUserID:   sourceID,
		AmountCents:   10,
		Status:        "PAID",
		CreatedAt:     now,
		PriceSnapshot: map[string]any{"buyerUserId": sourceID, "amountCents": 10},
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range []adminUser{target, source} {
		if err := insertUser(ctx, tx, user); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		insert into xz_order_price_quotes(
			id,quote_token_hash,tenant_id,user_id,plan_id,plan_version_id,price_plan_id,
			payment_binding_id,wechat_good_id,entry_type,transaction_price_cents,
			provider_price_snapshot_cents,wechat_goods_price_cents,channel,environment,
			offer_id,wechat_product_id,payment_mode,rights_snapshot,expires_at
		) values(
			$1,$1,'tenant_default',$2,$3,$4,$5,$6,$7,'PUBLIC',20,20,20,
			'WECHAT_VIRTUAL','SANDBOX','offer',$7,'short_series_goods','{}'::jsonb,now()+interval '5 minutes'
		)
	`, priceQuoteID, targetID, pricing.planID, pricing.versionID, pricing.pricePlanID, pricing.bindingID, pricing.goodID); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
		insert into xz_orders(
			id,user_id,buyer_user_id,amount_cents,status,created_at,
			price_snapshot,raw,snapshot_version,plan_version_id,price_plan_id,
			price_quote_id,transaction_price_cents,wechat_product_id_snapshot,
			wechat_goods_price_cents,payment_environment,rights_snapshot
		) values(
			$1,$2,$2,20,'PAID',$3,
			jsonb_build_object('snapshotVersion',2,'buyerUserId',$2::text,'amountCents',20),
			jsonb_build_object('id',$1::text,'userId',$2::text,'buyerUserId',$2::text,'amountCents',20,'status','PAID'),
			2,$4,$5,$6,20,$7,20,'SANDBOX','{}'::jsonb
		)
	`, targetV2OrderID, targetID, now, pricing.versionID, pricing.pricePlanID, priceQuoteID, pricing.goodID); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := insertOrder(ctx, tx, legacyOrder); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := insertAuthMergeRequest(ctx, tx, request); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var targetV2SnapshotBefore, targetV2RawBefore string
	if err := db.QueryRowContext(ctx, `
		select price_snapshot::text, raw::text
		from xz_orders
		where id = $1
	`, targetV2OrderID).Scan(&targetV2SnapshotBefore, &targetV2RawBefore); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := contextWithPostgresTestTimeout()
		defer cancel()
		cleanupTx, cleanupErr := db.BeginTx(cleanupCtx, nil)
		if cleanupErr != nil {
			t.Errorf("begin cleanup: %v", cleanupErr)
			return
		}
		defer func() { _ = cleanupTx.Rollback() }()
		for _, statement := range []struct {
			query string
			args  []any
		}{
			{`delete from xz_audit_logs where resource = 'auth_merge_request' and resource_id = $1`, []any{mergeID}},
			{`delete from xz_orders where id in ($1,$2)`, []any{legacyOrderID, targetV2OrderID}},
			{`delete from xz_order_price_quotes where id = $1`, []any{priceQuoteID}},
			{`delete from xz_auth_account_merge_requests where id = $1`, []any{mergeID}},
			{`delete from xz_users where id in ($1,$2)`, []any{sourceID, targetID}},
		} {
			if _, err := cleanupTx.ExecContext(cleanupCtx, statement.query, statement.args...); err != nil {
				t.Errorf("cleanup fixture: %v", err)
				return
			}
		}
		if err := cleanupTx.Commit(); err != nil {
			t.Errorf("commit cleanup: %v", err)
		}
	})

	store := &postgresStore{db: db, ready: true}
	updated, result, err := store.ExecuteAdminAuthMergeRequest(mergeID, adminAuthMergeExecuteRequest{
		TargetUserID:  target.ID,
		Confirm:       true,
		ReviewComment: "PostgreSQL regression test",
		ResolvedBy:    "test",
	})
	if err != nil {
		t.Fatalf("merge should leave existing target V2 orders untouched: %v", err)
	}
	if updated.Status != "RESOLVED" {
		t.Fatalf("merge request status = %q, want RESOLVED", updated.Status)
	}
	if result.Moved["orders"] != 1 {
		t.Fatalf("moved orders = %d, want 1", result.Moved["orders"])
	}

	var legacyUserID, legacyBuyerUserID string
	if err := db.QueryRowContext(ctx, `select user_id, buyer_user_id from xz_orders where id = $1`, legacyOrderID).Scan(&legacyUserID, &legacyBuyerUserID); err != nil {
		t.Fatal(err)
	}
	if legacyUserID != target.ID || legacyBuyerUserID != target.ID {
		t.Fatalf("legacy order identity = (%q, %q), want target %q", legacyUserID, legacyBuyerUserID, target.ID)
	}

	var targetV2SnapshotAfter, targetV2RawAfter string
	if err := db.QueryRowContext(ctx, `
		select price_snapshot::text, raw::text
		from xz_orders
		where id = $1
	`, targetV2OrderID).Scan(&targetV2SnapshotAfter, &targetV2RawAfter); err != nil {
		t.Fatal(err)
	}
	if targetV2SnapshotAfter != targetV2SnapshotBefore || targetV2RawAfter != targetV2RawBefore {
		t.Fatal("existing target V2 order was rewritten during auth merge")
	}
}

func openAuthMergeTestPostgres(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("XIANZHI_AUTH_MERGE_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("XIANZHI_AUTH_MERGE_TEST_DATABASE_URL is not configured")
	}
	if dsn != authMergeTestDSN {
		t.Fatalf("auth merge PostgreSQL tests refuse non-isolated DSN %q", dsn)
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

func contextWithPostgresTestTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 15*time.Second)
}
