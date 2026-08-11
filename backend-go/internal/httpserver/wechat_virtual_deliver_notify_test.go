package httpserver

import (
	"encoding/json"
	"testing"
)

func TestShouldNotifyProvideGoodsAfterEvent(t *testing.T) {
	if shouldNotifyProvideGoodsAfterEvent(virtualGoodsNotify) {
		t.Fatal("deliver-notify push path should not call notify_provide_goods")
	}
	if !shouldNotifyProvideGoodsAfterEvent("query_order_paid") {
		t.Fatal("query compensation path must call notify_provide_goods")
	}
	if !shouldNotifyProvideGoodsAfterEvent("grant_order_entitlements") {
		t.Fatal("admin/compensation grant path must call notify_provide_goods")
	}
}

func TestIsNotifyProvideGoodsSuccess(t *testing.T) {
	if !isNotifyProvideGoodsSuccess(wechatNotifyProvideGoodsResponse{ErrCode: 0, ErrMsg: "ok"}) {
		t.Fatal("errcode 0 must succeed")
	}
	if !isNotifyProvideGoodsSuccess(wechatNotifyProvideGoodsResponse{ErrCode: 268490004, ErrMsg: "duplicate"}) {
		t.Fatal("duplicate xpay write must be treated as success")
	}
	if !isNotifyProvideGoodsSuccess(wechatNotifyProvideGoodsResponse{ErrCode: 1, ErrMsg: "订单已发货"}) {
		t.Fatal("already-shipped message must be treated as success")
	}
	if isNotifyProvideGoodsSuccess(wechatNotifyProvideGoodsResponse{ErrCode: 268490003, ErrMsg: "sign error"}) {
		t.Fatal("signature errors must not be treated as success")
	}
}

func TestNotifyProvideGoodsRequestJSON(t *testing.T) {
	body, err := json.Marshal(wechatNotifyProvideGoodsRequest{OrderID: "ZQY1", Env: 0})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"order_id":"ZQY1","env":0}` {
		t.Fatalf("unexpected body: %s", body)
	}
	paySig := calcVirtualPaySig(notifyProvideGoodsURI, body, "test-app-key")
	if paySig == "" || len(paySig) != 64 {
		t.Fatalf("unexpected pay_sig length: %q", paySig)
	}
	withWx, err := json.Marshal(wechatNotifyProvideGoodsRequest{WxOrderID: "VPO1", Env: 1})
	if err != nil {
		t.Fatal(err)
	}
	if string(withWx) != `{"wx_order_id":"VPO1","env":1}` {
		t.Fatalf("unexpected wx body: %s", withWx)
	}
}

func TestDeliverNotifyEventIdentity(t *testing.T) {
	if got := deliverNotifyEventID("ZQY1"); got != "notify_provide_goods:ZQY1" {
		t.Fatalf("event id = %s", got)
	}
	if got := deliverNotifyIdempotencyKey("ZQY1"); got != "WECHAT_VIRTUAL:notify_provide_goods:ZQY1" {
		t.Fatalf("idempotency key = %s", got)
	}
}
