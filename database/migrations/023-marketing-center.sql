-- AI 运营中心营销端首批表结构。
-- 这批表承接扫码邀请、角色升级、多级分佣、钱包流水和数据权限，先作为非破坏性增量落地。

CREATE TABLE IF NOT EXISTS xz_marketing_roles (
  id TEXT PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  level INT NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS xz_marketing_permissions (
  id TEXT PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  module TEXT NOT NULL,
  action TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS xz_marketing_role_permissions (
  role_code TEXT NOT NULL,
  permission_code TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (role_code, permission_code)
);

CREATE TABLE IF NOT EXISTS xz_marketing_org_relations (
  ancestor_user_id TEXT NOT NULL,
  descendant_user_id TEXT NOT NULL,
  depth INT NOT NULL,
  bind_type TEXT NOT NULL DEFAULT 'INVITE',
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (ancestor_user_id, descendant_user_id, depth)
);

CREATE TABLE IF NOT EXISTS xz_marketing_invite_codes (
  id TEXT PRIMARY KEY,
  owner_user_id TEXT NOT NULL,
  code TEXT NOT NULL UNIQUE,
  qrcode_url TEXT NOT NULL DEFAULT '',
  landing_url TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  expire_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS xz_marketing_invite_records (
  id TEXT PRIMARY KEY,
  inviter_user_id TEXT NOT NULL,
  invitee_user_id TEXT,
  invite_code TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'QR',
  register_status TEXT NOT NULL DEFAULT 'PENDING',
  recharge_status TEXT NOT NULL DEFAULT 'PENDING',
  upgrade_status TEXT NOT NULL DEFAULT 'PENDING',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS xz_marketing_wallets (
  user_id TEXT PRIMARY KEY,
  balance_cents BIGINT NOT NULL DEFAULT 0,
  frozen_cents BIGINT NOT NULL DEFAULT 0,
  total_income_cents BIGINT NOT NULL DEFAULT 0,
  total_withdraw_cents BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS xz_marketing_wallet_records (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  biz_type TEXT NOT NULL,
  biz_id TEXT NOT NULL,
  amount_cents BIGINT NOT NULL,
  before_balance_cents BIGINT NOT NULL DEFAULT 0,
  after_balance_cents BIGINT NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'POSTED',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS xz_marketing_upgrade_plans (
  id TEXT PRIMARY KEY,
  from_role TEXT NOT NULL,
  to_role TEXT NOT NULL,
  price_cents BIGINT NOT NULL DEFAULT 0,
  condition_type TEXT NOT NULL DEFAULT 'PAID',
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS xz_marketing_upgrade_records (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  from_role TEXT NOT NULL,
  to_role TEXT NOT NULL,
  order_id TEXT,
  amount_cents BIGINT NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'PENDING',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS xz_marketing_commission_rules (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  order_type TEXT NOT NULL DEFAULT 'UPGRADE',
  earner_role TEXT NOT NULL,
  relation_depth INT NOT NULL DEFAULT 1,
  fixed_amount_cents BIGINT NOT NULL DEFAULT 0,
  rate NUMERIC NOT NULL DEFAULT 0,
  max_total_rate NUMERIC NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_xz_marketing_org_descendant ON xz_marketing_org_relations(descendant_user_id, depth);
CREATE INDEX IF NOT EXISTS idx_xz_marketing_invite_records_inviter ON xz_marketing_invite_records(inviter_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_marketing_wallet_records_user ON xz_marketing_wallet_records(user_id, created_at DESC);
