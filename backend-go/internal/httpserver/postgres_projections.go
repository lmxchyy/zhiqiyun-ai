package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

const runtimeProjectionSchema = `
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
  buyer_user_id TEXT,
  plan_id TEXT,
  order_type TEXT,
  business_order_type TEXT,
  amount_cents BIGINT NOT NULL DEFAULT 0,
  token_amount BIGINT NOT NULL DEFAULT 0,
  token_grant_amount BIGINT NOT NULL DEFAULT 0,
  token_grant_value_cents BIGINT NOT NULL DEFAULT 0,
  platform_income_cents BIGINT NOT NULL DEFAULT 0,
  direct_agent_id TEXT,
  parent_agent_id TEXT,
  operation_center_id TEXT,
  fulfillment_status TEXT,
  fulfilled_at TEXT,
  status TEXT,
  paid_at TEXT,
  created_at TEXT,
  reward_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
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

CREATE TABLE IF NOT EXISTS xz_billing_events (
  id TEXT PRIMARY KEY,
  transaction_id TEXT,
  user_id TEXT,
  agent_id TEXT,
  tenant_id TEXT,
  operation_center_id TEXT,
  module_code TEXT,
  task_id TEXT,
  metric_code TEXT,
  quantity BIGINT NOT NULL DEFAULT 0,
  unit_amount_cents BIGINT NOT NULL DEFAULT 0,
  amount_cents BIGINT NOT NULL DEFAULT 0,
  point_cost BIGINT NOT NULL DEFAULT 0,
  balance_before BIGINT NOT NULL DEFAULT 0,
  balance_after BIGINT NOT NULL DEFAULT 0,
  model TEXT,
  status TEXT,
  occurred_at TEXT,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS xz_payment_events (
  id TEXT PRIMARY KEY,
  provider TEXT NOT NULL,
  event_id TEXT NOT NULL,
  order_id TEXT NOT NULL,
  transaction_id TEXT,
  amount_cents BIGINT NOT NULL DEFAULT 0,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb,
  verified BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(provider, event_id),
  UNIQUE(provider, transaction_id)
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

CREATE TABLE IF NOT EXISTS xz_token_records (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  order_id TEXT,
  change_type TEXT NOT NULL,
  amount BIGINT NOT NULL DEFAULT 0,
  balance_after BIGINT NOT NULL DEFAULT 0,
  remark TEXT,
  created_at TEXT,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb,
  UNIQUE(order_id, change_type)
);

CREATE TABLE IF NOT EXISTS xz_operation_centers (
  id TEXT PRIMARY KEY,
  user_id TEXT UNIQUE,
  name TEXT,
  region TEXT,
  invite_code TEXT UNIQUE,
  status TEXT,
  join_order_id TEXT,
  join_fee_cents BIGINT NOT NULL DEFAULT 0,
  approved_at TEXT,
  created_at TEXT,
  updated_at TEXT,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS xz_generation_tasks (
  id TEXT PRIMARY KEY,
  user_id TEXT,
  module_code TEXT,
  type TEXT,
  model TEXT,
  billing_type TEXT,
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
  deleted_at TIMESTAMPTZ,
  created_at TEXT,
  updated_at TEXT,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

ALTER TABLE xz_assets
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS xz_ai_state (
  user_id TEXT PRIMARY KEY,
  favorite_task_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  hidden_task_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  favorite_collections JSONB NOT NULL DEFAULT '[]'::jsonb,
  agent_conversations JSONB NOT NULL DEFAULT '[]'::jsonb,
  active_conversation_id TEXT,
  active_collection_id TEXT,
  updated_at TEXT,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS xz_system_settings (
  id TEXT PRIMARY KEY,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_at TEXT
);

CREATE TABLE IF NOT EXISTS xz_api_channels (
  id TEXT PRIMARY KEY,
  name TEXT,
  base_url TEXT,
  protocol TEXT,
  status TEXT,
  priority INT NOT NULL DEFAULT 0,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS xz_api_keys (
  id TEXT PRIMARY KEY,
  customer TEXT,
  prefix TEXT,
  status TEXT,
  quota_limit BIGINT NOT NULL DEFAULT 0,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS xz_user_model_routes (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  provider TEXT,
  channel_id TEXT,
  channel TEXT,
  api_key_id TEXT,
  key_prefix TEXT,
  group_name TEXT,
  models JSONB NOT NULL DEFAULT '[]'::jsonb,
  quota_limit BIGINT NOT NULL DEFAULT 0,
  quota_used BIGINT NOT NULL DEFAULT 0,
  status TEXT,
  updated_at TEXT,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_xz_users_role ON xz_users(role);
CREATE INDEX IF NOT EXISTS idx_xz_orders_user_id ON xz_orders(user_id);
CREATE INDEX IF NOT EXISTS idx_xz_orders_user_created ON xz_orders(user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_xz_orders_status ON xz_orders(status);
CREATE INDEX IF NOT EXISTS idx_xz_channel_agents_user_id ON xz_channel_agents(user_id);
CREATE INDEX IF NOT EXISTS idx_xz_channel_agents_parent_id ON xz_channel_agents(parent_id);
CREATE INDEX IF NOT EXISTS idx_xz_operation_centers_user_id ON xz_operation_centers(user_id);
CREATE INDEX IF NOT EXISTS idx_xz_token_records_user_id ON xz_token_records(user_id);
CREATE INDEX IF NOT EXISTS idx_xz_token_records_user_created ON xz_token_records(user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_xz_generation_tasks_user_id ON xz_generation_tasks(user_id);
CREATE INDEX IF NOT EXISTS idx_xz_generation_tasks_user_created ON xz_generation_tasks(user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_xz_generation_tasks_module_code ON xz_generation_tasks(module_code);
CREATE INDEX IF NOT EXISTS idx_xz_assets_user_id ON xz_assets(user_id);
CREATE INDEX IF NOT EXISTS idx_xz_assets_user_created ON xz_assets(user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_xz_assets_user_active_created ON xz_assets(user_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_xz_billing_events_module_code ON xz_billing_events(module_code);
CREATE INDEX IF NOT EXISTS idx_xz_billing_events_user_occurred ON xz_billing_events(user_id, occurred_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_xz_payment_events_order ON xz_payment_events(order_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_commissions_agent_created ON xz_commissions(agent_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_xz_withdrawals_agent_created ON xz_withdrawals(agent_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_xz_api_channels_status ON xz_api_channels(status);
CREATE INDEX IF NOT EXISTS idx_xz_user_model_routes_user_id ON xz_user_model_routes(user_id);
CREATE INDEX IF NOT EXISTS idx_xz_user_model_routes_status ON xz_user_model_routes(status);
`

