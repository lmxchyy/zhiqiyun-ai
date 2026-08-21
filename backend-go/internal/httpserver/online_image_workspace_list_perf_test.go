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

	"xianzhi-ai/backend-go/internal/config"
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
	thumb, _ := testJPEGDataURL(t, 8)
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
	return api{
		store:       store,
		sessions:    sessions,
		fileService: service,
		cfg:         config.Config{InspirationDraftHMACSecret: "workspace-list-thumbnail-test-secret"},
	}, token, provider, assets
}

func TestPrepareWorkspaceListAssetsOmitsStorageOriginalsAndInlineThumbs(t *testing.T) {
	thumb := "data:image/jpeg;base64,abc"
	hugeOriginal := "data:image/png;base64," + strings.Repeat("A", 256*1024)
	items := prepareWorkspaceListAssets([]asset{
		{ID: "a1", URL: "storage://file_1", ThumbnailURL: thumb, Metadata: map[string]any{"fileId": "file_1"}},
		{ID: "a2", URL: "https://cdn.example/public.png", ThumbnailURL: thumb},
		{ID: "a3", URL: "storage://file_3", ThumbnailURL: "storage://cover_3", Metadata: map[string]any{"fileId": "file_3", "coverFileId": "cover_3"}},
		{ID: "a4", URL: hugeOriginal, ThumbnailURL: thumb, Metadata: map[string]any{"fileId": "file_4", "sourceUrl": hugeOriginal, "storageObjectKey": "tenants/t1/file_4.png", "fileSize": 1024, "projectId": "p1"}},
	})
	if items[0].URL != "" || items[0].ThumbnailURL != "" {
		t.Fatalf("storage original and inline thumb should be omitted from list DTO: %+v", items[0])
	}
	if items[1].URL != "https://cdn.example/public.png" || items[1].ThumbnailURL != "" {
		t.Fatalf("public original without fileId should stay, inline thumb must be omitted: %+v", items[1])
	}
	if items[2].URL != "" || items[2].ThumbnailURL != "" {
		t.Fatalf("storage thumbnails must not be returned unsigned: %+v", items[2])
	}
	if items[3].URL != "" {
		t.Fatalf("inline original URL must be omitted from workspace list: %+v", items[3])
	}
	if items[3].Metadata["sourceUrl"] != nil {
		t.Fatalf("workspace list leaked sourceUrl: %+v", items[3].Metadata)
	}
	if items[3].Metadata["storageObjectKey"] != nil {
		t.Fatalf("workspace list leaked storageObjectKey: %+v", items[3].Metadata)
	}
	if items[3].Metadata["fileId"] != "file_4" || items[3].Metadata["projectId"] != "p1" {
		t.Fatalf("workspace list dropped card metadata: %+v", items[3].Metadata)
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
	if response.Body.Len() > 200*1024 {
		t.Fatalf("first paint payload still too large: %d bytes", response.Body.Len())
	}
	if bytes.Contains(response.Body.Bytes(), []byte(`"sourceUrl"`)) {
		t.Fatal("first paint leaked sourceUrl")
	}
	for _, item := range payload.Assets {
		if item.URL != "" {
			t.Fatalf("workspace list leaked original URL for %s: %s", item.ID, item.URL)
		}
		assertWorkspaceListTicketThumbnailURL(t, item)
		if strings.Contains(item.ThumbnailURL, "/download/") {
			t.Fatalf("workspace list signed thumbnail for %s: %s", item.ID, item.ThumbnailURL)
		}
	}
	assertWorkspaceListThumbnailDedup(t, response.Body.Bytes(), payload.RecentTasks, payload.Assets)
	for _, task := range payload.RecentTasks {
		if strings.Contains(task.ImageURL, "/download/") || strings.HasPrefix(task.ImageURL, "storage://") {
			t.Fatalf("task original should not be signed or leak storage refs: %+v", task)
		}
	}
}

func TestUserOnlineImageFirstPaintOmitsLegacyInlineOriginals(t *testing.T) {
	handler, token, _, stored := newWorkspaceListPerfFixture(t, 8, 0)
	hugeOriginal := "data:image/png;base64," + strings.Repeat("A", 256*1024)
	store := handler.store.(*jsonStore)
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		for index := range data.Assets {
			if data.Assets[index].UserID != stored[0].UserID {
				continue
			}
			if data.Assets[index].Metadata == nil {
				data.Assets[index].Metadata = map[string]any{}
			}
			data.Assets[index].URL = hugeOriginal
			data.Assets[index].Metadata["sourceUrl"] = hugeOriginal
			data.Assets[index].Metadata["storageObjectKey"] = "tenants/t1/" + data.Assets[index].ID + ".png"
			data.Assets[index].Metadata["fileSize"] = 1024
		}
		for index := range data.GenerationTasks {
			if data.GenerationTasks[index].UserID != stored[0].UserID {
				continue
			}
			if data.GenerationTasks[index].Params == nil {
				data.GenerationTasks[index].Params = map[string]any{}
			}
			data.GenerationTasks[index].Params["size"] = "1024x1024"
			data.GenerationTasks[index].Params["quality"] = "low"
			data.GenerationTasks[index].Params["referenceImages"] = []any{hugeOriginal}
			data.GenerationTasks[index].Params["inputImagesSnapshot"] = []any{map[string]any{"url": hugeOriginal}}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/online-image?taskLimit=40&assetLimit=40", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.userOnlineImage(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Body.Len() > 200*1024 {
		t.Fatalf("legacy inline originals still inflate first paint: %d bytes", response.Body.Len())
	}
	body := response.Body.Bytes()
	for _, leaked := range []string{`"sourceUrl"`, `"storageObjectKey"`, hugeOriginal[:40], strings.Repeat("A", 64)} {
		if bytes.Contains(body, []byte(leaked)) {
			t.Fatalf("first paint leaked %q", leaked)
		}
	}
	var payload struct {
		RecentTasks []generationTask `json:"recentTasks"`
		Assets      []asset          `json:"assets"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Assets) != 8 || len(payload.RecentTasks) != 8 {
		t.Fatalf("counts assets=%d tasks=%d", len(payload.Assets), len(payload.RecentTasks))
	}
	assertWorkspaceListThumbnailDedup(t, body, payload.RecentTasks, payload.Assets)
	for _, item := range payload.Assets {
		if item.URL != "" || item.Metadata["sourceUrl"] != nil {
			t.Fatalf("asset still carries original: %+v", item)
		}
		if item.Metadata["fileSize"] != float64(1024) && item.Metadata["fileSize"] != 1024 {
			t.Fatalf("card metadata dropped fileSize: %+v", item.Metadata)
		}
	}
	for _, task := range payload.RecentTasks {
		if task.Params["size"] != "1024x1024" || task.Params["quality"] != "low" {
			t.Fatalf("reuse params dropped: %+v", task.Params)
		}
		if _, ok := task.Params["referenceImages"]; ok {
			t.Fatalf("task params still carry referenceImages: %+v", task.Params)
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

func TestUserOnlineImageWorkspaceListOmitsDuplicateTaskThumbnails(t *testing.T) {
	const itemCount = 8
	thumb := "data:image/jpeg;base64," + strings.Repeat("B", 32*1024)
	handler, token, _, stored := newWorkspaceListPerfFixture(t, itemCount, 0)
	store := handler.store.(*jsonStore)
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		for index := range data.Assets {
			if data.Assets[index].UserID != stored[0].UserID {
				continue
			}
			data.Assets[index].ThumbnailURL = thumb
			if data.Assets[index].Metadata == nil {
				data.Assets[index].Metadata = map[string]any{}
			}
			data.Assets[index].Metadata["storageObjectKey"] = "tenants/t1/" + data.Assets[index].ID + ".png"
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/online-image?taskLimit=40&assetLimit=40", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.userOnlineImage(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.Bytes()
	var payload struct {
		RecentTasks []generationTask `json:"recentTasks"`
		Assets      []asset          `json:"assets"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	assertWorkspaceListThumbnailDedup(t, body, payload.RecentTasks, payload.Assets)
	if bytes.Contains(body, []byte(`"storageObjectKey"`)) {
		t.Fatal("workspace list leaked storageObjectKey")
	}
	if bytes.Contains(body, []byte("data:image/")) {
		t.Fatal("workspace list still inlined data thumbnails")
	}
	got := response.Body.Len()
	if got >= 100*1024 {
		t.Fatalf("ticket list payload still too large: got=%d", got)
	}
	t.Logf("online-image payload after thumbnail tickets: %d bytes for %d assets", got, itemCount)
}

func TestWorkspaceListDownloadStaysOwnerScoped(t *testing.T) {
	handler, ownerToken, _, stored := newWorkspaceListPerfFixture(t, 2, 0)
	assetID := stored[0].ID
	ownerReq := httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+assetID+"/download", nil)
	ownerReq.SetPathValue("id", assetID)
	ownerReq.Header.Set("Authorization", "Bearer "+ownerToken)
	ownerRes := httptest.NewRecorder()
	handler.downloadAsset(ownerRes, ownerReq)
	if ownerRes.Code != http.StatusOK {
		t.Fatalf("owner download status=%d body=%s", ownerRes.Code, ownerRes.Body.String())
	}
	if ownerRes.Body.Len() == 0 {
		t.Fatal("owner download returned empty body")
	}

	store := handler.store.(*jsonStore)
	other, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Other User", Email: "other-download@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	otherToken := "other-download-token"
	if err := handler.sessions.Put(context.Background(), otherToken, other.ID, authSessionTTL); err != nil {
		t.Fatal(err)
	}
	otherReq := httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+assetID+"/download", nil)
	otherReq.SetPathValue("id", assetID)
	otherReq.Header.Set("Authorization", "Bearer "+otherToken)
	otherRes := httptest.NewRecorder()
	handler.downloadAsset(otherRes, otherReq)
	if otherRes.Code != http.StatusNotFound {
		t.Fatalf("non-owner download status=%d body=%s", otherRes.Code, otherRes.Body.String())
	}
}

