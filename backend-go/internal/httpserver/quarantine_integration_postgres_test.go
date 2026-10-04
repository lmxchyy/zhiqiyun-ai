package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/app/generation"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type mockCountingQuarantineImageProvider struct {
	mu            sync.Mutex
	generateCalls int
}

func (m *mockCountingQuarantineImageProvider) DefaultModel() string { return "mock-model" }
func (m *mockCountingQuarantineImageProvider) Name() string         { return "mock-counting-image" }
func (m *mockCountingQuarantineImageProvider) Generate(ctx context.Context, req generation.CreateRequest) ([]generation.GeneratedImage, error) {
	m.mu.Lock()
	m.generateCalls++
	m.mu.Unlock()
	return []generation.GeneratedImage{{URL: "https://example.com/mock.png"}}, nil
}

type mockCountingQuarantineVideoProvider struct {
	mu          sync.Mutex
	createCalls int
	getCalls    int
}

func (m *mockCountingQuarantineVideoProvider) DefaultModel() string { return "mock-model" }
func (m *mockCountingQuarantineVideoProvider) Name() string         { return "mock-counting-video" }
func (m *mockCountingQuarantineVideoProvider) Create(ctx context.Context, req generation.CreateRequest) (any, error) {
	m.mu.Lock()
	m.createCalls++
	m.mu.Unlock()
	return map[string]any{"id": "mock-video-id"}, nil
}
func (m *mockCountingQuarantineVideoProvider) Get(ctx context.Context, requestID string) (any, error) {
	m.mu.Lock()
	m.getCalls++
	m.mu.Unlock()
	return map[string]any{"id": requestID, "status": "succeeded"}, nil
}

func seedQuarantinedTaskAndExecution(t *testing.T, db *sql.DB, taskID, userID string) (generationTask, int64) {
	t.Helper()
	ctx := context.Background()

	_, _ = db.ExecContext(ctx, `INSERT INTO xz_users (id, name, role, status) VALUES ($1, $2, 'MEMBER', 'ACTIVE') ON CONFLICT (id) DO UPDATE SET status='ACTIVE'`, userID, userID)

	genTask := generationTask{
		ID:                  taskID,
		UserID:              userID,
		Type:                "TEXT_TO_IMAGE",
		Model:               "mock-model",
		Prompt:              "quarantined test prompt",
		Status:              "PROCESSING",
		TaskStatus:          "RUNNING",
		BillingStatus:       "RESERVED",
		Progress:            50,
		PointCost:           10,
		QuotedPoints:        10,
		ReservedPoints:      10,
		Params:              map[string]any{"_provider_execution_task_id": taskID},
		ResultIDs:           []string{},
		ExecutionGeneration: 1,
		CreatedAt:           time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:           time.Now().UTC().Format(time.RFC3339Nano),
		LeaseUntil:          time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339Nano),
		WorkerID:            "",
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := insertGenerationTask(ctx, tx, genTask); err != nil {
		_ = tx.Rollback()
		t.Fatalf("insert generation task: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit generation task: %v", err)
	}

	var execID int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO provider_executions (
			task_id, provider, provider_model, capability, attempt, status,
			request_fingerprint, created_at, updated_at
		) VALUES (
			$1, 'mock-provider', 'mock-model', 'image', 1, 'prepared',
			'0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
			now(), now()
		) RETURNING id`,
		taskID,
	).Scan(&execID)
	if err != nil {
		t.Fatalf("seed provider execution: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO provider_execution_quarantine (
			execution_id, task_id, attempt, generation,
			snapshot_sha256, evidence_sha256, approval_id, release_sha,
			not_before, expires_at
		) VALUES (
			$1, $2, 1, NULL,
			'0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
			'0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
			'approval-test-01',
			'0123456789abcdef0123456789abcdef01234567',
			now() - interval '10 minutes',
			now() + interval '2 hours'
		)`,
		execID, taskID,
	)
	if err != nil {
		t.Fatalf("seed provider execution quarantine: %v", err)
	}

	return genTask, execID
}

