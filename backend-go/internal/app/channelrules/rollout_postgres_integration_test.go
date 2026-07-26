package channelrules

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPinnedRuleVersionAndProtectionPostgres(t *testing.T) {
	dsn := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	var pinnedID string
	var pinnedVersion int
	if err := db.QueryRowContext(ctx, `
		SELECT pinned_rule_set_id,pinned_rule_set_version
		FROM xz_channel_rollout_configs
		WHERE tenant_id='tenant_default' AND mode='SHADOW' AND enabled=TRUE
	`).Scan(&pinnedID, &pinnedVersion); err != nil {
		t.Fatal(err)
	}
	if pinnedID != "channel_rules_v132_default_v1" || pinnedVersion != 1 {
		t.Fatalf("unexpected pinned rule: %s v%d", pinnedID, pinnedVersion)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewTransactionStore(tx)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := store.LoadShadowRuleBundle(ctx, RuleBundleQuery{
		TenantID: "tenant_default", PlanID: "plan_ai_creator_996",
		BusinessTime: time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.RuleSet.ID != pinnedID || bundle.RuleSet.Version != pinnedVersion {
		t.Fatalf("shadow loaded %s v%d, want %s v%d", bundle.RuleSet.ID, bundle.RuleSet.Version, pinnedID, pinnedVersion)
	}
	_ = tx.Rollback()

	assertPinnedValueUpdateBlocked(t, db, `
		UPDATE xz_commercial_rule_sets
		SET name=name || ' changed'
		WHERE id='channel_rules_v132_default_v1'
	`)
	assertPinnedValueUpdateBlocked(t, db, `
		UPDATE xz_commercial_plan_versions
		SET price_cents=price_cents+1
		WHERE rule_set_id='channel_rules_v132_default_v1' AND plan_id='plan_ai_creator_996'
	`)
	assertPinnedValueUpdateBlocked(t, db, `
		UPDATE xz_commission_rules
		SET fixed_amount_cents=fixed_amount_cents+1
		WHERE commercial_rule_set_id='channel_rules_v132_default_v1'
		  AND calculation_type='FIXED_AMOUNT'
	`)
	assertPinnedValueUpdateBlocked(t, db, `
		UPDATE xz_referral_reward_rule_versions
		SET amount_cents=amount_cents+1
		WHERE rule_set_id='channel_rules_v132_default_v1'
	`)

	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE xz_commission_rules SET status=status,updated_at=now()
		WHERE commercial_rule_set_id='channel_rules_v132_default_v1'
	`); err != nil {
		_ = tx.Rollback()
		t.Fatalf("status-only transition should remain available: %v", err)
	}
	_ = tx.Rollback()
}

func assertPinnedValueUpdateBlocked(t *testing.T, db *sql.DB, statement string) {
	t.Helper()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, execErr := tx.Exec(statement)
	_ = tx.Rollback()
	if execErr == nil || !strings.Contains(execErr.Error(), "pinned channel rule values are immutable") {
		t.Fatalf("expected pinned rule protection, got %v", execErr)
	}
}

func TestRolloutMigrationColumnConsistencyPostgres(t *testing.T) {
	dsn := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	checks := []struct {
		table  string
		column string
	}{
		{"xz_commission_rules", "commercial_rule_set_id"},
		{"xz_commercial_plan_versions", "rule_set_id"},
		{"xz_referral_reward_rule_versions", "rule_set_id"},
	}
	for _, check := range checks {
		var exists bool
		if err := db.QueryRow(`
			SELECT EXISTS (
			  SELECT 1 FROM information_schema.columns
			  WHERE table_schema=current_schema() AND table_name=$1 AND column_name=$2
			)
		`, check.table, check.column).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("missing %s.%s", check.table, check.column)
		}
	}
}
