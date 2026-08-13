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
		{model: "gpt-image-2", sizes: []any{"1024x1024", "1024x1536", "1536x1024"}, qualities: []any{"standard", "high"}, countOptions: []any{float64(1), float64(2), float64(4)}},
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

func TestNormalizeAICapabilityDefaultsReplacesStaleBuiltInImageSchema(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stale := defaultAIParameterSchemas(now)[0]
	stale.SchemaJSON.Fields = []adminAIParameterField{
		{Key: "prompt", Type: "textarea", Required: true},
		{Key: "size", Type: "select", Required: true, Default: "1024x1024", Options: anyOptions("1024x1024", "1024x1536", "1536x1024")},
		{Key: "quality", Type: "select", Required: true, Default: "standard", Options: anyOptions("standard", "high")},
	}
	data := normalizeAICapabilityDefaults(adminPlatformData{
		AIModules: defaultAIModules(now), AIModels: defaultAIModels(now),
		AIParameterSchemas: []adminAIParameterSchema{stale},
		TenantModuleLimits: defaultTenantModuleLimits(now), BillingRules: defaultBillingRules(now),
	})
	schema := findExactAIParameterSchema(data.AIParameterSchemas, moduleImageGeneration, "mock-standard")
	fields := imageContractFields(schema.SchemaJSON.Fields)
	if !reflect.DeepEqual(fields["size"].Options, []any{"1920x1080"}) {
		t.Fatalf("normalized mock size options = %#v", fields["size"].Options)
	}
	if _, ok := fields["quality"]; ok {
		t.Fatalf("stale mock quality survived normalization: %+v", schema.SchemaJSON.Fields)
	}
	if _, ok := fields["n"]; ok {
		t.Fatalf("stale mock count survived normalization: %+v", schema.SchemaJSON.Fields)
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
	gptCountOptions := []any{float64(1), float64(2), float64(4)}
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

func imageContractFields(fields []adminAIParameterField) map[string]adminAIParameterField {
	result := make(map[string]adminAIParameterField, len(fields))
	for _, field := range fields {
		result[field.Key] = field
	}
	return result
}
