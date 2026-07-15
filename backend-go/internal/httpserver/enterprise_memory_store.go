package httpserver

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

func normalizeEnterpriseMemoryState(state *enterpriseMemoryState) {
	if state.Wallets == nil {
		state.Wallets = map[string]enterpriseWalletSummary{}
	}
	if state.Subscriptions == nil {
		state.Subscriptions = map[string]enterpriseSubscriptionSummary{}
	}
	if state.CurrentContexts == nil {
		state.CurrentContexts = map[string]enterpriseCurrentState{}
	}
}

func memoryActiveUser(data adminPlatformData, userID string) (adminUser, bool) {
	for _, user := range data.Users {
		if user.ID == userID && strings.EqualFold(user.Status, "ACTIVE") {
			return user, true
		}
	}
	return adminUser{}, false
}

func memoryTenant(state enterpriseMemoryState, tenantID string) (enterpriseTenant, bool) {
	for _, tenant := range state.Tenants {
		if tenant.ID == tenantID {
			return tenant, true
		}
	}
	return enterpriseTenant{}, false
}

func memoryOrganization(state enterpriseMemoryState, tenantID string, organizationID string) (enterpriseOrganization, bool) {
	for _, organization := range state.Organizations {
		if organization.TenantID == tenantID && organization.ID == organizationID {
			return organization, true
		}
	}
	return enterpriseOrganization{}, false
}

func memoryEnterpriseContexts(data adminPlatformData, userID string) ([]enterpriseContext, error) {
	user, found := memoryActiveUser(data, userID)
	if !found {
		return nil, errUnauthorized
	}
	state := data.Enterprise
	normalizeEnterpriseMemoryState(&state)
	current := state.CurrentContexts[userID]
	if current.Type == "" {
		current = enterpriseCurrentState{Type: contextPersonal, TenantID: firstNonEmptyString(user.TenantID, "tenant_default"), OrganizationID: firstNonEmptyString(user.OrganizationID, "organization_default"), CurrentRole: roleUser}
	}
	globalRoles := rolesForUser(data, user)
	personalTenantID := firstNonEmptyString(user.TenantID, "tenant_default")
	personalOrganizationID := firstNonEmptyString(user.OrganizationID, "organization_default")
	items := []enterpriseContext{{
		Type: contextPersonal, TenantID: personalTenantID, TenantName: "个人空间", OrganizationID: personalOrganizationID,
		OrganizationName: "个人空间", MemberStatus: "ACTIVE", CertificationStatus: "NOT_REQUIRED", Roles: []string{roleUser},
		CurrentRole: roleUser, Permissions: permissionsForCurrentRole(roleUser), DataScope: dataScopeSelf,
		Entitlements: map[string]any{"identity": contextPersonal}, Current: current.Type == contextPersonal,
	}}
	if containsString(globalRoles, roleAgent) {
		items = append(items, enterpriseContext{
			Type: contextAgent, TenantID: "tenant_default", TenantName: "渠道空间", OrganizationID: personalOrganizationID,
			OrganizationName: "代理商工作台", MemberStatus: "ACTIVE", CertificationStatus: "NOT_REQUIRED", Roles: []string{roleAgent},
			CurrentRole: roleAgent, Permissions: permissionsForCurrentRole(roleAgent), DataScope: dataScopeSelf,
			Entitlements: map[string]any{"identity": contextAgent}, Current: current.Type == contextAgent,
		})
	}
	if containsString(globalRoles, roleOperation) {
		items = append(items, enterpriseContext{
			Type: contextOperation, TenantID: "tenant_default", TenantName: "渠道空间", OrganizationID: personalOrganizationID,
			OrganizationName: "运营中心", MemberStatus: "ACTIVE", CertificationStatus: "NOT_REQUIRED", Roles: []string{roleOperation},
			CurrentRole: roleOperation, Permissions: permissionsForCurrentRole(roleOperation), DataScope: dataScopeSelf,
			Entitlements: map[string]any{"identity": contextOperation}, Current: current.Type == contextOperation,
		})
	}
	for _, member := range state.Members {
		if member.UserID != userID || !strings.EqualFold(member.MemberStatus, "ACTIVE") {
			continue
		}
		tenant, ok := memoryTenant(state, member.TenantID)
		if !ok || !strings.EqualFold(tenant.Status, "ACTIVE") {
			continue
		}
		organization, ok := memoryOrganization(state, tenant.ID, member.PrimaryOrganizationID)
		if !ok || !strings.EqualFold(organization.Status, "ACTIVE") {
			continue
		}
		roles := normalizeEnterpriseRoles(member.Roles)
		if len(roles) == 0 {
			roles = []string{roleEnterpriseMember}
		}
		currentRole := roles[0]
		isCurrent := current.Type == contextEnterprise && current.TenantID == tenant.ID
		if isCurrent && containsString(roles, normalizeAppRole(current.CurrentRole)) {
			currentRole = normalizeAppRole(current.CurrentRole)
		}
		wallet := state.Wallets[tenant.ID]
		subscription := state.Subscriptions[tenant.ID]
		items = append(items, enterpriseContext{
			Type: contextEnterprise, TenantID: tenant.ID, TenantName: tenant.Name, OrganizationID: organization.ID,
			OrganizationName: organization.Name, MemberStatus: member.MemberStatus, CertificationStatus: firstNonEmptyString(tenant.CertificationStatus, "UNVERIFIED"),
			Roles: roles, CurrentRole: currentRole, Permissions: permissionsForCurrentRole(currentRole), DataScope: normalizedDataScope(member.DataScope, currentRole),
			Entitlements: subscription.Entitlements, Wallet: wallet, Current: isCurrent,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Current != items[j].Current {
			return items[i].Current
		}
		if items[i].Type != items[j].Type {
			return items[i].Type < items[j].Type
		}
		return items[i].TenantName < items[j].TenantName
	})
	return items, nil
}

func (s *jsonStore) EnterpriseContexts(userID string) ([]enterpriseContext, error) {
	data, err := s.AdminData()
	if err != nil {
		return nil, err
	}
	return memoryEnterpriseContexts(data, userID)
}

func (s *jsonStore) CurrentEnterpriseContext(userID string) (enterpriseContext, error) {
	items, err := s.EnterpriseContexts(userID)
	if err != nil {
		return enterpriseContext{}, err
	}
	for _, item := range items {
		if item.Current {
			return item, nil
		}
	}
	if len(items) == 0 {
		return enterpriseContext{}, errUnauthorized
	}
	items[0].Current = true
	return items[0], nil
}

func (s *jsonStore) SetEnterpriseCurrentContext(userID string, request enterpriseContextSwitchRequest) (enterpriseContext, error) {
	request.Type = strings.ToUpper(strings.TrimSpace(request.Type))
	request.TenantID = strings.TrimSpace(request.TenantID)
	request.OrganizationID = strings.TrimSpace(request.OrganizationID)
	request.Role = normalizeAppRole(request.Role)
	var selected enterpriseContext
	err := s.updateAdmin(func(data *adminPlatformData) error {
		normalizeEnterpriseMemoryState(&data.Enterprise)
		items, err := memoryEnterpriseContexts(*data, userID)
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.Type != request.Type {
				continue
			}
			if request.Type == contextEnterprise && item.TenantID != request.TenantID {
				continue
			}
			if request.OrganizationID != "" {
				organization, ok := memoryOrganization(data.Enterprise, item.TenantID, request.OrganizationID)
				if !ok || !strings.EqualFold(organization.Status, "ACTIVE") {
					return errForbidden
				}
				item.OrganizationID = organization.ID
				item.OrganizationName = organization.Name
			}
			if request.Role != "" {
				if !containsString(item.Roles, request.Role) {
					return errForbidden
				}
				item.CurrentRole = request.Role
				item.Permissions = permissionsForCurrentRole(request.Role)
			}
			selected = item
			selected.Current = true
			data.Enterprise.CurrentContexts[userID] = enterpriseCurrentState{
				Type: item.Type, TenantID: item.TenantID, OrganizationID: item.OrganizationID, CurrentRole: item.CurrentRole,
			}
			return nil
		}
		return errForbidden
	})
	return selected, err
}

