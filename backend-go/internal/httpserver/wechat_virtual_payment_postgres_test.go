package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestWechatVirtualPaymentPostgresLifecycle(t *testing.T) {
	databaseURL := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	userID := "wxvp_user_" + suffix
	wrongUserID := "wxvp_wrong_" + suffix
	prefix := "WXVP" + suffix
	tenantID := "personal:" + userID
	openid := "openid_" + suffix
	initialExpiry := time.Now().UTC().AddDate(0, 1, 0).Truncate(time.Second)
	insertVirtualTestUser(t, ctx, db, userID, initialExpiry)
	insertVirtualTestUser(t, ctx, db, wrongUserID, time.Time{})
	insertVirtualTestPointAccount(t, ctx, db, userID)
	defer cleanupVirtualPaymentTest(t, db, prefix, userID, wrongUserID)

	localSessions := newLocalAuthSessions().(*localAuthSessions)
	if err := localSessions.PutWeChatSession(ctx, userID, wechatMiniProgramSession{OpenID: openid, SessionKey: "session-key"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	service := &virtualPaymentService{
		db: db, sessions: localSessions, cfg: virtualPaymentConfig{
			Enabled: true, Env: 1, OfferID: "test-offer", AppKey: "test-app-key", NotifyToken: "test-token",
			Mode: "short_series_goods", AppID: "test-appid", AppSecret: "test-secret",
		},
	}
	service.accessToken = "mock-access-token"
	service.accessTokenExp = time.Now().Add(time.Hour)
	service.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"errcode":0,"errmsg":"ok"}`))}, nil
	})}

	t.Run("server product controls amount and session is required", func(t *testing.T) {
		created, err := service.createOrder(ctx, adminUser{ID: userID}, "", "MEMBER_PRO_YEAR_996")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanupVirtualOrderByNo(t, db, created.OrderNo)
		})
		if created.AmountCent != 99600 {
			t.Fatalf("server amount mismatch: %d", created.AmountCent)
		}
		var storedAmount int64
		var creditUnits int64
		var commissionTemplate string
		var commissionRuleCount int
		if err := db.QueryRowContext(ctx, `select amount_cents, (price_snapshot->>'creditUnits')::bigint, coalesce(price_snapshot->>'commissionTemplateCode',''), jsonb_array_length(coalesce(price_snapshot->'commissionRules','[]'::jsonb)) from xz_orders where order_no = $1`, created.OrderNo).Scan(&storedAmount, &creditUnits, &commissionTemplate, &commissionRuleCount); err != nil {
			t.Fatal(err)
		}
		if storedAmount != 99600 || creditUnits != 40000 || commissionTemplate != "COMMISSION_996_STANDARD" || commissionRuleCount != 3 {
			t.Fatalf("immutable snapshot mismatch: amount=%d credits=%d template=%s rules=%d", storedAmount, creditUnits, commissionTemplate, commissionRuleCount)
		}
		agentCreated, err := service.createOrder(ctx, adminUser{ID: userID}, "", "AGENT_STANDARD_996")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanupVirtualOrderByNo(t, db, agentCreated.OrderNo)
		})
		var agentLevel string
		var agentTokens int64
		if err := db.QueryRowContext(ctx, `select coalesce(price_snapshot->>'agentLevel', ''), (price_snapshot->>'creditUnits')::bigint from xz_orders where order_no = $1`, agentCreated.OrderNo).Scan(&agentLevel, &agentTokens); err != nil {
			t.Fatal(err)
		}
		if agentCreated.AmountCent != 99600 || agentLevel != "AGENT" || agentTokens != 20000 {
			t.Fatalf("agent upgrade snapshot mismatch: amount=%d level=%s tokens=%d", agentCreated.AmountCent, agentLevel, agentTokens)
		}
		if _, err := (&virtualPaymentService{db: db, sessions: newLocalAuthSessions().(*localAuthSessions), cfg: service.cfg}).createOrder(ctx, adminUser{ID: userID}, "", "MEMBER_PRO_YEAR_996"); !errors.Is(err, errVirtualPaymentRelogin) {
			t.Fatalf("missing session_key was not rejected: %v", err)
		}
	})

	t.Run("annual membership and credits are atomic and idempotent", func(t *testing.T) {
		orderNo := prefix + "MEMBER"
		insertVirtualTestOrder(t, ctx, db, orderNo, tenantID, userID, "plan_ai_creator_996", virtualOrderPaid, virtualOrderSnapshot{
			ProductCode: "MEMBER_PRO_YEAR_996", ProductName: "知启云AI Pro年度会员", ProductType: "MEMBERSHIP", PlanType: planTypeMemberPackage,
			AmountCents: 99600, MemberLevel: "PRO", MemberDays: 365, CreditUnits: 40000,
			OfferID: "offer", WeChatProductID: "member", Mode: "short_series_goods", Env: 1,
		}, openid)
		beforeCredits := virtualTestCreditBalance(t, ctx, db, userID)
		if err := service.GrantOrderEntitlements(ctx, orderNo); err != nil {
			t.Fatal(err)
		}
		if err := service.GrantOrderEntitlements(ctx, orderNo); err != nil {
			t.Fatal(err)
		}
		afterCredits := virtualTestCreditBalance(t, ctx, db, userID)
		if afterCredits-beforeCredits != 40000 {
			t.Fatalf("credits grant mismatch: before=%d after=%d", beforeCredits, afterCredits)
		}
		var membershipCount int
		var tokenCount int
		var expiresText string
		if err := db.QueryRowContext(ctx, `select count(*) from xz_membership_entitlement_records where source_order_no = $1`, orderNo).Scan(&membershipCount); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `select count(*) from xz_token_records where order_id = $1 or source_order_no = $1`, orderNo).Scan(&tokenCount); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `select subscription_expires_at from xz_users where id = $1`, userID).Scan(&expiresText); err != nil {
			t.Fatal(err)
		}
		expires, err := time.Parse(time.RFC3339Nano, expiresText)
		if err != nil {
			t.Fatal(err)
		}
		if membershipCount != 1 || tokenCount != 1 || !expires.Equal(initialExpiry.AddDate(0, 0, 365)) {
			t.Fatalf("membership idempotency mismatch: memberships=%d tokens=%d expires=%s", membershipCount, tokenCount, expires)
		}
		assertPersonalPointGrantRows(t, ctx, db, userID, orderNo, PointSourceMemberPackageGrant, int64(beforeCredits+40000), 1)
	})

	t.Run("ten-cent custom recharge completes the full signed callback flow", func(t *testing.T) {
		beforeCredits := virtualTestCreditBalance(t, ctx, db, userID)
		created, err := service.createOrder(ctx, adminUser{ID: userID}, "", "TOKEN_CUSTOM_1YUAN", 1)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cleanupVirtualOrderByNo(t, db, created.OrderNo) })
		var signData virtualPaySignData
		if err := json.Unmarshal([]byte(created.SignData), &signData); err != nil {
			t.Fatal(err)
		}
		if created.AmountCent != 10 || signData.BuyQuantity != 1 || signData.GoodsPrice != 10 || signData.ProductID != "TOKEN_CUSTOM_1YUAN" || created.PaySig == "" || created.Signature == "" {
			t.Fatalf("ten-cent signed order mismatch: amount=%d signData=%+v", created.AmountCent, signData)
		}
		notification := virtualPayNotification{
			ID: "event_" + created.OrderNo, Event: virtualGoodsNotify, OrderNo: created.OrderNo, OpenID: openid,
			WeChatOrderID: "wx_order_one_yuan_" + suffix, TransactionID: "wx_trade_one_yuan_" + suffix,
			ProductID: "TOKEN_CUSTOM_1YUAN", Quantity: 1, AmountCents: 10, Env: 1, HasEnv: true,
			PaidAt: time.Now().UTC(), Raw: []byte(`{"simulated":true}`),
		}
		if err := service.confirmPaidAndGrant(ctx, notification); err != nil {
			t.Fatal(err)
		}
		if err := service.confirmPaidAndGrant(ctx, notification); err != nil {
			t.Fatal(err)
		}
		afterCredits := virtualTestCreditBalance(t, ctx, db, userID)
		var orderStatus string
		var entitlementStatus string
		var paymentStatus string
		var ledgerCount int
		var eventCount int
		var snapshotQuantity int64
		var snapshotTokens int64
		if err := db.QueryRowContext(ctx, `select status, entitlement_status, (price_snapshot->>'buyQuantity')::bigint, (price_snapshot->>'creditUnits')::bigint from xz_orders where order_no = $1`, created.OrderNo).Scan(&orderStatus, &entitlementStatus, &snapshotQuantity, &snapshotTokens); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `select prepay_status from xz_payment_records where order_no = $1`, created.OrderNo).Scan(&paymentStatus); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `select count(*) from xz_wallet_ledger where reference_id = $1 and entry_type = 'RECHARGE'`, created.OrderNo).Scan(&ledgerCount); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `select count(*) from xz_billing_events where task_id = $1 and metric_code = 'compute.recharge'`, created.OrderNo).Scan(&eventCount); err != nil {
			t.Fatal(err)
		}
		if afterCredits-beforeCredits != 10 || orderStatus != virtualOrderPaid || entitlementStatus != entitlementSuccess || paymentStatus != virtualOrderPaid || ledgerCount != 1 || eventCount != 1 || snapshotQuantity != 1 || snapshotTokens != 10 {
			t.Fatalf("ten-cent flow mismatch: before=%d after=%d order=%s entitlement=%s payment=%s ledger=%d events=%d quantity=%d tokens=%d", beforeCredits, afterCredits, orderStatus, entitlementStatus, paymentStatus, ledgerCount, eventCount, snapshotQuantity, snapshotTokens)
		}
		t.Logf("simulated ten-cent flow: amount=10 quantity=%d tokens=+%d order=%s entitlement=%s payment=%s walletLedger=%d billingEvents=%d", snapshotQuantity, afterCredits-beforeCredits, orderStatus, entitlementStatus, paymentStatus, ledgerCount, eventCount)
		assertPersonalPointGrantRows(t, ctx, db, userID, created.OrderNo, PointSourceRecharge, int64(afterCredits), 1)
	})

	t.Run("coupon bonus and commercial billing records follow the paid order", func(t *testing.T) {
		couponID := "coupon_" + strings.ToLower(suffix)
		couponCode := "WXVP" + strings.ToUpper(suffix)
		_, err := db.ExecContext(ctx, `
			insert into xz_billing_coupons(id,code,name,benefit_type,benefit_value,applicable_product_codes,status)
			values($1,$2,'Virtual payment test coupon','BONUS_CREDITS',25,array['TOKEN_CUSTOM_1YUAN'],'ACTIVE')
		`, couponID, couponCode)
		if err != nil {
			t.Fatal(err)
		}
		beforeCredits := virtualTestCreditBalance(t, ctx, db, userID)
		created, err := service.createOrderWithCoupon(ctx, adminUser{ID: userID}, "", "TOKEN_CUSTOM_1YUAN", 1, couponCode)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanupVirtualOrderByNo(t, db, created.OrderNo)
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cleanupCancel()
			_, _ = db.ExecContext(cleanupCtx, `delete from xz_billing_coupons where id=$1`, couponID)
		})
		notification := virtualPayNotification{
			ID: "event_coupon_" + created.OrderNo, Event: virtualGoodsNotify, OrderNo: created.OrderNo, OpenID: openid,
			WeChatOrderID: "wx_coupon_order_" + suffix, TransactionID: "wx_coupon_trade_" + suffix,
			ProductID: "TOKEN_CUSTOM_1YUAN", Quantity: 1, AmountCents: 10, Env: 1, HasEnv: true,
			PaidAt: time.Now().UTC(), Raw: []byte(`{"simulated":true}`),
		}
		if err := service.confirmPaidAndGrant(ctx, notification); err != nil {
			t.Fatal(err)
		}
		if err := service.confirmPaidAndGrant(ctx, notification); err != nil {
			t.Fatal(err)
		}
		afterCredits := virtualTestCreditBalance(t, ctx, db, userID)
		var redemptionStatus, invoiceStatus, requestStatus string
		var invoiceCount, requestCount int
		if err := db.QueryRowContext(ctx, `select status from xz_billing_coupon_redemptions where order_no=$1`, created.OrderNo).Scan(&redemptionStatus); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `select count(*),max(payment_status) from xz_billing_invoices where order_no=$1`, created.OrderNo).Scan(&invoiceCount, &invoiceStatus); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `select count(*),max(status) from xz_billing_payment_requests where order_no=$1`, created.OrderNo).Scan(&requestCount, &requestStatus); err != nil {
			t.Fatal(err)
		}
		if afterCredits-beforeCredits != 35 || redemptionStatus != "APPLIED" || invoiceCount != 1 || invoiceStatus != "PAID" || requestCount != 1 || requestStatus != "SUCCEEDED" {
			t.Fatalf("commercial coupon flow mismatch: credits=%d redemption=%s invoices=%d/%s requests=%d/%s", afterCredits-beforeCredits, redemptionStatus, invoiceCount, invoiceStatus, requestCount, requestStatus)
		}
		assertPersonalPointGrantRows(t, ctx, db, userID, created.OrderNo, PointSourceRecharge, int64(afterCredits), 1)
		assertPersonalPointGrantRows(t, ctx, db, userID, created.OrderNo, PointSourceWechatVirtualCoupon, int64(afterCredits), 1)
	})

	t.Run("TOKEN_ONLY recharge grants the configured token amount once", func(t *testing.T) {
		orderNo := prefix + "TOKENONLY"
		insertVirtualTestOrder(t, ctx, db, orderNo, tenantID, userID, "recharge_100", virtualOrderPaid, virtualOrderSnapshot{
			ProductCode: "TOKEN_10000", ProductName: "100 元点数包", ProductType: "TOKEN_ONLY", PlanType: planTypeTokenRecharge,
			AmountCents: 10000, MemberDays: 730, CreditUnits: 10000,
			OfferID: "offer", WeChatProductID: "token-10000", Mode: "short_series_goods", Env: 1,
		}, openid)
		beforeCredits := virtualTestCreditBalance(t, ctx, db, userID)
		if err := service.GrantOrderEntitlements(ctx, orderNo); err != nil {
			t.Fatal(err)
		}
		if err := service.GrantOrderEntitlements(ctx, orderNo); err != nil {
			t.Fatal(err)
		}
		afterCredits := virtualTestCreditBalance(t, ctx, db, userID)
		var eventCount int
		var ledgerCount int
		if err := db.QueryRowContext(ctx, `select count(*) from xz_billing_events where task_id = $1 and metric_code = 'compute.recharge'`, orderNo).Scan(&eventCount); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `select count(*) from xz_wallet_ledger where reference_id = $1 and entry_type = 'RECHARGE'`, orderNo).Scan(&ledgerCount); err != nil {
			t.Fatal(err)
		}
		if afterCredits-beforeCredits != 10000 || eventCount != 1 || ledgerCount != 1 {
			t.Fatalf("TOKEN_ONLY idempotency mismatch: before=%d after=%d events=%d ledger=%d", beforeCredits, afterCredits, eventCount, ledgerCount)
		}
		assertPersonalPointGrantRows(t, ctx, db, userID, orderNo, PointSourceRecharge, int64(afterCredits), 1)
	})

	t.Run("IDENTITY grants points and agent identity atomically", func(t *testing.T) {
		orderNo := prefix + "AGENTUPGRADE"
		insertVirtualTestOrder(t, ctx, db, orderNo, tenantID, userID, "plan_agent_join_996", virtualOrderPaid, virtualOrderSnapshot{
			ProductCode: "AGENT_STANDARD_996", ProductName: "知启云AI官方代理商", ProductType: "IDENTITY", PlanType: planTypeAgentJoinPackage,
			AmountCents: 99600, AgentLevel: "AGENT", MemberDays: 0, CreditUnits: 20000,
			OfferID: "offer", WeChatProductID: "agent-996", Mode: "short_series_goods", Env: 1,
		}, openid)
		beforeCredits := virtualTestCreditBalance(t, ctx, db, userID)
		if err := service.GrantOrderEntitlements(ctx, orderNo); err != nil {
			t.Fatal(err)
		}
		if err := service.GrantOrderEntitlements(ctx, orderNo); err != nil {
			t.Fatal(err)
		}
		afterCredits := virtualTestCreditBalance(t, ctx, db, userID)
		var agentStatus string
		var channelCount int
		var profileCount int
		var tokenCount int
		if err := db.QueryRowContext(ctx, `select coalesce(agent_status, '') from xz_users where id = $1`, userID).Scan(&agentStatus); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `select count(*) from xz_channel_agents where user_id = $1 and join_order_id = $2`, userID, orderNo).Scan(&channelCount); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `select count(*) from xz_agent_profiles where user_id = $1 and join_order_id = $2`, userID, orderNo).Scan(&profileCount); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `select count(*) from xz_token_records where order_id = $1 and change_type = 'AGENT_JOIN_GRANT'`, orderNo).Scan(&tokenCount); err != nil {
			t.Fatal(err)
		}
		if afterCredits-beforeCredits != 20000 || agentStatus != agentStatusActive || channelCount != 1 || profileCount != 1 || tokenCount != 1 {
			t.Fatalf("IDENTITY mismatch: before=%d after=%d status=%s channel=%d profile=%d tokens=%d", beforeCredits, afterCredits, agentStatus, channelCount, profileCount, tokenCount)
		}
		assertPersonalPointGrantRows(t, ctx, db, userID, orderNo, PointSourceAgentJoinGrant, int64(afterCredits), 1)
	})

	t.Run("active agent can purchase membership without losing agent identity", func(t *testing.T) {
		orderNo := prefix + "AGENTMEMBER"
		insertVirtualTestOrder(t, ctx, db, orderNo, tenantID, userID, "plan_ai_creator_996", virtualOrderPaid, virtualOrderSnapshot{
			ProductCode: "MEMBER_PRO_YEAR_996", ProductName: "知启云AI Pro年度会员", ProductType: "MEMBERSHIP", PlanType: planTypeMemberPackage,
			AmountCents: 99600, MemberLevel: "PRO", MemberDays: 365, CreditUnits: 40000,
			OfferID: "offer", WeChatProductID: "member-after-agent", Mode: "short_series_goods", Env: 1,
		}, openid)
		beforeCredits := virtualTestCreditBalance(t, ctx, db, userID)
		if err := service.GrantOrderEntitlements(ctx, orderNo); err != nil {
			t.Fatal(err)
		}
		if err := service.GrantOrderEntitlements(ctx, orderNo); err != nil {
			t.Fatal(err)
		}
		afterCredits := virtualTestCreditBalance(t, ctx, db, userID)
		var agentStatus string
		var agentCount int
		if err := db.QueryRowContext(ctx, `select coalesce(agent_status,'') from xz_users where id=$1`, userID).Scan(&agentStatus); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `select count(*) from xz_channel_agents where user_id=$1 and status='ACTIVE'`, userID).Scan(&agentCount); err != nil {
			t.Fatal(err)
		}
		if afterCredits-beforeCredits != 40000 || agentStatus != agentStatusActive || agentCount != 1 {
			t.Fatalf("agent membership purchase mismatch: credits=%d status=%s agents=%d", afterCredits-beforeCredits, agentStatus, agentCount)
		}
	})

	t.Run("image quota callback is concurrent safe", func(t *testing.T) {
		orderNo := prefix + "IMAGE"
		insertVirtualTestOrder(t, ctx, db, orderNo, tenantID, userID, "plan_image_pack_1000", virtualOrderPending, virtualOrderSnapshot{
			ProductCode: "IMAGE_PACK_1000", ProductName: "1000张图片生成额度", ProductType: "IMAGE_QUOTA_PACK",
			AmountCents: 8000, ImageQuota: 1000, OfferID: "offer", WeChatProductID: "image", Mode: "short_series_goods", Env: 1,
		}, openid)
		notification := virtualPayNotification{
			ID: "event_" + orderNo, Event: virtualGoodsNotify, OrderNo: orderNo, OpenID: openid,
			WeChatOrderID: "wx_order_" + suffix, TransactionID: "wx_trade_" + suffix,
			AmountCents: 8000, PaidAt: time.Now().UTC(), Raw: []byte(`{"verified":true}`),
		}
		var wait sync.WaitGroup
		errorsFound := make(chan error, 2)
		for index := 0; index < 2; index++ {
			wait.Add(1)
			go func() {
				defer wait.Done()
				if err := service.confirmPaidAndGrant(context.Background(), notification); err != nil {
					errorsFound <- err
				}
			}()
		}
		wait.Wait()
		close(errorsFound)
		for err := range errorsFound {
			t.Fatal(err)
		}
		var remaining int64
		var ledgerCount int
		if err := db.QueryRowContext(ctx, `select remaining_images from xz_image_quota_accounts where tenant_id = $1 and user_id = $2`, tenantID, userID).Scan(&remaining); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `select count(*) from xz_image_quota_ledger where source_order_no = $1`, orderNo).Scan(&ledgerCount); err != nil {
			t.Fatal(err)
		}
		if remaining != 1000 || ledgerCount != 1 {
			t.Fatalf("image quota idempotency mismatch: remaining=%d ledger=%d", remaining, ledgerCount)
		}
		if _, err := service.ownedOrder(ctx, adminUser{ID: wrongUserID}, "", orderNo); !errors.Is(err, errVirtualOrderNotFound) {
			t.Fatalf("non-owner order access was not rejected: %v", err)
		}
	})

	t.Run("wechat query compensation confirms payment and grants entitlement", func(t *testing.T) {
		orderNo := prefix + "SYNC"
		insertVirtualTestOrder(t, ctx, db, orderNo, tenantID, userID, "plan_image_pack_1000", virtualOrderPending, virtualOrderSnapshot{
			ProductCode: "IMAGE_PACK_1000", ProductName: "1000张图片生成额度", ProductType: "IMAGE_QUOTA_PACK",
			AmountCents: 8000, ImageQuota: 1000, OfferID: "offer", WeChatProductID: "image", Mode: "short_series_goods", Env: 1,
		}, openid)
		service.accessToken = "mock-access-token"
		service.accessTokenExp = time.Now().Add(time.Hour)
		service.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Path == "/xpay/notify_provide_goods" {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"errcode":0,"errmsg":"ok"}`))}, nil
			}
			if request.URL.Path != "/xpay/query_order" || request.URL.Query().Get("pay_sig") == "" {
				t.Fatalf("unexpected query request: %s", request.URL.String())
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			var query wechatQueryOrderRequest
			if err := json.Unmarshal(body, &query); err != nil {
				t.Fatal(err)
			}
			if query.OpenID != openid || query.OrderID != orderNo || query.Env != 1 {
				t.Fatalf("wechat query body mismatch: %+v", query)
			}
			response := `{"errcode":0,"errmsg":"ok","order":{"order_id":"` + orderNo + `","status":2,"order_fee":8000,"paid_fee":8000,"paid_time":` + strconv.FormatInt(time.Now().Unix(), 10) + `,"wx_order_id":"wx_sync_` + suffix + `","wxpay_order_id":"wxpay_sync_` + suffix + `"}}`
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response))}, nil
		})}
		if err := service.syncOrder(ctx, userID, orderNo); err != nil {
			t.Fatal(err)
		}
		var orderStatus string
		var entitlementStatus string
		var imageBalance int64
		if err := db.QueryRowContext(ctx, `select status, entitlement_status from xz_orders where order_no = $1`, orderNo).Scan(&orderStatus, &entitlementStatus); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `select remaining_images from xz_image_quota_accounts where tenant_id = $1 and user_id = $2`, tenantID, userID).Scan(&imageBalance); err != nil {
			t.Fatal(err)
		}
		if orderStatus != virtualOrderPaid || entitlementStatus != entitlementSuccess || imageBalance != 2000 {
			t.Fatalf("query compensation mismatch: order=%s entitlement=%s imageBalance=%d", orderStatus, entitlementStatus, imageBalance)
		}
	})

	t.Run("transaction failure does not partially grant membership", func(t *testing.T) {
		orderNo := prefix + "ROLLBACK"
		insertVirtualTestOrder(t, ctx, db, orderNo, tenantID, userID, "plan_ai_creator_996", virtualOrderPaid, virtualOrderSnapshot{
			ProductCode: "MEMBER_PRO_YEAR_996", ProductName: "rollback", ProductType: "MEMBERSHIP",
			AmountCents: 99600, MemberLevel: "PRO", MemberDays: 365, CreditUnits: math.MaxInt64,
			OfferID: "offer", WeChatProductID: "member", Mode: "short_series_goods", Env: 1,
		}, openid)
		var beforeExpiry string
		if err := db.QueryRowContext(ctx, `select subscription_expires_at from xz_users where id = $1`, userID).Scan(&beforeExpiry); err != nil {
			t.Fatal(err)
		}
		if err := service.GrantOrderEntitlements(ctx, orderNo); err == nil {
			t.Fatal("expected wallet overflow failure")
		}
		var afterExpiry string
		var membershipCount int
		var tokenCount int
		if err := db.QueryRowContext(ctx, `select subscription_expires_at from xz_users where id = $1`, userID).Scan(&afterExpiry); err != nil {
			t.Fatal(err)
		}
		_ = db.QueryRowContext(ctx, `select count(*) from xz_membership_entitlement_records where source_order_no = $1`, orderNo).Scan(&membershipCount)
		_ = db.QueryRowContext(ctx, `select count(*) from xz_token_records where source_order_no = $1`, orderNo).Scan(&tokenCount)
		if beforeExpiry != afterExpiry || membershipCount != 0 || tokenCount != 0 {
			t.Fatalf("partial entitlement escaped rollback: before=%s after=%s memberships=%d tokens=%d", beforeExpiry, afterExpiry, membershipCount, tokenCount)
		}
		var lotCount int
		if err := db.QueryRowContext(ctx, `select count(*) from xz_personal_point_lots where user_id=$1 and reference_id=$2`, userID, orderNo).Scan(&lotCount); err != nil {
			t.Fatal(err)
		}
		if lotCount != 0 {
			t.Fatalf("partial entitlement escaped rollback into %d point lots", lotCount)
		}
	})

	t.Run("admin read models execute and redact secrets", func(t *testing.T) {
		api := virtualPaymentAPI{service: service}
		for _, testCase := range []struct {
			name    string
			handler http.HandlerFunc
		}{
			{name: "overview", handler: api.adminOverview},
			{name: "products", handler: api.adminProducts},
			{name: "orders", handler: api.adminList("orders")},
			{name: "records", handler: api.adminList("records")},
			{name: "notifications", handler: api.adminList("notifications")},
			{name: "memberships", handler: api.adminList("memberships")},
			{name: "wallet-ledger", handler: api.adminList("wallet-ledger")},
			{name: "refunds", handler: api.adminList("refunds")},
			{name: "failures", handler: api.adminList("failures")},
		} {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/payment/virtual/"+testCase.name+"?limit=5", nil)
			testCase.handler(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("admin %s returned %d: %s", testCase.name, recorder.Code, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), service.cfg.AppKey) || strings.Contains(recorder.Body.String(), service.cfg.NotifyToken) {
				t.Fatalf("admin %s exposed a payment secret", testCase.name)
			}
		}
	})

	t.Run("commercial billing read models execute against database", func(t *testing.T) {
		admin := adminAPI{store: &postgresStore{db: db, ready: true}}
		for _, view := range []string{"customers", "products", "subscriptions", "coupons", "invoices", "creditNotes", "paymentRequests", "payments"} {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/billing/"+view, nil)
			admin.commercialBillingList(recorder, request, view)
			if recorder.Code != http.StatusOK {
				t.Fatalf("commercial billing %s returned %d: %s", view, recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"source":"DATABASE"`) {
				t.Fatalf("commercial billing %s did not report database source: %s", view, recorder.Body.String())
			}
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func insertVirtualTestUser(t *testing.T, ctx context.Context, db *sql.DB, userID string, expiry time.Time) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	expires := ""
	if !expiry.IsZero() {
		expires = expiry.Format(time.RFC3339Nano)
	}
	user := adminUser{ID: userID, Email: userID + "@test.local", Name: "Virtual Pay Test", Role: "MEMBER", Status: "ACTIVE", MemberLevel: "PRO", PlanID: "plan_ai_creator_996", SubscriptionExpiresAt: expires, CreatedAt: now, UpdatedAt: now}
	_, err := db.ExecContext(ctx, `
		insert into xz_users(id, email, name, role, member_level, status, plan_id, subscription_expires_at, created_at, updated_at, raw)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9,$10::jsonb)
	`, user.ID, user.Email, user.Name, user.Role, user.MemberLevel, user.Status, user.PlanID, user.SubscriptionExpiresAt, now, jsonProjection(user))
	if err != nil {
		t.Fatal(err)
	}
}

func insertVirtualTestOrder(t *testing.T, ctx context.Context, db *sql.DB, orderNo string, tenantID string, userID string, planID string, status string, snapshot virtualOrderSnapshot, openid string) {
	t.Helper()
	now := time.Now().UTC()
	snapshotJSON, _ := json.Marshal(snapshot)
	raw, _ := json.Marshal(map[string]any{
		"id": orderNo, "orderNo": orderNo, "tenantId": tenantID, "userId": userID,
		"planId": planID, "amountCents": snapshot.AmountCents, "status": status,
		"paymentMethod": virtualPaymentChannel, "fulfillmentStatus": entitlementPending,
		"priceSnapshot": snapshot, "createdAt": now.Format(time.RFC3339Nano),
	})
	paidAt := ""
	if status == virtualOrderPaid {
		paidAt = now.Format(time.RFC3339Nano)
	}
	_, err := db.ExecContext(ctx, `
		insert into xz_orders(
		  id, order_no, tenant_id, user_id, buyer_user_id, plan_id, order_type, business_order_type,
		  amount_cents, status, paid_at, fulfillment_status, entitlement_status, product_code,
		  product_name, product_type, payment_channel, payment_scene, payment_mode, wechat_openid_hash,
		  created_at, updated_at, price_snapshot, reward_snapshot, raw
		) values ($1,$1,$2,$3,$3,$4,'VIRTUAL_PRODUCT','VIRTUAL_PRODUCT',$5,$6,$7,$8,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18::jsonb,'{}'::jsonb,$19::jsonb)
	`, orderNo, tenantID, userID, planID, snapshot.AmountCents, status, paidAt, entitlementPending,
		snapshot.ProductCode, snapshot.ProductName, snapshot.ProductType, virtualPaymentChannel,
		virtualPaymentScene, snapshot.Mode, hashSensitiveIdentifier(openid), now.Format(time.RFC3339Nano), now, snapshotJSON, raw)
	if err != nil {
		t.Fatal(err)
	}
}

func insertVirtualTestPointAccount(t *testing.T, ctx context.Context, db *sql.DB, userID string) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	item := adminPointAccount{ID: virtualPaymentResourceID("test_points", userID), UserID: userID}
	if err := insertPointAccount(ctx, tx, item); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func virtualTestCreditBalance(t *testing.T, ctx context.Context, db *sql.DB, userID string) int64 {
	t.Helper()
	var balance int64
	if err := db.QueryRowContext(ctx, `select coalesce(max(available), 0) from xz_point_accounts where user_id = $1`, userID).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	return balance
}

func cleanupVirtualPaymentTest(t *testing.T, db *sql.DB, prefix string, userIDs ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cleanupImmutableCommissionTestRows(t, ctx, db, prefix)
	cleanupImmutableIdentityChangeTestRows(t, ctx, db, userIDs)
	cleanupVirtualPersonalPointTestRows(t, ctx, db, userIDs)
	statements := []string{
		`delete from xz_billing_dunning_events where payment_request_id in (select id from xz_billing_payment_requests where order_no like $1)`,
		`delete from xz_billing_credit_notes where order_no like $1`,
		`delete from xz_billing_coupon_redemptions where order_no like $1`,
		`delete from xz_billing_subscriptions where source_order_no like $1`,
		`delete from xz_billing_payment_requests where order_no like $1`,
		`delete from xz_billing_invoices where order_no like $1`,
		`delete from xz_refund_records where order_no like $1`,
		`delete from xz_payment_events where order_id like $1`,
		`delete from xz_membership_entitlement_records where source_order_no like $1`,
		`delete from xz_image_quota_ledger where source_order_no like $1`,
		`delete from xz_token_records where order_id like $1 or source_order_no like $1`,
		`delete from xz_billing_events where task_id like $1`,
		`delete from xz_wallet_ledger where reference_id like $1`,
		`delete from xz_commissions where order_id like $1`,
		`delete from xz_payment_records where order_no like $1`,
		`delete from xz_orders where order_no like $1`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement, prefix+"%"); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	}
	for _, userID := range userIDs {
		for _, statement := range []string{
			`delete from xz_image_quota_accounts where user_id = $1`,
			`delete from xz_agent_wallets where user_id = $1`,
			`delete from xz_agent_profiles where user_id = $1`,
			`delete from xz_user_relationships where user_id = $1`,
			`delete from xz_user_business_identities where user_id = $1`,
			`delete from xz_channel_agents where user_id = $1`,
			`delete from xz_user_wallets where user_id = $1`,
			`delete from xz_point_accounts where user_id = $1`,
			`delete from xz_users where id = $1`,
		} {
			if _, err := db.ExecContext(ctx, statement, userID); err != nil {
				t.Logf("cleanup failed: %v", err)
			}
		}
	}
}

func cleanupVirtualPersonalPointTestRows(t *testing.T, ctx context.Context, db *sql.DB, userIDs []string) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Errorf("personal point cleanup transaction: %v", err)
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `set local session_replication_role = replica`); err != nil {
		t.Errorf("personal point cleanup trigger isolation: %v", err)
		return
	}
	for _, userID := range userIDs {
		for _, statement := range []string{
			`delete from xz_personal_point_lot_movements where user_id=$1`,
			`delete from xz_personal_point_reservation_allocations where user_id=$1`,
			`delete from xz_personal_point_reservations where user_id=$1`,
			`delete from xz_personal_point_lots where user_id=$1`,
		} {
			if _, err := tx.ExecContext(ctx, statement, userID); err != nil {
				t.Errorf("personal point cleanup: %v", err)
				return
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Errorf("personal point cleanup commit: %v", err)
	}
}

func cleanupVirtualOrderByNo(t *testing.T, db *sql.DB, orderNo string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, statement := range []string{
		`delete from xz_billing_dunning_events where payment_request_id in (select id from xz_billing_payment_requests where order_no = $1)`,
		`delete from xz_billing_credit_notes where order_no = $1`,
		`delete from xz_billing_coupon_redemptions where order_no = $1`,
		`delete from xz_billing_subscriptions where source_order_no = $1`,
		`delete from xz_billing_payment_requests where order_no = $1`,
		`delete from xz_billing_invoices where order_no = $1`,
		`delete from xz_refund_records where order_no = $1`,
		`delete from xz_payment_events where order_id = $1`,
		`delete from xz_membership_entitlement_records where source_order_no = $1`,
		`delete from xz_image_quota_ledger where source_order_no = $1`,
		`delete from xz_token_records where order_id = $1 or source_order_no = $1`,
		`delete from xz_billing_events where task_id = $1`,
		`delete from xz_wallet_ledger where reference_id = $1`,
		`delete from xz_commissions where order_id = $1`,
		`delete from xz_payment_records where order_no = $1`,
		`delete from xz_orders where order_no = $1`,
	} {
		if _, err := db.ExecContext(ctx, statement, orderNo); err != nil {
			t.Logf("order cleanup failed: %v", err)
		}
	}
}

func cleanupImmutableCommissionTestRows(t *testing.T, ctx context.Context, db *sql.DB, prefix string) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Errorf("commission cleanup transaction: %v", err)
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `set local session_replication_role = replica`); err != nil {
		t.Errorf("commission cleanup trigger isolation: %v", err)
		return
	}
	if _, err := tx.ExecContext(ctx, `delete from xz_commission_records where order_no like $1`, prefix+"%"); err != nil {
		t.Errorf("commission cleanup: %v", err)
		return
	}
	if err := tx.Commit(); err != nil {
		t.Errorf("commission cleanup commit: %v", err)
	}
}

func cleanupImmutableIdentityChangeTestRows(t *testing.T, ctx context.Context, db *sql.DB, userIDs []string) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Errorf("identity change cleanup transaction: %v", err)
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `set local session_replication_role = replica`); err != nil {
		t.Errorf("identity change cleanup trigger isolation: %v", err)
		return
	}
	for _, userID := range userIDs {
		if _, err := tx.ExecContext(ctx, `delete from xz_identity_change_records where user_id=$1`, userID); err != nil {
			t.Errorf("identity change cleanup: %v", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		t.Errorf("identity change cleanup commit: %v", err)
	}
}
