package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

func (s *virtualPaymentService) availableCouponByCode(ctx context.Context, userID string, product virtualPaymentProduct, code string) (*virtualCouponBenefit, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, nil
	}
	var item virtualCouponBenefit
	var status string
	var applicable []string
	var applicableJSON []byte
	var startsAt, endsAt sql.NullTime
	var maxRedemptions sql.NullInt64
	var perUserLimit int
	err := s.db.QueryRowContext(ctx, `
		select id, code, name, benefit_type, benefit_value, to_jsonb(applicable_product_codes),
		       status, starts_at, ends_at, max_redemptions, per_user_limit
		from xz_billing_coupons where code = $1
	`, code).Scan(&item.ID, &item.Code, &item.Name, &item.BenefitType, &item.BenefitValue, &applicableJSON,
		&status, &startsAt, &endsAt, &maxRedemptions, &perUserLimit)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("优惠券不存在")
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(applicableJSON, &applicable); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if status != "ACTIVE" || startsAt.Valid && startsAt.Time.After(now) || endsAt.Valid && !endsAt.Time.After(now) {
		return nil, errors.New("优惠券当前不可用")
	}
	if len(applicable) > 0 && !containsFold(applicable, product.Code) {
		return nil, errors.New("优惠券不适用于当前商品")
	}
	if item.BenefitType == "EXTEND_MEMBERSHIP_DAYS" && normalizePlanTypeString(product.PlanType) != planTypeMemberPackage {
		return nil, errors.New("会员延期券只能用于会员套餐")
	}
	var globalUsed, userUsed int64
	err = s.db.QueryRowContext(ctx, `
		select count(*) filter (where status in ('RESERVED','APPLIED')),
		       count(*) filter (where user_id=$2 and status in ('RESERVED','APPLIED'))
		from xz_billing_coupon_redemptions where coupon_id=$1
	`, item.ID, userID).Scan(&globalUsed, &userUsed)
	if err != nil {
		return nil, err
	}
	if maxRedemptions.Valid && globalUsed >= maxRedemptions.Int64 {
		return nil, errors.New("优惠券已领完")
	}
	if userUsed >= int64(perUserLimit) {
		return nil, errors.New("已达到该优惠券的个人使用上限")
	}
	return &item, nil
}

func containsFold(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}

