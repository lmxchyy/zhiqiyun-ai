package operationcenter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReferralRewardReleaseMigration092StaticContract(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "database", "migrations", "092-operation-center-referral-reward-release.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(raw)
	required := []string{
		"ADD COLUMN IF NOT EXISTS referral_release_task_id",
		"fk_xz_commission_wallet_release_task_092",
		"ck_xz_commission_wallet_referral_release_092",
		"ux_xz_commission_wallet_referral_release_092",
		"business_type='REFERRAL_REWARD_RELEASE'",
		"frozen_delta_cents+available_delta_cents=0",
		"recoverable_cents_delta=0",
		"append-only",
	}
	for _, fragment := range required {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("092 migration missing %q", fragment)
		}
	}
}
