package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"xianzhi-ai/backend-go/internal/config"
)

func TestVideoListBaselineParamsPreferShortestAnd720p(t *testing.T) {
	duration, resolution := videoListBaselineParams(adminVideoModelCapabilities{
		SupportedDurations:   []int{10, 6, 15},
		SupportedResolutions: []string{"480p", "720p", "1080p"},
	})
	if duration != 6 || resolution != "720p" {
		t.Fatalf("baseline = %ds/%s, want 6s/720p", duration, resolution)
	}

	duration, resolution = videoListBaselineParams(adminVideoModelCapabilities{
		SupportedDurations:   []int{10, 15},
		SupportedResolutions: []string{"480p", "1080p"},
	})
	if duration != 10 || resolution != "480p" {
		t.Fatalf("baseline without 720p = %ds/%s, want 10s/480p", duration, resolution)
	}
}

func TestVideoModelPublicHints(t *testing.T) {
	data := normalizeAICapabilityDefaults(adminPlatformData{})
	previewCaps := grokImagine15VideoPreviewCapabilities()
	if got := videoModelPriceHint(data, "grok-imagine-video-1.5-preview"); got != "100 积分/次" {
		t.Fatalf("preview priceHint = %q", got)
	}
	if got := videoModelCapabilityHint(previewCaps); got != "仅图生 · 10/15s · 需1张参考图" {
		t.Fatalf("preview capabilityHint = %q", got)
	}
	grokCaps := grokImagine15VideoCapabilities()
	if got := videoModelPriceHint(data, "grok-imagine-1.5-video"); got != "15 积分/秒" {
		t.Fatalf("grok priceHint = %q", got)
	}
	if got := videoModelCapabilityHint(grokCaps); got != "文生/图生 · 6–30s · 最多7图" {
		t.Fatalf("grok capabilityHint = %q", got)
	}
	item := map[string]any{"code": "grok-imagine-1.5-video"}
	attachVideoModelPublicPricing(item, data, "grok-imagine-1.5-video", grokCaps)
	if item["displayName"] != "Grok Imagine Video 1.5" {
		t.Fatalf("displayName = %#v", item["displayName"])
	}
	if item["priceLabel"] != "15 积分/秒 · 文生/图生 · 6–30s · 最多7图" {
		t.Fatalf("priceLabel = %#v", item["priceLabel"])
	}
}

func TestVideoModelListPriceUsesBillingRules(t *testing.T) {
	data := normalizeAICapabilityDefaults(adminPlatformData{})
	previewCaps := grokImagine15VideoPreviewCapabilities()
	previewPoints, previewLabel := videoModelListPrice(data, "grok-imagine-video-1.5-preview", previewCaps)
	if previewPoints != 100 {
		t.Fatalf("preview list price = %d, want 100", previewPoints)
	}
	if previewLabel != "100 积分/次" {
		t.Fatalf("preview label = %q", previewLabel)
	}

	grokCaps := grokImagine15VideoCapabilities()
	grokPoints, grokLabel := videoModelListPrice(data, "grok-imagine-1.5-video", grokCaps)
	if grokPoints != 90 {
		t.Fatalf("grok 1.5 list price = %d, want 90", grokPoints)
	}
	if grokLabel != "约 90 积分起（6s·720p）" {
		t.Fatalf("grok 1.5 label = %q", grokLabel)
	}

	seedanceModel := findAIModel(data.AIModels, moduleVideoGeneration, "seedance-fast-2.0")
	schema := findAIParameterSchema(data.AIParameterSchemas, moduleVideoGeneration, seedanceModel.ModelName)
	seedanceCaps := resolveVideoModelCapabilities(seedanceModel, schema.SchemaJSON)
	seedancePoints, _ := videoModelListPrice(data, "seedance-fast-2.0", seedanceCaps)
	if seedancePoints <= grokPoints || seedancePoints <= previewPoints {
		t.Fatalf("seedance list price %d should rank above grok/preview", seedancePoints)
	}
}

