package operationcenter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefundAdapterMigration095StaticContract(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "database")
	raw, err := os.ReadFile(filepath.Join(root, "migrations", "095-payment-refund-adapter-query-verification.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{"provider_response_summary", "provider_query_outcome", "verification_attempt_count", "UNKNOWN_VERIFYING", "ck_xz_oc_refund_query_outcome_095"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("095 migration missing %q", required)
		}
	}

	rollbackRaw, err := os.ReadFile(filepath.Join(root, "rollbacks", "095-payment-refund-adapter-query-verification.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	rollbackSQL := string(rollbackRaw)
	for _, required := range []string{
		"DROP INDEX IF EXISTS idx_xz_oc_refund_unknown_verification_095",
		"DROP CONSTRAINT IF EXISTS ck_xz_oc_refund_provider_summaries_095",
		"DROP CONSTRAINT IF EXISTS ck_xz_oc_refund_query_outcome_095",
		"DROP CONSTRAINT IF EXISTS ck_xz_oc_refund_verification_count_095",
		"DROP COLUMN IF EXISTS provider_response_summary",
		"DROP COLUMN IF EXISTS provider_query_outcome",
		"DROP COLUMN IF EXISTS provider_query_response_summary",
		"DROP COLUMN IF EXISTS verification_attempt_count",
		"DROP COLUMN IF EXISTS last_verification_at",
	} {
		if !strings.Contains(rollbackSQL, required) {
			t.Fatalf("095 rollback missing %q", required)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "DROP COLUMN IF EXISTS refund_status", "DROP COLUMN IF EXISTS unknown_since"} {
		if strings.Contains(rollbackSQL, forbidden) {
			t.Fatalf("095 rollback crosses the 094 boundary with %q", forbidden)
		}
	}
}
