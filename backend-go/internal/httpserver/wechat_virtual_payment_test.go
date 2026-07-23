package httpserver

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

func TestVirtualPaymentOfficialSignatureVector(t *testing.T) {
	body := []byte(`{"openid": "xxx", "user_ip": "127.0.0.1", "env": 0}`)
	if got := calcVirtualPaySig("/xpay/query_user_balance", body, "12345"); got != "c37809f27c6d7fd1837ad2500a04512b66b34fd793a39a385fade56dca89a4b5" {
		t.Fatalf("paySig mismatch: %s", got)
	}
	if got := calcVirtualSignature(body, "9hAb/NEYUlkaMBEsmFgzig=="); got != "089d9e8dc5d308977360c4b79ec600a93d736802802a807d634192328032f6c7" {
		t.Fatalf("signature mismatch: %s", got)
	}
}

func TestVirtualPaymentSignDataJSONIsStable(t *testing.T) {
	payload, err := json.Marshal(virtualPaySignData{
		OfferID: "offer", BuyQuantity: 1, Env: 1, CurrencyType: "CNY", ProductID: "product",
		GoodsPrice: 99600, OutTradeNo: "ZQY202607150001", Attach: "ZQY202607150001",
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"offerId":"offer","buyQuantity":1,"env":1,"currencyType":"CNY","productId":"product","goodsPrice":99600,"outTradeNo":"ZQY202607150001","attach":"ZQY202607150001"}`
	if string(payload) != expected {
		t.Fatalf("unexpected JSON serialization:\n%s", payload)
	}
}

func TestPointRechargeRequiresActiveMemberOrAgent(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		memberLevel string
		agentStatus string
		expiresAt   string
		want        bool
	}{
		{name: "free user", memberLevel: "FREE", want: false},
		{name: "stale basic user without expiry", memberLevel: "BASIC", want: false},
		{name: "active basic member", memberLevel: "BASIC", expiresAt: now.Add(24 * time.Hour).Format(time.RFC3339Nano), want: true},
		{name: "pro member without expiry", memberLevel: "PRO", want: false},
		{name: "active member", memberLevel: "PRO", expiresAt: now.Add(24 * time.Hour).Format(time.RFC3339Nano), want: true},
		{name: "expired member", memberLevel: "PRO", expiresAt: now.Add(-time.Second).Format(time.RFC3339Nano), want: false},
		{name: "active agent", memberLevel: "FREE", agentStatus: "ACTIVE", want: true},
		{name: "inactive agent", memberLevel: "FREE", agentStatus: "NONE", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := pointRechargeIdentityEligible(test.memberLevel, test.agentStatus, test.expiresAt, now); got != test.want {
				t.Fatalf("eligibility = %v, want %v", got, test.want)
			}
		})
	}
}

func TestVerifyWeChatNotifySignature(t *testing.T) {
	parts := []string{"notify-token", "1721000000", "nonce-value"}
	sort.Strings(parts)
	digest := sha1.Sum([]byte(strings.Join(parts, "")))
	signature := hex.EncodeToString(digest[:])
	if !verifyWeChatNotifySignature("notify-token", signature, "1721000000", "nonce-value") {
		t.Fatal("valid signature was rejected")
	}
	if verifyWeChatNotifySignature("notify-token", strings.Repeat("0", 40), "1721000000", "nonce-value") {
		t.Fatal("invalid signature was accepted")
	}
}

func TestVirtualPaymentConfigSelectsEnvironmentKey(t *testing.T) {
	cfg := virtualPaymentConfigFromApp(config.Config{
		WeChatVirtualPayEnabled: true, WeChatVirtualPayEnv: "sandbox", WeChatVirtualPayOfferID: "offer",
		WeChatVirtualPayAppKey: "production-key", WeChatVirtualPaySandboxKey: "sandbox-key",
		WeChatVirtualPayNotifyToken: "token", WeChatMiniProgramAppID: "appid", WeChatMiniProgramSecret: "secret",
	})
	if cfg.Env != 1 || cfg.AppKey != "sandbox-key" || !cfg.ready() {
		t.Fatalf("unexpected sandbox config: %#v", cfg)
	}
}

func TestVirtualPaymentSnapshotUsesOnlyServerProduct(t *testing.T) {
	product := virtualPaymentProduct{
		Code: "MEMBER_PRO_YEAR_996", Name: "知启云AI Pro年度会员", ProductType: "MEMBERSHIP", PlanType: planTypeMemberPackage,
		PriceCents: 99600, MemberLevel: "PRO", MemberDays: 365, CreditUnits: 40000,
		OfferID: "offer", WeChatProductID: "wechat-product", Mode: "short_series_goods", Env: 1,
	}
	snapshot := snapshotForVirtualProduct(product)
	if snapshot.AmountCents != 99600 || snapshot.CreditUnits != 40000 || snapshot.MemberDays != 365 || snapshot.ProductType != "MEMBERSHIP" || snapshot.PlanType != planTypeMemberPackage {
		t.Fatalf("server product was not preserved: %#v", snapshot)
	}
}

func TestTwo996ProductsRemainIndependentAndShareCommissionTemplate(t *testing.T) {
	member, ok := planCatalogByID("plan_ai_creator_996")
	if !ok {
		t.Fatal("membership plan is missing")
	}
	agent, ok := planCatalogByID("plan_agent_join_996")
	if !ok {
		t.Fatal("agent plan is missing")
	}
	if member.PriceCents != 99600 || member.GrantPoints != 40000 || member.DurationDays != 365 || member.MemberLevel != "PRO" {
		t.Fatalf("unexpected membership plan: %+v", member)
	}
	if agent.PriceCents != 99600 || agent.GrantPoints != 20000 || agent.DurationDays != 0 || agent.AgentLevel != "AGENT" {
		t.Fatalf("unexpected agent plan: %+v", agent)
	}
	if member.PlanType == agent.PlanType || stringValue(member.Entitlements["productType"]) == stringValue(agent.Entitlements["productType"]) {
		t.Fatalf("996 products were collapsed: member=%+v agent=%+v", member, agent)
	}
	if commissionTemplateCode(member) != "COMMISSION_996_STANDARD" || commissionTemplateCode(agent) != "COMMISSION_996_STANDARD" {
		t.Fatalf("996 commission template mismatch: member=%s agent=%s", commissionTemplateCode(member), commissionTemplateCode(agent))
	}
}

func TestExistingAgentKeepsOriginalReferralWhenIdentityIsFulfilledAgain(t *testing.T) {
	data := adminPlatformData{ChannelAgents: []adminChannelAgent{{
		ID: "agent-self", UserID: "user-agent", ParentID: "original-parent", OperationCenterID: "original-center",
		Status: "ACTIVE", InviteCode: "KEEP-ME",
	}}}
	order := adminOrder{ID: "order-agent", UserID: "user-agent", DirectAgentID: "new-parent", OperationCenterID: "new-center", AmountCents: 99600}
	ensureAgentForUser(&data, adminUser{ID: "user-agent"}, &order, commissionSettlementResult{TokenGrantAmount: 20000}, time.Now().UTC().Format(time.RFC3339Nano))
	got := data.ChannelAgents[0]
	if got.ParentID != "original-parent" || got.OperationCenterID != "original-center" || got.InviteCode != "KEEP-ME" {
		t.Fatalf("existing referral was overwritten: %+v", got)
	}
}

func TestVirtualPaymentTokenOnlySnapshotUsesServerAmountAndGrant(t *testing.T) {
	product := virtualPaymentProduct{
		Code: "TOKEN_10000", Name: "100 元 Token 包", ProductType: "TOKEN_ONLY", PlanType: planTypeTokenRecharge,
		PriceCents: 10000, MemberDays: 730, CreditUnits: 10000,
		OfferID: "offer", WeChatProductID: "wechat-token", Mode: "short_series_goods", Env: 0,
	}
	snapshot := snapshotForVirtualProduct(product)
	if snapshot.AmountCents != 10000 || snapshot.CreditUnits != 10000 || snapshot.MemberDays != 730 || snapshot.ProductType != "TOKEN_ONLY" || snapshot.PlanType != planTypeTokenRecharge {
		t.Fatalf("server token product was not preserved: %#v", snapshot)
	}
}

func TestVirtualPaymentCustomQuantityUsesServerUnitPriceAndGrant(t *testing.T) {
	product := virtualPaymentProduct{
		Code: "TOKEN_CUSTOM_1YUAN", Name: "自定义金额充值", ProductType: "TOKEN_ONLY", PlanType: planTypeTokenRecharge,
		PriceCents: 100, CreditUnits: 100, CustomQuantity: true, MinQuantity: 1, MaxQuantity: 5000,
		OfferID: "offer", WeChatProductID: "wechat-custom-token", Mode: "short_series_goods", Env: 0,
	}
	quantity, err := virtualPurchaseQuantity(product, 1)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := snapshotForVirtualProductQuantity(product, quantity)
	if snapshot.AmountCents != 100 || snapshot.CreditUnits != 100 || snapshot.BuyQuantity != 1 || snapshot.UnitPriceCents != 100 || snapshot.UnitCreditUnits != 100 {
		t.Fatalf("unexpected one-yuan snapshot: %#v", snapshot)
	}
	quantity, err = virtualPurchaseQuantity(product, 150)
	if err != nil {
		t.Fatal(err)
	}
	snapshot = snapshotForVirtualProductQuantity(product, quantity)
	if snapshot.AmountCents != 15000 || snapshot.CreditUnits != 15000 || snapshot.BuyQuantity != 150 {
		t.Fatalf("unexpected custom snapshot: %#v", snapshot)
	}
	if _, err := virtualPurchaseQuantity(product, 5001); !errors.Is(err, errVirtualQuantityInvalid) {
		t.Fatalf("out-of-range quantity was not rejected: %v", err)
	}
	fixed := product
	fixed.CustomQuantity = false
	if _, err := virtualPurchaseQuantity(fixed, 2); !errors.Is(err, errVirtualQuantityInvalid) {
		t.Fatalf("fixed product quantity was not rejected: %v", err)
	}
}

func TestVirtualPaymentTenCentProductUsesIntegerCents(t *testing.T) {
	product := virtualPaymentProduct{
		Code: "TOKEN_TEST_10FEN", Name: "Token payment integration test", ProductType: "TOKEN_ONLY", PlanType: planTypeTokenRecharge,
		PriceCents: 10, CreditUnits: 10,
		OfferID: "offer", WeChatProductID: "TOKEN_TEST_10FEN", Mode: "short_series_goods", Env: 0,
	}
	quantity, err := virtualPurchaseQuantity(product, 1)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := snapshotForVirtualProductQuantity(product, quantity)
	if snapshot.AmountCents != 10 || snapshot.CreditUnits != 10 || snapshot.BuyQuantity != 1 || snapshot.UnitPriceCents != 10 {
		t.Fatalf("unexpected ten-cent snapshot: %#v", snapshot)
	}
}

func TestVirtualPaymentOneCentProductUsesIntegerCents(t *testing.T) {
	product := virtualPaymentProduct{
		Code: "TOKEN_TEST_1FEN", Name: "Token payment one-cent integration test", ProductType: "TOKEN_ONLY", PlanType: planTypeTokenRecharge,
		PriceCents: 1, CreditUnits: 1,
		OfferID: "offer", WeChatProductID: "TOKEN_TEST_1FEN", Mode: "short_series_goods", Env: 0,
	}
	quantity, err := virtualPurchaseQuantity(product, 1)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := snapshotForVirtualProductQuantity(product, quantity)
	if snapshot.AmountCents != 1 || snapshot.CreditUnits != 1 || snapshot.BuyQuantity != 1 || snapshot.UnitPriceCents != 1 {
		t.Fatalf("unexpected one-cent snapshot: %#v", snapshot)
	}
}

func TestValidateVirtualPaymentConfirmationRejectsForgedAmount(t *testing.T) {
	order := lockedVirtualOrder{
		OrderNo: "ZQY1", ProductCode: "MEMBER_PRO_YEAR_996", AmountCents: 99600, Status: virtualOrderPending,
		Snapshot: virtualOrderSnapshot{ProductCode: "MEMBER_PRO_YEAR_996", AmountCents: 99600},
	}
	err := validateVirtualPaymentConfirmation(order, virtualPayNotification{AmountCents: 8000})
	if !errors.Is(err, errVirtualPaymentMismatch) {
		t.Fatalf("forged amount was not rejected: %v", err)
	}
}

func TestValidateVirtualPaymentConfirmationRejectsWrongPayer(t *testing.T) {
	order := lockedVirtualOrder{
		OrderNo: "ZQY1", ProductCode: "IMAGE_PACK_1000", AmountCents: 8000, Status: virtualOrderPending,
		WeChatOpenIDHash: hashSensitiveIdentifier("openid-a"),
		Snapshot:         virtualOrderSnapshot{ProductCode: "IMAGE_PACK_1000", AmountCents: 8000},
	}
	err := validateVirtualPaymentConfirmation(order, virtualPayNotification{AmountCents: 8000, OpenID: "openid-b"})
	if !errors.Is(err, errVirtualPaymentMismatch) {
		t.Fatalf("wrong payer was not rejected: %v", err)
	}
}

func TestValidateVirtualPaymentConfirmationRejectsWrongProduct(t *testing.T) {
	order := lockedVirtualOrder{
		OrderNo: "ZQY1", ProductCode: "IMAGE_PACK_1000", AmountCents: 8000, Status: virtualOrderPending,
		Snapshot: virtualOrderSnapshot{ProductCode: "IMAGE_PACK_1000", WeChatProductID: "wechat-image", AmountCents: 8000},
	}
	err := validateVirtualPaymentConfirmation(order, virtualPayNotification{AmountCents: 8000, ProductID: "wechat-member", Quantity: 1})
	if !errors.Is(err, errVirtualPaymentMismatch) {
		t.Fatalf("wrong product was not rejected: %v", err)
	}
}

func TestValidateVirtualPaymentConfirmationRejectsWrongEnvironment(t *testing.T) {
	order := lockedVirtualOrder{
		OrderNo: "ZQY1", ProductCode: "IMAGE_PACK_1000", AmountCents: 8000, Status: virtualOrderPending,
		Snapshot: virtualOrderSnapshot{ProductCode: "IMAGE_PACK_1000", AmountCents: 8000, Env: 0},
	}
	err := validateVirtualPaymentConfirmation(order, virtualPayNotification{AmountCents: 8000, Env: 1, HasEnv: true})
	if !errors.Is(err, errVirtualPaymentMismatch) {
		t.Fatalf("wrong payment environment was not rejected: %v", err)
	}
}

func TestValidateVirtualPaymentConfirmationChecksCustomQuantity(t *testing.T) {
	order := lockedVirtualOrder{
		OrderNo: "ZQY1", ProductCode: "TOKEN_CUSTOM_1YUAN", AmountCents: 300, Status: virtualOrderPending,
		Snapshot: virtualOrderSnapshot{ProductCode: "TOKEN_CUSTOM_1YUAN", AmountCents: 300, BuyQuantity: 3},
	}
	if err := validateVirtualPaymentConfirmation(order, virtualPayNotification{AmountCents: 300, Quantity: 2}); !errors.Is(err, errVirtualPaymentMismatch) {
		t.Fatalf("wrong custom quantity was not rejected: %v", err)
	}
	if err := validateVirtualPaymentConfirmation(order, virtualPayNotification{AmountCents: 300, Quantity: 3}); err != nil {
		t.Fatalf("valid custom quantity was rejected: %v", err)
	}
}

func TestValidateWechatQueryOrderResponseRejectsWrongOrderAndEnvironment(t *testing.T) {
	var response wechatQueryOrderResponse
	response.Order.OrderID = "ZQY-OTHER"
	response.Order.EnvType = 1
	if err := validateWechatQueryOrderResponse("ZQY-EXPECTED", 0, response); !errors.Is(err, errVirtualPaymentMismatch) {
		t.Fatalf("wrong queried order was not rejected: %v", err)
	}
	response.Order.OrderID = "ZQY-EXPECTED"
	response.Order.EnvType = 2
	if err := validateWechatQueryOrderResponse("ZQY-EXPECTED", 0, response); !errors.Is(err, errVirtualPaymentMismatch) {
		t.Fatalf("wrong queried environment was not rejected: %v", err)
	}
}

func TestParseVirtualPaymentJSONNotification(t *testing.T) {
	item, err := parseVirtualPayNotification([]byte(`{"Event":"xpay_goods_deliver_notify","OutTradeNo":"ZQY12345678","Env":1,"FromUserName":"official-openid","OpenId":"payer-openid","WeChatPayInfo":{"MchOrderNo":"wx-order","TransactionId":"wx-trade","PaidTime":1721000000},"GoodsInfo":{"ProductId":"wechat-image","Quantity":1,"ActualPrice":8000}}`))
	if err != nil {
		t.Fatal(err)
	}
	if item.OrderNo != "ZQY12345678" || item.OpenID != "payer-openid" || item.AmountCents != 8000 || item.ProductID != "wechat-image" || item.Quantity != 1 || item.TransactionID != "wx-trade" || item.WeChatOrderID != "wx-order" || !item.HasEnv || item.Env != 1 {
		t.Fatalf("unexpected notification: %#v", item)
	}
}

func TestVirtualPaymentProductsRequireAuthentication(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := newWithStoreAndSessions(config.Config{
		Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir(), AdminStaticDir: t.TempDir(),
	}, newJSONStore(dataPath), newLocalAuthSessions())
	recorder := request(t, server.Handler, http.MethodGet, "/api/v1/payment/products", nil)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated product list status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestParseVirtualPaymentXMLNotification(t *testing.T) {
	item, err := parseVirtualPayNotification([]byte(`<xml><Event>xpay_goods_deliver_notify</Event><OutTradeNo>ZQY87654321</OutTradeNo><Env>0</Env><FromUserName>official-openid</FromUserName><OpenId>payer-openid</OpenId><WeChatPayInfo><MchOrderNo>wx-order</MchOrderNo><TransactionId>wx-trade</TransactionId><PaidTime>1721000000</PaidTime></WeChatPayInfo><GoodsInfo><ProductId>wechat-member</ProductId><Quantity>1</Quantity><ActualPrice>99600</ActualPrice></GoodsInfo></xml>`))
	if err != nil {
		t.Fatal(err)
	}
	if item.OrderNo != "ZQY87654321" || item.OpenID != "payer-openid" || item.AmountCents != 99600 || item.ProductID != "wechat-member" || item.TransactionID != "wx-trade" || !item.HasEnv || item.Env != 0 {
		t.Fatalf("unexpected XML notification: %#v", item)
	}
}

func TestVirtualNotifyResponseMatchesRequestEncoding(t *testing.T) {
	jsonRecorder := httptest.NewRecorder()
	writeVirtualNotifyResponse(jsonRecorder, http.StatusOK, false, 0, "success")
	if jsonRecorder.Code != http.StatusOK || !strings.Contains(jsonRecorder.Header().Get("Content-Type"), "application/json") || !strings.Contains(jsonRecorder.Body.String(), `"ErrCode":0`) {
		t.Fatalf("unexpected JSON response: code=%d contentType=%s body=%s", jsonRecorder.Code, jsonRecorder.Header().Get("Content-Type"), jsonRecorder.Body.String())
	}
	xmlRecorder := httptest.NewRecorder()
	writeVirtualNotifyResponse(xmlRecorder, http.StatusInternalServerError, true, -1, "retry")
	if xmlRecorder.Code != http.StatusInternalServerError || !strings.Contains(xmlRecorder.Header().Get("Content-Type"), "application/xml") || !strings.Contains(xmlRecorder.Body.String(), "<ErrCode>-1</ErrCode>") {
		t.Fatalf("unexpected XML response: code=%d contentType=%s body=%s", xmlRecorder.Code, xmlRecorder.Header().Get("Content-Type"), xmlRecorder.Body.String())
	}
}

func TestMembershipStartsAtPaymentWhenExpired(t *testing.T) {
	paidAt := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	effective, expires := membershipExtensionWindow("2026-01-01T00:00:00Z", paidAt, 365)
	if !effective.Equal(paidAt) || !expires.Equal(paidAt.AddDate(0, 0, 365)) {
		t.Fatalf("unexpected expired membership window: %s -> %s", effective, expires)
	}
}

func TestMembershipExtendsFromCurrentExpiry(t *testing.T) {
	paidAt := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	currentExpiry := paidAt.AddDate(0, 1, 0)
	effective, expires := membershipExtensionWindow(currentExpiry.Format(time.RFC3339Nano), paidAt, 365)
	if !effective.Equal(currentExpiry) || !expires.Equal(currentExpiry.AddDate(0, 0, 365)) {
		t.Fatalf("unexpected active membership window: %s -> %s", effective, expires)
	}
}

func TestWechatSessionStoreExpiresSessionKey(t *testing.T) {
	store := newLocalAuthSessions().(*localAuthSessions)
	ctx := context.Background()
	if err := store.PutWeChatSession(ctx, "user", wechatMiniProgramSession{OpenID: "openid", SessionKey: "session"}, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Millisecond)
	if _, ok, err := store.WeChatSession(ctx, "user"); err != nil || ok {
		t.Fatalf("expired session remained available: ok=%v err=%v", ok, err)
	}
}

func TestVirtualPaymentDefaultTenantAliasUsesPersonalScope(t *testing.T) {
	service := &virtualPaymentService{}
	tenantID, err := service.resolveTenant(context.Background(), adminUser{ID: "wechat-user"}, "tenant_default")
	if err != nil {
		t.Fatal(err)
	}
	if tenantID != "personal:wechat-user" {
		t.Fatalf("unexpected personal tenant: %q", tenantID)
	}
}

func TestVirtualBusinessNumberMeetsWeChatConstraint(t *testing.T) {
	orderNo := newVirtualBusinessNo("ZQY")
	if len(orderNo) < 8 || len(orderNo) > 32 || strings.HasPrefix(orderNo, "_") {
		t.Fatalf("invalid order number: %q", orderNo)
	}
	for _, char := range orderNo {
		if !(char >= '0' && char <= '9') && !(char >= 'A' && char <= 'Z') {
			t.Fatalf("invalid order number character %q in %q", char, orderNo)
		}
	}
}
