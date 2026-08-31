package messaging

import (
	"context"
	"fmt"
	"sync"

	"github.com/rabbitmq/amqp091-go"
)

// ConsumerOption configures a consumer.
type ConsumerOption func(*consumerConfig)

type consumerConfig struct {
	prefetch       int
	autoAck        bool
	maxConcurrency int
	onMessage      func(ctx context.Context, envelope *Envelope) error
}

// WithPrefetch sets the prefetch count for the consumer.
func WithPrefetch(n int) ConsumerOption {
	return func(c *consumerConfig) {
		c.prefetch = n
	}
}

// WithAutoAck sets whether messages are auto-acknowledged.
func WithAutoAck(v bool) ConsumerOption {
	return func(c *consumerConfig) {
		c.autoAck = v
	}
}

// WithMaxConcurrency limits the number of concurrent message handlers.
func WithMaxConcurrency(n int) ConsumerOption {
	return func(c *consumerConfig) {
		c.maxConcurrency = n
	}
}

// WithOnMessage sets the message handler callback.
func WithOnMessage(fn func(ctx context.Context, envelope *Envelope) error) ConsumerOption {
	return func(c *consumerConfig) {
		c.onMessage = fn
	}
}

// Consumer processes messages from RabbitMQ queues.
// It supports manual ack, prefetch, and graceful shutdown.
type Consumer struct {
	connManager *ConnectionManager
	config      consumerConfig
	wg          sync.WaitGroup
	mu          sync.Mutex
	running     bool
}

// NewConsumer creates a new Consumer.
func NewConsumer(connManager *ConnectionManager, opts ...ConsumerOption) *Consumer {
	cfg := consumerConfig{
		prefetch:       DefaultPrefetch,
		autoAck:        false,
		maxConcurrency: 1,
		onMessage:      nil,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Consumer{
		connManager: connManager,
		config:      cfg,
	}
}

// Start begins consuming messages from the given queue.
// It blocks until Stop is called. The consumer respects ctx cancellation.
func (c *Consumer) Start(ctx context.Context, queue string) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("consumer already running")
	}
	c.running = true
	c.mu.Unlock()

	ch := c.connManager.GetChannel()
	if ch == nil {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
		return fmt.Errorf("no rabbitmq channel available")
	}

	if err := ch.Qos(c.config.prefetch, 0, false); err != nil {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
		return fmt.Errorf("set qos: %w", err)
	}

	deliveries, err := ch.ConsumeWithContext(ctx, queue, "", c.config.autoAck, false, false, false, nil)
	if err != nil {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
		return fmt.Errorf("consume: %w", err)
	}

	c.wg.Add(1)
	go c.run(ctx, deliveries)
	return nil
}

func (c *Consumer) run(ctx context.Context, deliveries <-chan amqp091.Delivery) {
	defer c.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case d, ok := <-deliveries:
			if !ok {
				return
			}
			if c.config.onMessage == nil {
				_ = d.Reject(false)
				continue
			}
			c.wg.Add(1)
			go func(delivery amqp091.Delivery) {
				defer c.wg.Done()
				c.handleDelivery(delivery)
			}(d)
		}
	}
}

func (c *Consumer) handleDelivery(delivery amqp091.Delivery) {
	if delivery.DeliveryTag == 0 {
		return
	}

	envelope, err := DecodeEnvelope(delivery.Body)
	if err != nil {
		_ = delivery.Nack(false, false)
		return
	}

	if err := c.config.onMessage(context.Background(), envelope); err != nil {
		_ = delivery.Nack(false, false)
		return
	}

	if c.config.autoAck {
		_ = delivery.Ack(false)
	} else {
		_ = delivery.Ack(false)
	}
}

// Stop gracefully stops the consumer and waits for all handlers to finish.
func (c *Consumer) Stop() {
	c.mu.Lock()
	c.running = false
	c.mu.Unlock()
	c.wg.Wait()
}

// Running returns true if the consumer is actively running.
func (c *Consumer) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}
