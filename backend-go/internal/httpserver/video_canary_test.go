package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/messaging"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

func stage0VideoCanaryConfig() config.Config {
	return config.Config{
		AsyncMessagingEnabled:             true,
		VideoAsyncCanaryEnabled:           true,
		ProviderExecutionSafetyEnabled:    true,
		VideoAsyncCanaryUsers:             "user-video-canary",
		VideoAsyncCanaryProviderAllowlist: "channel-video-stage0",
		VideoAsyncCanaryModelAllowlist:    "grok-imagine-1.5-video",
	}
}

func stage0VideoCanaryRequest() generation.CreateRequest {
	return generation.CreateRequest{
		UserID: "user-video-canary",
		Type:   "TEXT_TO_VIDEO",
		Model:  "grok-imagine-1.5-video",
		Params: map[string]any{"provider": "channel-video-stage0"},
	}
}

func TestVideoAsyncCanary_AllowedTypesSelectAsync(t *testing.T) {
	resetAsyncCanaryProcessMetricsForTest()
	for _, taskType := range []string{"TEXT_TO_VIDEO", "IMAGE_TO_VIDEO", "VIDEO_TO_VIDEO"} {
		req := stage0VideoCanaryRequest()
		req.Type = taskType
		selected, reason := (api{cfg: stage0VideoCanaryConfig()}).videoAsyncCanaryDecision(req)
		if !selected || reason != canaryReasonSelected {
			t.Errorf("type %q selected=%v reason=%s", taskType, selected, reason)
		}
	}
}

func TestVideoAsyncCanary_NonVideoTypesRejected(t *testing.T) {
	for _, taskType := range []string{"TEXT_TO_IMAGE", "IMAGE_TO_IMAGE", "PPT", "CHAT", "AUDIO", ""} {
		req := stage0VideoCanaryRequest()
		req.Type = taskType
		selected, reason := (api{cfg: stage0VideoCanaryConfig()}).videoAsyncCanaryDecision(req)
		if selected || reason != canaryReasonRejectedType {
			t.Errorf("type %q selected=%v reason=%s, want rejected_type", taskType, selected, reason)
		}
	}
}

func TestVideoAsyncCanary_DisabledConfigFallsBackSync(t *testing.T) {
	for name, cfg := range map[string]config.Config{
		"video canary disabled": func() config.Config {
			c := stage0VideoCanaryConfig()
			c.VideoAsyncCanaryEnabled = false
			return c
		}(),
		"async messaging disabled": func() config.Config {
			c := stage0VideoCanaryConfig()
			c.AsyncMessagingEnabled = false
			return c
		}(),
		"provider safety disabled": func() config.Config {
			c := stage0VideoCanaryConfig()
			c.ProviderExecutionSafetyEnabled = false
			return c
		}(),
	} {
		selected, reason := (api{cfg: cfg}).videoAsyncCanaryDecision(stage0VideoCanaryRequest())
		if selected || reason != canaryReasonDisabled {
			t.Errorf("%s selected=%v reason=%s, want disabled", name, selected, reason)
		}
	}
}

func TestVideoAsyncCanary_AllowlistsFailClosed(t *testing.T) {
	// Empty users
	cfg := stage0VideoCanaryConfig()
	cfg.VideoAsyncCanaryUsers = ""
	selected, reason := (api{cfg: cfg}).videoAsyncCanaryDecision(stage0VideoCanaryRequest())
	if selected || reason != canaryReasonRejectedUser {
		t.Fatalf("empty users allowlist selected=%v reason=%s", selected, reason)
	}

	// Empty provider
	cfg = stage0VideoCanaryConfig()
	cfg.VideoAsyncCanaryProviderAllowlist = ""
	selected, reason = (api{cfg: cfg}).videoAsyncCanaryDecision(stage0VideoCanaryRequest())
	if selected || reason != canaryReasonRejectedProvider {
		t.Fatalf("empty provider allowlist selected=%v reason=%s", selected, reason)
	}

	// Empty model
	cfg = stage0VideoCanaryConfig()
	cfg.VideoAsyncCanaryModelAllowlist = ""
	selected, reason = (api{cfg: cfg}).videoAsyncCanaryDecision(stage0VideoCanaryRequest())
	if selected || reason != canaryReasonRejectedModel {
		t.Fatalf("empty model allowlist selected=%v reason=%s", selected, reason)
	}
}

