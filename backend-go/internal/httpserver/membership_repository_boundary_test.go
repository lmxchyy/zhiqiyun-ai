package httpserver

import (
	"os"
	"strings"
	"testing"
)

func TestManualMembershipHandlerDoesNotOwnSubscriptionProjectionSQL(t *testing.T) {
	raw, err := os.ReadFile("admin_manual_entitlements.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "INSERT INTO xz_billing_subscriptions") {
		t.Fatal("manual membership handler must delegate subscription projection persistence")
	}
	for _, sql := range []string{
		"SELECT id,coalesce(code,''),coalesce(name,''),coalesce(plan_type,''),coalesce(member_level,''),",
		"UPDATE xz_users SET plan_id=",
		"INSERT INTO xz_membership_entitlement_records",
		"INSERT INTO xz_operation_logs",
		"INSERT INTO xz_audit_logs",
	} {
		if strings.Contains(string(raw), sql) {
			t.Fatalf("manual membership handler must delegate %q to membership repository", sql)
		}
	}
}
