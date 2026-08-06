package video

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
)

func TestOpenAICompatibleDoubaoSeedanceUsesVideosEndpointAndPayload(t *testing.T) {
	var requestPath string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		if r.Method != http.MethodPost || r.URL.Path != "/api/v3/videos/generations" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 200,
			"data": []any{
				map[string]any{
					"status":  "completed",
					"task_id": "task_123",
					"result": map[string]any{
						"videos": []any{
							map[string]any{"url": []any{"https://cdn.example/video.mp4"}},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		BaseURL: server.URL + "/api/v3",
		APIKey:  "sk-test",
		Models:  []string{"doubao-seedance-2.0"},
	})
	result, err := provider.Create(context.Background(), generation.CreateRequest{
		Model:  "doubao-seedance-2.0",
		Prompt: "city skyline",
		Params: map[string]any{
			"duration":     5,
			"aspect_ratio": "16:9",
			"resolution":   "1080p",
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if requestPath != "/api/v3/videos/generations" {
		t.Fatalf("request path = %q", requestPath)
	}
	if payload["duration"] != float64(5) {
		t.Fatalf("duration payload = %#v", payload["duration"])
	}
	if payload["size"] != "16:9" {
		t.Fatalf("size payload = %#v", payload["size"])
	}
	if payload["resolution"] != "1080p" {
		t.Fatalf("resolution payload = %#v", payload["resolution"])
	}
	if _, ok := payload["seconds"]; ok {
		t.Fatalf("doubao payload should not include seconds: %#v", payload)
	}
	if _, ok := payload["aspect_ratio"]; ok {
		t.Fatalf("doubao payload should not include aspect_ratio: %#v", payload)
	}
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if got := resultMap["videoUrl"]; got != "https://cdn.example/video.mp4" {
		t.Fatalf("videoUrl = %#v", got)
	}
}

func TestOpenAICompatibleDoubaoSeedanceUsesConfiguredContentTaskEndpoint(t *testing.T) {
	var requestPath string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		if r.Method != http.MethodPost || r.URL.Path != "/api/v3/contents/generations/tasks" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "task_123",
			"status": "succeeded",
			"content": map[string]any{
				"video_url": "https://cdn.example/video.mp4",
			},
		})
	}))
	defer server.Close()

	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		BaseURL:  server.URL + "/api/v3",
		APIKey:   "sk-test",
		Models:   []string{"doubao-seedance-2.0"},
		Endpoint: "contents/generations/tasks",
	})
	result, err := provider.Create(context.Background(), generation.CreateRequest{
		Model:  "doubao-seedance-2.0",
		Prompt: "city skyline",
		Params: map[string]any{
			"duration":       5,
			"aspect_ratio":   "16:9",
			"resolution":     "1080p",
			"generate_audio": true,
			"referenceImages": []any{
				map[string]any{"url": "https://cdn.example/frame.png"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if requestPath != "/api/v3/contents/generations/tasks" {
		t.Fatalf("request path = %q", requestPath)
	}
	if payload["duration"] != float64(5) {
		t.Fatalf("duration payload = %#v", payload["duration"])
	}
	if payload["ratio"] != "16:9" {
		t.Fatalf("ratio payload = %#v", payload["ratio"])
	}
	if payload["generate_audio"] != true {
		t.Fatalf("generate_audio payload = %#v", payload["generate_audio"])
	}
	if _, ok := payload["prompt"]; ok {
		t.Fatalf("content task payload should not include prompt: %#v", payload)
	}
	content, ok := payload["content"].([]any)
	if !ok || len(content) != 2 {
		t.Fatalf("content payload = %#v", payload["content"])
	}
	first, _ := content[0].(map[string]any)
	if first["type"] != "text" || first["text"] != "city skyline" {
		t.Fatalf("first content item = %#v", first)
	}
	second, _ := content[1].(map[string]any)
	if second["type"] != "image_url" || second["role"] != "first_frame" {
		t.Fatalf("second content item = %#v", second)
	}
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if got := resultMap["videoUrl"]; got != "https://cdn.example/video.mp4" {
		t.Fatalf("videoUrl = %#v", got)
	}
}

func TestDoubaoSeedanceEndpointHelpersRespectVersionedBaseURL(t *testing.T) {
	if got := videoProviderEndpointForModel("https://example.com/api/v3", "", "doubao-seedance-2.0"); got != "https://example.com/api/v3/videos/generations" {
		t.Fatalf("provider endpoint = %q", got)
	}
	if got := videoProviderEndpointForModel("https://example.com/api/v3", "contents/generations/tasks", "doubao-seedance-2.0"); got != "https://example.com/api/v3/contents/generations/tasks" {
		t.Fatalf("configured provider endpoint = %q", got)
	}
	if got := videoUnifiedTaskEndpoint("https://example.com/api/v3", "task_123"); got != "https://example.com/api/v3/tasks/task_123?language=zh" {
		t.Fatalf("task endpoint = %q", got)
	}
	if got := videoTaskEndpointForModel("https://example.com/api/v3", "contents/generations/tasks", "task_123", "doubao-seedance-2.0"); got != "https://example.com/api/v3/contents/generations/tasks/task_123" {
		t.Fatalf("configured task endpoint = %q", got)
	}
	if got := videoContentEndpointForModel("https://example.com/api/v3", "contents/generations/tasks", "task_123", "doubao-seedance-2.0"); got != "https://example.com/v1/videos/task_123/content" {
		t.Fatalf("configured content endpoint = %q", got)
	}
	if got := videoProviderEndpointForModel("https://example.com", "", "doubao-seedance-2.0"); got != "https://example.com/v1/videos/generations" {
		t.Fatalf("provider endpoint without version = %q", got)
	}
}

func TestVideoAspectRatioPrefersCanonicalAndSupportsLegacyTasks(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{name: "canonical", params: map[string]any{"aspect_ratio": "9:16"}, want: "9:16"},
		{name: "legacy", params: map[string]any{"ratio": "1:1"}, want: "1:1"},
		{name: "canonical wins", params: map[string]any{"aspect_ratio": "16:9", "ratio": "9:16"}, want: "16:9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := videoAspectRatio(tt.params); got != tt.want {
				t.Fatalf("videoAspectRatio(%+v) = %q, want %q", tt.params, got, tt.want)
			}
		})
	}
}

func TestVideoRequestBodyMapsCanonicalParametersByProtocol(t *testing.T) {
	tests := []struct {
		name            string
		model           string
		endpoint        string
		params          map[string]any
		wantRatioKey    string
		wantRatio       string
		wantResolution  string
		wantDurationKey string
		wantDuration    string
		wantAudio       *bool
	}{
		{name: "generic 16:9 480p", model: "video-model", endpoint: "/v1/video/generations", params: map[string]any{"aspect_ratio": "16:9", "resolution": "480p", "duration": 4}, wantRatioKey: "aspect_ratio", wantRatio: "16:9", wantResolution: "480p", wantDurationKey: "seconds", wantDuration: "4"},
		{name: "doubao video 9:16 720p", model: "doubao-seedance-2.0", endpoint: "/v1/videos/generations", params: map[string]any{"aspect_ratio": "9:16", "resolution": "720p", "duration": 5}, wantRatioKey: "size", wantRatio: "9:16", wantResolution: "720p", wantDurationKey: "duration", wantDuration: "5"},
		{name: "seedance content 1:1 1080p audio on", model: "doubao-seedance-2.0", endpoint: "/v1/contents/generations/tasks", params: map[string]any{"aspect_ratio": "1:1", "resolution": "1080p", "duration": 10, "generate_audio": true}, wantRatioKey: "ratio", wantRatio: "1:1", wantResolution: "1080p", wantDurationKey: "duration", wantDuration: "10", wantAudio: boolPointer(true)},
		{name: "seedance content audio off", model: "doubao-seedance-2.0", endpoint: "/v1/contents/generations/tasks", params: map[string]any{"aspect_ratio": "16:9", "resolution": "720p", "duration": 5, "generate_audio": false}, wantRatioKey: "ratio", wantRatio: "16:9", wantResolution: "720p", wantDurationKey: "duration", wantDuration: "5", wantAudio: boolPointer(false)},
		{name: "generic 4k", model: "video-model", endpoint: "/v1/video/generations", params: map[string]any{"aspect_ratio": "16:9", "resolution": "4k", "duration": 15}, wantRatioKey: "aspect_ratio", wantRatio: "16:9", wantResolution: "4k", wantDurationKey: "seconds", wantDuration: "15"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := videoRequestBodyForEndpoint(tt.model, generation.CreateRequest{Prompt: "test", Params: tt.params}, tt.endpoint, nil)
			if got := body[tt.wantRatioKey]; got != tt.wantRatio {
				t.Fatalf("%s = %#v, want %q; body=%+v", tt.wantRatioKey, got, tt.wantRatio, body)
			}
			if got := body["resolution"]; got != tt.wantResolution {
				t.Fatalf("resolution = %#v, want %q", got, tt.wantResolution)
			}
			if got := fmt.Sprint(body[tt.wantDurationKey]); got != tt.wantDuration {
				t.Fatalf("%s = %#v, want %s", tt.wantDurationKey, body[tt.wantDurationKey], tt.wantDuration)
			}
			if tt.wantAudio != nil && body["generate_audio"] != *tt.wantAudio {
				t.Fatalf("generate_audio = %#v, want %v", body["generate_audio"], *tt.wantAudio)
			}
		})
	}
}

