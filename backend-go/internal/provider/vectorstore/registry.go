package vectorstore

import (
	"fmt"
	"strings"
	"sync"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

// Registry is the runtime switch point for pgvector, Milvus, Qdrant and
// Weaviate drivers. Drivers implement the domain VectorStore port and can be
// replaced without changing ingestion or retrieval services.
type Registry struct {
	mu    sync.RWMutex
	items map[string]knowledgeapp.VectorStore
}

func NewRegistry(items ...knowledgeapp.VectorStore) *Registry {
	registry := &Registry{items: map[string]knowledgeapp.VectorStore{}}
	for _, item := range items {
		if item != nil {
			registry.items[strings.ToLower(item.Code())] = item
		}
	}
	return registry
}

func (r *Registry) Register(item knowledgeapp.VectorStore) {
	if r == nil || item == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[strings.ToLower(item.Code())] = item
}

func (r *Registry) Get(code string) (knowledgeapp.VectorStore, error) {
	if r == nil {
		return nil, fmt.Errorf("vector store registry is nil")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item := r.items[strings.ToLower(strings.TrimSpace(code))]
	if item == nil {
		return nil, fmt.Errorf("vector store %q is not registered", code)
	}
	return item, nil
}

func (r *Registry) Codes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]string, 0, len(r.items))
	for code := range r.items {
		result = append(result, code)
	}
	return result
}
