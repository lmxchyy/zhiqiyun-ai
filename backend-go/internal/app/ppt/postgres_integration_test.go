package ppt

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresPPTTaskPersistenceConcurrencyDoesNotMirrorLegacyFile(t *testing.T) {
	db := openPPTIntegrationTestDB(t)

	userID := "ppt_postgres_test_" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(context.Background(), `delete from xz_ppt_tasks where user_id=$1`, userID)
	}()
	legacyPath := filepath.Join(t.TempDir(), "ppt-tasks.json")
	legacyContents := []byte(`{"tasks":[{"taskId":"ppt_legacy_only","userId":"legacy-user"}]}`)
	if err := os.WriteFile(legacyPath, legacyContents, 0o600); err != nil {
		t.Fatal(err)
	}
	services := []*Service{NewPostgresService(db), NewPostgresService(db)}
	concurrencyRequest := GenerateRequest{Owner: testOwner(userID), Prompt: "Postgres active draft", SlideCount: 1}

	type result struct {
		response GenerateResponse
		err      error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, service := range services {
		wg.Add(1)
		go func(service *Service) {
			defer wg.Done()
			response, err := service.GenerateWithConcurrency(concurrencyRequest, 0, 1)
			results <- result{response: response, err: err}
		}(service)
	}
	wg.Wait()
	close(results)
	var draftTaskID string
	var successes, conflicts int
	for item := range results {
		switch {
		case item.err == nil:
			successes++
			draftTaskID = item.response.TaskID
			if item.response.Status != StatusPending {
				t.Fatalf("draft create status = %s, want %s", item.response.Status, StatusPending)
			}
		case errors.Is(item.err, ErrConcurrency):
			conflicts++
		default:
			t.Fatalf("unexpected generation error: %v", item.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}

	draftTask, err := services[1].GetTask(testOwner(userID), draftTaskID)
	if err != nil || draftTask.Stage != StageDraft || len(draftTask.Slides) != 0 {
		t.Fatalf("cross-instance draft read failed: task=%#v err=%v", draftTask, err)
	}

	readyResponse, err := services[0].Generate(GenerateRequest{
		Owner: testOwner(userID), Prompt: "Postgres PPT integration", SlideCount: 1, Theme: "techBlue", ImageSource: "ai",
		Outline: &Outline{Title: "Integration", Slides: []OutlineSlide{{Title: "Cover", Summary: "AI assistant", Layout: "cover", SlideType: "cover"}}},
	})
	if err != nil || readyResponse.Status != StatusSuccess {
		t.Fatalf("ready Generate() response=%#v err=%v", readyResponse, err)
	}
	task, err := services[1].GetTask(testOwner(userID), readyResponse.TaskID)
	if err != nil || task.Stage != StageReady || len(task.Slides) != 1 {
		t.Fatalf("cross-instance ready task read failed: task=%#v err=%v", task, err)
	}
	plan := NormalizeVisualPlan(VisualPlan{VisualType: "illustration"}, VisualPlannerInput{SlideType: "cover", SlideTitle: task.Slides[0].Title})
	const generatedImage = "storage://tenant_default/new-image.png"
	updated, err := services[1].CompleteSlideVisual(testOwner(userID), readyResponse.TaskID, task.Slides[0].ID, plan, VisualAsset{URL: generatedImage, TaskID: "image_task_test", ModelName: "image_model_test", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatal(err)
	}
	assertSlideImageRepresentations(t, updated.Slides[0], generatedImage)
	if updated.Slides[0].VisualTaskID != "image_task_test" || updated.Slides[0].VisualModelName != "image_model_test" || len(updated.Slides[0].VisualHistory) != 0 {
		t.Fatalf("atomic visual update failed: %#v", updated.Slides[0])
	}

	const replacement = "storage://tenant_default/replacement.png"
	replaced, err := services[1].UpdateSlideImage(testOwner(userID), readyResponse.TaskID, task.Slides[0].ID, replacement)
	if err != nil {
		t.Fatal(err)
	}
	assertSlideImageRepresentations(t, replaced.Slides[0], replacement)
	fresh := NewPostgresService(db)
	reread, err := fresh.GetTask(testOwner(userID), readyResponse.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	assertSlideImageRepresentations(t, reread.Slides[0], replacement)
	removed, err := fresh.DisableSlideVisual(testOwner(userID), readyResponse.TaskID, task.Slides[0].ID, VisualPlan{})
	if err != nil {
		t.Fatal(err)
	}
	assertSlideImageRepresentations(t, removed.Slides[0], "")
	reread, err = NewPostgresService(db).GetTask(testOwner(userID), readyResponse.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	assertSlideImageRepresentations(t, reread.Slides[0], "")

	actualLegacyContents, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(actualLegacyContents) != string(legacyContents) {
		t.Fatalf("postgres mutations changed legacy file\nwant=%s\ngot=%s", legacyContents, actualLegacyContents)
	}
}

func TestPostgresPPTActiveStateDoesNotExpireAndTerminalStatesDoNotBlock(t *testing.T) {
	db := openPPTIntegrationTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	service := NewPostgresService(db)
	if err := service.ensurePostgresReady(ctx); err != nil {
		t.Fatal(err)
	}
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	users := []string{"ppt_active_" + suffix, "ppt_failed_" + suffix, "ppt_cancelled_" + suffix}
	defer func() {
		for _, userID := range users {
			_, _ = db.ExecContext(context.Background(), `delete from xz_ppt_tasks where user_id=$1`, userID)
		}
	}()

	persistPPTIntegrationTask(t, db, NormalizeTask(Task{
		TaskID: "ppt_active_task_" + suffix, TenantID: "tenant_default", UserID: users[0], Stage: StageGenerating,
		Status: StatusProcessing, SlideCount: 3, CreatedAt: old, UpdatedAt: old,
	}))
	if _, err := service.GenerateWithConcurrency(GenerateRequest{Owner: testOwner(users[0]), Prompt: "blocked", SlideCount: 3}, 0, 1); !errors.Is(err, ErrConcurrency) {
		t.Fatalf("old active task error = %v, want ErrConcurrency", err)
	}

	for index, stage := range []Stage{StageFailed, StageCancelled} {
		persistPPTIntegrationTask(t, db, NormalizeTask(Task{
			TaskID:   "ppt_terminal_task_" + fmt.Sprint(index) + "_" + suffix,
			TenantID: "tenant_default", UserID: users[index+1], Stage: stage, Status: StageStatus(stage),
			SlideCount: 3, CreatedAt: old, UpdatedAt: old,
		}))
		if _, err := service.GenerateWithConcurrency(GenerateRequest{Owner: testOwner(users[index+1]), Prompt: "allowed", SlideCount: 3}, 0, 1); err != nil {
			t.Fatalf("terminal %s task blocked generation: %v", stage, err)
		}
	}
}

func TestPostgresPPTAgentStateTransitionsAndCancellation(t *testing.T) {
	db := openPPTIntegrationTestDB(t)
	service := NewPostgresService(db)
	userID := "ppt_agent_state_" + time.Now().UTC().Format("20060102150405.000000000")
	task, err := service.CreateSession(context.Background(), SessionRequest{
		Owner: testOwner(userID), Prompt: "Postgres agent state", SkillCode: "general", SourceFileIDs: []string{"file_test"}, SlideCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), `delete from xz_ppt_tasks where task_id=$1 and user_id=$2`, task.TaskID, userID)
	}()
	operation, _, err := service.BeginOperation(context.Background(), testOwner(userID), task.TaskID, "message", "message-key", "message-hash")
	if err != nil {
		t.Fatalf("BeginOperation() error = %v", err)
	}
	outline := Outline{Title: "Agent", Slides: []OutlineSlide{{Page: 1, Title: "Only"}}}
	if _, err := service.CompleteOutlineOperation(
		context.Background(), testOwner(userID), task.TaskID, operation, []AgentMessage{{Role: "assistant", Content: "outline"}}, outline); err != nil {
		t.Fatalf("CompleteOutlineOperation() error = %v", err)
	}
	if _, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_test", "confirm-key", "confirm-hash"); err != nil {
		t.Fatalf("BeginGeneration() error = %v", err)
	}
	run, _, err := service.ClaimGenerationRun(context.Background(), testOwner(userID), task.TaskID, time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimGenerationRun() error = %v", err)
	}
	if _, err := service.PersistGeneratedSlide(context.Background(), testOwner(userID), task.TaskID, run, Slide{ID: "slide_1", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "Only"}}}); err != nil {
		t.Fatalf("PersistGeneratedSlide() error = %v", err)
	}
	cancelClaim, _, err := service.BeginCancel(context.Background(), testOwner(userID), task.TaskID, "cancel-key", "cancel-hash")
	if err != nil {
		t.Fatalf("BeginCancel() error = %v", err)
	}
	if _, err := service.CompleteGeneration(context.Background(), testOwner(userID), task.TaskID, run); !errors.Is(err, ErrSessionCancelled) {
		t.Fatalf("CompleteGeneration() after cancel claim error = %v", err)
	}
	if _, err := service.CompleteCancel(context.Background(), testOwner(userID), task.TaskID, cancelClaim); err != nil {
		t.Fatalf("CompleteCancel() error = %v", err)
	}
	reread, err := NewPostgresService(db).GetTask(testOwner(userID), task.TaskID)
	if err != nil {
		t.Fatalf("fresh GetTask() error = %v", err)
	}
	if reread.SessionID != task.TaskID || reread.Stage != StageCancelled || reread.Status != StatusCancelled || reread.Progress != 100 {
		t.Fatalf("reread cancelled task = %#v", reread)
	}
	var columnSessionID, columnSkillCode, columnStage, columnStatus string
	var columnSourceFileIDs []byte
	if err := db.QueryRowContext(context.Background(), `select session_id,skill_code,stage,status,source_file_ids from xz_ppt_tasks where task_id=$1`, task.TaskID).Scan(
		&columnSessionID, &columnSkillCode, &columnStage, &columnStatus, &columnSourceFileIDs,
	); err != nil {
		t.Fatalf("read indexed projection: %v", err)
	}
	if columnSessionID != task.TaskID || columnSkillCode != "general" || columnStage != string(StageCancelled) || columnStatus != StatusCancelled || string(columnSourceFileIDs) != `["file_test"]` {
		t.Fatalf("indexed projection = %q/%q/%q/%q/%s", columnSessionID, columnSkillCode, columnStage, columnStatus, columnSourceFileIDs)
	}
}

func TestPostgresPPTNoopReplayBranchesAreReadOnly(t *testing.T) {
	db := openPPTIntegrationTestDB(t)
	service := NewPostgresService(db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := service.ensurePostgresReady(ctx); err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprint(time.Now().UTC().UnixNano())
	users := []string{}
	newUser := func(prefix string) string {
		userID := prefix + "_" + suffix
		users = append(users, userID)
		return userID
	}
	defer func() {
		for _, userID := range users {
			_, _ = db.ExecContext(context.Background(), `delete from xz_ppt_tasks where user_id=$1`, userID)
		}
	}()

	t.Run("duplicate outline completion", func(t *testing.T) {
		fixture := createCompletedOutlineNoopFixture(t, service, newUser("ppt_noop_outline"))
		before := snapshotPPTIntegrationNoopRow(t, db, fixture.Completed.TaskID)
		got, err := service.CompleteOutlineOperation(
			context.Background(), testOwner(fixture.Completed.UserID), fixture.Completed.TaskID, fixture.Claim,
			[]AgentMessage{{Role: "assistant", Content: "must not append"}},
			Outline{Title: "must not replace", Slides: []OutlineSlide{{Page: 1, Title: "must not replace"}}},
		)
		if err != nil || got.UpdatedAt != fixture.Completed.UpdatedAt || got.Outline == nil || got.Outline.Title != fixture.Completed.Outline.Title {
			t.Fatalf("duplicate outline completion task=%#v err=%v", got, err)
		}
		assertPPTIntegrationNoop(t, db, fixture.Completed.TaskID, before, fixture.Claim.Scope, fixture.Claim.Key)
	})

	t.Run("late operation failure", func(t *testing.T) {
		fixture := createCompletedOutlineNoopFixture(t, service, newUser("ppt_noop_late_fail"))
		before := snapshotPPTIntegrationNoopRow(t, db, fixture.Completed.TaskID)
		got, err := service.FailOperation(context.Background(), testOwner(fixture.Completed.UserID), fixture.Completed.TaskID, fixture.Claim, "LATE_FAILURE_MUST_NOT_WIN")
		if err != nil || got.UpdatedAt != fixture.Completed.UpdatedAt || got.ErrorCode != fixture.Completed.ErrorCode {
			t.Fatalf("late operation failure task=%#v err=%v", got, err)
		}
		assertPPTIntegrationNoop(t, db, fixture.Completed.TaskID, before, fixture.Claim.Scope, fixture.Claim.Key)
	})

	t.Run("late generation claim failure", func(t *testing.T) {
		fixture := createCompletedGenerationNoopFixture(t, service, newUser("ppt_noop_generation_fail"))
		before := snapshotPPTIntegrationNoopRow(t, db, fixture.Completed.TaskID)
		got, err := service.FailGenerationClaim(context.Background(), testOwner(fixture.Completed.UserID), fixture.Completed.TaskID, fixture.ConfirmClaim, "LATE_RESERVATION_FAILURE")
		if err != nil || got.Stage != StageReady || got.UpdatedAt != fixture.Completed.UpdatedAt {
			t.Fatalf("late generation claim failure task=%#v err=%v", got, err)
		}
		assertPPTIntegrationNoop(t, db, fixture.Completed.TaskID, before, idempotencyScopeConfirm, fixture.Key)
	})

	t.Run("existing stale cancel claim", func(t *testing.T) {
		now := time.Date(2026, time.August, 4, 10, 0, 0, 0, time.UTC)
		staleAt := now.Add(-operationProcessingStaleAfter - time.Minute).Format(time.RFC3339Nano)
		userID := newUser("ppt_noop_stale_cancel")
		task := NormalizeTask(Task{
			TaskID: "ppt_noop_stale_cancel_" + suffix, SessionID: "ppt_noop_stale_cancel_" + suffix, TenantID: "tenant_default", UserID: userID,
			SkillCode: "general", Stage: StageGenerating, Status: StatusProcessing, BillingTaskID: "billing-verified",
			Outline: &Outline{Title: "Stale", Slides: []OutlineSlide{{Page: 1, Title: "Only"}}}, SlideCount: 1,
			IdempotencyRecords: []IdempotencyRecord{
				{Scope: idempotencyScopeConfirm, Key: "confirm-stale", RequestHash: "confirm-hash", State: idempotencyStateProcessing, OperationToken: "confirm-token", CreatedAt: staleAt, UpdatedAt: staleAt},
				{Scope: idempotencyScopeCancel, Key: "cancel-existing", RequestHash: "cancel-hash", State: idempotencyStateFailed, ErrorCode: ErrSessionCancelled.Error(), OperationToken: "cancel-token", CreatedAt: staleAt, UpdatedAt: staleAt},
			},
			CreatedAt: staleAt, UpdatedAt: staleAt,
		})
		persistPPTIntegrationTask(t, db, task)
		before := snapshotPPTIntegrationNoopRow(t, db, task.TaskID)
		generationClaim := OperationClaim{Scope: idempotencyScopeConfirm, Key: "confirm-stale", RequestHash: "confirm-hash", OperationToken: "confirm-token"}
		claim, got, err := service.BeginCancelAfterStaleGenerationClaim(
			context.Background(), testOwner(userID), task.TaskID, generationClaim, "cancel-existing", "cancel-hash", now)
		if err != nil || !claim.Replay || claim.OperationToken != "cancel-token" || got.UpdatedAt != task.UpdatedAt {
			t.Fatalf("existing stale cancel replay claim=%#v task=%#v err=%v", claim, got, err)
		}
		assertPPTIntegrationNoop(t, db, task.TaskID, before, idempotencyScopeCancel, "cancel-existing")
	})

	t.Run("completed generation replay", func(t *testing.T) {
		fixture := createCompletedGenerationNoopFixture(t, service, newUser("ppt_noop_generation_ready"))
		before := snapshotPPTIntegrationNoopRow(t, db, fixture.Completed.TaskID)
		replay, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(fixture.Completed), fixture.Completed.TaskID, fixture.BillingTaskID, fixture.Key, fixture.RequestHash)
		if err != nil || replay.Stage != StageReady || replay.UpdatedAt != fixture.Completed.UpdatedAt || len(replay.IdempotencyRecords) != 0 {
			t.Fatalf("completed generation replay task=%#v err=%v", replay, err)
		}
		assertPPTIntegrationNoop(t, db, fixture.Completed.TaskID, before, idempotencyScopeConfirm, fixture.Key)
	})

	t.Run("processing generation replay", func(t *testing.T) {
		fixture := createGeneratingNoopFixture(t, service, newUser("ppt_noop_generation_processing"))
		before := snapshotPPTIntegrationNoopRow(t, db, fixture.Task.TaskID)
		got, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(fixture.Task), fixture.Task.TaskID, fixture.BillingTaskID, fixture.Key, fixture.RequestHash)
		if err != nil || got.Stage != StageGenerating || got.BillingTaskID != fixture.BillingTaskID || got.UpdatedAt != fixture.Task.UpdatedAt {
			t.Fatalf("processing generation replay task=%#v err=%v", got, err)
		}
		assertPPTIntegrationNoop(t, db, fixture.Task.TaskID, before, idempotencyScopeConfirm, fixture.Key)
	})

	t.Run("existing cancel claims", func(t *testing.T) {
		t.Run("processing", func(t *testing.T) {
			userID := newUser("ppt_noop_cancel_processing")
			task, err := service.CreateSession(context.Background(), SessionRequest{Owner: testOwner(userID), Prompt: "Cancel", SkillCode: "general", SlideCount: 1})
			if err != nil {
				t.Fatal(err)
			}
			firstClaim, current, err := service.BeginCancel(context.Background(), testOwner(userID), task.TaskID, "cancel-key", "cancel-hash")
			if err != nil {
				t.Fatal(err)
			}
			before := snapshotPPTIntegrationNoopRow(t, db, task.TaskID)
			replayClaim, got, err := service.BeginCancel(context.Background(), testOwner(userID), task.TaskID, "cancel-key", "cancel-hash")
			if err != nil || !replayClaim.Replay || replayClaim.OperationToken != firstClaim.OperationToken || got.UpdatedAt != current.UpdatedAt {
				t.Fatalf("processing cancel replay claim=%#v task=%#v err=%v", replayClaim, got, err)
			}
			assertPPTIntegrationNoop(t, db, task.TaskID, before, idempotencyScopeCancel, "cancel-key")
		})

		t.Run("completed", func(t *testing.T) {
			fixture := createCompletedCancelNoopFixture(t, service, newUser("ppt_noop_cancel_completed"))
			before := snapshotPPTIntegrationNoopRow(t, db, fixture.Completed.TaskID)
			replayClaim, replay, err := service.BeginCancel(context.Background(), testOwner(fixture.Completed.UserID), fixture.Completed.TaskID, fixture.Key, fixture.RequestHash)
			if err != nil || !replayClaim.Replay || replayClaim.OperationToken != fixture.Claim.OperationToken || replay.UpdatedAt != fixture.Completed.UpdatedAt || len(replay.IdempotencyRecords) != 0 {
				t.Fatalf("completed cancel replay claim=%#v task=%#v err=%v", replayClaim, replay, err)
			}
			assertPPTIntegrationNoop(t, db, fixture.Completed.TaskID, before, idempotencyScopeCancel, fixture.Key)
		})
	})

	t.Run("duplicate cancel completion", func(t *testing.T) {
		fixture := createCompletedCancelNoopFixture(t, service, newUser("ppt_noop_cancel_duplicate"))
		before := snapshotPPTIntegrationNoopRow(t, db, fixture.Completed.TaskID)
		got, err := service.CompleteCancel(context.Background(), testOwner(fixture.Completed.UserID), fixture.Completed.TaskID, fixture.Claim)
		if err != nil || got.Stage != StageCancelled || got.UpdatedAt != fixture.Completed.UpdatedAt || got.CompletedAt != fixture.Completed.CompletedAt {
			t.Fatalf("duplicate cancel completion task=%#v err=%v", got, err)
		}
		assertPPTIntegrationNoop(t, db, fixture.Completed.TaskID, before, idempotencyScopeCancel, fixture.Key)
	})
}

