-- Commission settlement and flexible-labor payout center.
-- Cash amounts are always integer cents. Percentage rules use integer basis points (1/100 of 1%).
-- Existing xz_commissions/xz_withdrawals remain compatibility read models; the tables below are authoritative.

BEGIN;

CREATE TABLE IF NOT EXISTS xz_commission_rules (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  rule_code TEXT NOT NULL,
  rule_name TEXT NOT NULL,
  product_type TEXT NOT NULL,
  product_id TEXT,
  beneficiary_role TEXT NOT NULL,
  relationship_level INT NOT NULL DEFAULT 0 CHECK (relationship_level >= 0),
  calculation_type TEXT NOT NULL CHECK (calculation_type IN (
    'FIXED_AMOUNT', 'ORDER_PERCENTAGE', 'PAID_AMOUNT_PERCENTAGE',
    'QUANTITY', 'TIERED', 'REMAINDER_TO_PLATFORM'
  )),
  fixed_amount_cents BIGINT NOT NULL DEFAULT 0 CHECK (fixed_amount_cents >= 0),
  percentage_bps BIGINT NOT NULL DEFAULT 0 CHECK (percentage_bps BETWEEN 0 AND 10000),
  calculation_config JSONB NOT NULL DEFAULT '{}'::jsonb,
  priority INT NOT NULL DEFAULT 100,
  freeze_days INT NOT NULL DEFAULT 0 CHECK (freeze_days >= 0),
  refund_policy TEXT NOT NULL DEFAULT 'REVERSE_OR_RECOVER',
  effective_start_at TIMESTAMPTZ NOT NULL,
  effective_end_at TIMESTAMPTZ,
  version INT NOT NULL CHECK (version > 0),
  status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('DRAFT', 'ACTIVE', 'INACTIVE', 'ARCHIVED')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (effective_end_at IS NULL OR effective_end_at > effective_start_at),
  CHECK (
    (calculation_type = 'FIXED_AMOUNT' AND fixed_amount_cents > 0) OR
    (calculation_type IN ('ORDER_PERCENTAGE', 'PAID_AMOUNT_PERCENTAGE') AND percentage_bps > 0) OR
    (calculation_type = 'QUANTITY' AND fixed_amount_cents > 0) OR
    calculation_type IN ('TIERED', 'REMAINDER_TO_PLATFORM')
  ),
  UNIQUE (tenant_id, rule_code, version)
);

CREATE INDEX IF NOT EXISTS idx_xz_commission_rules_lookup
  ON xz_commission_rules(tenant_id, product_type, product_id, status, effective_start_at, effective_end_at, priority);

CREATE TABLE IF NOT EXISTS xz_commission_records (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  order_id TEXT NOT NULL REFERENCES xz_orders(id),
  order_no TEXT NOT NULL,
  beneficiary_type TEXT NOT NULL CHECK (beneficiary_type IN ('AGENT', 'OPERATION_CENTER', 'PLATFORM')),
  beneficiary_id TEXT NOT NULL,
  source_user_id TEXT NOT NULL REFERENCES xz_users(id),
  rule_id TEXT NOT NULL REFERENCES xz_commission_rules(id),
  rule_version INT NOT NULL CHECK (rule_version > 0),
  amount_cents BIGINT NOT NULL CHECK (amount_cents <> 0),
  currency CHAR(3) NOT NULL DEFAULT 'CNY',
  record_type TEXT NOT NULL CHECK (record_type IN ('EARNING', 'REVERSAL', 'ADJUSTMENT')),
  status TEXT NOT NULL CHECK (status IN (
    'EXPECTED', 'FROZEN', 'AVAILABLE', 'SETTLING', 'SETTLED',
    'REVERSED', 'CANCELLED'
  )),
  freeze_until TIMESTAMPTZ,
  available_at TIMESTAMPTZ,
  reversal_of_id TEXT REFERENCES xz_commission_records(id),
  idempotency_key TEXT NOT NULL UNIQUE,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (
    (record_type = 'EARNING' AND amount_cents > 0 AND reversal_of_id IS NULL) OR
    (record_type = 'REVERSAL' AND amount_cents < 0 AND reversal_of_id IS NOT NULL) OR
    (record_type = 'ADJUSTMENT')
  )
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_commission_records_earning
  ON xz_commission_records(order_id, rule_id, rule_version, beneficiary_type, beneficiary_id)
  WHERE record_type = 'EARNING';
CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_commission_records_reversal
  ON xz_commission_records(reversal_of_id)
  WHERE record_type = 'REVERSAL';
CREATE INDEX IF NOT EXISTS idx_xz_commission_records_beneficiary
  ON xz_commission_records(tenant_id, beneficiary_type, beneficiary_id, status, available_at, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_commission_records_order
  ON xz_commission_records(tenant_id, order_id, created_at);

CREATE TABLE IF NOT EXISTS xz_commission_wallet_accounts (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  beneficiary_type TEXT NOT NULL CHECK (beneficiary_type IN ('AGENT', 'OPERATION_CENTER', 'PLATFORM')),
  beneficiary_id TEXT NOT NULL,
  expected_cents BIGINT NOT NULL DEFAULT 0 CHECK (expected_cents >= 0),
  frozen_cents BIGINT NOT NULL DEFAULT 0 CHECK (frozen_cents >= 0),
  available_cents BIGINT NOT NULL DEFAULT 0 CHECK (available_cents >= 0),
  settling_cents BIGINT NOT NULL DEFAULT 0 CHECK (settling_cents >= 0),
  settled_cents BIGINT NOT NULL DEFAULT 0 CHECK (settled_cents >= 0),
  recoverable_cents BIGINT NOT NULL DEFAULT 0 CHECK (recoverable_cents >= 0),
  version BIGINT NOT NULL DEFAULT 0 CHECK (version >= 0),
  status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'FROZEN', 'CLOSED')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, beneficiary_type, beneficiary_id)
);

