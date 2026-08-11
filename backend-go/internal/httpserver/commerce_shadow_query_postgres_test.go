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

func TestCommerceShadowDifferenceQueryPostgres(t *testing.T) {
	dsn := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	id := "shadow_query_" + suffix
	orderID := "shadow_order_" + suffix
	orderNo := "SHADOW" + suffix
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_commercial_shadow_differences (
		  id,tenant_id,order_id,order_no,plan_id,scenario_code,
		  shadow_rule_set_id,shadow_rule_set_version,shadow_version,
		  comparison_status,legacy_result,v132_result,difference,relationship_snapshot
		) VALUES (
		  $1,'tenant_default',$2,$3,'plan_ai_creator_996','MEMBER_PURCHASE',
		  'channel_rules_v132_default_v1',1,'V1.3.2','DIFFERENT',
		  '{"platformIncomeCents":4600}'::jsonb,
		  '{"platformAmountCents":9600}'::jsonb,
		  '{"platformDeltaCents":5000}'::jsonb,
		  '{"directAgentId":"agent-1"}'::jsonb
		)
	`, id, orderID, orderNo); err != nil {
		t.Fatal(err)
	}

	store := &postgresStore{db: db, ready: true}
	items, total, err := store.ListCommerceShadowDifferences(ctx, commerceShadowDifferenceQuery{
		TenantID: "tenant_default", Status: "DIFFERENT", OrderKeyword: orderNo,
		Limit: 20, Offset: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != id {
		t.Fatalf("unexpected shadow list: total=%d items=%+v", total, items)
	}
	detail, err := store.GetCommerceShadowDifference(ctx, "tenant_default", id)
	if err != nil {
		t.Fatal(err)
	}
	if detail.OrderID != orderID || detail.ShadowRuleSetVersion != 1 || detail.ComparisonStatus != "DIFFERENT" {
		t.Fatalf("unexpected shadow detail: %+v", detail)
	}
	config, err := store.GetChannelRolloutConfig(ctx, "tenant_default")
	if err != nil {
		t.Fatal(err)
	}
	if config.Mode != "SHADOW" || config.RealSwitchEnabled || config.PinnedRuleSetID != "channel_rules_v132_default_v1" {
		t.Fatalf("unexpected rollout config: %+v", config)
	}
}
