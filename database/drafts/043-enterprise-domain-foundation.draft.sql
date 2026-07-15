-- DRAFT ONLY - DO NOT RUN IN PRODUCTION.
-- 主控 SaaS 企业管理底层规范：配置化商品/套餐/计费、四套状态机、权限域、幂等与审计。
-- 本文件刻意放在 database/drafts，而不是 database/migrations，现有迁移器不会自动执行。
-- 文件末尾保留 ROLLBACK 安全闩；正式拆分、备份、预检和审批完成前不得改为 COMMIT。

BEGIN;

-- ---------------------------------------------------------------------------
-- 0. 复用边界
-- ---------------------------------------------------------------------------
-- 企业唯一根：xz_tenants WHERE tenant_type = 'ENTERPRISE'。
-- 不创建 enterprises / enterprise / tenant 的平行主表。
-- 复用 xz_organizations、xz_users、xz_tenant_members、xz_user_roles、
-- roles、permissions、role_permissions 和 xz_role_permissions。

COMMENT ON TABLE xz_tenants IS
  '租户唯一主表；企业记录由 tenant_type=ENTERPRISE 标识。status 只表达档案生命周期，不再兼任套餐、服务或风控状态。';

-- ---------------------------------------------------------------------------
-- 1. 商品、套餐、权益与赠送规则
-- ---------------------------------------------------------------------------

ALTER TABLE products
  ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS sale_scope TEXT NOT NULL DEFAULT 'ALL',
  ADD COLUMN IF NOT EXISTS currency_code TEXT NOT NULL DEFAULT 'CNY',
  ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS retired_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS updated_by TEXT REFERENCES xz_users(id),
  ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE product_entitlements
  ADD COLUMN IF NOT EXISTS value_type TEXT NOT NULL DEFAULT 'QUANTITY',
  ADD COLUMN IF NOT EXISTS aggregation TEXT NOT NULL DEFAULT 'NONE',
  ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'ACTIVE',
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE xz_plans
  ADD COLUMN IF NOT EXISTS catalog_version BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS currency_code TEXT NOT NULL DEFAULT 'CNY',
  ADD COLUMN IF NOT EXISTS billing_cycle TEXT NOT NULL DEFAULT 'ONE_TIME',
  ADD COLUMN IF NOT EXISTS validity_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS retired_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS xz_plan_versions (
  id TEXT PRIMARY KEY,
  plan_id TEXT NOT NULL REFERENCES xz_plans(id),
  version BIGINT NOT NULL CHECK (version > 0),
  status TEXT NOT NULL DEFAULT 'DRAFT'
    CHECK (status IN ('DRAFT', 'PUBLISHED', 'RETIRED')),
  name TEXT NOT NULL,
  currency_code TEXT NOT NULL DEFAULT 'CNY',
  price_cents BIGINT NOT NULL DEFAULT 0 CHECK (price_cents >= 0),
  billing_cycle TEXT NOT NULL DEFAULT 'ONE_TIME'
    CHECK (billing_cycle IN ('ONE_TIME', 'MONTHLY', 'QUARTERLY', 'YEARLY', 'CUSTOM')),
  validity_mode TEXT NOT NULL DEFAULT 'FIXED_DAYS'
    CHECK (validity_mode IN ('FIXED_DAYS', 'FIXED_WINDOW', 'SUBSCRIPTION_PERIOD', 'UNLIMITED', 'CUSTOM')),
  validity_value BIGINT CHECK (validity_value IS NULL OR validity_value > 0),
  valid_from TIMESTAMPTZ,
  valid_to TIMESTAMPTZ,
  product_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb,
  entitlement_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  grant_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  change_reason TEXT NOT NULL DEFAULT '',
  created_by TEXT REFERENCES xz_users(id),
  published_by TEXT REFERENCES xz_users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  retired_at TIMESTAMPTZ,
  UNIQUE (plan_id, version),
  CHECK (valid_to IS NULL OR valid_from IS NULL OR valid_to > valid_from),
  CHECK (status <> 'PUBLISHED' OR (published_at IS NOT NULL AND change_reason <> ''))
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_plan_versions_current_published
  ON xz_plan_versions(plan_id)
  WHERE status = 'PUBLISHED' AND retired_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_xz_plan_versions_lookup
  ON xz_plan_versions(plan_id, status, version DESC);

CREATE TABLE IF NOT EXISTS xz_plan_product_bindings (
  id TEXT PRIMARY KEY,
  plan_version_id TEXT NOT NULL REFERENCES xz_plan_versions(id) ON DELETE CASCADE,
  product_id UUID NOT NULL REFERENCES products(id),
  product_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  sort_order INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'DISABLED')),
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (plan_version_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_xz_plan_product_bindings_product
  ON xz_plan_product_bindings(product_id, status, plan_version_id);

CREATE TABLE IF NOT EXISTS xz_plan_entitlement_values (
  id TEXT PRIMARY KEY,
  plan_version_id TEXT NOT NULL REFERENCES xz_plan_versions(id) ON DELETE CASCADE,
  entitlement_id UUID NOT NULL REFERENCES product_entitlements(id),
  value_type TEXT NOT NULL
    CHECK (value_type IN ('QUANTITY', 'BOOLEAN', 'TEXT', 'JSON', 'UNLIMITED')),
  quantity BIGINT CHECK (quantity IS NULL OR quantity >= 0),
  value_boolean BOOLEAN,
  value_text TEXT,
  value_json JSONB,
  aggregation TEXT NOT NULL DEFAULT 'NONE'
    CHECK (aggregation IN ('NONE', 'SUM', 'MAX', 'LATEST')),
  reset_policy TEXT NOT NULL DEFAULT 'NEVER'
    CHECK (reset_policy IN ('NEVER', 'DAILY', 'WEEKLY', 'MONTHLY', 'BILLING_PERIOD', 'CUSTOM')),
  threshold_config JSONB NOT NULL DEFAULT '[]'::jsonb,
  validity_config JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'DISABLED')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (plan_version_id, entitlement_id),
  CHECK (
    (value_type = 'QUANTITY' AND quantity IS NOT NULL) OR
    (value_type = 'BOOLEAN' AND value_boolean IS NOT NULL) OR
    (value_type = 'TEXT' AND value_text IS NOT NULL) OR
    (value_type = 'JSON' AND value_json IS NOT NULL) OR
    value_type = 'UNLIMITED'
  )
);