func cleanupQuarantinedTask(db *sql.DB, taskID string, execID int64) {
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, `DELETE FROM provider_execution_quarantine WHERE execution_id=$1`, execID)
	_, _ = db.ExecContext(ctx, `DELETE FROM provider_execution_correlations WHERE execution_id=$1`, execID)
	_, _ = db.ExecContext(ctx, `DELETE FROM provider_executions WHERE id=$1`, execID)
	_, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE aggregate_id=$1`, taskID)
	_, _ = db.ExecContext(ctx, `DELETE FROM xz_generation_tasks WHERE id=$1`, taskID)
	_, _ = db.ExecContext(ctx, `DELETE FROM xz_assets WHERE task_id=$1`, taskID)
}

func TestQuarantineGenerationRecoveryBlocked(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()

	taskID := fmt.Sprintf("quarantine-rec-%d", time.Now().UnixNano())
	userID := fmt.Sprintf("quarantine-rec-u-%d", time.Now().UnixNano())
	task, execID := seedQuarantinedTaskAndExecution(t, db, taskID, userID)
	defer cleanupQuarantinedTask(db, taskID, execID)

	ctx := context.Background()
	a := newPPTDBAPI(t, config.Config{ProviderExecutionSafetyEnabled: true}, nil)

	// 1. operator_retry: rejectQuarantinedGeneration must block with ErrQuarantined
	if err := rejectQuarantinedGeneration(ctx, db, taskID, "operator_retry"); !errors.Is(err, pe.ErrQuarantined) {
		t.Fatalf("operator_retry expected ErrQuarantined, got %v", err)
	}

	// 2. operator_force_cancel: rejectQuarantinedGeneration must block with ErrQuarantined
	if err := rejectQuarantinedGeneration(ctx, db, taskID, "operator_force_cancel"); !errors.Is(err, pe.ErrQuarantined) {
		t.Fatalf("operator_force_cancel expected ErrQuarantined, got %v", err)
	}

	// 3. redrive_event: redriveGenerationEvent must block with ErrQuarantined
	_, redriveErr := a.redriveGenerationEvent(task, recoveryActionRequest{Action: recoveryActionRedrive})
	if !errors.Is(redriveErr, pe.ErrQuarantined) {
		t.Fatalf("redrive_event expected ErrQuarantined, got %v", redriveErr)
	}

	// 4. resolve_capture: resolveGenerationCapture must block with ErrQuarantined
	_, captureErr := a.resolveGenerationCapture(task, recoveryActionRequest{
		Action:   recoveryActionResolveCapture,
		Evidence: map[string]any{"providerOutcome": "succeeded", "providerRequestId": "req-1"},
	})
	if !errors.Is(captureErr, pe.ErrQuarantined) {
		t.Fatalf("resolve_capture expected ErrQuarantined, got %v", captureErr)
	}

	// 5. resolve_release: resolveGenerationRelease must block with ErrQuarantined
	_, releaseErr := a.resolveGenerationRelease(task, recoveryActionRequest{
		Action:   recoveryActionResolveRelease,
		Evidence: map[string]any{"providerOutcome": "not_submitted"},
	})
	if !errors.Is(releaseErr, pe.ErrQuarantined) {
		t.Fatalf("resolve_release expected ErrQuarantined, got %v", releaseErr)
	}

	// Verify ZERO state changes in DB
	var dbStatus, dbBillingStatus string
	if err := db.QueryRowContext(ctx, `SELECT status, billing_status FROM xz_generation_tasks WHERE id=$1`, taskID).Scan(&dbStatus, &dbBillingStatus); err != nil {
		t.Fatalf("query task status: %v", err)
	}
	if dbStatus != "PROCESSING" || dbBillingStatus != "RESERVED" {
		t.Fatalf("task state mutated: status=%s billing=%s", dbStatus, dbBillingStatus)
	}
}

func TestQuarantineFencingAndSchedulerBlocked(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()

	taskID := fmt.Sprintf("quarantine-fence-%d", time.Now().UnixNano())
	userID := fmt.Sprintf("quarantine-fence-u-%d", time.Now().UnixNano())
	_, execID := seedQuarantinedTaskAndExecution(t, db, taskID, userID)
	defer cleanupQuarantinedTask(db, taskID, execID)

	ctx := context.Background()

	var leaseBefore string
	if err := db.QueryRowContext(ctx, `SELECT lease_until FROM xz_generation_tasks WHERE id=$1`, taskID).Scan(&leaseBefore); err != nil {
		t.Fatalf("query lease_until before: %v", err)
	}

	// 1. claim_generation inside tx must block with ErrQuarantined
	tx1, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, claimErr := claimGenerationOwnershipTx(ctx, tx1, taskID, "new-worker", 30*time.Second)
	_ = tx1.Rollback()
	if !errors.Is(claimErr, pe.ErrQuarantined) {
		t.Fatalf("claim_generation expected ErrQuarantined, got %v", claimErr)
	}

	// 2. renew_generation_lease inside tx must block with ErrQuarantined
	tx2, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	renewErr := renewGenerationLeaseTx(ctx, tx2, taskID, "worker-init", 1, 30*time.Second)
	_ = tx2.Rollback()
	if !errors.Is(renewErr, pe.ErrQuarantined) {
		t.Fatalf("renew_generation_lease expected ErrQuarantined, got %v", renewErr)
	}

	// 3. scheduler_recovery: RejectTask with "scheduler_recovery" must block
	tx3, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	schedErr := pe.RejectTask(ctx, tx3, taskID, "scheduler_recovery")
	_ = tx3.Rollback()
	if !errors.Is(schedErr, pe.ErrQuarantined) {
		t.Fatalf("scheduler_recovery expected ErrQuarantined, got %v", schedErr)
	}

	// Verify ZERO lease updates
	var leaseAfter string
	if err := db.QueryRowContext(ctx, `SELECT lease_until FROM xz_generation_tasks WHERE id=$1`, taskID).Scan(&leaseAfter); err != nil {
		t.Fatalf("query lease_until after: %v", err)
	}
	if leaseAfter != leaseBefore {
		t.Fatalf("lease mutated under fencing/scheduler: before=%s after=%s", leaseBefore, leaseAfter)
	}
}

func TestQuarantineStoragePersistenceBlocked(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()

	taskID := fmt.Sprintf("quarantine-store-%d", time.Now().UnixNano())
	userID := fmt.Sprintf("quarantine-store-u-%d", time.Now().UnixNano())
	_, execID := seedQuarantinedTaskAndExecution(t, db, taskID, userID)
	defer cleanupQuarantinedTask(db, taskID, execID)

	ctx := context.Background()
	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}, payload: map[string][]byte{}}
	fileService := newTestArtifactStorageService(provider)
	a := newPPTDBAPI(t, config.Config{ProviderExecutionSafetyEnabled: true}, fileService)

	// 1. persist_generated_images must block with ErrQuarantined, 0 file objects created
	imgReq := generation.CreateRequest{
		GeneratedImages: []generation.GeneratedImage{{URL: "https://example.com/test.png", ContentType: "image/png"}},
		Params:          map[string]any{"task_id": taskID, "tenant_id": "tenant_default"},
	}
	_, storedImgs, imgErr := a.persistGeneratedImages(ctx, taskID, imgReq)
	if !errors.Is(imgErr, pe.ErrQuarantined) {
		t.Fatalf("persist_generated_images expected ErrQuarantined, got %v", imgErr)
	}
	if len(storedImgs) != 0 {
		t.Fatalf("persist_generated_images stored %d objects, expected 0", len(storedImgs))
	}

	// 2. persist_generated_videos must block with ErrQuarantined, 0 file objects created
	vidReq := generation.CreateRequest{
		Params: map[string]any{"task_id": taskID, "videoUrl": "https://example.com/test.mp4", "tenant_id": "tenant_default"},
	}
	_, storedVids, vidErr := a.persistGeneratedVideos(ctx, taskID, vidReq)
	if !errors.Is(vidErr, pe.ErrQuarantined) {
		t.Fatalf("persist_generated_videos expected ErrQuarantined, got %v", vidErr)
	}
	if len(storedVids) != 0 {
		t.Fatalf("persist_generated_videos stored %d objects, expected 0", len(storedVids))
	}

	// 3. ppt_artifact_storage must block with ErrQuarantined
	if err := rejectQuarantinedGeneration(ctx, a.pgDB(), taskID, "ppt_artifact_storage"); !errors.Is(err, pe.ErrQuarantined) {
		t.Fatalf("ppt_artifact_storage expected ErrQuarantined, got %v", err)
	}

	// 4. ppt_asset_insert must block with ErrQuarantined, 0 assets inserted
	_, assetErr := a.ensureDurablePPTAsset(ctx, taskID, userID, "tenant_default", "", "Test Deck", storagecenter.FileObject{}, "https://example.com/ppt.pptx")
	if !errors.Is(assetErr, pe.ErrQuarantined) {
		t.Fatalf("ppt_asset_insert expected ErrQuarantined, got %v", assetErr)
	}

	var assetCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_assets WHERE task_id=$1`, taskID).Scan(&assetCount); err != nil || assetCount != 0 {
		t.Fatalf("assets must be 0, got %d, err=%v", assetCount, err)
	}
	if len(provider.objects) != 0 {
		t.Fatalf("storage provider objects must be 0, got %d", len(provider.objects))
	}
}

