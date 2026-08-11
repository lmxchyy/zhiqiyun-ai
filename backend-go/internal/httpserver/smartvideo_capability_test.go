package httpserver

import (
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
)

func TestNormalizeAICapabilityDefaultsMergesSmartVideoEditing(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	data := normalizeAICapabilityDefaults(adminPlatformData{
		AIModules: []adminAIModule{
			{
				ID: "ai_module_image_generation", ModuleCode: moduleImageGeneration, Name: "AI生图",
				Status: "ACTIVE", CreatedAt: now, UpdatedAt: now,
			},
		},
		AIModels:           []adminAIModel{},
		AIParameterSchemas: []adminAIParameterSchema{},
		TenantModuleLimits: []adminTenantModuleLimit{},
		BillingRules:       []adminBillingRule{},
	})
	module := findAIModule(data.AIModules, moduleSmartVideoEditing)
	if module.ID == "" {
		t.Fatal("smart_video_editing module was not merged")
	}
	model := findAIModel(data.AIModels, moduleSmartVideoEditing, modelSmartVideoStandard)
	if model.ID == "" {
		t.Fatal("smart-video-standard model was not merged")
	}
	speech := findAIModel(data.AIModels, moduleSmartVideoEditing, modelSmartVideoSpeech)
	if speech.ID == "" {
		t.Fatal("smart-video-speech internal model was not merged")
	}
	schema := findAIParameterSchema(data.AIParameterSchemas, moduleSmartVideoEditing, modelSmartVideoStandard)
	if schema.ID == "" {
		t.Fatal("smart video schema was not merged")
	}
	rule := selectBillingRule(data.BillingRules, moduleSmartVideoEditing, modelSmartVideoStandard)
	if rule.ID == "" {
		t.Fatal("smart video billing rule was not merged")
	}
	limit := effectiveTenantModuleLimit(data.TenantModuleLimits, adminUser{TenantID: "default"}, moduleSmartVideoEditing, modelSmartVideoStandard)
	if limit.ID == "" {
		t.Fatal("smart video tenant limit was not merged")
	}
	if canonicalModuleCode("ai-auto-montage") != moduleSmartVideoEditing {
		t.Fatalf("canonical module = %q", canonicalModuleCode("ai-auto-montage"))
	}
}

func TestPrepareGenerationRequestRejectsSmartVideoEditing(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	data := normalizeAICapabilityDefaults(adminPlatformData{
		AIModules:          defaultAIModules(now),
		AIModels:           defaultAIModels(now),
		AIParameterSchemas: defaultAIParameterSchemas(now),
		TenantModuleLimits: defaultTenantModuleLimits(now),
		BillingRules:       defaultBillingRules(now),
	})
	api := api{}
	_, err := api.prepareGenerationRequest(data, adminUser{ID: "u1", TenantID: "default"}, generation.CreateRequest{
		Type:       "SMART_VIDEO_EDITING",
		ModuleCode: moduleSmartVideoEditing,
		Prompt:     "mix clips",
		Model:      modelSmartVideoStandard,
		Params:     map[string]any{},
	})
	if err == nil || !strings.Contains(err.Error(), "/video-projects") {
		t.Fatalf("error = %v", err)
	}
}
