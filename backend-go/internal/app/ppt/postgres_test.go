package ppt

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTaskFromGenerateRequestKeepsDeckVisualStyleAndNoTextDefault(t *testing.T) {
	req := normalizeRequest(GenerateRequest{
		UserID: "user_a", Prompt: "Enterprise AI", SlideCount: 1, Theme: "techBlue",
		ImageStyle: "corporate 3D", PeopleStyle: "natural", ImageLighting: "soft",
		Outline: &Outline{Slides: []OutlineSlide{{Title: "Cover", Summary: "AI assistant", SlideType: "cover"}}},
	})
	task := taskFromGenerateRequest(req)
	if task.UserID != req.UserID || task.ImageStyle != "corporate 3D" || task.TextInImage {
		t.Fatalf("unexpected task visual defaults: %#v", task)
	}
	if len(task.Slides) != 1 || task.Slides[0].VisualPlan == nil || task.Slides[0].VisualPlan.TextInImage {
		t.Fatalf("unexpected slide visual plan: %#v", task.Slides)
	}
}

func TestNewPostgresServiceWithoutDatabaseFailsInsteadOfFallingBackToFile(t *testing.T) {
	service := NewPostgresService(nil)
	if service == nil {
		t.Fatal("NewPostgresService(nil) returned nil")
	}
	if _, err := service.Generate(GenerateRequest{Owner: testOwner("user_without_database"), Prompt: "must not write a file"}); !errors.Is(err, ErrPostgresUnavailable) {
		t.Fatalf("Generate() error = %v, want ErrPostgresUnavailable", err)
	}
	if _, err := service.HistoryWithError(testOwner("user_without_database")); !errors.Is(err, ErrPostgresUnavailable) {
		t.Fatalf("HistoryWithError() error = %v, want ErrPostgresUnavailable", err)
	}
}

func TestPostgresUpdateSlideImagePersistsReplacementAcrossFreshRead(t *testing.T) {
	db, _ := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	response, err := service.Generate(GenerateRequest{
		Owner: testOwner("user_replace"), Prompt: "Replace image", SlideCount: 1, ImageSource: "ai",
		Outline: &Outline{Slides: []OutlineSlide{{Title: "Cover", SlideType: "cover"}}},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	const replacement = "storage://tenant_default/replacement_image"
	updated, err := service.UpdateSlideImage(testOwner("user_replace"), response.TaskID, "slide_1", replacement)
	if err != nil {
		t.Fatalf("UpdateSlideImage() error = %v", err)
	}
	assertSlideImageRepresentations(t, updated.Slides[0], replacement)

	reread, err := NewPostgresService(db).GetTask(testOwner("user_replace"), response.TaskID)
	if err != nil {
		t.Fatalf("fresh GetTask() error = %v", err)
	}
	assertSlideImageRepresentations(t, reread.Slides[0], replacement)
}

func TestPostgresDisableSlideVisualPersistsRemovalAcrossFreshRead(t *testing.T) {
	db, _ := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	response, err := service.Generate(GenerateRequest{
		Owner: testOwner("user_remove"), Prompt: "Remove image", SlideCount: 1, ImageSource: "ai",
		Outline: &Outline{Slides: []OutlineSlide{{Title: "Cover", SlideType: "cover"}}},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	updated, err := service.DisableSlideVisual(testOwner("user_remove"), response.TaskID, "slide_1", VisualPlan{})
	if err != nil {
		t.Fatalf("DisableSlideVisual() error = %v", err)
	}
	assertSlideImageRepresentations(t, updated.Slides[0], "")

	reread, err := NewPostgresService(db).GetTask(testOwner("user_remove"), response.TaskID)
	if err != nil {
		t.Fatalf("fresh GetTask() error = %v", err)
	}
	assertSlideImageRepresentations(t, reread.Slides[0], "")
}

func TestPostgresConcurrencyCountsOldActiveTaskWithoutWallClockExpiry(t *testing.T) {
	db, _ := newPPTPostgresTestDB(t)
	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	persistPPTPostgresTestTask(t, db, NormalizeTask(Task{
		TaskID: "ppt_old_active", UserID: "user_old_active", Stage: StageGenerating,
		Status: StatusProcessing, SlideCount: 3, CreatedAt: old, UpdatedAt: old,
	}))

	_, err := NewPostgresService(db).GenerateWithConcurrency(GenerateRequest{
		Owner: testOwner("user_old_active"), Prompt: "second task", SlideCount: 3,
	}, 0, 1)
	if !errors.Is(err, ErrConcurrency) {
		t.Fatalf("GenerateWithConcurrency() error = %v, want ErrConcurrency", err)
	}
}

func TestPostgresConcurrencyIgnoresFailedAndCancelledTasks(t *testing.T) {
	for _, stage := range []Stage{StageFailed, StageCancelled} {
		t.Run(string(stage), func(t *testing.T) {
			db, _ := newPPTPostgresTestDB(t)
			userID := "user_terminal_" + strings.ToLower(string(stage))
			old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
			persistPPTPostgresTestTask(t, db, NormalizeTask(Task{
				TaskID: "ppt_terminal", UserID: userID, Stage: stage,
				Status: StageStatus(stage), SlideCount: 3, CreatedAt: old, UpdatedAt: old,
			}))

			if _, err := NewPostgresService(db).GenerateWithConcurrency(GenerateRequest{
				Owner: testOwner(userID), Prompt: "allowed task", SlideCount: 3,
			}, 0, 1); err != nil {
				t.Fatalf("terminal %s task consumed concurrency slot: %v", stage, err)
			}
		})
	}
}

func TestGenerateBackendParityWithAndWithoutOutline(t *testing.T) {
	tests := []struct {
		name       string
		outline    *Outline
		wantStage  Stage
		wantStatus string
	}{
		{
			name: "with outline", wantStage: StageReady, wantStatus: StatusSuccess,
			outline: &Outline{Slides: []OutlineSlide{{Title: "Cover", Summary: "Ready body", SlideType: "cover"}}},
		},
		{name: "without outline", wantStage: StageDraft, wantStatus: StatusPending},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := GenerateRequest{
				Owner: testOwner("user_parity"), Prompt: "Backend parity", SlideCount: 1,
				ImageSource: "none", Outline: test.outline,
			}
			memory := NewService()
			memoryResponse, err := memory.Generate(request)
			if err != nil {
				t.Fatalf("memory Generate() error = %v", err)
			}

			db, state := newPPTPostgresTestDB(t)
			postgres := NewPostgresService(db)
			postgresResponse, err := postgres.Generate(request)
			if err != nil {
				t.Fatalf("postgres Generate() error = %v", err)
			}
			row, ok := state.snapshot(postgresResponse.TaskID)
			if !ok {
				t.Fatalf("postgres task %q was not persisted", postgresResponse.TaskID)
			}
			var persisted Task
			if err := json.Unmarshal(row.raw, &persisted); err != nil {
				t.Fatalf("decode persisted task: %v", err)
			}

			memoryTask, err := memory.GetTask(testOwner("user_parity"), memoryResponse.TaskID)
			if err != nil {
				t.Fatalf("memory GetTask() error = %v", err)
			}
			postgresTask, err := NewPostgresService(db).GetTask(testOwner("user_parity"), postgresResponse.TaskID)
			if err != nil {
				t.Fatalf("postgres GetTask() error = %v", err)
			}
			if memoryResponse.Status != test.wantStatus || postgresResponse.Status != test.wantStatus {
				t.Fatalf("create statuses memory/postgres = %s/%s, want %s", memoryResponse.Status, postgresResponse.Status, test.wantStatus)
			}
			if memoryTask.Stage != test.wantStage || postgresTask.Stage != test.wantStage || persisted.Stage != test.wantStage {
				t.Fatalf("stages memory/postgres/persisted = %s/%s/%s, want %s", memoryTask.Stage, postgresTask.Stage, persisted.Stage, test.wantStage)
			}
			if memoryTask.Status != test.wantStatus || postgresTask.Status != test.wantStatus || persisted.Status != test.wantStatus || row.status != test.wantStatus {
				t.Fatalf("statuses memory/postgres/raw/column = %s/%s/%s/%s, want %s", memoryTask.Status, postgresTask.Status, persisted.Status, row.status, test.wantStatus)
			}
		})
	}
}

func TestCreateSessionClientRequestIdempotency(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	request := SessionRequest{
		Owner: OwnerScope{UserID: " user_session_idempotent ", TenantID: " tenant_session "}, ClientRequestID: " connector:message-1 ",
		OrganizationID: " organization_session ", ContextType: " ENTERPRISE ",
		BillingScope: " ENTERPRISE ", BillingAccountID: " tenant_session ",
		Prompt: " Board update ", SkillCode: " general ", SourceFileIDs: []string{" file_1 ", "file_2"},
		SlideCount: 2, Language: " zh ", Audience: " management ",
	}

	first, err := service.CreateSession(context.Background(), request)
	if err != nil {
		t.Fatalf("first CreateSession() error = %v", err)
	}
	second, err := service.CreateSession(context.Background(), request)
	if err != nil {
		t.Fatalf("matching CreateSession() error = %v", err)
	}
	if second.TaskID != first.TaskID || second.SessionID != first.SessionID || second.CreatedAt != first.CreatedAt {
		t.Fatalf("matching CreateSession() returned a different session: first=%#v second=%#v", first, second)
	}
	state.mu.Lock()
	rowCount := len(state.tasks)
	state.mu.Unlock()
	if rowCount != 1 {
		t.Fatalf("matching CreateSession() persisted %d rows, want 1", rowCount)
	}

	conflicting := request
	conflicting.Prompt = "Different board update"
	if _, err := service.CreateSession(context.Background(), conflicting); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting CreateSession() error = %v, want ErrIdempotencyConflict", err)
	}
}

func TestCreateSessionDeckSpecSurvivesReloadAndIdempotentReplay(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	request := SessionRequest{
		Owner:          OwnerScope{TenantID: "tenant_deck_spec", UserID: "user_deck_spec"},
		OrganizationID: "organization_deck_spec", ContextType: "PERSONAL",
		BillingScope: "PERSONAL", BillingAccountID: "user_deck_spec",
		ClientRequestID: "deck-spec-replay", Prompt: "Deck spec", SkillCode: "general",
		SlideCount: 2, Language: "en", Audience: "investor",
		DeckSpec: DeckSpec{
			Tone: "pitch", TextContent: "detailed", Scenario: "analysis-report",
			GenerationAspectRatio: "16:9", Theme: "midnight", AutoThemeEnabled: true,
			EnableWebSearch: true, ImageSource: "ai", TextModel: "kimi-k2.6", ImageModel: "gpt-image-2",
			ImageStyle: "editorial isometric", PeopleStyle: "natural professionals",
			ImageLighting: "warm studio", ImageComposition: "image_left", TextInImage: false,
		},
	}

	created, err := NewPostgresService(db).CreateSession(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if got := deckSpecFromTask(created); got != request.DeckSpec {
		t.Fatalf("created DeckSpec = %#v, want %#v", got, request.DeckSpec)
	}

	reloaded, err := NewPostgresService(db).GetTask(request.Owner, created.TaskID)
	if err != nil {
		t.Fatalf("GetTask() reload error = %v", err)
	}
	if got := deckSpecFromTask(reloaded); got != request.DeckSpec {
		t.Fatalf("reloaded DeckSpec = %#v, want %#v", got, request.DeckSpec)
	}

	replayed, err := NewPostgresService(db).CreateSession(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateSession() replay error = %v", err)
	}
	if replayed.TaskID != created.TaskID || deckSpecFromTask(replayed) != request.DeckSpec {
		t.Fatalf("replayed task = %#v, want task %q with DeckSpec %#v", replayed, created.TaskID, request.DeckSpec)
	}
	state.mu.Lock()
	rowCount := len(state.tasks)
	state.mu.Unlock()
	if rowCount != 1 {
		t.Fatalf("DeckSpec replay persisted %d rows, want 1", rowCount)
	}

	conflicting := request
	conflicting.DeckSpec.ImageSource = "none"
	if _, err := NewPostgresService(db).CreateSession(context.Background(), conflicting); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("DeckSpec conflict error = %v, want ErrIdempotencyConflict", err)
	}
}

func TestReloadedDeckSpecDrivesBillingImageIntentAfterIdempotentReplay(t *testing.T) {
	for _, test := range []struct {
		name        string
		imageSource string
		wantImages  bool
	}{
		{name: "image enabled", imageSource: "ai", wantImages: true},
		{name: "images disabled", imageSource: "none", wantImages: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, _ := newPPTPostgresTestDB(t)
			request := SessionRequest{
				Owner:          OwnerScope{TenantID: "tenant_reload_billing", UserID: "user_reload_billing"},
				OrganizationID: "organization_reload_billing", ContextType: "PERSONAL",
				BillingScope: "PERSONAL", BillingAccountID: "user_reload_billing",
				ClientRequestID: "reload-billing-" + strings.ReplaceAll(test.name, " ", "-"),
				Prompt:          "Reload billing image intent", SkillCode: "general", SlideCount: 2,
				DeckSpec: DeckSpec{ImageSource: test.imageSource, TextModel: "kimi-k2.6", ImageModel: "gpt-image-2"},
			}
			created, err := NewPostgresService(db).CreateSession(context.Background(), request)
			if err != nil {
				t.Fatalf("CreateSession() error = %v", err)
			}
			reloaded, err := NewPostgresService(db).GetTask(request.Owner, created.TaskID)
			if err != nil {
				t.Fatalf("GetTask() reload error = %v", err)
			}
			replayed, err := NewPostgresService(db).CreateSession(context.Background(), request)
			if err != nil {
				t.Fatalf("CreateSession() replay error = %v", err)
			}
			if replayed.TaskID != created.TaskID {
				t.Fatalf("replay task ID = %q, want %q", replayed.TaskID, created.TaskID)
			}
			for name, task := range map[string]Task{"reloaded": reloaded, "replayed": replayed} {
				if got := task.WithImages(); got != test.wantImages {
					t.Fatalf("%s task WithImages() = %v, want %v; DeckSpec=%#v", name, got, test.wantImages, deckSpecFromTask(task))
				}
			}
		})
	}
}

func TestCreateSessionConcurrentClientRequestReturnsOneSession(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	services := []*Service{NewPostgresService(db), NewPostgresService(db)}
	request := SessionRequest{
		Owner: OwnerScope{UserID: "user_session_concurrent", TenantID: "tenant_concurrent"}, ClientRequestID: "connector:concurrent-message",
		OrganizationID: "organization_concurrent", ContextType: "ENTERPRISE",
		BillingScope: "ENTERPRISE", BillingAccountID: "tenant_concurrent",
		Prompt: "Concurrent board update", SkillCode: "general", SourceFileIDs: []string{"file_1"},
		SlideCount: 2, Language: "zh", Audience: "management",
	}
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
			task, err := service.CreateSession(context.Background(), request)
			results <- result{task: task, err: err}
		}(service)
	}
	close(start)
	wg.Wait()
	close(results)
	var taskID string
	for item := range results {
		if item.err != nil {
			t.Fatalf("concurrent CreateSession() error = %v", item.err)
		}
		if taskID == "" {
			taskID = item.task.TaskID
		} else if item.task.TaskID != taskID {
			t.Fatalf("concurrent CreateSession() task IDs = %q and %q, want one session", taskID, item.task.TaskID)
		}
	}
	state.mu.Lock()
	rowCount := len(state.tasks)
	state.mu.Unlock()
	if rowCount != 1 {
		t.Fatalf("concurrent CreateSession() persisted %d rows, want 1", rowCount)
	}
}

