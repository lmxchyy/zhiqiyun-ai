package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
	imageprovider "xianzhi-ai/backend-go/internal/provider/image"
)

func TestIssue124TierMapping(t *testing.T) {
	tests := []struct {
		size     string
		wantTier string
		aspect   string
	}{
		// 1K sizes with various aspect ratios
		{size: "1024x1024", wantTier: imageprovider.ImageBillingSizeTier1K, aspect: "1:1"},
		{size: "1536x1024", wantTier: imageprovider.ImageBillingSizeTier1K, aspect: "3:2"},
		{size: "1024x1536", wantTier: imageprovider.ImageBillingSizeTier1K, aspect: "2:3"},
		{size: "1024x1280", wantTier: imageprovider.ImageBillingSizeTier1K, aspect: "4:5"},

		// 2K sizes with various aspect ratios
		{size: "2048x2048", wantTier: imageprovider.ImageBillingSizeTier2K, aspect: "1:1"},
		{size: "2048x1152", wantTier: imageprovider.ImageBillingSizeTier2K, aspect: "16:9"},
		{size: "1152x2048", wantTier: imageprovider.ImageBillingSizeTier2K, aspect: "9:16"},
		{size: "1792x1024", wantTier: imageprovider.ImageBillingSizeTier2K, aspect: "7:4"},
		{size: "1600x1024", wantTier: imageprovider.ImageBillingSizeTier2K, aspect: "25:16"},
		{size: "1536x1536", wantTier: imageprovider.ImageBillingSizeTier2K, aspect: "1:1"},
		{size: "1280x1280", wantTier: imageprovider.ImageBillingSizeTier2K, aspect: "1:1"},

		// 4K sizes with various aspect ratios
		{size: "3840x2160", wantTier: imageprovider.ImageBillingSizeTier4K, aspect: "16:9"},
		{size: "2160x3840", wantTier: imageprovider.ImageBillingSizeTier4K, aspect: "9:16"},
		{size: "2880x2880", wantTier: imageprovider.ImageBillingSizeTier4K, aspect: "1:1"},
		{size: "2304x1280", wantTier: imageprovider.ImageBillingSizeTier4K, aspect: "9:5"},
		{size: "2560x1440", wantTier: imageprovider.ImageBillingSizeTier4K, aspect: "16:9"},

		// 720p sizes
		{size: "1280x720", wantTier: imageprovider.ImageBillingSizeTier720, aspect: "16:9"},
		{size: "720x1280", wantTier: imageprovider.ImageBillingSizeTier720, aspect: "9:16"},

		// auto
		{size: "auto", wantTier: imageprovider.ImageBillingSizeAuto, aspect: "auto"},
	}

	for _, tt := range tests {
		tier, err := imageprovider.NormalizeImageBillingSizeTier(tt.size)
		if err != nil {
			t.Fatalf("size %s (%s): unexpected error %v", tt.size, tt.aspect, err)
		}
		if tier != tt.wantTier {
			t.Fatalf("size %s (%s) got tier %s, want %s", tt.size, tt.aspect, tier, tt.wantTier)
		}
	}
}

func TestIssue124NoFallbackToLegacy1024x1536(t *testing.T) {
	// Simulate un-migrated legacy size rules that only had 1K dimensions
	legacySizeRulesOnly1K := map[string]any{
		"1024x1024": float64(1.0),
		"1024x1536": float64(1.2),
		"1536x1024": float64(1.2),
	}

	canonicalMultipliers := canonicalizeGPTImageBillingMultipliers(map[string]any{
		"size": legacySizeRulesOnly1K,
	})
	sizeRules := canonicalMultipliers["size"].(map[string]any)

	// 1. 2048x2048 must NOT fallback to 1024x1536 or 1.2
	lookup2K := gptImageBillingSizeLookupKey("2048x2048", sizeRules)
	if lookup2K == "1024x1536" {
		t.Fatalf("2048x2048 wrongly fell back to 1024x1536")
	}
	mult2K, ok := anyToFloat(sizeRules[lookup2K])
	if !ok || mult2K != 1.5 {
		t.Fatalf("2048x2048 lookup=%s multiplier=%v, want 1.5", lookup2K, mult2K)
	}

	// 2. 3840x2160 must NOT fallback to 1024x1536 or 1.2
	lookup4K := gptImageBillingSizeLookupKey("3840x2160", sizeRules)
	if lookup4K == "1024x1536" {
		t.Fatalf("3840x2160 wrongly fell back to 1024x1536")
	}
	mult4K, ok := anyToFloat(sizeRules[lookup4K])
	if !ok || mult4K != 2.0 {
		t.Fatalf("3840x2160 lookup=%s multiplier=%v, want 2.0", lookup4K, mult4K)
	}

	// 3. 1024x1536 must normalize to 1.0, not 1.2
	lookup1KWide := gptImageBillingSizeLookupKey("1024x1536", sizeRules)
	mult1KWide, ok := anyToFloat(sizeRules[lookup1KWide])
	if !ok || mult1KWide != 1.0 {
		t.Fatalf("1024x1536 lookup=%s multiplier=%v, want 1.0", lookup1KWide, mult1KWide)
	}

	// Even if raw legacy rules (without tier keys) are passed directly into gptImageBillingSizeLookupKey,
	// it must NEVER return 1024x1536 for 2K or 4K.
	rawLookup2K := gptImageBillingSizeLookupKey("2048x2048", legacySizeRulesOnly1K)
	if rawLookup2K == "1024x1536" {
		t.Fatalf("raw legacy lookup 2048x2048 fell back to 1024x1536")
	}
	if rawLookup2K != imageprovider.ImageBillingSizeTier2K {
		t.Fatalf("raw legacy lookup 2048x2048 = %s, want %s", rawLookup2K, imageprovider.ImageBillingSizeTier2K)
	}

	rawLookup4K := gptImageBillingSizeLookupKey("3840x2160", legacySizeRulesOnly1K)
	if rawLookup4K == "1024x1536" {
		t.Fatalf("raw legacy lookup 3840x2160 fell back to 1024x1536")
	}
	if rawLookup4K != imageprovider.ImageBillingSizeTier4K {
		t.Fatalf("raw legacy lookup 3840x2160 = %s, want %s", rawLookup4K, imageprovider.ImageBillingSizeTier4K)
	}
}

