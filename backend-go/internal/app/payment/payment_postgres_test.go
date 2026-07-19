package payment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func paymentTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("PAYMENT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PAYMENT_TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func paymentFixture(t *testing.T) (*Service, *MockPaymentProvider, *sql.DB, string) {
	t.Helper()
	db := paymentTestDB(t)
	userID := fmt.Sprintf("payment_test_user_%d", time.Now().UnixNano())
	_, err := db.Exec(`
		INSERT INTO xz_users(id,email,name,role,status,created_at,updated_at,raw)
		VALUES ($1,$2,'Payment Test','MEMBER','ACTIVE',$3,$3,jsonb_build_object('id',$1::text,'email',$2::text,'name','Payment Test','role','MEMBER','status','ACTIVE'))
	`, userID, userID+"@example.test", time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_audit_logs WHERE resource_id LIKE 'ZQP%'`)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_wallet_ledger WHERE user_id=$1`, userID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_token_records WHERE user_id=$1`, userID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_fulfillment_records WHERE user_id=$1`, userID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_payment_records WHERE user_id=$1`, userID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_orders WHERE user_id=$1`, userID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_user_wallets WHERE user_id=$1`, userID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_point_accounts WHERE user_id=$1`, userID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_users WHERE id=$1`, userID)
	})
	mock := NewMockPaymentProvider("test")
	return NewService(db, []PaymentProvider{mock}, nil), mock, db, userID
}

func createTokenOrder(t *testing.T, service *Service, userID, key string) CreateOrderResult {
	t.Helper()
	result, err := service.CreateOrder(context.Background(), CreateOrderInput{
		UserID: userID, TenantID: "personal:" + userID, ProductCode: "TOKEN_1000",
		Quantity: 1, Platform: "web", PaymentChannel: "mock", IdempotencyKey: key, ClientIP: "127.0.0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestCreateOrderUsesServerCatalogAmount(t *testing.T) {
	service, _, _, userID := paymentFixture(t)
	result := createTokenOrder(t, service, userID, "create")
	if result.Order.PayableAmount != 5000 || result.Order.Currency != "CNY" {
		t.Fatalf("server amount mismatch: %+v", result.Order)
	}
}

func TestDuplicateIdempotencyKeyReturnsSameOrder(t *testing.T) {
	service, _, _, userID := paymentFixture(t)
	first := createTokenOrder(t, service, userID, "same")
	second := createTokenOrder(t, service, userID, "same")
	if first.Order.OrderNo != second.Order.OrderNo {
		t.Fatalf("duplicate key created another order: %s != %s", first.Order.OrderNo, second.Order.OrderNo)
	}
}

func TestIdempotencyKeyRejectsDifferentPayload(t *testing.T) {
	service, _, _, userID := paymentFixture(t)
	_ = createTokenOrder(t, service, userID, "conflict")
	_, err := service.CreateOrder(context.Background(), CreateOrderInput{
		UserID: userID, TenantID: "personal:" + userID, ProductCode: "TOKEN_1000",
		Quantity: 2, Platform: "web", PaymentChannel: "mock", IdempotencyKey: "conflict",
	})
	if ErrorCodeOf(err) != CodeIdempotencyConflict {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
}

func TestProductNotFound(t *testing.T) {
	service, _, _, userID := paymentFixture(t)
	_, err := service.CreateOrder(context.Background(), CreateOrderInput{
		UserID: userID, TenantID: "personal:" + userID, ProductCode: "MISSING",
		Quantity: 1, Platform: "web", PaymentChannel: "mock", IdempotencyKey: "missing",
	})
	if ErrorCodeOf(err) != CodeProductNotFound {
		t.Fatalf("expected product not found, got %v", err)
	}
}

func TestInactiveProduct(t *testing.T) {
	service, _, db, userID := paymentFixture(t)
	if _, err := db.Exec(`UPDATE xz_payment_products SET status='INACTIVE' WHERE product_code='TOKEN_1000'`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`UPDATE xz_payment_products SET status='ACTIVE' WHERE product_code='TOKEN_1000'`)
	})
	_, err := service.CreateOrder(context.Background(), CreateOrderInput{
		UserID: userID, TenantID: "personal:" + userID, ProductCode: "TOKEN_1000",
		Quantity: 1, Platform: "web", PaymentChannel: "mock", IdempotencyKey: "inactive",
	})
	if ErrorCodeOf(err) != CodeProductInactive {
		t.Fatalf("expected inactive, got %v", err)
	}
}

func TestMissingChannelPrice(t *testing.T) {
	service, _, _, userID := paymentFixture(t)
	_, err := service.CreateOrder(context.Background(), CreateOrderInput{
		UserID: userID, TenantID: "personal:" + userID, ProductCode: "TOKEN_1000",
		Quantity: 1, Platform: "ios", PaymentChannel: "mock", IdempotencyKey: "no-price",
	})
	if ErrorCodeOf(err) != CodePriceNotConfigured {
		t.Fatalf("expected no price, got %v", err)
	}
}

func TestMockSuccessGrantsTokenAndCompletesOrder(t *testing.T) {
	service, mock, db, userID := paymentFixture(t)
	result := createTokenOrder(t, service, userID, "success")
	notification, err := mock.Notification(result.Order, MockSuccess, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.HandlePaymentNotification(context.Background(), notification); err != nil {
		t.Fatalf("payment success failed: %v (cause: %v)", err, errors.Unwrap(err))
	}
	order, err := service.GetOrder(context.Background(), result.Order.OrderNo)
	if err != nil {
		t.Fatal(err)
	}
	if order.OrderStatus != OrderCompleted || order.PaymentStatus != PaymentSuccess || order.FulfillmentStatus != FulfillmentSuccess {
		t.Fatalf("unexpected completed order: %+v", order)
	}
	var balance int64
	if err := db.QueryRow(`SELECT available FROM xz_point_accounts WHERE user_id=$1`, userID).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != 1000 {
		t.Fatalf("token balance=%d want=1000", balance)
	}
}

func TestDuplicateCallbackAndFulfillmentAreIdempotent(t *testing.T) {
	service, mock, db, userID := paymentFixture(t)
	result := createTokenOrder(t, service, userID, "duplicate")
	notification, _ := mock.Notification(result.Order, MockSuccess, "")
	for i := 0; i < 3; i++ {
		if err := service.HandlePaymentNotification(context.Background(), notification); err != nil {
			t.Fatal(err)
		}
	}
	var tokenCount, fulfillmentCount int
	if err := db.QueryRow(`SELECT count(*) FROM xz_token_records WHERE source_order_no=$1`, result.Order.OrderNo).Scan(&tokenCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM xz_fulfillment_records WHERE order_no=$1`, result.Order.OrderNo).Scan(&fulfillmentCount); err != nil {
		t.Fatal(err)
	}
	if tokenCount != 1 || fulfillmentCount != 1 {
		t.Fatalf("token=%d fulfillment=%d", tokenCount, fulfillmentCount)
	}
}

