CREATE TABLE IF NOT EXISTS xz_admin_exception_cases (
  id TEXT PRIMARY KEY,
  exception_key TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  module_id TEXT NOT NULL,
  severity TEXT NOT NULL DEFAULT 'warning',
  current_count INTEGER NOT NULL DEFAULT 0,
  roles JSONB NOT NULL DEFAULT '[]'::jsonb,
  assignee_id TEXT NOT NULL DEFAULT '',
  assignee_name TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'OPEN',
  sla_due_at TIMESTAMPTZ,
  first_detected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  closed_at TIMESTAMPTZ,
  close_reason TEXT NOT NULL DEFAULT '',
  history JSONB NOT NULL DEFAULT '[]'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_admin_exception_cases_status_sla ON xz_admin_exception_cases(status, sla_due_at);

CREATE TABLE IF NOT EXISTS xz_admin_experience_events (
  id TEXT PRIMARY KEY,
  actor_id TEXT NOT NULL DEFAULT '',
  actor_role TEXT NOT NULL DEFAULT '',
  event_type TEXT NOT NULL,
  module_id TEXT NOT NULL DEFAULT '',
  target_id TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_admin_experience_events_time_type ON xz_admin_experience_events(occurred_at DESC, event_type);
CREATE INDEX IF NOT EXISTS idx_admin_experience_events_module ON xz_admin_experience_events(module_id, occurred_at DESC);