func TestPostgresCreateSessionClientRequestIdempotencyAndConcurrentRecovery(t *testing.T) {
	db := openPPTIntegrationTestDB(t)
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	userID := "ppt_session_idempotency_" + suffix
	defer func() {
		_, _ = db.ExecContext(context.Background(), `delete from xz_ppt_tasks where user_id=$1`, userID)
	}()
	services := []*Service{NewPostgresService(db), NewPostgresService(db)}
	request := SessionRequest{
		Owner: OwnerScope{UserID: userID, TenantID: "tenant_session"}, ClientRequestID: "connector:exact:" + suffix,
		OrganizationID: "organization_session", ContextType: "ENTERPRISE",
		BillingScope: "ENTERPRISE", BillingAccountID: "tenant_session",
		Prompt: "Board update", SkillCode: "general", SourceFileIDs: []string{"file_1", "file_2"},
		SlideCount: 2, Language: "zh", Audience: "management",
	}
	first, err := services[0].CreateSession(context.Background(), request)
	if err != nil {
		t.Fatalf("first CreateSession() error = %v", err)
	}
	replayed, err := services[1].CreateSession(context.Background(), request)
	if err != nil || replayed.TaskID != first.TaskID || replayed.CreatedAt != first.CreatedAt {
		t.Fatalf("matching replay task=%#v first=%#v err=%v", replayed, first, err)
	}
	conflicting := request
	conflicting.Prompt = "Different board update"
	if _, err := services[1].CreateSession(context.Background(), conflicting); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting CreateSession() error = %v, want ErrIdempotencyConflict", err)
	}

	concurrentRequest := request
	concurrentRequest.ClientRequestID = "connector:concurrent:" + suffix
	type result struct {
		task Task
		err  error
	}
	start := make(chan struct{})
	results := make(chan result, len(services))
	var wg sync.WaitGroup
	for _, service := range services {
		wg.Add(1)
		go func(service *Service) {
			defer wg.Done()
			<-start
			task, err := service.CreateSession(context.Background(), concurrentRequest)
			results <- result{task: task, err: err}
		}(service)
	}
	close(start)
	wg.Wait()
	close(results)
	var concurrentTaskID string
	for item := range results {
		if item.err != nil {
			t.Fatalf("concurrent CreateSession() error = %v", item.err)
		}
		if concurrentTaskID == "" {
			concurrentTaskID = item.task.TaskID
		} else if item.task.TaskID != concurrentTaskID {
			t.Fatalf("concurrent task IDs = %q/%q, want one session", concurrentTaskID, item.task.TaskID)
		}
	}
	var rows int
	if err := db.QueryRowContext(context.Background(), `select count(*) from xz_ppt_tasks where user_id=$1 and client_request_id=$2`, userID, concurrentRequest.ClientRequestID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("concurrent persisted rows = %d, want 1", rows)
	}
}

