package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
	imageprovider "xianzhi-ai/backend-go/internal/provider/image"
)

func TestImageRequestCountDoesNotCreateUnsupportedSchemaParameter(t *testing.T) {
	req := generation.CreateRequest{
		ModuleCode: moduleImageGeneration,
		Params:     map[string]any{"count": 1},
	}

	normalizeRequestParamAliases(&req)

	if _, exists := req.Params["n"]; exists {
		t.Fatalf("image request count created unsupported schema parameter n: %+v", req.Params)
	}
}

func TestDefaultImageSchemasMatchBuiltInProviderCapabilities(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	schemas := defaultAIParameterSchemas(now)
	tests := []struct {
		model        string
		sizes        []any
		qualities    []any
		countOptions []any
	}{
		{model: "mock-standard", sizes: []any{"1920x1080"}},
		{model: "gpt-image-2", sizes: gptImage2SizeOptions(), qualities: []any{"auto", "low", "medium", "high"}, countOptions: []any{float64(1), float64(2), float64(3), float64(4)}},
		{model: "HY-Image-3.0-Plus-4090-Tob-v1.0", sizes: []any{"1024x1024", "1280x1280", "1280x720", "720x1280"}},
		{model: "HY-Image-v3.0-I2I-ToB-v1.0.1", sizes: []any{"1024x1024", "1280x1280", "1280x720", "720x1280"}},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			schema := findAIParameterSchema(schemas, moduleImageGeneration, tt.model)
			if schema.ID == "" || schema.ModelName != tt.model {
				t.Fatalf("schema = %+v, want exact model %s", schema, tt.model)
			}
			fields := imageContractFields(schema.SchemaJSON.Fields)
			if !reflect.DeepEqual(fields["size"].Options, tt.sizes) {
				t.Fatalf("size options = %#v, want %#v", fields["size"].Options, tt.sizes)
			}
			if !reflect.DeepEqual(fields["quality"].Options, tt.qualities) {
				t.Fatalf("quality options = %#v, want %#v", fields["quality"].Options, tt.qualities)
			}
			countField, hasCount := fields["n"]
			if tt.countOptions == nil {
				if hasCount {
					t.Fatalf("unsupported n field was declared: %+v", countField)
				}
			} else if !hasCount || !reflect.DeepEqual(countField.Options, tt.countOptions) {
				t.Fatalf("n options = %#v, want %#v", countField.Options, tt.countOptions)
			}
		})
	}
}

func TestNormalizeAICapabilityDefaultsPreservesCustomImageSchemaAndAlignsGPTImage(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	staleMock := defaultAIParameterSchemas(now)[0]
	staleMock.SchemaJSON.Fields = []adminAIParameterField{
		{Key: "prompt", Type: "textarea", Required: true},
		{Key: "size", Type: "select", Required: true, Default: "1024x1024", Options: anyOptions("1024x1024", "1024x1536", "1536x1024")},
		{Key: "custom_admin_field", Type: "text", UserEditable: true, Visible: true},
	}
	staleGPT := findAIParameterSchema(defaultAIParameterSchemas(now), moduleImageGeneration, "gpt-image-2")
	staleGPT.SchemaJSON.Fields = []adminAIParameterField{
		{Key: "prompt", Type: "textarea", Required: true},
		{Key: "size", Type: "select", Required: true, Default: "1024x1024", Options: anyOptions("1024x1024", "1024x1536", "1536x1024")},
		{Key: "quality", Type: "select", Required: true, Default: "standard", Options: anyOptions("standard", "high")},
		{Key: "n", Type: "number", Required: true, Default: float64(1), Options: anyOptions(float64(1), float64(2), float64(4))},
		{Key: "seed", Type: "number", UserEditable: true, Visible: true},
		{Key: "custom_admin_field", Type: "text", UserEditable: true, Visible: true},
	}
	data := normalizeAICapabilityDefaults(adminPlatformData{
		AIModules: defaultAIModules(now), AIModels: defaultAIModels(now),
		AIParameterSchemas: []adminAIParameterSchema{staleMock, staleGPT},
		TenantModuleLimits: defaultTenantModuleLimits(now), BillingRules: defaultBillingRules(now),
	})
	mockFields := imageContractFields(findExactAIParameterSchema(data.AIParameterSchemas, moduleImageGeneration, "mock-standard").SchemaJSON.Fields)
	if !reflect.DeepEqual(mockFields["size"].Options, []any{"1024x1024", "1024x1536", "1536x1024"}) {
		t.Fatalf("custom mock size options were overwritten: %#v", mockFields["size"].Options)
	}
	if _, ok := mockFields["custom_admin_field"]; !ok {
		t.Fatal("custom mock admin field was dropped")
	}
	gptFields := imageContractFields(findExactAIParameterSchema(data.AIParameterSchemas, moduleImageGeneration, "gpt-image-2").SchemaJSON.Fields)
	if !reflect.DeepEqual(gptFields["size"].Options, gptImage2SizeOptions()) {
		t.Fatalf("gpt size options = %#v", gptFields["size"].Options)
	}
	if !reflect.DeepEqual(gptFields["quality"].Options, []any{"auto", "low", "medium", "high"}) {
		t.Fatalf("gpt quality options = %#v", gptFields["quality"].Options)
	}
	if !reflect.DeepEqual(gptFields["n"].Options, []any{float64(1), float64(2), float64(3), float64(4)}) {
		t.Fatalf("gpt n options = %#v", gptFields["n"].Options)
	}
	if _, ok := gptFields["seed"]; ok {
		t.Fatal("unsupported gpt seed field survived alignment")
	}
	if _, ok := gptFields["custom_admin_field"]; !ok {
		t.Fatal("custom gpt admin field was dropped")
	}
}

