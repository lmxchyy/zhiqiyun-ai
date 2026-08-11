package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type inspirationAPI struct {
	repo        inspirationRepository
	store       platformStore
	sessions    authSessionStore
	draftSigner inspirationDraftSigner
}

func newInspirationAPI(repo inspirationRepository, store platformStore, sessions authSessionStore, draftHMACSecret string) inspirationAPI {
	return inspirationAPI{repo: repo, store: store, sessions: sessions, draftSigner: newInspirationDraftSigner([]byte(draftHMACSecret), 30*time.Minute, time.Now)}
}

func (a inspirationAPI) optionalIdentity(r *http.Request) (string, string) {
	userID, err := authenticatedUserID(r, a.sessions)
	if err != nil || userID == "" {
		return "", "default"
	}
	if store, ok := a.store.(activeIdentityStore); ok {
		if user, found, getErr := store.GetActiveUser(userID); getErr == nil && found {
			return user.ID, effectiveTenantID(user)
		}
	}
	data, err := a.store.AdminData()
	if err != nil {
		return userID, "default"
	}
	for _, user := range data.Users {
		if user.ID == userID {
			return userID, effectiveTenantID(user)
		}
	}
	return userID, "default"
}

func (a inspirationAPI) requiredIdentity(r *http.Request) (string, string, error) {
	userID, tenantID := a.optionalIdentity(r)
	if userID == "" {
		return "", "", errUnauthorized
	}
	return userID, tenantID, nil
}

func inspirationPageParams(r *http.Request, fallback int) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize < 1 {
		pageSize = fallback
	}
	if pageSize > 50 {
		pageSize = 50
	}
	return page, pageSize
}

