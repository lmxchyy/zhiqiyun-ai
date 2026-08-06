package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	pptskills "xianzhi-ai/backend-go/internal/app/ppt/skills"
)

func TestPPTConfirmReservesOnceAndCapturesAtReady(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, _ := preparePPTAgentOutlineReadyTask(t, harness, 2)
	events := &pptAgentGenerationTestEvents{}
	state := newPPTAgentGenerationTestState(harness.state, events)
	billing := newPPTAgentBillingTestStore(events)

	response := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-once")
	if response.Code != http.StatusOK {
		t.Fatalf("confirm status = %d, body = %s", response.Code, response.Body.String())
	}
	ready := waitForPPTAgentStage(t, state, taskID, pptapp.StageReady)
	if ready.Progress != 100 || ready.CurrentPage != 2 || len(ready.Slides) != 2 {
		t.Fatalf("ready task progress = %d page = %d slides = %d", ready.Progress, ready.CurrentPage, len(ready.Slides))
	}
	createCalls, reserveEffects, completeCalls, captureEffects, _, releaseEffects := billing.counts()
	if createCalls != 1 || reserveEffects != 1 || completeCalls != 1 || captureEffects != 1 || releaseEffects != 0 {
		t.Fatalf("billing counts = create %d reserve %d complete %d capture %d release %d", createCalls, reserveEffects, completeCalls, captureEffects, releaseEffects)
	}
	if state.runClaims != 1 || state.readyWrites != 1 {
		t.Fatalf("generation counts = run claims %d ready writes %d", state.runClaims, state.readyWrites)
	}
	request := billing.lastCreateRequest()
	if request.ClientRequestID != "ppt-confirm:"+taskID || request.UserID == "" || request.ModuleCode != modulePPTGeneration || request.Type != "PPT_GENERATION" {
		t.Fatalf("billing request identity = %#v", request)
	}
	if got := int(anyFloatOrDefault(request.Params["page_count"], 0)); got != 2 {
		t.Fatalf("billing page_count = %d, want 2", got)
	}
	if !events.before("billing:reserve", "state:begin-generation") || !events.before("billing:capture", "state:ready") {
		t.Fatalf("billing/state order = %#v", events.snapshot())
	}
}

func TestPPTAgentConfirmBillingRequestUsesCanonicalTaskImageIntent(t *testing.T) {
	for _, task := range []pptapp.Task{
		{TaskID: "ppt_images", UserID: "user_billing", ImageSource: "ai"},
		{TaskID: "ppt_no_images", UserID: "user_billing", ImageSource: "none"},
	} {
		request := pptAgentConfirmBillingRequest(task, createGenerationTaskRequest{}, 3)
		got, ok := request.Params["with_images"].(bool)
		if !ok || got != task.WithImages() {
			t.Fatalf("billing with_images = %#v, want canonical task intent %v for %#v", request.Params["with_images"], task.WithImages(), task)
		}
	}
}

func TestPPTGenerationPanicAfterBindingReleasesAndFencesFailed(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, _ := preparePPTAgentOutlineReadyTask(t, harness, 1)
	events := &pptAgentGenerationTestEvents{}
	state := newPPTAgentGenerationTestState(harness.state, events)
	state.panicOnPersist = true
	billing := newPPTAgentBillingTestStore(events)

	response := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-panic-cleanup")
	if response.Code != http.StatusOK {
		t.Fatalf("confirm status = %d, body = %s", response.Code, response.Body.String())
	}
	failed := waitForPPTAgentStage(t, state, taskID, pptapp.StageFailed)
	_, reserveEffects, _, captureEffects, _, releaseEffects := billing.counts()
	if failed.ErrorCode != "PPT_GENERATION_FAILED" || reserveEffects != 1 || captureEffects != 0 || releaseEffects != 1 || state.failedWrites != 1 {
		t.Fatalf("panic cleanup task=%#v reserve=%d capture=%d release=%d failedWrites=%d", failed, reserveEffects, captureEffects, releaseEffects, state.failedWrites)
	}
	if !events.before("billing:lookup", "billing:release") || !events.before("billing:release", "state:failed") {
		t.Fatalf("panic cleanup order = %#v", events.snapshot())
	}
}

func TestPPTGenerationCancelledContextUsesFreshCleanupContext(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, userID := preparePPTAgentOutlineReadyTask(t, harness, 1)
	events := &pptAgentGenerationTestEvents{}
	state := newPPTAgentGenerationTestState(harness.state, events)
	task, err := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	lease := &pptapp.GenerationLease{RunToken: "run-timeout-cleanup", LeaseUntil: time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)}
	const billingID = "billing-timeout-cleanup"
	state.seedGenerating(taskID, billingID, "confirm-timeout-cleanup", pptAgentConfirmHash(task), nil, lease)
	billing := newPPTAgentBillingTestStore(events)
	billing.seed(generationTask{ID: billingID, ClientRequestID: "ppt-confirm:" + taskID, UserID: userID,
		Status: "PROCESSING", TaskStatus: taskStatusQueued, BillingStatus: billingStatusReserved})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runPPTAgentGeneration(ctx, pptAgentGenerationRun{
		state: state, billing: billing, owner: pptAgentTestOwner(userID), taskID: taskID,
		claim:       pptapp.GenerationClaim{RunToken: lease.RunToken, LeaseUntil: lease.LeaseUntil},
		billingTask: generationTask{ID: billingID, BillingStatus: billingStatusReserved},
	})

	failed, err := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, captureEffects, _, releaseEffects := billing.counts()
	if failed.Stage != pptapp.StageFailed || state.failedWrites != 1 || captureEffects != 0 || releaseEffects != 1 {
		t.Fatalf("cancelled-context cleanup task=%#v failedWrites=%d capture=%d release=%d", failed, state.failedWrites, captureEffects, releaseEffects)
	}
	if !events.before("billing:lookup", "billing:release") || !events.before("billing:release", "state:failed") {
		t.Fatalf("cancelled-context cleanup order = %#v", events.snapshot())
	}
}

func TestPPTGenerationCleanupReleaseFailureStaysRetryableWithoutDuplicateEffects(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, userID := preparePPTAgentOutlineReadyTask(t, harness, 1)
	state := newPPTAgentGenerationTestState(harness.state, nil)
	task, _ := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	lease := &pptapp.GenerationLease{RunToken: "run-release-cleanup", LeaseUntil: time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)}
	const billingID = "billing-release-cleanup"
	state.seedGenerating(taskID, billingID, "confirm-release-cleanup", pptAgentConfirmHash(task), nil, lease)
	billing := newPPTAgentBillingTestStore(nil)
	billing.seed(generationTask{ID: billingID, ClientRequestID: "ppt-confirm:" + taskID, UserID: userID,
		Status: "PROCESSING", TaskStatus: taskStatusQueued, BillingStatus: billingStatusReserved})
	run := pptAgentGenerationRun{
		state: state, billing: billing, owner: pptAgentTestOwner(userID), taskID: taskID,
		claim:       pptapp.GenerationClaim{RunToken: lease.RunToken, LeaseUntil: lease.LeaseUntil},
		billingTask: generationTask{ID: billingID, BillingStatus: billingStatusReserved},
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	billing.releaseErr = errors.New("temporary release failure")
	runPPTAgentGeneration(cancelled, run)
	stillGenerating, _ := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	if stillGenerating.Stage != pptapp.StageGenerating || state.failedWrites != 0 {
		t.Fatalf("release failure wrote terminal state: task=%#v failedWrites=%d", stillGenerating, state.failedWrites)
	}

	billing.releaseErr = nil
	runPPTAgentGeneration(cancelled, run)
	runPPTAgentGeneration(cancelled, run)
	failed, _ := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	_, _, _, captureEffects, _, releaseEffects := billing.counts()
	if failed.Stage != pptapp.StageFailed || state.failedWrites != 1 || captureEffects != 0 || releaseEffects != 1 {
		t.Fatalf("cleanup replay task=%#v failedWrites=%d capture=%d release=%d", failed, state.failedWrites, captureEffects, releaseEffects)
	}
}

func TestPPTGenerationCleanupAuthoritativeCaptureWinsWithoutRelease(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, userID := preparePPTAgentOutlineReadyTask(t, harness, 1)
	events := &pptAgentGenerationTestEvents{}
	state := newPPTAgentGenerationTestState(harness.state, events)
	task, _ := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	lease := &pptapp.GenerationLease{RunToken: "run-capture-cleanup", LeaseUntil: time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)}
	slide := canonicalPPTAgentGeneratedSlide(pptapp.SlideFromOutline(task.Outline.Slides[0], pptAgentGenerateRequest(task)), task.TenantID)
	const billingID = "billing-capture-cleanup"
	state.seedGenerating(taskID, billingID, "confirm-capture-cleanup", pptAgentConfirmHash(task), []pptapp.Slide{slide}, lease)
	billing := newPPTAgentBillingTestStore(events)
	billing.seed(generationTask{ID: billingID, ClientRequestID: "ppt-confirm:" + taskID, UserID: userID,
		Status: "SUCCEEDED", TaskStatus: taskStatusSucceeded, BillingStatus: billingStatusCaptured})
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	runPPTAgentGeneration(cancelled, pptAgentGenerationRun{
		state: state, billing: billing, owner: pptAgentTestOwner(userID), taskID: taskID,
		claim: pptapp.GenerationClaim{RunToken: lease.RunToken, LeaseUntil: lease.LeaseUntil},
		// The captured state exists only in the authoritative store. The stale
		// in-memory run snapshot must never be allowed to release it.
		billingTask: generationTask{ID: billingID, BillingStatus: billingStatusReserved},
	})
	ready, _ := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	_, _, _, _, _, releaseEffects := billing.counts()
	if ready.Stage != pptapp.StageReady || state.readyWrites != 1 || state.failedWrites != 0 || releaseEffects != 0 {
		t.Fatalf("capture-won cleanup task=%#v ready=%d failed=%d release=%d", ready, state.readyWrites, state.failedWrites, releaseEffects)
	}
	if !events.before("billing:lookup", "state:ready") {
		t.Fatalf("capture-won cleanup order = %#v", events.snapshot())
	}
}

func TestPPTGenerationExpiredRunCleanupCannotReleaseSuccessorReservation(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, userID := preparePPTAgentOutlineReadyTask(t, harness, 1)
	events := &pptAgentGenerationTestEvents{}
	workerAState := newPPTAgentGenerationTestState(harness.state, events)
	workerBState := newPPTAgentGenerationTestState(harness.state, events)
	task, err := workerAState.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	runA := pptapp.GenerationClaim{RunToken: "run-expired-a", LeaseUntil: now.Add(-time.Minute).Format(time.RFC3339Nano)}
	const billingID = "billing-successor-lease"
	workerAState.seedGenerating(taskID, billingID, "confirm-successor-lease", pptAgentConfirmHash(task), nil, &pptapp.GenerationLease{
		RunToken: runA.RunToken, LeaseUntil: runA.LeaseUntil,
	})
	billing := newPPTAgentBillingTestStore(events)
	reserved := generationTask{ID: billingID, ClientRequestID: "ppt-confirm:" + taskID, UserID: userID,
		Status: "PROCESSING", TaskStatus: taskStatusQueued, BillingStatus: billingStatusReserved}
	billing.seed(reserved)

	runB, beforeCleanup, err := workerBState.ClaimGenerationRun(context.Background(), pptAgentTestOwner(userID), taskID, now)
	if err != nil {
		t.Fatalf("run B ClaimGenerationRun() error = %v", err)
	}
	if runB.RunToken == runA.RunToken {
		t.Fatalf("run B token = %q, want replacement for %q", runB.RunToken, runA.RunToken)
	}
	beforeJSON, err := json.Marshal(beforeCleanup)
	if err != nil {
		t.Fatal(err)
	}

	cleanupPPTAgentGeneration(pptAgentGenerationRun{
		state: workerAState, billing: billing, owner: pptAgentTestOwner(userID), taskID: taskID,
		claim: runA, billingTask: reserved,
	}, "PPT_GENERATION_FAILED")

	afterCleanup, err := workerBState.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	afterJSON, err := json.Marshal(afterCleanup)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, captureEffects, failCalls, releaseEffects := billing.counts()
	if string(afterJSON) != string(beforeJSON) || afterCleanup.GenerationLease == nil || afterCleanup.GenerationLease.RunToken != runB.RunToken {
		t.Fatalf("run A cleanup mutated run B state\nbefore=%s\nafter=%s", beforeJSON, afterJSON)
	}
	if failCalls != 0 || releaseEffects != 0 || captureEffects != 0 || workerAState.failedWrites != 0 || workerAState.readyWrites != 0 {
		t.Fatalf("run A cleanup effects: billing fail calls=%d release=%d capture=%d state failed=%d ready=%d", failCalls, releaseEffects, captureEffects, workerAState.failedWrites, workerAState.readyWrites)
	}

	runPPTAgentGeneration(context.Background(), pptAgentGenerationRun{
		state: workerBState, billing: billing, owner: pptAgentTestOwner(userID), taskID: taskID,
		claim: runB, billingTask: reserved,
	})
	ready, err := workerBState.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	_, _, completeCalls, captureEffects, failCalls, releaseEffects := billing.counts()
	if ready.Stage != pptapp.StageReady || workerBState.readyWrites != 1 || workerBState.failedWrites != 0 {
		t.Fatalf("run B completion task=%#v ready=%d failed=%d", ready, workerBState.readyWrites, workerBState.failedWrites)
	}
	if completeCalls != 1 || captureEffects != 1 || failCalls != 0 || releaseEffects != 0 {
		t.Fatalf("run B billing effects: complete=%d capture=%d fail calls=%d release=%d", completeCalls, captureEffects, failCalls, releaseEffects)
	}
}

