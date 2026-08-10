package httpserver

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

func TestPublicModelsDoNotLeakProviderRouting(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	handler := New(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()}).Handler
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/models", nil)
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("models status = %d, body = %s", res.Code, res.Body.String())
	}
	var items []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("public model list is empty")
	}
	for _, item := range items {
		for _, forbidden := range []string{"providerId", "providerName", "provider", "apiKey", "cost", "internalMultiplier"} {
			if _, exists := item[forbidden]; exists {
				t.Fatalf("public model leaked %s: %#v", forbidden, item)
			}
		}
		if item["id"] == nil || item["displayName"] == nil || item["capabilities"] == nil {
			t.Fatalf("public model is missing display fields: %#v", item)
		}
	}
}

func TestPublicCatalogIsAnonymousAndPresentationOnly(t *testing.T) {
	handler := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()}).Handler
	for _, route := range []string{"/api/v1/public/home", "/api/v1/public/cases", "/api/v1/public/templates", "/api/v1/public/agents", "/api/v1/public/models", "/api/v1/public/pricing"} {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, route, nil))
		if res.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", route, res.Code, res.Body.String())
		}
		body := res.Body.String()
		for _, forbidden := range []string{"apiKey", "providerId", "providerName", "tenantId", "userId", "internalMultiplier", "upstream"} {
			if strings.Contains(body, `"`+forbidden+`"`) {
				t.Fatalf("%s leaked forbidden field %s: %s", route, forbidden, body)
			}
		}
	}
}

