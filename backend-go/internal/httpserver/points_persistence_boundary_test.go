package httpserver

import (
	"os"
	"strings"
	"testing"
)

func TestLegacyPointStoreDoesNotOwnPointWriteSQL(t *testing.T) {
	raw, err := os.ReadFile("personal_points_postgres.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, forbidden := range []string{
		"INSERT INTO xz_personal_point_lots",
		"UPDATE xz_personal_point_lots",
		"INSERT INTO xz_wallet_ledger",
		"INSERT INTO xz_personal_point_lot_movements",
		"UPDATE xz_point_accounts",
		"UPDATE xz_personal_point_reservation_allocations",
		"UPDATE xz_personal_point_reservations",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("legacy point store must delegate %q to internal/points/repository", forbidden)
		}
	}
}
