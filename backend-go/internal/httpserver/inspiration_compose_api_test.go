package httpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

func TestInspirationComposeValidatesVersionInputsAndMaterials(t *testing.T) {
	cfg := config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir(), InspirationDraftHMACSecret: "stage5b-compose-test-secret"}
	store := newJSONStore(cfg.DataPath)
	handler := newWithStoreAndSessions(cfg, store, newLocalAuthSessions()).Handler
	token := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	adminID, otherID := stage5BTestUserIDs(t, store)
	if err := store.update(func(data *platformData) error {
		data.Assets = append(data.Assets,
			asset{ID: "asset-owned-image", UserID: adminID, MediaType: "image", URL: "https://private.test/owned.png", Metadata: map[string]any{"contentType": "image/png"}},
			asset{ID: "asset-owned-video", UserID: adminID, MediaType: "video", URL: "https://private.test/owned.mp4", Metadata: map[string]any{"contentType": "video/mp4"}},
			asset{ID: "asset-other-image", UserID: otherID, MediaType: "image", URL: "https://private.test/other.png", Metadata: map[string]any{"contentType": "image/png"}},
		)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	definition := composeAPITestDefinition("hint-one")
	template := createAdminInspiration(t, handler, token, "stage5b-compose", definition)
	template = publishAdminInspiration(t, handler, token, template)
	before, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	beforeTasks, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}

	validBody := map[string]any{
		"templateVersion": template.Version,
		"values":          map[string]any{"subject": "  ceramic cup  "},
		"materials":       []map[string]any{{"inputKey": "references", "assetId": "asset-owned-image"}},
	}
	first := composeInspirationRequest(t, handler, token, template.Slug, validBody)
	if first.Code != http.StatusOK {
		t.Fatalf("compose = %d %s", first.Code, first.Body.String())
	}
	for _, forbidden := range []string{`"finalModel"`, `"price"`, `"points"`, `"taskStatus"`, `"workStatus"`} {
		if bytes.Contains(first.Body.Bytes(), []byte(forbidden)) {
			t.Fatalf("creation draft leaked execution field %s: %s", forbidden, first.Body.String())
		}
	}
	var firstPayload struct {
		Draft CreationDraft `json:"draft"`
	}
	if err = json.NewDecoder(first.Body).Decode(&firstPayload); err != nil {
		t.Fatal(err)
	}
	if firstPayload.Draft.ContractVersion != 1 || firstPayload.Draft.TemplateRef.Version != template.Version {
		t.Fatalf("draft identity = %#v", firstPayload.Draft)
	}
	if firstPayload.Draft.BasePrompt != "Create ceramic cup" || firstPayload.Draft.NegativePrompt != "avoid ceramic cup" {
		t.Fatalf("draft prompts = %q / %q", firstPayload.Draft.BasePrompt, firstPayload.Draft.NegativePrompt)
	}
	if firstPayload.Draft.Handoff.IntentKey != "hint-one" || firstPayload.Draft.CapabilityKey != "image_generation" || firstPayload.Draft.ModelHint != "gpt-image-2" {
		t.Fatalf("draft handoff/capability = %#v", firstPayload.Draft)
	}
	if firstPayload.Draft.IntegrityToken == "" || firstPayload.Draft.ExpiresAt == "" {
		t.Fatalf("draft integrity metadata = %#v", firstPayload.Draft)
	}

	second := composeInspirationRequest(t, handler, token, template.Slug, validBody)
	var secondPayload struct {
		Draft CreationDraft `json:"draft"`
	}
	if err = json.NewDecoder(second.Body).Decode(&secondPayload); err != nil {
		t.Fatal(err)
	}
	if secondPayload.Draft.BasePrompt != firstPayload.Draft.BasePrompt || secondPayload.Draft.NegativePrompt != firstPayload.Draft.NegativePrompt {
		t.Fatalf("composer is not deterministic: %#v / %#v", firstPayload.Draft, secondPayload.Draft)
	}

	conflictBody := cloneComposeRequest(validBody)
	conflictBody["templateVersion"] = template.Version + 100
	if response := composeInspirationRequest(t, handler, token, template.Slug, conflictBody); response.Code != http.StatusConflict || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"INSPIRATION_TEMPLATE_VERSION_CONFLICT"`)) {
		t.Fatalf("version conflict = %d %s", response.Code, response.Body.String())
	}
	missingInput := cloneComposeRequest(validBody)
	missingInput["values"] = map[string]any{}
	if response := composeInspirationRequest(t, handler, token, template.Slug, missingInput); response.Code != http.StatusBadRequest {
		t.Fatalf("missing input = %d %s", response.Code, response.Body.String())
	}
	clientComposed := cloneComposeRequest(validBody)
	clientComposed["prompt"] = "client must not override the server composer"
	if response := composeInspirationRequest(t, handler, token, template.Slug, clientComposed); response.Code != http.StatusBadRequest {
		t.Fatalf("client prompt override = %d %s", response.Code, response.Body.String())
	}
	for name, assetID := range map[string]string{"wrong_owner": "asset-other-image", "wrong_type": "asset-owned-video"} {
		body := cloneComposeRequest(validBody)
		body["materials"] = []map[string]any{{"inputKey": "references", "assetId": assetID}}
		if response := composeInspirationRequest(t, handler, token, template.Slug, body); response.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s", name, response.Code, response.Body.String())
		}
	}
	tooMany := cloneComposeRequest(validBody)
	tooMany["materials"] = []map[string]any{
		{"inputKey": "references", "assetId": "asset-owned-image"},
		{"inputKey": "references", "assetId": "asset-owned-image"},
		{"inputKey": "references", "assetId": "asset-owned-image"},
	}
	if response := composeInspirationRequest(t, handler, token, template.Slug, tooMany); response.Code != http.StatusBadRequest {
		t.Fatalf("material count = %d %s", response.Code, response.Body.String())
	}

	publishedVersion := template.Version
	template.Definition.Prompt.Template = "Changed {{subject}}"
	updateRaw, _ := json.Marshal(template)
	updated := authedRequest(t, handler, http.MethodPut, "/api/v1/admin/inspirations/"+template.ID, bytes.NewBuffer(updateRaw), token)
	if updated.Code != http.StatusOK {
		t.Fatalf("update for historical compose = %d %s", updated.Code, updated.Body.String())
	}
	var updatedPayload struct {
		Item inspirationTemplate `json:"item"`
	}
	if err = json.NewDecoder(updated.Body).Decode(&updatedPayload); err != nil {
		t.Fatal(err)
	}
	template = updatedPayload.Item
	template = publishAdminInspiration(t, handler, token, template)
	historicalBody := cloneComposeRequest(validBody)
	historicalBody["templateVersion"] = publishedVersion
	historical := composeInspirationRequest(t, handler, token, template.Slug, historicalBody)
	if historical.Code != http.StatusOK || !bytes.Contains(historical.Body.Bytes(), []byte(`"basePrompt":"Create ceramic cup"`)) {
		t.Fatalf("historical compose = %d %s", historical.Code, historical.Body.String())
	}

	after, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	afterTasks, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(afterTasks) != len(beforeTasks) || len(after.BillingEvents) != len(before.BillingEvents) || len(after.BillingLifecycleEvents) != len(before.BillingLifecycleEvents) || len(after.WalletLedger) != len(before.WalletLedger) {
		t.Fatalf("compose produced task/billing side effects: tasks %d/%d billing %d/%d lifecycle %d/%d wallet %d/%d", len(beforeTasks), len(afterTasks), len(before.BillingEvents), len(after.BillingEvents), len(before.BillingLifecycleEvents), len(after.BillingLifecycleEvents), len(before.WalletLedger), len(after.WalletLedger))
	}
}

func TestInspirationComposeIntentKeyDoesNotChangeComposer(t *testing.T) {
	repo := newMemoryInspirationRepository()
	first := inspirationTemplate{ID: "intent-one", Slug: "intent-one", TenantID: "default", Title: "One", ContentType: "image", CategoryID: "inspiration-category-product", CoverURL: "cover", Definition: testInspirationDefinition("Create {{subject}}", ""), Platforms: []string{"miniprogram"}, Status: "PUBLISHED", AuditStatus: "APPROVED"}
	first.Definition.Handoff.IntentKey = "photo_restoration"
	second := first
	second.ID, second.Slug = "intent-two", "intent-two"
	second.Definition.Handoff.IntentKey = "product_image"
	first, _ = repo.SaveTemplate(context.Background(), first, "create")
	second, _ = repo.SaveTemplate(context.Background(), second, "create")

	api := newInspirationAPI(repo, newJSONStore(filepath.Join(t.TempDir(), "store.json")), newLocalAuthSessions(), "intent-test-secret")
	compose := func(template inspirationTemplate) CreationDraft {
		requestBody, _ := json.Marshal(map[string]any{"templateVersion": template.Version, "values": map[string]any{"subject": "same subject"}})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/inspirations/"+template.Slug+"/compose?platform=miniprogram", bytes.NewReader(requestBody))
		req.SetPathValue("slug", template.Slug)
		recorder := httptest.NewRecorder()
		api.compose(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("compose %s = %d %s", template.Slug, recorder.Code, recorder.Body.String())
		}
		var payload struct {
			Draft CreationDraft `json:"draft"`
		}
		if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return payload.Draft
	}
	if one, two := compose(first), compose(second); one.BasePrompt != two.BasePrompt {
		t.Fatalf("intentKey changed composer output: %q != %q", one.BasePrompt, two.BasePrompt)
	}
}

func TestInspirationComposeDoesNotRecordTemplateEventOrMutation(t *testing.T) {
	base := newMemoryInspirationRepository()
	template, err := base.SaveTemplate(context.Background(), inspirationTemplate{
		ID: "no-side-effects", Slug: "no-side-effects", TenantID: "default", Title: "No side effects",
		ContentType: "image", CategoryID: "inspiration-category-product", CoverURL: "cover",
		Definition: testInspirationDefinition("Create {{subject}}", ""), Platforms: []string{"miniprogram"},
		Status: "PUBLISHED", AuditStatus: "APPROVED",
	}, "create")
	if err != nil {
		t.Fatal(err)
	}
	tracking := &trackingInspirationRepository{inspirationRepository: base}
	api := newInspirationAPI(tracking, newJSONStore(filepath.Join(t.TempDir(), "store.json")), newLocalAuthSessions(), "side-effect-test-secret")
	body, _ := json.Marshal(map[string]any{"templateVersion": template.Version, "values": map[string]any{"subject": "lamp"}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inspirations/no-side-effects/compose?platform=miniprogram", bytes.NewReader(body))
	req.SetPathValue("slug", template.Slug)
	recorder := httptest.NewRecorder()
	api.compose(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("compose = %d %s", recorder.Code, recorder.Body.String())
	}
	if tracking.recordEventCalls != 0 || tracking.saveTemplateCalls != 0 {
		t.Fatalf("repository side effects: record=%d save=%d", tracking.recordEventCalls, tracking.saveTemplateCalls)
	}
}

func TestCreationDraftIntegrityTokenTamperAndExpire(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	signer := newInspirationDraftSigner([]byte("01234567890123456789012345678901"), 30*time.Minute, func() time.Time { return now })
	draft := CreationDraft{
		ContractVersion: 1,
		TemplateRef:     CreationDraftTemplateRef{ID: "template-1", Slug: "template", Version: 7},
		ContentType:     "image", Values: map[string]any{"subject": "cup"},
		Materials:  []TemplateComposeMaterial{{InputKey: "references", AssetID: "asset-1"}},
		BasePrompt: "private prompt", CapabilityKey: "image_generation",
	}
	if err := signer.issue(&draft); err != nil {
		t.Fatal(err)
	}
	if !signer.trustedAttribution(draft) {
		t.Fatal("fresh draft token is not trusted")
	}
	payloadPart := strings.Split(draft.IntegrityToken, ".")[0]
	payload, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte("private prompt")) || bytes.Contains(payload, []byte("https://")) || bytes.Contains(payload, []byte("asset-1")) {
		t.Fatalf("token payload contains prompt, URL or raw material reference: %s", payload)
	}
	tampered := draft
	tampered.TemplateRef.Version++
	if signer.trustedAttribution(tampered) {
		t.Fatal("tampered draft attribution remained trusted")
	}
	now = now.Add(31 * time.Minute)
	if signer.trustedAttribution(draft) {
		t.Fatal("expired draft attribution remained trusted")
	}
}

type trackingInspirationRepository struct {
	inspirationRepository
	recordEventCalls  int
	saveTemplateCalls int
}

func (r *trackingInspirationRepository) RecordEvent(context.Context, string, string, string, string, string, string, map[string]any) error {
	r.recordEventCalls++
	return nil
}

func (r *trackingInspirationRepository) SaveTemplate(ctx context.Context, item inspirationTemplate, note string) (inspirationTemplate, error) {
	r.saveTemplateCalls++
	return r.inspirationRepository.SaveTemplate(ctx, item, note)
}

func composeAPITestDefinition(intentKey string) InternalTemplateDefinition {
	definition := testInspirationDefinition("Create {{subject}}", "avoid {{subject}}")
	minimum, maximum := 1, 2
	definition.Inputs = append(definition.Inputs, TemplateInputDefinition{
		Key: "references", Type: TemplateInputImage, Label: "References", Required: true,
		Validation: TemplateInputValidation{MinItems: &minimum, MaxItems: &maximum, Accept: []string{"image/*"}},
	})
	definition.Bindings = append(definition.Bindings, TemplateBindingDefinition{Source: "materials.references", Target: "parameters.referenceAssetIds", Transform: TemplateTransformAssetIDs})
	definition.Handoff.IntentKey = intentKey
	return definition
}

func composeInspirationRequest(t *testing.T, handler http.Handler, token, slug string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return authedRequest(t, handler, http.MethodPost, "/api/v1/inspirations/"+slug+"/compose?platform=miniprogram", bytes.NewBuffer(raw), token)
}

func publishAdminInspiration(t *testing.T, handler http.Handler, token string, item inspirationTemplate) inspirationTemplate {
	t.Helper()
	approve := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations/"+item.ID+"/audit/approve", nil, token)
	if approve.Code != http.StatusOK {
		t.Fatalf("approve = %d %s", approve.Code, approve.Body.String())
	}
	publish := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations/"+item.ID+"/publish", nil, token)
	if publish.Code != http.StatusOK {
		t.Fatalf("publish = %d %s", publish.Code, publish.Body.String())
	}
	var payload struct {
		Item inspirationTemplate `json:"item"`
	}
	if err := json.NewDecoder(publish.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload.Item
}

func cloneComposeRequest(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func stage5BTestUserIDs(t *testing.T, store *jsonStore) (string, string) {
	t.Helper()
	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	adminID, otherID := "", ""
	for _, user := range data.Users {
		if user.Email == "admin@xianzhi.ai" {
			adminID = user.ID
		} else if otherID == "" {
			otherID = user.ID
		}
	}
	if adminID == "" || otherID == "" {
		t.Fatalf("test users unavailable: admin=%q other=%q", adminID, otherID)
	}
	return adminID, otherID
}
