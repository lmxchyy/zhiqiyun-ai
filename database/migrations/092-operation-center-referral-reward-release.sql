-- Operation-center referral reward release ledger contract.
-- Refund reversal and recoverable balance orchestration remain out of scope.

BEGIN;

ALTER TABLE xz_commission_wallet_ledger
  ADD COLUMN IF NOT EXISTS referral_release_task_id TEXT;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='xz_commission_wallet_ledger'::regclass
      AND conname='fk_xz_commission_wallet_release_task_092'
  ) THEN
    ALTER TABLE xz_commission_wallet_ledger
      ADD CONSTRAINT fk_xz_commission_wallet_release_task_092
      FOREIGN KEY (referral_release_task_id) REFERENCES xz_referral_reward_release_tasks(id);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='xz_commission_wallet_ledger'::regclass
      AND conname='ck_xz_commission_wallet_referral_release_092'
  ) THEN
    ALTER TABLE xz_commission_wallet_ledger
      ADD CONSTRAINT ck_xz_commission_wallet_referral_release_092 CHECK (
        business_type<>'REFERRAL_REWARD_RELEASE'
        OR (
          direction='TRANSFER'
          AND frozen_delta_cents<0
          AND available_delta_cents>0
          AND frozen_delta_cents+available_delta_cents=0
          AND expected_delta_cents=0
          AND settling_delta_cents=0
          AND settled_delta_cents=0
          AND recoverable_delta_cents=0
          AND recoverable_cents_delta=0
          AND referral_reward_id IS NOT NULL
          AND referral_event_id IS NOT NULL
          AND referral_eligibility_id IS NOT NULL
          AND original_ledger_id IS NOT NULL
          AND commercial_rule_set_id IS NOT NULL
          AND referral_release_task_id IS NOT NULL
        )
      );
  END IF;
END;
$$;

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_commission_wallet_referral_release_092
  ON xz_commission_wallet_ledger(referral_release_task_id)
  WHERE business_type='REFERRAL_REWARD_RELEASE'
    AND referral_release_task_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_xz_commission_wallet_release_reward_092
  ON xz_commission_wallet_ledger(referral_reward_id, referral_release_task_id, created_at, id)
  WHERE business_type='REFERRAL_REWARD_RELEASE';

CREATE OR REPLACE FUNCTION xz_protect_referral_wallet_grant_091()
RETURNS trigger AS $$
BEGIN
  IF OLD.business_type IN ('REFERRAL_REWARD_GRANT','REFERRAL_REWARD_RELEASE') THEN
    RAISE EXCEPTION 'referral reward wallet ledger is append-only';
  END IF;
  IF TG_OP='DELETE' THEN
    RETURN OLD;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMIT;
