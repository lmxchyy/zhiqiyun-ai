-- Zhiqiyun AI Billing Center V1
-- Scope: versioned customer prices, independent provider costs, task reconciliation,
-- billing lifecycle events and an immutable wallet ledger.

BEGIN;

CREATE TABLE IF NOT EXISTS xz_billing_rule_versions (
  id TEXT PRIMARY KEY,
  rule_key TEXT NOT NULL,
  legacy_rule_id TEXT,
  model_name TEXT NOT NULL,
  model_code TEXT NOT NULL,
  module_code TEXT NOT NULL,
  billing_unit TEXT NOT NULL,
  base_price_points NUMERIC(18, 6) NOT NULL,
  minimum_charge_points NUMERIC(18, 6) NOT NULL DEFAULT 0,
  parameter_rules JSONB NOT NULL DEFAULT '{}'::jsonb,
  rule_source TEXT NOT NULL DEFAULT 'DATABASE',
  tenant_id TEXT,
  plan_id TEXT,
  version INT NOT NULL,
  status TEXT NOT NULL DEFAULT 'DRAFT',
  effective_from TIMESTAMPTZ,
  effective_to TIMESTAMPTZ,
  validation_result JSONB NOT NULL DEFAULT '{"valid":false,"issues":[]}'::jsonb,
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  UNIQUE(rule_key, version),
  CHECK (billing_unit IN ('PER_REQUEST', 'PER_IMAGE', 'PER_SECOND', 'PER_PAGE', 'PER_TOKEN', 'PER_1K_TOKENS')),
  CHECK (rule_source IN ('DATABASE', 'CODE_DEFAULT', 'PLAN_OVERRIDE', 'TENANT_OVERRIDE')),
  CHECK (status IN ('DRAFT', 'PUBLISHED', 'ARCHIVED')),
  CHECK (base_price_points >= 0),
  CHECK (minimum_charge_points >= 0),
  CHECK (effective_to IS NULL OR effective_from IS NULL OR effective_to > effective_from)
);

CREATE INDEX IF NOT EXISTS idx_xz_billing_rule_versions_lookup
  ON xz_billing_rule_versions(module_code, model_code, status, effective_from DESC, version DESC);
CREATE INDEX IF NOT EXISTS idx_xz_billing_rule_versions_scope
  ON xz_billing_rule_versions(rule_source, tenant_id, plan_id, status);

