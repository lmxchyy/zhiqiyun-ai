package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestPhase2EPricingPermissionMappingSeparatesReadsAndSensitiveWrites(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodGet, "/api/v1/admin/business-plans", "pricing:plan:view"},
		{http.MethodGet, "/api/v1/admin/pricing-health", "pricing:plan:view"},
		{http.MethodGet, "/api/v1/admin/business-plans/plan_member", "pricing:plan:view"},
		{http.MethodGet, "/api/v1/admin/business-plans/plan_member/versions", "pricing:plan:view"},
		{http.MethodPost, "/api/v1/admin/business-plans/plan_member/versions", "pricing:entitlement:manage"},
		{http.MethodPatch, "/api/v1/admin/plan-versions/version_member_v2", "pricing:entitlement:manage"},
		{http.MethodPost, "/api/v1/admin/plan-versions/version_member_v2/activate", "pricing:entitlement:manage"},
		{http.MethodPost, "/api/v1/admin/plan-versions/version_member_v2/retire", "pricing:entitlement:manage"},
		{http.MethodGet, "/api/v1/admin/business-plans/plan_member/price-plans", "pricing:plan:view"},
		{http.MethodPost, "/api/v1/admin/business-plans/plan_member/price-plans", "pricing:price-plan:manage"},
		{http.MethodGet, "/api/v1/admin/price-plans/price_normal", "pricing:plan:view"},
		{http.MethodPatch, "/api/v1/admin/price-plans/price_normal", "pricing:price-plan:manage"},
		{http.MethodPost, "/api/v1/admin/price-plans/price_normal/clone", "pricing:price-plan:manage"},
		{http.MethodGet, "/api/v1/admin/price-plans/price_normal/validation", "pricing:plan:view"},
		{http.MethodPost, "/api/v1/admin/price-plans/price_normal/enable", "pricing:price-plan:manage"},
		{http.MethodPost, "/api/v1/admin/price-plans/price_normal/disable", "pricing:price-plan:manage"},
		{http.MethodPost, "/api/v1/admin/price-plans/price_normal/make-default", "pricing:price-plan:default"},
		{http.MethodGet, "/api/v1/admin/price-plans/price_test/whitelist", "pricing:plan:view"},
		{http.MethodPost, "/api/v1/admin/price-plans/price_test/whitelist", "pricing:test-whitelist:manage"},
		{http.MethodPatch, "/api/v1/admin/price-plans/price_test/whitelist/entry_1", "pricing:test-whitelist:manage"},
		{http.MethodPost, "/api/v1/admin/price-plans/price_test/whitelist/entry_1/disable", "pricing:test-whitelist:manage"},
		{http.MethodGet, "/api/v1/admin/wechat-virtual-goods", "pricing:plan:view"},
		{http.MethodGet, "/api/v1/admin/wechat-virtual-goods/good_1", "pricing:plan:view"},
		{http.MethodGet, "/api/v1/admin/wechat-virtual-goods/good_1/references", "pricing:plan:view"},
		{http.MethodPost, "/api/v1/admin/wechat-virtual-goods", "pricing:wechat-good:manage"},
		{http.MethodPatch, "/api/v1/admin/wechat-virtual-goods/good_1", "pricing:wechat-good:manage"},
		{http.MethodPost, "/api/v1/admin/wechat-virtual-goods/good_1/confirm-published", "pricing:wechat-good:manage"},
		{http.MethodPost, "/api/v1/admin/wechat-virtual-goods/good_1/disable", "pricing:wechat-good:manage"},
		{http.MethodGet, "/api/v1/admin/price-plans/price_normal/payment-bindings", "pricing:plan:view"},
		{http.MethodPost, "/api/v1/admin/price-plans/price_normal/payment-bindings", "pricing:price-plan:manage"},
		{http.MethodPatch, "/api/v1/admin/payment-bindings/binding_1", "pricing:price-plan:manage"},
		{http.MethodGet, "/api/v1/admin/pricing-audit-logs", "pricing:audit:view"},
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

