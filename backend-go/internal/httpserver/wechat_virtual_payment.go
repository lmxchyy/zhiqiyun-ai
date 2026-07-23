package httpserver

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"xianzhi-ai/backend-go/internal/config"
)

const (
	virtualPaymentChannel = "WECHAT_VIRTUAL"
	virtualPaymentScene   = "WECHAT_MINI_PROGRAM_VIRTUAL"
	requestVirtualPayURI  = "requestVirtualPayment"
	queryVirtualOrderURI  = "/xpay/query_order"

	virtualOrderPending       = "PENDING"
	virtualOrderPaid          = "PAID"
	virtualOrderClosed        = "CLOSED"
	virtualOrderRefundPending = "REFUND_PENDING"
	virtualOrderRefunded      = "REFUNDED"

	entitlementPending    = "PENDING"
	entitlementProcessing = "PROCESSING"
	entitlementSuccess    = "SUCCESS"
	entitlementFailed     = "FAILED"

	virtualGoodsNotify  = "xpay_goods_deliver_notify"
	virtualRefundNotify = "xpay_refund_notify"
)

var (
	errVirtualPaymentUnavailable = errors.New("微信虚拟支付当前不可用")
	errVirtualPaymentRelogin     = errors.New("微信登录态已失效，请重新登录后支付")
	errVirtualOrderNotFound      = errors.New("订单不存在或无权访问")
	errVirtualProductUnavailable = errors.New("虚拟商品不存在、未上架或未完成微信映射")
	errVirtualPaymentMismatch    = errors.New("微信支付结果与本地订单不一致")
	errVirtualQuantityInvalid    = errors.New("自定义充值数量无效")
	errVirtualRechargeRestricted = errors.New("点数充值仅向有效会员或代理商开放")
)

type virtualPaymentConfig struct {
	Enabled     bool
	Env         int
	OfferID     string
	AppKey      string
	NotifyToken string
	Mode        string
	AppID       string
	AppSecret   string
}

func virtualPaymentConfigFromApp(cfg config.Config) virtualPaymentConfig {
	env := 0
	if strings.EqualFold(strings.TrimSpace(cfg.WeChatVirtualPayEnv), "sandbox") || strings.TrimSpace(cfg.WeChatVirtualPayEnv) == "1" {
		env = 1
	}
	appKey := strings.TrimSpace(cfg.WeChatVirtualPayAppKey)
	if env == 1 {
		appKey = strings.TrimSpace(cfg.WeChatVirtualPaySandboxKey)
	}
	mode := strings.TrimSpace(cfg.WeChatVirtualPayMode)
	if mode == "" {
		mode = "short_series_goods"
	}
	return virtualPaymentConfig{
		Enabled:     cfg.WeChatVirtualPayEnabled,
		Env:         env,
		OfferID:     strings.TrimSpace(cfg.WeChatVirtualPayOfferID),
		AppKey:      appKey,
		NotifyToken: strings.TrimSpace(cfg.WeChatVirtualPayNotifyToken),
		Mode:        mode,
		AppID:       strings.TrimSpace(cfg.WeChatMiniProgramAppID),
		AppSecret:   strings.TrimSpace(cfg.WeChatMiniProgramSecret),
	}
}

func (c virtualPaymentConfig) ready() bool {
	return c.Enabled && c.OfferID != "" && c.AppKey != "" && c.NotifyToken != "" && c.AppID != "" && c.AppSecret != ""
}

type virtualPaymentProduct struct {
	PlanID          string `json:"id"`
	Code            string `json:"productCode"`
	Name            string `json:"name"`
	ProductType     string `json:"productType"`
	PlanType        string `json:"planType"`
	PriceCents      int64  `json:"amountCent"`
	MemberLevel     string `json:"memberLevel,omitempty"`
	AgentLevel      string `json:"agentLevel,omitempty"`
	MemberDays      int64  `json:"memberDays,omitempty"`
	CreditUnits     int64  `json:"creditUnits,omitempty"`
	ImageQuota      int64  `json:"imageQuota,omitempty"`
	CustomQuantity  bool   `json:"customQuantity,omitempty"`
	MinQuantity     int64  `json:"minQuantity,omitempty"`
	MaxQuantity     int64  `json:"maxQuantity,omitempty"`
	OfferID         string `json:"-"`
	WeChatProductID string `json:"-"`
	Mode            string `json:"mode"`
	Env             int    `json:"env"`
	Active          bool   `json:"active"`
	ValidityText    string `json:"validityText"`
	Description     string `json:"description"`
}

type virtualOrderSnapshot struct {
	ProductCode                string                   `json:"productCode"`
	ProductName                string                   `json:"productName"`
	ProductType                string                   `json:"productType"`
	PlanType                   string                   `json:"planType"`
	AmountCents                int64                    `json:"amountCents"`
	MemberLevel                string                   `json:"memberLevel,omitempty"`
	AgentLevel                 string                   `json:"agentLevel,omitempty"`
	MemberDays                 int64                    `json:"memberDays,omitempty"`
	CreditUnits                int64                    `json:"creditUnits,omitempty"`
	ImageQuota                 int64                    `json:"imageQuota,omitempty"`
	BuyQuantity                int64                    `json:"buyQuantity"`
	UnitPriceCents             int64                    `json:"unitPriceCents"`
	UnitCreditUnits            int64                    `json:"unitCreditUnits,omitempty"`
	CustomQuantity             bool                     `json:"customQuantity,omitempty"`
	OfferID                    string                   `json:"offerId"`
	WeChatProductID            string                   `json:"wechatProductId"`
	Mode                       string                   `json:"mode"`
	Env                        int                      `json:"env"`
	CouponCode                 string                   `json:"couponCode,omitempty"`
	CouponBenefitType          string                   `json:"couponBenefitType,omitempty"`
	CouponBenefitValue         int64                    `json:"couponBenefitValue,omitempty"`
	CommissionTemplateCode     string                   `json:"commissionTemplateCode,omitempty"`
	CommissionSnapshotCaptured bool                     `json:"commissionSnapshotCaptured"`
	DirectAgentID              string                   `json:"directAgentId,omitempty"`
	ParentAgentID              string                   `json:"parentAgentId,omitempty"`
	OperationCenterID          string                   `json:"operationCenterId,omitempty"`
	CommissionRules            []commissionRuleSnapshot `json:"commissionRules"`
}

