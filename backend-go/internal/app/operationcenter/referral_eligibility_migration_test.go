package operationcenter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReferralEligibilityMigration090StaticContract(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "database", "migrations", "090-operation-center-referral-eligibility.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(raw)
	required := []string{
		"CREATE TABLE IF NOT EXISTS xz_referral_eligibilities",
		"referral_event_id TEXT NOT NULL REFERENCES xz_referral_events(id)",
		"commercial_rule_set_id TEXT NOT NULL REFERENCES xz_commercial_rule_sets(id)",
		"referral_rule_version_id TEXT NOT NULL REFERENCES xz_referral_reward_rule_versions(id)",
		"beneficiary_user_id TEXT NOT NULL REFERENCES xz_users(id)",
		"relationship_snapshot JSONB NOT NULL",
		"idempotency_key TEXT NOT NULL UNIQUE",
		"UNIQUE (referral_event_id, referral_rule_version_id, beneficiary_user_id)",
		"xz_protect_referral_eligibility_identity_090",
	}
	for _, fragment := range required {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("090 migration missing %q", fragment)
		}
	}
	if strings.Contains(sqlText, "INSERT INTO xz_referral_rewards") || strings.Contains(sqlText, "xz_commission_wallet_ledger") {
		t.Fatal("090 eligibility migration must not create rewards or wallet entries")
	}
}
