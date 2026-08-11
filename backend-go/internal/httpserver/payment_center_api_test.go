package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/gin-gonic/gin"

	paymentapp "xianzhi-ai/backend-go/internal/app/payment"
	"xianzhi-ai/backend-go/internal/config"
)

func TestPaymentOrderOwnership(t *testing.T) {
	order := paymentapp.Order{UserID: "owner"}
	if !canQueryPaymentOrder(adminUser{ID: "owner", Role: "MEMBER"}, order) {
		t.Fatal("owner must be allowed to query order")
	}
	if canQueryPaymentOrder(adminUser{ID: "other", Role: "MEMBER"}, order) {
		t.Fatal("non-owner member must not query order")
	}
	if !canQueryPaymentOrder(adminUser{ID: "admin", Role: "SUPER_ADMIN"}, order) {
		t.Fatal("platform admin must be allowed to query all orders")
	}
}

func TestMockPaymentRoutesEnabledOutsideProduction(t *testing.T) {
	server := New(config.Config{Environment: "test", DataPath: filepath.Join(t.TempDir(), "store.json")})
	if !hasGinRoute(t, server.Handler.(*gin.Engine), "POST", "/api/v1/payment/mock/:orderNo/success") {
		t.Fatal("mock success route is missing outside production")
	}
}

func TestPaymentAdminCompatibilityRoutes(t *testing.T) {
	server := New(config.Config{Environment: "test", DataPath: filepath.Join(t.TempDir(), "store.json")})
	engine := server.Handler.(*gin.Engine)
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/admin/payment/orders"},
		{http.MethodGet, "/api/admin/payment/orders/:orderNo"},
		{http.MethodGet, "/api/admin/payment/transactions"},
		{http.MethodGet, "/api/admin/payment/fulfillments"},
		{http.MethodPost, "/api/admin/payment/fulfillments/:id/retry"},
	} {
		if !hasGinRoute(t, engine, route.method, route.path) {
			t.Fatalf("missing compatibility route %s %s", route.method, route.path)
		}
	}
}

