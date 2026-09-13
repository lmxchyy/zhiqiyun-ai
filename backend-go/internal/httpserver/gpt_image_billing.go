package httpserver

import (
	"fmt"
	"strings"

	imageprovider "xianzhi-ai/backend-go/internal/provider/image"
)

// canonicalizeGPTImageBillingMultipliers ensures GPT Image parameter rules have
// normalized tier-based size multipliers (tier_1k=1.0, tier_2k=1.5, tier_4k=2.0)
// and compatible quality rules (low=1.0, normal/medium=1.2, high=1.5).
func canonicalizeGPTImageBillingMultipliers(multipliers map[string]any) map[string]any {
	result := cloneAnyMap(multipliers)
	if result == nil {
		result = map[string]any{}
	}

	// 1. Normalize size multipliers to tier-based structure
	rawSize, _ := mapValue(result["size"])
	sizeRules := cloneAnyMap(rawSize)
	if sizeRules == nil {
		sizeRules = map[string]any{}
	}

	standardTiers := []struct {
		key  string
		mult float64
	}{
		{imageprovider.ImageBillingSizeAuto, 1.0},
		{imageprovider.ImageBillingSizeTier720, 1.0},
		{imageprovider.ImageBillingSizeTier1K, 1.0},
		{imageprovider.ImageBillingSizeTier2K, 1.5},
		{imageprovider.ImageBillingSizeTier4K, 2.0},
	}
	for _, st := range standardTiers {
		if _, ok := anyToFloat(sizeRules[st.key]); !ok {
			sizeRules[st.key] = st.mult
		}
	}
	// Align legacy size aliases so 1024x1536 is not 1.2
	for key, mult := range map[string]float64{
		"1024x1024": 1.0,
		"1024x1536": 1.0,
		"1536x1024": 1.0,
		"1280x720":  1.0,
		"720x1280":  1.0,
		"2048x2048": 1.5,
		"2048x1152": 1.5,
		"3840x2160": 2.0,
		"2160x3840": 2.0,
	} {
		if _, ok := anyToFloat(sizeRules[key]); !ok || key == "1024x1536" || key == "1536x1024" {
			sizeRules[key] = mult
		}
	}
	result["size"] = sizeRules

	// 2. Normalize quality multipliers
	rawQuality, _ := mapValue(result["quality"])
	qualityRules := cloneAnyMap(rawQuality)
	if qualityRules == nil {
		qualityRules = map[string]any{}
	}
	if _, ok := anyToFloat(qualityRules["low"]); !ok {
		qualityRules["low"] = 1.0
	}
	mediumMult := 1.2
	if m, ok := anyToFloat(qualityRules["medium"]); ok && m > 0 {
		mediumMult = m
	} else {
		qualityRules["medium"] = mediumMult
	}
	if _, ok := anyToFloat(qualityRules["normal"]); !ok {
		qualityRules["normal"] = mediumMult
	}
	if _, ok := anyToFloat(qualityRules["high"]); !ok {
		qualityRules["high"] = 1.5
	}
	if _, ok := anyToFloat(qualityRules["auto"]); !ok {
		qualityRules["auto"] = 1.0
	}
	if _, ok := anyToFloat(qualityRules["standard"]); !ok {
		qualityRules["standard"] = 1.0
	}
	result["quality"] = qualityRules

	return result
}

func gptImageBillingSizeLookupKey(rawSize any, sizeRules map[string]any) string {
	if len(sizeRules) == 0 {
		return strings.TrimSpace(fmt.Sprint(rawSize))
	}
	tier, err := imageprovider.NormalizeImageBillingSizeTier(rawSize)
	if err != nil {
		return strings.TrimSpace(fmt.Sprint(rawSize))
	}
	// 1. Primary: match tier directly from billing rule
	if _, ok := anyToFloat(sizeRules[tier]); ok {
		return tier
	}
	// 2. Compatibility fallback for legacy size rules missing tier keys:
	// Find any legacy rule key belonging to this tier.
	if key, ok := highestSizeRuleKeyForTier(sizeRules, tier); ok {
		return key
	}
	// 3. Fallback: return tier directly. Never fall back across tiers to 1024x1536 / 1.2!
	return tier
}

func parsePublishedImageSize(size string) (int, int, bool) {
	tier, err := imageprovider.NormalizeImageBillingSizeTier(size)
	if err != nil || tier == imageprovider.ImageBillingSizeAuto {
		return 0, 0, false
	}
	raw := strings.ToLower(strings.TrimSpace(size))
	parts := strings.Split(raw, "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	var width, height int
	if _, err := fmt.Sscanf(parts[0], "%d", &width); err != nil || width <= 0 {
		return 0, 0, false
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &height); err != nil || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func sizeRuleTier(key string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case imageprovider.ImageBillingSizeAuto, imageprovider.ImageBillingSizeTier720, imageprovider.ImageBillingSizeTier1K, imageprovider.ImageBillingSizeTier2K, imageprovider.ImageBillingSizeTier4K:
		return strings.ToLower(strings.TrimSpace(key)), true
	}
	tier, err := imageprovider.NormalizeImageBillingSizeTier(key)
	if err != nil {
		return "", false
	}
	return tier, true
}

func highestSizeRuleKeyForTier(sizeRules map[string]any, tier string) (string, bool) {
	bestKey := ""
	best := 0.0
	found := false
	for key, raw := range sizeRules {
		ratio, ok := anyToFloat(raw)
		if !ok || ratio <= 0 {
			continue
		}
		keyTier, ok := sizeRuleTier(fmt.Sprint(key))
		if !ok || keyTier != tier {
			continue
		}
		if !found || ratio > best || (ratio == best && fmt.Sprint(key) > bestKey) {
			bestKey = fmt.Sprint(key)
			best = ratio
			found = true
		}
	}
	return bestKey, found
}

func billingParamsForRequest(model string, params map[string]any, multipliers map[string]any) map[string]any {
	if !isGPTImage2SchemaModel(model) {
		return params
	}
	canonicalMultipliers := canonicalizeGPTImageBillingMultipliers(multipliers)
	sizeRules, ok := mapValue(canonicalMultipliers["size"])
	if !ok || len(sizeRules) == 0 {
		return params
	}
	billingParams := cloneAnyMap(params)
	billingParams["size"] = gptImageBillingSizeLookupKey(firstPresent(billingParams, "size"), sizeRules)
	if q := firstPresent(billingParams, "quality"); hasNonEmptyValue(q) {
		if mapped, ok := canonicalGPTImageQualityValue(q); ok {
			billingParams["quality"] = mapped
		}
	}
	return billingParams
}
