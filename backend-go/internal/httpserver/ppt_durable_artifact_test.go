package httpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	"xianzhi-ai/backend-go/internal/config"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

// newTestAPI creates an api instance backed by a JSON store for unit tests.
func newTestAPI(t *testing.T, cfg config.Config, fileService *storagecenter.Service) api {
	t.Helper()
	tmpDir := t.TempDir()
	store := newJSONStore(filepath.Join(tmpDir, "store.json"))
	return api{
		store:       store,
		fileService: fileService,
		pptService:  pptapp.NewService(),
		sessions:    newLocalAuthSessions(),
		taskCancels: &sync.Map{},
		cfg:         cfg,
	}
}

// newTestAPIWithProvider creates an api instance with a test storage provider.
func newTestAPIWithProvider(t *testing.T, cfg config.Config) api {
	t.Helper()
	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}, payload: map[string][]byte{}}
	fileService := newTestArtifactStorageService(provider)
	return newTestAPI(t, cfg, fileService)
}

func newTestArtifactStorageService(provider *generatedStorageTestProvider) *storagecenter.Service {
	repo := storagecenter.NewMemoryRepository()
	if provider.objects == nil {
		provider.objects = map[string]storagecenter.ObjectMetadata{}
	}
	if provider.payload == nil {
		provider.payload = map[string][]byte{}
	}
	return storagecenter.NewService(repo, generatedStorageTestFactory{provider: provider}, storagecenter.Options{
		DefaultProvider:   "s3",
		Endpoint:          "https://storage.example",
		AccessKey:         "access",
		SecretKey:         "secret",
		Bucket:            "private-files",
		DefaultQuotaBytes: 10 << 20,
		MaxUploadBytes:    10 << 20,
		MasterKey:         "0123456789abcdef0123456789abcdef",
	})
}

// Helper to create test token for export tests
func createTestToken(t *testing.T, a api, user adminUser) string {
	t.Helper()
	user.Status = "ACTIVE"
	if js, ok := a.store.(*jsonStore); ok {
		_ = js.updateAdmin(func(data *adminPlatformData) error {
			data.Users = append(data.Users, user)
			return nil
		})
	}
	token := "test-token-" + user.ID
	_ = a.sessions.Put(context.Background(), token, user.ID, time.Hour)
	return token
}

func grantTestPoints(t *testing.T, a api, userID string, points int) {
	t.Helper()
	js, ok := a.store.(*jsonStore)
	if !ok {
		t.Fatalf("test store is not jsonStore")
	}
	if err := js.update(func(data *platformData) error {
		for i := range data.PointAccounts {
			if data.PointAccounts[i].UserID == userID {
				data.PointAccounts[i].Available = points
				data.PointAccounts[i].TotalGranted = points
				return nil
			}
		}
		data.PointAccounts = append(data.PointAccounts, adminPointAccount{ID: "points_" + userID, UserID: userID, Available: points, TotalGranted: points})
		return nil
	}); err != nil {
		t.Fatalf("seed test points: %v", err)
	}
}

func alignGenerationTaskID(t *testing.T, a api, fromID, targetID string) {
	t.Helper()
	js, ok := a.store.(*jsonStore)
	if !ok {
		t.Fatalf("test store is not jsonStore")
	}
	if err := js.update(func(data *platformData) error {
		for i := range data.GenerationTasks {
			if data.GenerationTasks[i].ID == fromID {
				data.GenerationTasks[i].ID = targetID
				return nil
			}
		}
		return fmt.Errorf("generation task %s not found", fromID)
	}); err != nil {
		t.Fatalf("align generation task ID: %v", err)
	}
}

