package messaging

import (
	"context"
	"errors"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

func messagingIntegrationURL(t *testing.T) string {
	t.Helper()
	raw := os.Getenv("XIANZHI_MESSAGING_TEST_RABBITMQ_URL")
	if raw == "" {
		t.Skip("XIANZHI_MESSAGING_TEST_RABBITMQ_URL is not configured")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path == "" || parsed.Path == "/" {
		t.Fatal("messaging integration tests require a dedicated non-root RabbitMQ vhost")
	}
	return raw
}

func TestRabbitMQRuntimeRecoveryRetryDLQAndChannelRecreation(t *testing.T) {
	rawURL := messagingIntegrationURL(t)
	manager, err := NewConnectionManager(RabbitMQConfig{URL: rawURL, ReconnectBase: 50 * time.Millisecond, ReconnectMax: 250 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	manager.Start()
	defer manager.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := manager.WaitForConnection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	for _, queue := range []string{GenerationCanaryQueue, GenerationCanaryRetryQueue, GenerationCanaryDLQ} {
		_, _ = admin.QueuePurge(queue, false)
	}
	_ = admin.Close()

	var attempts atomic.Int32
	consumer := NewConsumer(manager,
		WithPrefetch(2), WithMaxConcurrency(2),
		WithRetryPolicy(ExchangeRetry, GenerationCanaryRetryKey, 2),
		WithOnMessage(func(_ context.Context, envelope *Envelope) error {
			if envelope.AggregateID == "poison" {
				return Permanent(errors.New("poison"))
			}
			if envelope.AggregateID == "transient" && attempts.Add(1) < 3 {
				return errors.New("temporary")
			}
			attempts.Add(1)
			return nil
		}),
	)
	if err := consumer.Start(ctx, GenerationCanaryQueue); err != nil {
		t.Fatal(err)
	}
	defer consumer.Stop()
	publisher := NewPublisher(manager)
	if err := publisher.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer publisher.Close()

	transient := MustEnvelope(GenerationCanaryRoutingKey, "generation_task", "transient", nil)
	if err := publisher.Publish(ctx, transient, GenerationCanaryRoutingKey); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, ctx, func() bool { return attempts.Load() >= 3 }, "bounded transient delivery")

	poison := MustEnvelope(GenerationCanaryRoutingKey, "generation_task", "poison", nil)
	if err := publisher.Publish(ctx, poison, GenerationCanaryRoutingKey); err != nil {
		t.Fatal(err)
	}
	var dead amqp091.Delivery
	waitUntil(t, ctx, func() bool {
		ch, err := manager.OpenChannel(ctx)
		if err != nil {
			return false
		}
		defer ch.Close()
		dead, _, err = ch.Get(GenerationCanaryDLQ, true)
		return err == nil && len(dead.Body) > 0
	}, "poison DLQ delivery")
	decoded, err := DecodeEnvelope(dead.Body)
	if err != nil || decoded.EventID != poison.EventID {
		t.Fatalf("DLQ identity got=%v err=%v", decoded, err)
	}
	deaths, ok := dead.Headers["x-death"].([]interface{})
	if !ok || len(deaths) == 0 {
		t.Fatalf("DLQ delivery missing x-death: %#v", dead.Headers)
	}

	// A consumer channel close must cause a fresh Consume session.
	consumer.mu.Lock()
	consumerChannel := consumer.channel
	consumer.mu.Unlock()
	if consumerChannel == nil {
		t.Fatal("consumer channel unavailable")
	}
	_ = consumerChannel.Close()
	before := attempts.Load()
	followup := MustEnvelope(GenerationCanaryRoutingKey, "generation_task", "followup", nil)
	if err := publisher.Publish(ctx, followup, GenerationCanaryRoutingKey); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, ctx, func() bool { return attempts.Load() > before }, "consumer channel recreation")

	// Publisher NotifyClose must be observed and the next publish must recreate.
	publisher.mu.Lock()
	publisherChannel := publisher.channel
	publisher.mu.Unlock()
	if publisherChannel == nil {
		t.Fatal("publisher channel unavailable")
	}
	_ = publisherChannel.Close()
	if err := publisher.Publish(ctx, MustEnvelope(GenerationCanaryRoutingKey, "generation_task", "publisher-reopen", nil), GenerationCanaryRoutingKey); err != nil {
		t.Fatal(err)
	}
	if err := publisher.Publish(ctx, MustEnvelope("x.ai.unroutable.more", "test", "return", nil), "x.ai.unroutable.more"); err == nil {
		t.Fatal("mandatory unroutable publish unexpectedly succeeded")
	}

	// Connection NotifyClose triggers bounded reconnect and topology recreation.
	oldConn := manager.GetConnection()
	_ = oldConn.Close()
	waitUntil(t, ctx, func() bool { return manager.IsConnected() && manager.GetConnection() != oldConn }, "connection recovery")
	check, err := manager.OpenChannel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	if _, err := check.QueueInspect(GenerationCanaryQueue); err != nil {
		t.Fatalf("topology was not recreated: %v", err)
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	if err := consumer.StopContext(stopCtx); err != nil {
		t.Fatal(err)
	}
}

func waitUntil(t *testing.T, ctx context.Context, condition func() bool, label string) {
	t.Helper()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for %s: %v", label, ctx.Err())
		case <-ticker.C:
		}
	}
}
