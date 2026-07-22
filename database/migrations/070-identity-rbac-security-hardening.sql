-- P0 identity/RBAC consistency repair. Commercial roles are projections of
-- active business identities and valid profiles, never of xz_users.role.
BEGIN;

CREATE TABLE IF NOT EXISTS xz_migration_070_user_roles_backup AS
SELECT * FROM xz_user_roles WHERE role IN ('AGENT', 'OPERATION');

CREATE TABLE IF NOT EXISTS xz_migration_070_role_context_backup AS
SELECT context.* FROM xz_user_role_context context
WHERE context.current_role_code IN ('AGENT', 'OPERATION');

WITH scope AS (
  SELECT user_id, tenant_id, organization_id
  FROM xz_user_roles
  WHERE role='USER' AND upper(status)='ACTIVE'
), expected_agent AS (
  SELECT DISTINCT identity.user_id
  FROM xz_user_business_identities identity
  JOIN xz_users users ON users.id=identity.user_id AND upper(coalesce(users.status,''))='ACTIVE'
  JOIN xz_channel_agents profile ON profile.user_id=identity.user_id AND upper(coalesce(profile.status,''))='ACTIVE'
  WHERE identity.identity_type='AGENT' AND identity.identity_status='ACTIVE'
    AND identity.ended_at IS NULL AND identity.effective_at<=clock_timestamp()
    AND (identity.expires_at IS NULL OR identity.expires_at>clock_timestamp())
)
INSERT INTO xz_user_roles(user_id,tenant_id,organization_id,role,status)
SELECT expected.user_id,scope.tenant_id,scope.organization_id,'AGENT','ACTIVE'
FROM expected_agent expected JOIN scope ON scope.user_id=expected.user_id
ON CONFLICT(user_id,tenant_id,organization_id,role)
DO UPDATE SET status='ACTIVE',updated_at=now();

WITH scope AS (
  SELECT user_id, tenant_id, organization_id
  FROM xz_user_roles
  WHERE role='USER' AND upper(status)='ACTIVE'
), expected_operation AS (
  SELECT DISTINCT identity.user_id
  FROM xz_user_business_identities identity
  JOIN xz_users users ON users.id=identity.user_id AND upper(coalesce(users.status,''))='ACTIVE'
  JOIN xz_operation_centers profile ON profile.user_id=identity.user_id AND upper(coalesce(profile.status,''))='ACTIVE'
  WHERE identity.identity_type='OPERATION_CENTER' AND identity.identity_status='ACTIVE'
    AND identity.ended_at IS NULL AND identity.effective_at<=clock_timestamp()
    AND (identity.expires_at IS NULL OR identity.expires_at>clock_timestamp())
)
INSERT INTO xz_user_roles(user_id,tenant_id,organization_id,role,status)
SELECT expected.user_id,scope.tenant_id,scope.organization_id,'OPERATION','ACTIVE'
FROM expected_operation expected JOIN scope ON scope.user_id=expected.user_id
ON CONFLICT(user_id,tenant_id,organization_id,role)
DO UPDATE SET status='ACTIVE',updated_at=now();

UPDATE xz_user_roles binding
SET status=CASE WHEN EXISTS(
  SELECT 1 FROM xz_user_business_identities identity
  JOIN xz_users users ON users.id=identity.user_id AND upper(coalesce(users.status,''))='ACTIVE'
  JOIN xz_channel_agents profile ON profile.user_id=identity.user_id AND upper(coalesce(profile.status,''))='ACTIVE'
  WHERE identity.user_id=binding.user_id AND identity.identity_type='AGENT'
    AND identity.identity_status='ACTIVE' AND identity.ended_at IS NULL
    AND identity.effective_at<=clock_timestamp() AND (identity.expires_at IS NULL OR identity.expires_at>clock_timestamp())
) THEN 'ACTIVE' ELSE 'INACTIVE' END,updated_at=now()
WHERE binding.role='AGENT';

UPDATE xz_user_roles binding
SET status=CASE WHEN EXISTS(
  SELECT 1 FROM xz_user_business_identities identity
  JOIN xz_users users ON users.id=identity.user_id AND upper(coalesce(users.status,''))='ACTIVE'
  JOIN xz_operation_centers profile ON profile.user_id=identity.user_id AND upper(coalesce(profile.status,''))='ACTIVE'
  WHERE identity.user_id=binding.user_id AND identity.identity_type='OPERATION_CENTER'
    AND identity.identity_status='ACTIVE' AND identity.ended_at IS NULL
    AND identity.effective_at<=clock_timestamp() AND (identity.expires_at IS NULL OR identity.expires_at>clock_timestamp())
) THEN 'ACTIVE' ELSE 'INACTIVE' END,updated_at=now()
WHERE binding.role='OPERATION';

UPDATE xz_user_role_context context
SET tenant_id='tenant_default',
    organization_id='organization_default_' || substr(md5('tenant_default'),1,16),
    current_role_code='USER',context_type='PERSONAL',updated_at=now()
WHERE NOT EXISTS(
  SELECT 1 FROM xz_user_roles binding
  WHERE binding.user_id=context.user_id AND binding.tenant_id=context.tenant_id
    AND binding.organization_id=context.organization_id AND binding.role=context.current_role_code
    AND upper(binding.status)='ACTIVE'
);

COMMIT;
