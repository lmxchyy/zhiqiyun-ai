package httpserver

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGenerationErrorMessageDeadline(t *testing.T) {
	if got := generationErrorMessage(context.DeadlineExceeded); got != "生成超时，请稍后重试" {
		t.Fatalf("deadline message = %q", got)
	}
	if !errors.Is(context.DeadlineExceeded, context.DeadlineExceeded) {
		t.Fatal("sanity check failed")
	}
}

func TestGenerationStaleWatchdogIntervalIsPeriodic(t *testing.T) {
	if generationStaleWatchdogInterval <= 0 {
		t.Fatal("stale watchdog interval must be positive")
	}
}

func TestGenerationStaleWatchdogCanBeStopped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a := &api{}
	a.startGenerationStaleWatchdog(ctx, time.Second)
	a.stopGenerationStaleWatchdog()
}

func TestGenerationUnknownGracePeriodConfigurable(t *testing.T) {
	t.Setenv("XIANZHI_GENERATION_UNKNOWN_GRACE", "7m")
	if got := generationUnknownGracePeriod(); got != 7*time.Minute {
		t.Fatalf("unknown grace = %s, want 7m", got)
	}
}
