-- Operation-center refund Saga provider completion contract.
-- Provider invocation remains outside database transactions.

BEGIN;

ALTER TABLE xz_operation_center_refund_tasks
  ADD COLUMN IF NOT EXISTS provider_refunded_at TIMESTAMPTZ;

UPDATE xz_operation_center_refund_tasks
SET provider_refunded_at=COALESCE(provider_refunded_at,completed_at,updated_at)
WHERE refund_status='SUCCEEDED' AND provider_refunded_at IS NULL;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='xz_operation_center_refund_tasks'::regclass
      AND conname='ck_xz_oc_refund_provider_completed_094'
  ) THEN
    ALTER TABLE xz_operation_center_refund_tasks
      ADD CONSTRAINT ck_xz_oc_refund_provider_completed_094 CHECK (
        refund_status<>'SUCCEEDED' OR provider_refunded_at IS NOT NULL
      );
  END IF;
END;
$$;

CREATE INDEX IF NOT EXISTS idx_xz_oc_refund_provider_completed_094
  ON xz_operation_center_refund_tasks(provider_refunded_at DESC, id)
  WHERE refund_status='SUCCEEDED';

COMMIT;
