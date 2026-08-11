package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"xianzhi-ai/backend-go/internal/app/operationcenter"
	"xianzhi-ai/backend-go/internal/app/payment"
)

func TestOperationCenterProviderUnavailableMapsToServiceUnavailable(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeOperationCenterRefundError(recorder, fmt.Errorf("%w: payment_channel=missing", operationcenter.ErrRefundProviderUnavailable))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestWechatVirtualRuntimeRefundDoesNotFakeActiveRefundSuccess(t *testing.T) {
	provider := newWechatVirtualRefundProvider(&virtualPaymentService{})
	result, err := provider.RefundPayment(context.Background(), payment.RefundPaymentRequest{RefundNo: "stable-refund"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != payment.RefundUnsupported || result.ProviderRefundID != "" {
		t.Fatalf("wechat virtual active refund must use manual path: %+v", result)
	}
}
