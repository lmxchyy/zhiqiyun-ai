package httpserver

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	providerCostIssueMissing   = "PROVIDER_COST_MISSING"
	providerCostIssueExpired   = "PROVIDER_COST_EXPIRED"
	providerCostIssueAmbiguous = "PROVIDER_COST_AMBIGUOUS"
	providerCostIssueInvalid   = "PROVIDER_COST_INVALID"
	marginIssueInvalid         = "MARGIN_CALCULATION_INVALID"
	marginIssueNegative        = "NEGATIVE_MARGIN"
	marginIssueBelowTarget     = "MARGIN_BELOW_TARGET"
)

type providerCostSelection struct {
	Cost  providerCost
	Found bool
	Issue string
}

func providerCostForTask(costs []providerCost, task generationTask) (providerCost, bool) {
	selection := selectProviderCost(costs, task, time.Now().UTC())
	return selection.Cost, selection.Found && selection.Issue == ""
}

func selectProviderCost(costs []providerCost, task generationTask, now time.Time) providerCostSelection {
	type candidate struct {
		cost        providerCost
		specificity int
	}
	candidates := make([]candidate, 0)
	seenModel := false
	seenExpired := false
	for _, cost := range costs {
		if upperTrim(cost.Status) != "ACTIVE" || !strings.EqualFold(cost.PlatformModelCode, task.Model) {
			continue
		}
		seenModel = true
		if !providerCostEffective(cost, now) {
			seenExpired = true
			continue
		}
		if task.ProviderChannel != "" && !strings.EqualFold(cost.Channel, task.ProviderChannel) {
			continue
		}
		matched, specificity := providerCostParamsMatch(cost.ParameterRange, task.Params)
		if matched {
			candidates = append(candidates, candidate{cost: cost, specificity: specificity})
		}
	}
	if len(candidates) == 0 {
		if seenExpired {
			return providerCostSelection{Issue: providerCostIssueExpired}
		}
		if seenModel {
			return providerCostSelection{Issue: providerCostIssueInvalid}
		}
		return providerCostSelection{Issue: providerCostIssueMissing}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].specificity != candidates[j].specificity {
			return candidates[i].specificity > candidates[j].specificity
		}
		if candidates[i].cost.EffectiveFrom != candidates[j].cost.EffectiveFrom {
			return candidates[i].cost.EffectiveFrom > candidates[j].cost.EffectiveFrom
		}
		if candidates[i].cost.CreatedAt != candidates[j].cost.CreatedAt {
			return candidates[i].cost.CreatedAt > candidates[j].cost.CreatedAt
		}
		return candidates[i].cost.ID < candidates[j].cost.ID
	})
	top := candidates[0]
	if len(candidates) > 1 && candidates[1].specificity == top.specificity && candidates[1].cost.EffectiveFrom == top.cost.EffectiveFrom && candidates[1].cost.CreatedAt == top.cost.CreatedAt {
		return providerCostSelection{Cost: top.cost, Found: true, Issue: providerCostIssueAmbiguous}
	}
	return providerCostSelection{Cost: top.cost, Found: true}
}

func providerCostEffective(cost providerCost, now time.Time) bool {
	from, err := time.Parse(time.RFC3339Nano, cost.EffectiveFrom)
	if err != nil || now.Before(from) {
		return false
	}
	if strings.TrimSpace(cost.EffectiveTo) == "" {
		return true
	}
	to, err := time.Parse(time.RFC3339Nano, cost.EffectiveTo)
	return err == nil && now.Before(to)
}

func providerCostParamsMatch(ranges map[string]any, params map[string]any) (bool, int) {
	specificity := 0
	for key, raw := range ranges {
		actual := firstPresent(params, key)
		if actual == nil {
			return false, 0
		}
		if options, ok := anySlice(raw); ok {
			matched := false
			for _, value := range options {
				if strings.EqualFold(fmt.Sprint(value), fmt.Sprint(actual)) {
					matched = true
					break
				}
			}
			if !matched {
				return false, 0
			}
			specificity++
			continue
		}
		if bounds, ok := mapValue(raw); ok {
			if exact, exists := bounds["value"]; exists && !strings.EqualFold(fmt.Sprint(exact), fmt.Sprint(actual)) {
				return false, 0
			}
			if min, exists := anyToFloat(bounds["min"]); exists {
				value, ok := anyToFloat(actual)
				if !ok || value < min {
					return false, 0
				}
			}
			if max, exists := anyToFloat(bounds["max"]); exists {
				value, ok := anyToFloat(actual)
				if !ok || value > max {
					return false, 0
				}
			}
			specificity++
			continue
		}
		if !strings.EqualFold(fmt.Sprint(raw), fmt.Sprint(actual)) {
			return false, 0
		}
		specificity++
	}
	return true, specificity
}

func providerCostCents(cost providerCost, task generationTask) (int64, bool) {
	if !strings.EqualFold(strings.TrimSpace(cost.Currency), "CNY") || cost.UnitCost < 0 {
		return 0, false
	}
	quantity := billingQuantity(legacyBillingType(cost.BillingUnit), createGenerationTaskRequest{Type: task.Type, Model: task.Model, Params: task.Params})
	return int64(mathRound(cost.UnitCost * quantity * 100)), true
}

func mathRound(value float64) float64 {
	if value >= 0 {
		return float64(int64(value + 0.5))
	}
	return float64(int64(value - 0.5))
}

type marginHealthStatus string

const (
	marginHealthHealthy marginHealthStatus = "HEALTHY"
	marginHealthWarning marginHealthStatus = "WARNING"
	marginHealthBlocked marginHealthStatus = "BLOCKED"
)

func assessMarginHealth(userChargeCents, supplierCostCents int64, targetRate *float64) (marginHealthStatus, string, int64, float64, bool) {
	if userChargeCents <= 0 || supplierCostCents < 0 {
		return marginHealthBlocked, marginIssueInvalid, 0, 0, false
	}
	marginCents := userChargeCents - supplierCostCents
	marginRate := float64(marginCents) / float64(userChargeCents)
	if marginCents < 0 {
		return marginHealthBlocked, marginIssueNegative, marginCents, marginRate, true
	}
	if targetRate != nil && marginRate < *targetRate {
		return marginHealthWarning, marginIssueBelowTarget, marginCents, marginRate, true
	}
	return marginHealthHealthy, "", marginCents, marginRate, true
}
