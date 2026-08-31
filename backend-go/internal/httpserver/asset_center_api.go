package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	providerexecution "xianzhi-ai/backend-go/internal/providerexecution"
)

type assetCenterListQuery struct {
	Limit       int
	Offset      int
	AssetType   string
	Status      string
	Keyword     string
	ProjectID   string
	TagIDs      []string
	Model       string
	CreatedFrom string
	CreatedTo   string
	Sort        string
}

type assetCenterMutation struct {
	Name        *string
	Favorite    *bool
	Archived    *bool
	ProjectID   *string
	ProjectName *string
	Restore     bool
	Permanent   bool
}

type assetCenterDataStore interface {
	ListAssetsForCenter(userID string, query assetCenterListQuery) ([]asset, int, error)
	MutateAssetForUser(userID string, id string, mutation assetCenterMutation) (asset, error)
	ListAssetProjectsForUser(userID string) ([]map[string]any, error)
}

type generationTaskControlStore interface {
	CancelGenerationTaskForUser(userID string, id string) (generationTask, error)
	DeleteGenerationTaskForUser(userID string, id string) error
}

func assetCenterQueryFromRequest(r *http.Request) assetCenterListQuery {
	limit := listLimitFromRequest(r, "limit", defaultUserContentListLimit)
	if pageSize, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("pageSize"))); pageSize > 0 {
		limit = pageSize
	}
	offset := listOffsetFromRequest(r)
	if page, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("page"))); page > 1 && strings.TrimSpace(r.URL.Query().Get("offset")) == "" {
		offset = (page - 1) * limit
	}
	return assetCenterListQuery{
		Limit:       limit,
		Offset:      offset,
		AssetType:   strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type"))),
		Status:      strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status"))),
		Keyword:     strings.TrimSpace(r.URL.Query().Get("keyword")),
		ProjectID:   strings.TrimSpace(r.URL.Query().Get("projectId")),
		TagIDs:      splitAssetCenterValues(r.URL.Query().Get("tagIds")),
		Model:       strings.TrimSpace(r.URL.Query().Get("model")),
		CreatedFrom: strings.TrimSpace(r.URL.Query().Get("createdFrom")),
		CreatedTo:   strings.TrimSpace(r.URL.Query().Get("createdTo")),
		Sort:        strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort"))),
	}
}

func splitAssetCenterValues(value string) []string {
	value = strings.ReplaceAll(value, "，", ",")
	items := []string{}
	seen := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		items = append(items, part)
	}
	return items
}

func (a api) assetsForCenter(userID string, tenantID string, query assetCenterListQuery) ([]asset, int, error) {
	if store, ok := a.store.(assetCenterDataStore); ok {
		return store.ListAssetsForCenter(userID, query)
	}
	tenantID = strings.TrimSpace(tenantID)
	items, err := a.store.ListAssets()
	if err != nil {
		return nil, 0, err
	}
	items = filterAssetCenterItems(items, userID, tenantID, query)
	total := len(items)
	start := query.Offset
	if start > total {
		start = total
	}
	end := start + query.Limit
	if end > total {
		end = total
	}
	return items[start:end], total, nil
}

func (a api) assetsOverview(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	summary, err := a.assetListSummaryForUser(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{
		"overview": map[string]any{
			"total":             summary.Total,
			"monthTotal":        summary.MonthTotal,
			"favoriteTotal":     summary.FavoriteTotal,
			"storageBytes":      summary.StorageBytes,
			"storageQuotaBytes": assetStorageQuotaBytes(),
		},
	})
}

func assetStorageQuotaBytes() int64 {
	value, _ := strconv.ParseInt(strings.TrimSpace(os.Getenv("ASSET_STORAGE_QUOTA_BYTES")), 10, 64)
	if value < 0 {
		return 0
	}
	return value
}

func (a api) assetProjects(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if store, ok := a.store.(assetCenterDataStore); ok {
		items, err := store.ListAssetProjectsForUser(user.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]any{"items": items})
		return
	}
	writeJSON(w, map[string]any{"items": []any{}})
}

