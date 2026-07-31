package video

import (
	"context"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
)

func TestMockProviderPreservesCanonicalVideoParameters(t *testing.T) {
	result, err := NewMockProvider().Create(context.Background(), generation.CreateRequest{
		Model: "mock-video",
		Params: map[string]any{
			"duration":        10,
			"resolution":      "1080p",
			"aspect_ratio":    "9:16",
			"fps":             30,
			"generate_audio":  false,
			"motion_strength": "high",
			"camera_movement": "push",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := result.(map[string]any)
	metadata := payload["metadata"].(map[string]any)
	for key, want := range map[string]any{
		"duration":        10,
		"resolution":      "1080p",
		"aspect_ratio":    "9:16",
		"fps":             30,
		"generate_audio":  false,
		"motion_strength": "high",
		"camera_movement": "push",
	} {
		if got := metadata[key]; got != want {
			t.Fatalf("metadata %s = %#v, want %#v", key, got, want)
		}
	}
	if _, exists := metadata["ratio"]; exists {
		t.Fatalf("legacy ratio leaked into mock metadata: %+v", metadata)
	}
}