func TestPublicGuestExperienceEventsAreWhitelistedAndSanitized(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	handler := newWithStore(config.Config{Addr: ":0", StaticDir: t.TempDir()}, store).Handler
	res := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"eventType":"guest_click_generate","moduleId":"userAiImage","metadata":{"action":"generate_image","platform":"web","prompt":"private prompt","token":"secret","mobile":"13800000000"}}`)
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v1/public/experience-events", body))
	if res.Code != http.StatusNoContent {
		t.Fatalf("guest experience status = %d, body = %s", res.Code, res.Body.String())
	}
	events, err := store.ListAdminExperienceEvents(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("experience event count = %d, want 1", len(events))
	}
	event := events[0]
	if event.EventType != "GUEST_CLICK_GENERATE" || event.ActorRole != "GUEST" || event.ModuleID != "userAiImage" {
		t.Fatalf("unexpected guest experience event: %+v", event)
	}
	if event.Metadata["action"] != "generate_image" || event.Metadata["platform"] != "web" {
		t.Fatalf("safe metadata was not retained: %#v", event.Metadata)
	}
	for _, forbidden := range []string{"prompt", "token", "mobile"} {
		if _, exists := event.Metadata[forbidden]; exists {
			t.Fatalf("guest experience event retained forbidden metadata %s: %#v", forbidden, event.Metadata)
		}
	}

	rejected := httptest.NewRecorder()
	handler.ServeHTTP(rejected, httptest.NewRequest(http.MethodPost, "/api/v1/public/experience-events", bytes.NewBufferString(`{"eventType":"arbitrary_event"}`)))
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("invalid guest experience status = %d, want %d", rejected.Code, http.StatusBadRequest)
	}
}

func TestGenerationTaskLifecycle(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	store := newJSONStore(dataPath)
	grantPermanentTestPoints(t, store, "user_000002", 100)
	server := newWithStore(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	}, store)
	handler := server.Handler

	assertStatus(t, handler, http.MethodGet, "/api/v1/health", nil, http.StatusOK)
	assertStatus(t, handler, http.MethodGet, "/api/v1/models", nil, http.StatusOK)
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

	createBody := bytes.NewBufferString(`{"type":"TEXT_TO_IMAGE","prompt":"画一只小猫","model":"mock-standard","params":{"count":1}}`)
	createRes := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks", createBody, token)
	if createRes.Code != http.StatusOK {
		t.Fatalf("create task status = %d, body = %s", createRes.Code, createRes.Body.String())
	}
	var task generationTask
	if err := json.NewDecoder(createRes.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	if task.Status != "SUCCEEDED" || len(task.ResultIDs) != 1 {
		t.Fatalf("unexpected task: %+v", task)
	}

	assertAuthedStatus(t, handler, http.MethodGet, "/api/v1/generation-tasks", nil, token, http.StatusOK)
	assetsRes := authedRequest(t, handler, http.MethodGet, "/api/v1/assets", nil, token)
	if assetsRes.Code != http.StatusOK {
		t.Fatalf("list assets status = %d, body = %s", assetsRes.Code, assetsRes.Body.String())
	}
	var assets []asset
	if err := json.NewDecoder(assetsRes.Body).Decode(&assets); err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 {
		t.Fatalf("assets length = %d, want 1", len(assets))
	}
	if strings.Contains(assets[0].URL, "picsum.photos") {
		t.Fatalf("asset URL still uses random placeholder: %s", assets[0].URL)
	}
	const prefix = "data:image/svg+xml;base64,"
	if !strings.HasPrefix(assets[0].URL, prefix) {
		t.Fatalf("asset URL = %q, want SVG data URL", assets[0].URL)
	}
	if assets[0].ThumbnailURL == "" {
		t.Fatalf("asset thumbnail URL is empty")
	}
	rawSVG, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(assets[0].URL, prefix))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawSVG), `id="cat-subject"`) {
		t.Fatalf("cat prompt did not render cat SVG: %s", string(rawSVG))
	}
	assertAuthedStatus(t, handler, http.MethodDelete, "/api/v1/assets/"+task.ResultIDs[0], nil, token, http.StatusOK)
}

func TestUserContentPagedResponses(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	store := newJSONStore(dataPath)
	grantPermanentTestPoints(t, store, "user_000002", 100)
	server := newWithStore(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()}, store)
	handler := server.Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

	for index := 1; index <= 3; index++ {
		body := bytes.NewBufferString(fmt.Sprintf(`{"type":"TEXT_TO_IMAGE","prompt":"paged work %d","model":"mock-standard","params":{"count":1}}`, index))
		response := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks", body, token)
		if response.Code != http.StatusOK {
			t.Fatalf("create paged task %d status = %d, body = %s", index, response.Code, response.Body.String())
		}
	}

	assetResponse := authedRequest(t, handler, http.MethodGet, "/api/v1/assets?paged=true&limit=2&offset=0", nil, token)
	if assetResponse.Code != http.StatusOK {
		t.Fatalf("paged assets status = %d, body = %s", assetResponse.Code, assetResponse.Body.String())
	}
	var assetPage struct {
		Items   []asset `json:"items"`
		Total   int     `json:"total"`
		HasMore bool    `json:"hasMore"`
	}
	if err := json.NewDecoder(assetResponse.Body).Decode(&assetPage); err != nil {
		t.Fatal(err)
	}
	if len(assetPage.Items) != 2 || assetPage.Total < 3 || !assetPage.HasMore {
		t.Fatalf("unexpected asset page: %+v", assetPage)
	}
	if assetPage.Items[0].CreatedAt < assetPage.Items[1].CreatedAt {
		t.Fatalf("assets are not newest-first: %+v", assetPage.Items)
	}

	taskResponse := authedRequest(t, handler, http.MethodGet, "/api/v1/generation-tasks?paged=true&limit=2&offset=0&priority=active", nil, token)
	if taskResponse.Code != http.StatusOK {
		t.Fatalf("paged tasks status = %d, body = %s", taskResponse.Code, taskResponse.Body.String())
	}
	var taskPage struct {
		Items   []generationTask `json:"items"`
		Total   int              `json:"total"`
		HasMore bool             `json:"hasMore"`
	}
	if err := json.NewDecoder(taskResponse.Body).Decode(&taskPage); err != nil {
		t.Fatal(err)
	}
	if len(taskPage.Items) != 2 || taskPage.Total < 3 || !taskPage.HasMore {
		t.Fatalf("unexpected task page: %+v", taskPage)
	}
}

func TestGenerationTaskPrioritySort(t *testing.T) {
	tasks := []generationTask{
		{ID: "completed-new", Status: "SUCCEEDED", CreatedAt: "2026-07-13T12:00:00Z"},
		{ID: "failed-old", Status: "FAILED", CreatedAt: "2026-07-12T12:00:00Z"},
		{ID: "queued-new", Status: "QUEUED", CreatedAt: "2026-07-13T11:00:00Z"},
		{ID: "completed-old", Status: "COMPLETED", CreatedAt: "2026-07-11T12:00:00Z"},
	}
	sortGenerationTasksForUserList(tasks, true)
	want := []string{"queued-new", "failed-old", "completed-new", "completed-old"}
	for index, id := range want {
		if tasks[index].ID != id {
			t.Fatalf("task order[%d] = %s, want %s; tasks=%+v", index, tasks[index].ID, id, tasks)
		}
	}
}

func TestVideoGenerationReturnsPendingAndCompletesAsync(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	store := newJSONStore(dataPath)
	grantPermanentTestPoints(t, store, "user_000002", 100)
	server := newWithStore(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	}, store)
	handler := server.Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

	createRes := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks", bytes.NewBufferString(`{
		"module_code":"video_generation",
		"type":"TEXT_TO_VIDEO",
		"prompt":"async mock video",
		"model":"mock-video",
		"params":{"duration":5,"ratio":"16:9","resolution":"720p"}
	}`), token)
	if createRes.Code != http.StatusOK {
		t.Fatalf("create video task status = %d, body = %s", createRes.Code, createRes.Body.String())
	}
	var task generationTask
	if err := json.NewDecoder(createRes.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	if task.Status != "PROCESSING" || len(task.ResultIDs) != 0 {
		t.Fatalf("video task should return pending before provider completion: %+v", task)
	}

	var completed generationTask
	for i := 0; i < 20; i++ {
		getRes := authedRequest(t, handler, http.MethodGet, "/api/v1/generation-tasks/"+task.ID, nil, token)
		if getRes.Code != http.StatusOK {
			t.Fatalf("get video task status = %d, body = %s", getRes.Code, getRes.Body.String())
		}
		if err := json.NewDecoder(getRes.Body).Decode(&completed); err != nil {
			t.Fatal(err)
		}
		if completed.Status == "SUCCEEDED" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if completed.Status != "SUCCEEDED" || len(completed.ResultIDs) != 1 {
		t.Fatalf("video task did not complete with one result: %+v", completed)
	}

	assetsRes := authedRequest(t, handler, http.MethodGet, "/api/v1/assets", nil, token)
	if assetsRes.Code != http.StatusOK {
		t.Fatalf("list assets status = %d, body = %s", assetsRes.Code, assetsRes.Body.String())
	}
	if !strings.Contains(assetsRes.Body.String(), `"mediaType":"video"`) || !strings.Contains(assetsRes.Body.String(), `/admin/static/mock-video.mp4`) {
		t.Fatalf("video asset was not persisted: %s", assetsRes.Body.String())
	}
}

func TestOfficeCLIStatusRequiresAuthAndReturnsGuidance(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler

	assertStatus(t, handler, http.MethodGet, "/api/v1/officecli/status", nil, http.StatusUnauthorized)
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

	res := authedRequest(t, handler, http.MethodGet, "/api/v1/officecli/status", nil, token)
	if res.Code != http.StatusOK {
		t.Fatalf("officecli status = %d, body = %s", res.Code, res.Body.String())
	}
	var status officeCLIStatusResponse
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.RunnerMode != "server-side-binary" {
		t.Fatalf("runner mode = %q", status.RunnerMode)
	}
	if len(status.InstallCommands) == 0 || len(status.MCPCommands) == 0 || len(status.Capabilities) == 0 {
		t.Fatalf("officecli guidance is incomplete: %+v", status)
	}
	if !strings.Contains(strings.Join(status.Formats, ","), "pptx") {
		t.Fatalf("officecli formats missing pptx: %+v", status.Formats)
	}

	assertStatus(t, handler, http.MethodPost, "/api/v1/officecli/documents", bytes.NewBufferString(`{"format":"docx","prompt":"demo"}`), http.StatusUnauthorized)
	invalid := authedRequest(t, handler, http.MethodPost, "/api/v1/officecli/documents", bytes.NewBufferString(`{"format":"pdf","prompt":"demo"}`), token)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid officecli format status = %d, body = %s", invalid.Code, invalid.Body.String())
	}
}

func TestGenerationErrorMessageExtractsProviderMessage(t *testing.T) {
	raw := `CMECloud Seedance bridge failed: exit status 1: response: {"error":{"message":"当前账号处未订购seedance2.0模型资费包，或资费包已到期，请先订购后才能使用","type":"invalid_authentication_error"}}`
	want := "当前账号处未订购seedance2.0模型资费包，或资费包已到期，请先订购后才能使用"
	if got := generationErrorMessage(errors.New(raw)); got != want {
		t.Fatalf("generationErrorMessage() = %q, want %q", got, want)
	}
}

func TestImageProviderRateLimitTriggersFallbackMessage(t *testing.T) {
	err := errors.New("image provider returned 429: Upstream rate limit exceeded")
	if !shouldFallbackImageGeneration(err) {
		t.Fatal("HTTP 429 image provider errors should trigger fallback")
	}
	want := "图像上游频率或额度受限，已尝试备用通道，请稍后重试或更换上游 API Key"
	if got := generationErrorMessage(err); got != want {
		t.Fatalf("generationErrorMessage() = %q, want %q", got, want)
	}

	permissionErr := errors.New("image provider returned 403: 无权访问 生图备用 分组")
	if !shouldFallbackImageGeneration(permissionErr) {
		t.Fatal("HTTP 403 upstream permission errors should trigger fallback")
	}
	wantPermission := "图像上游权限或分组不可用，已尝试备用通道，请检查上游 API Key、分组和模型权限"
	if got := generationErrorMessage(permissionErr); got != wantPermission {
		t.Fatalf("generationErrorMessage() = %q, want %q", got, wantPermission)
	}

	networkErr := errors.New(`Post "http://localhost:8001/v1/images/generations": dial tcp [::1]:8001: connect: connection refused`)
	if !shouldFallbackImageGeneration(networkErr) {
		t.Fatal("connection refused errors should trigger fallback")
	}
	wantNetwork := "图像上游网络不可达，已尝试备用通道，请检查上游地址或本地代理服务"
	if got := generationErrorMessage(networkErr); got != wantNetwork {
		t.Fatalf("generationErrorMessage() = %q, want %q", got, wantNetwork)
	}
}

func TestGenerationErrorMessagePreservesPrimaryRateLimitAfterFallbackFailure(t *testing.T) {
	primaryErr := errors.New(`image provider returned 429: {"error":{"message":"Upstream rate limit exceeded, please retry later"}}`)
	fallbackErr := errors.New(`fallback image provider failed: image provider returned 403: {"error":{"message":"permission denied"}}`)
	combinedErr := errors.Join(primaryErr, fallbackErr)

	want := generationErrorMessage(primaryErr)
	if got := generationErrorMessage(combinedErr); got != want {
		t.Fatalf("generationErrorMessage() = %q, want primary error message %q", got, want)
	}
}

func TestImageEditLineageParametersAreAllowedInternalParameters(t *testing.T) {
	for _, key := range []string{"sourceReferenceAssetId", "sourceReferenceTaskId"} {
		if !allowedGenerationInternalParam(key) {
			t.Fatalf("%s should be allowed for connector image-edit lineage", key)
		}
	}
}

func TestAICapabilitySchemaValidationAndOverview(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	testStore := newJSONStore(dataPath)
	grantPermanentTestPoints(t, testStore, "user_000002", 100)
	server := newWithStore(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	}, testStore)
	handler := server.Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	schemaRes := authedRequest(t, handler, http.MethodGet, "/api/v1/module-schema?module_code=image_generation", nil, token)
	if schemaRes.Code != http.StatusOK {
		t.Fatalf("module schema status = %d, body = %s", schemaRes.Code, schemaRes.Body.String())
	}
	var schemaPayload struct {
		ModuleCode string `json:"module_code"`
		ModelName  string `json:"model_name"`
		Fields     []struct {
			Key string `json:"key"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(schemaRes.Body).Decode(&schemaPayload); err != nil {
		t.Fatal(err)
	}
	fieldKeys := map[string]bool{}
	for _, field := range schemaPayload.Fields {
		fieldKeys[field.Key] = true
	}
	if schemaPayload.ModuleCode != moduleImageGeneration || schemaPayload.ModelName != "mock-standard" || !fieldKeys["prompt"] || fieldKeys["n"] || fieldKeys["duration"] {
		t.Fatalf("unexpected image schema payload: %+v", schemaPayload)
	}

	setTestPlan := func(planID string) error {
		return testStore.updateAdmin(func(data *adminPlatformData) error {
			for i := range data.Users {
				if data.Users[i].ID == "user_000002" {
					data.Users[i].PlanID = planID
					return nil
				}
			}
			return errors.New("test user not found")
		})
	}
	if err := setTestPlan("plan_free"); err != nil {
		t.Fatalf("set demo plan: %v", err)
	}
	gptSchemaRes := authedRequest(t, handler, http.MethodGet, "/api/v1/module-schema?module_code=image_generation&model_name=gpt-image-2", nil, token)
	if gptSchemaRes.Code != http.StatusBadRequest || !strings.Contains(gptSchemaRes.Body.String(), "not allowed") {
		t.Fatalf("disallowed model schema status = %d, body = %s", gptSchemaRes.Code, gptSchemaRes.Body.String())
	}
	if err := setTestPlan("plan_month"); err != nil {
		t.Fatalf("restore demo plan: %v", err)
	}

	invalid := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks", bytes.NewBufferString(`{"module_code":"image_generation","prompt":"image prompt","model":"mock-standard","params":{"duration":5}}`), token)
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "duration") {
		t.Fatalf("cross-module param was not rejected: %d %s", invalid.Code, invalid.Body.String())
	}

	legacyAssetMetadata := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks", bytes.NewBufferString(`{
		"module_code":"image_generation",
		"prompt":"edit an existing asset",
		"model":"mock-standard",
		"params":{
			"index":0,
			"providerRevisedPrompt":"legacy provider output",
			"provider_revised_prompt":"legacy provider output",
			"referenceCount":1,
			"contentType":"image/png",
			"providerTaskId":"provider-task",
			"thumbnailUrl":"https://example.test/thumb.png",
			"storageObjectKey":"tenant/asset.png",
			"ai_generated":true,
			"output_audit_status":"approved",
			"sourceReferenceAssetId":"asset_legacy"
		}
	}`), token)
	if legacyAssetMetadata.Code != http.StatusOK {
		t.Fatalf("legacy asset metadata was rejected: %d %s", legacyAssetMetadata.Code, legacyAssetMetadata.Body.String())
	}
	var legacyAssetTask generationTask
	if err := json.NewDecoder(legacyAssetMetadata.Body).Decode(&legacyAssetTask); err != nil {
		t.Fatal(err)
	}
	if _, exists := legacyAssetTask.Params["index"]; exists {
		t.Fatalf("legacy asset index leaked into generation params: %+v", legacyAssetTask.Params)
	}
	for _, key := range []string{
		"providerRevisedPrompt", "provider_revised_prompt", "referenceCount", "contentType",
		"providerTaskId", "thumbnailUrl", "storageObjectKey", "ai_generated", "output_audit_status",
	} {
		if _, exists := legacyAssetTask.Params[key]; exists {
			t.Fatalf("legacy provider metadata %s leaked into generation params: %+v", key, legacyAssetTask.Params)
		}
	}

	internalMeta := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks", bytes.NewBufferString(`{
		"module_code":"image_generation",
		"prompt":"image prompt with internal metadata",
		"model":"mock-standard",
		"params":{
			"imageRatio":"4:3",
			"sourceModule":"ai-image",
			"apiMode":"responses",
			"taskSnapshot":{"prompt":"image prompt with internal metadata","inputImageIds":["ref_1"]},
			"inputImageIds":["ref_1"],
			"inputImagesSnapshot":[{"id":"ref_1","name":"input.png","url":"data:image/png;base64,aGVsbG8="}],
			"referenceImageCount":1,
			"referenceImageNames":["input.png"],
			"referenceImageOrder":[{"id":"ref_1","order":1}],
			"userPrompt":"image prompt with internal metadata",
			"effectivePrompt":"image prompt with internal metadata"
		}
	}`), token)
	if internalMeta.Code != http.StatusOK {
		t.Fatalf("internal image metadata was rejected: %d %s", internalMeta.Code, internalMeta.Body.String())
	}

	videoReference := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks", bytes.NewBufferString(`{
		"module_code":"video_generation",
		"type":"IMAGE_TO_VIDEO",
		"prompt":"video prompt with reference image",
		"model":"mock-video",
		"params":{
			"duration":5,
			"ratio":"9:16",
			"resolution":"720p",
			"generate_audio":true,
			"generateAudio":true,
			"image_url":"data:image/png;base64,aGVsbG8=",
			"image_urls":["data:image/png;base64,aGVsbG8="],
			"referenceImages":[{"name":"input.png","url":"data:image/png;base64,aGVsbG8="}]
		}
	}`), token)
	if videoReference.Code != http.StatusOK {
		t.Fatalf("video reference image params were rejected: %d %s", videoReference.Code, videoReference.Body.String())
	}
	var videoTask generationTask
	if err := json.NewDecoder(videoReference.Body).Decode(&videoTask); err != nil {
		t.Fatal(err)
	}
	if videoTask.Params["aspect_ratio"] != "9:16" {
		t.Fatalf("video task snapshot aspect_ratio = %#v", videoTask.Params["aspect_ratio"])
	}
	for _, legacyKey := range []string{"ratio", "generateAudio"} {
		if _, exists := videoTask.Params[legacyKey]; exists {
			t.Fatalf("legacy parameter %s leaked into video task snapshot: %+v", legacyKey, videoTask.Params)
		}
	}
	if videoTask.Params["generate_audio"] != true || fmt.Sprint(videoTask.Params["duration"]) != "5" || videoTask.Params["resolution"] != "720p" {
		t.Fatalf("video task snapshot parameters changed: %+v", videoTask.Params)
	}

	create := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks", bytes.NewBufferString(`{"module_code":"image_generation","prompt":"image prompt","model":"mock-standard","params":{"size":"1920x1080"}}`), token)
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}
	var task generationTask
	if err := json.NewDecoder(create.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	if task.ModuleCode != moduleImageGeneration || task.BillingType != "per_image" || task.PointCost != 1 || len(task.ResultIDs) != 1 || task.FinalSchemaSnapshot == nil || task.LimitSnapshot == nil {
		t.Fatalf("task missing ai capability snapshot: %+v", task)
	}

	overview := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/ai/overview", nil, adminToken)
	body := overview.Body.String()
	if overview.Code != http.StatusOK || !strings.Contains(body, `"modules"`) || !strings.Contains(body, `"billingRules"`) || !strings.Contains(body, task.ID) {
		t.Fatalf("admin ai overview missing capability data: %d %s", overview.Code, body)
	}
}

func TestNormalizeAICapabilityDefaultsMergesNewSchemaFields(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	defaults := defaultAIParameterSchemas(now)
	videoSchema := defaults[1]
	trimmedFields := make([]adminAIParameterField, 0, len(videoSchema.SchemaJSON.Fields))
	for _, field := range videoSchema.SchemaJSON.Fields {
		if field.Key != "generate_audio" {
			trimmedFields = append(trimmedFields, field)
		}
	}
	videoSchema.SchemaJSON.Fields = trimmedFields
	data := normalizeAICapabilityDefaults(adminPlatformData{
		AIModules:          defaultAIModules(now),
		AIModels:           defaultAIModels(now),
		AIParameterSchemas: []adminAIParameterSchema{videoSchema},
		TenantModuleLimits: defaultTenantModuleLimits(now),
		BillingRules:       defaultBillingRules(now),
	})
	schema := findAIParameterSchema(data.AIParameterSchemas, moduleVideoGeneration, "mock-video")
	for _, field := range schema.SchemaJSON.Fields {
		if field.Key == "generate_audio" {
			return
		}
	}
	t.Fatalf("generate_audio was not merged into video schema: %+v", schema.SchemaJSON.Fields)
}

func TestNormalizeAICapabilityDefaultsMergesMissingBillingRules(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	legacyRules := []adminBillingRule{}
	for _, rule := range defaultBillingRules(now) {
		if rule.ModelName != "doubao-seedance-2.0" && rule.ModelName != "grok-video-image" {
			legacyRules = append(legacyRules, rule)
		}
	}
	data := normalizeAICapabilityDefaults(adminPlatformData{
		AIModules:          defaultAIModules(now),
		AIModels:           defaultAIModels(now),
		AIParameterSchemas: defaultAIParameterSchemas(now),
		TenantModuleLimits: defaultTenantModuleLimits(now),
		BillingRules:       legacyRules,
	})
	rule := selectBillingRule(data.BillingRules, moduleVideoGeneration, "doubao-seedance-2.0")
	if rule.ID != "billing_rule_video_doubao_seedance" {
		t.Fatalf("doubao billing rule was not merged: %+v", rule)
	}
	grokRule := selectBillingRule(data.BillingRules, moduleVideoGeneration, "grok-video-image")
	if grokRule.ID != "billing_rule_video_grok_image" {
		t.Fatalf("grok video billing rule was not merged: %+v", grokRule)
	}
	cost := generationPointCostForRequest(createGenerationTaskRequest{
		ModuleCode: moduleVideoGeneration,
		Type:       "IMAGE_TO_VIDEO",
		Model:      "doubao-seedance-2.0",
		Params: map[string]any{
			"duration":   float64(15),
			"resolution": "1080p",
		},
	}, data)
	if cost != 2400 {
		t.Fatalf("doubao point cost = %d, want 2400", cost)
	}
	grokCost := generationPointCostForRequest(createGenerationTaskRequest{
		ModuleCode: moduleVideoGeneration,
		Type:       "IMAGE_TO_VIDEO",
		Model:      "grok-video-image",
		Params: map[string]any{
			"duration":   float64(5),
			"resolution": "720p",
		},
	}, data)
	if grokCost != 6 {
		t.Fatalf("grok video point cost = %d, want 6", grokCost)
	}
	grokHDCost := generationPointCostForRequest(createGenerationTaskRequest{
		ModuleCode: moduleVideoGeneration,
		Type:       "IMAGE_TO_VIDEO",
		Model:      "grok-video-image",
		Params: map[string]any{
			"duration":   float64(15),
			"resolution": "1080p",
		},
	}, data)
	if grokHDCost != 30 {
		t.Fatalf("grok video 1080p point cost = %d, want 30", grokHDCost)
	}
}

func TestWebRoutesSeparatePublicUserAndProtectedAdminBundles(t *testing.T) {
	userStaticDir := t.TempDir()
	adminStaticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(userStaticDir, "index.html"), []byte("USER_BUNDLE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(userStaticDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userStaticDir, "assets", "login.js"), []byte("USER_ASSET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(adminStaticDir, "index.html"), []byte("ADMIN_BUNDLE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(userStaticDir, "static", "js"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userStaticDir, "static", "js", "smart-canvas.js"), []byte("SMART_CANVAS_BUNDLE"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := New(config.Config{
		Addr:           ":0",
		DataPath:       filepath.Join(t.TempDir(), "store.json"),
		StaticDir:      userStaticDir,
		AdminStaticDir: adminStaticDir,
	})

	desktopRoot := request(t, server.Handler, http.MethodGet, "/", nil)
	if desktopRoot.Code != http.StatusOK || strings.TrimSpace(desktopRoot.Body.String()) != "ADMIN_BUNDLE" {
		t.Fatalf("desktop / status = %d, body = %q, want ADMIN_BUNDLE", desktopRoot.Code, desktopRoot.Body.String())
	}

	mobileRoot := mobileRequest(t, server.Handler, http.MethodGet, "/")
	if mobileRoot.Code != http.StatusOK || strings.TrimSpace(mobileRoot.Body.String()) != "ADMIN_BUNDLE" {
		t.Fatalf("mobile / status = %d, body = %q, want ADMIN_BUNDLE", mobileRoot.Code, mobileRoot.Body.String())
	}

	for _, path := range []string{"/foo", "/app", "/app/workspace", "/workspace", "/workspace/video-generation"} {
		t.Run("desktop-redirect-"+path, func(t *testing.T) {
			res := request(t, server.Handler, http.MethodGet, path, nil)
			if res.Code != http.StatusTemporaryRedirect || res.Header().Get("Location") != "/" {
				t.Fatalf("status = %d, location = %q, want 307 /", res.Code, res.Header().Get("Location"))
			}
		})
	}
	for _, path := range []string{"/login", "/register"} {
		res := request(t, server.Handler, http.MethodGet, path, nil)
		if res.Code != http.StatusOK || strings.TrimSpace(res.Body.String()) != "ADMIN_BUNDLE" {
			t.Fatalf("auth page %s status = %d, body = %q, want ADMIN_BUNDLE", path, res.Code, res.Body.String())
		}
	}

	for _, path := range []string{"/mobile", "/mobile/workspace"} {
		t.Run("removed-"+path, func(t *testing.T) {
			res := request(t, server.Handler, http.MethodGet, path, nil)
			if res.Code != http.StatusNotFound {
				t.Fatalf("status = %d, body = %s, want 404", res.Code, res.Body.String())
			}
		})
	}

	for _, path := range []string{"/agent", "/agent/login", "/admin/", "/admin/login"} {
		t.Run(path, func(t *testing.T) {
			res := request(t, server.Handler, http.MethodGet, path, nil)
			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
			}
			if got := strings.TrimSpace(res.Body.String()); got != "ADMIN_BUNDLE" {
				t.Fatalf("body = %q, want ADMIN_BUNDLE", got)
			}
		})
	}

	missingAPIRes := request(t, server.Handler, http.MethodGet, "/api/v1/missing-route", nil)
	if missingAPIRes.Code != http.StatusNotFound {
		t.Fatalf("missing API status = %d, body = %s", missingAPIRes.Code, missingAPIRes.Body.String())
	}
	if contentType := missingAPIRes.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("missing API content type = %q, want application/json", contentType)
	}

	assetRes := request(t, server.Handler, http.MethodGet, "/assets/login.js", nil)
	if assetRes.Code != http.StatusOK {
		t.Fatalf("/assets/login.js status = %d, body = %s", assetRes.Code, assetRes.Body.String())
	}
	if got := strings.TrimSpace(assetRes.Body.String()); got != "USER_ASSET" {
		t.Fatalf("/assets/login.js body = %q, want USER_ASSET", got)
	}

	canvasBundleRes := request(t, server.Handler, http.MethodGet, "/static/js/smart-canvas.js", nil)
	if canvasBundleRes.Code != http.StatusOK {
		t.Fatalf("/static/js/smart-canvas.js status = %d, body = %s", canvasBundleRes.Code, canvasBundleRes.Body.String())
	}
	if got := strings.TrimSpace(canvasBundleRes.Body.String()); got != "SMART_CANVAS_BUNDLE" {
		t.Fatalf("/static/js/smart-canvas.js body = %q, want SMART_CANVAS_BUNDLE", got)
	}
	if cacheControl := canvasBundleRes.Header().Get("Cache-Control"); !strings.Contains(cacheControl, "public") {
		t.Fatalf("/static/js/smart-canvas.js Cache-Control = %q, want public cache", cacheControl)
	}
	gzipReq := httptest.NewRequest(http.MethodGet, "/static/js/smart-canvas.js", nil)
	gzipReq.Header.Set("Accept-Encoding", "gzip")
	gzipRes := httptest.NewRecorder()
	server.Handler.ServeHTTP(gzipRes, gzipReq)
	if got := gzipRes.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	reader, err := gzip.NewReader(gzipRes.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var uncompressed bytes.Buffer
	if _, err := uncompressed.ReadFrom(reader); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(uncompressed.String()); got != "SMART_CANVAS_BUNDLE" {
		t.Fatalf("gzip body = %q, want SMART_CANVAS_BUNDLE", got)
	}

	res := request(t, server.Handler, http.MethodGet, "/user", nil)
	if res.Code != http.StatusNotFound {
		t.Fatalf("/user status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestRequestIDHeaderIsMirroredOrGenerated(t *testing.T) {
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  filepath.Join(t.TempDir(), "store.json"),
		StaticDir: t.TempDir(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set(requestIDHeader, "client-trace-123")
	res := httptest.NewRecorder()
	server.Handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", res.Code, res.Body.String())
	}
	if got := res.Header().Get(requestIDHeader); got != "client-trace-123" {
		t.Fatalf("%s = %q, want client-trace-123", requestIDHeader, got)
	}
	if expose := res.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(expose, requestIDHeader) {
		t.Fatalf("Access-Control-Expose-Headers = %q, want %s", expose, requestIDHeader)
	}

	generatedReq := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	generatedRes := httptest.NewRecorder()
	server.Handler.ServeHTTP(generatedRes, generatedReq)
	if got := generatedRes.Header().Get(requestIDHeader); !strings.HasPrefix(got, "req_") {
		t.Fatalf("generated %s = %q, want req_ prefix", requestIDHeader, got)
	}
}

func TestCORSMiddlewareHandlesConfiguredOrigins(t *testing.T) {
	server := New(config.Config{
		Addr:               ":0",
		DataPath:           filepath.Join(t.TempDir(), "store.json"),
		StaticDir:          t.TempDir(),
		CORSAllowedOrigins: "https://app.example.com, https://desktop.example.com",
	})

	preflight := httptest.NewRequest(http.MethodOptions, "/api/v1/health", nil)
	preflight.Header.Set("Origin", "https://desktop.example.com")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodPost)
	preflight.Header.Set("Access-Control-Request-Headers", "Authorization, X-Request-Id, X-Client-Platform")
	preflightRes := httptest.NewRecorder()
	server.Handler.ServeHTTP(preflightRes, preflight)
	if preflightRes.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, body = %s", preflightRes.Code, preflightRes.Body.String())
	}
	if got := preflightRes.Header().Get("Access-Control-Allow-Origin"); got != "https://desktop.example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
	if got := preflightRes.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Access-Control-Allow-Credentials = %q", got)
	}
	if got := preflightRes.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Fatalf("Vary = %q, want Origin", got)
	}
	allowHeaders := preflightRes.Header().Get("Access-Control-Allow-Headers")
	for _, want := range []string{requestIDHeader, "X-Client-Platform", "X-Client-Version"} {
		if !strings.Contains(allowHeaders, want) {
			t.Fatalf("Access-Control-Allow-Headers missing %q: %q", want, allowHeaders)
		}
	}
	allowMethods := preflightRes.Header().Get("Access-Control-Allow-Methods")
	for _, want := range []string{http.MethodPost, http.MethodOptions} {
		if !strings.Contains(allowMethods, want) {
			t.Fatalf("Access-Control-Allow-Methods missing %q: %q", want, allowMethods)
		}
	}
	if expose := preflightRes.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(expose, requestIDHeader) {
		t.Fatalf("Access-Control-Expose-Headers = %q, want %s", expose, requestIDHeader)
	}

	blocked := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	blocked.Header.Set("Origin", "https://evil.example.com")
	blockedRes := httptest.NewRecorder()
	server.Handler.ServeHTTP(blockedRes, blocked)
	if blockedRes.Code != http.StatusOK {
		t.Fatalf("blocked-origin health status = %d, body = %s", blockedRes.Code, blockedRes.Body.String())
	}
	if got := blockedRes.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unconfigured origin should not receive CORS allow-origin, got %q", got)
	}
}

func TestGenerationAssetNameUsesTaskType(t *testing.T) {
	got := generationAssetName("IMAGE_TO_IMAGE", "task_000123", 0)
	if got != "IMAGE_TO_IMAGE-task_000123-01" {
		t.Fatalf("asset name = %q, want IMAGE_TO_IMAGE-task_000123-01", got)
	}
}

func TestUserGenerationAssetPointsAdminLoop(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	pointsBefore := authedRequest(t, handler, http.MethodGet, "/api/v1/points/account", nil, token)
	if pointsBefore.Code != http.StatusOK || !strings.Contains(pointsBefore.Body.String(), `"available":0`) {
		t.Fatalf("initial points response = %d %s", pointsBefore.Code, pointsBefore.Body.String())
	}
	grant := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/customers/user_000002/point-gifts", bytes.NewBufferString(`{"points":10,"reason":"generation closed-loop fixture","idempotencyKey":"generation-closed-loop-gift"}`), adminToken)
	if grant.Code != http.StatusOK {
		t.Fatalf("grant test points status = %d, body = %s", grant.Code, grant.Body.String())
	}

	createBody := bytes.NewBufferString(`{"type":"TEXT_TO_IMAGE","prompt":"闭环测试图片","model":"mock-standard","params":{"count":2}}`)
	createRes := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks", createBody, token)
	if createRes.Code != http.StatusOK {
		t.Fatalf("create task status = %d, body = %s", createRes.Code, createRes.Body.String())
	}
	var task generationTask
	if err := json.NewDecoder(createRes.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	if task.UserID != "user_000002" || task.PointCost != 2 || len(task.ResultIDs) != 2 || task.Status != "SUCCEEDED" {
		t.Fatalf("unexpected task: %+v", task)
	}

	pointsAfter := authedRequest(t, handler, http.MethodGet, "/api/v1/points/account", nil, token)
	if pointsAfter.Code != http.StatusOK || !strings.Contains(pointsAfter.Body.String(), `"available":8`) {
		t.Fatalf("deducted points response = %d %s", pointsAfter.Code, pointsAfter.Body.String())
	}

	assets := authedRequest(t, handler, http.MethodGet, "/api/v1/assets", nil, token)
	if assets.Code != http.StatusOK || !strings.Contains(assets.Body.String(), task.ResultIDs[0]) || !strings.Contains(assets.Body.String(), `"mediaType":"image"`) {
		t.Fatalf("assets not visible after generation: %d %s", assets.Code, assets.Body.String())
	}

	customers := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/customers", nil, adminToken)
	if customers.Code != http.StatusOK || !strings.Contains(customers.Body.String(), `"pointsAvailable":8`) || !strings.Contains(customers.Body.String(), "演示用户") {
		t.Fatalf("admin customers did not reflect deducted points: %d %s", customers.Code, customers.Body.String())
	}

	adminTasks := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/generation-tasks", nil, adminToken)
	adminTaskBody := adminTasks.Body.String()
	if adminTasks.Code != http.StatusOK || !strings.Contains(adminTaskBody, task.ID) || !strings.Contains(adminTaskBody, `"pointCost":2`) || !strings.Contains(adminTaskBody, task.ResultIDs[0]) {
		t.Fatalf("admin generation tasks missing closed-loop data: %d %s", adminTasks.Code, adminTaskBody)
	}

	overview := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/overview", nil, adminToken)
	if overview.Code != http.StatusOK || !strings.Contains(overview.Body.String(), `"generatedAssets":2`) {
		t.Fatalf("admin overview missing generated assets: %d %s", overview.Code, overview.Body.String())
	}

	usage := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/usage", nil, adminToken)
	if usage.Code != http.StatusOK || !strings.Contains(usage.Body.String(), `"apiCalls":1`) || !strings.Contains(usage.Body.String(), `"assets":2`) {
		t.Fatalf("admin usage missing generation counters: %d %s", usage.Code, usage.Body.String())
	}

	billing := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/billing/events", nil, adminToken)
	billingBody := billing.Body.String()
	if billing.Code != http.StatusOK || !strings.Contains(billingBody, task.ID) || !strings.Contains(billingBody, `"balanceBefore":10`) || !strings.Contains(billingBody, `"balanceAfter":8`) {
		t.Fatalf("billing events missing generation task: %d %s", billing.Code, billingBody)
	}

	login := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"agent1@xianzhi.ai","password":"Agent123!"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("agent login status = %d, body = %s", login.Code, login.Body.String())
	}
	var loginBody struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(login.Body).Decode(&loginBody); err != nil {
		t.Fatal(err)
	}
	channelReq := httptest.NewRequest(http.MethodGet, "/api/v1/channel/me", nil)
	channelReq.Header.Set("Authorization", "Bearer "+loginBody.AccessToken)
	channelRes := httptest.NewRecorder()
	handler.ServeHTTP(channelRes, channelReq)
	channelBody := channelRes.Body.String()
	if channelRes.Code != http.StatusOK || !strings.Contains(channelBody, task.ID) || !strings.Contains(channelBody, `"usageEvents"`) {
		t.Fatalf("channel center missing generation usage event: %d %s", channelRes.Code, channelBody)
	}
}

func TestFirstRechargeRequires996AgentPackage(t *testing.T) {
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  filepath.Join(t.TempDir(), "store.json"),
		StaticDir: t.TempDir(),
	})
	handler := server.Handler

	register := request(t, handler, http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"username":"首次充值用户","email":"first-recharge@example.com","password":"First123!","confirmPassword":"First123!"}`))
	if register.Code != http.StatusOK {
		t.Fatalf("register status = %d, body = %s", register.Code, register.Body.String())
	}
	var registerBody struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(register.Body).Decode(&registerBody); err != nil {
		t.Fatal(err)
	}

	blocked := authedRequest(t, handler, http.MethodPost, "/api/v1/points/recharge-orders", bytes.NewBufferString(`{"rechargePackageId":"recharge_standard","amountCents":9900,"paymentMethod":"wechat_mini_program"}`), registerBody.AccessToken)
	if blocked.Code != http.StatusConflict || !strings.Contains(blocked.Body.String(), "996") {
		t.Fatalf("first regular recharge should be blocked: %d %s", blocked.Code, blocked.Body.String())
	}

	join := authedRequest(t, handler, http.MethodPost, "/api/v1/agent/join-order", bytes.NewBufferString(`{"planId":"plan_agent_join_996","paymentMethod":"wechat_mini_program"}`), registerBody.AccessToken)
	if join.Code != http.StatusOK {
		t.Fatalf("996 agent order status = %d, body = %s", join.Code, join.Body.String())
	}
	var joinBody struct {
		Item adminOrder `json:"item"`
	}
	if err := json.NewDecoder(join.Body).Decode(&joinBody); err != nil {
		t.Fatal(err)
	}
	if joinBody.Item.ID == "" || joinBody.Item.PlanID != "plan_agent_join_996" {
		t.Fatalf("unexpected 996 agent order: %+v", joinBody.Item)
	}

	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	paid := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/orders/"+joinBody.Item.ID+"/mark-paid", bytes.NewBuffer(nil), adminToken)
	if paid.Code != http.StatusOK {
		t.Fatalf("mark 996 order paid status = %d, body = %s", paid.Code, paid.Body.String())
	}
	invoices := authedRequest(t, handler, http.MethodGet, "/api/v1/member/invoices", nil, registerBody.AccessToken)
	if invoices.Code != http.StatusOK || !strings.Contains(invoices.Body.String(), joinBody.Item.ID) || !strings.Contains(invoices.Body.String(), `"status":"AVAILABLE"`) {
		t.Fatalf("member invoices status = %d, body = %s", invoices.Code, invoices.Body.String())
	}
	refund := authedRequest(t, handler, http.MethodPost, "/api/v1/member/refund-requests", bytes.NewBufferString(`{"orderId":"`+joinBody.Item.ID+`","reason":"重复购买","remark":"自动化测试申请"}`), registerBody.AccessToken)
	if refund.Code != http.StatusOK || !strings.Contains(refund.Body.String(), `"status":"REFUND_REQUESTED"`) {
		t.Fatalf("member refund request status = %d, body = %s", refund.Code, refund.Body.String())
	}

	allowed := authedRequest(t, handler, http.MethodPost, "/api/v1/points/recharge-orders", bytes.NewBufferString(`{"rechargePackageId":"recharge_standard","amountCents":9900,"paymentMethod":"wechat_mini_program"}`), registerBody.AccessToken)
	if allowed.Code != http.StatusOK {
		t.Fatalf("regular recharge after first paid order status = %d, body = %s", allowed.Code, allowed.Body.String())
	}
}

