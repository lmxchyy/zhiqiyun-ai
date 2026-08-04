-- Personal gift-point expiry foundation.
--
-- Personal points remain separate from enterprise compute/token quota. The
-- existing xz_point_accounts.available/frozen columns are retained as the
-- compatibility projection; the tables below hold the batch-level source of
-- truth for new lot-aware code. This migration is additive and rerunnable.

BEGIN;

DO $$
BEGIN
  IF to_regclass('public.xz_point_accounts') IS NULL THEN
    RAISE EXCEPTION 'migration 103 requires xz_point_accounts';
  END IF;
END;
$$;

CREATE TABLE IF NOT EXISTS xz_point_expiry_policy_versions (
  id TEXT PRIMARY KEY,
  version BIGINT NOT NULL UNIQUE,
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  duration_value INTEGER NOT NULL DEFAULT 0 CHECK (duration_value >= 0),
  duration_unit TEXT NOT NULL DEFAULT 'CALENDAR_MONTH'
    CHECK (duration_unit IN ('CALENDAR_MONTH', 'DAY')),
  time_zone TEXT NOT NULL DEFAULT 'Asia/Shanghai',
  source_types JSONB NOT NULL DEFAULT '["REGISTRATION_GIFT","ACTIVITY_GIFT","ADMIN_GIFT"]'::jsonb,
  effective_from TIMESTAMPTZ NOT NULL DEFAULT now(),
  effective_to TIMESTAMPTZ,
  status TEXT NOT NULL DEFAULT 'DRAFT'
    CHECK (status IN ('DRAFT', 'PUBLISHED', 'ARCHIVED')),
  created_by TEXT NOT NULL DEFAULT '',
  change_reason TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (
    (enabled = FALSE)
    OR (duration_value > 0 AND duration_unit = 'CALENDAR_MONTH')
  ),
  CHECK (jsonb_typeof(source_types) = 'array'),
  CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE INDEX IF NOT EXISTS idx_xz_point_expiry_policy_versions_active
  ON xz_point_expiry_policy_versions(status, enabled, effective_from DESC, version DESC);

CREATE TABLE IF NOT EXISTS xz_personal_point_lots (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES xz_point_accounts(id),
  user_id TEXT NOT NULL,
  source_type TEXT NOT NULL
    CHECK (source_type IN (
      'REGISTRATION_GIFT', 'ACTIVITY_GIFT', 'ADMIN_GIFT',
      'RECHARGE', 'MEMBERSHIP_GRANT', 'MEMBER_PACKAGE_GRANT',
      'AGENT_GRANT', 'AGENT_JOIN_GRANT', 'OPERATION_CENTER_GRANT',
      'ORDER_GRANT', 'COMMERCE_ORDER', 'UNIFIED_PAYMENT_GRANT',
      'WECHAT_VIRTUAL_ORDER', 'WECHAT_VIRTUAL_COUPON', 'COUPON_GRANT',
      'REFUND', 'RELEASE', 'ADJUSTMENT', 'ADMIN_CORRECTION', 'CORRECTION',
      'LEGACY', 'SYSTEM_DEFAULT', 'REVERSAL', 'MANUAL'
    )),
  reference_type TEXT NOT NULL DEFAULT '',
  reference_id TEXT NOT NULL DEFAULT '',
  original_points BIGINT NOT NULL CHECK (original_points > 0),
  available_points BIGINT NOT NULL DEFAULT 0 CHECK (available_points >= 0),
  reserved_points BIGINT NOT NULL DEFAULT 0 CHECK (reserved_points >= 0),
  consumed_points BIGINT NOT NULL DEFAULT 0 CHECK (consumed_points >= 0),
  expired_points BIGINT NOT NULL DEFAULT 0 CHECK (expired_points >= 0),
  reversed_points BIGINT NOT NULL DEFAULT 0 CHECK (reversed_points >= 0),
  granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ,
  policy_version_id TEXT REFERENCES xz_point_expiry_policy_versions(id),
  policy_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  idempotency_key TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'ACTIVE'
    CHECK (status IN ('ACTIVE', 'EXHAUSTED', 'EXPIRED', 'REVERSED', 'LEGACY')),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (account_id, idempotency_key),
  CHECK (
    original_points = available_points + reserved_points + consumed_points
      + expired_points + reversed_points
  ),
  CHECK (available_points + reserved_points + consumed_points + expired_points + reversed_points >= 0),
  CHECK (expires_at IS NULL OR expires_at > granted_at),
  CHECK (
    expires_at IS NULL
    OR (
      source_type IN ('REGISTRATION_GIFT', 'ACTIVITY_GIFT', 'ADMIN_GIFT')
      AND policy_version_id IS NOT NULL
    )
  )
);

CREATE INDEX IF NOT EXISTS idx_xz_personal_point_lots_fefo
  ON xz_personal_point_lots(account_id, expires_at ASC NULLS LAST, granted_at ASC, id)
  WHERE available_points > 0 AND status IN ('ACTIVE', 'LEGACY');
CREATE INDEX IF NOT EXISTS idx_xz_personal_point_lots_expiry_scan
  ON xz_personal_point_lots(expires_at ASC, account_id, id)
  WHERE expires_at IS NOT NULL AND available_points > 0 AND status = 'ACTIVE';
CREATE INDEX IF NOT EXISTS idx_xz_personal_point_lots_user
  ON xz_personal_point_lots(user_id, status, expires_at ASC NULLS LAST, granted_at ASC);
CREATE INDEX IF NOT EXISTS idx_xz_personal_point_lots_reference
  ON xz_personal_point_lots(account_id, reference_type, reference_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_personal_point_lots_idempotency
  ON xz_personal_point_lots(account_id, idempotency_key);

CREATE TABLE IF NOT EXISTS xz_personal_point_reservations (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES xz_point_accounts(id),
  user_id TEXT NOT NULL,
  business_type TEXT NOT NULL,
  business_id TEXT NOT NULL,
  requested_points BIGINT NOT NULL CHECK (requested_points > 0),
  reserved_points BIGINT NOT NULL DEFAULT 0 CHECK (reserved_points >= 0),
  captured_points BIGINT NOT NULL DEFAULT 0 CHECK (captured_points >= 0),
  released_points BIGINT NOT NULL DEFAULT 0 CHECK (released_points >= 0),
  expired_points BIGINT NOT NULL DEFAULT 0 CHECK (expired_points >= 0),
  idempotency_key TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'RESERVED'
    CHECK (status IN ('RESERVED', 'PARTIAL', 'CAPTURED', 'RELEASED', 'EXPIRED', 'CANCELLED')),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (account_id, idempotency_key),
  UNIQUE (account_id, business_type, business_id),
  CHECK (
    requested_points = reserved_points + captured_points + released_points + expired_points
  )
);

CREATE INDEX IF NOT EXISTS idx_xz_personal_point_reservations_account
  ON xz_personal_point_reservations(account_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_personal_point_reservations_business
  ON xz_personal_point_reservations(account_id, business_type, business_id);
CREATE INDEX IF NOT EXISTS idx_xz_personal_point_reservations_idempotency
  ON xz_personal_point_reservations(account_id, idempotency_key);

CREATE TABLE IF NOT EXISTS xz_personal_point_reservation_allocations (
  id TEXT PRIMARY KEY,
  reservation_id TEXT NOT NULL REFERENCES xz_personal_point_reservations(id),
  lot_id TEXT NOT NULL REFERENCES xz_personal_point_lots(id),
  allocated_points BIGINT NOT NULL CHECK (allocated_points > 0),
  reserved_points BIGINT NOT NULL DEFAULT 0 CHECK (reserved_points >= 0),
  captured_points BIGINT NOT NULL DEFAULT 0 CHECK (captured_points >= 0),
  released_points BIGINT NOT NULL DEFAULT 0 CHECK (released_points >= 0),
  expired_points BIGINT NOT NULL DEFAULT 0 CHECK (expired_points >= 0),
  status TEXT NOT NULL DEFAULT 'RESERVED'
    CHECK (status IN ('RESERVED', 'PARTIAL', 'CAPTURED', 'RELEASED', 'EXPIRED')),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (reservation_id, lot_id),
  CHECK (
    allocated_points = reserved_points + captured_points + released_points + expired_points
  )
);

CREATE INDEX IF NOT EXISTS idx_xz_personal_point_reservation_allocations_reservation
  ON xz_personal_point_reservation_allocations(reservation_id, status, id);
CREATE INDEX IF NOT EXISTS idx_xz_personal_point_reservation_allocations_lot
  ON xz_personal_point_reservation_allocations(lot_id, status, id);

CREATE TABLE IF NOT EXISTS xz_personal_point_lot_movements (
  id TEXT PRIMARY KEY,
  lot_id TEXT NOT NULL REFERENCES xz_personal_point_lots(id),
  account_id TEXT NOT NULL REFERENCES xz_point_accounts(id),
  user_id TEXT NOT NULL,
  movement_type TEXT NOT NULL
    CHECK (movement_type IN (
      'OPENING', 'GRANT', 'RESERVE', 'CAPTURE', 'RELEASE', 'EXPIRE',
      'ADJUSTMENT', 'REVERSE'
    )),
  points BIGINT NOT NULL CHECK (points > 0),
  available_before BIGINT NOT NULL CHECK (available_before >= 0),
  available_after BIGINT NOT NULL CHECK (available_after >= 0),
  reserved_before BIGINT NOT NULL CHECK (reserved_before >= 0),
  reserved_after BIGINT NOT NULL CHECK (reserved_after >= 0),
  consumed_before BIGINT NOT NULL CHECK (consumed_before >= 0),
  consumed_after BIGINT NOT NULL CHECK (consumed_after >= 0),
  expired_before BIGINT NOT NULL CHECK (expired_before >= 0),
  expired_after BIGINT NOT NULL CHECK (expired_after >= 0),
  reversed_before BIGINT NOT NULL CHECK (reversed_before >= 0),
  reversed_after BIGINT NOT NULL CHECK (reversed_after >= 0),
  reference_type TEXT NOT NULL DEFAULT '',
  reference_id TEXT NOT NULL DEFAULT '',
  reservation_id TEXT REFERENCES xz_personal_point_reservations(id),
  idempotency_key TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (lot_id, idempotency_key),
  CHECK (
    CASE movement_type
      WHEN 'OPENING' THEN
        available_before = 0 AND reserved_before = 0 AND consumed_before = 0
        AND expired_before = 0 AND reversed_before = 0
        AND points = available_after + reserved_after + consumed_after
          + expired_after + reversed_after
      WHEN 'GRANT' THEN
        available_after = available_before + points
        AND reserved_after = reserved_before
        AND consumed_after = consumed_before
        AND expired_after = expired_before
        AND reversed_after = reversed_before
      WHEN 'RESERVE' THEN
        available_after = available_before - points
        AND reserved_after = reserved_before + points
        AND consumed_after = consumed_before
        AND expired_after = expired_before
        AND reversed_after = reversed_before
      WHEN 'CAPTURE' THEN
        available_after = available_before
        AND reserved_after = reserved_before - points
        AND consumed_after = consumed_before + points
        AND expired_after = expired_before
        AND reversed_after = reversed_before
      WHEN 'RELEASE' THEN
        available_after = available_before + points
        AND reserved_after = reserved_before - points
        AND consumed_after = consumed_before
        AND expired_after = expired_before
        AND reversed_after = reversed_before
      WHEN 'EXPIRE' THEN
        available_after = available_before - points
        AND reserved_after = reserved_before
        AND consumed_after = consumed_before
        AND expired_after = expired_before + points
        AND reversed_after = reversed_before
      WHEN 'REVERSE' THEN
        available_after = available_before - points
        AND reserved_after = reserved_before
        AND consumed_after = consumed_before
        AND expired_after = expired_before
        AND reversed_after = reversed_before + points
      WHEN 'ADJUSTMENT' THEN
        (
          available_after = available_before + points
          AND reserved_after = reserved_before
          AND consumed_after = consumed_before
          AND expired_after = expired_before
          AND reversed_after = reversed_before
        ) OR (
          available_after = available_before - points
          AND reserved_after = reserved_before
          AND consumed_after = consumed_before
          AND expired_after = expired_before
          AND reversed_after = reversed_before
        )
      ELSE FALSE
    END
  )
);

CREATE INDEX IF NOT EXISTS idx_xz_personal_point_lot_movements_lot_time
  ON xz_personal_point_lot_movements(lot_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_xz_personal_point_lot_movements_account_time
  ON xz_personal_point_lot_movements(account_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_xz_personal_point_lot_movements_reference
  ON xz_personal_point_lot_movements(account_id, reference_type, reference_id, created_at DESC);

CREATE OR REPLACE FUNCTION xz_reject_personal_point_lot_movement_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'xz_personal_point_lot_movements is append-only';
END;
$$;

DROP TRIGGER IF EXISTS trg_xz_personal_point_lot_movements_immutable
  ON xz_personal_point_lot_movements;
CREATE TRIGGER trg_xz_personal_point_lot_movements_immutable
BEFORE UPDATE OR DELETE ON xz_personal_point_lot_movements
FOR EACH ROW EXECUTE FUNCTION xz_reject_personal_point_lot_movement_mutation();

-- Frozen aggregate balances may only be reconstructed when the historical
-- reserve ledger proves the exact outstanding amount. A missing or mismatched
-- proof aborts this transaction; the migration never guesses an allocation.
DO $$
DECLARE
  mismatch_count BIGINT;
BEGIN
  IF EXISTS (
    SELECT 1
    FROM xz_point_accounts account
    WHERE account.frozen < 0 OR account.available < 0
  ) THEN
    RAISE EXCEPTION 'migration 103 refuses negative xz_point_accounts balance';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM xz_point_accounts account
    WHERE (account.available > 0 OR account.frozen > 0)
      AND NULLIF(BTRIM(account.user_id), '') IS NULL
  ) THEN
    RAISE EXCEPTION 'migration 103 refuses non-zero point account without user_id';
  END IF;

  IF EXISTS (SELECT 1 FROM xz_point_accounts WHERE frozen > 0) THEN
    IF to_regclass('public.xz_wallet_ledger') IS NULL THEN
      RAISE EXCEPTION 'migration 103 cannot reconstruct frozen balances without xz_wallet_ledger evidence';
    END IF;

    SELECT count(*)
    INTO mismatch_count
    FROM xz_point_accounts account
    LEFT JOIN (
      SELECT account_id,
             sum(
               CASE
                 WHEN entry_type = 'RESERVE' THEN points
                 WHEN entry_type IN ('CAPTURE', 'RELEASE') THEN -points
                 ELSE 0
               END
             ) AS frozen_points
      FROM xz_wallet_ledger
      WHERE entry_type IN ('RESERVE', 'CAPTURE', 'RELEASE')
      GROUP BY account_id
    ) evidence ON evidence.account_id = account.id
    WHERE account.frozen > 0
      AND coalesce(evidence.frozen_points, 0) <> account.frozen;

    IF mismatch_count > 0 THEN
      RAISE EXCEPTION 'migration 103 frozen balance reconstruction failed for % account(s)', mismatch_count;
    END IF;
  END IF;
END;
$$;

-- The policy is immutable once published; subsequent policy changes create a
-- new version. The row is deliberately inserted with a stable id and version
-- so rerunning this migration cannot create a second initial policy.
INSERT INTO xz_point_expiry_policy_versions(
  id, version, revision, enabled, duration_value, duration_unit, time_zone,
  source_types, effective_from, status, created_by, change_reason, metadata
)
VALUES (
  'point_expiry_policy_v1', 1, 1, TRUE, 3, 'CALENDAR_MONTH', 'Asia/Shanghai',
  '["REGISTRATION_GIFT","ACTIVITY_GIFT","ADMIN_GIFT"]'::jsonb,
  now(), 'PUBLISHED', 'migration:103', 'initial three-calendar-month Asia/Shanghai policy',
  '{"migration":"103-personal-gift-point-expiry","calendarMonthClamp":true}'::jsonb
)
ON CONFLICT (version) DO NOTHING;

-- Every existing non-zero aggregate balance becomes a permanent LEGACY lot.
-- The idempotency key is account-scoped and deterministic, so a rerun does not
-- grant a second balance and does not rewrite the old economic ledger.
INSERT INTO xz_personal_point_lots(
  id, account_id, user_id, source_type, reference_type, reference_id,
  original_points, available_points, reserved_points, consumed_points,
  expired_points, reversed_points, granted_at, expires_at, policy_version_id,
  policy_snapshot, idempotency_key, status, metadata
)
SELECT
  'personal_point_lot_legacy_' || substr(md5(account.id), 1, 24),
  account.id,
  account.user_id,
  'LEGACY',
  'POINT_ACCOUNT',
  account.id,
  account.available + account.frozen,
  account.available,
  account.frozen,
  0,
  0,
  0,
  now(),
  NULL,
  NULL,
  jsonb_build_object(
    'migration', '103-personal-gift-point-expiry',
    'permanent', TRUE,
    'legacyAvailable', account.available,
    'legacyFrozen', account.frozen
  ),
  'migration:103:legacy:' || account.id,
  CASE WHEN account.available + account.frozen > 0 THEN 'ACTIVE' ELSE 'EXHAUSTED' END,
  jsonb_build_object('migration', '103-personal-gift-point-expiry')
FROM xz_point_accounts account
WHERE account.available > 0 OR account.frozen > 0
ON CONFLICT (account_id, idempotency_key) DO NOTHING;

-- Opening movements make the migrated projection auditable without changing
-- any pre-existing xz_wallet_ledger or point transaction rows.
INSERT INTO xz_personal_point_lot_movements(
  id, lot_id, account_id, user_id, movement_type, points,
  available_before, available_after, reserved_before, reserved_after,
  consumed_before, consumed_after, expired_before, expired_after,
  reversed_before, reversed_after, reference_type, reference_id,
  idempotency_key, metadata
)
SELECT
  'personal_point_lot_movement_legacy_' || substr(md5(account.id), 1, 24),
  'personal_point_lot_legacy_' || substr(md5(account.id), 1, 24),
  account.id,
  account.user_id,
  'OPENING',
  account.available + account.frozen,
  0,
  account.available,
  0,
  account.frozen,
  0,
  0,
  0,
  0,
  0,
  0,
  'POINT_ACCOUNT',
  account.id,
  'migration:103:legacy-opening:' || account.id,
  jsonb_build_object('migration', '103-personal-gift-point-expiry')
FROM xz_point_accounts account
WHERE account.available > 0 OR account.frozen > 0
ON CONFLICT (lot_id, idempotency_key) DO NOTHING;

COMMIT;
