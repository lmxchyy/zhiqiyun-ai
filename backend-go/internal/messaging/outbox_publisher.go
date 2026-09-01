package messaging

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"
)

// OutboxPublisher is an embedded, opt-in publisher loop. It never creates or
// dispatches business events; callers decide which events enter the outbox.
type OutboxPublisher struct {
	Store        *OutboxStore
	Publisher    *Publisher
	BatchSize    int
	PollInterval time.Duration
	Owner        string
	Retry        RetryStrategy
}

func (p *OutboxPublisher) Run(ctx context.Context) error {
	if p == nil || p.Store == nil || p.Publisher == nil {
		return fmt.Errorf("outbox publisher dependencies are required")
	}
	batch := p.BatchSize
	if batch <= 0 {
		batch = 50
	}
	interval := p.PollInterval
	if interval <= 0 {
		interval = time.Second
	}
	owner := p.Owner
	if owner == "" {
		owner, _ = os.Hostname()
	}
	if owner == "" {
		owner = "api"
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if err := p.publishBatch(ctx, batch, owner); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("messaging outbox batch failed owner=%s: %v", owner, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (p *OutboxPublisher) publishBatch(ctx context.Context, batch int, owner string) error {
	rows, err := p.Store.Claim(ctx, batch, owner)
	if err != nil {
		return err
	}
	retry := p.Retry
	if retry.MaxAttempts <= 0 {
		retry = DefaultRetry()
	}
	var failures []error
	for _, row := range rows {
		env, publishErr := DecodeOutboxEnvelope(row.Data)
		if publishErr == nil {
			publishCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			publishErr = p.Publisher.Publish(publishCtx, env, env.EventType)
			cancel()
		}
		if publishErr == nil {
			if markErr := p.Store.MarkPublished(ctx, row.ID); markErr != nil {
				failures = append(failures, fmt.Errorf("mark outbox %d published: %w", row.ID, markErr))
			}
			continue
		}
		next := time.Now().UTC().Add(retry.NextDelay(row.AttemptCount + 1))
		if markErr := p.Store.MarkFailure(ctx, row.ID, publishErr, retry.MaxAttempts, next); markErr != nil {
			failures = append(failures, fmt.Errorf("outbox %d publish failed (%v), mark failure: %w", row.ID, publishErr, markErr))
		} else {
			failures = append(failures, fmt.Errorf("outbox %d publish: %w", row.ID, publishErr))
		}
	}
	return errors.Join(failures...)
}