func TestNormalizeAICapabilityDefaultsAlignsStaleGPTImageQualityLimits(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	data := normalizeAICapabilityDefaults(adminPlatformData{
		AIModules: defaultAIModules(now), AIModels: defaultAIModels(now),
		AIParameterSchemas: defaultAIParameterSchemas(now),
		TenantModuleLimits: []adminTenantModuleLimit{
			{ID: "limit_stale_image", TenantID: "default", ModuleCode: moduleImageGeneration, LimitJSON: map[string]any{"quality": map[string]any{"allowed": []any{"standard", "high"}}}, Status: "ACTIVE"},
			{ID: "limit_plan_free_image", TenantID: "default", PackageID: "plan_free", ModuleCode: moduleImageGeneration, LimitJSON: map[string]any{"quality": map[string]any{"allowed": []any{"standard"}}}, Status: "ACTIVE"},
		},
		BillingRules: defaultBillingRules(now),
	})
	defaultLimit := effectiveTenantModuleLimit(data.TenantModuleLimits, adminUser{PlanID: "plan_month"}, moduleImageGeneration, "gpt-image-2")
	quality, _ := mapValue(defaultLimit.LimitJSON["quality"])
	if !reflect.DeepEqual(quality["allowed"], []any{"auto", "low", "medium", "high"}) {
		t.Fatalf("stale paid quality allowed = %#v", quality["allowed"])
	}
	freeLimit := effectiveTenantModuleLimit(data.TenantModuleLimits, adminUser{PlanID: "plan_free"}, moduleImageGeneration, "mock-standard")
	freeQuality, _ := mapValue(freeLimit.LimitJSON["quality"])
	if !reflect.DeepEqual(freeQuality["allowed"], []any{"standard"}) {
		t.Fatalf("free mock quality allowed was rewritten: %#v", freeQuality["allowed"])
	}
}

