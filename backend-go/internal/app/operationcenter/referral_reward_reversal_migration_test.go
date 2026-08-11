package operationcenter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReferralRewardReversalMigration093StaticContract(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "database", "migrations", "093-operation-center-referral-reward-reversal.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(raw)
	required := []string{
		"ADD COLUMN IF NOT EXISTS reversal_amount_cents",
		"ADD COLUMN IF NOT EXISTS source_reward_status",
		"ADD COLUMN IF NOT EXISTS reversal_type",
		"ADD COLUMN IF NOT EXISTS original_referral_reward_id",
		"ADD COLUMN IF NOT EXISTS original_grant_ledger_id",
		"ADD COLUMN IF NOT EXISTS transaction_group_id",
		"ADD COLUMN IF NOT EXISTS cancellation_reason",
		"ux_xz_commission_wallet_referral_reversal_093",
		"ck_xz_referral_release_cancelled_093",
		"REFERRAL_REWARD_REVERSAL",
		"append-only",
	}
	for _, fragment := range required {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("093 migration missing %q", fragment)
		}
	}
}
