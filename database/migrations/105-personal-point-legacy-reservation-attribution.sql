-- Attribute pre-lot in-flight personal generation reservations to the LEGACY
-- lot created by migration 103.
--
-- Migration 103 deliberately preserved frozen aggregate balances, but an old
-- generation task could still finish through the aggregate-only terminal path.
-- This migration uses the immutable historical wallet RESERVE row as evidence,
-- creates only the missing reservation/allocation ownership records, and marks
-- the task for the lot-aware terminal path. No economic balance or movement is
-- created or rewritten here.

BEGIN;

DO $$
DECLARE
  required_table TEXT;
BEGIN
  FOREACH required_table IN ARRAY ARRAY[
    'xz_generation_tasks',
    'xz_point_accounts',
    'xz_wallet_ledger',
    'xz_personal_point_lots',
    'xz_personal_point_reservations',
    'xz_personal_point_reservation_allocations',
    'xz_personal_point_lot_movements'
  ] LOOP
    IF to_regclass('public.' || required_table) IS NULL THEN
      RAISE EXCEPTION 'migration 105 requires %', required_table;
    END IF;
  END LOOP;
END;
$$;

-- A coherent snapshot is mandatory. Concurrent generation terminal writes or
-- wallet updates must wait until attribution has committed or failed.
LOCK TABLE xz_generation_tasks IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE xz_point_accounts IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE xz_wallet_ledger IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE xz_personal_point_lots IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE xz_personal_point_reservations IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE xz_personal_point_reservation_allocations IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE xz_personal_point_lot_movements IN SHARE ROW EXCLUSIVE MODE;

CREATE TEMP TABLE migration_105_active_scope
ON COMMIT DROP
AS
SELECT
  task.id AS task_id,
  upper(coalesce(nullif(btrim(task.billing_account_type), ''), 'PERSONAL')) AS column_scope,
  upper(coalesce(nullif(btrim(task.params->>'billing_scope'), ''), '')) AS params_scope,
  upper(coalesce(nullif(btrim(task.raw->'params'->>'billing_scope'), ''), '')) AS raw_params_scope,
  upper(coalesce(nullif(btrim(task.raw->>'billingAccountType'), ''), '')) AS raw_account_scope
FROM xz_generation_tasks task
WHERE upper(coalesce(task.status, '')) IN ('PENDING', 'QUEUED', 'RUNNING', 'PROCESSING', 'RETRYING');

DO $$
DECLARE
  conflict_count BIGINT;
BEGIN
  SELECT count(*)
  INTO conflict_count
  FROM migration_105_active_scope scope
  WHERE (
    SELECT count(DISTINCT value)
    FROM unnest(ARRAY[
      scope.column_scope,
      scope.params_scope,
      scope.raw_params_scope,
      scope.raw_account_scope
    ]) value
    WHERE value <> ''
  ) > 1;

  IF conflict_count > 0 THEN
    RAISE EXCEPTION
      'migration 105 refuses % active reserved task(s) with conflicting billing scope',
      conflict_count;
  END IF;

  SELECT count(*)
  INTO conflict_count
  FROM migration_105_active_scope scope
  WHERE EXISTS (
    SELECT 1
    FROM unnest(ARRAY[
      scope.column_scope,
      scope.params_scope,
      scope.raw_params_scope,
      scope.raw_account_scope
    ]) value
    WHERE value <> ''
      AND value NOT IN ('PERSONAL', 'ENTERPRISE')
  );

  IF conflict_count > 0 THEN
    RAISE EXCEPTION
      'migration 105 refuses % active task(s) with unknown billing scope',
      conflict_count;
  END IF;
END;
$$;

CREATE TEMP TABLE migration_105_personal_tasks
ON COMMIT DROP
AS
SELECT
  task.id AS task_id,
  task.user_id,
  task.status,
  task.billing_status,
  task.point_cost,
  task.params,
  task.raw,
  task.reserved_points,
  task.captured_points,
  task.released_points,
  task.refunded_points,
  coalesce(nullif(btrim(task.raw->>'billingEngine'), ''), '') AS marker_engine,
  coalesce(nullif(btrim(task.raw->>'personalPointAccountId'), ''), '') AS marker_account_id,
  coalesce(nullif(btrim(task.raw->>'personalPointReservationId'), ''), '') AS marker_reservation_id
