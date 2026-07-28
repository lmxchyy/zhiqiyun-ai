-- Phase 2A: data-model governance for member/agent V2 price-plan administration.
-- Additive only: no legacy plan rename/backfill, no historical order rewrite,
-- and no automatic publication or synchronization with WeChat.

BEGIN;

ALTER TABLE xz_plan_versions
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS created_by TEXT,
  ADD COLUMN IF NOT EXISTS updated_by TEXT,
  ADD COLUMN IF NOT EXISTS activated_by TEXT,
  ADD COLUMN IF NOT EXISTS activated_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS retired_by TEXT,
  ADD COLUMN IF NOT EXISTS retired_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS change_reason TEXT NOT NULL DEFAULT '';

ALTER TABLE xz_price_plans
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS created_by TEXT,
  ADD COLUMN IF NOT EXISTS updated_by TEXT,
  ADD COLUMN IF NOT EXISTS enabled_by TEXT,
  ADD COLUMN IF NOT EXISTS enabled_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS disabled_by TEXT,
  ADD COLUMN IF NOT EXISTS disabled_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS change_reason TEXT NOT NULL DEFAULT '';

ALTER TABLE xz_wechat_virtual_goods
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS created_by TEXT,
  ADD COLUMN IF NOT EXISTS updated_by TEXT,
  ADD COLUMN IF NOT EXISTS verification_status TEXT NOT NULL DEFAULT 'UNCONFIRMED',
  ADD COLUMN IF NOT EXISTS verified_by TEXT,
  ADD COLUMN IF NOT EXISTS verified_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS verification_reason TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS verification_evidence TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS verification_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS verification_expires_at TIMESTAMPTZ;

ALTER TABLE xz_price_plan_payment_bindings
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS created_by TEXT,
  ADD COLUMN IF NOT EXISTS updated_by TEXT,
  ADD COLUMN IF NOT EXISTS enabled_by TEXT,
  ADD COLUMN IF NOT EXISTS enabled_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS disabled_by TEXT,
  ADD COLUMN IF NOT EXISTS disabled_at TIMESTAMPTZ;

ALTER TABLE xz_price_plan_user_whitelist
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS created_by TEXT,
  ADD COLUMN IF NOT EXISTS updated_by TEXT,
  ADD COLUMN IF NOT EXISTS disabled_by TEXT,
  ADD COLUMN IF NOT EXISTS disabled_at TIMESTAMPTZ;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'ck_xz_wechat_goods_verification_status_098'
      AND conrelid = 'xz_wechat_virtual_goods'::regclass
  ) THEN
    ALTER TABLE xz_wechat_virtual_goods
      ADD CONSTRAINT ck_xz_wechat_goods_verification_status_098
      CHECK (verification_status IN (
        'UNCONFIRMED',
        'MANUALLY_CONFIRMED_PUBLISHED',
        'PRICE_MISMATCH',
        'VERIFICATION_EXPIRED',
        'DISABLED'
      ));
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'ck_xz_wechat_goods_manual_confirmation_098'
      AND conrelid = 'xz_wechat_virtual_goods'::regclass
  ) THEN
    ALTER TABLE xz_wechat_virtual_goods
      ADD CONSTRAINT ck_xz_wechat_goods_manual_confirmation_098
      CHECK (
        verification_status <> 'MANUALLY_CONFIRMED_PUBLISHED'
        OR (
          NULLIF(BTRIM(verified_by), '') IS NOT NULL
          AND verified_at IS NOT NULL
          AND NULLIF(BTRIM(verification_reason), '') IS NOT NULL
          AND verification_snapshot @> jsonb_build_object(
            'productId', product_id,
            'offerId', offer_id,
            'environment', environment,
            'platformPriceCents', platform_price_cents
          )
        )
      );
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'ck_xz_wechat_goods_verification_expiry_098'
      AND conrelid = 'xz_wechat_virtual_goods'::regclass
  ) THEN
    ALTER TABLE xz_wechat_virtual_goods
      ADD CONSTRAINT ck_xz_wechat_goods_verification_expiry_098
      CHECK (verification_expires_at IS NULL OR verified_at IS NULL OR verification_expires_at > verified_at);
  END IF;
END $$;

COMMENT ON COLUMN xz_wechat_virtual_goods.verification_status IS
  'Local manual verification state only; it is not a real-time result from the WeChat platform.';
COMMENT ON COLUMN xz_wechat_virtual_goods.verification_evidence IS
  'Optional local reference to an operator-supplied WeChat screenshot or work-order number.';
COMMENT ON COLUMN xz_wechat_virtual_goods.verification_snapshot IS
  'Immutable-at-confirmation local snapshot of productId, offerId, environment, and platformPriceCents.';

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_plans_code_098
  ON xz_plans(code)
  WHERE NULLIF(BTRIM(code), '') IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_plan_versions_one_active_098
  ON xz_plan_versions(plan_id)
  WHERE status = 'ACTIVE';