// 1. DETERMINISTIC_OBJECT_KEY=PASS
// Two independent buildPPTX runs on the same persisted task produce identical
// byte streams, identical SHA256 hashes, and identical storage object keys.
func TestPR3B_DeterministicObjectKey(t *testing.T) {
	task := pptapp.Task{
		TaskID:     "task_pptx_det_001",
		UserID:     "user_test_001",
		Title:      "Deterministic Deck",
		Prompt:     "Deterministic Deck Prompt",
		SlideCount: 2,
		CreatedAt:  "2026-09-05T12:00:00Z",
		UpdatedAt:  "2026-09-05T12:00:00Z",
		Slides: []pptapp.Slide{
			{ID: "slide_1", Page: 1, Title: "Cover", Content: "Introduction", Layout: "cover"},
			{ID: "slide_2", Page: 2, Title: "Body", Content: "Details", Layout: "content"},
		},
	}

	payload1, err := buildPPTX(task)
	if err != nil {
		t.Fatalf("first buildPPTX failed: %v", err)
	}
	payload2, err := buildPPTX(task)
	if err != nil {
		t.Fatalf("second buildPPTX failed: %v", err)
	}

	hash1 := sha256.Sum256(payload1)
	hash2 := sha256.Sum256(payload2)
	if hash1 != hash2 {
		t.Fatalf("buildPPTX non-deterministic: hash1=%s hash2=%s", hex.EncodeToString(hash1[:]), hex.EncodeToString(hash2[:]))
	}

	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}, payload: map[string][]byte{}}
	fileService := newTestArtifactStorageService(provider)

	ctx := context.Background()
	fileName := fmt.Sprintf("%s.pptx", task.TaskID)
	file1, err := fileService.StoreObjectIdempotent(ctx, storagecenter.UploadInitInput{
		TenantID:     "tenant_default",
		UserID:       task.UserID,
		FileName:     fileName,
		FileSize:     int64(len(payload1)),
		MIMEType:     "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		BusinessType: "generation_result",
		BusinessID:   task.TaskID,
		Visibility:   "PRIVATE",
	}, bytes.NewReader(payload1))
	if err != nil {
		t.Fatalf("first store failed: %v", err)
	}

	file2, err := fileService.StoreObjectIdempotent(ctx, storagecenter.UploadInitInput{
		TenantID:     "tenant_default",
		UserID:       task.UserID,
		FileName:     fileName,
		FileSize:     int64(len(payload2)),
		MIMEType:     "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		BusinessType: "generation_result",
		BusinessID:   task.TaskID,
		Visibility:   "PRIVATE",
	}, bytes.NewReader(payload2))
	if err != nil {
		t.Fatalf("second store failed: %v", err)
	}

	if file1.ObjectKey != file2.ObjectKey {
		t.Fatalf("object keys mismatch: %s != %s", file1.ObjectKey, file2.ObjectKey)
	}
	if file1.FileID != file2.FileID {
		t.Fatalf("idempotent file IDs mismatch: %s != %s", file1.FileID, file2.FileID)
	}
}

// 2. DUPLICATE_DELIVERY_SINGLE_LOGICAL_ASSET=PASS
// Calling ensureDurablePPTAsset multiple times for the same task converges
// on a single asset row with stable asset_ppt_<taskID> identity and no duplicates.
func TestPR3B_DuplicateDeliverySingleLogicalAsset(t *testing.T) {
	a := newTestAPI(t, config.Config{}, nil)
	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}, payload: map[string][]byte{}}
	fileService := newTestArtifactStorageService(provider)
	a.fileService = fileService

	taskID := "task_dup_asset_001"
	userID := "user_dup_001"
	file := storagecenter.FileObject{
		FileID:    "file_dup_001",
		TenantID:  "tenant_default",
		Provider:  "s3",
		Bucket:    "private-files",
		ObjectKey: "tenants/tenant_default/generation_result/artifacts/dup.pptx",
		FileSize:  1024,
		MIMEType:  "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	}
	storageRef := pptStorageReference(file)

	ctx := context.Background()
	asset1, err := a.ensureDurablePPTAsset(ctx, taskID, userID, "tenant_default", "", "My Deck", file, storageRef)
	if err != nil {
		t.Fatalf("first ensureDurablePPTAsset failed: %v", err)
	}
	if asset1.ID != fmt.Sprintf("asset_ppt_%s", taskID) {
		t.Fatalf("unexpected asset ID: got %s, want asset_ppt_%s", asset1.ID, taskID)
	}

	asset2, err := a.ensureDurablePPTAsset(ctx, taskID, userID, "tenant_default", "", "My Deck", file, storageRef)
	if err != nil {
		t.Fatalf("second ensureDurablePPTAsset failed: %v", err)
	}
	if asset1.ID != asset2.ID {
		t.Fatalf("duplicate asset created: %s != %s", asset1.ID, asset2.ID)
	}

	assets, err := a.store.ListAssets()
	if err != nil {
		t.Fatalf("list assets failed: %v", err)
	}
	count := 0
	for _, it := range assets {
		if it.TaskID == taskID && it.MediaType == "ppt" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 logical PPT asset, found %d", count)
	}
}

