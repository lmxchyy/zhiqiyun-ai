package canarypreflight

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"xianzhi-ai/backend-go/internal/config"
)

func TestTEST_O_PreflightRejectsMissingMigrations(t *testing.T) {
	err := ValidateRequiredMigrations([]string{RequiredMigrations[0], RequiredMigrations[1]})
	if err == nil || !strings.Contains(err.Error(), RequiredMigrations[2]) {
		t.Fatalf("missing migration not rejected: %v", err)
	}
	if err := ValidateRequiredMigrations(RequiredMigrations); err != nil {
		t.Fatalf("complete migration set rejected: %v", err)
	}
}

func TestTEST_P_PreflightRejectsRabbitMQFailure(t *testing.T) {
	for _, raw := range []string{"not-an-amqp-url", "amqp://guest:guest@127.0.0.1:1/missing-vhost"} {
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		result := CheckRabbitMQ(ctx, raw)
		cancel()
		if result.RabbitMQ != "FAIL" || result.VHost != "FAIL" || result.Topology != "FAIL" {
			t.Errorf("%q unexpectedly passed: %+v", raw, result)
		}
	}
}

func TestTEST_Q_PreflightIsSideEffectFree(t *testing.T) {
	if os.Getenv("PR83_CANARY_PREFLIGHT_SIDE_EFFECT_CHECK") != "1" {
		t.Skip("requires PR83_CANARY_PREFLIGHT_SIDE_EFFECT_CHECK=1 (set only in the dedicated CI step)")
	}
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	businessFingerprint := func() string {
		var outbox, generations, executions, artifacts int
		for table, dest := range map[string]*int{
			"outbox_events":       &outbox,
			"xz_generation_tasks": &generations,
			"provider_executions": &executions,
			"xz_file_objects":     &artifacts,
		} {
			if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(dest); err != nil {
				t.Fatalf("fingerprint %s: %v", table, err)
			}
		}
		return fmt.Sprintf("%d|%d|%d|%d", outbox, generations, executions, artifacts)
	}
	before := businessFingerprint()
	cfg := config.Config{
		AsyncMessagingEnabled:                  true,
		GenerationAsyncCanaryEnabled:           true,
		ProviderExecutionSafetyEnabled:         true,
		GenerationAsyncCanaryUsers:             "user-canary",
		GenerationAsyncCanaryProviderAllowlist: "channel-stage0",
		GenerationAsyncCanaryModelAllowlist:    "gpt-image-2",
		MetricsEnabled:                         true,
		RabbitMQURL:                            "amqp://guest:guest@127.0.0.1:1/preflight-test",
	}
	result := Run(ctx, cfg, db)
	if result.Database != "PASS" {
		t.Fatalf("expected Database=PASS to prove the real DB read path ran: %+v", result)
	}
	if result.Ready == "PASS" {
		t.Fatalf("preflight unexpectedly ready against a dead broker: %+v", result)
	}
	after := businessFingerprint()
	if before != after {
		t.Fatalf("preflight mutated business state: before=%s after=%s", before, after)
	}
}

func TestPreflightOutputContainsStableMachineReadableKeys(t *testing.T) {
	lines := Result{}.Lines()
	joined := strings.Join(lines, "\n")
	for _, key := range []string{"PREFLIGHT_DATABASE=", "PREFLIGHT_MIGRATIONS=", "PREFLIGHT_RABBITMQ=", "PREFLIGHT_VHOST=", "PREFLIGHT_TOPOLOGY=", "PREFLIGHT_PROVIDER_ALLOWLIST=", "PREFLIGHT_MODEL_ALLOWLIST=", "PREFLIGHT_USER_ALLOWLIST=", "PREFLIGHT_METRICS=", "PREFLIGHT_READY="} {
		if !strings.Contains(joined, key) {
			t.Errorf("missing output key %s", key)
		}
	}
}

func TestPreflightUserAllowlistWildcardValid(t *testing.T) {
	cfg := config.Config{
		GenerationAsyncCanaryUsers: "*",
	}
	result := Run(context.Background(), cfg, nil)
	if result.UserAllowlist != "PASS" {
		t.Fatalf("expected UserAllowlist=PASS for wildcard: got %s", result.UserAllowlist)
	}

	cfg.GenerationAsyncCanaryUsers = ""
	result = Run(context.Background(), cfg, nil)
	if result.UserAllowlist != "FAIL" {
		t.Fatalf("expected UserAllowlist=FAIL for empty users: got %s", result.UserAllowlist)
	}
}

