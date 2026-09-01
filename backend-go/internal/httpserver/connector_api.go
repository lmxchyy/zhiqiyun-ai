package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/connector"
	feishuconnector "xianzhi-ai/backend-go/internal/connector/feishu"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

const (
	maxConnectorEventBytes          = 1 << 20
	connectorFailureHandlingTimeout = 30 * time.Second
)

type connectorAPI struct {
	cfg        config.Config
	repo       connectorRepository
	enterprise enterpriseAPI
	generator  api
	queue      *connectorJobQueue
	cipher     *storagecenter.SecretCipher
	cipherErr  error
	redis      *redis.Client
	limiter    *connectorRateLimiter
}

func newConnectorAPI(cfg config.Config, store platformStore, enterprise enterpriseAPI, generator api, redisClient *redis.Client) *connectorAPI {
	result := &connectorAPI{cfg: cfg, enterprise: enterprise, generator: generator, redis: redisClient, limiter: newConnectorRateLimiter()}
	pgStore, ok := store.(*postgresStore)
	if !ok || pgStore.db == nil {
		return result
	}
	result.repo = connectorRepository{db: pgStore.db}
	result.queue = newConnectorJobQueue(redisClient, cfg.ConnectorQueuePrefix)
	result.cipher, result.cipherErr = storagecenter.NewSecretCipher(cfg.ConnectorSecretEncryptionKey)
	go result.queue.Run(context.Background(), result.processJob)
	return result
}

func (a *connectorAPI) available(w http.ResponseWriter) bool {
	if a == nil || a.repo.db == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("enterprise connector store requires PostgreSQL"))
		return false
	}
	return true
}

func (a *connectorAPI) list(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	access, ok := a.enterprise.require(w, r, "enterprise.connector.read")
	if !ok {
		return
	}
	items, err := a.repo.listConnectors(r.Context(), access.TenantID)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	views := make([]connectorView, 0, len(items))
	for _, item := range items {
		views = append(views, a.view(item))
	}
	writeJSON(w, map[string]any{"items": views, "total": len(views)})
}

func (a *connectorAPI) getFeishu(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	access, ok := a.enterprise.require(w, r, "enterprise.connector.read")
	if !ok {
		return
	}
	item, found, err := a.repo.connectorForEnterprise(r.Context(), access.TenantID, "feishu")
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	if !found {
		writeJSON(w, map[string]any{"configured": false, "config": defaultConnectorConfig()})
		return
	}
	writeJSON(w, a.view(item))
}

func (a *connectorAPI) createFeishu(w http.ResponseWriter, r *http.Request) {
	a.saveFeishu(w, r, true)
}

func (a *connectorAPI) updateFeishu(w http.ResponseWriter, r *http.Request) {
	a.saveFeishu(w, r, false)
}

func (a *connectorAPI) saveFeishu(w http.ResponseWriter, r *http.Request, create bool) {
	if !a.available(w) {
		return
	}
	access, ok := a.enterprise.require(w, r, "enterprise.connector.manage")
	if !ok {
		return
	}
	var request connectorSaveRequest
	if !decodeEnterpriseJSON(w, r, &request) {
		return
	}
	request.AppID = strings.TrimSpace(request.AppID)
	if request.AppID == "" {
		writeConnectorError(w, errors.New("Feishu App ID is required"))
		return
	}
	existing, found, err := a.repo.connectorForEnterprise(r.Context(), access.TenantID, "feishu")
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	if create && found {
		writeConnectorError(w, errEnterpriseConflict)
		return
	}
	if !create && !found {
		writeConnectorError(w, errEnterpriseNotFound)
		return
	}
	item := existing
	if !found {
		item = enterpriseConnector{ID: newConnectorID("connector"), EnterpriseID: access.TenantID, ConnectorType: "feishu", ConnectorKey: newConnectorID("fsc"), Status: "disabled"}
	}
	item.ConnectorName = firstNonEmptyString(strings.TrimSpace(request.ConnectorName), "飞书机器人")
	item.AppID = request.AppID
	if request.Config == (connectorConfig{}) {
		item.Config = defaultConnectorConfig()
	} else {
		item.Config = normalizeConnectorConfig(request.Config)
	}
	if err := a.encryptUpdatedSecrets(&item, request); err != nil {
		writeConnectorError(w, err)
		return
	}
	if item.AppSecretEncrypted == "" || item.VerificationTokenEncrypted == "" {
		writeConnectorError(w, errors.New("Feishu App Secret and Verification Token are required"))
		return
	}
	if found {
		item, err = a.repo.updateConnector(r.Context(), item)
	} else {
		item, err = a.repo.createConnector(r.Context(), item)
	}
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	_ = insertTenantAuditDirect(r.Context(), a.repo.db, access, "enterprise.connector.save", "enterprise_connector", item.ID, "", map[string]any{"connectorType": "feishu", "appId": redactConnectorValue(item.AppID)})
	if !found {
		w.WriteHeader(http.StatusCreated)
	}
	writeJSON(w, a.view(item))
}

func (a *connectorAPI) testFeishu(w http.ResponseWriter, r *http.Request) {
	a.testOrChangeState(w, r, "test")
}