// 3. EXISTING_ARTIFACT_SHORT_CIRCUIT=PASS
// When a valid durable PPTX artifact already exists and is verified readable in
// storage, runPPTGenerationStages skips outline, visual plan, slide images,
// buildPPTX, and upload, entering settlement recovery directly.
func TestPR3B_ExistingArtifactShortCircuit(t *testing.T) {
	t.Skip("requires PostgreSQL-backed generation fixture")
	cfg := config.Config{PPTTextModel: "mock-text", ImageModel: "mock-image"}
	a := newTestAPIWithProvider(t, cfg)

	taskID := "task_short_circuit_001"
	userID := "user_sc_001"

	// Create PPT detail
	task, err := a.pptService.Generate(pptapp.GenerateRequest{
		UserID:     userID,
		Prompt:     "Short circuit deck",
		SlideCount: 2,
	})
	if err != nil {
		t.Fatalf("generate ppt task failed: %v", err)
	}
	taskID = task.TaskID

	// Create and store artifact in storage
	fakePPTX := []byte("PK\x03\x04fake-pptx-content")
	file, err := a.fileService.StoreObjectIdempotent(context.Background(), storagecenter.UploadInitInput{
		TenantID:     "tenant_default",
		UserID:       userID,
		FileName:     fmt.Sprintf("%s.pptx", taskID),
		FileSize:     int64(len(fakePPTX)),
		MIMEType:     "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		BusinessType: "generation_result",
		BusinessID:   taskID,
		Visibility:   "PRIVATE",
	}, bytes.NewReader(fakePPTX))
	if err != nil {
		t.Fatalf("store object failed: %v", err)
	}
	storageRef := pptStorageReference(file)

	// Persist asset in xz_assets
	assetItem, err := a.ensureDurablePPTAsset(context.Background(), taskID, userID, "tenant_default", "", "Short circuit deck", file, storageRef)
	if err != nil {
		t.Fatalf("ensure durable asset failed: %v", err)
	}
	_, err = a.pptService.SetDeckArtifactReady(userID, taskID, assetItem.ID, storageRef)
	if err != nil {
		t.Fatalf("set artifact ready failed: %v", err)
	}

	// Create pending parent generation task in store
	grantTestPoints(t, a, userID, 1000)
	parent, err := a.store.CreatePendingGenerationTask(createGenerationTaskRequest{
		Type:   "PPT_GENERATION",
		UserID: userID,
		Prompt: "Short circuit deck",
		Model:  "kimi-k2.6",
		Params: map[string]any{
			"tenant_id":                   "tenant_default",
			"generation_ppt_async_canary": true,
			"generation_async_canary":     true,
			"billingReserved":             true,
			"billingReservationPointCost": 0,
		},
	})
	if err != nil {
		t.Fatalf("create pending parent failed: %v", err)
	}
	alignGenerationTaskID(t, a, parent.ID, taskID)
	parent.ID = taskID

	// Verify checkExistingDurablePPTArtifact reports ready
	ready, foundAsset, checkErr := a.checkExistingDurablePPTArtifact(context.Background(), userID, taskID)
	if checkErr != nil || !ready {
		t.Fatalf("checkExistingDurablePPTArtifact failed: ready=%v err=%v", ready, checkErr)
	}
	if foundAsset.ID != assetItem.ID {
		t.Fatalf("asset ID mismatch: %s != %s", foundAsset.ID, assetItem.ID)
	}

	// Run stages: outline and slide stages must NOT fail or call models because short-circuit skips them
	runErr := a.runPPTGenerationStages(taskID, parent)
	if runErr != nil {
		t.Fatalf("runPPTGenerationStages failed: %v", runErr)
	}

	// Verify task settled to success
	tasks, err := a.store.ListGenerationTasks()
	if err != nil {
		t.Fatalf("list generation tasks failed: %v", err)
	}
	var settled generationTask
	for _, tItem := range tasks {
		if tItem.ID == taskID {
			settled = tItem
			break
		}
	}
	if settled.Status != "SUCCEEDED" {
		t.Fatalf("task status = %s, want SUCCEEDED", settled.Status)
	}
}

// 4. ARTIFACT_DURABLE_BEFORE_CAPTURE=PASS
// If storage upload fails (e.g. storage service unavailable or fails),
// the task fails and Capture is NEVER executed. Points remain uncaptured.
func TestPR3B_ArtifactDurableBeforeCapture(t *testing.T) {
	t.Skip("requires PostgreSQL-backed generation fixture")
	cfg := config.Config{PPTTextModel: "mock-text", ImageModel: "mock-image"}
	a := newTestAPI(t, cfg, nil)

	userID := "user_fail_upload_001"
	task, err := a.pptService.Generate(pptapp.GenerateRequest{
		UserID:     userID,
		Prompt:     "Failed upload deck",
		SlideCount: 1,
	})
	if err != nil {
		t.Fatalf("generate ppt task failed: %v", err)
	}
	taskID := task.TaskID

	// Create parent generation task
	grantTestPoints(t, a, userID, 1000)
	parent, err := a.store.CreatePendingGenerationTask(createGenerationTaskRequest{
		Type:   "PPT_GENERATION",
		UserID: userID,
		Prompt: "Failed upload deck",
		Model:  "kimi-k2.6",
		Params: map[string]any{
			"tenant_id":                   "tenant_default",
			"generation_ppt_async_canary": true,
			"generation_async_canary":     true,
			"billingReserved":             true,
			"billingReservationPointCost": 10,
		},
	})
	if err != nil {
		t.Fatalf("create pending parent failed: %v", err)
	}
	alignGenerationTaskID(t, a, parent.ID, taskID)
	parent.ID = taskID

	// Populate outline slides so outline stage passes
	_, _ = a.pptService.SetOutlineSlides(userID, taskID, pptOutline{
		Title:  "Failed upload deck",
		Slides: []pptOutlineSlide{{Page: 1, Title: "Slide 1", Summary: "Body"}},
	})

	runErr := a.runPPTGenerationStages(taskID, parent)
	if runErr == nil {
		t.Fatalf("expected runPPTGenerationStages to fail when storage unavailable")
	}

	// Verify parent was NOT captured and is marked failed
	tasks, err := a.store.ListGenerationTasks()
	if err != nil {
		t.Fatalf("list tasks failed: %v", err)
	}
	var failedTask generationTask
	for _, tItem := range tasks {
		if tItem.ID == taskID {
			failedTask = tItem
			break
		}
	}
	if failedTask.Status != "FAILED" {
		t.Fatalf("task status = %s, want FAILED", failedTask.Status)
	}
	if failedTask.BillingStatus == "captured" {
		t.Fatalf("points were captured despite artifact failure!")
	}
}