func TestVideoAsyncCanary_WildcardUserMatches(t *testing.T) {
	cfg := stage0VideoCanaryConfig()
	cfg.VideoAsyncCanaryUsers = "*"
	req := stage0VideoCanaryRequest()
	req.UserID = "any-random-user-id"
	selected, reason := (api{cfg: cfg}).videoAsyncCanaryDecision(req)
	if !selected || reason != canaryReasonSelected {
		t.Fatalf("wildcard user selected=%v reason=%s", selected, reason)
	}
}

func TestVideoAsyncCanary_IndependentFromImageCanary(t *testing.T) {
	// Video canary enabled, Image canary disabled
	cfg := stage0VideoCanaryConfig()
	cfg.GenerationAsyncCanaryEnabled = false
	cfg.VideoAsyncCanaryEnabled = true

	videoReq := stage0VideoCanaryRequest()
	selected, _ := (api{cfg: cfg}).videoAsyncCanaryDecision(videoReq)
	if !selected {
		t.Fatal("video canary should be selected when VideoAsyncCanaryEnabled=true even if GenerationAsyncCanaryEnabled=false")
	}

	imageReq := generation.CreateRequest{UserID: "user-video-canary", Type: "TEXT_TO_IMAGE", Model: "gpt-image-2"}
	imgSelected, _ := (api{cfg: cfg}).generationAsyncCanaryDecision(imageReq)
	if imgSelected {
		t.Fatal("image canary should be rejected when GenerationAsyncCanaryEnabled=false")
	}
}

