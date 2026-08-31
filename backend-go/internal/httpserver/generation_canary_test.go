package httpserver

import (
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
)

func TestGenerationAsyncCanaryEligibilityIsServerControlled(t *testing.T) {
	req := generation.CreateRequest{UserID: "user-canary", Type: "TEXT_TO_IMAGE", Model: "gpt-image-2", Params: map[string]any{"generation_async_canary": true}}
	base := api{cfg: config.Config{AsyncMessagingEnabled: true, GenerationAsyncCanaryEnabled: true, ProviderExecutionSafetyEnabled: true, GenerationAsyncCanaryUsers: "user-canary"}}
	if !base.generationAsyncCanaryEligible(req) {
		t.Fatal("allowlisted real image should be eligible")
	}
	for name, cfg := range map[string]config.Config{
		"async disabled":    {GenerationAsyncCanaryEnabled: true, ProviderExecutionSafetyEnabled: true, GenerationAsyncCanaryUsers: "user-canary"},
		"canary disabled":   {AsyncMessagingEnabled: true, ProviderExecutionSafetyEnabled: true, GenerationAsyncCanaryUsers: "user-canary"},
		"safety disabled":   {AsyncMessagingEnabled: true, GenerationAsyncCanaryEnabled: true, GenerationAsyncCanaryUsers: "user-canary"},
		"not allowlisted":   {AsyncMessagingEnabled: true, GenerationAsyncCanaryEnabled: true, ProviderExecutionSafetyEnabled: true, GenerationAsyncCanaryUsers: "other"},
		"mock remains sync": {AsyncMessagingEnabled: true, GenerationAsyncCanaryEnabled: true, ProviderExecutionSafetyEnabled: true, GenerationAsyncCanaryUsers: "user-canary"},
	} {
		candidate := req
		if name == "mock remains sync" {
			candidate.Model = "mock-standard"
		}
		if (api{cfg: cfg}).generationAsyncCanaryEligible(candidate) {
			t.Errorf("%s should not be eligible", name)
		}
	}
}
