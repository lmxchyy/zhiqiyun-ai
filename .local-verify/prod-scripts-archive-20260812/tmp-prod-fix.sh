#!/bin/bash
set -eu
PSQL="docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -v ON_ERROR_STOP=1"

echo "=== BEFORE ==="
$PSQL -c "
SELECT a.id AS account_id, a.available AS account_available, a.frozen AS account_frozen,
       l.id AS lot_id, l.available_points AS lot_available, l.consumed_points AS lot_consumed, l.status
FROM xz_point_accounts a
JOIN xz_personal_point_lots l ON l.account_id=a.id AND l.user_id=a.user_id
WHERE a.user_id='user_000003';
"

$PSQL <<'SQL'
BEGIN;

UPDATE xz_personal_point_lots
SET
  available_points = available_points - 1800,
  consumed_points  = consumed_points + 1800,
  updated_at = now()
WHERE account_id = 'points_000003'
  AND user_id = 'user_000003'
  AND source_type = 'LEGACY'
  AND available_points = 2999
  AND consumed_points = 1800;

INSERT INTO xz_personal_point_lot_movements (
  id, lot_id, account_id, user_id, movement_type,
  points,
  available_before, available_after,
  reserved_before, reserved_after,
  consumed_before, consumed_after,
  expired_before, expired_after,
  reversed_before, reversed_after,
  reference_type, reference_id, reservation_id, idempotency_key,
  metadata, created_at
)
SELECT
  'movement_' || substr(md5(l.id || ':drift-fix-1800'), 1, 32),
  l.id, l.account_id, l.user_id, 'CAPTURE',
  1800,
  l.available_points + 1800, l.available_points,
  0, 0,
  l.consumed_points - 1800, l.consumed_points,
  0, 0,
  0, 0,
  'DRIFT_CORRECTION', 'task_000122+task_000123', NULL,
  'drift-fix:' || l.id || ':1800',
  '{"reason":"sync lot consumed to match account after legacy video tasks task_000122(600)+task_000123(1200)","operator":"admin"}'::jsonb,
  now()
FROM xz_personal_point_lots l
WHERE l.account_id = 'points_000003'
  AND l.user_id = 'user_000003'
  AND l.source_type = 'LEGACY'
ON CONFLICT (lot_id, idempotency_key) DO NOTHING;

-- verify inside tx
DO $$
DECLARE
  acct bigint;
  lots bigint;
BEGIN
  SELECT available INTO acct FROM xz_point_accounts WHERE id='points_000003';
  SELECT COALESCE(SUM(available_points),0) INTO lots FROM xz_personal_point_lots WHERE account_id='points_000003' AND user_id='user_000003';
  IF acct IS DISTINCT FROM lots THEN
    RAISE EXCEPTION 'drift still present account=% lots=%', acct, lots;
  END IF;
END $$;

COMMIT;
SQL

echo "=== AFTER ==="
$PSQL -c "
SELECT a.available AS account_avail,
       COALESCE(SUM(l.available_points),0) AS lots_avail,
       a.available - COALESCE(SUM(l.available_points),0) AS drift,
       MAX(l.consumed_points) AS lot_consumed
FROM xz_point_accounts a
JOIN xz_personal_point_lots l ON l.account_id=a.id AND l.user_id=a.user_id
WHERE a.id='points_000003'
GROUP BY a.id, a.available;
"