func TestPaymentAdminQueriesSupportRequiredFiltersPostgres(t *testing.T) {
	dsn := os.Getenv("PAYMENT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PAYMENT_TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	request, err := http.NewRequest(http.MethodGet,
		"/?orderNo=missing&userId=missing&tenantId=missing&productCode=missing&platform=web&channel=mock"+
			"&orderStatus=CREATED&paymentStatus=PENDING&fulfillmentStatus=PENDING"+
			"&createdFrom=2026-01-01T00:00:00Z&createdTo=2027-01-01T00:00:00Z", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range []string{"orders", "transactions", "fulfillments"} {
		query, args := paymentAdminQuery(resource, request, 10, 0)
		rows, err := db.QueryContext(request.Context(), query, args...)
		if err != nil {
			t.Fatalf("%s filters failed: %v", resource, err)
		}
		rows.Close()
	}
}

func TestMockPaymentRoutesForbiddenInProduction(t *testing.T) {
	server := New(config.Config{Environment: "production", DataPath: filepath.Join(t.TempDir(), "store.json")})
	if hasGinRoute(t, server.Handler.(*gin.Engine), "POST", "/api/v1/payment/mock/:orderNo/success") {
		t.Fatal("mock success route must not be registered in production")
	}
}

func TestAdminCannotMarkUnifiedPaymentOrderPaidPostgres(t *testing.T) {
	dsn := os.Getenv("PAYMENT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PAYMENT_TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id := fmt.Sprintf("payment_admin_guard_%d", time.Now().UnixNano())
	_, err = db.Exec(`
		INSERT INTO xz_orders(id,order_no,product_id,status,order_status,created_at,raw)
		VALUES ($1,$1,'payment_product_token_1000','CREATED','CREATED',$2,'{}'::jsonb)
	`, id, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`DELETE FROM xz_orders WHERE id=$1`, id)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/orders/"+id+"/mark-paid", nil)
	request.SetPathValue("id", id)
	recorder := httptest.NewRecorder()
	adminAPI{store: &postgresStore{db: db, ready: true}}.markOrderPaid(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestUnifiedPaymentPersonalPointGrantHookUsesCallerTransactionPostgres(t *testing.T) {
	dsn := os.Getenv("PAYMENT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PAYMENT_TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	userID := "unified_point_hook_" + suffix
	insertVirtualTestUser(t, ctx, db, userID, time.Time{})
	insertVirtualTestPointAccount(t, ctx, db, userID)
	defer cleanupVirtualPaymentTest(t, db, "UNIFIEDPOINT"+suffix, userID)
	hook := newUnifiedPaymentPersonalPointGrantHook(db)
	request := paymentapp.PersonalPointGrantRequest{
		UserID: userID, TenantID: "personal:" + userID, Source: string(PointSourceUnifiedPaymentGrant), Points: 100,
		ReferenceType: "UNIFIED_PAYMENT_ORDER", ReferenceID: "UNIFIEDPOINT" + suffix,
		IdempotencyKey: "unified-payment-test:" + suffix, GrantedAt: time.Now().UTC(),
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := hook(ctx, tx, request)
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if grant.AvailableAfter-grant.AvailableBefore != 100 {
		_ = tx.Rollback()
		t.Fatalf("grant delta = %d", grant.AvailableAfter-grant.AvailableBefore)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	assertPersonalPointGrantRows(t, ctx, db, userID, request.ReferenceID, PointSourceUnifiedPaymentGrant, 0, 0)

	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hook(ctx, tx, request); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	assertPersonalPointGrantRows(t, ctx, db, userID, request.ReferenceID, PointSourceUnifiedPaymentGrant, 100, 1)
}

func assertPersonalPointGrantRows(t *testing.T, ctx context.Context, db *sql.DB, userID, referenceID string, source PointSource, wantBalance int64, wantLots int) {
	t.Helper()
	var balance int64
	if err := db.QueryRowContext(ctx, `SELECT available FROM xz_point_accounts WHERE user_id=$1`, userID).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	var lots int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM xz_personal_point_lots
		WHERE user_id=$1 AND reference_id=$2 AND source_type=$3 AND expires_at IS NULL
	`, userID, referenceID, source).Scan(&lots); err != nil {
		t.Fatal(err)
	}
	if balance != wantBalance || lots != wantLots {
		var observed string
		_ = db.QueryRowContext(ctx, `
			SELECT coalesce(string_agg(source_type || ':' || reference_type || ':' || reference_id || ':' || coalesce(expires_at::text,'PERMANENT'), ',' ORDER BY granted_at), '')
			FROM xz_personal_point_lots WHERE user_id=$1
		`, userID).Scan(&observed)
		t.Fatalf("personal point rows: balance=%d lots=%d want_balance=%d want_lots=%d observed=%s", balance, lots, wantBalance, wantLots, observed)
	}
}

func hasGinRoute(t *testing.T, engine *gin.Engine, method, path string) bool {
	t.Helper()
	for _, route := range engine.Routes() {
		if route.Method == method && route.Path == path {
			return true
		}
	}
	return false
}

func TestAndroidPaymentCapabilityDefaultsToPreparing(t *testing.T) {
	api := paymentCenterAPI{cfg: config.Config{}}
	result := api.paymentCapabilityFor("android")
	if result.Platform != "app-android" || result.PaymentCapability != "preparing" || result.PaymentStatus != "PREPARING" || result.Enabled {
		t.Fatalf("unexpected Android capability: %#v", result)
	}
}

func TestAndroidPaymentCapabilityRequiresConfiguredProvider(t *testing.T) {
	api := paymentCenterAPI{cfg: config.Config{
		AndroidPaymentCapability: "available",
		AndroidPaymentChannel:    "wechat_app",
	}}
	result := api.paymentCapabilityFor("app-android")
	if result.PaymentCapability != "preparing" || result.Enabled {
		t.Fatalf("missing provider must keep capability preparing: %#v", result)
	}
}

func TestAndroidPaymentCapabilityBecomesAvailableWithProvider(t *testing.T) {
	mock := paymentapp.NewMockPaymentProvider("test")
	api := paymentCenterAPI{
		cfg: config.Config{
			AndroidPaymentCapability: "available",
			AndroidPaymentChannel:    "mock",
		},
		service: paymentapp.NewService(&sql.DB{}, []paymentapp.PaymentProvider{mock}, nil),
	}
	result := api.paymentCapabilityFor("app-android")
	if result.PaymentCapability != "available" || result.PaymentStatus != "READY" || !result.Enabled {
		t.Fatalf("configured provider should enable capability: %#v", result)
	}
}

func TestPreparingAndroidPaymentCannotCreateOrder(t *testing.T) {
	api := paymentCenterAPI{cfg: config.Config{Environment: "production", AndroidPaymentCapability: "preparing"}}
	if api.orderCreationAllowed("app-android", "wechat_app") {
		t.Fatal("preparing Android capability must reject order creation")
	}
}
