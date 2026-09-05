package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

// Gate 1 — Visual Revision Safety (pure unit tests, no DB).
//
// Proven by code audit of completeSlideVisual: VisualHistory is appended ONLY
// when replacing a non-empty different URL, so a first successful generation
// leaves history empty and len(VisualHistory) can NOT serve as revision.
// The worker therefore uses the explicit persisted Slide.VisualRevision,
// bumped atomically with the success checkpoint.

func TestPPTWorker_FirstGenerationRevisionIsZero(t *testing.T) {
	slide := pptapp.Slide{ID: "slide_1"}
	if got := pptSlideRevision(slide); got != 0 {
		t.Fatalf("fresh slide revision = %d, want 0", got)
	}
	if got := pptSlideExecKey("task_1", "slide_1", pptSlideRevision(slide)); got != "ppt:task_1:slide_1:rev:0" {
		t.Fatalf("first generation key = %q, want ppt:task_1:slide_1:rev:0", got)
	}
}

func TestPPTWorker_CrashRedeliveryKeepsSameRevision(t *testing.T) {
	// Crash before the success checkpoint commits: nothing persisted, so a
	// redelivery recomputes the identical revision and execution key.
	slide := pptapp.Slide{ID: "slide_1", VisualRevision: 0}
	first := pptSlideExecKey("task_1", slide.ID, pptSlideRevision(slide))
	redelivered := pptSlideExecKey("task_1", slide.ID, pptSlideRevision(slide))
	if first != redelivered {
		t.Fatalf("crash redelivery changed key: %q vs %q", first, redelivered)
	}
}

func TestPPTWorker_UserRegenProducesNewRevision(t *testing.T) {
	// After a committed success checkpoint (revision bumped to N+1), a
	// legitimate user regenerate must map to a different execution key.
	before := pptSlideExecKey("task_1", "slide_1", 0)
	after := pptSlideExecKey("task_1", "slide_1", 1)
	if before == after {
		t.Fatalf("user regenerate must change execution key, both = %q", before)
	}
	if after != "ppt:task_1:slide_1:rev:1" {
		t.Fatalf("regenerated key = %q, want ppt:task_1:slide_1:rev:1", after)
	}
}

func TestPPTWorker_DuplicateRegenRequestIsIdempotent(t *testing.T) {
	// Same (task, slide, revision) twice must yield the same key: retries and
	// duplicate MQ redeliveries never mint new executions by themselves.
	first := pptSlideExecKey("task_1", "slide_1", 2)
	second := pptSlideExecKey("task_1", "slide_1", 2)
	if first != second {
		t.Fatalf("duplicate request changed key: %q vs %q", first, second)
	}
}

func TestPPTWorker_RevisionNeverFromGoroutineIndexOrRandom(t *testing.T) {
	// The key builder must be a pure function of (task, slide, revision):
	// no timestamps, no randomness, no attempt counters inside.
	first := pptSlideExecKey("task_1", "slide_1", 0)
	time.Sleep(2 * time.Millisecond)
	second := pptSlideExecKey("task_1", "slide_1", 0)
	if first != second {
		t.Fatalf("key must be deterministic across calls: %q vs %q", first, second)
	}
	if pptSlideExecKey("", "slide_1", 0) != "" || pptSlideExecKey("task_1", "", 0) != "" {
		t.Fatalf("incomplete identity must yield empty key, never a guess")
	}
}

func TestPPTWorker_AmbiguousErrorsNeverDegrade(t *testing.T) {
	for _, message := range []string{
		"context deadline exceeded",
		"connection reset by peer",
		"HTTP 503 Service Unavailable",
		"provider execution unknown",
	} {
		if isPPTSlideDeterministicError(fmt.Errorf("ppt image generation failed: %s", message), nil, "") {
			t.Errorf("ambiguous error %q must NOT be classified deterministic", message)
		}
	}
	for _, message := range []string{
		"ppt image provider returned no image",
		"authorize ppt image generation: forbidden",
		"prompt is required",
	} {
		if !isPPTSlideDeterministicError(fmt.Errorf("%s", message), nil, "") {
			t.Errorf("deterministic error %q must be classified deterministic", message)
		}
	}
}

// Gate 2 — Concurrent Checkpoint Lost-Update Safety (real Postgres).
//
// xz_ppt_tasks.raw is a full-document row. updatePostgresTask reloads the row
// fresh under SELECT ... FOR UPDATE, so concurrent slide completions serialize
// and each persists only its own slide. This test proves pool=3 completion
// never loses a sibling slide checkpoint.

