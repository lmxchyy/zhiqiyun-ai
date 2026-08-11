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

-- Composite ownership references are intentionally redundant with the
-- account primary key: they let child rows prove account/user identity in a
-- single database constraint without adding a global user uniqueness rule.
CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_point_accounts_id_user_103
  ON xz_point_accounts(id, user_id);

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
  account_id TEXT NOT NULL,
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
  CONSTRAINT fk_xz_personal_point_lots_account_user
    FOREIGN KEY (account_id, user_id) REFERENCES xz_point_accounts(id, user_id),
  CHECK (
    original_points = available_points + reserved_points + consumed_points
      + expired_points + reversed_points
  ),
  CHECK (available_points + reserved_points + consumed_points + expired_points + reversed_points >= 0),
  CONSTRAINT ck_xz_personal_point_lots_idempotency_nonblank
    CHECK (length(btrim(idempotency_key)) > 0),
  CHECK (expires_at IS NULL OR expires_at > granted_at),
  CONSTRAINT ck_xz_personal_point_lots_non_gift_expiry CHECK (
    source_type IN ('REGISTRATION_GIFT', 'ACTIVITY_GIFT', 'ADMIN_GIFT')
    OR (expires_at IS NULL AND policy_version_id IS NULL)
  ),
  CONSTRAINT ck_xz_personal_point_lots_non_gift_policy CHECK (
    source_type IN ('REGISTRATION_GIFT', 'ACTIVITY_GIFT', 'ADMIN_GIFT')
    OR policy_version_id IS NULL
  ),
  CONSTRAINT ck_xz_personal_point_lots_policy_snapshot_object
    CHECK (jsonb_typeof(policy_snapshot) = 'object'),
  CONSTRAINT ck_xz_personal_point_lots_legacy_status CHECK (
    (source_type = 'LEGACY' AND status = 'LEGACY' AND expires_at IS NULL AND policy_version_id IS NULL)
    OR (source_type <> 'LEGACY' AND status <> 'LEGACY')
  ),
  CONSTRAINT ck_xz_personal_point_lots_status_balance CHECK (
    status = 'LEGACY'
    OR (status = 'ACTIVE' AND (available_points > 0 OR reserved_points > 0))
    OR (
      status = 'EXHAUSTED'
      AND available_points = 0
      AND reserved_points = 0
      AND consumed_points + expired_points + reversed_points = original_points
    )
    OR (
      status = 'EXPIRED'
      AND available_points = 0
      AND reserved_points = 0
      AND expired_points > 0
    )
    OR (
      status = 'REVERSED'
      AND available_points = 0
      AND reserved_points = 0
      AND reversed_points > 0
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
  account_id TEXT NOT NULL,
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
  CONSTRAINT fk_xz_personal_point_reservations_account_user
    FOREIGN KEY (account_id, user_id) REFERENCES xz_point_accounts(id, user_id),
  CONSTRAINT ck_xz_personal_point_reservations_idempotency_nonblank
    CHECK (length(btrim(idempotency_key)) > 0),
  CONSTRAINT ck_xz_personal_point_reservations_conservation CHECK (
    requested_points = reserved_points + captured_points + released_points + expired_points
  ),
  CONSTRAINT ck_xz_personal_point_reservations_status_balance CHECK (
    (status = 'RESERVED' AND reserved_points > 0)
    OR (status = 'PARTIAL' AND (reserved_points > 0 OR captured_points > 0))
    OR (status = 'CAPTURED' AND captured_points > 0)
    OR (status = 'RELEASED' AND released_points > 0)
    OR (status = 'EXPIRED' AND expired_points > 0)
    OR (status = 'CANCELLED' AND released_points + expired_points > 0)
  )
);

CREATE INDEX IF NOT EXISTS idx_xz_personal_point_reservations_account
  ON xz_personal_point_reservations(account_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_personal_point_reservations_business
  ON xz_personal_point_reservations(account_id, business_type, business_id);
CREATE INDEX IF NOT EXISTS idx_xz_personal_point_reservations_idempotency
  ON xz_personal_point_reservations(account_id, idempotency_key);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_personal_point_lots_id_account_user
  ON xz_personal_point_lots(id, account_id, user_id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_personal_point_reservations_id_account_user
  ON xz_personal_point_reservations(id, account_id, user_id);

CREATE TABLE IF NOT EXISTS xz_personal_point_reservation_allocations (
  id TEXT PRIMARY KEY,
  reservation_id TEXT NOT NULL,
  lot_id TEXT NOT NULL,
  account_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
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
  CONSTRAINT fk_xz_personal_point_reservation_allocations_reservation_owner
    FOREIGN KEY (reservation_id, account_id, user_id)
    REFERENCES xz_personal_point_reservations(id, account_id, user_id),
  CONSTRAINT fk_xz_personal_point_reservation_allocations_lot_owner
    FOREIGN KEY (lot_id, account_id, user_id)
    REFERENCES xz_personal_point_lots(id, account_id, user_id),
  CONSTRAINT ck_xz_personal_point_reservation_allocations_conservation CHECK (
    allocated_points = reserved_points + captured_points + released_points + expired_points
  ),
  CONSTRAINT ck_xz_personal_point_reservation_allocations_status_balance CHECK (
    (status = 'RESERVED' AND reserved_points > 0)
    OR (status = 'PARTIAL' AND (reserved_points > 0 OR captured_points > 0))
    OR (status = 'CAPTURED' AND captured_points > 0)
    OR (status = 'RELEASED' AND released_points > 0)
    OR (status = 'EXPIRED' AND expired_points > 0)
  )
);

CREATE INDEX IF NOT EXISTS idx_xz_personal_point_reservation_allocations_reservation
  ON xz_personal_point_reservation_allocations(reservation_id, status, id);
CREATE INDEX IF NOT EXISTS idx_xz_personal_point_reservation_allocations_lot
  ON xz_personal_point_reservation_allocations(lot_id, status, id);

CREATE TABLE IF NOT EXISTS xz_personal_point_lot_movements (
  id TEXT PRIMARY KEY,
  lot_id TEXT NOT NULL,
  account_id TEXT NOT NULL,
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
  CONSTRAINT fk_xz_personal_point_lot_movements_lot_owner
    FOREIGN KEY (lot_id, account_id, user_id)
    REFERENCES xz_personal_point_lots(id, account_id, user_id),
  CONSTRAINT fk_xz_personal_point_lot_movements_reservation_owner
    FOREIGN KEY (reservation_id, account_id, user_id)
    REFERENCES xz_personal_point_reservations(id, account_id, user_id),
  CONSTRAINT ck_xz_personal_point_lot_movements_idempotency_nonblank
    CHECK (length(btrim(idempotency_key)) > 0),
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

-- The first version of 103 was already released to some isolated databases.
-- Upgrade those tables in place before adding the ownership checks; any
-- historic mismatch aborts this transaction instead of being silently fixed.
UPDATE xz_personal_point_lots
SET status = 'LEGACY'
WHERE source_type = 'LEGACY' AND status <> 'LEGACY';

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_personal_point_lots'::regclass
      AND conname = 'fk_xz_personal_point_lots_account_user'
  ) THEN
    ALTER TABLE xz_personal_point_lots
      ADD CONSTRAINT fk_xz_personal_point_lots_account_user
      FOREIGN KEY (account_id, user_id) REFERENCES xz_point_accounts(id, user_id);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_personal_point_reservations'::regclass
      AND conname = 'fk_xz_personal_point_reservations_account_user'
  ) THEN
    ALTER TABLE xz_personal_point_reservations
      ADD CONSTRAINT fk_xz_personal_point_reservations_account_user
      FOREIGN KEY (account_id, user_id) REFERENCES xz_point_accounts(id, user_id);
  END IF;
END;
$$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_personal_point_lots'::regclass
      AND conname = 'ck_xz_personal_point_lots_idempotency_nonblank'
  ) THEN
    ALTER TABLE xz_personal_point_lots
      ADD CONSTRAINT ck_xz_personal_point_lots_idempotency_nonblank
      CHECK (length(btrim(idempotency_key)) > 0);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_personal_point_lots'::regclass
      AND conname = 'ck_xz_personal_point_lots_non_gift_expiry'
  ) THEN
    ALTER TABLE xz_personal_point_lots
      ADD CONSTRAINT ck_xz_personal_point_lots_non_gift_expiry
      CHECK (
        source_type IN ('REGISTRATION_GIFT', 'ACTIVITY_GIFT', 'ADMIN_GIFT')
        OR (expires_at IS NULL AND policy_version_id IS NULL)
      );
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_personal_point_lots'::regclass
      AND conname = 'ck_xz_personal_point_lots_non_gift_policy'
  ) THEN
    ALTER TABLE xz_personal_point_lots
      ADD CONSTRAINT ck_xz_personal_point_lots_non_gift_policy
      CHECK (
        source_type IN ('REGISTRATION_GIFT', 'ACTIVITY_GIFT', 'ADMIN_GIFT')
        OR policy_version_id IS NULL
      );
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_personal_point_lots'::regclass
      AND conname = 'ck_xz_personal_point_lots_policy_snapshot_object'
  ) THEN
    ALTER TABLE xz_personal_point_lots
      ADD CONSTRAINT ck_xz_personal_point_lots_policy_snapshot_object
      CHECK (jsonb_typeof(policy_snapshot) = 'object');
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_personal_point_lots'::regclass
      AND conname = 'ck_xz_personal_point_lots_legacy_status'
  ) THEN
    ALTER TABLE xz_personal_point_lots
      ADD CONSTRAINT ck_xz_personal_point_lots_legacy_status
      CHECK (
        (source_type = 'LEGACY' AND status = 'LEGACY' AND expires_at IS NULL AND policy_version_id IS NULL)
        OR (source_type <> 'LEGACY' AND status <> 'LEGACY')
      );
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_personal_point_lots'::regclass
      AND conname = 'ck_xz_personal_point_lots_status_balance'
  ) THEN
    ALTER TABLE xz_personal_point_lots
      ADD CONSTRAINT ck_xz_personal_point_lots_status_balance
      CHECK (
        status = 'LEGACY'
        OR (status = 'ACTIVE' AND (available_points > 0 OR reserved_points > 0))
        OR (
          status = 'EXHAUSTED'
          AND available_points = 0
          AND reserved_points = 0
          AND consumed_points + expired_points + reversed_points = original_points
        )
        OR (
          status = 'EXPIRED'
          AND available_points = 0
          AND reserved_points = 0
          AND expired_points > 0
        )
        OR (
          status = 'REVERSED'
          AND available_points = 0
          AND reserved_points = 0
          AND reversed_points > 0
        )
      );
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_personal_point_reservations'::regclass
      AND conname = 'ck_xz_personal_point_reservations_idempotency_nonblank'
  ) THEN
    ALTER TABLE xz_personal_point_reservations
      ADD CONSTRAINT ck_xz_personal_point_reservations_idempotency_nonblank
      CHECK (length(btrim(idempotency_key)) > 0);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_personal_point_reservations'::regclass
      AND conname = 'ck_xz_personal_point_reservations_status_balance'
  ) THEN
    ALTER TABLE xz_personal_point_reservations
      ADD CONSTRAINT ck_xz_personal_point_reservations_status_balance
      CHECK (
        (status = 'RESERVED' AND reserved_points > 0)
        OR (status = 'PARTIAL' AND (reserved_points > 0 OR captured_points > 0))
        OR (status = 'CAPTURED' AND captured_points > 0)
        OR (status = 'RELEASED' AND released_points > 0)
        OR (status = 'EXPIRED' AND expired_points > 0)
        OR (status = 'CANCELLED' AND released_points + expired_points > 0)
      );
  END IF;
