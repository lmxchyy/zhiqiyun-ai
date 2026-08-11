package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/config"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type fileCenterAPI struct {
	service  *storagecenter.Service
	store    platformStore
	sessions authSessionStore
}

func newFileCenterAPI(service *storagecenter.Service, store platformStore, sessions authSessionStore) fileCenterAPI {
	return fileCenterAPI{service: service, store: store, sessions: sessions}
}

func fileCenterOptions(cfg config.Config) storagecenter.Options {
	return storagecenter.OptionsFromConfig(cfg)
}

func storageInt64(value string, fallback int64) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func (a fileCenterAPI) identity(r *http.Request, allowTenantOverride bool) (storagecenter.AccessContext, adminUser, error) {
	userID, err := authenticatedUserID(r, a.sessions)
	if err != nil {
		return storagecenter.AccessContext{}, adminUser{}, err
	}
	var user adminUser
	if optimized, ok := a.store.(activeIdentityStore); ok {
		var found bool
		user, found, err = optimized.GetActiveUser(userID)
		if err != nil {
			return storagecenter.AccessContext{}, adminUser{}, err
		}
		if !found {
			return storagecenter.AccessContext{}, adminUser{}, errUnauthorized
		}
	} else {
		data, dataErr := a.store.AdminData()
		if dataErr != nil {
			return storagecenter.AccessContext{}, adminUser{}, dataErr
		}
		for _, candidate := range data.Users {
			if candidate.ID == userID {
				user = candidate
				break
			}
		}
	}
	if user.ID == "" {
		return storagecenter.AccessContext{}, user, errUnauthorized
	}
	role := strings.ToUpper(strings.TrimSpace(user.Role))
	isAdmin := role == "SUPER_ADMIN" || role == "PLATFORM_ADMIN" || role == "ADMIN"
	tenantID := effectiveTenantID(user)
	if isAdmin && tenantID == "" {
		tenantID = "tenant_default"
	}
	if allowTenantOverride && isAdmin {
		requested := firstNonEmptyString(r.URL.Query().Get("tenantId"), r.Header.Get("X-Tenant-Id"))
		if requested != "" {
			tenantID = requested
		}
	}
	return storagecenter.AccessContext{TenantID: tenantID, UserID: user.ID, IsAdmin: isAdmin}, user, nil
}

type fileUploadInitRequest struct {
	FileName             string     `json:"file_name"`
	FileNameCamel        string     `json:"fileName"`
	FileSize             int64      `json:"file_size"`
	FileSizeCamel        int64      `json:"fileSize"`
	MIMEType             string     `json:"mime_type"`
	MIMETypeCamel        string     `json:"mimeType"`
	BusinessType         string     `json:"business_type"`
	BusinessTypeCamel    string     `json:"businessType"`
	BusinessID           string     `json:"business_id"`
	BusinessIDCamel      string     `json:"businessId"`
	Visibility           string     `json:"visibility"`
	IsTemporary          bool       `json:"is_temporary"`
	IsTemporaryCamel     bool       `json:"isTemporary"`
	ExpiresAt            *time.Time `json:"expires_at"`
	ExpiresAtCamel       *time.Time `json:"expiresAt"`
	StorageConfigID      string     `json:"storage_config_id"`
	StorageConfigIDCamel string     `json:"storageConfigId"`
}

