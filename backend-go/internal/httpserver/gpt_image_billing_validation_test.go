package httpserver

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// validGPTImageTestParameterRules returns a complete, structurally valid parameter rules map
// including all required tiers (tier_1k, tier_2k, tier_4k) and qualities (low, normal, high).
func validGPTImageTestParameterRules() map[string]any {
	return map[string]any{
		"quality": map[string]any{
			"low":    float64(1.0),
			"normal": float64(1.2),
			"medium": float64(1.2),
			"high":   float64(1.5),
			"auto":   float64(1.0),
		},
		"size": map[string]any{
			"auto":      float64(1.0),
			"tier_720p": float64(1.0),
			"tier_1k":   float64(1.0),
			"tier_2k":   float64(1.5),
			"tier_4k":   float64(2.0),
			"1024x1024": float64(1.0),
			"2048x2048": float64(1.5),
			"3840x2160": float64(2.0),
		},
	}
}

// 1. basePrice <= 0 -> 拒绝
func TestGPTImageValidation_BasePriceNonPositiveRejected(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	published, err := store.GetBillingRuleVersion("brv_billing_rule_image_gpt_v1")
	if err != nil {
		t.Fatalf("get published v1: %v", err)
	}

	for _, badPrice := range []float64{0, -1, -10} {
		draft, err := store.UpdateAdminBillingRule(published.ID, adminBillingRuleMutation{
			BillingType:         "per_image",
			BasePrice:           badPrice,
			BasePriceExplicit:   true,
			MinimumCharge:       1,
			ParameterMultiplier: validGPTImageTestParameterRules(),
			Status:              "DRAFT",
		})
		if err != nil {
			t.Fatalf("create draft with basePrice=%.0f: %v", badPrice, err)
		}

		validation, err := store.ValidateBillingRuleVersion(draft.ID)
		if err != nil {
			t.Fatalf("validate draft: %v", err)
		}
		if validation.Valid {
			t.Fatalf("expected validation.Valid=false for basePrice=%.0f", badPrice)
		}

		foundInvalidBasePrice := false
		for _, issue := range validation.Issues {
			if issue.Code == "INVALID_BASE_PRICE" {
				foundInvalidBasePrice = true
				if issue.Severity != "ERROR" {
					t.Fatalf("INVALID_BASE_PRICE severity=%s, want ERROR", issue.Severity)
				}
			}
		}
		if !foundInvalidBasePrice {
			t.Fatalf("expected INVALID_BASE_PRICE in issues for basePrice=%.0f: %+v", badPrice, validation.Issues)
		}

		// Even with confirmNegativeMargin=true, hard blocker basePrice <= 0 cannot be published
		_, err = store.PublishBillingRuleVersion(draft.ID, publishBillingRuleRequest{ConfirmNegativeMargin: true})
		if err == nil {
			t.Fatalf("expected publish to be rejected for basePrice=%.0f", badPrice)
		}
		var pubErr *billingRulePublishError
		if !errors.As(err, &pubErr) || pubErr.Code != "BILLING_RULE_VALIDATION_FAILED" {
			t.Fatalf("expected BILLING_RULE_VALIDATION_FAILED, got: %v", err)
		}
	}
}

// 2. 缺 tier_2k -> 拒绝
func TestGPTImageValidation_MissingTier2KRejected(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	published, err := store.GetBillingRuleVersion("brv_billing_rule_image_gpt_v1")
	if err != nil {
		t.Fatalf("get published v1: %v", err)
	}

	rules := validGPTImageTestParameterRules()
	sizeRules := cloneAnyMap(rules["size"].(map[string]any))
	delete(sizeRules, "tier_2k")
	rules["size"] = sizeRules

	draft, err := store.UpdateAdminBillingRule(published.ID, adminBillingRuleMutation{
		BillingType:         "per_image",
		BasePrice:           15,
		MinimumCharge:       1,
		ParameterMultiplier: rules,
		Status:              "DRAFT",
	})
	if err != nil {
		t.Fatalf("create draft without tier_2k: %v", err)
	}

	validation, err := store.ValidateBillingRuleVersion(draft.ID)
	if err != nil {
		t.Fatalf("validate draft: %v", err)
	}
	if validation.Valid {
		t.Fatal("expected validation.Valid=false when tier_2k is missing")
	}

	foundMissingTier2K := false
	for _, issue := range validation.Issues {
		if issue.Code == "MISSING_TIER_2K" {
			foundMissingTier2K = true
			if issue.Severity != "ERROR" {
				t.Fatalf("MISSING_TIER_2K severity=%s, want ERROR", issue.Severity)
			}
		}
	}
	if !foundMissingTier2K {
		t.Fatalf("expected MISSING_TIER_2K in issues: %+v", validation.Issues)
	}

	// Even with confirmNegativeMargin=true, hard blocker missing tier_2k cannot be published
	_, err = store.PublishBillingRuleVersion(draft.ID, publishBillingRuleRequest{ConfirmNegativeMargin: true})
	if err == nil {
		t.Fatal("expected publish to be rejected when tier_2k is missing")
	}
	var pubErr *billingRulePublishError
	if !errors.As(err, &pubErr) || pubErr.Code != "BILLING_RULE_VALIDATION_FAILED" {
		t.Fatalf("expected BILLING_RULE_VALIDATION_FAILED, got: %v", err)
	}
}

