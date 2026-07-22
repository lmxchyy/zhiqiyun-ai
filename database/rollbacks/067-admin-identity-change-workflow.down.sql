BEGIN;

DELETE FROM xz_role_permissions permission
USING xz_migration_067_added_role_permissions marker
WHERE permission.role=marker.role AND permission.permission=marker.permission;

DROP TABLE IF EXISTS xz_migration_067_added_role_permissions;
DROP TRIGGER IF EXISTS trg_xz_identity_change_approvals_immutable ON xz_identity_change_approvals;
DROP FUNCTION IF EXISTS xz_protect_identity_change_workflow_history();
DROP TABLE IF EXISTS xz_identity_change_executions;
DROP TABLE IF EXISTS xz_identity_change_approvals;
DROP TABLE IF EXISTS xz_identity_change_previews;

COMMIT;
