-- V6.0 user + tenant + organization + role + permission model.
-- The canonical UUID tables serve administration; xz_* tables serve the Go runtime projections.

CREATE TABLE IF NOT EXISTS tenants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(80) NOT NULL UNIQUE,
  name VARCHAR(160) NOT NULL,
  status VARCHAR(30) NOT NULL DEFAULT 'ACTIVE',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS organizations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  parent_id UUID REFERENCES organizations(id),
  code VARCHAR(80) NOT NULL,
  name VARCHAR(160) NOT NULL,
  status VARCHAR(30) NOT NULL DEFAULT 'ACTIVE',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, code)
);

ALTER TABLE user_roles ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id);
ALTER TABLE user_roles ADD COLUMN IF NOT EXISTS organization_id UUID REFERENCES organizations(id);
ALTER TABLE user_roles ADD COLUMN IF NOT EXISTS status VARCHAR(30) NOT NULL DEFAULT 'ACTIVE';
ALTER TABLE user_roles ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE TABLE IF NOT EXISTS user_role_context (
  user_id UUID PRIMARY KEY REFERENCES users(id),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  organization_id UUID NOT NULL REFERENCES organizations(id),
  current_role_id UUID NOT NULL REFERENCES roles(id),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO tenants (code, name)
VALUES ('default', '知启云默认租户')
ON CONFLICT (code) DO NOTHING;

INSERT INTO organizations (tenant_id, code, name)
SELECT id, 'default', '知启云默认组织'
FROM tenants
WHERE code = 'default'
ON CONFLICT (tenant_id, code) DO NOTHING;

INSERT INTO roles (code, name, description)
VALUES
  ('USER', '用户', 'AI 创作、作品、项目、钱包与设置'),
  ('AGENT', '代理商', '推广、客户、分润与提现'),
  ('OPERATION', '运营中心', '区域代理、订单、客户与报表'),
  ('ENTERPRISE_ADMIN', '企业管理员', '企业成员与组织管理'),
  ('AI_ADMIN', 'AI 管理员', '模型、知识库与 AI 能力治理'),
  ('FINANCE', '财务', '账单、发票、分润与资金审核'),
  ('CUSTOMER_SERVICE', '客服', '客户服务、工单与服务记录')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

INSERT INTO permissions (code, name, module, action)
VALUES
  ('ai:use', '使用 AI 能力', 'ai', 'use'),
  ('assets:view', '查看作品资产', 'assets', 'view'),
  ('project:view', '查看项目', 'project', 'view'),
  ('wallet:view', '查看钱包', 'wallet', 'view'),
  ('settings:view', '查看设置', 'settings', 'view'),
  ('agent:promotion', '查看推广中心', 'agent', 'promotion'),
  ('agent:promotion:create', '创建推广内容', 'agent', 'promotion_create'),
  ('agent:qrcode:view', '查看推广二维码', 'agent', 'qrcode_view'),
  ('agent:customer:view', '查看客户', 'agent', 'customer_view'),
  ('agent:commission:view', '查看分润', 'agent', 'commission_view'),
  ('agent:withdraw', '申请提现', 'agent', 'withdraw'),
  ('agent:material:view', '查看推广素材', 'agent', 'material_view'),
  ('operation:dashboard', '查看运营看板', 'operation', 'dashboard'),
  ('operation:agent:list', '查看代理列表', 'operation', 'agent_list'),
  ('operation:agent:approve', '审核代理', 'operation', 'agent_approve'),
  ('operation:order:view', '查看区域订单', 'operation', 'order_view'),
  ('operation:customer:view', '查看区域客户', 'operation', 'customer_view'),
  ('operation:report:view', '查看数据报表', 'operation', 'report_view'),
  ('operation:announcement:manage', '管理公告', 'operation', 'announcement_manage'),
  ('operation:renew', '续费运营中心', 'operation', 'renew'),
  ('enterprise:member:manage', '管理企业成员', 'enterprise', 'member_manage'),
  ('enterprise:organization:manage', '管理企业组织', 'enterprise', 'organization_manage'),
  ('ai:admin', '管理 AI 能力', 'ai', 'admin'),
  ('finance:view', '查看财务数据', 'finance', 'view'),
  ('finance:approve', '审核财务单据', 'finance', 'approve'),
  ('customer-service:manage', '管理客户服务', 'customer_service', 'manage')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, module = EXCLUDED.module, action = EXCLUDED.action;

WITH matrix(role_code, permission_code) AS (
  VALUES
    ('USER','ai:use'), ('USER','assets:view'), ('USER','project:view'), ('USER','wallet:view'), ('USER','settings:view'),
    ('AGENT','agent:promotion'), ('AGENT','agent:promotion:create'), ('AGENT','agent:qrcode:view'),
    ('AGENT','agent:customer:view'), ('AGENT','agent:commission:view'), ('AGENT','agent:withdraw'), ('AGENT','agent:material:view'),
    ('OPERATION','operation:dashboard'), ('OPERATION','operation:agent:list'), ('OPERATION','operation:agent:approve'),
    ('OPERATION','operation:order:view'), ('OPERATION','operation:customer:view'), ('OPERATION','operation:report:view'),
    ('OPERATION','operation:announcement:manage'), ('OPERATION','operation:renew'),
    ('ENTERPRISE_ADMIN','enterprise:member:manage'), ('ENTERPRISE_ADMIN','enterprise:organization:manage'),
    ('AI_ADMIN','ai:admin'), ('FINANCE','finance:view'), ('FINANCE','finance:approve'),
    ('CUSTOMER_SERVICE','customer-service:manage')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM matrix m
JOIN roles r ON r.code = m.role_code
JOIN permissions p ON p.code = m.permission_code
ON CONFLICT (role_id, permission_id) DO NOTHING;

WITH defaults AS (
  SELECT t.id AS tenant_id, o.id AS organization_id
  FROM tenants t
  JOIN organizations o ON o.tenant_id = t.id AND o.code = 'default'
  WHERE t.code = 'default'
)
UPDATE user_roles ur
SET tenant_id = COALESCE(ur.tenant_id, d.tenant_id),
    organization_id = COALESCE(ur.organization_id, d.organization_id),
    updated_at = now()
FROM defaults d
WHERE ur.tenant_id IS NULL OR ur.organization_id IS NULL;

WITH defaults AS (
  SELECT t.id AS tenant_id, o.id AS organization_id
  FROM tenants t
  JOIN organizations o ON o.tenant_id = t.id AND o.code = 'default'
  WHERE t.code = 'default'
), user_role AS (
  SELECT id FROM roles WHERE code = 'USER'
)
INSERT INTO user_roles (user_id, role_id, tenant_id, organization_id)
SELECT u.id, r.id, d.tenant_id, d.organization_id
FROM users u CROSS JOIN defaults d CROSS JOIN user_role r
ON CONFLICT (user_id, role_id) DO UPDATE
SET tenant_id = COALESCE(user_roles.tenant_id, EXCLUDED.tenant_id),
    organization_id = COALESCE(user_roles.organization_id, EXCLUDED.organization_id),
    status = 'ACTIVE',
    updated_at = now();

WITH defaults AS (
  SELECT t.id AS tenant_id, o.id AS organization_id
  FROM tenants t
  JOIN organizations o ON o.tenant_id = t.id AND o.code = 'default'
  WHERE t.code = 'default'
), user_role AS (
  SELECT id FROM roles WHERE code = 'USER'
)
INSERT INTO user_role_context (user_id, tenant_id, organization_id, current_role_id)
SELECT u.id, d.tenant_id, d.organization_id, r.id
FROM users u CROSS JOIN defaults d CROSS JOIN user_role r
ON CONFLICT (user_id) DO NOTHING;

CREATE TABLE IF NOT EXISTS xz_tenants (
  id TEXT PRIMARY KEY,
  tenant_type TEXT NOT NULL CHECK (tenant_type IN ('PLATFORM', 'ENTERPRISE', 'PERSONAL')),
  enterprise_ref TEXT,
  owner_user_id TEXT REFERENCES xz_users(id),
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (id, tenant_type)
);

CREATE TABLE IF NOT EXISTS xz_organizations (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  parent_id TEXT,
  organization_type TEXT NOT NULL DEFAULT 'DEPARTMENT',
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, parent_id) REFERENCES xz_organizations(tenant_id, id)
);

CREATE TABLE IF NOT EXISTS xz_user_roles (
  user_id TEXT NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  organization_id TEXT NOT NULL REFERENCES xz_organizations(id),
  role TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, tenant_id, organization_id, role)
);

