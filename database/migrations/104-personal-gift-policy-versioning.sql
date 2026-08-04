-- Repair the published personal gift-point policy lifecycle.
--
-- Migration 103 made a PUBLISHED policy immutable for every UPDATE. The
-- policy admin path, however, must close the current version exactly once and
-- insert the next version in the same transaction. This migration changes
-- only that trigger contract; policy data and all lot/ledger tables remain
-- untouched.

BEGIN;

DO $$
DECLARE
  published_count BIGINT;
BEGIN
  IF to_regclass('public.xz_point_expiry_policy_versions') IS NULL THEN
    RAISE EXCEPTION 'migration 104 requires xz_point_expiry_policy_versions';
  END IF;

  SELECT count(*)
  INTO published_count
  FROM xz_point_expiry_policy_versions
  WHERE status = 'PUBLISHED';
  IF published_count > 1 THEN
    RAISE EXCEPTION
      'migration 104 refuses multiple PUBLISHED policy versions (%)',
      published_count;
  END IF;

  IF EXISTS (
    SELECT 1
    FROM xz_point_expiry_policy_versions
    WHERE status = 'PUBLISHED' AND effective_to IS NOT NULL
  ) THEN
    RAISE EXCEPTION
      'migration 104 refuses PUBLISHED policy versions with effective_to';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM xz_point_expiry_policy_versions
    WHERE status = 'ARCHIVED'
      AND (effective_to IS NULL OR effective_to <= effective_from)
  ) THEN
    RAISE EXCEPTION
      'migration 104 refuses invalid archived policy effective range';
  END IF;
END;
$$;