func (a *connectorAPI) enableFeishu(w http.ResponseWriter, r *http.Request) {
	a.testOrChangeState(w, r, "enable")
}

func (a *connectorAPI) disableFeishu(w http.ResponseWriter, r *http.Request) {
	a.testOrChangeState(w, r, "disable")
}

func (a *connectorAPI) testOrChangeState(w http.ResponseWriter, r *http.Request, action string) {
	if !a.available(w) {
		return
	}
	access, ok := a.enterprise.require(w, r, "enterprise.connector.manage")
	if !ok {
		return
	}
	item, found, err := a.repo.connectorForEnterprise(r.Context(), access.TenantID, "feishu")
	if err != nil || !found {
		if err == nil {
			err = errEnterpriseNotFound
		}
		writeConnectorError(w, err)
		return
	}
	if action == "disable" {
		item, err = a.repo.updateConnectorState(r.Context(), access.TenantID, item.ID, "disabled", "", false)
	} else {
		client, clientErr := a.feishuClient(item)
		if clientErr != nil {
			writeConnectorError(w, clientErr)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), a.cfg.FeishuHTTPTimeout())
		var botInfo feishuconnector.BotInfo
		botInfo, err = client.BotInfo(ctx)
		cancel()
		if err != nil {
			_, _ = a.repo.updateConnectorState(r.Context(), access.TenantID, item.ID, "error", friendlyConnectorError(err), false)
			writeConnectorError(w, err)
			return
		}
		if err = a.repo.updateConnectorBotOpenID(r.Context(), access.TenantID, item.ID, botInfo.OpenID); err != nil {
			writeConnectorError(w, err)
			return
		}
		status := item.Status
		if action == "enable" {
			status = "active"
		}
		item, err = a.repo.updateConnectorState(r.Context(), access.TenantID, item.ID, status, "", true)
	}
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	_ = insertTenantAuditDirect(r.Context(), a.repo.db, access, "enterprise.connector."+action, "enterprise_connector", item.ID, "", map[string]any{"connectorType": "feishu", "status": item.Status})
	writeJSON(w, map[string]any{"ok": true, "connector": a.view(item)})
}

func (a *connectorAPI) users(w http.ResponseWriter, r *http.Request) {
	access, item, ok := a.requireFeishu(w, r, "enterprise.connector.read")
	if !ok {
		return
	}
	items, err := a.repo.listBindings(r.Context(), access.TenantID, item.ID, connectorLimit(r, 100))
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	for index := range items {
		items[index].DailyQuota = item.Config.MemberDailyQuota
		if memberQuota := intValue(items[index].Permission["dailyQuota"]); memberQuota > 0 {
			items[index].DailyQuota = memberQuota
		}
	}
	writeJSON(w, map[string]any{"items": items, "total": len(items)})
}

func (a *connectorAPI) updateUser(w http.ResponseWriter, r *http.Request) {
	access, item, ok := a.requireFeishu(w, r, "enterprise.connector.manage")
	if !ok {
		return
	}
	var request connectorBindingUpdateRequest
	if !decodeEnterpriseJSON(w, r, &request) {
		return
	}
	updated, err := a.repo.updateBinding(r.Context(), access.TenantID, item.ID, r.PathValue("id"), request)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	_ = insertTenantAuditDirect(r.Context(), a.repo.db, access, "enterprise.connector.user.update", "connector_user_binding", updated.ID, updated.InternalUserID, map[string]any{"status": updated.Status})
	writeJSON(w, updated)
}

func (a *connectorAPI) logs(w http.ResponseWriter, r *http.Request) {
	access, item, ok := a.requireFeishu(w, r, "enterprise.connector.read")
	if !ok {
		return
	}
	items, err := a.repo.listMessages(r.Context(), access.TenantID, item.ID, connectorLimit(r, 100))
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": len(items)})
}

func (a *connectorAPI) tasks(w http.ResponseWriter, r *http.Request) {
	access, item, ok := a.requireFeishu(w, r, "enterprise.connector.read")
	if !ok {
		return
	}
	items, err := a.repo.listTasks(r.Context(), access.TenantID, item.ID, r.URL.Query().Get("capability"), r.URL.Query().Get("status"), connectorLimit(r, 100))
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": len(items)})
}

