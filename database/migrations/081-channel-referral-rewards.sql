-- Channel ecosystem V1.3.2: referral rewards are independent marketing expenses.

BEGIN;

CREATE TABLE IF NOT EXISTS xz_referral_reward_rule_versions (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  rule_set_id TEXT NOT NULL REFERENCES xz_commercial_rule_sets(id),
  rule_code TEXT NOT NULL,
  version INT NOT NULL CHECK (version > 0),
  referrer_type TEXT NOT NULL CHECK (referrer_type IN ('AGENT', 'OPERATION_CENTER')),
  beneficiary_type TEXT NOT NULL CHECK (beneficiary_type IN ('AGENT', 'OPERATION_CENTER')),
  beneficiary_relation TEXT NOT NULL CHECK (beneficiary_relation IN ('REFERRER', 'REFERRER_OPERATION_CENTER')),
  amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
  freeze_days INT NOT NULL DEFAULT 7 CHECK (freeze_days >= 0),
  status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'PUBLISHED', 'RETIRED', 'ARCHIVED')),
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, rule_code, version)
);

CREATE INDEX IF NOT EXISTS idx_xz_referral_reward_rule_versions_lookup
  ON xz_referral_reward_rule_versions(rule_set_id, referrer_type, status);

CREATE TABLE IF NOT EXISTS xz_referral_events (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  referred_operation_center_user_id TEXT NOT NULL REFERENCES xz_users(id),
  referrer_type TEXT NOT NULL CHECK (referrer_type IN ('AGENT', 'OPERATION_CENTER')),
  referrer_user_id TEXT NOT NULL REFERENCES xz_users(id),
  referrer_operation_center_user_id TEXT,
  source_order_id TEXT NOT NULL REFERENCES xz_orders(id),
  source_order_no TEXT NOT NULL,
  payment_status_snapshot TEXT NOT NULL,
  review_status_snapshot TEXT NOT NULL,
  operation_center_status_snapshot TEXT NOT NULL,
  relationship_snapshot JSONB NOT NULL,
  triggered_at TIMESTAMPTZ NOT NULL,
  status TEXT NOT NULL DEFAULT 'READY' CHECK (status IN ('READY', 'REWARDED', 'REVERSED', 'CANCELLED')),
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (source_order_id)
);

CREATE INDEX IF NOT EXISTS idx_xz_referral_events_referred_oc
  ON xz_referral_events(tenant_id, referred_operation_center_user_id, status, triggered_at DESC);

CREATE TABLE IF NOT EXISTS xz_referral_rewards (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  referral_event_id TEXT NOT NULL REFERENCES xz_referral_events(id),
  reward_rule_id TEXT NOT NULL REFERENCES xz_referral_reward_rule_versions(id),
  beneficiary_type TEXT NOT NULL CHECK (beneficiary_type IN ('AGENT', 'OPERATION_CENTER')),
  beneficiary_user_id TEXT NOT NULL REFERENCES xz_users(id),
  amount_cents BIGINT NOT NULL CHECK (amount_cents <> 0),
  record_type TEXT NOT NULL CHECK (record_type IN ('REWARD', 'REVERSAL')),
  status TEXT NOT NULL CHECK (status IN ('FROZEN', 'AVAILABLE', 'SETTLED', 'REVERSED', 'CANCELLED')),
  freeze_until TIMESTAMPTZ,
  reversal_of_id TEXT REFERENCES xz_referral_rewards(id),
  idempotency_key TEXT NOT NULL UNIQUE,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (
    (record_type = 'REWARD' AND amount_cents > 0 AND reversal_of_id IS NULL) OR
    (record_type = 'REVERSAL' AND amount_cents < 0 AND reversal_of_id IS NOT NULL)
  )
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_referral_rewards_reward
  ON xz_referral_rewards(referral_event_id, reward_rule_id, beneficiary_user_id)
  WHERE record_type = 'REWARD';
CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_referral_rewards_reversal
  ON xz_referral_rewards(reversal_of_id)
  WHERE record_type = 'REVERSAL';
CREATE INDEX IF NOT EXISTS idx_xz_referral_rewards_beneficiary
  ON xz_referral_rewards(tenant_id, beneficiary_type, beneficiary_user_id, status, freeze_until);

COMMIT;
