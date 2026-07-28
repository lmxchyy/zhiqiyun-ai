package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

func (a virtualPaymentAPI) adminOverview(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	counts := map[string]int64{}
	queries := map[string]string{
		"products":             `select count(*) from xz_plans where coalesce(payment_product_code, '') <> ''`,
		"pendingOrders":        `select count(*) from xz_orders where payment_channel = 'WECHAT_VIRTUAL' and status = 'PENDING'`,
		"paidOrders":           `select count(*) from xz_orders where payment_channel = 'WECHAT_VIRTUAL' and status = 'PAID'`,
		"failedEntitlements":   `select count(*) from xz_orders where payment_channel = 'WECHAT_VIRTUAL' and entitlement_status = 'FAILED'`,
		"notifications":        `select count(*) from xz_payment_events where provider = 'WECHAT_VIRTUAL'`,
		"pendingRefundReviews": `select count(*) from xz_refund_records where status in ('PENDING_REVIEW','REFUND_PENDING')`,
	}
	for key, query := range queries {
		var count int64
		if err := a.service.db.QueryRowContext(r.Context(), query).Scan(&count); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		counts[key] = count
	}
	writeJSON(w, map[string]any{
		"counts": counts,
		"config": map[string]any{
			"enabled":               a.service.cfg.Enabled,
			"ready":                 a.service.cfg.ready(),
			"env":                   a.service.cfg.Env,
			"mode":                  a.service.cfg.Mode,
			"offerIdConfigured":     a.service.cfg.OfferID != "",
			"appKeyConfigured":      a.service.cfg.AppKey != "",
			"notifyTokenConfigured": a.service.cfg.NotifyToken != "",
		},
	})
}

