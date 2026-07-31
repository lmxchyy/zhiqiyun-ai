-- Keep the shared category on rollback: it may have existed before this migration,
-- and deleting catalog data cannot be made reliably reversible without provenance.

DROP INDEX IF EXISTS idx_inspiration_templates_scenario;

ALTER TABLE inspiration_templates
  DROP COLUMN IF EXISTS preset_config_json,
  DROP COLUMN IF EXISTS input_requirements_json,
  DROP COLUMN IF EXISTS display_config_json,
  DROP COLUMN IF EXISTS scenario_code;