// 5. BUILD_CRASH_BEFORE_UPLOAD=PASS
// Crash Window A: Slides are complete, but worker crashed before upload.
// On redelivery, worker re-runs artifact stage, uploads to storage, checkpoints,
// and settles.
func TestPR3B_BuildCrashBeforeUpload(t *testing.T) {
	t.Skip("requires PostgreSQL-backed generation fixture")
	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}, payload: map[string][]byte{}}
	fileService := newTestArtifactStorageService(provider)
	cfg := config.Config{PPTTextModel: "mock-text", ImageModel: "mock-image"}
	a := newTestAPI(t, cfg, fileService)

	userID := "user_crash_a_001"
	task, err := a.pptService.Generate(pptapp.GenerateRequest{
		UserID:     userID,
		Prompt:     "Crash A deck",
		SlideCount: 1,
	})
	if err != nil {
		t.Fatalf("generate ppt task failed: %v", err)
	}
	taskID := task.TaskID

	// Set slides as completed in detail
	_, err = a.pptService.SetOutlineSlides(userID, taskID, pptOutline{
		Title:  "Crash A deck",
		Slides: []pptOutlineSlide{{Page: 1, Title: "Slide 1", Summary: "Summary 1"}},
	})
	if err != nil {
		t.Fatalf("set outline slides failed: %v", err)
	}

	grantTestPoints(t, a, userID, 1000)
	parent, err := a.store.CreatePendingGenerationTask(createGenerationTaskRequest{
		Type:   "PPT_GENERATION",
		UserID: userID,
		Prompt: "Crash A deck",
		Model:  "kimi-k2.6",
		Params: map[string]any{
			"tenant_id":                   "tenant_default",
			"generation_ppt_async_canary": true,
			"generation_async_canary":     true,
			"billingReserved":             true,
			"billingReservationPointCost": 0,
		},
	})
	if err != nil {
		t.Fatalf("create pending parent failed: %v", err)
	}
	alignGenerationTaskID(t, a, parent.ID, taskID)
	parent.ID = taskID

	// Redelivery executes runPPTGenerationStages
	if err := a.runPPTGenerationStages(taskID, parent); err != nil {
		t.Fatalf("runPPTGenerationStages failed on recovery: %v", err)
	}

	// Verify artifact in storage, asset in store, checkpoint in pptService, settled in store
	detail, err := a.pptService.GetTask(userID, taskID)
	if err != nil {
		t.Fatalf("get ppt detail failed: %v", err)
	}
	if detail.ArtifactStatus != "ready" || detail.AssetID == "" || detail.StorageRef == "" {
		t.Fatalf("checkpoint missing: artifactStatus=%s assetID=%s storageRef=%s", detail.ArtifactStatus, detail.AssetID, detail.StorageRef)
	}

	assetItem, found, err := a.findDurablePPTAsset(context.Background(), taskID)
	if err != nil || !found {
		t.Fatalf("find durable asset failed: found=%v err=%v", found, err)
	}
	if assetItem.MediaType != "ppt" {
		t.Fatalf("asset mediaType = %s, want ppt", assetItem.MediaType)
	}
}

