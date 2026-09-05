package httpserver

import (
	"bytes"
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
	storagecenter "xianzhi-ai/backend-go/internal/storage"
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
	var fileRepository storagecenter.Repository = storagecenter.NewMemoryRepository()
	if db != nil {
		fileRepository = storagecenter.NewPostgresRepository(db)
	}
	fileService := storagecenter.NewService(fileRepository, storagecenter.S3ProviderFactory{AutoCreateBucket: cfg.StorageAutoCreateBucket}, fileCenterOptions(cfg))
	a := newAPI(store, cfg, nil, fileService)
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

// runPPTGenerationStages executes LOAD → SHORT-CIRCUIT → OUTLINE → SLIDE_IMAGES → ARTIFACT → SETTLE.
// PR3B compiles the durable PPTX asset before settlement/Capture, ensuring
// the artifact is durable and verified before points are captured.
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

	// PR3B Existing Artifact Recovery Short-Circuit:
	// If a valid durable PPTX artifact already exists and is verified readable:
	// skip outline, visual plan, slide images, buildPPTX and upload,
	// and directly enter settlement recovery.
	if ready, existingAsset, checkErr := a.checkExistingDurablePPTArtifact(ctx, userID, taskID); checkErr == nil && ready {
		generationCanaryMetrics.pptStageRecoveries.Add(1)
		_ = a.ensurePPTArtifactCheckpointed(userID, taskID, existingAsset.ID, existingAsset.URL)
		parent, err := a.loadPPTParentTask(taskID)
		if err != nil {
			return a.failPPTCanaryTask(userID, taskID, err)
		}
		return a.settlePPTCanarySuccess(userID, taskID, parent)
	}

	if err := a.runPPTOutlineStage(ctx, userID, taskID); err != nil {
		return a.failPPTCanaryTask(userID, taskID, err)
	}
	if err := a.runPPTSlidePoolStage(ctx, userID, taskID); err != nil {
		return a.failPPTCanaryTask(userID, taskID, err)
	}
	if err := a.runPPTArtifactStage(ctx, userID, taskID, parent); err != nil {
		return a.failPPTCanaryTask(userID, taskID, err)
	}
	if _, err := a.pptService.SetDeckSettling(userID, taskID); err != nil {
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
		if isPPTSlideDeterministicError(err, a.pgDB(), slideKey) {
			return a.degradePPTSlide(userID, taskID, slide, plan, err)
		}
		return fmt.Errorf("ppt slide image ambiguous/retryable slide=%s: %w", slideID, err)
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

// isPPTSlideDeterministicError classifies slide image failures: only strictly
// proven deterministic errors (such as OCR text rejection or prompt validation
// failures) with no ambiguous provider execution may safely degrade to text.
// Any execution in Submitting, Submitted, Processing, or Unknown MUST NEVER degrade.
// Retry budget exhaustion never turns an ambiguous failure into a deterministic one.
func isPPTSlideDeterministicError(err error, db *sql.DB, slideKey string) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, pe.ErrUnknownResubmitBlocked) ||
		errors.Is(err, pe.ErrProviderStillProcessing) ||
		errors.Is(err, pe.ErrTransitionConflict) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) {
		return false
	}
	if db != nil && slideKey != "" {
		store := pe.NewStore(db)
		if latest, getErr := store.GetLatestByTask(context.Background(), slideKey); getErr == nil {
			switch latest.Status {
			case pe.Submitting, pe.Submitted, pe.Processing, pe.Unknown:
				return false
			case pe.Failed:
				if latest.ErrorClass == nil || (*latest.ErrorClass != string(pe.DefinitiveNotSubmitted) && *latest.ErrorClass != string(pe.RetryableBeforeSubmit)) {
					return false
				}
			}
		}
	}
	if errors.Is(err, errPPTImageContainsText) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, ambiguousMarker := range []string{
		"unknown", "timeout", "deadline", "connection", "network",
		"500", "502", "503", "504", "rate limit", "429", "resubmit blocked", "still processing",
	} {
		if strings.Contains(message, ambiguousMarker) {
			return false
		}
	}
	for _, deterministicMarker := range []string{
		"ppt image provider returned no image",
		"empty ppt image url",
		"authorize ppt image generation",
		"prompt is required",
		"invalid prompt",
	} {
		if strings.Contains(message, deterministicMarker) {
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
	if prepared.Params == nil {
		prepared.Params = map[string]any{}
	}
	if detail.StorageRef != "" {
		prepared.Params["storageRef"] = detail.StorageRef
		prepared.Params["pptUrl"] = detail.StorageRef
	} else if detail.PPTURL != "" {
		prepared.Params["pptUrl"] = detail.PPTURL
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
	if db := a.pgDB(); db != nil {
		tx, err := db.BeginTx(ctx, nil)
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
	tasks, err := a.store.ListGenerationTasks()
	if err != nil {
		return generationTask{}, err
	}
	for _, task := range tasks {
		if task.ID == taskID {
			return task, nil
		}
	}
	return generationTask{}, fmt.Errorf("generation task %s not found", taskID)
}

// runPPTArtifactStage compiles and stores the durable PPTX artifact before settlement.
// Artifact durable BEFORE Capture invariant guarantees that users are never billed
// if the artifact build or storage upload fails.
func (a api) runPPTArtifactStage(ctx context.Context, userID, taskID string, parent generationTask) error {
	if ready, _, err := a.checkExistingDurablePPTArtifact(ctx, userID, taskID); err == nil && ready {
		generationCanaryMetrics.pptStageRecoveries.Add(1)
		return nil
	}
	detail, err := a.pptService.GetTask(userID, taskID)
	if err != nil {
		return fmt.Errorf("get ppt task for artifact: %w", err)
	}
	materialized := a.materializePPTTaskVisualURLs(ctx, adminUser{ID: userID}, detail)
	payload, err := buildPPTX(materialized)
	if err != nil {
		return fmt.Errorf("build pptx artifact: %w", err)
	}
	if a.fileService == nil {
		return fmt.Errorf("file storage service is required for durable pptx artifact")
	}
	tenantID := firstNonEmptyString(stringValue(parent.Params["tenant_id"]), "tenant_default")
	available, err := a.fileService.StorageAvailable(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("check ppt storage available: %w", err)
	}
	if !available {
		return fmt.Errorf("storage is not available for tenant %s", tenantID)
	}
	fileName := fmt.Sprintf("%s.pptx", taskID)
	file, err := a.fileService.StoreObjectIdempotent(ctx, storagecenter.UploadInitInput{
		TenantID:     tenantID,
		UserID:       userID,
		FileName:     fileName,
		FileSize:     int64(len(payload)),
		MIMEType:     "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		BusinessType: "generation_result",
		BusinessID:   taskID,
		Visibility:   "PRIVATE",
	}, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("store pptx artifact: %w", err)
	}
	storageRef := pptStorageReference(file)
	assetItem, err := a.ensureDurablePPTAsset(ctx, taskID, userID, tenantID, stringValue(parent.Params["organization_id"]), detail.Title, file, storageRef)
	if err != nil {
		return fmt.Errorf("ensure durable ppt asset: %w", err)
	}
	if _, err := a.pptService.SetDeckArtifactReady(userID, taskID, assetItem.ID, storageRef); err != nil {
		return fmt.Errorf("checkpoint ppt artifact: %w", err)
	}
	return nil
}

// checkExistingDurablePPTArtifact checks if a valid durable PPTX asset already
// exists and is verified readable in object storage.
func (a api) checkExistingDurablePPTArtifact(ctx context.Context, userID, taskID string) (bool, asset, error) {
	existingAsset, found, err := a.findDurablePPTAsset(ctx, taskID)
	if err != nil || !found {
		return false, asset{}, err
	}
	if userID != "" && existingAsset.UserID != "" && existingAsset.UserID != userID {
		return false, asset{}, fmt.Errorf("ppt asset user mismatch: want %s got %s", userID, existingAsset.UserID)
	}
	if a.fileService != nil {
		storageRef := firstNonEmptyString(existingAsset.URL, stringValue(existingAsset.Metadata["storageRef"]))
		tenantID, fileID, ok := parsePPTStorageReference(storageRef)
		if !ok {
			tenantID = firstNonEmptyString(stringValue(existingAsset.Metadata["storageTenantId"]), stringValue(existingAsset.Metadata["tenantId"]), "tenant_default")
			fileID = firstNonEmptyString(stringValue(existingAsset.Metadata["storageFileId"]), stringValue(existingAsset.Metadata["fileId"]))
		}
		if fileID == "" {
			return false, asset{}, fmt.Errorf("ppt asset missing storage file reference")
		}
		access := storagecenter.AccessContext{
			TenantID: tenantID,
			UserID:   existingAsset.UserID,
			IsAdmin:  true,
		}
		_, reader, openErr := a.fileService.OpenObject(ctx, access, fileID)
		if openErr != nil {
			return false, asset{}, fmt.Errorf("ppt durable artifact unreadable: %w", openErr)
		}
		_ = reader.Close()
	}
	return true, existingAsset, nil
}

// findDurablePPTAsset looks up an active PPT asset in xz_assets for taskID.
func (a api) findDurablePPTAsset(ctx context.Context, taskID string) (asset, bool, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return asset{}, false, nil
	}
	if db := a.pgDB(); db != nil {
		var existing asset
		var metadataRaw []byte
		err := db.QueryRowContext(ctx, `
			SELECT id, user_id, coalesce(tenant_id,''), coalesce(organization_id,''), task_id, name, media_type, url,
			       coalesce(thumbnail_url,''), favorite, metadata, coalesce(deleted_at::text,''), created_at::text, updated_at::text
			FROM xz_assets
			WHERE task_id = $1 AND media_type = 'ppt' AND deleted_at IS NULL
			ORDER BY created_at DESC LIMIT 1
		`, taskID).Scan(&existing.ID, &existing.UserID, &existing.TenantID, &existing.OrganizationID, &existing.TaskID,
			&existing.Name, &existing.MediaType, &existing.URL, &existing.ThumbnailURL, &existing.Favorite, &metadataRaw,
			&existing.DeletedAt, &existing.CreatedAt, &existing.UpdatedAt)
		if err == nil {
			_ = json.Unmarshal(metadataRaw, &existing.Metadata)
			return existing, true, nil
		}
		if errors.Is(err, sql.ErrNoRows) {
			return asset{}, false, nil
		}
		return asset{}, false, err
	}
	assets, err := a.store.ListAssets()
	if err != nil {
		return asset{}, false, err
	}
	for _, it := range assets {
		if it.TaskID == taskID && it.MediaType == "ppt" && it.DeletedAt == "" {
			return it, true, nil
		}
	}
	return asset{}, false, nil
}

// ensureDurablePPTAsset inserts or reuses a single logical PPT asset in xz_assets.
// Deterministic assetID (asset_ppt_<taskID>) and ON CONFLICT update ensure
// crash redeliveries never create duplicate asset rows.
func (a api) ensureDurablePPTAsset(ctx context.Context, taskID, userID, tenantID, organizationID, title string, file storagecenter.FileObject, storageRef string) (asset, error) {
	if existing, found, err := a.findDurablePPTAsset(ctx, taskID); err == nil && found {
		return existing, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	assetID := fmt.Sprintf("asset_ppt_%s", taskID)
	name := pptxDownloadFileName(pptapp.Task{Title: title})
	item := asset{
		ID:             assetID,
		UserID:         userID,
		TenantID:       tenantID,
		OrganizationID: organizationID,
		TaskID:         taskID,
		Name:           name,
		MediaType:      "ppt",
		URL:            storageRef,
		Favorite:       false,
		Metadata: map[string]any{
			"index":            "1",
			"fileId":           file.FileID,
			"storageFileId":    file.FileID,
			"storageTenantId":  file.TenantID,
			"storageProvider":  file.Provider,
			"storageBucket":    file.Bucket,
			"storageObjectKey": file.ObjectKey,
			"fileSize":         file.FileSize,
			"fileSizeBytes":    file.FileSize,
			"contentType":      "application/vnd.openxmlformats-officedocument.presentationml.presentation",
			"source":           "ppt_async",
			"type":             "PPT_GENERATION",
			"storageManaged":   true,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	return a.store.SaveUploadedAsset(item)
}

// ensurePPTArtifactCheckpointed writes the PPTX checkpoint to xz_ppt_tasks
// if not already present.
func (a api) ensurePPTArtifactCheckpointed(userID, taskID, assetID, storageRef string) error {
	detail, err := a.pptService.GetTask(userID, taskID)
	if err != nil {
		return err
	}
	if detail.ArtifactStatus == "ready" && detail.AssetID != "" && detail.StorageRef != "" {
		return nil
	}
	_, err = a.pptService.SetDeckArtifactReady(userID, taskID, assetID, storageRef)
	return err
}
