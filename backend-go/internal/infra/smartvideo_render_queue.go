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

type SmartVideoRenderQueue struct {
	client                                      *redis.Client
	pendingKey, workingKey, delayedKey, deadKey string
	recoverOnce                                 sync.Once
}

func NewSmartVideoRenderQueue(client *redis.Client) *SmartVideoRenderQueue {
	if client == nil {
		return nil
	}
	const prefix = "xianzhi:smartvideo:render:"
	return &SmartVideoRenderQueue{client: client, pendingKey: prefix + "pending", workingKey: prefix + "working", delayedKey: prefix + "delayed", deadKey: prefix + "dead"}
}

func (q *SmartVideoRenderQueue) Enqueue(ctx context.Context, job smartvideo.RenderJob, delay time.Duration) error {
	if q == nil || q.client == nil || job.TaskID == "" {
		return smartvideo.ErrAnalysisNotReady
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	if delay > 0 {
		return q.client.ZAdd(ctx, q.delayedKey, redis.Z{Score: float64(time.Now().Add(delay).UnixMilli()), Member: string(payload)}).Err()
	}
	return q.client.LPush(ctx, q.pendingKey, payload).Err()
}

func (q *SmartVideoRenderQueue) Run(ctx context.Context, handler func(context.Context, smartvideo.RenderJob) smartvideo.QueueDecision) error {
	q.recoverOnce.Do(func() {
		for {
			value, err := q.client.RPopLPush(ctx, q.workingKey, q.pendingKey).Result()
			if errors.Is(err, redis.Nil) || value == "" {
				break
			}
			if err != nil {
				break
			}
		}
	})
	for ctx.Err() == nil {
		_ = promoteSmartVideoDelayed.Run(ctx, q.client, []string{q.delayedKey, q.pendingKey},
			strconv.FormatInt(time.Now().UnixMilli(), 10), "100").Err()
		payload, err := q.client.BRPopLPush(ctx, q.pendingKey, q.workingKey, 5*time.Second).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		var job smartvideo.RenderJob
		if json.Unmarshal([]byte(payload), &job) != nil || job.TaskID == "" {
			_ = q.client.LRem(ctx, q.workingKey, 1, payload).Err()
			_ = q.client.LPush(ctx, q.deadKey, payload).Err()
			continue
		}
		decision := handler(ctx, job)
		if decision.Dead {
			_ = q.client.LPush(ctx, q.deadKey, payload).Err()
		} else if decision.RetryAfter > 0 {
			_ = q.client.ZAdd(ctx, q.delayedKey, redis.Z{Score: float64(time.Now().Add(decision.RetryAfter).UnixMilli()), Member: payload}).Err()
		}
		if err := q.client.LRem(ctx, q.workingKey, 1, payload).Err(); err != nil && ctx.Err() == nil {
			log.Printf("smartvideo_render_queue operation=ack task_id=%s result=failed", job.TaskID)
		}
	}
	return ctx.Err()
}