func (b postgresStateBackend) syncRuntimeProjections(ctx context.Context, content []byte) error {
	var data adminPlatformData
	if err := json.Unmarshal(content, &data); err != nil {
		return err
	}

	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := upsertUsers(ctx, tx, data.Users); err != nil {
		return err
	}
	if err := upsertPlans(ctx, tx, data.Plans); err != nil {
		return err
	}
	if err := upsertPointAccounts(ctx, tx, data.PointAccounts); err != nil {
		return err
	}
	if err := upsertTokenRecords(ctx, tx, data.TokenRecords); err != nil {
		return err
	}
	if err := upsertOrders(ctx, tx, data.Orders); err != nil {
		return err
	}
	if err := upsertPaymentEvents(ctx, tx, data.PaymentEvents); err != nil {
		return err
	}
	if err := upsertChannelAgents(ctx, tx, data.ChannelAgents); err != nil {
		return err
	}
	if err := upsertOperationCenters(ctx, tx, data.OperationCenters); err != nil {
		return err
	}
	if err := upsertCommissions(ctx, tx, data.Commissions); err != nil {
		return err
	}
	if err := upsertWithdrawals(ctx, tx, data.Withdrawals); err != nil {
		return err
	}
	if err := upsertGenerationTasks(ctx, tx, data.GenerationTasks); err != nil {
		return err
	}
	if err := upsertAssets(ctx, tx, data.Assets); err != nil {
		return err
	}
	if err := upsertAPIChannels(ctx, tx, data.APIChannels); err != nil {
		return err
	}
	if err := upsertAPIKeys(ctx, tx, data.APIKeys); err != nil {
		return err
	}
	if err := upsertUserModelRoutesFromUsers(ctx, tx, data.Users); err != nil {
		return err
	}

	return tx.Commit()
}

