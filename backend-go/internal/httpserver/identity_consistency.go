package httpserver

import (
	"errors"
	"net/http"
	"strings"
)

type identityConsistencyIssue struct {
	Code            string         `json:"code"`
	Severity        string         `json:"severity"`
	UserID          string         `json:"userId"`
	EntityID        string         `json:"entityId,omitempty"`
	Message         string         `json:"message"`
	SuggestedAction string         `json:"suggestedAction"`
	Details         map[string]any `json:"details,omitempty"`
}

type identityConsistencyFilter struct{ Code, Severity, UserID string }

type adminIdentityConsistencyStore interface {
	ListAdminIdentityConsistency(actorID, actorRole string, filter identityConsistencyFilter) ([]identityConsistencyIssue, error)
}

type identityConsistencyAPI struct{ store platformStore }

func (a identityConsistencyAPI) list(w http.ResponseWriter, r *http.Request) {
	store, ok := a.store.(adminIdentityConsistencyStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("identity consistency store is unavailable"))
		return
	}
	actorID, actorRole := actorFromRequest(r)
	items, err := store.ListAdminIdentityConsistency(actorID, actorRole, identityConsistencyFilter{Code: strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code"))), Severity: strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("severity"))), UserID: strings.TrimSpace(r.URL.Query().Get("userId"))})
	if err != nil {
		writeIdentityChangeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "summary": identityConsistencySummary(items)})
}

func identityConsistencySummary(items []identityConsistencyIssue) map[string]int {
	result := map[string]int{"total": len(items), "critical": 0, "high": 0, "medium": 0}
	for _, item := range items {
		result[strings.ToLower(item.Severity)]++
	}
	return result
}
