package channelrules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRolloutMigrationUsesTableSpecificRuleSetColumns(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "database", "migrations", "085-channel-rollout-config.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	for _, required := range []string{
		"TG_TABLE_NAME = 'xz_commission_rules'",
		"bound_rule_set_id := OLD.commercial_rule_set_id",
		"bound_rule_set_id := OLD.rule_set_id",
		"OLD.commercial_rule_set_id IS NOT NULL",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("085 migration is missing field mapping %q", required)
		}
	}
}
