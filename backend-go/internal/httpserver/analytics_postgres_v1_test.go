package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestAnalyticsPostgresV1AggregatesControlledFixture(t *testing.T) {
	dsn := os.Getenv("XIANZHI_ANALYTICS_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://codex:codex@127.0.0.1:55441/xianzhi_test?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("analytics PostgreSQL test database unavailable: %v", err)
	}
	var ready bool
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('public.xz_billing_events') IS NOT NULL AND to_regclass('public.model_call_logs') IS NOT NULL AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='xz_generation_tasks' AND column_name='supplier_cost')`).Scan(&ready); err != nil || !ready {
		t.Skip("analytics PostgreSQL prerequisites are not migrated")
	}

	suffix := fmt.Sprintf("analytics_%d", time.Now().UnixNano())
	now := time.Now().UTC().Format(time.RFC3339)
	userID := "user_" + suffix
	taskImage := "task_image_" + suffix
	taskVideo := "task_video_" + suffix
	eventID := "event_" + suffix
	tokenID := "token_" + suffix
	providerA := "provider-a-" + suffix
	providerB := "provider-b-" + suffix
	modelFixture := "analytics-model-" + suffix
	// Keep repeated local runs deterministic without touching non-fixture data.
	if _, err := db.ExecContext(ctx, `
		DELETE FROM xz_generation_tasks WHERE user_id IN (SELECT id FROM xz_users WHERE email LIKE 'user_analytics_%@test.invalid');
		DELETE FROM xz_token_records WHERE user_id IN (SELECT id FROM xz_users WHERE email LIKE 'user_analytics_%@test.invalid');
		DELETE FROM xz_billing_events WHERE user_id IN (SELECT id FROM xz_users WHERE email LIKE 'user_analytics_%@test.invalid');
		DELETE FROM xz_users WHERE email LIKE 'user_analytics_%@test.invalid';
		DELETE FROM model_call_logs WHERE provider_code LIKE 'provider-a-analytics_%' OR provider_code LIKE 'provider-b-analytics_%';
	`); err != nil {
		t.Fatal(err)
	}
	store := newPostgresPrimaryStore(db, "")
	baseline, err := store.AnalyticsOverview(ctx, AnalyticsQueryParams{Days: 1, Timezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatal(err)
	}
	baselineTrends, err := store.AnalyticsTrends(ctx, AnalyticsQueryParams{Days: 1, Timezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users(id,email,name,role,status,created_at) VALUES($1,$1||'@test.invalid',$1,'MEMBER','ACTIVE',$2),($3,$3||'@test.invalid',$3,'MEMBER','ACTIVE',$2)`, userID, now, userID+"_b"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_generation_tasks(id,user_id,type,model,status,created_at,supplier_cost) VALUES($1,$2,'TEXT_TO_IMAGE','gpt-image-2','SUCCEEDED',$3,40),($4,$5,'TEXT_TO_VIDEO','video-model','FAILED',$3,0)`, taskImage, userID, now, taskVideo, userID+"_b"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_billing_events(id,user_id,point_cost,amount_cents,metric_code,occurred_at,status) VALUES($1,$2,80,1000,'RECHARGE',$3,'SUCCEEDED')`, eventID, userID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_token_records(id,user_id,change_type,amount,created_at) VALUES($1,$2,'CONSUME',-120,$3)`, tokenID, userID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO model_call_logs(provider_code,model_code,status,latency_ms,cost_cents,created_at) VALUES($1,$4,'SUCCESS',100,40,$3),($2,$5,'FAILED',200,20,$3)`, providerA, providerB, now, modelFixture, modelFixture+"-failed"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM model_call_logs WHERE provider_code IN ($1,$2)`, providerA, providerB)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM xz_token_records WHERE id=$1`, tokenID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM xz_billing_events WHERE id=$1`, eventID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM xz_generation_tasks WHERE id IN ($1,$2)`, taskImage, taskVideo)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM xz_users WHERE id IN ($1,$2)`, userID, userID+"_b")
	})

	overview, err := store.AnalyticsOverview(ctx, AnalyticsQueryParams{Days: 1, Timezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatal(err)
	}
	if overview.NewUsersToday != baseline.NewUsersToday+2 || overview.ImagesGenerated != baseline.ImagesGenerated+1 || overview.PointsConsumed != baseline.PointsConsumed+80 || overview.TokensUsed != baseline.TokensUsed+120 || overview.FailedTasksToday != baseline.FailedTasksToday+1 {
		t.Fatalf("unexpected overview: %+v", overview)
	}
	trends, err := store.AnalyticsTrends(ctx, AnalyticsQueryParams{Days: 1, Timezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02")
	metricForToday := func(items []DailyMetric) float64 {
		for _, item := range items {
			if item.Date == today {
				return item.Value
			}
		}
		return 0
	}
	if metricForToday(trends.NewUsers) != metricForToday(baselineTrends.NewUsers)+2 || metricForToday(trends.Points) != metricForToday(baselineTrends.Points)+80 {
		t.Fatalf("unexpected trends: %+v", trends)
	}
	models, err := store.AnalyticsModels(ctx, AnalyticsQueryParams{Days: 1, Timezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatal(err)
	}
	if len(models.Models) < 2 || models.Models[0].ModelCode != modelFixture || models.Models[0].SuccessRate != 100 {
		t.Fatalf("unexpected models: %+v", models)
	}
	providers, err := store.AnalyticsProviders(ctx, AnalyticsQueryParams{Days: 1, Timezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatal(err)
	}
	if len(providers.Providers) != 2 || providers.Providers[0].ProviderCode != providerA || providers.Providers[0].SuccessRate != 100 {
		t.Fatalf("unexpected providers: %+v", providers)
	}
}
