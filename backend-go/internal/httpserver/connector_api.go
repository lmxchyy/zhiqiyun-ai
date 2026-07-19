package httpserver

import (
	"bytes"
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

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/connector"
	feishuconnector "xianzhi-ai/backend-go/internal/connector/feishu"
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
	items, err := a.repo.listTasks(r.Context(), access.TenantID, item.ID, connectorLimit(r, 100))
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": len(items)})
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
	ctx, cancel := context.WithTimeout(parent, imageGenerationTimeout+2*time.Minute)
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
	editRequested := isConnectorImageEditRequest(text)
	intent := (connector.RuleIntentRouter{}).Route(text, connector.IntentDefaults{Size: item.Config.DefaultSize, Count: item.Config.DefaultImageCount})
	prompt := ""
	if editRequested {
		intent.Name = connector.IntentImageGenerate
		prompt = connectorImageEditPrompt(text)
	} else if intent.Name == connector.IntentImageGenerate {
		prompt = (connector.EcommerceImagePromptBuilder{}).Build(intent)
	}
	taskType := connector.IntentImageGenerate
	taskIntent := intent.Name
	if editRequested {
		taskType = connector.IntentImageEdit
		taskIntent = connector.IntentImageEdit
	} else if intent.Name == connector.IntentModelInfo {
		taskType = connector.IntentModelInfo
	}
	task, created, err := a.repo.createConnectorTask(ctx, connectorTaskRecord{
		EnterpriseID: item.EnterpriseID, ConnectorID: item.ID, BindingID: binding.ID,
		ExternalChatID: message.ExternalChatID, ExternalMessageID: message.ExternalMessageID,
		TaskType: taskType, Intent: taskIntent, OriginalText: text,
		OptimizedPrompt: prompt, ModelID: item.Config.DefaultImageModel, Status: "pending", Progress: 0,
	})
	if err != nil {
		return a.failMessage(ctx, message, item, task, "TASK_CREATE_FAILED", err)
	}
	if !created && (task.Status == "succeeded" || task.Status == "ignored") {
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
	if intent.Name != connector.IntentImageGenerate {
		result, sendErr := client.SendText(ctx, target, connector.OutgoingMessage{Text: "目前我支持单轮 AI 生图。你可以发送：生成 iPhone 17 的电商图。"})
		if sendErr == nil {
			_ = a.repo.insertOutboundMessage(ctx, item, message.ExternalChatID, message.ExternalUserID, result.ExternalMessageID, "text", map[string]any{"text": "help"})
		}
		_ = a.repo.updateConnectorTask(ctx, task.ID, "ignored", 100, "", 0, map[string]any{"reason": "unsupported_intent"}, "", "")
		_ = a.repo.markMessage(ctx, message.ID, "ignored", "")
		return sendErr
	}
	if err := validateConnectorPermission(item, binding, message); err != nil {
		return a.failMessage(ctx, message, item, task, "PERMISSION_DENIED", err)
	}
	usage, err := a.repo.dailyBindingUsage(ctx, item.EnterpriseID, binding.ID)
	if err != nil {
		return a.failMessage(ctx, message, item, task, "USAGE_CHECK_FAILED", err)
	}
	dailyQuota := item.Config.MemberDailyQuota
	if memberQuota := intValue(binding.Permission["dailyQuota"]); memberQuota > 0 {
		dailyQuota = memberQuota
	}
	if usage > dailyQuota {
		return a.failMessage(ctx, message, item, task, "DAILY_QUOTA_EXCEEDED", errors.New("member daily quota exceeded"))
	}
	var reference connectorReferenceImage
	if editRequested {
		var found bool
		reference, found, err = a.repo.latestSuccessfulReferenceImage(ctx, item.EnterpriseID, item.ID, binding.ID, message.ExternalChatID)
		if err != nil {
			return a.failMessage(ctx, message, item, task, "REFERENCE_LOOKUP_FAILED", err)
		}
		if !found {
			missingText := "我没有在当前飞书会话中找到可编辑的上一张图片。请先让我生成一张图片，再发送“在刚才图片上……”的修改要求。"
			result, sendErr := client.SendText(ctx, target, connector.OutgoingMessage{Text: missingText})
			if sendErr == nil {
				_ = a.repo.insertOutboundMessage(ctx, item, message.ExternalChatID, message.ExternalUserID, result.ExternalMessageID, "text", map[string]any{"text": missingText})
			}
			_ = a.repo.updateConnectorTask(ctx, task.ID, "ignored", 100, "", 0, map[string]any{"reason": "reference_image_not_found"}, "REFERENCE_IMAGE_NOT_FOUND", missingText)
			_ = a.repo.markMessage(ctx, message.ID, "ignored", "")
			return sendErr
		}
	}
	_ = a.repo.updateConnectorTask(ctx, task.ID, "processing", 10, "", 0, map[string]any{}, "", "")
	progressText := "任务已创建，正在生成，请稍候…"
	if editRequested {
		progressText = "已找到上一张图片，正在按你的要求修改，请稍候…"
	}
	progressResult, err := client.SendText(ctx, target, connector.OutgoingMessage{Text: progressText})
	if err != nil {
		return a.failMessage(ctx, message, item, task, "FEISHU_SEND_FAILED", err)
	}
	_ = a.repo.insertOutboundMessage(ctx, item, message.ExternalChatID, message.ExternalUserID, progressResult.ExternalMessageID, "text", map[string]any{"text": progressText})
	requestType := "TEXT_TO_IMAGE"
	params := map[string]any{"size": intent.Size, "count": intent.Count, "n": intent.Count, "source_type": "feishu", "source_id": task.ID, "operator_external_id": message.ExternalUserID}
	if editRequested {
		requestType = "IMAGE_TO_IMAGE"
		params["referenceImages"] = []any{map[string]any{"assetId": reference.AssetID, "name": reference.Name, "url": reference.URL}}
		params["sourceReferenceAssetId"] = reference.AssetID
		params["sourceReferenceTaskId"] = reference.GenerationTaskID
	}
	req := generation.CreateRequest{
		Type: requestType, ClientRequestID: "feishu:" + message.ExternalMessageID, Prompt: prompt, Model: item.Config.DefaultImageModel,
		Params: params,
	}
	generatedTask, prepared, err := a.generator.executeConnectorImageGeneration(ctx, binding.InternalUserID, req)
	if err != nil {
		return a.failMessage(ctx, message, item, task, "GENERATION_FAILED", err)
	}
	resultPayload := map[string]any{"generationTaskId": generatedTask.ID, "assetIds": generatedTask.ResultIDs, "conceptImage": !editRequested, "editMode": editRequested}
	if editRequested {
		resultPayload["sourceReferenceAssetId"] = reference.AssetID
		resultPayload["sourceReferenceTaskId"] = reference.GenerationTaskID
	}
	if err := a.repo.updateConnectorTask(ctx, task.ID, "processing", 90, generatedTask.ID, int64(generatedTask.PointCost), resultPayload, "", ""); err != nil {
		return a.failMessage(ctx, message, item, task, "TASK_UPDATE_FAILED", err)
	}
	imageSent := 0
	for index, image := range prepared.GeneratedImages {
		raw, _, extension, readErr := readGeneratedArtifact(ctx, image.URL, image.ContentType)
		if readErr != nil {
			log.Printf("connector=feishu operation=read_generated_image task_id=%s index=%d result=failed error=%v", task.ID, index, readErr)
			continue
		}
		sendResult, sendErr := client.SendImage(ctx, target, connector.OutgoingMessage{Image: bytes.NewReader(raw), FileName: fmt.Sprintf("%s-%d.%s", generatedTask.ID, index+1, extension)})
		if sendErr != nil {
			log.Printf("connector=feishu operation=send_image task_id=%s index=%d result=failed error=%v", task.ID, index, sendErr)
			continue
		}
		imageSent++
		_ = a.repo.insertOutboundMessage(ctx, item, message.ExternalChatID, message.ExternalUserID, sendResult.ExternalMessageID, "image", map[string]any{"generationTaskId": generatedTask.ID, "index": index + 1})
	}
	finalText := fmt.Sprintf("生成完成，已保存到作品中心。本次消耗 %d 点。", generatedTask.PointCost)
	if imageSent == 0 {
		finalText = fmt.Sprintf("生成完成，作品已保存到作品中心（当前通道未能直接发送图片）。本次消耗 %d 点。", generatedTask.PointCost)
	}
	finalResult, sendErr := client.SendText(ctx, target, connector.OutgoingMessage{Text: finalText})
	if sendErr != nil {
		return a.failMessage(ctx, message, item, task, "FEISHU_SEND_FAILED", sendErr)
	}
	_ = a.repo.insertOutboundMessage(ctx, item, message.ExternalChatID, message.ExternalUserID, finalResult.ExternalMessageID, "text", map[string]any{"text": finalText})
	resultPayload["imagesSent"] = imageSent
	_ = a.repo.updateConnectorTask(ctx, task.ID, "succeeded", 100, generatedTask.ID, int64(generatedTask.PointCost), resultPayload, "", "")
	_ = a.repo.markMessage(ctx, message.ID, "completed", "")
	log.Printf("connector=feishu operation=generate connector_id=%s task_id=%s generation_task_id=%s result=succeeded", item.ID, task.ID, generatedTask.ID)
	return nil
}

func connectorModelInfoText(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "当前飞书机器人尚未配置默认生图模型，请联系企业管理员完成配置。"
	}
	return fmt.Sprintf("当前飞书机器人用于图片生成的模型是：%s。", model)
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
	case errors.Is(err, errForbidden) || strings.Contains(value, "permission") || strings.Contains(value, "disabled"):
		return "当前企业或成员没有 AI 生图权限，请联系企业管理员。"
	case strings.Contains(value, "daily quota"):
		return "你今天的 AI 生图额度已用完，请明天再试或联系企业管理员调整额度。"
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
