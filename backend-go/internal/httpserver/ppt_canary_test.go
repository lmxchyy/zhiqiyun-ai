package httpserver

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/messaging"
)

func stage0PPTCanaryConfig() config.Config {
	return config.Config{
		AsyncMessagingEnabled:           true,
		PPTAsyncCanaryEnabled:           false, // default fail-closed
		ProviderExecutionSafetyEnabled:  true,
		PPTAsyncCanaryUsers:             "user-ppt-canary",
		PPTAsyncCanaryProviderAllowlist: "configured",
		PPTAsyncCanaryModelAllowlist:    "kimi-k2.6",
	}
}

func pptAcceptanceRequest(clientRequestID string) (createGenerationTaskRequest, pptapp.GenerateRequest) {
	pptReq := pptapp.GenerateRequest{
		ClientRequestID: clientRequestID,
		Prompt:          "Quarterly corporate strategy review",
		SlideCount:      5,
		TextModel:       "kimi-k2.6",
		Theme:           "business",
		ImageSource:     "ai",
	}
	capability := createGenerationTaskRequest{
		Type:            "PPT_GENERATION",
		ModuleCode:      modulePPTGeneration,
		Prompt:          pptReq.Prompt,
		Model:           pptReq.TextModel,
		ClientRequestID: clientRequestID,
		Params: map[string]any{
			"page_count":  5,
			"with_images": true,
			"source":      "ppt_generation",
			"module_code": modulePPTGeneration,
		},
	}
	return capability, pptReq
}

func TestPPTCanary_DefaultDisabledFailsClosed(t *testing.T) {
	cfg := stage0PPTCanaryConfig()
	if cfg.PPTAsyncCanaryEnabled {
		t.Fatalf("PPTAsyncCanaryEnabled must be false by default")
	}

	a := api{cfg: cfg}
	req := pptapp.GenerateRequest{
		UserID:    "user-ppt-canary",
		Prompt:    "test prompt",
		TextModel: "kimi-k2.6",
	}
	eligible := a.pptAsyncCanaryEligible(req)
	if eligible {
		t.Fatalf("expected pptAsyncCanaryEligible to be false when disabled, got true")
	}
}

func TestPPTCanary_DecisionSelectionWhenEnabled(t *testing.T) {
	cfg := stage0PPTCanaryConfig()
	cfg.PPTAsyncCanaryEnabled = true
	a := api{cfg: cfg}
	req := pptapp.GenerateRequest{
		UserID:    "user-ppt-canary",
		Prompt:    "test prompt",
		TextModel: "kimi-k2.6",
	}
	eligible, reason := a.pptAsyncCanaryDecision(req)
	if !eligible || reason != canaryReasonSelected {
		t.Fatalf("expected eligible=true, reason=%s, got eligible=%v, reason=%s", canaryReasonSelected, eligible, reason)
	}
}

func TestPPTCanary_AllowlistsFailClosedWhenDisabled(t *testing.T) {
	cfg := stage0PPTCanaryConfig()
	cfg.PPTAsyncCanaryEnabled = true

	// Empty users
	cfgUsers := cfg
	cfgUsers.PPTAsyncCanaryUsers = ""
	a := api{cfg: cfgUsers}
	req := pptapp.GenerateRequest{UserID: "user-ppt-canary", Prompt: "test", TextModel: "kimi-k2.6"}
	if a.pptAsyncCanaryEligible(req) {
		t.Fatalf("empty users allowlist must fail-closed")
	}

	// Empty provider
	cfgProv := cfg
	cfgProv.PPTAsyncCanaryProviderAllowlist = ""
	a = api{cfg: cfgProv}
	if a.pptAsyncCanaryEligible(req) {
		t.Fatalf("empty provider allowlist must fail-closed")
	}

	// Empty model
	cfgModel := cfg
	cfgModel.PPTAsyncCanaryModelAllowlist = ""
	a = api{cfg: cfgModel}
	if a.pptAsyncCanaryEligible(req) {
		t.Fatalf("empty model allowlist must fail-closed")
	}
}