func (a fileCenterAPI) initUpload(w http.ResponseWriter, r *http.Request) {
	access, _, err := a.identity(r, false)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	var request fileUploadInitRequest
	if err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		writeStorageError(w, err)
		return
	}
	fileSize := request.FileSize
	if fileSize == 0 {
		fileSize = request.FileSizeCamel
	}
	expiresAt := request.ExpiresAt
	if expiresAt == nil {
		expiresAt = request.ExpiresAtCamel
	}
	ticket, err := a.service.InitUpload(r.Context(), storagecenter.UploadInitInput{
		TenantID: access.TenantID, UserID: access.UserID,
		FileName: firstNonEmptyString(request.FileName, request.FileNameCamel), FileSize: fileSize,
		MIMEType:     firstNonEmptyString(request.MIMEType, request.MIMETypeCamel),
		BusinessType: firstNonEmptyString(request.BusinessType, request.BusinessTypeCamel),
		BusinessID:   firstNonEmptyString(request.BusinessID, request.BusinessIDCamel), Visibility: request.Visibility,
		IsTemporary: request.IsTemporary || request.IsTemporaryCamel, ExpiresAt: expiresAt,
		StorageConfigID: firstNonEmptyString(request.StorageConfigID, request.StorageConfigIDCamel),
	})
	if err != nil {
		writeStorageError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{
		"fileId": ticket.File.FileID, "file_id": ticket.File.FileID,
		"objectKey": ticket.File.ObjectKey, "object_key": ticket.File.ObjectKey,
		"uploadMethod": ticket.UploadMethod, "upload_method": ticket.UploadMethod,
		"uploadUrl": ticket.UploadURL, "upload_url": ticket.UploadURL,
		"expiresIn": ticket.ExpiresIn, "expires_in": ticket.ExpiresIn, "headers": ticket.Headers,
	})
}

func (a fileCenterAPI) completeUpload(w http.ResponseWriter, r *http.Request) {
	access, _, err := a.identity(r, false)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	var request struct {
		FileID      string `json:"file_id"`
		FileIDCamel string `json:"fileId"`
	}
	if err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		writeStorageError(w, err)
		return
	}
	file, err := a.service.CompleteUpload(r.Context(), access, firstNonEmptyString(request.FileID, request.FileIDCamel))
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, map[string]any{"file": file})
}

func (a fileCenterAPI) getFile(w http.ResponseWriter, r *http.Request) {
	access, _, err := a.identity(r, false)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	file, err := a.service.GetFile(r.Context(), access, fileIDFromRequest(r))
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, map[string]any{"file": file})
}

func (a fileCenterAPI) accessURL(download bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, _, err := a.identity(r, strings.HasPrefix(r.URL.Path, "/api/v1/admin/"))
		if err != nil {
			writeStorageError(w, err)
			return
		}
		ticket, err := a.service.AccessURL(r.Context(), access, fileIDFromRequest(r), download)
		if err != nil {
			writeStorageError(w, err)
			return
		}
		writeJSON(w, map[string]any{"file": ticket.File, "url": ticket.URL, "expiresIn": ticket.ExpiresIn, "expires_in": ticket.ExpiresIn})
	}
}

func (a fileCenterAPI) deleteFile(w http.ResponseWriter, r *http.Request) {
	access, _, err := a.identity(r, strings.HasPrefix(r.URL.Path, "/api/v1/admin/"))
	if err != nil {
		writeStorageError(w, err)
		return
	}
	file, err := a.service.Delete(r.Context(), access, fileIDFromRequest(r))
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, map[string]any{"file": file})
}

func (a fileCenterAPI) restoreFile(w http.ResponseWriter, r *http.Request) {
	access, _, err := a.identity(r, strings.HasPrefix(r.URL.Path, "/api/v1/admin/"))
	if err != nil {
		writeStorageError(w, err)
		return
	}
	file, err := a.service.Restore(r.Context(), access, fileIDFromRequest(r))
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, map[string]any{"file": file})
}

