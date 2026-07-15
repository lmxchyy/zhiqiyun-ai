package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"xianzhi-ai/backend-go/internal/config"
)

type mediaAPI struct {
	repo      mediaRepository
	storage   mediaStorageProvider
	store     platformStore
	sessions  authSessionStore
	redis     *redis.Client
	maxUpload int64
}

func newMediaAPI(cfg config.Config, repo mediaRepository, storage mediaStorageProvider, store platformStore, sessions authSessionStore, redisClient *redis.Client) mediaAPI {
	return mediaAPI{repo: repo, storage: storage, store: store, sessions: sessions, redis: redisClient, maxUpload: parseMediaMaxBytes(cfg.MediaMaxUploadBytes)}
}

func (a mediaAPI) adminIdentity(r *http.Request) (adminUser, string, error) {
	userID, err := authenticatedUserID(r, a.sessions)
	if err != nil {
		return adminUser{}, "", err
	}
	var user adminUser
	if optimized, ok := a.store.(activeIdentityStore); ok {
		user, _, err = optimized.GetActiveUser(userID)
	} else {
		data, dataErr := a.store.AdminData()
		if dataErr != nil {
			return user, "", dataErr
		}
		for _, item := range data.Users {
			if item.ID == userID {
				user = item
				break
			}
		}
	}
	if err != nil {
		return user, "", err
	}
	if user.ID == "" {
		return user, "", errors.New("authenticated user not found")
	}
	tenant := effectiveTenantID(user)
	role := strings.ToUpper(user.Role)
	if role == "SUPER_ADMIN" || role == "PLATFORM_ADMIN" {
		tenant = "default"
	}
	requested := firstNonEmptyString(r.URL.Query().Get("tenantId"), r.Header.Get("X-Tenant-Id"))
	if requested != "" && (role == "SUPER_ADMIN" || role == "PLATFORM_ADMIN" || role == "ADMIN") {
		tenant = requested
	}
	if tenant == "" {
		tenant = "default"
	}
	return user, tenant, nil
}
func mediaPageCode(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "works" {
		value = "assets"
	}
	if value == "mine" {
		value = "profile"
	}
	switch value {
	case "home", "studio", "assets", "profile":
		return value, nil
	default:
		return "", errors.New("pageCode must be home, studio, assets or profile")
	}
}

