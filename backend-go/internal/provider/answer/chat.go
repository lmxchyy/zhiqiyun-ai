package answer

import (
	"context"
	"fmt"
	"strings"

	"xianzhi-ai/backend-go/internal/app/generation"
	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
	chatprovider "xianzhi-ai/backend-go/internal/provider/chat"
)

type Chat struct {
	router   chatprovider.Router
	fallback knowledgeapp.AnswerGenerator
}

func NewChat(router chatprovider.Router, fallback knowledgeapp.AnswerGenerator) Chat {
	return Chat{router: router, fallback: fallback}
}

func (g Chat) Generate(ctx context.Context, request knowledgeapp.AnswerRequest) (<-chan knowledgeapp.AnswerChunk, error) {
	messages := buildMessages(request)
	model := strings.TrimSpace(request.Agent.ModelName)
	if model == "" {
		model = g.router.DefaultModel()
	}
	stream, err := g.router.StreamChat(ctx, generation.CreateRequest{
		Type: "AGENT_CHAT", Model: model, Prompt: request.Question,
		Params: map[string]any{"messages": messages, "temperature": 0.2, "max_tokens": 4096},
	})
	if err != nil {
		if g.fallback != nil {
			return g.fallback.Generate(ctx, request)
		}
		return nil, err
	}
	output := make(chan knowledgeapp.AnswerChunk)
	go func() {
		defer close(output)
		for chunk := range stream {
			if message, ok := chunk.Metadata["error"].(string); ok && strings.TrimSpace(message) != "" {
				if g.fallback != nil {
					fallback, fallbackErr := g.fallback.Generate(ctx, request)
					if fallbackErr != nil {
						output <- knowledgeapp.AnswerChunk{Done: true, Err: fallbackErr}
						return
					}
					for item := range fallback {
						output <- item
					}
					return
				}
				output <- knowledgeapp.AnswerChunk{Done: true, Err: fmt.Errorf("chat provider: %s", message)}
				return
			}
			output <- knowledgeapp.AnswerChunk{Delta: chunk.Delta, Done: chunk.Done, Usage: chunk.Usage, Metadata: chunk.Metadata}
		}
	}()
	return output, nil
}

func buildMessages(request knowledgeapp.AnswerRequest) []chatprovider.Message {
	messages := make([]chatprovider.Message, 0, len(request.Messages)+2)
	system := strings.TrimSpace(request.Agent.SystemPrompt)
	if system == "" {
		system = "你是企业知识库智能体。只能依据提供的知识库上下文回答；资料不足时必须明确说明。回答中的关键结论使用 [1]、[2] 格式标注引用编号。"
	}
	system += "\n\n知识库上下文：\n" + contextBlock(request.Hits)
	messages = append(messages, chatprovider.Message{Role: "system", Content: system})
	start := len(request.Messages) - 12
	if start < 0 {
		start = 0
	}
	for _, item := range request.Messages[start:] {
		if item.Role == "user" || item.Role == "assistant" {
			messages = append(messages, chatprovider.Message{Role: item.Role, Content: item.Content})
		}
	}
	messages = append(messages, chatprovider.Message{Role: "user", Content: request.Question})
	return messages
}

func contextBlock(hits []knowledgeapp.RetrievalHit) string {
	if len(hits) == 0 {
		return "（未检索到相关资料）"
	}
	var result strings.Builder
	for index, hit := range hits {
		fmt.Fprintf(&result, "[%d] 文档：%s；标题：%s；相关度：%.4f\n%s\n\n", index+1, hit.DocumentName, hit.Title, hit.FinalScore, hit.Content)
	}
	return result.String()
}

var _ knowledgeapp.AnswerGenerator = Chat{}