func (a fileCenterAPI) permanentDeleteFile(w http.ResponseWriter, r *http.Request) {
	access, _, err := a.identity(r, strings.HasPrefix(r.URL.Path, "/api/v1/admin/"))
	if err != nil {
		writeStorageError(w, err)
		return
	}
	if err = a.service.PermanentDelete(r.Context(), access, fileIDFromRequest(r)); err != nil {
		writeStorageError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a fileCenterAPI) adminListFiles(w http.ResponseWriter, r *http.Request) {
	access, _, err := a.identity(r, true)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	page := storageIntQuery(r, "page", 1, 1, 100000)
	pageSize := storageIntQuery(r, "pageSize", 30, 1, 200)
	tenantID := access.TenantID
	if strings.EqualFold(r.URL.Query().Get("scope"), "platform") && access.IsAdmin {
		tenantID = ""
	}
	items, total, err := a.service.ListFiles(r.Context(), storagecenter.FileFilter{
		TenantID: tenantID, UserID: r.URL.Query().Get("userId"), Query: r.URL.Query().Get("q"),
		BusinessType: r.URL.Query().Get("businessType"), Status: r.URL.Query().Get("status"), Provider: r.URL.Query().Get("provider"),
		Limit: pageSize, Offset: (page - 1) * pageSize,
	})
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}

func (a fileCenterAPI) adminGetFile(w http.ResponseWriter, r *http.Request) {
	access, _, err := a.identity(r, true)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	access.IsAdmin = true
	file, err := a.service.GetFile(r.Context(), access, fileIDFromRequest(r))
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, map[string]any{"file": file})
}

func (a fileCenterAPI) adminOverview(w http.ResponseWriter, r *http.Request) {
	access, _, err := a.identity(r, true)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	tenantID := access.TenantID
	if strings.EqualFold(r.URL.Query().Get("scope"), "platform") {
		tenantID = ""
	}
	overview, err := a.service.Overview(r.Context(), tenantID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, overview)
}

