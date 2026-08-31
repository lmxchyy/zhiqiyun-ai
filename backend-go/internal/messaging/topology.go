package messaging

import (
	"fmt"
	"strings"

	"github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeEvents    = "x.ai.events"
	ExchangeDLX       = "x.ai.dlx"
	ExchangeType      = "topic"
	DLXType           = "fanout"
	MessagePrefix     = "x.ai."
	DefaultPrefetch   = 1
	MaxPublishRetries = 3
)

// Declaration represents a single topology declaration.
type Declaration struct {
	Name       string
	Kind       string // "exchange" or "queue"
	Durable    bool
	AutoDelete bool
	Arguments  map[string]interface{}
	Bindings   []Binding // for queues
}

// Binding represents a queue-to-exchange binding.
type Binding struct {
	QueueName    string
	ExchangeName string
	RoutingKey   string
	Arguments    map[string]interface{}
}

// TopologyBuilder declares all required exchanges and queues.
type TopologyBuilder struct {
	conn *amqp091.Connection
}

// NewTopologyBuilder creates a builder bound to an existing connection.
func NewTopologyBuilder(conn *amqp091.Connection) *TopologyBuilder {
	return &TopologyBuilder{conn: conn}
}

// Build declares the minimal topology required for PR1.
// It creates:
//   - Exchange: x.ai.events (topic, durable)
//   - Exchange: x.ai.dlx (fanout, durable)
//   - Queue: x.ai.test.generation.canary (durable) bound to x.ai.events
//   - Queue: x.ai.dlq (durable) bound to x.ai.dlx
func (tb *TopologyBuilder) Build() error {
	if tb == nil || tb.conn == nil {
		return fmt.Errorf("connection is nil")
	}
	ch, err := tb.conn.Channel()
	if err != nil {
		return fmt.Errorf("open topology channel: %w", err)
	}
	defer ch.Close()

	// Declare the events exchange.
	if err := declareExchange(ch, ExchangeEvents, ExchangeType, true, false, nil); err != nil {
		return fmt.Errorf("declare events exchange: %w", err)
	}

	// Declare the dead letter exchange.
	if err := declareExchange(ch, ExchangeDLX, DLXType, true, false, nil); err != nil {
		return fmt.Errorf("declare dlx exchange: %w", err)
	}

	// Declare the canary queue.
	if err := declareQueue(ch, "x.ai.test.generation.canary", true, false, nil); err != nil {
		return fmt.Errorf("declare canary queue: %w", err)
	}
	if err := bindQueue(ch, "x.ai.test.generation.canary", ExchangeEvents, MessagePrefix+"*", nil); err != nil {
		return fmt.Errorf("bind canary queue: %w", err)
	}

	// Declare the dead letter queue.
	if err := declareQueue(ch, "x.ai.dlq", true, false, nil); err != nil {
		return fmt.Errorf("declare dlq queue: %w", err)
	}
	if err := bindQueue(ch, "x.ai.dlq", ExchangeDLX, "", nil); err != nil {
		return fmt.Errorf("bind dlq queue: %w", err)
	}

	return nil
}

// declareExchange declares an exchange with the given properties.
func declareExchange(ch *amqp091.Channel, name, kind string, durable, autoDelete bool, args map[string]interface{}) error {
	err := ch.ExchangeDeclare(name, kind, durable, autoDelete, false, false, args)
	return err
}

// declareQueue declares a queue with the given properties.
func declareQueue(ch *amqp091.Channel, name string, durable, autoDelete bool, args map[string]interface{}) error {
	_, err := ch.QueueDeclare(name, durable, autoDelete, false, false, args)
	return err
}

// bindQueue binds a queue to an exchange with the given routing key.
func bindQueue(ch *amqp091.Channel, queue, exchange, routingKey string, args map[string]interface{}) error {
	err := ch.QueueBind(queue, routingKey, exchange, false, args)
	return err
}

// ValidateRoutingKey validates that a routing key follows the x.ai.* convention.
func ValidateRoutingKey(key string) error {
	if !strings.HasPrefix(key, MessagePrefix) {
		return fmt.Errorf("routing key %q must start with %q", key, MessagePrefix)
	}
	if key == MessagePrefix {
		return fmt.Errorf("routing key cannot be just the prefix")
	}
	return nil
}

// EnsureTopology declares the topology if RabbitMQ is connected.
// Returns nil if not connected (caller decides whether to treat as error).
func EnsureTopology(conn *amqp091.Connection) error {
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}
	tb := NewTopologyBuilder(conn)
	return tb.Build()
}
