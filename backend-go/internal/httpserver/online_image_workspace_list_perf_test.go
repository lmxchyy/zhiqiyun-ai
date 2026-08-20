package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type countingPresignProvider struct {
	*generatedStorageTestProvider
	delay time.Duration
	count atomic.Int64
}

func (p *countingPresignProvider) CreatePresignedDownloadURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	p.count.Add(1)
	if p.delay > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(p.delay):
		}
	}
	return p.generatedStorageTestProvider.CreatePresignedDownloadURL(ctx, key, ttl)
}

func newWorkspaceListPerfFixture(t *testing.T, assetCount int, delay time.Duration) (api, string, *countingPresignProvider, []asset) {
	t.Helper()
	provider := &countingPresignProvider{
		generatedStorageTestProvider: &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}},
		delay:                        delay,
	}
	service := storagecenter.NewService(storagecenter.NewMemoryRepository(), generatedStorageTestFactory{provider: provider}, storagecenter.Options{
		DefaultProvider:   "s3",
		Endpoint:          "https://storage.example",
		AccessKey:         "access",
		SecretKey:         "secret",
		Bucket:            "private-files",
		DefaultQuotaBytes: 1 << 20,
		MaxUploadBytes:    1 << 20,
		MasterKey:         "0123456789abcdef0123456789abcdef",
	})
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	created, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "History User", Email: "history-user@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	thumb := "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQAAAQABAAD/2wAAA=="
	assets := make([]asset, 0, assetCount)
	tasks := make([]generationTask, 0, assetCount)
	ctx := context.Background()
	for i := 0; i < assetCount; i++ {
		payload := []byte("png" + strconv.Itoa(i))
		file, err := service.StoreObject(ctx, storagecenter.UploadInitInput{
			TenantID:     "tenant_default",
			UserID:       created.ID,
			FileName:     "history-" + strconv.Itoa(i) + ".png",
			FileSize:     int64(len(payload)),
			MIMEType:     "image/png",
			BusinessType: "generation_result",
			BusinessID:   "task_hist_" + strconv.Itoa(i),
			Visibility:   "PRIVATE",
		}, bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("store object %d: %v", i, err)
		}
		taskID := "task_hist_" + strconv.Itoa(i)
		assetID := "asset_hist_" + strconv.Itoa(i)
		tasks = append(tasks, generationTask{ID: taskID, UserID: created.ID, Status: "SUCCEEDED", Type: "TEXT_TO_IMAGE", ResultIDs: []string{assetID}})
		assets = append(assets, asset{
			ID:           assetID,
			UserID:       created.ID,
			TaskID:       taskID,
			MediaType:    "IMAGE",
			URL:          "storage://" + file.FileID,
			ThumbnailURL: thumb,
			Metadata:     map[string]any{"fileId": file.FileID, "storageManaged": true},
		})
	}
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		data.GenerationTasks = append(data.GenerationTasks, tasks...)
		data.Assets = append(data.Assets, assets...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sessions := newLocalAuthSessions()
	token := "history-token"
	if err := sessions.Put(ctx, token, created.ID, authSessionTTL); err != nil {
		t.Fatal(err)
	}
	return api{store: store, sessions: sessions, fileService: service}, token, provider, assets
}

func TestPrepareWorkspaceListAssetsOmitsStorageOriginalsAndKeepsInlineThumbs(t *testing.T) {
	thumb := "data:image/jpeg;base64,abc"
	items := prepareWorkspaceListAssets([]asset{
		{ID: "a1", URL: "storage://file_1", ThumbnailURL: thumb, Metadata: map[string]any{"fileId": "file_1"}},
		{ID: "a2", URL: "https://cdn.example/public.png", ThumbnailURL: thumb},
		{ID: "a3", URL: "storage://file_3", ThumbnailURL: "storage://cover_3", Metadata: map[string]any{"fileId": "file_3", "coverFileId": "cover_3"}},
	})
	if items[0].URL != "" || items[0].ThumbnailURL != thumb {
		t.Fatalf("storage original should be omitted and inline thumb kept: %+v", items[0])
	}
	if items[1].URL != "https://cdn.example/public.png" {
		t.Fatalf("public original without fileId should stay: %+v", items[1])
	}
	if items[2].URL != "" || items[2].ThumbnailURL != "" {
		t.Fatalf("storage thumbnails must not be returned unsigned: %+v", items[2])
	}
}

