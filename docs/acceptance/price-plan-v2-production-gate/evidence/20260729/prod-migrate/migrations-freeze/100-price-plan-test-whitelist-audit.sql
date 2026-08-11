-- Phase 2E: TEST price-plan whitelist lifecycle, quote pin columns, and pricing audit governance.
-- Additive and compatibility preserving: no business rows are updated or deleted.

BEGIN;

ALTER TABLE xz_price_plan_user_whitelist
  ADD COLUMN IF NOT EXISTS lifecycle_status TEXT;

COMMENT ON COLUMN xz_price_plan_user_whitelist.lifecycle_status IS
  'Stored lifecycle for Phase 2E writes: ACTIVE, EXPIRED, or DISABLED. NULL is retained only for pre-100 rows and is interpreted through enabled.';

ALTER TABLE xz_order_price_quotes
  ADD COLUMN IF NOT EXISTS whitelist_entry_id TEXT,
  ADD COLUMN IF NOT EXISTS whitelist_revision BIGINT,
  ADD COLUMN IF NOT EXISTS whitelist_checked_at TIMESTAMPTZ;

-- A quote pin is either entirely absent (historical compatibility / non-TEST)
-- or complete. INSERT policy below distinguishes new TEST and non-TEST quotes.
ALTER TABLE xz_order_price_quotes
  DROP CONSTRAINT IF EXISTS ck_xz_order_price_quotes_whitelist_pin_100;

ALTER TABLE xz_order_price_quotes
  ADD CONSTRAINT ck_xz_order_price_quotes_whitelist_pin_100
  CHECK (
    (
      whitelist_entry_id IS NULL
      AND whitelist_revision IS NULL
      AND whitelist_checked_at IS NULL
    ) OR (
      NULLIF(BTRIM(whitelist_entry_id), '') IS NOT NULL
      AND whitelist_revision IS NOT NULL
      AND whitelist_revision > 0
      AND whitelist_checked_at IS NOT NULL
    )
  ) NOT VALID;

ALTER TABLE xz_audit_logs
  ADD COLUMN IF NOT EXISTS request_id TEXT,
  ADD COLUMN IF NOT EXISTS domain TEXT,
  ADD COLUMN IF NOT EXISTS result TEXT,
  ADD COLUMN IF NOT EXISTS error_code TEXT,
  ADD COLUMN IF NOT EXISTS change_reason TEXT,
  ADD COLUMN IF NOT EXISTS before_snapshot JSONB,
  ADD COLUMN IF NOT EXISTS after_snapshot JSONB,
  ADD COLUMN IF NOT EXISTS revision_before BIGINT,
  ADD COLUMN IF NOT EXISTS revision_after BIGINT,
  ADD COLUMN IF NOT EXISTS plan_id TEXT,
  ADD COLUMN IF NOT EXISTS plan_version_id TEXT,
  ADD COLUMN IF NOT EXISTS price_plan_id TEXT,
  ADD COLUMN IF NOT EXISTS wechat_good_id TEXT,
  ADD COLUMN IF NOT EXISTS payment_binding_id TEXT,
  ADD COLUMN IF NOT EXISTS whitelist_entry_id TEXT,
  ADD COLUMN IF NOT EXISTS environment TEXT;

-- Migration 100 is deliberately rerunnable in isolated verification databases.
-- Recreate this NOT VALID contract so pre-production refinements are applied without touching historical rows.
ALTER TABLE xz_audit_logs
  DROP CONSTRAINT IF EXISTS ck_xz_audit_logs_pricing_required_100;

DO $$
DECLARE
  identity_index RECORD;
