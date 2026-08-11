-- Channel ecosystem V1.3.2: immutable per-order engine decisions and single write-source guard.
-- Default rollout remains SHADOW; this migration does not enable real V1.3.2 settlement.

BEGIN;

ALTER TABLE xz_channel_rollout_configs
  ADD COLUMN IF NOT EXISTS allow_plan_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS percentage_rollout_enabled BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE xz_channel_rollout_configs SET mode = CASE mode
  WHEN 'V132_CANARY' THEN 'CANARY'
  WHEN 'V132_FULL' THEN 'V132'
  ELSE mode
END;

DO $$
DECLARE item RECORD;
BEGIN
  FOR item IN
    SELECT conname FROM pg_constraint
    WHERE conrelid='xz_channel_rollout_configs'::regclass AND contype='c'
      AND pg_get_constraintdef(oid) ILIKE '%mode%'
  LOOP
    EXECUTE format('ALTER TABLE xz_channel_rollout_configs DROP CONSTRAINT %I', item.conname);
  END LOOP;
END $$;

ALTER TABLE xz_channel_rollout_configs
  ADD CONSTRAINT ck_xz_channel_rollout_mode
    CHECK (mode IN ('LEGACY','SHADOW','CANARY','V132')),
  ADD CONSTRAINT ck_xz_channel_rollout_shadow
    CHECK (mode <> 'SHADOW' OR (canary_basis_points=0 AND percentage_rollout_enabled=FALSE)),
  ADD CONSTRAINT ck_xz_channel_rollout_canary
    CHECK (mode <> 'CANARY' OR real_switch_enabled=TRUE),
  ADD CONSTRAINT ck_xz_channel_rollout_percentage
    CHECK (
      percentage_rollout_enabled=FALSE OR
      (mode='CANARY' AND canary_basis_points BETWEEN 1 AND 10000)
    ),
  ADD CONSTRAINT ck_xz_channel_rollout_allow_plan_ids
    CHECK (jsonb_typeof(allow_plan_ids)='array');

CREATE TABLE IF NOT EXISTS xz_order_settlement_engine_decisions (
  order_id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  plan_id TEXT NOT NULL,
  settlement_engine TEXT NOT NULL CHECK (settlement_engine IN ('LEGACY','V132')),
  rule_set_id TEXT REFERENCES xz_commercial_rule_sets(id),
  rule_set_version INT,
  rollout_config_version INT,
  rollout_mode TEXT NOT NULL CHECK (rollout_mode IN ('LEGACY','SHADOW','CANARY','V132')),
  hash_bucket INT NOT NULL DEFAULT -1 CHECK (hash_bucket BETWEEN -1 AND 9999),
  decision_reason TEXT NOT NULL,
  decided_at TIMESTAMPTZ NOT NULL,
  CHECK (
    (settlement_engine='LEGACY' AND rule_set_id IS NULL AND rule_set_version IS NULL) OR
    (settlement_engine='V132' AND rule_set_id IS NOT NULL AND rule_set_version > 0)
  )
);

CREATE INDEX IF NOT EXISTS idx_xz_order_settlement_engine_decisions_tenant
  ON xz_order_settlement_engine_decisions(tenant_id,settlement_engine,decided_at DESC);

CREATE TABLE IF NOT EXISTS xz_order_settlement_write_sources (
  order_id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  settlement_engine TEXT NOT NULL CHECK (settlement_engine IN ('LEGACY','V132')),
  created_at TIMESTAMPTZ NOT NULL
);

CREATE OR REPLACE FUNCTION xz_protect_order_settlement_engine()
RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'order settlement engine and write source are immutable';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_order_settlement_engine_immutable ON xz_order_settlement_engine_decisions;
CREATE TRIGGER trg_xz_order_settlement_engine_immutable
BEFORE UPDATE OR DELETE ON xz_order_settlement_engine_decisions
FOR EACH ROW EXECUTE FUNCTION xz_protect_order_settlement_engine();

DROP TRIGGER IF EXISTS trg_xz_order_settlement_write_source_immutable ON xz_order_settlement_write_sources;
CREATE TRIGGER trg_xz_order_settlement_write_source_immutable
BEFORE UPDATE OR DELETE ON xz_order_settlement_write_sources
FOR EACH ROW EXECUTE FUNCTION xz_protect_order_settlement_engine();

CREATE OR REPLACE FUNCTION xz_protect_order_settlement_snapshot()
RETURNS trigger AS $$
BEGIN
  IF OLD.price_snapshot ? 'settlementEngine' AND
     NEW.price_snapshot->>'settlementEngine' IS DISTINCT FROM OLD.price_snapshot->>'settlementEngine' THEN
    RAISE EXCEPTION 'order settlement engine snapshot cannot be changed';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_order_settlement_snapshot_immutable ON xz_orders;
CREATE TRIGGER trg_xz_order_settlement_snapshot_immutable
BEFORE UPDATE ON xz_orders
FOR EACH ROW EXECUTE FUNCTION xz_protect_order_settlement_snapshot();

COMMIT;
