-- Enterprise Center V1 phase 1.
-- Extends the existing xz_* tenant/organization/RBAC projection; no parallel tenancy model is introduced.

BEGIN;

ALTER TABLE xz_tenant_members
  ADD COLUMN IF NOT EXISTS primary_organization_id TEXT,
  ADD COLUMN IF NOT EXISTS member_status TEXT NOT NULL DEFAULT 'ACTIVE',
  ADD COLUMN IF NOT EXISTS certification_status TEXT NOT NULL DEFAULT 'UNVERIFIED',
  ADD COLUMN IF NOT EXISTS data_scope TEXT NOT NULL DEFAULT 'SELF',
  ADD COLUMN IF NOT EXISTS joined_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS invited_by TEXT REFERENCES xz_users(id),
  ADD COLUMN IF NOT EXISTS disabled_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS disabled_by TEXT REFERENCES xz_users(id),
  ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE xz_tenant_members
SET member_status = upper(coalesce(nullif(status, ''), member_status, 'ACTIVE')),
    joined_at = coalesce(joined_at, created_at),
    data_scope = CASE upper(coalesce(role, ''))
      WHEN 'ENTERPRISE_ADMIN' THEN 'TENANT_ALL'
      WHEN 'PLATFORM_ADMIN' THEN 'TENANT_ALL'
      ELSE coalesce(nullif(data_scope, ''), 'SELF')
    END;

UPDATE xz_tenant_members member
SET primary_organization_id = (
  SELECT organization.id
  FROM xz_organizations organization
  WHERE organization.tenant_id = member.tenant_id
    AND upper(coalesce(organization.status, 'ACTIVE')) = 'ACTIVE'
  ORDER BY CASE WHEN organization.parent_id IS NULL THEN 0 ELSE 1 END, organization.created_at, organization.id
  LIMIT 1
)
WHERE member.primary_organization_id IS NULL;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_xz_tenant_members_primary_organization'
  ) THEN
    ALTER TABLE xz_tenant_members
      ADD CONSTRAINT fk_xz_tenant_members_primary_organization
      FOREIGN KEY (tenant_id, primary_organization_id)
      REFERENCES xz_organizations(tenant_id, id);
  END IF;
END $$;

ALTER TABLE xz_user_role_context
  ADD COLUMN IF NOT EXISTS context_type TEXT NOT NULL DEFAULT 'PERSONAL';

UPDATE xz_user_role_context
SET context_type = CASE
  WHEN current_role_code = 'AGENT' THEN 'AGENT'
  WHEN current_role_code = 'OPERATION' THEN 'OPERATION'
  WHEN current_role_code IN ('ENTERPRISE_ADMIN', 'AI_ADMIN', 'FINANCE', 'CUSTOMER_SERVICE', 'ENTERPRISE_MEMBER') THEN 'ENTERPRISE'
  ELSE 'PERSONAL'
END;

CREATE TABLE IF NOT EXISTS xz_tenant_invitations (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  invitation_code TEXT NOT NULL UNIQUE,
  invited_user_id TEXT REFERENCES xz_users(id),
  invited_email TEXT,
  default_organization_id TEXT,
  default_role TEXT NOT NULL DEFAULT 'ENTERPRISE_MEMBER',
  expires_at TIMESTAMPTZ NOT NULL,
  status TEXT NOT NULL DEFAULT 'PENDING',
  created_by TEXT NOT NULL REFERENCES xz_users(id),
  accepted_by TEXT REFERENCES xz_users(id),
  accepted_at TIMESTAMPTZ,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, default_organization_id) REFERENCES xz_organizations(tenant_id, id)
);

CREATE TABLE IF NOT EXISTS xz_tenant_join_requests (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  applicant_user_id TEXT NOT NULL REFERENCES xz_users(id),
  requested_organization_id TEXT,
  requested_role TEXT NOT NULL DEFAULT 'ENTERPRISE_MEMBER',
  reason TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'PENDING',
  reviewed_by TEXT REFERENCES xz_users(id),
  reviewed_at TIMESTAMPTZ,
  review_comment TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, requested_organization_id) REFERENCES xz_organizations(tenant_id, id)
);

