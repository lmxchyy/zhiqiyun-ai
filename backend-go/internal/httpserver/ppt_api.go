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
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	chatprovider "xianzhi-ai/backend-go/internal/provider/chat"
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

type pptOutline = pptapp.Outline
type pptOutlineSlide = pptapp.OutlineSlide
type pptSlide = pptapp.Slide

type pptImageSearchResponse struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	Source string `json:"source"`
}

type pptImageGenerateRequest struct {
	Slide       pptSlide `json:"slide"`
	Prompt      string   `json:"prompt"`
	DeckTitle   string   `json:"deckTitle"`
	Theme       string   `json:"theme"`
	Language    string   `json:"language"`
	ImageSource string   `json:"imageSource"`
	ImageModel  string   `json:"imageModel"`
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

func (a api) createPPTGenerationTask(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
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
	if req.Outline == nil {
		outline, err := a.generatePPTOutlineWithModel(r.Context(), outlineRequestFromGenerate(req))
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		req.Outline = &outline
		req.SlideCount = len(outline.Slides)
	}
	resp, err := a.pptService.Generate(req)
	if err != nil {
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
	if shouldAutoGeneratePPTImages(req) {
		go a.runPPTTaskImageGeneration(user, task)
	}
	writeJSON(w, resp)
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
	writeJSON(w, task)
}

func (a api) listPPTHistory(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, a.pptService.History(user.ID))
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
	if _, _, err := a.authenticatedUser(r); err != nil {
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

func (a api) exportPDF(w http.ResponseWriter, r *http.Request) {
	if _, _, err := a.authenticatedUser(r); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, map[string]string{"url": ""})
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

func shouldAutoGeneratePPTImages(req pptapp.GenerateRequest) bool {
	if strings.TrimSpace(req.ImageSource) != "ai" {
		return false
	}
	model := normalizedPPTImageModel(req.ImageModel)
	if model == "" || strings.EqualFold(model, "default-image") {
		return false
	}
	return req.Outline != nil && len(req.Outline.Slides) > 0
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
		slide := slide
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			req := pptImageGenerateRequest{
				Slide:       slide,
				Prompt:      task.Prompt,
				DeckTitle:   task.Title,
				Theme:       task.Theme,
				Language:    task.Language,
				ImageSource: task.ImageSource,
				ImageModel:  task.ImageModel,
			}
			var lastErr error
			for attempt := 1; attempt <= 2; attempt++ {
				ctx, cancel := context.WithTimeout(context.Background(), 115*time.Second)
				image, err := a.generatePPTTaskSlideImage(ctx, user, task.TaskID, req)
				cancel()
				if err == nil && strings.TrimSpace(image.URL) != "" {
					if _, updateErr := a.pptService.UpdateSlideImage(user.ID, task.TaskID, slide.ID, image.URL); updateErr != nil {
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
			log.Printf("ppt image generation failed task=%s slide=%s page=%d: %v", task.TaskID, slide.ID, slide.Page, lastErr)
		}()
	}
	wg.Wait()
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
	task, err := a.store.CreatePendingGenerationTask(createReq)
	if err != nil {
		return pptImageSearchResponse{}, err
	}
	prepared, err := service.PrepareImageTask(ctx, createReq)
	if err != nil {
		_, _ = a.store.FailGenerationTask(task.ID, generationErrorMessage(err))
		return pptImageSearchResponse{}, fmt.Errorf("ppt image generation failed: %w", err)
	}
	imageURL := pptGeneratedImageURL(prepared)
	if imageURL == "" {
		_, _ = a.store.FailGenerationTask(task.ID, "ppt image provider returned no image")
		return pptImageSearchResponse{}, errors.New("ppt image provider returned no image")
	}
	if _, err := a.store.CompleteGenerationTask(task.ID, prepared); err != nil {
		_, _ = a.store.FailGenerationTask(task.ID, generationErrorMessage(err))
		return pptImageSearchResponse{}, err
	}
	return pptImageSearchResponse{
		ID:     fmt.Sprintf("ppt_ai_%d", time.Now().UnixNano()),
		URL:    imageURL,
		Title:  firstNonEmpty([]string{req.Slide.Title, req.DeckTitle, "PPT"}) + " image",
		Source: "ai",
	}, nil
}

func pptImageGenerationCreateRequest(user adminUser, req pptImageGenerateRequest, model string, pptTaskID string) generation.CreateRequest {
	return generation.CreateRequest{
		Type:       "TEXT_TO_IMAGE",
		ModuleCode: moduleImageGeneration,
		UserID:     user.ID,
		Prompt:     pptImagePrompt(req),
		Model:      model,
		Params: map[string]any{
			"module_code":  moduleImageGeneration,
			"count":        1,
			"n":            1,
			"imageRatio":   "16:9",
			"size":         "1536x1024",
			"quality":      "standard",
			"purpose":      "ppt_slide_illustration",
			"sourceModule": "ppt-generation",
			"pptTaskId":    strings.TrimSpace(pptTaskID),
			"theme":        strings.TrimSpace(req.Theme),
			"language":     strings.TrimSpace(req.Language),
			"deckTitle":    strings.TrimSpace(req.DeckTitle),
			"slideId":      strings.TrimSpace(req.Slide.ID),
			"slidePage":    req.Slide.Page,
		},
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
	title := firstNonEmpty([]string{req.Slide.Title, req.DeckTitle, req.Prompt})
	content := strings.TrimSpace(req.Slide.Content)
	bullets := make([]string, 0, len(req.Slide.BulletPoints))
	for _, point := range req.Slide.BulletPoints {
		if text := strings.TrimSpace(point); text != "" {
			bullets = append(bullets, text)
		}
	}
	details := strings.Join(nonEmptyStringItems(content, strings.Join(bullets, "; ")), "; ")
	topic := strings.TrimSpace(title)
	if prompt := strings.TrimSpace(req.Prompt); prompt != "" && prompt != topic {
		topic = strings.TrimSpace(topic + " - " + prompt)
	}
	if topic == "" {
		topic = "business presentation"
	}
	style := strings.TrimSpace(req.Theme)
	if style == "" {
		style = "modern business"
	}
	return strings.TrimSpace(fmt.Sprintf(
		"Professional 16:9 presentation illustration, %s style, clean SaaS deck visual, no readable text, no logo, no watermark. Topic: %s. Visual cues: %s.",
		truncatePPTImagePromptText(style, 40),
		truncatePPTImagePromptText(topic, 80),
		truncatePPTImagePromptText(details, 120),
	))
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
		})
	}
	return pptOutline{
		Title:     title,
		Slides:    slides,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func (a api) generatePPTOutlineWithModel(ctx context.Context, req pptOutlineGenerateRequest) (pptOutline, error) {
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		return pptOutline{}, pptapp.ErrInvalidPrompt
	}
	if !a.pptProviderConfigured() {
		return buildPPTOutline(req), nil
	}
	model := strings.TrimSpace(a.cfg.PPTTextModel)
	if model == "" {
		model = strings.TrimSpace(req.TextModel)
	}
	if model == "" {
		return buildPPTOutline(req), nil
	}
	provider := chatprovider.NewOpenAICompatible(a.cfg)
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
		return pptOutline{}, fmt.Errorf("ppt outline model call failed: %w", err)
	}
	outline, err := parsePPTOutlineModelOutput(response.Message.Content, req)
	if err != nil {
		return pptOutline{}, fmt.Errorf("parse ppt outline model output: %w", err)
	}
	return outline, nil
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
				"The JSON shape is: {\"title\":\"...\",\"slides\":[{\"page\":1,\"title\":\"...\",\"summary\":\"...\",\"bulletPoints\":[\"...\"],\"layout\":\"cover|section|content|imageText|summary\"}]}",
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
	if errors.Is(err, pptapp.ErrTaskNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeError(w, http.StatusBadRequest, err)
}
