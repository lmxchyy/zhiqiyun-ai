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
	Points         int64                        `json:"points"`
	Reason         string                       `json:"reason"`
	IdempotencyKey string                       `json:"idempotencyKey"`
	ValidityDays   int                          `json:"validityDays,omitempty"`
	Membership     *adminMembershipGrantRequest `json:"membership,omitempty"`
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
	if request.Membership != nil {
		request.Membership.Reason = strings.TrimSpace(request.Membership.Reason)
		request.Membership.IdempotencyKey = strings.TrimSpace(request.Membership.IdempotencyKey)
		request.Membership.PlanID = strings.TrimSpace(request.Membership.PlanID)
		if request.Reason == "" {
			request.Reason = request.Membership.Reason
		}
		if request.IdempotencyKey == "" {
			request.IdempotencyKey = request.Membership.IdempotencyKey
		}
	}
	if request.Reason == "" || request.IdempotencyKey == "" {
		return request, ErrInvalidPointCommand
	}
	if request.ValidityDays < 0 || request.ValidityDays > adminManualMaxValidityDays {
		return request, ErrInvalidPointCommand
	}
	if request.Points == 0 && request.Membership == nil {
		return request, ErrInvalidPointCommand
	}
	return request, nil
}

func (a adminAPI) customerPointGift(w http.ResponseWriter, r *http.Request) {
	request, err := decodeAdminPointMutation(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if request.Membership != nil {
		if request.Points != 0 || request.ValidityDays != 0 {
			writeError(w, http.StatusBadRequest, errors.New("membership grant and point gift must be submitted separately"))
			return
		}
		membership := *request.Membership
		if membership.Reason == "" {
			membership.Reason = request.Reason
		}
		if membership.IdempotencyKey == "" {
			membership.IdempotencyKey = request.IdempotencyKey
		}
		actorID, actorRole := actorFromRequest(r)
		result, grantErr := a.grantManualMembership(r.Context(), actorID, actorRole, strings.TrimSpace(r.PathValue("id")), membership)
		if errors.Is(grantErr, ErrIdempotencyConflict) {
			writeError(w, http.StatusConflict, grantErr)
			return
		}
		if errors.Is(grantErr, errIdentityPermission) {
			writeError(w, http.StatusForbidden, grantErr)
			return
		}
		if errors.Is(grantErr, ErrInvalidPointCommand) {
			writeError(w, http.StatusBadRequest, grantErr)
			return
		}
		if errors.Is(grantErr, ErrPointNotFound) {
			writeError(w, http.StatusNotFound, grantErr)
			return
		}
		if grantErr != nil {
			writeError(w, http.StatusInternalServerError, grantErr)
			return
		}
		writeJSON(w, map[string]any{"membership": result, "idempotent": result.Idempotent})
		return
	}
	if request.Points <= 0 {
		writeError(w, http.StatusBadRequest, ErrInvalidPointCommand)
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
	if request.Membership != nil || request.ValidityDays != 0 {
		writeError(w, http.StatusBadRequest, errors.New("membership and validityDays are only supported by point gifts"))
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
	command := PersonalPointGrantCommand{
		AccountID: account.ID, UserID: userID, Source: source, Points: request.Points,
		ReferenceType: referenceType, ReferenceID: request.IdempotencyKey, IdempotencyKey: request.IdempotencyKey, Reason: request.Reason,
		Audit: PersonalPointAudit{ActorID: actorID, ActorRole: actorRole, Action: "personal_points.admin_gift", Method: r.Method, Path: r.URL.Path, RequestID: requestIDFromPointMutation(r, request.IdempotencyKey)},
	}
	var result PersonalPointGrantResult
	if source == PointSourceAdminGift && request.ValidityDays > 0 {
		result, err = grantAdminPointGiftWithValidity(r.Context(), service, command, request.ValidityDays)
	} else {
		result, err = service.Grant(r.Context(), command)
	}
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
