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

func TestPrepareGenerationRequestRecordsTrustedInspirationAttribution(t *testing.T) {
	cfg := config.Config{
		Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir(),
		InspirationDraftHMACSecret: "attribution-test-secret",
	}
	store := newJSONStore(cfg.DataPath)
	sessions := newLocalAuthSessions()
	handler := newWithStoreAndSessions(cfg, store, sessions).Handler
	token := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	adminID, _ := stage5BTestUserIDs(t, store)
	if err := store.update(func(data *platformData) error {
		data.Assets = append(data.Assets, asset{
			ID: "asset-owned-image", UserID: adminID, MediaType: "image", URL: "https://private.test/owned.png",
			Metadata: map[string]any{"contentType": "image/png"},
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	template := publishAdminInspiration(t, handler, token, createAdminInspiration(t, handler, token, "attribution-hero", composeAPITestDefinition("hint-one")))
	compose := composeInspirationRequest(t, handler, token, template.Slug, map[string]any{
		"templateVersion": template.Version,
		"values":          map[string]any{"subject": "ceramic cup"},
		"materials":       []map[string]any{{"inputKey": "references", "assetId": "asset-owned-image"}},
	})
	if compose.Code != http.StatusOK {
		t.Fatalf("compose = %d %s", compose.Code, compose.Body.String())
	}
	var payload struct {
		Draft CreationDraft `json:"draft"`
	}
	if err := json.NewDecoder(compose.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}

	api := newAPI(store, cfg, sessions, nil)
	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := api.prepareGenerationRequest(data, adminUser{ID: adminID, Role: "MEMBER", PlanID: "plan_month"}, generation.CreateRequest{
		Type: "TEXT_TO_IMAGE", ModuleCode: moduleImageGeneration, Prompt: payload.Draft.BasePrompt, Model: "gpt-image-2",
		Params: map[string]any{
			"size": "1024x1024", "quality": "high", "n": float64(1),
			"inspirationDraft": payload.Draft,
			"integrityToken":   payload.Draft.IntegrityToken,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Prompt != "Create ceramic cup" {
		t.Fatalf("prompt = %q", prepared.Prompt)
	}
	if prepared.Params["size"] != "1024x1024" || prepared.Params["quality"] != "high" {
		t.Fatalf("canonical generation params = %#v", prepared.Params)
	}
	if _, exists := prepared.Params["inspirationDraft"]; exists {
		t.Fatalf("client draft must not remain after prepare: %#v", prepared.Params["inspirationDraft"])
	}
	if prepared.Params["inspiration_trusted"] != true || prepared.Params["inspiration_source"] != "template" {
		t.Fatalf("trusted attribution missing: %#v", prepared.Params)
	}
	if prepared.Params["inspiration_template_id"] != template.ID || prepared.Params["inspiration_template_slug"] != template.Slug {
		t.Fatalf("template identity = %#v", prepared.Params)
	}
}

func TestPrepareGenerationRequestIgnoresTamperedInspirationAttribution(t *testing.T) {
	cfg := config.Config{
		Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir(),
		InspirationDraftHMACSecret: "attribution-test-secret",
	}
	store := newJSONStore(cfg.DataPath)
	sessions := newLocalAuthSessions()
	handler := newWithStoreAndSessions(cfg, store, sessions).Handler
	token := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	adminID, _ := stage5BTestUserIDs(t, store)
	template := publishAdminInspiration(t, handler, token, createAdminInspiration(t, handler, token, "attribution-tamper", testInspirationDefinition("Create {{subject}}", "")))
	compose := composeInspirationRequest(t, handler, token, template.Slug, map[string]any{
		"templateVersion": template.Version,
		"values":          map[string]any{"subject": "lamp"},
		"materials":       []map[string]any{},
	})
	if compose.Code != http.StatusOK {
		t.Fatalf("compose = %d %s", compose.Code, compose.Body.String())
	}
	var payload struct {
		Draft CreationDraft `json:"draft"`
	}
	if err := json.NewDecoder(compose.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	payload.Draft.IntegrityToken = "tampered." + payload.Draft.IntegrityToken
	payload.Draft.TemplateRef.ID = "forged-template"

	api := newAPI(store, cfg, sessions, nil)
	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := api.prepareGenerationRequest(data, adminUser{ID: adminID, Role: "MEMBER", PlanID: "plan_month"}, generation.CreateRequest{
		Type: "TEXT_TO_IMAGE", ModuleCode: moduleImageGeneration, Prompt: "Create lamp", Model: "gpt-image-2",
		Params: map[string]any{"size": "1024x1024", "quality": "low", "n": float64(1), "inspirationDraft": payload.Draft, "inspiration_trusted": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Prompt == "" {
		t.Fatal("tampered attribution must still allow ordinary creation")
	}
	if prepared.Params["inspiration_trusted"] != nil || prepared.Params["inspiration_template_id"] != nil {
		t.Fatalf("tampered draft was trusted: %#v", prepared.Params)
	}
}

func TestPrepareGenerationRequestIgnoresExpiredInspirationAttribution(t *testing.T) {
	cfg := config.Config{
		Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir(),
		InspirationDraftHMACSecret: "attribution-test-secret",
	}
	store := newJSONStore(cfg.DataPath)
	sessions := newLocalAuthSessions()
	handler := newWithStoreAndSessions(cfg, store, sessions).Handler
	token := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	adminID, _ := stage5BTestUserIDs(t, store)
	template := publishAdminInspiration(t, handler, token, createAdminInspiration(t, handler, token, "attribution-expired", testInspirationDefinition("Create {{subject}}", "")))
	compose := composeInspirationRequest(t, handler, token, template.Slug, map[string]any{
		"templateVersion": template.Version,
		"values":          map[string]any{"subject": "lamp"},
		"materials":       []map[string]any{},
	})
	if compose.Code != http.StatusOK {
		t.Fatalf("compose = %d %s", compose.Code, compose.Body.String())
	}
	var payload struct {
		Draft CreationDraft `json:"draft"`
	}
	if err := json.NewDecoder(compose.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	payload.Draft.ExpiresAt = "2020-01-01T00:00:00Z"

	api := newAPI(store, cfg, sessions, nil)
	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := api.prepareGenerationRequest(data, adminUser{ID: adminID, Role: "MEMBER", PlanID: "plan_month"}, generation.CreateRequest{
		Type: "TEXT_TO_IMAGE", ModuleCode: moduleImageGeneration, Prompt: "Create lamp", Model: "gpt-image-2",
		Params: map[string]any{"size": "1024x1024", "quality": "low", "n": float64(1), "inspirationDraft": payload.Draft},
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Params["inspiration_trusted"] != nil || prepared.Params["inspiration_template_id"] != nil {
		t.Fatalf("expired draft was trusted: %#v", prepared.Params)
	}
}

func TestAdminSaveRejectsLegacyPromptPayload(t *testing.T) {
	handler := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()}).Handler
	token := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	raw, err := json.Marshal(map[string]any{
		"title": "旧提示词模板", "contentType": "image", "categoryId": "inspiration-category-product",
		"coverUrl": "/cover.png", "prompt": "a photo", "modelId": "gpt-image-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	response := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations", bytes.NewBuffer(raw), token)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("legacy admin payload = %d %s", response.Code, response.Body.String())
	}
}
