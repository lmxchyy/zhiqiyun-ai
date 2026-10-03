-- 121-provider-execution-quarantine.sql
-- Append-only runtime barrier. Empty by default: no historical execution is enrolled.
-- A row blocks recovery, provider calls, persistence and settlement. It is not a
-- Safe Drain exemption and does not prove provider termination.
BEGIN;

CREATE TABLE IF NOT EXISTS provider_execution_quarantine (
  execution_id BIGINT PRIMARY KEY REFERENCES provider_executions(id),
  task_id TEXT NOT NULL,
  attempt INTEGER NOT NULL CHECK (attempt > 0),
  generation BIGINT,
  snapshot_sha256 CHAR(64) NOT NULL,
  evidence_sha256 CHAR(64) NOT NULL,
  approval_id TEXT NOT NULL,
  release_sha CHAR(40) NOT NULL,
  not_before TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT provider_execution_quarantine_window CHECK (expires_at > not_before),
  CONSTRAINT provider_execution_quarantine_snapshot CHECK (snapshot_sha256 ~ '^[0-9a-f]{64}$'),
  CONSTRAINT provider_execution_quarantine_evidence CHECK (evidence_sha256 ~ '^[0-9a-f]{64}$'),
  CONSTRAINT provider_execution_quarantine_release CHECK (release_sha ~ '^[0-9a-f]{40}$'),
  CONSTRAINT provider_execution_quarantine_approval CHECK (length(btrim(approval_id)) > 0),
  CONSTRAINT provider_execution_quarantine_task CHECK (length(btrim(task_id)) > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS provider_execution_quarantine_task_attempt_uidx
  ON provider_execution_quarantine(task_id, attempt);
CREATE INDEX IF NOT EXISTS provider_execution_quarantine_task_idx
  ON provider_execution_quarantine(task_id);

CREATE OR REPLACE FUNCTION provider_execution_quarantine_immutable()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'provider_execution_quarantine is append-only';
END;
$$;

DROP TRIGGER IF EXISTS trg_provider_execution_quarantine_immutable ON provider_execution_quarantine;
CREATE TRIGGER trg_provider_execution_quarantine_immutable
  BEFORE UPDATE OR DELETE ON provider_execution_quarantine
  FOR EACH ROW EXECUTE FUNCTION provider_execution_quarantine_immutable();

COMMIT;
