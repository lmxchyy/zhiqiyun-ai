package httpserver

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const pptVisualLockTTL = 8 * time.Minute

type pptVisualDistributedLocker interface {
	TryAcquire(context.Context, string, time.Duration) (func(), bool, error)
}

type redisPPTVisualLocker struct {
	client *redis.Client
}

func newRedisPPTVisualLocker(client *redis.Client) pptVisualDistributedLocker {
	if client == nil {
		return nil
	}
	return redisPPTVisualLocker{client: client}
}

func (l redisPPTVisualLocker) TryAcquire(ctx context.Context, key string, ttl time.Duration) (func(), bool, error) {
	if l.client == nil {
		return func() {}, false, errors.New("ppt visual redis locker is unavailable")
	}
	if ttl <= 0 {
		ttl = pptVisualLockTTL
	}
	redisKey := "ppt:visual-lock:" + key
	token := newRequestID()
	acquired, err := l.client.SetNX(ctx, redisKey, token, ttl).Result()
	if err != nil || !acquired {
		return func() {}, acquired, err
	}
	release := func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = l.client.Eval(releaseCtx, `
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("del", KEYS[1])
end
return 0
`, []string{redisKey}, token).Result()
	}
	return release, true, nil
}
