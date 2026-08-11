-- Operation-center referral reward grant and frozen commission-wallet credit.
-- Reward release execution and refund reversal are intentionally out of scope.

BEGIN;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conrelid='xz_referral_events'::regclass
      AND contype='c'
      AND position('REWARDED' IN pg_get_constraintdef(oid))>0
  ) THEN
    RAISE EXCEPTION 'xz_referral_events must support REWARDED before applying reward grants';
  END IF;
END;
$$;

ALTER TABLE xz_referral_eligibilities
  ADD COLUMN IF NOT EXISTS consumed_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS reward_id TEXT;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='xz_referral_eligibilities'::regclass
      AND conname='fk_xz_referral_eligibilities_reward_091'
  ) THEN
    ALTER TABLE xz_referral_eligibilities
      ADD CONSTRAINT fk_xz_referral_eligibilities_reward_091
      FOREIGN KEY (reward_id) REFERENCES xz_referral_rewards(id);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='xz_referral_eligibilities'::regclass
      AND conname='ck_xz_referral_eligibilities_consumption_091'
  ) THEN
    ALTER TABLE xz_referral_eligibilities
      ADD CONSTRAINT ck_xz_referral_eligibilities_consumption_091 CHECK (
        (eligibility_status='CONSUMED' AND consumed_at IS NOT NULL AND reward_id IS NOT NULL)
        OR
        (eligibility_status<>'CONSUMED' AND consumed_at IS NULL AND reward_id IS NULL)
      );
  END IF;
END;
$$;

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_referral_eligibilities_reward_091
  ON xz_referral_eligibilities(reward_id)
  WHERE reward_id IS NOT NULL;

ALTER TABLE xz_referral_rewards
  ADD COLUMN IF NOT EXISTS referral_eligibility_id TEXT,
  ADD COLUMN IF NOT EXISTS reward_rule_version INT,
  ADD COLUMN IF NOT EXISTS beneficiary_relation TEXT,
  ADD COLUMN IF NOT EXISTS relationship_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE xz_referral_rewards reward
SET reward_rule_version=COALESCE(reward.reward_rule_version, rule_version.version),
    beneficiary_relation=COALESCE(reward.beneficiary_relation, rule_version.beneficiary_relation),
    relationship_snapshot=CASE
      WHEN reward.relationship_snapshot='{}'::jsonb THEN event.relationship_snapshot
      ELSE reward.relationship_snapshot
    END,
    updated_at=reward.updated_at
FROM xz_referral_reward_rule_versions rule_version,
     xz_referral_events event
WHERE reward.reward_rule_id=rule_version.id
  AND reward.referral_event_id=event.id
  AND (
    reward.reward_rule_version IS NULL
    OR reward.beneficiary_relation IS NULL
    OR reward.relationship_snapshot='{}'::jsonb
  );

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='xz_referral_rewards'::regclass
      AND conname='fk_xz_referral_rewards_eligibility_091'
  ) THEN
    ALTER TABLE xz_referral_rewards
      ADD CONSTRAINT fk_xz_referral_rewards_eligibility_091
      FOREIGN KEY (referral_eligibility_id) REFERENCES xz_referral_eligibilities(id);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='xz_referral_rewards'::regclass
      AND conname='ck_xz_referral_rewards_grant_snapshot_091'
  ) THEN
    ALTER TABLE xz_referral_rewards
      ADD CONSTRAINT ck_xz_referral_rewards_grant_snapshot_091 CHECK (
        jsonb_typeof(relationship_snapshot)='object'
        AND (
          referral_eligibility_id IS NULL
          OR (
            reward_rule_version IS NOT NULL AND reward_rule_version>0
            AND beneficiary_relation IN ('REFERRER','REFERRER_OPERATION_CENTER')
            AND commercial_rule_set_id IS NOT NULL
          )
        )
      );
  END IF;
END;
$$;

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_referral_rewards_eligibility_091
  ON xz_referral_rewards(referral_eligibility_id)
  WHERE referral_eligibility_id IS NOT NULL AND record_type='REWARD';

ALTER TABLE xz_commission_wallet_ledger
  ADD COLUMN IF NOT EXISTS referral_event_id TEXT REFERENCES xz_referral_events(id),
  ADD COLUMN IF NOT EXISTS referral_eligibility_id TEXT REFERENCES xz_referral_eligibilities(id),
  ADD COLUMN IF NOT EXISTS commercial_rule_set_id TEXT REFERENCES xz_commercial_rule_sets(id);

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_commission_wallet_referral_grant_091
  ON xz_commission_wallet_ledger(referral_reward_id)
  WHERE business_type='REFERRAL_REWARD_GRANT'
    AND direction='CREDIT'
    AND referral_reward_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_xz_commission_wallet_referral_event_091
  ON xz_commission_wallet_ledger(referral_event_id, referral_eligibility_id, created_at, id)
  WHERE referral_event_id IS NOT NULL;

ALTER TABLE xz_referral_reward_release_tasks
  ADD COLUMN IF NOT EXISTS execute_at TIMESTAMPTZ;

