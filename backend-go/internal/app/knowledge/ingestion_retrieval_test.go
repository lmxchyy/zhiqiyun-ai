package knowledge_test

import (
	"context"
	"testing"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
	"xianzhi-ai/backend-go/internal/provider/chunker"
	"xianzhi-ai/backend-go/internal/provider/embedding"
	"xianzhi-ai/backend-go/internal/provider/parser"
	"xianzhi-ai/backend-go/internal/provider/rerank"
	"xianzhi-ai/backend-go/internal/provider/vectorstore"
	knowledgerepo "xianzhi-ai/backend-go/internal/repository/knowledge"
)

func TestIngestAndHybridSearchReturnsTraceableSource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := knowledgerepo.NewMemory()
	core := knowledgeapp.NewService(repo, repo)
	access, err := core.ResolveAccessContext(ctx, "user_owner", "tenant_a", "")
	if err != nil {
		t.Fatal(err)
	}
	base, err := core.CreateKnowledgeBase(ctx, access, knowledgeapp.CreateKnowledgeBaseInput{Name: "产品知识库", KnowledgeType: knowledgeapp.KnowledgePersonal})
	if err != nil {
		t.Fatal(err)
	}
	embedder := embedding.NewDeterministic(128)
	vectors := vectorstore.NewMemory()
	ingestion := knowledgeapp.NewIngestionService(core, repo, parser.NewDefaultRegistry(), chunker.NewDefaultRegistry(), embedder, vectors)
	result, err := ingestion.Ingest(ctx, access, base.ID, knowledgeapp.IngestDocumentInput{
		Name: "企业版产品手册.md", MIMEType: "text/markdown",
		Content:    []byte("# 企业版成员与权限\n\n企业版默认支持 100 个成员，并支持部门级知识库权限隔离。\n\n# 引用\n回答必须显示来源文档和页码。"),
		ChunkerKey: "heading", ChunkOptions: knowledgeapp.ChunkOptions{ChunkSize: 80, Overlap: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Document.Status != "READY" || len(result.Chunks) == 0 {
		t.Fatalf("unexpected ingestion result: %#v", result)
	}
	retrieval := knowledgeapp.NewRetrievalService(embedder, vectors, rerank.None{})
	hits, err := retrieval.Search(ctx, knowledgeapp.SearchRequest{
		Access: access, KnowledgeBaseIDs: []string{base.ID}, Query: "企业版支持多少成员", Mode: "HYBRID", TopK: 5, Threshold: 0.05,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected retrieval hit")
	}
	if hits[0].DocumentName != "企业版产品手册.md" || hits[0].DocumentID != result.Document.ID {
		t.Fatalf("expected traceable document source, got %#v", hits[0])
	}
}
