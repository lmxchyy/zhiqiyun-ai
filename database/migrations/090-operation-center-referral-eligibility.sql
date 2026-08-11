-- Operation-center referral eligibility baseline.
-- This migration records qualification only. It does not create rewards, wallet entries or release tasks.

BEGIN;

CREATE TABLE IF NOT EXISTS xz_referral_eligibilities (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  referral_event_id TEXT NOT NULL REFERENCES xz_referral_events(id),
  commercial_rule_set_id TEXT NOT NULL REFERENCES xz_commercial_rule_sets(id),
  commercial_rule_set_version INT NOT NULL CHECK (commercial_rule_set_version > 0),
  referral_rule_version_id TEXT NOT NULL REFERENCES xz_referral_reward_rule_versions(id),
  referral_rule_version INT NOT NULL CHECK (referral_rule_version > 0),
  beneficiary_type TEXT NOT NULL CHECK (beneficiary_type IN ('AGENT', 'OPERATION_CENTER')),
  beneficiary_user_id TEXT NOT NULL REFERENCES xz_users(id),
  beneficiary_relation TEXT NOT NULL CHECK (beneficiary_relation IN ('REFERRER', 'REFERRER_OPERATION_CENTER')),
  relationship_snapshot JSONB NOT NULL CHECK (jsonb_typeof(relationship_snapshot) = 'object'),
  eligibility_status TEXT NOT NULL DEFAULT 'ELIGIBLE' CHECK (eligibility_status IN ('ELIGIBLE', 'CONSUMED', 'CANCELLED')),
  idempotency_key TEXT NOT NULL UNIQUE CHECK (btrim(idempotency_key) <> ''),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (referral_event_id, referral_rule_version_id, beneficiary_user_id)
);

CREATE INDEX IF NOT EXISTS idx_xz_referral_eligibilities_beneficiary_090
  ON xz_referral_eligibilities(tenant_id, beneficiary_type, beneficiary_user_id, eligibility_status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xz_referral_eligibilities_event_090
  ON xz_referral_eligibilities(referral_event_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_xz_referral_eligibilities_rule_090
  ON xz_referral_eligibilities(commercial_rule_set_id, referral_rule_version_id, created_at DESC);

CREATE OR REPLACE FUNCTION xz_protect_referral_eligibility_identity_090()
RETURNS trigger AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'referral eligibility history cannot be deleted';
  END IF;
  IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
     OR NEW.referral_event_id IS DISTINCT FROM OLD.referral_event_id
     OR NEW.commercial_rule_set_id IS DISTINCT FROM OLD.commercial_rule_set_id
     OR NEW.commercial_rule_set_version IS DISTINCT FROM OLD.commercial_rule_set_version
     OR NEW.referral_rule_version_id IS DISTINCT FROM OLD.referral_rule_version_id
     OR NEW.referral_rule_version IS DISTINCT FROM OLD.referral_rule_version
     OR NEW.beneficiary_type IS DISTINCT FROM OLD.beneficiary_type
     OR NEW.beneficiary_user_id IS DISTINCT FROM OLD.beneficiary_user_id
     OR NEW.beneficiary_relation IS DISTINCT FROM OLD.beneficiary_relation
     OR NEW.relationship_snapshot IS DISTINCT FROM OLD.relationship_snapshot
     OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'referral eligibility identity and historical snapshots are immutable';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_xz_referral_eligibility_identity_immutable_090 ON xz_referral_eligibilities;
CREATE TRIGGER trg_xz_referral_eligibility_identity_immutable_090
BEFORE UPDATE OR DELETE ON xz_referral_eligibilities
FOR EACH ROW EXECUTE FUNCTION xz_protect_referral_eligibility_identity_090();

COMMIT;
