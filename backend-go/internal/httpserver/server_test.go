package httpserver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

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
	rawSVG, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(assets[0].URL, prefix))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawSVG), `id="cat-subject"`) {
		t.Fatalf("cat prompt did not render cat SVG: %s", string(rawSVG))
	}
	assertStatus(t, handler, http.MethodDelete, "/api/v1/assets/"+task.ResultIDs[0], nil, http.StatusOK)
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
