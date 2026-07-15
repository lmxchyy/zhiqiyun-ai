package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

var (
	errEnterpriseNotFound = errors.New("enterprise resource not found")
	errEnterpriseConflict = errors.New("enterprise resource conflict")
	errEnterpriseInvalid  = errors.New("invalid enterprise request")
)

type enterpriseAPI struct {
	store    enterpriseStore
	sessions authSessionStore
}

func newEnterpriseAPI(store platformStore, sessions authSessionStore) enterpriseAPI {
	repository, _ := store.(enterpriseStore)
	return enterpriseAPI{store: repository, sessions: sessions}
}

func (a enterpriseAPI) userID(w http.ResponseWriter, r *http.Request) (string, bool) {
	if a.store == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("enterprise center store is unavailable"))
		return "", false
	}
	userID, err := authenticatedUserID(r, a.sessions)
	if err != nil {
		writeEnterpriseError(w, err)
		return "", false
	}
	return userID, true
}

func (a enterpriseAPI) require(w http.ResponseWriter, r *http.Request, permission string) (enterpriseAccess, bool) {
	userID, ok := a.userID(w, r)
	if !ok {
		return enterpriseAccess{}, false
	}
	access, err := a.store.EnterpriseAccess(userID, permission)
	if err != nil {
		writeEnterpriseError(w, err)
		return enterpriseAccess{}, false
	}
	return access, true
}

func (a enterpriseAPI) contexts(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.userID(w, r)
	if !ok {
		return
	}
	items, err := a.store.EnterpriseContexts(userID)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	current := enterpriseContext{}
	for _, item := range items {
		if item.Current {
			current = item
			break
		}
	}
	writeJSON(w, map[string]any{"contexts": items, "current": current})
}

func (a enterpriseAPI) switchContext(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.userID(w, r)
	if !ok {
		return
	}
	var request enterpriseContextSwitchRequest
	if !decodeEnterpriseJSON(w, r, &request) {
		return
	}
	item, err := a.store.SetEnterpriseCurrentContext(userID, request)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	writeJSON(w, item)
}

func (a enterpriseAPI) createEnterprise(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.userID(w, r)
	if !ok {
		return
	}
	var request enterpriseCreateRequest
	if !decodeEnterpriseJSON(w, r, &request) {
		return
	}
	result, err := a.store.CreateEnterprise(userID, request)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, result)
}

func (a enterpriseAPI) overview(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.overview.read")
	if !ok {
		return
	}
	item, err := a.store.EnterpriseOverview(access)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	writeJSON(w, item)
}

func (a enterpriseAPI) members(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.member.read")
	if !ok {
		return
	}
	items, err := a.store.ListEnterpriseMembers(access)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": len(items)})
}

func (a enterpriseAPI) member(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.member.read")
	if !ok {
		return
	}
	item, err := a.store.GetEnterpriseMember(access, r.PathValue("id"))
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	writeJSON(w, item)
}

func (a enterpriseAPI) createInvitation(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.member.invite")
	if !ok {
		return
	}
	var request enterpriseInvitationCreateRequest
	if !decodeEnterpriseJSON(w, r, &request) {
		return
	}
	if request.DefaultRole != "" && request.DefaultRole != roleEnterpriseMember && !access.hasPermission("enterprise.role.assign") {
		writeEnterpriseError(w, errForbidden)
		return
	}
	item, err := a.store.CreateEnterpriseInvitation(access, request)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, item)
}

func (a enterpriseAPI) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.userID(w, r)
	if !ok {
		return
	}
	var request enterpriseInvitationAcceptRequest
	if !decodeEnterpriseJSON(w, r, &request) {
		return
	}
	item, err := a.store.AcceptEnterpriseInvitation(userID, request)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	writeJSON(w, item)
}

func (a enterpriseAPI) createJoinRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.userID(w, r)
	if !ok {
		return
	}
	var request enterpriseJoinRequestCreate
	if !decodeEnterpriseJSON(w, r, &request) {
		return
	}
	item, err := a.store.CreateEnterpriseJoinRequest(userID, request)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, item)
}

func (a enterpriseAPI) joinRequests(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.member.read")
	if !ok {
		return
	}
	items, err := a.store.ListEnterpriseJoinRequests(access)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": len(items)})
}

func (a enterpriseAPI) approveJoinRequest(w http.ResponseWriter, r *http.Request) {
	a.reviewJoinRequest(w, r, true)
}

func (a enterpriseAPI) rejectJoinRequest(w http.ResponseWriter, r *http.Request) {
	a.reviewJoinRequest(w, r, false)
}

func (a enterpriseAPI) reviewJoinRequest(w http.ResponseWriter, r *http.Request, approved bool) {
	access, ok := a.require(w, r, "enterprise.member.update")
	if !ok {
		return
	}
	var request struct {
		Comment string `json:"comment"`
	}
	if r.ContentLength > 0 && !decodeEnterpriseJSON(w, r, &request) {
		return
	}
	item, err := a.store.ReviewEnterpriseJoinRequest(access, r.PathValue("id"), approved, request.Comment)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	writeJSON(w, item)
}

