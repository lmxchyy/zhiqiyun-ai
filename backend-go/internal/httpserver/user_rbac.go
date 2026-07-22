package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
)

const (
	roleUser             = "USER"
	roleAgent            = "AGENT"
	roleOperation        = "OPERATION"
	roleEnterpriseAdmin  = "ENTERPRISE_ADMIN"
	roleAIAdmin          = "AI_ADMIN"
	roleFinance          = "FINANCE"
	roleCustomerService  = "CUSTOMER_SERVICE"
	roleEnterpriseMember = "ENTERPRISE_MEMBER"
)

var roleOrder = map[string]int{
	roleUser: 0, roleAgent: 10, roleOperation: 20, roleEnterpriseAdmin: 30,
	roleAIAdmin: 40, roleFinance: 50, roleCustomerService: 60, roleEnterpriseMember: 70,
}

var rolePermissionMatrix = map[string][]string{
	roleUser: {
		"ai:use", "assets:view", "project:view", "wallet:view", "settings:view",
	},
	roleAgent: {
		"agent:promotion", "agent:promotion:create", "agent:qrcode:view", "agent:customer:view",
		"agent:commission:view", "agent:withdraw", "agent:material:view",
	},
	roleOperation: {
		"operation:dashboard", "operation:agent:list", "operation:agent:approve", "operation:order:view",
		"operation:customer:view", "operation:report:view", "operation:announcement:manage", "operation:renew",
		"agent:promotion", "agent:promotion:create", "agent:qrcode:view", "agent:customer:view",
		"agent:commission:view", "agent:withdraw", "agent:material:view",
	},
	roleEnterpriseAdmin: {
		"enterprise.ai.use", "enterprise.compute.ledger.read",
		"enterprise.overview.read", "enterprise.organization.read", "enterprise.organization.create",
		"enterprise.organization.update", "enterprise.organization.delete", "enterprise.member.read",
		"enterprise.member.invite", "enterprise.member.update", "enterprise.member.disable",
		"enterprise.member.remove", "enterprise.role.read", "enterprise.role.assign",
		"enterprise.billing.read", "enterprise.audit.read", "enterprise.settings.read",
		"enterprise.settings.update", "enterprise.certification.submit",
		"enterprise.connector.read", "enterprise.connector.manage",
	},
	roleAIAdmin: {
		"ai:admin", "enterprise.ai.use", "enterprise.overview.read", "enterprise.organization.read", "enterprise.member.read",
		"enterprise.role.read", "enterprise.settings.read", "enterprise.connector.read", "enterprise.connector.manage",
	},
	roleFinance: {
		"finance:view", "finance:approve", "finance:commission-rule:view", "finance:commission-rule:manage", "enterprise.compute.ledger.read", "enterprise.overview.read", "enterprise.member.read",
		"enterprise.billing.read", "enterprise.audit.read",
	},
	roleCustomerService: {
		"customer-service:manage", "enterprise.overview.read", "enterprise.organization.read", "enterprise.member.read",
	},
	roleEnterpriseMember: {
		"enterprise.ai.use", "enterprise.overview.read", "enterprise.organization.read", "enterprise.member.read", "enterprise.settings.read",
	},
}

type userRoleAccess struct {
	UserID         string   `json:"userId"`
	TenantID       string   `json:"tenantId"`
	OrganizationID string   `json:"organizationId"`
	Roles          []string `json:"roles"`
	CurrentRole    string   `json:"currentRole"`
	Permissions    []string `json:"permissions"`
}

type userRoleAccessStore interface {
	GetUserRoleAccess(userID string) (userRoleAccess, bool, error)
	SetUserCurrentRole(userID string, role string) (userRoleAccess, error)
}

type userRBACAPI struct {
	store        platformStore
	sessions     authSessionStore
	currentRoles sync.Map
}

func newUserRBACAPI(store platformStore, sessions authSessionStore) *userRBACAPI {
	return &userRBACAPI{store: store, sessions: sessions}
}

