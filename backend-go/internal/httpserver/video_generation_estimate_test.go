package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
)

type generationEstimateVideoProviderSpy struct {
	calls int
}

func (p *generationEstimateVideoProviderSpy) DefaultModel() string {
	return "mock-video"
}

func (p *generationEstimateVideoProviderSpy) Create(context.Context, generation.CreateRequest) (any, error) {
	p.calls++
	return map[string]any{"id": "provider-task-should-not-exist"}, nil
}

type generationEstimateSideEffects struct {
	tasks            int
	billingEvents    int
	billingLifecycle int
	walletLedger     int
	tokenRecords     int
}

func snapshotGenerationEstimateSideEffects(t *testing.T, store platformStore) generationEstimateSideEffects {
	t.Helper()
	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	return generationEstimateSideEffects{
		tasks:            len(data.GenerationTasks),
		billingEvents:    len(data.BillingEvents),
		billingLifecycle: len(data.BillingLifecycleEvents),
		walletLedger:     len(data.WalletLedger),
		tokenRecords:     len(data.TokenRecords),
	}
}

func generationEstimateHTTPResponse(t *testing.T, handler http.Handler, token, platform, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/generation-tasks/estimate", bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Client-Platform", platform)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestGenerationEstimateUsesRequestHeaderForMiniProgramComplianceAndStaysReadOnly(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "platform.json")
	store := newJSONStore(dataPath)
	sessions := newLocalAuthSessions()
	if err := sessions.Put(context.Background(), "estimate-token", "user_000002", authSessionTTL); err != nil {
		t.Fatal(err)
	}
	server := newWithStoreAndSessions(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()}, store, sessions)
	token := "estimate-token"
	body := `{
		"module_code":"video_generation",
		"type":"TEXT_TO_VIDEO",
		"prompt":"read-only estimate",
		"model":"mock-video",
		"params":{"terminal":"web","duration":5,"resolution":"720p","aspect_ratio":"16:9"}
	}`
	before := snapshotGenerationEstimateSideEffects(t, store)

	response := generationEstimateHTTPResponse(t, server.Handler, token, "mp-weixin", body)
	if response.Code != http.StatusForbidden {
		t.Fatalf("mini-program estimate status = %d, body = %s", response.Code, response.Body.String())
	}
	if after := snapshotGenerationEstimateSideEffects(t, store); after != before {
		t.Fatalf("rejected estimate mutated task or billing state: before=%+v after=%+v", before, after)
	}
}

func TestGenerationEstimateIgnoresForgedMiniProgramTerminalOnWebAndStaysReadOnly(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "platform.json")
	store := newJSONStore(dataPath)
	sessions := newLocalAuthSessions()
	if err := sessions.Put(context.Background(), "estimate-token", "user_000002", authSessionTTL); err != nil {
		t.Fatal(err)
	}
	server := newWithStoreAndSessions(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()}, store, sessions)
	token := "estimate-token"
	body := `{
		"module_code":"video_generation",
		"type":"TEXT_TO_VIDEO",
		"prompt":"read-only web estimate",
		"model":"mock-video",
		"params":{"terminal":"miniprogram","duration":5,"resolution":"720p","aspect_ratio":"16:9"}
	}`
	before := snapshotGenerationEstimateSideEffects(t, store)

	response := generationEstimateHTTPResponse(t, server.Handler, token, "web", body)
	if response.Code != http.StatusOK {
		t.Fatalf("web estimate status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		PointCost int    `json:"pointCost"`
		Terminal  string `json:"terminal"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.PointCost != 6 {
		t.Fatalf("web estimate pointCost = %d, want 6", payload.PointCost)
	}
	if payload.Terminal != "web" {
		t.Fatalf("web estimate terminal = %q, want header-derived web", payload.Terminal)
	}
	if after := snapshotGenerationEstimateSideEffects(t, store); after != before {
		t.Fatalf("successful estimate mutated task or billing state: before=%+v after=%+v", before, after)
	}
}

func TestGenerationEstimateDoesNotCallVideoProvider(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "platform.json")
	store := newJSONStore(dataPath)
	sessions := newLocalAuthSessions()
	if err := sessions.Put(context.Background(), "estimate-token", "user_000002", authSessionTTL); err != nil {
		t.Fatal(err)
	}
	provider := &generationEstimateVideoProviderSpy{}
	estimateAPI := api{
		store:    store,
		sessions: sessions,
		generationService: generation.NewServiceWithOptions(generation.ServiceOptions{
			VideoProvider: provider,
		}),
	}
	before := snapshotGenerationEstimateSideEffects(t, store)
	response := generationEstimateHTTPResponse(t, http.HandlerFunc(estimateAPI.estimateGenerationTaskCost), "estimate-token", "web", `{
		"module_code":"video_generation",
		"type":"TEXT_TO_VIDEO",
		"prompt":"provider must stay idle",
		"model":"mock-video",
		"params":{"duration":5,"resolution":"720p","aspect_ratio":"16:9"}
	}`)
	if response.Code != http.StatusOK {
		t.Fatalf("estimate status = %d, body = %s", response.Code, response.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("estimate called video provider %d times", provider.calls)
	}
	if after := snapshotGenerationEstimateSideEffects(t, store); after != before {
		t.Fatalf("estimate mutated task or billing state: before=%+v after=%+v", before, after)
	}
}

func TestDefaultSeedanceBillingRulesUseEightyPointBasePrice(t *testing.T) {
	rules := defaultBillingRules("2026-08-06T00:00:00Z")
	for _, model := range []string{"seedance-fast-2.0", "doubao-seedance-2.0"} {
		rule := selectBillingRule(rules, moduleVideoGeneration, model)
		if rule.ID == "" {
			t.Fatalf("default billing rule missing for %s", model)
		}
		if rule.BasePrice != 80 {
			t.Fatalf("%s default BasePrice = %v, want 80", model, rule.BasePrice)
		}
	}
}
