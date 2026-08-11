package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
)

func (a smartVideoAPI) planTasks(w http.ResponseWriter, r *http.Request) {
	if a.plans == nil {
		writeSmartVideoError(w, smartvideo.ErrPlanNotReady)
		return
	}
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	projectID := strings.TrimSpace(r.PathValue("id"))
	switch r.Method {
	case http.MethodPost:
		var input smartvideo.CreatePlanTaskInput
		if err := decodeSmartVideoJSON(w, r, &input); err != nil {
			writeSmartVideoError(w, err)
			return
		}
		if input.IdempotencyKey == "" {
			input.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		}
		task, err := a.plans.CreatePlanTask(r.Context(), access, projectID, input)
		if err != nil {
			writeSmartVideoError(w, err)
			return
		}
		writeJSONStatus(w, http.StatusCreated, task)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a smartVideoAPI) planTask(w http.ResponseWriter, r *http.Request) {
	if a.plans == nil {
		writeSmartVideoError(w, smartvideo.ErrPlanNotReady)
		return
	}
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	task, err := a.plans.GetPlanTask(r.Context(), access, strings.TrimSpace(r.PathValue("taskId")))
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	if task.ProjectID != strings.TrimSpace(r.PathValue("id")) {
		writeSmartVideoError(w, smartvideo.ErrNotFound)
		return
	}
	writeJSON(w, task)
}

func (a smartVideoAPI) versions(w http.ResponseWriter, r *http.Request) {
	if a.plans == nil {
		writeSmartVideoError(w, smartvideo.ErrPlanNotReady)
		return
	}
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	items, err := a.plans.ListVersions(r.Context(), access, strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a smartVideoAPI) version(w http.ResponseWriter, r *http.Request) {
	if a.plans == nil {
		writeSmartVideoError(w, smartvideo.ErrPlanNotReady)
		return
	}
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, err := a.plans.GetVersion(r.Context(), access, strings.TrimSpace(r.PathValue("id")), strings.TrimSpace(r.PathValue("versionId")))
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	writeJSON(w, item)
}

type reviseVersionRequest struct {
	Plan       *smartvideo.EditPlanV1 `json:"plan"`
	ChangeNote string                 `json:"changeNote"`
	smartvideo.EditPlanV1
}

func (a smartVideoAPI) reviseVersion(w http.ResponseWriter, r *http.Request) {
	if a.plans == nil {
		writeSmartVideoError(w, smartvideo.ErrPlanNotReady)
		return
	}
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body reviseVersionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeSmartVideoError(w, err)
		return
	}
	plan := body.EditPlanV1
	if body.Plan != nil {
		plan = *body.Plan
	}
	item, err := a.plans.RevisePlan(r.Context(), access, strings.TrimSpace(r.PathValue("id")), strings.TrimSpace(r.PathValue("versionId")), smartvideo.RevisePlanInput{
		Plan: plan, ChangeNote: body.ChangeNote,
	})
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, item)
}

func (a smartVideoAPI) confirmVersion(w http.ResponseWriter, r *http.Request) {
	if a.plans == nil {
		writeSmartVideoError(w, smartvideo.ErrPlanNotReady)
		return
	}
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	project, version, err := a.plans.ConfirmPlan(r.Context(), access, strings.TrimSpace(r.PathValue("id")), strings.TrimSpace(r.PathValue("versionId")))
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	writeJSON(w, map[string]any{"project": project, "confirmedVersion": version})
}

func (a smartVideoAPI) renderEstimate(w http.ResponseWriter, r *http.Request) {
	if a.plans == nil {
		writeSmartVideoError(w, smartvideo.ErrPlanNotReady)
		return
	}
	access, err := a.access(r)
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	quote, err := a.plans.EstimateRender(r.Context(), access, strings.TrimSpace(r.PathValue("id")), strings.TrimSpace(r.PathValue("versionId")))
	if err != nil {
		writeSmartVideoError(w, err)
		return
	}
	writeJSON(w, quote)
}
