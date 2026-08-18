package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	imageprovider "xianzhi-ai/backend-go/internal/provider/image"
	videoprovider "xianzhi-ai/backend-go/internal/provider/video"
)

const (
	moduleImageGeneration   = "image_generation"
	moduleVideoGeneration   = "video_generation"
	modulePPTGeneration     = "ppt_generation"
	moduleSmartVideoEditing = "smart_video_editing"

	capabilitySmartVideoPlan  = "smart_video_plan"
	capabilitySpeechSynthesis = "speech_synthesis"

	modelSmartVideoStandard = "smart-video-standard"
	modelSmartVideoSpeech   = "smart-video-speech"
)

type adminAICapabilityConfig struct {
	AIModules          []adminAIModule          `json:"aiModules"`
	AIModels           []adminAIModel           `json:"aiModels"`
	AIParameterSchemas []adminAIParameterSchema `json:"aiParameterSchemas"`
	TenantModuleLimits []adminTenantModuleLimit `json:"tenantModuleLimits"`
	BillingRules       []adminBillingRule       `json:"billingRules"`
}

type resolvedModuleSchema struct {
	Module      adminAIModule
	Model       adminAIModel
	Schema      adminAIParameterSchema
	FinalSchema adminAIParameterSchemaJSON
	Limit       adminTenantModuleLimit
	BillingRule adminBillingRule
}

func normalizeAICapabilityDefaults(data adminPlatformData) adminPlatformData {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if len(data.AIModules) == 0 {
		data.AIModules = defaultAIModules(now)
	} else {
		data.AIModules = mergeDefaultAIModules(data.AIModules, defaultAIModules(now))
	}
	if len(data.AIModels) == 0 {
		data.AIModels = defaultAIModels(now)
	} else {
		data.AIModels = mergeDefaultAIModels(data.AIModels, defaultAIModels(now))
	}
	data = mergeDefaultVideoBoundModels(data)
	data = normalizeVideoModelCapabilityData(data)
	if len(data.AIParameterSchemas) == 0 {
		data.AIParameterSchemas = defaultAIParameterSchemas(now)
	} else {
		data.AIParameterSchemas = mergeDefaultAIParameterSchemaFields(data.AIParameterSchemas, defaultAIParameterSchemas(now))
	}
	if len(data.TenantModuleLimits) == 0 {
		data.TenantModuleLimits = defaultTenantModuleLimits(now)
	} else {
		data.TenantModuleLimits = ensureSmartVideoTenantLimit(data.TenantModuleLimits, defaultTenantModuleLimits(now))
	}
	data.TenantModuleLimits = alignGPTImageTenantQualityLimits(data.TenantModuleLimits)
	if len(data.BillingRules) == 0 {
		data.BillingRules = defaultBillingRules(now)
	} else {
		data.BillingRules = mergeDefaultBillingRules(data.BillingRules, defaultBillingRules(now))
	}
	data.AIModules = migrateLegacyPackageCapabilities(data.AIModules)
	return data
}

// Older capability records only listed the original monthly SKUs. Expand those
// legacy defaults once so enforcing package access does not unexpectedly block
// the newer single-month and yearly variants. Once a module has been edited in
// the plan editor, the version marker preserves the administrator's exact list.
func migrateLegacyPackageCapabilities(modules []adminAIModule) []adminAIModule {
	paidPlans := []string{
		"plan_month", "plan_basic_single", "plan_basic_year",
		"plan_pro", "plan_pro_single", "plan_pro_year",
		"plan_year", "plan_ultimate_single", "plan_ultimate_year",
		"plan_enterprise", "plan_ai_creator_996",
	}
	for index := range modules {
		if configVersion(modules[index].Config, "packageCapabilityVersion") >= packageCapabilityConfigVersion {
			continue
		}
		moduleCode := canonicalModuleCode(modules[index].ModuleCode)
		if moduleCode == moduleImageGeneration {
			modules[index].OpenPackageIDs = uniqueStringList(append(modules[index].OpenPackageIDs, "plan_free"))
		}
		if moduleCode == moduleImageGeneration || moduleCode == moduleVideoGeneration || moduleCode == modulePPTGeneration {
			modules[index].OpenPackageIDs = uniqueStringList(append(modules[index].OpenPackageIDs, paidPlans...))
		}
	}
	return modules
}

func configVersion(config map[string]any, key string) int {
	if config == nil {
		return 0
	}
	switch value := config[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, _ := value.Int64()
		return int(parsed)
	default:
		return 0
	}
}

func mergeDefaultBillingRules(current []adminBillingRule, defaults []adminBillingRule) []adminBillingRule {
	result := make([]adminBillingRule, len(current))
	copy(result, current)
	known := map[string]bool{}
	for _, rule := range result {
		rule = normalizeBillingRuleAliases(rule)
		key := canonicalModuleCode(rule.ModuleCode) + "\x00" + strings.ToLower(strings.TrimSpace(rule.ModelName))
		known[key] = true
	}
	for _, fallback := range defaults {
		fallback = normalizeBillingRuleAliases(fallback)
		key := canonicalModuleCode(fallback.ModuleCode) + "\x00" + strings.ToLower(strings.TrimSpace(fallback.ModelName))
		if known[key] {
			continue
		}
		result = append(result, fallback)
		known[key] = true
	}
	return result
}

func mergeDefaultAIModels(current []adminAIModel, defaults []adminAIModel) []adminAIModel {
	result := make([]adminAIModel, len(current))
	copy(result, current)
	known := map[string]bool{}
	for _, model := range result {
		known[strings.ToLower(strings.TrimSpace(model.ModelName))] = true
	}
	for _, fallback := range defaults {
		if canonicalModuleCode(fallback.ModuleCode) == moduleImageGeneration {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(fallback.ModelName))
		if key == "" || known[key] {
			continue
		}
		result = append(result, fallback)
		known[key] = true
	}
	return result
}

func mergeDefaultAIModules(current []adminAIModule, defaults []adminAIModule) []adminAIModule {
	result := make([]adminAIModule, len(current))
	copy(result, current)
	known := map[string]bool{}
	for _, module := range result {
		known[canonicalModuleCode(module.ModuleCode)] = true
	}
	for _, fallback := range defaults {
		key := canonicalModuleCode(fallback.ModuleCode)
		if key == "" || known[key] {
			continue
		}
		result = append(result, fallback)
		known[key] = true
	}
	return result
}

func mergeDefaultTenantModuleLimits(current []adminTenantModuleLimit, defaults []adminTenantModuleLimit) []adminTenantModuleLimit {
	result := make([]adminTenantModuleLimit, len(current))
	copy(result, current)
	known := map[string]bool{}
	for _, limit := range result {
		key := canonicalModuleCode(firstNonEmptyString(limit.ModuleCode, limit.ModuleCodeCamel)) + "\x00" +
			strings.ToLower(strings.TrimSpace(limit.TenantID)) + "\x00" +
			strings.ToLower(strings.TrimSpace(limit.PackageID))
		known[key] = true
	}
	for _, fallback := range defaults {
		key := canonicalModuleCode(firstNonEmptyString(fallback.ModuleCode, fallback.ModuleCodeCamel)) + "\x00" +
			strings.ToLower(strings.TrimSpace(fallback.TenantID)) + "\x00" +
			strings.ToLower(strings.TrimSpace(fallback.PackageID))
		if known[key] {
			continue
		}
		result = append(result, fallback)
		known[key] = true
	}
	return result
}

func ensureSmartVideoTenantLimit(current []adminTenantModuleLimit, defaults []adminTenantModuleLimit) []adminTenantModuleLimit {
	for _, limit := range current {
		if canonicalModuleCode(firstNonEmptyString(limit.ModuleCode, limit.ModuleCodeCamel)) == moduleSmartVideoEditing &&
			strings.EqualFold(strings.TrimSpace(limit.TenantID), "default") &&
			strings.TrimSpace(limit.PackageID) == "" {
			return current
		}
	}
	for _, fallback := range defaults {
		if canonicalModuleCode(firstNonEmptyString(fallback.ModuleCode, fallback.ModuleCodeCamel)) == moduleSmartVideoEditing {
			return append(append([]adminTenantModuleLimit{}, current...), fallback)
		}
	}
	return current
}

func alignGPTImageTenantQualityLimits(limits []adminTenantModuleLimit) []adminTenantModuleLimit {
	official := []any{"auto", "low", "medium", "high"}
	for index := range limits {
		if canonicalModuleCode(firstNonEmptyString(limits[index].ModuleCode, limits[index].ModuleCodeCamel)) != moduleImageGeneration {
			continue
		}
		if firstNonEmptyString(limits[index].PackageID, limits[index].PackageIDCamel) == "plan_free" {
			continue
		}
		limit, ok := mapValue(limits[index].LimitJSON)
		if !ok {
			continue
		}
		quality, ok := mapValue(limit["quality"])
		if !ok {
			continue
		}
		allowed, ok := anySlice(quality["allowed"])
		if !ok {
			continue
		}
		needsAlign := false
		for _, value := range allowed {
			switch strings.ToLower(strings.TrimSpace(fmt.Sprint(value))) {
			case "standard", "hd":
				needsAlign = true
			}
		}
		if !needsAlign {
			continue
		}
		quality["allowed"] = official
		limit["quality"] = quality
		limits[index].LimitJSON = limit
	}
	return limits
}

func mergeDefaultVideoBoundModels(data adminPlatformData) adminPlatformData {
	wanted := []string{"grok-imagine-video-1.5-preview", "grok-imagine-1.5-video"}
	for index := range data.AIModules {
		if canonicalModuleCode(data.AIModules[index].ModuleCode) != moduleVideoGeneration {
			continue
		}
		known := map[string]bool{}
		for _, modelName := range data.AIModules[index].BoundModels {
			known[strings.ToLower(strings.TrimSpace(modelName))] = true
		}
		for _, modelName := range wanted {
			key := strings.ToLower(strings.TrimSpace(modelName))
			if known[key] {
				continue
			}
			data.AIModules[index].BoundModels = append(data.AIModules[index].BoundModels, modelName)
			known[key] = true
		}
	}
	for index := range data.TenantModuleLimits {
		limit := &data.TenantModuleLimits[index]
		if canonicalModuleCode(firstNonEmptyString(limit.ModuleCode, limit.ModuleCodeCamel)) != moduleVideoGeneration {
			continue
		}
		if limit.LimitJSON == nil {
			limit.LimitJSON = firstNonNilMap(limit.LimitJSONCamel)
		}
		if limit.LimitJSON == nil {
			continue
		}
		models, ok := mapValue(limit.LimitJSON["models"])
		if !ok {
			continue
		}
		allowed := stringSliceFromAny(models["allowed"])
		known := map[string]bool{}
		for _, modelName := range allowed {
			known[strings.ToLower(strings.TrimSpace(modelName))] = true
		}
		changed := false
		for _, modelName := range wanted {
			key := strings.ToLower(strings.TrimSpace(modelName))
			if known[key] {
				continue
			}
			allowed = append(allowed, modelName)
			known[key] = true
			changed = true
		}
		if !changed {
			continue
		}
		next := map[string]any{}
		for key, value := range models {
			next[key] = value
		}
		allowedAny := make([]any, 0, len(allowed))
		for _, modelName := range allowed {
			allowedAny = append(allowedAny, modelName)
		}
		next["allowed"] = allowedAny
		limit.LimitJSON["models"] = next
	}
	data = mergeDefaultVideoChannelModels(data)
	return data
}

