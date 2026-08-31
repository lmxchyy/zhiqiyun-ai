package messaging

import (
	"context"
	"fmt"
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
		if err := p.publishBatch(ctx, batch, owner); err != nil && ctx.Err() != nil {
			return ctx.Err()
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
	for _, row := range rows {
		env, err := DecodeOutboxEnvelope(row.Data)
		if err == nil {
			err = p.Publisher.Publish(ctx, env, env.EventType)
		}
		if err == nil {
			err = p.Store.MarkPublished(ctx, row.ID)
		} else {
			_ = p.Store.MarkFailure(ctx, row.ID, err, DefaultRetry().MaxAttempts, time.Now().UTC().Add(DefaultRetry().NextDelay(row.AttemptCount+1)))
		}
		if err != nil {
			return err
		}
	}
	return nil
}