CREATE TABLE IF NOT EXISTS xz_settlement_applications (
  id TEXT PRIMARY KEY,
  application_no TEXT NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  applicant_type TEXT NOT NULL CHECK (applicant_type IN ('AGENT', 'OPERATION_CENTER')),
  applicant_id TEXT NOT NULL,
  settlement_period_start DATE NOT NULL,
  settlement_period_end DATE NOT NULL,
  applied_amount_cents BIGINT NOT NULL CHECK (applied_amount_cents > 0),
  approved_amount_cents BIGINT NOT NULL DEFAULT 0 CHECK (approved_amount_cents >= 0),
  rejected_amount_cents BIGINT NOT NULL DEFAULT 0 CHECK (rejected_amount_cents >= 0),
  status TEXT NOT NULL CHECK (status IN (
    'PENDING_REVIEW', 'REVIEWING', 'APPROVED', 'PARTIALLY_APPROVED', 'REJECTED',
    'BATCHED', 'PAYOUT_PROCESSING', 'PARTIALLY_SUCCEEDED', 'COMPLETED'
  )),
  reject_reason TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL UNIQUE,
  submitted_at TIMESTAMPTZ,
  approved_at TIMESTAMPTZ,
  approved_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (settlement_period_end >= settlement_period_start),
  CHECK (approved_amount_cents + rejected_amount_cents <= applied_amount_cents)
);

CREATE INDEX IF NOT EXISTS idx_xz_settlement_applications_applicant
  ON xz_settlement_applications(tenant_id, applicant_type, applicant_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_settlement_application_items (
  id TEXT PRIMARY KEY,
  application_id TEXT NOT NULL REFERENCES xz_settlement_applications(id),
  commission_record_id TEXT NOT NULL UNIQUE REFERENCES xz_commission_records(id),
  amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
  status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'BATCHED', 'PAID', 'FAILED')),
  reject_reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_xz_settlement_application_items_application
  ON xz_settlement_application_items(application_id, status);

CREATE TABLE IF NOT EXISTS xz_labor_worker_profiles (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  user_id TEXT NOT NULL REFERENCES xz_users(id),
  provider_code TEXT NOT NULL,
  provider_worker_id TEXT,
  subject_type TEXT NOT NULL CHECK (subject_type IN ('AGENT', 'OPERATION_CENTER')),
  real_name TEXT NOT NULL,
  id_card_ciphertext BYTEA NOT NULL,
  id_card_hash TEXT NOT NULL,
  mobile_ciphertext BYTEA NOT NULL,
  mobile_hash TEXT NOT NULL,
  bank_card_ciphertext BYTEA NOT NULL,
  bank_card_hash TEXT NOT NULL,
  bank_name TEXT NOT NULL,
  real_name_status TEXT NOT NULL DEFAULT 'PENDING',
  contract_status TEXT NOT NULL DEFAULT 'PENDING',
  bank_bind_status TEXT NOT NULL DEFAULT 'PENDING',
  risk_status TEXT NOT NULL DEFAULT 'NORMAL',
  contract_url TEXT NOT NULL DEFAULT '',
  contract_completed_at TIMESTAMPTZ,
  encryption_key_version TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, user_id, provider_code, subject_type),
  UNIQUE (tenant_id, provider_code, id_card_hash)
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_labor_worker_provider_id
  ON xz_labor_worker_profiles(provider_code, provider_worker_id)
  WHERE provider_worker_id IS NOT NULL AND provider_worker_id <> '';

