package httpserver

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func (s *jsonStore) ListAdminEnterprises(query adminEnterpriseListQuery) (adminEnterpriseListResult, error) {
	data, err := s.AdminData()
	if err != nil {
		return adminEnterpriseListResult{}, err
	}
	return buildMemoryAdminEnterpriseList(data, query), nil
}

func (s *jsonStore) GetAdminEnterprise(id string) (adminEnterpriseDetail, error) {
	data, err := s.AdminData()
	if err != nil {
		return adminEnterpriseDetail{}, err
	}
	return buildMemoryAdminEnterpriseDetail(data, strings.TrimSpace(id))
}

func (s *jsonStore) CreateAdminEnterprise(actorID string, actorRole string, request adminEnterpriseCreateRequest) (adminEnterpriseDetail, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.EnterpriseCode = strings.ToUpper(strings.TrimSpace(request.EnterpriseCode))
	request.OwnerUserID = strings.TrimSpace(request.OwnerUserID)
	request.PlanID = strings.TrimSpace(request.PlanID)
	request.PlanCode = strings.TrimSpace(request.PlanCode)
	request.SourceAgentID = strings.TrimSpace(request.SourceAgentID)
	request.OperationCenterID = strings.TrimSpace(request.OperationCenterID)
	if request.Name == "" || len([]rune(request.Name)) > 160 {
		return adminEnterpriseDetail{}, fmt.Errorf("%w: enterprise name is required and must not exceed 160 characters", errEnterpriseInvalid)
	}
	if request.SeatLimit <= 0 {
		request.SeatLimit = 20
	}
	if request.SeatLimit > 100000 {
		return adminEnterpriseDetail{}, fmt.Errorf("%w: seatLimit is too large", errEnterpriseInvalid)
	}
	var createdID string
	err := s.updateAdmin(func(data *adminPlatformData) error {
		normalizeEnterpriseMemoryState(&data.Enterprise)
		if request.OwnerUserID != "" {
			if _, found := memoryActiveUser(*data, request.OwnerUserID); !found {
				return fmt.Errorf("%w: owner user not found", errEnterpriseInvalid)
			}
		}
		if request.SourceAgentID != "" && !memoryAdminEnterpriseAgentExists(*data, request.SourceAgentID) {
			return fmt.Errorf("%w: source agent not found", errEnterpriseInvalid)
		}
		if request.OperationCenterID != "" && !memoryAdminEnterpriseOperationCenterExists(*data, request.OperationCenterID) {
			return fmt.Errorf("%w: operation center not found", errEnterpriseInvalid)
		}
		for _, tenant := range data.Enterprise.Tenants {
			code := strings.ToUpper(stringValue(tenant.Config["enterpriseCode"]))
			if strings.EqualFold(tenant.Name, request.Name) || (request.EnterpriseCode != "" && code == request.EnterpriseCode) {
				return fmt.Errorf("%w: enterprise name or code already exists", errEnterpriseConflict)
			}
		}
		now := enterpriseNow()
		createdID = newEnterpriseResourceID("tenant")
		organizationID := newEnterpriseResourceID("organization")
		if request.EnterpriseCode == "" {
			request.EnterpriseCode = strings.ToUpper("ENT-" + strings.TrimPrefix(createdID, "tenant_"))
		}
		if request.PlanCode == "" {
			request.PlanCode = "enterprise_trial"
		}
		config := map[string]any{
			"enterpriseCode":      request.EnterpriseCode,
			"seatLimit":           request.SeatLimit,
			"industry":            strings.TrimSpace(request.Industry),
			"companySize":         strings.TrimSpace(request.CompanySize),
			"sourceAgentId":       request.SourceAgentID,
			"operationCenterId":   request.OperationCenterID,
			"certificationStatus": "UNVERIFIED",
		}
		tenant := enterpriseTenant{ID: createdID, Name: request.Name, OwnerUserID: request.OwnerUserID, Status: "ACTIVE", CertificationStatus: "UNVERIFIED", Config: config, CreatedAt: now, UpdatedAt: now}
		organization := enterpriseOrganization{ID: organizationID, TenantID: createdID, OrganizationType: "ROOT", Name: request.Name, Status: "ACTIVE", Metadata: map[string]any{"root": true}, CreatedAt: now, UpdatedAt: now}
		data.Enterprise.Tenants = append(data.Enterprise.Tenants, tenant)
		data.Enterprise.Organizations = append(data.Enterprise.Organizations, organization)
		data.Enterprise.Wallets[createdID] = enterpriseWalletSummary{Status: "ACTIVE"}
		data.Enterprise.Subscriptions[createdID] = enterpriseSubscriptionSummary{
			ID: newEnterpriseResourceID("tenant_subscription"), PlanID: request.PlanID, PlanCode: request.PlanCode,
			Status: "TRIALING", TrialExpiresAt: time.Now().UTC().Add(14 * 24 * time.Hour).Format(time.RFC3339Nano), Entitlements: defaultEnterpriseTrialEntitlements(),
		}
		if request.OwnerUserID != "" {
			owner, _ := memoryActiveUser(*data, request.OwnerUserID)
			data.Enterprise.Members = append(data.Enterprise.Members, enterpriseMember{
				ID: newEnterpriseResourceID("tenant_member"), TenantID: createdID, UserID: owner.ID, Name: owner.Name, Email: owner.Email,
				PrimaryOrganizationID: organizationID, OrganizationName: organization.Name, MemberStatus: "ACTIVE",
				CertificationStatus: "UNVERIFIED", DataScope: dataScopeTenantAll, Roles: []string{roleEnterpriseAdmin}, JoinedAt: now, CreatedAt: now, UpdatedAt: now,
			})
		}
		appendMemoryEnterpriseAudit(&data.Enterprise, enterpriseAccess{UserID: actorID, TenantID: createdID, OrganizationID: organizationID, Role: actorRole}, "admin.enterprise.create", "tenant", createdID, request.OwnerUserID, map[string]any{"name": request.Name, "enterpriseCode": request.EnterpriseCode})
		return nil
	})
	if err != nil {
		return adminEnterpriseDetail{}, err
	}
	return s.GetAdminEnterprise(createdID)
}