END;
$$;

ALTER TABLE xz_personal_point_reservation_allocations
  ADD COLUMN IF NOT EXISTS account_id TEXT;
ALTER TABLE xz_personal_point_reservation_allocations
  ADD COLUMN IF NOT EXISTS user_id TEXT;
UPDATE xz_personal_point_reservation_allocations allocation
SET account_id = COALESCE(allocation.account_id, reservation.account_id),
    user_id = COALESCE(allocation.user_id, reservation.user_id)
FROM xz_personal_point_reservations reservation
WHERE reservation.id = allocation.reservation_id;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM xz_personal_point_reservation_allocations allocation
    LEFT JOIN xz_personal_point_reservations reservation
      ON reservation.id = allocation.reservation_id
    LEFT JOIN xz_personal_point_lots lot
      ON lot.id = allocation.lot_id
    WHERE allocation.account_id IS NULL
       OR allocation.user_id IS NULL
       OR reservation.id IS NULL
       OR lot.id IS NULL
       OR allocation.account_id IS DISTINCT FROM reservation.account_id
       OR allocation.user_id IS DISTINCT FROM reservation.user_id
       OR allocation.account_id IS DISTINCT FROM lot.account_id
       OR allocation.user_id IS DISTINCT FROM lot.user_id
  ) THEN
    RAISE EXCEPTION 'migration 103 reservation allocation ownership mismatch';
  END IF;
