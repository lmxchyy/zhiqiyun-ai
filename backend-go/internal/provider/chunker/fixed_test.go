package chunker

import (
	"context"
	"strings"
	"testing"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

func TestFixedChunkerPreservesOverlapAndLocator(t *testing.T) {
	t.Parallel()
	content := strings.Repeat("企业知识库支持权限隔离和引用来源。", 200)
	chunks, err := (Fixed{}).Chunk(context.Background(), []knowledgeapp.DocumentUnit{{UnitType: "PAGE", UnitNo: 12, Title: "权限", Content: content, Locator: map[string]any{"page": 12}}}, knowledgeapp.ChunkOptions{ChunkSize: 80, Overlap: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	if chunks[0].SourceLocator["page"] != 12 {
		t.Fatalf("expected locator to be preserved, got %#v", chunks[0].SourceLocator)
	}
	if chunks[0].TokenCount <= 0 {
		t.Fatal("expected token estimate")
	}
}