func upsertUsers(ctx context.Context, tx *sql.Tx, items []adminUser) error {
	for _, item := range items {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO xz_users (id, email, name, role, status, password_hash, plan_id, referred_by, subscription_expires_at, created_at, updated_at, raw)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb)
			ON CONFLICT (id) DO UPDATE SET
				email = excluded.email,
				name = excluded.name,
				role = excluded.role,
				status = excluded.status,
				password_hash = excluded.password_hash,
				plan_id = excluded.plan_id,
				referred_by = excluded.referred_by,
				subscription_expires_at = excluded.subscription_expires_at,
				created_at = excluded.created_at,
				updated_at = excluded.updated_at,
				raw = excluded.raw
		`, item.ID, item.Email, item.Name, item.Role, item.Status, item.PasswordHash, item.PlanID, item.ReferredBy, item.SubscriptionExpiresAt, item.CreatedAt, item.UpdatedAt, jsonProjection(item))
		if err != nil {
			return fmt.Errorf("upsert xz_users %s: %w", item.ID, err)
		}
	}
	return nil
}

func upsertPlans(ctx context.Context, tx *sql.Tx, items []adminPlan) error {
	for _, item := range items {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO xz_plans (id, code, name, price_cents, grant_points, duration_days, concurrency, active, entitlements, raw)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb)
			ON CONFLICT (id) DO UPDATE SET
				code = excluded.code,
				name = excluded.name,
				price_cents = excluded.price_cents,
				grant_points = excluded.grant_points,
				duration_days = excluded.duration_days,
				concurrency = excluded.concurrency,
				active = excluded.active,
				entitlements = excluded.entitlements,
				raw = excluded.raw
		`, item.ID, item.Code, item.Name, planPriceCents(item), planGrantPoints(item), item.DurationDays, item.Concurrency, item.Active, jsonProjection(item.Entitlements), jsonProjection(item))
		if err != nil {
			return fmt.Errorf("upsert xz_plans %s: %w", item.ID, err)
		}
	}
	return nil
}

