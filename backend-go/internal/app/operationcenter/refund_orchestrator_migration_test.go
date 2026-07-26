package operationcenter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefundOrchestratorMigration094StaticContract(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "database", "migrations", "094-operation-center-refund-saga-orchestrator.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{"provider_refunded_at", "ck_xz_oc_refund_provider_completed_094", "refund_status<>'SUCCEEDED'", "ADD COLUMN IF NOT EXISTS"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("094 migration missing %q", required)
		}
	}
}
