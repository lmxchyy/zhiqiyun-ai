package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
	"xianzhi-ai/backend-go/internal/config"
	answerprovider "xianzhi-ai/backend-go/internal/provider/answer"
	chatprovider "xianzhi-ai/backend-go/internal/provider/chat"
	"xianzhi-ai/backend-go/internal/provider/chunker"
	"xianzhi-ai/backend-go/internal/provider/cleaner"
	"xianzhi-ai/backend-go/internal/provider/embedding"
	"xianzhi-ai/backend-go/internal/provider/knowledgeprofile"
	"xianzhi-ai/backend-go/internal/provider/ocr"
	"xianzhi-ai/backend-go/internal/provider/parser"
	"xianzhi-ai/backend-go/internal/provider/queryrewrite"
	"xianzhi-ai/backend-go/internal/provider/rerank"
	"xianzhi-ai/backend-go/internal/provider/vectorstore"
	knowledgerepo "xianzhi-ai/backend-go/internal/repository/knowledge"
)

const maxInlineKnowledgeDocumentBytes = 20 << 20

func sanitizeKnowledgeProfileRecords(items []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if sanitized, ok := sanitizeKnowledgeProfileValue(item).(map[string]any); ok {
			result = append(result, sanitized)
		}
	}
	return result
}

func sanitizeKnowledgeProfileValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSensitiveKnowledgeProfileKey(key) {
				result[key+"Configured"] = strings.TrimSpace(fmt.Sprint(item)) != ""
				continue
			}
			result[key] = sanitizeKnowledgeProfileValue(item)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = sanitizeKnowledgeProfileValue(item)
		}
		return result
	default:
		return value
	}
}

func isSensitiveKnowledgeProfileKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(key)))
	switch normalized {
	case "apikey", "secret", "password", "token", "accesstoken", "refreshtoken", "authorization":
		return true
	default:
		return strings.HasSuffix(normalized, "password") || strings.HasSuffix(normalized, "secret")
	}
}

type knowledgeModule struct {
	core      *knowledgeapp.Service
	ingestion *knowledgeapp.IngestionService
	retrieval *knowledgeapp.RetrievalService
	agents    *knowledgeapp.AgentService
	rag       *knowledgeapp.RAGService
	repo      knowledgeapp.KnowledgeRepository
	admin     knowledgeapp.AdminRepository
}

func newMemoryKnowledgeModule(cfg config.Config) *knowledgeModule {
	repo := knowledgerepo.NewMemory()
	vectors := vectorstore.NewMemory()
	return buildKnowledgeModule(cfg, repo, repo, repo, repo, vectors)
}

func newPostgresKnowledgeModule(cfg config.Config, db *sql.DB) *knowledgeModule {
	repo := knowledgerepo.NewPostgres(db)
	return buildKnowledgeModule(cfg, repo, repo, repo, repo, vectorstore.NewPGVector(db))
}

func buildKnowledgeModule(cfg config.Config, tenants knowledgeapp.TenantRepository, repo knowledgeapp.KnowledgeRepository, agents knowledgeapp.AgentRepository, runs knowledgeapp.RAGRepository, vectors knowledgeapp.VectorStore) *knowledgeModule {
	core := knowledgeapp.NewService(tenants, repo)
	embedder := embedding.NewDeterministic(256)
	retrieval := knowledgeapp.NewRetrievalService(embedder, vectors, rerank.None{})
	vectorRegistry := vectorstore.NewRegistry(vectors)
	timeoutMS, _ := strconv.Atoi(strings.TrimSpace(cfg.ModelTimeoutMS))
	runtimeResolver := knowledgeprofile.NewResolver(repo.(knowledgeapp.RuntimeProfileRepository), vectorRegistry, cfg.ModelProviderURL, cfg.ModelProviderAPIKey, timeoutMS)
	retrieval.SetRuntimeResolver(core, runtimeResolver)
	fallback := answerprovider.Context{}
	chatRouter := chatprovider.NewRouter(chatprovider.NewOpenAICompatibleForModels(cfg, "deepseek-v4-flash", "gpt-5.2-chat-latest"))
	var ocrProvider knowledgeapp.OCRProvider
	if strings.TrimSpace(cfg.KnowledgeOCREndpoint) != "" {
		ocrProvider = ocr.NewHTTP(cfg.KnowledgeOCRProvider, cfg.KnowledgeOCREndpoint, cfg.KnowledgeOCRAPIKey, 0)
	}
	ingestion := knowledgeapp.NewIngestionServiceWithNormalizer(core, repo, parser.NewDefaultRegistryWithOCR(ocrProvider), chunker.NewDefaultRegistry(), embedder, vectors, cleaner.Standard{})
	ingestion.SetRuntimeResolver(runtimeResolver)
	return &knowledgeModule{
		core:      core,
		ingestion: ingestion,
		retrieval: retrieval,
		agents:    knowledgeapp.NewAgentService(core, agents),
		rag:       knowledgeapp.NewRAGService(agents, runs, retrieval, queryrewrite.Latest{}, answerprovider.NewChat(chatRouter, fallback)),
		repo:      repo,
		admin:     repo.(knowledgeapp.AdminRepository),
	}
}

