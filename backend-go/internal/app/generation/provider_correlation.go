package generation

import (
	"context"
	"strings"
)

// ProviderCorrelationEvent contains only safe execution-routing facts. It
// deliberately excludes prompt bodies, credentials, full URLs, and payloads.
type ProviderCorrelationEvent struct {
	Kind         string
	ProviderCode string
	Host         string
	Path         string
	JobID        string
	JobRole      string
	State        string
	HTTPStatus   int
	ErrorCode    string
	ErrorHash    string
}

type providerCorrelationListenerKey struct{}

func WithProviderCorrelationListener(ctx context.Context, fn func(ProviderCorrelationEvent)) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, providerCorrelationListenerKey{}, fn)
}

func NotifyProviderCorrelation(ctx context.Context, event ProviderCorrelationEvent) {
	if ctx == nil || strings.TrimSpace(event.Kind) == "" {
		return
	}
	if fn, ok := ctx.Value(providerCorrelationListenerKey{}).(func(ProviderCorrelationEvent)); ok && fn != nil {
		event.ProviderCode = strings.TrimSpace(event.ProviderCode)
		event.Host = strings.TrimSpace(event.Host)
		event.Path = strings.TrimSpace(event.Path)
		event.JobID = strings.TrimSpace(event.JobID)
		event.JobRole = strings.TrimSpace(event.JobRole)
		event.State = strings.TrimSpace(event.State)
		event.ErrorCode = strings.TrimSpace(event.ErrorCode)
		event.ErrorHash = strings.TrimSpace(event.ErrorHash)
		fn(event)
	}
}
