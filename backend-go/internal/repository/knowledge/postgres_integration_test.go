package knowledgerepo_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
	answerprovider "xianzhi-ai/backend-go/internal/provider/answer"
	"xianzhi-ai/backend-go/internal/provider/chunker"
	"xianzhi-ai/backend-go/internal/provider/embedding"
	"xianzhi-ai/backend-go/internal/provider/knowledgeprofile"
	"xianzhi-ai/backend-go/internal/provider/parser"
	"xianzhi-ai/backend-go/internal/provider/queryrewrite"
	"xianzhi-ai/backend-go/internal/provider/rerank"
	"xianzhi-ai/backend-go/internal/provider/vectorstore"
	knowledgerepo "xianzhi-ai/backend-go/internal/repository/knowledge"
)

func TestPostgresKnowledgeAgentVerticalFlow(t *testing.T) {
	dsn := os.Getenv("KNOWLEDGE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("KNOWLEDGE_TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	userID := fmt.Sprintf("user_knowledge_integration_%d", time.Now().UTC().UnixNano())
	if _, err := db.ExecContext(ctx, `
		insert into xz_users (id, email, name, role, status, plan_id)
		values ($1, $2, 'Knowledge Integration', 'MEMBER', 'ACTIVE', 'plan_free')
		on conflict (id) do update set status='ACTIVE'
	`, userID, fmt.Sprintf("knowledge-integration-%d@example.com", time.Now().UTC().UnixNano())); err != nil {
		t.Fatal(err)
	}
	repo := knowledgerepo.NewPostgres(db)
	core := knowledgeapp.NewService(repo, repo)
	access, err := core.ResolveAccessContext(ctx, userID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	for resource, profile := range map[string]map[string]any{
		"embedding-profiles": {"id": "embedding_integration", "tenantId": access.TenantID, "name": "Integration Embedding", "providerKey": "deterministic", "modelName": "integration-hash", "dimension": 256, "batchSize": 16, "normalized": true, "status": "ACTIVE"},
		"vector-stores":      {"id": "vector_integration", "tenantId": access.TenantID, "name": "Integration pgvector", "providerKey": "pgvector", "collectionPrefix": "integration", "distanceMetric": "COSINE", "status": "ACTIVE"},
		"ingestion-profiles": {"id": "ingestion_integration", "tenantId": access.TenantID, "name": "Integration Ingestion", "embeddingProfileId": "embedding_integration", "vectorStoreProfileId": "vector_integration", "parserKey": "auto", "chunkerKey": "heading", "chunkSize": 120, "overlap": 10, "minTokens": 1, "maxTokens": 300, "status": "ACTIVE"},
		"retrieval-profiles": {"id": "retrieval_integration", "tenantId": access.TenantID, "name": "Integration Retrieval", "searchMode": "HYBRID", "topK": 5, "threshold": 0.05, "vectorWeight": 0.7, "keywordWeight": 0.3, "contextTokenLimit": 6000, "queryRewriteEnabled": true, "metadataFilterEnabled": true, "status": "ACTIVE"},
	} {
		if _, err := repo.SaveKnowledgeAdminProfile(ctx, resource, profile); err != nil {
			t.Fatalf("save %s: %v", resource, err)
		}
	}
	resolvedProfile, err := repo.ResolveIngestionRuntimeProfile(ctx, access, "ingestion_integration")
	if err != nil || resolvedProfile.Embedding.ID != "embedding_integration" || resolvedProfile.VectorStore.ID != "vector_integration" {
		t.Fatalf("runtime profile not resolved: %#v err=%v", resolvedProfile, err)
	}
	base, err := core.CreateKnowledgeBase(ctx, access, knowledgeapp.CreateKnowledgeBaseInput{
		Name: "Postgres 产品知识库", KnowledgeType: knowledgeapp.KnowledgePersonal,
		IngestionProfileID: "ingestion_integration", RetrievalProfileID: "retrieval_integration",
	})
	if err != nil {
		t.Fatal(err)
	}
	embedder := embedding.NewDeterministic(256)
	vectors := vectorstore.NewPGVector(db)
	ingestion := knowledgeapp.NewIngestionService(core, repo, parser.NewDefaultRegistry(), chunker.NewDefaultRegistry(), embedder, vectors)
	runtimeResolver := knowledgeprofile.NewResolver(repo, vectorstore.NewRegistry(vectors), "", "", 30000)
	ingestion.SetRuntimeResolver(runtimeResolver)
	ingested, err := ingestion.Ingest(ctx, access, base.ID, knowledgeapp.IngestDocumentInput{
		Name: "Postgres 企业版手册.md", MIMEType: "text/markdown", Content: []byte("# 成员配额\n企业版默认支持 100 个成员，并支持部门知识库权限隔离。"),
		ChunkerKey: "heading", ChunkOptions: knowledgeapp.ChunkOptions{ChunkSize: 120, Overlap: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ingested.Document.Status != "READY" {
		t.Fatalf("document status = %s", ingested.Document.Status)
	}
	agents := knowledgeapp.NewAgentService(core, repo)
	agent, err := agents.CreateAgent(ctx, access, knowledgeapp.CreateAgentInput{Name: "Postgres 产品顾问", Status: "ACTIVE"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agents.ReplaceBindings(ctx, access, agent.ID, []knowledgeapp.BindKnowledgeBaseInput{{KnowledgeBaseID: base.ID}}); err != nil {
		t.Fatal(err)
	}
	conversation, err := agents.CreateConversation(ctx, access, agent.ID, "Postgres 咨询")
	if err != nil {
		t.Fatal(err)
	}
	retrieval := knowledgeapp.NewRetrievalService(embedder, vectors, rerank.None{})
	retrieval.SetRuntimeResolver(core, runtimeResolver)
	rag := knowledgeapp.NewRAGService(repo, repo, retrieval, queryrewrite.Latest{}, answerprovider.Context{})
	result, err := rag.Run(ctx, access, knowledgeapp.RunInput{ConversationID: conversation.ID, Question: "企业版支持多少成员？", Mode: "HYBRID", TopK: 5, Threshold: 0.05})
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.Status != "COMPLETED" || result.Run.InputTokens <= 0 || result.Run.OutputTokens <= 0 || result.Run.PointCost <= 0 || len(result.Citations) == 0 || result.Citations[0].DocumentID != ingested.Document.ID {
		t.Fatalf("unexpected result: %#v", result)
	}
	var runCount, citationCount int
	if err := db.QueryRowContext(ctx, `select count(*) from xz_rag_runs where tenant_id=$1 and id=$2`, access.TenantID, result.Run.ID).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `select count(*) from xz_rag_citations where tenant_id=$1 and rag_run_id=$2`, access.TenantID, result.Run.ID).Scan(&citationCount); err != nil {
		t.Fatal(err)
	}
	if runCount != 1 || citationCount == 0 {
		t.Fatalf("persistent run=%d citations=%d", runCount, citationCount)
	}
	overview, err := repo.KnowledgeAdminOverview(ctx, access.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	if overview.KnowledgeBaseCount != 1 || overview.RAGRunCount != 1 || overview.ChunkCount == 0 {
		t.Fatalf("unexpected admin overview: %#v", overview)
	}
	for _, resource := range []string{"bases", "documents", "chunks", "agents", "parsing-logs", "embedding-profiles", "vector-stores", "ingestion-profiles", "retrieval-profiles", "retrieval-logs", "usage", "hot-questions"} {
		if _, err := repo.ListKnowledgeAdminRecords(ctx, resource, access.TenantID, knowledgeapp.ListOptions{Limit: 20}); err != nil {
			t.Fatalf("admin resource %s: %v", resource, err)
		}
	}
	if err := ingestion.DeleteDocument(ctx, access, ingested.Document.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetDocument(ctx, access, ingested.Document.ID); !errors.Is(err, knowledgeapp.ErrNotFound) {
		t.Fatalf("deleted document should not be readable: %v", err)
	}
}
