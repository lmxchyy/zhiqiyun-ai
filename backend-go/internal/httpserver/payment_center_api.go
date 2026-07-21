package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	paymentapp "xianzhi-ai/backend-go/internal/app/payment"
	"xianzhi-ai/backend-go/internal/config"
)

type paymentCenterAPI struct {
	service  *paymentapp.Service
	success  paymentapp.PaymentSuccessHandler
	cfg      config.Config
	mock     *paymentapp.MockPaymentProvider
	db       *sql.DB
	store    platformStore
	sessions authSessionStore
	legacy   virtualPaymentAPI
}

func newPaymentCenterAPI(cfg config.Config, store platformStore, sessions authSessionStore, legacy virtualPaymentAPI) paymentCenterAPI {
	result := paymentCenterAPI{cfg: cfg, store: store, sessions: sessions, legacy: legacy}
	pgStore, ok := store.(*postgresStore)
	if !ok || pgStore.db == nil {
		return result
	}
	mock := paymentapp.NewMockPaymentProvider(cfg.Environment)
	result.db = pgStore.db
	result.mock = mock
	result.service = paymentapp.NewService(pgStore.db, []paymentapp.PaymentProvider{mock}, unifiedPaymentCommissionHook)
	result.success = paymentapp.NewPaymentSuccessHandler(result.service)
	return result
}

func unifiedPaymentCommissionHook(ctx context.Context, tx *sql.Tx, order paymentapp.Order) error {
	switch strings.ToLower(strings.TrimSpace(order.ProductType)) {
	case "membership", "agent_join", "operation_center":
	default:
		return nil
	}
	var plan adminPlan
	if err := tx.QueryRowContext(ctx, `SELECT raw FROM xz_plans WHERE id=$1`, order.SourcePlanID).Scan(rawScanner(&plan)); err != nil {
		return err
	}
	adminOrder := adminOrder{
		ID: order.ID, OrderNo: order.OrderNo, TenantID: order.TenantID, UserID: order.UserID,
		BuyerUserID: order.UserID, PlanID: order.SourcePlanID, AmountCents: int(order.PayableAmount),
		Status: string(paymentapp.OrderPaid), PaidAt: time.Now().UTC().Format(time.RFC3339Nano),
		PriceSnapshot: map[string]any{"quantity": order.Quantity, "productType": order.ProductType},
	}
	commerceCtx, err := commerceContextForOrderTx(ctx, tx, adminOrder, plan)
	if err != nil {
		return err
	}
	rules, err := loadEffectiveCommissionRulesTx(ctx, tx, firstNonEmptyString(order.TenantID, "tenant_default"),
		planBusinessType(plan), plan.ID, commissionTemplateCode(plan), time.Now().UTC())
	if err != nil || len(rules) == 0 {
		return err
	}
	_, err = generateCommissionRecordsForCommerceOrderTx(ctx, tx, adminOrder, plan, commerceCtx)
	return err
}

func (a paymentCenterAPI) available() bool {
	return a.service != nil && a.service.Ready()
}

type paymentCapabilityView struct {
	Platform          string `json:"platform"`
	PaymentCapability string `json:"paymentCapability"`
	PaymentStatus     string `json:"paymentStatus"`
	PaymentChannel    string `json:"paymentChannel,omitempty"`
	Message           string `json:"message"`
	Enabled           bool   `json:"enabled"`
}

func normalizedPaymentPlatform(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "android", "app", "app-plus":
		return "app-android"
	case "ios":
		return "app-ios"
	case "weixin", "wechat", "mini-program":
		return "mp-weixin"
	default:
		return value
	}
}

func (a paymentCenterAPI) paymentCapabilityFor(platform string) paymentCapabilityView {
	platform = normalizedPaymentPlatform(platform)
	result := paymentCapabilityView{
		Platform: platform, PaymentCapability: "unavailable", PaymentStatus: "UNAVAILABLE",
		Message: "当前平台暂未开放支付", Enabled: false,
	}
	switch platform {
	case "mp-weixin":
		if a.legacy.service != nil && a.legacy.service.cfg.ready() {
			result.PaymentCapability = "available"
			result.PaymentStatus = "READY"
			result.PaymentChannel = "wechat_virtual"
			result.Message = "微信虚拟支付可用"
			result.Enabled = true
		} else {
			result.PaymentCapability = "preparing"
			result.PaymentStatus = "PREPARING"
			result.PaymentChannel = "wechat_virtual"
			result.Message = "支付准备中"
		}
	case "app-android":
		channel := strings.ToLower(strings.TrimSpace(a.cfg.AndroidPaymentChannel))
		if channel == "" {
			channel = "wechat_app"
		}
		result.PaymentChannel = channel
		desired := strings.ToLower(strings.TrimSpace(a.cfg.AndroidPaymentCapability))
		if desired == "unavailable" {
			return result
		}
		result.PaymentCapability = "preparing"
		result.PaymentStatus = "PREPARING"
		result.Message = "支付准备中"
		if desired == "available" && a.service != nil && a.service.HasProvider(channel) {
			result.PaymentCapability = "available"
			result.PaymentStatus = "READY"
			result.Message = "支付可用"
			result.Enabled = true
		}
	}
	return result
}