func TestPostgresPPTAgentConcurrentSameRowTransitions(t *testing.T) {
	db := openPPTIntegrationTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	services := []*Service{NewPostgresService(db), NewPostgresService(db)}
	for _, service := range services {
		if err := service.ensurePostgresReady(ctx); err != nil {
			t.Fatalf("ensurePostgresReady() error = %v", err)
		}
	}

	t.Run("confirm then complete versus cancel", func(t *testing.T) {
		userID := "ppt_agent_confirm_cancel_" + time.Now().UTC().Format("20060102150405.000000000")
		task := createOutlineReadySession(t, services[0], userID, 1)
		defer func() {
			_, _ = db.ExecContext(context.Background(), `delete from xz_ppt_tasks where task_id=$1 and user_id=$2`, task.TaskID, userID)
		}()
		type generationResult struct {
			task Task
			err  error
		}
		start := make(chan struct{})
		results := make(chan generationResult, 2)
		var wg sync.WaitGroup
		for _, service := range services {
			wg.Add(1)
			go func(service *Service) {
				defer wg.Done()
				<-start
				got, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_concurrent", "confirm-concurrent", "confirm-hash")
				results <- generationResult{task: got, err: err}
			}(service)
		}
		close(start)
		wg.Wait()
		close(results)
		for result := range results {
			if result.err != nil || result.task.Stage != StageGenerating || result.task.BillingTaskID != "billing_concurrent" {
				t.Fatalf("concurrent confirm task=%#v err=%v", result.task, result.err)
			}
		}
		persisted, err := services[0].GetTask(testOwner(userID), task.TaskID)
		if err != nil {
			t.Fatalf("GetTask() error = %v", err)
		}
		confirmCount := 0
		for _, record := range persisted.IdempotencyRecords {
			if record.Scope == idempotencyScopeConfirm {
				confirmCount++
			}
		}
		if confirmCount != 1 {
			t.Fatalf("confirm record count = %d, want 1", confirmCount)
		}
		run, _, err := services[0].ClaimGenerationRun(context.Background(), testOwner(userID), task.TaskID, time.Now().UTC())
		if err != nil {
			t.Fatalf("ClaimGenerationRun() error = %v", err)
		}
		if _, err := services[0].PersistGeneratedSlide(context.Background(), testOwner(userID), task.TaskID, run, Slide{ID: "slide_1", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "Only"}}}); err != nil {
			t.Fatalf("PersistGeneratedSlide() error = %v", err)
		}

		type terminalResult struct {
			kind   string
			cancel CancelClaim
			err    error
		}
		terminalStart := make(chan struct{})
		terminalResults := make(chan terminalResult, 2)
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-terminalStart
			_, completeErr := services[0].CompleteGeneration(context.Background(), testOwner(userID), task.TaskID, run)
			terminalResults <- terminalResult{kind: "complete", err: completeErr}
		}()
		go func() {
			defer wg.Done()
			<-terminalStart
			cancelClaim, _, cancelErr := services[1].BeginCancel(context.Background(), testOwner(userID), task.TaskID, "cancel-concurrent", "cancel-hash")
			terminalResults <- terminalResult{kind: "cancel", cancel: cancelClaim, err: cancelErr}
		}()
		close(terminalStart)
		wg.Wait()
		close(terminalResults)
		successes := 0
		var acceptedCancel CancelClaim
		for result := range terminalResults {
			if result.err == nil {
				successes++
				if result.kind == "cancel" {
					acceptedCancel = result.cancel
				}
				continue
			}
			if !errors.Is(result.err, ErrInvalidStage) && !errors.Is(result.err, ErrSessionCancelled) {
				t.Fatalf("unexpected %s race error: %v", result.kind, result.err)
			}
		}
		if successes != 1 {
			t.Fatalf("terminal race successes = %d, want 1", successes)
		}
		if acceptedCancel.OperationToken != "" {
			if _, err := services[1].CompleteCancel(context.Background(), testOwner(userID), task.TaskID, acceptedCancel); err != nil {
				t.Fatalf("CompleteCancel() error = %v", err)
			}
		}
		terminal, err := services[0].GetTask(testOwner(userID), task.TaskID)
		if err != nil || terminal.Stage != StageReady && terminal.Stage != StageCancelled {
			t.Fatalf("terminal task=%#v err=%v", terminal, err)
		}
	})

	t.Run("complete versus fail", func(t *testing.T) {
		userID := "ppt_agent_complete_fail_" + time.Now().UTC().Format("20060102150405.000000000")
		task := createOutlineReadySession(t, services[0], userID, 1)
		defer func() {
			_, _ = db.ExecContext(context.Background(), `delete from xz_ppt_tasks where task_id=$1 and user_id=$2`, task.TaskID, userID)
		}()
		if _, err := claimAndBindGenerationForTest(context.Background(), services[0], taskOwner(task), task.TaskID, "billing_complete_fail", "confirm-complete-fail", "confirm-hash"); err != nil {
			t.Fatalf("BeginGeneration() error = %v", err)
		}
		run, _, err := services[0].ClaimGenerationRun(context.Background(), testOwner(userID), task.TaskID, time.Now().UTC())
		if err != nil {
			t.Fatalf("ClaimGenerationRun() error = %v", err)
		}
		if _, err := services[0].PersistGeneratedSlide(context.Background(), testOwner(userID), task.TaskID, run, Slide{ID: "slide_1", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "Only"}}}); err != nil {
			t.Fatalf("PersistGeneratedSlide() error = %v", err)
		}
		start := make(chan struct{})
		errorsCh := make(chan error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, completeErr := services[0].CompleteGeneration(context.Background(), testOwner(userID), task.TaskID, run)
			errorsCh <- completeErr
		}()
		go func() {
			defer wg.Done()
			<-start
			_, failErr := services[1].FailGenerationAfterRelease(context.Background(), testOwner(userID), task.TaskID, run, "PPT_AGENT_PROVIDER_UNAVAILABLE")
			errorsCh <- failErr
		}()
		close(start)
		wg.Wait()
		close(errorsCh)
		successes := 0
		for raceErr := range errorsCh {
			if raceErr == nil {
				successes++
			} else if !errors.Is(raceErr, ErrInvalidStage) {
				t.Fatalf("unexpected complete/fail race error: %v", raceErr)
			}
		}
		if successes != 1 {
			t.Fatalf("complete/fail successes = %d, want 1", successes)
		}
		terminal, err := services[0].GetTask(testOwner(userID), task.TaskID)
		if err != nil || terminal.Stage != StageReady && terminal.Stage != StageFailed {
			t.Fatalf("terminal task=%#v err=%v", terminal, err)
		}
	})
}

