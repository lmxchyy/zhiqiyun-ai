package httpserver

import (
	"context"
	"strings"
	"testing"

	"xianzhi-ai/backend-go/internal/config"
)

func seedDeepSeekPPTBillingRuleForTest(t *testing.T, store *jsonStore) {
	t.Helper()
	err := store.updateAdmin(func(data *adminPlatformData) error {
		*data = withDeepSeekPPTBillingRuleForTest(*data)
		return nil
	})
	if err != nil {
		t.Fatalf("seed DeepSeek PPT billing rule: %v", err)
	}
}

func withDeepSeekPPTBillingRuleForTest(data adminPlatformData) adminPlatformData {
	data.BillingRules = append(data.BillingRules, adminBillingRule{
		ID:           "test_deepseek_ppt_rule",
		ModuleCode:   modulePPTGeneration,
		ModelName:    "deepseek-v4-flash",
		BillingType:  "per_page",
		BasePrice:    1,
		CurrencyType: "credit",
		Status:       "ACTIVE",
	})
	return data
}

func TestPPTCapabilityFailsClosedUntilDeepSeekBillingRuleExists(t *testing.T) {
	data := normalizeAICapabilityDefaults(seedAdminData())
	user := adminUser{ID: "ppt-deepseek-user", Role: "MEMBER", PlanID: "plan_month"}

	_, err := resolveModuleSchema(data, user, modulePPTGeneration, "")
	if err == nil || !strings.Contains(err.Error(), "billing rule is not configured") {
		t.Fatalf("PPT capability without a DeepSeek billing rule should fail closed, got: %v", err)
	}

	data.BillingRules = append(data.BillingRules, adminBillingRule{
		ID:           "test_deepseek_ppt_rule",
		ModuleCode:   modulePPTGeneration,
		ModelName:    "deepseek-v4-flash",
		BillingType:  "per_page",
		BasePrice:    7,
		CurrencyType: "credit",
		Status:       "ACTIVE",
	})
	resolved, err := resolveModuleSchema(data, user, modulePPTGeneration, "")
	if err != nil {
		t.Fatalf("PPT capability with an explicit DeepSeek billing rule should resolve: %v", err)
	}
	if resolved.Model.ModelName != "deepseek-v4-flash" {
		t.Fatalf("resolved PPT model = %q, want deepseek-v4-flash", resolved.Model.ModelName)
	}
	if resolved.BillingRule.ID != "test_deepseek_ppt_rule" {
		t.Fatalf("resolved PPT billing rule = %q, want test_deepseek_ppt_rule", resolved.BillingRule.ID)
	}
}

func TestPPTTextModelOptionsExposeOnlyServerAllowedDeepSeek(t *testing.T) {
	server := api{cfg: config.Config{PPTTextModel: "kimi-k2.6"}}

	models := server.pptTextModelOptions()
	if len(models) != 1 {
		t.Fatalf("PPT text model options count = %d, want 1: %+v", len(models), models)
	}
	if models[0].Value != "deepseek-v4-flash" {
		t.Fatalf("PPT text model option = %q, want deepseek-v4-flash", models[0].Value)
	}
	if models[0].Disabled {
		t.Fatal("server-allowed DeepSeek PPT model must be enabled")
	}
}

func TestPPTDefaultModelHasNoFallbackBinding(t *testing.T) {
	data := normalizeAICapabilityDefaults(seedAdminData())

	module := findAIModule(data.AIModules, modulePPTGeneration)
	if len(module.BoundModels) != 1 || module.BoundModels[0] != "deepseek-v4-flash" {
		t.Fatalf("PPT bound models = %+v, want only deepseek-v4-flash", module.BoundModels)
	}
	model := findAIModel(data.AIModels, modulePPTGeneration, "deepseek-v4-flash")
	if model.ID == "" {
		t.Fatal("DeepSeek PPT model is not configured")
	}
	if model.FallbackModel != "" || model.AllowFallbackSwitch {
		t.Fatalf("DeepSeek PPT fallback binding must be disabled: %+v", model)
	}
	if legacy := findAIModel(data.AIModels, modulePPTGeneration, "kimi-k2.6"); legacy.ID != "" {
		t.Fatalf("legacy Kimi PPT model remains configured: %+v", legacy)
	}
	if local := findAIModel(data.AIModels, modulePPTGeneration, "ppt-text-model"); local.ID != "" {
		t.Fatalf("legacy local PPT fallback remains configured: %+v", local)
	}
}