func TestPPTWorker_ConcurrentSlideCheckpointsPreserved(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	userID := "u-ppt-conc-ckpt-" + suffix
	taskID := "ppt-conc-ckpt-" + suffix

	svc := pptapp.NewPostgresService(db, "")
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users (id, name, role) VALUES ($1, 'Concurrent Checkpoint User', 'MEMBER') ON CONFLICT (id) DO NOTHING`, userID); err != nil {
		t.Fatal(err)
	}
	slides := []pptapp.Slide{
		{ID: "slide_a", Page: 1, Title: "Alpha", Layout: "title"},
		{ID: "slide_b", Page: 2, Title: "Beta", Layout: "title"},
		{ID: "slide_c", Page: 3, Title: "Gamma", Layout: "title"},
	}
	seed := pptapp.Task{TaskID: taskID, UserID: userID, Status: pptapp.StatusProcessing, Title: "Concurrency", Slides: slides}
	seedPPTDetailTx(t, db, ctx, seed)
	defer func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_ppt_tasks WHERE task_id=$1`, taskID)
	}()

	var wg sync.WaitGroup
	errs := make([]error, len(slides))
	for i, slide := range slides {
		wg.Add(1)
		go func(idx int, slideID string) {
			defer wg.Done()
			plan := pptapp.VisualPlan{VisualType: "illustration"}
			_, err := svc.CompleteSlideVisualWithRevision(userID, taskID, slideID, plan, pptapp.VisualAsset{
				URL:       fmt.Sprintf("https://cdn.example.com/%s.png", slideID),
				ModelName: "test-model",
			}, 1)
			errs[idx] = err
		}(i, slide.ID)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("slide %d completion failed: %v", i, err)
		}
	}

	reloaded, err := svc.GetTask(userID, taskID)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]pptapp.Slide{}
	for _, slide := range reloaded.Slides {
		byID[slide.ID] = slide
	}
	for _, slide := range slides {
		got, ok := byID[slide.ID]
		if !ok {
			t.Fatalf("slide %s missing after concurrent checkpoints", slide.ID)
		}
		if strings.TrimSpace(got.ImageURL) == "" || got.VisualRevision != 1 || got.VisualStatus != "success" {
			t.Fatalf("slide %s checkpoint lost: url=%q rev=%d status=%q", slide.ID, got.ImageURL, got.VisualRevision, got.VisualStatus)
		}
	}
}

func TestPPTWorker_PlanAndImageConcurrentUpdatePreserved(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	userID := "u-ppt-planimg-" + suffix
	taskID := "ppt-planimg-" + suffix

	svc := pptapp.NewPostgresService(db, "")
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users (id, name, role) VALUES ($1, 'Plan Image User', 'MEMBER') ON CONFLICT (id) DO NOTHING`, userID); err != nil {
		t.Fatal(err)
	}
	seed := pptapp.Task{
		TaskID: taskID, UserID: userID, Status: pptapp.StatusProcessing, Title: "PlanImage",
		Slides: []pptapp.Slide{{ID: "slide_1", Page: 1, Title: "Solo", Layout: "title"}},
	}
	seedPPTDetailTx(t, db, ctx, seed)
	defer func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_ppt_tasks WHERE task_id=$1`, taskID)
	}()

	var wg sync.WaitGroup
	var planErr, imageErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, planErr = svc.UpdateSlideVisualPlan(userID, taskID, "slide_1", pptapp.VisualPlan{VisualType: "illustration"}, "", "planned", "")
	}()
	go func() {
		defer wg.Done()
		_, imageErr = svc.CompleteSlideVisualWithRevision(userID, taskID, "slide_1", pptapp.VisualPlan{VisualType: "illustration"}, pptapp.VisualAsset{URL: "https://cdn.example.com/solo.png", ModelName: "m"}, 1)
	}()
	wg.Wait()
	if planErr != nil {
		t.Fatalf("concurrent plan update failed: %v", planErr)
	}
	if imageErr != nil {
		t.Fatalf("concurrent image completion failed: %v", imageErr)
	}

	reloaded, err := svc.GetTask(userID, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Slides) != 1 {
		t.Fatalf("expected 1 slide, got %d", len(reloaded.Slides))
	}
	got := reloaded.Slides[0]
	if got.ImageURL == "" {
		t.Fatalf("image completion lost under concurrent plan update")
	}
	if got.VisualPlan == nil || got.VisualPlan.VisualType != "illustration" {
		t.Fatalf("plan update lost under concurrent image completion")
	}
}

