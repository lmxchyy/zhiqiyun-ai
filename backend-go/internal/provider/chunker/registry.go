package chunker

import (
	"context"
	"fmt"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type Registry struct {
	items map[string]knowledgeapp.Chunker
}

func NewRegistry(items ...knowledgeapp.Chunker) Registry {
	registry := Registry{items: map[string]knowledgeapp.Chunker{}}
	for _, item := range items {
		if item != nil {
			registry.items[item.Code()] = item
		}
	}
	return registry
}

func NewDefaultRegistry() Registry {
	fixed := Fixed{}
	heading := Heading{Fixed: fixed}
	return NewRegistry(fixed, heading, Markdown{Heading: heading}, Semantic{Fixed: fixed})
}

func (r Registry) Chunk(ctx context.Context, code string, units []knowledgeapp.DocumentUnit, options knowledgeapp.ChunkOptions) ([]knowledgeapp.Chunk, error) {
	item := r.items[code]
	if item == nil {
		item = r.items["fixed"]
	}
	if item == nil {
		return nil, fmt.Errorf("chunker %q is not registered", code)
	}
	return item.Chunk(ctx, units, options)
}
