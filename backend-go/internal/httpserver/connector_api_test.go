package httpserver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/connector"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type connectorIntegrationImageProvider struct {
	calls atomic.Int32
	last  atomic.Value
}

func (p *connectorIntegrationImageProvider) DefaultModel() string { return "gpt-image-2" }
func (p *connectorIntegrationImageProvider) Generate(_ context.Context, req generation.CreateRequest) ([]generation.GeneratedImage, error) {
	p.calls.Add(1)
	p.last.Store(req)
	return []generation.GeneratedImage{{
		URL:         "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
		ContentType: "image/png", Width: 1, Height: 1, Source: "connector-integration-test",
	}}, nil
}

type connectorFailingImageProvider struct{ calls atomic.Int32 }

func (p *connectorFailingImageProvider) DefaultModel() string { return "gpt-image-2" }
func (p *connectorFailingImageProvider) Generate(context.Context, generation.CreateRequest) ([]generation.GeneratedImage, error) {
	p.calls.Add(1)
	return nil, errors.New("simulated upstream failure")
}

func TestConnectorPermissionRules(t *testing.T) {
	if containsConnectorString([]string{"ou-other"}, "ou-bot") || !containsConnectorString([]string{"ou-other", "ou-bot"}, "ou-bot") {
		t.Fatal("bot mention matching must use the configured bot open_id")
	}
	item := enterpriseConnector{Config: defaultConnectorConfig()}
	binding := connectorUserBinding{Status: "active", Permission: map[string]any{"imageGenerate": true}}
	message := connectorMessageRecord{ChatType: "group", Content: map[string]any{"mentionedBot": false}}
	if err := validateConnectorPermission(item, binding, message); err == nil {
		t.Fatal("group message without mention should be rejected")
	}
	message.Content["mentionedBot"] = true
	if err := validateConnectorPermission(item, binding, message); err != nil {
		t.Fatalf("mentioned group message rejected: %v", err)
	}
	binding.Status = "disabled"
	if err := validateConnectorPermission(item, binding, message); err == nil {
		t.Fatal("disabled binding should be rejected")
	}
}

func TestConnectorManagementPermissionsAreInRuntimeRoleMatrix(t *testing.T) {
	for _, role := range []string{roleEnterpriseAdmin, roleAIAdmin} {
		permissions := rolePermissionMatrix[role]
		if !containsConnectorString(permissions, "enterprise.connector.read") || !containsConnectorString(permissions, "enterprise.connector.manage") {
			t.Fatalf("role %s is missing connector runtime permissions: %v", role, permissions)
		}
	}
}

func TestSanitizeConnectorPayload(t *testing.T) {
	value := sanitizeConnectorPayload([]byte(`{"header":{"token":"secret"},"event":{"text":"ok","access_token":"token"}}`))
	header := value["header"].(map[string]any)
	event := value["event"].(map[string]any)
	if header["token"] != "[REDACTED]" || event["access_token"] != "[REDACTED]" || event["text"] != "ok" {
		t.Fatalf("unexpected sanitized payload: %#v", value)
	}
}

func TestConnectorViewDoesNotExposeSecrets(t *testing.T) {
	service := connectorAPI{cfg: config.Config{ConnectorCallbackBaseURL: "https://api.example.test"}}
	view := service.view(enterpriseConnector{
		ID: "connector-secret-test", EnterpriseID: "tenant-secret-test", ConnectorType: "feishu",
		ConnectorName: "Secret Test", ConnectorKey: "safe-callback-key", AppID: "cli_public",
		AppSecretEncrypted: "ciphertext-app-secret", VerificationTokenEncrypted: "ciphertext-verification-token",
		EncryptKeyEncrypted: "ciphertext-encrypt-key", Status: "disabled", Config: defaultConnectorConfig(),
	})
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(raw)
	for _, forbidden := range []string{"ciphertext-app-secret", "ciphertext-verification-token", "ciphertext-encrypt-key", "appSecretEncrypted", "verificationTokenEncrypted", "encryptKeyEncrypted"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("connector view exposed secret field %q: %s", forbidden, serialized)
		}
	}
	if !view.SecretsConfigured.AppSecret || !view.SecretsConfigured.VerificationToken || !view.SecretsConfigured.EncryptKey {
		t.Fatalf("configured flags missing: %+v", view.SecretsConfigured)
	}
}

