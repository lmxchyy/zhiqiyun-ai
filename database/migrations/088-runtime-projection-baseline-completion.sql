-- Complete the SQL-managed runtime projection baseline.
-- These tables already exist in runtimeProjectionSchema, but were omitted from
-- 021-runtime-projections.sql. Keep the forward migration idempotent so
-- databases previously initialized by application startup remain compatible.
BEGIN;

CREATE TABLE IF NOT EXISTS xz_auth_account_merge_requests (
  id TEXT PRIMARY KEY,
  primary_user_id TEXT NOT NULL,
  secondary_user_id TEXT NOT NULL,
  mobile TEXT,
  wechat_open_id TEXT,
  wechat_union_id TEXT,
  conflict_code TEXT NOT NULL,
  source TEXT NOT NULL,
  status TEXT NOT NULL,
  reason TEXT,
  review_comment TEXT,
  resolved_by TEXT,
  resolved_at TEXT,
  created_at TEXT,
  updated_at TEXT,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_xz_auth_merge_primary
  ON xz_auth_account_merge_requests(primary_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_auth_merge_secondary
  ON xz_auth_account_merge_requests(secondary_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_auth_merge_status
  ON xz_auth_account_merge_requests(status, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_billing_events (
  id TEXT PRIMARY KEY,
  transaction_id TEXT,
  user_id TEXT,
  agent_id TEXT,
  tenant_id TEXT,
  operation_center_id TEXT,
  module_code TEXT,
  task_id TEXT,
  metric_code TEXT,
  quantity BIGINT NOT NULL DEFAULT 0,
  unit_amount_cents BIGINT NOT NULL DEFAULT 0,
  amount_cents BIGINT NOT NULL DEFAULT 0,
  point_cost BIGINT NOT NULL DEFAULT 0,
  balance_before BIGINT NOT NULL DEFAULT 0,
  balance_after BIGINT NOT NULL DEFAULT 0,
  model TEXT,
  status TEXT,
  occurred_at TEXT,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_xz_billing_events_module_code
  ON xz_billing_events(module_code);
CREATE INDEX IF NOT EXISTS idx_xz_billing_events_user_occurred
  ON xz_billing_events(user_id, occurred_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS xz_ai_state (
  user_id TEXT PRIMARY KEY,
  favorite_task_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  hidden_task_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  favorite_collections JSONB NOT NULL DEFAULT '[]'::jsonb,
  agent_conversations JSONB NOT NULL DEFAULT '[]'::jsonb,
  active_conversation_id TEXT,
  active_collection_id TEXT,
  updated_at TEXT,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS xz_system_settings (
  id TEXT PRIMARY KEY,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_at TEXT
);

CREATE TABLE IF NOT EXISTS xz_api_channels (
  id TEXT PRIMARY KEY,
  name TEXT,
  base_url TEXT,
  protocol TEXT,
  status TEXT,
  priority INT NOT NULL DEFAULT 0,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_xz_api_channels_status
  ON xz_api_channels(status);

CREATE TABLE IF NOT EXISTS xz_api_keys (
  id TEXT PRIMARY KEY,
  customer TEXT,
  prefix TEXT,
  status TEXT,
  quota_limit BIGINT NOT NULL DEFAULT 0,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS xz_user_model_routes (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  provider TEXT,
  channel_id TEXT,
  channel TEXT,
  api_key_id TEXT,
  key_prefix TEXT,
  group_name TEXT,
  models JSONB NOT NULL DEFAULT '[]'::jsonb,
  quota_limit BIGINT NOT NULL DEFAULT 0,
  quota_used BIGINT NOT NULL DEFAULT 0,
  status TEXT,
  updated_at TEXT,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_xz_user_model_routes_user_id
  ON xz_user_model_routes(user_id);
CREATE INDEX IF NOT EXISTS idx_xz_user_model_routes_status
  ON xz_user_model_routes(status);

COMMIT;