FROM xz_generation_tasks task
JOIN migration_105_active_scope scope ON scope.task_id = task.id
WHERE scope.column_scope <> 'ENTERPRISE';

DO $$
DECLARE
  invalid_count BIGINT;
BEGIN
  SELECT count(*)
  INTO invalid_count
  FROM migration_105_personal_tasks task
  WHERE nullif(btrim(coalesce(task.user_id, '')), '') IS NULL
     OR upper(coalesce(task.billing_status, '')) <> 'RESERVED'
     OR task.point_cost <= 0
     OR task.reserved_points <= 0
     OR task.reserved_points <> trunc(task.reserved_points)
     OR task.reserved_points <> task.point_cost::numeric
     OR task.captured_points <> 0
     OR task.released_points <> 0
     OR task.refunded_points <> 0
     OR jsonb_typeof(task.params) IS DISTINCT FROM 'object'
     OR task.params->'billingReserved' IS DISTINCT FROM 'true'::jsonb
     OR coalesce(task.params->'billingRefunded', 'false'::jsonb) = 'true'::jsonb
     OR jsonb_typeof(task.params->'billingReservationPointCost') IS DISTINCT FROM 'number'
     OR CASE
          WHEN jsonb_typeof(task.params->'billingReservationPointCost') = 'number'
          THEN (task.params->>'billingReservationPointCost')::numeric
          ELSE NULL
        END <> task.point_cost::numeric
     OR CASE
          WHEN jsonb_typeof(task.params->'billingReservationPointCost') = 'number'
          THEN (task.params->>'billingReservationPointCost')::numeric
          ELSE NULL
        END <> trunc(
          CASE
            WHEN jsonb_typeof(task.params->'billingReservationPointCost') = 'number'
            THEN (task.params->>'billingReservationPointCost')::numeric
            ELSE NULL
          END
        )
     OR jsonb_typeof(task.raw) IS DISTINCT FROM 'object'
     OR task.raw->>'id' IS DISTINCT FROM task.task_id
     OR task.raw->>'userId' IS DISTINCT FROM task.user_id
     OR upper(coalesce(task.raw->>'status', '')) IS DISTINCT FROM upper(task.status)
     OR jsonb_typeof(task.raw->'pointCost') IS DISTINCT FROM 'number'
     OR CASE
          WHEN jsonb_typeof(task.raw->'pointCost') = 'number'
          THEN (task.raw->>'pointCost')::numeric
          ELSE NULL
        END <> task.point_cost::numeric
     OR (
          task.raw ? 'billingStatus'
          AND upper(coalesce(task.raw->>'billingStatus', ''))
                IS DISTINCT FROM upper(task.billing_status)
        )
     OR (
          task.raw ? 'reservedPoints'
          AND (
            jsonb_typeof(task.raw->'reservedPoints') IS DISTINCT FROM 'number'
            OR CASE
                 WHEN jsonb_typeof(task.raw->'reservedPoints') = 'number'
                 THEN (task.raw->>'reservedPoints')::numeric
                 ELSE NULL
               END <> task.reserved_points
          )
        )
     OR (
          task.raw ? 'capturedPoints'
          AND (
            jsonb_typeof(task.raw->'capturedPoints') IS DISTINCT FROM 'number'
            OR CASE
                 WHEN jsonb_typeof(task.raw->'capturedPoints') = 'number'
                 THEN (task.raw->>'capturedPoints')::numeric
                 ELSE NULL
               END <> task.captured_points
          )
        )
     OR (
          task.raw ? 'releasedPoints'
          AND (
            jsonb_typeof(task.raw->'releasedPoints') IS DISTINCT FROM 'number'
            OR CASE
                 WHEN jsonb_typeof(task.raw->'releasedPoints') = 'number'
                 THEN (task.raw->>'releasedPoints')::numeric
                 ELSE NULL
               END <> task.released_points
          )
        )
     OR (
          task.raw ? 'refundedPoints'
          AND (
            jsonb_typeof(task.raw->'refundedPoints') IS DISTINCT FROM 'number'
            OR CASE
                 WHEN jsonb_typeof(task.raw->'refundedPoints') = 'number'
                 THEN (task.raw->>'refundedPoints')::numeric
                 ELSE NULL
               END <> task.refunded_points
          )
        )
     OR jsonb_typeof(task.raw->'params') IS DISTINCT FROM 'object'
     OR task.raw->'params'->'billingReserved'
          IS DISTINCT FROM task.params->'billingReserved'
     OR coalesce(task.raw->'params'->'billingRefunded', 'false'::jsonb)
          IS DISTINCT FROM coalesce(task.params->'billingRefunded', 'false'::jsonb)
     OR task.raw->'params'->'billingReservationPointCost'
          IS DISTINCT FROM task.params->'billingReservationPointCost'
     OR coalesce(task.raw->'params'->'billing_scope', 'null'::jsonb)
          IS DISTINCT FROM coalesce(task.params->'billing_scope', 'null'::jsonb)
     OR coalesce(task.raw->'params'->'billing_account_id', 'null'::jsonb)
          IS DISTINCT FROM coalesce(task.params->'billing_account_id', 'null'::jsonb);

  IF invalid_count > 0 THEN
    RAISE EXCEPTION
      'migration 105 refuses % active personal task(s) with invalid or drifted reservation evidence',
      invalid_count;
  END IF;

  SELECT count(*)
  INTO invalid_count
  FROM migration_105_personal_tasks task
  WHERE NOT (
    (
      task.marker_engine = ''
      AND task.marker_account_id = ''
      AND task.marker_reservation_id = ''
    )
    OR (
      task.marker_engine = 'PERSONAL_LOT_V1'
      AND task.marker_account_id <> ''
      AND task.marker_reservation_id <> ''
    )
  );

  IF invalid_count > 0 THEN
    RAISE EXCEPTION
      'migration 105 refuses % active personal task(s) with partial or unknown lot marker',
      invalid_count;
  END IF;
