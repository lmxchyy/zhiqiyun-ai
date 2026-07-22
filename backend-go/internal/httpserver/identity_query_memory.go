package httpserver

import (
	"sort"
	"strings"
)

func (s *jsonStore) GetAdminIdentityProfile(userID string) (adminIdentityProfile, error) {
	data, user, err := s.identityQueryUser(userID)
	if err != nil {
		return adminIdentityProfile{}, err
	}
	identities := memoryBusinessIdentities(data, user)
	return adminIdentityProfile{
		UserID: user.ID, AccountStatus: user.Status, LegacyRole: user.Role,
		AccountRoles: rolesForUser(data, user), PrimaryIdentity: primaryBusinessIdentity(identities),
		Identities: identities,
	}, nil
}

func (s *jsonStore) GetAdminIdentityHistory(userID string) (adminIdentityHistory, error) {
	data, user, err := s.identityQueryUser(userID)
	if err != nil {
		return adminIdentityHistory{}, err
	}
	return adminIdentityHistory{UserID: user.ID, Identities: memoryBusinessIdentities(data, user), ChangeRecords: []adminIdentityChangeRecord{}}, nil
}

func (s *jsonStore) GetAdminCurrentRelationship(userID string) (*adminUserRelationship, error) {
	data, user, err := s.identityQueryUser(userID)
	if err != nil {
		return nil, err
	}
	users := userMap(data.Users)
	agents := agentByIDMap(data.ChannelAgents)
	for _, relation := range data.CustomerRelations {
		if relation.CustomerUserID != user.ID || !strings.EqualFold(relation.Status, "ACTIVE") {
			continue
		}
		parent := agents[firstNonEmptyString(relation.DirectAgentID, relation.ParentAgentID)]
		center := operationCenterByID(data.OperationCenters, relation.OperationCenterID)
		return &adminUserRelationship{
			ID: relation.ID, TenantID: firstNonEmptyString(user.TenantID, "tenant_default"), UserID: user.ID,
			ParentAgentID: parent.ID, ParentAgentUserID: parent.UserID, ParentAgentName: users[parent.UserID].Name,
			OperationCenterID: center.ID, OperationCenterName: center.Name, EffectiveAt: relation.BindStartAt,
			Status: relation.Status, SourceType: relation.BindType, CreatedBy: "legacy_projection", CreatedAt: relation.CreatedAt, UpdatedAt: relation.UpdatedAt,
		}, nil
	}
	if agent, ok := channelAgentForUser(data.ChannelAgents, user.ID); ok {
		parent := agents[agent.ParentID]
		center := operationCenterByID(data.OperationCenters, agent.OperationCenterID)
		if parent.ID != "" || center.ID != "" {
			return &adminUserRelationship{
				ID: "legacy_relationship_" + agent.ID, TenantID: firstNonEmptyString(user.TenantID, "tenant_default"), UserID: user.ID,
				ParentAgentID: parent.ID, ParentAgentUserID: parent.UserID, ParentAgentName: users[parent.UserID].Name,
				OperationCenterID: center.ID, OperationCenterName: center.Name, EffectiveAt: agent.CreatedAt,
				Status: "ACTIVE", SourceType: "LEGACY_PROJECTION", CreatedBy: "legacy_projection", CreatedAt: agent.CreatedAt, UpdatedAt: agent.UpdatedAt,
			}, nil
		}
	}
	return nil, nil
}

func (s *jsonStore) GetAdminRelationshipHistory(userID string) ([]adminUserRelationship, error) {
	item, err := s.GetAdminCurrentRelationship(userID)
	if err != nil || item == nil {
		return []adminUserRelationship{}, err
	}
	return []adminUserRelationship{*item}, nil
}