func TestStaleGPTImageQualityLimitDoesNotRewriteAutoToStandard(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	data := normalizeAICapabilityDefaults(adminPlatformData{
		AIModules: defaultAIModules(now), AIModels: defaultAIModels(now),
		AIParameterSchemas: defaultAIParameterSchemas(now),
		TenantModuleLimits: []adminTenantModuleLimit{
			{
				ID: "limit_stale_camel", TenantID: "default", ModuleCode: moduleImageGeneration, Status: "ACTIVE",
				LimitJSONCamel: map[string]any{
					"models":  map[string]any{"allowed": []any{"gpt-image-2"}},
					"quality": map[string]any{"allowed": []any{"standard", "high"}},
					"n":       map[string]any{"max": float64(4)},
				},
			},
		},
		BillingRules: defaultBillingRules(now),
	})
	user := adminUser{ID: "user", Role: "MEMBER", PlanID: "plan_month"}
	service := api{}
	for _, tc := range []struct {
		size    string
		quality string
	}{
		{"1024x1024", "auto"},
		{"2048x2048", "auto"},
		{"2048x2048", "low"},
		{"2048x2048", "medium"},
		{"2048x2048", "high"},
	} {
		prepared, err := service.prepareGenerationRequest(data, user, generation.CreateRequest{
			Type: "TEXT_TO_IMAGE", ModuleCode: moduleImageGeneration, Model: "gpt-image-2", Prompt: "gpt image",
			Params: map[string]any{"size": tc.size, "quality": tc.quality, "n": float64(1)},
		})
		if err != nil {
			t.Fatalf("%s %s prepare: %v", tc.size, tc.quality, err)
		}
		if prepared.Params["quality"] != tc.quality {
			t.Fatalf("%s %s quality = %v, want unchanged", tc.size, tc.quality, prepared.Params["quality"])
		}
		if prepared.Params["size"] != tc.size {
			t.Fatalf("%s size = %v", tc.size, prepared.Params["size"])
		}
		if err := imageprovider.ValidateGPTImageQuality(prepared.Params["quality"]); err != nil {
			t.Fatalf("%s %s provider quality: %v", tc.size, tc.quality, err)
		}
		if err := imageprovider.ValidateGPTImageSize(prepared.Params["size"]); err != nil {
			t.Fatalf("%s provider size: %v", tc.size, err)
		}
	}
}

func TestGPTImageQualityAliasesMapToOfficialVocabulary(t *testing.T) {
	data := normalizeAICapabilityDefaults(adminPlatformData{})
	user := adminUser{ID: "user", Role: "MEMBER", PlanID: "plan_month"}
	service := api{}
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"standard", "low"},
		{"hd", "high"},
	} {
		prepared, err := service.prepareGenerationRequest(data, user, generation.CreateRequest{
			Type: "TEXT_TO_IMAGE", ModuleCode: moduleImageGeneration, Model: "gpt-image-2", Prompt: "gpt image",
			Params: map[string]any{"size": "2048x2048", "quality": tc.input, "n": float64(1)},
		})
		if err != nil {
			t.Fatalf("%s prepare: %v", tc.input, err)
		}
		if prepared.Params["quality"] != tc.want {
			t.Fatalf("%s quality = %v, want %s", tc.input, prepared.Params["quality"], tc.want)
		}
		if err := imageprovider.ValidateGPTImageQuality(prepared.Params["quality"]); err != nil {
			t.Fatalf("%s provider quality: %v", tc.input, err)
		}
	}
	_, err := service.prepareGenerationRequest(data, user, generation.CreateRequest{
		Type: "TEXT_TO_IMAGE", ModuleCode: moduleImageGeneration, Model: "gpt-image-2", Prompt: "gpt image",
		Params: map[string]any{"size": "2048x2048", "quality": "ultra", "n": float64(1)},
	})
	if err == nil {
		t.Fatal("unsupported quality ultra should fail prepare")
	}
}

func TestImageModuleSchemaDoesNotFallbackToAnotherModel(t *testing.T) {
	data := normalizeAICapabilityDefaults(adminPlatformData{})
	data.AIModels = append(data.AIModels, adminAIModel{
		ID: "ai_model_no_schema", ModelName: "no-schema-image", ModelType: "image", Provider: "test",
		CapabilityCode: []string{"text_to_image"}, ModuleCode: moduleImageGeneration, Status: "ACTIVE",
		FallbackModel: "mock-standard", AllowFallbackSwitch: true,
	})
	_, err := resolveClientModuleSchema(data, adminUser{ID: "user", Role: "MEMBER", PlanID: "plan_month"}, moduleImageGeneration, "no-schema-image")
	if err == nil || !strings.Contains(err.Error(), "parameter schema") {
		t.Fatalf("resolveClientModuleSchema() error = %v, want missing model-specific schema", err)
	}
}

