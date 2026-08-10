package image

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
)

const maxCloudBaseFunctionResponseBytes = 2 << 20

var cloudBaseOfficialImageModels = map[string]bool{
	"HY-Image-3.0-Plus-4090-Tob-v1.0": true,
	"HY-Image-v3.0-I2I-ToB-v1.0.1":    true,
}

type CloudBaseFunctionOptions struct {
	Code          string
	FunctionURL   string
	APIKey        string
	DefaultModel  string
	Models        []string
	WatermarkText string
	TimeoutMS     int
}

type CloudBaseFunction struct {
	code          string
	functionURL   string
	apiKey        string
	defaultModel  string
	models        []string
	watermarkText string
	client        *http.Client
}

func NewCloudBaseFunction(opts CloudBaseFunctionOptions) CloudBaseFunction {
	timeout := opts.TimeoutMS
	if timeout <= 0 {
		timeout = 150000
	}
	models := officialCloudBaseModels(opts.Models)
	defaultModel := strings.TrimSpace(opts.DefaultModel)
	if !cloudBaseOfficialImageModels[defaultModel] {
		defaultModel = "HY-Image-3.0-Plus-4090-Tob-v1.0"
	}
	if !containsExact(models, defaultModel) {
		models = append(models, defaultModel)
	}
	watermark := strings.TrimSpace(opts.WatermarkText)
	if watermark == "" {
		watermark = "AI生成"
	}
	if len([]rune(watermark)) > 16 {
		watermark = string([]rune(watermark)[:16])
	}
	return CloudBaseFunction{
		code:          firstCloudBaseValue(opts.Code, "cloudbase-function"),
		functionURL:   strings.TrimSpace(opts.FunctionURL),
		apiKey:        strings.TrimSpace(opts.APIKey),
		defaultModel:  defaultModel,
		models:        models,
		watermarkText: watermark,
		client:        &http.Client{Timeout: time.Duration(timeout) * time.Millisecond},
	}
}

func (p CloudBaseFunction) Code() string { return p.code }

func (p CloudBaseFunction) DefaultModel() string { return p.defaultModel }

func (p CloudBaseFunction) Supports(req generation.CreateRequest) bool {
	if p.functionURL == "" || p.apiKey == "" {
		return false
	}
	model := strings.TrimSpace(req.Model)
	return model == "" || containsExact(p.models, model)
}

func (p CloudBaseFunction) Generate(ctx context.Context, req generation.CreateRequest) ([]generation.GeneratedImage, error) {
	if !p.Supports(req) {
		return nil, ErrNoProvider
	}
	if _, err := validCloudBaseFunctionURL(p.functionURL); err != nil {
		return nil, err
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.defaultModel
	}
	if !cloudBaseOfficialImageModels[model] {
		return nil, fmt.Errorf("cloudbase image model is not in the current official allowlist: %s", model)
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return nil, generation.ErrInvalidPrompt
	}
	if len([]rune(prompt)) > 500 {
		return nil, errors.New("cloudbase image prompt exceeds the official 500 character limit")
	}
	size, err := cloudBaseImageSize(req.Params)
	if err != nil {
		return nil, err
	}
	references := cloudBaseReferenceURLs(req.Params)
	imageToImage := model == "HY-Image-v3.0-I2I-ToB-v1.0.1"
	if imageToImage && len(references) != 1 {
		return nil, errors.New("cloudbase image-to-image requires exactly one HTTPS reference image")
	}
	if !imageToImage {
		references = nil
	}
	payload := map[string]any{
		"requestId":       strings.TrimSpace(req.ClientRequestID),
		"type":            strings.TrimSpace(req.Type),
		"model":           model,
		"prompt":          prompt,
		"size":            size,
		"revise":          true,
		"footnote":        p.watermarkText,
		"referenceImages": references,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.functionURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	res, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("cloudbase function request failed: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxCloudBaseFunctionResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read cloudbase function response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("cloudbase function returned %d: %s", res.StatusCode, safeCloudBaseError(raw))
	}
	result, err := decodeCloudBaseFunctionResult(raw)
	if err != nil {
		return nil, err
	}
	return []generation.GeneratedImage{{
		URL:            result.URL,
		ContentType:    firstCloudBaseValue(result.ContentType, "image/jpeg"),
		Source:         "cloudbase",
		ProviderTaskID: result.ProviderTaskID,
		RevisedPrompt:  result.RevisedPrompt,
		ProviderMetadata: map[string]any{
			"provider": "cloudbase", "model": model, "request_id": result.RequestID,
			"watermark": p.watermarkText, "temporary_url": true,
		},
	}}, nil
}

type cloudBaseFunctionResult struct {
	URL            string
	ContentType    string
	ProviderTaskID string
	RequestID      string
	RevisedPrompt  string
}

func decodeCloudBaseFunctionResult(raw []byte) (cloudBaseFunctionResult, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return cloudBaseFunctionResult{}, errors.New("cloudbase function returned invalid JSON")
	}
	payload = unwrapCloudBaseResult(payload)
	if message := stringFromCloudBase(payload, "error", "message"); message != "" && stringFromCloudBase(payload, "url") == "" {
		return cloudBaseFunctionResult{}, fmt.Errorf("cloudbase function failed: %s", truncateCloudBaseText(message, 240))
	}
	result := cloudBaseFunctionResult{
		URL:            stringFromCloudBase(payload, "url", "imageUrl", "image_url"),
		ContentType:    stringFromCloudBase(payload, "contentType", "content_type"),
		ProviderTaskID: stringFromCloudBase(payload, "providerTaskId", "provider_task_id", "id"),
		RequestID:      stringFromCloudBase(payload, "requestId", "request_id"),
		RevisedPrompt:  stringFromCloudBase(payload, "revisedPrompt", "revised_prompt"),
	}
	if result.URL == "" {
		if data, ok := payload["data"].([]any); ok && len(data) > 0 {
			if item, ok := data[0].(map[string]any); ok {
				result.URL = stringFromCloudBase(item, "url", "imageUrl", "image_url")
				result.RevisedPrompt = firstCloudBaseValue(result.RevisedPrompt, stringFromCloudBase(item, "revisedPrompt", "revised_prompt"))
			}
		}
	}
	parsed, err := url.Parse(result.URL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" {
		return cloudBaseFunctionResult{}, errors.New("cloudbase function returned no valid HTTPS image URL")
	}
	return result, nil
}

