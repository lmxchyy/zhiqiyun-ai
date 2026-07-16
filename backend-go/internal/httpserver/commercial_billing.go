package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var errCommercialBillingRequiresPostgres = errors.New("commercial billing requires PostgreSQL")

func (a adminAPI) commercialBillingDB() (*sql.DB, error) {
	store, ok := a.store.(*postgresStore)
	if !ok || store.db == nil {
		return nil, errCommercialBillingRequiresPostgres
	}
	return store.db, nil
}

func queryCommercialBillingRows(ctx context.Context, db *sql.DB, query string, args ...any) ([]map[string]any, error) {
	var payload []byte
	if err := db.QueryRowContext(ctx, `select coalesce(jsonb_agg(to_jsonb(item)), '[]'::jsonb) from (`+query+`) item`, args...).Scan(&payload); err != nil {
		return nil, err
	}
	items := []map[string]any{}
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (a adminAPI) commercialBillingList(w http.ResponseWriter, r *http.Request, view string) {
	db, err := a.commercialBillingDB()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var query string
	switch view {
	case "customers":
		query = `
			select u.id as "customerId", coalesce(u.name, '') as "customerName",
			       coalesce(u.email, '') as email, coalesce(u.role, '') as role,
			       coalesce(u.status, '') as status,
			       coalesce(s.plan_id, u.plan_id, '') as "planId",
			       coalesce(s.product_code, '') as "productCode",
			       coalesce(s.status, 'NONE') as "subscriptionStatus",
			       s.ends_at as "subscriptionEndsAt",
			       coalesce(a.available, 0) as "availablePoints",
			       coalesce(a.frozen, 0) as "frozenPoints",
			       coalesce(o.order_count, 0) as "orderCount",
			       coalesce(o.paid_cents, 0) as "paidCents",
			       coalesce(o.refunded_cents, 0) as "refundedCents",
			       coalesce(o.last_order_at, null) as "lastOrderAt"
			from xz_users u
			left join lateral (
			  select plan_id, product_code, status, ends_at
			  from xz_billing_subscriptions where user_id = u.id
			  order by ends_at desc limit 1
			) s on true
			left join lateral (
			  select available, frozen from xz_point_accounts where user_id = u.id order by id limit 1
			) a on true
			left join lateral (
			  select count(*) as order_count,
			         coalesce(sum(amount_cents) filter (where upper(coalesce(status,'')) in ('PAID','SUCCEEDED')), 0) as paid_cents,
			         coalesce(sum(amount_cents) filter (where upper(coalesce(status,'')) = 'REFUNDED'), 0) as refunded_cents,
			         max(case when coalesce(created_at,'') ~ '^2[0-9]{3}-' then created_at::timestamptz end) as last_order_at
			  from xz_orders where user_id = u.id and payment_channel = 'WECHAT_VIRTUAL'
			) o on true
			where coalesce(o.order_count, 0) > 0 or s.plan_id is not null
			order by o.last_order_at desc nulls last, u.id limit 500`
	case "products", "plans":
		query = `
			select p.id as "planId", coalesce(p.code,'') as "planCode", coalesce(p.name,'') as name,
			       coalesce(p.payment_product_code,'') as "productCode",
			       coalesce(p.plan_type,'') as "planType", coalesce(p.product_type,'') as "productType",
			       p.price_cents as "priceCents", p.grant_points as "grantPoints",
			       coalesce(p.token_amount,0) as "tokenAmount", p.duration_days as "durationDays",
			       coalesce(p.member_level,'') as "memberLevel", coalesce(p.agent_level,'') as "agentLevel",
			       p.active, p.entitlements,
			       coalesce(jsonb_agg(jsonb_build_object(
			         'mappingId', m.id, 'environment', case m.env when 0 then 'PRODUCTION' else 'SANDBOX' end,
			         'wechatProductId', m.wechat_product_id, 'offerId', m.offer_id,
			         'mode', m.mode, 'enabled', m.enabled
			       ) order by m.env) filter (where m.id is not null), '[]'::jsonb) as "wechatMappings"
			from xz_plans p
			left join xz_wechat_virtual_product_mappings m on m.plan_id = p.id
			where coalesce(p.payment_product_code,'') <> ''
			group by p.id order by p.active desc, p.price_cents, p.id limit 500`
	case "subscriptions":
		query = `
			select s.id, s.tenant_id as "tenantId", s.user_id as "userId",
			       coalesce(u.name,'') as "customerName", coalesce(u.email,'') as email,
			       s.plan_id as "planId", coalesce(p.name,'') as "planName", s.product_code as "productCode",
			       s.source_order_no as "orderNo", s.status, s.starts_at as "startsAt", s.ends_at as "endsAt",
			       s.entitlement_snapshot as "entitlementSnapshot", s.cancelled_at as "cancelledAt",
			       s.created_at as "createdAt", s.updated_at as "updatedAt"
			from xz_billing_subscriptions s
			left join xz_users u on u.id = s.user_id
			left join xz_plans p on p.id = s.plan_id
			order by s.created_at desc limit 500`
	case "coupons":
		query = `
			select c.id, c.code, c.name, c.benefit_type as "benefitType", c.benefit_value as "benefitValue",
			       c.applicable_product_codes as "applicableProductCodes", c.max_redemptions as "maxRedemptions",
			       c.per_user_limit as "perUserLimit", c.starts_at as "startsAt", c.ends_at as "endsAt",
			       c.status, count(r.id) filter (where r.status = 'APPLIED') as "appliedCount",
			       count(r.id) filter (where r.status = 'RESERVED') as "reservedCount",
			       c.created_at as "createdAt", c.updated_at as "updatedAt"
			from xz_billing_coupons c
			left join xz_billing_coupon_redemptions r on r.coupon_id = c.id
			group by c.id order by c.created_at desc limit 500`
	case "invoices":
		query = `
			select i.id, i.invoice_no as "invoiceNo", i.tenant_id as "tenantId", i.user_id as "userId",
			       coalesce(u.name,'') as "customerName", i.order_no as "orderNo", i.invoice_type as "invoiceType",
			       i.currency, i.subtotal_cents as "subtotalCents", i.discount_cents as "discountCents",
			       i.tax_cents as "taxCents", i.total_cents as "totalCents", i.paid_cents as "paidCents",
			       i.status, i.payment_status as "paymentStatus", i.tax_invoice_status as "taxInvoiceStatus",
			       i.tax_title as "taxTitle", i.tax_number as "taxNumber", i.tax_email as "taxEmail",
			       i.issued_invoice_no as "issuedInvoiceNo", i.issued_at as "issuedAt",
			       i.due_at as "dueAt", i.created_at as "createdAt", i.updated_at as "updatedAt"
			from xz_billing_invoices i left join xz_users u on u.id = i.user_id
			order by i.created_at desc limit 500`
	case "creditNotes":
		query = `
			select c.id, c.credit_note_no as "creditNoteNo", c.invoice_id as "invoiceId",
			       i.invoice_no as "invoiceNo", c.order_no as "orderNo", c.amount_cents as "amountCents",
			       c.currency, c.reason, c.status, c.refund_status as "refundStatus",
			       c.refund_record_id as "refundRecordId", c.reviewed_by as "reviewedBy",
			       c.reviewed_at as "reviewedAt", c.created_at as "createdAt", c.updated_at as "updatedAt"
			from xz_billing_credit_notes c join xz_billing_invoices i on i.id = c.invoice_id
			order by c.created_at desc limit 500`
	case "paymentRequests":
		query = `
			select p.id, p.request_no as "requestNo", p.invoice_id as "invoiceId", i.invoice_no as "invoiceNo",
			       p.order_no as "orderNo", p.tenant_id as "tenantId", p.user_id as "userId",
			       coalesce(u.name,'') as "customerName", p.provider, p.amount_cents as "amountCents",
			       p.currency, p.status, p.dunning_status as "dunningStatus", p.dunning_attempts as "dunningAttempts",
			       p.due_at as "dueAt", p.expires_at as "expiresAt", p.paid_at as "paidAt",
			       p.created_at as "createdAt", p.updated_at as "updatedAt"
			from xz_billing_payment_requests p
			join xz_billing_invoices i on i.id = p.invoice_id left join xz_users u on u.id = p.user_id
			order by p.created_at desc limit 500`
	case "payments":
		query = `
			select p.id, p.payment_no as "paymentNo", p.order_no as "orderNo",
			       p.tenant_id as "tenantId", p.user_id as "userId", coalesce(u.name,'') as "customerName",
			       p.payment_channel as "paymentChannel", p.payment_scene as "paymentScene",
			       p.amount_cents as "amountCents", p.prepay_status as status,
			       coalesce(p.wechat_order_id,'') as "wechatOrderId",
			       coalesce(p.wechat_transaction_id,'') as "wechatTransactionId",
			       coalesce(p.failure_reason,'') as "failureReason", p.paid_at as "paidAt",
			       i.invoice_no as "invoiceNo", pr.request_no as "paymentRequestNo",
			       p.created_at as "createdAt", p.updated_at as "updatedAt"
			from xz_payment_records p left join xz_users u on u.id = p.user_id
			left join xz_billing_invoices i on i.order_id = p.order_id
			left join xz_billing_payment_requests pr on pr.order_id = p.order_id
			order by p.created_at desc limit 500`
	default:
		writeError(w, http.StatusNotFound, errors.New("unknown commercial billing view"))
		return
	}
	items, err := queryCommercialBillingRows(ctx, db, query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": len(items), "source": "DATABASE"})
}

type commercialCouponMutation struct {
	Code                   *string    `json:"code"`
	Name                   *string    `json:"name"`
	BenefitType            *string    `json:"benefitType"`
	BenefitValue           *int64     `json:"benefitValue"`
	ApplicableProductCodes *[]string  `json:"applicableProductCodes"`
	MaxRedemptions         *int64     `json:"maxRedemptions"`
	PerUserLimit           *int       `json:"perUserLimit"`
	StartsAt               *time.Time `json:"startsAt"`
	EndsAt                 *time.Time `json:"endsAt"`
	Status                 *string    `json:"status"`
}

func normalizeCouponBenefit(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func validCouponBenefit(value string) bool {
	switch normalizeCouponBenefit(value) {
	case "BONUS_CREDITS", "BONUS_IMAGE_QUOTA", "EXTEND_MEMBERSHIP_DAYS":
		return true
	default:
		return false
	}
}

func validCouponStatus(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "DRAFT", "ACTIVE", "INACTIVE", "EXPIRED":
		return true
	default:
		return false
	}
}

func (a adminAPI) createBillingCoupon(w http.ResponseWriter, r *http.Request) {
	db, err := a.commercialBillingDB()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	var input commercialCouponMutation
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if input.Code == nil || input.Name == nil || input.BenefitType == nil || input.BenefitValue == nil || *input.BenefitValue <= 0 || !validCouponBenefit(*input.BenefitType) {
		writeError(w, http.StatusBadRequest, errors.New("code, name, valid benefitType and positive benefitValue are required"))
		return
	}
	status := "DRAFT"
	if input.Status != nil {
		status = strings.ToUpper(strings.TrimSpace(*input.Status))
	}
	if !validCouponStatus(status) {
		writeError(w, http.StatusBadRequest, errors.New("invalid coupon status"))
		return
	}
	products := []string{}
	if input.ApplicableProductCodes != nil {
		products = *input.ApplicableProductCodes
	}
	perUserLimit := 1
	if input.PerUserLimit != nil {
		perUserLimit = *input.PerUserLimit
	}
	if perUserLimit <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("perUserLimit must be positive"))
		return
	}
	id := "coupon_" + strings.ToLower(newVirtualBusinessNo("C"))
	_, err = db.ExecContext(r.Context(), `
		insert into xz_billing_coupons(
		 id, code, name, benefit_type, benefit_value, applicable_product_codes,
		 max_redemptions, per_user_limit, starts_at, ends_at, status
		) values ($1, upper($2), $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, id, strings.TrimSpace(*input.Code), strings.TrimSpace(*input.Name), normalizeCouponBenefit(*input.BenefitType),
		*input.BenefitValue, products, input.MaxRedemptions, perUserLimit, input.StartsAt, input.EndsAt, status)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"id": id, "code": strings.ToUpper(strings.TrimSpace(*input.Code)), "status": status})
}

func (a adminAPI) updateBillingCoupon(w http.ResponseWriter, r *http.Request) {
	db, err := a.commercialBillingDB()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	var input commercialCouponMutation
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if input.BenefitType != nil && !validCouponBenefit(*input.BenefitType) {
		writeError(w, http.StatusBadRequest, errors.New("invalid benefitType"))
		return
	}
	if input.Status != nil && !validCouponStatus(*input.Status) {
		writeError(w, http.StatusBadRequest, errors.New("invalid status"))
		return
	}
	if input.BenefitValue != nil && *input.BenefitValue <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("benefitValue must be positive"))
		return
	}
	result, err := db.ExecContext(r.Context(), `
		update xz_billing_coupons set
		 code = case when $2::text is null then code else upper(btrim($2)) end,
		 name = coalesce($3, name), benefit_type = coalesce(upper($4), benefit_type),
		 benefit_value = coalesce($5, benefit_value),
		 applicable_product_codes = coalesce($6, applicable_product_codes),
		 max_redemptions = case when $7::bigint is null then max_redemptions else $7 end,
		 per_user_limit = coalesce($8, per_user_limit),
		 starts_at = case when $9::timestamptz is null then starts_at else $9 end,
		 ends_at = case when $10::timestamptz is null then ends_at else $10 end,
		 status = coalesce(upper($11), status), updated_at = now()
		where id = $1
	`, r.PathValue("id"), input.Code, input.Name, input.BenefitType, input.BenefitValue,
		input.ApplicableProductCodes, input.MaxRedemptions, input.PerUserLimit, input.StartsAt, input.EndsAt, input.Status)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, errors.New("coupon not found"))
		return
	}
	writeJSON(w, map[string]any{"id": r.PathValue("id"), "updated": true})
}

func (a adminAPI) updateBillingSubscription(w http.ResponseWriter, r *http.Request) {
	db, err := a.commercialBillingDB()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	var input struct {
		Status string     `json:"status"`
		EndsAt *time.Time `json:"endsAt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	status := strings.ToUpper(strings.TrimSpace(input.Status))
	if status != "ACTIVE" && status != "CANCELLED" {
		writeError(w, http.StatusBadRequest, errors.New("subscription status must be ACTIVE or CANCELLED"))
		return
	}
	result, err := db.ExecContext(r.Context(), `
		update xz_billing_subscriptions set status=$2,
		 ends_at=coalesce($3, ends_at), cancelled_at=case when $2='CANCELLED' then now() else null end,
		 updated_at=now() where id=$1
	`, r.PathValue("id"), status, input.EndsAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, errors.New("subscription not found"))
		return
	}
	writeJSON(w, map[string]any{"id": r.PathValue("id"), "status": status})
}

