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
}
