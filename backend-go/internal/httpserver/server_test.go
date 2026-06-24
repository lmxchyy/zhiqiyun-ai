package httpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

func TestGenerationTaskLifecycle(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler

	assertStatus(t, handler, http.MethodGet, "/api/v1/health", nil, http.StatusOK)
	assertStatus(t, handler, http.MethodGet, "/api/v1/models", nil, http.StatusOK)

	createBody := bytes.NewBufferString(`{"type":"TEXT_TO_IMAGE","prompt":"画一只小猫","model":"mock-standard","params":{"count":1}}`)
	createRes := request(t, handler, http.MethodPost, "/api/v1/generation-tasks", createBody)
	if createRes.Code != http.StatusOK {
		t.Fatalf("create task status = %d, body = %s", createRes.Code, createRes.Body.String())
	}
	var task generationTask
	if err := json.NewDecoder(createRes.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	if task.Status != "SUCCEEDED" || len(task.ResultIDs) != 1 {
		t.Fatalf("unexpected task: %+v", task)
	}

	assertStatus(t, handler, http.MethodGet, "/api/v1/generation-tasks", nil, http.StatusOK)
	assetsRes := request(t, handler, http.MethodGet, "/api/v1/assets", nil)
	if assetsRes.Code != http.StatusOK {
		t.Fatalf("list assets status = %d, body = %s", assetsRes.Code, assetsRes.Body.String())
	}
	var assets []asset
	if err := json.NewDecoder(assetsRes.Body).Decode(&assets); err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 {
		t.Fatalf("assets length = %d, want 1", len(assets))
	}
	if strings.Contains(assets[0].URL, "picsum.photos") {
		t.Fatalf("asset URL still uses random placeholder: %s", assets[0].URL)
	}
	const prefix = "data:image/svg+xml;base64,"
	if !strings.HasPrefix(assets[0].URL, prefix) {
		t.Fatalf("asset URL = %q, want SVG data URL", assets[0].URL)
	}
	if assets[0].ThumbnailURL == "" {
		t.Fatalf("asset thumbnail URL is empty")
	}
	rawSVG, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(assets[0].URL, prefix))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawSVG), `id="cat-subject"`) {
		t.Fatalf("cat prompt did not render cat SVG: %s", string(rawSVG))
	}
	assertStatus(t, handler, http.MethodDelete, "/api/v1/assets/"+task.ResultIDs[0], nil, http.StatusOK)
}