func upsertPointAccounts(ctx context.Context, tx *sql.Tx, items []adminPointAccount) error {
	for _, item := range items {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO xz_point_accounts (id, user_id, available, frozen, raw)
			VALUES ($1, $2, $3, $4, $5::jsonb)
			ON CONFLICT (id) DO UPDATE SET
				user_id = excluded.user_id,
				available = excluded.available,
				frozen = excluded.frozen,
				raw = excluded.raw
		`, item.ID, item.UserID, item.Available, item.Frozen, jsonProjection(item))
		if err != nil {
			return fmt.Errorf("upsert xz_point_accounts %s: %w", item.ID, err)
		}
		if err := upsertUserWalletFromPointAccount(ctx, tx, item); err != nil {
			return fmt.Errorf("upsert xz_user_wallets %s: %w", item.UserID, err)
		}
	}
	return nil
}

func upsertTokenRecords(ctx context.Context, tx *sql.Tx, items []adminTokenRecord) error {
	for _, item := range items {
		if err := insertTokenRecord(ctx, tx, item); err != nil {
			return fmt.Errorf("upsert xz_token_records %s: %w", item.ID, err)
		}
	}
	return nil
}

func upsertOrders(ctx context.Context, tx *sql.Tx, items []adminOrder) error {
	for _, item := range items {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO xz_orders (id, user_id, buyer_user_id, plan_id, order_type, business_order_type, amount_cents, token_amount, token_grant_amount, token_grant_value_cents, platform_income_cents, direct_agent_id, parent_agent_id, operation_center_id, fulfillment_status, fulfilled_at, status, paid_at, created_at, reward_snapshot, price_snapshot, raw)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20::jsonb, $21::jsonb, $22::jsonb)
			ON CONFLICT (id) DO UPDATE SET
				user_id = excluded.user_id,
				buyer_user_id = excluded.buyer_user_id,
				plan_id = excluded.plan_id,
				order_type = excluded.order_type,
				business_order_type = excluded.business_order_type,
				amount_cents = excluded.amount_cents,
				token_amount = excluded.token_amount,
				token_grant_amount = excluded.token_grant_amount,
				token_grant_value_cents = excluded.token_grant_value_cents,
				platform_income_cents = excluded.platform_income_cents,
				direct_agent_id = excluded.direct_agent_id,
				parent_agent_id = excluded.parent_agent_id,
				operation_center_id = excluded.operation_center_id,
				fulfillment_status = excluded.fulfillment_status,
				fulfilled_at = excluded.fulfilled_at,
				status = excluded.status,
				paid_at = excluded.paid_at,
				created_at = excluded.created_at,
				reward_snapshot = excluded.reward_snapshot,
				price_snapshot = excluded.price_snapshot,
				raw = excluded.raw
		`, item.ID, item.UserID, firstNonEmptyString(item.BuyerUserID, item.UserID), item.PlanID, item.OrderType, businessOrderTypeFromOrder(item), orderAmount(item), firstNonEmptyInt(item.TokenAmount, item.TokenGrantAmount), item.TokenGrantAmount, intValue(item.PriceSnapshot["tokenGrantValueCents"]), item.PlatformIncomeCents, item.DirectAgentID, item.ParentAgentID, item.OperationCenterID, item.FulfillmentStatus, item.FulfilledAt, item.Status, item.PaidAt, item.CreatedAt, jsonProjection(item.RewardSnapshot), jsonProjection(item.PriceSnapshot), jsonProjection(item))
		if err != nil {
			return fmt.Errorf("upsert xz_orders %s: %w", item.ID, err)
		}
	}
	return nil
}

func upsertChannelAgents(ctx context.Context, tx *sql.Tx, items []adminChannelAgent) error {
	for _, item := range items {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO xz_channel_agents (id, user_id, parent_id, level, status, invite_code, created_at, updated_at, raw)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb)
			ON CONFLICT (id) DO UPDATE SET
				user_id = excluded.user_id,
				parent_id = excluded.parent_id,
				level = excluded.level,
				status = excluded.status,
				invite_code = excluded.invite_code,
				created_at = excluded.created_at,
				updated_at = excluded.updated_at,
				raw = excluded.raw
		`, item.ID, item.UserID, item.ParentID, item.Level, item.Status, item.InviteCode, item.CreatedAt, item.UpdatedAt, jsonProjection(item))
		if err != nil {
			return fmt.Errorf("upsert xz_channel_agents %s: %w", item.ID, err)
		}
		if err := upsertAgentProfileFromChannelAgent(ctx, tx, item); err != nil {
			return fmt.Errorf("upsert xz_agent_profiles %s: %w", item.ID, err)
		}
	}
	return nil
}

func upsertOperationCenters(ctx context.Context, tx *sql.Tx, items []adminOperationCenter) error {
	for _, item := range items {
		if err := insertOperationCenter(ctx, tx, item); err != nil {
			return fmt.Errorf("upsert xz_operation_centers %s: %w", item.ID, err)
		}
	}
	return nil
}

func upsertCommissions(ctx context.Context, tx *sql.Tx, items []adminCommission) error {
	for _, item := range items {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO xz_commissions (id, order_id, agent_id, amount_cents, rate, status, rule_snapshot, created_at, raw)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9::jsonb)
			ON CONFLICT (id) DO UPDATE SET
				order_id = excluded.order_id,
				agent_id = excluded.agent_id,
				amount_cents = excluded.amount_cents,
				rate = excluded.rate,
				status = excluded.status,
				rule_snapshot = excluded.rule_snapshot,
				created_at = excluded.created_at,
				raw = excluded.raw
		`, item.ID, item.OrderID, item.AgentID, item.AmountCents, item.Rate, item.Status, jsonProjection(item.RuleSnapshot), item.CreatedAt, jsonProjection(item))
		if err != nil {
			return fmt.Errorf("upsert xz_commissions %s: %w", item.ID, err)
		}
		if err := refreshAgentWallet(ctx, tx, item.AgentID); err != nil {
			return fmt.Errorf("refresh xz_agent_wallets %s: %w", item.AgentID, err)
		}
	}
	return nil
}

