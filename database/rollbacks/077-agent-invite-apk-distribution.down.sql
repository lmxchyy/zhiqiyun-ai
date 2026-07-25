BEGIN;

DROP TABLE IF EXISTS xz_apk_download_events;
DROP INDEX IF EXISTS ux_xz_app_releases_one_published;
DROP INDEX IF EXISTS idx_xz_app_releases_lookup;
DROP TABLE IF EXISTS xz_app_releases;
DROP INDEX IF EXISTS ux_xz_agent_invite_events_request;
DROP INDEX IF EXISTS idx_xz_agent_invite_events_funnel;
DROP INDEX IF EXISTS idx_xz_agent_invite_events_user;
DROP TABLE IF EXISTS xz_agent_invite_events;
DROP INDEX IF EXISTS ux_xz_marketing_invite_codes_agent;
ALTER TABLE IF EXISTS xz_marketing_invite_codes
  DROP COLUMN IF EXISTS agent_id,
  DROP COLUMN IF EXISTS poster_url,
  DROP COLUMN IF EXISTS activity_intro;

-- The unique index, secure-format constraint and generated invite codes protect
-- permanent public links and are retained deliberately during feature rollback.

COMMIT;