UPDATE xz_referral_reward_release_tasks task
SET execute_at=COALESCE(task.execute_at, reward.freeze_until, task.next_retry_at, task.created_at),
    updated_at=task.updated_at
FROM xz_referral_rewards reward
WHERE task.referral_reward_id=reward.id
  AND task.execute_at IS NULL;

ALTER TABLE xz_referral_reward_release_tasks
  ALTER COLUMN execute_at SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_xz_referral_release_tasks_execute_091
  ON xz_referral_reward_release_tasks(release_status, execute_at, id)
  WHERE release_status IN ('PENDING','FAILED');

CREATE OR REPLACE FUNCTION xz_validate_referral_eligibility_consumption_091()
RETURNS trigger AS $$
BEGIN
  IF OLD.eligibility_status IN ('CONSUMED','CANCELLED')
     AND NEW.eligibility_status IS DISTINCT FROM OLD.eligibility_status THEN
    RAISE EXCEPTION 'terminal referral eligibility cannot transition';
  END IF;
  IF NEW.eligibility_status='CONSUMED'
     AND (NEW.consumed_at IS NULL OR NEW.reward_id IS NULL) THEN
    RAISE EXCEPTION 'consumed referral eligibility requires reward and consumed time';
  END IF;
  IF NEW.eligibility_status<>'CONSUMED'
     AND (NEW.consumed_at IS NOT NULL OR NEW.reward_id IS NOT NULL) THEN
    RAISE EXCEPTION 'unconsumed referral eligibility cannot reference reward';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_referral_eligibility_consumption_091 ON xz_referral_eligibilities;
CREATE TRIGGER trg_xz_referral_eligibility_consumption_091
BEFORE UPDATE ON xz_referral_eligibilities
FOR EACH ROW EXECUTE FUNCTION xz_validate_referral_eligibility_consumption_091();

CREATE OR REPLACE FUNCTION xz_protect_referral_reward_grant_identity_091()
RETURNS trigger AS $$
BEGIN
  IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
     OR NEW.referral_event_id IS DISTINCT FROM OLD.referral_event_id
     OR NEW.referral_eligibility_id IS DISTINCT FROM OLD.referral_eligibility_id
     OR NEW.reward_rule_id IS DISTINCT FROM OLD.reward_rule_id
     OR NEW.reward_rule_version IS DISTINCT FROM OLD.reward_rule_version
     OR NEW.commercial_rule_set_id IS DISTINCT FROM OLD.commercial_rule_set_id
     OR NEW.beneficiary_type IS DISTINCT FROM OLD.beneficiary_type
     OR NEW.beneficiary_user_id IS DISTINCT FROM OLD.beneficiary_user_id
     OR NEW.beneficiary_relation IS DISTINCT FROM OLD.beneficiary_relation
     OR NEW.amount_cents IS DISTINCT FROM OLD.amount_cents
     OR NEW.record_type IS DISTINCT FROM OLD.record_type
     OR NEW.freeze_until IS DISTINCT FROM OLD.freeze_until
     OR NEW.relationship_snapshot IS DISTINCT FROM OLD.relationship_snapshot
     OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'referral reward grant identity and snapshots are immutable';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_referral_reward_grant_identity_091 ON xz_referral_rewards;
CREATE TRIGGER trg_xz_referral_reward_grant_identity_091
BEFORE UPDATE ON xz_referral_rewards
FOR EACH ROW
WHEN (OLD.record_type='REWARD' AND OLD.referral_eligibility_id IS NOT NULL)
EXECUTE FUNCTION xz_protect_referral_reward_grant_identity_091();

CREATE OR REPLACE FUNCTION xz_protect_referral_release_schedule_091()
RETURNS trigger AS $$
BEGIN
  IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
     OR NEW.referral_reward_id IS DISTINCT FROM OLD.referral_reward_id
     OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
     OR NEW.execute_at IS DISTINCT FROM OLD.execute_at THEN
    RAISE EXCEPTION 'referral reward release identity and schedule are immutable';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_referral_release_schedule_091 ON xz_referral_reward_release_tasks;
CREATE TRIGGER trg_xz_referral_release_schedule_091
BEFORE UPDATE ON xz_referral_reward_release_tasks
FOR EACH ROW EXECUTE FUNCTION xz_protect_referral_release_schedule_091();

CREATE OR REPLACE FUNCTION xz_protect_referral_wallet_grant_091()
RETURNS trigger AS $$
BEGIN
  IF OLD.business_type='REFERRAL_REWARD_GRANT' THEN
    RAISE EXCEPTION 'referral reward grant wallet ledger is append-only';
  END IF;
  RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_referral_wallet_grant_append_only_091 ON xz_commission_wallet_ledger;
CREATE TRIGGER trg_xz_referral_wallet_grant_append_only_091
BEFORE UPDATE OR DELETE ON xz_commission_wallet_ledger
FOR EACH ROW EXECUTE FUNCTION xz_protect_referral_wallet_grant_091();

COMMIT;
