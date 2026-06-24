CREATE TABLE IF NOT EXISTS xz_audit_logs (
  id TEXT PRIMARY KEY,
  actor_id TEXT,
  actor_role TEXT,
  action TEXT NOT NULL,
  resource TEXT NOT NULL,
  resource_id TEXT,
  method TEXT,
  path TEXT,
  status INT,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS xz_operation_logs (
  id TEXT PRIMARY KEY,
  actor_id TEXT,
  operation TEXT NOT NULL,
  target TEXT NOT NULL,
  target_id TEXT,
  before_state JSONB,
  after_state JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS xz_role_permissions (
  role TEXT NOT NULL,
  permission TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (role, permission)
);

CREATE TABLE IF NOT EXISTS xz_backup_runs (
  id TEXT PRIMARY KEY,
  status TEXT NOT NULL,
  target TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at TIMESTAMPTZ
);

INSERT INTO xz_role_permissions (role, permission)
VALUES
  ('SUPER_ADMIN', 'admin.full'),
  ('SUPER_ADMIN', 'audit.read'),
  ('SUPER_ADMIN', 'backup.manage'),
  ('ADMIN', 'admin.read'),
  ('ADMIN', 'admin.write'),
  ('FINANCE', 'orders.read'),
  ('FINANCE', 'commissions.write'),
  ('CHANNEL_MANAGER', 'channel.write'),
  ('DELIVERY_MANAGER', 'delivery.write'),
  ('AGENT_L1', 'channel.dashboard'),
  ('AGENT_L2', 'channel.dashboard'),
  ('MEMBER', 'generation.create'),
  ('MEMBER', 'assets.read')
ON CONFLICT (role, permission) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_xz_audit_logs_actor_id ON xz_audit_logs(actor_id);
CREATE INDEX IF NOT EXISTS idx_xz_audit_logs_created_at ON xz_audit_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_xz_operation_logs_target ON xz_operation_logs(target, target_id);