func TestModuleSchemaResponseIdentifiesEachBuiltInImageModel(t *testing.T) {
	data := normalizeAICapabilityDefaults(adminPlatformData{})
	user := adminUser{ID: "user", Role: "MEMBER", PlanID: "plan_month"}
	gptCountOptions := []any{float64(1), float64(2), float64(3), float64(4)}
	for _, model := range []string{"mock-standard", "gpt-image-2", "HY-Image-3.0-Plus-4090-Tob-v1.0", "HY-Image-v3.0-I2I-ToB-v1.0.1"} {
		resolved, err := resolveModuleSchema(data, user, moduleImageGeneration, model)
		if err != nil {
			t.Fatalf("resolve %s: %v", model, err)
		}
		response := moduleSchemaResponse(resolved, user)
		if response["model_name"] != model || resolved.Schema.ModelName != model {
			t.Fatalf("model %s response/schema = %#v/%q", model, response["model_name"], resolved.Schema.ModelName)
		}
		responseSchema, ok := response["schema"].(adminAIParameterSchemaJSON)
		if !ok {
			t.Fatalf("model %s response schema type = %T", model, response["schema"])
		}
		countField, hasCount := imageContractFields(responseSchema.Fields)["n"]
		if model == "gpt-image-2" {
			if !hasCount || !reflect.DeepEqual(countField.Options, gptCountOptions) {
				t.Fatalf("model %s response n options = %#v, want %#v", model, countField.Options, gptCountOptions)
			}
		} else if hasCount {
			t.Fatalf("model %s response declared unsupported n field: %+v", model, countField)
		}
	}
}

func TestPublicModelsDoNotInventImageRatiosOrExposeSchemaLessModels(t *testing.T) {
	t.Setenv("MODEL_PROVIDER_URL", "https://provider.example.test/v1")
	t.Setenv("MODEL_PROVIDER_API_KEY", "test-key")
	t.Setenv("MODEL_PROVIDER_IMAGE_MODEL", "no-schema-image")
	handler := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()}).Handler
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/models", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("models status = %d, body = %s", res.Code, res.Body.String())
	}
	var items []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if _, ok := item["supportedRatios"]; ok {
			t.Fatalf("model invented supportedRatios: %#v", item)
		}
		if item["code"] == "no-schema-image" {
			t.Fatalf("schema-less image model was exposed: %#v", item)
		}
	}
}