func (s *jsonStore) GetAdminIdentityFinancialOverview(userID string) (adminIdentityFinancialOverview, error) {
	data, user, err := s.identityQueryUser(userID)
	if err != nil {
		return adminIdentityFinancialOverview{}, err
	}
	points := pointMap(data.PointAccounts)[user.ID]
	tokenCount, tokenGranted, tokenConsumed := 0, 0, 0
	for _, record := range data.TokenRecords {
		if record.UserID != user.ID {
			continue
		}
		tokenCount++
		if record.Amount >= 0 {
			tokenGranted += record.Amount
		} else {
			tokenConsumed -= record.Amount
		}
	}
	agent, _ := channelAgentForUser(data.ChannelAgents, user.ID)
	center, _ := activeOperationCenterForUser(data.OperationCenters, user.ID)
	commissionTotal, commissionCount := 0, 0
	for _, record := range data.Commissions {
		if record.AgentID == agent.ID || record.ReceiverID == agent.ID || record.ReceiverID == center.ID {
			commissionTotal += record.AmountCents
			commissionCount++
		}
	}
	return adminIdentityFinancialOverview{
		UserID:     user.ID,
		Membership: map[string]any{"level": user.MemberLevel, "planId": user.PlanID, "expiresAt": user.SubscriptionExpiresAt},
		Wallet:     map[string]any{"pointsAvailable": points.Available, "pointsFrozen": points.Frozen},
		Token:      map[string]any{"recordCount": tokenCount, "granted": tokenGranted, "consumed": tokenConsumed},
		Commission: map[string]any{"recordCount": commissionCount, "totalCents": commissionTotal, "source": "legacy_projection"},
	}, nil
}

func (s *jsonStore) identityQueryUser(userID string) (adminPlatformData, adminUser, error) {
	data, err := s.AdminData()
	if err != nil {
		return adminPlatformData{}, adminUser{}, err
	}
	for _, user := range data.Users {
		if user.ID == strings.TrimSpace(userID) {
			return data, user, nil
		}
	}
	return adminPlatformData{}, adminUser{}, errIdentityUserNotFound
}

func memoryBusinessIdentities(data adminPlatformData, user adminUser) []adminBusinessIdentity {
	createdAt := firstNonEmptyString(user.CreatedAt, user.UpdatedAt)
	items := []adminBusinessIdentity{{
		ID: "legacy_user_" + user.ID, TenantID: firstNonEmptyString(user.TenantID, "tenant_default"), UserID: user.ID,
		IdentityType: "USER", IdentityStatus: "ACTIVE", SourceType: "LEGACY_PROJECTION", EffectiveAt: createdAt,
		IdentityVersion: 1, CreatedBy: "legacy_projection", CreatedAt: createdAt, UpdatedAt: user.UpdatedAt,
	}}
	if agent, ok := channelAgentForUser(data.ChannelAgents, user.ID); ok {
		status := strings.ToUpper(firstNonEmptyString(agent.Status, user.AgentStatus, "ACTIVE"))
		items = append(items, adminBusinessIdentity{
			ID: "legacy_agent_" + agent.ID, TenantID: firstNonEmptyString(user.TenantID, "tenant_default"), UserID: user.ID,
			IdentityType: "AGENT", IdentityStatus: normalizeLegacyIdentityStatus(status), CommissionEnabled: status == "ACTIVE",
			SourceType: "LEGACY_PROJECTION", SourceOrderID: agent.JoinOrderID, EffectiveAt: agent.CreatedAt,
			IdentityVersion: 1, CreatedBy: "legacy_projection", CreatedAt: agent.CreatedAt, UpdatedAt: agent.UpdatedAt,
		})
	}
	if center, ok := activeOperationCenterForUser(data.OperationCenters, user.ID); ok {
		items = append(items, adminBusinessIdentity{
			ID: "legacy_operation_" + center.ID, TenantID: firstNonEmptyString(user.TenantID, "tenant_default"), UserID: user.ID,
			IdentityType: "OPERATION_CENTER", IdentityStatus: normalizeLegacyIdentityStatus(center.Status), CommissionEnabled: strings.EqualFold(center.Status, "ACTIVE"),
			SourceType: "LEGACY_PROJECTION", SourceOrderID: center.JoinOrderID, EffectiveAt: firstNonEmptyString(center.ApprovedAt, center.CreatedAt),
			IdentityVersion: 1, CreatedBy: "legacy_projection", CreatedAt: center.CreatedAt, UpdatedAt: center.UpdatedAt,
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].EffectiveAt > items[j].EffectiveAt })
	return items
}

func normalizeLegacyIdentityStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "ACTIVE", "FROZEN", "PENDING", "TERMINATED":
		return strings.ToUpper(strings.TrimSpace(status))
	case "DISABLED", "INACTIVE", "CLOSED":
		return "TERMINATED"
	default:
		return "PENDING"
	}
}

func operationCenterByID(items []adminOperationCenter, id string) adminOperationCenter {
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	return adminOperationCenter{}
}
