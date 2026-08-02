package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
)

type videoEstimateCapabilityStore struct {
	platformStore
	capabilityData adminPlatformData
	loadCount      int
}

func (s *videoEstimateCapabilityStore) aiCapabilityAdminData(context.Context) (adminPlatformData, error) {
	s.loadCount++
	return s.capabilityData, nil
}

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

	pending, err := store.CreatePendingGenerationTask(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if estimate.EstimatedPoints != pending.PointCost {
		t.Fatalf("estimate=%d formal=%d", estimate.EstimatedPoints, pending.PointCost)
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

func TestVideoGenerationEstimateHTTPUsesPublishedCapabilityBillingRules(t *testing.T) {
	baseStore := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	capabilityData, err := baseStore.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for index := range capabilityData.BillingRules {
		rule := &capabilityData.BillingRules[index]
		if canonicalModuleCode(rule.ModuleCode) != moduleVideoGeneration || rule.ModelName != "mock-video" {
			continue
		}
		rule.BasePrice = 80
		rule.MinimumCharge = 1
		rule.BillingType = "per_second"
		rule.BillingUnit = "PER_SECOND"
		rule.ParameterMultiplier = map[string]any{"resolution": map[string]any{"720p": float64(1.5)}}
		found = true
		break
	}
	if !found {
		t.Fatal("mock-video billing rule not found")
	}

	store := &videoEstimateCapabilityStore{platformStore: baseStore, capabilityData: capabilityData}
	handler := newWithStore(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "api.json"), StaticDir: t.TempDir()}, store).Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")
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
	if payload.EstimatedPoints != 600 {
		t.Fatalf("estimated points=%d, want published capability price 600", payload.EstimatedPoints)
	}
	if store.loadCount != 1 {
		t.Fatalf("capability billing load count=%d, want 1", store.loadCount)
	}
}
