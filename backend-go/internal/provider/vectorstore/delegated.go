package vectorstore

import (
	"context"
	"fmt"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

// Backend isolates vendor SDKs from the knowledge domain. Official Milvus,
// Qdrant or Weaviate SDK adapters can implement this small contract and be
// hot-registered in Registry.
type Backend interface {
	Upsert(context.Context, string, []knowledgeapp.VectorRecord) error
	DeleteByDocumentVersion(context.Context, knowledgeapp.AccessContext, string, string) error
	Search(context.Context, knowledgeapp.SearchRequest, []float32) ([]knowledgeapp.RetrievalHit, error)
}

type Delegated struct {
	code    string
	backend Backend
}

func NewMilvus(backend Backend) *Delegated  { return &Delegated{code: "milvus", backend: backend} }
func NewQdrant(backend Backend) *Delegated  { return &Delegated{code: "qdrant", backend: backend} }
func NewWeaviate(backend Backend) *Delegated { return &Delegated{code: "weaviate", backend: backend} }

func (d *Delegated) Code() string { return d.code }
func (d *Delegated) Upsert(ctx context.Context, indexID string, records []knowledgeapp.VectorRecord) error {
	if d == nil || d.backend == nil { return fmt.Errorf("%s vector backend is not configured", d.code) }
	return d.backend.Upsert(ctx, indexID, records)
}
func (d *Delegated) DeleteByDocumentVersion(ctx context.Context, access knowledgeapp.AccessContext, indexID, versionID string) error {
	if d == nil || d.backend == nil { return fmt.Errorf("%s vector backend is not configured", d.code) }
	return d.backend.DeleteByDocumentVersion(ctx, access, indexID, versionID)
}
func (d *Delegated) Search(ctx context.Context, request knowledgeapp.SearchRequest, vector []float32) ([]knowledgeapp.RetrievalHit, error) {
	if d == nil || d.backend == nil { return nil, fmt.Errorf("%s vector backend is not configured", d.code) }
	return d.backend.Search(ctx, request, vector)
}

var _ knowledgeapp.VectorStore = (*Delegated)(nil)
