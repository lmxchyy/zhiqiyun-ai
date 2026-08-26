package pricing

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

var ErrRuleNotFound = errors.New("pricing rule not found")

type Rule struct {
	ID             string
	ModelCode      string
	BillingUnit    string
	BasePrice      float64
	MinimumCharge  float64
	ParameterRules map[string]any
	Version        int
}

type Request struct {
	BusinessType string
	Model        string
	Parameters   map[string]any
}

type Quote struct {
	RequiredPoints       int            `json:"requiredPoints"`
	PricingRuleID        string         `json:"pricingRuleId"`
	PricingRuleVersion   int            `json:"pricingRuleVersion"`
	BillingUnit          string         `json:"billingUnit"`
	Quantity             float64        `json:"quantity"`
	Breakdown            map[string]any `json:"breakdown"`
	NormalizedParameters map[string]any `json:"normalizedParameters"`
}

func Calculate(request Request, rule Rule) (Quote, error) {
	if strings.TrimSpace(rule.ID) == "" || strings.TrimSpace(rule.ModelCode) == "" {
		return Quote{}, ErrRuleNotFound
	}
	quantity, quantityField := quantityFor(rule.BillingUnit, request.Parameters)
	multiplier, multiplierBreakdown := multiplierFor(rule.ParameterRules, request.Parameters)
	final := rule.BasePrice * quantity * multiplier
	minimum := math.Ceil(rule.MinimumCharge)
	points := int(math.Ceil(final))
	if float64(points) < minimum {
		points = int(minimum)
	}
	if points < 1 {
		points = 1
	}
	return Quote{
		RequiredPoints:     points,
		PricingRuleID:      rule.ID,
		PricingRuleVersion: rule.Version,
		BillingUnit:        strings.ToUpper(strings.TrimSpace(rule.BillingUnit)),
		Quantity:           quantity,
		Breakdown: map[string]any{
			"basePrice":     rule.BasePrice,
			"quantityField": quantityField,
			"quantity":      quantity,
			"multiplier":    multiplier,
			"parameters":    multiplierBreakdown,
			"minimumCharge": rule.MinimumCharge,
			"rounding":      "CEIL",
		},
		NormalizedParameters: cloneMap(request.Parameters),
	}, nil
}

func quantityFor(unit string, parameters map[string]any) (float64, string) {
	switch strings.ToUpper(strings.TrimSpace(unit)) {
	case "PER_SECOND":
		if value, ok := number(parameters["duration"]); ok && value > 0 {
			return value, "duration"
		}
		return 1, "duration"
	case "PER_PAGE":
		for _, key := range []string{"page_count", "slideCount", "pageCount"} {
			if value, ok := number(parameters[key]); ok && value > 0 {
				return value, key
			}
		}
		return 1, "page_count"
	case "PER_IMAGE":
		for _, key := range []string{"n", "count", "generationCount", "imageCount"} {
			if value, ok := number(parameters[key]); ok && value > 0 {
				return value, key
			}
		}
		return 1, "count"
	default:
		return 1, "request"
	}
}

func multiplierFor(rules map[string]any, parameters map[string]any) (float64, map[string]any) {
	result := 1.0
	breakdown := map[string]any{}
	for key, raw := range rules {
		options, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		value, exists := parameters[key]
		if !exists {
			for parameterKey, parameterValue := range parameters {
				if strings.EqualFold(parameterKey, key) {
					value, exists = parameterValue, true
					break
				}
			}
		}
		if !exists {
			continue
		}
		selected := fmt.Sprint(value)
		ratio, ok := number(options[selected])
		if !ok {
			for optionKey, optionValue := range options {
				if strings.EqualFold(optionKey, selected) {
					ratio, ok = number(optionValue)
					break
				}
			}
		}
		if ok && ratio > 0 {
			result *= ratio
			breakdown[key] = map[string]any{"value": value, "multiplier": ratio}
		}
	}
	return result, breakdown
}

func number(value any) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, !math.IsNaN(value) && !math.IsInf(value, 0)
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case jsonNumber:
		return float64(value), true
	default:
		return 0, false
	}
}

type jsonNumber float64

func cloneMap(input map[string]any) map[string]any {
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