func (a *userRBACAPI) profile(w http.ResponseWriter, r *http.Request) {
	access, err := a.accessForRequest(r)
	if err != nil {
		writeUserRBACError(w, err)
		return
	}
	writeJSON(w, access)
}

func (a *userRBACAPI) switchCurrentRole(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	requestedRole := normalizeAppRole(payload.Role)
	if requestedRole == "" {
		writeError(w, http.StatusBadRequest, errors.New("role is required"))
		return
	}
	access, err := a.accessForRequest(r)
	if err != nil {
		writeUserRBACError(w, err)
		return
	}
	if !containsString(access.Roles, requestedRole) {
		writeError(w, http.StatusForbidden, errForbidden)
		return
	}
	if store, ok := a.store.(userRoleAccessStore); ok {
		updated, err := store.SetUserCurrentRole(access.UserID, requestedRole)
		if err != nil {
			if errors.Is(err, errForbidden) {
				writeError(w, http.StatusForbidden, errForbidden)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, normalizedUserRoleAccess(updated))
		return
	}
	a.currentRoles.Store(access.UserID, requestedRole)
	access.CurrentRole = requestedRole
	access.Permissions = permissionsForCurrentRole(requestedRole)
	writeJSON(w, access)
}

func (a *userRBACAPI) accessForRequest(r *http.Request) (userRoleAccess, error) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		return userRoleAccess{}, err
	}
	if store, ok := a.store.(userRoleAccessStore); ok {
		access, found, err := store.GetUserRoleAccess(user.ID)
		if err != nil {
			return userRoleAccess{}, err
		}
		if found {
			return normalizedUserRoleAccess(access), nil
		}
	}
	roles := rolesForUser(data, user)
	currentRole := roleUser
	if stored, ok := a.currentRoles.Load(user.ID); ok {
		candidate := normalizeAppRole(stored)
		if containsString(roles, candidate) {
			currentRole = candidate
		}
	}
	return normalizedUserRoleAccess(userRoleAccess{
		UserID:         user.ID,
		TenantID:       firstNonEmptyString(user.TenantID, "tenant_default"),
		OrganizationID: firstNonEmptyString(user.OrganizationID, "organization_default"),
		Roles:          roles,
		CurrentRole:    currentRole,
	}), nil
}

func (a *userRBACAPI) authenticatedUser(r *http.Request) (adminPlatformData, adminUser, error) {
	if store, ok := a.store.(activeIdentityStore); ok {
		userID, err := authenticatedUserID(r, a.sessions)
		if err != nil {
			return adminPlatformData{}, adminUser{}, err
		}
		user, found, err := store.GetActiveUser(userID)
		if err != nil {
			return adminPlatformData{}, adminUser{}, err
		}
		if !found {
			return adminPlatformData{}, adminUser{}, errUnauthorized
		}
		data := adminPlatformData{Users: []adminUser{user}}
		if agent, found, err := store.GetChannelAgentForUser(user.ID); err != nil {
			return adminPlatformData{}, adminUser{}, err
		} else if found {
			data.ChannelAgents = []adminChannelAgent{agent}
		}
		if operationStore, ok := a.store.(operationCenterIdentityStore); ok {
			if center, found, err := operationStore.GetOperationCenterForUser(user.ID); err != nil {
				return adminPlatformData{}, adminUser{}, err
			} else if found {
				data.OperationCenters = []adminOperationCenter{center}
			}
		}
		return data, user, nil
	}
	data, err := a.store.AdminData()
	if err != nil {
		return adminPlatformData{}, adminUser{}, err
	}
	user, err := authAPI{store: a.store, sessions: a.sessions}.authenticatedUser(r, data)
	return data, user, err
}

