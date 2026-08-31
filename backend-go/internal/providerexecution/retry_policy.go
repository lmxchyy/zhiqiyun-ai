package providerexecution

import (
	"errors"
	"net"
	"time"
)

type ErrorClass string

const (
	DefinitiveNotSubmitted ErrorClass = "definitive_not_submitted"
	DefinitiveFailed       ErrorClass = "definitive_failed"
	RetryableBeforeSubmit  ErrorClass = "retryable_before_submit"
	PossiblySubmitted      ErrorClass = "possibly_submitted"
	ProviderProcessing     ErrorClass = "provider_processing"
	ProviderSucceeded      ErrorClass = "provider_succeeded"
	ProviderUnknown        ErrorClass = "provider_unknown"
)

type RetryDecision struct {
	Retry      bool
	QueryFirst bool
	Delay      time.Duration
	Reason     string
}
type ProviderPolicy struct {
	QuerySupported           bool
	IdempotencySupported     bool
	SafeResubmitAfterUnknown bool
}

type ClassifiedError struct {
	Class ErrorClass
	Err   error
}

func (e ClassifiedError) Error() string {
	if e.Err == nil {
		return string(e.Class)
	}
	return e.Err.Error()
}
func (e ClassifiedError) Unwrap() error { return e.Err }
func Classify(err error) ErrorClass {
	var ce ClassifiedError
	if errors.As(err, &ce) {
		return ce.Class
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return PossiblySubmitted
	}
	return PossiblySubmitted
}

// Decide is deliberately conservative: transport uncertainty never becomes a
// blind Create retry. Query-first is required whenever a provider can be queried.
func Decide(policy ProviderPolicy, class ErrorClass) RetryDecision {
	switch class {
	case DefinitiveNotSubmitted:
		return RetryDecision{Retry: true, Reason: "provider definitively did not receive request"}
	case RetryableBeforeSubmit:
		return RetryDecision{Retry: true, Delay: time.Second, Reason: "failure occurred before request submission"}
	case PossiblySubmitted, ProviderUnknown:
		if policy.QuerySupported {
			return RetryDecision{QueryFirst: true, Reason: "request outcome is uncertain; query provider first"}
		}
		return RetryDecision{Reason: "unknown outcome; automatic resubmission is unsafe"}
	case ProviderProcessing:
		return RetryDecision{QueryFirst: true, Reason: "provider is still processing"}
	case ProviderSucceeded:
		return RetryDecision{Reason: "provider already succeeded"}
	default:
		return RetryDecision{Reason: "definitive provider failure is not automatically retried"}
	}
}