func TestUserGenerationAssetPointsAdminLoop(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler

	pointsBefore := request(t, handler, http.MethodGet, "/api/v1/points/account", nil)
	if pointsBefore.Code != http.StatusOK || !strings.Contains(pointsBefore.Body.String(), `"available":959`) {
		t.Fatalf("initial points response = %d %s", pointsBefore.Code, pointsBefore.Body.String())
	}

	createBody := bytes.NewBufferString(`{"type":"TEXT_TO_IMAGE","prompt":"闭环测试图片","model":"mock-standard","params":{"count":2}}`)
	createRes := request(t, handler, http.MethodPost, "/api/v1/generation-tasks", createBody)
	if createRes.Code != http.StatusOK {
		t.Fatalf("create task status = %d, body = %s", createRes.Code, createRes.Body.String())
	}
	var task generationTask
	if err := json.NewDecoder(createRes.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	if task.UserID != "user_000002" || task.PointCost != 2 || len(task.ResultIDs) != 2 || task.Status != "SUCCEEDED" {
		t.Fatalf("unexpected task: %+v", task)
	}

	pointsAfter := request(t, handler, http.MethodGet, "/api/v1/points/account", nil)
	if pointsAfter.Code != http.StatusOK || !strings.Contains(pointsAfter.Body.String(), `"available":957`) {
		t.Fatalf("deducted points response = %d %s", pointsAfter.Code, pointsAfter.Body.String())
	}

	assets := request(t, handler, http.MethodGet, "/api/v1/assets", nil)
	if assets.Code != http.StatusOK || !strings.Contains(assets.Body.String(), task.ResultIDs[0]) || !strings.Contains(assets.Body.String(), `"mediaType":"image"`) {
		t.Fatalf("assets not visible after generation: %d %s", assets.Code, assets.Body.String())
	}

	customers := request(t, handler, http.MethodGet, "/api/v1/admin/customers", nil)
	if customers.Code != http.StatusOK || !strings.Contains(customers.Body.String(), `"pointsAvailable":957`) || !strings.Contains(customers.Body.String(), "演示用户") {
		t.Fatalf("admin customers did not reflect deducted points: %d %s", customers.Code, customers.Body.String())
	}

	adminTasks := request(t, handler, http.MethodGet, "/api/v1/admin/generation-tasks", nil)
	adminTaskBody := adminTasks.Body.String()
	if adminTasks.Code != http.StatusOK || !strings.Contains(adminTaskBody, task.ID) || !strings.Contains(adminTaskBody, `"pointCost":2`) || !strings.Contains(adminTaskBody, task.ResultIDs[0]) {
		t.Fatalf("admin generation tasks missing closed-loop data: %d %s", adminTasks.Code, adminTaskBody)
	}

	overview := request(t, handler, http.MethodGet, "/api/v1/admin/overview", nil)
	if overview.Code != http.StatusOK || !strings.Contains(overview.Body.String(), `"generatedAssets":2`) {
		t.Fatalf("admin overview missing generated assets: %d %s", overview.Code, overview.Body.String())
	}

	usage := request(t, handler, http.MethodGet, "/api/v1/admin/usage", nil)
	if usage.Code != http.StatusOK || !strings.Contains(usage.Body.String(), `"apiCalls":1`) || !strings.Contains(usage.Body.String(), `"assets":2`) {
		t.Fatalf("admin usage missing generation counters: %d %s", usage.Code, usage.Body.String())
	}
}
func TestAgentLoginAndChannelCenter(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	raw := `{
		"users":[
			{"id":"user_000002","email":"demo@xianzhi.ai","name":"演示用户","role":"MEMBER","status":"ACTIVE","planId":"plan_month","referredBy":"user_000003"},
			{"id":"user_000003","email":"agent1@xianzhi.ai","name":"华东一级代理","role":"AGENT_L1","status":"ACTIVE","planId":"plan_free"},
			{"id":"user_000004","email":"agent2@xianzhi.ai","name":"华东二级代理","role":"AGENT_L2","status":"ACTIVE","planId":"plan_free"}
		],
		"channelAgents":[
			{"id":"channel_000001","userId":"user_000003","level":1,"status":"ACTIVE","inviteCode":"EAST001"},
			{"id":"channel_000002","userId":"user_000004","parentId":"channel_000001","level":2,"status":"ACTIVE","inviteCode":"EAST002"}
		],
		"commissions":[{"id":"commission_000001","orderId":"order_000001","agentId":"channel_000001","amountCents":990,"rate":0.1,"status":"SETTLED"}],
		"withdrawals":[{"id":"withdrawal_000001","agentId":"channel_000001","amountCents":300,"status":"PENDING"}],
		"counters":{}
	}`
	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler

	login := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"agent1@xianzhi.ai","password":"Agent123!"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("agent login status = %d, body = %s", login.Code, login.Body.String())
	}
	var loginBody struct {
		AccessToken   string         `json:"accessToken"`
		DefaultModule string         `json:"defaultModule"`
		User          map[string]any `json:"user"`
		Agent         map[string]any `json:"agent"`
	}
	if err := json.NewDecoder(login.Body).Decode(&loginBody); err != nil {
		t.Fatal(err)
	}
	if loginBody.AccessToken == "" || loginBody.DefaultModule != "agentHome" || loginBody.Agent["inviteCode"] != "EAST001" {
		t.Fatalf("unexpected login body: %+v", loginBody)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/channel/me", nil)
	req.Header.Set("Authorization", "Bearer "+loginBody.AccessToken)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	body := res.Body.String()
	if res.Code != http.StatusOK || !strings.Contains(body, `"directCustomers":1`) || !strings.Contains(body, `"childAgents":1`) || !strings.Contains(body, `"totalCommission":990`) || !strings.Contains(body, "演示用户") {
		t.Fatalf("channel center response = %d %s", res.Code, body)
	}

	memberLogin := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"demo@xianzhi.ai","password":"Demo123!"}`))
	if memberLogin.Code != http.StatusOK {
		t.Fatalf("member login status = %d, body = %s", memberLogin.Code, memberLogin.Body.String())
	}
	var memberBody struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(memberLogin.Body).Decode(&memberBody); err != nil {
		t.Fatal(err)
	}
	memberReq := httptest.NewRequest(http.MethodGet, "/api/v1/channel/me", nil)
	memberReq.Header.Set("Authorization", "Bearer "+memberBody.AccessToken)
	memberRes := httptest.NewRecorder()
	handler.ServeHTTP(memberRes, memberReq)
	if memberRes.Code != http.StatusForbidden {
		t.Fatalf("member channel center status = %d, body = %s", memberRes.Code, memberRes.Body.String())
	}
}
func TestCreateChannelAgentPersistsUserAndTree(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler

	createL1 := request(t, handler, http.MethodPost, "/api/v1/admin/channel-agents", bytes.NewBufferString(`{"name":"测试一级代理","email":"agent-new@example.com","level":1,"inviteCode":"NEW001","status":"ACTIVE","available":88}`))
	if createL1.Code != http.StatusOK {
		t.Fatalf("create level 1 channel agent status = %d, body = %s", createL1.Code, createL1.Body.String())
	}
	var l1Body struct {
		Item map[string]any `json:"item"`
		User adminUser      `json:"user"`
	}
	if err := json.NewDecoder(createL1.Body).Decode(&l1Body); err != nil {
		t.Fatal(err)
	}
	if l1Body.Item["id"] == "" || l1Body.User.Role != "AGENT_L1" {
		t.Fatalf("unexpected level 1 body: %+v", l1Body)
	}

	parentID, _ := l1Body.Item["id"].(string)
	createL2 := request(t, handler, http.MethodPost, "/api/v1/admin/channel-agents", bytes.NewBufferString(`{"name":"测试二级代理","email":"agent-child@example.com","level":2,"parentId":"`+parentID+`","status":"ACTIVE"}`))
	if createL2.Code != http.StatusOK {
		t.Fatalf("create level 2 channel agent status = %d, body = %s", createL2.Code, createL2.Body.String())
	}

	login := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"agent-new@example.com","password":"Agent123!"}`))
	if login.Code != http.StatusOK || !strings.Contains(login.Body.String(), `"defaultModule":"agentHome"`) {
		t.Fatalf("created agent login failed: %d %s", login.Code, login.Body.String())
	}

	tree := request(t, handler, http.MethodGet, "/api/v1/admin/channel-agents/tree", nil)
	body := tree.Body.String()
	for _, want := range []string{"测试一级代理", "测试二级代理", "NEW001"} {
		if !strings.Contains(body+createL1.Body.String(), want) {
			t.Fatalf("channel tree or create response missing %q: tree=%s create=%s", want, body, createL1.Body.String())
		}
	}
}

