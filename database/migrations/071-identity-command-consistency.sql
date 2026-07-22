-- Phase 2 identity command safety, conversion policy and read-only consistency reporting.
BEGIN;

ALTER TABLE xz_operation_centers ADD COLUMN IF NOT EXISTS responsible_person TEXT NOT NULL DEFAULT '';
ALTER TABLE xz_operation_centers ADD COLUMN IF NOT EXISTS contact_info TEXT NOT NULL DEFAULT '';
ALTER TABLE xz_operation_centers ADD COLUMN IF NOT EXISTS settlement_profile JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE xz_operation_centers ADD COLUMN IF NOT EXISTS agreement_status TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS xz_migration_071_plan_entitlements_backup (
  id TEXT PRIMARY KEY,
  entitlements JSONB NOT NULL
);
INSERT INTO xz_migration_071_plan_entitlements_backup(id,entitlements)
SELECT id,entitlements FROM xz_plans WHERE id='plan_ai_creator_996'
ON CONFLICT DO NOTHING;

UPDATE xz_plans SET entitlements=coalesce(entitlements,'{}'::jsonb) || jsonb_build_object(
  'convertibleToAgent',true,
  'conversionTargetPlanIds',jsonb_build_array('plan_agent_join_996'),
  'conversionValuePolicy','ACTUAL_PAID',
  'conversionValidityDays',365,
  'tokenConversionPolicy',jsonb_build_array('KEEP_EXISTING','ADJUST_DIFFERENCE')
) WHERE id='plan_ai_creator_996';

CREATE TABLE IF NOT EXISTS xz_migration_071_removed_operation_permissions (
  role TEXT NOT NULL,
  permission TEXT NOT NULL,
  PRIMARY KEY(role,permission)
);
INSERT INTO xz_migration_071_removed_operation_permissions(role,permission)
SELECT permission.role,permission.permission
FROM xz_role_permissions permission
JOIN xz_migration_066_added_role_permissions inherited
  ON inherited.role=permission.role AND inherited.permission=permission.permission
WHERE permission.role='OPERATION'
ON CONFLICT DO NOTHING;
DELETE FROM xz_role_permissions permission
USING xz_migration_071_removed_operation_permissions removed
WHERE permission.role=removed.role AND permission.permission=removed.permission;

CREATE TABLE IF NOT EXISTS xz_migration_071_added_permissions (
  role TEXT NOT NULL,
  permission TEXT NOT NULL,
  PRIMARY KEY(role,permission)
);
WITH desired(role,permission) AS (
  VALUES
    ('SUPER_ADMIN','identity:change:special-price'),
    ('SUPER_ADMIN','identity:consistency:view'),
    ('ADMIN','identity:consistency:view'),
    ('SUPER_ADMIN','identity:operation-profile:update'),
    ('ADMIN','identity:operation-profile:update')
), missing AS (
  SELECT desired.* FROM desired LEFT JOIN xz_role_permissions existing
    ON existing.role=desired.role AND existing.permission=desired.permission
  WHERE existing.role IS NULL
)
INSERT INTO xz_migration_071_added_permissions SELECT * FROM missing ON CONFLICT DO NOTHING;
INSERT INTO xz_role_permissions(role,permission)
SELECT role,permission FROM xz_migration_071_added_permissions ON CONFLICT DO NOTHING;

CREATE OR REPLACE FUNCTION xz_validate_user_relationship()
RETURNS trigger AS $$
DECLARE
  parent_user_id TEXT;
  center_user_id TEXT;
  derived_center_id TEXT;
  cycle_found BOOLEAN;
BEGIN
  IF NEW.parent_agent_id IS NOT NULL THEN
    SELECT agent.user_id,nullif(coalesce(parent_relation.operation_center_id,agent.operation_center_id),'')
      INTO parent_user_id,derived_center_id
    FROM xz_channel_agents agent
    JOIN xz_users account ON account.id=agent.user_id AND upper(coalesce(account.status,''))='ACTIVE'
    JOIN xz_user_business_identities identity
      ON identity.user_id=agent.user_id AND identity.identity_type='AGENT'
     AND identity.identity_status='ACTIVE' AND identity.ended_at IS NULL
     AND identity.effective_at<=clock_timestamp()
     AND (identity.expires_at IS NULL OR identity.expires_at>clock_timestamp())
    LEFT JOIN xz_user_relationships parent_relation
      ON parent_relation.user_id=agent.user_id AND parent_relation.status='ACTIVE' AND parent_relation.ended_at IS NULL
    WHERE agent.id=NEW.parent_agent_id AND upper(coalesce(agent.status,''))='ACTIVE';
    IF parent_user_id IS NULL THEN RAISE EXCEPTION 'parent agent must have active identity and profile'; END IF;
    IF parent_user_id=NEW.user_id THEN RAISE EXCEPTION 'user cannot be their own parent agent'; END IF;
    IF NEW.operation_center_id IS DISTINCT FROM derived_center_id THEN
      RAISE EXCEPTION 'operation center must be derived from parent agent';
    END IF;
    WITH RECURSIVE ancestors(user_id) AS (
      SELECT parent_user_id
      UNION
      SELECT parent.user_id FROM ancestors child
      JOIN xz_user_relationships relation ON relation.user_id=child.user_id AND relation.status='ACTIVE' AND relation.ended_at IS NULL
      JOIN xz_channel_agents parent ON parent.id=relation.parent_agent_id
    ) SELECT EXISTS(SELECT 1 FROM ancestors WHERE user_id=NEW.user_id) INTO cycle_found;
    IF cycle_found THEN RAISE EXCEPTION 'circular agent relationship is not allowed'; END IF;
  END IF;
  IF NEW.operation_center_id IS NOT NULL THEN
    SELECT center.user_id INTO center_user_id
    FROM xz_operation_centers center
    JOIN xz_users account ON account.id=center.user_id AND upper(coalesce(account.status,''))='ACTIVE'
    JOIN xz_user_business_identities identity
      ON identity.user_id=center.user_id AND identity.identity_type='OPERATION_CENTER'
     AND identity.identity_status='ACTIVE' AND identity.ended_at IS NULL
     AND identity.effective_at<=clock_timestamp()
     AND (identity.expires_at IS NULL OR identity.expires_at>clock_timestamp())
    WHERE center.id=NEW.operation_center_id AND upper(coalesce(center.status,''))='ACTIVE';
    IF center_user_id IS NULL THEN RAISE EXCEPTION 'operation center must have active identity and profile'; END IF;
    IF center_user_id=NEW.user_id THEN RAISE EXCEPTION 'user cannot belong to their own operation center'; END IF;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMIT;
