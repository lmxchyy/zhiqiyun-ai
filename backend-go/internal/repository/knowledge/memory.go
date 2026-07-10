package knowledgerepo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type Memory struct {
	mu            sync.RWMutex
	bases         map[string]knowledgeapp.KnowledgeBase
	acl           map[string][]knowledgeapp.ACLRule
	tags          map[string]knowledgeapp.Tag
	baseTags      map[string][]string
	categories    map[string]knowledgeapp.Category
	documents     map[string]knowledgeapp.Document
	versions      map[string]knowledgeapp.DocumentVersion
	units         map[string][]knowledgeapp.DocumentUnit
	chunks        map[string]knowledgeapp.Chunk
	jobs          map[string]knowledgeapp.IngestionJob
	agents        map[string]knowledgeapp.Agent
	bindings      map[string][]knowledgeapp.AgentKnowledgeBinding
	conversations map[string]knowledgeapp.Conversation
	messages      map[string]knowledgeapp.Message
	runs          map[string]knowledgeapp.RAGRun
	hits          map[string][]knowledgeapp.RetrievalHit
	citations     map[string][]knowledgeapp.Citation
	events        map[string][]knowledgeapp.RunEvent
	adminProfiles map[string]map[string]map[string]any
}

func NewMemory() *Memory {
	memory := &Memory{
		bases:         map[string]knowledgeapp.KnowledgeBase{},
		acl:           map[string][]knowledgeapp.ACLRule{},
		tags:          map[string]knowledgeapp.Tag{},
		baseTags:      map[string][]string{},
		categories:    map[string]knowledgeapp.Category{},
		documents:     map[string]knowledgeapp.Document{},
		versions:      map[string]knowledgeapp.DocumentVersion{},
		units:         map[string][]knowledgeapp.DocumentUnit{},
		chunks:        map[string]knowledgeapp.Chunk{},
		jobs:          map[string]knowledgeapp.IngestionJob{},
		agents:        map[string]knowledgeapp.Agent{},
		bindings:      map[string][]knowledgeapp.AgentKnowledgeBinding{},
		conversations: map[string]knowledgeapp.Conversation{},
		messages:      map[string]knowledgeapp.Message{},
		runs:          map[string]knowledgeapp.RAGRun{},
		hits:          map[string][]knowledgeapp.RetrievalHit{},
		citations:     map[string][]knowledgeapp.Citation{},
		events:        map[string][]knowledgeapp.RunEvent{},
		adminProfiles: map[string]map[string]map[string]any{},
	}
	memory.adminProfiles["embedding-profiles"] = map[string]map[string]any{
		"embedding_deterministic_default": {"id": "embedding_deterministic_default", "tenantId": "", "name": "内置确定性 Embedding", "providerKey": "deterministic", "modelName": "xianzhi-hash-embedding-v1", "dimension": 256, "batchSize": 64, "normalized": true, "status": "ACTIVE"},
	}
	memory.adminProfiles["vector-stores"] = map[string]map[string]any{
		"vector_memory_default": {"id": "vector_memory_default", "tenantId": "", "name": "内存向量库", "providerKey": "memory", "collectionPrefix": "xianzhi_kb", "distanceMetric": "COSINE", "status": "ACTIVE"},
	}
	memory.adminProfiles["ingestion-profiles"] = map[string]map[string]any{
		"ingestion_default": {"id": "ingestion_default", "tenantId": "", "name": "默认解析配置", "embeddingProfileId": "embedding_deterministic_default", "vectorStoreProfileId": "vector_memory_default", "parserKey": "auto", "chunkerKey": "fixed", "chunkSize": 800, "overlap": 120, "minTokens": 40, "maxTokens": 1200, "status": "ACTIVE"},
	}
	memory.adminProfiles["retrieval-profiles"] = map[string]map[string]any{
		"retrieval_default": {"id": "retrieval_default", "tenantId": "", "name": "默认 Hybrid Search", "searchMode": "HYBRID", "topK": 8, "threshold": 0.2, "vectorWeight": 0.7, "keywordWeight": 0.3, "queryRewriteEnabled": true, "metadataFilterEnabled": true, "status": "ACTIVE"},
	}
	return memory
}