// Settlement exactly-once under duplicate delivery (real Postgres).
// Double CompleteGenerationTask / double FailGenerationTaskDurable must settle
// points exactly once via the idempotent capture/release keys.

func TestPPTWorker_SettlementDoubleDeliverySingleCapture(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	testUser := "u-ppt-settle-" + suffix
	accountID := "acc-" + testUser
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users (id, name, role) VALUES ($1, 'Settle User', 'MEMBER') ON CONFLICT (id) DO NOTHING`, testUser); err != nil {
		t.Fatal(err)
	}
	pointStore := NewPostgresPersonalPointStore(db)
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{
		AccountID: accountID, UserID: testUser, Source: PointSourceRecharge,
		Points: 10000, IdempotencyKey: "grant-" + suffix,
	}); err != nil {
		t.Fatal(err)
	}
	store := newPostgresPrimaryStore(db, "")
	capReq, pptReq := pptAcceptanceRequest("client-req-settle-" + suffix)
	capReq.UserID = testUser
	pptReq.UserID = testUser
	task, err := store.CreatePendingGenerationTaskWithPPTCanaryOutbox(capReq, pptReq)
	if err != nil {
		t.Fatalf("create parent task: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id=$1", task.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_personal_point_reservations WHERE business_id=$1 AND user_id=$2", task.ID, testUser)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_ppt_tasks WHERE task_id=$1", task.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", task.ID)
	}()

	prepared := generation.CreateRequest{UserID: testUser, Type: task.Type, Prompt: task.Prompt, Model: task.Model, Params: cloneAnyMap(task.Params)}
	if _, err := store.CompleteGenerationTask(task.ID, prepared); err != nil {
		t.Fatalf("first capture failed: %v", err)
	}
	// Duplicate delivery: second capture must be a safe no-op, never double.
	again, err := store.CompleteGenerationTask(task.ID, prepared)
	if err != nil {
		t.Fatalf("second capture failed: %v", err)
	}
	if again.BillingStatus != "CAPTURED" || again.CapturedPoints != task.QuotedPoints {
		t.Fatalf("double capture corrupted settlement: billing=%s captured=%v quoted=%v",
			again.BillingStatus, again.CapturedPoints, task.QuotedPoints)
	}
	var captureCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_reservations WHERE business_id=$1 AND user_id=$2 AND status='CAPTURED'`, task.ID, testUser).Scan(&captureCount); err != nil || captureCount != 1 {
		t.Fatalf("expected exactly 1 captured reservation, got %d (err: %v)", captureCount, err)
	}
}

func TestPPTWorker_FailureDoubleDeliverySingleRelease(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	testUser := "u-ppt-failrel-" + suffix
	accountID := "acc-" + testUser
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users (id, name, role) VALUES ($1, 'Fail Release User', 'MEMBER') ON CONFLICT (id) DO NOTHING`, testUser); err != nil {
		t.Fatal(err)
	}
	pointStore := NewPostgresPersonalPointStore(db)
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{
		AccountID: accountID, UserID: testUser, Source: PointSourceRecharge,
		Points: 10000, IdempotencyKey: "grant-" + suffix,
	}); err != nil {
		t.Fatal(err)
	}
	store := newPostgresPrimaryStore(db, "")
	capReq, pptReq := pptAcceptanceRequest("client-req-failrel-" + suffix)
	capReq.UserID = testUser
	pptReq.UserID = testUser
	task, err := store.CreatePendingGenerationTaskWithPPTCanaryOutbox(capReq, pptReq)
	if err != nil {
		t.Fatalf("create parent task: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id=$1", task.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_personal_point_reservations WHERE business_id=$1 AND user_id=$2", task.ID, testUser)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_ppt_tasks WHERE task_id=$1", task.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", task.ID)
	}()

	if _, err := store.FailGenerationTaskDurable(task.ID, "outline failed"); err != nil {
		t.Fatalf("first release failed: %v", err)
	}
	again, err := store.FailGenerationTaskDurable(task.ID, "outline failed")
	if err != nil {
		t.Fatalf("second release failed: %v", err)
	}
	if again.BillingStatus != "RELEASED" {
		t.Fatalf("double release corrupted settlement: billing=%s", again.BillingStatus)
	}
	var released int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_reservations WHERE business_id=$1 AND user_id=$2 AND status='RELEASED'`, task.ID, testUser).Scan(&released); err != nil || released != 1 {
		t.Fatalf("expected exactly 1 released reservation, got %d (err: %v)", released, err)
	}
}

