package httpserver

import (
	"fmt"
	"sort"
	"strings"
)

func (s *jsonStore) ListAdminEnterpriseCertifications() (adminEnterpriseSectionResult, error) {
	data, err := s.AdminData()
	if err != nil {
		return adminEnterpriseSectionResult{}, err
	}
	items := make([]map[string]any, 0, len(data.Enterprise.Certifications))
	for _, certification := range data.Enterprise.Certifications {
		tenant, found := memoryTenant(data.Enterprise, certification.TenantID)
		if !found {
			continue
		}
		items = append(items, map[string]any{
			"id": certification.ID, "enterpriseId": tenant.ID, "enterpriseName": tenant.Name,
			"legalName": certification.LegalName, "unifiedSocialCreditCode": maskEnterpriseCreditCode(certification.UnifiedSocialCreditCode),
			"legalRepresentativeName": certification.LegalRepresentativeName, "status": certification.Status,
			"submittedBy": certification.SubmittedBy, "reviewedBy": certification.ReviewedBy,
			"reviewedAt": certification.ReviewedAt, "reviewComment": certification.ReviewComment,
			"createdAt": certification.CreatedAt, "updatedAt": certification.UpdatedAt,
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return stringValue(items[i]["updatedAt"]) > stringValue(items[j]["updatedAt"]) })
	return adminEnterpriseSectionResult{Section: "certifications", Items: items, Total: len(items), Summary: countEnterpriseStatuses(items), Privacy: adminEnterprisePrivacyBoundary()}, nil
}

func (s *jsonStore) GetAdminEnterpriseSection(id string, section string) (adminEnterpriseSectionResult, error) {
	data, err := s.AdminData()
	if err != nil {
		return adminEnterpriseSectionResult{}, err
	}
	tenant, found := memoryTenant(data.Enterprise, strings.TrimSpace(id))
	if !found {
		return adminEnterpriseSectionResult{}, errEnterpriseNotFound
	}
	result := adminEnterpriseSectionResult{Section: section, Enterprise: adminEnterpriseRelationSummary{ID: tenant.ID, Name: tenant.Name}, Summary: map[string]any{}, Items: []map[string]any{}, Privacy: adminEnterprisePrivacyBoundary()}
	members := memoryAdminEnterpriseMembers(data, tenant.ID)
	memberIDs := map[string]bool{}
	for _, member := range data.Enterprise.Members {
		if member.TenantID == tenant.ID {
			memberIDs[member.UserID] = true
		}
	}
	switch section {
	case "certifications":
		for _, item := range data.Enterprise.Certifications {
			if item.TenantID == tenant.ID {
				result.Items = append(result.Items, map[string]any{"id": item.ID, "legalName": item.LegalName, "unifiedSocialCreditCode": maskEnterpriseCreditCode(item.UnifiedSocialCreditCode), "legalRepresentativeName": item.LegalRepresentativeName, "status": item.Status, "reviewComment": item.ReviewComment, "createdAt": item.CreatedAt, "updatedAt": item.UpdatedAt})
			}
		}
		result.Summary = countEnterpriseStatuses(result.Items)
	case "members":
		result.Items = members
		organizations := 0
		organizationItems := []map[string]any{}
		for _, item := range data.Enterprise.Organizations {
			if item.TenantID == tenant.ID && !strings.EqualFold(item.Status, "DELETED") {
				organizations++
				memberCount := 0
				for _, member := range data.Enterprise.Members {
					if member.TenantID == tenant.ID && member.PrimaryOrganizationID == item.ID {
						memberCount++
					}
				}
				organizationItems = append(organizationItems, map[string]any{"id": item.ID, "parentId": item.ParentID, "name": item.Name, "organizationType": item.OrganizationType, "status": item.Status, "memberCount": memberCount})
			}
		}
		result.Summary = map[string]any{"memberCount": len(members), "activeMembers": countMemoryMapStatus(members, "ACTIVE"), "seatLimit": maxEnterpriseInt(intValue(tenant.Config["seatLimit"]), 20), "organizationCount": organizations, "organizations": organizationItems}
	case "package":
		subscription := data.Enterprise.Subscriptions[tenant.ID]
		result.Items = []map[string]any{{"id": subscription.ID, "planId": subscription.PlanID, "planCode": subscription.PlanCode, "status": subscription.Status, "expiresAt": subscription.TrialExpiresAt, "entitlements": subscription.Entitlements}}
		result.Summary = map[string]any{"planCode": subscription.PlanCode, "status": subscription.Status, "expiresAt": subscription.TrialExpiresAt, "seatLimit": maxEnterpriseInt(intValue(tenant.Config["seatLimit"]), 20), "seatUsed": len(members)}
	case "compute", "transactions":
		wallet := data.Enterprise.Wallets[tenant.ID]
		for _, transaction := range data.Enterprise.PointTransactions {
			if transaction.TenantID != tenant.ID {
				continue
			}
			result.Items = append(result.Items, map[string]any{
				"id": transaction.ID, "type": transaction.TransactionType, "pointDelta": transaction.PointDelta,
				"balanceBefore": transaction.BalanceAfter - transaction.PointDelta, "balanceAfter": transaction.BalanceAfter,
				"referenceType": transaction.ReferenceType, "referenceId": transaction.ReferenceID,
				"reason": transaction.Reason, "actorUserId": transaction.ActorUserID,
				"requestId": transaction.RequestID, "createdAt": transaction.CreatedAt,
			})
		}
		for _, event := range data.BillingEvents {
			if event.TenantID != tenant.ID {
				continue
			}
			result.Items = append(result.Items, map[string]any{"id": event.ID, "type": event.MetricCode, "pointDelta": -event.PointCost, "balanceBefore": event.BalanceBefore, "balanceAfter": event.BalanceAfter, "status": event.Status, "referenceId": event.TaskID, "createdAt": event.OccurredAt})
		}
		result.Summary = map[string]any{"balance": wallet.PointBalance, "frozen": wallet.FrozenPoints, "cashBalanceCents": wallet.CashBalanceCents, "status": wallet.Status}
		result.Unit = "POINT"
	case "orders":
		for _, order := range data.Orders {
			if !memoryAdminOrderBelongsToEnterprise(data, order, tenant.ID) {
				continue
			}
			result.Items = append(result.Items, map[string]any{"id": order.ID, "orderNo": order.OrderNo, "orderType": firstNonEmptyString(order.BusinessOrderType, order.OrderType), "planId": order.PlanID, "amountCents": order.AmountCents, "status": order.Status, "paidAt": order.PaidAt, "createdAt": order.CreatedAt})
		}
		result.Summary = map[string]any{"orderCount": len(result.Items), "amountCents": sumEnterpriseOrderAmount(result.Items)}
		result.Unit = "CNY_CENT"
	case "ai-capabilities":
		for _, limit := range data.TenantModuleLimits {
			if firstNonEmptyString(limit.TenantID, limit.TenantIDCamel) != tenant.ID {
				continue
			}
			result.Items = append(result.Items, map[string]any{"id": limit.ID, "moduleCode": firstNonEmptyString(limit.ModuleCode, limit.ModuleCodeCamel), "modelName": firstNonEmptyString(limit.ModelName, limit.ModelNameCamel), "status": limit.Status, "limits": limit.LimitJSON})
		}
		result.Summary = map[string]any{"enabledCount": countMemoryMapStatus(result.Items, "ACTIVE"), "configuredCount": len(result.Items)}
	case "ai-employees":
		for _, agent := range data.Agents {
			if !memberIDs[agent.OwnerID] {
				continue
			}
			result.Items = append(result.Items, map[string]any{"id": agent.ID, "name": agent.Name, "status": agent.Status, "callCount": agent.CallCount, "version": agent.Version, "updatedAt": agent.UpdatedAt})
		}
		result.Summary = map[string]any{"total": len(result.Items), "active": countMemoryMapStatus(result.Items, "ACTIVE")}
	case "knowledge-bases":
		// JSON fallback does not persist knowledge content in adminPlatformData. Return summary only.
		result.Summary = map[string]any{"knowledgeBaseCount": 0, "documentCount": 0, "chunkCount": 0, "storageBytes": 0, "summaryOnly": true}
	case "attribution", "relationships":
		item := memoryAdminEnterpriseListItem(data, tenant)
		result.Items = []map[string]any{{"sourceAgent": item.SourceAgent, "operationCenter": item.OperationCenter, "status": tenant.Status, "updatedAt": tenant.UpdatedAt}}
		result.Summary = map[string]any{"sourceAgent": item.SourceAgent, "operationCenter": item.OperationCenter}
	case "risk":
		for _, audit := range data.Enterprise.AuditLogs {
			if audit.TenantID == tenant.ID && strings.HasPrefix(audit.Action, "admin.enterprise.risk") {
				result.Items = append(result.Items, map[string]any{"id": audit.ID, "action": audit.Action, "status": audit.Status, "reason": stringValue(audit.Metadata["reason"]), "actor": audit.ActorRole, "createdAt": audit.CreatedAt})
			}
		}
		result.Summary = map[string]any{"enterpriseStatus": tenant.Status, "riskRecordCount": len(result.Items)}
	case "audit-logs":
		for index := len(data.Enterprise.AuditLogs) - 1; index >= 0; index-- {
			audit := data.Enterprise.AuditLogs[index]
			if audit.TenantID != tenant.ID {
				continue
			}
			result.Items = append(result.Items, map[string]any{"id": audit.ID, "actorUserId": audit.ActorUserID, "actorRole": audit.ActorRole, "action": audit.Action, "resourceType": audit.ResourceType, "resourceId": audit.ResourceID, "status": audit.Status, "metadata": sanitizeEnterpriseAuditMetadata(audit.Metadata), "createdAt": audit.CreatedAt})
		}
		result.Summary = map[string]any{"total": len(result.Items)}
	default:
		return adminEnterpriseSectionResult{}, fmt.Errorf("%w: unsupported enterprise section", errEnterpriseInvalid)
	}
	result.Total = len(result.Items)
	return result, nil
}

func memoryAdminOrderBelongsToEnterprise(data adminPlatformData, order adminOrder, tenantID string) bool {
	explicitTenantID := firstNonEmptyString(order.TenantID, stringValue(order.PriceSnapshot["tenantId"]))
	if explicitTenantID != "" {
		return explicitTenantID == tenantID
	}
	memberships := map[string]bool{}
	for _, member := range data.Enterprise.Members {
		if member.UserID != order.UserID || !strings.EqualFold(member.MemberStatus, "ACTIVE") {
			continue
		}
		if _, found := memoryTenant(data.Enterprise, member.TenantID); found {
			memberships[member.TenantID] = true
		}
	}
	return len(memberships) == 1 && memberships[tenantID]
}

func (s *jsonStore) MutateAdminEnterprise(actorID string, actorRole string, id string, request adminEnterpriseMutationRequest) (adminEnterpriseMutationResult, error) {
	if err := validateAdminEnterpriseMutation(request); err != nil {
		return adminEnterpriseMutationResult{}, err
	}
	result := adminEnterpriseMutationResult{RequestID: request.RequestID, Action: request.Action, Enterprise: id, Status: "SUCCEEDED"}
	err := s.updateAdmin(func(data *adminPlatformData) error {
		normalizeEnterpriseMemoryState(&data.Enterprise)
		tenantIndex := -1
		for index := range data.Enterprise.Tenants {
			if data.Enterprise.Tenants[index].ID == id {
				tenantIndex = index
				break
			}
		}
		if tenantIndex < 0 {
			return errEnterpriseNotFound
		}
		for _, audit := range data.Enterprise.AuditLogs {
			if audit.TenantID == id && stringValue(audit.Metadata["requestId"]) == request.RequestID {
				return fmt.Errorf("%w: duplicate requestId", errEnterpriseConflict)
			}
		}
		tenant := &data.Enterprise.Tenants[tenantIndex]
		if tenant.Config == nil {
			tenant.Config = map[string]any{}
		}
		result.Before = map[string]any{}
		result.After = map[string]any{}
		switch request.Action {
		case "profile-update":
			name := strings.TrimSpace(request.Name)
			if name == "" {
				return fmt.Errorf("%w: enterprise name is required", errEnterpriseInvalid)
			}
			result.Before = map[string]any{"name": tenant.Name, "industry": stringValue(tenant.Config["industry"]), "companySize": stringValue(tenant.Config["companySize"])}
			oldName := tenant.Name
			tenant.Name = name
			tenant.Config["industry"], tenant.Config["companySize"] = strings.TrimSpace(request.Industry), strings.TrimSpace(request.CompanySize)
			for index := range data.Enterprise.Organizations {
				organization := &data.Enterprise.Organizations[index]
				if organization.TenantID == id && strings.EqualFold(organization.OrganizationType, "ROOT") && organization.Name == oldName {
					organization.Name = name
					organization.UpdatedAt = enterpriseNow()
				}
			}
			result.After = map[string]any{"name": tenant.Name, "industry": stringValue(tenant.Config["industry"]), "companySize": stringValue(tenant.Config["companySize"])}
		case "certification-review":
			status := strings.ToUpper(strings.TrimSpace(request.Status))
			if status != "APPROVED" && status != "REJECTED" {
				return fmt.Errorf("%w: certification status must be APPROVED or REJECTED", errEnterpriseInvalid)
			}
			certificationIndex := -1
			for index := len(data.Enterprise.Certifications) - 1; index >= 0; index-- {
				if data.Enterprise.Certifications[index].TenantID == id {
					certificationIndex = index
					break
				}
			}
			if certificationIndex < 0 {
				return fmt.Errorf("%w: pending certification not found", errEnterpriseInvalid)
			}
			certification := &data.Enterprise.Certifications[certificationIndex]
			if !strings.EqualFold(certification.Status, "PENDING") {
				return fmt.Errorf("%w: only pending certification can be reviewed", errEnterpriseConflict)
			}
			result.Before = map[string]any{"status": certification.Status}
			certification.Status, certification.ReviewedBy, certification.ReviewedAt, certification.ReviewComment, certification.UpdatedAt = status, actorID, enterpriseNow(), strings.TrimSpace(request.ReviewComment), enterpriseNow()
			tenant.CertificationStatus, tenant.Config["certificationStatus"] = status, status
			result.After = map[string]any{"status": status}
		case "package-adjust":
			subscription := data.Enterprise.Subscriptions[id]
			result.Before = map[string]any{"planId": subscription.PlanID, "planCode": subscription.PlanCode, "expiresAt": subscription.TrialExpiresAt}
			if request.PlanID != "" {
				subscription.PlanID = request.PlanID
			}
			if request.PlanCode != "" {
				subscription.PlanCode = request.PlanCode
			}
			if request.ExpiresAt != "" {
				subscription.TrialExpiresAt = request.ExpiresAt
			}
			subscription.Status = "ACTIVE"
			data.Enterprise.Subscriptions[id] = subscription
			result.After = map[string]any{"planId": subscription.PlanID, "planCode": subscription.PlanCode, "expiresAt": subscription.TrialExpiresAt}
		case "seat-adjust":
			if request.SeatLimit < 1 || request.SeatLimit > 100000 {
				return fmt.Errorf("%w: seatLimit must be between 1 and 100000", errEnterpriseInvalid)
			}
			activeMembers := 0
			for _, member := range data.Enterprise.Members {
				if member.TenantID == id && strings.EqualFold(member.MemberStatus, "ACTIVE") {
					activeMembers++
				}
			}
			if request.SeatLimit < activeMembers {
				return fmt.Errorf("%w: seatLimit cannot be lower than active member count", errEnterpriseInvalid)
			}
			result.Before = map[string]any{"seatLimit": maxEnterpriseInt(intValue(tenant.Config["seatLimit"]), 20)}
			tenant.Config["seatLimit"] = request.SeatLimit
			result.After = map[string]any{"seatLimit": request.SeatLimit}
		case "compute-adjust", "recharge":
			if request.PointDelta == 0 {
				return fmt.Errorf("%w: pointDelta must not be zero", errEnterpriseInvalid)
			}
			wallet := data.Enterprise.Wallets[id]
			result.Before = map[string]any{"balance": wallet.PointBalance, "unit": "POINT"}
			if wallet.PointBalance+request.PointDelta < 0 {
				return fmt.Errorf("%w: insufficient enterprise compute balance", errEnterpriseInvalid)
			}
			wallet.PointBalance += request.PointDelta
			data.Enterprise.Wallets[id] = wallet
			data.Enterprise.PointTransactions = append(data.Enterprise.PointTransactions, enterprisePointTransaction{
				ID: newEnterpriseResourceID("tenant_point_transaction"), TenantID: id,
				TransactionType: strings.ToUpper(request.Action), PointDelta: request.PointDelta,
				BalanceAfter: wallet.PointBalance, ReferenceType: "ADMIN_OPERATION", ReferenceID: request.RequestID,
				Reason: request.Reason, ActorUserID: actorID, RequestID: request.RequestID, CreatedAt: enterpriseNow(),
			})
			result.After = map[string]any{"balance": wallet.PointBalance, "unit": "POINT"}
		case "ai-capability-configure":
			moduleCode := strings.TrimSpace(request.ModuleCode)
			status := strings.ToUpper(strings.TrimSpace(request.Status))
			if moduleCode == "" {
				return fmt.Errorf("%w: moduleCode is required", errEnterpriseInvalid)
			}
			if status == "" {
				status = "ACTIVE"
			}
			if status != "ACTIVE" && status != "DISABLED" {
				return fmt.Errorf("%w: AI capability status must be ACTIVE or DISABLED", errEnterpriseInvalid)
			}
			limitIndex := -1
			for index := range data.TenantModuleLimits {
				limit := data.TenantModuleLimits[index]
				if firstNonEmptyString(limit.TenantID, limit.TenantIDCamel) == id && firstNonEmptyString(limit.ModuleCode, limit.ModuleCodeCamel) == moduleCode && firstNonEmptyString(limit.ModelName, limit.ModelNameCamel) == strings.TrimSpace(request.ModelName) {
					limitIndex = index
					break
				}
			}
			result.Before = map[string]any{"configured": false}
			now := enterpriseNow()
			if limitIndex >= 0 {
				limit := &data.TenantModuleLimits[limitIndex]
				result.Before = map[string]any{"moduleCode": firstNonEmptyString(limit.ModuleCode, limit.ModuleCodeCamel), "modelName": firstNonEmptyString(limit.ModelName, limit.ModelNameCamel), "status": limit.Status, "limits": limit.LimitJSON}
				limit.Status, limit.LimitJSON, limit.UpdatedAt = status, request.Limits, now
			} else {
				data.TenantModuleLimits = append(data.TenantModuleLimits, adminTenantModuleLimit{ID: newEnterpriseResourceID("tenant_module_limit"), TenantID: id, ModuleCode: moduleCode, ModelName: strings.TrimSpace(request.ModelName), LimitJSON: request.Limits, Status: status, CreatedAt: now, UpdatedAt: now})
			}
			result.After = map[string]any{"moduleCode": moduleCode, "modelName": strings.TrimSpace(request.ModelName), "status": status, "limits": request.Limits}
		case "attribution-change":
			result.Before = map[string]any{"sourceAgentId": stringValue(tenant.Config["sourceAgentId"]), "operationCenterId": stringValue(tenant.Config["operationCenterId"])}
			result.After = map[string]any{"sourceAgentId": request.SourceAgentID, "operationCenterId": request.OperationCenterID}
			if !strings.EqualFold(actorRole, "SUPER_ADMIN") {
				result.Status = "PENDING_APPROVAL"
				result.Message = "归属变更已提交审批，审批通过前不会修改当前归属"
			} else {
				tenant.Config["sourceAgentId"], tenant.Config["operationCenterId"] = request.SourceAgentID, request.OperationCenterID
			}
		case "risk-disable":
			result.Before = map[string]any{"status": tenant.Status}
			tenant.Status = "SUSPENDED"
			result.After = map[string]any{"status": tenant.Status}
		case "risk-restore":
			result.Before = map[string]any{"status": tenant.Status}
			tenant.Status = "ACTIVE"
			result.After = map[string]any{"status": tenant.Status}
		default:
			return fmt.Errorf("%w: unsupported enterprise action", errEnterpriseInvalid)
		}
		tenant.UpdatedAt = enterpriseNow()
		metadata := map[string]any{"requestId": request.RequestID, "reason": request.Reason, "before": result.Before, "after": result.After, "resultStatus": result.Status, "summary": request.Reason}
		appendMemoryEnterpriseAudit(&data.Enterprise, enterpriseAccess{UserID: actorID, TenantID: id, Role: actorRole}, "admin.enterprise."+request.Action, "tenant", id, "", metadata)
		result.AuditID = data.Enterprise.AuditLogs[len(data.Enterprise.AuditLogs)-1].ID
		if result.Message == "" {
			result.Message = "操作成功，审计记录已写入"
		}
		return nil
	})
	return result, err
}

func memoryAdminEnterpriseMembers(data adminPlatformData, tenantID string) []map[string]any {
	items := []map[string]any{}
	for _, member := range data.Enterprise.Members {
		if member.TenantID != tenantID {
			continue
		}
		items = append(items, map[string]any{"id": member.ID, "userId": member.UserID, "name": member.Name, "email": maskEnterpriseEmail(member.Email), "organizationId": member.PrimaryOrganizationID, "organizationName": member.OrganizationName, "roles": member.Roles, "dataScope": member.DataScope, "status": member.MemberStatus, "joinedAt": member.JoinedAt})
	}
	return items
}

func countEnterpriseStatuses(items []map[string]any) map[string]any {
	result := map[string]any{"total": len(items), "pending": 0, "approved": 0, "rejected": 0}
	for _, item := range items {
		key := strings.ToLower(stringValue(item["status"]))
		if key == "verified" {
			key = "approved"
		}
		if _, found := result[key]; found {
			result[key] = intValue(result[key]) + 1
		}
	}
	return result
}

func countMemoryMapStatus(items []map[string]any, status string) int {
	count := 0
	for _, item := range items {
		if strings.EqualFold(stringValue(item["status"]), status) {
			count++
		}
	}
	return count
}

func sumEnterpriseOrderAmount(items []map[string]any) int64 {
	var total int64
	for _, item := range items {
		total += int64(intValue(item["amountCents"]))
	}
	return total
}

func maskEnterpriseEmail(value string) string {
	parts := strings.Split(strings.TrimSpace(value), "@")
	if len(parts) != 2 || len(parts[0]) < 2 {
		return value
	}
	return parts[0][:1] + "***@" + parts[1]
}

func maskEnterpriseCreditCode(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return value
	}
	return value[:4] + "********" + value[len(value)-4:]
}

func sanitizeEnterpriseAuditMetadata(metadata map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range metadata {
		switch strings.ToLower(key) {
		case "prompt", "content", "input", "output", "document", "conversation":
			continue
		default:
			result[key] = value
		}
	}
	return result
}
