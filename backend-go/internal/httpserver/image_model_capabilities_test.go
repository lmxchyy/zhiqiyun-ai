package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
)

func TestGPTImageCapabilitiesExposeProviderMaxCount(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	handler := newWithStore(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "api.json"), StaticDir: t.TempDir()}, store).Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")
	response := authedRequest(t, handler, http.MethodGet, "/api/v1/models/gpt-image-2/capabilities", nil, token)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	refs, _ := payload["referenceImages"].(map[string]any)
	if refs == nil {
		if caps, ok := payload["capabilities"].(map[string]any); ok {
			refs, _ = caps["referenceImages"].(map[string]any)
		}
	}
	if refs == nil {
		t.Fatalf("missing referenceImages: %+v", payload)
	}
	maxCount, _ := anyToFloat(refs["maxCount"])
	supported, _ := refs["supported"].(bool)
	if !supported || int(maxCount) != 16 {
		t.Fatalf("gpt-image-2 capabilities = %+v, want supported maxCount=16", refs)
	}
}

func TestImageReferenceCountsFollowProviderCapability(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	user := videoEstimateTestUser(t, data)
	service := newAPI(store, config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "api.json"), StaticDir: t.TempDir()}, newLocalAuthSessions(), nil)
	for _, count := range []int{3, 10, 16} {
		_, err := service.prepareGenerationRequest(data, user, generation.CreateRequest{
			Type: "IMAGE_TO_IMAGE", Prompt: "edit product photo", Model: "gpt-image-2",
			Params: map[string]any{"size": "1024x1024", "quality": "low", "n": float64(1), "referenceImages": dummyImageReferenceItems(count)},
		})
		if err != nil {
			t.Fatalf("count=%d unexpected error: %v", count, err)
		}
	}
	_, err = service.prepareGenerationRequest(data, user, generation.CreateRequest{
		Type: "IMAGE_TO_IMAGE", Prompt: "edit product photo", Model: "gpt-image-2",
		Params: map[string]any{"size": "1024x1024", "quality": "low", "n": float64(1), "referenceImages": dummyImageReferenceItems(17)},
	})
	if err == nil {
		t.Fatal("17 reference images unexpectedly accepted")
	}
	var coded interface{ BusinessCode() string }
	if !errors.As(err, &coded) || coded.BusinessCode() != "REFERENCE_IMAGE_LIMIT_EXCEEDED" {
		t.Fatalf("err=%v code=%v, want REFERENCE_IMAGE_LIMIT_EXCEEDED", err, coded)
	}
	var detailed interface{ ErrorDetails() map[string]any }
	if !errors.As(err, &detailed) {
		t.Fatalf("missing error details: %v", err)
	}
	details := detailed.ErrorDetails()
	if int(details["maxReferenceImages"].(int)) != 16 || int(details["actualReferenceImages"].(int)) != 17 {
		t.Fatalf("details=%v", details)
	}
}

func TestImageQuoteGoldenPricesAndNoBillingSideEffects(t *testing.T) {
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
	cases := []struct {
		size, quality string
		n, want       int
	}{
		{"1024x1024", "low", 1, 10},
		{"1024x1024", "high", 1, 15},
		{"2048x2048", "low", 4, 60},
		{"3840x2160", "high", 4, 120},
	}
	for _, tt := range cases {
		_, quote, err := service.prepareGenerationQuote(data, user, generation.CreateRequest{
			Type: "TEXT_TO_IMAGE", Prompt: "quote only", Model: "gpt-image-2",
			Params: map[string]any{"size": tt.size, "quality": tt.quality, "n": float64(tt.n)},
		})
		if err != nil {
			t.Fatalf("%s %s n=%d err=%v", tt.size, tt.quality, tt.n, err)
		}
		if quote.RequiredPoints != tt.want {
			t.Fatalf("%s %s n=%d quoted %d, want %d", tt.size, tt.quality, tt.n, quote.RequiredPoints, tt.want)
		}
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

func TestImageQuoteHTTPIncludesPricingAndBalance(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	handler := newWithStore(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "api.json"), StaticDir: t.TempDir()}, store).Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")
	body, err := json.Marshal(generation.CreateRequest{
		Type: "TEXT_TO_IMAGE", Prompt: "quote only", Model: "gpt-image-2",
		Params: map[string]any{"size": "2048x2048", "quality": "low", "n": float64(4)},
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
	if payload.RequiredPoints != 60 {
		t.Fatalf("requiredPoints=%d want 60 payload=%+v", payload.RequiredPoints, payload)
	}
	if payload.Pricing == nil {
		t.Fatal("missing pricing breakdown")
	}
	if payload.CurrentPoints < 0 {
		t.Fatalf("currentPoints=%d", payload.CurrentPoints)
	}
}
