package payment

import (
	"context"
	"strings"
	"time"
)

type WeChatVirtualRefundRequest struct {
	OrderNo, PaymentNo, RefundNo string
	ProviderRefundID, PayerID    string
	Amount                       int64
	Currency, Description        string
}

type WeChatVirtualRefundResponse struct {
	Code, Status, ProviderRefundID string
	CompletedAt                    *time.Time
	Summary                        ProviderResponseSummary
}

type WeChatVirtualRefundGateway interface {
	RefundVirtualPayment(context.Context, WeChatVirtualRefundRequest) (WeChatVirtualRefundResponse, error)
	QueryVirtualRefund(context.Context, WeChatVirtualRefundRequest) (WeChatVirtualRefundResponse, error)
}

type WeChatVirtualRefundAdapter struct {
	gateway WeChatVirtualRefundGateway
}

func NewWeChatVirtualRefundAdapter(gateway WeChatVirtualRefundGateway) *WeChatVirtualRefundAdapter {
	return &WeChatVirtualRefundAdapter{gateway: gateway}
}

func (*WeChatVirtualRefundAdapter) GetProviderName() string { return "wechat_virtual" }

func (adapter *WeChatVirtualRefundAdapter) RefundPayment(ctx context.Context, request RefundPaymentRequest) (RefundPaymentResult, error) {
	if adapter == nil || adapter.gateway == nil {
		return RefundPaymentResult{Outcome: RefundUnsupported, ResponseSummary: ProviderResponseSummary{"provider": "wechat_virtual", "code": "GATEWAY_UNAVAILABLE"}}, nil
	}
	response, err := adapter.gateway.RefundVirtualPayment(ctx, WeChatVirtualRefundRequest{OrderNo: request.OrderNo, PaymentNo: request.PaymentNo, RefundNo: request.RefundNo, Amount: request.Amount, Currency: request.Currency, Description: request.Description})
	outcome := mapWeChatVirtualRefundOutcome(response, err)
	return RefundPaymentResult{ProviderRefundID: strings.TrimSpace(response.ProviderRefundID), Outcome: outcome, CompletedAt: response.CompletedAt, ResponseSummary: SanitizeProviderResponseSummary(response.Summary)}, nil
}

func (adapter *WeChatVirtualRefundAdapter) QueryRefund(ctx context.Context, request QueryRefundRequest) (QueryRefundResult, error) {
	if adapter == nil || adapter.gateway == nil {
		return QueryRefundResult{Outcome: QueryRefundUnsupported, ResponseSummary: ProviderResponseSummary{"provider": "wechat_virtual", "code": "GATEWAY_UNAVAILABLE"}}, nil
	}
	response, err := adapter.gateway.QueryVirtualRefund(ctx, WeChatVirtualRefundRequest{OrderNo: request.OrderNo, PaymentNo: request.PaymentNo, RefundNo: request.RefundNo, ProviderRefundID: request.ProviderRefundID, PayerID: request.PayerID})
	outcome := mapWeChatVirtualQueryOutcome(response, err)
	return QueryRefundResult{ProviderRefundID: strings.TrimSpace(response.ProviderRefundID), Outcome: outcome, CompletedAt: response.CompletedAt, ResponseSummary: SanitizeProviderResponseSummary(response.Summary)}, nil
}

func mapWeChatVirtualRefundOutcome(response WeChatVirtualRefundResponse, err error) RefundOutcome {
	if err != nil {
		return RefundUnknown
	}
	switch strings.ToUpper(strings.TrimSpace(response.Status)) {
	case "SUCCESS", "SUCCEEDED", "REFUNDED":
		if strings.TrimSpace(response.ProviderRefundID) != "" {
			return RefundSuccess
		}
		return RefundUnknown
	case "TEMPORARY_FAILURE", "RETRYABLE":
		return RefundTemporaryFailure
	case "UNSUPPORTED":
		return RefundUnsupported
	default:
		return RefundUnknown
	}
}

func mapWeChatVirtualQueryOutcome(response WeChatVirtualRefundResponse, err error) QueryRefundOutcome {
	if err != nil {
		return QueryRefundUnknown
	}
	switch strings.ToUpper(strings.TrimSpace(response.Status)) {
	case "SUCCESS", "SUCCEEDED", "REFUNDED":
		return QueryRefundSucceeded
	case "NOT_FOUND", "PAID_WITHOUT_REFUND":
		return QueryRefundNotFound
	case "PROCESSING", "REFUND_PENDING":
		return QueryRefundProcessing
	case "FAILED", "REFUND_FAILED":
		return QueryRefundFailed
	case "UNSUPPORTED":
		return QueryRefundUnsupported
	default:
		return QueryRefundUnknown
	}
}

func SanitizeProviderResponseSummary(summary ProviderResponseSummary) ProviderResponseSummary {
	result := ProviderResponseSummary{}
	for key, value := range summary {
		lower := strings.ToLower(strings.TrimSpace(key))
		if lower == "" || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "authorization") || strings.Contains(lower, "signature") || strings.Contains(lower, "openid") || strings.Contains(lower, "session") || strings.Contains(lower, "appkey") {
			continue
		}
		switch item := value.(type) {
		case map[string]any:
			result[key] = SanitizeProviderResponseSummary(ProviderResponseSummary(item))
		case ProviderResponseSummary:
			result[key] = SanitizeProviderResponseSummary(item)
		default:
			result[key] = item
		}
	}
	return result
}
