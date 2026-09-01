package messaging

import (
	"context"
	"errors"
	"fmt"
	"net"
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

// RabbitMQConfig holds the configuration for a RabbitMQ connection. Zero
// durations mean "use the safe default"; negative durations are invalid.
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

const (
	defaultHeartbeat     = 10 * time.Second
	defaultReconnectBase = time.Second
	defaultReconnectMax  = 30 * time.Second
)

func normalizeRabbitMQConfig(c RabbitMQConfig) (RabbitMQConfig, error) {
	if c.URL == "" {
		return c, fmt.Errorf("rabbitmq URL is required")
	}
	if c.Heartbeat < 0 {
		return c, fmt.Errorf("heartbeat must not be negative")
	}
	if c.ReconnectBase < 0 {
		return c, fmt.Errorf("reconnect base must not be negative")
	}
	if c.ReconnectMax < 0 {
		return c, fmt.Errorf("reconnect max must not be negative")
	}
	if c.ChannelMax < 0 || c.ChannelMax > int(^uint16(0)) {
		return c, fmt.Errorf("channel max must be between 0 and %d", ^uint16(0))
	}
	if c.FrameMax < 0 || c.ConnectionMax < 0 {
		return c, fmt.Errorf("frame max and connection max must not be negative")
	}
	if c.Heartbeat == 0 {
		c.Heartbeat = defaultHeartbeat
	}
	if c.ReconnectBase == 0 {
		c.ReconnectBase = defaultReconnectBase
	}
	if c.ReconnectMax == 0 {
		c.ReconnectMax = defaultReconnectMax
	}
	if c.ReconnectBase > c.ReconnectMax {
		return c, fmt.Errorf("reconnect base %s exceeds reconnect max %s", c.ReconnectBase, c.ReconnectMax)
	}
	return c, nil
}

// ConnectionManager is the single owner of an AMQP connection. Publisher and
// consumers obtain independent channels from it. It reconnects after initial
// dial failures and broker NotifyClose events without blocking API startup.
type ConnectionManager struct {
	cfg RabbitMQConfig

	mu      sync.RWMutex
	conn    *amqp091.Connection
	state   ConnectionState
	changed chan struct{}

	ctx       context.Context
	cancel    context.CancelFunc
	startOnce sync.Once
	closeOnce sync.Once
	wg        sync.WaitGroup
	attempts  atomic.Int64
}

// NewConnectionManager creates a manager with a fully normalized runtime config.
func NewConnectionManager(cfg RabbitMQConfig) (*ConnectionManager, error) {
	normalized, err := normalizeRabbitMQConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("invalid rabbitmq config: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &ConnectionManager{
		cfg: normalized, state: StateDisconnected, changed: make(chan struct{}), ctx: ctx, cancel: cancel,
	}, nil
}

// Config returns the normalized immutable configuration.
func (cm *ConnectionManager) Config() RabbitMQConfig { return cm.cfg }

// Start starts exactly one connection owner and returns immediately.
func (cm *ConnectionManager) Start() {
	if cm == nil {
		return
	}
	cm.startOnce.Do(func() {
		cm.wg.Add(1)
		go cm.run()
	})
}

func (cm *ConnectionManager) run() {
	defer cm.wg.Done()
	delay := time.Duration(0)
	for {
		if !waitContext(cm.ctx, delay) {
			return
		}
		cm.setState(StateConnecting)
		conn, err := cm.dialAndPrepare()
		if err != nil {
			if cm.ctx.Err() != nil {
				return
			}
			cm.setState(StateDisconnected)
			cm.logError("rabbitmq connect failed: %v", err)
			delay = nextReconnectDelay(delay, cm.cfg.ReconnectBase, cm.cfg.ReconnectMax)
			continue
		}

		delay = cm.cfg.ReconnectBase
		if !cm.installConnection(conn) {
			_ = conn.Close()
			return
		}
		cm.logError("connected to rabbitmq")
		closed := conn.NotifyClose(make(chan *amqp091.Error, 1))
		select {
		case <-cm.ctx.Done():
			cm.clearConnection(conn, StateClosing)
			_ = conn.Close()
			return
		case closeErr := <-closed:
			cm.clearConnection(conn, StateDisconnected)
			if closeErr != nil {
				cm.logError("rabbitmq connection closed: %v", closeErr)
			}
		}
	}
}

func (cm *ConnectionManager) dialAndPrepare() (*amqp091.Connection, error) {
	cm.attempts.Add(1)
	dialTimeout := cm.cfg.Heartbeat
	if dialTimeout > 5*time.Second {
		dialTimeout = 5 * time.Second
	}
	if dialTimeout <= 0 {
		dialTimeout = 5 * time.Second
	}
	dialer := net.Dialer{Timeout: dialTimeout, KeepAlive: cm.cfg.Heartbeat}
	amqpCfg := amqp091.Config{
		Heartbeat:  cm.cfg.Heartbeat,
		ChannelMax: uint16(cm.cfg.ChannelMax),
		Dial: func(network, addr string) (net.Conn, error) {
			conn, err := dialer.DialContext(cm.ctx, network, addr)
			if err != nil {
				return nil, err
			}
			// Bound TLS/AMQP handshaking; amqp091 clears this deadline after
			// open completes, matching its DefaultDial contract.
			if err := conn.SetDeadline(time.Now().Add(dialTimeout)); err != nil {
				_ = conn.Close()
				return nil, err
			}
			return conn, nil
		},
	}
	if cm.cfg.FrameMax > 0 {
		amqpCfg.FrameSize = cm.cfg.FrameMax
	}
	conn, err := amqp091.DialConfig(cm.cfg.URL, amqpCfg)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}
	if err := EnsureTopology(conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("declare topology: %w", err)
	}
	return conn, nil
}

