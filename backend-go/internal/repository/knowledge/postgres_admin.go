package knowledgerepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

func (p *Postgres) KnowledgeAdminOverview(ctx context.Context, tenantID string) (knowledgeapp.AdminOverview, error) {
	var item knowledgeapp.AdminOverview
	err := p.db.QueryRowContext(ctx, `
		select
			(select count(*) from xz_tenants where ($1='' or id=$1) and status='ACTIVE'),
			(select count(*) from xz_knowledge_bases where ($1='' or tenant_id=$1) and deleted_at is null),
			(select count(*) from xz_knowledge_documents where ($1='' or tenant_id=$1) and deleted_at is null),
			(select count(*) from xz_knowledge_chunks where ($1='' or tenant_id=$1) and deleted_at is null),
			(select count(*) from xz_knowledge_documents where ($1='' or tenant_id=$1) and deleted_at is null and status='READY'),
			(select count(*) from xz_knowledge_documents where ($1='' or tenant_id=$1) and deleted_at is null and status='FAILED'),
			(select count(*) from xz_ai_agents where ($1='' or tenant_id=$1) and deleted_at is null),
			(select count(*) from xz_rag_runs where ($1='' or tenant_id=$1)),
			(select count(*) from xz_rag_runs where ($1='' or tenant_id=$1) and status='COMPLETED'),
			(select coalesce(sum(input_tokens),0) from xz_rag_runs where ($1='' or tenant_id=$1)),
			(select coalesce(sum(output_tokens),0) from xz_rag_runs where ($1='' or tenant_id=$1)),
			(select coalesce(sum(point_cost),0) from xz_rag_runs where ($1='' or tenant_id=$1))
	`, strings.TrimSpace(tenantID)).Scan(&item.TenantCount, &item.KnowledgeBaseCount, &item.DocumentCount, &item.ChunkCount,
		&item.ReadyDocumentCount, &item.FailedDocumentCount, &item.AgentCount, &item.RAGRunCount, &item.CompletedRAGRunCount,
		&item.InputTokens, &item.OutputTokens, &item.PointCost)
	return item, err
}

func (p *Postgres) ListKnowledgeAdminRecords(ctx context.Context, resource string, tenantID string, options knowledgeapp.ListOptions) ([]map[string]any, error) {
	limit := normalizedLimit(options.Limit)
	query, args := knowledgeAdminQuery(strings.ToLower(strings.TrimSpace(resource)), strings.TrimSpace(tenantID), limit)
	if query == "" {
		return nil, knowledgeapp.ErrValidation
	}
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAdminRows(rows)
}