type PublicInspirationSummaryDTO struct {
	ID              string   `json:"id"`
	Slug            string   `json:"slug"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	ContentType     string   `json:"contentType"`
	CategoryID      string   `json:"categoryId"`
	CategoryCode    string   `json:"categoryCode,omitempty"`
	CategoryName    string   `json:"categoryName,omitempty"`
	CoverURL        string   `json:"coverUrl"`
	ThumbnailURL    string   `json:"thumbnailUrl,omitempty"`
	ResultURL       string   `json:"resultUrl,omitempty"`
	Platforms       []string `json:"platforms"`
	Tags            []string `json:"tags"`
	Featured        bool     `json:"featured"`
	Hot             bool     `json:"hot"`
	Pinned          bool     `json:"pinned"`
	SortOrder       int      `json:"sort"`
	TemplateVersion int      `json:"templateVersion"`
	Favorite        bool     `json:"favorite"`
	ViewCount       int64    `json:"viewCount"`
	CopyCount       int64    `json:"copyCount"`
	FavoriteCount   int64    `json:"favoriteCount"`
	UseCount        int64    `json:"useCount"`
	GenerateCount   int64    `json:"generateCount"`
}

type PublicTemplateDetailDTO struct {
	PublicInspirationSummaryDTO
	Schema PublicTemplateDefinition `json:"schema"`
}

func publicInspirationSummary(item inspirationTemplate) PublicInspirationSummaryDTO {
	return PublicInspirationSummaryDTO{
		ID: item.ID, Slug: item.Slug, Title: item.Title, Description: item.Description,
		ContentType: item.ContentType, CategoryID: item.CategoryID, CategoryCode: item.CategoryCode,
		CategoryName: item.CategoryName, CoverURL: item.CoverURL, ThumbnailURL: item.ThumbnailURL,
		ResultURL: item.ResultURL, Platforms: append([]string(nil), item.Platforms...), Tags: append([]string(nil), item.Tags...),
		Featured: item.Featured, Hot: item.Hot, Pinned: item.Pinned, SortOrder: item.SortOrder,
		TemplateVersion: item.Version, Favorite: item.Favorite, ViewCount: item.ViewCount, CopyCount: item.CopyCount,
		FavoriteCount: item.FavoriteCount, UseCount: item.UseCount, GenerateCount: item.GenerateCount,
	}
}

func (a inspirationAPI) categories(w http.ResponseWriter, r *http.Request) {
	_, tenantID := a.optionalIdentity(r)
	items, err := a.repo.ListCategories(r.Context(), tenantID, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a inspirationAPI) featured(w http.ResponseWriter, r *http.Request) {
	userID, tenantID := a.optionalIdentity(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 8
	}
	if limit > 10 {
		limit = 10
	}
	seed, _ := strconv.Atoi(r.URL.Query().Get("seed"))
	items, total, err := a.repo.ListTemplates(r.Context(), inspirationListFilter{TenantID: tenantID, UserID: userID, Category: strings.TrimSpace(r.URL.Query().Get("category")), Platform: firstNonEmptyString(strings.TrimSpace(r.URL.Query().Get("platform")), "miniprogram"), Featured: true, Published: true, Limit: limit, Seed: seed})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	publicItems := make([]PublicInspirationSummaryDTO, len(items))
	for index := range items {
		publicItems[index] = publicInspirationSummary(items[index])
	}
	writeJSON(w, map[string]any{"items": publicItems, "total": total, "seed": seed})
}

func (a inspirationAPI) list(w http.ResponseWriter, r *http.Request) {
	userID, tenantID := a.optionalIdentity(r)
	page, pageSize := inspirationPageParams(r, 12)
	items, total, err := a.repo.ListTemplates(r.Context(), inspirationListFilter{TenantID: tenantID, UserID: userID, Category: strings.TrimSpace(r.URL.Query().Get("category")), ContentType: strings.ToLower(strings.TrimSpace(r.URL.Query().Get("contentType"))), Query: strings.TrimSpace(r.URL.Query().Get("q")), Platform: firstNonEmptyString(strings.TrimSpace(r.URL.Query().Get("platform")), "miniprogram"), Hot: strings.EqualFold(r.URL.Query().Get("hot"), "true"), Published: true, Limit: pageSize, Offset: (page - 1) * pageSize})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	publicItems := make([]PublicInspirationSummaryDTO, len(items))
	for index := range items {
		publicItems[index] = publicInspirationSummary(items[index])
	}
	writeJSON(w, map[string]any{"items": publicItems, "total": total, "page": page, "pageSize": pageSize, "hasMore": page*pageSize < total})
}

func (a inspirationAPI) detail(w http.ResponseWriter, r *http.Request) {
	userID, tenantID := a.optionalIdentity(r)
	item, err := a.repo.GetTemplateBySlug(r.Context(), tenantID, userID, r.PathValue("slug"), false)
	if err != nil {
		writeInspirationError(w, err)
		return
	}
	platform := firstNonEmptyString(strings.TrimSpace(r.URL.Query().Get("platform")), requestTerminal(r), "miniprogram")
	if len(item.Platforms) > 0 && !stringListContains(item.Platforms, platform) {
		writeInspirationError(w, errInspirationNotFound)
		return
	}
	_ = a.repo.RecordEvent(r.Context(), tenantID, userID, item.ID, "view", "", platform, map[string]any{"requestId": strings.TrimSpace(r.Header.Get("X-Request-Id"))})
	writeJSON(w, map[string]any{"item": PublicTemplateDetailDTO{PublicInspirationSummaryDTO: publicInspirationSummary(item), Schema: projectPublicTemplateDefinition(item.Definition)}, "aiGenerated": true})
}

var inspirationEventTypes = map[string]bool{"view": true, "copy_prompt": true, "use_template": true, "generate_success": true}

func (a inspirationAPI) event(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventType        string         `json:"eventType"`
		GenerationTaskID string         `json:"generationTaskId"`
		Platform         string         `json:"platform"`
		Metadata         map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.EventType = strings.ToLower(strings.TrimSpace(req.EventType))
	if !inspirationEventTypes[req.EventType] {
		writeError(w, http.StatusBadRequest, errors.New("unsupported inspiration event type"))
		return
	}
	userID, tenantID := a.optionalIdentity(r)
	if req.EventType != "view" && userID == "" {
		writeError(w, http.StatusUnauthorized, errUnauthorized)
		return
	}
	item, err := a.repo.GetTemplateBySlug(r.Context(), tenantID, userID, r.PathValue("slug"), false)
	if err != nil {
		writeInspirationError(w, err)
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]any{}
	}
	req.Metadata["requestId"] = strings.TrimSpace(r.Header.Get("X-Request-Id"))
	if err := a.repo.RecordEvent(r.Context(), tenantID, userID, item.ID, req.EventType, strings.TrimSpace(req.GenerationTaskID), firstNonEmptyString(strings.TrimSpace(req.Platform), requestTerminal(r)), req.Metadata); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a inspirationAPI) favorite(favorite bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, tenantID, err := a.requiredIdentity(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
		}
		item, getErr := a.repo.GetTemplateBySlug(r.Context(), tenantID, userID, r.PathValue("slug"), false)
		if getErr != nil {
			writeInspirationError(w, getErr)
			return
		}
		if err = a.repo.SetFavorite(r.Context(), tenantID, userID, item.ID, favorite); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]any{"favorite": favorite})
	}
}

func (a inspirationAPI) adminScope(r *http.Request) (string, string) {
	actor, _ := actorFromRequest(r)
	tenantID := strings.TrimSpace(r.URL.Query().Get("tenantId"))
	if tenantID == "" {
		tenantID = "default"
	}
	return actor, tenantID
}

func (a inspirationAPI) adminList(w http.ResponseWriter, r *http.Request) {
	_, tenantID := a.adminScope(r)
	page, pageSize := inspirationPageParams(r, 20)
	items, total, err := a.repo.ListTemplates(r.Context(), inspirationListFilter{TenantID: tenantID, Category: strings.TrimSpace(r.URL.Query().Get("category")), ContentType: strings.TrimSpace(r.URL.Query().Get("contentType")), Query: strings.TrimSpace(r.URL.Query().Get("q")), Status: strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status"))), AuditStatus: strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("auditStatus"))), Limit: pageSize, Offset: (page - 1) * pageSize})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}

func normalizeInspirationTemplate(item *inspirationTemplate, tenantID, actor string, creating bool) error {
	item.Slug = strings.ToLower(strings.TrimSpace(item.Slug))
	item.Title = strings.TrimSpace(item.Title)
	item.Description = strings.TrimSpace(item.Description)
	item.ContentType = strings.ToLower(strings.TrimSpace(item.ContentType))
	item.CategoryID = strings.TrimSpace(item.CategoryID)
	item.CoverURL = strings.TrimSpace(item.CoverURL)
	if item.Slug == "" || item.Title == "" || item.CategoryID == "" || item.CoverURL == "" {
		return errors.New("slug, title, categoryId and coverUrl are required")
	}
	for _, char := range item.Slug {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
			return errors.New("slug must contain only lowercase letters, numbers or hyphen")
		}
	}
	if issues := validateTemplateDefinition(item.ContentType, item.Definition); len(issues) > 0 {
		return fmt.Errorf("invalid template definition at %s: %s", issues[0].Path, issues[0].Message)
	}
	if len(item.Platforms) == 0 {
		item.Platforms = []string{"miniprogram"}
	}
	if item.ApplicableTenantIDs == nil {
		item.ApplicableTenantIDs = []string{}
	}
	if item.Status == "" {
		item.Status = "DRAFT"
	}
	item.Status = strings.ToUpper(item.Status)
	if item.AuditStatus == "" {
		item.AuditStatus = "PENDING"
	}
	item.AuditStatus = strings.ToUpper(item.AuditStatus)
	if creating {
		item.ID = newInspirationID("template")
		item.CreatedBy = actor
	}
	item.TenantID = tenantID
	item.UpdatedBy = actor
	if item.SourceAssetID != "" && !item.SourceAuthorized {
		return errors.New("source asset must have explicit publication authorization")
	}
	return nil
}

func (a inspirationAPI) validateInspirationCapability(item inspirationTemplate, publishing bool) error {
	data, err := a.store.AdminData()
	if err != nil {
		return fmt.Errorf("load platform capabilities: %w", err)
	}
	capabilityKey := canonicalModuleCode(strings.TrimSpace(item.Definition.Capability.CapabilityKey))
	activeCapability := false
	for _, module := range data.AIModules {
		if canonicalModuleCode(firstNonEmptyString(module.ModuleCode, module.ModuleCodeCamel)) == capabilityKey && strings.EqualFold(module.Status, "ACTIVE") {
			activeCapability = true
			break
		}
	}
	modelHint := strings.TrimSpace(item.Definition.Capability.ModelHint)
	if modelHint != "" {
		validModel := false
		for _, model := range data.AIModels {
			name := firstNonEmptyString(model.ModelName, model.ModelNameCamel)
			modelCapability := canonicalModuleCode(firstNonEmptyString(model.ModuleCode, model.ModuleCodeCamel))
			if strings.EqualFold(name, modelHint) && modelCapability == capabilityKey && strings.EqualFold(model.Status, "ACTIVE") {
				validModel = true
				break
			}
		}
		if !validModel {
			return errors.New("capability.modelHint must reference an active compatible model")
		}
	}
	if publishing {
		if strings.EqualFold(item.ContentType, "workflow") {
			return errors.New("workflow template cannot be published without an available executor")
		}
		if !activeCapability {
			return errors.New("template capability is not available for publication")
		}
	}
	return nil
}

func (a inspirationAPI) adminSave(create bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, tenantID := a.adminScope(r)
		var item inspirationTemplate
		var current inspirationTemplate
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&item); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if !create {
			var err error
			current, err = a.repo.GetTemplate(r.Context(), tenantID, "", r.PathValue("id"), true)
			if err != nil {
				writeInspirationError(w, err)
				return
			}
			item.ID = current.ID
			item.CreatedBy = current.CreatedBy
			item.Status = current.Status
			item.AuditStatus = current.AuditStatus
			item.AuditNote = current.AuditNote
		}
		if err := normalizeInspirationTemplate(&item, tenantID, actor, create); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := a.validateInspirationCapability(item, false); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if create {
			item.Status = "DRAFT"
			item.AuditStatus = "PENDING"
			item.AuditNote = ""
		} else if current.Status == "PUBLISHED" {
			item.Status = "DRAFT"
			item.AuditStatus = "PENDING"
			item.AuditNote = ""
		}
		saved, err := a.repo.SaveTemplate(r.Context(), item, "admin save")
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, map[string]any{"item": saved})
	}
}

func (a inspirationAPI) adminGet(w http.ResponseWriter, r *http.Request) {
	_, tenantID := a.adminScope(r)
	item, err := a.repo.GetTemplate(r.Context(), tenantID, "", r.PathValue("id"), true)
	if err != nil {
		writeInspirationError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}
func (a inspirationAPI) adminDelete(w http.ResponseWriter, r *http.Request) {
	actor, tenantID := a.adminScope(r)
	if err := a.repo.DeleteTemplate(r.Context(), tenantID, r.PathValue("id"), actor); err != nil {
		writeInspirationError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a inspirationAPI) adminTransition(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, tenantID := a.adminScope(r)
		item, err := a.repo.GetTemplate(r.Context(), tenantID, "", r.PathValue("id"), true)
		if err != nil {
			writeInspirationError(w, err)
			return
		}
		switch action {
		case "publish":
			if item.AuditStatus != "APPROVED" {
				writeError(w, http.StatusConflict, errors.New("template must pass content audit before publication"))
				return
			}
			if item.SourceAssetID != "" && !item.SourceAuthorized {
				writeError(w, http.StatusConflict, errors.New("source asset publication authorization is required"))
				return
			}
			if err := a.validateInspirationCapability(item, true); err != nil {
				writeError(w, http.StatusConflict, err)
				return
			}
			item.Status = "PUBLISHED"
		case "withdraw":
			item.Status = "WITHDRAWN"
		case "approve":
			item.AuditStatus = "APPROVED"
		case "reject":
			item.AuditStatus = "REJECTED"
		}
		item.UpdatedBy = actor
		saved, err := a.repo.SaveTemplate(r.Context(), item, action)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, map[string]any{"item": saved})
	}
}

func (a inspirationAPI) adminCopy(w http.ResponseWriter, r *http.Request) {
	actor, tenantID := a.adminScope(r)
	item, err := a.repo.GetTemplate(r.Context(), tenantID, "", r.PathValue("id"), true)
	if err != nil {
		writeInspirationError(w, err)
		return
	}
	item.ID = newInspirationID("template")
	item.Slug = copyInspirationSlug(item.Slug)
	item.Title += " - 副本"
	item.Status = "DRAFT"
	item.AuditStatus = "PENDING"
	item.Featured = false
	item.Pinned = false
	item.CreatedBy = actor
	item.UpdatedBy = actor
	saved, err := a.repo.SaveTemplate(r.Context(), item, "copy")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": saved})
}

func copyInspirationSlug(source string) string {
	suffix := fmt.Sprintf("-copy-%d", time.Now().UTC().UnixNano())
	maximumSourceLength := 160 - len(suffix)
	if len(source) > maximumSourceLength {
		source = source[:maximumSourceLength]
	}
	return source + suffix
}

func (a inspirationAPI) adminCategories(w http.ResponseWriter, r *http.Request) {
	_, tenantID := a.adminScope(r)
	items, err := a.repo.ListCategories(r.Context(), tenantID, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a inspirationAPI) adminStatistics(w http.ResponseWriter, r *http.Request) {
	_, tenantID := a.adminScope(r)
	items, total, err := a.repo.ListTemplates(r.Context(), inspirationListFilter{TenantID: tenantID, Limit: 10000})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	stats := map[string]any{"templates": total, "published": 0, "pendingAudit": 0, "views": int64(0), "copies": int64(0), "favorites": int64(0), "uses": int64(0), "generated": int64(0)}
	for _, item := range items {
		if item.Status == "PUBLISHED" {
			stats["published"] = stats["published"].(int) + 1
		}
		if item.AuditStatus == "PENDING" {
			stats["pendingAudit"] = stats["pendingAudit"].(int) + 1
		}
		stats["views"] = stats["views"].(int64) + item.ViewCount
		stats["copies"] = stats["copies"].(int64) + item.CopyCount
		stats["favorites"] = stats["favorites"].(int64) + item.FavoriteCount
		stats["uses"] = stats["uses"].(int64) + item.UseCount
		stats["generated"] = stats["generated"].(int64) + item.GenerateCount
	}
	writeJSON(w, stats)
}

func (a inspirationAPI) adminSort(w http.ResponseWriter, r *http.Request) {
	actor, tenantID := a.adminScope(r)
	var req struct {
		Items []struct {
			ID   string `json:"id"`
			Sort int    `json:"sort"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(req.Items) == 0 || len(req.Items) > 100 {
		writeError(w, http.StatusBadRequest, errors.New("items must contain 1 to 100 records"))
		return
	}
	for _, entry := range req.Items {
		item, err := a.repo.GetTemplate(r.Context(), tenantID, "", entry.ID, true)
		if err != nil {
			writeInspirationError(w, err)
			return
		}
		item.SortOrder = entry.Sort
		item.UpdatedBy = actor
		if _, err = a.repo.SaveTemplate(r.Context(), item, "sort"); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true, "affected": len(req.Items)})
}
func (a inspirationAPI) adminSaveCategory(w http.ResponseWriter, r *http.Request) {
	_, tenantID := a.adminScope(r)
	var item inspirationCategory
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item.Code = strings.ToLower(strings.TrimSpace(item.Code))
	item.Name = strings.TrimSpace(item.Name)
	if item.Code == "" || item.Name == "" {
		writeError(w, http.StatusBadRequest, errors.New("code and name are required"))
		return
	}
	if item.ID == "" {
		item.ID = newInspirationID("category")
	}
	item.TenantID = tenantID
	if item.Status == "" {
		item.Status = "ACTIVE"
	}
	saved, err := a.repo.SaveCategory(r.Context(), item)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": saved})
}