func TestUserOnlineImageFirstPaintSkipsSerialAssetSigning(t *testing.T) {
	const assetCount = 20
	handler, token, provider, _ := newWorkspaceListPerfFixture(t, assetCount, 50*time.Millisecond)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/online-image?taskLimit=40&assetLimit=40", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	started := time.Now()
	handler.userOnlineImage(response, req)
	elapsed := time.Since(started)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if got := provider.count.Load(); got != 0 {
		t.Fatalf("first paint signed %d originals; want 0 serial AccessURL calls", got)
	}
	serialBudget := 50 * time.Millisecond * time.Duration(assetCount)
	if elapsed >= serialBudget/2 {
		t.Fatalf("first paint still looks serial: elapsed=%s serialBudget=%s", elapsed, serialBudget)
	}
	var payload struct {
		RecentTasks []generationTask `json:"recentTasks"`
		Assets      []asset          `json:"assets"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Assets) != assetCount || len(payload.RecentTasks) != assetCount {
		t.Fatalf("first-paint counts tasks=%d assets=%d want %d", len(payload.RecentTasks), len(payload.Assets), assetCount)
	}
	for _, item := range payload.Assets {
		if item.URL != "" {
			t.Fatalf("workspace list leaked original URL for %s: %s", item.ID, item.URL)
		}
		if !strings.HasPrefix(item.ThumbnailURL, "data:image/") {
			t.Fatalf("workspace list dropped inline thumbnail for %s: %s", item.ID, item.ThumbnailURL)
		}
		if strings.Contains(item.ThumbnailURL, "/download/") {
			t.Fatalf("workspace list signed thumbnail for %s: %s", item.ID, item.ThumbnailURL)
		}
	}
	for _, task := range payload.RecentTasks {
		if task.ThumbnailURL == "" || strings.Contains(task.ThumbnailURL, "/download/") {
			t.Fatalf("task thumbnail should be the inline cover: %+v", task)
		}
		if strings.Contains(task.ImageURL, "/download/") || strings.HasPrefix(task.ImageURL, "storage://") {
			t.Fatalf("task original should not be signed or leak storage refs: %+v", task)
		}
	}
}

func TestAssetsForUserStillSignsOriginalsForDownloadCallers(t *testing.T) {
	handler, _, provider, stored := newWorkspaceListPerfFixture(t, 4, 0)
	userID := stored[0].UserID
	signed, err := handler.assetsForUser(httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil), userID, 4)
	if err != nil {
		t.Fatal(err)
	}
	if provider.count.Load() < 4 {
		t.Fatalf("download/detail path must still sign originals: count=%d", provider.count.Load())
	}
	for _, item := range signed {
		if !strings.Contains(item.URL, "/download/") {
			t.Fatalf("signed original missing for %s: %s", item.ID, item.URL)
		}
	}
}

func TestUserOnlineImageDefaultLimitStillCapsFirstPaintCount(t *testing.T) {
	handler, token, _, _ := newWorkspaceListPerfFixture(t, 55, 0)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/online-image", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.userOnlineImage(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		RecentTasks []generationTask `json:"recentTasks"`
		Assets      []asset          `json:"assets"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.RecentTasks) > 40 || len(payload.Assets) > 40 {
		t.Fatalf("online-image default payload too large: tasks=%d assets=%d", len(payload.RecentTasks), len(payload.Assets))
	}
}
