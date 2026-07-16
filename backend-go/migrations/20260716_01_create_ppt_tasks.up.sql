CREATE TABLE IF NOT EXISTS xz_ppt_tasks (
  task_id VARCHAR(128) PRIMARY KEY,
  user_id VARCHAR(128) NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  raw JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_xz_ppt_tasks_user_created
  ON xz_ppt_tasks(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_xz_ppt_tasks_user_status
  ON xz_ppt_tasks(user_id, status);