func TestPostgresPPTGenerationCleanupFenceBlocksCrossInstanceTakeover(t *testing.T) {
	db := openPPTIntegrationTestDB(t)
	workerA := NewPostgresService(db)
	workerB := NewPostgresService(db)
	userID := "ppt_agent_cleanup_fence_" + time.Now().UTC().Format("20060102150405.000000000")
	task := createOutlineReadySession(t, workerA, userID, 1)
	defer func() {
		_, _ = db.ExecContext(context.Background(), `delete from xz_ppt_tasks where task_id=$1 and user_id=$2`, task.TaskID, userID)
	}()
	if _, err := claimAndBindGenerationForTest(context.Background(), workerA, taskOwner(task), task.TaskID, "billing_cleanup_fence", "confirm-cleanup-fence", "confirm-hash"); err != nil {
		t.Fatalf("BeginGeneration() error = %v", err)
	}
	startedAt := time.Now().UTC()
	runA, _, err := workerA.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, startedAt)
	if err != nil {
		t.Fatalf("ClaimGenerationRun(A) error = %v", err)
	}
	fenceAt := startedAt.Add(generationLeaseDuration + time.Second)
	fenced, _, err := workerA.AcquireGenerationCleanupFence(context.Background(), taskOwner(task), task.TaskID, runA, fenceAt)
	if err != nil {
		t.Fatalf("AcquireGenerationCleanupFence(A) error = %v", err)
	}
	if runB, _, claimErr := workerB.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, fenceAt.Add(time.Second)); !errors.Is(claimErr, ErrGenerationAlreadyRunning) || runB.RunToken != "" {
		t.Fatalf("ClaimGenerationRun(B) during cleanup fence = %#v err=%v, want ErrGenerationAlreadyRunning", runB, claimErr)
	}
	if _, err := workerA.FailGenerationAfterRelease(context.Background(), taskOwner(task), task.TaskID, fenced, "PPT_GENERATION_FAILED"); err != nil {
		t.Fatalf("FailGenerationAfterRelease(A) error = %v", err)
	}
	failed, err := workerB.GetTask(taskOwner(task), task.TaskID)
	if err != nil || failed.Stage != StageFailed || failed.GenerationLease != nil {
		t.Fatalf("fresh cleanup-fenced task=%#v err=%v", failed, err)
	}
}