END;
$$;

DO $$
DECLARE
  invalid_count BIGINT;
BEGIN
  SELECT count(*)
  INTO invalid_count
  FROM (
    SELECT task.task_id
    FROM migration_105_personal_tasks task
    LEFT JOIN xz_wallet_ledger ledger
      ON ledger.task_id = task.task_id
     AND upper(ledger.entry_type) = 'RESERVE'
    GROUP BY task.task_id
    HAVING count(ledger.id) <> 1
  ) invalid;

  IF invalid_count > 0 THEN
    RAISE EXCEPTION
      'migration 105 requires exactly one historical RESERVE ledger for % task(s)',
      invalid_count;
  END IF;

  SELECT count(DISTINCT task.task_id)
  INTO invalid_count
  FROM migration_105_personal_tasks task
  JOIN xz_wallet_ledger ledger ON ledger.task_id = task.task_id
  WHERE upper(ledger.entry_type) IN ('CAPTURE', 'RELEASE');

  IF invalid_count > 0 THEN
    RAISE EXCEPTION
      'migration 105 refuses % task(s) with terminal wallet evidence',
      invalid_count;
  END IF;
END;
$$;

CREATE TEMP TABLE migration_105_attribution
ON COMMIT DROP
AS
SELECT
  task.task_id,
  task.user_id,
  task.point_cost::BIGINT AS reserved_points,
  task.marker_engine,
  task.marker_account_id,
  task.marker_reservation_id,
  ledger.id AS ledger_id,
  ledger.account_id,
  ledger.created_at AS reserved_at,
  'personal_point_lot_legacy_' || substr(md5(ledger.account_id), 1, 24) AS lot_id,
  'personal_point_reservation_legacy_' ||
    substr(md5(ledger.account_id || ':' || task.task_id), 1, 24) AS reservation_id,
  'personal_point_allocation_legacy_' ||
    substr(md5(ledger.account_id || ':' || task.task_id), 1, 24) AS allocation_id
FROM migration_105_personal_tasks task
JOIN xz_wallet_ledger ledger
  ON ledger.task_id = task.task_id
 AND upper(ledger.entry_type) = 'RESERVE';

DO $$
DECLARE
  invalid_count BIGINT;