BEGIN
  SELECT
    indexed_table.relname AS indexed_table,
    index_method.amname AS index_method,
    index_item.indisunique AS is_unique,
    index_item.indisvalid AS is_valid,
    index_item.indisready AS is_ready,
    index_item.indnkeyatts AS key_attributes,
    index_item.indnatts AS attributes,
    index_item.indpred IS NOT NULL AS has_predicate,
    index_item.indexprs IS NOT NULL AS has_expressions,
    (
      SELECT array_agg(attribute.attname::TEXT ORDER BY key_item.ordinality)
      FROM unnest(index_item.indkey) WITH ORDINALITY AS key_item(attnum, ordinality)
      JOIN pg_attribute attribute
        ON attribute.attrelid = index_item.indrelid
       AND attribute.attnum = key_item.attnum
    ) AS columns
  INTO identity_index
  FROM pg_class index_relation
  JOIN pg_namespace index_namespace ON index_namespace.oid = index_relation.relnamespace
  JOIN pg_index index_item ON index_item.indexrelid = index_relation.oid
  JOIN pg_class indexed_table ON indexed_table.oid = index_item.indrelid
  JOIN pg_am index_method ON index_method.oid = index_relation.relam
  WHERE index_namespace.nspname = current_schema()
    AND index_relation.relname = 'ux_xz_price_plan_whitelist_identity_100';

  IF FOUND AND (
    identity_index.indexed_table IS DISTINCT FROM 'xz_price_plan_user_whitelist'
    OR identity_index.index_method IS DISTINCT FROM 'btree'
    OR identity_index.is_unique IS DISTINCT FROM TRUE
    OR identity_index.is_valid IS DISTINCT FROM TRUE
    OR identity_index.is_ready IS DISTINCT FROM TRUE
    OR identity_index.key_attributes IS DISTINCT FROM 3
    OR identity_index.attributes IS DISTINCT FROM 3
    OR identity_index.has_predicate IS DISTINCT FROM FALSE
    OR identity_index.has_expressions IS DISTINCT FROM FALSE
    OR identity_index.columns IS DISTINCT FROM ARRAY['id','price_plan_id','user_id']::TEXT[]
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MIGRATION_100_WHITELIST_IDENTITY_INDEX_DRIFT';
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_price_plan_whitelist_identity_100
  ON xz_price_plan_user_whitelist(id, price_plan_id, user_id);

-- Future production read-only preflight: run before validating the composite FK.
-- SELECT quote.id, quote.whitelist_entry_id, quote.price_plan_id, quote.user_id,
--        whitelist.price_plan_id AS whitelist_price_plan_id, whitelist.user_id AS whitelist_user_id
-- FROM xz_order_price_quotes AS quote
-- LEFT JOIN xz_price_plan_user_whitelist AS whitelist ON whitelist.id = quote.whitelist_entry_id
-- WHERE quote.whitelist_entry_id IS NOT NULL
--   AND (whitelist.id IS NULL
--        OR quote.price_plan_id IS DISTINCT FROM whitelist.price_plan_id
--        OR quote.user_id IS DISTINCT FROM whitelist.user_id);

DO $$
DECLARE
  constraint_name TEXT;
BEGIN
  FOR constraint_name IN
    SELECT constraint_item.conname
    FROM pg_constraint constraint_item
    WHERE constraint_item.conrelid = 'xz_price_plan_user_whitelist'::regclass
      AND constraint_item.contype = 'u'
      AND (
        SELECT array_agg(attribute.attname::TEXT ORDER BY key_item.ordinality)
        FROM unnest(constraint_item.conkey) WITH ORDINALITY AS key_item(attnum, ordinality)
        JOIN pg_attribute attribute
          ON attribute.attrelid = constraint_item.conrelid
         AND attribute.attnum = key_item.attnum
      ) = ARRAY['price_plan_id','user_id']::TEXT[]
  LOOP
    EXECUTE format('ALTER TABLE xz_price_plan_user_whitelist DROP CONSTRAINT %I', constraint_name);
  END LOOP;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'ck_xz_price_plan_whitelist_lifecycle_100'
      AND conrelid = 'xz_price_plan_user_whitelist'::regclass
  ) THEN
    ALTER TABLE xz_price_plan_user_whitelist
      ADD CONSTRAINT ck_xz_price_plan_whitelist_lifecycle_100
      CHECK (lifecycle_status IS NULL OR lifecycle_status IN ('ACTIVE','EXPIRED','DISABLED')) NOT VALID;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'ck_xz_price_plan_whitelist_enabled_100'
      AND conrelid = 'xz_price_plan_user_whitelist'::regclass
  ) THEN
    ALTER TABLE xz_price_plan_user_whitelist
      ADD CONSTRAINT ck_xz_price_plan_whitelist_enabled_100
      CHECK (lifecycle_status IS NULL OR enabled = (lifecycle_status = 'ACTIVE')) NOT VALID;
  END IF;

  ALTER TABLE xz_order_price_quotes
    DROP CONSTRAINT IF EXISTS fk_xz_order_price_quotes_whitelist_100;

  ALTER TABLE xz_order_price_quotes
    ADD CONSTRAINT fk_xz_order_price_quotes_whitelist_100
    FOREIGN KEY (whitelist_entry_id, price_plan_id, user_id)
    REFERENCES xz_price_plan_user_whitelist(id, price_plan_id, user_id) NOT VALID;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'ck_xz_audit_logs_pricing_result_100'
      AND conrelid = 'xz_audit_logs'::regclass
  ) THEN
    ALTER TABLE xz_audit_logs
      ADD CONSTRAINT ck_xz_audit_logs_pricing_result_100
      CHECK (domain IS NULL OR domain NOT LIKE 'PRICING%' OR result IN ('SUCCEEDED','FAILED')) NOT VALID;
  END IF;

  IF NOT EXISTS (
	SELECT 1 FROM pg_constraint
	WHERE conname = 'ck_xz_audit_logs_pricing_required_100'
	  AND conrelid = 'xz_audit_logs'::regclass
  ) THEN
	ALTER TABLE xz_audit_logs
	  ADD CONSTRAINT ck_xz_audit_logs_pricing_required_100
	  CHECK (
		domain IS NULL OR domain NOT LIKE 'PRICING%' OR (
		  NULLIF(BTRIM(request_id), '') IS NOT NULL
		  AND NULLIF(BTRIM(actor_id), '') IS NOT NULL
		  AND NULLIF(BTRIM(actor_role), '') IS NOT NULL
		  AND NULLIF(BTRIM(action), '') IS NOT NULL
		  AND NULLIF(BTRIM(resource), '') IS NOT NULL
		  AND NULLIF(BTRIM(resource_id), '') IS NOT NULL
		  AND NULLIF(BTRIM(change_reason), '') IS NOT NULL
		  AND result IN ('SUCCEEDED','FAILED')
		  AND (
			result = 'FAILED' OR (
			  revision_before IS NOT NULL
			  AND revision_after IS NOT NULL
			  AND after_snapshot IS NOT NULL
			  AND (before_snapshot IS NOT NULL OR action ~ '\.(create|clone)$')
			)
		  )
		)
	  ) NOT VALID;
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_price_plan_whitelist_active_100
  ON xz_price_plan_user_whitelist(price_plan_id, user_id)
  WHERE COALESCE(lifecycle_status, CASE WHEN enabled THEN 'ACTIVE' ELSE 'DISABLED' END) = 'ACTIVE';