func (s *virtualPaymentService) listAvailableCoupons(ctx context.Context, userID, productCode string) ([]virtualCouponBenefit, error) {
	rows, err := s.db.QueryContext(ctx, `
		select c.id, c.code, c.name, c.benefit_type, c.benefit_value
		from xz_billing_coupons c
		where c.status='ACTIVE'
		  and (c.starts_at is null or c.starts_at <= now())
		  and (c.ends_at is null or c.ends_at > now())
		  and ($2='' or cardinality(c.applicable_product_codes)=0 or $2=any(c.applicable_product_codes))
		  and (c.max_redemptions is null or c.max_redemptions > (
		    select count(*) from xz_billing_coupon_redemptions r where r.coupon_id=c.id and r.status in ('RESERVED','APPLIED')
		  ))
		  and c.per_user_limit > (
		    select count(*) from xz_billing_coupon_redemptions r where r.coupon_id=c.id and r.user_id=$1 and r.status in ('RESERVED','APPLIED')
		  )
		order by c.created_at desc
	`, userID, strings.TrimSpace(productCode))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []virtualCouponBenefit{}
	for rows.Next() {
		var item virtualCouponBenefit
		if err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.BenefitType, &item.BenefitValue); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func reserveVirtualCouponTx(ctx context.Context, tx *sql.Tx, orderNo, tenantID, userID, productCode string, expected virtualCouponBenefit) error {
	var item virtualCouponBenefit
	var status string
	var applicable []string
	var applicableJSON []byte
	var startsAt, endsAt sql.NullTime
	var maxRedemptions sql.NullInt64
	var perUserLimit int
	err := tx.QueryRowContext(ctx, `
		select id, code, name, benefit_type, benefit_value, to_jsonb(applicable_product_codes),
		       status, starts_at, ends_at, max_redemptions, per_user_limit
		from xz_billing_coupons where id=$1 for update
	`, expected.ID).Scan(&item.ID, &item.Code, &item.Name, &item.BenefitType, &item.BenefitValue, &applicableJSON,
		&status, &startsAt, &endsAt, &maxRedemptions, &perUserLimit)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(applicableJSON, &applicable); err != nil {
		return err
	}
	now := time.Now().UTC()
	if status != "ACTIVE" || startsAt.Valid && startsAt.Time.After(now) || endsAt.Valid && !endsAt.Time.After(now) ||
		!strings.EqualFold(item.Code, expected.Code) || item.BenefitType != expected.BenefitType || item.BenefitValue != expected.BenefitValue {
		return errors.New("优惠券状态已变化，请刷新后重试")
	}
	if len(applicable) > 0 && !containsFold(applicable, productCode) {
		return errors.New("优惠券不适用于当前商品")
	}
	var globalUsed, userUsed int64
	if err := tx.QueryRowContext(ctx, `
		select count(*) filter (where status in ('RESERVED','APPLIED')),
		       count(*) filter (where user_id=$2 and status in ('RESERVED','APPLIED'))
		from xz_billing_coupon_redemptions where coupon_id=$1
	`, item.ID, userID).Scan(&globalUsed, &userUsed); err != nil {
		return err
	}
	if maxRedemptions.Valid && globalUsed >= maxRedemptions.Int64 || userUsed >= int64(perUserLimit) {
		return errors.New("优惠券使用次数已达上限")
	}
	idempotencyKey := "virtual-payment:" + orderNo + ":coupon:" + item.Code
	_, err = tx.ExecContext(ctx, `
		insert into xz_billing_coupon_redemptions(
		 id, coupon_id, order_id, order_no, tenant_id, user_id, product_code,
		 benefit_type, benefit_value, idempotency_key
		) values($1,$2,$3,$3,$4,$5,$6,$7,$8,$9)
	`, virtualPaymentResourceID("coupon_redemption", idempotencyKey), item.ID, orderNo, tenantID, userID,
		productCode, item.BenefitType, item.BenefitValue, idempotencyKey)
	return err
}

func insertCommercialBillingOrderTx(ctx context.Context, tx *sql.Tx, orderNo, tenantID, userID string, amountCents int64, createdAt, expiresAt time.Time) error {
	invoiceID := virtualPaymentResourceID("invoice", orderNo)
	requestID := virtualPaymentResourceID("payment_request", orderNo)
	_, err := tx.ExecContext(ctx, `
		insert into xz_billing_invoices(
		 id, invoice_no, tenant_id, user_id, order_id, order_no, subtotal_cents,
		 total_cents, due_at, created_at, updated_at
		) values($1,$2,$3,$4,$5,$5,$6,$6,$7,$8,$8)
		on conflict(order_id) do nothing
	`, invoiceID, "INV-"+strings.ToUpper(hashSensitiveIdentifier(orderNo)[:16]), tenantID, userID,
		orderNo, amountCents, expiresAt, createdAt)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		insert into xz_billing_payment_requests(
		 id, request_no, invoice_id, order_id, order_no, tenant_id, user_id,
		 amount_cents, due_at, expires_at, created_at, updated_at
		) values($1,$2,$3,$4,$4,$5,$6,$7,$8,$8,$9,$9)
		on conflict(order_id) do nothing
	`, requestID, "PR-"+strings.ToUpper(hashSensitiveIdentifier(orderNo)[:16]), invoiceID, orderNo,
		tenantID, userID, amountCents, expiresAt, createdAt)
	return err
}

func markCommercialOrderPaidTx(ctx context.Context, tx *sql.Tx, order lockedVirtualOrder, paidAt time.Time) error {
	_, err := tx.ExecContext(ctx, `
		update xz_billing_invoices set status='PAID', payment_status='PAID', paid_cents=total_cents, updated_at=now()
		where order_id=$1
	`, order.ID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		update xz_billing_payment_requests set status='SUCCEEDED', dunning_status='RESOLVED', paid_at=$2, updated_at=now()
		where order_id=$1
	`, order.ID, paidAt)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		insert into xz_billing_subscriptions(
		 id, tenant_id, user_id, plan_id, product_code, source_order_id, source_order_no,
		 status, starts_at, ends_at, entitlement_snapshot, created_at, updated_at
		)
		select $1, $2, $3, $4, $5, $6, $7,
		       case when max(r.expires_at)>now() then 'ACTIVE' else 'EXPIRED' end,
		       min(r.effective_at), max(r.expires_at),
		       jsonb_build_object('productCode',$5::text,'memberLevel',$8::text,'memberDays',$9::bigint,'couponCode',$10::text), now(), now()
		from xz_membership_entitlement_records r where r.source_order_no=$7
		having count(*)>0
		on conflict(source_order_id) do update set status=excluded.status, ends_at=excluded.ends_at,
		 entitlement_snapshot=excluded.entitlement_snapshot, updated_at=now()
	`, virtualPaymentResourceID("subscription", order.OrderNo), order.TenantID, order.UserID, order.PlanID,
		order.ProductCode, order.ID, order.OrderNo, order.Snapshot.MemberLevel, order.Snapshot.MemberDays, order.Snapshot.CouponCode)
	return err
}

func markCommercialOrderClosed(ctx context.Context, db *sql.DB, orderNo string) {
	_, _ = db.ExecContext(ctx, `
		update xz_billing_payment_requests set status='CANCELLED', dunning_status='STOPPED', updated_at=now()
		where order_id=$1 and status='PENDING'
	`, orderNo)
	_, _ = db.ExecContext(ctx, `
		update xz_billing_coupon_redemptions set status='CANCELLED', updated_at=now()
		where order_id=$1 and status='RESERVED'
	`, orderNo)
}

func markCommercialOrderRefundedTx(ctx context.Context, tx *sql.Tx, order lockedVirtualOrder, refundID string, amountCents int64, succeeded bool) error {
	status, creditStatus, refundStatus := "REFUND_PENDING", "PENDING_REVIEW", "PENDING"
	if succeeded {
		status, creditStatus, refundStatus = "REFUNDED", "FINALIZED", "SUCCEEDED"
	} else {
		refundStatus = "FAILED"
	}
	_, err := tx.ExecContext(ctx, `
		update xz_billing_invoices set status=case when $2 then 'CREDITED' else status end,
		 payment_status=case when $2 then 'REFUNDED' else payment_status end, updated_at=now() where order_id=$1
	`, order.ID, succeeded)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		update xz_billing_payment_requests set status=$2, dunning_status='STOPPED', updated_at=now() where order_id=$1
	`, order.ID, status)
	if err != nil {
		return err
	}
	if amountCents <= 0 {
		amountCents = order.AmountCents
	}
	_, err = tx.ExecContext(ctx, `
		insert into xz_billing_credit_notes(
		 id, credit_note_no, invoice_id, order_id, order_no, refund_record_id,
		 amount_cents, reason, status, refund_status
		)
		select $1,$2,i.id,$3,$4,$5,$6,'微信虚拟支付退款通知',$7,$8
		from xz_billing_invoices i where i.order_id=$3
		on conflict(refund_record_id) where refund_record_id is not null
		do update set status=excluded.status, refund_status=excluded.refund_status, updated_at=now()
	`, virtualPaymentResourceID("credit_note", refundID), "CN-"+strings.ToUpper(hashSensitiveIdentifier(refundID)[:16]),
		order.ID, order.OrderNo, refundID, amountCents, creditStatus, refundStatus)
	return err
}