func (s *jsonStore) GetUserRoleAccess(userID string) (userRoleAccess, bool, error) {
	data, err := s.AdminData()
	if err != nil {
		return userRoleAccess{}, false, err
	}
	user, found := memoryActiveUser(data, userID)
	if !found {
		return userRoleAccess{}, false, nil
	}
	current, err := s.CurrentEnterpriseContext(userID)
	if err != nil {
		return userRoleAccess{}, false, err
	}
	roles := rolesForUser(data, user)
	for _, member := range data.Enterprise.Members {
		if member.UserID == userID && strings.EqualFold(member.MemberStatus, "ACTIVE") {
			roles = appendUnique(roles, normalizeEnterpriseRoles(member.Roles)...)
		}
	}
	return normalizedUserRoleAccess(userRoleAccess{
		UserID: userID, TenantID: current.TenantID, OrganizationID: current.OrganizationID, Roles: roles,
		CurrentRole: current.CurrentRole, Permissions: current.Permissions,
	}), true, nil
}

func (s *jsonStore) SetUserCurrentRole(userID string, role string) (userRoleAccess, error) {
	role = normalizeAppRole(role)
	if role == "" {
		return userRoleAccess{}, errForbidden
	}
	request := enterpriseContextSwitchRequest{Role: role}
	switch role {
	case roleUser:
		request.Type = contextPersonal
	case roleAgent:
		request.Type = contextAgent
	case roleOperation:
		request.Type = contextOperation
	default:
		request.Type = contextEnterprise
		items, err := s.EnterpriseContexts(userID)
		if err != nil {
			return userRoleAccess{}, err
		}
		for _, item := range items {
			if item.Type == contextEnterprise && containsString(item.Roles, role) {
				request.TenantID = item.TenantID
				break
			}
		}
	}
	if _, err := s.SetEnterpriseCurrentContext(userID, request); err != nil {
		return userRoleAccess{}, err
	}
	access, found, err := s.GetUserRoleAccess(userID)
	if err != nil {
		return userRoleAccess{}, err
	}
	if !found {
		return userRoleAccess{}, errUnauthorized
	}
	return access, nil
}

