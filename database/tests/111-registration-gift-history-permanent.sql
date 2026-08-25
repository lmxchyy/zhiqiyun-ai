-- Upgrade/replay fixture for migration 111.
-- Run after migrations 002..109, before 110/111, in an isolated test database.

\set ON_ERROR_STOP on

INSERT INTO xz_users(id)
VALUES ('verify_111_user')
ON CONFLICT (id) DO NOTHING;

INSERT INTO xz_point_accounts(id, user_id, available, frozen)
VALUES ('verify_111_account', 'verify_111_user', 0, 0)
ON CONFLICT (id) DO NOTHING;

-- Historical full, partial, and exhausted registration gifts.
INSERT INTO xz_personal_point_lots(
  id, account_id, user_id, source_type, original_points, available_points,
  reserved_points, consumed_points, expired_points, reversed_points,
  granted_at, expires_at, policy_version_id, policy_snapshot,
  idempotency_key, status
) VALUES
  (
    'verify_111_registration_full', 'verify_111_account', 'verify_111_user',
    'REGISTRATION_GIFT', 10, 10, 0, 0, 0, 0,
    now() - interval '1 day', now() + interval '3 months',
    'point_expiry_policy_v1',
    '{"version":1,"enabled":true,"duration_value":3,"duration_unit":"CALENDAR_MONTH","time_zone":"Asia/Shanghai"}'::jsonb,
    'verify_111_registration_full', 'ACTIVE'
  ),
  (
    'verify_111_registration_partial', 'verify_111_account', 'verify_111_user',
    'REGISTRATION_GIFT', 10, 5, 0, 5, 0, 0,
    now() - interval '2 days', now() + interval '3 months',
    'point_expiry_policy_v1',
    '{"version":1,"enabled":true,"duration_value":3,"duration_unit":"CALENDAR_MONTH","time_zone":"Asia/Shanghai"}'::jsonb,
    'verify_111_registration_partial', 'ACTIVE'
  ),
  (
    'verify_111_registration_exhausted', 'verify_111_account', 'verify_111_user',
    'REGISTRATION_GIFT', 10, 0, 0, 10, 0, 0,
    now() - interval '3 days', now() + interval '3 months',
    'point_expiry_policy_v1',
    '{"version":1,"enabled":true,"duration_value":3,"duration_unit":"CALENDAR_MONTH","time_zone":"Asia/Shanghai"}'::jsonb,
    'verify_111_registration_exhausted', 'EXHAUSTED'
  );

-- Non-registration gifts are control rows and must remain policy-bound.
INSERT INTO xz_personal_point_lots(
  id, account_id, user_id, source_type, original_points, available_points,
  reserved_points, consumed_points, expired_points, reversed_points,
  granted_at, expires_at, policy_version_id, policy_snapshot,
  idempotency_key, status
) VALUES
  (
    'verify_111_activity', 'verify_111_account', 'verify_111_user',
    'ACTIVITY_GIFT', 10, 10, 0, 0, 0, 0,
    now() - interval '1 day', now() + interval '3 months',
    'point_expiry_policy_v1',
    '{"version":1,"enabled":true,"duration_value":3,"duration_unit":"CALENDAR_MONTH","time_zone":"Asia/Shanghai"}'::jsonb,
    'verify_111_activity', 'ACTIVE'
  ),
  (
    'verify_111_admin', 'verify_111_account', 'verify_111_user',
    'ADMIN_GIFT', 10, 10, 0, 0, 0, 0,
    now() - interval '1 day', now() + interval '3 months',
    'point_expiry_policy_v1',
    '{"version":1,"enabled":true,"duration_value":3,"duration_unit":"CALENDAR_MONTH","time_zone":"Asia/Shanghai"}'::jsonb,
    'verify_111_admin', 'ACTIVE'
  );

CREATE TEMP TABLE verify_111_before AS
SELECT id, source_type, original_points, available_points, reserved_points,
       consumed_points, expired_points, reversed_points, granted_at,
       expires_at, policy_version_id, policy_snapshot, status, updated_at
