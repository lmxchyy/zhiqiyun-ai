package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"xianzhi-ai/backend-go/internal/config"
)

func TestInspirationPublicSummaryDetailAndAuthGate(t *testing.T) {
	handler := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()}).Handler

	featured := request(t, handler, http.MethodGet, "/api/v1/inspirations/featured?limit=6&platform=miniprogram", nil)
	if featured.Code != http.StatusOK || !strings.Contains(featured.Body.String(), `"items"`) {
		t.Fatalf("featured = %d %s", featured.Code, featured.Body.String())
	}
	if strings.Contains(featured.Body.String(), `"prompt":"`) {
		t.Fatalf("featured list leaked full prompt: %s", featured.Body.String())
	}
	if strings.Contains(featured.Body.String(), `"contentType":"video"`) {
		t.Fatalf("featured list still exposes video inspiration on miniprogram: %s", featured.Body.String())
	}

	categories := request(t, handler, http.MethodGet, "/api/v1/inspirations/categories?platform=miniprogram", nil)
	if categories.Code != http.StatusOK {
		t.Fatalf("categories = %d %s", categories.Code, categories.Body.String())
	}
	if strings.Contains(categories.Body.String(), `"AI视频"`) || strings.Contains(categories.Body.String(), `"code":"video"`) {
		t.Fatalf("categories still expose AI视频 on miniprogram: %s", categories.Body.String())
	}

	videoDetail := request(t, handler, http.MethodGet, "/api/v1/inspirations/inspiration-video-product?platform=miniprogram", nil)
	if videoDetail.Code != http.StatusNotFound {
		t.Fatalf("video detail should be hidden on miniprogram, got %d %s", videoDetail.Code, videoDetail.Body.String())
	}

	detail := request(t, handler, http.MethodGet, "/api/v1/inspirations/inspiration-product-clean?platform=miniprogram", nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"prompt":"`) || !strings.Contains(detail.Body.String(), `"aiGenerated":true`) {
		t.Fatalf("detail = %d %s", detail.Code, detail.Body.String())
	}

	favorite := request(t, handler, http.MethodPut, "/api/v1/inspirations/inspiration-product-clean/favorite", nil)
	if favorite.Code != http.StatusUnauthorized {
		t.Fatalf("guest favorite = %d %s", favorite.Code, favorite.Body.String())
	}
}

func TestInspirationAdminRequiresAuditBeforePublish(t *testing.T) {
	handler := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()}).Handler
	token := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	body := bytes.NewBufferString(`{"title":"测试商品图","description":"测试模板","contentType":"image","categoryId":"inspiration-product","coverUrl":"https://example.com/cover.webp","prompt":"专业商品摄影","modelId":"mock-standard","parameters":{"ratio":"1:1"},"platforms":["miniprogram"],"sourceAuthorized":true}`)
	created := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations", body, token)
	if created.Code != http.StatusOK {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	var payload struct {
		Item inspirationTemplate `json:"item"`
	}
	if err := json.NewDecoder(created.Body).Decode(&payload); err != nil || payload.Item.ID == "" {
		t.Fatalf("decode create: %v %s", err, created.Body.String())
	}

	publish := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations/"+payload.Item.ID+"/publish", nil, token)
	if publish.Code != http.StatusConflict {
		t.Fatalf("publish before audit = %d %s", publish.Code, publish.Body.String())
	}
	approve := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations/"+payload.Item.ID+"/audit/approve", nil, token)
	if approve.Code != http.StatusOK {
		t.Fatalf("approve = %d %s", approve.Code, approve.Body.String())
	}
	publish = authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations/"+payload.Item.ID+"/publish", nil, token)
	if publish.Code != http.StatusOK || !strings.Contains(publish.Body.String(), `"status":"PUBLISHED"`) {
		t.Fatalf("publish = %d %s", publish.Code, publish.Body.String())
	}
}

func TestInspirationPhotoRestorationContractRoundTrips(t *testing.T) {
	handler := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()}).Handler
	token := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	body := bytes.NewBufferString(`{
		"title":"Old photo restoration",
		"description":"Restore treasured memories",
		"contentType":"image",
		"categoryId":"inspiration-category-portrait",
		"coverUrl":"https://example.com/restoration-cover.webp",
		"prompt":"Restore the uploaded old photo while preserving identity",
		"modelId":"mock-standard",
		"scenarioCode":"photo_restoration",
		"displayConfig":{
			"comparisonMode":"side_by_side",
			"beforeUrl":"https://example.com/before.jpg",
			"afterUrl":"https://example.com/after.jpg"
		},
		"inputRequirements":{
			"referenceImageRequired":true
		},
		"presetConfig":{
			"colorMode":"natural",
			"identityProtection":true
		},
		"referenceAssets":["https://example.com/example-only.jpg"],
		"platforms":["miniprogram"],
		"sourceAuthorized":true
	}`)
	created := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations", body, token)
	if created.Code != http.StatusOK {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	var payload struct {
		Item map[string]any `json:"item"`
	}
	if err := json.NewDecoder(created.Body).Decode(&payload); err != nil {
		t.Fatalf("decode create: %v %s", err, created.Body.String())
	}
	if got := payload.Item["scenarioCode"]; got != "photo_restoration" {
		t.Fatalf("scenarioCode = %#v, want photo_restoration", got)
	}
	display, ok := payload.Item["displayConfig"].(map[string]any)
	if !ok || display["comparisonMode"] != "side_by_side" || display["beforeUrl"] == "" || display["afterUrl"] == "" {
		t.Fatalf("displayConfig = %#v", payload.Item["displayConfig"])
	}
	requirements, ok := payload.Item["inputRequirements"].(map[string]any)
	if !ok || requirements["referenceImageRequired"] != true || requirements["referenceImageMin"] != float64(1) || requirements["referenceImageMax"] != float64(1) {
		t.Fatalf("inputRequirements = %#v", payload.Item["inputRequirements"])
	}
	if refs, ok := payload.Item["referenceAssets"].([]any); !ok || len(refs) != 0 {
		t.Fatalf("referenceAssets = %#v, want empty for user-upload-required template", payload.Item["referenceAssets"])
	}
	if tenants, ok := payload.Item["applicableTenantIds"].([]any); !ok || len(tenants) != 0 {
		t.Fatalf("applicableTenantIds = %#v, want empty array", payload.Item["applicableTenantIds"])
	}
}

func TestInspirationRejectsInvalidReferenceImageRange(t *testing.T) {
	handler := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()}).Handler
	token := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	body := bytes.NewBufferString(`{
		"title":"Invalid restoration template",
		"contentType":"image",
		"categoryId":"inspiration-category-portrait",
		"coverUrl":"https://example.com/cover.webp",
		"prompt":"Restore photo",
		"inputRequirements":{
			"referenceImageRequired":true,
			"referenceImageMin":2,
			"referenceImageMax":1
		},
		"platforms":["miniprogram"],
		"sourceAuthorized":true
	}`)
	created := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations", body, token)
	if created.Code != http.StatusBadRequest {
		t.Fatalf("invalid reference image range = %d %s, want 400", created.Code, created.Body.String())
	}
}