func (s *jsonStore) CreateEnterprise(userID string, request enterpriseCreateRequest) (enterpriseCreateResult, error) {
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || len([]rune(request.Name)) > 160 {
		return enterpriseCreateResult{}, fmt.Errorf("%w: enterprise name is required and must not exceed 160 characters", errEnterpriseInvalid)
	}
	var result enterpriseCreateResult
	err := s.updateAdmin(func(data *adminPlatformData) error {
		user, found := memoryActiveUser(*data, userID)
		if !found {
			return errUnauthorized
		}
		normalizeEnterpriseMemoryState(&data.Enterprise)
		now := enterpriseNow()
		tenantID := newEnterpriseResourceID("tenant")
		organizationID := newEnterpriseResourceID("organization")
		memberID := newEnterpriseResourceID("tenant_member")
		tenant := enterpriseTenant{ID: tenantID, Name: request.Name, OwnerUserID: userID, Status: "ACTIVE", CertificationStatus: "UNVERIFIED", Config: map[string]any{}, CreatedAt: now, UpdatedAt: now}
		organization := enterpriseOrganization{ID: organizationID, TenantID: tenantID, OrganizationType: "ROOT", Name: request.Name, Status: "ACTIVE", Metadata: map[string]any{"root": true}, CreatedAt: now, UpdatedAt: now}
		member := enterpriseMember{ID: memberID, TenantID: tenantID, UserID: userID, Name: user.Name, Email: user.Email, PrimaryOrganizationID: organizationID, OrganizationName: organization.Name, MemberStatus: "ACTIVE", CertificationStatus: "UNVERIFIED", DataScope: dataScopeTenantAll, Roles: []string{roleEnterpriseAdmin}, JoinedAt: now, CreatedAt: now, UpdatedAt: now}
		wallet := enterpriseWalletSummary{PointBalance: 1000, Status: "ACTIVE"}
		subscription := enterpriseSubscriptionSummary{ID: newEnterpriseResourceID("tenant_subscription"), PlanCode: "enterprise_trial", Status: "TRIALING", TrialExpiresAt: time.Now().UTC().Add(14 * 24 * time.Hour).Format(time.RFC3339Nano), Entitlements: defaultEnterpriseTrialEntitlements()}
		invitation := enterpriseInvitation{ID: newEnterpriseResourceID("tenant_invitation"), TenantID: tenantID, InvitationCode: newEnterpriseInvitationCode(), DefaultOrganizationID: organizationID, DefaultRole: roleEnterpriseMember, ExpiresAt: time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339Nano), Status: "PENDING", CreatedBy: userID, CreatedAt: now, UpdatedAt: now}
		data.Enterprise.Tenants = append(data.Enterprise.Tenants, tenant)
		data.Enterprise.Organizations = append(data.Enterprise.Organizations, organization)
		data.Enterprise.Members = append(data.Enterprise.Members, member)
		data.Enterprise.Wallets[tenantID] = wallet
		data.Enterprise.Subscriptions[tenantID] = subscription
		data.Enterprise.Invitations = append(data.Enterprise.Invitations, invitation)
		data.Enterprise.CurrentContexts[userID] = enterpriseCurrentState{Type: contextEnterprise, TenantID: tenantID, OrganizationID: organizationID, CurrentRole: roleEnterpriseAdmin}
		appendMemoryEnterpriseAudit(&data.Enterprise, enterpriseAccess{UserID: userID, TenantID: tenantID, OrganizationID: organizationID, Role: roleEnterpriseAdmin}, "enterprise.create", "tenant", tenantID, userID, map[string]any{"name": request.Name})
		context := enterpriseContext{Type: contextEnterprise, TenantID: tenantID, TenantName: tenant.Name, OrganizationID: organizationID, OrganizationName: organization.Name, MemberStatus: "ACTIVE", CertificationStatus: "UNVERIFIED", Roles: []string{roleEnterpriseAdmin}, CurrentRole: roleEnterpriseAdmin, Permissions: permissionsForCurrentRole(roleEnterpriseAdmin), DataScope: dataScopeTenantAll, Entitlements: subscription.Entitlements, Wallet: wallet, Current: true}
		result = enterpriseCreateResult{Tenant: tenant, Context: context, Invitation: invitation, Organization: organization}
		return nil
	})
	return result, err
}

func (s *jsonStore) EnterpriseAccess(userID string, permission string) (enterpriseAccess, error) {
	current, err := s.CurrentEnterpriseContext(userID)
	if err != nil {
		return enterpriseAccess{}, err
	}
	if current.Type != contextEnterprise || !strings.EqualFold(current.MemberStatus, "ACTIVE") || !containsString(current.Permissions, permission) {
		return enterpriseAccess{}, errForbidden
	}
	data, err := s.AdminData()
	if err != nil {
		return enterpriseAccess{}, err
	}
	for _, member := range data.Enterprise.Members {
		if member.UserID == userID && member.TenantID == current.TenantID && strings.EqualFold(member.MemberStatus, "ACTIVE") {
			return enterpriseAccess{UserID: userID, TenantID: current.TenantID, TenantName: current.TenantName, OrganizationID: current.OrganizationID, MemberID: member.ID, Role: current.CurrentRole, Roles: current.Roles, Permissions: current.Permissions, DataScope: current.DataScope}, nil
		}
	}
	return enterpriseAccess{}, errForbidden
}

func (s *jsonStore) EnterpriseOverview(access enterpriseAccess) (enterpriseOverview, error) {
	data, err := s.AdminData()
	if err != nil {
		return enterpriseOverview{}, err
	}
	tenant, found := memoryTenant(data.Enterprise, access.TenantID)
	if !found {
		return enterpriseOverview{}, errEnterpriseNotFound
	}
	result := enterpriseOverview{Tenant: tenant, Wallet: data.Enterprise.Wallets[access.TenantID], Subscription: data.Enterprise.Subscriptions[access.TenantID]}
	for _, member := range data.Enterprise.Members {
		if member.TenantID == access.TenantID {
			result.MemberCount++
			if strings.EqualFold(member.MemberStatus, "ACTIVE") {
				result.ActiveMembers++
			}
		}
	}
	for _, organization := range data.Enterprise.Organizations {
		if organization.TenantID == access.TenantID && !strings.EqualFold(organization.Status, "DELETED") {
			result.OrganizationCount++
		}
	}
	for _, request := range data.Enterprise.JoinRequests {
		if request.TenantID == access.TenantID && strings.EqualFold(request.Status, "PENDING") {
			result.PendingJoinRequests++
		}
	}
	result.Current, _ = s.CurrentEnterpriseContext(access.UserID)
	return result, nil
}

func (s *jsonStore) ListEnterpriseMembers(access enterpriseAccess) ([]enterpriseMember, error) {
	data, err := s.AdminData()
	if err != nil {
		return nil, err
	}
	allowedOrganizations := memoryAllowedOrganizations(data.Enterprise, access)
	items := make([]enterpriseMember, 0)
	for _, member := range data.Enterprise.Members {
		if member.TenantID == access.TenantID && memoryMemberVisible(access, member, allowedOrganizations) {
			items = append(items, member)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].JoinedAt > items[j].JoinedAt })
	return items, nil
}

func (s *jsonStore) GetEnterpriseMember(access enterpriseAccess, id string) (enterpriseMember, error) {
	items, err := s.ListEnterpriseMembers(access)
	if err != nil {
		return enterpriseMember{}, err
	}
	for _, item := range items {
		if item.ID == strings.TrimSpace(id) {
			return item, nil
		}
	}
	return enterpriseMember{}, errEnterpriseNotFound
}

