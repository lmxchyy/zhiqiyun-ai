package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const connectorQueueMaxAttempts = 3

var connectorQueueMoveScript = redis.NewScript(`
local removed = redis.call('LREM', KEYS[1], 1, ARGV[1])
if removed > 0 then
  redis.call('RPUSH', KEYS[2], ARGV[2])
end
return removed
`)

type connectorJobQueue struct {
	redis      *redis.Client
	pendingKey string
	workingKey string
	dlqKey     string
	attemptKey string
	local      chan connectorJob
}

func newConnectorJobQueue(redisClient *redis.Client, prefix string) *connectorJobQueue {
	if prefix == "" {
		prefix = "xianzhi:connector:jobs:"
	}
	return &connectorJobQueue{
		redis:      redisClient,
		pendingKey: prefix + "pending",
		workingKey: prefix + "working",
		dlqKey:     prefix + "dlq",
		attemptKey: prefix + "attempts",
		local:      make(chan connectorJob, 256),
	}
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
			q.moveToDLQ(ctx, payload, "invalid connector job payload")
			continue
		}
		if err := handler(ctx, job); err != nil {
			attempts, retryErr := q.recordFailure(ctx, payload)
			log.Printf("connector_queue operation=process message_id=%s result=failed attempt=%d error=%v", job.MessageID, attempts, err)
			if retryErr != nil {
				log.Printf("connector_queue operation=retry message_id=%s result=failed error=%v", job.MessageID, retryErr)
				continue
			}
			if attempts < connectorQueueMaxAttempts {
				if err := q.movePayload(ctx, q.pendingKey, payload); err != nil {
					log.Printf("connector_queue operation=requeue message_id=%s result=failed error=%v", job.MessageID, err)
				}
				continue
			}
			q.moveToDLQ(ctx, payload, "maximum connector processing attempts exceeded")
			continue
		}
		_ = q.redis.LRem(ctx, q.workingKey, 1, payload).Err()
		_ = q.redis.HDel(ctx, q.attemptKey, job.MessageID).Err()
	}
}

func (q *connectorJobQueue) runLocal(ctx context.Context, handler func(context.Context, connectorJob) error) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-q.local:
			var err error
			for attempt := 1; attempt <= connectorQueueMaxAttempts; attempt++ {
				err = handler(ctx, job)
				if err == nil {
					break
				}
				log.Printf("connector_queue mode=local operation=process message_id=%s result=failed attempt=%d error=%v", job.MessageID, attempt, err)
			}
			if err != nil {
				log.Printf("connector_queue mode=local operation=dlq message_id=%s result=failed attempts=%d", job.MessageID, connectorQueueMaxAttempts)
			}
		}
	}
}

func (q *connectorJobQueue) recordFailure(ctx context.Context, payload string) (int64, error) {
	var job connectorJob
	if err := json.Unmarshal([]byte(payload), &job); err != nil || job.MessageID == "" {
		return connectorQueueMaxAttempts, errors.New("connector job has no message id")
	}
	return q.redis.HIncrBy(ctx, q.attemptKey, job.MessageID, 1).Result()
}

func (q *connectorJobQueue) movePayload(ctx context.Context, destination, payload string) error {
	return q.movePayloadValue(ctx, destination, payload, payload)
}

func (q *connectorJobQueue) movePayloadValue(ctx context.Context, destination, payload, value string) error {
	if q.redis == nil {
		return errors.New("redis connector queue is unavailable")
	}
	return connectorQueueMoveScript.Run(ctx, q.redis, []string{q.workingKey, destination}, payload, value).Err()
}

func (q *connectorJobQueue) moveToDLQ(ctx context.Context, payload, reason string) {
	if q.redis == nil {
		return
	}
	entry := connectorJSON(map[string]string{"payload": payload, "reason": reason})
	if err := q.movePayloadValue(ctx, q.dlqKey, payload, string(entry)); err != nil {
		log.Printf("connector_queue operation=dlq_move result=failed error=%v", err)
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