func TestPPTEstimateUsesBillingRulesWithoutDeductingPoints(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	store := newJSONStore(dataPath)
	grantPermanentTestPoints(t, store, "user_000002", 100)
	server := newWithStore(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()}, store)
	handler := server.Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

	beforeResponse := authedRequest(t, handler, http.MethodGet, "/api/v1/points/account", nil, token)
	if beforeResponse.Code != http.StatusOK {
		t.Fatalf("points before estimate status = %d, body = %s", beforeResponse.Code, beforeResponse.Body.String())
	}
	var before struct {
		Account pointAccount `json:"account"`
	}
	if err := json.NewDecoder(beforeResponse.Body).Decode(&before); err != nil {
		t.Fatal(err)
	}

	estimateResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/ppt/estimate", bytes.NewBufferString(`{"prompt":"门店增长计划","slideCount":3,"textModel":"kimi-k2.6","imageSource":"none"}`), token)
	if estimateResponse.Code != http.StatusOK {
		t.Fatalf("ppt estimate status = %d, body = %s", estimateResponse.Code, estimateResponse.Body.String())
	}
	var estimate struct {
		PointCost       int  `json:"pointCost"`
		SlideCount      int  `json:"slideCount"`
		AvailablePoints int  `json:"availablePoints"`
		Sufficient      bool `json:"sufficient"`
	}
	if err := json.NewDecoder(estimateResponse.Body).Decode(&estimate); err != nil {
		t.Fatal(err)
	}
	if estimate.PointCost <= 0 || estimate.SlideCount != 3 || estimate.AvailablePoints != before.Account.Available || !estimate.Sufficient {
		t.Fatalf("unexpected ppt estimate: %+v", estimate)
	}

	afterResponse := authedRequest(t, handler, http.MethodGet, "/api/v1/points/account", nil, token)
	var after struct {
		Account pointAccount `json:"account"`
	}
	if err := json.NewDecoder(afterResponse.Body).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if after.Account.Available != before.Account.Available {
		t.Fatalf("estimate deducted points: before=%d after=%d", before.Account.Available, after.Account.Available)
	}
}

func TestPPTGenerationCreatesUsageEvent(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	store := newJSONStore(dataPath)
	grantPermanentTestPoints(t, store, "user_000002", 100)
	server := newWithStore(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	}, store)
	handler := server.Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

	pointsBefore := authedRequest(t, handler, http.MethodGet, "/api/v1/points/account", nil, token)
	if pointsBefore.Code != http.StatusOK {
		t.Fatalf("points before status = %d, body = %s", pointsBefore.Code, pointsBefore.Body.String())
	}
	var beforePayload struct {
		Account pointAccount `json:"account"`
	}
	if err := json.NewDecoder(pointsBefore.Body).Decode(&beforePayload); err != nil {
		t.Fatal(err)
	}
	before := beforePayload.Account

	createBody := bytes.NewBufferString(`{
		"prompt":"Diabetes diet education",
		"slideCount":3,
		"language":"zh",
		"tone":"education",
		"theme":"medical",
		"imageSource":"ai",
		"textModel":"kimi-k2.6",
		"outline":{
			"title":"Diabetes diet education",
			"slides":[
				{"page":1,"title":"Cover","summary":"Opening","bulletPoints":["Audience","Goal"]},
				{"page":2,"title":"Plate method","summary":"Meal structure","bulletPoints":["Vegetables","Protein","Staple food"]},
				{"page":3,"title":"Action plan","summary":"Daily follow-up","bulletPoints":["Record","Review"]}
			]
		}
	}`)
	createRes := authedRequest(t, handler, http.MethodPost, "/api/v1/ppt/generate", createBody, token)
	if createRes.Code != http.StatusOK {
		t.Fatalf("create ppt status = %d, body = %s", createRes.Code, createRes.Body.String())
	}
	var createResp struct {
		TaskID string `json:"taskId"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(createRes.Body).Decode(&createResp); err != nil {
		t.Fatal(err)
	}
	if createResp.TaskID == "" {
		t.Fatalf("ppt response missing task ID: %+v", createResp)
	}

	pointsAfter := authedRequest(t, handler, http.MethodGet, "/api/v1/points/account", nil, token)
	if pointsAfter.Code != http.StatusOK {
		t.Fatalf("points after status = %d, body = %s", pointsAfter.Code, pointsAfter.Body.String())
	}
	var afterPayload struct {
		Account pointAccount `json:"account"`
	}
	if err := json.NewDecoder(pointsAfter.Body).Decode(&afterPayload); err != nil {
		t.Fatal(err)
	}
	after := afterPayload.Account
	if after.Available != before.Available-3 {
		t.Fatalf("ppt did not deduct slide points: before=%d after=%d", before.Available, after.Available)
	}

	usageRes := authedRequest(t, handler, http.MethodGet, "/api/v1/user/usage", nil, token)
	if usageRes.Code != http.StatusOK {
		t.Fatalf("usage status = %d, body = %s", usageRes.Code, usageRes.Body.String())
	}
	var usage struct {
		Items []struct {
			TaskID        string `json:"taskId"`
			MetricCode    string `json:"metricCode"`
			Type          string `json:"type"`
			Model         string `json:"model"`
			Quantity      int    `json:"quantity"`
			PointCost     int    `json:"pointCost"`
			BalanceBefore int    `json:"balanceBefore"`
			BalanceAfter  int    `json:"balanceAfter"`
			Status        string `json:"status"`
		} `json:"items"`
	}
	if err := json.NewDecoder(usageRes.Body).Decode(&usage); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range usage.Items {
		if item.TaskID != createResp.TaskID {
			continue
		}
		found = true
		if item.MetricCode != billingMetricPPTGenerate || item.Type != "PPT_GENERATION" || item.Model != "kimi-k2.6" || item.Quantity != 3 || item.PointCost != 3 || item.BalanceBefore != before.Available || item.BalanceAfter != after.Available || item.Status != "SUCCEEDED" {
			t.Fatalf("unexpected ppt usage item: %+v", item)
		}
	}
	if !found {
		t.Fatalf("ppt usage event for %s not found: %+v", createResp.TaskID, usage.Items)
	}

	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	billing := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/billing/events", nil, adminToken)
	if billing.Code != http.StatusOK || !strings.Contains(billing.Body.String(), createResp.TaskID) || !strings.Contains(billing.Body.String(), billingMetricPPTGenerate) {
		t.Fatalf("admin billing events missing ppt usage: %d %s", billing.Code, billing.Body.String())
	}
}

func TestPPTImageGenerationCreatesImageUsageEvent(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	store := newJSONStore(dataPath)
	grantPermanentTestPoints(t, store, "user_000002", 100)
	server := newWithStore(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	}, store)
	handler := server.Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

	pointsBefore := authedRequest(t, handler, http.MethodGet, "/api/v1/points/account", nil, token)
	if pointsBefore.Code != http.StatusOK {
		t.Fatalf("points before status = %d, body = %s", pointsBefore.Code, pointsBefore.Body.String())
	}
	var beforePayload struct {
		Account pointAccount `json:"account"`
	}
	if err := json.NewDecoder(pointsBefore.Body).Decode(&beforePayload); err != nil {
		t.Fatal(err)
	}

	createRes := authedRequest(t, handler, http.MethodPost, "/api/v1/ppt/images/generate", bytes.NewBufferString(`{
		"prompt":"Generate a business slide illustration",
		"deckTitle":"Business growth plan",
		"theme":"business",
		"language":"zh",
		"imageModel":"mock-standard",
		"slide":{"id":"slide_test","page":1,"title":"Growth overview","content":"Market growth and execution plan","bulletPoints":["Market","Execution"]}
	}`), token)
	if createRes.Code != http.StatusOK {
		t.Fatalf("ppt image status = %d, body = %s", createRes.Code, createRes.Body.String())
	}
	var imageResp pptImageSearchResponse
	if err := json.NewDecoder(createRes.Body).Decode(&imageResp); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(imageResp.URL) == "" {
		t.Fatalf("ppt image response missing URL: %+v", imageResp)
	}

	pointsAfter := authedRequest(t, handler, http.MethodGet, "/api/v1/points/account", nil, token)
	if pointsAfter.Code != http.StatusOK {
		t.Fatalf("points after status = %d, body = %s", pointsAfter.Code, pointsAfter.Body.String())
	}
	var afterPayload struct {
		Account pointAccount `json:"account"`
	}
	if err := json.NewDecoder(pointsAfter.Body).Decode(&afterPayload); err != nil {
		t.Fatal(err)
	}
	if afterPayload.Account.Available != beforePayload.Account.Available-1 {
		t.Fatalf("ppt image did not deduct image points: before=%d after=%d", beforePayload.Account.Available, afterPayload.Account.Available)
	}

	usageRes := authedRequest(t, handler, http.MethodGet, "/api/v1/user/usage", nil, token)
	if usageRes.Code != http.StatusOK {
		t.Fatalf("usage status = %d, body = %s", usageRes.Code, usageRes.Body.String())
	}
	usageBody := usageRes.Body.String()
	if !strings.Contains(usageBody, `"metricCode":"image.generations"`) || !strings.Contains(usageBody, `"model":"mock-standard"`) || !strings.Contains(usageBody, `"pointCost":1`) {
		t.Fatalf("usage body missing ppt image billing event: %s", usageBody)
	}
}

func TestPPTOutlineImageSourceControlsDefaultLayouts(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

	cases := []struct {
		name        string
		imageSource string
		wantLayout  string
	}{
		{name: "ai images", imageSource: "ai", wantLayout: "imageText"},
		{name: "no images", imageSource: "none", wantLayout: "content"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := bytes.NewBufferString(`{
				"prompt":"门店增长方案",
				"slideCount":3,
				"language":"zh",
				"tone":"professional",
				"textContent":"concise",
				"audience":"business",
				"scenario":"general",
				"generationAspectRatio":"dynamic",
				"autoThemeEnabled":true,
				"enableWebSearch":false,
				"textModel":"kimi-k2.6",
				"imageSource":"` + tc.imageSource + `",
				"imageModel":"default-image"
			}`)
			res := authedRequest(t, handler, http.MethodPost, "/api/v1/ppt/outline/generate", body, token)
			if res.Code != http.StatusOK {
				t.Fatalf("outline status = %d, body = %s", res.Code, res.Body.String())
			}
			var outline pptOutline
			if err := json.NewDecoder(res.Body).Decode(&outline); err != nil {
				t.Fatal(err)
			}
			if len(outline.Slides) != 3 {
				t.Fatalf("outline slides = %d, want 3", len(outline.Slides))
			}
			if got := outline.Slides[1].Layout; got != tc.wantLayout {
				t.Fatalf("middle slide layout = %q, want %q", got, tc.wantLayout)
			}
		})
	}
}

func TestAgentLoginAndChannelCenter(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	raw := `{
		"users":[
			{"id":"user_000001","email":"admin@xianzhi.ai","name":"平台管理员","role":"SUPER_ADMIN","status":"ACTIVE","planId":"plan_free"},
			{"id":"user_000002","email":"demo@xianzhi.ai","name":"演示用户","role":"MEMBER","status":"ACTIVE","planId":"plan_month"},
			{"id":"user_000003","email":"agent1@xianzhi.ai","name":"华东推广员","role":"AGENT_L1","status":"ACTIVE","planId":"plan_free"},
			{"id":"user_000004","email":"agent2@xianzhi.ai","name":"华东初级代理商","role":"AGENT_L2","status":"ACTIVE","planId":"plan_free"}
		],
		"channelAgents":[
			{"id":"channel_000001","userId":"user_000003","level":1,"status":"ACTIVE","inviteCode":"EAST001"},
			{"id":"channel_000002","userId":"user_000004","parentId":"channel_000001","level":2,"status":"ACTIVE","inviteCode":"EAST002"}
		],
		"commissions":[{"id":"commission_000001","orderId":"order_000001","agentId":"channel_000001","amountCents":990,"rate":0.1,"status":"SETTLED"}],
		"withdrawals":[{"id":"withdrawal_000001","agentId":"channel_000001","amountCents":300,"status":"PENDING"}],
		"counters":{}
	}`
	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	login := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"agent1@xianzhi.ai","password":"Agent123!"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("agent login status = %d, body = %s", login.Code, login.Body.String())
	}
	var loginBody struct {
		AccessToken   string         `json:"accessToken"`
		DefaultModule string         `json:"defaultModule"`
		Workspace     string         `json:"workspace"`
		Permissions   []string       `json:"permissions"`
		User          map[string]any `json:"user"`
		Agent         map[string]any `json:"agent"`
	}
	if err := json.NewDecoder(login.Body).Decode(&loginBody); err != nil {
		t.Fatal(err)
	}
	if loginBody.AccessToken == "" || loginBody.DefaultModule != "dashboard" || loginBody.Workspace != "user" || loginBody.Agent["inviteCode"] != "EAST001" || !stringSliceContains(loginBody.Permissions, "channel.dashboard") {
		t.Fatalf("unexpected login body: %+v", loginBody)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/channel/me", nil)
	req.Header.Set("Authorization", "Bearer "+loginBody.AccessToken)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	body := res.Body.String()
	if res.Code != http.StatusOK || !strings.Contains(body, `"directCustomers":0`) || !strings.Contains(body, `"childAgents":1`) || !strings.Contains(body, `"totalCommission":990`) || strings.Contains(body, "演示用户") || !strings.Contains(body, `"inviteLink":"http://localhost:3100/register?invite=EAST001"`) {
		t.Fatalf("channel center response = %d %s", res.Code, body)
	}

	registerMismatch := request(t, handler, http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"username":"邀请注册用户","email":"bad-invitee@example.com","password":"Invite123!","confirmPassword":"Invite456!","inviteCode":"EAST001"}`))
	if registerMismatch.Code != http.StatusBadRequest || !strings.Contains(registerMismatch.Body.String(), "password confirmation does not match") {
		t.Fatalf("invite register mismatch status = %d, body = %s", registerMismatch.Code, registerMismatch.Body.String())
	}

	register := request(t, handler, http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"username":"邀请注册用户","email":"invitee@example.com","password":"Invite123!","confirmPassword":"Invite123!","inviteCode":"EAST001"}`))
	if register.Code != http.StatusOK || !strings.Contains(register.Body.String(), `"defaultModule":"dashboard"`) {
		t.Fatalf("invite register status = %d, body = %s", register.Code, register.Body.String())
	}
	reqAfterRegister := httptest.NewRequest(http.MethodGet, "/api/v1/channel/me", nil)
	reqAfterRegister.Header.Set("Authorization", "Bearer "+loginBody.AccessToken)
	resAfterRegister := httptest.NewRecorder()
	handler.ServeHTTP(resAfterRegister, reqAfterRegister)
	if resAfterRegister.Code != http.StatusOK || !strings.Contains(resAfterRegister.Body.String(), "邀请注册用户") || !strings.Contains(resAfterRegister.Body.String(), `"directCustomers":1`) {
		t.Fatalf("channel center after register = %d %s", resAfterRegister.Code, resAfterRegister.Body.String())
	}
	adminCustomers := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/customers", nil, adminToken)
	adminCustomerBody := adminCustomers.Body.String()
	if adminCustomers.Code != http.StatusOK || !strings.Contains(adminCustomerBody, "邀请注册用户") || !strings.Contains(adminCustomerBody, `"sourceInviteCode":"EAST001"`) || !strings.Contains(adminCustomerBody, `"sourceAgentName":"华东推广员"`) {
		t.Fatalf("admin customers after invite register = %d %s", adminCustomers.Code, adminCustomerBody)
	}

	memberLogin := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"demo@xianzhi.ai","password":"Demo123!"}`))
	if memberLogin.Code != http.StatusOK {
		t.Fatalf("member login status = %d, body = %s", memberLogin.Code, memberLogin.Body.String())
	}
	var memberBody struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(memberLogin.Body).Decode(&memberBody); err != nil {
		t.Fatal(err)
	}
	memberReq := httptest.NewRequest(http.MethodGet, "/api/v1/channel/me", nil)
	memberReq.Header.Set("Authorization", "Bearer "+memberBody.AccessToken)
	memberRes := httptest.NewRecorder()
	handler.ServeHTTP(memberRes, memberReq)
	if memberRes.Code != http.StatusForbidden {
		t.Fatalf("member channel center status = %d, body = %s", memberRes.Code, memberRes.Body.String())
	}
}

