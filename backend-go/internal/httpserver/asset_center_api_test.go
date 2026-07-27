package httpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
)

func TestAssetCenterLifecycle(t *testing.T) {
	server := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()})
	handler := server.Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

	created := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks", bytes.NewBufferString(`{
		"type":"TEXT_TO_IMAGE",
		"prompt":"enterprise asset center",
		"model":"mock-standard",
		"params":{"count":1}
	}`), token)
	if created.Code != http.StatusOK {
		t.Fatalf("create asset task status = %d, body = %s", created.Code, created.Body.String())
	}
	var task generationTask
	if err := json.NewDecoder(created.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	if len(task.ResultIDs) != 1 {
		t.Fatalf("created task has no asset: %+v", task)
	}
	assetID := task.ResultIDs[0]
	retried := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks/"+task.ID+"/retry", nil, token)
	if retried.Code != http.StatusOK {
		t.Fatalf("retry completed task status = %d, body = %s", retried.Code, retried.Body.String())
	}

	assertAssetCenterStatus(t, handler, token, http.MethodGet, "/api/v1/assets/overview", nil, http.StatusOK)
	favorite := assertAssetCenterStatus(t, handler, token, http.MethodPost, "/api/v1/assets/"+assetID+"/favorite", nil, http.StatusOK)
	if !favorite.Item.Favorite {
		t.Fatalf("favorite mutation did not persist: %+v", favorite.Item)
	}

	rename := assertAssetCenterStatus(t, handler, token, http.MethodPatch, "/api/v1/assets/"+assetID, bytes.NewBufferString(`{"name":"品牌主视觉"}`), http.StatusOK)
	if rename.Item.Name != "品牌主视觉" {
		t.Fatalf("rename = %q, want 品牌主视觉", rename.Item.Name)
	}

	move := assertAssetCenterStatus(t, handler, token, http.MethodPost, "/api/v1/assets/"+assetID+"/move-project", bytes.NewBufferString(`{"projectId":"project_b","projectName":"新品发布"}`), http.StatusOK)
	if stringValue(move.Item.Metadata["projectId"]) != "project_b" {
		t.Fatalf("project mutation did not persist: %+v", move.Item.Metadata)
	}

	filtered := authedRequest(t, handler, http.MethodGet, "/api/v1/assets?paged=true&page=1&pageSize=20&keyword=%E5%93%81%E7%89%8C&type=image&status=favorite", nil, token)
	if filtered.Code != http.StatusOK || !bytes.Contains(filtered.Body.Bytes(), []byte(assetID)) {
		t.Fatalf("filtered list status = %d, body = %s", filtered.Code, filtered.Body.String())
	}

	assertAssetCenterStatus(t, handler, token, http.MethodDelete, "/api/v1/assets/"+assetID, nil, http.StatusOK)
	recycled := authedRequest(t, handler, http.MethodGet, "/api/v1/assets?paged=true&status=recycled", nil, token)
	if recycled.Code != http.StatusOK || !bytes.Contains(recycled.Body.Bytes(), []byte(assetID)) {
		t.Fatalf("recycle list status = %d, body = %s", recycled.Code, recycled.Body.String())
	}
	assertAssetCenterStatus(t, handler, token, http.MethodPost, "/api/v1/assets/"+assetID+"/restore", nil, http.StatusOK)
	assertAssetCenterStatus(t, handler, token, http.MethodDelete, "/api/v1/assets/"+assetID, nil, http.StatusOK)
	assertAssetCenterStatus(t, handler, token, http.MethodDelete, "/api/v1/assets/"+assetID+"/permanent", nil, http.StatusOK)
}

func TestAssetCenterTaskCancellation(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	task, err := store.CreatePendingGenerationTask(generation.CreateRequest{
		UserID: "user_000002",
		Type:   "TEXT_TO_IMAGE",
		Prompt: "cancel me",
		Model:  "mock-standard",
		Params: map[string]any{"count": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.CancelGenerationTaskForUser(task.UserID, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "CANCELLED" {
		t.Fatalf("cancelled status = %q, want CANCELLED", cancelled.Status)
	}
	if err := store.DeleteGenerationTaskForUser(task.UserID, task.ID); err != nil {
		t.Fatalf("delete cancelled task: %v", err)
	}
	if _, found, err := func() (generationTask, bool, error) {
		tasks, listErr := store.ListGenerationTasks()
		if listErr != nil {
			return generationTask{}, false, listErr
		}
		for _, item := range tasks {
			if item.ID == task.ID {
				return item, true, nil
			}
		}
		return generationTask{}, false, nil
	}(); err != nil || found {
		t.Fatalf("deleted task still present found=%v err=%v", found, err)
	}
}

func TestAssetCenterTaskDeletionRejectsActive(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	task, err := store.CreatePendingGenerationTask(generation.CreateRequest{
		UserID: "user_000002",
		Type:   "TEXT_TO_IMAGE",
		Prompt: "active task",
		Model:  "mock-standard",
		Params: map[string]any{"count": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteGenerationTaskForUser(task.UserID, task.ID); err == nil {
		t.Fatal("expected active task delete to fail")
	}
}

func TestAssetCenterLightweightListCanSkipSummary(t *testing.T) {
	server := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()})
	handler := server.Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

	withoutSummary := authedRequest(t, handler, http.MethodGet, "/api/v1/assets?paged=true&limit=4&lightweight=true&includeSummary=false", nil, token)
	if withoutSummary.Code != http.StatusOK {
		t.Fatalf("lightweight list status = %d, body = %s", withoutSummary.Code, withoutSummary.Body.String())
	}
	if bytes.Contains(withoutSummary.Body.Bytes(), []byte(`"summary"`)) {
		t.Fatalf("lightweight list unexpectedly contains summary: %s", withoutSummary.Body.String())
	}

	withSummary := authedRequest(t, handler, http.MethodGet, "/api/v1/assets?paged=true&limit=4&lightweight=true", nil, token)
	if withSummary.Code != http.StatusOK || !bytes.Contains(withSummary.Body.Bytes(), []byte(`"summary"`)) {
		t.Fatalf("default paged list must keep summary compatibility: status = %d, body = %s", withSummary.Code, withSummary.Body.String())
	}
}

func TestRecentWorksEndpointReturnsOnlyCompactCardFields(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	handler := newWithStore(config.Config{Addr: ":0", StaticDir: t.TempDir()}, store).Handler
	login := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"demo@xianzhi.ai","password":"Demo123!"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", login.Code, login.Body.String())
	}
	var authPayload struct {
		AccessToken string `json:"accessToken"`
		User        struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(login.Body).Decode(&authPayload); err != nil {
		t.Fatal(err)
	}
	if authPayload.AccessToken == "" || authPayload.User.ID == "" {
		t.Fatalf("login response is incomplete: %+v", authPayload)
	}
	if err := store.update(func(data *platformData) error {
		for index := 0; index < 25; index++ {
			data.Assets = append(data.Assets, asset{
				ID:           fmt.Sprintf("large_asset_%02d", index),
				UserID:       authPayload.User.ID,
				TaskID:       fmt.Sprintf("task_%02d", index),
				Name:         fmt.Sprintf("作品 %02d", index),
				MediaType:    "image",
				URL:          "data:image/png;base64," + strings.Repeat("A", 256*1024),
				ThumbnailURL: "data:image/jpeg;base64," + strings.Repeat("B", 1024),
				Metadata: map[string]any{
					"sourceUrl":   "data:image/png;base64," + strings.Repeat("A", 256*1024),
					"projectId":   "project_recent",
					"projectName": "最近项目",
					"fileSize":    1024,
				},
				CreatedAt: fmt.Sprintf("2026-07-%02dT12:00:00Z", index+1),
				UpdatedAt: fmt.Sprintf("2026-07-%02dT12:00:00Z", index+1),
			})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	directItems, err := store.ListRecentWorksForUser(authPayload.User.ID, maxRecentWorksLimit)
	if err != nil || len(directItems) != maxRecentWorksLimit {
		t.Fatalf("direct recent works count = %d, err = %v", len(directItems), err)
	}
	response := authedRequest(t, handler, http.MethodGet, "/api/v1/works/recent", nil, authPayload.AccessToken)
	if response.Code != http.StatusOK {
		t.Fatalf("recent works status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Server-Timing") == "" {
		t.Fatal("recent works response is missing Server-Timing")
	}
	if response.Body.Len() > 100*1024 {
		t.Fatalf("recent works response is not compact: %d bytes", response.Body.Len())
	}
	for _, forbidden := range []string{`"metadata"`, `"sourceUrl"`, `"url"`} {
		if bytes.Contains(response.Body.Bytes(), []byte(forbidden)) {
			t.Fatalf("recent works leaked %s", forbidden)
		}
	}
	var payload struct {
		Items []recentWork `json:"items"`
	}
	rawBody := append([]byte(nil), response.Body.Bytes()...)
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != maxRecentWorksLimit {
		t.Fatalf("recent works count = %d, want %d, body = %s", len(payload.Items), maxRecentWorksLimit, string(rawBody))
	}
	if payload.Items[0].ID != "large_asset_24" {
		t.Fatalf("recent works order starts with %q, want large_asset_24", payload.Items[0].ID)
	}
	if payload.Items[recentWorksFirstPaintCovers-1].ThumbnailURL == "" || payload.Items[recentWorksFirstPaintCovers].ThumbnailURL != "" {
		t.Fatalf("recent works must include only first-paint covers: %#v", payload.Items[:recentWorksFirstPaintCovers+1])
	}
}

func assertAssetCenterStatus(t *testing.T, handler http.Handler, token string, method string, path string, body *bytes.Buffer, want int) struct {
	Item asset `json:"item"`
} {
	t.Helper()
	response := authedRequest(t, handler, method, path, body, token)
	if response.Code != want {
		t.Fatalf("%s %s status = %d, body = %s", method, path, response.Code, response.Body.String())
	}
	var payload struct {
		Item asset `json:"item"`
	}
	if response.Body.Len() > 0 {
		_ = json.NewDecoder(response.Body).Decode(&payload)
	}
	return payload
}
