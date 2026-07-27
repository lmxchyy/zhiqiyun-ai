package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type openIDMatchingContentSecurity struct {
	validOpenID string
	attempts    []string
}

func (s *openIDMatchingContentSecurity) CheckImage(context.Context, []byte, string, string) error {
	return nil
}

func (s *openIDMatchingContentSecurity) CheckText(_ context.Context, _ string, openID string) error {
	s.attempts = append(s.attempts, openID)
	if openID == s.validOpenID {
		return nil
	}
	return errContentSecurityUnavailable
}

func TestMiniProgramTextCheckRequiresOpenID(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/generation-tasks", nil)
	request.Header.Set("X-Client-Platform", "mp-weixin")
	a := api{contentSecurity: &openIDMatchingContentSecurity{validOpenID: "openid-current-app"}}
	err := a.checkMiniProgramText(context.Background(), request, adminUser{ID: "user"}, "合规的创作提示词")
	if !errors.Is(err, errContentSecurityOpenIDRequired) {
		t.Fatalf("expected openid required, got %v", err)
	}
	if err.Error() != "内容安全检测暂不可用，请稍后重试" {
		t.Fatalf("unexpected openid required message: %v", err)
	}
}

func TestMiniProgramTextCheckFallsBackAcrossBoundOpenIDs(t *testing.T) {
	checker := &openIDMatchingContentSecurity{validOpenID: "openid-current-app"}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/generation-tasks", nil)
	request.Header.Set("X-Client-Platform", "mp-weixin")
	a := api{contentSecurity: checker}
	user := adminUser{ID: "user", WeChatOpenIDs: []string{"openid-other-app", "openid-current-app"}}
	if err := a.checkMiniProgramText(context.Background(), request, user, "合规的创作提示词"); err != nil {
		t.Fatalf("valid fallback openid was rejected: %v", err)
	}
	if len(checker.attempts) != 2 || checker.attempts[0] != "openid-other-app" || checker.attempts[1] != "openid-current-app" {
		t.Fatalf("unexpected openid attempts: %#v", checker.attempts)
	}
}

func TestWeChatContentSecurityChecksTextAndImage(t *testing.T) {
	var tokenCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			tokenCalls.Add(1)
			writeJSON(w, map[string]any{"access_token": "wechat-access-token", "expires_in": 7200})
		case "/wxa/img_sec_check":
			if r.URL.Query().Get("access_token") != "wechat-access-token" {
				t.Fatalf("unexpected image access token")
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			file, _, err := r.FormFile("media")
			if err != nil {
				t.Fatal(err)
			}
			file.Close()
			writeJSON(w, map[string]any{"errcode": 0, "errmsg": "ok"})
		case "/wxa/msg_sec_check":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request["openid"] != "openid-test" || request["content"] != "合规的创作提示词" {
				t.Fatalf("unexpected text security request: %#v", request)
			}
			writeJSON(w, map[string]any{"errcode": 0, "errmsg": "ok", "result": map[string]any{"suggest": "pass", "label": 100}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := &wechatContentSecurityService{
		appID: "appid", appSecret: "secret", baseURL: server.URL,
		client: &http.Client{Timeout: time.Second},
	}
	if err := service.CheckImage(context.Background(), []byte("image"), "test.png", "image/png"); err != nil {
		t.Fatalf("image check: %v", err)
	}
	if err := service.CheckText(context.Background(), "合规的创作提示词", "openid-test"); err != nil {
		t.Fatalf("text check: %v", err)
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token endpoint calls = %d, want 1", tokenCalls.Load())
	}
}

func TestWeChatContentSecurityReturnsUniformRiskMessage(t *testing.T) {
	for _, response := range []wechatSecurityResponse{
		{ErrCode: 87014, ErrMsg: "risky content details must not leak"},
		func() wechatSecurityResponse {
			item := wechatSecurityResponse{}
			item.Result.Suggest = "review"
			item.Result.Label = 20002
			return item
		}(),
	} {
		err := contentSecurityResult(response)
		if err == nil || err.Error() != "所发布内容含违规信息" {
			t.Fatalf("risk result = %v", err)
		}
		if (response.ErrMsg != "" && strings.Contains(err.Error(), response.ErrMsg)) || strings.Contains(err.Error(), "20002") {
			t.Fatalf("risk details leaked: %v", err)
		}
	}
}