func applyVirtualCouponBonusTx(ctx context.Context, tx *sql.Tx, order lockedVirtualOrder, now time.Time) error {
	if strings.TrimSpace(order.Snapshot.CouponCode) == "" {
		return nil
	}
	if order.Snapshot.CouponBenefitValue <= 0 {
		return errors.New("coupon benefit snapshot is invalid")
	}
	key := "virtual-payment:" + order.OrderNo + ":coupon:" + order.Snapshot.CouponCode
	switch order.Snapshot.CouponBenefitType {
	case "BONUS_CREDITS":
		if err := grantVirtualCouponCreditsTx(ctx, tx, order, order.Snapshot.CouponBenefitValue, key, now); err != nil {
			return err
		}
	case "BONUS_IMAGE_QUOTA":
		if err := grantVirtualCouponImageQuotaTx(ctx, tx, order, order.Snapshot.CouponBenefitValue, key); err != nil {
			return err
		}
	case "EXTEND_MEMBERSHIP_DAYS":
		if err := grantVirtualCouponMembershipTx(ctx, tx, order, order.Snapshot.CouponBenefitValue, key, now); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported coupon benefit type: %s", order.Snapshot.CouponBenefitType)
	}
	result, err := tx.ExecContext(ctx, `
		update xz_billing_coupon_redemptions set status='APPLIED', applied_at=now(), updated_at=now()
		where order_id=$1 and idempotency_key=$2 and status in ('RESERVED','APPLIED')
	`, order.ID, key)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return errors.New("reserved coupon redemption is missing")
	}
	return nil
}