END;
$$;

ALTER TABLE xz_personal_point_reservation_allocations
  ALTER COLUMN account_id SET NOT NULL;
ALTER TABLE xz_personal_point_reservation_allocations
  ALTER COLUMN user_id SET NOT NULL;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_personal_point_reservation_allocations'::regclass
      AND conname = 'fk_xz_personal_point_reservation_allocations_reservation_owner'
  ) THEN
    ALTER TABLE xz_personal_point_reservation_allocations
      ADD CONSTRAINT fk_xz_personal_point_reservation_allocations_reservation_owner
      FOREIGN KEY (reservation_id, account_id, user_id)
      REFERENCES xz_personal_point_reservations(id, account_id, user_id);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_personal_point_reservation_allocations'::regclass
      AND conname = 'fk_xz_personal_point_reservation_allocations_lot_owner'
  ) THEN
    ALTER TABLE xz_personal_point_reservation_allocations
      ADD CONSTRAINT fk_xz_personal_point_reservation_allocations_lot_owner
      FOREIGN KEY (lot_id, account_id, user_id)
      REFERENCES xz_personal_point_lots(id, account_id, user_id);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_personal_point_reservation_allocations'::regclass
      AND conname = 'ck_xz_personal_point_reservation_allocations_status_balance'
  ) THEN
    ALTER TABLE xz_personal_point_reservation_allocations
      ADD CONSTRAINT ck_xz_personal_point_reservation_allocations_status_balance
      CHECK (
        (status = 'RESERVED' AND reserved_points > 0)
        OR (status = 'PARTIAL' AND (reserved_points > 0 OR captured_points > 0))
        OR (status = 'CAPTURED' AND captured_points > 0)
        OR (status = 'RELEASED' AND released_points > 0)
        OR (status = 'EXPIRED' AND expired_points > 0)
      );
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_personal_point_lot_movements'::regclass
      AND conname = 'fk_xz_personal_point_lot_movements_lot_owner'
  ) THEN
    ALTER TABLE xz_personal_point_lot_movements
      ADD CONSTRAINT fk_xz_personal_point_lot_movements_lot_owner
      FOREIGN KEY (lot_id, account_id, user_id)
      REFERENCES xz_personal_point_lots(id, account_id, user_id);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_personal_point_lot_movements'::regclass
      AND conname = 'fk_xz_personal_point_lot_movements_reservation_owner'
  ) THEN
    ALTER TABLE xz_personal_point_lot_movements
      ADD CONSTRAINT fk_xz_personal_point_lot_movements_reservation_owner
      FOREIGN KEY (reservation_id, account_id, user_id)
      REFERENCES xz_personal_point_reservations(id, account_id, user_id);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_personal_point_lot_movements'::regclass
      AND conname = 'ck_xz_personal_point_lot_movements_idempotency_nonblank'
  ) THEN
    ALTER TABLE xz_personal_point_lot_movements
      ADD CONSTRAINT ck_xz_personal_point_lot_movements_idempotency_nonblank
      CHECK (length(btrim(idempotency_key)) > 0);
  END IF;
