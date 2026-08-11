package config

import (
	"testing"
	"time"
)

func TestAPIShutdownTimeout(t *testing.T) {
	t.Parallel()

	if got := (Config{ShutdownTimeout: "17s"}).APIShutdownTimeout(); got != 17*time.Second {
		t.Fatalf("APIShutdownTimeout() = %s, want 17s", got)
	}
	if got := (Config{}).APIShutdownTimeout(); got != 30*time.Second {
		t.Fatalf("empty APIShutdownTimeout() = %s, want safe 30s default", got)
	}
	if got := (Config{ShutdownTimeout: "invalid"}).APIShutdownTimeout(); got != 30*time.Second {
		t.Fatalf("invalid APIShutdownTimeout() = %s, want safe 30s default", got)
	}
}

func TestValidateProductionRejectsUnsafeShutdownTimeout(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"0s", "-1s", "11m", "invalid"} {
		cfg := Config{Environment: "test", ShutdownTimeout: value}
		if err := cfg.ValidateProduction(); err == nil {
			t.Fatalf("ValidateProduction(%q) unexpectedly succeeded", value)
		}
	}

	if err := (Config{Environment: "test"}).ValidateProduction(); err != nil {
		t.Fatalf("empty timeout should retain the backward-compatible 30s default: %v", err)
	}
}