func boolPointer(value bool) *bool { return &value }

func TestOpenAICompatibleRejectsUnsupportedOptionalParameterBeforeSending(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		BaseURL: server.URL, APIKey: "sk-test", Model: "video-model", Models: []string{"video-model"},
	})
	tests := []struct {
		key   string
		value any
	}{
		{key: "fps", value: 30},
		{key: "generate_audio", value: true},
		{key: "motion_strength", value: "high"},
		{key: "camera_movement", value: "pan"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if _, err := provider.Create(context.Background(), generation.CreateRequest{
				Model: "video-model", Prompt: "test", Params: map[string]any{tt.key: tt.value},
			}); err == nil {
				t.Fatalf("unsupported %s was accepted", tt.key)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("provider was called %d times for unsupported parameters", calls)
	}
}

func TestOpenAICompatibleLegacyRatioDoesNotLeakToGenericProtocol(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "legacy-ratio", "status": "succeeded", "video_url": "https://cdn.example/video.mp4",
		})
	}))
	defer server.Close()

	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		BaseURL: server.URL, APIKey: "sk-test", Model: "video-model", Models: []string{"video-model"},
	})
	if _, err := provider.Create(context.Background(), generation.CreateRequest{
		Model: "video-model", Prompt: "legacy", Params: map[string]any{"ratio": "9:16"},
	}); err != nil {
		t.Fatal(err)
	}
	if payload["aspect_ratio"] != "9:16" {
		t.Fatalf("canonical aspect_ratio = %#v", payload["aspect_ratio"])
	}
	if _, exists := payload["ratio"]; exists {
		t.Fatalf("legacy ratio leaked to generic protocol: %+v", payload)
	}
}