// 3. 缺 tier_4k -> 拒绝
func TestGPTImageValidation_MissingTier4KRejected(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	published, err := store.GetBillingRuleVersion("brv_billing_rule_image_gpt_v1")
	if err != nil {
		t.Fatalf("get published v1: %v", err)
	}

	rules := validGPTImageTestParameterRules()
	sizeRules := cloneAnyMap(rules["size"].(map[string]any))
	delete(sizeRules, "tier_4k")
	rules["size"] = sizeRules

	draft, err := store.UpdateAdminBillingRule(published.ID, adminBillingRuleMutation{
		BillingType:         "per_image",
		BasePrice:           15,
		MinimumCharge:       1,
		ParameterMultiplier: rules,
		Status:              "DRAFT",
	})
	if err != nil {
		t.Fatalf("create draft without tier_4k: %v", err)
	}

	validation, err := store.ValidateBillingRuleVersion(draft.ID)
	if err != nil {
		t.Fatalf("validate draft: %v", err)
	}
	if validation.Valid {
		t.Fatal("expected validation.Valid=false when tier_4k is missing")
	}

	foundMissingTier4K := false
	for _, issue := range validation.Issues {
		if issue.Code == "MISSING_TIER_4K" {
			foundMissingTier4K = true
			if issue.Severity != "ERROR" {
				t.Fatalf("MISSING_TIER_4K severity=%s, want ERROR", issue.Severity)
			}
		}
	}
	if !foundMissingTier4K {
		t.Fatalf("expected MISSING_TIER_4K in issues: %+v", validation.Issues)
	}

	// Even with confirmNegativeMargin=true, hard blocker missing tier_4k cannot be published
	_, err = store.PublishBillingRuleVersion(draft.ID, publishBillingRuleRequest{ConfirmNegativeMargin: true})
	if err == nil {
		t.Fatal("expected publish to be rejected when tier_4k is missing")
	}
	var pubErr *billingRulePublishError
	if !errors.As(err, &pubErr) || pubErr.Code != "BILLING_RULE_VALIDATION_FAILED" {
		t.Fatalf("expected BILLING_RULE_VALIDATION_FAILED, got: %v", err)
	}
}

// 4. multiplier <= 0 -> 拒绝
func TestGPTImageValidation_MultiplierNonPositiveRejected(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	published, err := store.GetBillingRuleVersion("brv_billing_rule_image_gpt_v1")
	if err != nil {
		t.Fatalf("get published v1: %v", err)
	}

	testCases := []struct {
		name       string
		modifyFn   func(map[string]any)
		checkField string
	}{
		{
			name: "tier_2k zero",
			modifyFn: func(m map[string]any) {
				sizes := cloneAnyMap(m["size"].(map[string]any))
				sizes["tier_2k"] = float64(0)
				m["size"] = sizes
			},
			checkField: "parameterRules.size.tier_2k",
		},
		{
			name: "tier_4k negative",
			modifyFn: func(m map[string]any) {
				sizes := cloneAnyMap(m["size"].(map[string]any))
				sizes["tier_4k"] = float64(-1.5)
				m["size"] = sizes
			},
			checkField: "parameterRules.size.tier_4k",
		},
		{
			name: "quality high zero",
			modifyFn: func(m map[string]any) {
				quals := cloneAnyMap(m["quality"].(map[string]any))
				quals["high"] = float64(0)
				m["quality"] = quals
			},
			checkField: "parameterRules.quality.high",
		},
		{
			name: "quality low string invalid",
			modifyFn: func(m map[string]any) {
				quals := cloneAnyMap(m["quality"].(map[string]any))
				quals["low"] = "not_a_number"
				m["quality"] = quals
			},
			checkField: "parameterRules.quality.low",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rules := validGPTImageTestParameterRules()
			tc.modifyFn(rules)

			draft, err := store.UpdateAdminBillingRule(published.ID, adminBillingRuleMutation{
				BillingType:         "per_image",
				BasePrice:           15,
				MinimumCharge:       1,
				ParameterMultiplier: rules,
				Status:              "DRAFT",
			})
			if err != nil {
				t.Fatalf("create draft: %v", err)
			}

			validation, err := store.ValidateBillingRuleVersion(draft.ID)
			if err != nil {
				t.Fatalf("validate draft: %v", err)
			}
			if validation.Valid {
				t.Fatal("expected validation.Valid=false for non-positive multiplier")
			}

			foundError := false
			for _, issue := range validation.Issues {
				if issue.Code == "INVALID_MULTIPLIER" || issue.Code == "INCOMPLETE_PARAMETER_PRICING" {
					foundError = true
				}
			}
			if !foundError {
				t.Fatalf("expected INVALID_MULTIPLIER or INCOMPLETE_PARAMETER_PRICING in issues: %+v", validation.Issues)
			}

			// Cannot publish even with confirmNegativeMargin=true
			_, err = store.PublishBillingRuleVersion(draft.ID, publishBillingRuleRequest{ConfirmNegativeMargin: true})
			if err == nil {
				t.Fatal("expected publish to be rejected for non-positive multiplier")
			}
			var pubErr *billingRulePublishError
			if !errors.As(err, &pubErr) || pubErr.Code != "BILLING_RULE_VALIDATION_FAILED" {
				t.Fatalf("expected BILLING_RULE_VALIDATION_FAILED, got: %v", err)
			}
		})
	}
}

