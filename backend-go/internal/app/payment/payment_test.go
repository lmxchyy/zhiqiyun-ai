package payment

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestIllegalOrderStateTransition(t *testing.T) {
	if err := ValidateOrderTransition(OrderCreated, OrderCompleted); ErrorCodeOf(err) != CodeInvalidTransition {
		t.Fatalf("expected invalid transition, got %v", err)
	}
	if err := ValidateOrderTransition(OrderPaid, OrderFulfilling); err != nil {
		t.Fatalf("valid transition rejected: %v", err)
	}
}

func TestIllegalPaymentStateTransition(t *testing.T) {
	if err := ValidatePaymentTransition(PaymentInit, PaymentSuccess); ErrorCodeOf(err) != CodeInvalidTransition {
		t.Fatalf("expected invalid transition, got %v", err)
	}
}

func TestMockPaymentAvailableOutsideProduction(t *testing.T) {
	provider := NewMockPaymentProvider("development")
	result, err := provider.CreatePayment(context.Background(), CreatePaymentRequest{
		OrderNo: "order", PaymentNo: "payment", Amount: 5000, Currency: "CNY",
	})
	if err != nil || result.PaymentStatus != PaymentPending {
		t.Fatalf("mock create failed: result=%+v err=%v", result, err)
	}
}

func TestMockPaymentForbiddenInProduction(t *testing.T) {
	provider := NewMockPaymentProvider("production")
	_, err := provider.CreatePayment(context.Background(), CreatePaymentRequest{OrderNo: "order"})
	if ErrorCodeOf(err) != CodeMockForbidden {
		t.Fatalf("expected mock forbidden, got %v", err)
	}
}

func TestMockSupportsSuccessFailureMismatchQueryAndRefund(t *testing.T) {
	provider := NewMockPaymentProvider("test")
	order := Order{OrderNo: "order", PayableAmount: 5000, Currency: "CNY"}
	if _, err := provider.CreatePayment(context.Background(), CreatePaymentRequest{
		OrderNo: order.OrderNo, PaymentNo: "payment", Amount: 5000, Currency: "CNY",
	}); err != nil {
		t.Fatal(err)
	}
	success, err := provider.Notification(order, MockSuccess, "")
	if err != nil || success.Status != PaymentSuccess || success.Amount != 5000 {
		t.Fatalf("success notification=%+v err=%v", success, err)
	}
	mismatch, err := provider.Notification(order, MockAmountMismatch, "another")
	if err != nil || mismatch.Amount != 5001 {
		t.Fatalf("mismatch notification=%+v err=%v", mismatch, err)
	}
	failure, err := provider.Notification(order, MockFailure, "")
	if err != nil || failure.Status != PaymentFailed {
		t.Fatalf("failure notification=%+v err=%v", failure, err)
	}
	if _, err := provider.QueryPayment(context.Background(), QueryPaymentRequest{OrderNo: order.OrderNo}); err != nil {
		t.Fatal(err)
	}
	provider.payments[order.OrderNo].Status = PaymentSuccess
	refund, err := provider.RefundPayment(context.Background(), RefundPaymentRequest{
		OrderNo: order.OrderNo, PaymentNo: "payment", RefundNo: "refund", Amount: 5000, Currency: "CNY",
	})
	if err != nil || refund.Status != PaymentRefunded {
		t.Fatalf("refund=%+v err=%v", refund, err)
	}
}

func TestMockDelayedNotification(t *testing.T) {
	provider := NewMockPaymentProvider("test")
	order := Order{OrderNo: "order", PayableAmount: 1, Currency: "CNY"}
	_, _ = provider.CreatePayment(context.Background(), CreatePaymentRequest{OrderNo: order.OrderNo, PaymentNo: "payment"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := provider.DelayedNotification(ctx, order, time.Millisecond); err != nil {
		t.Fatal(err)
	}
}

func TestCreateOrderValidationRequiresIdempotencyAndIgnoresClientAmountByDesign(t *testing.T) {
	input := CreateOrderInput{UserID: "user", TenantID: "tenant", ProductCode: "TOKEN_1000", Quantity: 1, Platform: "web", PaymentChannel: "mock"}
	if err := input.Validate(); ErrorCodeOf(err) != CodeInvalidRequest {
		t.Fatalf("missing idempotency should fail, got %v", err)
	}
	input.IdempotencyKey = "key"
	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDomainErrorWrapping(t *testing.T) {
	cause := errors.New("database failed")
	err := E(CodeFulfillmentFailed, "fulfillment failed", cause)
	if !errors.Is(err, cause) || ErrorCodeOf(err) != CodeFulfillmentFailed {
		t.Fatalf("domain wrapping failed: %v", err)
	}
}
