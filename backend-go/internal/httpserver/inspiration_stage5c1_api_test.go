package httpserver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"xianzhi-ai/backend-go/internal/config"
)

func TestReferenceImageUploadReturnsOwnedAssetForTemplateCompose(t *testing.T) {
	cfg := config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir(), InspirationDraftHMACSecret: "stage5c1-upload-test-secret"}
	store := newJSONStore(cfg.DataPath)
	handler := newWithStoreAndSessions(cfg, store, newLocalAuthSessions()).Handler
	token := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	image, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "reference.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(image); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/reference-images", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("upload = %d %s", response.Code, response.Body.String())
	}
	var uploadPayload struct {
		Item struct {
			AssetID     string `json:"assetId"`
			URL         string `json:"url"`
			ContentType string `json:"contentType"`
		} `json:"item"`
	}
	if err = json.NewDecoder(response.Body).Decode(&uploadPayload); err != nil {
		t.Fatal(err)
	}
	if uploadPayload.Item.AssetID == "" || uploadPayload.Item.URL == "" || uploadPayload.Item.ContentType != "image/png" {
		t.Fatalf("upload payload = %#v", uploadPayload.Item)
	}

	definition := composeAPITestDefinition("product_image")
	template := createAdminInspiration(t, handler, token, "stage5c1-upload-compose", definition)
	template = publishAdminInspiration(t, handler, token, template)
	compose := composeInspirationRequest(t, handler, token, template.Slug, map[string]any{
		"templateVersion": template.Version,
		"values":          map[string]any{"subject": "ceramic cup"},
		"materials":       []map[string]any{{"inputKey": "references", "assetId": uploadPayload.Item.AssetID}},
	})
	if compose.Code != http.StatusOK {
		t.Fatalf("compose uploaded asset = %d %s", compose.Code, compose.Body.String())
	}
	var composePayload struct {
		Draft CreationDraft `json:"draft"`
	}
	if err = json.NewDecoder(compose.Body).Decode(&composePayload); err != nil {
		t.Fatal(err)
	}
	if composePayload.Draft.CreatedAt == "" || composePayload.Draft.ExpiresAt == "" {
		t.Fatalf("creation timestamps = %#v", composePayload.Draft)
	}
}

func TestPublicTemplateDefinitionProjectsSchemaDrivenFormPresentation(t *testing.T) {
	definition := testInspirationDefinition("Create {{subject}}", "")
	definition.Inputs[0].Control = "SEGMENTED"
	definition.Inputs[0].Section = "preferences"
	definition.Inputs[0].Order = 30
	definition.Inputs[0].Advanced = true

	projected := projectPublicTemplateDefinition(definition)
	if len(projected.Inputs) == 0 {
		t.Fatal("public inputs are empty")
	}
	input := projected.Inputs[0]
	if input.Control != "SEGMENTED" || input.Section != "preferences" || input.Order != 30 || !input.Advanced {
		t.Fatalf("form presentation projection = %#v", input)
	}
}

func TestTemplateDefinitionRejectsInvalidFormControlAndSection(t *testing.T) {
	definition := testInspirationDefinition("Create {{subject}}", "")
	definition.Inputs[0].Control = "SLIDER"
	definition.Inputs[0].Section = "hidden"

	issues := validateTemplateDefinition("IMAGE", definition)
	codes := map[string]bool{}
	for _, issue := range issues {
		codes[issue.Code] = true
	}
	if !codes["INPUT_CONTROL_INCOMPATIBLE"] {
		t.Fatalf("expected incompatible control issue, got %#v", issues)
	}
	if !codes["INPUT_SECTION_INVALID"] {
		t.Fatalf("expected invalid section issue, got %#v", issues)
	}
}

func TestComposeSkipsRequiredInputsAndMaterialsWhileVisibilityConditionIsFalse(t *testing.T) {
	definition := testInspirationDefinition("Create {{subject}} {{details}}", "")
	definition.Inputs = append(definition.Inputs,
		TemplateInputDefinition{
			Key: "style", Type: TemplateInputSelect, Label: "Style", Default: "simple",
			Options: []TemplateInputOption{{Label: "Simple", Value: "simple"}, {Label: "Detailed", Value: "detailed"}},
		},
		TemplateInputDefinition{
			Key: "details", Type: TemplateInputTextarea, Label: "Details", Required: true,
			VisibleWhen: &TemplateVisibilityCondition{InputKey: "style", Operator: "eq", Value: "detailed"},
		},
		TemplateInputDefinition{
			Key: "references", Type: TemplateInputImage, Label: "References", Required: true,
			VisibleWhen: &TemplateVisibilityCondition{InputKey: "style", Operator: "eq", Value: "detailed"},
		},
	)
	definition.Bindings = append(definition.Bindings, TemplateBindingDefinition{Source: "inputs.details", Target: "prompt.variables.details", Transform: TemplateTransformTrim})

	composition, err := composeTemplateDefinition(definition, map[string]any{"subject": "cup", "style": "simple"}, []TemplateComposeMaterial{{InputKey: "references", AssetID: "ignored-hidden-asset"}})
	if err != nil {
		t.Fatalf("compose hidden inputs: %v", err)
	}
	if _, exists := composition.Values["details"]; exists {
		t.Fatalf("hidden details leaked into normalized values: %#v", composition.Values)
	}
	if composition.BasePrompt != "Create cup" {
		t.Fatalf("hidden prompt binding = %q", composition.BasePrompt)
	}
	if len(composition.Materials) != 0 {
		t.Fatalf("hidden materials leaked into composition: %#v", composition.Materials)
	}
	if _, err = composeTemplateDefinition(definition, map[string]any{"subject": "cup", "style": "detailed"}, nil); err == nil {
		t.Fatal("visible required inputs were not enforced")
	}
}

func TestTemplateDefinitionRejectsUnsupportedVisibilityOperator(t *testing.T) {
	definition := testInspirationDefinition("Create {{subject}}", "")
	definition.Inputs[0].VisibleWhen = &TemplateVisibilityCondition{InputKey: "other", Operator: "javascript", Value: true}
	definition.Inputs = append(definition.Inputs, TemplateInputDefinition{Key: "other", Type: TemplateInputBoolean, Label: "Other"})

	issues := validateTemplateDefinition("IMAGE", definition)
	for _, issue := range issues {
		if issue.Code == "VISIBILITY_OPERATOR_UNSUPPORTED" {
			return
		}
	}
	t.Fatalf("expected visibility operator issue, got %#v", issues)
}
