-- Channel ecosystem V1.3.2: versioned commercial rule sets and plan snapshots.
-- This migration is additive. No rule is published and no runtime path is switched.

BEGIN;

CREATE TABLE IF NOT EXISTS xz_commercial_rule_sets (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  rule_code TEXT NOT NULL,
  version INT NOT NULL CHECK (version > 0),
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'PUBLISHED', 'RETIRED', 'ARCHIVED')),
  effective_start_at TIMESTAMPTZ NOT NULL,
  effective_end_at TIMESTAMPTZ,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_by TEXT NOT NULL DEFAULT '',
  published_by TEXT NOT NULL DEFAULT '',
  published_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (effective_end_at IS NULL OR effective_end_at > effective_start_at),
  CHECK ((status = 'PUBLISHED' AND published_at IS NOT NULL) OR status <> 'PUBLISHED'),
  UNIQUE (tenant_id, rule_code, version)
);

CREATE INDEX IF NOT EXISTS idx_xz_commercial_rule_sets_effective
  ON xz_commercial_rule_sets(tenant_id, status, effective_start_at, effective_end_at, version DESC);

CREATE TABLE IF NOT EXISTS xz_commercial_plan_versions (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  rule_set_id TEXT NOT NULL REFERENCES xz_commercial_rule_sets(id),
  plan_id TEXT NOT NULL,
  version INT NOT NULL CHECK (version > 0),
  identity_type TEXT NOT NULL CHECK (identity_type IN ('MEMBER', 'AGENT', 'OPERATION_CENTER')),
  price_cents BIGINT NOT NULL CHECK (price_cents > 0),
  currency CHAR(3) NOT NULL DEFAULT 'CNY',
  token_grant_amount BIGINT NOT NULL DEFAULT 0 CHECK (token_grant_amount >= 0),
  token_rights_value_cents BIGINT NOT NULL DEFAULT 0 CHECK (token_rights_value_cents >= 0),
  duration_days INT NOT NULL DEFAULT 0 CHECK (duration_days >= 0),
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (token_rights_value_cents <= price_cents),
  UNIQUE (rule_set_id, plan_id),
  UNIQUE (tenant_id, plan_id, version)
);

CREATE INDEX IF NOT EXISTS idx_xz_commercial_plan_versions_lookup
  ON xz_commercial_plan_versions(tenant_id, plan_id, rule_set_id);

ALTER TABLE xz_commission_rules
  ADD COLUMN IF NOT EXISTS commercial_rule_set_id TEXT,
  ADD COLUMN IF NOT EXISTS commercial_scenario_code TEXT;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_xz_commission_rules_commercial_rule_set'
  ) THEN
    ALTER TABLE xz_commission_rules
      ADD CONSTRAINT fk_xz_commission_rules_commercial_rule_set
      FOREIGN KEY (commercial_rule_set_id) REFERENCES xz_commercial_rule_sets(id) NOT VALID;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'ck_xz_commission_rules_commercial_scenario'
  ) THEN
    ALTER TABLE xz_commission_rules
      ADD CONSTRAINT ck_xz_commission_rules_commercial_scenario
      CHECK (commercial_scenario_code IS NULL OR commercial_scenario_code IN (
        'MEMBER_PURCHASE', 'AGENT_JOIN', 'OPERATION_CENTER_SERVICE'
      )) NOT VALID;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'ck_xz_commission_rules_no_ancestor_agent'
  ) THEN
    ALTER TABLE xz_commission_rules
      ADD CONSTRAINT ck_xz_commission_rules_no_ancestor_agent
      CHECK (
        commercial_rule_set_id IS NULL OR beneficiary_role <> 'AGENT' OR relationship_level = 1
      ) NOT VALID;
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_xz_commission_rules_commercial_lookup
  ON xz_commission_rules(commercial_rule_set_id, commercial_scenario_code, status, priority);

COMMIT;