func TestPPTGenerationCleanupFenceBlocksLeaseTakeoverUntilReleaseSettles(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, userID := preparePPTAgentOutlineReadyTask(t, harness, 1)
	events := &pptAgentGenerationTestEvents{}
	workerAState := newPPTAgentGenerationTestState(harness.state, events)
	workerBState := newPPTAgentGenerationTestState(harness.state, events)
	task, err := workerAState.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	runA := pptapp.GenerationClaim{RunToken: "run-expired-cleanup-a", LeaseUntil: now.Add(-time.Minute).Format(time.RFC3339Nano)}
	const billingID = "billing-cleanup-fence"
	workerAState.seedGenerating(taskID, billingID, "confirm-cleanup-fence", pptAgentConfirmHash(task), nil, &pptapp.GenerationLease{
		RunToken: runA.RunToken, LeaseUntil: runA.LeaseUntil,
	})
	billing := newPPTAgentBillingTestStore(events)
	reserved := generationTask{ID: billingID, ClientRequestID: "ppt-confirm:" + taskID, UserID: userID,
		Status: "PROCESSING", TaskStatus: taskStatusQueued, BillingStatus: billingStatusReserved}
	billing.seed(reserved)

	fenceEntered := make(chan struct{})
	continueCleanup := make(chan struct{})
	interleavingState := &pptAgentCleanupInterleavingState{
		pptAgentGenerationState: workerAState,
		fenceEntered:            fenceEntered,
		continueCleanup:         continueCleanup,
	}
	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		cleanupPPTAgentGeneration(pptAgentGenerationRun{
			state: interleavingState, billing: billing, owner: pptAgentTestOwner(userID), taskID: taskID,
			claim: runA, billingTask: reserved,
		}, "PPT_GENERATION_FAILED")
	}()

	resumed := false
	defer func() {
		if !resumed {
			close(continueCleanup)
		}
	}()
	select {
	case <-fenceEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("cleanup did not reach the pre-billing interleaving point")
	}
	runB, _, claimErr := workerBState.ClaimGenerationRun(context.Background(), pptAgentTestOwner(userID), taskID, now)
	close(continueCleanup)
	resumed = true
	select {
	case <-cleanupDone:
	case <-time.After(3 * time.Second):
		t.Fatal("cleanup did not finish after the billing window resumed")
	}

	failed, err := workerBState.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, captureEffects, failCalls, releaseEffects := billing.counts()
	if !interleavingState.cleanupFenceAcquired() || !errors.Is(claimErr, pptapp.ErrGenerationAlreadyRunning) || strings.TrimSpace(runB.RunToken) != "" {
		t.Fatalf("cleanup fence=%v; run B claim=%#v err=%v; task stage=%s; billing fail calls=%d release=%d", interleavingState.cleanupFenceAcquired(), runB, claimErr, failed.Stage, failCalls, releaseEffects)
	}
	if failed.Stage != pptapp.StageFailed || failed.GenerationLease != nil || workerAState.failedWrites != 1 || workerBState.readyWrites != 0 {
		t.Fatalf("cleanup-fenced terminal state task=%#v workerA failed=%d workerB ready=%d", failed, workerAState.failedWrites, workerBState.readyWrites)
	}
	if captureEffects != 0 || failCalls != 1 || releaseEffects != 1 {
		t.Fatalf("cleanup-fenced billing effects: capture=%d fail calls=%d release=%d", captureEffects, failCalls, releaseEffects)
	}
	if !events.before("state:cleanup-fence", "billing:release") || !events.before("billing:release", "state:failed") {
		t.Fatalf("cleanup fence/release/state order = %#v", events.snapshot())
	}
}

func TestLegacyPPTGenerateUsesPersistedAgentStateAndSingleBillingLifecycle(t *testing.T) {
	if skill, ok := pptskills.Resolve("general"); !ok || skill.Code != "general" || skill.MaxSlides != 30 {
		t.Fatalf("default legacy skill is not available in the current catalog: %#v found=%v", skill, ok)
	}
	harness := newPPTAgentHTTPHarness(t)
	events := &pptAgentGenerationTestEvents{}
	state := newPPTAgentGenerationTestState(harness.state, events)
	billing := newPPTAgentBillingTestStore(events)
	reserveSawPersistedOutline := false
	billing.beforeReserve = func(request createGenerationTaskRequest) {
		taskID := strings.TrimSpace(stringValue(request.Params["source_task_id"]))
		task, err := harness.state.GetTask(context.Background(), pptapp.OwnerScope{
			TenantID: strings.TrimSpace(stringValue(request.Params["tenant_id"])),
			UserID:   request.UserID,
		}, taskID)
		reserveSawPersistedOutline = err == nil && task.Stage == pptapp.StageOutlineReady && task.Outline != nil && len(task.Outline.Slides) == 2
	}
	before, err := harness.store.AdminData()
	if err != nil {
		t.Fatal(err)
	}

	response := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost, "/api/v1/ppt/generate", `{
		"prompt":"Legacy persisted Agent deck",
		"slideCount":2,
		"language":"en",
		"tone":"pitch",
		"textContent":"detailed",
		"audience":"investor",
		"scenario":"analysis-report",
		"generationAspectRatio":"16:9",
		"theme":"legacy-theme",
		"autoThemeEnabled":true,
		"enableWebSearch":true,
			"imageSource":"ai",
			"textModel":"kimi-k2.6",
			"imageModel":"gpt-image-2",
		"imageStyle":"editorial isometric",
		"peopleStyle":"natural professionals",
		"imageLighting":"warm studio",
		"imageComposition":"image_left",
			"textInImage":false,
		"outline":{"title":"Legacy deck","slides":[
			{"page":1,"title":"Opening","summary":"Set the context","bulletPoints":["Audience","Goal"],"layout":"cover","slideType":"cover"},
			{"page":2,"title":"Decision","summary":"Make the decision","bulletPoints":["Evidence","Action"],"layout":"summary","slideType":"statement"}
		]}
	}`, "legacy-generate")
	if response.Code != http.StatusOK {
		t.Fatalf("legacy generate status = %d, body = %s", response.Code, response.Body.String())
	}
	var created pptapp.GenerateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.TaskID == "" || harness.state.createCalls != 1 {
		t.Fatalf("legacy response/state = %#v create calls=%d", created, harness.state.createCalls)
	}
	ready := waitForPPTAgentStage(t, state, created.TaskID, pptapp.StageReady)
	if ready.SessionID != created.TaskID || ready.Progress != 100 || ready.CurrentPage != 2 || len(ready.Slides) != 2 || ready.Outline == nil || len(ready.Outline.Slides) != 2 {
		t.Fatalf("legacy persisted task = %#v", ready)
	}
	if ready.SkillCode != "general" || ready.Language != "en" || ready.Audience != "investor" {
		t.Fatalf("legacy adapter did not preserve canonical session fields: %#v", ready)
	}
	if ready.Tone != "pitch" || ready.TextContent != "detailed" || ready.Scenario != "analysis-report" ||
		ready.GenerationAspectRatio != "16:9" || ready.Theme != "legacy-theme" || !ready.AutoThemeEnabled || !ready.EnableWebSearch ||
		ready.ImageSource != "ai" || ready.TextModel != "kimi-k2.6" || ready.ImageModel != "gpt-image-2" ||
		ready.ImageStyle != "editorial isometric" || ready.PeopleStyle != "natural professionals" ||
		ready.ImageLighting != "warm studio" || ready.ImageComposition != "image_left" || ready.TextInImage {
		t.Fatalf("legacy adapter lost canonical DeckSpec after generation: %#v", ready)
	}
	for _, slide := range ready.Slides {
		if slide.Title != "" || slide.Content != "" || len(slide.BulletPoints) != 0 || slide.ImageURL != "" || slide.SpeakerNotes != "" || len(slide.Blocks) == 0 {
			t.Fatalf("legacy adapter persisted dual slide IR: %#v", slide)
		}
		for _, block := range slide.Blocks {
			if block.Type == "image" && !strings.HasPrefix(block.ImageRef, "storage://tenant_default/") {
				t.Fatalf("legacy adapter persisted untrusted image reference: %#v", block)
			}
		}
	}
	if !reserveSawPersistedOutline {
		t.Fatal("legacy billing reserve ran before the persisted outline reached OUTLINE_READY")
	}
	createCalls, reserveEffects, completeCalls, captureEffects, _, releaseEffects := billing.counts()
	if createCalls != 1 || reserveEffects != 1 || completeCalls != 1 || captureEffects != 1 || releaseEffects != 0 || state.runClaims != 1 || state.readyWrites != 1 {
		t.Fatalf("legacy billing/state counts create=%d reserve=%d complete=%d capture=%d release=%d runs=%d ready=%d", createCalls, reserveEffects, completeCalls, captureEffects, releaseEffects, state.runClaims, state.readyWrites)
	}
	billingRequest := billing.lastCreateRequest()
	if billingRequest.ClientRequestID != "ppt-confirm:"+created.TaskID || stringValue(billingRequest.Params["source_type"]) != "ppt_agent" || stringValue(billingRequest.Params["source_task_id"]) != created.TaskID {
		t.Fatalf("legacy billing request = %#v", billingRequest)
	}
	if withImages, ok := billingRequest.Params["with_images"].(bool); !ok || !withImages {
		t.Fatalf("legacy billing lost persisted image intent: %#v", billingRequest.Params)
	}
	after, err := harness.store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.BillingEvents) != len(before.BillingEvents) {
		t.Fatalf("legacy route bypassed injected billing lifecycle: before=%d after=%d", len(before.BillingEvents), len(after.BillingEvents))
	}
}

func TestLegacyPPTGenerateWithoutProviderFailsClosedBeforeBilling(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	state := newPPTAgentGenerationTestState(harness.state, nil)
	billing := newPPTAgentBillingTestStore(nil)

	response := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost, "/api/v1/ppt/generate", `{
		"prompt":"Provider-generated outline is required",
		"slideCount":2,
		"imageSource":"none"
	}`, "legacy-provider-required")
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "PPT_AGENT_PROVIDER_UNAVAILABLE") {
		t.Fatalf("legacy provider-unavailable status = %d, body = %s", response.Code, response.Body.String())
	}
	createCalls, reserveEffects, completeCalls, captureEffects, failCalls, releaseEffects := billing.counts()
	if createCalls != 0 || reserveEffects != 0 || completeCalls != 0 || captureEffects != 0 || failCalls != 0 || releaseEffects != 0 || billing.taskCount() != 0 {
		t.Fatalf("provider-unavailable request reached billing: create=%d reserve=%d complete=%d capture=%d fail=%d release=%d tasks=%d", createCalls, reserveEffects, completeCalls, captureEffects, failCalls, releaseEffects, billing.taskCount())
	}
	if harness.state.createCalls != 0 || state.generationClaimCalls != 0 || state.runClaims != 0 {
		t.Fatalf("provider-unavailable request persisted or generated: sessions=%d claims=%d runs=%d", harness.state.createCalls, state.generationClaimCalls, state.runClaims)
	}
}

