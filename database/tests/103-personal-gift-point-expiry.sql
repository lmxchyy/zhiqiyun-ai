-- Targeted verification fixture for migration 103.
-- Run this file with psql -v ON_ERROR_STOP=1 against an isolated copy after
-- the repository migrations have been applied. It is deliberately read-only
-- apart from savepoint-scoped constraint probes.

\set ON_ERROR_STOP on

BEGIN;

DO $$
DECLARE
  required_table text;
BEGIN
  FOREACH required_table IN ARRAY ARRAY[
    'xz_point_expiry_policy_versions',
    'xz_personal_point_lots',
    'xz_personal_point_reservations',
    'xz_personal_point_reservation_allocations',
    'xz_personal_point_lot_movements'
  ] LOOP
    IF to_regclass('public.' || required_table) IS NULL THEN
      RAISE EXCEPTION 'migration 103 schema missing required table %', required_table;
    END IF;
  END LOOP;
END;
$$;

DO $$
DECLARE
  policy record;
BEGIN
  SELECT enabled, duration_value, duration_unit, time_zone, status
  INTO policy
  FROM xz_point_expiry_policy_versions
  WHERE version = 1
  ORDER BY effective_from
  LIMIT 1;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'migration 103 initial policy version is missing';
  END IF;
  IF policy.enabled IS DISTINCT FROM TRUE
     OR policy.duration_value IS DISTINCT FROM 3
     OR policy.duration_unit IS DISTINCT FROM 'CALENDAR_MONTH'
     OR policy.time_zone IS DISTINCT FROM 'Asia/Shanghai'
     OR policy.status IS DISTINCT FROM 'PUBLISHED' THEN
    RAISE EXCEPTION 'migration 103 initial policy does not match the three-calendar-month Asia/Shanghai contract';
  END IF;
END;
$$;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM xz_point_accounts account
    LEFT JOIN LATERAL (
      SELECT
        coalesce(sum(lot.available_points), 0) AS available_points,
        coalesce(sum(lot.reserved_points), 0) AS reserved_points
      FROM xz_personal_point_lots lot
      WHERE lot.account_id = account.id
    ) lots ON TRUE
    WHERE account.available <> lots.available_points
       OR account.frozen <> lots.reserved_points
  ) THEN
    RAISE EXCEPTION 'migration 103 legacy lot projection does not reconcile xz_point_accounts';
  END IF;
END;
$$;

-- Seed only zero-value probe rows; the surrounding transaction rolls them back.
INSERT INTO xz_users(id)
VALUES ('verify_103_user')
ON CONFLICT (id) DO NOTHING;
INSERT INTO xz_point_accounts(id, user_id, available, frozen)
VALUES ('verify_103_account', 'verify_103_user', 0, 0)
ON CONFLICT (id) DO NOTHING;

-- Negative amounts, invalid source values, and conservation drift must fail.
SAVEPOINT migration_103_constraint_probes;
DO $$
BEGIN
  BEGIN
    INSERT INTO xz_personal_point_lots(
      id, account_id, user_id, source_type, original_points, available_points,
      reserved_points, consumed_points, expired_points, reversed_points,
      granted_at, idempotency_key
    ) VALUES (
      'verify_103_negative', 'verify_103_account', 'verify_103_user',
      'ADMIN_GIFT', -1, 0, 0, 0, 0, 0, now(), 'verify_103_negative'
    );
    RAISE EXCEPTION 'negative lot amount unexpectedly accepted';
  EXCEPTION WHEN check_violation OR foreign_key_violation OR not_null_violation THEN
    NULL;
  END;

  BEGIN
    INSERT INTO xz_personal_point_lots(
      id, account_id, user_id, source_type, original_points, available_points,
      reserved_points, consumed_points, expired_points, reversed_points,
      granted_at, idempotency_key
    ) VALUES (
      'verify_103_unknown_source', 'verify_103_account', 'verify_103_user',
      'UNKNOWN', 1, 1, 0, 0, 0, 0, now(), 'verify_103_unknown_source'
    );
    RAISE EXCEPTION 'unknown lot source unexpectedly accepted';
  EXCEPTION WHEN check_violation OR foreign_key_violation OR not_null_violation THEN
    NULL;
  END;

  BEGIN
    INSERT INTO xz_personal_point_lots(
      id, account_id, user_id, source_type, original_points, available_points,
      reserved_points, consumed_points, expired_points, reversed_points,
      granted_at, idempotency_key
    ) VALUES (
      'verify_103_drift', 'verify_103_account', 'verify_103_user',
      'ADMIN_GIFT', 2, 1, 0, 0, 0, 0, now(), 'verify_103_drift'
    );
    RAISE EXCEPTION 'lot conservation drift unexpectedly accepted';
  EXCEPTION WHEN check_violation OR foreign_key_violation OR not_null_violation THEN
    NULL;
  END;
END;
$$;
ROLLBACK TO SAVEPOINT migration_103_constraint_probes;

SAVEPOINT migration_103_append_only_probe;
INSERT INTO xz_personal_point_lots(
  id, account_id, user_id, source_type, original_points, available_points,
  reserved_points, consumed_points, expired_points, reversed_points,
  granted_at, idempotency_key
) VALUES (
  'verify_103_append_only_lot', 'verify_103_account', 'verify_103_user',
  'ADMIN_GIFT', 1, 1, 0, 0, 0, 0, now(), 'verify_103_append_only_lot'
);
INSERT INTO xz_personal_point_lot_movements(
  id, lot_id, account_id, user_id, movement_type, points,
  available_before, available_after, reserved_before, reserved_after,
  consumed_before, consumed_after, expired_before, expired_after,
  reversed_before, reversed_after, idempotency_key
) VALUES (
  'verify_103_append_only_movement', 'verify_103_append_only_lot',
  'verify_103_account', 'verify_103_user', 'GRANT', 1,
  0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 'verify_103_append_only_movement'
);
DO $$
BEGIN
  BEGIN
    UPDATE xz_personal_point_lot_movements
    SET metadata = '{"unexpected":true}'::jsonb
    WHERE id = 'verify_103_append_only_movement';
    RAISE EXCEPTION 'lot movement update unexpectedly accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('append-only' IN SQLERRM) = 0 THEN
      RAISE;
    END IF;
  END;
END;
$$;
ROLLBACK TO SAVEPOINT migration_103_append_only_probe;

-- The fixture never mutates an existing economic ledger or production data.
ROLLBACK;