func TestDeleteMissingAssetReturnsNotFound(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})

	assertStatus(t, server.Handler, http.MethodDelete, "/api/v1/assets/missing", nil, http.StatusNotFound)
}

func TestAdminAPIsReadMasterControlData(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	raw := `{
		"users":[{"id":"user_000001","email":"admin@xianzhi.ai","name":"平台管理员","role":"SUPER_ADMIN","status":"ACTIVE","planId":"plan_free"},{"id":"user_000002","email":"demo@xianzhi.ai","name":"演示用户","role":"MEMBER","status":"ACTIVE","planId":"plan_month"}],
		"plans":[{"id":"plan_free","name":"免费会员","price":0,"points":100,"durationDays":36500,"concurrency":1},{"id":"plan_month","name":"月度会员","price":9900,"points":3000,"durationDays":30,"concurrency":3}],
		"pointAccounts":[{"id":"points_000002","userId":"user_000002","available":3000,"frozen":0}],
		"orders":[{"id":"order_000001","userId":"user_000002","planId":"plan_month","amount":9900,"status":"PAID","createdAt":"2026-06-18T00:00:00Z"}],
		"channelAgents":[{"id":"channel_000001","userId":"user_000002","level":1,"status":"ACTIVE","inviteCode":"EAST001"}],
		"generationTasks":[{"id":"task_000001","userId":"user_000002","type":"TEXT_TO_IMAGE","prompt":"测试","model":"mock-standard","status":"SUCCEEDED","progress":100,"pointCost":1,"resultIds":[]}],
		"agentCalls":[{"id":"agentcall_000001","agentId":"agent_000001","userId":"user_000002","tokenUsage":20,"cost":2}],
		"geoTasks":[{"id":"geo_000001","ownerId":"user_000002","brandId":"brand_000001","question":"测试","platform":"ChatGPT","status":"DONE"}],
		"assets":[],
		"counters":{}
	}`
	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})

	for _, path := range []string{
		"/api/v1/admin/overview",
		"/api/v1/admin/customers",
		"/api/v1/admin/channel-agents/tree",
		"/api/v1/admin/products",
		"/api/v1/admin/plans",
		"/api/v1/admin/orders",
		"/api/v1/admin/delivery-projects",
		"/api/v1/admin/usage",
		"/api/v1/admin/commissions",
		"/api/v1/admin/system/settings",
		"/api/v1/admin/api/provider-channels",
		"/api/v1/admin/api/models",
		"/api/v1/admin/api/keys",
		"/api/v1/admin/customer-groups",
		"/v1/dashboard/billing/subscription",
		"/v1/dashboard/billing/usage",
	} {
		assertStatus(t, server.Handler, http.MethodGet, path, nil, http.StatusOK)
	}

	overviewRes := request(t, server.Handler, http.MethodGet, "/api/v1/admin/overview", nil)
	var overview map[string]any
	if err := json.NewDecoder(overviewRes.Body).Decode(&overview); err != nil {
		t.Fatal(err)
	}
	if _, ok := overview["metrics"].([]any); !ok {
		t.Fatalf("overview metrics missing: %+v", overview)
	}
}