func TestPPTBillingCaptureDoesNotCreateSyntheticImageAsset(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	data, err := harness.store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	var user adminUser
	for _, candidate := range data.Users {
		if strings.EqualFold(candidate.Email, "demo@xianzhi.ai") {
			user = candidate
			break
		}
	}
	if user.ID == "" {
		t.Fatal("demo user not found")
	}
	account, err := harness.store.PointAccount(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.store.PersonalPointService().Grant(context.Background(), PersonalPointGrantCommand{
		AccountID: account.ID, UserID: user.ID, Source: PointSourceRecharge, Points: 10,
		ReferenceType: "TEST", ReferenceID: "ppt-no-synthetic-asset", IdempotencyKey: "ppt-no-synthetic-asset",
	}); err != nil {
		t.Fatal(err)
	}
	capability, err := (api{store: harness.authorizations}).preparePPTCapabilityRequest(data, user, "asset-free capture", "", 2, false, false)
	if err != nil {
		t.Fatalf("prepare capability: %v", err)
	}
	request := pptAgentConfirmBillingRequest(pptapp.Task{TaskID: "ppt-no-synthetic-asset", UserID: user.ID, Prompt: "asset-free capture"}, capability, 2)
	before, err := harness.store.ListAssets()
	if err != nil {
		t.Fatal(err)
	}
	pending, err := harness.store.CreatePendingGenerationTask(request)
	if err != nil {
		t.Fatalf("reserve PPT billing: %v", err)
	}
	if _, err := harness.store.CompleteGenerationTask(pending.ID, request); err != nil {
		t.Fatalf("capture PPT billing: %v", err)
	}
	after, err := harness.store.ListAssets()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("PPT billing capture created %d synthetic assets", len(after)-len(before))
	}
}

func TestPPTConfirmInsufficientBalanceDoesNotEnterGenerating(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, _ := preparePPTAgentOutlineReadyTask(t, harness, 2)
	state := newPPTAgentGenerationTestState(harness.state, nil)
	billing := newPPTAgentBillingTestStore(nil)
	billing.reserveErr = errors.New("insufficient remaining points: available 0, required 2")

	response := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-insufficient")
	if response.Code != http.StatusPaymentRequired || !strings.Contains(response.Body.String(), "PPT_BILLING_RESERVATION_FAILED") {
		t.Fatalf("confirm status = %d, body = %s", response.Code, response.Body.String())
	}
	task, err := state.GetTask(context.Background(), harness.state.ownerScope(taskID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Stage != pptapp.StageOutlineReady || task.Status != pptapp.StatusPending {
		t.Fatalf("task entered generation after failed reserve: stage=%s status=%s", task.Stage, task.Status)
	}
	if state.beginGenerationCalls != 0 || state.runClaims != 0 {
		t.Fatalf("state calls after failed reserve = begin %d run %d", state.beginGenerationCalls, state.runClaims)
	}
}

func TestPPTConfirmConcurrentReplayCreatesOneBillingTaskAndRun(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, _ := preparePPTAgentOutlineReadyTask(t, harness, 3)
	state := newPPTAgentGenerationTestState(harness.state, nil)
	billing := newPPTAgentBillingTestStore(nil)
	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, 2)
	for index := 0; index < 2; index++ {
		go func() {
			<-start
			responses <- requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
				"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-concurrent")
		}()
	}
	close(start)
	for index := 0; index < 2; index++ {
		response := <-responses
		if response.Code != http.StatusOK {
			t.Fatalf("concurrent confirm status = %d, body = %s", response.Code, response.Body.String())
		}
	}
	waitForPPTAgentStage(t, state, taskID, pptapp.StageReady)
	_, reserveEffects, _, captureEffects, _, _ := billing.counts()
	if reserveEffects != 1 || captureEffects != 1 || billing.taskCount() != 1 {
		t.Fatalf("billing effects = reserve %d capture %d tasks %d", reserveEffects, captureEffects, billing.taskCount())
	}
	if state.runClaims != 1 {
		t.Fatalf("generation run claims = %d, want 1", state.runClaims)
	}
}

func TestPPTGenerationProgressUsesCompletedSlides(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, _ := preparePPTAgentOutlineReadyTask(t, harness, 3)
	state := newPPTAgentGenerationTestState(harness.state, nil)
	state.persistedPages = make(chan int, 3)
	state.continuePages = make(chan struct{}, 3)
	billing := newPPTAgentBillingTestStore(nil)

	response := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-progress")
	if response.Code != http.StatusOK {
		t.Fatalf("confirm status = %d, body = %s", response.Code, response.Body.String())
	}
	assertPPTAgentPersistedProgress(t, state, taskID, 1, 33)
	state.continuePages <- struct{}{}
	assertPPTAgentPersistedProgress(t, state, taskID, 2, 66)
	state.continuePages <- struct{}{}
	assertPPTAgentPersistedProgress(t, state, taskID, 3, 100)
	state.continuePages <- struct{}{}
	waitForPPTAgentStage(t, state, taskID, pptapp.StageReady)
}

func TestPPTConfirmAfterRestartResumesOnlyMissingSlides(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, userID := preparePPTAgentOutlineReadyTask(t, harness, 3)
	state := newPPTAgentGenerationTestState(harness.state, nil)
	task, err := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	billingID := "billing-resume"
	requestHash := pptAgentConfirmHash(task)
	firstSlide := pptapp.SlideFromOutline(task.Outline.Slides[0], pptAgentGenerateRequest(task))
	state.seedGenerating(taskID, billingID, "ppt-confirm:"+taskID, requestHash, []pptapp.Slide{firstSlide}, nil)
	billing := newPPTAgentBillingTestStore(nil)
	billing.seed(generationTask{
		ID: billingID, ClientRequestID: "ppt-confirm:" + taskID, UserID: userID, Type: "PPT_GENERATION",
		ModuleCode: modulePPTGeneration, Status: "PROCESSING", TaskStatus: taskStatusQueued, BillingStatus: billingStatusReserved,
	})

	response := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-resume")
	if response.Code != http.StatusOK {
		t.Fatalf("resume status = %d, body = %s", response.Code, response.Body.String())
	}
	ready := waitForPPTAgentStage(t, state, taskID, pptapp.StageReady)
	if got := state.persistedPageSnapshot(); fmt.Sprint(got) != "[2 3]" {
		t.Fatalf("persisted pages after restart = %v, want [2 3]", got)
	}
	if len(ready.Slides) != 3 || ready.Slides[0].ID != firstSlide.ID || ready.Slides[0].Page != 1 {
		t.Fatalf("resumed slides = %#v", ready.Slides)
	}
	_, reserveEffects, _, captureEffects, _, _ := billing.counts()
	if reserveEffects != 1 || captureEffects != 1 || billing.taskCount() != 1 {
		t.Fatalf("resume billing = reserve %d capture %d tasks %d", reserveEffects, captureEffects, billing.taskCount())
	}
}

func TestPPTCancelReleasesReservationAndBlocksReadyWrite(t *testing.T) {
	t.Run("successful release cancels and stale run cannot write ready", func(t *testing.T) {
		harness := newPPTAgentHTTPHarness(t)
		taskID, _ := preparePPTAgentOutlineReadyTask(t, harness, 3)
		state := newPPTAgentGenerationTestState(harness.state, nil)
		state.persistedPages = make(chan int, 3)
		state.continuePages = make(chan struct{}, 3)
		billing := newPPTAgentBillingTestStore(nil)

		confirm := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
			"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-cancel")
		if confirm.Code != http.StatusOK {
			t.Fatalf("confirm status = %d, body = %s", confirm.Code, confirm.Body.String())
		}
		select {
		case page := <-state.persistedPages:
			if page != 1 {
				t.Fatalf("first persisted page = %d", page)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for first generated page")
		}
		cancel := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
			"/api/v1/ppt/sessions/"+taskID+"/cancel", `{}`, "cancel-once")
		if cancel.Code != http.StatusOK {
			t.Fatalf("cancel status = %d, body = %s", cancel.Code, cancel.Body.String())
		}
		state.continuePages <- struct{}{}
		cancelled := waitForPPTAgentStage(t, state, taskID, pptapp.StageCancelled)
		if cancelled.Progress != 33 || cancelled.CurrentPage != 1 || state.readyWrites != 0 {
			t.Fatalf("cancelled task = progress %d page %d ready writes %d", cancelled.Progress, cancelled.CurrentPage, state.readyWrites)
		}
		_, _, _, captureEffects, _, releaseEffects := billing.counts()
		if captureEffects != 0 || releaseEffects != 1 {
			t.Fatalf("cancel billing = captures %d releases %d", captureEffects, releaseEffects)
		}
	})

	t.Run("release failure stays retryable and does not claim cancelled", func(t *testing.T) {
		harness := newPPTAgentHTTPHarness(t)
		taskID, userID := preparePPTAgentOutlineReadyTask(t, harness, 2)
		state := newPPTAgentGenerationTestState(harness.state, nil)
		task, _ := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
		state.seedGenerating(taskID, "billing-release-retry", "confirm-release-retry", pptAgentConfirmHash(task), nil,
			&pptapp.GenerationLease{RunToken: "run-release-retry", LeaseUntil: time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)})
		billing := newPPTAgentBillingTestStore(nil)
		billing.seed(generationTask{ID: "billing-release-retry", ClientRequestID: "ppt-confirm:" + taskID, UserID: userID,
			Status: "PROCESSING", TaskStatus: taskStatusQueued, BillingStatus: billingStatusReserved})
		billing.releaseErr = errors.New("temporary release failure")

		first := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
			"/api/v1/ppt/sessions/"+taskID+"/cancel", `{}`, "cancel-release-retry")
		if first.Code != http.StatusServiceUnavailable || !strings.Contains(first.Body.String(), "PPT_BILLING_FINALIZE_FAILED") {
			t.Fatalf("release failure status = %d, body = %s", first.Code, first.Body.String())
		}
		stillGenerating, _ := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
		if stillGenerating.Stage != pptapp.StageGenerating || state.cancelWrites != 0 {
			t.Fatalf("release failure wrote terminal state: stage=%s cancel writes=%d", stillGenerating.Stage, state.cancelWrites)
		}
		billing.releaseErr = nil
		second := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
			"/api/v1/ppt/sessions/"+taskID+"/cancel", `{}`, "cancel-release-retry")
		if second.Code != http.StatusOK {
			t.Fatalf("release retry status = %d, body = %s", second.Code, second.Body.String())
		}
		waitForPPTAgentStage(t, state, taskID, pptapp.StageCancelled)
		_, _, _, _, _, releaseEffects := billing.counts()
		if releaseEffects != 1 {
			t.Fatalf("release effects = %d, want 1", releaseEffects)
		}
	})
}

func TestPPTCancelCrossInstanceReleaseWins(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, userID := preparePPTAgentOutlineReadyTask(t, harness, 1)
	state := newPPTAgentGenerationTestState(harness.state, nil)
	task, _ := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	slide := pptapp.SlideFromOutline(task.Outline.Slides[0], pptAgentGenerateRequest(task))
	lease := &pptapp.GenerationLease{RunToken: "run-cross-release", LeaseUntil: time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)}
	state.seedGenerating(taskID, "billing-cross-release", "confirm-cross-release", pptAgentConfirmHash(task), []pptapp.Slide{slide}, lease)
	billing := newPPTAgentBillingTestStore(nil)
	reserved := generationTask{ID: "billing-cross-release", ClientRequestID: "ppt-confirm:" + taskID, UserID: userID,
		Status: "PROCESSING", TaskStatus: taskStatusQueued, BillingStatus: billingStatusReserved}
	billing.seed(reserved)

	first := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/cancel", `{}`, "cancel-cross-release")
	if first.Code != http.StatusOK {
		t.Fatalf("release-won cancel status = %d, body = %s", first.Code, first.Body.String())
	}
	runPPTAgentGeneration(context.Background(), pptAgentGenerationRun{
		state: state, billing: billing, owner: pptAgentTestOwner(userID), taskID: taskID,
		claim: pptapp.GenerationClaim{RunToken: lease.RunToken, LeaseUntil: lease.LeaseUntil}, billingTask: reserved,
	})
	replay := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/cancel", `{}`, "cancel-cross-release")
	if replay.Code != http.StatusOK {
		t.Fatalf("release-won replay status = %d, body = %s", replay.Code, replay.Body.String())
	}
	final, _ := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	_, _, completeCalls, captureEffects, failCalls, releaseEffects := billing.counts()
	if final.Stage != pptapp.StageCancelled || state.readyWrites != 0 || completeCalls != 0 || captureEffects != 0 || failCalls != 1 || releaseEffects != 1 {
		t.Fatalf("release-won final=%s ready=%d complete=%d capture=%d fail=%d release=%d", final.Stage, state.readyWrites, completeCalls, captureEffects, failCalls, releaseEffects)
	}
}

