package httpserver

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"xianzhi-ai/backend-go/internal/config"
)

func TestReviewModeIsPublicAndDefaultsToDisabled(t *testing.T) {
	t.Setenv("REVIEW_MODE_ENABLED", "")
	server := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/app/review-mode", nil)
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected public review-mode endpoint to return 200, got %d: %s", response.Code, response.Body.String())
	}
	if body := response.Body.String(); body == "" || body == "null\n" {
		t.Fatalf("expected review-mode configuration payload, got %q", body)
	}
}

func TestConfiguredReviewModeReadsBackendFlags(t *testing.T) {
	t.Setenv("REVIEW_MODE_ENABLED", "true")
	t.Setenv("REVIEW_MODE_HIDE_RECHARGE", "1")
	t.Setenv("REVIEW_MODE_HIDE_WALLET", "false")

	config := configuredReviewMode()
	if !config.Enabled || !config.HideRecharge || config.HideWallet {
		t.Fatalf("unexpected review mode config: %+v", config)
	}
}
