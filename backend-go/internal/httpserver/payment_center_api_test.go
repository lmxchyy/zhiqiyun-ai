package httpserver

import (
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

func hasGinRoute(t *testing.T, engine *gin.Engine, method, path string) bool {
	t.Helper()
	for _, route := range engine.Routes() {
		if route.Method == method && route.Path == path {
			return true
		}
	}
	return false
}
