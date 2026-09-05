package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/messaging"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

// maxPPTSlideAttempts bounds total provider-side attempts for one slide
// visual across all redeliveries. Exhaustion degrades the slide to text
// instead of failing the deck (sync parity: images are best-effort).
const maxPPTSlideAttempts = 3

// RunGenerationPPTCanaryWorker consumes only the opt-in PPT canary queue.
// It mirrors RunGenerationVideoCanaryWorker: Prefetch=1, MaxConcurrency=1,
// manual ACK, retry-policy redelivery. Long work (outline, slide pool) runs
// outside any DB transaction; every stage completion commits its own durable
// checkpoint before the next stage starts.
func RunGenerationPPTCanaryWorker(ctx context.Context, cfg config.Config, db *sql.DB, manager *messaging.ConnectionManager) error {
	if db == nil || manager == nil {
		return fmt.Errorf("generation ppt worker dependencies are required")
	}
	if !generationCanaryDrainEnabled(cfg) {
		return fmt.Errorf("ASYNC_MESSAGING_ENABLED and PROVIDER_EXECUTION_SAFETY_ENABLED must be true")
	}
	store := newPostgresPrimaryStore(db, cfg.DataPath)
	a := newAPI(store, cfg, nil, nil)
	inbox := messaging.NewInboxStore(db)
	consumer := messaging.NewConsumer(manager,
		messaging.WithPrefetch(1),
		messaging.WithMaxConcurrency(1),
		messaging.WithAutoAck(false),
		messaging.WithRetryPolicy(messaging.ExchangeRetry, messaging.GenerationPPTCanaryRetryKey, 30),
		messaging.WithOnMessage(func(messageCtx context.Context, envelope *messaging.Envelope) error {
			return a.processGenerationPPTCanaryMessage(messageCtx, inbox, envelope)
		}),
	)
	if err := consumer.Start(ctx, messaging.GenerationPPTCanaryQueue); err != nil {
		return err
	}
	<-ctx.Done()
	consumer.Stop()
	return ctx.Err()
}

