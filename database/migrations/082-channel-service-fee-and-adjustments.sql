-- Channel ecosystem V1.3.2: operation-center service lifecycle and auditable adjustments.

BEGIN;

CREATE TABLE IF NOT EXISTS xz_operation_center_service_orders (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  order_id TEXT NOT NULL REFERENCES xz_orders(id),
  order_no TEXT NOT NULL,
  applicant_user_id TEXT NOT NULL REFERENCES xz_users(id),
  technical_service_fee_cents BIGINT NOT NULL CHECK (technical_service_fee_cents > 0),
  currency CHAR(3) NOT NULL DEFAULT 'CNY',
  status TEXT NOT NULL CHECK (status IN (
    'PENDING_PAYMENT', 'REVIEW_REQUIRED', 'ACTIVE', 'REJECTED',
    'REFUND_REVERSING', 'REFUNDING', 'REFUNDED', 'REVOKED'
  )),
  paid_at TIMESTAMPTZ,
  reviewed_at TIMESTAMPTZ,
  reviewed_by TEXT,
  activated_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ,
  refund_order_id TEXT,
  state_version BIGINT NOT NULL DEFAULT 0 CHECK (state_version >= 0),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (order_id),
  UNIQUE (tenant_id, order_no),
  CHECK (status <> 'ACTIVE' OR activated_at IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_xz_operation_center_service_orders_review
  ON xz_operation_center_service_orders(tenant_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_commercial_adjustments (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  source_order_id TEXT REFERENCES xz_orders(id),
  source_refund_id TEXT,
  account_type TEXT NOT NULL CHECK (account_type IN (
    'TOKEN', 'COMMISSION', 'REFERRAL_REWARD', 'FROZEN',
    'WITHDRAWABLE', 'WITHDRAWN', 'PLATFORM_INCOME'
  )),
  subject_type TEXT NOT NULL CHECK (subject_type IN ('USER', 'AGENT', 'OPERATION_CENTER', 'PLATFORM')),
  subject_id TEXT NOT NULL,
  amount_cents BIGINT NOT NULL CHECK (amount_cents <> 0),
  adjustment_type TEXT NOT NULL CHECK (adjustment_type IN ('GRANT', 'REVERSAL', 'RECOVERY', 'MANUAL')),
  status TEXT NOT NULL DEFAULT 'POSTED' CHECK (status IN ('PENDING', 'POSTED', 'FAILED', 'CANCELLED')),
  reversal_of_id TEXT REFERENCES xz_commercial_adjustments(id),
  idempotency_key TEXT NOT NULL UNIQUE,
  reason_code TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_by TEXT NOT NULL DEFAULT 'SYSTEM',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (
    adjustment_type <> 'REVERSAL' OR (amount_cents < 0 AND reversal_of_id IS NOT NULL)
  )
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_commercial_adjustments_reversal
  ON xz_commercial_adjustments(reversal_of_id, account_type)
  WHERE adjustment_type = 'REVERSAL';
CREATE INDEX IF NOT EXISTS idx_xz_commercial_adjustments_order
  ON xz_commercial_adjustments(tenant_id, source_order_id, created_at);
CREATE INDEX IF NOT EXISTS idx_xz_commercial_adjustments_subject
  ON xz_commercial_adjustments(tenant_id, subject_type, subject_id, account_type, created_at DESC);

COMMIT;
