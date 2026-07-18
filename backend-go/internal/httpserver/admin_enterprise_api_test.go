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

func TestAdminEnterprisePhaseOneFlowAndPrivacyBoundary(t *testing.T) {
	server := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()})
	handler := server.Handler
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	memberToken := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

	forbidden := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/enterprises", nil, memberToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("member enterprise list = %d %s", forbidden.Code, forbidden.Body.String())
	}

	create := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/enterprises", bytes.NewBufferString(`{
		"name":"知启云演示企业",
		"enterpriseCode":"ENT-DEMO-001",
		"planCode":"enterprise_trial",
		"seatLimit":50,
		"industry":"企业服务",
		"companySize":"21-100"
	}`), adminToken)
	if create.Code != http.StatusCreated {
		t.Fatalf("create enterprise = %d %s", create.Code, create.Body.String())
	}
	var created adminEnterpriseDetail
	if err := json.NewDecoder(create.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Enterprise.ID == "" || created.Enterprise.EnterpriseCode != "ENT-DEMO-001" || created.Enterprise.SeatLimit != 50 {
		t.Fatalf("unexpected enterprise: %+v", created.Enterprise)
	}

	list := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/enterprises?keyword=ENT-DEMO-001&page=1&pageSize=20", nil, adminToken)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"enterpriseCode":"ENT-DEMO-001"`) {
		t.Fatalf("list enterprises = %d %s", list.Code, list.Body.String())
	}

	detail := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/enterprises/"+created.Enterprise.ID, nil, adminToken)
	body := detail.Body.String()
	if detail.Code != http.StatusOK || !strings.Contains(body, `"summaryOnly":true`) {
		t.Fatalf("enterprise detail = %d %s", detail.Code, body)
	}
	for _, forbiddenField := range []string{`"knowledgeBaseContent":`, `"originalFileContent":`, `"conversationContent":`, `"promptContent":`, `"aiEmployeeTaskContent":`} {
		if strings.Contains(body, forbiddenField) {
			t.Fatalf("enterprise detail leaked %s: %s", forbiddenField, body)
		}
	}

	export := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/enterprises/export?keyword=ENT-DEMO-001", nil, adminToken)
	if export.Code != http.StatusOK || !strings.Contains(export.Header().Get("Content-Type"), "text/csv") || !strings.Contains(export.Body.String(), "ENT-DEMO-001") {
		t.Fatalf("export enterprises = %d %s", export.Code, export.Body.String())
	}

	for _, section := range []string{"members", "package", "compute", "transactions", "orders", "ai-capabilities", "ai-employees", "knowledge-bases", "integrations", "attribution", "relationships", "risk", "audit-logs"} {
		response := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/enterprises/"+created.Enterprise.ID+"/"+section, nil, adminToken)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"summary"`) || !strings.Contains(response.Body.String(), `"privacy"`) {
			t.Fatalf("enterprise section %s = %d %s", section, response.Code, response.Body.String())
		}
	}

	integrations := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/enterprises/"+created.Enterprise.ID+"/integrations", nil, adminToken)
	integrationBody := integrations.Body.String()
	if integrations.Code != http.StatusOK || !strings.Contains(integrationBody, `"adapterBoundary":"PlatformConnector"`) || !strings.Contains(integrationBody, `"secretsConfigured"`) {
		t.Fatalf("enterprise integrations = %d %s", integrations.Code, integrationBody)
	}
	for _, forbiddenField := range []string{`"connectorKey":`, `"appId":`, `"externalMessageId":`, `"lastErrorMessage":`, `"verificationTokenCiphertext":`, `"encryptKeyCiphertext":`, `"appSecretCiphertext":`} {
		if strings.Contains(integrationBody, forbiddenField) {
			t.Fatalf("enterprise integrations leaked %s: %s", forbiddenField, integrationBody)
		}
	}

	seatRequest := `{"requestId":"req-seat-001","reason":"扩容项目团队","seatLimit":80}`
	seat := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/enterprises/"+created.Enterprise.ID+"/seats/adjust", bytes.NewBufferString(seatRequest), adminToken)
	if seat.Code != http.StatusOK || !strings.Contains(seat.Body.String(), `"seatLimit":80`) {
		t.Fatalf("adjust seats = %d %s", seat.Code, seat.Body.String())
	}
	duplicateSeat := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/enterprises/"+created.Enterprise.ID+"/seats/adjust", bytes.NewBufferString(seatRequest), adminToken)
	if duplicateSeat.Code != http.StatusConflict {
		t.Fatalf("duplicate seat adjustment = %d %s", duplicateSeat.Code, duplicateSeat.Body.String())
	}

	compute := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/enterprises/"+created.Enterprise.ID+"/compute/adjust", bytes.NewBufferString(`{"requestId":"req-compute-001","reason":"活动补偿","pointDelta":1200}`), adminToken)
	if compute.Code != http.StatusOK || !strings.Contains(compute.Body.String(), `"balance":1200`) || !strings.Contains(compute.Body.String(), `"unit":"POINT"`) {
		t.Fatalf("adjust compute = %d %s", compute.Code, compute.Body.String())
	}
	profile := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/enterprises/"+created.Enterprise.ID, bytes.NewBufferString(`{"requestId":"req-profile-001","reason":"企业工商资料更新","name":"演示企业升级版","industry":"人工智能","companySize":"100-499人"}`), adminToken)
	if profile.Code != http.StatusOK || !strings.Contains(profile.Body.String(), `"industry":"人工智能"`) {
		t.Fatalf("update enterprise profile = %d %s", profile.Code, profile.Body.String())
	}
	aiConfigure := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/enterprises/"+created.Enterprise.ID+"/ai-capabilities/configure", bytes.NewBufferString(`{"requestId":"req-ai-001","reason":"开通文本生成能力","moduleCode":"text_generation","modelName":"demo-model","status":"ACTIVE","limits":{"dailyRequests":1000}}`), adminToken)
	if aiConfigure.Code != http.StatusOK || !strings.Contains(aiConfigure.Body.String(), `"moduleCode":"text_generation"`) {
		t.Fatalf("configure enterprise ai capability = %d %s", aiConfigure.Code, aiConfigure.Body.String())
	}

	disable := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/enterprises/"+created.Enterprise.ID+"/risk/disable", bytes.NewBufferString(`{"requestId":"req-risk-001","reason":"风控人工复核"}`), adminToken)
	if disable.Code != http.StatusOK || !strings.Contains(disable.Body.String(), `"status":"SUSPENDED"`) {
		t.Fatalf("disable enterprise = %d %s", disable.Code, disable.Body.String())
	}
	audit := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/enterprises/"+created.Enterprise.ID+"/audit-logs", nil, adminToken)
	if audit.Code != http.StatusOK || !strings.Contains(audit.Body.String(), "req-risk-001") || !strings.Contains(audit.Body.String(), "风控人工复核") {
		t.Fatalf("enterprise audit after mutations = %d %s", audit.Code, audit.Body.String())
	}
}