func (a api) processGenerationPPTCanaryMessage(ctx context.Context, inbox *messaging.InboxStore, envelope *messaging.Envelope) error {
	if envelope == nil || envelope.EventType != messaging.GenerationPPTCanaryRoutingKey || envelope.AggregateType != "generation_task" || envelope.AggregateID == "" {
		return messaging.Permanent(fmt.Errorf("invalid generation ppt canary envelope"))
	}
	taskID := envelope.AggregateID
	if value, ok := envelope.Data["task_id"].(string); ok && strings.TrimSpace(value) != taskID {
		return messaging.Permanent(fmt.Errorf("generation ppt canary task mismatch"))
	}
	shortCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := a.pgDB().BeginTx(shortCtx, nil)
	if err != nil {
		return err
	}
	duplicate, err := inbox.ClaimTx(shortCtx, tx, generationPPTCanaryConsumer, envelope.EventID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if duplicate {
		_ = tx.Rollback()
		return nil
	}
	task, err := generationTaskForUpdate(shortCtx, tx, taskID)
	if err != nil {
		_ = tx.Rollback()
		if err == sql.ErrNoRows {
			return messaging.Permanent(fmt.Errorf("generation task %s not found", taskID))
		}
		return err
	}
	if !isPPTGenerationTask(task) || !isPPTCanaryTask(task.Params) {
		_ = tx.Rollback()
		return messaging.Permanent(fmt.Errorf("generation task %s is not a ppt canary", taskID))
	}
	if !isRunningGenerationTaskStatus(task.Status) {
		if err := inbox.CompleteTx(shortCtx, tx, generationPPTCanaryConsumer, envelope.EventID, "completed", map[string]any{"task_id": taskID, "terminal": true}); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if err := a.runPPTGenerationStages(taskID, task); err != nil {
		terminal, checkErr := a.completePPTCanaryInboxIfTerminal(inbox, envelope.EventID, taskID)
		if checkErr != nil {
			return checkErr
		}
		if terminal {
			generationCanaryMetrics.failed.Add(1)
			return nil
		}
		return err
	}

	finishCtx, finishCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finishCancel()
	finishTx, err := a.pgDB().BeginTx(finishCtx, nil)
	if err != nil {
		return err
	}
	if err := inbox.CompleteTx(finishCtx, finishTx, generationPPTCanaryConsumer, envelope.EventID, "completed", map[string]any{"task_id": taskID}); err != nil {
		_ = finishTx.Rollback()
		return err
	}
	if err := finishTx.Commit(); err != nil {
		return err
	}
	generationCanaryMetrics.completed.Add(1)
	return nil
}

func isPPTGenerationTask(task generationTask) bool {
	switch strings.ToUpper(strings.TrimSpace(task.Type)) {
	case "PPT_GENERATION", "PPT":
		return true
	default:
		return false
	}
}

// runPPTGenerationStages executes LOAD → OUTLINE → SLIDE_IMAGES → SETTLE.
// PR3A does not compile the PPTX asset (PR3B); settlement lands on
// slides-complete, preserving synchronous deck-success semantics where images
// are best-effort and only a missing outline fails the deck.
func (a api) runPPTGenerationStages(taskID string, parent generationTask) error {
	ctx, cancel := context.WithTimeout(context.Background(), pptGenerationTimeout)
	a.registerGenerationTaskCancel(taskID, cancel)
	defer func() {
		a.unregisterGenerationTaskCancel(taskID)
		cancel()
	}()
	if terminal, err := a.generationTaskTerminal(ctx, taskID); err != nil || terminal {
		return err
	}
	userID := strings.TrimSpace(parent.UserID)

	if err := a.runPPTOutlineStage(ctx, userID, taskID); err != nil {
		return a.failPPTCanaryTask(userID, taskID, err)
	}
	if err := a.runPPTSlidePoolStage(ctx, userID, taskID); err != nil {
		return a.failPPTCanaryTask(userID, taskID, err)
	}
	// Reload the parent for settlement so prepared material reflects the
	// latest persisted billing snapshot, not the claim-time copy.
	parent, err := a.loadPPTParentTask(taskID)
	if err != nil {
		return a.failPPTCanaryTask(userID, taskID, err)
	}
	return a.settlePPTCanarySuccess(userID, taskID, parent)
}

// failPPTCanaryTask converges a terminally failed deck: durable release
// (exactly-once), explicit ppt detail failure, then the caller completes the
// inbox row and ACKs via completePPTCanaryInboxIfTerminal.
func (a api) failPPTCanaryTask(userID, taskID string, stageErr error) error {
	msg := generationErrorMessage(stageErr)
	if _, err := a.store.FailGenerationTaskDurable(taskID, msg); err != nil {
		return err
	}
	if _, err := a.pptService.SetDeckStatus(userID, taskID, pptapp.StatusFailed); err != nil {
		return err
	}
	return stageErr
}

// runPPTOutlineStage generates the deck outline exactly once per deck.
// Redelivery with persisted slides skips the model call entirely.
func (a api) runPPTOutlineStage(ctx context.Context, userID, taskID string) error {
	detail, err := a.pptService.GetTask(userID, taskID)
	if err != nil {
		return err
	}
	if len(detail.Slides) > 0 {
		generationCanaryMetrics.pptStageRecoveries.Add(1)
		return nil
	}
	outlineReq := pptOutlineGenerateRequest{
		Prompt:      strings.TrimSpace(detail.Prompt),
		SlideCount:  detail.SlideCount,
		Language:    detail.Language,
		TextModel:   detail.TextModel,
		ImageSource: detail.ImageSource,
		ImageModel:  detail.ImageModel,
	}
	if outlineReq.SlideCount <= 0 {
		outlineReq.SlideCount = 5
	}
	taskKey := "ppt:" + taskID + ":outline"
	model := firstNonEmptyString(detail.TextModel, a.cfg.PPTTextModel)
	semantic := pptOutlineSemanticParams(outlineReq)
	manifest, outcome, err := a.runPPTChatStageGuarded(ctx, taskKey, "configured", model, pptOutlineCapability, semantic,
		func(callCtx context.Context) ([]byte, error) {
			outline, callErr := a.generatePPTOutlineWithModel(callCtx, outlineReq)
			if callErr != nil {
				return nil, callErr
			}
			return json.Marshal(outline)
		})
	if err != nil {
		return fmt.Errorf("ppt outline stage: %w", err)
	}
	if outcome == pptChatStageReused {
		generationCanaryMetrics.pptStageRecoveries.Add(1)
		detail, err = a.pptService.GetTask(userID, taskID)
		if err != nil {
			return err
		}
		if len(detail.Slides) > 0 {
			return nil
		}
		return fmt.Errorf("ppt outline execution reused but no slides checkpointed")
	}
	var outline pptOutline
	if err := json.Unmarshal(manifest, &outline); err != nil {
		return fmt.Errorf("ppt outline manifest decode: %w", err)
	}
	if _, err := a.pptService.SetOutlineSlides(userID, taskID, outline); err != nil {
		return err
	}
	generationCanaryMetrics.pptOutlineCompleted.Add(1)
	return nil
}

// runPPTSlidePoolStage processes every slide with a bounded pool of 3.
// Completed slides (success checkpoint) are skipped, making redelivery resume
// exactly where the previous attempt stopped.
func (a api) runPPTSlidePoolStage(ctx context.Context, userID, taskID string) error {
	detail, err := a.pptService.GetTask(userID, taskID)
	if err != nil {
		return err
	}
	targets := make([]pptapp.Slide, 0, len(detail.Slides))
	for _, slide := range detail.Slides {
		if strings.TrimSpace(slide.ImageURL) != "" && strings.EqualFold(strings.TrimSpace(slide.VisualStatus), "success") {
			generationCanaryMetrics.pptSlidesSkipped.Add(1)
			continue
		}
		if !pptapp.ShouldGenerateImageForSlide(slide) && !hasRealPPTImageURL(slide.ImageURL) {
			continue
		}
		targets = append(targets, slide)
	}
	if len(targets) == 0 {
		return nil
	}
	poolCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sem := make(chan struct{}, pptSlideImagePoolSize)
	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	for _, slide := range targets {
		slide := slide
		select {
		case <-poolCtx.Done():
			break
		default:
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-poolCtx.Done():
				return
			}
			if slideErr := a.runPPTSingleSlide(poolCtx, userID, taskID, slide.ID); slideErr != nil {
				select {
				case errCh <- slideErr:
				default:
				}
				cancel()
			}
		}()
	}
	wg.Wait()
	select {
	case slideErr := <-errCh:
		return slideErr
	default:
		return nil
	}
}

// runPPTSingleSlide processes one slide: plan checkpoint → guarded image
// pipeline → success checkpoint with revision bump. Deterministic failures
// degrade the slide (sync parity); ambiguous failures return retryable errors;
// slide attempt budget (3) bounds redelivery loops before degradation.
func (a api) runPPTSingleSlide(ctx context.Context, userID, taskID, slideID string) error {
	detail, err := a.pptService.GetTask(userID, taskID)
	if err != nil {
		return err
	}
	var slide pptapp.Slide
	found := false
	for _, candidate := range detail.Slides {
		if candidate.ID == slideID {
			slide = candidate
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("ppt slide %s not found in task %s", slideID, taskID)
	}
	if strings.TrimSpace(slide.ImageURL) != "" && strings.EqualFold(strings.TrimSpace(slide.VisualStatus), "success") {
		generationCanaryMetrics.pptSlidesSkipped.Add(1)
		return nil
	}
	revision := pptSlideRevision(slide)
	slideKey := pptSlideExecKey(taskID, slideID, revision)
	if slideKey == "" {
		return fmt.Errorf("ppt slide execution identity unavailable for %s/%s", taskID, slideID)
	}

	lockKey := userID + ":" + taskID + ":" + slideID
	lockCtx, lockCancel := context.WithTimeout(ctx, 5*time.Second)
	releaseVisual, acquired, lockErr := a.tryAcquirePPTVisualOperation(lockCtx, lockKey)
	lockCancel()
	if lockErr != nil {
		return fmt.Errorf("ppt visual lock failed slide=%s: %w", slideID, lockErr)
	}
	if !acquired {
		// A sync user-regenerate holds the slide lock; redelivery later is
		// safe because no checkpoint was written for this attempt.
		return fmt.Errorf("ppt visual task active, retry later slide=%s", slideID)
	}
	defer releaseVisual()

	plan := slide.VisualPlan
	if plan == nil {
		planCtx, planCancel := context.WithTimeout(ctx, 35*time.Second)
		generated, planErr := a.generatePPTVisualPlan(planCtx, detail, slide)
		planCancel()
		if planErr == nil {
			slide.VisualPlan = &generated
			plan = &generated
			if _, updateErr := a.pptService.UpdateSlideVisualPlan(userID, taskID, slideID, generated, "", "planned", ""); updateErr != nil {
				return updateErr
			}
		}
		// Plan failure is non-fatal (sync parity): continue with nil plan.
	}

	model := pptImageProviderModel(detail.ImageModel, a.cfg.ImageModel)
	service, err := a.generationServiceForPPTImage(adminUser{ID: userID}, model)
	if err != nil {
		return a.degradePPTSlide(userID, taskID, slide, plan, err)
	}
	imgReq := pptImageGenerateRequest{
		Slide: slide, Prompt: detail.Prompt, DeckTitle: detail.Title,
		Theme: detail.Theme, Language: detail.Language,
		ImageSource: detail.ImageSource, ImageModel: detail.ImageModel,
		VisualPlan: plan, ImageStyle: detail.ImageStyle,
		PeopleStyle: detail.PeopleStyle, ImageLighting: detail.ImageLighting,
		ImageComposition: detail.ImageComposition, RetryAttempt: 1,
	}
	childClientReqID := pptChildClientRequestID(taskID, slideID, revision)
	image, err := a.generateBillablePPTImageWithKey(ctx, adminUser{ID: userID}, service, imgReq, model, taskID, slideKey, childClientReqID)
	if err != nil {
		if isPPTSlideDeterministicError(err) {
			return a.degradePPTSlide(userID, taskID, slide, plan, err)
		}
		if a.pptSlideAttemptBudgetExhausted(ctx, slideKey) {
			return a.degradePPTSlide(userID, taskID, slide, plan, err)
		}
		return fmt.Errorf("ppt slide image retryable slide=%s: %w", slideID, err)
	}
	resolvedPlan := pptapp.NormalizeVisualPlan(pptapp.VisualPlan{}, pptapp.VisualPlannerInput{SlideType: slide.SlideType, SlideTitle: slide.Title, CoreIdea: concisePPTVisualIdea(slide.Content)})
	if plan != nil {
		resolvedPlan = *plan
	}
	if _, err := a.pptService.CompleteSlideVisualWithRevision(userID, taskID, slideID, resolvedPlan, pptapp.VisualAsset{
		URL: firstNonEmptyString(image.StorageRef, image.URL), TaskID: image.TaskID,
		ModelName: model, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, revision+1); err != nil {
		return err
	}
	generationCanaryMetrics.pptSlidesCompleted.Add(1)
	return nil
}

// degradePPTSlide marks one slide failed without failing the deck (sync
// parity: slide images are best-effort background work).
func (a api) degradePPTSlide(userID, taskID string, slide pptapp.Slide, plan *pptapp.VisualPlan, cause error) error {
	resolved := pptapp.VisualPlan{}
	if plan != nil {
		resolved = *plan
	}
	if _, err := a.pptService.UpdateSlideVisualPlan(userID, taskID, slide.ID, resolved, "", "failed", generationErrorMessage(cause)); err != nil {
		return err
	}
	return nil
}

// pptSlideAttemptBudgetExhausted reports whether the slide execution already
// consumed its provider-side attempt budget across redeliveries.
func (a api) pptSlideAttemptBudgetExhausted(ctx context.Context, slideKey string) bool {
	store := pe.NewStore(a.pgDB())
	latest, err := store.GetLatestByTask(ctx, slideKey)
	if err != nil {
		return false
	}
	return latest.Attempt >= maxPPTSlideAttempts
}

// isPPTSlideDeterministicError classifies slide image failures: OCR strict
// rejections, empty provider output and definitive validation errors degrade
// the slide; everything else (timeouts, 5xx, unknown-blocked, still-processing)
// is ambiguous and must retry, never degrade-then-capture.
func isPPTSlideDeterministicError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errPPTImageContainsText) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"ppt image provider returned no image",
		"empty ppt image url",
		"authorize ppt image generation",
		"prompt is required",
		"invalid prompt",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

// settlePPTCanarySuccess converges a terminally complete deck: parent Capture
// (exactly-once via generation:capture key), explicit ppt success, then the
// caller completes the inbox row and ACKs. Deck success requires a persisted
// outline; slide visuals are best-effort (sync parity).
func (a api) settlePPTCanarySuccess(userID, taskID string, parent generationTask) error {
	detail, err := a.pptService.GetTask(userID, taskID)
	if err != nil {
		return err
	}
	if len(detail.Slides) == 0 {
		return fmt.Errorf("ppt deck has no slides to settle")
	}
	prepared := generation.CreateRequest{
		UserID: userID, Type: parent.Type, Prompt: parent.Prompt, Model: parent.Model,
		Params: cloneAnyMap(parent.Params),
	}
	if _, err := a.store.CompleteGenerationTask(taskID, prepared); err != nil {
		return err
	}
	if _, err := a.pptService.SetDeckStatus(userID, taskID, pptapp.StatusSuccess); err != nil {
		return err
	}
	return nil
}

// loadPPTParentTask reloads the parent generation task row for settlement.
func (a api) loadPPTParentTask(taskID string) (generationTask, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := a.pgDB().BeginTx(ctx, nil)
	if err != nil {
		return generationTask{}, err
	}
	defer tx.Rollback()
	task, err := generationTaskForUpdate(ctx, tx, taskID)
	if err != nil {
		return generationTask{}, err
	}
	return task, tx.Commit()
}
