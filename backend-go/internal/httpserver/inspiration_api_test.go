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