func TestAdminMutationAPIsPersistMasterControlData(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler

	createCustomer := request(t, handler, http.MethodPost, "/api/v1/admin/customers", bytes.NewBufferString(`{"name":"测试客户","email":"customer@example.com","planId":"plan_month","available":6000}`))
	if createCustomer.Code != http.StatusOK {
		t.Fatalf("create customer status = %d, body = %s", createCustomer.Code, createCustomer.Body.String())
	}
	var customerBody struct {
		Item adminUser `json:"item"`
	}
	if err := json.NewDecoder(createCustomer.Body).Decode(&customerBody); err != nil {
		t.Fatal(err)
	}
	if customerBody.Item.ID == "" {
		t.Fatalf("created customer missing id: %+v", customerBody)
	}

	updateCustomerPath := "/api/v1/admin/customers/" + customerBody.Item.ID
	assertStatus(t, handler, http.MethodPatch, updateCustomerPath, bytes.NewBufferString(`{"status":"DISABLED","planId":"plan_year","available":7000}`), http.StatusOK)

	createOrder := request(t, handler, http.MethodPost, "/api/v1/admin/orders", bytes.NewBufferString(`{"userId":"`+customerBody.Item.ID+`","planId":"plan_year","amountCents":89900}`))
	if createOrder.Code != http.StatusOK {
		t.Fatalf("create order status = %d, body = %s", createOrder.Code, createOrder.Body.String())
	}
	var orderBody struct {
		Item adminOrder `json:"item"`
	}
	if err := json.NewDecoder(createOrder.Body).Decode(&orderBody); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, handler, http.MethodPost, "/api/v1/admin/orders/"+orderBody.Item.ID+"/mark-paid", bytes.NewBuffer(nil), http.StatusOK)
	assertStatus(t, handler, http.MethodPost, "/api/v1/admin/orders/"+orderBody.Item.ID+"/renew", bytes.NewBuffer(nil), http.StatusOK)

	customers := request(t, handler, http.MethodGet, "/api/v1/admin/customers", nil)
	if !strings.Contains(customers.Body.String(), `"status":"DISABLED"`) || !strings.Contains(customers.Body.String(), `"pointsAvailable":7000`) {
		t.Fatalf("customer update was not persisted: %s", customers.Body.String())
	}
	orders := request(t, handler, http.MethodGet, "/api/v1/admin/orders", nil)
	if !strings.Contains(orders.Body.String(), `"status":"PAID"`) || !strings.Contains(orders.Body.String(), `"status":"PENDING"`) {
		t.Fatalf("order mutations were not persisted: %s", orders.Body.String())
	}
}