func TestSessionOperationStateMachineAndIdempotency(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task, err := service.CreateSession(context.Background(), SessionRequest{
		Owner: OwnerScope{UserID: "user_session", TenantID: "tenant_session"}, Prompt: "Board update", SkillCode: "general",
		SourceFileIDs: []string{"file_1"}, SlideCount: 2, Language: "zh", Audience: "management",
		OrganizationID: "organization_session", ContextType: "ENTERPRISE",
		BillingScope: "ENTERPRISE", BillingAccountID: "tenant_session",
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if task.TaskID == "" || task.SessionID != task.TaskID || task.Stage != StageDraft || task.Status != StatusPending {
		t.Fatalf("CreateSession() task = %#v", task)
	}
	if task.TenantID != "tenant_session" || task.OrganizationID != "organization_session" || task.ContextType != "ENTERPRISE" || task.BillingScope != "ENTERPRISE" || task.BillingAccountID != "tenant_session" {
		t.Fatalf("CreateSession() tenant context = %#v", task)
	}
	row, ok := state.snapshot(task.TaskID)
	if !ok || !strings.Contains(string(row.raw), `"tenantId":"tenant_session"`) || !strings.Contains(string(row.raw), `"organizationId":"organization_session"`) {
		t.Fatalf("CreateSession() raw tenant context missing: %s", row.raw)
	}

	if _, _, err := service.BeginOperation(context.Background(), taskOwner(task), task.TaskID, "message", "", "hash_1"); !errors.Is(err, ErrIdempotencyKeyRequired) {
		t.Fatalf("missing key error = %v, want ErrIdempotencyKeyRequired", err)
	}
	claim, claimedTask, err := service.BeginOperation(context.Background(), taskOwner(task), task.TaskID, "message", "key_1", "hash_1")
	if err != nil {
		t.Fatalf("BeginOperation() error = %v", err)
	}
	if claim.OperationToken == "" || claim.Replay || len(claimedTask.IdempotencyRecords) != 1 || claimedTask.IdempotencyRecords[0].State != idempotencyStateProcessing {
		t.Fatalf("BeginOperation() claim/task = %#v %#v", claim, claimedTask)
	}
	processingRow, ok := state.snapshot(task.TaskID)
	if !ok {
		t.Fatalf("processing task %q was not persisted", task.TaskID)
	}
	processingWrites := state.strictUpsertCount()
	replay, _, err := service.BeginOperation(context.Background(), taskOwner(task), task.TaskID, "message", "key_1", "hash_1")
	if !errors.Is(err, ErrOperationInProgress) || !replay.InFlight || replay.CompletedReplay || replay.OperationToken != claim.OperationToken {
		t.Fatalf("same-key in-flight claim = %#v, err = %v", replay, err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, processingRow)
	if got := state.strictUpsertCount(); got != processingWrites {
		t.Fatalf("in-flight replay persisted task: writes=%d, want %d", got, processingWrites)
	}
	if _, _, err := service.BeginOperation(context.Background(), taskOwner(task), task.TaskID, "message", "key_1", "different"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflict error = %v, want ErrIdempotencyConflict", err)
	}

	messages := make([]AgentMessage, 31)
	for index := range messages {
		messages[index] = AgentMessage{Role: "assistant", Content: fmt.Sprintf("message-%02d", index)}
	}
	outline := Outline{Title: "Board update", Slides: []OutlineSlide{{Page: 1, Title: "Cover"}, {Page: 2, Title: "Results"}}}
	completed, err := service.CompleteOutlineOperation(
		context.Background(), taskOwner(task), task.TaskID, claim, messages, outline)
	if err != nil {
		t.Fatalf("CompleteOutlineOperation() error = %v", err)
	}
	if completed.Stage != StageOutlineReady || completed.Status != StatusPending || len(completed.AgentMessages) != 30 || completed.AgentMessages[0].Content != "message-01" {
		t.Fatalf("CompleteOutlineOperation() task = %#v", completed)
	}
	completedRow, ok := state.snapshot(task.TaskID)
	if !ok {
		t.Fatalf("completed task %q was not persisted", task.TaskID)
	}
	completedWrites := state.strictUpsertCount()
	completedReplay, replayTask, err := service.BeginOperation(context.Background(), taskOwner(task), task.TaskID, "message", "key_1", "hash_1")
	if err != nil || !completedReplay.Replay || !completedReplay.CompletedReplay || completedReplay.InFlight || replayTask.Stage != StageOutlineReady {
		t.Fatalf("completed replay = %#v task=%#v err=%v", completedReplay, replayTask, err)
	}
	if completed.UpdatedAt != replayTask.UpdatedAt {
		t.Fatalf("completion/replay updatedAt = %q/%q, want identical metadata", completed.UpdatedAt, replayTask.UpdatedAt)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, completedRow)
	if got := state.strictUpsertCount(); got != completedWrites {
		t.Fatalf("completed replay persisted task: writes=%d, want %d", got, completedWrites)
	}
	badClaim := claim
	badClaim.OperationToken = "wrong-token"
	if _, err := service.FailOperation(context.Background(), taskOwner(task), task.TaskID, badClaim, "PPT_AGENT_PROVIDER_UNAVAILABLE"); !errors.Is(err, ErrOperationTokenMismatch) {
		t.Fatalf("token mismatch error = %v, want ErrOperationTokenMismatch", err)
	}
}

func TestUpdatePostgresTaskContextUsesOneTimestampForCompletedIdempotencySnapshot(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	transactionTime := time.Date(2026, time.August, 3, 20, 30, 0, 123456789, time.UTC)
	timestamp := transactionTime.Format(time.RFC3339Nano)
	task := NormalizeTask(Task{
		TaskID: "ppt_same_tick_completion", SessionID: "ppt_same_tick_completion", UserID: "user_same_tick",
		SkillCode: "general", Stage: StageDraft, Status: StatusPending,
		IdempotencyRecords: []IdempotencyRecord{{
			Scope: "message", Key: "same-tick-key", RequestHash: "same-tick-hash", State: idempotencyStateProcessing,
			OperationToken: "op_same_tick", CreatedAt: timestamp, UpdatedAt: timestamp,
		}},
		CreatedAt: timestamp, UpdatedAt: timestamp,
	})
	persistPPTPostgresTestTask(t, db, task)

	completed, err := service.updatePostgresTaskContextAt(context.Background(), taskOwner(task), task.TaskID, transactionTime, func(current *Task, mutationTime time.Time) error {
		completeIdempotencyRecord(current, 0, mutationTime)
		return nil
	})
	if err != nil {
		t.Fatalf("updatePostgresTaskContext() error = %v", err)
	}
	row, ok := state.snapshot(task.TaskID)
	if !ok {
		t.Fatalf("completed task %q was not persisted", task.TaskID)
	}
	var persisted Task
	if err := json.Unmarshal(row.raw, &persisted); err != nil {
		t.Fatalf("unmarshal persisted task: %v", err)
	}
	record, ok := idempotencyRecordByScope(persisted.IdempotencyRecords, "message")
	if !ok || record.State != idempotencyStateCompleted || strings.TrimSpace(record.ResponseJSON) == "" {
		t.Fatalf("persisted completion record = %#v, found=%v", record, ok)
	}
	replay := idempotencyResponseTask(record, persisted)
	if completed.UpdatedAt != timestamp || persisted.UpdatedAt != timestamp || record.UpdatedAt != timestamp || replay.UpdatedAt != timestamp {
		t.Fatalf(
			"same-tick completion timestamps: completed=%q persisted=%q record=%q replay=%q, want %q",
			completed.UpdatedAt, persisted.UpdatedAt, record.UpdatedAt, replay.UpdatedAt, timestamp,
		)
	}
	if len(completed.IdempotencyRecords) != 1 || len(persisted.IdempotencyRecords) != 1 || len(replay.IdempotencyRecords) != 0 {
		t.Fatalf(
			"completion snapshot idempotency records: completed=%d persisted=%d replay=%d, want 1/1/0",
			len(completed.IdempotencyRecords), len(persisted.IdempotencyRecords), len(replay.IdempotencyRecords),
		)
	}
}

func TestDuplicateCompleteOutlineOperationIsReadOnly(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	fixture := createCompletedOutlineNoopFixture(t, service, "user_noop_outline")
	before, _ := state.snapshot(fixture.Completed.TaskID)
	writes := state.strictUpsertCount()

	got, err := service.CompleteOutlineOperation(
		context.Background(), testOwner(fixture.Completed.UserID), fixture.Completed.TaskID, fixture.Claim,
		[]AgentMessage{{Role: "assistant", Content: "must not append"}},
		Outline{Title: "must not replace", Slides: []OutlineSlide{{Page: 1, Title: "must not replace"}}},
	)
	if err != nil {
		t.Fatalf("duplicate CompleteOutlineOperation() error = %v", err)
	}
	if got.UpdatedAt != fixture.Completed.UpdatedAt || got.Outline == nil || got.Outline.Title != fixture.Completed.Outline.Title || len(got.AgentMessages) != len(fixture.Completed.AgentMessages) {
		t.Fatalf("duplicate completion returned changed task: got=%#v completed=%#v", got, fixture.Completed)
	}
	assertPPTPostgresNoop(t, state, fixture.Completed.TaskID, before, writes, fixture.Claim.Scope, fixture.Claim.Key)
}

func TestLateFailOperationAfterCompletionIsReadOnly(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	fixture := createCompletedOutlineNoopFixture(t, service, "user_noop_late_fail")
	before, _ := state.snapshot(fixture.Completed.TaskID)
	writes := state.strictUpsertCount()

	got, err := service.FailOperation(context.Background(), testOwner(fixture.Completed.UserID), fixture.Completed.TaskID, fixture.Claim, "LATE_FAILURE_MUST_NOT_WIN")
	if err != nil {
		t.Fatalf("late FailOperation() error = %v", err)
	}
	if got.UpdatedAt != fixture.Completed.UpdatedAt || got.ErrorCode != fixture.Completed.ErrorCode {
		t.Fatalf("late failure changed completed task: got=%#v completed=%#v", got, fixture.Completed)
	}
	assertPPTPostgresNoop(t, state, fixture.Completed.TaskID, before, writes, fixture.Claim.Scope, fixture.Claim.Key)
}

func TestFailGenerationClaimAfterCompletionIsReadOnly(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	fixture := createCompletedGenerationNoopFixture(t, service, "user_noop_generation_fail")
	before, _ := state.snapshot(fixture.Completed.TaskID)
	writes := state.strictUpsertCount()

	got, err := service.FailGenerationClaim(context.Background(), testOwner(fixture.Completed.UserID), fixture.Completed.TaskID, fixture.ConfirmClaim, "LATE_RESERVATION_FAILURE")
	if err != nil {
		t.Fatalf("late FailGenerationClaim() error = %v", err)
	}
	if got.Stage != StageReady || got.UpdatedAt != fixture.Completed.UpdatedAt || got.ErrorCode != fixture.Completed.ErrorCode {
		t.Fatalf("late generation-claim failure changed completed task: got=%#v completed=%#v", got, fixture.Completed)
	}
	assertPPTPostgresNoop(t, state, fixture.Completed.TaskID, before, writes, idempotencyScopeConfirm, fixture.Key)
}

func TestExistingStaleGenerationCancelClaimReplayIsReadOnly(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	now := time.Date(2026, time.August, 4, 10, 0, 0, 0, time.UTC)
	staleAt := now.Add(-operationProcessingStaleAfter - time.Minute).Format(time.RFC3339Nano)
	task := NormalizeTask(Task{
		TaskID: "ppt_noop_stale_cancel", SessionID: "ppt_noop_stale_cancel", UserID: "user_noop_stale_cancel",
		SkillCode: "general", Stage: StageGenerating, Status: StatusProcessing, BillingTaskID: "billing-verified",
		Outline: &Outline{Title: "Stale", Slides: []OutlineSlide{{Page: 1, Title: "Only"}}}, SlideCount: 1,
		IdempotencyRecords: []IdempotencyRecord{
			{Scope: idempotencyScopeConfirm, Key: "confirm-stale", RequestHash: "confirm-hash", State: idempotencyStateProcessing, OperationToken: "confirm-token", CreatedAt: staleAt, UpdatedAt: staleAt},
			{Scope: idempotencyScopeCancel, Key: "cancel-existing", RequestHash: "cancel-hash", State: idempotencyStateFailed, ErrorCode: ErrSessionCancelled.Error(), OperationToken: "cancel-token", CreatedAt: staleAt, UpdatedAt: staleAt},
		},
		CreatedAt: staleAt, UpdatedAt: staleAt,
	})
	persistPPTPostgresTestTask(t, db, task)
	before, _ := state.snapshot(task.TaskID)
	writes := state.strictUpsertCount()
	generationClaim := OperationClaim{Scope: idempotencyScopeConfirm, Key: "confirm-stale", RequestHash: "confirm-hash", OperationToken: "confirm-token"}

	claim, got, err := service.BeginCancelAfterStaleGenerationClaim(
		context.Background(), taskOwner(task), task.TaskID, generationClaim, "cancel-existing", "cancel-hash", now,
	)
	if err != nil {
		t.Fatalf("existing stale cancel replay error = %v", err)
	}
	if !claim.Replay || claim.OperationToken != "cancel-token" || got.UpdatedAt != task.UpdatedAt || got.Stage != StageGenerating || got.BillingTaskID != "billing-verified" {
		t.Fatalf("existing stale cancel replay claim/task = %#v %#v", claim, got)
	}
	assertPPTPostgresNoop(t, state, task.TaskID, before, writes, idempotencyScopeCancel, "cancel-existing")
}

func TestBeginGenerationCompletedReadyReplayIsReadOnly(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	fixture := createCompletedGenerationNoopFixture(t, service, "user_noop_generation_ready")
	before, _ := state.snapshot(fixture.Completed.TaskID)
	writes := state.strictUpsertCount()

	replay, err := service.BindGenerationBilling(context.Background(), taskOwner(fixture.Completed), fixture.Completed.TaskID, fixture.ConfirmClaim, fixture.BillingTaskID)
	if err != nil {
		t.Fatalf("completed BeginGeneration() replay error = %v", err)
	}
	if replay.Stage != StageReady || replay.UpdatedAt != fixture.Completed.UpdatedAt || len(replay.IdempotencyRecords) != 0 || len(replay.Slides) != len(fixture.Completed.Slides) {
		t.Fatalf("completed BeginGeneration() replay = %#v, completed=%#v", replay, fixture.Completed)
	}
	assertPPTPostgresNoop(t, state, fixture.Completed.TaskID, before, writes, idempotencyScopeConfirm, fixture.Key)
}

func TestBeginGenerationProcessingGeneratingReplayIsReadOnly(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	fixture := createGeneratingNoopFixture(t, service, "user_noop_generation_processing")
	before, _ := state.snapshot(fixture.Task.TaskID)
	writes := state.strictUpsertCount()

	got, err := service.BindGenerationBilling(context.Background(), taskOwner(fixture.Task), fixture.Task.TaskID, fixture.ConfirmClaim, fixture.BillingTaskID)
	if err != nil {
		t.Fatalf("processing BeginGeneration() replay error = %v", err)
	}
	if got.Stage != StageGenerating || got.BillingTaskID != fixture.BillingTaskID || got.UpdatedAt != fixture.Task.UpdatedAt {
		t.Fatalf("processing BeginGeneration() replay = %#v, want current=%#v", got, fixture.Task)
	}
	assertPPTPostgresNoop(t, state, fixture.Task.TaskID, before, writes, idempotencyScopeConfirm, fixture.Key)
}

func TestBeginCancelExistingClaimsAreReadOnly(t *testing.T) {
	t.Run("processing", func(t *testing.T) {
		db, state := newPPTPostgresTestDB(t)
		service := NewPostgresService(db)
		task, err := service.CreateSession(context.Background(), SessionRequest{Owner: testOwner("user_noop_cancel_processing"), Prompt: "Cancel", SkillCode: "general", SlideCount: 1})
		if err != nil {
			t.Fatal(err)
		}
		firstClaim, current, err := service.BeginCancel(context.Background(), taskOwner(task), task.TaskID, "cancel-key", "cancel-hash")
		if err != nil {
			t.Fatal(err)
		}
		before, _ := state.snapshot(task.TaskID)
		writes := state.strictUpsertCount()

		replayClaim, got, err := service.BeginCancel(context.Background(), taskOwner(task), task.TaskID, "cancel-key", "cancel-hash")
		if err != nil {
			t.Fatalf("processing BeginCancel() replay error = %v", err)
		}
		if !replayClaim.Replay || replayClaim.OperationToken != firstClaim.OperationToken || got.UpdatedAt != current.UpdatedAt || got.Stage != current.Stage {
			t.Fatalf("processing BeginCancel() replay claim/task = %#v %#v", replayClaim, got)
		}
		assertPPTPostgresNoop(t, state, task.TaskID, before, writes, idempotencyScopeCancel, "cancel-key")
	})

	t.Run("completed", func(t *testing.T) {
		db, state := newPPTPostgresTestDB(t)
		service := NewPostgresService(db)
		fixture := createCompletedCancelNoopFixture(t, service, "user_noop_cancel_completed")
		before, _ := state.snapshot(fixture.Completed.TaskID)
		writes := state.strictUpsertCount()

		replayClaim, replay, err := service.BeginCancel(context.Background(), testOwner(fixture.Completed.UserID), fixture.Completed.TaskID, fixture.Key, fixture.RequestHash)
		if err != nil {
			t.Fatalf("completed BeginCancel() replay error = %v", err)
		}
		if !replayClaim.Replay || replayClaim.OperationToken != fixture.Claim.OperationToken || replay.Stage != StageCancelled || replay.UpdatedAt != fixture.Completed.UpdatedAt || len(replay.IdempotencyRecords) != 0 {
			t.Fatalf("completed BeginCancel() replay claim/task = %#v %#v", replayClaim, replay)
		}
		assertPPTPostgresNoop(t, state, fixture.Completed.TaskID, before, writes, idempotencyScopeCancel, fixture.Key)
	})
}

func TestDuplicateCompleteCancelIsReadOnly(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	fixture := createCompletedCancelNoopFixture(t, service, "user_noop_cancel_duplicate")
	before, _ := state.snapshot(fixture.Completed.TaskID)
	writes := state.strictUpsertCount()

	got, err := service.CompleteCancel(context.Background(), testOwner(fixture.Completed.UserID), fixture.Completed.TaskID, fixture.Claim)
	if err != nil {
		t.Fatalf("duplicate CompleteCancel() error = %v", err)
	}
	if got.Stage != StageCancelled || got.UpdatedAt != fixture.Completed.UpdatedAt || got.CompletedAt != fixture.Completed.CompletedAt {
		t.Fatalf("duplicate CompleteCancel() task = %#v, completed=%#v", got, fixture.Completed)
	}
	assertPPTPostgresNoop(t, state, fixture.Completed.TaskID, before, writes, idempotencyScopeCancel, fixture.Key)
}

func TestCompleteImportOutlineOperationPersistsSourceFilesAndReplaysSnapshot(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task, err := service.CreateSession(context.Background(), SessionRequest{
		Owner: testOwner("user_import"), Prompt: "Import", SkillCode: "general", SourceFileIDs: []string{"file_old"}, SlideCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	claim, _, err := service.BeginOperation(context.Background(), taskOwner(task), task.TaskID, "import-outline", "import-key", "import-hash")
	if err != nil {
		t.Fatal(err)
	}
	outline := Outline{Title: "Imported", Slides: []OutlineSlide{{Page: 1, Title: "One"}, {Page: 2, Title: "Two"}}}
	completed, err := service.CompleteImportOutlineOperation(
		context.Background(), taskOwner(task), task.TaskID, claim,
		[]AgentMessage{{Role: "user", Content: "Imported 3 Markdown files"}, {Role: "assistant", Content: "outline"}},
		outline, []string{" file_b ", "file_a", "file_b"},
	)
	if err != nil {
		t.Fatalf("CompleteImportOutlineOperation() error = %v", err)
	}
	if completed.Stage != StageOutlineReady || completed.Outline == nil || len(completed.SourceFileIDs) != 2 || completed.SourceFileIDs[0] != "file_b" || completed.SourceFileIDs[1] != "file_a" {
		t.Fatalf("CompleteImportOutlineOperation() task = %#v", completed)
	}
	row, ok := state.snapshot(task.TaskID)
	if !ok {
		t.Fatalf("completed import task %q was not persisted", task.TaskID)
	}
	var projected []string
	if err := json.Unmarshal(row.sourceFileIDs, &projected); err != nil {
		t.Fatal(err)
	}
	var raw Task
	if err := json.Unmarshal(row.raw, &raw); err != nil {
		t.Fatal(err)
	}
	if len(projected) != 2 || projected[0] != "file_b" || projected[1] != "file_a" || len(raw.SourceFileIDs) != 2 || raw.SourceFileIDs[0] != "file_b" || raw.SourceFileIDs[1] != "file_a" {
		t.Fatalf("source file projection/raw mismatch: projected=%#v raw=%#v", projected, raw.SourceFileIDs)
	}
	writes := state.strictUpsertCount()
	replayClaim, replayTask, err := service.BeginOperation(context.Background(), taskOwner(task), task.TaskID, "import-outline", "import-key", "import-hash")
	if err != nil || !replayClaim.CompletedReplay {
		t.Fatalf("completed import replay claim=%#v err=%v", replayClaim, err)
	}
	if replayTask.UpdatedAt != completed.UpdatedAt || len(replayTask.SourceFileIDs) != 2 || replayTask.SourceFileIDs[0] != "file_b" || replayTask.SourceFileIDs[1] != "file_a" {
		t.Fatalf("completed import replay task = %#v, completed = %#v", replayTask, completed)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, row)
	if got := state.strictUpsertCount(); got != writes {
		t.Fatalf("completed import replay persisted task: writes=%d, want %d", got, writes)
	}
}

func TestCompleteOutlineOperationsRejectInvalidActualSizeWithoutWrite(t *testing.T) {
	operations := []struct {
		name     string
		scope    string
		complete func(*Service, OwnerScope, string, OperationClaim, Outline) (Task, error)
	}{
		{
			name:  "generated outline",
			scope: "message",
			complete: func(service *Service, owner OwnerScope, taskID string, claim OperationClaim, outline Outline) (Task, error) {
				return service.CompleteOutlineOperation(context.Background(), owner, taskID, claim, []AgentMessage{{Role: "assistant", Content: "outline"}}, outline)
			},
		},
		{
			name:  "imported outline",
			scope: "import-outline",
			complete: func(service *Service, owner OwnerScope, taskID string, claim OperationClaim, outline Outline) (Task, error) {
				return service.CompleteImportOutlineOperation(context.Background(), owner, taskID, claim, []AgentMessage{{Role: "assistant", Content: "import"}}, outline, []string{"new-file"})
			},
		},
	}
	invalidSizes := []struct {
		name  string
		count int
	}{
		{name: "empty", count: 0},
		{name: "oversize", count: 13},
	}
	for _, operation := range operations {
		for _, size := range invalidSizes {
			t.Run(operation.name+"/"+size.name, func(t *testing.T) {
				db, state := newPPTPostgresTestDB(t)
				service := NewPostgresService(db)
				owner := OwnerScope{TenantID: "tenant_skill_outline", UserID: "user_" + operation.scope + "_" + size.name}
				task, err := service.CreateSession(context.Background(), SessionRequest{
					Owner: owner, Prompt: "Skill-bound outline", SkillCode: "meeting_summary", SlideCount: 1, SourceFileIDs: []string{"old-file"},
				})
				if err != nil {
					t.Fatalf("CreateSession() error = %v", err)
				}
				claim, _, err := service.BeginOperation(context.Background(), owner, task.TaskID, operation.scope, "outline-size-key", "outline-size-hash")
				if err != nil {
					t.Fatalf("BeginOperation() error = %v", err)
				}
				before, _ := state.snapshot(task.TaskID)
				if _, err := operation.complete(service, owner, task.TaskID, claim, outlineWithTestSlideCount(size.count)); !errors.Is(err, ErrInvalidSkill) {
					t.Fatalf("complete outline with %d slides error = %v, want ErrInvalidSkill", size.count, err)
				}
				assertPPTPostgresRowUnchanged(t, state, task.TaskID, before)
			})
		}
	}
}

func TestCompleteOutlineOperationsAcceptExactSkillMaxSlides(t *testing.T) {
	operations := []struct {
		name     string
		scope    string
		complete func(*Service, OwnerScope, string, OperationClaim, Outline) (Task, error)
	}{
		{
			name:  "generated outline",
			scope: "message",
			complete: func(service *Service, owner OwnerScope, taskID string, claim OperationClaim, outline Outline) (Task, error) {
				return service.CompleteOutlineOperation(context.Background(), owner, taskID, claim, nil, outline)
			},
		},
		{
			name:  "imported outline",
			scope: "import-outline",
			complete: func(service *Service, owner OwnerScope, taskID string, claim OperationClaim, outline Outline) (Task, error) {
				return service.CompleteImportOutlineOperation(context.Background(), owner, taskID, claim, nil, outline, []string{" boundary-file ", "boundary-file"})
			},
		},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			db, _ := newPPTPostgresTestDB(t)
			service := NewPostgresService(db)
			owner := OwnerScope{TenantID: "tenant_skill_boundary", UserID: "user_" + operation.scope}
			task, err := service.CreateSession(context.Background(), SessionRequest{
				Owner: owner, Prompt: "Skill boundary", SkillCode: "meeting_summary", SlideCount: 1, SourceFileIDs: []string{"old-file"},
			})
			if err != nil {
				t.Fatalf("CreateSession() error = %v", err)
			}
			claim, _, err := service.BeginOperation(context.Background(), owner, task.TaskID, operation.scope, "outline-boundary-key", "outline-boundary-hash")
			if err != nil {
				t.Fatalf("BeginOperation() error = %v", err)
			}
			completed, err := operation.complete(service, owner, task.TaskID, claim, outlineWithTestSlideCount(12))
			if err != nil {
				t.Fatalf("complete exact-boundary outline error = %v", err)
			}
			if completed.Stage != StageOutlineReady || completed.SlideCount != 12 || completed.Outline == nil || len(completed.Outline.Slides) != 12 {
				t.Fatalf("exact-boundary task = %#v", completed)
			}
			record, ok := idempotencyRecordByScope(completed.IdempotencyRecords, operation.scope)
			if !ok || record.State != idempotencyStateCompleted {
				t.Fatalf("exact-boundary idempotency record = %#v found=%v", record, ok)
			}
			if operation.scope == "import-outline" && (len(completed.SourceFileIDs) != 1 || completed.SourceFileIDs[0] != "boundary-file") {
				t.Fatalf("exact-boundary import files = %#v", completed.SourceFileIDs)
			}
		})
	}
}

func outlineWithTestSlideCount(count int) Outline {
	slides := make([]OutlineSlide, count)
	for index := range slides {
		slides[index] = OutlineSlide{Page: index + 1, Title: fmt.Sprintf("Slide %d", index+1)}
	}
	return Outline{Title: "Skill-bound outline", Slides: slides}
}

func TestCompleteSlideRevisionAtomicallyReplacesOnlyReadyTargetAndSnapshotsReplay(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	originalFirst := NormalizeSlideIR(Slide{
		ID: "slide_1", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "Original first"}, {Type: "paragraph", Text: "Keep this page"}, {Type: "bullets", Items: []string{"First point"}}, {Type: "image", ImageRef: "https://cdn.example/first.png"}, {Type: "note", Text: "Keep notes"}}, Layout: "imageText",
		VisualTaskID: "visual_1", VisualModelName: "image-model", VisualStatus: "success",
	})
	originalSecond := NormalizeSlideIR(Slide{
		ID: "slide_2", Page: 2, Blocks: []SlideBlock{{Type: "title", Text: "Original second"}, {Type: "paragraph", Text: "Old body"}, {Type: "bullets", Items: []string{"Old point"}}, {Type: "image", ImageRef: "https://cdn.example/second.png"}, {Type: "note", Text: "Original notes"}}, Layout: "imageText",
		VisualTaskID: "visual_2", VisualModelName: "image-model", VisualStatus: "success",
	})
	task := NormalizeTask(Task{
		TaskID: "ppt_revision", SessionID: "ppt_revision", UserID: "user_revision", SkillCode: "general",
		Stage: StageReady, Status: StatusSuccess, Progress: 100, CurrentPage: 2, SlideCount: 2,
		Slides: []Slide{originalFirst, originalSecond}, CreatedAt: now, UpdatedAt: now,
	})
	persistPPTPostgresTestTask(t, db, task)

	claim, _, err := service.BeginOperation(context.Background(), taskOwner(task), task.TaskID, "revise-slide", "revision-key", "revision-hash")
	if err != nil {
		t.Fatalf("BeginOperation() error = %v", err)
	}
	revised, err := service.CompleteSlideRevision(
		context.Background(), taskOwner(task), task.TaskID, claim, "slide_2", Slide{
			ID: "provider-must-not-change-id", Page: 99, Layout: "provider-must-not-change-layout",
			Blocks: []SlideBlock{
				{Type: "title", Text: "Investor update"},
				{Type: "paragraph", Text: "Revised body"},
				{Type: "bullets", Items: []string{"Growth", "Margin"}},
				{Type: "image", ImageRef: "https://attacker.invalid/replacement.png"},
				{Type: "note", Text: "Provider note must not replace stored notes"},
			},
		})
	if err != nil {
		t.Fatalf("CompleteSlideRevision() error = %v", err)
	}
	if revised.Stage != StageReady || revised.Status != StatusSuccess || revised.Progress != 100 || revised.CurrentPage != 2 || len(revised.Slides) != 2 {
		t.Fatalf("revision changed terminal task shape: %#v", revised)
	}
	if got := revised.Slides[0]; got.ID != originalFirst.ID || got.Page != originalFirst.Page || slideTitle(got) != slideTitle(originalFirst) || slideContent(got) != slideContent(originalFirst) || slideImageRef(got) != slideImageRef(originalFirst) {
		t.Fatalf("non-target slide changed: got=%#v want=%#v", got, originalFirst)
	}
	got := revised.Slides[1]
	if got.ID != originalSecond.ID || got.Page != originalSecond.Page || got.Layout != originalSecond.Layout {
		t.Fatalf("target coordinates/layout changed: %#v", got)
	}
	if slideTitle(got) != "Investor update" || slideContent(got) != "Revised body" || len(got.Blocks) < 3 || got.Blocks[2].Items[0] != "Growth" {
		t.Fatalf("target content was not blocks-first normalized: %#v", got)
	}
	if slideImageRef(got) != slideImageRef(originalSecond) || firstSlideBlockText(got, "note") != firstSlideBlockText(originalSecond, "note") || got.VisualTaskID != originalSecond.VisualTaskID || got.VisualModelName != originalSecond.VisualModelName || got.VisualStatus != originalSecond.VisualStatus {
		t.Fatalf("revision changed visual/note metadata: got=%#v want=%#v", got, originalSecond)
	}
	record, ok := idempotencyRecordByScope(revised.IdempotencyRecords, "revise-slide")
	if !ok || record.State != idempotencyStateCompleted || strings.TrimSpace(record.ResponseJSON) == "" {
		t.Fatalf("revision idempotency snapshot = %#v, found=%v", record, ok)
	}
	writes := state.strictUpsertCount()
	replayClaim, replayTask, err := service.BeginOperation(context.Background(), taskOwner(task), task.TaskID, "revise-slide", "revision-key", "revision-hash")
	if err != nil || !replayClaim.CompletedReplay || replayTask.UpdatedAt != revised.UpdatedAt || slideTitle(replayTask.Slides[1]) != "Investor update" {
		t.Fatalf("revision replay claim=%#v task=%#v err=%v", replayClaim, replayTask, err)
	}
	if gotWrites := state.strictUpsertCount(); gotWrites != writes {
		t.Fatalf("revision replay persisted again: writes=%d want=%d", gotWrites, writes)
	}
}

func TestBeginOperationReclaimsStaleProcessingAndRejectsOldToken(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	oldTime := time.Now().UTC().Add(-operationProcessingStaleAfter - time.Minute).Format(time.RFC3339Nano)
	task := NormalizeTask(Task{
		TaskID: "ppt_stale_operation", SessionID: "ppt_stale_operation", UserID: "user_stale_operation",
		Stage: StageDraft, Status: StatusPending, Prompt: "Recover stale operation", SlideCount: 1,
		CreatedAt: oldTime, UpdatedAt: oldTime,
		IdempotencyRecords: []IdempotencyRecord{{
			Scope: "message", Key: "key_stale", RequestHash: "hash_stale", State: idempotencyStateProcessing,
			OperationToken: "op_old", CreatedAt: oldTime, UpdatedAt: oldTime,
		}},
	})
	persistPPTPostgresTestTask(t, db, task)
	staleRow, ok := state.snapshot(task.TaskID)
	if !ok {
		t.Fatalf("stale task %q was not persisted", task.TaskID)
	}

	claim, claimedTask, err := service.BeginOperation(context.Background(), taskOwner(task), task.TaskID, "message", "key_stale", "hash_stale")
	if err != nil {
		t.Fatalf("BeginOperation() stale reclaim error = %v", err)
	}
	if claim.OperationToken == "" || claim.OperationToken == "op_old" || claim.InFlight || claim.CompletedReplay {
		t.Fatalf("reclaimed claim = %#v", claim)
	}
	if len(claimedTask.IdempotencyRecords) != 1 || claimedTask.IdempotencyRecords[0].OperationToken != claim.OperationToken || claimedTask.IdempotencyRecords[0].State != idempotencyStateProcessing {
		t.Fatalf("reclaimed task record = %#v", claimedTask.IdempotencyRecords)
	}
	reclaimedRow, ok := state.snapshot(task.TaskID)
	if !ok || !reclaimedRow.updatedAt.After(staleRow.updatedAt) || string(reclaimedRow.raw) == string(staleRow.raw) {
		t.Fatalf("stale reclaim did not persist new token/time: before=%#v after=%#v", staleRow, reclaimedRow)
	}

	oldClaim := OperationClaim{Scope: "message", Key: "key_stale", RequestHash: "hash_stale", OperationToken: "op_old"}
	if _, err := service.CompleteOutlineOperation(
		context.Background(), taskOwner(task), task.TaskID, oldClaim, nil, Outline{}); !errors.Is(err, ErrOperationTokenMismatch) {
		t.Fatalf("old-token CompleteOutlineOperation() error = %v, want ErrOperationTokenMismatch", err)
	}
	if _, err := service.FailOperation(context.Background(), taskOwner(task), task.TaskID, oldClaim, "PPT_AGENT_PROVIDER_UNAVAILABLE"); !errors.Is(err, ErrOperationTokenMismatch) {
		t.Fatalf("old-token FailOperation() error = %v, want ErrOperationTokenMismatch", err)
	}
}

func TestBeginOperationRejectsInvalidStage(t *testing.T) {
	db, _ := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task := NormalizeTask(Task{
		TaskID: "ppt_ready", SessionID: "ppt_ready", UserID: "user_ready",
		Stage: StageReady, Status: StatusSuccess, SlideCount: 1,
		Slides: []Slide{{ID: "slide_1", Page: 1}}, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	persistPPTPostgresTestTask(t, db, task)
	if _, _, err := service.BeginOperation(context.Background(), taskOwner(task), task.TaskID, "message", "key", "hash"); !errors.Is(err, ErrInvalidStage) {
		t.Fatalf("BeginOperation() error = %v, want ErrInvalidStage", err)
	}
}

func TestFailOperationAllowsSameKeyRetryWithNewToken(t *testing.T) {
	db, _ := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task, err := service.CreateSession(context.Background(), SessionRequest{Owner: testOwner("user_retry"), Prompt: "Retry", SkillCode: "general", SlideCount: 1})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	first, _, err := service.BeginOperation(context.Background(), taskOwner(task), task.TaskID, "message", "retry-key", "retry-hash")
	if err != nil {
		t.Fatalf("BeginOperation() error = %v", err)
	}
	failed, err := service.FailOperation(context.Background(), taskOwner(task), task.TaskID, first, "PPT_AGENT_PROVIDER_UNAVAILABLE")
	if err != nil {
		t.Fatalf("FailOperation() error = %v", err)
	}
	if failed.ErrorCode != "PPT_AGENT_PROVIDER_UNAVAILABLE" || failed.IdempotencyRecords[0].State != idempotencyStateFailed {
		t.Fatalf("FailOperation() task = %#v", failed)
	}
	retry, retriedTask, err := service.BeginOperation(context.Background(), taskOwner(task), task.TaskID, "message", "retry-key", "retry-hash")
	if err != nil {
		t.Fatalf("retry BeginOperation() error = %v", err)
	}
	if retry.Replay || retry.OperationToken == "" || retry.OperationToken == first.OperationToken || retriedTask.ErrorCode != "" || retriedTask.IdempotencyRecords[0].State != idempotencyStateProcessing {
		t.Fatalf("retry claim/task = %#v %#v", retry, retriedTask)
	}
}

func TestIdempotencyRecordCapPreservesLiveConfirmAndCancel(t *testing.T) {
	records := make([]IdempotencyRecord, 0, 65)
	for index := 0; index < 63; index++ {
		records = append(records, IdempotencyRecord{Scope: "message", Key: fmt.Sprintf("old-%02d", index), RequestHash: "hash", State: idempotencyStateCompleted, CreatedAt: fmt.Sprintf("2026-08-01T00:00:%02dZ", index%60)})
	}
	records = append(records,
		IdempotencyRecord{Scope: idempotencyScopeConfirm, Key: "confirm-live", RequestHash: "hash", State: idempotencyStateProcessing, OperationToken: "confirm-token", CreatedAt: "2026-08-01T01:00:00Z"},
		IdempotencyRecord{Scope: idempotencyScopeCancel, Key: "cancel-live", RequestHash: "hash", State: idempotencyStateProcessing, OperationToken: "cancel-token", CreatedAt: "2026-08-01T01:00:01Z"},
		IdempotencyRecord{Scope: "message", Key: "new-key", RequestHash: "new-hash", State: idempotencyStateProcessing, OperationToken: "new-token", CreatedAt: "2026-08-01T01:00:02Z"},
	)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	updated := Task{
		TaskID: "ppt_cap", SessionID: "ppt_cap", UserID: "user_cap", Stage: StageDraft, Status: StatusPending,
		IdempotencyRecords: records, CreatedAt: now, UpdatedAt: now,
	}
	pruneIdempotencyRecords(&updated)
	if len(updated.IdempotencyRecords) != maxIdempotencyRecords {
		t.Fatalf("record count = %d, want %d", len(updated.IdempotencyRecords), maxIdempotencyRecords)
	}
	for _, key := range []string{"confirm-live", "cancel-live", "new-key"} {
		if !hasIdempotencyKey(updated.IdempotencyRecords, key) {
			t.Fatalf("protected/current idempotency key %q was pruned: %#v", key, updated.IdempotencyRecords)
		}
	}
}

func TestGenerationLeaseProgressReclaimAndCompletion(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task := createOutlineReadySession(t, service, "user_generation", 2)
	generating, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_1", "confirm-key", "confirm-hash")
	if err != nil {
		t.Fatalf("BeginGeneration() error = %v", err)
	}
	if generating.Stage != StageGenerating || generating.BillingTaskID != "billing_1" || generating.Progress != 0 {
		t.Fatalf("BeginGeneration() task = %#v", generating)
	}
	replayed, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_1", "confirm-key", "confirm-hash")
	if err != nil || replayed.Stage != StageGenerating || len(replayed.IdempotencyRecords) != len(generating.IdempotencyRecords) {
		t.Fatalf("BeginGeneration() replay task=%#v err=%v", replayed, err)
	}
	if _, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_1", "confirm-key", "different"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("BeginGeneration() conflict = %v", err)
	}

	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	claim, leased, err := service.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, now)
	if err != nil || claim.RunToken == "" || leased.GenerationLease == nil || leased.GenerationLease.RunToken != claim.RunToken {
		t.Fatalf("ClaimGenerationRun() claim=%#v task=%#v err=%v", claim, leased, err)
	}
	if _, _, err := service.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, now.Add(time.Second)); !errors.Is(err, ErrGenerationAlreadyRunning) {
		t.Fatalf("active lease error = %v, want ErrGenerationAlreadyRunning", err)
	}
	reclaimed, _, err := service.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, now.Add(generationLeaseDuration+time.Second))
	if err != nil || reclaimed.RunToken == "" || reclaimed.RunToken == claim.RunToken {
		t.Fatalf("reclaimed claim=%#v err=%v", reclaimed, err)
	}
	if _, err := service.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, claim, Slide{ID: "slide_1", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "Cover"}}}); !errors.Is(err, ErrGenerationRunMismatch) {
		t.Fatalf("stale run write error = %v, want ErrGenerationRunMismatch", err)
	}
	beforeLegacyWrite, _ := state.snapshot(task.TaskID)
	if _, err := service.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, reclaimed, Slide{ID: "slide_1", Page: 1, Title: "legacy-only"}); !errors.Is(err, ErrInvalidSlideIR) {
		t.Fatalf("legacy-only slide write error = %v, want ErrInvalidSlideIR", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, beforeLegacyWrite)
	one, err := service.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, reclaimed, Slide{ID: "slide_1", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "Cover"}}})
	if err != nil || one.CurrentPage != 1 || one.Progress != 50 {
		t.Fatalf("first slide task=%#v err=%v", one, err)
	}
	duplicate, err := service.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, reclaimed, Slide{ID: "slide_1", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "Changed duplicate"}}})
	if err != nil || len(duplicate.Slides) != 1 || duplicate.Progress != 50 || slideTitle(duplicate.Slides[0]) != "Cover" {
		t.Fatalf("duplicate slide task=%#v err=%v", duplicate, err)
	}
	if _, err := service.CompleteGeneration(context.Background(), taskOwner(task), task.TaskID, reclaimed); !errors.Is(err, ErrGenerationIncomplete) {
		t.Fatalf("incomplete completion error = %v, want ErrGenerationIncomplete", err)
	}
	two, err := service.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, reclaimed, Slide{ID: "slide_2", Page: 2, Blocks: []SlideBlock{{Type: "title", Text: "Results"}}})
	if err != nil || two.CurrentPage != 2 || two.Progress != 100 {
		t.Fatalf("second slide task=%#v err=%v", two, err)
	}
	ready, err := service.CompleteGeneration(context.Background(), taskOwner(task), task.TaskID, reclaimed)
	if err != nil || ready.Stage != StageReady || ready.Status != StatusSuccess || ready.Progress != 100 || ready.GenerationLease != nil {
		t.Fatalf("CompleteGeneration() task=%#v err=%v", ready, err)
	}
	beforeCompletedMismatch, _ := state.snapshot(task.TaskID)
	if _, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_other", "confirm-key", "confirm-hash"); !errors.Is(err, ErrBillingBindingMismatch) {
		t.Fatalf("completed confirm billing mismatch error = %v, want ErrBillingBindingMismatch", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, beforeCompletedMismatch)
	if _, err := service.updatePostgresTask(taskOwner(task), task.TaskID, func(current *Task) error {
		current.Title = "later mutation"
		return nil
	}); err != nil {
		t.Fatalf("mutate completed task for replay proof: %v", err)
	}
	completedReplay, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_1", "confirm-key", "confirm-hash")
	if err != nil || completedReplay.Title != ready.Title || completedReplay.GenerationLease != nil {
		t.Fatalf("completed confirm replay task=%#v err=%v, want original snapshot without lease", completedReplay, err)
	}
	if _, _, err := service.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, now.Add(time.Hour)); !errors.Is(err, ErrInvalidStage) {
		t.Fatalf("READY reclaim error = %v, want ErrInvalidStage", err)
	}
}