type virtualCouponBenefit struct {
	ID           string `json:"id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	BenefitType  string `json:"benefitType"`
	BenefitValue int64  `json:"benefitValue"`
}

type virtualPaySignData struct {
	OfferID      string `json:"offerId"`
	BuyQuantity  int64  `json:"buyQuantity"`
	Env          int    `json:"env"`
	CurrencyType string `json:"currencyType"`
	ProductID    string `json:"productId"`
	GoodsPrice   int64  `json:"goodsPrice"`
	OutTradeNo   string `json:"outTradeNo"`
	Attach       string `json:"attach"`
}

type virtualOrderView struct {
	OrderNo           string               `json:"orderNo"`
	Product           virtualOrderSnapshot `json:"product"`
	AmountCents       int64                `json:"amountCent"`
	OrderStatus       string               `json:"orderStatus"`
	PaymentStatus     string               `json:"paymentStatus"`
	EntitlementStatus string               `json:"entitlementStatus"`
	EntitlementError  string               `json:"entitlementError,omitempty"`
	WeChatOrderID     string               `json:"wechatOrderId,omitempty"`
	WeChatTradeNo     string               `json:"wechatTransactionId,omitempty"`
	PaidAt            *time.Time           `json:"paidAt,omitempty"`
	CreatedAt         string               `json:"createdAt"`
	UpdatedAt         time.Time            `json:"updatedAt"`
}

type createVirtualOrderResponse struct {
	OrderNo    string `json:"orderNo"`
	AmountCent int64  `json:"amountCent"`
	SignData   string `json:"signData"`
	PaySig     string `json:"paySig"`
	Signature  string `json:"signature"`
	Mode       string `json:"mode"`
}

type virtualPaymentService struct {
	db       *sql.DB
	redis    *redis.Client
	sessions wechatMiniProgramSessionStore
	cfg      virtualPaymentConfig
	client   *http.Client

	tokenMu        sync.Mutex
	accessToken    string
	accessTokenExp time.Time
}

type virtualPaymentAPI struct {
	service  *virtualPaymentService
	store    platformStore
	sessions authSessionStore
}

func newVirtualPaymentAPI(appConfig config.Config, store platformStore, sessions authSessionStore, redisClient *redis.Client) virtualPaymentAPI {
	result := virtualPaymentAPI{store: store, sessions: sessions}
	pgStore, hasDatabase := store.(*postgresStore)
	wechatSessions, hasWeChatSessions := sessions.(wechatMiniProgramSessionStore)
	if !hasDatabase || !hasWeChatSessions {
		return result
	}
	result.service = &virtualPaymentService{
		db:       pgStore.db,
		redis:    redisClient,
		sessions: wechatSessions,
		cfg:      virtualPaymentConfigFromApp(appConfig),
		client:   &http.Client{Timeout: 10 * time.Second},
	}
	if result.service.cfg.ready() {
		go result.service.runCompensationLoop()
	}
	return result
}

func calcVirtualPaySig(uri string, signData []byte, appKey string) string {
	return hmacSHA256Hex([]byte(uri+"&"+string(signData)), []byte(appKey))
}

func calcVirtualSignature(signData []byte, sessionKey string) string {
	return hmacSHA256Hex(signData, []byte(sessionKey))
}

func hmacSHA256Hex(message []byte, key []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(message)
	return hex.EncodeToString(mac.Sum(nil))
}

func hashSensitiveIdentifier(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(digest[:])
}

func virtualPaymentResourceID(prefix string, seed string) string {
	return prefix + "_" + hashSensitiveIdentifier(prefix + ":" + seed)[:24]
}

func (a virtualPaymentAPI) available(w http.ResponseWriter) bool {
	if a.service == nil || a.service.db == nil {
		writeError(w, http.StatusServiceUnavailable, errVirtualPaymentUnavailable)
		return false
	}
	return true
}

func (a virtualPaymentAPI) authenticatedUser(r *http.Request) (adminUser, error) {
	return (api{store: a.store, sessions: a.sessions}).currentUser(r)
}

func (a virtualPaymentAPI) products(w http.ResponseWriter, r *http.Request) {
	if _, err := a.authenticatedUser(r); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if !a.available(w) {
		return
	}
	products, err := a.service.listProducts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{
		"items":       products,
		"enabled":     a.service.cfg.ready(),
		"environment": map[int]string{0: "production", 1: "sandbox"}[a.service.cfg.Env],
	})
}

func (a virtualPaymentAPI) coupons(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if !a.available(w) {
		return
	}
	items, err := a.service.listAvailableCoupons(r.Context(), user.ID, strings.TrimSpace(r.URL.Query().Get("productCode")))
	if err != nil {
		writeVirtualPaymentError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "discountsPaymentAmount": false})
}

func (a virtualPaymentAPI) createOrder(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var request struct {
		ProductCode string `json:"productCode"`
		Quantity    int64  `json:"quantity"`
		CouponCode  string `json:"couponCode"`
		WxLoginCode string `json:"wxLoginCode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var paymentSession *wechatMiniProgramSession
	if code := strings.TrimSpace(request.WxLoginCode); code != "" {
		session, exchangeErr := exchangeWeChatMiniProgramCode(r.Context(), code)
		if exchangeErr != nil {
			writeJSONWithStatus(w, http.StatusUnauthorized, map[string]any{"code": "WECHAT_SESSION_REFRESH_FAILED", "error": "微信登录态刷新失败，请重试"})
			return
		}
		data, dataErr := a.store.AdminData()
		if dataErr != nil {
			writeError(w, http.StatusInternalServerError, dataErr)
			return
		}
		if existing, found := findUserByWechatIdentity(data.Users, session); found && existing.ID != user.ID {
			writeAuthFlowError(w, http.StatusConflict, "AUTH_WECHAT_ALREADY_BOUND", "该微信身份已绑定其他账号")
			return
		}
		updated, updateErr := a.store.UpdateAdminCustomer(user.ID, adminCustomerMutation{
			WeChatOpenID: session.OpenID, WeChatUnionID: session.UnionID,
		})
		if updateErr != nil {
			writeError(w, http.StatusInternalServerError, updateErr)
			return
		}
		if sessionStore, ok := a.sessions.(wechatMiniProgramSessionStore); ok {
			if storeErr := sessionStore.PutWeChatSession(r.Context(), updated.ID, session, authSessionTTL); storeErr != nil {
				writeError(w, http.StatusServiceUnavailable, errAuthSessionUnavailable)
				return
			}
		}
		user = updated
		paymentSession = &session
	}
	result, err := a.service.createOrderWithCouponAndSession(
		r.Context(), user, strings.TrimSpace(r.Header.Get("X-Tenant-Id")),
		request.ProductCode, request.Quantity, request.CouponCode, paymentSession,
	)
	if err != nil {
		writeVirtualPaymentError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, result)
}

func (a virtualPaymentAPI) order(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	item, err := a.service.ownedOrder(r.Context(), user, strings.TrimSpace(r.Header.Get("X-Tenant-Id")), r.PathValue("orderNo"))
	if err != nil {
		writeVirtualPaymentError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a virtualPaymentAPI) orderStatus(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	item, err := a.service.ownedOrder(r.Context(), user, strings.TrimSpace(r.Header.Get("X-Tenant-Id")), r.PathValue("orderNo"))
	if err != nil {
		writeVirtualPaymentError(w, err)
		return
	}
	balances, balanceErr := a.service.entitlementBalances(r.Context(), user.ID, item.OrderNo)
	if balanceErr != nil {
		writeVirtualPaymentError(w, balanceErr)
		return
	}
	writeJSON(w, map[string]any{
		"orderNo":           item.OrderNo,
		"orderStatus":       item.OrderStatus,
		"paymentStatus":     item.PaymentStatus,
		"entitlementStatus": item.EntitlementStatus,
		"entitlementError":  item.EntitlementError,
		"completed":         item.OrderStatus == virtualOrderPaid && item.EntitlementStatus == entitlementSuccess,
		"item":              item,
		"balances":          balances,
	})
}

func (s *virtualPaymentService) entitlementBalances(ctx context.Context, userID string, orderNo string) (map[string]any, error) {
	var tenantID string
	if err := s.db.QueryRowContext(ctx, `select tenant_id from xz_orders where order_no = $1 and user_id = $2`, orderNo, userID).Scan(&tenantID); err != nil {
		return nil, err
	}
	result := map[string]any{"creditBalance": int64(0), "imageQuota": int64(0), "memberLevel": "", "membershipExpiresAt": ""}
	var creditBalance int64
	if err := s.db.QueryRowContext(ctx, `select coalesce(max(available), 0) from xz_point_accounts where user_id = $1`, userID).Scan(&creditBalance); err != nil {
		return nil, err
	}
	result["creditBalance"] = creditBalance
	var imageQuota int64
	if err := s.db.QueryRowContext(ctx, `select coalesce(max(remaining_images), 0) from xz_image_quota_accounts where tenant_id = $1 and user_id = $2`, tenantID, userID).Scan(&imageQuota); err != nil {
		return nil, err
	}
	result["imageQuota"] = imageQuota
	var memberLevel string
	var expiresAt string
	if err := s.db.QueryRowContext(ctx, `select coalesce(member_level, raw->>'memberLevel', ''), coalesce(subscription_expires_at, '') from xz_users where id = $1`, userID).Scan(&memberLevel, &expiresAt); err != nil {
		return nil, err
	}
	result["memberLevel"] = memberLevel
	result["membershipExpiresAt"] = expiresAt
	return result, nil
}

func (a virtualPaymentAPI) syncOrder(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	item, err := a.service.syncOwnedOrder(r.Context(), user, strings.TrimSpace(r.Header.Get("X-Tenant-Id")), r.PathValue("orderNo"))
	if err != nil {
		writeVirtualPaymentError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item, "synced": true})
}