func (a adminAPI) updateBillingInvoice(w http.ResponseWriter, r *http.Request) {
	db, err := a.commercialBillingDB()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	var input struct {
		TaxInvoiceStatus string `json:"taxInvoiceStatus"`
		TaxTitle         string `json:"taxTitle"`
		TaxNumber        string `json:"taxNumber"`
		TaxEmail         string `json:"taxEmail"`
		IssuedInvoiceNo  string `json:"issuedInvoiceNo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	status := strings.ToUpper(strings.TrimSpace(input.TaxInvoiceStatus))
	if status != "REQUESTED" && status != "ISSUED" && status != "REJECTED" {
		writeError(w, http.StatusBadRequest, errors.New("invalid taxInvoiceStatus"))
		return
	}
	if status == "ISSUED" && strings.TrimSpace(input.IssuedInvoiceNo) == "" {
		writeError(w, http.StatusBadRequest, errors.New("issuedInvoiceNo is required"))
		return
	}
	result, err := db.ExecContext(r.Context(), `
		update xz_billing_invoices set tax_invoice_status=$2, tax_title=$3, tax_number=$4,
		 tax_email=$5, issued_invoice_no=$6,
		 issued_at=case when $2='ISSUED' then now() else issued_at end, updated_at=now()
		where id=$1
	`, r.PathValue("id"), status, strings.TrimSpace(input.TaxTitle), strings.TrimSpace(input.TaxNumber),
		strings.TrimSpace(input.TaxEmail), strings.TrimSpace(input.IssuedInvoiceNo))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, errors.New("invoice not found"))
		return
	}
	writeJSON(w, map[string]any{"id": r.PathValue("id"), "taxInvoiceStatus": status})
}

func (a adminAPI) createBillingCreditNote(w http.ResponseWriter, r *http.Request) {
	db, err := a.commercialBillingDB()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	var input struct {
		InvoiceID   string `json:"invoiceId"`
		AmountCents int64  `json:"amountCents"`
		Reason      string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(input.InvoiceID) == "" || input.AmountCents <= 0 || strings.TrimSpace(input.Reason) == "" {
		writeError(w, http.StatusBadRequest, errors.New("invoiceId, positive amountCents and reason are required"))
		return
	}
	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var orderID, orderNo string
	var total, credited int64
	err = tx.QueryRowContext(r.Context(), `
		select i.order_id, i.order_no, i.total_cents,
		 coalesce((select sum(c.amount_cents) from xz_billing_credit_notes c where c.invoice_id=i.id and c.status <> 'REJECTED'),0)
		from xz_billing_invoices i where i.id=$1 for update
	`, input.InvoiceID).Scan(&orderID, &orderNo, &total, &credited)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("invoice not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if input.AmountCents > total-credited {
		writeError(w, http.StatusBadRequest, errors.New("credit note exceeds remaining invoice amount"))
		return
	}
	id, number := "credit_note_"+strings.ToLower(newVirtualBusinessNo("C")), newVirtualBusinessNo("CN")
	_, err = tx.ExecContext(r.Context(), `
		insert into xz_billing_credit_notes(id, credit_note_no, invoice_id, order_id, order_no, amount_cents, reason)
		values($1,$2,$3,$4,$5,$6,$7)
	`, id, number, input.InvoiceID, orderID, orderNo, input.AmountCents, strings.TrimSpace(input.Reason))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"id": id, "creditNoteNo": number, "status": "PENDING_REVIEW"})
}

func (a adminAPI) reviewBillingCreditNote(w http.ResponseWriter, r *http.Request) {
	db, err := a.commercialBillingDB()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	var input struct {
		Status     string `json:"status"`
		ReviewedBy string `json:"reviewedBy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	status := strings.ToUpper(strings.TrimSpace(input.Status))
	if status != "FINALIZED" && status != "REJECTED" {
		writeError(w, http.StatusBadRequest, errors.New("status must be FINALIZED or REJECTED"))
		return
	}
	result, err := db.ExecContext(r.Context(), `update xz_billing_credit_notes set status=$2, reviewed_by=$3, reviewed_at=now(), updated_at=now() where id=$1 and status='PENDING_REVIEW'`, r.PathValue("id"), status, strings.TrimSpace(input.ReviewedBy))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusConflict, errors.New("credit note not found or already reviewed"))
		return
	}
	writeJSON(w, map[string]any{"id": r.PathValue("id"), "status": status, "refundTriggered": false})
}