func (a mediaAPI) listAssets(w http.ResponseWriter, r *http.Request) {
	_, tenant, err := a.adminIdentity(r)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	limit := mediaIntQuery(r, "pageSize", 24, 1, 100)
	page := mediaIntQuery(r, "page", 1, 1, 100000)
	items, total, err := a.repo.ListAssets(r.Context(), tenant, mediaAssetFilter{Query: strings.TrimSpace(r.URL.Query().Get("q")), CategoryID: strings.TrimSpace(r.URL.Query().Get("categoryId")), Status: strings.TrimSpace(r.URL.Query().Get("status")), Limit: limit, Offset: (page - 1) * limit})
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": total, "page": page, "pageSize": limit})
}
func (a mediaAPI) getAsset(w http.ResponseWriter, r *http.Request) {
	_, tenant, err := a.adminIdentity(r)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	item, err := a.repo.GetAsset(r.Context(), tenant, r.PathValue("id"))
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a mediaAPI) uploadAsset(w http.ResponseWriter, r *http.Request) {
	user, tenant, err := a.adminIdentity(r)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	if err = r.ParseMultipartForm(a.maxUpload * 20); err != nil {
		writeMediaError(w, fmt.Errorf("invalid multipart form: %w", err))
		return
	}
	headers := r.MultipartForm.File["file"]
	if len(headers) == 0 {
		headers = r.MultipartForm.File["files"]
	}
	if len(headers) == 0 {
		writeMediaError(w, errors.New("file is required"))
		return
	}
	items := make([]mediaAsset, 0, len(headers))
	for _, header := range headers {
		validated, validateErr := validateMediaUpload(header, a.maxUpload)
		if validateErr != nil {
			writeMediaError(w, fmt.Errorf("%s: %w", header.Filename, validateErr))
			return
		}
		existing, found, findErr := a.repo.FindAssetByHash(r.Context(), tenant, validated.Hash)
		if findErr != nil {
			writeMediaError(w, findErr)
			return
		}
		if found {
			items = append(items, existing)
			continue
		}
		categoryID := strings.TrimSpace(r.FormValue("categoryId"))
		categoryCode := strings.TrimSpace(r.FormValue("categoryCode"))
		if categoryCode == "" {
			categoryCode = categoryID
		}
		key := mediaStorageKey(tenant, categoryCode, validated.Hash, validated.Extension)
		stored, storeErr := a.storage.Upload(r.Context(), key, bytes.NewReader(validated.Bytes))
		if storeErr != nil {
			writeMediaError(w, storeErr)
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			name = strings.TrimSuffix(validated.Original, "."+validated.Extension)
		}
		item := mediaAsset{ID: "media_" + newRequestID(), TenantID: tenant, Name: name, CategoryID: categoryID, AssetType: "IMAGE", MimeType: validated.MimeType, FileExt: validated.Extension, OriginalName: validated.Original, StorageProvider: stored.Provider, StorageBucket: stored.Bucket, StorageKey: stored.Key, OriginalURL: stored.PublicURL, CDNURL: stored.PublicURL, ThumbnailURL: stored.PublicURL, Width: validated.Width, Height: validated.Height, AspectRatio: validated.Ratio, FileSize: int64(len(validated.Bytes)), FileHash: validated.Hash, Status: "ACTIVE", AuditStatus: "APPROVED", SourceType: firstNonEmptyString(r.FormValue("sourceType"), "OPERATION_UPLOAD"), SourceName: r.FormValue("sourceName"), LicenseType: r.FormValue("licenseType"), LicenseNote: r.FormValue("licenseNote"), Prompt: r.FormValue("prompt"), ModelName: r.FormValue("modelName"), CopyrightOwner: r.FormValue("copyrightOwner"), Metadata: map[string]any{"keepOriginal": true}, CreatedBy: user.ID, UpdatedBy: user.ID}
		saved, saveErr := a.repo.SaveAsset(r.Context(), item)
		if saveErr != nil {
			_ = a.storage.Delete(r.Context(), stored.Key)
			writeMediaError(w, saveErr)
			return
		}
		items = append(items, saved)
	}
	status := http.StatusCreated
	if len(items) > 1 {
		w.WriteHeader(status)
		writeJSON(w, map[string]any{"items": items})
		return
	}
	w.WriteHeader(status)
	writeJSON(w, map[string]any{"item": items[0]})
}
func (a mediaAPI) updateAsset(w http.ResponseWriter, r *http.Request) {
	_, tenant, err := a.adminIdentity(r)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	patch := map[string]any{}
	if err = decodeMediaJSON(r, &patch); err != nil {
		writeMediaError(w, err)
		return
	}
	item, err := a.repo.UpdateAsset(r.Context(), tenant, r.PathValue("id"), patch)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}
func (a mediaAPI) enableAsset(w http.ResponseWriter, r *http.Request) {
	a.setAssetStatus(w, r, "ACTIVE")
}
func (a mediaAPI) disableAsset(w http.ResponseWriter, r *http.Request) {
	a.setAssetStatus(w, r, "DISABLED")
}
func (a mediaAPI) setAssetStatus(w http.ResponseWriter, r *http.Request, status string) {
	_, tenant, err := a.adminIdentity(r)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	item, err := a.repo.UpdateAsset(r.Context(), tenant, r.PathValue("id"), map[string]any{"status": status})
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}
func (a mediaAPI) deleteAsset(w http.ResponseWriter, r *http.Request) {
	_, tenant, err := a.adminIdentity(r)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	item, err := a.repo.GetAsset(r.Context(), tenant, r.PathValue("id"))
	if err != nil {
		writeMediaError(w, err)
		return
	}
	if err = a.repo.DeleteAsset(r.Context(), tenant, item.ID); err != nil {
		writeMediaError(w, err)
		return
	}
	if item.StorageProvider == "local" {
		_ = a.storage.Delete(r.Context(), item.StorageKey)
	}
	w.WriteHeader(http.StatusNoContent)
}
func (a mediaAPI) assetUsages(w http.ResponseWriter, r *http.Request) {
	_, tenant, err := a.adminIdentity(r)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	items, err := a.repo.ListAssetUsages(r.Context(), tenant, r.PathValue("id"))
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a mediaAPI) listCategories(w http.ResponseWriter, r *http.Request) {
	_, tenant, err := a.adminIdentity(r)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	items, err := a.repo.ListCategories(r.Context(), tenant, true)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}