CREATE TABLE IF NOT EXISTS xz_tenant_wallets (
  tenant_id TEXT PRIMARY KEY REFERENCES xz_tenants(id),
  point_balance BIGINT NOT NULL DEFAULT 0,
  frozen_points BIGINT NOT NULL DEFAULT 0,
  cash_balance_cents BIGINT NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS xz_tenant_subscriptions (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  plan_id TEXT REFERENCES xz_plans(id),
  plan_code TEXT NOT NULL DEFAULT 'enterprise_trial',
  status TEXT NOT NULL DEFAULT 'TRIALING',
  trial_started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  trial_expires_at TIMESTAMPTZ,
  entitlements JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id)
);

CREATE TABLE IF NOT EXISTS xz_tenant_audit_logs (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  actor_user_id TEXT REFERENCES xz_users(id),
  actor_role TEXT,
  organization_id TEXT,
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT,
  target_user_id TEXT REFERENCES xz_users(id),
  request_id TEXT,
  ip_address TEXT,
  status TEXT NOT NULL DEFAULT 'SUCCEEDED',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, organization_id) REFERENCES xz_organizations(tenant_id, id)
);

CREATE TABLE IF NOT EXISTS xz_tenant_certifications (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  legal_name TEXT NOT NULL,
  unified_social_credit_code TEXT NOT NULL,
  legal_representative_name TEXT NOT NULL DEFAULT '',
  document_urls JSONB NOT NULL DEFAULT '[]'::jsonb,
  status TEXT NOT NULL DEFAULT 'PENDING',
  submitted_by TEXT NOT NULL REFERENCES xz_users(id),
  reviewed_by TEXT REFERENCES xz_users(id),
  reviewed_at TIMESTAMPTZ,
  review_comment TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id)
);

INSERT INTO roles (code, name, description)
VALUES ('ENTERPRISE_MEMBER', '企业成员', '企业工作空间内的普通成员身份')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

INSERT INTO permissions (code, name, module, action)
VALUES
  ('enterprise.overview.read', '查看企业概览', 'enterprise', 'overview_read'),
  ('enterprise.organization.read', '查看企业组织', 'enterprise', 'organization_read'),
  ('enterprise.organization.create', '创建企业组织', 'enterprise', 'organization_create'),
  ('enterprise.organization.update', '更新企业组织', 'enterprise', 'organization_update'),
  ('enterprise.organization.delete', '删除企业组织', 'enterprise', 'organization_delete'),
  ('enterprise.member.read', '查看企业成员', 'enterprise', 'member_read'),
  ('enterprise.member.invite', '邀请企业成员', 'enterprise', 'member_invite'),
  ('enterprise.member.update', '更新企业成员', 'enterprise', 'member_update'),
  ('enterprise.member.disable', '禁用企业成员', 'enterprise', 'member_disable'),
  ('enterprise.member.remove', '移除企业成员', 'enterprise', 'member_remove'),
  ('enterprise.role.read', '查看企业角色', 'enterprise', 'role_read'),
  ('enterprise.role.assign', '分配企业角色', 'enterprise', 'role_assign'),
  ('enterprise.billing.read', '查看企业财务', 'enterprise', 'billing_read'),
  ('enterprise.audit.read', '查看企业审计', 'enterprise', 'audit_read'),
  ('enterprise.settings.read', '查看企业设置', 'enterprise', 'settings_read'),
  ('enterprise.settings.update', '更新企业设置', 'enterprise', 'settings_update'),
  ('enterprise.certification.submit', '提交企业认证', 'enterprise', 'certification_submit')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, module = EXCLUDED.module, action = EXCLUDED.action;

