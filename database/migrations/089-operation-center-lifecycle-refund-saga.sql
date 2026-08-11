-- Operation-center phase-1 lifecycle, refund Saga, referral reward release and audit baseline.
-- This is an additive forward migration. It does not enable rollout modes or real settlement.

BEGIN;

-- Keep the existing service status column authoritative while adding the phase-1 REVOKING state.
-- The previous enum values remain valid so historical Legacy/Shadow/Canary/V132 rows are untouched.
DO $$
DECLARE
  constraint_name TEXT;
BEGIN
  FOR constraint_name IN
    SELECT c.conname
    FROM pg_constraint c
    WHERE c.conrelid = 'xz_operation_center_service_orders'::regclass
      AND c.contype = 'c'
      AND c.conname <> 'ck_xz_oc_service_orders_service_status_089'
      AND pg_get_constraintdef(c.oid) LIKE '%PENDING_PAYMENT%'
      AND pg_get_constraintdef(c.oid) LIKE '%REVIEW_REQUIRED%'
      AND pg_get_constraintdef(c.oid) LIKE '%REFUND_REVERSING%'
  LOOP
    EXECUTE format(
      'ALTER TABLE xz_operation_center_service_orders DROP CONSTRAINT %I',
      constraint_name
    );
  END LOOP;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_operation_center_service_orders'::regclass
      AND conname = 'ck_xz_oc_service_orders_service_status_089'
  ) THEN
    ALTER TABLE xz_operation_center_service_orders
      ADD CONSTRAINT ck_xz_oc_service_orders_service_status_089
      CHECK (status IN (
        'PENDING_PAYMENT', 'REVIEW_REQUIRED', 'ACTIVE', 'REJECTED',
        'REVOKING', 'REFUND_REVERSING', 'REFUNDING', 'REFUNDED', 'REVOKED'
      ));
  END IF;
END;
$$;

ALTER TABLE xz_operation_center_service_orders
  ADD COLUMN IF NOT EXISTS refund_status TEXT NOT NULL DEFAULT 'NONE',
  ADD COLUMN IF NOT EXISTS commercial_rule_set_id TEXT REFERENCES xz_commercial_rule_sets(id),
  ADD COLUMN IF NOT EXISTS commercial_rule_set_version INT,
  ADD COLUMN IF NOT EXISTS plan_version_id TEXT REFERENCES xz_commercial_plan_versions(id),
  ADD COLUMN IF NOT EXISTS commercial_order_snapshot_id TEXT REFERENCES xz_commercial_order_rule_snapshots(id),
  ADD COLUMN IF NOT EXISTS relationship_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS refund_policy_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS review_idempotency_key TEXT,
  ADD COLUMN IF NOT EXISTS refund_idempotency_key TEXT,
  ADD COLUMN IF NOT EXISTS payment_channel TEXT,
  ADD COLUMN IF NOT EXISTS provider_refund_no TEXT,
  ADD COLUMN IF NOT EXISTS refund_failure_class TEXT,
  ADD COLUMN IF NOT EXISTS refund_failure_detail JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS refund_attempt_count INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS next_refund_retry_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS manual_refund_voucher_reference TEXT,
  ADD COLUMN IF NOT EXISTS manual_refund_voucher_file_hash TEXT,
  ADD COLUMN IF NOT EXISTS manual_refund_submitted_by TEXT REFERENCES xz_users(id),
  ADD COLUMN IF NOT EXISTS manual_refund_approved_by TEXT REFERENCES xz_users(id),
  ADD COLUMN IF NOT EXISTS current_refund_task_id TEXT;

