package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	imageprovider "xianzhi-ai/backend-go/internal/provider/image"
)

type gptImageSmokeCase struct {
	name    string
	size    string
	quality string
	n       float64
}

func gptImageLaunchSmokeCases() []gptImageSmokeCase {
	return []gptImageSmokeCase{
		{name: "default-1k-low-n1", size: "1024x1024", quality: "low", n: 1},
		{name: "non-default-1536x1024-low-n1", size: "1536x1024", quality: "low", n: 1},
		{name: "custom-1792x1024-low-n1", size: "1792x1024", quality: "low", n: 1},
		{name: "default-1k-low-n3", size: "1024x1024", quality: "low", n: 3},
	}
}

func TestGPTImageLaunchSmokeChainMockProvider(t *testing.T) {
	data := normalizeAICapabilityDefaults(adminPlatformData{})
	published := publishedGPTImageSizeRules()
	user := adminUser{ID: "user", Role: "MEMBER", PlanID: "plan_month"}
	resolved, err := resolveModuleSchema(data, user, moduleImageGeneration, "gpt-image-2")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range gptImageLaunchSmokeCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := generation.CreateRequest{
				ModuleCode: moduleImageGeneration,
				Model:      "gpt-image-2",
				Prompt:     "launch smoke",
				Params: map[string]any{
					"size":    tc.size,
					"quality": tc.quality,
					"n":       tc.n,
				},
			}
			normalizeGPTImageCanonicalParams(&req)
			if err := validateGenerationParams(req, resolved); err != nil {
				t.Fatalf("validate: %v", err)
			}
			if req.Params["quality"] != tc.quality {
				t.Fatalf("canonical quality = %v, want %s", req.Params["quality"], tc.quality)
			}
			if req.Params["n"] != tc.n {
				t.Fatalf("canonical n = %v, want %v", req.Params["n"], tc.n)
			}
			if req.Params["size"] != tc.size {
				t.Fatalf("canonical size = %v, want %s", req.Params["size"], tc.size)
			}
			tier, err := imageprovider.NormalizeImageBillingSizeTier(tc.size)
			if err != nil {
				t.Fatal(err)
			}
			quote := generationPointCostForRequest(createGenerationTaskRequest{
				Type: "TEXT_TO_IMAGE", Model: "gpt-image-2",
				Params: map[string]any{"size": tc.size, "quality": tc.quality, "n": tc.n},
			}, data)
			lookup := gptImageBillingSizeLookupKey(tc.size, published)
			if _, ok := anyToFloat(published[lookup]); !ok {
				t.Fatalf("billing lookup %s not in PUBLISHED", lookup)
			}

			var captured map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
					t.Error(err)
				}
				count := 1
				if n, ok := captured["n"].(float64); ok {
					count = int(n)
				}
				data := make([]map[string]string, 0, count)
				for i := 0; i < count; i++ {
					data = append(data, map[string]string{"b64_json": "Z2Vu"})
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
			}))
			defer server.Close()
			provider := imageprovider.NewOpenAICompatibleWithOptions(imageprovider.OpenAICompatibleOptions{
				BaseURL: server.URL + "/v1", APIKey: "test-key", ImageModel: "gpt-image-2", TimeoutMS: 5000,
			})
			images, err := provider.Generate(context.Background(), req)
			if err != nil {
				t.Fatalf("provider: %v", err)
			}
			if captured["size"] != tc.size {
				t.Fatalf("provider payload size = %v, want %s", captured["size"], tc.size)
			}
			if captured["quality"] != tc.quality {
				t.Fatalf("provider payload quality = %v, want %s", captured["quality"], tc.quality)
			}
			if captured["n"] != tc.n {
				t.Fatalf("provider payload n = %v, want %v", captured["n"], tc.n)
			}
			if len(images) != int(tc.n) {
				t.Fatalf("returned images = %d, want %v", len(images), tc.n)
			}
			t.Logf("request=%s/%s/n=%v canonical=%v tier=%s lookup=%s points=%d provider=%v returned=%d",
				tc.size, tc.quality, tc.n, req.Params, tier, lookup, quote, captured, len(images))
		})
	}
}

