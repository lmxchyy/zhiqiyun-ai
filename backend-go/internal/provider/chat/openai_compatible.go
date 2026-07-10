package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
)

type OpenAICompatible struct {
	code            string
	baseURL         string
	apiKey          string
	model           string
	models          []string
	disableThinking bool
	client          *http.Client
}

type OpenAICompatibleOptions struct {
	Code            string
	BaseURL         string
	APIKey          string
	Model           string
	Models          []string
	DisableThinking bool
	TimeoutMS       int
}

func NewOpenAICompatible(cfg config.Config) OpenAICompatible {
	return NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{
		Code:            "openai-compatible-chat",
		BaseURL:         cfg.PPTProviderURL,
		APIKey:          cfg.PPTProviderAPIKey,
		Model:           cfg.PPTTextModel,
		Models:          nonEmptyStrings(cfg.PPTTextModel),
		DisableThinking: cfg.PPTDisableThinking,
		TimeoutMS:       intValue(cfg.ModelTimeoutMS),
	})
}

func NewOpenAICompatibleWithOptions(opts OpenAICompatibleOptions) OpenAICompatible {
	timeoutMS := opts.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 180000
	}
	model := strings.TrimSpace(opts.Model)
	models := nonEmptyStrings(opts.Models...)
	if model != "" {
		models = appendIfMissing(models, model)
	}
	return OpenAICompatible{
		code:            strings.TrimSpace(opts.Code),
		baseURL:         strings.TrimSpace(opts.BaseURL),
		apiKey:          strings.TrimSpace(opts.APIKey),
		model:           model,
		models:          models,
		disableThinking: opts.DisableThinking,
		client:          &http.Client{Timeout: time.Duration(timeoutMS) * time.Millisecond},
	}
}

func (p OpenAICompatible) Code() string {
	if p.code != "" {
		return p.code
	}
	return "openai-compatible-chat"
}

func (p OpenAICompatible) DefaultModel() string {
	return p.model
}

func (p OpenAICompatible) Supports(req generation.CreateRequest) bool {
	if strings.TrimSpace(p.baseURL) == "" || strings.TrimSpace(p.apiKey) == "" {
		return false
	}
	if req.Type != "" && !isChatType(req.Type) {
		return false
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.model
	}
	return model == "" || len(p.models) == 0 || containsString(p.models, model)
}

func (p OpenAICompatible) Chat(ctx context.Context, req generation.CreateRequest) (Response, error) {
	if strings.TrimSpace(p.baseURL) == "" || strings.TrimSpace(p.apiKey) == "" {
		return Response{}, errors.New("chat provider requires base url and api key")
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.model
	}
	if model == "" {
		return Response{}, errors.New("chat model is required")
	}
	if len(p.models) > 0 && !containsString(p.models, model) {
		return Response{}, fmt.Errorf("chat provider %s does not support model %s", p.Code(), model)
	}
	messages := requestMessages(req)
	if len(messages) == 0 {
		messages = []Message{{Role: "user", Content: req.Prompt}}
	}
	body := map[string]any{
		"model":       model,
		"messages":    messages,
		"temperature": numberParam(req.Params, "temperature", 0.2),
		"max_tokens":  intParam(req.Params, "max_tokens", 4096),
	}
	if p.disableThinking {
		body["chat_template_kwargs"] = map[string]any{"enable_thinking": false}
	}
	if responseFormat, ok := req.Params["response_format"]; ok {
		body["response_format"] = responseFormat
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionsEndpoint(p.baseURL), bytes.NewReader(payload))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	httpReq.Header.Set("Accept", "application/json")
	res, err := p.client.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 12<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return Response{}, fmt.Errorf("chat provider %s returned HTTP %d: %s", p.Code(), res.StatusCode, strings.TrimSpace(string(raw)))
	}
	var decoded chatCompletionResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return Response{}, fmt.Errorf("decode chat provider response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return Response{}, errors.New("chat provider returned no choices")
	}
	choice := decoded.Choices[0]
	content := firstNonEmptyString(choice.Message.Content, choice.Message.ReasoningContent, choice.Message.Reasoning)
	if strings.TrimSpace(content) == "" {
		return Response{}, errors.New("chat provider returned empty content")
	}
	role := strings.TrimSpace(choice.Message.Role)
	if role == "" {
		role = "assistant"
	}
	return Response{
		ProviderCode: p.Code(),
		Model:        firstNonEmptyString(decoded.Model, model),
		Message:      Message{Role: role, Content: content},
		Usage:        decoded.Usage,
		Metadata: map[string]any{
			"id":           decoded.ID,
			"finishReason": choice.FinishReason,
		},
	}, nil
}

