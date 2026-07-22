-- Controlled AGENT / OPERATION_CENTER downgrade workflow.
BEGIN;

CREATE TABLE IF NOT EXISTS xz_identity_downgrade_previews (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  user_id TEXT NOT NULL REFERENCES xz_users(id),
  actor_id TEXT NOT NULL REFERENCES xz_users(id),
  actor_role TEXT NOT NULL,
  current_identity TEXT NOT NULL CHECK (current_identity IN ('AGENT', 'OPERATION_CENTER')),
  target_identity TEXT CHECK (target_identity IS NULL OR target_identity = 'AGENT'),
  request_snapshot JSONB NOT NULL,
  result_snapshot JSONB NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('READY', 'BLOCKED', 'WAITING', 'SCHEDULED', 'CONSUMED', 'EXPIRED')),
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  request_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_xz_identity_downgrade_previews_user
  ON xz_identity_downgrade_previews(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_identity_downgrade_requests (
  id TEXT PRIMARY KEY,
  preview_id TEXT NOT NULL UNIQUE REFERENCES xz_identity_downgrade_previews(id),
  user_id TEXT NOT NULL REFERENCES xz_users(id),
  actor_id TEXT NOT NULL REFERENCES xz_users(id),
  current_identity TEXT NOT NULL CHECK (current_identity IN ('AGENT', 'OPERATION_CENTER')),
  target_identity TEXT CHECK (target_identity IS NULL OR target_identity = 'AGENT'),
  child_strategy TEXT NOT NULL CHECK (child_strategy IN ('TRANSFER_TO_AGENT', 'DIRECT_OPERATION_CENTER', 'PRESERVE_HISTORY')),
  target_agent_id TEXT REFERENCES xz_channel_agents(id),
  target_operation_center_id TEXT REFERENCES xz_operation_centers(id),
  wait_for_settlement BOOLEAN NOT NULL DEFAULT FALSE,
  effective_at TIMESTAMPTZ NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('WAITING', 'SCHEDULED', 'PROCESSING', 'SUCCEEDED', 'FAILED', 'CANCELLED')),
  blocker_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb,
  result_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  reason TEXT NOT NULL CHECK (length(btrim(reason)) > 0),
  remark TEXT NOT NULL DEFAULT '',
  failure_message TEXT NOT NULL DEFAULT '',
  last_checked_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_xz_identity_downgrade_due
  ON xz_identity_downgrade_requests(status, effective_at)
  WHERE status IN ('WAITING', 'SCHEDULED');
CREATE INDEX IF NOT EXISTS idx_xz_identity_downgrade_user
  ON xz_identity_downgrade_requests(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_migration_068_added_role_permissions (
  role TEXT NOT NULL,
  permission TEXT NOT NULL,
  PRIMARY KEY(role, permission)
);

WITH desired(role, permission) AS (
  VALUES
    ('SUPER_ADMIN', 'identity:downgrade:preview'),
    ('SUPER_ADMIN', 'identity:downgrade:confirm'),
    ('SUPER_ADMIN', 'identity:downgrade:view')
), missing AS (
  SELECT desired.* FROM desired
  LEFT JOIN xz_role_permissions existing
    ON existing.role=desired.role AND existing.permission=desired.permission
  WHERE existing.role IS NULL
)
INSERT INTO xz_migration_068_added_role_permissions(role, permission)
SELECT role, permission FROM missing
ON CONFLICT DO NOTHING;

INSERT INTO xz_role_permissions(role, permission)
SELECT role, permission FROM xz_migration_068_added_role_permissions
ON CONFLICT DO NOTHING;

COMMIT;
