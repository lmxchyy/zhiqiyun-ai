Exit code: 0
Wall time: 0.4 seconds
Output:
package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type inspirationAPI struct {
	repo     inspirationRepository
	store    platformStore
	sessions authSessionStore
}

func newInspirationAPI(repo inspirationRepository, store platformStore, sessions authSessionStore) inspirationAPI {
	return inspirationAPI{repo: repo, store: store, sessions: sessions}
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

func publicInspirationSummary(item inspirationTemplate) inspirationTemplate {
	item.Prompt = ""
	item.NegativePrompt = ""
	item.Parameters = nil
	item.ReferenceAssets = nil
	item.ApplicableTenantIDs = nil
	item.AuditNote = ""
	item.CreatedBy = ""
	item.UpdatedBy = ""
	return item
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
	for index := range items {
		items[index] = publicInspirationSummary(items[index])
	}
	writeJSON(w, map[string]any{"items": items, "total": total, "seed": seed})
}

func (a inspirationAPI) list(w http.ResponseWriter, r *http.Request) {
	userID, tenantID := a.optionalIdentity(r)
	page, pageSize := inspirationPageParams(r, 12)
	items, total, err := a.repo.ListTemplates(r.Context(), inspirationListFilter{TenantID: tenantID, UserID: userID, Category: strings.TrimSpace(r.URL.Query().Get("category")), ContentType: strings.ToLower(strings.TrimSpace(r.URL.Query().Get("contentType"))), Query: strings.TrimSpace(r.URL.Query().Get("q")), Platform: firstNonEmptyString(strings.TrimSpace(r.URL.Query().Get("platform")), "miniprogram"), Hot: strings.EqualFold(r.URL.Query().Get("hot"), "true"), Published: true, Limit: pageSize, Offset: (page - 1) * pageSize})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for index := range items {
		items[index] = publicInspirationSummary(items[index])
	}
	writeJSON(w, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize, "hasMore": page*pageSize < total})
}

func modelModuleForContentType(contentType string) string {
	switch contentType {
	case "video":
		return moduleVideoGeneration
	case "ppt":
		return modulePPTGeneration
	default:
		return moduleImageGeneration
	}
}

func (a inspirationAPI) modelResolution(item inspirationTemplate) (bool, string) {
	data, err := a.store.AdminData()
	if err != nil {
		return true, item.ModelID
	}
	moduleCode := modelModuleForContentType(item.ContentType)
	fallback := ""
	for _, model := range data.AIModels {
		name := firstNonEmptyString(model.ModelName, model.ModelNameCamel)
		modelModule := canonicalModuleCode(firstNonEmptyString(model.ModuleCode, model.ModuleCodeCamel))
		if modelModule != canonicalModuleCode(moduleCode) || !strings.EqualFold(model.Status, "ACTIVE") {
			continue
		}
		if fallback == "" {
			fallback = name
		}
		if strings.EqualFold(name, item.ModelID) {
			return true, name
		}
	}
	if fallback == "" {
		for _, model := range data.APIModels {
			if strings.EqualFold(model.Status, "ACTIVE") && fallback == "" {
				fallback = model.Model
			}
			if strings.EqualFold(model.Model, item.ModelID) && strings.EqualFold(model.Status, "ACTIVE") {
				return true, model.Model
			}
		}
	}
	return false, fallback
}

