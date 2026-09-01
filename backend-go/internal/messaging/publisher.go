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

// Publisher owns one confirm-enabled channel. Publish is serialized so returns
// and confirmations can be correlated without sharing a channel with consumers.
type Publisher struct {
	connManager *ConnectionManager
	retry       RetryStrategy

	mu      sync.Mutex
	channel *amqp091.Channel
	returns <-chan amqp091.Return
	closed  <-chan *amqp091.Error
	stopped bool

	publishCount atomic.Int64
	publishFail  atomic.Int64
}

func NewPublisher(connManager *ConnectionManager) *Publisher {
	return NewPublisherWithRetry(connManager, RetryStrategy{InitialDelay: time.Second, MaxDelay: time.Second, Multiplier: 1, MaxAttempts: 1})
}

func NewPublisherWithRetry(connManager *ConnectionManager, retry RetryStrategy) *Publisher {
	if retry.MaxAttempts <= 0 {
		retry = RetryStrategy{InitialDelay: time.Second, MaxDelay: time.Second, Multiplier: 1, MaxAttempts: 1}
	}
	return &Publisher{connManager: connManager, retry: retry}
}

// Start eagerly creates the publisher channel. Publish also starts lazily.
func (p *Publisher) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return fmt.Errorf("publisher stopped")
	}
	return p.ensureChannelLocked(ctx)
}

// Close is idempotent and prevents later publishes.
func (p *Publisher) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped = true
	return p.resetChannelLocked()
}

func (p *Publisher) Publish(ctx context.Context, envelope *Envelope, routingKey string) error {
	if envelope == nil {
		return fmt.Errorf("envelope is nil")
	}
	if routingKey == "" {
		routingKey = envelope.EventType
	}
	if err := ValidateRoutingKey(routingKey); err != nil {
		return err
	}
	payload, err := envelope.Payload()
	if err != nil {
		return fmt.Errorf("encode envelope: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return fmt.Errorf("publisher stopped")
	}
	var lastErr error
	for attempt := 1; attempt <= p.retry.MaxAttempts; attempt++ {
		if err := p.publishOnceLocked(ctx, payload, routingKey); err != nil {
			lastErr = err
			p.publishFail.Add(1)
			_ = p.resetChannelLocked()
			if attempt == p.retry.MaxAttempts {
				break
			}
			if !waitContext(ctx, p.retry.NextDelay(attempt)) {
				return fmt.Errorf("publish cancelled: %w", ctx.Err())
			}
			continue
		}
		p.publishCount.Add(1)
		return nil
	}
	return fmt.Errorf("publish failed after %d attempts: %w", p.retry.MaxAttempts, lastErr)
}

func (p *Publisher) ensureChannelLocked(ctx context.Context) error {
	if p.channel != nil {
		select {
		case <-p.closed:
			_ = p.resetChannelLocked()
		default:
			return nil
		}
	}
	if p.connManager == nil {
		return fmt.Errorf("connection manager is required")
	}
	ch, err := p.connManager.OpenChannel(ctx)
	if err != nil {
		return err
	}
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		return fmt.Errorf("enable publisher confirms: %w", err)
	}
	p.channel = ch
	p.returns = ch.NotifyReturn(make(chan amqp091.Return, 1))
	p.closed = ch.NotifyClose(make(chan *amqp091.Error, 1))
	return nil
}

func (p *Publisher) publishOnceLocked(ctx context.Context, payload []byte, routingKey string) error {
	if err := p.ensureChannelLocked(ctx); err != nil {
		return err
	}
	msg := amqp091.Publishing{
		ContentType:  "application/json",
		Body:         payload,
		DeliveryMode: amqp091.Persistent,
		MessageId:    extractEventID(payload),
		Timestamp:    time.Now().UTC(),
		Headers: amqp091.Table{
			"event_id": extractEventID(payload), "event_type": routingKey, "version": int32(1),
		},
	}
	confirmation, err := p.channel.PublishWithDeferredConfirmWithContext(ctx, ExchangeEvents, routingKey, true, false, msg)
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	if confirmation == nil {
		return fmt.Errorf("publisher confirmation unavailable")
	}
	confirmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		select {
		case returned, ok := <-p.returns:
			if !ok {
				return fmt.Errorf("publisher return channel closed")
			}
			return fmt.Errorf("message returned by broker: %s", returned.ReplyText)
		case closeErr, ok := <-p.closed:
			if ok && closeErr != nil {
				return fmt.Errorf("publisher channel closed: %w", closeErr)
			}
			return fmt.Errorf("publisher channel closed")
		case <-confirmation.Done():
			if !confirmation.Acked() {
				return fmt.Errorf("publisher nack received for delivery tag %d", confirmation.DeliveryTag)
			}
			// RabbitMQ sends basic.return before the confirm for a mandatory
			// unroutable publish. Drain it before declaring success.
			select {
			case returned := <-p.returns:
				return fmt.Errorf("message returned by broker: %s", returned.ReplyText)
			default:
				return nil
			}
		case <-confirmCtx.Done():
			return fmt.Errorf("confirm timeout: %w", confirmCtx.Err())
		}
	}
}

func (p *Publisher) resetChannelLocked() error {
	ch := p.channel
	p.channel, p.returns, p.closed = nil, nil, nil
	if ch == nil || ch.IsClosed() {
		return nil
	}
	return ch.Close()
}

func extractEventID(payload []byte) string {
	var env struct {
		EventID string `json:"event_id"`
	}
	if err := json.Unmarshal(payload, &env); err == nil {
		return env.EventID
	}
	return ""
}

func (p *Publisher) PublishCount() int64     { return p.publishCount.Load() }
func (p *Publisher) PublishFailCount() int64 { return p.publishFail.Load() }
