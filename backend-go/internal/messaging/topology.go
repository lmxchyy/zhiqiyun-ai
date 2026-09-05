package messaging

import (
	"fmt"
	"strings"

	"github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeEvents               = "x.ai.events"
	ExchangeDLX                  = "x.ai.dlx"
	ExchangeRetry                = "x.ai.retry"
	ExchangeType                 = "topic"
	DLXType                      = "fanout"
	MessagePrefix                = "x.ai."
	DefaultPrefetch              = 1
	MaxPublishRetries            = 3
	GenerationCanaryQueue        = "x.ai.generation.image.canary"
	GenerationCanaryRetryQueue   = "x.ai.generation.image.canary.retry"
	GenerationCanaryDLQ          = "x.ai.generation.image.canary.dlq"
	GenerationCanaryRoutingKey      = "x.ai.generation.image.canary.requested"
	GenerationCanaryRetryKey        = "x.ai.generation.image.canary.retry"
	GenerationCanaryDeadKey         = "x.ai.generation.image.canary.dead"
	GenerationVideoCanaryQueue      = "x.ai.generation.video.canary"
	GenerationVideoCanaryRetryQueue = "x.ai.generation.video.canary.retry"
	GenerationVideoCanaryDLQ        = "x.ai.generation.video.canary.dlq"
	GenerationVideoCanaryRoutingKey = "x.ai.generation.video.canary.requested"
	GenerationVideoCanaryRetryKey   = "x.ai.generation.video.canary.retry"
	GenerationVideoCanaryDeadKey    = "x.ai.generation.video.canary.dead"
	GenerationPPTCanaryQueue        = "x.ai.generation.ppt.canary"
	GenerationPPTCanaryRetryQueue   = "x.ai.generation.ppt.canary.retry"
	GenerationPPTCanaryDLQ          = "x.ai.generation.ppt.canary.dlq"
	GenerationPPTCanaryRoutingKey   = "x.ai.generation.ppt.canary.requested"
	GenerationPPTCanaryRetryKey     = "x.ai.generation.ppt.canary.retry"
	GenerationPPTCanaryDeadKey      = "x.ai.generation.ppt.canary.dead"
	DefaultConsumerMaxRetries       = 3
	defaultRetryQueueDelayMillis = int32(1000)
)

// Declaration represents a single topology declaration.
type Declaration struct {
	Name       string
	Kind       string
	Durable    bool
	AutoDelete bool
	Arguments  map[string]interface{}
	Bindings   []Binding
}

type Binding struct {
	QueueName    string
	ExchangeName string
	RoutingKey   string
	Arguments    map[string]interface{}
}

type TopologyBuilder struct{ conn *amqp091.Connection }

func NewTopologyBuilder(conn *amqp091.Connection) *TopologyBuilder {
	return &TopologyBuilder{conn: conn}
}

