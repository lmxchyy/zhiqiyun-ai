package httpserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	"xianzhi-ai/backend-go/internal/connector"
	feishuconnector "xianzhi-ai/backend-go/internal/connector/feishu"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type connectorCapabilityRuntime struct {
	api     *connectorAPI
	item    enterpriseConnector
	binding connectorUserBinding
	message connectorMessageRecord
	task    connectorTaskRecord
	client  *feishuconnector.Client
}

type connectorHandlerBase struct{ runtime *connectorCapabilityRuntime }
type imageGenerateHandler struct {
	connectorHandlerBase
	prepared generation.CreateRequest
}
type imageEditHandler struct {
	connectorHandlerBase
	prepared generation.CreateRequest
}
type videoGenerateHandler struct {
	connectorHandlerBase
	prepared generation.CreateRequest
}
type imageToVideoHandler struct {
	connectorHandlerBase
	prepared generation.CreateRequest
}
type pptGenerateHandler struct {
	connectorHandlerBase
	prepared pptapp.GenerateRequest
}
type taskQueryHandler struct{ connectorHandlerBase }

func (h connectorHandlerBase) validate(c connector.AICommand, edit bool) error {
	if err := validateConnectorChatPermission(h.runtime.item, h.runtime.binding, h.runtime.message); err != nil {
		return err
	}
	if !h.runtime.item.Config.AIImageEnabled {
		return errors.New("enterprise image generation is disabled")
	}
	if edit && !h.runtime.item.Config.AIImageEditEnabled {
		return errors.New("enterprise image editing is disabled")
	}
	if h.runtime.binding.Status != "active" || !boolValue(h.runtime.binding.Permission["imageGenerate"]) {
		return errForbidden
	}
	if edit && len(c.ReferenceAssets) == 0 {
		return errors.New("reference image not found")
	}
	return nil
}

// Pointer-based factory preserves handler estimates for Execute.
func connectorHandlers(runtime *connectorCapabilityRuntime) []connector.CapabilityHandler {
	return []connector.CapabilityHandler{
		&imageGenerateHandler{connectorHandlerBase: connectorHandlerBase{runtime}},
		&imageEditHandler{connectorHandlerBase: connectorHandlerBase{runtime}},
		&videoGenerateHandler{connectorHandlerBase: connectorHandlerBase{runtime}},
		&imageToVideoHandler{connectorHandlerBase: connectorHandlerBase{runtime}},
		&pptGenerateHandler{connectorHandlerBase: connectorHandlerBase{runtime}},
		&taskQueryHandler{connectorHandlerBase: connectorHandlerBase{runtime}},
	}
}

func (h *imageGenerateHandler) CanHandle(c connector.AICommand) bool {
	return c.Intent == connector.IntentImageGenerate
}
func (h *imageEditHandler) CanHandle(c connector.AICommand) bool {
	return c.Intent == connector.IntentImageEdit
}
func (h *videoGenerateHandler) CanHandle(c connector.AICommand) bool {
	return c.Intent == connector.IntentVideoGenerate
}
func (h *imageToVideoHandler) CanHandle(c connector.AICommand) bool {
	return c.Intent == connector.IntentVideoImageToVideo
}
func (h *pptGenerateHandler) CanHandle(c connector.AICommand) bool {
	return c.Intent == connector.IntentPPTGenerate
}
func (h *taskQueryHandler) CanHandle(c connector.AICommand) bool {
	return c.Intent == connector.IntentTaskQuery
}

func (h *imageGenerateHandler) Validate(_ context.Context, c connector.AICommand) error {
	return h.connectorHandlerBase.validate(c, false)
}
func (h *imageEditHandler) Validate(_ context.Context, c connector.AICommand) error {
	return h.connectorHandlerBase.validate(c, true)
}
func (h *imageGenerateHandler) EstimateCost(ctx context.Context, c connector.AICommand) (int64, error) {
	prepared, cost, err := h.runtime.api.generator.estimateConnectorGeneration(ctx, c.InternalUserID, c.EnterpriseID, connectorImageRequest(h.runtime, c, false))
	h.prepared = prepared
	return cost, err
}
func (h *imageEditHandler) EstimateCost(ctx context.Context, c connector.AICommand) (int64, error) {
	prepared, cost, err := h.runtime.api.generator.estimateConnectorGeneration(ctx, c.InternalUserID, c.EnterpriseID, connectorImageRequest(h.runtime, c, true))
	h.prepared = prepared
	return cost, err
}
func (h *imageGenerateHandler) Execute(ctx context.Context, c connector.AICommand) (connector.CapabilityResult, error) {
	return h.execute(ctx, c, false)
}
func (h *imageEditHandler) Execute(ctx context.Context, c connector.AICommand) (connector.CapabilityResult, error) {
	return h.execute(ctx, c, true)
}
func (h *imageGenerateHandler) execute(ctx context.Context, c connector.AICommand, edit bool) (connector.CapabilityResult, error) {
	return executeConnectorImageHandler(ctx, h.runtime, c, edit, h.prepared)
}
func (h *imageEditHandler) execute(ctx context.Context, c connector.AICommand, edit bool) (connector.CapabilityResult, error) {
	return executeConnectorImageHandler(ctx, h.runtime, c, edit, h.prepared)
}

