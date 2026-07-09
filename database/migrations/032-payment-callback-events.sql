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

CREATE INDEX IF NOT EXISTS idx_xz_payment_events_order
  ON xz_payment_events(order_id, created_at DESC);
