package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

func TestPostgresAuthMergeDoesNotRewriteExistingTargetV2Orders(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)

	var target adminUser
	var targetV2OrderID string
	if err := db.QueryRowContext(ctx, `
		select users.raw, users.id, orders.id
		from xz_orders orders
		join xz_users users on users.id = orders.user_id
		where orders.snapshot_version = 2
		  and users.status = 'ACTIVE'
		  and users.raw->>'id' = users.id
		  and orders.raw->>'id' = orders.id
		  and not exists (select 1 from xz_channel_agents agents where agents.user_id = users.id)
		  and not exists (select 1 from xz_operation_centers centers where centers.user_id = users.id)
		order by orders.created_at desc, orders.id desc
		limit 1
	`).Scan(rawScanner(&target), &target.ID, &targetV2OrderID); err != nil {
		if err == sql.ErrNoRows {
			t.Skip("isolated PostgreSQL fixture has no eligible V2 target order")
		}
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

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	sourceID := "user_auth_merge_source_" + suffix
	mergeID := "auth_merge_test_" + suffix
	legacyOrderID := "order_auth_merge_legacy_" + suffix
	now := time.Now().UTC().Format(time.RFC3339Nano)
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
		SecondaryUserID: target.ID,
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
	if err := insertUser(ctx, tx, source); err != nil {
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

	t.Cleanup(func() {
		cleanupCtx, cancel := contextWithPostgresTestTimeout()
		defer cancel()
		cleanupTx, cleanupErr := db.BeginTx(cleanupCtx, nil)
		if cleanupErr != nil {
			t.Errorf("begin cleanup: %v", cleanupErr)
			return
		}
		defer func() { _ = cleanupTx.Rollback() }()
		if err := insertUser(cleanupCtx, cleanupTx, target); err != nil {
			t.Errorf("restore target user: %v", err)
			return
		}
		for _, statement := range []struct {
			query string
			args  []any
		}{
			{`delete from xz_audit_logs where resource = 'auth_merge_request' and resource_id = $1`, []any{mergeID}},
			{`delete from xz_orders where id = $1`, []any{legacyOrderID}},
			{`delete from xz_auth_account_merge_requests where id = $1`, []any{mergeID}},
			{`delete from xz_users where id = $1`, []any{sourceID}},
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

func contextWithPostgresTestTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 15*time.Second)
}