func executeConnectorImageHandler(ctx context.Context, runtime *connectorCapabilityRuntime, c connector.AICommand, edit bool, prepared generation.CreateRequest) (connector.CapabilityResult, error) {
	if prepared.Type == "" {
		prepared = connectorImageRequest(runtime, c, edit)
	}
	task, output, err := runtime.api.generator.executeConnectorImageGeneration(ctx, c.InternalUserID, c.EnterpriseID, prepared)
	if err != nil {
		return connector.CapabilityResult{}, err
	}
	return connector.CapabilityResult{InternalTaskID: task.ID, Status: "completed", Progress: 100, ActualCost: int64(task.PointCost), AssetIDs: task.ResultIDs,
		Data: map[string]any{"generationRequest": output, "editMode": edit}}, nil
}

func connectorImageRequest(runtime *connectorCapabilityRuntime, c connector.AICommand, edit bool) generation.CreateRequest {
	prompt := stringValue(c.Parameters["prompt"])
	if edit {
		prompt = connectorImageEditPrompt(c.OriginalText)
	}
	requestType := "TEXT_TO_IMAGE"
	params := map[string]any{"size": stringValue(c.Parameters["size"]), "count": intValue(c.Parameters["count"]), "n": intValue(c.Parameters["count"]),
		"source_type": "feishu", "source_id": runtime.task.ID, "operator_external_id": c.ExternalUserID, "connector_id": c.ConnectorID,
		"external_message_id": c.ExternalMessageID, "capability": c.Intent}
	if edit && len(c.ReferenceAssets) > 0 {
		requestType = "IMAGE_TO_IMAGE"
		ref := c.ReferenceAssets[0]
		params["referenceImages"] = []any{map[string]any{"assetId": ref.ID, "name": ref.Name, "url": ref.URL}}
		params["sourceReferenceAssetId"], params["sourceReferenceTaskId"] = ref.ID, ref.TaskID
	}
	return generation.CreateRequest{Type: requestType, ClientRequestID: "feishu:" + c.ExternalMessageID, Prompt: prompt, Model: runtime.item.Config.DefaultImageModel, Params: params}
}

func (h *videoGenerateHandler) Validate(_ context.Context, c connector.AICommand) error {
	return h.validateVideo(c, false)
}
func (h *imageToVideoHandler) Validate(_ context.Context, c connector.AICommand) error {
	return h.validateVideo(c, true)
}
func (h connectorHandlerBase) validateVideo(c connector.AICommand, imageToVideo bool) error {
	if err := validateConnectorChatPermission(h.runtime.item, h.runtime.binding, h.runtime.message); err != nil {
		return err
	}
	cfg := h.runtime.item.Config
	if !cfg.AIVideoEnabled || cfg.VideoPermissionMode == "deny" {
		return errors.New("enterprise video generation is disabled")
	}
	if cfg.VideoPermissionMode == "approval" {
		return errors.New("video generation requires administrator approval")
	}
	if h.runtime.binding.Status != "active" || !boolValue(h.runtime.binding.Permission["videoGenerate"]) {
		return errForbidden
	}
	if imageToVideo && !cfg.AllowImageToVideo {
		return errors.New("image-to-video is disabled")
	}
	if imageToVideo && len(c.ReferenceAssets) == 0 {
		return errors.New("reference image not found")
	}
	duration := intValue(c.Parameters["duration"])
	maxDuration := cfg.VideoMaxDuration
	if member := intValue(h.runtime.binding.Permission["maxVideoDuration"]); member > 0 && member < maxDuration {
		maxDuration = member
	}
	if duration <= 0 || duration > maxDuration {
		return fmt.Errorf("video duration exceeds limit %d seconds", maxDuration)
	}
	if resolutionRank(stringValue(c.Parameters["resolution"])) > resolutionRank(cfg.VideoMaxResolution) {
		return errors.New("video resolution exceeds enterprise limit")
	}
	return nil
}

