-- Historical registration gifts are permanent under migration 110.
-- Clear only the expiry metadata that was written before that invariant.
-- Amounts, balances, status, timestamps, snapshots, and ledger history remain
-- unchanged. The predicate makes a retry a no-op after the repair.
BEGIN;

UPDATE xz_personal_point_lots
SET expires_at = NULL,
    policy_version_id = NULL
WHERE source_type = 'REGISTRATION_GIFT'
  AND (expires_at IS NOT NULL OR policy_version_id IS NOT NULL);

COMMIT;