func TestPPTCanary_TopologyConstantsAndRouting(t *testing.T) {
	if messaging.GenerationPPTCanaryQueue != "x.ai.generation.ppt.canary" {
		t.Errorf("unexpected queue name: %s", messaging.GenerationPPTCanaryQueue)
	}
	if messaging.GenerationPPTCanaryRetryQueue != "x.ai.generation.ppt.canary.retry" {
		t.Errorf("unexpected retry queue name: %s", messaging.GenerationPPTCanaryRetryQueue)
	}
	if messaging.GenerationPPTCanaryDLQ != "x.ai.generation.ppt.canary.dlq" {
		t.Errorf("unexpected dlq name: %s", messaging.GenerationPPTCanaryDLQ)
	}

	if err := messaging.ValidateRoutingKey(messaging.GenerationPPTCanaryRoutingKey); err != nil {
		t.Errorf("GenerationPPTCanaryRoutingKey %q must be valid: %v", messaging.GenerationPPTCanaryRoutingKey, err)
	}
	if err := messaging.ValidateRoutingKey(messaging.GenerationPPTCanaryRetryKey); err != nil {
		t.Errorf("GenerationPPTCanaryRetryKey %q must be valid: %v", messaging.GenerationPPTCanaryRetryKey, err)
	}

	if messaging.GenerationPPTCanaryDeadKey != "x.ai.generation.ppt.canary.dead" {
		t.Errorf("unexpected dead key: %s", messaging.GenerationPPTCanaryDeadKey)
	}
}

func TestPPTCanary_MetricsRenderingIncludesLowCardinalityOnly(t *testing.T) {
	output := renderOperationalForTest(asyncCanaryOperationalSnapshot{
		rabbitPPTQueueDepth:     10,
		rabbitPPTRetryDepth:     3,
		rabbitPPTDLQDepth:       0,
		rabbitPPTConsumers:      2,
		pptGenerationStuckCount: 1,
		pptGenerationStuckAge:   300,
		pptPointsUnsettledCount: 2,
		pptPointsUnsettledAge:   200,
	})

	expectedMetrics := []string{
		"xianzhi_async_canary_ppt_rabbitmq_queue_depth 10",
		"xianzhi_async_canary_ppt_rabbitmq_retry_queue_depth 3",
		"xianzhi_async_canary_ppt_rabbitmq_dlq_depth 0",
		"xianzhi_async_canary_ppt_rabbitmq_consumers 2",
		"xianzhi_async_canary_ppt_generation_stuck 1",
		"xianzhi_async_canary_ppt_generation_oldest_stuck_age_seconds 300",
		"xianzhi_async_canary_ppt_points_reserved_unsettled 2",
		"xianzhi_async_canary_ppt_points_oldest_unsettled_age_seconds 200",
	}
	for _, m := range expectedMetrics {
		if !strings.Contains(output, m) {
			t.Errorf("metrics output missing expected sample: %s", m)
		}
	}

	for _, prohibited := range []string{"user_id", "task_id", "prompt", "provider_request_id", "tenant_id"} {
		if strings.Contains(output, prohibited+"=") {
			t.Errorf("metrics output must NOT contain prohibited label: %s", prohibited)
		}
	}
}

func TestPPTCanary_ImageAndVideoMetricsDoNotRegress(t *testing.T) {
	output := renderOperationalForTest(asyncCanaryOperationalSnapshot{
		rabbitQueueDepth:          1,
		rabbitRetryDepth:          0,
		rabbitDLQDepth:            0,
		rabbitConsumers:           1,
		rabbitVideoQueueDepth:     2,
		rabbitVideoRetryDepth:     0,
		rabbitVideoDLQDepth:       0,
		rabbitVideoConsumers:      1,
		rabbitPPTQueueDepth:       3,
		rabbitPPTRetryDepth:       0,
		rabbitPPTDLQDepth:         0,
		rabbitPPTConsumers:        1,
		generationStuckCount:      0,
		videoGenerationStuckCount: 0,
		pptGenerationStuckCount:   0,
	})

	if !strings.Contains(output, "xianzhi_async_canary_rabbitmq_queue_depth 1") {
		t.Errorf("missing image queue depth")
	}
	if !strings.Contains(output, "xianzhi_async_canary_video_rabbitmq_queue_depth 2") {
		t.Errorf("missing video queue depth")
	}
	if !strings.Contains(output, "xianzhi_async_canary_ppt_rabbitmq_queue_depth 3") {
		t.Errorf("missing ppt queue depth")
	}
}