func mergeDefaultVideoChannelModels(data adminPlatformData) adminPlatformData {
	wantedByChannel := map[string][]string{
		"channel_newapi_gateway":      {"grok-imagine-video-1.5-preview", "grok-imagine-1.5-video", "doubao-seedance-2.0", "seedance-fast-2.0"},
		"channel_newapi_grok_imagine": {"grok-imagine-video-1.5-preview", "grok-imagine-1.5-video", "seedance-fast-2.0", "doubao-seedance-2.0"},
	}
	for index := range data.APIChannels {
		channel := &data.APIChannels[index]
		wanted, ok := wantedByChannel[strings.TrimSpace(channel.ID)]
		if !ok {
			baseURL := strings.ToLower(strings.TrimSpace(channel.BaseURL))
			if !strings.Contains(baseURL, "newapi") {
				continue
			}
			wanted = []string{"grok-imagine-video-1.5-preview", "grok-imagine-1.5-video", "seedance-fast-2.0", "doubao-seedance-2.0"}
		}
		known := map[string]bool{}
		for _, modelName := range channel.Models {
			known[strings.ToLower(strings.TrimSpace(modelName))] = true
		}
		for _, modelName := range wanted {
			key := strings.ToLower(strings.TrimSpace(modelName))
			if key == "" || known[key] {
				continue
			}
			channel.Models = append(channel.Models, modelName)
			known[key] = true
		}
	}
	return data
}

func mergeDefaultAIParameterSchemaFields(current []adminAIParameterSchema, defaults []adminAIParameterSchema) []adminAIParameterSchema {
	result := make([]adminAIParameterSchema, len(current))
	copy(result, current)
	for _, fallback := range defaults {
		matched := false
		for index := range result {
			if !strings.EqualFold(strings.TrimSpace(result[index].ModuleCode), strings.TrimSpace(fallback.ModuleCode)) ||
				!strings.EqualFold(strings.TrimSpace(result[index].ModelName), strings.TrimSpace(fallback.ModelName)) {
				continue
			}
			matched = true
			known := map[string]bool{}
			for _, field := range result[index].SchemaJSON.Fields {
				known[field.Key] = true
			}
			for _, field := range fallback.SchemaJSON.Fields {
				if known[field.Key] {
					continue
				}
				result[index].SchemaJSON.Fields = append(result[index].SchemaJSON.Fields, field)
				known[field.Key] = true
			}
			break
		}
		if !matched {
			result = append(result, fallback)
		}
	}
	return alignGPTImageParameterSchemas(result)
}

func alignGPTImageParameterSchemas(schemas []adminAIParameterSchema) []adminAIParameterSchema {
	official := gptImage2OfficialFields()
	for index := range schemas {
		if !isGPTImage2SchemaModel(schemas[index].ModelName) {
			continue
		}
		fields := make([]adminAIParameterField, 0, len(schemas[index].SchemaJSON.Fields)+len(official))
		replaced := map[string]bool{}
		for _, field := range schemas[index].SchemaJSON.Fields {
			key := strings.TrimSpace(field.Key)
			if key == "seed" || key == "negative_prompt" {
				continue
			}
			if officialField, ok := official[key]; ok {
				fields = append(fields, officialField)
				replaced[key] = true
				continue
			}
			fields = append(fields, field)
		}
		for _, key := range []string{"prompt", "size", "quality", "n"} {
			if replaced[key] {
				continue
			}
			if field, ok := official[key]; ok {
				fields = append(fields, field)
			}
		}
		schemas[index].SchemaJSON.Fields = fields
	}
	return schemas
}

func isGPTImage2SchemaModel(modelName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(modelName))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	return strings.Contains(normalized, "gpt-image-2")
}

func gptImage2OfficialFields() map[string]adminAIParameterField {
	return map[string]adminAIParameterField{
		"prompt": {Key: "prompt", Label: "图片提示词", Type: "textarea", Required: true, Placeholder: "描述你想生成的图片", UserEditable: true, Visible: true},
		"size": {
			Key: "size", Label: "图片尺寸", Type: "select", Required: false, Default: "auto",
			Options: gptImage2SizeOptions(), UserEditable: true, Visible: true,
		},
		"quality": {
			Key: "quality", Label: "图片质量", Type: "select", Required: false, Default: "low",
			Options: anyOptions("auto", "low", "medium", "high"), UserEditable: true, Visible: true,
		},
		"n": {
			Key: "n", Label: "生成数量", Type: "number", Required: false, Default: float64(1),
			Options: anyOptions(float64(1), float64(2), float64(3), float64(4)), Min: floatPtr(1), Max: floatPtr(4),
			UserEditable: true, Visible: true,
		},
	}
}

func gptImage2SizeOptions() []any {
	// Official GPT Image sizes shown in production UI.
	// Provider also accepts any other legal WxH; these are the listed SKUs.
	return anyOptions(
		"auto",
		"1024x1024",
		"1536x1024",
		"1024x1536",
		"1280x720",
		"720x1280",
		"2048x1152",
		"2048x2048",
		"3840x2160",
		"2160x3840",
	)
}

func gptImage2DeferredProductionSizes() []string {
	return []string{"1280x720", "720x1280", "2048x1152", "2048x2048", "3840x2160", "2160x3840"}
}

const (
	// gptImage2Phase1BasePrice is 10 credits per image at quality=low.
	gptImage2Phase1BasePrice = 10
	// gptImage2Phase1MinimumCharge matches one low-quality 1K image.
	gptImage2Phase1MinimumCharge = 10
)

// gptImage2Phase1BillingParameterRules is the unpublished Phase-1 GPT Image
// customer price. n is billed only via per_image quantity (no n multiplier).
// quality=auto and size=auto are temporarily billed as 1K medium (55 credits).
// Size keys are billing tiers, not provider WxH. Do not publish from this helper.
func gptImage2Phase1BillingParameterRules() map[string]any {
	return map[string]any{
		"quality": map[string]any{
			"low":    float64(1),
			"medium": float64(5.5),
			"high":   float64(22),
			"auto":   float64(5.5),
		},
		"size": map[string]any{
			"auto":      float64(1),
			"tier_720p": float64(1),
			"tier_1k":   float64(1),
			"tier_2k":   float64(1.5),
			"tier_4k":   float64(2),
		},
	}
}

func defaultAIModules(now string) []adminAIModule {
	return []adminAIModule{
		{
			ID: "ai_module_image_generation", ModuleCode: moduleImageGeneration, Name: "AI生图",
			Description: "统一管理文生图、图生图、图片编辑的模型、参数和调用策略。",
			Status:      "ACTIVE", OpenPackageIDs: []string{"plan_free", "plan_month", "plan_basic_single", "plan_pro", "plan_year"},
			BoundModels: []string{"mock-standard", "gpt-image-2", "HY-Image-3.0-Plus-4090-Tob-v1.0", "HY-Image-v3.0-I2I-ToB-v1.0.1"}, DefaultSchemaID: "schema_image_generation_default",
			AllowAgents: true, AllowEndUsers: true, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "ai_module_video_generation", ModuleCode: moduleVideoGeneration, Name: "视频生成",
			Description: "统一管理文生视频、图生视频、首尾帧视频的模型、参数和调用策略。",
			Status:      "ACTIVE", OpenPackageIDs: []string{"plan_month", "plan_pro", "plan_year"},
			BoundModels: []string{"mock-video", "grok-imagine-video-1.5-preview", "grok-imagine-1.5-video", "seedance-fast-2.0", "doubao-seedance-2.0"}, DefaultSchemaID: "schema_video_generation_default",
			AllowAgents: true, AllowEndUsers: true, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "ai_module_ppt_generation", ModuleCode: modulePPTGeneration, Name: "PPT文档生成",
			Description: "统一管理 PPT 提纲、内容生成、配图和导出参数。",
			Status:      "ACTIVE", OpenPackageIDs: []string{"plan_month", "plan_pro", "plan_year"},
			BoundModels: []string{"kimi-k2.6", "ppt-text-model"}, DefaultSchemaID: "schema_ppt_generation_default",
			AllowAgents: true, AllowEndUsers: true, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "ai_module_smart_video_editing", ModuleCode: moduleSmartVideoEditing, Name: "AI自动混剪",
			Description: "素材理解、镜头规划、配音字幕与时间线导出。用户选择成片规格，内部能力由服务端解析。",
			Status:      "ACTIVE", OpenPackageIDs: []string{"plan_month", "plan_pro", "plan_year"},
			BoundModels: []string{modelSmartVideoStandard}, DefaultSchemaID: "schema_smart_video_editing_default",
			AllowAgents: false, AllowEndUsers: true, CreatedAt: now, UpdatedAt: now,
			Config: map[string]any{
				"internalCapabilities": []any{capabilitySmartVideoPlan, capabilitySpeechSynthesis},
				"publicWorkflowModel":  modelSmartVideoStandard,
			},
		},
	}
}

func defaultAIModels(now string) []adminAIModel {
	return []adminAIModel{
		{ID: "ai_model_mock_standard", ModelName: "mock-standard", ModelType: "image", Provider: "Local", CapabilityCode: []string{"text_to_image", "image_to_image"}, ModuleCode: moduleImageGeneration, Status: "ACTIVE", SortWeight: 10, CreatedAt: now, UpdatedAt: now},
		{ID: "ai_model_gpt_image_2", ModelName: "gpt-image-2", ModelType: "image", Provider: "NewAPI", CapabilityCode: []string{"text_to_image", "image_to_image", "image_edit"}, ModuleCode: moduleImageGeneration, Status: "ACTIVE", FallbackModel: "mock-standard", SortWeight: 20, AllowFallbackSwitch: true, CreatedAt: now, UpdatedAt: now},
		{ID: "ai_model_cloudbase_hy_image", ModelName: "HY-Image-3.0-Plus-4090-Tob-v1.0", ModelType: "image", Provider: "CloudBase", CapabilityCode: []string{"text_to_image"}, ModuleCode: moduleImageGeneration, Status: "ACTIVE", SortWeight: 30, CreatedAt: now, UpdatedAt: now},
		{ID: "ai_model_cloudbase_hy_image_i2i", ModelName: "HY-Image-v3.0-I2I-ToB-v1.0.1", ModelType: "image", Provider: "CloudBase", CapabilityCode: []string{"image_to_image"}, ModuleCode: moduleImageGeneration, Status: "ACTIVE", SortWeight: 40, CreatedAt: now, UpdatedAt: now},
		{ID: "ai_model_mock_video", ModelName: "mock-video", ModelType: "video", Provider: "Local", CapabilityCode: []string{"text_to_video", "image_to_video"}, ModuleCode: moduleVideoGeneration, Status: "ACTIVE", SortWeight: 10, CreatedAt: now, UpdatedAt: now},
		{ID: "ai_model_grok_imagine_video_15_preview", ModelName: "grok-imagine-video-1.5-preview", ModelType: "video", Provider: "NewAPI", CapabilityCode: []string{"image_to_video"}, ModuleCode: moduleVideoGeneration, Status: "ACTIVE", SortWeight: 14, VideoCapabilities: grokImagine15VideoPreviewCapabilitiesPtr(), CreatedAt: now, UpdatedAt: now},
		{ID: "ai_model_grok_imagine_15_video", ModelName: "grok-imagine-1.5-video", ModelType: "video", Provider: "NewAPI", CapabilityCode: []string{"text_to_video", "image_to_video"}, ModuleCode: moduleVideoGeneration, Status: "ACTIVE", SortWeight: 15, VideoCapabilities: grokImagine15VideoCapabilitiesPtr(), CreatedAt: now, UpdatedAt: now},
		{ID: "ai_model_seedance_fast_20", ModelName: "seedance-fast-2.0", ModelType: "video", Provider: "NewAPI", CapabilityCode: []string{"text_to_video", "image_to_video"}, ModuleCode: moduleVideoGeneration, Status: "ACTIVE", FallbackModel: "mock-video", SortWeight: 20, AllowFallbackSwitch: true, CreatedAt: now, UpdatedAt: now},
		{ID: "ai_model_doubao_seedance_20", ModelName: "doubao-seedance-2.0", ModelType: "video", Provider: "NewAPI", ChannelID: "channel_newapi_gateway", CapabilityCode: []string{"text_to_video", "image_to_video"}, ModuleCode: moduleVideoGeneration, Status: "ACTIVE", FallbackModel: "mock-video", SortWeight: 30, AllowFallbackSwitch: true, CreatedAt: now, UpdatedAt: now},
		{ID: "ai_model_kimi_k26", ModelName: "kimi-k2.6", ModelType: "text", Provider: "NewAPI", CapabilityCode: []string{"ppt_outline", "ppt_content", "ppt_export"}, ModuleCode: modulePPTGeneration, Status: "ACTIVE", FallbackModel: "ppt-text-model", SortWeight: 10, AllowFallbackSwitch: true, CreatedAt: now, UpdatedAt: now},
		{ID: "ai_model_ppt_text", ModelName: "ppt-text-model", ModelType: "text", Provider: "Local", CapabilityCode: []string{"ppt_outline", "ppt_content"}, ModuleCode: modulePPTGeneration, Status: "ACTIVE", SortWeight: 20, CreatedAt: now, UpdatedAt: now},
		{ID: "ai_model_smart_video_standard", ModelName: modelSmartVideoStandard, ModelType: "workflow", Provider: "Local", CapabilityCode: []string{capabilitySmartVideoPlan, capabilitySpeechSynthesis}, ModuleCode: moduleSmartVideoEditing, Status: "ACTIVE", SortWeight: 10, AllowedCapabilities: []string{capabilitySmartVideoPlan, capabilitySpeechSynthesis}, CreatedAt: now, UpdatedAt: now},
		{ID: "ai_model_smart_video_speech", ModelName: modelSmartVideoSpeech, ModelType: "speech", Provider: "NewAPI", CapabilityCode: []string{capabilitySpeechSynthesis}, ModuleCode: moduleSmartVideoEditing, Status: "ACTIVE", SortWeight: 90, AllowedCapabilities: []string{capabilitySpeechSynthesis}, MiniProgramEnabled: false, CreatedAt: now, UpdatedAt: now},
	}
}

