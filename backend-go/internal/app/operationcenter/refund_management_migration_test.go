package operationcenter

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRefundManagementMigration096StaticContract(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "database", "migrations", "096-operation-center-refund-management.sql"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{
		"xz_operation_center_refund_request_events", "xz_operation_center_manual_refund_events",
		"ck_xz_oc_refund_tasks_manual_submitted_evidence_096", "ck_xz_oc_refund_tasks_success_evidence_096",
		"idx_xz_oc_refund_tasks_retry_scheduler_096", "idx_xz_oc_refund_tasks_verify_scheduler_096",
		"DEFERRABLE INITIALLY DEFERRED", "refund_status = 'REFUND_RETRYABLE'", "refund_status = 'UNKNOWN_VERIFYING'",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("096 missing %q", required)
		}
	}
	if strings.Contains(sql, "DROP TABLE") || strings.Contains(sql, "TRUNCATE") {
		t.Fatal("096 contains destructive data operation")
	}
}
