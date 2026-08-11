-- Post-migration verification for legacy frozen personal generation tasks.
-- The PowerShell gate seeds two old, active tasks before migration 103, then
-- applies 103/104 and finally migration 105. This fixture asserts observable
-- economic state rather than migration source text.

\set ON_ERROR_STOP on

DO $$
DECLARE
  account_available BIGINT;
  account_frozen BIGINT;
  lot_available BIGINT;
  lot_reserved BIGINT;
  allocation_reserved BIGINT;
BEGIN
  SELECT available, frozen
  INTO account_available, account_frozen
  FROM xz_point_accounts
  WHERE id = 'acct_valid';

  IF account_available IS DISTINCT FROM 7 OR account_frozen IS DISTINCT FROM 5 THEN
    RAISE EXCEPTION
      'migration 105 changed aggregate account balance: available %, frozen %',
      account_available, account_frozen;
  END IF;

  SELECT available_points, reserved_points
  INTO lot_available, lot_reserved
  FROM xz_personal_point_lots
  WHERE id = 'personal_point_lot_legacy_' || substr(md5('acct_valid'), 1, 24);

  IF lot_available IS DISTINCT FROM 7 OR lot_reserved IS DISTINCT FROM 5 THEN
    RAISE EXCEPTION
      'migration 105 changed LEGACY lot balance: available %, reserved %',
      lot_available, lot_reserved;
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM xz_personal_point_lots
    WHERE id = 'personal_point_lot_legacy_' || substr(md5('acct_valid'), 1, 24)
      AND policy_snapshot->'legacyAvailable' = '7'::jsonb
      AND policy_snapshot->'legacyFrozen' = '5'::jsonb
  ) THEN
    RAISE EXCEPTION 'migration 105 accepted drifted migration 103 LEGACY snapshot';
  END IF;

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
    WHERE account.id = 'acct_valid'
      AND (
        account.available <> lots.available_points
        OR account.frozen <> lots.reserved_points
      )
  ) THEN
    RAISE EXCEPTION 'migration 105 account/lot available or frozen projection mismatch';
  END IF;

  IF (SELECT count(*) FROM xz_personal_point_reservations
      WHERE metadata->>'migration' = '105-personal-point-legacy-reservation-attribution') <> 2 THEN
    RAISE EXCEPTION 'migration 105 did not create exactly two attributed reservations';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM xz_personal_point_reservations
    WHERE id = 'personal_point_reservation_legacy_d62bb9c78f2e0b6e1923ca55'
      AND account_id = 'acct_valid'
      AND user_id = 'user_valid'
      AND business_type = 'GENERATION_TASK'
      AND business_id = 'task_valid_a'
      AND requested_points = 2
      AND reserved_points = 2
      AND captured_points = 0
      AND released_points = 0
      AND expired_points = 0
      AND idempotency_key = 'generation:reserve:task_valid_a'
      AND status = 'RESERVED'
      AND metadata->>'legacyReserveLedgerId' = 'ledger_valid_a'
  ) THEN
    RAISE EXCEPTION 'migration 105 task A reservation attribution is invalid';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM xz_personal_point_reservations
    WHERE id = 'personal_point_reservation_legacy_e6a627a6adca54eaa0272697'
      AND account_id = 'acct_valid'
      AND user_id = 'user_valid'
      AND business_type = 'GENERATION_TASK'
      AND business_id = 'task_valid_b'
      AND requested_points = 3
      AND reserved_points = 3
      AND idempotency_key = 'generation:reserve:task_valid_b'
      AND status = 'RESERVED'
      AND metadata->>'legacyReserveLedgerId' = 'ledger_valid_b'
  ) THEN
    RAISE EXCEPTION 'migration 105 task B reservation attribution is invalid';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM xz_personal_point_reservation_allocations allocation
    WHERE allocation.id = 'personal_point_allocation_legacy_d62bb9c78f2e0b6e1923ca55'
      AND allocation.reservation_id = 'personal_point_reservation_legacy_d62bb9c78f2e0b6e1923ca55'
      AND allocation.lot_id = 'personal_point_lot_legacy_' || substr(md5('acct_valid'), 1, 24)
      AND allocation.account_id = 'acct_valid'
      AND allocation.user_id = 'user_valid'
      AND allocation.allocated_points = 2
      AND allocation.reserved_points = 2
      AND allocation.captured_points = 0
      AND allocation.released_points = 0
      AND allocation.expired_points = 0
      AND allocation.status = 'RESERVED'
      AND allocation.metadata->>'legacyReserveLedgerId' = 'ledger_valid_a'
  ) THEN
    RAISE EXCEPTION 'migration 105 task A allocation attribution is invalid';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM xz_personal_point_reservation_allocations allocation
    WHERE allocation.id = 'personal_point_allocation_legacy_e6a627a6adca54eaa0272697'
      AND allocation.reservation_id = 'personal_point_reservation_legacy_e6a627a6adca54eaa0272697'
      AND allocation.lot_id = 'personal_point_lot_legacy_' || substr(md5('acct_valid'), 1, 24)
      AND allocation.account_id = 'acct_valid'
      AND allocation.user_id = 'user_valid'
      AND allocation.allocated_points = 3
      AND allocation.reserved_points = 3
      AND allocation.status = 'RESERVED'
      AND allocation.metadata->>'legacyReserveLedgerId' = 'ledger_valid_b'
  ) THEN
    RAISE EXCEPTION 'migration 105 task B allocation attribution is invalid';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM (VALUES
      ('task_valid_a', 'acct_valid', 'personal_point_reservation_legacy_d62bb9c78f2e0b6e1923ca55'),
      ('task_valid_b', 'acct_valid', 'personal_point_reservation_legacy_e6a627a6adca54eaa0272697')
    ) expected(task_id, account_id, reservation_id)
    LEFT JOIN xz_generation_tasks task ON task.id = expected.task_id
    WHERE task.id IS NULL
       OR task.raw->>'billingEngine' IS DISTINCT FROM 'PERSONAL_LOT_V1'
       OR task.raw->>'personalPointAccountId' IS DISTINCT FROM expected.account_id
       OR task.raw->>'personalPointReservationId' IS DISTINCT FROM expected.reservation_id
  ) THEN
    RAISE EXCEPTION 'migration 105 task marker attribution is invalid';
  END IF;

  SELECT coalesce(sum(reserved_points), 0)
  INTO allocation_reserved
  FROM xz_personal_point_reservation_allocations
  WHERE account_id = 'acct_valid';

  IF allocation_reserved IS DISTINCT FROM 5 THEN
    RAISE EXCEPTION
      'migration 105 allocation projection mismatch: reserved %',
      allocation_reserved;
  END IF;

  IF (SELECT count(*) FROM xz_personal_point_lot_movements
      WHERE account_id = 'acct_valid' AND movement_type = 'RESERVE') <> 0 THEN
    RAISE EXCEPTION 'migration 105 fabricated a duplicate RESERVE lot movement';
  END IF;

  IF (SELECT count(*) FROM xz_wallet_ledger
      WHERE account_id = 'acct_valid' AND entry_type = 'RESERVE') <> 2 THEN
    RAISE EXCEPTION 'migration 105 rewrote the historical wallet reserve evidence';
  END IF;
END;
$$;