func (a *connectorAPI) retryDelivery(w http.ResponseWriter, r *http.Request) {
	access, item, ok := a.requireFeishu(w, r, "enterprise.connector.manage")
	if !ok {
		return
	}
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	task, err := a.repo.taskByIDForEnterprise(r.Context(), access.TenantID, item.ID, taskID)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	if task.PlatformTaskID == "" || (task.UnifiedStatus != "completed" && task.UnifiedStatus != "delivery_failed") {
		writeError(w, http.StatusConflict, errors.New("only successfully generated tasks can be delivered again"))
		return
	}
	binding, found, err := a.repo.bindingByExternalUser(r.Context(), item.ID, task.ExternalUserID)
	if err != nil || !found || binding.InternalUserID == "" {
		if err == nil {
			err = errors.New("connector user binding not found")
		}
		writeConnectorError(w, err)
		return
	}
	stored, found, err := a.repo.assetForInternalTask(r.Context(), binding.InternalUserID, task.PlatformTaskID)
	if err != nil || !found {
		if err == nil {
			err = errors.New("generated asset not found")
		}
		writeConnectorError(w, err)
		return
	}
	signed := a.generator.signStoredAssetURLs(r.Context(), binding.InternalUserID, []asset{stored})
	if len(signed) == 0 || strings.TrimSpace(signed[0].URL) == "" {
		writeConnectorError(w, errors.New("generated asset access URL is unavailable"))
		return
	}
	client, err := a.feishuClient(item)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	result := connector.CapabilityResult{
		InternalTaskID: task.PlatformTaskID,
		Status:         "completed",
		Progress:       100,
		ActualCost:     task.PointsCost,
		AssetIDs:       []string{stored.ID},
		Data: map[string]any{
			"downloadURL":     signed[0].URL,
			"connectorTaskId": task.ID,
			"redelivery":      true,
		},
	}
	title := connectorTaskCompletionTitle(task.TaskType)
	message := connectorResultMessage(title, connector.AICommand{Intent: task.TaskType}, result)
	sent, sendErr := client.SendCard(r.Context(), connector.MessageTarget{ChatID: task.ExternalChatID}, message)
	if sendErr != nil {
		_ = a.repo.markConnectorDelivery(r.Context(), task.ID, "failed", false, sendErr.Error())
		writeConnectorError(w, sendErr)
		return
	}
	_ = a.repo.insertOutboundMessage(r.Context(), item, task.ExternalChatID, task.ExternalUserID, sent.ExternalMessageID, "card", map[string]any{"taskId": task.ID, "redelivery": true})
	_ = a.repo.markConnectorDelivery(r.Context(), task.ID, "delivered", true, "")
	_ = insertTenantAuditDirect(r.Context(), a.repo.db, access, "enterprise.connector.task.retry_delivery", "connector_ai_task", task.ID, binding.InternalUserID, map[string]any{"platformTaskId": task.PlatformTaskID})
	writeJSON(w, map[string]any{"ok": true, "taskId": task.ID, "deliveryStatus": "delivered"})
}

func connectorTaskCompletionTitle(taskType string) string {
	switch taskType {
	case connector.IntentImageGenerate:
		return "图片生成完成"
	case connector.IntentImageEdit:
		return "图片修改完成"
	case connector.IntentVideoGenerate:
		return "视频生成完成"
	case connector.IntentVideoImageToVideo:
		return "图生视频完成"
	case connector.IntentPPTGenerate:
		return "PPT 生成完成"
	default:
		return "AI 任务完成"
	}
}

func (a *connectorAPI) requireFeishu(w http.ResponseWriter, r *http.Request, permission string) (enterpriseAccess, enterpriseConnector, bool) {
	if !a.available(w) {
		return enterpriseAccess{}, enterpriseConnector{}, false
	}
	access, ok := a.enterprise.require(w, r, permission)
	if !ok {
		return enterpriseAccess{}, enterpriseConnector{}, false
	}
	item, found, err := a.repo.connectorForEnterprise(r.Context(), access.TenantID, "feishu")
	if err != nil || !found {
		if err == nil {
			err = errEnterpriseNotFound
		}
		writeConnectorError(w, err)
		return enterpriseAccess{}, enterpriseConnector{}, false
	}
	return access, item, true
}

func (a *connectorAPI) event(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	connectorKey := strings.TrimSpace(r.PathValue("connectorKey"))
	if connectorKey == "" || !a.limiter.Allow(connectorKey+"|"+connectorRemoteIP(r), 120, time.Minute) {
		writeError(w, http.StatusTooManyRequests, errors.New("connector event rate limit exceeded"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxConnectorEventBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid connector event body"))
		return
	}
	item, err := a.repo.connectorByKey(r.Context(), connectorKey)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("connector not found"))
		return
	}
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	client, err := a.feishuClient(item)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	verified, err := client.VerifyEvent(r.Context(), connector.EventRequest{Body: raw, Headers: r.Header.Clone()})
	if err != nil {
		log.Printf("connector=feishu operation=verify connector_id=%s result=denied error=%v", item.ID, err)
		writeError(w, http.StatusUnauthorized, errors.New("invalid Feishu event"))
		return
	}
	parsed, err := client.ParseEvent(r.Context(), verified)
	if err != nil {
		if errors.Is(err, feishuconnector.ErrUnsupportedMessage) {
			writeJSON(w, map[string]any{"ok": true, "ignored": true})
			return
		}
		writeError(w, http.StatusBadRequest, errors.New("invalid Feishu message event"))
		return
	}
	if parsed.Challenge != "" {
		writeJSON(w, map[string]string{"challenge": parsed.Challenge})
		return
	}
	if item.Status != "active" {
		writeError(w, http.StatusForbidden, errors.New("connector is disabled"))
		return
	}
	if parsed.Message == nil {
		writeJSON(w, map[string]any{"ok": true, "ignored": true})
		return
	}
	parsed.Message.MentionedBot = containsConnectorString(parsed.Message.MentionOpenIDs, item.BotOpenID)
	a.repo.updateExternalTenantKey(r.Context(), item.ID, parsed.Message.ExternalTenantKey)
	record, inserted, err := a.repo.insertIncomingMessage(r.Context(), item, *parsed.Message, sanitizeConnectorPayload(verified))
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	if inserted {
		if err := a.queue.Enqueue(r.Context(), connectorJob{MessageID: record.ExternalMessageID}); err != nil {
			_ = a.repo.markMessage(r.Context(), record.ID, "failed", err.Error())
			writeError(w, http.StatusServiceUnavailable, errors.New("connector queue is unavailable"))
			return
		}
		_ = a.repo.markMessage(r.Context(), record.ID, "queued", "")
	}
	writeJSON(w, map[string]any{"ok": true, "duplicate": !inserted})
}

func connectorRemoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func containsConnectorString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func isConnectorImageEditRequest(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return false
	}
	referenceWords := []string{"刚才", "上一张", "上张", "上图", "上面", "这张图", "这张图片", "刚生成", "之前生成", "前一张"}
	editWords := []string{"加上", "添加", "加个", "加一个", "修改", "改成", "换成", "替换", "去掉", "删除", "移除", "调整", "编辑", "logo", "水印", "文字"}
	return containsAnyConnectorText(normalized, referenceWords) && containsAnyConnectorText(normalized, editWords)
}

func containsAnyConnectorText(text string, values []string) bool {
	for _, value := range values {
		if strings.Contains(text, strings.ToLower(value)) {
			return true
		}
	}
	return false
}

func connectorImageEditPrompt(text string) string {
	return "基于提供的参考图进行局部编辑。保持原图的商品主体、构图、背景、光影、色彩、画面比例和清晰度不变，只执行用户明确要求的修改：" + strings.TrimSpace(text) + "。除明确要求外，不要重新设计、替换商品或生成另一套画面。"
}

func (a *connectorAPI) processJob(parent context.Context, job connectorJob) error {
	ctx, cancel := context.WithTimeout(parent, videoGenerationTimeout+5*time.Minute)
	defer cancel()
	message, err := a.repo.messageByExternalID(ctx, "feishu", job.MessageID)
	if err != nil {
		return err
	}
	if message.ProcessingStatus == "completed" || message.ProcessingStatus == "ignored" {
		return nil
	}
	_ = a.repo.markMessage(ctx, message.ID, "processing", "")
	item, err := a.repo.connectorByID(ctx, message.ConnectorID)
	if err != nil {
		return a.failMessage(ctx, message, enterpriseConnector{}, connectorTaskRecord{}, "CONNECTOR_NOT_FOUND", err)
	}
	if item.Status != "active" {
		return a.failMessage(ctx, message, item, connectorTaskRecord{}, "CONNECTOR_DISABLED", errors.New("connector is disabled"))
	}
	client, err := a.feishuClient(item)
	if err != nil {
		return a.failMessage(ctx, message, item, connectorTaskRecord{}, "CONNECTOR_CONFIG", err)
	}
	binding, err := a.repo.loadOrCreateBinding(ctx, item, message)
	if err != nil {
		return a.failMessage(ctx, message, item, connectorTaskRecord{}, "BINDING_FAILED", err)
	}
	a.repo.touchBinding(ctx, binding.ID)
	text := stringValue(message.Content["text"])
	intent := (connector.RuleIntentRouter{}).Route(text, connector.IntentDefaults{
		Size: item.Config.DefaultSize, Count: item.Config.DefaultImageCount, VideoDuration: item.Config.DefaultVideoDuration,
		VideoAspectRatio: item.Config.DefaultVideoAspectRatio, VideoResolution: item.Config.DefaultVideoResolution,
		VideoModelID: item.Config.DefaultVideoModel, PPTPageCount: item.Config.DefaultPPTPageCount,
		PPTTemplateID: item.Config.DefaultPPTTemplate, PPTTheme: item.Config.DefaultPPTTemplate, PPTLanguage: "zh",
		UseEnterpriseLogo: item.Config.PPTUseEnterpriseLogo, UseEnterpriseKnowledge: item.Config.PPTUseEnterpriseKnowledge,
	})
	prompt := strings.TrimSpace(intent.Topic)
	if intent.Name == connector.IntentImageGenerate {
		prompt = (connector.EcommerceImagePromptBuilder{}).Build(intent)
	} else if intent.Name == connector.IntentImageEdit {
		prompt = connectorImageEditPrompt(text)
	}
	modelID := item.Config.DefaultImageModel
	if intent.Name == connector.IntentVideoGenerate || intent.Name == connector.IntentVideoImageToVideo {
		modelID = item.Config.DefaultVideoModel
	} else if intent.Name == connector.IntentPPTGenerate {
		modelID = item.Config.DefaultPPTTemplate
	}
	task, created, err := a.repo.createConnectorTask(ctx, connectorTaskRecord{
		EnterpriseID: item.EnterpriseID, ConnectorID: item.ID, BindingID: binding.ID,
		ExternalChatID: message.ExternalChatID, ExternalMessageID: message.ExternalMessageID,
		TaskType: intent.Name, Intent: intent.Name, OriginalText: text,
		OptimizedPrompt: prompt, ModelID: modelID, Status: "pending", UnifiedStatus: "created", Progress: 0,
	})
	if err != nil {
		return a.failMessage(ctx, message, item, task, "TASK_CREATE_FAILED", err)
	}
	if !created && (task.UnifiedStatus == "completed" || task.UnifiedStatus == "delivery_failed" || task.UnifiedStatus == "failed" || task.Status == "ignored") {
		_ = a.repo.markMessage(ctx, message.ID, "completed", "")
		return nil
	}
	target := connector.MessageTarget{ChatID: message.ExternalChatID}
	if intent.Name == connector.IntentModelInfo {
		responseText := connectorModelInfoText(item.Config.DefaultImageModel)
		result, sendErr := client.SendText(ctx, target, connector.OutgoingMessage{Text: responseText})
		if sendErr != nil {
			return a.failMessage(ctx, message, item, task, "FEISHU_SEND_FAILED", sendErr)
		}
		_ = a.repo.insertOutboundMessage(ctx, item, message.ExternalChatID, message.ExternalUserID, result.ExternalMessageID, "text", map[string]any{"text": responseText})
		_ = a.repo.updateConnectorTask(ctx, task.ID, "ignored", 100, "", 0, map[string]any{"reason": "model_info_query", "model": strings.TrimSpace(item.Config.DefaultImageModel)}, "", "")
		_ = a.repo.markMessage(ctx, message.ID, "completed", "")
		return nil
	}
	if intent.Name == connector.IntentCapabilityInfo {
		responseText := connectorCapabilityInfoText()
		result, sendErr := client.SendText(ctx, target, connector.OutgoingMessage{Text: responseText})
		if sendErr != nil {
			return a.failMessage(ctx, message, item, task, "FEISHU_SEND_FAILED", sendErr)
		}
		_ = a.repo.insertOutboundMessage(ctx, item, message.ExternalChatID, message.ExternalUserID, result.ExternalMessageID, "text", map[string]any{"text": responseText})
		_ = a.repo.updateConnectorTask(ctx, task.ID, "ignored", 100, "", 0, map[string]any{"reason": "capability_info_query"}, "", "")
		_ = a.repo.markMessage(ctx, message.ID, "completed", "")
		return nil
	}
	if intent.Name == connector.IntentUnknown || intent.Name == connector.IntentHelp {
		result, sendErr := client.SendText(ctx, target, connector.OutgoingMessage{Text: connectorCapabilityInfoText()})
		if sendErr == nil {
			_ = a.repo.insertOutboundMessage(ctx, item, message.ExternalChatID, message.ExternalUserID, result.ExternalMessageID, "text", map[string]any{"text": "help"})
		}
		_ = a.repo.updateConnectorTask(ctx, task.ID, "ignored", 100, "", 0, map[string]any{"reason": "unsupported_intent"}, "", "")
		_ = a.repo.markMessage(ctx, message.ID, "ignored", "")
		return sendErr
	}
	command := connectorCommandFromIntent(item, binding, message, intent, prompt)
	if intent.ReferenceAssetRequested {
		ref, found, lookupErr := a.repo.latestSuccessfulReferenceImage(ctx, item.EnterpriseID, item.ID, binding.ID, message.ExternalChatID)
		if lookupErr != nil {
			return a.failMessage(ctx, message, item, task, "REFERENCE_LOOKUP_FAILED", lookupErr)
		}
		if found {
			command.ReferenceAssets = []connector.ReferenceAsset{{ID: ref.AssetID, TaskID: ref.GenerationTaskID, Name: ref.Name, MediaType: "image", URL: ref.URL}}
		}
	}
	if session, found, sessionErr := a.repo.sessionContext(ctx, item.EnterpriseID, item.ID, message.ExternalChatID, message.ExternalUserID); sessionErr == nil && found {
		command.Context = map[string]any{"last_intent": session.LastIntent, "last_task_type": session.LastTaskType, "last_task_id": session.LastTaskID, "last_asset_ids": session.LastAssetIDs, "last_topic": session.LastTopic, "last_parameters": session.LastParameters, "last_prompt": session.LastPrompt}
	}
	runtime := &connectorCapabilityRuntime{api: a, item: item, binding: binding, message: message, task: task, client: client}
	var handler connector.CapabilityHandler
	for _, candidate := range connectorHandlers(runtime) {
		if candidate.CanHandle(command) {
			handler = candidate
			break
		}
	}
	if handler == nil {
		return a.failMessage(ctx, message, item, task, "UNSUPPORTED_INTENT", errors.New("unsupported connector capability"))
	}
	if err := handler.Validate(ctx, command); err != nil {
		return a.failMessage(ctx, message, item, task, "PERMISSION_DENIED", err)
	}
	estimated, err := handler.EstimateCost(ctx, command)
	if err != nil {
		return a.failMessage(ctx, message, item, task, "ESTIMATE_FAILED", err)
	}
	if err := a.validateConnectorQuota(ctx, item, binding, intent.Name, estimated); err != nil {
		return a.failMessage(ctx, message, item, task, "QUOTA_EXCEEDED", err)
	}
	_ = a.repo.setConnectorTaskEstimate(ctx, task.ID, estimated, modelID)
	if intent.Name != connector.IntentTaskQuery {
		progress := connectorTaskCreatedMessage(intent, task.ID, estimated)
		sent, sendErr := client.SendCard(ctx, target, progress)
		if sendErr != nil {
			fallback := fmt.Sprintf("任务已创建，正在处理。任务编号：%s，预计消耗：%d 积分。", task.ID, estimated)
			sent, sendErr = client.SendText(ctx, target, connector.OutgoingMessage{Text: fallback})
			if sendErr != nil {
				return a.failMessage(ctx, message, item, task, "FEISHU_SEND_FAILED", sendErr)
			}
		}
		_ = a.repo.insertOutboundMessage(ctx, item, message.ExternalChatID, message.ExternalUserID, sent.ExternalMessageID, "card", map[string]any{"taskId": task.ID, "status": "created"})
	}
	_ = a.repo.updateConnectorTaskState(ctx, task.ID, "processing", "processing", "processing", 15, "", 0, map[string]any{"estimatedPoints": estimated}, "", "")
	capabilityResult, err := handler.Execute(ctx, command)
	if err != nil {
		if errors.Is(err, pe.ErrUnknownResubmitBlocked) || errors.Is(err, pe.ErrProviderStillProcessing) {
			// Keep the connector message/task retryable while the durable provider
			// execution is being queried; failMessage would make recovery terminal.
			return err
		}
		return a.failMessage(ctx, message, item, task, "GENERATION_FAILED", err)
	}
	resultPayload := connectorCapabilityResultPayload(capabilityResult, command)
	_ = a.repo.updateConnectorTaskState(ctx, task.ID, "succeeded", "uploading", "uploading", 95, capabilityResult.InternalTaskID, capabilityResult.ActualCost, resultPayload, "", "")
	if err := a.deliverCapabilityResult(ctx, runtime, handler, command, capabilityResult); err != nil {
		_ = a.repo.markConnectorDelivery(ctx, task.ID, "failed", false, err.Error())
		_ = a.repo.markMessage(ctx, message.ID, "completed", "")
		log.Printf("connector=feishu operation=deliver task_id=%s result=failed error=%v", task.ID, err)
		return nil
	}
	switch intent.Name {
	case connector.IntentImageGenerate, connector.IntentImageEdit:
		resultPayload["imagesSent"] = len(capabilityResult.AssetIDs)
	case connector.IntentVideoGenerate, connector.IntentVideoImageToVideo, connector.IntentPPTGenerate:
		resultPayload["filesSent"] = 1
	}
	_ = a.repo.updateConnectorTaskState(ctx, task.ID, "succeeded", "completed", "completed", 100, capabilityResult.InternalTaskID, capabilityResult.ActualCost, resultPayload, "", "")
	_ = a.repo.markConnectorDelivery(ctx, task.ID, "delivered", true, "")
	if intent.Name != connector.IntentTaskQuery {
		_ = a.repo.upsertSessionContext(ctx, connectorSessionContext{EnterpriseID: item.EnterpriseID, ConnectorID: item.ID, ExternalChatID: message.ExternalChatID, ExternalUserID: message.ExternalUserID, LastIntent: intent.Name, LastTaskType: intent.Name, LastTaskID: task.ID, LastAssetIDs: capabilityResult.AssetIDs, LastTopic: intent.Topic, LastParameters: command.Parameters, LastPrompt: prompt, ExpiresAt: time.Now().UTC().Add(24 * time.Hour)})
	}
	_ = a.repo.markMessage(ctx, message.ID, "completed", "")
	log.Printf("connector=feishu operation=capability connector_id=%s task_id=%s internal_task_id=%s capability=%s result=succeeded", item.ID, task.ID, capabilityResult.InternalTaskID, intent.Name)
	return nil
}

