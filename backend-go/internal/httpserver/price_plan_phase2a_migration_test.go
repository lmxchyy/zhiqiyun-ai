package httpserver

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPricePlanPhase2AMigration098IsAdditiveAndGoverned(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "database", "migrations", "098-price-plan-admin-governance.sql"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))

	for _, required := range []string{
		"add column if not exists revision",
		"add column if not exists created_by",
		"add column if not exists updated_by",
		"add column if not exists verification_status",
		"add column if not exists verified_by",
		"add column if not exists verified_at",
		"add column if not exists verification_reason",
		"add column if not exists verification_evidence",
		"add column if not exists verification_snapshot",
		"ux_xz_plans_code_098",
		"ux_xz_plan_versions_one_active_098",
		"xz_enforce_plan_code_governance_098",
		"xz_guard_plan_version_098",
		"xz_guard_price_plan_098",
		"xz_touch_price_plan_admin_record_098",
		"manually_confirmed_published",
		"verification_expired",
		"price_mismatch",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration is missing %q", required)
		}
	}

	for _, permission := range []string{
		"pricing:plan:view",
		"pricing:entitlement:manage",
		"pricing:price-plan:manage",
		"pricing:price-plan:default",
		"pricing:wechat-good:manage",
		"pricing:test-whitelist:manage",
		"pricing:audit:view",
	} {
		if !strings.Contains(sql, permission) {
			t.Fatalf("migration does not register permission %q", permission)
		}
	}

	for _, forbidden := range []string{
		"drop table",
		"drop column",
		"truncate ",
		"delete from",
		"update xz_plans set",
		"insert into xz_plans",
		"insert into xz_plan_versions",
		"insert into xz_price_plans",
		"insert into xz_wechat_virtual_goods",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration contains forbidden destructive or business-data operation %q", forbidden)
		}
	}
}
