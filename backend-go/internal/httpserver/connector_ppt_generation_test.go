package httpserver

import (
	"testing"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
)

func TestConnectorPPTBillingRequestUsesPreparedEnterpriseAuthorization(t *testing.T) {
	data := withAdminDefaults(adminPlatformData{})
	user := adminUser{ID: "connector-user", Role: "MEMBER", Status: "ACTIVE", PlanID: "plan_free"}
	authorization := modelCallAuthorization{
		ContextType: contextEnterprise, TenantID: "tenant-connector", OrganizationID: "organization-connector",
		UserID: user.ID, Role: "ENTERPRISE_MEMBER", BillingScope: contextEnterprise,
		BillingAccountID: "tenant-connector", ServiceState: "ACTIVE",
	}
	generator := api{}
	capability, err := generator.preparePPTCapabilityRequestWithAuthorization(data, user, "新能源汽车行业分析", "", 8, true, false, &authorization)
	if err != nil {
		t.Fatalf("prepare connector PPT capability: %v", err)
	}
	billing, err := connectorPPTBillingRequest(data, user, authorization, capability, pptapp.GenerateRequest{
		Prompt: "新能源汽车行业分析", SlideCount: 8, ImageSource: "ai",
	}, "feishu:message-1", map[string]any{"connector_task_id": "connector-task-1"})
	if err != nil {
		t.Fatalf("build connector PPT billing request: %v", err)
	}
	if billing.ModuleCode != modulePPTGeneration || billing.Type != "PPT_GENERATION" || billing.Model == "" {
		t.Fatalf("billing request=%+v", billing)
	}
	if stringValue(billing.Params["tenant_id"]) != authorization.TenantID || stringValue(billing.Params["organization_id"]) != authorization.OrganizationID {
		t.Fatalf("billing tenant params=%+v", billing.Params)
	}
	if stringValue(billing.Params["billing_scope"]) != contextEnterprise || stringValue(billing.Params["billing_account_id"]) != authorization.TenantID {
		t.Fatalf("billing scope params=%+v", billing.Params)
	}
	if stringValue(billing.Params["billing_type"]) == "" || stringValue(billing.Params["connector_task_id"]) != "connector-task-1" {
		t.Fatalf("billing metadata=%+v", billing.Params)
	}
}
