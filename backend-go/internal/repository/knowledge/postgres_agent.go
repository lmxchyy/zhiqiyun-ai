package knowledgerepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

func (p *Postgres) CreateAgent(ctx context.Context, access knowledgeapp.AccessContext, item knowledgeapp.Agent) (knowledgeapp.Agent, error) {
	row := p.db.QueryRowContext(ctx, `
		insert into xz_ai_agents (id, tenant_id, owner_user_id, name, description, model_name, system_prompt, status, version, config, created_at, updated_at)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12)
		returning id, tenant_id, owner_user_id, name, description, model_name, system_prompt, status, version, config, created_at, updated_at
	`, item.ID, access.TenantID, item.OwnerUserID, item.Name, item.Description, item.ModelName, item.SystemPrompt, item.Status, item.Version, jsonText(item.Config), item.CreatedAt, item.UpdatedAt)
	return scanAgent(row)
}

func (p *Postgres) ListAgents(ctx context.Context, access knowledgeapp.AccessContext, options knowledgeapp.ListOptions) ([]knowledgeapp.Agent, string, error) {
	limit := normalizedLimit(options.Limit)
	rows, err := p.db.QueryContext(ctx, `
		select id, tenant_id, owner_user_id, name, description, model_name, system_prompt, status, version, config, created_at, updated_at
		from xz_ai_agents
		where tenant_id=$1 and deleted_at is null
		  and ($2 or owner_user_id=$3)
		  and ($4='' or status=$4)
		  and ($5='' or name ilike '%' || $5 || '%' or description ilike '%' || $5 || '%')
		order by updated_at desc, id desc limit $6
	`, access.TenantID, access.HasRole("PLATFORM_ADMIN", "ENTERPRISE_ADMIN"), access.UserID,
		strings.ToUpper(strings.TrimSpace(options.Status)), strings.TrimSpace(options.Query), limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]knowledgeapp.Agent, 0)
	for rows.Next() {
		item, err := scanAgent(rows)
		if err != nil {
			return nil, "", err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) == limit && len(items) > 0 {
		next = items[len(items)-1].ID
	}
	return items, next, nil
}

func (p *Postgres) GetAgent(ctx context.Context, access knowledgeapp.AccessContext, id string) (knowledgeapp.Agent, error) {
	item, err := scanAgent(p.db.QueryRowContext(ctx, `
		select id, tenant_id, owner_user_id, name, description, model_name, system_prompt, status, version, config, created_at, updated_at
		from xz_ai_agents
		where tenant_id=$1 and id=$2 and deleted_at is null and ($3 or owner_user_id=$4)
	`, access.TenantID, id, access.HasRole("PLATFORM_ADMIN", "ENTERPRISE_ADMIN"), access.UserID))
	if errors.Is(err, sql.ErrNoRows) {
		return knowledgeapp.Agent{}, knowledgeapp.ErrNotFound
	}
	return item, err
}