END;
$$;

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

CREATE OR REPLACE FUNCTION xz_reject_published_personal_point_policy_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP IN ('UPDATE', 'DELETE') AND OLD.status = 'PUBLISHED' THEN
    RAISE EXCEPTION 'published point expiry policy versions are immutable';
  END IF;
  IF TG_OP = 'DELETE' THEN
    RETURN OLD;
  END IF;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_xz_point_expiry_policy_versions_immutable
  ON xz_point_expiry_policy_versions;
CREATE TRIGGER trg_xz_point_expiry_policy_versions_immutable
BEFORE UPDATE OR DELETE ON xz_point_expiry_policy_versions
FOR EACH ROW EXECUTE FUNCTION xz_reject_published_personal_point_policy_mutation();

CREATE OR REPLACE FUNCTION xz_validate_personal_point_lot_policy()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  policy RECORD;
BEGIN
  IF NEW.source_type IN ('REGISTRATION_GIFT', 'ACTIVITY_GIFT', 'ADMIN_GIFT') THEN
    IF NEW.policy_version_id IS NULL THEN
      RAISE EXCEPTION 'gift point lot requires an explicit policy version';
    END IF;

    SELECT version, enabled, duration_value, duration_unit, time_zone,
           source_types, status
    INTO policy
    FROM xz_point_expiry_policy_versions
    WHERE id = NEW.policy_version_id;

    IF NOT FOUND THEN
      RAISE EXCEPTION 'gift point lot references missing policy version %', NEW.policy_version_id;
    END IF;
    IF policy.status NOT IN ('PUBLISHED', 'ARCHIVED') THEN
      RAISE EXCEPTION 'gift point lot references unpublished policy version %', NEW.policy_version_id;
    END IF;
    IF NOT (policy.source_types ? NEW.source_type) THEN
      RAISE EXCEPTION 'policy version % does not cover gift source %', NEW.policy_version_id, NEW.source_type;
    END IF;
    IF jsonb_typeof(NEW.policy_snapshot) <> 'object'
       OR NEW.policy_snapshot = '{}'::jsonb
       OR NEW.policy_snapshot->>'version' IS DISTINCT FROM policy.version::text
       OR NEW.policy_snapshot->>'enabled' IS DISTINCT FROM policy.enabled::text
       OR NEW.policy_snapshot->>'duration_value' IS DISTINCT FROM policy.duration_value::text
       OR NEW.policy_snapshot->>'duration_unit' IS DISTINCT FROM policy.duration_unit
       OR NEW.policy_snapshot->>'time_zone' IS DISTINCT FROM policy.time_zone THEN
      RAISE EXCEPTION 'gift point lot policy snapshot does not match policy version %', NEW.policy_version_id;
    END IF;
    IF policy.enabled AND NEW.expires_at IS NULL THEN
      RAISE EXCEPTION 'enabled gift point policy requires expires_at';
    END IF;
    IF NOT policy.enabled AND NEW.expires_at IS NOT NULL THEN
      RAISE EXCEPTION 'disabled gift point policy must remain permanent';
    END IF;
  ELSIF NEW.policy_version_id IS NOT NULL OR NEW.expires_at IS NOT NULL THEN
    RAISE EXCEPTION 'non-gift point lot cannot carry expiry policy or expires_at';
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_xz_personal_point_lots_policy_guard
  ON xz_personal_point_lots;