FROM xz_personal_point_lots
WHERE id LIKE 'verify_111_%';

\ir ../migrations/110-registration-gift-points-permanent.sql
\ir ../migrations/111-registration-gift-history-permanent.sql

DO $$
DECLARE
  registration_count bigint;
  control_count bigint;
  changed_count bigint;
BEGIN
  SELECT count(*) INTO registration_count
  FROM xz_personal_point_lots
  WHERE id LIKE 'verify_111_registration_%'
    AND expires_at IS NULL
    AND policy_version_id IS NULL;
  IF registration_count <> 3 THEN
    RAISE EXCEPTION 'migration 111 repaired % registration lots, expected 3', registration_count;
  END IF;

  SELECT count(*) INTO control_count
  FROM xz_personal_point_lots lot
  JOIN verify_111_before before_row USING (id)
  WHERE lot.source_type IN ('ACTIVITY_GIFT', 'ADMIN_GIFT')
    AND lot.original_points IS NOT DISTINCT FROM before_row.original_points
    AND lot.available_points IS NOT DISTINCT FROM before_row.available_points
    AND lot.reserved_points IS NOT DISTINCT FROM before_row.reserved_points
    AND lot.consumed_points IS NOT DISTINCT FROM before_row.consumed_points
    AND lot.expired_points IS NOT DISTINCT FROM before_row.expired_points
    AND lot.reversed_points IS NOT DISTINCT FROM before_row.reversed_points
    AND lot.granted_at IS NOT DISTINCT FROM before_row.granted_at
    AND lot.expires_at IS NOT DISTINCT FROM before_row.expires_at
    AND lot.policy_version_id IS NOT DISTINCT FROM before_row.policy_version_id
    AND lot.policy_snapshot IS NOT DISTINCT FROM before_row.policy_snapshot
    AND lot.status IS NOT DISTINCT FROM before_row.status
    AND lot.updated_at IS NOT DISTINCT FROM before_row.updated_at;
  IF control_count <> 2 THEN
    RAISE EXCEPTION 'migration 111 changed % non-registration control lots', 2 - control_count;
  END IF;

  SELECT count(*) INTO changed_count
  FROM xz_personal_point_lots lot
  JOIN verify_111_before before_row USING (id)
  WHERE lot.source_type = 'REGISTRATION_GIFT'
    AND (
      lot.original_points IS DISTINCT FROM before_row.original_points
      OR lot.available_points IS DISTINCT FROM before_row.available_points
      OR lot.reserved_points IS DISTINCT FROM before_row.reserved_points
      OR lot.consumed_points IS DISTINCT FROM before_row.consumed_points
      OR lot.expired_points IS DISTINCT FROM before_row.expired_points
      OR lot.reversed_points IS DISTINCT FROM before_row.reversed_points
      OR lot.granted_at IS DISTINCT FROM before_row.granted_at
      OR lot.policy_snapshot IS DISTINCT FROM before_row.policy_snapshot
      OR lot.status IS DISTINCT FROM before_row.status
      OR lot.updated_at IS DISTINCT FROM before_row.updated_at
    );
  IF changed_count <> 0 THEN
    RAISE EXCEPTION 'migration 111 changed % protected registration lot fields', changed_count;
  END IF;
END;
$$;

-- Re-execution must be a no-op for every row and leave the repaired state intact.
\ir ../migrations/111-registration-gift-history-permanent.sql

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM xz_personal_point_lots
    WHERE id LIKE 'verify_111_registration_%'
      AND (expires_at IS NOT NULL OR policy_version_id IS NOT NULL)
  ) THEN
    RAISE EXCEPTION 'migration 111 is not idempotent';
  END IF;
END;
$$;

DELETE FROM xz_personal_point_lots WHERE id LIKE 'verify_111_%';
DELETE FROM xz_point_accounts WHERE id = 'verify_111_account';
DELETE FROM xz_users WHERE id = 'verify_111_user';