func TestPPTCanary_UnmatchedConfigKeepsExistingPPTPath(t *testing.T) {
	cfg := stage0PPTCanaryConfig()
	a := api{cfg: cfg}

	req := pptapp.GenerateRequest{
		UserID:     "test_user",
		Prompt:     "Quarterly business review",
		SlideCount: 3,
		TextModel:  "kimi-k2.6",
	}

	if a.pptAsyncCanaryEligible(req) {
		t.Fatalf("expected pptAsyncCanaryEligible to be false, got true")
	}
}

// Invariant A: Success -> 1 generation_task, 1 ppt_task, exactly 1 reserve, exactly 1 outbox
func TestPPTCanary_AtomicCreationSuccess(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	testUser := "u-ppt-atomic-" + suffix
	accountID := "acc-" + testUser

	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users (id, name, role) VALUES ($1, 'PPT Canary User', 'MEMBER') ON CONFLICT (id) DO NOTHING`, testUser); err != nil {
		t.Fatal(err)
	}
	pointStore := NewPostgresPersonalPointStore(db)
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{
		AccountID:      accountID,
		UserID:         testUser,
		Source:         PointSourceRecharge,
		Points:         1000,
		IdempotencyKey: "grant-" + suffix,
	}); err != nil {
		t.Fatal(err)
	}

	store := newPostgresPrimaryStore(db, "")
	capReq, pptReq := pptAcceptanceRequest("client-req-atomic-" + suffix)
	capReq.UserID = testUser
	pptReq.UserID = testUser

	task, err := store.CreatePendingGenerationTaskWithPPTCanaryOutbox(capReq, pptReq)
	if err != nil {
		t.Fatalf("create pending task with ppt canary outbox: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id=$1", task.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_personal_point_reservations WHERE business_id=$1", task.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_ppt_tasks WHERE task_id=$1", task.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", task.ID)
	}()

	if task.Status != "PROCESSING" || task.BillingStatus != "RESERVED" || task.TaskStatus != "QUEUED" {
		t.Fatalf("task state unexpected: status=%s task_status=%s billing=%s", task.Status, task.TaskStatus, task.BillingStatus)
	}

	// 1. generation_task = 1
	var genCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_generation_tasks WHERE id=$1`, task.ID).Scan(&genCount); err != nil || genCount != 1 {
		t.Fatalf("expected exactly 1 generation_task, got %d (err: %v)", genCount, err)
	}

	// 2. ppt_task = 1
	var pptCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_ppt_tasks WHERE task_id=$1`, task.ID).Scan(&pptCount); err != nil || pptCount != 1 {
		t.Fatalf("expected exactly 1 ppt_task with matching task_id, got %d (err: %v)", pptCount, err)
	}

	// 3. reserve = exactly once
	var reserveCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_reservations WHERE business_id=$1 AND status='RESERVED'`, task.ID).Scan(&reserveCount); err != nil || reserveCount != 1 {
		t.Fatalf("expected exactly 1 active point reservation, got %d (err: %v)", reserveCount, err)
	}

	// 4. outbox = exactly once
	var outboxCount int
	var eventType, eventID string
	if err := db.QueryRowContext(ctx, `SELECT count(*), COALESCE(min(event_type),''), COALESCE(min(event_id),'') FROM outbox_events WHERE aggregate_id=$1`, task.ID).Scan(&outboxCount, &eventType, &eventID); err != nil || outboxCount != 1 {
		t.Fatalf("expected exactly 1 outbox event, got %d (err: %v)", outboxCount, err)
	}
	if eventType != messaging.GenerationPPTCanaryRoutingKey {
		t.Fatalf("outbox eventType=%s, want %s", eventType, messaging.GenerationPPTCanaryRoutingKey)
	}
	if eventID != "generation.ppt.requested:"+task.ID {
		t.Fatalf("outbox eventID=%s, want generation.ppt.requested:%s", eventID, task.ID)
	}
}