CREATE TABLE IF NOT EXISTS xz_payout_excel_templates (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  template_name TEXT NOT NULL,
  provider_code TEXT NOT NULL,
  file_type TEXT NOT NULL DEFAULT 'XLSX' CHECK (file_type IN ('XLSX')),
  sheet_name TEXT NOT NULL DEFAULT 'Sheet1',
  title_row INT NOT NULL DEFAULT 1 CHECK (title_row > 0),
  data_start_row INT NOT NULL DEFAULT 2 CHECK (data_start_row > 0),
  field_mapping JSONB NOT NULL,
  amount_unit TEXT NOT NULL DEFAULT 'YUAN' CHECK (amount_unit IN ('CENT', 'YUAN')),
  date_format TEXT NOT NULL DEFAULT 'yyyy-mm-dd hh:mm:ss',
  default_task_name TEXT NOT NULL DEFAULT '',
  default_remark TEXT NOT NULL DEFAULT '',
  requires_id_card BOOLEAN NOT NULL DEFAULT TRUE,
  requires_bank_card BOOLEAN NOT NULL DEFAULT TRUE,
  import_mapping JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('DRAFT', 'ACTIVE', 'INACTIVE')),
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, provider_code, template_name)
);

CREATE TABLE IF NOT EXISTS xz_labor_payout_provider_configs (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  provider_code TEXT NOT NULL,
  provider_name TEXT NOT NULL,
  provider_type TEXT NOT NULL CHECK (provider_type IN ('EXCEL_MANUAL', 'API')),
  business_scene TEXT NOT NULL,
  limits JSONB NOT NULL DEFAULT '{}'::jsonb,
  encrypted_config BYTEA,
  encryption_key_version TEXT,
  status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, provider_code, business_scene)
);

CREATE TABLE IF NOT EXISTS xz_commission_payout_batches (
  id TEXT PRIMARY KEY,
  batch_no TEXT NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  provider_code TEXT NOT NULL,
  business_scene TEXT NOT NULL,
  template_id TEXT REFERENCES xz_payout_excel_templates(id),
  settlement_period_start DATE NOT NULL,
  settlement_period_end DATE NOT NULL,
  total_count INT NOT NULL DEFAULT 0 CHECK (total_count >= 0),
  total_amount_cents BIGINT NOT NULL DEFAULT 0 CHECK (total_amount_cents >= 0),
  success_count INT NOT NULL DEFAULT 0 CHECK (success_count >= 0),
  success_amount_cents BIGINT NOT NULL DEFAULT 0 CHECK (success_amount_cents >= 0),
  failed_count INT NOT NULL DEFAULT 0 CHECK (failed_count >= 0),
  failed_amount_cents BIGINT NOT NULL DEFAULT 0 CHECK (failed_amount_cents >= 0),
  provider_fee_cents BIGINT NOT NULL DEFAULT 0 CHECK (provider_fee_cents >= 0),
  status TEXT NOT NULL CHECK (status IN (
    'DRAFT', 'VALIDATING', 'VALIDATION_FAILED', 'PENDING_APPROVAL', 'APPROVED',
    'READY_TO_EXPORT', 'EXPORTED', 'SUBMITTED', 'PROCESSING',
    'PARTIAL_SUCCESS', 'SUCCESS', 'CLOSED'
  )),
  export_file_url TEXT NOT NULL DEFAULT '',
  export_file_hash TEXT NOT NULL DEFAULT '',
  provider_batch_no TEXT NOT NULL DEFAULT '',
  submitted_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  created_by TEXT NOT NULL,
  approved_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (settlement_period_end >= settlement_period_start),
  CHECK (success_count + failed_count <= total_count),
  CHECK (success_amount_cents + failed_amount_cents <= total_amount_cents)
);

