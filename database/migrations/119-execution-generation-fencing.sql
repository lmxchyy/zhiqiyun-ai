-- 119-execution-generation-fencing.sql
--
-- Issue #145 (GO_IMPLEMENT): execution-generation fencing for generation tasks.
--
-- Root cause: task settlement (Complete / Fail / Durable / UnknownGrace /
-- MANUAL_REVIEW / Capture / Release) is guarded only by row-lock
-- serialization + terminal-first-wins. Any live-or-resurrected holder of a
-- task id can commit final state, artifacts, and billing after its lease
-- expired and the task was requeued to a new owner.
--
-- This migration adds the fencing token (execution_generation) plus the
-- ownership/lease columns consumed by the claim/requeue/settlement predicates
-- in backend-go/internal/httpserver/generation_fencing.go, and binds each
-- provider execution row to the task generation it was created under so a
-- stale attempt's late success can never settle the current generation.
--
-- Replay/idempotency: every statement is IF NOT EXISTS / guarded, matching
-- the replayable style of 114-provider-execution-safety.sql. Safe to apply
-- multiple times and safe on databases that already carry these columns.
--
-- Backfill: execution_generation is NOT NULL DEFAULT 1, so pre-existing rows
-- fence correctly from the first claim/requeue bump onward. A guarded UPDATE
-- covers columns that may have been added NULLABLE by an older partial apply.
-- provider_executions.task_execution_generation stays NULL for pre-fencing
-- rows; NULL means "legacy, compare skipped" (see checkSucceededExecution-
-- Generation), never "stale".
--
-- Rollback (forward-fix preferred; requeue bumps rather than drops):
--   ALTER TABLE xz_generation_tasks DROP COLUMN IF EXISTS last_heartbeat_at;
--   ALTER TABLE xz_generation_tasks DROP COLUMN IF EXISTS lease_until;
--   ALTER TABLE xz_generation_tasks DROP COLUMN IF EXISTS worker_id;
--   ALTER TABLE xz_generation_tasks DROP COLUMN IF EXISTS execution_generation;
--   ALTER TABLE provider_executions DROP COLUMN IF EXISTS task_execution_generation;
--   DROP INDEX IF EXISTS xz_generation_tasks_fencing_status_gen_idx;
--   DROP INDEX IF EXISTS xz_generation_tasks_fencing_lease_idx;
--   DROP INDEX IF EXISTS provider_executions_task_generation_idx;
-- No status/billing/artifact rows are touched, so dropping the columns only
-- resumes unfenced (pre-#145) semantics.

BEGIN;

-- (a) Task fencing token + ownership lease. DB now() is the lease clock:
-- eligibility queries compare lease_until against now() so writers never
-- depend on a single host's wall clock.
ALTER TABLE xz_generation_tasks
  ADD COLUMN IF NOT EXISTS execution_generation BIGINT NOT NULL DEFAULT 1;
ALTER TABLE xz_generation_tasks
  ADD COLUMN IF NOT EXISTS worker_id TEXT;
ALTER TABLE xz_generation_tasks
  ADD COLUMN IF NOT EXISTS lease_until TIMESTAMPTZ;
ALTER TABLE xz_generation_tasks
  ADD COLUMN IF NOT EXISTS last_heartbeat_at TIMESTAMPTZ;

-- Guarded backfill for replay scenarios where the column predates this file.
UPDATE xz_generation_tasks
  SET execution_generation = 1
  WHERE execution_generation IS NULL;

-- Scheduler/reaper scans: (task_status, generation) for ownership transfer
-- lookups, partial lease index for `lease_until < now()` eligibility sweeps.
CREATE INDEX IF NOT EXISTS xz_generation_tasks_fencing_status_gen_idx
  ON xz_generation_tasks (task_status, execution_generation);
CREATE INDEX IF NOT EXISTS xz_generation_tasks_fencing_lease_idx
  ON xz_generation_tasks (lease_until)
  WHERE lease_until IS NOT NULL;

-- (b) Durable execution -> generation binding. Nullable by design: NULL =
-- pre-fencing execution, comparison skipped for compatibility.
ALTER TABLE provider_executions
  ADD COLUMN IF NOT EXISTS task_execution_generation BIGINT;

CREATE INDEX IF NOT EXISTS provider_executions_task_generation_idx
  ON provider_executions (task_id, task_execution_generation);

COMMIT;
