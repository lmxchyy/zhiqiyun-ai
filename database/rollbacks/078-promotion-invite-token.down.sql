BEGIN;

DROP INDEX IF EXISTS ux_xz_marketing_invite_codes_token;
ALTER TABLE IF EXISTS xz_marketing_invite_codes
  DROP COLUMN IF EXISTS invite_token,
  DROP COLUMN IF EXISTS identity_type;

COMMIT;