func grantVirtualCouponCreditsTx(ctx context.Context, tx *sql.Tx, order lockedVirtualOrder, amount int64, key string, now time.Time) error {
	if amount <= 0 || amount > math.MaxInt {
		return errors.New("coupon credit amount is invalid")
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `select exists(select 1 from xz_token_records where idempotency_key=$1)`, key).Scan(&exists); err != nil || exists {
		return err
	}
	account, err := pointAccountForUpdate(ctx, tx, order.UserID)
	if err != nil {
		return err
	}
	if account.Available < 0 || amount > int64(math.MaxInt-account.Available) {
		return errors.New("coupon credit would overflow wallet")
	}
	before := account.Available
	account.Available += int(amount)
	account.TotalGranted += int(amount)
	if err := insertPointAccount(ctx, tx, account); err != nil {
		return err
	}
	if err := insertAccountBalanceLedgerV1(ctx, tx, account, "GRANT", int(amount), before, account.Available, "WECHAT_VIRTUAL_COUPON", order.OrderNo, "WeChat virtual payment coupon bonus"); err != nil {
		return err
	}
	raw, _ := json.Marshal(map[string]any{"couponCode": order.Snapshot.CouponCode, "productCode": order.ProductCode})
	_, err = tx.ExecContext(ctx, `
		insert into xz_token_records(id,user_id,order_id,change_type,amount,balance_before,balance_after,remark,created_at,tenant_id,idempotency_key,source_order_no,raw)
		values($1,$2,$3,'WECHAT_VIRTUAL_COUPON_BONUS',$4,$5,$6,'wechat_virtual_coupon_bonus',$7,$8,$9,$3,$10::jsonb)
		on conflict do nothing
	`, virtualPaymentResourceID("coupon_token", key), order.UserID, order.OrderNo, amount, before, account.Available,
		now.Format(time.RFC3339Nano), order.TenantID, key, raw)
	return err
}

func grantVirtualCouponImageQuotaTx(ctx context.Context, tx *sql.Tx, order lockedVirtualOrder, amount int64, key string) error {
	if amount <= 0 {
		return errors.New("coupon image quota is invalid")
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `select exists(select 1 from xz_image_quota_ledger where idempotency_key=$1)`, key).Scan(&exists); err != nil || exists {
		return err
	}
	accountID := virtualPaymentResourceID("image_account", order.TenantID+":"+order.UserID)
	if _, err := tx.ExecContext(ctx, `insert into xz_image_quota_accounts(id,tenant_id,user_id) values($1,$2,$3) on conflict(tenant_id,user_id) do nothing`, accountID, order.TenantID, order.UserID); err != nil {
		return err
	}
	var storedID string
	var before int64
	if err := tx.QueryRowContext(ctx, `select id,remaining_images from xz_image_quota_accounts where tenant_id=$1 and user_id=$2 for update`, order.TenantID, order.UserID).Scan(&storedID, &before); err != nil {
		return err
	}
	if amount > math.MaxInt64-before {
		return errors.New("coupon image quota would overflow")
	}
	after := before + amount
	if _, err := tx.ExecContext(ctx, `update xz_image_quota_accounts set remaining_images=$3,total_granted=total_granted+$4,version=version+1,updated_at=now() where tenant_id=$1 and user_id=$2`, order.TenantID, order.UserID, after, amount); err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]any{"couponCode": order.Snapshot.CouponCode, "productCode": order.ProductCode})
	_, err := tx.ExecContext(ctx, `
		insert into xz_image_quota_ledger(id,tenant_id,user_id,account_id,image_delta,balance_before,balance_after,source_order_no,business_type,idempotency_key,metadata)
		values($1,$2,$3,$4,$5,$6,$7,$8,'WECHAT_VIRTUAL_COUPON',$9,$10::jsonb) on conflict(idempotency_key) do nothing
	`, virtualPaymentResourceID("coupon_image", key), order.TenantID, order.UserID, storedID, amount, before, after, order.OrderNo, key, metadata)
	return err
}

func grantVirtualCouponMembershipTx(ctx context.Context, tx *sql.Tx, order lockedVirtualOrder, days int64, key string, now time.Time) error {
	if days <= 0 || normalizePlanTypeString(order.Snapshot.PlanType) != planTypeMemberPackage {
		return errors.New("coupon membership extension is invalid")
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `select exists(select 1 from xz_membership_entitlement_records where idempotency_key=$1)`, key).Scan(&exists); err != nil || exists {
		return err
	}
	user, err := userByIDForUpdateTx(ctx, tx, order.UserID)
	if err != nil {
		return err
	}
	effectiveAt, expiresAt := membershipExtensionWindow(user.SubscriptionExpiresAt, now, days)
	user.SubscriptionExpiresAt = expiresAt.Format(time.RFC3339Nano)
	user.UpdatedAt = now.Format(time.RFC3339Nano)
	if err := insertUser(ctx, tx, user); err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]any{"couponCode": order.Snapshot.CouponCode, "days": days, "productCode": order.ProductCode})
	_, err = tx.ExecContext(ctx, `
		insert into xz_membership_entitlement_records(id,tenant_id,user_id,member_level,effective_at,expires_at,source_order_no,idempotency_key,metadata)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb) on conflict(idempotency_key) do nothing
	`, virtualPaymentResourceID("coupon_membership", key), order.TenantID, order.UserID,
		strings.ToUpper(order.Snapshot.MemberLevel), effectiveAt, expiresAt, order.OrderNo, key, metadata)
	return err
}
