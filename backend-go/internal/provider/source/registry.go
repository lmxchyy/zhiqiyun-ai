package source

import (
	"context"
	"fmt"
	"strings"
	"sync"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type SyncRequest struct {
	TenantID string
	SourceID string
	Cursor   string
	Config   map[string]any
}

type SyncResult struct {
	Documents  []knowledgeapp.SourceDocument
	NextCursor string
	Metadata   map[string]any
}

type Connector interface {
	Code() string
	Sync(context.Context, SyncRequest) (SyncResult, error)
}

type Registry struct {
	mu    sync.RWMutex
	items map[string]Connector
}

func NewRegistry(connectors ...Connector) *Registry {
	r := &Registry{items: map[string]Connector{}}
	for _, item := range connectors {
		r.Register(item)
	}
	return r
}
func (r *Registry) Register(item Connector) {
	if item == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[strings.ToLower(item.Code())] = item
}
func (r *Registry) Get(code string) (Connector, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item := r.items[strings.ToLower(strings.TrimSpace(code))]
	if item == nil {
		return nil, fmt.Errorf("source connector %q is not registered", code)
	}
	return item, nil
}

// StaticUpload keeps direct uploads on the same source connector contract as
// future Web, Notion, Feishu, DingTalk, WeCom, OSS, S3, GitHub and RSS sync.
type StaticUpload struct{ Documents []knowledgeapp.SourceDocument }

func (StaticUpload) Code() string { return "upload" }
func (s StaticUpload) Sync(_ context.Context, _ SyncRequest) (SyncResult, error) {
	return SyncResult{Documents: append([]knowledgeapp.SourceDocument(nil), s.Documents...), Metadata: map[string]any{"mode": "snapshot"}}, nil
}