CREATE TRIGGER trg_xz_personal_point_lots_policy_guard
BEFORE INSERT OR UPDATE OF source_type, expires_at, policy_version_id, policy_snapshot, granted_at
ON xz_personal_point_lots
FOR EACH ROW EXECUTE FUNCTION xz_validate_personal_point_lot_policy();

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

DO $$
DECLARE
  policy RECORD;
BEGIN
  SELECT id, version, revision, enabled, duration_value, duration_unit,
         time_zone, source_types, effective_to, status, created_by,
         change_reason, metadata
  INTO policy
  FROM xz_point_expiry_policy_versions
  WHERE version = 1;

  IF NOT FOUND
     OR policy.id IS DISTINCT FROM 'point_expiry_policy_v1'
     OR policy.version IS DISTINCT FROM 1
     OR policy.revision IS DISTINCT FROM 1
     OR policy.enabled IS DISTINCT FROM TRUE
     OR policy.duration_value IS DISTINCT FROM 3
     OR policy.duration_unit IS DISTINCT FROM 'CALENDAR_MONTH'
     OR policy.time_zone IS DISTINCT FROM 'Asia/Shanghai'
     OR policy.source_types IS DISTINCT FROM '["REGISTRATION_GIFT","ACTIVITY_GIFT","ADMIN_GIFT"]'::jsonb
     OR policy.effective_to IS NOT NULL
     OR policy.status IS DISTINCT FROM 'PUBLISHED'
     OR policy.created_by IS DISTINCT FROM 'migration:103'
     OR policy.change_reason IS DISTINCT FROM 'initial three-calendar-month Asia/Shanghai policy'
     OR policy.metadata IS DISTINCT FROM '{"migration":"103-personal-gift-point-expiry","calendarMonthClamp":true}'::jsonb THEN
    RAISE EXCEPTION 'migration 103 initial policy version 1 collision or drift detected';
  END IF;
