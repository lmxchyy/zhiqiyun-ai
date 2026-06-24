package httpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"xianzhi-ai/backend-go/internal/app/generation"

	"xianzhi-ai/backend-go/internal/config"
	imageprovider "xianzhi-ai/backend-go/internal/provider/image"
)

type platformStore interface {
	ListGenerationTasks() ([]generationTask, error)
	CreateGenerationTask(createGenerationTaskRequest) (generationTask, error)
	ListAssets() ([]asset, error)
	UserAIState(string) (userAIState, error)
	UpdateUserAIState(string, userAIState) (userAIState, error)
	UpdateAssetThumbnails(map[string]string) (int, error)
	UpdateAssetImageInfo(map[string]assetImageInfo) (int, error)
	DeleteAsset(id string) error
	PointAccount() (pointAccount, error)
	AdminData() (adminPlatformData, error)
	UpdateUserPassword(string, string) (adminUser, error)
	CreateAdminCustomer(adminCustomerMutation) (adminUser, error)
	UpdateAdminCustomer(string, adminCustomerMutation) (adminUser, error)
	CreateAdminChannelAgent(adminChannelCreateMutation) (adminChannelAgent, adminUser, error)
	UpdateAdminChannelAgent(string, adminChannelMutation) (adminChannelAgent, error)
	UpdateAdminProduct(string, adminProductMutation) (adminProduct, error)
	UpdateAdminPlan(string, adminPlanMutation) (adminPlan, error)
	CreateAdminOrder(adminOrderMutation) (adminOrder, error)
	MarkAdminOrderPaid(string) (adminOrder, error)
	RenewAdminOrder(string) (adminOrder, error)
	UpdateAdminDeliveryProject(string, adminDeliveryMutation) (map[string]any, error)
	UpdateAdminSystemSettings(adminSystemMutation) (adminSystemSettings, error)
	CreateAdminAPIChannel(adminAPIChannelMutation) (adminAPIChannel, error)
	UpdateAdminAPIChannel(string, adminAPIChannelMutation) (adminAPIChannel, error)
	TestAdminAPIChannel(string, adminAPIChannelTestRequest) (map[string]any, error)
	UpdateAdminAPIModel(string, adminAPIModelMutation) (adminAPIModel, error)
	CreateAdminAPIKey(adminAPIKeyMutation) (adminAPIKey, error)
	UpdateAdminAPIKey(string, adminAPIKeyMutation) (adminAPIKey, error)
	UpdateAdminCustomerGroup(string, adminCustomerGroupMutation) (adminCustomerGroup, error)
	CreateAdminCommission(adminCommissionMutation) (adminCommission, error)
	CreateAdminWithdrawal(adminWithdrawalMutation) (adminWithdrawal, error)
	ReviewAdminWithdrawal(string, string) (adminWithdrawal, error)
}

type api struct {
	store             platformStore
	generationService generation.Service
	cfg               config.Config
}

type generatedImageDecorator struct{}

func (generatedImageDecorator) Decorate(ctx context.Context, images []generation.GeneratedImage) []generation.GeneratedImage {
	for i := range images {
		images[i].ThumbnailURL = thumbnailForImage(ctx, images[i].URL)
		if width, height, ok := imageDimensionsForImage(ctx, images[i].URL); ok {
			images[i].Width = width
			images[i].Height = height
		}
	}
	return images
}

func newAPI(store platformStore, cfg config.Config) api {
	provider := imageprovider.NewDefaultRouter(cfg)
	service := generation.NewService(provider, generatedImageDecorator{}, func(req generation.CreateRequest) (any, error) {
		return store.CreateGenerationTask(req)
	})
	return api{store: store, generationService: service, cfg: cfg}
}

func (a api) listGenerationTasks(w http.ResponseWriter, _ *http.Request) {
	tasks, err := a.store.ListGenerationTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, tasks)
}

func (a api) createGenerationTask(w http.ResponseWriter, r *http.Request) {
	var req generation.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	service := a.generationService
	if providerID := selectedGenerationProvider(req.Params); providerID != "" {
		dynamicService, err := a.generationServiceForProvider(providerID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		service = dynamicService
	} else if configuredService, ok, err := a.generationServiceForConfiguredModel(req.Model); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	} else if ok {
		service = configuredService
	}
	task, err := service.Create(r.Context(), req)
	if err != nil {
		if errors.Is(err, generation.ErrInvalidPrompt) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, task)
}

