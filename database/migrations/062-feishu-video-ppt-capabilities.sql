-- Feishu connector unified capability tasks, delivery state and isolated chat context.
-- Additive: existing image tasks and bindings remain valid.

BEGIN;

ALTER TABLE connector_ai_tasks
  ADD COLUMN IF NOT EXISTS unified_status TEXT NOT NULL DEFAULT 'created',
  ADD COLUMN IF NOT EXISTS internal_stage TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS delivery_status TEXT NOT NULL DEFAULT 'pending',
  ADD COLUMN IF NOT EXISTS delivery_attempts INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS estimated_points BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS delivered_at TIMESTAMPTZ;

ALTER TABLE connector_ai_tasks DROP CONSTRAINT IF EXISTS connector_ai_tasks_status_check;
ALTER TABLE connector_ai_tasks ADD CONSTRAINT connector_ai_tasks_status_check
  CHECK (status IN ('pending','processing','succeeded','completed','delivery_failed','failed','ignored','cancelled','refunded'));

ALTER TABLE connector_ai_tasks DROP CONSTRAINT IF EXISTS connector_ai_tasks_unified_status_check;
ALTER TABLE connector_ai_tasks ADD CONSTRAINT connector_ai_tasks_unified_status_check
  CHECK (unified_status IN ('created','queued','validating','reserved','processing','rendering','uploading','completed','delivery_failed','failed','cancelled','refunded'));

ALTER TABLE connector_ai_tasks DROP CONSTRAINT IF EXISTS connector_ai_tasks_delivery_status_check;
ALTER TABLE connector_ai_tasks ADD CONSTRAINT connector_ai_tasks_delivery_status_check
  CHECK (delivery_status IN ('pending','sending','delivered','failed','not_required'));

CREATE INDEX IF NOT EXISTS idx_connector_ai_tasks_capability_status
  ON connector_ai_tasks(enterprise_id, connector_id, task_type, unified_status, created_at DESC);

CREATE TABLE IF NOT EXISTS connector_session_contexts (
  enterprise_id TEXT NOT NULL REFERENCES xz_tenants(id),
  connector_id TEXT NOT NULL REFERENCES enterprise_connectors(id) ON DELETE CASCADE,
  external_chat_id TEXT NOT NULL,
  external_user_id TEXT NOT NULL,
  last_intent TEXT NOT NULL DEFAULT '',
  last_task_type TEXT NOT NULL DEFAULT '',
  last_task_id TEXT NOT NULL DEFAULT '',
  last_asset_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  last_topic TEXT NOT NULL DEFAULT '',
  last_parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
  last_prompt TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(connector_id, external_chat_id, external_user_id)
);

CREATE INDEX IF NOT EXISTS idx_connector_session_context_expiry
  ON connector_session_contexts(expires_at);

CREATE TABLE IF NOT EXISTS xz_ppt_tasks (
  task_id VARCHAR(128) PRIMARY KEY,
  user_id VARCHAR(128) NOT NULL,
  client_request_id VARCHAR(256) NOT NULL DEFAULT '',
  status VARCHAR(32) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  raw JSONB NOT NULL
);
ALTER TABLE xz_ppt_tasks ADD COLUMN IF NOT EXISTS client_request_id VARCHAR(256) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_xz_ppt_tasks_user_created ON xz_ppt_tasks(user_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_ppt_tasks_user_status ON xz_ppt_tasks(user_id,status);
DO $$
BEGIN
  IF to_regclass('public.xz_ppt_tasks') IS NOT NULL THEN
    CREATE UNIQUE INDEX IF NOT EXISTS uk_xz_ppt_tasks_user_client_request
      ON xz_ppt_tasks(user_id,client_request_id) WHERE client_request_id<>'';
  END IF;
END $$;

UPDATE connector_user_bindings
SET permission_json = permission_json || '{"videoGenerate":true,"pptGenerate":true}'::jsonb
WHERE platform='feishu';

COMMIT;
