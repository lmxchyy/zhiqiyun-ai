package chunker

import (
	"context"
	"regexp"
	"strings"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type Semantic struct{ Fixed Fixed }
type Markdown struct{ Heading Heading }

var sentenceBoundary = regexp.MustCompile(`([。！？!?；;]\s*|\n{2,})`)

func (Semantic) Code() string { return "semantic" }
func (Markdown) Code() string { return "markdown" }

func (m Markdown) Chunk(ctx context.Context, units []knowledgeapp.DocumentUnit, options knowledgeapp.ChunkOptions) ([]knowledgeapp.Chunk, error) {
	return m.Heading.Chunk(ctx, units, options)
}

func (s Semantic) Chunk(ctx context.Context, units []knowledgeapp.DocumentUnit, options knowledgeapp.ChunkOptions) ([]knowledgeapp.Chunk, error) {
	if options.ChunkSize <= 0 {
		options.ChunkSize = 800
	}
	semanticUnits := make([]knowledgeapp.DocumentUnit, 0)
	for _, unit := range units {
		parts := sentenceBoundary.Split(unit.Content, -1)
		current := strings.Builder{}
		flush := func() {
			content := strings.TrimSpace(current.String())
			if content != "" {
				copyUnit := unit
				copyUnit.Content = content
				semanticUnits = append(semanticUnits, copyUnit)
			}
			current.Reset()
		}
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if current.Len() > 0 && len([]rune(current.String()))+len([]rune(part)) > options.ChunkSize {
				flush()
			}
			if current.Len() > 0 {
				current.WriteString("。")
			}
			current.WriteString(part)
		}
		flush()
	}
	return s.Fixed.Chunk(ctx, semanticUnits, options)
}

var _ knowledgeapp.Chunker = Semantic{}
var _ knowledgeapp.Chunker = Markdown{}
