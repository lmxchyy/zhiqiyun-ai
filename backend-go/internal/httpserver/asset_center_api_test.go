package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
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
