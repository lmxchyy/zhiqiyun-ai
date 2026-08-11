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
INSERT INTO xz_users(id)
VALUES ('verify_103_user_2')
ON CONFLICT (id) DO NOTHING;
INSERT INTO xz_point_accounts(id, user_id, available, frozen)
VALUES ('verify_103_account_2', 'verify_103_user_2', 0, 0)
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
  EXCEPTION WHEN OTHERS THEN
    IF position('unexpectedly accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
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
  EXCEPTION WHEN OTHERS THEN
    IF position('unexpectedly accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
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
  EXCEPTION WHEN OTHERS THEN
    IF position('unexpectedly accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;

  BEGIN
    INSERT INTO xz_personal_point_lots(
      id, account_id, user_id, source_type, original_points, available_points,
      reserved_points, consumed_points, expired_points, reversed_points,
      granted_at, idempotency_key, status
    ) VALUES (
      'verify_103_expired_with_reserved', 'verify_103_account', 'verify_103_user',
      'MANUAL', 2, 0, 1, 0, 1, 0, now(),
      'verify_103_expired_with_reserved', 'EXPIRED'
    );
    RAISE EXCEPTION 'expired lot with reserved balance unexpectedly accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('unexpectedly accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;
END;
$$;
ROLLBACK TO SAVEPOINT migration_103_constraint_probes;

-- Gift lots must always bind an explicit policy snapshot. Enabled policies
-- require an expiry timestamp; disabled policies are the only permanent gift
-- path. Legacy lots are permanent and must remain outside expiry status.
SAVEPOINT migration_103_policy_and_status_probes;
DO $$
BEGIN
  BEGIN
    INSERT INTO xz_personal_point_lots(
      id, account_id, user_id, source_type, original_points, available_points,
      reserved_points, consumed_points, expired_points, reversed_points,
      granted_at, idempotency_key, status
    ) VALUES (
      'verify_103_legacy_active', 'verify_103_account', 'verify_103_user',
      'LEGACY', 1, 1, 0, 0, 0, 0, now(),
      'verify_103_legacy_active', 'ACTIVE'
    );
    RAISE EXCEPTION 'legacy ACTIVE status unexpectedly accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('unexpectedly accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;

  BEGIN
    INSERT INTO xz_personal_point_lots(
      id, account_id, user_id, source_type, original_points, available_points,
      reserved_points, consumed_points, expired_points, reversed_points,
      granted_at, idempotency_key, status
    ) VALUES (
      'verify_103_gift_without_policy', 'verify_103_account', 'verify_103_user',
      'ADMIN_GIFT', 1, 1, 0, 0, 0, 0, now(),
      'verify_103_gift_without_policy', 'ACTIVE'
    );
    RAISE EXCEPTION 'gift without policy unexpectedly accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('unexpectedly accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;

  BEGIN
    INSERT INTO xz_personal_point_lots(
      id, account_id, user_id, source_type, original_points, available_points,
      reserved_points, consumed_points, expired_points, reversed_points,
      granted_at, idempotency_key, status, policy_version_id, policy_snapshot
    ) VALUES (
      'verify_103_gift_enabled_without_expiry', 'verify_103_account', 'verify_103_user',
      'ADMIN_GIFT', 1, 1, 0, 0, 0, 0, now(),
      'verify_103_gift_enabled_without_expiry', 'ACTIVE',
      'point_expiry_policy_v1',
      '{"version":1,"enabled":true,"duration_value":3,"duration_unit":"CALENDAR_MONTH","time_zone":"Asia/Shanghai"}'::jsonb
    );
    RAISE EXCEPTION 'enabled gift without expiry unexpectedly accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('unexpectedly accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;

  BEGIN
    INSERT INTO xz_personal_point_lots(
      id, account_id, user_id, source_type, original_points, available_points,
      reserved_points, consumed_points, expired_points, reversed_points,
      granted_at, expires_at, idempotency_key, status, policy_version_id,
      policy_snapshot
    ) VALUES (
      'verify_103_gift_enabled_empty_snapshot', 'verify_103_account', 'verify_103_user',
      'ADMIN_GIFT', 1, 1, 0, 0, 0, 0, now(), now() + interval '3 months',
      'verify_103_gift_enabled_empty_snapshot', 'ACTIVE',
      'point_expiry_policy_v1', '{}'::jsonb
    );
    RAISE EXCEPTION 'gift with empty policy snapshot unexpectedly accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('unexpectedly accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;

  BEGIN
    INSERT INTO xz_personal_point_lots(
      id, account_id, user_id, source_type, original_points, available_points,
      reserved_points, consumed_points, expired_points, reversed_points,
      granted_at, expires_at, idempotency_key, status, policy_version_id,
      policy_snapshot
    ) VALUES (
      'verify_103_gift_whitespace_key', 'verify_103_account', 'verify_103_user',
      'ADMIN_GIFT', 1, 1, 0, 0, 0, 0, now(), now() + interval '3 months',
      '   ', 'ACTIVE', 'point_expiry_policy_v1',
      '{"version":1,"enabled":true,"duration_value":3,"duration_unit":"CALENDAR_MONTH","time_zone":"Asia/Shanghai"}'::jsonb
    );
    RAISE EXCEPTION 'blank lot idempotency unexpectedly accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('unexpectedly accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;

  BEGIN
    INSERT INTO xz_personal_point_lots(
      id, account_id, user_id, source_type, original_points, available_points,
      reserved_points, consumed_points, expired_points, reversed_points,
      granted_at, idempotency_key, status
    ) VALUES (
      'verify_103_exhausted_with_balance', 'verify_103_account', 'verify_103_user',
      'MANUAL', 1, 1, 0, 0, 0, 0, now(),
      'verify_103_exhausted_with_balance', 'EXHAUSTED'
    );
    RAISE EXCEPTION 'exhausted lot with available balance unexpectedly accepted';
  EXCEPTION WHEN check_violation OR foreign_key_violation OR not_null_violation THEN
    NULL;
  END;
END;
$$;

-- A disabled policy is the approved permanent-gift case, but it still has an
-- explicit immutable version and a matching snapshot.
INSERT INTO xz_point_expiry_policy_versions(
  id, version, revision, enabled, duration_value, duration_unit, time_zone,
  source_types, effective_from, status, created_by, change_reason, metadata
)
VALUES (
  'point_expiry_policy_v2_disabled', 2, 1, FALSE, 0, 'CALENDAR_MONTH', 'Asia/Shanghai',
  '["REGISTRATION_GIFT","ACTIVITY_GIFT","ADMIN_GIFT"]'::jsonb,
  now(), 'PUBLISHED', 'verify', 'disabled permanent gift probe', '{}'::jsonb
)
ON CONFLICT (version) DO NOTHING;

INSERT INTO xz_personal_point_lots(
  id, account_id, user_id, source_type, original_points, available_points,
  reserved_points, consumed_points, expired_points, reversed_points,
  granted_at, idempotency_key, status, policy_version_id, policy_snapshot
) VALUES (
  'verify_103_valid_disabled_gift', 'verify_103_account', 'verify_103_user',
  'ADMIN_GIFT', 1, 1, 0, 0, 0, 0, now(),
  'verify_103_valid_disabled_gift', 'ACTIVE', 'point_expiry_policy_v2_disabled',
  '{"version":2,"enabled":false,"duration_value":0,"duration_unit":"CALENDAR_MONTH","time_zone":"Asia/Shanghai"}'::jsonb
);

INSERT INTO xz_personal_point_lots(
  id, account_id, user_id, source_type, original_points, available_points,
  reserved_points, consumed_points, expired_points, reversed_points,
  granted_at, expires_at, idempotency_key, status, policy_version_id,
  policy_snapshot
) VALUES (
  'verify_103_valid_enabled_gift', 'verify_103_account', 'verify_103_user',
  'ADMIN_GIFT', 1, 1, 0, 0, 0, 0, now(), now() + interval '3 months',
  'verify_103_valid_enabled_gift', 'ACTIVE', 'point_expiry_policy_v1',
  '{"version":1,"enabled":true,"duration_value":3,"duration_unit":"CALENDAR_MONTH","time_zone":"Asia/Shanghai"}'::jsonb
);

DO $$
BEGIN
  BEGIN
    UPDATE xz_point_expiry_policy_versions
    SET duration_value = 4
    WHERE id = 'point_expiry_policy_v1';
    RAISE EXCEPTION 'published policy update unexpectedly accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('immutable' IN SQLERRM) = 0 THEN
      RAISE;
    END IF;
  END;
END;
$$;
ROLLBACK TO SAVEPOINT migration_103_policy_and_status_probes;

DO $$
BEGIN
  BEGIN
    INSERT INTO xz_personal_point_lots(
      id, account_id, user_id, source_type, original_points, available_points,
      reserved_points, consumed_points, expired_points, reversed_points,
      granted_at, idempotency_key, status, policy_version_id
    ) VALUES (
      'verify_103_non_gift_policy_bypass', 'verify_103_account', 'verify_103_user',
      'MANUAL', 1, 1, 0, 0, 0, 0, now(),
      'verify_103_non_gift_policy_bypass', 'ACTIVE', 'point_expiry_policy_v1'
    );
    RAISE EXCEPTION 'non-gift policy bypass unexpectedly accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('unexpectedly accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;

  BEGIN
    INSERT INTO xz_personal_point_reservations(
      id, account_id, user_id, business_type, business_id, requested_points,
      reserved_points, captured_points, released_points, expired_points,
      idempotency_key, status
    ) VALUES (
      'verify_103_blank_reservation_key', 'verify_103_account', 'verify_103_user',
      'VERIFY', 'blank-reservation-key', 1, 1, 0, 0, 0, '   ', 'RESERVED'
    );
    RAISE EXCEPTION 'blank reservation idempotency unexpectedly accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('unexpectedly accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;

  BEGIN
    INSERT INTO xz_personal_point_reservations(
      id, account_id, user_id, business_type, business_id, requested_points,
      reserved_points, captured_points, released_points, expired_points,
      idempotency_key, status
    ) VALUES (
      'verify_103_captured_without_capture', 'verify_103_account', 'verify_103_user',
      'VERIFY', 'captured-without-capture', 1, 1, 0, 0, 0,
      'verify_103_captured_without_capture', 'CAPTURED'
    );
    RAISE EXCEPTION 'captured reservation with no captured points unexpectedly accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('unexpectedly accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;
END;
$$;

-- Reservation/lot allocations carry the same account and user keys as both
-- parents. Composite foreign keys must reject any cross-account or forged
-- ownership combination.
SAVEPOINT migration_103_ownership_probes;
INSERT INTO xz_personal_point_lots(
  id, account_id, user_id, source_type, original_points, available_points,
  reserved_points, consumed_points, expired_points, reversed_points,
  granted_at, idempotency_key, status
) VALUES
  ('verify_103_manual_lot_1', 'verify_103_account', 'verify_103_user',
   'MANUAL', 1, 1, 0, 0, 0, 0, now(), 'verify_103_manual_lot_1', 'ACTIVE'),
  ('verify_103_manual_lot_2', 'verify_103_account_2', 'verify_103_user_2',
   'MANUAL', 1, 1, 0, 0, 0, 0, now(), 'verify_103_manual_lot_2', 'ACTIVE');
INSERT INTO xz_personal_point_reservations(
  id, account_id, user_id, business_type, business_id, requested_points,
  reserved_points, captured_points, released_points, expired_points,
  idempotency_key, status
) VALUES
  ('verify_103_reservation_1', 'verify_103_account', 'verify_103_user',
   'VERIFY', 'business-1', 1, 1, 0, 0, 0, 'verify_103_reservation_1', 'RESERVED'),
  ('verify_103_reservation_2', 'verify_103_account_2', 'verify_103_user_2',
   'VERIFY', 'business-2', 1, 1, 0, 0, 0, 'verify_103_reservation_2', 'RESERVED');

INSERT INTO xz_personal_point_reservation_allocations(
  id, reservation_id, lot_id, account_id, user_id, allocated_points,
  reserved_points, captured_points, released_points, expired_points, status
) VALUES (
  'verify_103_valid_allocation', 'verify_103_reservation_1',
  'verify_103_manual_lot_1', 'verify_103_account', 'verify_103_user',
  1, 1, 0, 0, 0, 'RESERVED'
);
DO $$
BEGIN
  BEGIN
    INSERT INTO xz_personal_point_reservation_allocations(
      id, reservation_id, lot_id, account_id, user_id, allocated_points,
      reserved_points, captured_points, released_points, expired_points, status
    ) VALUES (
      'verify_103_captured_allocation_without_capture', 'verify_103_reservation_2',
      'verify_103_manual_lot_2', 'verify_103_account_2', 'verify_103_user_2',
      1, 1, 0, 0, 0, 'CAPTURED'
    );
    RAISE EXCEPTION 'captured allocation with no captured points unexpectedly accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('unexpectedly accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;
  BEGIN
    INSERT INTO xz_personal_point_reservation_allocations(
      id, reservation_id, lot_id, account_id, user_id, allocated_points,
      reserved_points, captured_points, released_points, expired_points, status
    ) VALUES (
      'verify_103_cross_account_allocation', 'verify_103_reservation_1',
      'verify_103_manual_lot_2', 'verify_103_account', 'verify_103_user',
      1, 1, 0, 0, 0, 'RESERVED'
    );
    RAISE EXCEPTION 'cross-account allocation unexpectedly accepted';
  EXCEPTION WHEN foreign_key_violation OR check_violation OR not_null_violation THEN
    NULL;
  END;
  BEGIN
    INSERT INTO xz_personal_point_reservation_allocations(
      id, reservation_id, lot_id, account_id, user_id, allocated_points,
      reserved_points, captured_points, released_points, expired_points, status
    ) VALUES (
      'verify_103_forged_allocation_owner', 'verify_103_reservation_2',
      'verify_103_manual_lot_1', 'verify_103_account_2', 'verify_103_user_2',
      1, 1, 0, 0, 0, 'RESERVED'
    );
    RAISE EXCEPTION 'forged allocation owner unexpectedly accepted';
  EXCEPTION WHEN foreign_key_violation OR check_violation OR not_null_violation THEN
    NULL;
  END;
END;
$$;

DO $$
BEGIN
  BEGIN
    INSERT INTO xz_personal_point_lot_movements(
      id, lot_id, account_id, user_id, movement_type, points,
      available_before, available_after, reserved_before, reserved_after,
      consumed_before, consumed_after, expired_before, expired_after,
      reversed_before, reversed_after, idempotency_key
    ) VALUES (
      'verify_103_forged_movement_owner', 'verify_103_manual_lot_1',
      'verify_103_account_2', 'verify_103_user_2', 'GRANT', 1,
      0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 'verify_103_forged_movement_owner'
    );
    RAISE EXCEPTION 'forged movement owner unexpectedly accepted';
  EXCEPTION WHEN foreign_key_violation OR check_violation OR not_null_violation THEN
    NULL;
  END;
  BEGIN
    INSERT INTO xz_personal_point_lot_movements(
      id, lot_id, account_id, user_id, movement_type, points,
      available_before, available_after, reserved_before, reserved_after,
      consumed_before, consumed_after, expired_before, expired_after,
      reversed_before, reversed_after, idempotency_key
    ) VALUES (
      'verify_103_blank_movement_key', 'verify_103_manual_lot_1',
      'verify_103_account', 'verify_103_user', 'GRANT', 1,
      0, 1, 0, 0, 0, 0, 0, 0, 0, 0, '   '
    );
    RAISE EXCEPTION 'blank movement idempotency unexpectedly accepted';
  EXCEPTION WHEN foreign_key_violation OR check_violation OR not_null_violation THEN
    NULL;
  END;
END;
$$;
ROLLBACK TO SAVEPOINT migration_103_ownership_probes;

SAVEPOINT migration_103_append_only_probe;
INSERT INTO xz_personal_point_lots(
  id, account_id, user_id, source_type, original_points, available_points,
  reserved_points, consumed_points, expired_points, reversed_points,
  granted_at, idempotency_key
) VALUES (
  'verify_103_append_only_lot', 'verify_103_account', 'verify_103_user',
  'MANUAL', 1, 1, 0, 0, 0, 0, now(), 'verify_103_append_only_lot'
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
  BEGIN
    DELETE FROM xz_personal_point_lot_movements
    WHERE id = 'verify_103_append_only_movement';
    RAISE EXCEPTION 'lot movement delete unexpectedly accepted';
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
