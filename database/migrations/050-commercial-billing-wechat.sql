-- Commercial billing records driven by real WeChat virtual-payment orders.
-- Coupons are entitlement bonuses only: they never alter the WeChat goods price.

BEGIN;

CREATE TABLE IF NOT EXISTS xz_billing_coupons (
  id TEXT PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  benefit_type TEXT NOT NULL CHECK (benefit_type IN ('BONUS_CREDITS', 'BONUS_IMAGE_QUOTA', 'EXTEND_MEMBERSHIP_DAYS')),
  benefit_value BIGINT NOT NULL CHECK (benefit_value > 0),
  applicable_product_codes TEXT[] NOT NULL DEFAULT '{}',
  max_redemptions BIGINT,
  per_user_limit INT NOT NULL DEFAULT 1 CHECK (per_user_limit > 0),
  starts_at TIMESTAMPTZ,
  ends_at TIMESTAMPTZ,
  status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'ACTIVE', 'INACTIVE', 'EXPIRED')),
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (max_redemptions IS NULL OR max_redemptions > 0),
  CHECK (ends_at IS NULL OR starts_at IS NULL OR ends_at > starts_at)
);

CREATE INDEX IF NOT EXISTS idx_xz_billing_coupons_status
  ON xz_billing_coupons(status, starts_at, ends_at);

CREATE TABLE IF NOT EXISTS xz_billing_coupon_redemptions (
  id TEXT PRIMARY KEY,
  coupon_id TEXT NOT NULL REFERENCES xz_billing_coupons(id),
  order_id TEXT NOT NULL REFERENCES xz_orders(id),
  order_no TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  product_code TEXT NOT NULL,
  benefit_type TEXT NOT NULL,
  benefit_value BIGINT NOT NULL CHECK (benefit_value > 0),
  status TEXT NOT NULL DEFAULT 'RESERVED' CHECK (status IN ('RESERVED', 'APPLIED', 'CANCELLED')),
  idempotency_key TEXT NOT NULL UNIQUE,
  applied_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(coupon_id, order_id)
);

CREATE INDEX IF NOT EXISTS idx_xz_billing_coupon_redemptions_user
  ON xz_billing_coupon_redemptions(coupon_id, user_id, status);

CREATE TABLE IF NOT EXISTS xz_billing_subscriptions (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  plan_id TEXT NOT NULL,
  product_code TEXT NOT NULL,
  source_order_id TEXT NOT NULL REFERENCES xz_orders(id),
  source_order_no TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'EXPIRED', 'CANCELLED')),
  starts_at TIMESTAMPTZ NOT NULL,
  ends_at TIMESTAMPTZ NOT NULL,
  entitlement_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  cancelled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(source_order_id)
);

CREATE INDEX IF NOT EXISTS idx_xz_billing_subscriptions_customer
  ON xz_billing_subscriptions(tenant_id, user_id, status, ends_at DESC);

CREATE TABLE IF NOT EXISTS xz_billing_invoices (
  id TEXT PRIMARY KEY,
  invoice_no TEXT NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  order_id TEXT NOT NULL REFERENCES xz_orders(id),
  order_no TEXT NOT NULL,
  invoice_type TEXT NOT NULL DEFAULT 'VIRTUAL_PAYMENT',
  currency TEXT NOT NULL DEFAULT 'CNY',
  subtotal_cents BIGINT NOT NULL CHECK (subtotal_cents >= 0),
  discount_cents BIGINT NOT NULL DEFAULT 0 CHECK (discount_cents >= 0),
  tax_cents BIGINT NOT NULL DEFAULT 0 CHECK (tax_cents >= 0),
  total_cents BIGINT NOT NULL CHECK (total_cents >= 0),
  paid_cents BIGINT NOT NULL DEFAULT 0 CHECK (paid_cents >= 0),
  status TEXT NOT NULL DEFAULT 'FINALIZED' CHECK (status IN ('FINALIZED', 'PAID', 'CREDITED', 'VOID')),
  payment_status TEXT NOT NULL DEFAULT 'PENDING' CHECK (payment_status IN ('PENDING', 'PAID', 'FAILED', 'REFUNDED')),
  tax_invoice_status TEXT NOT NULL DEFAULT 'NOT_REQUESTED' CHECK (tax_invoice_status IN ('NOT_REQUESTED', 'REQUESTED', 'ISSUED', 'REJECTED')),
  tax_title TEXT NOT NULL DEFAULT '',
  tax_number TEXT NOT NULL DEFAULT '',
  tax_email TEXT NOT NULL DEFAULT '',
  issued_invoice_no TEXT NOT NULL DEFAULT '',
  issued_at TIMESTAMPTZ,
  due_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(order_id),
  CHECK (total_cents = subtotal_cents - discount_cents + tax_cents)
);