CREATE INDEX IF NOT EXISTS idx_xz_plan_versions_admin_098
  ON xz_plan_versions(plan_id, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_price_plans_admin_098
  ON xz_price_plans(plan_id, channel, environment, status, enabled, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_wechat_goods_verification_098
  ON xz_wechat_virtual_goods(environment, verification_status, enabled, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_price_plan_bindings_admin_098
  ON xz_price_plan_payment_bindings(price_plan_id, status, enabled, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_price_plan_whitelist_admin_098
  ON xz_price_plan_user_whitelist(price_plan_id, enabled, updated_at DESC);

CREATE OR REPLACE FUNCTION xz_enforce_plan_code_governance_098()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'UPDATE' THEN
    IF NEW.id IS DISTINCT FROM OLD.id OR NEW.code IS DISTINCT FROM OLD.code THEN
      RAISE EXCEPTION USING
        ERRCODE = '23514',
        MESSAGE = 'PLAN_CODE_IMMUTABLE';
    END IF;
    RETURN NEW;
  END IF;

  IF NULLIF(BTRIM(NEW.code), '') IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'PLAN_CODE_REQUIRED';
  END IF;

  IF LENGTH(NEW.code) < 3
     OR LENGTH(NEW.code) > 64
     OR NEW.code !~ '^[a-z][a-z0-9_]*[a-z0-9]$' THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'PLAN_CODE_FORMAT_INVALID';
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_xz_plans_code_governance_098 ON xz_plans;
CREATE TRIGGER trg_xz_plans_code_governance_098
BEFORE INSERT OR UPDATE OF id, code ON xz_plans
FOR EACH ROW EXECUTE FUNCTION xz_enforce_plan_code_governance_098();

CREATE OR REPLACE FUNCTION xz_guard_plan_version_098()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  core_changed BOOLEAN;
BEGIN
  IF TG_OP = 'DELETE' THEN
    IF OLD.status <> 'DRAFT' THEN
      RAISE EXCEPTION USING
        ERRCODE = '23514',
        MESSAGE = 'PLAN_VERSION_PUBLISHED_DELETE_FORBIDDEN';
    END IF;
    RETURN OLD;
  END IF;

  IF NEW.id IS DISTINCT FROM OLD.id
     OR NEW.plan_id IS DISTINCT FROM OLD.plan_id
     OR NEW.version_no IS DISTINCT FROM OLD.version_no THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'PLAN_VERSION_IDENTITY_IMMUTABLE';
  END IF;

  core_changed :=
       NEW.business_type IS DISTINCT FROM OLD.business_type
    OR NEW.rights_snapshot IS DISTINCT FROM OLD.rights_snapshot
    OR NEW.member_level IS DISTINCT FROM OLD.member_level
    OR NEW.agent_level IS DISTINCT FROM OLD.agent_level
    OR NEW.token_amount IS DISTINCT FROM OLD.token_amount
    OR NEW.points_amount IS DISTINCT FROM OLD.points_amount
    OR NEW.duration_days IS DISTINCT FROM OLD.duration_days
    OR NEW.commission_rule_version IS DISTINCT FROM OLD.commission_rule_version
    OR NEW.commission_snapshot IS DISTINCT FROM OLD.commission_snapshot
    OR NEW.effective_at IS DISTINCT FROM OLD.effective_at
    OR NEW.expires_at IS DISTINCT FROM OLD.expires_at;

  IF OLD.status = 'RETIRED' AND (
    core_changed OR NEW.status IS DISTINCT FROM OLD.status
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'PLAN_VERSION_RETIRED_IMMUTABLE';
  END IF;

  IF OLD.status = 'ACTIVE' AND core_changed THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'PLAN_VERSION_ACTIVE_RIGHTS_IMMUTABLE';
  END IF;

  IF OLD.status = 'ACTIVE' AND NEW.status NOT IN ('ACTIVE', 'RETIRED') THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'PLAN_VERSION_STATUS_REGRESSION_FORBIDDEN';
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_xz_plan_versions_guard_098 ON xz_plan_versions;
CREATE TRIGGER trg_xz_plan_versions_guard_098
BEFORE UPDATE OR DELETE ON xz_plan_versions
FOR EACH ROW EXECUTE FUNCTION xz_guard_plan_version_098();

CREATE OR REPLACE FUNCTION xz_guard_price_plan_098()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  economic_changed BOOLEAN;
  has_order BOOLEAN;
BEGIN
  IF TG_OP = 'DELETE' THEN
    SELECT EXISTS (
      SELECT 1 FROM xz_orders orders
      WHERE orders.price_plan_id = OLD.id
    ) INTO has_order;
    IF has_order THEN
      RAISE EXCEPTION USING
        ERRCODE = '23514',
        MESSAGE = 'PRICE_PLAN_REFERENCED_DELETE_FORBIDDEN';
    END IF;
    RETURN OLD;
  END IF;

  IF NEW.id IS DISTINCT FROM OLD.id OR NEW.code IS DISTINCT FROM OLD.code THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'PRICE_PLAN_IDENTITY_IMMUTABLE';
  END IF;

  economic_changed :=
       NEW.plan_id IS DISTINCT FROM OLD.plan_id
    OR NEW.plan_version_id IS DISTINCT FROM OLD.plan_version_id
    OR NEW.price_type IS DISTINCT FROM OLD.price_type
    OR NEW.channel IS DISTINCT FROM OLD.channel
    OR NEW.environment IS DISTINCT FROM OLD.environment
    OR NEW.sale_price_cents IS DISTINCT FROM OLD.sale_price_cents
    OR NEW.original_price_cents IS DISTINCT FROM OLD.original_price_cents
    OR NEW.bonus_points IS DISTINCT FROM OLD.bonus_points
    OR NEW.bonus_tokens IS DISTINCT FROM OLD.bonus_tokens
    OR NEW.audience_rule IS DISTINCT FROM OLD.audience_rule
    OR NEW.is_visible IS DISTINCT FROM OLD.is_visible
    OR NEW.effective_at IS DISTINCT FROM OLD.effective_at
    OR NEW.expires_at IS DISTINCT FROM OLD.expires_at;

  IF economic_changed THEN
    SELECT EXISTS (
      SELECT 1 FROM xz_orders orders
      WHERE orders.price_plan_id = OLD.id
    ) INTO has_order;
    IF OLD.status <> 'DRAFT' OR has_order THEN
      RAISE EXCEPTION USING
        ERRCODE = '23514',
        MESSAGE = 'PRICE_PLAN_CLONE_REQUIRED';
    END IF;
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_xz_price_plans_guard_098 ON xz_price_plans;
CREATE TRIGGER trg_xz_price_plans_guard_098
BEFORE UPDATE OR DELETE ON xz_price_plans
FOR EACH ROW EXECUTE FUNCTION xz_guard_price_plan_098();

CREATE OR REPLACE FUNCTION xz_touch_price_plan_admin_record_098()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
  NEW.updated_at := now();
  NEW.revision := OLD.revision + 1;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_xz_plan_versions_touch_098 ON xz_plan_versions;
CREATE TRIGGER trg_xz_plan_versions_touch_098
BEFORE UPDATE ON xz_plan_versions
FOR EACH ROW EXECUTE FUNCTION xz_touch_price_plan_admin_record_098();

DROP TRIGGER IF EXISTS trg_xz_price_plans_touch_098 ON xz_price_plans;
CREATE TRIGGER trg_xz_price_plans_touch_098
BEFORE UPDATE ON xz_price_plans
FOR EACH ROW EXECUTE FUNCTION xz_touch_price_plan_admin_record_098();

DROP TRIGGER IF EXISTS trg_xz_wechat_goods_touch_098 ON xz_wechat_virtual_goods;
CREATE TRIGGER trg_xz_wechat_goods_touch_098
BEFORE UPDATE ON xz_wechat_virtual_goods
FOR EACH ROW EXECUTE FUNCTION xz_touch_price_plan_admin_record_098();

DROP TRIGGER IF EXISTS trg_xz_price_plan_bindings_touch_098 ON xz_price_plan_payment_bindings;
CREATE TRIGGER trg_xz_price_plan_bindings_touch_098
BEFORE UPDATE ON xz_price_plan_payment_bindings
FOR EACH ROW EXECUTE FUNCTION xz_touch_price_plan_admin_record_098();

DROP TRIGGER IF EXISTS trg_xz_price_plan_whitelist_touch_098 ON xz_price_plan_user_whitelist;
CREATE TRIGGER trg_xz_price_plan_whitelist_touch_098
BEFORE UPDATE ON xz_price_plan_user_whitelist
FOR EACH ROW EXECUTE FUNCTION xz_touch_price_plan_admin_record_098();

INSERT INTO xz_role_permissions(role, permission)
VALUES
  ('SUPER_ADMIN', 'pricing:plan:view'),
  ('SUPER_ADMIN', 'pricing:entitlement:manage'),
  ('SUPER_ADMIN', 'pricing:price-plan:manage'),
  ('SUPER_ADMIN', 'pricing:price-plan:default'),
  ('SUPER_ADMIN', 'pricing:wechat-good:manage'),
  ('SUPER_ADMIN', 'pricing:test-whitelist:manage'),
  ('SUPER_ADMIN', 'pricing:audit:view')
ON CONFLICT (role, permission) DO NOTHING;

COMMIT;