func TestQuarantineBillingAndPersonalPointsBlocked(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()

	taskID := fmt.Sprintf("quarantine-bill-%d", time.Now().UnixNano())
	userID := fmt.Sprintf("quarantine-bill-u-%d", time.Now().UnixNano())
	accountID := "acc-" + userID
	_, execID := seedQuarantinedTaskAndExecution(t, db, taskID, userID)
	defer cleanupQuarantinedTask(db, taskID, execID)

	ctx := context.Background()
	pointStore := NewPostgresPersonalPointStore(db)

	// Grant initial 100 points
	grantCmd := PersonalPointGrantCommand{
		AccountID:      accountID,
		UserID:         userID,
		Source:         PointSourceRecharge,
		Points:         100,
		ReferenceType:  "TEST_INIT",
		ReferenceID:    "init-" + taskID,
		IdempotencyKey: "grant-init-" + taskID,
	}
	if _, err := pointStore.grant(ctx, grantCmd); err != nil {
		t.Fatalf("grant initial points: %v", err)
	}

	var availBefore, frozenBefore int64
	if err := db.QueryRowContext(ctx, `SELECT available, frozen FROM xz_point_accounts WHERE id=$1`, accountID).Scan(&availBefore, &frozenBefore); err != nil {
		t.Fatalf("query balance before: %v", err)
	}
	var ledgerCountBefore, resCountBefore int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_wallet_ledger WHERE account_id=$1`, accountID).Scan(&ledgerCountBefore)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_reservations WHERE account_id=$1`, accountID).Scan(&resCountBefore)

	// 1. reserve on quarantined point key / task must block with ErrQuarantined
	reserveCmd := PersonalPointReserveCommand{
		AccountID:       accountID,
		UserID:          userID,
		BusinessType:    "IMAGE_GENERATION",
		BusinessID:      taskID,
		RequestedPoints: 20,
		IdempotencyKey:  "generation:reserve:" + taskID,
	}
	_, resErr := pointStore.reserve(ctx, reserveCmd)
	if !errors.Is(resErr, pe.ErrQuarantined) {
		t.Fatalf("reserve expected ErrQuarantined, got %v", resErr)
	}

	// 2. capture on quarantined point key must block with ErrQuarantined
	captureCmd := PersonalPointCaptureCommand{
		AccountID:      accountID,
		UserID:         userID,
		ReservationID:  "dummy-res-id",
		Points:         20,
		IdempotencyKey: "generation:capture:" + taskID,
	}
	_, capErr := pointStore.capture(ctx, captureCmd)
	if !errors.Is(capErr, pe.ErrQuarantined) {
		t.Fatalf("capture expected ErrQuarantined, got %v", capErr)
	}

	// 3. release on quarantined point key must block with ErrQuarantined
	releaseCmd := PersonalPointReleaseCommand{
		AccountID:      accountID,
		UserID:         userID,
		ReservationID:  "dummy-res-id",
		Points:         20,
		IdempotencyKey: "generation:release:" + taskID,
	}
	_, relErr := pointStore.release(ctx, releaseCmd)
	if !errors.Is(relErr, pe.ErrQuarantined) {
		t.Fatalf("release expected ErrQuarantined, got %v", relErr)
	}

	// Verify ZERO balance changes, 0 wallet mutations, 0 ledger rows
	var availAfter, frozenAfter int64
	if err := db.QueryRowContext(ctx, `SELECT available, frozen FROM xz_point_accounts WHERE id=$1`, accountID).Scan(&availAfter, &frozenAfter); err != nil {
		t.Fatalf("query balance after: %v", err)
	}
	if availAfter != availBefore || frozenAfter != frozenBefore {
		t.Fatalf("balance mutated: before=(%d,%d) after=(%d,%d)", availBefore, frozenBefore, availAfter, frozenAfter)
	}

	var ledgerCountAfter, resCountAfter int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_wallet_ledger WHERE account_id=$1`, accountID).Scan(&ledgerCountAfter)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_reservations WHERE account_id=$1`, accountID).Scan(&resCountAfter)

	if ledgerCountAfter != ledgerCountBefore {
		t.Fatalf("ledger mutated: before=%d after=%d", ledgerCountBefore, ledgerCountAfter)
	}
	if resCountAfter != resCountBefore {
		t.Fatalf("reservations mutated: before=%d after=%d", resCountBefore, resCountAfter)
	}
}

