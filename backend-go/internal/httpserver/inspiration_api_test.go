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
	if strings.Contains(featured.Body.String(), `"definition"`) || strings.Contains(featured.Body.String(), `"prompt"`) {
		t.Fatalf("featured list leaked internal definition: %s", featured.Body.String())
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
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"schema"`) || !strings.Contains(detail.Body.String(), `"templateVersion":1`) || !strings.Contains(detail.Body.String(), `"aiGenerated":true`) {
		t.Fatalf("detail = %d %s", detail.Code, detail.Body.String())
	}
	if strings.Contains(detail.Body.String(), `"prompt"`) || strings.Contains(detail.Body.String(), `"modelHint"`) {
		t.Fatalf("detail leaked internal composition data: %s", detail.Body.String())
	}

	favorite := request(t, handler, http.MethodPut, "/api/v1/inspirations/inspiration-product-clean/favorite", nil)
	if favorite.Code != http.StatusUnauthorized {
		t.Fatalf("guest favorite = %d %s", favorite.Code, favorite.Body.String())
	}
}

func TestInspirationAdminRequiresAuditBeforePublish(t *testing.T) {
	handler := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()}).Handler
	token := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	created := createAdminInspiration(t, handler, token, "audit-gate-template", testInspirationDefinition("Create {{subject}}", ""))

	publish := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations/"+created.ID+"/publish", nil, token)
	if publish.Code != http.StatusConflict {
		t.Fatalf("publish before audit = %d %s", publish.Code, publish.Body.String())
	}
	approve := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations/"+created.ID+"/audit/approve", nil, token)
	if approve.Code != http.StatusOK {
		t.Fatalf("approve = %d %s", approve.Code, approve.Body.String())
	}
	publish = authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations/"+created.ID+"/publish", nil, token)
	if publish.Code != http.StatusOK || !strings.Contains(publish.Body.String(), `"status":"PUBLISHED"`) {
		t.Fatalf("publish = %d %s", publish.Code, publish.Body.String())
	}
}

func TestInspirationRejectsInvalidDefinitionInputRange(t *testing.T) {
	handler := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()}).Handler
	token := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	definition := testInspirationDefinition("Create {{subject}}", "")
	minimum, maximum := 2, 1
	definition.Inputs = append(definition.Inputs, TemplateInputDefinition{
		Key: "references", Type: TemplateInputImage, Label: "References",
		Validation: TemplateInputValidation{MinItems: &minimum, MaxItems: &maximum},
	})
	body, err := json.Marshal(map[string]any{
		"slug": "invalid-input-range", "title": "Invalid", "contentType": "image",
		"categoryId": "inspiration-category-product", "coverUrl": "https://example.test/cover.webp",
		"definition": definition, "platforms": []string{"miniprogram"}, "sourceAuthorized": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	created := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/inspirations", bytes.NewBuffer(body), token)
	if created.Code != http.StatusBadRequest {
		t.Fatalf("invalid input range = %d %s, want 400", created.Code, created.Body.String())
	}
}
