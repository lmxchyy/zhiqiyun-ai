package httpserver

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGPTImageBillingRulePhase26Draft(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	versions, err := store.ListBillingRuleVersions()
	if err != nil {
		t.Fatalf("list billing rule versions: %v", err)
	}
	var published billingRuleVersion
	for _, item := range versions {
		if item.RuleKey == "billing_rule_image_gpt" && item.Status == "PUBLISHED" {
			published = item
			break
		}
	}
	if published.ID == "" {
		t.Fatal("published billing_rule_image_gpt not found")
	}

	draftProjection, err := store.UpdateAdminBillingRule(published.ID, adminBillingRuleMutation{
		BillingType:         "per_image",
		BasePrice:           gptImage2Phase1BasePrice,
		MinimumCharge:       gptImage2Phase1MinimumCharge,
		ParameterMultiplier: gptImage2Phase1BillingParameterRules(),
		Status:              "DRAFT",
	})
	if err != nil {
		t.Fatalf("create billing_rule_image_gpt draft: %v", err)
	}
	if draftProjection.Status == "PUBLISHED" || draftProjection.Status == "ACTIVE" && draftProjection.ID == published.ID {
		t.Fatalf("draft must not replace published: %+v", draftProjection)
	}

	versions, err = store.ListBillingRuleVersions()
	if err != nil {
		t.Fatalf("reload versions: %v", err)
	}
	var draft billingRuleVersion
	publishedCount := 0
	for _, item := range versions {
		if item.RuleKey != "billing_rule_image_gpt" {
			continue
		}
		if item.Status == "PUBLISHED" {
			publishedCount++
			if item.ID != published.ID {
				t.Fatalf("published id changed to %s, draft must not publish", item.ID)
			}
		}
		if item.Status == "DRAFT" && item.Version > published.Version {
			draft = item
		}
	}
	if publishedCount != 1 {
		t.Fatalf("published billing_rule_image_gpt count = %d, want 1", publishedCount)
	}
	if draft.ID == "" {
		t.Fatal("phase 2.6 draft not found")
	}

	validation, err := store.ValidateBillingRuleVersion(draft.ID)
	if err != nil {
		t.Fatalf("validate draft: %v", err)
	}
	if !validation.Valid {
		t.Fatalf("draft validation failed: %+v", validation.Issues)
	}
	draft, err = store.GetBillingRuleVersion(draft.ID)
	if err != nil {
		t.Fatalf("reload validated draft: %v", err)
	}

	raw, err := json.MarshalIndent(draft, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("billing_rule_image_gpt DRAFT JSON:\n%s", raw)
	t.Logf("basePrice=%v minimumCharge=%v", draft.BasePrice, draft.MinimumCharge)
	t.Logf("parameterRules=%#v", draft.ParameterRules)
	t.Logf("n is billed as per_image quantity only; no n multiplier key")
	t.Logf("auto quality/size temporarily billed as 1K medium (55); do not back-charge")

	if draft.BasePrice != gptImage2Phase1BasePrice || draft.MinimumCharge != gptImage2Phase1MinimumCharge {
		t.Fatalf("draft prices = %v/%v, want %v/%v", draft.BasePrice, draft.MinimumCharge, gptImage2Phase1BasePrice, gptImage2Phase1MinimumCharge)
	}
	if _, ok := draft.ParameterRules["n"]; ok {
		t.Fatal("draft must not include an n multiplier")
	}

	wantMatrix := map[string][]int{
		"low":    {10, 20, 30, 40},
		"medium": {55, 110, 165, 220},
		"high":   {220, 440, 660, 880},
		"auto":   {55, 110, 165, 220},
	}
	sizes := []string{"1024x1024", "1024x1536", "1536x1024"}
	phase1Rule := adminBillingRule{
		ID:                  "billing_rule_image_gpt",
		ModuleCode:          moduleImageGeneration,
		ModelName:           "gpt-image-2",
		BillingType:         "per_image",
		BasePrice:           gptImage2Phase1BasePrice,
		MinimumCharge:       gptImage2Phase1MinimumCharge,
		ParameterMultiplier: gptImage2Phase1BillingParameterRules(),
		Status:              "ACTIVE",
	}
	data := normalizeAICapabilityDefaults(adminPlatformData{BillingRules: []adminBillingRule{phase1Rule}})
	for _, size := range sizes {
		for quality, expectedByN := range wantMatrix {
			for i, n := range []int{1, 2, 3, 4} {
				req := createGenerationTaskRequest{
					Type:  "TEXT_TO_IMAGE",
					Model: "gpt-image-2",
					Params: map[string]any{
						"size":    size,
						"quality": quality,
						"n":       float64(n),
					},
				}
				got := generationPointCostForRequest(req, data)
				if got != expectedByN[i] {
					t.Fatalf("%s %s n=%d quoted %d, want %d", size, quality, n, got, expectedByN[i])
				}
			}
		}
	}
	autoSize := generationPointCostForRequest(createGenerationTaskRequest{
		Type:  "TEXT_TO_IMAGE",
		Model: "gpt-image-2",
		Params: map[string]any{
			"size":    "auto",
			"quality": "auto",
			"n":       float64(1),
		},
	}, data)
	if autoSize != 55 {
		t.Fatalf("size=auto quality=auto quoted %d, want 55", autoSize)
	}
	customTwoK := generationPointCostForRequest(createGenerationTaskRequest{
		Type:  "TEXT_TO_IMAGE",
		Model: "gpt-image-2",
		Params: map[string]any{
			"size":    "1792x1024",
			"quality": "low",
			"n":       float64(1),
		},
	}, data)
	if customTwoK != 15 {
		t.Fatalf("custom 1792x1024 low quoted %d, want 15 (tier_2k)", customTwoK)
	}

	t.Logf("PUBLISHED vs DRAFT diff: basePrice %v -> %v; minimumCharge %v -> %v", published.BasePrice, draft.BasePrice, published.MinimumCharge, draft.MinimumCharge)
	t.Logf("PUBLISHED parameterRules=%#v", published.ParameterRules)
	t.Logf("DRAFT parameterRules=%#v", draft.ParameterRules)
	if reflect.DeepEqual(published.ParameterRules, draft.ParameterRules) {
		t.Fatal("expected published vs draft parameterRules to differ")
	}

	after, err := store.ListBillingRuleVersions()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range after {
		if item.RuleKey == "billing_rule_image_gpt" && item.ID == draft.ID && item.Status == "PUBLISHED" {
			t.Fatal("phase 2.6 draft was published")
		}
	}
}