CREATE INDEX IF NOT EXISTS idx_xz_commission_payout_batches_status
  ON xz_commission_payout_batches(tenant_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_commission_payout_details (
  id TEXT PRIMARY KEY,
  batch_id TEXT NOT NULL REFERENCES xz_commission_payout_batches(id),
  detail_no TEXT NOT NULL UNIQUE,
  settlement_application_id TEXT NOT NULL REFERENCES xz_settlement_applications(id),
  beneficiary_type TEXT NOT NULL CHECK (beneficiary_type IN ('AGENT', 'OPERATION_CENTER')),
  beneficiary_id TEXT NOT NULL,
  worker_profile_id TEXT NOT NULL REFERENCES xz_labor_worker_profiles(id),
  amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
  task_name TEXT NOT NULL,
  service_period TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN (
    'PENDING', 'VALIDATION_FAILED', 'READY', 'SUBMITTED', 'PROCESSING', 'SUCCESS', 'FAILED', 'CLOSED'
  )),
  failure_code TEXT NOT NULL DEFAULT '',
  failure_message TEXT NOT NULL DEFAULT '',
  provider_order_no TEXT NOT NULL DEFAULT '',
  paid_at TIMESTAMPTZ,
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_commission_payout_details_application
  ON xz_commission_payout_details(settlement_application_id);
CREATE INDEX IF NOT EXISTS idx_xz_commission_payout_details_batch
  ON xz_commission_payout_details(batch_id, status, detail_no);

CREATE TABLE IF NOT EXISTS xz_payout_import_records (
  id TEXT PRIMARY KEY,
  batch_id TEXT NOT NULL REFERENCES xz_commission_payout_batches(id),
  file_name TEXT NOT NULL,
  file_url TEXT NOT NULL,
  file_hash TEXT NOT NULL,
  total_rows INT NOT NULL DEFAULT 0 CHECK (total_rows >= 0),
  matched_rows INT NOT NULL DEFAULT 0 CHECK (matched_rows >= 0),
  unmatched_rows INT NOT NULL DEFAULT 0 CHECK (unmatched_rows >= 0),
  error_rows INT NOT NULL DEFAULT 0 CHECK (error_rows >= 0),
  status TEXT NOT NULL CHECK (status IN ('UPLOADED', 'PARSING', 'COMPLETED', 'COMPLETED_WITH_ERRORS', 'FAILED')),
  operator_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  UNIQUE (batch_id, file_hash),
  CHECK (matched_rows + unmatched_rows + error_rows <= total_rows)
);

CREATE TABLE IF NOT EXISTS xz_payout_import_exceptions (
  id TEXT PRIMARY KEY,
  import_record_id TEXT NOT NULL REFERENCES xz_payout_import_records(id),
  batch_id TEXT NOT NULL REFERENCES xz_commission_payout_batches(id),
  row_number INT NOT NULL CHECK (row_number > 0),
  detail_no TEXT NOT NULL DEFAULT '',
  exception_type TEXT NOT NULL CHECK (exception_type IN (
    'HEADER_MISMATCH', 'UNMATCHED', 'DUPLICATE', 'AMOUNT_MISMATCH',
    'RECEIVER_MISMATCH', 'INVALID_STATUS', 'INVALID_ROW'
  )),
  message TEXT NOT NULL,
  row_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'RESOLVED', 'IGNORED')),
  resolved_by TEXT,
  resolved_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (import_record_id, row_number, exception_type)
);

CREATE TABLE IF NOT EXISTS xz_payout_callback_logs (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  provider_code TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  headers JSONB NOT NULL DEFAULT '{}'::jsonb,
  raw_body BYTEA NOT NULL,
  signature_verified BOOLEAN NOT NULL DEFAULT FALSE,
  processing_status TEXT NOT NULL DEFAULT 'RECEIVED' CHECK (processing_status IN ('RECEIVED', 'PROCESSED', 'IGNORED', 'FAILED')),
  error_message TEXT NOT NULL DEFAULT '',
  received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  processed_at TIMESTAMPTZ,
  UNIQUE (provider_code, idempotency_key)
);

