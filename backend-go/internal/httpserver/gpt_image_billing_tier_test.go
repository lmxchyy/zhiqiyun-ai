package httpserver

import (
	"fmt"
	"strings"
	"testing"

	imageprovider "xianzhi-ai/backend-go/internal/provider/image"
)

func publishedGPTImageSizeRules() map[string]any {
	return map[string]any{
		"auto":      float64(1),
		"1024x1024": float64(1),
		"1024x1536": float64(1.2),
		"1536x1024": float64(1.2),
		"1280x720":  float64(1),
		"720x1280":  float64(1),
		"2048x2048": float64(1.5),
		"2048x1152": float64(1.5),
		"3840x2160": float64(2),
		"2160x3840": float64(2),
	}
}

func currentGPTImageUISizes() []string {
	sizes := []string{}
	for _, item := range gptImage2SizeOptions() {
		sizes = append(sizes, fmt.Sprint(item))
	}
	presets := []string{
		"1152x2048",
		"2880x2880",
		"3456x2304",
		"2304x3456",
		"1792x1024",
		"1600x1024",
		"1024x640",
	}
	seen := map[string]bool{}
	for _, size := range sizes {
		seen[size] = true
	}
	for _, size := range presets {
		if !seen[size] {
			sizes = append(sizes, size)
			seen[size] = true
		}
	}
	return sizes
}

func TestAllCurrentUISizesMapToPublishedBillingTiers(t *testing.T) {
	published := publishedGPTImageSizeRules()
	minPublished := 0.0
	for _, raw := range published {
		mult, ok := anyToFloat(raw)
		if !ok {
			continue
		}
		if minPublished == 0 || mult < minPublished {
			minPublished = mult
		}
	}
	data := normalizeAICapabilityDefaults(adminPlatformData{})
	for _, size := range currentGPTImageUISizes() {
		tier, err := imageprovider.NormalizeImageBillingSizeTier(size)
		if err != nil {
			t.Fatalf("UI size %s is not official-legal: %v", size, err)
		}
		lookup := gptImageBillingSizeLookupKey(size, published)
		mult, ok := anyToFloat(published[lookup])
		if !ok || mult <= 0 {
			t.Fatalf("size %s lookup %s is missing from PUBLISHED size rules", size, lookup)
		}
		_, exact := anyToFloat(published[strings.ToLower(size)])
		if !exact && size != "auto" {
			maxTier := 0.0
			if key, found := highestSizeRuleKeyForTier(published, tier); found {
				maxTier, _ = anyToFloat(published[key])
			}
			if maxTier > minPublished && mult <= minPublished {
				t.Fatalf("size %s missed exact key and fell to cheapest multiplier %v", size, mult)
			}
			if maxTier > 0 && mult != maxTier {
				t.Fatalf("size %s custom lookup multiplier %v, want tier max %v", size, mult, maxTier)
			}
		}
		req := createGenerationTaskRequest{
			Type:   "TEXT_TO_IMAGE",
			Model:  "gpt-image-2",
			Params: map[string]any{"size": size, "quality": "low", "n": float64(1)},
		}
		if req.Params["size"] != size {
			t.Fatalf("provider size rewritten before quote")
		}
		points := generationPointCostForRequest(req, data)
		if points < 1 {
			t.Fatalf("size %s quoted %d, want a positive charge", size, points)
		}
		if req.Params["size"] != size {
			t.Fatalf("quoting mutated provider size %s -> %v", size, req.Params["size"])
		}
		t.Logf("size=%s tier=%s lookup=%s multiplier=%v points=%d", size, tier, lookup, mult, points)
	}
}

