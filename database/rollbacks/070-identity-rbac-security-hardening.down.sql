BEGIN;

DELETE FROM xz_user_roles current
WHERE current.role IN ('AGENT','OPERATION')
  AND NOT EXISTS(
    SELECT 1 FROM xz_migration_070_user_roles_backup backup
    WHERE backup.user_id=current.user_id AND backup.tenant_id=current.tenant_id
      AND backup.organization_id=current.organization_id AND backup.role=current.role
  );

INSERT INTO xz_user_roles(user_id,tenant_id,organization_id,role,status,assigned_at,updated_at)
SELECT user_id,tenant_id,organization_id,role,status,assigned_at,updated_at
FROM xz_migration_070_user_roles_backup
ON CONFLICT(user_id,tenant_id,organization_id,role)
DO UPDATE SET status=excluded.status,assigned_at=excluded.assigned_at,updated_at=excluded.updated_at;

UPDATE xz_user_role_context current
SET tenant_id=backup.tenant_id,organization_id=backup.organization_id,
    current_role_code=backup.current_role_code,context_type=backup.context_type,updated_at=backup.updated_at
FROM xz_migration_070_role_context_backup backup
WHERE current.user_id=backup.user_id;

DROP TABLE xz_migration_070_role_context_backup;
DROP TABLE xz_migration_070_user_roles_backup;

COMMIT;