func buildMemoryAdminEnterpriseList(data adminPlatformData, query adminEnterpriseListQuery) adminEnterpriseListResult {
	items := make([]adminEnterpriseListItem, 0, len(data.Enterprise.Tenants))
	for _, tenant := range data.Enterprise.Tenants {
		item := memoryAdminEnterpriseListItem(data, tenant)
		if matchesAdminEnterpriseQuery(item, query) {
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CreatedAt != items[j].CreatedAt {
			return items[i].CreatedAt > items[j].CreatedAt
		}
		return items[i].ID > items[j].ID
	})
	total := len(items)
	start := (query.Page - 1) * query.PageSize
	if start > total {
		start = total
	}
	end := start + query.PageSize
	if end > total {
		end = total
	}
	return adminEnterpriseListResult{
		Items: append([]adminEnterpriseListItem(nil), items[start:end]...), Total: total, Page: query.Page, PageSize: query.PageSize,
		Stats: memoryAdminEnterpriseStats(data), Filters: memoryAdminEnterpriseFilters(data),
	}
}

func memoryAdminEnterpriseListItem(data adminPlatformData, tenant enterpriseTenant) adminEnterpriseListItem {
	config := tenant.Config
	if config == nil {
		config = map[string]any{}
	}
	memberCount := 0
	activeMembers := 0
	for _, member := range data.Enterprise.Members {
		if member.TenantID != tenant.ID {
			continue
		}
		memberCount++
		if strings.EqualFold(member.MemberStatus, "ACTIVE") {
			activeMembers++
		}
	}
	subscription := data.Enterprise.Subscriptions[tenant.ID]
	planName := subscription.PlanCode
	for _, plan := range data.Plans {
		if plan.ID == subscription.PlanID || (subscription.PlanCode != "" && plan.Code == subscription.PlanCode) {
			planName = plan.Name
			break
		}
	}
	if planName == "" {
		planName = "试用版"
	}
	wallet := data.Enterprise.Wallets[tenant.ID]
	return adminEnterpriseListItem{
		ID: tenant.ID, EnterpriseCode: firstNonEmptyString(stringValue(config["enterpriseCode"]), tenant.ID), Name: tenant.Name,
		CertificationStatus: firstNonEmptyString(tenant.CertificationStatus, stringValue(config["certificationStatus"]), "UNVERIFIED"),
		Plan:                adminEnterprisePlanSummary{ID: subscription.PlanID, Code: subscription.PlanCode, Name: planName, Status: subscription.Status, ExpiresAt: subscription.TrialExpiresAt},
		MemberCount:         memberCount, ActiveMemberCount: activeMembers, SeatLimit: maxEnterpriseInt(intValue(config["seatLimit"]), 20),
		Compute:         adminEnterpriseComputeSummary{Balance: wallet.PointBalance, Frozen: wallet.FrozenPoints, Unit: "POINT"},
		SourceAgent:     memoryAdminEnterpriseSourceAgent(data, stringValue(config["sourceAgentId"])),
		OperationCenter: memoryAdminEnterpriseOperationCenter(data, stringValue(config["operationCenterId"])),
		Status:          firstNonEmptyString(tenant.Status, "ACTIVE"), CreatedAt: tenant.CreatedAt, UpdatedAt: tenant.UpdatedAt,
	}
}

