BEGIN;

CREATE TABLE IF NOT EXISTS provider_executions (
    id BIGSERIAL PRIMARY KEY,
    task_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    provider_channel TEXT NOT NULL DEFAULT '',
    provider_model TEXT NOT NULL DEFAULT '',
    capability TEXT NOT NULL,
    attempt INTEGER NOT NULL CHECK (attempt > 0),
    status TEXT NOT NULL CHECK (status IN ('prepared','submitting','submitted','processing','succeeded','failed','unknown')),
    request_fingerprint CHAR(64) NOT NULL,
    provider_operation_key TEXT NOT NULL DEFAULT '',
    provider_request_id TEXT,
    result_metadata JSONB,
    submitted_at TIMESTAMPTZ,
    processing_at TIMESTAMPTZ,
    succeeded_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    unknown_at TIMESTAMPTZ,
    last_checked_at TIMESTAMPTZ,
    next_check_at TIMESTAMPTZ,
    error_code TEXT,
    error_class TEXT,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT provider_executions_task_attempt_unique UNIQUE (task_id, attempt)
);

-- Keep this migration replayable for databases that ran an earlier PR6 draft.
ALTER TABLE provider_executions ADD COLUMN IF NOT EXISTS provider_operation_key TEXT NOT NULL DEFAULT '';
ALTER TABLE provider_executions ADD COLUMN IF NOT EXISTS result_metadata JSONB;

CREATE UNIQUE INDEX IF NOT EXISTS provider_executions_active_task_unique
 ON provider_executions(task_id)
 WHERE status NOT IN ('succeeded','failed');
CREATE INDEX IF NOT EXISTS provider_executions_recovery_idx
 ON provider_executions(status,next_check_at);
CREATE INDEX IF NOT EXISTS provider_executions_fingerprint_idx
 ON provider_executions(task_id,request_fingerprint);
CREATE INDEX IF NOT EXISTS provider_executions_provider_request_idx
 ON provider_executions(provider,provider_request_id)
 WHERE provider_request_id IS NOT NULL;

COMMIT;
