package httpserver

import (
	"os"
	"strings"
	"testing"
)

func TestAdminPointGiftDoesNotOwnLotExpirySQL(t *testing.T) {
	raw, err := os.ReadFile("admin_manual_entitlements.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "UPDATE xz_personal_point_lots") {
		t.Fatal("admin point gift handler must delegate lot expiry persistence to the Points repository")
	}
}
