package knowledgerepo

import (
	"context"
	"sort"
	"strings"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

func (m *Memory) ListKnowledgeTags(_ context.Context, access knowledgeapp.AccessContext) ([]knowledgeapp.Tag, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := []knowledgeapp.Tag{}
	for _, item := range m.tags {
		if item.TenantID == access.TenantID {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (m *Memory) SaveKnowledgeTag(_ context.Context, access knowledgeapp.AccessContext, item knowledgeapp.Tag) (knowledgeapp.Tag, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if item.TenantID != access.TenantID || strings.TrimSpace(item.Name) == "" {
		return knowledgeapp.Tag{}, knowledgeapp.ErrValidation
	}
	for _, current := range m.tags {
		if current.TenantID == access.TenantID && strings.EqualFold(current.Name, item.Name) {
			item.ID = current.ID
		}
	}
	m.tags[item.ID] = item
	return item, nil
}

func (m *Memory) ReplaceKnowledgeBaseTags(_ context.Context, access knowledgeapp.AccessContext, knowledgeBaseID string, tagIDs []string) ([]knowledgeapp.Tag, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	base, ok := m.bases[knowledgeBaseID]
	if !ok || base.TenantID != access.TenantID {
		return nil, knowledgeapp.ErrNotFound
	}
	seen := map[string]bool{}
	ids := []string{}
	items := []knowledgeapp.Tag{}
	for _, id := range tagIDs {
		tag, ok := m.tags[id]
		if !ok || tag.TenantID != access.TenantID {
			return nil, knowledgeapp.ErrNotFound
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
			items = append(items, tag)
		}
	}
	m.baseTags[knowledgeBaseID] = ids
	return items, nil
}

func (m *Memory) ListKnowledgeCategories(_ context.Context, access knowledgeapp.AccessContext) ([]knowledgeapp.Category, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := []knowledgeapp.Category{}
	for _, item := range m.categories {
		if item.TenantID == access.TenantID {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].SortOrder == items[j].SortOrder {
			return items[i].Name < items[j].Name
		}
		return items[i].SortOrder < items[j].SortOrder
	})
	return items, nil
}

func (m *Memory) SaveKnowledgeCategory(_ context.Context, access knowledgeapp.AccessContext, item knowledgeapp.Category) (knowledgeapp.Category, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if item.TenantID != access.TenantID || strings.TrimSpace(item.Name) == "" {
		return knowledgeapp.Category{}, knowledgeapp.ErrValidation
	}
	if item.ParentID != "" {
		parent, ok := m.categories[item.ParentID]
		if !ok || parent.TenantID != access.TenantID {
			return knowledgeapp.Category{}, knowledgeapp.ErrNotFound
		}
	}
	m.categories[item.ID] = item
	return item, nil
}
