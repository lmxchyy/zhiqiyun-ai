package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

// Publisher publishes envelope messages to RabbitMQ with publisher confirms.
// It is safe for concurrent use by multiple goroutines.
type Publisher struct {
	connManager  *ConnectionManager
	retry        RetryStrategy
	publishCount atomic.Int64
	publishFail  atomic.Int64
	mu           sync.Mutex
}

// NewPublisher creates a new Publisher.
func NewPublisher(connManager *ConnectionManager) *Publisher {
	return &Publisher{
		connManager: connManager,
		retry:       DefaultRetry(),
	}
}

// Publish publishes an Envelope to the given routing key.
// It waits for publisher confirm before returning.
// Returns nil on success, or an error if publish fails after all retries.
func (p *Publisher) Publish(ctx context.Context, envelope *Envelope, routingKey string) error {
	if envelope == nil {
		return fmt.Errorf("envelope is nil")
	}
	if routingKey == "" {
		routingKey = envelope.EventType
	}

	payload, err := envelope.Payload()
	if err != nil {
		return fmt.Errorf("encode envelope: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= p.retry.MaxAttempts; attempt++ {
		if err := p.publishOnce(ctx, payload, routingKey); err != nil {
			lastErr = err
			p.publishFail.Add(1)
			if attempt >= p.retry.MaxAttempts {
				return fmt.Errorf("publish failed after %d attempts: %w", attempt, lastErr)
			}
			delay := p.retry.NextDelay(attempt)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return fmt.Errorf("publish cancelled: %w", ctx.Err())
			}
			continue
		}
		p.publishCount.Add(1)
		return nil
	}
	return fmt.Errorf("publish exhausted retries: %w", lastErr)
}

func (p *Publisher) publishOnce(ctx context.Context, payload []byte, routingKey string) error {
	ch := p.connManager.GetChannel()
	if ch == nil {
		return fmt.Errorf("no rabbitmq channel available")
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	// Enable publisher confirms before publishing; confirmations are ordered per channel.
	if err := ch.Confirm(false); err != nil {
		return fmt.Errorf("enable confirms: %w", err)
	}

	eventID := extractEventID(payload)
	msg := amqp091.Publishing{
		ContentType:  "application/json",
		Body:         payload,
		DeliveryMode: amqp091.Persistent,
		Headers: map[string]interface{}{
			"event_id":   eventID,
			"event_type": routingKey,
			"version":    1,
		},
	}

	ackChan, nackChan := ch.NotifyConfirm(make(chan uint64, 1), make(chan uint64, 1))
	returnChan := ch.NotifyReturn(make(chan amqp091.Return, 1))
	if err := ch.PublishWithContext(ctx, ExchangeEvents, routingKey, false, true, msg); err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	// Wait for confirm with timeout; mandatory unroutable messages are failures.

	confirmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	select {
	case returned := <-returnChan:
		return fmt.Errorf("message returned by broker: %s", returned.ReplyText)
	case <-ackChan:
		return nil
	case <-nackChan:
		return fmt.Errorf("publisher nack received")
	case <-confirmCtx.Done():
		return fmt.Errorf("confirm timeout: %w", confirmCtx.Err())
	}
}

// extractEventID parses the event_id from the JSON payload.
func extractEventID(payload []byte) string {
	var env struct {
		EventID string `json:"event_id"`
	}
	if err := json.Unmarshal(payload, &env); err == nil {
		return env.EventID
	}
	return ""
}

// PublishCount returns the total number of successful publishes.
func (p *Publisher) PublishCount() int64 {
	return p.publishCount.Load()
}

// PublishFailCount returns the total number of failed publishes.
func (p *Publisher) PublishFailCount() int64 {
	return p.publishFail.Load()
}