CREATE OR REPLACE FUNCTION xz_reject_published_personal_point_policy_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  published_count BIGINT;
BEGIN
  -- PostgreSQL CHECK constraints also protect the status vocabulary, but this
  -- trigger intentionally fails closed before a constraint can be relaxed.
  IF TG_OP = 'INSERT' THEN
    IF NEW.status IS NULL OR NEW.status NOT IN ('DRAFT', 'PUBLISHED', 'ARCHIVED') THEN
      RAISE EXCEPTION 'unknown point expiry policy status %', NEW.status;
    END IF;

    IF NEW.status = 'PUBLISHED' THEN
      IF NEW.effective_to IS NOT NULL THEN
        RAISE EXCEPTION
          'new PUBLISHED policy version must not have effective_to';
      END IF;

      -- A transaction-scoped advisory lock serializes publishers without a
      -- partial unique index that would interfere with policy history.
      PERFORM pg_advisory_xact_lock(
        hashtextextended('xz_point_expiry_policy_versions:published', 0)
      );
      SELECT count(*)
      INTO published_count
      FROM xz_point_expiry_policy_versions
      WHERE status = 'PUBLISHED';
      IF published_count > 0 THEN
        RAISE EXCEPTION
          'published point expiry policy version already exists; revision conflict';
      END IF;
    END IF;

    RETURN NEW;
  END IF;

  IF OLD.status IS NULL OR OLD.status NOT IN ('DRAFT', 'PUBLISHED', 'ARCHIVED') THEN
    RAISE EXCEPTION 'unknown point expiry policy status %', OLD.status;
  END IF;

  IF TG_OP = 'DELETE' THEN
    IF OLD.status IN ('PUBLISHED', 'ARCHIVED') THEN
      RAISE EXCEPTION
        '% point expiry policy versions cannot be deleted', OLD.status;
    END IF;
    RETURN OLD;
  END IF;

  IF NEW.status IS NULL OR NEW.status NOT IN ('DRAFT', 'PUBLISHED', 'ARCHIVED') THEN
    RAISE EXCEPTION 'unknown point expiry policy status %', NEW.status;
  END IF;

  IF OLD.status = 'ARCHIVED' THEN
    RAISE EXCEPTION 'archived point expiry policy versions are immutable';
  END IF;

  IF OLD.status = 'PUBLISHED' THEN
    IF NEW.status <> 'ARCHIVED' THEN
      RAISE EXCEPTION
        'published point expiry policy versions may only close to ARCHIVED';
    END IF;
    IF OLD.effective_to IS NOT NULL THEN
      RAISE EXCEPTION
        'published point expiry policy version is already closed';
    END IF;
    IF NEW.effective_to IS NULL OR NEW.effective_to <= OLD.effective_from THEN
      RAISE EXCEPTION
        'archived point expiry policy version requires effective_to > effective_from';
    END IF;
    IF NEW.updated_at < OLD.updated_at THEN
      RAISE EXCEPTION
        'point expiry policy updated_at cannot move backwards';
    END IF;

    -- A close is the sole mutable operation on a PUBLISHED version. Every
    -- business field is compared with IS NOT DISTINCT FROM, so NULL/JSON
    -- changes cannot hide behind a generic row comparison.
    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.version IS DISTINCT FROM OLD.version
       OR NEW.revision IS DISTINCT FROM OLD.revision
       OR NEW.enabled IS DISTINCT FROM OLD.enabled
       OR NEW.duration_value IS DISTINCT FROM OLD.duration_value
       OR NEW.duration_unit IS DISTINCT FROM OLD.duration_unit
       OR NEW.time_zone IS DISTINCT FROM OLD.time_zone
       OR NEW.source_types IS DISTINCT FROM OLD.source_types
       OR NEW.effective_from IS DISTINCT FROM OLD.effective_from
       OR NEW.created_by IS DISTINCT FROM OLD.created_by
       OR NEW.change_reason IS DISTINCT FROM OLD.change_reason
       OR NEW.metadata IS DISTINCT FROM OLD.metadata
       OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
      RAISE EXCEPTION
        'published point expiry policy content is immutable';
    END IF;

    PERFORM pg_advisory_xact_lock(
      hashtextextended('xz_point_expiry_policy_versions:published', 0)
    );
    SELECT count(*)
    INTO published_count
    FROM xz_point_expiry_policy_versions
    WHERE status = 'PUBLISHED' AND id <> OLD.id;
    IF published_count > 0 THEN
      RAISE EXCEPTION
        'cannot close PUBLISHED policy while another PUBLISHED version exists';
    END IF;

    RETURN NEW;
  END IF;

  IF OLD.status = 'DRAFT' AND NEW.status = 'ARCHIVED' THEN
    IF NEW.effective_to IS NULL OR NEW.effective_to <= NEW.effective_from THEN
      RAISE EXCEPTION
        'archived point expiry policy version requires effective_to > effective_from';
    END IF;
  END IF;

  -- DRAFT rows keep the existing authoring behavior. Promoting one to
  -- PUBLISHED uses the same serialization and one-current-version guard as a
  -- direct INSERT; a DRAFT may not carry a closing timestamp.
  IF NEW.status = 'PUBLISHED' THEN
    IF NEW.effective_to IS NOT NULL THEN
      RAISE EXCEPTION
        'new PUBLISHED policy version must not have effective_to';
    END IF;
    PERFORM pg_advisory_xact_lock(
      hashtextextended('xz_point_expiry_policy_versions:published', 0)
    );
    SELECT count(*)
    INTO published_count
    FROM xz_point_expiry_policy_versions
    WHERE status = 'PUBLISHED' AND id <> OLD.id;
    IF published_count > 0 THEN
      RAISE EXCEPTION
        'published point expiry policy version already exists; revision conflict';
    END IF;
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_xz_point_expiry_policy_versions_immutable
  ON xz_point_expiry_policy_versions;
CREATE TRIGGER trg_xz_point_expiry_policy_versions_immutable
BEFORE INSERT OR UPDATE OR DELETE ON xz_point_expiry_policy_versions
FOR EACH ROW EXECUTE FUNCTION xz_reject_published_personal_point_policy_mutation();

COMMIT;