// Build recreates the durable business topology after every connection.
func (tb *TopologyBuilder) Build() error {
	if tb == nil || tb.conn == nil {
		return fmt.Errorf("connection is nil")
	}
	ch, err := tb.conn.Channel()
	if err != nil {
		return fmt.Errorf("open topology channel: %w", err)
	}
	defer ch.Close()

	for _, exchange := range []struct{ name, kind string }{
		{ExchangeEvents, ExchangeType}, {ExchangeDLX, DLXType}, {ExchangeRetry, "direct"},
	} {
		if err := declareExchange(ch, exchange.name, exchange.kind, true, false, nil); err != nil {
			return fmt.Errorf("declare exchange %s: %w", exchange.name, err)
		}
	}

	// Retain the foundation test queue with its original declaration. Durable
	// queue arguments are immutable and changing them would break upgrades.
	if err := declareQueue(ch, "x.ai.test.generation.canary", true, false, nil); err != nil {
		return fmt.Errorf("declare test canary queue: %w", err)
	}
	if err := bindQueue(ch, "x.ai.test.generation.canary", ExchangeEvents, MessagePrefix+"*", nil); err != nil {
		return fmt.Errorf("bind test canary queue: %w", err)
	}

	// Preserve the PR3 durable queue contract. x.ai.dlx is fanout, so rejected
	// messages reach the durable canary DLQ without an unsafe queue redeclare.
	canaryArgs := amqp091.Table{"x-dead-letter-exchange": ExchangeDLX}
	if err := declareQueue(ch, GenerationCanaryQueue, true, false, canaryArgs); err != nil {
		return fmt.Errorf("declare generation canary queue: %w", err)
	}
	if err := bindQueue(ch, GenerationCanaryQueue, ExchangeEvents, GenerationCanaryRoutingKey, nil); err != nil {
		return fmt.Errorf("bind generation canary queue: %w", err)
	}

	// Transient failures are republished here. Queue TTL creates a bounded delay,
	// then dead-letters the same body/event identity back to the business route.
	retryArgs := amqp091.Table{
		"x-message-ttl":             defaultRetryQueueDelayMillis,
		"x-dead-letter-exchange":    ExchangeEvents,
		"x-dead-letter-routing-key": GenerationCanaryRoutingKey,
	}
	if err := declareQueue(ch, GenerationCanaryRetryQueue, true, false, retryArgs); err != nil {
		return fmt.Errorf("declare generation canary retry queue: %w", err)
	}
	if err := bindQueue(ch, GenerationCanaryRetryQueue, ExchangeRetry, GenerationCanaryRetryKey, nil); err != nil {
		return fmt.Errorf("bind generation canary retry queue: %w", err)
	}

	if err := declareQueue(ch, GenerationCanaryDLQ, true, false, nil); err != nil {
		return fmt.Errorf("declare generation canary dlq: %w", err)
	}
	if err := bindQueue(ch, GenerationCanaryDLQ, ExchangeDLX, "", nil); err != nil {
		return fmt.Errorf("bind generation canary dlq: %w", err)
	}

	videoCanaryArgs := amqp091.Table{"x-dead-letter-exchange": ExchangeDLX}
	if err := declareQueue(ch, GenerationVideoCanaryQueue, true, false, videoCanaryArgs); err != nil {
		return fmt.Errorf("declare generation video canary queue: %w", err)
	}
	if err := bindQueue(ch, GenerationVideoCanaryQueue, ExchangeEvents, GenerationVideoCanaryRoutingKey, nil); err != nil {
		return fmt.Errorf("bind generation video canary queue: %w", err)
	}

	videoRetryArgs := amqp091.Table{
		"x-message-ttl":             defaultRetryQueueDelayMillis,
		"x-dead-letter-exchange":    ExchangeEvents,
		"x-dead-letter-routing-key": GenerationVideoCanaryRoutingKey,
	}
	if err := declareQueue(ch, GenerationVideoCanaryRetryQueue, true, false, videoRetryArgs); err != nil {
		return fmt.Errorf("declare generation video canary retry queue: %w", err)
	}
	if err := bindQueue(ch, GenerationVideoCanaryRetryQueue, ExchangeRetry, GenerationVideoCanaryRetryKey, nil); err != nil {
		return fmt.Errorf("bind generation video canary retry queue: %w", err)
	}

	if err := declareQueue(ch, GenerationVideoCanaryDLQ, true, false, nil); err != nil {
		return fmt.Errorf("declare generation video canary dlq: %w", err)
	}
	if err := bindQueue(ch, GenerationVideoCanaryDLQ, ExchangeDLX, "", nil); err != nil {
		return fmt.Errorf("bind generation video canary dlq: %w", err)
	}

	pptCanaryArgs := amqp091.Table{"x-dead-letter-exchange": ExchangeDLX}
	if err := declareQueue(ch, GenerationPPTCanaryQueue, true, false, pptCanaryArgs); err != nil {
		return fmt.Errorf("declare generation ppt canary queue: %w", err)
	}
	if err := bindQueue(ch, GenerationPPTCanaryQueue, ExchangeEvents, GenerationPPTCanaryRoutingKey, nil); err != nil {
		return fmt.Errorf("bind generation ppt canary queue: %w", err)
	}

	pptRetryArgs := amqp091.Table{
		"x-message-ttl":             defaultRetryQueueDelayMillis,
		"x-dead-letter-exchange":    ExchangeEvents,
		"x-dead-letter-routing-key": GenerationPPTCanaryRoutingKey,
	}
	if err := declareQueue(ch, GenerationPPTCanaryRetryQueue, true, false, pptRetryArgs); err != nil {
		return fmt.Errorf("declare generation ppt canary retry queue: %w", err)
	}
	if err := bindQueue(ch, GenerationPPTCanaryRetryQueue, ExchangeRetry, GenerationPPTCanaryRetryKey, nil); err != nil {
		return fmt.Errorf("bind generation ppt canary retry queue: %w", err)
	}

	if err := declareQueue(ch, GenerationPPTCanaryDLQ, true, false, nil); err != nil {
		return fmt.Errorf("declare generation ppt canary dlq: %w", err)
	}
	if err := bindQueue(ch, GenerationPPTCanaryDLQ, ExchangeDLX, "", nil); err != nil {
		return fmt.Errorf("bind generation ppt canary dlq: %w", err)
	}

	// Keep the shared DLQ for non-business foundation routes.
	if err := declareQueue(ch, "x.ai.dlq", true, false, nil); err != nil {
		return fmt.Errorf("declare dlq queue: %w", err)
	}
	if err := bindQueue(ch, "x.ai.dlq", ExchangeDLX, "", nil); err != nil {
		return fmt.Errorf("bind dlq queue: %w", err)
	}
	return nil
}

func declareExchange(ch *amqp091.Channel, name, kind string, durable, autoDelete bool, args map[string]interface{}) error {
	return ch.ExchangeDeclare(name, kind, durable, autoDelete, false, false, args)
}
func declareQueue(ch *amqp091.Channel, name string, durable, autoDelete bool, args map[string]interface{}) error {
	_, err := ch.QueueDeclare(name, durable, autoDelete, false, false, args)
	return err
}
func bindQueue(ch *amqp091.Channel, queue, exchange, routingKey string, args map[string]interface{}) error {
	return ch.QueueBind(queue, routingKey, exchange, false, args)
}
func ValidateRoutingKey(key string) error {
	if !strings.HasPrefix(key, MessagePrefix) {
		return fmt.Errorf("routing key %q must start with %q", key, MessagePrefix)
	}
	if key == MessagePrefix {
		return fmt.Errorf("routing key cannot be just the prefix")
	}
	return nil
}
func EnsureTopology(conn *amqp091.Connection) error {
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}
	return NewTopologyBuilder(conn).Build()
}