CREATE INDEX IF NOT EXISTS idx_xz_plan_entitlement_values_entitlement
  ON xz_plan_entitlement_values(entitlement_id, status, plan_version_id);

CREATE TABLE IF NOT EXISTS xz_plan_grant_rules (
  id TEXT PRIMARY KEY,
  plan_version_id TEXT NOT NULL REFERENCES xz_plan_versions(id) ON DELETE CASCADE,
  resource_code TEXT NOT NULL CHECK (resource_code = 'COMPUTE_UNIT'),
  base_amount BIGINT NOT NULL DEFAULT 0 CHECK (base_amount >= 0),
  bonus_amount BIGINT NOT NULL DEFAULT 0 CHECK (bonus_amount >= 0),
  trigger_type TEXT NOT NULL
    CHECK (trigger_type IN ('ON_ACTIVATION', 'ON_RENEWAL', 'SCHEDULED', 'MANUAL_APPROVED')),
  recurrence TEXT NOT NULL DEFAULT 'ONCE'
    CHECK (recurrence IN ('ONCE', 'DAILY', 'MONTHLY', 'BILLING_PERIOD', 'CUSTOM')),
  grant_limit BIGINT CHECK (grant_limit IS NULL OR grant_limit > 0),
  validity_seconds BIGINT CHECK (validity_seconds IS NULL OR validity_seconds > 0),
  valid_from TIMESTAMPTZ,
  valid_to TIMESTAMPTZ,
  condition_config JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'DISABLED')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (valid_to IS NULL OR valid_from IS NULL OR valid_to > valid_from),
  CHECK (base_amount + bonus_amount > 0)
);

CREATE INDEX IF NOT EXISTS idx_xz_plan_grant_rules_effective
  ON xz_plan_grant_rules(plan_version_id, resource_code, status, valid_from, valid_to);

-- ---------------------------------------------------------------------------
-- 2. 模型计费策略与原始用量
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS xz_model_billing_policies (
  id TEXT PRIMARY KEY,
  model_code TEXT NOT NULL,
  capability TEXT NOT NULL,
  billing_unit TEXT NOT NULL
    CHECK (billing_unit IN (
      'REQUEST', 'INPUT_TOKEN', 'OUTPUT_TOKEN', 'CACHED_INPUT_TOKEN',
      'REASONING_TOKEN', 'IMAGE', 'SECOND', 'CHARACTER'
    )),
  version BIGINT NOT NULL CHECK (version > 0),
  status TEXT NOT NULL DEFAULT 'DRAFT'
    CHECK (status IN ('DRAFT', 'PUBLISHED', 'RETIRED')),
  unit_size BIGINT NOT NULL CHECK (unit_size > 0),
  compute_unit_numerator BIGINT NOT NULL DEFAULT 0 CHECK (compute_unit_numerator >= 0),
  compute_unit_denominator BIGINT NOT NULL DEFAULT 1 CHECK (compute_unit_denominator > 0),
  price_cents_per_unit BIGINT NOT NULL DEFAULT 0 CHECK (price_cents_per_unit >= 0),
  cost_cents_per_unit BIGINT NOT NULL DEFAULT 0 CHECK (cost_cents_per_unit >= 0),
  rounding_mode TEXT NOT NULL DEFAULT 'CEILING'
    CHECK (rounding_mode IN ('CEILING', 'FLOOR', 'HALF_UP')),
  valid_from TIMESTAMPTZ,
  valid_to TIMESTAMPTZ,
  change_reason TEXT NOT NULL DEFAULT '',
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_by TEXT REFERENCES xz_users(id),
  published_by TEXT REFERENCES xz_users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  retired_at TIMESTAMPTZ,
  UNIQUE (model_code, capability, billing_unit, version),
  CHECK (valid_to IS NULL OR valid_from IS NULL OR valid_to > valid_from),
  CHECK (status <> 'PUBLISHED' OR (published_at IS NOT NULL AND change_reason <> ''))
);

