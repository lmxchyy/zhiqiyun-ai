CREATE TABLE IF NOT EXISTS xz_customer_relations (
  id TEXT PRIMARY KEY,
  customer_user_id TEXT NOT NULL REFERENCES xz_users(id),
  direct_agent_id TEXT REFERENCES xz_channel_agents(id),
  parent_agent_id TEXT REFERENCES xz_channel_agents(id),
  operation_center_id TEXT REFERENCES xz_operation_centers(id),
  bind_type TEXT NOT NULL DEFAULT 'ADMIN_ASSIGNMENT',
  bind_start_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  bind_end_at TIMESTAMPTZ,
  status TEXT NOT NULL DEFAULT 'ACTIVE'
    CHECK (status IN ('ACTIVE', 'SUPERSEDED', 'DISABLED')),
  reason TEXT NOT NULL DEFAULT '',
  actor_user_id TEXT REFERENCES xz_users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_customer_relations_current
  ON xz_customer_relations(customer_user_id)
  WHERE status = 'ACTIVE' AND bind_end_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_xz_customer_relations_agent
  ON xz_customer_relations(direct_agent_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_xz_customer_relations_operation_center
  ON xz_customer_relations(operation_center_id, status, created_at DESC);