func TestPhase2EPricingRBACPostgresRoleMatrixAndStableErrors(t *testing.T) {
	t.Setenv("XIANZHI_TEST_DATABASE_URL", phase2ETestDSN)
	t.Setenv("XIANZHI_APPLY_TEST_MIGRATION_100", "true")
	t.Setenv("XIANZHI_ENFORCE_RBAC", "true")
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	type identity struct {
		role  string
		user  string
		token string
	}
	identities := map[string]identity{
		"readonly":    {role: "PRICING_READONLY_" + suffix, user: "pricing_readonly_" + suffix, token: "pricing-readonly-token-" + suffix},
		"operator":    {role: "PRICING_OPERATOR_" + suffix, user: "pricing_operator_" + suffix, token: "pricing-operator-token-" + suffix},
		"entitlement": {role: "PRICING_ENTITLEMENT_" + suffix, user: "pricing_entitlement_" + suffix, token: "pricing-entitlement-token-" + suffix},
		"owner":       {role: "PRICING_OWNER_" + suffix, user: "pricing_owner_" + suffix, token: "pricing-owner-token-" + suffix},
		"wechat":      {role: "PRICING_WECHAT_" + suffix, user: "pricing_wechat_" + suffix, token: "pricing-wechat-token-" + suffix},
		"test":        {role: "PRICING_TEST_" + suffix, user: "pricing_test_" + suffix, token: "pricing-test-token-" + suffix},
		"legacy_full": {role: "LEGACY_ADMIN_FULL_" + suffix, user: "pricing_legacy_full_" + suffix, token: "pricing-legacy-full-token-" + suffix},
		"admin":       {role: "ADMIN", user: "pricing_plain_admin_" + suffix, token: "pricing-admin-token-" + suffix},
		"super_admin": {role: "SUPER_ADMIN", user: "pricing_super_admin_" + suffix, token: "pricing-super-admin-token-" + suffix},
	}
	permissions := map[string][]string{
		"readonly":    {"pricing:plan:view", "pricing:audit:view"},
		"operator":    {"pricing:plan:view", "pricing:price-plan:manage"},
		"entitlement": {"pricing:plan:view", "pricing:entitlement:manage"},
		"owner":       {"pricing:plan:view", "pricing:price-plan:manage", "pricing:price-plan:default"},
		"wechat":      {"pricing:plan:view", "pricing:wechat-good:manage"},
		"test":        {"pricing:plan:view", "pricing:test-whitelist:manage"},
		"legacy_full": {"admin.full"},
	}

	for _, item := range identities {
		if _, err := db.ExecContext(ctx, `
			insert into xz_users(id,email,name,role,status,raw)
			values($1,$1||'@example.test',$1,$2,'ACTIVE',jsonb_build_object(
				'id',$1::text,'email',$1::text||'@example.test','name',$1::text,'role',$2::text,'status','ACTIVE'
			))
		`, item.user, item.role); err != nil {
			t.Fatal(err)
		}
	}
	for name, rolePermissions := range permissions {
		for _, permission := range rolePermissions {
			if _, err := db.ExecContext(ctx, `insert into xz_role_permissions(role,permission) values($1,$2)`, identities[name].role, permission); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Cleanup(func() {
		for name, item := range identities {
			_, _ = db.Exec(`delete from xz_users where id=$1`, item.user)
			if name != "admin" && name != "super_admin" {
				_, _ = db.Exec(`delete from xz_role_permissions where role=$1`, item.role)
			}
		}
	})

	sessions := newLocalAuthSessions()
	for _, item := range identities {
		if err := sessions.Put(ctx, item.token, item.user, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	store := &postgresStore{db: db, ready: true}
	auth := newAuthAPI(store, sessions)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		permission := adminPermissionForRequest(c.Request)
		store.rbacMiddleware(auth, permission)(c)
	})
	router.Any("/api/v1/admin/*path", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	tests := []struct {
		name   string
		actor  string
		method string
		path   string
		status int
	}{
		{"readonly views plans", "readonly", http.MethodGet, "/api/v1/admin/business-plans", http.StatusNoContent},
		{"readonly views pricing health", "readonly", http.MethodGet, "/api/v1/admin/pricing-health", http.StatusNoContent},
		{"readonly views audit", "readonly", http.MethodGet, "/api/v1/admin/pricing-audit-logs", http.StatusNoContent},
		{"readonly cannot edit entitlements", "readonly", http.MethodPost, "/api/v1/admin/business-plans/plan_member/versions", http.StatusForbidden},
		{"readonly cannot edit prices", "readonly", http.MethodPost, "/api/v1/admin/business-plans/plan_member/price-plans", http.StatusForbidden},
		{"operator cannot edit entitlements", "operator", http.MethodPost, "/api/v1/admin/business-plans/plan_member/versions", http.StatusForbidden},
		{"operator edits prices", "operator", http.MethodPost, "/api/v1/admin/business-plans/plan_member/price-plans", http.StatusNoContent},
		{"operator edits bindings", "operator", http.MethodPost, "/api/v1/admin/price-plans/price_normal/payment-bindings", http.StatusNoContent},
		{"operator cannot switch default", "operator", http.MethodPost, "/api/v1/admin/price-plans/price_normal/make-default", http.StatusForbidden},
		{"operator cannot view audit", "operator", http.MethodGet, "/api/v1/admin/pricing-audit-logs", http.StatusForbidden},
		{"owner switches default", "owner", http.MethodPost, "/api/v1/admin/price-plans/price_normal/make-default", http.StatusNoContent},
		{"entitlement manager edits entitlements", "entitlement", http.MethodPost, "/api/v1/admin/business-plans/plan_member/versions", http.StatusNoContent},
		{"entitlement manager cannot edit prices", "entitlement", http.MethodPatch, "/api/v1/admin/price-plans/price_normal", http.StatusForbidden},
		{"owner edits prices", "owner", http.MethodPatch, "/api/v1/admin/price-plans/price_normal", http.StatusNoContent},
		{"wechat owner views goods", "wechat", http.MethodGet, "/api/v1/admin/wechat-virtual-goods", http.StatusNoContent},
		{"readonly views good references", "readonly", http.MethodGet, "/api/v1/admin/wechat-virtual-goods/good_1/references", http.StatusNoContent},
		{"ordinary admin cannot view good references", "admin", http.MethodGet, "/api/v1/admin/wechat-virtual-goods/good_1/references", http.StatusForbidden},
		{"wechat owner confirms goods", "wechat", http.MethodPost, "/api/v1/admin/wechat-virtual-goods/good_1/confirm-published", http.StatusNoContent},
		{"wechat owner cannot edit prices", "wechat", http.MethodPatch, "/api/v1/admin/price-plans/price_normal", http.StatusForbidden},
		{"test owner views whitelist", "test", http.MethodGet, "/api/v1/admin/price-plans/price_test/whitelist", http.StatusNoContent},
		{"test owner edits whitelist", "test", http.MethodPost, "/api/v1/admin/price-plans/price_test/whitelist", http.StatusNoContent},
		{"test owner cannot edit prices", "test", http.MethodPatch, "/api/v1/admin/price-plans/price_test", http.StatusForbidden},
		{"ordinary admin has no implicit pricing read", "admin", http.MethodGet, "/api/v1/admin/business-plans", http.StatusForbidden},
		{"ordinary admin has no implicit pricing health", "admin", http.MethodGet, "/api/v1/admin/pricing-health", http.StatusForbidden},
		{"ordinary admin has no implicit sensitive write", "admin", http.MethodPost, "/api/v1/admin/price-plans/price_test/whitelist", http.StatusForbidden},
		{"legacy admin full does not imply pricing read", "legacy_full", http.MethodGet, "/api/v1/admin/business-plans", http.StatusForbidden},
		{"legacy admin full does not imply pricing health", "legacy_full", http.MethodGet, "/api/v1/admin/pricing-health", http.StatusForbidden},
		{"legacy admin full does not imply pricing write", "legacy_full", http.MethodPatch, "/api/v1/admin/price-plans/price_normal", http.StatusForbidden},
		{"legacy admin full remains valid outside pricing", "legacy_full", http.MethodGet, "/api/v1/admin/overview", http.StatusNoContent},
		{"super admin views audit without explicit lookup dependency", "super_admin", http.MethodGet, "/api/v1/admin/pricing-audit-logs", http.StatusNoContent},
		{"super admin switches default", "super_admin", http.MethodPost, "/api/v1/admin/price-plans/price_normal/make-default", http.StatusNoContent},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := phase2ERBACRequest(router, test.method, test.path, identities[test.actor].token)
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s want=%d", response.Code, response.Body.String(), test.status)
			}
			if test.status == http.StatusForbidden {
				requirePhase2ERBACError(t, response, "ADMIN_PERMISSION_DENIED")
			}
		})
	}

	t.Run("unauthenticated request has stable 401 payload", func(t *testing.T) {
		response := phase2ERBACRequest(router, http.MethodGet, "/api/v1/admin/business-plans", "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		requirePhase2ERBACError(t, response, "ADMIN_AUTHENTICATION_REQUIRED")
	})

	t.Run("disabled RBAC keeps the existing SUPER_ADMIN-only gate", func(t *testing.T) {
		t.Setenv("XIANZHI_ENFORCE_RBAC", "false")
		operator := phase2ERBACRequest(router, http.MethodPost, "/api/v1/admin/business-plans/plan_member/price-plans", identities["operator"].token)
		if operator.Code != http.StatusForbidden {
			t.Fatalf("operator status=%d body=%s", operator.Code, operator.Body.String())
		}
		requirePhase2ERBACError(t, operator, "ADMIN_PERMISSION_DENIED")
		superAdmin := phase2ERBACRequest(router, http.MethodPost, "/api/v1/admin/business-plans/plan_member/price-plans", identities["super_admin"].token)
		if superAdmin.Code != http.StatusNoContent {
			t.Fatalf("super admin status=%d body=%s", superAdmin.Code, superAdmin.Body.String())
		}
	})
}

func phase2ERBACRequest(handler http.Handler, method, path, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func requirePhase2ERBACError(t *testing.T, response *httptest.ResponseRecorder, code string) {
	t.Helper()
	var payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error payload: %v body=%s", err, response.Body.String())
	}
	if payload.Code != code || payload.Message == "" || payload.Error == "" {
		t.Fatalf("unstable error payload=%+v want code=%s", payload, code)
	}
}
