package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

func TestPromotionRecordsExcludeSeedReferralWithoutRegistrationTime(t *testing.T) {
	handler := newPromotionAPITestHandler(t)
	agentToken := loginToken(t, handler, "agent@example.com", "Agent123!")

	records := authedRequest(t, handler, http.MethodGet, "/api/v1/promotion/records?page=1&pageSize=10", nil, agentToken)
	if records.Code != http.StatusOK {
		t.Fatalf("promotion records status = %d %s", records.Code, records.Body.String())
	}
	var payload struct {
		Items []promotionRecord `json:"items"`
		Total int               `json:"total"`
	}
	decodePromotionResponse(t, records.Body, &payload)
	if payload.Total != 0 || len(payload.Items) != 0 {
		t.Fatalf("seed referral without registration time must not be reported: %+v", payload)
	}
}

func TestPromotionCenterRoleTemplatesCodeAndAttribution(t *testing.T) {
	t.Setenv("WECHAT_MINI_PROGRAM_APPID", "")
	t.Setenv("WECHAT_MINI_PROGRAM_SECRET", "")
	handler := newPromotionAPITestHandler(t)
	agentToken := loginToken(t, handler, "agent@example.com", "Agent123!")
	inviteeToken := loginToken(t, handler, "invitee@example.com", "Demo123!")
	otherToken := loginToken(t, handler, "other@example.com", "Agent123!")

	profile := authedRequest(t, handler, http.MethodGet, "/api/v1/promotion/profile", nil, agentToken)
	if profile.Code != http.StatusOK || !strings.Contains(profile.Body.String(), `"currentRole":"USER"`) || !strings.Contains(profile.Body.String(), `"inviteCode":"ZQ`) {
		t.Fatalf("USER promotion profile = %d %s", profile.Code, profile.Body.String())
	}
	switchRole := authedRequest(t, handler, http.MethodPost, "/api/v1/user/current-role", bytes.NewBufferString(`{"role":"AGENT"}`), agentToken)
	if switchRole.Code != http.StatusOK {
		t.Fatalf("switch AGENT = %d %s", switchRole.Code, switchRole.Body.String())
	}
	profile = authedRequest(t, handler, http.MethodGet, "/api/v1/promotion/profile", nil, agentToken)
	if profile.Code != http.StatusOK || !strings.Contains(profile.Body.String(), `"inviteCode":"AGENT-A"`) || !strings.Contains(profile.Body.String(), `"roleLabel":"推广伙伴"`) {
		t.Fatalf("AGENT promotion profile = %d %s", profile.Code, profile.Body.String())
	}
	templates := authedRequest(t, handler, http.MethodGet, "/api/v1/promotion/poster-templates", nil, agentToken)
	if templates.Code != http.StatusOK || !strings.Contains(templates.Body.String(), `"total":10`) || !strings.Contains(templates.Body.String(), `poster.partner.recruit`) {
		t.Fatalf("promotion templates = %d %s", templates.Code, templates.Body.String())
	}
	code := authedRequest(t, handler, http.MethodPost, "/api/v1/promotion/miniprogram-code", bytes.NewBufferString(`{"templateId":"poster.brand.simple"}`), agentToken)
	if code.Code != http.StatusOK || !strings.Contains(code.Body.String(), `"isPlaceholder":true`) || !strings.Contains(code.Body.String(), `data:image/png;base64,`) {
		t.Fatalf("development mini program code = %d %s", code.Code, code.Body.String())
	}
	var codePayload struct {
		Scene       string `json:"scene"`
		InviteToken string `json:"inviteToken"`
	}
	decodePromotionResponse(t, code.Body, &codePayload)
	if !strings.HasPrefix(codePayload.Scene, "inv_") || codePayload.Scene != codePayload.InviteToken || len(codePayload.Scene) > 32 || strings.Contains(codePayload.Scene, "AGENT-A") {
		t.Fatalf("mini program scene must be an opaque short invite token: %+v", codePayload)
	}
	resolved := request(t, handler, http.MethodGet, "/api/v1/invite/resolve?inviteToken="+codePayload.InviteToken, nil)
	if resolved.Code != http.StatusOK || !strings.Contains(resolved.Body.String(), `"valid":true`) || !strings.Contains(resolved.Body.String(), `"identityType":"AGENT"`) || !strings.Contains(resolved.Body.String(), `"inviteCode":"AGENT-A"`) {
		t.Fatalf("invite token resolve = %d %s", resolved.Code, resolved.Body.String())
	}

	visitBody := bytes.NewBufferString(`{"inviteToken":"` + codePayload.InviteToken + `","source":"poster","templateId":"poster.brand.simple"}`)
	visit := authedRequest(t, handler, http.MethodPost, "/api/v1/promotion/visit", visitBody, inviteeToken)
	if visit.Code != http.StatusOK || !strings.Contains(visit.Body.String(), `"status":"visited"`) {
		t.Fatalf("promotion visit = %d %s", visit.Code, visit.Body.String())
	}
	bind := authedRequest(t, handler, http.MethodPost, "/api/v1/promotion/bind", bytes.NewBufferString(`{"inviteToken":"`+codePayload.InviteToken+`","source":"poster","templateId":"poster.brand.simple"}`), inviteeToken)
	if bind.Code != http.StatusOK || !strings.Contains(bind.Body.String(), `"bound":true`) || !strings.Contains(bind.Body.String(), `"status":"registered"`) {
		t.Fatalf("promotion bind = %d %s", bind.Code, bind.Body.String())
	}
	repeatedBind := authedRequest(t, handler, http.MethodPost, "/api/v1/promotion/bind", bytes.NewBufferString(`{"inviteToken":"`+codePayload.InviteToken+`","source":"poster","templateId":"poster.brand.simple"}`), inviteeToken)
	if repeatedBind.Code != http.StatusOK || !strings.Contains(repeatedBind.Body.String(), `"bound":true`) {
		t.Fatalf("idempotent promotion bind = %d %s", repeatedBind.Code, repeatedBind.Body.String())
	}

	records := authedRequest(t, handler, http.MethodGet, "/api/v1/promotion/records?page=1&pageSize=10", nil, agentToken)
	if records.Code != http.StatusOK || !strings.Contains(records.Body.String(), `Invitee`) || !strings.Contains(records.Body.String(), `"total":1`) {
		t.Fatalf("promotion records = %d %s", records.Code, records.Body.String())
	}
	analytics := authedRequest(t, handler, http.MethodGet, "/api/v1/promotion/analytics?days=7", nil, agentToken)
	if analytics.Code != http.StatusOK || !strings.Contains(analytics.Body.String(), `"registerCount":1`) || !strings.Contains(analytics.Body.String(), `"label":"推广海报"`) {
		t.Fatalf("promotion analytics = %d %s", analytics.Code, analytics.Body.String())
	}

	overwrite := authedRequest(t, handler, http.MethodPost, "/api/v1/promotion/bind", bytes.NewBufferString(`{"inviteCode":"AGENT-B"}`), inviteeToken)
	if overwrite.Code != http.StatusConflict || !strings.Contains(overwrite.Body.String(), "already exists") {
		t.Fatalf("promotion overwrite = %d %s", overwrite.Code, overwrite.Body.String())
	}
	selfInvite := authedRequest(t, handler, http.MethodPost, "/api/v1/promotion/bind", bytes.NewBufferString(`{"inviteCode":"AGENT-B"}`), otherToken)
	if selfInvite.Code != http.StatusConflict || !strings.Contains(selfInvite.Body.String(), "invite themselves") {
		t.Fatalf("promotion self invite = %d %s", selfInvite.Code, selfInvite.Body.String())
	}
}

type promotionCodeRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn promotionCodeRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestWechatMiniProgramCodeRejectsJSONErrorWithImageContentType(t *testing.T) {
	t.Setenv("WECHAT_MINI_PROGRAM_APPID", "test-appid")
	t.Setenv("WECHAT_MINI_PROGRAM_SECRET", "test-secret")
	service := newWechatMiniProgramCodeService(config.Config{Environment: "production"})
	service.client = &http.Client{Transport: promotionCodeRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `{"access_token":"test-token","expires_in":7200}`
		contentType := "application/json"
		if strings.Contains(request.URL.Path, "getwxacodeunlimit") {
			body = `{"errcode":41030,"errmsg":"invalid page hint: [test]"}`
			contentType = "image/png"
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{contentType}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}

	_, _, err := service.Generate("inv_a1234567890abcd", defaultPromotionPage)
	if err == nil || !strings.Contains(err.Error(), "41030") || !strings.Contains(err.Error(), "invalid page") {
		t.Fatalf("wechat JSON error was not preserved: %v", err)
	}
}

func TestWechatMiniProgramCodeAcceptsJPEG(t *testing.T) {
	t.Setenv("WECHAT_MINI_PROGRAM_APPID", "test-appid")
	t.Setenv("WECHAT_MINI_PROGRAM_SECRET", "test-secret")
	jpegBody := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0xff, 0xd9}
	service := newWechatMiniProgramCodeService(config.Config{Environment: "production"})
	service.client = &http.Client{Transport: promotionCodeRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `{"access_token":"test-token","expires_in":7200}`
		contentType := "application/json"
		var reader io.Reader = strings.NewReader(body)
		if strings.Contains(request.URL.Path, "getwxacodeunlimit") {
			contentType = "image/jpeg"
			reader = bytes.NewReader(jpegBody)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{contentType}},
			Body:       io.NopCloser(reader),
		}, nil
	})}

	got, placeholder, err := service.Generate("inv_a1234567890abcd", defaultPromotionPage)
	if err != nil {
		t.Fatalf("Generate jpeg: %v", err)
	}
	if placeholder {
		t.Fatalf("expected official jpeg code, got placeholder")
	}
	if !bytes.Equal(got, jpegBody) {
		t.Fatalf("unexpected jpeg body: %x", got)
	}
	if media := promotionMiniProgramImageMediaType(got); media != "image/jpeg" {
		t.Fatalf("media type = %q", media)
	}
}

