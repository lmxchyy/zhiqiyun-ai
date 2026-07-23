package httpserver

import (
	"bytes"
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
)

func TestResolveModuleSchemaEnforcesPackageCapability(t *testing.T) {
	data := normalizeAICapabilityDefaults(seedAdminData())
	freeUser := adminUser{ID: "user_free", Role: "MEMBER", PlanID: "plan_free"}

	if _, err := resolveModuleSchema(data, freeUser, moduleImageGeneration, "mock-standard"); err != nil {
		t.Fatalf("free image capability should remain available: %v", err)
	}
	if _, err := resolveModuleSchema(data, freeUser, moduleVideoGeneration, "mock-video"); err == nil || !strings.Contains(err.Error(), "not included in package") {
		t.Fatalf("free video capability should be rejected, got: %v", err)
	}
}

func TestPPTOutlineEndpointEnforcesPackageCapability(t *testing.T) {
	sessions := newLocalAuthSessions()
	if err := sessions.Put(context.Background(), "free-ppt-token", "user_000010", time.Minute); err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(t.TempDir(), "platform.json")
	server := newWithStoreAndSessions(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()}, newJSONStore(dataPath), sessions)
	response := authedRequest(t, server.Handler, http.MethodPost, "/api/v1/ppt/outline/generate", bytes.NewBufferString(`{"prompt":"free plan ppt","slideCount":5,"textModel":"kimi-k2.6"}`), "free-ppt-token")
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "not included in package") {
		t.Fatalf("free PPT outline should be rejected by package capability: %d %s", response.Code, response.Body.String())
	}
}

func TestResolveModuleSchemaRejectsExpiredPersonalPackage(t *testing.T) {
	data := normalizeAICapabilityDefaults(seedAdminData())
	user := adminUser{ID: "expired_user", Role: "MEMBER", PlanID: "plan_month", SubscriptionExpiresAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)}
	if _, err := resolveModuleSchema(data, user, moduleImageGeneration, "mock-standard"); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired package should be rejected, got: %v", err)
	}
	user.SubscriptionExpiresAt = ""
	if _, err := resolveModuleSchema(data, user, moduleImageGeneration, "mock-standard"); err != nil {
		t.Fatalf("legacy user without expiry should remain compatible: %v", err)
	}
}

func TestResolveModuleSchemaSelectsFirstPackageAllowedModel(t *testing.T) {
	data := normalizeAICapabilityDefaults(seedAdminData())
	if err := applyAdminPlanCapabilities(&data, "plan_free", adminPlanCapabilitiesMutation{Modules: []adminPlanCapabilityModule{{
		ModuleCode: moduleImageGeneration, Enabled: true, AllowedModels: []string{"gpt-image-2"},
		Limits: map[string]any{"n": map[string]any{"max": float64(1)}, "quality": map[string]any{"allowed": []any{"standard"}}},
	}}}); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveModuleSchema(data, adminUser{ID: "free_user", Role: "MEMBER", PlanID: "plan_free"}, moduleImageGeneration, "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Model.ModelName != "gpt-image-2" {
		t.Fatalf("default package model = %s, want gpt-image-2", resolved.Model.ModelName)
	}
}

func TestApplyAdminPlanCapabilitiesRejectsEmptyAllowedParameters(t *testing.T) {
	data := normalizeAICapabilityDefaults(seedAdminData())
	err := applyAdminPlanCapabilities(&data, "plan_free", adminPlanCapabilitiesMutation{Modules: []adminPlanCapabilityModule{{
		ModuleCode: moduleVideoGeneration, Enabled: true, AllowedModels: []string{"mock-video"},
		Limits: map[string]any{"duration": map[string]any{"max": float64(6)}, "resolution": map[string]any{"allowed": []any{}}},
	}}})
	if err == nil || !strings.Contains(err.Error(), "requires at least one value") {
		t.Fatalf("empty resolution allowlist should be rejected, got: %v", err)
	}
}