UPDATE xz_operation_center_service_orders service_order
SET commercial_rule_set_id = COALESCE(service_order.commercial_rule_set_id, snapshot.rule_set_id),
    commercial_rule_set_version = COALESCE(service_order.commercial_rule_set_version, snapshot.rule_set_version),
    plan_version_id = COALESCE(service_order.plan_version_id, snapshot.plan_version_id),
    commercial_order_snapshot_id = COALESCE(service_order.commercial_order_snapshot_id, snapshot.id),
    relationship_snapshot = CASE
      WHEN service_order.relationship_snapshot = '{}'::jsonb THEN snapshot.relationship_snapshot
      ELSE service_order.relationship_snapshot
    END,
    updated_at = service_order.updated_at
FROM xz_commercial_order_rule_snapshots snapshot
WHERE snapshot.order_id = service_order.order_id
  AND (
    service_order.commercial_rule_set_id IS NULL
    OR service_order.commercial_rule_set_version IS NULL
    OR service_order.plan_version_id IS NULL
    OR service_order.commercial_order_snapshot_id IS NULL
    OR service_order.relationship_snapshot = '{}'::jsonb
  );

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_operation_center_service_orders'::regclass
      AND conname = 'ck_xz_oc_service_orders_refund_status_089'
  ) THEN
    ALTER TABLE xz_operation_center_service_orders
      ADD CONSTRAINT ck_xz_oc_service_orders_refund_status_089
      CHECK (refund_status IN (
        'NONE', 'PENDING', 'REVERSING', 'PROVIDER_PENDING', 'REFUND_RETRYABLE',
        'UNKNOWN_VERIFYING', 'MANUAL_REQUIRED', 'MANUAL_SUBMITTED',
        'SUCCEEDED', 'CANCELLED'
      ));
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_operation_center_service_orders'::regclass
      AND conname = 'ck_xz_oc_service_orders_rule_version_089'
  ) THEN
    ALTER TABLE xz_operation_center_service_orders
      ADD CONSTRAINT ck_xz_oc_service_orders_rule_version_089
      CHECK (commercial_rule_set_version IS NULL OR commercial_rule_set_version > 0);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_operation_center_service_orders'::regclass
      AND conname = 'ck_xz_oc_service_orders_snapshot_json_089'
  ) THEN
    ALTER TABLE xz_operation_center_service_orders
      ADD CONSTRAINT ck_xz_oc_service_orders_snapshot_json_089
      CHECK (
        jsonb_typeof(relationship_snapshot) = 'object'
        AND jsonb_typeof(refund_policy_snapshot) = 'object'
        AND jsonb_typeof(refund_failure_detail) = 'object'
      );
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_operation_center_service_orders'::regclass
      AND conname = 'ck_xz_oc_service_orders_refund_failure_class_089'
  ) THEN
    ALTER TABLE xz_operation_center_service_orders
      ADD CONSTRAINT ck_xz_oc_service_orders_refund_failure_class_089
      CHECK (refund_failure_class IS NULL OR refund_failure_class IN (
        'PROVIDER_UNSUPPORTED', 'TEMPORARY_FAILURE', 'UNKNOWN',
        'MANUAL_REQUIRED', 'VALIDATION_FAILURE'
      ));
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_operation_center_service_orders'::regclass
      AND conname = 'ck_xz_oc_service_orders_refund_attempts_089'
  ) THEN
    ALTER TABLE xz_operation_center_service_orders
      ADD CONSTRAINT ck_xz_oc_service_orders_refund_attempts_089
      CHECK (refund_attempt_count >= 0);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_operation_center_service_orders'::regclass
      AND conname = 'ck_xz_oc_service_orders_refund_identity_089'
  ) THEN
    ALTER TABLE xz_operation_center_service_orders
      ADD CONSTRAINT ck_xz_oc_service_orders_refund_identity_089
      CHECK (
        refund_status = 'NONE'
        OR (
          refund_idempotency_key IS NOT NULL
          AND btrim(refund_idempotency_key) <> ''
          AND payment_channel IS NOT NULL
          AND btrim(payment_channel) <> ''
        )
      );
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_operation_center_service_orders'::regclass
      AND conname = 'ck_xz_oc_service_orders_refund_success_evidence_089'
  ) THEN
    ALTER TABLE xz_operation_center_service_orders
      ADD CONSTRAINT ck_xz_oc_service_orders_refund_success_evidence_089
      CHECK (
        refund_status <> 'SUCCEEDED'
        OR (provider_refund_no IS NOT NULL AND btrim(provider_refund_no) <> '')
        OR (
          manual_refund_voucher_reference IS NOT NULL
          AND btrim(manual_refund_voucher_reference) <> ''
          AND manual_refund_voucher_file_hash ~ '^[0-9a-f]{64}$'
          AND manual_refund_submitted_by IS NOT NULL
          AND manual_refund_approved_by IS NOT NULL
        )
      );
  END IF;
