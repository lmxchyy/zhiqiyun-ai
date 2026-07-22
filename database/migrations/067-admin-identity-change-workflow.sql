BEGIN;

CREATE TABLE IF NOT EXISTS xz_identity_change_previews (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  user_id TEXT NOT NULL REFERENCES xz_users(id),
  actor_id TEXT NOT NULL REFERENCES xz_users(id),
  actor_role TEXT NOT NULL,
  change_action TEXT NOT NULL CHECK (change_action IN (
    'UPGRADE', 'FREEZE', 'RESTORE', 'TERMINATE',
    'ADJUST_PARENT_AGENT', 'ADJUST_OPERATION_CENTER'
  )),
  change_method TEXT NOT NULL CHECK (change_method IN (
    'ONLY_IDENTITY', 'OFFLINE_ORDER', 'SPECIAL_GRANT', 'PACKAGE_CONVERSION'
  )),
  target_identity TEXT CHECK (target_identity IS NULL OR target_identity IN ('AGENT', 'OPERATION_CENTER')),
  request_snapshot JSONB NOT NULL,
  result_snapshot JSONB NOT NULL,
  status TEXT NOT NULL CHECK (status IN (
    'READY', 'BLOCKED', 'REVIEW_REQUIRED', 'APPROVED', 'REJECTED', 'CONSUMED', 'EXPIRED'
  )),
  high_risk BOOLEAN NOT NULL DEFAULT FALSE,
  source_membership_order_id TEXT,
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  execution_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_xz_identity_change_previews_user
  ON xz_identity_change_previews(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_identity_change_previews_expiry
  ON xz_identity_change_previews(status, expires_at);

CREATE TABLE IF NOT EXISTS xz_identity_change_approvals (
  id TEXT PRIMARY KEY,
  preview_id TEXT NOT NULL UNIQUE REFERENCES xz_identity_change_previews(id),
  reviewer_id TEXT NOT NULL REFERENCES xz_users(id),
  reviewer_role TEXT NOT NULL,
  decision TEXT NOT NULL CHECK (decision IN ('APPROVED', 'REJECTED')),
  reason TEXT NOT NULL CHECK (length(btrim(reason)) > 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS xz_identity_change_executions (
  id TEXT PRIMARY KEY,
  preview_id TEXT NOT NULL UNIQUE REFERENCES xz_identity_change_previews(id),
  user_id TEXT NOT NULL REFERENCES xz_users(id),
  actor_id TEXT NOT NULL REFERENCES xz_users(id),
  actor_role TEXT NOT NULL,
  change_action TEXT NOT NULL,
  change_method TEXT NOT NULL,
  source_membership_order_id TEXT,
  order_id TEXT REFERENCES xz_orders(id),
  status TEXT NOT NULL CHECK (status IN ('PROCESSING', 'SUCCEEDED', 'FAILED')),
  result_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_identity_conversion_source_order
  ON xz_identity_change_executions(source_membership_order_id)
  WHERE change_method = 'PACKAGE_CONVERSION'
    AND source_membership_order_id IS NOT NULL
    AND status IN ('PROCESSING', 'SUCCEEDED');

CREATE OR REPLACE FUNCTION xz_protect_identity_change_workflow_history()
RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_identity_change_approvals_immutable ON xz_identity_change_approvals;
CREATE TRIGGER trg_xz_identity_change_approvals_immutable
BEFORE UPDATE OR DELETE ON xz_identity_change_approvals
FOR EACH ROW EXECUTE FUNCTION xz_protect_identity_change_workflow_history();

CREATE TABLE IF NOT EXISTS xz_migration_067_added_role_permissions (
  role TEXT NOT NULL,
  permission TEXT NOT NULL,
  PRIMARY KEY(role, permission)
);

WITH desired(role, permission) AS (
  VALUES
    ('SUPER_ADMIN', 'identity:change:preview'),
    ('SUPER_ADMIN', 'identity:change:confirm'),
    ('SUPER_ADMIN', 'identity:change:review'),
    ('ADMIN', 'identity:change:preview'),
    ('ADMIN', 'identity:change:confirm'),
    ('ADMIN', 'identity:change:review')
), missing AS (
  SELECT desired.* FROM desired
  LEFT JOIN xz_role_permissions existing
    ON existing.role=desired.role AND existing.permission=desired.permission
  WHERE existing.role IS NULL
)
INSERT INTO xz_migration_067_added_role_permissions(role, permission)
SELECT role, permission FROM missing
ON CONFLICT DO NOTHING;

INSERT INTO xz_role_permissions(role, permission)
SELECT role, permission FROM xz_migration_067_added_role_permissions
ON CONFLICT DO NOTHING;

COMMIT;
