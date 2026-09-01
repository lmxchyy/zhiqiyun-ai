package main

import (
	"context"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/messaging"
)

func TestAsyncRuntimeDisabledStartsNothing(t *testing.T) {
	runtime := newAsyncMessagingRuntime(config.Config{}, nil, nil)
	runtime.Start(context.Background())
	if got := runtime.Status(); got != asyncStatusDisabled {
		t.Fatalf("status=%s", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := runtime.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAsyncRuntimeEnabledReportsConnectingDegradedAndStopped(t *testing.T) {
	manager, err := messaging.NewConnectionManager(messaging.RabbitMQConfig{URL: "amqp://guest:guest@127.0.0.1:1/"})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newAsyncMessagingRuntime(config.Config{AsyncMessagingEnabled: true}, nil, manager)
	if got := runtime.Status(); got != asyncStatusConnecting {
		t.Fatalf("pre-start status=%s", got)
	}
	runtime.Start(context.Background())
	if got := runtime.Status(); got != asyncStatusDegraded {
		t.Fatalf("missing DB status=%s", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runtime.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if got := runtime.Status(); got != asyncStatusStopped {
		t.Fatalf("stopped status=%s", got)
	}
}

func TestAsyncRuntimeDegradedAfterLosingPreviouslyReadyConnection(t *testing.T) {
	runtime := &asyncMessagingRuntime{enabled: true, everReady: true}
	if got := runtime.Status(); got != asyncStatusDegraded {
		t.Fatalf("status=%s", got)
	}
}