func TestAdminSystemAndAPIGatewayMutationsPersist(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler

	assertStatus(t, handler, http.MethodPatch, "/api/v1/admin/system/settings", bytes.NewBufferString(`{"brand":{"name":"先知主控","domain":"admin.example.com","logo":"控"},"payments":[{"channel":"manual","status":"ACTIVE"}],"permissions":["SUPER_ADMIN","FINANCE"]}`), http.StatusOK)
	assertStatus(t, handler, http.MethodPost, "/api/v1/admin/api/provider-channels", bytes.NewBufferString(`{"name":"测试上游","baseUrl":"https://provider.example.com/v1","status":"ACTIVE","priority":30,"models":["gpt-image-2"]}`), http.StatusOK)
	assertStatus(t, handler, http.MethodPatch, "/api/v1/admin/api/models/model_gpt_image_2", bytes.NewBufferString(`{"name":"OpenAI 图像模型","capability":"IMAGE","billingMode":"PER_REQUEST","fixedQuota":12,"modelRatio":1,"completionRatio":1,"status":"ACTIVE"}`), http.StatusOK)
	assertStatus(t, handler, http.MethodPost, "/api/v1/admin/api/keys", bytes.NewBufferString(`{"customer":"测试客户","status":"ACTIVE","models":["gpt-image-2"],"quotaLimit":50000}`), http.StatusOK)
	assertStatus(t, handler, http.MethodPatch, "/api/v1/admin/customer-groups/group_vip", bytes.NewBufferString(`{"name":"vip","ratio":0.7,"models":["gpt-image-2"],"description":"测试倍率"}`), http.StatusOK)

	system := request(t, handler, http.MethodGet, "/api/v1/admin/system/settings", nil)
	body := system.Body.String()
	for _, want := range []string{"先知主控", "admin.example.com", "测试上游", `"fixedQuota":12`, "测试客户", `"ratio":0.7`} {
		if !strings.Contains(body, want) {
			t.Fatalf("system mutation response missing %q: %s", want, body)
		}
	}
}