func TestGPTImageBillingSizeLookupUsesTiersWithoutCheapFallback(t *testing.T) {
	published := publishedGPTImageSizeRules()
	tests := []struct {
		requestSize   string
		wantTier      string
		wantLookupKey string
		wantMult      float64
	}{
		{requestSize: "auto", wantTier: imageprovider.ImageBillingSizeAuto, wantLookupKey: "auto", wantMult: 1},
		{requestSize: "1024x1024", wantTier: imageprovider.ImageBillingSizeTier1K, wantLookupKey: "1024x1024", wantMult: 1},
		{requestSize: "1536x1024", wantTier: imageprovider.ImageBillingSizeTier1K, wantLookupKey: "1536x1024", wantMult: 1.2},
		{requestSize: "1024x1536", wantTier: imageprovider.ImageBillingSizeTier1K, wantLookupKey: "1024x1536", wantMult: 1.2},
		{requestSize: "1280x720", wantTier: imageprovider.ImageBillingSizeTier720, wantLookupKey: "1280x720", wantMult: 1},
		{requestSize: "2048x2048", wantTier: imageprovider.ImageBillingSizeTier2K, wantLookupKey: "2048x2048", wantMult: 1.5},
		{requestSize: "1792x1024", wantTier: imageprovider.ImageBillingSizeTier2K, wantLookupKey: "2048x2048", wantMult: 1.5},
		{requestSize: "1600x1024", wantTier: imageprovider.ImageBillingSizeTier2K, wantLookupKey: "2048x2048", wantMult: 1.5},
		{requestSize: "2880x2880", wantTier: imageprovider.ImageBillingSizeTier4K, wantLookupKey: "3840x2160", wantMult: 2},
	}
	for _, tt := range tests {
		tier, err := imageprovider.NormalizeImageBillingSizeTier(tt.requestSize)
		if err != nil {
			t.Fatalf("size %s: %v", tt.requestSize, err)
		}
		if tier != tt.wantTier {
			t.Fatalf("size %s tier = %s, want %s", tt.requestSize, tier, tt.wantTier)
		}
		gotKey := gptImageBillingSizeLookupKey(tt.requestSize, published)
		if gotKey != tt.wantLookupKey {
			t.Fatalf("size %s lookup = %s, want %s", tt.requestSize, gotKey, tt.wantLookupKey)
		}
		gotMult, ok := anyToFloat(published[gotKey])
		if !ok || gotMult != tt.wantMult {
			t.Fatalf("size %s multiplier = %v, want %v", tt.requestSize, gotMult, tt.wantMult)
		}
	}

	draft := gptImage2Phase1BillingParameterRules()["size"].(map[string]any)
	if got := gptImageBillingSizeLookupKey("1792x1024", draft); got != imageprovider.ImageBillingSizeTier2K {
		t.Fatalf("draft custom lookup = %s, want %s", got, imageprovider.ImageBillingSizeTier2K)
	}
	if got := gptImageBillingSizeLookupKey("1024x1024", draft); got != imageprovider.ImageBillingSizeTier1K {
		t.Fatalf("draft 1K lookup = %s, want %s", got, imageprovider.ImageBillingSizeTier1K)
	}
}

func TestGPTImagePublishedBillingSmokePaths(t *testing.T) {
	data := normalizeAICapabilityDefaults(adminPlatformData{})
	tests := []struct {
		size    string
		quality string
		n       float64
		points  int
	}{
		{size: "1024x1024", quality: "low", n: 1, points: 10},
		{size: "1536x1024", quality: "medium", n: 1, points: 15},
		{size: "1280x720", quality: "low", n: 1, points: 10},
		{size: "2048x2048", quality: "low", n: 1, points: 15},
		{size: "1792x1024", quality: "low", n: 1, points: 15},
		{size: "1024x1024", quality: "low", n: 3, points: 30},
	}
	for _, tt := range tests {
		req := createGenerationTaskRequest{
			Type:  "TEXT_TO_IMAGE",
			Model: "gpt-image-2",
			Params: map[string]any{
				"size":    tt.size,
				"quality": tt.quality,
				"n":       tt.n,
			},
		}
		got := generationPointCostForRequest(req, data)
		if got != tt.points {
			t.Fatalf("%s %s n=%v quoted %d, want %d", tt.size, tt.quality, tt.n, got, tt.points)
		}
		billingParams := billingParamsForRequest(req.Model, req.Params, billingRuleForRequest(req, data).ParameterMultiplier)
		if billingParams["size"] == tt.size && tt.size == "1792x1024" {
			t.Fatal("custom WxH must not look up its own missing exact key")
		}
		if req.Params["size"] != tt.size {
			t.Fatalf("provider size mutated to %v, want original %s", req.Params["size"], tt.size)
		}
	}
}

func TestGPTImageAliasDoesNotChangePublishedQuoteWhenCanonicalPresent(t *testing.T) {
	data := normalizeAICapabilityDefaults(adminPlatformData{})
	canonical := generationPointCostForRequest(createGenerationTaskRequest{
		Type:   "TEXT_TO_IMAGE",
		Model:  "gpt-image-2",
		Params: map[string]any{"size": "1536x1024", "quality": "high", "n": float64(2)},
	}, data)
	aliased := generationPointCostForRequest(createGenerationTaskRequest{
		Type:  "TEXT_TO_IMAGE",
		Model: "gpt-image-2",
		Params: map[string]any{
			"size":         "1536x1024",
			"imageRatio":   "1:1",
			"quality":      "high",
			"imageQuality": "low",
			"n":            float64(2),
			"count":        float64(4),
		},
	}, data)
	if canonical != aliased {
		t.Fatalf("alias override changed quote %d -> %d", canonical, aliased)
	}
}
