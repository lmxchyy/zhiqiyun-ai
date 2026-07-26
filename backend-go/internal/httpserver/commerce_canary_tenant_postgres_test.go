package httpserver

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"xianzhi-ai/backend-go/internal/app/channelrules"
)

func TestCanaryTenantWhitelistRoundTripPostgres(t *testing.T) {
	dsn := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		UPDATE xz_channel_rollout_configs
		SET mode='CANARY',real_switch_enabled=TRUE,percentage_rollout_enabled=FALSE,
		    canary_basis_points=0,allow_tenant_ids='["tenant_default"]'::jsonb,
		    allow_order_ids='[]'::jsonb,allow_user_ids='[]'::jsonb,allow_plan_ids='[]'::jsonb,
		    change_reason='tenant whitelist integration',updated_by='test'
		WHERE tenant_id='tenant_default'
	`); err != nil {
		t.Fatal(err)
	}
	config, found, err := loadChannelRolloutConfigTx(ctx, tx, "tenant_default")
	if err != nil {
		t.Fatal(err)
	}
	if !found || len(config.AllowTenantIDs) != 1 || config.AllowTenantIDs[0] != "tenant_default" || config.PercentageRolloutEnabled {
		t.Fatalf("tenant whitelist config mismatch found=%v config=%+v", found, config)
	}
	decision, err := channelrules.EvaluateRollout(config, channelrules.RolloutSubject{
		TenantID: "tenant_default", OrderID: "tenant_whitelist_order", UserID: "tenant_whitelist_user", PlanID: "plan_ai_creator_996",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.UseV132Settlement || decision.Reason != "ALLOW_LIST" {
		t.Fatalf("tenant whitelist must select V1.3.2: %+v", decision)
	}
}
