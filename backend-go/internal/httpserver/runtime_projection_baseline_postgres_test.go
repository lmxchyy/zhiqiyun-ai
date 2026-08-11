package httpserver

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestRuntimeProjectionBaselineCompletePostgres(t *testing.T) {
	dsn := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	expected := []string{
		"xz_users", "xz_auth_account_merge_requests", "xz_plans", "xz_point_accounts", "xz_orders",
		"xz_channel_agents", "xz_commissions", "xz_billing_events", "xz_payment_events", "xz_withdrawals",
		"xz_token_records", "xz_operation_centers", "xz_generation_tasks", "xz_assets", "xz_ai_state",
		"xz_system_settings", "xz_api_channels", "xz_api_keys", "xz_user_model_routes",
	}
	for _, table := range expected {
		var relation sql.NullString
		if err := db.QueryRowContext(context.Background(), `SELECT to_regclass('public.' || $1)::text`, table).Scan(&relation); err != nil {
			t.Fatalf("check runtime projection %s: %v", table, err)
		}
		if !relation.Valid || relation.String == "" {
			t.Errorf("runtime projection table %s is missing from the SQL baseline", table)
		}
	}
}
