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

func TestNewOpenAICompatibleForModelsAcceptsKnowledgeAgentModels(t *testing.T) {
	provider := NewOpenAICompatibleForModels(config.Config{
		PPTProviderURL:    "https://example.com/v1",
		PPTProviderAPIKey: "test-key",
		PPTTextModel:      "kimi-k2.6",
	}, "deepseek-v4-flash", "gpt-5.2-chat-latest")

	for _, model := range []string{"kimi-k2.6", "deepseek-v4-flash", "gpt-5.2-chat-latest"} {
		if !provider.Supports(generation.CreateRequest{Type: "AGENT_CHAT", Model: model}) {
			t.Fatalf("provider rejected knowledge-agent model %q", model)
		}
	}
	if provider.Supports(generation.CreateRequest{Type: "AGENT_CHAT", Model: "unconfigured-model"}) {
		t.Fatal("provider unexpectedly accepted an unconfigured knowledge-agent model")
	}
}