func TestOperationCenterLoginAndScopedAPIs(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	raw := `{
		"users":[
			{"id":"user_member","email":"member@example.com","name":"Member","role":"MEMBER","status":"ACTIVE","planId":"plan_free"},
			{"id":"user_agent","email":"agent@example.com","name":"East Agent","role":"AGENT_L1","status":"ACTIVE","agentStatus":"ACTIVE","planId":"plan_free"},
			{"id":"user_operation","email":"operation@example.com","name":"East Operation Center","role":"OPERATION_CENTER","status":"ACTIVE","operationCenterStatus":"ACTIVE","planId":"plan_free"}
		],
		"operationCenters":[
			{"id":"operation_center_1","userId":"user_operation","name":"East Operation Center","region":"East","inviteCode":"OC-EAST","status":"ACTIVE","createdAt":"2026-07-01T00:00:00Z"}
		],
		"channelAgents":[
			{"id":"channel_1","userId":"user_agent","operationCenterId":"operation_center_1","level":1,"status":"ACTIVE","inviteCode":"EAST001","createdAt":"2026-07-01T00:00:00Z"}
		],
		"orders":[
			{"id":"order_1","userId":"user_member","planId":"plan_agent_join_996","orderType":"AGENT_JOIN","amountCents":99600,"status":"PAID","operationCenterId":"operation_center_1","paidAt":"2026-07-02T00:00:00Z","createdAt":"2026-07-02T00:00:00Z","priceSnapshot":{}}
		],
		"commissions":[
			{"id":"commission_oc_1","orderId":"order_1","receiverType":"OPERATION_CENTER","receiverId":"operation_center_1","amountCents":20000,"commissionType":"OPERATION_CENTER_REWARD","rate":0,"status":"PENDING","settleStatus":"PENDING","ruleSnapshot":{},"createdAt":"2026-07-02T00:00:00Z"}
		],
		"counters":{}
	}`
	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler

	login := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"operation@example.com","password":"Demo123!"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("operation center login status = %d, body = %s", login.Code, login.Body.String())
	}
	var loginBody struct {
		AccessToken     string         `json:"accessToken"`
		DefaultModule   string         `json:"defaultModule"`
		DefaultRoute    string         `json:"defaultRoute"`
		Workspace       string         `json:"workspace"`
		Permissions     []string       `json:"permissions"`
		OperationCenter map[string]any `json:"operationCenter"`
	}
	if err := json.NewDecoder(login.Body).Decode(&loginBody); err != nil {
		t.Fatal(err)
	}
	if loginBody.AccessToken == "" || loginBody.DefaultModule != "operationCenterDashboard" || loginBody.DefaultRoute != "/app/operation-center" || loginBody.Workspace != "user" || loginBody.OperationCenter["inviteCode"] != "OC-EAST" || !stringSliceContains(loginBody.Permissions, "operation_center.dashboard") {
		t.Fatalf("unexpected operation center login body: %+v", loginBody)
	}

	profile := authedRequest(t, handler, http.MethodGet, "/api/v1/operation-center/profile", nil, loginBody.AccessToken)
	if profile.Code != http.StatusOK || !strings.Contains(profile.Body.String(), `"agents":1`) || !strings.Contains(profile.Body.String(), `"orders":1`) || !strings.Contains(profile.Body.String(), `"totalCents":20000`) {
		t.Fatalf("operation center profile = %d %s", profile.Code, profile.Body.String())
	}
	agents := authedRequest(t, handler, http.MethodGet, "/api/v1/operation-center/agents", nil, loginBody.AccessToken)
	if agents.Code != http.StatusOK || !strings.Contains(agents.Body.String(), "EAST001") {
		t.Fatalf("operation center agents = %d %s", agents.Code, agents.Body.String())
	}
	orders := authedRequest(t, handler, http.MethodGet, "/api/v1/operation-center/orders", nil, loginBody.AccessToken)
	if orders.Code != http.StatusOK || !strings.Contains(orders.Body.String(), "order_1") || !strings.Contains(orders.Body.String(), `"operationCenterId":"operation_center_1"`) {
		t.Fatalf("operation center orders = %d %s", orders.Code, orders.Body.String())
	}
	commissions := authedRequest(t, handler, http.MethodGet, "/api/v1/operation-center/commissions", nil, loginBody.AccessToken)
	if commissions.Code != http.StatusOK || !strings.Contains(commissions.Body.String(), "commission_oc_1") || !strings.Contains(commissions.Body.String(), `"pendingCents":20000`) {
		t.Fatalf("operation center commissions = %d %s", commissions.Code, commissions.Body.String())
	}

	memberToken := loginToken(t, handler, "member@example.com", "Demo123!")
	memberAgents := authedRequest(t, handler, http.MethodGet, "/api/v1/operation-center/agents", nil, memberToken)
	if memberAgents.Code != http.StatusForbidden {
		t.Fatalf("member operation center agents status = %d, body = %s", memberAgents.Code, memberAgents.Body.String())
	}
}

func TestWeChatMiniProgramMockLogin(t *testing.T) {
	t.Setenv("WECHAT_MINI_PROGRAM_APPID", "")
	t.Setenv("WECHAT_MINI_PROGRAM_SECRET", "")
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler

	phoneMissing := request(t, handler, http.MethodPost, "/api/v1/auth/wechat/phone-login", bytes.NewBufferString(`{"wxLoginCode":""}`))
	if phoneMissing.Code != http.StatusBadRequest || !strings.Contains(phoneMissing.Body.String(), "wechat mini program code is required") {
		t.Fatalf("anonymous phone login status = %d, body = %s", phoneMissing.Code, phoneMissing.Body.String())
	}
	phoneWithStaleToken := authedRequest(t, handler, http.MethodPost, "/api/v1/auth/wechat/phone-login", bytes.NewBufferString(`{"wxLoginCode":""}`), "stale-login-token")
	if phoneWithStaleToken.Code != http.StatusBadRequest || !strings.Contains(phoneWithStaleToken.Body.String(), "wechat mini program code is required") {
		t.Fatalf("phone login must ignore stale bearer token, status = %d, body = %s", phoneWithStaleToken.Code, phoneWithStaleToken.Body.String())
	}

	missing := request(t, handler, http.MethodPost, "/api/v1/auth/wechat-mini-program/login", bytes.NewBufferString(`{"code":""}`))
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "wechat mini program code is required") {
		t.Fatalf("missing wechat code status = %d, body = %s", missing.Code, missing.Body.String())
	}

	unconfigured := request(t, handler, http.MethodPost, "/api/v1/auth/wechat-mini-program/login", bytes.NewBufferString(`{"code":"real-devtools-code"}`))
	if unconfigured.Code != http.StatusNotImplemented || !strings.Contains(unconfigured.Body.String(), "wechat mini program login is not configured") {
		t.Fatalf("unconfigured wechat login status = %d, body = %s", unconfigured.Code, unconfigured.Body.String())
	}

	t.Setenv("XIANZHI_ENABLE_MOCK_LOGIN", "true")
	login := request(t, handler, http.MethodPost, "/api/v1/auth/wechat-mini-program/login", bytes.NewBufferString(`{"code":"mock-devtools-code"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("mock wechat login status = %d, body = %s", login.Code, login.Body.String())
	}
	var body struct {
		AccessToken   string `json:"accessToken"`
		DefaultModule string `json:"defaultModule"`
		User          struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		} `json:"user"`
	}
	if err := json.NewDecoder(login.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.AccessToken == "" || body.DefaultModule != "dashboard" || body.User.Email != "demo@xianzhi.ai" || body.User.Role != "MEMBER" {
		t.Fatalf("unexpected mock wechat login body: %+v", body)
	}

	me := authedRequest(t, handler, http.MethodGet, "/api/v1/auth/me", nil, body.AccessToken)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"email":"demo@xianzhi.ai"`) {
		t.Fatalf("wechat login token auth/me = %d, body = %s", me.Code, me.Body.String())
	}
}