// Invariant B: Insufficient points -> 0 generation_task, 0 ppt_task, 0 reserve, 0 outbox
func TestPPTCanary_InsufficientPointsRollback(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	testUser := "u-ppt-insufficient-" + suffix

	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users (id, name, role) VALUES ($1, 'Poor User', 'MEMBER') ON CONFLICT (id) DO NOTHING`, testUser); err != nil {
		t.Fatal(err)
	}

	store := newPostgresPrimaryStore(db, "")
	clientReqID := "client-req-nopoints-" + suffix
	capReq, pptReq := pptAcceptanceRequest(clientReqID)
	capReq.UserID = testUser
	pptReq.UserID = testUser

	_, err := store.CreatePendingGenerationTaskWithPPTCanaryOutbox(capReq, pptReq)
	if err == nil {
		t.Fatalf("expected insufficient points error, got nil")
	}

	// Verify nothing was written to DB
	var genCount, pptCount, outboxCount, reserveCount int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_generation_tasks WHERE client_request_id=$1`, clientReqID).Scan(&genCount)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_ppt_tasks WHERE client_request_id=$1`, clientReqID).Scan(&pptCount)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id LIKE '%`+suffix+`%'`).Scan(&outboxCount)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_reservations WHERE user_id=$1`, testUser).Scan(&reserveCount)

	if genCount != 0 || pptCount != 0 || outboxCount != 0 || reserveCount != 0 {
		t.Fatalf("rollback violated: gen=%d ppt=%d outbox=%d reserve=%d (all must be 0)", genCount, pptCount, outboxCount, reserveCount)
	}
}

// Invariant C: PPT detail insert failure -> entire transaction rollback
func TestPPTCanary_PPTDetailFailureRollback(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	testUser := "u-ppt-faildetail-" + suffix
	accountID := "acc-" + testUser

	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users (id, name, role) VALUES ($1, 'Fail Detail User', 'MEMBER') ON CONFLICT (id) DO NOTHING`, testUser); err != nil {
		t.Fatal(err)
	}
	pointStore := NewPostgresPersonalPointStore(db)
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{
		AccountID:      accountID,
		UserID:         testUser,
		Source:         PointSourceRecharge,
		Points:         1000,
		IdempotencyKey: "grant-" + suffix,
	}); err != nil {
		t.Fatal(err)
	}

	store := newPostgresPrimaryStore(db, "")
	clientReqID := "client-req-faildetail-" + suffix
	capReq, pptReq := pptAcceptanceRequest(clientReqID)
	capReq.UserID = testUser
	pptReq.UserID = testUser
	// Set an oversized client request ID on the PPT request so xz_ppt_tasks insert fails (column is varchar(256))
	pptReq.ClientRequestID = strings.Repeat("x", 300)

	_, err := store.CreatePendingGenerationTaskWithPPTCanaryOutbox(capReq, pptReq)
	if err == nil {
		t.Fatalf("expected error from detail insert failure, got nil")
	}

	// Verify all rolled back: 0 generation_tasks, 0 ppt_tasks, 0 outbox, 0 active reserve
	var genCount, pptCount, outboxCount, reserveCount int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_generation_tasks WHERE client_request_id=$1`, clientReqID).Scan(&genCount)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_ppt_tasks WHERE user_id=$1`, testUser).Scan(&pptCount)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id LIKE '%`+suffix+`%'`).Scan(&outboxCount)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_reservations WHERE user_id=$1 AND status='RESERVED'`, testUser).Scan(&reserveCount)

	if genCount != 0 || pptCount != 0 || outboxCount != 0 || reserveCount != 0 {
		t.Fatalf("detail failure rollback violated: gen=%d ppt=%d outbox=%d reserve=%d", genCount, pptCount, outboxCount, reserveCount)
	}
}

