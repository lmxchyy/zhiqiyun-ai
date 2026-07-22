package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

var (
	errIdentityDowngradeInvalid    = errors.New("invalid identity downgrade request")
	errIdentityDowngradeBlocked    = errors.New("identity downgrade is blocked")
	errIdentityDowngradeNotFound   = errors.New("identity downgrade preview not found")
	errIdentityDowngradeExpired    = errors.New("identity downgrade preview expired")
	errIdentityDowngradePermission = errors.New("identity downgrade requires super administrator")
)

type identityDowngradeRequest struct {
	TargetIdentity          string `json:"targetIdentity,omitempty"`
	ChildStrategy           string `json:"childStrategy"`
	TargetAgentID           string `json:"targetAgentId,omitempty"`
	TargetOperationCenterID string `json:"targetOperationCenterId,omitempty"`
	WaitForSettlement       bool   `json:"waitForSettlement"`
	EffectiveAt             string `json:"effectiveAt,omitempty"`
	Reason                  string `json:"reason"`
	Remark                  string `json:"remark,omitempty"`
}

type identityDowngradeCheck struct {
	Code        string `json:"code"`
	Label       string `json:"label"`
	Count       int64  `json:"count"`
	AmountCents int64  `json:"amountCents"`
	Blocking    bool   `json:"blocking"`
}

type identityDowngradePreview struct {
	PreviewToken       string                   `json:"previewToken,omitempty"`
	PreviewID          string                   `json:"previewId"`
	UserID             string                   `json:"userId"`
	CurrentIdentity    string                   `json:"currentIdentity"`
	TargetIdentity     string                   `json:"targetIdentity,omitempty"`
	ChildStrategy      string                   `json:"childStrategy"`
	EffectiveAt        string                   `json:"effectiveAt"`
	WaitForSettlement  bool                     `json:"waitForSettlement"`
	Checks             []identityDowngradeCheck `json:"checks"`
	DownlineMembers    int64                    `json:"downlineMembers"`
	DownlineAgents     int64                    `json:"downlineAgents"`
	MigrationCount     int64                    `json:"migrationCount"`
	UnassignedCount    int64                    `json:"unassignedCount"`
	CommissionImpact   string                   `json:"commissionImpact"`
	RelationshipBefore map[string]any           `json:"relationshipBefore"`
	RelationshipAfter  map[string]any           `json:"relationshipAfter"`
	Blockers           []string                 `json:"blockers"`
	RiskWarnings       []string                 `json:"riskWarnings"`
	Status             string                   `json:"status"`
	ExpiresAt          string                   `json:"expiresAt"`
}

type identityDowngradeConfirmRequest struct {
	PreviewToken      string `json:"previewToken"`
	HighRiskConfirmed bool   `json:"highRiskConfirmed"`
	ConfirmationText  string `json:"confirmationText,omitempty"`
}

type identityDowngradeRescheduleRequest struct {
	EffectiveAt string `json:"effectiveAt"`
	Reason      string `json:"reason"`
}

type identityDowngradeResult struct {
	RequestID             string   `json:"requestId"`
	UserID                string   `json:"userId"`
	Status                string   `json:"status"`
	EffectiveAt           string   `json:"effectiveAt"`
	MigratedMembers       int64    `json:"migratedMembers"`
	MigratedAgents        int64    `json:"migratedAgents"`
	MigratedRelationships int64    `json:"migratedRelationships"`
	Idempotent            bool     `json:"idempotent"`
	Blockers              []string `json:"blockers,omitempty"`
	FailureMessage        string   `json:"failureMessage,omitempty"`
	LastCheckedAt         string   `json:"lastCheckedAt,omitempty"`
	CreatedAt             string   `json:"createdAt,omitempty"`
	TimeoutWarning        bool     `json:"timeoutWarning"`
}

type adminIdentityDowngradeStore interface {
	PreviewAdminIdentityDowngrade(actorID, actorRole, userID string, request identityDowngradeRequest) (identityDowngradePreview, error)
	ConfirmAdminIdentityDowngrade(actorID, actorRole, userID string, request identityDowngradeConfirmRequest) (identityDowngradeResult, error)
	ListAdminIdentityDowngrades(actorID, actorRole, userID string) ([]identityDowngradeResult, error)
	RecheckAdminIdentityDowngrade(actorID, actorRole, userID, requestID string) (identityDowngradeResult, error)
	CancelAdminIdentityDowngrade(actorID, actorRole, userID, requestID string) (identityDowngradeResult, error)
	RescheduleAdminIdentityDowngrade(actorID, actorRole, userID, requestID string, request identityDowngradeRescheduleRequest) (identityDowngradeResult, error)
}

type identityDowngradeAPI struct{ commands *identityCommandService }

func newIdentityDowngradeAPI(store platformStore) identityDowngradeAPI {
	return identityDowngradeAPI{commands: newIdentityCommandService(store)}
}

func (a identityDowngradeAPI) preview(w http.ResponseWriter, r *http.Request) {
	var request identityDowngradeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	result, err := a.commands.PreviewDowngrade(actorID, actorRole, strings.TrimSpace(r.PathValue("id")), request)
	if err != nil {
		writeIdentityDowngradeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": result})
}

func (a identityDowngradeAPI) confirm(w http.ResponseWriter, r *http.Request) {
	var request identityDowngradeConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	result, err := a.commands.ConfirmDowngrade(actorID, actorRole, strings.TrimSpace(r.PathValue("id")), request)
	if err != nil {
		writeIdentityDowngradeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": result})
}

func (a identityDowngradeAPI) list(w http.ResponseWriter, r *http.Request) {
	actorID, actorRole := actorFromRequest(r)
	items, err := a.commands.ListDowngrades(actorID, actorRole, strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeIdentityDowngradeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a identityDowngradeAPI) recheck(w http.ResponseWriter, r *http.Request) {
	actorID, actorRole := actorFromRequest(r)
	item, err := a.commands.RecheckDowngrade(actorID, actorRole, strings.TrimSpace(r.PathValue("id")), strings.TrimSpace(r.PathValue("requestId")))
	if err != nil {
		writeIdentityDowngradeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a identityDowngradeAPI) cancel(w http.ResponseWriter, r *http.Request) {
	actorID, actorRole := actorFromRequest(r)
	item, err := a.commands.CancelDowngrade(actorID, actorRole, strings.TrimSpace(r.PathValue("id")), strings.TrimSpace(r.PathValue("requestId")))
	if err != nil {
		writeIdentityDowngradeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a identityDowngradeAPI) reschedule(w http.ResponseWriter, r *http.Request) {
	var request identityDowngradeRescheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := a.commands.RescheduleDowngrade(actorID, actorRole, strings.TrimSpace(r.PathValue("id")), strings.TrimSpace(r.PathValue("requestId")), request)
	if err != nil {
		writeIdentityDowngradeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func writeIdentityDowngradeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errIdentityDowngradePermission):
		writeError(w, http.StatusForbidden, err)
	case errors.Is(err, errIdentityDowngradeNotFound), errors.Is(err, errIdentityUserNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, errIdentityDowngradeBlocked), errors.Is(err, errIdentityDowngradeExpired):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, errIdentityDowngradeInvalid):
		writeError(w, http.StatusBadRequest, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
