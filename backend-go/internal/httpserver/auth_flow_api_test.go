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

type authFlowTestResponse struct {
	AccessToken      string `json:"accessToken"`
	IsNewUser        bool   `json:"isNewUser"`
	InviteBindStatus string `json:"inviteBindStatus"`
	User             struct {
		ID           string `json:"id"`
		MobileMasked string `json:"mobileMasked"`
		PasswordSet  bool   `json:"passwordSet"`
	} `json:"user"`
}

func newAuthFlowTestServer(t *testing.T, dataPath string) http.Handler {
	t.Helper()
	return New(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()}).Handler
}

func sendTestSMS(t *testing.T, handler http.Handler, mobile string) {
	t.Helper()
	response := request(t, handler, http.MethodPost, "/api/v1/auth/sms/send", bytes.NewBufferString(`{"mobile":"`+mobile+`","purpose":"login"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("send sms status = %d, body = %s", response.Code, response.Body.String())
	}
}

func loginTestSMS(t *testing.T, handler http.Handler, mobile, inviteCode string) authFlowTestResponse {
	t.Helper()
	body := `{"mobile":"` + mobile + `","smsCode":"123456","inviteCode":"` + inviteCode + `","idempotencyKey":"test-key"}`
	response := request(t, handler, http.MethodPost, "/api/v1/auth/sms/login", bytes.NewBufferString(body))
	if response.Code != http.StatusOK {
		t.Fatalf("sms login status = %d, body = %s", response.Code, response.Body.String())
	}
	var result authFlowTestResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestSMSLoginCreatesOnceAndInvalidInviteDoesNotBlock(t *testing.T) {
	t.Setenv("XIANZHI_ENV", "development")
	t.Setenv("XIANZHI_SMS_DEV_CODE", "123456")
	dataPath := filepath.Join(t.TempDir(), "store.json")
	handler := newAuthFlowTestServer(t, dataPath)

	sendTestSMS(t, handler, "138 8888 8888")
	created := loginTestSMS(t, handler, "13888888888", "NOT-VALID")
	if !created.IsNewUser || created.AccessToken == "" || created.User.MobileMasked != "138****8888" {
		t.Fatalf("unexpected new sms login: %+v", created)
	}
	if created.InviteBindStatus != "ignored_invalid" {
		t.Fatalf("invite bind status = %q, want ignored_invalid", created.InviteBindStatus)
	}

	secondHandler := newAuthFlowTestServer(t, dataPath)
	sendTestSMS(t, secondHandler, "13888888888")
	existing := loginTestSMS(t, secondHandler, "13888888888", "ANOTHER-CODE")
	if existing.IsNewUser || existing.User.ID != created.User.ID || existing.InviteBindStatus != "ignored_existing" {
		t.Fatalf("same mobile should reuse account: created=%+v existing=%+v", created, existing)
	}
}

func TestSMSCodeErrorsAndRateLimitUseStableCodes(t *testing.T) {
	t.Setenv("XIANZHI_ENV", "development")
	t.Setenv("XIANZHI_SMS_DEV_CODE", "123456")
	handler := newAuthFlowTestServer(t, filepath.Join(t.TempDir(), "store.json"))
	sendTestSMS(t, handler, "13900001111")

	invalid := request(t, handler, http.MethodPost, "/api/v1/auth/sms/login", bytes.NewBufferString(`{"mobile":"13900001111","smsCode":"654321"}`))
	if invalid.Code != http.StatusUnauthorized || !strings.Contains(invalid.Body.String(), "SMS_CODE_INVALID") {
		t.Fatalf("invalid sms response = %d %s", invalid.Code, invalid.Body.String())
	}
	rateLimited := request(t, handler, http.MethodPost, "/api/v1/auth/sms/send", bytes.NewBufferString(`{"mobile":"13900001111"}`))
	if rateLimited.Code != http.StatusTooManyRequests || !strings.Contains(rateLimited.Body.String(), "SMS_TOO_FREQUENT") {
		t.Fatalf("rate limited response = %d %s", rateLimited.Code, rateLimited.Body.String())
	}
}

func TestPhoneAccountCanSetPasswordWithoutCurrentPassword(t *testing.T) {
	t.Setenv("XIANZHI_ENV", "development")
	t.Setenv("XIANZHI_SMS_DEV_CODE", "123456")
	handler := newAuthFlowTestServer(t, filepath.Join(t.TempDir(), "store.json"))
	sendTestSMS(t, handler, "13700002222")
	login := loginTestSMS(t, handler, "13700002222", "")

	security := authedRequest(t, handler, http.MethodGet, "/api/v1/auth/security", nil, login.AccessToken)
	if security.Code != http.StatusOK || !strings.Contains(security.Body.String(), `"passwordSet":false`) {
		t.Fatalf("initial security = %d %s", security.Code, security.Body.String())
	}
	changed := authedRequest(t, handler, http.MethodPost, "/api/v1/auth/change-password", bytes.NewBufferString(`{"currentPassword":"","newPassword":"Secure123!"}`), login.AccessToken)
	if changed.Code != http.StatusOK || !strings.Contains(changed.Body.String(), `"passwordSet":true`) {
		t.Fatalf("set password = %d %s", changed.Code, changed.Body.String())
	}
	passwordLogin := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"account":"13700002222","password":"Secure123!"}`))
	if passwordLogin.Code != http.StatusOK {
		t.Fatalf("password login after set = %d %s", passwordLogin.Code, passwordLogin.Body.String())
	}
}