func (h *videoGenerateHandler) EstimateCost(ctx context.Context, c connector.AICommand) (int64, error) {
	prepared, cost, err := h.runtime.api.generator.estimateConnectorGeneration(ctx, c.InternalUserID, c.EnterpriseID, connectorVideoRequest(h.runtime, c, false))
	h.prepared = prepared
	return cost, err
}
func (h *imageToVideoHandler) EstimateCost(ctx context.Context, c connector.AICommand) (int64, error) {
	prepared, cost, err := h.runtime.api.generator.estimateConnectorGeneration(ctx, c.InternalUserID, c.EnterpriseID, connectorVideoRequest(h.runtime, c, true))
	h.prepared = prepared
	return cost, err
}
func (h *videoGenerateHandler) Execute(ctx context.Context, c connector.AICommand) (connector.CapabilityResult, error) {
	return executeConnectorVideoHandler(ctx, h.runtime, c, false, h.prepared)
}
func (h *imageToVideoHandler) Execute(ctx context.Context, c connector.AICommand) (connector.CapabilityResult, error) {
	return executeConnectorVideoHandler(ctx, h.runtime, c, true, h.prepared)
}

func connectorVideoRequest(runtime *connectorCapabilityRuntime, c connector.AICommand, imageToVideo bool) generation.CreateRequest {
	typeName := "TEXT_TO_VIDEO"
	params := map[string]any{"prompt": stringValue(c.Parameters["prompt"]), "duration": intValue(c.Parameters["duration"]), "resolution": stringValue(c.Parameters["resolution"]), "aspect_ratio": stringValue(c.Parameters["aspect_ratio"]), "count": 1, "n": 1, "style": stringValue(c.Parameters["style"]), "source_type": "feishu", "source_id": runtime.task.ID, "operator_external_id": c.ExternalUserID, "connector_id": c.ConnectorID, "external_message_id": c.ExternalMessageID, "capability": c.Intent}
	if imageToVideo && len(c.ReferenceAssets) > 0 {
		typeName = "IMAGE_TO_VIDEO"
		ref := c.ReferenceAssets[0]
		params["referenceImages"] = []any{map[string]any{"assetId": ref.ID, "name": ref.Name, "url": ref.URL}}
		params["reference_image"] = ref.URL
		params["sourceReferenceAssetId"] = ref.ID
	}
	return generation.CreateRequest{Type: typeName, ClientRequestID: "feishu:" + c.ExternalMessageID, Prompt: stringValue(c.Parameters["prompt"]), Model: runtime.item.Config.DefaultVideoModel, Params: params}
}

func executeConnectorVideoHandler(ctx context.Context, runtime *connectorCapabilityRuntime, c connector.AICommand, imageToVideo bool, prepared generation.CreateRequest) (connector.CapabilityResult, error) {
	if prepared.Type == "" {
		prepared = connectorVideoRequest(runtime, c, imageToVideo)
	}
	task, output, file, err := runtime.api.generator.executeConnectorVideoGeneration(ctx, c.InternalUserID, c.EnterpriseID, prepared)
	if err != nil {
		return connector.CapabilityResult{}, err
	}
	data := map[string]any{"generationRequest": output, "file": file, "duration": c.Parameters["duration"], "aspectRatio": c.Parameters["aspect_ratio"], "resolution": c.Parameters["resolution"], "model": prepared.Model, "topic": c.Parameters["topic"]}
	return connector.CapabilityResult{InternalTaskID: task.ID, Status: "completed", Progress: 100, ActualCost: int64(task.PointCost), AssetIDs: task.ResultIDs, Data: data}, nil
}