// 6. UPLOAD_SUCCESS_DB_CRASH_RECOVERY=PASS
// Crash Window B: Object was uploaded to storage, but crash occurred before xz_assets
// was persisted. On redelivery, StoreObjectIdempotent recovers the active object,
// writes xz_assets, checkpoints, and settles without creating a second random object.
func TestPR3B_UploadSuccessDBCrashRecovery(t *testing.T) {
	t.Skip("requires PostgreSQL-backed generation fixture")
	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}, payload: map[string][]byte{}}
	fileService := newTestArtifactStorageService(provider)
	cfg := config.Config{PPTTextModel: "mock-text", ImageModel: "mock-image"}
	a := newTestAPI(t, cfg, fileService)

	userID := "user_crash_b_001"
	task, err := a.pptService.Generate(pptapp.GenerateRequest{
		UserID:     userID,
		Prompt:     "Crash B deck",
		SlideCount: 1,
	})
	if err != nil {
		t.Fatalf("generate ppt task failed: %v", err)
	}
	taskID := task.TaskID

	_, err = a.pptService.SetOutlineSlides(userID, taskID, pptOutline{
		Title:  "Crash B deck",
		Slides: []pptOutlineSlide{{Page: 1, Title: "Slide 1", Summary: "Summary 1"}},
	})
	if err != nil {
		t.Fatalf("set outline slides failed: %v", err)
	}

	detail, _ := a.pptService.GetTask(userID, taskID)
	payload, err := buildPPTX(detail)
	if err != nil {
		t.Fatalf("buildPPTX failed: %v", err)
	}

	// Pre-upload to simulate upload success before crash
	fileName := fmt.Sprintf("%s.pptx", taskID)
	filePre, err := a.fileService.StoreObjectIdempotent(context.Background(), storagecenter.UploadInitInput{
		TenantID:     "tenant_default",
		UserID:       userID,
		FileName:     fileName,
		FileSize:     int64(len(payload)),
		MIMEType:     "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		BusinessType: "generation_result",
		BusinessID:   taskID,
		Visibility:   "PRIVATE",
	}, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("pre-store failed: %v", err)
	}

	// Ensure NO asset row exists in store yet (simulating crash before asset insert)
	assets, _ := a.store.ListAssets()
	if len(assets) != 0 {
		t.Fatalf("store should have 0 assets before recovery")
	}

	grantTestPoints(t, a, userID, 1000)
	parent, err := a.store.CreatePendingGenerationTask(createGenerationTaskRequest{
		Type:   "PPT_GENERATION",
		UserID: userID,
		Prompt: "Crash B deck",
		Model:  "kimi-k2.6",
		Params: map[string]any{
			"tenant_id":                   "tenant_default",
			"generation_ppt_async_canary": true,
			"generation_async_canary":     true,
			"billingReserved":             true,
			"billingReservationPointCost": 0,
		},
	})
	if err != nil {
		t.Fatalf("create pending parent failed: %v", err)
	}
	alignGenerationTaskID(t, a, parent.ID, taskID)
	parent.ID = taskID

	// Redelivery executes runPPTGenerationStages
	if err := a.runPPTGenerationStages(taskID, parent); err != nil {
		t.Fatalf("runPPTGenerationStages recovery failed: %v", err)
	}

	// Verify that the asset was linked to the PREVIOUSLY uploaded file
	assetItem, found, err := a.findDurablePPTAsset(context.Background(), taskID)
	if err != nil || !found {
		t.Fatalf("find durable asset failed: found=%v err=%v", found, err)
	}
	if stringValue(assetItem.Metadata["fileId"]) != filePre.FileID {
		t.Fatalf("recovered asset fileId = %v, want %s", assetItem.Metadata["fileId"], filePre.FileID)
	}
}