func selectedGenerationProvider(params map[string]any) string {
	if params == nil {
		return ""
	}
	raw, ok := params["provider"]
	if !ok || raw == nil {
		return ""
	}
	provider, ok := raw.(string)
	if !ok {
		return ""
	}
	provider = strings.TrimSpace(provider)
	if provider == "" || provider == "channel_runtime_env" {
		return ""
	}
	return provider
}

func (a api) generationServiceForConfiguredModel(model string) (generation.Service, bool, error) {
	if strings.EqualFold(strings.TrimSpace(model), "mock-standard") {
		return generation.Service{}, false, nil
	}
	data, err := a.store.AdminData()
	if err != nil {
		return generation.Service{}, false, err
	}
	channel, ok := selectAPIChannelForModel(configuredGenerationChannels(data), model)
	if !ok {
		return generation.Service{}, false, fmt.Errorf("请先在主控 SaaS 的 API 配置中启用支持模型 %s 的上游渠道", firstNonEmpty([]string{model, "gpt-image-2"}))
	}
	service, err := a.generationServiceForChannel(data, channel)
	if err != nil {
		return generation.Service{}, false, err
	}
	return service, true, nil
}

func (a api) generationServiceForProvider(providerID string) (generation.Service, error) {
	data, err := a.store.AdminData()
	if err != nil {
		return generation.Service{}, err
	}
	var channel adminAPIChannel
	for _, item := range data.APIChannels {
		if item.ID == providerID {
			channel = item
			break
		}
	}
	if channel.ID == "" {
		return generation.Service{}, fmt.Errorf("api provider not found: %s", providerID)
	}
	return a.generationServiceForChannel(data, channel)
}

func (a api) generationServiceForChannel(data adminPlatformData, channel adminAPIChannel) (generation.Service, error) {
	if !strings.EqualFold(channel.Status, "ACTIVE") && !strings.EqualFold(channel.Status, "CONFIGURABLE") && !strings.EqualFold(channel.Status, "ENABLED") {
		return generation.Service{}, fmt.Errorf("api provider is not enabled: %s", channel.Name)
	}
	apiKeyEnv := strings.TrimSpace(channel.APIKeyEnv)
	apiKey := ""
	if apiKeyEnv != "" {
		apiKey = strings.TrimSpace(os.Getenv(apiKeyEnv))
	}
	if apiKey == "" {
		apiKey = savedAPIKeyForChannel(data.APIKeys, channel)
	}
	if apiKey == "" {
		if apiKeyEnv != "" {
			return generation.Service{}, fmt.Errorf("api provider %s requires saved API Key or environment variable %s", channel.Name, apiKeyEnv)
		}
		return generation.Service{}, fmt.Errorf("api provider %s requires saved API Key", channel.Name)
	}
	model := firstNonEmpty(channel.Models)
	provider := imageprovider.NewOpenAICompatibleWithOptions(imageprovider.OpenAICompatibleOptions{
		Code:       channel.ID,
		BaseURL:    channel.BaseURL,
		APIKey:     apiKey,
		ImageModel: model,
		Models:     channel.Models,
		TimeoutMS:  intConfigValue(a.cfg.ModelTimeoutMS),
	})
	return generation.NewService(provider, generatedImageDecorator{}, func(req generation.CreateRequest) (any, error) {
		if req.Params == nil {
			req.Params = map[string]any{}
		}
		req.Params["provider"] = channel.ID
		req.Params["providerName"] = channel.Name
		return a.store.CreateGenerationTask(req)
	}), nil
}

func selectAPIChannelForModel(channels []adminAPIChannel, model string) (adminAPIChannel, bool) {
	model = strings.TrimSpace(model)
	var fallback adminAPIChannel
	hasFallback := false
	for _, channel := range channels {
		if !apiChannelUsableForGeneration(channel) {
			continue
		}
		if !hasFallback || channel.Primary || priorityLess(channel, fallback) {
			fallback = channel
			hasFallback = true
		}
		if model != "" && apiChannelSupportsModel(channel, model) {
			return channel, true
		}
	}
	if model == "" && hasFallback {
		return fallback, true
	}
	return adminAPIChannel{}, false
}

