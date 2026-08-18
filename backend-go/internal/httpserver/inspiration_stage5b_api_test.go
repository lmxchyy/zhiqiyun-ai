package httpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"xianzhi-ai/backend-go/internal/config"
)

func TestPublicInspirationDetailUsesWhitelistProjection(t *testing.T) {
	handler := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()}).Handler
	token := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	definition := testInspirationDefinition("Create {{subject}}", "avoid {{subject}}")
	definition.Presentation["provider"] = "private-provider"
	definition.Presentation["nested"] = map[string]any{"executorKey": "private-executor", "safeLabel": "Visible"}
	published := publishAdminInspiration(t, handler, token, createAdminInspiration(t, handler, token, "public-projection", definition))
	response := request(t, handler, http.MethodGet, "/api/v1/inspirations/"+published.Slug+"?platform=miniprogram", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("detail = %d %s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	item, ok := payload["item"].(map[string]any)
	if !ok {
		t.Fatalf("item = %#v", payload["item"])
	}
	if item["slug"] != published.Slug || item["templateVersion"] != float64(published.Version) {
		t.Fatalf("public identity/version = %#v", item)
	}
	schema, ok := item["schema"].(map[string]any)
	if !ok || schema["inputs"] == nil || schema["presentation"] == nil || schema["handoff"] == nil {
		t.Fatalf("public schema projection = %#v", item["schema"])
	}
	for _, forbidden := range []string{"definition", "prompt", "negativePrompt", "composer", "bindings", "modelHint", "provider", "executorKey", "workflow", "failurePolicy"} {
		if jsonContainsKey(payload, forbidden) {
			t.Fatalf("public detail leaked %q: %s", forbidden, response.Body.String())
		}
	}
}

func TestAdminDefinitionSaveLoadAndPublishedEditRequiresReview(t *testing.T) {
	handler := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()}).Handler
	token := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	definition := testInspirationDefinition("Create {{subject}}", "avoid {{subject}}")
	created := createAdminInspiration(t, handler, token, "stage5b-admin-definition", definition)
	if created.Definition.Prompt.Template != definition.Prompt.Template {
		t.Fatalf("created definition = %#v", created.Definition)
	}

	get := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/inspirations/"+created.ID, nil, token)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"definition"`) || !strings.Contains(get.Body.String(), `"Create {{subject}}"`) {
		t.Fatalf("admin get = %d %s", get.Code, get.Body.String())
	}

	approve := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations/"+created.ID+"/audit/approve", nil, token)
	if approve.Code != http.StatusOK {
		t.Fatalf("approve = %d %s", approve.Code, approve.Body.String())
	}
	publish := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations/"+created.ID+"/publish", nil, token)
	if publish.Code != http.StatusOK {
		t.Fatalf("publish = %d %s", publish.Code, publish.Body.String())
	}
	var published struct {
		Item inspirationTemplate `json:"item"`
	}
	if err := json.NewDecoder(publish.Body).Decode(&published); err != nil {
		t.Fatal(err)
	}
	published.Item.Definition.Prompt.Template = "Updated {{subject}}"
	updateBody, _ := json.Marshal(published.Item)
	updated := authedRequest(t, handler, http.MethodPut, "/api/v1/admin/inspirations/"+created.ID, bytes.NewBuffer(updateBody), token)
	if updated.Code != http.StatusOK {
		t.Fatalf("update = %d %s", updated.Code, updated.Body.String())
	}
	var updatedPayload struct {
		Item inspirationTemplate `json:"item"`
	}
	if err := json.NewDecoder(updated.Body).Decode(&updatedPayload); err != nil {
		t.Fatal(err)
	}
	if updatedPayload.Item.Status != "DRAFT" || updatedPayload.Item.AuditStatus != "PENDING" {
		t.Fatalf("published edit gate = status %s audit %s", updatedPayload.Item.Status, updatedPayload.Item.AuditStatus)
	}
	if updatedPayload.Item.Version <= published.Item.Version {
		t.Fatalf("published edit version = %d, want > %d", updatedPayload.Item.Version, published.Item.Version)
	}
	republished := publishAdminInspiration(t, handler, token, updatedPayload.Item)
	rollbackBody := bytes.NewBufferString(`{"version":` + fmt.Sprintf("%d", published.Item.Version) + `}`)
	rollback := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations/"+republished.ID+"/rollback", rollbackBody, token)
	if rollback.Code != http.StatusOK {
		t.Fatalf("rollback = %d %s", rollback.Code, rollback.Body.String())
	}
	var rollbackPayload struct {
		Item inspirationTemplate `json:"item"`
	}
	if err := json.NewDecoder(rollback.Body).Decode(&rollbackPayload); err != nil {
		t.Fatal(err)
	}
	if rollbackPayload.Item.Status != "DRAFT" || rollbackPayload.Item.AuditStatus != "PENDING" {
		t.Fatalf("rollback review gate = status %s audit %s", rollbackPayload.Item.Status, rollbackPayload.Item.AuditStatus)
	}
}

func TestAdminRejectsInvalidDefinitionAndModelHint(t *testing.T) {
	handler := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()}).Handler
	token := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	invalidSchema := testInspirationDefinition("Create {{subject}}", "")
	invalidSchema.Prompt.Composer.Version = 99
	response := createAdminInspirationResponse(t, handler, token, "stage5b-invalid-schema", invalidSchema)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid schema = %d %s", response.Code, response.Body.String())
	}

	invalidModel := testInspirationDefinition("Create {{subject}}", "")
	invalidModel.Capability.ModelHint = "missing-model"
	response = createAdminInspirationResponse(t, handler, token, "stage5b-invalid-model", invalidModel)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid model hint = %d %s", response.Code, response.Body.String())
	}
}

