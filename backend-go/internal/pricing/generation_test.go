package pricing

import (
	"errors"
	"testing"
)

func TestCalculateImageGoldenPrices(t *testing.T) {
	rule := Rule{ID: "image-v1", ModelCode: "gpt-image-2", BillingUnit: "PER_IMAGE", BasePrice: 10, MinimumCharge: 1, Version: 1, ParameterRules: map[string]any{
		"quality": map[string]any{"standard": 1.0, "high": 1.5},
		"size":    map[string]any{"1024x1024": 1.0, "1024x1536": 1.2},
	}}
	tests := []struct {
		name, size, quality string
		count               float64
		want                int
	}{
		{"standard", "1024x1024", "standard", 1, 10},
		{"high", "1024x1024", "high", 1, 15},
		{"wide", "1024x1536", "standard", 1, 12},
		{"wide-high", "1024x1536", "high", 1, 18},
		{"count", "1024x1024", "standard", 2, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Calculate(Request{BusinessType: "IMAGE", Model: rule.ModelCode, Parameters: map[string]any{"size": tt.size, "quality": tt.quality, "count": tt.count}}, rule)
			if err != nil || got.RequiredPoints != tt.want {
				t.Fatalf("quote=%+v err=%v want=%d", got, err, tt.want)
			}
		})
	}
}

func TestCalculateVideoGoldenPrices(t *testing.T) {
	cases := []struct {
		model, unit string
		base, min   float64
		params      map[string]any
		want        int
	}{
		{"grok-imagine-video-1.5-preview", "PER_REQUEST", 100, 15, map[string]any{"duration": 10}, 100},
		{"grok-imagine-1.5-video", "PER_SECOND", 15, 15, map[string]any{"duration": 6, "resolution": "720p"}, 108},
		{"seedance-fast-2.0", "PER_SECOND", 80, 1, map[string]any{"duration": 5, "resolution": "720p"}, 600},
		{"doubao-seedance-2.0", "PER_SECOND", 80, 1, map[string]any{"duration": 5, "resolution": "4k"}, 1600},
	}
	for _, tt := range cases {
		resolutionRules := map[string]any{"480p": 1.0, "720p": 1.5, "1080p": 2.0, "4k": 4.0}
		if tt.model == "grok-imagine-1.5-video" {
			resolutionRules["720p"] = 1.2
		}
		rule := Rule{ID: tt.model + "-v2", ModelCode: tt.model, BillingUnit: tt.unit, BasePrice: tt.base, MinimumCharge: tt.min, Version: 2, ParameterRules: map[string]any{"resolution": resolutionRules}}
		got, err := Calculate(Request{BusinessType: "VIDEO", Model: tt.model, Parameters: tt.params}, rule)
		if err != nil || got.RequiredPoints != tt.want {
			t.Fatalf("model=%s quote=%+v err=%v want=%d", tt.model, got, err, tt.want)
		}
	}
}

func TestCalculateRequiresRule(t *testing.T) {
	_, err := Calculate(Request{BusinessType: "VIDEO", Model: "unknown", Parameters: map[string]any{}}, Rule{})
	if !errors.Is(err, ErrRuleNotFound) {
		t.Fatalf("err=%v", err)
	}
}
