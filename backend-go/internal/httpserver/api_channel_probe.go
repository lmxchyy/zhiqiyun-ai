package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func testAPIChannelConnection(item adminAPIChannel, req adminAPIChannelTestRequest) map[string]any {
	start := time.Now()
	baseURL := strings.TrimSpace(firstNonEmptyString(req.BaseURL, item.BaseURL))
	protocol := strings.TrimSpace(firstNonEmptyString(req.Protocol, item.Protocol, "openai"))
	imageRequestMode := strings.TrimSpace(firstNonEmptyString(req.ImageRequestMode, item.ImageRequestMode, "openai"))
	fetchModelsPath := strings.TrimSpace(firstNonEmptyString(req.FetchModelsPath, item.FetchModelsPath, "/models"))
	apiKeyEnv := strings.TrimSpace(item.APIKeyEnv)
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" && apiKeyEnv != "" {
		apiKey = strings.TrimSpace(os.Getenv(apiKeyEnv))
	}
	result := map[string]any{
		"id":               item.ID,
		"name":             item.Name,
		"baseUrl":          baseURL,
		"protocol":         protocol,
		"imageRequestMode": imageRequestMode,
		"fetchModelsPath":  fetchModelsPath,
		"apiKeyEnv":        apiKeyEnv,
		"apiKeyConfigured": apiKey != "",
		"checkedAt":        time.Now().UTC().Format(time.RFC3339Nano),
		"latencyMs":        int64(0),
		"status":           "ERROR",
		"statusCode":       0,
		"ok":               false,
		"message":          "地址验证未通过",
		"modelCount":       0,
		"all":              []string{},
		"imageModels":      []string{},
		"chatModels":       []string{},
		"videoModels":      []string{},
		"raw":              map[string]any{},
	}
	if req.ProbeProtocol {
		probeAPIChannelProtocol(result, baseURL, apiKey, imageRequestMode, fetchModelsPath)
		return result
	}
	modelsURL, err := joinAPIURL(baseURL, fetchModelsPath, protocol)
	if err != nil {
		result["message"] = err.Error()
		result["latencyMs"] = time.Since(start).Milliseconds()
		return result
	}
	result["modelsUrl"] = modelsURL
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		result["message"] = err.Error()
		result["latencyMs"] = time.Since(start).Milliseconds()
		return result
	}
	httpReq.Header.Set("Accept", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		result["message"] = err.Error()
		result["latencyMs"] = time.Since(start).Milliseconds()
		return result
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	result["statusCode"] = resp.StatusCode
	result["latencyMs"] = time.Since(start).Milliseconds()
	var raw any
	if len(body) > 0 && json.Unmarshal(body, &raw) == nil {
		result["raw"] = raw
	} else {
		result["raw"] = string(body)
	}
	models := extractProviderModelIDs(raw)
	imageModels, chatModels, videoModels := splitProviderModels(models)
	result["all"] = models
	result["imageModels"] = imageModels
	result["chatModels"] = chatModels
	result["videoModels"] = videoModels
	result["modelCount"] = len(models)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result["status"] = "OK"
		result["ok"] = true
		result["message"] = fmt.Sprintf("地址验证通过 · 找到 %d 个模型", len(models))
		return result
	}
	result["message"] = upstreamErrorMessage(raw, resp.Status)
	if strings.EqualFold(protocol, "openai") {
		probeAPIChannelProtocol(result, baseURL, apiKey, imageRequestMode, fetchModelsPath)
	}
	return result
}

func publicAPIKeys(items []adminAPIKey) []adminAPIKey {
	result := make([]adminAPIKey, len(items))
	copy(result, items)
	for i := range result {
		result[i].Secret = ""
	}
	return result
}

func annotateAPIChannelsWithKeys(channels []adminAPIChannel, keys []adminAPIKey) []adminAPIChannel {
	result := make([]adminAPIChannel, len(channels))
	copy(result, channels)
	for i := range result {
		if secret := savedAPIKeyForChannel(keys, result[i]); secret != "" {
			result[i].APIKeyConfigured = true
			result[i].KeyPreview = apiKeyPrefix(secret, i+1)
		}
	}
	return result
}

func apiKeyPrefix(secret string, fallbackSeed int) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "sk-" + fmtSix(fallbackSeed%1000000)
	}
	if len(secret) <= 10 {
		return secret
	}
	return secret[:7] + "..." + secret[len(secret)-4:]
}