func (a inspirationAPI) adminVersions(w http.ResponseWriter, r *http.Request) {
	_, tenantID := a.adminScope(r)
	items, err := a.repo.ListVersions(r.Context(), tenantID, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}
func (a inspirationAPI) adminRollback(w http.ResponseWriter, r *http.Request) {
	actor, tenantID := a.adminScope(r)
	var req struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Version < 1 {
		writeError(w, http.StatusBadRequest, errors.New("valid version is required"))
		return
	}
	item, err := a.repo.Rollback(r.Context(), tenantID, r.PathValue("id"), req.Version, actor)
	if err != nil {
		writeInspirationError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a inspirationAPI) adminBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(req.IDs) == 0 || len(req.IDs) > 100 {
		writeError(w, http.StatusBadRequest, errors.New("ids must contain 1 to 100 items"))
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action != "publish" && action != "withdraw" && action != "approve" && action != "reject" {
		writeError(w, http.StatusBadRequest, errors.New("unsupported batch action"))
		return
	}
	handler := a.adminTransition(action)
	for _, id := range req.IDs {
		clone := r.Clone(r.Context())
		clone.SetPathValue("id", id)
		rec := newCaptureResponseWriter()
		handler(rec, clone)
		if rec.status >= 400 {
			writeError(w, rec.status, errors.New(string(rec.body)))
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true, "affected": len(req.IDs)})
}

type captureResponseWriter struct {
	header http.Header
	body   []byte
	status int
}

func newCaptureResponseWriter() *captureResponseWriter {
	return &captureResponseWriter{header: http.Header{}, status: http.StatusOK}
}
func (c *captureResponseWriter) Header() http.Header    { return c.header }
func (c *captureResponseWriter) WriteHeader(status int) { c.status = status }
func (c *captureResponseWriter) Write(value []byte) (int, error) {
	c.body = append(c.body, value...)
	return len(value), nil
}

func writeInspirationError(w http.ResponseWriter, err error) {
	if errors.Is(err, errInspirationNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}

func activeInspirationNow(item inspirationTemplate) bool {
	now := time.Now().UTC()
	if item.StartTime != "" {
		if value, err := time.Parse(time.RFC3339, item.StartTime); err == nil && value.After(now) {
			return false
		}
	}
	if item.EndTime != "" {
		if value, err := time.Parse(time.RFC3339, item.EndTime); err == nil && !value.After(now) {
			return false
		}
	}
	return true
}
