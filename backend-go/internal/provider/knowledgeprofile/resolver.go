package knowledgeprofile

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
	"xianzhi-ai/backend-go/internal/provider/embedding"
	"xianzhi-ai/backend-go/internal/provider/rerank"
	"xianzhi-ai/backend-go/internal/provider/vectorstore"
)

type Resolver struct {
	profiles  knowledgeapp.RuntimeProfileRepository
	vectors   *vectorstore.Registry
	baseURL   string
	apiKey    string
	timeoutMS int
}

func NewResolver(profiles knowledgeapp.RuntimeProfileRepository, vectors *vectorstore.Registry, baseURL string, apiKey string, timeoutMS int) *Resolver {
	return &Resolver{profiles: profiles, vectors: vectors, baseURL: strings.TrimSpace(baseURL), apiKey: strings.TrimSpace(apiKey), timeoutMS: timeoutMS}
}

func (r *Resolver) ResolveIngestionRuntime(ctx context.Context, access knowledgeapp.AccessContext, base knowledgeapp.KnowledgeBase) (knowledgeapp.IngestionRuntimeSelection, error) {
	if r == nil || r.profiles == nil || r.vectors == nil {
		return knowledgeapp.IngestionRuntimeSelection{}, fmt.Errorf("knowledge profile resolver is not configured")
	}
	profile, err := r.profiles.ResolveIngestionRuntimeProfile(ctx, access, base.IngestionProfileID)
	if err != nil {
		return knowledgeapp.IngestionRuntimeSelection{}, err
	}
	retrievalProfile, err := r.profiles.ResolveRetrievalRuntimeProfile(ctx, access, base.RetrievalProfileID)
	if err != nil {
		return knowledgeapp.IngestionRuntimeSelection{}, err
	}
	embedder, err := embedding.NewFromProfile(embedding.Profile{
		ProviderKey: profile.Embedding.ProviderKey,
		BaseURL:     firstString(profile.Embedding.Config, r.baseURL, "baseUrl", "endpoint"),
		APIKey:      firstString(profile.Embedding.Config, r.apiKey, "apiKey"),
		Model:       profile.Embedding.ModelName, Dimension: profile.Embedding.Dimension,
		TimeoutMS: firstInt(profile.Embedding.Config, r.timeoutMS, "timeoutMs"),
	})
	if err != nil {
		return knowledgeapp.IngestionRuntimeSelection{}, err
	}
	vectors, err := r.vectors.Get(profile.VectorStore.ProviderKey)
	if err != nil {
		return knowledgeapp.IngestionRuntimeSelection{}, err
	}
	var runtimeReranker knowledgeapp.Reranker = rerank.None{}
	if retrievalProfile.Rerank != nil && !strings.EqualFold(retrievalProfile.Rerank.ProviderKey, "none") {
		runtimeReranker = rerank.NewHTTP(rerank.HTTPOptions{
			Code:      retrievalProfile.Rerank.ProviderKey,
			BaseURL:   firstString(retrievalProfile.Rerank.Config, r.baseURL, "baseUrl", "endpoint"),
			APIKey:    firstString(retrievalProfile.Rerank.Config, r.apiKey, "apiKey"),
			Model:     retrievalProfile.Rerank.ModelName,
			TimeoutMS: firstInt(retrievalProfile.Rerank.Config, r.timeoutMS, "timeoutMs"),
		})
	}
	return knowledgeapp.IngestionRuntimeSelection{Profile: profile, Retrieval: retrievalProfile, Embedder: embedder, VectorStore: vectors, Reranker: runtimeReranker}, nil
}

func firstString(config map[string]any, fallback string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(fmt.Sprint(config[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return fallback
}

func firstInt(config map[string]any, fallback int, keys ...string) int {
	for _, key := range keys {
		value, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(config[key])))
		if err == nil && value > 0 {
			return value
		}
	}
	return fallback
}

var _ knowledgeapp.IngestionRuntimeResolver = (*Resolver)(nil)