func TestGenerationLeaseRenewalPreservesLongOwnerLease(t *testing.T) {
	db, _ := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task := createOutlineReadySession(t, service, "user_generation_renew", 1)
	if _, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_renew", "confirm-renew", "confirm-renew-hash"); err != nil {
		t.Fatalf("BeginGeneration() error = %v", err)
	}
	now := time.Now().UTC()
	claim, _, err := service.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, now)
	if err != nil {
		t.Fatalf("ClaimGenerationRun() error = %v", err)
	}
	renewedClaim, renewed, err := service.RenewGenerationRun(context.Background(), taskOwner(task), task.TaskID, claim, now.Add(time.Second), 10*time.Minute)
	if err != nil {
		t.Fatalf("RenewGenerationRun() error = %v", err)
	}
	if renewedClaim.RunToken != claim.RunToken || renewed.GenerationLease == nil || renewed.GenerationLease.RunToken != claim.RunToken {
		t.Fatalf("renewed claim=%#v task lease=%#v", renewedClaim, renewed.GenerationLease)
	}
	wantUntil := now.Add(time.Second).Add(10 * time.Minute)
	gotUntil, parseErr := time.Parse(time.RFC3339Nano, renewed.GenerationLease.LeaseUntil)
	if parseErr != nil || !gotUntil.Equal(wantUntil) {
		t.Fatalf("renewed lease until=%q parsed=%s err=%v, want %s", renewed.GenerationLease.LeaseUntil, gotUntil, parseErr, wantUntil)
	}
	afterSlide, err := service.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, renewedClaim, Slide{ID: "slide_1", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "Cover"}}})
	if err != nil {
		t.Fatalf("PersistGeneratedSlide() error = %v", err)
	}
	afterSlideUntil, parseErr := time.Parse(time.RFC3339Nano, afterSlide.GenerationLease.LeaseUntil)
	if parseErr != nil || afterSlideUntil.Before(gotUntil) {
		t.Fatalf("slide persistence shortened long lease from %s to %q (err=%v)", gotUntil, afterSlide.GenerationLease.LeaseUntil, parseErr)
	}
	if _, _, err := service.RenewGenerationRun(context.Background(), taskOwner(task), task.TaskID, GenerationClaim{RunToken: "wrong-run"}, now.Add(2*time.Second), time.Minute); !errors.Is(err, ErrGenerationRunMismatch) {
		t.Fatalf("mismatched renewal error=%v, want ErrGenerationRunMismatch", err)
	}
}