type knowledgeAPI struct {
	module     *knowledgeModule
	sessions   authSessionStore
	authorizer modelCallAuthorizer
}

func newKnowledgeAPI(module *knowledgeModule, sessions authSessionStore, store platformStore) knowledgeAPI {
	authorizer, _ := store.(modelCallAuthorizer)
	return knowledgeAPI{module: module, sessions: sessions, authorizer: authorizer}
}

func (a knowledgeAPI) requireModelCall(w http.ResponseWriter, access knowledgeapp.AccessContext) bool {
	if a.authorizer == nil {
		return true
	}
	authorization, err := a.authorizer.AuthorizeModelCall(access.UserID, "knowledge_agent")
	if err != nil {
		writeModelAuthorizationError(w, err)
		return false
	}
	if authorization.ContextType == contextEnterprise && authorization.TenantID != access.TenantID {
		writeModelAuthorizationError(w, errForbidden)
		return false
	}
	return true
}

func (a knowledgeAPI) access(r *http.Request) (knowledgeapp.AccessContext, error) {
	if a.module == nil || a.module.core == nil {
		return knowledgeapp.AccessContext{}, errors.New("knowledge module is unavailable")
	}
	userID, err := authenticatedUserID(r, a.sessions)
	if err != nil {
		return knowledgeapp.AccessContext{}, err
	}
	requestedTenantID := strings.TrimSpace(r.Header.Get("X-Tenant-Id"))
	requestedOrganizationID := strings.TrimSpace(r.Header.Get("X-Organization-Id"))
	access, err := a.module.core.ResolveAccessContext(r.Context(), userID, requestedTenantID, requestedOrganizationID)
	if err == nil || requestedTenantID == "" || !errors.Is(err, knowledgeapp.ErrForbidden) {
		return access, err
	}
	// A cached tenant header can outlive a role/context switch in the mini program.
	// Falling back to the user's own resolved tenant never grants access to the
	// rejected tenant, but keeps personal Agent creation usable after that switch.
	return a.module.core.ResolveAccessContext(r.Context(), userID, "", "")
}

func (a knowledgeAPI) context(w http.ResponseWriter, r *http.Request) {
	access, err := a.access(r)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, access)
}

func (a knowledgeAPI) adminOverview(w http.ResponseWriter, r *http.Request) {
	if _, err := authenticatedUserID(r, a.sessions); err != nil {
		writeKnowledgeError(w, err)
		return
	}
	if a.module == nil || a.module.admin == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("knowledge admin repository is unavailable"))
		return
	}
	item, err := a.module.admin.KnowledgeAdminOverview(r.Context(), strings.TrimSpace(r.URL.Query().Get("tenantId")))
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, item)
}

func (a knowledgeAPI) adminRecords(w http.ResponseWriter, r *http.Request) {
	if _, err := authenticatedUserID(r, a.sessions); err != nil {
		writeKnowledgeError(w, err)
		return
	}
	if a.module == nil || a.module.admin == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("knowledge admin repository is unavailable"))
		return
	}
	items, err := a.module.admin.ListKnowledgeAdminRecords(
		r.Context(), r.PathValue("resource"), strings.TrimSpace(r.URL.Query().Get("tenantId")), listOptions(r),
	)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": sanitizeKnowledgeProfileRecords(items)})
}

