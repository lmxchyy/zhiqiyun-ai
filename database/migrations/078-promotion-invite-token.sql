-- Opaque promotion invite tokens. Existing invite codes and relationship tables remain authoritative.
BEGIN;

ALTER TABLE xz_marketing_invite_codes
  ADD COLUMN IF NOT EXISTS invite_token TEXT,
  ADD COLUMN IF NOT EXISTS identity_type TEXT NOT NULL DEFAULT 'USER';

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_marketing_invite_codes_token
  ON xz_marketing_invite_codes(lower(invite_token))
  WHERE nullif(btrim(invite_token), '') IS NOT NULL;

COMMIT;
