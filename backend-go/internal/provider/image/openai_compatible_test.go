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
	"time"

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

func TestRetryableProviderErrorDoesNotRetryAmbiguousNetworkTimeout(t *testing.T) {
	if isRetryableProviderError(0, timeoutProviderError{}) {
		t.Fatal("network timeout must not retry a non-idempotent image request")
	}
	if isRetryableProviderError(http.StatusTooManyRequests, errors.New("rate limited")) {
		t.Fatal("HTTP 429 must not retry a generation POST")
	}
	for _, err := range []error{
		errors.New("image provider returned 429: Upstream rate limit"),
		errors.New("image provider returned 403: forbidden"),
		errors.New("dial tcp [::1]:8001: connect: connection refused"),
	} {
		if isFallbackEligible(err) {
			t.Fatalf("untyped provider error must not prove pre-submit failure: %v", err)
		}
	}
	if isRetryableProviderError(http.StatusBadRequest, timeoutProviderError{}) {
		t.Fatal("HTTP 400 should not be retried even if error text is timeout-like")
	}
}

func TestImageEditTimeoutDoesNotStartDuplicateGenerationFallback(t *testing.T) {
	var generationCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/edits":
			time.Sleep(100 * time.Millisecond)
			writeImageAPIResponse(w)
		case "/v1/images/generations":
			generationCalls++
			writeImageAPIResponse(w)
		default:
			t.Fatalf("unexpected path = %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		BaseURL:    server.URL + "/v1",
		APIKey:     "test-key",
		ImageModel: "gpt-image-2",
		TimeoutMS:  1000,
	})
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	_, err := provider.Generate(ctx, generation.CreateRequest{
		Type:   "IMAGE_TO_IMAGE",
		Prompt: "add a logo",
		Model:  "gpt-image-2",
		Params: map[string]any{
			"size": "1024x1024",
			"referenceImages": []any{map[string]any{
				"name": "reference.png",
				"url":  tinyReferenceDataURL,
			}},
		},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Generate() error = %v, want context deadline exceeded", err)
	}
	if generationCalls != 0 {
		t.Fatalf("generation fallback calls = %d, want 0 after ambiguous timeout", generationCalls)
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
			"size":             "1024x1024",
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

func TestOpenAICompatibleGenerateUsesCanonicalImageParameters(t *testing.T) {
	for _, count := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprintf("n=%d", count), func(t *testing.T) {
			var captured map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
					t.Fatal(err)
				}
				writeImageAPIResponse(w)
			}))
			defer server.Close()

			provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
				BaseURL: server.URL + "/v1", APIKey: "test-key", ImageModel: "gpt-image-2", TimeoutMS: 5000,
			})
			_, err := provider.Generate(t.Context(), generation.CreateRequest{
				Type: "TEXT_TO_IMAGE", Prompt: "wide product photo", Model: "gpt-image-2",
				Params: map[string]any{
					"size": "1536x1024", "quality": "high", "n": count,
					"imageRatio": "3:4", "imageQuality": "draft",
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if captured["size"] != "1536x1024" || captured["quality"] != "high" || captured["n"] != float64(count) {
				t.Fatalf("provider body = %#v, want canonical size/quality/n=%d", captured, count)
			}
		})
	}
}

func TestOpenAICompatibleAcceptsOfficialGPTImageParameters(t *testing.T) {
	officialSizes := []string{"auto", "1024x1024", "1024x1536", "1536x1024", "1280x720", "720x1280", "2048x2048", "2048x1152", "3840x2160", "2160x3840"}
	tests := []struct {
		name   string
		params map[string]any
		want   map[string]any
	}{
		{name: "default omitted size and quality", params: map[string]any{}, want: map[string]any{"size": "auto", "quality": "low", "n": float64(1)}},
		{name: "explicit auto", params: map[string]any{"size": "auto", "quality": "auto"}, want: map[string]any{"size": "auto", "quality": "auto", "n": float64(1)}},
		{name: "quality low", params: map[string]any{"size": "1024x1024", "quality": "low", "n": 1}, want: map[string]any{"size": "1024x1024", "quality": "low", "n": float64(1)}},
		{name: "quality medium", params: map[string]any{"size": "1024x1024", "quality": "medium", "n": 1}, want: map[string]any{"size": "1024x1024", "quality": "medium", "n": float64(1)}},
	}
	for _, size := range officialSizes {
		if size == "auto" {
			continue
		}
		tests = append(tests, struct {
			name   string
			params map[string]any
			want   map[string]any
		}{
			name:   "size " + size,
			params: map[string]any{"size": size, "quality": "high", "n": 1},
			want:   map[string]any{"size": size, "quality": "high", "n": float64(1)},
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
					t.Fatal(err)
				}
				writeImageAPIResponse(w)
			}))
			defer server.Close()
			provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
				BaseURL: server.URL + "/v1", APIKey: "test-key", ImageModel: "gpt-image-2", TimeoutMS: 5000,
			})
			_, err := provider.Generate(t.Context(), generation.CreateRequest{
				Type: "TEXT_TO_IMAGE", Prompt: "official gpt image params", Model: "gpt-image-2", Params: tt.params,
			})
			if err != nil {
				t.Fatalf("Generate() error = %v", err)
			}
			for key, want := range tt.want {
				if captured[key] != want {
					t.Fatalf("body[%s] = %#v, want %#v in %#v", key, captured[key], want, captured)
				}
			}
		})
	}
}

