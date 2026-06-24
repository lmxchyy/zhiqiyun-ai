package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

CREATE INDEX IF NOT EXISTS idx_xz_users_role ON xz_users(role);
CREATE INDEX IF NOT EXISTS idx_xz_orders_user_id ON xz_orders(user_id);
CREATE INDEX IF NOT EXISTS idx_xz_orders_status ON xz_orders(status);
CREATE INDEX IF NOT EXISTS idx_xz_channel_agents_user_id ON xz_channel_agents(user_id);
CREATE INDEX IF NOT EXISTS idx_xz_generation_tasks_user_id ON xz_generation_tasks(user_id);
CREATE INDEX IF NOT EXISTS idx_xz_assets_user_id ON xz_assets(user_id);
CREATE INDEX IF NOT EXISTS idx_xz_api_channels_status ON xz_api_channels(status);
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
	if err := upsertOrders(ctx, tx, data.Orders); err != nil {
		return err
	}
	if err := upsertChannelAgents(ctx, tx, data.ChannelAgents); err != nil {
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
	}
	return nil
}

func upsertOrders(ctx context.Context, tx *sql.Tx, items []adminOrder) error {
	for _, item := range items {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO xz_orders (id, user_id, plan_id, amount_cents, status, paid_at, created_at, price_snapshot, raw)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::jsonb)
			ON CONFLICT (id) DO UPDATE SET
				user_id = excluded.user_id,
				plan_id = excluded.plan_id,
				amount_cents = excluded.amount_cents,
				status = excluded.status,
				paid_at = excluded.paid_at,
				created_at = excluded.created_at,
				price_snapshot = excluded.price_snapshot,
				raw = excluded.raw
		`, item.ID, item.UserID, item.PlanID, orderAmount(item), item.Status, item.PaidAt, item.CreatedAt, jsonProjection(item.PriceSnapshot), jsonProjection(item))
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
			INSERT INTO xz_assets (id, user_id, task_id, name, media_type, url, thumbnail_url, favorite, metadata, created_at, updated_at, raw)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11, $12::jsonb)
			ON CONFLICT (id) DO UPDATE SET
				user_id = excluded.user_id,
				task_id = excluded.task_id,
				name = excluded.name,
				media_type = excluded.media_type,
				url = excluded.url,
				thumbnail_url = excluded.thumbnail_url,
				favorite = excluded.favorite,
				metadata = excluded.metadata,
				created_at = excluded.created_at,
				updated_at = excluded.updated_at,
				raw = excluded.raw
		`, item.ID, item.UserID, item.TaskID, item.Name, item.MediaType, item.URL, item.ThumbnailURL, item.Favorite, jsonProjection(item.Metadata), item.CreatedAt, item.UpdatedAt, jsonProjection(item))
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
