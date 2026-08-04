-- Restore migration 103's strict immutable trigger.
--
-- Rollback is deliberately refused after any 104-managed policy history is
-- live. Returning to the 103 trigger would make those archived/new published
-- versions impossible to manage safely.

BEGIN;

DO $$
BEGIN
  IF to_regclass('public.xz_point_expiry_policy_versions') IS NULL THEN
    RAISE EXCEPTION 'migration 104 rollback requires xz_point_expiry_policy_versions';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM xz_point_expiry_policy_versions
    WHERE status = 'ARCHIVED'
       OR (status IN ('PUBLISHED', 'ARCHIVED')
           AND (id <> 'point_expiry_policy_v1' OR version <> 1))
  ) THEN
    RAISE EXCEPTION
      'migration 104 rollback refused: published policy history is live';
  END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_xz_point_expiry_policy_versions_current_published
  ON xz_point_expiry_policy_versions;
DROP FUNCTION IF EXISTS xz_assert_personal_point_policy_current_published();

CREATE OR REPLACE FUNCTION xz_reject_published_personal_point_policy_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP IN ('UPDATE', 'DELETE') AND OLD.status = 'PUBLISHED' THEN
    RAISE EXCEPTION 'published point expiry policy versions are immutable';
  END IF;
  IF TG_OP = 'DELETE' THEN
    RETURN OLD;
  END IF;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_xz_point_expiry_policy_versions_immutable
  ON xz_point_expiry_policy_versions;
CREATE TRIGGER trg_xz_point_expiry_policy_versions_immutable
BEFORE UPDATE OR DELETE ON xz_point_expiry_policy_versions
FOR EACH ROW EXECUTE FUNCTION xz_reject_published_personal_point_policy_mutation();

COMMIT;