func (a paymentCenterAPI) capability(w http.ResponseWriter, r *http.Request) {
	platform := strings.TrimSpace(r.URL.Query().Get("platform"))
	if platform == "" {
		platform = strings.TrimSpace(r.Header.Get("X-Client-Platform"))
	}
	writeJSON(w, a.paymentCapabilityFor(platform))
}

func (a paymentCenterAPI) orderCreationAllowed(platform, channel string) bool {
	platform = normalizedPaymentPlatform(platform)
	channel = strings.ToLower(strings.TrimSpace(channel))
	if channel == "mock" {
		return strings.ToLower(strings.TrimSpace(a.cfg.Environment)) != "production"
	}
	capability := a.paymentCapabilityFor(platform)
	return capability.Enabled && strings.EqualFold(capability.PaymentChannel, channel)
}

func (a paymentCenterAPI) currentUser(r *http.Request) (adminUser, error) {
	return api{store: a.store, sessions: a.sessions}.currentUser(r)
}

func (a paymentCenterAPI) resolveTenant(ctx context.Context, user adminUser) (string, error) {
	if strings.TrimSpace(user.TenantID) == "" {
		return "personal:" + user.ID, nil
	}
	var count int
	err := a.db.QueryRowContext(ctx, `
		SELECT count(*) FROM xz_tenant_members
		WHERE tenant_id=$1 AND user_id=$2
		  AND upper(coalesce(nullif(member_status,''),status,'ACTIVE'))='ACTIVE'
	`, user.TenantID, user.ID).Scan(&count)
	if err != nil {
		return "", err
	}
	if count == 0 {
		return "", errForbidden
	}
	return user.TenantID, nil
}