CREATE TABLE IF NOT EXISTS xz_payout_reconciliation_differences (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  reconciliation_date DATE NOT NULL,
  provider_code TEXT NOT NULL,
  batch_id TEXT REFERENCES xz_commission_payout_batches(id),
  payout_detail_id TEXT REFERENCES xz_commission_payout_details(id),
  difference_type TEXT NOT NULL CHECK (difference_type IN ('STATUS', 'AMOUNT', 'ORDER', 'FEE', 'MISSING_INTERNAL', 'MISSING_PROVIDER')),
  internal_status TEXT NOT NULL DEFAULT '',
  provider_status TEXT NOT NULL DEFAULT '',
  internal_amount_cents BIGINT,
  provider_amount_cents BIGINT,
  internal_fee_cents BIGINT,
  provider_fee_cents BIGINT,
  details JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'RESOLVED', 'IGNORED')),
  resolved_by TEXT,
  resolved_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_xz_payout_reconciliation_open
  ON xz_payout_reconciliation_differences(tenant_id, reconciliation_date DESC, status);

CREATE TABLE IF NOT EXISTS xz_commission_wallet_ledger (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  account_id TEXT NOT NULL REFERENCES xz_commission_wallet_accounts(id),
  beneficiary_type TEXT NOT NULL,
  beneficiary_id TEXT NOT NULL,
  business_type TEXT NOT NULL,
  business_id TEXT NOT NULL,
  direction TEXT NOT NULL CHECK (direction IN ('CREDIT', 'DEBIT', 'TRANSFER')),
  expected_delta_cents BIGINT NOT NULL DEFAULT 0,
  frozen_delta_cents BIGINT NOT NULL DEFAULT 0,
  available_delta_cents BIGINT NOT NULL DEFAULT 0,
  settling_delta_cents BIGINT NOT NULL DEFAULT 0,
  settled_delta_cents BIGINT NOT NULL DEFAULT 0,
  recoverable_delta_cents BIGINT NOT NULL DEFAULT 0,
  balances_before JSONB NOT NULL,
  balances_after JSONB NOT NULL,
  commission_record_id TEXT REFERENCES xz_commission_records(id),
  settlement_application_id TEXT REFERENCES xz_settlement_applications(id),
  payout_detail_id TEXT REFERENCES xz_commission_payout_details(id),
  idempotency_key TEXT NOT NULL UNIQUE,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (
    expected_delta_cents <> 0 OR frozen_delta_cents <> 0 OR available_delta_cents <> 0 OR
    settling_delta_cents <> 0 OR settled_delta_cents <> 0 OR recoverable_delta_cents <> 0
  )
);

