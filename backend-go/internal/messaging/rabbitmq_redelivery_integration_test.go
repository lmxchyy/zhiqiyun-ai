package messaging

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

// TestRabbitMQGenerationManualAckRedelivery uses broker redelivery, not a
// second handler invocation: the first consumer channel is closed before ACK.
func TestRabbitMQGenerationManualAckRedelivery(t *testing.T) {
	rawURL := messagingIntegrationURL(t)
	manager, err := NewConnectionManager(RabbitMQConfig{URL: rawURL, ReconnectBase: 50 * time.Millisecond, ReconnectMax: 250 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	manager.Start()
	defer manager.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := manager.WaitForConnection(ctx); err != nil {
		t.Fatal(err)
	}
	admin, err := manager.OpenChannel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = admin.QueuePurge(GenerationCanaryQueue, false)
	_ = admin.Close()
	publisher := NewPublisher(manager)
	if err := publisher.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer publisher.Close()
	envelope := MustEnvelope(GenerationCanaryRoutingKey, "generation_task", "redelivery-task", nil)
	if err := publisher.Publish(ctx, envelope, GenerationCanaryRoutingKey); err != nil {
		t.Fatal(err)
	}
	first, err := manager.OpenChannel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	deliveries, err := first.Consume(GenerationCanaryQueue, "redelivery-first", false, false, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	var seen atomic.Int32
	var d amqp091.Delivery
	select {
	case d = <-deliveries:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	decoded, err := DecodeEnvelope(d.Body)
	if err != nil || decoded.EventID != envelope.EventID {
		t.Fatalf("first delivery event=%v err=%v", decoded, err)
	}
	seen.Add(1)
	// Closing the channel requeues the unacked generation message.
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := manager.OpenChannel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	redeliveries, err := second.Consume(GenerationCanaryQueue, "redelivery-second", false, false, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case redelivered := <-redeliveries:
		if !redelivered.Redelivered {
			t.Fatal("broker did not mark generation message redelivered")
		}
		decoded, err := DecodeEnvelope(redelivered.Body)
		if err != nil || decoded.EventID != envelope.EventID {
			t.Fatalf("redelivery event=%v err=%v", decoded, err)
		}
		seen.Add(1)
		if err := redelivered.Ack(false); err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if seen.Load() != 2 {
		t.Fatalf("deliveries=%d", seen.Load())
	}
	t.Log("RABBITMQ_GENERATION_REDELIVERY=PASS")
}