func TestVideoCanary_OutboxCreatedAtomically(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	testUser := "u-vcanary-" + suffix
	accountID := "acc-" + testUser

	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users (id, name, role) VALUES ($1, 'Video Canary User', 'MEMBER') ON CONFLICT (id) DO NOTHING`, testUser); err != nil {
		t.Fatal(err)
	}
	grantID := "grant-" + testUser
	pointStore := NewPostgresPersonalPointStore(db)
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{
		AccountID:      accountID,
		UserID:         testUser,
		Source:         PointSourceRecharge,
		Points:         10000,
		IdempotencyKey: grantID,
	}); err != nil {
		t.Fatal(err)
	}

	store := newPostgresPrimaryStore(db, "")
	req := videoAcceptanceRequest("outbox-atomic-video-" + suffix)
	req.UserID = testUser
	req.Model = "mock-video"
	req.Params["duration"] = 5
	req.Params["resolution"] = "480p"

	task, err := store.CreatePendingGenerationTaskWithVideoCanaryOutbox(req)
	if err != nil {
		t.Fatalf("create pending task with video canary outbox: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id=$1", task.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", task.ID)
	}()

	if task.Status != "PROCESSING" || task.BillingStatus != "RESERVED" {
		t.Fatalf("task state unexpected: status=%s billing=%s", task.Status, task.BillingStatus)
	}

	// Verify outbox_events row exists
	var eventType, status, aggregateType string
	err = db.QueryRowContext(ctx, `SELECT event_type, status, aggregate_type FROM outbox_events WHERE aggregate_id=$1`, task.ID).Scan(&eventType, &status, &aggregateType)
	if err != nil {
		t.Fatalf("query outbox_events: %v", err)
	}
	if eventType != messaging.GenerationVideoCanaryRoutingKey {
		t.Fatalf("eventType=%s, want %s", eventType, messaging.GenerationVideoCanaryRoutingKey)
	}
	if status != "pending" {
		t.Fatalf("outbox status=%s, want pending", status)
	}
	if aggregateType != "generation_task" {
		t.Fatalf("aggregateType=%s, want generation_task", aggregateType)
	}
}

func TestVideoCanary_WorkerDeduplication(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	store := newPostgresPrimaryStore(db, "")
	taskID := "test-worker-dedup-" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM consumer_inbox WHERE event_id=$1", "evt-"+taskID)
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", taskID)
	}()

	if _, err := db.ExecContext(ctx, `INSERT INTO xz_generation_tasks (id,user_id,type,status,params,created_at,updated_at) VALUES ($1,'test-user','TEXT_TO_VIDEO','PENDING','{"generation_async_canary":true}'::jsonb,now()::text,now()::text)`, taskID); err != nil {
		t.Fatal(err)
	}

	inbox := messaging.NewInboxStore(db)
	envelope := &messaging.Envelope{
		EventID:       "evt-" + taskID,
		EventType:     messaging.GenerationVideoCanaryRoutingKey,
		AggregateType: "generation_task",
		AggregateID:   taskID,
		Data:          map[string]interface{}{"task_id": taskID},
	}

	cfg := stage0VideoCanaryConfig()
	apiInstance := newAPI(store, cfg, nil, nil)

	// Pre-seed completed inbox record to simulate already-processed delivery
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = inbox.ClaimTx(ctx, tx, generationVideoCanaryConsumer, envelope.EventID)
	if err := inbox.CompleteTx(ctx, tx, generationVideoCanaryConsumer, envelope.EventID, "completed", map[string]any{"task_id": taskID}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Delivering duplicate envelope must return nil without re-processing
	err = apiInstance.processGenerationVideoCanaryMessage(ctx, inbox, envelope)
	if err != nil {
		t.Fatalf("duplicate delivery should return nil, got: %v", err)
	}
}

func TestVideoCanary_WorkerStillProcessingReturnsErrorForRetry(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	store := newPostgresPrimaryStore(db, "")
	taskID := "test-worker-retry-" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM consumer_inbox WHERE event_id=$1", "evt-"+taskID)
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", taskID)
	}()

	rawJSON := fmt.Sprintf(`{"id":%q,"userId":"test-user","type":"TEXT_TO_VIDEO","model":"mock-video","status":"PENDING","params":{"generation_async_canary":true,"provider":"channel_runtime_env","duration":5,"aspect_ratio":"16:9","resolution":"720p"}}`, taskID)
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_generation_tasks (id,user_id,type,model,status,params,raw,created_at,updated_at) VALUES ($1,'test-user','TEXT_TO_VIDEO','mock-video','PENDING','{"generation_async_canary":true,"provider":"channel_runtime_env","duration":5,"aspect_ratio":"16:9","resolution":"720p"}'::jsonb,$2::jsonb,now()::text,now()::text)`, taskID, rawJSON); err != nil {
		t.Fatal(err)
	}

	inbox := messaging.NewInboxStore(db)
	envelope := &messaging.Envelope{
		EventID:       "evt-" + taskID,
		EventType:     messaging.GenerationVideoCanaryRoutingKey,
		AggregateType: "generation_task",
		AggregateID:   taskID,
		Data:          map[string]interface{}{"task_id": taskID},
	}

	cfg := stage0VideoCanaryConfig()
	videoProv := &mockVideoProvider{
		getFn: func(ctx context.Context, id string) (any, error) {
			return map[string]any{
				"provider":       "mock-video",
				"providerTaskId": id,
				"status":         "PROCESSING",
			}, nil
		},
	}
	genService := generation.NewServiceWithOptions(generation.ServiceOptions{
		VideoProvider:  videoProv,
		ExecutionHooks: providerExecutionHooks(store, true),
	})
	apiInstance := newAPI(store, cfg, nil, nil)
	apiInstance.generationService = genService

	// Provider execution seeded in Submitted state with matching fingerprint
	fp, err := pe.Fingerprint(taskID, "configured", "mock-video", "video", map[string]any{
		"provider":                "channel_runtime_env",
		"duration":                5,
		"aspect_ratio":            "16:9",
		"resolution":              "720p",
		"generation_async_canary": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	peStore := pe.NewStore(db)
	claimed, err := peStore.CreatePrepared(ctx, pe.Execution{
		TaskID:             taskID,
		Provider:           "configured",
		ProviderModel:      "mock-video",
		Capability:         "video",
		RequestFingerprint: fp,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("claimed ID: %d", claimed.ID)
	if claimed, err = peStore.ClaimPrepared(ctx, taskID); err != nil {
		t.Fatal(err)
	}
	t.Logf("claimed status: %s", claimed.Status)
	provID := "prov-async-retry-123"
	if err := peStore.Transition(ctx, claimed.ID, pe.Submitted, &provID, nil, nil); err != nil {
		t.Fatal(err)
	}
	latestCheck, _ := peStore.GetLatestByTask(ctx, taskID)
	t.Logf("seeded status: %s, requestID: %v, fingerprint: %s", latestCheck.Status, latestCheck.ProviderRequestID, latestCheck.RequestFingerprint)

	// Delivery should invoke runVideoGenerationTask. Since mock provider Get is not configured on the
	// live API instance, it returns ErrUnknownResubmitBlocked or ErrProviderStillProcessing.
	t.Logf("calling processGenerationVideoCanaryMessage...")
	err = apiInstance.processGenerationVideoCanaryMessage(ctx, inbox, envelope)
	t.Logf("processGenerationVideoCanaryMessage returned err: %v", err)
	if err == nil {
		t.Fatal("expected error to trigger consumer retry")
	}

	// Verify inbox is NOT completed (processed_at is NULL), allowing redelivery
	var processedAt sql.NullTime
	err = db.QueryRowContext(ctx, `SELECT processed_at FROM consumer_inbox WHERE event_id=$1`, envelope.EventID).Scan(&processedAt)
	if err != nil {
		t.Fatalf("query inbox: %v", err)
	}
	if processedAt.Valid {
		t.Fatal("inbox should remain unprocessed during retryable in-progress states")
	}
}

func TestVideoCanary_WorkerSuccessCompletesInboxAndTask(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	testUser := "u-vsuccess-" + suffix
	accountID := "acc-" + testUser

	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users (id, name, role) VALUES ($1, 'Video Success User', 'MEMBER') ON CONFLICT (id) DO NOTHING`, testUser); err != nil {
		t.Fatal(err)
	}
	grantID := "grant-" + testUser
	pointStore := NewPostgresPersonalPointStore(db)
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{
		AccountID:      accountID,
		UserID:         testUser,
		Source:         PointSourceRecharge,
		Points:         10000,
		IdempotencyKey: grantID,
	}); err != nil {
		t.Fatal(err)
	}

	store := newPostgresPrimaryStore(db, "")
	req := videoAcceptanceRequest("worker-success-" + suffix)
	req.UserID = testUser
	req.Model = "mock-video"
	req.Params["duration"] = 5
	req.Params["resolution"] = "480p"
	req.Params["provider"] = "channel_runtime_env"

	task, err := store.CreatePendingGenerationTaskWithVideoCanaryOutbox(req)
	if err != nil {
		t.Fatalf("create pending task: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM consumer_inbox WHERE event_id=$1", "evt-"+task.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id=$1", task.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", task.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", task.ID)
	}()

	inbox := messaging.NewInboxStore(db)
	envelope := &messaging.Envelope{
		EventID:       "evt-" + task.ID,
		EventType:     messaging.GenerationVideoCanaryRoutingKey,
		AggregateType: "generation_task",
		AggregateID:   task.ID,
		Data:          map[string]interface{}{"task_id": task.ID},
	}

	cfg := stage0VideoCanaryConfig()
	videoProv := &mockVideoProvider{
		createFn: func(ctx context.Context, req generation.CreateRequest) (any, error) {
			return map[string]any{
				"provider":       "mock-video",
				"providerTaskId": "task-success-999",
				"status":         "SUCCEEDED",
				"videoUrl":       "https://cdn.example.com/success.mp4",
			}, nil
		},
	}
	genService := generation.NewServiceWithOptions(generation.ServiceOptions{
		VideoProvider:  videoProv,
		ExecutionHooks: providerExecutionHooks(store, true),
	})
	apiInstance := newAPI(store, cfg, nil, nil)
	apiInstance.generationService = genService

	err = apiInstance.processGenerationVideoCanaryMessage(ctx, inbox, envelope)
	if err != nil {
		t.Fatalf("expected nil error on success, got: %v", err)
	}

	// Verify inbox was marked completed
	var processedAt sql.NullTime
	var result sql.NullString
	err = db.QueryRowContext(ctx, `SELECT processed_at, result FROM consumer_inbox WHERE event_id=$1`, envelope.EventID).Scan(&processedAt, &result)
	if err != nil {
		t.Fatalf("query inbox: %v", err)
	}
	if !processedAt.Valid || result.String != "completed" {
		t.Fatalf("inbox not completed: processedAt=%v result=%v", processedAt, result)
	}

	// Verify task in DB is SUCCEEDED with captured points
	var taskStatus, billingStatus string
	var capturedPoints float64
	err = db.QueryRowContext(ctx, `SELECT status, billing_status, captured_points FROM xz_generation_tasks WHERE id=$1`, task.ID).Scan(&taskStatus, &billingStatus, &capturedPoints)
	if err != nil {
		t.Fatalf("query task: %v", err)
	}
	if taskStatus != "SUCCEEDED" {
		t.Fatalf("taskStatus=%s, want SUCCEEDED", taskStatus)
	}
	if billingStatus != "CAPTURED" {
		t.Fatalf("billingStatus=%s, want CAPTURED", billingStatus)
	}
	if capturedPoints <= 0 {
		t.Fatalf("capturedPoints=%v, want > 0", capturedPoints)
	}
}
