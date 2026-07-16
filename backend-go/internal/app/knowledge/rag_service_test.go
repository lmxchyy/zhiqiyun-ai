package knowledge_test

import (
	"context"
	"sync"
	"testing"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
	answerprovider "xianzhi-ai/backend-go/internal/provider/answer"
	"xianzhi-ai/backend-go/internal/provider/chunker"
	"xianzhi-ai/backend-go/internal/provider/embedding"
	"xianzhi-ai/backend-go/internal/provider/parser"
	"xianzhi-ai/backend-go/internal/provider/queryrewrite"
	"xianzhi-ai/backend-go/internal/provider/rerank"
	"xianzhi-ai/backend-go/internal/provider/vectorstore"
	knowledgerepo "xianzhi-ai/backend-go/internal/repository/knowledge"
)

type billingRecorder struct {
	mu    sync.Mutex
	items []knowledgeapp.RAGBillingUsage
}

func (r *billingRecorder) RecordRAGUsage(_ context.Context, usage knowledgeapp.RAGBillingUsage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, usage)
	return nil
}

func TestAgentRAGRunPersistsMessagesEventsAndCitations(t *testing.T) {
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
	if _, err := ingestion.Ingest(ctx, access, base.ID, knowledgeapp.IngestDocumentInput{
		Name: "企业版手册.md", MIMEType: "text/markdown", Content: []byte("# 成员配额\n\n企业版默认支持 100 个成员，并支持部门级知识库权限隔离。"),
		ChunkerKey: "heading", ChunkOptions: knowledgeapp.ChunkOptions{ChunkSize: 100, Overlap: 10},
	}); err != nil {
		t.Fatal(err)
	}
	agents := knowledgeapp.NewAgentService(core, repo)
	agent, err := agents.CreateAgent(ctx, access, knowledgeapp.CreateAgentInput{Name: "产品顾问", Status: "ACTIVE"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agents.ReplaceBindings(ctx, access, agent.ID, []knowledgeapp.BindKnowledgeBaseInput{{KnowledgeBaseID: base.ID}}); err != nil {
		t.Fatal(err)
	}
	conversation, err := agents.CreateConversation(ctx, access, agent.ID, "企业版咨询")
	if err != nil {
		t.Fatal(err)
	}
	retrieval := knowledgeapp.NewRetrievalService(embedder, vectors, rerank.None{})
	rag := knowledgeapp.NewRAGService(repo, repo, retrieval, queryrewrite.Latest{}, answerprovider.Context{})
	billing := &billingRecorder{}
	rag.SetBillingRecorder(billing)
	result, err := rag.Run(ctx, access, knowledgeapp.RunInput{
		ConversationID: conversation.ID, Question: "企业版支持多少成员？", Mode: "HYBRID", TopK: 5, Threshold: 0.05,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.Status != "COMPLETED" || result.Message.Role != "assistant" || len(result.Citations) == 0 {
		t.Fatalf("unexpected RAG result: %#v", result)
	}
	if result.Run.InputTokens <= 0 || result.Run.OutputTokens <= 0 || result.Run.PointCost <= 0 {
		t.Fatalf("RAG usage was not metered: %#v", result.Run)
	}
	if len(billing.items) != 1 || billing.items[0].RunID != result.Run.ID || billing.items[0].PointCost != result.Run.PointCost {
		t.Fatalf("RAG billing event was not recorded: %#v", billing.items)
	}
	if result.Citations[0].DocumentName != "企业版手册.md" || result.Citations[0].ChunkID == "" {
		t.Fatalf("citation is not traceable: %#v", result.Citations[0])
	}
	events, err := rag.ListEvents(ctx, access, result.Run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 5 || events[len(events)-1].EventType != "run.completed" {
		t.Fatalf("unexpected run events: %#v", events)
	}
	messages, _, err := agents.ListMessages(ctx, access, conversation.ID, knowledgeapp.ListOptions{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Role != "user" || messages[1].Role != "assistant" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if !messages[1].CreatedAt.After(messages[0].CreatedAt) {
		t.Fatalf("assistant message timestamp must follow its parent: %#v", messages)
	}
}