func TestPPTBillingLookupDoesNotSynthesizeOnePointPerPageRule(t *testing.T) {
	req := createGenerationTaskRequest{
		Type:       "PPT_GENERATION",
		ModuleCode: modulePPTGeneration,
		Model:      "deepseek-v4-flash",
	}
	rule := billingRuleForRequest(req, adminPlatformData{})

	if rule.ID != "" {
		t.Fatalf("missing PPT billing rule must not synthesize a fallback: %+v", rule)
	}
	if points := generationPointCostForRequest(req, adminPlatformData{}); points != 0 {
		t.Fatalf("missing PPT billing rule point cost = %d, want 0", points)
	}
}

func TestPPTRejectsPersistedLegacyKimiConfiguration(t *testing.T) {
	data := normalizeAICapabilityDefaults(seedAdminData())
	data.AIModules[2].BoundModels = []string{"kimi-k2.6", "deepseek-v4-flash"}
	data.AIModels = append(data.AIModels, adminAIModel{
		ID: "legacy_kimi", ModelName: "kimi-k2.6", ModelType: "text", ModuleCode: modulePPTGeneration, Status: "ACTIVE",
	})
	data.AIParameterSchemas = append(data.AIParameterSchemas, adminAIParameterSchema{
		ID: "legacy_kimi_schema", ModuleCode: modulePPTGeneration, ModelName: "kimi-k2.6", SchemaJSON: adminAIParameterSchemaJSON{}, Status: "ACTIVE",
	})
	data.TenantModuleLimits[2].LimitJSON["models"] = map[string]any{"allowed": []any{"kimi-k2.6", "deepseek-v4-flash"}}
	data.BillingRules = append(data.BillingRules, adminBillingRule{
		ID: "legacy_kimi_rule", ModuleCode: modulePPTGeneration, ModelName: "kimi-k2.6", BillingType: "per_page", BasePrice: 1, Status: "ACTIVE",
	})

	_, err := resolveModuleSchema(data, adminUser{ID: "legacy-user", PlanID: "plan_month"}, modulePPTGeneration, "kimi-k2.6")
	if err == nil || !strings.Contains(err.Error(), "deepseek-v4-flash") {
		t.Fatalf("persisted legacy Kimi PPT configuration must be rejected, got: %v", err)
	}
}

func TestJSONPPTBillingStoreFailsClosedWithoutRule(t *testing.T) {
	store := newJSONStore(t.TempDir() + "/platform.json")
	req := createGenerationTaskRequest{
		ClientRequestID: "ppt-missing-rule", UserID: "user_000002", Type: "PPT_GENERATION",
		ModuleCode: modulePPTGeneration, Model: "deepseek-v4-flash", Params: map[string]any{"page_count": 2},
	}
	if _, err := store.CreateGenerationTask(req); err == nil || !strings.Contains(err.Error(), "billing rule") {
		t.Fatalf("CreateGenerationTask without PPT billing rule = %v", err)
	}
	if _, err := store.CreatePendingGenerationTask(req); err == nil || !strings.Contains(err.Error(), "billing rule") {
		t.Fatalf("CreatePendingGenerationTask without PPT billing rule = %v", err)
	}

	seedDeepSeekPPTBillingRuleForTest(t, store)
	account, err := store.PointAccount(req.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersonalPointService().Grant(context.Background(), PersonalPointGrantCommand{
		AccountID: account.ID, UserID: req.UserID, Source: PointSourceRecharge, Points: 10,
		ReferenceType: "TEST", ReferenceID: "ppt-missing-rule", IdempotencyKey: "ppt-missing-rule",
	}); err != nil {
		t.Fatal(err)
	}
	pending, err := store.CreatePendingGenerationTask(req)
	if err != nil {
		t.Fatalf("seed pending PPT task: %v", err)
	}
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		data.BillingRules = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteGenerationTask(pending.ID, req); err == nil || !strings.Contains(err.Error(), "billing rule") {
		t.Fatalf("CompleteGenerationTask without PPT billing rule = %v", err)
	}
}

func TestNonPPTBillingLookupKeepsExistingFallbackBehavior(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		moduleCode  string
		billingType string
	}{
		{name: "image", moduleCode: moduleImageGeneration, billingType: "per_image"},
		{name: "video", moduleCode: moduleVideoGeneration, billingType: "per_second"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			rule := billingRuleForRequest(createGenerationTaskRequest{
				ModuleCode: testCase.moduleCode,
				Model:      "unconfigured-test-model",
			}, adminPlatformData{})
			if rule.ID == "" || rule.BillingType != testCase.billingType || rule.BasePrice != 1 {
				t.Fatalf("%s fallback changed: %+v", testCase.name, rule)
			}
		})
	}
}
