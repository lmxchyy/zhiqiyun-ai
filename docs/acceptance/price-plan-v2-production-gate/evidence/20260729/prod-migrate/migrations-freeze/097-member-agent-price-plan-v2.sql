-- Phase 1: member/agent price-plan and immutable payment quote model.
-- Additive only: no legacy plan backfill and no historical order rewrite.

BEGIN;

CREATE TABLE IF NOT EXISTS xz_plan_versions (
  id TEXT PRIMARY KEY,
  plan_id TEXT NOT NULL REFERENCES xz_plans(id),
  version_no INTEGER NOT NULL CHECK (version_no > 0),
  business_type TEXT NOT NULL CHECK (business_type IN ('MEMBER', 'AGENT')),
  rights_snapshot JSONB NOT NULL,
  member_level TEXT,
  agent_level TEXT,
  token_amount BIGINT NOT NULL DEFAULT 0 CHECK (token_amount >= 0),
  points_amount BIGINT NOT NULL DEFAULT 0 CHECK (points_amount >= 0),
  duration_days INTEGER NOT NULL DEFAULT 0 CHECK (duration_days >= 0),
  commission_rule_version TEXT NOT NULL DEFAULT '',
  commission_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'ACTIVE', 'RETIRED')),
  effective_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(plan_id, version_no),
  CHECK (expires_at IS NULL OR effective_at IS NULL OR expires_at > effective_at),
  CHECK ((business_type = 'MEMBER' AND member_level IS NOT NULL) OR business_type <> 'MEMBER'),
  CHECK ((business_type = 'AGENT' AND agent_level IS NOT NULL) OR business_type <> 'AGENT')
);

CREATE INDEX IF NOT EXISTS idx_xz_plan_versions_effective_097
  ON xz_plan_versions(plan_id, status, effective_at, expires_at);

CREATE TABLE IF NOT EXISTS xz_price_plans (
  id TEXT PRIMARY KEY,
  plan_id TEXT NOT NULL REFERENCES xz_plans(id),
  plan_version_id TEXT NOT NULL REFERENCES xz_plan_versions(id),
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  price_type TEXT NOT NULL CHECK (price_type IN ('NORMAL', 'ACTIVITY', 'GRAY', 'TEST')),
  channel TEXT NOT NULL,
  environment TEXT NOT NULL CHECK (environment IN ('PRODUCTION', 'SANDBOX')),
  sale_price_cents BIGINT NOT NULL CHECK (sale_price_cents > 0),
  original_price_cents BIGINT NOT NULL CHECK (original_price_cents >= sale_price_cents),
  bonus_points BIGINT NOT NULL DEFAULT 0 CHECK (bonus_points >= 0),
  bonus_tokens BIGINT NOT NULL DEFAULT 0 CHECK (bonus_tokens >= 0),
  audience_rule JSONB NOT NULL DEFAULT '{}'::jsonb,
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  is_visible BOOLEAN NOT NULL DEFAULT TRUE,
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'ACTIVE', 'INACTIVE', 'EXPIRED')),
  effective_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (expires_at IS NULL OR effective_at IS NULL OR expires_at > effective_at),
  CHECK (price_type <> 'TEST' OR (is_default = FALSE AND is_visible = FALSE))
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_price_plans_default_097
  ON xz_price_plans(plan_id, channel, environment)
  WHERE is_default = TRUE;
CREATE INDEX IF NOT EXISTS idx_xz_price_plans_resolve_097
  ON xz_price_plans(plan_id, channel, environment, price_type, enabled, status, effective_at, expires_at);

CREATE TABLE IF NOT EXISTS xz_wechat_virtual_goods (
  id TEXT PRIMARY KEY,
  channel TEXT NOT NULL DEFAULT 'WECHAT_VIRTUAL',
  environment TEXT NOT NULL CHECK (environment IN ('PRODUCTION', 'SANDBOX')),
  offer_id TEXT NOT NULL,
  product_id TEXT NOT NULL,
  goods_name TEXT NOT NULL,
  platform_price_cents BIGINT NOT NULL CHECK (platform_price_cents > 0),
  mode TEXT NOT NULL DEFAULT 'short_series_goods',
  published BOOLEAN NOT NULL DEFAULT FALSE,
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'PUBLISHED', 'DISABLED')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(channel, environment, offer_id, product_id),
  UNIQUE(id, channel, environment)
);

CREATE INDEX IF NOT EXISTS idx_xz_wechat_virtual_goods_lookup_097
  ON xz_wechat_virtual_goods(channel, environment, product_id, enabled, published);

CREATE TABLE IF NOT EXISTS xz_price_plan_payment_bindings (
  id TEXT PRIMARY KEY,
  price_plan_id TEXT NOT NULL REFERENCES xz_price_plans(id),
  wechat_good_id TEXT NOT NULL,
  channel TEXT NOT NULL,
  environment TEXT NOT NULL CHECK (environment IN ('PRODUCTION', 'SANDBOX')),
  provider_price_snapshot_cents BIGINT NOT NULL CHECK (provider_price_snapshot_cents > 0),
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'ACTIVE', 'DISABLED')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(price_plan_id, channel, environment),
  FOREIGN KEY (wechat_good_id, channel, environment)
    REFERENCES xz_wechat_virtual_goods(id, channel, environment)
);

CREATE INDEX IF NOT EXISTS idx_xz_price_plan_bindings_good_097
  ON xz_price_plan_payment_bindings(wechat_good_id, enabled, status);

