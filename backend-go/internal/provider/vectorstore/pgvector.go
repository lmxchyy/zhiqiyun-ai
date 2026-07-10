package vectorstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type PGVector struct {
	db *sql.DB
}

func NewPGVector(db *sql.DB) *PGVector { return &PGVector{db: db} }
func (*PGVector) Code() string         { return "pgvector" }

func (p *PGVector) Upsert(ctx context.Context, indexID string, records []knowledgeapp.VectorRecord) error {
	if len(records) == 0 {
		return nil
	}
	first := records[0]
	dimension := len(first.Embedding)
	embeddingProfileID := strings.TrimSpace(first.EmbeddingProfileID)
	if embeddingProfileID == "" {
		embeddingProfileID = "embedding_deterministic_default"
	}
	vectorStoreProfileID := strings.TrimSpace(first.VectorStoreProfileID)
	if vectorStoreProfileID == "" {
		vectorStoreProfileID = "vector_pgvector_default"
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		insert into xz_knowledge_vector_indices (
			id, tenant_id, knowledge_base_id, embedding_profile_id, vector_store_profile_id,
			revision, dimension, physical_index_name, status, is_active, indexed_chunk_count
		) values ($1,$2,$3,$4,$5,1,$6,$1,'ACTIVE',true,0)
		on conflict (id) do update set embedding_profile_id=excluded.embedding_profile_id,
			vector_store_profile_id=excluded.vector_store_profile_id, dimension=excluded.dimension, status='ACTIVE', updated_at=now()
	`, indexID, first.TenantID, first.KnowledgeBaseID, embeddingProfileID, vectorStoreProfileID, dimension); err != nil {
		return err
	}
	for _, record := range records {
		if record.TenantID != first.TenantID || record.KnowledgeBaseID != first.KnowledgeBaseID || len(record.Embedding) != dimension {
			return fmt.Errorf("vector record index scope or dimension mismatch")
		}
		metadata, _ := json.Marshal(record.FilterMetadata)
		if _, err := tx.ExecContext(ctx, `
			insert into xz_knowledge_vector_entries (
				id, tenant_id, vector_index_id, chunk_id, embedding, search_text, embedding_hash, filter_metadata, status
			) values ($1,$2,$3,$4,$5::vector,$6,$7,$8::jsonb,'ACTIVE')
			on conflict (vector_index_id, chunk_id) do update set
				embedding=excluded.embedding, search_text=excluded.search_text, embedding_hash=excluded.embedding_hash,
				filter_metadata=excluded.filter_metadata, status='ACTIVE', updated_at=now()
		`, "vector_"+record.ChunkID, record.TenantID, indexID, record.ChunkID, vectorLiteral(record.Embedding), record.SearchText, record.EmbeddingHash, string(metadata)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `update xz_knowledge_vector_indices set indexed_chunk_count=(select count(*) from xz_knowledge_vector_entries where vector_index_id=$1 and status='ACTIVE'), updated_at=now() where id=$1`, indexID); err != nil {
		return err
	}
	return tx.Commit()
}

func (p *PGVector) DeleteByDocumentVersion(ctx context.Context, access knowledgeapp.AccessContext, indexID string, documentVersionID string) error {
	_, err := p.db.ExecContext(ctx, `
		delete from xz_knowledge_vector_entries e
		using xz_knowledge_chunks c
		where e.tenant_id=$1 and e.vector_index_id=$2 and e.chunk_id=c.id and c.tenant_id=$1 and c.document_version_id=$3
	`, access.TenantID, indexID, documentVersionID)
	return err
}

func (p *PGVector) Search(ctx context.Context, request knowledgeapp.SearchRequest, queryVector []float32) ([]knowledgeapp.RetrievalHit, error) {
	if request.VectorWeight == 0 && request.KeywordWeight == 0 {
		request.VectorWeight = 0.7
		request.KeywordWeight = 0.3
	}
	args := []any{request.Access.TenantID, vectorLiteral(queryVector), request.Query, request.VectorWeight, request.KeywordWeight}
	where := "e.tenant_id=$1 and e.status='ACTIVE' and i.is_active=true and c.deleted_at is null"
	if len(request.KnowledgeBaseIDs) > 0 {
		placeholders := make([]string, 0, len(request.KnowledgeBaseIDs))
		for _, id := range request.KnowledgeBaseIDs {
			args = append(args, id)
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
		}
		where += " and i.knowledge_base_id in (" + strings.Join(placeholders, ",") + ")"
	}
	if len(request.Filters) > 0 {
		encodedFilters, err := json.Marshal(request.Filters)
		if err != nil {
			return nil, fmt.Errorf("encode metadata filters: %w", err)
		}
		args = append(args, string(encodedFilters))
		where += fmt.Sprintf(" and e.filter_metadata @> $%d::jsonb", len(args))
	}
	threshold := request.Threshold
	topK := request.TopK
	if topK <= 0 {
		topK = 8
	}
	args = append(args, threshold, topK)
	thresholdPosition, limitPosition := len(args)-1, len(args)
	mode := strings.ToUpper(strings.TrimSpace(request.Mode))
	finalExpression := "vector_score"
	if mode == "FULLTEXT" {
		finalExpression = "keyword_score"
	} else if mode == "HYBRID" || mode == "" {
		finalExpression = "vector_score * $4 + keyword_score * $5"
	}
	query := `
		with scored as (
			select c.id as chunk_id, c.tenant_id, c.knowledge_base_id, c.document_id, c.document_version_id,
				d.name as document_name, c.title, c.content, c.source_locator, c.metadata,
				greatest(0, 1 - (e.embedding <=> $2::vector)) as vector_score,
				ts_rank_cd(e.search_vector, plainto_tsquery('simple', $3)) as keyword_score
			from xz_knowledge_vector_entries e
			join xz_knowledge_vector_indices i on i.id=e.vector_index_id and i.tenant_id=e.tenant_id
			join xz_knowledge_chunks c on c.id=e.chunk_id and c.tenant_id=e.tenant_id
			join xz_knowledge_documents d on d.id=c.document_id and d.tenant_id=c.tenant_id
			where ` + where + `
		), ranked as (
			select *, ` + finalExpression + ` as final_score from scored
		)
		select chunk_id, tenant_id, knowledge_base_id, document_id, document_version_id, document_name, title, content,
			source_locator, metadata, vector_score, keyword_score, final_score
		from ranked where final_score >= $` + strconv.Itoa(thresholdPosition) + `
		order by final_score desc, chunk_id
		limit $` + strconv.Itoa(limitPosition)
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hits := []knowledgeapp.RetrievalHit{}
	for rows.Next() {
		var hit knowledgeapp.RetrievalHit
		var locatorRaw, metadataRaw []byte
		if err := rows.Scan(&hit.ChunkID, &hit.TenantID, &hit.KnowledgeBaseID, &hit.DocumentID, &hit.DocumentVersionID,
			&hit.DocumentName, &hit.Title, &hit.Content, &locatorRaw, &metadataRaw, &hit.VectorScore, &hit.KeywordScore, &hit.FinalScore); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(locatorRaw, &hit.SourceLocator)
		_ = json.Unmarshal(metadataRaw, &hit.Metadata)
		hit.InitialRank = len(hits) + 1
		hit.FinalRank = len(hits) + 1
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}

func vectorLiteral(values []float32) string {
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = strconv.FormatFloat(float64(value), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

var _ knowledgeapp.VectorStore = (*PGVector)(nil)
