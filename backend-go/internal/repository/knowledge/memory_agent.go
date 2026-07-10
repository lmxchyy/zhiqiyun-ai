package knowledgerepo

import (
	"context"
	"sort"
	"strings"
	"time"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

func (m *Memory) CreateAgent(_ context.Context, access knowledgeapp.AccessContext, item knowledgeapp.Agent) (knowledgeapp.Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if item.TenantID != access.TenantID {
		return knowledgeapp.Agent{}, knowledgeapp.ErrForbidden
	}
	if _, exists := m.agents[item.ID]; exists {
		return knowledgeapp.Agent{}, knowledgeapp.ErrConflict
	}
	m.agents[item.ID] = item
	return item, nil
}

func (m *Memory) ListAgents(_ context.Context, access knowledgeapp.AccessContext, options knowledgeapp.ListOptions) ([]knowledgeapp.Agent, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	query := strings.ToLower(strings.TrimSpace(options.Query))
	items := make([]knowledgeapp.Agent, 0)
	for _, item := range m.agents {
		if item.TenantID != access.TenantID || (item.OwnerUserID != access.UserID && !access.HasRole("PLATFORM_ADMIN", "ENTERPRISE_ADMIN")) {
			continue
		}
		if options.Status != "" && !strings.EqualFold(item.Status, options.Status) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(item.Name+" "+item.Description), query) {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	limit := normalizedLimit(options.Limit)
	if len(items) > limit {
		return items[:limit], items[limit-1].ID, nil
	}
	return items, "", nil
}

func (m *Memory) GetAgent(_ context.Context, access knowledgeapp.AccessContext, id string) (knowledgeapp.Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, exists := m.agents[id]
	if !exists || item.TenantID != access.TenantID || (item.OwnerUserID != access.UserID && !access.HasRole("PLATFORM_ADMIN", "ENTERPRISE_ADMIN")) {
		return knowledgeapp.Agent{}, knowledgeapp.ErrNotFound
	}
	return item, nil
}

func (m *Memory) ReplaceAgentKnowledgeBindings(_ context.Context, access knowledgeapp.AccessContext, agentID string, bindings []knowledgeapp.AgentKnowledgeBinding) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	agent, exists := m.agents[agentID]
	if !exists || agent.TenantID != access.TenantID || (agent.OwnerUserID != access.UserID && !access.HasRole("PLATFORM_ADMIN", "ENTERPRISE_ADMIN")) {
		return knowledgeapp.ErrNotFound
	}
	for _, binding := range bindings {
		base, exists := m.bases[binding.KnowledgeBaseID]
		if !exists || base.TenantID != access.TenantID || binding.TenantID != access.TenantID || binding.AgentID != agentID {
			return knowledgeapp.ErrForbidden
		}
	}
	m.bindings[agentID] = append([]knowledgeapp.AgentKnowledgeBinding(nil), bindings...)
	return nil
}

func (m *Memory) ListAgentKnowledgeBindings(_ context.Context, access knowledgeapp.AccessContext, agentID string) ([]knowledgeapp.AgentKnowledgeBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	agent, exists := m.agents[agentID]
	if !exists || agent.TenantID != access.TenantID {
		return nil, knowledgeapp.ErrNotFound
	}
	items := append([]knowledgeapp.AgentKnowledgeBinding(nil), m.bindings[agentID]...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority == items[j].Priority {
			return items[i].KnowledgeBaseID < items[j].KnowledgeBaseID
		}
		return items[i].Priority > items[j].Priority
	})
	return items, nil
}

func (m *Memory) CreateConversation(_ context.Context, access knowledgeapp.AccessContext, item knowledgeapp.Conversation) (knowledgeapp.Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	agent, exists := m.agents[item.AgentID]
	if !exists || agent.TenantID != access.TenantID || item.TenantID != access.TenantID || item.UserID != access.UserID {
		return knowledgeapp.Conversation{}, knowledgeapp.ErrForbidden
	}
	if _, exists := m.conversations[item.ID]; exists {
		return knowledgeapp.Conversation{}, knowledgeapp.ErrConflict
	}
	m.conversations[item.ID] = item
	return item, nil
}

