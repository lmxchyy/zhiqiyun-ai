package payment

import (
	"context"
	"errors"
	"testing"
)

type fakeWeChatVirtualRefundGateway struct {
	refund WeChatVirtualRefundResponse
	query  WeChatVirtualRefundResponse
	err    error
}

func (gateway fakeWeChatVirtualRefundGateway) RefundVirtualPayment(context.Context, WeChatVirtualRefundRequest) (WeChatVirtualRefundResponse, error) {
	return gateway.refund, gateway.err
}
func (gateway fakeWeChatVirtualRefundGateway) QueryVirtualRefund(context.Context, WeChatVirtualRefundRequest) (WeChatVirtualRefundResponse, error) {
	return gateway.query, gateway.err
}

func TestWeChatVirtualRefundAdapterMapsAllOutcomesAndSanitizes(t *testing.T) {
	refundCases := map[string]RefundOutcome{"REFUNDED": RefundSuccess, "RETRYABLE": RefundTemporaryFailure, "UNSUPPORTED": RefundUnsupported, "UNKNOWN": RefundUnknown}
	for status, expected := range refundCases {
		adapter := NewWeChatVirtualRefundAdapter(fakeWeChatVirtualRefundGateway{refund: WeChatVirtualRefundResponse{Status: status, ProviderRefundID: map[bool]string{true: "wx_refund"}[status == "REFUNDED"], Summary: ProviderResponseSummary{"status": status, "accessToken": "secret", "nested": map[string]any{"openid": "secret", "code": "ok"}}}})
		result, err := adapter.RefundPayment(context.Background(), RefundPaymentRequest{RefundNo: "stable_refund"})
		if err != nil || result.Outcome != expected {
			t.Fatalf("refund status=%s result=%+v err=%v", status, result, err)
		}
		if _, exists := result.ResponseSummary["accessToken"]; exists {
			t.Fatal("sensitive token remained in summary")
		}
	}
	queryCases := map[string]QueryRefundOutcome{"REFUNDED": QueryRefundSucceeded, "PAID_WITHOUT_REFUND": QueryRefundNotFound, "PROCESSING": QueryRefundProcessing, "REFUND_FAILED": QueryRefundFailed, "UNSUPPORTED": QueryRefundUnsupported, "UNKNOWN": QueryRefundUnknown}
	for status, expected := range queryCases {
		adapter := NewWeChatVirtualRefundAdapter(fakeWeChatVirtualRefundGateway{query: WeChatVirtualRefundResponse{Status: status}})
		result, err := adapter.QueryRefund(context.Background(), QueryRefundRequest{RefundNo: "stable_refund"})
		if err != nil || result.Outcome != expected {
			t.Fatalf("query status=%s result=%+v err=%v", status, result, err)
		}
	}
	timeoutAdapter := NewWeChatVirtualRefundAdapter(fakeWeChatVirtualRefundGateway{err: errors.New("timeout")})
	refund, _ := timeoutAdapter.RefundPayment(context.Background(), RefundPaymentRequest{})
	query, _ := timeoutAdapter.QueryRefund(context.Background(), QueryRefundRequest{})
	if refund.Outcome != RefundUnknown || query.Outcome != QueryRefundUnknown {
		t.Fatalf("timeout must be unknown refund=%s query=%s", refund.Outcome, query.Outcome)
	}
}
