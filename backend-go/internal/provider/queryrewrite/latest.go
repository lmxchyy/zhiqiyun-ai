package queryrewrite

import (
	"context"
	"strings"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

// Latest keeps explicit questions unchanged and only resolves a small set of
// follow-up pronouns with the latest user turn. It is deterministic and can be
// replaced by an LLM-based rewriter through the knowledge QueryRewriter port.
type Latest struct{}

func (Latest) Rewrite(_ context.Context, messages []knowledgeapp.Message, question string) (string, error) {
	question = strings.TrimSpace(question)
	if question == "" || !looksLikeFollowUp(question) {
		return question, nil
	}
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == "user" && strings.TrimSpace(messages[index].Content) != "" {
			return strings.TrimSpace(messages[index].Content) + "；追问：" + question, nil
		}
	}
	return question, nil
}

func looksLikeFollowUp(question string) bool {
	for _, marker := range []string{"它", "这个", "上述", "前面", "那", "其", "这些"} {
		if strings.Contains(question, marker) {
			return true
		}
	}
	return false
}

var _ knowledgeapp.QueryRewriter = Latest{}