func assertWorkspaceListThumbnailDedup(t *testing.T, body []byte, tasks []generationTask, assets []asset) {
	t.Helper()
	var envelope struct {
		RecentTasks []json.RawMessage `json:"recentTasks"`
		Assets      []json.RawMessage `json:"assets"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.RecentTasks) == 0 || len(envelope.Assets) == 0 {
		t.Fatalf("workspace list missing tasks or assets: tasks=%d assets=%d", len(envelope.RecentTasks), len(envelope.Assets))
	}
	for i, raw := range envelope.RecentTasks {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			t.Fatal(err)
		}
		if _, ok := obj["thumbnailUrl"]; ok {
			t.Fatalf("recentTasks[%d] still has thumbnailUrl", i)
		}
		if _, ok := obj["resultIds"]; !ok {
			t.Fatalf("recentTasks[%d] dropped resultIds association", i)
		}
	}
	for i, raw := range envelope.Assets {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			t.Fatal(err)
		}
		if _, ok := obj["thumbnailUrl"]; !ok {
			t.Fatalf("assets[%d] dropped thumbnailUrl", i)
		}
		if _, ok := obj["taskId"]; !ok {
			t.Fatalf("assets[%d] dropped taskId association", i)
		}
	}
	for _, task := range tasks {
		if task.ThumbnailURL != "" {
			t.Fatalf("decoded task still carries thumbnailUrl: %+v", task)
		}
		if len(task.ResultIDs) == 0 {
			t.Fatalf("task lost resultIds: %+v", task)
		}
	}
	for _, item := range assets {
		assertWorkspaceListTicketThumbnailURL(t, item)
		if item.TaskID == "" {
			t.Fatalf("asset lost taskId: %+v", item)
		}
	}
}
