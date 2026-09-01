package messaging

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

type ConsumerOption func(*consumerConfig)

type consumerConfig struct {
	prefetch        int
	autoAck         bool
	maxConcurrency  int
	maxRetries      int
	retryExchange   string
	retryRoutingKey string
	onMessage       func(ctx context.Context, envelope *Envelope) error
}

func WithPrefetch(n int) ConsumerOption { return func(c *consumerConfig) { c.prefetch = n } }
func WithAutoAck(v bool) ConsumerOption { return func(c *consumerConfig) { c.autoAck = v } }
func WithMaxConcurrency(n int) ConsumerOption {
	return func(c *consumerConfig) { c.maxConcurrency = n }
}
func WithOnMessage(fn func(context.Context, *Envelope) error) ConsumerOption {
	return func(c *consumerConfig) { c.onMessage = fn }
}

// WithRetryPolicy routes transient failures through a durable delay queue.
func WithRetryPolicy(exchange, routingKey string, maxRetries int) ConsumerOption {
	return func(c *consumerConfig) {
		c.retryExchange, c.retryRoutingKey, c.maxRetries = exchange, routingKey, maxRetries
	}
}

type permanentDeliveryError struct{ err error }

func (e permanentDeliveryError) Error() string { return e.err.Error() }
func (e permanentDeliveryError) Unwrap() error { return e.err }

// Permanent marks malformed or invalid business input for immediate dead-letter.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentDeliveryError{err: err}
}

func IsPermanent(err error) bool {
	var target permanentDeliveryError
	return errors.As(err, &target)
}

// Consumer owns one channel at a time and recreates QoS/Consume after channel or
// connection closure. Start is asynchronous; Stop is idempotent and bounded.
type Consumer struct {
	connManager *ConnectionManager
	config      consumerConfig

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	channel *amqp091.Channel
	done    chan struct{}
}

func NewConsumer(connManager *ConnectionManager, opts ...ConsumerOption) *Consumer {
	cfg := consumerConfig{prefetch: DefaultPrefetch, maxConcurrency: 1}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.prefetch <= 0 {
		cfg.prefetch = DefaultPrefetch
	}
	if cfg.maxConcurrency <= 0 {
		cfg.maxConcurrency = 1
	}
	return &Consumer{connManager: connManager, config: cfg}
}

func (c *Consumer) Start(ctx context.Context, queue string) error {
	if c == nil || c.connManager == nil {
		return fmt.Errorf("connection manager is required")
	}
	if queue == "" {
		return fmt.Errorf("consumer queue is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return fmt.Errorf("consumer already running")
	}
	runCtx, cancel := context.WithCancel(ctx)
	c.running, c.cancel, c.done = true, cancel, make(chan struct{})
	go c.supervise(runCtx, queue, c.done)
	return nil
}

func (c *Consumer) supervise(ctx context.Context, queue string, done chan struct{}) {
	defer func() {
		c.mu.Lock()
		if c.done == done {
			c.running, c.cancel, c.channel = false, nil, nil
			close(done)
		}
		c.mu.Unlock()
	}()

	for ctx.Err() == nil {
		ch, err := c.connManager.OpenChannel(ctx)
		if err != nil {
			return
		}
		if err := ch.Qos(c.config.prefetch, 0, false); err != nil {
			_ = ch.Close()
			if !waitContext(ctx, 100*time.Millisecond) {
				return
			}
			continue
		}
		tag := fmt.Sprintf("xianzhi-%s-%d", sanitizeConsumerTag(queue), time.Now().UnixNano())
		deliveries, err := ch.ConsumeWithContext(ctx, queue, tag, c.config.autoAck, false, false, false, nil)
		if err != nil {
			_ = ch.Close()
			if !waitContext(ctx, 100*time.Millisecond) {
				return
			}
			continue
		}
		closed := ch.NotifyClose(make(chan *amqp091.Error, 1))
		c.mu.Lock()
		if c.done != done || !c.running {
			c.mu.Unlock()
			_ = ch.Close()
			return
		}
		c.channel = ch
		c.mu.Unlock()

		c.dispatch(ctx, ch, deliveries, closed)
		c.mu.Lock()
		if c.channel == ch {
			c.channel = nil
		}
		c.mu.Unlock()
		_ = ch.Close()
		if !waitContext(ctx, 100*time.Millisecond) {
			return
		}
	}
}

func (c *Consumer) dispatch(ctx context.Context, ch *amqp091.Channel, deliveries <-chan amqp091.Delivery, closed <-chan *amqp091.Error) {
	sem := make(chan struct{}, c.config.maxConcurrency)
	var handlers sync.WaitGroup
	defer handlers.Wait()
	for {
		select {
		case <-ctx.Done():
			return
		case <-closed:
			return
		case delivery, ok := <-deliveries:
			if !ok {
				return
			}
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			case <-closed:
				return
			}
			handlers.Add(1)
			go func(d amqp091.Delivery) {
				defer handlers.Done()
				defer func() { <-sem }()
				c.handleDelivery(ctx, d)
			}(delivery)
		}
	}
}

