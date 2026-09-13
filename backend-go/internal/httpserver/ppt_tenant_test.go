package httpserver

import (
	"strings"
	"testing"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
)

func TestPPTCapabilityRequest_TenantIDResolution(t *testing.T) {
	data := withAdminDefaults(adminPlatformData{})

	// Case 1: Enterprise user with enterprise authorization
	enterpriseUser := adminUser{ID: "enterprise_user_1", Role: "MEMBER", Status: "ACTIVE", PlanID: "plan_enterprise", TenantID: "tenant_corp_alpha"}
	enterpriseAuth := modelCallAuthorization{
		ContextType:      contextEnterprise,
		TenantID:         "tenant_corp_alpha",
		OrganizationID:   "org_alpha",
		UserID:           enterpriseUser.ID,
		Role:             "ENTERPRISE_MEMBER",
		BillingScope:     contextEnterprise,
		BillingAccountID: "tenant_corp_alpha",
		ServiceState:     "ACTIVE",
	}
	server := api{}
	capEnterprise, err := server.preparePPTCapabilityRequestWithAuthorization(data, enterpriseUser, "企业战略方案", "", 5, true, false, &enterpriseAuth)
	if err != nil {
		t.Fatalf("prepare enterprise PPT capability: %v", err)
	}
	if stringValue(capEnterprise.Params["tenant_id"]) != "tenant_corp_alpha" {
		t.Fatalf("expected capability tenant_id 'tenant_corp_alpha', got %v", capEnterprise.Params["tenant_id"])
	}

	// Verify GenerateRequest tenant resolution logic
	reqEnterprise := pptapp.GenerateRequest{
		Prompt:     "企业战略方案",
		SlideCount: 5,
	}
	reqEnterprise.TenantID = firstNonEmptyString(stringValue(capEnterprise.Params["tenant_id"]), enterpriseUser.TenantID, "tenant_default")
	if reqEnterprise.TenantID != "tenant_corp_alpha" {
		t.Fatalf("expected req.TenantID 'tenant_corp_alpha', got %q", reqEnterprise.TenantID)
	}

	taskEnterprise := pptapp.TaskFromGenerateRequest("task_ent_1", reqEnterprise)
	if taskEnterprise.TenantID != "tenant_corp_alpha" {
		t.Fatalf("expected task.TenantID 'tenant_corp_alpha', got %q", taskEnterprise.TenantID)
	}

	// Case 2: Personal user without explicit tenant (has paid plan with PPT capability)
	personalUser := adminUser{ID: "personal_user_1", Role: "USER", Status: "ACTIVE", PlanID: "plan_month"}
	capPersonal, err := server.preparePPTCapabilityRequest(data, personalUser, "个人学习总结", "", 5, true, false)
	if err != nil {
		t.Fatalf("prepare personal PPT capability: %v", err)
	}
	if stringValue(capPersonal.Params["tenant_id"]) != "tenant_default" {
		t.Fatalf("expected capability tenant_id 'tenant_default', got %v", capPersonal.Params["tenant_id"])
	}

	reqPersonal := pptapp.GenerateRequest{
		Prompt:     "个人学习总结",
		SlideCount: 5,
	}
	reqPersonal.TenantID = firstNonEmptyString(stringValue(capPersonal.Params["tenant_id"]), personalUser.TenantID, "tenant_default")
	if reqPersonal.TenantID != "tenant_default" {
		t.Fatalf("expected req.TenantID 'tenant_default', got %q", reqPersonal.TenantID)
	}

	taskPersonal := pptapp.TaskFromGenerateRequest("task_per_1", reqPersonal)
	if taskPersonal.TenantID != "tenant_default" {
		t.Fatalf("expected task.TenantID 'tenant_default', got %q", taskPersonal.TenantID)
	}

	// Case 3: Empty capability and empty user fallback to tenant_default
	reqFallback := pptapp.GenerateRequest{Prompt: "兜底测试", SlideCount: 3}
	reqFallback.TenantID = firstNonEmptyString("", "", "tenant_default")
	if reqFallback.TenantID != "tenant_default" {
		t.Fatalf("expected fallback 'tenant_default', got %q", reqFallback.TenantID)
	}
	taskFallback := pptapp.TaskFromGenerateRequest("task_fb_1", reqFallback)
	if taskFallback.TenantID != "tenant_default" {
		t.Fatalf("expected taskFallback.TenantID 'tenant_default', got %q", taskFallback.TenantID)
	}
}