func TestGenerationCleanupFenceReclaimsExpiredMatchingLeaseAndBlocksTakeover(t *testing.T) {
	db, _ := newPPTPostgresTestDB(t)
	workerA := NewPostgresService(db)
	workerB := NewPostgresService(db)
	task := createOutlineReadySession(t, workerA, "user_cleanup_fence_a_wins", 1)
	if _, err := claimAndBindGenerationForTest(context.Background(), workerA, taskOwner(task), task.TaskID, "billing_cleanup_fence", "confirm-cleanup-fence", "confirm-hash"); err != nil {
		t.Fatalf("BeginGeneration() error = %v", err)
	}
	startedAt := time.Date(2026, time.August, 6, 12, 0, 0, 0, time.UTC)
	runA, _, err := workerA.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, startedAt)
	if err != nil {
		t.Fatalf("ClaimGenerationRun(A) error = %v", err)
	}
	fenceAt := startedAt.Add(generationLeaseDuration + time.Second)
	fenced, fencedTask, err := workerA.AcquireGenerationCleanupFence(context.Background(), taskOwner(task), task.TaskID, runA, fenceAt)
	if err != nil {
		t.Fatalf("AcquireGenerationCleanupFence(A) error = %v", err)
	}
	wantUntil := fenceAt.Add(generationLeaseDuration)
	gotUntil, parseErr := time.Parse(time.RFC3339Nano, fenced.LeaseUntil)
	if parseErr != nil || fenced.RunToken != runA.RunToken || !gotUntil.Equal(wantUntil) || fencedTask.GenerationLease == nil || fencedTask.GenerationLease.RunToken != runA.RunToken {
		t.Fatalf("cleanup fence claim=%#v task lease=%#v wantUntil=%s parseErr=%v", fenced, fencedTask.GenerationLease, wantUntil, parseErr)
	}
	if runB, _, claimErr := workerB.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, fenceAt.Add(time.Second)); !errors.Is(claimErr, ErrGenerationAlreadyRunning) || runB.RunToken != "" {
		t.Fatalf("ClaimGenerationRun(B) during cleanup fence = %#v err=%v, want ErrGenerationAlreadyRunning", runB, claimErr)
	}
	failed, err := workerA.FailGenerationAfterRelease(context.Background(), taskOwner(task), task.TaskID, fenced, "PPT_GENERATION_FAILED")
	if err != nil {
		t.Fatalf("FailGenerationAfterRelease(A) error = %v", err)
	}
	if failed.Stage != StageFailed || failed.GenerationLease != nil || failed.ErrorCode != "PPT_GENERATION_FAILED" {
		t.Fatalf("cleanup-fenced failure task = %#v", failed)
	}
}

func TestGenerationCleanupFenceRejectsReplacedRunWithoutWrite(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	workerA := NewPostgresService(db)
	workerB := NewPostgresService(db)
	task := createOutlineReadySession(t, workerA, "user_cleanup_fence_b_wins", 1)
	if _, err := claimAndBindGenerationForTest(context.Background(), workerA, taskOwner(task), task.TaskID, "billing_cleanup_b_wins", "confirm-cleanup-b-wins", "confirm-hash"); err != nil {
		t.Fatalf("BeginGeneration() error = %v", err)
	}
	startedAt := time.Date(2026, time.August, 6, 13, 0, 0, 0, time.UTC)
	runA, _, err := workerA.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, startedAt)
	if err != nil {
		t.Fatalf("ClaimGenerationRun(A) error = %v", err)
	}
	takeoverAt := startedAt.Add(generationLeaseDuration + time.Second)
	runB, _, err := workerB.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, takeoverAt)
	if err != nil {
		t.Fatalf("ClaimGenerationRun(B) error = %v", err)
	}
	before, ok := state.snapshot(task.TaskID)
	if !ok {
		t.Fatalf("task %q was not persisted", task.TaskID)
	}
	writesBefore := state.strictUpsertCount()
	if _, _, err := workerA.AcquireGenerationCleanupFence(context.Background(), taskOwner(task), task.TaskID, runA, takeoverAt.Add(time.Second)); !errors.Is(err, ErrGenerationRunMismatch) {
		t.Fatalf("AcquireGenerationCleanupFence(stale A) error = %v, want ErrGenerationRunMismatch", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, before)
	if writesAfter := state.strictUpsertCount(); writesAfter != writesBefore {
		t.Fatalf("stale cleanup fence caused PostgreSQL write: before=%d after=%d", writesBefore, writesAfter)
	}
	if _, err := workerB.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, runB, Slide{ID: "slide_1", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "Successor"}}}); err != nil {
		t.Fatalf("PersistGeneratedSlide(B) error = %v", err)
	}
	ready, err := workerB.CompleteGenerationAfterCapture(context.Background(), taskOwner(task), task.TaskID, runB)
	if err != nil || ready.Stage != StageReady {
		t.Fatalf("CompleteGenerationAfterCapture(B) task=%#v err=%v", ready, err)
	}
}

func TestGenerationCleanupFenceAllowsLiveCancellationToSettle(t *testing.T) {
	db, _ := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task := createOutlineReadySession(t, service, "user_cleanup_fence_cancel", 1)
	if _, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_cleanup_cancel", "confirm-cleanup-cancel", "confirm-hash"); err != nil {
		t.Fatalf("BeginGeneration() error = %v", err)
	}
	startedAt := time.Date(2026, time.August, 6, 14, 0, 0, 0, time.UTC)
	run, _, err := service.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, startedAt)
	if err != nil {
		t.Fatalf("ClaimGenerationRun() error = %v", err)
	}
	cancelClaim, _, err := service.BeginCancel(context.Background(), taskOwner(task), task.TaskID, "cancel-cleanup-fence", "cancel-hash")
	if err != nil {
		t.Fatalf("BeginCancel() error = %v", err)
	}
	if _, _, err := service.AcquireGenerationCleanupFence(context.Background(), taskOwner(task), task.TaskID, run, startedAt.Add(generationLeaseDuration+time.Second)); err != nil {
		t.Fatalf("AcquireGenerationCleanupFence(live cancel) error = %v", err)
	}
	cancelled, err := service.CompleteCancel(context.Background(), taskOwner(task), task.TaskID, cancelClaim)
	if err != nil || cancelled.Stage != StageCancelled {
		t.Fatalf("CompleteCancel() task=%#v err=%v", cancelled, err)
	}
}

func TestGenerationRequiresExactValidPageCoverage(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task := createOutlineReadySession(t, service, "user_page_coverage", 3)
	if _, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_pages", "confirm-pages", "confirm-hash"); err != nil {
		t.Fatalf("BeginGeneration() error = %v", err)
	}
	claim, _, err := service.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimGenerationRun() error = %v", err)
	}
	pageTwo, err := service.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, claim, Slide{ID: "slide_2", Page: 2, Blocks: []SlideBlock{{Type: "title", Text: "Second"}}})
	if err != nil || pageTwo.CurrentPage != 1 || pageTwo.Progress != 33 {
		t.Fatalf("out-of-order page 2 task=%#v err=%v", pageTwo, err)
	}

	invalidCoordinates := []struct {
		name  string
		slide Slide
	}{
		{name: "page above total", slide: Slide{ID: "slide_99", Page: 99, Blocks: []SlideBlock{{Type: "title", Text: "Outside"}}}},
		{name: "zero page after out of order write", slide: Slide{ID: "slide_0", Page: 0, Blocks: []SlideBlock{{Type: "title", Text: "Zero"}}}},
		{name: "missing id", slide: Slide{Page: 1, Title: "Blank ID"}},
	}
	for _, test := range invalidCoordinates {
		t.Run(test.name, func(t *testing.T) {
			before, _ := state.snapshot(task.TaskID)
			if _, err := service.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, claim, test.slide); !errors.Is(err, ErrInvalidSlideCoordinate) {
				t.Fatalf("PersistGeneratedSlide() error = %v, want ErrInvalidSlideCoordinate", err)
			}
			assertPPTPostgresRowUnchanged(t, state, task.TaskID, before)
		})
	}

	pageThree, err := service.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, claim, Slide{ID: "slide_3", Page: 3, Blocks: []SlideBlock{{Type: "title", Text: "Third"}}})
	if err != nil || pageThree.CurrentPage != 2 || pageThree.Progress != 66 {
		t.Fatalf("out-of-order page 3 task=%#v err=%v", pageThree, err)
	}
	collisions := []Slide{
		{ID: "slide_2", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "ID collision"}}},
		{ID: "different_id", Page: 2, Blocks: []SlideBlock{{Type: "title", Text: "Page collision"}}},
	}
	for _, collision := range collisions {
		before, _ := state.snapshot(task.TaskID)
		if _, err := service.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, claim, collision); !errors.Is(err, ErrSlideCoordinateConflict) {
			t.Fatalf("collision %#v error = %v, want ErrSlideCoordinateConflict", collision, err)
		}
		assertPPTPostgresRowUnchanged(t, state, task.TaskID, before)
	}

	beforeIncomplete, _ := state.snapshot(task.TaskID)
	if _, err := service.CompleteGeneration(context.Background(), taskOwner(task), task.TaskID, claim); !errors.Is(err, ErrGenerationIncomplete) {
		t.Fatalf("missing page completion error = %v, want ErrGenerationIncomplete", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, beforeIncomplete)
	pageOne, err := service.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, claim, Slide{ID: "slide_1", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "First"}}})
	if err != nil || pageOne.CurrentPage != 3 || pageOne.Progress != 100 || len(pageOne.Slides) != 3 || pageOne.Slides[0].Page != 1 || pageOne.Slides[1].Page != 2 || pageOne.Slides[2].Page != 3 {
		t.Fatalf("exact out-of-order coverage task=%#v err=%v", pageOne, err)
	}
	ready, err := service.CompleteGeneration(context.Background(), taskOwner(task), task.TaskID, claim)
	if err != nil || ready.Stage != StageReady {
		t.Fatalf("exact coverage completion task=%#v err=%v", ready, err)
	}
}