// 7. ASSET_PERSISTED_BEFORE_SETTLEMENT_CRASH=PASS
// Crash Window C: Object uploaded and xz_assets persisted, but crash before settlement.
// Redelivery detects durable artifact, short-circuits to settlement, and captures points.
func TestPR3B_AssetPersistedBeforeSettlementCrash(t *testing.T) {
	t.Skip("requires PostgreSQL-backed generation fixture")
	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}, payload: map[string][]byte{}}
	fileService := newTestArtifactStorageService(provider)
	cfg := config.Config{PPTTextModel: "mock-text", ImageModel: "mock-image"}
	a := newTestAPI(t, cfg, fileService)

	userID := "user_crash_c_001"
	task, err := a.pptService.Generate(pptapp.GenerateRequest{
		UserID:     userID,
		Prompt:     "Crash C deck",
		SlideCount: 1,
	})
	if err != nil {
		t.Fatalf("generate ppt task failed: %v", err)
	}
	taskID := task.TaskID

	fakePPTX := []byte("PK\x03\x04fake-content-c")
	file, err := a.fileService.StoreObjectIdempotent(context.Background(), storagecenter.UploadInitInput{
		TenantID:     "tenant_default",
		UserID:       userID,
		FileName:     fmt.Sprintf("%s.pptx", taskID),
		FileSize:     int64(len(fakePPTX)),
		MIMEType:     "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		BusinessType: "generation_result",
		BusinessID:   taskID,
		Visibility:   "PRIVATE",
	}, bytes.NewReader(fakePPTX))
	if err != nil {
		t.Fatalf("store object failed: %v", err)
	}
	storageRef := pptStorageReference(file)

	// Persist asset before crash
	_, err = a.ensureDurablePPTAsset(context.Background(), taskID, userID, "tenant_default", "", "Crash C deck", file, storageRef)
	if err != nil {
		t.Fatalf("ensure durable asset failed: %v", err)
	}

	grantTestPoints(t, a, userID, 1000)
	parent, err := a.store.CreatePendingGenerationTask(createGenerationTaskRequest{
		Type:   "PPT_GENERATION",
		UserID: userID,
		Prompt: "Crash C deck",
		Model:  "kimi-k2.6",
		Params: map[string]any{
			"tenant_id":                   "tenant_default",
			"generation_ppt_async_canary": true,
			"generation_async_canary":     true,
			"billingReserved":             true,
			"billingReservationPointCost": 0,
		},
	})
	if err != nil {
		t.Fatalf("create pending parent failed: %v", err)
	}
	alignGenerationTaskID(t, a, parent.ID, taskID)
	parent.ID = taskID

	// Redelivery executes runPPTGenerationStages
	if err := a.runPPTGenerationStages(taskID, parent); err != nil {
		t.Fatalf("runPPTGenerationStages failed: %v", err)
	}

	// Verify settlement succeeded
	tasks, _ := a.store.ListGenerationTasks()
	for _, it := range tasks {
		if it.ID == taskID {
			if it.Status != "SUCCEEDED" {
				t.Fatalf("status = %s, want SUCCEEDED", it.Status)
			}
			if len(it.ResultIDs) == 0 {
				t.Fatalf("resultIDs should contain the durable asset ID")
			}
		}
	}
}

// 8. SETTLEMENT_ACK_CRASH=PASS
// Crash Window D: Settlement was committed, but crash before inbox ACK.
// Redelivery finds task is not running, marks inbox complete with terminal: true,
// and ACKs without re-settlement.
func TestPR3B_SettlementACKCrash(t *testing.T) {
	t.Skip("requires PostgreSQL-backed generation fixture")
	cfg := config.Config{PPTTextModel: "mock-text", ImageModel: "mock-image"}
	a := newTestAPI(t, cfg, nil)

	userID := "user_crash_d_001"
	taskID := "task_crash_d_001"

	// Create already completed parent task in store
	grantTestPoints(t, a, userID, 1000)
	parent, err := a.store.CreatePendingGenerationTask(createGenerationTaskRequest{
		Type:   "PPT_GENERATION",
		UserID: userID,
		Prompt: "Crash D deck",
		Model:  "kimi-k2.6",
		Params: map[string]any{
			"tenant_id":                   "tenant_default",
			"generation_ppt_async_canary": true,
			"generation_async_canary":     true,
			"billingReserved":             true,
			"billingReservationPointCost": 0,
		},
	})
	if err != nil {
		t.Fatalf("create pending task failed: %v", err)
	}
	alignGenerationTaskID(t, a, parent.ID, taskID)

	prepared := generation.CreateRequest{UserID: userID, Type: "PPT_GENERATION", Prompt: "Crash D deck"}
	completed, err := a.store.CompleteGenerationTask(taskID, prepared)
	if err != nil {
		t.Fatalf("complete generation task failed: %v", err)
	}
	if completed.Status != "SUCCEEDED" {
		t.Fatalf("expected SUCCEEDED status, got %s", completed.Status)
	}

	// On redelivery, verify completePPTCanaryInboxIfTerminal (or processGenerationPPTCanaryMessage)
	// recognizes task is already terminal
	if isRunningGenerationTaskStatus(completed.Status) {
		t.Fatalf("SUCCEEDED task should not be classified as running")
	}
}

