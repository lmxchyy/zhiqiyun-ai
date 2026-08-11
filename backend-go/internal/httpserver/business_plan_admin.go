package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var (
	businessPlanCodeFormat      = regexp.MustCompile(`^[a-z][a-z0-9_]{1,62}[a-z0-9]$`)
	businessPlanPriceWord       = regexp.MustCompile(`(^|_)(price|rmb|amount|yuan)(_|$)|(^|_)[0-9]+yuan(_|$)`)
	businessPlanPricedIdentity  = regexp.MustCompile(`^(plan_)?(member|agent)_[1-9][0-9]{1,5}$`)
	errBusinessPlanCodeFormat   = errors.New("business plan code must use 3-64 lowercase letters, digits, or underscores")
	errBusinessPlanCodeHasPrice = errors.New("business plan code contains explicit price semantics")
)

type businessPlanDomainService struct{}

func (businessPlanDomainService) ValidateNewCode(code string) error {
	code = strings.TrimSpace(code)
	if !businessPlanCodeFormat.MatchString(code) {
		return errBusinessPlanCodeFormat
	}
	if businessPlanPriceWord.MatchString(code) || businessPlanPricedIdentity.MatchString(code) {
		return errBusinessPlanCodeHasPrice
	}
	return nil
}

type businessPlanAdminError struct {
	status  int
	code    string
	message string
}

func (e *businessPlanAdminError) Error() string        { return e.message }
func (e *businessPlanAdminError) BusinessCode() string { return e.code }

func newBusinessPlanAdminError(status int, code, message string) error {
	return &businessPlanAdminError{status: status, code: code, message: message}
}

func writeBusinessPlanAdminError(w http.ResponseWriter, err error) {
	var businessErr *businessPlanAdminError
	if errors.As(err, &businessErr) {
		writeError(w, businessErr.status, businessErr)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}

type businessPlanAdminView struct {
	ID              string `json:"id"`
	Code            string `json:"code"`
	Name            string `json:"name"`
	BusinessType    string `json:"businessType"`
	LegacyCode      bool   `json:"legacyCode"`
	CodeReadOnly    bool   `json:"codeReadOnly"`
	Active          bool   `json:"active"`
	ActiveVersionID string `json:"activeVersionId,omitempty"`
}

type businessPlanVersionView struct {
	ID                    string         `json:"id"`
	PlanID                string         `json:"planId"`
	VersionNo             int            `json:"versionNo"`
	BusinessType          string         `json:"businessType"`
	RightsSnapshot        map[string]any `json:"rightsSnapshot"`
	MemberLevel           string         `json:"memberLevel,omitempty"`
	AgentLevel            string         `json:"agentLevel,omitempty"`
	TokenAmount           int64          `json:"tokenAmount"`
	PointsAmount          int64          `json:"pointsAmount"`
	DurationDays          int            `json:"durationDays"`
	CommissionRuleVersion string         `json:"commissionRuleVersion"`
	CommissionSnapshot    map[string]any `json:"commissionSnapshot"`
	Status                string         `json:"status"`
	Revision              int64          `json:"revision"`
	EffectiveAt           *time.Time     `json:"effectiveAt,omitempty"`
	ExpiresAt             *time.Time     `json:"expiresAt,omitempty"`
	CreatedBy             string         `json:"createdBy,omitempty"`
	UpdatedBy             string         `json:"updatedBy,omitempty"`
	ActivatedBy           string         `json:"activatedBy,omitempty"`
	ActivatedAt           *time.Time     `json:"activatedAt,omitempty"`
	RetiredBy             string         `json:"retiredBy,omitempty"`
	RetiredAt             *time.Time     `json:"retiredAt,omitempty"`
	ChangeReason          string         `json:"changeReason,omitempty"`
	CreatedAt             time.Time      `json:"createdAt"`
	UpdatedAt             time.Time      `json:"updatedAt"`
}

type businessPlanVersionMutation struct {
	Revision              int64          `json:"revision"`
	MemberLevel           *string        `json:"memberLevel"`
	AgentLevel            *string        `json:"agentLevel"`
	TokenAmount           *int64         `json:"tokenAmount"`
	PointsAmount          *int64         `json:"pointsAmount"`
	DurationDays          *int           `json:"durationDays"`
	RightsSnapshot        map[string]any `json:"rightsSnapshot"`
	CommissionRuleVersion *string        `json:"commissionRuleVersion"`
	CommissionSnapshot    map[string]any `json:"commissionSnapshot"`
	EffectiveAt           *time.Time     `json:"effectiveAt"`
	ExpiresAt             *time.Time     `json:"expiresAt"`
	Reason                string         `json:"reason"`
}

type businessPlanVersionTransition struct {
	Revision int64  `json:"revision"`
	Reason   string `json:"reason"`
}

type businessPlanAdminAPI struct {
	store *postgresStore
}

func newBusinessPlanAdminAPI(store platformStore) businessPlanAdminAPI {
	postgres, _ := store.(*postgresStore)
	return businessPlanAdminAPI{store: postgres}
}

func (a businessPlanAdminAPI) requireStore(w http.ResponseWriter) bool {
	if a.store != nil && a.store.db != nil {
		return true
	}
	writeError(w, http.StatusServiceUnavailable, errors.New("business plan administration requires PostgreSQL"))
	return false
}

func (a businessPlanAdminAPI) plans(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	items, err := a.store.listBusinessPlans(r.Context())
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": len(items)})
}

func (a businessPlanAdminAPI) plan(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	item, err := a.store.businessPlan(r.Context(), r.PathValue("planId"))
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a businessPlanAdminAPI) versions(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	items, err := a.store.listBusinessPlanVersions(r.Context(), r.PathValue("planId"))
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": len(items)})
}

func (a businessPlanAdminAPI) createVersion(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	var mutation businessPlanVersionMutation
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&mutation); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := a.store.createBusinessPlanVersion(r.Context(), r.PathValue("planId"), mutation, actorID, actorRole)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"item": item})
}