func TestPostgresReadinessOnMigratedSchemaIsReadOnly(t *testing.T) {
	db := openPPTIntegrationTestDB(t)
	before := snapshotPPTReadinessState(t, db)
	service := NewPostgresService(db)
	if err := service.ensurePostgresReady(context.Background()); err != nil {
		t.Fatalf("first ensurePostgresReady() error = %v", err)
	}
	if err := service.ensurePostgresReady(context.Background()); err != nil {
		t.Fatalf("cached ensurePostgresReady() error = %v", err)
	}
	after := snapshotPPTReadinessState(t, db)
	if after != before {
		t.Fatalf("ensurePostgresReady() changed schema or data\nbefore=%#v\nafter=%#v", before, after)
	}
}

func TestPostgresReadinessWorksWithReadOnlyTransactions(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PPT_READONLY_DATABASE_URL"))
	if dsn == "" {
		t.Skip("PPT_READONLY_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	var readOnly string
	if err := db.QueryRowContext(ctx, `show default_transaction_read_only`).Scan(&readOnly); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(strings.TrimSpace(readOnly), "on") {
		t.Fatalf("default_transaction_read_only = %q, want on", readOnly)
	}
	if err := NewPostgresService(db).ensurePostgresReady(ctx); err != nil {
		t.Fatalf("ensurePostgresReady() under read-only transactions error = %v", err)
	}
}

func TestPostgresReadinessFailsClosedBeforePhaseOneMigration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PPT_MISSING_SCHEMA_DATABASE_URL"))
	if dsn == "" {
		t.Skip("PPT_MISSING_SCHEMA_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	var databaseName string
	if err := db.QueryRowContext(ctx, `select current_database()`).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if !isPPTAgentPhase1IntegrationTestDatabaseName(databaseName) {
		t.Fatalf("PPT_MISSING_SCHEMA_DATABASE_URL must target a dedicated PPT Agent Phase 1 database, got %q", databaseName)
	}
	if hasPPTIntegrationColumn(t, db, "stage") {
		t.Fatalf("missing-schema database %q already has stage", databaseName)
	}
	if err := NewPostgresService(db).ensurePostgresReady(ctx); !errors.Is(err, ErrPostgresUnavailable) {
		t.Fatalf("ensurePostgresReady() error = %v, want wrapped ErrPostgresUnavailable", err)
	}
	if hasPPTIntegrationColumn(t, db, "stage") {
		t.Fatal("ensurePostgresReady() added stage to pre-migration database")
	}
}

type pptReadinessState struct {
	Columns string
	Indexes string
	Rows    string
}

func snapshotPPTReadinessState(t *testing.T, db *sql.DB) pptReadinessState {
	t.Helper()
	var state pptReadinessState
	if err := db.QueryRowContext(context.Background(), `
		select coalesce(string_agg(attname || ':' || attnotnull::text || ':' || format_type(atttypid, atttypmod) || ':' || coalesce(pg_get_expr(adbin, adrelid), ''), E'\n' order by attnum), '')
		from pg_catalog.pg_attribute a
		left join pg_catalog.pg_attrdef d on d.adrelid = a.attrelid and d.adnum = a.attnum
		where a.attrelid = 'public.xz_ppt_tasks'::regclass and a.attnum > 0 and not a.attisdropped
	`).Scan(&state.Columns); err != nil {
		t.Fatalf("snapshot readiness columns: %v", err)
	}
	if err := db.QueryRowContext(context.Background(), `
		select coalesce(string_agg(indexrel.relname || ':' || pg_get_indexdef(indexrel.oid), E'\n' order by indexrel.relname), '')
		from pg_catalog.pg_index i
		join pg_catalog.pg_class indexrel on indexrel.oid = i.indexrelid
		where i.indrelid = 'public.xz_ppt_tasks'::regclass
	`).Scan(&state.Indexes); err != nil {
		t.Fatalf("snapshot readiness indexes: %v", err)
	}
	if err := db.QueryRowContext(context.Background(), `
		select count(*)::text || ':' || coalesce(md5(string_agg(task_id || ':' || user_id || ':' || client_request_id || ':' || status || ':' || coalesce(session_id, '') || ':' || skill_code || ':' || stage || ':' || source_file_ids::text || ':' || raw::text || ':' || xmin::text, E'\n' order by task_id)), md5(''))
		from xz_ppt_tasks
	`).Scan(&state.Rows); err != nil {
		t.Fatalf("snapshot readiness rows: %v", err)
	}
	return state
}

func hasPPTIntegrationColumn(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var exists bool
	if err := db.QueryRowContext(context.Background(), `
		select exists(
			select 1 from pg_catalog.pg_attribute
			where attrelid = 'public.xz_ppt_tasks'::regclass and attname = $1 and attnum > 0 and not attisdropped
		)
	`, name).Scan(&exists); err != nil {
		t.Fatalf("check %s column: %v", name, err)
	}
	return exists
}

func openPPTIntegrationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("PPT_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("PPT_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	var databaseName string
	if err := db.QueryRowContext(ctx, `select current_database()`).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if !isPPTAgentPhase1IntegrationTestDatabaseName(databaseName) {
		t.Fatalf("PPT_TEST_DATABASE_URL must target a dedicated PPT Agent Phase 1 test database, got %q", databaseName)
	}
	return db
}

func isPPTAgentPhase1IntegrationTestDatabaseName(name string) bool {
	const prefix = "ppt_agent_phase1_"
	name = strings.TrimSpace(name)
	return strings.HasPrefix(name, prefix) && strings.TrimSpace(strings.TrimPrefix(name, prefix)) != ""
}

func TestPostgresPPTIntegrationDatabaseNameAllowed(t *testing.T) {
	for _, test := range []struct {
		name string
		want bool
	}{
		{name: "ppt_agent_phase1_candidate_20260805", want: true},
		{name: "ppt_agent_phase1_isolated", want: true},
		{name: "xianzhi_test", want: false},
		{name: "production", want: false},
		{name: "ppt_agent_phase2_candidate", want: false},
	} {
		if got := isPPTAgentPhase1IntegrationTestDatabaseName(test.name); got != test.want {
			t.Errorf("database name %q allowed=%v, want %v", test.name, got, test.want)
		}
	}
}

func persistPPTIntegrationTask(t *testing.T, db *sql.DB, task Task) {
	t.Helper()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := persistPostgresTask(context.Background(), tx, task); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

type pptIntegrationNoopRow struct {
	TaskID            string
	UserID            string
	ClientRequestID   string
	Status            string
	SessionID         string
	SkillCode         string
	Stage             string
	SourceFileIDsJSON string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	RawJSON           string
	RowVersion        string
}

func snapshotPPTIntegrationNoopRow(t *testing.T, db *sql.DB, taskID string) pptIntegrationNoopRow {
	t.Helper()
	var row pptIntegrationNoopRow
	if err := db.QueryRowContext(context.Background(), `
		select task_id,user_id,client_request_id,status,coalesce(session_id,''),skill_code,stage,
		       source_file_ids::text,created_at,updated_at,raw::text,xmin::text
		from xz_ppt_tasks where task_id=$1
	`, taskID).Scan(
		&row.TaskID, &row.UserID, &row.ClientRequestID, &row.Status, &row.SessionID, &row.SkillCode, &row.Stage,
		&row.SourceFileIDsJSON, &row.CreatedAt, &row.UpdatedAt, &row.RawJSON, &row.RowVersion,
	); err != nil {
		t.Fatalf("snapshot PPT PostgreSQL row: %v", err)
	}
	return row
}

func assertPPTIntegrationNoop(t *testing.T, db *sql.DB, taskID string, before pptIntegrationNoopRow, scope, key string) {
	t.Helper()
	after := snapshotPPTIntegrationNoopRow(t, db, taskID)
	if after != before {
		t.Fatalf("read-only replay changed PostgreSQL row\nbefore=%#v\nafter=%#v", before, after)
	}
	beforeResponseJSON := pptResponseJSONFromRaw(t, []byte(before.RawJSON), scope, key)
	afterResponseJSON := pptResponseJSONFromRaw(t, []byte(after.RawJSON), scope, key)
	if afterResponseJSON != beforeResponseJSON {
		t.Fatalf("read-only replay changed responseJSON: before=%q after=%q", beforeResponseJSON, afterResponseJSON)
	}
}