BEGIN
  SELECT count(*)
  INTO invalid_count
  FROM migration_105_attribution attribution
  JOIN xz_wallet_ledger ledger ON ledger.id = attribution.ledger_id
  LEFT JOIN xz_point_accounts account ON account.id = ledger.account_id
  WHERE account.id IS NULL
     OR account.user_id IS DISTINCT FROM attribution.user_id
     OR ledger.user_id IS DISTINCT FROM attribution.user_id
     OR ledger.points <= 0
     OR ledger.points <> trunc(ledger.points)
     OR ledger.points <> attribution.reserved_points::numeric
     OR ledger.available_after <> ledger.available_before - ledger.points
     OR ledger.frozen_after <> ledger.frozen_before + ledger.points
     OR ledger.idempotency_key IS DISTINCT FROM attribution.task_id || ':RESERVE'
     OR upper(coalesce(ledger.reference_type, '')) IS DISTINCT FROM 'GENERATION_TASK'
     OR ledger.reference_id IS DISTINCT FROM attribution.task_id;

  IF invalid_count > 0 THEN
    RAISE EXCEPTION
      'migration 105 refuses % task(s) with invalid historical RESERVE ledger ownership or transition',
      invalid_count;
  END IF;

  SELECT count(*)
  INTO invalid_count
  FROM migration_105_attribution attribution
  JOIN xz_point_accounts account ON account.id = attribution.account_id
  LEFT JOIN xz_personal_point_lots lot ON lot.id = attribution.lot_id
  WHERE lot.id IS NULL
     OR lot.account_id IS DISTINCT FROM account.id
     OR lot.user_id IS DISTINCT FROM account.user_id
     OR lot.source_type IS DISTINCT FROM 'LEGACY'
     OR lot.status IS DISTINCT FROM 'LEGACY'
     OR lot.available_points IS DISTINCT FROM account.available
     OR lot.reserved_points IS DISTINCT FROM account.frozen
     OR jsonb_typeof(lot.policy_snapshot->'legacyAvailable') IS DISTINCT FROM 'number'
     OR CASE
          WHEN jsonb_typeof(lot.policy_snapshot->'legacyAvailable') = 'number'
          THEN (lot.policy_snapshot->>'legacyAvailable')::numeric
          ELSE NULL
        END <> account.available::numeric
     OR jsonb_typeof(lot.policy_snapshot->'legacyFrozen') IS DISTINCT FROM 'number'
     OR CASE
          WHEN jsonb_typeof(lot.policy_snapshot->'legacyFrozen') = 'number'
          THEN (lot.policy_snapshot->>'legacyFrozen')::numeric
          ELSE NULL
        END <> account.frozen::numeric;

  IF invalid_count > 0 THEN
    RAISE EXCEPTION
      'migration 105 refuses % task(s) without an exact migration 103 LEGACY lot',
      invalid_count;
  END IF;

  SELECT count(*)
  INTO invalid_count
  FROM (
    SELECT account.id
    FROM xz_point_accounts account
    LEFT JOIN migration_105_attribution attribution
      ON attribution.account_id = account.id
    WHERE account.frozen > 0
    GROUP BY account.id, account.frozen
    HAVING count(attribution.task_id) = 0
       OR coalesce(sum(attribution.reserved_points), 0) <> account.frozen
  ) mismatch;

  IF invalid_count > 0 THEN
    RAISE EXCEPTION
      'migration 105 frozen balance cannot be closed by active personal tasks for % account(s)',
      invalid_count;
  END IF;

  SELECT count(*)
  INTO invalid_count
  FROM (
    SELECT account.id
    FROM xz_point_accounts account
    JOIN (
      SELECT DISTINCT account_id
      FROM migration_105_attribution
    ) attributed ON attributed.account_id = account.id
    LEFT JOIN LATERAL (
      SELECT
        coalesce(sum(lot.available_points), 0) AS available_points,
        coalesce(sum(lot.reserved_points), 0) AS reserved_points
      FROM xz_personal_point_lots lot
      WHERE lot.account_id = account.id
    ) lots ON TRUE
    WHERE lots.available_points <> account.available
       OR lots.reserved_points <> account.frozen
  ) mismatch;

  IF invalid_count > 0 THEN
    RAISE EXCEPTION
      'migration 105 lot reserved projection mismatch for % account(s)',
      invalid_count;
  END IF;
END;
$$;

DO $$
DECLARE
  collision_count BIGINT;
