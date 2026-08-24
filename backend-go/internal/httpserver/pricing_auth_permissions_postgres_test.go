package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestAuthPermissionsForRoleAllowsOnlySupportedAdminPermissions(t *testing.T) {
	t.Setenv("XIANZHI_TEST_DATABASE_URL", phase2ETestDSN)
	db, ctx := openPhase2ETestPostgres(t)
	if _, err := db.ExecContext(ctx, `
		create table if not exists xz_role_permissions (
			role text not null,
			permission text not null,
			primary key(role,permission)
		)
	`); err != nil {
		t.Fatal(err)
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	role := "AUTH_POINTS_EXACT_" + suffix
	configured := []string{
		"pricing:plan:view",
		"points:gift-policy:view",
		"points:gift-policy:manage",
		"points:gift:grant",
		"points:balance:correct",
		"points:lot:view",
		"points:unlisted",
		"enterprise:tenant:view",
		"admin.full",
	}
	for _, permission := range configured {
		if _, err := db.ExecContext(ctx, `insert into xz_role_permissions(role,permission) values($1,$2)`, role, permission); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _, _ = db.Exec(`delete from xz_role_permissions where role=$1`, role) })

	permissions, err := (&postgresStore{db: db, ready: true}).AuthPermissionsForRole(ctx, role)
	if err != nil {
		t.Fatal(err)
	}
	for _, permission := range configured[:6] {
		if !stringSliceContains(permissions, permission) {
			t.Fatalf("permissions=%v missing supported=%s", permissions, permission)
		}
	}
	for _, permission := range configured[6:] {
		if stringSliceContains(permissions, permission) {
			t.Fatalf("permissions=%v exposed unsupported=%s", permissions, permission)
		}
	}
}

