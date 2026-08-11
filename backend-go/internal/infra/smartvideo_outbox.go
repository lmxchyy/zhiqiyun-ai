package infra

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
)

type OutboxPublisherOptions struct {
	BatchSize     int
	PollInterval  time.Duration
	MaxAttempts   int
	BaseBackoff   time.Duration
	MaxBackoff    time.Duration
}

type OutboxQueues struct {
	Analysis smartvideo.AnalysisQueue
	Plan     smartvideo.PlanQueue
	Render   smartvideo.RenderQueue
}

type OutboxPublisher struct {
	repository smartvideo.OutboxRepository
	queues     OutboxQueues
	options    OutboxPublisherOptions
}

func NewOutboxPublisher(repository smartvideo.OutboxRepository, queues OutboxQueues, options OutboxPublisherOptions) *OutboxPublisher {
	if options.BatchSize <= 0 {
		options.BatchSize = 50
	}
	if options.PollInterval <= 0 {
		options.PollInterval = 2 * time.Second
	}
	if options.MaxAttempts <= 0 {
		options.MaxAttempts = 8
	}
	if options.BaseBackoff <= 0 {
		options.BaseBackoff = 2 * time.Second
	}
	if options.MaxBackoff <= 0 {
		options.MaxBackoff = 5 * time.Minute
	}
	return &OutboxPublisher{repository: repository, queues: queues, options: options}
}

func (p *OutboxPublisher) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.options.PollInterval)
	defer ticker.Stop()
	for {
		if err := p.PublishOnce(ctx); err != nil && ctx.Err() == nil {
			log.Printf("smartvideo_outbox operation=publish_once result=failed")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (p *OutboxPublisher) PublishOnce(ctx context.Context) error {
	events, err := p.repository.PublishOutbox(ctx, p.options.BatchSize)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := p.dispatch(ctx, event); err != nil {
			if event.Attempts >= p.options.MaxAttempts {
				_ = p.repository.MarkOutboxFailed(ctx, event.ID, trimOutboxError(err))
				continue
			}
			delay := p.backoff(event.Attempts)
			_ = p.repository.RequeueOutbox(ctx, event.ID, delay, trimOutboxError(err))
			continue
		}
	}
	return nil
}

func (p *OutboxPublisher) dispatch(ctx context.Context, event smartvideo.OutboxEvent) error {
	taskID := extractOutboxTaskID(event)
	if taskID == "" {
		return smartvideo.ErrInvalidInput
	}
	switch strings.ToLower(strings.TrimSpace(event.AggregateType)) {
	case "analysis":
		if p.queues.Analysis == nil {
			return smartvideo.ErrAnalysisNotReady
		}
		return p.queues.Analysis.Enqueue(ctx, smartvideo.AnalysisJob{TaskID: taskID}, 0)
	case "plan":
		if p.queues.Plan == nil {
			return smartvideo.ErrAnalysisNotReady
		}
		return p.queues.Plan.Enqueue(ctx, smartvideo.PlanJob{TaskID: taskID}, 0)
	case "render":
		if p.queues.Render == nil {
			return smartvideo.ErrAnalysisNotReady
		}
		return p.queues.Render.Enqueue(ctx, smartvideo.RenderJob{TaskID: taskID}, 0)
	default:
		return smartvideo.ErrInvalidInput
	}
}

func (p *OutboxPublisher) backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := p.options.BaseBackoff
	for i := 1; i < attempts; i++ {
		if delay >= p.options.MaxBackoff/2 {
			return p.options.MaxBackoff
		}
		delay *= 2
	}
	if delay > p.options.MaxBackoff {
		return p.options.MaxBackoff
	}
	return delay
}

func extractOutboxTaskID(event smartvideo.OutboxEvent) string {
	if id := strings.TrimSpace(event.AggregateID); id != "" {
		return id
	}
	if len(event.Payload) == 0 {
		return ""
	}
	var payload map[string]string
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload["taskId"])
}

func trimOutboxError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 500 {
		return msg[:500]
	}
	return msg
}