func TestQuarantineGuardedProviderHooksBlocked(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()

	taskID := fmt.Sprintf("quarantine-guard-%d", time.Now().UnixNano())
	userID := fmt.Sprintf("quarantine-guard-u-%d", time.Now().UnixNano())
	_, execID := seedQuarantinedTaskAndExecution(t, db, taskID, userID)
	defer cleanupQuarantinedTask(db, taskID, execID)

	ctx := context.Background()
	peStore := pe.NewStore(db)

	imgProvider := &mockCountingQuarantineImageProvider{}
	vidProvider := &mockCountingQuarantineVideoProvider{}

	imgReq := generation.CreateRequest{
		Params: map[string]any{"_provider_execution_task_id": taskID, "provider": "mock-counting-image", "model": "m"},
	}
	_, imgErr := guardedImage(ctx, imgReq, imgProvider, peStore)
	if !errors.Is(imgErr, pe.ErrQuarantined) {
		t.Fatalf("guardedImage expected ErrQuarantined, got %v", imgErr)
	}
	if imgProvider.generateCalls != 0 {
		t.Fatalf("guardedImage invoked provider %d times; must be 0", imgProvider.generateCalls)
	}

	vidReq := generation.CreateRequest{
		Params: map[string]any{"_provider_execution_task_id": taskID, "provider": "mock-counting-video", "model": "m", "videoPrompt": "p"},
	}
	_, vidErr := guardedVideo(ctx, vidReq, vidProvider, peStore, nil)
	if !errors.Is(vidErr, pe.ErrQuarantined) {
		t.Fatalf("guardedVideo expected ErrQuarantined, got %v", vidErr)
	}
	if vidProvider.createCalls != 0 || vidProvider.getCalls != 0 {
		t.Fatalf("guardedVideo invoked provider (creates=%d, gets=%d); must be 0", vidProvider.createCalls, vidProvider.getCalls)
	}
}