func TestPPTCancelCrossInstanceCaptureWins(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, userID := preparePPTAgentOutlineReadyTask(t, harness, 1)
	state := newPPTAgentGenerationTestState(harness.state, nil)
	task, _ := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	slide := pptapp.SlideFromOutline(task.Outline.Slides[0], pptAgentGenerateRequest(task))
	lease := &pptapp.GenerationLease{RunToken: "run-cross-capture", LeaseUntil: time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)}
	state.seedGenerating(taskID, "billing-cross-capture", "confirm-cross-capture", pptAgentConfirmHash(task), []pptapp.Slide{slide}, lease)
	billing := newPPTAgentBillingTestStore(nil)
	request := createGenerationTaskRequest{UserID: userID, ClientRequestID: "ppt-confirm:" + taskID, Type: "PPT_GENERATION", ModuleCode: modulePPTGeneration}
	billing.seed(generationTask{ID: "billing-cross-capture", ClientRequestID: request.ClientRequestID, UserID: userID,
		Status: "PROCESSING", TaskStatus: taskStatusQueued, BillingStatus: billingStatusReserved})
	if captured, err := billing.CompleteGenerationTask("billing-cross-capture", request); err != nil || !pptAgentBillingCaptured(captured) {
		t.Fatalf("capture fixture task=%#v err=%v", captured, err)
	}

	first := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/cancel", `{}`, "cancel-cross-capture")
	if first.Code != http.StatusConflict || !strings.Contains(first.Body.String(), "PPT_BILLING_ALREADY_CAPTURED") {
		t.Fatalf("capture-won cancel status = %d, body = %s", first.Code, first.Body.String())
	}
	replay := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/cancel", `{}`, "cancel-cross-capture")
	if replay.Code != http.StatusConflict || !strings.Contains(replay.Body.String(), "PPT_BILLING_ALREADY_CAPTURED") {
		t.Fatalf("capture-won replay status = %d, body = %s", replay.Code, replay.Body.String())
	}
	final, _ := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	cancelRecord, ok := findPPTAgentTestRecordValue(final.IdempotencyRecords, "cancel", "cancel-cross-capture")
	_, _, completeCalls, captureEffects, failCalls, releaseEffects := billing.counts()
	if final.Stage != pptapp.StageReady || !ok || cancelRecord.State != "failed" || cancelRecord.ErrorCode != pptapp.ErrBillingAlreadyCaptured.Error() {
		t.Fatalf("capture-won final=%s cancel=%#v found=%v", final.Stage, cancelRecord, ok)
	}
	if completeCalls != 1 || captureEffects != 1 || failCalls != 0 || releaseEffects != 0 || state.readyWrites != 1 {
		t.Fatalf("capture-won complete=%d capture=%d fail=%d release=%d ready=%d", completeCalls, captureEffects, failCalls, releaseEffects, state.readyWrites)
	}
}

func TestPPTGenerationCancelClaimObservedAfterPagesPreventsCapture(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, userID := preparePPTAgentOutlineReadyTask(t, harness, 1)
	events := &pptAgentGenerationTestEvents{}
	base := newPPTAgentGenerationTestState(harness.state, events)
	task, err := base.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	lease := &pptapp.GenerationLease{RunToken: "run-cancel-before-capture", LeaseUntil: time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)}
	const billingID = "billing-cancel-before-capture"
	base.seedGenerating(taskID, billingID, "confirm-cancel-before-capture", pptAgentConfirmHash(task), nil, lease)
	state := &pptAgentCancelBeforeCaptureState{pptAgentGenerationState: base}
	billing := newPPTAgentBillingTestStore(events)
	billing.seed(generationTask{
		ID: billingID, ClientRequestID: "ppt-confirm:" + taskID, UserID: userID,
		Status: "PROCESSING", TaskStatus: taskStatusQueued, BillingStatus: billingStatusReserved,
	})
	runPPTAgentGeneration(context.Background(), pptAgentGenerationRun{
		state: state, billing: billing, owner: pptAgentTestOwner(userID), taskID: taskID,
		claim:       pptapp.GenerationClaim{RunToken: lease.RunToken, LeaseUntil: lease.LeaseUntil},
		billingTask: generationTask{ID: billingID, BillingStatus: billingStatusReserved},
	})

	if state.cancelErr != nil || strings.TrimSpace(state.cancelClaim.OperationToken) == "" {
		t.Fatalf("post-page cancel injection claim=%#v err=%v", state.cancelClaim, state.cancelErr)
	}
	current, err := base.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	_, _, completeCalls, captureEffects, _, releaseEffects := billing.counts()
	if current.Stage != pptapp.StageGenerating || completeCalls != 0 || captureEffects != 0 || releaseEffects != 1 || base.readyWrites != 0 {
		t.Fatalf("post-page cancel task=%#v complete=%d capture=%d release=%d ready=%d", current, completeCalls, captureEffects, releaseEffects, base.readyWrites)
	}
	if !events.before("state:begin-cancel", "billing:release") {
		t.Fatalf("post-page cancel order = %#v", events.snapshot())
	}
	cancelled, err := base.CompleteCancel(context.Background(), pptAgentTestOwner(userID), taskID, state.cancelClaim)
	if err != nil || cancelled.Stage != pptapp.StageCancelled {
		t.Fatalf("complete post-page cancel task=%#v err=%v", cancelled, err)
	}
}

func TestPPTImageFailureDoesNotFailReadyDeck(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, _ := preparePPTAgentOutlineReadyTask(t, harness, 2)
	state := newPPTAgentGenerationTestState(harness.state, nil)
	billing := newPPTAgentBillingTestStore(nil)
	imageCalled := make(chan struct{}, 1)
	imageRunner := pptAgentImageRunner(func(context.Context, pptapp.Task) error {
		imageCalled <- struct{}{}
		return errors.New("image provider failed")
	})

	response := requestPPTAgentGeneration(harness, state, billing, imageRunner, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-image-failure")
	if response.Code != http.StatusOK {
		t.Fatalf("confirm status = %d, body = %s", response.Code, response.Body.String())
	}
	ready := waitForPPTAgentStage(t, state, taskID, pptapp.StageReady)
	select {
	case <-imageCalled:
	case <-time.After(3 * time.Second):
		t.Fatal("image runner was not invoked after text READY")
	}
	if ready.Stage != pptapp.StageReady || ready.Status != pptapp.StatusSuccess || state.failedWrites != 0 {
		t.Fatalf("image failure changed deck terminal state: %#v failed writes=%d", ready, state.failedWrites)
	}
}

func TestPPTBillingCaptureReplayIsIdempotent(t *testing.T) {
	t.Run("capture succeeded but ready write failed", func(t *testing.T) {
		harness := newPPTAgentHTTPHarness(t)
		taskID, _ := preparePPTAgentOutlineReadyTask(t, harness, 2)
		state := newPPTAgentGenerationTestState(harness.state, nil)
		state.completeFailures = 1
		state.completeAttempts = make(chan struct{}, 2)
		billing := newPPTAgentBillingTestStore(nil)

		first := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
			"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-ready-replay")
		if first.Code != http.StatusOK {
			t.Fatalf("first confirm status = %d, body = %s", first.Code, first.Body.String())
		}
		select {
		case <-state.completeAttempts:
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for injected READY write failure")
		}
		ready := waitForPPTAgentStage(t, state, taskID, pptapp.StageReady)
		if len(ready.Slides) != 2 || state.readyWrites != 1 {
			t.Fatalf("cleanup did not recover captured task after READY write failure: task=%#v readyWrites=%d", ready, state.readyWrites)
		}
		_, _, completeCalls, captureEffects, _, _ := billing.counts()
		if completeCalls != 1 || captureEffects != 1 {
			t.Fatalf("capture after first run = calls %d effects %d", completeCalls, captureEffects)
		}

		replay := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
			"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-ready-replay")
		if replay.Code != http.StatusOK {
			t.Fatalf("replay status = %d, body = %s", replay.Code, replay.Body.String())
		}
		waitForPPTAgentStage(t, state, taskID, pptapp.StageReady)
		_, _, completeCalls, captureEffects, _, _ = billing.counts()
		if completeCalls != 1 || captureEffects != 1 || state.runClaims != 1 {
			t.Fatalf("replay duplicated capture/run = complete calls %d capture effects %d runs %d", completeCalls, captureEffects, state.runClaims)
		}
	})

	t.Run("ambiguous capture result is recovered without release", func(t *testing.T) {
		harness := newPPTAgentHTTPHarness(t)
		taskID, _ := preparePPTAgentOutlineReadyTask(t, harness, 2)
		state := newPPTAgentGenerationTestState(harness.state, nil)
		billing := newPPTAgentBillingTestStore(nil)
		billing.captureErrAfterCommit = errors.New("commit result unavailable")

		response := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
			"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-capture-ambiguous")
		if response.Code != http.StatusOK {
			t.Fatalf("confirm status = %d, body = %s", response.Code, response.Body.String())
		}
		waitForPPTAgentStage(t, state, taskID, pptapp.StageReady)
		_, _, completeCalls, captureEffects, _, releaseEffects := billing.counts()
		if completeCalls != 1 || captureEffects != 1 || releaseEffects != 0 {
			t.Fatalf("ambiguous capture = complete calls %d capture effects %d release effects %d", completeCalls, captureEffects, releaseEffects)
		}
	})
}

func TestPPTConfirmTransientBindingFailureRetriesFreshAndReusesReservation(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, _ := preparePPTAgentOutlineReadyTask(t, harness, 2)
	state := newPPTAgentGenerationTestState(harness.state, nil)
	state.beginFailures = 1
	billing := newPPTAgentBillingTestStore(nil)

	first := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-binding-retry")
	if first.Code != http.StatusOK {
		t.Fatalf("first confirm status = %d, body = %s", first.Code, first.Body.String())
	}
	second := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-binding-retry")
	if second.Code != http.StatusOK {
		t.Fatalf("second confirm status = %d, body = %s", second.Code, second.Body.String())
	}
	waitForPPTAgentStage(t, state, taskID, pptapp.StageReady)
	_, reserveEffects, _, captureEffects, _, _ := billing.counts()
	if reserveEffects != 1 || captureEffects != 1 || billing.taskCount() != 1 {
		t.Fatalf("binding retry billing = reserve %d capture %d tasks %d", reserveEffects, captureEffects, billing.taskCount())
	}
	if state.beginGenerationCalls != 2 {
		t.Fatalf("binding attempts = %d, want initial plus fresh-context retry", state.beginGenerationCalls)
	}
}

func TestPPTConfirmPersistentBindingFailureReleasesAndClosesClaim(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, userID := preparePPTAgentOutlineReadyTask(t, harness, 2)
	events := &pptAgentGenerationTestEvents{}
	state := newPPTAgentGenerationTestState(harness.state, events)
	state.beginFailures = 2
	billing := newPPTAgentBillingTestStore(events)

	response := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-binding-fails")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "PPT_BILLING_FINALIZE_FAILED") {
		t.Fatalf("persistent binding failure status = %d, body = %s", response.Code, response.Body.String())
	}
	task, err := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	record, ok := findPPTAgentTestRecordValue(task.IdempotencyRecords, "confirm-outline", "ppt-confirm:"+taskID)
	if task.Stage != pptapp.StageOutlineReady || task.BillingTaskID != "" || !ok || record.State != "failed" || state.failedGenerationClaims != 1 || state.runClaims != 0 {
		t.Fatalf("persistent binding cleanup task=%#v record=%#v found=%v failedClaims=%d runs=%d", task, record, ok, state.failedGenerationClaims, state.runClaims)
	}
	_, reserveEffects, _, captureEffects, _, releaseEffects := billing.counts()
	if reserveEffects != 1 || captureEffects != 0 || releaseEffects != 1 {
		t.Fatalf("persistent binding cleanup reserve=%d capture=%d release=%d", reserveEffects, captureEffects, releaseEffects)
	}
	if !events.before("billing:lookup", "billing:release") || !events.before("billing:release", "state:fail-generation-claim") {
		t.Fatalf("persistent binding cleanup order = %#v", events.snapshot())
	}
	cancelled := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/cancel", `{}`, "cancel-after-binding-failure")
	if cancelled.Code != http.StatusOK {
		t.Fatalf("cancel after binding cleanup status = %d, body = %s", cancelled.Code, cancelled.Body.String())
	}
	final := waitForPPTAgentStage(t, state, taskID, pptapp.StageCancelled)
	if final.Stage != pptapp.StageCancelled {
		t.Fatalf("cancel after binding cleanup task=%#v", final)
	}
}

