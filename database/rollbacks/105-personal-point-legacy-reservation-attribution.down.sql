-- Remove only unsettled attribution records created by migration 105.
-- Once any attributed reservation has reached a terminal/partial state, the
-- lot and wallet histories have changed and rollback must fail closed.

BEGIN;

DO $$
BEGIN
  IF to_regclass('public.xz_generation_tasks') IS NULL
     OR to_regclass('public.xz_point_accounts') IS NULL
     OR to_regclass('public.xz_wallet_ledger') IS NULL
     OR to_regclass('public.xz_personal_point_lots') IS NULL
     OR to_regclass('public.xz_personal_point_reservations') IS NULL
     OR to_regclass('public.xz_personal_point_reservation_allocations') IS NULL
     OR to_regclass('public.xz_personal_point_lot_movements') IS NULL THEN
    RAISE EXCEPTION 'rollback 105 requires the personal point attribution schema';
  END IF;
END;
$$;

LOCK TABLE xz_generation_tasks IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE xz_point_accounts IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE xz_wallet_ledger IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE xz_personal_point_lots IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE xz_personal_point_reservations IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE xz_personal_point_reservation_allocations IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE xz_personal_point_lot_movements IN SHARE ROW EXCLUSIVE MODE;

CREATE TEMP TABLE rollback_105_reservations
ON COMMIT DROP
AS
SELECT
  reservation.id AS reservation_id,
  reservation.account_id,
  reservation.user_id,
  reservation.business_id AS task_id,
  reservation.requested_points,
  'personal_point_lot_legacy_' || substr(md5(reservation.account_id), 1, 24) AS lot_id
FROM xz_personal_point_reservations reservation
WHERE reservation.metadata->>'migration'
  = '105-personal-point-legacy-reservation-attribution';

DO $$
DECLARE
  settled_count BIGINT;
