package messaging

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

const (
	OutboxPending    = "pending"
	OutboxPublishing = "publishing"
	OutboxPublished  = "published"
	OutboxFailed     = "failed"
)

type OutboxStore struct {
	DB    *sql.DB
	Lease time.Duration
}

func NewOutboxStore(db *sql.DB) *OutboxStore { return &OutboxStore{DB: db, Lease: StaleClaimThreshold} }

// InsertTx writes an envelope in the caller's transaction. The transaction must
// contain the business state change that caused the event.
func (s *OutboxStore) InsertTx(ctx context.Context, tx *sql.Tx, e *Envelope, aggregateType, aggregateID, traceID string) error {
	if s == nil || tx == nil || e == nil {
		return fmt.Errorf("outbox insert requires transaction and envelope")
	}
	if err := e.Validate(); err != nil {
		return err
	}
	payload, err := e.Payload()
	if err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339, e.OccurredAt); err != nil {
		return fmt.Errorf("invalid occurred_at: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO outbox_events
		(event_id, aggregate_type, aggregate_id, event_type, event_version, payload, trace_id, status, next_attempt_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'pending',now())`, e.EventID, aggregateType, aggregateID, e.EventType, e.Version, payload, traceID)
	return err
}

// Claim atomically leases rows without deleting them. PostgreSQL row locks make
// concurrent API replicas safe; stale publishing rows are reclaimable.
func (s *OutboxStore) Claim(ctx context.Context, batch int, owner string) ([]OutboxEventRow, error) {
	if s == nil || s.DB == nil {
		return nil, fmt.Errorf("outbox database is required")
	}
	if batch <= 0 {
		return nil, nil
	}
	if owner == "" {
		return nil, fmt.Errorf("claim owner is required")
	}
	lease := s.Lease
	if lease <= 0 {
		lease = StaleClaimThreshold
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,event_id,event_type,event_version,occurred_at,trace_id,
		'',aggregate_type,aggregate_id,payload,status,attempt_count,published_at,last_error,created_at,updated_at
		FROM outbox_events WHERE ((status='pending' AND next_attempt_at<=now())
		 OR (status='publishing' AND claimed_at < now()-$1::interval))
		ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT $2`, fmt.Sprintf("%f seconds", lease.Seconds()), batch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []OutboxEventRow
	for rows.Next() {
		var r OutboxEventRow
		if err := rows.Scan(&r.ID, &r.EventID, &r.EventType, &r.Version, &r.OccurredAt, &r.TraceID, &r.Producer, &r.AggregateType, &r.AggregateID, &r.Data, &r.Status, &r.AttemptCount, &r.PublishedAt, &r.Error, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, r := range result {
		if _, err := tx.ExecContext(ctx, `UPDATE outbox_events SET status='publishing',claim_owner=$1,claimed_at=now(),updated_at=now() WHERE id=$2`, owner, r.ID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *OutboxStore) MarkPublished(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE outbox_events SET status='published',published_at=now(),claimed_at=NULL,claim_owner=NULL,updated_at=now() WHERE id=$1 AND status='publishing'`, id)
	return err
}
func (s *OutboxStore) MarkFailure(ctx context.Context, id int64, publishErr error, maxAttempts int, next time.Time) error {
	status := OutboxPending
	if maxAttempts > 0 {
		var attempts int
		_ = s.DB.QueryRowContext(ctx, `SELECT attempt_count FROM outbox_events WHERE id=$1`, id).Scan(&attempts)
		if attempts+1 >= maxAttempts {
			status = OutboxFailed
		}
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE outbox_events SET status=$1,attempt_count=attempt_count+1,last_error=$2,next_attempt_at=$3,claimed_at=NULL,claim_owner=NULL,updated_at=now() WHERE id=$4 AND status='publishing'`, status, errorString(publishErr), next, id)
	return err
}
func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// DecodeOutboxEnvelope validates the JSON payload kept as the source of truth.
func DecodeOutboxEnvelope(payload []byte) (*Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(payload, &e); err != nil {
		return nil, err
	}
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return &e, nil
}