func TestBindGenerationBillingRequiresExactClaimAndStableBinding(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task := createOutlineReadySession(t, service, "user_billing_binding", 1)
	claim, _, err := service.BeginGenerationClaim(context.Background(), taskOwner(task), task.TaskID, "confirm-billing", "confirm-hash")
	if err != nil {
		t.Fatal(err)
	}
	beforeMissing, _ := state.snapshot(task.TaskID)
	if _, err := service.BindGenerationBilling(context.Background(), taskOwner(task), task.TaskID, claim, ""); !errors.Is(err, ErrBillingTaskRequired) {
		t.Fatalf("empty billing error = %v, want ErrBillingTaskRequired", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, beforeMissing)
	wrong := claim
	wrong.OperationToken = "wrong-token"
	if _, err := service.BindGenerationBilling(context.Background(), taskOwner(task), task.TaskID, wrong, "billing_original"); !errors.Is(err, ErrOperationTokenMismatch) {
		t.Fatalf("wrong claim error = %v, want ErrOperationTokenMismatch", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, beforeMissing)
	if _, err := service.BindGenerationBilling(context.Background(), taskOwner(task), task.TaskID, claim, "billing_original"); err != nil {
		t.Fatalf("BindGenerationBilling() error = %v", err)
	}
	bound, _ := state.snapshot(task.TaskID)
	if _, err := service.BindGenerationBilling(context.Background(), taskOwner(task), task.TaskID, claim, "billing_original"); err != nil {
		t.Fatalf("idempotent bind error = %v", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, bound)
	beforeMismatch, _ := state.snapshot(task.TaskID)
	if _, err := service.BindGenerationBilling(context.Background(), taskOwner(task), task.TaskID, claim, "billing_other"); !errors.Is(err, ErrBillingBindingMismatch) {
		t.Fatalf("billing mismatch error = %v, want ErrBillingBindingMismatch", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, beforeMismatch)
	if _, err := service.updatePostgresTask(taskOwner(task), task.TaskID, func(current *Task) error {
		current.BillingTaskID = ""
		return nil
	}); err != nil {
		t.Fatalf("clear binding fixture: %v", err)
	}
	beforeMissingStored, _ := state.snapshot(task.TaskID)
	if _, err := service.BindGenerationBilling(context.Background(), taskOwner(task), task.TaskID, claim, "billing_original"); !errors.Is(err, ErrBillingBindingMissing) {
		t.Fatalf("missing stored billing error = %v, want ErrBillingBindingMissing", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, beforeMissingStored)
}

func TestGenerationReservationClaimBlocksOutlineReadyCancelUntilBillingBind(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	confirmer := NewPostgresService(db)
	canceller := NewPostgresService(db)
	task := createOutlineReadySession(t, confirmer, "user_prebind_claim", 1)

	claim, claimed, err := confirmer.BeginGenerationClaim(context.Background(), taskOwner(task), task.TaskID, "confirm-prebind", "confirm-hash")
	if err != nil || claim.OperationToken == "" || claimed.Stage != StageOutlineReady || claimed.BillingTaskID != "" {
		t.Fatalf("BeginGenerationClaim() claim=%#v task=%#v err=%v", claim, claimed, err)
	}
	beforeCancel, _ := state.snapshot(task.TaskID)
	if _, _, err := canceller.BeginCancel(context.Background(), taskOwner(task), task.TaskID, "cancel-during-prebind", "cancel-hash"); !errors.Is(err, ErrOperationInProgress) {
		t.Fatalf("cancel during pre-bind claim error = %v, want ErrOperationInProgress", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, beforeCancel)
	if _, _, err := canceller.BeginGenerationClaim(context.Background(), taskOwner(task), task.TaskID, "other-confirm", "other-hash"); !errors.Is(err, ErrOperationInProgress) {
		t.Fatalf("different confirm during pre-bind claim error = %v, want ErrOperationInProgress", err)
	}

	bound, err := claimAndBindGenerationForTest(context.Background(), confirmer, taskOwner(task), task.TaskID, "billing-prebind", claim.Key, claim.RequestHash)
	if err != nil || bound.Stage != StageGenerating || bound.BillingTaskID != "billing-prebind" {
		t.Fatalf("BeginGeneration() task=%#v err=%v", bound, err)
	}
	replayClaim, replayed, err := confirmer.BeginGenerationClaim(context.Background(), taskOwner(task), task.TaskID, claim.Key, claim.RequestHash)
	if err != nil || !replayClaim.Replay || replayClaim.OperationToken != claim.OperationToken || replayed.Stage != StageGenerating || replayed.BillingTaskID != "billing-prebind" {
		t.Fatalf("claim replay=%#v task=%#v err=%v", replayClaim, replayed, err)
	}
}

func TestFailedGenerationReservationClaimCanRetryAndThenCancel(t *testing.T) {
	db, _ := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task := createOutlineReadySession(t, service, "user_prebind_retry", 1)

	first, _, err := service.BeginGenerationClaim(context.Background(), taskOwner(task), task.TaskID, "confirm-retry", "confirm-hash")
	if err != nil {
		t.Fatalf("first BeginGenerationClaim() error = %v", err)
	}
	failed, err := service.FailGenerationClaim(context.Background(), taskOwner(task), task.TaskID, first, "PPT_BILLING_RESERVATION_FAILED")
	if err != nil {
		t.Fatalf("FailGenerationClaim() error = %v", err)
	}
	record, ok := idempotencyRecordByScope(failed.IdempotencyRecords, idempotencyScopeConfirm)
	if !ok || record.State != idempotencyStateFailed || record.ErrorCode != "PPT_BILLING_RESERVATION_FAILED" || failed.Stage != StageOutlineReady {
		t.Fatalf("failed claim record=%#v found=%v task=%#v", record, ok, failed)
	}

	retried, _, err := service.BeginGenerationClaim(context.Background(), taskOwner(task), task.TaskID, first.Key, first.RequestHash)
	if err != nil || retried.OperationToken == "" || retried.OperationToken == first.OperationToken {
		t.Fatalf("retried claim=%#v err=%v", retried, err)
	}
	if _, err := service.FailGenerationClaim(context.Background(), taskOwner(task), task.TaskID, retried, "PPT_BILLING_RESERVATION_FAILED"); err != nil {
		t.Fatalf("second FailGenerationClaim() error = %v", err)
	}
	cancelClaim, _, err := service.BeginCancel(context.Background(), taskOwner(task), task.TaskID, "cancel-after-failed-reserve", "cancel-hash")
	if err != nil {
		t.Fatalf("BeginCancel() after failed claim error = %v", err)
	}
	cancelled, err := service.CompleteCancel(context.Background(), taskOwner(task), task.TaskID, cancelClaim)
	if err != nil || cancelled.Stage != StageCancelled {
		t.Fatalf("CompleteCancel() task=%#v err=%v", cancelled, err)
	}
}

func TestGenerationClaimReplayRefreshesLeaseAndBlocksStaleCancelTransfer(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task := createOutlineReadySession(t, service, "user_prebind_heartbeat", 1)
	claim, _, err := service.BeginGenerationClaim(context.Background(), taskOwner(task), task.TaskID, "confirm-heartbeat", "confirm-hash")
	if err != nil {
		t.Fatalf("BeginGenerationClaim() error = %v", err)
	}
	if _, err := service.BindGenerationBilling(context.Background(), taskOwner(task), task.TaskID, claim, "billing-heartbeat"); err != nil {
		t.Fatalf("BindGenerationBilling() error = %v", err)
	}
	staleAt := time.Now().UTC().Add(-operationProcessingStaleAfter - time.Minute).Format(time.RFC3339Nano)
	if _, err := service.updatePostgresTask(taskOwner(task), task.TaskID, func(current *Task) error {
		index := findIdempotencyRecord(current.IdempotencyRecords, idempotencyScopeConfirm, claim.Key)
		current.IdempotencyRecords[index].UpdatedAt = staleAt
		return nil
	}); err != nil {
		t.Fatalf("age confirm claim: %v", err)
	}
	replayed, refreshed, err := service.BeginGenerationClaim(context.Background(), taskOwner(task), task.TaskID, claim.Key, claim.RequestHash)
	if err != nil || !replayed.Replay {
		t.Fatalf("claim replay=%#v task=%#v err=%v", replayed, refreshed, err)
	}
	record, ok := idempotencyRecordByScope(refreshed.IdempotencyRecords, idempotencyScopeConfirm)
	if !ok || record.UpdatedAt == staleAt {
		t.Fatalf("refreshed confirm record=%#v found=%v", record, ok)
	}
	before, _ := state.snapshot(task.TaskID)
	if _, _, err := service.BeginCancelAfterStaleGenerationClaim(
		context.Background(), taskOwner(task), task.TaskID, replayed, "cancel-heartbeat", "cancel-hash", time.Now().UTC()); !errors.Is(err, ErrOperationInProgress) {
		t.Fatalf("fresh replay cancel transfer error = %v, want ErrOperationInProgress", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, before)
}

func TestStaleGenerationClaimUsesOnlyFencedBillingBindingForRetryableCancel(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task := createOutlineReadySession(t, service, "user_prebind_stale_cancel", 1)
	claim, _, err := service.BeginGenerationClaim(context.Background(), taskOwner(task), task.TaskID, "confirm-stale-cancel", "confirm-hash")
	if err != nil {
		t.Fatalf("BeginGenerationClaim() error = %v", err)
	}
	bound, err := service.BindGenerationBilling(context.Background(), taskOwner(task), task.TaskID, claim, "billing-verified")
	if err != nil || bound.Stage != StageGenerating || bound.BillingTaskID != "billing-verified" {
		t.Fatalf("BindGenerationBilling() task=%#v err=%v", bound, err)
	}
	now := time.Now().UTC()
	if _, err := service.updatePostgresTask(taskOwner(task), task.TaskID, func(current *Task) error {
		index := findIdempotencyRecord(current.IdempotencyRecords, idempotencyScopeConfirm, claim.Key)
		current.IdempotencyRecords[index].UpdatedAt = now.Add(-operationProcessingStaleAfter - time.Minute).Format(time.RFC3339Nano)
		return nil
	}); err != nil {
		t.Fatalf("age confirm claim: %v", err)
	}
	beforeNegative, _ := state.snapshot(task.TaskID)
	wrongClaim := claim
	wrongClaim.OperationToken = "wrong-token"
	if _, _, err := service.BeginCancelAfterStaleGenerationClaim(
		context.Background(), taskOwner(task), task.TaskID, wrongClaim, "cancel-wrong-proof", "cancel-hash", now); !errors.Is(err, ErrOperationTokenMismatch) {
		t.Fatalf("wrong-proof stale cancel error = %v, want ErrOperationTokenMismatch", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, beforeNegative)
	if _, _, err := service.BeginCancelAfterStaleGenerationClaim(
		context.Background(), OwnerScope{TenantID: "other-tenant", UserID: task.UserID}, task.TaskID, claim, "cancel-cross-account", "cancel-hash", now); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("cross-tenant stale cancel error = %v, want ErrTaskNotFound", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, beforeNegative)
	cancelClaim, recovered, err := service.BeginCancelAfterStaleGenerationClaim(
		context.Background(), taskOwner(task), task.TaskID, claim, "cancel-stale", "cancel-hash", now)
	if err != nil || cancelClaim.OperationToken == "" || recovered.Stage != StageGenerating || recovered.BillingTaskID != "billing-verified" {
		t.Fatalf("BeginCancelAfterStaleGenerationClaim() claim=%#v task=%#v err=%v", cancelClaim, recovered, err)
	}
	confirmRecord, ok := idempotencyRecordByScope(recovered.IdempotencyRecords, idempotencyScopeConfirm)
	if !ok || confirmRecord.State != idempotencyStateFailed || confirmRecord.ErrorCode != ErrSessionCancelled.Error() {
		t.Fatalf("closed confirm record=%#v found=%v", confirmRecord, ok)
	}
	cancelRecord, ok := idempotencyRecordByScope(recovered.IdempotencyRecords, idempotencyScopeCancel)
	if !ok || cancelRecord.State != idempotencyStateProcessing {
		t.Fatalf("cancel record=%#v found=%v", cancelRecord, ok)
	}
}

func TestStaleGenerationClaimWithoutBillingBindingFailsClosedWithoutWrite(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task := createOutlineReadySession(t, service, "user_prebind_missing", 1)
	claim, _, err := service.BeginGenerationClaim(context.Background(), taskOwner(task), task.TaskID, "confirm-missing", "confirm-hash")
	if err != nil {
		t.Fatalf("BeginGenerationClaim() error = %v", err)
	}
	now := time.Now().UTC()
	if _, err := service.updatePostgresTask(taskOwner(task), task.TaskID, func(current *Task) error {
		index := findIdempotencyRecord(current.IdempotencyRecords, idempotencyScopeConfirm, claim.Key)
		current.IdempotencyRecords[index].UpdatedAt = now.Add(-operationProcessingStaleAfter - time.Minute).Format(time.RFC3339Nano)
		return nil
	}); err != nil {
		t.Fatalf("age confirm claim: %v", err)
	}
	before, _ := state.snapshot(task.TaskID)
	if _, _, err := service.BeginCancelAfterStaleGenerationClaim(
		context.Background(), taskOwner(task), task.TaskID, claim, "cancel-missing", "cancel-hash", now); !errors.Is(err, ErrBillingBindingMissing) {
		t.Fatalf("missing-binding stale cancel error = %v, want ErrBillingBindingMissing", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, before)
}

func TestCancelClaimPreventsReadyWriteAndReplays(t *testing.T) {
	db, _ := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task := createOutlineReadySession(t, service, "user_cancel", 1)
	if _, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_cancel", "confirm", "confirm-hash"); err != nil {
		t.Fatalf("BeginGeneration() error = %v", err)
	}
	claim, _, err := service.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimGenerationRun() error = %v", err)
	}
	if _, err := service.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, claim, Slide{ID: "slide_1", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "Only"}}}); err != nil {
		t.Fatalf("PersistGeneratedSlide() error = %v", err)
	}
	cancelClaim, cancelling, err := service.BeginCancel(context.Background(), taskOwner(task), task.TaskID, "cancel-key", "cancel-hash")
	if err != nil || cancelClaim.OperationToken == "" || cancelling.Stage != StageGenerating {
		t.Fatalf("BeginCancel() claim=%#v task=%#v err=%v", cancelClaim, cancelling, err)
	}
	if _, err := service.CompleteGeneration(context.Background(), taskOwner(task), task.TaskID, claim); !errors.Is(err, ErrSessionCancelled) {
		t.Fatalf("READY write after cancel claim error = %v, want ErrSessionCancelled", err)
	}
	cancelled, err := service.CompleteCancel(context.Background(), taskOwner(task), task.TaskID, cancelClaim)
	if err != nil || cancelled.Stage != StageCancelled || cancelled.Status != StatusCancelled || cancelled.Progress != 100 {
		t.Fatalf("CompleteCancel() task=%#v err=%v", cancelled, err)
	}
	if _, err := service.CompleteGeneration(context.Background(), taskOwner(task), task.TaskID, claim); !errors.Is(err, ErrSessionCancelled) {
		t.Fatalf("CANCELLED READY write error = %v, want ErrSessionCancelled", err)
	}
	replay, replayTask, err := service.BeginCancel(context.Background(), taskOwner(task), task.TaskID, "cancel-key", "cancel-hash")
	if err != nil || !replay.Replay || replayTask.Stage != StageCancelled {
		t.Fatalf("cancel replay claim=%#v task=%#v err=%v", replay, replayTask, err)
	}
}

func TestCompleteCancelClosesConfirmAndRejectsOriginalReplay(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task := createOutlineReadySession(t, service, "user_cancel_confirm", 1)
	if _, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_cancel_confirm", "confirm-original", "confirm-hash"); err != nil {
		t.Fatalf("BeginGeneration() error = %v", err)
	}
	cancelClaim, _, err := service.BeginCancel(context.Background(), taskOwner(task), task.TaskID, "cancel-original", "cancel-hash")
	if err != nil {
		t.Fatalf("BeginCancel() error = %v", err)
	}
	cancelled, err := service.CompleteCancel(context.Background(), taskOwner(task), task.TaskID, cancelClaim)
	if err != nil {
		t.Fatalf("CompleteCancel() error = %v", err)
	}
	confirmRecord, ok := idempotencyRecordByScope(cancelled.IdempotencyRecords, idempotencyScopeConfirm)
	if !ok || confirmRecord.State != idempotencyStateFailed || confirmRecord.ErrorCode != ErrSessionCancelled.Error() {
		t.Fatalf("closed confirm record = %#v, found=%v", confirmRecord, ok)
	}
	cancelRecord, ok := idempotencyRecordByScope(cancelled.IdempotencyRecords, idempotencyScopeCancel)
	if !ok || cancelRecord.State != idempotencyStateCompleted {
		t.Fatalf("completed cancel record = %#v, found=%v", cancelRecord, ok)
	}
	beforeReplay, _ := state.snapshot(task.TaskID)
	if _, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_cancel_confirm", "confirm-original", "confirm-hash"); !errors.Is(err, ErrSessionCancelled) {
		t.Fatalf("cancelled stale confirm replay error = %v, want ErrSessionCancelled", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, beforeReplay)
	if _, _, err := service.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, time.Now().UTC().Add(time.Hour)); !errors.Is(err, ErrSessionCancelled) {
		t.Fatalf("cancelled new run error = %v, want ErrSessionCancelled", err)
	}
}

func TestTerminalProcessingOrFailedConfirmCannotStartNewRun(t *testing.T) {
	tests := []struct {
		name      string
		terminate func(*testing.T, *Service, Task, GenerationClaim) Task
	}{
		{
			name: "ready with processing confirm",
			terminate: func(t *testing.T, service *Service, task Task, claim GenerationClaim) Task {
				t.Helper()
				if _, err := service.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, claim, Slide{ID: "slide_1", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "Ready"}}}); err != nil {
					t.Fatalf("PersistGeneratedSlide() error = %v", err)
				}
				_, err := service.CompleteGeneration(context.Background(), taskOwner(task), task.TaskID, claim)
				if err != nil {
					t.Fatalf("CompleteGeneration() error = %v", err)
				}
				mutated, err := service.updatePostgresTask(taskOwner(task), task.TaskID, func(current *Task) error {
					for index := range current.IdempotencyRecords {
						if current.IdempotencyRecords[index].Scope == idempotencyScopeConfirm {
							current.IdempotencyRecords[index].State = idempotencyStateProcessing
							current.IdempotencyRecords[index].ResponseJSON = ""
						}
					}
					return nil
				})
				if err != nil {
					t.Fatalf("create terminal processing fixture: %v", err)
				}
				return mutated
			},
		},
		{
			name: "failed",
			terminate: func(t *testing.T, service *Service, task Task, claim GenerationClaim) Task {
				t.Helper()
				got, err := service.FailGenerationAfterRelease(context.Background(), taskOwner(task), task.TaskID, claim, "PPT_AGENT_PROVIDER_UNAVAILABLE")
				if err != nil {
					t.Fatalf("FailGenerationAfterRelease() error = %v", err)
				}
				return got
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, state := newPPTPostgresTestDB(t)
			service := NewPostgresService(db)
			task := createOutlineReadySession(t, service, "user_terminal_confirm_"+test.name, 1)
			if _, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_terminal", "confirm-terminal", "confirm-hash"); err != nil {
				t.Fatalf("BeginGeneration() error = %v", err)
			}
			claim, _, err := service.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, time.Now().UTC())
			if err != nil {
				t.Fatalf("ClaimGenerationRun() error = %v", err)
			}
			terminal := test.terminate(t, service, task, claim)
			beforeReplay, _ := state.snapshot(task.TaskID)
			if _, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_terminal", "confirm-terminal", "confirm-hash"); !errors.Is(err, ErrInvalidStage) {
				t.Fatalf("%s stale confirm replay error = %v, want ErrInvalidStage", terminal.Stage, err)
			}
			assertPPTPostgresRowUnchanged(t, state, task.TaskID, beforeReplay)
			if _, _, err := service.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, time.Now().UTC().Add(time.Hour)); !errors.Is(err, ErrInvalidStage) {
				t.Fatalf("%s new run error = %v, want ErrInvalidStage", terminal.Stage, err)
			}
		})
	}
}