func rolesForUser(data adminPlatformData, user adminUser) []string {
	roles := []string{roleUser}
	legacyRole := strings.ToUpper(strings.TrimSpace(user.Role))
	if strings.EqualFold(user.AgentStatus, "ACTIVE") {
		roles = appendUnique(roles, roleAgent)
	}
	if strings.EqualFold(user.OperationCenterStatus, "ACTIVE") {
		roles = appendUnique(roles, roleOperation)
	}
	if agent, ok := channelAgentForUser(data.ChannelAgents, user.ID); ok && strings.EqualFold(agent.Status, "ACTIVE") {
		roles = appendUnique(roles, roleAgent)
	}
	if _, ok := activeOperationCenterForUser(data.OperationCenters, user.ID); ok {
		roles = appendUnique(roles, roleOperation)
	}
	if legacyRole == "SUPER_ADMIN" {
		roles = appendUnique(roles, roleEnterpriseAdmin)
		roles = appendUnique(roles, roleAIAdmin)
		roles = appendUnique(roles, roleFinance)
		roles = appendUnique(roles, roleCustomerService)
	}
	return sortedRoles(roles)
}

// userHasActiveChannelProfile is the explicit JSON-store compatibility path.
// PostgreSQL authorization uses xz_user_business_identities through
// GetChannelWorkbenchAgentForUser; this helper never grants endpoint access.
func userHasActiveChannelProfile(data adminPlatformData, userID string) bool {
	if agent, ok := channelAgentForUser(data.ChannelAgents, userID); ok && strings.EqualFold(agent.Status, "ACTIVE") {
		return true
	}
	_, ok := activeOperationCenterForUser(data.OperationCenters, userID)
	return ok
}

func normalizedUserRoleAccess(access userRoleAccess) userRoleAccess {
	access.UserID = strings.TrimSpace(access.UserID)
	access.TenantID = firstNonEmptyString(access.TenantID, "tenant_default")
	access.OrganizationID = firstNonEmptyString(access.OrganizationID, "organization_default")
	access.Roles = sortedRoles(appendUnique(access.Roles, roleUser))
	access.CurrentRole = normalizeAppRole(access.CurrentRole)
	if !containsString(access.Roles, access.CurrentRole) {
		access.CurrentRole = roleUser
	}
	if len(access.Permissions) == 0 {
		access.Permissions = permissionsForCurrentRole(access.CurrentRole)
	} else {
		permissions := make([]string, 0, len(access.Permissions))
		for _, permission := range access.Permissions {
			permissions = appendUnique(permissions, strings.TrimSpace(permission))
		}
		sort.Strings(permissions)
		access.Permissions = permissions
	}
	return access
}

func normalizeAppRole(value any) string {
	role := strings.ToUpper(strings.TrimSpace(stringValue(value)))
	if _, ok := rolePermissionMatrix[role]; ok {
		return role
	}
	return ""
}

func permissionsForCurrentRole(currentRole string) []string {
	permissions := append([]string{}, rolePermissionMatrix[roleUser]...)
	if currentRole != roleUser {
		for _, permission := range rolePermissionMatrix[currentRole] {
			permissions = appendUnique(permissions, permission)
		}
	}
	sort.Strings(permissions)
	return permissions
}

func sortedRoles(roles []string) []string {
	result := make([]string, 0, len(roles))
	for _, role := range roles {
		role = normalizeAppRole(role)
		if role != "" {
			result = appendUnique(result, role)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, leftFound := roleOrder[result[i]]
		right, rightFound := roleOrder[result[j]]
		if leftFound && rightFound && left != right {
			return left < right
		}
		return result[i] < result[j]
	})
	return result
}

func appendUnique(items []string, values ...string) []string {
	for _, value := range values {
		if value != "" && !containsString(items, value) {
			items = append(items, value)
		}
	}
	return items
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func writeUserRBACError(w http.ResponseWriter, err error) {
	if errors.Is(err, errUnauthorized) {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if errors.Is(err, errForbidden) {
		writeError(w, http.StatusForbidden, err)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}
