package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
	"xianzhi-ai/backend-go/internal/config"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type smartVideoAPI struct {
	service  *smartvideo.Service
	analysis *smartvideo.AnalysisService
	plans    *smartvideo.PlanService
	exports  *smartvideo.ExportService
	files    fileCenterAPI
}

func newSmartVideoAPI(service *smartvideo.Service, analysis *smartvideo.AnalysisService, plans *smartvideo.PlanService, exports *smartvideo.ExportService, files fileCenterAPI) smartVideoAPI {
	return smartVideoAPI{service: service, analysis: analysis, plans: plans, exports: exports, files: files}
}

type smartVideoFileResolver struct{ service *storagecenter.Service }

func (r smartVideoFileResolver) ResolveFile(ctx context.Context, access smartvideo.Access, fileID string) (smartvideo.FileReference, error) {
	file, err := r.service.GetFile(ctx, storagecenter.AccessContext{TenantID: access.TenantID, UserID: access.UserID}, fileID)
	if err != nil {
		return smartvideo.FileReference{}, err
	}
	return smartvideo.FileReference{
		FileID: file.FileID, TenantID: file.TenantID, UserID: file.UserID, ObjectKey: file.ObjectKey, Status: file.Status,
		Metadata: smartvideo.AssetMetadata{
			OriginalName: file.OriginalName, MIMEType: file.MIMEType, FileSize: file.FileSize,
			FileHash: file.FileHash,
		},
	}, nil
}

func (a smartVideoAPI) access(r *http.Request) (smartvideo.Access, error) {
	access, _, err := a.files.identity(r, false)
	if err != nil {
		return smartvideo.Access{}, err
	}
	return smartvideo.Access{TenantID: access.TenantID, UserID: access.UserID}, nil
}

