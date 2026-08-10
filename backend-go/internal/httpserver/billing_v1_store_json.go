package httpserver

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

func normalizeBillingV1Defaults(data adminPlatformData) adminPlatformData {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if len(data.BillingRuleVersions) == 0 {
		data.BillingRuleVersions = make([]billingRuleVersion, 0, len(data.BillingRules))
		for _, rule := range data.BillingRules {
			data.BillingRuleVersions = append(data.BillingRuleVersions, billingRuleVersion{
				ID:               "brv_" + safeID(rule.ID) + "_v1",
				RuleKey:          rule.ID,
				LegacyRuleID:     rule.ID,
				ModelName:        rule.ModelName,
				ModelCode:        rule.ModelName,
				ModuleCode:       rule.ModuleCode,
				BillingUnit:      billingUnitFromLegacy(rule.BillingType),
				BasePrice:        rule.BasePrice,
				MinimumCharge:    math.Max(1, rule.MinimumCharge),
				ParameterRules:   cloneAnyMap(rule.ParameterMultiplier),
				RuleSource:       "CODE_DEFAULT",
				Version:          1,
				Status:           "PUBLISHED",
				EffectiveFrom:    firstNonEmptyString(rule.CreatedAt, now),
				CreatedAt:        firstNonEmptyString(rule.CreatedAt, now),
				UpdatedAt:        firstNonEmptyString(rule.UpdatedAt, now),
				PublishedAt:      firstNonEmptyString(rule.UpdatedAt, now),
				ValidationResult: billingRuleValidationResult{Valid: true, ValidatedAt: now, Issues: []billingRuleValidationIssue{}},
			})
		}
	}
	if len(data.ProviderCosts) == 0 {
		data.ProviderCosts = defaultProviderCosts(now)
	}
	if data.BillingLifecycleEvents == nil {
		data.BillingLifecycleEvents = []billingLifecycleEvent{}
	}
	if data.WalletLedger == nil {
		data.WalletLedger = []walletLedgerEntry{}
	}
	return data
}