func TestQuarantineConcurrentRaceSafetyHTTPServer(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()

	taskID := fmt.Sprintf("quarantine-race-http-%d", time.Now().UnixNano())
	userID := fmt.Sprintf("quarantine-race-u-%d", time.Now().UnixNano())
	accountID := "acc-race-" + userID
	task, execID := seedQuarantinedTaskAndExecution(t, db, taskID, userID)
	defer cleanupQuarantinedTask(db, taskID, execID)

	ctx := context.Background()
	pointStore := NewPostgresPersonalPointStore(db)

	// Grant initial 100 points
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{
		AccountID: accountID, UserID: userID, Source: PointSourceRecharge,
		Points: 100, ReferenceType: "RACE_TEST", ReferenceID: "init-" + taskID,
		IdempotencyKey: "grant-race-" + taskID,
	}); err != nil {
		t.Fatalf("grant points: %v", err)
	}

	a := newPPTDBAPI(t, config.Config{ProviderExecutionSafetyEnabled: true}, nil)

	const concurrency = 20
	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		workerIdx := i
		go func() {
			defer wg.Done()
			var opErr error
			switch workerIdx % 5 {
			case 0: // claim generation
				tx, err := db.BeginTx(ctx, nil)
				if err == nil {
					_, opErr = claimGenerationOwnershipTx(ctx, tx, taskID, fmt.Sprintf("worker-%d", workerIdx), 30*time.Second)
					_ = tx.Rollback()
				} else {
					opErr = err
				}
			case 1: // renew lease
				tx, err := db.BeginTx(ctx, nil)
				if err == nil {
					opErr = renewGenerationLeaseTx(ctx, tx, taskID, "worker-init", 1, 30*time.Second)
					_ = tx.Rollback()
				} else {
					opErr = err
				}
			case 2: // resolve capture
				_, opErr = a.resolveGenerationCapture(task, recoveryActionRequest{
					Action:   recoveryActionResolveCapture,
					Evidence: map[string]any{"providerOutcome": "succeeded", "providerRequestId": "req-race"},
				})
			case 3: // persist images
				_, _, opErr = a.persistGeneratedImages(ctx, taskID, generation.CreateRequest{
					GeneratedImages: []generation.GeneratedImage{{URL: "https://example.com/1.png"}},
					Params:          map[string]any{"task_id": taskID},
				})
			case 4: // billing reserve
				_, opErr = pointStore.reserve(ctx, PersonalPointReserveCommand{
					AccountID:       accountID,
					UserID:          userID,
					BusinessType:    "IMAGE_GENERATION",
					BusinessID:      taskID,
					RequestedPoints: 10,
					IdempotencyKey:  fmt.Sprintf("generation:reserve:%s", taskID),
				})
			}

			if !errors.Is(opErr, pe.ErrQuarantined) {
				errCh <- fmt.Errorf("worker %d returned non-quarantine error: %w", workerIdx, opErr)
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for wErr := range errCh {
		t.Errorf("race safety error: %v", wErr)
	}

	// Verify zero mutations in DB
	var dbStatus, dbBilling string
	if err := db.QueryRowContext(ctx, `SELECT status, billing_status FROM xz_generation_tasks WHERE id=$1`, taskID).Scan(&dbStatus, &dbBilling); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if dbStatus != "PROCESSING" || dbBilling != "RESERVED" {
		t.Fatalf("state mutated under race: status=%s billing=%s", dbStatus, dbBilling)
	}

	var avail, frozen int64
	if err := db.QueryRowContext(ctx, `SELECT available, frozen FROM xz_point_accounts WHERE id=$1`, accountID).Scan(&avail, &frozen); err != nil {
		t.Fatalf("query balance: %v", err)
	}
	if avail != 100 || frozen != 0 {
		t.Fatalf("points mutated under race: available=%d frozen=%d", avail, frozen)
	}
}
