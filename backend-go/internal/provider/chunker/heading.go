package chunker

import (
	"context"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type Heading struct {
	Fixed Fixed
}

func (Heading) Code() string { return "heading" }

func (h Heading) Chunk(ctx context.Context, units []knowledgeapp.DocumentUnit, options knowledgeapp.ChunkOptions) ([]knowledgeapp.Chunk, error) {
	return h.Fixed.Chunk(ctx, units, options)
}
