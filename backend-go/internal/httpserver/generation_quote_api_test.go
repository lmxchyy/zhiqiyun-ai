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

func TestGenerationQuoteUsesPricingEngineWithoutBillingSideEffects(t *testing.T) {
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

	prepared, quote, err := service.prepareGenerationQuote(data, user, generation.CreateRequest{
		Type: "TEXT_TO_VIDEO", Prompt: "quote only", Model: "mock-video",
		Params: map[string]any{"duration": float64(5), "resolution": "720p"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if quote.RequiredPoints <= 0 || quote.PricingRuleID == "" {
		t.Fatalf("invalid quote: %+v", quote)
	}
	formal, err := generationQuoteForRequest(prepared, data)
	if err != nil {
		t.Fatal(err)
	}
	if quote.RequiredPoints != formal.RequiredPoints || quote.PricingRuleID != formal.PricingRuleID {
		t.Fatalf("quote diverged from submit pricing: quote=%+v formal=%+v", quote, formal)
	}
	afterTasks, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}
	afterData, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	if len(afterTasks) != len(beforeTasks) {
		t.Fatalf("quote created task: before=%d after=%d", len(beforeTasks), len(afterTasks))
	}
	if pointsAvailableForAdminUser(afterData, user.ID) != beforePoints {
		t.Fatal("quote changed point balance")
	}
}

func TestGenerationQuoteUnknownModelFailsClosed(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	user := videoEstimateTestUser(t, data)
	service := newAPI(store, config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "api.json"), StaticDir: t.TempDir()}, newLocalAuthSessions(), nil)
	_, _, err = service.prepareGenerationQuote(data, user, generation.CreateRequest{
		Type: "TEXT_TO_VIDEO", Prompt: "unknown", Model: "not-a-published-model",
		Params: map[string]any{"duration": float64(5), "resolution": "720p"},
	})
	if err == nil {
		t.Fatal("unknown model unexpectedly received a quote")
	}
	if err.Error() == "" {
		t.Fatal("unknown model returned an empty error")
	}
}

func TestGenerationQuoteHTTPReturnsPreviewOnly(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	handler := newWithStore(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "api.json"), StaticDir: t.TempDir()}, store).Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")
	body, err := json.Marshal(generation.CreateRequest{
		Type: "TEXT_TO_VIDEO", Prompt: "quote only", Model: "mock-video",
		Params: map[string]any{"duration": float64(5), "resolution": "720p"},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks/quote", bytes.NewBuffer(body), token)
	if response.Code != http.StatusOK {
		t.Fatalf("quote status=%d body=%s", response.Code, response.Body.String())
	}
	var payload generationQuoteResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.RequiredPoints <= 0 || payload.PricingRuleID == "" || payload.BusinessType == "" {
		t.Fatalf("invalid quote payload: %+v", payload)
	}
}

func TestGenerationQuoteHTTPIsNotBlockedByInsufficientPoints(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	handler := newWithStore(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "api.json"), StaticDir: t.TempDir()}, store).Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	user := videoEstimateTestUser(t, data)
	err = store.updateAdmin(func(adminData *adminPlatformData) error {
		for i := range adminData.PointAccounts {
			if adminData.PointAccounts[i].UserID == user.ID {
				adminData.PointAccounts[i].Available = 0
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(generation.CreateRequest{
		Type: "TEXT_TO_VIDEO", Prompt: "quote insufficient", Model: "mock-video",
		Params: map[string]any{"duration": float64(5), "resolution": "720p"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Quote endpoint must return HTTP 200 without top-level code="INSUFFICIENT_POINTS"
	response := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks/quote", bytes.NewBuffer(body), token)
	if response.Code != http.StatusOK {
		t.Fatalf("quote status=%d body=%s", response.Code, response.Body.String())
	}
	var payload generationQuoteResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.RequiredPoints <= 0 {
		t.Fatalf("expected positive requiredPoints, got %d", payload.RequiredPoints)
	}
	if payload.Sufficient == nil || *payload.Sufficient != false {
		t.Fatalf("expected sufficient=false, got %v", payload.Sufficient)
	}
	if payload.Shortfall != payload.RequiredPoints {
		t.Fatalf("expected shortfall=%d, got %d", payload.RequiredPoints, payload.Shortfall)
	}
	if payload.Code != "" {
		t.Fatalf("quote HTTP 200 must NOT return top-level code %q, want empty string", payload.Code)
	}
}
