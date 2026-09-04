package canarypreflight

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/rabbitmq/amqp091-go"

	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/messaging"
)

var RequiredMigrations = []string{
	"113-async-messaging-foundation.sql",
	"114-provider-execution-safety.sql",
	"115-generation-artifact-identity.sql",
}

type Result struct {
	Database, Migrations, RabbitMQ, VHost, Topology  string
	ProviderAllowlist, ModelAllowlist, UserAllowlist string
	Metrics, Flags, Ready                            string
}

func pass(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

func (r Result) Lines() []string {
	return []string{
		"PREFLIGHT_DATABASE=" + r.Database,
		"PREFLIGHT_MIGRATIONS=" + r.Migrations,
		"PREFLIGHT_RABBITMQ=" + r.RabbitMQ,
		"PREFLIGHT_VHOST=" + r.VHost,
		"PREFLIGHT_TOPOLOGY=" + r.Topology,
		"PREFLIGHT_PROVIDER_ALLOWLIST=" + r.ProviderAllowlist,
		"PREFLIGHT_MODEL_ALLOWLIST=" + r.ModelAllowlist,
		"PREFLIGHT_USER_ALLOWLIST=" + r.UserAllowlist,
		"PREFLIGHT_METRICS=" + r.Metrics,
		"PREFLIGHT_FLAGS=" + r.Flags,
		"PREFLIGHT_READY=" + r.Ready,
	}
}

// Run performs only reads and AMQP passive declarations. It does not execute
// migrations, write business tables, publish messages, or change feature flags.
func Run(ctx context.Context, cfg config.Config, db *sql.DB) Result {
	result := Result{
		Database: "FAIL", Migrations: "FAIL", RabbitMQ: "FAIL", VHost: "FAIL", Topology: "FAIL",
		ProviderAllowlist: pass(nonEmptyCSV(cfg.GenerationAsyncCanaryProviderAllowlist)),
		ModelAllowlist:    pass(nonEmptyCSV(cfg.GenerationAsyncCanaryModelAllowlist)),
		UserAllowlist:     pass(nonEmptyCSV(cfg.GenerationAsyncCanaryUsers)),
		Metrics:           pass(cfg.MetricsEnabled),
		Flags:             pass(cfg.AsyncMessagingEnabled && cfg.GenerationAsyncCanaryEnabled && cfg.ProviderExecutionSafetyEnabled),
		Ready:             "FAIL",
	}
	if db != nil && db.PingContext(ctx) == nil {
		result.Database = "PASS"
		if MissingRequiredMigrations(ctx, db) == nil {
			result.Migrations = "PASS"
		}
	}
	rabbit := CheckRabbitMQ(ctx, cfg.RabbitMQURL)
	result.RabbitMQ, result.VHost, result.Topology = rabbit.RabbitMQ, rabbit.VHost, rabbit.Topology
	if result.Database == "PASS" && result.Migrations == "PASS" && result.RabbitMQ == "PASS" && result.VHost == "PASS" && result.Topology == "PASS" && result.ProviderAllowlist == "PASS" && result.ModelAllowlist == "PASS" && result.UserAllowlist == "PASS" && result.Metrics == "PASS" && result.Flags == "PASS" {
		result.Ready = "PASS"
	}
	return result
}

func nonEmptyCSV(raw string) bool {
	for _, value := range strings.Split(raw, ",") {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func MissingRequiredMigrations(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	rows, err := db.QueryContext(ctx, `SELECT filename FROM schema_migrations WHERE filename IN ($1,$2,$3)`, RequiredMigrations[0], RequiredMigrations[1], RequiredMigrations[2])
	if err != nil {
		return err
	}
	defer rows.Close()
	found := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	applied := make([]string, 0, len(found))
	for name := range found {
		applied = append(applied, name)
	}
	return ValidateRequiredMigrations(applied)
}

func ValidateRequiredMigrations(applied []string) error {
	found := map[string]bool{}
	for _, name := range applied {
		found[name] = true
	}
	missing := []string{}
	for _, required := range RequiredMigrations {
		if !found[required] {
			missing = append(missing, required)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("missing required migrations: %s", strings.Join(missing, ","))
	}
	return nil
}

type RabbitResult struct{ RabbitMQ, VHost, Topology string }

func CheckRabbitMQ(ctx context.Context, rawURL string) RabbitResult {
	result := RabbitResult{RabbitMQ: "FAIL", VHost: "FAIL", Topology: "FAIL"}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "amqp" && parsed.Scheme != "amqps") || parsed.Host == "" {
		return result
	}
	amqpConfig := amqp091.Config{Heartbeat: 5 * time.Second, Dial: func(network, address string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, network, address)
	}}
	conn, err := amqp091.DialConfig(rawURL, amqpConfig)
	if err != nil {
		return result
	}
	defer conn.Close()
	result.RabbitMQ, result.VHost = "PASS", "PASS"
	ch, err := conn.Channel()
	if err != nil {
		return result
	}
	defer ch.Close()
	if err := inspectTopology(ch); err == nil {
		result.Topology = "PASS"
	}
	return result
}

func inspectTopology(ch *amqp091.Channel) error {
	for _, exchange := range []struct{ name, kind string }{
		{messaging.ExchangeEvents, messaging.ExchangeType},
		{messaging.ExchangeDLX, messaging.DLXType},
		{messaging.ExchangeRetry, "direct"},
	} {
		if err := ch.ExchangeDeclarePassive(exchange.name, exchange.kind, true, false, false, false, nil); err != nil {
			return fmt.Errorf("exchange %s: %w", exchange.name, err)
		}
	}
	queues := []struct {
		name string
		args amqp091.Table
	}{
		{messaging.GenerationCanaryQueue, amqp091.Table{"x-dead-letter-exchange": messaging.ExchangeDLX}},
		{messaging.GenerationCanaryRetryQueue, amqp091.Table{"x-message-ttl": int32(1000), "x-dead-letter-exchange": messaging.ExchangeEvents, "x-dead-letter-routing-key": messaging.GenerationCanaryRoutingKey}},
		{messaging.GenerationCanaryDLQ, nil},
		{messaging.GenerationVideoCanaryQueue, amqp091.Table{"x-dead-letter-exchange": messaging.ExchangeDLX}},
		{messaging.GenerationVideoCanaryRetryQueue, amqp091.Table{"x-message-ttl": int32(1000), "x-dead-letter-exchange": messaging.ExchangeEvents, "x-dead-letter-routing-key": messaging.GenerationVideoCanaryRoutingKey}},
		{messaging.GenerationVideoCanaryDLQ, nil},
	}
	for _, queue := range queues {
		if _, err := ch.QueueDeclarePassive(queue.name, true, false, false, false, queue.args); err != nil {
			return fmt.Errorf("queue %s: %w", queue.name, err)
		}
	}
	return nil
}