func TestPromotionInviteTokenRecordRejectsExpiredAndInactive(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Minute)
	if err := promotionInviteTokenRecordError(promotionInviteTokenRecord{Status: "ACTIVE", ExpiresAt: &expiredAt}, now); !errors.Is(err, errPromotionInviteTokenExpired) {
		t.Fatalf("expired token error = %v", err)
	}
	if err := promotionInviteTokenRecordError(promotionInviteTokenRecord{Status: "DISABLED"}, now); !errors.Is(err, errPromotionInviteTokenInactive) {
		t.Fatalf("inactive token error = %v", err)
	}
	if err := promotionInviteTokenRecordError(promotionInviteTokenRecord{Status: "ACTIVE"}, now); err != nil {
		t.Fatalf("active token error = %v", err)
	}
}

func TestPromotionCodeFailsClosedInProductionWithoutWechatCredentials(t *testing.T) {
	t.Setenv("WECHAT_MINI_PROGRAM_APPID", "")
	t.Setenv("WECHAT_MINI_PROGRAM_SECRET", "")
	handler := newPromotionAPITestHandlerWithConfig(t, config.Config{Environment: "production"})
	token := loginToken(t, handler, "agent@example.com", "Agent123!")
	res := authedRequest(t, handler, http.MethodPost, "/api/v1/promotion/miniprogram-code", bytes.NewBufferString(`{"templateId":"poster.brand.simple"}`), token)
	if res.Code != http.StatusServiceUnavailable || !strings.Contains(res.Body.String(), "credentials are missing") {
		t.Fatalf("production code fallback = %d %s", res.Code, res.Body.String())
	}
}

