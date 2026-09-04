package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnalyticsScope_FailClosed(t *testing.T) {
	scope := FailClosedScope("test-user")
	if !scope.IsFailClosed() {
		t.Fatalf("expected scope to be fail closed")
	}

	clause, args, _ := scope.ScopeSQLFilter("xz_generation_tasks", 1)
	if clause != "1=0" || len(args) != 0 {
		t.Fatalf("expected 1=0 for fail closed scope, got %q with args %v", clause, args)
	}

	clause, args, _ = scope.ScopeSQLFilter("xz_billing_events", 1)
	if clause != "1=0" || len(args) != 0 {
		t.Fatalf("expected 1=0 for fail closed billing, got %q with args %v", clause, args)
	}
}

func TestAnalyticsScope_PlatformUnrestricted(t *testing.T) {
	scope := PlatformScope("admin-user")
	if scope.IsFailClosed() {
		t.Fatalf("expected platform scope not to be fail closed")
	}
	if !scope.IsPlatform {
		t.Fatalf("expected isPlatform to be true")
	}

	clause, args, _ := scope.ScopeSQLFilter("xz_generation_tasks", 1)
	if clause != "1=1" || len(args) != 0 {
		t.Fatalf("expected 1=1 for platform scope, got %q with args %v", clause, args)
	}
}

func TestAnalyticsScope_TenantBoundaries(t *testing.T) {
	scope := AnalyticsScope{
		Level:      ScopeTenant,
		TenantIDs:  []string{"tenant-a"},
		IsPlatform: false,
	}
	if scope.IsFailClosed() {
		t.Fatalf("valid tenant scope should not fail closed")
	}

	clause, args, next := scope.ScopeSQLFilter("xz_generation_tasks", 1)
	if clause != "(tenant_id = ANY($1))" {
		t.Fatalf("unexpected tenant clause: %q", clause)
	}
	if len(args) != 1 || next != 2 {
		t.Fatalf("unexpected args: %v, next: %d", args, next)
	}
}

func TestAnalyticsScope_AgentBoundaries(t *testing.T) {
	scope := AnalyticsScope{
		Level:      ScopeAgent,
		AgentIDs:   []string{"agent-1", "agent-2"},
		IsPlatform: false,
	}
	clause, args, next := scope.ScopeSQLFilter("xz_generation_tasks", 1)
	if clause != "(agent_id = ANY($1))" {
		t.Fatalf("unexpected agent clause: %q", clause)
	}
	if len(args) != 1 || next != 2 {
		t.Fatalf("unexpected args: %v, next: %d", args, next)
	}
}

func TestAnalyticsScope_OperationCenterBoundaries(t *testing.T) {
	scope := AnalyticsScope{
		Level:              ScopeOperationCenter,
		OperationCenterIDs: []string{"oc-alpha"},
		IsPlatform:         false,
	}
	clause, args, next := scope.ScopeSQLFilter("xz_generation_tasks", 1)
	if clause != "(operation_center_id = ANY($1))" {
		t.Fatalf("unexpected oc clause: %q", clause)
	}
	if len(args) != 1 || next != 2 {
		t.Fatalf("unexpected args: %v, next: %d", args, next)
	}
}

func TestAnalyticsScope_ClientForgeryIgnored(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/analytics/overview?days=7&tenant_id=malicious-tenant&agent_id=stolen-agent&operation_center_id=fake-oc", nil)
	params := parseAnalyticsQuery(req.URL.Query())

	if params.Days != 7 {
		t.Fatalf("expected 7 days, got %d", params.Days)
	}
	if params.Scope.IsPlatform || len(params.Scope.TenantIDs) > 0 || len(params.Scope.AgentIDs) > 0 {
		t.Fatalf("client injected params must not populate Scope: %+v", params.Scope)
	}
}

func TestAnalyticsScope_EmptyScopeDoesNotFallBackToPlatform(t *testing.T) {
	// A scope explicitly marked fail-closed
	fc := FailClosedScope("test-user")
	if !fc.IsFailClosed() {
		t.Fatalf("expected scope to be fail-closed")
	}

	clause, _, _ := fc.ScopeSQLFilter("xz_generation_tasks", 1)
	if clause != "1=0" {
		t.Fatalf("fail-closed scope must resolve to 1=0, got %q", clause)
	}
}

func TestAnalyticsScope_BuildScopedUserFilter(t *testing.T) {
	// Platform
	platClause, platArgs, _ := buildScopedUserFilter(PlatformScope("root"), 1)
	if platClause != "role <> 'SUPER_ADMIN'" || len(platArgs) != 0 {
		t.Fatalf("unexpected platform user clause: %q", platClause)
	}

	// FailClosed
	fcClause, fcArgs, _ := buildScopedUserFilter(FailClosedScope("guest"), 1)
	if fcClause != "1=0" || len(fcArgs) != 0 {
		t.Fatalf("unexpected fail closed user clause: %q", fcClause)
	}

	// Tenant
	tScope := AnalyticsScope{Level: ScopeTenant, TenantIDs: []string{"t1"}}
	tClause, tArgs, _ := buildScopedUserFilter(tScope, 1)
	if tClause != "(id IN (SELECT user_id FROM xz_tenant_members WHERE tenant_id = ANY($1)))" {
		t.Fatalf("unexpected tenant user filter: %q", tClause)
	}
	if len(tArgs) != 1 {
		t.Fatalf("expected 1 arg for tenant filter, got %v", tArgs)
	}
}
