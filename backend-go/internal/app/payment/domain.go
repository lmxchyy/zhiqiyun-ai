package payment

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ErrorCode string

const (
	CodeInvalidRequest         ErrorCode = "PAYMENT_INVALID_REQUEST"
	CodeProductNotFound        ErrorCode = "PAYMENT_PRODUCT_NOT_FOUND"
	CodeProductInactive        ErrorCode = "PAYMENT_PRODUCT_INACTIVE"
	CodePriceNotConfigured     ErrorCode = "PAYMENT_PRICE_NOT_CONFIGURED"
	CodeIdempotencyConflict    ErrorCode = "PAYMENT_IDEMPOTENCY_CONFLICT"
	CodeOrderNotFound          ErrorCode = "PAYMENT_ORDER_NOT_FOUND"
	CodeOrderForbidden         ErrorCode = "PAYMENT_ORDER_FORBIDDEN"
	CodeInvalidTransition      ErrorCode = "PAYMENT_INVALID_STATE_TRANSITION"
	CodePaymentMismatch        ErrorCode = "PAYMENT_CONFIRMATION_MISMATCH"
	CodeDuplicateTransaction   ErrorCode = "PAYMENT_DUPLICATE_TRANSACTION"
	CodeMockForbidden          ErrorCode = "PAYMENT_MOCK_FORBIDDEN"
	CodeFulfillmentUnsupported ErrorCode = "PAYMENT_FULFILLMENT_UNSUPPORTED"
	CodeFulfillmentFailed      ErrorCode = "PAYMENT_FULFILLMENT_FAILED"
)

type DomainError struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *DomainError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *DomainError) Unwrap() error { return e.Cause }

func E(code ErrorCode, message string, cause ...error) error {
	var inner error
	if len(cause) > 0 {
		inner = cause[0]
	}
	return &DomainError{Code: code, Message: message, Cause: inner}
}

func ErrorCodeOf(err error) ErrorCode {
	var domain *DomainError
	if errors.As(err, &domain) {
		return domain.Code
	}
	return ""
}

type OrderStatus string

const (
	OrderCreated         OrderStatus = "CREATED"
	OrderPaying          OrderStatus = "PAYING"
	OrderPaid            OrderStatus = "PAID"
	OrderFulfilling      OrderStatus = "FULFILLING"
	OrderCompleted       OrderStatus = "COMPLETED"
	OrderClosed          OrderStatus = "CLOSED"
	OrderCancelled       OrderStatus = "CANCELLED"
	OrderRefunding       OrderStatus = "REFUNDING"
	OrderRefunded        OrderStatus = "REFUNDED"
	OrderPartialRefunded OrderStatus = "PARTIAL_REFUNDED"
	OrderFailed          OrderStatus = "FAILED"
)

type PaymentStatus string

const (
	PaymentInit      PaymentStatus = "INIT"
	PaymentPending   PaymentStatus = "PENDING"
	PaymentSuccess   PaymentStatus = "SUCCESS"
	PaymentFailed    PaymentStatus = "FAILED"
	PaymentClosed    PaymentStatus = "CLOSED"
	PaymentRefunding PaymentStatus = "REFUNDING"
	PaymentRefunded  PaymentStatus = "REFUNDED"
)

type FulfillmentStatus string

const (
	FulfillmentPending    FulfillmentStatus = "PENDING"
	FulfillmentProcessing FulfillmentStatus = "PROCESSING"
	FulfillmentSuccess    FulfillmentStatus = "SUCCESS"
	FulfillmentFailed     FulfillmentStatus = "FAILED"
)

type Product struct {
	ID                 string
	SourcePlanID       string
	Code               string
	Name               string
	ProductType        string
	FulfillmentType    string
	Description        string
	Status             string
	FulfillmentPayload json.RawMessage
}

type Price struct {
	ID         string
	ProductID  string
	Channel    string
	Platform   string
	Currency   string
	Amount     int64
	Status     string
	ExternalID string
}

type Order struct {
	ID                 string
	OrderNo            string
	UserID             string
	TenantID           string
	ProductID          string
	SourcePlanID       string
	ProductCode        string
	ProductName        string
	ProductType        string
	FulfillmentType    string
	Quantity           int64
	Currency           string
	OriginalAmount     int64
	DiscountAmount     int64
	PayableAmount      int64
	Platform           string
	Channel            string
	OrderStatus        OrderStatus
	PaymentStatus      PaymentStatus
	FulfillmentStatus  FulfillmentStatus
	IdempotencyKey     string
	ClientIP           string
	FulfillmentPayload json.RawMessage
	CreatedAt          time.Time
	PaidAt             *time.Time
	FulfilledAt        *time.Time
}

type CreateOrderInput struct {
	UserID         string
	TenantID       string
	ProductCode    string
	Quantity       int64
	Platform       string
	PaymentChannel string
	IdempotencyKey string
	ClientIP       string
}

func (in CreateOrderInput) Validate() error {
	if strings.TrimSpace(in.UserID) == "" || strings.TrimSpace(in.TenantID) == "" {
		return E(CodeInvalidRequest, "authenticated user and tenant are required")
	}
	if strings.TrimSpace(in.ProductCode) == "" || strings.TrimSpace(in.Platform) == "" || strings.TrimSpace(in.PaymentChannel) == "" {
		return E(CodeInvalidRequest, "productCode, platform and paymentChannel are required")
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		return E(CodeInvalidRequest, "Idempotency-Key header is required")
	}
	if in.Quantity <= 0 {
		return E(CodeInvalidRequest, "quantity must be positive")
	}
	return nil
}

type PaymentNotification struct {
	OrderNo               string
	Provider              string
	ProviderTradeNo       string
	ProviderTransactionID string
	Amount                int64
	Currency              string
	Status                PaymentStatus
	Payload               json.RawMessage
	PaidAt                time.Time
}

func (n PaymentNotification) Validate() error {
	if strings.TrimSpace(n.OrderNo) == "" || strings.TrimSpace(n.Provider) == "" || strings.TrimSpace(n.ProviderTransactionID) == "" {
		return E(CodeInvalidRequest, "order, provider and provider transaction are required")
	}
	if n.Amount < 0 || len(strings.TrimSpace(n.Currency)) != 3 {
		return E(CodeInvalidRequest, "payment amount or currency is invalid")
	}
	return nil
}

type AdminFilter struct {
	OrderNo           string
	UserID            string
	TenantID          string
	ProductCode       string
	Platform          string
	Channel           string
	OrderStatus       string
	PaymentStatus     string
	FulfillmentStatus string
	CreatedFrom       *time.Time
	CreatedTo         *time.Time
	Limit             int
	Offset            int
}

func normalizeCurrency(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "CNY"
	}
	return value
}

func safeJSON(value []byte) []byte {
	if len(value) == 0 || !json.Valid(value) {
		return []byte("{}")
	}
	return value
}

func checkedMultiply(left, right int64) (int64, error) {
	if left <= 0 || right < 0 || (left != 0 && right > (1<<63-1)/left) {
		return 0, E(CodeInvalidRequest, fmt.Sprintf("invalid amount calculation: %d x %d", left, right))
	}
	return left * right, nil
}
