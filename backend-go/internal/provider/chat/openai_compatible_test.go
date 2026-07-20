package chat

import (
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
)

func TestNewOpenAICompatibleForModelAcceptsCapabilitySelectedModel(t *testing.T) {
	provider := NewOpenAICompatibleForModel(config.Config{
		PPTProviderURL:    "https://example.com/v1",
		PPTProviderAPIKey: "test-key",
		PPTTextModel:      "environment-default",
	}, "tenant-selected-model")

	if provider.DefaultModel() != "tenant-selected-model" {
		t.Fatalf("default model = %q", provider.DefaultModel())
	}
	if !provider.Supports(generation.CreateRequest{Type: "CHAT_COMPLETION", Model: "tenant-selected-model"}) {
		t.Fatal("provider rejected the capability-selected model")
	}
	if provider.Supports(generation.CreateRequest{Type: "CHAT_COMPLETION", Model: "environment-default"}) {
		t.Fatal("provider unexpectedly accepted a model outside the selected model boundary")
	}
}

func TestNewOpenAICompatibleForModelFallsBackToEnvironmentDefault(t *testing.T) {
	provider := NewOpenAICompatibleForModel(config.Config{
		PPTProviderURL:    "https://example.com/v1",
		PPTProviderAPIKey: "test-key",
		PPTTextModel:      "environment-default",
	}, "")

	if provider.DefaultModel() != "environment-default" {
		t.Fatalf("default model = %q", provider.DefaultModel())
	}
}
