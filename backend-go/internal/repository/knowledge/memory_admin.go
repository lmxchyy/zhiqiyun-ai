package knowledgerepo

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

func (m *Memory) KnowledgeAdminOverview(_ context.Context, tenantID string) (knowledgeapp.AdminOverview, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	match := func(value string) bool { return tenantID == "" || value == tenantID }
	overview := knowledgeapp.AdminOverview{}
	tenants := map[string]bool{}
	for _, item := range m.bases {
		if match(item.TenantID) && item.DeletedAt == nil {
			overview.KnowledgeBaseCount++
			tenants[item.TenantID] = true
		}
	}
	for _, item := range m.documents {
		if !match(item.TenantID) || item.DeletedAt != nil {
			continue
		}
		overview.DocumentCount++
		if item.Status == "READY" {
			overview.ReadyDocumentCount++
		}
		if item.Status == "FAILED" {
			overview.FailedDocumentCount++
		}
	}
	for _, item := range m.chunks {
		if match(item.TenantID) && item.DeletedAt == nil {
			overview.ChunkCount++
		}
	}
	for _, item := range m.agents {
		if match(item.TenantID) {
			overview.AgentCount++
		}
	}
	for _, item := range m.runs {
		if !match(item.TenantID) {
			continue
		}
		overview.RAGRunCount++
		if item.Status == "COMPLETED" {
			overview.CompletedRAGRunCount++
		}
		overview.InputTokens += int64(item.InputTokens)
		overview.OutputTokens += int64(item.OutputTokens)
		overview.PointCost += item.PointCost
	}
	overview.TenantCount = int64(len(tenants))
	return overview, nil
}

func (m *Memory) ListKnowledgeAdminRecords(_ context.Context, resource string, tenantID string, options knowledgeapp.ListOptions) ([]map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	match := func(value string) bool { return tenantID == "" || value == tenantID }
	records := make([]map[string]any, 0)
	appendRecord := func(value any) { records = append(records, toAdminRecord(value)) }
	switch strings.ToLower(strings.TrimSpace(resource)) {
	case "bases":
		for _, item := range m.bases {
			if match(item.TenantID) && item.DeletedAt == nil {
				appendRecord(item)
			}
		}
	case "documents":
		for _, item := range m.documents {
			if match(item.TenantID) && item.DeletedAt == nil {
				appendRecord(item)
			}
		}
	case "chunks":
		for _, item := range m.chunks {
			if match(item.TenantID) && item.DeletedAt == nil {
				appendRecord(item)
			}
		}
	case "ingestion-jobs", "parsing-logs":
		for _, item := range m.jobs {
			if match(item.TenantID) {
				appendRecord(item)
			}
		}
	case "rag-runs", "retrieval-logs", "usage", "hot-questions":
		for _, item := range m.runs {
			if match(item.TenantID) {
				appendRecord(item)
			}
		}
	case "agents":
		for _, item := range m.agents {
			if match(item.TenantID) {
				appendRecord(item)
			}
		}
	case "embedding-profiles", "vector-stores", "ingestion-profiles", "retrieval-profiles":
		for _, item := range m.adminProfiles[strings.ToLower(strings.TrimSpace(resource))] {
			if match(adminStringValue(item["tenantId"])) {
				records = append(records, cloneAdminRecord(item))
			}
		}
	default:
		return nil, knowledgeapp.ErrValidation
	}
	sort.SliceStable(records, func(i, j int) bool {
		return adminStringValue(records[i]["updatedAt"]) > adminStringValue(records[j]["updatedAt"])
	})
	limit := normalizedLimit(options.Limit)
	if len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}

func (m *Memory) SaveKnowledgeAdminProfile(_ context.Context, resource string, input map[string]any) (map[string]any, error) {
	resource = strings.ToLower(strings.TrimSpace(resource))
	if resource != "embedding-profiles" && resource != "vector-stores" && resource != "ingestion-profiles" && resource != "retrieval-profiles" {
		return nil, knowledgeapp.ErrValidation
	}
	id := adminStringValue(input["id"])
	if id == "" {
		return nil, knowledgeapp.ErrValidation
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.adminProfiles[resource] == nil {
		m.adminProfiles[resource] = map[string]map[string]any{}
	}
	item := cloneAdminRecord(input)
	m.adminProfiles[resource][id] = item
	return cloneAdminRecord(item), nil
}

func cloneAdminRecord(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func toAdminRecord(value any) map[string]any {
	raw, _ := json.Marshal(value)
	result := map[string]any{}
	_ = json.Unmarshal(raw, &result)
	return result
}

func adminStringValue(value any) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

var _ knowledgeapp.AdminRepository = (*Memory)(nil)
