-- Registration welcome points are permanent and do not participate in the
-- expiring gift-policy history. Activity and admin gifts keep migration 103's
-- published policy and can be governed independently later.
BEGIN;

CREATE OR REPLACE FUNCTION xz_validate_personal_point_lot_policy()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  policy RECORD;
BEGIN
  IF NEW.source_type = 'REGISTRATION_GIFT' THEN
    IF NEW.policy_version_id IS NOT NULL OR NEW.expires_at IS NOT NULL THEN
      RAISE EXCEPTION 'registration gift point lot must be permanent';
    END IF;
    RETURN NEW;
  END IF;

  IF NEW.source_type IN ('ACTIVITY_GIFT', 'ADMIN_GIFT') THEN
    IF NEW.policy_version_id IS NULL THEN
      RAISE EXCEPTION 'gift point lot requires an explicit policy version';
    END IF;

    SELECT version, enabled, duration_value, duration_unit, time_zone,
           source_types, status
    INTO policy
    FROM xz_point_expiry_policy_versions
    WHERE id = NEW.policy_version_id;

    IF NOT FOUND THEN
      RAISE EXCEPTION 'gift point lot references missing policy version %', NEW.policy_version_id;
    END IF;
    IF policy.status NOT IN ('PUBLISHED', 'ARCHIVED') THEN
      RAISE EXCEPTION 'gift point lot references unpublished policy version %', NEW.policy_version_id;
    END IF;
    IF NOT (policy.source_types ? NEW.source_type) THEN
      RAISE EXCEPTION 'policy version % does not cover gift source %', NEW.policy_version_id, NEW.source_type;
    END IF;
    IF jsonb_typeof(NEW.policy_snapshot) <> 'object'
       OR NEW.policy_snapshot = '{}'::jsonb
       OR NEW.policy_snapshot->>'version' IS DISTINCT FROM policy.version::text
       OR NEW.policy_snapshot->>'enabled' IS DISTINCT FROM policy.enabled::text
       OR NEW.policy_snapshot->>'duration_value' IS DISTINCT FROM policy.duration_value::text
       OR NEW.policy_snapshot->>'duration_unit' IS DISTINCT FROM policy.duration_unit
       OR NEW.policy_snapshot->>'time_zone' IS DISTINCT FROM policy.time_zone THEN
      RAISE EXCEPTION 'gift point lot policy snapshot does not match policy version %', NEW.policy_version_id;
    END IF;
    IF policy.enabled AND NEW.expires_at IS NULL THEN
      RAISE EXCEPTION 'enabled gift point policy requires expires_at';
    END IF;
    IF NOT policy.enabled AND NEW.expires_at IS NOT NULL THEN
      RAISE EXCEPTION 'disabled gift point policy must remain permanent';
    END IF;
  ELSIF NEW.policy_version_id IS NOT NULL OR NEW.expires_at IS NOT NULL THEN
    RAISE EXCEPTION 'non-gift point lot cannot carry expiry policy or expires_at';
  END IF;

  RETURN NEW;
END;
$$;

COMMIT;
