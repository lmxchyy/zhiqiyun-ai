package rerank

import (
	"context"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type None struct{}

func (None) Code() string { return "none" }

func (None) Rerank(_ context.Context, _ string, hits []knowledgeapp.RetrievalHit, limit int) ([]knowledgeapp.RetrievalHit, error) {
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	for index := range hits {
		hits[index].FinalRank = index + 1
	}
	return hits, nil
}