CREATE INDEX IF NOT EXISTS idx_xz_commission_wallet_ledger_account
  ON xz_commission_wallet_ledger(account_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_xz_commission_wallet_ledger_business
  ON xz_commission_wallet_ledger(tenant_id, business_type, business_id);

CREATE TABLE IF NOT EXISTS xz_finance_audit_logs (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  operator_id TEXT NOT NULL,
  operator_role TEXT NOT NULL,
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  batch_id TEXT REFERENCES xz_commission_payout_batches(id),
  affected_count INT NOT NULL DEFAULT 0 CHECK (affected_count >= 0),
  affected_amount_cents BIGINT NOT NULL DEFAULT 0,
  client_ip INET,
  request_id TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_xz_finance_audit_logs_scope
  ON xz_finance_audit_logs(tenant_id, created_at DESC, action, resource_type);

CREATE OR REPLACE FUNCTION xz_protect_commission_record_immutability()
RETURNS trigger AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'commission records cannot be physically deleted';
  END IF;
  IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
     OR NEW.order_id IS DISTINCT FROM OLD.order_id
     OR NEW.order_no IS DISTINCT FROM OLD.order_no
     OR NEW.beneficiary_type IS DISTINCT FROM OLD.beneficiary_type
     OR NEW.beneficiary_id IS DISTINCT FROM OLD.beneficiary_id
     OR NEW.source_user_id IS DISTINCT FROM OLD.source_user_id
     OR NEW.rule_id IS DISTINCT FROM OLD.rule_id
     OR NEW.rule_version IS DISTINCT FROM OLD.rule_version
     OR NEW.amount_cents IS DISTINCT FROM OLD.amount_cents
     OR NEW.currency IS DISTINCT FROM OLD.currency
     OR NEW.record_type IS DISTINCT FROM OLD.record_type
     OR NEW.reversal_of_id IS DISTINCT FROM OLD.reversal_of_id
     OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'immutable commission record fields cannot be changed';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_commission_records_immutable ON xz_commission_records;
CREATE TRIGGER trg_xz_commission_records_immutable
  BEFORE UPDATE OR DELETE ON xz_commission_records
  FOR EACH ROW EXECUTE FUNCTION xz_protect_commission_record_immutability();

CREATE OR REPLACE FUNCTION xz_prevent_submitted_payout_batch_delete()
RETURNS trigger AS $$
BEGIN
  IF OLD.status IN ('SUBMITTED', 'PROCESSING', 'PARTIAL_SUCCESS', 'SUCCESS', 'CLOSED')
     OR OLD.provider_batch_no <> '' OR OLD.submitted_at IS NOT NULL THEN
    RAISE EXCEPTION 'submitted payout batches cannot be physically deleted';
  END IF;
  RETURN OLD;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_payout_batches_no_submitted_delete ON xz_commission_payout_batches;
CREATE TRIGGER trg_xz_payout_batches_no_submitted_delete
  BEFORE DELETE ON xz_commission_payout_batches
  FOR EACH ROW EXECUTE FUNCTION xz_prevent_submitted_payout_batch_delete();

CREATE OR REPLACE FUNCTION xz_prevent_submitted_payout_detail_delete()
RETURNS trigger AS $$
DECLARE batch_status TEXT;
BEGIN
  SELECT status INTO batch_status FROM xz_commission_payout_batches WHERE id = OLD.batch_id;
  IF OLD.status IN ('SUBMITTED', 'PROCESSING', 'SUCCESS', 'FAILED', 'CLOSED')
     OR OLD.provider_order_no <> ''
     OR batch_status IN ('SUBMITTED', 'PROCESSING', 'PARTIAL_SUCCESS', 'SUCCESS', 'CLOSED') THEN
    RAISE EXCEPTION 'submitted payout details cannot be physically deleted';
  END IF;
  RETURN OLD;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_payout_details_no_submitted_delete ON xz_commission_payout_details;
CREATE TRIGGER trg_xz_payout_details_no_submitted_delete
  BEFORE DELETE ON xz_commission_payout_details
  FOR EACH ROW EXECUTE FUNCTION xz_prevent_submitted_payout_detail_delete();

INSERT INTO xz_commission_rules (
  id, tenant_id, rule_code, rule_name, product_type, product_id,
  beneficiary_role, relationship_level, calculation_type, fixed_amount_cents,
  percentage_bps, priority, freeze_days, refund_policy, effective_start_at, version, status
)
VALUES
  ('commission_rule_member_996_agent_v1', 'tenant_default', 'MEMBER_996_DIRECT_AGENT', '996会员直属代理现金分润',
   'MEMBER_PACKAGE', 'plan_ai_creator_996', 'AGENT', 1, 'FIXED_AMOUNT', 30000, 0, 10, 7,
   'REVERSE_OR_RECOVER', '2020-01-01T00:00:00Z', 1, 'ACTIVE'),
  ('commission_rule_member_996_operation_v1', 'tenant_default', 'MEMBER_996_OPERATION_CENTER', '996会员运营中心服务分润',
   'MEMBER_PACKAGE', 'plan_ai_creator_996', 'OPERATION_CENTER', 1, 'FIXED_AMOUNT', 20000, 0, 20, 7,
   'REVERSE_OR_RECOVER', '2020-01-01T00:00:00Z', 1, 'ACTIVE'),
  ('commission_rule_member_996_platform_v1', 'tenant_default', 'MEMBER_996_PLATFORM_REMAINDER', '996会员平台现金留存',
   'MEMBER_PACKAGE', 'plan_ai_creator_996', 'PLATFORM', 0, 'REMAINDER_TO_PLATFORM', 0, 0, 1000, 0,
   'REVERSE_OR_RECOVER', '2020-01-01T00:00:00Z', 1, 'ACTIVE')
ON CONFLICT (tenant_id, rule_code, version) DO NOTHING;

INSERT INTO xz_labor_payout_provider_configs (
  id, tenant_id, provider_code, provider_name, provider_type, business_scene, limits, status
)
VALUES (
  'labor_provider_excel_manual_default', 'tenant_default', 'EXCEL_MANUAL', 'Excel半自动发佣',
  'EXCEL_MANUAL', 'COMMISSION_PAYOUT',
  '{"minSettlementAmountCents":1,"maxSinglePayoutAmountCents":5000000}'::jsonb, 'ACTIVE'
)
ON CONFLICT (tenant_id, provider_code, business_scene) DO NOTHING;

COMMIT;
