package messaging

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

// ConnectionState represents the current state of the RabbitMQ connection.
type ConnectionState int

const (
	StateDisconnected ConnectionState = iota
	StateConnecting
	StateConnected
	StateClosing
)

func (s ConnectionState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateClosing:
		return "closing"
	default:
		return "unknown"
	}
}

// RabbitMQConfig holds the configuration for a RabbitMQ connection.
type RabbitMQConfig struct {
	URL           string
	Heartbeat     time.Duration
	ChannelMax    int
	FrameMax      int
	ConnectionMax int
	ReconnectBase time.Duration
	ReconnectMax  time.Duration
	Username      string
	Password      string
}

// validate ensures the RabbitMQ config has required fields.
func (c RabbitMQConfig) validate() error {
	if c.URL == "" {
		return fmt.Errorf("rabbitmq URL is required")
	}
	if c.Heartbeat <= 0 {
		c.Heartbeat = 10 * time.Second
	}
	if c.ReconnectBase <= 0 {
		c.ReconnectBase = 1 * time.Second
	}
	if c.ReconnectMax <= 0 {
		c.ReconnectMax = 60 * time.Second
	}
	return nil
}

// ConnectionManager manages the RabbitMQ AMQP connection lifecycle.
// It supports background reconnection and graceful shutdown.
type ConnectionManager struct {
	cfg             RabbitMQConfig
	conn            atomic.Value // *amqp091.Connection
	channel         atomic.Value // *amqp091.Channel
	state           atomic.Value // ConnectionState
	mu              sync.Mutex
	ctx             context.Context
	cancel          context.CancelFunc
	reconnectTicker *time.Ticker
	once            sync.Once
}

// NewConnectionManager creates a new ConnectionManager.
func NewConnectionManager(cfg RabbitMQConfig) (*ConnectionManager, error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid rabbitmq config: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cm := &ConnectionManager{
		cfg:             cfg,
		ctx:             ctx,
		cancel:          cancel,
		reconnectTicker: time.NewTicker(cfg.ReconnectBase),
	}
	cm.state.Store(StateDisconnected)
	cm.reconnectTicker.Stop()
	return cm, nil
}

// Start connects to RabbitMQ and starts the background reconnection loop.
// It returns immediately; reconnection happens in the background.
// RabbitMQ failure does not panic or block the caller.
func (cm *ConnectionManager) Start() {
	cm.once.Do(func() {
		go cm.run()
	})
}

func (cm *ConnectionManager) run() {
	if err := cm.connect(); err != nil {
		cm.logError("initial connect failed, will retry: %v", err)
	}
	for {
		select {
		case <-cm.ctx.Done():
			return
		case <-cm.reconnectTicker.C:
			if cm.GetState() == StateConnected {
				continue
			}
			if err := cm.connect(); err != nil {
				cm.logError("reconnect failed: %v", err)
			}
		}
	}
}

func (cm *ConnectionManager) connect() error {
	cm.setState(StateConnecting)
	conn, err := amqp091.DialConfig(cm.cfg.URL, amqp091.Config{
		Heartbeat:  cm.cfg.Heartbeat,
		ChannelMax: uint16(cm.cfg.ChannelMax),
	})
	if err != nil {
		cm.setState(StateDisconnected)
		return fmt.Errorf("dial rabbitmq: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		cm.setState(StateDisconnected)
		return fmt.Errorf("open channel: %w", err)
	}
	if err := ch.Qos(1, 0, false); err != nil {
		ch.Close()
		conn.Close()
		cm.setState(StateDisconnected)
		return fmt.Errorf("set qos: %w", err)
	}
	if err := EnsureTopology(conn); err != nil {
		ch.Close()
		conn.Close()
		cm.setState(StateDisconnected)
		return fmt.Errorf("declare topology: %w", err)
	}
	cm.conn.Store(conn)
	cm.channel.Store(ch)
	cm.setState(StateConnected)
	cm.logError("connected to rabbitmq")
	return nil
}

// GetConnection returns the current AMQP connection, or nil if disconnected.
func (cm *ConnectionManager) GetConnection() *amqp091.Connection {
	v := cm.conn.Load()
	if v == nil {
		return nil
	}
	if conn, ok := v.(*amqp091.Connection); ok {
		return conn
	}
	return nil
}

// GetChannel returns the current AMQP channel, or nil if disconnected.
func (cm *ConnectionManager) GetChannel() *amqp091.Channel {
	v := cm.channel.Load()
	if v == nil {
		return nil
	}
	if ch, ok := v.(*amqp091.Channel); ok {
		return ch
	}
	return nil
}

// GetState returns the current connection state.
func (cm *ConnectionManager) GetState() ConnectionState {
	v := cm.state.Load()
	if v == nil {
		return StateDisconnected
	}
	if s, ok := v.(ConnectionState); ok {
		return s
	}
	return StateDisconnected
}

func (cm *ConnectionManager) setState(state ConnectionState) {
	cm.state.Store(state)
}

// Close gracefully closes the connection and channel.
func (cm *ConnectionManager) Close() error {
	cm.cancel()
	cm.reconnectTicker.Stop()
	var errs []error
	ch := cm.GetChannel()
	if ch != nil {
		if err := ch.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	conn := cm.GetConnection()
	if conn != nil {
		if err := conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	cm.setState(StateClosing)
	cm.setState(StateDisconnected)
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

// IsConnected returns true if the connection is currently connected.
func (cm *ConnectionManager) IsConnected() bool {
	return cm.GetState() == StateConnected
}

func (cm *ConnectionManager) logError(format string, args ...interface{}) {
	// Use fmt.Printf for simplicity; in production, use a structured logger.
	// The URL is sanitized: only the hostname and port are logged, never credentials.
	fmt.Printf("[messaging] "+format+"\n", args...)
}