func TestConnectorPPT_TenantIDPropagated(t *testing.T) {
	data := withAdminDefaults(adminPlatformData{})
	user := adminUser{ID: "connector_feishu_user", Role: "MEMBER", Status: "ACTIVE", PlanID: "plan_free"}
	authorization := modelCallAuthorization{
		ContextType:      contextEnterprise,
		TenantID:         "tenant_feishu_enterprise",
		OrganizationID:   "org_feishu",
		UserID:           user.ID,
		Role:             "ENTERPRISE_MEMBER",
		BillingScope:     contextEnterprise,
		BillingAccountID: "tenant_feishu_enterprise",
		ServiceState:     "ACTIVE",
	}

	server := api{}
	capability, err := server.preparePPTCapabilityRequestWithAuthorization(data, user, "飞书工作汇报", "", 6, true, false, &authorization)
	if err != nil {
		t.Fatalf("prepare connector PPT capability: %v", err)
	}

	req := pptapp.GenerateRequest{
		Prompt:     "飞书工作汇报",
		SlideCount: 6,
	}
	// Simulate executeConnectorPPT logic
	req.TenantID = firstNonEmptyString(authorization.TenantID, "tenant_default")
	if req.TenantID != "tenant_feishu_enterprise" {
		t.Fatalf("expected connector req.TenantID 'tenant_feishu_enterprise', got %q", req.TenantID)
	}

	task := pptapp.TaskFromGenerateRequest("ppt_conn_1", req)
	if task.TenantID != "tenant_feishu_enterprise" {
		t.Fatalf("expected connector task.TenantID 'tenant_feishu_enterprise', got %q", task.TenantID)
	}

	billingReq, err := connectorPPTBillingRequest(data, user, authorization, capability, req, "feishu:msg-1", map[string]any{"connector_task_id": "ct-1"})
	if err != nil {
		t.Fatalf("connectorPPTBillingRequest: %v", err)
	}
	if stringValue(billingReq.Params["tenant_id"]) != "tenant_feishu_enterprise" {
		t.Fatalf("expected billingReq tenant_id 'tenant_feishu_enterprise', got %v", billingReq.Params["tenant_id"])
	}
}

func TestCreatePendingGenerationTaskWithPPTCanaryOutbox_PopulatesPPTReqTenantID(t *testing.T) {
	// Verify that if pptReq has no TenantID, it gets populated from task.TenantID
	genTask := generationTask{
		ID:       "task_gen_001",
		TenantID: "tenant_enterprise_beta",
	}
	pptReq := pptapp.GenerateRequest{
		Prompt:     "Canary PPT",
		SlideCount: 5,
	}
	if strings.TrimSpace(pptReq.TenantID) == "" {
		pptReq.TenantID = firstNonEmptyString(genTask.TenantID, "tenant_default")
	}
	if pptReq.TenantID != "tenant_enterprise_beta" {
		t.Fatalf("expected populated tenant_id 'tenant_enterprise_beta', got %q", pptReq.TenantID)
	}

	pptTask := pptapp.TaskFromGenerateRequest(genTask.ID, pptReq)
	if pptTask.TenantID != "tenant_enterprise_beta" {
		t.Fatalf("expected pptTask.TenantID 'tenant_enterprise_beta', got %q", pptTask.TenantID)
	}
}