func (c *Consumer) handleDelivery(ctx context.Context, delivery amqp091.Delivery) {
	if delivery.DeliveryTag == 0 {
		return
	}
	envelope, err := DecodeEnvelope(delivery.Body)
	if err != nil {
		if !c.config.autoAck {
			_ = delivery.Reject(false)
		}
		return
	}
	if c.config.onMessage == nil {
		if !c.config.autoAck {
			_ = delivery.Reject(false)
		}
		return
	}
	err = c.config.onMessage(ctx, envelope)
	if c.config.autoAck {
		return
	}
	if err == nil {
		_ = delivery.Ack(false)
		return
	}
	if IsPermanent(err) || deliveryRetryCount(delivery.Headers) >= c.config.maxRetries || c.config.maxRetries <= 0 {
		_ = delivery.Reject(false)
		return
	}
	if err := c.publishRetry(ctx, delivery); err != nil {
		// Keep the original broker-owned on infrastructure failure. Closing or
		// requeueing cannot silently discard a transient event.
		_ = waitContext(ctx, 100*time.Millisecond)
		_ = delivery.Nack(false, true)
		return
	}
	_ = delivery.Ack(false)
}

func (c *Consumer) publishRetry(ctx context.Context, delivery amqp091.Delivery) error {
	if c.config.retryExchange == "" || c.config.retryRoutingKey == "" {
		return fmt.Errorf("retry topology is not configured")
	}
	ch, err := c.connManager.OpenChannel(ctx)
	if err != nil {
		return err
	}
	defer ch.Close()
	if err := ch.Confirm(false); err != nil {
		return err
	}
	headers := cloneHeaders(delivery.Headers)
	headers["x-retry-count"] = int32(deliveryRetryCount(delivery.Headers) + 1)
	confirmation, err := ch.PublishWithDeferredConfirmWithContext(ctx, c.config.retryExchange, c.config.retryRoutingKey, true, false, amqp091.Publishing{
		Headers: headers, ContentType: delivery.ContentType, ContentEncoding: delivery.ContentEncoding,
		DeliveryMode: amqp091.Persistent, Priority: delivery.Priority, CorrelationId: delivery.CorrelationId,
		ReplyTo: delivery.ReplyTo, Expiration: delivery.Expiration, MessageId: delivery.MessageId,
		Timestamp: delivery.Timestamp, Type: delivery.Type, UserId: delivery.UserId, AppId: delivery.AppId,
		Body: delivery.Body,
	})
	if err != nil {
		return err
	}
	if confirmation == nil {
		return fmt.Errorf("retry publisher confirmation unavailable")
	}
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	acked, err := confirmation.WaitContext(waitCtx)
	if err != nil {
		return err
	}
	if !acked {
		return fmt.Errorf("retry publish nacked")
	}
	return nil
}

func cloneHeaders(source amqp091.Table) amqp091.Table {
	result := amqp091.Table{}
	for key, value := range source {
		result[key] = value
	}
	return result
}

func deliveryRetryCount(headers amqp091.Table) int {
	value, ok := headers["x-retry-count"]
	if !ok {
		return 0
	}
	switch n := value.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	default:
		return 0
	}
}

func sanitizeConsumerTag(queue string) string {
	result := make([]rune, 0, len(queue))
	for _, r := range queue {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			result = append(result, r)
		} else {
			result = append(result, '-')
		}
	}
	return string(result)
}

// Stop waits at most 30 seconds. Use StopContext for a caller-owned deadline.
func (c *Consumer) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = c.StopContext(ctx)
}

func (c *Consumer) StopContext(ctx context.Context) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	cancel, ch, done := c.cancel, c.channel, c.done
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if ch != nil {
		_ = ch.Close()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("consumer stop timeout: %w", ctx.Err())
	}
}

func (c *Consumer) Running() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.running }
