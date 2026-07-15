package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xianzhi-ai/backend-go/internal/config"
)

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

	visitBody := bytes.NewBufferString(`{"inviteCode":"AGENT-A","source":"poster","templateId":"poster.brand.simple"}`)
	visit := authedRequest(t, handler, http.MethodPost, "/api/v1/promotion/visit", visitBody, inviteeToken)
	if visit.Code != http.StatusOK || !strings.Contains(visit.Body.String(), `"status":"visited"`) {
		t.Fatalf("promotion visit = %d %s", visit.Code, visit.Body.String())
	}
	bind := authedRequest(t, handler, http.MethodPost, "/api/v1/promotion/bind", bytes.NewBufferString(`{"inviteCode":"AGENT-A","source":"poster","templateId":"poster.brand.simple"}`), inviteeToken)
	if bind.Code != http.StatusOK || !strings.Contains(bind.Body.String(), `"bound":true`) || !strings.Contains(bind.Body.String(), `"status":"registered"`) {
		t.Fatalf("promotion bind = %d %s", bind.Code, bind.Body.String())
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
			{"id":"user_invitee","tenantId":"tenant_a","email":"invitee@example.com","name":"Invitee","role":"MEMBER","status":"ACTIVE","planId":"plan_free"}
		],
		"channelAgents":[
			{"id":"channel_a","userId":"user_agent_a","level":1,"status":"ACTIVE","inviteCode":"AGENT-A"},
			{"id":"channel_b","userId":"user_agent_b","level":1,"status":"ACTIVE","inviteCode":"AGENT-B"}
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
