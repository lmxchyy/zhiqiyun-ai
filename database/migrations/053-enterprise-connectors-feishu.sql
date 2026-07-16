-- Enterprise connector foundation and Feishu phase-1 integration.
-- This migration is additive and reuses xz_tenants, xz_users, RBAC, generation,
-- asset, compute-ledger and audit tables.

BEGIN;

CREATE TABLE IF NOT EXISTS enterprise_connectors (
  id TEXT PRIMARY KEY,
  enterprise_id TEXT NOT NULL REFERENCES xz_tenants(id),
  connector_type TEXT NOT NULL CHECK (connector_type IN ('feishu')),
  connector_name TEXT NOT NULL DEFAULT '',
  connector_key TEXT NOT NULL UNIQUE,
  app_id TEXT NOT NULL DEFAULT '',
  app_secret_encrypted TEXT NOT NULL DEFAULT '',
  verification_token_encrypted TEXT NOT NULL DEFAULT '',
  encrypt_key_encrypted TEXT NOT NULL DEFAULT '',
  external_tenant_key TEXT NOT NULL DEFAULT '',
  bot_open_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'disabled' CHECK (status IN ('disabled','active','error')),
  config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  last_connected_at TIMESTAMPTZ,
  last_error_message TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (enterprise_id, connector_type)
);

CREATE INDEX IF NOT EXISTS idx_enterprise_connectors_enterprise
  ON enterprise_connectors(enterprise_id, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS connector_user_bindings (
  id TEXT PRIMARY KEY,
  enterprise_id TEXT NOT NULL REFERENCES xz_tenants(id),
  connector_id TEXT NOT NULL REFERENCES enterprise_connectors(id) ON DELETE CASCADE,
  platform TEXT NOT NULL CHECK (platform IN ('feishu')),
  external_user_id TEXT NOT NULL,
  external_union_id TEXT NOT NULL DEFAULT '',
  external_name TEXT NOT NULL DEFAULT '',
  external_avatar TEXT NOT NULL DEFAULT '',
  internal_user_id TEXT REFERENCES xz_users(id),
  permission_json JSONB NOT NULL DEFAULT '{"imageGenerate":true}'::jsonb,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
  last_active_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (connector_id, external_user_id)
);

CREATE INDEX IF NOT EXISTS idx_connector_bindings_enterprise
  ON connector_user_bindings(enterprise_id, connector_id, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS connector_messages (
  id TEXT PRIMARY KEY,
  enterprise_id TEXT NOT NULL REFERENCES xz_tenants(id),
  connector_id TEXT NOT NULL REFERENCES enterprise_connectors(id) ON DELETE CASCADE,
  platform TEXT NOT NULL CHECK (platform IN ('feishu')),
  external_message_id TEXT NOT NULL,
  external_chat_id TEXT NOT NULL DEFAULT '',
  external_user_id TEXT NOT NULL DEFAULT '',
  chat_type TEXT NOT NULL DEFAULT '',
  message_type TEXT NOT NULL DEFAULT '',
  direction TEXT NOT NULL CHECK (direction IN ('inbound','outbound')),
  content_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  raw_payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  processing_status TEXT NOT NULL DEFAULT 'received'
    CHECK (processing_status IN ('received','queued','processing','completed','ignored','failed')),
  error_message TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (platform, external_message_id)
);

CREATE INDEX IF NOT EXISTS idx_connector_messages_enterprise
  ON connector_messages(enterprise_id, connector_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_connector_messages_processing
  ON connector_messages(processing_status, created_at) WHERE processing_status IN ('received','queued','processing');

CREATE TABLE IF NOT EXISTS connector_ai_tasks (
  id TEXT PRIMARY KEY,
  enterprise_id TEXT NOT NULL REFERENCES xz_tenants(id),
  connector_id TEXT NOT NULL REFERENCES enterprise_connectors(id) ON DELETE CASCADE,
  binding_id TEXT REFERENCES connector_user_bindings(id),
  platform TEXT NOT NULL CHECK (platform IN ('feishu')),
  external_chat_id TEXT NOT NULL,
  external_message_id TEXT NOT NULL,
  task_type TEXT NOT NULL DEFAULT 'image.generate',
  intent TEXT NOT NULL DEFAULT '',
  original_text TEXT NOT NULL DEFAULT '',
  optimized_prompt TEXT NOT NULL DEFAULT '',
  model_id TEXT NOT NULL DEFAULT '',
  platform_task_id TEXT,
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','processing','succeeded','failed','ignored')),
  progress INT NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
  result_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  token_cost BIGINT NOT NULL DEFAULT 0 CHECK (token_cost >= 0),
  points_cost BIGINT NOT NULL DEFAULT 0 CHECK (points_cost >= 0),
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (platform, external_message_id),
  UNIQUE (platform_task_id)
);

CREATE INDEX IF NOT EXISTS idx_connector_ai_tasks_enterprise
  ON connector_ai_tasks(enterprise_id, connector_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_connector_ai_tasks_binding_day
  ON connector_ai_tasks(enterprise_id, binding_id, created_at DESC);

INSERT INTO permissions(code, name, module, action)
VALUES
  ('enterprise.connector.read', 'View enterprise connectors', 'enterprise', 'connector_read'),
  ('enterprise.connector.manage', 'Manage enterprise connectors', 'enterprise', 'connector_manage')
ON CONFLICT (code) DO UPDATE SET name=excluded.name, module=excluded.module, action=excluded.action;

INSERT INTO xz_role_permissions(role, permission)
SELECT * FROM (VALUES
  ('ENTERPRISE_ADMIN','enterprise.connector.read'),
  ('ENTERPRISE_ADMIN','enterprise.connector.manage'),
  ('AI_ADMIN','enterprise.connector.read'),
  ('AI_ADMIN','enterprise.connector.manage')
) AS matrix(role_code, permission_code)
ON CONFLICT (role, permission) DO NOTHING;

INSERT INTO role_permissions(role_id, permission_id)
SELECT role.id, permission.id
FROM (VALUES
  ('ENTERPRISE_ADMIN','enterprise.connector.read'),
  ('ENTERPRISE_ADMIN','enterprise.connector.manage'),
  ('AI_ADMIN','enterprise.connector.read'),
  ('AI_ADMIN','enterprise.connector.manage')
) AS matrix(role_code, permission_code)
JOIN roles role ON role.code=matrix.role_code
JOIN permissions permission ON permission.code=matrix.permission_code
ON CONFLICT (role_id, permission_id) DO NOTHING;

COMMIT;
