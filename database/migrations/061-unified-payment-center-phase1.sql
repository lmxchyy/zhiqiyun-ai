-- Unified payment center phase 1.
-- PostgreSQL is the project source of truth. Existing order, payment, wallet,
-- token and commission tables are extended instead of creating parallel ledgers.

BEGIN;

CREATE TABLE IF NOT EXISTS xz_payment_products (
  id TEXT PRIMARY KEY,
  source_plan_id TEXT REFERENCES xz_plans(id),
  product_code TEXT NOT NULL UNIQUE,
  product_name TEXT NOT NULL,
  product_type TEXT NOT NULL,
  fulfillment_type TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
  fulfillment_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS xz_product_prices (
  id TEXT PRIMARY KEY,
  product_id TEXT NOT NULL REFERENCES xz_payment_products(id),
  channel TEXT NOT NULL,
  platform TEXT NOT NULL,
  currency CHAR(3) NOT NULL DEFAULT 'CNY',
  amount BIGINT NOT NULL CHECK (amount >= 0),
  external_product_id TEXT,
  status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(product_id, channel, platform)
);

ALTER TABLE xz_orders
  ADD COLUMN IF NOT EXISTS product_id TEXT REFERENCES xz_payment_products(id),
  ADD COLUMN IF NOT EXISTS quantity BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS currency CHAR(3) NOT NULL DEFAULT 'CNY',
  ADD COLUMN IF NOT EXISTS original_amount_cents BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS discount_amount_cents BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS payable_amount_cents BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS platform TEXT,
  ADD COLUMN IF NOT EXISTS channel TEXT,
  ADD COLUMN IF NOT EXISTS order_status TEXT,
  ADD COLUMN IF NOT EXISTS idempotency_key TEXT,
  ADD COLUMN IF NOT EXISTS client_ip TEXT,
  ADD COLUMN IF NOT EXISTS expired_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS closed_at TIMESTAMPTZ;

UPDATE xz_orders
SET original_amount_cents = CASE WHEN original_amount_cents = 0 THEN amount_cents ELSE original_amount_cents END,
    payable_amount_cents = CASE WHEN payable_amount_cents = 0 THEN amount_cents ELSE payable_amount_cents END,
    order_status = coalesce(nullif(order_status, ''), status),
    channel = coalesce(nullif(channel, ''), payment_channel)
WHERE original_amount_cents = 0 OR payable_amount_cents = 0 OR order_status IS NULL OR channel IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_orders_user_idempotency
  ON xz_orders(user_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';

ALTER TABLE xz_payment_records
  ADD COLUMN IF NOT EXISTS provider TEXT,
  ADD COLUMN IF NOT EXISTS provider_trade_no TEXT,
  ADD COLUMN IF NOT EXISTS provider_transaction_id TEXT,
  ADD COLUMN IF NOT EXISTS currency CHAR(3) NOT NULL DEFAULT 'CNY',
  ADD COLUMN IF NOT EXISTS payment_status TEXT,
  ADD COLUMN IF NOT EXISTS notify_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS failure_code TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS failure_message TEXT NOT NULL DEFAULT '';

UPDATE xz_payment_records
SET provider = coalesce(nullif(provider, ''), payment_channel),
    payment_status = coalesce(nullif(payment_status, ''), prepay_status),
    provider_transaction_id = coalesce(nullif(provider_transaction_id, ''), wechat_transaction_id),
    notify_payload = CASE WHEN notify_payload = '{}'::jsonb THEN callback_payload ELSE notify_payload END
WHERE provider IS NULL OR payment_status IS NULL OR provider_transaction_id IS NULL OR notify_payload = '{}'::jsonb;

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_payment_records_provider_transaction
  ON xz_payment_records(provider, provider_transaction_id)
  WHERE provider_transaction_id IS NOT NULL AND provider_transaction_id <> '';

CREATE TABLE IF NOT EXISTS xz_fulfillment_records (
  id TEXT PRIMARY KEY,
  order_no TEXT NOT NULL,
  user_id TEXT NOT NULL,
  fulfillment_type TEXT NOT NULL,
  fulfillment_status TEXT NOT NULL DEFAULT 'PENDING'
    CHECK (fulfillment_status IN ('PENDING', 'PROCESSING', 'SUCCESS', 'FAILED')),
  fulfillment_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  retry_count INT NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
  failure_message TEXT NOT NULL DEFAULT '',
  fulfilled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(order_no, fulfillment_type)
);

INSERT INTO xz_plans(
  id, code, name, plan_type, product_type, payment_product_code,
  price_cents, grant_points, token_amount, token_rights_value_cents,
  duration_days, concurrency, active, entitlements, raw
)
VALUES (
  'plan_token_1000', 'token_1000', '1000 Token', 'TOKEN_RECHARGE', 'TOKEN_ONLY', 'TOKEN_1000',
  5000, 1000, 1000, 5000, 0, 0, TRUE,
  '{"productType":"TOKEN_ONLY","fulfillmentType":"grant_token","tokenGrantAmount":1000,"nonWithdrawable":true,"nonTransferable":true}'::jsonb,
  '{"id":"plan_token_1000","code":"token_1000","name":"1000 Token","paymentProductCode":"TOKEN_1000","priceCents":5000,"grantPoints":1000,"active":true}'::jsonb
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO xz_payment_products(
  id, source_plan_id, product_code, product_name, product_type,
  fulfillment_type, description, status, fulfillment_payload
)
VALUES
  ('payment_product_token_1000', 'plan_token_1000', 'TOKEN_1000', '1000 Token',
   'token_package', 'grant_token', 'Mock payment token package', 'ACTIVE', '{"tokenAmount":1000}'::jsonb),
  ('payment_product_member_year', 'plan_ai_creator_996', 'MEMBER_YEAR', '知启云AI年度会员',
   'membership', 'grant_membership', 'Annual membership; fulfillment is reserved for phase 2', 'ACTIVE', '{"memberDays":365}'::jsonb)
ON CONFLICT (product_code) DO UPDATE SET
  source_plan_id = EXCLUDED.source_plan_id,
  product_name = EXCLUDED.product_name,
  product_type = EXCLUDED.product_type,
  fulfillment_type = EXCLUDED.fulfillment_type,
  description = EXCLUDED.description,
  fulfillment_payload = EXCLUDED.fulfillment_payload,
  updated_at = now();

INSERT INTO xz_product_prices(id, product_id, channel, platform, currency, amount, status)
VALUES
  ('payment_price_token_1000_mock_web', 'payment_product_token_1000', 'mock', 'web', 'CNY', 5000, 'ACTIVE'),
  ('payment_price_member_year_mock_web', 'payment_product_member_year', 'mock', 'web', 'CNY', 99600, 'ACTIVE')
ON CONFLICT (product_id, channel, platform) DO UPDATE SET
  currency = EXCLUDED.currency,
  amount = EXCLUDED.amount,
  status = EXCLUDED.status,
  updated_at = now();

COMMIT;
