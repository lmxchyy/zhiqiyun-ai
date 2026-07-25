package httpserver

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestSMSDailyLimitsUseSharedRedisCounters(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("XIANZHI_SMS_TEST_REDIS_URL"))
	if redisURL == "" {
		t.Skip("XIANZHI_SMS_TEST_REDIS_URL is not configured")
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	store := redisAuthSessions{client: client}
	namespace := fmt.Sprintf("zhiqiyun:test:sms:%d", time.Now().UnixNano())

	for _, dimension := range []string{"mobile", "ip", "device"} {
		key := namespace + ":daily:" + dimension + ":identity"
		t.Cleanup(func() { _ = client.Del(context.Background(), "auth:sms:rate:"+key).Err() })
		for attempt := int64(1); attempt <= 3; attempt++ {
			allowed, ttl, err := store.AllowSMSRequest(context.Background(), key, 2, time.Hour)
			if err != nil {
				t.Fatalf("%s attempt %d: %v", dimension, attempt, err)
			}
			if allowed != (attempt <= 2) {
				t.Fatalf("%s attempt %d allowed=%v", dimension, attempt, allowed)
			}
			if ttl <= 0 {
				t.Fatalf("%s attempt %d ttl=%s", dimension, attempt, ttl)
			}
		}
	}
}
