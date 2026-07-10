package knowledgerepo

import (
	"context"
	"fmt"
	"strings"
	"time"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

func (p *Postgres) SaveKnowledgeAdminProfile(ctx context.Context, resource string, input map[string]any) (map[string]any, error) {
	resource = strings.ToLower(strings.TrimSpace(resource))
	id, name := profileString(input, "id"), profileString(input, "name")
	if id == "" || name == "" {
		return nil, fmt.Errorf("profile id and name are required: %w", knowledgeapp.ErrValidation)
	}
	tenantID := nullableText(profileString(input, "tenantId"))
	status := strings.ToUpper(profileString(input, "status"))
	if status == "" {
		status = "ACTIVE"
	}
	now := time.Now().UTC()
	var err error
	switch resource {
	case "embedding-profiles":
		provider, model := profileString(input, "providerKey"), profileString(input, "modelName")
		dimension := profileInt(input, "dimension", 256)
		if provider == "" || model == "" || dimension <= 0 {
			return nil, fmt.Errorf("provider, model and dimension are required: %w", knowledgeapp.ErrValidation)
		}
		_, err = p.db.ExecContext(ctx, `
			insert into xz_knowledge_embedding_profiles (id,tenant_id,name,provider_key,model_name,dimension,batch_size,normalized,status,config,updated_at)
			values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11)
			on conflict (id) do update set tenant_id=excluded.tenant_id,name=excluded.name,provider_key=excluded.provider_key,
				model_name=excluded.model_name,dimension=excluded.dimension,batch_size=excluded.batch_size,normalized=excluded.normalized,
				status=excluded.status,config=excluded.config,version=xz_knowledge_embedding_profiles.version+1,updated_at=excluded.updated_at
		`, id, tenantID, name, provider, model, dimension, profileInt(input, "batchSize", 32), profileBool(input, "normalized", true), status, jsonText(profileMap(input, "config")), now)
	case "vector-stores":
		provider := profileString(input, "providerKey")
		if provider == "" {
			return nil, fmt.Errorf("vector provider is required: %w", knowledgeapp.ErrValidation)
		}
		_, err = p.db.ExecContext(ctx, `
			insert into xz_knowledge_vector_store_profiles (id,tenant_id,name,provider_key,endpoint,credential_ref,collection_prefix,distance_metric,status,config,updated_at)
			values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11)
			on conflict (id) do update set tenant_id=excluded.tenant_id,name=excluded.name,provider_key=excluded.provider_key,
				endpoint=excluded.endpoint,credential_ref=excluded.credential_ref,collection_prefix=excluded.collection_prefix,
				distance_metric=excluded.distance_metric,status=excluded.status,config=excluded.config,updated_at=excluded.updated_at
		`, id, tenantID, name, provider, nullableText(profileString(input, "endpoint")), nullableText(profileString(input, "credentialRef")),
			profileDefaultString(input, "collectionPrefix", "xianzhi_kb"), profileDefaultString(input, "distanceMetric", "COSINE"), status, jsonText(profileMap(input, "config")), now)
	case "ingestion-profiles":
		embeddingID, vectorID := profileString(input, "embeddingProfileId"), profileString(input, "vectorStoreProfileId")
		if embeddingID == "" || vectorID == "" {
			return nil, fmt.Errorf("embedding and vector profiles are required: %w", knowledgeapp.ErrValidation)
		}
		_, err = p.db.ExecContext(ctx, `
			insert into xz_knowledge_ingestion_profiles (id,tenant_id,embedding_profile_id,vector_store_profile_id,name,parser_key,ocr_provider_key,chunker_key,chunk_size,overlap,min_tokens,max_tokens,status,cleaning_config,updated_at)
			values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb,$15)
			on conflict (id) do update set tenant_id=excluded.tenant_id,embedding_profile_id=excluded.embedding_profile_id,
				vector_store_profile_id=excluded.vector_store_profile_id,name=excluded.name,parser_key=excluded.parser_key,
				ocr_provider_key=excluded.ocr_provider_key,chunker_key=excluded.chunker_key,chunk_size=excluded.chunk_size,
				overlap=excluded.overlap,min_tokens=excluded.min_tokens,max_tokens=excluded.max_tokens,status=excluded.status,
				cleaning_config=excluded.cleaning_config,version=xz_knowledge_ingestion_profiles.version+1,updated_at=excluded.updated_at
		`, id, tenantID, embeddingID, vectorID, name, profileDefaultString(input, "parserKey", "auto"), nullableText(profileString(input, "ocrProviderKey")),
			profileDefaultString(input, "chunkerKey", "fixed"), profileInt(input, "chunkSize", 800), profileInt(input, "overlap", 120),
			profileInt(input, "minTokens", 40), profileInt(input, "maxTokens", 1200), status, jsonText(profileMap(input, "cleaningConfig")), now)
	case "retrieval-profiles":
		_, err = p.db.ExecContext(ctx, `
			insert into xz_knowledge_retrieval_profiles (id,tenant_id,name,search_mode,top_k,threshold,vector_weight,keyword_weight,rerank_profile_id,context_token_limit,query_rewrite_enabled,metadata_filter_enabled,status,config,updated_at)
			values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb,$15)
			on conflict (id) do update set tenant_id=excluded.tenant_id,name=excluded.name,search_mode=excluded.search_mode,
				top_k=excluded.top_k,threshold=excluded.threshold,vector_weight=excluded.vector_weight,keyword_weight=excluded.keyword_weight,
				rerank_profile_id=excluded.rerank_profile_id,context_token_limit=excluded.context_token_limit,
				query_rewrite_enabled=excluded.query_rewrite_enabled,metadata_filter_enabled=excluded.metadata_filter_enabled,
				status=excluded.status,config=excluded.config,updated_at=excluded.updated_at
		`, id, tenantID, name, profileDefaultString(input, "searchMode", "HYBRID"), profileInt(input, "topK", 8),
			profileFloat(input, "threshold", 0.2), profileFloat(input, "vectorWeight", 0.7), profileFloat(input, "keywordWeight", 0.3),
			nullableText(profileString(input, "rerankProfileId")), profileInt(input, "contextTokenLimit", 6000),
			profileBool(input, "queryRewriteEnabled", true), profileBool(input, "metadataFilterEnabled", true), status, jsonText(profileMap(input, "config")), now)
	default:
		return nil, knowledgeapp.ErrValidation
	}
	if err != nil {
		return nil, err
	}
	items, err := p.ListKnowledgeAdminRecords(ctx, resource, profileString(input, "tenantId"), knowledgeapp.ListOptions{Limit: 200})
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if profileString(item, "id") == id {
			return item, nil
		}
	}
	return nil, knowledgeapp.ErrNotFound
}

func profileString(input map[string]any, key string) string {
	value, exists := input[key]
	if !exists || value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "<nil>" {
		return ""
	}
	return text
}
func profileDefaultString(input map[string]any, key string, fallback string) string {
	if value := profileString(input, key); value != "" && value != "<nil>" {
		return value
	}
	return fallback
}
func profileInt(input map[string]any, key string, fallback int) int {
	if value, ok := input[key].(float64); ok && int(value) > 0 {
		return int(value)
	}
	if value, ok := input[key].(int); ok && value > 0 {
		return value
	}
	return fallback
}
func profileFloat(input map[string]any, key string, fallback float64) float64 {
	if value, ok := input[key].(float64); ok {
		return value
	}
	if value, ok := input[key].(int); ok {
		return float64(value)
	}
	return fallback
}
func profileBool(input map[string]any, key string, fallback bool) bool {
	if value, ok := input[key].(bool); ok {
		return value
	}
	return fallback
}
func profileMap(input map[string]any, key string) map[string]any {
	if value, ok := input[key].(map[string]any); ok {
		return value
	}
	return map[string]any{}
}
