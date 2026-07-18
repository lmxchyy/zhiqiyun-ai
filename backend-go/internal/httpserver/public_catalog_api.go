package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// publicCatalog contains presentation-only records. It is deliberately
// independent from user assets, tenant settings and provider configuration.
type publicCatalogAPI struct {
	store platformStore
}

var publicGuestExperienceEvents = map[string]struct{}{
	"GUEST_OPEN_APP": {}, "GUEST_VIEW_HOME": {}, "GUEST_OPEN_CREATOR": {}, "GUEST_INPUT_PROMPT": {}, "GUEST_CLICK_GENERATE": {},
	"LOGIN_MODAL_SHOW": {}, "LOGIN_START": {}, "LOGIN_SUCCESS": {}, "LOGIN_CANCEL": {},
	"PENDING_ACTION_RESUME_SUCCESS": {}, "PENDING_ACTION_RESUME_FAILED": {}, "GENERATION_SUCCESS_AFTER_LOGIN": {},
}

var publicGuestMetadataKeys = map[string]struct{}{
	"action": {}, "authMethod": {}, "module": {}, "platform": {}, "reason": {}, "route": {},
}

func sanitizePublicGuestMetadata(input map[string]any) map[string]any {
	output := make(map[string]any)
	for key, value := range input {
		if _, allowed := publicGuestMetadataKeys[key]; !allowed {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if len(text) > 120 {
			text = text[:120]
		}
		if text != "" {
			output[key] = text
		}
	}
	return output
}

func (a publicCatalogAPI) recordGuestExperienceEvent(w http.ResponseWriter, r *http.Request) {
	store, ok := a.store.(adminExperienceStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("experience analytics are unavailable"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var input struct {
		EventType string         `json:"eventType"`
		ModuleID  string         `json:"moduleId"`
		Metadata  map[string]any `json:"metadata"`
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	eventType := strings.ToUpper(strings.TrimSpace(input.EventType))
	if _, allowed := publicGuestExperienceEvents[eventType]; !allowed {
		writeError(w, http.StatusBadRequest, errors.New("invalid guest experience event type"))
		return
	}
	moduleID := strings.TrimSpace(input.ModuleID)
	if len(moduleID) > 80 {
		moduleID = moduleID[:80]
	}
	event := adminExperienceEvent{
		EventType: eventType,
		ActorRole: "GUEST",
		ModuleID:  moduleID,
		Metadata:  sanitizePublicGuestMetadata(input.Metadata),
	}
	if err := store.RecordAdminExperienceEvent(event); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (publicCatalogAPI) home(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"title":       "知启云 AI",
		"description": "从灵感、模板和参数开始创作，在生成、保存或下载时登录。",
		"capabilities": []map[string]any{
			{"id": "image", "name": "AI 生图", "description": "支持提示词、比例、模型与参考图"},
			{"id": "video", "name": "AI 视频", "description": "支持文生视频与图生视频"},
			{"id": "ppt", "name": "PPT 文档", "description": "从主题和大纲生成演示文稿"},
			{"id": "agent", "name": "智能体", "description": "浏览官方智能体与使用场景"},
		},
	})
}

func (publicCatalogAPI) cases(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"items": []map[string]any{
		{"id": "official-brand-poster", "name": "科技品牌发布海报", "prompt": "白色空间与青紫主光，产品居中，适合品牌发布活动", "category": "品牌设计", "model": "AI Image", "ratio": "3:4", "official": true, "status": "SUCCEEDED"},
		{"id": "official-commerce-main", "name": "高级电商商品主图", "prompt": "干净浅灰背景，柔和阴影，突出产品材质与细节", "category": "电商营销", "model": "AI Image", "ratio": "1:1", "official": true, "status": "SUCCEEDED"},
		{"id": "official-social-cover", "name": "生活方式内容封面", "prompt": "明亮自然光，真实生活场景，画面留白充足", "category": "内容封面", "model": "AI Image", "ratio": "3:4", "official": true, "status": "SUCCEEDED"},
		{"id": "official-ip-character", "name": "品牌 IP 角色设定", "prompt": "友好、简洁、易识别的品牌角色，包含正侧面设定", "category": "IP 角色", "model": "AI Image", "ratio": "1:1", "official": true, "status": "SUCCEEDED"},
	}})
}

func (publicCatalogAPI) templates(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"items": []map[string]any{
		{"id": "brand", "name": "品牌设计", "description": "LOGO、VI 与品牌主视觉", "capability": "image", "enabled": true},
		{"id": "commerce", "name": "电商营销", "description": "商品主图、详情图与促销海报", "capability": "image", "enabled": true},
		{"id": "presentation", "name": "商业演示", "description": "汇报、路演与培训课件", "capability": "ppt", "enabled": true},
	}})
}

func (publicCatalogAPI) agents(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"items": []map[string]any{
		{"id": "official-brand-agent", "name": "品牌设计智能体", "description": "协助梳理品牌视觉方向与创作提示词", "capabilities": []string{"image"}, "official": true, "enabled": true},
		{"id": "official-commerce-agent", "name": "电商营销智能体", "description": "协助生成商品图与营销素材方案", "capabilities": []string{"image", "document"}, "official": true, "enabled": true},
		{"id": "official-ppt-agent", "name": "PPT 文档智能体", "description": "协助生成大纲、页面结构和演示文稿", "capabilities": []string{"ppt", "document"}, "official": true, "enabled": true},
	}})
}