CREATE INDEX IF NOT EXISTS idx_xz_model_billing_policies_effective
  ON xz_model_billing_policies(model_code, capability, billing_unit, status, valid_from, valid_to);

CREATE TABLE IF NOT EXISTS xz_model_billing_tiers (
  id TEXT PRIMARY KEY,
  policy_id TEXT NOT NULL REFERENCES xz_model_billing_policies(id) ON DELETE CASCADE,
  from_quantity BIGINT NOT NULL DEFAULT 0 CHECK (from_quantity >= 0),
  to_quantity BIGINT CHECK (to_quantity IS NULL OR to_quantity > from_quantity),
  compute_unit_numerator BIGINT NOT NULL DEFAULT 0 CHECK (compute_unit_numerator >= 0),
  compute_unit_denominator BIGINT NOT NULL DEFAULT 1 CHECK (compute_unit_denominator > 0),
  price_cents_per_unit BIGINT NOT NULL DEFAULT 0 CHECK (price_cents_per_unit >= 0),
  cost_cents_per_unit BIGINT NOT NULL DEFAULT 0 CHECK (cost_cents_per_unit >= 0),
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (policy_id, from_quantity)
);

CREATE INDEX IF NOT EXISTS idx_xz_model_billing_tiers_range
  ON xz_model_billing_tiers(policy_id, from_quantity, to_quantity);

CREATE TABLE IF NOT EXISTS xz_model_billing_multipliers (
  id TEXT PRIMARY KEY,
  policy_id TEXT NOT NULL REFERENCES xz_model_billing_policies(id) ON DELETE CASCADE,
  dimension TEXT NOT NULL,
  operator TEXT NOT NULL CHECK (operator IN ('EQ', 'IN', 'GTE', 'GT', 'LTE', 'LT', 'BETWEEN')),
  operand JSONB NOT NULL,
  multiplier_numerator BIGINT NOT NULL CHECK (multiplier_numerator > 0),
  multiplier_denominator BIGINT NOT NULL DEFAULT 1 CHECK (multiplier_denominator > 0),
  priority INTEGER NOT NULL DEFAULT 100,
  stacking_mode TEXT NOT NULL DEFAULT 'MULTIPLY'
    CHECK (stacking_mode IN ('MULTIPLY', 'MAX', 'FIRST_MATCH')),
  valid_from TIMESTAMPTZ,
  valid_to TIMESTAMPTZ,
  status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'DISABLED')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (valid_to IS NULL OR valid_from IS NULL OR valid_to > valid_from)
);

CREATE INDEX IF NOT EXISTS idx_xz_model_billing_multipliers_match
  ON xz_model_billing_multipliers(policy_id, status, priority, valid_from, valid_to);

CREATE TABLE IF NOT EXISTS xz_model_usage_records (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  user_id TEXT REFERENCES xz_users(id),
  task_id TEXT,
  provider_code TEXT NOT NULL,
  provider_request_id TEXT,
  model_code TEXT NOT NULL,
  capability TEXT NOT NULL,
  input_tokens BIGINT NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
  output_tokens BIGINT NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
  cached_input_tokens BIGINT NOT NULL DEFAULT 0 CHECK (cached_input_tokens >= 0),
  reasoning_tokens BIGINT NOT NULL DEFAULT 0 CHECK (reasoning_tokens >= 0),
  total_tokens BIGINT NOT NULL DEFAULT 0 CHECK (total_tokens >= 0),
  raw_usage JSONB NOT NULL DEFAULT '{}'::jsonb,
  billing_policy_id TEXT REFERENCES xz_model_billing_policies(id),
  pricing_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  compute_units_charged BIGINT NOT NULL DEFAULT 0 CHECK (compute_units_charged >= 0),
  amount_cents_charged BIGINT NOT NULL DEFAULT 0 CHECK (amount_cents_charged >= 0),
  status TEXT NOT NULL DEFAULT 'RECORDED'
    CHECK (status IN ('RECORDED', 'RATED', 'SETTLED', 'FAILED', 'REVERSED')),
  idempotency_key TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, idempotency_key)
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_model_usage_provider_request
  ON xz_model_usage_records(provider_code, provider_request_id, model_code)
  WHERE provider_request_id IS NOT NULL AND provider_request_id <> '';