func (a adminAPI) recordBillingDunning(w http.ResponseWriter, r *http.Request) {
	db, err := a.commercialBillingDB()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	var input struct {
		Action  string `json:"action"`
		Channel string `json:"channel"`
		Note    string `json:"note"`
		ActorID string `json:"actorId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	action := strings.ToUpper(strings.TrimSpace(input.Action))
	channel := strings.ToUpper(strings.TrimSpace(input.Channel))
	if action == "" {
		action = "REMINDER_RECORDED"
	}
	if channel == "" {
		channel = "MANUAL"
	}
	if action != "REMINDER_RECORDED" && action != "MANUAL_CONTACT" && action != "STOP_DUNNING" {
		writeError(w, http.StatusBadRequest, errors.New("invalid dunning action"))
		return
	}
	if channel != "MANUAL" && channel != "IN_APP" && channel != "SMS" && channel != "EMAIL" {
		writeError(w, http.StatusBadRequest, errors.New("invalid dunning channel"))
		return
	}
	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var paymentStatus string
	if err := tx.QueryRowContext(r.Context(), `select status from xz_billing_payment_requests where id=$1 for update`, r.PathValue("id")).Scan(&paymentStatus); err != nil {
		writeError(w, http.StatusNotFound, errors.New("payment request not found"))
		return
	}
	if paymentStatus != "PENDING" && action != "STOP_DUNNING" {
		writeError(w, http.StatusConflict, fmt.Errorf("payment request status %s cannot be dunned", paymentStatus))
		return
	}
	id := "dunning_" + strings.ToLower(newVirtualBusinessNo("D"))
	_, err = tx.ExecContext(r.Context(), `insert into xz_billing_dunning_events(id,payment_request_id,action,channel,actor_id,note) values($1,$2,$3,$4,$5,$6)`, id, r.PathValue("id"), action, channel, strings.TrimSpace(input.ActorID), strings.TrimSpace(input.Note))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	_, err = tx.ExecContext(r.Context(), `update xz_billing_payment_requests set dunning_status=case when $2='STOP_DUNNING' then 'STOPPED' else 'IN_PROGRESS' end, dunning_attempts=dunning_attempts+case when $2='STOP_DUNNING' then 0 else 1 end, updated_at=now() where id=$1`, r.PathValue("id"), action)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"id": id, "recorded": true, "externalMessageSent": false})
}