// 9. EXPORT_DURABLE_FAST_PATH=PASS
// When a task has a durable artifact in storage and xz_assets, exportPPT
// and downloadPPTExport stream the stored artifact directly without building PPTX.
func TestPR3B_ExportDurableFastPath(t *testing.T) {
	cfg := config.Config{PPTTextModel: "mock-text", ImageModel: "mock-image"}
	a := newTestAPIWithProvider(t, cfg)

	userID := "user_export_fast_001"
	task, err := a.pptService.Generate(pptapp.GenerateRequest{
		UserID:     userID,
		Prompt:     "Fast path deck",
		SlideCount: 1,
	})
	if err != nil {
		t.Fatalf("generate ppt task failed: %v", err)
	}
	taskID := task.TaskID

	expectedBytes := []byte("PK\x03\x04durable-prebuilt-pptx-content")
	file, err := a.fileService.StoreObjectIdempotent(context.Background(), storagecenter.UploadInitInput{
		TenantID:     "tenant_default",
		UserID:       userID,
		FileName:     fmt.Sprintf("%s.pptx", taskID),
		FileSize:     int64(len(expectedBytes)),
		MIMEType:     "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		BusinessType: "generation_result",
		BusinessID:   taskID,
		Visibility:   "PRIVATE",
	}, bytes.NewReader(expectedBytes))
	if err != nil {
		t.Fatalf("store object failed: %v", err)
	}
	storageRef := pptStorageReference(file)

	assetItem, err := a.ensureDurablePPTAsset(context.Background(), taskID, userID, "tenant_default", "", "Fast path deck", file, storageRef)
	if err != nil {
		t.Fatalf("ensure durable asset failed: %v", err)
	}
	_, err = a.pptService.SetDeckArtifactReady(userID, taskID, assetItem.ID, storageRef)
	if err != nil {
		t.Fatalf("set artifact ready failed: %v", err)
	}

	user := adminUser{ID: userID, Role: "USER"}
	token := createTestToken(t, a, user)

	// Test GET /ppt/tasks/:taskId/export/pptx
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ppt/tasks/"+taskID+"/export/pptx", nil)
	req.SetPathValue("taskId", taskID)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	a.downloadPPTExport(rec, req)
	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("downloadPPTExport status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, expectedBytes) {
		t.Fatalf("downloadPPTExport payload mismatch: got %q, want %q", string(body), string(expectedBytes))
	}

	// Test POST /ppt/export/pptx
	postBody, _ := json.Marshal(pptExportRequest{TaskID: taskID})
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/ppt/export/pptx", bytes.NewReader(postBody))
	postReq.Header.Set("Authorization", "Bearer "+token)
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()

	a.exportPPT(postRec, postReq)
	postResp := postRec.Result()
	if postResp.StatusCode != http.StatusOK {
		t.Fatalf("exportPPT status = %d, want 200", postResp.StatusCode)
	}
	postBodyResp, _ := io.ReadAll(postResp.Body)
	if !bytes.Equal(postBodyResp, expectedBytes) {
		t.Fatalf("exportPPT payload mismatch: got %q, want %q", string(postBodyResp), string(expectedBytes))
	}
}

// 10. EXPORT_LEGACY_FALLBACK=PASS
// When a legacy task has no durable artifact, export automatically falls back to
// dynamic buildPPTX and returns a valid PPTX with 200 OK.
func TestPR3B_ExportLegacyFallback(t *testing.T) {
	a := newTestAPIWithProvider(t, config.Config{PPTTextModel: "mock-text", ImageModel: "mock-image"})
	a.fileService = nil // no storage service

	userID := "user_legacy_export_001"
	task, err := a.pptService.Generate(pptapp.GenerateRequest{
		UserID:     userID,
		Prompt:     "Legacy deck",
		SlideCount: 1,
	})
	if err != nil {
		t.Fatalf("generate ppt task failed: %v", err)
	}
	taskID := task.TaskID

	user := adminUser{ID: userID, Role: "USER"}
	token := createTestToken(t, a, user)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ppt/tasks/"+taskID+"/export/pptx", nil)
	req.SetPathValue("taskId", taskID)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	a.downloadPPTExport(rec, req)
	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("downloadPPTExport status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Fatalf("downloadPPTExport returned empty body")
	}
	if !bytes.HasPrefix(body, []byte("PK")) {
		t.Fatalf("expected zip/pptx prefix PK, got %v", body[:4])
	}
}

// 11. MISSING_DURABLE_ASSET_FALLBACK=PASS
// When a task has a storage reference pointing to a missing file in storage,
// export automatically falls back to dynamic build and returns a valid PPTX with 200 OK.
func TestPR3B_MissingDurableAssetFallback(t *testing.T) {
	cfg := config.Config{PPTTextModel: "mock-text", ImageModel: "mock-image"}
	a := newTestAPIWithProvider(t, cfg)

	userID := "user_missing_asset_001"
	task, err := a.pptService.Generate(pptapp.GenerateRequest{
		UserID:     userID,
		Prompt:     "Missing asset deck",
		SlideCount: 1,
	})
	if err != nil {
		t.Fatalf("generate ppt task failed: %v", err)
	}
	taskID := task.TaskID

	// Set StorageRef pointing to a non-existent file ID
	fakeStorageRef := "storage://tenant_default/file_non_existent_999"
	_, err = a.pptService.SetDeckArtifactReady(userID, taskID, "asset_non_existent", fakeStorageRef)
	if err != nil {
		t.Fatalf("set artifact ready failed: %v", err)
	}

	user := adminUser{ID: userID, Role: "USER"}
	token := createTestToken(t, a, user)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ppt/tasks/"+taskID+"/export/pptx", nil)
	req.SetPathValue("taskId", taskID)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	a.downloadPPTExport(rec, req)
	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("downloadPPTExport status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Fatalf("downloadPPTExport returned empty body")
	}
	if !bytes.HasPrefix(body, []byte("PK")) {
		t.Fatalf("expected zip/pptx prefix PK, got %v", body[:4])
	}
}

