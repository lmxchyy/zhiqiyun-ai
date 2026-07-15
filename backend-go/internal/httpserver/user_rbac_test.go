package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"xianzhi-ai/backend-go/internal/config"
)

func TestUserRBACProfileAndCurrentRole(t *testing.T) {
	server := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()})
	handler := server.Handler

	memberToken := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")
	member := getUserRoleAccess(t, handler, memberToken)
	if len(member.Roles) != 1 || member.Roles[0] != roleUser || member.CurrentRole != roleUser {
		t.Fatalf("unexpected member access: %+v", member)
	}
	if !containsString(member.Permissions, "assets:view") || containsString(member.Permissions, "agent:promotion") {
		t.Fatalf("unexpected member permissions: %+v", member.Permissions)
	}
	denied := authedRequest(t, handler, http.MethodPost, "/api/v1/user/current-role", bytes.NewBufferString(`{"role":"AGENT"}`), memberToken)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("member AGENT switch status = %d, body = %s", denied.Code, denied.Body.String())
	}

	agentToken := loginToken(t, handler, "agent1@xianzhi.ai", "Agent123!")
	agent := getUserRoleAccess(t, handler, agentToken)
	if !containsString(agent.Roles, roleUser) || !containsString(agent.Roles, roleAgent) || containsString(agent.Roles, roleOperation) {
		t.Fatalf("unexpected agent roles: %+v", agent.Roles)
	}
	switched := authedRequest(t, handler, http.MethodPost, "/api/v1/user/current-role", bytes.NewBufferString(`{"role":"AGENT"}`), agentToken)
	if switched.Code != http.StatusOK {
		t.Fatalf("agent role switch status = %d, body = %s", switched.Code, switched.Body.String())
	}
	var switchedAccess userRoleAccess
	if err := json.NewDecoder(switched.Body).Decode(&switchedAccess); err != nil {
		t.Fatal(err)
	}
	if switchedAccess.CurrentRole != roleAgent || !containsString(switchedAccess.Permissions, "agent:promotion") || !containsString(switchedAccess.Permissions, "wallet:view") {
		t.Fatalf("unexpected switched access: %+v", switchedAccess)
	}

	// Switching roles keeps the same access token; the next profile read observes the new role.
	afterSwitch := getUserRoleAccess(t, handler, agentToken)
	if afterSwitch.CurrentRole != roleAgent {
		t.Fatalf("current role not retained for same token: %+v", afterSwitch)
	}

	operationToken := loginToken(t, handler, "operation@xianzhi.ai", "Demo123!")
	operation := getUserRoleAccess(t, handler, operationToken)
	if !containsString(operation.Roles, roleUser) || !containsString(operation.Roles, roleOperation) || containsString(operation.Roles, roleAgent) {
		t.Fatalf("unexpected operation roles: %+v", operation.Roles)
	}
}

func TestUserRBACRequiresAuthentication(t *testing.T) {
	server := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()})
	assertStatus(t, server.Handler, http.MethodGet, "/api/v1/user/profile", nil, http.StatusUnauthorized)
	assertStatus(t, server.Handler, http.MethodPost, "/api/v1/user/current-role", bytes.NewBufferString(`{"role":"USER"}`), http.StatusUnauthorized)
}

func getUserRoleAccess(t *testing.T, handler http.Handler, token string) userRoleAccess {
	t.Helper()
	response := authedRequest(t, handler, http.MethodGet, "/api/v1/user/profile", nil, token)
	if response.Code != http.StatusOK {
		t.Fatalf("user profile status = %d, body = %s", response.Code, response.Body.String())
	}
	var access userRoleAccess
	if err := json.NewDecoder(response.Body).Decode(&access); err != nil {
		t.Fatal(err)
	}
	return access
}
