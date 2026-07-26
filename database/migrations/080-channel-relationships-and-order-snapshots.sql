-- Channel ecosystem V1.3.2: temporal relationships and immutable order rule snapshots.

BEGIN;

CREATE TABLE IF NOT EXISTS xz_channel_relationship_history (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  subject_user_id TEXT NOT NULL REFERENCES xz_users(id),
  direct_agent_user_id TEXT,
  operation_center_user_id TEXT,
  agent_depth INT NOT NULL DEFAULT 0 CHECK (agent_depth >= 0),
  agent_path JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(agent_path) = 'array'),
  source_type TEXT NOT NULL DEFAULT 'SYSTEM',
  source_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'ENDED', 'REVOKED')),
  effective_from TIMESTAMPTZ NOT NULL,
  effective_to TIMESTAMPTZ,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (effective_to IS NULL OR effective_to > effective_from),
  CHECK (direct_agent_user_id IS NULL OR direct_agent_user_id <> subject_user_id),
  CHECK (operation_center_user_id IS NULL OR operation_center_user_id <> subject_user_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_channel_relationship_history_current
  ON xz_channel_relationship_history(tenant_id, subject_user_id)
  WHERE status = 'ACTIVE' AND effective_to IS NULL;
CREATE INDEX IF NOT EXISTS idx_xz_channel_relationship_history_effective
  ON xz_channel_relationship_history(tenant_id, subject_user_id, effective_from DESC, effective_to);
CREATE INDEX IF NOT EXISTS idx_xz_channel_relationship_history_direct_agent
  ON xz_channel_relationship_history(tenant_id, direct_agent_user_id, status);
CREATE INDEX IF NOT EXISTS idx_xz_channel_relationship_history_operation_center
  ON xz_channel_relationship_history(tenant_id, operation_center_user_id, status);

CREATE TABLE IF NOT EXISTS xz_commercial_order_rule_snapshots (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  order_id TEXT NOT NULL REFERENCES xz_orders(id),
  order_no TEXT NOT NULL,
  source_user_id TEXT NOT NULL REFERENCES xz_users(id),
  plan_id TEXT NOT NULL,
  plan_version_id TEXT NOT NULL REFERENCES xz_commercial_plan_versions(id),
  rule_set_id TEXT NOT NULL REFERENCES xz_commercial_rule_sets(id),
  rule_set_version INT NOT NULL CHECK (rule_set_version > 0),
  scenario_code TEXT NOT NULL CHECK (scenario_code IN (
    'MEMBER_PURCHASE', 'AGENT_JOIN', 'OPERATION_CENTER_SERVICE'
  )),
  paid_amount_cents BIGINT NOT NULL CHECK (paid_amount_cents > 0),
  token_rights_value_cents BIGINT NOT NULL DEFAULT 0 CHECK (token_rights_value_cents >= 0),
  token_grant_amount BIGINT NOT NULL DEFAULT 0 CHECK (token_grant_amount >= 0),
  direct_agent_user_id TEXT,
  operation_center_user_id TEXT,
  business_time TIMESTAMPTZ NOT NULL,
  relationship_snapshot JSONB NOT NULL,
  commission_rule_snapshot JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (token_rights_value_cents <= paid_amount_cents),
  UNIQUE (order_id),
  UNIQUE (tenant_id, order_no)
);

CREATE INDEX IF NOT EXISTS idx_xz_commercial_order_rule_snapshots_rule_set
  ON xz_commercial_order_rule_snapshots(tenant_id, rule_set_id, business_time DESC);

CREATE OR REPLACE FUNCTION xz_protect_commercial_order_rule_snapshot()
RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'commercial order rule snapshots are immutable';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_commercial_order_rule_snapshot_immutable ON xz_commercial_order_rule_snapshots;
CREATE TRIGGER trg_xz_commercial_order_rule_snapshot_immutable
BEFORE UPDATE OR DELETE ON xz_commercial_order_rule_snapshots
FOR EACH ROW EXECUTE FUNCTION xz_protect_commercial_order_rule_snapshot();

COMMIT;