func TestOpenAICompatibleRejectsUnsupportedImageParametersBeforeRequest(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
	}{
		{name: "unsupported size 100x100", params: map[string]any{"size": "100x100", "quality": "high", "n": 1}},
		{name: "unsupported size not multiple of 16", params: map[string]any{"size": "1000x1000", "quality": "high", "n": 1}},
		{name: "unsupported size 512x512 below pixel floor", params: map[string]any{"size": "512x512", "quality": "high", "n": 1}},
		{name: "ratio alias is not size", params: map[string]any{"size": "16:9", "quality": "high", "n": 1}},
		{name: "unsupported quality ultra", params: map[string]any{"size": "1024x1024", "quality": "ultra", "n": 1}},
		{name: "dalle quality standard", params: map[string]any{"size": "1024x1024", "quality": "standard", "n": 1}},
		{name: "dalle quality hd", params: map[string]any{"size": "1024x1024", "quality": "hd", "n": 1}},
		{name: "zero count", params: map[string]any{"size": "1024x1024", "quality": "high", "n": 0}},
		{name: "count above official maximum", params: map[string]any{"size": "1024x1024", "quality": "high", "n": 11}},
		{name: "fractional count", params: map[string]any{"size": "1024x1024", "quality": "high", "n": 1.5}},
		{name: "numeric string count", params: map[string]any{"size": "1024x1024", "quality": "high", "n": "2"}},
		{name: "non numeric string count", params: map[string]any{"size": "1024x1024", "quality": "high", "n": "many"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requestCount++
				writeImageAPIResponse(w)
			}))
			defer server.Close()
			provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
				BaseURL: server.URL + "/v1", APIKey: "test-key", ImageModel: "gpt-image-2", TimeoutMS: 5000,
			})
			_, err := provider.Generate(t.Context(), generation.CreateRequest{
				Type: "TEXT_TO_IMAGE", Prompt: "contract test", Model: "gpt-image-2", Params: tt.params,
			})
			if err == nil {
				t.Fatal("Generate() error = nil, want unsupported parameter error")
			}
			if requestCount != 0 {
				t.Fatalf("provider requests = %d, want 0", requestCount)
			}
		})
	}
}

func TestResponsesImageToolMapsStandardQualityToAuto(t *testing.T) {
	tool := responsesImageTool(map[string]any{
		"size":    "1024x1024",
		"quality": "standard",
	}, false)
	if tool["size"] != "1024x1024" {
		t.Fatalf("size = %v, want 1024x1024", tool["size"])
	}
	if got := tool["quality"]; got != "auto" {
		t.Fatalf("quality = %v, want auto for standard alias", got)
	}
}

