package operationcenter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReferralRewardGrantMigration091StaticContract(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "database", "migrations", "091-operation-center-referral-reward-grant.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(raw)
	required := []string{
		"ADD COLUMN IF NOT EXISTS consumed_at", "ADD COLUMN IF NOT EXISTS reward_id",
		"ADD COLUMN IF NOT EXISTS referral_eligibility_id", "ADD COLUMN IF NOT EXISTS reward_rule_version",
		"ADD COLUMN IF NOT EXISTS referral_event_id", "ADD COLUMN IF NOT EXISTS commercial_rule_set_id",
		"ADD COLUMN IF NOT EXISTS execute_at", "ux_xz_referral_rewards_eligibility_091",
		"ux_xz_commission_wallet_referral_grant_091", "ck_xz_referral_eligibilities_consumption_091",
		"REWARDED", "append-only",
	}
	for _, fragment := range required {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("091 migration missing %q", fragment)
		}
	}
}
