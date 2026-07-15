-- 主控 SaaS 企业管理中心（第一阶段）。
-- 复用 xz_tenants、xz_organizations、xz_tenant_members、xz_tenant_wallets、
-- xz_tenant_subscriptions、xz_tenant_certifications 与 xz_tenant_audit_logs，
-- 不创建第二套 tenant / organization / user 模型。

BEGIN;

ALTER TABLE xz_tenants
  ADD COLUMN IF NOT EXISTS enterprise_code TEXT,
  ADD COLUMN IF NOT EXISTS source_agent_id TEXT,
  ADD COLUMN IF NOT EXISTS operation_center_id TEXT,
  ADD COLUMN IF NOT EXISTS seat_limit INTEGER NOT NULL DEFAULT 20,
  ADD COLUMN IF NOT EXISTS industry TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS company_size TEXT NOT NULL DEFAULT '';

UPDATE xz_tenants
SET enterprise_code = 'ENT-' || upper(substr(md5(id), 1, 12))
WHERE tenant_type = 'ENTERPRISE'
  AND coalesce(enterprise_code, '') = '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_xz_tenants_enterprise_code
  ON xz_tenants(enterprise_code)
  WHERE tenant_type = 'ENTERPRISE';

CREATE INDEX IF NOT EXISTS idx_xz_tenants_enterprise_admin_filter
  ON xz_tenants(tenant_type, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_xz_tenants_enterprise_relation
  ON xz_tenants(source_agent_id, operation_center_id)
  WHERE tenant_type = 'ENTERPRISE';

ALTER TABLE xz_orders ADD COLUMN IF NOT EXISTS tenant_id TEXT;
CREATE INDEX IF NOT EXISTS idx_xz_orders_tenant ON xz_orders(tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_tenant_point_transactions (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  transaction_type TEXT NOT NULL,
  point_delta BIGINT NOT NULL,
  balance_after BIGINT NOT NULL,
  reference_type TEXT NOT NULL DEFAULT '',
  reference_id TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL,
  actor_user_id TEXT,
  request_id TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_xz_tenant_point_transactions_scope
  ON xz_tenant_point_transactions(tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_admin_enterprise_change_requests (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  action_type TEXT NOT NULL,
  reason TEXT NOT NULL,
  before_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  after_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'PENDING_APPROVAL',
  requested_by TEXT,
  approved_by TEXT,
  request_id TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_xz_admin_enterprise_change_requests_scope
  ON xz_admin_enterprise_change_requests(tenant_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_admin_enterprise_requests (
  request_id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  action TEXT NOT NULL,
  status TEXT NOT NULL,
  result JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS xz_admin_enterprise_risk_records (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  risk_level TEXT NOT NULL DEFAULT 'MEDIUM',
  action TEXT NOT NULL,
  reason TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  actor_user_id TEXT,
  request_id TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_xz_admin_enterprise_risk_records_scope
  ON xz_admin_enterprise_risk_records(tenant_id, created_at DESC);

INSERT INTO roles (code, name, description)
VALUES
  ('SUPER_ADMIN', '平台超级管理员', '主控 SaaS 全量管理权限'),
  ('ENTERPRISE_OPERATOR', '企业运营', '企业资料、套餐、算力与归属运营'),
  ('CERTIFICATION_REVIEWER', '认证审核员', '企业认证资料审核'),
  ('FINANCE', '财务', '企业套餐、算力、流水与订单查看'),
  ('RISK_MANAGER', '风控', '企业风险查看、暂停与恢复服务'),
  ('CUSTOMER_SERVICE', '客服', '企业服务信息只读查看')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    description = EXCLUDED.description,
    status = 'ACTIVE';

INSERT INTO permissions (code, name, module, action)
VALUES
  ('enterprise:list', '查看企业列表', 'enterprise', 'list'),
  ('enterprise:detail', '查看企业详情', 'enterprise', 'detail'),
  ('enterprise:create', '创建企业', 'enterprise', 'create'),
  ('enterprise:update', '修改企业资料', 'enterprise', 'update'),
  ('enterprise:certification:review', '审核企业认证', 'enterprise', 'certification_review'),
  ('enterprise:member:view', '查看企业成员', 'enterprise', 'member_view'),
  ('enterprise:package:view', '查看企业套餐', 'enterprise', 'package_view'),
  ('enterprise:package:adjust', '调整企业套餐', 'enterprise', 'package_adjust'),
  ('enterprise:seat:adjust', '调整成员席位', 'enterprise', 'seat_adjust'),
  ('enterprise:compute:view', '查看企业算力', 'enterprise', 'compute_view'),
  ('enterprise:compute:adjust', '调整企业算力', 'enterprise', 'compute_adjust'),
  ('enterprise:transaction:view', '查看充值消费明细', 'enterprise', 'transaction_view'),
  ('enterprise:order:view', '查看企业订单', 'enterprise', 'order_view'),
  ('enterprise:ai:view', '查看企业 AI 能力', 'enterprise', 'ai_view'),
  ('enterprise:ai:configure', '配置企业 AI 能力', 'enterprise', 'ai_configure'),
  ('enterprise:employee:view', '查看企业 AI 员工', 'enterprise', 'employee_view'),
  ('enterprise:knowledge:view', '查看企业知识库统计', 'enterprise', 'knowledge_view'),
  ('enterprise:attribution:view', '查看企业客户归属', 'enterprise', 'attribution_view'),
  ('enterprise:attribution:change', '变更企业归属', 'enterprise', 'attribution_change'),
  ('enterprise:risk:view', '查看企业风险', 'enterprise', 'risk_view'),
  ('enterprise:risk:disable', '暂停企业服务', 'enterprise', 'risk_disable'),
  ('enterprise:risk:restore', '恢复企业服务', 'enterprise', 'risk_restore'),
  ('enterprise:audit:view', '查看企业审计日志', 'enterprise', 'audit_view'),
  ('enterprise:export', '导出企业数据', 'enterprise', 'export')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    module = EXCLUDED.module,
    action = EXCLUDED.action;

WITH matrix(role_code, permission_code) AS (
  VALUES
    ('SUPER_ADMIN','enterprise:list'), ('SUPER_ADMIN','enterprise:detail'), ('SUPER_ADMIN','enterprise:create'),
    ('SUPER_ADMIN','enterprise:update'), ('SUPER_ADMIN','enterprise:certification:review'), ('SUPER_ADMIN','enterprise:member:view'),
    ('SUPER_ADMIN','enterprise:package:view'), ('SUPER_ADMIN','enterprise:package:adjust'), ('SUPER_ADMIN','enterprise:seat:adjust'),
    ('SUPER_ADMIN','enterprise:compute:view'), ('SUPER_ADMIN','enterprise:compute:adjust'), ('SUPER_ADMIN','enterprise:transaction:view'),
    ('SUPER_ADMIN','enterprise:order:view'), ('SUPER_ADMIN','enterprise:ai:view'), ('SUPER_ADMIN','enterprise:ai:configure'),
    ('SUPER_ADMIN','enterprise:employee:view'), ('SUPER_ADMIN','enterprise:knowledge:view'), ('SUPER_ADMIN','enterprise:attribution:view'),
    ('SUPER_ADMIN','enterprise:attribution:change'), ('SUPER_ADMIN','enterprise:risk:view'), ('SUPER_ADMIN','enterprise:risk:disable'),
    ('SUPER_ADMIN','enterprise:risk:restore'), ('SUPER_ADMIN','enterprise:audit:view'), ('SUPER_ADMIN','enterprise:export'),

    ('ENTERPRISE_OPERATOR','enterprise:list'), ('ENTERPRISE_OPERATOR','enterprise:detail'), ('ENTERPRISE_OPERATOR','enterprise:create'),
    ('ENTERPRISE_OPERATOR','enterprise:update'), ('ENTERPRISE_OPERATOR','enterprise:member:view'), ('ENTERPRISE_OPERATOR','enterprise:package:view'),
    ('ENTERPRISE_OPERATOR','enterprise:package:adjust'), ('ENTERPRISE_OPERATOR','enterprise:seat:adjust'), ('ENTERPRISE_OPERATOR','enterprise:compute:view'),
    ('ENTERPRISE_OPERATOR','enterprise:compute:adjust'), ('ENTERPRISE_OPERATOR','enterprise:transaction:view'), ('ENTERPRISE_OPERATOR','enterprise:order:view'),
    ('ENTERPRISE_OPERATOR','enterprise:ai:view'), ('ENTERPRISE_OPERATOR','enterprise:ai:configure'), ('ENTERPRISE_OPERATOR','enterprise:employee:view'),
    ('ENTERPRISE_OPERATOR','enterprise:knowledge:view'), ('ENTERPRISE_OPERATOR','enterprise:attribution:view'), ('ENTERPRISE_OPERATOR','enterprise:attribution:change'),
    ('ENTERPRISE_OPERATOR','enterprise:risk:view'), ('ENTERPRISE_OPERATOR','enterprise:audit:view'), ('ENTERPRISE_OPERATOR','enterprise:export'),

    ('CERTIFICATION_REVIEWER','enterprise:list'), ('CERTIFICATION_REVIEWER','enterprise:detail'),
    ('CERTIFICATION_REVIEWER','enterprise:certification:review'), ('CERTIFICATION_REVIEWER','enterprise:audit:view'),
    ('CERTIFICATION_REVIEWER','enterprise:export'),

    ('FINANCE','enterprise:list'), ('FINANCE','enterprise:detail'), ('FINANCE','enterprise:package:view'),
    ('FINANCE','enterprise:compute:view'), ('FINANCE','enterprise:compute:adjust'), ('FINANCE','enterprise:transaction:view'),
    ('FINANCE','enterprise:order:view'), ('FINANCE','enterprise:audit:view'), ('FINANCE','enterprise:export'),

    ('RISK_MANAGER','enterprise:list'), ('RISK_MANAGER','enterprise:detail'), ('RISK_MANAGER','enterprise:risk:view'),
    ('RISK_MANAGER','enterprise:risk:disable'), ('RISK_MANAGER','enterprise:risk:restore'),
    ('RISK_MANAGER','enterprise:audit:view'), ('RISK_MANAGER','enterprise:export'),

    ('CUSTOMER_SERVICE','enterprise:list'), ('CUSTOMER_SERVICE','enterprise:detail'), ('CUSTOMER_SERVICE','enterprise:member:view'),
    ('CUSTOMER_SERVICE','enterprise:package:view'), ('CUSTOMER_SERVICE','enterprise:compute:view'), ('CUSTOMER_SERVICE','enterprise:order:view'),
    ('CUSTOMER_SERVICE','enterprise:ai:view'), ('CUSTOMER_SERVICE','enterprise:employee:view'), ('CUSTOMER_SERVICE','enterprise:knowledge:view'),
    ('CUSTOMER_SERVICE','enterprise:attribution:view'), ('CUSTOMER_SERVICE','enterprise:risk:view')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT role.id, permission.id
FROM matrix
JOIN roles role ON role.code = matrix.role_code
JOIN permissions permission ON permission.code = matrix.permission_code
ON CONFLICT (role_id, permission_id) DO NOTHING;

WITH matrix(role_code, permission_code) AS (
  VALUES
    ('SUPER_ADMIN','enterprise:list'), ('SUPER_ADMIN','enterprise:detail'), ('SUPER_ADMIN','enterprise:create'),
    ('SUPER_ADMIN','enterprise:update'), ('SUPER_ADMIN','enterprise:certification:review'), ('SUPER_ADMIN','enterprise:member:view'),
    ('SUPER_ADMIN','enterprise:package:view'), ('SUPER_ADMIN','enterprise:package:adjust'), ('SUPER_ADMIN','enterprise:seat:adjust'),
    ('SUPER_ADMIN','enterprise:compute:view'), ('SUPER_ADMIN','enterprise:compute:adjust'), ('SUPER_ADMIN','enterprise:transaction:view'),
    ('SUPER_ADMIN','enterprise:order:view'), ('SUPER_ADMIN','enterprise:ai:view'), ('SUPER_ADMIN','enterprise:ai:configure'),
    ('SUPER_ADMIN','enterprise:employee:view'), ('SUPER_ADMIN','enterprise:knowledge:view'), ('SUPER_ADMIN','enterprise:attribution:view'),
    ('SUPER_ADMIN','enterprise:attribution:change'), ('SUPER_ADMIN','enterprise:risk:view'), ('SUPER_ADMIN','enterprise:risk:disable'),
    ('SUPER_ADMIN','enterprise:risk:restore'), ('SUPER_ADMIN','enterprise:audit:view'), ('SUPER_ADMIN','enterprise:export'),
    ('ENTERPRISE_OPERATOR','enterprise:list'), ('ENTERPRISE_OPERATOR','enterprise:detail'), ('ENTERPRISE_OPERATOR','enterprise:create'),
    ('ENTERPRISE_OPERATOR','enterprise:update'), ('ENTERPRISE_OPERATOR','enterprise:member:view'), ('ENTERPRISE_OPERATOR','enterprise:package:view'),
    ('ENTERPRISE_OPERATOR','enterprise:package:adjust'), ('ENTERPRISE_OPERATOR','enterprise:seat:adjust'), ('ENTERPRISE_OPERATOR','enterprise:compute:view'),
    ('ENTERPRISE_OPERATOR','enterprise:compute:adjust'), ('ENTERPRISE_OPERATOR','enterprise:transaction:view'), ('ENTERPRISE_OPERATOR','enterprise:order:view'),
    ('ENTERPRISE_OPERATOR','enterprise:ai:view'), ('ENTERPRISE_OPERATOR','enterprise:ai:configure'), ('ENTERPRISE_OPERATOR','enterprise:employee:view'),
    ('ENTERPRISE_OPERATOR','enterprise:knowledge:view'), ('ENTERPRISE_OPERATOR','enterprise:attribution:view'), ('ENTERPRISE_OPERATOR','enterprise:attribution:change'),
    ('ENTERPRISE_OPERATOR','enterprise:risk:view'), ('ENTERPRISE_OPERATOR','enterprise:audit:view'), ('ENTERPRISE_OPERATOR','enterprise:export'),
    ('CERTIFICATION_REVIEWER','enterprise:list'), ('CERTIFICATION_REVIEWER','enterprise:detail'),
    ('CERTIFICATION_REVIEWER','enterprise:certification:review'), ('CERTIFICATION_REVIEWER','enterprise:audit:view'),
    ('CERTIFICATION_REVIEWER','enterprise:export'),
    ('FINANCE','enterprise:list'), ('FINANCE','enterprise:detail'), ('FINANCE','enterprise:package:view'),
    ('FINANCE','enterprise:compute:view'), ('FINANCE','enterprise:compute:adjust'), ('FINANCE','enterprise:transaction:view'),
    ('FINANCE','enterprise:order:view'), ('FINANCE','enterprise:audit:view'), ('FINANCE','enterprise:export'),
    ('RISK_MANAGER','enterprise:list'), ('RISK_MANAGER','enterprise:detail'), ('RISK_MANAGER','enterprise:risk:view'),
    ('RISK_MANAGER','enterprise:risk:disable'), ('RISK_MANAGER','enterprise:risk:restore'),
    ('RISK_MANAGER','enterprise:audit:view'), ('RISK_MANAGER','enterprise:export'),
    ('CUSTOMER_SERVICE','enterprise:list'), ('CUSTOMER_SERVICE','enterprise:detail'), ('CUSTOMER_SERVICE','enterprise:member:view'),
    ('CUSTOMER_SERVICE','enterprise:package:view'), ('CUSTOMER_SERVICE','enterprise:compute:view'), ('CUSTOMER_SERVICE','enterprise:order:view'),
    ('CUSTOMER_SERVICE','enterprise:ai:view'), ('CUSTOMER_SERVICE','enterprise:employee:view'), ('CUSTOMER_SERVICE','enterprise:knowledge:view'),
    ('CUSTOMER_SERVICE','enterprise:attribution:view'), ('CUSTOMER_SERVICE','enterprise:risk:view')
)
INSERT INTO xz_role_permissions (role, permission)
SELECT role_code, permission_code FROM matrix
ON CONFLICT (role, permission) DO NOTHING;

COMMIT;