func TestSortPublicModelsVideoByListPriceAscending(t *testing.T) {
	items := []map[string]any{
		{"code": "mock-standard", "capabilities": []string{"TEXT_TO_IMAGE"}, "listPricePoints": 1},
		{"code": "seedance-fast-2.0", "capabilities": []string{"TEXT_TO_VIDEO"}, "listPricePoints": 480, "videoCapabilities": map[string]any{}, "priceLabel": "约 480 积分起"},
		{"code": "grok-imagine-1.5-video", "capabilities": []string{"TEXT_TO_VIDEO"}, "listPricePoints": 90, "videoCapabilities": map[string]any{}, "priceLabel": "约 90 积分起"},
		{"code": "grok-imagine-video-1.5-preview", "capabilities": []string{"IMAGE_TO_VIDEO"}, "listPricePoints": 100, "videoCapabilities": map[string]any{}, "priceLabel": "100 积分/次"},
	}
	sorted := sortPublicModelsVideoByListPrice(items)
	want := []string{"mock-standard", "grok-imagine-1.5-video", "grok-imagine-video-1.5-preview", "seedance-fast-2.0"}
	got := make([]string, 0, len(sorted))
	for _, item := range sorted {
		got = append(got, publicModelCode(item))
	}
	if len(got) != len(want) {
		t.Fatalf("sorted codes = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("sorted codes = %v, want %v", got, want)
		}
	}
}

func TestFormalVideoModelsListPriceOrder(t *testing.T) {
	data := normalizeAICapabilityDefaults(adminPlatformData{})
	items := make([]map[string]any, 0, 8)
	for _, model := range data.AIModels {
		if !isVideoAIModel(model) || !isActiveLike(model.Status) {
			continue
		}
		code := strings.TrimSpace(model.ModelName)
		schema := findAIParameterSchema(data.AIParameterSchemas, moduleVideoGeneration, code)
		caps := resolveVideoModelCapabilities(model, schema.SchemaJSON)
		points, label := videoModelListPrice(data, code, caps)
		items = append(items, map[string]any{
			"code":               code,
			"capabilities":       publicModelCapabilities(model, schema.SchemaJSON),
			"videoCapabilities":  caps,
			"listPricePoints":    points,
			"priceLabel":         label,
		})
	}
	sorted := sortPublicModelsVideoByListPrice(items)
	indexOf := func(code string) int {
		for index, item := range sorted {
			if publicModelCode(item) == code {
				return index
			}
		}
		return -1
	}
	grok := indexOf("grok-imagine-1.5-video")
	preview := indexOf("grok-imagine-video-1.5-preview")
	seedance := indexOf("seedance-fast-2.0")
	doubao := indexOf("doubao-seedance-2.0")
	if grok < 0 || preview < 0 || seedance < 0 || doubao < 0 {
		t.Fatalf("missing formal video models in %#v", sorted)
	}
	if !(grok < preview && preview < seedance && preview < doubao) {
		codes := make([]string, 0, len(sorted))
		for _, item := range sorted {
			codes = append(codes, publicModelCode(item))
		}
		t.Fatalf("unexpected formal video price order: %v", codes)
	}
}

func TestPublicModelsIncludeVideoPriceFields(t *testing.T) {
	handler := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()}).Handler
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/models", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("models status = %d, body = %s", res.Code, res.Body.String())
	}
	var items []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	foundMockVideo := false
	for _, item := range items {
		if publicModelCode(item) != "mock-video" {
			continue
		}
		foundMockVideo = true
		if _, ok := item["listPricePoints"]; !ok {
			t.Fatalf("mock-video missing listPricePoints: %#v", item)
		}
		if label, _ := item["priceLabel"].(string); strings.TrimSpace(label) == "" {
			t.Fatalf("mock-video missing priceLabel: %#v", item)
		}
	}
	if !foundMockVideo {
		t.Fatal("mock-video missing from public models")
	}
}
