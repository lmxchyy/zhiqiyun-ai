package video

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
)

func TestClassifyProviderFailure(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		code      string
		message   string
		cause     error
		wantClass string
	}{
		{name: "unsupported duration", status: 400, message: "duration must be <= 15", wantClass: ProviderUnsupportedParameter},
		{name: "invalid model", status: 400, message: "model not found", wantClass: ProviderInvalidModel},
		{name: "auth 401", status: 401, wantClass: ProviderAuthError},
		{name: "auth 403", status: 403, wantClass: ProviderAuthError},
		{name: "rate limit", status: 429, wantClass: ProviderRateLimit},
		{name: "quota", status: 402, wantClass: ProviderQuotaExceeded},
		{name: "timeout", cause: context.DeadlineExceeded, wantClass: ProviderTimeout},
		{name: "network", cause: errors.New("connection refused"), wantClass: ProviderNetworkError},
		{name: "server", status: 500, wantClass: Provider5xx},
		{name: "other client", status: 422, message: "invalid aspect ratio", wantClass: Provider4xx},
		{name: "unknown", status: 200, wantClass: ProviderUnknownError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyProviderFailure(tt.status, tt.code, tt.message, tt.cause)
			if got != tt.wantClass {
				t.Fatalf("classification = %q, want %q", got, tt.wantClass)
			}
		})
	}
}

func TestSanitizeProviderMessageRedactsSecretsAndURLQueries(t *testing.T) {
	input := `Authorization: Bearer sk-secret api_key=abc123 prompt="private story" failed at https://cdn.example/video.mp4?signature=secret&expires=123`
	got := sanitizeProviderMessage(input)
	if strings.Contains(got, "sk-secret") || strings.Contains(got, "abc123") || strings.Contains(got, "private story") || strings.Contains(got, "signature=") {
		t.Fatalf("sanitized message leaked secret: %q", got)
	}
	if !strings.Contains(got, "https://cdn.example/video.mp4") {
		t.Fatalf("sanitized message lost safe URL path: %q", got)
	}
}

func TestProviderResponseFieldsExtractsNestedError(t *testing.T) {
	code, message, jobID := providerResponseFields([]byte(`{"error":{"code":"invalid_model","message":"model not found"},"id":"provider-job-1"}`))
	if code != "invalid_model" || message != "model not found" || jobID != "provider-job-1" {
		t.Fatalf("fields = (%q, %q, %q)", code, message, jobID)
	}
}

func TestNewProviderErrorCarriesSafeClassification(t *testing.T) {
	err := newProviderError("newapi", 400, "invalid_parameter", "duration must be <= 15", "req-1", "job-1", nil)
	if err.FailureClassification() != ProviderUnsupportedParameter {
		t.Fatalf("failure class = %q", err.FailureClassification())
	}
	if err.ProviderRequestID() != "req-1" {
		t.Fatalf("request id = %q", err.ProviderRequestID())
	}
	if strings.Contains(err.Error(), "duration must be <= 15") == false {
		t.Fatalf("error message missing safe provider detail: %q", err.Error())
	}
}

func TestProviderHTTPFailuresExposeStableClassification(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantClass string
	}{
		{name: "unsupported duration", status: http.StatusBadRequest, body: `{"error":{"code":"invalid_parameter","message":"duration must be <= 15"}}`, wantClass: ProviderUnsupportedParameter},
		{name: "invalid model", status: http.StatusBadRequest, body: `{"error":{"code":"invalid_model","message":"model not found"}}`, wantClass: ProviderInvalidModel},
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"error":{"message":"invalid api key"}}`, wantClass: ProviderAuthError},
		{name: "forbidden", status: http.StatusForbidden, body: `{"error":{"message":"forbidden"}}`, wantClass: ProviderAuthError},
		{name: "rate limit", status: http.StatusTooManyRequests, body: `{"error":{"message":"rate limit exceeded"}}`, wantClass: ProviderRateLimit},
		{name: "server", status: http.StatusBadGateway, body: `{"error":{"message":"upstream unavailable"}}`, wantClass: Provider5xx},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{BaseURL: server.URL, APIKey: "test", Model: "video-model", Models: []string{"video-model"}})
			_, err := provider.Create(context.Background(), generation.CreateRequest{Type: "TEXT_TO_VIDEO", Model: "video-model", Prompt: "safe", Params: map[string]any{"duration": 6, "aspect_ratio": "16:9", "resolution": "720p"}})
			if err == nil {
				t.Fatal("expected provider error")
			}
			var providerErr *ProviderError
			if !errors.As(err, &providerErr) {
				t.Fatalf("error type = %T, want *ProviderError", err)
			}
			if providerErr.FailureClassification() != tt.wantClass {
				t.Fatalf("classification = %q, want %q", providerErr.FailureClassification(), tt.wantClass)
			}
		})
	}
}

func TestProviderTimeoutAndNetworkFailuresAreObservable(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(50 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"late","status":"completed","video_url":"https://cdn.example/video.mp4"}`))
		}))
		defer server.Close()
		provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{BaseURL: server.URL, APIKey: "test", Model: "video-model", Models: []string{"video-model"}, TimeoutMS: 5})
		_, err := provider.Create(context.Background(), generation.CreateRequest{Type: "TEXT_TO_VIDEO", Model: "video-model", Prompt: "safe", Params: map[string]any{"duration": 6, "aspect_ratio": "16:9", "resolution": "720p"}})
		var providerErr *ProviderError
		if !errors.As(err, &providerErr) || providerErr.FailureClassification() != ProviderTimeout {
			t.Fatalf("error = %v, classification = %v", err, providerErr)
		}
	})

	t.Run("network", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		baseURL := server.URL
		server.Close()
		provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{BaseURL: baseURL, APIKey: "test", Model: "video-model", Models: []string{"video-model"}})
		_, err := provider.Create(context.Background(), generation.CreateRequest{Type: "TEXT_TO_VIDEO", Model: "video-model", Prompt: "safe", Params: map[string]any{"duration": 6, "aspect_ratio": "16:9", "resolution": "720p"}})
		var providerErr *ProviderError
		if !errors.As(err, &providerErr) || providerErr.FailureClassification() != ProviderNetworkError {
			t.Fatalf("error = %v, classification = %v", err, providerErr)
		}
		if strings.Contains(fmt.Sprint(err), "Authorization") {
			t.Fatal("provider error exposed authorization data")
		}
	})
}
