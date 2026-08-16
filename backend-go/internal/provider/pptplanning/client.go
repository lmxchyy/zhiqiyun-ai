package pptplanning

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	"xianzhi-ai/backend-go/internal/provider/chat"
)

type ChatCaller interface {
	Chat(context.Context, generation.CreateRequest) (chat.Response, error)
}

type Options struct {
	Model   string
	Timeout time.Duration
}

type Client struct {
	chat    ChatCaller
	model   string
	timeout time.Duration
}

func NewClient(caller ChatCaller, options Options) *Client {
	if options.Timeout <= 0 {
		options.Timeout = 90 * time.Second
	}
	return &Client{chat: caller, model: strings.TrimSpace(options.Model), timeout: options.Timeout}
}

func (c *Client) PlanStoryline(ctx context.Context, input pptapp.StorylinePlanningInput) (pptapp.StorylinePlanningOutput, error) {
	if c == nil || c.chat == nil || c.model == "" {
		return pptapp.StorylinePlanningOutput{}, planningError(pptapp.PlanningProviderUnavailable, "规划服务暂时不可用，请稍后重试。", nil)
	}
	prompt, err := storylinePrompt(input)
	if err != nil {
		return pptapp.StorylinePlanningOutput{}, planningError(pptapp.PlanningContractValidationFailed, "规划输入未通过校验，请重试。", err)
	}
	response, err := c.call(ctx, "You are a professional presentation narrative planner. Return one strict JSON object and no Markdown.", prompt)
	if err != nil {
		return pptapp.StorylinePlanningOutput{}, err
	}
	var draft pptapp.StorylineDraft
	if err := decodeStrictJSON(response.Message.Content, &draft); err != nil {
		return pptapp.StorylinePlanningOutput{}, planningError(pptapp.PlanningInvalidOutput, "规划结果无法解析，请重试。", err)
	}
	return pptapp.StorylinePlanningOutput{Draft: draft, Provenance: planningProvenance(response)}, nil
}

func (c *Client) PlanOutline(ctx context.Context, input pptapp.OutlinePlanningInput) (pptapp.OutlinePlanningOutput, error) {
	if c == nil || c.chat == nil || c.model == "" {
		return pptapp.OutlinePlanningOutput{}, planningError(pptapp.PlanningProviderUnavailable, "规划服务暂时不可用，请稍后重试。", nil)
	}
	prompt, err := outlinePrompt(input)
	if err != nil {
		return pptapp.OutlinePlanningOutput{}, planningError(pptapp.PlanningContractValidationFailed, "规划输入未通过校验，请重试。", err)
	}
	response, err := c.call(ctx, "You are a professional presentation outline planner. Return one strict JSON object and no Markdown.", prompt)
	if err != nil {
		return pptapp.OutlinePlanningOutput{}, err
	}
	var draft pptapp.OutlinePlanDraft
	if err := decodeStrictJSON(response.Message.Content, &draft); err != nil {
		return pptapp.OutlinePlanningOutput{}, planningError(pptapp.PlanningInvalidOutput, "规划结果无法解析，请重试。", err)
	}
	return pptapp.OutlinePlanningOutput{Draft: draft, Provenance: planningProvenance(response)}, nil
}

func (c *Client) PlanSlideContent(ctx context.Context, input pptapp.SlideContentPlanningInput) (pptapp.SlideContentPlanningOutput, error) {
	if c == nil || c.chat == nil || c.model == "" {
		return pptapp.SlideContentPlanningOutput{}, planningError(pptapp.ContentProviderUnavailable, "内容生成服务暂时不可用，请稍后重试。", nil)
	}
	prompt, err := slideContentPrompt(input)
	if err != nil {
		return pptapp.SlideContentPlanningOutput{}, planningError(pptapp.ContentContractValidationFailed, "页面内容输入未通过校验，请重试。", err)
	}
	response, err := c.callContent(ctx, "You are a professional presentation slide content planner. Return one strict JSON object and no Markdown.", prompt)
	if err != nil {
		return pptapp.SlideContentPlanningOutput{}, err
	}
	var draft pptapp.SlideContentDraft
	if err := decodeStrictJSON(response.Message.Content, &draft); err != nil {
		return pptapp.SlideContentPlanningOutput{}, planningError(pptapp.ContentInvalidOutput, "页面内容结果无法解析，请重试。", err)
	}
	return pptapp.SlideContentPlanningOutput{Draft: draft, Provenance: planningProvenance(response)}, nil
}

func (c *Client) call(ctx context.Context, systemPrompt, prompt string) (chat.Response, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.chat.Chat(callCtx, generation.CreateRequest{
		Type: "AGENT_CHAT", Model: c.model, Prompt: prompt,
		Params: map[string]any{
			"temperature":     0.2,
			"max_tokens":      8192,
			"response_format": map[string]any{"type": "json_object"},
			"messages": []chat.Message{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: prompt},
			},
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return chat.Response{}, planningError(pptapp.PlanningTimeout, "规划服务响应超时，请重试。", err)
		}
		return chat.Response{}, planningError(pptapp.PlanningProviderUnavailable, "规划服务暂时不可用，请稍后重试。", err)
	}
	if strings.TrimSpace(response.Message.Content) == "" {
		return chat.Response{}, planningError(pptapp.PlanningInvalidOutput, "规划结果为空，请重试。", nil)
	}
	return response, nil
}