func TestSeedanceBridgeAPIKeyNormalizesAccidentalSecondLine(t *testing.T) {
	first := "vs-QkGS8cpP149SujUjaW2vLuWUb4wFoR7zyE1y6atA"
	second := "T45zxdDJ9bonDghQZrOlnfoBL1QL6pfuA5Zi7BmF60w"
	if got := seedanceBridgeAPIKey(first + "\n" + second); got != first {
		t.Fatalf("newline key = %q", got)
	}
	if got := seedanceBridgeAPIKey(first + second); got != first {
		t.Fatalf("joined key = %q", got)
	}
}

func TestDecodeSeedanceBridgeResponseIgnoresSDKNoise(t *testing.T) {
	stdout := []byte("sdk log before\n" + seedanceBridgeResultPrefix + `{"status":"succeeded","videoUrl":"/api/v1/generated-media/video.mp4"}` + "\n-----BEGIN PUBLIC KEY-----\n")
	result, err := decodeSeedanceBridgeResponse(stdout)
	if err != nil {
		t.Fatalf("decode returned error: %v", err)
	}
	if got := result["videoUrl"]; got != "/api/v1/generated-media/video.mp4" {
		t.Fatalf("videoUrl = %#v", got)
	}
}

func TestOpenAICompatibleCanonicalFirstFrameUsesOneImage(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "task-first-frame", "status": "succeeded", "video_url": "https://cdn.example/video.mp4",
		})
	}))
	defer server.Close()

	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		BaseURL: server.URL, APIKey: "sk-test", Model: "video-model", Models: []string{"video-model"},
	})
	_, err := provider.Create(context.Background(), generation.CreateRequest{
		Type: "IMAGE_TO_VIDEO", Model: "video-model", Prompt: "camera push in",
		Params: map[string]any{"first_frame": "https://cdn.example/first.png"},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	imageURLs, ok := payload["image_urls"].([]any)
	if !ok || len(imageURLs) != 1 || imageURLs[0] != "https://cdn.example/first.png" {
		t.Fatalf("image_urls payload = %#v", payload["image_urls"])
	}
	inputReference, ok := payload["input_reference"].(map[string]any)
	if !ok || inputReference["image_url"] != "https://cdn.example/first.png" {
		t.Fatalf("input_reference payload = %#v", payload["input_reference"])
	}
}

func TestSeedanceContentItemsPreserveFirstAndLastFrameRoles(t *testing.T) {
	items := seedanceContentItems(
		"a flower blooming",
		"https://cdn.example/first.png",
		"https://cdn.example/last.png",
	)
	if len(items) != 3 {
		t.Fatalf("content item count = %d, items=%#v", len(items), items)
	}
	if items[1]["role"] != "first_frame" || items[2]["role"] != "last_frame" {
		t.Fatalf("frame roles were not preserved: %#v", items)
	}
}

func TestGrokRejectsMultipleImagesBeforeCallingUpstream(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "task-grok", "status": "succeeded", "video_url": "https://cdn.example/video.mp4",
		})
	}))
	defer server.Close()

	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		BaseURL: server.URL, APIKey: "sk-test", Model: "grok-video-1.5", Models: []string{"grok-video-1.5"},
	})
	_, err := provider.Create(context.Background(), generation.CreateRequest{
		Type: "IMAGE_TO_VIDEO", Model: "grok-video-1.5", Prompt: "camera push in",
		Params: map[string]any{
			"image_urls": []any{"https://cdn.example/first.png", "https://cdn.example/second.png"},
		},
	})
	if err == nil {
		t.Fatal("multiple images were silently truncated")
	}
	if requestCount != 0 {
		t.Fatalf("upstream was called %d times for an invalid request", requestCount)
	}
}

func TestOpenAICompatibleSeedanceCanonicalFirstFrameUsesStringInputReference(t *testing.T) {
	var requestPath string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		if r.Method != http.MethodPost || r.URL.Path != "/v1/videos/generations" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "task-seedance-first-frame", "status": "succeeded", "video_url": "https://cdn.example/video.mp4",
		})
	}))
	defer server.Close()

	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		BaseURL: server.URL, APIKey: "sk-test", Model: "doubao-seedance-2.0", Models: []string{"doubao-seedance-2.0"}, Endpoint: "/v1/videos/generations",
	})
	_, err := provider.Create(context.Background(), generation.CreateRequest{
		Type: "IMAGE_TO_VIDEO", Model: "doubao-seedance-2.0", Prompt: "camera push in",
		Params: map[string]any{"first_frame": "https://cdn.example/first.png"},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if requestPath != "/v1/videos/generations" {
		t.Fatalf("request path = %q", requestPath)
	}
	if got := payload["input_reference"]; got != "https://cdn.example/first.png" {
		t.Fatalf("input_reference payload = %#v, want URL string", got)
	}
}