func TestConnectorCallbackFastAckAndDuplicateQueueing(t *testing.T) {
	_, db := enterpriseP0TestStore(t)
	fixture := seedEnterpriseP0Fixture(t, db, 1, 100)
	masterKey := "connector-callback-secret-key-at-least-32-bytes"
	cipher, err := storagecenter.NewSecretCipher(masterKey)
	if err != nil {
		t.Fatal(err)
	}
	service := &connectorAPI{
		cfg: config.Config{ConnectorSecretEncryptionKey: masterKey}, repo: connectorRepository{db: db},
		cipher: cipher, queue: newConnectorJobQueue(nil, "test:connector:"), limiter: newConnectorRateLimiter(),
	}
	item := enterpriseConnector{
		ID: newConnectorID("connector_callback"), EnterpriseID: fixture.tenantIDs[0], ConnectorType: "feishu",
		ConnectorName: "Callback Test", ConnectorKey: newConnectorID("connector_key"), AppID: "app-callback",
		Config: defaultConnectorConfig(),
	}
	item.AppSecretEncrypted, err = cipher.Encrypt("app-secret", item.ID+":app_secret")
	if err != nil {
		t.Fatal(err)
	}
	item.VerificationTokenEncrypted, err = cipher.Encrypt("verify-token", item.ID+":verification_token")
	if err != nil {
		t.Fatal(err)
	}
	item, err = service.repo.createConnector(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	item, err = service.repo.updateConnectorState(context.Background(), item.EnterpriseID, item.ID, "active", "", true)
	if err != nil {
		t.Fatal(err)
	}
	event := map[string]any{
		"schema": "2.0",
		"header": map[string]any{"event_id": "evt-fast", "event_type": "im.message.receive_v1", "token": "verify-token", "tenant_key": "tenant-external"},
		"event": map[string]any{
			"sender":  map[string]any{"sender_id": map[string]any{"open_id": "ou-fast"}},
			"message": map[string]any{"message_id": fixture.prefix + "_fast_message", "chat_id": "oc-fast", "chat_type": "p2p", "message_type": "text", "content": `{"text":"生成商品图"}`},
		},
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	invoke := func() (*httptest.ResponseRecorder, time.Duration) {
		req := httptest.NewRequest(http.MethodPost, "/api/open/connectors/feishu/events/"+item.ConnectorKey, bytes.NewReader(raw))
		req.SetPathValue("connectorKey", item.ConnectorKey)
		res := httptest.NewRecorder()
		started := time.Now()
		service.event(res, req)
		return res, time.Since(started)
	}
	first, elapsed := invoke()
	if first.Code != http.StatusOK || elapsed > time.Second {
		t.Fatalf("first callback status=%d elapsed=%s body=%s", first.Code, elapsed, first.Body.String())
	}
	if len(service.queue.local) != 1 {
		t.Fatalf("queued jobs=%d, want 1", len(service.queue.local))
	}
	message, err := service.repo.messageByExternalID(context.Background(), "feishu", fixture.prefix+"_fast_message")
	if err != nil || message.ProcessingStatus != "queued" {
		t.Fatalf("message=%+v err=%v", message, err)
	}
	var storedToken string
	if err := db.QueryRow(`SELECT raw_payload_json #>> '{header,token}' FROM connector_messages WHERE id=$1`, message.ID).Scan(&storedToken); err != nil || storedToken != "[REDACTED]" {
		t.Fatalf("stored token=%q err=%v", storedToken, err)
	}
	second, _ := invoke()
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"duplicate":true`) || len(service.queue.local) != 1 {
		t.Fatalf("duplicate callback status=%d queued=%d body=%s", second.Code, len(service.queue.local), second.Body.String())
	}
}

func TestConnectorRepositoryIdempotencyAndTenantIsolation(t *testing.T) {
	_, db := enterpriseP0TestStore(t)
	fixture := seedEnterpriseP0Fixture(t, db, 2, 100)
	repo := connectorRepository{db: db}
	ctx := context.Background()
	connectors := make([]enterpriseConnector, 0, 2)
	for index, tenantID := range fixture.tenantIDs {
		item, err := repo.createConnector(ctx, enterpriseConnector{
			ID: newConnectorID("connector_test"), EnterpriseID: tenantID, ConnectorType: "feishu",
			ConnectorName: "test", ConnectorKey: newConnectorID("connector_key"), AppID: "app",
			AppSecretEncrypted: "enc", VerificationTokenEncrypted: "enc", Config: defaultConnectorConfig(),
		})
		if err != nil {
			t.Fatalf("create connector %d: %v", index, err)
		}
		connectors = append(connectors, item)
	}
	for index, tenantID := range fixture.tenantIDs {
		items, err := repo.listConnectors(ctx, tenantID)
		if err != nil || len(items) != 1 || items[0].ID != connectors[index].ID {
			t.Fatalf("tenant %d connector scope leaked: items=%+v err=%v", index, items, err)
		}
	}
	message := connector.IncomingMessage{Platform: "feishu", ExternalMessageID: fixture.prefix + "_message", ExternalChatID: "chat", ExternalUserID: "external-user", ChatType: "p2p", MessageType: "text", Text: "生成商品图"}
	first, inserted, err := repo.insertIncomingMessage(ctx, connectors[0], message, map[string]any{"event": "safe"})
	if err != nil || !inserted {
		t.Fatalf("first message insert: inserted=%v err=%v", inserted, err)
	}
	second, inserted, err := repo.insertIncomingMessage(ctx, connectors[0], message, map[string]any{"event": "duplicate"})
	if err != nil || inserted || second.ID != first.ID {
		t.Fatalf("duplicate message insert: first=%+v second=%+v inserted=%v err=%v", first, second, inserted, err)
	}
	binding, err := repo.loadOrCreateBinding(ctx, connectors[0], first)
	if err != nil || binding.EnterpriseID != fixture.tenantIDs[0] || binding.InternalUserID == "" {
		t.Fatalf("binding mismatch: binding=%+v err=%v", binding, err)
	}
	var otherTenantMembership int
	if err := db.QueryRow(`SELECT count(*) FROM xz_tenant_members WHERE tenant_id=$1 AND user_id=$2`, fixture.tenantIDs[1], binding.InternalUserID).Scan(&otherTenantMembership); err != nil || otherTenantMembership != 0 {
		t.Fatalf("binding leaked to tenant B: count=%d err=%v", otherTenantMembership, err)
	}
	otherMessage := connector.IncomingMessage{Platform: "feishu", ExternalMessageID: fixture.prefix + "_message_b", ExternalChatID: "chat-b", ExternalUserID: "external-user-b", ChatType: "p2p", MessageType: "text", Text: "生成商品图"}
	otherRecord, inserted, err := repo.insertIncomingMessage(ctx, connectors[1], otherMessage, map[string]any{"event": "safe"})
	if err != nil || !inserted {
		t.Fatalf("tenant B message insert: inserted=%v err=%v", inserted, err)
	}
	otherBinding, err := repo.loadOrCreateBinding(ctx, connectors[1], otherRecord)
	if err != nil {
		t.Fatalf("tenant B binding: %v", err)
	}
	if _, err := repo.updateBinding(ctx, fixture.tenantIDs[0], connectors[0].ID, binding.ID, connectorBindingUpdateRequest{
		InternalUserID: otherBinding.InternalUserID, Permission: map[string]any{"imageGenerate": true}, Status: "active",
	}); !errors.Is(err, errEnterpriseInvalid) {
		t.Fatalf("cross-tenant binding remap was not rejected: %v", err)
	}
	unchanged, found, err := repo.bindingByExternalUser(ctx, connectors[0].ID, message.ExternalUserID)
	if err != nil || !found || unchanged.InternalUserID != binding.InternalUserID {
		t.Fatalf("binding changed after rejected remap: binding=%+v err=%v", unchanged, err)
	}
	taskInput := connectorTaskRecord{EnterpriseID: fixture.tenantIDs[0], ConnectorID: connectors[0].ID, BindingID: binding.ID, ExternalChatID: "chat", ExternalMessageID: message.ExternalMessageID, TaskType: connector.IntentImageGenerate, Intent: connector.IntentImageGenerate, OriginalText: message.Text, OptimizedPrompt: "prompt", Status: "pending"}
	task, created, err := repo.createConnectorTask(ctx, taskInput)
	if err != nil || !created {
		t.Fatalf("first task create: task=%+v created=%v err=%v", task, created, err)
	}
	duplicate, created, err := repo.createConnectorTask(ctx, taskInput)
	if err != nil || created || duplicate.ID != task.ID {
		t.Fatalf("duplicate task create: task=%+v duplicate=%+v created=%v err=%v", task, duplicate, created, err)
	}
}

func TestConnectorEndToEndGenerationAndDuplicateDelivery(t *testing.T) {
	store, db := enterpriseP0TestStore(t)
	fixture := seedEnterpriseP0Fixture(t, db, 1, 100)
	ctx := context.Background()
	var tokenCalls, messageCalls, imageUploads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/v3/tenant_access_token/internal":
			tokenCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"tenant-token","expire":3600}`))
		case "/im/v1/images":
			imageUploads.Add(1)
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				t.Errorf("parse image upload: %v", err)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"image_key":"img-key"}}`))
		case "/im/v1/messages":
			id := messageCalls.Add(1)
			_, _ = fmt.Fprintf(w, `{"code":0,"data":{"message_id":"om-out-%d"}}`, id)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	masterKey := "connector-integration-secret-key-at-least-32-bytes"
	cipher, err := storagecenter.NewSecretCipher(masterKey)
	if err != nil {
		t.Fatal(err)
	}
	provider := &connectorIntegrationImageProvider{}
	service := generation.NewService(provider, nil, nil)
	generator := api{
		store: store, generationService: service, connectorGenerationService: &service,
		cfg: config.Config{ModelTimeoutMS: "30000"},
	}
	connectorService := &connectorAPI{
		cfg: config.Config{
			FeishuAPIBaseURL: server.URL, FeishuHTTPTimeoutSeconds: "5",
			ConnectorSecretEncryptionKey: masterKey,
		},
		repo: connectorRepository{db: db}, generator: generator, cipher: cipher,
	}
	item := enterpriseConnector{
		ID: newConnectorID("connector_e2e"), EnterpriseID: fixture.tenantIDs[0], ConnectorType: "feishu",
		ConnectorName: "Feishu E2E", ConnectorKey: newConnectorID("connector_key"), AppID: "app-e2e",
		Config: defaultConnectorConfig(),
	}
	item.Config.DefaultImageModel = "gpt-image-2"
	item.AppSecretEncrypted, err = cipher.Encrypt("app-secret", item.ID+":app_secret")
	if err != nil {
		t.Fatal(err)
	}
	item.VerificationTokenEncrypted, err = cipher.Encrypt("verify-token", item.ID+":verification_token")
	if err != nil {
		t.Fatal(err)
	}
	item, err = connectorService.repo.createConnector(ctx, item)
	if err != nil {
		t.Fatal(err)
	}
	item, err = connectorService.repo.updateConnectorState(ctx, item.EnterpriseID, item.ID, "active", "", true)
	if err != nil {
		t.Fatal(err)
	}

	externalMessageID := fixture.prefix + "_connector_message"
	record, inserted, err := connectorService.repo.insertIncomingMessage(ctx, item, connector.IncomingMessage{
		Platform: "feishu", ExternalMessageID: externalMessageID, ExternalChatID: "oc-chat",
		ExternalUserID: "ou-user", ChatType: "p2p", MessageType: "text", Text: "生成一张猫咪电商主图",
	}, map[string]any{"event": "sanitized"})
	if err != nil || !inserted {
		t.Fatalf("insert inbound message: inserted=%v err=%v", inserted, err)
	}
	if err := connectorService.processJob(ctx, connectorJob{MessageID: externalMessageID}); err != nil {
		t.Fatalf("process connector job: %v", err)
	}
	task, err := connectorService.repo.taskByExternalMessage(ctx, "feishu", externalMessageID)
	if err != nil || task.Status != "succeeded" || intValue(task.Result["imagesSent"]) != 1 || task.PlatformTaskID == "" || task.ExternalUserID != "ou-user" || task.ExternalUserName == "" {
		t.Fatalf("connector task=%+v err=%v", task, err)
	}
	message, err := connectorService.repo.messageByExternalID(ctx, "feishu", externalMessageID)
	if err != nil || message.ID != record.ID || message.ProcessingStatus != "completed" {
		t.Fatalf("message=%+v err=%v", message, err)
	}
	var assetCount int
	if err := db.QueryRow(`SELECT count(*) FROM xz_assets WHERE task_id=$1 AND tenant_id=$2`, task.PlatformTaskID, item.EnterpriseID).Scan(&assetCount); err != nil || assetCount != 1 {
		t.Fatalf("asset count=%d err=%v", assetCount, err)
	}
	var walletBalance int
	if err := db.QueryRow(`SELECT point_balance FROM xz_tenant_wallets WHERE tenant_id=$1`, item.EnterpriseID).Scan(&walletBalance); err != nil || walletBalance != 90 {
		t.Fatalf("wallet balance=%d err=%v", walletBalance, err)
	}
	if provider.calls.Load() != 1 || tokenCalls.Load() != 1 || imageUploads.Load() != 1 || messageCalls.Load() != 3 {
		t.Fatalf("calls provider=%d token=%d uploads=%d messages=%d", provider.calls.Load(), tokenCalls.Load(), imageUploads.Load(), messageCalls.Load())
	}
	if err := connectorService.processJob(ctx, connectorJob{MessageID: externalMessageID}); err != nil {
		t.Fatalf("process duplicate job: %v", err)
	}
	if provider.calls.Load() != 1 || imageUploads.Load() != 1 || messageCalls.Load() != 3 {
		t.Fatalf("duplicate delivery repeated side effects: provider=%d uploads=%d messages=%d", provider.calls.Load(), imageUploads.Load(), messageCalls.Load())
	}
	if strings.TrimSpace(task.OptimizedPrompt) == "" || !strings.Contains(task.OptimizedPrompt, "猫咪") {
		t.Fatalf("optimized prompt=%q", task.OptimizedPrompt)
	}

	var sourceAssetID string
	if err := db.QueryRow(`SELECT id FROM xz_assets WHERE task_id=$1 AND tenant_id=$2 ORDER BY created_at DESC LIMIT 1`, task.PlatformTaskID, item.EnterpriseID).Scan(&sourceAssetID); err != nil {
		t.Fatal(err)
	}
	editExternalMessageID := fixture.prefix + "_connector_edit_message"
	_, inserted, err = connectorService.repo.insertIncomingMessage(ctx, item, connector.IncomingMessage{
		Platform: "feishu", ExternalMessageID: editExternalMessageID, ExternalChatID: "oc-chat",
		ExternalUserID: "ou-user", ChatType: "p2p", MessageType: "text", Text: "将上面电商图加上京东logo",
	}, map[string]any{"event": "sanitized"})
	if err != nil || !inserted {
		t.Fatalf("insert edit message: inserted=%v err=%v", inserted, err)
	}
	if err := connectorService.processJob(ctx, connectorJob{MessageID: editExternalMessageID}); err != nil {
		t.Fatalf("process connector edit job: %v", err)
	}
	editTask, err := connectorService.repo.taskByExternalMessage(ctx, "feishu", editExternalMessageID)
	if err != nil || editTask.Status != "succeeded" || editTask.Intent != connector.IntentImageEdit {
		t.Fatalf("connector edit task=%+v err=%v", editTask, err)
	}
	if stringValue(editTask.Result["sourceReferenceAssetId"]) != sourceAssetID || !boolValue(editTask.Result["editMode"]) {
		t.Fatalf("connector edit result did not retain reference lineage: %+v", editTask.Result)
	}
	request, ok := provider.last.Load().(generation.CreateRequest)
	if !ok || request.Type != "IMAGE_TO_IMAGE" {
		t.Fatalf("provider edit request=%+v", request)
	}
	references, ok := request.Params["referenceImages"].([]any)
	if !ok || len(references) != 1 || stringValue(references[0].(map[string]any)["assetId"]) != sourceAssetID {
		t.Fatalf("provider reference images=%#v", request.Params["referenceImages"])
	}
	if !strings.Contains(request.Prompt, "保持原图") || !strings.Contains(request.Prompt, "京东logo") {
		t.Fatalf("provider edit prompt=%q", request.Prompt)
	}
	if err := db.QueryRow(`SELECT point_balance FROM xz_tenant_wallets WHERE tenant_id=$1`, item.EnterpriseID).Scan(&walletBalance); err != nil || walletBalance != 80 {
		t.Fatalf("wallet balance after edit=%d err=%v", walletBalance, err)
	}
}

func TestConnectorGenerationInsufficientBalanceAndFailureRelease(t *testing.T) {
	t.Run("insufficient balance does not call provider or create task", func(t *testing.T) {
		provider := &connectorIntegrationImageProvider{}
		generator, db, fixture, userID := connectorGenerationTestSubject(t, 0, provider)
		clientRequestID := "feishu:" + fixture.prefix + "_insufficient"
		_, _, err := generator.executeConnectorImageGeneration(context.Background(), userID, connectorGenerationTestRequest(clientRequestID))
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "insufficient") {
			t.Fatalf("error=%v", err)
		}
		if provider.calls.Load() != 0 {
			t.Fatalf("provider calls=%d, want 0", provider.calls.Load())
		}
		var taskCount, balance int
		if err := db.QueryRow(`SELECT count(*) FROM xz_generation_tasks WHERE client_request_id=$1`, clientRequestID).Scan(&taskCount); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT point_balance FROM xz_tenant_wallets WHERE tenant_id=$1`, fixture.tenantIDs[0]).Scan(&balance); err != nil {
			t.Fatal(err)
		}
		if taskCount != 0 || balance != 0 {
			t.Fatalf("task count=%d balance=%d", taskCount, balance)
		}
	})

	t.Run("upstream failure releases enterprise reservation", func(t *testing.T) {
		provider := &connectorFailingImageProvider{}
		generator, db, fixture, userID := connectorGenerationTestSubject(t, 100, provider)
		clientRequestID := "feishu:" + fixture.prefix + "_provider_failure"
		_, _, err := generator.executeConnectorImageGeneration(context.Background(), userID, connectorGenerationTestRequest(clientRequestID))
		if err == nil || !strings.Contains(err.Error(), "simulated upstream failure") {
			t.Fatalf("error=%v", err)
		}
		if provider.calls.Load() != 1 {
			t.Fatalf("provider calls=%d, want 1", provider.calls.Load())
		}
		var status, billingState string
		var released float64
		if err := db.QueryRow(`
			SELECT status,billing_status,released_points FROM xz_generation_tasks WHERE client_request_id=$1
		`, clientRequestID).Scan(&status, &billingState, &released); err != nil {
			t.Fatal(err)
		}
		var balance int
		if err := db.QueryRow(`SELECT point_balance FROM xz_tenant_wallets WHERE tenant_id=$1`, fixture.tenantIDs[0]).Scan(&balance); err != nil {
			t.Fatal(err)
		}
		if status != "FAILED" || billingState != billingStatusReleased || released != 10 || balance != 100 {
			t.Fatalf("status=%s billing=%s released=%.0f balance=%d", status, billingState, released, balance)
		}
	})
}

