package httpserver

import (
	"errors"
	"testing"
)

func TestShouldFallbackPPTOutlineForTransientProviderFailures(t *testing.T) {
	for _, message := range []string{
		`chat provider returned HTTP 503: {"code":"system_memory_overloaded"}`,
		"chat provider returned HTTP 429: rate limited",
		"Post request: context deadline exceeded",
		"connection reset by peer",
	} {
		if !shouldFallbackPPTOutline(errors.New(message)) {
			t.Fatalf("expected local outline fallback for %q", message)
		}
	}
}

func TestShouldFallbackPPTOutlineKeepsPermanentFailuresVisible(t *testing.T) {
	for _, message := range []string{
		"chat provider returned HTTP 401: invalid token",
		"chat provider does not support model configured-model",
		"decode chat provider response: invalid character",
	} {
		if shouldFallbackPPTOutline(errors.New(message)) {
			t.Fatalf("unexpected local outline fallback for %q", message)
		}
	}
}