func configuredGenerationChannels(data adminPlatformData) []adminAPIChannel {
	channels := annotateAPIChannelsWithKeys(data.APIChannels, data.APIKeys)
	defaultModels := configuredImageModels(data)
	result := make([]adminAPIChannel, 0, len(channels))
	for _, channel := range channels {
		if !apiChannelUsableForGeneration(channel) || !apiChannelHasCredential(data.APIKeys, channel) {
			continue
		}
		if len(nonEmptyStringItems(channel.Models...)) == 0 {
			channel.Models = defaultModels
		}
		result = append(result, channel)
	}
	return result
}

func nonEmptyStringItems(values ...string) []string {
	items := []string{}
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			items = append(items, text)
		}
	}
	return items
}

func configuredImageModels(data adminPlatformData) []string {
	models := []string{}
	seen := map[string]bool{}
	for _, item := range data.APIModels {
		capability := strings.ToUpper(strings.TrimSpace(item.Capability))
		status := strings.ToUpper(strings.TrimSpace(item.Status))
		if status != "" && status != "ACTIVE" && status != "ENABLED" {
			continue
		}
		if capability != "" && !strings.Contains(capability, "IMAGE") && !strings.Contains(capability, "TEXT_TO_IMAGE") {
			continue
		}
		model := strings.TrimSpace(item.Model)
		if model == "" || strings.EqualFold(model, "mock-standard") || seen[model] {
			continue
		}
		seen[model] = true
		models = append(models, model)
	}
	if len(models) == 0 {
		models = append(models, "gpt-image-2")
	}
	return models
}

func apiChannelHasCredential(keys []adminAPIKey, channel adminAPIChannel) bool {
	if strings.TrimSpace(channel.APIKeyEnv) != "" && strings.TrimSpace(os.Getenv(channel.APIKeyEnv)) != "" {
		return true
	}
	return strings.TrimSpace(savedAPIKeyForChannel(keys, channel)) != ""
}

func apiChannelUsableForGeneration(channel adminAPIChannel) bool {
	if channel.ID == "" || strings.EqualFold(channel.ID, "channel_runtime_env") {
		return false
	}
	return strings.EqualFold(channel.Status, "ACTIVE") || strings.EqualFold(channel.Status, "CONFIGURABLE") || strings.EqualFold(channel.Status, "ENABLED")
}

func apiChannelSupportsModel(channel adminAPIChannel, model string) bool {
	for _, item := range channel.Models {
		if strings.EqualFold(strings.TrimSpace(item), model) {
			return true
		}
	}
	return false
}

func priorityLess(left adminAPIChannel, right adminAPIChannel) bool {
	if left.Primary != right.Primary {
		return left.Primary
	}
	leftPriority := left.Priority
	if leftPriority <= 0 {
		leftPriority = 1000
	}
	rightPriority := right.Priority
	if rightPriority <= 0 {
		rightPriority = 1000
	}
	return leftPriority < rightPriority
}

func firstNonEmpty(items []string) string {
	for _, item := range items {
		if text := strings.TrimSpace(item); text != "" {
			return text
		}
	}
	return ""
}

func intConfigValue(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}

func (a api) models(w http.ResponseWriter, _ *http.Request) {
	items := []map[string]any{
		{"code": "mock-standard", "name": "本地演示模型", "capabilities": []string{"TEXT_TO_IMAGE"}, "online": true, "pointCost": 1},
	}
	seen := map[string]bool{"mock-standard": true}
	data, err := a.store.AdminData()
	if err == nil {
		for _, channel := range configuredGenerationChannels(data) {
			for _, model := range channel.Models {
				code := strings.TrimSpace(model)
				if code == "" || seen[code] {
					continue
				}
				seen[code] = true
				items = append(items, map[string]any{
					"code":         code,
					"name":         code,
					"capabilities": []string{"TEXT_TO_IMAGE", "IMAGE_TO_IMAGE"},
					"online":       true,
					"pointCost":    10,
					"providerId":   channel.ID,
					"providerName": channel.Name,
				})
			}
		}
	}
	writeJSON(w, items)
}
func (a api) listAssets(w http.ResponseWriter, _ *http.Request) {
	assets, err := a.store.ListAssets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, assets)
}