func (a knowledgeAPI) listProfiles(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	resource := strings.ToLower(strings.TrimSpace(r.PathValue("resource")))
	if resource != "embedding-profiles" && resource != "vector-stores" && resource != "ingestion-profiles" && resource != "retrieval-profiles" {
		writeKnowledgeError(w, knowledgeapp.ErrValidation)
		return
	}
	items, err := a.module.admin.ListKnowledgeAdminRecords(r.Context(), resource, access.TenantID, knowledgeapp.ListOptions{Limit: 200})
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": sanitizeKnowledgeProfileRecords(items)})
}

func (a knowledgeAPI) getDocument(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	item, err := a.module.repo.GetDocument(r.Context(), access, r.PathValue("id"))
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	if _, err := a.module.core.GetKnowledgeBase(r.Context(), access, item.KnowledgeBaseID); err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, item)
}

func (a knowledgeAPI) saveAdminProfile(w http.ResponseWriter, r *http.Request) {
	if _, err := authenticatedUserID(r, a.sessions); err != nil {
		writeKnowledgeError(w, err)
		return
	}
	if a.module == nil || a.module.admin == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("knowledge admin repository is unavailable"))
		return
	}
	request := map[string]any{}
	if !decodeKnowledgeJSON(w, r, &request) {
		return
	}
	if pathID := strings.TrimSpace(r.PathValue("id")); pathID != "" {
		request["id"] = pathID
	}
	if strings.TrimSpace(fmt.Sprint(request["id"])) == "" {
		request["id"] = "profile_" + newRequestID()
	}
	item, err := a.module.admin.SaveKnowledgeAdminProfile(r.Context(), r.PathValue("resource"), request)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	sanitized, _ := sanitizeKnowledgeProfileValue(item).(map[string]any)
	writeKnowledgeJSON(w, http.StatusOK, map[string]any{"item": sanitized})
}

type createKnowledgeBaseRequest struct {
	Name               string                     `json:"name"`
	Description        string                     `json:"description"`
	OrganizationID     string                     `json:"organizationId"`
	CategoryID         string                     `json:"categoryId"`
	KnowledgeType      knowledgeapp.KnowledgeType `json:"knowledgeType"`
	Visibility         string                     `json:"visibility"`
	LogoObjectKey      string                     `json:"logoObjectKey"`
	IngestionProfileID string                     `json:"ingestionProfileId"`
	RetrievalProfileID string                     `json:"retrievalProfileId"`
	Metadata           map[string]any             `json:"metadata"`
	TagIDs             []string                   `json:"tagIds"`
}

func (a knowledgeAPI) createKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	var request createKnowledgeBaseRequest
	if !decodeKnowledgeJSON(w, r, &request) {
		return
	}
	item, err := a.module.core.CreateKnowledgeBase(r.Context(), access, knowledgeapp.CreateKnowledgeBaseInput{
		Name: request.Name, Description: request.Description, OrganizationID: request.OrganizationID, CategoryID: request.CategoryID,
		KnowledgeType: request.KnowledgeType, Visibility: request.Visibility, LogoObjectKey: request.LogoObjectKey,
		IngestionProfileID: request.IngestionProfileID, RetrievalProfileID: request.RetrievalProfileID, Metadata: request.Metadata,
		TagIDs: request.TagIDs,
	})
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeKnowledgeJSON(w, http.StatusCreated, item)
}

func (a knowledgeAPI) listKnowledgeBases(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	items, cursor, err := a.module.core.ListKnowledgeBases(r.Context(), access, listOptions(r))
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "nextCursor": cursor})
}

func (a knowledgeAPI) getKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	item, err := a.module.core.GetKnowledgeBase(r.Context(), access, r.PathValue("id"))
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, item)
}

type updateKnowledgeBaseRequest struct {
	Name               *string        `json:"name"`
	Description        *string        `json:"description"`
	OrganizationID     *string        `json:"organizationId"`
	CategoryID         *string        `json:"categoryId"`
	Visibility         *string        `json:"visibility"`
	Status             *string        `json:"status"`
	LogoObjectKey      *string        `json:"logoObjectKey"`
	IngestionProfileID *string        `json:"ingestionProfileId"`
	RetrievalProfileID *string        `json:"retrievalProfileId"`
	Metadata           map[string]any `json:"metadata"`
	ExpectedVersion    int64          `json:"expectedVersion"`
	TagIDs             []string       `json:"tagIds"`
}

