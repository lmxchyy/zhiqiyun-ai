package image

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
)

const tinyReferenceDataURL = "data:image/png;base64,aGVsbG8="

type timeoutProviderError struct{}

func (timeoutProviderError) Error() string {
	return "net/http: TLS handshake timeout"
}

func (timeoutProviderError) Timeout() bool {
	return true
}

func TestReferenceBytesReadsUploadedImageFromLocalStorage(t *testing.T) {
	dir := t.TempDir()
	storedName := "local-reference.png"
	raw := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x01, 0x02, 0x03}
	if err := os.WriteFile(filepath.Join(dir, storedName), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	provider := OpenAICompatible{referenceImageDir: dir}
	got, filename, err := provider.referenceBytes(context.Background(), referenceImage{
		Name: "logo.png",
		URL:  "https://192.168.1.12:3100/api/v1/reference-images/" + storedName,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatalf("local reference bytes = %v, want %v", got, raw)
	}
	if filename != "logo.png" {
		t.Fatalf("filename = %q, want logo.png", filename)
	}
	if _, ok := provider.localReferenceImagePath("https://192.168.1.12:3100/api/v1/reference-images/../secret.png"); ok {
		t.Fatal("path traversal URL must not resolve to local storage")
	}
}

func TestRetryableProviderErrorIncludesNetworkTimeout(t *testing.T) {
	if !isRetryableProviderError(0, timeoutProviderError{}) {
		t.Fatal("network timeout with no HTTP status should be retryable")
	}
	if !isRetryableProviderError(http.StatusTooManyRequests, errors.New("rate limited")) {
		t.Fatal("HTTP 429 should be retried briefly before provider fallback")
	}
	if !isFallbackEligible(errors.New("image provider returned 429: Upstream rate limit")) {
		t.Fatal("HTTP 429 should be eligible for router fallback")
	}
	if !isFallbackEligible(errors.New("image provider returned 403: forbidden")) {
		t.Fatal("HTTP 403 should be eligible for router fallback")
	}
	if !isFallbackEligible(errors.New("dial tcp [::1]:8001: connect: connection refused")) {
		t.Fatal("connection refused should be eligible for router fallback")
	}
	if isRetryableProviderError(http.StatusBadRequest, timeoutProviderError{}) {
		t.Fatal("HTTP 400 should not be retried even if error text is timeout-like")
	}
}

func TestCompatibleReferenceFieldsSkipOversizedDataURLParts(t *testing.T) {
	provider := OpenAICompatible{}
	fields := map[string]string{}
	largeReference := "data:image/png;base64," + strings.Repeat("A", 500<<10)
	err := provider.addCompatibleReferenceFields(context.Background(), fields, []referenceImage{
		{Name: "first.png", URL: largeReference},
		{Name: "second.png", URL: largeReference},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fields["image_url"] == "" {
		t.Fatal("single compatible image_url should remain when it fits the upstream part limit")
	}
	for _, key := range []string{"image_urls", "reference_images", "referenceImages", "images"} {
		if _, exists := fields[key]; exists {
			t.Fatalf("oversized compatible field %s must be omitted", key)
		}
	}
}

func TestOpenAICompatibleImageEditUsesImageArrayField(t *testing.T) {
	var sawImageArray bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/custom/edits" {
			t.Fatalf("path = %s, want /custom/edits", r.URL.Path)
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatal(err)
		}
		if _, _, err := r.FormFile("image[]"); err != nil {
			t.Fatalf("missing image[] field: %v", err)
		}
		if _, _, err := r.FormFile("image"); err != nil {
			t.Fatalf("missing image compatibility alias field: %v", err)
		}
		if r.FormValue("image_url") == "" {
			t.Fatal("missing compatible image_url field for non-official endpoint")
		}
		if r.FormValue("image_urls") == "" || r.FormValue("reference_images") == "" {
			t.Fatal("missing compatible reference URL array fields for non-official endpoint")
		}
		sawImageArray = true
		writeImageAPIResponse(w)
	}))
	defer server.Close()

	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		BaseURL:           server.URL + "/v1",
		APIKey:            "test-key",
		ImageModel:        "gpt-image-2",
		ImageEditEndpoint: "/custom/edits",
		TimeoutMS:         5000,
	})
	images, err := provider.Generate(t.Context(), generation.CreateRequest{
		Type:   "IMAGE_TO_IMAGE",
		Prompt: "edit product",
		Model:  "gpt-image-2",
		Params: map[string]any{
			"imageRequestMode": "openai",
			"referenceImages":  []any{map[string]any{"name": "input.png", "url": tinyReferenceDataURL}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawImageArray {
		t.Fatal("server did not receive image[] request")
	}
	if len(images) != 1 || !strings.HasPrefix(images[0].URL, "data:image/png;base64,") {
		t.Fatalf("images = %#v, want one data URL image", images)
	}
}

func TestOpenAICompatibleOfficialEndpointOmitsCompatibleURLFields(t *testing.T) {
	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		BaseURL:    "https://api.openai.com/v1",
		APIKey:     "test-key",
		ImageModel: "gpt-image-2",
		TimeoutMS:  5000,
	})
	fields := map[string]string{}
	err := provider.addCompatibleReferenceFields(t.Context(), fields, []referenceImage{{
		Name: "input.png",
		URL:  tinyReferenceDataURL,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !provider.usesOfficialOpenAIEndpoint() {
		t.Fatal("api.openai.com should be detected as official endpoint")
	}
	// Official requests call this helper only behind the non-official guard; keep
	// the endpoint detector covered so unknown URL fields are not sent to OpenAI.
	if fields["image_url"] == "" {
		t.Fatal("helper should still build URL fields when explicitly called")
	}
}

func TestAddOptionalImageEditFields(t *testing.T) {
	fields := map[string]string{}
	addOptionalImageEditFields(fields, map[string]any{
		"quality":            "high",
		"output_format":      "png",
		"moderation":         "auto",
		"output_compression": 80,
	}, 2)
	for key, want := range map[string]string{
		"n":                  "2",
		"quality":            "high",
		"output_format":      "png",
		"moderation":         "auto",
		"output_compression": "80",
	} {
		if got := fields[key]; got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestResponsesImageToolOmitsAutoSizeAndQuality(t *testing.T) {
	tool := responsesImageTool(map[string]any{
		"size":         "auto",
		"imageQuality": "auto",
	}, false)
	if _, ok := tool["size"]; ok {
		t.Fatalf("size = %v, want omitted for auto", tool["size"])
	}
	if _, ok := tool["quality"]; ok {
		t.Fatalf("quality = %v, want omitted for auto", tool["quality"])
	}
}

func TestResponsesImageToolKeepsValidSizeAndQuality(t *testing.T) {
	tool := responsesImageTool(map[string]any{
		"size":    "1536x1024",
		"quality": "high",
	}, false)
	if got := tool["size"]; got != "1536x1024" {
		t.Fatalf("size = %v, want 1536x1024", got)
	}
	if got := tool["quality"]; got != "high" {
		t.Fatalf("quality = %v, want high", got)
	}
}

func TestResponsesImageToolOmitsUnsupportedQuality(t *testing.T) {
	for _, quality := range []string{"standard", "medium", "low", "draft"} {
		t.Run(quality, func(t *testing.T) {
			tool := responsesImageTool(map[string]any{
				"quality": quality,
			}, false)
			if _, ok := tool["quality"]; ok {
				t.Fatalf("quality = %v, want omitted", tool["quality"])
			}
		})
	}
}

func TestOpenAICompatibleGPTImage2UsesResponsesGenerate(t *testing.T) {
	var sawResponsesGenerate bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %s, want /v1/responses", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		tools, ok := body["tools"].([]any)
		if !ok || len(tools) != 1 {
			t.Fatalf("tools = %#v, want one image_generation tool", body["tools"])
		}
		tool, ok := tools[0].(map[string]any)
		if !ok || tool["type"] != "image_generation" || tool["action"] != "generate" {
			t.Fatalf("tool = %#v, want image_generation generate", tools[0])
		}
		if _, ok := tool["quality"]; ok {
			t.Fatalf("tool quality = %v, want omitted for frontend low quality", tool["quality"])
		}
		input, ok := body["input"].([]any)
		if !ok || len(input) != 1 {
			t.Fatalf("input = %#v, want one user message", body["input"])
		}
		message, ok := input[0].(map[string]any)
		if !ok {
			t.Fatalf("input[0] = %#v, want object", input[0])
		}
		content, ok := message["content"].([]any)
		if !ok || len(content) != 1 {
			t.Fatalf("content = %#v, want prompt text only", message["content"])
		}
		text, ok := content[0].(map[string]any)
		if !ok || text["type"] != "input_text" || !strings.Contains(fmt.Sprint(text["text"]), "生成卖书的电商图") {
			t.Fatalf("text content = %#v, want original prompt", content[0])
		}
		sawResponsesGenerate = true
		writeResponsesImageResponse(w)
	}))
	defer server.Close()

	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		BaseURL:    server.URL + "/v1",
		APIKey:     "test-key",
		ImageModel: "gpt-image-2",
		TimeoutMS:  5000,
	})
	images, err := provider.Generate(t.Context(), generation.CreateRequest{
		Type:   "TEXT_TO_IMAGE",
		Prompt: "生成卖书的电商图",
		Model:  "gpt-image-2",
		Params: map[string]any{
			"apiMode":      "responses",
			"size":         "auto",
			"imageQuality": "low",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawResponsesGenerate {
		t.Fatal("server did not receive responses generate request")
	}
	if len(images) != 1 || !strings.HasPrefix(images[0].URL, "data:image/png;base64,") {
		t.Fatalf("images = %#v, want one data URL image", images)
	}
}

func TestOpenAICompatibleGPTImage2FallsBackToResponsesGenerateAfterGeneration502(t *testing.T) {
	imageGenerationCalls := 0
	var sawResponsesGenerate bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/generations":
			imageGenerationCalls += 1
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":{"message":"Upstream 502"}}`))
		case "/v1/responses":
			sawResponsesGenerate = true
			writeResponsesImageResponse(w)
		default:
			t.Fatalf("unexpected path = %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		BaseURL:    server.URL + "/v1",
		APIKey:     "test-key",
		ImageModel: "gpt-image-2",
		TimeoutMS:  5000,
	})
	images, err := provider.Generate(t.Context(), generation.CreateRequest{
		Type:   "TEXT_TO_IMAGE",
		Prompt: "生成卖书的电商图",
		Model:  "gpt-image-2",
		Params: map[string]any{
			"apiMode": "images",
			"size":    "1024x1024",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if imageGenerationCalls != 3 {
		t.Fatalf("imageGenerationCalls = %d, want 3 retry attempts", imageGenerationCalls)
	}
	if !sawResponsesGenerate {
		t.Fatal("server did not receive responses fallback request")
	}
	if len(images) != 1 || !strings.HasPrefix(images[0].URL, "data:image/png;base64,") {
		t.Fatalf("images = %#v, want one data URL image", images)
	}
}

func TestProviderEndpointResolvesConfiguredPaths(t *testing.T) {
	base := "https://api.example.com/v1"
	tests := []struct {
		name       string
		configured string
		want       string
	}{
		{name: "default keeps v1 base", configured: "", want: "https://api.example.com/v1/images/edits"},
		{name: "absolute path uses origin", configured: "/custom/edits", want: "https://api.example.com/custom/edits"},
		{name: "relative path appends to base", configured: "custom/edits", want: "https://api.example.com/v1/custom/edits"},
		{name: "absolute url wins", configured: "https://other.example.com/edit", want: "https://other.example.com/edit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := providerEndpoint(base, tt.configured, "images/edits")
			if got != tt.want {
				t.Fatalf("endpoint = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestOpenAICompatibleImageEditFallsBackToImageField(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount += 1
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatal(err)
		}
		if requestCount == 1 {
			if _, _, err := r.FormFile("image[]"); err != nil {
				t.Fatalf("first request missing image[] field: %v", err)
			}
			http.Error(w, "field image[] is not supported", http.StatusBadRequest)
			return
		}
		if _, _, err := r.FormFile("image"); err != nil {
			t.Fatalf("fallback request missing image field: %v", err)
		}
		writeImageAPIResponse(w)
	}))
	defer server.Close()

	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		BaseURL:    server.URL + "/v1",
		APIKey:     "test-key",
		ImageModel: "gpt-image-2",
		TimeoutMS:  5000,
	})
	_, err := provider.Generate(t.Context(), generation.CreateRequest{
		Type:   "IMAGE_TO_IMAGE",
		Prompt: "edit product",
		Model:  "gpt-image-2",
		Params: map[string]any{
			"imageRequestMode": "openai",
			"referenceImages":  []any{map[string]any{"name": "input.png", "url": tinyReferenceDataURL}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 2 {
		t.Fatalf("requestCount = %d, want 2", requestCount)
	}
}

func TestOpenAICompatibleGPTImage2UsesResponsesEdit(t *testing.T) {
	var sawResponsesEdit bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %s, want /v1/responses", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		tools, ok := body["tools"].([]any)
		if !ok || len(tools) != 1 {
			t.Fatalf("tools = %#v, want one image_generation tool", body["tools"])
		}
		tool, ok := tools[0].(map[string]any)
		if !ok || tool["type"] != "image_generation" || tool["action"] != "edit" {
			t.Fatalf("tool = %#v, want image_generation edit", tools[0])
		}
		if _, ok := tool["size"]; ok {
			t.Fatalf("tool size = %v, want omitted for frontend auto size", tool["size"])
		}
		if _, ok := tool["quality"]; ok {
			t.Fatalf("tool quality = %v, want omitted for frontend auto quality", tool["quality"])
		}
		input, ok := body["input"].([]any)
		if !ok || len(input) != 1 {
			t.Fatalf("input = %#v, want one user message", body["input"])
		}
		message, ok := input[0].(map[string]any)
		if !ok {
			t.Fatalf("input[0] = %#v, want object", input[0])
		}
		content, ok := message["content"].([]any)
		if !ok || len(content) != 2 {
			t.Fatalf("content = %#v, want prompt text and input image", message["content"])
		}
		image, ok := content[1].(map[string]any)
		if !ok || image["type"] != "input_image" || !strings.HasPrefix(fmt.Sprint(image["image_url"]), "data:image/") {
			t.Fatalf("image content = %#v, want input_image data URL", content[1])
		}
		sawResponsesEdit = true
		writeResponsesImageResponse(w)
	}))
	defer server.Close()

	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		BaseURL:    server.URL + "/v1",
		APIKey:     "test-key",
		ImageModel: "gpt-image-2",
		TimeoutMS:  5000,
	})
	images, err := provider.Generate(t.Context(), generation.CreateRequest{
		Type:   "IMAGE_TO_IMAGE",
		Prompt: "edit product",
		Model:  "gpt-image-2",
		Params: map[string]any{
			"apiMode":         "responses",
			"size":            "auto",
			"imageQuality":    "auto",
			"referenceImages": []any{map[string]any{"name": "input.png", "url": tinyReferenceDataURL}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawResponsesEdit {
		t.Fatal("server did not receive responses edit request")
	}
	if len(images) != 1 || !strings.HasPrefix(images[0].URL, "data:image/png;base64,") {
		t.Fatalf("images = %#v, want one data URL image", images)
	}
}

func TestOpenAICompatibleGPTImage2UsesImagesEditWhenFrontendApiModeIsImages(t *testing.T) {
	var sawImagesEdit bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("path = %s, want /v1/images/edits", r.URL.Path)
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatal(err)
		}
		if _, _, err := r.FormFile("image[]"); err != nil {
			t.Fatalf("missing image[] field: %v", err)
		}
		sawImagesEdit = true
		writeImageAPIResponse(w)
	}))
	defer server.Close()

	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		BaseURL:    server.URL + "/v1",
		APIKey:     "test-key",
		ImageModel: "gpt-image-2",
		TimeoutMS:  5000,
	})
	_, err := provider.Generate(t.Context(), generation.CreateRequest{
		Type:   "IMAGE_TO_IMAGE",
		Prompt: "edit product",
		Model:  "gpt-image-2",
		Params: map[string]any{
			"apiMode":         "images",
			"referenceImages": []any{map[string]any{"name": "input.png", "url": tinyReferenceDataURL}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawImagesEdit {
		t.Fatal("server did not receive images edit request")
	}
}

func TestOpenAICompatibleMultiReferenceUsesConfiguredImagesMode(t *testing.T) {
	var imageFileCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("path = %s, want /v1/images/edits", r.URL.Path)
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatal(err)
		}
		files := r.MultipartForm.File["image[]"]
		imageFileCount = len(files)
		if imageFileCount != 2 {
			t.Fatalf("image[] files = %d, want 2", imageFileCount)
		}
		writeImageAPIResponse(w)
	}))
	defer server.Close()

	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		BaseURL:    server.URL + "/v1",
		APIKey:     "test-key",
		ImageModel: "gpt-image-2",
		TimeoutMS:  5000,
	})
	_, err := provider.Generate(t.Context(), generation.CreateRequest{
		Type:   "IMAGE_TO_IMAGE",
		Prompt: "保留图1产品主体，参考图2的logo",
		Model:  "gpt-image-2",
		Params: map[string]any{
			"apiMode": "images",
			"referenceImages": []any{
				map[string]any{"name": "product.png", "url": tinyReferenceDataURL},
				map[string]any{"name": "logo-poster.png", "url": tinyReferenceDataURL},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if imageFileCount != 2 {
		t.Fatalf("server received %d image files, want 2", imageFileCount)
	}
}

func TestOpenAICompatibleImageEditSkipsInvalidImageQuality(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatal(err)
		}
		if quality := r.FormValue("quality"); quality != "" {
			t.Fatalf("quality = %q, want omitted for invalid imageQuality", quality)
		}
		writeImageAPIResponse(w)
	}))
	defer server.Close()

	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		BaseURL:    server.URL + "/v1",
		APIKey:     "test-key",
		ImageModel: "gpt-image-2",
		TimeoutMS:  5000,
	})
	_, err := provider.Generate(t.Context(), generation.CreateRequest{
		Type:   "IMAGE_TO_IMAGE",
		Prompt: "edit product",
		Model:  "gpt-image-2",
		Params: map[string]any{
			"apiMode":         "images",
			"imageQuality":    "1K",
			"referenceImages": []any{map[string]any{"name": "input.png", "url": tinyReferenceDataURL}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func writeImageAPIResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": []map[string]string{
			{"b64_json": base64.StdEncoding.EncodeToString([]byte("generated"))},
		},
	})
}

func writeResponsesImageResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"output": []map[string]any{
			{
				"type":   "image_generation_call",
				"result": base64.StdEncoding.EncodeToString([]byte("generated")),
			},
		},
	})
}
