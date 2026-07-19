package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type CreatePaymentRequest struct {
	OrderNo   string
	PaymentNo string
	Amount    int64
	Currency  string
	Subject   string
}

type CreatePaymentResult struct {
	Provider        string         `json:"provider"`
	ProviderTradeNo string         `json:"providerTradeNo"`
	PaymentStatus   PaymentStatus  `json:"paymentStatus"`
	PaymentParams   map[string]any `json:"paymentParams"`
}

type QueryPaymentRequest struct {
	OrderNo   string
	PaymentNo string
}

type RefundPaymentRequest struct {
	OrderNo     string
	PaymentNo   string
	RefundNo    string
	Amount      int64
	Currency    string
	Description string
}

type RefundPaymentResult struct {
	ProviderRefundID string        `json:"providerRefundId"`
	Status           PaymentStatus `json:"status"`
}

type PaymentProvider interface {
	CreatePayment(context.Context, CreatePaymentRequest) (CreatePaymentResult, error)
	QueryPayment(context.Context, QueryPaymentRequest) (PaymentStatus, error)
	ClosePayment(context.Context, QueryPaymentRequest) error
	RefundPayment(context.Context, RefundPaymentRequest) (RefundPaymentResult, error)
	VerifyNotification(context.Context, []byte, map[string]string) (PaymentNotification, error)
	GetProviderName() string
}

type MockScenario string

const (
	MockSuccess        MockScenario = "success"
	MockFailure        MockScenario = "failure"
	MockAmountMismatch MockScenario = "amount_mismatch"
)

type mockPayment struct {
	Request       CreatePaymentRequest
	Status        PaymentStatus
	TransactionID string
}

type MockPaymentProvider struct {
	mu       sync.Mutex
	enabled  bool
	payments map[string]*mockPayment
}

func NewMockPaymentProvider(environment string) *MockPaymentProvider {
	env := strings.ToLower(strings.TrimSpace(environment))
	return &MockPaymentProvider{enabled: env != "production" && env != "prod", payments: map[string]*mockPayment{}}
}

func (p *MockPaymentProvider) ensureEnabled() error {
	if !p.enabled {
		return E(CodeMockForbidden, "mock payment is disabled in production")
	}
	return nil
}

func (p *MockPaymentProvider) GetProviderName() string { return "mock" }

func (p *MockPaymentProvider) CreatePayment(_ context.Context, req CreatePaymentRequest) (CreatePaymentResult, error) {
	if err := p.ensureEnabled(); err != nil {
		return CreatePaymentResult{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	tradeNo := "mock_trade_" + req.PaymentNo
	p.payments[req.OrderNo] = &mockPayment{Request: req, Status: PaymentPending, TransactionID: tradeNo}
	return CreatePaymentResult{
		Provider: "mock", ProviderTradeNo: tradeNo, PaymentStatus: PaymentPending,
		PaymentParams: map[string]any{"mock": true, "orderNo": req.OrderNo, "paymentNo": req.PaymentNo, "providerTradeNo": tradeNo},
	}, nil
}

func (p *MockPaymentProvider) QueryPayment(_ context.Context, req QueryPaymentRequest) (PaymentStatus, error) {
	if err := p.ensureEnabled(); err != nil {
		return "", err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	item := p.payments[req.OrderNo]
	if item == nil {
		return "", E(CodeOrderNotFound, "mock payment not found")
	}
	return item.Status, nil
}

func (p *MockPaymentProvider) ClosePayment(_ context.Context, req QueryPaymentRequest) error {
	if err := p.ensureEnabled(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	item := p.payments[req.OrderNo]
	if item == nil {
		return E(CodeOrderNotFound, "mock payment not found")
	}
	item.Status = PaymentClosed
	return nil
}

func (p *MockPaymentProvider) RefundPayment(_ context.Context, req RefundPaymentRequest) (RefundPaymentResult, error) {
	if err := p.ensureEnabled(); err != nil {
		return RefundPaymentResult{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	item := p.payments[req.OrderNo]
	if item == nil || item.Status != PaymentSuccess {
		return RefundPaymentResult{}, errors.New("mock payment is not refundable")
	}
	item.Status = PaymentRefunded
	return RefundPaymentResult{ProviderRefundID: "mock_refund_" + req.RefundNo, Status: PaymentRefunded}, nil
}

func (p *MockPaymentProvider) VerifyNotification(_ context.Context, body []byte, _ map[string]string) (PaymentNotification, error) {
	if err := p.ensureEnabled(); err != nil {
		return PaymentNotification{}, err
	}
	var notification PaymentNotification
	if err := json.Unmarshal(body, &notification); err != nil {
		return PaymentNotification{}, err
	}
	return notification, notification.Validate()
}

func (p *MockPaymentProvider) Notification(order Order, scenario MockScenario, transactionID string) (PaymentNotification, error) {
	if err := p.ensureEnabled(); err != nil {
		return PaymentNotification{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	item := p.payments[order.OrderNo]
	if item == nil {
		return PaymentNotification{}, E(CodeOrderNotFound, "mock payment not found")
	}
	if transactionID == "" {
		transactionID = item.TransactionID
	}
	amount := order.PayableAmount
	status := PaymentSuccess
	if scenario == MockFailure {
		status = PaymentFailed
	}
	if scenario == MockAmountMismatch {
		amount++
	}
	item.Status = status
	payload, _ := json.Marshal(map[string]any{"scenario": scenario, "orderNo": order.OrderNo})
	return PaymentNotification{
		OrderNo: order.OrderNo, Provider: "mock", ProviderTradeNo: item.TransactionID,
		ProviderTransactionID: transactionID, Amount: amount, Currency: order.Currency,
		Status: status, Payload: payload, PaidAt: time.Now().UTC(),
	}, nil
}

func (p *MockPaymentProvider) DelayedNotification(ctx context.Context, order Order, delay time.Duration) (PaymentNotification, error) {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return PaymentNotification{}, ctx.Err()
	case <-timer.C:
		return p.Notification(order, MockSuccess, "")
	}
}

func ProviderByName(providers map[string]PaymentProvider, name string) (PaymentProvider, error) {
	provider := providers[strings.ToLower(strings.TrimSpace(name))]
	if provider == nil {
		return nil, E(CodePriceNotConfigured, fmt.Sprintf("payment provider %q is not configured", name))
	}
	return provider, nil
}