func TestGPTImageLaunchLiveSmoke(t *testing.T) {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("MODEL_PROVIDER_URL")), "/")
	key := strings.TrimSpace(os.Getenv("MODEL_PROVIDER_API_KEY"))
	model := strings.TrimSpace(os.Getenv("MODEL_PROVIDER_IMAGE_MODEL"))
	if model == "" {
		model = "gpt-image-2"
	}
	if os.Getenv("GPT_IMAGE_LIVE_SMOKE") != "1" || base == "" || key == "" {
		t.Skip("live GPT Image smoke test requires explicit channel credentials")
	}
	data := normalizeAICapabilityDefaults(adminPlatformData{})
	published := publishedGPTImageSizeRules()
	provider := imageprovider.NewOpenAICompatibleWithOptions(imageprovider.OpenAICompatibleOptions{
		BaseURL: base, APIKey: key, ImageModel: model, TimeoutMS: 180000,
	})
	for _, tc := range gptImageLaunchSmokeCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := generation.CreateRequest{
				Type:   "TEXT_TO_IMAGE",
				Model:  model,
				Prompt: "Launch smoke: a simple red ceramic mug on a white table, no text.",
				Params: map[string]any{
					"size":          tc.size,
					"quality":       tc.quality,
					"n":             tc.n,
					"output_format": "jpeg",
					"apiMode":       "images",
				},
			}
			tier, err := imageprovider.NormalizeImageBillingSizeTier(tc.size)
			if err != nil {
				t.Fatal(err)
			}
			lookup := gptImageBillingSizeLookupKey(tc.size, published)
			points := generationPointCostForRequest(createGenerationTaskRequest{
				Type: "TEXT_TO_IMAGE", Model: "gpt-image-2",
				Params: map[string]any{"size": tc.size, "quality": tc.quality, "n": tc.n},
			}, data)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			images, err := provider.Generate(ctx, req)
			if err != nil {
				t.Fatal("live generate failed")
			}
			if req.Params["size"] != tc.size {
				t.Fatal("live request size mutated")
			}
			t.Logf("LIVE request size=%s quality=%s n=%v tier=%s lookup=%s points=%d returned=%d",
				tc.size, tc.quality, tc.n, tier, lookup, points, len(images))
			if len(images) != int(tc.n) {
				t.Fatalf("returned images = %d, want %v", len(images), tc.n)
			}
		})
	}
}

func gptImageMinProdSmokeCases() []gptImageSmokeCase {
	return []gptImageSmokeCase{
		{name: "1024x1024-auto-n1", size: "1024x1024", quality: "auto", n: 1},
		{name: "2048x2048-auto-n1", size: "2048x2048", quality: "auto", n: 1},
		{name: "2048x2048-low-n1", size: "2048x2048", quality: "low", n: 1},
		{name: "1024x1024-low-n3", size: "1024x1024", quality: "low", n: 3},
	}
}

func staleGPTImageProdLikeData() adminPlatformData {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return normalizeAICapabilityDefaults(adminPlatformData{
		AIModules: defaultAIModules(now), AIModels: defaultAIModels(now),
		AIParameterSchemas: defaultAIParameterSchemas(now),
		TenantModuleLimits: []adminTenantModuleLimit{{
			ID: "limit_stale_camel", TenantID: "default", ModuleCode: moduleImageGeneration, Status: "ACTIVE",
			LimitJSONCamel: map[string]any{
				"models":  map[string]any{"allowed": []any{"gpt-image-2"}},
				"quality": map[string]any{"allowed": []any{"standard", "high"}},
				"n":       map[string]any{"max": float64(4)},
			},
		}},
		BillingRules: defaultBillingRules(now),
	})
}

func qualityLooksLegacy(value any) bool {
	quality := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
	return quality == "standard" || quality == "hd"
}