END;
$$;

-- Existing 103 installations may already contain gift lots written before the
-- policy trigger existed. Validate those rows before this migration commits;
-- a new-write trigger alone cannot repair or silently accept historical drift.
DO $$
DECLARE
  invalid_gift_count BIGINT;
BEGIN
  SELECT count(*)
  INTO invalid_gift_count
  FROM xz_personal_point_lots lot
  LEFT JOIN xz_point_expiry_policy_versions policy
    ON policy.id = lot.policy_version_id
  WHERE lot.source_type IN ('REGISTRATION_GIFT', 'ACTIVITY_GIFT', 'ADMIN_GIFT')
    AND (
      lot.policy_version_id IS NULL
      OR policy.id IS NULL
      OR policy.status NOT IN ('PUBLISHED', 'ARCHIVED')
      OR NOT (policy.source_types ? lot.source_type)
      OR jsonb_typeof(lot.policy_snapshot) IS DISTINCT FROM 'object'
      OR lot.policy_snapshot = '{}'::jsonb
      OR lot.policy_snapshot->>'version' IS DISTINCT FROM policy.version::text
      OR lot.policy_snapshot->>'enabled' IS DISTINCT FROM policy.enabled::text
      OR lot.policy_snapshot->>'duration_value' IS DISTINCT FROM policy.duration_value::text
      OR lot.policy_snapshot->>'duration_unit' IS DISTINCT FROM policy.duration_unit
      OR lot.policy_snapshot->>'time_zone' IS DISTINCT FROM policy.time_zone
      OR (policy.enabled AND lot.expires_at IS NULL)
      OR (NOT policy.enabled AND lot.expires_at IS NOT NULL)
    );

  IF invalid_gift_count > 0 THEN
    RAISE EXCEPTION
      'migration 103 gift lot policy preflight failed for % row(s)', invalid_gift_count;
  END IF;
END;
$$;

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
  'LEGACY',
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