func defaultAIParameterSchemas(now string) []adminAIParameterSchema {
	return []adminAIParameterSchema{
		{
			ID: "schema_image_generation_default", ModuleCode: moduleImageGeneration, ModelName: "mock-standard",
			SchemaJSON: adminAIParameterSchemaJSON{Fields: []adminAIParameterField{
				{Key: "prompt", Label: "图片提示词", Type: "textarea", Required: true, Placeholder: "描述你想生成的图片", UserEditable: true, Visible: true},
				{Key: "size", Label: "图片尺寸", Type: "select", Required: true, Default: "1920x1080", Options: anyOptions("1920x1080"), UserEditable: true, Visible: true},
				{Key: "reference_image", Label: "参考图", Type: "image_upload", UserEditable: true, Visible: true},
				{Key: "seed", Label: "种子值", Type: "number", UserEditable: true, Visible: true},
				{Key: "negative_prompt", Label: "负面提示词", Type: "textarea", UserEditable: true, Visible: true},
			}},
			Status: "ACTIVE", CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "schema_video_generation_default", ModuleCode: moduleVideoGeneration, ModelName: "mock-video",
			SchemaJSON: adminAIParameterSchemaJSON{Fields: []adminAIParameterField{
				{Key: "prompt", Label: "视频提示词", Type: "textarea", Required: true, Placeholder: "描述视频画面、运动和风格", UserEditable: true, Visible: true},
				{Key: "duration", Label: "视频时长", Type: "select", Required: true, Default: float64(5), Options: anyOptions(float64(4), float64(5), float64(6), float64(8), float64(10), float64(12), float64(15)), Unit: "秒", UserEditable: true, Visible: true},
				{Key: "resolution", Label: "分辨率", Type: "select", Required: true, Default: "720p", Options: anyOptions("480p", "720p", "1080p", "4k"), UserEditable: true, Visible: true},
				{Key: "aspect_ratio", Label: "画面比例", Type: "select", Required: true, Default: "16:9", Options: anyOptions("16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "adaptive"), UserEditable: true, Visible: true},
				{Key: "fps", Label: "帧率", Type: "select", Default: float64(24), Options: anyOptions(float64(24), float64(30)), UserEditable: true, Visible: true},
				{Key: "motion_strength", Label: "运动强度", Type: "select", Options: anyOptions("low", "medium", "high"), UserEditable: true, Visible: true},
				{Key: "camera_movement", Label: "镜头运动", Type: "select", Options: anyOptions("static", "pan", "push", "pull"), UserEditable: true, Visible: true},
				{Key: "generate_audio", Label: "生成音频", Type: "boolean", Default: true, UserEditable: true, Visible: true},
				{Key: "first_frame", Label: "首帧图", Type: "image_upload", UserEditable: true, Visible: true},
				{Key: "last_frame", Label: "尾帧图", Type: "image_upload", UserEditable: true, Visible: true},
			}},
			Status: "ACTIVE", CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "schema_ppt_generation_default", ModuleCode: modulePPTGeneration, ModelName: "kimi-k2.6",
			SchemaJSON: adminAIParameterSchemaJSON{Fields: []adminAIParameterField{
				{Key: "topic", Label: "PPT主题", Type: "textarea", Required: true, Placeholder: "输入演示文稿主题和要求", UserEditable: true, Visible: true},
				{Key: "page_count", Label: "页数", Type: "number", Required: true, Default: float64(5), Min: floatPtr(1), Max: floatPtr(20), UserEditable: true, Visible: true},
				{Key: "template_id", Label: "模板ID", Type: "template_select", UserEditable: true, Visible: true},
				{Key: "theme_style", Label: "主题风格", Type: "select", Default: "business", Options: anyOptions("business", "minimal", "technology", "education"), UserEditable: true, Visible: true},
				{Key: "language", Label: "语言", Type: "select", Default: "zh-CN", Options: anyOptions("zh-CN", "en-US"), UserEditable: true, Visible: true},
				{Key: "export_format", Label: "导出格式", Type: "select", Default: "pptx", Options: anyOptions("pptx", "pdf"), UserEditable: true, Visible: true},
				{Key: "with_images", Label: "生成配图", Type: "switch", Default: true, UserEditable: true, Visible: true},
				{Key: "web_search_enabled", Label: "联网搜索", Type: "switch", Default: false, UserEditable: true, Visible: true},
				{Key: "uploaded_file", Label: "上传参考文档", Type: "file_upload", UserEditable: true, Visible: true},
			}},
			Status: "ACTIVE", CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "schema_smart_video_editing_default", ModuleCode: moduleSmartVideoEditing, ModelName: modelSmartVideoStandard,
			SchemaJSON: adminAIParameterSchemaJSON{Fields: []adminAIParameterField{
				{Key: "requirement", Label: "成片需求", Type: "textarea", Required: true, Placeholder: "用一句话描述想要的成片", UserEditable: true, Visible: true},
				{Key: "aspect_ratio", Label: "画幅", Type: "select", Required: true, Default: "9:16", Options: anyOptions("9:16", "16:9"), UserEditable: true, Visible: true},
				{Key: "resolution", Label: "清晰度", Type: "select", Required: true, Default: "720p", Options: anyOptions("720p", "1080p"), UserEditable: true, Visible: true},
				{Key: "duration_ms", Label: "成片时长", Type: "number", Required: true, Default: float64(30000), Min: floatPtr(15000), Max: floatPtr(60000), Unit: "毫秒", UserEditable: true, Visible: true},
				{Key: "voice_enabled", Label: "AI配音", Type: "switch", Default: true, UserEditable: true, Visible: true},
				{Key: "subtitle_enabled", Label: "配音字幕", Type: "switch", Default: true, UserEditable: true, Visible: true},
			}},
			Status: "ACTIVE", CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "schema_image_generation_gpt_image_2", ModuleCode: moduleImageGeneration, ModelName: "gpt-image-2",
			SchemaJSON: adminAIParameterSchemaJSON{Fields: []adminAIParameterField{
				gptImage2OfficialFields()["prompt"],
				gptImage2OfficialFields()["size"],
				gptImage2OfficialFields()["quality"],
				gptImage2OfficialFields()["n"],
			}},
			Status: "ACTIVE", CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "schema_image_generation_cloudbase_hy_image", ModuleCode: moduleImageGeneration, ModelName: "HY-Image-3.0-Plus-4090-Tob-v1.0",
			SchemaJSON: adminAIParameterSchemaJSON{Fields: []adminAIParameterField{
				{Key: "prompt", Label: "图片提示词", Type: "textarea", Required: true, Placeholder: "描述你想生成的图片", UserEditable: true, Visible: true},
				{Key: "size", Label: "图片尺寸", Type: "select", Required: true, Default: "1024x1024", Options: anyOptions("1024x1024", "1280x1280", "1280x720", "720x1280"), UserEditable: true, Visible: true},
			}},
			Status: "ACTIVE", CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "schema_image_generation_cloudbase_hy_image_i2i", ModuleCode: moduleImageGeneration, ModelName: "HY-Image-v3.0-I2I-ToB-v1.0.1",
			SchemaJSON: adminAIParameterSchemaJSON{Fields: []adminAIParameterField{
				{Key: "prompt", Label: "图片提示词", Type: "textarea", Required: true, Placeholder: "描述你想生成的图片", UserEditable: true, Visible: true},
				{Key: "size", Label: "图片尺寸", Type: "select", Required: true, Default: "1024x1024", Options: anyOptions("1024x1024", "1280x1280", "1280x720", "720x1280"), UserEditable: true, Visible: true},
				{Key: "reference_image", Label: "参考图", Type: "image_upload", Required: true, UserEditable: true, Visible: true},
			}},
			Status: "ACTIVE", CreatedAt: now, UpdatedAt: now,
		},
	}
}

func defaultTenantModuleLimits(now string) []adminTenantModuleLimit {
	return []adminTenantModuleLimit{
		{ID: "limit_default_image", TenantID: "default", ModuleCode: moduleImageGeneration, LimitJSON: map[string]any{"models": map[string]any{"allowed": []any{"mock-standard", "gpt-image-2", "HY-Image-3.0-Plus-4090-Tob-v1.0", "HY-Image-v3.0-I2I-ToB-v1.0.1"}}, "n": map[string]any{"max": float64(4)}, "quality": map[string]any{"allowed": []any{"auto", "low", "medium", "high"}}}, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{ID: "limit_default_video", TenantID: "default", ModuleCode: moduleVideoGeneration, LimitJSON: map[string]any{"models": map[string]any{"allowed": []any{"mock-video", "grok-imagine-video-1.5-preview", "grok-imagine-1.5-video", "seedance-fast-2.0", "doubao-seedance-2.0"}}, "resolution": map[string]any{"allowed": []any{"480p", "720p", "1080p", "4k"}}, "duration": map[string]any{"max": float64(30)}}, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{ID: "limit_default_ppt", TenantID: "default", ModuleCode: modulePPTGeneration, LimitJSON: map[string]any{"models": map[string]any{"allowed": []any{"kimi-k2.6", "ppt-text-model"}}, "page_count": map[string]any{"max": float64(20)}, "uploaded_file": map[string]any{"enabled": true}, "with_images": map[string]any{"enabled": true}}, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{ID: "limit_default_smart_video", TenantID: "default", ModuleCode: moduleSmartVideoEditing, LimitJSON: map[string]any{"models": map[string]any{"allowed": []any{modelSmartVideoStandard}}, "resolution": map[string]any{"allowed": []any{"720p", "1080p"}}, "duration_ms": map[string]any{"min": float64(15000), "max": float64(60000)}, "plan_per_day": map[string]any{"max": float64(20)}}, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{ID: "limit_plan_free_image", TenantID: "default", PackageID: "plan_free", ModuleCode: moduleImageGeneration, LimitJSON: map[string]any{"models": map[string]any{"allowed": []any{"mock-standard"}}, "n": map[string]any{"max": float64(1)}, "quality": map[string]any{"allowed": []any{"standard"}}}, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
	}
}

func defaultBillingRules(now string) []adminBillingRule {
	return []adminBillingRule{
		{ID: "billing_rule_image_mock", ModuleCode: moduleImageGeneration, ModelName: "mock-standard", BillingType: "per_image", BasePrice: 1, CostPrice: 0, CurrencyType: "credit", ParameterMultiplier: map[string]any{"quality": map[string]any{"standard": float64(1), "high": float64(1.5)}}, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		// CODE_DEFAULT / local JSON auto-publish seed. Phase-1 customer SKU
		// (low=10, medium=55, high=220, auto=55) lives in unpublished DRAFT
		// created by TestGPTImageBillingRulePhase26Draft. Do not treat this
		// seed as the published production price.
		{ID: "billing_rule_image_gpt", ModuleCode: moduleImageGeneration, ModelName: "gpt-image-2", BillingType: "per_image", BasePrice: 10, CostPrice: 6, CurrencyType: "credit", ParameterMultiplier: map[string]any{"quality": map[string]any{"auto": float64(1), "low": float64(1), "medium": float64(1.2), "high": float64(1.5)}, "size": map[string]any{"auto": float64(1), "1024x1024": float64(1), "1024x1536": float64(1.2), "1536x1024": float64(1.2), "1280x720": float64(1), "720x1280": float64(1), "2048x2048": float64(1.5), "2048x1152": float64(1.5), "3840x2160": float64(2), "2160x3840": float64(2)}}, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{ID: "billing_rule_video_mock", ModuleCode: moduleVideoGeneration, ModelName: "mock-video", BillingType: "per_second", BasePrice: 1, CostPrice: 0, CurrencyType: "credit", ParameterMultiplier: map[string]any{"resolution": map[string]any{"480p": float64(1), "720p": float64(1.2), "1080p": float64(2)}}, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{ID: "billing_rule_video_grok_image", ModuleCode: moduleVideoGeneration, ModelName: "grok-video-image", BillingType: "per_second", BasePrice: 1, CostPrice: 0, CurrencyType: "credit", ParameterMultiplier: map[string]any{"resolution": map[string]any{"480p": float64(1), "720p": float64(1.2), "1080p": float64(2)}}, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{ID: "billing_rule_video_grok_imagine_15_preview", ModuleCode: moduleVideoGeneration, ModelName: "grok-imagine-video-1.5-preview", BillingType: "per_request", BasePrice: 100, MinimumCharge: 100, CostPrice: 80, CurrencyType: "credit", ParameterMultiplier: map[string]any{}, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{ID: "billing_rule_video_grok_imagine_15", ModuleCode: moduleVideoGeneration, ModelName: "grok-imagine-1.5-video", BillingType: "per_second", BasePrice: 15, MinimumCharge: 15, CostPrice: 13, CurrencyType: "credit", ParameterMultiplier: map[string]any{}, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{ID: "billing_rule_video_seedance", ModuleCode: moduleVideoGeneration, ModelName: "seedance-fast-2.0", BillingType: "per_second", BasePrice: 80, CostPrice: 8, CurrencyType: "credit", ParameterMultiplier: map[string]any{"resolution": map[string]any{"480p": float64(1), "720p": float64(1.5), "1080p": float64(2)}}, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{ID: "billing_rule_video_doubao_seedance", ModuleCode: moduleVideoGeneration, ModelName: "doubao-seedance-2.0", BillingType: "per_second", BasePrice: 80, CostPrice: 8, CurrencyType: "credit", ParameterMultiplier: map[string]any{"resolution": map[string]any{"480p": float64(1), "720p": float64(1.5), "1080p": float64(2), "4k": float64(4)}}, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{ID: "billing_rule_ppt_kimi", ModuleCode: modulePPTGeneration, ModelName: "kimi-k2.6", BillingType: "per_page", BasePrice: 1, CostPrice: 0.4, CurrencyType: "credit", ParameterMultiplier: map[string]any{"with_images": map[string]any{"true": float64(1), "false": float64(1)}, "uploaded_file": map[string]any{"true": float64(1), "false": float64(1)}}, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{ID: "billing_rule_smart_video_standard", ModuleCode: moduleSmartVideoEditing, ModelName: modelSmartVideoStandard, BillingType: "per_second", BasePrice: 2, CostPrice: 0.5, CurrencyType: "credit", ParameterMultiplier: map[string]any{"resolution": map[string]any{"720p": float64(1), "1080p": float64(1.5)}, "voice_enabled": map[string]any{"true": float64(1.2), "false": float64(1)}}, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
	}
}

func (a api) moduleSchema(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	moduleCode := canonicalModuleCode(r.URL.Query().Get("module_code"))
	if moduleCode == "" {
		moduleCode = canonicalModuleCode(r.URL.Query().Get("moduleCode"))
	}
	modelName := strings.TrimSpace(firstNonEmptyString(r.URL.Query().Get("model_name"), r.URL.Query().Get("modelName")))
	if authorizer, ok := a.store.(modelCallAuthorizer); ok {
		authorization, authErr := authorizer.AuthorizeModelCall(user.ID, moduleCode)
		if authErr != nil {
			writeError(w, http.StatusForbidden, authErr)
			return
		}
		user.TenantID = authorization.TenantID
		user.OrganizationID = authorization.OrganizationID
	}
	resolved, err := resolveClientModuleSchema(data, user, moduleCode, modelName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if isWeChatMiniProgramRequest(r) {
		resolved, err = resolveMiniProgramCompliantModuleSchema(data, user, moduleCode, resolved)
		if err != nil {
			writeError(w, http.StatusForbidden, err)
			return
		}
	}
	writeJSON(w, moduleSchemaResponse(resolved, user))
}

func (a api) publicModuleSchema(w http.ResponseWriter, r *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	data = normalizeAICapabilityDefaults(data)
	moduleCode := canonicalModuleCode(firstNonEmptyString(
		r.URL.Query().Get("module_code"),
		r.URL.Query().Get("moduleCode"),
	))
	modelName := strings.TrimSpace(firstNonEmptyString(
		r.URL.Query().Get("model_name"),
		r.URL.Query().Get("modelName"),
	))
	resolved, err := resolvePublicModuleSchema(data, moduleCode, modelName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if isWeChatMiniProgramRequest(r) {
		if err := validateExactMiniProgramModuleSchema(resolved); err != nil {
			writeError(w, http.StatusForbidden, err)
			return
		}
	}
	writeJSON(w, publicModuleSchemaResponse(resolved))
}

func resolveClientModuleSchema(data adminPlatformData, user adminUser, moduleCode string, modelName string) (resolvedModuleSchema, error) {
	resolved, err := resolveModuleSchema(data, user, moduleCode, modelName)
	if err == nil || strings.TrimSpace(modelName) == "" || canonicalModuleCode(moduleCode) == moduleImageGeneration {
		return resolved, err
	}
	requestedModel := findAIModel(data.AIModels, moduleCode, modelName)
	fallbackName := strings.TrimSpace(requestedModel.FallbackModel)
	if requestedModel.ID == "" || !requestedModel.AllowFallbackSwitch || fallbackName == "" {
		return resolvedModuleSchema{}, err
	}
	fallback, fallbackErr := resolveModuleSchema(data, user, moduleCode, fallbackName)
	if fallbackErr != nil {
		return resolvedModuleSchema{}, err
	}
	return fallback, nil
}

func (a api) aiOverview(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{
		"summary": map[string]any{
			"modules": len(data.AIModules), "models": len(data.AIModels), "schemas": len(data.AIParameterSchemas),
			"limits": len(data.TenantModuleLimits), "billingRules": len(data.BillingRules), "logs": len(data.GenerationTasks) + len(data.BillingEvents),
		},
		"modules":      data.AIModules,
		"models":       data.AIModels,
		"schemas":      data.AIParameterSchemas,
		"limits":       data.TenantModuleLimits,
		"channels":     data.APIChannels,
		"billingRules": data.BillingRules,
		"logs":         aiGenerationLogRows(data),
	})
}

func (a adminAPI) aiOverview(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{
		"summary": map[string]any{
			"modules": len(data.AIModules), "models": len(data.AIModels), "schemas": len(data.AIParameterSchemas),
			"limits": len(data.TenantModuleLimits), "billingRules": len(data.BillingRules), "logs": len(data.GenerationTasks) + len(data.BillingEvents),
		},
		"modules":      data.AIModules,
		"models":       data.AIModels,
		"schemas":      data.AIParameterSchemas,
		"limits":       data.TenantModuleLimits,
		"channels":     data.APIChannels,
		"billingRules": data.BillingRules,
		"logs":         aiGenerationLogRows(data),
	})
}

func (a adminAPI) updateAIModule(w http.ResponseWriter, r *http.Request) {
	var req adminAIModuleMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.UpdateAdminAIModule(r.PathValue("code"), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) createAIModel(w http.ResponseWriter, r *http.Request) {
	var req adminAIModelMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.CreateAdminAIModel(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) updateAIModel(w http.ResponseWriter, r *http.Request) {
	var req adminAIModelMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.UpdateAdminAIModel(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) updateAIParameterSchema(w http.ResponseWriter, r *http.Request) {
	var req adminAIParameterSchemaMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.UpdateAdminAIParameterSchema(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) updateTenantModuleLimit(w http.ResponseWriter, r *http.Request) {
	var req adminTenantModuleLimitMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.UpdateAdminTenantModuleLimit(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) updateBillingRule(w http.ResponseWriter, r *http.Request) {
	var req adminBillingRuleMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.UpdateAdminBillingRule(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a api) prepareGenerationRequest(data adminPlatformData, user adminUser, req generation.CreateRequest) (generation.CreateRequest, error) {
	return a.prepareGenerationRequestWithAuthorization(data, user, req, nil)
}

func (a api) prepareGenerationRequestWithAuthorization(data adminPlatformData, user adminUser, req generation.CreateRequest, authorizationOverride *modelCallAuthorization) (generation.CreateRequest, error) {
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	moduleCode := requestModuleCode(req)
	if moduleCode == "" {
		moduleCode = moduleCodeForType(req.Type)
	}
	moduleCode = canonicalModuleCode(moduleCode)
	if moduleCode == "" {
		return req, errors.New("module_code is required")
	}
	if moduleCode == modulePPTGeneration {
		return req, errors.New("ppt_generation must use /api/v1/ppt/generate")
	}
	if moduleCode == moduleSmartVideoEditing {
		return req, errors.New("smart_video_editing must use /video-projects APIs")
	}
	if req.Type == "" {
		req.Type = defaultTaskTypeForModule(moduleCode)
	}
	if !typeBelongsToModule(req.Type, moduleCode) {
		return req, fmt.Errorf("task type %s does not belong to module_code %s", req.Type, moduleCode)
	}
	req.ModuleCode = moduleCode
	req.ModuleCodeCamel = ""
	if req.Prompt == "" {
		req.Prompt = strings.TrimSpace(stringValue(req.Params["prompt"]))
	}
	if req.Model == "" {
		req.Model = defaultModelNameForModule(data, moduleCode)
	}
	authorization := modelCallAuthorization{}
	if authorizationOverride != nil {
		authorization = *authorizationOverride
	} else {
		authorization = modelCallAuthorization{
			ContextType: contextPersonal, TenantID: "tenant_default", OrganizationID: defaultOrganizationID("tenant_default"),
			UserID: user.ID, Role: roleUser, BillingScope: contextPersonal, BillingAccountID: user.ID, ServiceState: "ACTIVE",
		}
		if authorizer, ok := a.store.(modelCallAuthorizer); ok {
			resolvedAuthorization, authErr := authorizer.AuthorizeModelCall(user.ID, moduleCode)
			if authErr != nil {
				return req, authErr
			}
			authorization = resolvedAuthorization
		}
	}
	user.TenantID = authorization.TenantID
	user.OrganizationID = authorization.OrganizationID
	normalizeRequestParamAliases(&req)
	resolved, err := resolveModuleSchema(data, user, moduleCode, req.Model)
	if err != nil {
		return req, err
	}
	req.Model = resolved.Model.ModelName
	normalizeGPTImageCanonicalParams(&req)
	if moduleCode == moduleVideoGeneration {
		if err := validateVideoGenerationRequest(&req, resolved); err != nil {
			return req, err
		}
	}
	removeLegacyGenerationMetadata(&req, resolved)
	normalizeGenerationQualityForLimit(&req, resolved)
	if err := validateGenerationParams(req, resolved); err != nil {
		return req, err
	}
	req.Model = resolved.Model.ModelName
	req.Params["module_code"] = moduleCode
	req.Params["model_name"] = req.Model
	req.Params["billing_type"] = resolved.BillingRule.BillingType
	req.Params["final_schema_snapshot"] = map[string]any{"fields": resolved.FinalSchema.Fields}
	req.Params["limit_snapshot"] = resolved.Limit.LimitJSON
	req.Params["tenant_id"] = authorization.TenantID
	req.Params["organization_id"] = authorization.OrganizationID
	req.Params["billing_scope"] = authorization.BillingScope
	req.Params["billing_account_id"] = authorization.BillingAccountID
	req.Params["authorized_role"] = authorization.Role
	req.Params["agent_id"] = effectiveAgentID(data, user)
	req.Params["package_id"] = user.PlanID
	return req, nil
}

func removeLegacyGenerationMetadata(req *generation.CreateRequest, resolved resolvedModuleSchema) {
	if req == nil || req.Params == nil {
		return
	}
	allowedFields := map[string]bool{}
	for _, field := range resolved.Schema.SchemaJSON.Fields {
		allowedFields[field.Key] = true
	}
	for _, key := range []string{
		"index", "providerRevisedPrompt", "provider_revised_prompt", "referenceCount",
		"type", "module_code", "billing_type", "sourceType", "contentType", "source", "provider",
		"providerTaskId", "provider_task_id", "thumbnailUrl", "thumbnail_url", "width", "height", "resolution",
		"fileId", "storageFileId", "storageTenantId", "storageProvider", "storageBucket", "storageObjectKey",
		"fileSize", "fileSizeBytes", "sourceUrl", "storageManaged", "inputImageIds", "inputImagesSnapshot",
		"terminal", "provider_name", "provider_company", "algorithm_name", "algorithm_filing_no", "algorithm_type",
		"model_version", "input_audit_status", "input_audit_service", "input_audit_request_id",
		"output_audit_status", "output_audit_service", "output_audit_request_id", "output_audit_reason",
		"ai_generated", "ai_label_status", "ai_label_text", "content_id", "generated_at", "download_derivative_required",
	} {
		if !allowedFields[key] {
			delete(req.Params, key)
		}
	}
}

func normalizeGenerationQualityForLimit(req *generation.CreateRequest, resolved resolvedModuleSchema) {
	if req == nil || req.Params == nil {
		return
	}
	value, ok := req.Params["quality"]
	if !ok || !hasNonEmptyValue(value) {
		return
	}
	var schemaField adminAIParameterField
	var finalField adminAIParameterField
	for _, field := range resolved.Schema.SchemaJSON.Fields {
		if field.Key == "quality" {
			schemaField = field
			break
		}
	}
	for _, field := range resolved.FinalSchema.Fields {
		if field.Key == "quality" {
			finalField = field
			break
		}
	}
	if len(schemaField.Options) == 0 || len(finalField.Options) == 0 {
		return
	}
	if !anyListContains(schemaField.Options, value) || anyListContains(finalField.Options, value) {
		return
	}
	if finalField.Default != nil && anyListContains(finalField.Options, finalField.Default) {
		req.Params["quality"] = finalField.Default
		return
	}
	req.Params["quality"] = finalField.Options[0]
}

func resolveModuleSchema(data adminPlatformData, user adminUser, moduleCode string, modelName string) (resolvedModuleSchema, error) {
	moduleCode = canonicalModuleCode(moduleCode)
	if moduleCode == "" {
		return resolvedModuleSchema{}, errors.New("module_code is required")
	}
	module := findAIModule(data.AIModules, moduleCode)
	if module.ID == "" {
		return resolvedModuleSchema{}, fmt.Errorf("ai module not found: %s", moduleCode)
	}
	if !isActiveLike(module.Status) {
		return resolvedModuleSchema{}, fmt.Errorf("ai module is disabled: %s", moduleCode)
	}
	enterpriseContext := isEnterpriseCapabilityContext(user)
	if !enterpriseContext {
		if err := validatePersonalPackagePeriod(user, time.Now().UTC()); err != nil {
			return resolvedModuleSchema{}, err
		}
	}
	if !enterpriseContext && !stringListContains(module.OpenPackageIDs, user.PlanID) {
		if moduleCode == moduleVideoGeneration {
			return resolvedModuleSchema{}, fmt.Errorf("当前套餐不支持视频生成，请升级后重试")
		}
		return resolvedModuleSchema{}, fmt.Errorf("当前套餐不支持该能力，请升级后重试")
	}
	if enterpriseContext && len(module.OpenTenantIDs) > 0 && !stringListContains(module.OpenTenantIDs, effectiveTenantID(user)) {
		return resolvedModuleSchema{}, fmt.Errorf("module %s is not open to tenant %s", moduleCode, effectiveTenantID(user))
	}
	commercialAgent := userHasActiveChannelProfile(data, user.ID)
	if commercialAgent && !module.AllowAgents {
		return resolvedModuleSchema{}, fmt.Errorf("module %s is not open to agents", moduleCode)
	}
	if !commercialAgent && !module.AllowEndUsers {
		return resolvedModuleSchema{}, fmt.Errorf("module %s is not open to end users", moduleCode)
	}
	if modelName == "" {
		modelName = defaultAllowedModelNameForModule(data, user, moduleCode)
	}
	model := findAIModel(data.AIModels, moduleCode, modelName)
	if model.ID == "" {
		return resolvedModuleSchema{}, fmt.Errorf("ai model %s is not configured for module %s", modelName, moduleCode)
	}
	if !isActiveLike(model.Status) {
		return resolvedModuleSchema{}, fmt.Errorf("ai model is disabled: %s", model.ModelName)
	}
	limit := effectiveTenantModuleLimit(data.TenantModuleLimits, user, moduleCode, model.ModelName)
	return resolveConfiguredModuleSchema(data, module, model, limit)
}

func resolvePublicModuleSchema(data adminPlatformData, moduleCode string, modelName string) (resolvedModuleSchema, error) {
	moduleCode = canonicalModuleCode(moduleCode)
	if moduleCode == "" {
		return resolvedModuleSchema{}, errors.New("module_code is required")
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return resolvedModuleSchema{}, errors.New("model_name is required")
	}
	module := findAIModule(data.AIModules, moduleCode)
	if module.ID == "" {
		return resolvedModuleSchema{}, fmt.Errorf("ai module not found: %s", moduleCode)
	}
	if !isActiveLike(module.Status) {
		return resolvedModuleSchema{}, fmt.Errorf("ai module is disabled: %s", moduleCode)
	}
	model := findAIModel(data.AIModels, moduleCode, modelName)
	if model.ID == "" {
		return resolvedModuleSchema{}, fmt.Errorf("ai model %s is not configured for module %s", modelName, moduleCode)
	}
	if !isActiveLike(model.Status) {
		return resolvedModuleSchema{}, fmt.Errorf("ai model is disabled: %s", model.ModelName)
	}
	publicContext := adminUser{TenantID: "default"}
	limit := effectiveTenantModuleLimit(data.TenantModuleLimits, publicContext, moduleCode, model.ModelName)
	return resolveConfiguredModuleSchema(data, module, model, limit)
}

func resolveConfiguredModuleSchema(
	data adminPlatformData,
	module adminAIModule,
	model adminAIModel,
	limit adminTenantModuleLimit,
) (resolvedModuleSchema, error) {
	moduleCode := canonicalModuleCode(module.ModuleCode)
	var schema adminAIParameterSchema
	if moduleCode == moduleImageGeneration {
		schema = findExactAIParameterSchema(data.AIParameterSchemas, moduleCode, model.ModelName)
	} else {
		schema = findAIParameterSchema(data.AIParameterSchemas, moduleCode, model.ModelName)
	}
	if schema.ID == "" && moduleCode != moduleImageGeneration {
		schema = findAIParameterSchema(data.AIParameterSchemas, moduleCode, "")
	}
	if schema.ID == "" {
		return resolvedModuleSchema{}, fmt.Errorf("parameter schema not found for model %s in module %s", model.ModelName, moduleCode)
	}
	if err := validateModelAllowedByLimit(model.ModelName, limit.LimitJSON); err != nil {
		return resolvedModuleSchema{}, err
	}
	rule := selectBillingRule(data.BillingRules, moduleCode, model.ModelName)
	if rule.ID == "" {
		rule = fallbackBillingRule(moduleCode, model.ModelName)
	}
	finalSchema := applyLimitToSchema(schema.SchemaJSON, limit.LimitJSON)
	if moduleCode == moduleVideoGeneration {
		capabilities := resolveVideoModelCapabilities(model, schema.SchemaJSON)
		capabilities.SupportedParameters = resolveVideoProviderSupportedParameters(data, model)
		model.VideoCapabilities = &capabilities
		model.CapabilityCode = syncVideoCapabilityCodes(model.CapabilityCode, capabilities)
		model.CapabilityCodeCamel = append([]string(nil), model.CapabilityCode...)
		finalSchema = applyVideoCapabilitiesToSchema(finalSchema, capabilities)
	}
	return resolvedModuleSchema{Module: module, Model: model, Schema: schema, FinalSchema: finalSchema, Limit: limit, BillingRule: rule}, nil
}

func resolveVideoProviderSupportedParameters(data adminPlatformData, model adminAIModel) []string {
	if strings.EqualFold(strings.TrimSpace(model.ModelName), "mock-video") {
		return []string{"duration", "resolution", "aspect_ratio", "fps", "generate_audio", "motion_strength", "camera_movement"}
	}
	channel, routed, err := selectAPIChannelForConfiguredModel(data, model.ModelName)
	if err != nil || !routed || strings.EqualFold(strings.TrimSpace(channel.Protocol), "cloudbase-function") {
		return append([]string(nil), videoCoreParameters...)
	}
	return videoprovider.OpenAICompatibleSupportedParameters(videoprovider.OpenAICompatibleOptions{
		Code:     channel.ID,
		BaseURL:  channel.BaseURL,
		Model:    model.ModelName,
		Models:   channel.Models,
		Endpoint: channel.VideoGenerationEndpoint,
	}, model.ModelName)
}

func moduleSchemaResponse(resolved resolvedModuleSchema, user adminUser) map[string]any {
	return map[string]any{
		"module_code":        resolved.Module.ModuleCode,
		"model_name":         resolved.Model.ModelName,
		"schema":             resolved.FinalSchema,
		"fields":             resolved.FinalSchema.Fields,
		"limit_json":         resolved.Limit.LimitJSON,
		"module":             resolved.Module,
		"model":              resolved.Model,
		"video_capabilities": resolved.Model.VideoCapabilities,
		"billing_rule":       resolved.BillingRule,
		"context": map[string]any{
			"user_id": user.ID, "tenant_id": effectiveTenantID(user), "agent_id": user.ReferredBy, "package_id": user.PlanID,
		},
	}
}

func publicModuleSchemaResponse(resolved resolvedModuleSchema) map[string]any {
	return map[string]any{
		"module_code":        resolved.Module.ModuleCode,
		"model_name":         resolved.Model.ModelName,
		"schema":             resolved.FinalSchema,
		"fields":             resolved.FinalSchema.Fields,
		"video_capabilities": resolved.Model.VideoCapabilities,
	}
}

func validateGenerationParams(req generation.CreateRequest, resolved resolvedModuleSchema) error {
	schema := resolved.Schema.SchemaJSON
	if canonicalModuleCode(resolved.Module.ModuleCode) == moduleVideoGeneration {
		schema = resolved.FinalSchema
		schema.Fields = append([]adminAIParameterField(nil), schema.Fields...)
		for index := range schema.Fields {
			if strings.EqualFold(strings.TrimSpace(schema.Fields[index].Key), "first_frame") {
				// IMAGE_TO_VIDEO presence is mode-dependent and is enforced by
				// validateVideoGenerationRequest, not by the shared schema pass.
				schema.Fields[index].Required = false
			}
		}
	}
	fields := map[string]adminAIParameterField{}
	for _, rawField := range schema.Fields {
		field := canonicalGenerationSchemaField(resolved.Module.ModuleCode, rawField)
		fields[field.Key] = field
		if _, ok := req.Params[field.Key]; !ok && field.Default != nil {
			req.Params[field.Key] = field.Default
		}
	}
	for _, rawField := range schema.Fields {
		field := canonicalGenerationSchemaField(resolved.Module.ModuleCode, rawField)
		value, ok := valueForField(req, field.Key)
		if field.Required && !hasNonEmptyValue(value) {
			return fmt.Errorf("parameter %s is required", field.Key)
		}
		if !ok || !hasNonEmptyValue(value) {
			continue
		}
		if isGPTImage2SchemaModel(resolved.Model.ModelName) && field.Key == "size" {
			if err := imageprovider.ValidateGPTImageSize(value); err != nil {
				return err
			}
		} else if isGPTImage2SchemaModel(resolved.Model.ModelName) && field.Key == "quality" {
			if err := imageprovider.ValidateGPTImageQuality(value); err != nil {
				return err
			}
		} else if err := validateFieldValue(field, value); err != nil {
			return err
		}
		if err := validateLimitValue(field.Key, value, resolved.Limit.LimitJSON); err != nil {
			return err
		}
	}
	for key := range req.Params {
		if _, ok := fields[key]; ok || allowedGenerationInternalParam(key) {
			continue
		}
		return fmt.Errorf("parameter %s is not allowed by schema for module %s", key, resolved.Module.ModuleCode)
	}
	return nil
}

func canonicalGenerationSchemaField(moduleCode string, field adminAIParameterField) adminAIParameterField {
	if canonicalModuleCode(moduleCode) != moduleVideoGeneration {
		return field
	}
	switch strings.TrimSpace(field.Key) {
	case "ratio":
		field.Key = "aspect_ratio"
	case "generateAudio":
		field.Key = "generate_audio"
	}
	return field
}

func validateFieldValue(field adminAIParameterField, value any) error {
	switch field.Type {
	case "number":
		number, ok := anyToFloat(value)
		if !ok {
			return fmt.Errorf("parameter %s must be a number", field.Key)
		}
		if field.Min != nil && number < *field.Min {
			return fmt.Errorf("parameter %s must be >= %s", field.Key, formatFloat(*field.Min))
		}
		if field.Max != nil && number > *field.Max {
			return fmt.Errorf("parameter %s must be <= %s", field.Key, formatFloat(*field.Max))
		}
	case "select", "radio":
		if len(field.Options) > 0 && !anyListContains(field.Options, value) {
			return fmt.Errorf("parameter %s value %v is not in schema options", field.Key, value)
		}
	}
	return nil
}

func validateLimitValue(key string, value any, limit map[string]any) error {
	rule, ok := mapValue(limit[key])
	if !ok {
		return nil
	}
	if enabled, ok := optionalBoolValue(rule["enabled"]); ok && !enabled && hasNonEmptyValue(value) {
		return fmt.Errorf("parameter %s is disabled by tenant/package limit", key)
	}
	if allowed, ok := anySlice(rule["allowed"]); ok {
		if len(allowed) == 0 {
			return fmt.Errorf("parameter %s is disabled by an empty tenant/package allowed list", key)
		}
		if !anyListContains(allowed, value) {
			return fmt.Errorf("parameter %s value %v exceeds tenant/package allowed list", key, value)
		}
	}
	if max, ok := anyToFloat(rule["max"]); ok {
		if number, numberOK := anyToFloat(value); numberOK && number > max {
			return fmt.Errorf("parameter %s must be <= %s by tenant/package limit", key, formatFloat(max))
		}
	}
	if min, ok := anyToFloat(rule["min"]); ok {
		if number, numberOK := anyToFloat(value); numberOK && number < min {
			return fmt.Errorf("parameter %s must be >= %s by tenant/package limit", key, formatFloat(min))
		}
	}
	return nil
}

func applyLimitToSchema(schema adminAIParameterSchemaJSON, limit map[string]any) adminAIParameterSchemaJSON {
	result := adminAIParameterSchemaJSON{Fields: make([]adminAIParameterField, 0, len(schema.Fields))}
	for _, field := range schema.Fields {
		if rule, ok := mapValue(limit[field.Key]); ok {
			if enabled, ok := optionalBoolValue(rule["enabled"]); ok && !enabled {
				field.Visible = false
				field.UserEditable = false
			}
			if allowed, ok := anySlice(rule["allowed"]); ok {
				field.Options = allowed
			}
			if max, ok := anyToFloat(rule["max"]); ok {
				field.Max = &max
			}
			if min, ok := anyToFloat(rule["min"]); ok {
				field.Min = &min
			}
		}
		result.Fields = append(result.Fields, field)
	}
	return result
}

func generationPointCostForRequest(req createGenerationTaskRequest, data adminPlatformData) int {
	rule := billingRuleForRequest(req, data)
	if rule.ID == "" {
		moduleCode := canonicalModuleCode(requestModuleCode(req))
		if moduleCode == "" {
			moduleCode = moduleCodeForType(req.Type)
		}
		if moduleCode == moduleImageGeneration || moduleCode == "" {
			return imageCount(req.Params) * modelPointCost(req.Model)
		}
		return 1
	}
	quantity := billingQuantity(rule.BillingType, req)
	multiplier := billingMultiplier(rule.ParameterMultiplier, billingParamsForRequest(req.Model, req.Params, rule.ParameterMultiplier))
	total := int(math.Ceil(rule.BasePrice * quantity * multiplier))
	minimumCharge := int(math.Ceil(rule.MinimumCharge))
	if total < minimumCharge {
		total = minimumCharge
	}
	if total < 1 {
		return 1
	}
	return total
}

func billingRuleForRequest(req createGenerationTaskRequest, data adminPlatformData) adminBillingRule {
	data = normalizeAICapabilityDefaults(data)
	moduleCode := canonicalModuleCode(requestModuleCode(req))
	if moduleCode == "" {
		moduleCode = moduleCodeForType(req.Type)
	}
	rule := selectBillingRule(data.BillingRules, moduleCode, req.Model)
	if rule.ID == "" {
		return fallbackBillingRule(moduleCode, req.Model)
	}
	return rule
}

func adminDataFromPlatformData(data platformData) adminPlatformData {
	return applyPublishedBillingRulesV1(normalizeAICapabilityDefaults(adminPlatformData{
		Users:                  data.Users,
		Plans:                  data.Plans,
		PointAccounts:          data.PointAccounts,
		TokenRecords:           data.TokenRecords,
		Orders:                 data.Orders,
		Payments:               data.Payments,
		PaymentEvents:          data.PaymentEvents,
		ChannelAgents:          data.ChannelAgents,
		OperationCenters:       data.OperationCenters,
		CustomerRelations:      data.CustomerRelations,
		Commissions:            data.Commissions,
		CommissionRules:        data.CommissionRules,
		BillingRules:           data.BillingRules,
		BillingEvents:          data.BillingEvents,
		BillingRuleVersions:    data.BillingRuleVersions,
		ProviderCosts:          data.ProviderCosts,
		BillingLifecycleEvents: data.BillingLifecycleEvents,
		WalletLedger:           data.WalletLedger,
		PersonalPoints:         data.PersonalPoints,
		PersonalPointImport:    data.PersonalPointImport,
		Withdrawals:            data.Withdrawals,
		Presentations:          data.Presentations,
		Agents:                 data.Agents,
		AgentCalls:             data.AgentCalls,
		GeoBrands:              data.GeoBrands,
		GeoTasks:               data.GeoTasks,
		AdminProducts:          data.AdminProducts,
		SystemSettings:         data.SystemSettings,
		APIChannels:            data.APIChannels,
		APIModels:              data.APIModels,
		APIKeys:                data.APIKeys,
		CustomerGroups:         data.CustomerGroups,
		AIModules:              data.AIModules,
		AIModels:               data.AIModels,
		AIParameterSchemas:     data.AIParameterSchemas,
		TenantModuleLimits:     data.TenantModuleLimits,
		Enterprise:             data.Enterprise,
		PromotionRecords:       data.PromotionRecords,
		AuthMergeRequests:      data.AuthMergeRequests,
		AdminExceptionCases:    data.AdminExceptionCases,
		AdminExperienceEvents:  data.AdminExperienceEvents,
		GenerationTasks:        data.GenerationTasks,
		Assets:                 data.Assets,
		AIState:                data.AIState,
		Counters:               data.Counters,
		PointsAvailable:        data.PointsAvailable,
	}))
}

func applyAdminDataToPlatformData(data *platformData, admin adminPlatformData) {
	if data == nil {
		return
	}
	data.Users = admin.Users
	data.Plans = admin.Plans
	data.PointAccounts = admin.PointAccounts
	data.TokenRecords = admin.TokenRecords
	data.Orders = admin.Orders
	data.Payments = admin.Payments
	data.PaymentEvents = admin.PaymentEvents
	data.ChannelAgents = admin.ChannelAgents
	data.OperationCenters = admin.OperationCenters
	data.CustomerRelations = admin.CustomerRelations
	data.Commissions = admin.Commissions
	data.CommissionRules = admin.CommissionRules
	data.BillingRules = admin.BillingRules
	data.BillingEvents = admin.BillingEvents
	data.BillingRuleVersions = admin.BillingRuleVersions
	data.ProviderCosts = admin.ProviderCosts
	data.BillingLifecycleEvents = admin.BillingLifecycleEvents
	data.WalletLedger = admin.WalletLedger
	data.PersonalPoints = admin.PersonalPoints
	data.PersonalPointImport = admin.PersonalPointImport
	data.Withdrawals = admin.Withdrawals
	data.Presentations = admin.Presentations
	data.Agents = admin.Agents
	data.AgentCalls = admin.AgentCalls
	data.GeoBrands = admin.GeoBrands
	data.GeoTasks = admin.GeoTasks
	data.AdminProducts = admin.AdminProducts
	data.SystemSettings = admin.SystemSettings
	data.APIChannels = admin.APIChannels
	data.APIModels = admin.APIModels
	data.APIKeys = admin.APIKeys
	data.CustomerGroups = admin.CustomerGroups
	data.AIModules = admin.AIModules
	data.AIModels = admin.AIModels
	data.AIParameterSchemas = admin.AIParameterSchemas
	data.TenantModuleLimits = admin.TenantModuleLimits
	data.Enterprise = admin.Enterprise
	data.PromotionRecords = admin.PromotionRecords
	data.AuthMergeRequests = admin.AuthMergeRequests
	data.AdminExceptionCases = admin.AdminExceptionCases
	data.AdminExperienceEvents = admin.AdminExperienceEvents
	data.GenerationTasks = admin.GenerationTasks
	data.Assets = admin.Assets
	data.AIState = admin.AIState
	data.Counters = admin.Counters
	data.PointsAvailable = admin.PointsAvailable
}

func billingQuantity(billingType string, req createGenerationTaskRequest) float64 {
	switch strings.ToLower(strings.TrimSpace(billingType)) {
	case "per_second":
		if value, ok := anyToFloat(req.Params["duration"]); ok && value > 0 {
			return value
		}
		return 1
	case "per_page":
		if value, ok := anyToFloat(firstPresent(req.Params, "page_count", "slideCount", "pageCount")); ok && value > 0 {
			return value
		}
		return 1
	case "per_image":
		return float64(imageCount(req.Params))
	default:
		return 1
	}
}

func billingQuantityField(billingType string) string {
	switch strings.ToLower(strings.TrimSpace(billingType)) {
	case "per_second":
		return "duration"
	case "per_page":
		return "page_count"
	case "per_image":
		return "count"
	default:
		return "request"
	}
}

func billingMultiplier(config map[string]any, params map[string]any) float64 {
	multiplier := 1.0
	for key, raw := range config {
		options, ok := mapValue(raw)
		if !ok {
			continue
		}
		value := firstPresent(params, key)
		if value == nil {
			continue
		}
		if ratio, ok := anyToFloat(options[fmt.Sprint(value)]); ok && ratio > 0 {
			multiplier *= ratio
			continue
		}
		if ratio, ok := anyToFloat(options[strings.ToLower(fmt.Sprint(value))]); ok && ratio > 0 {
			multiplier *= ratio
		}
	}
	if multiplier <= 0 {
		return 1
	}
	return multiplier
}

func applyGenerationTaskCapabilitySnapshot(task *generationTask, req createGenerationTaskRequest, rule adminBillingRule) {
	if task == nil {
		return
	}
	moduleCode := canonicalModuleCode(requestModuleCode(req))
	if moduleCode == "" {
		moduleCode = canonicalModuleCode(stringValue(req.Params["module_code"]))
	}
	task.ModuleCode = moduleCode
	task.BillingType = firstNonEmptyString(stringValue(req.Params["billing_type"]), rule.BillingType)
	if task.BillingRuleVersionID == "" && rule.Version > 0 {
		task.BillingRuleVersionID = rule.ID
	}
	task.TenantID = stringValue(req.Params["tenant_id"])
	task.OrganizationID = stringValue(req.Params["organization_id"])
	task.BillingAccountType = firstNonEmptyString(stringValue(req.Params["billing_scope"]), contextPersonal)
	task.BillingAccountID = firstNonEmptyString(stringValue(req.Params["billing_account_id"]), task.UserID)
	task.AgentID = stringValue(req.Params["agent_id"])
	task.OperationCenterID = stringValue(req.Params["operation_center_id"])
	task.FinalSchemaSnapshot, _ = mapValue(req.Params["final_schema_snapshot"])
	task.LimitSnapshot, _ = mapValue(req.Params["limit_snapshot"])
	task.UpstreamProvider = firstNonEmptyString(stringValue(req.Params["provider"]), stringValue(req.Params["upstream_provider"]))
	task.UpstreamRequestID = providerTaskString(req, "id")
	task.UserChargeAmount = task.PointCost * pointUnitAmountCents
	task.UpstreamCost = int(math.Ceil(rule.CostPrice * billingQuantity(rule.BillingType, req)))
	task.PlatformProfit = task.UserChargeAmount - task.UpstreamCost
}

func enrichBillingEventWithTask(event adminBillingEvent, task generationTask) adminBillingEvent {
	event.ModuleCode = task.ModuleCode
	event.TenantID = task.TenantID
	event.OperationCenterID = task.OperationCenterID
	if event.Metadata == nil {
		event.Metadata = map[string]any{}
	}
	event.Metadata["module_code"] = task.ModuleCode
	event.Metadata["billing_type"] = task.BillingType
	event.Metadata["request_params"] = task.Params
	event.Metadata["final_schema_snapshot"] = task.FinalSchemaSnapshot
	event.Metadata["limit_snapshot"] = task.LimitSnapshot
	event.Metadata["upstream_provider"] = task.UpstreamProvider
	event.Metadata["upstream_request_id"] = task.UpstreamRequestID
	event.Metadata["upstream_cost"] = task.UpstreamCost
	event.Metadata["platform_profit"] = task.PlatformProfit
	return event
}

func aiGenerationLogRows(data adminPlatformData) []map[string]any {
	eventsByTask := map[string]adminBillingEvent{}
	for _, event := range data.BillingEvents {
		eventsByTask[event.TaskID] = event
	}
	users := userMap(data.Users)
	rows := []map[string]any{}
	for _, task := range data.GenerationTasks {
		moduleCode := firstNonEmptyString(task.ModuleCode, stringValue(task.Params["module_code"]), moduleCodeForType(task.Type))
		event := eventsByTask[task.ID]
		rows = append(rows, map[string]any{
			"id": task.ID, "user_id": task.UserID, "user": users[task.UserID].Name, "tenant_id": firstNonEmptyString(task.TenantID, stringValue(task.Params["tenant_id"])),
			"agent_id": firstNonEmptyString(task.AgentID, event.AgentID), "operation_center_id": task.OperationCenterID,
			"module_code": moduleCode, "model_name": firstNonEmptyString(task.Model, event.Model), "billing_type": firstNonEmptyString(task.BillingType, stringValue(task.Params["billing_type"])),
			"request_params": task.Params, "final_schema_snapshot": task.FinalSchemaSnapshot, "limit_snapshot": task.LimitSnapshot,
			"upstream_provider": firstNonEmptyString(task.UpstreamProvider, stringValue(task.Params["provider"])), "upstream_request_id": task.UpstreamRequestID,
			"user_charge_amount": firstNonEmptyInt(task.UserChargeAmount, event.AmountCents), "upstream_cost": task.UpstreamCost, "platform_profit": task.PlatformProfit,
			"agent_commission": task.AgentCommission, "operation_center_commission": task.OperationCenterCommission,
			"task_status": task.Status, "failure_reason": firstNonEmptyString(task.FailureReason, stringValue(task.Error)), "result_url": firstNonEmptyString(task.ResultURL, task.OutputURL, task.ImageURL),
			"created_at": task.CreatedAt, "updated_at": task.UpdatedAt,
		})
	}
	return rows
}

func requestModuleCode(req generation.CreateRequest) string {
	return canonicalModuleCode(firstNonEmptyString(req.ModuleCode, req.ModuleCodeCamel, stringValue(req.Params["module_code"]), stringValue(req.Params["moduleCode"])))
}

func moduleCodeForType(taskType string) string {
	switch strings.ToUpper(strings.TrimSpace(taskType)) {
	case "", "TEXT_TO_IMAGE", "IMAGE_TO_IMAGE":
		return moduleImageGeneration
	case "TEXT_TO_VIDEO", "IMAGE_TO_VIDEO", "VIDEO_TO_VIDEO":
		return moduleVideoGeneration
	case "PPT_GENERATION":
		return modulePPTGeneration
	case "SMART_VIDEO_EDITING", "AI_AUTO_MONTAGE":
		return moduleSmartVideoEditing
	default:
		return ""
	}
}

func canonicalModuleCode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	switch value {
	case "image", "text_to_image", "image_to_image", "image_generation":
		return moduleImageGeneration
	case "video", "text_to_video", "image_to_video", "video_generation":
		return moduleVideoGeneration
	case "ppt", "ppt_generation", "presentation", "presentation_generation":
		return modulePPTGeneration
	case "smart_video", "smart_video_editing", "ai_auto_montage", "auto_montage":
		return moduleSmartVideoEditing
	default:
		return ""
	}
}

func defaultTaskTypeForModule(moduleCode string) string {
	switch canonicalModuleCode(moduleCode) {
	case moduleVideoGeneration:
		return "TEXT_TO_VIDEO"
	case modulePPTGeneration:
		return "PPT_GENERATION"
	case moduleSmartVideoEditing:
		return "SMART_VIDEO_EDITING"
	default:
		return "TEXT_TO_IMAGE"
	}
}

func defaultAIModelTypeForModule(moduleCode string) string {
	switch canonicalModuleCode(moduleCode) {
	case moduleVideoGeneration:
		return "video"
	case modulePPTGeneration:
		return "text"
	case moduleSmartVideoEditing:
		return "workflow"
	default:
		return "image"
	}
}

func defaultAICapabilitiesForModule(moduleCode string) []string {
	switch canonicalModuleCode(moduleCode) {
	case moduleVideoGeneration:
		return []string{"text_to_video"}
	case modulePPTGeneration:
		return []string{"ppt_outline", "ppt_content"}
	case moduleSmartVideoEditing:
		return []string{capabilitySmartVideoPlan, capabilitySpeechSynthesis}
	default:
		return []string{"text_to_image"}
	}
}

func bindAIModelToModule(data *adminPlatformData, moduleCode string, modelName string) {
	if data == nil {
		return
	}
	moduleCode = canonicalModuleCode(moduleCode)
	modelName = strings.TrimSpace(modelName)
	if moduleCode == "" || modelName == "" {
		return
	}
	for index := range data.AIModules {
		if canonicalModuleCode(data.AIModules[index].ModuleCode) != moduleCode {
			continue
		}
		for _, existing := range data.AIModules[index].BoundModels {
			if strings.EqualFold(strings.TrimSpace(existing), modelName) {
				return
			}
		}
		data.AIModules[index].BoundModels = append(data.AIModules[index].BoundModels, modelName)
		data.AIModules[index].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return
	}
}

func typeBelongsToModule(taskType string, moduleCode string) bool {
	return moduleCodeForType(taskType) == canonicalModuleCode(moduleCode)
}

func normalizeRequestParamAliases(req *generation.CreateRequest) {
	if req == nil {
		return
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	if req.ModuleCode == moduleVideoGeneration {
		if _, ok := req.Params["aspect_ratio"]; !ok {
			if ratio, exists := req.Params["ratio"]; exists {
				req.Params["aspect_ratio"] = ratio
			}
		}
		delete(req.Params, "ratio")
		if _, ok := req.Params["generate_audio"]; !ok {
			if generateAudio, exists := req.Params["generateAudio"]; exists {
				req.Params["generate_audio"] = generateAudio
			}
		}
		delete(req.Params, "generateAudio")
	}
	normalizeGPTImageCanonicalParams(req)
}

func normalizeGPTImageCanonicalParams(req *generation.CreateRequest) {
	if req == nil || req.Params == nil {
		return
	}
	if !isGPTImage2SchemaModel(req.Model) {
		return
	}
	if !hasNonEmptyValue(req.Params["quality"]) {
		if alias, ok := canonicalGPTImageQualityValue(req.Params["imageQuality"]); ok {
			req.Params["quality"] = alias
		}
	}
	if _, ok := req.Params["n"]; !ok {
		if count, exists := req.Params["count"]; exists {
			req.Params["n"] = count
		}
	}
}

func canonicalGPTImageQualityValue(value any) (string, bool) {
	if !hasNonEmptyValue(value) {
		return "", false
	}
	quality := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
	switch quality {
	case "auto", "low", "medium", "high":
		return quality, true
	case "standard":
		return "auto", true
	case "draft":
		return "low", true
	default:
		return "", false
	}
}

func findAIModule(items []adminAIModule, moduleCode string) adminAIModule {
	for _, item := range items {
		if canonicalModuleCode(firstNonEmptyString(item.ModuleCode, item.ModuleCodeCamel)) == moduleCode {
			return item
		}
	}
	return adminAIModule{}
}

func findAIModel(items []adminAIModel, moduleCode string, modelName string) adminAIModel {
	for _, item := range items {
		if canonicalModuleCode(firstNonEmptyString(item.ModuleCode, item.ModuleCodeCamel)) != moduleCode {
			continue
		}
		if strings.EqualFold(firstNonEmptyString(item.ModelName, item.ModelNameCamel), modelName) {
			if item.ModelName == "" {
				item.ModelName = item.ModelNameCamel
			}
			return item
		}
	}
	return adminAIModel{}
}

func findAIParameterSchema(items []adminAIParameterSchema, moduleCode string, modelName string) adminAIParameterSchema {
	var fallback adminAIParameterSchema
	for _, item := range items {
		if canonicalModuleCode(firstNonEmptyString(item.ModuleCode, item.ModuleCodeCamel)) != moduleCode || !isActiveLike(item.Status) {
			continue
		}
		if modelName == "" && fallback.ID == "" {
			fallback = item
		}
		if modelName != "" && (strings.EqualFold(item.ModelName, modelName) || item.ModelName == "") {
			return item
		}
	}
	return fallback
}

func findExactAIParameterSchema(items []adminAIParameterSchema, moduleCode string, modelName string) adminAIParameterSchema {
	for _, item := range items {
		if canonicalModuleCode(firstNonEmptyString(item.ModuleCode, item.ModuleCodeCamel)) != moduleCode || !isActiveLike(item.Status) {
			continue
		}
		if strings.EqualFold(firstNonEmptyString(item.ModelName, item.ModelNameCamel), modelName) {
			if item.ModelName == "" {
				item.ModelName = item.ModelNameCamel
			}
			return item
		}
	}
	return adminAIParameterSchema{}
}

func defaultModelNameForModule(data adminPlatformData, moduleCode string) string {
	module := findAIModule(data.AIModules, moduleCode)
	if len(module.BoundModels) > 0 {
		return module.BoundModels[0]
	}
	for _, model := range data.AIModels {
		if canonicalModuleCode(model.ModuleCode) == moduleCode && isActiveLike(model.Status) {
			return model.ModelName
		}
	}
	return ""
}

func defaultAllowedModelNameForModule(data adminPlatformData, user adminUser, moduleCode string) string {
	defaultModel := defaultModelNameForModule(data, moduleCode)
	limit := effectiveTenantModuleLimit(data.TenantModuleLimits, user, moduleCode, "")
	models, ok := mapValue(limit.LimitJSON["models"])
	if !ok {
		return defaultModel
	}
	allowed, ok := anySlice(models["allowed"])
	if !ok {
		return defaultModel
	}
	if len(allowed) == 0 {
		return ""
	}
	if anyListContains(allowed, defaultModel) {
		return defaultModel
	}
	for _, value := range allowed {
		modelName := strings.TrimSpace(fmt.Sprint(value))
		model := findAIModel(data.AIModels, moduleCode, modelName)
		if model.ID != "" && isActiveLike(model.Status) {
			return model.ModelName
		}
	}
	return defaultModel
}

func validatePersonalPackagePeriod(user adminUser, now time.Time) error {
	expiresAt := strings.TrimSpace(user.SubscriptionExpiresAt)
	if expiresAt == "" {
		// Legacy and system-created users may not have a period snapshot yet.
		// Keep them compatible until the data backfill assigns an explicit date.
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, expiresAt)
	}
	if err != nil {
		return fmt.Errorf("package expiry is invalid for user %s", user.ID)
	}
	if !parsed.After(now) {
		return fmt.Errorf("package %s expired at %s", user.PlanID, parsed.UTC().Format(time.RFC3339))
	}
	return nil
}

func effectiveTenantModuleLimit(items []adminTenantModuleLimit, user adminUser, moduleCode string, modelName string) adminTenantModuleLimit {
	base := adminTenantModuleLimit{ID: "limit_system_default", TenantID: "default", ModuleCode: moduleCode, ModelName: modelName, Status: "ACTIVE", LimitJSON: map[string]any{}}
	candidates := []adminTenantModuleLimit{}
	for _, item := range items {
		if !isActiveLike(item.Status) || canonicalModuleCode(firstNonEmptyString(item.ModuleCode, item.ModuleCodeCamel)) != moduleCode {
			continue
		}
		itemModel := firstNonEmptyString(item.ModelName, item.ModelNameCamel)
		if itemModel != "" && !strings.EqualFold(itemModel, modelName) {
			continue
		}
		candidates = append(candidates, item)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return limitSpecificity(candidates[i], user) < limitSpecificity(candidates[j], user)
	})
	for _, item := range candidates {
		if limitMatchesUser(item, user) {
			base.LimitJSON = mergeMap(base.LimitJSON, firstNonNilMap(item.LimitJSON, item.LimitJSONCamel))
			base.ID = item.ID
			base.TenantID = firstNonEmptyString(item.TenantID, item.TenantIDCamel, base.TenantID)
			base.AgentID = firstNonEmptyString(item.AgentID, item.AgentIDCamel, base.AgentID)
			base.PackageID = firstNonEmptyString(item.PackageID, item.PackageIDCamel, base.PackageID)
		}
	}
	return base
}

func limitMatchesUser(item adminTenantModuleLimit, user adminUser) bool {
	tenantID := firstNonEmptyString(item.TenantID, item.TenantIDCamel)
	agentID := firstNonEmptyString(item.AgentID, item.AgentIDCamel)
	packageID := firstNonEmptyString(item.PackageID, item.PackageIDCamel)
	if tenantID != "" && tenantID != "default" && tenantID != effectiveTenantID(user) {
		return false
	}
	if agentID != "" && agentID != user.ReferredBy {
		return false
	}
	if packageID != "" {
		if isEnterpriseCapabilityContext(user) || packageID != user.PlanID {
			return false
		}
	}
	return true
}

func isEnterpriseCapabilityContext(user adminUser) bool {
	tenantID := strings.TrimSpace(effectiveTenantID(user))
	return tenantID != "" && !strings.EqualFold(tenantID, "default") && !strings.EqualFold(tenantID, "tenant_default")
}

func limitSpecificity(item adminTenantModuleLimit, user adminUser) int {
	score := 0
	if firstNonEmptyString(item.TenantID, item.TenantIDCamel) != "" {
		score++
	}
	if firstNonEmptyString(item.AgentID, item.AgentIDCamel) != "" {
		score += 2
	}
	if firstNonEmptyString(item.PackageID, item.PackageIDCamel) != "" {
		score += 3
	}
	return score
}

func validateModelAllowedByLimit(modelName string, limit map[string]any) error {
	models, ok := mapValue(limit["models"])
	if !ok {
		return nil
	}
	allowed, ok := anySlice(models["allowed"])
	if ok {
		if len(allowed) == 0 {
			return fmt.Errorf("no models are allowed by tenant/package limit")
		}
		if !anyListContains(allowed, modelName) {
			return fmt.Errorf("model %s is not allowed by tenant/package limit", modelName)
		}
	}
	return nil
}

func selectBillingRule(items []adminBillingRule, moduleCode string, modelName string) adminBillingRule {
	for _, item := range items {
		if !isActiveLike(item.Status) {
			continue
		}
		itemModuleCode := canonicalModuleCode(firstNonEmptyString(item.ModuleCode, item.ModuleCodeCamel))
		itemModelName := firstNonEmptyString(item.ModelName, item.ModelNameCamel)
		if itemModuleCode == moduleCode && strings.EqualFold(itemModelName, modelName) {
			return normalizeBillingRuleAliases(item)
		}
	}
	return adminBillingRule{}
}

func fallbackBillingRule(moduleCode string, modelName string) adminBillingRule {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	rule := adminBillingRule{ID: "billing_rule_fallback_" + safeID(moduleCode+"_"+modelName), ModuleCode: moduleCode, ModelName: modelName, BillingType: "per_request", BasePrice: 1, CostPrice: 0, CurrencyType: "credit", ParameterMultiplier: map[string]any{}, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now}
	if moduleCode == moduleImageGeneration {
		rule.BillingType = "per_image"
	}
	if moduleCode == moduleVideoGeneration {
		rule.BillingType = "per_second"
	}
	if moduleCode == modulePPTGeneration {
		rule.BillingType = "per_page"
	}
	return rule
}

func normalizeBillingRuleAliases(rule adminBillingRule) adminBillingRule {
	rule.ModuleCode = firstNonEmptyString(rule.ModuleCode, rule.ModuleCodeCamel)
	rule.ModelName = firstNonEmptyString(rule.ModelName, rule.ModelNameCamel)
	rule.BillingType = firstNonEmptyString(rule.BillingType, rule.BillingTypeCamel)
	if rule.BasePrice == 0 {
		rule.BasePrice = rule.BasePriceCamel
	}
	if rule.CostPrice == 0 {
		rule.CostPrice = rule.CostPriceCamel
	}
	rule.CurrencyType = firstNonEmptyString(rule.CurrencyType, rule.CurrencyTypeCamel)
	if rule.ParameterMultiplier == nil {
		rule.ParameterMultiplier = rule.ParameterMultiplierCamel
	}
	if rule.ParameterMultiplier == nil {
		rule.ParameterMultiplier = map[string]any{}
	}
	return rule
}

func allowedGenerationInternalParam(key string) bool {
	switch key {
	case "module_code", "moduleCode", "model_name", "modelName", "count", "provider", "providerName", "providerTask",
		"terminal", "provider_name", "provider_company", "algorithm_name", "algorithm_filing_no", "algorithm_type", "model_version", "compliance_snapshot_at",
		"input_audit_status", "input_audit_service", "input_audit_request_id", "output_audit_status", "output_audit_service", "output_audit_request_id", "output_audit_reason",
		"ai_generated", "ai_label_status", "ai_label_text", "generated_at", "download_derivative_required",
		"modelRouteId", "modelGroup", "modelApiKeyId", "billing_type", "tenant_id", "organization_id", "billing_scope", "billing_account_id", "authorized_role", "billing_ledger_id", "billing_reserved", "agent_id", "package_id", "operation_center_id",
		"final_schema_snapshot", "limit_snapshot", "sourceModule", "apiMode", "taskSnapshot", "referenceImages", "sourceReferenceAssetId", "sourceReferenceTaskId",
		"referenceImageCount", "referenceImageNames", "referenceImageOrder", "inputImageIds", "inputImagesSnapshot",
		"maskDraft", "maskTargetImageId", "maskImageId", "imageQuality", "imageRatio", "output_format", "outputFormat",
		"output_compression", "outputCompression", "transparent_output", "transparentOutput", "moderation", "ratio",
		"resolution", "width", "height", "inputMode", "hasInputImage", "hasInputVideo", "userPrompt", "effectivePrompt", "promptForApi",
		"generate_audio", "generateAudio",
		"image_url", "imageUrl", "image_urls", "imageUrls", "inputImageUrl", "input_image_url", "inputImageUrls",
		"reference_images", "input_reference", "inputVideoUrl", "video_url", "videoUrl",
		"purpose", "pptTaskId", "deckTitle", "slideId", "slidePage", "theme", "language", "visualPlan", "negativePrompt":
		return true
	default:
		return false
	}
}

func valueForField(req generation.CreateRequest, key string) (any, bool) {
	if key == "prompt" && strings.TrimSpace(req.Prompt) != "" {
		return req.Prompt, true
	}
	if key == "topic" && strings.TrimSpace(req.Prompt) != "" {
		return req.Prompt, true
	}
	if value, ok := req.Params[key]; ok {
		return value, true
	}
	if key == "n" {
		if value, ok := req.Params["count"]; ok {
			return value, true
		}
	}
	return nil, false
}

func firstPresent(params map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := params[key]; ok {
			return value
		}
	}
	return nil
}

func hasNonEmptyValue(value any) bool {
	if value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	return true
}

func anyOptions(values ...any) []any {
	return values
}

func floatPtr(value float64) *float64 {
	return &value
}

func anyToFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case json.Number:
		value, err := typed.Float64()
		return value, err == nil
	case string:
		var result float64
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%f", &result); err == nil {
			return result, true
		}
	}
	return 0, false
}

func anySlice(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []string:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
		return items, true
	case []int:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, float64(item))
		}
		return items, true
	}
	return nil, false
}

func anyListContains(items []any, value any) bool {
	for _, item := range items {
		if strings.EqualFold(fmt.Sprint(item), fmt.Sprint(value)) {
			return true
		}
		if left, ok := anyToFloat(item); ok {
			if right, rightOK := anyToFloat(value); rightOK && math.Abs(left-right) < 0.000001 {
				return true
			}
		}
	}
	return false
}

func mapValue(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	if typed, ok := value.(map[string]any); ok {
		return typed, true
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, false
	}
	return result, true
}

func optionalBoolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes", "enabled":
			return true, true
		case "false", "0", "no", "disabled":
			return false, true
		}
	}
	return false, false
}

func firstNonNilMap(values ...map[string]any) map[string]any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return map[string]any{}
}

func firstNonEmptyInt(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func isActiveLike(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "", "ACTIVE", "ENABLED", "CONFIGURABLE":
		return true
	default:
		return false
	}
}

func effectiveTenantID(user adminUser) string {
	return firstNonEmptyString(user.TenantID, "tenant_default")
}

func effectiveAgentID(data adminPlatformData, user adminUser) string {
	if user.ReferredBy == "" {
		return ""
	}
	if agent, ok := agentByUserMap(data.ChannelAgents)[user.ReferredBy]; ok {
		return agent.ID
	}
	return user.ReferredBy
}

func formatFloat(value float64) string {
	if math.Abs(value-math.Round(value)) < 0.000001 {
		return fmt.Sprintf("%.0f", value)
	}
	return fmt.Sprintf("%.2f", value)
}
