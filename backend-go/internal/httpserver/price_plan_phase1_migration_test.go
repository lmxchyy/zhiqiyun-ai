package httpserver

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPricePlanPhase1Migration097IsAdditiveAndComplete(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "database", "migrations", "097-member-agent-price-plan-v2.sql"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, table := range []string{
		"xz_plan_versions", "xz_price_plans", "xz_wechat_virtual_goods",
		"xz_price_plan_payment_bindings", "xz_price_plan_user_whitelist", "xz_order_price_quotes",
	} {
		if !strings.Contains(sql, "create table if not exists "+table) {
			t.Fatalf("migration does not create %s", table)
		}
	}
	for _, column := range []string{
		"plan_version_id", "price_plan_id", "snapshot_version", "transaction_price_cents",
		"wechat_product_id_snapshot", "wechat_goods_price_cents", "payment_environment",
	} {
		if !strings.Contains(sql, "add column if not exists "+column) {
			t.Fatalf("migration does not add xz_orders.%s", column)
		}
	}
	for _, forbidden := range []string{"drop table", "drop column", "delete from", "truncate ", "update xz_orders", "insert into xz_price_plans"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration contains forbidden data/destructive operation %q", forbidden)
		}
	}
}