BEGIN
  SELECT count(*)
  INTO settled_count
  FROM rollback_105_reservations attributed
  JOIN xz_personal_point_reservations reservation
    ON reservation.id = attributed.reservation_id
  WHERE reservation.status <> 'RESERVED'
     OR reservation.reserved_points <> reservation.requested_points
     OR reservation.captured_points <> 0
     OR reservation.released_points <> 0
     OR reservation.expired_points <> 0;

  IF settled_count > 0 THEN
    RAISE EXCEPTION
      'rollback 105 refuses % settled or partially settled reservation(s)',
      settled_count;
  END IF;

  SELECT count(*)
  INTO settled_count
  FROM rollback_105_reservations attributed
  LEFT JOIN xz_generation_tasks task ON task.id = attributed.task_id
  WHERE task.id IS NULL
     OR task.user_id IS DISTINCT FROM attributed.user_id
     OR upper(coalesce(task.status, '')) NOT IN ('PENDING', 'QUEUED', 'RUNNING', 'PROCESSING', 'RETRYING')
     OR upper(coalesce(task.billing_status, '')) <> 'RESERVED'
     OR task.reserved_points <> attributed.requested_points::numeric
     OR task.captured_points <> 0
     OR task.released_points <> 0
     OR task.refunded_points <> 0
     OR task.params->'billingReserved' IS DISTINCT FROM 'true'::jsonb
     OR coalesce(task.params->'billingRefunded', 'false'::jsonb) = 'true'::jsonb
     OR task.raw->>'billingEngine' IS DISTINCT FROM 'PERSONAL_LOT_V1'
     OR task.raw->>'personalPointAccountId' IS DISTINCT FROM attributed.account_id
     OR task.raw->>'personalPointReservationId' IS DISTINCT FROM attributed.reservation_id;

  IF settled_count > 0 THEN
    RAISE EXCEPTION
      'rollback 105 refuses % attributed task(s) with missing or drifted marker ownership',
      settled_count;
  END IF;

  SELECT count(*)
  INTO settled_count
  FROM rollback_105_reservations attributed
  LEFT JOIN LATERAL (
    SELECT
      count(*) AS allocation_count,
      count(*) FILTER (
        WHERE allocation.status = 'RESERVED'
          AND allocation.allocated_points = attributed.requested_points
          AND allocation.reserved_points = attributed.requested_points
          AND allocation.captured_points = 0
          AND allocation.released_points = 0
          AND allocation.expired_points = 0
          AND allocation.account_id = attributed.account_id
          AND allocation.user_id = attributed.user_id
          AND allocation.lot_id = attributed.lot_id
          AND allocation.metadata->>'migration'
            = '105-personal-point-legacy-reservation-attribution'
      ) AS valid_count
    FROM xz_personal_point_reservation_allocations allocation
    WHERE allocation.reservation_id = attributed.reservation_id
  ) allocations ON TRUE
  WHERE allocations.allocation_count <> 1
     OR allocations.valid_count <> 1;

  IF settled_count > 0 THEN
    RAISE EXCEPTION
      'rollback 105 refuses % reservation(s) with settled or drifted allocation',
      settled_count;
  END IF;

  SELECT count(DISTINCT attributed.reservation_id)
  INTO settled_count
  FROM rollback_105_reservations attributed
  JOIN xz_personal_point_lot_movements movement
    ON movement.reservation_id = attributed.reservation_id;

  IF settled_count > 0 THEN
    RAISE EXCEPTION
      'rollback 105 refuses % reservation(s) with terminal lot movement history',
      settled_count;
  END IF;

  SELECT count(DISTINCT attributed.reservation_id)
  INTO settled_count
  FROM rollback_105_reservations attributed
  JOIN xz_wallet_ledger ledger ON ledger.task_id = attributed.task_id
  WHERE upper(ledger.entry_type) IN ('CAPTURE', 'RELEASE');

  IF settled_count > 0 THEN
    RAISE EXCEPTION
      'rollback 105 refuses % reservation(s) with terminal wallet history',
      settled_count;
  END IF;

  SELECT count(*)
  INTO settled_count
  FROM (
    SELECT account.id
    FROM xz_point_accounts account
    JOIN (
      SELECT DISTINCT account_id, lot_id
      FROM rollback_105_reservations
    ) attributed ON attributed.account_id = account.id
    JOIN xz_personal_point_lots lot
      ON lot.id = attributed.lot_id
    LEFT JOIN LATERAL (
      SELECT
        coalesce(sum(candidate.available_points), 0) AS available_points,
        coalesce(sum(candidate.reserved_points), 0) AS reserved_points
      FROM xz_personal_point_lots candidate
      WHERE candidate.account_id = account.id
    ) lots ON TRUE
    LEFT JOIN LATERAL (
      SELECT coalesce(sum(allocation.reserved_points), 0) AS reserved_points
      FROM xz_personal_point_reservation_allocations allocation
      WHERE allocation.account_id = account.id
    ) allocations ON TRUE
    WHERE account.available <> lot.available_points
       OR account.available <> lots.available_points
       OR account.frozen <> lot.reserved_points
       OR account.frozen <> lots.reserved_points
       OR account.frozen <> allocations.reserved_points
  ) mismatch;

  IF settled_count > 0 THEN
    RAISE EXCEPTION
      'rollback 105 refuses % account(s) with changed frozen projection',
      settled_count;
  END IF;
END;
$$;

UPDATE xz_generation_tasks task
SET raw = task.raw
          - 'billingEngine'
          - 'personalPointAccountId'
          - 'personalPointReservationId'
FROM rollback_105_reservations attributed
WHERE task.id = attributed.task_id;

DELETE FROM xz_personal_point_reservation_allocations allocation
USING rollback_105_reservations attributed
WHERE allocation.reservation_id = attributed.reservation_id;

DELETE FROM xz_personal_point_reservations reservation
USING rollback_105_reservations attributed
WHERE reservation.id = attributed.reservation_id;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM xz_personal_point_reservations
    WHERE metadata->>'migration'
      = '105-personal-point-legacy-reservation-attribution'
  ) THEN
    RAISE EXCEPTION 'rollback 105 reservation cleanup postcondition failed';
  END IF;
END;
$$;

COMMIT;
