package messaging

import (
	"database/sql"
	"time"
)

// OutboxEvent represents a row in the outbox_events table.
type OutboxEvent struct {
	ID            int64      `db:"id"`
	EventID       string     `db:"event_id"`
	EventType     string     `db:"event_type"`
	Version       int        `db:"version"`
	OccurredAt    time.Time  `db:"occurred_at"`
	TraceID       string     `db:"trace_id"`
	Producer      string     `db:"producer"`
	AggregateType string     `db:"aggregate_type"`
	AggregateID   string     `db:"aggregate_id"`
	Data          []byte     `db:"data"`
	Status        string     `db:"status"` // pending, publishing, published, failed
	AttemptCount  int        `db:"attempt_count"`
	PublishedAt   *time.Time `db:"published_at"`
	Error         *string    `db:"error"`
	CreatedAt     time.Time  `db:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at"`
}

// ConsumerInbox represents a row in the consumer_inbox table.
type ConsumerInbox struct {
	ID           int64      `db:"id"`
	ConsumerName string     `db:"consumer_name"`
	EventID      string     `db:"event_id"`
	ProcessedAt  *time.Time `db:"processed_at"`
	Result       *string    `db:"result"`
	Metadata     []byte     `db:"metadata"`
	ErrorMessage *string    `db:"error_message"`
	CreatedAt    time.Time  `db:"created_at"`
}

// OutboxEventRow is the raw row type returned by the database query.
// It mirrors the migration schema exactly.
type OutboxEventRow struct {
	ID            int64          `db:"id"`
	EventID       string         `db:"event_id"`
	EventType     string         `db:"event_type"`
	Version       int            `db:"version"`
	OccurredAt    time.Time      `db:"occurred_at"`
	TraceID       string         `db:"trace_id"`
	Producer      string         `db:"producer"`
	AggregateType string         `db:"aggregate_type"`
	AggregateID   string         `db:"aggregate_id"`
	Data          []byte         `db:"data"`
	Status        string         `db:"status"`
	AttemptCount  int            `db:"attempt_count"`
	PublishedAt   sql.NullTime   `db:"published_at"`
	Error         sql.NullString `db:"error"`
	CreatedAt     time.Time      `db:"created_at"`
	UpdatedAt     time.Time      `db:"updated_at"`
}

// ConsumerInboxRow is the raw row type returned by the database query.
type ConsumerInboxRow struct {
	ID           int64          `db:"id"`
	ConsumerName string         `db:"consumer_name"`
	EventID      string         `db:"event_id"`
	ProcessedAt  sql.NullTime   `db:"processed_at"`
	Result       sql.NullString `db:"result"`
	Metadata     []byte         `db:"metadata"`
	ErrorMessage sql.NullString `db:"error_message"`
	CreatedAt    time.Time      `db:"created_at"`
}

// ToOutboxEvent converts a row to a domain OutboxEvent.
func (r OutboxEventRow) ToOutboxEvent() OutboxEvent {
	var publishedAt *time.Time
	if r.PublishedAt.Valid {
		publishedAt = &r.PublishedAt.Time
	}
	var err *string
	if r.Error.Valid {
		err = &r.Error.String
	}
	return OutboxEvent{
		ID:            r.ID,
		EventID:       r.EventID,
		EventType:     r.EventType,
		Version:       r.Version,
		OccurredAt:    r.OccurredAt,
		TraceID:       r.TraceID,
		Producer:      r.Producer,
		AggregateType: r.AggregateType,
		AggregateID:   r.AggregateID,
		Data:          r.Data,
		Status:        r.Status,
		PublishedAt:   publishedAt,
		Error:         err,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

// ToConsumerInbox converts a row to a domain ConsumerInbox.
func (r ConsumerInboxRow) ToConsumerInbox() ConsumerInbox {
	var processedAt *time.Time
	if r.ProcessedAt.Valid {
		processedAt = &r.ProcessedAt.Time
	}
	var result *string
	if r.Result.Valid {
		result = &r.Result.String
	}
	var errorMessage *string
	if r.ErrorMessage.Valid {
		errorMessage = &r.ErrorMessage.String
	}
	return ConsumerInbox{
		ID:           r.ID,
		ConsumerName: r.ConsumerName,
		EventID:      r.EventID,
		ProcessedAt:  processedAt,
		Result:       result,
		Metadata:     r.Metadata,
		ErrorMessage: errorMessage,
		CreatedAt:    r.CreatedAt,
	}
}