func (a smartVideoAPI) projects(w http.ResponseWriter, r *http.Request) {
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := a.service.ListProjects(r.Context(), access)
		if err != nil {
			writeSmartVideoError(w, err)
			return
		}
		writeJSON(w, map[string]any{"items": items})
	case http.MethodPost:
		var input smartvideo.CreateProjectInput
		if err := decodeSmartVideoJSON(w, r, &input); err != nil {
			writeSmartVideoError(w, err)
			return
		}
		item, err := a.service.CreateProject(r.Context(), access, input)
		if err != nil {
			writeSmartVideoError(w, err)
			return
		}
		writeJSONStatus(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a smartVideoAPI) project(w http.ResponseWriter, r *http.Request) {
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	switch r.Method {
	case http.MethodGet:
		item, err := a.service.GetProject(r.Context(), access, id)
		if err != nil {
			writeSmartVideoError(w, err)
			return
		}
		writeJSON(w, item)
	case http.MethodPatch:
		var input smartvideo.UpdateProjectInput
		if err := decodeSmartVideoJSON(w, r, &input); err != nil {
			writeSmartVideoError(w, err)
			return
		}
		item, err := a.service.UpdateProject(r.Context(), access, id, input)
		if err != nil {
			writeSmartVideoError(w, err)
			return
		}
		writeJSON(w, item)
	case http.MethodDelete:
		if err := a.service.DeleteProject(r.Context(), access, id); err != nil {
			writeSmartVideoError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a smartVideoAPI) assets(w http.ResponseWriter, r *http.Request) {
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	projectID := strings.TrimSpace(r.PathValue("id"))
	switch r.Method {
	case http.MethodGet:
		items, err := a.service.ListAssets(r.Context(), access, projectID)
		if err != nil {
			writeSmartVideoError(w, err)
			return
		}
		writeJSON(w, map[string]any{"items": items})
	case http.MethodPost:
		var input smartvideo.CreateAssetInput
		if err := decodeSmartVideoJSON(w, r, &input); err != nil {
			writeSmartVideoError(w, err)
			return
		}
		item, err := a.service.CreateAsset(r.Context(), access, projectID, input)
		if err != nil {
			writeSmartVideoError(w, err)
			return
		}
		writeJSONStatus(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a smartVideoAPI) reorderAssets(w http.ResponseWriter, r *http.Request) {
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	var input smartvideo.ReorderAssetsInput
	if err := decodeSmartVideoJSON(w, r, &input); err != nil {
		writeSmartVideoError(w, err)
		return
	}
	items, err := a.service.ReorderAssets(r.Context(), access, r.PathValue("id"), input)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a smartVideoAPI) deleteAsset(w http.ResponseWriter, r *http.Request) {
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	if err := a.service.DeleteAsset(r.Context(), access, r.PathValue("id"), r.PathValue("assetId")); err != nil {
		writeSmartVideoError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a smartVideoAPI) createRenderTask(w http.ResponseWriter, r *http.Request) {
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	if a.exports == nil {
		writeSmartVideoError(w, smartvideo.ErrExportNotReady)
		return
	}
	var input smartvideo.ExportCreateInput
	if err := decodeSmartVideoJSON(w, r, &input); err != nil {
		writeSmartVideoError(w, err)
		return
	}
	if input.IdempotencyKey == "" {
		input.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	task, err := a.exports.CreateExport(r.Context(), access, r.PathValue("id"), input)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusAccepted, task)
}

func (a smartVideoAPI) renderTask(w http.ResponseWriter, r *http.Request) {
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	task, err := a.service.GetRenderTask(r.Context(), access, r.PathValue("id"), r.PathValue("taskId"))
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	if task.OutputFileID != "" {
		if ticket, ticketErr := a.files.service.AccessURL(r.Context(), storagecenter.AccessContext{TenantID: access.TenantID, UserID: access.UserID}, task.OutputFileID, false); ticketErr == nil {
			task.Output.VideoURL = ticket.URL
		}
	}
	if task.CoverFileID != "" {
		if ticket, ticketErr := a.files.service.AccessURL(r.Context(), storagecenter.AccessContext{TenantID: access.TenantID, UserID: access.UserID}, task.CoverFileID, false); ticketErr == nil {
			task.Output.CoverURL = ticket.URL
		}
	}
	writeJSON(w, task)
}

func (a smartVideoAPI) cancelRenderTask(w http.ResponseWriter, r *http.Request) {
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	if a.exports == nil {
		writeSmartVideoError(w, smartvideo.ErrExportNotReady)
		return
	}
	task, err := a.exports.CancelExport(r.Context(), access, r.PathValue("id"), r.PathValue("taskId"))
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	writeJSON(w, task)
}

func (a smartVideoAPI) retryRenderTask(w http.ResponseWriter, r *http.Request) {
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	if a.exports == nil {
		writeSmartVideoError(w, smartvideo.ErrExportNotReady)
		return
	}
	task, err := a.exports.RetryExport(r.Context(), access, r.PathValue("id"), r.PathValue("taskId"))
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusAccepted, task)
}

func (a smartVideoAPI) analyze(w http.ResponseWriter, r *http.Request) {
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	if a.analysis == nil {
		writeSmartVideoError(w, smartvideo.ErrAnalysisNotReady)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writeSmartVideoError(w, smartvideo.ErrIdempotencyKeyRequired)
		return
	}
	summary, err := a.analysis.RequestProjectAnalysis(r.Context(), access, r.PathValue("id"), idempotencyKey)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusAccepted, summary)
}

func (a smartVideoAPI) analysisStatus(w http.ResponseWriter, r *http.Request) {
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	if a.analysis == nil {
		writeSmartVideoError(w, smartvideo.ErrAnalysisNotReady)
		return
	}
	summary, err := a.analysis.GetProjectAnalysis(r.Context(), access, r.PathValue("id"))
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	writeJSON(w, summary)
}

func (a smartVideoAPI) retryAnalysis(w http.ResponseWriter, r *http.Request) {
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	if a.analysis == nil {
		writeSmartVideoError(w, smartvideo.ErrAnalysisNotReady)
		return
	}
	summary, err := a.analysis.RetryAsset(r.Context(), access, r.PathValue("id"), r.PathValue("assetId"))
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusAccepted, summary)
}

func smartVideoAnalysisOptions(cfg config.Config) smartvideo.AnalysisOptions {
	maxAttempts, err := strconv.Atoi(strings.TrimSpace(cfg.SmartVideoAnalysisMaxAttempts))
	if err != nil || maxAttempts <= 0 || maxAttempts > 20 {
		maxAttempts = 3
	}
	return smartvideo.AnalysisOptions{Enabled: cfg.SmartVideoAnalysisEnabled, MaxAttempts: maxAttempts}
}

func decodeSmartVideoJSON(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeSmartVideoError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, errUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, smartvideo.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, smartvideo.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, smartvideo.ErrInvalidInput), errors.Is(err, smartvideo.ErrIdempotencyKeyRequired):
		status = http.StatusBadRequest
	case errors.Is(err, smartvideo.ErrContentSafetyRejected):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, smartvideo.ErrInvalidStateTransition), errors.Is(err, smartvideo.ErrProjectNotConfirmed), errors.Is(err, smartvideo.ErrVersionImmutable),
		errors.Is(err, smartvideo.ErrRenderNotCancellable), errors.Is(err, smartvideo.ErrQuoteExpired):
		status = http.StatusConflict
	case errors.Is(err, smartvideo.ErrInsufficientPoints):
		status = http.StatusPaymentRequired
		if _, ok := err.(*InsufficientPointsError); !ok {
			err = newInsufficientPointsError(0, 0)
		}
	case errors.Is(err, smartvideo.ErrFileNotReady),
		errors.Is(err, storagecenter.ErrFileNotFound),
		errors.Is(err, storagecenter.ErrFileForbidden):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, smartvideo.ErrAnalysisDisabled), errors.Is(err, smartvideo.ErrAnalysisNotReady), errors.Is(err, smartvideo.ErrPlanNotReady),
		errors.Is(err, smartvideo.ErrExportNotReady):
		status = http.StatusServiceUnavailable
	case errors.Is(err, smartvideo.ErrAnalysisNotFailed), errors.Is(err, smartvideo.ErrPlanDailyLimitExceeded):
		status = http.StatusConflict
	case isEditPlanValidationError(err):
		status = http.StatusBadRequest
	}
	writeError(w, status, err)
}

func isEditPlanValidationError(err error) bool {
	var validation *smartvideo.EditPlanValidationError
	return errors.As(err, &validation)
}
