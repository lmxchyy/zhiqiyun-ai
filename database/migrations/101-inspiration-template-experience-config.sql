ALTER TABLE inspiration_templates
  ADD COLUMN IF NOT EXISTS scenario_code varchar(64) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS display_config_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS input_requirements_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS preset_config_json jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_inspiration_templates_scenario
  ON inspiration_templates (tenant_id, scenario_code, status, updated_at DESC)
  WHERE deleted_at IS NULL AND scenario_code <> '';

INSERT INTO inspiration_categories (id, tenant_id, code, name, sort_order, status)
VALUES ('inspiration-category-image-enhancement', 'default', 'image-enhancement', 'AI图像增强', 75, 'ACTIVE')
ON CONFLICT (id) DO UPDATE SET
  code = EXCLUDED.code,
  name = EXCLUDED.name,
  sort_order = EXCLUDED.sort_order,
  status = EXCLUDED.status,
  updated_at = now();