func savedAPIKeyForChannel(keys []adminAPIKey, channel adminAPIChannel) string {
	channelName := strings.TrimSpace(strings.ToLower(channel.Name))
	channelID := strings.TrimSpace(strings.ToLower(channel.ID))
	for _, key := range keys {
		if !strings.EqualFold(key.Status, "ACTIVE") {
			continue
		}
		customer := strings.TrimSpace(strings.ToLower(key.Customer))
		if customer == "" {
			continue
		}
		if customer == channelName || customer == channelID {
			if strings.TrimSpace(key.Secret) != "" {
				return strings.TrimSpace(key.Secret)
			}
		}
	}
	for _, key := range keys {
		if !strings.EqualFold(key.Status, "ACTIVE") {
			continue
		}
		customer := strings.TrimSpace(strings.ToLower(key.Customer))
		if len(customer) < 4 || len(channelName) < 4 {
			continue
		}
		if strings.Contains(channelName, customer) || strings.Contains(customer, channelName) {
			if strings.TrimSpace(key.Secret) != "" {
				return strings.TrimSpace(key.Secret)
			}
		}
	}
	return ""
}

func savedNonPlaceholderAPIKeyForChannel(keys []adminAPIKey, channel adminAPIChannel) string {
	channelName := strings.TrimSpace(strings.ToLower(channel.Name))
	channelID := strings.TrimSpace(strings.ToLower(channel.ID))
	for _, key := range keys {
		if !strings.EqualFold(key.Status, "ACTIVE") || isPlaceholderAPIKeySecret(key.Secret) {
			continue
		}
		customer := strings.TrimSpace(strings.ToLower(key.Customer))
		if customer != "" && customer == channelName {
			return strings.TrimSpace(key.Secret)
		}
	}
	for _, key := range keys {
		if !strings.EqualFold(key.Status, "ACTIVE") || isPlaceholderAPIKeySecret(key.Secret) {
			continue
		}
		customer := strings.TrimSpace(strings.ToLower(key.Customer))
		if customer != "" && customer == channelID {
			return strings.TrimSpace(key.Secret)
		}
	}
	return ""
}

func isPlaceholderAPIKeySecret(secret string) bool {
	secret = strings.TrimSpace(secret)
	return secret == "" || strings.HasPrefix(secret, "sk-user-") || strings.EqualFold(secret, "sk-local-admin")
}

func probeAPIChannelProtocol(result map[string]any, baseURL string, apiKey string, imageRequestMode string, fetchModelsPath string) {
	start := time.Now()
	asyncURL, err := joinAPIURL(baseURL, "/tasks/healthcheck_probe_do_not_submit", "openai")
	if err != nil {
		result["message"] = err.Error()
		result["latencyMs"] = time.Since(start).Milliseconds()
		return
	}
	asyncProbe := requestProbeURL(asyncURL, apiKey)
	asyncStatus := intFromAny(asyncProbe["status"])
	asyncMessage := "/v1/tasks/ 返回 " + fmt.Sprint(asyncStatus)
	if asyncStatus == 404 {
		asyncMessage = "平台不支持 /v1/tasks/ 端点，可能不是 APIMart 异步协议"
	} else if asyncStatus == 400 && strings.Contains(strings.ToLower(fmt.Sprint(asyncProbe["raw"])), "invalid task id") {
		asyncMessage = "APIMart 异步任务端点可用，API Key 已通过认证"
		result["status"] = "OK"
		result["ok"] = true
		result["protocol"] = "apimart"
		result["statusCode"] = asyncStatus
		result["message"] = asyncMessage
		result["raw"] = map[string]any{"async_probe": mergeProbeMessage(asyncProbe, asyncMessage)}
		result["latencyMs"] = time.Since(start).Milliseconds()
		return
	} else if asyncStatus == 401 || asyncStatus == 403 {
		asyncMessage = "/v1/tasks/ 返回鉴权失败"
	}
	modelsURL, err := joinAPIURL(baseURL, fetchModelsPath, "openai")
	if err != nil {
		result["message"] = err.Error()
		result["latencyMs"] = time.Since(start).Milliseconds()
		return
	}
	openaiProbe := requestProbeURL(modelsURL, apiKey)
	openaiRaw := openaiProbe["raw"]
	models := extractProviderModelIDs(openaiRaw)
	imageModels, chatModels, videoModels := splitProviderModels(models)
	openaiStatus := intFromAny(openaiProbe["status"])
	result["statusCode"] = openaiStatus
	result["latencyMs"] = time.Since(start).Milliseconds()
	result["modelCount"] = len(models)
	result["all"] = models
	result["imageModels"] = imageModels
	result["chatModels"] = chatModels
	result["videoModels"] = videoModels
	result["imageRequestMode"] = imageRequestMode
	result["raw"] = map[string]any{
		"async_probe":  mergeProbeMessage(asyncProbe, asyncMessage),
		"openai_probe": openaiRaw,
	}
	if openaiStatus >= 200 && openaiStatus < 300 {
		result["status"] = "OK"
		result["ok"] = true
		result["protocol"] = "openai"
		result["message"] = fmt.Sprintf("OpenAI 兼容模型列表端点可用，找到 %d 个模型", len(models))
		return
	}
	result["status"] = "ERROR"
	result["ok"] = false
	result["protocol"] = "openai"
	result["message"] = upstreamErrorMessage(openaiRaw, fmt.Sprintf("HTTP %d", openaiStatus))
}