func (a api) downloadAsset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	assets, err := a.store.ListAssets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, item := range assets {
		if item.ID != id {
			continue
		}
		if item.URL == "" {
			writeError(w, http.StatusNotFound, errAssetNotFound)
			return
		}
		a.writeAssetDownload(w, r, item)
		return
	}
	writeError(w, http.StatusNotFound, errAssetNotFound)
}

func (a api) writeAssetDownload(w http.ResponseWriter, r *http.Request, item asset) {
	contentType := stringMetadataValue(item, "contentType")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if strings.HasPrefix(item.URL, "data:") {
		comma := strings.IndexByte(item.URL, ',')
		if comma < 0 {
			writeError(w, http.StatusBadRequest, errors.New("invalid data URL"))
			return
		}
		header := item.URL[:comma]
		if mediaType := strings.TrimPrefix(strings.Split(header, ";")[0], "data:"); mediaType != "" {
			contentType = mediaType
		}
		raw := []byte(item.URL[comma+1:])
		if strings.Contains(header, ";base64") {
			decoded, err := base64.StdEncoding.DecodeString(string(raw))
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			raw = decoded
		}
		writeAttachmentHeaders(w, contentType, downloadAssetName(item, contentType))
		_, _ = w.Write(raw)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, item.URL, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, fmt.Errorf("asset download returned %d", res.StatusCode))
		return
	}
	if res.Header.Get("Content-Type") != "" {
		contentType = res.Header.Get("Content-Type")
	}
	writeAttachmentHeaders(w, contentType, downloadAssetName(item, contentType))
	_, _ = io.Copy(w, io.LimitReader(res.Body, 50<<20))
}

