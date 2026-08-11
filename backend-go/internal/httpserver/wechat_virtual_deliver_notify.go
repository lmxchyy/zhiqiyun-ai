package httpserver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	notifyProvideGoodsURI     = "/xpay/notify_provide_goods"
	virtualDeliverNotifyEvent = "notify_provide_goods"
)

type wechatNotifyProvideGoodsRequest struct {
	OrderID   string `json:"order_id,omitempty"`
	WxOrderID string `json:"wx_order_id,omitempty"`
	Env       int    `json:"env"`
}

type wechatNotifyProvideGoodsResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

func deliverNotifyEventID(orderNo string) string {
	return virtualDeliverNotifyEvent + ":" + strings.TrimSpace(orderNo)
}

func deliverNotifyIdempotencyKey(orderNo string) string {
	return "WECHAT_VIRTUAL:" + deliverNotifyEventID(orderNo)
}

// notifyProvideGoodsForOrder tells WeChat the cash order is already shipped.
// Official docs: after a successful xpay_goods_deliver_notify response this is unnecessary;
// it is required when fulfillment happened via query_order compensation (or other non-push paths).
// Local entitlements must already be SUCCESS; this call never grants or re-grants rights.
func (s *virtualPaymentService) notifyProvideGoodsForOrder(ctx context.Context, orderNo string) error {
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return errVirtualOrderNotFound
	}
	if s == nil || !s.cfg.ready() {
		return errVirtualPaymentUnavailable
	}

	var status, entitlementStatus, wechatOrderID string
	var env int
	err := s.db.QueryRowContext(ctx, `
		select status, entitlement_status, coalesce(wechat_order_id, ''),
		       case when upper(coalesce(payment_environment,'')) in ('SANDBOX','1') then 1
		            when coalesce((price_snapshot->>'env')::int, 0) = 1 then 1
		            else 0 end
		from xz_orders
		where order_no = $1 and payment_channel = $2
	`, orderNo, virtualPaymentChannel).Scan(&status, &entitlementStatus, &wechatOrderID, &env)
	if errors.Is(err, sql.ErrNoRows) {
		return errVirtualOrderNotFound
	}
	if err != nil {
		return err
	}
	if !strings.EqualFold(status, virtualOrderPaid) || !strings.EqualFold(entitlementStatus, entitlementSuccess) {
		return fmt.Errorf("order %s is not paid and fulfilled (status=%s entitlement=%s)", orderNo, status, entitlementStatus)
	}
	if s.cfg.Env == 1 {
		env = 1
	}

	already, err := s.deliverNotifyAlreadySucceeded(ctx, orderNo)
	if err != nil {
		return err
	}
	if already {
		return nil
	}

	if err := s.markDeliverNotifyProcessing(ctx, orderNo, wechatOrderID, env); err != nil {
		return err
	}

	callErr := s.callNotifyProvideGoods(ctx, orderNo, wechatOrderID, env)
	if callErr != nil {
		_ = s.markDeliverNotifyResult(ctx, orderNo, "FAILED", truncateVirtualPaymentError(callErr))
		return callErr
	}
	return s.markDeliverNotifyResult(ctx, orderNo, "SUCCESS", "")
}

func (s *virtualPaymentService) callNotifyProvideGoods(ctx context.Context, orderID string, wxOrderID string, env int) error {
	request := wechatNotifyProvideGoodsRequest{Env: env}
	if strings.TrimSpace(orderID) != "" {
		request.OrderID = strings.TrimSpace(orderID)
	} else if strings.TrimSpace(wxOrderID) != "" {
		request.WxOrderID = strings.TrimSpace(wxOrderID)
	} else {
		return fmt.Errorf("notify_provide_goods requires order_id or wx_order_id")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	accessToken, err := s.wechatAccessToken(ctx)
	if err != nil {
		return err
	}
	paySig := calcVirtualPaySig(notifyProvideGoodsURI, body, s.cfg.AppKey)
	endpoint := "https://api.weixin.qq.com/xpay/notify_provide_goods?access_token=" +
		url.QueryEscape(accessToken) + "&pay_sig=" + url.QueryEscape(paySig)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(httpRequest)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("wechat notify_provide_goods status %d", response.StatusCode)
	}
	var result wechatNotifyProvideGoodsResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return err
	}
	if isNotifyProvideGoodsSuccess(result) {
		return nil
	}
	return fmt.Errorf("wechat notify_provide_goods failed: code=%d message=%s", result.ErrCode, strings.TrimSpace(result.ErrMsg))
}

