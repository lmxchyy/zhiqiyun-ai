package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

func TestPublicModuleSchemaReturnsExactGuestModelWithoutPrivateCapabilityData(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	expiresAt := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		for index := range data.AIModels {
			if data.AIModels[index].ModelName != "gpt-image-2" {
				continue
			}
			qualified := compliantMiniProgramModel(expiresAt)
			qualified.ID = data.AIModels[index].ID
			qualified.ModelName = "gpt-image-2"
			qualified.ModuleCode = moduleImageGeneration
			qualified.Status = "ACTIVE"
			data.AIModels[index] = qualified
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	handler := newWithStore(config.Config{Addr: ":0", StaticDir: t.TempDir()}, store).Handler
	request := httptest.NewRequest(http.MethodGet, "/api/v1/public/module-schema?module_code=image_generation&model_name=gpt-image-2", nil)
	request.Header.Set("X-Client-Platform", "mp-weixin")
	request.Header.Set("X-Client-Name", "xianzhi-mini-program")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("public module schema status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["module_code"] != moduleImageGeneration || payload["model_name"] != "gpt-image-2" {
		t.Fatalf("unexpected public schema identity: %#v", payload)
	}
	for _, required := range []string{"schema", "fields"} {
		if payload[required] == nil {
			t.Fatalf("public schema missing %s: %#v", required, payload)
		}
	}
	for _, forbidden := range []string{"limit_json", "billing_rule", "context", "module", "model"} {
		if _, exists := payload[forbidden]; exists {
			t.Fatalf("public schema leaked %s: %#v", forbidden, payload)
		}
	}
}

func TestPublicModuleSchemaReturnsGuestVideoFieldsAndCapabilities(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	expiresAt := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		for index := range data.AIModels {
			if data.AIModels[index].ModelName != "grok-imagine-1.5-video" {
				continue
			}
			qualified := compliantMiniProgramModel(expiresAt)
			qualified.ID = data.AIModels[index].ID
			qualified.ModelName = "grok-imagine-1.5-video"
			qualified.ModelType = "video"
			qualified.ModuleCode = moduleVideoGeneration
			qualified.Status = "ACTIVE"
			qualified.AllowedCapabilities = []string{"video"}
			data.AIModels[index] = qualified
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	handler := newWithStore(config.Config{Addr: ":0", StaticDir: t.TempDir()}, store).Handler
	request := httptest.NewRequest(http.MethodGet, "/api/v1/public/module-schema?module_code=video_generation&model_name=grok-imagine-1.5-video", nil)
	request.Header.Set("X-Client-Platform", "mp-weixin")
	request.Header.Set("X-Client-Name", "xianzhi-mini-program")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("public video schema status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	fields, ok := payload["fields"].([]any)
	if !ok || len(fields) == 0 {
		t.Fatalf("public video schema has no fields: %#v", payload)
	}
	if payload["video_capabilities"] == nil {
		t.Fatalf("public video schema has no capabilities: %#v", payload)
	}
}

func TestPublicModuleSchemaRejectsNonCompliantExactModelWithoutSwitching(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	expiresAt := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		for index := range data.AIModels {
			switch data.AIModels[index].ModelName {
			case "gpt-image-2":
				data.AIModels[index].ComplianceStatus = "draft"
			case "mock-standard":
				qualified := compliantMiniProgramModel(expiresAt)
				qualified.ID = data.AIModels[index].ID
				qualified.ModelName = "mock-standard"
				qualified.ModuleCode = moduleImageGeneration
				qualified.Status = "ACTIVE"
				data.AIModels[index] = qualified
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	handler := newWithStore(config.Config{Addr: ":0", StaticDir: t.TempDir()}, store).Handler
	request := httptest.NewRequest(http.MethodGet, "/api/v1/public/module-schema?module_code=image_generation&model_name=gpt-image-2", nil)
	request.Header.Set("X-Client-Platform", "mp-weixin")
	request.Header.Set("X-Client-Name", "xianzhi-mini-program")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("non-compliant exact model status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "mock-standard") {
		t.Fatalf("public schema silently switched models: %s", response.Body.String())
	}
}

func TestPublicModuleSchemaRequiresAnExactModel(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	handler := newWithStore(config.Config{Addr: ":0", StaticDir: t.TempDir()}, store).Handler
	request := httptest.NewRequest(http.MethodGet, "/api/v1/public/module-schema?module_code=image_generation", nil)
	request.Header.Set("X-Client-Platform", "mp-weixin")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing exact model status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "model_name is required") {
		t.Fatalf("missing exact model error = %s", response.Body.String())
	}
}