func TestChannelScopedAPIsFilterCustomersAndMoney(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	raw := `{
		"users":[
			{"id":"user_agent_l1","email":"agent1@xianzhi.ai","name":"推广员","role":"AGENT_L1","status":"ACTIVE","planId":"plan_free"},
			{"id":"user_agent_l2","email":"agent2@xianzhi.ai","name":"初级代理商","role":"AGENT_L2","status":"ACTIVE","planId":"plan_free"},
			{"id":"user_direct","email":"direct@example.com","name":"直推客户","role":"MEMBER","status":"ACTIVE","planId":"plan_month","referredBy":"user_agent_l1"},
			{"id":"user_child","email":"child@example.com","name":"下级客户","role":"MEMBER","status":"ACTIVE","planId":"plan_year","referredBy":"user_agent_l2"},
			{"id":"user_outside","email":"outside@example.com","name":"外部客户","role":"MEMBER","status":"ACTIVE","planId":"plan_month"}
		],
		"plans":[
			{"id":"plan_free","name":"免费会员","price":0,"points":100,"durationDays":36500,"concurrency":1},
			{"id":"plan_month","name":"月度会员","price":9900,"points":3000,"durationDays":30,"concurrency":3},
			{"id":"plan_year","name":"年度会员","price":89900,"points":50000,"durationDays":365,"concurrency":8}
		],
		"pointAccounts":[
			{"id":"points_direct","userId":"user_direct","available":1200},
			{"id":"points_child","userId":"user_child","available":800}
		],
		"orders":[
			{"id":"order_direct","userId":"user_direct","planId":"plan_month","amountCents":9900,"status":"PAID"},
			{"id":"order_child","userId":"user_child","planId":"plan_year","amountCents":89900,"status":"PENDING"},
			{"id":"order_outside","userId":"user_outside","planId":"plan_month","amountCents":9900,"status":"PAID"}
		],
		"channelAgents":[
			{"id":"channel_l1","userId":"user_agent_l1","level":1,"status":"ACTIVE","inviteCode":"L1"},
			{"id":"channel_l2","userId":"user_agent_l2","parentId":"channel_l1","level":2,"status":"ACTIVE","inviteCode":"L2"}
		],
		"commissions":[
			{"id":"commission_l1","orderId":"order_direct","agentId":"channel_l1","amountCents":990,"rate":0.1,"status":"SETTLED"},
			{"id":"commission_l2","orderId":"order_child","agentId":"channel_l2","amountCents":8990,"rate":0.1,"status":"PENDING"}
		],
		"generationTasks":[{"id":"task_direct","userId":"user_direct","type":"TEXT_TO_IMAGE","model":"mock-standard","status":"SUCCEEDED"}],
		"assets":[{"id":"asset_direct","userId":"user_direct","taskId":"task_direct","name":"作品","mediaType":"image","url":"data:image/png;base64,AA=="}],
		"withdrawals":[{"id":"withdrawal_l1","agentId":"channel_l1","amountCents":300,"status":"PENDING"}],
		"counters":{}
	}`
	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	server := New(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()})
	handler := server.Handler

	login := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"agent1@xianzhi.ai","password":"Agent123!"}`))
	var loginBody struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(login.Body).Decode(&loginBody); err != nil {
		t.Fatal(err)
	}

	channelRequest := func(method string, path string, body *bytes.Buffer) *httptest.ResponseRecorder {
		if body == nil {
			body = bytes.NewBuffer(nil)
		}
		req := httptest.NewRequest(method, path, body)
		req.Header.Set("Authorization", "Bearer "+loginBody.AccessToken)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	customers := channelRequest(http.MethodGet, "/api/v1/channel/customers", nil)
	customerBody := customers.Body.String()
	if customers.Code != http.StatusOK || !strings.Contains(customerBody, "直推客户") || !strings.Contains(customerBody, "下级客户") || strings.Contains(customerBody, "外部客户") {
		t.Fatalf("channel customers response = %d %s", customers.Code, customerBody)
	}

	detail := channelRequest(http.MethodGet, "/api/v1/channel/customers/user_direct", nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "order_direct") || !strings.Contains(detail.Body.String(), "task_direct") || !strings.Contains(detail.Body.String(), "asset_direct") {
		t.Fatalf("channel customer detail response = %d %s", detail.Code, detail.Body.String())
	}
	outside := channelRequest(http.MethodGet, "/api/v1/channel/customers/user_outside", nil)
	if outside.Code != http.StatusNotFound {
		t.Fatalf("outside customer detail status = %d, body = %s", outside.Code, outside.Body.String())
	}

	orders := channelRequest(http.MethodGet, "/api/v1/channel/orders", nil)
	orderBody := orders.Body.String()
	if orders.Code != http.StatusOK || !strings.Contains(orderBody, "order_direct") || !strings.Contains(orderBody, "order_child") || strings.Contains(orderBody, "order_outside") {
		t.Fatalf("channel orders response = %d %s", orders.Code, orderBody)
	}

	commissions := channelRequest(http.MethodGet, "/api/v1/channel/commissions", nil)
	commissionBody := commissions.Body.String()
	if commissions.Code != http.StatusOK || !strings.Contains(commissionBody, "commission_l1") || strings.Contains(commissionBody, "commission_l2") {
		t.Fatalf("channel commissions response = %d %s", commissions.Code, commissionBody)
	}

	withdrawal := channelRequest(http.MethodPost, "/api/v1/channel/withdrawals", bytes.NewBufferString(`{"agentId":"channel_l2","amountCents":500}`))
	withdrawalBody := withdrawal.Body.String()
	if withdrawal.Code != http.StatusOK || !strings.Contains(withdrawalBody, `"agentId":"channel_l1"`) {
		t.Fatalf("channel withdrawal response = %d %s", withdrawal.Code, withdrawalBody)
	}
}
func TestLegacyCreateChannelAgentIsBlocked(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	testStore := newJSONStore(dataPath)
	server := newWithStore(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	}, testStore)
	handler := server.Handler
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	createL1 := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/channel-agents", bytes.NewBufferString(`{"name":"测试推广员","email":"agent-new@example.com","level":1,"inviteCode":"NEW001","status":"ACTIVE","available":88}`), adminToken)
	if createL1.Code != http.StatusConflict || !strings.Contains(createL1.Body.String(), "customer 360 identity management") {
		t.Fatalf("legacy channel write was not blocked: %d %s", createL1.Code, createL1.Body.String())
	}
	if _, _, err := testStore.CreateAdminChannelAgent(adminChannelCreateMutation{Name: "Bypass", Email: "bypass@example.test", Level: 1}); err == nil {
		t.Fatal("store-level legacy channel creation bypass was not blocked")
	}
	if _, err := testStore.UpdateAdminChannelAgent("channel_000001", adminChannelMutation{Status: "TERMINATED"}); err == nil {
		t.Fatal("store-level legacy channel update bypass was not blocked")
	}
	data, err := testStore.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range data.Users {
		if strings.EqualFold(user.Email, "agent-new@example.com") {
			t.Fatalf("blocked legacy write created user %+v", user)
		}
	}
}

func TestAdminCustomerProfileRejectsRoleEscalation(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	testStore := newJSONStore(dataPath)
	server := newWithStore(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()}, testStore)
	handler := server.Handler
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	before, err := testStore.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	var target adminUser
	for _, candidate := range before.Users {
		if candidate.ID == "user_000002" {
			target = candidate
			break
		}
	}
	if target.ID == "" {
		t.Fatal("test customer not found")
	}
	response := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/customers/"+target.ID, bytes.NewBufferString(`{"role":"SUPER_ADMIN","name":"tampered"}`), adminToken)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "protected fields: role") {
		t.Fatalf("role escalation was not explicitly rejected: %d %s", response.Code, response.Body.String())
	}
	after, err := testStore.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	var unchanged adminUser
	for _, candidate := range after.Users {
		if candidate.ID == target.ID {
			unchanged = candidate
			break
		}
	}
	if unchanged.Role != target.Role || unchanged.Name != target.Name {
		t.Fatalf("rejected request mutated customer before=%+v after=%+v", target, unchanged)
	}
	if updated, err := testStore.UpdateAdminCustomer(target.ID, adminCustomerMutation{Role: "SUPER_ADMIN", Name: "safe-name"}); err != nil {
		t.Fatal(err)
	} else if updated.Role != target.Role {
		t.Fatalf("store-level role write was not blocked: %s", updated.Role)
	}
	create := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/customers", bytes.NewBufferString(`{"name":"escalation","email":"escalation@example.test","role":"SUPER_ADMIN"}`), adminToken)
	if create.Code != http.StatusBadRequest || !strings.Contains(create.Body.String(), "protected fields: role") {
		t.Fatalf("customer creation role escalation was not rejected: %d %s", create.Code, create.Body.String())
	}
	data, err := testStore.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range data.Users {
		if strings.EqualFold(user.Email, "escalation@example.test") {
			t.Fatalf("rejected role escalation created a user: %+v", user)
		}
	}
}

func TestDeleteMissingAssetReturnsNotFound(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	token := loginToken(t, server.Handler, "demo@xianzhi.ai", "Demo123!")

	assertAuthedStatus(t, server.Handler, http.MethodDelete, "/api/v1/assets/missing", nil, token, http.StatusNotFound)
}

func TestUserAssetAndTaskListsAreIsolatedByLoginUser(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	raw := `{
		"users":[
			{"id":"user_000002","email":"demo@xianzhi.ai","name":"demo","role":"MEMBER","status":"ACTIVE","planId":"plan_month"},
			{"id":"user_000010","email":"demo2@xianzhi.ai","name":"demo2","role":"MEMBER","status":"ACTIVE","planId":"plan_free"}
		],
		"pointAccounts":[
			{"id":"points_000002","userId":"user_000002","available":3000,"frozen":0},
			{"id":"points_000010","userId":"user_000010","available":100,"frozen":0}
		],
		"generationTasks":[
			{"id":"task_demo","userId":"user_000002","type":"TEXT_TO_IMAGE","prompt":"demo only","model":"mock-standard","status":"SUCCEEDED","progress":100,"pointCost":1,"resultIds":["asset_demo"]},
			{"id":"task_demo2","userId":"user_000010","type":"TEXT_TO_IMAGE","prompt":"demo2 only","model":"mock-standard","status":"SUCCEEDED","progress":100,"pointCost":1,"resultIds":["asset_demo2"]}
		],
		"assets":[
			{"id":"asset_demo","userId":"user_000002","taskId":"task_demo","name":"demo asset","mediaType":"image","url":"data:image/svg+xml;base64,PHN2Zy8+","favorite":false,"metadata":{}},
			{"id":"asset_demo2","userId":"user_000010","taskId":"task_demo2","name":"demo2 asset","mediaType":"image","url":"data:image/svg+xml;base64,PHN2Zy8+","favorite":false,"metadata":{}},
			{"id":"asset_private","userId":"user_000010","taskId":"task_demo2","name":"private asset","mediaType":"image","url":"http://127.0.0.1/private.png","favorite":false,"metadata":{}}
		],
		"counters":{}
	}`
	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	server := New(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()})
	handler := server.Handler
	demo2Token := loginToken(t, handler, "demo2@xianzhi.ai", "Demo123!")

	tasks := authedRequest(t, handler, http.MethodGet, "/api/v1/generation-tasks", nil, demo2Token)
	if tasks.Code != http.StatusOK || strings.Contains(tasks.Body.String(), "task_demo\"") || !strings.Contains(tasks.Body.String(), "task_demo2") {
		t.Fatalf("demo2 task isolation failed: %d %s", tasks.Code, tasks.Body.String())
	}
	assets := authedRequest(t, handler, http.MethodGet, "/api/v1/assets", nil, demo2Token)
	if assets.Code != http.StatusOK || strings.Contains(assets.Body.String(), "asset_demo\"") || !strings.Contains(assets.Body.String(), "asset_demo2") {
		t.Fatalf("demo2 asset isolation failed: %d %s", assets.Code, assets.Body.String())
	}
	download := authedRequest(t, handler, http.MethodGet, "/api/v1/assets/asset_demo/download", nil, demo2Token)
	if download.Code != http.StatusNotFound {
		t.Fatalf("demo2 could download demo asset: %d %s", download.Code, download.Body.String())
	}
	privateDownload := authedRequest(t, handler, http.MethodGet, "/api/v1/assets/asset_private/download", nil, demo2Token)
	if privateDownload.Code != http.StatusBadRequest || !strings.Contains(privateDownload.Body.String(), "not public") {
		t.Fatalf("private asset download was not blocked: %d %s", privateDownload.Code, privateDownload.Body.String())
	}
	deleteOther := authedRequest(t, handler, http.MethodDelete, "/api/v1/assets/asset_demo", nil, demo2Token)
	if deleteOther.Code != http.StatusNotFound {
		t.Fatalf("demo2 could delete demo asset: %d %s", deleteOther.Code, deleteOther.Body.String())
	}
	demoToken := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")
	demoAssets := authedRequest(t, handler, http.MethodGet, "/api/v1/assets", nil, demoToken)
	if demoAssets.Code != http.StatusOK || !strings.Contains(demoAssets.Body.String(), "asset_demo") {
		t.Fatalf("demo asset was removed by other user delete attempt: %d %s", demoAssets.Code, demoAssets.Body.String())
	}
	deleteOwn := authedRequest(t, handler, http.MethodDelete, "/api/v1/assets/asset_demo2", nil, demo2Token)
	if deleteOwn.Code != http.StatusOK {
		t.Fatalf("demo2 could not delete own asset: %d %s", deleteOwn.Code, deleteOwn.Body.String())
	}

	create := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks", bytes.NewBufferString(`{"type":"TEXT_TO_IMAGE","prompt":"demo2 create","model":"mock-standard","params":{"count":1}}`), demo2Token)
	if create.Code != http.StatusOK {
		t.Fatalf("demo2 create task status = %d, body = %s", create.Code, create.Body.String())
	}
	var created generationTask
	if err := json.NewDecoder(create.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.UserID != "user_000010" {
		t.Fatalf("created task user = %q, want user_000010: %+v", created.UserID, created)
	}
	createdTasks := authedRequest(t, handler, http.MethodGet, "/api/v1/generation-tasks", nil, demo2Token)
	if createdTasks.Code != http.StatusOK || !strings.Contains(createdTasks.Body.String(), created.ID) || strings.Contains(createdTasks.Body.String(), `"userId":"user_000002"`) {
		t.Fatalf("demo2 created task not isolated: %d %s", createdTasks.Code, createdTasks.Body.String())
	}
}

func TestSelectedProviderMustSupportRequestedModel(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	raw := `{
		"users":[{"id":"user_000002","email":"demo@xianzhi.ai","name":"demo","role":"MEMBER","status":"ACTIVE","planId":"plan_month"}],
		"pointAccounts":[{"id":"points_000002","userId":"user_000002","available":3000,"frozen":0}],
		"apiChannels":[
			{"id":"channel_video_only","name":"video-only","baseUrl":"https://provider.example.com/v1","status":"ACTIVE","priority":10,"models":["doubao-seedance-2.0"]}
		],
		"apiKeys":[{"id":"key_video_only","customer":"channel_video_only","secret":"sk-video-only","status":"ACTIVE","models":["doubao-seedance-2.0"]}],
		"counters":{}
	}`
	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	server := New(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()})
	token := loginToken(t, server.Handler, "demo@xianzhi.ai", "Demo123!")
	body := `{"type":"TEXT_TO_IMAGE","prompt":"should not route","model":"gpt-image-2","params":{"provider":"channel_video_only","count":1}}`
	res := authedRequest(t, server.Handler, http.MethodPost, "/api/v1/generation-tasks", bytes.NewBufferString(body), token)
	rejection := res.Body.String()
	if res.Code != http.StatusBadRequest ||
		(!strings.Contains(rejection, "does not support model gpt-image-2") &&
			!strings.Contains(rejection, "支持模型 gpt-image-2")) {
		t.Fatalf("unsupported provider/model was not rejected: %d %s", res.Code, res.Body.String())
	}
	tasks := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/generation-tasks", nil, token)
	if tasks.Code != http.StatusOK || strings.Contains(tasks.Body.String(), "should not route") {
		t.Fatalf("task was created despite unsupported provider/model: %d %s", tasks.Code, tasks.Body.String())
	}
}

func TestAdminAPIsReadMasterControlData(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	raw := `{
		"users":[{"id":"user_000001","email":"admin@xianzhi.ai","name":"平台管理员","role":"SUPER_ADMIN","status":"ACTIVE","planId":"plan_free"},{"id":"user_000002","email":"demo@xianzhi.ai","name":"演示用户","role":"MEMBER","status":"ACTIVE","planId":"plan_month"}],
		"plans":[{"id":"plan_free","name":"免费会员","price":0,"points":100,"durationDays":36500,"concurrency":1},{"id":"plan_month","name":"月度会员","price":9900,"points":3000,"durationDays":30,"concurrency":3}],
		"pointAccounts":[{"id":"points_000002","userId":"user_000002","available":3000,"frozen":0}],
		"orders":[{"id":"order_000001","userId":"user_000002","planId":"plan_month","amount":9900,"status":"PAID","createdAt":"2026-06-18T00:00:00Z"}],
		"channelAgents":[{"id":"channel_000001","userId":"user_000002","level":1,"status":"ACTIVE","inviteCode":"EAST001"}],
		"generationTasks":[{"id":"task_000001","userId":"user_000002","type":"TEXT_TO_IMAGE","prompt":"测试","model":"mock-standard","status":"SUCCEEDED","progress":100,"pointCost":1,"resultIds":[]}],
		"agentCalls":[{"id":"agentcall_000001","agentId":"agent_000001","userId":"user_000002","tokenUsage":20,"cost":2}],
		"geoTasks":[{"id":"geo_000001","ownerId":"user_000002","brandId":"brand_000001","question":"测试","platform":"ChatGPT","status":"DONE"}],
		"assets":[],
		"counters":{}
	}`
	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	adminToken := loginToken(t, server.Handler, "admin@xianzhi.ai", "Admin123!")

	for _, path := range []string{
		"/api/v1/admin/overview",
		"/api/v1/admin/customers",
		"/api/v1/admin/channel-agents/tree",
		"/api/v1/admin/products",
		"/api/v1/admin/plans",
		"/api/v1/admin/orders",
		"/api/v1/admin/delivery-projects",
		"/api/v1/admin/usage",
		"/api/v1/admin/commissions",
		"/api/v1/admin/system/settings",
		"/api/v1/admin/api/provider-channels",
		"/api/v1/admin/api/models",
		"/api/v1/admin/api/keys",
		"/api/v1/admin/customer-groups",
		"/v1/dashboard/billing/subscription",
		"/v1/dashboard/billing/usage",
	} {
		assertAuthedStatus(t, server.Handler, http.MethodGet, path, nil, adminToken, http.StatusOK)
	}

	overviewRes := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/admin/overview", nil, adminToken)
	var overview map[string]any
	if err := json.NewDecoder(overviewRes.Body).Decode(&overview); err != nil {
		t.Fatal(err)
	}
	if _, ok := overview["metrics"].([]any); !ok {
		t.Fatalf("overview metrics missing: %+v", overview)
	}
}

func TestAdminMutationAPIsPersistMasterControlData(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	createCustomer := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/customers", bytes.NewBufferString(`{"name":"测试客户","email":"customer@example.com","planId":"plan_month","available":6000}`), adminToken)
	if createCustomer.Code != http.StatusOK {
		t.Fatalf("create customer status = %d, body = %s", createCustomer.Code, createCustomer.Body.String())
	}
	var customerBody struct {
		Item adminUser `json:"item"`
	}
	if err := json.NewDecoder(createCustomer.Body).Decode(&customerBody); err != nil {
		t.Fatal(err)
	}
	if customerBody.Item.ID == "" {
		t.Fatalf("created customer missing id: %+v", customerBody)
	}

	updateCustomerPath := "/api/v1/admin/customers/" + customerBody.Item.ID
	assertAuthedStatus(t, handler, http.MethodPatch, updateCustomerPath, bytes.NewBufferString(`{"status":"DISABLED","available":7000}`), adminToken, http.StatusOK)

	createOrder := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/orders", bytes.NewBufferString(`{"userId":"`+customerBody.Item.ID+`","planId":"plan_year","amountCents":89900}`), adminToken)
	if createOrder.Code != http.StatusOK {
		t.Fatalf("create order status = %d, body = %s", createOrder.Code, createOrder.Body.String())
	}
	var orderBody struct {
		Item adminOrder `json:"item"`
	}
	if err := json.NewDecoder(createOrder.Body).Decode(&orderBody); err != nil {
		t.Fatal(err)
	}
	assertAuthedStatus(t, handler, http.MethodPost, "/api/v1/admin/orders/"+orderBody.Item.ID+"/mark-paid", bytes.NewBuffer(nil), adminToken, http.StatusOK)
	assertAuthedStatus(t, handler, http.MethodPost, "/api/v1/admin/orders/"+orderBody.Item.ID+"/renew", bytes.NewBuffer(nil), adminToken, http.StatusOK)

	customers := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/customers", nil, adminToken)
	if !strings.Contains(customers.Body.String(), `"status":"DISABLED"`) || !strings.Contains(customers.Body.String(), `"pointsAvailable":107000`) {
		t.Fatalf("customer update was not persisted: %s", customers.Body.String())
	}
	orders := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/orders", nil, adminToken)
	if !strings.Contains(orders.Body.String(), `"status":"PAID"`) || !strings.Contains(orders.Body.String(), `"status":"PENDING"`) {
		t.Fatalf("order mutations were not persisted: %s", orders.Body.String())
	}

	customer360 := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/customers/"+customerBody.Item.ID+"/360", nil, adminToken)
	if customer360.Code != http.StatusOK || !strings.Contains(customer360.Body.String(), `"profile"`) || !strings.Contains(customer360.Body.String(), `"wallet"`) || !strings.Contains(customer360.Body.String(), `"orders"`) {
		t.Fatalf("customer 360 response = %d %s", customer360.Code, customer360.Body.String())
	}
	if strings.Contains(customer360.Body.String(), "passwordHash") || strings.Contains(customer360.Body.String(), "externalKey") {
		t.Fatalf("customer 360 leaked secret fields: %s", customer360.Body.String())
	}

	timeline := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/orders/"+orderBody.Item.ID+"/timeline", nil, adminToken)
	if timeline.Code != http.StatusOK || !strings.Contains(timeline.Body.String(), `"timeline"`) || !strings.Contains(timeline.Body.String(), `"支付确认"`) || !strings.Contains(timeline.Body.String(), `"权益发放"`) {
		t.Fatalf("order timeline response = %d %s", timeline.Code, timeline.Body.String())
	}

	overview := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/overview", nil, adminToken)
	if overview.Code != http.StatusOK || !strings.Contains(overview.Body.String(), `"tasks"`) || !strings.Contains(overview.Body.String(), `"exceptions"`) {
		t.Fatalf("overview work center response = %d %s", overview.Code, overview.Body.String())
	}

	search := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/search?q=customer%40example.com", nil, adminToken)
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), customerBody.Item.ID) || !strings.Contains(search.Body.String(), `"module":"customers"`) {
		t.Fatalf("admin global search response = %d %s", search.Code, search.Body.String())
	}
	if strings.Contains(search.Body.String(), "passwordHash") || strings.Contains(search.Body.String(), "externalKey") {
		t.Fatalf("admin global search leaked secret fields: %s", search.Body.String())
	}
	orderSearch := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/search?q="+url.QueryEscape(orderBody.Item.ID), nil, adminToken)
	if orderSearch.Code != http.StatusOK || !strings.Contains(orderSearch.Body.String(), orderBody.Item.ID) || !strings.Contains(orderSearch.Body.String(), `"module":"orders"`) {
		t.Fatalf("admin order search response = %d %s", orderSearch.Code, orderSearch.Body.String())
	}
	searchData, err := newJSONStore(dataPath).AdminData()
	if err != nil {
		t.Fatal(err)
	}
	if len(searchData.Enterprise.Tenants) > 0 {
		enterpriseSearch := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/search?q="+url.QueryEscape(searchData.Enterprise.Tenants[0].ID), nil, adminToken)
		if enterpriseSearch.Code != http.StatusOK || !strings.Contains(enterpriseSearch.Body.String(), `"type":"enterprise"`) {
			t.Fatalf("enterprise search response = %d %s", enterpriseSearch.Code, enterpriseSearch.Body.String())
		}
	}
	if len(searchData.GenerationTasks) > 0 {
		taskSearch := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/search?q="+url.QueryEscape(searchData.GenerationTasks[0].ID), nil, adminToken)
		if taskSearch.Code != http.StatusOK || !strings.Contains(taskSearch.Body.String(), `"type":"generation_task"`) {
			t.Fatalf("generation task search response = %d %s", taskSearch.Code, taskSearch.Body.String())
		}
	}
	invoiceSearch := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/search?q=XZ-BILL", nil, adminToken)
	if invoiceSearch.Code != http.StatusOK || !strings.Contains(invoiceSearch.Body.String(), `"type":"invoice"`) {
		t.Fatalf("invoice search response = %d %s", invoiceSearch.Code, invoiceSearch.Body.String())
	}
	if len(searchData.Payments) > 0 {
		paymentSearch := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/search?q="+url.QueryEscape(searchData.Payments[0].ID), nil, adminToken)
		if paymentSearch.Code != http.StatusOK || !strings.Contains(paymentSearch.Body.String(), `"type":"payment"`) {
			t.Fatalf("payment search response = %d %s", paymentSearch.Code, paymentSearch.Body.String())
		}
	}

	started := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/experience-events", bytes.NewBufferString(`{"eventType":"TASK_STARTED","moduleId":"orders","targetId":"`+orderBody.Item.ID+`","metadata":{"sessionId":"human-session-1"}}`), adminToken)
	if started.Code != http.StatusNoContent {
		t.Fatalf("record experience event = %d %s", started.Code, started.Body.String())
	}
	completed := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/experience-events", bytes.NewBufferString(`{"eventType":"TASK_COMPLETED","moduleId":"orders","targetId":"`+orderBody.Item.ID+`","metadata":{"sessionId":"human-session-1"}}`), adminToken)
	if completed.Code != http.StatusNoContent {
		t.Fatalf("record completed experience event = %d %s", completed.Code, completed.Body.String())
	}
	synthetic := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/experience-events", bytes.NewBufferString(`{"eventType":"MODULE_VIEW","moduleId":"orders","metadata":{"sessionId":"e2e-session","synthetic":true}}`), adminToken)
	if synthetic.Code != http.StatusNoContent {
		t.Fatalf("record synthetic experience event = %d %s", synthetic.Code, synthetic.Body.String())
	}
	analytics := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/experience-analytics?days=30", nil, adminToken)
	if analytics.Code != http.StatusOK || !strings.Contains(analytics.Body.String(), `"TASK_STARTED":1`) || !strings.Contains(analytics.Body.String(), `"taskCompletionRate":1`) || !strings.Contains(analytics.Body.String(), `"syntheticEvents":1`) || !strings.Contains(analytics.Body.String(), `"totalEvents":2`) || !strings.Contains(analytics.Body.String(), `"uniqueSessions":1`) || !strings.Contains(analytics.Body.String(), `"sampleReady":false`) {
		t.Fatalf("experience analytics = %d %s", analytics.Code, analytics.Body.String())
	}

	if strings.Contains(overview.Body.String(), `"id":"generation-failures"`) {
		assign := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/exceptions/generation-failures", bytes.NewBufferString(`{"assigneeName":"值班运营","status":"IN_PROGRESS","note":"开始排查"}`), adminToken)
		if assign.Code != http.StatusOK || !strings.Contains(assign.Body.String(), `"assigneeName":"值班运营"`) || !strings.Contains(assign.Body.String(), `"status":"IN_PROGRESS"`) {
			t.Fatalf("assign exception case = %d %s", assign.Code, assign.Body.String())
		}
		closeCase := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/exceptions/generation-failures", bytes.NewBufferString(`{"assigneeName":"值班运营","status":"CLOSED","closeReason":"已完成失败任务复核"}`), adminToken)
		if closeCase.Code != http.StatusOK || !strings.Contains(closeCase.Body.String(), `"status":"CLOSED"`) || !strings.Contains(closeCase.Body.String(), `"closeReason":"已完成失败任务复核"`) || !strings.Contains(closeCase.Body.String(), `"history"`) {
			t.Fatalf("close exception case = %d %s", closeCase.Code, closeCase.Body.String())
		}
	}
}

func TestAdminCustomerIdentityOperations(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	store := newJSONStore(dataPath)
	phoneOnly, err := store.CreateAdminCustomer(adminCustomerMutation{
		Name: "Phone Only", Email: phoneSyntheticEmail("13500009999"), Mobile: "13500009999", Role: "MEMBER", Status: "ACTIVE",
	})
	if err != nil {
		t.Fatal(err)
	}
	identityUser, err := store.CreateAdminCustomer(adminCustomerMutation{
		Name: "Identity User", Email: "identity@example.test", Mobile: "13600008888", WeChatOpenID: "openid-admin-identity", Role: "MEMBER", Status: "ACTIVE",
	})
	if err != nil {
		t.Fatal(err)
	}
	passwordHash, err := hashPassword("Identity123!")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateUserPassword(identityUser.ID, passwordHash); err != nil {
		t.Fatal(err)
	}
	sessions := newLocalAuthSessions()
	server := newWithStoreAndSessions(config.Config{
		Addr:           ":0",
		DataPath:       dataPath,
		StaticDir:      t.TempDir(),
		AdminStaticDir: t.TempDir(),
	}, store, sessions)
	handler := server.Handler
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	identity := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/customers/"+identityUser.ID+"/identities", nil, adminToken)
	if identity.Code != http.StatusOK || !strings.Contains(identity.Body.String(), "wechat_mini_program") || !strings.Contains(identity.Body.String(), "mobile_sms") {
		t.Fatalf("identity detail status = %d, body = %s", identity.Code, identity.Body.String())
	}
	mergeRequest, err := store.CreateAdminAuthMergeRequest(adminAuthMergeRequestMutation{
		PrimaryUserID:   identityUser.ID,
		SecondaryUserID: phoneOnly.ID,
		Mobile:          "13500009999",
		WeChatOpenID:    "openid-admin-identity",
		ConflictCode:    "AUTH_ACCOUNT_MERGE_REQUIRED",
		Source:          "test",
		Reason:          "test merge conflict",
	})
	if err != nil {
		t.Fatal(err)
	}
	mergeRequests := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/customers/"+identityUser.ID+"/account-merge-requests", nil, adminToken)
	if mergeRequests.Code != http.StatusOK || !strings.Contains(mergeRequests.Body.String(), mergeRequest.ID) || !strings.Contains(mergeRequests.Body.String(), "135****9999") || strings.Contains(mergeRequests.Body.String(), "13500009999") {
		t.Fatalf("merge requests status = %d, body = %s", mergeRequests.Code, mergeRequests.Body.String())
	}
	updateMerge := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/account-merge-requests/"+mergeRequest.ID+"/status", bytes.NewBufferString(`{"status":"IN_REVIEW","reviewComment":"人工核验中"}`), adminToken)
	if updateMerge.Code != http.StatusOK || !strings.Contains(updateMerge.Body.String(), `"status":"IN_REVIEW"`) {
		t.Fatalf("update merge request status = %d, body = %s", updateMerge.Code, updateMerge.Body.String())
	}
	sourceForMerge, err := store.CreateAdminCustomer(adminCustomerMutation{
		Name: "Merge Source", Email: "merge-source@example.test", WeChatOpenID: "openid-merge-source", Role: "MEMBER", Status: "ACTIVE", Available: pointBalancePointer(77),
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		data.Presentations = append(data.Presentations, adminPresentation{ID: "presentation_merge_source", UserID: sourceForMerge.ID, Topic: "Merge PPT", Status: "READY", CreatedAt: now, UpdatedAt: now})
		data.Agents = append(data.Agents, adminAgent{ID: "agent_merge_source", OwnerID: sourceForMerge.ID, Name: "Merge Agent", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now})
		data.AgentCalls = append(data.AgentCalls, adminAgentCall{ID: "agent_call_merge_source", AgentID: "agent_merge_source", UserID: sourceForMerge.ID, TokenUsage: 12, CreatedAt: now})
		data.GeoBrands = append(data.GeoBrands, adminGeoBrand{ID: "geo_brand_merge_source", OwnerID: sourceForMerge.ID, Name: "Merge Brand", CreatedAt: now})
		data.GeoTasks = append(data.GeoTasks, adminGeoTask{ID: "geo_task_merge_source", OwnerID: sourceForMerge.ID, BrandID: "geo_brand_merge_source", Status: "READY", CreatedAt: now})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	executableMerge, err := store.CreateAdminAuthMergeRequest(adminAuthMergeRequestMutation{
		PrimaryUserID:   identityUser.ID,
		SecondaryUserID: sourceForMerge.ID,
		WeChatOpenID:    "openid-merge-source",
		ConflictCode:    "AUTH_ACCOUNT_MERGE_REQUIRED",
		Source:          "test_execute",
		Reason:          "execute merge conflict",
	})
	if err != nil {
		t.Fatal(err)
	}
	previewMerge := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/account-merge-requests/"+executableMerge.ID+"/preview?targetUserId="+identityUser.ID, nil, adminToken)
	if previewMerge.Code != http.StatusOK || !strings.Contains(previewMerge.Body.String(), `"executable":true`) || !strings.Contains(previewMerge.Body.String(), `"presentations":1`) || !strings.Contains(previewMerge.Body.String(), `"agentCalls":1`) {
		t.Fatalf("preview merge request status = %d, body = %s", previewMerge.Code, previewMerge.Body.String())
	}
	executeMerge := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/account-merge-requests/"+executableMerge.ID+"/execute", bytes.NewBufferString(`{"targetUserId":"`+identityUser.ID+`","confirm":true,"reviewComment":"确认合并"}`), adminToken)
	if executeMerge.Code != http.StatusOK || !strings.Contains(executeMerge.Body.String(), `"status":"RESOLVED"`) || !strings.Contains(executeMerge.Body.String(), `"sourceUserId":"`+sourceForMerge.ID+`"`) || !strings.Contains(executeMerge.Body.String(), `"presentations":1`) || !strings.Contains(executeMerge.Body.String(), `"agentCalls":1`) {
		t.Fatalf("execute merge request status = %d, body = %s", executeMerge.Code, executeMerge.Body.String())
	}
	mergedData, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	mergedUsers := userMap(mergedData.Users)
	if mergedUsers[sourceForMerge.ID].Status != "MERGED" || len(mergedUsers[sourceForMerge.ID].WeChatOpenIDs) != 0 {
		t.Fatalf("source user not merged: %+v", mergedUsers[sourceForMerge.ID])
	}
	if !containsFold(mergedUsers[identityUser.ID].WeChatOpenIDs, "openid-merge-source") {
		t.Fatalf("target user missing merged wechat openid: %+v", mergedUsers[identityUser.ID].WeChatOpenIDs)
	}
	if mergedData.Presentations[len(mergedData.Presentations)-1].UserID != identityUser.ID {
		t.Fatalf("presentation owner not merged: %+v", mergedData.Presentations[len(mergedData.Presentations)-1])
	}
	if mergedData.Agents[len(mergedData.Agents)-1].OwnerID != identityUser.ID || mergedData.AgentCalls[len(mergedData.AgentCalls)-1].UserID != identityUser.ID {
		t.Fatalf("agent owner/call user not merged: %+v %+v", mergedData.Agents[len(mergedData.Agents)-1], mergedData.AgentCalls[len(mergedData.AgentCalls)-1])
	}
	if mergedData.GeoBrands[len(mergedData.GeoBrands)-1].OwnerID != identityUser.ID || mergedData.GeoTasks[len(mergedData.GeoTasks)-1].OwnerID != identityUser.ID {
		t.Fatalf("geo owner not merged: %+v %+v", mergedData.GeoBrands[len(mergedData.GeoBrands)-1], mergedData.GeoTasks[len(mergedData.GeoTasks)-1])
	}

	blocked := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/customers/"+phoneOnly.ID+"/identities/mobile/unlink", bytes.NewBufferString(`{"reason":"test"}`), adminToken)
	if blocked.Code != http.StatusBadRequest || !strings.Contains(blocked.Body.String(), "last usable login identity") {
		t.Fatalf("last identity unlink status = %d, body = %s", blocked.Code, blocked.Body.String())
	}

	userToken := loginToken(t, handler, "identity@example.test", "Identity123!")
	unlinkWeChat := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/customers/"+identityUser.ID+"/identities/wechat-mini-program/unlink", bytes.NewBufferString(`{"reason":"test"}`), adminToken)
	if unlinkWeChat.Code != http.StatusOK || !strings.Contains(unlinkWeChat.Body.String(), `"wechatLinked":false`) {
		t.Fatalf("unlink wechat status = %d, body = %s", unlinkWeChat.Code, unlinkWeChat.Body.String())
	}
	revokedAfterUnlink := authedRequest(t, handler, http.MethodGet, "/api/v1/auth/me", nil, userToken)
	if revokedAfterUnlink.Code != http.StatusUnauthorized {
		t.Fatalf("token after unlink status = %d, body = %s", revokedAfterUnlink.Code, revokedAfterUnlink.Body.String())
	}

	userToken = loginToken(t, handler, "identity@example.test", "Identity123!")
	freeze := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/customers/"+identityUser.ID+"/freeze-login", bytes.NewBufferString(`{"reason":"test"}`), adminToken)
	if freeze.Code != http.StatusOK || !strings.Contains(freeze.Body.String(), `"status":"DISABLED"`) {
		t.Fatalf("freeze status = %d, body = %s", freeze.Code, freeze.Body.String())
	}
	revokedAfterFreeze := authedRequest(t, handler, http.MethodGet, "/api/v1/auth/me", nil, userToken)
	if revokedAfterFreeze.Code != http.StatusUnauthorized {
		t.Fatalf("token after freeze status = %d, body = %s", revokedAfterFreeze.Code, revokedAfterFreeze.Body.String())
	}
	unfreeze := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/customers/"+identityUser.ID+"/unfreeze-login", bytes.NewBufferString(`{"reason":"test"}`), adminToken)
	if unfreeze.Code != http.StatusOK || !strings.Contains(unfreeze.Body.String(), `"status":"ACTIVE"`) {
		t.Fatalf("unfreeze status = %d, body = %s", unfreeze.Code, unfreeze.Body.String())
	}
}

func TestMergeUserAIStateValues(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	target := userAIState{
		UserID:          "user_target",
		FavoriteTaskIDs: []string{"task_a"},
		HiddenTaskIDs:   []string{"task_hidden_a"},
		FavoriteCollections: []aiFavoriteCollection{{
			ID: "collection_default", Name: "Default", TaskIDs: []string{"task_a"}, CreatedAt: now,
		}},
		AgentConversations: []aiAgentConversation{{ID: "conversation_same", Title: "Target", CreatedAt: now}},
	}
	source := userAIState{
		UserID:          "user_source",
		FavoriteTaskIDs: []string{"task_a", "task_b"},
		HiddenTaskIDs:   []string{"task_hidden_b"},
		FavoriteCollections: []aiFavoriteCollection{
			{ID: "collection_default", Name: "Default", TaskIDs: []string{"task_b"}, CreatedAt: now},
			{ID: "collection_source", Name: "Source", TaskIDs: []string{"task_c"}, CreatedAt: now},
		},
		AgentConversations: []aiAgentConversation{
			{ID: "conversation_same", Title: "Source Same", CreatedAt: now},
			{ID: "conversation_source", Title: "Source", CreatedAt: now},
		},
		ActiveConversationID: "conversation_same",
	}
	merged, moved := mergeUserAIStateValues(target, source, "user_target")
	if moved == 0 || merged.UserID != "user_target" {
		t.Fatalf("unexpected merge result moved=%d state=%+v", moved, merged)
	}
	if !containsFold(merged.FavoriteTaskIDs, "task_b") || !containsFold(merged.HiddenTaskIDs, "task_hidden_b") {
		t.Fatalf("tasks were not merged: %+v %+v", merged.FavoriteTaskIDs, merged.HiddenTaskIDs)
	}
	if len(merged.FavoriteCollections) != 2 || !containsFold(merged.FavoriteCollections[0].TaskIDs, "task_b") {
		t.Fatalf("collections were not merged: %+v", merged.FavoriteCollections)
	}
	conversationIDs := []string{}
	for _, conversation := range merged.AgentConversations {
		conversationIDs = append(conversationIDs, conversation.ID)
	}
	if len(merged.AgentConversations) != 3 || !containsFold(conversationIDs, "user_target-conversation_same") {
		t.Fatalf("conversation conflict was not renamed: %+v", merged.AgentConversations)
	}
	if merged.ActiveConversationID != "user_target-conversation_same" {
		t.Fatalf("active conversation did not follow renamed source conversation: %+v", merged.ActiveConversationID)
	}
}

func TestAdminAuthMergePreviewBlocksChannelAgentConflict(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	store := newJSONStore(dataPath)
	target, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Merge Target Agent", Email: "merge-target-agent@example.test", Role: "MEMBER", Status: "ACTIVE"})
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Merge Source Agent", Email: "merge-source-agent@example.test", Role: "MEMBER", Status: "ACTIVE"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		data.ChannelAgents = append(data.ChannelAgents,
			adminChannelAgent{ID: "channel_target_merge", UserID: target.ID, Level: 1, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
			adminChannelAgent{ID: "channel_source_merge", UserID: source.ID, Level: 1, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	mergeRequest, err := store.CreateAdminAuthMergeRequest(adminAuthMergeRequestMutation{
		PrimaryUserID:   target.ID,
		SecondaryUserID: source.ID,
		ConflictCode:    "AUTH_ACCOUNT_MERGE_REQUIRED",
		Source:          "test_preview_blocker",
		Reason:          "agent identity conflict",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := newWithStoreAndSessions(config.Config{
		Addr:           ":0",
		DataPath:       dataPath,
		StaticDir:      t.TempDir(),
		AdminStaticDir: t.TempDir(),
	}, store, newLocalAuthSessions())
	adminToken := loginToken(t, server.Handler, "admin@xianzhi.ai", "Admin123!")
	preview := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/admin/account-merge-requests/"+mergeRequest.ID+"/preview?targetUserId="+target.ID, nil, adminToken)
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), `"executable":false`) || !strings.Contains(preview.Body.String(), "both users have channel agent identities") {
		t.Fatalf("blocked preview status = %d, body = %s", preview.Code, preview.Body.String())
	}
	execute := authedRequest(t, server.Handler, http.MethodPost, "/api/v1/admin/account-merge-requests/"+mergeRequest.ID+"/execute", bytes.NewBufferString(`{"targetUserId":"`+target.ID+`","confirm":true}`), adminToken)
	if execute.Code != http.StatusBadRequest || !strings.Contains(execute.Body.String(), "both users have channel agent identities") {
		t.Fatalf("blocked execute status = %d, body = %s", execute.Code, execute.Body.String())
	}
}

func TestRechargeOrderPaymentAddsPointsAndAgentCommission(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	createOrder := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/orders", bytes.NewBufferString(`{"userId":"user_000002","planId":"recharge_100","amountCents":10000}`), adminToken)
	if createOrder.Code != http.StatusOK {
		t.Fatalf("create recharge order status = %d, body = %s", createOrder.Code, createOrder.Body.String())
	}
	var orderBody struct {
		Item adminOrder `json:"item"`
	}
	if err := json.NewDecoder(createOrder.Body).Decode(&orderBody); err != nil {
		t.Fatal(err)
	}
	markPaid := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/orders/"+orderBody.Item.ID+"/mark-paid", bytes.NewBuffer(nil), adminToken)
	if markPaid.Code != http.StatusOK {
		t.Fatalf("mark recharge paid status = %d, body = %s", markPaid.Code, markPaid.Body.String())
	}
	body := markPaid.Body.String()
	for _, want := range []string{`"status":"PAID"`, `"orderType":"COMPUTE_RECHARGE"`, `"rechargePoints":10000`, `"newapiSyncStatus":"READY"`, `"newapiGroup":"生图备份"`, `"newapiKeyId":"key_user_000002"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("paid recharge response missing %q: %s", want, body)
		}
	}
	customers := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/customers", nil, adminToken)
	customerBody := customers.Body.String()
	if customers.Code != http.StatusOK || !strings.Contains(customerBody, `"pointsAvailable":10000`) || !strings.Contains(customerBody, `"modelGroup":"生图备份"`) || !strings.Contains(customerBody, `"modelApiKeyId":"key_user_000002"`) {
		t.Fatalf("recharge points not reflected: %d %s", customers.Code, customers.Body.String())
	}
	commissions := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/commissions", nil, adminToken)
	commissionBody := commissions.Body.String()
	if commissions.Code != http.StatusOK || !strings.Contains(commissionBody, orderBody.Item.ID) || !strings.Contains(commissionBody, `"amountCents":800`) || !strings.Contains(commissionBody, `"source":"compute_recharge"`) || !strings.Contains(commissionBody, `"ruleId":"rule_recharge_l1_direct"`) {
		t.Fatalf("recharge commission missing: %d %s", commissions.Code, commissionBody)
	}
	markPaidAgain := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/orders/"+orderBody.Item.ID+"/mark-paid", bytes.NewBuffer(nil), adminToken)
	if markPaidAgain.Code != http.StatusOK {
		t.Fatalf("repeat mark paid status = %d, body = %s", markPaidAgain.Code, markPaidAgain.Body.String())
	}
	customersAfterRepeat := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/customers", nil, adminToken)
	if strings.Count(customersAfterRepeat.Body.String(), `"pointsAvailable":10000`) == 0 || strings.Contains(customersAfterRepeat.Body.String(), `"pointsAvailable":20000`) {
		t.Fatalf("repeat mark paid was not idempotent: %s", customersAfterRepeat.Body.String())
	}
	commissionsAfterRepeat := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/commissions", nil, adminToken)
	if strings.Count(commissionsAfterRepeat.Body.String(), `"ruleId":"rule_recharge_l1_direct"`) != 1 {
		t.Fatalf("repeat mark paid duplicated commissions: %s", commissionsAfterRepeat.Body.String())
	}
	walletRecords := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/marketing/wallet-records", nil, adminToken)
	if walletRecords.Code != http.StatusOK || !strings.Contains(walletRecords.Body.String(), `"bizType":"COMMISSION_INCOME"`) || !strings.Contains(walletRecords.Body.String(), `"rule_recharge_l1_direct"`) {
		t.Fatalf("wallet records missing recharge commission: %d %s", walletRecords.Code, walletRecords.Body.String())
	}
	statements := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/marketing/settlement-statements", nil, adminToken)
	if statements.Code != http.StatusOK || !strings.Contains(statements.Body.String(), `"pendingCents":800`) {
		t.Fatalf("settlement statements missing pending commission: %d %s", statements.Code, statements.Body.String())
	}
}

