BEGIN;

DELETE FROM xz_role_permissions permission
USING xz_migration_068_added_role_permissions added
WHERE permission.role=added.role AND permission.permission=added.permission;

DROP TABLE IF EXISTS xz_migration_068_added_role_permissions;
DROP TABLE IF EXISTS xz_identity_downgrade_requests;
DROP TABLE IF EXISTS xz_identity_downgrade_previews;

COMMIT;