func (cm *ConnectionManager) installConnection(conn *amqp091.Connection) bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.ctx.Err() != nil {
		return false
	}
	cm.conn = conn
	cm.state = StateConnected
	cm.signalChangedLocked()
	return true
}

func (cm *ConnectionManager) clearConnection(conn *amqp091.Connection, state ConnectionState) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.conn == conn {
		cm.conn = nil
		cm.state = state
		cm.signalChangedLocked()
	}
}

func (cm *ConnectionManager) setState(state ConnectionState) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.state != state {
		cm.state = state
		cm.signalChangedLocked()
	}
}

func (cm *ConnectionManager) signalChangedLocked() {
	close(cm.changed)
	cm.changed = make(chan struct{})
}

// WaitForConnection waits interruptibly for the current live connection.
func (cm *ConnectionManager) WaitForConnection(ctx context.Context) (*amqp091.Connection, error) {
	if cm == nil {
		return nil, fmt.Errorf("connection manager is nil")
	}
	for {
		cm.mu.RLock()
		conn, state, changed := cm.conn, cm.state, cm.changed
		cm.mu.RUnlock()
		if conn != nil && state == StateConnected && !conn.IsClosed() {
			return conn, nil
		}
		if state == StateClosing || cm.ctx.Err() != nil {
			return nil, fmt.Errorf("connection manager stopped")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-cm.ctx.Done():
			return nil, fmt.Errorf("connection manager stopped")
		case <-changed:
		}
	}
}

// OpenChannel returns a new caller-owned channel from the current connection.
func (cm *ConnectionManager) OpenChannel(ctx context.Context) (*amqp091.Channel, error) {
	conn, err := cm.WaitForConnection(ctx)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}
	return ch, nil
}

// GetConnection returns the current connection for diagnostics only.
func (cm *ConnectionManager) GetConnection() *amqp091.Connection {
	if cm == nil {
		return nil
	}
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.conn
}

// GetChannel returns a fresh caller-owned channel when connected. Deprecated:
// callers should use OpenChannel with a bounded context.
func (cm *ConnectionManager) GetChannel() *amqp091.Channel {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ch, _ := cm.OpenChannel(ctx)
	return ch
}

func (cm *ConnectionManager) GetState() ConnectionState {
	if cm == nil {
		return StateDisconnected
	}
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.state
}

// Close stops reconnects, closes the live connection, and waits for the owner.
func (cm *ConnectionManager) Close() error {
	if cm == nil {
		return nil
	}
	var closeErr error
	cm.closeOnce.Do(func() {
		cm.setState(StateClosing)
		cm.cancel()
		cm.mu.RLock()
		conn := cm.conn
		cm.mu.RUnlock()
		if conn != nil {
			if err := conn.Close(); err != nil && !errors.Is(err, amqp091.ErrClosed) {
				closeErr = err
			}
		}
		cm.wg.Wait()
		cm.mu.Lock()
		cm.conn = nil
		cm.state = StateDisconnected
		cm.signalChangedLocked()
		cm.mu.Unlock()
	})
	return closeErr
}

// ConnectAttempts is a diagnostic counter used by health/tests.
func (cm *ConnectionManager) ConnectAttempts() int64 {
	if cm == nil {
		return 0
	}
	return cm.attempts.Load()
}

func (cm *ConnectionManager) IsConnected() bool {
	conn := cm.GetConnection()
	return cm.GetState() == StateConnected && conn != nil && !conn.IsClosed()
}

func nextReconnectDelay(previous, base, max time.Duration) time.Duration {
	if previous < base {
		return base
	}
	if previous >= max/2 {
		return max
	}
	return previous * 2
}

func waitContext(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return false
		default:
			return true
		}
	}
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (cm *ConnectionManager) logError(format string, args ...interface{}) {
	fmt.Printf("[messaging] "+format+"\n", args...)
}