CREATE TABLE IF NOT EXISTS xz_user_role_context (
  user_id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  organization_id TEXT NOT NULL REFERENCES xz_organizations(id),
  current_role_code TEXT NOT NULL DEFAULT 'USER',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO xz_tenants (id, tenant_type, name)
VALUES ('tenant_default', 'PLATFORM', '知启云默认租户')
ON CONFLICT (id) DO NOTHING;

INSERT INTO xz_organizations (id, tenant_id, organization_type, name)
SELECT 'organization_default_' || substr(md5(id), 1, 16), id, 'DEPARTMENT', '默认组织'
FROM xz_tenants
ON CONFLICT (id) DO NOTHING;

WITH user_scope AS (
  SELECT u.id AS user_id,
         coalesce((
           SELECT tm.tenant_id FROM xz_tenant_members tm
           WHERE tm.user_id = u.id AND upper(tm.status) = 'ACTIVE'
           ORDER BY tm.created_at LIMIT 1
         ), 'tenant_default') AS tenant_id
  FROM xz_users u
)
INSERT INTO xz_user_roles (user_id, tenant_id, organization_id, role)
SELECT user_id, tenant_id, 'organization_default_' || substr(md5(tenant_id), 1, 16), 'USER'
FROM user_scope
ON CONFLICT (user_id, tenant_id, organization_id, role) DO UPDATE SET status = 'ACTIVE', updated_at = now();

INSERT INTO xz_user_roles (user_id, tenant_id, organization_id, role)
SELECT agent.user_id, scope.tenant_id, scope.organization_id, 'AGENT'
FROM xz_channel_agents agent
JOIN xz_user_roles scope ON scope.user_id = agent.user_id AND scope.role = 'USER' AND upper(scope.status) = 'ACTIVE'
WHERE upper(coalesce(agent.status, 'ACTIVE')) = 'ACTIVE'
ON CONFLICT (user_id, tenant_id, organization_id, role) DO UPDATE SET status = 'ACTIVE', updated_at = now();

INSERT INTO xz_user_roles (user_id, tenant_id, organization_id, role)
SELECT center.user_id, scope.tenant_id, scope.organization_id, 'OPERATION'
FROM xz_operation_centers center
JOIN xz_user_roles scope ON scope.user_id = center.user_id AND scope.role = 'USER' AND upper(scope.status) = 'ACTIVE'
WHERE upper(coalesce(center.status, '')) = 'ACTIVE'
ON CONFLICT (user_id, tenant_id, organization_id, role) DO UPDATE SET status = 'ACTIVE', updated_at = now();

INSERT INTO xz_user_role_context (user_id, tenant_id, organization_id, current_role_code)
SELECT user_id, tenant_id, organization_id, 'USER'
FROM xz_user_roles
WHERE role = 'USER' AND upper(status) = 'ACTIVE'
ON CONFLICT (user_id) DO NOTHING;

INSERT INTO xz_role_permissions (role, permission)
VALUES
  ('USER','ai:use'), ('USER','assets:view'), ('USER','project:view'), ('USER','wallet:view'), ('USER','settings:view'),
  ('AGENT','agent:promotion'), ('AGENT','agent:promotion:create'), ('AGENT','agent:qrcode:view'),
  ('AGENT','agent:customer:view'), ('AGENT','agent:commission:view'), ('AGENT','agent:withdraw'), ('AGENT','agent:material:view'),
  ('OPERATION','operation:dashboard'), ('OPERATION','operation:agent:list'), ('OPERATION','operation:agent:approve'),
  ('OPERATION','operation:order:view'), ('OPERATION','operation:customer:view'), ('OPERATION','operation:report:view'),
  ('OPERATION','operation:announcement:manage'), ('OPERATION','operation:renew'),
  ('ENTERPRISE_ADMIN','enterprise:member:manage'), ('ENTERPRISE_ADMIN','enterprise:organization:manage'),
  ('AI_ADMIN','ai:admin'), ('FINANCE','finance:view'), ('FINANCE','finance:approve'),
  ('CUSTOMER_SERVICE','customer-service:manage')
ON CONFLICT (role, permission) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_xz_user_roles_user ON xz_user_roles(user_id, status);
CREATE INDEX IF NOT EXISTS idx_xz_user_roles_scope ON xz_user_roles(tenant_id, organization_id, role, status);
CREATE INDEX IF NOT EXISTS idx_xz_organizations_rbac_scope ON xz_organizations(tenant_id, status);