func knowledgeAdminQuery(resource string, tenantID string, limit int) (string, []any) {
	args := []any{tenantID, limit}
	switch resource {
	case "bases":
		return `select id, tenant_id, organization_id, owner_user_id, knowledge_type, name, description, visibility, status, document_count, chunk_count, ingestion_profile_id, retrieval_profile_id, version, created_at, updated_at from xz_knowledge_bases where ($1='' or tenant_id=$1) and deleted_at is null order by updated_at desc limit $2`, args
	case "documents":
		return `select id, tenant_id, knowledge_base_id, owner_user_id, name, document_type, mime_type, status, latest_version_id, version, created_at, updated_at from xz_knowledge_documents where ($1='' or tenant_id=$1) and deleted_at is null order by updated_at desc limit $2`, args
	case "chunks":
		return `select id, tenant_id, knowledge_base_id, document_id, document_version_id, sequence_no, title, token_count, page_start, page_end, status, created_at, updated_at from xz_knowledge_chunks where ($1='' or tenant_id=$1) and deleted_at is null order by updated_at desc limit $2`, args
	case "agents":
		return `select id, tenant_id, owner_user_id, name, description, model_name, status, version, created_at, updated_at from xz_ai_agents where ($1='' or tenant_id=$1) and deleted_at is null order by updated_at desc limit $2`, args
	case "ingestion-jobs", "parsing-logs":
		return `select j.id, j.tenant_id, v.document_id, j.document_version_id, j.ingestion_profile_id, j.stage, j.status, j.attempt, j.progress, j.error_code, j.error_message, j.created_at, j.updated_at from xz_knowledge_ingestion_jobs j join xz_knowledge_document_versions v on v.tenant_id=j.tenant_id and v.id=j.document_version_id where ($1='' or j.tenant_id=$1) order by j.updated_at desc limit $2`, args
	case "rag-runs", "retrieval-logs":
		return `select id, tenant_id, conversation_id, agent_id, original_query, rewritten_query, status, retrieval_latency_ms, generation_latency_ms, input_tokens, output_tokens, point_cost, error_code, error_message, created_at, updated_at from xz_rag_runs where ($1='' or tenant_id=$1) order by created_at desc limit $2`, args
	case "usage":
		return `select tenant_id, date_trunc('day', created_at) as usage_day, count(*) as run_count, sum(input_tokens) as input_tokens, sum(output_tokens) as output_tokens, sum(point_cost) as point_cost from xz_rag_runs where ($1='' or tenant_id=$1) group by tenant_id, date_trunc('day', created_at) order by usage_day desc limit $2`, args
	case "hot-questions":
		return `select tenant_id, original_query as question, count(*) as ask_count, avg(retrieval_latency_ms)::bigint as avg_retrieval_latency_ms, max(created_at) as last_asked_at from xz_rag_runs where ($1='' or tenant_id=$1) group by tenant_id, original_query order by ask_count desc, last_asked_at desc limit $2`, args
	case "embedding-profiles":
		return `select id, tenant_id, name, provider_key, model_name, dimension, batch_size, normalized, status, version, created_at, updated_at from xz_knowledge_embedding_profiles where ($1='' or tenant_id=$1 or tenant_id is null) order by tenant_id nulls first, updated_at desc limit $2`, args
	case "vector-stores":
		return `select id, tenant_id, name, provider_key, endpoint, collection_prefix, distance_metric, status, created_at, updated_at from xz_knowledge_vector_store_profiles where ($1='' or tenant_id=$1 or tenant_id is null) order by tenant_id nulls first, updated_at desc limit $2`, args
	case "ingestion-profiles":
		return `select id, tenant_id, name, embedding_profile_id, vector_store_profile_id, parser_key, ocr_provider_key, chunker_key, chunk_size, overlap, min_tokens, max_tokens, status, version, created_at, updated_at from xz_knowledge_ingestion_profiles where ($1='' or tenant_id=$1 or tenant_id is null) order by tenant_id nulls first, updated_at desc limit $2`, args
	case "retrieval-profiles":
		return `select id, tenant_id, name, search_mode, top_k, threshold, vector_weight, keyword_weight, rerank_profile_id, context_token_limit, query_rewrite_enabled, metadata_filter_enabled, status, created_at, updated_at from xz_knowledge_retrieval_profiles where ($1='' or tenant_id=$1 or tenant_id is null) order by tenant_id nulls first, updated_at desc limit $2`, args
	default:
		return "", nil
	}
}

func scanAdminRows(rows *sql.Rows) ([]map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for index := range values {
			pointers[index] = &values[index]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		item := make(map[string]any, len(columns))
		for index, column := range columns {
			item[toCamelCase(column)] = normalizeAdminValue(values[index])
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func normalizeAdminValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		var decoded any
		if json.Unmarshal(typed, &decoded) == nil {
			return decoded
		}
		return string(typed)
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return value
	}
}

func toCamelCase(value string) string {
	parts := strings.Split(value, "_")
	for index := 1; index < len(parts); index++ {
		if parts[index] != "" {
			parts[index] = strings.ToUpper(parts[index][:1]) + parts[index][1:]
		}
	}
	return strings.Join(parts, "")
}

var _ knowledgeapp.AdminRepository = (*Postgres)(nil)
