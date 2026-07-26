-- Operation-center refund management, schedulers and manual-refund audit.
-- Additive forward migration only. It does not enable schedulers or rollout modes.

BEGIN;

CREATE TABLE IF NOT EXISTS xz_operation_center_refund_request_events (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  service_order_id TEXT NOT NULL REFERENCES xz_operation_center_service_orders(id),
  refund_task_id TEXT NOT NULL REFERENCES xz_operation_center_refund_tasks(id),
  requested_by TEXT NOT NULL REFERENCES xz_users(id),
  request_id TEXT,
  idempotency_key TEXT NOT NULL CHECK (btrim(idempotency_key) <> ''),
  reason TEXT NOT NULL CHECK (btrim(reason) <> ''),
  expected_service_status TEXT NOT NULL CHECK (expected_service_status = 'ACTIVE'),
  request_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(request_snapshot) = 'object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, idempotency_key),
  UNIQUE (service_order_id, refund_task_id)
);

CREATE INDEX IF NOT EXISTS idx_xz_oc_refund_request_service_096
  ON xz_operation_center_refund_request_events(tenant_id, service_order_id, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_operation_center_manual_refund_events (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  refund_task_id TEXT NOT NULL REFERENCES xz_operation_center_refund_tasks(id),
  manual_refund_id TEXT NOT NULL REFERENCES xz_operation_center_manual_refunds(id),
  event_type TEXT NOT NULL CHECK (event_type IN ('SUBMITTED', 'APPROVED', 'REJECTED')),
  actor_id TEXT NOT NULL REFERENCES xz_users(id),
  request_id TEXT,
  idempotency_key TEXT NOT NULL CHECK (btrim(idempotency_key) <> ''),
  reason TEXT NOT NULL CHECK (btrim(reason) <> ''),
  before_status TEXT NOT NULL CHECK (before_status IN ('MANUAL_REQUIRED', 'MANUAL_SUBMITTED')),
  after_status TEXT NOT NULL CHECK (after_status IN ('MANUAL_REQUIRED', 'MANUAL_SUBMITTED', 'SUCCEEDED')),
  event_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(event_snapshot) = 'object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_xz_oc_manual_refund_events_task_096
  ON xz_operation_center_manual_refund_events(refund_task_id, created_at DESC, id DESC);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_operation_center_refund_tasks'::regclass
      AND conname = 'ck_xz_oc_refund_tasks_manual_submitted_evidence_096'
  ) THEN
    ALTER TABLE xz_operation_center_refund_tasks
      ADD CONSTRAINT ck_xz_oc_refund_tasks_manual_submitted_evidence_096 CHECK (
        refund_status <> 'MANUAL_SUBMITTED'
        OR (
          manual_provider_transaction_id IS NOT NULL
          AND btrim(manual_provider_transaction_id) <> ''
          AND manual_voucher_reference IS NOT NULL
          AND btrim(manual_voucher_reference) <> ''
          AND manual_voucher_file_hash ~ '^[0-9a-f]{64}$'
          AND manual_submitted_by IS NOT NULL
          AND manual_approved_by IS NULL
        )
      );
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'xz_operation_center_refund_tasks'::regclass
      AND conname = 'ck_xz_oc_refund_tasks_success_evidence_096'
  ) THEN
    ALTER TABLE xz_operation_center_refund_tasks
      ADD CONSTRAINT ck_xz_oc_refund_tasks_success_evidence_096 CHECK (
        refund_status <> 'SUCCEEDED'
        OR (
          provider_outcome = 'SUCCESS'
          AND provider_refund_no IS NOT NULL
          AND btrim(provider_refund_no) <> ''
          AND provider_refunded_at IS NOT NULL
        )
        OR (
          manual_provider_transaction_id IS NOT NULL
          AND btrim(manual_provider_transaction_id) <> ''
          AND manual_voucher_reference IS NOT NULL
          AND btrim(manual_voucher_reference) <> ''
          AND manual_voucher_file_hash ~ '^[0-9a-f]{64}$'
          AND manual_submitted_by IS NOT NULL
          AND manual_approved_by IS NOT NULL
          AND manual_submitted_by <> manual_approved_by
          AND provider_refunded_at IS NOT NULL
        )
      );
  END IF;
END;
$$;

CREATE INDEX IF NOT EXISTS idx_xz_oc_refund_tasks_retry_scheduler_096
  ON xz_operation_center_refund_tasks(next_retry_at, lease_expires_at, created_at, id)
  WHERE refund_status = 'REFUND_RETRYABLE';

CREATE INDEX IF NOT EXISTS idx_xz_oc_refund_tasks_verify_scheduler_096
  ON xz_operation_center_refund_tasks(next_retry_at, lease_expires_at, created_at, id)
  WHERE refund_status = 'UNKNOWN_VERIFYING';

CREATE INDEX IF NOT EXISTS idx_xz_oc_refund_tasks_admin_query_096
  ON xz_operation_center_refund_tasks(tenant_id, refund_status, payment_channel, created_at DESC, id);

CREATE OR REPLACE FUNCTION xz_assert_operation_center_refund_service_invariant_096()
RETURNS TRIGGER AS $$
DECLARE
  target_service_id TEXT;
  active_violation BOOLEAN;
BEGIN
  IF TG_TABLE_NAME = 'xz_operation_center_refund_tasks' THEN
    target_service_id := NEW.service_order_id;
  ELSE
    target_service_id := NEW.id;
  END IF;

  SELECT EXISTS (
    SELECT 1
    FROM xz_operation_center_service_orders service
    JOIN xz_operation_center_refund_tasks task ON task.service_order_id = service.id
    WHERE service.id = target_service_id
      AND service.status = 'ACTIVE'
      AND task.refund_status IN (
        'PROVIDER_PENDING', 'REFUND_RETRYABLE', 'UNKNOWN_VERIFYING',
        'MANUAL_REQUIRED', 'MANUAL_SUBMITTED', 'SUCCEEDED'
      )
  ) INTO active_violation;

  IF active_violation THEN
    RAISE EXCEPTION 'operation center cannot remain ACTIVE after refund processing starts'
      USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'ct_xz_oc_refund_task_service_invariant_096') THEN
    CREATE CONSTRAINT TRIGGER ct_xz_oc_refund_task_service_invariant_096
      AFTER INSERT OR UPDATE OF refund_status, service_order_id
      ON xz_operation_center_refund_tasks
      DEFERRABLE INITIALLY DEFERRED
      FOR EACH ROW EXECUTE FUNCTION xz_assert_operation_center_refund_service_invariant_096();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'ct_xz_oc_service_refund_invariant_096') THEN
    CREATE CONSTRAINT TRIGGER ct_xz_oc_service_refund_invariant_096
      AFTER UPDATE OF status
      ON xz_operation_center_service_orders
      DEFERRABLE INITIALLY DEFERRED
      FOR EACH ROW EXECUTE FUNCTION xz_assert_operation_center_refund_service_invariant_096();
  END IF;
END;
$$;

COMMIT;
