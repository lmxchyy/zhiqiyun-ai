-- 120-provider-execution-correlation.sql
-- Immutable, redacted Provider route/job/state evidence per execution attempt.
BEGIN;

CREATE TABLE IF NOT EXISTS provider_execution_correlations (
  id BIGSERIAL PRIMARY KEY,
  execution_id BIGINT NOT NULL REFERENCES provider_executions(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  provider_code TEXT NOT NULL DEFAULT '',
  base_url_host TEXT NOT NULL DEFAULT '',
  endpoint_path TEXT NOT NULL DEFAULT '',
  provider_job_id TEXT,
  job_role TEXT NOT NULL DEFAULT '',
  provider_state TEXT NOT NULL DEFAULT '',
  http_status INTEGER,
  error_code TEXT NOT NULL DEFAULT '',
  error_hash TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS provider_execution_correlations_execution_idx
  ON provider_execution_correlations(execution_id, id);
CREATE INDEX IF NOT EXISTS provider_execution_correlations_job_idx
  ON provider_execution_correlations(provider_job_id)
  WHERE provider_job_id IS NOT NULL;

COMMIT;
