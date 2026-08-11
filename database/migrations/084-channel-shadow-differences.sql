-- Channel ecosystem V1.3.2: immutable shadow comparison records.
-- Shadow records are diagnostic only and never drive settlement or correction.

BEGIN;

CREATE TABLE IF NOT EXISTS xz_commercial_shadow_differences (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  order_id TEXT NOT NULL,
  order_no TEXT NOT NULL,
  plan_id TEXT NOT NULL,
  scenario_code TEXT,
  shadow_rule_set_id TEXT,
  shadow_rule_set_version INT,
  shadow_version TEXT NOT NULL DEFAULT 'V1.3.2',
  comparison_status TEXT NOT NULL CHECK (comparison_status IN ('MATCH', 'DIFFERENT', 'ERROR')),
  legacy_result JSONB NOT NULL,
  v132_result JSONB,
  difference JSONB,
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  relationship_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (order_id, shadow_version, shadow_rule_set_id)
);

CREATE INDEX IF NOT EXISTS idx_xz_commercial_shadow_differences_status
  ON xz_commercial_shadow_differences(tenant_id, comparison_status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_commercial_shadow_differences_order
  ON xz_commercial_shadow_differences(tenant_id, order_id, created_at DESC);

CREATE OR REPLACE FUNCTION xz_protect_commercial_shadow_difference()
RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'commercial shadow differences are immutable and cannot be auto-corrected';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_commercial_shadow_difference_immutable ON xz_commercial_shadow_differences;
CREATE TRIGGER trg_xz_commercial_shadow_difference_immutable
BEFORE UPDATE OR DELETE ON xz_commercial_shadow_differences
FOR EACH ROW EXECUTE FUNCTION xz_protect_commercial_shadow_difference();

COMMIT;