func (c *Client) callContent(ctx context.Context, systemPrompt, prompt string) (chat.Response, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.chat.Chat(callCtx, generation.CreateRequest{
		Type: "AGENT_CHAT", Model: c.model, Prompt: prompt,
		Params: map[string]any{
			"temperature": 0.2, "max_tokens": 4096,
			"response_format": map[string]any{"type": "json_object"},
			"messages":        []chat.Message{{Role: "system", Content: systemPrompt}, {Role: "user", Content: prompt}},
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return chat.Response{}, planningError(pptapp.ContentTimeout, "页面内容生成超时，请重试。", err)
		}
		return chat.Response{}, planningError(pptapp.ContentProviderUnavailable, "内容生成服务暂时不可用，请稍后重试。", err)
	}
	if strings.TrimSpace(response.Message.Content) == "" {
		return chat.Response{}, planningError(pptapp.ContentInvalidOutput, "页面内容结果为空，请重试。", nil)
	}
	return response, nil
}

func storylinePrompt(input pptapp.StorylinePlanningInput) (string, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`Plan a coherent professional presentation storyline from the authoritative input below.
Use the requested language exactly. Base factual narrative sections on real claim IDs; never invent evidence.
The storyline decides what the deck proves and in what order. It is not a slide list.
Return exactly this JSON shape:
{"language":"zh-CN|en-US","thesis":"...","audienceTakeaway":"...","narrativeArc":["section-key"],"sections":[{"key":"stable-semantic-key","title":"...","objective":"...","evidenceRefs":["claim-id"]}],"closingAction":"..."}
Every narrativeArc item must match one section key, in the same order.
Authoritative input:
%s`, payload), nil
}

func outlinePrompt(input pptapp.OutlinePlanningInput) (string, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`Plan a 6-12 page professional presentation outline from the authoritative intent, research pack, and storyline below.
Respect an explicit page count exactly; otherwise choose a count inside the requested range based on narrative density.
Use the requested language exactly. Do not allocate claims by array position.
For every factual slide, set evidenceRequired=true and select only claims that semantically support its purpose/keyMessage.
Every selected claim needs a concrete rationale explaining why it supports that slide. Cover, divider, and closing action slides may use evidenceRequired=false.
Include IMAGE in expectedElementTypes for one to three semantically appropriate content slides so the professional deck has real visual evidence; do not add IMAGE to every slide.
Return exactly this JSON shape:
{"language":"zh-CN|en-US","slides":[{"title":"...","purpose":"...","keyMessage":"...","evidenceRequired":true,"evidence":[{"claimId":"claim-id","rationale":"..."}],"visualIntent":"...","expectedElementTypes":["TEXT","SHAPE"]}]}
Do not output slide IDs, revisions, timestamps, tenant data, Markdown, or commentary.
Authoritative input:
%s`, payload), nil
}

func slideContentPrompt(input pptapp.SlideContentPlanningInput) (string, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`Write one professional presentation slide from the authoritative approved objective below.
Use the requested language exactly. Preserve every approved claim ID in citationRefs and never invent evidence.
Use image asset intents only when the approved expectedElementTypes includes IMAGE. Do not include URLs or image bytes.
When IMAGE is approved, choose text-image or image-text and return exactly one image asset intent; otherwise choose a non-image layout and return no asset intents.
Choose one layoutHint from: cover, section, title-body, title-bullets, two-column, text-image, image-text, key-metric, closing-action.
Return exactly this JSON shape:
{"language":"zh-CN|en-US","title":"...","subtitle":"...","bodyBlocks":[{"heading":"...","text":"..."}],"bullets":["..."],"supportingText":"...","speakerNotes":"...","assetIntents":[{"id":"semantic-key","kind":"image","prompt":"...","altText":"..."}],"citationRefs":["claim-id"],"layoutHint":"title-bullets"}
The title, body, notes, layout and image prompt must serve the approved purpose and keyMessage. Speaker notes must include readable source context for factual slides.
Authoritative input:
%s`, payload), nil
}

func decodeStrictJSON(content string, target any) error {
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("planning output contains trailing JSON")
		}
		return err
	}
	return nil
}

func planningProvenance(response chat.Response) pptapp.PlanningProvenance {
	requestID := ""
	if response.Metadata != nil {
		requestID, _ = response.Metadata["id"].(string)
	}
	return pptapp.PlanningProvenance{
		Mode: pptapp.PlanningModeAI, Provider: strings.TrimSpace(response.ProviderCode),
		Model: strings.TrimSpace(response.Model), ProviderRequestID: strings.TrimSpace(requestID),
	}
}

func planningError(code, message string, cause error) *pptapp.AgentWorkflowError {
	return pptapp.NewAgentWorkflowError(code, message, true, cause)
}

var _ pptapp.StorylinePlanningPort = (*Client)(nil)
var _ pptapp.OutlinePlanningPort = (*Client)(nil)
var _ pptapp.SlideContentPlanningPort = (*Client)(nil)
