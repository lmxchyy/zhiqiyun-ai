-- PR1: additive generation-task lifecycle state and provider-attempt observability.
-- Keep the legacy status/billing/provider columns intact; these fields are
-- nullable (apart from status/counters) so existing rows remain readable.
ALTER TABLE IF EXISTS xz_generation_tasks
  ADD COLUMN IF NOT EXISTS task_status TEXT NOT NULL DEFAULT 'CREATED',
  ADD COLUMN IF NOT EXISTS worker_id TEXT,
  ADD COLUMN IF NOT EXISTS lease_until TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS queue_timeout_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS timeout_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS submitted_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_heartbeat_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS finished_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS provider_request_id TEXT,
  ADD COLUMN IF NOT EXISTS provider_execution_id TEXT,
  ADD COLUMN IF NOT EXISTS error_code TEXT,
  ADD COLUMN IF NOT EXISTS error_class TEXT,
  ADD COLUMN IF NOT EXISTS error_message TEXT,
  ADD COLUMN IF NOT EXISTS attempt_count INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS last_checked_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS manual_review_reason TEXT;

ALTER TABLE IF EXISTS generation_task_attempts
  ADD COLUMN IF NOT EXISTS provider_execution_id TEXT,
  ADD COLUMN IF NOT EXISTS queued_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS submitted_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_heartbeat_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS error_code TEXT,
  ADD COLUMN IF NOT EXISTS error_class TEXT,
  ADD COLUMN IF NOT EXISTS error_message TEXT,
  ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE UNIQUE INDEX IF NOT EXISTS ux_generation_task_attempts_task_attempt
  ON generation_task_attempts (task_id, attempt);

CREATE INDEX IF NOT EXISTS idx_xz_generation_tasks_worker_lease
  ON xz_generation_tasks (worker_id, lease_until)
  WHERE worker_id IS NOT NULL AND lease_until IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_xz_generation_tasks_status_timeout
  ON xz_generation_tasks (task_status, timeout_at)
  WHERE timeout_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_xz_generation_tasks_provider_execution
  ON xz_generation_tasks (provider_execution_id)
  WHERE provider_execution_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_generation_task_attempts_task_created
  ON generation_task_attempts (task_id, created_at DESC);