CREATE TABLE IF NOT EXISTS xz_provider_costs (
  id TEXT PRIMARY KEY,
  provider TEXT NOT NULL,
  channel TEXT NOT NULL,
  platform_model_code TEXT NOT NULL,
  upstream_model_name TEXT NOT NULL,
  billing_unit TEXT NOT NULL,
  parameter_range JSONB NOT NULL DEFAULT '{}'::jsonb,
  unit_cost NUMERIC(18, 6) NOT NULL,
  currency TEXT NOT NULL DEFAULT 'CNY',
  effective_from TIMESTAMPTZ NOT NULL DEFAULT now(),
  effective_to TIMESTAMPTZ,
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (billing_unit IN ('PER_REQUEST', 'PER_IMAGE', 'PER_SECOND', 'PER_PAGE', 'PER_TOKEN', 'PER_1K_TOKENS')),
  CHECK (unit_cost >= 0),
  CHECK (status IN ('ACTIVE', 'INACTIVE')),
  CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE INDEX IF NOT EXISTS idx_xz_provider_costs_lookup
  ON xz_provider_costs(platform_model_code, channel, status, effective_from DESC);

CREATE TABLE IF NOT EXISTS xz_billing_lifecycle_events (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  user_id TEXT,
  tenant_id TEXT,
  model_code TEXT,
  event_type TEXT NOT NULL,
  billing_status TEXT NOT NULL,
  points NUMERIC(18, 6) NOT NULL DEFAULT 0,
  rule_version_id TEXT,
  provider_channel TEXT,
  idempotency_key TEXT NOT NULL UNIQUE,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (event_type IN ('QUOTE', 'RESERVE', 'CAPTURE', 'RELEASE', 'REFUND', 'BILLING_FAILED')),
  CHECK (billing_status IN ('UNQUOTED', 'QUOTED', 'RESERVED', 'CAPTURED', 'RELEASED', 'REFUNDED', 'BILLING_FAILED')),
  CHECK (points >= 0)
);

CREATE INDEX IF NOT EXISTS idx_xz_billing_lifecycle_events_task
  ON xz_billing_lifecycle_events(task_id, created_at);

CREATE TABLE IF NOT EXISTS xz_wallet_ledger (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  user_id TEXT,
  tenant_id TEXT,
  task_id TEXT,
  billing_event_id TEXT,
  entry_type TEXT NOT NULL,
  points NUMERIC(18, 6) NOT NULL,
  available_before NUMERIC(18, 6) NOT NULL,
  available_after NUMERIC(18, 6) NOT NULL,
  frozen_before NUMERIC(18, 6) NOT NULL,
  frozen_after NUMERIC(18, 6) NOT NULL,
  idempotency_key TEXT NOT NULL UNIQUE,
  reference_type TEXT NOT NULL DEFAULT '',
  reference_id TEXT NOT NULL DEFAULT '',
  remark TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (entry_type IN ('RECHARGE', 'GRANT', 'RESERVE', 'CAPTURE', 'RELEASE', 'REFUND', 'ADJUSTMENT', 'EXPIRE')),
  CHECK (points >= 0),
  CHECK (available_before >= 0 AND available_after >= 0),
  CHECK (frozen_before >= 0 AND frozen_after >= 0),
  CONSTRAINT xz_wallet_ledger_transition_check CHECK (
    (entry_type = 'RESERVE' AND available_after = available_before - points AND frozen_after = frozen_before + points) OR
    (entry_type = 'CAPTURE' AND available_after = available_before AND frozen_after = frozen_before - points) OR
    (entry_type = 'RELEASE' AND available_after = available_before + points AND frozen_after = frozen_before - points) OR
    (entry_type = 'REFUND' AND available_after = available_before + points AND frozen_after = frozen_before) OR
    (entry_type IN ('RECHARGE', 'GRANT', 'ADJUSTMENT') AND available_after = available_before + points AND frozen_after = frozen_before) OR
    (entry_type = 'EXPIRE' AND available_after = available_before - points AND frozen_after = frozen_before)
  )
);

CREATE INDEX IF NOT EXISTS idx_xz_wallet_ledger_account
  ON xz_wallet_ledger(account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_wallet_ledger_task
  ON xz_wallet_ledger(task_id, created_at);

-- ADJUSTMENT is an audited manual correction and may increase or decrease the
-- available balance. All other entry types keep a single, explicit direction.
ALTER TABLE xz_wallet_ledger
  DROP CONSTRAINT IF EXISTS xz_wallet_ledger_check2,
  DROP CONSTRAINT IF EXISTS xz_wallet_ledger_transition_check;
ALTER TABLE xz_wallet_ledger
  ADD CONSTRAINT xz_wallet_ledger_transition_check CHECK (
    (entry_type = 'RESERVE' AND available_after = available_before - points AND frozen_after = frozen_before + points) OR
    (entry_type = 'CAPTURE' AND available_after = available_before AND frozen_after = frozen_before - points) OR
    (entry_type = 'RELEASE' AND available_after = available_before + points AND frozen_after = frozen_before - points) OR
    (entry_type = 'REFUND' AND available_after = available_before + points AND frozen_after = frozen_before) OR
    (entry_type IN ('RECHARGE', 'GRANT') AND available_after = available_before + points AND frozen_after = frozen_before) OR
    (entry_type = 'ADJUSTMENT' AND available_after IN (available_before - points, available_before + points) AND frozen_after = frozen_before) OR
    (entry_type = 'EXPIRE' AND available_after = available_before - points AND frozen_after = frozen_before)
  );

ALTER TABLE xz_generation_tasks
  ADD COLUMN IF NOT EXISTS client_request_id TEXT,
  ADD COLUMN IF NOT EXISTS task_status TEXT NOT NULL DEFAULT 'CREATED',
  ADD COLUMN IF NOT EXISTS billing_status TEXT NOT NULL DEFAULT 'UNQUOTED',
  ADD COLUMN IF NOT EXISTS billing_rule_version_id TEXT,
  ADD COLUMN IF NOT EXISTS quoted_points NUMERIC(18, 6) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS reserved_points NUMERIC(18, 6) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS captured_points NUMERIC(18, 6) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS released_points NUMERIC(18, 6) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS refunded_points NUMERIC(18, 6) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS supplier_cost NUMERIC(18, 6),
  ADD COLUMN IF NOT EXISTS estimated_margin NUMERIC(18, 6),
  ADD COLUMN IF NOT EXISTS provider_channel TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_generation_tasks_client_request
  ON xz_generation_tasks(user_id, client_request_id)
  WHERE client_request_id IS NOT NULL AND client_request_id <> '';
CREATE INDEX IF NOT EXISTS idx_xz_generation_tasks_reconciliation
  ON xz_generation_tasks(task_status, billing_status, created_at DESC);

-- Existing runtime rows remain queryable and receive canonical, independent statuses.
UPDATE xz_generation_tasks
SET task_status = CASE upper(coalesce(status, ''))
    WHEN 'PENDING' THEN 'QUEUED'
    WHEN 'PROCESSING' THEN 'RUNNING'
    WHEN 'COMPLETED' THEN 'SUCCEEDED'
    WHEN 'SUCCEEDED' THEN 'SUCCEEDED'
    WHEN 'FAILED' THEN 'FAILED'
    WHEN 'CANCELLED' THEN 'CANCELLED'
    ELSE 'CREATED'
  END,
  billing_status = CASE upper(coalesce(status, ''))
    WHEN 'COMPLETED' THEN 'CAPTURED'
    WHEN 'SUCCEEDED' THEN 'CAPTURED'
    WHEN 'FAILED' THEN CASE WHEN lower(coalesce(params->>'billingRefunded','false')) = 'true' THEN 'RELEASED' ELSE 'BILLING_FAILED' END
    WHEN 'CANCELLED' THEN CASE WHEN lower(coalesce(params->>'billingRefunded','false')) = 'true' THEN 'RELEASED' ELSE 'BILLING_FAILED' END
    WHEN 'PENDING' THEN CASE WHEN lower(coalesce(params->>'billingReserved','false')) = 'true' THEN 'RESERVED' ELSE 'UNQUOTED' END
    ELSE billing_status
  END,
  quoted_points = CASE WHEN point_cost > 0 THEN point_cost ELSE quoted_points END,
  reserved_points = CASE WHEN lower(coalesce(params->>'billingReserved','false')) = 'true' THEN point_cost ELSE reserved_points END,
  captured_points = CASE WHEN upper(coalesce(status, '')) IN ('COMPLETED', 'SUCCEEDED') THEN point_cost ELSE captured_points END,
  released_points = CASE WHEN lower(coalesce(params->>'billingRefunded','false')) = 'true' THEN point_cost ELSE released_points END;

-- Seed the currently deployed code defaults as immutable published version 1 records.
INSERT INTO xz_billing_rule_versions(
  id, rule_key, legacy_rule_id, model_name, model_code, module_code, billing_unit,
  base_price_points, minimum_charge_points, parameter_rules, rule_source, version,
  status, effective_from, validation_result, published_at
)
SELECT
  'brv_' || regexp_replace(lower(br.id), '[^a-z0-9]+', '_', 'g') || '_v1',
  br.id,
  br.id,
  br.model_name,
  br.model_name,
  br.module_code,
  CASE lower(br.billing_type)
    WHEN 'per_image' THEN 'PER_IMAGE'
    WHEN 'per_second' THEN 'PER_SECOND'
    WHEN 'per_page' THEN 'PER_PAGE'
    WHEN 'per_token' THEN 'PER_TOKEN'
    WHEN 'per_1k_tokens' THEN 'PER_1K_TOKENS'
    ELSE 'PER_REQUEST'
  END,
  br.base_price,
  1,
  coalesce(br.parameter_multiplier, '{}'::jsonb),
  'CODE_DEFAULT',
  1,
  'PUBLISHED',
  coalesce(br.created_at, now()),
  '{"valid":true,"issues":[]}'::jsonb,
  coalesce(br.updated_at, now())
FROM billing_rules br
WHERE upper(coalesce(br.status, 'ACTIVE')) = 'ACTIVE'
ON CONFLICT (rule_key, version) DO NOTHING;

INSERT INTO xz_billing_rule_versions(
  id, rule_key, legacy_rule_id, model_name, model_code, module_code, billing_unit,
  base_price_points, minimum_charge_points, parameter_rules, rule_source, version,
  status, effective_from, validation_result, published_at
)
VALUES
  ('brv_billing_rule_image_mock_v1','billing_rule_image_mock','billing_rule_image_mock','Mock Standard','mock-standard','image_generation','PER_IMAGE',1,1,'{"quality":{"standard":1,"high":1.5}}','CODE_DEFAULT',1,'PUBLISHED',now(),'{"valid":true,"issues":[]}',now()),
  ('brv_billing_rule_image_gpt_v1','billing_rule_image_gpt','billing_rule_image_gpt','GPT Image 2','gpt-image-2','image_generation','PER_IMAGE',10,1,'{"quality":{"standard":1,"high":1.5},"size":{"1024x1024":1,"1024x1536":1.2,"1536x1024":1.2}}','CODE_DEFAULT',1,'PUBLISHED',now(),'{"valid":true,"issues":[]}',now()),
  ('brv_billing_rule_video_mock_v1','billing_rule_video_mock','billing_rule_video_mock','Mock Video','mock-video','video_generation','PER_SECOND',1,1,'{"resolution":{"480p":1,"720p":1.2,"1080p":2}}','CODE_DEFAULT',1,'PUBLISHED',now(),'{"valid":true,"issues":[]}',now()),
  ('brv_billing_rule_video_grok_image_v1','billing_rule_video_grok_image','billing_rule_video_grok_image','Grok Video Image','grok-video-image','video_generation','PER_SECOND',1,1,'{"resolution":{"480p":1,"720p":1.2,"1080p":2}}','CODE_DEFAULT',1,'PUBLISHED',now(),'{"valid":true,"issues":[]}',now()),
  ('brv_billing_rule_video_seedance_v1','billing_rule_video_seedance','billing_rule_video_seedance','Seedance Fast 2.0','seedance-fast-2.0','video_generation','PER_SECOND',12,1,'{"resolution":{"480p":1,"720p":1.5,"1080p":2}}','CODE_DEFAULT',1,'PUBLISHED',now(),'{"valid":true,"issues":[]}',now()),
  ('brv_billing_rule_video_doubao_seedance_v1','billing_rule_video_doubao_seedance','billing_rule_video_doubao_seedance','Doubao Seedance 2.0','doubao-seedance-2.0','video_generation','PER_SECOND',12,1,'{"resolution":{"480p":1,"720p":1.5,"1080p":2,"4k":4}}','CODE_DEFAULT',1,'PUBLISHED',now(),'{"valid":true,"issues":[]}',now()),
  ('brv_billing_rule_ppt_kimi_v1','billing_rule_ppt_kimi','billing_rule_ppt_kimi','Kimi K2.6','kimi-k2.6','ppt_generation','PER_PAGE',1,1,'{"with_images":{"true":1,"false":1},"uploaded_file":{"true":1,"false":1}}','CODE_DEFAULT',1,'PUBLISHED',now(),'{"valid":true,"issues":[]}',now())
ON CONFLICT (rule_key, version) DO NOTHING;

-- Provider prices are deliberately separate from customer billing rule versions.
INSERT INTO xz_provider_costs(
  id, provider, channel, platform_model_code, upstream_model_name, billing_unit,
  parameter_range, unit_cost, currency, effective_from, status
)
VALUES
  ('pcost_openai_gpt_image_2', 'OPENAI', 'channel_openai', 'gpt-image-2', 'gpt-image-2', 'PER_IMAGE', '{"quality":["standard","high"]}', 0.60, 'CNY', now(), 'ACTIVE'),
  ('pcost_seedance_fast_720', 'CME_CLOUD', 'channel_cmecloud_seedance', 'seedance-fast-2.0', 'seedance-fast-2.0', 'PER_SECOND', '{"resolution":["720p"]}', 0.80, 'CNY', now(), 'ACTIVE'),
  ('pcost_doubao_seedance_720', 'CME_CLOUD', 'channel_cmecloud_seedance', 'doubao-seedance-2.0', 'doubao-seedance-2.0', 'PER_SECOND', '{"resolution":["720p"]}', 0.80, 'CNY', now(), 'ACTIVE')
ON CONFLICT (id) DO NOTHING;

COMMIT;
