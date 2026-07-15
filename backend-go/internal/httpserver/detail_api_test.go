package httpserver

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xianzhi-ai/backend-go/internal/config"
)

func TestMemberDetailAPIsEnforceOwnership(t *testing.T) {
	handler := newDetailAPITestHandler(t)
	memberToken := loginToken(t, handler, "member-a@example.com", "Demo123!")
	otherToken := loginToken(t, handler, "member-b@example.com", "Demo123!")

	assertDetailAPIResponse(t, handler, memberToken, "/api/v1/member/orders", http.StatusOK, "order_a")
	assertDetailAPIResponse(t, handler, memberToken, "/api/v1/member/orders/order_a", http.StatusOK, "order_a")
	assertDetailAPIResponse(t, handler, memberToken, "/api/v1/assets/asset_a", http.StatusOK, "asset_a")
	assertDetailAPIResponse(t, handler, memberToken, "/api/v1/user/usage/usage_a", http.StatusOK, "usage_a")

	assertDetailAPIResponse(t, handler, otherToken, "/api/v1/member/orders/order_a", http.StatusNotFound, "")
	assertDetailAPIResponse(t, handler, otherToken, "/api/v1/assets/asset_a", http.StatusNotFound, "")
	assertDetailAPIResponse(t, handler, otherToken, "/api/v1/user/usage/usage_a", http.StatusNotFound, "")
	assertStatus(t, handler, http.MethodGet, "/api/v1/member/orders/order_a", nil, http.StatusUnauthorized)
}

func TestChannelDetailAPIsEnforceAgentScope(t *testing.T) {
	handler := newDetailAPITestHandler(t)
	agentToken := loginToken(t, handler, "agent@example.com", "Agent123!")
	memberToken := loginToken(t, handler, "member-a@example.com", "Demo123!")

	assertDetailAPIResponse(t, handler, agentToken, "/api/v1/channel/orders/order_a", http.StatusOK, "order_a")
	assertDetailAPIResponse(t, handler, agentToken, "/api/v1/channel/commissions/commission_agent_a", http.StatusOK, "commission_agent_a")
	assertDetailAPIResponse(t, handler, agentToken, "/api/v1/channel/withdrawals/withdrawal_a", http.StatusOK, "withdrawal_a")
	assertDetailAPIResponse(t, handler, agentToken, "/api/v1/channel/children/channel_child", http.StatusOK, "channel_child")
	assertDetailAPIResponse(t, handler, agentToken, "/api/v1/channel/invite-records", http.StatusOK, "user_member_a")

	assertDetailAPIResponse(t, handler, agentToken, "/api/v1/channel/orders/order_b", http.StatusNotFound, "")
	assertDetailAPIResponse(t, handler, memberToken, "/api/v1/channel/orders/order_a", http.StatusForbidden, "")
}

func TestOperationCenterDetailAPIsEnforceCenterScope(t *testing.T) {
	handler := newDetailAPITestHandler(t)
	operationToken := loginToken(t, handler, "operation@example.com", "Demo123!")
	memberToken := loginToken(t, handler, "member-a@example.com", "Demo123!")

	assertDetailAPIResponse(t, handler, operationToken, "/api/v1/operation-center/agents/channel_agent_a", http.StatusOK, "channel_agent_a")
	assertDetailAPIResponse(t, handler, operationToken, "/api/v1/operation-center/orders/order_a", http.StatusOK, "order_a")
	assertDetailAPIResponse(t, handler, operationToken, "/api/v1/operation-center/commissions/commission_operation_a", http.StatusOK, "commission_operation_a")

	assertDetailAPIResponse(t, handler, operationToken, "/api/v1/operation-center/agents/channel_agent_b", http.StatusNotFound, "")
	assertDetailAPIResponse(t, handler, operationToken, "/api/v1/operation-center/orders/order_b", http.StatusNotFound, "")
	assertDetailAPIResponse(t, handler, memberToken, "/api/v1/operation-center/agents/channel_agent_a", http.StatusForbidden, "")
}

func assertDetailAPIResponse(t *testing.T, handler http.Handler, token string, path string, wantStatus int, wantBody string) {
	t.Helper()
	response := authedRequest(t, handler, http.MethodGet, path, nil, token)
	if response.Code != wantStatus {
		t.Fatalf("GET %s status = %d, want %d, body = %s", path, response.Code, wantStatus, response.Body.String())
	}
	if wantBody != "" && !strings.Contains(response.Body.String(), wantBody) {
		t.Fatalf("GET %s body = %s, want it to contain %q", path, response.Body.String(), wantBody)
	}
}