BEGIN
  SELECT count(DISTINCT attribution.task_id)
  INTO collision_count
  FROM migration_105_attribution attribution
  JOIN xz_personal_point_reservations reservation
    ON reservation.id = attribution.reservation_id
    OR (
      reservation.account_id = attribution.account_id
      AND reservation.idempotency_key = 'generation:reserve:' || attribution.task_id
    )
    OR (
      reservation.account_id = attribution.account_id
      AND reservation.business_type = 'GENERATION_TASK'
      AND reservation.business_id = attribution.task_id
    )
  WHERE attribution.marker_engine = '';

  IF collision_count > 0 THEN
    RAISE EXCEPTION
      'migration 105 refuses reservation identity collision for % unresolved task(s)',
      collision_count;
  END IF;

  SELECT count(DISTINCT attribution.task_id)
  INTO collision_count
  FROM migration_105_attribution attribution
  JOIN xz_personal_point_reservation_allocations allocation
    ON allocation.id = attribution.allocation_id
    OR (
      allocation.reservation_id = attribution.reservation_id
      AND allocation.lot_id = attribution.lot_id
    )
  WHERE attribution.marker_engine = '';

  IF collision_count > 0 THEN
    RAISE EXCEPTION
      'migration 105 refuses allocation identity collision for % unresolved task(s)',
      collision_count;
  END IF;

  SELECT count(*)
  INTO collision_count
  FROM migration_105_attribution attribution
  LEFT JOIN xz_personal_point_reservations reservation
    ON reservation.id = attribution.reservation_id
  LEFT JOIN xz_personal_point_reservation_allocations allocation
    ON allocation.id = attribution.allocation_id
  WHERE attribution.marker_engine = 'PERSONAL_LOT_V1'
    AND (
      attribution.marker_account_id IS DISTINCT FROM attribution.account_id
      OR attribution.marker_reservation_id IS DISTINCT FROM attribution.reservation_id
      OR reservation.id IS NULL
      OR reservation.account_id IS DISTINCT FROM attribution.account_id
      OR reservation.user_id IS DISTINCT FROM attribution.user_id
      OR reservation.business_type IS DISTINCT FROM 'GENERATION_TASK'
      OR reservation.business_id IS DISTINCT FROM attribution.task_id
      OR reservation.requested_points IS DISTINCT FROM attribution.reserved_points
      OR reservation.reserved_points IS DISTINCT FROM attribution.reserved_points
      OR reservation.captured_points <> 0
      OR reservation.released_points <> 0
      OR reservation.expired_points <> 0
      OR reservation.idempotency_key IS DISTINCT FROM 'generation:reserve:' || attribution.task_id
      OR reservation.status IS DISTINCT FROM 'RESERVED'
      OR reservation.metadata->>'migration'
           IS DISTINCT FROM '105-personal-point-legacy-reservation-attribution'
      OR reservation.metadata->>'legacyReserveLedgerId' IS DISTINCT FROM attribution.ledger_id
      OR allocation.id IS NULL
      OR allocation.reservation_id IS DISTINCT FROM attribution.reservation_id
      OR allocation.lot_id IS DISTINCT FROM attribution.lot_id
      OR allocation.account_id IS DISTINCT FROM attribution.account_id
      OR allocation.user_id IS DISTINCT FROM attribution.user_id
      OR allocation.allocated_points IS DISTINCT FROM attribution.reserved_points
      OR allocation.reserved_points IS DISTINCT FROM attribution.reserved_points
      OR allocation.captured_points <> 0
      OR allocation.released_points <> 0
      OR allocation.expired_points <> 0
      OR allocation.status IS DISTINCT FROM 'RESERVED'
      OR allocation.metadata->>'migration'
           IS DISTINCT FROM '105-personal-point-legacy-reservation-attribution'
      OR allocation.metadata->>'legacyReserveLedgerId' IS DISTINCT FROM attribution.ledger_id
    );

  IF collision_count > 0 THEN
    RAISE EXCEPTION
      'migration 105 rerun validation failed for % attributed task(s)',
      collision_count;
  END IF;
END;
$$;

INSERT INTO xz_personal_point_reservations(
  id, account_id, user_id, business_type, business_id,
  requested_points, reserved_points, captured_points, released_points,
  expired_points, idempotency_key, status, metadata, created_at, updated_at
)
SELECT
  attribution.reservation_id,
  attribution.account_id,
  attribution.user_id,
  'GENERATION_TASK',
  attribution.task_id,
  attribution.reserved_points,
  attribution.reserved_points,
  0,
  0,
  0,
  'generation:reserve:' || attribution.task_id,
  'RESERVED',
  jsonb_build_object(
    'migration', '105-personal-point-legacy-reservation-attribution',
    'legacyReserveLedgerId', attribution.ledger_id
  ),
  attribution.reserved_at,
  attribution.reserved_at
