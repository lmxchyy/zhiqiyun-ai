package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	paymentapp "xianzhi-ai/backend-go/internal/app/payment"
)

type wechatVirtualRefundGateway struct {
	service *virtualPaymentService
}

func newWechatVirtualRefundProvider(service *virtualPaymentService) paymentapp.RefundProvider {
	return paymentapp.NewWeChatVirtualRefundAdapter(&wechatVirtualRefundGateway{service: service})
}

func (gateway *wechatVirtualRefundGateway) RefundVirtualPayment(context.Context, paymentapp.WeChatVirtualRefundRequest) (paymentapp.WeChatVirtualRefundResponse, error) {
	return paymentapp.WeChatVirtualRefundResponse{Code: "ACTIVE_REFUND_NOT_IMPLEMENTED", Status: "UNSUPPORTED", Summary: paymentapp.ProviderResponseSummary{"provider": "wechat_virtual", "code": "ACTIVE_REFUND_NOT_IMPLEMENTED"}}, nil
}

func (gateway *wechatVirtualRefundGateway) QueryVirtualRefund(ctx context.Context, request paymentapp.WeChatVirtualRefundRequest) (paymentapp.WeChatVirtualRefundResponse, error) {
	if gateway == nil || gateway.service == nil || !gateway.service.cfg.ready() {
		return paymentapp.WeChatVirtualRefundResponse{Code: "GATEWAY_UNAVAILABLE", Status: "UNSUPPORTED", Summary: paymentapp.ProviderResponseSummary{"provider": "wechat_virtual", "code": "GATEWAY_UNAVAILABLE"}}, nil
	}
	session, ok, err := gateway.service.sessions.WeChatSession(ctx, request.PayerID)
	if err != nil || !ok || strings.TrimSpace(session.OpenID) == "" {
		return paymentapp.WeChatVirtualRefundResponse{Code: "PAYER_SESSION_UNAVAILABLE", Status: "UNKNOWN", Summary: paymentapp.ProviderResponseSummary{"provider": "wechat_virtual", "code": "PAYER_SESSION_UNAVAILABLE"}}, err
	}
	payload, err := json.Marshal(wechatQueryOrderRequest{OpenID: session.OpenID, Env: gateway.service.cfg.Env, OrderID: request.OrderNo})
	if err != nil {
		return paymentapp.WeChatVirtualRefundResponse{}, err
	}
	token, err := gateway.service.wechatAccessToken(ctx)
	if err != nil {
		return paymentapp.WeChatVirtualRefundResponse{}, err
	}
	signature := calcVirtualPaySig(queryVirtualOrderURI, payload, gateway.service.cfg.AppKey)
	endpoint := "https://api.weixin.qq.com/xpay/query_order?access_token=" + url.QueryEscape(token) + "&pay_sig=" + url.QueryEscape(signature)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return paymentapp.WeChatVirtualRefundResponse{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := gateway.service.client.Do(httpRequest)
	if err != nil {
		return paymentapp.WeChatVirtualRefundResponse{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return paymentapp.WeChatVirtualRefundResponse{}, err
	}
	summary := paymentapp.ProviderResponseSummary{"provider": "wechat_virtual", "httpStatus": response.StatusCode}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		summary["code"] = fmt.Sprintf("HTTP_%d", response.StatusCode)
		return paymentapp.WeChatVirtualRefundResponse{Code: fmt.Sprint(summary["code"]), Status: "UNKNOWN", Summary: summary}, nil
	}
	var result wechatQueryOrderResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return paymentapp.WeChatVirtualRefundResponse{}, err
	}
	summary["errorCode"] = result.ErrCode
	summary["orderStatus"] = result.Order.Status
	if result.ErrCode != 0 {
		return paymentapp.WeChatVirtualRefundResponse{Code: fmt.Sprintf("WECHAT_%d", result.ErrCode), Status: "UNKNOWN", Summary: summary}, nil
	}
	status := "UNKNOWN"
	switch result.Order.Status {
	case 5, 8:
		status = "REFUNDED"
	case 7:
		status = "REFUND_FAILED"
	case 2, 3, 4:
		status = "PAID_WITHOUT_REFUND"
	default:
		status = "PROCESSING"
	}
	return paymentapp.WeChatVirtualRefundResponse{Code: "SUCCESS", Status: status, ProviderRefundID: strings.TrimSpace(result.Order.WxRefundID), Summary: summary}, nil
}