func TestPaymentCallbackRequiresSecretAndValidatesAmount(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:                  ":0",
		DataPath:              dataPath,
		StaticDir:             t.TempDir(),
		PaymentCallbackSecret: "callback-secret",
	})
	handler := server.Handler
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	createOrder := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/orders", bytes.NewBufferString(`{"userId":"user_000002","planId":"recharge_100","amountCents":10000}`), adminToken)
	if createOrder.Code != http.StatusOK {
		t.Fatalf("create recharge order status = %d, body = %s", createOrder.Code, createOrder.Body.String())
	}
	var orderBody struct {
		Item adminOrder `json:"item"`
	}
	if err := json.NewDecoder(createOrder.Body).Decode(&orderBody); err != nil {
		t.Fatal(err)
	}
	callbackBody := `{"orderId":"` + orderBody.Item.ID + `","paid":true,"amountCents":10000,"eventId":"evt_1","providerTransactionId":"wx_txn_1"}`

	missingSecret := request(t, handler, http.MethodPost, "/api/v1/pay/callback", bytes.NewBufferString(callbackBody))
	if missingSecret.Code != http.StatusUnauthorized {
		t.Fatalf("missing callback secret status = %d, body = %s", missingSecret.Code, missingSecret.Body.String())
	}

	wrongAmountReq := httptest.NewRequest(http.MethodPost, "/api/v1/pay/callback", bytes.NewBufferString(`{"orderId":"`+orderBody.Item.ID+`","paid":true,"amountCents":9999}`))
	wrongAmountReq.Header.Set("X-Xianzhi-Payment-Secret", "callback-secret")
	wrongAmount := httptest.NewRecorder()
	handler.ServeHTTP(wrongAmount, wrongAmountReq)
	if wrongAmount.Code != http.StatusBadRequest || !strings.Contains(wrongAmount.Body.String(), "payment amount mismatch") {
		t.Fatalf("wrong amount status = %d, body = %s", wrongAmount.Code, wrongAmount.Body.String())
	}

	callbackReq := httptest.NewRequest(http.MethodPost, "/api/v1/pay/callback", bytes.NewBufferString(callbackBody))
	callbackReq.Header.Set("X-Xianzhi-Payment-Secret", "callback-secret")
	callback := httptest.NewRecorder()
	handler.ServeHTTP(callback, callbackReq)
	if callback.Code != http.StatusOK || !strings.Contains(callback.Body.String(), `"status":"PAID"`) || !strings.Contains(callback.Body.String(), `"providerTransactionId":"wx_txn_1"`) {
		t.Fatalf("valid callback status = %d, body = %s", callback.Code, callback.Body.String())
	}
	data, err := newJSONStore(dataPath).AdminData()
	if err != nil {
		t.Fatal(err)
	}
	var paidOrder adminOrder
	for _, item := range data.Orders {
		if item.ID == orderBody.Item.ID {
			paidOrder = item
			break
		}
	}
	if paidOrder.ID == "" {
		t.Fatalf("paid order not found in store")
	}
	if got := stringValue(paidOrder.PriceSnapshot["eventId"]); got != "evt_1" {
		t.Fatalf("callback eventId not persisted: %q", got)
	}
	if got := stringValue(paidOrder.PriceSnapshot["providerTransactionId"]); got != "wx_txn_1" {
		t.Fatalf("callback providerTransactionId not persisted: %q", got)
	}
	if got := intValue(paidOrder.PriceSnapshot["paidAmountCents"]); got != 10000 {
		t.Fatalf("callback paidAmountCents not persisted: %d", got)
	}
	if len(data.PaymentEvents) != 1 {
		t.Fatalf("payment events after callback = %d, want 1: %+v", len(data.PaymentEvents), data.PaymentEvents)
	}
	if data.PaymentEvents[0].EventID != "evt_1" || data.PaymentEvents[0].TransactionID != "wx_txn_1" || data.PaymentEvents[0].OrderID != orderBody.Item.ID {
		t.Fatalf("payment event was not persisted with callback identity: %+v", data.PaymentEvents[0])
	}

	repeatReq := httptest.NewRequest(http.MethodPost, "/api/v1/pay/callback", bytes.NewBufferString(callbackBody))
	repeatReq.Header.Set("X-Xianzhi-Payment-Secret", "callback-secret")
	repeat := httptest.NewRecorder()
	handler.ServeHTTP(repeat, repeatReq)
	if repeat.Code != http.StatusOK {
		t.Fatalf("repeat callback status = %d, body = %s", repeat.Code, repeat.Body.String())
	}
	customers := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/customers", nil, adminToken)
	if !strings.Contains(customers.Body.String(), `"pointsAvailable":1000`) || strings.Contains(customers.Body.String(), `"pointsAvailable":2000`) {
		t.Fatalf("repeat callback duplicated point grant: %s", customers.Body.String())
	}
	data, err = newJSONStore(dataPath).AdminData()
	if err != nil {
		t.Fatal(err)
	}
	if len(data.PaymentEvents) != 1 {
		t.Fatalf("repeat callback duplicated payment events: %+v", data.PaymentEvents)
	}

	createSecondOrder := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/orders", bytes.NewBufferString(`{"userId":"user_000002","planId":"recharge_100","amountCents":10000}`), adminToken)
	if createSecondOrder.Code != http.StatusOK {
		t.Fatalf("create second recharge order status = %d, body = %s", createSecondOrder.Code, createSecondOrder.Body.String())
	}
	var secondOrderBody struct {
		Item adminOrder `json:"item"`
	}
	if err := json.NewDecoder(createSecondOrder.Body).Decode(&secondOrderBody); err != nil {
		t.Fatal(err)
	}
	reusedTxnReq := httptest.NewRequest(http.MethodPost, "/api/v1/pay/callback", bytes.NewBufferString(`{"orderId":"`+secondOrderBody.Item.ID+`","paid":true,"amountCents":10000,"eventId":"evt_2","providerTransactionId":"wx_txn_1"}`))
	reusedTxnReq.Header.Set("X-Xianzhi-Payment-Secret", "callback-secret")
	reusedTxn := httptest.NewRecorder()
	handler.ServeHTTP(reusedTxn, reusedTxnReq)
	if reusedTxn.Code != http.StatusBadRequest || !strings.Contains(reusedTxn.Body.String(), "payment transaction already belongs") {
		t.Fatalf("reused transaction status = %d, body = %s", reusedTxn.Code, reusedTxn.Body.String())
	}
}

