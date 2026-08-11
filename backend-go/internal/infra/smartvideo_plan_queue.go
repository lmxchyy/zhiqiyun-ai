package infra

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
)

type SmartVideoPlanQueue struct {
	client                                      *redis.Client
	pendingKey, workingKey, delayedKey, deadKey string
	recoverOnce                                 sync.Once
}

func NewSmartVideoPlanQueue(client *redis.Client) *SmartVideoPlanQueue {
	if client == nil {
		return nil
	}
	const prefix = "xianzhi:smartvideo:plan:"
	return &SmartVideoPlanQueue{
		client: client, pendingKey: prefix + "pending", workingKey: prefix + "working",
		delayedKey: prefix + "delayed", deadKey: prefix + "dead",
	}
}

func (q *SmartVideoPlanQueue) Enqueue(ctx context.Context, job smartvideo.PlanJob, delay time.Duration) error {
	if q == nil || q.client == nil || job.TaskID == "" {
		return smartvideo.ErrAnalysisNotReady
	}
	payload, err := json.Marshal(smartvideo.PlanJob{TaskID: job.TaskID})
	if err != nil {
		return err
	}
	if delay > 0 {
		return q.client.ZAdd(ctx, q.delayedKey, redis.Z{
			Score: float64(time.Now().UTC().Add(delay).UnixMilli()), Member: string(payload),
		}).Err()
	}
	return q.client.LPush(ctx, q.pendingKey, payload).Err()
}

func (q *SmartVideoPlanQueue) Publish(ctx context.Context, taskID string) error {
	return q.Enqueue(ctx, smartvideo.PlanJob{TaskID: taskID}, 0)
}

func (q *SmartVideoPlanQueue) Run(ctx context.Context, handler func(context.Context, smartvideo.PlanJob) smartvideo.QueueDecision) error {
	if q == nil || q.client == nil {
		return smartvideo.ErrAnalysisNotReady
	}
	q.recoverOnce.Do(func() { q.recoverWorking(ctx) })
	for ctx.Err() == nil {
		_ = promoteSmartVideoDelayed.Run(ctx, q.client, []string{q.delayedKey, q.pendingKey},
			strconv.FormatInt(time.Now().UTC().UnixMilli(), 10), "100").Err()
		payload, err := q.client.BRPopLPush(ctx, q.pendingKey, q.workingKey, 5*time.Second).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("smartvideo_plan_queue operation=dequeue result=failed")
			continue
		}
		var job smartvideo.PlanJob
		if json.Unmarshal([]byte(payload), &job) != nil || job.TaskID == "" {
			_ = deadLetterSmartVideoJob.Run(ctx, q.client, []string{q.workingKey, q.deadKey}, payload).Err()
			continue
		}
		decision := handler(ctx, job)
		var transitionErr error
		switch {
		case decision.Dead:
			transitionErr = deadLetterSmartVideoJob.Run(ctx, q.client, []string{q.workingKey, q.deadKey}, payload).Err()
		case decision.RetryAfter > 0:
			transitionErr = retrySmartVideoJob.Run(
				ctx, q.client, []string{q.workingKey, q.delayedKey}, payload,
				strconv.FormatInt(time.Now().UTC().Add(decision.RetryAfter).UnixMilli(), 10),
			).Err()
		default:
			transitionErr = q.client.LRem(ctx, q.workingKey, 1, payload).Err()
		}
		if transitionErr != nil && ctx.Err() == nil {
			log.Printf("smartvideo_plan_queue operation=ack task_id=%s result=failed", job.TaskID)
		}
	}
	return ctx.Err()
}

func (q *SmartVideoPlanQueue) recoverWorking(ctx context.Context) {
	for ctx.Err() == nil {
		payload, err := q.client.RPopLPush(ctx, q.workingKey, q.pendingKey).Result()
		if errors.Is(err, redis.Nil) || payload == "" {
			return
		}
		if err != nil {
			log.Printf("smartvideo_plan_queue operation=recover result=failed")
			return
		}
	}
}

func (q *SmartVideoPlanQueue) Depth(ctx context.Context) (SmartVideoQueueDepth, error) {
	if q == nil || q.client == nil {
		return SmartVideoQueueDepth{}, smartvideo.ErrAnalysisNotReady
	}
	pipe := q.client.Pipeline()
	pending := pipe.LLen(ctx, q.pendingKey)
	working := pipe.LLen(ctx, q.workingKey)
	delayed := pipe.ZCard(ctx, q.delayedKey)
	dead := pipe.LLen(ctx, q.deadKey)
	if _, err := pipe.Exec(ctx); err != nil {
		return SmartVideoQueueDepth{}, err
	}
	return SmartVideoQueueDepth{
		Pending: pending.Val(), Working: working.Val(), Delayed: delayed.Val(), Dead: dead.Val(),
	}, nil
}