CREATE TABLE IF NOT EXISTS xz_price_plan_user_whitelist (
  id TEXT PRIMARY KEY,
  price_plan_id TEXT NOT NULL REFERENCES xz_price_plans(id),
  user_id TEXT NOT NULL REFERENCES xz_users(id),
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  effective_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(price_plan_id, user_id),
  CHECK (expires_at IS NULL OR effective_at IS NULL OR expires_at > effective_at)
);

CREATE INDEX IF NOT EXISTS idx_xz_price_plan_whitelist_user_097
  ON xz_price_plan_user_whitelist(user_id, enabled, effective_at, expires_at);

CREATE TABLE IF NOT EXISTS xz_order_price_quotes (
  id TEXT PRIMARY KEY,
  quote_token_hash TEXT NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL REFERENCES xz_users(id),
  plan_id TEXT NOT NULL REFERENCES xz_plans(id),
  plan_version_id TEXT NOT NULL REFERENCES xz_plan_versions(id),
  price_plan_id TEXT NOT NULL REFERENCES xz_price_plans(id),
  payment_binding_id TEXT NOT NULL REFERENCES xz_price_plan_payment_bindings(id),
  wechat_good_id TEXT NOT NULL REFERENCES xz_wechat_virtual_goods(id),
  entry_type TEXT NOT NULL CHECK (entry_type IN ('PUBLIC', 'TEST', 'LEGACY_PRODUCT_CODE')),
  transaction_price_cents BIGINT NOT NULL CHECK (transaction_price_cents > 0),
  provider_price_snapshot_cents BIGINT NOT NULL CHECK (provider_price_snapshot_cents > 0),
  wechat_goods_price_cents BIGINT NOT NULL CHECK (wechat_goods_price_cents > 0),
  channel TEXT NOT NULL,
  environment TEXT NOT NULL CHECK (environment IN ('PRODUCTION', 'SANDBOX')),
  offer_id TEXT NOT NULL,
  wechat_product_id TEXT NOT NULL,
  payment_mode TEXT NOT NULL,
  rights_snapshot JSONB NOT NULL,
  commission_rule_version TEXT NOT NULL DEFAULT '',
  commission_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  snapshot_version SMALLINT NOT NULL DEFAULT 2 CHECK (snapshot_version = 2),
  status TEXT NOT NULL DEFAULT 'AVAILABLE' CHECK (status IN ('AVAILABLE', 'CONSUMED', 'EXPIRED', 'CANCELLED')),
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  consumed_order_no TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (expires_at > created_at)
);

CREATE INDEX IF NOT EXISTS idx_xz_order_price_quotes_consume_097
  ON xz_order_price_quotes(quote_token_hash, user_id, status, expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_order_price_quotes_consumed_order_097
  ON xz_order_price_quotes(consumed_order_no)
  WHERE consumed_order_no IS NOT NULL;

ALTER TABLE xz_orders
  ADD COLUMN IF NOT EXISTS plan_version_id TEXT,
  ADD COLUMN IF NOT EXISTS price_plan_id TEXT,
  ADD COLUMN IF NOT EXISTS price_quote_id TEXT,
  ADD COLUMN IF NOT EXISTS snapshot_version SMALLINT,
  ADD COLUMN IF NOT EXISTS transaction_price_cents BIGINT,
  ADD COLUMN IF NOT EXISTS wechat_product_id_snapshot TEXT,
  ADD COLUMN IF NOT EXISTS wechat_goods_price_cents BIGINT,
  ADD COLUMN IF NOT EXISTS payment_environment TEXT,
  ADD COLUMN IF NOT EXISTS rights_snapshot JSONB,
  ADD COLUMN IF NOT EXISTS commission_rule_version_snapshot TEXT,
  ADD COLUMN IF NOT EXISTS commission_snapshot_v2 JSONB;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_xz_orders_plan_version_097') THEN
    ALTER TABLE xz_orders ADD CONSTRAINT fk_xz_orders_plan_version_097
      FOREIGN KEY (plan_version_id) REFERENCES xz_plan_versions(id) NOT VALID;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_xz_orders_price_plan_097') THEN
    ALTER TABLE xz_orders ADD CONSTRAINT fk_xz_orders_price_plan_097
      FOREIGN KEY (price_plan_id) REFERENCES xz_price_plans(id) NOT VALID;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_xz_orders_price_quote_097') THEN
    ALTER TABLE xz_orders ADD CONSTRAINT fk_xz_orders_price_quote_097
      FOREIGN KEY (price_quote_id) REFERENCES xz_order_price_quotes(id) NOT VALID;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ck_xz_orders_snapshot_v2_097') THEN
    ALTER TABLE xz_orders ADD CONSTRAINT ck_xz_orders_snapshot_v2_097 CHECK (
      snapshot_version IS NULL OR snapshot_version <> 2 OR (
        plan_version_id IS NOT NULL AND price_plan_id IS NOT NULL AND price_quote_id IS NOT NULL AND
        transaction_price_cents > 0 AND wechat_product_id_snapshot IS NOT NULL AND
        wechat_goods_price_cents = transaction_price_cents AND
        payment_environment IN ('PRODUCTION','SANDBOX') AND rights_snapshot IS NOT NULL
      )
    ) NOT VALID;
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_xz_orders_price_plan_097
  ON xz_orders(price_plan_id, created_at DESC)
  WHERE snapshot_version = 2;
CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_orders_price_quote_097
  ON xz_orders(price_quote_id)
  WHERE price_quote_id IS NOT NULL;

COMMIT;
