-- Roll back Payment refund adapter query-verification persistence only.
--
-- Operational requirement: stop 095 verifier writes and back up the five
-- columns below before applying this rollback. Their stored provider query
-- evidence and verification counters are destroyed and cannot be reconstructed
-- from the pre-095 schema. Refund Saga state introduced by 094 is preserved.

BEGIN;

DROP INDEX IF EXISTS idx_xz_oc_refund_unknown_verification_095;

ALTER TABLE xz_operation_center_refund_tasks
  DROP CONSTRAINT IF EXISTS ck_xz_oc_refund_provider_summaries_095,
  DROP CONSTRAINT IF EXISTS ck_xz_oc_refund_query_outcome_095,
  DROP CONSTRAINT IF EXISTS ck_xz_oc_refund_verification_count_095,
  DROP COLUMN IF EXISTS provider_response_summary,
  DROP COLUMN IF EXISTS provider_query_outcome,
  DROP COLUMN IF EXISTS provider_query_response_summary,
  DROP COLUMN IF EXISTS verification_attempt_count,
  DROP COLUMN IF EXISTS last_verification_at;

COMMIT;
