package image

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
)

type cloudBaseRoundTripper func(*http.Request) (*http.Response, error)

func (fn cloudBaseRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestCloudBaseFunctionGenerateUsesServerCredentialAndFixedWatermark(t *testing.T) {
	provider := NewCloudBaseFunction(CloudBaseFunctionOptions{
		FunctionURL:   "https://env-id.api.tcloudbasegateway.com/v1/functions/zhiqiyun-ai-image",
		APIKey:        "server-only-key",
		DefaultModel:  "HY-Image-3.0-Plus-4090-Tob-v1.0",
		Models:        []string{"HY-Image-3.0-Plus-4090-Tob-v1.0"},
		WatermarkText: "AI生成",
	})
	provider.client.Transport = cloudBaseRoundTripper(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Authorization"); got != "Bearer server-only-key" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "HY-Image-3.0-Plus-4090-Tob-v1.0" {
			t.Fatalf("unexpected model: %#v", body["model"])
		}
		if body["footnote"] != "AI生成" || body["revise"] != true {
			t.Fatalf("required provider safeguards missing: %#v", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"result":{"url":"https://example.com/generated.jpg","providerTaskId":"cb-task-1","revisedPrompt":"safe prompt"},"requestId":"cb-request-1"}`)),
			Header:     make(http.Header),
		}, nil
	})

	images, err := provider.Generate(context.Background(), generation.CreateRequest{
		Model: "HY-Image-3.0-Plus-4090-Tob-v1.0", Prompt: "生成商品宣传图", ClientRequestID: "client-1",
		Params: map[string]any{"size": "1024x1024"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || images[0].ProviderTaskID != "cb-task-1" || images[0].Source != "cloudbase" {
		t.Fatalf("unexpected generated images: %#v", images)
	}
}

func TestCloudBaseFunctionRejectsRetiredAndInvalidImageToImageRequests(t *testing.T) {
	provider := NewCloudBaseFunction(CloudBaseFunctionOptions{
		FunctionURL: "https://env-id.api.tcloudbasegateway.com/v1/functions/zhiqiyun-ai-image",
		APIKey:      "server-only-key",
		Models:      []string{"hunyuan-image", "HY-Image-v3.0-I2I-ToB-v1.0.1"},
	})
	if provider.Supports(generation.CreateRequest{Model: "hunyuan-image"}) {
		t.Fatal("retired model must not be supported")
	}
	_, err := provider.Generate(context.Background(), generation.CreateRequest{
		Model: "HY-Image-v3.0-I2I-ToB-v1.0.1", Prompt: "转换图片风格",
		Params: map[string]any{"size": "1024x1024"},
	})
	if err == nil || !strings.Contains(err.Error(), "exactly one HTTPS reference image") {
		t.Fatalf("expected image-to-image reference validation, got %v", err)
	}
}

func TestValidCloudBaseFunctionURLRejectsNonOfficialHost(t *testing.T) {
	if _, err := validCloudBaseFunctionURL("https://example.com/v1/functions/image"); err == nil {
		t.Fatal("non-CloudBase function host must be rejected")
	}
}

func TestCloudBaseFunctionRejectsUnsupportedSizeBeforeRequestForEachModel(t *testing.T) {
	for _, model := range []string{"HY-Image-3.0-Plus-4090-Tob-v1.0", "HY-Image-v3.0-I2I-ToB-v1.0.1"} {
		t.Run(model, func(t *testing.T) {
			requestCount := 0
			provider := NewCloudBaseFunction(CloudBaseFunctionOptions{
				FunctionURL: "https://env-id.api.tcloudbasegateway.com/v1/functions/zhiqiyun-ai-image",
				APIKey:      "server-only-key", DefaultModel: model, Models: []string{model},
			})
			provider.client.Transport = cloudBaseRoundTripper(func(_ *http.Request) (*http.Response, error) {
				requestCount++
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"url":"https://example.com/generated.jpg"}`)), Header: make(http.Header)}, nil
			})
			params := map[string]any{"size": "16:9", "quality": "high", "n": 4}
			if model == "HY-Image-v3.0-I2I-ToB-v1.0.1" {
				params["referenceImages"] = []any{map[string]any{"url": "https://example.com/reference.jpg"}}
			}
			_, err := provider.Generate(t.Context(), generation.CreateRequest{Model: model, Prompt: "contract test", Params: params})
			if err == nil {
				t.Fatal("Generate() error = nil, want unsupported size error")
			}
			if requestCount != 0 {
				t.Fatalf("provider requests = %d, want 0", requestCount)
			}
		})
	}
}

func TestCloudBaseFunctionBodyUsesCanonicalSizeAndOmitsUnsupportedParameters(t *testing.T) {
	provider := NewCloudBaseFunction(CloudBaseFunctionOptions{
		FunctionURL: "https://env-id.api.tcloudbasegateway.com/v1/functions/zhiqiyun-ai-image",
		APIKey:      "server-only-key", DefaultModel: "HY-Image-3.0-Plus-4090-Tob-v1.0",
		Models: []string{"HY-Image-3.0-Plus-4090-Tob-v1.0"},
	})
	provider.client.Transport = cloudBaseRoundTripper(func(req *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["size"] != "720x1280" {
			t.Fatalf("size = %#v, want 720x1280", body["size"])
		}
		for _, key := range []string{"quality", "n"} {
			if _, ok := body[key]; ok {
				t.Fatalf("unsupported %s leaked into body: %#v", key, body)
			}
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"url":"https://example.com/generated.jpg"}`)), Header: make(http.Header)}, nil
	})
	_, err := provider.Generate(t.Context(), generation.CreateRequest{
		Model: "HY-Image-3.0-Plus-4090-Tob-v1.0", Prompt: "portrait",
		Params: map[string]any{"size": "720x1280", "aspect_ratio": "16:9", "quality": "high", "n": 4},
	})
	if err != nil {
		t.Fatal(err)
	}
}