func TestPPTConfirmFailedTerminalDoesNotReserveOrRun(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, userID := preparePPTAgentOutlineReadyTask(t, harness, 2)
	state := newPPTAgentGenerationTestState(harness.state, nil)
	state.setStage(taskID, pptapp.StageFailed)
	billing := newPPTAgentBillingTestStore(nil)

	response := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-failed")
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "PPT_INVALID_STAGE") {
		t.Fatalf("failed confirm status = %d, body = %s", response.Code, response.Body.String())
	}
	task, _ := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	if task.Stage != pptapp.StageFailed || billing.taskCount() != 0 || state.runClaims != 0 {
		t.Fatalf("failed task was reused: stage=%s billing tasks=%d runs=%d", task.Stage, billing.taskCount(), state.runClaims)
	}
}

func TestPPTConfirmRejectsClientBillingBinding(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, _ := preparePPTAgentOutlineReadyTask(t, harness, 2)
	state := newPPTAgentGenerationTestState(harness.state, nil)
	billing := newPPTAgentBillingTestStore(nil)

	response := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{"billingTaskId":"external-task"}`, "confirm-client-billing")
	if response.Code != http.StatusBadRequest || billing.taskCount() != 0 || state.beginGenerationCalls != 0 {
		t.Fatalf("client billing binding status=%d body=%s billing=%d begin=%d", response.Code, response.Body.String(), billing.taskCount(), state.beginGenerationCalls)
	}
}

func TestPPTConfirmReserveBeforeBindCancelRaceReplaysExactlyOnce(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, userID := preparePPTAgentOutlineReadyTask(t, harness, 2)
	state := newPPTAgentGenerationTestState(harness.state, nil)
	state.bindEntered = make(chan struct{}, 1)
	state.continueBind = make(chan struct{})
	billing := newPPTAgentBillingTestStore(nil)
	billing.releaseErr = errors.New("injected release failure")

	confirmResponse := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		confirmResponse <- requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
			"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-prebind-race")
	}()
	select {
	case <-state.bindEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting after reserve and before billing bind")
	}

	cancel := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/cancel", `{}`, "cancel-prebind-race")
	close(state.continueBind)
	confirm := <-confirmResponse
	if cancel.Code != http.StatusConflict || !strings.Contains(cancel.Body.String(), "PPT_OPERATION_IN_PROGRESS") {
		t.Fatalf("pre-bind cancel status = %d, body = %s", cancel.Code, cancel.Body.String())
	}
	if confirm.Code != http.StatusOK {
		t.Fatalf("confirm status = %d, body = %s", confirm.Code, confirm.Body.String())
	}
	ready := waitForPPTAgentStage(t, state, taskID, pptapp.StageReady)
	if ready.Stage != pptapp.StageReady || ready.BillingTaskID == "" {
		t.Fatalf("ready task = %#v", ready)
	}

	replay := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-prebind-race")
	if replay.Code != http.StatusOK {
		t.Fatalf("confirm replay status = %d, body = %s", replay.Code, replay.Body.String())
	}
	final, _ := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	_, reserveEffects, completeCalls, captureEffects, failCalls, releaseEffects := billing.counts()
	if final.Stage != pptapp.StageReady || reserveEffects != 1 || completeCalls != 1 || captureEffects != 1 || failCalls != 0 || releaseEffects != 0 || state.runClaims != 1 {
		t.Fatalf("final=%s reserve=%d complete=%d capture=%d fail=%d release=%d runs=%d", final.Stage, reserveEffects, completeCalls, captureEffects, failCalls, releaseEffects, state.runClaims)
	}
}

func TestPPTConfirmReserveFailureClearsClaimForRetry(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, _ := preparePPTAgentOutlineReadyTask(t, harness, 1)
	state := newPPTAgentGenerationTestState(harness.state, nil)
	billing := newPPTAgentBillingTestStore(nil)
	billing.reserveErr = errors.New("insufficient remaining points")

	first := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-reserve-retry")
	if first.Code != http.StatusPaymentRequired {
		t.Fatalf("reserve failure status = %d, body = %s", first.Code, first.Body.String())
	}
	billing.reserveErr = nil
	second := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-reserve-retry")
	if second.Code != http.StatusOK {
		t.Fatalf("reserve retry status = %d, body = %s", second.Code, second.Body.String())
	}
	waitForPPTAgentStage(t, state, taskID, pptapp.StageReady)
	_, reserveEffects, _, captureEffects, _, releaseEffects := billing.counts()
	if state.generationClaimCalls != 2 || state.failedGenerationClaims != 1 || reserveEffects != 1 || captureEffects != 1 || releaseEffects != 0 {
		t.Fatalf("claims=%d failedClaims=%d reserve=%d capture=%d release=%d", state.generationClaimCalls, state.failedGenerationClaims, reserveEffects, captureEffects, releaseEffects)
	}
}

func TestPPTConfirmAmbiguousReserveRecoversWithoutDuplicate(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, _ := preparePPTAgentOutlineReadyTask(t, harness, 1)
	state := newPPTAgentGenerationTestState(harness.state, nil)
	billing := newPPTAgentBillingTestStore(nil)
	billing.reserveErrAfterCommit = errors.New("reserve commit result unavailable")

	response := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/confirm-outline", `{}`, "confirm-reserve-ambiguous")
	if response.Code != http.StatusOK {
		t.Fatalf("ambiguous reserve status = %d, body = %s", response.Code, response.Body.String())
	}
	waitForPPTAgentStage(t, state, taskID, pptapp.StageReady)
	_, reserveEffects, completeCalls, captureEffects, failCalls, releaseEffects := billing.counts()
	if state.generationClaimCalls != 1 || state.failedGenerationClaims != 0 || reserveEffects != 1 || completeCalls != 1 || captureEffects != 1 || failCalls != 0 || releaseEffects != 0 {
		t.Fatalf("claims=%d failedClaims=%d reserve=%d complete=%d capture=%d fail=%d release=%d", state.generationClaimCalls, state.failedGenerationClaims, reserveEffects, completeCalls, captureEffects, failCalls, releaseEffects)
	}
}

func TestPPTStalePreBindReservationCancelReleaseFailureRetriesExactlyOnce(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID, userID := preparePPTAgentOutlineReadyTask(t, harness, 1)
	state := newPPTAgentGenerationTestState(harness.state, nil)
	task, _ := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	confirmKey := "confirm-stale-reservation"
	if _, _, err := state.BeginGenerationClaim(context.Background(), pptAgentTestOwner(userID), taskID, confirmKey, pptAgentConfirmHash(task)); err != nil {
		t.Fatalf("seed generation claim: %v", err)
	}
	state.ageGenerationClaim(taskID, confirmKey, time.Now().UTC().Add(-10*time.Minute))
	billing := newPPTAgentBillingTestStore(nil)
	billing.seed(generationTask{ID: "billing-stale-reservation", ClientRequestID: "ppt-confirm:" + taskID, UserID: userID,
		Status: "PROCESSING", TaskStatus: taskStatusQueued, BillingStatus: billingStatusReserved})
	billing.releaseErr = errors.New("injected release failure")

	first := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/cancel", `{}`, "cancel-stale-reservation")
	if first.Code != http.StatusServiceUnavailable || !strings.Contains(first.Body.String(), "PPT_BILLING_FINALIZE_FAILED") {
		t.Fatalf("first stale cancel status = %d, body = %s", first.Code, first.Body.String())
	}
	afterFailure, _ := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	if afterFailure.Stage != pptapp.StageGenerating || afterFailure.BillingTaskID != "billing-stale-reservation" || !pptAgentTaskHasLiveCancel(afterFailure) || state.cancelWrites != 0 {
		t.Fatalf("release failure claimed terminal state: %#v cancelWrites=%d", afterFailure, state.cancelWrites)
	}
	billing.releaseErr = nil
	second := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost,
		"/api/v1/ppt/sessions/"+taskID+"/cancel", `{}`, "cancel-stale-reservation")
	if second.Code != http.StatusOK {
		t.Fatalf("stale cancel retry status = %d, body = %s", second.Code, second.Body.String())
	}
	final := waitForPPTAgentStage(t, state, taskID, pptapp.StageCancelled)
	_, reserveEffects, completeCalls, captureEffects, failCalls, releaseEffects := billing.counts()
	if final.Stage != pptapp.StageCancelled || reserveEffects != 1 || completeCalls != 0 || captureEffects != 0 || failCalls != 2 || releaseEffects != 1 || state.cancelWrites != 1 {
		t.Fatalf("final=%s reserve=%d complete=%d capture=%d fail=%d release=%d cancelWrites=%d", final.Stage, reserveEffects, completeCalls, captureEffects, failCalls, releaseEffects, state.cancelWrites)
	}
}

func TestPPTConfirmAndCancelRequireEmptyObjectBody(t *testing.T) {
	tests := []struct {
		name    string
		path    func(string) string
		prepare func(*testing.T, pptAgentHTTPHarness) string
	}{
		{name: "confirm", path: func(taskID string) string { return "/api/v1/ppt/sessions/" + taskID + "/confirm-outline" }, prepare: func(t *testing.T, harness pptAgentHTTPHarness) string {
			taskID, _ := preparePPTAgentOutlineReadyTask(t, harness, 1)
			return taskID
		}},
		{name: "cancel", path: func(taskID string) string { return "/api/v1/ppt/sessions/" + taskID + "/cancel" }, prepare: func(t *testing.T, harness pptAgentHTTPHarness) string {
			return harness.createSession(t, "general", 1)
		}},
	}
	for _, test := range tests {
		for _, body := range []string{"null", `{"unexpected":true}`} {
			t.Run(test.name+"/"+body, func(t *testing.T) {
				harness := newPPTAgentHTTPHarness(t)
				taskID := test.prepare(t, harness)
				state := newPPTAgentGenerationTestState(harness.state, nil)
				billing := newPPTAgentBillingTestStore(nil)
				response := requestPPTAgentGeneration(harness, state, billing, nil, http.MethodPost, test.path(taskID), body, "strict-empty-object")
				if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "PPT_REQUEST_INVALID") {
					t.Fatalf("body %s status = %d, response = %s", body, response.Code, response.Body.String())
				}
				if billing.taskCount() != 0 || state.generationClaimCalls != 0 || state.cancelWrites != 0 {
					t.Fatalf("invalid body caused work: billing=%d claims=%d cancels=%d", billing.taskCount(), state.generationClaimCalls, state.cancelWrites)
				}
			})
		}
	}
}

type pptAgentGenerationTestEvents struct {
	mu      sync.Mutex
	entries []string
}

func (e *pptAgentGenerationTestEvents) add(value string) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.entries = append(e.entries, value)
}

func (e *pptAgentGenerationTestEvents) snapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.entries...)
}

func (e *pptAgentGenerationTestEvents) before(first, second string) bool {
	items := e.snapshot()
	firstIndex, secondIndex := -1, -1
	for index, item := range items {
		if item == first && firstIndex < 0 {
			firstIndex = index
		}
		if item == second && secondIndex < 0 {
			secondIndex = index
		}
	}
	return firstIndex >= 0 && secondIndex > firstIndex
}

type pptAgentGenerationTestState struct {
	base             *pptAgentTestState
	events           *pptAgentGenerationTestEvents
	beginFailures    int
	completeFailures int
	panicOnPersist   bool
	bindEntered      chan struct{}
	continueBind     chan struct{}

	beginGenerationCalls   int
	generationClaimCalls   int
	failedGenerationClaims int
	runClaims              int
	readyWrites            int
	cancelWrites           int
	failedWrites           int
	runSequence            int
	cancelSequence         int
	claimSequence          int
	cancelPending          map[string]pptapp.CancelClaim
	persisted              []int
	persistedPages         chan int
	continuePages          chan struct{}
	completeAttempts       chan struct{}
	stageChanges           chan pptapp.Stage
}

type pptAgentCancelBeforeCaptureState struct {
	pptAgentGenerationState
	mu          sync.Mutex
	exactReads  int
	cancelClaim pptapp.CancelClaim
	cancelErr   error
}

type pptAgentCleanupInterleavingState struct {
	pptAgentGenerationState
	fenceEntered    chan struct{}
	continueCleanup chan struct{}
	pauseOnce       sync.Once
	mu              sync.Mutex
	fenceAcquired   bool
}

type pptAgentCleanupFenceState interface {
	AcquireGenerationCleanupFence(context.Context, pptapp.OwnerScope, string, pptapp.GenerationClaim, time.Time) (pptapp.GenerationClaim, pptapp.Task, error)
}

func (s *pptAgentCleanupInterleavingState) GetTask(ctx context.Context, owner pptapp.OwnerScope, taskID string) (pptapp.Task, error) {
	task, err := s.pptAgentGenerationState.GetTask(ctx, owner, taskID)
	if err == nil {
		s.pauseCleanup(false)
	}
	return task, err
}

func (s *pptAgentCleanupInterleavingState) AcquireGenerationCleanupFence(ctx context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.GenerationClaim, now time.Time) (pptapp.GenerationClaim, pptapp.Task, error) {
	state, ok := s.pptAgentGenerationState.(pptAgentCleanupFenceState)
	if !ok {
		return pptapp.GenerationClaim{}, pptapp.Task{}, errors.New("cleanup fence state is unavailable")
	}
	fenced, task, err := state.AcquireGenerationCleanupFence(ctx, owner, taskID, claim, now)
	if err == nil {
		s.pauseCleanup(true)
	}
	return fenced, task, err
}

func (s *pptAgentCleanupInterleavingState) pauseCleanup(fenced bool) {
	s.pauseOnce.Do(func() {
		s.mu.Lock()
		s.fenceAcquired = fenced
		s.mu.Unlock()
		close(s.fenceEntered)
		<-s.continueCleanup
	})
}

func (s *pptAgentCleanupInterleavingState) cleanupFenceAcquired() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fenceAcquired
}

func (s *pptAgentCancelBeforeCaptureState) GetTask(ctx context.Context, owner pptapp.OwnerScope, taskID string) (pptapp.Task, error) {
	task, err := s.pptAgentGenerationState.GetTask(ctx, owner, taskID)
	if err != nil {
		return task, err
	}
	if _, exact := pptAgentGenerationClaimForCompleteTask(task); !exact {
		return task, nil
	}
	s.mu.Lock()
	s.exactReads++
	trigger := s.exactReads == 2
	s.mu.Unlock()
	if !trigger {
		return task, nil
	}
	claim, _, cancelErr := s.pptAgentGenerationState.BeginCancel(context.Background(), owner, taskID, "cancel-before-capture", "cancel-before-capture-hash")
	s.mu.Lock()
	s.cancelClaim = claim
	s.cancelErr = cancelErr
	s.mu.Unlock()
	if cancelErr != nil {
		return pptapp.Task{}, cancelErr
	}
	return s.pptAgentGenerationState.GetTask(ctx, owner, taskID)
}

func newPPTAgentGenerationTestState(base *pptAgentTestState, events *pptAgentGenerationTestEvents) *pptAgentGenerationTestState {
	return &pptAgentGenerationTestState{
		base: base, events: events, cancelPending: map[string]pptapp.CancelClaim{}, stageChanges: make(chan pptapp.Stage, 32),
	}
}

func (s *pptAgentGenerationTestState) GetTask(ctx context.Context, owner pptapp.OwnerScope, taskID string) (pptapp.Task, error) {
	return s.base.GetTask(ctx, owner, taskID)
}

func (s *pptAgentGenerationTestState) BeginGenerationClaim(_ context.Context, owner pptapp.OwnerScope, taskID, key, requestHash string) (pptapp.OperationClaim, pptapp.Task, error) {
	s.base.mu.Lock()
	defer s.base.mu.Unlock()
	s.generationClaimCalls++
	task, ok := s.base.tasks[taskID]
	if !ok || !pptAgentTestOwnerMatches(task, owner) {
		return pptapp.OperationClaim{}, pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	if index := findPPTAgentTestRecord(task.IdempotencyRecords, "confirm-outline", key); index >= 0 {
		record := &task.IdempotencyRecords[index]
		if record.RequestHash != requestHash {
			return pptapp.OperationClaim{}, pptapp.Task{}, pptapp.ErrIdempotencyConflict
		}
		if record.State == "processing" {
			record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			s.base.tasks[taskID] = task
			return pptapp.OperationClaim{Scope: record.Scope, Key: record.Key, RequestHash: record.RequestHash, OperationToken: record.OperationToken, Replay: true}, pptapp.NormalizeTask(task), nil
		}
		if record.State == "completed" {
			return pptapp.OperationClaim{Scope: record.Scope, Key: record.Key, RequestHash: record.RequestHash, OperationToken: record.OperationToken, Replay: true, CompletedReplay: true}, pptapp.NormalizeTask(task), nil
		}
		if task.Stage != pptapp.StageOutlineReady {
			return pptapp.OperationClaim{}, pptapp.Task{}, pptapp.ErrInvalidStage
		}
		s.claimSequence++
		record.State = "processing"
		record.OperationToken = fmt.Sprintf("confirm-claim-%d", s.claimSequence)
		record.ErrorCode = ""
		task.ErrorCode = ""
		s.base.tasks[taskID] = task
		return pptapp.OperationClaim{Scope: record.Scope, Key: record.Key, RequestHash: record.RequestHash, OperationToken: record.OperationToken}, pptapp.NormalizeTask(task), nil
	}
	if task.Stage != pptapp.StageOutlineReady {
		return pptapp.OperationClaim{}, pptapp.Task{}, pptapp.ErrInvalidStage
	}
	for _, record := range task.IdempotencyRecords {
		if record.Scope == "confirm-outline" && record.State == "processing" {
			return pptapp.OperationClaim{}, pptapp.Task{}, pptapp.ErrOperationInProgress
		}
	}
	s.claimSequence++
	now := time.Now().UTC().Format(time.RFC3339Nano)
	record := pptapp.IdempotencyRecord{Scope: "confirm-outline", Key: key, RequestHash: requestHash, State: "processing", OperationToken: fmt.Sprintf("confirm-claim-%d", s.claimSequence), CreatedAt: now, UpdatedAt: now}
	task.IdempotencyRecords = append(task.IdempotencyRecords, record)
	s.base.tasks[taskID] = task
	s.events.add("state:begin-generation-claim")
	return pptapp.OperationClaim{Scope: record.Scope, Key: record.Key, RequestHash: record.RequestHash, OperationToken: record.OperationToken}, pptapp.NormalizeTask(task), nil
}

func (s *pptAgentGenerationTestState) FailGenerationClaim(_ context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim, errorCode string) (pptapp.Task, error) {
	s.base.mu.Lock()
	defer s.base.mu.Unlock()
	task, ok := s.base.tasks[taskID]
	if !ok || !pptAgentTestOwnerMatches(task, owner) {
		return pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	index := findPPTAgentTestRecord(task.IdempotencyRecords, "confirm-outline", claim.Key)
	if index < 0 || task.IdempotencyRecords[index].OperationToken != claim.OperationToken || task.IdempotencyRecords[index].RequestHash != claim.RequestHash {
		return pptapp.Task{}, pptapp.ErrOperationTokenMismatch
	}
	if task.Stage != pptapp.StageOutlineReady || task.BillingTaskID != "" {
		return pptapp.Task{}, pptapp.ErrInvalidStage
	}
	task.IdempotencyRecords[index].State = "failed"
	task.IdempotencyRecords[index].ErrorCode = errorCode
	task.ErrorCode = errorCode
	s.failedGenerationClaims++
	s.base.tasks[taskID] = task
	s.events.add("state:fail-generation-claim")
	return pptapp.NormalizeTask(task), nil
}

func (s *pptAgentGenerationTestState) BindGenerationBilling(_ context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim, billingTaskID string) (pptapp.Task, error) {
	if s.bindEntered != nil {
		select {
		case s.bindEntered <- struct{}{}:
		default:
		}
	}
	if s.continueBind != nil {
		<-s.continueBind
	}
	s.base.mu.Lock()
	defer s.base.mu.Unlock()
	s.beginGenerationCalls++
	if s.beginFailures > 0 {
		s.beginFailures--
		return pptapp.Task{}, errors.New("injected begin generation persistence failure")
	}
	task, ok := s.base.tasks[taskID]
	if !ok || !pptAgentTestOwnerMatches(task, owner) {
		return pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	index := findPPTAgentTestRecord(task.IdempotencyRecords, claim.Scope, claim.Key)
	if index >= 0 {
		record := task.IdempotencyRecords[index]
		if record.RequestHash != claim.RequestHash || record.OperationToken != claim.OperationToken {
			return pptapp.Task{}, pptapp.ErrOperationTokenMismatch
		}
		if task.Stage == pptapp.StageOutlineReady && record.State == "processing" && task.BillingTaskID == "" {
			now := time.Now().UTC().Format(time.RFC3339Nano)
			task.Stage = pptapp.StageGenerating
			task.Status = pptapp.StatusProcessing
			task.BillingTaskID = billingTaskID
			task.GenerationStartedAt = now
			s.base.tasks[taskID] = task
			s.events.add("state:begin-generation")
			s.stageChanges <- pptapp.StageGenerating
			return pptapp.NormalizeTask(task), nil
		}
		if task.BillingTaskID != billingTaskID {
			return pptapp.Task{}, pptapp.ErrBillingBindingMismatch
		}
		if task.Stage == pptapp.StageGenerating || (task.Stage == pptapp.StageReady && record.State == "completed") {
			return pptapp.NormalizeTask(task), nil
		}
		if task.Stage == pptapp.StageCancelled {
			return pptapp.Task{}, pptapp.ErrSessionCancelled
		}
		return pptapp.Task{}, pptapp.ErrInvalidStage
	}
	return pptapp.Task{}, pptapp.ErrOperationTokenMismatch
}

func (s *pptAgentGenerationTestState) ClaimGenerationRun(_ context.Context, owner pptapp.OwnerScope, taskID string, now time.Time) (pptapp.GenerationClaim, pptapp.Task, error) {
	s.base.mu.Lock()
	defer s.base.mu.Unlock()
	task, ok := s.base.tasks[taskID]
	if !ok || !pptAgentTestOwnerMatches(task, owner) {
		return pptapp.GenerationClaim{}, pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	if task.Stage != pptapp.StageGenerating {
		return pptapp.GenerationClaim{}, pptapp.Task{}, pptapp.ErrInvalidStage
	}
	if _, cancelling := s.cancelPending[taskID]; cancelling {
		return pptapp.GenerationClaim{}, pptapp.Task{}, pptapp.ErrSessionCancelled
	}
	if task.GenerationLease != nil {
		leaseUntil, err := time.Parse(time.RFC3339Nano, task.GenerationLease.LeaseUntil)
		if err == nil && leaseUntil.After(now) {
			return pptapp.GenerationClaim{}, pptapp.Task{}, pptapp.ErrGenerationAlreadyRunning
		}
	}
	s.runSequence++
	s.runClaims++
	claim := pptapp.GenerationClaim{RunToken: fmt.Sprintf("run-%d", s.runSequence), LeaseUntil: now.Add(time.Minute).Format(time.RFC3339Nano)}
	task.GenerationLease = &pptapp.GenerationLease{RunToken: claim.RunToken, LeaseUntil: claim.LeaseUntil}
	s.base.tasks[taskID] = task
	s.events.add("state:claim-run")
	return claim, pptapp.NormalizeTask(task), nil
}

func (s *pptAgentGenerationTestState) RenewGenerationRun(_ context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.GenerationClaim, now time.Time, leaseDuration time.Duration) (pptapp.GenerationClaim, pptapp.Task, error) {
	s.base.mu.Lock()
	defer s.base.mu.Unlock()
	task, ok := s.base.tasks[taskID]
	if !ok || !pptAgentTestOwnerMatches(task, owner) {
		return pptapp.GenerationClaim{}, pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	if task.Stage != pptapp.StageGenerating {
		return pptapp.GenerationClaim{}, pptapp.Task{}, pptapp.ErrInvalidStage
	}
	if task.GenerationLease == nil || task.GenerationLease.RunToken != claim.RunToken || leaseDuration <= 0 {
		return pptapp.GenerationClaim{}, pptapp.Task{}, pptapp.ErrGenerationRunMismatch
	}
	currentUntil, err := time.Parse(time.RFC3339Nano, task.GenerationLease.LeaseUntil)
	if err != nil || !currentUntil.After(now) {
		return pptapp.GenerationClaim{}, pptapp.Task{}, pptapp.ErrGenerationRunMismatch
	}
	leaseUntil := now.UTC().Add(leaseDuration).Format(time.RFC3339Nano)
	renewed := pptapp.GenerationClaim{RunToken: claim.RunToken, LeaseUntil: leaseUntil}
	task.GenerationLease = &pptapp.GenerationLease{RunToken: claim.RunToken, LeaseUntil: leaseUntil}
	s.base.tasks[taskID] = task
	s.events.add("state:renew-run")
	return renewed, pptapp.NormalizeTask(task), nil
}

func (s *pptAgentGenerationTestState) AcquireGenerationCleanupFence(_ context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.GenerationClaim, now time.Time) (pptapp.GenerationClaim, pptapp.Task, error) {
	s.base.mu.Lock()
	defer s.base.mu.Unlock()
	task, ok := s.base.tasks[taskID]
	if !ok || !pptAgentTestOwnerMatches(task, owner) {
		return pptapp.GenerationClaim{}, pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	if task.Stage != pptapp.StageGenerating || task.GenerationLease == nil || task.GenerationLease.RunToken != claim.RunToken {
		return pptapp.GenerationClaim{}, pptapp.Task{}, pptapp.ErrGenerationRunMismatch
	}
	leaseUntil := now.UTC().Add(time.Minute)
	if currentUntil, err := time.Parse(time.RFC3339Nano, task.GenerationLease.LeaseUntil); err == nil && currentUntil.After(leaseUntil) {
		leaseUntil = currentUntil
	}
	fenced := pptapp.GenerationClaim{RunToken: claim.RunToken, LeaseUntil: leaseUntil.Format(time.RFC3339Nano)}
	task.GenerationLease = &pptapp.GenerationLease{RunToken: fenced.RunToken, LeaseUntil: fenced.LeaseUntil}
	s.base.tasks[taskID] = task
	s.events.add("state:cleanup-fence")
	return fenced, pptapp.NormalizeTask(task), nil
}

func (s *pptAgentGenerationTestState) PersistGeneratedSlide(ctx context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.GenerationClaim, slide pptapp.Slide) (pptapp.Task, error) {
	if s.panicOnPersist {
		panic("injected persist panic")
	}
	s.base.mu.Lock()
	task, ok := s.base.tasks[taskID]
	if !ok || !pptAgentTestOwnerMatches(task, owner) {
		s.base.mu.Unlock()
		return pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	if task.Stage != pptapp.StageGenerating {
		s.base.mu.Unlock()
		return pptapp.Task{}, pptapp.ErrInvalidStage
	}
	if _, cancelling := s.cancelPending[taskID]; cancelling {
		s.base.mu.Unlock()
		return pptapp.Task{}, pptapp.ErrSessionCancelled
	}
	if task.GenerationLease == nil || task.GenerationLease.RunToken != claim.RunToken {
		s.base.mu.Unlock()
		return pptapp.Task{}, pptapp.ErrGenerationRunMismatch
	}
	for _, existing := range task.Slides {
		if existing.Page == slide.Page && existing.ID == slide.ID {
			s.base.mu.Unlock()
			return pptapp.NormalizeTask(task), nil
		}
		if existing.Page == slide.Page || existing.ID == slide.ID {
			s.base.mu.Unlock()
			return pptapp.Task{}, pptapp.ErrSlideCoordinateConflict
		}
	}
	task.Slides = append(task.Slides, pptapp.NormalizeSlideIR(slide))
	sort.SliceStable(task.Slides, func(i, j int) bool { return task.Slides[i].Page < task.Slides[j].Page })
	task.CurrentPage = len(task.Slides)
	task.Progress = task.CurrentPage * 100 / task.SlideCount
	s.persisted = append(s.persisted, slide.Page)
	s.base.tasks[taskID] = task
	s.base.mu.Unlock()
	s.events.add(fmt.Sprintf("state:slide-%d", slide.Page))
	if s.persistedPages != nil {
		s.persistedPages <- slide.Page
	}
	if s.continuePages != nil {
		select {
		case <-ctx.Done():
			return pptapp.Task{}, ctx.Err()
		case <-s.continuePages:
		}
	}
	return pptapp.NormalizeTask(task), nil
}

func (s *pptAgentGenerationTestState) CompleteGenerationAfterCapture(_ context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.GenerationClaim) (pptapp.Task, error) {
	s.base.mu.Lock()
	defer s.base.mu.Unlock()
	if s.completeAttempts != nil {
		select {
		case s.completeAttempts <- struct{}{}:
		default:
		}
	}
	task, ok := s.base.tasks[taskID]
	if !ok || !pptAgentTestOwnerMatches(task, owner) {
		return pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	if task.Stage != pptapp.StageGenerating {
		return pptapp.Task{}, pptapp.ErrInvalidStage
	}
	if task.GenerationLease == nil || task.GenerationLease.RunToken != claim.RunToken {
		return pptapp.Task{}, pptapp.ErrGenerationRunMismatch
	}
	if len(task.Slides) != task.SlideCount {
		return pptapp.Task{}, pptapp.ErrGenerationIncomplete
	}
	if s.completeFailures > 0 {
		s.completeFailures--
		return pptapp.Task{}, errors.New("injected ready persistence failure")
	}
	task.Stage = pptapp.StageReady
	task.Status = pptapp.StatusSuccess
	task.Progress = 100
	task.CurrentPage = task.SlideCount
	task.GenerationLease = nil
	for index := range task.IdempotencyRecords {
		switch {
		case task.IdempotencyRecords[index].Scope == "confirm-outline" && task.IdempotencyRecords[index].State == "processing":
			task.IdempotencyRecords[index].State = "completed"
		case task.IdempotencyRecords[index].Scope == "cancel" && task.IdempotencyRecords[index].State == "processing":
			task.IdempotencyRecords[index].State = "failed"
			task.IdempotencyRecords[index].ErrorCode = pptapp.ErrBillingAlreadyCaptured.Error()
		}
	}
	s.readyWrites++
	s.base.tasks[taskID] = task
	s.events.add("state:ready")
	s.stageChanges <- pptapp.StageReady
	return pptapp.NormalizeTask(task), nil
}

func (s *pptAgentGenerationTestState) BeginCancel(_ context.Context, owner pptapp.OwnerScope, taskID, key, requestHash string) (pptapp.CancelClaim, pptapp.Task, error) {
	s.base.mu.Lock()
	defer s.base.mu.Unlock()
	task, ok := s.base.tasks[taskID]
	if !ok || !pptAgentTestOwnerMatches(task, owner) {
		return pptapp.CancelClaim{}, pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	if existing, ok := s.cancelPending[taskID]; ok {
		if existing.Key != key || existing.RequestHash != requestHash {
			return pptapp.CancelClaim{}, pptapp.Task{}, pptapp.ErrIdempotencyConflict
		}
		if record, found := findPPTAgentTestRecordValue(task.IdempotencyRecords, "cancel", key); found && record.State == "failed" && record.ErrorCode == pptapp.ErrBillingAlreadyCaptured.Error() {
			return pptapp.CancelClaim{}, pptapp.Task{}, pptapp.ErrBillingAlreadyCaptured
		}
		existing.Replay = true
		return existing, pptapp.NormalizeTask(task), nil
	}
	if task.Stage == pptapp.StageCancelled {
		return pptapp.CancelClaim{}, pptapp.Task{}, pptapp.ErrSessionCancelled
	}
	if task.Stage == pptapp.StageOutlineReady {
		for _, record := range task.IdempotencyRecords {
			if record.Scope == "confirm-outline" && record.State == "processing" {
				return pptapp.CancelClaim{}, pptapp.Task{}, pptapp.ErrOperationInProgress
			}
		}
	}
	if task.Stage != pptapp.StageDraft && task.Stage != pptapp.StageOutlineReady && task.Stage != pptapp.StageGenerating {
		return pptapp.CancelClaim{}, pptapp.Task{}, pptapp.ErrInvalidStage
	}
	s.cancelSequence++
	claim := pptapp.CancelClaim{Key: key, RequestHash: requestHash, OperationToken: fmt.Sprintf("cancel-%d", s.cancelSequence)}
	s.cancelPending[taskID] = claim
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task.IdempotencyRecords = append(task.IdempotencyRecords, pptapp.IdempotencyRecord{
		Scope: "cancel", Key: key, RequestHash: requestHash, State: "processing", OperationToken: claim.OperationToken, CreatedAt: now, UpdatedAt: now,
	})
	s.base.tasks[taskID] = task
	s.events.add("state:begin-cancel")
	return claim, pptapp.NormalizeTask(task), nil
}

func (s *pptAgentGenerationTestState) BeginCancelAfterStaleGenerationClaim(_ context.Context, owner pptapp.OwnerScope, taskID string, generationClaim pptapp.OperationClaim, key, requestHash string, now time.Time) (pptapp.CancelClaim, pptapp.Task, error) {
	s.base.mu.Lock()
	defer s.base.mu.Unlock()
	task, ok := s.base.tasks[taskID]
	if !ok || !pptAgentTestOwnerMatches(task, owner) {
		return pptapp.CancelClaim{}, pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	index := findPPTAgentTestRecord(task.IdempotencyRecords, "confirm-outline", generationClaim.Key)
	if index < 0 || task.IdempotencyRecords[index].OperationToken != generationClaim.OperationToken || task.IdempotencyRecords[index].RequestHash != generationClaim.RequestHash || task.IdempotencyRecords[index].State != "processing" {
		return pptapp.CancelClaim{}, pptapp.Task{}, pptapp.ErrOperationTokenMismatch
	}
	updatedAt, parseErr := time.Parse(time.RFC3339Nano, task.IdempotencyRecords[index].UpdatedAt)
	if parseErr == nil && updatedAt.Add(5*time.Minute).After(now) {
		return pptapp.CancelClaim{}, pptapp.Task{}, pptapp.ErrOperationInProgress
	}
	if task.Stage != pptapp.StageGenerating || strings.TrimSpace(task.BillingTaskID) == "" {
		return pptapp.CancelClaim{}, pptapp.Task{}, pptapp.ErrInvalidStage
	}
	task.IdempotencyRecords[index].State = "failed"
	task.IdempotencyRecords[index].ErrorCode = pptapp.ErrSessionCancelled.Error()
	s.cancelSequence++
	claim := pptapp.CancelClaim{Key: key, RequestHash: requestHash, OperationToken: fmt.Sprintf("cancel-%d", s.cancelSequence)}
	s.cancelPending[taskID] = claim
	timestamp := now.UTC().Format(time.RFC3339Nano)
	task.IdempotencyRecords = append(task.IdempotencyRecords, pptapp.IdempotencyRecord{
		Scope: "cancel", Key: key, RequestHash: requestHash, State: "processing", OperationToken: claim.OperationToken, CreatedAt: timestamp, UpdatedAt: timestamp,
	})
	s.base.tasks[taskID] = task
	s.events.add("state:recover-stale-generation-claim")
	return claim, pptapp.NormalizeTask(task), nil
}

func (s *pptAgentGenerationTestState) CompleteCancel(_ context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.CancelClaim) (pptapp.Task, error) {
	s.base.mu.Lock()
	defer s.base.mu.Unlock()
	task, ok := s.base.tasks[taskID]
	if !ok || !pptAgentTestOwnerMatches(task, owner) {
		return pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	existing, ok := s.cancelPending[taskID]
	if !ok || existing.OperationToken != claim.OperationToken {
		return pptapp.Task{}, pptapp.ErrOperationTokenMismatch
	}
	if task.Stage == pptapp.StageCancelled {
		return pptapp.NormalizeTask(task), nil
	}
	task.Stage = pptapp.StageCancelled
	task.Status = pptapp.StatusCancelled
	task.GenerationLease = nil
	for index := range task.IdempotencyRecords {
		switch {
		case task.IdempotencyRecords[index].Scope == "cancel" && task.IdempotencyRecords[index].OperationToken == claim.OperationToken:
			task.IdempotencyRecords[index].State = "completed"
		case task.IdempotencyRecords[index].Scope == "confirm-outline" && task.IdempotencyRecords[index].State == "processing":
			task.IdempotencyRecords[index].State = "failed"
			task.IdempotencyRecords[index].ErrorCode = pptapp.ErrSessionCancelled.Error()
		}
	}
	s.cancelWrites++
	s.base.tasks[taskID] = task
	s.events.add("state:cancelled")
	s.stageChanges <- pptapp.StageCancelled
	return pptapp.NormalizeTask(task), nil
}

func (s *pptAgentGenerationTestState) FailGenerationAfterRelease(_ context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.GenerationClaim, errorCode string) (pptapp.Task, error) {
	s.base.mu.Lock()
	defer s.base.mu.Unlock()
	task, ok := s.base.tasks[taskID]
	if !ok || !pptAgentTestOwnerMatches(task, owner) {
		return pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	if task.Stage != pptapp.StageGenerating || task.GenerationLease == nil || task.GenerationLease.RunToken != claim.RunToken {
		return pptapp.Task{}, pptapp.ErrGenerationRunMismatch
	}
	if _, cancelling := s.cancelPending[taskID]; cancelling {
		return pptapp.Task{}, pptapp.ErrSessionCancelled
	}
	task.Stage = pptapp.StageFailed
	task.Status = pptapp.StatusFailed
	task.ErrorCode = errorCode
	task.GenerationLease = nil
	s.failedWrites++
	s.base.tasks[taskID] = task
	s.events.add("state:failed")
	s.stageChanges <- pptapp.StageFailed
	return pptapp.NormalizeTask(task), nil
}

func (s *pptAgentGenerationTestState) seedGenerating(taskID, billingTaskID, key, requestHash string, slides []pptapp.Slide, lease *pptapp.GenerationLease) {
	s.base.mu.Lock()
	defer s.base.mu.Unlock()
	task := s.base.tasks[taskID]
	task.Stage = pptapp.StageGenerating
	task.Status = pptapp.StatusProcessing
	task.BillingTaskID = billingTaskID
	task.Slides = append([]pptapp.Slide(nil), slides...)
	task.CurrentPage = len(slides)
	if task.SlideCount > 0 {
		task.Progress = len(slides) * 100 / task.SlideCount
	}
	task.GenerationLease = lease
	task.IdempotencyRecords = append(task.IdempotencyRecords, pptapp.IdempotencyRecord{
		Scope: "confirm-outline", Key: key, RequestHash: requestHash, State: "processing", OperationToken: "confirm-seeded",
	})
	s.base.tasks[taskID] = task
}

func (s *pptAgentGenerationTestState) setStage(taskID string, stage pptapp.Stage) {
	s.base.mu.Lock()
	defer s.base.mu.Unlock()
	task := s.base.tasks[taskID]
	task.Stage = stage
	task.Status = pptapp.StageStatus(stage)
	s.base.tasks[taskID] = task
}

func (s *pptAgentGenerationTestState) ageGenerationClaim(taskID, key string, updatedAt time.Time) {
	s.base.mu.Lock()
	defer s.base.mu.Unlock()
	task := s.base.tasks[taskID]
	if index := findPPTAgentTestRecord(task.IdempotencyRecords, "confirm-outline", key); index >= 0 {
		task.IdempotencyRecords[index].UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	}
	s.base.tasks[taskID] = task
}

func (s *pptAgentGenerationTestState) persistedPageSnapshot() []int {
	s.base.mu.Lock()
	defer s.base.mu.Unlock()
	return append([]int(nil), s.persisted...)
}

func findPPTAgentTestRecordValue(records []pptapp.IdempotencyRecord, scope, key string) (pptapp.IdempotencyRecord, bool) {
	index := findPPTAgentTestRecord(records, scope, key)
	if index < 0 {
		return pptapp.IdempotencyRecord{}, false
	}
	return records[index], true
}

type pptAgentBillingTestStore struct {
	mu                    sync.Mutex
	events                *pptAgentGenerationTestEvents
	tasks                 map[string]generationTask
	byClient              map[string]string
	requests              []createGenerationTaskRequest
	createCalls           int
	reserveEffects        int
	completeCalls         int
	captureEffects        int
	failCalls             int
	releaseEffects        int
	lookupCalls           int
	reserveErr            error
	reserveErrAfterCommit error
	releaseErr            error
	captureErrAfterCommit error
	beforeReserve         func(createGenerationTaskRequest)
}

func newPPTAgentBillingTestStore(events *pptAgentGenerationTestEvents) *pptAgentBillingTestStore {
	return &pptAgentBillingTestStore{events: events, tasks: map[string]generationTask{}, byClient: map[string]string{}}
}

func (s *pptAgentBillingTestStore) CreatePendingGenerationTask(req createGenerationTaskRequest) (generationTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.beforeReserve != nil {
		s.beforeReserve(req)
	}
	s.createCalls++
	s.requests = append(s.requests, cloneGenerationCreateRequest(req))
	clientKey := strings.TrimSpace(req.UserID) + "\x00" + strings.TrimSpace(req.ClientRequestID)
	if id := s.byClient[clientKey]; id != "" {
		return s.tasks[id], nil
	}
	if s.reserveErr != nil {
		return generationTask{}, s.reserveErr
	}
	id := fmt.Sprintf("billing-%d", len(s.tasks)+1)
	task := generationTask{
		ID: id, ClientRequestID: strings.TrimSpace(req.ClientRequestID), UserID: strings.TrimSpace(req.UserID),
		TenantID:           strings.TrimSpace(stringValue(req.Params["tenant_id"])),
		OrganizationID:     strings.TrimSpace(stringValue(req.Params["organization_id"])),
		BillingAccountType: strings.TrimSpace(stringValue(req.Params["billing_scope"])),
		BillingAccountID:   strings.TrimSpace(stringValue(req.Params["billing_account_id"])),
		Type:               req.Type, ModuleCode: req.ModuleCode, Prompt: req.Prompt, Model: req.Model, Params: req.Params,
		Status: "PROCESSING", TaskStatus: taskStatusQueued, BillingStatus: billingStatusReserved,
	}
	s.tasks[id] = task
	s.byClient[clientKey] = id
	s.reserveEffects++
	s.events.add("billing:reserve")
	if s.reserveErrAfterCommit != nil {
		err := s.reserveErrAfterCommit
		s.reserveErrAfterCommit = nil
		return generationTask{}, err
	}
	return task, nil
}

func (s *pptAgentBillingTestStore) CompleteGenerationTask(id string, _ createGenerationTaskRequest) (generationTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completeCalls++
	task, ok := s.tasks[id]
	if !ok {
		return generationTask{}, errors.New("billing task not found")
	}
	if task.Status != "SUCCEEDED" {
		task.Status = "SUCCEEDED"
		task.TaskStatus = taskStatusSucceeded
		task.BillingStatus = billingStatusCaptured
		s.captureEffects++
		s.events.add("billing:capture")
		s.tasks[id] = task
	}
	if s.captureErrAfterCommit != nil {
		err := s.captureErrAfterCommit
		s.captureErrAfterCommit = nil
		return generationTask{}, err
	}
	return task, nil
}

func (s *pptAgentBillingTestStore) FailGenerationTask(id string, _ string) (generationTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failCalls++
	task, ok := s.tasks[id]
	if !ok {
		return generationTask{}, errors.New("billing task not found")
	}
	if task.Status == "SUCCEEDED" {
		return task, nil
	}
	if task.BillingStatus == billingStatusReleased {
		return task, nil
	}
	if s.releaseErr != nil {
		return generationTask{}, s.releaseErr
	}
	task.Status = "FAILED"
	task.TaskStatus = taskStatusFailed
	task.BillingStatus = billingStatusReleased
	s.releaseEffects++
	s.events.add("billing:release")
	s.tasks[id] = task
	return task, nil
}

func (s *pptAgentBillingTestStore) ListGenerationTasks() ([]generationTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lookupCalls++
	s.events.add("billing:lookup")
	items := make([]generationTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		items = append(items, task)
	}
	return items, nil
}

func (s *pptAgentBillingTestStore) seed(task generationTask) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(task.TenantID) == "" {
		task.TenantID = "tenant_default"
	}
	if strings.TrimSpace(task.OrganizationID) == "" {
		task.OrganizationID = defaultOrganizationID(task.TenantID)
	}
	if strings.TrimSpace(task.BillingAccountType) == "" {
		task.BillingAccountType = contextPersonal
	}
	if strings.TrimSpace(task.BillingAccountID) == "" {
		task.BillingAccountID = task.UserID
	}
	if strings.TrimSpace(task.Type) == "" {
		task.Type = "PPT_GENERATION"
	}
	if strings.TrimSpace(task.ModuleCode) == "" {
		task.ModuleCode = modulePPTGeneration
	}
	s.tasks[task.ID] = task
	s.byClient[strings.TrimSpace(task.UserID)+"\x00"+strings.TrimSpace(task.ClientRequestID)] = task.ID
	if task.BillingStatus == billingStatusReserved {
		s.reserveEffects++
	}
}

func (s *pptAgentBillingTestStore) counts() (int, int, int, int, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createCalls, s.reserveEffects, s.completeCalls, s.captureEffects, s.failCalls, s.releaseEffects
}

func (s *pptAgentBillingTestStore) taskCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tasks)
}

func (s *pptAgentBillingTestStore) lastCreateRequest() createGenerationTaskRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) == 0 {
		return createGenerationTaskRequest{}
	}
	return cloneGenerationCreateRequest(s.requests[len(s.requests)-1])
}

func preparePPTAgentOutlineReadyTask(t *testing.T, harness pptAgentHTTPHarness, slideCount int) (string, string) {
	t.Helper()
	taskID := harness.createSession(t, "general", slideCount)
	harness.state.mu.Lock()
	task := harness.state.tasks[taskID]
	pages := make([]pptapp.OutlineSlide, slideCount)
	for index := range pages {
		pages[index] = pptapp.OutlineSlide{
			Page: index + 1, Title: fmt.Sprintf("Page %d", index+1), Summary: fmt.Sprintf("Summary %d", index+1),
			BulletPoints: []string{fmt.Sprintf("Point %d", index+1)}, Layout: "content", SlideType: "text_image",
		}
	}
	task.Title = "Confirmed deck"
	task.Outline = &pptapp.Outline{Title: task.Title, Slides: pages}
	task.SlideCount = slideCount
	task.Stage = pptapp.StageOutlineReady
	task.Status = pptapp.StatusPending
	harness.state.tasks[taskID] = task
	harness.state.mu.Unlock()
	return taskID, task.UserID
}

func requestPPTAgentGeneration(harness pptAgentHTTPHarness, state *pptAgentGenerationTestState, billing *pptAgentBillingTestStore, imageRunner pptAgentImageRunner, method, path, body, key string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+harness.token)
	request.Header.Set("Content-Type", "application/json")
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	ctx := context.WithValue(request.Context(), pptAgentStateContextKey{}, pptAgentStateStore(harness.state))
	ctx = context.WithValue(ctx, pptAgentGenerationStateContextKey{}, pptAgentGenerationState(state))
	ctx = context.WithValue(ctx, pptAgentBillingContextKey{}, pptAgentBillingStore(billing))
	if imageRunner == nil {
		imageRunner = func(context.Context, pptapp.Task) error { return nil }
	}
	ctx = context.WithValue(ctx, pptAgentImageRunnerContextKey{}, imageRunner)
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()
	harness.handler.ServeHTTP(response, request)
	return response
}

func waitForPPTAgentStage(t *testing.T, state *pptAgentGenerationTestState, taskID string, want pptapp.Stage) pptapp.Task {
	t.Helper()
	userID := pptAgentTaskUserID(t, state.base, taskID)
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		task, err := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
		if err != nil {
			t.Fatal(err)
		}
		if task.Stage == want {
			return task
		}
		select {
		case <-state.stageChanges:
		case <-deadline.C:
			t.Fatalf("task %s stage = %s, want %s", taskID, task.Stage, want)
		}
	}
}

func assertPPTAgentPersistedProgress(t *testing.T, state *pptAgentGenerationTestState, taskID string, wantPage, wantProgress int) {
	t.Helper()
	select {
	case page := <-state.persistedPages:
		if page != wantPage {
			t.Fatalf("persisted page = %d, want %d", page, wantPage)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for persisted page %d", wantPage)
	}
	userID := pptAgentTaskUserID(t, state.base, taskID)
	task, err := state.GetTask(context.Background(), pptAgentTestOwner(userID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Stage != pptapp.StageGenerating || task.CurrentPage != wantPage || task.Progress != wantProgress || len(task.Slides) != wantPage {
		t.Fatalf("progress after page %d = stage %s page %d progress %d slides %d", wantPage, task.Stage, task.CurrentPage, task.Progress, len(task.Slides))
	}
}

func pptAgentTaskUserID(t *testing.T, state *pptAgentTestState, taskID string) string {
	t.Helper()
	state.mu.Lock()
	defer state.mu.Unlock()
	task, ok := state.tasks[taskID]
	if !ok {
		t.Fatalf("task %s not found", taskID)
	}
	return task.UserID
}

func pptAgentTestOwner(userID string) pptapp.OwnerScope {
	return pptapp.OwnerScope{TenantID: "tenant_default", UserID: userID}
}

func findPPTAgentTestRecord(records []pptapp.IdempotencyRecord, scope, key string) int {
	for index := range records {
		if records[index].Scope == scope && records[index].Key == key {
			return index
		}
	}
	return -1
}

func decodePPTAgentGenerationResponse(t *testing.T, response *httptest.ResponseRecorder) pptTaskPublicResponse {
	t.Helper()
	var task pptTaskPublicResponse
	if err := json.Unmarshal(response.Body.Bytes(), &task); err != nil {
		t.Fatalf("decode task response: %v", err)
	}
	return task
}
