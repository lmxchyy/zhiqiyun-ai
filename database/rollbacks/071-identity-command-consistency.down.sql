BEGIN;

DELETE FROM xz_role_permissions permission
USING xz_migration_071_added_permissions added
WHERE permission.role=added.role AND permission.permission=added.permission;
INSERT INTO xz_role_permissions(role,permission)
SELECT role,permission FROM xz_migration_071_removed_operation_permissions
ON CONFLICT DO NOTHING;

UPDATE xz_plans plan SET entitlements=backup.entitlements
FROM xz_migration_071_plan_entitlements_backup backup WHERE plan.id=backup.id;

ALTER TABLE xz_operation_centers DROP COLUMN IF EXISTS agreement_status;
ALTER TABLE xz_operation_centers DROP COLUMN IF EXISTS settlement_profile;
ALTER TABLE xz_operation_centers DROP COLUMN IF EXISTS contact_info;
ALTER TABLE xz_operation_centers DROP COLUMN IF EXISTS responsible_person;

CREATE OR REPLACE FUNCTION xz_validate_user_relationship()
RETURNS trigger AS $$
DECLARE parent_user_id TEXT; cycle_found BOOLEAN;
BEGIN
  IF NEW.parent_agent_id IS NOT NULL THEN
    SELECT user_id INTO parent_user_id FROM xz_channel_agents WHERE id=NEW.parent_agent_id;
    IF parent_user_id IS NULL THEN RAISE EXCEPTION 'parent agent not found: %',NEW.parent_agent_id; END IF;
    IF parent_user_id=NEW.user_id THEN RAISE EXCEPTION 'user cannot be their own parent agent'; END IF;
    WITH RECURSIVE ancestors(user_id) AS (
      SELECT parent_user_id UNION
      SELECT next_agent.user_id FROM ancestors
      JOIN xz_user_relationships relation ON relation.tenant_id=NEW.tenant_id AND relation.user_id=ancestors.user_id AND relation.status='ACTIVE' AND relation.ended_at IS NULL
      JOIN xz_channel_agents next_agent ON next_agent.id=relation.parent_agent_id
    ) SELECT EXISTS(SELECT 1 FROM ancestors WHERE user_id=NEW.user_id) INTO cycle_found;
    IF cycle_found THEN RAISE EXCEPTION 'circular agent relationship is not allowed'; END IF;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TABLE IF EXISTS xz_migration_071_added_permissions;
DROP TABLE IF EXISTS xz_migration_071_removed_operation_permissions;
DROP TABLE IF EXISTS xz_migration_071_plan_entitlements_backup;

COMMIT;