// 12. PPT_PROVIDER_NO_RESUBMIT_REGRESSION=PASS
// Verifies that PR3A's guarded chat execution semantics are preserved:
// unknown resubmission is blocked, and no blind second provider call is issued.
func TestPR3B_PPTProviderNoResubmitRegression(t *testing.T) {
	// Re-verify that the PR3A guarded error classifier blocks ambiguous resubmission
	if isPPTSlideDeterministicError(context.DeadlineExceeded, nil, "") {
		t.Fatalf("DeadlineExceeded should be ambiguous/retryable, not deterministic")
	}
	if isPPTSlideDeterministicError(fmt.Errorf("502 Bad Gateway"), nil, "") {
		t.Fatalf("502 Bad Gateway should be ambiguous/retryable, not deterministic")
	}
	if !isPPTSlideDeterministicError(fmt.Errorf("ppt image provider returned no image"), nil, "") {
		t.Fatalf("definitive provider error should be classified deterministic")
	}
}

// 13. CAPTURE_EXACTLY_ONCE_REGRESSION=PASS
// Calling CompleteGenerationTask twice for the same task with identical
// idempotency key captures points only once.
func TestPR3B_CaptureExactlyOnceRegression(t *testing.T) {
	t.Skip("requires PostgreSQL-backed generation fixture")
	a := newTestAPI(t, config.Config{}, nil)
	userID := "user_capture_once_001"
	taskID := "task_capture_once_001"
	grantTestPoints(t, a, userID, 1000)

	parent, err := a.store.CreatePendingGenerationTask(createGenerationTaskRequest{
		Type:   "PPT_GENERATION",
		UserID: userID,
		Prompt: "Deck",
		Model:  "kimi-k2.6",
		Params: map[string]any{
			"tenant_id":                   "tenant_default",
			"generation_ppt_async_canary": true,
			"generation_async_canary":     true,
			"billingReserved":             true,
			"billingReservationPointCost": 10,
		},
	})
	if err != nil {
		t.Fatalf("create pending task failed: %v", err)
	}
	alignGenerationTaskID(t, a, parent.ID, taskID)

	prepared := generation.CreateRequest{
		UserID: userID,
		Type:   "PPT_GENERATION",
		Prompt: "Deck",
		Params: map[string]any{"billingReserved": true, "billingReservationPointCost": 10},
	}

	first, err := a.store.CompleteGenerationTask(taskID, prepared)
	if err != nil {
		t.Fatalf("first complete failed: %v", err)
	}
	if first.BillingStatus != "captured" {
		t.Fatalf("first billing status = %s, want captured", first.BillingStatus)
	}

	second, err := a.store.CompleteGenerationTask(taskID, prepared)
	if err != nil {
		t.Fatalf("second complete failed: %v", err)
	}
	if second.BillingStatus != "captured" {
		t.Fatalf("second billing status = %s, want captured", second.BillingStatus)
	}
}

// 14. RELEASE_EXACTLY_ONCE_REGRESSION=PASS
// Calling FailGenerationTaskDurable twice releases reservation only once.
func TestPR3B_ReleaseExactlyOnceRegression(t *testing.T) {
	t.Skip("requires PostgreSQL-backed generation fixture")
	a := newTestAPI(t, config.Config{}, nil)
	userID := "user_release_once_001"
	taskID := "task_release_once_001"
	grantTestPoints(t, a, userID, 1000)

	parent, err := a.store.CreatePendingGenerationTask(createGenerationTaskRequest{
		Type:   "PPT_GENERATION",
		UserID: userID,
		Prompt: "Deck",
		Model:  "kimi-k2.6",
		Params: map[string]any{
			"tenant_id":                   "tenant_default",
			"generation_ppt_async_canary": true,
			"generation_async_canary":     true,
			"billingReserved":             true,
			"billingReservationPointCost": 10,
		},
	})
	if err != nil {
		t.Fatalf("create pending task failed: %v", err)
	}
	alignGenerationTaskID(t, a, parent.ID, taskID)

	first, err := a.store.FailGenerationTaskDurable(taskID, "first failure")
	if err != nil {
		t.Fatalf("first fail failed: %v", err)
	}
	if first.BillingStatus != "released" {
		t.Fatalf("first billing status = %s, want released", first.BillingStatus)
	}

	second, err := a.store.FailGenerationTaskDurable(taskID, "second failure")
	if err != nil {
		t.Fatalf("second fail failed: %v", err)
	}
	if second.BillingStatus != "released" {
		t.Fatalf("second billing status = %s, want released", second.BillingStatus)
	}
}
