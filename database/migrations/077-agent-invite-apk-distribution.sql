-- Agent invitation registration and Android APK distribution, phase 1.
-- Reuses xz_channel_agents, xz_user_relationships and the existing commission chain.

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Keep already compliant codes stable. Repair missing, short, malformed or
-- duplicate legacy values once before public permanent links are issued.
CREATE TABLE IF NOT EXISTS xz_agent_invite_code_migrations (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL UNIQUE REFERENCES xz_channel_agents(id),
  old_code TEXT,
  new_code TEXT NOT NULL UNIQUE,
  reason TEXT NOT NULL,
  migrated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

WITH ranked AS (
  SELECT id, invite_code, created_at,
         row_number() OVER (
           PARTITION BY upper(btrim(invite_code))
           ORDER BY created_at NULLS LAST, id
         ) AS duplicate_rank
  FROM xz_channel_agents
)
INSERT INTO xz_agent_invite_code_migrations(id, agent_id, old_code, new_code, reason)
SELECT
  'invite_code_migration_' || substr(md5(ranked.id), 1, 20),
  ranked.id,
  nullif(btrim(ranked.invite_code), ''),
  upper(encode(gen_random_bytes(6), 'hex')),
  CASE
    WHEN nullif(btrim(ranked.invite_code), '') IS NULL THEN 'missing'
    WHEN ranked.duplicate_rank > 1 THEN 'duplicate'
    ELSE 'insecure_format'
  END
FROM ranked
WHERE nullif(btrim(ranked.invite_code), '') IS NULL
   OR ranked.duplicate_rank > 1
   OR ranked.invite_code !~ '^[A-Z0-9]{8,12}$'
ON CONFLICT (agent_id) DO NOTHING;

UPDATE xz_channel_agents agent
SET invite_code = migration.new_code,
    updated_at = now()::text
FROM xz_agent_invite_code_migrations migration
WHERE agent.id = migration.agent_id
  AND agent.invite_code IS DISTINCT FROM migration.new_code;

UPDATE xz_channel_agents
SET raw = jsonb_set(coalesce(raw, '{}'::jsonb), '{inviteCode}', to_jsonb(invite_code), true)
WHERE coalesce(raw->>'inviteCode', '') IS DISTINCT FROM invite_code;

UPDATE xz_agent_profiles profile
SET invite_code = agent.invite_code,
    raw = jsonb_set(coalesce(profile.raw, '{}'::jsonb), '{inviteCode}', to_jsonb(agent.invite_code), true),
    updated_at = now()
FROM xz_channel_agents agent
WHERE profile.id = agent.id
  AND profile.invite_code IS DISTINCT FROM agent.invite_code;

ALTER TABLE xz_channel_agents
  DROP CONSTRAINT IF EXISTS ck_xz_channel_agents_invite_code_format;
ALTER TABLE xz_channel_agents
  ADD CONSTRAINT ck_xz_channel_agents_invite_code_format
  CHECK (invite_code ~ '^[A-Z0-9]{8,12}$');

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_channel_agents_invite_code
  ON xz_channel_agents (upper(btrim(invite_code)))
  WHERE nullif(btrim(invite_code), '') IS NOT NULL;

ALTER TABLE xz_marketing_invite_codes
  ADD COLUMN IF NOT EXISTS agent_id TEXT REFERENCES xz_channel_agents(id),
  ADD COLUMN IF NOT EXISTS poster_url TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS activity_intro TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_marketing_invite_codes_agent
  ON xz_marketing_invite_codes(agent_id)
  WHERE agent_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS xz_agent_invite_events (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL DEFAULT 'tenant_default',
  inviter_user_id TEXT NOT NULL REFERENCES xz_users(id),
  agent_id TEXT NOT NULL REFERENCES xz_channel_agents(id),
  invite_code TEXT NOT NULL,
  user_id TEXT REFERENCES xz_users(id),
  event_type TEXT NOT NULL CHECK (
    event_type IN ('page_view', 'sms_sent', 'sms_verified', 'registered', 'apk_downloaded', 'app_activated')
  ),
  source TEXT NOT NULL DEFAULT 'h5',
  request_key_hash TEXT,
  release_id TEXT,
  client_family TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_agent_invite_events_request
  ON xz_agent_invite_events(event_type, request_key_hash)
  WHERE request_key_hash IS NOT NULL AND request_key_hash <> '';
CREATE INDEX IF NOT EXISTS idx_xz_agent_invite_events_funnel
  ON xz_agent_invite_events(agent_id, event_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_agent_invite_events_user
  ON xz_agent_invite_events(user_id, created_at DESC)
  WHERE user_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS xz_app_releases (
  id TEXT PRIMARY KEY,
  platform TEXT NOT NULL,
  channel TEXT NOT NULL,
  version_name TEXT NOT NULL,
  version_code BIGINT NOT NULL CHECK (version_code > 0),
  apk_url TEXT NOT NULL,
  file_size BIGINT NOT NULL DEFAULT 0 CHECK (file_size >= 0),
  sha256 TEXT NOT NULL,
  release_notes TEXT NOT NULL DEFAULT '',
  min_supported_version_code BIGINT NOT NULL DEFAULT 0 CHECK (min_supported_version_code >= 0),
  force_update BOOLEAN NOT NULL DEFAULT FALSE,
  status TEXT NOT NULL CHECK (status IN ('draft', 'testing', 'published', 'disabled')),
  published_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (platform <> 'android' OR lower(apk_url) LIKE '%.apk%'),
  CHECK (platform <> 'android' OR position(version_name in apk_url) > 0),
  UNIQUE(platform, channel, version_code),
  UNIQUE(platform, channel, version_name)
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_app_releases_one_published
  ON xz_app_releases(platform, channel)
  WHERE status = 'published';
CREATE INDEX IF NOT EXISTS idx_xz_app_releases_lookup
  ON xz_app_releases(platform, channel, status, published_at DESC, version_code DESC);

CREATE TABLE IF NOT EXISTS xz_apk_download_events (
  id TEXT PRIMARY KEY,
  release_id TEXT NOT NULL REFERENCES xz_app_releases(id),
  agent_id TEXT REFERENCES xz_channel_agents(id),
  user_id TEXT REFERENCES xz_users(id),
  invite_event_id TEXT REFERENCES xz_agent_invite_events(id),
  channel TEXT NOT NULL DEFAULT 'official',
  client_family TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_xz_apk_download_events_release
  ON xz_apk_download_events(release_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_apk_download_events_agent
  ON xz_apk_download_events(agent_id, created_at DESC)
  WHERE agent_id IS NOT NULL;

COMMIT;
