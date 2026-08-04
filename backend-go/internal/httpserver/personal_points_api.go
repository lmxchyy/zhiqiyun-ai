package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type adminPointMutationRequest struct {
	Points         int64  `json:"points"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotencyKey"`
}

func decodeAdminPointMutation(r *http.Request) (adminPointMutationRequest, error) {
	var request adminPointMutationRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("request body must contain one JSON object")
		}
		return request, err
	}
	request.Reason = strings.TrimSpace(request.Reason)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.Points == 0 || request.Reason == "" || request.IdempotencyKey == "" {
		return request, ErrInvalidPointCommand
	}
	return request, nil
}

func (a adminAPI) customerPointGift(w http.ResponseWriter, r *http.Request) {
	request, err := decodeAdminPointMutation(r)
	if err != nil || request.Points <= 0 {
		if err == nil {
			err = ErrInvalidPointCommand
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	a.customerPointGrant(w, r, request, PointSourceAdminGift, "ADMIN_GIFT")
}

func (a adminAPI) customerPointCorrection(w http.ResponseWriter, r *http.Request) {
	request, err := decodeAdminPointMutation(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	userID := strings.TrimSpace(r.PathValue("id"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, errors.New("user id is required"))
		return
	}
	account, err := a.store.PointAccount(userID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	service, err := personalPointServiceForStore(a.store)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	result, err := service.Correct(r.Context(), PersonalPointCorrectionCommand{
		AccountID: account.ID, UserID: userID, Points: request.Points, Reason: request.Reason, IdempotencyKey: request.IdempotencyKey,
		Audit: PersonalPointAudit{ActorID: actorID, ActorRole: actorRole, Action: "personal_points.admin_correction", Method: r.Method, Path: r.URL.Path, RequestID: requestIDFromPointMutation(r, request.IdempotencyKey)},
	})
	if errors.Is(err, ErrIdempotencyConflict) {
		writeError(w, http.StatusConflict, err)
		return
	}
	if errors.Is(err, ErrInvalidPointCommand) || errors.Is(err, ErrInsufficientPoints) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, result)
}

func (a adminAPI) customerPointGrant(w http.ResponseWriter, r *http.Request, request adminPointMutationRequest, source PointSource, referenceType string) {
	userID := strings.TrimSpace(r.PathValue("id"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, errors.New("user id is required"))
		return
	}
	account, err := a.store.PointAccount(userID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	service, err := personalPointServiceForStore(a.store)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	result, err := service.Grant(r.Context(), PersonalPointGrantCommand{
		AccountID: account.ID, UserID: userID, Source: source, Points: request.Points,
		ReferenceType: referenceType, ReferenceID: request.IdempotencyKey, IdempotencyKey: request.IdempotencyKey, Reason: request.Reason,
		Audit: PersonalPointAudit{ActorID: actorID, ActorRole: actorRole, Action: "personal_points.admin_gift", Method: r.Method, Path: r.URL.Path, RequestID: requestIDFromPointMutation(r, request.IdempotencyKey)},
	})
	if errors.Is(err, ErrIdempotencyConflict) {
		writeError(w, http.StatusConflict, err)
		return
	}
	if errors.Is(err, ErrInvalidPointCommand) || errors.Is(err, ErrUnknownPointSource) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": result.Lot, "idempotent": result.Idempotent})
}

func requestIDFromPointMutation(r *http.Request, fallback string) string {
	return firstNonEmptyString(strings.TrimSpace(r.Header.Get("X-Request-ID")), strings.TrimSpace(r.Header.Get("Idempotency-Key")), fallback)
}

type personalPointServiceStore interface {
	PersonalPointService() *PersonalPointService
}

func personalPointServiceForStore(store platformStore) (*PersonalPointService, error) {
	provider, ok := store.(personalPointServiceStore)
	if !ok || provider.PersonalPointService() == nil {
		return nil, ErrInvalidPointCommand
	}
	return provider.PersonalPointService(), nil
}

type personalPointPolicyRequest struct {
	Revision      int64  `json:"revision"`
	Enabled       bool   `json:"enabled"`
	DurationValue int    `json:"durationValue"`
	ChangeReason  string `json:"changeReason"`
}

func (a adminAPI) pointExpiryPolicy(w http.ResponseWriter, r *http.Request) {
	service, err := personalPointServiceForStore(a.store)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	if r.Method == http.MethodGet {
		policy, err := service.CurrentPolicy(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]any{"item": policy})
		return
	}
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request personalPointPolicyRequest
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, _ := actorFromRequest(r)
	policy, err := service.PublishPolicy(r.Context(), PersonalPointPolicyPublishCommand{
		ExpectedRevision: request.Revision,
		Enabled:          request.Enabled,
		DurationValue:    request.DurationValue,
		ChangeReason:     request.ChangeReason,
		ActorID:          actorID,
	})
	if errors.Is(err, ErrPointPolicyRevisionConflict) {
		writeError(w, http.StatusConflict, err)
		return
	}
	if errors.Is(err, ErrInvalidPointCommand) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": policy})
}

func (a adminAPI) customerPointLots(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, errors.New("user id is required"))
		return
	}
	account, err := a.store.PointAccount(userID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	service, err := personalPointServiceForStore(a.store)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	lots, err := service.ListLots(r.Context(), account.ID, userID, PersonalPointLotFilter{
		Source: PointSource(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("source")))),
		Status: strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status"))),
		Limit:  limit, Offset: offset,
	})
	if errors.Is(err, ErrPointOwnership) {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if errors.Is(err, ErrInvalidPointCommand) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": lots})
}
