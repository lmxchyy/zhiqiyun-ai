-- Enterprise P0 safety foundation.
-- Reuses the existing tenant, membership, RBAC, wallet, payment-event and audit models.

BEGIN;

ALTER TABLE xz_tenant_wallets
  ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS total_recharge_units BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS total_bonus_units BIGINT NOT NULL DEFAULT 0;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ck_xz_tenant_wallets_non_negative') THEN
    ALTER TABLE xz_tenant_wallets
      ADD CONSTRAINT ck_xz_tenant_wallets_non_negative
      CHECK (point_balance >= 0 AND frozen_points >= 0 AND cash_balance_cents >= 0
        AND total_recharge_units >= 0 AND total_bonus_units >= 0);
  END IF;
END $$;

COMMENT ON COLUMN xz_tenant_wallets.point_balance IS
  'Available compute units. Integer minimum unit; never money or provider token count.';
COMMENT ON COLUMN xz_tenant_wallets.cash_balance_cents IS
  'Cash balance in CNY cents. Never stored as a floating point value.';

CREATE TABLE IF NOT EXISTS xz_tenant_service_states (
  tenant_id TEXT PRIMARY KEY REFERENCES xz_tenants(id),
  lifecycle_state TEXT NOT NULL DEFAULT 'ACTIVE'
    CHECK (lifecycle_state IN ('PROVISIONING','ACTIVE','PAUSED','DISABLED','TERMINATED')),
  reason TEXT NOT NULL DEFAULT '',
  state_version BIGINT NOT NULL DEFAULT 0 CHECK (state_version >= 0),
  changed_by TEXT REFERENCES xz_users(id),
  changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  status TEXT NOT NULL DEFAULT 'ACTIVE'
    CHECK (status IN ('ACTIVE','ARCHIVED')),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_xz_tenant_service_states_status
  ON xz_tenant_service_states(lifecycle_state, status, changed_at DESC, tenant_id);

INSERT INTO xz_tenant_service_states(tenant_id, lifecycle_state, reason, metadata)
SELECT tenant.id,
       CASE upper(coalesce(tenant.status, 'ACTIVE'))
         WHEN 'SUSPENDED' THEN 'PAUSED'
         WHEN 'PAUSED' THEN 'PAUSED'
         WHEN 'DISABLED' THEN 'DISABLED'
         WHEN 'TERMINATED' THEN 'TERMINATED'
         ELSE 'ACTIVE'
       END,
       'legacy tenant status backfill',
       jsonb_build_object('legacyTenantStatus', tenant.status)
FROM xz_tenants tenant
WHERE tenant.tenant_type = 'ENTERPRISE'
ON CONFLICT (tenant_id) DO NOTHING;

CREATE OR REPLACE FUNCTION xz_create_enterprise_service_state()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.tenant_type = 'ENTERPRISE' THEN
    INSERT INTO xz_tenant_service_states(tenant_id, lifecycle_state, reason)
    VALUES (NEW.id, CASE WHEN upper(coalesce(NEW.status, 'ACTIVE')) = 'ACTIVE' THEN 'ACTIVE' ELSE 'PROVISIONING' END,
            'tenant created')
    ON CONFLICT (tenant_id) DO NOTHING;
  END IF;
  RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_xz_create_enterprise_service_state ON xz_tenants;
CREATE TRIGGER trg_xz_create_enterprise_service_state
AFTER INSERT ON xz_tenants
FOR EACH ROW EXECUTE FUNCTION xz_create_enterprise_service_state();

CREATE TABLE IF NOT EXISTS xz_compute_credit_lots (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  account_id TEXT NOT NULL REFERENCES xz_tenant_wallets(tenant_id),
  source_type TEXT NOT NULL
    CHECK (source_type IN ('RECHARGE','BONUS','PACKAGE','ACTIVITY','LEGACY','REVERSAL','MANUAL')),
  original_units BIGINT NOT NULL CHECK (original_units > 0),
  remaining_units BIGINT NOT NULL CHECK (remaining_units >= 0 AND remaining_units <= original_units),
  amount_cents BIGINT NOT NULL DEFAULT 0 CHECK (amount_cents >= 0),
  reference_type TEXT NOT NULL DEFAULT '',
  reference_id TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL,
  expires_at TIMESTAMPTZ,
  status TEXT NOT NULL DEFAULT 'ACTIVE'
    CHECK (status IN ('ACTIVE','EXHAUSTED','EXPIRED','REVERSED')),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_xz_compute_credit_lots_consume
  ON xz_compute_credit_lots(tenant_id, status, expires_at, created_at, id)
  WHERE remaining_units > 0;
CREATE INDEX IF NOT EXISTS idx_xz_compute_credit_lots_reference
  ON xz_compute_credit_lots(tenant_id, reference_type, reference_id, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_compute_ledger_entries (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  account_id TEXT NOT NULL REFERENCES xz_tenant_wallets(tenant_id),
  entry_type TEXT NOT NULL
    CHECK (entry_type IN ('CREDIT','DEBIT','FREEZE','UNFREEZE','REVERSAL')),
  source_type TEXT NOT NULL
    CHECK (source_type IN ('RECHARGE','BONUS','PACKAGE','ACTIVITY','MODEL_USAGE','REFUND','LEGACY','MANUAL','REVERSAL')),
  compute_unit_delta BIGINT NOT NULL CHECK (compute_unit_delta <> 0),
  balance_before BIGINT NOT NULL CHECK (balance_before >= 0),
  balance_after BIGINT NOT NULL CHECK (balance_after >= 0),
  amount_cents BIGINT NOT NULL DEFAULT 0 CHECK (amount_cents >= 0),
  reference_type TEXT NOT NULL DEFAULT '',
  reference_id TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL,
  actor_user_id TEXT REFERENCES xz_users(id),
  request_id TEXT NOT NULL DEFAULT '',
  lot_allocations JSONB NOT NULL DEFAULT '[]'::jsonb,
  before_value JSONB NOT NULL DEFAULT '{}'::jsonb,
  after_value JSONB NOT NULL DEFAULT '{}'::jsonb,
  previous_hash TEXT NOT NULL DEFAULT '',
  entry_hash TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'POSTED'
    CHECK (status IN ('POSTED','REVERSED')),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, idempotency_key),
  CHECK (balance_after = balance_before + compute_unit_delta)
);

CREATE INDEX IF NOT EXISTS idx_xz_compute_ledger_tenant_time
  ON xz_compute_ledger_entries(tenant_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_xz_compute_ledger_reference
  ON xz_compute_ledger_entries(tenant_id, reference_type, reference_id, created_at DESC);

CREATE OR REPLACE FUNCTION xz_fill_compute_ledger_hash()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  last_hash TEXT;
BEGIN
  SELECT entry_hash INTO last_hash
  FROM xz_compute_ledger_entries
  WHERE tenant_id = NEW.tenant_id
  ORDER BY created_at DESC, id DESC
  LIMIT 1;
  NEW.previous_hash := coalesce(last_hash, '');
  NEW.entry_hash := md5(concat_ws('|', NEW.tenant_id, NEW.account_id, NEW.entry_type,
    NEW.source_type, NEW.compute_unit_delta, NEW.balance_before, NEW.balance_after,
    NEW.amount_cents, NEW.reference_type, NEW.reference_id, NEW.idempotency_key,
    NEW.previous_hash, NEW.created_at));
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION xz_reject_compute_ledger_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'xz_compute_ledger_entries is append-only';
END $$;

DROP TRIGGER IF EXISTS trg_xz_compute_ledger_hash ON xz_compute_ledger_entries;
CREATE TRIGGER trg_xz_compute_ledger_hash
BEFORE INSERT ON xz_compute_ledger_entries
FOR EACH ROW EXECUTE FUNCTION xz_fill_compute_ledger_hash();

DROP TRIGGER IF EXISTS trg_xz_compute_ledger_immutable ON xz_compute_ledger_entries;
CREATE TRIGGER trg_xz_compute_ledger_immutable
BEFORE UPDATE OR DELETE ON xz_compute_ledger_entries
FOR EACH ROW EXECUTE FUNCTION xz_reject_compute_ledger_mutation();

INSERT INTO xz_compute_credit_lots(
  id, tenant_id, account_id, source_type, original_units, remaining_units,
  reference_type, reference_id, idempotency_key, status, metadata
)
SELECT 'credit_legacy_' || substr(md5(wallet.tenant_id), 1, 20), wallet.tenant_id, wallet.tenant_id,
       'LEGACY', wallet.point_balance, wallet.point_balance, 'TENANT_WALLET', wallet.tenant_id,
       'legacy-wallet-balance', CASE WHEN wallet.point_balance > 0 THEN 'ACTIVE' ELSE 'EXHAUSTED' END,
       jsonb_build_object('migration', '044-enterprise-p0-safety')
FROM xz_tenant_wallets wallet
WHERE wallet.point_balance > 0
ON CONFLICT (tenant_id, idempotency_key) DO NOTHING;

INSERT INTO xz_compute_ledger_entries(
  id, tenant_id, account_id, entry_type, source_type, compute_unit_delta,
  balance_before, balance_after, reference_type, reference_id, idempotency_key,
  before_value, after_value, metadata
)
SELECT 'ledger_legacy_' || substr(md5(wallet.tenant_id), 1, 20), wallet.tenant_id, wallet.tenant_id,
       'CREDIT', 'LEGACY', wallet.point_balance, 0, wallet.point_balance,
       'TENANT_WALLET', wallet.tenant_id, 'legacy-wallet-opening-balance',
       jsonb_build_object('balance', 0, 'unit', 'COMPUTE_UNIT'),
       jsonb_build_object('balance', wallet.point_balance, 'unit', 'COMPUTE_UNIT'),
       jsonb_build_object('migration', '044-enterprise-p0-safety')
FROM xz_tenant_wallets wallet
WHERE wallet.point_balance > 0
ON CONFLICT (tenant_id, idempotency_key) DO NOTHING;

CREATE TABLE IF NOT EXISTS xz_model_usage_records (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  organization_id TEXT,
  user_id TEXT NOT NULL REFERENCES xz_users(id),
  task_id TEXT NOT NULL,
  provider_code TEXT NOT NULL DEFAULT '',
  provider_request_id TEXT,
  model_code TEXT NOT NULL,
  capability TEXT NOT NULL,
  input_tokens BIGINT NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
  output_tokens BIGINT NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
  cached_input_tokens BIGINT NOT NULL DEFAULT 0 CHECK (cached_input_tokens >= 0),
  reasoning_tokens BIGINT NOT NULL DEFAULT 0 CHECK (reasoning_tokens >= 0),
  total_tokens BIGINT NOT NULL DEFAULT 0 CHECK (total_tokens >= 0),
  compute_units_charged BIGINT NOT NULL DEFAULT 0 CHECK (compute_units_charged >= 0),
  amount_cents_charged BIGINT NOT NULL DEFAULT 0 CHECK (amount_cents_charged >= 0),
  idempotency_key TEXT NOT NULL,
  raw_usage JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'RECORDED'
    CHECK (status IN ('RECORDED','SETTLED','FAILED','REVERSED')),
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, idempotency_key),
  FOREIGN KEY (tenant_id, organization_id) REFERENCES xz_organizations(tenant_id, id)
);

CREATE INDEX IF NOT EXISTS idx_xz_model_usage_tenant_time
  ON xz_model_usage_records(tenant_id, occurred_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_xz_model_usage_task
  ON xz_model_usage_records(tenant_id, task_id, created_at DESC);

ALTER TABLE xz_generation_tasks
  ADD COLUMN IF NOT EXISTS tenant_id TEXT,
  ADD COLUMN IF NOT EXISTS organization_id TEXT,
  ADD COLUMN IF NOT EXISTS billing_account_type TEXT NOT NULL DEFAULT 'PERSONAL',
  ADD COLUMN IF NOT EXISTS billing_account_id TEXT;
CREATE INDEX IF NOT EXISTS idx_xz_generation_tasks_tenant_user
  ON xz_generation_tasks(tenant_id, user_id, created_at DESC);

ALTER TABLE xz_assets
  ADD COLUMN IF NOT EXISTS tenant_id TEXT,
  ADD COLUMN IF NOT EXISTS organization_id TEXT;
CREATE INDEX IF NOT EXISTS idx_xz_assets_tenant_user
  ON xz_assets(tenant_id, user_id, created_at DESC);

ALTER TABLE xz_payment_events
  ADD COLUMN IF NOT EXISTS tenant_id TEXT,
  ADD COLUMN IF NOT EXISTS idempotency_key TEXT,
  ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'RECEIVED',
  ADD COLUMN IF NOT EXISTS processed_at TIMESTAMPTZ;

UPDATE xz_payment_events
SET idempotency_key = provider || ':' || event_id
WHERE coalesce(idempotency_key, '') = '';

ALTER TABLE xz_payment_events ALTER COLUMN idempotency_key SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_payment_events_idempotency
  ON xz_payment_events(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_xz_payment_events_tenant_time
  ON xz_payment_events(tenant_id, created_at DESC)
  WHERE tenant_id IS NOT NULL;

ALTER TABLE xz_tenant_audit_logs
  ADD COLUMN IF NOT EXISTS before_value JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS after_value JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS idempotency_key TEXT,
  ADD COLUMN IF NOT EXISTS previous_hash TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS entry_hash TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_xz_tenant_audit_resource
  ON xz_tenant_audit_logs(tenant_id, resource_type, resource_id, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_customer_attribution_history (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  source_agent_id TEXT,
  operation_center_id TEXT,
  previous_source_agent_id TEXT,
  previous_operation_center_id TEXT,
  effective_from TIMESTAMPTZ NOT NULL DEFAULT now(),
  effective_to TIMESTAMPTZ,
  reason TEXT NOT NULL DEFAULT '',
  change_request_id TEXT,
  actor_user_id TEXT REFERENCES xz_users(id),
  before_value JSONB NOT NULL DEFAULT '{}'::jsonb,
  after_value JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'ACTIVE'
    CHECK (status IN ('ACTIVE','SUPERSEDED','REJECTED')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_customer_attribution_current
  ON xz_customer_attribution_history(tenant_id)
  WHERE status = 'ACTIVE' AND effective_to IS NULL;
CREATE INDEX IF NOT EXISTS idx_xz_customer_attribution_history
  ON xz_customer_attribution_history(tenant_id, created_at DESC, id DESC);

INSERT INTO xz_customer_attribution_history(
  id, tenant_id, source_agent_id, operation_center_id, reason,
  before_value, after_value, status
)
SELECT 'attribution_legacy_' || substr(md5(tenant.id), 1, 20), tenant.id,
       tenant.source_agent_id, tenant.operation_center_id, 'legacy attribution backfill',
       '{}'::jsonb,
       jsonb_build_object('sourceAgentId', coalesce(tenant.source_agent_id, ''),
                          'operationCenterId', coalesce(tenant.operation_center_id, '')),
       'ACTIVE'
FROM xz_tenants tenant
WHERE tenant.tenant_type = 'ENTERPRISE'
ON CONFLICT DO NOTHING;

INSERT INTO permissions(code, name, module, action)
VALUES
  ('enterprise.ai.use', 'Use enterprise AI capability', 'enterprise', 'ai_use'),
  ('enterprise.compute.ledger.read', 'Read enterprise compute ledger', 'enterprise', 'compute_ledger_read'),
  ('enterprise:service:transition', 'Transition enterprise service state', 'enterprise', 'service_transition')
ON CONFLICT (code) DO UPDATE SET name=excluded.name, module=excluded.module, action=excluded.action;

INSERT INTO xz_role_permissions(role, permission)
SELECT * FROM (VALUES
  ('ENTERPRISE_ADMIN','enterprise.ai.use'),
  ('AI_ADMIN','enterprise.ai.use'),
  ('ENTERPRISE_MEMBER','enterprise.ai.use'),
  ('ENTERPRISE_ADMIN','enterprise.compute.ledger.read'),
  ('FINANCE','enterprise.compute.ledger.read'),
  ('SUPER_ADMIN','enterprise:service:transition'),
  ('RISK_MANAGER','enterprise:service:transition')
) AS matrix(role_code, permission_code)
ON CONFLICT (role, permission) DO NOTHING;

INSERT INTO role_permissions(role_id, permission_id)
SELECT role.id, permission.id
FROM (VALUES
  ('ENTERPRISE_ADMIN','enterprise.ai.use'),
  ('AI_ADMIN','enterprise.ai.use'),
  ('ENTERPRISE_MEMBER','enterprise.ai.use'),
  ('ENTERPRISE_ADMIN','enterprise.compute.ledger.read'),
  ('FINANCE','enterprise.compute.ledger.read'),
  ('SUPER_ADMIN','enterprise:service:transition'),
  ('RISK_MANAGER','enterprise:service:transition')
) AS matrix(role_code, permission_code)
JOIN roles role ON role.code=matrix.role_code
JOIN permissions permission ON permission.code=matrix.permission_code
ON CONFLICT (role_id, permission_id) DO NOTHING;

COMMIT;
