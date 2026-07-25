package infra

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
)

type SmartVideoAnalysisQueue struct {
	client      *redis.Client
	pendingKey  string
	workingKey  string
	delayedKey  string
	deadKey     string
	recoverOnce sync.Once
}

var promoteSmartVideoDelayed = redis.NewScript(`
local jobs = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1], 'LIMIT', 0, ARGV[2])
for _, job in ipairs(jobs) do
	if redis.call('ZREM', KEYS[1], job) == 1 then
		redis.call('LPUSH', KEYS[2], job)
	end
end
return #jobs
`)

var retrySmartVideoJob = redis.NewScript(`
redis.call('ZADD', KEYS[2], ARGV[2], ARGV[1])
return redis.call('LREM', KEYS[1], 1, ARGV[1])
`)

var deadLetterSmartVideoJob = redis.NewScript(`
redis.call('LPUSH', KEYS[2], ARGV[1])
return redis.call('LREM', KEYS[1], 1, ARGV[1])
`)

func NewSmartVideoAnalysisQueue(client *redis.Client, prefix string) *SmartVideoAnalysisQueue {
	if client == nil {
		return nil
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "xianzhi:smartvideo:analysis:"
	}
	return &SmartVideoAnalysisQueue{
		client: client, pendingKey: prefix + "pending", workingKey: prefix + "working",
		delayedKey: prefix + "delayed", deadKey: prefix + "dead",
	}
}

func (q *SmartVideoAnalysisQueue) Enqueue(ctx context.Context, job smartvideo.AnalysisJob, delay time.Duration) error {
	if q == nil || q.client == nil {
		return smartvideo.ErrAnalysisNotReady
	}
	if strings.TrimSpace(job.TaskID) == "" {
		return smartvideo.ErrInvalidInput
	}
	payload, err := json.Marshal(job)
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

func (q *SmartVideoAnalysisQueue) Run(ctx context.Context, handler func(context.Context, smartvideo.AnalysisJob) smartvideo.QueueDecision) error {
	if q == nil || q.client == nil {
		return smartvideo.ErrAnalysisNotReady
	}
	q.recoverOnce.Do(func() { q.recoverWorking(ctx) })
	for ctx.Err() == nil {
		q.promoteDelayed(ctx, 100)
		payload, err := q.client.BRPopLPush(ctx, q.pendingKey, q.workingKey, 5*time.Second).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("smartvideo_queue operation=dequeue result=failed")
			continue
		}
		var job smartvideo.AnalysisJob
		if err := json.Unmarshal([]byte(payload), &job); err != nil || strings.TrimSpace(job.TaskID) == "" {
			_ = deadLetterSmartVideoJob.Run(ctx, q.client, []string{q.workingKey, q.deadKey}, payload).Err()
			continue
		}
		decision := handler(ctx, job)
		var transitionErr error
		switch {
		case decision.Dead:
			transitionErr = deadLetterSmartVideoJob.Run(
				ctx, q.client, []string{q.workingKey, q.deadKey}, payload,
			).Err()
		case decision.RetryAfter > 0:
			transitionErr = retrySmartVideoJob.Run(
				ctx,
				q.client,
				[]string{q.workingKey, q.delayedKey},
				payload,
				strconv.FormatInt(time.Now().UTC().Add(decision.RetryAfter).UnixMilli(), 10),
			).Err()
		default:
			transitionErr = q.client.LRem(ctx, q.workingKey, 1, payload).Err()
		}
		if transitionErr != nil && ctx.Err() == nil {
			log.Printf("smartvideo_queue operation=ack task_id=%s result=failed", job.TaskID)
		}
	}
	return ctx.Err()
}

func (q *SmartVideoAnalysisQueue) recoverWorking(ctx context.Context) {
	for ctx.Err() == nil {
		payload, err := q.client.RPopLPush(ctx, q.workingKey, q.pendingKey).Result()
		if errors.Is(err, redis.Nil) || payload == "" {
			return
		}
		if err != nil {
			log.Printf("smartvideo_queue operation=recover result=failed")
			return
		}
	}
}

func (q *SmartVideoAnalysisQueue) promoteDelayed(ctx context.Context, limit int64) {
	max := strconv.FormatInt(time.Now().UTC().UnixMilli(), 10)
	if err := promoteSmartVideoDelayed.Run(
		ctx,
		q.client,
		[]string{q.delayedKey, q.pendingKey},
		max,
		strconv.FormatInt(limit, 10),
	).Err(); err != nil && ctx.Err() == nil {
		log.Printf("smartvideo_queue operation=promote_delayed result=failed")
	}
}