// 5. NEGATIVE_MARGIN + 未确认 -> 拒绝
func TestGPTImageValidation_NegativeMarginUnconfirmedRejected(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	published, err := store.GetBillingRuleVersion("brv_billing_rule_image_gpt_v1")
	if err != nil {
		t.Fatalf("get published v1: %v", err)
	}

	draft, err := store.UpdateAdminBillingRule(published.ID, adminBillingRuleMutation{
		BillingType:         "per_image",
		BasePrice:           10,
		MinimumCharge:       1,
		ParameterMultiplier: validGPTImageTestParameterRules(),
		Status:              "DRAFT",
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	validation, err := store.ValidateBillingRuleVersion(draft.ID)
	if err != nil {
		t.Fatalf("validate draft: %v", err)
	}
	if validation.Valid {
		t.Fatal("expected validation.Valid=false due to NEGATIVE_MARGIN")
	}

	hasNegativeMargin := false
	for _, issue := range validation.Issues {
		if issue.Code == "NEGATIVE_MARGIN" {
			hasNegativeMargin = true
		}
	}
	if !hasNegativeMargin {
		t.Fatalf("expected NEGATIVE_MARGIN in issues: %+v", validation.Issues)
	}

	// 1) Publish without arguments (default false) -> Rejected
	_, err = store.PublishBillingRuleVersion(draft.ID)
	if err == nil {
		t.Fatal("expected publish to be rejected without confirmation")
	}
	var pubErr *billingRulePublishError
	if !errors.As(err, &pubErr) || pubErr.Code != "NEGATIVE_MARGIN_CONFIRMATION_REQUIRED" {
		t.Fatalf("expected NEGATIVE_MARGIN_CONFIRMATION_REQUIRED, got: %v", err)
	}

	// 2) Publish with confirmNegativeMargin=false explicitly -> Rejected
	_, err = store.PublishBillingRuleVersion(draft.ID, publishBillingRuleRequest{ConfirmNegativeMargin: false})
	if err == nil {
		t.Fatal("expected publish to be rejected with confirmNegativeMargin=false")
	}
	if !errors.As(err, &pubErr) || pubErr.Code != "NEGATIVE_MARGIN_CONFIRMATION_REQUIRED" {
		t.Fatalf("expected NEGATIVE_MARGIN_CONFIRMATION_REQUIRED, got: %v", err)
	}

	// Verify draft status is still DRAFT
	current, err := store.GetBillingRuleVersion(draft.ID)
	if err != nil {
		t.Fatalf("get draft: %v", err)
	}
	if current.Status != "DRAFT" {
		t.Fatalf("draft status changed to %s, want DRAFT", current.Status)
	}
}

// 6. NEGATIVE_MARGIN + confirmNegativeMargin=true -> 允许
func TestGPTImageValidation_NegativeMarginConfirmedAllowed(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	published, err := store.GetBillingRuleVersion("brv_billing_rule_image_gpt_v1")
	if err != nil {
		t.Fatalf("get published v1: %v", err)
	}

	draft, err := store.UpdateAdminBillingRule(published.ID, adminBillingRuleMutation{
		BillingType:         "per_image",
		BasePrice:           15,
		MinimumCharge:       1,
		ParameterMultiplier: validGPTImageTestParameterRules(),
		Status:              "DRAFT",
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	// Publish with confirmNegativeMargin=true
	newPublished, err := store.PublishBillingRuleVersion(draft.ID, publishBillingRuleRequest{
		ConfirmNegativeMargin: true,
		ActorID:                "admin_user_001",
		ActorRole:              "SUPER_ADMIN",
	})
	if err != nil {
		t.Fatalf("publish with confirmNegativeMargin=true failed: %v", err)
	}

	if newPublished.Status != "PUBLISHED" {
		t.Fatalf("new status = %s, want PUBLISHED", newPublished.Status)
	}
	if newPublished.ID != draft.ID {
		t.Fatalf("new published ID = %s, want %s", newPublished.ID, draft.ID)
	}
	if newPublished.PublishedAt == "" {
		t.Fatal("expected non-empty publishedAt")
	}

	// NEGATIVE_MARGIN should still be recorded in validation result
	hasNegativeMargin := false
	for _, issue := range newPublished.ValidationResult.Issues {
		if issue.Code == "NEGATIVE_MARGIN" {
			hasNegativeMargin = true
		}
	}
	if !hasNegativeMargin {
		t.Fatalf("NEGATIVE_MARGIN was removed from validation result: %+v", newPublished.ValidationResult.Issues)
	}
}

// 7. NEGATIVE_MARGIN + 同时存在结构错误 + confirm=true -> 仍然拒绝
func TestGPTImageValidation_NegativeMarginPlusStructuralErrorStillRejected(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	published, err := store.GetBillingRuleVersion("brv_billing_rule_image_gpt_v1")
	if err != nil {
		t.Fatalf("get published v1: %v", err)
	}

	// BasePrice 10 has NEGATIVE_MARGIN, AND delete tier_4k to introduce structural error
	rules := validGPTImageTestParameterRules()
	sizeRules := cloneAnyMap(rules["size"].(map[string]any))
	delete(sizeRules, "tier_4k")
	rules["size"] = sizeRules

	draft, err := store.UpdateAdminBillingRule(published.ID, adminBillingRuleMutation{
		BillingType:         "per_image",
		BasePrice:           10,
		MinimumCharge:       1,
		ParameterMultiplier: rules,
		Status:              "DRAFT",
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	validation, err := store.ValidateBillingRuleVersion(draft.ID)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	hasNegativeMargin := false
	hasMissingTier4K := false
	for _, issue := range validation.Issues {
		if issue.Code == "NEGATIVE_MARGIN" {
			hasNegativeMargin = true
		}
		if issue.Code == "MISSING_TIER_4K" {
			hasMissingTier4K = true
		}
	}
	if !hasNegativeMargin || !hasMissingTier4K {
		t.Fatalf("expected both NEGATIVE_MARGIN and MISSING_TIER_4K, got: %+v", validation.Issues)
	}

	// Attempt to publish with confirmNegativeMargin=true -> MUST BE REJECTED
	_, err = store.PublishBillingRuleVersion(draft.ID, publishBillingRuleRequest{
		ConfirmNegativeMargin: true,
	})
	if err == nil {
		t.Fatal("expected publish to be rejected due to hard blocker MISSING_TIER_4K")
	}
	var pubErr *billingRulePublishError
	if !errors.As(err, &pubErr) || pubErr.Code != "BILLING_RULE_VALIDATION_FAILED" {
		t.Fatalf("expected BILLING_RULE_VALIDATION_FAILED, got: %v", err)
	}
}

// 8. PUBLISHED v2 -> 创建 DRAFT v3 -> publish -> v2 ARCHIVED、v3 PUBLISHED
func TestGPTImageValidation_PublishV2ToV3Lifecycle(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))

	// Setup initial state: make v2 PUBLISHED
	err := store.updateAdmin(func(data *adminPlatformData) error {
		*data = normalizeBillingV1Defaults(normalizeAICapabilityDefaults(*data))
		// Ensure v2 exists and is PUBLISHED
		for i := range data.BillingRuleVersions {
			if data.BillingRuleVersions[i].RuleKey == "billing_rule_image_gpt" {
				data.BillingRuleVersions[i].ID = "brv_billing_rule_image_gpt_v2"
				data.BillingRuleVersions[i].Version = 2
				data.BillingRuleVersions[i].Status = "PUBLISHED"
				data.BillingRuleVersions[i].BasePrice = 10
				data.BillingRuleVersions[i].ParameterRules = validGPTImageTestParameterRules()
				data.BillingRuleVersions[i].EffectiveFrom = "2026-09-01T00:00:00Z"
				data.BillingRuleVersions[i].EffectiveTo = ""
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("setup v2: %v", err)
	}

	v2, err := store.GetBillingRuleVersion("brv_billing_rule_image_gpt_v2")
	if err != nil || v2.Status != "PUBLISHED" || v2.Version != 2 {
		t.Fatalf("v2 setup failed: %+v, err: %v", v2, err)
	}

	// Create DRAFT v3
	v3Draft, err := store.UpdateAdminBillingRule(v2.ID, adminBillingRuleMutation{
		BillingType:         "per_image",
		BasePrice:           15,
		MinimumCharge:       1,
		ParameterMultiplier: validGPTImageTestParameterRules(),
		Status:              "DRAFT",
	})
	if err != nil {
		t.Fatalf("create draft v3: %v", err)
	}
	if v3Draft.Version != 3 || v3Draft.Status != "DRAFT" {
		t.Fatalf("draft v3 version/status = %d/%s, want 3/DRAFT", v3Draft.Version, v3Draft.Status)
	}

	// Publish v3 with confirmNegativeMargin=true
	v3Published, err := store.PublishBillingRuleVersion(v3Draft.ID, publishBillingRuleRequest{
		ConfirmNegativeMargin: true,
		ActorID:                "operator_release",
		ActorRole:              "SUPER_ADMIN",
	})
	if err != nil {
		t.Fatalf("publish v3: %v", err)
	}
	if v3Published.Status != "PUBLISHED" || v3Published.Version != 3 {
		t.Fatalf("v3 published status/version = %s/%d, want PUBLISHED/3", v3Published.Status, v3Published.Version)
	}

	// Check v2 is now ARCHIVED and effective_to is populated
	v2After, err := store.GetBillingRuleVersion(v2.ID)
	if err != nil {
		t.Fatalf("reload v2: %v", err)
	}
	if v2After.Status != "ARCHIVED" {
		t.Fatalf("v2 status = %s, want ARCHIVED", v2After.Status)
	}
	if v2After.EffectiveTo == "" {
		t.Fatal("v2 effectiveTo is empty, want timestamp")
	}

	// Verify exactly one PUBLISHED rule version exists for billing_rule_image_gpt
	allVersions, err := store.ListBillingRuleVersions()
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	publishedCount := 0
	for _, ver := range allVersions {
		if ver.RuleKey == "billing_rule_image_gpt" && ver.Status == "PUBLISHED" {
			publishedCount++
			if ver.ID != v3Published.ID {
				t.Fatalf("unexpected published version: %+v", ver)
			}
		}
	}
	if publishedCount != 1 {
		t.Fatalf("published version count = %d, want 1", publishedCount)
	}
}

// 9. 发布 basePrice=15 后 Quote：
//    - 1K low = 15
//    - 2K low = 23
//    - 4K low = 30
func TestGPTImageValidation_QuoteAfterPublishBasePrice15(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	published, err := store.GetBillingRuleVersion("brv_billing_rule_image_gpt_v1")
	if err != nil {
		t.Fatalf("get v1: %v", err)
	}

	draft, err := store.UpdateAdminBillingRule(published.ID, adminBillingRuleMutation{
		BillingType:         "per_image",
		BasePrice:           15,
		MinimumCharge:       1,
		ParameterMultiplier: validGPTImageTestParameterRules(),
		Status:              "DRAFT",
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	_, err = store.PublishBillingRuleVersion(draft.ID, publishBillingRuleRequest{
		ConfirmNegativeMargin: true,
		ActorID:                "admin",
	})
	if err != nil {
		t.Fatalf("publish basePrice=15 rule: %v", err)
	}

	data, err := store.AdminData()
	if err != nil {
		t.Fatalf("get admin data: %v", err)
	}

	testCases := []struct {
		name       string
		size       string
		quality    string
		n          float64
		wantPoints int
	}{
		// 1K low = 15 * 1.0 * 1.0 = 15
		{"1K low 1024x1024", "1024x1024", "low", 1, 15},
		{"1K low 1536x1024", "1536x1024", "low", 1, 15},
		{"1K low 720p 1280x720", "1280x720", "low", 1, 15},
		// 1K normal = 15 * 1.0 * 1.2 = 18
		{"1K normal 1024x1024", "1024x1024", "normal", 1, 18},
		// 2K low = 15 * 1.5 * 1.0 = 22.5 -> ceil = 23
		{"2K low 2048x2048", "2048x2048", "low", 1, 23},
		{"2K low custom 1792x1024", "1792x1024", "low", 1, 23},
		{"2K low custom 1600x1024", "1600x1024", "low", 1, 23},
		// 2K normal = 15 * 1.5 * 1.2 = 27
		{"2K normal 2048x2048", "2048x2048", "normal", 1, 27},
		// 4K low = 15 * 2.0 * 1.0 = 30 -> 30
		{"4K low 3840x2160", "3840x2160", "low", 1, 30},
		{"4K low 2880x2880", "2880x2880", "low", 1, 30},
		// 4K high = 15 * 2.0 * 1.5 = 45
		{"4K high 3840x2160", "3840x2160", "high", 1, 45},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := createGenerationTaskRequest{
				Type:   "TEXT_TO_IMAGE",
				Model:  "gpt-image-2",
				Params: map[string]any{"size": tc.size, "quality": tc.quality, "n": tc.n},
			}
			points := generationPointCostForRequest(req, data)
			if points != tc.wantPoints {
				t.Fatalf("%s (%s %s) quoted %d points, want %d", tc.name, tc.size, tc.quality, points, tc.wantPoints)
			}
		})
	}
}

// 10. 历史任务/账单不被重新计算
func TestGPTImageValidation_HistoricalTasksUnchanged(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))

	// 1. Initial published rule is basePrice=10 (v1)
	v1, err := store.GetBillingRuleVersion("brv_billing_rule_image_gpt_v1")
	if err != nil {
		t.Fatalf("get v1: %v", err)
	}

	// Create an initial completed task under v1
	historicalTask := generationTask{
		ID:                   "task_historical_001",
		UserID:               "user_test_001",
		Type:                 "TEXT_TO_IMAGE",
		Model:                "gpt-image-2",
		Status:               taskStatusSucceeded,
		PointCost:            10,
		QuotedPoints:         10,
		ReservedPoints:       10,
		CapturedPoints:       10,
		BillingRuleVersionID: v1.ID,
		Params: map[string]any{
			"size":    "1024x1024",
			"quality": "low",
			"n":       float64(1),
		},
		CreatedAt: "2026-09-01T10:00:00Z",
		UpdatedAt: "2026-09-01T10:01:00Z",
	}

	err = store.updateAdmin(func(data *adminPlatformData) error {
		data.GenerationTasks = append(data.GenerationTasks, historicalTask)
		data.BillingLifecycleEvents = append(data.BillingLifecycleEvents, billingLifecycleEvent{
			ID:             "ble_hist_001",
			TaskID:         historicalTask.ID,
			UserID:         historicalTask.UserID,
			EventType:      "CAPTURED",
			BillingStatus:  billingStatusCaptured,
			Points:         10,
			RuleVersionID:  v1.ID,
			IdempotencyKey: "idem_hist_001",
			CreatedAt:      "2026-09-01T10:01:00Z",
		})
		data.WalletLedger = append(data.WalletLedger, walletLedgerEntry{
			ID:             "wle_hist_001",
			AccountID:      "acc_test_001",
			UserID:         historicalTask.UserID,
			TaskID:         historicalTask.ID,
			EntryType:      "DEBIT",
			Points:         10,
			IdempotencyKey: "idem_hist_001",
			CreatedAt:      "2026-09-01T10:01:00Z",
		})
		return nil
	})
	if err != nil {
		t.Fatalf("seed historical task: %v", err)
	}

	// 2. Now create and publish v2 with BasePrice=15
	draft, err := store.UpdateAdminBillingRule(v1.ID, adminBillingRuleMutation{
		BillingType:         "per_image",
		BasePrice:           15,
		MinimumCharge:       1,
		ParameterMultiplier: validGPTImageTestParameterRules(),
		Status:              "DRAFT",
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	v2Published, err := store.PublishBillingRuleVersion(draft.ID, publishBillingRuleRequest{
		ConfirmNegativeMargin: true,
		ActorID:                "admin_migration",
	})
	if err != nil {
		t.Fatalf("publish v2: %v", err)
	}

	// 3. Verify historical task data has NOT changed
	data, err := store.AdminData()
	if err != nil {
		t.Fatalf("get admin data: %v", err)
	}

	var foundHistorical *generationTask
	for i := range data.GenerationTasks {
		if data.GenerationTasks[i].ID == historicalTask.ID {
			foundHistorical = &data.GenerationTasks[i]
			break
		}
	}
	if foundHistorical == nil {
		t.Fatal("historical task not found")
	}

	if foundHistorical.PointCost != 10 {
		t.Fatalf("historical task PointCost changed: got %d, want 10", foundHistorical.PointCost)
	}
	if foundHistorical.QuotedPoints != 10 {
		t.Fatalf("historical task QuotedPoints changed: got %v, want 10", foundHistorical.QuotedPoints)
	}
	if foundHistorical.CapturedPoints != 10 {
		t.Fatalf("historical task CapturedPoints changed: got %v, want 10", foundHistorical.CapturedPoints)
	}
	if foundHistorical.BillingRuleVersionID != v1.ID {
		t.Fatalf("historical task BillingRuleVersionID changed: got %s, want %s", foundHistorical.BillingRuleVersionID, v1.ID)
	}

	// Verify reconciliation does not report unexpected recalculation anomalies
	reconciliation, err := store.ListBillingReconciliation()
	if err != nil {
		t.Fatalf("list reconciliation: %v", err)
	}
	for _, item := range reconciliation {
		if item.TaskID == historicalTask.ID {
			if item.QuotedPoints != 10 || item.CapturedPoints != 10 {
				t.Fatalf("reconciliation changed historical task points: quote=%.0f capture=%.0f, want 10", item.QuotedPoints, item.CapturedPoints)
			}
			if item.RuleVersionID != v1.ID {
				t.Fatalf("reconciliation changed historical ruleVersionId: got %s, want %s", item.RuleVersionID, v1.ID)
			}
		}
	}

	// 4. Verify a newly quoted task uses the newly published v2 price (15)
	newTaskQuote := generationPointCostForRequest(createGenerationTaskRequest{
		Type:   "TEXT_TO_IMAGE",
		Model:  "gpt-image-2",
		Params: map[string]any{"size": "1024x1024", "quality": "low", "n": float64(1)},
	}, data)
	if newTaskQuote != 15 {
		t.Fatalf("new task quote = %d, want 15 (v2 price)", newTaskQuote)
	}
	_ = v2Published
}

// HTTP API POST /api/v1/admin/billing/rules/:id/publish 测试
func TestGPTImageBillingPublishHTTPApi(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	api := adminAPI{store: store}
	published, err := store.GetBillingRuleVersion("brv_billing_rule_image_gpt_v1")
	if err != nil {
		t.Fatalf("get published: %v", err)
	}

	// Create a draft that has negative margin
	draft, err := store.UpdateAdminBillingRule(published.ID, adminBillingRuleMutation{
		BillingType:         "per_image",
		BasePrice:           10,
		MinimumCharge:       1,
		ParameterMultiplier: validGPTImageTestParameterRules(),
		Status:              "DRAFT",
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	// Case A: POST without body -> rejected with 400
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/billing/rules/"+draft.ID+"/publish", nil)
		req.SetPathValue("id", draft.ID)
		rec := httptest.NewRecorder()
		api.publishBillingRuleV1(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
		}
		var errPayload map[string]any
		_ = json.NewDecoder(rec.Body).Decode(&errPayload)
		if errPayload["code"] != "NEGATIVE_MARGIN_CONFIRMATION_REQUIRED" {
			t.Fatalf("expected code NEGATIVE_MARGIN_CONFIRMATION_REQUIRED, got: %v", errPayload)
		}
	}

	// Case B: POST with {"confirmNegativeMargin": false} -> rejected with 400
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/billing/rules/"+draft.ID+"/publish", strings.NewReader(`{"confirmNegativeMargin": false}`))
		req.SetPathValue("id", draft.ID)
		rec := httptest.NewRecorder()
		api.publishBillingRuleV1(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// Case C: POST with {"force": true} -> does NOT override, rejected with 400
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/billing/rules/"+draft.ID+"/publish", strings.NewReader(`{"force": true}`))
		req.SetPathValue("id", draft.ID)
		rec := httptest.NewRecorder()
		api.publishBillingRuleV1(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for force=true, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// Case D: POST with malformed JSON -> 400 Bad Request
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/billing/rules/"+draft.ID+"/publish", strings.NewReader(`{malformed_json`))
		req.SetPathValue("id", draft.ID)
		rec := httptest.NewRecorder()
		api.publishBillingRuleV1(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for malformed JSON, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// Case E: POST with {"confirmNegativeMargin": true} -> 200 OK, published
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/billing/rules/"+draft.ID+"/publish", strings.NewReader(`{"confirmNegativeMargin": true}`))
		req.SetPathValue("id", draft.ID)
		rec := httptest.NewRecorder()
		api.publishBillingRuleV1(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK with confirmNegativeMargin=true, got %d: %s", rec.Code, rec.Body.String())
		}
		var respPayload map[string]any
		_ = json.NewDecoder(rec.Body).Decode(&respPayload)
		item, ok := respPayload["item"].(map[string]any)
		if !ok || item["status"] != "PUBLISHED" {
			t.Fatalf("expected item.status=PUBLISHED, got: %v", respPayload)
		}
	}
}

// TestGPTImageE2ECompleteLifecycleChain 验证 Phase 3 完整端到端链路：
// 管理后台读取当前 PUBLISHED rule
// → 创建新 DRAFT
// → 修改 basePrice / tier / quality (basePrice=15)
// → validate 返回 NEGATIVE_MARGIN
// → 未确认时 publish 被拒绝
// → confirmNegativeMargin=true 后发布成功
// → 原版本 ARCHIVED，新版本 PUBLISHED
// → generation quote 读取新版本，验证 6 大关键规格点数 (15/23/30/18/27/45)
// → 与管理端 UI 实时试算算法做 100% 对齐校验
// → 历史任务、历史账单不发生追溯修改
func TestGPTImageE2ECompleteLifecycleChain(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))

	// 1. 读取当前正式规则 (PUBLISHED)
	versions, err := store.ListBillingRuleVersions()
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	var currentPublished *billingRuleVersion
	for i := range versions {
		if versions[i].RuleKey == "billing_rule_image_gpt" && versions[i].Status == "PUBLISHED" {
			currentPublished = &versions[i]
			break
		}
	}
	if currentPublished == nil {
		t.Fatal("step 1 failed: current PUBLISHED rule not found")
	}
	initialVersion := currentPublished.Version
	t.Logf("Step 1: Current PUBLISHED rule is %s (version=%d, basePrice=%.0f)", currentPublished.ID, initialVersion, currentPublished.BasePrice)

	// 2. 模拟创建历史已扣费任务（在旧版本下）
	historicalTaskID := "task_e2e_historical_001"
	err = store.updateAdmin(func(data *adminPlatformData) error {
		data.GenerationTasks = append(data.GenerationTasks, generationTask{
			ID:                   historicalTaskID,
			UserID:               "user_e2e_001",
			Type:                 "TEXT_TO_IMAGE",
			Model:                "gpt-image-2",
			Status:               taskStatusSucceeded,
			PointCost:            10,
			QuotedPoints:         10,
			CapturedPoints:       10,
			BillingRuleVersionID: currentPublished.ID,
			CreatedAt:            "2026-09-01T10:00:00Z",
		})
		data.BillingLifecycleEvents = append(data.BillingLifecycleEvents, billingLifecycleEvent{
			ID:             "ble_e2e_001",
			TaskID:         historicalTaskID,
			UserID:         "user_e2e_001",
			EventType:      "CAPTURED",
			BillingStatus:  billingStatusCaptured,
			Points:         10,
			RuleVersionID:  currentPublished.ID,
			IdempotencyKey: "idem_e2e_001",
			CreatedAt:      "2026-09-01T10:00:00Z",
		})
		return nil
	})
	if err != nil {
		t.Fatalf("seed historical task: %v", err)
	}

	// 3. 基于当前版本创建 DRAFT，version 正确递增
	targetRules := validGPTImageTestParameterRules()
	draft, err := store.UpdateAdminBillingRule(currentPublished.ID, adminBillingRuleMutation{
		BillingType:         "per_image",
		BasePrice:           15,
		MinimumCharge:       1,
		ParameterMultiplier: targetRules,
		Status:              "DRAFT",
	})
	if err != nil {
		t.Fatalf("step 3 failed: create draft: %v", err)
	}
	expectedNextVersion := initialVersion + 1
	if draft.Version != expectedNextVersion || draft.Status != "DRAFT" {
		t.Fatalf("step 3 failed: draft version=%d, status=%s; want %d/DRAFT", draft.Version, draft.Status, expectedNextVersion)
	}
	t.Logf("Step 3: Created DRAFT version=%d (id=%s)", draft.Version, draft.ID)

	// 4. validate 能返回真实 NEGATIVE_MARGIN
	validation, err := store.ValidateBillingRuleVersion(draft.ID)
	if err != nil {
		t.Fatalf("step 4 failed: validate draft: %v", err)
	}
	hasNegativeMargin := false
	for _, issue := range validation.Issues {
		if issue.Code == "NEGATIVE_MARGIN" {
			hasNegativeMargin = true
		}
	}
	if !hasNegativeMargin {
		t.Fatalf("step 4 failed: expected NEGATIVE_MARGIN in validation issues: %+v", validation.Issues)
	}
	t.Logf("Step 4: Validation correctly caught NEGATIVE_MARGIN risk")

	// 5. 未确认时 publish 被拒绝
	_, err = store.PublishBillingRuleVersion(draft.ID, publishBillingRuleRequest{ConfirmNegativeMargin: false})
	if err == nil {
		t.Fatal("step 5 failed: unconfirmed publish should be rejected")
	}
	var pubErr *billingRulePublishError
	if !errors.As(err, &pubErr) || pubErr.Code != "NEGATIVE_MARGIN_CONFIRMATION_REQUIRED" {
		t.Fatalf("step 5 failed: expected NEGATIVE_MARGIN_CONFIRMATION_REQUIRED, got %v", err)
	}
	t.Logf("Step 5: Unconfirmed publish successfully rejected with NEGATIVE_MARGIN_CONFIRMATION_REQUIRED")

	// 6. confirmNegativeMargin=true 后发布成功
	publishedNew, err := store.PublishBillingRuleVersion(draft.ID, publishBillingRuleRequest{
		ConfirmNegativeMargin: true,
		ActorID:                "admin_e2e_tester",
		ActorRole:              "SUPER_ADMIN",
	})
	if err != nil {
		t.Fatalf("step 6 failed: publish with confirmNegativeMargin=true: %v", err)
	}
	if publishedNew.Status != "PUBLISHED" || publishedNew.Version != expectedNextVersion {
		t.Fatalf("step 6 failed: publishedNew status=%s, version=%d", publishedNew.Status, publishedNew.Version)
	}
	t.Logf("Step 6: Published new version=%d successfully", publishedNew.Version)

	// 7. 原版本变 ARCHIVED，新版本变 PUBLISHED
	reloadedOld, err := store.GetBillingRuleVersion(currentPublished.ID)
	if err != nil {
		t.Fatalf("get old rule: %v", err)
	}
	if reloadedOld.Status != "ARCHIVED" || reloadedOld.EffectiveTo == "" {
		t.Fatalf("step 7 failed: old rule status=%s, effectiveTo=%s, want ARCHIVED", reloadedOld.Status, reloadedOld.EffectiveTo)
	}
	t.Logf("Step 7: Old version %s is ARCHIVED with effectiveTo=%s", reloadedOld.ID, reloadedOld.EffectiveTo)

	// 8. 服务端 Quote 真实计算与 UI 试算预期做 100% 对齐校验
	adminData, err := store.AdminData()
	if err != nil {
		t.Fatalf("get admin data: %v", err)
	}

	quoteCheckSpecs := []struct {
		desc          string
		size          string
		quality       string
		wantServerPts int
		// UI 预览计算算法: Math.ceil(basePrice * sizeMult * qualityMult)
		uiSizeMult    float64
		uiQualityMult float64
	}{
		{"1K low", "1024x1024", "low", 15, 1.0, 1.0},
		{"2K low", "2048x2048", "low", 23, 1.5, 1.0},
		{"4K low", "3840x2160", "low", 30, 2.0, 1.0},
		{"1K normal", "1024x1024", "normal", 18, 1.0, 1.2},
		{"2K normal", "2048x2048", "normal", 27, 1.5, 1.2},
		{"4K high", "3840x2160", "high", 45, 2.0, 1.5},
	}

	for _, spec := range quoteCheckSpecs {
		req := createGenerationTaskRequest{
			Type:   "TEXT_TO_IMAGE",
			Model:  "gpt-image-2",
			Params: map[string]any{"size": spec.size, "quality": spec.quality, "n": float64(1)},
		}
		quote, err := generationQuoteForRequest(req, adminData)
		if err != nil {
			t.Fatalf("quote failed for %s: %v", spec.desc, err)
		}
		if quote.RequiredPoints != spec.wantServerPts {
			t.Fatalf("%s server quote = %d, want %d", spec.desc, quote.RequiredPoints, spec.wantServerPts)
		}

		// 验证 UI 试算算法与服务端真实 Quote 结果一致
		uiPreviewPts := int(math.Ceil(15.0 * spec.uiSizeMult * spec.uiQualityMult))
		if uiPreviewPts != quote.RequiredPoints {
			t.Fatalf("%s UI preview (%d) != server quote (%d)", spec.desc, uiPreviewPts, quote.RequiredPoints)
		}
		t.Logf("Step 8 Quote verify: %s -> server quote = %d, UI preview = %d (MATCH)", spec.desc, quote.RequiredPoints, uiPreviewPts)
	}

	// 9. 历史任务与账单不发生追溯修改
	historicalTaskFound := false
	for _, task := range adminData.GenerationTasks {
		if task.ID == historicalTaskID {
			historicalTaskFound = true
			if task.PointCost != 10 || task.QuotedPoints != 10 || task.CapturedPoints != 10 {
				t.Fatalf("historical task points altered: pointCost=%d quote=%.0f capture=%.0f, want 10", task.PointCost, task.QuotedPoints, task.CapturedPoints)
			}
			if task.BillingRuleVersionID != currentPublished.ID {
				t.Fatalf("historical task ruleVersionId altered: got %s, want %s", task.BillingRuleVersionID, currentPublished.ID)
			}
		}
	}
	if !historicalTaskFound {
		t.Fatal("historical task missing from admin data")
	}
	t.Logf("Step 9: Historical task %s unchanged with PointCost=10 and ruleVersion=%s", historicalTaskID, currentPublished.ID)
}
