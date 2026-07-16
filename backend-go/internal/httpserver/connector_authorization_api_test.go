package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xianzhi-ai/backend-go/internal/config"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

func TestAuthorizationTicketIsRandomAndStoredAsHash(t *testing.T) {
	first, err := newAuthorizationTicket()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newAuthorizationTicket()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || len(first) < 40 {
		t.Fatalf("authorization tickets must be high entropy: %q %q", first, second)
	}
	if authorizationTicketHash(first) == first || len(authorizationTicketHash(first)) != 64 {
		t.Fatal("authorization session must persist only a SHA-256 ticket digest")
	}
}

func TestAuthorizationHTMLDoesNotInterpolateUntrustedMarkup(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeAuthorizationHTML(recorder, http.StatusBadRequest, `<script>alert(1)</script>`, `<img src=x>`, "")
	body := recorder.Body.String()
	if strings.Contains(body, `<script>alert(1)</script>`) || strings.Contains(body, `<img src=x>`) {
		t.Fatal("authorization result page must escape provider and request messages")
	}
	if !strings.Contains(body, "&lt;script&gt;") || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("authorization result page must be escaped and non-cacheable")
	}
}

func TestFetchFeishuOAuthIdentityDoesNotPersistOrExposeToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/authen/v2/oauth/token":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["client_secret"] != "test-app-secret" || payload["code"] != "oauth-code" {
				t.Fatalf("unexpected token exchange payload: %#v", payload)
			}
			_, _ = w.Write([]byte(`{"code":0,"access_token":"short-lived-user-token"}`))
		case "/authen/v1/user_info":
			if r.Header.Get("Authorization") != "Bearer short-lived-user-token" {
				t.Fatalf("missing OAuth bearer token: %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"open_id":"ou_123","union_id":"on_123","tenant_key":"tenant_external","name":"扫码用户"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cipher, err := storagecenter.NewSecretCipher("connector-authorization-test-master-key-32-bytes")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := cipher.Encrypt("test-app-secret", "connector_test")
	if err != nil {
		t.Fatal(err)
	}
	api := connectorAPI{cfg: config.Config{FeishuAPIBaseURL: server.URL}, cipher: cipher}
	identity, err := api.fetchFeishuOAuthIdentity(context.Background(), enterpriseConnector{ID: "connector_test", AppID: "cli_test", AppSecretEncrypted: encrypted}, "oauth-code", "https://example.test/callback")
	if err != nil {
		t.Fatal(err)
	}
	if identity.ExternalUserID != "ou_123" || identity.ExternalName != "扫码用户" || identity.ExternalTenantKey != "tenant_external" {
		t.Fatalf("unexpected Feishu identity: %#v", identity)
	}
}

func TestFetchWeChatOAuthIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/sns/oauth2/access_token":
			if r.URL.Query().Get("appid") != "wx_test" || r.URL.Query().Get("secret") != "wx_secret" {
				t.Fatal("WeChat credentials were not sent to the server-side token exchange")
			}
			_, _ = w.Write([]byte(`{"access_token":"wechat-token","openid":"openid-1"}`))
		case "/sns/userinfo":
			_, _ = w.Write([]byte(`{"openid":"openid-1","unionid":"unionid-1","nickname":"微信用户","headimgurl":"https://example.test/avatar.png"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	api := connectorAPI{cfg: config.Config{WeChatOpenAppID: "wx_test", WeChatOpenAppSecret: "wx_secret", WeChatOpenAPIBaseURL: server.URL}}
	identity, err := api.fetchWeChatOAuthIdentity(context.Background(), "oauth-code")
	if err != nil {
		t.Fatal(err)
	}
	if identity.ExternalUserID != "openid-1" || identity.ExternalUnionID != "unionid-1" || identity.ExternalName != "微信用户" {
		t.Fatalf("unexpected WeChat identity: %#v", identity)
	}
}
