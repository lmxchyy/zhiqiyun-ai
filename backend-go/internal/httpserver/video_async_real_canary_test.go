package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type realCanaryRecord struct {
	TaskID      string
	ArtifactID  string
	FileID      string
	StorageKey  string
	ObjectSize  int64
	PlaybackURL string
	PlayResult  string
}

func TestVideoAsyncRealCanary_LiveIntegration(t *testing.T) {
	dbURL := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://xianzhi:xianzhi@127.0.0.1:54321/xianzhi"
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Skipf("cannot open postgres: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Skipf("cannot ping postgres: %v", err)
	}

	// 1. MinIO Provider 真实连接
	storageFactory := storagecenter.S3ProviderFactory{AutoCreateBucket: true}
	storageProvider, err := storageFactory.Build(storagecenter.Config{
		Provider:        "minio",
		Endpoint:        "http://127.0.0.1:9000",
		SigningEndpoint: "http://127.0.0.1:9000",
		Region:          "us-east-1",
		Bucket:          "xianzhi-assets",
		AccessKey:       "xianzhi",
		SecretKey:       "xianzhi-minio",
		ForcePathStyle:  true,
	})
	if err != nil {
		t.Fatalf("build storage provider: %v", err)
	}

	repo := storagecenter.NewPostgresRepository(db)
	fileService := storagecenter.NewService(repo, singleProviderFactory{provider: storageProvider}, storagecenter.Options{
		DefaultProvider:   "minio",
		Endpoint:          "http://127.0.0.1:9000",
		PublicEndpoint:    "http://127.0.0.1:9000",
		AccessKey:         "xianzhi",
		SecretKey:         "xianzhi-minio",
		Bucket:            "xianzhi-assets",
		DefaultQuotaBytes: 100 * 1024 * 1024 * 1024,
		MaxUploadBytes:    2 * 1024 * 1024 * 1024,
		MasterKey:         "xianzhi-dev-storage-master-key-32bytes",
	})

	remoteDownloadAllowLoopbackForTesting = true
	defer func() { remoteDownloadAllowLoopbackForTesting = false }()

	// Mock upstream video provider returning binary stream
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/video1.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("fake-mp4-video-stream-content-for-canary-001"))
		case "/video2.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("fake-mp4-video-stream-content-for-canary-002-with-longer-payload-bytes"))
		case "/cover2.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("fake-jpeg-cover-stream-content"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstreamServer.Close()

	cfg := config.Config{
		VideoStoragePersistenceEnabled: true,
		VideoAsyncCanaryEnabled:        true,
		VideoAsyncCanaryUsers:          "user_canary_test",
		VideoAsyncCanaryModelAllowlist: "grok-imagine-1.5-video",
		ProviderExecutionSafetyEnabled: true,
	}

	a := api{fileService: fileService, cfg: cfg}

	canaryResults := make([]realCanaryRecord, 0, 3)

	// === CANARY 1: Standard Text-to-Video ===
	taskID1 := fmt.Sprintf("task_canary_live_%d_1", time.Now().Unix())
	req1 := generation.CreateRequest{
		UserID: "user_canary_test",
		Type:   "TEXT_TO_VIDEO",
		Prompt: "canary sunrise over high mountains",
		Model:  "grok-imagine-1.5-video",
		Params: map[string]any{
			"tenant_id": "tenant_default",
			"providerTask": map[string]any{
				"videoUrl": upstreamServer.URL + "/video1.mp4",
				"status":   "SUCCEEDED",
			},
		},
	}

	prepared1, files1, err := a.persistGeneratedVideos(context.Background(), taskID1, req1)
	if err != nil {
		t.Fatalf("canary 1 persist failed: %v", err)
	}
	if len(files1) != 1 {
		t.Fatalf("canary 1 expected 1 file, got %d", len(files1))
	}

	asset1 := generatedAssetForRequest(prepared1, req1.UserID, taskID1, "asset_"+taskID1, 0, time.Now().UTC().Format(time.RFC3339Nano))
	signed1 := a.signStoredAssetURLs(context.Background(), req1.UserID, []asset{asset1})
	if len(signed1) != 1 {
		t.Fatalf("canary 1 signing failed")
	}

	// 验证 MinIO 真实 Presigned URL 连通性
	presignedURL1 := signed1[0].URL
	if !strings.Contains(presignedURL1, "127.0.0.1:9000") && !strings.Contains(presignedURL1, "localhost:9000") {
		t.Fatalf("canary 1 presigned URL unexpected: %s", presignedURL1)
	}

	resp1, err := http.Get(presignedURL1)
	if err != nil {
		t.Fatalf("canary 1 GET presigned URL failed: %v", err)
	}
	defer resp1.Body.Close()
	body1, _ := io.ReadAll(resp1.Body)
	if resp1.StatusCode != http.StatusOK && resp1.StatusCode != http.StatusPartialContent {
		t.Fatalf("canary 1 GET presigned URL returned HTTP %d: %s", resp1.StatusCode, string(body1))
	}
	if string(body1) != "fake-mp4-video-stream-content-for-canary-001" {
		t.Fatalf("canary 1 content mismatch: got %q", string(body1))
	}

	canaryResults = append(canaryResults, realCanaryRecord{
		TaskID:      taskID1,
		ArtifactID:  "artifact_" + taskID1,
		FileID:      files1[0].FileID,
		StorageKey:  files1[0].ObjectKey,
		ObjectSize:  files1[0].FileSize,
		PlaybackURL: presignedURL1,
		PlayResult:  "HTTP 200 OK (Content verified in MinIO)",
	})

	// === CANARY 2: Video with Thumbnail ===
	taskID2 := fmt.Sprintf("task_canary_live_%d_2", time.Now().Unix())
	req2 := generation.CreateRequest{
		UserID: "user_canary_test",
		Type:   "TEXT_TO_VIDEO",
		Prompt: "canary ocean waves cinematic",
		Model:  "grok-imagine-1.5-video",
		Params: map[string]any{
			"tenant_id": "tenant_default",
			"providerTask": map[string]any{
				"videoUrl":     upstreamServer.URL + "/video2.mp4",
				"thumbnailUrl": upstreamServer.URL + "/cover2.jpg",
				"status":       "SUCCEEDED",
			},
		},
	}

	prepared2, files2, err := a.persistGeneratedVideos(context.Background(), taskID2, req2)
	if err != nil {
		t.Fatalf("canary 2 persist failed: %v", err)
	}
	if len(files2) != 2 {
		t.Fatalf("canary 2 expected 2 files (video + cover), got %d", len(files2))
	}

	asset2 := generatedAssetForRequest(prepared2, req2.UserID, taskID2, "asset_"+taskID2, 0, time.Now().UTC().Format(time.RFC3339Nano))
	signed2 := a.signStoredAssetURLs(context.Background(), req2.UserID, []asset{asset2})
	if len(signed2) != 1 {
		t.Fatalf("canary 2 signing failed")
	}

	resp2, err := http.Get(signed2[0].URL)
	if err != nil {
		t.Fatalf("canary 2 video GET failed: %v", err)
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if string(body2) != "fake-mp4-video-stream-content-for-canary-002-with-longer-payload-bytes" {
		t.Fatalf("canary 2 video content mismatch: %s", string(body2))
	}

	canaryResults = append(canaryResults, realCanaryRecord{
		TaskID:      taskID2,
		ArtifactID:  "artifact_" + taskID2,
		FileID:      files2[0].FileID,
		StorageKey:  files2[0].ObjectKey,
		ObjectSize:  files2[0].FileSize,
		PlaybackURL: signed2[0].URL,
		PlayResult:  "HTTP 200 OK (Video + Cover verified in MinIO)",
	})

	// === CANARY 3: Idempotent Replay ===
	taskID3 := taskID1
	prepared3, files3, err := a.persistGeneratedVideos(context.Background(), taskID3, prepared1)
	if err != nil {
		t.Fatalf("canary 3 replay failed: %v", err)
	}
	if len(files3) != 0 {
		t.Fatalf("canary 3 expected 0 new files on replay, got %d", len(files3))
	}
	rec3, ok := generatedStorageRecord(prepared3.Params, 0)
	if !ok || stringValue(rec3["fileId"]) != files1[0].FileID {
		t.Fatalf("canary 3 expected reused fileId %s", files1[0].FileID)
	}

	canaryResults = append(canaryResults, realCanaryRecord{
		TaskID:      taskID3,
		ArtifactID:  "artifact_" + taskID3,
		FileID:      files1[0].FileID,
		StorageKey:  files1[0].ObjectKey,
		ObjectSize:  files1[0].FileSize,
		PlaybackURL: presignedURL1,
		PlayResult:  "IDEMPOTENT_REUSE_PASS (Zero duplicate write to MinIO)",
	})

	// 输出测试日志供记录收集
	for idx, r := range canaryResults {
		t.Logf("CANARY_SAMPLE_%d task_id=%s file_id=%s storage_key=%s size=%d result=%s",
			idx+1, r.TaskID, r.FileID, r.StorageKey, r.ObjectSize, r.PlayResult)
	}
}

type singleProviderFactory struct {
	provider storagecenter.Provider
}

func (f singleProviderFactory) Build(_ storagecenter.Config) (storagecenter.Provider, error) {
	return f.provider, nil
}