func defaultProviderCosts(now string) []providerCost {
	return []providerCost{
		{ID: "pcost_openai_gpt_image_2", Provider: "OPENAI", Channel: "channel_openai", PlatformModelCode: "gpt-image-2", UpstreamModelName: "gpt-image-2", BillingUnit: "PER_IMAGE", ParameterRange: map[string]any{"quality": []any{"standard", "high"}}, UnitCost: 0.6, Currency: "CNY", EffectiveFrom: now, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{ID: "pcost_newapi_grok_imagine_15_video", Provider: "NEWAPI", Channel: "channel_runtime_env", PlatformModelCode: "grok-imagine-1.5-video", UpstreamModelName: "grok-imagine-1.5-video", BillingUnit: "PER_SECOND", ParameterRange: map[string]any{"duration": map[string]any{"min": float64(6), "max": float64(30)}, "resolution": []any{"480p", "720p"}}, UnitCost: 0.13, Currency: "CNY", EffectiveFrom: now, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{ID: "pcost_seedance_fast_720", Provider: "CME_CLOUD", Channel: "channel_cmecloud_seedance", PlatformModelCode: "seedance-fast-2.0", UpstreamModelName: "seedance-fast-2.0", BillingUnit: "PER_SECOND", ParameterRange: map[string]any{"resolution": []any{"720p"}}, UnitCost: 0.8, Currency: "CNY", EffectiveFrom: now, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{ID: "pcost_doubao_seedance_720", Provider: "CME_CLOUD", Channel: "channel_cmecloud_seedance", PlatformModelCode: "doubao-seedance-2.0", UpstreamModelName: "doubao-seedance-2.0", BillingUnit: "PER_SECOND", ParameterRange: map[string]any{"resolution": []any{"720p"}}, UnitCost: 0.8, Currency: "CNY", EffectiveFrom: now, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
	}
}

func billingUnitFromLegacy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "per_image":
		return "PER_IMAGE"
	case "per_second":
		return "PER_SECOND"
	case "per_page":
		return "PER_PAGE"
	case "per_token":
		return "PER_TOKEN"
	case "per_1k_tokens":
		return "PER_1K_TOKENS"
	default:
		return "PER_REQUEST"
	}
}

func legacyBillingType(value string) string {
	switch upperTrim(value) {
	case "PER_IMAGE":
		return "per_image"
	case "PER_SECOND":
		return "per_second"
	case "PER_PAGE":
		return "per_page"
	case "PER_TOKEN":
		return "per_token"
	case "PER_1K_TOKENS":
		return "per_1k_tokens"
	default:
		return "per_request"
	}
}

func billingRuleVersionProjection(item billingRuleVersion) adminBillingRule {
	projectionStatus := item.Status
	if upperTrim(item.Status) == "PUBLISHED" {
		projectionStatus = "ACTIVE"
	}
	return normalizeBillingRuleAliases(adminBillingRule{
		ID:                  item.ID,
		ModuleCode:          item.ModuleCode,
		ModelName:           item.ModelCode,
		ModelCode:           item.ModelCode,
		BillingType:         legacyBillingType(item.BillingUnit),
		BillingUnit:         item.BillingUnit,
		BasePrice:           item.BasePrice,
		MinimumCharge:       item.MinimumCharge,
		CurrencyType:        "credit",
		ParameterMultiplier: cloneAnyMap(item.ParameterRules),
		Status:              projectionStatus,
		RuleSource:          item.RuleSource,
		Version:             item.Version,
		CreatedAt:           item.CreatedAt,
		UpdatedAt:           item.UpdatedAt,
	})
}

func applyPublishedBillingRulesV1(data adminPlatformData) adminPlatformData {
	data = normalizeBillingV1Defaults(data)
	published := make([]billingRuleVersion, 0, len(data.BillingRuleVersions))
	for _, item := range data.BillingRuleVersions {
		if upperTrim(item.Status) == "PUBLISHED" && billingRuleVersionEffective(item, time.Now().UTC()) {
			published = append(published, item)
		}
	}
	sort.SliceStable(published, func(i, j int) bool {
		left, right := billingRuleSourcePriority(published[i].RuleSource), billingRuleSourcePriority(published[j].RuleSource)
		if left != right {
			return left > right
		}
		return published[i].Version > published[j].Version
	})
	for _, version := range published {
		index := -1
		for i, rule := range data.BillingRules {
			if canonicalModuleCode(rule.ModuleCode) == canonicalModuleCode(version.ModuleCode) && strings.EqualFold(strings.TrimSpace(rule.ModelName), strings.TrimSpace(version.ModelCode)) {
				index = i
				break
			}
		}
		projection := billingRuleVersionProjection(version)
		if index >= 0 {
			data.BillingRules[index] = projection
		} else {
			data.BillingRules = append(data.BillingRules, projection)
		}
	}
	return data
}

func billingRuleSourcePriority(source string) int {
	switch upperTrim(source) {
	case "TENANT_OVERRIDE":
		return 4
	case "PLAN_OVERRIDE":
		return 3
	case "DATABASE":
		return 2
	default:
		return 1
	}
}

func billingRuleVersionEffective(item billingRuleVersion, now time.Time) bool {
	if item.EffectiveFrom != "" {
		if value, err := time.Parse(time.RFC3339Nano, item.EffectiveFrom); err == nil && value.After(now) {
			return false
		}
	}
	if item.EffectiveTo != "" {
		if value, err := time.Parse(time.RFC3339Nano, item.EffectiveTo); err == nil && !value.After(now) {
			return false
		}
	}
	return true
}

func (s *jsonStore) ListBillingRuleVersions() ([]billingRuleVersion, error) {
	data, err := s.AdminData()
	if err != nil {
		return nil, err
	}
	items := append([]billingRuleVersion(nil), normalizeBillingV1Defaults(data).BillingRuleVersions...)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].RuleKey != items[j].RuleKey {
			return items[i].RuleKey < items[j].RuleKey
		}
		return items[i].Version > items[j].Version
	})
	return items, nil
}