func TestIssue124PriceMatrixCombinations(t *testing.T) {
	data := normalizeAICapabilityDefaults(adminPlatformData{})

	// Testing low, normal, high combinations with 1K, 2K, 4K and n=1, n=2, n=3, n=4
	// basePrice = 10
	// Tier multipliers: 1K=1.0, 2K=1.5, 4K=2.0
	// Quality multipliers: low=1.0, normal=1.2 (or medium=1.2), high=1.5
	tests := []struct {
		name    string
		size    string
		quality string
		n       float64
		want    int
	}{
		// --- Single image (n=1), low quality (Requirement 5) ---
		{name: "1k-low-n1 (1024x1024)", size: "1024x1024", quality: "low", n: 1, want: 10},
		{name: "1k-low-n1 (1024x1536)", size: "1024x1536", quality: "low", n: 1, want: 10},
		{name: "1k-low-n1 (1536x1024)", size: "1536x1024", quality: "low", n: 1, want: 10},
		{name: "2k-low-n1 (2048x2048)", size: "2048x2048", quality: "low", n: 1, want: 15},
		{name: "2k-low-n1 (2048x1152)", size: "2048x1152", quality: "low", n: 1, want: 15},
		{name: "2k-low-n1 (1152x2048)", size: "1152x2048", quality: "low", n: 1, want: 15},
		{name: "4k-low-n1 (3840x2160)", size: "3840x2160", quality: "low", n: 1, want: 20},
		{name: "4k-low-n1 (2160x3840)", size: "2160x3840", quality: "low", n: 1, want: 20},
		{name: "4k-low-n1 (2880x2880)", size: "2880x2880", quality: "low", n: 1, want: 20},

		// --- Single image (n=1), normal / medium quality ---
		// 1K: 10 * 1 * 1.2 * 1.0 = 12
		// 2K: 10 * 1 * 1.2 * 1.5 = 18
		// 4K: 10 * 1 * 1.2 * 2.0 = 24
		{name: "1k-normal-n1", size: "1024x1024", quality: "normal", n: 1, want: 12},
		{name: "1k-medium-n1", size: "1024x1536", quality: "medium", n: 1, want: 12},
		{name: "2k-normal-n1", size: "2048x2048", quality: "normal", n: 1, want: 18},
		{name: "2k-medium-n1", size: "2048x1152", quality: "medium", n: 1, want: 18},
		{name: "4k-normal-n1", size: "3840x2160", quality: "normal", n: 1, want: 24},
		{name: "4k-medium-n1", size: "2880x2880", quality: "medium", n: 1, want: 24},

		// --- Single image (n=1), high quality ---
		// 1K: 10 * 1 * 1.5 * 1.0 = 15
		// 2K: ceil(10 * 1 * 1.5 * 1.5) = ceil(22.5) = 23
		// 4K: 10 * 1 * 1.5 * 2.0 = 30
		{name: "1k-high-n1 (1024x1024)", size: "1024x1024", quality: "high", n: 1, want: 15},
		{name: "1k-high-n1 (1024x1536)", size: "1024x1536", quality: "high", n: 1, want: 15},
		{name: "2k-high-n1 (2048x2048)", size: "2048x2048", quality: "high", n: 1, want: 23},
		{name: "2k-high-n1 (2048x1152)", size: "2048x1152", quality: "high", n: 1, want: 23},
		{name: "4k-high-n1 (3840x2160)", size: "3840x2160", quality: "high", n: 1, want: 30},
		{name: "4k-high-n1 (2880x2880)", size: "2880x2880", quality: "high", n: 1, want: 30},

		// --- Multi-image (n=2) ---
		// low quality
		{name: "1k-low-n2", size: "1024x1024", quality: "low", n: 2, want: 20},
		{name: "2k-low-n2", size: "2048x2048", quality: "low", n: 2, want: 30},
		{name: "4k-low-n2", size: "3840x2160", quality: "low", n: 2, want: 40},
		// normal quality
		{name: "1k-normal-n2", size: "1024x1024", quality: "normal", n: 2, want: 24},
		{name: "2k-normal-n2", size: "2048x2048", quality: "normal", n: 2, want: 36},
		{name: "4k-normal-n2", size: "3840x2160", quality: "normal", n: 2, want: 48},
		// high quality
		{name: "1k-high-n2", size: "1024x1024", quality: "high", n: 2, want: 30},
		{name: "2k-high-n2", size: "2048x2048", quality: "high", n: 2, want: 45},
		{name: "4k-high-n2", size: "3840x2160", quality: "high", n: 2, want: 60},

		// --- Multi-image (n=3 and n=4) ---
		{name: "2k-low-n3", size: "2048x2048", quality: "low", n: 3, want: 45},
		{name: "4k-normal-n3", size: "3840x2160", quality: "normal", n: 3, want: 72},
		{name: "1k-high-n4", size: "1024x1024", quality: "high", n: 4, want: 60},
		{name: "2k-high-n4", size: "2048x2048", quality: "high", n: 4, want: 90},
		{name: "4k-high-n4", size: "3840x2160", quality: "high", n: 4, want: 120},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createGenerationTaskRequest{
				Type:  "TEXT_TO_IMAGE",
				Model: "gpt-image-2",
				Params: map[string]any{
					"size":    tt.size,
					"quality": tt.quality,
					"n":       tt.n,
				},
			}
			quote, err := generationQuoteForRequest(req, data)
			if err != nil {
				t.Fatalf("generationQuoteForRequest(%s, %s, n=%v): %v", tt.size, tt.quality, tt.n, err)
			}
			if quote.RequiredPoints != tt.want {
				t.Fatalf("%s (%s %s n=%v) quote = %d, want %d", tt.name, tt.size, tt.quality, tt.n, quote.RequiredPoints, tt.want)
			}

			// Also test via generationPointCostForRequest
			points := generationPointCostForRequest(req, data)
			if points != tt.want {
				t.Fatalf("%s (%s %s n=%v) pointCost = %d, want %d", tt.name, tt.size, tt.quality, tt.n, points, tt.want)
			}
		})
	}
}