func connectorModelInfoText(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "当前飞书机器人尚未配置默认生图模型，请联系企业管理员完成配置。视频和 PPT 会分别使用企业后台配置的模型或模板。"
	}
	return fmt.Sprintf("当前飞书机器人用于图片生成的模型是：%s。视频和 PPT 会分别使用企业后台配置的模型或模板。", model)
}

func connectorCapabilityInfoText() string {
	return "目前我支持以下功能：\n" +
		"1. 生图：例如“生成 iPhone 17 的电商主图”。\n" +
		"2. 改图：例如“把刚才的图片加上京东 Logo”。\n" +
		"3. 生视频：例如“生成 10 秒、16:9、1080p 的产品宣传视频”。\n" +
		"4. 图生视频：例如“用刚才的图片生成 5 秒视频”。\n" +
		"5. 生 PPT：例如“生成一份 10 页的新能源汽车发布会 PPT”。\n" +
		"6. 查任务：发送“查询最近任务”或带任务编号查询。\n" +
		"7. 查模型：发送“使用的是什么模型”。\n" +
		"生成结果会保存到知启云 AI 作品中心；可用能力和额度以企业后台配置为准。"
}

func (a *connectorAPI) failMessage(ctx context.Context, message connectorMessageRecord, item enterpriseConnector, task connectorTaskRecord, code string, cause error) error {
	friendly := friendlyConnectorError(cause)
	log.Printf("connector=feishu operation=process connector_id=%s external_message_id=%s task_id=%s result=failed code=%s error=%v", item.ID, message.ExternalMessageID, task.ID, code, cause)
	failureCtx, cancel := connectorFailureContext(ctx)
	defer cancel()
	if task.ID != "" {
		if err := a.repo.updateConnectorTask(failureCtx, task.ID, "failed", 100, task.PlatformTaskID, task.PointsCost, task.Result, code, friendly); err != nil {
			log.Printf("connector=feishu operation=mark_task_failed task_id=%s result=failed error=%v", task.ID, err)
		}
	}
	if err := a.repo.markMessage(failureCtx, message.ID, "failed", friendly); err != nil {
		log.Printf("connector=feishu operation=mark_message_failed external_message_id=%s result=failed error=%v", message.ExternalMessageID, err)
	}
	if item.ID != "" && message.ExternalChatID != "" {
		if client, err := a.feishuClient(item); err == nil {
			if result, sendErr := client.SendText(failureCtx, connector.MessageTarget{ChatID: message.ExternalChatID}, connector.OutgoingMessage{Text: friendly}); sendErr == nil {
				if err := a.repo.insertOutboundMessage(failureCtx, item, message.ExternalChatID, message.ExternalUserID, result.ExternalMessageID, "text", map[string]any{"text": friendly, "errorCode": code}); err != nil {
					log.Printf("connector=feishu operation=record_failure_message external_message_id=%s result=failed error=%v", message.ExternalMessageID, err)
				}
			} else {
				log.Printf("connector=feishu operation=send_failure_message external_message_id=%s result=failed error=%v", message.ExternalMessageID, sendErr)
			}
		} else {
			log.Printf("connector=feishu operation=create_failure_client connector_id=%s result=failed error=%v", item.ID, err)
		}
	}
	return cause
}

func connectorFailureContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), connectorFailureHandlingTimeout)
}

func validateConnectorPermission(item enterpriseConnector, binding connectorUserBinding, message connectorMessageRecord) error {
	if !item.Config.AIImageEnabled {
		return errors.New("enterprise image generation is disabled")
	}
	if binding.Status != "active" || !boolValue(binding.Permission["imageGenerate"]) {
		return errForbidden
	}
	if strings.EqualFold(message.ChatType, "group") {
		if !item.Config.AllowGroupChat {
			return errors.New("group chat generation is disabled")
		}
		if item.Config.GroupRequireMention && !boolValue(message.Content["mentionedBot"]) {
			return errors.New("please mention the bot in group chat")
		}
	}
	return nil
}

func (a *connectorAPI) feishuClient(item enterpriseConnector) (*feishuconnector.Client, error) {
	if a.cipherErr != nil || a.cipher == nil {
		return nil, fmt.Errorf("CONNECTOR_SECRET_ENCRYPTION_KEY is required: %w", a.cipherErr)
	}
	secret, err := a.cipher.Decrypt(item.AppSecretEncrypted, item.ID+":app_secret")
	if err != nil {
		return nil, err
	}
	verificationToken, err := a.cipher.Decrypt(item.VerificationTokenEncrypted, item.ID+":verification_token")
	if err != nil {
		return nil, err
	}
	encryptKey, err := a.cipher.Decrypt(item.EncryptKeyEncrypted, item.ID+":encrypt_key")
	if err != nil {
		return nil, err
	}
	clientConfig := feishuconnector.Config{
		AppID: item.AppID, AppSecret: secret, VerificationToken: verificationToken, EncryptKey: encryptKey,
		BaseURL: a.cfg.FeishuAPIBaseURL, TokenCachePrefix: a.cfg.FeishuTokenCachePrefix,
		Timeout: a.cfg.FeishuHTTPTimeout(), Retries: 2,
	}
	if a.redis != nil {
		clientConfig.Redis = a.redis
	}
	return feishuconnector.New(clientConfig), nil
}