CREATE INDEX IF NOT EXISTS idx_xz_model_usage_tenant_time
  ON xz_model_usage_records(tenant_id, occurred_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_xz_model_usage_model_time
  ON xz_model_usage_records(model_code, capability, occurred_at DESC);

ALTER TABLE IF EXISTS xz_billing_events
  ADD COLUMN IF NOT EXISTS usage_record_id TEXT REFERENCES xz_model_usage_records(id),
  ADD COLUMN IF NOT EXISTS billing_policy_id TEXT REFERENCES xz_model_billing_policies(id),
  ADD COLUMN IF NOT EXISTS compute_unit_delta BIGINT NOT NULL DEFAULT 0;

DO $$
BEGIN
  IF to_regclass('public.xz_billing_events') IS NOT NULL THEN
    EXECUTE 'CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_billing_events_usage_record
      ON xz_billing_events(usage_record_id)
      WHERE usage_record_id IS NOT NULL';
  END IF;
END $$;

-- ---------------------------------------------------------------------------
-- 3. 四套状态机投影与转换日志
-- ---------------------------------------------------------------------------

ALTER TABLE xz_tenant_certifications
  ADD COLUMN IF NOT EXISTS state_version BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS last_transition_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE xz_tenant_subscriptions
  ADD COLUMN IF NOT EXISTS plan_version_id TEXT REFERENCES xz_plan_versions(id),
  ADD COLUMN IF NOT EXISTS current_period_start TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS current_period_end TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS grace_ends_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS cancel_at_period_end BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS suspended_from_status TEXT,
  ADD COLUMN IF NOT EXISTS state_version BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS state_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS last_transition_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE TABLE IF NOT EXISTS xz_tenant_service_states (
  tenant_id TEXT PRIMARY KEY REFERENCES xz_tenants(id),
  state TEXT NOT NULL DEFAULT 'PROVISIONING'
    CHECK (state IN ('PROVISIONING', 'ACTIVE', 'PAUSED', 'DISABLED', 'TERMINATED')),
  state_version BIGINT NOT NULL DEFAULT 0 CHECK (state_version >= 0),
  reason TEXT NOT NULL DEFAULT '',
  paused_until TIMESTAMPTZ,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  changed_by TEXT REFERENCES xz_users(id),
  changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_xz_tenant_service_states_state
  ON xz_tenant_service_states(state, changed_at DESC, tenant_id);

CREATE TABLE IF NOT EXISTS xz_tenant_risk_states (
  tenant_id TEXT PRIMARY KEY REFERENCES xz_tenants(id),
  state TEXT NOT NULL DEFAULT 'NORMAL'
    CHECK (state IN ('NORMAL', 'MONITORING', 'RESTRICTED', 'BLOCKED')),
  risk_level TEXT NOT NULL DEFAULT 'LOW'
    CHECK (risk_level IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
  state_version BIGINT NOT NULL DEFAULT 0 CHECK (state_version >= 0),
  reason TEXT NOT NULL DEFAULT '',
  restriction_config JSONB NOT NULL DEFAULT '{}'::jsonb,
  evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  changed_by TEXT REFERENCES xz_users(id),
  changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_xz_tenant_risk_states_state
  ON xz_tenant_risk_states(state, risk_level, changed_at DESC, tenant_id);

CREATE TABLE IF NOT EXISTS xz_tenant_state_transitions (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  aggregate_type TEXT NOT NULL
    CHECK (aggregate_type IN ('CERTIFICATION', 'SUBSCRIPTION', 'SERVICE', 'RISK')),
  aggregate_id TEXT NOT NULL,
  command TEXT NOT NULL,
  from_state TEXT NOT NULL,
  to_state TEXT NOT NULL,
  from_version BIGINT NOT NULL CHECK (from_version >= 0),
  to_version BIGINT NOT NULL CHECK (to_version = from_version + 1),
  reason TEXT NOT NULL,
  actor_id TEXT REFERENCES xz_users(id),
  actor_role TEXT,
  permission_domain TEXT NOT NULL CHECK (permission_domain IN ('PLATFORM', 'TENANT', 'SYSTEM')),
  permission_code TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  approval_request_id TEXT,
  before_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  after_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  result_status TEXT NOT NULL DEFAULT 'SUCCEEDED'
    CHECK (result_status IN ('SUCCEEDED', 'FAILED', 'PENDING_APPROVAL')),
  error_code TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, aggregate_type, aggregate_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_xz_tenant_state_transitions_scope
  ON xz_tenant_state_transitions(tenant_id, aggregate_type, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_xz_tenant_state_transitions_actor
  ON xz_tenant_state_transitions(actor_id, created_at DESC)
  WHERE actor_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- 4. 统一幂等记录
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS xz_idempotency_records (
  id TEXT PRIMARY KEY,
  permission_domain TEXT NOT NULL CHECK (permission_domain IN ('PLATFORM', 'TENANT', 'SYSTEM')),
  actor_id TEXT NOT NULL,
  tenant_id TEXT REFERENCES xz_tenants(id),
  method TEXT NOT NULL,
  canonical_path TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  request_id TEXT,
  status TEXT NOT NULL DEFAULT 'PROCESSING'
    CHECK (status IN ('PROCESSING', 'SUCCEEDED', 'FAILED')),
  resource_type TEXT,
  resource_id TEXT,
  response_status INTEGER,
  response_headers JSONB NOT NULL DEFAULT '{}'::jsonb,
  response_body JSONB,
  error_code TEXT,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ NOT NULL,
  UNIQUE (permission_domain, actor_id, method, canonical_path, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_xz_idempotency_records_expiry
  ON xz_idempotency_records(expires_at, status);

CREATE INDEX IF NOT EXISTS idx_xz_idempotency_records_tenant
  ON xz_idempotency_records(tenant_id, started_at DESC)
  WHERE tenant_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- 5. 权限域：复用现有 roles / permissions，不创建平行角色权限主表
-- ---------------------------------------------------------------------------

ALTER TABLE permissions
  ADD COLUMN IF NOT EXISTS permission_domain TEXT NOT NULL DEFAULT 'PLATFORM',
  ADD COLUMN IF NOT EXISTS route_prefix TEXT,
  ADD COLUMN IF NOT EXISTS sensitivity TEXT NOT NULL DEFAULT 'NORMAL';

UPDATE permissions
SET permission_domain = CASE
      WHEN code LIKE 'enterprise.%' THEN 'TENANT'
      ELSE 'PLATFORM'
    END,
    route_prefix = CASE
      WHEN code LIKE 'enterprise.%' THEN '/api/v1/enterprise'
      ELSE '/api/v1/admin'
    END
WHERE route_prefix IS NULL
   OR permission_domain NOT IN ('PLATFORM', 'TENANT', 'SYSTEM');

ALTER TABLE xz_role_permissions
  ADD COLUMN IF NOT EXISTS permission_domain TEXT NOT NULL DEFAULT 'PLATFORM';

UPDATE xz_role_permissions
SET permission_domain = CASE
  WHEN permission LIKE 'enterprise.%' THEN 'TENANT'
  ELSE 'PLATFORM'
END;

CREATE INDEX IF NOT EXISTS idx_permissions_domain_module
  ON permissions(permission_domain, module, action, code);

CREATE INDEX IF NOT EXISTS idx_xz_role_permissions_domain
  ON xz_role_permissions(permission_domain, role, permission);

INSERT INTO permissions(code, name, module, action, permission_domain, route_prefix, sensitivity)
VALUES
  ('enterprise:certification:revoke', '撤销企业认证', 'enterprise', 'certification_revoke', 'PLATFORM', '/api/v1/admin', 'HIGH'),
  ('enterprise:subscription:transition', '转换企业套餐状态', 'enterprise', 'subscription_transition', 'PLATFORM', '/api/v1/admin', 'HIGH'),
  ('enterprise:service:view', '查看企业服务状态', 'enterprise', 'service_view', 'PLATFORM', '/api/v1/admin', 'NORMAL'),
  ('enterprise:service:transition', '转换企业服务状态', 'enterprise', 'service_transition', 'PLATFORM', '/api/v1/admin', 'CRITICAL'),
  ('enterprise:risk:transition', '转换企业风控状态', 'enterprise', 'risk_transition', 'PLATFORM', '/api/v1/admin', 'CRITICAL'),
  ('billing:product:view', '查看商品目录', 'billing', 'product_view', 'PLATFORM', '/api/v1/admin', 'NORMAL'),
  ('billing:product:manage', '管理商品目录', 'billing', 'product_manage', 'PLATFORM', '/api/v1/admin', 'HIGH'),
  ('billing:plan:view', '查看套餐版本', 'billing', 'plan_view', 'PLATFORM', '/api/v1/admin', 'NORMAL'),
  ('billing:plan:manage', '管理套餐草稿', 'billing', 'plan_manage', 'PLATFORM', '/api/v1/admin', 'HIGH'),
  ('billing:plan:publish', '发布套餐版本', 'billing', 'plan_publish', 'PLATFORM', '/api/v1/admin', 'CRITICAL'),
  ('billing:model-policy:view', '查看模型计费策略', 'billing', 'model_policy_view', 'PLATFORM', '/api/v1/admin', 'NORMAL'),
  ('billing:model-policy:manage', '管理模型计费策略草稿', 'billing', 'model_policy_manage', 'PLATFORM', '/api/v1/admin', 'HIGH'),
  ('billing:model-policy:publish', '发布模型计费策略', 'billing', 'model_policy_publish', 'PLATFORM', '/api/v1/admin', 'CRITICAL'),
  ('enterprise.package.read', '查看企业套餐', 'enterprise', 'package_read', 'TENANT', '/api/v1/enterprise', 'NORMAL'),
  ('enterprise.compute.read', '查看企业算力', 'enterprise', 'compute_read', 'TENANT', '/api/v1/enterprise', 'NORMAL'),
  ('enterprise.usage.read', '查看企业用量', 'enterprise', 'usage_read', 'TENANT', '/api/v1/enterprise', 'NORMAL'),
  ('enterprise.service.read', '查看企业服务状态', 'enterprise', 'service_read', 'TENANT', '/api/v1/enterprise', 'NORMAL'),
  ('enterprise.certification.read', '查看企业认证状态', 'enterprise', 'certification_read', 'TENANT', '/api/v1/enterprise', 'SENSITIVE')
ON CONFLICT (code) DO UPDATE SET
  name = EXCLUDED.name,
  module = EXCLUDED.module,
  action = EXCLUDED.action,
  permission_domain = EXCLUDED.permission_domain,
  route_prefix = EXCLUDED.route_prefix,
  sensitivity = EXCLUDED.sensitivity;

-- 平台新权限矩阵。原 041 migration 中已有权限保持不变。
WITH matrix(role_code, permission_code) AS (
  VALUES
    ('SUPER_ADMIN', 'enterprise:certification:revoke'),
    ('SUPER_ADMIN', 'enterprise:subscription:transition'),
    ('SUPER_ADMIN', 'enterprise:service:view'),
    ('SUPER_ADMIN', 'enterprise:service:transition'),
    ('SUPER_ADMIN', 'enterprise:risk:transition'),
    ('SUPER_ADMIN', 'billing:product:view'),
    ('SUPER_ADMIN', 'billing:product:manage'),
    ('SUPER_ADMIN', 'billing:plan:view'),
    ('SUPER_ADMIN', 'billing:plan:manage'),
    ('SUPER_ADMIN', 'billing:plan:publish'),
    ('SUPER_ADMIN', 'billing:model-policy:view'),
    ('SUPER_ADMIN', 'billing:model-policy:manage'),
    ('SUPER_ADMIN', 'billing:model-policy:publish'),
    ('ENTERPRISE_OPERATOR', 'enterprise:subscription:transition'),
    ('ENTERPRISE_OPERATOR', 'enterprise:service:view'),
    ('ENTERPRISE_OPERATOR', 'enterprise:service:transition'),
    ('ENTERPRISE_OPERATOR', 'billing:product:view'),
    ('ENTERPRISE_OPERATOR', 'billing:product:manage'),
    ('ENTERPRISE_OPERATOR', 'billing:plan:view'),
    ('ENTERPRISE_OPERATOR', 'billing:plan:manage'),
    ('ENTERPRISE_OPERATOR', 'billing:model-policy:view'),
    ('ENTERPRISE_OPERATOR', 'billing:model-policy:manage'),
    ('CERTIFICATION_REVIEWER', 'enterprise:certification:revoke'),
    ('FINANCE', 'enterprise:service:view'),
    ('FINANCE', 'billing:product:view'),
    ('FINANCE', 'billing:plan:view'),
    ('FINANCE', 'billing:plan:publish'),
    ('FINANCE', 'billing:model-policy:view'),
    ('FINANCE', 'billing:model-policy:publish'),
    ('RISK_MANAGER', 'enterprise:service:view'),
    ('RISK_MANAGER', 'enterprise:service:transition'),
    ('RISK_MANAGER', 'enterprise:risk:transition'),
    ('CUSTOMER_SERVICE', 'enterprise:service:view'),
    ('CUSTOMER_SERVICE', 'billing:product:view'),
    ('CUSTOMER_SERVICE', 'billing:plan:view'),
    ('CUSTOMER_SERVICE', 'billing:model-policy:view')
)
INSERT INTO role_permissions(role_id, permission_id)
SELECT role.id, permission.id
FROM matrix
JOIN roles role ON role.code = matrix.role_code
JOIN permissions permission ON permission.code = matrix.permission_code
ON CONFLICT (role_id, permission_id) DO NOTHING;

WITH matrix(role_code, permission_code) AS (
  VALUES
    ('SUPER_ADMIN', 'enterprise:certification:revoke'),
    ('SUPER_ADMIN', 'enterprise:subscription:transition'),
    ('SUPER_ADMIN', 'enterprise:service:view'),
    ('SUPER_ADMIN', 'enterprise:service:transition'),
    ('SUPER_ADMIN', 'enterprise:risk:transition'),
    ('SUPER_ADMIN', 'billing:product:view'),
    ('SUPER_ADMIN', 'billing:product:manage'),
    ('SUPER_ADMIN', 'billing:plan:view'),
    ('SUPER_ADMIN', 'billing:plan:manage'),
    ('SUPER_ADMIN', 'billing:plan:publish'),
    ('SUPER_ADMIN', 'billing:model-policy:view'),
    ('SUPER_ADMIN', 'billing:model-policy:manage'),
    ('SUPER_ADMIN', 'billing:model-policy:publish'),
    ('ENTERPRISE_OPERATOR', 'enterprise:subscription:transition'),
    ('ENTERPRISE_OPERATOR', 'enterprise:service:view'),
    ('ENTERPRISE_OPERATOR', 'enterprise:service:transition'),
    ('ENTERPRISE_OPERATOR', 'billing:product:view'),
    ('ENTERPRISE_OPERATOR', 'billing:product:manage'),
    ('ENTERPRISE_OPERATOR', 'billing:plan:view'),
    ('ENTERPRISE_OPERATOR', 'billing:plan:manage'),
    ('ENTERPRISE_OPERATOR', 'billing:model-policy:view'),
    ('ENTERPRISE_OPERATOR', 'billing:model-policy:manage'),
    ('CERTIFICATION_REVIEWER', 'enterprise:certification:revoke'),
    ('FINANCE', 'enterprise:service:view'),
    ('FINANCE', 'billing:product:view'),
    ('FINANCE', 'billing:plan:view'),
    ('FINANCE', 'billing:plan:publish'),
    ('FINANCE', 'billing:model-policy:view'),
    ('FINANCE', 'billing:model-policy:publish'),
    ('RISK_MANAGER', 'enterprise:service:view'),
    ('RISK_MANAGER', 'enterprise:service:transition'),
    ('RISK_MANAGER', 'enterprise:risk:transition'),
    ('CUSTOMER_SERVICE', 'enterprise:service:view'),
    ('CUSTOMER_SERVICE', 'billing:product:view'),
    ('CUSTOMER_SERVICE', 'billing:plan:view'),
    ('CUSTOMER_SERVICE', 'billing:model-policy:view')
)
INSERT INTO xz_role_permissions(role, permission, permission_domain)
SELECT role_code, permission_code, 'PLATFORM'
FROM matrix
ON CONFLICT (role, permission) DO UPDATE
SET permission_domain = EXCLUDED.permission_domain;

-- 企业端新只读权限；角色赋权仍受 tenant_id / organization_id 上下文限制。
WITH matrix(role_code, permission_code) AS (
  VALUES
    ('ENTERPRISE_ADMIN', 'enterprise.package.read'),
    ('ENTERPRISE_ADMIN', 'enterprise.compute.read'),
    ('ENTERPRISE_ADMIN', 'enterprise.usage.read'),
    ('ENTERPRISE_ADMIN', 'enterprise.service.read'),
    ('ENTERPRISE_ADMIN', 'enterprise.certification.read'),
    ('AI_ADMIN', 'enterprise.package.read'),
    ('AI_ADMIN', 'enterprise.compute.read'),
    ('AI_ADMIN', 'enterprise.usage.read'),
    ('AI_ADMIN', 'enterprise.service.read'),
    ('FINANCE', 'enterprise.package.read'),
    ('FINANCE', 'enterprise.compute.read'),
    ('FINANCE', 'enterprise.usage.read'),
    ('FINANCE', 'enterprise.service.read'),
    ('CUSTOMER_SERVICE', 'enterprise.package.read'),
    ('CUSTOMER_SERVICE', 'enterprise.compute.read'),
    ('CUSTOMER_SERVICE', 'enterprise.service.read'),
    ('ENTERPRISE_MEMBER', 'enterprise.package.read'),
    ('ENTERPRISE_MEMBER', 'enterprise.service.read')
)
INSERT INTO xz_role_permissions(role, permission, permission_domain)
SELECT role_code, permission_code, 'TENANT'
FROM matrix
ON CONFLICT (role, permission) DO UPDATE
SET permission_domain = EXCLUDED.permission_domain;

-- ---------------------------------------------------------------------------
-- 6. 数据回填草案（正式执行前必须先生成 dry-run 质量报告）
-- ---------------------------------------------------------------------------

-- 现有 xz_plans 生成版本 1。grant_points 解释为算力最小单位，不解释为 Token。
INSERT INTO xz_plan_versions(
  id, plan_id, version, status, name, currency_code, price_cents,
  billing_cycle, validity_mode, validity_value, entitlement_snapshot,
  grant_snapshot, config, change_reason, published_at
)
SELECT
  'plan_version_' || substr(md5(plan.id || ':1'), 1, 24),
  plan.id,
  1,
  CASE WHEN plan.active THEN 'PUBLISHED' ELSE 'RETIRED' END,
  coalesce(nullif(plan.name, ''), plan.code, plan.id),
  coalesce(nullif(plan.currency_code, ''), 'CNY'),
  plan.price_cents,
  coalesce(nullif(plan.billing_cycle, ''), 'ONE_TIME'),
  CASE WHEN plan.duration_days > 0 THEN 'FIXED_DAYS' ELSE 'CUSTOM' END,
  CASE WHEN plan.duration_days > 0 THEN plan.duration_days::BIGINT ELSE NULL END,
  coalesce(plan.entitlements, '{}'::jsonb),
  CASE WHEN plan.grant_points > 0
    THEN jsonb_build_array(jsonb_build_object(
      'resourceCode', 'COMPUTE_UNIT',
      'baseAmount', plan.grant_points,
      'triggerType', 'ON_ACTIVATION',
      'recurrence', 'ONCE'
    ))
    ELSE '[]'::jsonb
  END,
  jsonb_build_object('source', 'xz_plans', 'legacyRaw', coalesce(plan.raw, '{}'::jsonb)),
  '历史套餐兼容回填',
  CASE WHEN plan.active THEN coalesce(plan.published_at, now()) ELSE NULL END
FROM xz_plans plan
ON CONFLICT (plan_id, version) DO NOTHING;

INSERT INTO xz_plan_grant_rules(
  id, plan_version_id, resource_code, base_amount, bonus_amount,
  trigger_type, recurrence, validity_seconds, status
)
SELECT
  'plan_grant_' || substr(md5(version.id || ':compute'), 1, 24),
  version.id,
  'COMPUTE_UNIT',
  plan.grant_points,
  0,
  'ON_ACTIVATION',
  'ONCE',
  CASE WHEN plan.duration_days > 0 THEN plan.duration_days::BIGINT * 86400 ELSE NULL END,
  CASE WHEN plan.active THEN 'ACTIVE' ELSE 'DISABLED' END
FROM xz_plans plan
JOIN xz_plan_versions version ON version.plan_id = plan.id AND version.version = 1
WHERE plan.grant_points > 0
ON CONFLICT (id) DO NOTHING;

UPDATE xz_tenant_subscriptions subscription
SET plan_version_id = version.id,
    current_period_start = coalesce(subscription.current_period_start, subscription.trial_started_at, subscription.created_at),
    current_period_end = coalesce(subscription.current_period_end, subscription.trial_expires_at),
    last_transition_at = coalesce(subscription.updated_at, now())
FROM xz_plan_versions version
WHERE subscription.plan_id = version.plan_id
  AND version.status = 'PUBLISHED'
  AND version.retired_at IS NULL
  AND subscription.plan_version_id IS NULL;

INSERT INTO xz_tenant_service_states(tenant_id, state, state_version, reason, metadata)
SELECT tenant.id,
       CASE upper(coalesce(tenant.status, 'ACTIVE'))
         WHEN 'SUSPENDED' THEN 'PAUSED'
         WHEN 'DISABLED' THEN 'DISABLED'
         WHEN 'TERMINATED' THEN 'TERMINATED'
         ELSE 'ACTIVE'
       END,
       0,
       '历史 tenant.status 兼容回填',
       jsonb_build_object('legacyTenantStatus', tenant.status)
FROM xz_tenants tenant
WHERE tenant.tenant_type = 'ENTERPRISE'
ON CONFLICT (tenant_id) DO NOTHING;

INSERT INTO xz_tenant_risk_states(tenant_id, state, risk_level, state_version, reason, metadata)
SELECT tenant.id,
       CASE WHEN EXISTS (
         SELECT 1
         FROM xz_admin_enterprise_risk_records risk
         WHERE risk.tenant_id = tenant.id
           AND upper(risk.status) = 'ACTIVE'
           AND upper(risk.action) IN ('RISK-DISABLE', 'RISK_DISABLE')
       ) THEN 'BLOCKED' ELSE 'NORMAL' END,
       CASE WHEN EXISTS (
         SELECT 1
         FROM xz_admin_enterprise_risk_records risk
         WHERE risk.tenant_id = tenant.id
           AND upper(risk.status) = 'ACTIVE'
       ) THEN 'HIGH' ELSE 'LOW' END,
       0,
       '历史风险记录兼容回填',
       '{}'::jsonb
FROM xz_tenants tenant
WHERE tenant.tenant_type = 'ENTERPRISE'
ON CONFLICT (tenant_id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 7. 执行前质量闸门（正式 migration 应拆分为单独预检）
-- ---------------------------------------------------------------------------

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM xz_plan_versions
    WHERE status = 'PUBLISHED' AND change_reason = ''
  ) THEN
    RAISE EXCEPTION 'published plan versions must have a change reason';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM xz_tenant_subscriptions subscription
    JOIN xz_tenants tenant ON tenant.id = subscription.tenant_id
    WHERE tenant.tenant_type <> 'ENTERPRISE'
  ) THEN
    RAISE EXCEPTION 'enterprise subscriptions contain a non-enterprise tenant';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM xz_model_usage_records
    WHERE input_tokens < 0 OR output_tokens < 0 OR cached_input_tokens < 0
       OR reasoning_tokens < 0 OR total_tokens < 0
  ) THEN
    RAISE EXCEPTION 'model token usage must be raw non-negative counts';
  END IF;
END $$;

-- 安全闩：草案即使被人工 psql 执行也不落库。
-- 正式上线必须拆分为多个编号 migration，经备份、dry-run、回滚演练与审批后再使用 COMMIT。
ROLLBACK;