func TestConnectorModelInfoText(t *testing.T) {
	if got := connectorModelInfoText(" gpt-image-2 "); !strings.Contains(got, "gpt-image-2") || !strings.Contains(got, "视频和 PPT") {
		t.Fatalf("model info text = %q", got)
	}
	if got := connectorModelInfoText(" "); !strings.Contains(got, "尚未配置") {
		t.Fatalf("empty model info text = %q", got)
	}
}

func TestConnectorCapabilityInfoText(t *testing.T) {
	text := connectorCapabilityInfoText()
	for _, expected := range []string{"生图", "改图", "生视频", "图生视频", "生 PPT", "查任务", "作品中心"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("capability info missing %q: %s", expected, text)
		}
	}
}

func connectorGenerationTestSubject(t *testing.T, balance int64, provider generation.ImageProvider) (api, *sql.DB, enterpriseP0Fixture, string) {
	t.Helper()
	store, db := enterpriseP0TestStore(t)
	fixture := seedEnterpriseP0Fixture(t, db, 1, balance)
	repo := connectorRepository{db: db}
	item, err := repo.createConnector(context.Background(), enterpriseConnector{
		ID: newConnectorID("connector_billing"), EnterpriseID: fixture.tenantIDs[0], ConnectorType: "feishu",
		ConnectorName: "Billing Test", ConnectorKey: newConnectorID("connector_key"), AppID: "app-billing",
		AppSecretEncrypted: "enc", VerificationTokenEncrypted: "enc", Config: defaultConnectorConfig(),
	})
	if err != nil {
		t.Fatal(err)
	}
	record, inserted, err := repo.insertIncomingMessage(context.Background(), item, connector.IncomingMessage{
		Platform: "feishu", ExternalMessageID: fixture.prefix + "_binding_message", ExternalChatID: "oc-billing",
		ExternalUserID: "ou-billing", ChatType: "p2p", MessageType: "text", Text: "生成商品图",
	}, map[string]any{"event": "billing-test"})
	if err != nil || !inserted {
		t.Fatalf("insert binding message: inserted=%v err=%v", inserted, err)
	}
	binding, err := repo.loadOrCreateBinding(context.Background(), item, record)
	if err != nil {
		t.Fatal(err)
	}
	service := generation.NewService(provider, nil, nil)
	return api{
		store: store, generationService: service, connectorGenerationService: &service,
		cfg: config.Config{ModelTimeoutMS: "30000"},
	}, db, fixture, binding.InternalUserID
}

func connectorGenerationTestRequest(clientRequestID string) generation.CreateRequest {
	return generation.CreateRequest{
		ClientRequestID: clientRequestID, Prompt: "commercial product image", Model: "gpt-image-2",
		Params: map[string]any{
			"size": "1024x1024", "count": 1, "n": 1,
			"source_type": "feishu", "source_id": clientRequestID, "operator_external_id": "ou-billing",
		},
	}
}
