package httpserver

import (
	"context"
	"testing"
	"time"
)

func TestSelectAPIChannelForModelKeepsHighestPriorityPrimary(t *testing.T) {
	channels := []adminAPIChannel{
		{ID: "channel_runtime_env", Primary: true, Priority: 1, Status: "ACTIVE", Models: []string{"gpt-image-2"}},
		{ID: "channel_openai", Primary: true, Priority: 20, Status: "CONFIGURABLE", Models: []string{"gpt-image-2"}},
	}

	selected, ok := selectAPIChannelForModel(channels, "gpt-image-2")
	if !ok {
		t.Fatal("expected a channel match")
	}
	if selected.ID != "channel_runtime_env" {
		t.Fatalf("selected channel=%s, want channel_runtime_env", selected.ID)
	}
}

func TestSelectAPIChannelForModelPrefersPrimaryOverNonPrimary(t *testing.T) {
	channels := []adminAPIChannel{
		{ID: "channel_non_primary", Priority: 1, Status: "ACTIVE", Models: []string{"gpt-image-2"}},
		{ID: "channel_primary", Primary: true, Priority: 20, Status: "ACTIVE", Models: []string{"gpt-image-2"}},
	}

	selected, ok := selectAPIChannelForModel(channels, "gpt-image-2")
	if !ok {
		t.Fatal("expected a channel match")
	}
	if selected.ID != "channel_primary" {
		t.Fatalf("selected channel=%s, want channel_primary", selected.ID)
	}
}

func TestConnectorFailureContextDetachesExpiredParent(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	ctx, cancel := connectorFailureContext(parent)
	defer cancel()
	if err := ctx.Err(); err != nil {
		t.Fatalf("failure context inherited cancellation: %v", err)
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("failure context must have a deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > connectorFailureHandlingTimeout {
		t.Fatalf("unexpected failure context deadline: %s", remaining)
	}
}
