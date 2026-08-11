-- Operation-center referral reward reversal contract.
-- This migration does not advance refund tasks or call payment providers.

BEGIN;

ALTER TABLE xz_referral_rewards
  ADD COLUMN IF NOT EXISTS reversal_amount_cents BIGINT,
  ADD COLUMN IF NOT EXISTS source_reward_status TEXT,
  ADD COLUMN IF NOT EXISTS reversal_type TEXT,
  ADD COLUMN IF NOT EXISTS transaction_group_id TEXT;

UPDATE xz_referral_rewards reversal
SET reversal_amount_cents=COALESCE(reversal.reversal_amount_cents,abs(reversal.amount_cents)),
    source_reward_status=COALESCE(reversal.source_reward_status,reversal.metadata->>'sourceRewardStatus','SETTLED'),
    reversal_type=COALESCE(reversal.reversal_type,reversal.metadata->>'reversalType','SETTLED_RECOVERABLE'),
    transaction_group_id=COALESCE(reversal.transaction_group_id,reversal.metadata->>'transactionGroupId','legacy-reversal-'||reversal.id),
    updated_at=reversal.updated_at
WHERE reversal.record_type='REVERSAL'
  AND reversal.refund_task_id IS NOT NULL
  AND (
    reversal.reversal_amount_cents IS NULL
    OR reversal.source_reward_status IS NULL
    OR reversal.reversal_type IS NULL
    OR reversal.transaction_group_id IS NULL
  );

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='xz_referral_rewards'::regclass
      AND conname='ck_xz_referral_rewards_reversal_contract_093'
  ) THEN
    ALTER TABLE xz_referral_rewards
      ADD CONSTRAINT ck_xz_referral_rewards_reversal_contract_093 CHECK (
        record_type<>'REVERSAL'
        OR refund_task_id IS NULL
        OR (
          reversal_of_id IS NOT NULL
          AND reversal_amount_cents=abs(amount_cents)
          AND reversal_amount_cents>0
          AND source_reward_status IN ('FROZEN','AVAILABLE','SETTLED')
          AND reversal_type IN ('FROZEN_DEBIT','AVAILABLE_DEBIT','AVAILABLE_RECOVERABLE','SETTLED_RECOVERABLE')
          AND transaction_group_id IS NOT NULL
          AND btrim(transaction_group_id)<>''
          AND referral_event_id IS NOT NULL
          AND referral_eligibility_id IS NOT NULL
          AND commercial_rule_set_id IS NOT NULL
        )
      );
  END IF;
END;
$$;

CREATE INDEX IF NOT EXISTS idx_xz_referral_rewards_refund_reversal_093
  ON xz_referral_rewards(refund_task_id, reversal_of_id, created_at, id)
  WHERE record_type='REVERSAL' AND refund_task_id IS NOT NULL;

ALTER TABLE xz_commission_wallet_ledger
  ADD COLUMN IF NOT EXISTS original_referral_reward_id TEXT REFERENCES xz_referral_rewards(id),
  ADD COLUMN IF NOT EXISTS original_grant_ledger_id TEXT REFERENCES xz_commission_wallet_ledger(id),
  ADD COLUMN IF NOT EXISTS original_release_ledger_id TEXT REFERENCES xz_commission_wallet_ledger(id),
  ADD COLUMN IF NOT EXISTS transaction_group_id TEXT;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='xz_commission_wallet_ledger'::regclass
      AND conname='ck_xz_commission_wallet_referral_reversal_093'
  ) THEN
    ALTER TABLE xz_commission_wallet_ledger
      ADD CONSTRAINT ck_xz_commission_wallet_referral_reversal_093 CHECK (
        business_type<>'REFERRAL_REWARD_REVERSAL'
        OR (
          direction='DEBIT'
          AND frozen_delta_cents<=0
          AND available_delta_cents<=0
          AND settled_delta_cents=0
          AND recoverable_delta_cents>=0
          AND recoverable_cents_delta=recoverable_delta_cents
          AND (-frozen_delta_cents)+(-available_delta_cents)+recoverable_cents_delta>0
          AND referral_reward_id IS NOT NULL
          AND original_referral_reward_id IS NOT NULL
          AND original_grant_ledger_id IS NOT NULL
          AND refund_task_id IS NOT NULL
          AND referral_event_id IS NOT NULL
          AND referral_eligibility_id IS NOT NULL
          AND commercial_rule_set_id IS NOT NULL
          AND transaction_group_id IS NOT NULL
          AND btrim(transaction_group_id)<>''
        )
      );
  END IF;
END;
$$;

CREATE UNIQUE INDEX IF NOT EXISTS ux_xz_commission_wallet_referral_reversal_093
  ON xz_commission_wallet_ledger(refund_task_id,original_referral_reward_id)
  WHERE business_type='REFERRAL_REWARD_REVERSAL'
    AND refund_task_id IS NOT NULL
    AND original_referral_reward_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_xz_commission_wallet_reversal_group_093
  ON xz_commission_wallet_ledger(transaction_group_id,created_at,id)
  WHERE business_type='REFERRAL_REWARD_REVERSAL';

ALTER TABLE xz_referral_reward_release_tasks
  ADD COLUMN IF NOT EXISTS cancellation_reason TEXT,
  ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;

UPDATE xz_referral_reward_release_tasks
SET cancellation_reason=COALESCE(cancellation_reason,failure_detail->>'cancellationReason','LEGACY_CANCELLED'),
    cancelled_at=COALESCE(cancelled_at,completed_at,updated_at,created_at),
    completed_at=COALESCE(completed_at,cancelled_at,updated_at,created_at),
    updated_at=updated_at
WHERE release_status='CANCELLED'
  AND (cancellation_reason IS NULL OR cancelled_at IS NULL OR completed_at IS NULL);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='xz_referral_reward_release_tasks'::regclass
      AND conname='ck_xz_referral_release_cancelled_093'
  ) THEN
    ALTER TABLE xz_referral_reward_release_tasks
      ADD CONSTRAINT ck_xz_referral_release_cancelled_093 CHECK (
        release_status<>'CANCELLED'
        OR (
          cancellation_reason IS NOT NULL
          AND btrim(cancellation_reason)<>''
          AND cancelled_at IS NOT NULL
          AND completed_at IS NOT NULL
        )
      );
  END IF;
END;
$$;

CREATE OR REPLACE FUNCTION xz_protect_referral_wallet_grant_091()
RETURNS trigger AS $$
BEGIN
  IF OLD.business_type IN ('REFERRAL_REWARD_GRANT','REFERRAL_REWARD_RELEASE','REFERRAL_REWARD_REVERSAL') THEN
    RAISE EXCEPTION 'referral reward wallet ledger is append-only';
  END IF;
  IF TG_OP='DELETE' THEN
    RETURN OLD;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMIT;
