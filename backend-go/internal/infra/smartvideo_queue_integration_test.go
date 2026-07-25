package infra

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
)

func TestSmartVideoQueueRecoversWorkingAndPromotesDelayed(t *testing.T) {
	address := os.Getenv("SMARTVIDEO_REDIS_TEST_ADDR")
	if address == "" {
		t.Skip("SMARTVIDEO_REDIS_TEST_ADDR is not configured")
	}
	client := redis.NewClient(&redis.Options{Addr: address})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	prefix := "test:smartvideo:" + time.Now().UTC().Format("20060102150405.000000000") + ":"
	queue := NewSmartVideoAnalysisQueue(client, prefix)
	defer func() {
		_ = client.Del(context.Background(), queue.pendingKey, queue.workingKey, queue.delayedKey, queue.deadKey).Err()
	}()

	recovered := smartvideo.AnalysisJob{TaskID: "task_recovered", ProjectID: "project_1", AssetID: "asset_1"}
	payload, err := json.Marshal(recovered)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.LPush(ctx, queue.workingKey, payload).Err(); err != nil {
		t.Fatal(err)
	}
	if err := queue.Enqueue(ctx, smartvideo.AnalysisJob{
		TaskID: "task_delayed", ProjectID: "project_1", AssetID: "asset_2",
	}, 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)

	var handled atomic.Int32
	runCtx, stop := context.WithCancel(ctx)
	handledAll := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- queue.Run(runCtx, func(_ context.Context, job smartvideo.AnalysisJob) smartvideo.QueueDecision {
			if job.TaskID != "task_recovered" && job.TaskID != "task_delayed" {
				t.Errorf("unexpected queue payload: %+v", job)
			}
			if handled.Add(1) == 2 {
				close(handledAll)
			}
			return smartvideo.QueueDecision{}
		})
	}()
	select {
	case <-handledAll:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		working, err := client.LLen(context.Background(), queue.workingKey).Result()
		if err != nil {
			t.Fatal(err)
		}
		if working == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("working queue contains %d unacknowledged jobs", working)
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context cancellation", err)
	}
	if handled.Load() != 2 {
		t.Fatalf("handled %d jobs, want 2", handled.Load())
	}
}