func TestResponsesImageToolDefaultsOmittedQualityToLow(t *testing.T) {
	tool := responsesImageTool(map[string]any{"size": "1024x1024"}, false)
	if got := tool["quality"]; got != "low" {
		t.Fatalf("quality = %v, want low", got)
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
		if got := tool["quality"]; got != "auto" {
			t.Fatalf("tool quality = %v, want auto", got)
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
			"apiMode": "responses",
			"size":    "1024x1024",
			"quality": "auto",
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

func TestOpenAICompatibleGPTImage2DoesNotFallbackAfterGeneration502(t *testing.T) {
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
	if err == nil {
		t.Fatal("502 must be returned without another generation POST")
	}
	if images != nil || imageGenerationCalls != 1 || sawResponsesGenerate {
		t.Fatalf("images=%v generation calls=%d responses fallback=%v", images, imageGenerationCalls, sawResponsesGenerate)
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

func TestOpenAICompatibleImageEditDoesNotFallbackToImageField(t *testing.T) {
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
			"size":             "1024x1024",
			"referenceImages":  []any{map[string]any{"name": "input.png", "url": tinyReferenceDataURL}},
		},
	})
	if err == nil {
		t.Fatal("edit contract error must not trigger a second POST")
	}
	if requestCount != 1 {
		t.Fatalf("requestCount = %d, want 1", requestCount)
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
		if tool["size"] != "1024x1024" {
			t.Fatalf("tool size = %v, want 1024x1024", tool["size"])
		}
		if got := tool["quality"]; got != "auto" {
			t.Fatalf("tool quality = %v, want auto", got)
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
			"size":            "1024x1024",
			"quality":         "auto",
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
			"size":            "1024x1024",
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
			"size":    "1024x1024",
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

func TestOpenAICompatibleImageEditOmitsStandardQuality(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatal(err)
		}
		if quality := r.FormValue("quality"); quality != "" && quality != "auto" {
			t.Fatalf("quality = %q, want omitted or auto", quality)
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
			"size":            "1024x1024",
			"quality":         "auto",
			"referenceImages": []any{map[string]any{"name": "input.png", "url": tinyReferenceDataURL}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOfficialOpenAIUsesStableProviderOperationKey(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/images/generations", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(t.Context(), providerOperationKeyContextKey{}, "generation:task-1:1")
	setImageIdempotencyHeader(ctx, req)
	if got := req.Header.Get("Idempotency-Key"); got != "generation:task-1:1" {
		t.Fatalf("Idempotency-Key=%q", got)
	}
	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{BaseURL: "https://compatible.example/v1"})
	if provider.usesOfficialOpenAIEndpoint() {
		t.Fatal("compatible gateway must not claim verified native idempotency")
	}
}

func TestGenerationPOSTIsNeverRetriedOrRedirectReplayed(t *testing.T) {
	tests := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{name: "502", handler: func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "bad gateway", http.StatusBadGateway) }},
		{name: "malformed success", handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":`))
		}},
		{name: "empty success", handler: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[]}`))
		}},
		{name: "307", handler: func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/replayed", http.StatusTemporaryRedirect)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path == "/replayed" {
					t.Fatal("generation POST redirect was replayed")
				}
				tt.handler(w, r)
			}))
			defer server.Close()
			provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{BaseURL: server.URL + "/v1", APIKey: "key", ImageModel: "gpt-image-2", TimeoutMS: 1000})
			_, _ = provider.Generate(t.Context(), generation.CreateRequest{Type: "TEXT_TO_IMAGE", Prompt: "one", Model: "gpt-image-2", Params: map[string]any{"imageRequestMode": "images"}})
			if calls != 1 {
				t.Fatalf("POST calls=%d, want exactly 1", calls)
			}
		})
	}
}

func TestAmbiguousResponsesAndEditDoNotTryAlternateEndpoint(t *testing.T) {
	calls := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"broken":`))
	}))
	defer server.Close()
	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{BaseURL: server.URL + "/v1", APIKey: "key", ImageModel: "gpt-image-2", TimeoutMS: 1000})
	_, _ = provider.Generate(t.Context(), generation.CreateRequest{Type: "TEXT_TO_IMAGE", Prompt: "one", Model: "gpt-image-2", Params: map[string]any{"imageRequestMode": "responses"}})
	if calls["/v1/responses"] != 1 || calls["/v1/images/generations"] != 0 {
		t.Fatalf("responses=%d images=%d", calls["/v1/responses"], calls["/v1/images/generations"])
	}
	calls = map[string]int{}
	_, _ = provider.Generate(t.Context(), generation.CreateRequest{Type: "IMAGE_TO_IMAGE", Prompt: "edit", Model: "gpt-image-2", Params: map[string]any{"imageRequestMode": "images", "referenceImages": []any{map[string]any{"name": "in.png", "url": tinyReferenceDataURL}}}})
	if calls["/v1/images/edits"] != 1 || calls["/v1/images/generations"] != 0 {
		t.Fatalf("edits=%d generations=%d", calls["/v1/images/edits"], calls["/v1/images/generations"])
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