FROM migration_105_attribution attribution
WHERE attribution.marker_engine = '';

INSERT INTO xz_personal_point_reservation_allocations(
  id, reservation_id, lot_id, account_id, user_id,
  allocated_points, reserved_points, captured_points, released_points,
  expired_points, status, metadata, created_at, updated_at
)
SELECT
  attribution.allocation_id,
  attribution.reservation_id,
  attribution.lot_id,
  attribution.account_id,
  attribution.user_id,
  attribution.reserved_points,
  attribution.reserved_points,
  0,
  0,
  0,
  'RESERVED',
  jsonb_build_object(
    'migration', '105-personal-point-legacy-reservation-attribution',
    'legacyReserveLedgerId', attribution.ledger_id
  ),
  attribution.reserved_at,
  attribution.reserved_at
FROM migration_105_attribution attribution
WHERE attribution.marker_engine = '';

UPDATE xz_generation_tasks task
SET raw = jsonb_set(
            jsonb_set(
              jsonb_set(
                task.raw,
                '{billingEngine}',
                to_jsonb('PERSONAL_LOT_V1'::TEXT),
                TRUE
              ),
              '{personalPointAccountId}',
              to_jsonb(attribution.account_id),
              TRUE
            ),
            '{personalPointReservationId}',
            to_jsonb(attribution.reservation_id),
            TRUE
          )
FROM migration_105_attribution attribution
WHERE task.id = attribution.task_id
  AND attribution.marker_engine = '';

DO $$
DECLARE
  mismatch_count BIGINT;
BEGIN
  SELECT count(*)
  INTO mismatch_count
  FROM migration_105_attribution attribution
  JOIN xz_generation_tasks task ON task.id = attribution.task_id
  LEFT JOIN xz_personal_point_reservations reservation
    ON reservation.id = attribution.reservation_id
  LEFT JOIN xz_personal_point_reservation_allocations allocation
    ON allocation.id = attribution.allocation_id
  WHERE task.raw->>'billingEngine' IS DISTINCT FROM 'PERSONAL_LOT_V1'
     OR task.raw->>'personalPointAccountId' IS DISTINCT FROM attribution.account_id
     OR task.raw->>'personalPointReservationId' IS DISTINCT FROM attribution.reservation_id
     OR reservation.id IS NULL
     OR reservation.account_id IS DISTINCT FROM attribution.account_id
     OR reservation.user_id IS DISTINCT FROM attribution.user_id
     OR reservation.business_type IS DISTINCT FROM 'GENERATION_TASK'
     OR reservation.business_id IS DISTINCT FROM attribution.task_id
     OR reservation.requested_points IS DISTINCT FROM attribution.reserved_points
     OR reservation.reserved_points IS DISTINCT FROM attribution.reserved_points
     OR reservation.status IS DISTINCT FROM 'RESERVED'
     OR reservation.metadata->>'legacyReserveLedgerId' IS DISTINCT FROM attribution.ledger_id
     OR allocation.id IS NULL
     OR allocation.reservation_id IS DISTINCT FROM attribution.reservation_id
     OR allocation.lot_id IS DISTINCT FROM attribution.lot_id
     OR allocation.account_id IS DISTINCT FROM attribution.account_id
     OR allocation.user_id IS DISTINCT FROM attribution.user_id
     OR allocation.allocated_points IS DISTINCT FROM attribution.reserved_points
     OR allocation.reserved_points IS DISTINCT FROM attribution.reserved_points
     OR allocation.status IS DISTINCT FROM 'RESERVED';

  IF mismatch_count > 0 THEN
    RAISE EXCEPTION
      'migration 105 postcondition failed for % task(s)',
      mismatch_count;
  END IF;

  SELECT count(*)
  INTO mismatch_count
  FROM (
    SELECT account.id
    FROM xz_point_accounts account
    JOIN (
      SELECT DISTINCT account_id, lot_id
      FROM migration_105_attribution
    ) attribution ON attribution.account_id = account.id
    JOIN xz_personal_point_lots lot
      ON lot.id = attribution.lot_id
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

  IF mismatch_count > 0 THEN
    RAISE EXCEPTION
      'migration 105 economic projection postcondition failed for % account(s)',
      mismatch_count;
  END IF;
END;
$$;

COMMIT;