END;
$$;

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_oc_service_orders_review_idempotency_089
  ON xz_operation_center_service_orders(tenant_id, review_idempotency_key)
  WHERE review_idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_oc_service_orders_refund_idempotency_089
  ON xz_operation_center_service_orders(refund_idempotency_key)
  WHERE refund_idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_oc_service_orders_provider_refund_089
  ON xz_operation_center_service_orders(payment_channel, provider_refund_no)
  WHERE provider_refund_no IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_xz_oc_service_orders_lifecycle_089
  ON xz_operation_center_service_orders(tenant_id, status, refund_status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_oc_service_orders_retry_089
  ON xz_operation_center_service_orders(refund_status, next_refund_retry_at)
  WHERE refund_status IN ('REFUND_RETRYABLE', 'UNKNOWN_VERIFYING');

CREATE TABLE IF NOT EXISTS xz_operation_center_refund_tasks (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  service_order_id TEXT NOT NULL REFERENCES xz_operation_center_service_orders(id),
  order_id TEXT NOT NULL REFERENCES xz_orders(id),
  payment_record_id TEXT REFERENCES xz_payment_records(id),
  commercial_rule_set_id TEXT NOT NULL REFERENCES xz_commercial_rule_sets(id),
  origin_type TEXT NOT NULL CHECK (origin_type IN ('REVIEW_REJECTION', 'ACTIVE_REVOCATION')),
  refund_scope TEXT NOT NULL DEFAULT 'FULL' CHECK (refund_scope = 'FULL'),
  amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
  currency TEXT NOT NULL CHECK (btrim(currency) <> ''),
  payment_channel TEXT NOT NULL CHECK (btrim(payment_channel) <> ''),
  provider_payment_no TEXT,
  provider_refund_no TEXT,
  provider_outcome TEXT CHECK (
    provider_outcome IS NULL
    OR provider_outcome IN ('SUCCESS', 'TEMPORARY_FAILURE', 'UNSUPPORTED', 'UNKNOWN')
  ),
  refund_status TEXT NOT NULL DEFAULT 'PENDING' CHECK (refund_status IN (
    'PENDING', 'REVERSING', 'PROVIDER_PENDING', 'REFUND_RETRYABLE',
    'UNKNOWN_VERIFYING', 'MANUAL_REQUIRED', 'MANUAL_SUBMITTED',
    'SUCCEEDED', 'CANCELLED'
  )),
  failure_class TEXT CHECK (failure_class IS NULL OR failure_class IN (
    'PROVIDER_UNSUPPORTED', 'TEMPORARY_FAILURE', 'UNKNOWN',
    'MANUAL_REQUIRED', 'VALIDATION_FAILURE'
  )),
  failure_detail JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(failure_detail) = 'object'),
  idempotency_key TEXT NOT NULL CHECK (btrim(idempotency_key) <> ''),
  attempt_count INT NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  next_retry_at TIMESTAMPTZ,
  lease_owner TEXT,
  lease_expires_at TIMESTAMPTZ,
  unknown_since TIMESTAMPTZ,
  prepared_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  manual_provider_transaction_id TEXT,
  manual_voucher_reference TEXT,
  manual_voucher_file_hash TEXT,
  manual_submitted_by TEXT REFERENCES xz_users(id),
  manual_approved_by TEXT REFERENCES xz_users(id),
  state_version BIGINT NOT NULL DEFAULT 0 CHECK (state_version >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (idempotency_key),
  UNIQUE (service_order_id, refund_scope),
  CHECK (refund_status <> 'REFUND_RETRYABLE' OR failure_class = 'TEMPORARY_FAILURE'),
  CHECK (refund_status <> 'UNKNOWN_VERIFYING' OR (failure_class = 'UNKNOWN' AND unknown_since IS NOT NULL)),
  CHECK (refund_status NOT IN ('PROVIDER_PENDING', 'REFUND_RETRYABLE', 'UNKNOWN_VERIFYING', 'MANUAL_REQUIRED', 'MANUAL_SUBMITTED', 'SUCCEEDED') OR prepared_at IS NOT NULL),
  CHECK (refund_status <> 'SUCCEEDED' OR completed_at IS NOT NULL),
  CONSTRAINT ck_xz_oc_refund_tasks_success_evidence_089 CHECK (
    refund_status <> 'SUCCEEDED'
    OR (provider_refund_no IS NOT NULL AND btrim(provider_refund_no) <> '')
    OR (
      manual_provider_transaction_id IS NOT NULL
      AND btrim(manual_provider_transaction_id) <> ''
      AND manual_voucher_reference IS NOT NULL
      AND btrim(manual_voucher_reference) <> ''
      AND manual_voucher_file_hash ~ '^[0-9a-f]{64}$'
      AND manual_submitted_by IS NOT NULL
      AND manual_approved_by IS NOT NULL
    )
  ),
  CHECK (manual_approved_by IS NULL OR manual_approved_by <> manual_submitted_by)
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_oc_refund_tasks_provider_refund_089
  ON xz_operation_center_refund_tasks(payment_channel, provider_refund_no)
  WHERE provider_refund_no IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_xz_oc_refund_tasks_claim_089
  ON xz_operation_center_refund_tasks(refund_status, next_retry_at, created_at, id)
  WHERE refund_status IN ('PROVIDER_PENDING', 'REFUND_RETRYABLE', 'UNKNOWN_VERIFYING');
CREATE INDEX IF NOT EXISTS idx_xz_oc_refund_tasks_service_089
  ON xz_operation_center_refund_tasks(tenant_id, service_order_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_oc_refund_tasks_lease_089
  ON xz_operation_center_refund_tasks(lease_expires_at)
  WHERE lease_owner IS NOT NULL;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_operation_center_service_orders'::regclass
      AND conname = 'fk_xz_oc_service_orders_current_refund_task_089'
  ) THEN
    ALTER TABLE xz_operation_center_service_orders
      ADD CONSTRAINT fk_xz_oc_service_orders_current_refund_task_089
      FOREIGN KEY (current_refund_task_id)
      REFERENCES xz_operation_center_refund_tasks(id)
      NOT VALID;
    ALTER TABLE xz_operation_center_service_orders
      VALIDATE CONSTRAINT fk_xz_oc_service_orders_current_refund_task_089;
  END IF;
END;
$$;

CREATE TABLE IF NOT EXISTS xz_operation_center_manual_refunds (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  refund_task_id TEXT NOT NULL REFERENCES xz_operation_center_refund_tasks(id),
  payment_channel TEXT NOT NULL CHECK (btrim(payment_channel) <> ''),
  amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
  currency TEXT NOT NULL CHECK (btrim(currency) <> ''),
  provider_transaction_id TEXT NOT NULL CHECK (btrim(provider_transaction_id) <> ''),
  provider_refund_no TEXT,
  voucher_reference TEXT NOT NULL CHECK (btrim(voucher_reference) <> ''),
  voucher_file_hash TEXT NOT NULL CHECK (voucher_file_hash ~ '^[0-9a-f]{64}$'),
  status TEXT NOT NULL DEFAULT 'SUBMITTED' CHECK (status IN ('SUBMITTED', 'APPROVED', 'REJECTED')),
  submitted_by TEXT NOT NULL REFERENCES xz_users(id),
  submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  approved_by TEXT REFERENCES xz_users(id),
  approved_at TIMESTAMPTZ,
  rejection_reason TEXT,
  remark TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (approved_by IS NULL OR approved_by <> submitted_by),
  CHECK ((status = 'SUBMITTED' AND approved_by IS NULL AND approved_at IS NULL) OR status <> 'SUBMITTED'),
  CHECK ((status = 'APPROVED' AND approved_by IS NOT NULL AND approved_at IS NOT NULL) OR status <> 'APPROVED'),
  CHECK ((status = 'REJECTED' AND rejection_reason IS NOT NULL AND btrim(rejection_reason) <> '') OR status <> 'REJECTED')
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_oc_manual_refunds_approved_089
  ON xz_operation_center_manual_refunds(refund_task_id)
  WHERE status = 'APPROVED';
CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_oc_manual_refunds_provider_tx_089
  ON xz_operation_center_manual_refunds(payment_channel, provider_transaction_id);
CREATE INDEX IF NOT EXISTS idx_xz_oc_manual_refunds_task_089
  ON xz_operation_center_manual_refunds(refund_task_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_operation_center_review_events (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  service_order_id TEXT NOT NULL REFERENCES xz_operation_center_service_orders(id),
  decision TEXT NOT NULL CHECK (decision IN ('APPROVED', 'REJECTED')),
  event_status TEXT NOT NULL DEFAULT 'PENDING' CHECK (event_status IN ('PENDING', 'APPLIED', 'FAILED')),
  reviewed_by TEXT NOT NULL REFERENCES xz_users(id),
  request_id TEXT,
  idempotency_key TEXT NOT NULL CHECK (btrim(idempotency_key) <> ''),
  failure_class TEXT CHECK (failure_class IS NULL OR failure_class IN ('TEMPORARY_FAILURE', 'UNKNOWN', 'VALIDATION_FAILURE')),
  failure_detail JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(failure_detail) = 'object'),
  event_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(event_snapshot) = 'object'),
  applied_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, idempotency_key),
  UNIQUE (service_order_id, idempotency_key),
  CHECK ((event_status = 'APPLIED' AND applied_at IS NOT NULL) OR event_status <> 'APPLIED'),
  CHECK ((event_status = 'FAILED' AND failure_class IS NOT NULL) OR event_status <> 'FAILED')
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_oc_review_events_applied_089
  ON xz_operation_center_review_events(service_order_id)
  WHERE event_status = 'APPLIED';
CREATE INDEX IF NOT EXISTS idx_xz_oc_review_events_service_089
  ON xz_operation_center_review_events(service_order_id, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_referral_reward_release_tasks (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  referral_reward_id TEXT NOT NULL REFERENCES xz_referral_rewards(id),
  idempotency_key TEXT NOT NULL CHECK (btrim(idempotency_key) <> ''),
  release_status TEXT NOT NULL DEFAULT 'PENDING' CHECK (release_status IN ('PENDING', 'PROCESSING', 'SUCCEEDED', 'FAILED', 'CANCELLED')),
  attempt_count INT NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  next_retry_at TIMESTAMPTZ,
  lease_owner TEXT,
  lease_expires_at TIMESTAMPTZ,
  failure_class TEXT CHECK (failure_class IS NULL OR failure_class IN ('TEMPORARY_FAILURE', 'UNKNOWN', 'VALIDATION_FAILURE')),
  failure_detail JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(failure_detail) = 'object'),
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, idempotency_key),
  UNIQUE (referral_reward_id),
  CHECK ((release_status = 'PROCESSING' AND started_at IS NOT NULL) OR release_status <> 'PROCESSING'),
  CHECK ((release_status = 'SUCCEEDED' AND completed_at IS NOT NULL) OR release_status <> 'SUCCEEDED'),
  CHECK ((release_status = 'FAILED' AND failure_class IS NOT NULL) OR release_status <> 'FAILED')
);

CREATE INDEX IF NOT EXISTS idx_xz_referral_release_tasks_claim_089
  ON xz_referral_reward_release_tasks(release_status, next_retry_at, created_at, id)
  WHERE release_status IN ('PENDING', 'FAILED');
CREATE INDEX IF NOT EXISTS idx_xz_referral_release_tasks_lease_089
  ON xz_referral_reward_release_tasks(lease_expires_at)
  WHERE lease_owner IS NOT NULL;

CREATE TABLE IF NOT EXISTS xz_operation_center_state_transitions (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  entity_type TEXT NOT NULL CHECK (entity_type IN ('SERVICE_ORDER', 'REFUND_TASK', 'REFERRAL_REWARD', 'REWARD_RELEASE_TASK')),
  entity_id TEXT NOT NULL,
  from_status TEXT,
  to_status TEXT NOT NULL CHECK (btrim(to_status) <> ''),
  action TEXT NOT NULL CHECK (btrim(action) <> ''),
  actor_id TEXT,
  request_id TEXT,
  idempotency_key TEXT NOT NULL CHECK (btrim(idempotency_key) <> ''),
  transition_group_key TEXT NOT NULL CHECK (btrim(transition_group_key) <> ''),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (entity_type, entity_id, idempotency_key, to_status),
  CHECK (from_status IS NULL OR from_status <> to_status)
);

CREATE INDEX IF NOT EXISTS idx_xz_oc_state_transitions_entity_089
  ON xz_operation_center_state_transitions(entity_type, entity_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_xz_oc_state_transitions_group_089
  ON xz_operation_center_state_transitions(transition_group_key, created_at, id);
CREATE INDEX IF NOT EXISTS idx_xz_oc_state_transitions_tenant_089
  ON xz_operation_center_state_transitions(tenant_id, action, created_at DESC);

ALTER TABLE xz_referral_rewards
  ADD COLUMN IF NOT EXISTS commercial_rule_set_id TEXT REFERENCES xz_commercial_rule_sets(id),
  ADD COLUMN IF NOT EXISTS grant_wallet_ledger_id TEXT REFERENCES xz_commission_wallet_ledger(id),
  ADD COLUMN IF NOT EXISTS release_wallet_ledger_id TEXT REFERENCES xz_commission_wallet_ledger(id),
  ADD COLUMN IF NOT EXISTS original_wallet_ledger_id TEXT REFERENCES xz_commission_wallet_ledger(id),
  ADD COLUMN IF NOT EXISTS reversal_wallet_ledger_id TEXT REFERENCES xz_commission_wallet_ledger(id),
  ADD COLUMN IF NOT EXISTS refund_task_id TEXT REFERENCES xz_operation_center_refund_tasks(id),
  ADD COLUMN IF NOT EXISTS current_release_task_id TEXT REFERENCES xz_referral_reward_release_tasks(id),
  ADD COLUMN IF NOT EXISTS recoverable_cents BIGINT NOT NULL DEFAULT 0;

UPDATE xz_referral_rewards reward
SET commercial_rule_set_id = rule_version.rule_set_id,
    updated_at = reward.updated_at
FROM xz_referral_reward_rule_versions rule_version
WHERE reward.reward_rule_id = rule_version.id
  AND reward.commercial_rule_set_id IS NULL;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_referral_rewards'::regclass
      AND conname = 'ck_xz_referral_rewards_recoverable_089'
  ) THEN
    ALTER TABLE xz_referral_rewards
      ADD CONSTRAINT ck_xz_referral_rewards_recoverable_089
      CHECK (recoverable_cents >= 0 AND recoverable_cents <= abs(amount_cents));
  END IF;
END;
$$;

CREATE INDEX IF NOT EXISTS idx_xz_referral_rewards_rule_set_089
  ON xz_referral_rewards(commercial_rule_set_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_referral_rewards_refund_task_089
  ON xz_referral_rewards(refund_task_id, created_at)
  WHERE refund_task_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_referral_rewards_release_task_089
  ON xz_referral_rewards(current_release_task_id)
  WHERE current_release_task_id IS NOT NULL;

ALTER TABLE xz_commission_wallet_ledger
  ADD COLUMN IF NOT EXISTS referral_reward_id TEXT REFERENCES xz_referral_rewards(id),
  ADD COLUMN IF NOT EXISTS original_ledger_id TEXT REFERENCES xz_commission_wallet_ledger(id),
  ADD COLUMN IF NOT EXISTS refund_task_id TEXT REFERENCES xz_operation_center_refund_tasks(id),
  ADD COLUMN IF NOT EXISTS recoverable_cents_delta BIGINT NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_commission_wallet_ledger_refund_original_089
  ON xz_commission_wallet_ledger(refund_task_id, original_ledger_id)
  WHERE refund_task_id IS NOT NULL AND original_ledger_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_xz_commission_wallet_ledger_referral_reward_089
  ON xz_commission_wallet_ledger(referral_reward_id, created_at, id)
  WHERE referral_reward_id IS NOT NULL;

CREATE OR REPLACE FUNCTION xz_protect_operation_center_refund_identity_089()
RETURNS trigger AS $$
BEGIN
  IF NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
     OR NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
     OR NEW.service_order_id IS DISTINCT FROM OLD.service_order_id
     OR NEW.order_id IS DISTINCT FROM OLD.order_id
     OR NEW.payment_record_id IS DISTINCT FROM OLD.payment_record_id
     OR NEW.commercial_rule_set_id IS DISTINCT FROM OLD.commercial_rule_set_id
     OR NEW.refund_scope IS DISTINCT FROM OLD.refund_scope
     OR NEW.amount_cents IS DISTINCT FROM OLD.amount_cents
     OR NEW.currency IS DISTINCT FROM OLD.currency
     OR NEW.payment_channel IS DISTINCT FROM OLD.payment_channel THEN
    RAISE EXCEPTION 'operation center refund identity is immutable';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_oc_refund_identity_immutable_089 ON xz_operation_center_refund_tasks;
CREATE TRIGGER trg_xz_oc_refund_identity_immutable_089
BEFORE UPDATE ON xz_operation_center_refund_tasks
FOR EACH ROW EXECUTE FUNCTION xz_protect_operation_center_refund_identity_089();

CREATE OR REPLACE FUNCTION xz_protect_idempotency_key_089()
RETURNS trigger AS $$
BEGIN
  IF NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key THEN
    RAISE EXCEPTION 'idempotency key is immutable';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_oc_review_idempotency_immutable_089 ON xz_operation_center_review_events;
CREATE TRIGGER trg_xz_oc_review_idempotency_immutable_089
BEFORE UPDATE ON xz_operation_center_review_events
FOR EACH ROW EXECUTE FUNCTION xz_protect_idempotency_key_089();

DROP TRIGGER IF EXISTS trg_xz_referral_release_idempotency_immutable_089 ON xz_referral_reward_release_tasks;
CREATE TRIGGER trg_xz_referral_release_idempotency_immutable_089
BEFORE UPDATE ON xz_referral_reward_release_tasks
FOR EACH ROW EXECUTE FUNCTION xz_protect_idempotency_key_089();

CREATE OR REPLACE FUNCTION xz_protect_oc_service_idempotency_089()
RETURNS trigger AS $$
BEGIN
  IF OLD.review_idempotency_key IS NOT NULL
     AND NEW.review_idempotency_key IS DISTINCT FROM OLD.review_idempotency_key THEN
    RAISE EXCEPTION 'operation center review idempotency key is immutable';
  END IF;
  IF OLD.refund_idempotency_key IS NOT NULL
     AND NEW.refund_idempotency_key IS DISTINCT FROM OLD.refund_idempotency_key THEN
    RAISE EXCEPTION 'operation center refund idempotency key is immutable';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_oc_service_idempotency_immutable_089 ON xz_operation_center_service_orders;
CREATE TRIGGER trg_xz_oc_service_idempotency_immutable_089
BEFORE UPDATE ON xz_operation_center_service_orders
FOR EACH ROW EXECUTE FUNCTION xz_protect_oc_service_idempotency_089();

CREATE OR REPLACE FUNCTION xz_protect_oc_state_transition_089()
RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'operation center state transitions are append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_oc_state_transition_append_only_089 ON xz_operation_center_state_transitions;
CREATE TRIGGER trg_xz_oc_state_transition_append_only_089
BEFORE UPDATE OR DELETE ON xz_operation_center_state_transitions
FOR EACH ROW EXECUTE FUNCTION xz_protect_oc_state_transition_089();

INSERT INTO permissions (code, name, module, action)
VALUES
  ('channel:operation-center:review', 'Review operation center application', 'channel', 'operation_center_review'),
  ('channel:operation-center:refund', 'Request operation center refund', 'channel', 'operation_center_refund'),
  ('finance:operation-center-refund:view', 'View operation center refunds', 'finance', 'operation_center_refund_view'),
  ('finance:operation-center-refund:retry', 'Retry operation center refund', 'finance', 'operation_center_refund_retry'),
  ('finance:operation-center-refund:verify', 'Verify operation center refund', 'finance', 'operation_center_refund_verify'),
  ('finance:operation-center-refund:manual-submit', 'Submit manual operation center refund', 'finance', 'operation_center_refund_manual_submit'),
  ('finance:operation-center-refund:manual-approve', 'Approve manual operation center refund', 'finance', 'operation_center_refund_manual_approve')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name, module = EXCLUDED.module, action = EXCLUDED.action;

WITH matrix(role_code, permission_code) AS (
  VALUES
    ('SUPER_ADMIN', 'channel:operation-center:review'),
    ('SUPER_ADMIN', 'channel:operation-center:refund'),
    ('SUPER_ADMIN', 'finance:operation-center-refund:view'),
    ('SUPER_ADMIN', 'finance:operation-center-refund:retry'),
    ('SUPER_ADMIN', 'finance:operation-center-refund:verify'),
    ('SUPER_ADMIN', 'finance:operation-center-refund:manual-submit'),
    ('SUPER_ADMIN', 'finance:operation-center-refund:manual-approve'),
    ('FINANCE', 'finance:operation-center-refund:view'),
    ('FINANCE', 'finance:operation-center-refund:retry'),
    ('FINANCE', 'finance:operation-center-refund:verify'),
    ('FINANCE', 'finance:operation-center-refund:manual-submit'),
    ('FINANCE', 'finance:operation-center-refund:manual-approve')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT role_row.id, permission_row.id
FROM matrix
JOIN roles role_row ON role_row.code = matrix.role_code
JOIN permissions permission_row ON permission_row.code = matrix.permission_code
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO xz_role_permissions (role, permission)
VALUES
  ('SUPER_ADMIN', 'channel:operation-center:review'),
  ('SUPER_ADMIN', 'channel:operation-center:refund'),
  ('SUPER_ADMIN', 'finance:operation-center-refund:view'),
  ('SUPER_ADMIN', 'finance:operation-center-refund:retry'),
  ('SUPER_ADMIN', 'finance:operation-center-refund:verify'),
  ('SUPER_ADMIN', 'finance:operation-center-refund:manual-submit'),
  ('SUPER_ADMIN', 'finance:operation-center-refund:manual-approve'),
  ('FINANCE', 'finance:operation-center-refund:view'),
  ('FINANCE', 'finance:operation-center-refund:retry'),
  ('FINANCE', 'finance:operation-center-refund:verify'),
  ('FINANCE', 'finance:operation-center-refund:manual-submit'),
  ('FINANCE', 'finance:operation-center-refund:manual-approve')
ON CONFLICT (role, permission) DO NOTHING;

COMMIT;