func TestAdminEnterprisePermissionMapping(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodGet, "/api/v1/admin/enterprises", permissionEnterpriseList},
		{http.MethodPost, "/api/v1/admin/enterprises", permissionEnterpriseCreate},
		{http.MethodGet, "/api/v1/admin/enterprises/export", permissionEnterpriseExport},
		{http.MethodGet, "/api/v1/admin/enterprises/tenant_demo", permissionEnterpriseDetail},
		{http.MethodPatch, "/api/v1/admin/enterprises/tenant_demo", permissionEnterpriseUpdate},
		{http.MethodGet, "/api/v1/admin/enterprises/certifications", permissionEnterpriseCertificationReview},
		{http.MethodGet, "/api/v1/admin/enterprises/tenant_demo/members", permissionEnterpriseMemberView},
		{http.MethodGet, "/api/v1/admin/enterprises/tenant_demo/package", permissionEnterprisePackageView},
		{http.MethodPost, "/api/v1/admin/enterprises/tenant_demo/package/adjust", permissionEnterprisePackageAdjust},
		{http.MethodPost, "/api/v1/admin/enterprises/tenant_demo/seats/adjust", permissionEnterpriseSeatAdjust},
		{http.MethodGet, "/api/v1/admin/enterprises/tenant_demo/compute", permissionEnterpriseComputeView},
		{http.MethodPost, "/api/v1/admin/enterprises/tenant_demo/compute/adjust", permissionEnterpriseComputeAdjust},
		{http.MethodPost, "/api/v1/admin/enterprises/tenant_demo/recharge", permissionEnterpriseComputeAdjust},
		{http.MethodGet, "/api/v1/admin/enterprises/tenant_demo/transactions", permissionEnterpriseTransactionView},
		{http.MethodGet, "/api/v1/admin/enterprises/tenant_demo/orders", permissionEnterpriseOrderView},
		{http.MethodGet, "/api/v1/admin/enterprises/tenant_demo/knowledge-bases", permissionEnterpriseKnowledgeView},
		{http.MethodGet, "/api/v1/admin/enterprises/tenant_demo/integrations", permissionEnterpriseConnectorView},
		{http.MethodPost, "/api/v1/admin/enterprises/tenant_demo/ai-capabilities/configure", permissionEnterpriseAIConfigure},
		{http.MethodPost, "/api/v1/admin/enterprises/tenant_demo/attribution/change", permissionEnterpriseAttributionChange},
		{http.MethodPost, "/api/v1/admin/enterprises/tenant_demo/risk/disable", permissionEnterpriseRiskDisable},
	}
	for _, item := range tests {
		request, err := http.NewRequest(item.method, item.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := adminPermissionForRequest(request); got != item.want {
			t.Fatalf("%s %s permission = %s, want %s", item.method, item.path, got, item.want)
		}
	}
}