CREATE INDEX IF NOT EXISTS idx_xz_billing_invoices_customer
  ON xz_billing_invoices(tenant_id, user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_billing_credit_notes (
  id TEXT PRIMARY KEY,
  credit_note_no TEXT NOT NULL UNIQUE,
  invoice_id TEXT NOT NULL REFERENCES xz_billing_invoices(id),
  order_id TEXT NOT NULL REFERENCES xz_orders(id),
  order_no TEXT NOT NULL,
  refund_record_id TEXT REFERENCES xz_refund_records(id),
  amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
  currency TEXT NOT NULL DEFAULT 'CNY',
  reason TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'PENDING_REVIEW' CHECK (status IN ('PENDING_REVIEW', 'FINALIZED', 'REJECTED')),
  refund_status TEXT NOT NULL DEFAULT 'PENDING' CHECK (refund_status IN ('PENDING', 'SUCCEEDED', 'FAILED')),
  created_by TEXT NOT NULL DEFAULT '',
  reviewed_by TEXT NOT NULL DEFAULT '',
  reviewed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_billing_credit_notes_refund
  ON xz_billing_credit_notes(refund_record_id)
  WHERE refund_record_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS xz_billing_payment_requests (
  id TEXT PRIMARY KEY,
  request_no TEXT NOT NULL UNIQUE,
  invoice_id TEXT NOT NULL REFERENCES xz_billing_invoices(id),
  order_id TEXT NOT NULL REFERENCES xz_orders(id),
  order_no TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT 'WECHAT_VIRTUAL',
  amount_cents BIGINT NOT NULL CHECK (amount_cents >= 0),
  currency TEXT NOT NULL DEFAULT 'CNY',
  status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'SUCCEEDED', 'FAILED', 'CANCELLED', 'REFUNDED')),
  dunning_status TEXT NOT NULL DEFAULT 'NOT_STARTED' CHECK (dunning_status IN ('NOT_STARTED', 'IN_PROGRESS', 'RESOLVED', 'STOPPED')),
  dunning_attempts INT NOT NULL DEFAULT 0 CHECK (dunning_attempts >= 0),
  due_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  paid_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(order_id)
);