func TestProductionPaymentCallbackRequiresOfficialWechatSignature(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	devServer := New(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()})
	order := createAdminPaymentOrder(t, devServer.Handler, "wechat")

	prodServer := New(config.Config{
		Environment:           "production",
		Addr:                  ":0",
		DataPath:              dataPath,
		StaticDir:             t.TempDir(),
		PaymentCallbackSecret: "callback-secret",
	})
	callbackBody := `{"orderId":"` + order.ID + `","provider":"wechat","paid":true,"amountCents":10000,"eventId":"evt_unsigned"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pay/callback", bytes.NewBufferString(callbackBody))
	req.Header.Set("X-Xianzhi-Payment-Secret", "callback-secret")
	rec := httptest.NewRecorder()
	prodServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "requires official signature") {
		t.Fatalf("unsigned production wechat callback status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestWeChatPayV3CallbackVerifiesSignatureAndDecryptsResource(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	devServer := New(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()})
	order := createAdminPaymentOrder(t, devServer.Handler, "wechat")

	privateKey := testRSAPrivateKey(t)
	publicKeyPEM := testRSAPublicKeyPEM(t, privateKey)
	apiV3Key := "0123456789abcdef0123456789abcdef"
	resourceNonce := "resource1234"
	associatedData := "transaction"
	resource := map[string]any{
		"out_trade_no":   order.ID,
		"transaction_id": "wx_txn_verified",
		"trade_state":    "SUCCESS",
		"amount": map[string]any{
			"total": 10000,
		},
	}
	resourcePlain, err := json.Marshal(resource)
	if err != nil {
		t.Fatal(err)
	}
	bodyBytes, err := json.Marshal(map[string]any{
		"id":            "evt_wx_verified",
		"event_type":    "TRANSACTION.SUCCESS",
		"resource_type": "encrypt-resource",
		"resource": map[string]any{
			"algorithm":       "AEAD_AES_256_GCM",
			"ciphertext":      testEncryptWeChatPayResource(t, apiV3Key, resourceNonce, associatedData, resourcePlain),
			"associated_data": associatedData,
			"nonce":           resourceNonce,
			"original_type":   "transaction",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	timestamp := "1700000000"
	nonce := "notify123456"
	message := timestamp + "\n" + nonce + "\n" + string(bodyBytes) + "\n"

	prodServer := New(config.Config{
		Environment:           "production",
		Addr:                  ":0",
		DataPath:              dataPath,
		StaticDir:             t.TempDir(),
		PaymentCallbackSecret: "callback-secret",
		WeChatPayAPIv3Key:     apiV3Key,
		WeChatPayPlatformKey:  publicKeyPEM,
		WeChatPayPlatformPath: "",
		AlipayPublicKey:       "",
		AlipayPublicKeyPath:   "",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pay/callback", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Xianzhi-Payment-Secret", "callback-secret")
	req.Header.Set("Wechatpay-Timestamp", timestamp)
	req.Header.Set("Wechatpay-Nonce", nonce)
	req.Header.Set("Wechatpay-Signature", testRSASignature(t, privateKey, message))
	rec := httptest.NewRecorder()
	prodServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"PAID"`) {
		t.Fatalf("wechat callback status = %d, body = %s", rec.Code, rec.Body.String())
	}
	data, err := newJSONStore(dataPath).AdminData()
	if err != nil {
		t.Fatal(err)
	}
	paidOrder := findTestOrder(t, data.Orders, order.ID)
	if got := stringValue(paidOrder.PriceSnapshot["eventId"]); got != "evt_wx_verified" {
		t.Fatalf("wechat event id not persisted: %q", got)
	}
	if got := stringValue(paidOrder.PriceSnapshot["providerTransactionId"]); got != "wx_txn_verified" {
		t.Fatalf("wechat provider transaction id not persisted: %q", got)
	}
}

func TestAlipayCallbackVerifiesRSA2Signature(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	devServer := New(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()})
	order := createAdminPaymentOrder(t, devServer.Handler, "alipay")

	privateKey := testRSAPrivateKey(t)
	publicKeyPEM := testRSAPublicKeyPEM(t, privateKey)
	values := url.Values{}
	values.Set("app_id", "2026000000000000")
	values.Set("notify_id", "evt_alipay_verified")
	values.Set("notify_type", "trade_status_sync")
	values.Set("out_trade_no", order.ID)
	values.Set("total_amount", "100.00")
	values.Set("trade_no", "ali_txn_verified")
	values.Set("trade_status", "TRADE_SUCCESS")
	values.Set("sign_type", "RSA2")
	values.Set("sign", testRSASignature(t, privateKey, alipaySignContent(values)))

	prodServer := New(config.Config{
		Environment:           "production",
		Addr:                  ":0",
		DataPath:              dataPath,
		StaticDir:             t.TempDir(),
		PaymentCallbackSecret: "callback-secret",
		AlipayPublicKey:       publicKeyPEM,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pay/callback", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Xianzhi-Payment-Secret", "callback-secret")
	rec := httptest.NewRecorder()
	prodServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"PAID"`) {
		t.Fatalf("alipay callback status = %d, body = %s", rec.Code, rec.Body.String())
	}
	data, err := newJSONStore(dataPath).AdminData()
	if err != nil {
		t.Fatal(err)
	}
	paidOrder := findTestOrder(t, data.Orders, order.ID)
	if got := stringValue(paidOrder.PriceSnapshot["eventId"]); got != "evt_alipay_verified" {
		t.Fatalf("alipay event id not persisted: %q", got)
	}
	if got := stringValue(paidOrder.PriceSnapshot["providerTransactionId"]); got != "ali_txn_verified" {
		t.Fatalf("alipay provider transaction id not persisted: %q", got)
	}
}

func createAdminPaymentOrder(t *testing.T, handler http.Handler, paymentMethod string) adminOrder {
	t.Helper()
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	createOrder := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/orders", bytes.NewBufferString(`{"userId":"user_000002","planId":"recharge_100","amountCents":10000,"paymentMethod":"`+paymentMethod+`"}`), adminToken)
	if createOrder.Code != http.StatusOK {
		t.Fatalf("create payment order status = %d, body = %s", createOrder.Code, createOrder.Body.String())
	}
	var body struct {
		Item adminOrder `json:"item"`
	}
	if err := json.NewDecoder(createOrder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Item
}

func findTestOrder(t *testing.T, orders []adminOrder, id string) adminOrder {
	t.Helper()
	for _, item := range orders {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("order not found: %s", id)
	return adminOrder{}
}

func testRSAPrivateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func testRSAPublicKeyPEM(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func testRSASignature(t *testing.T, key *rsa.PrivateKey, message string) string {
	t.Helper()
	hash := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(signature)
}

func testEncryptWeChatPayResource(t *testing.T, key string, nonce string, associatedData string, plaintext []byte) string {
	t.Helper()
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nil, []byte(nonce), plaintext, []byte(associatedData)))
}

func TestRechargeCommissionUsesUpdatedRuleRate(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	updateRule := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/marketing/commission-rules/rule_recharge_l1_direct", bytes.NewBufferString(`{"name":"L1 推广员点数包返佣","orderType":"COMPUTE_RECHARGE","earnerRole":"AGENT_L1","relationDepth":1,"fixedAmountCents":0,"rate":0.15,"maxTotalRate":0.2,"status":"ACTIVE"}`), adminToken)
	if updateRule.Code != http.StatusOK || !strings.Contains(updateRule.Body.String(), `"rate":0.15`) {
		t.Fatalf("update commission rule status = %d, body = %s", updateRule.Code, updateRule.Body.String())
	}
	rules := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/marketing/commission-rules", nil, adminToken)
	if rules.Code != http.StatusOK || !strings.Contains(rules.Body.String(), `"rate":0.15`) {
		t.Fatalf("updated commission rule not visible: %d %s", rules.Code, rules.Body.String())
	}

	createOrder := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/orders", bytes.NewBufferString(`{"userId":"user_000002","planId":"recharge_100","amountCents":10000}`), adminToken)
	if createOrder.Code != http.StatusOK {
		t.Fatalf("create recharge order status = %d, body = %s", createOrder.Code, createOrder.Body.String())
	}
	var orderBody struct {
		Item adminOrder `json:"item"`
	}
	if err := json.NewDecoder(createOrder.Body).Decode(&orderBody); err != nil {
		t.Fatal(err)
	}
	markPaid := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/orders/"+orderBody.Item.ID+"/mark-paid", bytes.NewBuffer(nil), adminToken)
	if markPaid.Code != http.StatusOK {
		t.Fatalf("mark recharge paid status = %d, body = %s", markPaid.Code, markPaid.Body.String())
	}
	commissions := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/commissions", nil, adminToken)
	body := commissions.Body.String()
	for _, want := range []string{`"amountCents":1500`, `"rate":0.15`, `"maxTotalRate":0.2`, `"ruleId":"rule_recharge_l1_direct"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("updated rule commission missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `"amountCents":2000`) {
		t.Fatalf("commission still uses old 20%% rule: %s", body)
	}
}

func TestRechargeCommissionUsesL3DifferentialRuleForL2Child(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	raw := `{
		"users":[
			{"id":"user_admin","email":"admin@xianzhi.ai","name":"平台管理员","role":"SUPER_ADMIN","status":"ACTIVE","planId":"plan_free"},
			{"id":"user_customer","email":"customer@xianzhi.ai","name":"客户","role":"MEMBER","status":"ACTIVE","planId":"plan_free","referredBy":"user_child_agent"},
			{"id":"user_parent_agent","email":"parent@xianzhi.ai","name":"上级代理","role":"AGENT_L3","status":"ACTIVE","planId":"plan_free"},
			{"id":"user_child_agent","email":"child@xianzhi.ai","name":"直推代理","role":"AGENT_L2","status":"ACTIVE","planId":"plan_free"}
		],
		"pointAccounts":[{"id":"points_customer","userId":"user_customer","available":0}],
		"channelAgents":[
			{"id":"channel_parent","userId":"user_parent_agent","level":3,"status":"ACTIVE","inviteCode":"PARENT"},
			{"id":"channel_child","userId":"user_child_agent","parentId":"channel_parent","level":2,"status":"ACTIVE","inviteCode":"CHILD"}
		],
		"orders":[],
		"commissions":[],
		"withdrawals":[],
		"counters":{}
	}`
	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	server := New(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()})
	handler := server.Handler
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	createOrder := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/orders", bytes.NewBufferString(`{"userId":"user_customer","planId":"recharge_100","amountCents":10000}`), adminToken)
	if createOrder.Code != http.StatusOK {
		t.Fatalf("create recharge order status = %d, body = %s", createOrder.Code, createOrder.Body.String())
	}
	var orderBody struct {
		Item adminOrder `json:"item"`
	}
	if err := json.NewDecoder(createOrder.Body).Decode(&orderBody); err != nil {
		t.Fatal(err)
	}
	markPaid := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/orders/"+orderBody.Item.ID+"/mark-paid", bytes.NewBuffer(nil), adminToken)
	if markPaid.Code != http.StatusOK {
		t.Fatalf("mark recharge paid status = %d, body = %s", markPaid.Code, markPaid.Body.String())
	}
	commissions := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/commissions", nil, adminToken)
	body := commissions.Body.String()
	for _, want := range []string{`"agentId":"channel_child"`, `"amountCents":1200`, `"ruleId":"rule_recharge_l2_direct"`, `"agentId":"channel_parent"`, `"amountCents":800`, `"ruleId":"rule_recharge_l3_diff_from_l2"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("L2 direct or L3 differential commission response missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `"amountCents":2000`) {
		t.Fatalf("L3 parent should receive differential commission, not full direct commission: %s", body)
	}
}

func TestAdminSystemAndAPIGatewayMutationsPersist(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	assertAuthedStatus(t, handler, http.MethodPatch, "/api/v1/admin/system/settings", bytes.NewBufferString(`{"brand":{"name":"先知主控","domain":"admin.example.com","logo":"控"},"payments":[{"channel":"manual","status":"ACTIVE"}],"permissions":["SUPER_ADMIN","FINANCE"]}`), adminToken, http.StatusOK)
	assertAuthedStatus(t, handler, http.MethodPost, "/api/v1/admin/api/provider-channels", bytes.NewBufferString(`{"name":"测试上游","baseUrl":"https://provider.example.com/v1","status":"ACTIVE","priority":30,"models":["gpt-image-2"]}`), adminToken, http.StatusOK)
	assertAuthedStatus(t, handler, http.MethodPatch, "/api/v1/admin/api/models/model_gpt_image_2", bytes.NewBufferString(`{"name":"OpenAI 图像模型","capability":"IMAGE","billingMode":"PER_REQUEST","fixedQuota":12,"modelRatio":1,"completionRatio":1,"status":"ACTIVE"}`), adminToken, http.StatusOK)
	assertAuthedStatus(t, handler, http.MethodPost, "/api/v1/admin/api/keys", bytes.NewBufferString(`{"customer":"测试客户","status":"ACTIVE","models":["gpt-image-2"],"quotaLimit":50000}`), adminToken, http.StatusOK)
	assertAuthedStatus(t, handler, http.MethodPatch, "/api/v1/admin/customer-groups/group_vip", bytes.NewBufferString(`{"name":"vip","ratio":0.7,"models":["gpt-image-2"],"description":"测试倍率"}`), adminToken, http.StatusOK)

	system := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/system/settings", nil, adminToken)
	body := system.Body.String()
	for _, want := range []string{"先知主控", "admin.example.com", "测试上游", `"fixedQuota":12`, "测试客户", `"ratio":0.7`} {
		if !strings.Contains(body, want) {
			t.Fatalf("system mutation response missing %q: %s", want, body)
		}
	}
}

func TestAdminNewAPIGroupsReturnsEmptyWhenGatewayIsNotConfigured(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()})
	adminToken := loginToken(t, server.Handler, "admin@xianzhi.ai", "Admin123!")

	response := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/admin/newapi/groups", nil, adminToken)
	if response.Code != http.StatusOK {
		t.Fatalf("newapi groups status = %d, body = %s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"items":[]`, `"configured":false`, `"available":false`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("newapi groups response missing %q: %s", want, response.Body.String())
		}
	}
}

func TestAdminNewAPIGroupsDegradesWhenGatewayCredentialIsRejected(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"success":false,"message":"unauthorized"}`))
	}))
	defer upstream.Close()

	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()})
	adminToken := loginToken(t, server.Handler, "admin@xianzhi.ai", "Admin123!")
	settings := fmt.Sprintf(`{"apiGateway":{"newapi":{"enabled":true,"baseUrl":%q,"adminToken":"expired-test-token","adminUserId":"1","timeoutSeconds":2}}}`, upstream.URL)
	assertAuthedStatus(t, server.Handler, http.MethodPatch, "/api/v1/admin/system/settings", bytes.NewBufferString(settings), adminToken, http.StatusOK)

	response := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/admin/newapi/groups", nil, adminToken)
	if response.Code != http.StatusOK {
		t.Fatalf("newapi groups degraded status = %d, body = %s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"items":[]`, `"configured":true`, `"available":false`, `"warning":`, "NewAPI"} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("newapi groups degraded response missing %q: %s", want, response.Body.String())
		}
	}
}

func TestAdminAICapabilityModelsCanBeCreated(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	assertAuthedStatus(t, handler, http.MethodPost, "/api/v1/admin/ai/models", bytes.NewBufferString(`{
		"model_name":"ppt-maker-pro",
		"model_type":"text",
		"provider":"NewAPI",
		"module_code":"ppt_generation",
		"capability_code":["ppt_outline","ppt_content","ppt_export"],
		"fallback_model":"",
		"sort_weight":12,
		"allow_fallback_switch":true,
		"status":"ACTIVE"
	}`), adminToken, http.StatusOK)

	overview := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/ai/overview", nil, adminToken)
	body := overview.Body.String()
	for _, want := range []string{"ppt-maker-pro", `"module_code":"ppt_generation"`, `"ppt_export"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("ai overview missing %q after model creation: %s", want, body)
		}
	}
}

func TestSavedAPIKeyForChannelPrefersExactChannelBinding(t *testing.T) {
	keys := []adminAPIKey{
		{Customer: "API", Status: "ACTIVE", Secret: "sk-local-admin"},
		{Customer: "channel_api_123", Status: "ACTIVE", Secret: "sk-real-provider-key"},
	}
	channel := adminAPIChannel{ID: "channel_api_123", Name: "uni-api"}
	if got := savedAPIKeyForChannel(keys, channel); got != "sk-real-provider-key" {
		t.Fatalf("expected exact channel key, got %q", got)
	}
	if got := savedAPIKeyForChannel(keys[:1], channel); got != "" {
		t.Fatalf("short generic customer API should not match uni-api, got %q", got)
	}
}

func TestUserAPISettingsRequiresAuthAndDoesNotExposeSecrets(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	raw := fmt.Sprintf(`{
		"users":[{"id":"user_000001","email":"admin@xianzhi.ai","name":"Admin","role":"SUPER_ADMIN","status":"ACTIVE","passwordHash":%q,"planId":"plan_free"},{"id":"user_000002","email":"demo@xianzhi.ai","name":"Demo","role":"MEMBER","status":"ACTIVE","passwordHash":%q,"planId":"plan_free"}],
		"apiModels":[{"id":"model_mock","model":"mock-standard","name":"Mock","capability":"TEXT_TO_IMAGE","status":"ACTIVE"},{"id":"model_hidden","model":"hidden-model","name":"Hidden","capability":"TEXT","status":"INACTIVE"}],
		"apiKeys":[{"id":"key_secret","customer":"Demo","prefix":"sk-real","secret":"sk-real-provider-secret","status":"ACTIVE","models":["mock-standard"],"quotaLimit":1000}],
		"customerGroups":[{"id":"group_default","name":"default","ratio":1,"models":["mock-standard"],"description":"Default"}],
		"pointAccounts":[{"id":"points_demo","userId":"user_000002","available":123,"frozen":0}],
		"counters":{}
	}`, legacySHA256PasswordHash("Admin123!"), legacySHA256PasswordHash("Demo123!"))
	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	server := New(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir(), AdminStaticDir: t.TempDir()})

	anonymous := request(t, server.Handler, http.MethodGet, "/api/v1/user/api-settings", nil)
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous api settings status = %d, body = %s", anonymous.Code, anonymous.Body.String())
	}
	token := loginToken(t, server.Handler, "demo@xianzhi.ai", "Demo123!")
	res := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/user/api-settings", nil, token)
	body := res.Body.String()
	if res.Code != http.StatusOK {
		t.Fatalf("authed api settings status = %d, body = %s", res.Code, body)
	}
	for _, forbidden := range []string{"sk-real-provider-secret", `"apiKeys"`, `"apiChannels"`, `"customerGroups"`, "hidden-model"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("user api settings leaked %q: %s", forbidden, body)
		}
	}
	for _, want := range []string{`"models"`, "mock-standard", `"capabilities"`, `"quota"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("user api settings missing %q: %s", want, body)
		}
	}
}