func TestAdminUsageAndCommissionOperations(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	raw := `{
		"users":[{"id":"user_000002","email":"demo@xianzhi.ai","name":"演示用户","role":"MEMBER","status":"ACTIVE","planId":"plan_month"},{"id":"user_000003","email":"agent1@xianzhi.ai","name":"华东一级代理","role":"AGENT_L1","status":"ACTIVE","planId":"plan_free"}],
		"plans":[{"id":"plan_month","name":"月度会员","price":9900,"points":3000,"durationDays":30,"concurrency":3}],
		"orders":[{"id":"order_000001","userId":"user_000002","planId":"plan_month","amount":9900,"status":"PAID"}],
		"channelAgents":[{"id":"channel_000001","userId":"user_000003","level":1,"status":"ACTIVE","inviteCode":"EAST001"}],
		"generationTasks":[{"id":"task_000001","userId":"user_000002","type":"TEXT_TO_IMAGE","prompt":"测试","model":"mock-standard","status":"SUCCEEDED","progress":100,"pointCost":1,"resultIds":[]}],
		"agentCalls":[{"id":"agentcall_000001","agentId":"agent_000001","userId":"user_000002","tokenUsage":20,"cost":2}],
		"geoTasks":[{"id":"geo_000001","ownerId":"user_000002","brandId":"brand_000001","question":"测试","platform":"ChatGPT","status":"DONE"}],
		"assets":[],
		"counters":{}
	}`
	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler

	usage := request(t, handler, http.MethodGet, "/api/v1/admin/usage?product=Agent", nil)
	if usage.Code != http.StatusOK || !strings.Contains(usage.Body.String(), `"product":"Agent"`) || strings.Contains(usage.Body.String(), "GEO 任务") {
		t.Fatalf("usage filter failed: status = %d, body = %s", usage.Code, usage.Body.String())
	}
	export := request(t, handler, http.MethodGet, "/api/v1/admin/usage/export?product=Agent", nil)
	if export.Code != http.StatusOK || !strings.Contains(export.Body.String(), "Agent") || !strings.Contains(export.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("usage export failed: status = %d, type = %s, body = %s", export.Code, export.Header().Get("Content-Type"), export.Body.String())
	}

	createCommission := request(t, handler, http.MethodPost, "/api/v1/admin/commissions", bytes.NewBufferString(`{"orderId":"order_000001","agentId":"channel_000001","amountCents":990,"rate":0.1}`))
	if createCommission.Code != http.StatusOK {
		t.Fatalf("create commission status = %d, body = %s", createCommission.Code, createCommission.Body.String())
	}
	createWithdrawal := request(t, handler, http.MethodPost, "/api/v1/admin/withdrawals", bytes.NewBufferString(`{"agentId":"channel_000001","amountCents":500}`))
	if createWithdrawal.Code != http.StatusOK {
		t.Fatalf("create withdrawal status = %d, body = %s", createWithdrawal.Code, createWithdrawal.Body.String())
	}
	var withdrawalBody struct {
		Item adminWithdrawal `json:"item"`
	}
	if err := json.NewDecoder(createWithdrawal.Body).Decode(&withdrawalBody); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, handler, http.MethodPost, "/api/v1/admin/withdrawals/"+withdrawalBody.Item.ID+"/approve", bytes.NewBuffer(nil), http.StatusOK)

	commissions := request(t, handler, http.MethodGet, "/api/v1/admin/commissions", nil)
	body := commissions.Body.String()
	for _, want := range []string{`"amountCents":990`, `"amountCents":500`, `"status":"APPROVED"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("commissions response missing %q: %s", want, body)
		}
	}
}

func TestConcurrentGenerationTaskCreatesKeepUniqueIDs(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler

	const requestCount = 20
	errs := make(chan string, requestCount)
	var wg sync.WaitGroup
	for i := 0; i < requestCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := bytes.NewBufferString(`{"type":"TEXT_TO_IMAGE","prompt":"cat ` + string(rune('a'+i)) + `","model":"mock-standard","params":{"count":1}}`)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/generation-tasks", body)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != http.StatusOK {
				errs <- res.Body.String()
				return
			}
			var task generationTask
			if err := json.NewDecoder(res.Body).Decode(&task); err != nil {
				errs <- err.Error()
				return
			}
			if task.ID == "" || len(task.ResultIDs) != 1 {
				errs <- "created task is missing ID or result"
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	tasksRes := request(t, handler, http.MethodGet, "/api/v1/generation-tasks", nil)
	if tasksRes.Code != http.StatusOK {
		t.Fatalf("list tasks status = %d, body = %s", tasksRes.Code, tasksRes.Body.String())
	}
	var tasks []generationTask
	if err := json.NewDecoder(tasksRes.Body).Decode(&tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks) != requestCount {
		t.Fatalf("tasks length = %d, want %d", len(tasks), requestCount)
	}
	seenTasks := map[string]bool{}
	seenAssets := map[string]bool{}
	for _, task := range tasks {
		if seenTasks[task.ID] {
			t.Fatalf("duplicate task ID %q", task.ID)
		}
		seenTasks[task.ID] = true
		for _, assetID := range task.ResultIDs {
			if seenAssets[assetID] {
				t.Fatalf("duplicate asset ID %q", assetID)
			}
			seenAssets[assetID] = true
		}
	}
}

func TestWriteFileAtomicallyReplacesTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomically(path, []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "new\n" {
		t.Fatalf("store content = %q, want %q", string(raw), "new\n")
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".store.json.*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files were not cleaned up: %v", matches)
	}
}

func TestChangePasswordPersistsForLogin(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:           ":0",
		DataPath:       dataPath,
		StaticDir:      t.TempDir(),
		AdminStaticDir: t.TempDir(),
	})
	handler := server.Handler

	login := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"admin@xianzhi.ai","password":"Admin123!"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("admin login status = %d, body = %s", login.Code, login.Body.String())
	}
	var loginBody struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(login.Body).Decode(&loginBody); err != nil {
		t.Fatal(err)
	}
	if loginBody.AccessToken == "" {
		t.Fatal("login response missing access token")
	}

	changeReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", bytes.NewBufferString(`{"currentPassword":"Admin123!","newPassword":"Admin456!"}`))
	changeReq.Header.Set("Authorization", "Bearer "+loginBody.AccessToken)
	changeRes := httptest.NewRecorder()
	handler.ServeHTTP(changeRes, changeReq)
	if changeRes.Code != http.StatusOK {
		t.Fatalf("change password status = %d, body = %s", changeRes.Code, changeRes.Body.String())
	}

	oldLogin := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"admin@xianzhi.ai","password":"Admin123!"}`))
	if oldLogin.Code != http.StatusUnauthorized {
		t.Fatalf("old password login status = %d, body = %s", oldLogin.Code, oldLogin.Body.String())
	}
	newLogin := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"admin@xianzhi.ai","password":"Admin456!"}`))
	if newLogin.Code != http.StatusOK {
		t.Fatalf("new password login status = %d, body = %s", newLogin.Code, newLogin.Body.String())
	}
}

type memoryAuthSessions struct {
	mu     sync.Mutex
	userID map[string]string
}

func newMemoryAuthSessions() *memoryAuthSessions {
	return &memoryAuthSessions{userID: map[string]string{}}
}

func (s *memoryAuthSessions) Put(_ context.Context, token string, userID string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userID[token] = userID
	return nil
}

func (s *memoryAuthSessions) UserID(_ context.Context, token string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	userID, ok := s.userID[token]
	return userID, ok, nil
}

func (s *memoryAuthSessions) Delete(_ context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.userID, token)
	return nil
}

func TestAuthSessionStoreLogoutRevokesToken(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	sessions := newMemoryAuthSessions()
	server := newWithStoreAndSessions(config.Config{
		Addr:           ":0",
		DataPath:       dataPath,
		StaticDir:      t.TempDir(),
		AdminStaticDir: t.TempDir(),
	}, newJSONStore(dataPath), sessions)
	handler := server.Handler

	login := request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"agent1@xianzhi.ai","password":"Agent123!"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", login.Code, login.Body.String())
	}
	var loginBody struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(login.Body).Decode(&loginBody); err != nil {
		t.Fatal(err)
	}
	if loginBody.AccessToken == "" {
		t.Fatal("login response missing access token")
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+loginBody.AccessToken)
	meRes := httptest.NewRecorder()
	handler.ServeHTTP(meRes, meReq)
	if meRes.Code != http.StatusOK {
		t.Fatalf("me status = %d, body = %s", meRes.Code, meRes.Body.String())
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutReq.Header.Set("Authorization", "Bearer "+loginBody.AccessToken)
	logoutRes := httptest.NewRecorder()
	handler.ServeHTTP(logoutRes, logoutReq)
	if logoutRes.Code != http.StatusOK {
		t.Fatalf("logout status = %d, body = %s", logoutRes.Code, logoutRes.Body.String())
	}

	revokedReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	revokedReq.Header.Set("Authorization", "Bearer "+loginBody.AccessToken)
	revokedRes := httptest.NewRecorder()
	handler.ServeHTTP(revokedRes, revokedReq)
	if revokedRes.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d, body = %s", revokedRes.Code, revokedRes.Body.String())
	}
}
func assertStatus(t *testing.T, handler http.Handler, method string, path string, body *bytes.Buffer, want int) {
	t.Helper()
	res := request(t, handler, method, path, body)
	if res.Code != want {
		t.Fatalf("%s %s status = %d, want %d, body = %s", method, path, res.Code, want, res.Body.String())
	}
}

func request(t *testing.T, handler http.Handler, method string, path string, body *bytes.Buffer) *httptest.ResponseRecorder {
	t.Helper()
	if body == nil {
		body = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, body)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}
