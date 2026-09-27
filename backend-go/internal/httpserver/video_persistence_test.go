package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

func TestPersistGeneratedVideos_Success(t *testing.T) {
	remoteDownloadAllowLoopbackForTesting = true
	defer func() { remoteDownloadAllowLoopbackForTesting = false }()

	videoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("fake-mp4-video-binary-content"))
	}))
	defer videoServer.Close()

	repo := storagecenter.NewMemoryRepository()
	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
	service := storagecenter.NewService(repo, generatedStorageTestFactory{provider: provider}, storagecenter.Options{
		DefaultProvider:   "s3",
		Endpoint:          "https://storage.example",
		AccessKey:         "access",
		SecretKey:         "secret",
		Bucket:            "xianzhi-assets",
		DefaultQuotaBytes: 10 * 1024 * 1024,
		MaxUploadBytes:    10 * 1024 * 1024,
		MasterKey:         "0123456789abcdef0123456789abcdef",
	})

	a := api{fileService: service, cfg: config.Config{VideoStoragePersistenceEnabled: true}}

	req := generation.CreateRequest{
		UserID: "user_test",
		Type:   "TEXT_TO_VIDEO",
		Prompt: "a cinematic landscape",
		Model:  "grok-imagine-1.5-video",
		Params: map[string]any{
			"tenant_id": "tenant_default",
			"providerTask": map[string]any{
				"videoUrl": videoServer.URL + "/generated-video.mp4",
				"status":   "SUCCEEDED",
			},
		},
	}

	taskID := "task_video_001"
	prepared, files, err := a.persistGeneratedVideos(context.Background(), taskID, req)
	if err != nil {
		t.Fatalf("persistGeneratedVideos failed: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 stored file, got %d", len(files))
	}
	if files[0].BusinessID != taskID {
		t.Fatalf("expected businessID %s, got %s", taskID, files[0].BusinessID)
	}
	if _, ok := provider.objects[files[0].ObjectKey]; !ok {
		t.Fatalf("object %s not written to storage provider", files[0].ObjectKey)
	}

	item := generatedAssetForRequest(prepared, req.UserID, taskID, "asset_v001", 0, time.Now().UTC().Format(time.RFC3339Nano))
	if stringValue(item.Metadata["fileId"]) != files[0].FileID {
		t.Fatalf("asset metadata fileId mismatch: got %s, want %s", stringValue(item.Metadata["fileId"]), files[0].FileID)
	}
	if !boolValue(item.Metadata["storageManaged"]) {
		t.Fatalf("asset metadata storageManaged must be true")
	}
	if item.URL != "storage://"+files[0].FileID {
		t.Fatalf("asset URL must be storage://%s, got %s", files[0].FileID, item.URL)
	}

	signed := a.signStoredAssetURLs(context.Background(), req.UserID, []asset{item})
	if len(signed) != 1 {
		t.Fatalf("expected 1 signed asset, got %d", len(signed))
	}
	if !strings.Contains(signed[0].URL, "/download/"+files[0].ObjectKey) {
		t.Fatalf("signed asset URL must point to object storage access, got %s", signed[0].URL)
	}
	if signed[0].Availability != AssetAvailabilityAvailable {
		t.Fatalf("managed asset availability must be AVAILABLE, got %s", signed[0].Availability)
	}
}

func TestPersistGeneratedVideos_Idempotent(t *testing.T) {
	remoteDownloadAllowLoopbackForTesting = true
	defer func() { remoteDownloadAllowLoopbackForTesting = false }()

	callCount := 0
	videoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("fake-mp4-video-content-idempotent"))
	}))
	defer videoServer.Close()

	repo := storagecenter.NewMemoryRepository()
	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
	service := storagecenter.NewService(repo, generatedStorageTestFactory{provider: provider}, storagecenter.Options{
		DefaultProvider:   "s3",
		Endpoint:          "https://storage.example",
		AccessKey:         "access",
		SecretKey:         "secret",
		Bucket:            "xianzhi-assets",
		DefaultQuotaBytes: 10 * 1024 * 1024,
		MaxUploadBytes:    10 * 1024 * 1024,
		MasterKey:         "0123456789abcdef0123456789abcdef",
	})

	a := api{fileService: service, cfg: config.Config{VideoStoragePersistenceEnabled: true}}
	taskID := "task_idempotent_001"
	req := generation.CreateRequest{
		UserID: "user_test",
		Type:   "TEXT_TO_VIDEO",
		Params: map[string]any{
			"tenant_id": "tenant_default",
			"providerTask": map[string]any{
				"videoUrl": videoServer.URL + "/video.mp4",
			},
		},
	}

	// First persist
	prepared1, files1, err1 := a.persistGeneratedVideos(context.Background(), taskID, req)
	if err1 != nil || len(files1) != 1 {
		t.Fatalf("first persist failed: err=%v len=%d", err1, len(files1))
	}
	firstFileID := files1[0].FileID

	// Second persist (replay)
	prepared2, files2, err2 := a.persistGeneratedVideos(context.Background(), taskID, prepared1)
	if err2 != nil {
		t.Fatalf("second persist failed: %v", err2)
	}
	if len(files2) != 0 {
		t.Fatalf("reused files should not be appended to new cleanup slice, got %d", len(files2))
	}

	record, ok := generatedStorageRecord(prepared2.Params, 0)
	if !ok || stringValue(record["fileId"]) != firstFileID {
		t.Fatalf("expected record to retain first fileID %s, got %+v", firstFileID, record)
	}
	if callCount != 1 {
		t.Fatalf("upstream video was downloaded %d times, want exactly 1", callCount)
	}
}

