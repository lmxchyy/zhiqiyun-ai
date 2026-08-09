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

func videoEstimateTestUser(t *testing.T, data adminPlatformData) adminUser {
	t.Helper()
	for _, user := range data.Users {
		if user.Email == "demo@xianzhi.ai" {
			return user
		}
	}
	t.Fatal("seed demo user not found")
	return adminUser{}
}

func videoEstimateTestRequest() generation.CreateRequest {
	return generation.CreateRequest{
		Type:   "TEXT_TO_VIDEO",
		Prompt: "a cinematic sunrise",
		Model:  "mock-video",
		Params: map[string]any{
			"duration":     float64(5),
			"resolution":   "720p",
			"aspect_ratio": "16:9",
		},
	}
}

func TestVideoGenerationEstimateMatchesFormalPointCostWithoutSideEffects(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	user := videoEstimateTestUser(t, data)
	beforeTasks, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}
	beforePoints := pointsAvailableForAdminUser(data, user.ID)

	service := newAPI(store, config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "api.json"), StaticDir: t.TempDir()}, newLocalAuthSessions(), nil)
	prepared, estimate, err := service.prepareVideoGenerationEstimate(data, user, videoEstimateTestRequest())
	if err != nil {
		t.Fatal(err)
	}

	afterEstimateTasks, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}
	afterEstimateData, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	if len(afterEstimateTasks) != len(beforeTasks) {
		t.Fatalf("estimate created tasks: before=%d after=%d", len(beforeTasks), len(afterEstimateTasks))
	}
	if got := pointsAvailableForAdminUser(afterEstimateData, user.ID); got != beforePoints {
		t.Fatalf("estimate changed points: before=%d after=%d", beforePoints, got)
	}

	formal := generationPointCostForRequest(prepared, data)
	if estimate.EstimatedPoints != formal {
		t.Fatalf("estimate=%d formal=%d", estimate.EstimatedPoints, formal)
	}
	if estimate.EstimatedPoints <= 0 || estimate.Model == "" || estimate.BillingType == "" {
		t.Fatalf("invalid estimate payload: %+v", estimate)
	}
}

func TestVideoGenerationEstimateHTTPIsReadOnly(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	handler := newWithStore(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "api.json"), StaticDir: t.TempDir()}, store).Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")
	beforeTasks, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(videoEstimateTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	response := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks/estimate", bytes.NewBuffer(body), token)
	if response.Code != http.StatusOK {
		t.Fatalf("estimate status=%d body=%s", response.Code, response.Body.String())
	}
	var payload videoGenerationEstimate
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.EstimatedPoints <= 0 || payload.Model != "mock-video" || payload.BillingType == "" {
		t.Fatalf("invalid estimate payload: %+v", payload)
	}
	afterTasks, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(afterTasks) != len(beforeTasks) {
		t.Fatalf("estimate endpoint created tasks: before=%d after=%d", len(beforeTasks), len(afterTasks))
	}
}
