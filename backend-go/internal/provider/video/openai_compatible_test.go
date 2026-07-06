package video

import (
	"context"
	"encoding/json"
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
			"duration":   5,
			"ratio":      "16:9",
			"resolution": "1080p",
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
			"ratio":          "16:9",
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
	if second["type"] != "image_url" || second["role"] != "reference_image" {
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