func TestPaymentAmountMismatchDoesNotGrant(t *testing.T) {
	service, mock, db, userID := paymentFixture(t)
	result := createTokenOrder(t, service, userID, "mismatch")
	notification, _ := mock.Notification(result.Order, MockAmountMismatch, "")
	err := service.HandlePaymentNotification(context.Background(), notification)
	if ErrorCodeOf(err) != CodePaymentMismatch {
		t.Fatalf("expected mismatch, got %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM xz_token_records WHERE source_order_no=$1`, result.Order.OrderNo).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("mismatched payment granted token")
	}
}

func TestUnsupportedMembershipPersistsPaidRetryableFailure(t *testing.T) {
	service, mock, _, userID := paymentFixture(t)
	result, err := service.CreateOrder(context.Background(), CreateOrderInput{
		UserID: userID, TenantID: "personal:" + userID, ProductCode: "MEMBER_YEAR",
		Quantity: 1, Platform: "web", PaymentChannel: "mock", IdempotencyKey: "membership",
	})
	if err != nil {
		t.Fatal(err)
	}
	notification, _ := mock.Notification(result.Order, MockSuccess, "")
	err = service.HandlePaymentNotification(context.Background(), notification)
	if ErrorCodeOf(err) != CodeFulfillmentUnsupported {
		t.Fatalf("expected unsupported fulfillment, got %v", err)
	}
	order, err := service.GetOrder(context.Background(), result.Order.OrderNo)
	if err != nil {
		t.Fatal(err)
	}
	if order.PaymentStatus != PaymentSuccess || order.FulfillmentStatus != FulfillmentFailed {
		t.Fatalf("unsupported fulfillment state=%+v", order)
	}
}

func TestTokenGrantTransactionRollback(t *testing.T) {
	service, mock, db, userID := paymentFixture(t)
	result := createTokenOrder(t, service, userID, "rollback")
	_, err := db.Exec(`
		INSERT INTO xz_point_accounts(id,user_id,available,frozen,raw)
		VALUES ($1,$2,$3,0,jsonb_build_object('id',$1::text,'userId',$2::text,'available',$3::bigint,'frozen',0))
	`, "overflow_"+userID, userID, int64(^uint64(0)>>1))
	if err != nil {
		t.Fatal(err)
	}
	notification, _ := mock.Notification(result.Order, MockSuccess, "")
	err = service.HandlePaymentNotification(context.Background(), notification)
	if ErrorCodeOf(err) != CodeFulfillmentFailed {
		t.Fatalf("expected fulfillment failure, got %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM xz_token_records WHERE source_order_no=$1`, result.Order.OrderNo).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("rollback left a token ledger record")
	}
}

func TestConcurrentCallbacksGrantOnce(t *testing.T) {
	service, mock, db, userID := paymentFixture(t)
	result := createTokenOrder(t, service, userID, "concurrent")
	notification, _ := mock.Notification(result.Order, MockSuccess, "")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = service.HandlePaymentNotification(context.Background(), notification)
		}()
	}
	wg.Wait()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM xz_token_records WHERE source_order_no=$1`, result.Order.OrderNo).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("concurrent callbacks granted %d times", count)
	}
}

func TestProviderTransactionCannotBindAnotherOrder(t *testing.T) {
	service, mock, _, userID := paymentFixture(t)
	first := createTokenOrder(t, service, userID, "tx-first")
	second := createTokenOrder(t, service, userID, "tx-second")
	firstNotify, _ := mock.Notification(first.Order, MockSuccess, "shared-transaction")
	if err := service.HandlePaymentNotification(context.Background(), firstNotify); err != nil {
		t.Fatal(err)
	}
	secondNotify, _ := mock.Notification(second.Order, MockSuccess, "shared-transaction")
	if err := service.HandlePaymentNotification(context.Background(), secondNotify); ErrorCodeOf(err) != CodeDuplicateTransaction {
		t.Fatalf("expected duplicate transaction, got %v", err)
	}
}

func TestMockFailureMovesOrderToFailed(t *testing.T) {
	service, mock, _, userID := paymentFixture(t)
	result := createTokenOrder(t, service, userID, "failed")
	notification, _ := mock.Notification(result.Order, MockFailure, "")
	if err := service.HandlePaymentNotification(context.Background(), notification); err != nil {
		t.Fatal(err)
	}
	order, err := service.GetOrder(context.Background(), result.Order.OrderNo)
	if err != nil {
		t.Fatal(err)
	}
	if order.OrderStatus != OrderFailed || order.PaymentStatus != PaymentFailed {
		t.Fatalf("failed state=%+v", order)
	}
}

func TestGetOrderReturnsNotFound(t *testing.T) {
	service, _, _, _ := paymentFixture(t)
	_, err := service.GetOrder(context.Background(), "missing")
	if !errors.Is(err, err) || ErrorCodeOf(err) != CodeOrderNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}
