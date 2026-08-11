-- Payment refund adapter response summaries and UNKNOWN verification state.

BEGIN;

ALTER TABLE xz_operation_center_refund_tasks
  ADD COLUMN IF NOT EXISTS provider_response_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS provider_query_outcome TEXT,
  ADD COLUMN IF NOT EXISTS provider_query_response_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS verification_attempt_count INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS last_verification_at TIMESTAMPTZ;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='xz_operation_center_refund_tasks'::regclass
      AND conname='ck_xz_oc_refund_provider_summaries_095'
  ) THEN
    ALTER TABLE xz_operation_center_refund_tasks
      ADD CONSTRAINT ck_xz_oc_refund_provider_summaries_095 CHECK (
        jsonb_typeof(provider_response_summary)='object'
        AND jsonb_typeof(provider_query_response_summary)='object'
      );
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='xz_operation_center_refund_tasks'::regclass
      AND conname='ck_xz_oc_refund_query_outcome_095'
  ) THEN
    ALTER TABLE xz_operation_center_refund_tasks
      ADD CONSTRAINT ck_xz_oc_refund_query_outcome_095 CHECK (
        provider_query_outcome IS NULL OR provider_query_outcome IN (
          'SUCCEEDED','NOT_FOUND','PROCESSING','FAILED','UNSUPPORTED','UNKNOWN'
        )
      );
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='xz_operation_center_refund_tasks'::regclass
      AND conname='ck_xz_oc_refund_verification_count_095'
  ) THEN
    ALTER TABLE xz_operation_center_refund_tasks
      ADD CONSTRAINT ck_xz_oc_refund_verification_count_095 CHECK (verification_attempt_count>=0);
  END IF;
END;
$$;

CREATE INDEX IF NOT EXISTS idx_xz_oc_refund_unknown_verification_095
  ON xz_operation_center_refund_tasks(next_retry_at, last_verification_at, id)
  WHERE refund_status='UNKNOWN_VERIFYING';

COMMIT;
