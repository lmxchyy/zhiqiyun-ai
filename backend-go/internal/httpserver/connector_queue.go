package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

type connectorJobQueue struct {
	redis      *redis.Client
	pendingKey string
	workingKey string
	local      chan connectorJob
}

func newConnectorJobQueue(redisClient *redis.Client, prefix string) *connectorJobQueue {
	if prefix == "" {
		prefix = "xianzhi:connector:jobs:"
	}
	return &connectorJobQueue{redis: redisClient, pendingKey: prefix + "pending", workingKey: prefix + "working", local: make(chan connectorJob, 256)}
}

func (q *connectorJobQueue) Enqueue(ctx context.Context, job connectorJob) error {
	if job.MessageID == "" {
		return errors.New("connector job message id is required")
	}
	if q.redis != nil {
		return q.redis.LPush(ctx, q.pendingKey, connectorJSON(job)).Err()
	}
	select {
	case q.local <- job:
		return nil
	default:
		return errors.New("local connector queue is full")
	}
}

func (q *connectorJobQueue) Run(ctx context.Context, handler func(context.Context, connectorJob) error) {
	if q.redis == nil {
		q.runLocal(ctx, handler)
		return
	}
	q.recoverWorking(ctx)
	for ctx.Err() == nil {
		payload, err := q.redis.BRPopLPush(ctx, q.pendingKey, q.workingKey, 5*time.Second).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("connector_queue operation=dequeue result=failed error=%v", err)
			}
			continue
		}
		var job connectorJob
		if err := json.Unmarshal([]byte(payload), &job); err != nil {
			_ = q.redis.LRem(ctx, q.workingKey, 1, payload).Err()
			continue
		}
		if err := handler(ctx, job); err != nil {
			log.Printf("connector_queue operation=process message_id=%s result=failed error=%v", job.MessageID, err)
		}
		_ = q.redis.LRem(ctx, q.workingKey, 1, payload).Err()
	}
}

func (q *connectorJobQueue) runLocal(ctx context.Context, handler func(context.Context, connectorJob) error) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-q.local:
			if err := handler(ctx, job); err != nil {
				log.Printf("connector_queue mode=local operation=process message_id=%s result=failed error=%v", job.MessageID, err)
			}
		}
	}
}

func (q *connectorJobQueue) recoverWorking(ctx context.Context) {
	for {
		moved, err := q.redis.RPopLPush(ctx, q.workingKey, q.pendingKey).Result()
		if errors.Is(err, redis.Nil) || moved == "" {
			return
		}
		if err != nil {
			log.Printf("connector_queue operation=recover result=failed error=%v", err)
			return
		}
	}
}