func upsertWithdrawals(ctx context.Context, tx *sql.Tx, items []adminWithdrawal) error {
	for _, item := range items {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO xz_withdrawals (id, agent_id, amount_cents, status, created_at, reviewed_at, raw)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
			ON CONFLICT (id) DO UPDATE SET
				agent_id = excluded.agent_id,
				amount_cents = excluded.amount_cents,
				status = excluded.status,
				created_at = excluded.created_at,
				reviewed_at = excluded.reviewed_at,
				raw = excluded.raw
		`, item.ID, item.AgentID, item.AmountCents, item.Status, item.CreatedAt, item.ReviewedAt, jsonProjection(item))
		if err != nil {
			return fmt.Errorf("upsert xz_withdrawals %s: %w", item.ID, err)
		}
		if err := refreshAgentWallet(ctx, tx, item.AgentID); err != nil {
			return fmt.Errorf("refresh xz_agent_wallets %s: %w", item.AgentID, err)
		}
	}
	return nil
}

func upsertPaymentEvents(ctx context.Context, tx *sql.Tx, items []adminPaymentEvent) error {
	for _, item := range items {
		item = normalizePaymentCallbackEvent(item)
		_, err := tx.ExecContext(ctx, `
			INSERT INTO xz_payment_events (id, provider, event_id, order_id, transaction_id, amount_cents, raw, verified, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9::timestamptz)
			ON CONFLICT (id) DO UPDATE SET
				provider = excluded.provider,
				event_id = excluded.event_id,
				order_id = excluded.order_id,
				transaction_id = excluded.transaction_id,
				amount_cents = excluded.amount_cents,
				raw = excluded.raw,
				verified = excluded.verified,
				created_at = excluded.created_at
		`, item.ID, item.Provider, item.EventID, item.OrderID, nullableSQLString(item.TransactionID), item.AmountCents, jsonProjection(item), item.Verified, item.CreatedAt)
		if err != nil {
			return fmt.Errorf("upsert xz_payment_events %s: %w", item.ID, err)
		}
	}
	return nil
}

func upsertGenerationTasks(ctx context.Context, tx *sql.Tx, items []generationTask) error {
	for _, item := range items {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO xz_generation_tasks (id, user_id, type, model, status, progress, point_cost, prompt, params, result_ids, error, created_at, updated_at, worker_finished_at, raw)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb, $11::jsonb, $12, $13, $14, $15::jsonb)
			ON CONFLICT (id) DO UPDATE SET
				user_id = excluded.user_id,
				type = excluded.type,
				model = excluded.model,
				status = excluded.status,
				progress = excluded.progress,
				point_cost = excluded.point_cost,
				prompt = excluded.prompt,
				params = excluded.params,
				result_ids = excluded.result_ids,
				error = excluded.error,
				created_at = excluded.created_at,
				updated_at = excluded.updated_at,
				worker_finished_at = excluded.worker_finished_at,
				raw = excluded.raw
		`, item.ID, item.UserID, item.Type, item.Model, item.Status, item.Progress, item.PointCost, item.Prompt, jsonProjection(item.Params), jsonProjection(item.ResultIDs), jsonProjection(item.Error), item.CreatedAt, item.UpdatedAt, item.WorkerFinishedAt, jsonProjection(item))
		if err != nil {
			return fmt.Errorf("upsert xz_generation_tasks %s: %w", item.ID, err)
		}
	}
	return nil
}