func writeAttachmentHeaders(w http.ResponseWriter, contentType string, filename string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func downloadAssetName(item asset, contentType string) string {
	name := regexp.MustCompile(`[\\/:*?"<>|]+`).ReplaceAllString(item.Name, "-")
	if name == "" {
		name = item.ID
	}
	if regexp.MustCompile(`(?i)\.(png|jpe?g|webp|gif|svg)$`).MatchString(name) {
		return name
	}
	switch {
	case strings.Contains(contentType, "svg"):
		return name + ".svg"
	case strings.Contains(contentType, "jpeg"), strings.Contains(contentType, "jpg"):
		return name + ".jpg"
	case strings.Contains(contentType, "webp"):
		return name + ".webp"
	default:
		return name + ".png"
	}
}

func stringMetadataValue(item asset, key string) string {
	value, ok := item.Metadata[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func (a api) backfillAssetThumbnails(w http.ResponseWriter, r *http.Request) {
	assets, err := a.store.ListAssets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	thumbnailUpdates := map[string]string{}
	infoUpdates := map[string]assetImageInfo{}
	missing := 0
	for _, item := range assets {
		if item.MediaType != "image" || item.URL == "" {
			continue
		}
		info := assetImageInfo{}
		if item.ThumbnailURL == "" {
			missing++
			if thumbnailURL := thumbnailForImage(r.Context(), item.URL); thumbnailURL != "" {
				info.ThumbnailURL = thumbnailURL
				thumbnailUpdates[item.ID] = thumbnailURL
			}
		}
		if width, height, ok := imageDimensionsForImage(r.Context(), item.URL); ok {
			info.Width = width
			info.Height = height
		}
		if info.ThumbnailURL != "" || info.Width > 0 || info.Height > 0 {
			infoUpdates[item.ID] = info
		}
	}

	updated, err := a.store.UpdateAssetThumbnails(thumbnailUpdates)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	infoUpdated, err := a.store.UpdateAssetImageInfo(infoUpdates)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{
		"ok":          true,
		"missing":     missing,
		"updated":     updated,
		"infoUpdated": infoUpdated,
	})
}

func (a api) pointAccount(w http.ResponseWriter, _ *http.Request) {
	account, err := a.store.PointAccount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{
		"account":      account,
		"transactions": []any{},
	})
}

func (a api) userDashboard(w http.ResponseWriter, _ *http.Request) {
	tasks, err := a.store.ListGenerationTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	assets, err := a.store.ListAssets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	points, err := a.store.PointAccount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	succeeded := 0
	totalPointCost := 0
	for _, task := range tasks {
		if strings.EqualFold(task.Status, "SUCCEEDED") {
			succeeded++
		}
		totalPointCost += task.PointCost
	}
	writeJSON(w, map[string]any{
		"summary": map[string]any{
			"availablePoints":      points.Available,
			"todayGenerations":     len(tasks),
			"succeededGenerations": succeeded,
			"assets":               len(assets),
			"totalPointCost":       totalPointCost,
		},
		"metrics": []map[string]any{
			{"label": "可用点数", "value": points.Available},
			{"label": "今日生成", "value": len(tasks)},
			{"label": "作品数量", "value": len(assets)},
			{"label": "点数消耗", "value": totalPointCost},
		},
		"recentTasks":  tasks,
		"recentAssets": assets,
	})
}

func (a api) userOnlineImage(w http.ResponseWriter, _ *http.Request) {
	tasks, err := a.store.ListGenerationTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	assets, err := a.store.ListAssets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	points, err := a.store.PointAccount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	settings, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	aiState, err := a.store.UserAIState("user_000002")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	queued := 0
	running := 0
	completed := 0
	failed := 0
	totalPointCost := 0
	for _, task := range tasks {
		status := strings.ToUpper(task.Status)
		switch status {
		case "PENDING", "QUEUED":
			queued++
		case "RUNNING", "PROCESSING":
			running++
		case "SUCCEEDED", "COMPLETED":
			completed++
		case "FAILED", "ERROR":
			failed++
		}
		totalPointCost += task.PointCost
	}
	generationChannels := configuredGenerationChannels(settings)
	providers := make([]map[string]any, 0, len(generationChannels))
	for index, channel := range generationChannels {
		providers = append(providers, map[string]any{
			"id":               channel.ID,
			"name":             channel.Name,
			"baseUrl":          channel.BaseURL,
			"status":           channel.Status,
			"models":           channel.Models,
			"apiKeyConfigured": channel.APIKeyConfigured,
			"latencyMs":        120 + index*35,
			"quota":            100000 - index*12000,
		})
	}
	writeJSON(w, map[string]any{
		"summary": map[string]any{
			"availablePoints":  points.Available,
			"todayGenerations": len(tasks),
			"queueTasks":       queued + running,
			"apiPlatforms":     len(providers),
			"totalPointCost":   totalPointCost,
		},
		"metrics": []map[string]any{
			{"label": "可用点数", "value": points.Available},
			{"label": "今日生成", "value": len(tasks)},
			{"label": "队列任务", "value": queued + running},
			{"label": "可用 API 平台", "value": len(providers)},
		},
		"queue":       map[string]any{"queued": queued, "running": running, "completed": completed, "failed": failed},
		"providers":   providers,
		"models":      settings.APIModels,
		"recentTasks": tasks,
		"assets":      assets,
		"aiState":     aiState,
	})
}

func (a api) updateUserAIState(w http.ResponseWriter, r *http.Request) {
	var req userAIState
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	state, err := a.store.UpdateUserAIState("user_000002", req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, state)
}

func (a api) userAPISettings(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{
		"summary": map[string]any{
			"apiChannels":    len(data.APIChannels),
			"apiModels":      len(data.APIModels),
			"apiKeys":        len(data.APIKeys),
			"customerGroups": len(data.CustomerGroups),
		},
		"apiChannels":    data.APIChannels,
		"apiModels":      data.APIModels,
		"apiKeys":        data.APIKeys,
		"customerGroups": data.CustomerGroups,
	})
}

func (a api) userUsage(w http.ResponseWriter, _ *http.Request) {
	tasks, err := a.store.ListGenerationTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items := make([]map[string]any, 0, len(tasks))
	totalPointCost := 0
	for _, task := range tasks {
		totalPointCost += task.PointCost
		items = append(items, map[string]any{
			"id":        task.ID,
			"model":     task.Model,
			"type":      task.Type,
			"status":    task.Status,
			"pointCost": task.PointCost,
			"createdAt": task.CreatedAt,
		})
	}
	writeJSON(w, map[string]any{
		"summary": map[string]any{"records": len(items), "totalPointCost": totalPointCost},
		"items":   items,
	})
}

func (a api) deleteAsset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.store.DeleteAsset(id); err != nil {
		if errors.Is(err, errAssetNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
