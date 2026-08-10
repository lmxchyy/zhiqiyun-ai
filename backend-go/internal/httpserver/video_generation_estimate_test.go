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

func TestVideoGenerationEstimateSeedanceDefaultMatches600(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	user := videoEstimateTestUser(t, data)
	service := newAPI(store, config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "api.json"), StaticDir: t.TempDir()}, newLocalAuthSessions(), nil)
	_, estimate, err := service.prepareVideoGenerationEstimate(data, user, generation.CreateRequest{
		Type:   "TEXT_TO_VIDEO",
		Prompt: "seedance default estimate",
		Model:  "doubao-seedance-2.0",
		Params: map[string]any{
			"duration":   float64(5),
			"resolution": "720p",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if estimate.EstimatedPoints != 600 {
		t.Fatalf("seedance 5s 720p estimate=%d, want 600", estimate.EstimatedPoints)
	}
	if estimate.Model != "doubao-seedance-2.0" {
		t.Fatalf("estimate model=%q, want doubao-seedance-2.0", estimate.Model)
	}
}

func TestGrokImagine15VideoEstimateIsFlatFifteenPointsPerSecond(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	user := videoEstimateTestUser(t, data)
	service := newAPI(store, config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "api.json"), StaticDir: t.TempDir()}, newLocalAuthSessions(), nil)
	tests := []struct {
		name       string
		duration   float64
		resolution string
		want       int
	}{
		{name: "6s 480p", duration: 6, resolution: "480p", want: 90},
		{name: "6s 720p", duration: 6, resolution: "720p", want: 90},
		{name: "30s 480p", duration: 30, resolution: "480p", want: 450},
		{name: "30s 720p", duration: 30, resolution: "720p", want: 450},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepared, estimate, err := service.prepareVideoGenerationEstimate(data, user, generation.CreateRequest{
				Type: "TEXT_TO_VIDEO", Prompt: "a cinematic sunrise", Model: "grok-imagine-1.5-video",
				Params: map[string]any{"duration": tt.duration, "resolution": tt.resolution, "aspect_ratio": "16:9"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if estimate.EstimatedPoints != tt.want {
				t.Fatalf("estimate=%d, want %d", estimate.EstimatedPoints, tt.want)
			}
			if got := generationPointCostForRequest(prepared, data); got != tt.want {
				t.Fatalf("formal point cost=%d, want %d", got, tt.want)
			}
			if estimate.BillingType != "per_second" {
				t.Fatalf("billing type=%q, want per_second", estimate.BillingType)
			}
		})
	}
}

func TestGrokImagine15VideoDefaultProviderCostIsThirteenCentsPerSecond(t *testing.T) {
	costs := defaultProviderCosts("2026-08-10T00:00:00Z")
	for _, cost := range costs {
		if cost.PlatformModelCode != "grok-imagine-1.5-video" {
			continue
		}
		if cost.BillingUnit != "PER_SECOND" || cost.UnitCost != 0.13 || cost.Currency != "CNY" {
			t.Fatalf("provider cost = %+v", cost)
		}
		return
	}
	t.Fatal("Grok Imagine 1.5 provider cost is missing")
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