func (a virtualPaymentAPI) verifyNotify(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	query := r.URL.Query()
	if !verifyWeChatNotifySignature(a.service.cfg.NotifyToken, query.Get("signature"), query.Get("timestamp"), query.Get("nonce")) {
		writeError(w, http.StatusUnauthorized, errors.New("invalid wechat callback signature"))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, query.Get("echostr"))
}

func (a virtualPaymentAPI) notify(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	query := r.URL.Query()
	signature := firstNonEmptyString(query.Get("signature"), query.Get("msg_signature"))
	if !verifyWeChatNotifySignature(a.service.cfg.NotifyToken, signature, query.Get("timestamp"), query.Get("nonce")) {
		writeError(w, http.StatusUnauthorized, errors.New("invalid wechat callback signature"))
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	notification, err := parseVirtualPayNotification(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	isXML := len(bytes.TrimSpace(body)) > 0 && bytes.TrimSpace(body)[0] == '<'
	if err := a.service.processNotification(r.Context(), notification); err != nil {
		writeVirtualNotifyResponse(w, http.StatusInternalServerError, isXML, -1, "retry")
		return
	}
	writeVirtualNotifyResponse(w, http.StatusOK, isXML, 0, "success")
}

func writeVirtualNotifyResponse(w http.ResponseWriter, status int, asXML bool, code int, message string) {
	if asXML {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, "<xml><ErrCode>%d</ErrCode><ErrMsg><![CDATA[%s]]></ErrMsg></xml>", code, message)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"ErrCode": code, "ErrMsg": message})
}

func writeJSONWithStatus(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeVirtualPaymentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errUnauthorized):
		writeError(w, http.StatusUnauthorized, err)
	case errors.Is(err, errForbidden), errors.Is(err, errVirtualOrderNotFound), errors.Is(err, errVirtualRechargeRestricted):
		writeError(w, http.StatusForbidden, err)
	case errors.Is(err, errVirtualPaymentRelogin):
		writeJSONWithStatus(w, http.StatusUnauthorized, map[string]any{"code": "WECHAT_SESSION_EXPIRED", "error": err.Error()})
	case errors.Is(err, errVirtualProductUnavailable), errors.Is(err, errVirtualPaymentMismatch), errors.Is(err, errVirtualQuantityInvalid):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, errVirtualPaymentUnavailable):
		writeError(w, http.StatusServiceUnavailable, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}

