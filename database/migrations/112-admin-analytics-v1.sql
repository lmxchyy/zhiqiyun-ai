-- V1 analytics query indexes. Runtime projection timestamps are ISO text; the
-- plain timestamp-leading indexes still support the bounded source scans.
CREATE INDEX IF NOT EXISTS idx_xz_generation_tasks_analytics_time
  ON xz_generation_tasks (created_at, type, status, model);

CREATE INDEX IF NOT EXISTS idx_model_call_logs_analytics_dimensions
  ON model_call_logs (created_at, provider_code, model_code, status);

CREATE INDEX IF NOT EXISTS idx_xz_billing_events_analytics_time
  ON xz_billing_events (occurred_at, module_code, model, status);