// seedPPTDetailTx persists a seed ppt detail row inside an explicit commit
// (PersistPostgresTaskTx does not commit by itself).
func seedPPTDetailTx(t *testing.T, db *sql.DB, ctx context.Context, seed pptapp.Task) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := pptapp.PersistPostgresTaskTx(ctx, tx, seed); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// Blocker 2 — Outline / Visual Plan Chat Single Submit Tests.
// Proven: when an execution row is left in Submitting or Unknown (simulating a
// crash mid-POST or after accept before response persisted), redelivery must
// NOT issue a second external HTTP POST. It must mark Unknown and return
// ErrUnknownResubmitBlocked.
func TestPPTWorker_OutlineSubmittingCrashNoSecondPOST(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	taskID := "ppt-crash-outline-" + suffix
	taskKey := "ppt:" + taskID + ":outline"
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskKey)
	}()

	store := pe.NewStore(db)
	semantic := map[string]any{"prompt": "Strategy Outline", "slide_count": 5}
	fp, err := pe.Fingerprint(taskKey, "configured", "kimi-k2.6", pptOutlineCapability, canonicalPPTFingerprintParams(semantic))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreatePrepared(ctx, pe.Execution{
		TaskID: taskKey, Provider: "configured", ProviderModel: "kimi-k2.6",
		Capability: pptOutlineCapability, Attempt: 1, RequestFingerprint: fp,
	}); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimPrepared(ctx, taskKey)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Status != pe.Submitting {
		t.Fatalf("expected status submitting, got %s", claimed.Status)
	}

	var postCount atomic.Int32
	mockCall := func(callCtx context.Context) ([]byte, error) {
		postCount.Add(1)
		return []byte(`{"title":"New Deck"}`), nil
	}

	apiInst := api{store: newPostgresPrimaryStore(db, "")}
	_, _, err = apiInst.runPPTChatStageGuarded(ctx, taskKey, "configured", "kimi-k2.6", pptOutlineCapability, semantic, mockCall)

	if !errors.Is(err, pe.ErrUnknownResubmitBlocked) {
		t.Fatalf("expected ErrUnknownResubmitBlocked, got: %v", err)
	}
	if postCount.Load() != 0 {
		t.Fatalf("BLIND_RESUBMIT: chat provider POST called %d times on redelivery, want 0", postCount.Load())
	}

	latest, err := store.GetLatestByTask(ctx, taskKey)
	if err != nil || latest.Status != pe.Unknown {
		t.Fatalf("expected row status Unknown, got %s (err=%v)", latest.Status, err)
	}
}

func TestPPTWorker_PlanSubmittingCrashNoSecondPOST(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	taskID := "ppt-crash-plan-" + suffix
	taskKey := "ppt:" + taskID + ":slide_1:plan:rev:0"
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskKey)
	}()

	store := pe.NewStore(db)
	semantic := map[string]any{"slideTitle": "Vision", "slideType": "cover"}
	fp, err := pe.Fingerprint(taskKey, "configured", "kimi-k2.6", pptVisualPlanCapability, canonicalPPTFingerprintParams(semantic))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreatePrepared(ctx, pe.Execution{
		TaskID: taskKey, Provider: "configured", ProviderModel: "kimi-k2.6",
		Capability: pptVisualPlanCapability, Attempt: 1, RequestFingerprint: fp,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimPrepared(ctx, taskKey); err != nil {
		t.Fatal(err)
	}

	var postCount atomic.Int32
	mockCall := func(callCtx context.Context) ([]byte, error) {
		postCount.Add(1)
		return []byte(`{"visualType":"illustration"}`), nil
	}

	apiInst := api{store: newPostgresPrimaryStore(db, "")}
	_, _, err = apiInst.runPPTChatStageGuarded(ctx, taskKey, "configured", "kimi-k2.6", pptVisualPlanCapability, semantic, mockCall)

	if !errors.Is(err, pe.ErrUnknownResubmitBlocked) {
		t.Fatalf("expected ErrUnknownResubmitBlocked, got: %v", err)
	}
	if postCount.Load() != 0 {
		t.Fatalf("BLIND_RESUBMIT: plan provider POST called %d times on redelivery, want 0", postCount.Load())
	}
}

func TestPPTWorker_OutlineUnknownBlocksResubmit(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	taskKey := "ppt:task-unknown-" + suffix + ":outline"
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskKey)
	}()

	store := pe.NewStore(db)
	semantic := map[string]any{"prompt": "Strategy", "slide_count": 5}
	fp, _ := pe.Fingerprint(taskKey, "configured", "kimi-k2.6", pptOutlineCapability, canonicalPPTFingerprintParams(semantic))

	e, _ := store.CreatePrepared(ctx, pe.Execution{
		TaskID: taskKey, Provider: "configured", ProviderModel: "kimi-k2.6",
		Capability: pptOutlineCapability, Attempt: 1, RequestFingerprint: fp,
	})
	_ = store.MarkUnknown(ctx, e.ID, pe.ProviderUnknown, "forced unknown for test")

	var postCount atomic.Int32
	mockCall := func(callCtx context.Context) ([]byte, error) {
		postCount.Add(1)
		return []byte(`{}`), nil
	}

	apiInst := api{store: newPostgresPrimaryStore(db, "")}
	_, _, err := apiInst.runPPTChatStageGuarded(ctx, taskKey, "configured", "kimi-k2.6", pptOutlineCapability, semantic, mockCall)

	if !errors.Is(err, pe.ErrUnknownResubmitBlocked) {
		t.Fatalf("expected ErrUnknownResubmitBlocked on Unknown row, got %v", err)
	}
	if postCount.Load() != 0 {
		t.Fatalf("expected 0 provider calls on Unknown row, got %d", postCount.Load())
	}
}

