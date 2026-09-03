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

func TestTEST_01_WildcardTextToImageSelectsAsync(t *testing.T) {
	cfg := stage0CanaryConfig()
	cfg.GenerationAsyncCanaryUsers = "*"
	req := stage0CanaryRequest()
	req.Type = "TEXT_TO_IMAGE"
	req.UserID = "any_user_123"
	selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(req)
	if !selected || reason != canaryReasonSelected {
		t.Fatalf("wildcard users must select async for text to image: selected=%v reason=%s", selected, reason)
	}
}

func TestTEST_02_WildcardImageToImageSelectsAsync(t *testing.T) {
	cfg := stage0CanaryConfig()
	cfg.GenerationAsyncCanaryUsers = "*"
	req := stage0CanaryRequest()
	req.Type = "IMAGE_TO_IMAGE"
	req.UserID = "any_user_456"
	selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(req)
	if !selected || reason != canaryReasonSelected {
		t.Fatalf("wildcard users must select async for image to image: selected=%v reason=%s", selected, reason)
	}
}

func TestTEST_03_WildcardProviderDeniedFallsBackSync(t *testing.T) {
	cfg := stage0CanaryConfig()
	cfg.GenerationAsyncCanaryUsers = "*"
	cfg.GenerationAsyncCanaryProviderAllowlist = "other-provider"
	req := stage0CanaryRequest()
	req.UserID = "any_user"
	selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(req)
	if selected || reason != canaryReasonRejectedProvider {
		t.Fatalf("wildcard users must not bypass provider denial: selected=%v reason=%s", selected, reason)
	}
}

func TestTEST_04_WildcardModelDeniedFallsBackSync(t *testing.T) {
	cfg := stage0CanaryConfig()
	cfg.GenerationAsyncCanaryUsers = "*"
	cfg.GenerationAsyncCanaryModelAllowlist = "other-model"
	req := stage0CanaryRequest()
	req.UserID = "any_user"
	selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(req)
	if selected || reason != canaryReasonRejectedModel {
		t.Fatalf("wildcard users must not bypass model denial: selected=%v reason=%s", selected, reason)
	}
}

func TestTEST_05_WildcardVideoNeverSelected(t *testing.T) {
	cfg := stage0CanaryConfig()
	cfg.GenerationAsyncCanaryUsers = "*"
	for _, taskType := range []string{"TEXT_TO_VIDEO", "IMAGE_TO_VIDEO", "VIDEO_TO_VIDEO"} {
		req := stage0CanaryRequest()
		req.Type = taskType
		req.UserID = "any_user"
		selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(req)
		if selected || reason != canaryReasonRejectedType {
			t.Errorf("type %q must not be selected: selected=%v reason=%s", taskType, selected, reason)
		}
	}
}

func TestTEST_06_EmptyUsersFailsClosed(t *testing.T) {
	cfg := stage0CanaryConfig()
	cfg.GenerationAsyncCanaryUsers = ""
	req := stage0CanaryRequest()
	req.UserID = "user_000002"
	selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(req)
	if selected || reason != canaryReasonRejectedUser {
		t.Fatalf("empty users allowlist must fail closed: selected=%v reason=%s", selected, reason)
	}
}

func TestTEST_07_ExplicitUserMatches(t *testing.T) {
	cfg := stage0CanaryConfig()
	cfg.GenerationAsyncCanaryUsers = "user_000002"
	req := stage0CanaryRequest()
	req.UserID = "user_000002"
	selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(req)
	if !selected || reason != canaryReasonSelected {
		t.Fatalf("explicit matching user must be selected: selected=%v reason=%s", selected, reason)
	}
}

func TestTEST_08_ExplicitUserMismatchedRejected(t *testing.T) {
	cfg := stage0CanaryConfig()
	cfg.GenerationAsyncCanaryUsers = "user_000002"
	req := stage0CanaryRequest()
	req.UserID = "user_other"
	selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(req)
	if selected || reason != canaryReasonRejectedUser {
		t.Fatalf("mismatched user must be rejected: selected=%v reason=%s", selected, reason)
	}
}

func TestTEST_09_KillSwitchWithWildcardUsers(t *testing.T) {
	cfg := stage0CanaryConfig()
	cfg.GenerationAsyncCanaryEnabled = false
	cfg.GenerationAsyncCanaryUsers = "*"
	req := stage0CanaryRequest()
	req.UserID = "any_user"
	selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(req)
	if selected || reason != canaryReasonDisabled {
		t.Fatalf("canary kill switch must stop async even with wildcard users: selected=%v reason=%s", selected, reason)
	}
}

func TestTEST_10_MixedCSVWithWildcard(t *testing.T) {
	cfg := stage0CanaryConfig()
	cfg.GenerationAsyncCanaryUsers = "user_000002,*"
	for _, uid := range []string{"user_000002", "user_000003", "random_user"} {
		req := stage0CanaryRequest()
		req.UserID = uid
		selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(req)
		if !selected || reason != canaryReasonSelected {
			t.Errorf("mixed csv with wildcard must allow user %s: selected=%v reason=%s", uid, selected, reason)
		}
	}
}

func TestTEST_11_TokensLikeAllNotWildcard(t *testing.T) {
	for _, token := range []string{" ALL ", "all", "ALL"} {
		cfg := stage0CanaryConfig()
		cfg.GenerationAsyncCanaryUsers = token
		req := stage0CanaryRequest()
		req.UserID = "user_other"
		selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(req)
		if selected || reason != canaryReasonRejectedUser {
			t.Errorf("token %q must not act as wildcard: selected=%v reason=%s", token, selected, reason)
		}
	}
}

func TestTEST_12_TokenTrueNotWildcard(t *testing.T) {
	cfg := stage0CanaryConfig()
	cfg.GenerationAsyncCanaryUsers = "true"
	req := stage0CanaryRequest()
	req.UserID = "user_other"
	selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(req)
	if selected || reason != canaryReasonRejectedUser {
		t.Fatalf("token 'true' must not act as wildcard: selected=%v reason=%s", selected, reason)
	}
}

func TestProviderAndModelAllowlistsDoNotInheritWildcard(t *testing.T) {
	cfg := stage0CanaryConfig()
	cfg.GenerationAsyncCanaryUsers = "user-canary"
	cfg.GenerationAsyncCanaryProviderAllowlist = "*"
	req := stage0CanaryRequest()
	// req provider is "channel-stage0", provider allowlist is literally "*"
	selected, reason := (api{cfg: cfg}).generationAsyncCanaryDecision(req)
	if selected || reason != canaryReasonRejectedProvider {
		t.Fatalf("provider allowlist must not inherit wildcard semantics: selected=%v reason=%s", selected, reason)
	}

	cfg = stage0CanaryConfig()
	cfg.GenerationAsyncCanaryUsers = "user-canary"
	cfg.GenerationAsyncCanaryModelAllowlist = "*"
	// req model is "gpt-image-2", model allowlist is literally "*"
	selected, reason = (api{cfg: cfg}).generationAsyncCanaryDecision(req)
	if selected || reason != canaryReasonRejectedModel {
		t.Fatalf("model allowlist must not inherit wildcard semantics: selected=%v reason=%s", selected, reason)
	}
}