func TestJSONGenerationConcurrencyUsesActiveTaskStatuses(t *testing.T) {
	data := platformData{
		Users:           []adminUser{{ID: "limited_user", PlanID: "limited_plan"}},
		Plans:           []adminPlan{{ID: "limited_plan", Concurrency: 1}},
		GenerationTasks: []generationTask{{ID: "active_task", UserID: "limited_user", Status: "PROCESSING"}},
	}
	if err := enforceJSONGenerationConcurrency(data, "limited_user"); err == nil || !strings.Contains(err.Error(), "concurrency limit reached") {
		t.Fatalf("active task should exhaust concurrency, got: %v", err)
	}
	data.GenerationTasks[0].Status = "FAILED"
	data.GenerationTasks[0].TaskStatus = taskStatusFailed
	if err := enforceJSONGenerationConcurrency(data, "limited_user"); err != nil {
		t.Fatalf("failed task should release concurrency: %v", err)
	}
}

func TestGenerationEndpointReturnsTooManyRequestsAtConcurrencyLimit(t *testing.T) {
	sessions := newLocalAuthSessions()
	if err := sessions.Put(context.Background(), "limited-token", "user_000010", time.Minute); err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(t.TempDir(), "platform.json")
	store := newJSONStore(dataPath)
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		for index := range data.Plans {
			if data.Plans[index].ID == "plan_free" {
				data.Plans[index].Concurrency = 1
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreatePendingGenerationTask(generation.CreateRequest{
		UserID: "user_000010", Type: "TEXT_TO_IMAGE", ModuleCode: moduleImageGeneration,
		Prompt: "occupy package concurrency", Model: "mock-standard",
		Params: map[string]any{"n": float64(1), "quality": "standard", "size": "1024x1024"},
	}); err != nil {
		t.Fatal(err)
	}
	server := newWithStoreAndSessions(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()}, store, sessions)
	response := authedRequest(t, server.Handler, http.MethodPost, "/api/v1/generation-tasks", bytes.NewBufferString(`{"type":"TEXT_TO_IMAGE","moduleCode":"image_generation","prompt":"should be throttled","model":"mock-standard","params":{"n":1,"quality":"standard","size":"1024x1024"}}`), "limited-token")
	if response.Code != http.StatusTooManyRequests || !strings.Contains(response.Body.String(), "concurrency limit reached") {
		t.Fatalf("concurrency limit should return 429: %d %s", response.Code, response.Body.String())
	}
}

func TestResolveModuleSchemaUsesTenantPolicyForEnterpriseContext(t *testing.T) {
	data := normalizeAICapabilityDefaults(seedAdminData())
	enterpriseUser := adminUser{ID: "enterprise_user", Role: "ENTERPRISE_MEMBER", PlanID: "plan_free", TenantID: "enterprise-a"}
	if _, err := resolveModuleSchema(data, enterpriseUser, moduleImageGeneration, "gpt-image-2"); err != nil {
		t.Fatalf("enterprise model should use tenant limits instead of personal package limits: %v", err)
	}
	for index := range data.AIModules {
		if canonicalModuleCode(data.AIModules[index].ModuleCode) == moduleImageGeneration {
			data.AIModules[index].OpenTenantIDs = []string{"enterprise-b"}
		}
	}
	if _, err := resolveModuleSchema(data, enterpriseUser, moduleImageGeneration, "gpt-image-2"); err == nil || !strings.Contains(err.Error(), "not open to tenant") {
		t.Fatalf("explicit tenant allowlist should be enforced, got: %v", err)
	}
}

func TestApplyAdminPlanCapabilitiesPersistsAccessModelsAndLimits(t *testing.T) {
	data := normalizeAICapabilityDefaults(seedAdminData())
	req := adminPlanCapabilitiesMutation{Modules: []adminPlanCapabilityModule{
		{
			ModuleCode:    moduleVideoGeneration,
			Enabled:       true,
			AllowedModels: []string{"mock-video"},
			Limits: map[string]any{
				"duration":   map[string]any{"max": float64(6)},
				"resolution": map[string]any{"allowed": []any{"480p", "720p"}},
			},
		},
	}}
	if err := applyAdminPlanCapabilities(&data, "plan_free", req); err != nil {
		t.Fatalf("apply capabilities: %v", err)
	}

	items, err := buildAdminPlanCapabilities(data, "plan_free")
	if err != nil {
		t.Fatalf("build capabilities: %v", err)
	}
	var video adminPlanCapabilityModule
	for _, item := range items {
		if item.ModuleCode == moduleVideoGeneration {
			video = item
			break
		}
	}
	if !video.Enabled || len(video.AllowedModels) != 1 || video.AllowedModels[0] != "mock-video" {
		t.Fatalf("unexpected video capability: %+v", video)
	}

	freeUser := adminUser{ID: "user_free", Role: "MEMBER", PlanID: "plan_free"}
	resolved, err := resolveModuleSchema(data, freeUser, moduleVideoGeneration, "mock-video")
	if err != nil {
		t.Fatalf("configured free video capability should resolve: %v", err)
	}
	if err := validateGenerationParams(generation.CreateRequest{
		ModuleCode: moduleVideoGeneration,
		Model:      "mock-video",
		Prompt:     "test video",
		Params: map[string]any{
			"duration": float64(8), "resolution": "720p",
		},
	}, resolved); err == nil || !strings.Contains(err.Error(), "duration") {
		t.Fatalf("package duration limit should be enforced, got: %v", err)
	}
	if _, err := resolveModuleSchema(data, freeUser, moduleVideoGeneration, "seedance-fast-2.0"); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("package model allowlist should be enforced, got: %v", err)
	}
}