func buildMemoryAdminEnterpriseDetail(data adminPlatformData, id string) (adminEnterpriseDetail, error) {
	tenant, found := memoryTenant(data.Enterprise, id)
	if !found {
		return adminEnterpriseDetail{}, errEnterpriseNotFound
	}
	item := memoryAdminEnterpriseListItem(data, tenant)
	profile := adminEnterpriseProfile{Industry: stringValue(tenant.Config["industry"]), CompanySize: stringValue(tenant.Config["companySize"]), OwnerUserID: tenant.OwnerUserID}
	for index := len(data.Enterprise.Certifications) - 1; index >= 0; index-- {
		certification := data.Enterprise.Certifications[index]
		if certification.TenantID == tenant.ID {
			profile.LegalName = certification.LegalName
			profile.UnifiedSocialCreditCode = certification.UnifiedSocialCreditCode
			profile.LegalRepresentativeName = certification.LegalRepresentativeName
			break
		}
	}
	organizationCount := 0
	for _, organization := range data.Enterprise.Organizations {
		if organization.TenantID == tenant.ID && !strings.EqualFold(organization.Status, "DELETED") {
			organizationCount++
		}
	}
	recent := make([]adminEnterpriseRecentOperation, 0, 5)
	for index := len(data.Enterprise.AuditLogs) - 1; index >= 0 && len(recent) < 5; index-- {
		log := data.Enterprise.AuditLogs[index]
		if log.TenantID != tenant.ID {
			continue
		}
		recent = append(recent, adminEnterpriseRecentOperation{ID: log.ID, Actor: firstNonEmptyString(log.ActorRole, log.ActorUserID), Action: log.Action, Summary: stringValue(log.Metadata["summary"]), CreatedAt: log.CreatedAt})
	}
	return adminEnterpriseDetail{Enterprise: item, Profile: profile, OrganizationCount: organizationCount, RecentOperations: recent, Privacy: adminEnterprisePrivacyBoundary()}, nil
}