func (a businessPlanAdminAPI) updateVersion(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	var mutation businessPlanVersionMutation
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&mutation); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := a.store.updateBusinessPlanVersion(r.Context(), r.PathValue("versionId"), mutation, actorID, actorRole)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a businessPlanAdminAPI) activateVersion(w http.ResponseWriter, r *http.Request) {
	a.transitionVersion(w, r, "ACTIVE")
}

func (a businessPlanAdminAPI) retireVersion(w http.ResponseWriter, r *http.Request) {
	a.transitionVersion(w, r, "RETIRED")
}

func (a businessPlanAdminAPI) transitionVersion(w http.ResponseWriter, r *http.Request, targetStatus string) {
	if !a.requireStore(w) {
		return
	}
	var transition businessPlanVersionTransition
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&transition); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := a.store.transitionBusinessPlanVersion(r.Context(), r.PathValue("versionId"), targetStatus, transition, actorID, actorRole)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func isLegacyBusinessPlanCode(id, code string) bool {
	for _, value := range []string{id, code} {
		switch strings.TrimSpace(value) {
		case "plan_ai_creator_996", "plan_agent_join_996", "ai_creator_996", "agent_join_996":
			return true
		}
	}
	return false
}

func managedPlanRequiresVersionError() error {
	return newBusinessPlanAdminError(http.StatusConflict, "MANAGED_PLAN_REQUIRES_VERSION", "V2 managed plan must be changed through an entitlement version")
}

func validateVersionMutationReason(reason string) error {
	if strings.TrimSpace(reason) == "" {
		return newBusinessPlanAdminError(http.StatusBadRequest, "REASON_REQUIRED", "reason is required")
	}
	return nil
}

func validateVersionRights(item businessPlanVersionView) error {
	if item.TokenAmount < 0 || item.PointsAmount < 0 || item.DurationDays < 0 {
		return newBusinessPlanAdminError(http.StatusBadRequest, "INVALID_PLAN_VERSION", "entitlement amounts cannot be negative")
	}
	switch item.BusinessType {
	case "MEMBER":
		if strings.TrimSpace(item.MemberLevel) == "" {
			return newBusinessPlanAdminError(http.StatusBadRequest, "INVALID_PLAN_VERSION", "memberLevel is required")
		}
	case "AGENT":
		if strings.TrimSpace(item.AgentLevel) == "" {
			return newBusinessPlanAdminError(http.StatusBadRequest, "INVALID_PLAN_VERSION", "agentLevel is required")
		}
	default:
		return newBusinessPlanAdminError(http.StatusNotFound, "BUSINESS_PLAN_NOT_FOUND", fmt.Sprintf("unsupported business type %q", item.BusinessType))
	}
	if item.ExpiresAt != nil && item.EffectiveAt != nil && !item.ExpiresAt.After(*item.EffectiveAt) {
		return newBusinessPlanAdminError(http.StatusBadRequest, "INVALID_PLAN_VERSION", "expiresAt must be after effectiveAt")
	}
	return nil
}

func canonicalVersionRights(item businessPlanVersionView) map[string]any {
	rights := map[string]any{}
	for key, value := range item.RightsSnapshot {
		rights[key] = value
	}
	rights["memberLevel"] = item.MemberLevel
	rights["agentLevel"] = item.AgentLevel
	rights["tokenAmount"] = item.TokenAmount
	rights["pointsAmount"] = item.PointsAmount
	rights["durationDays"] = item.DurationDays
	return rights
}