func (a knowledgeAPI) updateKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	var request updateKnowledgeBaseRequest
	if !decodeKnowledgeJSON(w, r, &request) {
		return
	}
	item, err := a.module.core.UpdateKnowledgeBase(r.Context(), access, r.PathValue("id"), knowledgeapp.UpdateKnowledgeBaseInput{
		Name: request.Name, Description: request.Description, OrganizationID: request.OrganizationID, CategoryID: request.CategoryID,
		Visibility: request.Visibility, Status: request.Status, LogoObjectKey: request.LogoObjectKey,
		IngestionProfileID: request.IngestionProfileID, RetrievalProfileID: request.RetrievalProfileID,
		Metadata: request.Metadata, ExpectedVersion: request.ExpectedVersion, TagIDs: request.TagIDs,
	})
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, item)
}

func (a knowledgeAPI) listTags(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	items, err := a.module.core.ListTags(r.Context(), access)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a knowledgeAPI) createTag(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	var request struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if !decodeKnowledgeJSON(w, r, &request) {
		return
	}
	item, err := a.module.core.SaveTag(r.Context(), access, request.Name, request.Color)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeKnowledgeJSON(w, http.StatusCreated, item)
}

func (a knowledgeAPI) listCategories(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	items, err := a.module.core.ListCategories(r.Context(), access)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a knowledgeAPI) createCategory(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	var request struct {
		Name      string `json:"name"`
		ParentID  string `json:"parentId"`
		SortOrder int    `json:"sortOrder"`
	}
	if !decodeKnowledgeJSON(w, r, &request) {
		return
	}
	item, err := a.module.core.SaveCategory(r.Context(), access, request.Name, request.ParentID, request.SortOrder)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeKnowledgeJSON(w, http.StatusCreated, item)
}

func (a knowledgeAPI) deleteKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	if err := a.module.core.DeleteKnowledgeBase(r.Context(), access, r.PathValue("id")); err != nil {
		writeKnowledgeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a knowledgeAPI) listKnowledgeBaseACL(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	if _, err := a.module.core.GetKnowledgeBase(r.Context(), access, r.PathValue("id")); err != nil {
		writeKnowledgeError(w, err)
		return
	}
	items, err := a.module.repo.ListKnowledgeBaseACL(r.Context(), access, r.PathValue("id"))
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a knowledgeAPI) replaceKnowledgeBaseACL(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	base, err := a.module.core.GetKnowledgeBase(r.Context(), access, r.PathValue("id"))
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	allowed, err := a.module.core.AuthorizeKnowledgeBase(r.Context(), access, base, "SHARE")
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	if !allowed {
		writeKnowledgeError(w, knowledgeapp.ErrForbidden)
		return
	}
	var request struct {
		Items []knowledgeapp.ACLRule `json:"items"`
	}
	if !decodeKnowledgeJSON(w, r, &request) {
		return
	}
	if len(request.Items) > 100 {
		writeKnowledgeError(w, fmt.Errorf("at most 100 ACL rules are allowed: %w", knowledgeapp.ErrValidation))
		return
	}
	for index := range request.Items {
		request.Items[index].ID = "acl_" + newRequestID()
		request.Items[index].TenantID = access.TenantID
		request.Items[index].KnowledgeBaseID = base.ID
		request.Items[index].SubjectType = strings.ToUpper(strings.TrimSpace(request.Items[index].SubjectType))
		request.Items[index].Permission = strings.ToUpper(strings.TrimSpace(request.Items[index].Permission))
		request.Items[index].Effect = strings.ToUpper(strings.TrimSpace(request.Items[index].Effect))
		if request.Items[index].Effect == "" {
			request.Items[index].Effect = "ALLOW"
		}
		if !knowledgeACLValueAllowed(request.Items[index].SubjectType, "USER", "ROLE", "ORGANIZATION", "DEPARTMENT", "TENANT", "EVERYONE", "GUEST", "SHARE") ||
			!knowledgeACLValueAllowed(request.Items[index].Permission, "READ", "VIEW", "UPLOAD", "EDIT", "DELETE", "SHARE", "MANAGE") ||
			!knowledgeACLValueAllowed(request.Items[index].Effect, "ALLOW", "DENY") {
			writeKnowledgeError(w, fmt.Errorf("invalid ACL rule at index %d: %w", index, knowledgeapp.ErrValidation))
			return
		}
	}
	if err := a.module.repo.ReplaceKnowledgeBaseACL(r.Context(), access, base.ID, request.Items); err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": request.Items})
}

