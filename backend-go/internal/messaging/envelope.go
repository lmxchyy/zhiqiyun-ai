// Package messaging implements reliable messaging infrastructure for PR1:
// RabbitMQ connection, message envelope, outbox publisher, consumer inbox,
// topology declaration, retry/backoff, and metrics.
//
// Business behavior is NOT changed. This package provides infrastructure only.
package messaging

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"
)

// EventID generates a unique event identifier using UUID v4 semantics.
func EventID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate event id: %w", err)
	}
	// Set version (4) and variant (RFC 4122)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// Envelope represents a generic message envelope for reliable messaging.
// Database is the source of truth; RabbitMQ is trigger/transport only.
type Envelope struct {
	EventID       string                 `json:"event_id"`
	EventType     string                 `json:"event_type"`
	Version       int                    `json:"version"`
	OccurredAt    string                 `json:"occurred_at"`
	TraceID       string                 `json:"trace_id,omitempty"`
	Producer      string                 `json:"producer"`
	AggregateType string                 `json:"aggregate_type"`
	AggregateID   string                 `json:"aggregate_id"`
	Data          map[string]interface{} `json:"data"`
}

// NewEnvelope creates a new Envelope with a unique event_id and current UTC time.
func NewEnvelope(eventType, aggregateType, aggregateID string, data map[string]interface{}) (*Envelope, error) {
	eventID, err := EventID()
	if err != nil {
		return nil, fmt.Errorf("create envelope: %w", err)
	}
	return &Envelope{
		EventID:       eventID,
		EventType:     eventType,
		Version:       1,
		OccurredAt:    time.Now().UTC().Format(time.RFC3339),
		Producer:      "xianzhi-ai-go-gin",
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		Data:          data,
	}, nil
}

// MustEnvelope is like NewEnvelope but panics on error (for test convenience).
func MustEnvelope(eventType, aggregateType, aggregateID string, data map[string]interface{}) *Envelope {
	e, err := NewEnvelope(eventType, aggregateType, aggregateID, data)
	if err != nil {
		panic(err)
	}
	return e
}

// EnvelopePayload returns the envelope as JSON bytes for publishing.
func (e *Envelope) Payload() ([]byte, error) {
	if e == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(e)
}

// DecodeEnvelope decodes JSON bytes into an Envelope.
func DecodeEnvelope(payload []byte) (*Envelope, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}
	var e Envelope
	if err := json.Unmarshal(payload, &e); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}
	if e.EventID == "" {
		return nil, fmt.Errorf("envelope missing event_id")
	}
	if e.Version == 0 {
		return nil, fmt.Errorf("envelope missing version")
	}
	return &e, nil
}

// Validate returns nil if the envelope has all required fields.
func (e *Envelope) Validate() error {
	if e == nil {
		return fmt.Errorf("nil envelope")
	}
	if e.EventID == "" {
		return fmt.Errorf("envelope missing event_id")
	}
	if e.EventType == "" {
		return fmt.Errorf("envelope missing event_type")
	}
	if e.Version == 0 {
		return fmt.Errorf("envelope missing version")
	}
	if e.AggregateType == "" {
		return fmt.Errorf("envelope missing aggregate_type")
	}
	if e.AggregateID == "" {
		return fmt.Errorf("envelope missing aggregate_id")
	}
	return nil
}
