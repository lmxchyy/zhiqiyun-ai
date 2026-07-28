-- Phase 2D: multi-price-plan administration and transaction-safe default switching.
-- Schema-only governance: no business-row backfill, price mutation, or historical order rewrite.

BEGIN;

ALTER TABLE xz_price_plans
  ADD COLUMN IF NOT EXISTS currency CHAR(3) NOT NULL DEFAULT 'CNY',
  ADD COLUMN IF NOT EXISTS audience_type TEXT NOT NULL DEFAULT 'PUBLIC';

ALTER TABLE xz_order_price_quotes
  ADD COLUMN IF NOT EXISTS currency CHAR(3) NOT NULL DEFAULT 'CNY',
  ADD COLUMN IF NOT EXISTS bonus_points BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS bonus_tokens BIGINT NOT NULL DEFAULT 0;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'ck_xz_price_plans_currency_099'
      AND conrelid = 'xz_price_plans'::regclass
  ) THEN
    ALTER TABLE xz_price_plans
      ADD CONSTRAINT ck_xz_price_plans_currency_099
      CHECK (currency ~ '^[A-Z]{3}$') NOT VALID;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'ck_xz_price_plans_audience_099'
      AND conrelid = 'xz_price_plans'::regclass
  ) THEN
    ALTER TABLE xz_price_plans
      ADD CONSTRAINT ck_xz_price_plans_audience_099
      CHECK (audience_type IN ('PUBLIC', 'RULE', 'WHITELIST', 'INVITE', 'TEST')) NOT VALID;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'ck_xz_price_plans_audience_rule_099'
      AND conrelid = 'xz_price_plans'::regclass
  ) THEN
    ALTER TABLE xz_price_plans
      ADD CONSTRAINT ck_xz_price_plans_audience_rule_099
      CHECK (jsonb_typeof(audience_rule) = 'object') NOT VALID;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'ck_xz_price_plans_code_format_099'
      AND conrelid = 'xz_price_plans'::regclass
  ) THEN
    ALTER TABLE xz_price_plans
      ADD CONSTRAINT ck_xz_price_plans_code_format_099
      CHECK (code ~ '^[a-z][a-z0-9_]{1,62}[a-z0-9]$') NOT VALID;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'ck_xz_price_plans_test_scope_099'
      AND conrelid = 'xz_price_plans'::regclass
  ) THEN
    ALTER TABLE xz_price_plans
      ADD CONSTRAINT ck_xz_price_plans_test_scope_099
      CHECK (
        price_type <> 'TEST'
        OR (
          is_default = FALSE
          AND is_visible = FALSE
          AND (audience_type <> 'PUBLIC' OR created_by IS NULL)
        )
      ) NOT VALID;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'ck_xz_price_plans_default_state_099'
      AND conrelid = 'xz_price_plans'::regclass
  ) THEN
    ALTER TABLE xz_price_plans
      ADD CONSTRAINT ck_xz_price_plans_default_state_099
      CHECK (
        is_default = FALSE
        OR (
          price_type <> 'TEST'
          AND enabled = TRUE
          AND status = 'ACTIVE'
          AND is_visible = TRUE
          AND audience_type = 'PUBLIC'
        )
      ) NOT VALID;
  END IF;
END $$;

COMMENT ON COLUMN xz_price_plans.audience_type IS
  'Audience selector. Legacy TEST rows created before price-plan administration may retain PUBLIC with created_by NULL; they remain hidden, non-default, whitelist-only compatibility rows.';

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_price_plans_default_currency_099
  ON xz_price_plans(plan_id, channel, environment, currency)
  WHERE is_default = TRUE;

CREATE INDEX IF NOT EXISTS idx_xz_price_plans_public_resolve_099
  ON xz_price_plans(plan_id, channel, environment, currency, updated_at DESC)
  WHERE is_default = TRUE AND enabled = TRUE AND status = 'ACTIVE';

CREATE INDEX IF NOT EXISTS idx_xz_order_price_quotes_price_plan_099
  ON xz_order_price_quotes(price_plan_id, created_at DESC);