CREATE INDEX IF NOT EXISTS idx_xz_billing_payment_requests_dunning
  ON xz_billing_payment_requests(status, dunning_status, due_at, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_billing_dunning_events (
  id TEXT PRIMARY KEY,
  payment_request_id TEXT NOT NULL REFERENCES xz_billing_payment_requests(id),
  action TEXT NOT NULL CHECK (action IN ('REMINDER_RECORDED', 'MANUAL_CONTACT', 'STOP_DUNNING')),
  channel TEXT NOT NULL DEFAULT 'MANUAL' CHECK (channel IN ('MANUAL', 'IN_APP', 'SMS', 'EMAIL')),
  status TEXT NOT NULL DEFAULT 'RECORDED' CHECK (status IN ('RECORDED', 'SENT', 'FAILED')),
  actor_id TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Backfill only from real persisted WeChat virtual-payment orders.
INSERT INTO xz_billing_invoices(
  id, invoice_no, tenant_id, user_id, order_id, order_no, subtotal_cents,
  total_cents, paid_cents, status, payment_status, due_at, created_at, updated_at
)
SELECT
  'inv_' || substr(md5(o.id), 1, 20),
  'INV-' || upper(substr(md5(o.id), 1, 16)),
  coalesce(nullif(o.tenant_id, ''), 'personal:' || o.user_id),
  o.user_id, o.id, coalesce(nullif(o.order_no, ''), o.id), o.amount_cents,
  o.amount_cents,
  CASE WHEN upper(coalesce(o.status, '')) IN ('PAID', 'SUCCEEDED') THEN o.amount_cents ELSE 0 END,
  CASE WHEN upper(coalesce(o.status, '')) IN ('PAID', 'SUCCEEDED') THEN 'PAID'
       WHEN upper(coalesce(o.status, '')) = 'REFUNDED' THEN 'CREDITED' ELSE 'FINALIZED' END,
  CASE WHEN upper(coalesce(o.status, '')) IN ('PAID', 'SUCCEEDED') THEN 'PAID'
       WHEN upper(coalesce(o.status, '')) = 'REFUNDED' THEN 'REFUNDED' ELSE 'PENDING' END,
  coalesce(o.payment_expires_at, now() + interval '30 minutes'),
  CASE WHEN coalesce(o.created_at, '') ~ '^2[0-9]{3}-' THEN o.created_at::timestamptz ELSE now() END,
  coalesce(o.updated_at, now())
FROM xz_orders o
WHERE o.payment_channel = 'WECHAT_VIRTUAL'
ON CONFLICT (order_id) DO NOTHING;

INSERT INTO xz_billing_payment_requests(
  id, request_no, invoice_id, order_id, order_no, tenant_id, user_id,
  amount_cents, status, dunning_status, due_at, expires_at, paid_at, created_at, updated_at
)
SELECT
  'pr_' || substr(md5(o.id), 1, 20),
  'PR-' || upper(substr(md5(o.id), 1, 16)),
  i.id, o.id, i.order_no, i.tenant_id, i.user_id, i.total_cents,
  CASE WHEN upper(coalesce(o.status, '')) IN ('PAID', 'SUCCEEDED') THEN 'SUCCEEDED'
       WHEN upper(coalesce(o.status, '')) = 'REFUNDED' THEN 'REFUNDED'
       WHEN upper(coalesce(o.status, '')) = 'CLOSED' THEN 'CANCELLED' ELSE 'PENDING' END,
  CASE WHEN upper(coalesce(o.status, '')) IN ('PAID', 'SUCCEEDED') THEN 'RESOLVED'
       WHEN upper(coalesce(o.status, '')) IN ('REFUNDED', 'CLOSED') THEN 'STOPPED' ELSE 'NOT_STARTED' END,
  i.due_at, o.payment_expires_at,
  CASE WHEN coalesce(o.paid_at, '') ~ '^2[0-9]{3}-' THEN o.paid_at::timestamptz ELSE NULL END,
  i.created_at, coalesce(o.updated_at, now())
FROM xz_orders o
JOIN xz_billing_invoices i ON i.order_id = o.id
WHERE o.payment_channel = 'WECHAT_VIRTUAL'
ON CONFLICT (order_id) DO NOTHING;

INSERT INTO xz_billing_subscriptions(
  id, tenant_id, user_id, plan_id, product_code, source_order_id, source_order_no,
  status, starts_at, ends_at, entitlement_snapshot, created_at, updated_at
)
SELECT
  'sub_' || substr(md5(o.id), 1, 20), r.tenant_id, r.user_id, o.plan_id,
  coalesce(o.product_code, ''), o.id, r.source_order_no,
  CASE WHEN r.expires_at > now() THEN 'ACTIVE' ELSE 'EXPIRED' END,
  r.effective_at, r.expires_at, r.metadata, r.created_at, now()
FROM xz_membership_entitlement_records r
JOIN xz_orders o ON coalesce(nullif(o.order_no, ''), o.id) = r.source_order_no
ON CONFLICT (source_order_id) DO NOTHING;

INSERT INTO xz_billing_credit_notes(
  id, credit_note_no, invoice_id, order_id, order_no, refund_record_id,
  amount_cents, reason, status, refund_status, created_at, updated_at
)
SELECT
  'cn_' || substr(md5(r.id), 1, 20),
  'CN-' || upper(substr(md5(r.id), 1, 16)),
  i.id, i.order_id, i.order_no, r.id,
  greatest(1, coalesce(r.amount_cents, i.total_cents)),
  '微信虚拟支付退款通知',
  CASE WHEN upper(r.status) = 'REFUNDED' THEN 'FINALIZED' ELSE 'PENDING_REVIEW' END,
  CASE WHEN upper(r.status) = 'REFUNDED' THEN 'SUCCEEDED'
       WHEN upper(r.status) LIKE '%FAILED%' THEN 'FAILED' ELSE 'PENDING' END,
  r.created_at, r.updated_at
FROM xz_refund_records r
JOIN xz_billing_invoices i ON i.order_id = r.order_id
ON CONFLICT (refund_record_id) WHERE refund_record_id IS NOT NULL DO NOTHING;

COMMIT;
