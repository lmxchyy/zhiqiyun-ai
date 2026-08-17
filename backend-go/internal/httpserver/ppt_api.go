package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	"xianzhi-ai/backend-go/internal/config"
	chatprovider "xianzhi-ai/backend-go/internal/provider/chat"
	ocrprovider "xianzhi-ai/backend-go/internal/provider/ocr"
)

var (
	errPPTImageContainsText               = errors.New("generated ppt image contains readable text")
	errPPTVisualTextValidationUnavailable = errors.New("ppt visual text validation unavailable")
)

type pptOutlineGenerateRequest struct {
	Prompt                string `json:"prompt"`
	SlideCount            int    `json:"slideCount"`
	Language              string `json:"language"`
	Tone                  string `json:"tone"`
	TextContent           string `json:"textContent"`
	Audience              string `json:"audience"`
	Scenario              string `json:"scenario"`
	GenerationAspectRatio string `json:"generationAspectRatio"`
	AutoThemeEnabled      bool   `json:"autoThemeEnabled"`
	EnableWebSearch       bool   `json:"enableWebSearch"`
	TextModel             string `json:"textModel"`
	ImageSource           string `json:"imageSource"`
	ImageModel            string `json:"imageModel"`
}

type pptEstimateRequest struct {
	Prompt      string `json:"prompt"`
	SlideCount  int    `json:"slideCount"`
	TextModel   string `json:"textModel"`
	ImageSource string `json:"imageSource"`
}

type pptOutline = pptapp.Outline
type pptOutlineSlide = pptapp.OutlineSlide
type pptSlide = pptapp.Slide

type pptImageSearchResponse struct {
	ID         string `json:"id"`
	TaskID     string `json:"taskId,omitempty"`
	URL        string `json:"url"`
	StorageRef string `json:"storageRef,omitempty"`
	Title      string `json:"title"`
	Source     string `json:"source"`
}

type pptImageGenerateRequest struct {
	Slide            pptSlide           `json:"slide"`
	Prompt           string             `json:"prompt"`
	DeckTitle        string             `json:"deckTitle"`
	Theme            string             `json:"theme"`
	Language         string             `json:"language"`
	ImageSource      string             `json:"imageSource"`
	ImageModel       string             `json:"imageModel"`
	VisualPlan       *pptapp.VisualPlan `json:"visualPlan,omitempty"`
	ImageStyle       string             `json:"imageStyle,omitempty"`
	PeopleStyle      string             `json:"peopleStyle,omitempty"`
	ImageLighting    string             `json:"imageLighting,omitempty"`
	ImageComposition string             `json:"imageComposition,omitempty"`
	RetryAttempt     int                `json:"-"`
}

type pptRegenerateVisualRequest struct {
	VisualType         string `json:"visualType"`
	Style              string `json:"style"`
	Composition        string `json:"composition"`
	CustomInstruction  string `json:"customInstruction"`
	KeepCurrentContent bool   `json:"keepCurrentContent"`
}

type pptRegenerateVisualResponse struct {
	TaskID string       `json:"taskId,omitempty"`
	Status string       `json:"status"`
	Slide  pptapp.Slide `json:"slide"`
}

type pptRestoreVisualRequest struct {
	CreatedAt  string `json:"createdAt"`
	URL        string `json:"url"`
	StorageRef string `json:"storageRef"`
}

type pptModelOption struct {
	Label        string `json:"label"`
	Value        string `json:"value"`
	Provider     string `json:"provider,omitempty"`
	ProviderType string `json:"providerType,omitempty"`
	Group        string `json:"group,omitempty"`
	Description  string `json:"description,omitempty"`
	Downloadable bool   `json:"downloadable,omitempty"`
	Disabled     bool   `json:"disabled,omitempty"`
}

var pptTextModels = []pptModelOption{
	{Label: "Kimi K2.6", Value: "kimi-k2.6", Provider: "Moonshot/Kimi", ProviderType: "openai", Group: "当前后台配置", Description: "通过后台 OpenAI 兼容网关生成 PPT 大纲"},
	{Label: "GPT-4o-mini", Value: "gpt-4o-mini", Provider: "OpenAI", ProviderType: "openai", Group: "云模型", Description: "快速生成日常演示草稿"},
	{Label: "GPT-4o", Value: "gpt-4o", Provider: "OpenAI", ProviderType: "openai", Group: "云模型", Description: "兼顾速度和质量的云模型"},
	{Label: "GPT-4.1-mini", Value: "gpt-4.1-mini", Provider: "OpenAI", ProviderType: "openai", Group: "云模型", Description: "适合结构化生成的轻量模型"},
	{Label: "GPT-4.1-nano", Value: "gpt-4.1-nano", Provider: "OpenAI", ProviderType: "openai", Group: "云模型", Description: "更快、更低成本的 GPT-4.1 模型"},
	{Label: "GPT-4.1", Value: "gpt-4.1", Provider: "OpenAI", ProviderType: "openai", Group: "云模型", Description: "复杂演示的高质量生成模型"},
	{Label: "GPT-5.2", Value: "gpt-5.2", Provider: "OpenAI", ProviderType: "openai", Group: "云模型", Description: "旗舰 GPT 模型"},
	{Label: "GPT-5.2 Chat", Value: "gpt-5.2-chat-latest", Provider: "OpenAI", ProviderType: "openai", Group: "云模型", Description: "ChatGPT 风格的 GPT-5.2 模型"},
	{Label: "GPT-5.2 Pro", Value: "gpt-5.2-pro", Provider: "OpenAI", ProviderType: "openai", Group: "云模型", Description: "适合更高难度任务的高算力模型"},
	{Label: "GPT-5.1", Value: "gpt-5.1", Provider: "OpenAI", ProviderType: "openai", Group: "云模型", Description: "支持复杂推理的旗舰模型"},
	{Label: "GPT-5", Value: "gpt-5", Provider: "OpenAI", ProviderType: "openai", Group: "云模型", Description: "上一代 GPT-5 推理模型"},
	{Label: "GPT-5-mini", Value: "gpt-5-mini", Provider: "OpenAI", ProviderType: "openai", Group: "云模型", Description: "更快、更经济的 GPT-5 模型"},
	{Label: "GPT-5-nano", Value: "gpt-5-nano", Provider: "OpenAI", ProviderType: "openai", Group: "云模型", Description: "最快、最低成本的 GPT-5 模型"},
	{Label: "DeepSeek-V3", Value: "deepseek-v3", Provider: "NewAPI", ProviderType: "newapi", Group: "平台模型", Description: "通过当前平台网关接入的 DeepSeek 文本模型"},
	{Label: "Qwen-Max", Value: "qwen-max", Provider: "NewAPI", ProviderType: "newapi", Group: "平台模型", Description: "通过当前平台网关接入的通义文本模型"},
	{Label: "Doubao Pro", Value: "doubao-pro", Provider: "NewAPI", ProviderType: "newapi", Group: "平台模型", Description: "通过当前平台网关接入的火山文本模型"},
	{Label: "llama3.1:8b", Value: "ollama:llama3.1:8b", Provider: "Ollama", ProviderType: "ollama", Group: "可下载 Ollama 模型", Description: "参考项目的本地模型建议，首次使用时可由后端拉取", Downloadable: true},
	{Label: "llama3.1:70b", Value: "ollama:llama3.1:70b", Provider: "Ollama", ProviderType: "ollama", Group: "可下载 Ollama 模型", Description: "参考项目的本地模型建议，适合更强本地推理", Downloadable: true},
	{Label: "llama3.2:3b", Value: "ollama:llama3.2:3b", Provider: "Ollama", ProviderType: "ollama", Group: "可下载 Ollama 模型", Description: "参考项目的轻量本地模型建议", Downloadable: true},
	{Label: "llama3.2:8b", Value: "ollama:llama3.2:8b", Provider: "Ollama", ProviderType: "ollama", Group: "可下载 Ollama 模型", Description: "参考项目的通用本地模型建议", Downloadable: true},
	{Label: "mistral:7b", Value: "ollama:mistral:7b", Provider: "Ollama", ProviderType: "ollama", Group: "可下载 Ollama 模型", Description: "参考项目的本地模型建议", Downloadable: true},
	{Label: "codellama:7b", Value: "ollama:codellama:7b", Provider: "Ollama", ProviderType: "ollama", Group: "可下载 Ollama 模型", Description: "参考项目的代码和结构化内容本地模型建议", Downloadable: true},
	{Label: "qwen2.5:7b", Value: "ollama:qwen2.5:7b", Provider: "Ollama", ProviderType: "ollama", Group: "可下载 Ollama 模型", Description: "参考项目的中文友好本地模型建议", Downloadable: true},
	{Label: "gemma2:9b", Value: "ollama:gemma2:9b", Provider: "Ollama", ProviderType: "ollama", Group: "可下载 Ollama 模型", Description: "参考项目的高性价比本地模型建议", Downloadable: true},
	{Label: "phi3:3.8b", Value: "ollama:phi3:3.8b", Provider: "Ollama", ProviderType: "ollama", Group: "可下载 Ollama 模型", Description: "参考项目的小尺寸本地模型建议", Downloadable: true},
	{Label: "neural-chat:7b", Value: "ollama:neural-chat:7b", Provider: "Ollama", ProviderType: "ollama", Group: "可下载 Ollama 模型", Description: "参考项目的对话式本地模型建议", Downloadable: true},
	{Label: "LM Studio 本地模型", Value: "lmstudio:local-model", Provider: "LM Studio", ProviderType: "lmstudio", Group: "本地 LM Studio 模型", Description: "后端接入本地模型发现接口后会替换为真实模型列表"},
}

