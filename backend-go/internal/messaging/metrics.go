package messaging

import (
	"fmt"
	"sync/atomic"
	"time"
)

// Metrics holds the Prometheus-style counters and gauges for messaging.
// It is safe for concurrent use.
type Metrics struct {
	// Publish related.
	MessagesPublished atomic.Int64
	PublishErrors     atomic.Int64
	PublishLatencyMs  atomic.Int64
	ConfirmLatencyMs  atomic.Int64

	// Consumer related.
	MessagesConsumed    atomic.Int64
	ConsumptionErrors   atomic.Int64
	ProcessingLatencyMs atomic.Int64
	Rejections          atomic.Int64
	Nacks               atomic.Int64

	// Connection related.
	ConnectionState  atomic.Int64 // 0=disconnected, 1=connecting, 2=connected, 3=closing
	ReconnectCount   atomic.Int64
	ConnectionErrors atomic.Int64

	// General.
	OutboxPendingEvents atomic.Int64
	ShutdownRequests    atomic.Int64

	startTime time.Time
}

// NewMetrics creates a new Metrics instance.
func NewMetrics() *Metrics {
	return &Metrics{
		startTime: time.Now().UTC(),
	}
}

// Uptime returns the uptime since metrics creation.
func (m *Metrics) Uptime() time.Duration {
	return time.Since(m.startTime)
}

// String returns a Prometheus-style text exposition of the metrics.
// This matches the existing metrics pattern in the repository.
func (m *Metrics) String() string {
	return fmt.Sprintf(
		`# HELP messaging_messages_total Total messages processed.
# TYPE messaging_messages_total counter
messaging_messages_published_total %d
messaging_messages_publish_errors_total %d
messaging_messages_consume_total %d
messaging_messages_consume_errors_total %d
messaging_messages_rejections_total %d
messaging_messages_nacks_total %d
# HELP messaging_latency_seconds Latency in milliseconds.
# TYPE messaging_latency_seconds gauge
messaging_publish_latency_ms %d
messaging_confirm_latency_ms %d
messaging_processing_latency_ms %d
# HELP messaging_connection_state Connection state (0=disconnected, 1=connecting, 2=connected, 3=closing).
# TYPE messaging_connection_state gauge
messaging_connection_state %d
messaging_reconnect_total %d
messaging_connection_errors_total %d
messaging_outbox_pending_events %d
messaging_shutdown_requests_total %d
`,
		m.MessagesPublished.Load(),
		m.PublishErrors.Load(),
		m.MessagesConsumed.Load(),
		m.ConsumptionErrors.Load(),
		m.Rejections.Load(),
		m.Nacks.Load(),
		m.PublishLatencyMs.Load(),
		m.ConfirmLatencyMs.Load(),
		m.ProcessingLatencyMs.Load(),
		m.ConnectionState.Load(),
		m.ReconnectCount.Load(),
		m.ConnectionErrors.Load(),
		m.OutboxPendingEvents.Load(),
		m.ShutdownRequests.Load(),
	)
}

// RecordPublish records a successful publish with latency.
func (m *Metrics) RecordPublish(latencyMs int64) {
	m.MessagesPublished.Add(1)
	if latencyMs > 0 {
		m.PublishLatencyMs.Store(latencyMs)
	}
}

// RecordPublishError records a publish error.
func (m *Metrics) RecordPublishError() {
	m.PublishErrors.Add(1)
}

// RecordConsume records a successful consumption with latency.
func (m *Metrics) RecordConsume(latencyMs int64) {
	m.MessagesConsumed.Add(1)
	if latencyMs > 0 {
		m.ProcessingLatencyMs.Store(latencyMs)
	}
}

// RecordConsumeError records a consumption error.
func (m *Metrics) RecordConsumeError() {
	m.ConsumptionErrors.Add(1)
}

// RecordConnectionState updates the connection state.
func (m *Metrics) RecordConnectionState(state ConnectionState) {
	m.ConnectionState.Store(int64(state))
}

// RecordReconnect increments the reconnect counter.
func (m *Metrics) RecordReconnect() {
	m.ReconnectCount.Add(1)
}

// RecordConnectionError increments the connection error counter.
func (m *Metrics) RecordConnectionError() {
	m.ConnectionErrors.Add(1)
}

// SetOutboxPendingEvents sets the number of pending outbox events.
func (m *Metrics) SetOutboxPendingEvents(n int64) {
	m.OutboxPendingEvents.Store(n)
}

// RecordShutdown increments the shutdown request counter.
func (m *Metrics) RecordShutdown() {
	m.ShutdownRequests.Add(1)
}
