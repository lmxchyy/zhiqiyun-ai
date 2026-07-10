package knowledgerepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

func (p *Postgres) ResolveIngestionRuntimeProfile(ctx context.Context, access knowledgeapp.AccessContext, id string) (knowledgeapp.IngestionRuntimeProfile, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		id = "ingestion_default"
	}
	var profile knowledgeapp.IngestionRuntimeProfile
	var embeddingConfig, vectorConfig []byte
	err := p.db.QueryRowContext(ctx, `
		select i.id, i.parser_key, coalesce(i.ocr_provider_key,''), i.chunker_key, i.chunk_size, i.overlap, i.min_tokens, i.max_tokens,
			e.id, e.provider_key, e.model_name, e.dimension, e.config,
			v.id, v.provider_key, coalesce(v.endpoint,''), coalesce(v.credential_ref,''), v.collection_prefix, v.config
		from xz_knowledge_ingestion_profiles i
		join xz_knowledge_embedding_profiles e on e.id=i.embedding_profile_id and e.status='ACTIVE'
		join xz_knowledge_vector_store_profiles v on v.id=i.vector_store_profile_id and v.status='ACTIVE'
		where i.id=$1 and i.status='ACTIVE'
		  and (i.tenant_id is null or i.tenant_id=$2)
		  and (e.tenant_id is null or e.tenant_id=$2)
		  and (v.tenant_id is null or v.tenant_id=$2)
	`, id, access.TenantID).Scan(
		&profile.ID, &profile.ParserKey, &profile.OCRProviderKey, &profile.ChunkerKey,
		&profile.ChunkOptions.ChunkSize, &profile.ChunkOptions.Overlap, &profile.ChunkOptions.MinTokens, &profile.ChunkOptions.MaxTokens,
		&profile.Embedding.ID, &profile.Embedding.ProviderKey, &profile.Embedding.ModelName, &profile.Embedding.Dimension, &embeddingConfig,
		&profile.VectorStore.ID, &profile.VectorStore.ProviderKey, &profile.VectorStore.Endpoint, &profile.VectorStore.CredentialRef,
		&profile.VectorStore.CollectionPrefix, &vectorConfig,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return knowledgeapp.IngestionRuntimeProfile{}, knowledgeapp.ErrNotFound
	}
	if err != nil {
		return knowledgeapp.IngestionRuntimeProfile{}, err
	}
	_ = json.Unmarshal(embeddingConfig, &profile.Embedding.Config)
	_ = json.Unmarshal(vectorConfig, &profile.VectorStore.Config)
	if profile.Embedding.Config == nil {
		profile.Embedding.Config = map[string]any{}
	}
	if profile.VectorStore.Config == nil {
		profile.VectorStore.Config = map[string]any{}
	}
	return profile, nil
}

var _ knowledgeapp.RuntimeProfileRepository = (*Postgres)(nil)

func (p *Postgres) ResolveRetrievalRuntimeProfile(ctx context.Context, access knowledgeapp.AccessContext, id string) (knowledgeapp.RetrievalRuntimeProfile, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		id = "retrieval_default"
	}
	var profile knowledgeapp.RetrievalRuntimeProfile
	var config []byte
	err := p.db.QueryRowContext(ctx, `
		select id, search_mode, top_k, threshold, vector_weight, keyword_weight, coalesce(rerank_profile_id,''),
			context_token_limit, query_rewrite_enabled, metadata_filter_enabled, config
		from xz_knowledge_retrieval_profiles
		where id=$1 and status='ACTIVE' and (tenant_id is null or tenant_id=$2)
	`, id, access.TenantID).Scan(&profile.ID, &profile.SearchMode, &profile.TopK, &profile.Threshold, &profile.VectorWeight,
		&profile.KeywordWeight, &profile.RerankProfileID, &profile.ContextTokenLimit, &profile.QueryRewriteEnabled,
		&profile.MetadataFilterEnabled, &config)
	if errors.Is(err, sql.ErrNoRows) {
		return knowledgeapp.RetrievalRuntimeProfile{}, knowledgeapp.ErrNotFound
	}
	if err != nil {
		return knowledgeapp.RetrievalRuntimeProfile{}, err
	}
	_ = json.Unmarshal(config, &profile.Config)
	if profile.Config == nil {
		profile.Config = map[string]any{}
	}
	if profile.RerankProfileID != "" {
		var rerank knowledgeapp.RerankRuntimeProfile
		var rerankConfig []byte
		err = p.db.QueryRowContext(ctx, `
			select id, provider_key, model_name, candidate_limit, config
			from xz_knowledge_rerank_profiles
			where id=$1 and status='ACTIVE' and (tenant_id is null or tenant_id=$2)
		`, profile.RerankProfileID, access.TenantID).Scan(&rerank.ID, &rerank.ProviderKey, &rerank.ModelName, &rerank.CandidateLimit, &rerankConfig)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return knowledgeapp.RetrievalRuntimeProfile{}, knowledgeapp.ErrNotFound
			}
			return knowledgeapp.RetrievalRuntimeProfile{}, err
		}
		_ = json.Unmarshal(rerankConfig, &rerank.Config)
		if rerank.Config == nil {
			rerank.Config = map[string]any{}
		}
		profile.Rerank = &rerank
	}
	return profile, nil
}
