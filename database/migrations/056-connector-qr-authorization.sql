-- Unified enterprise connector QR authorization sessions.
-- This migration is additive and keeps the existing Feishu connector intact.

BEGIN;

ALTER TABLE enterprise_connectors
  DROP CONSTRAINT IF EXISTS enterprise_connectors_connector_type_check;
ALTER TABLE enterprise_connectors
  ADD CONSTRAINT enterprise_connectors_connector_type_check
  CHECK (connector_type IN ('feishu','wecom','dingtalk','wechat'));

ALTER TABLE connector_user_bindings
  DROP CONSTRAINT IF EXISTS connector_user_bindings_platform_check;
ALTER TABLE connector_user_bindings
  ADD CONSTRAINT connector_user_bindings_platform_check
  CHECK (platform IN ('feishu','wecom','dingtalk','wechat'));

ALTER TABLE connector_messages
  DROP CONSTRAINT IF EXISTS connector_messages_platform_check;
ALTER TABLE connector_messages
  ADD CONSTRAINT connector_messages_platform_check
  CHECK (platform IN ('feishu','wecom','dingtalk','wechat'));

ALTER TABLE connector_ai_tasks
  DROP CONSTRAINT IF EXISTS connector_ai_tasks_platform_check;
ALTER TABLE connector_ai_tasks
  ADD CONSTRAINT connector_ai_tasks_platform_check
  CHECK (platform IN ('feishu','wecom','dingtalk','wechat'));

CREATE TABLE IF NOT EXISTS connector_authorization_sessions (
  id TEXT PRIMARY KEY,
  enterprise_id TEXT NOT NULL REFERENCES xz_tenants(id),
  platform TEXT NOT NULL CHECK (platform IN ('universal','feishu','wecom','dingtalk','wechat')),
  state_hash TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL DEFAULT 'PENDING'
    CHECK (status IN ('PENDING','AUTHORIZING','AUTHORIZED','FAILED','EXPIRED','CANCELLED')),
  created_by_user_id TEXT NOT NULL REFERENCES xz_users(id),
  created_by_role TEXT NOT NULL DEFAULT '',
  organization_id TEXT NOT NULL DEFAULT '',
  connector_id TEXT REFERENCES enterprise_connectors(id) ON DELETE SET NULL,
  external_tenant_key TEXT NOT NULL DEFAULT '',
  external_user_id TEXT NOT NULL DEFAULT '',
  external_user_name TEXT NOT NULL DEFAULT '',
  result_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_connector_auth_sessions_enterprise
  ON connector_authorization_sessions(enterprise_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_connector_auth_sessions_pending
  ON connector_authorization_sessions(status, expires_at)
  WHERE status IN ('PENDING','AUTHORIZING');

COMMIT;