func (m *Memory) ResolveAccessContext(_ context.Context, userID string, requestedTenantID string, requestedOrganizationID string) (knowledgeapp.AccessContext, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return knowledgeapp.AccessContext{}, knowledgeapp.ErrValidation
	}
	tenantID := strings.TrimSpace(requestedTenantID)
	if tenantID == "" {
		digest := sha256.Sum256([]byte(userID))
		tenantID = "tenant_personal_" + hex.EncodeToString(digest[:8])
	}
	return knowledgeapp.AccessContext{
		TenantID:       tenantID,
		OrganizationID: strings.TrimSpace(requestedOrganizationID),
		UserID:         userID,
		Roles:          []string{"MEMBER"},
		Permissions:    []string{"knowledge.view", "knowledge.upload", "knowledge.edit", "knowledge.delete", "knowledge.share", "knowledge.agent.bind"},
	}, nil
}

func (m *Memory) ResolveIngestionRuntimeProfile(_ context.Context, _ knowledgeapp.AccessContext, id string) (knowledgeapp.IngestionRuntimeProfile, error) {
	if strings.TrimSpace(id) == "" {
		id = "ingestion_default"
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if item := m.adminProfiles["ingestion-profiles"][id]; item != nil {
		embeddingItem := m.adminProfiles["embedding-profiles"][adminStringValue(item["embeddingProfileId"])]
		vectorItem := m.adminProfiles["vector-stores"][adminStringValue(item["vectorStoreProfileId"])]
		if embeddingItem == nil || vectorItem == nil {
			return knowledgeapp.IngestionRuntimeProfile{}, knowledgeapp.ErrNotFound
		}
		return knowledgeapp.IngestionRuntimeProfile{
			ID: id, ParserKey: adminStringValue(item["parserKey"]), OCRProviderKey: adminStringValue(item["ocrProviderKey"]), ChunkerKey: adminStringValue(item["chunkerKey"]),
			ChunkOptions: knowledgeapp.ChunkOptions{ChunkSize: memoryAdminInt(item["chunkSize"], 800), Overlap: memoryAdminInt(item["overlap"], 120), MinTokens: memoryAdminInt(item["minTokens"], 40), MaxTokens: memoryAdminInt(item["maxTokens"], 1200)},
			Embedding:    knowledgeapp.EmbeddingRuntimeProfile{ID: adminStringValue(embeddingItem["id"]), ProviderKey: adminStringValue(embeddingItem["providerKey"]), ModelName: adminStringValue(embeddingItem["modelName"]), Dimension: memoryAdminInt(embeddingItem["dimension"], 256), Config: memoryAdminMap(embeddingItem["config"])},
			VectorStore:  knowledgeapp.VectorStoreRuntimeProfile{ID: adminStringValue(vectorItem["id"]), ProviderKey: adminStringValue(vectorItem["providerKey"]), Endpoint: adminStringValue(vectorItem["endpoint"]), CredentialRef: adminStringValue(vectorItem["credentialRef"]), CollectionPrefix: adminStringValue(vectorItem["collectionPrefix"]), Config: memoryAdminMap(vectorItem["config"])},
		}, nil
	}
	return knowledgeapp.IngestionRuntimeProfile{
		ID: id, ParserKey: "auto", ChunkerKey: "fixed",
		ChunkOptions: knowledgeapp.ChunkOptions{ChunkSize: 800, Overlap: 120, MinTokens: 40, MaxTokens: 1200},
		Embedding:    knowledgeapp.EmbeddingRuntimeProfile{ID: "embedding_deterministic_default", ProviderKey: "deterministic", ModelName: "xianzhi-hash-embedding-v1", Dimension: 256, Config: map[string]any{}},
		VectorStore:  knowledgeapp.VectorStoreRuntimeProfile{ID: "vector_memory_default", ProviderKey: "memory", Config: map[string]any{}},
	}, nil
}

func (m *Memory) ResolveRetrievalRuntimeProfile(_ context.Context, _ knowledgeapp.AccessContext, id string) (knowledgeapp.RetrievalRuntimeProfile, error) {
	if strings.TrimSpace(id) == "" {
		id = "retrieval_default"
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if item := m.adminProfiles["retrieval-profiles"][id]; item != nil {
		return knowledgeapp.RetrievalRuntimeProfile{
			ID: id, SearchMode: adminStringValue(item["searchMode"]), TopK: memoryAdminInt(item["topK"], 8), Threshold: memoryAdminFloat(item["threshold"], 0.2),
			VectorWeight: memoryAdminFloat(item["vectorWeight"], 0.7), KeywordWeight: memoryAdminFloat(item["keywordWeight"], 0.3),
			RerankProfileID: adminStringValue(item["rerankProfileId"]), ContextTokenLimit: memoryAdminInt(item["contextTokenLimit"], 6000),
			QueryRewriteEnabled: memoryAdminBool(item["queryRewriteEnabled"], true), MetadataFilterEnabled: memoryAdminBool(item["metadataFilterEnabled"], true), Config: memoryAdminMap(item["config"]),
		}, nil
	}
	return knowledgeapp.RetrievalRuntimeProfile{
		ID: id, SearchMode: "HYBRID", TopK: 8, Threshold: 0.2, VectorWeight: 0.7, KeywordWeight: 0.3,
		ContextTokenLimit: 6000, QueryRewriteEnabled: true, MetadataFilterEnabled: true, Config: map[string]any{},
	}, nil
}

func memoryAdminInt(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	default:
		return fallback
	}
}

func memoryAdminFloat(value any, fallback float64) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case float64:
		return typed
	default:
		return fallback
	}
}

