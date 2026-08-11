package smartvideoplan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/app/smartvideo"
	"xianzhi-ai/backend-go/internal/provider/chat"
)

var (
	ErrInvalidJSON          = errors.New("provider_invalid_json")
	ErrRateLimited          = errors.New("provider_rate_limited")
	ErrProviderUnavailable  = errors.New("provider_unavailable")
	ErrEmptyPlan            = errors.New("provider_empty_plan")
)

type ChatCaller interface {
	Chat(context.Context, generation.CreateRequest) (chat.Response, error)
}

type Client struct {
	chat     ChatCaller
	modelKey string
	timeout  time.Duration
}

type Options struct {
	ModelKey string
	Timeout  time.Duration
}

type ProviderUsage struct {
	ModelKey          string
	ProviderRequestID string
	PromptTokens      int64
	CompletionTokens  int64
	TotalTokens       int64
	LatencyMs         int64
}

func NewClient(caller ChatCaller, opts Options) *Client {
	if opts.Timeout <= 0 {
		opts.Timeout = 90 * time.Second
	}
	if strings.TrimSpace(opts.ModelKey) == "" {
		opts.ModelKey = "smart-video-standard"
	}
	return &Client{chat: caller, modelKey: opts.ModelKey, timeout: opts.Timeout}
}

func (c *Client) Plan(ctx context.Context, request smartvideo.PlanRequest) (smartvideo.EditPlanV1, ProviderUsage, error) {
	if c == nil || c.chat == nil {
		return smartvideo.EditPlanV1{}, ProviderUsage{}, ErrProviderUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	started := time.Now()
	owned := map[string]smartvideo.ProjectAsset{}
	for _, asset := range request.Assets {
		owned[asset.ID] = asset
	}
	req := generation.CreateRequest{
		Type:   "SMART_VIDEO_PLAN",
		Model:  c.modelKey,
		Prompt: buildPlanPrompt(request),
		Params: map[string]any{
			"temperature": 0.2,
			"max_tokens":  8192,
			"response_format": map[string]any{
				"type": "json_schema",
				"json_schema": map[string]any{
					"name":   "edit_plan_v1",
					"strict": true,
					"schema": editPlanJSONSchema(),
				},
			},
			"messages": []chat.Message{
				{Role: "system", Content: planSystemPrompt()},
				{Role: "user", Content: buildPlanPrompt(request)},
			},
		},
	}
	response, err := c.chat.Chat(ctx, req)
	usage := ProviderUsage{
		ModelKey:  firstNonEmpty(response.Model, c.modelKey),
		LatencyMs: time.Since(started).Milliseconds(),
	}
	if response.Metadata != nil {
		if id, ok := response.Metadata["id"].(string); ok {
			usage.ProviderRequestID = id
		}
	}
	fillUsage(&usage, response.Usage)
	if err != nil {
		return smartvideo.EditPlanV1{}, usage, mapProviderError(err)
	}
	content := strings.TrimSpace(response.Message.Content)
	if content == "" {
		return smartvideo.EditPlanV1{}, usage, ErrEmptyPlan
	}
	plan, err := decodeEditPlan(content)
	if err != nil {
		return smartvideo.EditPlanV1{}, usage, err
	}
	if err := smartvideo.ValidateEditPlanV1(plan, owned); err != nil {
		return smartvideo.EditPlanV1{}, usage, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	return plan, usage, nil
}

func planSystemPrompt() string {
	return strings.TrimSpace(`
你是 AI 自动混剪规划器。只能输出符合 EditPlanV1 JSON Schema 的 JSON 对象。
约束：
1. schemaVersion 必须为 1
2. 只能引用用户提供的 assetId
3. 目标时长必须在 15000..60000 毫秒
4. 转场仅允许 cut/fade/dissolve/wipeleft/wiperight/slideleft/slideright
5. 不要输出 Markdown，不要解释
`)
}

func buildPlanPrompt(request smartvideo.PlanRequest) string {
	var b strings.Builder
	b.WriteString("用户需求：")
	b.WriteString(strings.TrimSpace(request.Requirement))
	b.WriteString("\n补充指令：")
	b.WriteString(strings.TrimSpace(request.Instruction))
	b.WriteString("\n目标规格：")
	_ = json.NewEncoder(&b).Encode(request.TargetSpec)
	b.WriteString("可用素材（按顺序）：\n")
	for index, asset := range request.Assets {
		b.WriteString(fmt.Sprintf("%d. assetId=%s type=%s durationMs=%d", index, asset.ID, asset.AssetType, asset.DurationMS))
		if asset.NormalizedMetadata != nil {
			raw, _ := json.Marshal(asset.NormalizedMetadata)
			b.WriteString(" metadata=")
			b.Write(raw)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func editPlanJSONSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"schemaVersion", "title", "summary", "language", "target", "voice", "subtitles", "audio", "scenes"},
		"properties": map[string]any{
			"schemaVersion": map[string]any{"type": "integer", "const": 1},
			"title":         map[string]any{"type": "string"},
			"summary":       map[string]any{"type": "string"},
			"language":      map[string]any{"type": "string", "enum": []string{"zh-CN", "en-US"}},
			"target": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"aspectRatio", "resolution", "durationMs"},
				"properties": map[string]any{
					"aspectRatio": map[string]any{"type": "string", "enum": []string{"9:16", "16:9"}},
					"resolution":  map[string]any{"type": "string", "enum": []string{"720p", "1080p"}},
					"durationMs":  map[string]any{"type": "integer", "minimum": 15000, "maximum": 60000},
				},
			},
			"voice": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"enabled", "modelKey", "voiceKey", "speed"},
				"properties": map[string]any{
					"enabled":  map[string]any{"type": "boolean"},
					"modelKey": map[string]any{"type": "string"},
					"voiceKey": map[string]any{"type": "string"},
					"speed":    map[string]any{"type": "number", "minimum": 0.8, "maximum": 1.2},
				},
			},
			"subtitles": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"enabled", "preset", "position"},
				"properties": map[string]any{
					"enabled":  map[string]any{"type": "boolean"},
					"preset":   map[string]any{"type": "string", "enum": []string{"clean", "emphasis"}},
					"position": map[string]any{"type": "string", "enum": []string{"bottom", "center"}},
				},
			},
			"audio": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"sourceGain", "voiceGain"},
				"properties": map[string]any{
					"sourceGain": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
					"voiceGain":  map[string]any{"type": "number", "minimum": 0, "maximum": 1},
				},
			},
			"scenes": map[string]any{
				"type":     "array",
				"minItems": 1,
				"maxItems": 40,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"id", "index", "title", "durationMs", "narration", "clips", "transition"},
					"properties": map[string]any{
						"id":         map[string]any{"type": "string"},
						"index":      map[string]any{"type": "integer", "minimum": 0},
						"title":      map[string]any{"type": "string"},
						"durationMs": map[string]any{"type": "integer", "minimum": 1},
						"narration":  map[string]any{"type": "string"},
						"clips": map[string]any{
							"type":     "array",
							"minItems": 1,
							"maxItems": 6,
							"items": map[string]any{
								"type":                 "object",
								"additionalProperties": false,
								"required": []string{
									"assetId", "assetType", "sourceInMs", "sourceOutMs",
									"displayDurationMs", "fitMode", "motion", "originalAudioGain",
								},
								"properties": map[string]any{
									"assetId":           map[string]any{"type": "string"},
									"assetType":         map[string]any{"type": "string", "enum": []string{"image", "video"}},
									"sourceInMs":        map[string]any{"type": "integer", "minimum": 0},
									"sourceOutMs":       map[string]any{"type": "integer", "minimum": 0},
									"displayDurationMs": map[string]any{"type": "integer", "minimum": 1},
									"fitMode":           map[string]any{"type": "string", "enum": []string{"cover", "contain"}},
									"motion":            map[string]any{"type": "string", "enum": []string{"static", "push", "pull", "pan_left", "pan_right"}},
									"originalAudioGain": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
								},
							},
						},
						"transition": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"type", "durationMs"},
							"properties": map[string]any{
								"type":       map[string]any{"type": "string"},
								"durationMs": map[string]any{"type": "integer", "minimum": 0, "maximum": 1000},
							},
						},
					},
				},
			},
		},
	}
}

