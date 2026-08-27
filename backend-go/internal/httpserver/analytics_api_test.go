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

var analyticsEndpointPaths = []string{
	"/api/v1/admin/analytics/overview",
	"/api/v1/admin/analytics/users",
	"/api/v1/admin/analytics/generation",
	"/api/v1/admin/analytics/tokens",
	"/api/v1/admin/analytics/points",
	"/api/v1/admin/analytics/models",
	"/api/v1/admin/analytics/providers",
	"/api/v1/admin/analytics/trends",
}

func newAnalyticsTestServer(t *testing.T) http.Handler {
	t.Helper()
	dataPath := filepath.Join(t.TempDir(), "store.json")
	store := newJSONStore(dataPath)
	server := newWithStore(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	}, store)
	return server.Handler
}

func TestAnalyticsEndpointsRejectRegularUsers(t *testing.T) {
	handler := newAnalyticsTestServer(t)
	userToken := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

	for _, path := range analyticsEndpointPaths {
		// 未登录请求必须被拒绝
		anonymous := httptest.NewRecorder()
		handler.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, path, nil))
		if anonymous.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s status = %d, want %d", path, anonymous.Code, http.StatusUnauthorized)
		}

		// 普通用户（MEMBER）不能访问 analytics API
		res := authedRequest(t, handler, http.MethodGet, path, nil, userToken)
		if res.Code != http.StatusForbidden && res.Code != http.StatusUnauthorized {
			t.Fatalf("regular user %s status = %d, want 401/403, body = %s", path, res.Code, res.Body.String())
		}
	}
}

func TestAnalyticsEndpointsReturnAggregatesForSuperAdmin(t *testing.T) {
	handler := newAnalyticsTestServer(t)
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	// 每个端点都应返回 200，并且能解析为 JSON 对象
	for _, path := range analyticsEndpointPaths {
		res := authedRequest(t, handler, http.MethodGet, path, nil, adminToken)
		if res.Code != http.StatusOK {
			t.Fatalf("admin %s status = %d, body = %s", path, res.Code, res.Body.String())
		}
		var payload map[string]any
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			t.Fatalf("%s response is not a JSON object: %v", path, err)
		}
	}

	// 概览端点应包含核心卡片字段
	overview := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/analytics/overview", nil, adminToken)
	var overviewPayload map[string]any
	if err := json.NewDecoder(overview.Body).Decode(&overviewPayload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"newUsersToday", "dau", "wau", "mau", "aiUsersToday",
		"imagesGenerated", "videosGenerated", "pointsConsumed", "tokensUsed",
		"failedTasksToday", "successRate",
	} {
		if _, ok := overviewPayload[key]; !ok {
			t.Fatalf("overview missing key %q: %v", key, overviewPayload)
		}
	}

	// days 参数应被接受（1-90 范围）
	for _, days := range []string{"1", "7", "30", "90"} {
		res := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/analytics/trends?days="+days, nil, adminToken)
		if res.Code != http.StatusOK {
			t.Fatalf("trends?days=%s status = %d, body = %s", days, res.Code, res.Body.String())
		}
	}

	// 非法 days 参数不应导致 5xx
	invalid := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/analytics/trends?days=abc", nil, adminToken)
	if invalid.Code >= 500 {
		t.Fatalf("trends?days=abc status = %d, want < 500", invalid.Code)
	}
}

func TestAnalyticsResponsesDoNotLeakSensitiveFields(t *testing.T) {
	handler := newAnalyticsTestServer(t)
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	// V1 安全约束：analytics 只返回聚合结果，
	// 禁止出现 prompt 内容、token 明文、provider secret、API key、手机号等敏感字段。
	forbiddenFields := []string{
		"prompt", "apiKey", "api_key", "secret", "accessToken",
		"mobile", "phoneNumber", "phone_number", "password",
		"emailList", "userEmails", "rawPayload",
	}
	for _, path := range analyticsEndpointPaths {
		res := authedRequest(t, handler, http.MethodGet, path, nil, adminToken)
		if res.Code != http.StatusOK {
			t.Fatalf("admin %s status = %d, body = %s", path, res.Code, res.Body.String())
		}
		body := res.Body.String()
		for _, field := range forbiddenFields {
			if strings.Contains(body, `"`+field+`"`) {
				t.Fatalf("%s leaked forbidden field %q: %s", path, field, body)
			}
		}
	}
}
