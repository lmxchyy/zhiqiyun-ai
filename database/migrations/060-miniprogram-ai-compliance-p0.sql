-- WeChat mini-program AI generation compliance foundation (P0).
-- Existing PC/Web models and capabilities are preserved. New models default to
-- unavailable for mini-program use until all compliance gates are satisfied.

ALTER TABLE IF EXISTS ai_models ADD COLUMN IF NOT EXISTS provider_name TEXT;
ALTER TABLE IF EXISTS ai_models ADD COLUMN IF NOT EXISTS provider_company TEXT;
ALTER TABLE IF EXISTS ai_models ADD COLUMN IF NOT EXISTS algorithm_name TEXT;
ALTER TABLE IF EXISTS ai_models ADD COLUMN IF NOT EXISTS algorithm_filing_no TEXT;
ALTER TABLE IF EXISTS ai_models ADD COLUMN IF NOT EXISTS algorithm_type TEXT;
ALTER TABLE IF EXISTS ai_models ADD COLUMN IF NOT EXISTS contract_status TEXT NOT NULL DEFAULT 'draft';
ALTER TABLE IF EXISTS ai_models ADD COLUMN IF NOT EXISTS contract_expire_at TIMESTAMPTZ;
ALTER TABLE IF EXISTS ai_models ADD COLUMN IF NOT EXISTS compliance_status TEXT NOT NULL DEFAULT 'draft';
ALTER TABLE IF EXISTS ai_models ADD COLUMN IF NOT EXISTS allowed_terminals JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE IF EXISTS ai_models ADD COLUMN IF NOT EXISTS allowed_capabilities JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE IF EXISTS ai_models ADD COLUMN IF NOT EXISTS miniprogram_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE IF EXISTS ai_models ADD COLUMN IF NOT EXISTS compliance_remark TEXT;
ALTER TABLE IF EXISTS ai_models ADD COLUMN IF NOT EXISTS model_version TEXT;

CREATE INDEX IF NOT EXISTS idx_ai_models_miniprogram_compliance
  ON ai_models(miniprogram_enabled, compliance_status, contract_expire_at);

ALTER TABLE IF EXISTS generation_tasks ADD COLUMN IF NOT EXISTS terminal TEXT;
ALTER TABLE IF EXISTS generation_tasks ADD COLUMN IF NOT EXISTS provider_company TEXT;
ALTER TABLE IF EXISTS generation_tasks ADD COLUMN IF NOT EXISTS algorithm_name TEXT;
ALTER TABLE IF EXISTS generation_tasks ADD COLUMN IF NOT EXISTS algorithm_filing_no TEXT;
ALTER TABLE IF EXISTS generation_tasks ADD COLUMN IF NOT EXISTS model_version TEXT;
ALTER TABLE IF EXISTS generation_tasks ADD COLUMN IF NOT EXISTS input_audit_status TEXT;
ALTER TABLE IF EXISTS generation_tasks ADD COLUMN IF NOT EXISTS output_audit_status TEXT;
ALTER TABLE IF EXISTS generation_tasks ADD COLUMN IF NOT EXISTS ai_label_status TEXT;

ALTER TABLE IF EXISTS xz_generation_tasks ADD COLUMN IF NOT EXISTS terminal TEXT;
ALTER TABLE IF EXISTS xz_generation_tasks ADD COLUMN IF NOT EXISTS input_audit_status TEXT;
ALTER TABLE IF EXISTS xz_generation_tasks ADD COLUMN IF NOT EXISTS output_audit_status TEXT;
ALTER TABLE IF EXISTS xz_generation_tasks ADD COLUMN IF NOT EXISTS ai_label_status TEXT;

CREATE TABLE IF NOT EXISTS xz_content_audits (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  tenant_id TEXT,
  terminal TEXT NOT NULL,
  stage TEXT NOT NULL CHECK (stage IN ('input','output','download','share')),
  content_type TEXT NOT NULL,
  content_id TEXT,
  status TEXT NOT NULL CHECK (status IN ('pending','approved','rejected','manual_review')),
  service_kind TEXT NOT NULL DEFAULT 'mock',
  service_request_id TEXT,
  reason_code TEXT,
  reason_detail_encrypted TEXT,
  refund_idempotency_key TEXT,
  reviewed_by TEXT,
  reviewed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(task_id, stage, content_type, content_id)
);
CREATE INDEX IF NOT EXISTS idx_xz_content_audits_review ON xz_content_audits(status, created_at DESC);

CREATE TABLE IF NOT EXISTS xz_ai_download_derivatives (
  id TEXT PRIMARY KEY,
  content_id TEXT NOT NULL,
  original_file_id TEXT NOT NULL,
  marked_file_id TEXT,
  label_config_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  metadata_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'PENDING',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(content_id, original_file_id)
);

CREATE TABLE IF NOT EXISTS xz_legal_documents (
  id TEXT PRIMARY KEY,
  code TEXT NOT NULL,
  title TEXT NOT NULL,
  version TEXT NOT NULL,
  content TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'DRAFT',
  published_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(code, version)
);

CREATE TABLE IF NOT EXISTS xz_user_agreement_acceptances (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  document_code TEXT NOT NULL,
  document_version TEXT NOT NULL,
  terminal TEXT NOT NULL,
  accepted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  request_id TEXT,
  UNIQUE(user_id, document_code, document_version, terminal)
);

CREATE TABLE IF NOT EXISTS xz_complaints (
  id TEXT PRIMARY KEY,
  user_id TEXT,
  tenant_id TEXT,
  terminal TEXT NOT NULL,
  category TEXT NOT NULL,
  content_id TEXT,
  description TEXT NOT NULL,
  contact_encrypted TEXT,
  status TEXT NOT NULL DEFAULT 'PENDING',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS xz_ai_label_settings (
  id TEXT PRIMARY KEY,
  terminal TEXT NOT NULL UNIQUE,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  label_text TEXT NOT NULL DEFAULT '本内容由人工智能生成',
  short_label_text TEXT NOT NULL DEFAULT 'AI生成',
  position TEXT NOT NULL DEFAULT 'bottom-right',
  opacity NUMERIC(4,3) NOT NULL DEFAULT 0.650,
  size_ratio NUMERIC(5,4) NOT NULL DEFAULT 0.0350,
  implicit_label_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO xz_ai_label_settings(id, terminal)
VALUES ('ai_label_miniprogram', 'miniprogram')
ON CONFLICT (terminal) DO NOTHING;