func verifyWeChatNotifySignature(token string, signature string, timestamp string, nonce string) bool {
	if strings.TrimSpace(token) == "" || strings.TrimSpace(signature) == "" || strings.TrimSpace(timestamp) == "" || strings.TrimSpace(nonce) == "" {
		return false
	}
	parts := []string{token, timestamp, nonce}
	sort.Strings(parts)
	digest := sha1.Sum([]byte(strings.Join(parts, "")))
	expected := hex.EncodeToString(digest[:])
	actual := strings.ToLower(strings.TrimSpace(signature))
	return len(actual) == len(expected) && subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func (s *virtualPaymentService) listProducts(ctx context.Context) ([]virtualPaymentProduct, error) {
	rows, err := s.db.QueryContext(ctx, `
		select plan.id, plan.payment_product_code, plan.name,
		       coalesce(nullif(plan.product_type, ''), nullif(plan.plan_type, ''), plan.entitlements->>'productType', ''),
		       coalesce(nullif(plan.plan_type, ''), plan.entitlements->>'planType', plan.raw->>'planType', ''),
		       plan.price_cents, coalesce(plan.member_level, plan.entitlements->>'memberLevel', ''),
		       coalesce(plan.agent_level, plan.entitlements->>'agentLevel', ''),
		       case when coalesce(plan.duration_days, 0) > 0 then plan.duration_days
		            when coalesce(plan.entitlements->>'memberDays', '') ~ '^[0-9]+$' then (plan.entitlements->>'memberDays')::bigint else 0 end,
		       case when coalesce(plan.entitlements->>'bonusCreditCny', '') ~ '^[0-9]+$'
		              then (plan.entitlements->>'bonusCreditCny')::bigint * coalesce(
		                (select integer_value from xz_billing_config where config_key = 'CREDITS_PER_CNY_YUAN'), 100)
		            when coalesce(plan.entitlements->>'creditUnits', '') ~ '^[0-9]+$' then (plan.entitlements->>'creditUnits')::bigint
		            else coalesce(plan.grant_points, 0) end,
		       case when coalesce(plan.entitlements->>'imageQuota', '') ~ '^[0-9]+$' then (plan.entitlements->>'imageQuota')::bigint else 0 end,
		       lower(coalesce(plan.entitlements->>'customQuantity', 'false')) = 'true',
		       case when coalesce(plan.entitlements->>'minQuantity', '') ~ '^[0-9]+$' then (plan.entitlements->>'minQuantity')::bigint else 1 end,
		       case when coalesce(plan.entitlements->>'maxQuantity', '') ~ '^[0-9]+$' then (plan.entitlements->>'maxQuantity')::bigint else 1 end,
		       coalesce(mapping.offer_id, ''), mapping.wechat_product_id, mapping.mode, mapping.env, plan.active and mapping.enabled
		from xz_plans plan
		join xz_wechat_virtual_product_mappings mapping on mapping.plan_id = plan.id and mapping.env = $1
		where coalesce(plan.payment_product_code, '') <> ''
		  and ($1 <> 0 or lower(coalesce(plan.entitlements->>'testOnly', plan.raw->>'testOnly', 'false')) <> 'true')
		order by plan.price_cents, plan.id
	`, s.cfg.Env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []virtualPaymentProduct{}
	for rows.Next() {
		var item virtualPaymentProduct
		if err := rows.Scan(&item.PlanID, &item.Code, &item.Name, &item.ProductType, &item.PlanType, &item.PriceCents,
			&item.MemberLevel, &item.AgentLevel, &item.MemberDays, &item.CreditUnits, &item.ImageQuota,
			&item.CustomQuantity, &item.MinQuantity, &item.MaxQuantity, &item.OfferID,
			&item.WeChatProductID, &item.Mode, &item.Env, &item.Active); err != nil {
			return nil, err
		}
		item.ProductType = strings.ToUpper(strings.TrimSpace(item.ProductType))
		item.PlanType = normalizePlanTypeString(item.PlanType)
		item.OfferID = firstNonEmptyString(s.cfg.OfferID, item.OfferID)
		item.Mode = firstNonEmptyString(s.cfg.Mode, item.Mode, "short_series_goods")
		switch item.ProductType {
		case "TOKEN_ONLY":
			if item.CustomQuantity {
				item.ValidityText = fmt.Sprintf("可选%d-%d元", item.MinQuantity, item.MaxQuantity)
				item.Description = fmt.Sprintf("每1元到账%d Token｜金额由服务端计算", item.CreditUnits)
			} else {
				item.ValidityText = "仅限平台内使用"
				item.Description = fmt.Sprintf("充值%d Token｜不可提现或转账", item.CreditUnits)
			}
		case "IDENTITY":
			item.ValidityText = "官方代理商商业身份"
			item.Description = fmt.Sprintf("到账 %d 点，开通代理后台、推广与返佣权限", item.CreditUnits)
		case "MEMBERSHIP":
			item.ValidityText = fmt.Sprintf("%d天", item.MemberDays)
			item.Description = fmt.Sprintf("到账 %d 点，开通或续费 Pro 会员 %s", item.CreditUnits, item.ValidityText)
		case "TOKEN_UPGRADE":
			if item.PlanType == planTypeAgentJoinPackage {
				item.ValidityText = "开通代理商身份"
				item.Description = fmt.Sprintf("开通代理商身份 + %d Token", item.CreditUnits)
			} else {
				item.ValidityText = fmt.Sprintf("%d天", item.MemberDays)
				item.Description = fmt.Sprintf("开通%s会员%s + %d Token", item.MemberLevel, item.ValidityText, item.CreditUnits)
			}
		case "IMAGE_QUOTA_PACK":
			item.ValidityText = "长期有效"
			item.Description = fmt.Sprintf("%d张图片生成额度｜%s｜长期有效", item.ImageQuota, formatCNYCents(item.PriceCents))
		default:
			item.ValidityText = fmt.Sprintf("%d天", item.MemberDays)
			item.Description = fmt.Sprintf("%s｜%s", item.Name, formatCNYCents(item.PriceCents))
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func formatCNYCents(cents int64) string {
	if cents%100 == 0 {
		return strconv.FormatInt(cents/100, 10) + "元"
	}
	return fmt.Sprintf("%d.%02d元", cents/100, cents%100)
}

func (s *virtualPaymentService) productByCode(ctx context.Context, code string) (virtualPaymentProduct, error) {
	products, err := s.listProducts(ctx)
	if err != nil {
		return virtualPaymentProduct{}, err
	}
	for _, item := range products {
		if strings.EqualFold(item.Code, strings.TrimSpace(code)) && item.Active && item.PriceCents > 0 && item.OfferID != "" && item.WeChatProductID != "" {
			return item, nil
		}
	}
	return virtualPaymentProduct{}, errVirtualProductUnavailable
}

func snapshotForVirtualProduct(product virtualPaymentProduct) virtualOrderSnapshot {
	return snapshotForVirtualProductQuantity(product, 1)
}

func snapshotForVirtualProductQuantity(product virtualPaymentProduct, quantity int64) virtualOrderSnapshot {
	return virtualOrderSnapshot{
		ProductCode: product.Code, ProductName: product.Name, ProductType: product.ProductType, PlanType: product.PlanType,
		AmountCents: product.PriceCents * quantity, MemberLevel: product.MemberLevel, AgentLevel: product.AgentLevel, MemberDays: product.MemberDays,
		CreditUnits: product.CreditUnits * quantity, ImageQuota: product.ImageQuota * quantity,
		BuyQuantity: quantity, UnitPriceCents: product.PriceCents, UnitCreditUnits: product.CreditUnits, CustomQuantity: product.CustomQuantity,
		OfferID:         product.OfferID,
		WeChatProductID: product.WeChatProductID, Mode: product.Mode, Env: product.Env,
	}
}

func virtualPurchaseQuantity(product virtualPaymentProduct, requested int64) (int64, error) {
	if requested == 0 {
		requested = 1
	}
	if !product.CustomQuantity {
		if requested != 1 {
			return 0, fmt.Errorf("%w: fixed products only support quantity 1", errVirtualQuantityInvalid)
		}
		return 1, nil
	}
	minimum := product.MinQuantity
	maximum := product.MaxQuantity
	if minimum <= 0 {
		minimum = 1
	}
	if maximum < minimum {
		maximum = minimum
	}
	if requested < minimum || requested > maximum {
		return 0, fmt.Errorf("%w: quantity must be between %d and %d", errVirtualQuantityInvalid, minimum, maximum)
	}
	if product.PriceCents <= 0 || product.CreditUnits <= 0 || requested > math.MaxInt64/product.PriceCents || requested > math.MaxInt64/product.CreditUnits {
		return 0, fmt.Errorf("%w: calculated amount overflow", errVirtualQuantityInvalid)
	}
	return requested, nil
}

func (s *virtualPaymentService) resolveTenant(ctx context.Context, user adminUser, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "tenant_default" && strings.TrimSpace(user.TenantID) == "" {
		requested = ""
	}
	if requested == "" {
		requested = strings.TrimSpace(user.TenantID)
	}
	if requested == "" {
		return "personal:" + user.ID, nil
	}
	if requested == strings.TrimSpace(user.TenantID) {
		return requested, nil
	}
	var count int
	err := s.db.QueryRowContext(ctx, `
		select count(*) from xz_tenant_members
		where tenant_id = $1 and user_id = $2
		  and upper(coalesce(nullif(member_status, ''), status, 'ACTIVE')) = 'ACTIVE'
	`, requested, user.ID).Scan(&count)
	if err != nil {
		return "", err
	}
	if count == 0 {
		return "", errForbidden
	}
	return requested, nil
}

func (s *virtualPaymentService) createOrder(ctx context.Context, user adminUser, requestedTenant string, productCode string, requestedQuantity ...int64) (createVirtualOrderResponse, error) {
	quantity := int64(0)
	if len(requestedQuantity) > 0 {
		quantity = requestedQuantity[0]
	}
	return s.createOrderWithCoupon(ctx, user, requestedTenant, productCode, quantity, "")
}

func (s *virtualPaymentService) createOrderWithCoupon(ctx context.Context, user adminUser, requestedTenant string, productCode string, requestedQuantity int64, couponCode string) (createVirtualOrderResponse, error) {
	return s.createOrderWithCouponAndSession(ctx, user, requestedTenant, productCode, requestedQuantity, couponCode, nil)
}

func (s *virtualPaymentService) createOrderWithCouponAndSession(ctx context.Context, user adminUser, requestedTenant string, productCode string, requestedQuantity int64, couponCode string, paymentSession *wechatMiniProgramSession) (createVirtualOrderResponse, error) {
	if !s.cfg.ready() {
		return createVirtualOrderResponse{}, errVirtualPaymentUnavailable
	}
	product, err := s.productByCode(ctx, productCode)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	if strings.EqualFold(product.ProductType, "TOKEN_ONLY") {
		eligible, eligibilityErr := s.pointRechargeEligible(ctx, user.ID)
		if eligibilityErr != nil {
			return createVirtualOrderResponse{}, eligibilityErr
		}
		if !eligible {
			return createVirtualOrderResponse{}, errVirtualRechargeRestricted
		}
	}
	quantity, err := virtualPurchaseQuantity(product, requestedQuantity)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	snapshot := snapshotForVirtualProductQuantity(product, quantity)
	coupon, err := s.availableCouponByCode(ctx, user.ID, product, couponCode)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	if coupon != nil {
		snapshot.CouponCode = coupon.Code
		snapshot.CouponBenefitType = coupon.BenefitType
		snapshot.CouponBenefitValue = coupon.BenefitValue
	}
	tenantID, err := s.resolveTenant(ctx, user, requestedTenant)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	wechatSession := wechatMiniProgramSession{}
	if paymentSession != nil {
		wechatSession = *paymentSession
	} else {
		var ok bool
		wechatSession, ok, err = s.sessions.WeChatSession(ctx, user.ID)
		if err != nil {
			return createVirtualOrderResponse{}, err
		}
		if !ok {
			return createVirtualOrderResponse{}, errVirtualPaymentRelogin
		}
	}
	if strings.TrimSpace(wechatSession.OpenID) == "" || strings.TrimSpace(wechatSession.SessionKey) == "" {
		return createVirtualOrderResponse{}, errVirtualPaymentRelogin
	}

	orderNo := newVirtualBusinessNo("ZQY")
	paymentNo := newVirtualBusinessNo("PAY")
	signData := virtualPaySignData{
		OfferID: product.OfferID, BuyQuantity: quantity, Env: product.Env, CurrencyType: "CNY",
		ProductID: product.WeChatProductID, GoodsPrice: product.PriceCents, OutTradeNo: orderNo, Attach: orderNo,
	}
	signDataJSON, err := json.Marshal(signData)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	paySig := calcVirtualPaySig(requestVirtualPayURI, signDataJSON, s.cfg.AppKey)
	signature := calcVirtualSignature(signDataJSON, wechatSession.SessionKey)
	now := time.Now().UTC()
	expiresAt := now.Add(30 * time.Minute)
	responseAudit, _ := json.Marshal(map[string]any{"mode": product.Mode, "signDataHash": hashSensitiveIdentifier(string(signDataJSON))})

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	defer func() { _ = tx.Rollback() }()
	plan, ok := planCatalogByID(product.PlanID)
	if !ok {
		return createVirtualOrderResponse{}, fmt.Errorf("virtual commerce plan not found: %s", product.PlanID)
	}
	commerceOrder := adminOrder{
		ID: orderNo, OrderNo: orderNo, TenantID: tenantID, UserID: user.ID, BuyerUserID: user.ID,
		PlanID: product.PlanID, Amount: int(snapshot.AmountCents), AmountCents: int(snapshot.AmountCents),
		CreatedAt: now.Format(time.RFC3339Nano), PriceSnapshot: map[string]any{},
	}
	commerceCtx, err := commerceContextForOrderTx(ctx, tx, commerceOrder, plan)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	rules, err := loadEffectiveCommissionRulesTx(ctx, tx, tenantID, planBusinessType(plan), plan.ID, commissionTemplateCode(plan), now)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	snapshot.CommissionTemplateCode = commissionTemplateCode(plan)
	snapshot.CommissionSnapshotCaptured = true
	snapshot.DirectAgentID = commerceCtx.DirectAgentID
	snapshot.ParentAgentID = commerceCtx.ParentAgentID
	snapshot.OperationCenterID = commerceCtx.OperationCenterID
	snapshot.CommissionRules = snapshotCommissionRules(rules)
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	rewardSnapshot := map[string]any{
		"commissionTemplateCode":     snapshot.CommissionTemplateCode,
		"commissionSnapshotCaptured": true,
		"referral": map[string]any{
			"directAgentId":     snapshot.DirectAgentID,
			"parentAgentId":     snapshot.ParentAgentID,
			"operationCenterId": snapshot.OperationCenterID,
		},
		"commissionRules": snapshot.CommissionRules,
	}
	rewardJSON, err := json.Marshal(rewardSnapshot)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	rawOrder, _ := json.Marshal(map[string]any{
		"id": orderNo, "orderNo": orderNo, "tenantId": tenantID, "userId": user.ID, "buyerUserId": user.ID,
		"planId": product.PlanID, "amountCents": snapshot.AmountCents, "status": virtualOrderPending,
		"paymentMethod": virtualPaymentChannel, "fulfillmentStatus": entitlementPending,
		"priceSnapshot": snapshot, "rewardSnapshot": rewardSnapshot, "createdAt": now.Format(time.RFC3339Nano),
	})
	_, err = tx.ExecContext(ctx, `
		insert into xz_orders(
			id, order_no, tenant_id, user_id, buyer_user_id, plan_id, order_type, business_order_type,
			amount_cents, status, fulfillment_status, entitlement_status, product_code, product_name,
			product_type, payment_channel, payment_scene, payment_mode, wechat_openid_hash,
			payment_expires_at, created_at, updated_at, price_snapshot, reward_snapshot, raw
		) values (
			$1,$1,$2,$3,$3,$4,'VIRTUAL_PRODUCT','VIRTUAL_PRODUCT',$5,$6,$7,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18::jsonb,$19::jsonb,$20::jsonb
		)
	`, orderNo, tenantID, user.ID, product.PlanID, snapshot.AmountCents, virtualOrderPending, entitlementPending,
		product.Code, product.Name, product.ProductType, virtualPaymentChannel, virtualPaymentScene, product.Mode,
		hashSensitiveIdentifier(wechatSession.OpenID), expiresAt, now.Format(time.RFC3339Nano), now, snapshotJSON, rewardJSON, rawOrder)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	_, err = tx.ExecContext(ctx, `
		insert into xz_payment_records(
			id, payment_no, order_id, order_no, tenant_id, user_id, payment_channel, payment_scene,
			amount_cents, prepay_status, request_payload, response_payload
		) values ($1,$1,$2,$2,$3,$4,$5,$6,$7,'SIGNED',$8::jsonb,$9::jsonb)
	`, paymentNo, orderNo, tenantID, user.ID, virtualPaymentChannel, virtualPaymentScene, snapshot.AmountCents, signDataJSON, responseAudit)
	if err != nil {
		return createVirtualOrderResponse{}, err
	}
	if coupon != nil {
		if err := reserveVirtualCouponTx(ctx, tx, orderNo, tenantID, user.ID, product.Code, *coupon); err != nil {
			return createVirtualOrderResponse{}, err
		}
	}
	if err := insertCommercialBillingOrderTx(ctx, tx, orderNo, tenantID, user.ID, snapshot.AmountCents, now, expiresAt); err != nil {
		return createVirtualOrderResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return createVirtualOrderResponse{}, err
	}
	return createVirtualOrderResponse{
		OrderNo: orderNo, AmountCent: snapshot.AmountCents, SignData: string(signDataJSON),
		PaySig: paySig, Signature: signature, Mode: product.Mode,
	}, nil
}

func (s *virtualPaymentService) pointRechargeEligible(ctx context.Context, userID string) (bool, error) {
	var memberLevel string
	var agentStatus string
	var expiresAt string
	if err := s.db.QueryRowContext(ctx, `
		select coalesce(member_level, raw->>'memberLevel', ''),
		       coalesce(agent_status, raw->>'agentStatus', ''),
		       coalesce(subscription_expires_at, '')
		from xz_users where id = $1
	`, userID).Scan(&memberLevel, &agentStatus, &expiresAt); err != nil {
		return false, err
	}
	return pointRechargeIdentityEligible(memberLevel, agentStatus, expiresAt, time.Now().UTC()), nil
}

func pointRechargeIdentityEligible(memberLevel, agentStatus, expiresAt string, now time.Time) bool {
	if strings.EqualFold(strings.TrimSpace(agentStatus), "ACTIVE") {
		return true
	}
	memberLevel = strings.TrimSpace(memberLevel)
	if memberLevel == "" || strings.EqualFold(memberLevel, "FREE") {
		return false
	}
	expiresAt = strings.TrimSpace(expiresAt)
	if expiresAt == "" {
		return true
	}
	expires, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return false
	}
	return expires.After(now)
}

func newVirtualBusinessNo(prefix string) string {
	seed := newRequestID() + ":" + strconv.FormatInt(time.Now().UnixNano(), 36)
	suffix := strings.ToUpper(hashSensitiveIdentifier(seed)[:10])
	return strings.ToUpper(prefix) + time.Now().UTC().Format("20060102150405") + suffix
}

func (s *virtualPaymentService) ownedOrder(ctx context.Context, user adminUser, requestedTenant string, orderNo string) (virtualOrderView, error) {
	tenantID, err := s.resolveTenant(ctx, user, requestedTenant)
	if err != nil {
		return virtualOrderView{}, err
	}
	item, storedTenant, storedUser, err := s.orderView(ctx, orderNo)
	if errors.Is(err, sql.ErrNoRows) {
		return virtualOrderView{}, errVirtualOrderNotFound
	}
	if err != nil {
		return virtualOrderView{}, err
	}
	if storedUser != user.ID || storedTenant != tenantID {
		return virtualOrderView{}, errVirtualOrderNotFound
	}
	return item, nil
}

func (s *virtualPaymentService) orderView(ctx context.Context, orderNo string) (virtualOrderView, string, string, error) {
	var item virtualOrderView
	var tenantID string
	var userID string
	var snapshotJSON []byte
	var paidAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		select order_no, tenant_id, user_id, amount_cents, status, status, entitlement_status,
		       entitlement_error, coalesce(wechat_order_id, ''), coalesce(wechat_transaction_id, ''),
		       case when coalesce(paid_at, '') ~ '^2[0-9]{3}-' then paid_at::timestamptz else null end,
		       coalesce(created_at, ''), updated_at, price_snapshot
		from xz_orders where order_no = $1 and payment_channel = $2
	`, strings.TrimSpace(orderNo), virtualPaymentChannel).Scan(
		&item.OrderNo, &tenantID, &userID, &item.AmountCents, &item.OrderStatus, &item.PaymentStatus,
		&item.EntitlementStatus, &item.EntitlementError, &item.WeChatOrderID, &item.WeChatTradeNo,
		&paidAt, &item.CreatedAt, &item.UpdatedAt, &snapshotJSON,
	)
	if err != nil {
		return virtualOrderView{}, "", "", err
	}
	if paidAt.Valid {
		item.PaidAt = &paidAt.Time
	}
	if err := json.Unmarshal(snapshotJSON, &item.Product); err != nil {
		return virtualOrderView{}, "", "", err
	}
	return item, tenantID, userID, nil
}

type virtualPayNotification struct {
	ID            string
	Event         string
	OrderNo       string
	OpenID        string
	WeChatOrderID string
	TransactionID string
	AmountCents   int64
	ProductID     string
	Quantity      int64
	Env           int
	HasEnv        bool
	ResultCode    int64
	ResultMessage string
	PaidAt        time.Time
	Raw           []byte
	Payload       map[string]any
}

type virtualNotifyXML struct {
	XMLName       xml.Name `xml:"xml"`
	FromUserName  string   `xml:"FromUserName"`
	OpenID        string   `xml:"OpenId"`
	CreateTime    int64    `xml:"CreateTime"`
	Env           int      `xml:"Env"`
	Event         string   `xml:"Event"`
	OutTradeNo    string   `xml:"OutTradeNo"`
	MchOrderID    string   `xml:"MchOrderId"`
	WxOrderID     string   `xml:"WxOrderId"`
	WxRefundID    string   `xml:"WxRefundId"`
	PaidFee       int64    `xml:"PaidFee"`
	OrderFee      int64    `xml:"OrderFee"`
	RefundFee     int64    `xml:"RefundFee"`
	RetCode       int64    `xml:"RetCode"`
	RetMsg        string   `xml:"RetMsg"`
	PayTime       int64    `xml:"PayTime"`
	MsgID         string   `xml:"MsgId"`
	WeChatPayInfo struct {
		MchOrderNo    string `xml:"MchOrderNo"`
		TransactionID string `xml:"TransactionId"`
		PayTradeNo    string `xml:"PayTradeNo"`
		PaidTime      int64  `xml:"PaidTime"`
	} `xml:"WeChatPayInfo"`
	GoodsInfo struct {
		ProductID   string `xml:"ProductId"`
		Quantity    int64  `xml:"Quantity"`
		OrigPrice   int64  `xml:"OrigPrice"`
		ActualPrice int64  `xml:"ActualPrice"`
		Attach      string `xml:"Attach"`
	} `xml:"GoodsInfo"`
}

func parseVirtualPayNotification(body []byte) (virtualPayNotification, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return virtualPayNotification{}, errors.New("empty wechat notification")
	}
	payload := map[string]any{}
	hasEnv := false
	if body[0] == '<' {
		var item virtualNotifyXML
		if err := xml.Unmarshal(body, &item); err != nil {
			return virtualPayNotification{}, err
		}
		hasEnv = bytes.Contains(body, []byte("<Env>"))
		payload = map[string]any{
			"Event": item.Event, "OutTradeNo": item.OutTradeNo, "MchOrderId": item.MchOrderID,
			"WxOrderId": item.WxOrderID, "WxRefundId": item.WxRefundID,
			"FromUserName": item.FromUserName, "OpenId": item.OpenID,
			"CreateTime": item.CreateTime, "Env": item.Env, "PaidFee": item.PaidFee, "OrderFee": item.OrderFee,
			"RefundFee": item.RefundFee, "RetCode": item.RetCode, "RetMsg": item.RetMsg,
			"PayTime": item.PayTime, "MsgId": item.MsgID,
			"WeChatPayInfo": map[string]any{
				"MchOrderNo": item.WeChatPayInfo.MchOrderNo, "TransactionId": item.WeChatPayInfo.TransactionID,
				"PayTradeNo": item.WeChatPayInfo.PayTradeNo, "PaidTime": item.WeChatPayInfo.PaidTime,
			},
			"GoodsInfo": map[string]any{
				"ProductId": item.GoodsInfo.ProductID, "Quantity": item.GoodsInfo.Quantity,
				"OrigPrice": item.GoodsInfo.OrigPrice, "ActualPrice": item.GoodsInfo.ActualPrice,
				"Attach": item.GoodsInfo.Attach,
			},
		}
	} else {
		if err := json.Unmarshal(body, &payload); err != nil {
			return virtualPayNotification{}, err
		}
		hasEnv = mapHasKey(payload, "Env", "env")
	}
	event := mapString(payload, "Event", "event")
	orderNo := mapString(payload, "OutTradeNo", "out_trade_no", "order_id", "MchOrderId", "mch_order_id")
	if event == "" || orderNo == "" {
		return virtualPayNotification{}, errors.New("wechat notification missing event or order number")
	}
	wechatPay := mapValueMap(payload, "WeChatPayInfo", "wechat_pay_info", "order")
	goodsInfo := mapValueMap(payload, "GoodsInfo", "goods_info")
	transactionID := firstNonEmptyString(
		mapString(wechatPay, "PayTradeNo", "TransactionId", "transaction_id", "wxpay_order_id"),
		mapString(payload, "PayTradeNo", "TransactionId", "transaction_id", "wxpay_order_id", "WxpayRefundTransactionId"),
	)
	wechatOrderID := firstNonEmptyString(
		mapString(wechatPay, "MchOrderNo", "WxOrderId", "wx_order_id", "channel_order_id"),
		mapString(payload, "MchOrderNo", "WxOrderId", "wx_order_id", "channel_order_id", "WxRefundId"),
	)
	amount := firstPositiveInt64(
		mapInt64(goodsInfo, "ActualPrice", "actual_price"),
		mapInt64(payload, "RefundFee", "refund_fee"),
		mapInt64(payload, "PaidFee", "paid_fee", "OrderFee", "order_fee", "GoodsPrice", "goods_price"),
		mapInt64(wechatPay, "PaidFee", "paid_fee", "OrderFee", "order_fee"),
	)
	paidAt := parseWechatNotificationTime(firstNonEmptyString(mapString(wechatPay, "PaidTime", "PayTime", "paid_time"), mapString(payload, "PayTime", "paid_time", "RefundSuccTimestamp")))
	eventID := mapString(payload, "MsgId", "msgid", "event_id")
	if eventID == "" {
		digest := sha256.Sum256(append([]byte(strings.ToLower(event)+"|"+orderNo+"|"+transactionID+"|"), body...))
		eventID = "wxevt_" + hex.EncodeToString(digest[:16])
	}
	return virtualPayNotification{
		ID: eventID, Event: event, OrderNo: orderNo, OpenID: mapString(payload, "OpenId", "openid", "FromUserName"),
		WeChatOrderID: wechatOrderID, TransactionID: transactionID, AmountCents: amount,
		ProductID: mapString(goodsInfo, "ProductId", "product_id"), Quantity: mapInt64(goodsInfo, "Quantity", "quantity"),
		Env: int(mapInt64(payload, "Env", "env")), HasEnv: hasEnv,
		ResultCode: mapInt64(payload, "RetCode", "ret_code"), ResultMessage: mapString(payload, "RetMsg", "ret_msg"),
		PaidAt: paidAt, Raw: append([]byte(nil), body...), Payload: payload,
	}, nil
}

func mapHasKey(payload map[string]any, keys ...string) bool {
	for key := range payload {
		for _, expected := range keys {
			if strings.EqualFold(strings.TrimSpace(key), strings.TrimSpace(expected)) {
				return true
			}
		}
	}
	return false
}

func mapValueMap(payload map[string]any, keys ...string) map[string]any {
	for key, value := range payload {
		for _, expected := range keys {
			if strings.EqualFold(strings.TrimSpace(key), strings.TrimSpace(expected)) {
				if result, ok := value.(map[string]any); ok {
					return result
				}
			}
		}
	}
	return map[string]any{}
}

func mapString(payload map[string]any, keys ...string) string {
	for _, expected := range keys {
		for key, value := range payload {
			if strings.EqualFold(strings.TrimSpace(key), strings.TrimSpace(expected)) {
				switch item := value.(type) {
				case string:
					if item = strings.TrimSpace(item); item != "" {
						return item
					}
				case json.Number:
					return item.String()
				case float64:
					return strconv.FormatInt(int64(item), 10)
				case int64:
					return strconv.FormatInt(item, 10)
				case int:
					return strconv.Itoa(item)
				}
			}
		}
	}
	return ""
}

func mapInt64(payload map[string]any, keys ...string) int64 {
	value := mapString(payload, keys...)
	parsed, _ := strconv.ParseInt(value, 10, 64)
	return parsed
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func parseWechatNotificationTime(value string) time.Time {
	if unixSeconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && unixSeconds > 0 {
		return time.Unix(unixSeconds, 0).UTC()
	}
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed.UTC()
		}
	}
	return time.Now().UTC()
}

func (s *virtualPaymentService) processNotification(ctx context.Context, notification virtualPayNotification) error {
	switch strings.ToLower(strings.TrimSpace(notification.Event)) {
	case virtualGoodsNotify:
		return s.confirmPaidAndGrant(ctx, notification)
	case virtualRefundNotify:
		return s.recordRefundNotification(ctx, notification)
	default:
		return s.recordIgnoredNotification(ctx, notification)
	}
}

type wechatQueryOrderRequest struct {
	OpenID    string `json:"openid"`
	Env       int    `json:"env"`
	OrderID   string `json:"order_id,omitempty"`
	WxOrderID string `json:"wx_order_id,omitempty"`
}

type wechatQueryOrderResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
	Order   struct {
		OrderID        string `json:"order_id"`
		Status         int    `json:"status"`
		OrderFee       int64  `json:"order_fee"`
		PaidFee        int64  `json:"paid_fee"`
		PaidTime       int64  `json:"paid_time"`
		WxOrderID      string `json:"wx_order_id"`
		ChannelOrderID string `json:"channel_order_id"`
		WxPayOrderID   string `json:"wxpay_order_id"`
		EnvType        int    `json:"env_type"`
	} `json:"order"`
}

func (s *virtualPaymentService) syncOwnedOrder(ctx context.Context, user adminUser, requestedTenant string, orderNo string) (virtualOrderView, error) {
	item, err := s.ownedOrder(ctx, user, requestedTenant, orderNo)
	if err != nil {
		return virtualOrderView{}, err
	}
	if item.OrderStatus == virtualOrderPaid && item.EntitlementStatus == entitlementSuccess {
		return item, nil
	}
	if err := s.syncOrder(ctx, user.ID, item.OrderNo); err != nil {
		return virtualOrderView{}, err
	}
	item, err = s.ownedOrder(ctx, user, requestedTenant, orderNo)
	return item, err
}

func (s *virtualPaymentService) syncOrder(ctx context.Context, userID string, orderNo string) error {
	wechatSession, ok, err := s.sessions.WeChatSession(ctx, userID)
	if err != nil {
		return err
	}
	if !ok || wechatSession.OpenID == "" {
		return errVirtualPaymentRelogin
	}
	request := wechatQueryOrderRequest{OpenID: wechatSession.OpenID, Env: s.cfg.Env, OrderID: orderNo}
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	accessToken, err := s.wechatAccessToken(ctx)
	if err != nil {
		return err
	}
	paySig := calcVirtualPaySig(queryVirtualOrderURI, body, s.cfg.AppKey)
	endpoint := "https://api.weixin.qq.com/xpay/query_order?access_token=" + url.QueryEscape(accessToken) + "&pay_sig=" + url.QueryEscape(paySig)
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
		return fmt.Errorf("wechat query order status %d", response.StatusCode)
	}
	var result wechatQueryOrderResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return err
	}
	if result.ErrCode != 0 {
		if result.ErrCode == 268490009 {
			return errVirtualPaymentRelogin
		}
		return fmt.Errorf("wechat query order failed: code=%d message=%s", result.ErrCode, strings.TrimSpace(result.ErrMsg))
	}
	if err := s.recordQueryResponse(ctx, orderNo, result, responseBody); err != nil {
		return err
	}
	if err := validateWechatQueryOrderResponse(orderNo, s.cfg.Env, result); err != nil {
		return err
	}
	switch result.Order.Status {
	case 2, 3, 4:
		amount := firstPositiveInt64(result.Order.PaidFee, result.Order.OrderFee)
		return s.confirmPaidAndGrant(ctx, virtualPayNotification{
			ID: "query:" + orderNo + ":paid", Event: "query_order_paid", OrderNo: orderNo,
			OpenID: wechatSession.OpenID, WeChatOrderID: firstNonEmptyString(result.Order.WxOrderID, result.Order.ChannelOrderID),
			TransactionID: result.Order.WxPayOrderID, AmountCents: amount,
			Env: s.cfg.Env, HasEnv: true,
			PaidAt: time.Unix(result.Order.PaidTime, 0).UTC(), Raw: responseBody,
		})
	case 5, 8:
		return s.recordRefundNotification(ctx, virtualPayNotification{
			ID: "query:" + orderNo + ":refund", Event: "query_order_refunded", OrderNo: orderNo,
			OpenID: wechatSession.OpenID, WeChatOrderID: result.Order.WxOrderID,
			TransactionID: result.Order.WxPayOrderID, AmountCents: result.Order.PaidFee,
			PaidAt: time.Now().UTC(), Raw: responseBody,
		})
	case 7:
		return s.recordRefundNotification(ctx, virtualPayNotification{
			ID: "query:" + orderNo + ":refund-failed", Event: "query_order_refund_failed", OrderNo: orderNo,
			OpenID: wechatSession.OpenID, WeChatOrderID: result.Order.WxOrderID,
			TransactionID: result.Order.WxPayOrderID, AmountCents: result.Order.PaidFee,
			ResultCode: 7, ResultMessage: "wechat refund failed", PaidAt: time.Now().UTC(), Raw: responseBody,
		})
	case 6:
		_, err = s.db.ExecContext(ctx, `
			update xz_orders set status = $2, updated_at = now()
			where order_no = $1 and status = $3
		`, orderNo, virtualOrderClosed, virtualOrderPending)
		if err == nil {
			markCommercialOrderClosed(ctx, s.db, orderNo)
		}
		return err
	default:
		return nil
	}
}

func validateWechatQueryOrderResponse(orderNo string, env int, result wechatQueryOrderResponse) error {
	if strings.TrimSpace(result.Order.OrderID) == "" || !strings.EqualFold(strings.TrimSpace(result.Order.OrderID), strings.TrimSpace(orderNo)) {
		return fmt.Errorf("%w: wechat query returned another order", errVirtualPaymentMismatch)
	}
	if result.Order.EnvType > 0 && result.Order.EnvType != env+1 {
		return fmt.Errorf("%w: wechat query environment mismatch", errVirtualPaymentMismatch)
	}
	return nil
}

func (s *virtualPaymentService) wechatAccessToken(ctx context.Context) (string, error) {
	cacheKey := "wechat:access-token:" + hashSensitiveIdentifier(s.cfg.AppID)[:16]
	if s.redis != nil {
		if token, err := s.redis.Get(ctx, cacheKey).Result(); err == nil && token != "" {
			return token, nil
		} else if err != nil && !errors.Is(err, redis.Nil) {
			return "", err
		}
	}
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	if s.accessToken != "" && time.Now().Before(s.accessTokenExp) {
		return s.accessToken, nil
	}
	endpoint := "https://api.weixin.qq.com/cgi-bin/stable_token"
	body, _ := json.Marshal(map[string]any{"grant_type": "client_credential", "appid": s.cfg.AppID, "secret": s.cfg.AppSecret, "force_refresh": false})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return "", err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices || result.ErrCode != 0 || result.AccessToken == "" {
		return "", fmt.Errorf("wechat access token unavailable: code=%d", result.ErrCode)
	}
	ttl := time.Duration(result.ExpiresIn-300) * time.Second
	if ttl < time.Minute {
		ttl = time.Hour
	}
	s.accessToken = result.AccessToken
	s.accessTokenExp = time.Now().Add(ttl)
	if s.redis != nil {
		if err := s.redis.Set(ctx, cacheKey, result.AccessToken, ttl).Err(); err != nil {
			return "", err
		}
	}
	return result.AccessToken, nil
}

func (s *virtualPaymentService) recordQueryResponse(ctx context.Context, orderNo string, response wechatQueryOrderResponse, raw []byte) error {
	redacted := map[string]any{"errcode": response.ErrCode, "errmsg": response.ErrMsg, "order": response.Order}
	payload, _ := json.Marshal(redacted)
	_, err := s.db.ExecContext(ctx, `
		update xz_payment_records
		set response_payload = $2::jsonb, wechat_order_id = nullif($3, ''),
		    wechat_transaction_id = nullif($4, ''), updated_at = now()
		where order_no = $1
	`, orderNo, payload, firstNonEmptyString(response.Order.WxOrderID, response.Order.ChannelOrderID), response.Order.WxPayOrderID)
	_ = raw
	return err
}

func (s *virtualPaymentService) runCompensationLoop() {
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		<-timer.C
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		_ = s.compensateOrders(ctx)
		cancel()
		timer.Reset(2 * time.Minute)
	}
}

func (s *virtualPaymentService) compensateOrders(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		select order_no, user_id
		from xz_orders
		where payment_channel = $1
		  and ((status = $2 and created_at::timestamptz < now() - interval '2 minutes')
		       or (status = $3 and entitlement_status in ($4, $5)))
		  and (compensation_locked_until is null or compensation_locked_until < now())
		order by created_at
		limit 50
	`, virtualPaymentChannel, virtualOrderPending, virtualOrderPaid, entitlementPending, entitlementFailed)
	if err != nil {
		return err
	}
	type candidate struct{ orderNo, userID string }
	candidates := []candidate{}
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.orderNo, &item.userID); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range candidates {
		var claimed string
		err := s.db.QueryRowContext(ctx, `
			update xz_orders set compensation_locked_until = now() + interval '90 seconds', updated_at = now()
			where order_no = $1 and (compensation_locked_until is null or compensation_locked_until < now())
			returning order_no
		`, item.orderNo).Scan(&claimed)
		if err != nil {
			continue
		}
		view, _, _, viewErr := s.orderView(ctx, item.orderNo)
		if viewErr == nil && view.OrderStatus == virtualOrderPaid {
			_ = s.GrantOrderEntitlements(ctx, item.orderNo)
		} else {
			_ = s.syncOrder(ctx, item.userID, item.orderNo)
			_, _ = s.db.ExecContext(ctx, `
				update xz_orders set status = $2, updated_at = now()
				where order_no = $1 and status = $3 and payment_expires_at < now()
			`, item.orderNo, virtualOrderClosed, virtualOrderPending)
			markCommercialOrderClosed(ctx, s.db, item.orderNo)
		}
		_, _ = s.db.ExecContext(ctx, `update xz_orders set compensation_locked_until = null where order_no = $1`, item.orderNo)
	}
	return nil
}