func (s *jsonStore) CreateEnterpriseInvitation(access enterpriseAccess, request enterpriseInvitationCreateRequest) (enterpriseInvitation, error) {
	request.InvitedUserID = strings.TrimSpace(request.InvitedUserID)
	request.InvitedEmail = strings.ToLower(strings.TrimSpace(request.InvitedEmail))
	request.DefaultOrganizationID = strings.TrimSpace(request.DefaultOrganizationID)
	request.DefaultRole = normalizeEnterpriseRole(request.DefaultRole)
	if request.DefaultRole == "" {
		request.DefaultRole = roleEnterpriseMember
	}
	if request.ExpiresInHours <= 0 || request.ExpiresInHours > 24*30 {
		request.ExpiresInHours = 24 * 7
	}
	var item enterpriseInvitation
	err := s.updateAdmin(func(data *adminPlatformData) error {
		normalizeEnterpriseMemoryState(&data.Enterprise)
		if request.DefaultOrganizationID == "" {
			request.DefaultOrganizationID = access.OrganizationID
		}
		if _, ok := memoryOrganization(data.Enterprise, access.TenantID, request.DefaultOrganizationID); !ok {
			return errEnterpriseNotFound
		}
		if request.InvitedUserID != "" {
			if _, ok := memoryActiveUser(*data, request.InvitedUserID); !ok {
				return errEnterpriseNotFound
			}
		}
		now := enterpriseNow()
		item = enterpriseInvitation{ID: newEnterpriseResourceID("tenant_invitation"), TenantID: access.TenantID, InvitationCode: newEnterpriseInvitationCode(), InvitedUserID: request.InvitedUserID, InvitedEmail: request.InvitedEmail, DefaultOrganizationID: request.DefaultOrganizationID, DefaultRole: request.DefaultRole, ExpiresAt: time.Now().UTC().Add(time.Duration(request.ExpiresInHours) * time.Hour).Format(time.RFC3339Nano), Status: "PENDING", CreatedBy: access.UserID, CreatedAt: now, UpdatedAt: now}
		data.Enterprise.Invitations = append(data.Enterprise.Invitations, item)
		appendMemoryEnterpriseAudit(&data.Enterprise, access, "enterprise.member.invite", "invitation", item.ID, request.InvitedUserID, map[string]any{"email": request.InvitedEmail, "role": request.DefaultRole})
		return nil
	})
	return item, err
}

func (s *jsonStore) AcceptEnterpriseInvitation(userID string, request enterpriseInvitationAcceptRequest) (enterpriseContext, error) {
	request.InvitationCode = strings.ToUpper(strings.TrimSpace(request.InvitationCode))
	if request.InvitationCode == "" {
		return enterpriseContext{}, fmt.Errorf("%w: invitationCode is required", errEnterpriseInvalid)
	}
	var selected enterpriseContext
	err := s.updateAdmin(func(data *adminPlatformData) error {
		user, found := memoryActiveUser(*data, userID)
		if !found {
			return errUnauthorized
		}
		normalizeEnterpriseMemoryState(&data.Enterprise)
		invitationIndex := -1
		for index := range data.Enterprise.Invitations {
			if strings.EqualFold(data.Enterprise.Invitations[index].InvitationCode, request.InvitationCode) {
				invitationIndex = index
				break
			}
		}
		if invitationIndex < 0 {
			return errEnterpriseNotFound
		}
		invitation := &data.Enterprise.Invitations[invitationIndex]
		expiresAt, _ := time.Parse(time.RFC3339Nano, invitation.ExpiresAt)
		if !strings.EqualFold(invitation.Status, "PENDING") || expiresAt.Before(time.Now().UTC()) {
			return errEnterpriseConflict
		}
		if invitation.InvitedUserID != "" && invitation.InvitedUserID != userID {
			return errForbidden
		}
		if invitation.InvitedEmail != "" && !strings.EqualFold(invitation.InvitedEmail, user.Email) {
			return errForbidden
		}
		tenant, ok := memoryTenant(data.Enterprise, invitation.TenantID)
		if !ok || !strings.EqualFold(tenant.Status, "ACTIVE") {
			return errForbidden
		}
		organization, ok := memoryOrganization(data.Enterprise, tenant.ID, invitation.DefaultOrganizationID)
		if !ok || !strings.EqualFold(organization.Status, "ACTIVE") {
			return errForbidden
		}
		for _, member := range data.Enterprise.Members {
			if member.TenantID == tenant.ID && member.UserID == userID && strings.EqualFold(member.MemberStatus, "ACTIVE") {
				return errEnterpriseConflict
			}
		}
		now := enterpriseNow()
		role := normalizeEnterpriseRole(invitation.DefaultRole)
		if role == "" {
			role = roleEnterpriseMember
		}
		member := enterpriseMember{ID: newEnterpriseResourceID("tenant_member"), TenantID: tenant.ID, UserID: userID, Name: user.Name, Email: user.Email, PrimaryOrganizationID: organization.ID, OrganizationName: organization.Name, MemberStatus: "ACTIVE", CertificationStatus: tenant.CertificationStatus, DataScope: defaultDataScopeForRole(role), Roles: []string{role}, JoinedAt: now, InvitedBy: invitation.CreatedBy, CreatedAt: now, UpdatedAt: now}
		data.Enterprise.Members = append(data.Enterprise.Members, member)
		invitation.Status = "ACCEPTED"
		invitation.AcceptedBy = userID
		invitation.AcceptedAt = now
		invitation.UpdatedAt = now
		data.Enterprise.CurrentContexts[userID] = enterpriseCurrentState{Type: contextEnterprise, TenantID: tenant.ID, OrganizationID: organization.ID, CurrentRole: role}
		subscription := data.Enterprise.Subscriptions[tenant.ID]
		selected = enterpriseContext{Type: contextEnterprise, TenantID: tenant.ID, TenantName: tenant.Name, OrganizationID: organization.ID, OrganizationName: organization.Name, MemberStatus: "ACTIVE", CertificationStatus: tenant.CertificationStatus, Roles: []string{role}, CurrentRole: role, Permissions: permissionsForCurrentRole(role), DataScope: member.DataScope, Entitlements: subscription.Entitlements, Wallet: data.Enterprise.Wallets[tenant.ID], Current: true}
		appendMemoryEnterpriseAudit(&data.Enterprise, enterpriseAccess{UserID: userID, TenantID: tenant.ID, OrganizationID: organization.ID, Role: role}, "enterprise.invitation.accept", "member", member.ID, userID, map[string]any{"invitationId": invitation.ID})
		return nil
	})
	return selected, err
}

