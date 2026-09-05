package httpserver

import (
	"net/http"
	"testing"
)

func TestPR4ARecoveryPermissionMapping(t *testing.T) {
	viewReq, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/generation-tasks/task_1/recovery-diagnosis", nil)
	if got := adminPermissionForRequest(viewReq); got != "generation:recovery:view" {
		t.Fatalf("diagnosis permission=%q", got)
	}
	actionReq, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/generation-tasks/task_1/recovery-actions", nil)
	if got := adminPermissionForRequest(actionReq); got != "generation:recovery:manage" {
		t.Fatalf("action permission=%q", got)
	}
}