func decodeEditPlan(content string) (smartvideo.EditPlanV1, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	var plan smartvideo.EditPlanV1
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return smartvideo.EditPlanV1{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	return plan, nil
}

func mapProviderError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "429"), strings.Contains(msg, "rate limit"):
		return fmt.Errorf("%w: %v", ErrRateLimited, err)
	case strings.Contains(msg, "http 5"), strings.Contains(msg, "timeout"), strings.Contains(msg, "unavailable"):
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	default:
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
}

func fillUsage(usage *ProviderUsage, raw map[string]any) {
	if usage == nil || raw == nil {
		return
	}
	usage.PromptTokens = int64Value(raw["prompt_tokens"])
	usage.CompletionTokens = int64Value(raw["completion_tokens"])
	usage.TotalTokens = int64Value(raw["total_tokens"])
}

func int64Value(raw any) int64 {
	switch value := raw.(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	case json.Number:
		parsed, _ := value.Int64()
		return parsed
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// StatusCodeFromError helps HTTP layers map provider failures.
func StatusCodeFromError(err error) int {
	switch {
	case errors.Is(err, ErrRateLimited):
		return http.StatusTooManyRequests
	case errors.Is(err, ErrInvalidJSON), errors.Is(err, ErrEmptyPlan):
		return http.StatusBadGateway
	default:
		return http.StatusBadGateway
	}
}
