package knowledge

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type RetrievalService struct {
	embedder Embedder
	vectors  VectorStore
	reranker Reranker
	core     *Service
	runtime  IngestionRuntimeResolver
}

func (s *RetrievalService) SetRuntimeResolver(core *Service, resolver IngestionRuntimeResolver) {
	if s != nil {
		s.core, s.runtime = core, resolver
	}
}

func NewRetrievalService(embedder Embedder, vectors VectorStore, reranker Reranker) *RetrievalService {
	return &RetrievalService{embedder: embedder, vectors: vectors, reranker: reranker}
}

func (s *RetrievalService) Search(ctx context.Context, request SearchRequest) ([]RetrievalHit, error) {
	if s == nil || s.embedder == nil || s.vectors == nil {
		return nil, fmt.Errorf("knowledge retrieval service is not configured: %w", ErrValidation)
	}
	request.Query = strings.TrimSpace(request.Query)
	if request.Query == "" || request.Access.TenantID == "" || request.Access.UserID == "" {
		return nil, fmt.Errorf("query and access context are required: %w", ErrValidation)
	}
	if len(request.KnowledgeBaseIDs) == 0 {
		return nil, fmt.Errorf("at least one knowledge base is required: %w", ErrValidation)
	}
	explicitMode := strings.TrimSpace(request.Mode) != ""
	explicitTopK := request.TopK > 0 && request.TopK <= 50
	explicitThreshold := request.Threshold > 0
	explicitWeights := request.VectorWeight != 0 || request.KeywordWeight != 0
	if request.Mode == "" {
		request.Mode = "HYBRID"
	}
	if request.TopK <= 0 || request.TopK > 50 {
		request.TopK = 8
	}
	if request.Threshold <= 0 {
		request.Threshold = 0.2
	}
	if request.VectorWeight == 0 && request.KeywordWeight == 0 {
		request.VectorWeight, request.KeywordWeight = 0.7, 0.3
	}
	hits, err := s.searchConfiguredRuntimes(ctx, request, explicitMode, explicitTopK, explicitThreshold, explicitWeights)
	if err != nil {
		return nil, err
	}
	if s.reranker != nil {
		return s.reranker.Rerank(ctx, request.Query, hits, request.TopK)
	}
	return hits, nil
}

func (s *RetrievalService) searchConfiguredRuntimes(ctx context.Context, request SearchRequest, explicitMode bool, explicitTopK bool, explicitThreshold bool, explicitWeights bool) ([]RetrievalHit, error) {
	if s.runtime == nil || s.core == nil {
		return searchOneRuntime(ctx, request, s.embedder, s.vectors)
	}
	type runtimeGroup struct {
		selection IngestionRuntimeSelection
		baseIDs   []string
	}
	groups := map[string]*runtimeGroup{}
	for _, baseID := range request.KnowledgeBaseIDs {
		base, err := s.core.GetKnowledgeBase(ctx, request.Access, baseID)
		if err != nil {
			return nil, err
		}
		if override := strings.TrimSpace(request.RetrievalProfileIDs[baseID]); override != "" {
			base.RetrievalProfileID = override
		}
		selection, err := s.runtime.ResolveIngestionRuntime(ctx, request.Access, base)
		if err != nil {
			return nil, fmt.Errorf("resolve retrieval runtime for %s: %w", baseID, err)
		}
		key := selection.Profile.Embedding.ID + "\x00" + selection.Profile.VectorStore.ID + "\x00" + selection.Retrieval.ID
		group := groups[key]
		if group == nil {
			group = &runtimeGroup{selection: selection}
			groups[key] = group
		}
		group.baseIDs = append(group.baseIDs, baseID)
	}
	combined := make([]RetrievalHit, 0, request.TopK*len(groups))
	combinedLimit := request.TopK
	for _, group := range groups {
		groupRequest := request
		groupRequest.KnowledgeBaseIDs = group.baseIDs
		profile := group.selection.Retrieval
		if !explicitMode && profile.SearchMode != "" {
			groupRequest.Mode = profile.SearchMode
		}
		if !explicitTopK && profile.TopK > 0 {
			groupRequest.TopK = profile.TopK
			if profile.TopK > combinedLimit {
				combinedLimit = profile.TopK
			}
		}
		if !explicitThreshold && profile.Threshold > 0 {
			groupRequest.Threshold = profile.Threshold
		}
		if !explicitWeights && (profile.VectorWeight != 0 || profile.KeywordWeight != 0) {
			groupRequest.VectorWeight, groupRequest.KeywordWeight = profile.VectorWeight, profile.KeywordWeight
		}
		if !profile.MetadataFilterEnabled {
			groupRequest.Filters = nil
		}
		items, err := searchOneRuntime(ctx, groupRequest, group.selection.Embedder, group.selection.VectorStore)
		if err != nil {
			return nil, err
		}
		if group.selection.Reranker != nil {
			items, err = group.selection.Reranker.Rerank(ctx, groupRequest.Query, items, groupRequest.TopK)
			if err != nil {
				return nil, err
			}
		}
		combined = append(combined, items...)
	}
	sort.SliceStable(combined, func(i, j int) bool { return combined[i].FinalScore > combined[j].FinalScore })
	if len(combined) > combinedLimit {
		combined = combined[:combinedLimit]
	}
	for index := range combined {
		combined[index].FinalRank = index + 1
	}
	return combined, nil
}

func searchOneRuntime(ctx context.Context, request SearchRequest, embedder Embedder, vectors VectorStore) ([]RetrievalHit, error) {
	if embedder == nil || vectors == nil {
		return nil, fmt.Errorf("retrieval runtime is incomplete: %w", ErrValidation)
	}
	embeddings, err := embedder.Embed(ctx, []string{request.Query})
	if err != nil {
		return nil, err
	}
	if len(embeddings) != 1 {
		return nil, fmt.Errorf("query embedding returned %d vectors", len(embeddings))
	}
	return vectors.Search(ctx, request, embeddings[0])
}