func (a mediaAPI) createCategory(w http.ResponseWriter, r *http.Request) {
	user, tenant, err := a.adminIdentity(r)
	_ = user
	if err != nil {
		writeMediaError(w, err)
		return
	}
	var item mediaCategory
	if err = decodeMediaJSON(r, &item); err != nil {
		writeMediaError(w, err)
		return
	}
	item.TenantID = tenant
	if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Code) == "" {
		writeMediaError(w, errors.New("name and code are required"))
		return
	}
	saved, err := a.repo.SaveCategory(r.Context(), item)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"item": saved})
}
func (a mediaAPI) updateCategory(w http.ResponseWriter, r *http.Request) {
	_, tenant, err := a.adminIdentity(r)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	var item mediaCategory
	if err = decodeMediaJSON(r, &item); err != nil {
		writeMediaError(w, err)
		return
	}
	item.ID = r.PathValue("id")
	item.TenantID = tenant
	saved, err := a.repo.SaveCategory(r.Context(), item)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": saved})
}
func (a mediaAPI) deleteCategory(w http.ResponseWriter, r *http.Request) {
	_, tenant, err := a.adminIdentity(r)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	if err = a.repo.DeleteCategory(r.Context(), tenant, r.PathValue("id")); err != nil {
		writeMediaError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a mediaAPI) listPageSlots(w http.ResponseWriter, r *http.Request) {
	_, tenant, err := a.adminIdentity(r)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	page, err := mediaPageCode(r.PathValue("pageCode"))
	if err != nil {
		writeMediaError(w, err)
		return
	}
	items, err := a.repo.ListSlots(r.Context(), tenant, page, true)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "pageCode": page, "tenantId": tenant})
}
func (a mediaAPI) updatePageSlot(w http.ResponseWriter, r *http.Request) {
	_, tenant, err := a.adminIdentity(r)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	page, err := mediaPageCode(r.PathValue("pageCode"))
	if err != nil {
		writeMediaError(w, err)
		return
	}
	slotKey := r.PathValue("slotKey")
	var item pageAssetSlot
	if err = decodeMediaJSON(r, &item); err != nil {
		writeMediaError(w, err)
		return
	}
	defaults, err := a.repo.ListSlots(r.Context(), tenant, page, true)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	var base pageAssetSlot
	for _, slot := range defaults {
		if slot.SlotKey == slotKey {
			base = slot
			break
		}
	}
	if base.SlotKey == "" {
		writeMediaError(w, errMediaNotFound)
		return
	}
	base.ID = "slot_" + newRequestID()
	base.TenantID = tenant
	base.PageCode = page
	base.SlotKey = slotKey
	if item.AssetID != "" {
		if _, err = a.repo.GetAsset(r.Context(), tenant, item.AssetID); err != nil && tenant != "default" {
			_, err = a.repo.GetAsset(r.Context(), "default", item.AssetID)
		}
		if err != nil {
			writeMediaError(w, errors.New("asset does not belong to current tenant or platform"))
			return
		}
		base.AssetID = item.AssetID
	}
	if item.FallbackAssetID != "" {
		base.FallbackAssetID = item.FallbackAssetID
	}
	if item.ModuleCode != "" {
		base.ModuleCode = item.ModuleCode
	}
	if item.SlotName != "" {
		base.SlotName = item.SlotName
	}
	base.AltText = item.AltText
	base.SortOrder = item.SortOrder
	base.IsEnabled = item.IsEnabled
	base.EffectiveStartTime = item.EffectiveStartTime
	base.EffectiveEndTime = item.EffectiveEndTime
	base.ExtraConfig = item.ExtraConfig
	saved, err := a.repo.SaveSlot(r.Context(), base)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	a.invalidatePageCache(r.Context(), tenant, page)
	writeJSON(w, map[string]any{"item": saved})
}

