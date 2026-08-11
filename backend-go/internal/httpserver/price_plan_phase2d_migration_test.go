package httpserver

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPricePlanPhase2DMigration099AddsDefaultSwitchGovernance(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "database", "migrations", "099-price-plan-default-switch.sql"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))

	for _, required := range []string{
		"add column if not exists currency",
		"add column if not exists audience_type",
		"ux_xz_price_plans_default_currency_099",
		"idx_xz_order_price_quotes_price_plan_099",
		"xz_guard_price_plan_099",
		"xz_guard_order_v2_snapshot_099",
		"trg_xz_orders_v2_snapshot_guard_099",
		"order_v2_snapshot_immutable",
		"old.snapshot_version = 2",
		"new.snapshot_version = 2 and old.snapshot_version is distinct from 2",
		"new.price_snapshot is distinct from old.price_snapshot",
		"new.rights_snapshot is distinct from old.rights_snapshot",
		"new.buyer_user_id is distinct from old.buyer_user_id",
		"new.wechat_product_id_snapshot is distinct from old.wechat_product_id_snapshot",
		"xz_price_plan_payment_bindings",
		"xz_order_price_quotes",
		"price_plan_clone_required",
		"created_by is null",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration is missing %q", required)
		}
	}

	for _, forbidden := range []string{
		"drop table",
		"drop column",
		"truncate ",
		"delete from",
		"update xz_price_plans set",
		"insert into xz_price_plans",
		"insert into xz_wechat_virtual_goods",
		"insert into xz_price_plan_payment_bindings",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration contains forbidden destructive or business-data operation %q", forbidden)
		}
	}
}
