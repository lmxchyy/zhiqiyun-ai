package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminCommerceShadowDifferencePermissions(t *testing.T) {
	for _, path := range []string{
		"/api/v1/admin/channel-ecosystem/shadow-differences",
		"/api/v1/admin/channel-ecosystem/shadow-differences/shadow-1",
		"/api/v1/admin/channel-ecosystem/rollout-config",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		if got := adminPermissionForRequest(request); got != "finance:commission-rule:view" {
			t.Fatalf("GET %s permission=%s", path, got)
		}
	}
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/channel-ecosystem/rollout-config", nil)
	if got := adminPermissionForRequest(request); got != "finance:commission-rule:manage" {
		t.Fatalf("PUT rollout permission=%s", got)
	}
}

func TestCommerceShadowDifferenceQueryNormalizesFilters(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/channel-ecosystem/shadow-differences?status=different&page=2&pageSize=500&orderKeyword=ORDER-1", nil)
	query := commerceShadowDifferenceQueryFromRequest(request, "tenant_default")
	if query.Status != "DIFFERENT" || query.Limit != 200 || query.Offset != 200 || query.OrderKeyword != "ORDER-1" {
		t.Fatalf("unexpected query: %+v", query)
	}
}