func (a mediaAPI) getAdminPageConfig(w http.ResponseWriter, r *http.Request) {
	_, tenant, err := a.adminIdentity(r)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	page, err := mediaPageCode(r.PathValue("pageCode"))
	if err != nil {
		writeMediaError(w, err)
		return
	}
	item, err := a.repo.GetPageConfig(r.Context(), tenant, page)
	if errors.Is(err, errMediaNotFound) && tenant != "default" {
		item, err = a.repo.GetPageConfig(r.Context(), "default", page)
	}
	if err != nil {
		writeMediaError(w, err)
		return
	}
	slots, _ := a.repo.ListSlots(r.Context(), tenant, page, true)
	writeJSON(w, map[string]any{"item": item, "slots": slots})
}
func (a mediaAPI) savePageDraft(w http.ResponseWriter, r *http.Request) {
	user, tenant, err := a.adminIdentity(r)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	page, err := mediaPageCode(r.PathValue("pageCode"))
	if err != nil {
		writeMediaError(w, err)
		return
	}
	payload := map[string]any{}
	if err = decodeMediaJSON(r, &payload); err != nil {
		writeMediaError(w, err)
		return
	}
	configJSON := payload
	if nested, ok := payload["config"].(map[string]any); ok {
		configJSON = nested
	}
	item, err := a.repo.SavePageDraft(r.Context(), pageConfig{TenantID: tenant, PageCode: page, ConfigJSON: configJSON, UpdatedBy: user.ID})
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}
func (a mediaAPI) publishPage(w http.ResponseWriter, r *http.Request) {
	user, tenant, err := a.adminIdentity(r)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	page, err := mediaPageCode(r.PathValue("pageCode"))
	if err != nil {
		writeMediaError(w, err)
		return
	}
	payload := struct {
		ChangeNote string `json:"changeNote"`
	}{}
	_ = decodeMediaJSONOptional(r, &payload)
	item, err := a.repo.PublishPage(r.Context(), tenant, page, payload.ChangeNote, user.ID)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	a.invalidatePageCache(r.Context(), tenant, page)
	writeJSON(w, map[string]any{"item": item})
}
func (a mediaAPI) listPageVersions(w http.ResponseWriter, r *http.Request) {
	_, tenant, err := a.adminIdentity(r)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	page, err := mediaPageCode(r.PathValue("pageCode"))
	if err != nil {
		writeMediaError(w, err)
		return
	}
	items, err := a.repo.ListPageVersions(r.Context(), tenant, page)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}
func (a mediaAPI) rollbackPage(w http.ResponseWriter, r *http.Request) {
	user, tenant, err := a.adminIdentity(r)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	page, err := mediaPageCode(r.PathValue("pageCode"))
	if err != nil {
		writeMediaError(w, err)
		return
	}
	version, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || version < 1 {
		writeMediaError(w, errors.New("invalid page version"))
		return
	}
	item, err := a.repo.RollbackPage(r.Context(), tenant, page, version, user.ID)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	a.invalidatePageCache(r.Context(), tenant, page)
	writeJSON(w, map[string]any{"item": item})
}