func (h *pptGenerateHandler) Validate(_ context.Context, c connector.AICommand) error {
	if err := validateConnectorChatPermission(h.runtime.item, h.runtime.binding, h.runtime.message); err != nil {
		return err
	}
	cfg := h.runtime.item.Config
	if !cfg.PPTEnabled || cfg.PPTPermissionMode == "deny" {
		return errors.New("enterprise ppt generation is disabled")
	}
	if cfg.PPTPermissionMode == "approval" {
		return errors.New("ppt generation requires administrator approval")
	}
	if h.runtime.binding.Status != "active" || !boolValue(h.runtime.binding.Permission["pptGenerate"]) {
		return errForbidden
	}
	pages := intValue(c.Parameters["page_count"])
	maxPages := cfg.PPTMaxPageCount
	if member := intValue(h.runtime.binding.Permission["maxPptPages"]); member > 0 && member < maxPages {
		maxPages = member
	}
	if pages <= 0 || pages > maxPages {
		return fmt.Errorf("ppt page count exceeds limit %d", maxPages)
	}
	return nil
}
func (h *pptGenerateHandler) EstimateCost(ctx context.Context, c connector.AICommand) (int64, error) {
	req := connectorPPTRequest(h.runtime, c)
	prepared, cost, err := h.runtime.api.generator.estimateConnectorPPT(ctx, c.InternalUserID, c.EnterpriseID, req)
	h.prepared = prepared
	return cost, err
}
func (h *pptGenerateHandler) Execute(ctx context.Context, c connector.AICommand) (connector.CapabilityResult, error) {
	req := h.prepared
	if req.Prompt == "" {
		req = connectorPPTRequest(h.runtime, c)
	}
	execution, err := h.runtime.api.generator.executeConnectorPPT(ctx, c.InternalUserID, c.EnterpriseID, "feishu:"+c.ExternalMessageID, req, map[string]any{
		"connector_id": c.ConnectorID, "external_user_id": c.ExternalUserID, "external_message_id": c.ExternalMessageID,
		"capability": c.Intent, "connector_task_id": h.runtime.task.ID,
	})
	if err != nil {
		return connector.CapabilityResult{}, err
	}
	asset, err := h.runtime.api.repo.ensureConnectorPPTAsset(ctx, c.InternalUserID, execution.TenantID, execution.OrganizationID, execution.Task.TaskID, execution.Task.Title, execution.File)
	if err != nil {
		_, _ = h.runtime.api.generator.store.FailGenerationTask(execution.BillingTask.ID, generationErrorMessage(err))
		h.runtime.api.generator.cleanupGeneratedFiles([]storagecenter.FileObject{execution.File})
		return connector.CapabilityResult{}, err
	}
	completedBilling, err := h.runtime.api.generator.store.CompleteGenerationTask(execution.BillingTask.ID, execution.BillingRequest)
	if err != nil {
		_, _ = h.runtime.api.generator.store.FailGenerationTask(execution.BillingTask.ID, generationErrorMessage(err))
		h.runtime.api.generator.cleanupGeneratedFiles([]storagecenter.FileObject{execution.File})
		_ = h.runtime.api.repo.deleteConnectorAsset(ctx, asset.ID, c.InternalUserID)
		return connector.CapabilityResult{}, fmt.Errorf("commit ppt billing: %w", err)
	}
	return connector.CapabilityResult{InternalTaskID: execution.Task.TaskID, Status: "completed", Progress: 100, ActualCost: int64(completedBilling.PointCost), AssetIDs: []string{asset.ID}, Data: map[string]any{"pptTask": execution.Task, "pptBytes": execution.Payload, "file": execution.File, "fileName": asset.Name, "fileSize": len(execution.Payload), "pageCount": execution.Task.SlideCount, "template": c.Parameters["template_id"], "topic": c.Parameters["topic"], "model": execution.Task.TextModel}}, nil
}
func connectorPPTRequest(runtime *connectorCapabilityRuntime, c connector.AICommand) pptapp.GenerateRequest {
	return pptapp.GenerateRequest{Prompt: stringValue(c.Parameters["prompt"]), SlideCount: intValue(c.Parameters["page_count"]), Audience: stringValue(c.Parameters["audience"]), Scenario: stringValue(c.Parameters["purpose"]), Theme: stringValue(c.Parameters["theme"]), Language: stringValue(c.Parameters["language"]), ImageSource: "ai", ImageModel: runtime.item.Config.DefaultImageModel, AutoThemeEnabled: true}
}

func (h *taskQueryHandler) Validate(_ context.Context, c connector.AICommand) error {
	return validateConnectorChatPermission(h.runtime.item, h.runtime.binding, h.runtime.message)
}
func (h *taskQueryHandler) EstimateCost(context.Context, connector.AICommand) (int64, error) {
	return 0, nil
}
func (h *taskQueryHandler) Execute(ctx context.Context, c connector.AICommand) (connector.CapabilityResult, error) {
	taskID := stringValue(c.Context["last_task_id"])
	if taskID == "" {
		return connector.CapabilityResult{Status: "completed", Data: map[string]any{"message": "当前会话还没有可查询的任务。"}}, nil
	}
	task, err := h.runtime.api.repo.taskByIDForBinding(ctx, c.EnterpriseID, c.ConnectorID, h.runtime.binding.ID, taskID)
	if err != nil {
		return connector.CapabilityResult{}, err
	}
	return connector.CapabilityResult{InternalTaskID: task.PlatformTaskID, Status: task.UnifiedStatus, Progress: task.Progress, ActualCost: task.PointsCost, Data: map[string]any{"task": task}}, nil
}