func TestIssue124IllegalAndUnknownSizeSafetyBehavior(t *testing.T) {
	illegalSizes := []string{
		"1023x1024",
		"4096x4096",
		"64x64",
		"100x100",
		"3840x3840",
		"not-a-size",
		"9999x9999",
	}

	for _, size := range illegalSizes {
		// 1. NormalizeImageBillingSizeTier must return error
		if _, err := imageprovider.NormalizeImageBillingSizeTier(size); err == nil {
			t.Fatalf("illegal size %q should return error from NormalizeImageBillingSizeTier", size)
		}

		// 2. ValidateGPTImageSize must return error
		if err := imageprovider.ValidateGPTImageSize(size); err == nil {
			t.Fatalf("illegal size %q should return error from ValidateGPTImageSize", size)
		}

		// 3. gptImageBillingSizeLookupKey must return the raw size string, never a legitimate tier
		rules := publishedGPTImageSizeRules()
		lookup := gptImageBillingSizeLookupKey(size, rules)
		if lookup == imageprovider.ImageBillingSizeTier1K || lookup == imageprovider.ImageBillingSizeTier2K || lookup == imageprovider.ImageBillingSizeTier4K {
			t.Fatalf("illegal size %q received legitimate tier %q from lookup", size, lookup)
		}
	}
}

func TestIssue124ServerQuoteAPIEndpoint(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	handler := newWithStore(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "api.json"), StaticDir: t.TempDir()}, store).Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

	testCases := []struct {
		size    string
		quality string
		n       float64
		want    int
	}{
		{"1024x1024", "low", 1, 10},
		{"2048x2048", "low", 1, 15},
		{"3840x2160", "low", 1, 20},
		{"1024x1536", "low", 1, 10},
		{"2048x2048", "normal", 1, 18},
		{"3840x2160", "high", 1, 30},
		{"2048x2048", "low", 2, 30},
		{"3840x2160", "low", 2, 40},
	}

	for _, tc := range testCases {
		body, err := json.Marshal(generation.CreateRequest{
			Type:  "TEXT_TO_IMAGE",
			Model: "gpt-image-2",
			Prompt: "issue 124 quote test",
			Params: map[string]any{
				"size":    tc.size,
				"quality": tc.quality,
				"n":       tc.n,
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		response := authedRequest(t, handler, http.MethodPost, "/api/v1/generation-tasks/quote", bytes.NewBuffer(body), token)
		if response.Code != http.StatusOK {
			t.Fatalf("POST /generation-tasks/quote for %s %s n=%v status = %d: %s", tc.size, tc.quality, tc.n, response.Code, response.Body.String())
		}

		var payload generationQuoteResponse
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatalf("unmarshal quote response: %v", err)
		}
		if payload.RequiredPoints != tc.want {
			t.Fatalf("size=%s quality=%s n=%v quoted requiredPoints=%d, want %d", tc.size, tc.quality, tc.n, payload.RequiredPoints, tc.want)
		}
	}
}