func TestLiveCancelClaimBlocksNewOperationsAndGeneration(t *testing.T) {
	db, _ := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task := createOutlineReadySession(t, service, "user_cancel_blocks", 1)
	if _, _, err := service.BeginCancel(context.Background(), taskOwner(task), task.TaskID, "cancel-block", "cancel-hash"); err != nil {
		t.Fatalf("BeginCancel() error = %v", err)
	}
	if _, _, err := service.BeginOperation(context.Background(), taskOwner(task), task.TaskID, "message", "message-after-cancel", "message-hash"); !errors.Is(err, ErrSessionCancelled) {
		t.Fatalf("BeginOperation() after cancel claim error = %v, want ErrSessionCancelled", err)
	}
	if _, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_after_cancel", "confirm-after-cancel", "confirm-hash"); !errors.Is(err, ErrSessionCancelled) {
		t.Fatalf("BeginGeneration() after cancel claim error = %v, want ErrSessionCancelled", err)
	}
}

func TestFailGenerationAfterReleasePersistsTerminalFailure(t *testing.T) {
	db, _ := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task := createOutlineReadySession(t, service, "user_failed_generation", 2)
	if _, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_failed", "confirm-failed", "confirm-hash"); err != nil {
		t.Fatalf("BeginGeneration() error = %v", err)
	}
	claim, _, err := service.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimGenerationRun() error = %v", err)
	}
	if _, err := service.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, claim, Slide{ID: "slide_1", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "Partial"}}}); err != nil {
		t.Fatalf("PersistGeneratedSlide() error = %v", err)
	}
	failed, err := service.FailGenerationAfterRelease(context.Background(), taskOwner(task), task.TaskID, claim, "PPT_AGENT_PROVIDER_UNAVAILABLE")
	if err != nil {
		t.Fatalf("FailGenerationAfterRelease() error = %v", err)
	}
	if failed.Stage != StageFailed || failed.Status != StatusFailed || failed.ErrorCode != "PPT_AGENT_PROVIDER_UNAVAILABLE" || failed.CurrentPage != 1 || failed.Progress != 50 || failed.GenerationLease != nil {
		t.Fatalf("FailGenerationAfterRelease() task = %#v", failed)
	}
	if _, _, err := service.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, time.Now().UTC().Add(time.Hour)); !errors.Is(err, ErrInvalidStage) {
		t.Fatalf("FAILED reclaim error = %v, want ErrInvalidStage", err)
	}
}

func TestOnlyGeneratingTasksCanReclaimExpiredLease(t *testing.T) {
	for _, stage := range []Stage{StageReady, StageFailed, StageCancelled} {
		t.Run(string(stage), func(t *testing.T) {
			db, _ := newPPTPostgresTestDB(t)
			now := time.Now().UTC()
			task := Task{
				TaskID: "ppt_terminal_lease", SessionID: "ppt_terminal_lease", UserID: "user_terminal_lease",
				Stage: stage, Status: StageStatus(stage), SlideCount: 1, CurrentPage: 1, Progress: 100,
				Slides:          []Slide{{ID: "slide_1", Page: 1}},
				GenerationLease: &GenerationLease{RunToken: "expired", LeaseUntil: now.Add(-time.Minute).Format(time.RFC3339Nano)},
				CreatedAt:       now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano),
			}
			persistPPTPostgresTestTask(t, db, task)
			_, _, err := NewPostgresService(db).ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, now)
			if stage == StageCancelled {
				if !errors.Is(err, ErrSessionCancelled) {
					t.Fatalf("ClaimGenerationRun() error = %v, want ErrSessionCancelled", err)
				}
			} else if !errors.Is(err, ErrInvalidStage) {
				t.Fatalf("ClaimGenerationRun() error = %v, want ErrInvalidStage", err)
			}
		})
	}
}

func TestGenerationBillingRaceReleaseWinsBeforeCapturedCompletion(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	generator := NewPostgresService(db)
	canceller := NewPostgresService(db)
	task := createOutlineReadySession(t, generator, "user_release_wins", 1)
	if _, err := claimAndBindGenerationForTest(context.Background(), generator, taskOwner(task), task.TaskID, "billing_release_wins", "confirm-release-wins", "confirm-hash"); err != nil {
		t.Fatalf("BeginGeneration() error = %v", err)
	}
	runClaim, _, err := generator.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimGenerationRun() error = %v", err)
	}
	if _, err := generator.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, runClaim, Slide{ID: "slide_1", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "Only"}}}); err != nil {
		t.Fatalf("PersistGeneratedSlide() error = %v", err)
	}
	cancelClaim, _, err := canceller.BeginCancel(context.Background(), taskOwner(task), task.TaskID, "cancel-release-wins", "cancel-hash")
	if err != nil {
		t.Fatalf("BeginCancel() error = %v", err)
	}
	cancelled, err := canceller.CompleteCancel(context.Background(), taskOwner(task), task.TaskID, cancelClaim)
	if err != nil || cancelled.Stage != StageCancelled {
		t.Fatalf("CompleteCancel() task=%#v err=%v", cancelled, err)
	}
	before, _ := state.snapshot(task.TaskID)
	if _, err := generator.CompleteGenerationAfterCapture(context.Background(), taskOwner(task), task.TaskID, runClaim); !errors.Is(err, ErrSessionCancelled) {
		t.Fatalf("captured completion after release won error = %v, want ErrSessionCancelled", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, before)
}

func TestGenerationBillingRaceCaptureWinsAndClosesCancelClaim(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	generator := NewPostgresService(db)
	canceller := NewPostgresService(db)
	task := createOutlineReadySession(t, generator, "user_capture_wins", 1)
	if _, err := claimAndBindGenerationForTest(context.Background(), generator, taskOwner(task), task.TaskID, "billing_capture_wins", "confirm-capture-wins", "confirm-hash"); err != nil {
		t.Fatalf("BeginGeneration() error = %v", err)
	}
	runClaim, _, err := generator.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimGenerationRun() error = %v", err)
	}
	if _, err := generator.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, runClaim, Slide{ID: "slide_1", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "Only"}}}); err != nil {
		t.Fatalf("PersistGeneratedSlide() error = %v", err)
	}
	cancelClaim, _, err := canceller.BeginCancel(context.Background(), taskOwner(task), task.TaskID, "cancel-capture-wins", "cancel-hash")
	if err != nil {
		t.Fatalf("BeginCancel() error = %v", err)
	}
	if _, err := generator.CompleteGeneration(context.Background(), taskOwner(task), task.TaskID, runClaim); !errors.Is(err, ErrSessionCancelled) {
		t.Fatalf("ordinary completion error = %v, want ErrSessionCancelled", err)
	}
	ready, err := generator.CompleteGenerationAfterCapture(context.Background(), taskOwner(task), task.TaskID, runClaim)
	if err != nil || ready.Stage != StageReady || ready.GenerationLease != nil {
		t.Fatalf("CompleteGenerationAfterCapture() task=%#v err=%v", ready, err)
	}
	confirmRecord, ok := idempotencyRecordByScope(ready.IdempotencyRecords, idempotencyScopeConfirm)
	if !ok || confirmRecord.State != idempotencyStateCompleted {
		t.Fatalf("confirm record = %#v, found=%v", confirmRecord, ok)
	}
	cancelRecord, ok := idempotencyRecordByScope(ready.IdempotencyRecords, idempotencyScopeCancel)
	if !ok || cancelRecord.State != idempotencyStateFailed || cancelRecord.ErrorCode != ErrBillingAlreadyCaptured.Error() {
		t.Fatalf("cancel record = %#v, found=%v", cancelRecord, ok)
	}
	beforeReplay, _ := state.snapshot(task.TaskID)
	if _, _, err := canceller.BeginCancel(context.Background(), taskOwner(task), task.TaskID, cancelClaim.Key, cancelClaim.RequestHash); !errors.Is(err, ErrBillingAlreadyCaptured) {
		t.Fatalf("captured cancel replay error = %v, want ErrBillingAlreadyCaptured", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, beforeReplay)
}

func TestCapturedCompletionStillRequiresExactPageCoverage(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	task := createOutlineReadySession(t, service, "user_capture_incomplete", 2)
	if _, err := claimAndBindGenerationForTest(context.Background(), service, taskOwner(task), task.TaskID, "billing_capture_incomplete", "confirm-capture-incomplete", "confirm-hash"); err != nil {
		t.Fatalf("BeginGeneration() error = %v", err)
	}
	claim, _, err := service.ClaimGenerationRun(context.Background(), taskOwner(task), task.TaskID, time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimGenerationRun() error = %v", err)
	}
	if _, err := service.PersistGeneratedSlide(context.Background(), taskOwner(task), task.TaskID, claim, Slide{ID: "slide_1", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "Partial"}}}); err != nil {
		t.Fatalf("PersistGeneratedSlide() error = %v", err)
	}
	if _, _, err := service.BeginCancel(context.Background(), taskOwner(task), task.TaskID, "cancel-capture-incomplete", "cancel-hash"); err != nil {
		t.Fatalf("BeginCancel() error = %v", err)
	}
	before, _ := state.snapshot(task.TaskID)
	if _, err := service.CompleteGenerationAfterCapture(context.Background(), taskOwner(task), task.TaskID, claim); !errors.Is(err, ErrGenerationIncomplete) {
		t.Fatalf("incomplete captured completion error = %v, want ErrGenerationIncomplete", err)
	}
	assertPPTPostgresRowUnchanged(t, state, task.TaskID, before)
}