func knowledgeACLValueAllowed(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

type ingestDocumentRequest struct {
	Name         string                    `json:"name"`
	MIMEType     string                    `json:"mimeType"`
	Content      string                    `json:"content"`
	ObjectKey    string                    `json:"objectKey"`
	Metadata     map[string]any            `json:"metadata"`
	ChunkerKey   string                    `json:"chunkerKey"`
	ChunkOptions knowledgeapp.ChunkOptions `json:"chunkOptions"`
}

func (a knowledgeAPI) ingestDocument(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	request, err := readIngestDocumentRequest(w, r)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	result, err := a.module.ingestion.Ingest(r.Context(), access, r.PathValue("id"), knowledgeapp.IngestDocumentInput{
		Name: request.Name, MIMEType: request.MIMEType, ObjectKey: request.ObjectKey, Content: []byte(request.Content), Metadata: request.Metadata,
		ChunkerKey: request.ChunkerKey, ChunkOptions: request.ChunkOptions,
	})
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeKnowledgeJSON(w, http.StatusCreated, result)
}

func (a knowledgeAPI) listDocuments(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	if _, err := a.module.core.GetKnowledgeBase(r.Context(), access, r.PathValue("id")); err != nil {
		writeKnowledgeError(w, err)
		return
	}
	items, cursor, err := a.module.repo.ListDocuments(r.Context(), access, r.PathValue("id"), listOptions(r))
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "nextCursor": cursor})
}

func (a knowledgeAPI) listChunks(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	baseID := r.URL.Query().Get("knowledgeBaseId")
	baseIDs := []string{}
	if baseID != "" {
		if _, err := a.module.core.GetKnowledgeBase(r.Context(), access, baseID); err != nil {
			writeKnowledgeError(w, err)
			return
		}
		baseIDs = append(baseIDs, baseID)
	} else {
		bases, _, err := a.module.core.ListKnowledgeBases(r.Context(), access, knowledgeapp.ListOptions{Limit: 100})
		if err != nil {
			writeKnowledgeError(w, err)
			return
		}
		for _, base := range bases {
			baseIDs = append(baseIDs, base.ID)
		}
		if len(baseIDs) == 0 {
			writeJSON(w, map[string]any{"items": []knowledgeapp.Chunk{}})
			return
		}
	}
	items, err := a.module.repo.ListChunks(r.Context(), access, baseIDs, listOptions(r))
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a knowledgeAPI) deleteDocument(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	if err := a.module.ingestion.DeleteDocument(r.Context(), access, r.PathValue("id")); err != nil {
		writeKnowledgeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a knowledgeAPI) search(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	var request struct {
		KnowledgeBaseIDs []string       `json:"knowledgeBaseIds"`
		Query            string         `json:"query"`
		Mode             string         `json:"mode"`
		TopK             int            `json:"topK"`
		Threshold        float64        `json:"threshold"`
		VectorWeight     float64        `json:"vectorWeight"`
		KeywordWeight    float64        `json:"keywordWeight"`
		Filters          map[string]any `json:"filters"`
	}
	if !decodeKnowledgeJSON(w, r, &request) {
		return
	}
	for _, id := range request.KnowledgeBaseIDs {
		if _, err := a.module.core.GetKnowledgeBase(r.Context(), access, id); err != nil {
			writeKnowledgeError(w, err)
			return
		}
	}
	hits, err := a.module.retrieval.Search(r.Context(), knowledgeapp.SearchRequest{
		Access: access, KnowledgeBaseIDs: request.KnowledgeBaseIDs, Query: request.Query, Mode: request.Mode,
		TopK: request.TopK, Threshold: request.Threshold, VectorWeight: request.VectorWeight, KeywordWeight: request.KeywordWeight, Filters: request.Filters,
	})
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": hits})
}

type createAgentRequest struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	ModelName    string         `json:"modelName"`
	SystemPrompt string         `json:"systemPrompt"`
	Status       string         `json:"status"`
	Config       map[string]any `json:"config"`
}