func (a *connectorAPI) encryptUpdatedSecrets(item *enterpriseConnector, request connectorSaveRequest) error {
	if request.AppSecret == "" && request.VerificationToken == "" && request.EncryptKey == "" {
		return nil
	}
	if a.cipherErr != nil || a.cipher == nil {
		return fmt.Errorf("CONNECTOR_SECRET_ENCRYPTION_KEY is required: %w", a.cipherErr)
	}
	var err error
	if strings.TrimSpace(request.AppSecret) != "" {
		item.AppSecretEncrypted, err = a.cipher.Encrypt(strings.TrimSpace(request.AppSecret), item.ID+":app_secret")
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(request.VerificationToken) != "" {
		item.VerificationTokenEncrypted, err = a.cipher.Encrypt(strings.TrimSpace(request.VerificationToken), item.ID+":verification_token")
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(request.EncryptKey) != "" {
		item.EncryptKeyEncrypted, err = a.cipher.Encrypt(strings.TrimSpace(request.EncryptKey), item.ID+":encrypt_key")
	}
	return err
}

func (a *connectorAPI) view(item enterpriseConnector) connectorView {
	view := connectorView{
		ID: item.ID, EnterpriseID: item.EnterpriseID, ConnectorType: item.ConnectorType,
		ConnectorName: item.ConnectorName, ConnectorKey: item.ConnectorKey, AppID: item.AppID,
		ExternalTenantKey: item.ExternalTenantKey, BotOpenID: item.BotOpenID, Status: item.Status,
		Config: item.Config, LastErrorMessage: item.LastErrorMessage,
		SecretsConfigured: connectorSecretState{AppSecret: item.AppSecretEncrypted != "", VerificationToken: item.VerificationTokenEncrypted != "", EncryptKey: item.EncryptKeyEncrypted != ""},
		CreatedAt:         connectorTime(item.CreatedAt), UpdatedAt: connectorTime(item.UpdatedAt),
	}
	if item.LastConnectedAt != nil {
		view.LastConnectedAt = connectorTime(*item.LastConnectedAt)
	}
	base := strings.TrimRight(strings.TrimSpace(a.cfg.ConnectorCallbackBaseURL), "/")
	view.CallbackURL = base + "/api/open/connectors/feishu/events/" + item.ConnectorKey
	if base == "" {
		view.CallbackURL = "/api/open/connectors/feishu/events/" + item.ConnectorKey
	}
	return view
}

func connectorLimit(r *http.Request, fallback int) int {
	value, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if value <= 0 || value > 500 {
		return fallback
	}
	return value
}

func friendlyConnectorError(err error) string {
	if err == nil {
		return "操作失败，请稍后重试。"
	}
	value := strings.ToLower(err.Error())
	switch {
	case strings.Contains(value, "insufficient") || strings.Contains(value, "余额") || strings.Contains(value, "算力"):
		return "企业算力余额不足，请联系企业管理员充值后重试。"
	case strings.Contains(value, "reference image") || strings.Contains(value, "reference asset"):
		return "没有找到当前会话中可用的上一张图片。请先发送或生成一张图片，再使用“用刚才的图片生成视频/修改图片”。"
	case strings.Contains(value, "approval"):
		return "该能力需要企业审批，当前自动审批流程尚未通过，请联系企业管理员。"
	case errors.Is(err, errForbidden) || strings.Contains(value, "permission") || strings.Contains(value, "disabled"):
		return "当前企业或成员没有使用该 AI 能力的权限，请联系企业管理员在“企业管理中心 → 飞书连接器”中启用。"
	case strings.Contains(value, "daily quota") || strings.Contains(value, "daily point"):
		return "你今天的该项 AI 额度已用完，请明天再试或联系企业管理员调整额度。"
	case strings.Contains(value, "monthly"):
		return "你本月的该项 AI 积分额度已用完，请联系企业管理员调整额度。"
	case strings.Contains(value, "duration"):
		return "请求的视频时长超过企业允许的上限，请缩短时长后重试。"
	case strings.Contains(value, "page"):
		return "请求的 PPT 页数超过企业允许的上限，请减少页数后重试。"
	case strings.Contains(value, "resolution"):
		return "请求的视频分辨率超过企业允许的上限，请降低分辨率后重试。"
	case strings.Contains(value, "mention"):
		return "群聊中请先 @机器人，再发送生图指令。"
	default:
		return "任务处理失败，请稍后重试；如持续失败请联系企业管理员。"
	}
}

func writeConnectorError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, errEnterpriseNotFound), errors.Is(err, sql.ErrNoRows):
		status = http.StatusNotFound
	case errors.Is(err, errEnterpriseConflict):
		status = http.StatusConflict
	case errors.Is(err, errEnterpriseInvalid), errors.Is(err, storagecenter.ErrSecretCipherRequired):
		status = http.StatusBadRequest
	}
	message := friendlyConnectorError(err)
	if status == http.StatusBadRequest || status == http.StatusConflict || status == http.StatusNotFound {
		message = truncateConnectorError(err.Error())
	}
	writeError(w, status, errors.New(message))
}