func TestOperationCenterPromotionCodeUsesOpaqueInviteToken(t *testing.T) {
	t.Setenv("XIANZHI_ENV", "development")
	t.Setenv("XIANZHI_SMS_DEV_CODE", "123456")
	t.Setenv("WECHAT_MINI_PROGRAM_APPID", "")
	t.Setenv("WECHAT_MINI_PROGRAM_SECRET", "")
	handler := newPromotionAPITestHandler(t)
	token := loginToken(t, handler, "center@example.com", "Demo123!")
	switchRole := authedRequest(t, handler, http.MethodPost, "/api/v1/user/current-role", bytes.NewBufferString(`{"role":"OPERATION"}`), token)
	if switchRole.Code != http.StatusOK {
		t.Fatalf("switch OPERATION = %d %s", switchRole.Code, switchRole.Body.String())
	}
	response := authedRequest(t, handler, http.MethodPost, "/api/v1/promotion/miniprogram-code", bytes.NewBufferString(`{"templateId":"poster.brand.simple"}`), token)
	var codePayload struct {
		Scene       string `json:"scene"`
		InviteToken string `json:"inviteToken"`
	}
	decodePromotionResponse(t, response.Body, &codePayload)
	if !strings.HasPrefix(codePayload.Scene, "inv_o") || codePayload.Scene != codePayload.InviteToken {
		t.Fatalf("operation center scene = %+v", codePayload)
	}
	resolved := request(t, handler, http.MethodGet, "/api/v1/invite/resolve?inviteToken="+codePayload.InviteToken, nil)
	if resolved.Code != http.StatusOK || !strings.Contains(resolved.Body.String(), `"identityType":"OPERATION_CENTER"`) || !strings.Contains(resolved.Body.String(), `"inviteCode":"CENTER-A"`) {
		t.Fatalf("operation center token resolve = %d %s", resolved.Code, resolved.Body.String())
	}
	sendTestSMS(t, handler, "13800007777")
	registration := request(t, handler, http.MethodPost, "/api/v1/auth/sms/login", bytes.NewBufferString(`{"mobile":"13800007777","smsCode":"123456","inviteToken":"`+codePayload.InviteToken+`","idempotencyKey":"operation-center-token"}`))
	if registration.Code != http.StatusOK || !strings.Contains(registration.Body.String(), `"inviteBindStatus":"bound"`) {
		t.Fatalf("operation center token registration = %d %s", registration.Code, registration.Body.String())
	}
}

func TestPromotionInviteTokenFallsBackWhenSchemaUnavailable(t *testing.T) {
	err := errors.New("ERROR: column \"invite_token\" does not exist (SQLSTATE 42703)")
	if !promotionInviteTokenSchemaUnavailable(err) {
		t.Fatalf("schema unavailable not detected: %v", err)
	}
	if promotionInviteTokenSchemaUnavailable(errors.New("other failure")) {
		t.Fatal("unexpected schema unavailable match")
	}
	store := failingPromotionInviteTokenStore{err: err}
	token, ensureErr := ensurePromotionInviteToken(context.Background(), store, "user_agent_a", roleAgent, "AGENT-A")
	if ensureErr != nil {
		t.Fatalf("ensure fallback error = %v", ensureErr)
	}
	expected := fallbackPromotionInviteToken("user_agent_a", roleAgent, "AGENT-A")
	if token != expected {
		t.Fatalf("ensure fallback token = %s want %s", token, expected)
	}
	data := adminPlatformData{
		Users:         []adminUser{{ID: "user_agent_a", Name: "Agent A", Status: "ACTIVE", TenantID: "tenant_a"}},
		ChannelAgents: []adminChannelAgent{{ID: "channel_a", UserID: "user_agent_a", Status: "ACTIVE", InviteCode: "AGENT-A"}},
	}
	invitation, resolveErr := resolvePromotionInvitation(context.Background(), store, data, token, "")
	if resolveErr != nil {
		t.Fatalf("resolve fallback error = %v", resolveErr)
	}
	if invitation.InviteCode != "AGENT-A" || invitation.IdentityType != roleAgent || invitation.InviterUserID != "user_agent_a" {
		t.Fatalf("resolve fallback invitation = %+v", invitation)
	}
}

