package image

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
)

type Router struct {
	providers []Provider
}

func NewRouter(providers ...Provider) Router {
	items := make([]Provider, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			items = append(items, provider)
		}
	}
	return Router{providers: items}
}

func NewDefaultRouter(cfg config.Config) Router {
	providers := providersFromJSON(cfg.ModelProvidersJSON)
	if len(providers) == 0 {
		providers = append(providers, NewOpenAICompatible(cfg))
	}
	return NewRouter(providers...)
}

func (r Router) DefaultModel() string {
	for _, provider := range r.providers {
		model := strings.TrimSpace(provider.DefaultModel())
		if model != "" {
			return model
		}
	}
	return ""
}

func (r Router) Generate(ctx context.Context, req generation.CreateRequest) ([]generation.GeneratedImage, error) {
	var lastErr error
	for _, provider := range r.providers {
		if !provider.Supports(req) {
			continue
		}
		images, err := provider.Generate(ctx, req)
		if err == nil {
			return images, nil
		}
		lastErr = err
		if !isFallbackEligible(err) {
			return nil, err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrNoProvider
}

func isFallbackEligible(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || isTimeoutError(err) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "returned 429") ||
		strings.Contains(lower, "returned 403") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "too many requests") ||
		strings.Contains(lower, "insufficient_quota") ||
		strings.Contains(lower, "quota exceeded") ||
		strings.Contains(lower, "no available image quota") ||
		strings.Contains(lower, "forbidden") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "无权访问") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "no such host") ||
		strings.Contains(lower, "network is unreachable") ||
		strings.Contains(lower, "connection reset")
}

type providersJSON struct {
	Providers []providerJSON `json:"providers"`
}

type providerJSON struct {
	Code                    string   `json:"code"`
	Kind                    string   `json:"kind"`
	BaseURL                 string   `json:"baseUrl"`
	APIKey                  string   `json:"apiKey"`
	ImageModel              string   `json:"imageModel"`
	Models                  []string `json:"models"`
	ImageGenerationEndpoint string   `json:"imageGenerationEndpoint"`
	ImageEditEndpoint       string   `json:"imageEditEndpoint"`
	TimeoutMS               any      `json:"timeoutMs"`
	Status                  string   `json:"status"`
	Enabled                 *bool    `json:"enabled"`
}

func providersFromJSON(raw string) []Provider {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	configs := decodeProviderConfigs(raw)
	providers := make([]Provider, 0, len(configs))
	for _, item := range configs {
		if !providerEnabled(item) || !providerKindSupported(item.Kind) {
			continue
		}
		providers = append(providers, NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
			Code:                    item.Code,
			BaseURL:                 item.BaseURL,
			APIKey:                  item.APIKey,
			ImageModel:              item.ImageModel,
			Models:                  item.Models,
			ImageGenerationEndpoint: item.ImageGenerationEndpoint,
			ImageEditEndpoint:       item.ImageEditEndpoint,
			TimeoutMS:               intValue(item.TimeoutMS),
		}))
	}
	return providers
}

func decodeProviderConfigs(raw string) []providerJSON {
	var wrapped providersJSON
	if err := json.Unmarshal([]byte(raw), &wrapped); err == nil && len(wrapped.Providers) > 0 {
		return wrapped.Providers
	}
	var list []providerJSON
	if err := json.Unmarshal([]byte(raw), &list); err == nil {
		return list
	}
	return nil
}

func providerEnabled(item providerJSON) bool {
	if item.Enabled != nil && !*item.Enabled {
		return false
	}
	status := strings.ToUpper(strings.TrimSpace(item.Status))
	return status == "" || status == "ACTIVE" || status == "ENABLED"
}

func providerKindSupported(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "openai", "openai-compatible", "openai_compatible", "compatible":
		return true
	default:
		return false
	}
}

func intValue(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		return 0
	}
}