func decodeAssetCenterMutation(r *http.Request) (assetCenterMutation, error) {
	var body struct {
		Name        *string `json:"name"`
		ProjectID   *string `json:"projectId"`
		ProjectName *string `json:"projectName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return assetCenterMutation{}, err
	}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		body.Name = &name
	}
	return assetCenterMutation{Name: body.Name, ProjectID: body.ProjectID, ProjectName: body.ProjectName}, nil
}

func (a api) mutateAsset(w http.ResponseWriter, r *http.Request, mutation assetCenterMutation) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	store, ok := a.store.(assetCenterDataStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("asset mutation store is unavailable"))
		return
	}
	item, err := store.MutateAssetForUser(user.ID, strings.TrimSpace(r.PathValue("id")), mutation)
	if err != nil {
		if errors.Is(err, errAssetNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a api) updateAsset(w http.ResponseWriter, r *http.Request) {
	mutation, err := decodeAssetCenterMutation(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if mutation.Name == nil || strings.TrimSpace(*mutation.Name) == "" {
		writeError(w, http.StatusBadRequest, errors.New("asset name is required"))
		return
	}
	a.mutateAsset(w, r, mutation)
}

func (a api) favoriteAsset(w http.ResponseWriter, r *http.Request) {
	value := r.Method != http.MethodDelete
	a.mutateAsset(w, r, assetCenterMutation{Favorite: &value})
}

func (a api) archiveAsset(w http.ResponseWriter, r *http.Request) {
	value := true
	a.mutateAsset(w, r, assetCenterMutation{Archived: &value})
}

func (a api) restoreAsset(w http.ResponseWriter, r *http.Request) {
	a.mutateAsset(w, r, assetCenterMutation{Restore: true})
}

func (a api) permanentlyDeleteAsset(w http.ResponseWriter, r *http.Request) {
	a.mutateAsset(w, r, assetCenterMutation{Permanent: true})
}

func (a api) moveAssetProject(w http.ResponseWriter, r *http.Request) {
	mutation, err := decodeAssetCenterMutation(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	a.mutateAsset(w, r, mutation)
}

func (a api) batchAssets(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	store, ok := a.store.(assetCenterDataStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("asset mutation store is unavailable"))
		return
	}
	var req struct {
		Action      string   `json:"action"`
		IDs         []string `json:"ids"`
		ProjectID   string   `json:"projectId"`
		ProjectName string   `json:"projectName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(req.IDs) == 0 || len(req.IDs) > 100 {
		writeError(w, http.StatusBadRequest, errors.New("asset batch must contain 1 to 100 ids"))
		return
	}
	items := []asset{}
	for _, id := range req.IDs {
		mutation := assetCenterMutation{}
		switch strings.ToLower(strings.TrimSpace(req.Action)) {
		case "favorite":
			value := true
			mutation.Favorite = &value
		case "unfavorite":
			value := false
			mutation.Favorite = &value
		case "archive":
			value := true
			mutation.Archived = &value
		case "move":
			mutation.ProjectID, mutation.ProjectName = &req.ProjectID, &req.ProjectName
		case "restore":
			mutation.Restore = true
		case "permanent":
			mutation.Permanent = true
		case "delete":
			if err := a.store.DeleteAssetForUser(user.ID, id); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			continue
		default:
			writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported asset batch action: %s", req.Action))
			return
		}
		item, err := store.MutateAssetForUser(user.ID, id, mutation)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if !mutation.Permanent {
			items = append(items, item)
		}
	}
	writeJSON(w, map[string]any{"ok": true, "items": items})
}

func (a api) cancelGenerationTask(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	store, ok := a.store.(generationTaskControlStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("generation task cancellation is unavailable"))
		return
	}
	task, err := store.CancelGenerationTaskForUser(user.ID, id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if cancel, ok := a.generationTaskCancel(id); ok {
		cancel()
	}
	writeJSON(w, task)
}

func (a api) deleteGenerationTask(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	store, ok := a.store.(generationTaskControlStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("generation task deletion is unavailable"))
		return
	}
	if err := store.DeleteGenerationTaskForUser(user.ID, id); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func (a api) registerGenerationTaskCancel(id string, cancel context.CancelFunc) {
	if a.taskCancels != nil && strings.TrimSpace(id) != "" && cancel != nil {
		a.taskCancels.Store(id, cancel)
	}
}