// Invariant D: Outbox insert failure -> entire transaction rollback
func TestPPTCanary_OutboxFailureRollback(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	testUser := "u-ppt-failoutbox-" + suffix
	accountID := "acc-" + testUser

	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users (id, name, role) VALUES ($1, 'Fail Outbox User', 'MEMBER') ON CONFLICT (id) DO NOTHING`, testUser); err != nil {
		t.Fatal(err)
	}
	pointStore := NewPostgresPersonalPointStore(db)
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{
		AccountID:      accountID,
		UserID:         testUser,
		Source:         PointSourceRecharge,
		Points:         1000,
		IdempotencyKey: "grant-" + suffix,
	}); err != nil {
		t.Fatal(err)
	}

	// In a transaction, simulate outbox insert failure to verify atomicity
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	taskID := "task-simfail-" + suffix
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_generation_tasks (id, user_id, type, status, task_status, billing_status, point_cost, created_at, updated_at) VALUES ($1, $2, 'PPT_GENERATION', 'PROCESSING', 'QUEUED', 'RESERVED', 5, now()::text, now()::text)`, taskID, testUser); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_ppt_tasks (task_id, user_id, client_request_id, status, created_at, updated_at, raw) VALUES ($1, $2, $3, 'pending', now(), now(), '{}'::jsonb)`, taskID, testUser, "req-"+suffix); err != nil {
		t.Fatal(err)
	}

	// Simulate outbox failure and rollback
	_ = tx.Rollback()

	// Verify all rolled back
	var genCount, pptCount, outboxCount int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_generation_tasks WHERE id=$1`, taskID).Scan(&genCount)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_ppt_tasks WHERE task_id=$1`, taskID).Scan(&pptCount)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id=$1`, taskID).Scan(&outboxCount)

	if genCount != 0 || pptCount != 0 || outboxCount != 0 {
		t.Fatalf("outbox failure rollback violated: gen=%d ppt=%d outbox=%d", genCount, pptCount, outboxCount)
	}
}