func TestAdminWorkflowTemplateCannotPublishWithoutExecutor(t *testing.T) {
	handler := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()}).Handler
	token := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	definition := InternalTemplateDefinition{
		SchemaVersion: 1,
		Inputs:        []TemplateInputDefinition{},
		Prompt:        TemplatePromptDefinition{Composer: TemplateComposerDefinition{Key: "deterministic-template", Version: 1}},
		Bindings:      []TemplateBindingDefinition{},
		Presets:       TemplatePresetsDefinition{InputDefaults: map[string]any{}, GenerationDefaults: map[string]any{}},
		Presentation:  map[string]any{},
		Handoff:       TemplateHandoffDefinition{TargetType: "WORKFLOW_CREATION", TargetKey: "workflow.create"},
		Capability:    TemplateCapabilityDefinition{CapabilityKey: "workflow_execution"},
		Workflow: &TemplateWorkflowDefinition{
			WorkflowVersion: 1, ExecutorKey: "future-workflow-executor",
			Nodes: []TemplateWorkflowNode{{ID: "inputNode", Type: "INPUT"}}, Edges: []TemplateWorkflowEdge{},
			FailurePolicy: TemplateWorkflowFailurePolicy{Strategy: "FAIL_FAST"},
		},
	}
	body, _ := json.Marshal(map[string]any{
		"slug": "workflow-draft-only", "title": "Workflow draft", "contentType": "workflow",
		"categoryId": "inspiration-category-product", "coverUrl": "https://example.test/workflow.webp",
		"definition": definition, "platforms": []string{"miniprogram"}, "sourceAuthorized": true,
	})
	created := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations", bytes.NewBuffer(body), token)
	if created.Code != http.StatusOK {
		t.Fatalf("save workflow draft = %d %s", created.Code, created.Body.String())
	}
	var payload struct {
		Item inspirationTemplate `json:"item"`
	}
	if err := json.NewDecoder(created.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	approve := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations/"+payload.Item.ID+"/audit/approve", nil, token)
	if approve.Code != http.StatusOK {
		t.Fatalf("approve workflow = %d %s", approve.Code, approve.Body.String())
	}
	publish := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations/"+payload.Item.ID+"/publish", nil, token)
	if publish.Code != http.StatusConflict || !strings.Contains(publish.Body.String(), "executor") {
		t.Fatalf("workflow publish gate = %d %s", publish.Code, publish.Body.String())
	}
}

func TestInspirationSlugRoutesAndCopyContract(t *testing.T) {
	handler := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()}).Handler
	token := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	created := createAdminInspiration(t, handler, token, "slug-route-template", testInspirationDefinition("Create {{subject}}", ""))
	published := publishAdminInspiration(t, handler, token, created)

	event := authedRequest(t, handler, http.MethodPost, "/api/v1/inspirations/"+published.Slug+"/events", bytes.NewBufferString(`{"eventType":"use_template","platform":"miniprogram"}`), token)
	if event.Code != http.StatusOK {
		t.Fatalf("slug event route = %d %s", event.Code, event.Body.String())
	}
	favorite := authedRequest(t, handler, http.MethodPut, "/api/v1/inspirations/"+published.Slug+"/favorite", nil, token)
	if favorite.Code != http.StatusOK {
		t.Fatalf("slug favorite route = %d %s", favorite.Code, favorite.Body.String())
	}

	copyResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations/"+published.ID+"/copy", nil, token)
	if copyResponse.Code != http.StatusOK {
		t.Fatalf("copy = %d %s", copyResponse.Code, copyResponse.Body.String())
	}
	var copyPayload struct {
		Item inspirationTemplate `json:"item"`
	}
	if err := json.NewDecoder(copyResponse.Body).Decode(&copyPayload); err != nil {
		t.Fatal(err)
	}
	if copyPayload.Item.Slug == published.Slug || !strings.HasPrefix(copyPayload.Item.Slug, published.Slug+"-copy-") {
		t.Fatalf("copy slug = %q, source %q", copyPayload.Item.Slug, published.Slug)
	}
}

func createAdminInspiration(t *testing.T, handler http.Handler, token, slug string, definition InternalTemplateDefinition) inspirationTemplate {
	t.Helper()
	response := createAdminInspirationResponse(t, handler, token, slug, definition)
	if response.Code != http.StatusOK {
		t.Fatalf("create = %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Item inspirationTemplate `json:"item"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload.Item
}

func createAdminInspirationResponse(t *testing.T, handler http.Handler, token, slug string, definition InternalTemplateDefinition) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"slug": slug, "title": "Stage 5B template", "description": "contract test",
		"contentType": "image", "categoryId": "inspiration-category-product",
		"coverUrl": "https://example.test/cover.webp", "platforms": []string{"miniprogram"},
		"sourceAuthorized": true, "definition": definition,
	})
	if err != nil {
		t.Fatal(err)
	}
	return authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations", bytes.NewBuffer(body), token)
}

func jsonContainsKey(value any, key string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for candidate, child := range typed {
			if strings.EqualFold(candidate, key) || jsonContainsKey(child, key) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if jsonContainsKey(child, key) {
				return true
			}
		}
	}
	return false
}