func memoryAdminBool(value any, fallback bool) bool {
	if typed, ok := value.(bool); ok {
		return typed
	}
	return fallback
}

func memoryAdminMap(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return cloneAdminRecord(typed)
	}
	return map[string]any{}
}

func (m *Memory) CreateKnowledgeBase(_ context.Context, access knowledgeapp.AccessContext, item knowledgeapp.KnowledgeBase) (knowledgeapp.KnowledgeBase, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if item.TenantID != access.TenantID {
		return knowledgeapp.KnowledgeBase{}, knowledgeapp.ErrForbidden
	}
	if _, exists := m.bases[item.ID]; exists {
		return knowledgeapp.KnowledgeBase{}, knowledgeapp.ErrConflict
	}
	m.bases[item.ID] = item
	return item, nil
}

func (m *Memory) ListKnowledgeBases(_ context.Context, access knowledgeapp.AccessContext, options knowledgeapp.ListOptions) ([]knowledgeapp.KnowledgeBase, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := make([]knowledgeapp.KnowledgeBase, 0)
	query := strings.ToLower(strings.TrimSpace(options.Query))
	for _, item := range m.bases {
		if item.TenantID != access.TenantID || item.DeletedAt != nil {
			continue
		}
		if options.Status != "" && item.Status != options.Status {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(item.Name+" "+item.Description), query) {
			continue
		}
		if item.OwnerUserID != access.UserID && !access.HasRole("PLATFORM_ADMIN", "ENTERPRISE_ADMIN") {
			aclAllowed, aclDenied := memoryACLDecision(access, m.acl[item.ID], "VIEW")
			visible := item.Visibility == "TENANT" || item.Visibility == "SHARED" || (item.Visibility == "ORGANIZATION" && access.OrganizationID != "" && access.OrganizationID == item.OrganizationID)
			if aclDenied || (!visible && !aclAllowed) {
				continue
			}
		}
		item.Tags = m.memoryTagsForBase(item.ID)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	if len(items) > limit {
		return items[:limit], items[limit-1].ID, nil
	}
	return items, "", nil
}

func memoryACLDecision(access knowledgeapp.AccessContext, rules []knowledgeapp.ACLRule, permission string) (bool, bool) {
	allowed := false
	for _, rule := range rules {
		if rule.ExpiresAt != nil && rule.ExpiresAt.Before(time.Now().UTC()) {
			continue
		}
		matches := false
		switch strings.ToUpper(rule.SubjectType) {
		case "USER":
			matches = rule.SubjectID == access.UserID
		case "ORGANIZATION", "DEPARTMENT":
			matches = rule.SubjectID != "" && rule.SubjectID == access.OrganizationID
		case "TENANT":
			matches = rule.SubjectID == "" || rule.SubjectID == access.TenantID
		case "ROLE":
			matches = access.HasRole(rule.SubjectID)
		case "EVERYONE", "GUEST":
			matches = true
		}
		rulePermission := strings.ToUpper(strings.TrimSpace(rule.Permission))
		if !matches || (rulePermission != permission && rulePermission != "MANAGE" && !(permission == "VIEW" && rulePermission == "READ")) {
			continue
		}
		if strings.EqualFold(rule.Effect, "DENY") {
			return false, true
		}
		if strings.EqualFold(rule.Effect, "ALLOW") {
			allowed = true
		}
	}
	return allowed, false
}

func (m *Memory) GetKnowledgeBase(_ context.Context, access knowledgeapp.AccessContext, id string) (knowledgeapp.KnowledgeBase, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, exists := m.bases[id]
	if !exists || item.TenantID != access.TenantID || item.DeletedAt != nil {
		return knowledgeapp.KnowledgeBase{}, knowledgeapp.ErrNotFound
	}
	item.Tags = m.memoryTagsForBase(item.ID)
	return item, nil
}

func (m *Memory) memoryTagsForBase(knowledgeBaseID string) []knowledgeapp.Tag {
	items := make([]knowledgeapp.Tag, 0, len(m.baseTags[knowledgeBaseID]))
	for _, tagID := range m.baseTags[knowledgeBaseID] {
		if tag, ok := m.tags[tagID]; ok {
			items = append(items, tag)
		}
	}
	return items
}

func (m *Memory) UpdateKnowledgeBase(_ context.Context, access knowledgeapp.AccessContext, item knowledgeapp.KnowledgeBase, expectedVersion int64) (knowledgeapp.KnowledgeBase, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, exists := m.bases[item.ID]
	if !exists || current.TenantID != access.TenantID || current.DeletedAt != nil {
		return knowledgeapp.KnowledgeBase{}, knowledgeapp.ErrNotFound
	}
	if expectedVersion > 0 && current.Version != expectedVersion {
		return knowledgeapp.KnowledgeBase{}, knowledgeapp.ErrConflict
	}
	item.Version = current.Version + 1
	m.bases[item.ID] = item
	return item, nil
}

func (m *Memory) SoftDeleteKnowledgeBase(_ context.Context, access knowledgeapp.AccessContext, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, exists := m.bases[id]
	if !exists || item.TenantID != access.TenantID || item.DeletedAt != nil {
		return knowledgeapp.ErrNotFound
	}
	now := time.Now().UTC()
	item.DeletedAt = &now
	item.Status = "DELETING"
	item.UpdatedAt = now
	item.Version++
	m.bases[id] = item
	return nil
}

func (m *Memory) ReplaceKnowledgeBaseACL(_ context.Context, access knowledgeapp.AccessContext, knowledgeBaseID string, rules []knowledgeapp.ACLRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, exists := m.bases[knowledgeBaseID]
	if !exists || item.TenantID != access.TenantID {
		return knowledgeapp.ErrNotFound
	}
	copyRules := make([]knowledgeapp.ACLRule, len(rules))
	copy(copyRules, rules)
	m.acl[knowledgeBaseID] = copyRules
	return nil
}

func (m *Memory) ListKnowledgeBaseACL(_ context.Context, access knowledgeapp.AccessContext, knowledgeBaseID string) ([]knowledgeapp.ACLRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, exists := m.bases[knowledgeBaseID]
	if !exists || item.TenantID != access.TenantID {
		return nil, knowledgeapp.ErrNotFound
	}
	rules := append([]knowledgeapp.ACLRule(nil), m.acl[knowledgeBaseID]...)
	return rules, nil
}

func (m *Memory) CreateDocumentBundle(_ context.Context, access knowledgeapp.AccessContext, document knowledgeapp.Document, version knowledgeapp.DocumentVersion, units []knowledgeapp.DocumentUnit, chunks []knowledgeapp.Chunk, job knowledgeapp.IngestionJob) (knowledgeapp.Document, knowledgeapp.IngestionJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	base, exists := m.bases[document.KnowledgeBaseID]
	if !exists || base.TenantID != access.TenantID || document.TenantID != access.TenantID {
		return knowledgeapp.Document{}, knowledgeapp.IngestionJob{}, knowledgeapp.ErrNotFound
	}
	if _, exists := m.documents[document.ID]; exists {
		return knowledgeapp.Document{}, knowledgeapp.IngestionJob{}, knowledgeapp.ErrConflict
	}
	document.LatestVersionID = version.ID
	m.documents[document.ID] = document
	m.versions[version.ID] = version
	m.units[version.ID] = append([]knowledgeapp.DocumentUnit(nil), units...)
	for _, chunk := range chunks {
		m.chunks[chunk.ID] = chunk
	}
	m.jobs[job.ID] = job
	base.DocumentCount++
	base.ChunkCount += int64(len(chunks))
	base.UpdatedAt = time.Now().UTC()
	m.bases[base.ID] = base
	return document, job, nil
}

func (m *Memory) ListDocuments(_ context.Context, access knowledgeapp.AccessContext, knowledgeBaseID string, options knowledgeapp.ListOptions) ([]knowledgeapp.Document, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := make([]knowledgeapp.Document, 0)
	query := strings.ToLower(strings.TrimSpace(options.Query))
	for _, item := range m.documents {
		if item.TenantID != access.TenantID || item.KnowledgeBaseID != knowledgeBaseID || item.DeletedAt != nil {
			continue
		}
		if options.Status != "" && item.Status != options.Status {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(item.Name), query) {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	if len(items) > limit {
		return items[:limit], items[limit-1].ID, nil
	}
	return items, "", nil
}

func (m *Memory) GetDocument(_ context.Context, access knowledgeapp.AccessContext, id string) (knowledgeapp.Document, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, exists := m.documents[id]
	if !exists || item.TenantID != access.TenantID || item.DeletedAt != nil {
		return knowledgeapp.Document{}, knowledgeapp.ErrNotFound
	}
	return item, nil
}

func (m *Memory) SoftDeleteDocument(_ context.Context, access knowledgeapp.AccessContext, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, exists := m.documents[id]
	if !exists || item.TenantID != access.TenantID || item.DeletedAt != nil {
		return knowledgeapp.ErrNotFound
	}
	now := time.Now().UTC()
	item.Status = "DELETED"
	item.DeletedAt = &now
	item.UpdatedAt = now
	item.Version++
	m.documents[id] = item
	removedChunks := int64(0)
	for chunkID, chunk := range m.chunks {
		if chunk.TenantID == access.TenantID && chunk.DocumentID == id && chunk.DeletedAt == nil {
			chunk.Status = "DELETED"
			chunk.DeletedAt = &now
			chunk.UpdatedAt = now
			m.chunks[chunkID] = chunk
			removedChunks++
		}
	}
	base := m.bases[item.KnowledgeBaseID]
	if base.DocumentCount > 0 {
		base.DocumentCount--
	}
	if base.ChunkCount >= removedChunks {
		base.ChunkCount -= removedChunks
	} else {
		base.ChunkCount = 0
	}
	base.UpdatedAt = now
	m.bases[base.ID] = base
	return nil
}

func (m *Memory) ListChunks(_ context.Context, access knowledgeapp.AccessContext, knowledgeBaseIDs []string, options knowledgeapp.ListOptions) ([]knowledgeapp.Chunk, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	allowed := make(map[string]bool, len(knowledgeBaseIDs))
	for _, id := range knowledgeBaseIDs {
		allowed[id] = true
	}
	items := make([]knowledgeapp.Chunk, 0)
	for _, item := range m.chunks {
		if item.TenantID != access.TenantID || item.DeletedAt != nil || (len(allowed) > 0 && !allowed[item.KnowledgeBaseID]) {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].DocumentID == items[j].DocumentID {
			return items[i].SequenceNo < items[j].SequenceNo
		}
		return items[i].DocumentID < items[j].DocumentID
	})
	if options.Limit > 0 && len(items) > options.Limit {
		items = items[:options.Limit]
	}
	return items, nil
}

func (m *Memory) ReplaceChunks(_ context.Context, access knowledgeapp.AccessContext, documentVersionID string, chunks []knowledgeapp.Chunk) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, chunk := range m.chunks {
		if chunk.TenantID == access.TenantID && chunk.DocumentVersionID == documentVersionID {
			delete(m.chunks, id)
		}
	}
	for _, chunk := range chunks {
		if chunk.TenantID != access.TenantID {
			return knowledgeapp.ErrForbidden
		}
		m.chunks[chunk.ID] = chunk
	}
	return nil
}

func (m *Memory) UpdateIngestionJob(_ context.Context, access knowledgeapp.AccessContext, job knowledgeapp.IngestionJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, exists := m.jobs[job.ID]
	if !exists || current.TenantID != access.TenantID {
		return knowledgeapp.ErrNotFound
	}
	m.jobs[job.ID] = job
	return nil
}

func (m *Memory) UpdateDocumentStatus(_ context.Context, access knowledgeapp.AccessContext, documentID string, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, exists := m.documents[documentID]
	if !exists || item.TenantID != access.TenantID {
		return knowledgeapp.ErrNotFound
	}
	item.Status = status
	item.UpdatedAt = time.Now().UTC()
	item.Version++
	m.documents[documentID] = item
	return nil
}