var pptImageModels = []pptModelOption{
	{Label: "系统默认图片模型", Value: "default-image", Provider: "system", ProviderType: "system"},
	{Label: "gpt-image-2", Value: "gpt-image-2", Provider: "OpenAI", ProviderType: "openai"},
	{Label: "ComfyUI 工作流", Value: "comfyui-ppt", Provider: "ComfyUI", ProviderType: "comfyui"},
}

var pptVisualTaskLocks sync.Map

func (a api) createPPTGenerationTask(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var req pptapp.GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.UserID = user.ID
	req.TenantID = effectiveTenantID(user)
	req.OrganizationID = user.OrganizationID
	if err := a.checkMiniProgramText(r.Context(), r, user, req.Prompt); err != nil {
		writeContentSecurityError(w, err)
		return
	}
	pageCount := req.SlideCount
	if req.Outline != nil && len(req.Outline.Slides) > 0 {
		pageCount = len(req.Outline.Slides)
	}
	capability, err := a.preparePPTCapabilityRequest(data, user, req.Prompt, req.TextModel, pageCount, pptImagesEnabled(req.ImageSource), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.TextModel = capability.Model
	req.SlideCount = int(anyFloatOrDefault(capability.Params["page_count"], 5))
	if req.Outline == nil {
		outline, err := a.generatePPTOutlineWithModel(r.Context(), outlineRequestFromGenerate(req))
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		req.Outline = &outline
		req.SlideCount = len(outline.Slides)
	}
	externalActive := 0
	concurrencyLimit := 0
	if !strings.EqualFold(stringValue(capability.Params["billing_scope"]), contextEnterprise) {
		concurrencyLimit = adminPlanConcurrencyLimit(data, user)
		var listErr error
		externalActive, listErr = activeGenerationTaskCountForStore(a.store, user.ID)
		if listErr != nil {
			writeError(w, http.StatusInternalServerError, listErr)
			return
		}
	}
	resp, err := a.pptService.GenerateWithConcurrency(req, externalActive, concurrencyLimit)
	if err != nil {
		if errors.Is(err, pptapp.ErrConcurrency) {
			writeError(w, http.StatusTooManyRequests, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	task, err := a.pptService.GetTask(user.ID, resp.TaskID)
	if err != nil {
		_ = a.pptService.Delete(user.ID, resp.TaskID)
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := a.store.RecordPPTGenerationUsage(task); err != nil {
		_ = a.pptService.Delete(user.ID, resp.TaskID)
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if shouldAutoGeneratePPTImages(req, a.cfg) {
		if pptAutoImageEnabled(a.cfg) {
			go a.runPPTTaskImageGeneration(user, task)
		} else {
			log.Printf("ppt automatic image generation disabled presentationId=%s userId=%s mode=%s", task.TaskID, user.ID, firstNonEmptyString(a.cfg.PPTAutoImageMode, "enabled"))
		}
	}
	writeJSON(w, resp)
}

func (a api) estimatePPTGenerationCost(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var req pptEstimateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	capability, err := a.preparePPTCapabilityRequest(data, user, req.Prompt, req.TextModel, req.SlideCount, pptImagesEnabled(req.ImageSource), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	slideCount := int(anyFloatOrDefault(capability.Params["page_count"], 5))
	task := pptapp.Task{
		UserID:      user.ID,
		Prompt:      strings.TrimSpace(req.Prompt),
		SlideCount:  slideCount,
		TextModel:   capability.Model,
		ImageSource: normalizedPPTImageSource(req.ImageSource),
	}
	pointCost := pptPointCostWithRules(task, data)
	response := map[string]any{
		"pointCost":  pointCost,
		"slideCount": slideCount,
		"model":      capability.Model,
	}
	if account, accountErr := a.store.PointAccount(user.ID); accountErr == nil {
		response["availablePoints"] = account.Available
		response["sufficient"] = account.Available >= pointCost
	}
	writeJSON(w, response)
}

func (a api) listPPTTextModels(w http.ResponseWriter, r *http.Request) {
	if _, _, err := a.authenticatedUser(r); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, a.pptTextModelOptions())
}

func (a api) listPPTImageModels(w http.ResponseWriter, r *http.Request) {
	if _, _, err := a.authenticatedUser(r); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, pptImageModels)
}

func (a api) getPPTTask(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	task, err := a.pptService.GetTask(user.ID, taskID)
	if err != nil {
		writePPTError(w, err)
		return
	}
	task = a.materializePPTTaskVisualURLs(r.Context(), user, task)
	writeJSON(w, task)
}

func (a api) listPPTHistory(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	items, err := a.pptService.HistoryWithError(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for index := range items {
		items[index] = a.materializePPTTaskVisualURLs(r.Context(), user, items[index])
	}
	writeJSON(w, items)
}

func (a api) deletePPTTask(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	if err := a.pptService.Delete(user.ID, taskID); err != nil {
		writePPTError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a api) generatePPTOutline(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var req pptOutlineGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, pptapp.ErrInvalidPrompt)
		return
	}
	if err := a.checkMiniProgramText(r.Context(), r, user, req.Prompt); err != nil {
		writeContentSecurityError(w, err)
		return
	}
	capability, err := a.preparePPTCapabilityRequest(data, user, req.Prompt, req.TextModel, req.SlideCount, pptImagesEnabled(req.ImageSource), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.TextModel = capability.Model
	req.SlideCount = int(anyFloatOrDefault(capability.Params["page_count"], 5))
	outline, err := a.generatePPTOutlineWithModel(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, outline)
}

func (a api) savePPTOutline(w http.ResponseWriter, r *http.Request) {
	if _, _, err := a.authenticatedUser(r); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var outline pptOutline
	if err := json.NewDecoder(r.Body).Decode(&outline); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	outline.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	writeJSON(w, outline)
}

func (a api) regeneratePPTSlide(w http.ResponseWriter, r *http.Request) {
	if _, _, err := a.authenticatedUser(r); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var slide pptSlide
	if err := json.NewDecoder(r.Body).Decode(&slide); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if slide.ID == "" {
		slide.ID = strings.TrimSpace(r.PathValue("slideId"))
	}
	if slide.Layout == "" {
		slide.Layout = "content"
	}
	if len(slide.BulletPoints) == 0 {
		slide.BulletPoints = []string{"补充核心观点", "强化页面表达"}
	}
	slide.SpeakerNotes = "当前页已重新生成，后续可接入真实 PPT 页面生成服务。"
	writeJSON(w, slide)
}

func (a api) updatePPTSlide(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var slide pptSlide
	if err := json.NewDecoder(r.Body).Decode(&slide); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	taskID := strings.TrimSpace(r.PathValue("id"))
	slideID := strings.TrimSpace(r.PathValue("slideId"))
	task, err := a.pptService.UpdateSlideContent(user.ID, taskID, slideID, slide)
	if err != nil {
		writePPTError(w, err)
		return
	}
	for _, updated := range task.Slides {
		if updated.ID == slideID {
			writeJSON(w, updated)
			return
		}
	}
	writePPTError(w, pptapp.ErrTaskNotFound)
}

func (a api) updatePPTSlideImage(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var req struct {
		ImageURL string `json:"imageUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.ImageURL) == "" {
		writeError(w, http.StatusBadRequest, errors.New("ppt slide image url is required"))
		return
	}
	taskID := strings.TrimSpace(r.PathValue("id"))
	slideID := strings.TrimSpace(r.PathValue("slideId"))
	updated, err := a.pptService.UpdateSlideImage(user.ID, taskID, slideID, req.ImageURL)
	if err != nil {
		writePPTError(w, err)
		return
	}
	updated = a.materializePPTTaskVisualURLs(r.Context(), user, updated)
	for _, slide := range updated.Slides {
		if slide.ID == slideID {
			writeJSON(w, slide)
			return
		}
	}
	writePPTError(w, pptapp.ErrTaskNotFound)
}

func (a api) regeneratePPTSlideVisual(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	presentationID := strings.TrimSpace(r.PathValue("id"))
	slideID := strings.TrimSpace(r.PathValue("slideId"))
	task, slide, err := a.pptService.GetSlide(user.ID, presentationID, slideID)
	if err != nil {
		writePPTError(w, err)
		return
	}
	var req pptRegenerateVisualRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	lockKey := user.ID + ":" + presentationID + ":" + slideID
	releaseVisual, acquired, lockErr := a.tryAcquirePPTVisualOperation(r.Context(), lockKey)
	if lockErr != nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("acquire visual generation lock: %w", lockErr))
		return
	}
	if !acquired {
		writeError(w, http.StatusConflict, errors.New("a visual generation task is already active for this slide"))
		return
	}
	defer releaseVisual()

	plan, planErr := a.generatePPTVisualPlan(r.Context(), task, slide)
	if planErr != nil && slide.VisualPlan != nil {
		plan = *slide.VisualPlan
	}
	if value := strings.TrimSpace(req.VisualType); value != "" {
		plan.VisualType = value
	}
	if value := strings.TrimSpace(req.Style); value != "" {
		plan.Style = value
	}
	if value := strings.TrimSpace(req.CustomInstruction); value != "" {
		plan.Action = concisePPTVisualIdea(value)
	}
	plan = pptapp.NormalizeVisualPlan(plan, pptapp.VisualPlannerInput{
		DeckTheme: task.Theme, SlideType: slide.SlideType, SlideTitle: slide.Title,
		CoreIdea: concisePPTVisualIdea(slide.Content), Layout: slide.Layout,
		ImagePosition: req.Composition, ImageStyle: firstNonEmptyString(plan.Style, task.ImageStyle),
		PeopleStyle: task.PeopleStyle, ImageLighting: task.ImageLighting,
		ImageComposition: firstNonEmptyString(req.Composition, task.ImageComposition),
	})
	if _, err := a.pptService.UpdateSlideVisualPlan(user.ID, presentationID, slideID, plan, "", "processing", ""); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !plan.ImageRequired {
		updated, err := a.pptService.DisableSlideVisual(user.ID, presentationID, slideID, plan)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		updated = a.materializePPTTaskVisualURLs(r.Context(), user, updated)
		for _, item := range updated.Slides {
			if item.ID == slideID {
				writeJSON(w, pptRegenerateVisualResponse{Status: "success", Slide: item})
				return
			}
		}
		writePPTError(w, pptapp.ErrTaskNotFound)
		return
	}

	imageReq := pptImageGenerateRequest{
		Slide: slide, Prompt: task.Prompt, DeckTitle: task.Title, Theme: task.Theme,
		Language: task.Language, ImageSource: "ai", ImageModel: task.ImageModel,
		VisualPlan: &plan, ImageStyle: task.ImageStyle, PeopleStyle: task.PeopleStyle,
		ImageLighting: task.ImageLighting, ImageComposition: firstNonEmptyString(req.Composition, task.ImageComposition),
	}
	model := pptImageProviderModel(imageReq.ImageModel, a.cfg.ImageModel)
	service, err := a.generationServiceForPPTImage(user, model)
	if err != nil {
		_, _ = a.pptService.UpdateSlideVisualPlan(user.ID, presentationID, slideID, plan, "", "failed", generationErrorMessage(err))
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var image pptImageSearchResponse
	var generationErr error
	for attempt := 1; attempt <= 2; attempt++ {
		imageReq.RetryAttempt = attempt
		ctx, cancel := context.WithTimeout(r.Context(), 115*time.Second)
		image, generationErr = a.generateBillablePPTImage(ctx, user, service, imageReq, model, presentationID)
		cancel()
		if generationErr == nil && strings.TrimSpace(image.URL) != "" {
			break
		}
		if r.Context().Err() != nil {
			break
		}
		if attempt < 2 {
			time.Sleep(time.Second)
		}
	}
	if generationErr != nil {
		_, _ = a.pptService.UpdateSlideVisualPlan(user.ID, presentationID, slideID, plan, "", "failed", generationErrorMessage(generationErr))
		writeError(w, http.StatusBadGateway, generationErr)
		return
	}
	updated, err := a.pptService.CompleteSlideVisual(user.ID, presentationID, slideID, plan, pptapp.VisualAsset{
		URL: firstNonEmptyString(image.StorageRef, image.URL), TaskID: image.TaskID, ModelName: model, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		_, _ = a.pptService.UpdateSlideVisualPlan(user.ID, presentationID, slideID, plan, "", "failed", generationErrorMessage(err))
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	updated = a.materializePPTTaskVisualURLs(r.Context(), user, updated)
	for _, item := range updated.Slides {
		if item.ID == slideID {
			writeJSON(w, pptRegenerateVisualResponse{TaskID: image.TaskID, Status: "success", Slide: item})
			return
		}
	}
	writePPTError(w, pptapp.ErrTaskNotFound)
}

func acquirePPTVisualTask(lockMap *sync.Map, key string) bool {
	if lockMap == nil {
		return false
	}
	_, loaded := lockMap.LoadOrStore(key, struct{}{})
	return !loaded
}

func (a api) tryAcquirePPTVisualOperation(ctx context.Context, key string) (func(), bool, error) {
	lockMap := a.pptVisualTasks
	if lockMap == nil {
		lockMap = &pptVisualTaskLocks
	}
	if !acquirePPTVisualTask(lockMap, key) {
		return func() {}, false, nil
	}
	releaseLocal := func() { lockMap.Delete(key) }
	if a.pptVisualLocker == nil {
		return releaseLocal, true, nil
	}
	releaseDistributed, acquired, err := a.pptVisualLocker.TryAcquire(ctx, key, pptVisualLockTTL)
	if err != nil || !acquired {
		releaseLocal()
		return func() {}, acquired, err
	}
	return func() {
		releaseDistributed()
		releaseLocal()
	}, true, nil
}

func (a api) deletePPTSlideVisual(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	presentationID := strings.TrimSpace(r.PathValue("id"))
	slideID := strings.TrimSpace(r.PathValue("slideId"))
	lockKey := user.ID + ":" + presentationID + ":" + slideID
	releaseVisual, acquired, lockErr := a.tryAcquirePPTVisualOperation(r.Context(), lockKey)
	if lockErr != nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("acquire visual operation lock: %w", lockErr))
		return
	}
	if !acquired {
		writeError(w, http.StatusConflict, errors.New("a visual operation is already active for this slide"))
		return
	}
	defer releaseVisual()
	_, slide, err := a.pptService.GetSlide(user.ID, presentationID, slideID)
	if err != nil {
		writePPTError(w, err)
		return
	}
	plan := pptapp.VisualPlan{VisualType: "none", ImageRequired: false}
	if slide.VisualPlan != nil {
		plan = *slide.VisualPlan
		plan.VisualType = "none"
		plan.ImageRequired = false
	}
	updated, err := a.pptService.DisableSlideVisual(user.ID, presentationID, slideID, plan)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	updated = a.materializePPTTaskVisualURLs(r.Context(), user, updated)
	for _, item := range updated.Slides {
		if item.ID == slideID {
			writeJSON(w, pptRegenerateVisualResponse{Status: "success", Slide: item})
			return
		}
	}
}

func (a api) restorePPTSlideVisual(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	presentationID := strings.TrimSpace(r.PathValue("id"))
	slideID := strings.TrimSpace(r.PathValue("slideId"))
	var req pptRestoreVisualRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	visualReference := firstNonEmptyString(req.StorageRef, req.URL)
	if strings.TrimSpace(req.CreatedAt) == "" || strings.TrimSpace(visualReference) == "" {
		writeError(w, http.StatusBadRequest, errors.New("visual history createdAt and url are required"))
		return
	}
	lockKey := user.ID + ":" + presentationID + ":" + slideID
	releaseVisual, acquired, lockErr := a.tryAcquirePPTVisualOperation(r.Context(), lockKey)
	if lockErr != nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("acquire visual operation lock: %w", lockErr))
		return
	}
	if !acquired {
		writeError(w, http.StatusConflict, errors.New("a visual operation is already active for this slide"))
		return
	}
	defer releaseVisual()
	updated, err := a.pptService.RestoreSlideVisual(user.ID, presentationID, slideID, req.CreatedAt, visualReference)
	if err != nil {
		writePPTError(w, err)
		return
	}
	updated = a.materializePPTTaskVisualURLs(r.Context(), user, updated)
	for _, slide := range updated.Slides {
		if slide.ID == slideID {
			log.Printf("ppt visual restored presentationId=%s slideId=%s visualCreatedAt=%s", presentationID, slideID, req.CreatedAt)
			writeJSON(w, pptRegenerateVisualResponse{Status: "success", Slide: slide})
			return
		}
	}
	writePPTError(w, pptapp.ErrTaskNotFound)
}

func (a api) exportPPT(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var req pptExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	taskID := strings.TrimSpace(req.TaskID)
	if taskID == "" {
		writeError(w, http.StatusBadRequest, errors.New("ppt task id is required"))
		return
	}
	task, err := a.pptService.GetTask(user.ID, taskID)
	if err != nil {
		writePPTError(w, err)
		return
	}
	task = a.materializePPTTaskVisualURLs(r.Context(), user, task)
	payload, err := buildPPTX(task)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	fileName := pptxDownloadFileName(task)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.presentationml.presentation")
	w.Header().Set("Content-Disposition", pptxContentDisposition(fileName))
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(payload); err != nil {
		log.Printf("ppt export write failed task=%s err=%v", task.TaskID, err)
	}
}

func (a api) downloadPPTExport(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	task, err := a.pptService.GetTask(user.ID, taskID)
	if err != nil {
		writePPTError(w, err)
		return
	}
	task = a.materializePPTTaskVisualURLs(r.Context(), user, task)
	payload, err := buildPPTX(task)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	fileName := pptxDownloadFileName(task)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.presentationml.presentation")
	w.Header().Set("Content-Disposition", pptxContentDisposition(fileName))
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(payload); err != nil {
		log.Printf("ppt direct export write failed task=%s err=%v", task.TaskID, err)
	}
}

func (a api) exportPDF(w http.ResponseWriter, r *http.Request) {
	if _, _, err := a.authenticatedUser(r); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	writeError(w, http.StatusNotImplemented, errors.New("ppt pdf export is not available yet"))
}

func (a api) generatePPTImage(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req, err := decodePPTImageGenerateRequest(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	model := pptImageProviderModel(req.ImageModel, a.cfg.ImageModel)
	service, err := a.generationServiceForPPTImage(user, model)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 115*time.Second)
	defer cancel()
	response, err := a.generateBillablePPTImage(ctx, user, service, req, model, "")
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, response)
}

func shouldAutoGeneratePPTImages(req pptapp.GenerateRequest, cfg config.Config) bool {
	if strings.TrimSpace(req.ImageSource) != "ai" {
		return false
	}
	model := pptImageProviderModel(req.ImageModel, cfg.ImageModel)
	if strings.TrimSpace(model) == "" {
		return false
	}
	return req.Outline != nil && len(req.Outline.Slides) > 0
}

func pptImagesEnabled(imageSource string) bool {
	return normalizedPPTImageSource(imageSource) != "none"
}

func pptVisualPlannerModelEnabled(cfg config.Config) bool {
	switch strings.ToLower(strings.TrimSpace(cfg.PPTVisualPlannerMode)) {
	case "local", "disabled", "off", "false", "0":
		return false
	default:
		return true
	}
}

func pptAutoImageEnabled(cfg config.Config) bool {
	switch strings.ToLower(strings.TrimSpace(cfg.PPTAutoImageMode)) {
	case "disabled", "off", "false", "0":
		return false
	default:
		return true
	}
}

func pptVisualOCRStrict(cfg config.Config) bool {
	switch strings.ToLower(strings.TrimSpace(cfg.PPTVisualOCRFailureMode)) {
	case "fail_open", "fail-open", "open":
		return false
	default:
		return true
	}
}

func (a api) runPPTTaskImageGeneration(user adminUser, task pptapp.Task) {
	if task.TaskID == "" || len(task.Slides) == 0 {
		return
	}
	parallelism := 3
	if len(task.Slides) < parallelism {
		parallelism = len(task.Slides)
	}
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup
	for _, slide := range task.Slides {
		if hasRealPPTImageURL(slide.ImageURL) {
			continue
		}
		if !pptapp.ShouldGenerateImageForSlide(slide) {
			continue
		}
		slide := slide
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			lockKey := user.ID + ":" + task.TaskID + ":" + slide.ID
			lockCtx, lockCancel := context.WithTimeout(context.Background(), 5*time.Second)
			releaseVisual, acquired, lockErr := a.tryAcquirePPTVisualOperation(lockCtx, lockKey)
			lockCancel()
			if lockErr != nil {
				log.Printf("ppt image lock failed presentationId=%s slideId=%s modelName=%s err=%v", task.TaskID, slide.ID, task.ImageModel, lockErr)
				return
			}
			if !acquired {
				log.Printf("ppt image generation skipped because visual task is active presentationId=%s slideId=%s modelName=%s", task.TaskID, slide.ID, task.ImageModel)
				return
			}
			defer releaseVisual()
			planCtx, planCancel := context.WithTimeout(context.Background(), 35*time.Second)
			if plan, planErr := a.generatePPTVisualPlan(planCtx, task, slide); planErr == nil {
				slide.VisualPlan = &plan
				_, _ = a.pptService.UpdateSlideVisualPlan(user.ID, task.TaskID, slide.ID, plan, "", "planned", "")
			}
			planCancel()
			req := pptImageGenerateRequest{
				Slide:            slide,
				Prompt:           task.Prompt,
				DeckTitle:        task.Title,
				Theme:            task.Theme,
				Language:         task.Language,
				ImageSource:      task.ImageSource,
				ImageModel:       task.ImageModel,
				VisualPlan:       slide.VisualPlan,
				ImageStyle:       task.ImageStyle,
				PeopleStyle:      task.PeopleStyle,
				ImageLighting:    task.ImageLighting,
				ImageComposition: task.ImageComposition,
			}
			var lastErr error
			for attempt := 1; attempt <= 2; attempt++ {
				req.RetryAttempt = attempt
				ctx, cancel := context.WithTimeout(context.Background(), 115*time.Second)
				image, err := a.generatePPTTaskSlideImage(ctx, user, task.TaskID, req)
				cancel()
				if err == nil && strings.TrimSpace(image.URL) != "" {
					plan := pptapp.NormalizeVisualPlan(pptapp.VisualPlan{}, pptapp.VisualPlannerInput{SlideType: slide.SlideType, SlideTitle: slide.Title, CoreIdea: concisePPTVisualIdea(slide.Content)})
					if slide.VisualPlan != nil {
						plan = *slide.VisualPlan
					}
					model := pptImageProviderModel(req.ImageModel, a.cfg.ImageModel)
					if _, updateErr := a.pptService.CompleteSlideVisual(user.ID, task.TaskID, slide.ID, plan, pptapp.VisualAsset{
						URL: firstNonEmptyString(image.StorageRef, image.URL), TaskID: image.TaskID, ModelName: model, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
					}); updateErr != nil {
						_, _ = a.pptService.UpdateSlideVisualPlan(user.ID, task.TaskID, slide.ID, plan, "", "failed", generationErrorMessage(updateErr))
						log.Printf("ppt image update failed task=%s slide=%s: %v", task.TaskID, slide.ID, updateErr)
					}
					return
				}
				if err != nil {
					lastErr = err
				} else {
					lastErr = errors.New("empty ppt image url")
				}
				time.Sleep(time.Duration(attempt) * time.Second)
			}
			if slide.VisualPlan != nil {
				_, _ = a.pptService.UpdateSlideVisualPlan(user.ID, task.TaskID, slide.ID, *slide.VisualPlan, "", "failed", generationErrorMessage(lastErr))
			}
			log.Printf("ppt image generation failed presentationId=%s slideId=%s page=%d modelName=%s err=%v", task.TaskID, slide.ID, slide.Page, task.ImageModel, lastErr)
		}()
	}
	wg.Wait()
}

func (a api) generatePPTVisualPlan(ctx context.Context, task pptapp.Task, slide pptapp.Slide) (pptapp.VisualPlan, error) {
	input := pptapp.VisualPlannerInput{
		DeckTheme: task.Theme, SlideType: slide.SlideType, SlideTitle: slide.Title,
		CoreIdea: concisePPTVisualIdea(slide.Content), ContentSummary: concisePPTVisualIdea(slide.Content),
		Layout: slide.Layout, ImagePosition: task.ImageComposition, ImageStyle: task.ImageStyle,
		PeopleStyle: task.PeopleStyle, ImageLighting: task.ImageLighting, ImageComposition: task.ImageComposition,
	}
	var modelPlanner pptapp.VisualPlanModelFunc
	if pptVisualPlannerModelEnabled(a.cfg) && a.pptProviderConfigured() {
		modelPlanner = func(ctx context.Context, plannerInput pptapp.VisualPlannerInput) (pptapp.VisualPlan, error) {
			model := firstNonEmptyString(task.TextModel, a.cfg.PPTTextModel)
			provider := chatprovider.NewOpenAICompatibleForModel(a.cfg, model)
			response, err := provider.Chat(ctx, generation.CreateRequest{
				Type: "CHAT_COMPLETION", Model: model,
				Params: map[string]any{
					"temperature": 0.1, "max_tokens": 1200,
					"messages": []any{
						map[string]any{"role": "system", "content": "Create a visual plan for one presentation slide. Return JSON only with fields visualType,imageRequired,chartRequired,diagramRequired,textInImage,subject,scene,action,objects,mood,composition,style,prompt,negativePrompt. Never copy slide prose or bullet lists into prompt. textInImage must be false for ordinary visuals."},
						map[string]any{"role": "user", "content": fmt.Sprintf("Deck theme: %s\nSlide type: %s\nSlide title: %s\nCore visual idea: %s\nLayout: %s\nImage position: %s\nDeck image style: %s\nExtract subject, scene, action, objects, mood, composition and style. Do not reproduce the slide copy.", plannerInput.DeckTheme, plannerInput.SlideType, plannerInput.SlideTitle, plannerInput.CoreIdea, plannerInput.Layout, plannerInput.ImagePosition, plannerInput.ImageStyle)},
					},
				},
			})
			if err != nil {
				return pptapp.VisualPlan{}, err
			}
			content := extractJSONObject(response.Message.Content)
			if content == "" {
				content = strings.TrimSpace(response.Message.Content)
			}
			var plan pptapp.VisualPlan
			if err := json.Unmarshal([]byte(content), &plan); err != nil {
				return pptapp.VisualPlan{}, err
			}
			return plan, nil
		}
	} else if !pptVisualPlannerModelEnabled(a.cfg) {
		log.Printf("ppt visual planner using local mode presentationId=%s slideId=%s mode=%s", task.TaskID, slide.ID, firstNonEmptyString(a.cfg.PPTVisualPlannerMode, "model"))
	}
	return pptapp.NewVisualPlannerService(modelPlanner).Plan(ctx, input)
}

func (a api) generatePPTTaskSlideImage(ctx context.Context, user adminUser, pptTaskID string, req pptImageGenerateRequest) (pptImageSearchResponse, error) {
	model := pptImageProviderModel(req.ImageModel, a.cfg.ImageModel)
	service, err := a.generationServiceForPPTImage(user, model)
	if err != nil {
		return pptImageSearchResponse{}, err
	}
	return a.generateBillablePPTImage(ctx, user, service, req, model, pptTaskID)
}

func (a api) generateBillablePPTImage(ctx context.Context, user adminUser, service generation.Service, req pptImageGenerateRequest, model string, pptTaskID string) (pptImageSearchResponse, error) {
	createReq := pptImageGenerationCreateRequest(user, req, model, pptTaskID)
	retryAttempt, retrySeed := createReq.Params["retryAttempt"], createReq.Params["seed"]
	delete(createReq.Params, "retryAttempt")
	delete(createReq.Params, "seed")
	data, err := a.store.AdminData()
	if err != nil {
		return pptImageSearchResponse{}, err
	}
	createReq, err = a.prepareGenerationRequest(data, user, createReq)
	if err != nil {
		return pptImageSearchResponse{}, fmt.Errorf("authorize ppt image generation: %w", err)
	}
	createReq.Params["retryAttempt"] = retryAttempt
	createReq.Params["seed"] = retrySeed
	task, err := a.store.CreatePendingGenerationTask(createReq)
	if err != nil {
		return pptImageSearchResponse{}, err
	}
	log.Printf("ppt visual generation started presentationId=%s slideId=%s taskId=%s modelName=%s", pptTaskID, req.Slide.ID, task.ID, model)
	prepared, err := service.PrepareImageTask(ctx, createReq)
	if err != nil {
		_, _ = a.store.FailGenerationTask(task.ID, generationErrorMessage(err))
		log.Printf("ppt visual generation failed presentationId=%s slideId=%s taskId=%s modelName=%s err=%v", pptTaskID, req.Slide.ID, task.ID, model, err)
		return pptImageSearchResponse{}, fmt.Errorf("ppt image generation failed: %w", err)
	}
	if err := a.validatePPTImageHasNoText(ctx, task.ID, prepared); err != nil {
		_, _ = a.store.FailGenerationTask(task.ID, generationErrorMessage(err))
		log.Printf("ppt visual text validation failed presentationId=%s slideId=%s taskId=%s modelName=%s retryAttempt=%d err=%v", pptTaskID, req.Slide.ID, task.ID, model, req.RetryAttempt, err)
		return pptImageSearchResponse{}, err
	}
	prepared, storedFiles, err := a.persistGeneratedImages(ctx, task.ID, prepared)
	if err != nil {
		_, _ = a.store.FailGenerationTask(task.ID, generationErrorMessage(err))
		log.Printf("ppt visual storage failed presentationId=%s slideId=%s taskId=%s modelName=%s err=%v", pptTaskID, req.Slide.ID, task.ID, model, err)
		return pptImageSearchResponse{}, fmt.Errorf("persist ppt image: %w", err)
	}
	imageURL := pptGeneratedImageURL(prepared)
	if imageURL == "" {
		a.cleanupGeneratedFiles(storedFiles)
		_, _ = a.store.FailGenerationTask(task.ID, "ppt image provider returned no image")
		log.Printf("ppt visual generation empty presentationId=%s slideId=%s taskId=%s modelName=%s", pptTaskID, req.Slide.ID, task.ID, model)
		return pptImageSearchResponse{}, errors.New("ppt image provider returned no image")
	}
	if _, err := a.store.CompleteGenerationTask(task.ID, prepared); err != nil {
		a.cleanupGeneratedFiles(storedFiles)
		_, _ = a.store.FailGenerationTask(task.ID, generationErrorMessage(err))
		log.Printf("ppt visual database update failed presentationId=%s slideId=%s taskId=%s modelName=%s err=%v", pptTaskID, req.Slide.ID, task.ID, model, err)
		return pptImageSearchResponse{}, err
	}
	storageRef := ""
	if len(storedFiles) > 0 {
		storageRef = pptStorageReference(storedFiles[0])
	}
	displayURL := imageURL
	if signed, ok := a.resolvePPTStorageReference(ctx, user, storageRef); ok {
		displayURL = signed
	}
	return pptImageSearchResponse{
		ID: fmt.Sprintf("ppt_ai_%d", time.Now().UnixNano()), TaskID: task.ID,
		URL: displayURL, StorageRef: storageRef,
		Title: firstNonEmpty([]string{req.Slide.Title, req.DeckTitle, "PPT"}) + " image", Source: "ai",
	}, nil
}

func (a api) validatePPTImageHasNoText(ctx context.Context, taskID string, req generation.CreateRequest) error {
	if strings.TrimSpace(a.cfg.KnowledgeOCREndpoint) == "" || len(req.GeneratedImages) == 0 {
		return nil
	}
	provider := ocrprovider.NewHTTP(a.cfg.KnowledgeOCRProvider, a.cfg.KnowledgeOCREndpoint, a.cfg.KnowledgeOCRAPIKey, 20*time.Second)
	for index, image := range req.GeneratedImages {
		raw, contentType, extension, err := readGeneratedArtifact(ctx, image.URL, image.ContentType)
		if err != nil {
			if !pptVisualOCRStrict(a.cfg) {
				return nil
			}
			return fmt.Errorf("%w: read generated artifact: %v", errPPTVisualTextValidationUnavailable, err)
		}
		units, err := provider.Recognize(ctx, knowledgeapp.SourceDocument{
			Name: fmt.Sprintf("ppt-visual-%02d.%s", index+1, extension), MIMEType: contentType, Content: raw,
		})
		if err != nil {
			log.Printf("ppt visual text validation unavailable presentationId=%s slideId=%s taskId=%s modelName=%s provider=%s failureMode=%s err=%v", stringValue(req.Params["pptTaskId"]), stringValue(req.Params["slideId"]), taskID, req.Model, provider.Code(), firstNonEmptyString(a.cfg.PPTVisualOCRFailureMode, "strict"), err)
			if !pptVisualOCRStrict(a.cfg) {
				return nil
			}
			return fmt.Errorf("%w: %v", errPPTVisualTextValidationUnavailable, err)
		}
		if pptOCRContainsReadableText(units) {
			return errPPTImageContainsText
		}
	}
	return nil
}

func pptOCRContainsReadableText(units []knowledgeapp.DocumentUnit) bool {
	readable := 0
	for _, unit := range units {
		for _, r := range unit.Title + " " + unit.Content {
			if r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '\u4e00' && r <= '\u9fff' {
				readable++
				if readable >= 2 {
					return true
				}
			}
		}
	}
	return false
}

func pptImageGenerationCreateRequest(user adminUser, req pptImageGenerateRequest, model string, pptTaskID string) generation.CreateRequest {
	negativePrompt := pptapp.DefaultNegativePrompt
	visualPlan := normalizePPTImageVisualPlan(req)
	if visualPlan != nil {
		negativePrompt = visualPlan.NegativePrompt
	}
	imageSize := "1536x1024"
	imageParams := map[string]any{
		"module_code":    moduleImageGeneration,
		"n":              1,
		"size":           imageSize,
		"quality":        "standard",
		"purpose":        "ppt_slide_illustration",
		"sourceModule":   "ppt-generation",
		"pptTaskId":      strings.TrimSpace(pptTaskID),
		"theme":          strings.TrimSpace(req.Theme),
		"language":       strings.TrimSpace(req.Language),
		"deckTitle":      strings.TrimSpace(req.DeckTitle),
		"slideId":        strings.TrimSpace(req.Slide.ID),
		"slidePage":      req.Slide.Page,
		"negativePrompt": negativePrompt,
		"visualPlan":     visualPlan,
		"seed":           time.Now().UnixNano(),
		"retryAttempt":   req.RetryAttempt,
	}
	if strings.EqualFold(strings.TrimSpace(model), "mock-standard") {
		imageParams["size"] = "1920x1080"
		delete(imageParams, "quality")
		delete(imageParams, "n")
	}
	return generation.CreateRequest{
		Type:       "TEXT_TO_IMAGE",
		ModuleCode: moduleImageGeneration,
		UserID:     user.ID,
		Prompt:     pptImagePrompt(req),
		Model:      model,
		Params:     imageParams,
	}
}

func pptGeneratedImageURL(prepared generation.CreateRequest) string {
	if len(prepared.GeneratedImages) > 0 {
		return strings.TrimSpace(prepared.GeneratedImages[0].URL)
	}
	if strings.EqualFold(strings.TrimSpace(prepared.Model), "mock-standard") {
		return promptPreviewImage(prepared.Prompt)
	}
	return ""
}

func hasRealPPTImageURL(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return true
	}
	if strings.HasPrefix(value, pptStorageReferenceScheme+"://") {
		return true
	}
	return strings.HasPrefix(value, "data:image/png") || strings.HasPrefix(value, "data:image/jpeg") || strings.HasPrefix(value, "data:image/jpg") || strings.HasPrefix(value, "data:image/webp")
}

func (a api) generatePPTImageReserved(w http.ResponseWriter, r *http.Request) {
	if _, _, err := a.authenticatedUser(r); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, pptImageSearchResponse{
		ID:     "ppt_image_reserved",
		URL:    "",
		Title:  "AI 配图接口已预留",
		Source: "ai",
	})
}

func (a api) generationServiceForPPTImage(user adminUser, model string) (generation.Service, error) {
	model = strings.TrimSpace(model)
	route := primaryUserModelRoute(user)
	if route.ID != "" && apiRouteUsableForGeneration(route) && model != "" && stringListContains(route.Models, model) {
		if routeService, ok, err := a.generationServiceForUserRoute(user, model); err != nil {
			return generation.Service{}, err
		} else if ok {
			return routeService, nil
		}
	}
	if runtimeChannel, ok := runtimeGenerationChannelFromEnv(); ok && (model == "" || apiChannelSupportsModel(runtimeChannel, model)) {
		data, err := a.store.AdminData()
		if err != nil {
			return generation.Service{}, err
		}
		return a.generationServiceForChannel(data, runtimeChannel)
	}
	if configuredService, ok, err := a.generationServiceForConfiguredModel(model); err != nil {
		return generation.Service{}, err
	} else if ok {
		return configuredService, nil
	}
	return a.generationService, nil
}

func decodePPTImageGenerateRequest(raw json.RawMessage) (pptImageGenerateRequest, error) {
	var req pptImageGenerateRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return pptImageGenerateRequest{}, err
	}
	if strings.TrimSpace(req.Slide.ID) == "" && strings.TrimSpace(req.Slide.Title) == "" && strings.TrimSpace(req.Slide.Content) == "" {
		var slide pptSlide
		if err := json.Unmarshal(raw, &slide); err == nil {
			req.Slide = slide
		}
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	req.DeckTitle = strings.TrimSpace(req.DeckTitle)
	req.Theme = strings.TrimSpace(req.Theme)
	req.Language = strings.TrimSpace(req.Language)
	req.ImageSource = strings.TrimSpace(req.ImageSource)
	req.ImageModel = strings.TrimSpace(req.ImageModel)
	req.Slide.Title = strings.TrimSpace(req.Slide.Title)
	req.Slide.Content = strings.TrimSpace(req.Slide.Content)
	req.Slide.SpeakerNotes = strings.TrimSpace(req.Slide.SpeakerNotes)
	if req.Slide.Page <= 0 {
		req.Slide.Page = 1
	}
	if req.Slide.Layout == "" {
		req.Slide.Layout = "imageText"
	}
	if req.Prompt == "" && req.DeckTitle == "" && req.Slide.Title == "" && req.Slide.Content == "" && len(req.Slide.BulletPoints) == 0 {
		return pptImageGenerateRequest{}, pptapp.ErrInvalidPrompt
	}
	return req, nil
}

func pptImageProviderModel(selected string, fallback string) string {
	model := normalizedPPTImageModel(selected)
	if model == "" || strings.EqualFold(model, "default-image") {
		if configured := strings.TrimSpace(fallback); configured != "" {
			return configured
		}
		return "gpt-image-2"
	}
	return model
}

func pptImagePrompt(req pptImageGenerateRequest) string {
	if plan := normalizePPTImageVisualPlan(req); plan != nil {
		return pptImagePromptVariation(plan.Prompt, req.RetryAttempt)
	}
	plan := pptapp.NormalizeVisualPlan(pptapp.VisualPlan{}, pptapp.VisualPlannerInput{
		DeckTheme: req.Theme, SlideType: req.Slide.SlideType, SlideTitle: req.Slide.Title,
		CoreIdea: concisePPTVisualIdea(req.Slide.Content), Layout: req.Slide.Layout,
		ImagePosition: req.ImageComposition, ImageStyle: req.ImageStyle,
		PeopleStyle: req.PeopleStyle, ImageLighting: req.ImageLighting,
	})
	return pptImagePromptVariation(plan.Prompt, req.RetryAttempt)
}

func normalizePPTImageVisualPlan(req pptImageGenerateRequest) *pptapp.VisualPlan {
	if req.VisualPlan == nil {
		return nil
	}
	plan := pptapp.NormalizeVisualPlan(*req.VisualPlan, pptapp.VisualPlannerInput{
		DeckTheme: req.Theme, SlideType: req.Slide.SlideType, SlideTitle: req.Slide.Title,
		CoreIdea: concisePPTVisualIdea(req.Slide.Content), ContentSummary: concisePPTVisualIdea(req.Slide.Content), Layout: req.Slide.Layout,
		ImagePosition: req.ImageComposition, ImageStyle: req.ImageStyle,
		PeopleStyle: req.PeopleStyle, ImageLighting: req.ImageLighting,
	})
	return &plan
}

func pptImagePromptVariation(prompt string, retryAttempt int) string {
	if retryAttempt <= 1 {
		return prompt
	}
	return strings.TrimSpace(prompt) + " Use a distinctly different camera angle and alternate object arrangement while preserving the same deck style and clean copy-safe negative space."
}

func concisePPTVisualIdea(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 36 {
		return string(runes[:36])
	}
	return value
}

func truncatePPTImagePromptText(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if maxRunes <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}

func (a api) searchPPTImages(w http.ResponseWriter, r *http.Request) {
	if _, _, err := a.authenticatedUser(r); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	if keyword == "" {
		keyword = "演示文稿配图"
	}
	writeJSON(w, []pptImageSearchResponse{
		{ID: "ppt_stock_1", URL: "", Title: keyword + " 配图一", Source: "stock"},
		{ID: "ppt_stock_2", URL: "", Title: keyword + " 配图二", Source: "stock"},
	})
}

func buildPPTOutline(req pptOutlineGenerateRequest) pptOutline {
	slideCount := req.SlideCount
	if slideCount <= 0 {
		slideCount = 5
	}
	title := titleFromPromptForOutline(req.Prompt)
	slides := make([]pptOutlineSlide, 0, slideCount)
	for i := 1; i <= slideCount; i++ {
		slideTitle := title
		summary := "围绕主题提炼核心观点，形成适合演示的页面内容。"
		points := []string{"明确页面目标", "提炼关键论据", "形成清晰表达"}
		switch i {
		case 1:
			slideTitle = title
			summary = "封面页，突出主题和演示定位。"
			points = []string{"主题名称", "目标受众", "演示价值"}
		case slideCount:
			slideTitle = "总结与行动建议"
			summary = "收束主要观点，并给出下一步行动建议。"
			points = []string{"关键结论", "落地路径", "下一步计划"}
		default:
			slideTitle = title + " · 第" + strconv.Itoa(i) + "部分"
		}
		slides = append(slides, pptOutlineSlide{
			Page:         i,
			Title:        slideTitle,
			Summary:      summary,
			BulletPoints: points,
			Layout:       normalizedPPTLayout("", i-1, slideCount, req.ImageSource),
			SlideType:    defaultPPTSlideType(i, slideCount),
		})
	}
	return pptOutline{
		Title:     title,
		Slides:    slides,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func defaultPPTSlideType(page, total int) string {
	if page == 1 {
		return "cover"
	}
	if page == total {
		return "statement"
	}
	return "text_image"
}

func (a api) generatePPTOutlineWithModel(ctx context.Context, req pptOutlineGenerateRequest) (pptOutline, error) {
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		return pptOutline{}, pptapp.ErrInvalidPrompt
	}
	if !a.pptProviderConfigured() {
		return buildPPTOutline(req), nil
	}
	model := strings.TrimSpace(req.TextModel)
	if model == "" {
		model = strings.TrimSpace(a.cfg.PPTTextModel)
	}
	if model == "" {
		return buildPPTOutline(req), nil
	}
	provider := chatprovider.NewOpenAICompatibleForModel(a.cfg, model)
	response, err := provider.Chat(ctx, generation.CreateRequest{
		Type:   "CHAT_COMPLETION",
		Prompt: req.Prompt,
		Model:  model,
		Params: map[string]any{
			"messages":    pptOutlineMessages(req),
			"temperature": 0.2,
			"max_tokens":  maxPPTOutlineTokens(req.SlideCount),
		},
	})
	if err != nil {
		if shouldFallbackPPTOutline(err) {
			log.Printf("ppt outline provider unavailable model=%s fallback=local error=%v", model, err)
			return buildPPTOutline(req), nil
		}
		return pptOutline{}, fmt.Errorf("ppt outline model call failed: %w", err)
	}
	outline, err := parsePPTOutlineModelOutput(response.Message.Content, req)
	if err != nil {
		return pptOutline{}, fmt.Errorf("parse ppt outline model output: %w", err)
	}
	return outline, nil
}

func shouldFallbackPPTOutline(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"http 429", "http 502", "http 503", "http 504",
		"system_memory_overloaded", "system memory overloaded",
		"context deadline exceeded", "client.timeout", "connection reset",
		"connection refused", "temporary failure", "no such host",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func (a api) preparePPTCapabilityRequest(data adminPlatformData, user adminUser, prompt string, model string, pageCount int, withImages bool, uploadedFile bool) (generation.CreateRequest, error) {
	return a.preparePPTCapabilityRequestWithAuthorization(data, user, prompt, model, pageCount, withImages, uploadedFile, nil)
}

func (a api) preparePPTCapabilityRequestWithAuthorization(data adminPlatformData, user adminUser, prompt string, model string, pageCount int, withImages bool, uploadedFile bool, authorizationOverride *modelCallAuthorization) (generation.CreateRequest, error) {
	if pageCount <= 0 {
		pageCount = 5
	}
	authorization := modelCallAuthorization{}
	if authorizationOverride != nil {
		authorization = *authorizationOverride
	} else {
		authorization = modelCallAuthorization{
			ContextType: contextPersonal, TenantID: "tenant_default", OrganizationID: defaultOrganizationID("tenant_default"),
			UserID: user.ID, Role: roleUser, BillingScope: contextPersonal, BillingAccountID: user.ID, ServiceState: "ACTIVE",
		}
		if authorizer, ok := a.store.(modelCallAuthorizer); ok {
			resolvedAuthorization, err := authorizer.AuthorizeModelCall(user.ID, modulePPTGeneration)
			if err != nil {
				return generation.CreateRequest{}, err
			}
			authorization = resolvedAuthorization
		}
	}
	user.TenantID = authorization.TenantID
	user.OrganizationID = authorization.OrganizationID
	request := generation.CreateRequest{
		Type:       "PPT_GENERATION",
		ModuleCode: modulePPTGeneration,
		UserID:     user.ID,
		Prompt:     strings.TrimSpace(prompt),
		Model:      strings.TrimSpace(model),
		Params: map[string]any{
			"topic": prompt, "page_count": pageCount, "with_images": withImages, "uploaded_file": uploadedFile,
		},
	}
	resolved, err := resolveModuleSchema(data, user, modulePPTGeneration, request.Model)
	if err != nil {
		return generation.CreateRequest{}, err
	}
	request.Model = resolved.Model.ModelName
	if err := validateGenerationParams(request, resolved); err != nil {
		return generation.CreateRequest{}, err
	}
	request.Params["module_code"] = modulePPTGeneration
	request.Params["model_name"] = request.Model
	request.Params["package_id"] = user.PlanID
	request.Params["tenant_id"] = authorization.TenantID
	request.Params["organization_id"] = authorization.OrganizationID
	request.Params["billing_scope"] = authorization.BillingScope
	request.Params["billing_account_id"] = authorization.BillingAccountID
	request.Params["final_schema_snapshot"] = map[string]any{"fields": resolved.FinalSchema.Fields}
	request.Params["limit_snapshot"] = resolved.Limit.LimitJSON
	return request, nil
}

func anyFloatOrDefault(value any, fallback float64) float64 {
	if parsed, ok := anyToFloat(value); ok {
		return parsed
	}
	return fallback
}

func (a api) pptProviderConfigured() bool {
	return strings.TrimSpace(a.cfg.PPTProviderURL) != "" && strings.TrimSpace(a.cfg.PPTProviderAPIKey) != ""
}

func (a api) pptTextModelOptions() []pptModelOption {
	models := append([]pptModelOption{}, pptTextModels...)
	configured := strings.TrimSpace(a.cfg.PPTTextModel)
	if configured == "" || pptModelOptionExists(models, configured) {
		return models
	}
	return append([]pptModelOption{{
		Label:        configured,
		Value:        configured,
		Provider:     "Configured",
		ProviderType: "openai",
		Group:        "当前后台配置",
		Description:  "后台环境变量 PPT_TEXT_MODEL 当前配置的文本模型",
	}}, models...)
}

func pptModelOptionExists(models []pptModelOption, value string) bool {
	for _, item := range models {
		if strings.EqualFold(strings.TrimSpace(item.Value), strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}

func outlineRequestFromGenerate(req pptapp.GenerateRequest) pptOutlineGenerateRequest {
	return pptOutlineGenerateRequest{
		Prompt:                req.Prompt,
		SlideCount:            req.SlideCount,
		Language:              req.Language,
		Tone:                  req.Tone,
		TextContent:           req.TextContent,
		Audience:              req.Audience,
		Scenario:              req.Scenario,
		GenerationAspectRatio: req.GenerationAspectRatio,
		AutoThemeEnabled:      req.AutoThemeEnabled,
		EnableWebSearch:       req.EnableWebSearch,
		TextModel:             req.TextModel,
		ImageSource:           req.ImageSource,
		ImageModel:            normalizedPPTImageModel(req.ImageModel),
	}
}

func pptOutlineMessages(req pptOutlineGenerateRequest) []any {
	return []any{
		map[string]any{
			"role": "system",
			"content": strings.Join([]string{
				"You generate presentation outlines for a PPT document generation product.",
				"Return only valid JSON. Do not wrap it in Markdown.",
				"The JSON shape is: {\"title\":\"...\",\"slides\":[{\"page\":1,\"title\":\"...\",\"summary\":\"...\",\"bulletPoints\":[\"...\"],\"layout\":\"cover|section|content|imageText|summary\",\"slideType\":\"cover|section|statement|text_image|case_study|product_showcase|industry_scene|agenda|feature_grid|process|timeline|comparison|data_chart|swot|matrix|organization|table\"}]}",
				"Use the exact field names title, slides, page, summary, bulletPoints, and layout.",
				"Use Simplified Chinese when language is zh. Use English when language is en.",
				"Every slide must have 2 to 4 concise bulletPoints.",
			}, "\n"),
		},
		map[string]any{
			"role":    "user",
			"content": pptOutlinePrompt(req),
		},
	}
}

func pptOutlinePrompt(req pptOutlineGenerateRequest) string {
	slideCount := req.SlideCount
	if slideCount <= 0 {
		slideCount = 5
	}
	return fmt.Sprintf(
		"Topic: %s\nSlides: %d\nLanguage: %s\nTone: %s\nContent density: %s\nAudience: %s\nScenario: %s\nAspect ratio: %s\nAuto theme: %t\nWeb search requested: %t\nImage source: %s\nImage model: %s\nImage instruction: %s\n\nCreate a practical, presentation-ready outline. Page 1 should be a cover. The last page should be a summary or action page.",
		req.Prompt,
		slideCount,
		firstNonEmptyString(req.Language, "zh"),
		firstNonEmptyString(req.Tone, "professional"),
		firstNonEmptyString(req.TextContent, "concise"),
		firstNonEmptyString(req.Audience, "auto"),
		firstNonEmptyString(req.Scenario, "auto"),
		firstNonEmptyString(req.GenerationAspectRatio, "dynamic"),
		req.AutoThemeEnabled,
		req.EnableWebSearch,
		normalizedPPTImageSource(req.ImageSource),
		normalizedPPTImageModel(req.ImageModel),
		pptOutlineImageInstruction(req),
	)
}

func pptOutlineImageInstruction(req pptOutlineGenerateRequest) string {
	switch normalizedPPTImageSource(req.ImageSource) {
	case "none":
		return "Prefer content, section, and summary layouts. Avoid imageText layouts unless the user explicitly asks for visual comparison."
	case "stock":
		if strings.EqualFold(strings.TrimSpace(req.ImageModel), "gif") {
			return "Use concise, visual-friendly pages that can later pair with GIF or motion assets. Use imageText only where motion helps the story."
		}
		return "Use imageText layouts for pages that benefit from图库/联网配图; keep summaries visual-friendly and concrete."
	default:
		return "Use imageText layouts for pages that benefit from AI-generated visuals; keep visual pages concrete enough for later image generation."
	}
}

func normalizedPPTImageSource(value string) string {
	switch strings.TrimSpace(value) {
	case "stock", "none":
		return strings.TrimSpace(value)
	default:
		return "ai"
	}
}

func normalizedPPTImageModel(value string) string {
	model := strings.TrimSpace(value)
	switch strings.ToLower(model) {
	case "":
		return "default-image"
	case "gpt-image":
		return "gpt-image-2"
	default:
		return model
	}
}

func maxPPTOutlineTokens(slideCount int) int {
	if slideCount <= 0 {
		slideCount = 5
	}
	tokens := 1200 + slideCount*420
	if tokens < 2400 {
		return 2400
	}
	if tokens > 6400 {
		return 6400
	}
	return tokens
}

type pptOutlineModelPayload struct {
	Title     string                 `json:"title"`
	Slides    []pptOutlineSlideModel `json:"slides"`
	UpdatedAt string                 `json:"updatedAt"`
}

type pptOutlineSlideModel struct {
	Page              int            `json:"page"`
	Title             string         `json:"title"`
	Summary           string         `json:"summary"`
	Description       string         `json:"description"`
	Content           string         `json:"content"`
	BulletPoints      pptStringItems `json:"bulletPoints"`
	BulletPointsSnake pptStringItems `json:"bullet_points"`
	Bullets           pptStringItems `json:"bullets"`
	Points            pptStringItems `json:"points"`
	KeyPoints         pptStringItems `json:"keyPoints"`
	KeyPointsSnake    pptStringItems `json:"key_points"`
	Takeaways         pptStringItems `json:"takeaways"`
	Layout            string         `json:"layout"`
	SlideType         string         `json:"slideType"`
}

type pptStringItems []string

func (items *pptStringItems) UnmarshalJSON(raw []byte) error {
	var stringsValue []string
	if err := json.Unmarshal(raw, &stringsValue); err == nil {
		*items = nonEmptyPPTStrings(stringsValue)
		return nil
	}
	var anyValue []any
	if err := json.Unmarshal(raw, &anyValue); err == nil {
		result := make([]string, 0, len(anyValue))
		for _, item := range anyValue {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" && text != "<nil>" {
				result = append(result, text)
			}
		}
		*items = result
		return nil
	}
	var textValue string
	if err := json.Unmarshal(raw, &textValue); err == nil {
		textValue = strings.TrimSpace(textValue)
		if textValue != "" {
			*items = []string{textValue}
		}
		return nil
	}
	return nil
}

func parsePPTOutlineModelOutput(content string, req pptOutlineGenerateRequest) (pptOutline, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return pptOutline{}, errors.New("model output is empty")
	}
	var payload pptOutlineModelPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		extracted := extractJSONObject(content)
		if extracted == "" {
			return pptOutline{}, err
		}
		if parseErr := json.Unmarshal([]byte(extracted), &payload); parseErr != nil {
			return pptOutline{}, parseErr
		}
	}
	outline := payload.toOutline()
	outline = normalizeModelOutline(outline, req)
	if outline.Title == "" || len(outline.Slides) == 0 {
		return pptOutline{}, errors.New("model output does not contain a usable outline")
	}
	return outline, nil
}

func (payload pptOutlineModelPayload) toOutline() pptOutline {
	slides := make([]pptOutlineSlide, 0, len(payload.Slides))
	for _, item := range payload.Slides {
		slides = append(slides, pptOutlineSlide{
			Page:         item.Page,
			Title:        item.Title,
			Summary:      firstNonEmptyString(item.Summary, item.Description, item.Content),
			BulletPoints: firstPPTStringItems(item.BulletPoints, item.BulletPointsSnake, item.Bullets, item.Points, item.KeyPoints, item.KeyPointsSnake, item.Takeaways),
			Layout:       item.Layout,
			SlideType:    item.SlideType,
		})
	}
	return pptOutline{
		Title:     payload.Title,
		Slides:    slides,
		UpdatedAt: payload.UpdatedAt,
	}
}

func firstPPTStringItems(values ...pptStringItems) []string {
	for _, items := range values {
		normalized := nonEmptyPPTStrings(items)
		if len(normalized) > 0 {
			return normalized
		}
	}
	return nil
}

func nonEmptyPPTStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func extractJSONObject(content string) string {
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return ""
	}
	return content[start : end+1]
}

func normalizeModelOutline(outline pptOutline, req pptOutlineGenerateRequest) pptOutline {
	outline.Title = strings.TrimSpace(outline.Title)
	if outline.Title == "" {
		outline.Title = titleFromPromptForOutline(req.Prompt)
	}
	if outline.UpdatedAt == "" {
		outline.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	slides := make([]pptOutlineSlide, 0, len(outline.Slides))
	for i, slide := range outline.Slides {
		slide.Page = i + 1
		slide.Title = strings.TrimSpace(slide.Title)
		if slide.Title == "" {
			slide.Title = fmt.Sprintf("%s %d", outline.Title, i+1)
		}
		slide.Summary = strings.TrimSpace(slide.Summary)
		points := make([]string, 0, len(slide.BulletPoints))
		for _, point := range slide.BulletPoints {
			point = strings.TrimSpace(point)
			if point != "" {
				points = append(points, point)
			}
		}
		if len(points) == 0 && slide.Summary != "" {
			points = append(points, slide.Summary)
		}
		slide.BulletPoints = points
		slide.Layout = normalizedPPTLayout(slide.Layout, i, len(outline.Slides), req.ImageSource)
		if strings.TrimSpace(slide.SlideType) == "" {
			if i == 0 {
				slide.SlideType = "cover"
			} else if slide.Layout == "section" {
				slide.SlideType = "section"
			} else {
				slide.SlideType = "text_image"
			}
		}
		slide.SlideType = pptapp.NormalizeSlideType(slide.SlideType)
		slides = append(slides, slide)
	}
	outline.Slides = slides
	return outline
}

func normalizedPPTLayout(layout string, index int, total int, imageSource string) string {
	switch strings.TrimSpace(layout) {
	case "cover", "section", "content", "imageText", "summary":
		return strings.TrimSpace(layout)
	}
	if index == 0 {
		return "cover"
	}
	if total > 1 && index == total-1 {
		return "summary"
	}
	if normalizedPPTImageSource(imageSource) == "none" {
		return "content"
	}
	return "imageText"
}

func titleFromPromptForOutline(prompt string) string {
	title := strings.Join(strings.Fields(prompt), " ")
	if len([]rune(title)) <= 42 {
		return title
	}
	return string([]rune(title)[:42])
}

func writePPTError(w http.ResponseWriter, err error) {
	if errors.Is(err, pptapp.ErrTaskNotFound) || errors.Is(err, pptapp.ErrVisualNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeError(w, http.StatusBadRequest, err)
}
