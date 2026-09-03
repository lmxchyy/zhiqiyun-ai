package httpserver

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestAsyncCanaryOperationalMetricsSampleDurablePostgresState(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	taskID := "metrics-canary-" + suffix
	eventID := "metrics-event-" + suffix
	defer func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM provider_executions WHERE task_id=$1`, taskID)
		_, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE event_id=$1`, eventID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_generation_tasks WHERE id=$1`, taskID)
	}()
	old := time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `INSERT INTO outbox_events(event_id,aggregate_type,aggregate_id,event_type,event_version,payload,status,attempt_count,next_attempt_at,created_at,updated_at) VALUES($1,'generation_task',$2,$3,1,'{}','pending',2,now(),now()-interval '20 minutes',now())`, eventID, taskID, "x.ai.generation.image.canary.requested"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_generation_tasks(id,user_id,type,model,status,params,created_at,updated_at,billing_status,raw) VALUES($1,'metrics-user','TEXT_TO_IMAGE','gpt-image-2','PROCESSING','{"generation_async_canary":true}',$2,$2,'RESERVED','{}')`, taskID, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO provider_executions(task_id,provider,provider_model,capability,attempt,status,request_fingerprint,updated_at) VALUES($1,'configured','gpt-image-2','image',1,'unknown',$2,now()-interval '20 minutes')`, taskID, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	snapshot := (&asyncCanaryOperationalCollector{db: db}).snapshot()
	if snapshot.dbScrapeOK != 1 || snapshot.outboxPending < 1 || snapshot.outboxPublishRetries < 2 {
		t.Fatalf("outbox snapshot not updated: %+v", snapshot)
	}
	if snapshot.providerCount["unknown"] < 1 || snapshot.providerAge["unknown"] < 1100 {
		t.Fatalf("provider snapshot not updated: count=%v age=%v", snapshot.providerCount, snapshot.providerAge)
	}
	if snapshot.generationStuckCount < 1 || snapshot.pointsUnsettledCount < 1 || snapshot.pointsUnsettledAge < 1100 {
		t.Fatalf("generation/points snapshot not updated: %+v", snapshot)
	}
}
