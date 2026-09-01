package messaging

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

func TestRabbitMQConfigAppliesSafeZeroAndPartialDefaults(t *testing.T) {
	manager, err := NewConnectionManager(RabbitMQConfig{URL: "amqp://guest:guest@127.0.0.1:1/", ReconnectBase: 25 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	cfg := manager.Config()
	if cfg.Heartbeat <= 0 || cfg.ReconnectBase != 25*time.Millisecond || cfg.ReconnectMax <= 0 || cfg.ReconnectBase > cfg.ReconnectMax {
		t.Fatalf("unsafe normalized config: %+v", cfg)
	}
	_ = manager.Close()
}

func TestRabbitMQConfigRejectsInvalidExplicitDurations(t *testing.T) {
	tests := []RabbitMQConfig{
		{URL: "amqp://localhost", Heartbeat: -time.Second},
		{URL: "amqp://localhost", ReconnectBase: -time.Second},
		{URL: "amqp://localhost", ReconnectMax: -time.Second},
		{URL: "amqp://localhost", ReconnectBase: 2 * time.Second, ReconnectMax: time.Second},
	}
	for _, cfg := range tests {
		if _, err := NewConnectionManager(cfg); err == nil {
			t.Fatalf("expected invalid config error for %+v", cfg)
		}
	}
	if _, err := NewConnectionManager(RabbitMQConfig{}); err == nil || !strings.Contains(err.Error(), "URL is required") {
		t.Fatalf("expected clear URL error, got %v", err)
	}
}

func TestConnectionManagerOutageRetriesAreBoundedAndShutdownInterruptsBackoff(t *testing.T) {
	manager, err := NewConnectionManager(RabbitMQConfig{
		URL: "amqp://guest:guest@127.0.0.1:1/", Heartbeat: 50 * time.Millisecond,
		ReconnectBase: 15 * time.Millisecond, ReconnectMax: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	manager.Start()
	time.Sleep(110 * time.Millisecond)
	attempts := manager.ConnectAttempts()
	if attempts < 2 || attempts > 10 {
		t.Fatalf("unexpected retry count %d", attempts)
	}
	started := time.Now()
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("shutdown took %s", elapsed)
	}
	finalAttempts := manager.ConnectAttempts()
	time.Sleep(60 * time.Millisecond)
	if manager.ConnectAttempts() != finalAttempts {
		t.Fatal("reconnect continued after shutdown")
	}
	if manager.GetState() != StateDisconnected {
		t.Fatalf("state=%s", manager.GetState())
	}
}

type recordingAcknowledger struct {
	mu                   sync.Mutex
	acks, nacks, rejects int
	requeue              bool
}

func (a *recordingAcknowledger) Ack(uint64, bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.acks++
	return nil
}
func (a *recordingAcknowledger) Nack(_ uint64, _ bool, requeue bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.nacks++
	a.requeue = requeue
	return nil
}
func (a *recordingAcknowledger) Reject(_ uint64, requeue bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rejects++
	a.requeue = requeue
	return nil
}

func TestConsumerPoisonAndExhaustedTransientRejectToDLQ(t *testing.T) {
	poisonAck := &recordingAcknowledger{}
	consumer := NewConsumer(nil, WithOnMessage(func(context.Context, *Envelope) error { return nil }), WithRetryPolicy(ExchangeRetry, GenerationCanaryRetryKey, 3))
	consumer.handleDelivery(context.Background(), amqp091.Delivery{Acknowledger: poisonAck, DeliveryTag: 1, Body: []byte("not-json")})
	if poisonAck.rejects != 1 || poisonAck.requeue {
		t.Fatalf("poison acknowledgement=%+v", poisonAck)
	}

	transientAck := &recordingAcknowledger{}
	consumer = NewConsumer(nil,
		WithOnMessage(func(context.Context, *Envelope) error { return errors.New("temporary") }),
		WithRetryPolicy(ExchangeRetry, GenerationCanaryRetryKey, 3),
	)
	envelope := MustEnvelope(GenerationCanaryRoutingKey, "generation_task", "task-1", map[string]interface{}{"task_id": "task-1"})
	body, _ := envelope.Payload()
	consumer.handleDelivery(context.Background(), amqp091.Delivery{
		Acknowledger: transientAck, DeliveryTag: 2, Body: body,
		Headers: amqp091.Table{"x-retry-count": int32(3)},
	})
	if transientAck.rejects != 1 || transientAck.requeue {
		t.Fatalf("exhausted acknowledgement=%+v", transientAck)
	}
}

func TestConsumerPermanentAndDuplicateClassification(t *testing.T) {
	permanentAck := &recordingAcknowledger{}
	envelope := MustEnvelope(GenerationCanaryRoutingKey, "generation_task", "task-1", nil)
	body, _ := envelope.Payload()
	consumer := NewConsumer(nil, WithOnMessage(func(context.Context, *Envelope) error { return Permanent(errors.New("bad input")) }))
	consumer.handleDelivery(context.Background(), amqp091.Delivery{Acknowledger: permanentAck, DeliveryTag: 1, Body: body})
	if permanentAck.rejects != 1 {
		t.Fatalf("permanent rejects=%d", permanentAck.rejects)
	}

	duplicateAck := &recordingAcknowledger{}
	consumer = NewConsumer(nil, WithOnMessage(func(context.Context, *Envelope) error { return nil }))
	consumer.handleDelivery(context.Background(), amqp091.Delivery{Acknowledger: duplicateAck, DeliveryTag: 1, Body: body})
	if duplicateAck.acks != 1 {
		t.Fatalf("duplicate/success acks=%d", duplicateAck.acks)
	}
}

func TestConsumerEnforcesMaxConcurrencyAndPrefetchConfig(t *testing.T) {
	var active, maximum atomic.Int32
	release := make(chan struct{})
	consumer := NewConsumer(nil, WithPrefetch(2), WithMaxConcurrency(2), WithOnMessage(func(context.Context, *Envelope) error {
		current := active.Add(1)
		for {
			seen := maximum.Load()
			if current <= seen || maximum.CompareAndSwap(seen, current) {
				break
			}
		}
		<-release
		active.Add(-1)
		return nil
	}))
	envelope := MustEnvelope(GenerationCanaryRoutingKey, "generation_task", "task", nil)
	body, _ := envelope.Payload()
	ack := &recordingAcknowledger{}
	deliveries := make(chan amqp091.Delivery, 6)
	for i := 1; i <= 6; i++ {
		deliveries <- amqp091.Delivery{Acknowledger: ack, DeliveryTag: uint64(i), Body: body}
	}
	close(deliveries)
	done := make(chan struct{})
	go func() {
		consumer.dispatch(context.Background(), nil, deliveries, make(chan *amqp091.Error))
		close(done)
	}()
	deadline := time.Now().Add(time.Second)
	for active.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if active.Load() != 2 {
		t.Fatalf("active=%d", active.Load())
	}
	time.Sleep(20 * time.Millisecond)
	if active.Load() > 2 || maximum.Load() > 2 {
		t.Fatalf("concurrency active=%d max=%d", active.Load(), maximum.Load())
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("dispatcher did not finish")
	}
	if ack.acks != 6 {
		t.Fatalf("acks=%d", ack.acks)
	}
}

func TestPublisherStartStopIsIdempotent(t *testing.T) {
	publisher := NewPublisher(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := publisher.Start(ctx); err == nil {
		t.Fatal("expected missing manager error")
	}
	if err := publisher.Close(); err != nil {
		t.Fatal(err)
	}
	if err := publisher.Close(); err != nil {
		t.Fatal(err)
	}
	if err := publisher.Publish(context.Background(), MustEnvelope("x.ai.test", "test", "1", nil), "x.ai.test"); err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("publish after stop error=%v", err)
	}
}