func upsertAssets(ctx context.Context, tx *sql.Tx, items []asset) error {
	for _, item := range items {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO xz_assets (id, user_id, task_id, name, media_type, url, thumbnail_url, favorite, metadata, deleted_at, created_at, updated_at, raw)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11, $12, $13::jsonb)
			ON CONFLICT (id) DO UPDATE SET
				user_id = excluded.user_id,
				task_id = excluded.task_id,
				name = excluded.name,
				media_type = excluded.media_type,
				url = excluded.url,
				thumbnail_url = excluded.thumbnail_url,
				favorite = excluded.favorite,
				metadata = excluded.metadata,
				deleted_at = excluded.deleted_at,
				created_at = excluded.created_at,
				updated_at = excluded.updated_at,
				raw = excluded.raw
		`, item.ID, item.UserID, item.TaskID, item.Name, item.MediaType, item.URL, item.ThumbnailURL, item.Favorite, jsonProjection(item.Metadata), nullableSQLString(item.DeletedAt), item.CreatedAt, item.UpdatedAt, jsonProjection(item))
		if err != nil {
			return fmt.Errorf("upsert xz_assets %s: %w", item.ID, err)
		}
	}
	return nil
}

func upsertAPIChannels(ctx context.Context, tx *sql.Tx, items []adminAPIChannel) error {
	for _, item := range items {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO xz_api_channels (id, name, base_url, protocol, status, priority, raw)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
			ON CONFLICT (id) DO UPDATE SET
				name = excluded.name,
				base_url = excluded.base_url,
				protocol = excluded.protocol,
				status = excluded.status,
				priority = excluded.priority,
				raw = excluded.raw
		`, item.ID, item.Name, item.BaseURL, item.Protocol, item.Status, item.Priority, jsonProjection(item))
		if err != nil {
			return fmt.Errorf("upsert xz_api_channels %s: %w", item.ID, err)
		}
	}
	return nil
}

func upsertAPIKeys(ctx context.Context, tx *sql.Tx, items []adminAPIKey) error {
	for _, item := range items {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO xz_api_keys (id, customer, prefix, status, quota_limit, raw)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb)
			ON CONFLICT (id) DO UPDATE SET
				customer = excluded.customer,
				prefix = excluded.prefix,
				status = excluded.status,
				quota_limit = excluded.quota_limit,
				raw = excluded.raw
		`, item.ID, item.Customer, item.Prefix, item.Status, item.QuotaLimit, jsonProjection(item))
		if err != nil {
			return fmt.Errorf("upsert xz_api_keys %s: %w", item.ID, err)
		}
	}
	return nil
}

func upsertUserModelRoutesFromUsers(ctx context.Context, tx *sql.Tx, users []adminUser) error {
	for _, user := range users {
		for _, route := range user.ModelRoutes {
			if route.ID == "" {
				continue
			}
			if err := insertUserModelRoute(ctx, tx, user.ID, route); err != nil {
				return fmt.Errorf("upsert xz_user_model_routes %s: %w", route.ID, err)
			}
		}
	}
	return nil
}

func jsonProjection(value any) string {
	if value == nil {
		return "null"
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(raw)
}

func nullableSQLString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func planPriceCents(plan adminPlan) int {
	if plan.PriceCents > 0 {
		return plan.PriceCents
	}
	return plan.Price
}

func planGrantPoints(plan adminPlan) int {
	if plan.GrantPoints > 0 {
		return plan.GrantPoints
	}
	return plan.Points
}
