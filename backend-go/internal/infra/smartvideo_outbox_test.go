package infra

import (
	"context"
	"errors"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
)

type memoryOutboxRepo struct {
	events   []smartvideo.OutboxEvent
	requeued []requeueCall
}

type requeueCall struct {
	ID    int64
	Delay time.Duration
	Err   string
}

func (r *memoryOutboxRepo) PublishOutbox(_ context.Context, limit int) ([]smartvideo.OutboxEvent, error) {
	if limit <= 0 || len(r.events) == 0 {
		return nil, nil
	}
	if limit > len(r.events) {
		limit = len(r.events)
	}
	batch := append([]smartvideo.OutboxEvent{}, r.events[:limit]...)
	r.events = r.events[limit:]
	return batch, nil
}

func (r *memoryOutboxRepo) MarkOutboxFailed(_ context.Context, _ int64, _ string) error { return nil }

func (r *memoryOutboxRepo) RequeueOutbox(_ context.Context, eventID int64, delay time.Duration, errMsg string) error {
	r.requeued = append(r.requeued, requeueCall{ID: eventID, Delay: delay, Err: errMsg})
	return nil
}

type memoryAnalysisQueue struct {
	jobs  []smartvideo.AnalysisJob
	fail  bool
	calls int
}

func (q *memoryAnalysisQueue) Enqueue(_ context.Context, job smartvideo.AnalysisJob, _ time.Duration) error {
	q.calls++
	if q.fail {
		return errors.New("redis unavailable")
	}
	q.jobs = append(q.jobs, job)
	return nil
}

type memoryPlanQueue struct {
	jobs []smartvideo.PlanJob
}

func (q *memoryPlanQueue) Enqueue(_ context.Context, job smartvideo.PlanJob, _ time.Duration) error {
	q.jobs = append(q.jobs, job)
	return nil
}

type memoryRenderQueue struct {
	jobs []smartvideo.RenderJob
}

func (q *memoryRenderQueue) Enqueue(_ context.Context, job smartvideo.RenderJob, _ time.Duration) error {
	q.jobs = append(q.jobs, job)
	return nil
}

func TestOutboxPublisherDispatchesByAggregateType(t *testing.T) {
	repo := &memoryOutboxRepo{events: []smartvideo.OutboxEvent{
		{ID: 1, AggregateType: "analysis", AggregateID: "a1", EventType: "enqueue_requested"},
		{ID: 2, AggregateType: "plan", AggregateID: "p1", EventType: "enqueue_requested"},
		{ID: 3, AggregateType: "render", AggregateID: "r1", EventType: "enqueue_requested"},
	}}
	analysisQ := &memoryAnalysisQueue{}
	planQ := &memoryPlanQueue{}
	renderQ := &memoryRenderQueue{}
	publisher := NewOutboxPublisher(repo, OutboxQueues{Analysis: analysisQ, Plan: planQ, Render: renderQ}, OutboxPublisherOptions{
		BatchSize: 10, BaseBackoff: time.Second, MaxBackoff: time.Minute,
	})
	if err := publisher.PublishOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(analysisQ.jobs) != 1 || analysisQ.jobs[0].TaskID != "a1" {
		t.Fatalf("analysis jobs = %+v", analysisQ.jobs)
	}
	if len(planQ.jobs) != 1 || planQ.jobs[0].TaskID != "p1" {
		t.Fatalf("plan jobs = %+v", planQ.jobs)
	}
	if len(renderQ.jobs) != 1 || renderQ.jobs[0].TaskID != "r1" {
		t.Fatalf("render jobs = %+v", renderQ.jobs)
	}
	if len(repo.requeued) != 0 {
		t.Fatalf("unexpected requeue: %+v", repo.requeued)
	}
}

func TestOutboxPublisherRequeuesOnRedisFailure(t *testing.T) {
	repo := &memoryOutboxRepo{events: []smartvideo.OutboxEvent{
		{ID: 9, AggregateType: "analysis", AggregateID: "a9", EventType: "enqueue_requested"},
	}}
	analysisQ := &memoryAnalysisQueue{fail: true}
	publisher := NewOutboxPublisher(repo, OutboxQueues{Analysis: analysisQ}, OutboxPublisherOptions{
		BatchSize: 10, BaseBackoff: 3 * time.Second, MaxBackoff: time.Minute,
	})
	if err := publisher.PublishOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if analysisQ.calls != 1 {
		t.Fatalf("enqueue calls = %d", analysisQ.calls)
	}
	if len(repo.requeued) != 1 || repo.requeued[0].ID != 9 || repo.requeued[0].Delay != 3*time.Second {
		t.Fatalf("requeued = %+v", repo.requeued)
	}
}

func TestOutboxPublisherIgnoresDuplicateDeliverySemantics(t *testing.T) {
	// Consumers must claim idempotently; publisher may deliver the same task ID twice.
	repo := &memoryOutboxRepo{events: []smartvideo.OutboxEvent{
		{ID: 1, AggregateType: "plan", AggregateID: "same", EventType: "enqueue_requested"},
		{ID: 2, AggregateType: "plan", AggregateID: "same", EventType: "enqueue_requested"},
	}}
	planQ := &memoryPlanQueue{}
	publisher := NewOutboxPublisher(repo, OutboxQueues{Plan: planQ}, OutboxPublisherOptions{BatchSize: 10})
	if err := publisher.PublishOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(planQ.jobs) != 2 {
		t.Fatalf("expected duplicate delivery, got %+v", planQ.jobs)
	}
}