func (h *imageGenerateHandler) QueryStatus(context.Context, connector.AICommand) (connector.CapabilityResult, error) {
	return connector.CapabilityResult{}, nil
}
func (h *imageEditHandler) QueryStatus(context.Context, connector.AICommand) (connector.CapabilityResult, error) {
	return connector.CapabilityResult{}, nil
}
func (h *videoGenerateHandler) QueryStatus(context.Context, connector.AICommand) (connector.CapabilityResult, error) {
	return connector.CapabilityResult{}, nil
}
func (h *imageToVideoHandler) QueryStatus(context.Context, connector.AICommand) (connector.CapabilityResult, error) {
	return connector.CapabilityResult{}, nil
}
func (h *pptGenerateHandler) QueryStatus(context.Context, connector.AICommand) (connector.CapabilityResult, error) {
	return connector.CapabilityResult{}, nil
}
func (h *taskQueryHandler) QueryStatus(ctx context.Context, c connector.AICommand) (connector.CapabilityResult, error) {
	return h.Execute(ctx, c)
}

func (h *imageGenerateHandler) BuildResult(_ context.Context, c connector.AICommand, r connector.CapabilityResult) (connector.OutgoingMessage, error) {
	return connectorResultMessage("图片生成完成", c, r), nil
}
func (h *imageEditHandler) BuildResult(_ context.Context, c connector.AICommand, r connector.CapabilityResult) (connector.OutgoingMessage, error) {
	return connectorResultMessage("图片修改完成", c, r), nil
}
func (h *videoGenerateHandler) BuildResult(_ context.Context, c connector.AICommand, r connector.CapabilityResult) (connector.OutgoingMessage, error) {
	return connectorResultMessage("视频生成完成", c, r), nil
}
func (h *imageToVideoHandler) BuildResult(_ context.Context, c connector.AICommand, r connector.CapabilityResult) (connector.OutgoingMessage, error) {
	return connectorResultMessage("图生视频完成", c, r), nil
}
func (h *pptGenerateHandler) BuildResult(_ context.Context, c connector.AICommand, r connector.CapabilityResult) (connector.OutgoingMessage, error) {
	return connectorResultMessage("PPT 生成完成", c, r), nil
}
func (h *taskQueryHandler) BuildResult(_ context.Context, c connector.AICommand, r connector.CapabilityResult) (connector.OutgoingMessage, error) {
	if m := stringValue(r.Data["message"]); m != "" {
		return connector.OutgoingMessage{Text: m}, nil
	}
	return connectorResultMessage("最近任务状态", c, r), nil
}

func connectorResultMessage(title string, c connector.AICommand, r connector.CapabilityResult) connector.OutgoingMessage {
	lines := []string{fmt.Sprintf("**状态：** %s", connectorStatusLabel(r.Status)), fmt.Sprintf("**进度：** %d%%", r.Progress)}
	if r.InternalTaskID != "" {
		lines = append(lines, "**任务编号：** "+r.InternalTaskID)
	}
	if r.ActualCost > 0 {
		lines = append(lines, fmt.Sprintf("**消耗积分：** %d", r.ActualCost))
	}
	if topic := stringValue(r.Data["topic"]); topic != "" {
		lines = append(lines, "**主题：** "+topic)
	}
	if duration := intValue(r.Data["duration"]); duration > 0 {
		lines = append(lines, fmt.Sprintf("**视频参数：** %d 秒 · %s · %s", duration, stringValue(r.Data["aspectRatio"]), stringValue(r.Data["resolution"])))
	}
	if pages := intValue(r.Data["pageCount"]); pages > 0 {
		lines = append(lines, fmt.Sprintf("**PPT：** %d 页 · %s", pages, stringValue(r.Data["template"])))
	}
	if model := stringValue(r.Data["model"]); model != "" {
		lines = append(lines, "**模型：** "+model)
	}
	if name := stringValue(r.Data["fileName"]); name != "" {
		lines = append(lines, "**文件：** "+name)
	}
	if size := intValue(r.Data["fileSize"]); size > 0 {
		lines = append(lines, fmt.Sprintf("**文件大小：** %.2f MB", float64(size)/(1024*1024)))
	}
	if url := stringValue(r.Data["downloadURL"]); url != "" {
		lines = append(lines, "[查看/下载作品]("+url+")（链接短期有效）")
	}
	connectorTaskID := firstNonEmptyString(stringValue(r.Data["connectorTaskId"]), r.InternalTaskID)
	buttons := []any{
		map[string]any{"tag": "button", "text": map[string]any{"tag": "plain_text", "content": "查看任务"}, "type": "default", "value": map[string]any{"action": "task.view", "task_id": connectorTaskID}},
		map[string]any{"tag": "button", "text": map[string]any{"tag": "plain_text", "content": "重新投递"}, "type": "default", "value": map[string]any{"action": "task.retry_delivery", "task_id": connectorTaskID}},
	}
	if c.Intent == connector.IntentVideoGenerate || c.Intent == connector.IntentVideoImageToVideo {
		buttons = append(buttons, map[string]any{"tag": "button", "text": map[string]any{"tag": "plain_text", "content": "生成同款"}, "type": "default", "value": map[string]any{"action": "video.generate_similar", "task_id": connectorTaskID}})
	}
	if c.Intent == connector.IntentPPTGenerate {
		buttons = append(buttons, map[string]any{"tag": "button", "text": map[string]any{"tag": "plain_text", "content": "重新生成"}, "type": "default", "value": map[string]any{"action": "ppt.regenerate", "task_id": connectorTaskID}})
	}
	if stringValue(r.Data["downloadURL"]) != "" {
		buttons = append(buttons, map[string]any{"tag": "button", "text": map[string]any{"tag": "plain_text", "content": "下载作品"}, "type": "primary", "value": map[string]any{"action": "asset.download", "task_id": connectorTaskID}})
	}
	return connector.OutgoingMessage{Card: map[string]any{
		"config": map[string]any{"wide_screen_mode": true},
		"header": map[string]any{"title": map[string]any{"tag": "plain_text", "content": title}, "template": "blue"},
		"elements": []any{
			map[string]any{"tag": "div", "text": map[string]any{"tag": "lark_md", "content": strings.Join(lines, "\n")}},
			map[string]any{"tag": "action", "actions": buttons},
		},
	}}
}

