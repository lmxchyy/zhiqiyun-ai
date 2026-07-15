package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"xianzhi-ai/backend-go/internal/config"
)

func TestEnterpriseCenterLifecycleIsolationAndPermissions(t *testing.T) {
	server := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()})
	handler := server.Handler
	adminToken := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")
	agentToken := loginToken(t, handler, "agent1@xianzhi.ai", "Agent123!")
	operationToken := loginToken(t, handler, "operation@xianzhi.ai", "Demo123!")

	companyA := createEnterpriseForTest(t, handler, adminToken, "知启云测试企业 A")
	if companyA.Context.CurrentRole != roleEnterpriseAdmin || !containsString(companyA.Context.Permissions, "enterprise.member.invite") {
		t.Fatalf("creator context is not enterprise admin: %+v", companyA.Context)
	}
	assertAuthedStatus(t, handler, http.MethodGet, "/api/v1/enterprise/overview", nil, adminToken, http.StatusOK)
	certificationResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/enterprise/certifications", bytes.NewBufferString(`{"legalName":"Xianzhi Test Co., Ltd.","unifiedSocialCreditCode":"91310000TEST000001","documentUrls":["https://example.invalid/license.png"]}`), adminToken)
	if certificationResponse.Code != http.StatusCreated {
		t.Fatalf("submit certification status=%d body=%s", certificationResponse.Code, certificationResponse.Body.String())
	}
	var certification enterpriseCertification
	decodeTestJSON(t, certificationResponse, &certification)
	if certification.TenantID != companyA.Tenant.ID || certification.Status != "PENDING" {
		t.Fatalf("unexpected certification: %+v", certification)
	}

	departmentResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/enterprise/organizations", bytes.NewBufferString(`{"name":"研发部","organizationType":"DEPARTMENT"}`), adminToken)
	if departmentResponse.Code != http.StatusCreated {
		t.Fatalf("create organization status=%d body=%s", departmentResponse.Code, departmentResponse.Body.String())
	}
	var department enterpriseOrganization
	decodeTestJSON(t, departmentResponse, &department)

	invitationResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/enterprise/invitations", bytes.NewBufferString(`{"invitedEmail":"agent1@xianzhi.ai","defaultOrganizationId":"`+department.ID+`","defaultRole":"ENTERPRISE_MEMBER"}`), adminToken)
	if invitationResponse.Code != http.StatusCreated {
		t.Fatalf("create invitation status=%d body=%s", invitationResponse.Code, invitationResponse.Body.String())
	}
	var invitation enterpriseInvitation
	decodeTestJSON(t, invitationResponse, &invitation)

	acceptResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/enterprise/invitations/accept", bytes.NewBufferString(`{"invitationCode":"`+invitation.InvitationCode+`"}`), agentToken)
	if acceptResponse.Code != http.StatusOK {
		t.Fatalf("accept invitation status=%d body=%s", acceptResponse.Code, acceptResponse.Body.String())
	}
	var agentContext enterpriseContext
	decodeTestJSON(t, acceptResponse, &agentContext)
	if agentContext.TenantID != companyA.Tenant.ID || agentContext.CurrentRole != roleEnterpriseMember {
		t.Fatalf("unexpected accepted context: %+v", agentContext)
	}

	// An ordinary enterprise member cannot invite members or assign an administrator role.
	assertAuthedStatus(t, handler, http.MethodPost, "/api/v1/enterprise/invitations", bytes.NewBufferString(`{"defaultRole":"ENTERPRISE_ADMIN"}`), agentToken, http.StatusForbidden)
	assertAuthedStatus(t, handler, http.MethodPost, "/api/v1/enterprise/certifications", bytes.NewBufferString(`{"legalName":"Forbidden","unifiedSocialCreditCode":"FORBIDDEN","documentUrls":["https://example.invalid/forbidden.png"]}`), agentToken, http.StatusForbidden)

	switchEnterpriseContextForTest(t, handler, adminToken, companyA.Tenant.ID, roleEnterpriseAdmin)
	membersResponse := authedRequest(t, handler, http.MethodGet, "/api/v1/enterprise/members", nil, adminToken)
	if membersResponse.Code != http.StatusOK {
		t.Fatalf("list members status=%d body=%s", membersResponse.Code, membersResponse.Body.String())
	}
	var membersPayload struct {
		Items []enterpriseMember `json:"items"`
	}
	decodeTestJSON(t, membersResponse, &membersPayload)
	var agentMember, creatorMember enterpriseMember
	for _, member := range membersPayload.Items {
		if member.UserID == "user_000003" {
			agentMember = member
		}
		if member.UserID == "user_000002" {
			creatorMember = member
		}
	}
	if agentMember.ID == "" || creatorMember.ID == "" {
		t.Fatalf("members missing after invitation: %+v", membersPayload.Items)
	}

	updateRoleResponse := authedRequest(t, handler, http.MethodPatch, "/api/v1/enterprise/members/"+agentMember.ID, bytes.NewBufferString(`{"roles":["AI_ADMIN"],"dataScope":"ORG_AND_CHILDREN"}`), adminToken)
	if updateRoleResponse.Code != http.StatusOK {
		t.Fatalf("assign AI admin status=%d body=%s", updateRoleResponse.Code, updateRoleResponse.Body.String())
	}
	switchEnterpriseContextForTest(t, handler, agentToken, companyA.Tenant.ID, roleAIAdmin)
	assertAuthedStatus(t, handler, http.MethodGet, "/api/v1/enterprise/overview", nil, agentToken, http.StatusOK)
	assertAuthedStatus(t, handler, http.MethodGet, "/api/v1/enterprise/billing/summary", nil, agentToken, http.StatusForbidden)

	// Finance can read billing, but cannot change AI/enterprise role assignments.
	switchEnterpriseContextForTest(t, handler, adminToken, companyA.Tenant.ID, roleEnterpriseAdmin)
	updateRoleResponse = authedRequest(t, handler, http.MethodPatch, "/api/v1/enterprise/members/"+agentMember.ID, bytes.NewBufferString(`{"roles":["FINANCE"],"dataScope":"TENANT_ALL"}`), adminToken)
	if updateRoleResponse.Code != http.StatusOK {
		t.Fatalf("assign finance status=%d body=%s", updateRoleResponse.Code, updateRoleResponse.Body.String())
	}
	switchEnterpriseContextForTest(t, handler, agentToken, companyA.Tenant.ID, roleFinance)
	assertAuthedStatus(t, handler, http.MethodGet, "/api/v1/enterprise/billing/summary", nil, agentToken, http.StatusOK)
	assertAuthedStatus(t, handler, http.MethodPatch, "/api/v1/enterprise/members/"+creatorMember.ID, bytes.NewBufferString(`{"roles":["AI_ADMIN"]}`), agentToken, http.StatusForbidden)

	// Channel and operation identities do not imply enterprise membership.
	assertAuthedStatus(t, handler, http.MethodGet, "/api/v1/enterprise/overview", nil, operationToken, http.StatusForbidden)

	// The same user can own/switch multiple enterprises, and organization data is tenant scoped.
	companyB := createEnterpriseForTest(t, handler, adminToken, "知启云测试企业 B")
	companyC := createEnterpriseForTest(t, handler, adminToken, "知启云测试企业 C")
	contextsResponse := authedRequest(t, handler, http.MethodGet, "/api/v1/user/enterprise-contexts", nil, adminToken)
	var contextsPayload struct {
		Contexts []enterpriseContext `json:"contexts"`
	}
	decodeTestJSON(t, contextsResponse, &contextsPayload)
	enterpriseCount := 0
	for _, item := range contextsPayload.Contexts {
		if item.Type == contextEnterprise {
			enterpriseCount++
		}
	}
	if enterpriseCount < 3 {
		t.Fatalf("expected three enterprise contexts, got %+v", contextsPayload.Contexts)
	}
	switchEnterpriseContextForTest(t, handler, adminToken, companyB.Tenant.ID, roleEnterpriseAdmin)
	treeResponse := authedRequest(t, handler, http.MethodGet, "/api/v1/enterprise/organizations/tree", nil, adminToken)
	if treeResponse.Code != http.StatusOK || bytes.Contains(treeResponse.Body.Bytes(), []byte("研发部")) {
		t.Fatalf("tenant B leaked tenant A organization: status=%d body=%s", treeResponse.Code, treeResponse.Body.String())
	}
	switchEnterpriseContextForTest(t, handler, adminToken, companyA.Tenant.ID, roleEnterpriseAdmin)
	treeResponse = authedRequest(t, handler, http.MethodGet, "/api/v1/enterprise/organizations/tree", nil, adminToken)
	if treeResponse.Code != http.StatusOK || !bytes.Contains(treeResponse.Body.Bytes(), []byte("研发部")) {
		t.Fatalf("tenant A organization missing: status=%d body=%s", treeResponse.Code, treeResponse.Body.String())
	}

	// Disabling a member immediately invalidates the enterprise context and permissions.
	disableResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/enterprise/members/"+agentMember.ID+"/disable", nil, adminToken)
	if disableResponse.Code != http.StatusOK {
		t.Fatalf("disable member status=%d body=%s", disableResponse.Code, disableResponse.Body.String())
	}
	assertAuthedStatus(t, handler, http.MethodGet, "/api/v1/enterprise/overview", nil, agentToken, http.StatusForbidden)
	assertAuthedStatus(t, handler, http.MethodPost, "/api/v1/user/current-context", bytes.NewBufferString(`{"type":"ENTERPRISE","tenantId":"`+companyA.Tenant.ID+`","role":"FINANCE"}`), agentToken, http.StatusForbidden)

	// Creating more tenants did not alter the first enterprise owner or personal/channel APIs.
	_ = companyC
	assertAuthedStatus(t, handler, http.MethodGet, "/api/v1/member/profile", nil, adminToken, http.StatusOK)
	assertAuthedStatus(t, handler, http.MethodGet, "/api/v1/channel/me", nil, agentToken, http.StatusOK)
}

func createEnterpriseForTest(t *testing.T, handler http.Handler, token string, name string) enterpriseCreateResult {
	t.Helper()
	response := authedRequest(t, handler, http.MethodPost, "/api/v1/enterprises", bytes.NewBufferString(`{"name":"`+name+`"}`), token)
	if response.Code != http.StatusCreated {
		t.Fatalf("create enterprise status=%d body=%s", response.Code, response.Body.String())
	}
	var result enterpriseCreateResult
	decodeTestJSON(t, response, &result)
	return result
}

func switchEnterpriseContextForTest(t *testing.T, handler http.Handler, token string, tenantID string, role string) enterpriseContext {
	t.Helper()
	response := authedRequest(t, handler, http.MethodPost, "/api/v1/user/current-context", bytes.NewBufferString(`{"type":"ENTERPRISE","tenantId":"`+tenantID+`","role":"`+role+`"}`), token)
	if response.Code != http.StatusOK {
		t.Fatalf("switch enterprise context status=%d body=%s", response.Code, response.Body.String())
	}
	var result enterpriseContext
	decodeTestJSON(t, response, &result)
	return result
}

func decodeTestJSON(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatal(err)
	}
}