func (a mediaAPI) publicPage(w http.ResponseWriter, r *http.Request) {
	pageValue := firstNonEmptyString(r.PathValue("pageCode"), r.PathValue("page"))
	page, err := mediaPageCode(pageValue)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	tenant := firstNonEmptyString(r.Header.Get("X-Tenant-Id"), r.URL.Query().Get("tenantId"), "default")
	cacheKey := "media:page:" + tenant + ":" + page
	if a.redis != nil {
		if raw, cacheErr := a.redis.Get(r.Context(), cacheKey).Bytes(); cacheErr == nil {
			w.Header().Set("X-Page-Config-Cache", "HIT")
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write(raw)
			return
		}
	}
	response, err := a.resolvePublicPage(r.Context(), tenant, page)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	raw, _ := json.Marshal(response)
	if a.redis != nil {
		_ = a.redis.Set(r.Context(), cacheKey, raw, 10*time.Minute).Err()
	}
	w.Header().Set("X-Page-Config-Cache", "MISS")
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(raw)
}
func (a mediaAPI) resolvePublicPage(ctx context.Context, tenant, page string) (map[string]any, error) {
	item, err := a.repo.GetPageConfig(ctx, tenant, page)
	if errors.Is(err, errMediaNotFound) && tenant != "default" {
		item, err = a.repo.GetPageConfig(ctx, "default", page)
	}
	if err != nil {
		return nil, err
	}
	slots, err := a.repo.ListSlots(ctx, tenant, page, true)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	slotMap := map[string]any{}
	activeSlots := make([]pageAssetSlot, 0, len(slots))
	for _, slot := range slots {
		if !slot.IsEnabled {
			continue
		}
		if slot.EffectiveStartTime != "" {
			start, _ := time.Parse(time.RFC3339, slot.EffectiveStartTime)
			if !start.IsZero() && now.Before(start) {
				continue
			}
		}
		if slot.EffectiveEndTime != "" {
			end, _ := time.Parse(time.RFC3339, slot.EffectiveEndTime)
			if !end.IsZero() && now.After(end) {
				continue
			}
		}
		if slot.MaterialURL == "" {
			slot.MaterialURL = slot.FallbackURL
		}
		slotMap[slot.SlotKey] = map[string]any{"assetId": slot.AssetID, "imageUrl": slot.MaterialURL, "fallbackUrl": firstNonEmptyString(slot.FallbackURL, localMediaFallback(page)), "altText": slot.AltText, "enabled": slot.IsEnabled, "extraConfig": slot.ExtraConfig}
		activeSlots = append(activeSlots, slot)
	}
	modules := item.ConfigJSON["modules"]
	if modules == nil {
		modules = []any{}
	}
	return map[string]any{"code": 0, "message": "success", "data": map[string]any{"pageCode": page, "tenantId": tenant, "version": strconv.Itoa(item.Version), "status": item.Status, "modules": modules, "slots": slotMap, "slotList": activeSlots}}, nil
}
func localMediaFallback(page string) string {
	switch page {
	case "profile":
		return "/static/fallbacks/default-avatar.svg"
	case "home":
		return "/static/fallbacks/default-banner.svg"
	default:
		return "/static/fallbacks/default-cover.svg"
	}
}
func (a mediaAPI) invalidatePageCache(ctx context.Context, tenant, page string) {
	if a.redis == nil {
		return
	}
	_ = a.redis.Del(ctx, "media:page:"+tenant+":"+page).Err()
}

func decodeMediaJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}
func decodeMediaJSONOptional(r *http.Request, target any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	return decodeMediaJSON(r, target)
}
func mediaIntQuery(r *http.Request, key string, fallback, min, max int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		value = fallback
	}
	if value < min {
		value = min
	}
	if value > max {
		value = max
	}
	return value
}
func writeMediaError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, errMediaNotFound):
		status = http.StatusNotFound
	case errors.Is(err, errMediaInUse):
		status = http.StatusConflict
	case errors.Is(err, errUnauthorized), errors.Is(err, errForbidden):
		status = http.StatusForbidden
	case strings.Contains(strings.ToLower(err.Error()), "unavailable"):
		status = http.StatusServiceUnavailable
	}
	writeError(w, status, err)
}

var _ = sort.Strings