func TestPostgresProjectionColumnsOverrideCanonicalRaw(t *testing.T) {
	raw, err := json.Marshal(Task{
		TaskID: "raw_task", TenantID: "raw_tenant", UserID: "raw_user", SessionID: "raw_session", SkillCode: "raw_skill", Stage: StageReady,
		Status: StatusSuccess, SourceFileIDs: []string{"raw_file"}, Slides: []Slide{{ID: "slide_1", Page: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	projected, err := taskFromPostgresProjection(postgresTaskProjection{
		TaskID: "column_task", TenantID: "column_tenant", UserID: "column_user", ClientRequestID: "column_request",
		SessionID: "column_session", SkillCode: "column_skill", Stage: StageGenerating,
		Status: StatusProcessing, SourceFileIDsJSON: []byte(`["column_file"]`), Raw: raw,
	})
	if err != nil {
		t.Fatalf("taskFromPostgresProjection() error = %v", err)
	}
	if projected.TaskID != "column_task" || projected.TenantID != "column_tenant" || projected.UserID != "column_user" || projected.SessionID != "column_session" || projected.SkillCode != "column_skill" || projected.Stage != StageGenerating || projected.Status != StatusProcessing || len(projected.SourceFileIDs) != 1 || projected.SourceFileIDs[0] != "column_file" {
		t.Fatalf("projected task = %#v", projected)
	}
}

func TestPostgresPersistenceWritesIndexedColumnsAndNormalizedRawAtomically(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	task, err := NewPostgresService(db).CreateSession(context.Background(), SessionRequest{
		Owner: testOwner("user_projection"), Prompt: "Projection", SkillCode: "general", SourceFileIDs: []string{"file_a"}, SlideCount: 3,
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	row, ok := state.snapshot(task.TaskID)
	if !ok {
		t.Fatalf("task %q was not persisted", task.TaskID)
	}
	var raw Task
	if err := json.Unmarshal(row.raw, &raw); err != nil {
		t.Fatal(err)
	}
	var sourceFileIDs []string
	if err := json.Unmarshal(row.sourceFileIDs, &sourceFileIDs); err != nil {
		t.Fatal(err)
	}
	if row.sessionID != task.TaskID || row.skillCode != "general" || row.stage != StageDraft || len(sourceFileIDs) != 1 || sourceFileIDs[0] != "file_a" {
		t.Fatalf("indexed row = %#v sourceFileIDs=%#v", row, sourceFileIDs)
	}
	if raw.SessionID != row.sessionID || raw.SkillCode != row.skillCode || raw.Stage != row.stage || raw.Status != row.status || len(raw.SourceFileIDs) != 1 || raw.SourceFileIDs[0] != "file_a" {
		t.Fatalf("raw/indexed projection mismatch: raw=%#v row=%#v", raw, row)
	}
	if _, _, err := NewPostgresService(db).BeginOperation(context.Background(), taskOwner(task), task.TaskID, "message", "projection-lock", "projection-hash"); err != nil {
		t.Fatalf("BeginOperation() error = %v", err)
	}
	strictUpserts, lockedMutations := state.sqlEvidence()
	if strictUpserts < 2 || lockedMutations < 1 {
		t.Fatalf("SQL evidence strictUpserts=%d lockedMutations=%d, want >=2/>=1", strictUpserts, lockedMutations)
	}
}

func TestPostgresPersistenceStoresBlocksAsOnlySlideContentIR(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := Task{
		TaskID: "ppt_blocks_only", SessionID: "ppt_blocks_only", TenantID: "tenant_blocks", UserID: "user_blocks",
		SkillCode: "general", Stage: StageReady, Status: StatusSuccess, SlideCount: 1, CurrentPage: 1, Progress: 100,
		Slides: []Slide{{
			ID: "slide_1", Page: 1, Layout: "imageText",
			Blocks: []SlideBlock{{Type: "title", Text: "Canonical title"}, {Type: "paragraph", Text: "Canonical body"}, {Type: "image", ImageRef: "asset://image_1"}},
		}},
		CreatedAt: now, UpdatedAt: now,
	}
	persistPPTPostgresTestTask(t, db, task)
	row, ok := state.snapshot(task.TaskID)
	if !ok {
		t.Fatalf("task %q was not persisted", task.TaskID)
	}
	var document map[string]any
	if err := json.Unmarshal(row.raw, &document); err != nil {
		t.Fatal(err)
	}
	slides, ok := document["slides"].([]any)
	if !ok || len(slides) != 1 {
		t.Fatalf("raw slides = %#v", document["slides"])
	}
	slide, ok := slides[0].(map[string]any)
	if !ok {
		t.Fatalf("raw slide = %#v", slides[0])
	}
	for _, legacy := range []string{"title", "content", "bulletPoints", "imageUrl", "speakerNotes"} {
		if _, exists := slide[legacy]; exists {
			t.Fatalf("persisted raw contains legacy slide field %q: %#v", legacy, slide)
		}
	}
	blocks, ok := slide["blocks"].([]any)
	if !ok || len(blocks) != 3 {
		t.Fatalf("persisted canonical blocks = %#v", slide["blocks"])
	}
}

func TestPostgresOwnerScopeIsolatesSameUserAcrossTenants(t *testing.T) {
	db, _ := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	ownerA := OwnerScope{TenantID: "tenant_a", UserID: "shared_user"}
	ownerB := OwnerScope{TenantID: "tenant_b", UserID: "shared_user"}
	request := SessionRequest{ClientRequestID: "same-request", Prompt: "Tenant-isolated", SkillCode: "general", SlideCount: 1}
	request.Owner = ownerA
	taskA, err := service.CreateSession(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateSession(tenant A) error = %v", err)
	}
	request.Owner = ownerB
	taskB, err := service.CreateSession(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateSession(tenant B) error = %v", err)
	}
	if taskA.TaskID == taskB.TaskID || taskA.TenantID != ownerA.TenantID || taskB.TenantID != ownerB.TenantID {
		t.Fatalf("cross-tenant idempotency collision: taskA=%#v taskB=%#v", taskA, taskB)
	}
	if _, err := service.GetTask(ownerA, taskB.TaskID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("cross-tenant GetTask() error = %v, want ErrTaskNotFound", err)
	}
	if _, _, err := service.BeginOperation(context.Background(), ownerA, taskB.TaskID, "message", "scope-key", "scope-hash"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("cross-tenant BeginOperation() error = %v, want ErrTaskNotFound", err)
	}
	if _, err := service.UpdateSlideContent(ownerA, taskB.TaskID, "missing", Slide{Blocks: []SlideBlock{{Type: "title", Text: "blocked"}}}); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("cross-tenant UpdateSlideContent() error = %v, want ErrTaskNotFound", err)
	}
	if err := service.Delete(ownerA, taskB.TaskID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("cross-tenant Delete() error = %v, want ErrTaskNotFound", err)
	}
	historyA, err := service.HistoryWithError(ownerA)
	if err != nil || len(historyA) != 1 || historyA[0].TaskID != taskA.TaskID {
		t.Fatalf("tenant A history=%#v err=%v", historyA, err)
	}
	historyB, err := service.HistoryWithError(ownerB)
	if err != nil || len(historyB) != 1 || historyB[0].TaskID != taskB.TaskID {
		t.Fatalf("tenant B history=%#v err=%v", historyB, err)
	}
}

func TestPostgresProjectionRejectsInvalidStageMetadata(t *testing.T) {
	raw, err := json.Marshal(Task{TaskID: "raw", TenantID: "tenant", UserID: "user", Stage: StageReady, Status: StatusSuccess})
	if err != nil {
		t.Fatal(err)
	}
	for _, projection := range []postgresTaskProjection{
		{TaskID: "missing", TenantID: "tenant", UserID: "user", Stage: "", Status: StatusPending, Raw: raw},
		{TaskID: "unknown", TenantID: "tenant", UserID: "user", Stage: Stage("UNKNOWN"), Status: StatusPending, Raw: raw},
		{TaskID: "conflict", TenantID: "tenant", UserID: "user", Stage: StageReady, Status: StatusPending, Raw: raw},
	} {
		if _, err := taskFromPostgresProjection(projection); !errors.Is(err, ErrInvalidStage) {
			t.Fatalf("taskFromPostgresProjection(%q) error = %v, want ErrInvalidStage", projection.TaskID, err)
		}
	}
}

func TestPPTPostgresFakeRejectsWeakPersistenceAndMutationSQL(t *testing.T) {
	state := &pptPostgresTestState{tasks: map[string]pptPostgresTestRow{}}
	conn := &pptPostgresTestConn{state: state}
	now := time.Now().UTC()
	legacyArgs := pptPostgresNamedValues("task", "user", "request", StatusPending, now, now, `{}`)
	if _, err := conn.ExecContext(context.Background(), `insert into xz_ppt_tasks(task_id,user_id,client_request_id,status,created_at,updated_at,raw) values($1,$2,$3,$4,$5,$6,$7::jsonb)`, legacyArgs); err == nil {
		t.Fatal("fake driver accepted legacy 7-argument persistence")
	}
	weakElevenArgs := pptPostgresNamedValues("task", "user", "request", StatusPending, "task", "general", string(StageDraft), `[]`, now, now, `{}`)
	if _, err := conn.ExecContext(context.Background(), `insert into xz_ppt_tasks(task_id,user_id,client_request_id,status,session_id,skill_code,stage,source_file_ids,created_at,updated_at,raw) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, weakElevenArgs); err == nil {
		t.Fatal("fake driver accepted persistence without exact JSONB casts")
	}
	tx, err := conn.BeginTx(context.Background(), driver.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := conn.QueryContext(context.Background(), postgresTaskProjectionSQL+` where task_id=$1 and user_id=$2`, pptPostgresNamedValues("task", "user")); err == nil {
		t.Fatal("fake driver accepted unlocked mutation projection")
	}
}

func pptPostgresNamedValues(values ...any) []driver.NamedValue {
	result := make([]driver.NamedValue, len(values))
	for index, value := range values {
		result[index] = driver.NamedValue{Ordinal: index + 1, Value: value}
	}
	return result
}

func testOwner(userID string) OwnerScope {
	return OwnerScope{TenantID: "tenant_default", UserID: userID}
}

func taskOwner(task Task) OwnerScope {
	tenantID := task.TenantID
	if tenantID == "" {
		tenantID = "tenant_default"
	}
	return OwnerScope{TenantID: tenantID, UserID: task.UserID}
}

// claimAndBindGenerationForTest exercises the same two-step, claim-fenced
// choreography required by production callers.
func claimAndBindGenerationForTest(ctx context.Context, service *Service, owner OwnerScope, taskID, billingTaskID, key, requestHash string) (Task, error) {
	claim, _, err := service.BeginGenerationClaim(ctx, owner, taskID, key, requestHash)
	if err != nil {
		return Task{}, err
	}
	return service.BindGenerationBilling(ctx, owner, taskID, claim, billingTaskID)
}

func TestEnsurePostgresReadyUsesReadOnlyCatalogChecks(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	if err := NewPostgresService(db).ensurePostgresReady(context.Background()); err != nil {
		t.Fatalf("ensurePostgresReady() error = %v", err)
	}
	if got := state.catalogQueryCount(); got != 3 {
		t.Fatalf("catalog query count = %d, want 3", got)
	}
	if query := state.lastDDL(); query != "" {
		t.Fatalf("ensurePostgresReady() executed DDL: %s", query)
	}
	if got := state.mutationQueryCount(); got != 0 {
		t.Fatalf("ensurePostgresReady() executed mutation query count=%d", got)
	}
}

func TestEnsurePostgresReadyFailsClosedForMissingSchemaComponents(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*pptPostgresTestState)
	}{
		{name: "table", prepare: func(state *pptPostgresTestState) { state.schemaTablePresent = false }},
		{name: "column", prepare: func(state *pptPostgresTestState) { delete(state.schemaColumns, "stage") }},
		{name: "base type", prepare: func(state *pptPostgresTestState) {
			state.schemaColumns["task_id"] = pptPostgresCatalogColumn{notNull: true, typeName: "character varying(127)"}
		}},
		{name: "base nullability", prepare: func(state *pptPostgresTestState) {
			state.schemaColumns["raw"] = pptPostgresCatalogColumn{notNull: false, typeName: "jsonb"}
		}},
		{name: "client request default", prepare: func(state *pptPostgresTestState) {
			state.schemaColumns["client_request_id"] = pptPostgresCatalogColumn{notNull: true, typeName: "character varying(256)", defaultExpr: "'unexpected'::character varying"}
		}},
		{name: "session type", prepare: func(state *pptPostgresTestState) {
			state.schemaColumns["session_id"] = pptPostgresCatalogColumn{notNull: false, typeName: "text"}
		}},
		{name: "session nullability", prepare: func(state *pptPostgresTestState) {
			state.schemaColumns["session_id"] = pptPostgresCatalogColumn{notNull: true, typeName: "character varying(128)"}
		}},
		{name: "index", prepare: func(state *pptPostgresTestState) { delete(state.schemaIndexes, "idx_xz_ppt_tasks_tenant_user_session") }},
		{name: "index predicate near match", prepare: func(state *pptPostgresTestState) {
			index := state.schemaIndexes["idx_xz_ppt_tasks_tenant_user_session"]
			index.predicate = "(session_id IS NOT NULL) OR (user_id <> '')"
			state.schemaIndexes["idx_xz_ppt_tasks_tenant_user_session"] = index
		}},
		{name: "index key order", prepare: func(state *pptPostgresTestState) {
			index := state.schemaIndexes["idx_xz_ppt_tasks_tenant_user_stage_updated"]
			index.keys = []pptPostgresCatalogIndexKey{{column: "tenant_id"}, {column: "user_id"}, {column: "updated_at", descending: true}, {column: "stage"}}
			state.schemaIndexes["idx_xz_ppt_tasks_tenant_user_stage_updated"] = index
		}},
		{name: "index sort", prepare: func(state *pptPostgresTestState) {
			index := state.schemaIndexes["idx_xz_ppt_tasks_tenant_user_stage_updated"]
			index.keys[0].descending = true
			state.schemaIndexes["idx_xz_ppt_tasks_tenant_user_stage_updated"] = index
		}},
		{name: "idempotency unique", prepare: func(state *pptPostgresTestState) {
			index := state.schemaIndexes["idx_xz_ppt_tasks_tenant_user_client_request"]
			index.unique = false
			state.schemaIndexes["idx_xz_ppt_tasks_tenant_user_client_request"] = index
		}},
		{name: "default", prepare: func(state *pptPostgresTestState) {
			state.schemaColumns["stage"] = pptPostgresCatalogColumn{notNull: true, typeName: "character varying", defaultExpr: "'WRONG'::character varying"}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, state := newPPTPostgresTestDB(t)
			test.prepare(state)
			err := NewPostgresService(db).ensurePostgresReady(context.Background())
			if !errors.Is(err, ErrPostgresUnavailable) {
				t.Fatalf("ensurePostgresReady() error = %v, want wrapped ErrPostgresUnavailable", err)
			}
			if state.lastDDL() != "" || state.mutationQueryCount() != 0 {
				t.Fatalf("failed readiness mutated schema/data: ddl=%q mutations=%d", state.lastDDL(), state.mutationQueryCount())
			}
		})
	}
}

func TestCreateSessionRejectsUnknownAndOversizeSkills(t *testing.T) {
	db, _ := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	owner := OwnerScope{TenantID: "tenant_skill", UserID: "user_skill"}
	for _, req := range []SessionRequest{
		{Owner: owner, Prompt: "unknown", SkillCode: "does-not-exist", SlideCount: 1},
		{Owner: owner, Prompt: "oversize", SkillCode: "meeting_summary", SlideCount: 13},
		{Owner: owner, Prompt: "blank", SkillCode: "", SlideCount: 1},
	} {
		if _, err := service.CreateSession(context.Background(), req); !errors.Is(err, ErrInvalidSkill) {
			t.Fatalf("CreateSession(%#v) error = %v, want ErrInvalidSkill", req, err)
		}
	}
}

func TestPostgresIndexMatcherRequiresUniqueWhenRequested(t *testing.T) {
	index := postgresSchemaIndex{valid: true, ready: true, unique: false, keys: []postgresSchemaIndexKey{{Column: "tenant_id"}, {Column: "user_id"}, {Column: "client_request_id"}}, predicate: "client_request_id <> ''"}
	if postgresIndexMatches(index, index.keys, "client_request_id <> ''", true) {
		t.Fatal("postgresIndexMatches accepted non-unique idempotency index")
	}
	index.unique = true
	if !postgresIndexMatches(index, index.keys, "client_request_id <> ''", true) {
		t.Fatal("postgresIndexMatches rejected matching unique idempotency index")
	}
}

func TestEnsurePostgresReadyPreservesCatalogQueryCause(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	want := errors.New("catalog unavailable")
	state.catalogQueryErr = want
	err := NewPostgresService(db).ensurePostgresReady(context.Background())
	if !errors.Is(err, ErrPostgresUnavailable) {
		t.Fatalf("ensurePostgresReady() error = %v, want wrapped ErrPostgresUnavailable", err)
	}
	if !errors.Is(err, want) {
		t.Fatalf("ensurePostgresReady() error = %v, want wrapped catalog cause", err)
	}
}

func createOutlineReadySession(t *testing.T, service *Service, userID string, slideCount int) Task {
	t.Helper()
	task, err := service.CreateSession(context.Background(), SessionRequest{Owner: testOwner(userID), Prompt: "Deck", SkillCode: "general", SlideCount: slideCount})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	claim, _, err := service.BeginOperation(context.Background(), testOwner(userID), task.TaskID, "message", "outline-key", "outline-hash")
	if err != nil {
		t.Fatalf("BeginOperation() error = %v", err)
	}
	slides := make([]OutlineSlide, slideCount)
	for index := range slides {
		slides[index] = OutlineSlide{Page: index + 1, Title: fmt.Sprintf("Slide %d", index+1)}
	}
	completed, err := service.CompleteOutlineOperation(
		context.Background(), testOwner(userID), task.TaskID, claim, nil, Outline{Title: "Deck", Slides: slides})
	if err != nil {
		t.Fatalf("CompleteOutlineOperation() error = %v", err)
	}
	return completed
}

type pptCompletedOutlineNoopFixture struct {
	Completed Task
	Claim     OperationClaim
}

func createCompletedOutlineNoopFixture(t *testing.T, service *Service, userID string) pptCompletedOutlineNoopFixture {
	t.Helper()
	task, err := service.CreateSession(context.Background(), SessionRequest{Owner: testOwner(userID), Prompt: "No-op outline", SkillCode: "general", SlideCount: 1})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	claim, _, err := service.BeginOperation(context.Background(), testOwner(userID), task.TaskID, "message", "noop-outline-key", "noop-outline-hash")
	if err != nil {
		t.Fatalf("BeginOperation() error = %v", err)
	}
	completed, err := service.CompleteOutlineOperation(
		context.Background(), testOwner(userID), task.TaskID, claim,
		[]AgentMessage{{Role: "assistant", Content: "original outline"}},
		Outline{Title: "Original outline", Slides: []OutlineSlide{{Page: 1, Title: "Only"}}},
	)
	if err != nil {
		t.Fatalf("CompleteOutlineOperation() error = %v", err)
	}
	return pptCompletedOutlineNoopFixture{Completed: completed, Claim: claim}
}

type pptGeneratingNoopFixture struct {
	Task          Task
	ConfirmClaim  OperationClaim
	BillingTaskID string
	Key           string
	RequestHash   string
}

func createGeneratingNoopFixture(t *testing.T, service *Service, userID string) pptGeneratingNoopFixture {
	t.Helper()
	outline := createCompletedOutlineNoopFixture(t, service, userID)
	const key = "noop-confirm-key"
	const requestHash = "noop-confirm-hash"
	const billingTaskID = "billing_noop"
	claim, _, err := service.BeginGenerationClaim(context.Background(), testOwner(userID), outline.Completed.TaskID, key, requestHash)
	if err != nil {
		t.Fatalf("BeginGenerationClaim() error = %v", err)
	}
	generating, err := claimAndBindGenerationForTest(context.Background(), service, testOwner(userID), outline.Completed.TaskID, billingTaskID, key, requestHash)
	if err != nil {
		t.Fatalf("BeginGeneration() error = %v", err)
	}
	return pptGeneratingNoopFixture{
		Task: generating, ConfirmClaim: claim, BillingTaskID: billingTaskID, Key: key, RequestHash: requestHash,
	}
}

type pptCompletedGenerationNoopFixture struct {
	Completed     Task
	ConfirmClaim  OperationClaim
	BillingTaskID string
	Key           string
	RequestHash   string
}

func createCompletedGenerationNoopFixture(t *testing.T, service *Service, userID string) pptCompletedGenerationNoopFixture {
	t.Helper()
	generating := createGeneratingNoopFixture(t, service, userID)
	run, _, err := service.ClaimGenerationRun(context.Background(), testOwner(userID), generating.Task.TaskID, time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimGenerationRun() error = %v", err)
	}
	if _, err := service.PersistGeneratedSlide(context.Background(), testOwner(userID), generating.Task.TaskID, run, Slide{ID: "slide_1", Page: 1, Blocks: []SlideBlock{{Type: "title", Text: "Only"}}}); err != nil {
		t.Fatalf("PersistGeneratedSlide() error = %v", err)
	}
	completed, err := service.CompleteGeneration(context.Background(), testOwner(userID), generating.Task.TaskID, run)
	if err != nil {
		t.Fatalf("CompleteGeneration() error = %v", err)
	}
	return pptCompletedGenerationNoopFixture{
		Completed: completed, ConfirmClaim: generating.ConfirmClaim,
		BillingTaskID: generating.BillingTaskID, Key: generating.Key, RequestHash: generating.RequestHash,
	}
}

type pptCompletedCancelNoopFixture struct {
	Completed   Task
	Claim       CancelClaim
	Key         string
	RequestHash string
}

func createCompletedCancelNoopFixture(t *testing.T, service *Service, userID string) pptCompletedCancelNoopFixture {
	t.Helper()
	task, err := service.CreateSession(context.Background(), SessionRequest{Owner: testOwner(userID), Prompt: "No-op cancel", SkillCode: "general", SlideCount: 1})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	const key = "noop-cancel-key"
	const requestHash = "noop-cancel-hash"
	claim, _, err := service.BeginCancel(context.Background(), testOwner(userID), task.TaskID, key, requestHash)
	if err != nil {
		t.Fatalf("BeginCancel() error = %v", err)
	}
	completed, err := service.CompleteCancel(context.Background(), testOwner(userID), task.TaskID, claim)
	if err != nil {
		t.Fatalf("CompleteCancel() error = %v", err)
	}
	return pptCompletedCancelNoopFixture{Completed: completed, Claim: claim, Key: key, RequestHash: requestHash}
}

func assertPPTPostgresNoop(t *testing.T, state *pptPostgresTestState, taskID string, before pptPostgresTestRow, writes int, scope, key string) {
	t.Helper()
	beforeResponseJSON := pptPostgresRowResponseJSON(t, before, scope, key)
	assertPPTPostgresRowUnchanged(t, state, taskID, before)
	after, ok := state.snapshot(taskID)
	if !ok {
		t.Fatalf("task %q disappeared", taskID)
	}
	if got := state.strictUpsertCount(); got != writes {
		t.Fatalf("read-only replay persisted task: writes=%d, want %d", got, writes)
	}
	if afterResponseJSON := pptPostgresRowResponseJSON(t, after, scope, key); afterResponseJSON != beforeResponseJSON {
		t.Fatalf("read-only replay changed responseJSON: before=%q after=%q", beforeResponseJSON, afterResponseJSON)
	}
}

func pptPostgresRowResponseJSON(t *testing.T, row pptPostgresTestRow, scope, key string) string {
	t.Helper()
	return pptResponseJSONFromRaw(t, row.raw, scope, key)
}

func pptResponseJSONFromRaw(t *testing.T, raw []byte, scope, key string) string {
	t.Helper()
	var task Task
	if err := json.Unmarshal(raw, &task); err != nil {
		t.Fatalf("unmarshal persisted task: %v", err)
	}
	for _, record := range task.IdempotencyRecords {
		if record.Scope == scope && record.Key == key {
			return record.ResponseJSON
		}
	}
	t.Fatalf("idempotency record %q/%q not found in persisted task", scope, key)
	return ""
}

func hasIdempotencyKey(records []IdempotencyRecord, key string) bool {
	for _, record := range records {
		if record.Key == key {
			return true
		}
	}
	return false
}

func idempotencyRecordByScope(records []IdempotencyRecord, scope string) (IdempotencyRecord, bool) {
	for index := len(records) - 1; index >= 0; index-- {
		if records[index].Scope == scope {
			return records[index], true
		}
	}
	return IdempotencyRecord{}, false
}

func assertPPTPostgresRowUnchanged(t *testing.T, state *pptPostgresTestState, taskID string, before pptPostgresTestRow) {
	t.Helper()
	after, ok := state.snapshot(taskID)
	if !ok {
		t.Fatalf("task %q disappeared", taskID)
	}
	if after.taskID != before.taskID || after.userID != before.userID || after.clientRequestID != before.clientRequestID || after.status != before.status || after.sessionID != before.sessionID || after.skillCode != before.skillCode || after.stage != before.stage || string(after.sourceFileIDs) != string(before.sourceFileIDs) || !after.createdAt.Equal(before.createdAt) || !after.updatedAt.Equal(before.updatedAt) || string(after.raw) != string(before.raw) {
		t.Fatalf("persisted row changed after rejected mutation\nbefore=%#v\nafter=%#v", before, after)
	}
}

func assertSlideImageRepresentations(t *testing.T, slide Slide, want string) {
	t.Helper()
	if slide.ImageURL != "" {
		t.Fatalf("canonical slide retained legacy imageUrl = %q", slide.ImageURL)
	}
	imageRefs := []string{}
	for _, block := range slide.Blocks {
		if block.Type == "image" {
			imageRefs = append(imageRefs, block.ImageRef)
		}
	}
	if want == "" && len(imageRefs) != 0 {
		t.Fatalf("image blocks = %#v, want none", imageRefs)
	}
	if want != "" && (len(imageRefs) != 1 || imageRefs[0] != want) {
		t.Fatalf("image blocks = %#v, want [%q]", imageRefs, want)
	}
}

func persistPPTPostgresTestTask(t *testing.T, db *sql.DB, task Task) {
	t.Helper()
	if task.TenantID == "" {
		task.TenantID = "tenant_default"
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := persistPostgresTask(context.Background(), tx, task); err != nil {
		t.Fatalf("persistPostgresTask() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
}

var pptPostgresTestDriverID atomic.Uint64

type pptPostgresTestState struct {
	mu                    sync.Mutex
	tasks                 map[string]pptPostgresTestRow
	ddlQueries            []string
	catalogQueries        []string
	catalogQueryErr       error
	mutationQueries       int
	schemaTablePresent    bool
	schemaColumns         map[string]pptPostgresCatalogColumn
	schemaIndexes         map[string]pptPostgresCatalogIndex
	strictUpserts         int
	lockedMutationQueries int
}

type pptPostgresCatalogColumn struct {
	notNull     bool
	typeName    string
	defaultExpr string
}

type pptPostgresCatalogIndexKey struct {
	column     string
	descending bool
}

type pptPostgresCatalogIndex struct {
	valid     bool
	ready     bool
	unique    bool
	keys      []pptPostgresCatalogIndexKey
	predicate string
}

type pptPostgresTestRow struct {
	taskID          string
	tenantID        string
	userID          string
	clientRequestID string
	status          string
	sessionID       string
	skillCode       string
	stage           Stage
	sourceFileIDs   []byte
	createdAt       time.Time
	updatedAt       time.Time
	raw             []byte
}

func (s *pptPostgresTestState) snapshot(taskID string) (pptPostgresTestRow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.tasks[taskID]
	row.raw = append([]byte(nil), row.raw...)
	row.sourceFileIDs = append([]byte(nil), row.sourceFileIDs...)
	return row, ok
}

func (s *pptPostgresTestState) strictUpsertCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.strictUpserts
}

func (s *pptPostgresTestState) lastDDL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.ddlQueries) == 0 {
		return ""
	}
	return s.ddlQueries[len(s.ddlQueries)-1]
}

func (s *pptPostgresTestState) catalogQueryCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.catalogQueries)
}

func (s *pptPostgresTestState) mutationQueryCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutationQueries
}

func (s *pptPostgresTestState) sqlEvidence() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.strictUpserts, s.lockedMutationQueries
}

func newPPTPostgresTestDB(t *testing.T) (*sql.DB, *pptPostgresTestState) {
	t.Helper()
	state := &pptPostgresTestState{
		tasks:              map[string]pptPostgresTestRow{},
		schemaTablePresent: true,
		schemaColumns: map[string]pptPostgresCatalogColumn{
			"task_id":           {notNull: true, typeName: "character varying(128)"},
			"tenant_id":         {notNull: true, typeName: "character varying(128)"},
			"user_id":           {notNull: true, typeName: "character varying(128)"},
			"client_request_id": {notNull: true, typeName: "character varying(256)", defaultExpr: "''::character varying"},
			"status":            {notNull: true, typeName: "character varying(32)"},
			"created_at":        {notNull: true, typeName: "timestamp with time zone"},
			"updated_at":        {notNull: true, typeName: "timestamp with time zone"},
			"raw":               {notNull: true, typeName: "jsonb"},
			"session_id":        {notNull: false, typeName: "character varying(128)"},
			"skill_code":        {notNull: true, typeName: "character varying(64)", defaultExpr: "''::character varying"},
			"stage":             {notNull: true, typeName: "character varying(32)", defaultExpr: "'DRAFT'::character varying"},
			"source_file_ids":   {notNull: true, typeName: "jsonb", defaultExpr: "'[]'::jsonb"},
		},
		schemaIndexes: map[string]pptPostgresCatalogIndex{
			"idx_xz_ppt_tasks_tenant_user_client_request": {valid: true, ready: true, unique: true, keys: []pptPostgresCatalogIndexKey{{column: "tenant_id"}, {column: "user_id"}, {column: "client_request_id"}}, predicate: "(client_request_id <> '')"},
			"idx_xz_ppt_tasks_tenant_user_session":        {valid: true, ready: true, keys: []pptPostgresCatalogIndexKey{{column: "tenant_id"}, {column: "user_id"}, {column: "session_id"}}, predicate: "(session_id IS NOT NULL)"},
			"idx_xz_ppt_tasks_tenant_user_stage_updated":  {valid: true, ready: true, keys: []pptPostgresCatalogIndexKey{{column: "tenant_id"}, {column: "user_id"}, {column: "stage"}, {column: "updated_at", descending: true}}},
		},
	}
	driverName := fmt.Sprintf("ppt-postgres-test-%d", pptPostgresTestDriverID.Add(1))
	sql.Register(driverName, &pptPostgresTestDriver{state: state})
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, state
}

type pptPostgresTestDriver struct{ state *pptPostgresTestState }

func (d *pptPostgresTestDriver) Open(string) (driver.Conn, error) {
	return &pptPostgresTestConn{state: d.state}, nil
}

type pptPostgresTestConn struct {
	state *pptPostgresTestState
	txMu  sync.Mutex
	inTx  bool
}

func (c *pptPostgresTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("ppt postgres test driver does not prepare statements")
}

func (c *pptPostgresTestConn) Close() error { return nil }

func (c *pptPostgresTestConn) Begin() (driver.Tx, error) {
	return c.beginTx()
}

func (c *pptPostgresTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return c.beginTx()
}

func (c *pptPostgresTestConn) beginTx() (driver.Tx, error) {
	c.txMu.Lock()
	defer c.txMu.Unlock()
	if c.inTx {
		return nil, errors.New("ppt postgres test driver transaction already active")
	}
	c.inTx = true
	return &pptPostgresTestTx{conn: c}, nil
}

func (c *pptPostgresTestConn) transactionActive() bool {
	c.txMu.Lock()
	defer c.txMu.Unlock()
	return c.inTx
}

func (c *pptPostgresTestConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	statement := normalizePPTPostgresTestSQL(query)
	switch {
	case strings.Contains(statement, "create table if not exists xz_ppt_tasks"):
		c.state.mu.Lock()
		c.state.ddlQueries = append(c.state.ddlQueries, query)
		c.state.mu.Unlock()
		return driver.RowsAffected(0), nil
	case strings.Contains(statement, "select pg_advisory_xact_lock"):
		return driver.RowsAffected(0), nil
	case strings.Contains(statement, "insert into xz_ppt_tasks"):
		c.state.mu.Lock()
		c.state.mutationQueries++
		c.state.mu.Unlock()
		const requiredPersistenceSQL = "insert into xz_ppt_tasks(task_id,tenant_id,user_id,client_request_id,status,session_id,skill_code,stage,source_file_ids,created_at,updated_at,raw) values($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12::jsonb) on conflict(task_id) do update set"
		if len(args) != 12 {
			return nil, fmt.Errorf("persist args = %d, want exactly 12", len(args))
		}
		if !strings.HasPrefix(statement, requiredPersistenceSQL) || strings.Count(statement, "::jsonb") != 2 {
			return nil, fmt.Errorf("persistence SQL must use the exact 12-column projection and JSONB casts: %s", statement)
		}
		row := pptPostgresTestRow{
			taskID: fmt.Sprint(args[0].Value), tenantID: fmt.Sprint(args[1].Value), userID: fmt.Sprint(args[2].Value), clientRequestID: fmt.Sprint(args[3].Value), status: fmt.Sprint(args[4].Value),
			sessionID: fmt.Sprint(args[5].Value), skillCode: fmt.Sprint(args[6].Value), stage: Stage(fmt.Sprint(args[7].Value)), sourceFileIDs: []byte(fmt.Sprint(args[8].Value)),
			createdAt: args[9].Value.(time.Time), updatedAt: args[10].Value.(time.Time), raw: []byte(fmt.Sprint(args[11].Value)),
		}
		c.state.mu.Lock()
		for _, existing := range c.state.tasks {
			if row.clientRequestID != "" && existing.tenantID == row.tenantID && existing.userID == row.userID && existing.clientRequestID == row.clientRequestID && existing.taskID != row.taskID {
				c.state.mu.Unlock()
				return nil, errors.New("duplicate key value violates unique constraint uk_xz_ppt_tasks_user_client_request")
			}
		}
		c.state.tasks[row.taskID] = row
		c.state.strictUpserts++
		c.state.mu.Unlock()
		return driver.RowsAffected(1), nil
	case strings.HasPrefix(statement, "delete from xz_ppt_tasks"):
		c.state.mu.Lock()
		c.state.mutationQueries++
		c.state.mu.Unlock()
		tenantID, userID := fmt.Sprint(args[1].Value), fmt.Sprint(args[2].Value)
		taskID := fmt.Sprint(args[0].Value)
		c.state.mu.Lock()
		row, ok := c.state.tasks[taskID]
		if ok && row.tenantID == tenantID && row.userID == userID {
			delete(c.state.tasks, taskID)
		}
		c.state.mu.Unlock()
		if ok && row.tenantID == tenantID && row.userID == userID {
			return driver.RowsAffected(1), nil
		}
		return driver.RowsAffected(0), nil
	default:
		return nil, fmt.Errorf("unexpected ExecContext query: %s", statement)
	}
}

func (c *pptPostgresTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	statement := normalizePPTPostgresTestSQL(query)
	switch {
	case strings.Contains(statement, "from pg_catalog.pg_class c") && strings.Contains(statement, "c.relname = 'xz_ppt_tasks'"):
		c.state.mu.Lock()
		c.state.catalogQueries = append(c.state.catalogQueries, query)
		present := c.state.schemaTablePresent
		err := c.state.catalogQueryErr
		c.state.mu.Unlock()
		if err != nil {
			return nil, err
		}
		return &pptPostgresTestRows{columns: []string{"present"}, values: [][]driver.Value{{present}}}, nil
	case strings.Contains(statement, "from pg_catalog.pg_attribute a") && strings.Contains(statement, "a.attname"):
		c.state.mu.Lock()
		c.state.catalogQueries = append(c.state.catalogQueries, query)
		if c.state.catalogQueryErr != nil {
			err := c.state.catalogQueryErr
			c.state.mu.Unlock()
			return nil, err
		}
		values := make([][]driver.Value, 0, len(c.state.schemaColumns))
		for name, column := range c.state.schemaColumns {
			values = append(values, []driver.Value{name, column.notNull, column.typeName, column.defaultExpr})
		}
		c.state.mu.Unlock()
		return &pptPostgresTestRows{columns: []string{"attname", "attnotnull", "type_name", "default_expr"}, values: values}, nil
	case strings.Contains(statement, "from pg_catalog.pg_index") && strings.Contains(statement, "jsonb_agg"):
		c.state.mu.Lock()
		c.state.catalogQueries = append(c.state.catalogQueries, query)
		if c.state.catalogQueryErr != nil {
			err := c.state.catalogQueryErr
			c.state.mu.Unlock()
			return nil, err
		}
		values := make([][]driver.Value, 0, len(c.state.schemaIndexes))
		for name, index := range c.state.schemaIndexes {
			keys := make([]postgresSchemaIndexKey, 0, len(index.keys))
			for _, key := range index.keys {
				keys = append(keys, postgresSchemaIndexKey{Column: key.column, Descending: key.descending})
			}
			rawKeys, err := json.Marshal(keys)
			if err != nil {
				c.state.mu.Unlock()
				return nil, err
			}
			values = append(values, []driver.Value{name, index.valid, index.ready, index.unique, rawKeys, index.predicate})
		}
		c.state.mu.Unlock()
		return &pptPostgresTestRows{columns: []string{"indexname", "indisvalid", "indisready", "indisunique", "keys", "predicate"}, values: values}, nil
	case strings.HasPrefix(statement, "select task_id,status from xz_ppt_tasks"):
		tenantID, userID, clientRequestID := fmt.Sprint(args[0].Value), fmt.Sprint(args[1].Value), fmt.Sprint(args[2].Value)
		c.state.mu.Lock()
		defer c.state.mu.Unlock()
		for _, row := range c.state.tasks {
			if row.tenantID == tenantID && row.userID == userID && row.clientRequestID == clientRequestID {
				return &pptPostgresTestRows{columns: []string{"task_id", "status"}, values: [][]driver.Value{{row.taskID, row.status}}}, nil
			}
		}
		return &pptPostgresTestRows{columns: []string{"task_id", "status"}}, nil
	case strings.HasPrefix(statement, "select count(*) from xz_ppt_tasks where user_id=$1 and status in"):
		userID := fmt.Sprint(args[0].Value)
		active := int64(0)
		c.state.mu.Lock()
		for _, row := range c.state.tasks {
			if row.userID != userID || row.status != StatusPending && row.status != StatusProcessing {
				continue
			}
			if strings.Contains(statement, "created_at >") && !row.createdAt.After(time.Now().Add(-3*time.Second)) {
				continue
			}
			active++
		}
		c.state.mu.Unlock()
		return &pptPostgresTestRows{columns: []string{"count"}, values: [][]driver.Value{{active}}}, nil
	case strings.HasPrefix(statement, "select stage,status from xz_ppt_tasks where tenant_id=$1 and user_id=$2"):
		tenantID, userID := fmt.Sprint(args[0].Value), fmt.Sprint(args[1].Value)
		values := [][]driver.Value{}
		c.state.mu.Lock()
		for _, row := range c.state.tasks {
			if row.tenantID == tenantID && row.userID == userID {
				values = append(values, []driver.Value{string(row.stage), row.status})
			}
		}
		c.state.mu.Unlock()
		return &pptPostgresTestRows{columns: []string{"stage", "status"}, values: values}, nil
	case strings.HasPrefix(statement, "select task_id,tenant_id,user_id,client_request_id,status,session_id,skill_code,stage,source_file_ids,raw from xz_ppt_tasks where task_id=$1"):
		if c.transactionActive() {
			if !strings.HasSuffix(statement, " for update") {
				return nil, fmt.Errorf("mutation projection must use FOR UPDATE: %s", statement)
			}
			c.state.mu.Lock()
			c.state.lockedMutationQueries++
			c.state.mu.Unlock()
		}
		taskID, tenantID, userID := fmt.Sprint(args[0].Value), fmt.Sprint(args[1].Value), fmt.Sprint(args[2].Value)
		c.state.mu.Lock()
		row, ok := c.state.tasks[taskID]
		c.state.mu.Unlock()
		if !ok || row.tenantID != tenantID || row.userID != userID {
			return &pptPostgresTestRows{columns: postgresTaskProjectionColumns()}, nil
		}
		return &pptPostgresTestRows{columns: postgresTaskProjectionColumns(), values: [][]driver.Value{postgresTestProjectionValues(row)}}, nil
	case strings.HasPrefix(statement, "select task_id,tenant_id,user_id,client_request_id,status,session_id,skill_code,stage,source_file_ids,raw from xz_ppt_tasks where tenant_id=$1 and user_id=$2 and client_request_id=$3"):
		tenantID, userID, clientRequestID := fmt.Sprint(args[0].Value), fmt.Sprint(args[1].Value), fmt.Sprint(args[2].Value)
		c.state.mu.Lock()
		defer c.state.mu.Unlock()
		for _, row := range c.state.tasks {
			if row.tenantID == tenantID && row.userID == userID && row.clientRequestID == clientRequestID {
				return &pptPostgresTestRows{columns: postgresTaskProjectionColumns(), values: [][]driver.Value{postgresTestProjectionValues(row)}}, nil
			}
		}
		return &pptPostgresTestRows{columns: postgresTaskProjectionColumns()}, nil
	case strings.HasPrefix(statement, "select task_id,tenant_id,user_id,client_request_id,status,session_id,skill_code,stage,source_file_ids,raw from xz_ppt_tasks where tenant_id=$1 and user_id=$2"):
		tenantID, userID := fmt.Sprint(args[0].Value), fmt.Sprint(args[1].Value)
		values := [][]driver.Value{}
		c.state.mu.Lock()
		for _, row := range c.state.tasks {
			if row.tenantID == tenantID && row.userID == userID {
				values = append(values, postgresTestProjectionValues(row))
			}
		}
		c.state.mu.Unlock()
		return &pptPostgresTestRows{columns: postgresTaskProjectionColumns(), values: values}, nil
	case strings.HasPrefix(statement, "select task_id,user_id,client_request_id,status,session_id,skill_code,stage,source_file_ids,raw from xz_ppt_tasks order by created_at desc"):
		values := [][]driver.Value{}
		c.state.mu.Lock()
		for _, row := range c.state.tasks {
			values = append(values, postgresTestProjectionValues(row))
		}
		c.state.mu.Unlock()
		return &pptPostgresTestRows{columns: postgresTaskProjectionColumns(), values: values}, nil
	case strings.HasPrefix(statement, "select raw from xz_ppt_tasks where task_id=$1"):
		taskID, userID := fmt.Sprint(args[0].Value), fmt.Sprint(args[1].Value)
		c.state.mu.Lock()
		row, ok := c.state.tasks[taskID]
		c.state.mu.Unlock()
		if !ok || row.userID != userID {
			return &pptPostgresTestRows{columns: []string{"raw"}}, nil
		}
		return &pptPostgresTestRows{columns: []string{"raw"}, values: [][]driver.Value{{append([]byte(nil), row.raw...)}}}, nil
	case strings.HasPrefix(statement, "select raw from xz_ppt_tasks where user_id=$1"):
		userID := fmt.Sprint(args[0].Value)
		values := [][]driver.Value{}
		c.state.mu.Lock()
		for _, row := range c.state.tasks {
			if row.userID == userID {
				values = append(values, []driver.Value{append([]byte(nil), row.raw...)})
			}
		}
		c.state.mu.Unlock()
		return &pptPostgresTestRows{columns: []string{"raw"}, values: values}, nil
	default:
		return nil, fmt.Errorf("unexpected QueryContext query: %s", statement)
	}
}

func postgresTaskProjectionColumns() []string {
	return []string{"task_id", "tenant_id", "user_id", "client_request_id", "status", "session_id", "skill_code", "stage", "source_file_ids", "raw"}
}

func postgresTestProjectionValues(row pptPostgresTestRow) []driver.Value {
	return []driver.Value{
		row.taskID, row.tenantID, row.userID, row.clientRequestID, row.status, row.sessionID, row.skillCode,
		string(row.stage), append([]byte(nil), row.sourceFileIDs...), append([]byte(nil), row.raw...),
	}
}

type pptPostgresTestTx struct {
	conn *pptPostgresTestConn
}

func (tx *pptPostgresTestTx) Commit() error   { return tx.finish() }
func (tx *pptPostgresTestTx) Rollback() error { return tx.finish() }

func (tx *pptPostgresTestTx) finish() error {
	if tx == nil || tx.conn == nil {
		return nil
	}
	tx.conn.txMu.Lock()
	defer tx.conn.txMu.Unlock()
	tx.conn.inTx = false
	return nil
}

type pptPostgresTestRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *pptPostgresTestRows) Columns() []string { return r.columns }
func (r *pptPostgresTestRows) Close() error      { return nil }

func (r *pptPostgresTestRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

func normalizePPTPostgresTestSQL(query string) string {
	return strings.ToLower(strings.Join(strings.Fields(query), " "))
}
