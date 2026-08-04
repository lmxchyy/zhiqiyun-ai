package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

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