func unwrapCloudBaseResult(payload map[string]any) map[string]any {
	raw, ok := payload["result"]
	if !ok {
		return payload
	}
	if nested, ok := raw.(map[string]any); ok {
		for _, key := range []string{"requestId", "request_id"} {
			if _, exists := nested[key]; !exists && payload[key] != nil {
				nested[key] = payload[key]
			}
		}
		return nested
	}
	if text, ok := raw.(string); ok {
		var nested map[string]any
		if json.Unmarshal([]byte(text), &nested) == nil {
			return nested
		}
	}
	return payload
}

func cloudBaseReferenceURLs(params map[string]any) []string {
	if params == nil {
		return nil
	}
	urls := []string{}
	appendURL := func(value string) {
		parsed, err := url.Parse(strings.TrimSpace(value))
		if err == nil && strings.EqualFold(parsed.Scheme, "https") && parsed.Host != "" && !containsExact(urls, parsed.String()) {
			urls = append(urls, parsed.String())
		}
	}
	if raw, ok := params["referenceImages"].([]any); ok {
		for _, value := range raw {
			if item, ok := value.(map[string]any); ok {
				appendURL(fmt.Sprint(item["url"]))
			}
		}
	}
	for _, key := range []string{"reference_image", "image_url", "imageUrl", "inputImageUrl"} {
		if value := strings.TrimSpace(fmt.Sprint(params[key])); value != "" && value != "<nil>" {
			appendURL(value)
		}
	}
	if len(urls) > 1 {
		return urls[:1]
	}
	return urls
}

func cloudBaseImageSize(params map[string]any) (string, error) {
	value := strings.ToLower(strings.TrimSpace(fmt.Sprint(params["size"])))
	switch value {
	case "1024x1024", "1280x1280", "1280x720", "720x1280":
		return value, nil
	default:
		return "", fmt.Errorf("unsupported CloudBase image size %q; supported sizes: 1024x1024, 1280x1280, 1280x720, 720x1280", value)
	}
}

func officialCloudBaseModels(values []string) []string {
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if cloudBaseOfficialImageModels[value] && !containsExact(result, value) {
			result = append(result, value)
		}
	}
	return result
}

func validCloudBaseFunctionURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" {
		return nil, errors.New("cloudbase function URL must be HTTPS")
	}
	if !strings.HasSuffix(strings.ToLower(parsed.Hostname()), ".api.tcloudbasegateway.com") && strings.TrimSpace(strings.ToLower(parsed.Hostname())) != "localhost" {
		return nil, errors.New("cloudbase function URL must use the official tcloudbasegateway.com domain")
	}
	return parsed, nil
}

func safeCloudBaseError(raw []byte) string {
	var payload map[string]any
	if json.Unmarshal(raw, &payload) == nil {
		if message := stringFromCloudBase(payload, "message", "error", "code"); message != "" {
			return truncateCloudBaseText(message, 240)
		}
	}
	return "upstream request failed"
}

func stringFromCloudBase(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key]; ok && value != nil {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" && text != "<nil>" && !strings.HasPrefix(text, "map[") {
				return text
			}
		}
	}
	return ""
}

func containsExact(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func firstCloudBaseValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func truncateCloudBaseText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}
