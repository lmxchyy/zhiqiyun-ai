package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
	"xianzhi-ai/backend-go/internal/config"
)

type staleTenantKnowledgeRepository struct {
	requestedTenantIDs []string
}

func (r *staleTenantKnowledgeRepository) ResolveAccessContext(_ context.Context, userID string, tenantID string, organizationID string) (knowledgeapp.AccessContext, error) {
	r.requestedTenantIDs = append(r.requestedTenantIDs, tenantID)
	if tenantID != "" {
		return knowledgeapp.AccessContext{}, knowledgeapp.ErrForbidden
	}
	return knowledgeapp.AccessContext{TenantID: "tenant_personal", UserID: userID, Roles: []string{"MEMBER"}}, nil
}

func TestKnowledgeAccessFallsBackFromStaleTenantHeader(t *testing.T) {
	repository := &staleTenantKnowledgeRepository{}
	sessions := newLocalAuthSessions()
	if err := sessions.Put(context.Background(), "member-token", "user-member", time.Hour); err != nil {
		t.Fatal(err)
	}
	a := knowledgeAPI{
		module:   &knowledgeModule{core: knowledgeapp.NewService(repository, nil)},
		sessions: sessions,
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge-agents", nil)
	request.Header.Set("Authorization", "Bearer member-token")
	request.Header.Set("X-Tenant-Id", "tenant-stale")
	request.Header.Set("X-Organization-Id", "organization-stale")
	access, err := a.access(request)
	if err != nil {
		t.Fatal(err)
	}
	if access.TenantID != "tenant_personal" || access.UserID != "user-member" {
		t.Fatalf("unexpected fallback access: %#v", access)
	}
	if strings.Join(repository.requestedTenantIDs, ",") != "tenant-stale," {
		t.Fatalf("tenant resolution attempts = %#v", repository.requestedTenantIDs)
	}
}

func TestKnowledgeAgentHTTPVerticalFlow(t *testing.T) {
	server := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()})
	handler := server.Handler
	token := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

	baseResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/knowledge-bases", bytes.NewBufferString(`{
		"name":"产品知识库","knowledgeType":"PERSONAL","visibility":"PRIVATE"
	}`), token)
	if baseResponse.Code != http.StatusCreated {
		t.Fatalf("create knowledge base = %d %s", baseResponse.Code, baseResponse.Body.String())
	}
	var base knowledgeapp.KnowledgeBase
	if err := json.NewDecoder(baseResponse.Body).Decode(&base); err != nil {
		t.Fatal(err)
	}

	ingestResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/knowledge-bases/"+base.ID+"/documents:ingest", bytes.NewBufferString(`{
		"name":"企业版手册.md","mimeType":"text/markdown","content":"# 成员配额\n企业版默认支持 100 个成员，并支持部门知识库权限隔离。",
		"chunkerKey":"heading","chunkOptions":{"chunkSize":120,"overlap":10}
	}`), token)
	if ingestResponse.Code != http.StatusCreated || !strings.Contains(ingestResponse.Body.String(), `"status":"READY"`) {
		t.Fatalf("ingest document = %d %s", ingestResponse.Code, ingestResponse.Body.String())
	}

	searchResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/knowledge-search", bytes.NewBufferString(`{
		"knowledgeBaseIds":["`+base.ID+`"],"query":"企业版支持多少成员","mode":"HYBRID","topK":5,"threshold":0.05
	}`), token)
	if searchResponse.Code != http.StatusOK || !strings.Contains(searchResponse.Body.String(), "企业版手册.md") {
		t.Fatalf("search knowledge = %d %s", searchResponse.Code, searchResponse.Body.String())
	}

	agentResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/knowledge-agents", bytes.NewBufferString(`{
		"name":"产品顾问","status":"ACTIVE"
	}`), token)
	if agentResponse.Code != http.StatusCreated {
		t.Fatalf("create agent = %d %s", agentResponse.Code, agentResponse.Body.String())
	}
	var agent knowledgeapp.Agent
	if err := json.NewDecoder(agentResponse.Body).Decode(&agent); err != nil {
		t.Fatal(err)
	}

	bindingsResponse := authedRequest(t, handler, http.MethodPut, "/api/v1/knowledge-agents/"+agent.ID+"/knowledge-bindings", bytes.NewBufferString(`{
		"items":[{"knowledgeBaseId":"`+base.ID+`","priority":100,"weight":1,"enabled":true}]
	}`), token)
	if bindingsResponse.Code != http.StatusOK || !strings.Contains(bindingsResponse.Body.String(), base.ID) {
		t.Fatalf("bind knowledge base = %d %s", bindingsResponse.Code, bindingsResponse.Body.String())
	}

	conversationResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/knowledge-conversations", bytes.NewBufferString(`{
		"agentId":"`+agent.ID+`","title":"企业版咨询"
	}`), token)
	if conversationResponse.Code != http.StatusCreated {
		t.Fatalf("create conversation = %d %s", conversationResponse.Code, conversationResponse.Body.String())
	}
	var conversation knowledgeapp.Conversation
	if err := json.NewDecoder(conversationResponse.Body).Decode(&conversation); err != nil {
		t.Fatal(err)
	}

	runResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/knowledge-conversations/"+conversation.ID+"/runs", bytes.NewBufferString(`{
		"question":"企业版支持多少成员？","topK":5,"threshold":0.05,"mode":"HYBRID"
	}`), token)
	if runResponse.Code != http.StatusOK {
		t.Fatalf("run RAG = %d %s", runResponse.Code, runResponse.Body.String())
	}
	var result knowledgeapp.RunResult
	if err := json.NewDecoder(runResponse.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Run.Status != "COMPLETED" || len(result.Citations) == 0 || result.Citations[0].DocumentName != "企业版手册.md" {
		t.Fatalf("unexpected RAG result: %#v", result)
	}

	citationsResponse := authedRequest(t, handler, http.MethodGet, "/api/v1/knowledge-runs/"+result.Run.ID+"/citations", nil, token)
	if citationsResponse.Code != http.StatusOK || !strings.Contains(citationsResponse.Body.String(), "企业版手册.md") {
		t.Fatalf("list citations = %d %s", citationsResponse.Code, citationsResponse.Body.String())
	}

	streamResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/knowledge-conversations/"+conversation.ID+"/runs:stream", bytes.NewBufferString(`{
		"question":"成员配额是什么？","topK":5,"threshold":0.05,"mode":"HYBRID"
	}`), token)
	streamBody := streamResponse.Body.String()
	if streamResponse.Code != http.StatusOK || !strings.Contains(streamBody, "event: answer.delta") || !strings.Contains(streamBody, "event: result") {
		t.Fatalf("stream RAG = %d %s", streamResponse.Code, streamBody)
	}
}

func TestKnowledgeAdminEndpointsRequireAdminAndReturnGovernanceData(t *testing.T) {
	server := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()})
	handler := server.Handler
	memberToken := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")
	memberResponse := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/knowledge/overview", nil, memberToken)
	if memberResponse.Code != http.StatusForbidden {
		t.Fatalf("member admin overview = %d %s", memberResponse.Code, memberResponse.Body.String())
	}
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	overviewResponse := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/knowledge/overview", nil, adminToken)
	if overviewResponse.Code != http.StatusOK || !strings.Contains(overviewResponse.Body.String(), `"knowledgeBaseCount":0`) {
		t.Fatalf("admin overview = %d %s", overviewResponse.Code, overviewResponse.Body.String())
	}
	for _, resource := range []string{"bases", "documents", "chunks", "agents", "parsing-logs", "embedding-profiles", "vector-stores", "ingestion-profiles", "retrieval-profiles", "retrieval-logs", "usage", "hot-questions"} {
		response := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/knowledge/"+resource, nil, adminToken)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items"`) {
			t.Fatalf("admin resource %s = %d %s", resource, response.Code, response.Body.String())
		}
	}
}

func TestSanitizeKnowledgeProfileRecordsRedactsSecrets(t *testing.T) {
	items := sanitizeKnowledgeProfileRecords([]map[string]any{{
		"id": "embedding_openai",
		"config": map[string]any{
			"baseUrl":      "https://example.com/v1",
			"apiKey":       "sk-secret",
			"access_token": "access-secret",
		},
	}})
	payload, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	body := string(payload)
	for _, forbidden := range []string{"sk-secret", "access-secret", `"apiKey":`, `"access_token":`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("profile response leaked %q: %s", forbidden, body)
		}
	}
	for _, expected := range []string{`"baseUrl":"https://example.com/v1"`, `"apiKeyConfigured":true`, `"access_tokenConfigured":true`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("profile response missing %q: %s", expected, body)
		}
	}
}