func (s *jsonStore) CreateEnterpriseJoinRequest(userID string, request enterpriseJoinRequestCreate) (enterpriseJoinRequest, error) {
	request.TenantID = strings.TrimSpace(request.TenantID)
	request.RequestedOrganizationID = strings.TrimSpace(request.RequestedOrganizationID)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.TenantID == "" {
		return enterpriseJoinRequest{}, fmt.Errorf("%w: tenantId is required", errEnterpriseInvalid)
	}
	var item enterpriseJoinRequest
	err := s.updateAdmin(func(data *adminPlatformData) error {
		user, found := memoryActiveUser(*data, userID)
		if !found {
			return errUnauthorized
		}
		normalizeEnterpriseMemoryState(&data.Enterprise)
		tenant, ok := memoryTenant(data.Enterprise, request.TenantID)
		if !ok || !strings.EqualFold(tenant.Status, "ACTIVE") {
			return errEnterpriseNotFound
		}
		if request.RequestedOrganizationID == "" {
			for _, organization := range data.Enterprise.Organizations {
				if organization.TenantID == tenant.ID && organization.ParentID == "" && strings.EqualFold(organization.Status, "ACTIVE") {
					request.RequestedOrganizationID = organization.ID
					break
				}
			}
		}
		if _, ok := memoryOrganization(data.Enterprise, tenant.ID, request.RequestedOrganizationID); !ok {
			return errEnterpriseNotFound
		}
		for _, member := range data.Enterprise.Members {
			if member.TenantID == tenant.ID && member.UserID == userID && strings.EqualFold(member.MemberStatus, "ACTIVE") {
				return errEnterpriseConflict
			}
		}
		for _, existing := range data.Enterprise.JoinRequests {
			if existing.TenantID == tenant.ID && existing.ApplicantUserID == userID && strings.EqualFold(existing.Status, "PENDING") {
				return errEnterpriseConflict
			}
		}
		now := enterpriseNow()
		item = enterpriseJoinRequest{ID: newEnterpriseResourceID("tenant_join_request"), TenantID: tenant.ID, ApplicantUserID: userID, ApplicantName: user.Name, ApplicantEmail: user.Email, RequestedOrganizationID: request.RequestedOrganizationID, RequestedRole: roleEnterpriseMember, Reason: request.Reason, Status: "PENDING", CreatedAt: now, UpdatedAt: now}
		data.Enterprise.JoinRequests = append(data.Enterprise.JoinRequests, item)
		return nil
	})
	return item, err
}