func createBillingRuleDraftInData(data *adminPlatformData, id string, req adminBillingRuleMutation) (billingRuleVersion, error) {
	if data == nil {
		return billingRuleVersion{}, errors.New("billing data is required")
	}
	*data = normalizeBillingV1Defaults(normalizeAICapabilityDefaults(*data))
	sourceIndex := -1
	for i := range data.BillingRuleVersions {
		item := data.BillingRuleVersions[i]
		if item.ID == id || item.RuleKey == id || item.LegacyRuleID == id {
			if sourceIndex < 0 || item.Version > data.BillingRuleVersions[sourceIndex].Version {
				sourceIndex = i
			}
		}
	}
	if sourceIndex < 0 {
		return billingRuleVersion{}, fmt.Errorf("billing rule not found: %s", id)
	}
	source := data.BillingRuleVersions[sourceIndex]
	nextVersion := source.Version + 1
	for _, item := range data.BillingRuleVersions {
		if item.RuleKey == source.RuleKey && item.Version >= nextVersion {
			nextVersion = item.Version + 1
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	draft := source
	draft.ID = fmt.Sprintf("brv_%s_v%d", safeID(source.RuleKey), nextVersion)
	draft.Version = nextVersion
	draft.Status = "DRAFT"
	draft.RuleSource = firstNonEmptyString(source.RuleSource, "DATABASE")
	if draft.RuleSource == "CODE_DEFAULT" {
		draft.RuleSource = "DATABASE"
	}
	draft.EffectiveFrom = ""
	draft.EffectiveTo = ""
	draft.PublishedAt = ""
	draft.CreatedAt = now
	draft.UpdatedAt = now
	draft.ValidationResult = billingRuleValidationResult{Valid: false, Issues: []billingRuleValidationIssue{}}
	if req.BillingType != "" {
		draft.BillingUnit = billingUnitFromLegacy(req.BillingType)
	}
	if req.BasePrice > 0 {
		draft.BasePrice = req.BasePrice
	}
	if req.MinimumCharge >= 0 {
		draft.MinimumCharge = req.MinimumCharge
	}
	if req.ParameterMultiplier != nil {
		draft.ParameterRules = cloneAnyMap(req.ParameterMultiplier)
	}
	data.BillingRuleVersions = append(data.BillingRuleVersions, draft)
	return draft, nil
}

func (s *jsonStore) GetBillingRuleVersion(id string) (billingRuleVersion, error) {
	items, err := s.ListBillingRuleVersions()
	if err != nil {
		return billingRuleVersion{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return billingRuleVersion{}, errors.New("billing rule version not found")
}

func (s *jsonStore) ValidateBillingRuleVersion(id string) (billingRuleValidationResult, error) {
	var result billingRuleValidationResult
	err := s.updateAdmin(func(data *adminPlatformData) error {
		*data = normalizeBillingV1Defaults(normalizeAICapabilityDefaults(*data))
		for i := range data.BillingRuleVersions {
			if data.BillingRuleVersions[i].ID != id {
				continue
			}
			result = validateBillingRuleVersionData(data.BillingRuleVersions[i], *data)
			data.BillingRuleVersions[i].ValidationResult = result
			data.BillingRuleVersions[i].UpdatedAt = result.ValidatedAt
			return nil
		}
		return errors.New("billing rule version not found")
	})
	return result, err
}

func (s *jsonStore) PublishBillingRuleVersion(id string) (billingRuleVersion, error) {
	var published billingRuleVersion
	err := s.updateAdmin(func(data *adminPlatformData) error {
		*data = normalizeBillingV1Defaults(normalizeAICapabilityDefaults(*data))
		index := -1
		for i := range data.BillingRuleVersions {
			if data.BillingRuleVersions[i].ID == id {
				index = i
				break
			}
		}
		if index < 0 {
			return errors.New("billing rule version not found")
		}
		result := validateBillingRuleVersionData(data.BillingRuleVersions[index], *data)
		if !result.Valid {
			data.BillingRuleVersions[index].ValidationResult = result
			return errors.New("billing rule validation failed")
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for i := range data.BillingRuleVersions {
			if i != index && data.BillingRuleVersions[i].RuleKey == data.BillingRuleVersions[index].RuleKey && upperTrim(data.BillingRuleVersions[i].Status) == "PUBLISHED" {
				data.BillingRuleVersions[i].Status = "ARCHIVED"
				data.BillingRuleVersions[i].EffectiveTo = now
				data.BillingRuleVersions[i].UpdatedAt = now
			}
		}
		data.BillingRuleVersions[index].Status = "PUBLISHED"
		data.BillingRuleVersions[index].PublishedAt = now
		data.BillingRuleVersions[index].UpdatedAt = now
		data.BillingRuleVersions[index].ValidationResult = result
		if data.BillingRuleVersions[index].EffectiveFrom == "" {
			data.BillingRuleVersions[index].EffectiveFrom = now
		}
		published = data.BillingRuleVersions[index]
		return nil
	})
	return published, err
}

func (s *jsonStore) ListProviderCosts() ([]providerCost, error) {
	data, err := s.AdminData()
	if err != nil {
		return nil, err
	}
	items := append([]providerCost(nil), normalizeBillingV1Defaults(data).ProviderCosts...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	return items, nil
}

func (s *jsonStore) UpdateProviderCost(id string, req providerCostMutation) (providerCost, error) {
	var updated providerCost
	err := s.updateAdmin(func(data *adminPlatformData) error {
		*data = normalizeBillingV1Defaults(*data)
		for i := range data.ProviderCosts {
			if data.ProviderCosts[i].ID != id {
				continue
			}
			item := data.ProviderCosts[i]
			applyProviderCostMutation(&item, req)
			if err := validateProviderCost(item); err != nil {
				return err
			}
			item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			data.ProviderCosts[i] = item
			updated = item
			return nil
		}
		return errors.New("provider cost not found")
	})
	return updated, err
}

func (s *jsonStore) ListBillingLifecycleEvents() ([]billingLifecycleEvent, error) {
	data, err := s.AdminData()
	if err != nil {
		return nil, err
	}
	items := append([]billingLifecycleEvent(nil), data.BillingLifecycleEvents...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
	return items, nil
}

func (s *jsonStore) ListWalletLedger() ([]walletLedgerEntry, error) {
	data, err := s.AdminData()
	if err != nil {
		return nil, err
	}
	items := append([]walletLedgerEntry(nil), data.WalletLedger...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
	return items, nil
}

func (s *jsonStore) ListBillingReconciliation() ([]billingReconciliationItem, error) {
	data, err := s.AdminData()
	if err != nil {
		return nil, err
	}
	return buildBillingReconciliation(data.GenerationTasks, data.BillingLifecycleEvents, data.WalletLedger), nil
}

func validateBillingRuleVersionData(item billingRuleVersion, data adminPlatformData) billingRuleValidationResult {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	issues := []billingRuleValidationIssue{}
	add := func(code, field, severity, message string) {
		issues = append(issues, billingRuleValidationIssue{Code: code, Field: field, Severity: severity, Message: message})
	}
	if item.BasePrice <= 0 {
		add("INVALID_BASE_PRICE", "basePrice", "ERROR", "基础售价必须大于 0")
	}
	if !validBillingUnit(item.BillingUnit) {
		add("INVALID_BILLING_UNIT", "billingUnit", "ERROR", "计费单位无效")
	}
	if item.MinimumCharge < 0 {
		add("INVALID_MINIMUM_CHARGE", "minimumCharge", "ERROR", "最低扣费不能小于 0")
	}
	if !parameterPricingComplete(item.ParameterRules) {
		add("INCOMPLETE_PARAMETER_PRICING", "parameterRules", "ERROR", "参数定价包含空值、非数字或非正数")
	}
	modelExists := false
	for _, model := range data.AIModels {
		if strings.EqualFold(model.ModelName, item.ModelCode) && canonicalModuleCode(model.ModuleCode) == canonicalModuleCode(item.ModuleCode) {
			modelExists = true
			break
		}
	}
	if !modelExists {
		add("MODEL_CODE_MISMATCH", "modelCode", "ERROR", "模型编码与所属模块不一致")
	}
	matchingCosts := []providerCost{}
	for _, cost := range data.ProviderCosts {
		if upperTrim(cost.Status) == "ACTIVE" && strings.EqualFold(cost.PlatformModelCode, item.ModelCode) {
			matchingCosts = append(matchingCosts, cost)
		}
	}
	if len(matchingCosts) == 0 {
		add("MISSING_PROVIDER_COST", "modelCode", "ERROR", "缺少生效中的供应商成本")
	} else {
		minimumRevenue := math.Max(item.BasePrice, item.MinimumCharge) * float64(pointUnitAmountCents) / 100
		for _, cost := range matchingCosts {
			if upperTrim(cost.BillingUnit) == upperTrim(item.BillingUnit) && cost.UnitCost > minimumRevenue {
				add("NEGATIVE_MARGIN", "basePrice", "ERROR", fmt.Sprintf("单位售价折算 %.2f CNY，低于供应商成本 %.2f CNY", minimumRevenue, cost.UnitCost))
				break
			}
		}
	}
	for _, other := range data.BillingRuleVersions {
		if item.EffectiveFrom == "" || other.ID == item.ID || other.RuleKey != item.RuleKey || upperTrim(other.Status) != "PUBLISHED" {
			continue
		}
		if effectiveRangesOverlap(item.EffectiveFrom, item.EffectiveTo, other.EffectiveFrom, other.EffectiveTo) {
			add("EFFECTIVE_TIME_CONFLICT", "effectiveFrom", "ERROR", "生效时间与现有正式版本冲突")
			break
		}
	}
	if upperTrim(item.RuleSource) == "CODE_DEFAULT" {
		dependencyFound := false
		for _, legacy := range data.BillingRules {
			if legacy.ID == item.LegacyRuleID || (canonicalModuleCode(legacy.ModuleCode) == canonicalModuleCode(item.ModuleCode) && strings.EqualFold(legacy.ModelName, item.ModelCode)) {
				dependencyFound = true
				break
			}
		}
		if !dependencyFound {
			add("MISSING_CODE_DEFAULT", "ruleSource", "ERROR", "代码默认规则依赖不存在")
		}
	}
	channelByID := map[string]adminAPIChannel{}
	for _, channel := range data.APIChannels {
		channelByID[channel.ID] = channel
	}
	for _, cost := range matchingCosts {
		channel, ok := channelByID[cost.Channel]
		if !ok || upperTrim(channel.Status) == "INACTIVE" || upperTrim(channel.Status) == "DISABLED" {
			add("INVALID_PROVIDER_CHANNEL", "providerChannel", "ERROR", "供应商成本绑定了无效通道："+cost.Channel)
		}
	}
	return billingRuleValidationResult{Valid: !hasBillingValidationErrors(issues), ValidatedAt: now, Issues: issues}
}

func hasBillingValidationErrors(issues []billingRuleValidationIssue) bool {
	for _, issue := range issues {
		if upperTrim(issue.Severity) == "ERROR" {
			return true
		}
	}
	return false
}

func validBillingUnit(value string) bool {
	switch upperTrim(value) {
	case "PER_REQUEST", "PER_IMAGE", "PER_SECOND", "PER_PAGE", "PER_TOKEN", "PER_1K_TOKENS":
		return true
	default:
		return false
	}
}

func parameterPricingComplete(rules map[string]any) bool {
	for _, raw := range rules {
		options, ok := mapValue(raw)
		if !ok || len(options) == 0 {
			return false
		}
		for _, value := range options {
			number, ok := anyToFloat(value)
			if !ok || number <= 0 {
				return false
			}
		}
	}
	return true
}

func effectiveRangesOverlap(leftFrom, leftTo, rightFrom, rightTo string) bool {
	parse := func(value string, fallback time.Time) time.Time {
		if value == "" {
			return fallback
		}
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed
		}
		return fallback
	}
	min := time.Unix(0, 0).UTC()
	max := time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	return parse(leftFrom, min).Before(parse(rightTo, max)) && parse(rightFrom, min).Before(parse(leftTo, max))
}

func applyProviderCostMutation(item *providerCost, req providerCostMutation) {
	if item == nil {
		return
	}
	if req.Provider != "" {
		item.Provider = strings.TrimSpace(req.Provider)
	}
	if req.Channel != "" {
		item.Channel = strings.TrimSpace(req.Channel)
	}
	if req.PlatformModelCode != "" {
		item.PlatformModelCode = strings.TrimSpace(req.PlatformModelCode)
	}
	if req.UpstreamModelName != "" {
		item.UpstreamModelName = strings.TrimSpace(req.UpstreamModelName)
	}
	if req.BillingUnit != "" {
		item.BillingUnit = upperTrim(req.BillingUnit)
	}
	if req.ParameterRange != nil {
		item.ParameterRange = cloneAnyMap(req.ParameterRange)
	}
	if req.UnitCost != nil {
		item.UnitCost = *req.UnitCost
	}
	if req.Currency != "" {
		item.Currency = upperTrim(req.Currency)
	}
	if req.EffectiveFrom != "" {
		item.EffectiveFrom = req.EffectiveFrom
	}
	if req.EffectiveTo != "" {
		item.EffectiveTo = req.EffectiveTo
	}
	if req.Status != "" {
		item.Status = upperTrim(req.Status)
	}
}

func validateProviderCost(item providerCost) error {
	if strings.TrimSpace(item.Provider) == "" || strings.TrimSpace(item.Channel) == "" || strings.TrimSpace(item.PlatformModelCode) == "" || strings.TrimSpace(item.UpstreamModelName) == "" {
		return errors.New("provider, channel, platformModelCode and upstreamModelName are required")
	}
	if !validBillingUnit(item.BillingUnit) {
		return errors.New("invalid billing unit")
	}
	if item.UnitCost < 0 {
		return errors.New("unit cost cannot be negative")
	}
	if upperTrim(item.Status) != "ACTIVE" && upperTrim(item.Status) != "INACTIVE" {
		return errors.New("invalid provider cost status")
	}
	return nil
}

func buildBillingReconciliation(tasks []generationTask, events []billingLifecycleEvent, ledger []walletLedgerEntry) []billingReconciliationItem {
	eventsByTask := map[string][]billingLifecycleEvent{}
	ledgerByTask := map[string][]walletLedgerEntry{}
	for _, event := range events {
		eventsByTask[event.TaskID] = append(eventsByTask[event.TaskID], event)
	}
	for _, entry := range ledger {
		ledgerByTask[entry.TaskID] = append(ledgerByTask[entry.TaskID], entry)
	}
	items := make([]billingReconciliationItem, 0, len(tasks))
	for _, task := range tasks {
		item := reconciliationItemForTask(task, eventsByTask[task.ID], ledgerByTask[task.ID])
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
	return items
}

func reconciliationItemForTask(task generationTask, events []billingLifecycleEvent, ledger []walletLedgerEntry) billingReconciliationItem {
	taskStatus := firstNonEmptyString(task.TaskStatus, canonicalTaskStatus(task.Status))
	billingStatus := firstNonEmptyString(task.BillingStatus, legacyBillingStatus(task))
	quoted := task.QuotedPoints
	if quoted == 0 {
		quoted = float64(task.PointCost)
	}
	item := billingReconciliationItem{
		TaskID: task.ID, UserID: task.UserID, TenantID: task.TenantID, ModelCode: task.Model,
		TaskStatus: taskStatus, BillingStatus: billingStatus, QuotedPoints: quoted,
		ReservedPoints: task.ReservedPoints, CapturedPoints: task.CapturedPoints,
		ReleasedPoints: task.ReleasedPoints, RefundedPoints: task.RefundedPoints,
		SupplierCost: task.SupplierCost, EstimatedMargin: task.EstimatedMargin,
		ProviderChannel: task.ProviderChannel, RuleVersionID: task.BillingRuleVersionID,
		ClientRequestID: task.ClientRequestID, BillingEventCount: len(events), WalletLedgerCount: len(ledger),
		Anomalies: []string{}, CreatedAt: task.CreatedAt,
	}
	if taskStatus == taskStatusSucceeded && billingStatus != billingStatusCaptured && billingStatus != billingStatusRefunded {
		item.Anomalies = append(item.Anomalies, "TASK_SUCCEEDED_NOT_CAPTURED")
	}
	if (taskStatus == taskStatusFailed || taskStatus == taskStatusCancelled) && item.ReservedPoints > item.ReleasedPoints && billingStatus != billingStatusRefunded {
		item.Anomalies = append(item.Anomalies, "TASK_FAILED_NOT_RELEASED")
	}
	if item.CapturedPoints > 0 && math.Abs(item.CapturedPoints-item.QuotedPoints) > 0.000001 {
		item.Anomalies = append(item.Anomalies, "CAPTURED_NOT_EQUAL_QUOTED")
	}
	if taskStatus == taskStatusSucceeded && item.SupplierCost == nil {
		item.Anomalies = append(item.Anomalies, "MISSING_PROVIDER_COST")
	}
	if item.QuotedPoints > 0 && len(events) == 0 {
		item.Anomalies = append(item.Anomalies, "MISSING_BILLING_EVENT")
	}
	if (item.ReservedPoints > 0 || item.CapturedPoints > 0 || item.ReleasedPoints > 0 || item.RefundedPoints > 0) && len(ledger) == 0 {
		item.Anomalies = append(item.Anomalies, "MISSING_WALLET_LEDGER")
	}
	captures := 0
	for _, entry := range ledger {
		if entry.EntryType == "CAPTURE" {
			captures++
		}
	}
	if captures > 1 {
		item.Anomalies = append(item.Anomalies, "DUPLICATE_CAPTURE")
	}
	if item.EstimatedMargin != nil && *item.EstimatedMargin < 0 {
		item.Anomalies = append(item.Anomalies, "NEGATIVE_MARGIN")
	}
	return item
}

func legacyBillingStatus(task generationTask) string {
	if task.CapturedPoints > 0 || upperTrim(task.Status) == "SUCCEEDED" || upperTrim(task.Status) == "COMPLETED" {
		return billingStatusCaptured
	}
	if task.ReleasedPoints > 0 || generationTaskBillingRefunded(task) {
		return billingStatusReleased
	}
	if task.ReservedPoints > 0 || generationTaskBillingReserved(task) {
		return billingStatusReserved
	}
	return billingStatusUnquoted
}

func findGenerationTaskByClientRequest(tasks []generationTask, userID, clientRequestID string) (generationTask, bool) {
	clientRequestID = strings.TrimSpace(clientRequestID)
	if clientRequestID == "" {
		return generationTask{}, false
	}
	for _, task := range tasks {
		if task.UserID == userID && task.ClientRequestID == clientRequestID {
			return task, true
		}
	}
	return generationTask{}, false
}

func appendBillingLifecycleEventJSON(data *platformData, task generationTask, eventType string, points float64, metadata map[string]any) billingLifecycleEvent {
	key := task.ID + ":" + upperTrim(eventType)
	for _, item := range data.BillingLifecycleEvents {
		if item.IdempotencyKey == key {
			return item
		}
	}
	item := billingLifecycleEvent{ID: deterministicBillingID("ble", key), TaskID: task.ID, UserID: task.UserID, TenantID: task.TenantID, ModelCode: task.Model, EventType: upperTrim(eventType), BillingStatus: billingStatusForEvent(eventType), Points: points, RuleVersionID: task.BillingRuleVersionID, ProviderChannel: task.ProviderChannel, IdempotencyKey: key, Metadata: cloneAnyMap(metadata), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	data.BillingLifecycleEvents = append(data.BillingLifecycleEvents, item)
	return item
}

func platformPointAccount(data *platformData, userID string) (adminPointAccount, int) {
	for i, item := range data.PointAccounts {
		if item.UserID == userID {
			return item, i
		}
	}
	available := pointsAvailableForUser(*data, userID)
	item := adminPointAccount{ID: uniqueAdminID("points", pointIDs(data.PointAccounts)), UserID: userID, Available: available}
	data.PointAccounts = append(data.PointAccounts, item)
	return item, len(data.PointAccounts) - 1
}

func applyJSONWalletEntry(data *platformData, task generationTask, entryType string, points int, remark string) (walletLedgerEntry, error) {
	entryType = upperTrim(entryType)
	key := task.ID + ":" + entryType
	for _, item := range data.WalletLedger {
		if item.IdempotencyKey == key {
			return item, nil
		}
	}
	account, index := platformPointAccount(data, task.UserID)
	next := account
	switch entryType {
	case "RESERVE":
		next.Available -= points
		next.Frozen += points
	case "CAPTURE":
		next.Frozen -= points
	case "RELEASE":
		next.Available += points
		next.Frozen -= points
	case "REFUND", "RECHARGE", "GRANT", "ADJUSTMENT":
		next.Available += points
	case "EXPIRE":
		next.Available -= points
	default:
		return walletLedgerEntry{}, errors.New("unsupported wallet entry type")
	}
	if next.Available < 0 || next.Frozen < 0 {
		return walletLedgerEntry{}, errors.New("wallet balance would become negative")
	}
	data.PointAccounts[index] = next
	data.PointsAvailable = &next.Available
	entry := walletLedgerEntry{ID: deterministicBillingID("wle", key), AccountID: account.ID, UserID: task.UserID, TenantID: task.TenantID, TaskID: task.ID, EntryType: entryType, Points: float64(points), AvailableBefore: float64(account.Available), AvailableAfter: float64(next.Available), FrozenBefore: float64(account.Frozen), FrozenAfter: float64(next.Frozen), IdempotencyKey: key, ReferenceType: "GENERATION_TASK", ReferenceID: task.ID, Remark: remark, Metadata: map[string]any{"modelCode": task.Model, "ruleVersionId": task.BillingRuleVersionID}, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	data.WalletLedger = append(data.WalletLedger, entry)
	return entry, nil
}

func adminPointAccountV1(data *adminPlatformData, userID string) (adminPointAccount, int) {
	for i, item := range data.PointAccounts {
		if item.UserID == userID {
			return item, i
		}
	}
	item := adminPointAccount{ID: uniqueAdminID("points", pointIDs(data.PointAccounts)), UserID: userID}
	data.PointAccounts = append(data.PointAccounts, item)
	return item, len(data.PointAccounts) - 1
}

func appendAdminWalletLedgerV1(data *adminPlatformData, account adminPointAccount, next adminPointAccount, entryType string, points int, referenceType string, referenceID string, remark string, metadata map[string]any) walletLedgerEntry {
	key := strings.Join([]string{upperTrim(referenceType), strings.TrimSpace(referenceID), upperTrim(entryType)}, ":")
	if upperTrim(referenceType) == "GENERATION_TASK" {
		key = strings.TrimSpace(referenceID) + ":" + upperTrim(entryType)
	}
	for _, item := range data.WalletLedger {
		if item.IdempotencyKey == key {
			return item
		}
	}
	item := walletLedgerEntry{
		ID: deterministicBillingID("wle", key), AccountID: account.ID, UserID: account.UserID,
		EntryType: upperTrim(entryType), Points: float64(points),
		AvailableBefore: float64(account.Available), AvailableAfter: float64(next.Available),
		FrozenBefore: float64(account.Frozen), FrozenAfter: float64(next.Frozen),
		IdempotencyKey: key, ReferenceType: referenceType, ReferenceID: referenceID,
		Remark: remark, Metadata: cloneAnyMap(metadata), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	data.WalletLedger = append(data.WalletLedger, item)
	return item
}

func setAdminPointAccountWithLedgerV1(data *adminPlatformData, userID string, available int, entryType string, referenceType string, referenceID string, remark string) error {
	if available < 0 {
		return errors.New("wallet balance cannot be negative")
	}
	account, index := adminPointAccountV1(data, userID)
	if account.Available == available {
		return nil
	}
	next := account
	next.Available = available
	points := available - account.Available
	if points < 0 {
		points = -points
	}
	appendAdminWalletLedgerV1(data, account, next, entryType, points, referenceType, referenceID, remark, map[string]any{"direction": map[bool]string{true: "INCREASE", false: "DECREASE"}[available > account.Available]})
	data.PointAccounts[index] = next
	data.PointsAvailable = &next.Available
	return nil
}

func applyAdminJSONWalletEntryV1(data *adminPlatformData, task generationTask, entryType string, points int, remark string) (walletLedgerEntry, error) {
	entryType = upperTrim(entryType)
	key := task.ID + ":" + entryType
	for _, item := range data.WalletLedger {
		if item.IdempotencyKey == key {
			return item, nil
		}
	}
	account, index := adminPointAccountV1(data, task.UserID)
	next := account
	switch entryType {
	case "RESERVE":
		next.Available -= points
		next.Frozen += points
	case "CAPTURE":
		next.Frozen -= points
	case "RELEASE":
		next.Available += points
		next.Frozen -= points
	case "REFUND", "RECHARGE", "GRANT", "ADJUSTMENT":
		next.Available += points
	case "EXPIRE":
		next.Available -= points
	default:
		return walletLedgerEntry{}, errors.New("unsupported wallet entry type")
	}
	if points < 0 || next.Available < 0 || next.Frozen < 0 {
		return walletLedgerEntry{}, errors.New("wallet balance would become negative")
	}
	entry := appendAdminWalletLedgerV1(data, account, next, entryType, points, "GENERATION_TASK", task.ID, remark, map[string]any{"modelCode": task.Model, "ruleVersionId": task.BillingRuleVersionID})
	entry.TaskID = task.ID
	entry.TenantID = task.TenantID
	for i := range data.WalletLedger {
		if data.WalletLedger[i].ID == entry.ID {
			data.WalletLedger[i] = entry
			break
		}
	}
	data.PointAccounts[index] = next
	data.PointsAvailable = &next.Available
	return entry, nil
}

func applyTaskSupplierCost(task *generationTask, costs []providerCost) {
	if task == nil {
		return
	}
	if cost, ok := providerCostForTask(costs, *task); ok {
		value := supplierCostForTask(cost, *task)
		margin := float64(task.PointCost)*float64(pointUnitAmountCents)/100 - value
		task.SupplierCost = &value
		task.EstimatedMargin = &margin
		task.ProviderChannel = firstNonEmptyString(task.ProviderChannel, cost.Channel)
	}
}
