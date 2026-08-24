package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newMetricsTestRouter(collector *httpMetricsCollector) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(collector.middleware())
	router.GET("/api/v1/ping", wrapF(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	router.POST("/api/v1/boom", wrapF(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	return router
}

func TestHTTPMetricsMiddlewareCountsRoutesAndRendersPrometheusText(t *testing.T) {
	collector := newHTTPMetricsCollector()
	router := newMetricsTestRouter(collector)

	for i := 0; i < 3; i++ {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("ping returned %d, want 200", response.Code)
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/boom", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("boom returned %d, want 500", response.Code)
	}
	unmatched := httptest.NewRecorder()
	router.ServeHTTP(unmatched, httptest.NewRequest(http.MethodGet, "/nowhere", nil))

	rendered := httptest.NewRecorder()
	collector.handler(rendered, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	output := rendered.Body.String()

	expectations := []string{
		"# TYPE xianzhi_http_requests_total counter",
		`xianzhi_http_requests_total{method="GET",path="/api/v1/ping",status="200"} 3`,
		`xianzhi_http_requests_total{method="POST",path="/api/v1/boom",status="500"} 1`,
		`xianzhi_http_requests_total{method="GET",path="unmatched",status="404"} 1`,
		`xianzhi_http_request_duration_seconds_count{method="GET",path="/api/v1/ping"} 3`,
		"# TYPE xianzhi_process_uptime_seconds gauge",
	}
	for _, expected := range expectations {
		if !strings.Contains(output, expected) {
			t.Fatalf("metrics output missing %q\noutput:\n%s", expected, output)
		}
	}
}

func TestHTTPEscapeLabelValue(t *testing.T) {
	cases := map[string]string{
		`plain`:      `plain`,
		`back\slash`: `back\\slash`,
		`quo"te`:     `quo\"te`,
		"new\nline":  `new\nline`,
	}
	for input, want := range cases {
		if got := escapeLabelValue(input); got != want {
			t.Fatalf("escapeLabelValue(%q) = %q, want %q", input, got, want)
		}
	}
}