func TestNormalizeGenerationQualityForPackageLimit(t *testing.T) {
	data := normalizeAICapabilityDefaults(seedAdminData())
	user := adminUser{ID: "user_free", Role: "MEMBER", PlanID: "plan_free"}
	resolved, err := resolveModuleSchema(data, user, moduleImageGeneration, "mock-standard")
	if err != nil {
		t.Fatal(err)
	}
	request := generation.CreateRequest{
		ModuleCode: moduleImageGeneration,
		Model:      "mock-standard",
		Prompt:     "test image",
		Params: map[string]any{
			"size": "1024x1024", "quality": "high", "n": float64(1),
		},
	}
	normalizeGenerationQualityForLimit(&request, resolved)
	if request.Params["quality"] != "standard" {
		t.Fatalf("quality was not normalized to package option: %#v", request.Params["quality"])
	}
	if err := validateGenerationParams(request, resolved); err != nil {
		t.Fatalf("normalized request should pass validation: %v", err)
	}

	request.Params["quality"] = "unsupported"
	normalizeGenerationQualityForLimit(&request, resolved)
	if request.Params["quality"] != "unsupported" {
		t.Fatalf("invalid schema value must not be silently normalized: %#v", request.Params["quality"])
	}
}

func TestLegacyPackageCapabilitiesExpandPaidVariantsOnlyBeforeFirstEdit(t *testing.T) {
	data := normalizeAICapabilityDefaults(seedAdminData())
	video := findAIModule(data.AIModules, moduleVideoGeneration)
	if !stringListContains(video.OpenPackageIDs, "plan_basic_year") {
		t.Fatalf("legacy paid variant was not migrated: %+v", video.OpenPackageIDs)
	}
	if stringListContains(video.OpenPackageIDs, "plan_free") {
		t.Fatalf("legacy migration unexpectedly opened video to free plan")
	}

	for index := range data.AIModules {
		if canonicalModuleCode(data.AIModules[index].ModuleCode) != moduleVideoGeneration {
			continue
		}
		data.AIModules[index].OpenPackageIDs = []string{"plan_pro"}
		data.AIModules[index].Config = map[string]any{"packageCapabilityVersion": float64(packageCapabilityConfigVersion)}
	}
	data = normalizeAICapabilityDefaults(data)
	video = findAIModule(data.AIModules, moduleVideoGeneration)
	if stringListContains(video.OpenPackageIDs, "plan_basic_year") {
		t.Fatalf("edited package list must not be expanded again: %+v", video.OpenPackageIDs)
	}
}