func TestAdminEnterpriseSecondPhaseValidationLedgerAndIsolation(t *testing.T) {
	cfg := config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()}
	store := newJSONStore(cfg.DataPath)
	handler := newWithStore(cfg, store).Handler
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	createEnterprise := func(code string) adminEnterpriseDetail {
		t.Helper()
		response := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/enterprises", bytes.NewBufferString(`{
			"name":"`+code+` company",
			"enterpriseCode":"`+code+`",
			"ownerUserId":"user_000002",
			"planCode":"enterprise_trial",
			"seatLimit":3
		}`), adminToken)
		if response.Code != http.StatusCreated {
			t.Fatalf("create %s = %d %s", code, response.Code, response.Body.String())
		}
		var detail adminEnterpriseDetail
		if err := json.NewDecoder(response.Body).Decode(&detail); err != nil {
			t.Fatal(err)
		}
		return detail
	}

	companyA := createEnterprise("ENT-PHASE2-A")
	companyB := createEnterprise("ENT-PHASE2-B")
	err := store.updateAdmin(func(data *adminPlatformData) error {
		now := enterpriseNow()
		data.Enterprise.Members = append(data.Enterprise.Members, enterpriseMember{
			ID: "member_phase2_unique", TenantID: companyA.Enterprise.ID, UserID: "user_phase2_unique",
			MemberStatus: "ACTIVE", Roles: []string{roleEnterpriseMember}, CreatedAt: now, UpdatedAt: now,
		})
		data.Enterprise.Certifications = append(data.Enterprise.Certifications, enterpriseCertification{
			ID: "cert_phase2_pending", TenantID: companyA.Enterprise.ID, LegalName: "Phase 2 Company",
			UnifiedSocialCreditCode: "91310000PHASE20001", Status: "PENDING", CreatedAt: now, UpdatedAt: now,
		})
		data.Orders = append(data.Orders,
			adminOrder{ID: "order_phase2_a", TenantID: companyA.Enterprise.ID, UserID: "user_000002", AmountCents: 100, Status: "PAID", PriceSnapshot: map[string]any{}, CreatedAt: now},
			adminOrder{ID: "order_phase2_b", UserID: "user_000002", AmountCents: 200, Status: "PAID", PriceSnapshot: map[string]any{"tenantId": companyB.Enterprise.ID}, CreatedAt: now},
			adminOrder{ID: "order_phase2_ambiguous", UserID: "user_000002", AmountCents: 300, Status: "PAID", PriceSnapshot: map[string]any{}, CreatedAt: now},
			adminOrder{ID: "order_phase2_unique", UserID: "user_phase2_unique", AmountCents: 400, Status: "PAID", PriceSnapshot: map[string]any{}, CreatedAt: now},
		)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	seat := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/enterprises/"+companyA.Enterprise.ID+"/seats/adjust", bytes.NewBufferString(`{"requestId":"phase2-seat-low","reason":"capacity review","seatLimit":1}`), adminToken)
	if seat.Code != http.StatusBadRequest {
		t.Fatalf("seat below active members = %d %s", seat.Code, seat.Body.String())
	}
	emptyPackage := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/enterprises/"+companyA.Enterprise.ID+"/package/adjust", bytes.NewBufferString(`{"requestId":"phase2-package-empty","reason":"package review"}`), adminToken)
	if emptyPackage.Code != http.StatusBadRequest {
		t.Fatalf("empty package adjustment = %d %s", emptyPackage.Code, emptyPackage.Body.String())
	}
	invalidPackageExpiry := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/enterprises/"+companyA.Enterprise.ID+"/package/adjust", bytes.NewBufferString(`{"requestId":"phase2-package-date","reason":"package review","planCode":"enterprise_pro","expiresAt":"2026-99-99"}`), adminToken)
	if invalidPackageExpiry.Code != http.StatusBadRequest {
		t.Fatalf("invalid package expiry = %d %s", invalidPackageExpiry.Code, invalidPackageExpiry.Body.String())
	}
	packageAdjust := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/enterprises/"+companyA.Enterprise.ID+"/package/adjust", bytes.NewBufferString(`{"requestId":"phase2-package-ok","reason":"annual contract","planCode":"enterprise_pro","expiresAt":"2027-07-14T00:00:00Z"}`), adminToken)
	if packageAdjust.Code != http.StatusOK || !strings.Contains(packageAdjust.Body.String(), `"planCode":"enterprise_pro"`) {
		t.Fatalf("package adjustment = %d %s", packageAdjust.Code, packageAdjust.Body.String())
	}

	rejectWithoutComment := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/enterprises/"+companyA.Enterprise.ID+"/certifications/review", bytes.NewBufferString(`{"requestId":"phase2-cert-empty","reason":"material mismatch","status":"REJECTED"}`), adminToken)
	if rejectWithoutComment.Code != http.StatusBadRequest {
		t.Fatalf("cert rejection without comment = %d %s", rejectWithoutComment.Code, rejectWithoutComment.Body.String())
	}
	reject := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/enterprises/"+companyA.Enterprise.ID+"/certifications/review", bytes.NewBufferString(`{"requestId":"phase2-cert-reject","reason":"material mismatch","status":"REJECTED","reviewComment":"credit code image is unclear"}`), adminToken)
	if reject.Code != http.StatusOK {
		t.Fatalf("cert rejection = %d %s", reject.Code, reject.Body.String())
	}
	reviewAgain := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/enterprises/"+companyA.Enterprise.ID+"/certifications/review", bytes.NewBufferString(`{"requestId":"phase2-cert-repeat","reason":"repeat review","status":"APPROVED"}`), adminToken)
	if reviewAgain.Code != http.StatusConflict {
		t.Fatalf("repeat certification review = %d %s", reviewAgain.Code, reviewAgain.Body.String())
	}

	negativeRecharge := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/enterprises/"+companyA.Enterprise.ID+"/recharge", bytes.NewBufferString(`{"requestId":"phase2-recharge-negative","reason":"invalid recharge","pointDelta":-1}`), adminToken)
	if negativeRecharge.Code != http.StatusBadRequest {
		t.Fatalf("negative recharge = %d %s", negativeRecharge.Code, negativeRecharge.Body.String())
	}
	compute := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/enterprises/"+companyA.Enterprise.ID+"/compute/adjust", bytes.NewBufferString(`{"requestId":"phase2-compute","reason":"service compensation","pointDelta":300}`), adminToken)
	if compute.Code != http.StatusOK {
		t.Fatalf("compute adjustment = %d %s", compute.Code, compute.Body.String())
	}
	transactions := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/enterprises/"+companyA.Enterprise.ID+"/transactions", nil, adminToken)
	if transactions.Code != http.StatusOK || !strings.Contains(transactions.Body.String(), `"requestId":"phase2-compute"`) || !strings.Contains(transactions.Body.String(), `"pointDelta":300`) {
		t.Fatalf("memory compute ledger = %d %s", transactions.Code, transactions.Body.String())
	}

	ordersA := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/enterprises/"+companyA.Enterprise.ID+"/orders", nil, adminToken)
	bodyA := ordersA.Body.String()
	if ordersA.Code != http.StatusOK || !strings.Contains(bodyA, "order_phase2_a") || !strings.Contains(bodyA, "order_phase2_unique") || strings.Contains(bodyA, "order_phase2_b") || strings.Contains(bodyA, "order_phase2_ambiguous") {
		t.Fatalf("enterprise A order isolation = %d %s", ordersA.Code, bodyA)
	}
	ordersB := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/enterprises/"+companyB.Enterprise.ID+"/orders", nil, adminToken)
	bodyB := ordersB.Body.String()
	if ordersB.Code != http.StatusOK || !strings.Contains(bodyB, "order_phase2_b") || strings.Contains(bodyB, "order_phase2_a") || strings.Contains(bodyB, "order_phase2_unique") || strings.Contains(bodyB, "order_phase2_ambiguous") {
		t.Fatalf("enterprise B order isolation = %d %s", ordersB.Code, bodyB)
	}
}

func TestAdminEnterpriseSecondPhaseRoleMatrix(t *testing.T) {
	tests := []struct {
		role    string
		allowed []string
		denied  []string
	}{
		{"SUPER_ADMIN", adminEnterprisePermissions, nil},
		{"ENTERPRISE_OPERATOR", []string{permissionEnterpriseMemberView, permissionEnterprisePackageAdjust, permissionEnterpriseSeatAdjust, permissionEnterpriseComputeAdjust, permissionEnterpriseTransactionView, permissionEnterpriseOrderView}, []string{permissionEnterpriseCertificationReview, permissionEnterpriseRiskDisable}},
		{"CERTIFICATION_REVIEWER", []string{permissionEnterpriseCertificationReview}, []string{permissionEnterprisePackageAdjust, permissionEnterpriseComputeAdjust, permissionEnterpriseOrderView}},
		{"FINANCE", []string{permissionEnterprisePackageView, permissionEnterpriseComputeAdjust, permissionEnterpriseTransactionView, permissionEnterpriseOrderView}, []string{permissionEnterpriseSeatAdjust, permissionEnterpriseCertificationReview}},
		{"RISK_MANAGER", []string{permissionEnterpriseRiskView}, []string{permissionEnterpriseMemberView, permissionEnterpriseComputeAdjust, permissionEnterpriseOrderView}},
		{"CUSTOMER_SERVICE", []string{permissionEnterpriseMemberView, permissionEnterprisePackageView, permissionEnterpriseComputeView, permissionEnterpriseOrderView}, []string{permissionEnterprisePackageAdjust, permissionEnterpriseSeatAdjust, permissionEnterpriseComputeAdjust}},
	}
	for _, test := range tests {
		permissions := permissionsForRole(test.role)
		for _, permission := range test.allowed {
			if !stringSliceContains(permissions, permission) {
				t.Fatalf("%s missing permission %s", test.role, permission)
			}
		}
		for _, permission := range test.denied {
			if stringSliceContains(permissions, permission) {
				t.Fatalf("%s unexpectedly has permission %s", test.role, permission)
			}
		}
	}
}
