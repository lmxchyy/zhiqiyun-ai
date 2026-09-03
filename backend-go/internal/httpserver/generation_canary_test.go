package httpserver

import (
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
)

func stage0CanaryConfig() config.Config {
	return config.Config{
		AsyncMessagingEnabled: true, GenerationAsyncCanaryEnabled: true, ProviderExecutionSafetyEnabled: true,
		GenerationAsyncCanaryUsers:             "user-canary",
		GenerationAsyncCanaryProviderAllowlist: "channel-stage0",
		GenerationAsyncCanaryModelAllowlist:    "gpt-image-2",
	}
}

func stage0CanaryRequest() generation.CreateRequest {
	return generation.CreateRequest{UserID: "user-canary", Type: "TEXT_TO_IMAGE", Model: "gpt-image-2", Params: map[string]any{"provider": "channel-stage0"}}
}

func TestTEST_A_AllowedImageSelectsAsync(t *testing.T) {
	resetAsyncCanaryProcessMetricsForTest()
	for _, taskType := range []string{"TEXT_TO_IMAGE", "IMAGE_TO_IMAGE"} {
		req := stage0CanaryRequest()
		req.Type = taskType
		selected, reason := (api{cfg: stage0CanaryConfig()}).generationAsyncCanaryDecision(req)
		if !selected || reason != canaryReasonSelected {
			t.Errorf("type %q selected=%v reason=%s", taskType, selected, reason)
		}
	}
}

func TestTEST_B_ProviderDeniedFallsBackSync(t *testing.T) {
	cfg := stage0CanaryConfig()
	cfg.GenerationAsyncCanaryProviderAllowlist = "other-provider"
	selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(stage0CanaryRequest())
	if selected || reason != canaryReasonRejectedProvider {
		t.Fatalf("provider denial must return false for existing synchronous path: selected=%v reason=%s", selected, reason)
	}
}

func TestTEST_C_ModelDeniedFallsBackSync(t *testing.T) {
	cfg := stage0CanaryConfig()
	cfg.GenerationAsyncCanaryModelAllowlist = "other-model"
	selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(stage0CanaryRequest())
	if selected || reason != canaryReasonRejectedModel {
		t.Fatalf("model denial must return false for existing synchronous path: selected=%v reason=%s", selected, reason)
	}
}

func TestTEST_D_UserDeniedFallsBackSync(t *testing.T) {
	for name, cfg := range map[string]config.Config{
		"not allowlisted": func() config.Config { c := stage0CanaryConfig(); c.GenerationAsyncCanaryUsers = "other-user"; return c }(),
		"empty allowlist": {AsyncMessagingEnabled: true, GenerationAsyncCanaryEnabled: true, ProviderExecutionSafetyEnabled: true,
			GenerationAsyncCanaryProviderAllowlist: "channel-stage0", GenerationAsyncCanaryModelAllowlist: "gpt-image-2", GenerationAsyncCanaryUsers: ""},
	} {
		selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(stage0CanaryRequest())
		if selected || reason != canaryReasonRejectedUser {
			t.Errorf("%s selected=%v reason=%s", name, selected, reason)
		}
	}
}

func TestTEST_E_EmptyProviderAllowlistFailsClosed(t *testing.T) {
	cfg := stage0CanaryConfig()
	cfg.GenerationAsyncCanaryProviderAllowlist = ""
	selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(stage0CanaryRequest())
	if selected || reason != canaryReasonRejectedProvider {
		t.Fatalf("empty provider allowlist selected=%v reason=%s", selected, reason)
	}
}

func TestTEST_F_EmptyModelAllowlistFailsClosed(t *testing.T) {
	cfg := stage0CanaryConfig()
	cfg.GenerationAsyncCanaryModelAllowlist = ""
	selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(stage0CanaryRequest())
	if selected || reason != canaryReasonRejectedModel {
		t.Fatalf("empty model allowlist selected=%v reason=%s", selected, reason)
	}
}

func TestTEST_G_VideoNeverSelectedForStage0(t *testing.T) {
	for _, taskType := range []string{"TEXT_TO_VIDEO", "IMAGE_TO_VIDEO", "VIDEO_TO_VIDEO"} {
		req := stage0CanaryRequest()
		req.Type = taskType
		selected, reason := (api{cfg: stage0CanaryConfig()}).generationAsyncCanaryDecision(req)
		if selected || reason != canaryReasonRejectedType {
			t.Errorf("type %q selected=%v reason=%s", taskType, selected, reason)
		}
	}
}

func TestTEST_H_UnsupportedGenerationTypesNeverSelected(t *testing.T) {
	for _, taskType := range []string{"PPT", "CONNECTOR", "", "AUDIO", "UNKNOWN_KIND"} {
		req := stage0CanaryRequest()
		req.Type = taskType
		selected, reason := (api{cfg: stage0CanaryConfig()}).generationAsyncCanaryDecision(req)
		if selected || reason != canaryReasonRejectedType {
			t.Errorf("type %q selected=%v reason=%s", taskType, selected, reason)
		}
	}
}

func TestGenerationAsyncCanaryEligibilityIsServerControlled(t *testing.T) {
	req := stage0CanaryRequest()
	if !(api{cfg: stage0CanaryConfig()}).generationAsyncCanaryEligible(req) {
		t.Fatal("allowlisted real image should be eligible")
	}
	for name, cfg := range map[string]config.Config{
		"async disabled":  {GenerationAsyncCanaryEnabled: true, ProviderExecutionSafetyEnabled: true},
		"canary disabled": {AsyncMessagingEnabled: true, ProviderExecutionSafetyEnabled: true},
		"safety disabled": {AsyncMessagingEnabled: true, GenerationAsyncCanaryEnabled: true},
		"not allowlisted": func() config.Config { c := stage0CanaryConfig(); c.GenerationAsyncCanaryUsers = "other"; return c }(),
	} {
		if (api{cfg: cfg}).generationAsyncCanaryEligible(req) {
			t.Errorf("%s should not be eligible", name)
		}
	}
}

func TestTEST_N_KillSwitchStopsNewAsyncPreservesDrain(t *testing.T) {
	cfg := stage0CanaryConfig()
	cfg.GenerationAsyncCanaryEnabled = false
	if (api{cfg: cfg}).generationAsyncCanaryEligible(stage0CanaryRequest()) {
		t.Fatal("canary kill switch must stop new async selection")
	}
	if !generationCanaryDrainEnabled(cfg) {
		t.Fatal("global async runtime must remain enabled to drain existing outbox/messages")
	}
	cfg.AsyncMessagingEnabled = false
	if generationCanaryDrainEnabled(cfg) {
		t.Fatal("global async kill switch must stop publisher/consumer")
	}
}