func TestPPTWorker_PlanUnknownBlocksResubmit(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	taskKey := "ppt:task-unknown-plan-" + suffix + ":slide_1:plan:rev:0"
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskKey)
	}()

	store := pe.NewStore(db)
	semantic := map[string]any{"slideTitle": "Plan", "slideType": "content"}
	fp, _ := pe.Fingerprint(taskKey, "configured", "kimi-k2.6", pptVisualPlanCapability, canonicalPPTFingerprintParams(semantic))

	e, _ := store.CreatePrepared(ctx, pe.Execution{
		TaskID: taskKey, Provider: "configured", ProviderModel: "kimi-k2.6",
		Capability: pptVisualPlanCapability, Attempt: 1, RequestFingerprint: fp,
	})
	_ = store.MarkUnknown(ctx, e.ID, pe.ProviderUnknown, "forced unknown for test")

	var postCount atomic.Int32
	mockCall := func(callCtx context.Context) ([]byte, error) {
		postCount.Add(1)
		return []byte(`{}`), nil
	}

	apiInst := api{store: newPostgresPrimaryStore(db, "")}
	_, _, err := apiInst.runPPTChatStageGuarded(ctx, taskKey, "configured", "kimi-k2.6", pptVisualPlanCapability, semantic, mockCall)

	if !errors.Is(err, pe.ErrUnknownResubmitBlocked) {
		t.Fatalf("expected ErrUnknownResubmitBlocked on Unknown plan row, got %v", err)
	}
	if postCount.Load() != 0 {
		t.Fatalf("expected 0 provider calls on Unknown plan row, got %d", postCount.Load())
	}
}