CREATE INDEX IF NOT EXISTS idx_xz_price_plan_whitelist_list_100
  ON xz_price_plan_user_whitelist(price_plan_id, created_at DESC, id);

CREATE INDEX IF NOT EXISTS idx_xz_price_plan_whitelist_user_lifecycle_100
  ON xz_price_plan_user_whitelist(user_id, lifecycle_status, effective_at, expires_at);

CREATE INDEX IF NOT EXISTS idx_xz_order_price_quotes_whitelist_100
  ON xz_order_price_quotes(whitelist_entry_id, created_at DESC)
  WHERE whitelist_entry_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_xz_audit_logs_pricing_created_100
  ON xz_audit_logs(domain, created_at DESC)
  WHERE domain LIKE 'PRICING%';

CREATE INDEX IF NOT EXISTS idx_xz_audit_logs_pricing_request_100
  ON xz_audit_logs(request_id, created_at DESC)
  WHERE request_id IS NOT NULL AND domain LIKE 'PRICING%';

CREATE INDEX IF NOT EXISTS idx_xz_audit_logs_pricing_price_plan_100
  ON xz_audit_logs(price_plan_id, created_at DESC)
  WHERE price_plan_id IS NOT NULL AND domain LIKE 'PRICING%';

CREATE INDEX IF NOT EXISTS idx_xz_audit_logs_pricing_whitelist_100
  ON xz_audit_logs(whitelist_entry_id, created_at DESC)
  WHERE whitelist_entry_id IS NOT NULL AND domain LIKE 'PRICING%';

