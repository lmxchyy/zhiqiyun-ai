package messaging

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestEnvelopeRoundTripAndValidation(t *testing.T) {
	e := MustEnvelope("x.ai.test", "test", "42", map[string]interface{}{"ok": true})
	payload, err := e.Payload()
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeEnvelope(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got.EventID != e.EventID || got.Version != 1 || got.OccurredAt == "" {
		t.Fatalf("round trip lost required fields: %+v", got)
	}
	if err := (&Envelope{}).Validate(); err == nil {
		t.Fatal("expected required field validation error")
	}
	if _, err := DecodeEnvelope([]byte("not-json")); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestEnvelopeVersionIsExplicit(t *testing.T) {
	payload, _ := json.Marshal(map[string]interface{}{"event_id": "e", "event_type": "t", "version": 1, "aggregate_type": "a", "aggregate_id": "1"})
	if _, err := DecodeEnvelope(payload); err != nil {
		t.Fatal(err)
	}
	payload, _ = json.Marshal(map[string]interface{}{"event_id": "e", "event_type": "t", "aggregate_type": "a", "aggregate_id": "1"})
	if _, err := DecodeEnvelope(payload); err == nil {
		t.Fatal("expected missing version error")
	}
}

func TestRetryBackoffBounded(t *testing.T) {
	r := RetryStrategy{InitialDelay: time.Second, MaxDelay: 5 * time.Second, Multiplier: 2, MaxAttempts: 10}
	if r.NextDelay(1) != time.Second || r.NextDelay(3) != 4*time.Second || r.NextDelay(5) != 5*time.Second {
		t.Fatalf("unexpected backoff")
	}
}

func TestInboxRequiresTransactionalHandler(t *testing.T) {
	store := &InboxStore{}
	if _, err := store.ProcessTx(context.Background(), nil, "consumer", "event", func(context.Context) (string, map[string]any, error) { return "", nil, nil }); err == nil {
		t.Fatal("expected transaction requirement")
	}
}

func TestVideoCanaryRoutingKeyValidation(t *testing.T) {
	if err := ValidateRoutingKey(GenerationVideoCanaryRoutingKey); err != nil {
		t.Fatalf("GenerationVideoCanaryRoutingKey %q must be valid: %v", GenerationVideoCanaryRoutingKey, err)
	}
	if err := ValidateRoutingKey(GenerationVideoCanaryRetryKey); err != nil {
		t.Fatalf("GenerationVideoCanaryRetryKey %q must be valid: %v", GenerationVideoCanaryRetryKey, err)
	}
	if GenerationVideoCanaryQueue == "" || GenerationVideoCanaryDLQ == "" {
		t.Fatal("video queue names must not be empty")
	}
}
