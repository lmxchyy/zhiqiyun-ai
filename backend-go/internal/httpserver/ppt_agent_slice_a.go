package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type pptAgentGuideRequest struct {
	IdempotencyKey    string `json:"idempotencyKey"`
	Text              string `json:"text"`
	Audience          string `json:"audience,omitempty"`
	Scenario          string `json:"scenario,omitempty"`
	Language          string `json:"language,omitempty"`
	ProfessionalStyle string `json:"professionalStyle,omitempty"`
	PageCount         int    `json:"pageCount,omitempty"`
	ResearchRequired  *bool  `json:"researchRequired,omitempty"`
}

type pptAgentOutlineUpdateRequest struct {
	ExpectedRevision int                         `json:"expectedRevision"`
	Commands         []pptapp.OutlineEditCommand `json:"commands"`
}

type pptAgentOutlineApproveRequest struct {
	ExpectedRevision int `json:"expectedRevision"`
}

func (a api) guidePPTAgent(w http.ResponseWriter, r *http.Request) {
	user, ok := a.pptAgentUser(w, r)
	if !ok {
		return
	}
	var request pptAgentGuideRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.Text = strings.TrimSpace(request.Text)
	if request.IdempotencyKey == "" || request.Text == "" {
		writeError(w, http.StatusBadRequest, pptapp.ErrGenerationJobInvalid)
		return
	}
	if err := a.checkMiniProgramText(r.Context(), r, user, request.Text); err != nil {
		writeContentSecurityError(w, err)
		return
	}
	result, err := a.pptAgentService.Guide(r.Context(), pptapp.GuideAgentRequest{
		TenantID: effectiveTenantID(user), UserID: user.ID, OrganizationID: user.OrganizationID,
		IdempotencyKey: request.IdempotencyKey,
		Request: pptapp.IntentRequest{
			Text: request.Text, Audience: request.Audience, Scenario: request.Scenario, Language: request.Language,
			ProfessionalStyle: request.ProfessionalStyle, PageCount: request.PageCount, ResearchRequired: request.ResearchRequired,
		},
		Now: time.Now().UTC(),
	})
	if err != nil {
		writePPTAgentError(w, err)
		return
	}
	writeJSON(w, result)
}

func (a api) getPPTAgentState(w http.ResponseWriter, r *http.Request) {
	user, ok := a.pptAgentUser(w, r)
	if !ok {
		return
	}
	state, err := a.pptAgentService.Get(r.Context(), pptapp.GenerationJobScope{TenantID: effectiveTenantID(user), UserID: user.ID}, r.PathValue("jobId"))
	if err != nil {
		writePPTAgentError(w, err)
		return
	}
	writeJSON(w, state)
}

func (a api) downloadPPTAgentDeck(w http.ResponseWriter, r *http.Request) {
	user, ok := a.pptAgentUser(w, r)
	if !ok {
		return
	}
	state, err := a.pptAgentService.Get(r.Context(), pptapp.GenerationJobScope{TenantID: effectiveTenantID(user), UserID: user.ID}, r.PathValue("jobId"))
	if err != nil {
		writePPTAgentError(w, err)
		return
	}
	if state.Job.Status != pptapp.GenerationJobSucceeded || state.Job.Stage != pptapp.GenerationStageCompleted || state.Job.FileID == "" {
		writePPTAgentError(w, pptapp.ErrGenerationJobNotReady)
		return
	}
	if a.fileService == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("private file storage is unavailable"))
		return
	}
	ticket, err := a.fileService.AccessURL(r.Context(), storagecenter.AccessContext{TenantID: effectiveTenantID(user), UserID: user.ID}, state.Job.FileID, true)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]any{"url": ticket.URL, "expiresIn": ticket.ExpiresIn, "fileId": ticket.File.FileID})
}

func (a api) updatePPTAgentOutline(w http.ResponseWriter, r *http.Request) {
	user, ok := a.pptAgentUser(w, r)
	if !ok {
		return
	}
	var request pptAgentOutlineUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	state, err := a.pptAgentService.UpdateOutline(
		r.Context(), pptapp.GenerationJobScope{TenantID: effectiveTenantID(user), UserID: user.ID},
		r.PathValue("jobId"), request.ExpectedRevision, request.Commands, time.Now().UTC(),
	)
	if err != nil {
		writePPTAgentError(w, err)
		return
	}
	writeJSON(w, state)
}

func (a api) approvePPTAgentOutline(w http.ResponseWriter, r *http.Request) {
	user, ok := a.pptAgentUser(w, r)
	if !ok {
		return
	}
	var request pptAgentOutlineApproveRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	state, err := a.pptAgentService.ApproveOutline(
		r.Context(), pptapp.GenerationJobScope{TenantID: effectiveTenantID(user), UserID: user.ID},
		r.PathValue("jobId"), request.ExpectedRevision, time.Now().UTC(),
	)
	if err != nil {
		writePPTAgentError(w, err)
		return
	}
	writeJSON(w, state)
}

func (a api) retryPPTAgentPlanning(w http.ResponseWriter, r *http.Request) {
	user, ok := a.pptAgentUser(w, r)
	if !ok {
		return
	}
	state, err := a.pptAgentService.Retry(
		r.Context(), pptapp.GenerationJobScope{TenantID: effectiveTenantID(user), UserID: user.ID},
		r.PathValue("jobId"), time.Now().UTC(),
	)
	if err != nil {
		writePPTAgentError(w, err)
		return
	}
	writeJSON(w, state)
}

func (a api) pptAgentUser(w http.ResponseWriter, r *http.Request) (adminUser, bool) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return adminUser{}, false
	}
	if a.pptAgentService == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("ppt v2 agent planning is unavailable"))
		return adminUser{}, false
	}
	return user, true
}

func writePPTAgentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pptapp.ErrGenerationJobNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, pptapp.ErrStaleOutlineRevision), errors.Is(err, pptapp.ErrOutlinePlanApproved), errors.Is(err, pptapp.ErrGenerationJobTransition), errors.Is(err, pptapp.ErrGenerationJobIdempotencyConflict), errors.Is(err, pptapp.ErrGenerationJobTerminal), errors.Is(err, pptapp.ErrGenerationJobNotReady):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, pptapp.ErrGenerationJobInvalid), errors.Is(err, pptapp.ErrInvalidResearchPack), errors.Is(err, pptapp.ErrInvalidStoryline), errors.Is(err, pptapp.ErrInvalidOutlinePlan), errors.Is(err, pptapp.ErrOutlineSlideNotFound):
		writeError(w, http.StatusBadRequest, err)
	default:
		writeError(w, http.StatusBadGateway, err)
	}
}
