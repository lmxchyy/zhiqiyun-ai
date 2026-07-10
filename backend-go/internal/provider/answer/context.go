package answer

import (
	"context"
	"fmt"
	"strings"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

// Context is an offline-safe answer generator. Production wiring prefers the
// configured chat model and falls back to this generator when no model channel
// is configured or the provider is temporarily unavailable.
type Context struct{}

func (Context) Generate(ctx context.Context, request knowledgeapp.AnswerRequest) (<-chan knowledgeapp.AnswerChunk, error) {
	stream := make(chan knowledgeapp.AnswerChunk, 1)
	go func() {
		defer close(stream)
		select {
		case <-ctx.Done():
			stream <- knowledgeapp.AnswerChunk{Done: true, Err: ctx.Err()}
			return
		default:
		}
		answer := buildContextAnswer(request.Question, request.Hits)
		stream <- knowledgeapp.AnswerChunk{
			Delta: answer, Done: true,
			Usage:    map[string]any{"input_tokens": estimateTokens(request.Question, request.Hits), "output_tokens": estimateTextTokens(answer)},
			Metadata: map[string]any{"provider": "knowledge-context-fallback"},
		}
	}()
	return stream, nil
}

func estimateTextTokens(value string) int {
	return (len([]rune(strings.TrimSpace(value))) + 2) / 3
}

func buildContextAnswer(question string, hits []knowledgeapp.RetrievalHit) string {
	if len(hits) == 0 {
		return "当前绑定的知识库中没有检索到足够相关的内容。请换一种问法，或检查知识库文档是否已完成解析和索引。"
	}
	limit := len(hits)
	if limit > 4 {
		limit = 4
	}
	var result strings.Builder
	result.WriteString("根据知识库中检索到的资料，以下内容与问题最相关：\n\n")
	for index := 0; index < limit; index++ {
		hit := hits[index]
		content := truncate(hit.Content, 420)
		fmt.Fprintf(&result, "%d. %s [%d]\n", index+1, content, index+1)
	}
	result.WriteString("\n请以上述引用原文为依据核对结论；引用编号可查看来源文档、页码或原文定位。")
	return result.String()
}

func estimateTokens(question string, hits []knowledgeapp.RetrievalHit) int {
	length := len([]rune(question))
	for _, hit := range hits {
		length += len([]rune(hit.Content))
	}
	return (length + 2) / 3
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

var _ knowledgeapp.AnswerGenerator = Context{}