func (a inspirationAPI) detail(w http.ResponseWriter, r *http.Request) {
	userID, tenantID := a.optionalIdentity(r)
	item, err := a.repo.GetTemplate(r.Context(), tenantID, userID, r.PathValue("id"), false)
	if err != nil {
		writeInspirationError(w, err)
		return
	}
	platform := firstNonEmptyString(strings.TrimSpace(r.URL.Query().Get("platform")), requestTerminal(r), "miniprogram")
	if len(item.Platforms) > 0 && !stringListContains(item.Platforms, platform) {
		writeInspirationError(w, errInspirationNotFound)
		return
	}
	available, compatible := a.modelResolution(item)
	_ = a.repo.RecordEvent(r.Context(), tenantID, userID, item.ID, "view", "", platform, map[string]any{"requestId": strings.TrimSpace(r.Header.Get("X-Request-Id"))})
	writeJSON(w, map[string]any{"item": item, "modelAvailable": available, "compatibleModelId": compatible, "aiGenerated": true})
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
	if _, err := a.repo.GetTemplate(r.Context(), tenantID, userID, r.PathValue("id"), false); err != nil {
		writeInspirationError(w, err)
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]any{}
	}
	req.Metadata["requestId"] = strings.TrimSpace(r.Header.Get("X-Request-Id"))
	if err := a.repo.RecordEvent(r.Context(), tenantID, userID, r.PathValue("id"), req.EventType, strings.TrimSpace(req.GenerationTaskID), firstNonEmptyString(strings.TrimSpace(req.Platform), requestTerminal(r)), req.Metadata); err != nil {
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
		if _, err = a.repo.GetTemplate(r.Context(), tenantID, userID, r.PathValue("id"), false); err != nil {
			writeInspirationError(w, err)
			return
		}
		if err = a.repo.SetFavorite(r.Context(), tenantID, userID, r.PathValue("id"), favorite); err != nil {
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
	item.Title = strings.TrimSpace(item.Title)
	item.Description = strings.TrimSpace(item.Description)
	item.ContentType = strings.ToLower(strings.TrimSpace(item.ContentType))
	item.CategoryID = strings.TrimSpace(item.CategoryID)
	item.CoverURL = strings.TrimSpace(item.CoverURL)
	item.Prompt = strings.TrimSpace(item.Prompt)
	item.ModelID = strings.TrimSpace(item.ModelID)
	item.ScenarioCode = strings.ToLower(strings.TrimSpace(item.ScenarioCode))
	if item.Title == "" || item.CategoryID == "" || item.CoverURL == "" || item.Prompt == "" {
		return errors.New("title, categoryId, coverUrl and prompt are required")
	}
	if item.ContentType != "image" && item.ContentType != "video" && item.ContentType != "ppt" {
		return errors.New("contentType must be image, video or ppt")
	}
	if item.Parameters == nil {
		item.Parameters = map[string]any{}
	}
	if item.DisplayConfig == nil {
		item.DisplayConfig = map[string]any{}
	}
	if item.InputRequirements == nil {
		item.InputRequirements = map[string]any{}
	}
	if item.PresetConfig == nil {
		item.PresetConfig = map[string]any{}
	}
	if item.ScenarioCode != "" {
		for _, char := range item.ScenarioCode {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' {
				return errors.New("scenarioCode must contain only lowercase letters, numbers, underscore or hyphen")
			}
		}
	}
	if err := normalizeInspirationInputRequirements(item); err != nil {
		return err
	}
	if item.ReferenceAssets == nil {
		item.ReferenceAssets = []any{}
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

func inspirationConfigInt(config map[string]any, key string) (int, bool) {
	value, found := config[key]
	if !found || value == nil {
		return 0, false
	}
	switch number := value.(type) {
	case float64:
		return int(number), number == float64(int(number))
	case int:
		return number, true
	case int64:
		return int(number), true
	default:
		return 0, false
	}
}

func normalizeInspirationInputRequirements(item *inspirationTemplate) error {
	required, _ := item.InputRequirements["referenceImageRequired"].(bool)
	minimum, hasMinimum := inspirationConfigInt(item.InputRequirements, "referenceImageMin")
	maximum, hasMaximum := inspirationConfigInt(item.InputRequirements, "referenceImageMax")
	if required {
		if !hasMinimum {
			minimum = 1
		}
		if !hasMaximum {
			maximum = minimum
		}
	}
	if minimum < 0 || maximum < 0 || minimum > 3 || maximum > 3 || maximum < minimum {
		return errors.New("reference image range must be between 0 and 3 and max must be greater than or equal to min")
	}
	if required && minimum < 1 {
		return errors.New("referenceImageMin must be at least 1 when a reference image is required")
	}
	if hasMinimum || required {
		item.InputRequirements["referenceImageMin"] = minimum
	}
	if hasMaximum || required {
		item.InputRequirements["referenceImageMax"] = maximum
	}
	if required && item.ContentType != "image" {
		return errors.New("required reference images are currently supported only for image templates")
	}
	if required && item.ScenarioCode == "photo_restoration" {
		item.ReferenceAssets = []any{}
	}
	return nil
}

func (a inspirationAPI) adminSave(create bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, tenantID := a.adminScope(r)
		var item inspirationTemplate
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if !create {
			current, err := a.repo.GetTemplate(r.Context(), tenantID, "", r.PathValue("id"), true)
			if err != nil {
				writeInspirationError(w, err)
				return
			}
			item.ID = current.ID
			item.CreatedBy = current.CreatedBy
		}
		if err := normalizeInspirationTemplate(&item, tenantID, actor, create); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
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

