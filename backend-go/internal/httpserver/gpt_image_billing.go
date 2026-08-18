package httpserver

import (
	"fmt"
	"strings"

	imageprovider "xianzhi-ai/backend-go/internal/provider/image"
)

func gptImageBillingSizeLookupKey(rawSize any, sizeRules map[string]any) string {
	if len(sizeRules) == 0 {
		return strings.TrimSpace(fmt.Sprint(rawSize))
	}
	tier, err := imageprovider.NormalizeImageBillingSizeTier(rawSize)
	if err != nil {
		return strings.TrimSpace(fmt.Sprint(rawSize))
	}
	if _, ok := anyToFloat(sizeRules[tier]); ok {
		return tier
	}
	normalized := strings.ToLower(strings.TrimSpace(fmt.Sprint(rawSize)))
	if normalized != "" && normalized != "<nil>" {
		if _, ok := anyToFloat(sizeRules[normalized]); ok {
			return normalized
		}
		wxh := strings.ToLower(strings.TrimSpace(fmt.Sprint(rawSize)))
		if width, height, ok := parsePublishedImageSize(wxh); ok {
			key := fmt.Sprintf("%dx%d", width, height)
			if _, exists := anyToFloat(sizeRules[key]); exists {
				return key
			}
		}
	}
	if key, ok := highestSizeRuleKeyForTier(sizeRules, tier); ok {
		return key
	}
	if key, ok := highestSizeRuleKey(sizeRules); ok {
		return key
	}
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

func highestSizeRuleKey(sizeRules map[string]any) (string, bool) {
	bestKey := ""
	best := 0.0
	found := false
	for key, raw := range sizeRules {
		ratio, ok := anyToFloat(raw)
		if !ok || ratio <= 0 {
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
	sizeRules, ok := mapValue(multipliers["size"])
	if !ok || len(sizeRules) == 0 {
		return params
	}
	billingParams := cloneAnyMap(params)
	billingParams["size"] = gptImageBillingSizeLookupKey(firstPresent(billingParams, "size"), sizeRules)
	return billingParams
}