func TestGPTImageMinProdSmokeChain(t *testing.T) {
	data := staleGPTImageProdLikeData()
	user := adminUser{ID: "user_min_prod_smoke", Role: "MEMBER", PlanID: "plan_month"}
	service := api{}
	for _, tc := range gptImageMinProdSmokeCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			frontend := map[string]any{"size": tc.size, "quality": tc.quality, "n": tc.n}
			prepared, err := service.prepareGenerationRequest(data, user, generation.CreateRequest{
				Type: "TEXT_TO_IMAGE", ModuleCode: moduleImageGeneration, Model: "gpt-image-2",
				Prompt: "min prod smoke: a simple red ceramic mug on a white table, no text.",
				Params: map[string]any{"size": tc.size, "quality": tc.quality, "n": tc.n},
			})
			if err != nil {
				t.Fatalf("prepare: %v", err)
			}
			if prepared.Params["size"] != tc.size || prepared.Params["quality"] != tc.quality || prepared.Params["n"] != tc.n {
				t.Fatalf("canonical mutated: size=%v quality=%v n=%v", prepared.Params["size"], prepared.Params["quality"], prepared.Params["n"])
			}
			if qualityLooksLegacy(prepared.Params["quality"]) {
				t.Fatalf("canonical quality rewritten to legacy %v", prepared.Params["quality"])
			}

			dataPath := filepath.Join(t.TempDir(), "store.json")
			writeGenerationBillingPointSeed(t, dataPath, user.ID, 100000)
			store := newJSONStore(dataPath)
			pendingReq := prepared
			pendingReq.UserID = user.ID
			pendingReq.ClientRequestID = "smoke_" + tc.name
			task, err := store.CreatePendingGenerationTask(pendingReq)
			if err != nil {
				t.Fatalf("create pending: %v", err)
			}
			if qualityLooksLegacy(task.Params["quality"]) {
				t.Fatalf("stored quality rewritten to legacy %v", task.Params["quality"])
			}
			if task.Params["size"] != tc.size || task.Params["n"] != tc.n {
				t.Fatalf("stored size/n mutated: size=%v n=%v", task.Params["size"], task.Params["n"])
			}

			var captured map[string]any
			var endpoint string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				endpoint = r.URL.Path
				if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
					t.Error(err)
				}
				count := 1
				if n, ok := captured["n"].(float64); ok {
					count = int(n)
				}
				payload := make([]map[string]string, 0, count)
				for i := 0; i < count; i++ {
					payload = append(payload, map[string]string{"b64_json": "Z2Vu"})
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"data": payload})
			}))
			defer server.Close()
			provider := imageprovider.NewOpenAICompatibleWithOptions(imageprovider.OpenAICompatibleOptions{
				BaseURL: server.URL + "/v1", APIKey: "test-key", ImageModel: "gpt-image-2", TimeoutMS: 5000,
			})
			images, err := provider.Generate(context.Background(), prepared)
			if err != nil {
				t.Fatalf("provider: %v", err)
			}
			if captured["size"] != tc.size {
				t.Fatalf("provider size = %v, want %s", captured["size"], tc.size)
			}
			if captured["quality"] != tc.quality {
				t.Fatalf("provider quality = %v, want %s", captured["quality"], tc.quality)
			}
			if qualityLooksLegacy(captured["quality"]) {
				t.Fatalf("provider quality is legacy %v", captured["quality"])
			}
			if captured["n"] != tc.n {
				t.Fatalf("provider n = %v, want %v", captured["n"], tc.n)
			}
			if len(images) != int(tc.n) {
				t.Fatalf("returned images = %d, want %v", len(images), tc.n)
			}
			t.Logf("PASS frontend=%v canonical={size:%v quality:%v n:%v} stored={id:%s size:%v quality:%v n:%v} provider=%v endpoint=%s returned=%d created=%s",
				frontend, prepared.Params["size"], prepared.Params["quality"], prepared.Params["n"],
				task.ID, task.Params["size"], task.Params["quality"], task.Params["n"],
				captured, endpoint, len(images), task.ID)
		})
	}
}

func TestGPTImageMinProdLiveSmoke(t *testing.T) {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("MODEL_PROVIDER_URL")), "/")
	key := strings.TrimSpace(os.Getenv("MODEL_PROVIDER_API_KEY"))
	model := strings.TrimSpace(os.Getenv("MODEL_PROVIDER_IMAGE_MODEL"))
	if model == "" {
		model = "gpt-image-2"
	}
	if os.Getenv("GPT_IMAGE_LIVE_SMOKE") != "1" || base == "" || key == "" {
		t.Skip("live GPT Image min-prod smoke requires GPT_IMAGE_LIVE_SMOKE=1 and channel credentials")
	}
	data := staleGPTImageProdLikeData()
	user := adminUser{ID: "user_min_prod_smoke", Role: "MEMBER", PlanID: "plan_month"}
	service := api{}
	provider := imageprovider.NewOpenAICompatibleWithOptions(imageprovider.OpenAICompatibleOptions{
		BaseURL: base, APIKey: key, ImageModel: model, TimeoutMS: 180000,
	})
	endpoint := strings.TrimRight(base, "/") + "/v1/images/generations"
	for _, tc := range gptImageMinProdSmokeCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			prepared, err := service.prepareGenerationRequest(data, user, generation.CreateRequest{
				Type: "TEXT_TO_IMAGE", ModuleCode: moduleImageGeneration, Model: "gpt-image-2",
				Prompt: "min prod smoke: a simple red ceramic mug on a white table, no text.",
				Params: map[string]any{
					"size": tc.size, "quality": tc.quality, "n": tc.n,
					"output_format": "jpeg", "apiMode": "images",
				},
			})
			if err != nil {
				t.Fatalf("prepare: %v", err)
			}
			if prepared.Params["quality"] != tc.quality || qualityLooksLegacy(prepared.Params["quality"]) {
				t.Fatalf("canonical quality = %v", prepared.Params["quality"])
			}
			if prepared.Params["size"] != tc.size {
				t.Fatalf("canonical size = %v", prepared.Params["size"])
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			images, err := provider.Generate(ctx, prepared)
			if err != nil {
				t.Fatalf("live generate failed: %v", err)
			}
			if len(images) != int(tc.n) {
				t.Fatalf("returned images = %d, want %v", len(images), tc.n)
			}
			t.Logf("LIVE PASS size=%s quality=%s n=%v endpoint=%s returned=%d", tc.size, tc.quality, tc.n, endpoint, len(images))
		})
	}
}