func (a enterpriseAPI) updateMember(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.member.update")
	if !ok {
		return
	}
	var request enterpriseMemberUpdateRequest
	if !decodeEnterpriseJSON(w, r, &request) {
		return
	}
	if len(request.Roles) > 0 && !access.hasPermission("enterprise.role.assign") {
		writeEnterpriseError(w, errForbidden)
		return
	}
	item, err := a.store.UpdateEnterpriseMember(access, r.PathValue("id"), request)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	writeJSON(w, item)
}

func (a enterpriseAPI) disableMember(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.member.disable")
	if !ok {
		return
	}
	item, err := a.store.DisableEnterpriseMember(access, r.PathValue("id"))
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	writeJSON(w, item)
}

func (a enterpriseAPI) removeMember(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.member.remove")
	if !ok {
		return
	}
	if err := a.store.RemoveEnterpriseMember(access, r.PathValue("id")); err != nil {
		writeEnterpriseError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a enterpriseAPI) organizationTree(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.organization.read")
	if !ok {
		return
	}
	items, err := a.store.EnterpriseOrganizationTree(access)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a enterpriseAPI) createOrganization(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.organization.create")
	if !ok {
		return
	}
	var request enterpriseOrganizationCreateRequest
	if !decodeEnterpriseJSON(w, r, &request) {
		return
	}
	item, err := a.store.CreateEnterpriseOrganization(access, request)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, item)
}

func (a enterpriseAPI) updateOrganization(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.organization.update")
	if !ok {
		return
	}
	var request enterpriseOrganizationUpdateRequest
	if !decodeEnterpriseJSON(w, r, &request) {
		return
	}
	item, err := a.store.UpdateEnterpriseOrganization(access, r.PathValue("id"), request)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	writeJSON(w, item)
}

func (a enterpriseAPI) moveOrganization(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.organization.update")
	if !ok {
		return
	}
	var request enterpriseOrganizationMoveRequest
	if !decodeEnterpriseJSON(w, r, &request) {
		return
	}
	item, err := a.store.MoveEnterpriseOrganization(access, r.PathValue("id"), request)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	writeJSON(w, item)
}

func (a enterpriseAPI) deleteOrganization(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.organization.delete")
	if !ok {
		return
	}
	if err := a.store.DeleteEnterpriseOrganization(access, r.PathValue("id")); err != nil {
		writeEnterpriseError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a enterpriseAPI) auditLogs(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.audit.read")
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := a.store.EnterpriseAuditLogs(access, limit)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": len(items)})
}

func (a enterpriseAPI) submitCertification(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.certification.submit")
	if !ok {
		return
	}
	var request enterpriseCertificationSubmitRequest
	if !decodeEnterpriseJSON(w, r, &request) {
		return
	}
	item, err := a.store.SubmitEnterpriseCertification(access, request)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, item)
}

func (a enterpriseAPI) billingSummary(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.billing.read")
	if !ok {
		return
	}
	item, err := a.store.EnterpriseOverview(access)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	writeJSON(w, map[string]any{"wallet": item.Wallet, "subscription": item.Subscription})
}

func (a enterpriseAPI) computeAccount(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.compute.ledger.read")
	if !ok {
		return
	}
	store, ok := a.store.(enterpriseComputeStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errors.New("enterprise compute store is unavailable"))
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	wallet, entries, err := store.EnterpriseComputeAccount(access, limit)
	if err != nil {
		writeEnterpriseError(w, err)
		return
	}
	writeJSON(w, map[string]any{"wallet": wallet, "entries": entries, "total": len(entries), "unit": "COMPUTE_UNIT", "currencyUnit": "CNY_CENT"})
}

func (a enterpriseAPI) roles(w http.ResponseWriter, r *http.Request) {
	access, ok := a.require(w, r, "enterprise.role.read")
	if !ok {
		return
	}
	items := make([]map[string]any, 0, 5)
	for _, role := range []string{roleEnterpriseAdmin, roleAIAdmin, roleFinance, roleCustomerService, roleEnterpriseMember} {
		items = append(items, map[string]any{
			"code": role, "permissions": permissionsForCurrentRole(role), "assignable": access.hasPermission("enterprise.role.assign"),
		})
	}
	writeJSON(w, map[string]any{"items": items})
}

func decodeEnterpriseJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func writeEnterpriseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errUnauthorized):
		writeError(w, http.StatusUnauthorized, errUnauthorized)
	case errors.Is(err, errForbidden):
		writeError(w, http.StatusForbidden, errForbidden)
	case errors.Is(err, errEnterpriseServiceUnavailable):
		writeError(w, http.StatusForbidden, err)
	case errors.Is(err, errEnterpriseNotFound):
		writeError(w, http.StatusNotFound, errEnterpriseNotFound)
	case errors.Is(err, errEnterpriseConflict):
		writeError(w, http.StatusConflict, errEnterpriseConflict)
	case errors.Is(err, errEnterpriseInvalid), strings.Contains(strings.ToLower(err.Error()), "invalid"):
		writeError(w, http.StatusBadRequest, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