func (a knowledgeAPI) createAgent(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	var request createAgentRequest
	if !decodeKnowledgeJSON(w, r, &request) {
		return
	}
	item, err := a.module.agents.CreateAgent(r.Context(), access, knowledgeapp.CreateAgentInput{
		Name: request.Name, Description: request.Description, ModelName: request.ModelName,
		SystemPrompt: request.SystemPrompt, Status: request.Status, Config: request.Config,
	})
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeKnowledgeJSON(w, http.StatusCreated, item)
}

func (a knowledgeAPI) listAgents(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	items, cursor, err := a.module.agents.ListAgents(r.Context(), access, listOptions(r))
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "nextCursor": cursor})
}

func (a knowledgeAPI) getAgent(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	item, err := a.module.agents.GetAgent(r.Context(), access, r.PathValue("id"))
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	bindings, err := a.module.agents.ListBindings(r.Context(), access, item.ID)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"agent": item, "knowledgeBindings": bindings})
}

func (a knowledgeAPI) replaceAgentBindings(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	var request struct {
		Items []knowledgeapp.BindKnowledgeBaseInput `json:"items"`
	}
	if !decodeKnowledgeJSON(w, r, &request) {
		return
	}
	items, err := a.module.agents.ReplaceBindings(r.Context(), access, r.PathValue("id"), request.Items)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a knowledgeAPI) createConversation(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	var request struct {
		AgentID string `json:"agentId"`
		Title   string `json:"title"`
	}
	if !decodeKnowledgeJSON(w, r, &request) {
		return
	}
	item, err := a.module.agents.CreateConversation(r.Context(), access, request.AgentID, request.Title)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeKnowledgeJSON(w, http.StatusCreated, item)
}

func (a knowledgeAPI) listConversations(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	items, cursor, err := a.module.agents.ListConversations(r.Context(), access, r.URL.Query().Get("agentId"), listOptions(r))
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "nextCursor": cursor})
}

func (a knowledgeAPI) listMessages(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	items, cursor, err := a.module.agents.ListMessages(r.Context(), access, r.PathValue("id"), listOptions(r))
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "nextCursor": cursor})
}

func (a knowledgeAPI) runRAG(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	if !a.requireModelCall(w, access) {
		return
	}
	input, ok := decodeRunInput(w, r)
	if !ok {
		return
	}
	input.ConversationID = r.PathValue("id")
	result, err := a.module.rag.Run(r.Context(), access, input)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, result)
}

func (a knowledgeAPI) streamRAG(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	if !a.requireModelCall(w, access) {
		return
	}
	input, ok := decodeRunInput(w, r)
	if !ok {
		return
	}
	input.ConversationID = r.PathValue("id")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("streaming is not supported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	sink := func(event knowledgeapp.RunEvent) error {
		return writeSSE(w, flusher, event.EventType, event)
	}
	result, err := a.module.rag.RunWithSink(r.Context(), access, input, sink)
	if err != nil {
		_ = writeSSE(w, flusher, "error", map[string]any{"error": err.Error()})
		return
	}
	_ = writeSSE(w, flusher, "result", result)
}

func (a knowledgeAPI) getRun(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	run, err := a.module.rag.GetRun(r.Context(), access, r.PathValue("id"))
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, run)
}

func (a knowledgeAPI) cancelRun(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	run, err := a.module.rag.Cancel(r.Context(), access, r.PathValue("id"))
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, run)
}

func (a knowledgeAPI) retryRun(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	if !a.requireModelCall(w, access) {
		return
	}
	previous, err := a.module.rag.GetRun(r.Context(), access, r.PathValue("id"))
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	result, err := a.module.rag.Run(r.Context(), access, knowledgeapp.RunInput{
		ConversationID: previous.ConversationID, Question: previous.OriginalQuery, RetryOfRunID: previous.ID,
	})
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, result)
}