func (a api) unregisterGenerationTaskCancel(id string) {
	if a.taskCancels != nil {
		a.taskCancels.Delete(id)
	}
}

func (a api) generationTaskCancel(id string) (context.CancelFunc, bool) {
	if a.taskCancels == nil {
		return nil, false
	}
	value, ok := a.taskCancels.Load(id)
	if !ok {
		return nil, false
	}
	cancel, ok := value.(context.CancelFunc)
	return cancel, ok
}

func (a api) retryGenerationTask(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	var original generationTask
	var found bool
	if optimized, ok := a.store.(optimizedUserContentStore); ok {
		original, found, err = optimized.GetGenerationTaskForUser(user.ID, id)
	} else {
		var tasks []generationTask
		tasks, err = a.store.ListGenerationTasks()
		if err == nil {
			for _, item := range tasks {
				if item.ID == id && item.UserID == user.ID {
					original, found = item, true
					break
				}
			}
		}
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, errors.New("generation task not found"))
		return
	}
	execution, hasExecution, err := providerExecutionForRetry(a.store, a.cfg, original.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if hasExecution && isVideoGenerationRequest(original.Type) && (execution.Status == providerexecution.Prepared || execution.ProviderRequestID != nil && (execution.Status == providerexecution.Unknown || execution.Status == providerexecution.Submitted || execution.Status == providerexecution.Processing || execution.Status == providerexecution.Succeeded)) {
		req := generation.CreateRequest{UserID: user.ID, Type: original.Type, Prompt: original.Prompt, Model: original.Model, Params: cloneAnyMap(original.Params), ModuleCode: original.ModuleCode}
		if req.Params == nil {
			req.Params = map[string]any{}
		}
		req.Params["retryOf"] = original.ID
		service, serviceErr := a.retryGenerationService(user, req)
		if serviceErr != nil {
			writeError(w, http.StatusBadGateway, serviceErr)
			return
		}
		go a.runVideoGenerationTask(original.ID, service, req)
		writeJSON(w, original)
		return
	}
	if hasExecution {
		switch execution.Status {
		case providerexecution.Unknown, providerexecution.Submitting:
			writeError(w, http.StatusConflict, providerexecution.ErrUnknownResubmitBlocked)
			return
		case providerexecution.Prepared:
			writeError(w, http.StatusConflict, errors.New("provider execution is prepared; retry must resume existing execution"))
			return
		}
	}
	if original.Status == "PENDING" || original.Status == "QUEUED" || original.Status == "RUNNING" || original.Status == "PROCESSING" || original.Status == "RETRYING" {
		writeError(w, http.StatusConflict, errors.New("active generation tasks cannot be retried"))
		return
	}
	params := cloneAnyMap(original.Params)
	deleteGenerationBillingParams(params)
	req := generation.CreateRequest{UserID: user.ID, Type: original.Type, Prompt: original.Prompt, Model: original.Model, Params: params, ModuleCode: original.ModuleCode}
	req.Params["terminal"] = requestTerminal(r)
	req, err = a.prepareGenerationRequest(data, user, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := enforceMiniProgramModelCompliance(data, &req); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	req.Params["retryOf"] = original.ID
	task, err := a.startRetriedGenerationTask(r.Context(), user, req)
	if err != nil {
		if errors.Is(err, errGenerationConcurrencyLimit) {
			writeError(w, http.StatusTooManyRequests, err)
			return
		}
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, task)
}

func deleteGenerationBillingParams(params map[string]any) {
	for _, key := range []string{generationBillingReservedKey, generationBillingReservedAtKey, generationBillingReservationPointCostKey, generationBillingReservationBalanceBeforeKey, generationBillingReservationBalanceAfterKey, generationBillingRefundedKey, generationBillingRefundedAtKey, generationBillingRefundBalanceBeforeKey, generationBillingRefundBalanceAfterKey} {
		delete(params, key)
	}
}

func (a api) retryGenerationService(user adminUser, req generation.CreateRequest) (generation.Service, error) {
	service := a.generationService
	if routeService, ok, err := a.generationServiceForUserRoute(user, req.Model); err != nil {
		return generation.Service{}, err
	} else if ok {
		service = routeService
	} else if providerID := selectedGenerationProvider(req.Params); providerID != "" {
		dynamicService, err := a.generationServiceForProvider(providerID, req)
		if err != nil {
			return generation.Service{}, err
		}
		service = dynamicService
	} else if configuredService, ok, err := a.generationServiceForConfiguredModel(req.Model); err != nil {
		return generation.Service{}, err
	} else if ok {
		service = configuredService
	}
	return service, nil
}

func (a api) startRetriedGenerationTask(ctx context.Context, user adminUser, req generation.CreateRequest) (generationTask, error) {
	service, err := a.retryGenerationService(user, req)
	if err != nil {
		return generationTask{}, err
	}
	if isVideoGenerationRequest(req.Type) {
		task, err := a.store.CreatePendingGenerationTask(req)
		if err == nil {
			if task.IdempotentReplay {
				return task, nil
			}
			go a.runVideoGenerationTask(task.ID, service, cloneGenerationCreateRequest(req))
		}
		return task, err
	}
	if isImageGenerationRequest(req.Type) && !strings.EqualFold(strings.TrimSpace(req.Model), "mock-standard") {
		task, err := a.store.CreatePendingGenerationTask(req)
		if err == nil {
			if task.IdempotentReplay {
				return task, nil
			}
			go a.runGenerationTask(task.ID, service, cloneGenerationCreateRequest(req))
		}
		return task, err
	}
	created, err := service.Create(ctx, req)
	if err != nil {
		return generationTask{}, err
	}
	if task, ok := created.(generationTask); ok {
		return task, nil
	}
	raw, err := json.Marshal(created)
	if err != nil {
		return generationTask{}, err
	}
	var task generationTask
	if err := json.Unmarshal(raw, &task); err != nil {
		return generationTask{}, err
	}
	return task, nil
}

func filterAssetCenterItems(items []asset, userID string, tenantID string, query assetCenterListQuery) []asset {
	filtered := make([]asset, 0, len(items))
	for _, item := range items {
		if item.UserID != userID || !assetMatchesCenterQuery(item, query) {
			continue
		}
		if tenantID != "" {
			itemTenantID := strings.TrimSpace(item.TenantID)
			if itemTenantID != "" && itemTenantID != tenantID {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(i, j int) bool { return assetCenterLess(filtered[i], filtered[j], query.Sort) })
	return filtered
}

func assetMatchesCenterQuery(item asset, query assetCenterListQuery) bool {
	deleted := assetDeleted(item)
	if query.Status == "recycled" {
		if !deleted {
			return false
		}
	} else if deleted {
		return false
	}
	metadata := item.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	assetType := normalizedAssetCenterType(firstNonEmptyString(item.MediaType, stringValue(metadata["type"]), stringValue(metadata["mediaType"])))
	if query.AssetType != "" && query.AssetType != "all" && assetType != query.AssetType {
		return false
	}
	status := normalizedAssetCenterStatus(item)
	switch query.Status {
	case "queued", "generating", "completed", "failed", "archived":
		if status != query.Status {
			return false
		}
	case "favorite":
		if !item.Favorite {
			return false
		}
	}
	searchable := strings.ToLower(item.Name + " " + item.MediaType + " " + normalizedAssetCenterType(item.MediaType) + " " + stringValue(metadata["prompt"]) + " " + stringValue(metadata["projectName"]) + " " + stringValue(metadata["model"]) + " " + fmt.Sprint(metadata["tags"]))
	if query.Keyword != "" && !strings.Contains(searchable, strings.ToLower(query.Keyword)) {
		return false
	}
	if query.ProjectID != "" && query.ProjectID != stringValue(metadata["projectId"]) && !strings.EqualFold(query.ProjectID, stringValue(metadata["projectName"])) {
		return false
	}
	if query.Model != "" && !strings.Contains(strings.ToLower(stringValue(metadata["model"])), strings.ToLower(query.Model)) {
		return false
	}
	for _, tag := range query.TagIDs {
		if !strings.Contains(strings.ToLower(fmt.Sprint(metadata["tags"])), strings.ToLower(tag)) {
			return false
		}
	}
	return (query.CreatedFrom == "" || item.CreatedAt >= query.CreatedFrom) && (query.CreatedTo == "" || item.CreatedAt <= query.CreatedTo+"T23:59:59")
}

func normalizedAssetCenterType(value string) string {
	value = strings.ToLower(value)
	for _, item := range []string{"video", "ppt", "agent", "infographic", "knowledge", "prompt", "template", "document", "image"} {
		if strings.Contains(value, item) {
			return item
		}
	}
	if strings.Contains(value, "presentation") {
		return "ppt"
	}
	if strings.Contains(value, "pdf") || strings.Contains(value, "doc") || strings.Contains(value, "text") {
		return "document"
	}
	return "image"
}

func normalizedAssetCenterStatus(item asset) string {
	if assetDeleted(item) {
		return "recycled"
	}
	if boolValue(item.Metadata["archived"]) {
		return "archived"
	}
	value := strings.ToUpper(firstNonEmptyString(stringValue(item.Metadata["status"]), stringValue(item.Metadata["taskStatus"]), "COMPLETED"))
	if value == "PENDING" || value == "QUEUED" {
		return "queued"
	}
	if value == "RUNNING" || value == "PROCESSING" || value == "RETRYING" || value == "GENERATING" {
		return "generating"
	}
	if value == "FAILED" || value == "ERROR" {
		return "failed"
	}
	return "completed"
}

func assetCenterLess(left asset, right asset, order string) bool {
	switch order {
	case "created_asc":
		return left.CreatedAt < right.CreatedAt
	case "updated_desc":
		return left.UpdatedAt > right.UpdatedAt
	case "name_asc":
		return strings.ToLower(left.Name) < strings.ToLower(right.Name)
	case "size_desc":
		return assetCenterNumber(left.Metadata, "fileSize", "fileSizeBytes", "sizeBytes") > assetCenterNumber(right.Metadata, "fileSize", "fileSizeBytes", "sizeBytes")
	case "usage_desc":
		return assetCenterNumber(left.Metadata, "usageCount") > assetCenterNumber(right.Metadata, "usageCount")
	default:
		return left.CreatedAt > right.CreatedAt
	}
}

func assetCenterNumber(metadata map[string]any, keys ...string) int64 {
	for _, key := range keys {
		value, ok := metadata[key]
		if !ok {
			continue
		}
		parsed, _ := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
		return parsed
	}
	return 0
}

func mutateAssetItem(item asset, mutation assetCenterMutation) asset {
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	if mutation.Name != nil {
		item.Name = strings.TrimSpace(*mutation.Name)
	}
	if mutation.Favorite != nil {
		item.Favorite = *mutation.Favorite
		item.Metadata["favorite"] = *mutation.Favorite
	}
	if mutation.Archived != nil {
		item.Metadata["archived"] = *mutation.Archived
	}
	if mutation.ProjectID != nil {
		item.Metadata["projectId"] = strings.TrimSpace(*mutation.ProjectID)
	}
	if mutation.ProjectName != nil {
		item.Metadata["projectName"] = strings.TrimSpace(*mutation.ProjectName)
	}
	if mutation.Restore {
		item.DeletedAt = ""
		delete(item.Metadata, "deletedAt")
	}
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return item
}

func (s *jsonStore) ListAssetsForCenter(userID string, query assetCenterListQuery) ([]asset, int, error) {
	data, err := s.load()
	if err != nil {
		return nil, 0, err
	}
	items := filterAssetCenterItems(data.Assets, userID, "", query)
	total := len(items)
	start := query.Offset
	if start > total {
		start = total
	}
	end := start + query.Limit
	if end > total {
		end = total
	}
	return items[start:end], total, nil
}

func (s *jsonStore) MutateAssetForUser(userID string, id string, mutation assetCenterMutation) (asset, error) {
	var result asset
	err := s.update(func(data *platformData) error {
		for index := range data.Assets {
			item := data.Assets[index]
			if item.ID != id || item.UserID != userID {
				continue
			}
			if mutation.Permanent {
				if !assetDeleted(item) {
					return errors.New("asset must be in recycle bin before permanent deletion")
				}
				data.Assets = append(data.Assets[:index], data.Assets[index+1:]...)
				return nil
			}
			if assetDeleted(item) && !mutation.Restore {
				return fmt.Errorf("%w: %s", errAssetNotFound, id)
			}
			result = mutateAssetItem(item, mutation)
			data.Assets[index] = result
			return nil
		}
		return fmt.Errorf("%w: %s", errAssetNotFound, id)
	})
	return result, err
}

func (s *jsonStore) ListAssetProjectsForUser(userID string) ([]map[string]any, error) {
	data, err := s.load()
	if err != nil {
		return nil, err
	}
	return assetCenterProjectsFromItems(data.Assets, userID), nil
}

func assetCenterProjectsFromItems(items []asset, userID string) []map[string]any {
	counts := map[string]int{}
	names := map[string]string{}
	for _, item := range items {
		if item.UserID != userID || assetDeleted(item) {
			continue
		}
		id := strings.TrimSpace(stringValue(item.Metadata["projectId"]))
		name := strings.TrimSpace(stringValue(item.Metadata["projectName"]))
		if id == "" || name == "" {
			continue
		}
		counts[id]++
		names[id] = name
	}
	result := make([]map[string]any, 0, len(counts))
	for id, count := range counts {
		result = append(result, map[string]any{"id": id, "name": names[id], "assetCount": count})
	}
	sort.Slice(result, func(i, j int) bool { return stringValue(result[i]["name"]) < stringValue(result[j]["name"]) })
	return result
}

func (s *jsonStore) CancelGenerationTaskForUser(userID string, id string) (generationTask, error) {
	var task generationTask
	err := s.updateWithPersonalPoints(context.Background(), func(data *platformData, points *JSONPersonalPointStore) error {
		found := false
		for _, item := range data.GenerationTasks {
			if item.ID != id || item.UserID != userID {
				continue
			}
			found = true
			if upperTrim(item.Status) == "CANCELLED" {
				task = item
				return nil
			}
			if !activeGenerationTaskStatus(item.Status) {
				return errors.New("only active tasks can be cancelled")
			}
			break
		}
		if !found {
			return errors.New("generation task not found")
		}
		var err error
		task, err = mutateJSONGenerationFailure(data, points, userID, id, "用户取消生成", "CANCELLED", taskStatusCancelled)
		return err
	})
	return task, err
}

func generationTaskDeletable(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "FAILED", "ERROR", "CANCELLED", "CANCELED", "REJECTED":
		return true
	default:
		return false
	}
}

func (s *jsonStore) DeleteGenerationTaskForUser(userID string, id string) error {
	return s.update(func(data *platformData) error {
		for index := range data.GenerationTasks {
			item := data.GenerationTasks[index]
			if item.ID != id || item.UserID != userID {
				continue
			}
			if !generationTaskDeletable(item.Status) {
				return errors.New("only failed or cancelled tasks can be deleted")
			}
			data.GenerationTasks = append(data.GenerationTasks[:index], data.GenerationTasks[index+1:]...)
			return nil
		}
		return errors.New("generation task not found")
	})
}

func (s *postgresStore) ListAssetsForCenter(userID string, query assetCenterListQuery) ([]asset, int, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, 0, err
	}
	contextType, tenantID, _, err := s.currentTenantScopeForUser(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	where := []string{
		"user_id = $1",
		"(($2='ENTERPRISE' and tenant_id=$3) or ($2<>'ENTERPRISE' and (tenant_id is null or tenant_id='tenant_default')))",
	}
	args := []any{userID, contextType, tenantID}
	add := func(condition string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(condition, len(args)))
	}
	if query.Status == "recycled" {
		where = append(where, "deleted_at is not null")
	} else {
		where = append(where, "deleted_at is null")
	}
	if query.AssetType != "" && query.AssetType != "all" {
		add("lower(coalesce(media_type, metadata->>'type', metadata->>'mediaType', '')) like '%%' || $%d || '%%'", query.AssetType)
	}
	switch query.Status {
	case "favorite":
		where = append(where, "coalesce(favorite, false) = true")
	case "archived":
		where = append(where, "lower(coalesce(metadata->>'archived', 'false')) = 'true'")
	case "queued":
		where = append(where, "upper(coalesce(metadata->>'status', metadata->>'taskStatus', 'COMPLETED')) in ('PENDING','QUEUED')")
	case "generating":
		where = append(where, "upper(coalesce(metadata->>'status', metadata->>'taskStatus', 'COMPLETED')) in ('RUNNING','PROCESSING','RETRYING','GENERATING')")
	case "failed":
		where = append(where, "upper(coalesce(metadata->>'status', metadata->>'taskStatus', 'COMPLETED')) in ('FAILED','ERROR')")
	case "completed":
		where = append(where, "upper(coalesce(metadata->>'status', metadata->>'taskStatus', 'COMPLETED')) in ('SUCCEEDED','SUCCESS','COMPLETED')")
	}
	if query.Keyword != "" {
		args = append(args, query.Keyword)
		index := len(args)
		where = append(where, fmt.Sprintf("(lower(coalesce(name, '')) like '%%' || lower($%d) || '%%' or lower(coalesce(media_type, '')) like '%%' || lower($%d) || '%%' or lower(coalesce(metadata::text, '')) like '%%' || lower($%d) || '%%')", index, index, index))
	}
	if query.ProjectID != "" {
		args = append(args, query.ProjectID)
		index := len(args)
		where = append(where, fmt.Sprintf("(coalesce(metadata->>'projectId', '') = $%d or lower(coalesce(metadata->>'projectName', '')) like '%%' || lower($%d) || '%%')", index, index))
	}
	if query.Model != "" {
		add("lower(coalesce(metadata->>'model', metadata->>'modelName', '')) like '%%' || lower($%d) || '%%'", query.Model)
	}
	for _, tag := range query.TagIDs {
		add("lower(coalesce((metadata->'tags')::text, (metadata->'tagIds')::text, '')) like '%%' || lower($%d) || '%%'", tag)
	}
	if query.CreatedFrom != "" {
		add("coalesce(created_at, '') >= $%d", query.CreatedFrom)
	}
	if query.CreatedTo != "" {
		add("coalesce(created_at, '') <= $%d", query.CreatedTo+"T23:59:59.999999999Z")
	}
	whereSQL := strings.Join(where, " and ")
	var total int
	if err := s.db.QueryRowContext(ctx, "select count(*) from xz_assets where "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	orderBy := "created_at desc nulls last, id desc"
	switch query.Sort {
	case "created_asc":
		orderBy = "created_at asc nulls last, id asc"
	case "updated_desc":
		orderBy = "updated_at desc nulls last, id desc"
	case "name_asc":
		orderBy = "lower(name) asc, id asc"
	case "size_desc":
		orderBy = "case when jsonb_typeof(metadata->'fileSize')='number' then (metadata->>'fileSize')::bigint else 0 end desc, created_at desc"
	case "usage_desc":
		orderBy = "case when jsonb_typeof(metadata->'usageCount')='number' then (metadata->>'usageCount')::bigint else 0 end desc, created_at desc"
	}
	args = append(args, query.Limit, query.Offset)
	rows, err := s.db.QueryContext(ctx, assetSummarySelect+" where "+whereSQL+" order by "+orderBy+fmt.Sprintf(" limit $%d offset $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanAssetSummaryRows(rows)
	return items, total, err
}

func (s *postgresStore) MutateAssetForUser(userID string, id string, mutation assetCenterMutation) (asset, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return asset{}, err
	}
	contextType, tenantID, _, err := s.currentTenantScopeForUser(ctx, userID)
	if err != nil {
		return asset{}, err
	}
	rows, err := s.db.QueryContext(ctx, assetSummarySelect+" where id = $1 and user_id = $2 and (($3='ENTERPRISE' and tenant_id=$4) or ($3<>'ENTERPRISE' and (tenant_id is null or tenant_id='tenant_default'))) limit 1", id, userID, contextType, tenantID)
	if err != nil {
		return asset{}, err
	}
	items, scanErr := scanAssetSummaryRows(rows)
	_ = rows.Close()
	if scanErr != nil {
		return asset{}, scanErr
	}
	if len(items) == 0 {
		return asset{}, fmt.Errorf("%w: %s", errAssetNotFound, id)
	}
	item := items[0]
	if mutation.Permanent {
		if !assetDeleted(item) {
			return asset{}, errors.New("asset must be in recycle bin before permanent deletion")
		}
		result, err := s.db.ExecContext(ctx, "delete from xz_assets where id = $1 and user_id = $2 and deleted_at is not null and (($3='ENTERPRISE' and tenant_id=$4) or ($3<>'ENTERPRISE' and (tenant_id is null or tenant_id='tenant_default')))", id, userID, contextType, tenantID)
		if err != nil {
			return asset{}, err
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return asset{}, fmt.Errorf("%w: %s", errAssetNotFound, id)
		}
		return item, nil
	}
	if assetDeleted(item) && !mutation.Restore {
		return asset{}, fmt.Errorf("%w: %s", errAssetNotFound, id)
	}
	item = mutateAssetItem(item, mutation)
	metadataJSON, _ := json.Marshal(item.Metadata)
	_, err = s.db.ExecContext(ctx, `
		update xz_assets set
			name = $3::text,
			favorite = $4::boolean,
			metadata = $5::jsonb,
			deleted_at = nullif($6::text, '')::timestamptz,
			updated_at = $7::text,
			raw = coalesce(raw, '{}'::jsonb) || jsonb_build_object(
				'name', $3::text,
				'favorite', $4::boolean,
				'metadata', $5::jsonb,
				'deletedAt', nullif($6::text, ''),
				'updatedAt', $7::text
			)
		where id = $1 and user_id = $2
		  and (($8='ENTERPRISE' and tenant_id=$9) or ($8<>'ENTERPRISE' and (tenant_id is null or tenant_id='tenant_default')))
	`, id, userID, item.Name, item.Favorite, string(metadataJSON), item.DeletedAt, item.UpdatedAt, contextType, tenantID)
	return item, err
}

func (s *postgresStore) ListAssetProjectsForUser(userID string) ([]map[string]any, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	contextType, tenantID, _, err := s.currentTenantScopeForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		select metadata->>'projectId', max(metadata->>'projectName'), count(*)
		from xz_assets
		where user_id=$1 and deleted_at is null
		  and (($2='ENTERPRISE' and tenant_id=$3) or ($2<>'ENTERPRISE' and (tenant_id is null or tenant_id='tenant_default')))
		  and coalesce(metadata->>'projectId','') <> '' and coalesce(metadata->>'projectName','') <> ''
		group by metadata->>'projectId'
		order by max(metadata->>'projectName')
	`, userID, contextType, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name string
		var count int
		if err := rows.Scan(&id, &name, &count); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"id": id, "name": name, "assetCount": count})
	}
	return items, rows.Err()
}

func (s *postgresStore) CancelGenerationTaskForUser(userID string, id string) (generationTask, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return generationTask{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return generationTask{}, err
	}
	defer func() { _ = tx.Rollback() }()
	task, err := generationTaskForUpdate(ctx, tx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && task.UserID != userID) {
		return generationTask{}, errors.New("generation task not found")
	}
	if err != nil {
		return generationTask{}, err
	}
	if upperTrim(task.Status) == "CANCELLED" {
		return task, tx.Commit()
	}
	if !activeGenerationTaskStatus(task.Status) {
		return generationTask{}, errors.New("only active tasks can be cancelled")
	}
	task, refunded, _, err := s.mutatePostgresGenerationFailureTx(ctx, tx, task, "用户取消生成", "CANCELLED", taskStatusCancelled)
	if err != nil {
		return generationTask{}, err
	}
	if err := insertAuditLog(ctx, tx, task.UserID, "MEMBER", "generation.cancel", "generation_task", task.ID, "", "", 200, map[string]any{"pointCost": task.PointCost, "billingRefunded": refunded}); err != nil {
		return generationTask{}, err
	}
	return task, tx.Commit()
}

func (s *postgresStore) DeleteGenerationTaskForUser(userID string, id string) error {
	task, found, err := s.GetGenerationTaskForUser(userID, id)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("generation task not found")
	}
	if !generationTaskDeletable(task.Status) {
		return errors.New("only failed or cancelled tasks can be deleted")
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	result, err := s.db.ExecContext(ctx, `
		delete from xz_generation_tasks
		where id = $1 and user_id = $2
		  and upper(coalesce(status, '')) in ('FAILED', 'ERROR', 'CANCELLED', 'CANCELED', 'REJECTED')
	`, id, userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("generation task not found")
	}
	return nil
}

var _ assetCenterDataStore = (*jsonStore)(nil)
var _ assetCenterDataStore = (*postgresStore)(nil)
var _ generationTaskControlStore = (*jsonStore)(nil)
var _ generationTaskControlStore = (*postgresStore)(nil)
