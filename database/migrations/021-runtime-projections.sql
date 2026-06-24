CREATE TABLE IF NOT EXISTS xz_users (
  id TEXT PRIMARY KEY,
  email TEXT UNIQUE,
  name TEXT,
  role TEXT,
  status TEXT,
  password_hash TEXT,
  plan_id TEXT,
  referred_by TEXT,
  subscription_expires_at TEXT,
  created_at TEXT,
  updated_at TEXT,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS xz_plans (
  id TEXT PRIMARY KEY,
  code TEXT,
  name TEXT,
  price_cents BIGINT NOT NULL DEFAULT 0,
  grant_points BIGINT NOT NULL DEFAULT 0,
  duration_days INT NOT NULL DEFAULT 0,
  concurrency INT NOT NULL DEFAULT 0,
  active BOOLEAN NOT NULL DEFAULT FALSE,
  entitlements JSONB NOT NULL DEFAULT '{}'::jsonb,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS xz_point_accounts (
  id TEXT PRIMARY KEY,
  user_id TEXT,
  available BIGINT NOT NULL DEFAULT 0,
  frozen BIGINT NOT NULL DEFAULT 0,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS xz_orders (
  id TEXT PRIMARY KEY,
  user_id TEXT,
  plan_id TEXT,
  amount_cents BIGINT NOT NULL DEFAULT 0,
  status TEXT,
  paid_at TEXT,
  created_at TEXT,
  price_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS xz_channel_agents (
  id TEXT PRIMARY KEY,
  user_id TEXT,
  parent_id TEXT,
  level INT NOT NULL DEFAULT 0,
  status TEXT,
  invite_code TEXT,
  created_at TEXT,
  updated_at TEXT,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS xz_commissions (
  id TEXT PRIMARY KEY,
  order_id TEXT,
  agent_id TEXT,
  amount_cents BIGINT NOT NULL DEFAULT 0,
  rate NUMERIC NOT NULL DEFAULT 0,
  status TEXT,
  rule_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TEXT,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS xz_withdrawals (
  id TEXT PRIMARY KEY,
  agent_id TEXT,
  amount_cents BIGINT NOT NULL DEFAULT 0,
  status TEXT,
  created_at TEXT,
  reviewed_at TEXT,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS xz_generation_tasks (
  id TEXT PRIMARY KEY,
  user_id TEXT,
  type TEXT,
  model TEXT,
  status TEXT,
  progress INT NOT NULL DEFAULT 0,
  point_cost BIGINT NOT NULL DEFAULT 0,
  prompt TEXT,
  params JSONB NOT NULL DEFAULT '{}'::jsonb,
  result_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  error JSONB NOT NULL DEFAULT 'null'::jsonb,
  created_at TEXT,
  updated_at TEXT,
  worker_finished_at TEXT,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS xz_assets (
  id TEXT PRIMARY KEY,
  user_id TEXT,
  task_id TEXT,
  name TEXT,
  media_type TEXT,
  url TEXT,
  thumbnail_url TEXT,
  favorite BOOLEAN NOT NULL DEFAULT FALSE,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TEXT,
  updated_at TEXT,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_xz_users_role ON xz_users(role);
CREATE INDEX IF NOT EXISTS idx_xz_orders_user_id ON xz_orders(user_id);
CREATE INDEX IF NOT EXISTS idx_xz_orders_status ON xz_orders(status);
CREATE INDEX IF NOT EXISTS idx_xz_channel_agents_user_id ON xz_channel_agents(user_id);
CREATE INDEX IF NOT EXISTS idx_xz_generation_tasks_user_id ON xz_generation_tasks(user_id);
CREATE INDEX IF NOT EXISTS idx_xz_assets_user_id ON xz_assets(user_id);