func matchesAdminEnterpriseQuery(item adminEnterpriseListItem, query adminEnterpriseListQuery) bool {
	keyword := strings.ToLower(query.Keyword)
	if keyword != "" && !strings.Contains(strings.ToLower(item.Name), keyword) && !strings.Contains(strings.ToLower(item.ID), keyword) && !strings.Contains(strings.ToLower(item.EnterpriseCode), keyword) {
		return false
	}
	if query.Certification != "" && !strings.EqualFold(item.CertificationStatus, query.Certification) {
		return false
	}
	if query.PlanCode != "" && item.Plan.Code != query.PlanCode {
		return false
	}
	if query.Status != "" && !strings.EqualFold(item.Status, query.Status) {
		return false
	}
	if query.SourceAgentID != "" && item.SourceAgent.ID != query.SourceAgentID {
		return false
	}
	if query.OperationCenterID != "" && item.OperationCenter.ID != query.OperationCenterID {
		return false
	}
	createdAt, _ := time.Parse(time.RFC3339Nano, item.CreatedAt)
	if query.CreatedFrom != nil && createdAt.Before(*query.CreatedFrom) {
		return false
	}
	if query.CreatedTo != nil && createdAt.After(*query.CreatedTo) {
		return false
	}
	return true
}

func memoryAdminEnterpriseStats(data adminPlatformData) adminEnterpriseStats {
	result := adminEnterpriseStats{Total: len(data.Enterprise.Tenants)}
	monthStart := time.Now().UTC()
	monthStart = time.Date(monthStart.Year(), monthStart.Month(), 1, 0, 0, 0, 0, time.UTC)
	for _, tenant := range data.Enterprise.Tenants {
		if strings.EqualFold(tenant.CertificationStatus, "APPROVED") || strings.EqualFold(tenant.CertificationStatus, "CERTIFIED") {
			result.Certified++
		}
		if !strings.EqualFold(tenant.Status, "ACTIVE") {
			result.Abnormal++
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, tenant.CreatedAt)
		if !createdAt.Before(monthStart) {
			result.CreatedThisMonth++
		}
	}
	return result
}

func memoryAdminEnterpriseFilters(data adminPlatformData) adminEnterpriseFilters {
	result := adminEnterpriseFilters{}
	for _, plan := range data.Plans {
		if plan.Active {
			result.Plans = append(result.Plans, adminEnterpriseFilterOption{Value: plan.Code, Label: plan.Name})
		}
	}
	users := userMap(data.Users)
	for _, agent := range data.ChannelAgents {
		name := agent.ID
		if user, ok := users[agent.UserID]; ok {
			name = user.Name
		}
		result.SourceAgents = append(result.SourceAgents, adminEnterpriseFilterOption{Value: agent.ID, Label: name})
	}
	for _, center := range data.OperationCenters {
		result.OperationCenters = append(result.OperationCenters, adminEnterpriseFilterOption{Value: center.ID, Label: center.Name})
	}
	return result
}

func memoryAdminEnterpriseSourceAgent(data adminPlatformData, id string) adminEnterpriseRelationSummary {
	if id == "" {
		return adminEnterpriseRelationSummary{}
	}
	users := userMap(data.Users)
	for _, agent := range data.ChannelAgents {
		if agent.ID == id {
			name := agent.ID
			if user, ok := users[agent.UserID]; ok {
				name = user.Name
			}
			return adminEnterpriseRelationSummary{ID: id, Name: name}
		}
	}
	return adminEnterpriseRelationSummary{ID: id}
}

func memoryAdminEnterpriseOperationCenter(data adminPlatformData, id string) adminEnterpriseRelationSummary {
	if id == "" {
		return adminEnterpriseRelationSummary{}
	}
	for _, center := range data.OperationCenters {
		if center.ID == id {
			return adminEnterpriseRelationSummary{ID: id, Name: center.Name}
		}
	}
	return adminEnterpriseRelationSummary{ID: id}
}

func memoryAdminEnterpriseAgentExists(data adminPlatformData, id string) bool {
	for _, item := range data.ChannelAgents {
		if item.ID == id {
			return true
		}
	}
	return false
}

func memoryAdminEnterpriseOperationCenterExists(data adminPlatformData, id string) bool {
	for _, item := range data.OperationCenters {
		if item.ID == id {
			return true
		}
	}
	return false
}

func maxEnterpriseInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
