-- WeChat Mini Program virtual payment, immutable order snapshots and idempotent entitlement ledgers.
-- Reuses xz_plans, xz_orders, xz_point_accounts, xz_user_wallets, xz_token_records and xz_payment_events.

BEGIN;

CREATE TABLE IF NOT EXISTS xz_billing_config (
  config_key TEXT PRIMARY KEY,
  integer_value BIGINT NOT NULL CHECK (integer_value > 0),
  description TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO xz_billing_config(config_key, integer_value, description)
VALUES ('CREDITS_PER_CNY_YUAN', 100, 'AI creation credits granted per CNY yuan')
ON CONFLICT (config_key) DO NOTHING;

ALTER TABLE xz_plans
  ADD COLUMN IF NOT EXISTS payment_product_code TEXT,
  ADD COLUMN IF NOT EXISTS product_type TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_plans_payment_product_code
  ON xz_plans(payment_product_code)
  WHERE payment_product_code IS NOT NULL AND payment_product_code <> '';

INSERT INTO xz_plans(
  id, code, name, plan_type, product_type, payment_product_code,
  price_cents, grant_points, token_amount, token_rights_value_cents,
  member_level, duration_days, concurrency, active, entitlements, raw
)
VALUES (
  'plan_image_pack_1000', 'image_pack_1000', '1000张图片生成额度',
  'IMAGE_QUOTA_PACK', 'IMAGE_QUOTA_PACK', 'IMAGE_PACK_1000',
  8000, 0, 0, 0, NULL, 0, 0, TRUE,
  '{"productType":"IMAGE_QUOTA_PACK","imageQuota":1000,"validity":"PERMANENT","withdrawable":false,"transferable":false,"displayPrice":"80 元"}'::jsonb,
  '{"id":"plan_image_pack_1000","code":"image_pack_1000","name":"1000张图片生成额度","planType":"IMAGE_QUOTA_PACK","productType":"IMAGE_QUOTA_PACK","paymentProductCode":"IMAGE_PACK_1000","priceCents":8000,"imageQuota":1000,"active":true}'::jsonb
)
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  plan_type = EXCLUDED.plan_type,
  product_type = EXCLUDED.product_type,
  payment_product_code = EXCLUDED.payment_product_code,
  price_cents = EXCLUDED.price_cents,
  active = EXCLUDED.active,
  entitlements = EXCLUDED.entitlements,
  raw = EXCLUDED.raw;

UPDATE xz_plans
SET name = '知启云AI年度会员',
    payment_product_code = 'MEMBER_YEAR_996',
    product_type = 'MEMBER_PACKAGE',
    price_cents = 99600,
    grant_points = 400 * (SELECT integer_value FROM xz_billing_config WHERE config_key = 'CREDITS_PER_CNY_YUAN'),
    token_amount = 400 * (SELECT integer_value FROM xz_billing_config WHERE config_key = 'CREDITS_PER_CNY_YUAN'),
    token_rights_value_cents = 40000,
    member_level = 'PRO',
    duration_days = 365,
    active = TRUE,
    entitlements = coalesce(entitlements, '{}'::jsonb) || jsonb_build_object(
      'productType', 'MEMBER_PACKAGE',
      'memberLevel', 'PRO',
      'memberDays', 365,
      'bonusCreditCny', 400,
      'creditUnits', 400 * (SELECT integer_value FROM xz_billing_config WHERE config_key = 'CREDITS_PER_CNY_YUAN'),
      'creditsPerYuanConfig', 'CREDITS_PER_CNY_YUAN',
      'withdrawable', false,
      'transferable', false,
      'displayPrice', '996 元'
    ),
    raw = coalesce(raw, '{}'::jsonb) || jsonb_build_object(
      'name', '知启云AI年度会员',
      'paymentProductCode', 'MEMBER_YEAR_996',
      'productType', 'MEMBER_PACKAGE',
      'priceCents', 99600,
      'grantPoints', 400 * (SELECT integer_value FROM xz_billing_config WHERE config_key = 'CREDITS_PER_CNY_YUAN'),
      'memberLevel', 'PRO',
      'durationDays', 365,
      'active', true
    )
WHERE id = 'plan_ai_creator_996';

CREATE TABLE IF NOT EXISTS xz_wechat_virtual_product_mappings (
  id TEXT PRIMARY KEY,
  plan_id TEXT NOT NULL REFERENCES xz_plans(id),
  offer_id TEXT NOT NULL DEFAULT '',
  wechat_product_id TEXT NOT NULL,
  mode TEXT NOT NULL DEFAULT 'short_series_goods',
  env SMALLINT NOT NULL CHECK (env IN (0, 1)),
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(plan_id, env)
);

INSERT INTO xz_wechat_virtual_product_mappings(id, plan_id, wechat_product_id, mode, env, enabled)
VALUES
  ('wxvp_image_pack_1000_prod', 'plan_image_pack_1000', 'IMAGE_PACK_1000', 'short_series_goods', 0, TRUE),
  ('wxvp_image_pack_1000_sandbox', 'plan_image_pack_1000', 'IMAGE_PACK_1000', 'short_series_goods', 1, TRUE),
  ('wxvp_member_year_996_prod', 'plan_ai_creator_996', 'MEMBER_YEAR_996', 'short_series_goods', 0, TRUE),
  ('wxvp_member_year_996_sandbox', 'plan_ai_creator_996', 'MEMBER_YEAR_996', 'short_series_goods', 1, TRUE)
ON CONFLICT (plan_id, env) DO NOTHING;

ALTER TABLE xz_orders
  ADD COLUMN IF NOT EXISTS product_code TEXT,
  ADD COLUMN IF NOT EXISTS product_name TEXT,
  ADD COLUMN IF NOT EXISTS product_type TEXT,
  ADD COLUMN IF NOT EXISTS payment_channel TEXT,
  ADD COLUMN IF NOT EXISTS payment_scene TEXT,
  ADD COLUMN IF NOT EXISTS payment_mode TEXT,
  ADD COLUMN IF NOT EXISTS wechat_order_id TEXT,
  ADD COLUMN IF NOT EXISTS wechat_transaction_id TEXT,
  ADD COLUMN IF NOT EXISTS wechat_openid_hash TEXT,
  ADD COLUMN IF NOT EXISTS entitlement_status TEXT NOT NULL DEFAULT 'PENDING',
  ADD COLUMN IF NOT EXISTS entitlement_error TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS entitlement_started_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS entitlement_granted_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS payment_expires_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS compensation_locked_until TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_orders_order_no
  ON xz_orders(order_no)
  WHERE order_no IS NOT NULL AND order_no <> '';
CREATE INDEX IF NOT EXISTS idx_xz_orders_virtual_compensation
  ON xz_orders(status, entitlement_status, payment_expires_at, compensation_locked_until)
  WHERE payment_channel = 'WECHAT_VIRTUAL';

CREATE TABLE IF NOT EXISTS xz_payment_records (
  id TEXT PRIMARY KEY,
  payment_no TEXT NOT NULL UNIQUE,
  order_id TEXT NOT NULL REFERENCES xz_orders(id),
  order_no TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  payment_channel TEXT NOT NULL,
  payment_scene TEXT NOT NULL,
  amount_cents BIGINT NOT NULL CHECK (amount_cents >= 0),
  prepay_status TEXT NOT NULL DEFAULT 'CREATED',
  wechat_order_id TEXT,
  wechat_transaction_id TEXT,
  request_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  response_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  callback_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  failure_reason TEXT NOT NULL DEFAULT '',
  paid_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(order_id, payment_channel)
);

CREATE INDEX IF NOT EXISTS idx_xz_payment_records_order
  ON xz_payment_records(order_no, created_at DESC);

ALTER TABLE xz_token_records
  ADD COLUMN IF NOT EXISTS tenant_id TEXT,
  ADD COLUMN IF NOT EXISTS balance_before BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS idempotency_key TEXT,
  ADD COLUMN IF NOT EXISTS source_order_no TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_token_records_idempotency
  ON xz_token_records(idempotency_key)
  WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';

CREATE TABLE IF NOT EXISTS xz_image_quota_accounts (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  remaining_images BIGINT NOT NULL DEFAULT 0 CHECK (remaining_images >= 0),
  total_granted BIGINT NOT NULL DEFAULT 0 CHECK (total_granted >= 0),
  total_used BIGINT NOT NULL DEFAULT 0 CHECK (total_used >= 0),
  version BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(tenant_id, user_id)
);

CREATE TABLE IF NOT EXISTS xz_image_quota_ledger (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  account_id TEXT NOT NULL REFERENCES xz_image_quota_accounts(id),
  image_delta BIGINT NOT NULL CHECK (image_delta <> 0),
  balance_before BIGINT NOT NULL CHECK (balance_before >= 0),
  balance_after BIGINT NOT NULL CHECK (balance_after >= 0),
  source_order_no TEXT NOT NULL,
  business_type TEXT NOT NULL,
  idempotency_key TEXT NOT NULL UNIQUE,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (balance_after = balance_before + image_delta)
);

CREATE TABLE IF NOT EXISTS xz_membership_entitlement_records (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  member_level TEXT NOT NULL,
  effective_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  source_order_no TEXT NOT NULL,
  idempotency_key TEXT NOT NULL UNIQUE,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (expires_at > effective_at)
);

ALTER TABLE xz_payment_events
  ADD COLUMN IF NOT EXISTS event_type TEXT,
  ADD COLUMN IF NOT EXISTS raw_body TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS processing_status TEXT NOT NULL DEFAULT 'RECEIVED',
  ADD COLUMN IF NOT EXISTS process_attempts INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS error_message TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS xz_refund_records (
  id TEXT PRIMARY KEY,
  order_id TEXT NOT NULL REFERENCES xz_orders(id),
  order_no TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  provider TEXT NOT NULL,
  provider_refund_id TEXT,
  amount_cents BIGINT,
  status TEXT NOT NULL DEFAULT 'PENDING_REVIEW',
  idempotency_key TEXT NOT NULL UNIQUE,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMIT;