func (a paymentCenterAPI) createOrder(w http.ResponseWriter, r *http.Request) {
	if !a.available() {
		writePaymentError(w, http.StatusServiceUnavailable, errors.New("payment center is unavailable"))
		return
	}
	user, err := a.currentUser(r)
	if err != nil {
		writePaymentError(w, http.StatusUnauthorized, err)
		return
	}
	tenantID, err := a.resolveTenant(r.Context(), user)
	if err != nil {
		writePaymentError(w, http.StatusForbidden, err)
		return
	}
	var req struct {
		ProductCode    string `json:"productCode"`
		Quantity       int64  `json:"quantity"`
		Platform       string `json:"platform"`
		PaymentChannel string `json:"paymentChannel"`
		Amount         *int64 `json:"amount,omitempty"`
		AmountCents    *int64 `json:"amountCents,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writePaymentError(w, http.StatusBadRequest, err)
		return
	}
	req.Platform = normalizedPaymentPlatform(req.Platform)
	if !a.orderCreationAllowed(req.Platform, req.PaymentChannel) {
		writePaymentErrorWithCode(w, http.StatusConflict, paymentapp.CodeCapabilityUnavailable, "payment capability is not available for current platform")
		return
	}
	result, err := a.service.CreateOrder(r.Context(), paymentapp.CreateOrderInput{
		UserID: user.ID, TenantID: tenantID, ProductCode: req.ProductCode, Quantity: req.Quantity,
		Platform: req.Platform, PaymentChannel: req.PaymentChannel,
		IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")), ClientIP: paymentClientIP(r),
	})
	if err != nil {
		writePaymentDomainError(w, err)
		return
	}
	writeJSON(w, map[string]any{
		"orderNo": result.Order.OrderNo, "paymentNo": result.PaymentNo,
		"productName": result.Order.ProductName, "amount": result.Order.PayableAmount,
		"currency": result.Order.Currency, "platform": result.Order.Platform, "channel": result.Order.Channel,
		"orderStatus": result.Order.OrderStatus, "paymentStatus": result.Order.PaymentStatus,
		"fulfillmentStatus": result.Order.FulfillmentStatus, "paymentParams": result.PaymentParams,
	})
}

func (a paymentCenterAPI) order(w http.ResponseWriter, r *http.Request) {
	if !a.available() {
		if a.legacy.service != nil && a.legacy.service.cfg.ready() {
			a.legacy.order(w, r)
			return
		}
		writePaymentError(w, http.StatusServiceUnavailable, errors.New("payment center is unavailable"))
		return
	}
	user, err := a.currentUser(r)
	if err != nil {
		writePaymentError(w, http.StatusUnauthorized, err)
		return
	}
	order, err := a.service.GetOrder(r.Context(), r.PathValue("orderNo"))
	if err != nil {
		if paymentapp.ErrorCodeOf(err) == paymentapp.CodeOrderNotFound && a.legacy.service != nil {
			a.legacy.order(w, r)
			return
		}
		writePaymentDomainError(w, err)
		return
	}
	if !canQueryPaymentOrder(user, order) {
		writePaymentErrorWithCode(w, http.StatusForbidden, paymentapp.CodeOrderForbidden, "order does not belong to current user")
		return
	}
	writeJSON(w, paymentOrderResponse(order))
}

func isPlatformAdmin(user adminUser) bool {
	role := strings.ToUpper(strings.TrimSpace(user.Role))
	return role == "SUPER_ADMIN" || role == "ADMIN" || role == "FINANCE"
}

func canQueryPaymentOrder(user adminUser, order paymentapp.Order) bool {
	return order.UserID == user.ID || isPlatformAdmin(user)
}

func paymentOrderResponse(order paymentapp.Order) map[string]any {
	return map[string]any{
		"orderNo": order.OrderNo, "productName": order.ProductName, "amount": order.PayableAmount,
		"currency": order.Currency, "platform": order.Platform, "channel": order.Channel,
		"orderStatus": order.OrderStatus, "paymentStatus": order.PaymentStatus,
		"fulfillmentStatus": order.FulfillmentStatus, "createdAt": order.CreatedAt,
		"paidAt": order.PaidAt, "fulfilledAt": order.FulfilledAt,
	}
}

func (a paymentCenterAPI) mockAction(scenario paymentapp.MockScenario, duplicate bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.available() || a.mock == nil {
			writePaymentError(w, http.StatusNotFound, errors.New("mock payment is unavailable"))
			return
		}
		user, err := a.currentUser(r)
		if err != nil {
			writePaymentError(w, http.StatusUnauthorized, err)
			return
		}
		order, err := a.service.GetOrder(r.Context(), r.PathValue("orderNo"))
		if err != nil {
			writePaymentDomainError(w, err)
			return
		}
		if !canQueryPaymentOrder(user, order) {
			writePaymentErrorWithCode(w, http.StatusForbidden, paymentapp.CodeOrderForbidden, "order does not belong to current user")
			return
		}
		if !strings.EqualFold(order.Channel, "mock") {
			writePaymentErrorWithCode(w, http.StatusBadRequest, paymentapp.CodeInvalidRequest, "order is not a mock payment order")
			return
		}
		notification, err := a.mock.Notification(order, scenario, "")
		if err == nil {
			if notification.Status == paymentapp.PaymentSuccess {
				err = a.success.Handle(r.Context(), notification)
			} else {
				err = a.service.HandlePaymentNotification(r.Context(), notification)
			}
		}
		if err == nil && duplicate {
			err = a.success.Handle(r.Context(), notification)
		}
		if err != nil {
			writePaymentDomainError(w, err)
			return
		}
		updated, err := a.service.GetOrder(r.Context(), order.OrderNo)
		if err != nil {
			writePaymentDomainError(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "duplicate": duplicate, "order": paymentOrderResponse(updated)})
	}
}

func (a paymentCenterAPI) mockDelayedSuccess(w http.ResponseWriter, r *http.Request) {
	if !a.available() || a.mock == nil {
		writePaymentError(w, http.StatusNotFound, errors.New("mock payment is unavailable"))
		return
	}
	user, err := a.currentUser(r)
	if err != nil {
		writePaymentError(w, http.StatusUnauthorized, err)
		return
	}
	order, err := a.service.GetOrder(r.Context(), r.PathValue("orderNo"))
	if err != nil {
		writePaymentDomainError(w, err)
		return
	}
	if !canQueryPaymentOrder(user, order) {
		writePaymentErrorWithCode(w, http.StatusForbidden, paymentapp.CodeOrderForbidden, "order does not belong to current user")
		return
	}
	delayMS, _ := strconv.Atoi(r.URL.Query().Get("delayMs"))
	if delayMS <= 0 || delayMS > 30000 {
		delayMS = 500
	}
	notification, err := a.mock.DelayedNotification(r.Context(), order, time.Duration(delayMS)*time.Millisecond)
	if err == nil {
		err = a.success.Handle(r.Context(), notification)
	}
	if err != nil {
		writePaymentDomainError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "delayMs": delayMS})
}

func (a paymentCenterAPI) adminOrders(w http.ResponseWriter, r *http.Request) {
	a.adminList(w, r, "orders")
}

func (a paymentCenterAPI) adminOrder(w http.ResponseWriter, r *http.Request) {
	order, err := a.service.GetOrder(r.Context(), r.PathValue("orderNo"))
	if err != nil {
		writePaymentDomainError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": paymentOrderResponse(order)})
}

func (a paymentCenterAPI) adminTransactions(w http.ResponseWriter, r *http.Request) {
	a.adminList(w, r, "transactions")
}

func (a paymentCenterAPI) adminFulfillments(w http.ResponseWriter, r *http.Request) {
	a.adminList(w, r, "fulfillments")
}

func (a paymentCenterAPI) adminRetryFulfillment(w http.ResponseWriter, r *http.Request) {
	if err := a.service.RetryFulfillment(r.Context(), r.PathValue("id")); err != nil {
		writePaymentDomainError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a paymentCenterAPI) adminList(w http.ResponseWriter, r *http.Request, resource string) {
	if !a.available() {
		writePaymentError(w, http.StatusServiceUnavailable, errors.New("payment center is unavailable"))
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	query, args := paymentAdminQuery(resource, r, limit, offset)
	rows, err := a.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		writePaymentError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			writePaymentError(w, http.StatusInternalServerError, err)
			return
		}
		var item map[string]any
		if err := json.Unmarshal(raw, &item); err != nil {
			writePaymentError(w, http.StatusInternalServerError, err)
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writePaymentError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": len(items), "limit": limit, "offset": offset})
}

func paymentAdminQuery(resource string, r *http.Request, limit, offset int) (string, []any) {
	args := []any{}
	where := []string{"1=1"}
	add := func(column, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		args = append(args, value)
		where = append(where, fmt.Sprintf("%s=$%d", column, len(args)))
	}
	switch resource {
	case "orders":
		add("o.order_no", r.URL.Query().Get("orderNo"))
		add("o.user_id", r.URL.Query().Get("userId"))
		add("o.tenant_id", r.URL.Query().Get("tenantId"))
		add("o.product_code", r.URL.Query().Get("productCode"))
		add("o.platform", r.URL.Query().Get("platform"))
		add("coalesce(o.channel,o.payment_channel)", r.URL.Query().Get("channel"))
		add("coalesce(o.order_status,o.status)", r.URL.Query().Get("orderStatus"))
		add("coalesce(p.payment_status,p.prepay_status)", r.URL.Query().Get("paymentStatus"))
		add("o.fulfillment_status", r.URL.Query().Get("fulfillmentStatus"))
		addPaymentCreatedRange(&where, &args, "p.created_at", r)
		args = append(args, limit, offset)
		return `SELECT jsonb_build_object(
		  'orderNo',o.order_no,'userId',o.user_id,'tenantId',o.tenant_id,'productCode',o.product_code,
		  'productName',o.product_name,'amount',o.payable_amount_cents,'currency',o.currency,
		  'platform',o.platform,'channel',coalesce(o.channel,o.payment_channel),
		  'orderStatus',coalesce(o.order_status,o.status),'paymentStatus',coalesce(p.payment_status,p.prepay_status),
		  'fulfillmentStatus',o.fulfillment_status,'createdAt',o.created_at,'paidAt',o.paid_at,'fulfilledAt',o.fulfilled_at)
		FROM xz_orders o LEFT JOIN LATERAL (
		  SELECT * FROM xz_payment_records item WHERE item.order_id=o.id ORDER BY item.created_at DESC LIMIT 1
		) p ON true WHERE ` + strings.Join(where, " AND ") +
			fmt.Sprintf(" ORDER BY p.created_at DESC NULLS LAST LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args
	case "transactions":
		add("p.order_no", r.URL.Query().Get("orderNo"))
		add("p.user_id", r.URL.Query().Get("userId"))
		add("p.tenant_id", r.URL.Query().Get("tenantId"))
		add("o.product_code", r.URL.Query().Get("productCode"))
		add("o.platform", r.URL.Query().Get("platform"))
		add("coalesce(o.order_status,o.status)", r.URL.Query().Get("orderStatus"))
		add("o.fulfillment_status", r.URL.Query().Get("fulfillmentStatus"))
		add("p.payment_channel", r.URL.Query().Get("channel"))
		add("coalesce(p.payment_status,p.prepay_status)", r.URL.Query().Get("paymentStatus"))
		addPaymentCreatedRange(&where, &args, "p.created_at", r)
		args = append(args, limit, offset)
		return `SELECT jsonb_build_object(
		  'paymentNo',p.payment_no,'orderNo',p.order_no,'userId',p.user_id,'tenantId',p.tenant_id,
		  'provider',p.provider,'providerTradeNo',p.provider_trade_no,'providerTransactionId',p.provider_transaction_id,
		  'amount',p.amount_cents,'currency',p.currency,'paymentStatus',p.payment_status,
		  'failureCode',p.failure_code,'failureMessage',p.failure_message,'paidAt',p.paid_at,'createdAt',p.created_at)
		FROM xz_payment_records p JOIN xz_orders o ON o.id=p.order_id WHERE ` + strings.Join(where, " AND ") +
			fmt.Sprintf(" ORDER BY p.created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args
	default:
		add("f.order_no", r.URL.Query().Get("orderNo"))
		add("f.user_id", r.URL.Query().Get("userId"))
		add("o.tenant_id", r.URL.Query().Get("tenantId"))
		add("o.product_code", r.URL.Query().Get("productCode"))
		add("o.platform", r.URL.Query().Get("platform"))
		add("coalesce(o.channel,o.payment_channel)", r.URL.Query().Get("channel"))
		add("coalesce(o.order_status,o.status)", r.URL.Query().Get("orderStatus"))
		add("coalesce(p.payment_status,p.prepay_status)", r.URL.Query().Get("paymentStatus"))
		add("f.fulfillment_status", r.URL.Query().Get("fulfillmentStatus"))
		addPaymentCreatedRange(&where, &args, "f.created_at", r)
		args = append(args, limit, offset)
		return `SELECT jsonb_build_object(
		  'id',f.id,'orderNo',f.order_no,'userId',f.user_id,'fulfillmentType',f.fulfillment_type,
		  'fulfillmentStatus',f.fulfillment_status,'retryCount',f.retry_count,'failureMessage',f.failure_message,
		  'payload',f.fulfillment_payload,'fulfilledAt',f.fulfilled_at,'createdAt',f.created_at,'updatedAt',f.updated_at)
		FROM xz_fulfillment_records f JOIN xz_orders o ON o.order_no=f.order_no LEFT JOIN LATERAL (
		  SELECT * FROM xz_payment_records item WHERE item.order_id=o.id ORDER BY item.created_at DESC LIMIT 1
		) p ON true WHERE ` + strings.Join(where, " AND ") +
			fmt.Sprintf(" ORDER BY f.created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args
	}
}

func addPaymentCreatedRange(where *[]string, args *[]any, column string, r *http.Request) {
	for _, item := range []struct{ key, op string }{{"createdFrom", ">="}, {"createdTo", "<="}} {
		value := strings.TrimSpace(r.URL.Query().Get(item.key))
		if value == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			*args = append(*args, parsed)
			*where = append(*where, fmt.Sprintf("%s %s $%d", column, item.op, len(*args)))
		}
	}
}

func paymentClientIP(r *http.Request) string {
	value := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0])
	if value == "" {
		value = r.RemoteAddr
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	return value
}

func writePaymentDomainError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch paymentapp.ErrorCodeOf(err) {
	case paymentapp.CodeProductNotFound, paymentapp.CodeOrderNotFound:
		status = http.StatusNotFound
	case paymentapp.CodeOrderForbidden, paymentapp.CodeMockForbidden:
		status = http.StatusForbidden
	case paymentapp.CodeIdempotencyConflict, paymentapp.CodeDuplicateTransaction, paymentapp.CodeInvalidTransition, paymentapp.CodeCapabilityUnavailable:
		status = http.StatusConflict
	case paymentapp.CodeFulfillmentUnsupported:
		status = http.StatusUnprocessableEntity
	case paymentapp.CodeFulfillmentFailed:
		status = http.StatusInternalServerError
	}
	writePaymentErrorWithCode(w, status, paymentapp.ErrorCodeOf(err), err.Error())
}

func writePaymentError(w http.ResponseWriter, status int, err error) {
	writePaymentErrorWithCode(w, status, paymentapp.ErrorCodeOf(err), err.Error())
}

func writePaymentErrorWithCode(w http.ResponseWriter, status int, code paymentapp.ErrorCode, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message, "code": code})
}