CREATE INDEX IF NOT EXISTS idx_xz_audit_logs_pricing_action_100
  ON xz_audit_logs(action, created_at DESC, id DESC)
  WHERE domain LIKE 'PRICING%';

CREATE INDEX IF NOT EXISTS idx_xz_audit_logs_pricing_operator_100
  ON xz_audit_logs(actor_id, created_at DESC, id DESC)
  WHERE actor_id IS NOT NULL AND domain LIKE 'PRICING%';

CREATE INDEX IF NOT EXISTS idx_xz_audit_logs_pricing_operator_role_100
  ON xz_audit_logs(actor_role, created_at DESC, id DESC)
  WHERE actor_role IS NOT NULL AND domain LIKE 'PRICING%';

CREATE INDEX IF NOT EXISTS idx_xz_audit_logs_pricing_plan_100
  ON xz_audit_logs(plan_id, created_at DESC, id DESC)
  WHERE plan_id IS NOT NULL AND domain LIKE 'PRICING%';

CREATE INDEX IF NOT EXISTS idx_xz_audit_logs_pricing_plan_version_100
  ON xz_audit_logs(plan_version_id, created_at DESC, id DESC)
  WHERE plan_version_id IS NOT NULL AND domain LIKE 'PRICING%';

CREATE INDEX IF NOT EXISTS idx_xz_audit_logs_pricing_wechat_good_100
  ON xz_audit_logs(wechat_good_id, created_at DESC, id DESC)
  WHERE wechat_good_id IS NOT NULL AND domain LIKE 'PRICING%';

CREATE INDEX IF NOT EXISTS idx_xz_audit_logs_pricing_binding_100
  ON xz_audit_logs(payment_binding_id, created_at DESC, id DESC)
  WHERE payment_binding_id IS NOT NULL AND domain LIKE 'PRICING%';

CREATE INDEX IF NOT EXISTS idx_xz_audit_logs_pricing_result_100
  ON xz_audit_logs(result, created_at DESC, id DESC)
  WHERE result IS NOT NULL AND domain LIKE 'PRICING%';

CREATE OR REPLACE FUNCTION xz_guard_price_plan_test_whitelist_100()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  old_lifecycle TEXT;
  legacy_normalization BOOLEAN := FALSE;
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'WHITELIST_DELETE_FORBIDDEN';
  END IF;

  IF TG_OP = 'INSERT' AND NEW.lifecycle_status IS NULL THEN
    NEW.lifecycle_status := CASE WHEN NEW.enabled THEN 'ACTIVE' ELSE 'DISABLED' END;
  END IF;

  IF TG_OP = 'UPDATE' AND OLD.lifecycle_status IS NULL AND NEW.lifecycle_status IS NULL THEN
    old_lifecycle := CASE WHEN OLD.enabled THEN 'ACTIVE' ELSE 'DISABLED' END;
    IF old_lifecycle = 'DISABLED' AND NEW.enabled IS DISTINCT FROM OLD.enabled THEN
      RAISE EXCEPTION USING
        ERRCODE = '23514',
        MESSAGE = 'WHITELIST_TERMINAL_IMMUTABLE';
    END IF;
    NEW.lifecycle_status := CASE WHEN NEW.enabled THEN 'ACTIVE' ELSE 'DISABLED' END;
    legacy_normalization := TRUE;
  END IF;

  IF NEW.lifecycle_status IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'WHITELIST_LIFECYCLE_REQUIRED';
  END IF;

  IF NEW.lifecycle_status NOT IN ('ACTIVE','EXPIRED','DISABLED')
     OR NEW.enabled IS DISTINCT FROM (NEW.lifecycle_status = 'ACTIVE') THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'WHITELIST_LIFECYCLE_INCONSISTENT';
  END IF;

  IF TG_OP = 'INSERT' THEN
    RETURN NEW;
  END IF;

  IF NEW.id IS DISTINCT FROM OLD.id
     OR NEW.price_plan_id IS DISTINCT FROM OLD.price_plan_id
     OR NEW.user_id IS DISTINCT FROM OLD.user_id
     OR NEW.created_by IS DISTINCT FROM OLD.created_by
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'WHITELIST_IDENTITY_IMMUTABLE';
  END IF;

  old_lifecycle := COALESCE(
    OLD.lifecycle_status,
    CASE WHEN OLD.enabled THEN 'ACTIVE' ELSE 'DISABLED' END
  );

  IF old_lifecycle IN ('EXPIRED','DISABLED') AND NOT legacy_normalization THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'WHITELIST_TERMINAL_IMMUTABLE';
  END IF;

  IF OLD.expires_at IS NOT NULL
     AND OLD.expires_at <= statement_timestamp()
     AND NEW.lifecycle_status <> 'EXPIRED' THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'WHITELIST_TEMPORALLY_EXPIRED_IMMUTABLE';
  END IF;

  IF NEW.lifecycle_status NOT IN ('ACTIVE','EXPIRED','DISABLED') THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'WHITELIST_LIFECYCLE_TRANSITION_INVALID';
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_xz_price_plan_whitelist_guard_100 ON xz_price_plan_user_whitelist;
CREATE TRIGGER trg_xz_price_plan_whitelist_guard_100
BEFORE INSERT OR UPDATE OR DELETE ON xz_price_plan_user_whitelist
FOR EACH ROW EXECUTE FUNCTION xz_guard_price_plan_test_whitelist_100();