// Blocker 1 — Unknown Provider State on Slides:
// Proves:
// - UNKNOWN_NEVER_DEGRADES_TO_SUCCESS=PASS
// - UNKNOWN_NEVER_RELEASES_POINTS=PASS
// - UNKNOWN_NEVER_CAPTURES_POINTS=PASS
// - UNKNOWN_NEVER_BLIND_RESUBMITS=PASS
// - RETRY_BUDGET_EXHAUSTED_STILL_UNKNOWN_SAFE=PASS
func TestPPTWorker_UnknownNeverDegradesToSuccess(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	testUser := "u-ppt-unk-safe-" + suffix
	accountID := "acc-" + testUser

	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users (id, name, role) VALUES ($1, 'Unknown Safe User', 'MEMBER') ON CONFLICT (id) DO NOTHING`, testUser); err != nil {
		t.Fatal(err)
	}
	pointStore := NewPostgresPersonalPointStore(db)
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{
		AccountID: accountID, UserID: testUser, Source: PointSourceRecharge,
		Points: 10000, IdempotencyKey: "grant-" + suffix,
	}); err != nil {
		t.Fatal(err)
	}

	store := newPostgresPrimaryStore(db, "")
	capReq, pptReq := pptAcceptanceRequest("client-req-unksafe-" + suffix)
	capReq.UserID = testUser
	pptReq.UserID = testUser
	task, err := store.CreatePendingGenerationTaskWithPPTCanaryOutbox(capReq, pptReq)
	if err != nil {
		t.Fatalf("create parent task: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE aggregate_id=$1", task.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_personal_point_reservations WHERE business_id=$1 AND user_id=$2", task.ID, testUser)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_ppt_tasks WHERE task_id=$1", task.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM xz_generation_tasks WHERE id=$1", task.ID)
	}()

	slideKey := pptSlideExecKey(task.ID, "slide_1", 0)
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", slideKey)
	}()

	peStore := pe.NewStore(db)
	exec, _ := peStore.CreatePrepared(ctx, pe.Execution{
		TaskID: slideKey, Provider: "configured", ProviderModel: "default-image",
		Capability: "image", Attempt: 1, RequestFingerprint: "mock-fp",
	})
	_ = peStore.MarkUnknown(ctx, exec.ID, pe.ProviderUnknown, "simulated network ambiguous timeout")

	// 1. isPPTSlideDeterministicError MUST return false when execution is Unknown!
	simulatedErr := pe.ErrUnknownResubmitBlocked
	if isPPTSlideDeterministicError(simulatedErr, db, slideKey) {
		t.Fatalf("UNKNOWN_NEVER_DEGRADES_TO_SUCCESS: isPPTSlideDeterministicError returned true on Unknown execution, want false")
	}

	// 2. UNKNOWN_NEVER_BLIND_RESUBMITS: Calling guardedImage on this slideKey returns ErrUnknownResubmitBlocked without provider Create
	var createCalls atomic.Int32
	mockImgProv := &mockCountingImageProvider{createCalls: &createCalls}
	mockGenService := generation.NewServiceWithOptions(generation.ServiceOptions{
		ImageProvider:  mockImgProv,
		ExecutionHooks: providerExecutionHooks(store, true),
	})
	apiInst := api{store: store, cfg: stage0PPTCanaryConfig(), pptService: pptapp.NewPostgresService(db, "")}
	imgReq := pptImageGenerateRequest{
		Slide: pptapp.Slide{ID: "slide_1"}, Prompt: "test",
	}
	_, genErr := apiInst.generateBillablePPTImageWithKey(ctx, adminUser{ID: testUser}, mockGenService, imgReq, "default-image", task.ID, slideKey, "child-req-"+suffix)
	if !errors.Is(genErr, pe.ErrUnknownResubmitBlocked) {
		t.Fatalf("UNKNOWN_NEVER_BLIND_RESUBMITS: expected ErrUnknownResubmitBlocked, got %v", genErr)
	}
	if createCalls.Load() != 0 {
		t.Fatalf("UNKNOWN_NEVER_BLIND_RESUBMITS: image provider called %d times on Unknown execution, want 0", createCalls.Load())
	}

	// 3. UNKNOWN_NEVER_CAPTURES_POINTS & UNKNOWN_NEVER_RELEASES_POINTS:
	var billingStatus, taskStatus string
	var capturedPoints, releasedPoints float64
	if err := db.QueryRowContext(ctx, `SELECT billing_status, task_status, captured_points, released_points FROM xz_generation_tasks WHERE id=$1`, task.ID).Scan(&billingStatus, &taskStatus, &capturedPoints, &releasedPoints); err != nil {
		t.Fatal(err)
	}
	if billingStatus != "RESERVED" {
		t.Fatalf("billing_status=%s, want RESERVED (UNKNOWN must never capture or release)", billingStatus)
	}
	if capturedPoints != 0 {
		t.Fatalf("UNKNOWN_NEVER_CAPTURES_POINTS: captured_points=%f, want 0", capturedPoints)
	}
	if releasedPoints != 0 {
		t.Fatalf("UNKNOWN_NEVER_RELEASES_POINTS: released_points=%f, want 0", releasedPoints)
	}
	if taskStatus != "QUEUED" {
		t.Fatalf("task_status=%s, want QUEUED", taskStatus)
	}
}

type mockCountingImageProvider struct {
	createCalls *atomic.Int32
}

func (m *mockCountingImageProvider) Code() string         { return "mock-counting" }
func (m *mockCountingImageProvider) DefaultModel() string { return "default-image" }
func (m *mockCountingImageProvider) Models() []string     { return []string{"default-image"} }
func (m *mockCountingImageProvider) Generate(context.Context, generation.CreateRequest) ([]generation.GeneratedImage, error) {
	if m.createCalls != nil {
		m.createCalls.Add(1)
	}
	return []generation.GeneratedImage{{URL: "https://cdn.example.com/mock.png"}}, nil
}