func TestAuthMeAndPricingRBACUseExactDatabaseRolePermissions(t *testing.T) {
	if os.Getenv("XIANZHI_TEST_DATABASE_URL") == "" {
		t.Setenv("XIANZHI_TEST_DATABASE_URL", phase2ETestDSN)
	}
	if os.Getenv("XIANZHI_APPLY_TEST_MIGRATION_100") == "" {
		t.Setenv("XIANZHI_APPLY_TEST_MIGRATION_100", "true")
	}
	t.Setenv("XIANZHI_ENFORCE_RBAC", "true")
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)
	if _, err := db.ExecContext(ctx, `
		insert into xz_tenants(id,tenant_type,name,status)
		values('tenant_default','PLATFORM','test default tenant','ACTIVE')
		on conflict(id) do nothing;
		insert into xz_organizations(id,tenant_id,organization_type,name,status)
		values('organization_default_test','tenant_default','DEPARTMENT','test default organization','ACTIVE')
		on conflict(id) do nothing
	`); err != nil {
		t.Fatal(err)
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	type fixture struct {
		role        string
		userID      string
		token       string
		password    string
		permissions []string
	}
	fixtures := map[string]fixture{
		"exact": {
			role: "PRICING_AUTH_EXACT_" + suffix, userID: "pricing_auth_exact_" + suffix, token: "pricing-auth-exact-" + suffix,
			password:    "PricingExact123!",
			permissions: []string{"admin.full", "pricing:plan:view", "pricing:price-plan:manage"},
		},
		"legacy": {
			role: "PRICING_AUTH_LEGACY_" + suffix, userID: "pricing_auth_legacy_" + suffix, token: "pricing-auth-legacy-" + suffix,
			permissions: []string{"admin.full"},
		},
		"points_policy_view": {
			role: "POINTS_POLICY_VIEW_" + suffix, userID: "points_policy_view_" + suffix, token: "points-policy-view-" + suffix,
			permissions: []string{"points:gift-policy:view", "admin.full", "points:unlisted", "enterprise:tenant:view"},
		},
		"points_policy_manage": {
			role: "POINTS_POLICY_MANAGE_" + suffix, userID: "points_policy_manage_" + suffix, token: "points-policy-manage-" + suffix,
			permissions: []string{"points:gift-policy:manage"},
		},
		"points_gift_grant": {
			role: "POINTS_GIFT_GRANT_" + suffix, userID: "points_gift_grant_" + suffix, token: "points-gift-grant-" + suffix,
			permissions: []string{"points:gift:grant"},
		},
		"points_balance_correct": {
			role: "POINTS_BALANCE_CORRECT_" + suffix, userID: "points_balance_correct_" + suffix, token: "points-balance-correct-" + suffix,
			permissions: []string{"points:balance:correct"},
		},
		"points_lot_view": {
			role: "POINTS_LOT_VIEW_" + suffix, userID: "points_lot_view_" + suffix, token: "points-lot-view-" + suffix,
			permissions: []string{"points:lot:view"},
		},
		"points_role_name_only": {
			role: "POINTS_GIFT_POLICY_MANAGE_" + suffix, userID: "points_role_name_only_" + suffix, token: "points-role-name-only-" + suffix,
		},
		"super": {
			role: "SUPER_ADMIN", userID: "pricing_auth_super_" + suffix, token: "pricing-auth-super-" + suffix,
		},
	}
	for _, item := range fixtures {
		passwordHash := ""
		if item.password != "" {
			var err error
			passwordHash, err = hashPassword(item.password)
			if err != nil {
				t.Fatal(err)
			}
		}
		if _, err := db.ExecContext(ctx, `
			insert into xz_users(id,email,name,role,status,password_hash,raw)
			values($1,$1||'@example.test',$1,$2,'ACTIVE',$3,jsonb_build_object(
				'id',$1::text,'email',$1::text||'@example.test','name',$1::text,'role',$2::text,'status','ACTIVE','passwordHash',$3::text
			))
		`, item.userID, item.role, passwordHash); err != nil {
			t.Fatal(err)
		}
		for _, permission := range item.permissions {
			if _, err := db.ExecContext(ctx, `insert into xz_role_permissions(role,permission) values($1,$2) on conflict do nothing`, item.role, permission); err != nil {
				t.Fatal(err)
			}
		}
	}
	contextRole := "POINTS_TENANT_CONTEXT_" + suffix
	if _, err := db.ExecContext(ctx, `insert into xz_role_permissions(role,permission) values($1,'points:balance:correct')`, contextRole); err != nil {
		t.Fatal(err)
	}
	var tenantID, organizationID string
	if err := db.QueryRowContext(ctx, `select tenant_id,id from xz_organizations where tenant_id='tenant_default' order by id limit 1`).Scan(&tenantID, &organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_user_role_context(user_id,tenant_id,organization_id,current_role_code,context_type,updated_at)
		values($1,$2,$3,$4,'PERSONAL',now())
	`, fixtures["points_policy_view"].userID, tenantID, organizationID, contextRole); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`delete from xz_user_role_context where user_id=$1`, fixtures["points_policy_view"].userID)
		_, _ = db.Exec(`delete from xz_role_permissions where role=$1`, contextRole)
		for name, item := range fixtures {
			_, _ = db.Exec(`delete from xz_users where id=$1`, item.userID)
			if name != "super" {
				_, _ = db.Exec(`delete from xz_role_permissions where role=$1`, item.role)
			}
		}
	})

	sessions := newLocalAuthSessions()
	for _, item := range fixtures {
		if err := sessions.Put(ctx, item.token, item.userID, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	store := &postgresStore{db: db, ready: true}
	auth := newAuthAPI(store, sessions)
	gin.SetMode(gin.TestMode)
	meRouter := gin.New()
	meRouter.GET("/api/v1/auth/me", wrapF(auth.me))

	authMe := func(item fixture) []string {
		t.Helper()
		response := authedRequest(t, meRouter, http.MethodGet, "/api/v1/auth/me", nil, item.token)
		if response.Code != http.StatusOK {
			t.Fatalf("auth/me role=%s status=%d body=%s", item.role, response.Code, response.Body.String())
		}
		var payload struct {
			User struct {
				Role string `json:"role"`
			} `json:"user"`
			Permissions []string `json:"permissions"`
		}
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.User.Role != item.role {
			t.Fatalf("auth/me role=%q want=%q", payload.User.Role, item.role)
		}
		return payload.Permissions
	}

	exactPermissions := authMe(fixtures["exact"])
	allPricingPermissions := []string{
		"pricing:plan:view",
		"pricing:entitlement:manage",
		"pricing:price-plan:manage",
		"pricing:price-plan:default",
		"pricing:wechat-good:manage",
		"pricing:test-whitelist:manage",
		"pricing:audit:view",
	}
	for _, permission := range []string{"pricing:plan:view", "pricing:price-plan:manage"} {
		if !stringSliceContains(exactPermissions, permission) {
			t.Fatalf("exact auth/me permissions=%v missing=%s", exactPermissions, permission)
		}
	}
	if stringSliceContains(exactPermissions, "pricing:price-plan:default") {
		t.Fatalf("auth/me invented unassigned pricing permission: %v", exactPermissions)
	}
	legacyPermissions := authMe(fixtures["legacy"])
	for _, permission := range allPricingPermissions {
		if stringSliceContains(legacyPermissions, permission) {
			t.Fatalf("admin.full expanded to %s in auth/me: %v", permission, legacyPermissions)
		}
	}
	superPermissions := authMe(fixtures["super"])
	for _, permission := range allPricingPermissions {
		if !stringSliceContains(superPermissions, permission) {
			t.Fatalf("SUPER_ADMIN auth/me missing bypass permission %s: %v", permission, superPermissions)
		}
	}
	allPointsPermissions := []string{
		"points:gift-policy:view",
		"points:gift-policy:manage",
		"points:gift:grant",
		"points:balance:correct",
		"points:lot:view",
	}
	pointFixtures := []struct {
		name       string
		permission string
	}{
		{"points_policy_view", "points:gift-policy:view"},
		{"points_policy_manage", "points:gift-policy:manage"},
		{"points_gift_grant", "points:gift:grant"},
		{"points_balance_correct", "points:balance:correct"},
		{"points_lot_view", "points:lot:view"},
	}
	for _, test := range pointFixtures {
		t.Run(test.name+" returns only its configured points permission", func(t *testing.T) {
			permissions := authMe(fixtures[test.name])
			for _, permission := range allPointsPermissions {
				if permission == test.permission && !stringSliceContains(permissions, permission) {
					t.Fatalf("auth/me permissions=%v missing configured=%s", permissions, permission)
				}
				if permission != test.permission && stringSliceContains(permissions, permission) {
					t.Fatalf("auth/me permissions=%v invented unconfigured=%s", permissions, permission)
				}
			}
			for _, permission := range []string{"admin.full", "points:unlisted", "enterprise:tenant:view"} {
				if stringSliceContains(permissions, permission) {
					t.Fatalf("auth/me exposed unrelated database permission %s: %v", permission, permissions)
				}
			}
		})
	}
	contextPermissions := authMe(fixtures["points_policy_view"])
	if stringSliceContains(contextPermissions, "points:balance:correct") {
		t.Fatalf("tenant context role permission leaked into platform auth permissions: %v", contextPermissions)
	}
	roleNamePermissions := authMe(fixtures["points_role_name_only"])
	for _, permission := range allPointsPermissions {
		if stringSliceContains(roleNamePermissions, permission) {
			t.Fatalf("role name expanded to %s in auth/me: %v", permission, roleNamePermissions)
		}
	}
	for _, permission := range allPointsPermissions {
		if !stringSliceContains(superPermissions, permission) {
			t.Fatalf("SUPER_ADMIN auth/me missing points bypass permission %s: %v", permission, superPermissions)
		}
	}

	rbacRouter := gin.New()
	rbacRouter.Use(store.rbacMiddleware(auth, "pricing:plan:view"))
	rbacRouter.GET("/api/v1/admin/pricing-probe", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, test := range []struct {
		name   string
		item   fixture
		status int
	}{
		{"exact database permission authorizes", fixtures["exact"], http.StatusNoContent},
		{"admin full does not authorize pricing", fixtures["legacy"], http.StatusForbidden},
		{"super admin bypass authorizes", fixtures["super"], http.StatusNoContent},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/pricing-probe", nil)
			request.Header.Set("Authorization", "Bearer "+test.item.token)
			response := httptest.NewRecorder()
			rbacRouter.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s want=%d", response.Code, response.Body.String(), test.status)
			}
		})
	}

	t.Run("login and refresh return the same exact database permissions as auth me", func(t *testing.T) {
		item := fixtures["exact"]
		authRouter := http.NewServeMux()
		authRouter.HandleFunc("/api/v1/auth/login", auth.login)
		authRouter.HandleFunc("/api/v1/auth/refresh", auth.refresh)

		login := request(t, authRouter, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"`+item.userID+`@example.test","password":"`+item.password+`"}`))
		loginPayload := decodePricingAuthTokenResponse(t, login)
		assertSamePricingAuthPermissions(t, "postgres login", loginPayload.Permissions, authPermissionsForToken(t, meRouter, loginPayload.AccessToken))

		refresh := request(t, authRouter, http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refreshToken":"`+loginPayload.RefreshToken+`"}`))
		refreshPayload := decodePricingAuthTokenResponse(t, refresh)
		assertSamePricingAuthPermissions(t, "postgres refresh", refreshPayload.Permissions, authPermissionsForToken(t, meRouter, refreshPayload.AccessToken))
	})

	t.Run("register returns the same exact database permissions as auth me", func(t *testing.T) {
		permission := "pricing:audit:view"
		inserted, err := db.ExecContext(ctx, `insert into xz_role_permissions(role,permission) values('MEMBER',$1) on conflict do nothing`, permission)
		if err != nil {
			t.Fatal(err)
		}
		if rows, err := inserted.RowsAffected(); err == nil && rows > 0 {
			t.Cleanup(func() {
				_, _ = db.Exec(`delete from xz_role_permissions where role='MEMBER' and permission=$1`, permission)
			})
		}

		authRouter := http.NewServeMux()
		authRouter.HandleFunc("/api/v1/auth/register", auth.register)
		email := "pricing-register-" + suffix + "@example.test"
		register := request(t, authRouter, http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"username":"Pricing Register","email":"`+email+`","password":"Register123!","confirmPassword":"Register123!"}`))
		registerPayload := decodePricingAuthTokenResponse(t, register)
		var registered struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		}
		if err := json.Unmarshal(register.Body.Bytes(), &registered); err != nil {
			t.Fatal(err)
		}
		if registered.User.ID == "" {
			t.Fatal("register response missing user id")
		}
		// Deliberately keep the registered user row instead of cleaning it up:
		// deleting the row would let nextTableID recycle the same ID on a later
		// run while the append-only registration point lot survives, tripping
		// the grant idempotency key with "point idempotency conflict".
		assertSamePricingAuthPermissions(t, "postgres register", registerPayload.Permissions, authPermissionsForToken(t, meRouter, registerPayload.AccessToken))
	})
}

func authPermissionsForToken(t *testing.T, handler http.Handler, token string) []string {
	t.Helper()
	response := authedRequest(t, handler, http.MethodGet, "/api/v1/auth/me", nil, token)
	if response.Code != http.StatusOK {
		t.Fatalf("auth/me status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload.Permissions
}

func assertSamePricingAuthPermissions(t *testing.T, source string, got []string, want []string) {
	t.Helper()
	got = append([]string(nil), got...)
	want = append([]string(nil), want...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s permissions=%v auth/me permissions=%v", source, got, want)
	}
}
