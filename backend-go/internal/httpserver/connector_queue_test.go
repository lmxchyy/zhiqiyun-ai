package httpserver

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestConnectorRedisQueueRecoveryAndAck(t *testing.T) {
	rawURL := strings.TrimSpace(os.Getenv("XIANZHI_CONNECTOR_TEST_REDIS_URL"))
	if rawURL == "" {
		t.Skip("XIANZHI_CONNECTOR_TEST_REDIS_URL is not configured")
	}
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(options)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Fatalf("ping Redis: %v", err)
	}
	prefix := "test:xianzhi:connector:" + newConnectorID("queue") + ":"
	queue := newConnectorJobQueue(client, prefix)
	t.Cleanup(func() {
		_ = client.Del(context.Background(), queue.pendingKey, queue.workingKey).Err()
		_ = client.Close()
	})
	job := connectorJob{MessageID: "message-recovery"}
	if err := queue.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}
	payload, err := client.BRPopLPush(ctx, queue.pendingKey, queue.workingKey, time.Second).Result()
	if err != nil || payload == "" {
		t.Fatalf("move to working payload=%q err=%v", payload, err)
	}
	queue.recoverWorking(ctx)
	if pending, _ := client.LLen(ctx, queue.pendingKey).Result(); pending != 1 {
		t.Fatalf("pending after recovery=%d, want 1", pending)
	}
	if working, _ := client.LLen(ctx, queue.workingKey).Result(); working != 0 {
		t.Fatalf("working after recovery=%d, want 0", working)
	}

	runCtx, stop := context.WithCancel(context.Background())
	handled := make(chan connectorJob, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		queue.Run(runCtx, func(_ context.Context, item connectorJob) error {
			handled <- item
			return nil
		})
	}()
	select {
	case item := <-handled:
		if item.MessageID != job.MessageID {
			t.Fatalf("handled=%+v", item)
		}
		stop()
	case <-time.After(5 * time.Second):
		stop()
		t.Fatal("connector queue handler timed out")
	}
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("connector queue did not stop")
	}
	if pending, _ := client.LLen(ctx, queue.pendingKey).Result(); pending != 0 {
		t.Fatalf("pending after ack=%d, want 0", pending)
	}
	if working, _ := client.LLen(ctx, queue.workingKey).Result(); working != 0 {
		t.Fatalf("working after ack=%d, want 0", working)
	}
}