func (p OpenAICompatible) StreamChat(ctx context.Context, req generation.CreateRequest) (<-chan Chunk, error) {
	ch := make(chan Chunk, 1)
	go func() {
		defer close(ch)
		response, err := p.Chat(ctx, req)
		if err != nil {
			ch <- Chunk{Done: true, Metadata: map[string]any{"error": err.Error()}}
			return
		}
		ch <- Chunk{Delta: response.Message.Content, Done: true, Usage: response.Usage, Metadata: response.Metadata}
	}()
	return ch, nil
}

type chatCompletionResponse struct {
	ID      string         `json:"id"`
	Model   string         `json:"model"`
	Choices []chatChoice   `json:"choices"`
	Usage   map[string]any `json:"usage"`
}

type chatChoice struct {
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type chatMessage struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	Reasoning        string `json:"reasoning"`
	ReasoningContent string `json:"reasoning_content"`
}

func requestMessages(req generation.CreateRequest) []Message {
	if req.Params == nil {
		return nil
	}
	raw, ok := req.Params["messages"]
	if !ok {
		return nil
	}
	switch typed := raw.(type) {
	case []Message:
		return normalizeMessages(typed)
	case []any:
		messages := make([]Message, 0, len(typed))
		for _, item := range typed {
			if object, ok := item.(map[string]any); ok {
				messages = append(messages, Message{
					Role:    strings.TrimSpace(fmt.Sprint(object["role"])),
					Content: strings.TrimSpace(fmt.Sprint(object["content"])),
				})
			}
		}
		return normalizeMessages(messages)
	default:
		return nil
	}
}

func normalizeMessages(items []Message) []Message {
	messages := make([]Message, 0, len(items))
	for _, item := range items {
		role := strings.TrimSpace(item.Role)
		content := strings.TrimSpace(item.Content)
		if role == "" || content == "" {
			continue
		}
		messages = append(messages, Message{Role: role, Content: content})
	}
	return messages
}

func isChatType(taskType string) bool {
	switch strings.ToUpper(strings.TrimSpace(taskType)) {
	case "", "CHAT", "CHAT_COMPLETION", "AGENT_CHAT":
		return true
	default:
		return false
	}
}

func chatCompletionsEndpoint(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(base)
	if err == nil && strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/v1") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}

func intParam(params map[string]any, key string, fallback int) int {
	if params == nil {
		return fallback
	}
	switch typed := params[key].(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	case string:
		value, _ := strconv.Atoi(strings.TrimSpace(typed))
		if value > 0 {
			return value
		}
	}
	return fallback
}

func numberParam(params map[string]any, key string, fallback float64) float64 {
	if params == nil {
		return fallback
	}
	switch typed := params[key].(type) {
	case int:
		return float64(typed)
	case float64:
		return typed
	case string:
		value, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return value
		}
	}
	return fallback
}

func intValue(value string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(value))
	return parsed
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), value) {
			return true
		}
	}
	return false
}

func nonEmptyStrings(items ...string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[strings.ToLower(item)] {
			continue
		}
		seen[strings.ToLower(item)] = true
		result = append(result, item)
	}
	return result
}

func appendIfMissing(items []string, item string) []string {
	if containsString(items, item) {
		return items
	}
	return append(items, item)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