func validateConnectorChatPermission(item enterpriseConnector, binding connectorUserBinding, message connectorMessageRecord) error {
	if binding.Status != "active" {
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
func resolutionRank(v string) int {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "480p":
		return 1
	case "720p":
		return 2
	case "1080p":
		return 3
	case "2160p", "4k":
		return 4
	default:
		return 0
	}
}
func connectorStatusLabel(v string) string {
	switch v {
	case "created", "queued":
		return "排队中"
	case "validating", "reserved", "processing", "rendering", "uploading":
		return "生成中"
	case "completed", "succeeded":
		return "已完成"
	case "delivery_failed":
		return "生成完成，投递失败"
	case "failed":
		return "失败"
	default:
		return v
	}
}

func (a *connectorAPI) deliverCapabilityResult(ctx context.Context, runtime *connectorCapabilityRuntime, handler connector.CapabilityHandler, command connector.AICommand, result connector.CapabilityResult) error {
	target := connector.MessageTarget{ChatID: runtime.message.ExternalChatID}
	if result.Data == nil {
		result.Data = map[string]any{}
	}
	result.Data["connectorTaskId"] = runtime.task.ID
	mediaSent := false
	switch command.Intent {
	case connector.IntentImageGenerate, connector.IntentImageEdit:
		if req, ok := result.Data["generationRequest"].(generation.CreateRequest); ok {
			for index, image := range req.GeneratedImages {
				raw, _, ext, err := readGeneratedArtifact(ctx, image.URL, image.ContentType)
				if err != nil {
					continue
				}
				sent, err := runtime.client.SendImage(ctx, target, connector.OutgoingMessage{Image: bytes.NewReader(raw), FileName: fmt.Sprintf("%s-%d.%s", result.InternalTaskID, index+1, ext)})
				if err == nil {
					mediaSent = true
					_ = a.repo.insertOutboundMessage(ctx, runtime.item, runtime.message.ExternalChatID, runtime.message.ExternalUserID, sent.ExternalMessageID, "image", map[string]any{"generationTaskId": result.InternalTaskID, "index": index + 1})
				}
			}
		}
	case connector.IntentVideoGenerate, connector.IntentVideoImageToVideo:
		if file, ok := result.Data["file"].(storagecenter.FileObject); ok && file.FileID != "" {
			sent, sendErr := sendConnectorStoredVideo(ctx, a.generator.fileService, runtime.client, target, command.InternalUserID, file)
			if sendErr != nil {
				return sendErr
			}
			mediaSent = true
			_ = a.repo.insertOutboundMessage(ctx, runtime.item, runtime.message.ExternalChatID, runtime.message.ExternalUserID, sent.ExternalMessageID, "file", map[string]any{"generationTaskId": result.InternalTaskID, "mediaType": "video"})
			if url, _, err := a.generator.connectorStoredFileURL(ctx, command.InternalUserID, file); err == nil {
				result.Data["downloadURL"] = url
			}
		}
	case connector.IntentPPTGenerate:
		raw, _ := result.Data["pptBytes"].([]byte)
		name := firstNonEmptyString(stringValue(result.Data["fileName"]), result.InternalTaskID+".pptx")
		if len(raw) > 0 {
			sent, err := runtime.client.SendFile(ctx, target, connector.OutgoingMessage{File: bytes.NewReader(raw), FileName: name, MIMEType: "application/vnd.openxmlformats-officedocument.presentationml.presentation"})
			if err == nil {
				mediaSent = true
				_ = a.repo.insertOutboundMessage(ctx, runtime.item, runtime.message.ExternalChatID, runtime.message.ExternalUserID, sent.ExternalMessageID, "file", map[string]any{"pptTaskId": result.InternalTaskID, "mediaType": "ppt"})
			}
		}
		if file, ok := result.Data["file"].(storagecenter.FileObject); ok && file.FileID != "" {
			if url, _, err := a.generator.connectorStoredFileURL(ctx, command.InternalUserID, file); err == nil {
				result.Data["downloadURL"] = url
			}
		}
	}
	message, err := handler.BuildResult(ctx, command, result)
	if err != nil {
		return err
	}
	var sent connector.SendResult
	if len(message.Card) > 0 {
		sent, err = runtime.client.SendCard(ctx, target, message)
	} else {
		sent, err = runtime.client.SendText(ctx, target, message)
	}
	if err != nil {
		return err
	}
	kind := "text"
	if len(message.Card) > 0 {
		kind = "card"
	}
	_ = a.repo.insertOutboundMessage(ctx, runtime.item, runtime.message.ExternalChatID, runtime.message.ExternalUserID, sent.ExternalMessageID, kind, map[string]any{"taskId": result.InternalTaskID, "mediaSent": mediaSent})
	return nil
}

func sendConnectorStoredVideo(ctx context.Context, fileService *storagecenter.Service, client *feishuconnector.Client, target connector.MessageTarget, userID string, file storagecenter.FileObject) (connector.SendResult, error) {
	if fileService == nil || client == nil || strings.TrimSpace(file.FileID) == "" {
		return connector.SendResult{}, errors.New("generated video private delivery is unavailable")
	}
	var sent connector.SendResult
	err := runGeneratedVideoProcess(ctx, func() error {
		stored, stream, err := fileService.OpenObject(ctx, storagecenter.AccessContext{TenantID: file.TenantID, UserID: userID}, file.FileID)
		if err != nil {
			return fmt.Errorf("open generated video for connector delivery: %w", err)
		}
		defer stream.Close()
		name := firstNonEmptyString(strings.TrimSpace(stored.OriginalName), strings.TrimSpace(file.OriginalName), file.FileID+".mp4")
		mimeType := firstNonEmptyString(strings.TrimSpace(stored.MIMEType), strings.TrimSpace(file.MIMEType), "video/mp4")
		sent, err = client.SendFile(ctx, target, connector.OutgoingMessage{File: stream, FileName: name, MIMEType: mimeType})
		return err
	})
	return sent, err
}

func connectorCommandFromIntent(item enterpriseConnector, binding connectorUserBinding, message connectorMessageRecord, intent connector.Intent, prompt string) connector.AICommand {
	parameters := map[string]any{
		"topic": intent.Topic, "prompt": prompt, "count": intent.Count, "size": intent.Size, "duration": intent.Duration,
		"aspect_ratio": intent.AspectRatio, "resolution": intent.Resolution, "model_id": intent.ModelID, "style": intent.Style,
		"generation_mode": intent.GenerationMode, "page_count": intent.PageCount, "audience": intent.Audience, "purpose": intent.Purpose,
		"template_id": intent.TemplateID, "theme": intent.Theme, "language": intent.Language,
		"use_enterprise_logo": intent.UseEnterpriseLogo, "use_enterprise_knowledge": intent.UseEnterpriseKnowledge,
	}
	return connector.AICommand{EnterpriseID: item.EnterpriseID, InternalUserID: binding.InternalUserID, ExternalUserID: message.ExternalUserID,
		ConnectorID: item.ID, Source: "feishu", ChatID: message.ExternalChatID, ExternalMessageID: message.ExternalMessageID,
		OriginalText: stringValue(message.Content["text"]), Intent: intent.Name, Parameters: parameters, Context: map[string]any{}}
}

func connectorTaskCreatedMessage(intent connector.Intent, taskID string, estimated int64) connector.OutgoingMessage {
	title := "AI 任务已创建"
	details := []string{"**任务状态：** 排队中", "**任务编号：** " + taskID, fmt.Sprintf("**预计积分：** %d", estimated)}
	switch intent.Name {
	case connector.IntentVideoGenerate, connector.IntentVideoImageToVideo:
		title = "视频任务已创建"
		details = append(details, fmt.Sprintf("**时长：** %d 秒", intent.Duration), "**比例：** "+intent.AspectRatio, "**分辨率：** "+intent.Resolution)
	case connector.IntentPPTGenerate:
		title = "PPT 任务已创建"
		details = append(details, fmt.Sprintf("**页数：** %d 页", intent.PageCount), "**模板：** "+intent.TemplateID)
	case connector.IntentImageEdit:
		title = "改图任务已创建"
	case connector.IntentImageGenerate:
		title = "生图任务已创建"
	}
	if strings.TrimSpace(intent.Topic) != "" {
		details = append(details, "**主题：** "+intent.Topic)
	}
	if strings.TrimSpace(intent.ModelID) != "" {
		details = append(details, "**模型：** "+intent.ModelID)
	}
	return connector.OutgoingMessage{Card: map[string]any{"config": map[string]any{"wide_screen_mode": true}, "header": map[string]any{"title": map[string]any{"tag": "plain_text", "content": title}, "template": "turquoise"}, "elements": []any{map[string]any{"tag": "div", "text": map[string]any{"tag": "lark_md", "content": strings.Join(details, "\n")}}}}}
}

func (a *connectorAPI) validateConnectorQuota(ctx context.Context, item enterpriseConnector, binding connectorUserBinding, capability string, estimated int64) error {
	if estimated < 0 {
		return errors.New("invalid estimated points")
	}
	if capability == connector.IntentImageGenerate || capability == connector.IntentImageEdit {
		usage, err := a.repo.dailyBindingUsage(ctx, item.EnterpriseID, binding.ID)
		if err != nil {
			return err
		}
		limit := item.Config.MemberDailyQuota
		if member := intValue(binding.Permission["dailyQuota"]); member > 0 {
			limit = member
		}
		if usage >= limit {
			return errors.New("member daily image quota exceeded")
		}
		return nil
	}
	var perRequest, daily, monthly int
	switch capability {
	case connector.IntentVideoGenerate, connector.IntentVideoImageToVideo:
		perRequest, daily, monthly = item.Config.VideoPerRequestPointLimit, item.Config.VideoDailyPointLimit, item.Config.VideoMonthlyPointLimit
		if v := intValue(binding.Permission["videoPerRequestLimit"]); v > 0 {
			perRequest = v
		}
		if v := intValue(binding.Permission["videoDailyLimit"]); v > 0 {
			daily = v
		}
		if v := intValue(binding.Permission["videoMonthlyLimit"]); v > 0 {
			monthly = v
		}
	case connector.IntentPPTGenerate:
		perRequest, daily, monthly = item.Config.PPTPerRequestPointLimit, item.Config.PPTDailyPointLimit, item.Config.PPTMonthlyPointLimit
		if v := intValue(binding.Permission["pptPerRequestLimit"]); v > 0 {
			perRequest = v
		}
		if v := intValue(binding.Permission["pptDailyLimit"]); v > 0 {
			daily = v
		}
		if v := intValue(binding.Permission["pptMonthlyLimit"]); v > 0 {
			monthly = v
		}
	default:
		return nil
	}
	if perRequest > 0 && estimated > int64(perRequest) {
		return fmt.Errorf("single task point limit exceeded: %d", perRequest)
	}
	now := time.Now().UTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	dayUsed, err := a.repo.bindingPointUsage(ctx, item.EnterpriseID, binding.ID, capability, dayStart)
	if err != nil {
		return err
	}
	if daily > 0 && dayUsed+estimated > int64(daily) {
		return fmt.Errorf("daily point limit exceeded: %d", daily)
	}
	monthUsed, err := a.repo.bindingPointUsage(ctx, item.EnterpriseID, binding.ID, capability, monthStart)
	if err != nil {
		return err
	}
	if monthly > 0 && monthUsed+estimated > int64(monthly) {
		return fmt.Errorf("monthly point limit exceeded: %d", monthly)
	}
	return nil
}

func connectorCapabilityResultPayload(result connector.CapabilityResult, command connector.AICommand) map[string]any {
	payload := map[string]any{"internalTaskId": result.InternalTaskID, "assetIds": result.AssetIDs, "capability": command.Intent, "sourceType": "feishu", "sourceTaskId": command.ExternalMessageID, "connectorId": command.ConnectorID, "externalUserId": command.ExternalUserID, "actualPoints": result.ActualCost}
	for _, key := range []string{"duration", "aspectRatio", "resolution", "pageCount", "template", "fileName", "fileSize", "model", "topic", "editMode"} {
		if value, ok := result.Data[key]; ok {
			payload[key] = value
		}
	}
	if len(command.ReferenceAssets) > 0 {
		payload["sourceReferenceAssetId"] = command.ReferenceAssets[0].ID
		payload["sourceReferenceTaskId"] = command.ReferenceAssets[0].TaskID
	}
	return payload
}
