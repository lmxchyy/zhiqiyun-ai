package httpserver

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"xianzhi-ai/backend-go/internal/config"
)

func TestSmartVideoRoutesAreRegisteredAndProtected(t *testing.T) {
	server := New(config.Config{Environment: "test", DataPath: filepath.Join(t.TempDir(), "store.json")})
	engine := server.Handler.(*gin.Engine)
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/video-projects"},
		{http.MethodPost, "/api/v1/video-projects"},
		{http.MethodGet, "/api/v1/video-projects/:id"},
		{http.MethodPatch, "/api/v1/video-projects/:id"},
		{http.MethodDelete, "/api/v1/video-projects/:id"},
		{http.MethodPost, "/api/v1/video-projects/:id/assets"},
		{http.MethodPut, "/api/v1/video-projects/:id/assets/order"},
		{http.MethodDelete, "/api/v1/video-projects/:id/assets/:assetId"},
		{http.MethodPost, "/api/v1/video-projects/:id/render-tasks"},
		{http.MethodPost, "/api/v1/video-projects/:id/analyze"},
		{http.MethodGet, "/api/v1/video-projects/:id/analysis"},
		{http.MethodPost, "/api/v1/video-projects/:id/assets/:assetId/retry-analysis"},
	} {
		if !hasGinRoute(t, engine, route.method, route.path) {
			t.Fatalf("missing smart-video route %s %s", route.method, route.path)
		}
	}

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/video-projects", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous list status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestSmartVideoAssetPayloadRejectsRemoteURLAndLocalPath(t *testing.T) {
	for _, body := range []string{
		`{"fileId":"file_1","assetType":"VIDEO","url":"http://127.0.0.1/private"}`,
		`{"fileId":"file_1","assetType":"VIDEO","path":"C:\\secret\\video.mp4"}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/video-projects/project/assets", strings.NewReader(body))
		recorder := httptest.NewRecorder()
		var input struct {
			FileID    string `json:"fileId"`
			AssetType string `json:"assetType"`
		}
		if err := decodeSmartVideoJSON(recorder, request, &input); err == nil {
			t.Fatalf("payload unexpectedly accepted: %s", body)
		}
	}
}
