-- Rollback for 066-user-business-identity-foundation.sql.
-- Existing user, role, order, wallet and commission data is not modified.

BEGIN;

DO $$
BEGIN
  IF to_regclass('public.xz_migration_066_added_role_permissions') IS NOT NULL THEN
    DELETE FROM xz_role_permissions role_permission
    USING xz_migration_066_added_role_permissions added
    WHERE role_permission.role = added.role
      AND role_permission.permission = added.permission;
  END IF;
END;
$$;

DROP TABLE IF EXISTS xz_migration_066_added_role_permissions;

DROP TRIGGER IF EXISTS trg_xz_protect_identity_change_record ON xz_identity_change_records;
DROP TRIGGER IF EXISTS trg_xz_protect_user_relationship_history ON xz_user_relationships;
DROP TRIGGER IF EXISTS trg_xz_validate_user_relationship ON xz_user_relationships;

DROP FUNCTION IF EXISTS xz_protect_identity_change_record();
DROP FUNCTION IF EXISTS xz_protect_user_relationship_history();
DROP FUNCTION IF EXISTS xz_validate_user_relationship();

DROP TABLE IF EXISTS xz_identity_change_records;
DROP TABLE IF EXISTS xz_user_relationships;
DROP TABLE IF EXISTS xz_user_business_identities;

COMMIT;
