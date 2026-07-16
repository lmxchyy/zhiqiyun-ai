package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func newAuthFlowTestServerWithSessions(t *testing.T, dataPath string, sessions authSessionStore) http.Handler {
	t.Helper()
	return newWithStoreAndSessions(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()}, newJSONStore(dataPath), sessions).Handler
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

func TestSMSCodeUsesSharedSessionStoreAcrossAuthAPIInstances(t *testing.T) {
	t.Setenv("XIANZHI_ENV", "development")
	t.Setenv("XIANZHI_SMS_DEV_CODE", "123456")
	dataPath := filepath.Join(t.TempDir(), "store.json")
	sessions := newLocalAuthSessions()
	sender := newAuthFlowTestServerWithSessions(t, dataPath, sessions)
	verifier := newAuthFlowTestServerWithSessions(t, dataPath, sessions)

	sendTestSMS(t, sender, "13500001111")
	login := loginTestSMS(t, verifier, "13500001111", "")
	if !login.IsNewUser || login.User.MobileMasked != "135****1111" {
		t.Fatalf("sms login through shared session store = %+v", login)
	}
}

func TestPhoneWechatIdentityConflictRequiresManualMerge(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	phoneUser, err := store.CreateAdminCustomer(adminCustomerMutation{
		Name: "Phone User", Email: "phone-user@example.test", Mobile: "13500002222", Role: "MEMBER", Status: "ACTIVE",
	})
	if err != nil {
		t.Fatal(err)
	}
	wechatUser, err := store.CreateAdminCustomer(adminCustomerMutation{
		Name: "WeChat User", Email: "wechat-user@example.test", WeChatOpenID: "openid-conflict", Role: "MEMBER", Status: "ACTIVE",
	})
	if err != nil {
		t.Fatal(err)
	}
	if phoneUser.ID == wechatUser.ID {
		t.Fatal("test setup requires distinct users")
	}
	auth := newAuthAPI(store, newLocalAuthSessions())
	_, _, _, _, err = auth.userForPhoneIdentity("13500002222", wechatMiniProgramSession{OpenID: "openid-conflict"}, authRegistrationInput{})
	var flowErr *authFlowError
	if !errors.As(err, &flowErr) {
		t.Fatalf("expected auth flow conflict, got %v", err)
	}
	if flowErr.code != "AUTH_ACCOUNT_MERGE_REQUIRED" || flowErr.status != http.StatusConflict {
		t.Fatalf("conflict = status %d code %s, want %d AUTH_ACCOUNT_MERGE_REQUIRED", flowErr.status, flowErr.code, http.StatusConflict)
	}
	mergeRequestID, _ := flowErr.details["mergeRequestId"].(string)
	if mergeRequestID == "" {
		t.Fatalf("conflict did not include merge request id: %+v", flowErr.details)
	}
	requests, err := store.ListAdminAuthMergeRequests(phoneUser.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || requests[0].ID != mergeRequestID || requests[0].SecondaryUserID != wechatUser.ID {
		t.Fatalf("merge request not persisted: %+v", requests)
	}
}

func TestBindMobileUpdatesCurrentUserAndRejectsUsedMobile(t *testing.T) {
	t.Setenv("XIANZHI_ENV", "development")
	t.Setenv("XIANZHI_SMS_DEV_CODE", "123456")
	dataPath := filepath.Join(t.TempDir(), "store.json")
	store := newJSONStore(dataPath)
	if _, err := store.CreateAdminCustomer(adminCustomerMutation{
		Name: "Bound Phone User", Email: "bound-phone@example.test", Mobile: "13600003333", Role: "MEMBER", Status: "ACTIVE",
	}); err != nil {
		t.Fatal(err)
	}
	handler := newAuthFlowTestServer(t, dataPath)
	login := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"account":"agent1@xianzhi.ai","password":"Agent123!"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", login.Code, login.Body.String())
	}
	var loginBody struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(login.Body).Decode(&loginBody); err != nil {
		t.Fatal(err)
	}

	sendTestSMS(t, handler, "13500004444")
	bind := authedRequest(t, handler, http.MethodPost, "/api/v1/auth/mobile/bind", bytes.NewBufferString(`{"mobile":"13500004444","smsCode":"123456"}`), loginBody.AccessToken)
	if bind.Code != http.StatusOK || !strings.Contains(bind.Body.String(), `"mobileMasked":"135****4444"`) {
		t.Fatalf("bind mobile status = %d, body = %s", bind.Code, bind.Body.String())
	}

	sendTestSMS(t, handler, "13600003333")
	conflict := authedRequest(t, handler, http.MethodPost, "/api/v1/auth/mobile/bind", bytes.NewBufferString(`{"mobile":"13600003333","smsCode":"123456"}`), loginBody.AccessToken)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "AUTH_MOBILE_ALREADY_BOUND") {
		t.Fatalf("bound mobile conflict = %d, body = %s", conflict.Code, conflict.Body.String())
	}
}

func TestSMSLoginUsesConfiguredNewcomerPlan(t *testing.T) {
	t.Setenv("XIANZHI_ENV", "development")
	t.Setenv("XIANZHI_SMS_DEV_CODE", "123456")
	dataPath := filepath.Join(t.TempDir(), "store.json")
	store := newJSONStore(dataPath)
	grantPoints, durationDays, concurrency, active := 321, 14, 2, true
	if _, err := store.UpdateAdminPlan("plan_free", adminPlanMutation{
		GrantPoints: &grantPoints, DurationDays: &durationDays, Concurrency: &concurrency, Active: &active,
	}); err != nil {
		t.Fatal(err)
	}

	handler := newAuthFlowTestServer(t, dataPath)
	sendTestSMS(t, handler, "13600003333")
	created := loginTestSMS(t, handler, "13600003333", "")
	if !created.IsNewUser {
		t.Fatalf("expected new user response: %+v", created)
	}

	data, err := newJSONStore(dataPath).AdminData()
	if err != nil {
		t.Fatal(err)
	}
	user, ok := findUserByMobile(data.Users, "13600003333")
	if !ok {
		t.Fatal("configured newcomer user was not persisted")
	}
	account := pointMap(data.PointAccounts)[user.ID]
	if account.Available != grantPoints {
		t.Fatalf("newcomer points = %d, want %d", account.Available, grantPoints)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, user.SubscriptionExpiresAt)
	if err != nil {
		t.Fatalf("subscription expiry = %q: %v", user.SubscriptionExpiresAt, err)
	}
	remaining := time.Until(expiresAt)
	if remaining < 13*24*time.Hour || remaining > 15*24*time.Hour {
		t.Fatalf("subscription remaining = %s, want about 14 days", remaining)
	}
	plan := configuredNewcomerPlan(data.Plans)
	if planPoints(plan) != grantPoints || plan.DurationDays != durationDays || plan.Concurrency != concurrency {
		t.Fatalf("configured plan was not preserved after reload: %+v", plan)
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