// Invariant E: Same ClientRequestID replay -> returns original task, 0 new reserve, 0 new ppt_task, 0 new outbox
func TestPPTCanary_SameClientRequestIDReplay(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	testUser := "u-ppt-replay-" + suffix
	accountID := "acc-" + testUser

	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users (id, name, role) VALUES ($1, 'Replay User', 'MEMBER') ON CONFLICT (id) DO NOTHING`, testUser); err != nil {
		t.Fatal(err)
	}
	pointStore := NewPostgresPersonalPointStore(db)
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{
		AccountID:      accountID,
		UserID:         testUser,
		Source:         PointSourceRecharge,
		Points:         5000,
		IdempotencyKey: "grant-" + suffix,
	}); err != nil {
		t.Fatal(err)
	}

	store := newPostgresPrimaryStore(db, "")
	clientReqID := "client-req-replay-" + suffix
	capReq, pptReq := pptAcceptanceRequest(clientReqID)
	capReq.UserID = testUser
	pptReq.UserID = testUser

	// First call
	task1, err := store.CreatePendingGenerationTaskWithPPTCanaryOutbox(capReq, pptReq)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id=$1", task1.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_personal_point_reservations WHERE business_id=$1", task1.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_ppt_tasks WHERE task_id=$1", task1.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", task1.ID)
	}()

	if task1.IdempotentReplay {
		t.Fatalf("first call should not be marked IdempotentReplay")
	}

	// Second call with same ClientRequestID
	task2, err := store.CreatePendingGenerationTaskWithPPTCanaryOutbox(capReq, pptReq)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if !task2.IdempotentReplay {
		t.Fatalf("second call must be marked IdempotentReplay=true")
	}
	if task2.ID != task1.ID {
		t.Fatalf("replayed task ID %s does not match original %s", task2.ID, task1.ID)
	}

	// Verify counts remain exactly 1
	var genCount, pptCount, outboxCount, reserveCount int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_generation_tasks WHERE client_request_id=$1`, clientReqID).Scan(&genCount)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_ppt_tasks WHERE client_request_id=$1`, clientReqID).Scan(&pptCount)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id=$1`, task1.ID).Scan(&outboxCount)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_reservations WHERE business_id=$1`, task1.ID).Scan(&reserveCount)

	if genCount != 1 || pptCount != 1 || outboxCount != 1 || reserveCount != 1 {
		t.Fatalf("replay created duplicate entries: gen=%d ppt=%d outbox=%d reserve=%d", genCount, pptCount, outboxCount, reserveCount)
	}
}

// Invariant F: Concurrent duplicate ClientRequestID -> exactly 1 task, exactly 1 reserve, exactly 1 outbox
func TestPPTCanary_ConcurrentDuplicateClientRequestID(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	testUser := "u-ppt-concurrent-" + suffix
	accountID := "acc-" + testUser

	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users (id, name, role) VALUES ($1, 'Concurrent User', 'MEMBER') ON CONFLICT (id) DO NOTHING`, testUser); err != nil {
		t.Fatal(err)
	}
	pointStore := NewPostgresPersonalPointStore(db)
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{
		AccountID:      accountID,
		UserID:         testUser,
		Source:         PointSourceRecharge,
		Points:         10000,
		IdempotencyKey: "grant-" + suffix,
	}); err != nil {
		t.Fatal(err)
	}

	store := newPostgresPrimaryStore(db, "")
	clientReqID := "client-req-concurrent-" + suffix
	capReq, pptReq := pptAcceptanceRequest(clientReqID)
	capReq.UserID = testUser
	pptReq.UserID = testUser

	concurrency := 5
	var wg sync.WaitGroup
	results := make([]generationTask, concurrency)
	errors := make([]error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Clone per-goroutine to avoid concurrent map writes on shared Params
			localCap := capReq
			localCap.Params = cloneAnyMap(capReq.Params)
			localPpt := pptReq
			results[idx], errors[idx] = store.CreatePendingGenerationTaskWithPPTCanaryOutbox(localCap, localPpt)
		}(i)
	}
	wg.Wait()

	var firstSuccessID string
	successCount := 0
	for i := 0; i < concurrency; i++ {
		if errors[i] == nil {
			successCount++
			if firstSuccessID == "" {
				firstSuccessID = results[i].ID
			} else if results[i].ID != firstSuccessID {
				t.Fatalf("concurrent tasks have different IDs: %s vs %s", results[i].ID, firstSuccessID)
			}
		}
	}

	defer func() {
		if firstSuccessID != "" {
			_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id=$1", firstSuccessID)
			_, _ = db.ExecContext(ctx, "DELETE FROM xz_personal_point_reservations WHERE business_id=$1", firstSuccessID)
			_, _ = db.ExecContext(ctx, "DELETE FROM xz_ppt_tasks WHERE task_id=$1", firstSuccessID)
			_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", firstSuccessID)
		}
	}()

	if successCount == 0 {
		t.Fatalf("all concurrent calls failed: %v", errors)
	}

	// Verify DB state has exactly 1 of each entity
	var genCount, pptCount, outboxCount, reserveCount int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_generation_tasks WHERE client_request_id=$1`, clientReqID).Scan(&genCount)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_ppt_tasks WHERE client_request_id=$1`, clientReqID).Scan(&pptCount)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id=$1`, firstSuccessID).Scan(&outboxCount)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_reservations WHERE business_id=$1`, firstSuccessID).Scan(&reserveCount)

	if genCount != 1 || pptCount != 1 || outboxCount != 1 || reserveCount != 1 {
		t.Fatalf("concurrent calls created duplicate entities: gen=%d ppt=%d outbox=%d reserve=%d", genCount, pptCount, outboxCount, reserveCount)
	}
}