CREATE OR REPLACE FUNCTION xz_guard_order_price_quote_whitelist_pin_100()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  pin_complete BOOLEAN;
  pin_empty BOOLEAN;
BEGIN
  pin_complete := NULLIF(BTRIM(NEW.whitelist_entry_id), '') IS NOT NULL
    AND NEW.whitelist_revision IS NOT NULL
    AND NEW.whitelist_revision > 0
    AND NEW.whitelist_checked_at IS NOT NULL;
  pin_empty := NEW.whitelist_entry_id IS NULL
    AND NEW.whitelist_revision IS NULL
    AND NEW.whitelist_checked_at IS NULL;

  IF TG_OP = 'INSERT' THEN
    IF NEW.entry_type = 'TEST' AND NOT pin_complete THEN
      RAISE EXCEPTION USING
        ERRCODE = '23514',
        MESSAGE = 'PRICE_QUOTE_TEST_WHITELIST_PIN_REQUIRED';
    END IF;
    IF NEW.entry_type <> 'TEST' AND NOT pin_empty THEN
      RAISE EXCEPTION USING
        ERRCODE = '23514',
        MESSAGE = 'PRICE_QUOTE_NON_TEST_WHITELIST_PIN_FORBIDDEN';
    END IF;
    RETURN NEW;
  END IF;

  IF NEW.entry_type IS DISTINCT FROM OLD.entry_type
     OR NEW.whitelist_entry_id IS DISTINCT FROM OLD.whitelist_entry_id
     OR NEW.whitelist_revision IS DISTINCT FROM OLD.whitelist_revision
     OR NEW.whitelist_checked_at IS DISTINCT FROM OLD.whitelist_checked_at THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'PRICE_QUOTE_WHITELIST_PIN_IMMUTABLE';
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_xz_order_price_quotes_whitelist_pin_100 ON xz_order_price_quotes;
CREATE TRIGGER trg_xz_order_price_quotes_whitelist_pin_100
BEFORE INSERT OR UPDATE OF entry_type, whitelist_entry_id, whitelist_revision, whitelist_checked_at
ON xz_order_price_quotes
FOR EACH ROW EXECUTE FUNCTION xz_guard_order_price_quote_whitelist_pin_100();

CREATE OR REPLACE FUNCTION xz_guard_pricing_audit_100()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
  IF OLD.domain LIKE 'PRICING%' THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = 'PRICING_AUDIT_IMMUTABLE';
  END IF;
  IF TG_OP = 'UPDATE' THEN
	IF NEW.domain LIKE 'PRICING%' THEN
	  RAISE EXCEPTION USING
		ERRCODE = '23514',
		MESSAGE = 'PRICING_AUDIT_IMMUTABLE';
	END IF;
    RETURN NEW;
  END IF;
  RETURN OLD;
END;
$$;

DROP TRIGGER IF EXISTS trg_xz_audit_logs_pricing_guard_100 ON xz_audit_logs;
CREATE TRIGGER trg_xz_audit_logs_pricing_guard_100
BEFORE UPDATE OR DELETE ON xz_audit_logs
FOR EACH ROW EXECUTE FUNCTION xz_guard_pricing_audit_100();

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