WITH matrix(role_code, permission_code) AS (
  VALUES
    ('ENTERPRISE_ADMIN','enterprise.overview.read'),
    ('ENTERPRISE_ADMIN','enterprise.organization.read'),
    ('ENTERPRISE_ADMIN','enterprise.organization.create'),
    ('ENTERPRISE_ADMIN','enterprise.organization.update'),
    ('ENTERPRISE_ADMIN','enterprise.organization.delete'),
    ('ENTERPRISE_ADMIN','enterprise.member.read'),
    ('ENTERPRISE_ADMIN','enterprise.member.invite'),
    ('ENTERPRISE_ADMIN','enterprise.member.update'),
    ('ENTERPRISE_ADMIN','enterprise.member.disable'),
    ('ENTERPRISE_ADMIN','enterprise.member.remove'),
    ('ENTERPRISE_ADMIN','enterprise.role.read'),
    ('ENTERPRISE_ADMIN','enterprise.role.assign'),
    ('ENTERPRISE_ADMIN','enterprise.billing.read'),
    ('ENTERPRISE_ADMIN','enterprise.audit.read'),
    ('ENTERPRISE_ADMIN','enterprise.settings.read'),
    ('ENTERPRISE_ADMIN','enterprise.settings.update'),
    ('ENTERPRISE_ADMIN','enterprise.certification.submit'),
    ('AI_ADMIN','enterprise.overview.read'),
    ('AI_ADMIN','enterprise.organization.read'),
    ('AI_ADMIN','enterprise.member.read'),
    ('AI_ADMIN','enterprise.role.read'),
    ('AI_ADMIN','enterprise.settings.read'),
    ('FINANCE','enterprise.overview.read'),
    ('FINANCE','enterprise.member.read'),
    ('FINANCE','enterprise.billing.read'),
    ('FINANCE','enterprise.audit.read'),
    ('CUSTOMER_SERVICE','enterprise.overview.read'),
    ('CUSTOMER_SERVICE','enterprise.organization.read'),
    ('CUSTOMER_SERVICE','enterprise.member.read'),
    ('ENTERPRISE_MEMBER','enterprise.overview.read'),
    ('ENTERPRISE_MEMBER','enterprise.organization.read'),
    ('ENTERPRISE_MEMBER','enterprise.member.read'),
    ('ENTERPRISE_MEMBER','enterprise.settings.read')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT role.id, permission.id
FROM matrix
JOIN roles role ON role.code = matrix.role_code
JOIN permissions permission ON permission.code = matrix.permission_code
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO xz_role_permissions (role, permission)
SELECT * FROM (VALUES
  ('ENTERPRISE_ADMIN','enterprise.overview.read'),
  ('ENTERPRISE_ADMIN','enterprise.organization.read'),
  ('ENTERPRISE_ADMIN','enterprise.organization.create'),
  ('ENTERPRISE_ADMIN','enterprise.organization.update'),
  ('ENTERPRISE_ADMIN','enterprise.organization.delete'),
  ('ENTERPRISE_ADMIN','enterprise.member.read'),
  ('ENTERPRISE_ADMIN','enterprise.member.invite'),
  ('ENTERPRISE_ADMIN','enterprise.member.update'),
  ('ENTERPRISE_ADMIN','enterprise.member.disable'),
  ('ENTERPRISE_ADMIN','enterprise.member.remove'),
  ('ENTERPRISE_ADMIN','enterprise.role.read'),
  ('ENTERPRISE_ADMIN','enterprise.role.assign'),
  ('ENTERPRISE_ADMIN','enterprise.billing.read'),
  ('ENTERPRISE_ADMIN','enterprise.audit.read'),
  ('ENTERPRISE_ADMIN','enterprise.settings.read'),
  ('ENTERPRISE_ADMIN','enterprise.settings.update'),
  ('ENTERPRISE_ADMIN','enterprise.certification.submit'),
  ('AI_ADMIN','enterprise.overview.read'),
  ('AI_ADMIN','enterprise.organization.read'),
  ('AI_ADMIN','enterprise.member.read'),
  ('AI_ADMIN','enterprise.role.read'),
  ('AI_ADMIN','enterprise.settings.read'),
  ('FINANCE','enterprise.overview.read'),
  ('FINANCE','enterprise.member.read'),
  ('FINANCE','enterprise.billing.read'),
  ('FINANCE','enterprise.audit.read'),
  ('CUSTOMER_SERVICE','enterprise.overview.read'),
  ('CUSTOMER_SERVICE','enterprise.organization.read'),
  ('CUSTOMER_SERVICE','enterprise.member.read'),
  ('ENTERPRISE_MEMBER','enterprise.overview.read'),
  ('ENTERPRISE_MEMBER','enterprise.organization.read'),
  ('ENTERPRISE_MEMBER','enterprise.member.read'),
  ('ENTERPRISE_MEMBER','enterprise.settings.read')
) AS matrix(role, permission)
ON CONFLICT (role, permission) DO NOTHING;

-- Existing tenant membership is the source of truth. Roles are projected separately.
INSERT INTO xz_user_roles (user_id, tenant_id, organization_id, role, status)
SELECT member.user_id,
       member.tenant_id,
       member.primary_organization_id,
       CASE upper(member.role)
         WHEN 'ENTERPRISE_ADMIN' THEN 'ENTERPRISE_ADMIN'
         WHEN 'PLATFORM_ADMIN' THEN 'ENTERPRISE_ADMIN'
         ELSE 'ENTERPRISE_MEMBER'
       END,
       CASE WHEN upper(member.member_status) = 'ACTIVE' THEN 'ACTIVE' ELSE 'DISABLED' END
FROM xz_tenant_members member
JOIN xz_tenants tenant ON tenant.id = member.tenant_id AND tenant.tenant_type = 'ENTERPRISE'
WHERE member.primary_organization_id IS NOT NULL
ON CONFLICT (user_id, tenant_id, organization_id, role)
DO UPDATE SET status = EXCLUDED.status, updated_at = now();

-- Safe backfill for enterprise roles that predate TenantMember.
INSERT INTO xz_tenant_members (
  id, tenant_id, user_id, role, status, primary_organization_id,
  member_status, joined_at, data_scope
)
SELECT 'tenant_member_' || substr(md5(role.user_id || ':' || role.tenant_id), 1, 20),
       role.tenant_id,
       role.user_id,
       CASE WHEN role.role = 'ENTERPRISE_ADMIN' THEN 'ENTERPRISE_ADMIN' ELSE 'MEMBER' END,
       CASE WHEN upper(role.status) = 'ACTIVE' THEN 'ACTIVE' ELSE 'DISABLED' END,
       role.organization_id,
       CASE WHEN upper(role.status) = 'ACTIVE' THEN 'ACTIVE' ELSE 'DISABLED' END,
       coalesce(role.assigned_at, now()),
       CASE WHEN role.role = 'ENTERPRISE_ADMIN' THEN 'TENANT_ALL' ELSE 'SELF' END
FROM xz_user_roles role
JOIN xz_tenants tenant ON tenant.id = role.tenant_id AND tenant.tenant_type = 'ENTERPRISE'
WHERE role.role IN ('ENTERPRISE_ADMIN', 'AI_ADMIN', 'FINANCE', 'CUSTOMER_SERVICE', 'ENTERPRISE_MEMBER')
ON CONFLICT (tenant_id, user_id) DO NOTHING;

-- Personal and channel identities always live in the platform context.
INSERT INTO xz_organizations (id, tenant_id, organization_type, name)
VALUES ('organization_default_' || substr(md5('tenant_default'), 1, 16), 'tenant_default', 'DEPARTMENT', '默认组织')
ON CONFLICT (id) DO NOTHING;

INSERT INTO xz_user_roles (user_id, tenant_id, organization_id, role, status)
SELECT user_account.id,
       'tenant_default',
       'organization_default_' || substr(md5('tenant_default'), 1, 16),
       'USER',
       'ACTIVE'
FROM xz_users user_account
ON CONFLICT (user_id, tenant_id, organization_id, role)
DO UPDATE SET status = 'ACTIVE', updated_at = now();

CREATE INDEX IF NOT EXISTS idx_xz_tenant_members_scope
  ON xz_tenant_members(tenant_id, member_status, primary_organization_id, user_id);
CREATE INDEX IF NOT EXISTS idx_xz_tenant_invitations_tenant_status
  ON xz_tenant_invitations(tenant_id, status, expires_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_tenant_invitations_invitee
  ON xz_tenant_invitations(invited_user_id, lower(invited_email), status);
CREATE INDEX IF NOT EXISTS idx_xz_tenant_join_requests_tenant_status
  ON xz_tenant_join_requests(tenant_id, status, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_xz_tenant_join_requests_pending_user
  ON xz_tenant_join_requests(tenant_id, applicant_user_id)
  WHERE upper(status) = 'PENDING';
CREATE INDEX IF NOT EXISTS idx_xz_tenant_subscriptions_active
  ON xz_tenant_subscriptions(tenant_id, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_tenant_audit_logs_scope
  ON xz_tenant_audit_logs(tenant_id, created_at DESC, action);
CREATE INDEX IF NOT EXISTS idx_xz_tenant_audit_logs_target
  ON xz_tenant_audit_logs(tenant_id, target_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_tenant_certifications_scope
  ON xz_tenant_certifications(tenant_id, status, updated_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_xz_tenant_certifications_pending
  ON xz_tenant_certifications(tenant_id)
  WHERE upper(status) = 'PENDING';
CREATE INDEX IF NOT EXISTS idx_xz_user_role_context_type
  ON xz_user_role_context(context_type, tenant_id, organization_id);

COMMIT;
