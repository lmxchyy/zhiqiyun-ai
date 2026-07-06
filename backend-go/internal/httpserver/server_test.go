package httpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

func TestGenerationTaskLifecycle(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
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

func TestVideoGenerationReturnsPendingAndCompletesAsync(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
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

func TestGenerationErrorMessageExtractsProviderMessage(t *testing.T) {
	raw := `CMECloud Seedance bridge failed: exit status 1: response: {"error":{"message":"当前账号处未订购seedance2.0模型资费包，或资费包已到期，请先订购后才能使用","type":"invalid_authentication_error"}}`
	want := "当前账号处未订购seedance2.0模型资费包，或资费包已到期，请先订购后才能使用"
	if got := generationErrorMessage(errors.New(raw)); got != want {
		t.Fatalf("generationErrorMessage() = %q, want %q", got, want)
	}
}

func TestAICapabilitySchemaValidationAndOverview(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

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
	if schemaPayload.ModuleCode != moduleImageGeneration || schemaPayload.ModelName != "mock-standard" || !fieldKeys["prompt"] || !fieldKeys["n"] || fieldKeys["duration"] {
		t.Fatalf("unexpected image schema payload: %+v", schemaPayload)
	}

	invalid := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks", bytes.NewBufferString(`{"module_code":"image_generation","prompt":"image prompt","model":"mock-standard","params":{"duration":5}}`), token)
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "duration") {
		t.Fatalf("cross-module param was not rejected: %d %s", invalid.Code, invalid.Body.String())
	}

	internalMeta := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks", bytes.NewBufferString(`{
		"module_code":"image_generation",
		"prompt":"image prompt with internal metadata",
		"model":"mock-standard",
		"params":{
			"n":1,
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
			"ratio":"16:9",
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

	create := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks", bytes.NewBufferString(`{"module_code":"image_generation","prompt":"image prompt","model":"mock-standard","params":{"n":2}}`), token)
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}
	var task generationTask
	if err := json.NewDecoder(create.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	if task.ModuleCode != moduleImageGeneration || task.BillingType != "per_image" || task.PointCost != 2 || len(task.ResultIDs) != 2 || task.FinalSchemaSnapshot == nil || task.LimitSnapshot == nil {
		t.Fatalf("task missing ai capability snapshot: %+v", task)
	}

	overview := request(t, handler, http.MethodGet, "/api/v1/admin/ai/overview", nil)
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
	if cost != 360 {
		t.Fatalf("doubao point cost = %d, want 360", cost)
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

func TestWebRoutesUseAdminBundle(t *testing.T) {
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

	server := New(config.Config{
		Addr:           ":0",
		DataPath:       filepath.Join(t.TempDir(), "store.json"),
		StaticDir:      userStaticDir,
		AdminStaticDir: adminStaticDir,
	})

	for _, path := range []string{"/", "/foo", "/app", "/app/workspace", "/agent", "/admin/"} {
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

	for _, path := range []string{"/login", "/register"} {
		t.Run(path, func(t *testing.T) {
			res := request(t, server.Handler, http.MethodGet, path, nil)
			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
			}
			if got := strings.TrimSpace(res.Body.String()); got != "USER_BUNDLE" {
				t.Fatalf("body = %q, want USER_BUNDLE", got)
			}
		})
	}

	assetRes := request(t, server.Handler, http.MethodGet, "/assets/login.js", nil)
	if assetRes.Code != http.StatusOK {
		t.Fatalf("/assets/login.js status = %d, body = %s", assetRes.Code, assetRes.Body.String())
	}
	if got := strings.TrimSpace(assetRes.Body.String()); got != "USER_ASSET" {
		t.Fatalf("/assets/login.js body = %q, want USER_ASSET", got)
	}

	res := request(t, server.Handler, http.MethodGet, "/user", nil)
	if res.Code != http.StatusNotFound {
		t.Fatalf("/user status = %d, body = %s", res.Code, res.Body.String())
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

	pointsBefore := authedRequest(t, handler, http.MethodGet, "/api/v1/points/account", nil, token)
	if pointsBefore.Code != http.StatusOK || !strings.Contains(pointsBefore.Body.String(), `"available":959`) {
		t.Fatalf("initial points response = %d %s", pointsBefore.Code, pointsBefore.Body.String())
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
	if pointsAfter.Code != http.StatusOK || !strings.Contains(pointsAfter.Body.String(), `"available":957`) {
		t.Fatalf("deducted points response = %d %s", pointsAfter.Code, pointsAfter.Body.String())
	}

	assets := authedRequest(t, handler, http.MethodGet, "/api/v1/assets", nil, token)
	if assets.Code != http.StatusOK || !strings.Contains(assets.Body.String(), task.ResultIDs[0]) || !strings.Contains(assets.Body.String(), `"mediaType":"image"`) {
		t.Fatalf("assets not visible after generation: %d %s", assets.Code, assets.Body.String())
	}

	customers := request(t, handler, http.MethodGet, "/api/v1/admin/customers", nil)
	if customers.Code != http.StatusOK || !strings.Contains(customers.Body.String(), `"pointsAvailable":957`) || !strings.Contains(customers.Body.String(), "演示用户") {
		t.Fatalf("admin customers did not reflect deducted points: %d %s", customers.Code, customers.Body.String())
	}

	adminTasks := request(t, handler, http.MethodGet, "/api/v1/admin/generation-tasks", nil)
	adminTaskBody := adminTasks.Body.String()
	if adminTasks.Code != http.StatusOK || !strings.Contains(adminTaskBody, task.ID) || !strings.Contains(adminTaskBody, `"pointCost":2`) || !strings.Contains(adminTaskBody, task.ResultIDs[0]) {
		t.Fatalf("admin generation tasks missing closed-loop data: %d %s", adminTasks.Code, adminTaskBody)
	}

	overview := request(t, handler, http.MethodGet, "/api/v1/admin/overview", nil)
	if overview.Code != http.StatusOK || !strings.Contains(overview.Body.String(), `"generatedAssets":2`) {
		t.Fatalf("admin overview missing generated assets: %d %s", overview.Code, overview.Body.String())
	}

	usage := request(t, handler, http.MethodGet, "/api/v1/admin/usage", nil)
	if usage.Code != http.StatusOK || !strings.Contains(usage.Body.String(), `"apiCalls":1`) || !strings.Contains(usage.Body.String(), `"assets":2`) {
		t.Fatalf("admin usage missing generation counters: %d %s", usage.Code, usage.Body.String())
	}

	billing := request(t, handler, http.MethodGet, "/api/v1/admin/billing/events", nil)
	billingBody := billing.Body.String()
	if billing.Code != http.StatusOK || !strings.Contains(billingBody, task.ID) || !strings.Contains(billingBody, `"balanceBefore":959`) || !strings.Contains(billingBody, `"balanceAfter":957`) {
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

func TestPPTGenerationCreatesUsageEvent(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
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

	billing := request(t, handler, http.MethodGet, "/api/v1/admin/billing/events", nil)
	if billing.Code != http.StatusOK || !strings.Contains(billing.Body.String(), createResp.TaskID) || !strings.Contains(billing.Body.String(), billingMetricPPTGenerate) {
		t.Fatalf("admin billing events missing ppt usage: %d %s", billing.Code, billing.Body.String())
	}
}

func TestPPTImageGenerationCreatesImageUsageEvent(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
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
			{"id":"user_000002","email":"demo@xianzhi.ai","name":"演示用户","role":"MEMBER","status":"ACTIVE","planId":"plan_month","referredBy":"user_000003"},
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
	if res.Code != http.StatusOK || !strings.Contains(body, `"directCustomers":1`) || !strings.Contains(body, `"childAgents":1`) || !strings.Contains(body, `"totalCommission":990`) || !strings.Contains(body, "演示用户") || !strings.Contains(body, `"inviteLink":"http://localhost:3100/register?invite=EAST001"`) {
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
	if resAfterRegister.Code != http.StatusOK || !strings.Contains(resAfterRegister.Body.String(), "邀请注册用户") || !strings.Contains(resAfterRegister.Body.String(), `"directCustomers":2`) {
		t.Fatalf("channel center after register = %d %s", resAfterRegister.Code, resAfterRegister.Body.String())
	}
	adminCustomers := request(t, handler, http.MethodGet, "/api/v1/admin/customers", nil)
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
func TestCreateChannelAgentPersistsUserAndTree(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler

	createL1 := request(t, handler, http.MethodPost, "/api/v1/admin/channel-agents", bytes.NewBufferString(`{"name":"测试推广员","email":"agent-new@example.com","level":1,"inviteCode":"NEW001","status":"ACTIVE","available":88}`))
	if createL1.Code != http.StatusOK {
		t.Fatalf("create level 1 channel agent status = %d, body = %s", createL1.Code, createL1.Body.String())
	}
	var l1Body struct {
		Item map[string]any `json:"item"`
		User adminUser      `json:"user"`
	}
	if err := json.NewDecoder(createL1.Body).Decode(&l1Body); err != nil {
		t.Fatal(err)
	}
	if l1Body.Item["id"] == "" || l1Body.User.Role != "AGENT_L1" {
		t.Fatalf("unexpected level 1 body: %+v", l1Body)
	}

	parentID, _ := l1Body.Item["id"].(string)
	createL2 := request(t, handler, http.MethodPost, "/api/v1/admin/channel-agents", bytes.NewBufferString(`{"name":"测试初级代理商","email":"agent-child@example.com","level":2,"parentId":"`+parentID+`","status":"ACTIVE"}`))
	if createL2.Code != http.StatusOK {
		t.Fatalf("create level 2 channel agent status = %d, body = %s", createL2.Code, createL2.Body.String())
	}
	createL3 := request(t, handler, http.MethodPost, "/api/v1/admin/channel-agents", bytes.NewBufferString(`{"name":"测试高级代理商","email":"agent-senior@example.com","level":3,"status":"ACTIVE"}`))
	if createL3.Code != http.StatusOK || !strings.Contains(createL3.Body.String(), `"role":"AGENT_L3"`) {
		t.Fatalf("create level 3 channel agent status = %d, body = %s", createL3.Code, createL3.Body.String())
	}

	login := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"agent-new@example.com","password":"Agent123!"}`))
	if login.Code != http.StatusOK || !strings.Contains(login.Body.String(), `"defaultModule":"dashboard"`) || !strings.Contains(login.Body.String(), `"workspace":"user"`) || !strings.Contains(login.Body.String(), `"channel.dashboard"`) {
		t.Fatalf("created agent login failed: %d %s", login.Code, login.Body.String())
	}

	tree := request(t, handler, http.MethodGet, "/api/v1/admin/channel-agents/tree", nil)
	body := tree.Body.String()
	for _, want := range []string{"测试推广员", "测试初级代理商", "测试高级代理商", "NEW001"} {
		if !strings.Contains(body+createL1.Body.String(), want) {
			t.Fatalf("channel tree or create response missing %q: tree=%s create=%s", want, body, createL1.Body.String())
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
			{"id":"asset_demo2","userId":"user_000010","taskId":"task_demo2","name":"demo2 asset","mediaType":"image","url":"data:image/svg+xml;base64,PHN2Zy8+","favorite":false,"metadata":{}}
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
		assertStatus(t, server.Handler, http.MethodGet, path, nil, http.StatusOK)
	}

	overviewRes := request(t, server.Handler, http.MethodGet, "/api/v1/admin/overview", nil)
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

	createCustomer := request(t, handler, http.MethodPost, "/api/v1/admin/customers", bytes.NewBufferString(`{"name":"测试客户","email":"customer@example.com","planId":"plan_month","available":6000}`))
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
	assertStatus(t, handler, http.MethodPatch, updateCustomerPath, bytes.NewBufferString(`{"status":"DISABLED","planId":"plan_year","available":7000}`), http.StatusOK)

	createOrder := request(t, handler, http.MethodPost, "/api/v1/admin/orders", bytes.NewBufferString(`{"userId":"`+customerBody.Item.ID+`","planId":"plan_year","amountCents":89900}`))
	if createOrder.Code != http.StatusOK {
		t.Fatalf("create order status = %d, body = %s", createOrder.Code, createOrder.Body.String())
	}
	var orderBody struct {
		Item adminOrder `json:"item"`
	}
	if err := json.NewDecoder(createOrder.Body).Decode(&orderBody); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, handler, http.MethodPost, "/api/v1/admin/orders/"+orderBody.Item.ID+"/mark-paid", bytes.NewBuffer(nil), http.StatusOK)
	assertStatus(t, handler, http.MethodPost, "/api/v1/admin/orders/"+orderBody.Item.ID+"/renew", bytes.NewBuffer(nil), http.StatusOK)

	customers := request(t, handler, http.MethodGet, "/api/v1/admin/customers", nil)
	if !strings.Contains(customers.Body.String(), `"status":"DISABLED"`) || !strings.Contains(customers.Body.String(), `"pointsAvailable":107000`) {
		t.Fatalf("customer update was not persisted: %s", customers.Body.String())
	}
	orders := request(t, handler, http.MethodGet, "/api/v1/admin/orders", nil)
	if !strings.Contains(orders.Body.String(), `"status":"PAID"`) || !strings.Contains(orders.Body.String(), `"status":"PENDING"`) {
		t.Fatalf("order mutations were not persisted: %s", orders.Body.String())
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

	createOrder := request(t, handler, http.MethodPost, "/api/v1/admin/orders", bytes.NewBufferString(`{"userId":"user_000002","planId":"recharge_100","amountCents":10000}`))
	if createOrder.Code != http.StatusOK {
		t.Fatalf("create recharge order status = %d, body = %s", createOrder.Code, createOrder.Body.String())
	}
	var orderBody struct {
		Item adminOrder `json:"item"`
	}
	if err := json.NewDecoder(createOrder.Body).Decode(&orderBody); err != nil {
		t.Fatal(err)
	}
	markPaid := request(t, handler, http.MethodPost, "/api/v1/admin/orders/"+orderBody.Item.ID+"/mark-paid", bytes.NewBuffer(nil))
	if markPaid.Code != http.StatusOK {
		t.Fatalf("mark recharge paid status = %d, body = %s", markPaid.Code, markPaid.Body.String())
	}
	body := markPaid.Body.String()
	for _, want := range []string{`"status":"PAID"`, `"orderType":"COMPUTE_RECHARGE"`, `"rechargePoints":1000`, `"newapiSyncStatus":"READY"`, `"newapiGroup":"生图备份"`, `"newapiKeyId":"key_user_000002"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("paid recharge response missing %q: %s", want, body)
		}
	}
	customers := request(t, handler, http.MethodGet, "/api/v1/admin/customers", nil)
	customerBody := customers.Body.String()
	if customers.Code != http.StatusOK || !strings.Contains(customerBody, `"pointsAvailable":1959`) || !strings.Contains(customerBody, `"modelGroup":"生图备份"`) || !strings.Contains(customerBody, `"modelApiKeyId":"key_user_000002"`) {
		t.Fatalf("recharge points not reflected: %d %s", customers.Code, customers.Body.String())
	}
	commissions := request(t, handler, http.MethodGet, "/api/v1/admin/commissions", nil)
	commissionBody := commissions.Body.String()
	if commissions.Code != http.StatusOK || !strings.Contains(commissionBody, orderBody.Item.ID) || !strings.Contains(commissionBody, `"amountCents":800`) || !strings.Contains(commissionBody, `"source":"compute_recharge"`) || !strings.Contains(commissionBody, `"ruleId":"rule_recharge_l1_direct"`) {
		t.Fatalf("recharge commission missing: %d %s", commissions.Code, commissionBody)
	}
	markPaidAgain := request(t, handler, http.MethodPost, "/api/v1/admin/orders/"+orderBody.Item.ID+"/mark-paid", bytes.NewBuffer(nil))
	if markPaidAgain.Code != http.StatusOK {
		t.Fatalf("repeat mark paid status = %d, body = %s", markPaidAgain.Code, markPaidAgain.Body.String())
	}
	customersAfterRepeat := request(t, handler, http.MethodGet, "/api/v1/admin/customers", nil)
	if strings.Count(customersAfterRepeat.Body.String(), `"pointsAvailable":1959`) == 0 || strings.Contains(customersAfterRepeat.Body.String(), `"pointsAvailable":2959`) {
		t.Fatalf("repeat mark paid was not idempotent: %s", customersAfterRepeat.Body.String())
	}
	commissionsAfterRepeat := request(t, handler, http.MethodGet, "/api/v1/admin/commissions", nil)
	if strings.Count(commissionsAfterRepeat.Body.String(), `"ruleId":"rule_recharge_l1_direct"`) != 1 {
		t.Fatalf("repeat mark paid duplicated commissions: %s", commissionsAfterRepeat.Body.String())
	}
	walletRecords := request(t, handler, http.MethodGet, "/api/v1/admin/marketing/wallet-records", nil)
	if walletRecords.Code != http.StatusOK || !strings.Contains(walletRecords.Body.String(), `"bizType":"COMMISSION_INCOME"`) || !strings.Contains(walletRecords.Body.String(), `"rule_recharge_l1_direct"`) {
		t.Fatalf("wallet records missing recharge commission: %d %s", walletRecords.Code, walletRecords.Body.String())
	}
	statements := request(t, handler, http.MethodGet, "/api/v1/admin/marketing/settlement-statements", nil)
	if statements.Code != http.StatusOK || !strings.Contains(statements.Body.String(), `"pendingCents":800`) {
		t.Fatalf("settlement statements missing pending commission: %d %s", statements.Code, statements.Body.String())
	}
}

func TestRechargeCommissionUsesUpdatedRuleRate(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler

	updateRule := request(t, handler, http.MethodPatch, "/api/v1/admin/marketing/commission-rules/rule_recharge_l1_direct", bytes.NewBufferString(`{"name":"L1 推广员点数包返佣","orderType":"COMPUTE_RECHARGE","earnerRole":"AGENT_L1","relationDepth":1,"fixedAmountCents":0,"rate":0.15,"maxTotalRate":0.2,"status":"ACTIVE"}`))
	if updateRule.Code != http.StatusOK || !strings.Contains(updateRule.Body.String(), `"rate":0.15`) {
		t.Fatalf("update commission rule status = %d, body = %s", updateRule.Code, updateRule.Body.String())
	}
	rules := request(t, handler, http.MethodGet, "/api/v1/admin/marketing/commission-rules", nil)
	if rules.Code != http.StatusOK || !strings.Contains(rules.Body.String(), `"rate":0.15`) {
		t.Fatalf("updated commission rule not visible: %d %s", rules.Code, rules.Body.String())
	}

	createOrder := request(t, handler, http.MethodPost, "/api/v1/admin/orders", bytes.NewBufferString(`{"userId":"user_000002","planId":"recharge_100","amountCents":10000}`))
	if createOrder.Code != http.StatusOK {
		t.Fatalf("create recharge order status = %d, body = %s", createOrder.Code, createOrder.Body.String())
	}
	var orderBody struct {
		Item adminOrder `json:"item"`
	}
	if err := json.NewDecoder(createOrder.Body).Decode(&orderBody); err != nil {
		t.Fatal(err)
	}
	markPaid := request(t, handler, http.MethodPost, "/api/v1/admin/orders/"+orderBody.Item.ID+"/mark-paid", bytes.NewBuffer(nil))
	if markPaid.Code != http.StatusOK {
		t.Fatalf("mark recharge paid status = %d, body = %s", markPaid.Code, markPaid.Body.String())
	}
	commissions := request(t, handler, http.MethodGet, "/api/v1/admin/commissions", nil)
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

	createOrder := request(t, handler, http.MethodPost, "/api/v1/admin/orders", bytes.NewBufferString(`{"userId":"user_customer","planId":"recharge_100","amountCents":10000}`))
	if createOrder.Code != http.StatusOK {
		t.Fatalf("create recharge order status = %d, body = %s", createOrder.Code, createOrder.Body.String())
	}
	var orderBody struct {
		Item adminOrder `json:"item"`
	}
	if err := json.NewDecoder(createOrder.Body).Decode(&orderBody); err != nil {
		t.Fatal(err)
	}
	markPaid := request(t, handler, http.MethodPost, "/api/v1/admin/orders/"+orderBody.Item.ID+"/mark-paid", bytes.NewBuffer(nil))
	if markPaid.Code != http.StatusOK {
		t.Fatalf("mark recharge paid status = %d, body = %s", markPaid.Code, markPaid.Body.String())
	}
	commissions := request(t, handler, http.MethodGet, "/api/v1/admin/commissions", nil)
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

	assertStatus(t, handler, http.MethodPatch, "/api/v1/admin/system/settings", bytes.NewBufferString(`{"brand":{"name":"先知主控","domain":"admin.example.com","logo":"控"},"payments":[{"channel":"manual","status":"ACTIVE"}],"permissions":["SUPER_ADMIN","FINANCE"]}`), http.StatusOK)
	assertStatus(t, handler, http.MethodPost, "/api/v1/admin/api/provider-channels", bytes.NewBufferString(`{"name":"测试上游","baseUrl":"https://provider.example.com/v1","status":"ACTIVE","priority":30,"models":["gpt-image-2"]}`), http.StatusOK)
	assertStatus(t, handler, http.MethodPatch, "/api/v1/admin/api/models/model_gpt_image_2", bytes.NewBufferString(`{"name":"OpenAI 图像模型","capability":"IMAGE","billingMode":"PER_REQUEST","fixedQuota":12,"modelRatio":1,"completionRatio":1,"status":"ACTIVE"}`), http.StatusOK)
	assertStatus(t, handler, http.MethodPost, "/api/v1/admin/api/keys", bytes.NewBufferString(`{"customer":"测试客户","status":"ACTIVE","models":["gpt-image-2"],"quotaLimit":50000}`), http.StatusOK)
	assertStatus(t, handler, http.MethodPatch, "/api/v1/admin/customer-groups/group_vip", bytes.NewBufferString(`{"name":"vip","ratio":0.7,"models":["gpt-image-2"],"description":"测试倍率"}`), http.StatusOK)

	system := request(t, handler, http.MethodGet, "/api/v1/admin/system/settings", nil)
	body := system.Body.String()
	for _, want := range []string{"先知主控", "admin.example.com", "测试上游", `"fixedQuota":12`, "测试客户", `"ratio":0.7`} {
		if !strings.Contains(body, want) {
			t.Fatalf("system mutation response missing %q: %s", want, body)
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

	assertStatus(t, handler, http.MethodPost, "/api/v1/admin/ai/models", bytes.NewBufferString(`{
		"model_name":"ppt-maker-pro",
		"model_type":"text",
		"provider":"NewAPI",
		"module_code":"ppt_generation",
		"capability_code":["ppt_outline","ppt_content","ppt_export"],
		"fallback_model":"",
		"sort_weight":12,
		"allow_fallback_switch":true,
		"status":"ACTIVE"
	}`), http.StatusOK)

	overview := request(t, handler, http.MethodGet, "/api/v1/admin/ai/overview", nil)
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

func TestAdminUsageAndCommissionOperations(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	raw := `{
		"users":[{"id":"user_000002","email":"demo@xianzhi.ai","name":"演示用户","role":"MEMBER","status":"ACTIVE","planId":"plan_month"},{"id":"user_000003","email":"agent1@xianzhi.ai","name":"华东推广员","role":"AGENT_L1","status":"ACTIVE","planId":"plan_free"}],
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

	usage := request(t, handler, http.MethodGet, "/api/v1/admin/usage?product=Agent", nil)
	if usage.Code != http.StatusOK || !strings.Contains(usage.Body.String(), `"product":"Agent"`) || strings.Contains(usage.Body.String(), "GEO 任务") {
		t.Fatalf("usage filter failed: status = %d, body = %s", usage.Code, usage.Body.String())
	}
	export := request(t, handler, http.MethodGet, "/api/v1/admin/usage/export?product=Agent", nil)
	if export.Code != http.StatusOK || !strings.Contains(export.Body.String(), "Agent") || !strings.Contains(export.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("usage export failed: status = %d, type = %s, body = %s", export.Code, export.Header().Get("Content-Type"), export.Body.String())
	}

	createCommission := request(t, handler, http.MethodPost, "/api/v1/admin/commissions", bytes.NewBufferString(`{"orderId":"order_000001","agentId":"channel_000001","amountCents":990,"rate":0.1}`))
	if createCommission.Code != http.StatusOK {
		t.Fatalf("create commission status = %d, body = %s", createCommission.Code, createCommission.Body.String())
	}
	createWithdrawal := request(t, handler, http.MethodPost, "/api/v1/admin/withdrawals", bytes.NewBufferString(`{"agentId":"channel_000001","amountCents":500}`))
	if createWithdrawal.Code != http.StatusOK {
		t.Fatalf("create withdrawal status = %d, body = %s", createWithdrawal.Code, createWithdrawal.Body.String())
	}
	var withdrawalBody struct {
		Item adminWithdrawal `json:"item"`
	}
	if err := json.NewDecoder(createWithdrawal.Body).Decode(&withdrawalBody); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, handler, http.MethodPost, "/api/v1/admin/withdrawals/"+withdrawalBody.Item.ID+"/approve", bytes.NewBuffer(nil), http.StatusOK)

	commissions := request(t, handler, http.MethodGet, "/api/v1/admin/commissions", nil)
	body := commissions.Body.String()
	for _, want := range []string{`"amountCents":990`, `"amountCents":500`, `"status":"APPROVED"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("commissions response missing %q: %s", want, body)
		}
	}
}

func TestConcurrentGenerationTaskCreatesKeepUniqueIDs(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
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
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(login.Body).Decode(&loginBody); err != nil {
		t.Fatal(err)
	}
	if loginBody.AccessToken == "" {
		t.Fatal("login response missing access token")
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+loginBody.AccessToken)
	meRes := httptest.NewRecorder()
	handler.ServeHTTP(meRes, meReq)
	if meRes.Code != http.StatusOK {
		t.Fatalf("me status = %d, body = %s", meRes.Code, meRes.Body.String())
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
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