func TestPersistGeneratedVideos_UpstreamFailure(t *testing.T) {
	remoteDownloadAllowLoopbackForTesting = true
	defer func() { remoteDownloadAllowLoopbackForTesting = false }()

	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer failServer.Close()

	repo := storagecenter.NewMemoryRepository()
	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
	service := storagecenter.NewService(repo, generatedStorageTestFactory{provider: provider}, storagecenter.Options{
		DefaultProvider: "s3", Endpoint: "https://storage.example", AccessKey: "access", SecretKey: "secret",
		Bucket: "xianzhi-assets", DefaultQuotaBytes: 1024, MaxUploadBytes: 1024, MasterKey: "0123456789abcdef0123456789abcdef",
	})
	a := api{fileService: service, cfg: config.Config{VideoStoragePersistenceEnabled: true}}

	req := generation.CreateRequest{
		UserID: "user_test",
		Type:   "TEXT_TO_VIDEO",
		Params: map[string]any{
			"tenant_id": "tenant_default",
			"providerTask": map[string]any{
				"videoUrl": failServer.URL + "/notfound.mp4",
			},
		},
	}

	_, _, err := a.persistGeneratedVideos(context.Background(), "task_fail_001", req)
	if err == nil {
		t.Fatalf("expected error when upstream returns 404, got nil")
	}
	if !strings.Contains(err.Error(), "upstream returned 404") {
		t.Fatalf("expected 'upstream returned 404', got %v", err)
	}
}

func TestRunVideoGenerationTask_StorageFailurePreventsCapture(t *testing.T) {
	store := newBillingAcceptanceStore(t)
	pending, err := store.CreatePendingGenerationTask(videoAcceptanceRequest("video-storage-fail-closed"))
	if err != nil {
		t.Fatalf("create pending task: %v", err)
	}
	service := generation.NewServiceWithOptions(generation.ServiceOptions{
		VideoProvider: &mockVideoProvider{},
	})
	req := generation.CreateRequest{
		UserID: pending.UserID,
		Type:   pending.Type,
		Prompt: pending.Prompt,
		Model:  pending.Model,
		Params: cloneAnyMap(pending.Params),
	}

	err = (api{
		store: store,
		cfg:   config.Config{VideoStoragePersistenceEnabled: true},
	}).runVideoGenerationTask(pending.ID, service, req)
	if err == nil || !strings.Contains(err.Error(), "private video storage is unavailable") {
		t.Fatalf("expected durable-storage failure, got %v", err)
	}

	tasks, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatalf("list generation tasks: %v", err)
	}
	task := generationBillingTaskByID(t, tasks, pending.ID)
	if task.Status != taskStatusFailed || task.BillingStatus != billingStatusReleased {
		t.Fatalf("storage failure must fail and release task before capture, got status=%s billing=%s", task.Status, task.BillingStatus)
	}
	if task.CapturedPoints != 0 || task.ReleasedPoints != pending.ReservedPoints {
		t.Fatalf("storage failure must not capture points; captured=%v released=%v reserved=%v", task.CapturedPoints, task.ReleasedPoints, pending.ReservedPoints)
	}
}

func TestPersistGeneratedVideos_RequiresDurableStorage(t *testing.T) {
	req := generation.CreateRequest{
		UserID: "user_test",
		Type:   "TEXT_TO_VIDEO",
		Params: map[string]any{
			"tenant_id": "tenant_default",
			"providerTask": map[string]any{
				"videoUrl": "https://provider.example/video.mp4",
			},
		},
	}

	t.Run("missing service fails closed", func(t *testing.T) {
		a := api{cfg: config.Config{VideoStoragePersistenceEnabled: true}}
		_, _, err := a.persistGeneratedVideos(context.Background(), "task_storage_missing", req)
		if err == nil || !strings.Contains(err.Error(), "private video storage is unavailable") {
			t.Fatalf("expected unavailable-storage error, got %v", err)
		}
	})

	t.Run("unconfigured service fails closed", func(t *testing.T) {
		service := storagecenter.NewService(storagecenter.NewMemoryRepository(), generatedStorageTestFactory{}, storagecenter.Options{})
		a := api{fileService: service, cfg: config.Config{VideoStoragePersistenceEnabled: true}}
		_, _, err := a.persistGeneratedVideos(context.Background(), "task_storage_unconfigured", req)
		if err == nil || !strings.Contains(err.Error(), "private video storage is not configured") {
			t.Fatalf("expected unconfigured-storage error, got %v", err)
		}
	})
}

func TestPersistGeneratedVideos_FeatureFlagOff(t *testing.T) {
	// Feature Flag 为 OFF 时，runVideoGenerationTask does not call persistGeneratedVideos.
	cfg := config.Config{VideoStoragePersistenceEnabled: false}
	if cfg.VideoStoragePersistenceEnabled {
		t.Fatalf("expected VideoStoragePersistenceEnabled to be false by default")
	}
}