func (m *Memory) ListConversations(_ context.Context, access knowledgeapp.AccessContext, agentID string, options knowledgeapp.ListOptions) ([]knowledgeapp.Conversation, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := make([]knowledgeapp.Conversation, 0)
	for _, item := range m.conversations {
		if item.TenantID != access.TenantID || item.UserID != access.UserID || item.DeletedAt != nil || (agentID != "" && item.AgentID != agentID) {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	limit := normalizedLimit(options.Limit)
	if len(items) > limit {
		return items[:limit], items[limit-1].ID, nil
	}
	return items, "", nil
}

func (m *Memory) GetConversation(_ context.Context, access knowledgeapp.AccessContext, id string) (knowledgeapp.Conversation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, exists := m.conversations[id]
	if !exists || item.TenantID != access.TenantID || item.UserID != access.UserID || item.DeletedAt != nil {
		return knowledgeapp.Conversation{}, knowledgeapp.ErrNotFound
	}
	return item, nil
}

func (m *Memory) CreateMessage(_ context.Context, access knowledgeapp.AccessContext, item knowledgeapp.Message) (knowledgeapp.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	conversation, exists := m.conversations[item.ConversationID]
	if !exists || conversation.TenantID != access.TenantID || conversation.UserID != access.UserID || item.TenantID != access.TenantID {
		return knowledgeapp.Message{}, knowledgeapp.ErrForbidden
	}
	if _, exists := m.messages[item.ID]; exists {
		return knowledgeapp.Message{}, knowledgeapp.ErrConflict
	}
	m.messages[item.ID] = item
	conversation.UpdatedAt = time.Now().UTC()
	m.conversations[conversation.ID] = conversation
	return item, nil
}

func (m *Memory) ListMessages(_ context.Context, access knowledgeapp.AccessContext, conversationID string, options knowledgeapp.ListOptions) ([]knowledgeapp.Message, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conversation, exists := m.conversations[conversationID]
	if !exists || conversation.TenantID != access.TenantID || conversation.UserID != access.UserID {
		return nil, "", knowledgeapp.ErrNotFound
	}
	items := make([]knowledgeapp.Message, 0)
	for _, item := range m.messages {
		if item.TenantID == access.TenantID && item.ConversationID == conversationID {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	limit := normalizedLimit(options.Limit)
	if len(items) > limit {
		return items[:limit], items[limit-1].ID, nil
	}
	return items, "", nil
}

func (m *Memory) CreateRun(_ context.Context, access knowledgeapp.AccessContext, item knowledgeapp.RAGRun) (knowledgeapp.RAGRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	conversation, exists := m.conversations[item.ConversationID]
	if !exists || conversation.TenantID != access.TenantID || conversation.UserID != access.UserID || item.TenantID != access.TenantID {
		return knowledgeapp.RAGRun{}, knowledgeapp.ErrForbidden
	}
	if _, exists := m.runs[item.ID]; exists {
		return knowledgeapp.RAGRun{}, knowledgeapp.ErrConflict
	}
	m.runs[item.ID] = item
	return item, nil
}

func (m *Memory) GetRun(_ context.Context, access knowledgeapp.AccessContext, id string) (knowledgeapp.RAGRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, exists := m.runs[id]
	if !exists || item.TenantID != access.TenantID {
		return knowledgeapp.RAGRun{}, knowledgeapp.ErrNotFound
	}
	conversation, exists := m.conversations[item.ConversationID]
	if !exists || conversation.UserID != access.UserID {
		return knowledgeapp.RAGRun{}, knowledgeapp.ErrNotFound
	}
	return item, nil
}

func (m *Memory) UpdateRun(_ context.Context, access knowledgeapp.AccessContext, item knowledgeapp.RAGRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, exists := m.runs[item.ID]
	if !exists || current.TenantID != access.TenantID || item.TenantID != access.TenantID {
		return knowledgeapp.ErrNotFound
	}
	m.runs[item.ID] = item
	return nil
}

func (m *Memory) SaveRetrievalHits(_ context.Context, access knowledgeapp.AccessContext, runID string, hits []knowledgeapp.RetrievalHit) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, exists := m.runs[runID]
	if !exists || run.TenantID != access.TenantID {
		return knowledgeapp.ErrNotFound
	}
	m.hits[runID] = append([]knowledgeapp.RetrievalHit(nil), hits...)
	return nil
}

func (m *Memory) SaveCitations(_ context.Context, access knowledgeapp.AccessContext, runID string, citations []knowledgeapp.Citation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, exists := m.runs[runID]
	if !exists || run.TenantID != access.TenantID {
		return knowledgeapp.ErrNotFound
	}
	m.citations[runID] = append([]knowledgeapp.Citation(nil), citations...)
	return nil
}

func (m *Memory) ListCitations(_ context.Context, access knowledgeapp.AccessContext, runID string) ([]knowledgeapp.Citation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	run, exists := m.runs[runID]
	if !exists || run.TenantID != access.TenantID {
		return nil, knowledgeapp.ErrNotFound
	}
	items := append([]knowledgeapp.Citation(nil), m.citations[runID]...)
	sort.Slice(items, func(i, j int) bool { return items[i].Order < items[j].Order })
	return items, nil
}

func (m *Memory) AppendRunEvent(_ context.Context, access knowledgeapp.AccessContext, event knowledgeapp.RunEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, exists := m.runs[event.RAGRunID]
	if !exists || run.TenantID != access.TenantID || event.TenantID != access.TenantID {
		return knowledgeapp.ErrNotFound
	}
	for _, current := range m.events[event.RAGRunID] {
		if current.SequenceNo == event.SequenceNo {
			return knowledgeapp.ErrConflict
		}
	}
	m.events[event.RAGRunID] = append(m.events[event.RAGRunID], event)
	return nil
}

func (m *Memory) ListRunEvents(_ context.Context, access knowledgeapp.AccessContext, runID string, afterSequence int64) ([]knowledgeapp.RunEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	run, exists := m.runs[runID]
	if !exists || run.TenantID != access.TenantID {
		return nil, knowledgeapp.ErrNotFound
	}
	items := make([]knowledgeapp.RunEvent, 0)
	for _, item := range m.events[runID] {
		if item.SequenceNo > afterSequence {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].SequenceNo < items[j].SequenceNo })
	return items, nil
}

func normalizedLimit(limit int) int {
	if limit <= 0 || limit > 200 {
		return 20
	}
	return limit
}

var _ knowledgeapp.AgentRepository = (*Memory)(nil)
var _ knowledgeapp.RAGRepository = (*Memory)(nil)