CREATE OR REPLACE FUNCTION xz_guard_price_plan_099()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  economic_changed BOOLEAN;
  has_quote BOOLEAN;
  has_order BOOLEAN;
  has_active_binding BOOLEAN;
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'PRICE_PLAN_DELETE_FORBIDDEN';
  END IF;

  IF NEW.id IS DISTINCT FROM OLD.id OR NEW.code IS DISTINCT FROM OLD.code THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'PRICE_PLAN_IDENTITY_IMMUTABLE';
  END IF;

  IF OLD.status <> 'DRAFT' AND NEW.status = 'DRAFT' THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'PRICE_PLAN_STATUS_REGRESSION_FORBIDDEN';
  END IF;

  IF OLD.enabled_at IS NOT NULL AND NEW.enabled_at IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'PRICE_PLAN_ENABLED_HISTORY_IMMUTABLE';
  END IF;

  economic_changed :=
       NEW.plan_id IS DISTINCT FROM OLD.plan_id
    OR NEW.plan_version_id IS DISTINCT FROM OLD.plan_version_id
    OR NEW.price_type IS DISTINCT FROM OLD.price_type
    OR NEW.channel IS DISTINCT FROM OLD.channel
    OR NEW.environment IS DISTINCT FROM OLD.environment
    OR NEW.currency IS DISTINCT FROM OLD.currency
    OR NEW.sale_price_cents IS DISTINCT FROM OLD.sale_price_cents
    OR NEW.original_price_cents IS DISTINCT FROM OLD.original_price_cents
    OR NEW.bonus_points IS DISTINCT FROM OLD.bonus_points
    OR NEW.bonus_tokens IS DISTINCT FROM OLD.bonus_tokens
    OR NEW.audience_type IS DISTINCT FROM OLD.audience_type
    OR NEW.audience_rule IS DISTINCT FROM OLD.audience_rule
    OR NEW.is_visible IS DISTINCT FROM OLD.is_visible
    OR NEW.effective_at IS DISTINCT FROM OLD.effective_at
    OR NEW.expires_at IS DISTINCT FROM OLD.expires_at;

  IF economic_changed THEN
    SELECT EXISTS (
      SELECT 1 FROM xz_order_price_quotes quotes
      WHERE quotes.price_plan_id = OLD.id
    ) INTO has_quote;

    SELECT EXISTS (
      SELECT 1 FROM xz_orders orders
      WHERE orders.price_plan_id = OLD.id
    ) INTO has_order;

    SELECT EXISTS (
      SELECT 1 FROM xz_price_plan_payment_bindings bindings
      WHERE bindings.price_plan_id = OLD.id
        AND bindings.enabled = TRUE
        AND bindings.status = 'ACTIVE'
    ) INTO has_active_binding;

    IF OLD.status <> 'DRAFT'
       OR OLD.enabled = TRUE
       OR OLD.enabled_at IS NOT NULL
       OR has_quote
       OR has_order THEN
      RAISE EXCEPTION USING
        ERRCODE = '23514',
        MESSAGE = 'PRICE_PLAN_CLONE_REQUIRED';
    END IF;

    IF has_active_binding THEN
      RAISE EXCEPTION USING
        ERRCODE = '23514',
        MESSAGE = 'PRICE_PLAN_ACTIVE_BINDING_REQUIRES_DISABLE';
    END IF;
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_xz_price_plans_guard_098 ON xz_price_plans;
DROP TRIGGER IF EXISTS trg_xz_price_plans_guard_099 ON xz_price_plans;
CREATE TRIGGER trg_xz_price_plans_guard_099
BEFORE UPDATE OR DELETE ON xz_price_plans
FOR EACH ROW EXECUTE FUNCTION xz_guard_price_plan_099();

CREATE OR REPLACE FUNCTION xz_guard_order_v2_snapshot_099()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
  IF (NEW.snapshot_version = 2 AND OLD.snapshot_version IS DISTINCT FROM 2)
     OR (OLD.snapshot_version = 2 AND (
       NEW.id IS DISTINCT FROM OLD.id
    OR NEW.order_no IS DISTINCT FROM OLD.order_no
    OR NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
    OR NEW.user_id IS DISTINCT FROM OLD.user_id
    OR NEW.buyer_user_id IS DISTINCT FROM OLD.buyer_user_id
    OR NEW.plan_id IS DISTINCT FROM OLD.plan_id
    OR NEW.plan_version_id IS DISTINCT FROM OLD.plan_version_id
    OR NEW.price_plan_id IS DISTINCT FROM OLD.price_plan_id
    OR NEW.price_quote_id IS DISTINCT FROM OLD.price_quote_id
    OR NEW.snapshot_version IS DISTINCT FROM OLD.snapshot_version
    OR NEW.amount_cents IS DISTINCT FROM OLD.amount_cents
    OR NEW.quantity IS DISTINCT FROM OLD.quantity
    OR NEW.currency IS DISTINCT FROM OLD.currency
    OR NEW.transaction_price_cents IS DISTINCT FROM OLD.transaction_price_cents
    OR NEW.wechat_product_id_snapshot IS DISTINCT FROM OLD.wechat_product_id_snapshot
    OR NEW.wechat_goods_price_cents IS DISTINCT FROM OLD.wechat_goods_price_cents
    OR NEW.product_code IS DISTINCT FROM OLD.product_code
    OR NEW.product_name IS DISTINCT FROM OLD.product_name
    OR NEW.product_type IS DISTINCT FROM OLD.product_type
    OR NEW.payment_channel IS DISTINCT FROM OLD.payment_channel
    OR NEW.payment_scene IS DISTINCT FROM OLD.payment_scene
    OR NEW.payment_mode IS DISTINCT FROM OLD.payment_mode
    OR NEW.payment_environment IS DISTINCT FROM OLD.payment_environment
    OR NEW.wechat_openid_hash IS DISTINCT FROM OLD.wechat_openid_hash
    OR NEW.price_snapshot IS DISTINCT FROM OLD.price_snapshot
    OR NEW.rights_snapshot IS DISTINCT FROM OLD.rights_snapshot
    OR NEW.commission_rule_version_snapshot IS DISTINCT FROM OLD.commission_rule_version_snapshot
    OR NEW.commission_snapshot_v2 IS DISTINCT FROM OLD.commission_snapshot_v2
  )) THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'ORDER_V2_SNAPSHOT_IMMUTABLE';
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_xz_orders_v2_snapshot_guard_099 ON xz_orders;
CREATE TRIGGER trg_xz_orders_v2_snapshot_guard_099
BEFORE UPDATE ON xz_orders
FOR EACH ROW EXECUTE FUNCTION xz_guard_order_v2_snapshot_099();

COMMIT;