func requestProbeURL(targetURL string, apiKey string) map[string]any {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return map[string]any{"status": 0, "raw": map[string]any{"error": err.Error()}}
	}
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return map[string]any{"status": 0, "raw": map[string]any{"error": err.Error()}}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var raw any
	if len(body) > 0 && json.Unmarshal(body, &raw) == nil {
		raw = raw
	} else {
		raw = string(body)
	}
	return map[string]any{"status": resp.StatusCode, "raw": raw}
}

func mergeProbeMessage(probe map[string]any, message string) map[string]any {
	return map[string]any{
		"status":  intFromAny(probe["status"]),
		"message": message,
		"raw":     probe["raw"],
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case int32:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	default:
		return 0
	}
}

func stringSliceFromAny(value any) []string {
	result := []string{}
	switch typed := value.(type) {
	case []string:
		return append(result, typed...)
	case []any:
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				result = append(result, text)
			}
		}
	}
	return result
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func joinAPIURL(baseURL string, path string, protocol string) (string, error) {
	if strings.TrimSpace(baseURL) == "" {
		return "", fmt.Errorf("请先填写平台地址")
	}
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("平台地址格式不正确")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("平台地址只支持 http 或 https")
	}
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		trimmedPath = "/models"
	}
	parsed.Path = upstreamModelsPath(parsed.Path, trimmedPath, protocol)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func upstreamModelsPath(basePath string, requestPath string, protocol string) string {
	basePath = strings.TrimRight(basePath, "/")
	requestPath = "/" + strings.TrimLeft(strings.TrimSpace(requestPath), "/")
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if requestPath != "/models" {
		return basePath + requestPath
	}
	switch protocol {
	case "gemini":
		if strings.HasSuffix(basePath, "/v1beta") {
			return basePath + "/models"
		}
		return basePath + "/v1beta/models"
	case "volcengine":
		if strings.HasSuffix(basePath, "/api/v3") {
			return basePath + "/models"
		}
		return basePath + "/api/v3/models"
	default:
		if strings.HasSuffix(basePath, "/v1") {
			return basePath + "/models"
		}
		return basePath + "/v1/models"
	}
}

func extractProviderModelIDs(raw any) []string {
	seen := map[string]bool{}
	models := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		models = append(models, value)
	}
	var visit func(any)
	visit = func(value any) {
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				visit(item)
			}
		case map[string]any:
			for _, key := range []string{"id", "model", "name"} {
				if text, ok := typed[key].(string); ok {
					add(text)
					return
				}
			}
			for _, key := range []string{"data", "models", "items", "list", "all", "image_models", "chat_models", "video_models"} {
				if nested, ok := typed[key]; ok {
					visit(nested)
				}
			}
		case string:
			add(typed)
		}
	}
	visit(raw)
	return models
}

func splitProviderModels(models []string) ([]string, []string, []string) {
	imageModels := []string{}
	chatModels := []string{}
	videoModels := []string{}
	for _, model := range models {
		lower := strings.ToLower(model)
		switch {
		case strings.Contains(lower, "veo") || strings.Contains(lower, "sora") || strings.Contains(lower, "video") || strings.Contains(lower, "seedance"):
			videoModels = append(videoModels, model)
		case strings.Contains(lower, "image") || strings.Contains(lower, "img") || strings.Contains(lower, "flux") || strings.Contains(lower, "dall") || strings.Contains(lower, "z-image"):
			imageModels = append(imageModels, model)
		default:
			chatModels = append(chatModels, model)
		}
	}
	return imageModels, chatModels, videoModels
}

func upstreamErrorMessage(raw any, fallback string) string {
	if object, ok := raw.(map[string]any); ok {
		if errorObject, ok := object["error"].(map[string]any); ok {
			if message, ok := errorObject["message"].(string); ok && strings.TrimSpace(message) != "" {
				return message
			}
		}
		if message, ok := object["message"].(string); ok && strings.TrimSpace(message) != "" {
			return message
		}
	}
	return "地址验证未通过 (" + fallback + ")"
}
