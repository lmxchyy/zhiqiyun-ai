package httpserver

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

var errIdentityUserNotFound = errors.New("user identity profile not found")

type adminBusinessIdentity struct {
	ID                string `json:"id"`
	TenantID          string `json:"tenantId"`
	UserID            string `json:"userId"`
	IdentityType      string `json:"identityType"`
	IdentityStatus    string `json:"identityStatus"`
	CommissionEnabled bool   `json:"commissionEnabled"`
	SourceType        string `json:"sourceType"`
	SourceOrderID     string `json:"sourceOrderId,omitempty"`
	EffectiveAt       string `json:"effectiveAt"`
	ExpiresAt         string `json:"expiresAt,omitempty"`
	EndedAt           string `json:"endedAt,omitempty"`
	StatusReason      string `json:"statusReason,omitempty"`
	IdentityVersion   int64  `json:"identityVersion"`
	CreatedBy         string `json:"createdBy"`
	CreatedAt         string `json:"createdAt"`
	UpdatedAt         string `json:"updatedAt"`
}

type adminUserRelationship struct {
	ID                  string `json:"id"`
	TenantID            string `json:"tenantId"`
	UserID              string `json:"userId"`
	ParentAgentID       string `json:"parentAgentId,omitempty"`
	ParentAgentUserID   string `json:"parentAgentUserId,omitempty"`
	ParentAgentName     string `json:"parentAgentName,omitempty"`
	OperationCenterID   string `json:"operationCenterId,omitempty"`
	OperationCenterName string `json:"operationCenterName,omitempty"`
	EffectiveAt         string `json:"effectiveAt"`
	EndedAt             string `json:"endedAt,omitempty"`
	Status              string `json:"status"`
	SourceType          string `json:"sourceType"`
	CreatedBy           string `json:"createdBy"`
	CreatedAt           string `json:"createdAt"`
	UpdatedAt           string `json:"updatedAt"`
}

type adminIdentityChangeRecord struct {
	ID                   string         `json:"id"`
	TenantID             string         `json:"tenantId"`
	UserID               string         `json:"userId"`
	OldIdentity          map[string]any `json:"oldIdentity,omitempty"`
	NewIdentity          map[string]any `json:"newIdentity,omitempty"`
	ChangeType           string         `json:"changeType"`
	SourceType           string         `json:"sourceType"`
	SourceOrderID        string         `json:"sourceOrderId,omitempty"`
	OldParentAgentID     string         `json:"oldParentAgentId,omitempty"`
	NewParentAgentID     string         `json:"newParentAgentId,omitempty"`
	OldOperationCenterID string         `json:"oldOperationCenterId,omitempty"`
	NewOperationCenterID string         `json:"newOperationCenterId,omitempty"`
	Reason               string         `json:"reason,omitempty"`
	Remark               string         `json:"remark,omitempty"`
	OperatorID           string         `json:"operatorId"`
	RequestID            string         `json:"requestId,omitempty"`
	CreatedAt            string         `json:"createdAt"`
}

type adminIdentityProfile struct {
	UserID          string                  `json:"userId"`
	AccountStatus   string                  `json:"accountStatus"`
	LegacyRole      string                  `json:"legacyRole"`
	AccountRoles    []string                `json:"accountRoles"`
	PrimaryIdentity string                  `json:"primaryIdentity"`
	Identities      []adminBusinessIdentity `json:"identities"`
}

type adminIdentityHistory struct {
	UserID        string                      `json:"userId"`
	Identities    []adminBusinessIdentity     `json:"identities"`
	ChangeRecords []adminIdentityChangeRecord `json:"changeRecords"`
}

type adminIdentityFinancialOverview struct {
	UserID     string         `json:"userId"`
	Membership map[string]any `json:"membership"`
	Wallet     map[string]any `json:"wallet"`
	Token      map[string]any `json:"token"`
	Commission map[string]any `json:"commission"`
}

type adminIdentityQueryStore interface {
	GetAdminIdentityProfile(userID string) (adminIdentityProfile, error)
	GetAdminIdentityHistory(userID string) (adminIdentityHistory, error)
	GetAdminCurrentRelationship(userID string) (*adminUserRelationship, error)
	GetAdminRelationshipHistory(userID string) ([]adminUserRelationship, error)
	GetAdminIdentityFinancialOverview(userID string) (adminIdentityFinancialOverview, error)
}

type identityQueryAPI struct {
	store platformStore
}

func newIdentityQueryAPI(store platformStore) identityQueryAPI {
	return identityQueryAPI{store: store}
}

func (a identityQueryAPI) profile(w http.ResponseWriter, r *http.Request) {
	store, ok := a.store.(adminIdentityQueryStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("identity query store is unavailable"))
		return
	}
	item, err := store.GetAdminIdentityProfile(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeIdentityQueryError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a identityQueryAPI) history(w http.ResponseWriter, r *http.Request) {
	store, ok := a.store.(adminIdentityQueryStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("identity query store is unavailable"))
		return
	}
	item, err := store.GetAdminIdentityHistory(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeIdentityQueryError(w, err)
		return
	}
	writeJSON(w, item)
}

func (a identityQueryAPI) currentRelationship(w http.ResponseWriter, r *http.Request) {
	store, ok := a.store.(adminIdentityQueryStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("identity query store is unavailable"))
		return
	}
	item, err := store.GetAdminCurrentRelationship(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeIdentityQueryError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a identityQueryAPI) relationshipHistory(w http.ResponseWriter, r *http.Request) {
	store, ok := a.store.(adminIdentityQueryStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("identity query store is unavailable"))
		return
	}
	items, err := store.GetAdminRelationshipHistory(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeIdentityQueryError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a identityQueryAPI) financialOverview(w http.ResponseWriter, r *http.Request) {
	store, ok := a.store.(adminIdentityQueryStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("identity query store is unavailable"))
		return
	}
	item, err := store.GetAdminIdentityFinancialOverview(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeIdentityQueryError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func writeIdentityQueryError(w http.ResponseWriter, err error) {
	if errors.Is(err, errIdentityUserNotFound) || errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errIdentityUserNotFound)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}

func decodeIdentitySnapshot(raw []byte) map[string]any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return value
}

func primaryBusinessIdentity(items []adminBusinessIdentity) string {
	primary := roleUser
	for _, item := range items {
		if item.IdentityStatus == "TERMINATED" {
			continue
		}
		switch item.IdentityType {
		case "OPERATION_CENTER":
			return "OPERATION_CENTER"
		case "AGENT":
			primary = "AGENT"
		}
	}
	return primary
}