type failingPromotionInviteTokenStore struct {
	platformStore
	err error
}

func (s failingPromotionInviteTokenStore) EnsurePromotionInviteToken(context.Context, string, string, string) (string, error) {
	return "", s.err
}

func (s failingPromotionInviteTokenStore) ResolvePromotionInviteToken(context.Context, string) (promotionInviteTokenRecord, error) {
	return promotionInviteTokenRecord{}, s.err
}

func TestPromotionInviteTokenH5Redirect(t *testing.T) {
	handler := newPromotionAPITestHandler(t)
	valid := request(t, handler, http.MethodGet, "/i/inv_a1234567890abcdef", nil)
	if valid.Code != http.StatusFound || valid.Header().Get("Cache-Control") != "no-store" || !strings.Contains(valid.Header().Get("Location"), "WechatLoginPage?inviteToken=inv_a1234567890abcdef") {
		t.Fatalf("token H5 redirect = %d location=%s cache=%s", valid.Code, valid.Header().Get("Location"), valid.Header().Get("Cache-Control"))
	}
	invalid := request(t, handler, http.MethodGet, "/i/not-a-token", nil)
	if invalid.Code != http.StatusNotFound || !strings.Contains(invalid.Body.String(), "邀请链接无效") {
		t.Fatalf("invalid token H5 redirect = %d %s", invalid.Code, invalid.Body.String())
	}
}

func newPromotionAPITestHandler(t *testing.T) http.Handler {
	return newPromotionAPITestHandlerWithConfig(t, config.Config{})
}

func newPromotionAPITestHandlerWithConfig(t *testing.T, cfg config.Config) http.Handler {
	t.Helper()
	dataPath := filepath.Join(t.TempDir(), "store.json")
	raw := `{
		"users":[
			{"id":"user_agent_a","tenantId":"tenant_a","email":"agent@example.com","name":"Agent A","role":"AGENT_L1","agentStatus":"ACTIVE","status":"ACTIVE","planId":"plan_free"},
			{"id":"user_agent_b","tenantId":"tenant_b","email":"other@example.com","name":"Agent B","role":"AGENT_L1","agentStatus":"ACTIVE","status":"ACTIVE","planId":"plan_free"},
			{"id":"user_invitee","tenantId":"tenant_a","email":"invitee@example.com","name":"Invitee","role":"MEMBER","status":"ACTIVE","planId":"plan_free"},
			{"id":"user_demo_seed","tenantId":"tenant_a","email":"seed@example.com","name":"Seed User","role":"MEMBER","status":"ACTIVE","planId":"plan_free","referredBy":"user_agent_a"},
			{"id":"user_center","tenantId":"tenant_a","email":"center@example.com","name":"华东运营中心","role":"OPERATION_CENTER","operationCenterStatus":"ACTIVE","status":"ACTIVE","planId":"plan_free"}
		],
		"channelAgents":[
			{"id":"channel_a","userId":"user_agent_a","level":1,"status":"ACTIVE","inviteCode":"AGENT-A"},
			{"id":"channel_b","userId":"user_agent_b","level":1,"status":"ACTIVE","inviteCode":"AGENT-B"}
		],
		"operationCenters":[
			{"id":"center_a","userId":"user_center","name":"华东运营中心","status":"ACTIVE","inviteCode":"CENTER-A"}
		],
		"orders":[],"commissions":[],"promotionRecords":[],"counters":{}
	}`
	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.Addr = ":0"
	cfg.DataPath = dataPath
	cfg.StaticDir = t.TempDir()
	server := newWithStoreAndSessions(cfg, newJSONStore(dataPath), newLocalAuthSessions())
	return server.Handler
}

func decodePromotionResponse(t *testing.T, responseBody *bytes.Buffer, target any) {
	t.Helper()
	if err := json.NewDecoder(responseBody).Decode(target); err != nil {
		t.Fatal(err)
	}
}