func TestPublicCloudBaseModelsRequireConfiguredAIModelAndResolveExactSchema(t *testing.T) {
	t.Setenv("CLOUDBASE_ENABLED", "true")
	t.Setenv("CLOUDBASE_API_KEY", "test-key")
	t.Setenv("CLOUDBASE_IMAGE_FUNCTION_URL", "https://cloudbase.example.test/image")
	t.Setenv("MODEL_PROVIDER_URL", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("MODEL_PROVIDER_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	cloudBaseModels := map[string]bool{
		"HY-Image-3.0-Plus-4090-Tob-v1.0": true,
		"HY-Image-v3.0-I2I-ToB-v1.0.1":    true,
	}
	tests := []struct {
		name          string
		storedModels  []string
		wantCloudBase bool
	}{
		{name: "default models include routable CloudBase models", wantCloudBase: true},
		{name: "nonempty stored models do not gain channel-only CloudBase models", storedModels: []string{"mock-standard"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
			if len(tt.storedModels) > 0 {
				if err := store.updateAdmin(func(data *adminPlatformData) error {
					models := make([]adminAIModel, 0, len(tt.storedModels))
					for _, modelName := range tt.storedModels {
						model := findAIModel(data.AIModels, moduleImageGeneration, modelName)
						if model.ID == "" {
							t.Fatalf("seed model %s not found", modelName)
						}
						models = append(models, model)
					}
					data.AIModels = models
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}

			res := httptest.NewRecorder()
			(api{store: store}).models(res, httptest.NewRequest(http.MethodGet, "/api/v1/models", nil))
			if res.Code != http.StatusOK {
				t.Fatalf("models status = %d, body = %s", res.Code, res.Body.String())
			}
			var items []map[string]any
			if err := json.NewDecoder(res.Body).Decode(&items); err != nil {
				t.Fatal(err)
			}
			data, err := store.AdminData()
			if err != nil {
				t.Fatal(err)
			}
			data = normalizeAICapabilityDefaults(data)
			user := adminUser{ID: "user", Role: "MEMBER", PlanID: "plan_month"}
			seenCloudBase := 0
			for _, item := range items {
				code, _ := item["code"].(string)
				if findExactAIParameterSchema(data.AIParameterSchemas, moduleImageGeneration, code).ID == "" {
					continue
				}
				resolved, err := resolveModuleSchema(data, user, moduleImageGeneration, code)
				if err != nil {
					t.Fatalf("public image model %s cannot resolve module schema: %v", code, err)
				}
				if got := moduleSchemaResponse(resolved, user)["model_name"]; got != code {
					t.Fatalf("public model %s resolved model_name = %v", code, got)
				}
				if cloudBaseModels[code] {
					seenCloudBase++
				}
			}
			if tt.wantCloudBase && seenCloudBase != len(cloudBaseModels) {
				t.Fatalf("public CloudBase models = %d, want %d: %#v", seenCloudBase, len(cloudBaseModels), items)
			}
			if !tt.wantCloudBase && seenCloudBase != 0 {
				t.Fatalf("channel-only CloudBase models were exposed: %#v", items)
			}
		})
	}
}

func TestBuiltInImageInspirationSeedsUseCanonicalParameters(t *testing.T) {
	repository := newMemoryInspirationRepository()
	tests := []struct {
		id    string
		ratio string
	}{
		{id: "inspiration-product-clean", ratio: "1:1"},
		{id: "inspiration-poster-brand", ratio: "2:3"},
		{id: "inspiration-portrait-office", ratio: "2:3"},
		{id: "inspiration-brand-identity", ratio: "3:2"},
	}
	for _, tt := range tests {
		item, err := repository.GetTemplate(context.Background(), "default", "", tt.id, true)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"ratio": tt.ratio, "quality": "high", "count": 1}
		if !reflect.DeepEqual(item.Parameters, want) {
			t.Fatalf("%s parameters = %#v, want %#v", tt.id, item.Parameters, want)
		}
	}
}

func TestValidateGenerationParamsAcceptsOfficialGPTImageSizesAndCount(t *testing.T) {
	data := normalizeAICapabilityDefaults(adminPlatformData{})
	user := adminUser{ID: "user", Role: "MEMBER", PlanID: "plan_month"}
	resolved, err := resolveModuleSchema(data, user, moduleImageGeneration, "gpt-image-2")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range gptImage2SizeOptions() {
		sizeValue, _ := size.(string)
		req := generation.CreateRequest{
			ModuleCode: moduleImageGeneration, Model: "gpt-image-2", Prompt: "gpt image",
			Params: map[string]any{"size": sizeValue, "quality": "high", "n": float64(3)},
		}
		if err := validateGenerationParams(req, resolved); err != nil {
			t.Fatalf("size %s n=3 should pass: %v", sizeValue, err)
		}
	}
	for _, count := range []float64{1, 2, 3, 4} {
		req := generation.CreateRequest{
			ModuleCode: moduleImageGeneration, Model: "gpt-image-2", Prompt: "gpt image",
			Params: map[string]any{"size": "1024x1024", "quality": "low", "n": count},
		}
		if err := validateGenerationParams(req, resolved); err != nil {
			t.Fatalf("n=%v should pass: %v", count, err)
		}
	}
	omitted := generation.CreateRequest{
		ModuleCode: moduleImageGeneration, Model: "gpt-image-2", Prompt: "gpt image",
		Params: map[string]any{},
	}
	if err := validateGenerationParams(omitted, resolved); err != nil {
		t.Fatalf("omitted params should fill official defaults: %v", err)
	}
	if omitted.Params["size"] != "auto" {
		t.Fatalf("omitted size filled %v, want auto", omitted.Params["size"])
	}
	if omitted.Params["quality"] != "low" {
		t.Fatalf("omitted quality filled %v, want low", omitted.Params["quality"])
	}
	if omitted.Params["n"] != float64(1) {
		t.Fatalf("omitted n filled %v, want 1", omitted.Params["n"])
	}
	explicitAuto := generation.CreateRequest{
		ModuleCode: moduleImageGeneration, Model: "gpt-image-2", Prompt: "gpt image",
		Params: map[string]any{"size": "auto", "quality": "auto"},
	}
	if err := validateGenerationParams(explicitAuto, resolved); err != nil {
		t.Fatalf("explicit auto should pass: %v", err)
	}
	for _, size := range gptImage2DeferredProductionSizes() {
		req := generation.CreateRequest{
			ModuleCode: moduleImageGeneration, Model: "gpt-image-2", Prompt: "gpt image",
			Params: map[string]any{"size": size, "quality": "medium", "n": float64(1)},
		}
		if err := validateGenerationParams(req, resolved); err != nil {
			t.Fatalf("deferred size %s should still pass provider validation: %v", size, err)
		}
	}
	custom := generation.CreateRequest{
		ModuleCode: moduleImageGeneration, Model: "gpt-image-2", Prompt: "gpt image",
		Params: map[string]any{"size": "1792x1024", "quality": "low", "n": float64(1)},
	}
	if err := validateGenerationParams(custom, resolved); err != nil {
		t.Fatalf("custom legal size 1792x1024 should pass: %v", err)
	}
	illegal := generation.CreateRequest{
		ModuleCode: moduleImageGeneration, Model: "gpt-image-2", Prompt: "gpt image",
		Params: map[string]any{"size": "100x100", "quality": "high", "n": float64(1)},
	}
	if err := validateGenerationParams(illegal, resolved); err == nil {
		t.Fatal("illegal size 100x100 should fail")
	}
	illegalCount := generation.CreateRequest{
		ModuleCode: moduleImageGeneration, Model: "gpt-image-2", Prompt: "gpt image",
		Params: map[string]any{"size": "1024x1024", "quality": "high", "n": float64(5)},
	}
	if err := validateGenerationParams(illegalCount, resolved); err == nil {
		t.Fatal("n=5 should fail tenant/schema max 4")
	}
}

func TestGPTImageCanonicalParamsBeatAliases(t *testing.T) {
	req := generation.CreateRequest{
		ModuleCode: moduleImageGeneration,
		Model:      "gpt-image-2",
		Prompt:     "gpt image",
		Params: map[string]any{
			"size":         "1536x1024",
			"imageRatio":   "1:1",
			"quality":      "high",
			"imageQuality": "low",
			"n":            float64(2),
			"count":        float64(4),
		},
	}
	normalizeGPTImageCanonicalParams(&req)
	if req.Params["size"] != "1536x1024" {
		t.Fatalf("size = %v, want canonical 1536x1024", req.Params["size"])
	}
	if req.Params["quality"] != "high" {
		t.Fatalf("quality = %v, want canonical high", req.Params["quality"])
	}
	if req.Params["n"] != float64(2) {
		t.Fatalf("n = %v, want canonical 2", req.Params["n"])
	}
	if imageCount(req.Params) != 2 {
		t.Fatalf("billing quantity used alias count: %d", imageCount(req.Params))
	}

	aliasOnly := generation.CreateRequest{
		ModuleCode: moduleImageGeneration,
		Model:      "gpt-image-2",
		Params: map[string]any{
			"imageQuality": "medium",
			"count":        float64(3),
		},
	}
	normalizeGPTImageCanonicalParams(&aliasOnly)
	if aliasOnly.Params["quality"] != "medium" {
		t.Fatalf("alias quality = %v, want medium", aliasOnly.Params["quality"])
	}
	if aliasOnly.Params["n"] != float64(3) {
		t.Fatalf("alias n = %v, want 3", aliasOnly.Params["n"])
	}
}

func imageContractFields(fields []adminAIParameterField) map[string]adminAIParameterField {
	result := make(map[string]adminAIParameterField, len(fields))
	for _, field := range fields {
		result[field.Key] = field
	}
	return result
}