func (p *Postgres) ReplaceAgentKnowledgeBindings(ctx context.Context, access knowledgeapp.AccessContext, agentID string, bindings []knowledgeapp.AgentKnowledgeBinding) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var allowed bool
	if err := tx.QueryRowContext(ctx, `
		select exists(select 1 from xz_ai_agents where tenant_id=$1 and id=$2 and deleted_at is null and ($3 or owner_user_id=$4))
	`, access.TenantID, agentID, access.HasRole("PLATFORM_ADMIN", "ENTERPRISE_ADMIN"), access.UserID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return knowledgeapp.ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `delete from xz_agent_knowledge_bindings where tenant_id=$1 and agent_id=$2`, access.TenantID, agentID); err != nil {
		return err
	}
	for _, item := range bindings {
		if item.TenantID != access.TenantID || item.AgentID != agentID {
			return knowledgeapp.ErrForbidden
		}
		if _, err := tx.ExecContext(ctx, `
			insert into xz_agent_knowledge_bindings (id, tenant_id, agent_id, knowledge_base_id, retrieval_profile_id, priority, weight, enabled, retrieval_overrides)
			values ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)
		`, item.ID, access.TenantID, agentID, item.KnowledgeBaseID, nullableText(item.RetrievalProfileID), item.Priority,
			item.Weight, item.Enabled, jsonText(item.RetrievalOverrides)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *Postgres) ListAgentKnowledgeBindings(ctx context.Context, access knowledgeapp.AccessContext, agentID string) ([]knowledgeapp.AgentKnowledgeBinding, error) {
	rows, err := p.db.QueryContext(ctx, `
		select b.id, b.tenant_id, b.agent_id, b.knowledge_base_id, coalesce(b.retrieval_profile_id, ''), b.priority, b.weight, b.enabled, b.retrieval_overrides
		from xz_agent_knowledge_bindings b
		join xz_ai_agents a on a.tenant_id=b.tenant_id and a.id=b.agent_id and a.deleted_at is null
		where b.tenant_id=$1 and b.agent_id=$2
		order by b.priority desc, b.knowledge_base_id
	`, access.TenantID, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]knowledgeapp.AgentKnowledgeBinding, 0)
	for rows.Next() {
		var item knowledgeapp.AgentKnowledgeBinding
		var overrides []byte
		if err := rows.Scan(&item.ID, &item.TenantID, &item.AgentID, &item.KnowledgeBaseID, &item.RetrievalProfileID,
			&item.Priority, &item.Weight, &item.Enabled, &overrides); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(overrides, &item.RetrievalOverrides)
		if item.RetrievalOverrides == nil {
			item.RetrievalOverrides = map[string]any{}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (p *Postgres) CreateConversation(ctx context.Context, access knowledgeapp.AccessContext, item knowledgeapp.Conversation) (knowledgeapp.Conversation, error) {
	row := p.db.QueryRowContext(ctx, `
		insert into xz_ai_agent_conversations (id, tenant_id, organization_id, agent_id, user_id, title, status, created_at, updated_at)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		returning id, tenant_id, coalesce(organization_id, ''), agent_id, user_id, title, status, created_at, updated_at, deleted_at
	`, item.ID, access.TenantID, nullableText(item.OrganizationID), item.AgentID, access.UserID, item.Title, item.Status, item.CreatedAt, item.UpdatedAt)
	return scanConversation(row)
}

func (p *Postgres) ListConversations(ctx context.Context, access knowledgeapp.AccessContext, agentID string, options knowledgeapp.ListOptions) ([]knowledgeapp.Conversation, string, error) {
	limit := normalizedLimit(options.Limit)
	rows, err := p.db.QueryContext(ctx, `
		select id, tenant_id, coalesce(organization_id, ''), agent_id, user_id, title, status, created_at, updated_at, deleted_at
		from xz_ai_agent_conversations
		where tenant_id=$1 and user_id=$2 and deleted_at is null and ($3='' or agent_id=$3)
		order by updated_at desc, id desc limit $4
	`, access.TenantID, access.UserID, agentID, limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]knowledgeapp.Conversation, 0)
	for rows.Next() {
		item, err := scanConversation(rows)
		if err != nil {
			return nil, "", err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) == limit && len(items) > 0 {
		next = items[len(items)-1].ID
	}
	return items, next, nil
}

func (p *Postgres) GetConversation(ctx context.Context, access knowledgeapp.AccessContext, id string) (knowledgeapp.Conversation, error) {
	item, err := scanConversation(p.db.QueryRowContext(ctx, `
		select id, tenant_id, coalesce(organization_id, ''), agent_id, user_id, title, status, created_at, updated_at, deleted_at
		from xz_ai_agent_conversations where tenant_id=$1 and id=$2 and user_id=$3 and deleted_at is null
	`, access.TenantID, id, access.UserID))
	if errors.Is(err, sql.ErrNoRows) {
		return knowledgeapp.Conversation{}, knowledgeapp.ErrNotFound
	}
	return item, err
}

func (p *Postgres) CreateMessage(ctx context.Context, access knowledgeapp.AccessContext, item knowledgeapp.Message) (knowledgeapp.Message, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return knowledgeapp.Message{}, err
	}
	defer tx.Rollback()
	created, err := scanMessage(tx.QueryRowContext(ctx, `
		insert into xz_ai_agent_messages (id, tenant_id, conversation_id, parent_message_id, role, content, status, input_tokens, output_tokens, metadata, created_at)
		select $1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11
		where exists(select 1 from xz_ai_agent_conversations where tenant_id=$2 and id=$3 and user_id=$12 and deleted_at is null)
		returning id, tenant_id, conversation_id, coalesce(parent_message_id, ''), role, content, status, input_tokens, output_tokens, metadata, created_at
	`, item.ID, access.TenantID, item.ConversationID, nullableText(item.ParentMessageID), item.Role, item.Content, item.Status,
		item.InputTokens, item.OutputTokens, jsonText(item.Metadata), item.CreatedAt, access.UserID))
	if errors.Is(err, sql.ErrNoRows) {
		return knowledgeapp.Message{}, knowledgeapp.ErrNotFound
	}
	if err != nil {
		return knowledgeapp.Message{}, err
	}
	if _, err := tx.ExecContext(ctx, `update xz_ai_agent_conversations set updated_at=now() where tenant_id=$1 and id=$2`, access.TenantID, item.ConversationID); err != nil {
		return knowledgeapp.Message{}, err
	}
	if err := tx.Commit(); err != nil {
		return knowledgeapp.Message{}, err
	}
	return created, nil
}

func (p *Postgres) ListMessages(ctx context.Context, access knowledgeapp.AccessContext, conversationID string, options knowledgeapp.ListOptions) ([]knowledgeapp.Message, string, error) {
	limit := normalizedLimit(options.Limit)
	rows, err := p.db.QueryContext(ctx, `
		select m.id, m.tenant_id, m.conversation_id, coalesce(m.parent_message_id, ''), m.role, m.content, m.status,
			m.input_tokens, m.output_tokens, m.metadata, m.created_at
		from xz_ai_agent_messages m
		join xz_ai_agent_conversations c on c.tenant_id=m.tenant_id and c.id=m.conversation_id
		where m.tenant_id=$1 and m.conversation_id=$2 and c.user_id=$3 and c.deleted_at is null
		order by m.created_at, m.id limit $4
	`, access.TenantID, conversationID, access.UserID, limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]knowledgeapp.Message, 0)
	for rows.Next() {
		item, err := scanMessage(rows)
		if err != nil {
			return nil, "", err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) == limit && len(items) > 0 {
		next = items[len(items)-1].ID
	}
	return items, next, nil
}

func (p *Postgres) CreateRun(ctx context.Context, access knowledgeapp.AccessContext, item knowledgeapp.RAGRun) (knowledgeapp.RAGRun, error) {
	created, err := scanRun(p.db.QueryRowContext(ctx, `
		insert into xz_rag_runs (id, tenant_id, conversation_id, user_message_id, assistant_message_id, agent_id, retry_of_run_id,
			original_query, rewritten_query, status, retrieval_latency_ms, generation_latency_ms, input_tokens, output_tokens,
			point_cost, binding_snapshot, retrieval_snapshot, error_code, error_message, created_at, updated_at)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16::jsonb,$17::jsonb,$18,$19,$20,$21)
		returning id, tenant_id, conversation_id, user_message_id, coalesce(assistant_message_id, ''), agent_id,
			coalesce(retry_of_run_id, ''), original_query, rewritten_query, status, retrieval_latency_ms, generation_latency_ms,
			input_tokens, output_tokens, point_cost, binding_snapshot, retrieval_snapshot, coalesce(error_code, ''), coalesce(error_message, ''), created_at, updated_at
	`, item.ID, access.TenantID, item.ConversationID, item.UserMessageID, nullableText(item.AssistantMessageID), item.AgentID,
		nullableText(item.RetryOfRunID), item.OriginalQuery, item.RewrittenQuery, item.Status, item.RetrievalLatencyMS,
		item.GenerationLatencyMS, item.InputTokens, item.OutputTokens, item.PointCost, jsonText(item.BindingSnapshot),
		jsonText(item.RetrievalSnapshot), nullableText(item.ErrorCode), nullableText(item.ErrorMessage), item.CreatedAt, item.UpdatedAt))
	if errors.Is(err, sql.ErrNoRows) {
		return knowledgeapp.RAGRun{}, knowledgeapp.ErrNotFound
	}
	return created, err
}

func (p *Postgres) GetRun(ctx context.Context, access knowledgeapp.AccessContext, id string) (knowledgeapp.RAGRun, error) {
	item, err := scanRun(p.db.QueryRowContext(ctx, `
		select r.id, r.tenant_id, r.conversation_id, r.user_message_id, coalesce(r.assistant_message_id, ''), r.agent_id,
			coalesce(r.retry_of_run_id, ''), r.original_query, r.rewritten_query, r.status, r.retrieval_latency_ms, r.generation_latency_ms,
			r.input_tokens, r.output_tokens, r.point_cost, r.binding_snapshot, r.retrieval_snapshot, coalesce(r.error_code, ''),
			coalesce(r.error_message, ''), r.created_at, r.updated_at
		from xz_rag_runs r
		join xz_ai_agent_conversations c on c.tenant_id=r.tenant_id and c.id=r.conversation_id
		where r.tenant_id=$1 and r.id=$2 and c.user_id=$3
	`, access.TenantID, id, access.UserID))
	if errors.Is(err, sql.ErrNoRows) {
		return knowledgeapp.RAGRun{}, knowledgeapp.ErrNotFound
	}
	return item, err
}

func (p *Postgres) UpdateRun(ctx context.Context, access knowledgeapp.AccessContext, item knowledgeapp.RAGRun) error {
	result, err := p.db.ExecContext(ctx, `
		update xz_rag_runs set assistant_message_id=$3, rewritten_query=$4, status=$5, retrieval_latency_ms=$6,
			generation_latency_ms=$7, input_tokens=$8, output_tokens=$9, point_cost=$10, binding_snapshot=$11::jsonb,
			retrieval_snapshot=$12::jsonb, error_code=$13, error_message=$14, updated_at=$15,
			started_at=case when $5 in ('RUNNING','RETRIEVING','GENERATING') then coalesce(started_at, now()) else started_at end,
			finished_at=case when $5 in ('COMPLETED','FAILED','CANCELLED') then now() else null end
		where tenant_id=$1 and id=$2
	`, access.TenantID, item.ID, nullableText(item.AssistantMessageID), item.RewrittenQuery, item.Status, item.RetrievalLatencyMS,
		item.GenerationLatencyMS, item.InputTokens, item.OutputTokens, item.PointCost, jsonText(item.BindingSnapshot),
		jsonText(item.RetrievalSnapshot), nullableText(item.ErrorCode), nullableText(item.ErrorMessage), item.UpdatedAt)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (p *Postgres) SaveRetrievalHits(ctx context.Context, access knowledgeapp.AccessContext, runID string, hits []knowledgeapp.RetrievalHit) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `delete from xz_rag_retrieval_hits where tenant_id=$1 and rag_run_id=$2`, access.TenantID, runID); err != nil {
		return err
	}
	for _, item := range hits {
		metadata := cloneJSONMap(item.Metadata)
		metadata["documentId"] = item.DocumentID
		metadata["documentVersionId"] = item.DocumentVersionID
		metadata["documentName"] = item.DocumentName
		metadata["title"] = item.Title
		metadata["content"] = item.Content
		metadata["sourceLocator"] = item.SourceLocator
		if _, err := tx.ExecContext(ctx, `
			insert into xz_rag_retrieval_hits (id, tenant_id, rag_run_id, knowledge_base_id, chunk_id, initial_rank, final_rank,
				vector_score, keyword_score, rerank_score, final_score, metadata_snapshot)
			values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb)
		`, item.ID, access.TenantID, runID, item.KnowledgeBaseID, item.ChunkID, item.InitialRank, item.FinalRank,
			nullableFloat(item.VectorScore), nullableFloat(item.KeywordScore), nullableFloat(item.RerankScore), item.FinalScore, jsonText(metadata)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *Postgres) SaveCitations(ctx context.Context, access knowledgeapp.AccessContext, runID string, citations []knowledgeapp.Citation) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `delete from xz_rag_citations where tenant_id=$1 and rag_run_id=$2`, access.TenantID, runID); err != nil {
		return err
	}
	for _, item := range citations {
		if _, err := tx.ExecContext(ctx, `
			insert into xz_rag_citations (id, tenant_id, rag_run_id, assistant_message_id, document_id, document_version_id,
				chunk_id, citation_order, document_name_snapshot, quote_snapshot, locator_snapshot, similarity_score)
			values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12)
		`, item.ID, access.TenantID, runID, item.AssistantMessageID, item.DocumentID, item.DocumentVersionID, item.ChunkID,
			item.Order, item.DocumentName, item.Quote, jsonText(item.Locator), nullableFloat(item.SimilarityScore)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *Postgres) ListCitations(ctx context.Context, access knowledgeapp.AccessContext, runID string) ([]knowledgeapp.Citation, error) {
	rows, err := p.db.QueryContext(ctx, `
		select c.id, c.tenant_id, c.rag_run_id, c.assistant_message_id, c.document_id, c.document_version_id, c.chunk_id,
			c.citation_order, c.document_name_snapshot, c.quote_snapshot, c.locator_snapshot, c.similarity_score
		from xz_rag_citations c
		join xz_rag_runs r on r.tenant_id=c.tenant_id and r.id=c.rag_run_id
		join xz_ai_agent_conversations ac on ac.tenant_id=r.tenant_id and ac.id=r.conversation_id
		where c.tenant_id=$1 and c.rag_run_id=$2 and ac.user_id=$3
		order by c.citation_order
	`, access.TenantID, runID, access.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]knowledgeapp.Citation, 0)
	for rows.Next() {
		var item knowledgeapp.Citation
		var locator []byte
		var score sql.NullFloat64
		if err := rows.Scan(&item.ID, &item.TenantID, &item.RAGRunID, &item.AssistantMessageID, &item.DocumentID,
			&item.DocumentVersionID, &item.ChunkID, &item.Order, &item.DocumentName, &item.Quote, &locator, &score); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(locator, &item.Locator)
		if score.Valid {
			item.SimilarityScore = score.Float64
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (p *Postgres) AppendRunEvent(ctx context.Context, access knowledgeapp.AccessContext, item knowledgeapp.RunEvent) error {
	_, err := p.db.ExecContext(ctx, `
		insert into xz_rag_run_events (id, tenant_id, rag_run_id, sequence_no, event_type, payload, created_at)
		values ($1,$2,$3,$4,$5,$6::jsonb,$7)
	`, item.ID, access.TenantID, item.RAGRunID, item.SequenceNo, item.EventType, jsonText(item.Payload), item.CreatedAt)
	return err
}

func (p *Postgres) ListRunEvents(ctx context.Context, access knowledgeapp.AccessContext, runID string, afterSequence int64) ([]knowledgeapp.RunEvent, error) {
	rows, err := p.db.QueryContext(ctx, `
		select e.id, e.tenant_id, e.rag_run_id, e.sequence_no, e.event_type, e.payload, e.created_at
		from xz_rag_run_events e
		join xz_rag_runs r on r.tenant_id=e.tenant_id and r.id=e.rag_run_id
		join xz_ai_agent_conversations c on c.tenant_id=r.tenant_id and c.id=r.conversation_id
		where e.tenant_id=$1 and e.rag_run_id=$2 and e.sequence_no>$3 and c.user_id=$4
		order by e.sequence_no
	`, access.TenantID, runID, afterSequence, access.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]knowledgeapp.RunEvent, 0)
	for rows.Next() {
		var item knowledgeapp.RunEvent
		var payload []byte
		if err := rows.Scan(&item.ID, &item.TenantID, &item.RAGRunID, &item.SequenceNo, &item.EventType, &payload, &item.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(payload, &item.Payload)
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanAgent(row scanner) (knowledgeapp.Agent, error) {
	var item knowledgeapp.Agent
	var config []byte
	err := row.Scan(&item.ID, &item.TenantID, &item.OwnerUserID, &item.Name, &item.Description, &item.ModelName,
		&item.SystemPrompt, &item.Status, &item.Version, &config, &item.CreatedAt, &item.UpdatedAt)
	_ = json.Unmarshal(config, &item.Config)
	return item, err
}

func scanConversation(row scanner) (knowledgeapp.Conversation, error) {
	var item knowledgeapp.Conversation
	var deleted sql.NullTime
	err := row.Scan(&item.ID, &item.TenantID, &item.OrganizationID, &item.AgentID, &item.UserID, &item.Title,
		&item.Status, &item.CreatedAt, &item.UpdatedAt, &deleted)
	if deleted.Valid {
		item.DeletedAt = &deleted.Time
	}
	return item, err
}

func scanMessage(row scanner) (knowledgeapp.Message, error) {
	var item knowledgeapp.Message
	var metadata []byte
	err := row.Scan(&item.ID, &item.TenantID, &item.ConversationID, &item.ParentMessageID, &item.Role, &item.Content,
		&item.Status, &item.InputTokens, &item.OutputTokens, &metadata, &item.CreatedAt)
	_ = json.Unmarshal(metadata, &item.Metadata)
	return item, err
}

func scanRun(row scanner) (knowledgeapp.RAGRun, error) {
	var item knowledgeapp.RAGRun
	var bindingSnapshot, retrievalSnapshot []byte
	err := row.Scan(&item.ID, &item.TenantID, &item.ConversationID, &item.UserMessageID, &item.AssistantMessageID,
		&item.AgentID, &item.RetryOfRunID, &item.OriginalQuery, &item.RewrittenQuery, &item.Status, &item.RetrievalLatencyMS,
		&item.GenerationLatencyMS, &item.InputTokens, &item.OutputTokens, &item.PointCost, &bindingSnapshot, &retrievalSnapshot,
		&item.ErrorCode, &item.ErrorMessage, &item.CreatedAt, &item.UpdatedAt)
	_ = json.Unmarshal(bindingSnapshot, &item.BindingSnapshot)
	_ = json.Unmarshal(retrievalSnapshot, &item.RetrievalSnapshot)
	return item, err
}

func cloneJSONMap(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value)+6)
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func nullableFloat(value float64) any {
	if value == 0 {
		return nil
	}
	return value
}

var _ knowledgeapp.AgentRepository = (*Postgres)(nil)
var _ knowledgeapp.RAGRepository = (*Postgres)(nil)