func (a knowledgeAPI) listRunEvents(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("afterSequence"), 10, 64)
	items, err := a.module.rag.ListEvents(r.Context(), access, r.PathValue("id"), after)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a knowledgeAPI) listRunCitations(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireAccess(w, r)
	if !ok {
		return
	}
	items, err := a.module.rag.ListCitations(r.Context(), access, r.PathValue("id"))
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a knowledgeAPI) requireAccess(w http.ResponseWriter, r *http.Request) (knowledgeapp.AccessContext, bool) {
	access, err := a.access(r)
	if err != nil {
		writeKnowledgeError(w, err)
		return knowledgeapp.AccessContext{}, false
	}
	return access, true
}

func decodeKnowledgeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxInlineKnowledgeDocumentBytes+1))
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func decodeRunInput(w http.ResponseWriter, r *http.Request) (knowledgeapp.RunInput, bool) {
	var request struct {
		Question  string  `json:"question"`
		TopK      int     `json:"topK"`
		Threshold float64 `json:"threshold"`
		Mode      string  `json:"mode"`
	}
	if !decodeKnowledgeJSON(w, r, &request) {
		return knowledgeapp.RunInput{}, false
	}
	return knowledgeapp.RunInput{Question: request.Question, TopK: request.TopK, Threshold: request.Threshold, Mode: request.Mode}, true
}

func readIngestDocumentRequest(w http.ResponseWriter, r *http.Request) (ingestDocumentRequest, error) {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		var request ingestDocumentRequest
		decoder := json.NewDecoder(io.LimitReader(r.Body, maxInlineKnowledgeDocumentBytes+1))
		if err := decoder.Decode(&request); err != nil {
			return ingestDocumentRequest{}, fmt.Errorf("decode document request: %w: %w", err, knowledgeapp.ErrValidation)
		}
		return request, nil
	}
	if err := r.ParseMultipartForm(maxInlineKnowledgeDocumentBytes); err != nil {
		return ingestDocumentRequest{}, err
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return ingestDocumentRequest{}, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxInlineKnowledgeDocumentBytes+1))
	if err != nil {
		return ingestDocumentRequest{}, err
	}
	if len(raw) > maxInlineKnowledgeDocumentBytes {
		return ingestDocumentRequest{}, fmt.Errorf("file exceeds %d bytes: %w", maxInlineKnowledgeDocumentBytes, knowledgeapp.ErrValidation)
	}
	request := ingestDocumentRequest{
		Name: firstNonEmptyFormValue(r.MultipartForm, "name", header.Filename), MIMEType: firstNonEmptyFormValue(r.MultipartForm, "mimeType", header.Header.Get("Content-Type")),
		Content: string(raw), ChunkerKey: firstNonEmptyFormValue(r.MultipartForm, "chunkerKey", "fixed"),
		ChunkOptions: knowledgeapp.ChunkOptions{ChunkSize: formInt(r.MultipartForm, "chunkSize"), Overlap: formInt(r.MultipartForm, "overlap"), MaxTokens: formInt(r.MultipartForm, "maxTokens"), MinTokens: formInt(r.MultipartForm, "minTokens")},
	}
	return request, nil
}

func firstNonEmptyFormValue(form *multipart.Form, key string, fallback string) string {
	if form != nil && len(form.Value[key]) > 0 && strings.TrimSpace(form.Value[key][0]) != "" {
		return strings.TrimSpace(form.Value[key][0])
	}
	return strings.TrimSpace(fallback)
}

func formInt(form *multipart.Form, key string) int {
	value, _ := strconv.Atoi(firstNonEmptyFormValue(form, key, "0"))
	return value
}

func listOptions(r *http.Request) knowledgeapp.ListOptions {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return knowledgeapp.ListOptions{Limit: limit, Cursor: r.URL.Query().Get("cursor"), Query: r.URL.Query().Get("q"), Status: strings.ToUpper(r.URL.Query().Get("status"))}
}

func writeKnowledgeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, errUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, knowledgeapp.ErrValidation):
		status = http.StatusBadRequest
	case errors.Is(err, knowledgeapp.ErrForbidden), errors.Is(err, errForbidden):
		status = http.StatusForbidden
	case errors.Is(err, errEnterpriseServiceUnavailable):
		status = http.StatusForbidden
	case errors.Is(err, knowledgeapp.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, knowledgeapp.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, context.Canceled):
		status = 499
	}
	writeError(w, status, err)
}

func writeKnowledgeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
