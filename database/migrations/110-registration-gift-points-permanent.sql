-- Registration welcome points are permanent. Activity and admin gifts keep
-- the existing v1 expiry policy and can be governed independently later.
BEGIN;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM xz_point_expiry_policy_versions
    WHERE id = 'point_expiry_policy_registration_permanent'
  ) THEN
    INSERT INTO xz_point_expiry_policy_versions(
      id, version, revision, enabled, duration_value, duration_unit, time_zone,
      source_types, effective_from, status, created_by, change_reason
    ) VALUES (
      'point_expiry_policy_registration_permanent', 2, 2, FALSE, 0, 'CALENDAR_MONTH', 'Asia/Shanghai',
      '["REGISTRATION_GIFT"]'::jsonb, now(), 'PUBLISHED', 'system',
      'Registration welcome points are permanent'
    );
  END IF;
END;
$$;

COMMIT;
