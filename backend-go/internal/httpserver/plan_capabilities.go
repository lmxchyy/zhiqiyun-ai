package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const packageCapabilityConfigVersion = 1

func (a adminAPI) planCapabilities(w http.ResponseWriter, r *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items, err := buildAdminPlanCapabilities(data, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, map[string]any{"planId": r.PathValue("id"), "items": items})
}

func (a adminAPI) updatePlanCapabilities(w http.ResponseWriter, r *http.Request) {
	var req adminPlanCapabilitiesMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(req.Modules) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("at least one capability module is required"))
		return
	}
	planID := strings.TrimSpace(r.PathValue("id"))
	if err := a.store.UpdateAdminPlanCapabilities(planID, req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items, err := buildAdminPlanCapabilities(data, planID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"planId": planID, "items": items})
}

func buildAdminPlanCapabilities(data adminPlatformData, planID string) ([]adminPlanCapabilityModule, error) {
	planID = strings.TrimSpace(planID)
	if _, ok := planMap(data.Plans)[planID]; !ok {
		return nil, fmt.Errorf("plan not found: %s", planID)
	}
	data = normalizeAICapabilityDefaults(data)
	items := make([]adminPlanCapabilityModule, 0, len(data.AIModules))
	for _, module := range data.AIModules {
		moduleCode := canonicalModuleCode(module.ModuleCode)
		availableModels := availableModelsForModule(data, module)
		user := adminUser{PlanID: planID}
		limit := effectiveTenantModuleLimit(data.TenantModuleLimits, user, moduleCode, "")
		limits := mergeMap(map[string]any{}, limit.LimitJSON)
		allowedModels := allowedModelsFromLimits(limits)
		if len(allowedModels) == 0 {
			allowedModels = append([]string{}, availableModels...)
		}
		items = append(items, adminPlanCapabilityModule{
			ModuleCode: moduleCode, Name: module.Name, Description: module.Description,
			Enabled: planStringListContains(module.OpenPackageIDs, planID), AllowedModels: allowedModels,
			AvailableModels: availableModels, Limits: limits,
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ModuleCode < items[j].ModuleCode })
	return items, nil
}

func applyAdminPlanCapabilities(data *adminPlatformData, planID string, req adminPlanCapabilitiesMutation) error {
	*data = normalizeAICapabilityDefaults(*data)
	planID = strings.TrimSpace(planID)
	if _, ok := planMap(data.Plans)[planID]; !ok {
		return fmt.Errorf("plan not found: %s", planID)
	}
	requested := map[string]adminPlanCapabilityModule{}
	for _, config := range req.Modules {
		moduleCode := canonicalModuleCode(config.ModuleCode)
		if moduleCode == "" {
			return errors.New("moduleCode is required")
		}
		if _, exists := requested[moduleCode]; exists {
			return fmt.Errorf("duplicate capability module: %s", moduleCode)
		}
		config.ModuleCode = moduleCode
		requested[moduleCode] = config
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for moduleIndex := range data.AIModules {
		module := &data.AIModules[moduleIndex]
		moduleCode := canonicalModuleCode(module.ModuleCode)
		config, ok := requested[moduleCode]
		if !ok {
			continue
		}
		available := availableModelsForModule(*data, *module)
		availableSet := map[string]bool{}
		for _, model := range available {
			availableSet[strings.ToLower(model)] = true
		}
		allowed := uniqueStringList(config.AllowedModels)
		if config.Enabled && len(available) > 0 && len(allowed) == 0 {
			return fmt.Errorf("enabled module %s requires at least one allowed model", moduleCode)
		}
		for _, model := range allowed {
			if !availableSet[strings.ToLower(model)] {
				return fmt.Errorf("model %s is not bound to module %s", model, moduleCode)
			}
		}
		if err := validatePackageCapabilityLimits(moduleCode, config.Enabled, config.Limits); err != nil {
			return err
		}
		module.OpenPackageIDs = setStringListMembership(module.OpenPackageIDs, planID, config.Enabled)
		if module.Config == nil {
			module.Config = map[string]any{}
		}
		module.Config["packageCapabilityVersion"] = packageCapabilityConfigVersion
		module.UpdatedAt = now

		limits := mergeMap(map[string]any{}, config.Limits)
		limits["models"] = map[string]any{"allowed": stringsToAny(allowed)}
		limitIndex := exactPackageModuleLimitIndex(data.TenantModuleLimits, planID, moduleCode)
		if limitIndex < 0 {
			data.TenantModuleLimits = append(data.TenantModuleLimits, adminTenantModuleLimit{
				ID: "limit_package_" + shortID(planID+"_"+moduleCode), TenantID: "default", PackageID: planID,
				ModuleCode: moduleCode, LimitJSON: limits, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now,
			})
		} else {
			data.TenantModuleLimits[limitIndex].LimitJSON = limits
			data.TenantModuleLimits[limitIndex].Status = "ACTIVE"
			data.TenantModuleLimits[limitIndex].UpdatedAt = now
		}
		delete(requested, moduleCode)
	}
	for moduleCode := range requested {
		return fmt.Errorf("ai module not found: %s", moduleCode)
	}
	return nil
}

func validatePackageCapabilityLimits(moduleCode string, enabled bool, limits map[string]any) error {
	if !enabled {
		return nil
	}
	switch moduleCode {
	case moduleImageGeneration:
		if err := validatePositiveCapabilityMaximum(limits, "n", 1); err != nil {
			return err
		}
		return validateCapabilityAllowedValues(limits, "quality", []string{"standard", "high"})
	case moduleVideoGeneration:
		if err := validatePositiveCapabilityMaximum(limits, "duration", 4); err != nil {
			return err
		}
		return validateCapabilityAllowedValues(limits, "resolution", []string{"480p", "720p", "1080p", "4k"})
	case modulePPTGeneration:
		return validatePositiveCapabilityMaximum(limits, "page_count", 1)
	default:
		return nil
	}
}

func validatePositiveCapabilityMaximum(limits map[string]any, key string, minimum float64) error {
	rule, ok := mapValue(limits[key])
	if !ok {
		return nil
	}
	maximum, ok := anyToFloat(rule["max"])
	if !ok {
		return fmt.Errorf("capability limit %s.max must be a number", key)
	}
	if maximum < minimum {
		return fmt.Errorf("capability limit %s.max must be >= %s", key, formatFloat(minimum))
	}
	return nil
}

func validateCapabilityAllowedValues(limits map[string]any, key string, supported []string) error {
	rule, ok := mapValue(limits[key])
	if !ok {
		return nil
	}
	values, ok := anySlice(rule["allowed"])
	if !ok {
		return fmt.Errorf("capability limit %s.allowed must be an array", key)
	}
	if len(values) == 0 {
		return fmt.Errorf("capability limit %s.allowed requires at least one value", key)
	}
	for _, value := range values {
		candidate := strings.TrimSpace(fmt.Sprint(value))
		if !stringListContains(supported, candidate) {
			return fmt.Errorf("unsupported capability limit %s value: %s", key, candidate)
		}
	}
	return nil
}

func availableModelsForModule(data adminPlatformData, module adminAIModule) []string {
	items := append([]string{}, module.BoundModels...)
	for _, model := range data.AIModels {
		if canonicalModuleCode(model.ModuleCode) == canonicalModuleCode(module.ModuleCode) && isActiveLike(model.Status) {
			items = append(items, model.ModelName)
		}
	}
	return uniqueStringList(items)
}

func allowedModelsFromLimits(limits map[string]any) []string {
	models, ok := mapValue(limits["models"])
	if !ok {
		return nil
	}
	values, ok := anySlice(models["allowed"])
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if item := strings.TrimSpace(fmt.Sprint(value)); item != "" {
			result = append(result, item)
		}
	}
	return uniqueStringList(result)
}

func exactPackageModuleLimitIndex(items []adminTenantModuleLimit, planID string, moduleCode string) int {
	for index, item := range items {
		if firstNonEmptyString(item.PackageID, item.PackageIDCamel) == planID &&
			canonicalModuleCode(firstNonEmptyString(item.ModuleCode, item.ModuleCodeCamel)) == moduleCode &&
			firstNonEmptyString(item.AgentID, item.AgentIDCamel) == "" &&
			firstNonEmptyString(item.TenantID, item.TenantIDCamel, "default") == "default" {
			return index
		}
	}
	return -1
}

func planStringListContains(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func uniqueStringList(items []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		key := strings.ToLower(item)
		if item == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func setStringListMembership(items []string, target string, enabled bool) []string {
	result := []string{}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), target) {
			continue
		}
		result = append(result, item)
	}
	if enabled {
		result = append(result, target)
	}
	return uniqueStringList(result)
}

func stringsToAny(items []string) []any {
	result := make([]any, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	return result
}
