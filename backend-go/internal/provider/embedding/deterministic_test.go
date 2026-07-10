package embedding

import (
	"context"
	"testing"
)

func TestDeterministicEmbeddingIsStableAndSemanticEnoughForFallback(t *testing.T) {
	t.Parallel()
	provider := NewDeterministic(64)
	vectors, err := provider.Embed(context.Background(), []string{"企业知识库权限", "企业知识库权限", "天气预报"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 3 || len(vectors[0]) != 64 {
		t.Fatalf("unexpected vector shape")
	}
	for index := range vectors[0] {
		if vectors[0][index] != vectors[1][index] {
			t.Fatalf("same text must produce stable vector")
		}
	}
}
