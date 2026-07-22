-- User business identity foundation.
-- This migration is additive. Legacy xz_users role and identity projection fields remain intact.

BEGIN;

CREATE TABLE IF NOT EXISTS xz_user_business_identities (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL DEFAULT 'tenant_default' REFERENCES xz_tenants(id),
  user_id TEXT NOT NULL REFERENCES xz_users(id),
  identity_type TEXT NOT NULL CHECK (identity_type IN ('USER', 'AGENT', 'OPERATION_CENTER')),
  identity_status TEXT NOT NULL CHECK (identity_status IN ('PENDING', 'ACTIVE', 'FROZEN', 'TERMINATED')),
  commission_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  source_type TEXT NOT NULL DEFAULT 'MIGRATION',
  source_order_id TEXT,
  effective_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ,
  ended_at TIMESTAMPTZ,
  status_reason TEXT NOT NULL DEFAULT '',
  identity_version BIGINT NOT NULL DEFAULT 1 CHECK (identity_version > 0),
  created_by TEXT NOT NULL DEFAULT 'system',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (expires_at IS NULL OR expires_at > effective_at),
  CHECK (ended_at IS NULL OR ended_at >= effective_at),
  CHECK (identity_status <> 'TERMINATED' OR ended_at IS NOT NULL),
  CHECK (identity_type <> 'USER' OR commission_enabled = FALSE)
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_user_business_identity_current_type
  ON xz_user_business_identities(tenant_id, user_id, identity_type)
  WHERE identity_status IN ('PENDING', 'ACTIVE', 'FROZEN');

-- A user may retain historical AGENT rows after upgrading, but cannot have a
-- current AGENT and OPERATION_CENTER business identity at the same time.
CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_user_business_identity_current_channel
  ON xz_user_business_identities(tenant_id, user_id)
  WHERE identity_type IN ('AGENT', 'OPERATION_CENTER')
    AND identity_status IN ('PENDING', 'ACTIVE', 'FROZEN');

CREATE INDEX IF NOT EXISTS idx_xz_user_business_identities_user_history
  ON xz_user_business_identities(tenant_id, user_id, effective_at DESC, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_user_relationships (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL DEFAULT 'tenant_default' REFERENCES xz_tenants(id),
  user_id TEXT NOT NULL REFERENCES xz_users(id),
  parent_agent_id TEXT REFERENCES xz_channel_agents(id),
  operation_center_id TEXT REFERENCES xz_operation_centers(id),
  effective_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ended_at TIMESTAMPTZ,
  status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'ENDED', 'CANCELLED')),
  source_type TEXT NOT NULL DEFAULT 'MIGRATION',
  created_by TEXT NOT NULL DEFAULT 'system',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (parent_agent_id IS NOT NULL OR operation_center_id IS NOT NULL),
  CHECK (ended_at IS NULL OR ended_at >= effective_at),
  CHECK ((status = 'ACTIVE' AND ended_at IS NULL) OR status <> 'ACTIVE')
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_user_relationships_current
  ON xz_user_relationships(tenant_id, user_id)
  WHERE status = 'ACTIVE' AND ended_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_xz_user_relationships_history
  ON xz_user_relationships(tenant_id, user_id, effective_at DESC, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_user_relationships_parent
  ON xz_user_relationships(tenant_id, parent_agent_id, status, effective_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_user_relationships_operation_center
  ON xz_user_relationships(tenant_id, operation_center_id, status, effective_at DESC);

CREATE TABLE IF NOT EXISTS xz_identity_change_records (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL DEFAULT 'tenant_default' REFERENCES xz_tenants(id),
  user_id TEXT NOT NULL REFERENCES xz_users(id),
  old_identity JSONB,
  new_identity JSONB,
  change_type TEXT NOT NULL,
  source_type TEXT NOT NULL DEFAULT 'ADMIN',
  source_order_id TEXT,
  old_parent_agent_id TEXT,
  new_parent_agent_id TEXT,
  old_operation_center_id TEXT,
  new_operation_center_id TEXT,
  reason TEXT NOT NULL DEFAULT '',
  remark TEXT NOT NULL DEFAULT '',
  operator_id TEXT NOT NULL,
  request_id TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_identity_change_records_idempotency
  ON xz_identity_change_records(tenant_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';
CREATE INDEX IF NOT EXISTS idx_xz_identity_change_records_user_history
  ON xz_identity_change_records(tenant_id, user_id, created_at DESC, id DESC);

CREATE OR REPLACE FUNCTION xz_validate_user_relationship()
RETURNS trigger AS $$
DECLARE
  parent_user_id TEXT;
  cycle_found BOOLEAN;
BEGIN
  IF NEW.parent_agent_id IS NOT NULL THEN
    SELECT user_id INTO parent_user_id
    FROM xz_channel_agents
    WHERE id = NEW.parent_agent_id;

    IF parent_user_id IS NULL THEN
      RAISE EXCEPTION 'parent agent not found: %', NEW.parent_agent_id;
    END IF;
    IF parent_user_id = NEW.user_id THEN
      RAISE EXCEPTION 'user cannot be their own parent agent';
    END IF;

    WITH RECURSIVE ancestors(user_id) AS (
      SELECT parent_user_id
      UNION
      SELECT next_agent.user_id
      FROM ancestors
      JOIN xz_user_relationships relation
        ON relation.tenant_id = NEW.tenant_id
       AND relation.user_id = ancestors.user_id
       AND relation.status = 'ACTIVE'
       AND relation.ended_at IS NULL
      JOIN xz_channel_agents next_agent ON next_agent.id = relation.parent_agent_id
    )
    SELECT EXISTS(SELECT 1 FROM ancestors WHERE user_id = NEW.user_id)
      INTO cycle_found;

    IF cycle_found THEN
      RAISE EXCEPTION 'circular agent relationship is not allowed';
    END IF;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_validate_user_relationship ON xz_user_relationships;
CREATE TRIGGER trg_xz_validate_user_relationship
BEFORE INSERT OR UPDATE ON xz_user_relationships
FOR EACH ROW EXECUTE FUNCTION xz_validate_user_relationship();

CREATE OR REPLACE FUNCTION xz_protect_user_relationship_history()
RETURNS trigger AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'user relationship history cannot be deleted';
  END IF;
  IF OLD.user_id IS DISTINCT FROM NEW.user_id
     OR OLD.tenant_id IS DISTINCT FROM NEW.tenant_id
     OR OLD.parent_agent_id IS DISTINCT FROM NEW.parent_agent_id
     OR OLD.operation_center_id IS DISTINCT FROM NEW.operation_center_id
     OR OLD.effective_at IS DISTINCT FROM NEW.effective_at THEN
    RAISE EXCEPTION 'relationship assignment cannot be overwritten; end it and create a new record';
  END IF;
  IF OLD.status <> 'ACTIVE' THEN
    RAISE EXCEPTION 'ended relationship history cannot be changed';
  END IF;
  IF NEW.status = 'ACTIVE' OR NEW.ended_at IS NULL THEN
    RAISE EXCEPTION 'relationship update must end the active record';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_protect_user_relationship_history ON xz_user_relationships;
CREATE TRIGGER trg_xz_protect_user_relationship_history
BEFORE UPDATE OR DELETE ON xz_user_relationships
FOR EACH ROW EXECUTE FUNCTION xz_protect_user_relationship_history();

CREATE OR REPLACE FUNCTION xz_protect_identity_change_record()
RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'identity change records are append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_protect_identity_change_record ON xz_identity_change_records;
CREATE TRIGGER trg_xz_protect_identity_change_record
BEFORE UPDATE OR DELETE ON xz_identity_change_records
FOR EACH ROW EXECUTE FUNCTION xz_protect_identity_change_record();

-- Base USER identity is independent from membership and partner identities.
INSERT INTO xz_user_business_identities (
  id, tenant_id, user_id, identity_type, identity_status, commission_enabled,
  source_type, effective_at, created_by
)
SELECT
  'business_identity_user_' || substr(md5(user_record.id), 1, 20),
  'tenant_default', user_record.id, 'USER', 'ACTIVE', FALSE,
  'MIGRATION', coalesce(nullif(user_record.created_at, '')::timestamptz, now()), 'migration:066'
FROM xz_users user_record
ON CONFLICT DO NOTHING;

-- Operation center wins when legacy data contains both current channel identities.
INSERT INTO xz_user_business_identities (
  id, tenant_id, user_id, identity_type, identity_status, commission_enabled,
  source_type, source_order_id, effective_at, ended_at, status_reason, created_by
)
SELECT
  'business_identity_operation_' || substr(md5(center.id), 1, 20),
  'tenant_default', center.user_id, 'OPERATION_CENTER',
  CASE WHEN upper(coalesce(center.status, '')) = 'ACTIVE' THEN 'ACTIVE' ELSE 'TERMINATED' END,
  upper(coalesce(center.status, '')) = 'ACTIVE', 'MIGRATION', nullif(center.join_order_id, ''),
  coalesce(nullif(center.approved_at, '')::timestamptz, nullif(center.created_at, '')::timestamptz, now()),
  CASE WHEN upper(coalesce(center.status, '')) = 'ACTIVE' THEN NULL ELSE coalesce(nullif(center.updated_at, '')::timestamptz, now()) END,
  'legacy_operation_center_projection', 'migration:066'
FROM xz_operation_centers center
ON CONFLICT DO NOTHING;

INSERT INTO xz_user_business_identities (
  id, tenant_id, user_id, identity_type, identity_status, commission_enabled,
  source_type, source_order_id, effective_at, ended_at, status_reason, created_by
)
SELECT
  'business_identity_agent_' || substr(md5(agent.id), 1, 20),
  'tenant_default', agent.user_id, 'AGENT',
  CASE
    WHEN EXISTS (
      SELECT 1 FROM xz_operation_centers center
      WHERE center.user_id = agent.user_id AND upper(coalesce(center.status, '')) = 'ACTIVE'
    ) THEN 'TERMINATED'
    WHEN upper(coalesce(agent.status, 'ACTIVE')) = 'ACTIVE' THEN 'ACTIVE'
    ELSE 'TERMINATED'
  END,
  CASE
    WHEN EXISTS (
      SELECT 1 FROM xz_operation_centers center
      WHERE center.user_id = agent.user_id AND upper(coalesce(center.status, '')) = 'ACTIVE'
    ) THEN FALSE
    ELSE upper(coalesce(agent.status, 'ACTIVE')) = 'ACTIVE'
  END,
  'MIGRATION', nullif(agent.join_order_id, ''),
  coalesce(nullif(agent.created_at, '')::timestamptz, now()),
  CASE
    WHEN upper(coalesce(agent.status, 'ACTIVE')) <> 'ACTIVE'
      OR EXISTS (
        SELECT 1 FROM xz_operation_centers center
        WHERE center.user_id = agent.user_id AND upper(coalesce(center.status, '')) = 'ACTIVE'
      )
    THEN coalesce(nullif(agent.updated_at, '')::timestamptz, now())
    ELSE NULL
  END,
  CASE
    WHEN EXISTS (
      SELECT 1 FROM xz_operation_centers center
      WHERE center.user_id = agent.user_id AND upper(coalesce(center.status, '')) = 'ACTIVE'
    ) THEN 'upgraded_to_operation_center'
    ELSE 'legacy_agent_projection'
  END,
  'migration:066'
FROM xz_channel_agents agent
ON CONFLICT DO NOTHING;

-- Existing agent hierarchy and customer referral attribution become effective
-- from their original creation time. Historic order snapshots are untouched.
INSERT INTO xz_user_relationships (
  id, tenant_id, user_id, parent_agent_id, operation_center_id,
  effective_at, status, source_type, created_by
)
SELECT
  'user_relationship_agent_' || substr(md5(agent.id), 1, 20),
  'tenant_default', agent.user_id, nullif(agent.parent_id, ''), nullif(agent.operation_center_id, ''),
  coalesce(nullif(agent.created_at, '')::timestamptz, now()), 'ACTIVE', 'MIGRATION', 'migration:066'
FROM xz_channel_agents agent
WHERE (nullif(agent.parent_id, '') IS NOT NULL OR nullif(agent.operation_center_id, '') IS NOT NULL)
  AND upper(coalesce(agent.status, 'ACTIVE')) = 'ACTIVE'
ON CONFLICT DO NOTHING;

INSERT INTO xz_user_relationships (
  id, tenant_id, user_id, parent_agent_id, operation_center_id,
  effective_at, status, source_type, created_by
)
SELECT
  'user_relationship_referral_' || substr(md5(user_record.id), 1, 20),
  'tenant_default', user_record.id, source_agent.id, nullif(source_agent.operation_center_id, ''),
  coalesce(nullif(user_record.created_at, '')::timestamptz, now()), 'ACTIVE', 'MIGRATION', 'migration:066'
FROM xz_users user_record
JOIN xz_channel_agents source_agent ON source_agent.user_id = user_record.referred_by
WHERE nullif(user_record.referred_by, '') IS NOT NULL
  AND upper(coalesce(source_agent.status, 'ACTIVE')) = 'ACTIVE'
  AND NOT EXISTS (
    SELECT 1 FROM xz_user_relationships relation
    WHERE relation.tenant_id = 'tenant_default'
      AND relation.user_id = user_record.id
      AND relation.status = 'ACTIVE'
      AND relation.ended_at IS NULL
  )
ON CONFLICT DO NOTHING;

INSERT INTO xz_identity_change_records (
  id, tenant_id, user_id, new_identity, change_type, source_type,
  source_order_id, reason, operator_id, idempotency_key, created_at
)
SELECT
  'identity_change_migration_' || substr(md5(identity_record.id), 1, 20),
  identity_record.tenant_id, identity_record.user_id,
  jsonb_build_object(
    'id', identity_record.id,
    'identityType', identity_record.identity_type,
    'identityStatus', identity_record.identity_status,
    'commissionEnabled', identity_record.commission_enabled,
    'effectiveAt', identity_record.effective_at,
    'endedAt', identity_record.ended_at
  ),
  'MIGRATED', 'MIGRATION', identity_record.source_order_id,
  identity_record.status_reason, 'migration:066',
  'migration:066:identity:' || identity_record.id, identity_record.created_at
FROM xz_user_business_identities identity_record
ON CONFLICT DO NOTHING;

INSERT INTO xz_identity_change_records (
  id, tenant_id, user_id, change_type, source_type,
  new_parent_agent_id, new_operation_center_id, reason,
  operator_id, idempotency_key, created_at
)
SELECT
  'identity_change_relationship_' || substr(md5(relation.id), 1, 20),
  relation.tenant_id, relation.user_id, 'RELATION_MIGRATED', 'MIGRATION',
  relation.parent_agent_id, relation.operation_center_id,
  'legacy_relationship_projection', 'migration:066',
  'migration:066:relationship:' || relation.id, relation.created_at
FROM xz_user_relationships relation
ON CONFLICT DO NOTHING;

-- OPERATION is a distinct business identity but inherits the agent workbench permissions.
CREATE TABLE IF NOT EXISTS xz_migration_066_added_role_permissions (
  role TEXT NOT NULL,
  permission TEXT NOT NULL,
  PRIMARY KEY (role, permission)
);

INSERT INTO xz_migration_066_added_role_permissions(role, permission)
SELECT 'OPERATION', agent_permission.permission
FROM xz_role_permissions agent_permission
WHERE agent_permission.role = 'AGENT'
  AND NOT EXISTS (
    SELECT 1 FROM xz_role_permissions existing
    WHERE existing.role = 'OPERATION'
      AND existing.permission = agent_permission.permission
  )
ON CONFLICT DO NOTHING;

INSERT INTO xz_role_permissions(role, permission)
SELECT role, permission
FROM xz_migration_066_added_role_permissions
ON CONFLICT (role, permission) DO NOTHING;

COMMIT;
