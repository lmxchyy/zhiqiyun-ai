-- 知启云AI：推广中心 / 我的推广码 V1
-- 扩展既有邀请记录，不创建平行的奖励或身份体系。

ALTER TABLE IF EXISTS xz_marketing_invite_records
  ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'tenant_default',
  ADD COLUMN IF NOT EXISTS visitor_id TEXT,
  ADD COLUMN IF NOT EXISTS visitor_name TEXT,
  ADD COLUMN IF NOT EXISTS masked_mobile TEXT,
  ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'visited',
  ADD COLUMN IF NOT EXISTS template_id TEXT NOT NULL DEFAULT 'poster.brand.simple',
  ADD COLUMN IF NOT EXISTS activity_id TEXT,
  ADD COLUMN IF NOT EXISTS visit_time TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS register_time TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS paid_time TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS reward_amount_cents BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS reward_status TEXT NOT NULL DEFAULT 'PENDING';

CREATE INDEX IF NOT EXISTS idx_xz_marketing_invite_records_tenant_inviter
  ON xz_marketing_invite_records(tenant_id, inviter_user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_xz_marketing_invite_records_status
  ON xz_marketing_invite_records(tenant_id, inviter_user_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_xz_marketing_invite_records_visitor
  ON xz_marketing_invite_records(visitor_id, created_at DESC);