func (a fileCenterAPI) listConfigs(w http.ResponseWriter, r *http.Request) {
	access, _, err := a.identity(r, true)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	items, err := a.service.ListConfigs(r.Context(), access.TenantID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

type storageConfigRequest struct {
	TenantID        string `json:"tenantId"`
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	Endpoint        string `json:"endpoint"`
	SigningEndpoint string `json:"signingEndpoint"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	AccessKey       string `json:"accessKey"`
	SecretKey       string `json:"secretKey"`
	SessionToken    string `json:"sessionToken"`
	PublicDomain    string `json:"publicDomain"`
	CDNDomain       string `json:"cdnDomain"`
	UseSSL          bool   `json:"useSSL"`
	ForcePathStyle  bool   `json:"forcePathStyle"`
	IsDefault       bool   `json:"isDefault"`
	Status          string `json:"status"`
}

func (a fileCenterAPI) saveConfig(create bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, user, err := a.identity(r, true)
		if err != nil {
			writeStorageError(w, err)
			return
		}
		var request storageConfigRequest
		if err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
			writeStorageError(w, err)
			return
		}
		id := ""
		if !create {
			id = r.PathValue("id")
		}
		tenantID := firstNonEmptyString(request.TenantID, access.TenantID)
		if strings.EqualFold(request.TenantID, "platform") && access.IsAdmin {
			tenantID = storagecenter.PlatformTenantID
		}
		item, err := a.service.SaveConfig(r.Context(), storagecenter.Config{
			ID: id, TenantID: tenantID, Name: request.Name, Provider: request.Provider, Endpoint: request.Endpoint, SigningEndpoint: request.SigningEndpoint,
			Region: request.Region, Bucket: request.Bucket, PublicDomain: request.PublicDomain, CDNDomain: request.CDNDomain,
			UseSSL: request.UseSSL, ForcePathStyle: request.ForcePathStyle, IsDefault: request.IsDefault,
			Status: request.Status, CreatedBy: user.ID, UpdatedBy: user.ID,
		}, request.AccessKey, request.SecretKey, request.SessionToken)
		if err != nil {
			writeStorageError(w, err)
			return
		}
		if create {
			w.WriteHeader(http.StatusCreated)
		}
		writeJSON(w, map[string]any{"item": item})
	}
}

func (a fileCenterAPI) deleteConfig(w http.ResponseWriter, r *http.Request) {
	if _, _, err := a.identity(r, true); err != nil {
		writeStorageError(w, err)
		return
	}
	if err := a.service.DeleteConfig(r.Context(), r.PathValue("id")); err != nil {
		writeStorageError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a fileCenterAPI) testConfig(w http.ResponseWriter, r *http.Request) {
	if _, _, err := a.identity(r, true); err != nil {
		writeStorageError(w, err)
		return
	}
	if err := a.service.TestConfig(r.Context(), r.PathValue("id")); err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, map[string]any{"status": "SUCCESS", "message": "connection succeeded"})
}

func (a fileCenterAPI) getQuota(w http.ResponseWriter, r *http.Request) {
	access, _, err := a.identity(r, true)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	tenantID := firstNonEmptyString(r.URL.Query().Get("tenantId"), access.TenantID)
	quota, err := a.service.Quota(r.Context(), tenantID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": []storagecenter.Quota{quota}})
}

func (a fileCenterAPI) updateQuota(w http.ResponseWriter, r *http.Request) {
	if _, _, err := a.identity(r, true); err != nil {
		writeStorageError(w, err)
		return
	}
	var request struct {
		QuotaBytes      int64 `json:"quotaBytes"`
		WarningPercent  int   `json:"warningPercent"`
		CriticalPercent int   `json:"criticalPercent"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		writeStorageError(w, err)
		return
	}
	quota, err := a.service.UpdateQuota(r.Context(), storagecenter.Quota{TenantID: r.PathValue("tenantId"), QuotaBytes: request.QuotaBytes, WarningPercent: request.WarningPercent, CriticalPercent: request.CriticalPercent})
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": quota})
}

func fileIDFromRequest(r *http.Request) string {
	return firstNonEmptyString(r.PathValue("fileId"), r.PathValue("id"))
}

func storageIntQuery(r *http.Request, key string, fallback int, min int, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get(key)))
	if err != nil || value < min {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func writeStorageError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, errUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, storagecenter.ErrFileForbidden):
		status = http.StatusForbidden
	case errors.Is(err, storagecenter.ErrFileNotFound), errors.Is(err, storagecenter.ErrConfigNotFound),
		errors.Is(err, storagecenter.ErrMultipartNotFound):
		status = http.StatusNotFound
	case errors.Is(err, storagecenter.ErrQuotaExceeded):
		status = http.StatusInsufficientStorage
	case errors.Is(err, storagecenter.ErrInvalidFileType), errors.Is(err, storagecenter.ErrInvalidFileSize),
		errors.Is(err, storagecenter.ErrUploadConfirmFailed), errors.Is(err, storagecenter.ErrInvalidMultipartPart),
		errors.Is(err, storagecenter.ErrMultipartExpired), errors.Is(err, storagecenter.ErrMultipartState):
		status = http.StatusBadRequest
	case errors.Is(err, storagecenter.ErrDeleteFailed), errors.Is(err, storagecenter.ErrConfigDisabled):
		status = http.StatusConflict
	case errors.Is(err, storagecenter.ErrConnectionFailed):
		status = http.StatusBadGateway
	case errors.Is(err, storagecenter.ErrSecretCipherRequired):
		status = http.StatusServiceUnavailable
	}
	writeJSONStatus(w, status, map[string]any{"error": err.Error(), "code": storageErrorCode(err)})
}

func storageErrorCode(err error) string {
	for _, code := range []string{
		"STORAGE_CONFIG_NOT_FOUND", "STORAGE_CONFIG_DISABLED", "STORAGE_PROVIDER_UNSUPPORTED", "STORAGE_CONNECTION_FAILED",
		"STORAGE_FILE_NOT_FOUND", "STORAGE_FILE_FORBIDDEN", "STORAGE_FILE_EXPIRED", "STORAGE_FILE_QUARANTINED",
		"STORAGE_QUOTA_EXCEEDED", "STORAGE_INVALID_FILE_TYPE", "STORAGE_INVALID_FILE_SIZE", "STORAGE_UPLOAD_CONFIRM_FAILED", "STORAGE_DELETE_FAILED",
		"STORAGE_INVALID_MULTIPART_PART", "STORAGE_MULTIPART_NOT_FOUND", "STORAGE_MULTIPART_EXPIRED", "STORAGE_MULTIPART_INVALID_STATE", "STORAGE_MULTIPART_INIT_FAILED",
	} {
		if strings.Contains(err.Error(), code) {
			return code
		}
	}
	return "STORAGE_INTERNAL_ERROR"
}

func writeJSONStatus(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
