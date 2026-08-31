package messaging

import (
	"time"
)

// AsyncMessagingOptions holds all configuration for the async messaging package.
type AsyncMessagingOptions struct {
	// Enabled controls whether async messaging is active.
	// Default false. Production defaults to false until traffic is switched.
	Enabled bool

	// RabbitMQ URL for the connection.
	// Overrides the global RabbitMQURL config when set.
	RabbitMQURL string

	// Prefetch is the number of unacknowledged messages to fetch.
	// Default 1.
	Prefetch int

	// PublisherConfirms enables publisher confirmations.
	// Default true.
	PublisherConfirms bool

	// RetryMaxAttempts is the maximum number of publish retries.
	// Default 15.
	RetryMaxAttempts int

	// RetryInitialDelay is the initial backoff delay.
	// Default 1 second.
	RetryInitialDelay time.Duration

	// RetryMaxDelay is the maximum backoff delay.
	// Default 10 minutes.
	RetryMaxDelay time.Duration

	// Heartbeat is the AMQP heartbeat interval.
	// Default 10 seconds.
	Heartbeat time.Duration

	// StaleClaimThreshold is the duration after which a stale publishing
	// claim can be reclaimed.
	// Default 5 minutes.
	StaleClaimThreshold time.Duration

	// MetricsInterval is the interval for metrics collection.
	// Set to 0 to disable periodic collection.
	MetricsInterval time.Duration

	// GracefulShutdownTimeout is the time allowed for graceful shutdown.
	// Default 30 seconds.
	GracefulShutdownTimeout time.Duration
}

// ApplyDefaults sets default values for unset options.
func (o *AsyncMessagingOptions) ApplyDefaults() {
	if o.Prefetch <= 0 {
		o.Prefetch = DefaultPrefetch
	}
	if o.RetryMaxAttempts <= 0 {
		o.RetryMaxAttempts = DefaultRetry().MaxAttempts
	}
	if o.RetryInitialDelay <= 0 {
		o.RetryInitialDelay = DefaultRetry().InitialDelay
	}
	if o.RetryMaxDelay <= 0 {
		o.RetryMaxDelay = DefaultRetry().MaxDelay
	}
	if o.Heartbeat <= 0 {
		o.Heartbeat = 10 * time.Second
	}
	if o.StaleClaimThreshold <= 0 {
		o.StaleClaimThreshold = StaleClaimThreshold
	}
	if o.GracefulShutdownTimeout <= 0 {
		o.GracefulShutdownTimeout = 30 * time.Second
	}
}

// Validate checks that the options are valid for use.
func (o *AsyncMessagingOptions) Validate() error {
	if o.Prefetch < 0 {
		return errInvalidPrefetch
	}
	if o.RetryMaxAttempts <= 0 {
		return errInvalidRetryAttempts
	}
	if o.GracefulShutdownTimeout <= 0 {
		return errInvalidShutdownTimeout
	}
	return nil
}

var (
	errInvalidPrefetch        = ErrMessaging("prefetch must be non-negative")
	errInvalidRetryAttempts   = ErrMessaging("retry max attempts must be positive")
	errInvalidShutdownTimeout = ErrMessaging("graceful shutdown timeout must be positive")
)

// ErrMessaging represents a messaging configuration error.
type ErrMessaging string

func (e ErrMessaging) Error() string {
	return string(e)
}
