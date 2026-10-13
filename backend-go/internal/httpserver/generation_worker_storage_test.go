package httpserver

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/messaging"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

func TestGenerationImageWorkerStorageConstructor(t *testing.T) {
	db := new(sql.DB) // Constructor must not need a running database or provider.
	cfg := config.Config{DataPath: filepath.Join(t.TempDir(), "data.json")}
	worker := newGenerationImageWorkerAPI(db, cfg)
	if worker.fileService == nil {
		t.Fatal("async image consumer must receive the HTTP API's private StorageService")
	}
	store, ok := worker.store.(*postgresStore)
	if !ok || store.db != db {
		t.Fatal("async image consumer must use its supplied PostgreSQL database")
	}
}

// Run the actual normal consumer (including its production constructor), real
// PostgreSQL transactions and RabbitMQ delivery. Provider and S3 HTTP traffic
// use loopback test servers only; no production task/account/media is touched.
func TestIssue190PostgresImageWorkerStorage(t *testing.T) {
	db := issue190DB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rabbitURL := os.Getenv("XIANZHI_MESSAGING_TEST_RABBITMQ_URL")
	if rabbitURL == "" {
		t.Fatal("isolated RabbitMQ required for actual normal image consumer")
	}
	// The dedicated migration-only test DB must not redirect artifacts to a
	// seeded/cloud storage config instead of this test's loopback S3 endpoint.
	var defaults int
	if err := db.QueryRow(`SELECT count(*) FROM xz_storage_configs WHERE deleted_at IS NULL AND is_default AND status='ENABLED' AND tenant_id IN ('tenant_default','platform')`).Scan(&defaults); err != nil || defaults != 0 {
		t.Fatalf("isolated storage configuration required defaults=%d err=%v", defaults, err)
	}
	f := issue190SetupAtPrice(t, db, imageDispatchNormal, false, 10)
	if f.task.PointCost != 10 {
		t.Fatalf("server quote must reserve10, got %d", f.task.PointCost)
	}
	var puts atomic.Int32
	storageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusOK) // BucketExists; PutObject uses configured region.
		case http.MethodPut:
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				http.Error(w, "read failed", 500)
				return
			}
			puts.Add(1)
			w.Header().Set("ETag", `"worker-storage-object"`)
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected S3 method", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(storageServer.Close)
	cfg := config.Config{DataPath: filepath.Join(t.TempDir(), "data.json"), ProviderExecutionSafetyEnabled: true, AsyncMessagingEnabled: true,
		StorageDefaultProvider: "s3", S3Endpoint: storageServer.URL, S3Region: "us-east-1", S3AccessKey: "isolated-access", S3SecretKey: "isolated-secret", S3Bucket: "worker-private", StorageForcePathStyle: true,
		StorageMasterKey: "0123456789abcdef0123456789abcdef", StorageMaxUploadBytes: "1048576", StorageDefaultQuotaBytes: "1048576"}
	manager, err := messaging.NewConnectionManager(messaging.RabbitMQConfig{URL: rabbitURL})
	if err != nil {
		t.Fatal(err)
	}
	manager.Start()
	defer manager.Close()
	if _, err := manager.WaitForConnection(ctx); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- RunGenerationImageNormalWorker(ctx, cfg, db, manager) }()
	defer func() {
		cancel()
		manager.Close()
		select {
		case err := <-done:
			if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
				t.Errorf("consumer shutdown: %v", err)
			}
		case <-time.After(35 * time.Second): // Consumer.Stop has a native 30s bound.
			t.Error("consumer did not stop within its native shutdown bound")
		}
	}()
	publisher := messaging.NewPublisher(manager)
	if err := publisher.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer publisher.Close()
	event := f.dispatch(t)
	if err := publisher.Publish(ctx, event, event.EventType); err != nil {
		t.Fatal(err)
	}
	waitProcessed := func(eventID string) {
		t.Helper()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			var processed bool
			if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM consumer_inbox WHERE consumer_name='generation-image-normal-worker' AND event_id=$1 AND processed_at IS NOT NULL)`, eventID).Scan(&processed); err != nil {
				t.Fatal(err)
			}
			if processed {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatalf("normal consumer did not finish event %s: %v", eventID, ctx.Err())
			case <-ticker.C:
			}
		}
	}
	waitProcessed(event.EventID)
	assertCompleted := func(wantInbox int) {
		t.Helper()
		f.assertEffects(t, true, 1, wantInbox)
		gen, state, _, _ := f.taskState(t)
		execution, err := pe.NewStore(db).GetLatestByTask(ctx, f.task.ID)
		if err != nil || execution.Status != pe.Succeeded || execution.TaskGeneration == nil || *execution.TaskGeneration != gen || gen != eventGeneration(event)+1 || state != taskStatusSucceeded {
			t.Fatalf("generation/provider/task completion gen=%d state=%s execution=%+v err=%v", gen, state, execution, err)
		}
		var files, linked int
		if err := db.QueryRow(`SELECT (SELECT count(*) FROM xz_file_objects WHERE business_id=$1 AND business_type='generation_result' AND status='ACTIVE' AND visibility='PRIVATE' AND file_size>0),(SELECT count(*) FROM xz_assets a JOIN xz_file_objects f ON a.metadata->>'fileId'=f.file_id AND a.metadata->>'storageManaged'='true' WHERE a.task_id=$1 AND f.business_id=$1 AND a.media_type='image')`, f.task.ID).Scan(&files, &linked); err != nil || files != 1 || linked != 1 {
			t.Fatalf("private file/artwork association files=%d linked=%d err=%v", files, linked, err)
		}
		status, reserved, captured, released := getReservationState(t, db, f.task.PersonalPointReservationID)
		if status != "CAPTURED" || reserved != 0 || captured != 10 || released != 0 || puts.Load() != 1 || f.provider.calls.Load() != 1 {
			t.Fatalf("capture/storage/provider status=%s reserve=%d capture=%d release=%d PUT=%d Generate=%d", status, reserved, captured, released, puts.Load(), f.provider.calls.Load())
		}
	}
	assertCompleted(1)
	// Distinct transport event, SAME completed task: wait for a durable terminal
	// inbox ACK, then prove no extra provider/object/file/artwork/capture or bump.
	replay := *event
	replay.EventID = "issue190:" + f.task.ID + ":terminal-redelivery"
	if err := publisher.Publish(ctx, &replay, replay.EventType); err != nil {
		t.Fatal(err)
	}
	waitProcessed(replay.EventID)
	assertCompleted(2)
	t.Logf("WORKER_STORAGE_CONSUMER_PASS task=%s provider_success=1 private_file=1 artwork=1 SUCCEEDED=1 CAPTURE=10 capture_events=1 generation=%d duplicate_event_ack=1 PUT=1", f.task.ID, eventGeneration(event)+1)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM xz_file_objects WHERE business_id=$1`, f.task.ID)
	})
}

func eventGeneration(event *messaging.Envelope) int64 {
	return envelopeExecutionGeneration(event.Data)
}
