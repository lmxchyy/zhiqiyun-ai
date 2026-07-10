package embedding

import (
	"fmt"
	"strings"
	"time"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type Profile struct {
	ProviderKey string
	BaseURL     string
	APIKey      string
	Model       string
	Dimension   int
	TimeoutMS   int
}

type Registry struct {
	items map[string]knowledgeapp.Embedder
}

func NewRegistry(items ...knowledgeapp.Embedder) Registry {
	registry := Registry{items: map[string]knowledgeapp.Embedder{}}
	for _, item := range items {
		if item != nil {
			registry.items[strings.ToLower(item.Code())] = item
		}
	}
	return registry
}

func (r Registry) Get(code string) (knowledgeapp.Embedder, bool) {
	item, ok := r.items[strings.ToLower(strings.TrimSpace(code))]
	return item, ok
}

func NewFromProfile(profile Profile) (knowledgeapp.Embedder, error) {
	provider := strings.ToLower(strings.TrimSpace(profile.ProviderKey))
	if provider == "" || provider == "deterministic" {
		return NewDeterministic(profile.Dimension), nil
	}
	if strings.TrimSpace(profile.Model) == "" {
		return nil, fmt.Errorf("embedding model is required for provider %s", provider)
	}
	if provider == "gemini" || provider == "google" {
		return NewGemini(GeminiOptions{BaseURL: profile.BaseURL, APIKey: profile.APIKey, Model: profile.Model, Dimension: profile.Dimension, TimeoutMS: profile.TimeoutMS}), nil
	}
	supported := map[string]bool{
		"openai": true, "qwen": true, "dashscope": true, "bge": true, "bce": true, "jina": true,
		"siliconflow": true, "oneapi": true, "one-api": true, "newapi": true, "new-api": true, "openai-compatible": true,
	}
	if !supported[provider] {
		return nil, fmt.Errorf("unsupported embedding provider %q", provider)
	}
	return NewOpenAICompatible(OpenAICompatibleOptions{
		Code: provider, BaseURL: profile.BaseURL, APIKey: profile.APIKey, Model: profile.Model,
		Dimension: profile.Dimension, Timeout: time.Duration(profile.TimeoutMS) * time.Millisecond,
	}), nil
}
