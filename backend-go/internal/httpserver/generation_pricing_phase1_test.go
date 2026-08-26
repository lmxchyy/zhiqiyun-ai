package httpserver

import (
	"errors"
	"path/filepath"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
)

func TestGenerationQuoteRequiresPublishedRule(t *testing.T) {
	data := normalizeAICapabilityDefaults(adminPlatformData{})
	_, err := generationQuoteForRequest(createGenerationTaskRequest{
		Type: "TEXT_TO_IMAGE", Model: "not-published", Params: map[string]any{"n": float64(1)},
	}, data)
	var coded interface{ BusinessCode() string }
	if !errors.As(err, &coded) || coded.BusinessCode() != "PRICING_RULE_NOT_FOUND" {
		t.Fatalf("error=%v, code=%v", err, coded)
	}
}

func TestGenerationQuoteDoesNotUseCodeDefaultWhenPublishedRulesExist(t *testing.T) {
	data := adminPlatformData{
		BillingRuleVersions: []billingRuleVersion{{
			ID: "published-image-v7", ModelCode: "published-image", ModuleCode: moduleImageGeneration,
			BillingUnit: "PER_IMAGE", BasePrice: 260, MinimumCharge: 1, Version: 7,
			Status: "PUBLISHED", EffectiveFrom: "2020-01-01T00:00:00Z",
		}},
		BillingRules: []adminBillingRule{{
			ID: "published-image-v7", ModelName: "published-image", ModuleCode: moduleImageGeneration,
			BillingType: "per_image", BasePrice: 260, Status: "ACTIVE", Version: 7,
		}},
	}
	if _, err := generationQuoteForRequest(createGenerationTaskRequest{
		Type: "TEXT_TO_IMAGE", Model: "gpt-image-2", Params: map[string]any{"n": 1},
	}, data); err == nil {
		t.Fatal("code default model must not be used when published pricing rules exist")
	}
}

func TestGenerationCaptureSnapshotRequiresStoredPointCost(t *testing.T) {
	if _, err := generationTaskSnapshotPointCost(generationTask{}); err == nil {
		t.Fatal("missing billing snapshot must fail instead of recalculating current price")
	}
}

func TestUnknownGenerationModelFailsBeforeTaskCreation(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	before, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreatePendingGenerationTask(generation.CreateRequest{
		Type: "TEXT_TO_VIDEO", Model: "not-published", UserID: "user_000002",
		Prompt: "should not run", ClientRequestID: "pricing-unknown-model",
		Params: map[string]any{"duration": float64(5)},
	})
	var coded interface{ BusinessCode() string }
	if !errors.As(err, &coded) || coded.BusinessCode() != "PRICING_RULE_NOT_FOUND" {
		t.Fatalf("error=%v, code=%v", err, coded)
	}
	after, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("unknown model created task: before=%d after=%d", len(before), len(after))
	}
}