func (a virtualPaymentAPI) adminProducts(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	items, err := queryVirtualAdminRows(r.Context(), a.service.db, `
		select jsonb_build_object(
		  'id', mapping.id, 'planId', plan.id, 'productCode', plan.payment_product_code,
		  'name', plan.name, 'productType', plan.product_type, 'amountCent', plan.price_cents,
		  'active', plan.active, 'offerId', mapping.offer_id, 'wechatProductId', mapping.wechat_product_id,
		  'mode', mapping.mode, 'env', mapping.env, 'enabled', mapping.enabled,
		  'entitlements', plan.entitlements, 'updatedAt', mapping.updated_at
		)
		from xz_wechat_virtual_product_mappings mapping
		join xz_plans plan on plan.id = mapping.plan_id
		order by mapping.env, plan.price_cents, plan.id
	`, 200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a virtualPaymentAPI) adminUpdateMapping(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	var managed bool
	if err := a.service.db.QueryRowContext(r.Context(), `
		select exists(
			select 1
			from xz_wechat_virtual_product_mappings mapping
			join xz_plans plan on plan.id=mapping.plan_id
			join xz_plan_versions version on version.plan_id=plan.id
			where mapping.id=$1
			  and ((plan.plan_type='MEMBER_PACKAGE' and version.business_type='MEMBER')
			    or (plan.plan_type='AGENT_JOIN_PACKAGE' and version.business_type='AGENT'))
		)
	`, r.PathValue("id")).Scan(&managed); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if managed {
		writeBusinessPlanAdminError(w, newBusinessPlanAdminError(http.StatusConflict, "MANAGED_PLAN_REQUIRES_PAYMENT_BINDING", "V2 managed plan must use the price-plan payment binding API"))
		return
	}
	var request struct {
		OfferID         *string `json:"offerId"`
		WeChatProductID *string `json:"wechatProductId"`
		Mode            *string `json:"mode"`
		Enabled         *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var offerID any
	var productID any
	var mode any
	var enabled any
	if request.OfferID != nil {
		offerID = strings.TrimSpace(*request.OfferID)
	}
	if request.WeChatProductID != nil {
		productID = strings.TrimSpace(*request.WeChatProductID)
		if productID == "" {
			writeError(w, http.StatusBadRequest, errors.New("wechatProductId cannot be empty"))
			return
		}
	}
	if request.Mode != nil {
		mode = strings.TrimSpace(*request.Mode)
		if mode != "short_series_goods" && mode != "short_series_coin" {
			writeError(w, http.StatusBadRequest, errors.New("unsupported virtual payment mode"))
			return
		}
	}
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	var item []byte
	err := a.service.db.QueryRowContext(r.Context(), `
		update xz_wechat_virtual_product_mappings
		set offer_id = case when $2::text is null then offer_id else $2 end,
		    wechat_product_id = case when $3::text is null then wechat_product_id else $3 end,
		    mode = case when $4::text is null then mode else $4 end,
		    enabled = case when $5::boolean is null then enabled else $5 end,
		    updated_at = now()
		where id = $1
		returning jsonb_build_object('id', id, 'planId', plan_id, 'offerId', offer_id,
		  'wechatProductId', wechat_product_id, 'mode', mode, 'env', env, 'enabled', enabled, 'updatedAt', updated_at)
	`, r.PathValue("id"), offerID, productID, mode, enabled).Scan(&item)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("mapping not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var decoded map[string]any
	if err := json.Unmarshal(item, &decoded); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": decoded})
}

func (a virtualPaymentAPI) adminList(resource string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.available(w) {
			return
		}
		limit := virtualAdminListLimit(r)
		query := ""
		switch resource {
		case "orders":
			query = `select jsonb_build_object('orderNo', order_no, 'tenantId', tenant_id, 'userId', user_id,
			  'productCode', product_code, 'productName', product_name, 'amountCent', amount_cents,
			  'orderStatus', status, 'entitlementStatus', entitlement_status, 'entitlementError', entitlement_error,
			  'wechatOrderId', wechat_order_id, 'wechatTransactionId', wechat_transaction_id,
			  'paidAt', paid_at, 'createdAt', created_at, 'updatedAt', updated_at)
			from xz_orders where payment_channel = 'WECHAT_VIRTUAL' order by updated_at desc`
		case "records":
			query = `select jsonb_build_object('paymentNo', payment_no, 'orderNo', order_no, 'tenantId', tenant_id,
			  'userId', user_id, 'amountCent', amount_cents, 'status', prepay_status,
			  'wechatOrderId', wechat_order_id, 'wechatTransactionId', wechat_transaction_id,
			  'failureReason', failure_reason, 'paidAt', paid_at, 'createdAt', created_at, 'updatedAt', updated_at)
			from xz_payment_records order by updated_at desc`
		case "notifications":
			query = `select jsonb_build_object('eventId', event_id, 'eventType', event_type, 'orderNo', order_id,
			  'transactionId', transaction_id, 'amountCent', amount_cents, 'verified', verified,
			  'processingStatus', processing_status, 'processAttempts', process_attempts,
			  'errorMessage', error_message, 'createdAt', created_at, 'processedAt', processed_at)
			from xz_payment_events where provider = 'WECHAT_VIRTUAL' order by created_at desc`
		case "memberships":
			query = `select jsonb_build_object('id', id, 'tenantId', tenant_id, 'userId', user_id,
			  'memberLevel', member_level, 'effectiveAt', effective_at, 'expiresAt', expires_at,
			  'sourceOrderNo', source_order_no, 'idempotencyKey', idempotency_key, 'createdAt', created_at)
			from xz_membership_entitlement_records order by created_at desc`
		case "wallet-ledger":
			query = `select jsonb_build_object('id', id, 'ledgerType', 'CREDITS', 'tenantId', tenant_id, 'userId', user_id,
			  'sourceOrderNo', source_order_no, 'delta', amount, 'balanceBefore', balance_before,
			  'balanceAfter', balance_after, 'idempotencyKey', idempotency_key, 'createdAt', created_at)
			from xz_token_records where idempotency_key like 'virtual-payment:%'
			union all
			select jsonb_build_object('id', id, 'ledgerType', 'IMAGE_QUOTA', 'tenantId', tenant_id, 'userId', user_id,
			  'sourceOrderNo', source_order_no, 'delta', image_delta, 'balanceBefore', balance_before,
			  'balanceAfter', balance_after, 'idempotencyKey', idempotency_key, 'createdAt', created_at)
			from xz_image_quota_ledger order by 1 desc`
		case "refunds":
			query = `select jsonb_build_object('id', id, 'orderNo', order_no, 'tenantId', tenant_id, 'userId', user_id,
			  'providerRefundId', provider_refund_id, 'amountCent', amount_cents, 'status', status,
			  'idempotencyKey', idempotency_key, 'createdAt', created_at, 'updatedAt', updated_at)
			from xz_refund_records order by created_at desc`
		case "failures":
			query = `select jsonb_build_object('orderNo', order_no, 'tenantId', tenant_id, 'userId', user_id,
			  'productCode', product_code, 'orderStatus', status, 'entitlementStatus', entitlement_status,
			  'entitlementError', entitlement_error, 'updatedAt', updated_at)
			from xz_orders where payment_channel = 'WECHAT_VIRTUAL' and entitlement_status = 'FAILED' order by updated_at desc`
		default:
			writeError(w, http.StatusNotFound, errors.New("unknown payment resource"))
			return
		}
		items, err := queryVirtualAdminRows(r.Context(), a.service.db, query, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]any{"items": items, "limit": limit})
	}
}

func (a virtualPaymentAPI) adminGrantOrder(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	if err := a.service.GrantOrderEntitlements(r.Context(), r.PathValue("orderNo")); err != nil {
		writeVirtualPaymentError(w, err)
		return
	}
	item, _, _, err := a.service.orderView(r.Context(), r.PathValue("orderNo"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": item, "message": "权益发放已完成或已幂等确认"})
}

func virtualAdminListLimit(r *http.Request) int {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	return limit
}

func queryVirtualAdminRows(ctx context.Context, db *sql.DB, query string, limit int) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, "select item from ("+query+") as payment_admin(item) limit $1", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var item map[string]any
		if err := json.Unmarshal(payload, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
