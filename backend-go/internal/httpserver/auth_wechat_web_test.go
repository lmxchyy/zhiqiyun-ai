package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWeChatWebLoginExistingIdentityIssuesTokenOnlyOnce(t *testing.T) {
	const openID = "wechat-web-openid-demo"
	const unionID = "wechat-web-unionid-demo"
	wechat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sns/oauth2/access_token" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"openid":%q,"unionid":%q,"access_token":"oauth-token"}`, openID, unionID)
	}))
	defer wechat.Close()

	t.Setenv("XIANZHI_ENV", "test")
	t.Setenv("WECHAT_OPEN_APP_ID", "test-app-id")
	t.Setenv("WECHAT_OPEN_APP_SECRET", "test-app-secret")
	t.Setenv("WECHAT_OPEN_API_BASE_URL", wechat.URL)
	t.Setenv("WECHAT_OPEN_AUTHORIZE_BASE_URL", "https://open.weixin.qq.com")
	t.Setenv("WECHAT_OPEN_REDIRECT_URL", "http://localhost/api/v1/auth/wechat/callback")

	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	demo, ok := findUserByEmail(data.Users, "demo@xianzhi.ai")
	if !ok {
		t.Fatal("demo user is missing")
	}
	if _, err := store.UpdateAdminCustomer(demo.ID, adminCustomerMutation{WeChatOpenID: openID, WeChatUnionID: unionID}); err != nil {
		t.Fatal(err)
	}

	auth := newAuthAPI(store, newLocalAuthSessions())
	qrResponse := httptest.NewRecorder()
	auth.wechatWebQRCode(qrResponse, httptest.NewRequest(http.MethodGet, "/api/v1/auth/wechat/qrcode", nil))
	if qrResponse.Code != http.StatusOK {
		t.Fatalf("qrcode status = %d, body = %s", qrResponse.Code, qrResponse.Body.String())
	}
	var qr struct {
		QRCodeID string `json:"qrCodeId"`
		QRURL    string `json:"qrUrl"`
	}
	if err := json.Unmarshal(qrResponse.Body.Bytes(), &qr); err != nil || qr.QRCodeID == "" || !strings.Contains(qr.QRURL, "state=") {
		t.Fatalf("invalid qrcode response: err=%v body=%s", err, qrResponse.Body.String())
	}

	callback := httptest.NewRecorder()
	callbackURL := "/api/v1/auth/wechat/callback?state=" + url.QueryEscape(qr.QRCodeID) + "&code=wechat-code"
	auth.wechatWebCallback(callback, httptest.NewRequest(http.MethodGet, callbackURL, nil))
	if callback.Code != http.StatusOK || !strings.Contains(callback.Body.String(), "扫码成功") {
		t.Fatalf("callback status = %d, body = %s", callback.Code, callback.Body.String())
	}

	statusURL := "/api/v1/auth/wechat/status?qrCodeId=" + url.QueryEscape(qr.QRCodeID)
	status := httptest.NewRecorder()
	auth.wechatWebStatus(status, httptest.NewRequest(http.MethodGet, statusURL, nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"SUCCESS"`) || status.Header().Get("Set-Cookie") == "" {
		t.Fatalf("status = %d, cookie=%q body=%s", status.Code, status.Header().Get("Set-Cookie"), status.Body.String())
	}

	repeat := httptest.NewRecorder()
	auth.wechatWebStatus(repeat, httptest.NewRequest(http.MethodGet, statusURL, nil))
	if repeat.Code != http.StatusGone {
		t.Fatalf("repeat status = %d, want %d; body=%s", repeat.Code, http.StatusGone, repeat.Body.String())
	}
}

func TestLocalWeChatWebLoginTakeIsOneTime(t *testing.T) {
	store := newLocalAuthSessions().(*localAuthSessions)
	session := wechatWebLoginSession{ID: "qr-1", Status: "CONFIRMED", ExpiresAt: time.Now().Add(time.Minute)}
	if err := store.PutWeChatWebLogin(t.Context(), session.ID, session, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.TakeWeChatWebLogin(t.Context(), session.ID); err != nil || !found {
		t.Fatalf("first take: found=%v err=%v", found, err)
	}
	if _, found, err := store.TakeWeChatWebLogin(t.Context(), session.ID); err != nil || found {
		t.Fatalf("second take: found=%v err=%v", found, err)
	}
}

func TestWeChatWebQRCodeRequiresConfiguration(t *testing.T) {
	t.Setenv("WECHAT_OPEN_APP_ID", "")
	t.Setenv("WECHAT_OPEN_APP_SECRET", "")
	t.Setenv("WECHAT_OPEN_REDIRECT_URL", "")
	auth := newAuthAPI(newJSONStore(filepath.Join(t.TempDir(), "store.json")), newLocalAuthSessions())
	response := httptest.NewRecorder()
	auth.wechatWebQRCode(response, httptest.NewRequest(http.MethodGet, "/api/v1/auth/wechat/qrcode", nil))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "WECHAT_WEB_LOGIN_NOT_CONFIGURED") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