func TestAdminUsageAndCommissionOperations(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	raw := `{
		"users":[{"id":"user_000001","email":"admin@xianzhi.ai","name":"平台管理员","role":"SUPER_ADMIN","status":"ACTIVE","planId":"plan_free"},{"id":"user_000002","email":"demo@xianzhi.ai","name":"演示用户","role":"MEMBER","status":"ACTIVE","planId":"plan_month"},{"id":"user_000003","email":"agent1@xianzhi.ai","name":"华东推广员","role":"AGENT_L1","status":"ACTIVE","planId":"plan_free"}],
		"plans":[{"id":"plan_month","name":"月度会员","price":9900,"points":3000,"durationDays":30,"concurrency":3}],
		"orders":[{"id":"order_000001","userId":"user_000002","planId":"plan_month","amount":9900,"status":"PAID"}],
		"channelAgents":[{"id":"channel_000001","userId":"user_000003","level":1,"status":"ACTIVE","inviteCode":"EAST001"}],
		"generationTasks":[{"id":"task_000001","userId":"user_000002","type":"TEXT_TO_IMAGE","prompt":"测试","model":"mock-standard","status":"SUCCEEDED","progress":100,"pointCost":1,"resultIds":[]}],
		"agentCalls":[{"id":"agentcall_000001","agentId":"agent_000001","userId":"user_000002","tokenUsage":20,"cost":2}],
		"geoTasks":[{"id":"geo_000001","ownerId":"user_000002","brandId":"brand_000001","question":"测试","platform":"ChatGPT","status":"DONE"}],
		"assets":[],
		"counters":{}
	}`
	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	usage := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/usage?product=Agent", nil, adminToken)
	if usage.Code != http.StatusOK || !strings.Contains(usage.Body.String(), `"product":"Agent"`) || strings.Contains(usage.Body.String(), "GEO 任务") {
		t.Fatalf("usage filter failed: status = %d, body = %s", usage.Code, usage.Body.String())
	}
	export := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/usage/export?product=Agent", nil, adminToken)
	if export.Code != http.StatusOK || !strings.Contains(export.Body.String(), "Agent") || !strings.Contains(export.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("usage export failed: status = %d, type = %s, body = %s", export.Code, export.Header().Get("Content-Type"), export.Body.String())
	}

	createCommission := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/commissions", bytes.NewBufferString(`{"orderId":"order_000001","agentId":"channel_000001","amountCents":990,"rate":0.1}`), adminToken)
	if createCommission.Code != http.StatusOK {
		t.Fatalf("create commission status = %d, body = %s", createCommission.Code, createCommission.Body.String())
	}
	createWithdrawal := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/withdrawals", bytes.NewBufferString(`{"agentId":"channel_000001","amountCents":500}`), adminToken)
	if createWithdrawal.Code != http.StatusOK {
		t.Fatalf("create withdrawal status = %d, body = %s", createWithdrawal.Code, createWithdrawal.Body.String())
	}
	var withdrawalBody struct {
		Item adminWithdrawal `json:"item"`
	}
	if err := json.NewDecoder(createWithdrawal.Body).Decode(&withdrawalBody); err != nil {
		t.Fatal(err)
	}
	assertAuthedStatus(t, handler, http.MethodPost, "/api/v1/admin/withdrawals/"+withdrawalBody.Item.ID+"/approve", bytes.NewBuffer(nil), adminToken, http.StatusOK)

	commissions := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/commissions", nil, adminToken)
	body := commissions.Body.String()
	for _, want := range []string{`"amountCents":990`, `"amountCents":500`, `"status":"APPROVED"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("commissions response missing %q: %s", want, body)
		}
	}
}

func TestConcurrentGenerationTaskCreatesKeepUniqueIDs(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	store := newJSONStore(dataPath)
	grantPermanentTestPoints(t, store, "user_000002", 100)
	server := newWithStore(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	}, store)
	handler := server.Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

	const requestCount = 20
	errs := make(chan string, requestCount)
	var wg sync.WaitGroup
	for i := 0; i < requestCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := bytes.NewBufferString(`{"type":"TEXT_TO_IMAGE","prompt":"cat ` + string(rune('a'+i)) + `","model":"mock-standard","params":{"count":1}}`)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/generation-tasks", body)
			req.Header.Set("Authorization", "Bearer "+token)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != http.StatusOK {
				errs <- res.Body.String()
				return
			}
			var task generationTask
			if err := json.NewDecoder(res.Body).Decode(&task); err != nil {
				errs <- err.Error()
				return
			}
			if task.ID == "" || len(task.ResultIDs) != 1 {
				errs <- "created task is missing ID or result"
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	tasksRes := authedRequest(t, handler, http.MethodGet, "/api/v1/generation-tasks", nil, token)
	if tasksRes.Code != http.StatusOK {
		t.Fatalf("list tasks status = %d, body = %s", tasksRes.Code, tasksRes.Body.String())
	}
	var tasks []generationTask
	if err := json.NewDecoder(tasksRes.Body).Decode(&tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks) != requestCount {
		t.Fatalf("tasks length = %d, want %d", len(tasks), requestCount)
	}
	seenTasks := map[string]bool{}
	seenAssets := map[string]bool{}
	for _, task := range tasks {
		if seenTasks[task.ID] {
			t.Fatalf("duplicate task ID %q", task.ID)
		}
		seenTasks[task.ID] = true
		for _, assetID := range task.ResultIDs {
			if seenAssets[assetID] {
				t.Fatalf("duplicate asset ID %q", assetID)
			}
			seenAssets[assetID] = true
		}
	}
}

func TestWriteFileAtomicallyReplacesTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomically(path, []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "new\n" {
		t.Fatalf("store content = %q, want %q", string(raw), "new\n")
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".store.json.*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files were not cleaned up: %v", matches)
	}
}

func TestUnsignedAuthFallbackRequiresExplicitDevFlag(t *testing.T) {
	t.Setenv("XIANZHI_DEV_AUTH_FALLBACK", "")
	t.Setenv("XIANZHI_ENV", "")
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := newWithStoreAndSessions(config.Config{
		Addr:           ":0",
		DataPath:       dataPath,
		StaticDir:      t.TempDir(),
		AdminStaticDir: t.TempDir(),
	}, newJSONStore(dataPath), nil)

	login := request(t, server.Handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"admin@xianzhi.ai","password":"Admin123!"}`))
	if login.Code != http.StatusServiceUnavailable {
		t.Fatalf("login without sessions status = %d, body = %s", login.Code, login.Body.String())
	}
}

func TestExplicitDevAuthFallbackStillWorksOutsideProduction(t *testing.T) {
	t.Setenv("XIANZHI_DEV_AUTH_FALLBACK", "true")
	t.Setenv("XIANZHI_ENV", "")
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := newWithStoreAndSessions(config.Config{
		Addr:           ":0",
		DataPath:       dataPath,
		StaticDir:      t.TempDir(),
		AdminStaticDir: t.TempDir(),
	}, newJSONStore(dataPath), nil)

	token := loginToken(t, server.Handler, "admin@xianzhi.ai", "Admin123!")
	me := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/auth/me", nil, token)
	if me.Code != http.StatusOK {
		t.Fatalf("dev fallback token auth/me = %d, body = %s", me.Code, me.Body.String())
	}
}

func TestLoginUpgradesLegacySHA256PasswordHash(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	raw := fmt.Sprintf(`{"users":[{"id":"user_legacy","email":"legacy@example.com","name":"Legacy","role":"MEMBER","status":"ACTIVE","passwordHash":%q,"planId":"plan_free"}],"counters":{}}`, legacySHA256PasswordHash("Legacy123!"))
	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	server := New(config.Config{
		Addr:           ":0",
		DataPath:       dataPath,
		StaticDir:      t.TempDir(),
		AdminStaticDir: t.TempDir(),
	})

	login := request(t, server.Handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"legacy@example.com","password":"Legacy123!"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("legacy login status = %d, body = %s", login.Code, login.Body.String())
	}
	data, err := os.ReadFile(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), legacySHA256PasswordHash("Legacy123!")) || !strings.Contains(string(data), `bcrypt:$2`) {
		t.Fatalf("legacy password hash was not upgraded: %s", string(data))
	}
}

func TestJSONAdminAPIsRequireSuperAdmin(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:           ":0",
		DataPath:       dataPath,
		StaticDir:      t.TempDir(),
		AdminStaticDir: t.TempDir(),
	})
	handler := server.Handler

	anonymous := request(t, handler, http.MethodGet, "/api/v1/admin/overview", nil)
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous admin status = %d, body = %s", anonymous.Code, anonymous.Body.String())
	}
	memberToken := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")
	member := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/overview", nil, memberToken)
	if member.Code != http.StatusForbidden {
		t.Fatalf("member admin status = %d, body = %s", member.Code, member.Body.String())
	}
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	admin := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/overview", nil, adminToken)
	if admin.Code != http.StatusOK {
		t.Fatalf("super admin status = %d, body = %s", admin.Code, admin.Body.String())
	}
}

func TestProductionConfigValidationRequiresSecurityEnv(t *testing.T) {
	t.Setenv("XIANZHI_DEV_AUTH_FALLBACK", "false")
	t.Setenv("XIANZHI_ALLOW_INSECURE_AUTH_TOKEN", "false")
	t.Setenv("XIANZHI_ENABLE_MOCK_LOGIN", "false")
	t.Setenv("XIANZHI_ALLOW_WECHAT_MOCK_LOGIN", "false")
	err := (config.Config{Environment: "production"}).ValidateProduction()
	if err == nil ||
		!strings.Contains(err.Error(), "DATABASE_URL") ||
		!strings.Contains(err.Error(), "REDIS_URL") ||
		!strings.Contains(err.Error(), "RABBITMQ_URL") ||
		!strings.Contains(err.Error(), "S3_ENDPOINT") ||
		!strings.Contains(err.Error(), "STORAGE_PUBLIC_ENDPOINT") ||
		!strings.Contains(err.Error(), "S3_ACCESS_KEY") ||
		!strings.Contains(err.Error(), "S3_SECRET_KEY") ||
		!strings.Contains(err.Error(), "S3_BUCKET") ||
		!strings.Contains(err.Error(), "STORAGE_MASTER_KEY") ||
		!strings.Contains(err.Error(), "PAYMENT_CALLBACK_SECRET") {
		t.Fatalf("unexpected production validation error: %v", err)
	}
	err = (config.Config{
		Environment:           "production",
		DatabaseURL:           "postgresql://example",
		RedisURL:              "redis://example",
		RabbitMQURL:           "amqp://example",
		S3Endpoint:            "http://s3.example",
		StoragePublicEndpoint: "https://storage.example",
		S3AccessKey:           "access",
		S3SecretKey:           "secret",
		S3Bucket:              "xianzhi-assets",
		StorageMasterKey:      "0123456789abcdef0123456789abcdef",
		PaymentCallbackSecret: "secret",
	}).ValidateProduction()
	if err != nil {
		t.Fatalf("complete production config failed validation: %v", err)
	}
	t.Setenv("XIANZHI_ALLOW_INSECURE_AUTH_TOKEN", "true")
	err = (config.Config{
		Environment:           "production",
		DatabaseURL:           "postgresql://example",
		RedisURL:              "redis://example",
		RabbitMQURL:           "amqp://example",
		S3Endpoint:            "http://s3.example",
		StoragePublicEndpoint: "https://storage.example",
		S3AccessKey:           "access",
		S3SecretKey:           "secret",
		S3Bucket:              "xianzhi-assets",
		StorageMasterKey:      "0123456789abcdef0123456789abcdef",
		PaymentCallbackSecret: "secret",
	}).ValidateProduction()
	if err == nil || !strings.Contains(err.Error(), "XIANZHI_ALLOW_INSECURE_AUTH_TOKEN") {
		t.Fatalf("production insecure auth flag validation error = %v", err)
	}
}

func TestChangePasswordPersistsForLogin(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:           ":0",
		DataPath:       dataPath,
		StaticDir:      t.TempDir(),
		AdminStaticDir: t.TempDir(),
	})
	handler := server.Handler

	login := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"admin@xianzhi.ai","password":"Admin123!"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("admin login status = %d, body = %s", login.Code, login.Body.String())
	}
	var loginBody struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(login.Body).Decode(&loginBody); err != nil {
		t.Fatal(err)
	}
	if loginBody.AccessToken == "" {
		t.Fatal("login response missing access token")
	}

	changeReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", bytes.NewBufferString(`{"currentPassword":"Admin123!","newPassword":"Admin456!"}`))
	changeReq.Header.Set("Authorization", "Bearer "+loginBody.AccessToken)
	changeRes := httptest.NewRecorder()
	handler.ServeHTTP(changeRes, changeReq)
	if changeRes.Code != http.StatusOK {
		t.Fatalf("change password status = %d, body = %s", changeRes.Code, changeRes.Body.String())
	}

	oldLogin := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"admin@xianzhi.ai","password":"Admin123!"}`))
	if oldLogin.Code != http.StatusUnauthorized {
		t.Fatalf("old password login status = %d, body = %s", oldLogin.Code, oldLogin.Body.String())
	}
	newLogin := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"admin@xianzhi.ai","password":"Admin456!"}`))
	if newLogin.Code != http.StatusOK {
		t.Fatalf("new password login status = %d, body = %s", newLogin.Code, newLogin.Body.String())
	}
}

func TestMemberCanUpdateOwnProfileWithoutChangingPoints(t *testing.T) {
	server := New(config.Config{
		Addr:           ":0",
		DataPath:       filepath.Join(t.TempDir(), "store.json"),
		StaticDir:      t.TempDir(),
		AdminStaticDir: t.TempDir(),
	})
	token := loginToken(t, server.Handler, "agent1@xianzhi.ai", "Agent123!")

	before := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/points/account", nil, token)
	if before.Code != http.StatusOK {
		t.Fatalf("points before profile update status = %d, body = %s", before.Code, before.Body.String())
	}
	var beforeBody struct {
		Account pointAccount `json:"account"`
	}
	if err := json.NewDecoder(before.Body).Decode(&beforeBody); err != nil {
		t.Fatal(err)
	}

	updated := authedRequest(t, server.Handler, http.MethodPatch, "/api/v1/member/profile", bytes.NewBufferString(`{"name":"代理一号更新","email":"agent1.updated@xianzhi.ai"}`), token)
	if updated.Code != http.StatusOK {
		t.Fatalf("profile update status = %d, body = %s", updated.Code, updated.Body.String())
	}
	profile := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/member/profile", nil, token)
	if profile.Code != http.StatusOK {
		t.Fatalf("profile after update status = %d, body = %s", profile.Code, profile.Body.String())
	}
	var profileBody struct {
		User adminUser `json:"user"`
	}
	if err := json.NewDecoder(profile.Body).Decode(&profileBody); err != nil {
		t.Fatal(err)
	}
	if profileBody.User.Name != "代理一号更新" || profileBody.User.Email != "agent1.updated@xianzhi.ai" {
		t.Fatalf("updated profile = %+v", profileBody.User)
	}

	after := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/points/account", nil, token)
	var afterBody struct {
		Account pointAccount `json:"account"`
	}
	if err := json.NewDecoder(after.Body).Decode(&afterBody); err != nil {
		t.Fatal(err)
	}
	if afterBody.Account.Available != beforeBody.Account.Available {
		t.Fatalf("points changed after profile update: before=%d after=%d", beforeBody.Account.Available, afterBody.Account.Available)
	}
}

type memoryAuthSessions struct {
	mu     sync.Mutex
	userID map[string]string
}

func newMemoryAuthSessions() *memoryAuthSessions {
	return &memoryAuthSessions{userID: map[string]string{}}
}

func (s *memoryAuthSessions) Put(_ context.Context, token string, userID string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userID[token] = userID
	return nil
}

func (s *memoryAuthSessions) UserID(_ context.Context, token string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	userID, ok := s.userID[token]
	return userID, ok, nil
}

func (s *memoryAuthSessions) Delete(_ context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.userID, token)
	return nil
}

func TestAuthSessionStoreLogoutRevokesToken(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	sessions := newMemoryAuthSessions()
	server := newWithStoreAndSessions(config.Config{
		Addr:           ":0",
		DataPath:       dataPath,
		StaticDir:      t.TempDir(),
		AdminStaticDir: t.TempDir(),
	}, newJSONStore(dataPath), sessions)
	handler := server.Handler

	login := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"agent1@xianzhi.ai","password":"Agent123!"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", login.Code, login.Body.String())
	}
	var loginBody struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(login.Body).Decode(&loginBody); err != nil {
		t.Fatal(err)
	}
	if loginBody.AccessToken == "" || loginBody.RefreshToken == "" {
		t.Fatal("login response missing access or refresh token")
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+loginBody.AccessToken)
	meRes := httptest.NewRecorder()
	handler.ServeHTTP(meRes, meReq)
	if meRes.Code != http.StatusOK {
		t.Fatalf("me status = %d, body = %s", meRes.Code, meRes.Body.String())
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewBufferString(`{"refreshToken":"`+loginBody.RefreshToken+`"}`))
	logoutReq.Header.Set("Content-Type", "application/json")
	logoutReq.Header.Set("Authorization", "Bearer "+loginBody.AccessToken)
	logoutRes := httptest.NewRecorder()
	handler.ServeHTTP(logoutRes, logoutReq)
	if logoutRes.Code != http.StatusOK {
		t.Fatalf("logout status = %d, body = %s", logoutRes.Code, logoutRes.Body.String())
	}

	revokedReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	revokedReq.Header.Set("Authorization", "Bearer "+loginBody.AccessToken)
	revokedRes := httptest.NewRecorder()
	handler.ServeHTTP(revokedRes, revokedReq)
	if revokedRes.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d, body = %s", revokedRes.Code, revokedRes.Body.String())
	}
	refreshAfterLogout := request(t, handler, http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refreshToken":"`+loginBody.RefreshToken+`"}`))
	if refreshAfterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("refresh token after logout status = %d, body = %s", refreshAfterLogout.Code, refreshAfterLogout.Body.String())
	}
}

func TestAuthRefreshTokenRotatesSession(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	sessions := newMemoryAuthSessions()
	server := newWithStoreAndSessions(config.Config{
		Addr:           ":0",
		DataPath:       dataPath,
		StaticDir:      t.TempDir(),
		AdminStaticDir: t.TempDir(),
	}, newJSONStore(dataPath), sessions)
	handler := server.Handler

	login := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"agent1@xianzhi.ai","password":"Agent123!"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", login.Code, login.Body.String())
	}
	var loginBody struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(login.Body).Decode(&loginBody); err != nil {
		t.Fatal(err)
	}
	if loginBody.AccessToken == "" || loginBody.RefreshToken == "" {
		t.Fatalf("login response missing tokens: %+v", loginBody)
	}

	var refreshCookie *http.Cookie
	for _, cookie := range login.Result().Cookies() {
		if cookie.Name == authRefreshCookieName {
			refreshCookie = cookie
			break
		}
	}
	if refreshCookie == nil || !refreshCookie.HttpOnly || refreshCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("login refresh cookie missing security attributes: %+v", refreshCookie)
	}
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	refreshReq.AddCookie(refreshCookie)
	refresh := httptest.NewRecorder()
	handler.ServeHTTP(refresh, refreshReq)
	if refresh.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, body = %s", refresh.Code, refresh.Body.String())
	}
	var refreshBody struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(refresh.Body).Decode(&refreshBody); err != nil {
		t.Fatal(err)
	}
	if refreshBody.AccessToken == "" || refreshBody.RefreshToken == "" {
		t.Fatalf("refresh response missing tokens: %+v", refreshBody)
	}
	if refreshBody.RefreshToken == loginBody.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}

	reuse := request(t, handler, http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refreshToken":"`+loginBody.RefreshToken+`"}`))
	if reuse.Code != http.StatusUnauthorized {
		t.Fatalf("old refresh token status = %d, body = %s", reuse.Code, reuse.Body.String())
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+refreshBody.AccessToken)
	meRes := httptest.NewRecorder()
	handler.ServeHTTP(meRes, meReq)
	if meRes.Code != http.StatusOK {
		t.Fatalf("me with refreshed token status = %d, body = %s", meRes.Code, meRes.Body.String())
	}
}

func TestAuthLogoutAllRevokesAccessAndRefreshTokens(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	sessions := newLocalAuthSessions()
	server := newWithStoreAndSessions(config.Config{
		Addr:           ":0",
		DataPath:       dataPath,
		StaticDir:      t.TempDir(),
		AdminStaticDir: t.TempDir(),
	}, newJSONStore(dataPath), sessions)
	handler := server.Handler

	login := func() struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	} {
		t.Helper()
		res := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"agent1@xianzhi.ai","password":"Agent123!"}`))
		if res.Code != http.StatusOK {
			t.Fatalf("login status = %d, body = %s", res.Code, res.Body.String())
		}
		var body struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
		}
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.AccessToken == "" || body.RefreshToken == "" {
			t.Fatalf("login response missing tokens: %+v", body)
		}
		return body
	}

	first := login()
	second := login()
	logoutAll := authedRequest(t, handler, http.MethodPost, "/api/v1/auth/logout-all", nil, first.AccessToken)
	if logoutAll.Code != http.StatusOK {
		t.Fatalf("logout all status = %d, body = %s", logoutAll.Code, logoutAll.Body.String())
	}

	me := authedRequest(t, handler, http.MethodGet, "/api/v1/auth/me", nil, second.AccessToken)
	if me.Code != http.StatusUnauthorized {
		t.Fatalf("second access token after logout all status = %d, body = %s", me.Code, me.Body.String())
	}
	refresh := request(t, handler, http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refreshToken":"`+second.RefreshToken+`"}`))
	if refresh.Code != http.StatusUnauthorized {
		t.Fatalf("second refresh token after logout all status = %d, body = %s", refresh.Code, refresh.Body.String())
	}
}

func assertStatus(t *testing.T, handler http.Handler, method string, path string, body *bytes.Buffer, want int) {
	t.Helper()
	res := request(t, handler, method, path, body)
	if res.Code != want {
		t.Fatalf("%s %s status = %d, want %d, body = %s", method, path, res.Code, want, res.Body.String())
	}
}

func assertAuthedStatus(t *testing.T, handler http.Handler, method string, path string, body *bytes.Buffer, token string, want int) {
	t.Helper()
	res := authedRequest(t, handler, method, path, body, token)
	if res.Code != want {
		t.Fatalf("%s %s status = %d, want %d, body = %s", method, path, res.Code, want, res.Body.String())
	}
}

func request(t *testing.T, handler http.Handler, method string, path string, body *bytes.Buffer) *httptest.ResponseRecorder {
	t.Helper()
	if body == nil {
		body = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, body)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func mobileRequest(t *testing.T, handler http.Handler, method string, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func authedRequest(t *testing.T, handler http.Handler, method string, path string, body *bytes.Buffer, token string) *httptest.ResponseRecorder {
	t.Helper()
	if body == nil {
		body = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func loginToken(t *testing.T, handler http.Handler, email string, password string) string {
	t.Helper()
	body := bytes.NewBufferString(`{"email":"` + email + `","password":"` + password + `"}`)
	res := request(t, handler, http.MethodPost, "/api/v1/auth/login", body)
	if res.Code != http.StatusOK {
		t.Fatalf("login %s status = %d, body = %s", email, res.Code, res.Body.String())
	}
	var payload struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.AccessToken == "" {
		t.Fatalf("login %s returned empty token", email)
	}
	return payload.AccessToken
}