func sanitizeConnectorPayload(raw []byte) map[string]any {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return map[string]any{"invalid": true}
	}
	return sanitizeConnectorMap(value)
}

func sanitizeConnectorMap(input map[string]any) map[string]any {
	result := make(map[string]any, len(input))
	for key, value := range input {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "encrypt") || strings.Contains(lower, "authorization") {
			result[key] = "[REDACTED]"
			continue
		}
		switch typed := value.(type) {
		case map[string]any:
			result[key] = sanitizeConnectorMap(typed)
		case []any:
			items := make([]any, len(typed))
			for index, item := range typed {
				if object, ok := item.(map[string]any); ok {
					items[index] = sanitizeConnectorMap(object)
				} else {
					items[index] = item
				}
			}
			result[key] = items
		default:
			result[key] = value
		}
	}
	return result
}

func redactConnectorValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 8 {
		return "***"
	}
	return value[:3] + "***" + value[len(value)-3:]
}

type connectorRateLimiter struct {
	mu      sync.Mutex
	windows map[string]connectorRateWindow
}

type connectorRateWindow struct {
	Started time.Time
	Count   int
}

func newConnectorRateLimiter() *connectorRateLimiter {
	return &connectorRateLimiter{windows: map[string]connectorRateWindow{}}
}

func (l *connectorRateLimiter) Allow(key string, limit int, duration time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	window := l.windows[key]
	if window.Started.IsZero() || now.Sub(window.Started) >= duration {
		l.windows[key] = connectorRateWindow{Started: now, Count: 1}
		return true
	}
	if window.Count >= limit {
		return false
	}
	window.Count++
	l.windows[key] = window
	return true
}