func newDetailAPITestHandler(t *testing.T) http.Handler {
	t.Helper()
	dataPath := filepath.Join(t.TempDir(), "store.json")
	raw := `{
		"users":[
			{"id":"user_member_a","email":"member-a@example.com","name":"Member A","role":"MEMBER","status":"ACTIVE","planId":"plan_month","referredBy":"user_agent"},
			{"id":"user_member_b","email":"member-b@example.com","name":"Member B","role":"MEMBER","status":"ACTIVE","planId":"plan_month","referredBy":"user_agent_b"},
			{"id":"user_agent","email":"agent@example.com","name":"Agent A","role":"AGENT_L1","agentStatus":"ACTIVE","status":"ACTIVE","planId":"plan_free"},
			{"id":"user_child","email":"child@example.com","name":"Child Agent","role":"AGENT_L1","agentStatus":"ACTIVE","status":"ACTIVE","planId":"plan_free"},
			{"id":"user_agent_b","email":"agent-b@example.com","name":"Agent B","role":"AGENT_L1","agentStatus":"ACTIVE","status":"ACTIVE","planId":"plan_free"},
			{"id":"user_operation","email":"operation@example.com","name":"Operation A","role":"OPERATION_CENTER","operationCenterStatus":"ACTIVE","status":"ACTIVE","planId":"plan_free"},
			{"id":"user_operation_b","email":"operation-b@example.com","name":"Operation B","role":"OPERATION_CENTER","operationCenterStatus":"ACTIVE","status":"ACTIVE","planId":"plan_free"}
		],
		"plans":[
			{"id":"plan_free","name":"Free","priceCents":0,"status":"ACTIVE"},
			{"id":"plan_month","name":"Member","priceCents":9900,"grantPoints":1000,"status":"ACTIVE"}
		],
		"orders":[
			{"id":"order_a","orderNo":"NO-A","userId":"user_member_a","planId":"plan_month","amountCents":9900,"status":"PAID","directAgentId":"channel_agent_a","operationCenterId":"operation_a","priceSnapshot":{},"paidAt":"2026-07-10T00:00:00Z","createdAt":"2026-07-10T00:00:00Z"},
			{"id":"order_b","orderNo":"NO-B","userId":"user_member_b","planId":"plan_month","amountCents":9900,"status":"PAID","directAgentId":"channel_agent_b","operationCenterId":"operation_b","priceSnapshot":{},"paidAt":"2026-07-10T00:00:00Z","createdAt":"2026-07-10T00:00:00Z"}
		],
		"channelAgents":[
			{"id":"channel_agent_a","userId":"user_agent","operationCenterId":"operation_a","level":1,"status":"ACTIVE","inviteCode":"AGENT-A","createdAt":"2026-07-01T00:00:00Z"},
			{"id":"channel_child","userId":"user_child","parentId":"channel_agent_a","operationCenterId":"operation_a","level":1,"status":"ACTIVE","inviteCode":"CHILD-A","createdAt":"2026-07-02T00:00:00Z"},
			{"id":"channel_agent_b","userId":"user_agent_b","operationCenterId":"operation_b","level":1,"status":"ACTIVE","inviteCode":"AGENT-B","createdAt":"2026-07-01T00:00:00Z"}
		],
		"operationCenters":[
			{"id":"operation_a","userId":"user_operation","name":"Operation A","inviteCode":"OP-A","status":"ACTIVE","createdAt":"2026-07-01T00:00:00Z"},
			{"id":"operation_b","userId":"user_operation_b","name":"Operation B","inviteCode":"OP-B","status":"ACTIVE","createdAt":"2026-07-01T00:00:00Z"}
		],
		"customerRelations":[
			{"id":"relation_a","customerUserId":"user_member_a","directAgentId":"channel_agent_a","operationCenterId":"operation_a","bindType":"INVITE","status":"ACTIVE","createdAt":"2026-07-01T00:00:00Z"},
			{"id":"relation_b","customerUserId":"user_member_b","directAgentId":"channel_agent_b","operationCenterId":"operation_b","bindType":"INVITE","status":"ACTIVE","createdAt":"2026-07-01T00:00:00Z"}
		],
		"commissions":[
			{"id":"commission_agent_a","orderId":"order_a","agentId":"channel_agent_a","receiverType":"AGENT","receiverId":"channel_agent_a","amountCents":1000,"rate":0.1,"status":"PENDING","settleStatus":"PENDING","ruleSnapshot":{},"createdAt":"2026-07-10T00:00:00Z"},
			{"id":"commission_operation_a","orderId":"order_a","receiverType":"OPERATION_CENTER","receiverId":"operation_a","amountCents":500,"rate":0.05,"status":"PENDING","settleStatus":"PENDING","ruleSnapshot":{},"createdAt":"2026-07-10T00:00:00Z"}
		],
		"withdrawals":[
			{"id":"withdrawal_a","agentId":"channel_agent_a","amountCents":300,"status":"PENDING","createdAt":"2026-07-11T00:00:00Z"}
		],
		"billingEvents":[
			{"id":"usage_a","transactionId":"tx_a","userId":"user_member_a","taskId":"task_a","metricCode":"image.generations","quantity":1,"pointCost":10,"balanceBefore":100,"balanceAfter":90,"model":"mock-standard","status":"SUCCEEDED","occurredAt":"2026-07-10T00:00:00Z","metadata":{}},
			{"id":"usage_b","transactionId":"tx_b","userId":"user_member_b","taskId":"task_b","metricCode":"image.generations","quantity":1,"pointCost":10,"balanceBefore":100,"balanceAfter":90,"model":"mock-standard","status":"SUCCEEDED","occurredAt":"2026-07-10T00:00:00Z","metadata":{}}
		],
		"assets":[
			{"id":"asset_a","userId":"user_member_a","taskId":"task_a","name":"Asset A","mediaType":"image","url":"https://example.com/a.png","favorite":false,"metadata":{},"createdAt":"2026-07-10T00:00:00Z","updatedAt":"2026-07-10T00:00:00Z"},
			{"id":"asset_b","userId":"user_member_b","taskId":"task_b","name":"Asset B","mediaType":"image","url":"https://example.com/b.png","favorite":false,"metadata":{},"createdAt":"2026-07-10T00:00:00Z","updatedAt":"2026-07-10T00:00:00Z"}
		],
		"counters":{}
	}`
	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	server := New(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()})
	return server.Handler
}
