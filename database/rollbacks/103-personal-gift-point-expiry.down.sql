-- Roll back migration 103 only while the personal-lot domain is unused.
--
-- Once a lot movement, reservation, allocation, or lot exists this rollback
-- refuses to run. Removing those rows would destroy economic history; disable
-- the feature and retain the append-only tables instead.

BEGIN;

DO $$
DECLARE
  live_movements BIGINT := 0;
  live_lots BIGINT := 0;
  live_reservations BIGINT := 0;
  live_allocations BIGINT := 0;
BEGIN
  IF to_regclass('public.xz_personal_point_lot_movements') IS NOT NULL THEN
    SELECT count(*) INTO live_movements FROM xz_personal_point_lot_movements;
  END IF;
  IF to_regclass('public.xz_personal_point_lots') IS NOT NULL THEN
    SELECT count(*) INTO live_lots FROM xz_personal_point_lots;
  END IF;
  IF to_regclass('public.xz_personal_point_reservations') IS NOT NULL THEN
    SELECT count(*) INTO live_reservations FROM xz_personal_point_reservations;
  END IF;
  IF to_regclass('public.xz_personal_point_reservation_allocations') IS NOT NULL THEN
    SELECT count(*) INTO live_allocations FROM xz_personal_point_reservation_allocations;
  END IF;

  IF live_movements > 0 OR live_lots > 0 OR live_reservations > 0 OR live_allocations > 0 THEN
    RAISE EXCEPTION
      'migration 103 rollback refused: personal point economic history exists (movements %, lots %, reservations %, allocations %)',
      live_movements, live_lots, live_reservations, live_allocations;
  END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_xz_personal_point_lot_movements_immutable
  ON xz_personal_point_lot_movements;
DROP TRIGGER IF EXISTS trg_xz_personal_point_lots_policy_guard
  ON xz_personal_point_lots;
DROP TRIGGER IF EXISTS trg_xz_point_expiry_policy_versions_immutable
  ON xz_point_expiry_policy_versions;
DROP FUNCTION IF EXISTS xz_reject_personal_point_lot_movement_mutation();
DROP FUNCTION IF EXISTS xz_validate_personal_point_lot_policy();
DROP FUNCTION IF EXISTS xz_reject_published_personal_point_policy_mutation();

DROP TABLE IF EXISTS xz_personal_point_lot_movements;
DROP TABLE IF EXISTS xz_personal_point_reservation_allocations;
DROP TABLE IF EXISTS xz_personal_point_reservations;
DROP TABLE IF EXISTS xz_personal_point_lots;
DROP TABLE IF EXISTS xz_point_expiry_policy_versions;
DROP INDEX IF EXISTS ux_xz_point_accounts_id_user_103;

COMMIT;