func (s *jsonStore) ListEnterpriseJoinRequests(access enterpriseAccess) ([]enterpriseJoinRequest, error) {
	data, err := s.AdminData()
	if err != nil {
		return nil, err
	}
	items := make([]enterpriseJoinRequest, 0)
	for _, item := range data.Enterprise.JoinRequests {
		if item.TenantID == access.TenantID {
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
	return items, nil
}

func (s *jsonStore) ReviewEnterpriseJoinRequest(access enterpriseAccess, id string, approved bool, comment string) (enterpriseJoinRequest, error) {
	var item enterpriseJoinRequest
	err := s.updateAdmin(func(data *adminPlatformData) error {
		normalizeEnterpriseMemoryState(&data.Enterprise)
		index := -1
		for i := range data.Enterprise.JoinRequests {
			if data.Enterprise.JoinRequests[i].TenantID == access.TenantID && data.Enterprise.JoinRequests[i].ID == strings.TrimSpace(id) {
				index = i
				break
			}
		}
		if index < 0 {
			return errEnterpriseNotFound
		}
		joinRequest := &data.Enterprise.JoinRequests[index]
		if !strings.EqualFold(joinRequest.Status, "PENDING") {
			return errEnterpriseConflict
		}
		now := enterpriseNow()
		joinRequest.Status = map[bool]string{true: "APPROVED", false: "REJECTED"}[approved]
		joinRequest.ReviewedBy = access.UserID
		joinRequest.ReviewedAt = now
		joinRequest.ReviewComment = strings.TrimSpace(comment)
		joinRequest.UpdatedAt = now
		if approved {
			user, found := memoryActiveUser(*data, joinRequest.ApplicantUserID)
			if !found {
				return errEnterpriseNotFound
			}
			organization, found := memoryOrganization(data.Enterprise, access.TenantID, joinRequest.RequestedOrganizationID)
			if !found {
				return errEnterpriseNotFound
			}
			member := enterpriseMember{ID: newEnterpriseResourceID("tenant_member"), TenantID: access.TenantID, UserID: user.ID, Name: user.Name, Email: user.Email, PrimaryOrganizationID: organization.ID, OrganizationName: organization.Name, MemberStatus: "ACTIVE", CertificationStatus: "UNVERIFIED", DataScope: dataScopeSelf, Roles: []string{roleEnterpriseMember}, JoinedAt: now, CreatedAt: now, UpdatedAt: now}
			data.Enterprise.Members = append(data.Enterprise.Members, member)
		}
		item = *joinRequest
		appendMemoryEnterpriseAudit(&data.Enterprise, access, "enterprise.join_request."+strings.ToLower(item.Status), "join_request", item.ID, item.ApplicantUserID, map[string]any{"comment": item.ReviewComment})
		return nil
	})
	return item, err
}

func (s *jsonStore) UpdateEnterpriseMember(access enterpriseAccess, id string, request enterpriseMemberUpdateRequest) (enterpriseMember, error) {
	request.PrimaryOrganizationID = strings.TrimSpace(request.PrimaryOrganizationID)
	request.Roles = normalizeEnterpriseRoles(request.Roles)
	request.DataScope = normalizedDataScope(request.DataScope, "")
	var item enterpriseMember
	err := s.updateAdmin(func(data *adminPlatformData) error {
		normalizeEnterpriseMemoryState(&data.Enterprise)
		index := memoryMemberIndex(data.Enterprise, access.TenantID, strings.TrimSpace(id))
		if index < 0 {
			return errEnterpriseNotFound
		}
		member := &data.Enterprise.Members[index]
		if request.PrimaryOrganizationID != "" {
			organization, ok := memoryOrganization(data.Enterprise, access.TenantID, request.PrimaryOrganizationID)
			if !ok || !strings.EqualFold(organization.Status, "ACTIVE") {
				return errEnterpriseNotFound
			}
			member.PrimaryOrganizationID = organization.ID
			member.OrganizationName = organization.Name
		}
		if len(request.Roles) > 0 {
			if member.UserID == memoryTenantOwner(data.Enterprise, access.TenantID) && !containsString(request.Roles, roleEnterpriseAdmin) {
				return errEnterpriseConflict
			}
			member.Roles = request.Roles
		}
		if request.DataScope != "" {
			member.DataScope = request.DataScope
		}
		member.UpdatedAt = enterpriseNow()
		item = *member
		appendMemoryEnterpriseAudit(&data.Enterprise, access, "enterprise.member.update", "member", member.ID, member.UserID, map[string]any{"roles": member.Roles, "organizationId": member.PrimaryOrganizationID})
		return nil
	})
	return item, err
}

func (s *jsonStore) DisableEnterpriseMember(access enterpriseAccess, id string) (enterpriseMember, error) {
	var item enterpriseMember
	err := s.updateAdmin(func(data *adminPlatformData) error {
		normalizeEnterpriseMemoryState(&data.Enterprise)
		index := memoryMemberIndex(data.Enterprise, access.TenantID, strings.TrimSpace(id))
		if index < 0 {
			return errEnterpriseNotFound
		}
		member := &data.Enterprise.Members[index]
		if member.UserID == access.UserID || member.UserID == memoryTenantOwner(data.Enterprise, access.TenantID) {
			return errEnterpriseConflict
		}
		member.MemberStatus = "DISABLED"
		member.UpdatedAt = enterpriseNow()
		delete(data.Enterprise.CurrentContexts, member.UserID)
		item = *member
		appendMemoryEnterpriseAudit(&data.Enterprise, access, "enterprise.member.disable", "member", member.ID, member.UserID, nil)
		return nil
	})
	return item, err
}

func (s *jsonStore) RemoveEnterpriseMember(access enterpriseAccess, id string) error {
	return s.updateAdmin(func(data *adminPlatformData) error {
		normalizeEnterpriseMemoryState(&data.Enterprise)
		index := memoryMemberIndex(data.Enterprise, access.TenantID, strings.TrimSpace(id))
		if index < 0 {
			return errEnterpriseNotFound
		}
		member := data.Enterprise.Members[index]
		if member.UserID == access.UserID || member.UserID == memoryTenantOwner(data.Enterprise, access.TenantID) {
			return errEnterpriseConflict
		}
		data.Enterprise.Members[index].MemberStatus = "REMOVED"
		data.Enterprise.Members[index].UpdatedAt = enterpriseNow()
		delete(data.Enterprise.CurrentContexts, member.UserID)
		appendMemoryEnterpriseAudit(&data.Enterprise, access, "enterprise.member.remove", "member", member.ID, member.UserID, nil)
		return nil
	})
}

func (s *jsonStore) EnterpriseOrganizationTree(access enterpriseAccess) ([]enterpriseOrganization, error) {
	data, err := s.AdminData()
	if err != nil {
		return nil, err
	}
	allowed := memoryAllowedOrganizations(data.Enterprise, access)
	flat := make([]enterpriseOrganization, 0)
	for _, organization := range data.Enterprise.Organizations {
		if organization.TenantID == access.TenantID && !strings.EqualFold(organization.Status, "DELETED") && allowed[organization.ID] {
			organization.Children = nil
			organization.MemberCount = 0
			for _, member := range data.Enterprise.Members {
				if member.TenantID == access.TenantID && member.PrimaryOrganizationID == organization.ID && strings.EqualFold(member.MemberStatus, "ACTIVE") {
					organization.MemberCount++
				}
			}
			flat = append(flat, organization)
		}
	}
	return buildEnterpriseOrganizationTree(flat), nil
}

func (s *jsonStore) CreateEnterpriseOrganization(access enterpriseAccess, request enterpriseOrganizationCreateRequest) (enterpriseOrganization, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.ParentID = strings.TrimSpace(request.ParentID)
	request.OrganizationType = strings.ToUpper(strings.TrimSpace(request.OrganizationType))
	if request.Name == "" {
		return enterpriseOrganization{}, fmt.Errorf("%w: organization name is required", errEnterpriseInvalid)
	}
	if request.OrganizationType == "" {
		request.OrganizationType = "DEPARTMENT"
	}
	var item enterpriseOrganization
	err := s.updateAdmin(func(data *adminPlatformData) error {
		if request.ParentID == "" {
			request.ParentID = access.OrganizationID
		}
		if _, ok := memoryOrganization(data.Enterprise, access.TenantID, request.ParentID); !ok {
			return errEnterpriseNotFound
		}
		now := enterpriseNow()
		item = enterpriseOrganization{ID: newEnterpriseResourceID("organization"), TenantID: access.TenantID, ParentID: request.ParentID, OrganizationType: request.OrganizationType, Name: request.Name, Status: "ACTIVE", Metadata: request.Metadata, CreatedAt: now, UpdatedAt: now}
		data.Enterprise.Organizations = append(data.Enterprise.Organizations, item)
		appendMemoryEnterpriseAudit(&data.Enterprise, access, "enterprise.organization.create", "organization", item.ID, "", map[string]any{"parentId": item.ParentID, "name": item.Name})
		return nil
	})
	return item, err
}

func (s *jsonStore) UpdateEnterpriseOrganization(access enterpriseAccess, id string, request enterpriseOrganizationUpdateRequest) (enterpriseOrganization, error) {
	var item enterpriseOrganization
	err := s.updateAdmin(func(data *adminPlatformData) error {
		index := memoryOrganizationIndex(data.Enterprise, access.TenantID, strings.TrimSpace(id))
		if index < 0 {
			return errEnterpriseNotFound
		}
		organization := &data.Enterprise.Organizations[index]
		if strings.TrimSpace(request.Name) != "" {
			organization.Name = strings.TrimSpace(request.Name)
		}
		if value := strings.ToUpper(strings.TrimSpace(request.OrganizationType)); value != "" {
			organization.OrganizationType = value
		}
		if value := strings.ToUpper(strings.TrimSpace(request.Status)); value != "" {
			organization.Status = value
		}
		if request.Metadata != nil {
			organization.Metadata = request.Metadata
		}
		organization.UpdatedAt = enterpriseNow()
		item = *organization
		appendMemoryEnterpriseAudit(&data.Enterprise, access, "enterprise.organization.update", "organization", item.ID, "", map[string]any{"name": item.Name, "status": item.Status})
		return nil
	})
	return item, err
}

func (s *jsonStore) MoveEnterpriseOrganization(access enterpriseAccess, id string, request enterpriseOrganizationMoveRequest) (enterpriseOrganization, error) {
	request.ParentID = strings.TrimSpace(request.ParentID)
	if request.ParentID == "" || request.ParentID == strings.TrimSpace(id) {
		return enterpriseOrganization{}, fmt.Errorf("%w: a different parentId is required", errEnterpriseInvalid)
	}
	var item enterpriseOrganization
	err := s.updateAdmin(func(data *adminPlatformData) error {
		index := memoryOrganizationIndex(data.Enterprise, access.TenantID, strings.TrimSpace(id))
		if index < 0 {
			return errEnterpriseNotFound
		}
		if data.Enterprise.Organizations[index].ParentID == "" {
			return errEnterpriseConflict
		}
		if _, ok := memoryOrganization(data.Enterprise, access.TenantID, request.ParentID); !ok {
			return errEnterpriseNotFound
		}
		for parent := request.ParentID; parent != ""; {
			if parent == id {
				return errEnterpriseConflict
			}
			organization, ok := memoryOrganization(data.Enterprise, access.TenantID, parent)
			if !ok {
				break
			}
			parent = organization.ParentID
		}
		data.Enterprise.Organizations[index].ParentID = request.ParentID
		data.Enterprise.Organizations[index].UpdatedAt = enterpriseNow()
		item = data.Enterprise.Organizations[index]
		appendMemoryEnterpriseAudit(&data.Enterprise, access, "enterprise.organization.move", "organization", item.ID, "", map[string]any{"parentId": item.ParentID})
		return nil
	})
	return item, err
}

func (s *jsonStore) DeleteEnterpriseOrganization(access enterpriseAccess, id string) error {
	return s.updateAdmin(func(data *adminPlatformData) error {
		index := memoryOrganizationIndex(data.Enterprise, access.TenantID, strings.TrimSpace(id))
		if index < 0 {
			return errEnterpriseNotFound
		}
		organization := &data.Enterprise.Organizations[index]
		if organization.ParentID == "" {
			return errEnterpriseConflict
		}
		for _, child := range data.Enterprise.Organizations {
			if child.TenantID == access.TenantID && child.ParentID == organization.ID && !strings.EqualFold(child.Status, "DELETED") {
				return errEnterpriseConflict
			}
		}
		for _, member := range data.Enterprise.Members {
			if member.TenantID == access.TenantID && member.PrimaryOrganizationID == organization.ID && strings.EqualFold(member.MemberStatus, "ACTIVE") {
				return errEnterpriseConflict
			}
		}
		organization.Status = "DELETED"
		organization.UpdatedAt = enterpriseNow()
		appendMemoryEnterpriseAudit(&data.Enterprise, access, "enterprise.organization.delete", "organization", organization.ID, "", nil)
		return nil
	})
}

func (s *jsonStore) EnterpriseAuditLogs(access enterpriseAccess, limit int) ([]enterpriseAuditLog, error) {
	data, err := s.AdminData()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	items := make([]enterpriseAuditLog, 0, limit)
	for index := len(data.Enterprise.AuditLogs) - 1; index >= 0 && len(items) < limit; index-- {
		if data.Enterprise.AuditLogs[index].TenantID == access.TenantID {
			items = append(items, data.Enterprise.AuditLogs[index])
		}
	}
	return items, nil
}

func (s *jsonStore) SubmitEnterpriseCertification(access enterpriseAccess, request enterpriseCertificationSubmitRequest) (enterpriseCertification, error) {
	request.LegalName = strings.TrimSpace(request.LegalName)
	request.UnifiedSocialCreditCode = strings.ToUpper(strings.TrimSpace(request.UnifiedSocialCreditCode))
	request.LegalRepresentativeName = strings.TrimSpace(request.LegalRepresentativeName)
	if request.LegalName == "" || request.UnifiedSocialCreditCode == "" || len(request.DocumentURLs) == 0 {
		return enterpriseCertification{}, fmt.Errorf("%w: legal name, unified social credit code and document URLs are required", errEnterpriseInvalid)
	}
	var result enterpriseCertification
	err := s.updateAdmin(func(data *adminPlatformData) error {
		now := enterpriseNow()
		result = enterpriseCertification{
			ID: newEnterpriseResourceID("tenant_certification"), TenantID: access.TenantID,
			LegalName: request.LegalName, UnifiedSocialCreditCode: request.UnifiedSocialCreditCode,
			LegalRepresentativeName: request.LegalRepresentativeName, DocumentURLs: request.DocumentURLs,
			Status: "PENDING", SubmittedBy: access.UserID, Metadata: request.Metadata, CreatedAt: now, UpdatedAt: now,
		}
		for index := range data.Enterprise.Certifications {
			if data.Enterprise.Certifications[index].TenantID == access.TenantID && strings.EqualFold(data.Enterprise.Certifications[index].Status, "PENDING") {
				result.ID = data.Enterprise.Certifications[index].ID
				result.CreatedAt = data.Enterprise.Certifications[index].CreatedAt
				data.Enterprise.Certifications[index] = result
				goto updateTenant
			}
		}
		data.Enterprise.Certifications = append(data.Enterprise.Certifications, result)
	updateTenant:
		for index := range data.Enterprise.Tenants {
			if data.Enterprise.Tenants[index].ID == access.TenantID {
				data.Enterprise.Tenants[index].CertificationStatus = "PENDING"
				if data.Enterprise.Tenants[index].Config == nil {
					data.Enterprise.Tenants[index].Config = map[string]any{}
				}
				data.Enterprise.Tenants[index].Config["certificationStatus"] = "PENDING"
				data.Enterprise.Tenants[index].UpdatedAt = now
				break
			}
		}
		appendMemoryEnterpriseAudit(&data.Enterprise, access, "enterprise.certification.submit", "tenant_certification", result.ID, "", map[string]any{"status": "PENDING"})
		return nil
	})
	return result, err
}

func normalizeEnterpriseRole(value string) string {
	role := normalizeAppRole(value)
	switch role {
	case roleEnterpriseAdmin, roleAIAdmin, roleFinance, roleCustomerService, roleEnterpriseMember:
		return role
	default:
		return ""
	}
}

func normalizeEnterpriseRoles(values []string) []string {
	roles := make([]string, 0, len(values))
	for _, value := range values {
		if role := normalizeEnterpriseRole(value); role != "" {
			roles = appendUnique(roles, role)
		}
	}
	return sortedRoles(roles)
}

func normalizedDataScope(value string, role string) string {
	scope := strings.ToUpper(strings.TrimSpace(value))
	switch scope {
	case dataScopeTenantAll, dataScopeOrgAndChildren, dataScopeOrgSelf, dataScopeOwner, dataScopeSelf:
		return scope
	}
	if role != "" {
		return defaultDataScopeForRole(role)
	}
	return ""
}

func defaultDataScopeForRole(role string) string {
	switch role {
	case roleEnterpriseAdmin, roleFinance:
		return dataScopeTenantAll
	case roleAIAdmin, roleCustomerService:
		return dataScopeOrgAndChildren
	default:
		return dataScopeSelf
	}
}

func defaultEnterpriseTrialEntitlements() map[string]any {
	return map[string]any{"plan": "enterprise_trial", "trialDays": 14, "memberLimit": 20, "organizationLimit": 10, "knowledgeBaseLimit": 5, "sharedAgents": true}
}

func newEnterpriseResourceID(prefix string) string {
	return prefix + "_" + strings.TrimPrefix(newRequestID(), "req_")
}

func newEnterpriseInvitationCode() string {
	value := strings.ToUpper(strings.ReplaceAll(strings.TrimPrefix(newRequestID(), "req_"), "_", ""))
	if len(value) > 12 {
		value = value[len(value)-12:]
	}
	return "XZ" + value
}

func appendMemoryEnterpriseAudit(state *enterpriseMemoryState, access enterpriseAccess, action string, resourceType string, resourceID string, targetUserID string, metadata map[string]any) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	state.AuditLogs = append(state.AuditLogs, enterpriseAuditLog{ID: newEnterpriseResourceID("tenant_audit"), TenantID: access.TenantID, ActorUserID: access.UserID, ActorRole: access.Role, OrganizationID: access.OrganizationID, Action: action, ResourceType: resourceType, ResourceID: resourceID, TargetUserID: targetUserID, Status: "SUCCEEDED", Metadata: metadata, CreatedAt: enterpriseNow()})
}

func memoryMemberIndex(state enterpriseMemoryState, tenantID string, id string) int {
	for index := range state.Members {
		if state.Members[index].TenantID == tenantID && state.Members[index].ID == id {
			return index
		}
	}
	return -1
}

func memoryOrganizationIndex(state enterpriseMemoryState, tenantID string, id string) int {
	for index := range state.Organizations {
		if state.Organizations[index].TenantID == tenantID && state.Organizations[index].ID == id {
			return index
		}
	}
	return -1
}

func memoryTenantOwner(state enterpriseMemoryState, tenantID string) string {
	if tenant, ok := memoryTenant(state, tenantID); ok {
		return tenant.OwnerUserID
	}
	return ""
}

func memoryAllowedOrganizations(state enterpriseMemoryState, access enterpriseAccess) map[string]bool {
	allowed := map[string]bool{}
	switch access.DataScope {
	case dataScopeTenantAll:
		for _, organization := range state.Organizations {
			if organization.TenantID == access.TenantID {
				allowed[organization.ID] = true
			}
		}
	case dataScopeOrgAndChildren:
		allowed[access.OrganizationID] = true
		for changed := true; changed; {
			changed = false
			for _, organization := range state.Organizations {
				if organization.TenantID == access.TenantID && allowed[organization.ParentID] && !allowed[organization.ID] {
					allowed[organization.ID] = true
					changed = true
				}
			}
		}
	case dataScopeOrgSelf:
		allowed[access.OrganizationID] = true
	default:
		allowed[access.OrganizationID] = true
	}
	return allowed
}

func memoryMemberVisible(access enterpriseAccess, member enterpriseMember, allowedOrganizations map[string]bool) bool {
	switch access.DataScope {
	case dataScopeSelf, dataScopeOwner:
		return member.UserID == access.UserID
	default:
		return allowedOrganizations[member.PrimaryOrganizationID]
	}
}

func buildEnterpriseOrganizationTree(flat []enterpriseOrganization) []enterpriseOrganization {
	byParent := map[string][]enterpriseOrganization{}
	known := map[string]bool{}
	for _, item := range flat {
		known[item.ID] = true
		byParent[item.ParentID] = append(byParent[item.ParentID], item)
	}
	var build func(string, map[string]bool) []enterpriseOrganization
	build = func(parentID string, visited map[string]bool) []enterpriseOrganization {
		items := append([]enterpriseOrganization{}, byParent[parentID]...)
		sort.SliceStable(items, func(i, j int) bool { return items[i].Name < items[j].Name })
		for index := range items {
			if visited[items[index].ID] {
				continue
			}
			nextVisited := make(map[string]bool, len(visited)+1)
			for key, value := range visited {
				nextVisited[key] = value
			}
			nextVisited[items[index].ID] = true
			items[index].Children = build(items[index].ID, nextVisited)
		}
		return items
	}
	roots := make([]enterpriseOrganization, 0)
	for _, item := range flat {
		if item.ParentID == "" || !known[item.ParentID] {
			item.Children = build(item.ID, map[string]bool{item.ID: true})
			roots = append(roots, item)
		}
	}
	sort.SliceStable(roots, func(i, j int) bool { return roots[i].Name < roots[j].Name })
	return roots
}

var _ enterpriseStore = (*jsonStore)(nil)
var _ userRoleAccessStore = (*jsonStore)(nil)

// Keep errors imported when downstream build tags remove optional methods.
var _ = errors.Is