func isNotifyProvideGoodsSuccess(result wechatNotifyProvideGoodsResponse) bool {
	if result.ErrCode == 0 {
		return true
	}
	// 268490004 = duplicate / already applied for several xpay write APIs.
	if result.ErrCode == 268490004 {
		return true
	}
	msg := strings.ToLower(strings.TrimSpace(result.ErrMsg))
	if strings.Contains(msg, "already") || strings.Contains(msg, "已发货") || strings.Contains(msg, "重复") {
		return true
	}
	return false
}

func (s *virtualPaymentService) deliverNotifyAlreadySucceeded(ctx context.Context, orderNo string) (bool, error) {
	var status string
	err := s.db.QueryRowContext(ctx, `
		select processing_status from xz_payment_events
		where idempotency_key = $1
	`, deliverNotifyIdempotencyKey(orderNo)).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.EqualFold(status, "SUCCESS"), nil
}

func (s *virtualPaymentService) markDeliverNotifyProcessing(ctx context.Context, orderNo string, wechatOrderID string, env int) error {
	eventID := deliverNotifyEventID(orderNo)
	idempotencyKey := deliverNotifyIdempotencyKey(orderNo)
	resourceID := virtualPaymentResourceID("payment_event", idempotencyKey)
	rawJSON, _ := json.Marshal(map[string]any{
		"event": virtualDeliverNotifyEvent, "orderNo": orderNo, "wechatOrderId": wechatOrderID, "env": env,
	})
	_, err := s.db.ExecContext(ctx, `
		insert into xz_payment_events(
			id, provider, event_id, event_type, order_id, transaction_id, amount_cents,
			raw, raw_body, verified, idempotency_key, status, processing_status,
			process_attempts, error_message
		) values ($1,$2,$3,$4,$5,null,0,$6::jsonb,$7,true,$8,'RECEIVED','PROCESSING',1,'')
		on conflict (idempotency_key) do update set
			process_attempts = xz_payment_events.process_attempts + 1,
			processing_status = case when xz_payment_events.processing_status = 'SUCCESS'
				then xz_payment_events.processing_status else 'PROCESSING' end,
			error_message = case when xz_payment_events.processing_status = 'SUCCESS'
				then xz_payment_events.error_message else '' end,
			raw = excluded.raw,
			raw_body = excluded.raw_body
	`, resourceID, virtualPaymentChannel, eventID, virtualDeliverNotifyEvent, orderNo,
		rawJSON, string(rawJSON), idempotencyKey)
	return err
}

func (s *virtualPaymentService) markDeliverNotifyResult(ctx context.Context, orderNo string, status string, errorMessage string) error {
	_, err := s.db.ExecContext(ctx, `
		update xz_payment_events
		set processing_status = $2, status = $2, error_message = $3,
		    processed_at = case when $2 = 'SUCCESS' then now() else processed_at end
		where idempotency_key = $1
	`, deliverNotifyIdempotencyKey(orderNo), status, errorMessage)
	return err
}

func shouldNotifyProvideGoodsAfterEvent(event string) bool {
	// Successful deliver-notify push response already marks the order shipped on WeChat side.
	return !strings.EqualFold(strings.TrimSpace(event), virtualGoodsNotify)
}

func (s *virtualPaymentService) maybeNotifyProvideGoodsAfterGrant(ctx context.Context, orderNo string, sourceEvent string) {
	if !shouldNotifyProvideGoodsAfterEvent(sourceEvent) {
		return
	}
	if err := s.notifyProvideGoodsForOrder(ctx, orderNo); err != nil {
		// Local grant already committed; leave FAILED event for admin/oneshot retry.
		_ = err
	}
}
