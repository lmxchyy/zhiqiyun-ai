package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPricePlanAdminPermissionMapping(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodGet, "/api/v1/admin/business-plans/plan_member/price-plans", "pricing:plan:view"},
		{http.MethodPost, "/api/v1/admin/business-plans/plan_member/price-plans", "pricing:price-plan:manage"},
		{http.MethodGet, "/api/v1/admin/price-plans/price_normal", "pricing:plan:view"},
		{http.MethodPatch, "/api/v1/admin/price-plans/price_normal", "pricing:price-plan:manage"},
		{http.MethodPost, "/api/v1/admin/price-plans/price_normal/clone", "pricing:price-plan:manage"},
		{http.MethodGet, "/api/v1/admin/price-plans/price_normal/validation", "pricing:plan:view"},
		{http.MethodPost, "/api/v1/admin/price-plans/price_normal/enable", "pricing:price-plan:manage"},
		{http.MethodPost, "/api/v1/admin/price-plans/price_normal/disable", "pricing:price-plan:manage"},
		{http.MethodPost, "/api/v1/admin/price-plans/price_normal/make-default", "pricing:price-plan:default"},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			if got := adminPermissionForRequest(request); got != test.want {
				t.Fatalf("permission=%q want=%q", got, test.want)
			}
		})
	}
}
