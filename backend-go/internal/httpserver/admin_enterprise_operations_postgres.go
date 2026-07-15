package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (s *postgresStore) ListAdminEnterpriseCertifications() (adminEnterpriseSectionResult, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminEnterpriseSectionResult{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT certification.id,certification.tenant_id,tenant.name,certification.legal_name,
		       certification.unified_social_credit_code,certification.legal_representative_name,
		       certification.status,certification.submitted_by,coalesce(certification.reviewed_by,''),
		       certification.reviewed_at,certification.review_comment,certification.created_at,certification.updated_at
		FROM xz_tenant_certifications certification
		JOIN xz_tenants tenant ON tenant.id=certification.tenant_id AND tenant.tenant_type='ENTERPRISE'
		ORDER BY certification.updated_at DESC,certification.id DESC
	`)
	if err != nil {
		return adminEnterpriseSectionResult{}, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, tenantID, tenantName, legalName, creditCode, representative, status, submittedBy, reviewedBy, comment string
		var reviewedAt *time.Time
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &tenantID, &tenantName, &legalName, &creditCode, &representative, &status, &submittedBy, &reviewedBy, &reviewedAt, &comment, &createdAt, &updatedAt); err != nil {
			return adminEnterpriseSectionResult{}, err
		}
		items = append(items, map[string]any{"id": id, "enterpriseId": tenantID, "enterpriseName": tenantName, "legalName": legalName, "unifiedSocialCreditCode": maskEnterpriseCreditCode(creditCode), "legalRepresentativeName": representative, "status": status, "submittedBy": submittedBy, "reviewedBy": reviewedBy, "reviewedAt": formatOptionalEnterpriseTime(reviewedAt), "reviewComment": comment, "createdAt": createdAt.UTC().Format(time.RFC3339Nano), "updatedAt": updatedAt.UTC().Format(time.RFC3339Nano)})
	}
	if err := rows.Err(); err != nil {
		return adminEnterpriseSectionResult{}, err
	}
	return adminEnterpriseSectionResult{Section: "certifications", Items: items, Total: len(items), Summary: countEnterpriseStatuses(items), Privacy: adminEnterprisePrivacyBoundary()}, nil
}

func (s *postgresStore) GetAdminEnterpriseSection(id string, section string) (adminEnterpriseSectionResult, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminEnterpriseSectionResult{}, err
	}
	var enterpriseName string
	if err := s.db.QueryRowContext(ctx, `SELECT name FROM xz_tenants WHERE id=$1 AND tenant_type='ENTERPRISE'`, id).Scan(&enterpriseName); err != nil {
		if err == sql.ErrNoRows {
			return adminEnterpriseSectionResult{}, errEnterpriseNotFound
		}
		return adminEnterpriseSectionResult{}, err
	}
	result := adminEnterpriseSectionResult{Section: section, Enterprise: adminEnterpriseRelationSummary{ID: id, Name: enterpriseName}, Summary: map[string]any{}, Items: []map[string]any{}, Privacy: adminEnterprisePrivacyBoundary()}
	var err error
	switch section {
	case "certifications":
		result.Items, err = s.postgresEnterpriseCertifications(ctx, id)
		result.Summary = countEnterpriseStatuses(result.Items)
	case "members":
		result.Items, result.Summary, err = s.postgresEnterpriseMembers(ctx, id)
	case "package":
		result.Items, result.Summary, err = s.postgresEnterprisePackage(ctx, id)
	case "compute", "transactions":
		result.Items, result.Summary, err = s.postgresEnterpriseTransactions(ctx, id)
		result.Unit = "POINT"
	case "orders":
		result.Items, result.Summary, err = s.postgresEnterpriseOrders(ctx, id)
		result.Unit = "CNY_CENT"
	case "ai-capabilities":
		result.Items, result.Summary, err = s.postgresEnterpriseAICapabilities(ctx, id)
	case "ai-employees":
		result.Items, result.Summary, err = s.postgresEnterpriseAIEmployees(ctx, id)
	case "knowledge-bases":
		result.Summary, err = s.postgresEnterpriseKnowledgeSummary(ctx, id)
	case "attribution", "relationships":
		result.Items, result.Summary, err = s.postgresEnterpriseAttribution(ctx, id)
	case "risk":
		result.Items, result.Summary, err = s.postgresEnterpriseRisk(ctx, id)
	case "audit-logs":
		result.Items, result.Summary, err = s.postgresEnterpriseAudits(ctx, id)
	default:
		return adminEnterpriseSectionResult{}, fmt.Errorf("%w: unsupported enterprise section", errEnterpriseInvalid)
	}
	if err != nil {
		return adminEnterpriseSectionResult{}, err
	}
	result.Total = len(result.Items)
	return result, nil
}

func (s *postgresStore) postgresEnterpriseCertifications(ctx context.Context, tenantID string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,legal_name,unified_social_credit_code,legal_representative_name,status,coalesce(reviewed_by,''),reviewed_at,review_comment,created_at,updated_at FROM xz_tenant_certifications WHERE tenant_id=$1 ORDER BY updated_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, legalName, code, representative, status, reviewer, comment string
		var reviewedAt *time.Time
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &legalName, &code, &representative, &status, &reviewer, &reviewedAt, &comment, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"id": id, "legalName": legalName, "unifiedSocialCreditCode": maskEnterpriseCreditCode(code), "legalRepresentativeName": representative, "status": status, "reviewedBy": reviewer, "reviewedAt": formatOptionalEnterpriseTime(reviewedAt), "reviewComment": comment, "createdAt": createdAt.UTC().Format(time.RFC3339Nano), "updatedAt": updatedAt.UTC().Format(time.RFC3339Nano)})
	}
	return items, rows.Err()
}

func (s *postgresStore) postgresEnterpriseMembers(ctx context.Context, tenantID string) ([]map[string]any, map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT member.id,member.user_id,coalesce(users.name,''),coalesce(users.email,''),
		       coalesce(member.primary_organization_id,''),coalesce(organization.name,''),
		       coalesce(nullif(member.member_status,''),member.status,'ACTIVE'),member.role,
		       coalesce(member.data_scope,'SELF'),coalesce(member.joined_at,member.created_at)
		FROM xz_tenant_members member
		LEFT JOIN xz_users users ON users.id=member.user_id
		LEFT JOIN xz_organizations organization ON organization.tenant_id=member.tenant_id AND organization.id=member.primary_organization_id
		WHERE member.tenant_id=$1 ORDER BY member.created_at DESC
	`, tenantID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	active := 0
	for rows.Next() {
		var id, userID, name, email, organizationID, organizationName, status, role, dataScope string
		var joinedAt time.Time
		if err := rows.Scan(&id, &userID, &name, &email, &organizationID, &organizationName, &status, &role, &dataScope, &joinedAt); err != nil {
			return nil, nil, err
		}
		if strings.EqualFold(status, "ACTIVE") {
			active++
		}
		items = append(items, map[string]any{"id": id, "userId": userID, "name": name, "email": maskEnterpriseEmail(email), "organizationId": organizationID, "organizationName": organizationName, "roles": []string{role}, "dataScope": dataScope, "status": status, "joinedAt": joinedAt.UTC().Format(time.RFC3339Nano)})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	var seatLimit, organizationCount int
	if err := s.db.QueryRowContext(ctx, `SELECT seat_limit,(SELECT count(*) FROM xz_organizations WHERE tenant_id=$1 AND upper(status)<>'DELETED') FROM xz_tenants WHERE id=$1`, tenantID).Scan(&seatLimit, &organizationCount); err != nil {
		return nil, nil, err
	}
	organizationRows, err := s.db.QueryContext(ctx, `
		SELECT organization.id,coalesce(organization.parent_id,''),organization.name,organization.organization_type,organization.status,
		       count(member.id)::int
		FROM xz_organizations organization
		LEFT JOIN xz_tenant_members member ON member.tenant_id=organization.tenant_id AND member.primary_organization_id=organization.id
		WHERE organization.tenant_id=$1 AND upper(organization.status)<>'DELETED'
		GROUP BY organization.id,organization.parent_id,organization.name,organization.organization_type,organization.status
		ORDER BY coalesce(organization.parent_id,''),organization.created_at,organization.id
	`, tenantID)
	if err != nil {
		return nil, nil, err
	}
	defer organizationRows.Close()
	organizations := []map[string]any{}
	for organizationRows.Next() {
		var organizationID, parentID, name, organizationType, status string
		var memberCount int
		if err := organizationRows.Scan(&organizationID, &parentID, &name, &organizationType, &status, &memberCount); err != nil {
			return nil, nil, err
		}
		organizations = append(organizations, map[string]any{"id": organizationID, "parentId": parentID, "name": name, "organizationType": organizationType, "status": status, "memberCount": memberCount})
	}
	if err := organizationRows.Err(); err != nil {
		return nil, nil, err
	}
	return items, map[string]any{"memberCount": len(items), "activeMembers": active, "seatLimit": seatLimit, "organizationCount": organizationCount, "organizations": organizations}, nil
}

func (s *postgresStore) postgresEnterprisePackage(ctx context.Context, tenantID string) ([]map[string]any, map[string]any, error) {
	var id, planID, planCode, status string
	var expiresAt *time.Time
	var entitlementRaw []byte
	err := s.db.QueryRowContext(ctx, `SELECT id,coalesce(plan_id,''),plan_code,status,trial_expires_at,entitlements FROM xz_tenant_subscriptions WHERE tenant_id=$1 ORDER BY updated_at DESC,id DESC LIMIT 1`, tenantID).Scan(&id, &planID, &planCode, &status, &expiresAt, &entitlementRaw)
	if err == sql.ErrNoRows {
		return []map[string]any{}, map[string]any{"seatLimit": 0, "seatUsed": 0}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	entitlements := map[string]any{}
	_ = json.Unmarshal(entitlementRaw, &entitlements)
	var seatLimit, seatUsed int
	if err := s.db.QueryRowContext(ctx, `SELECT seat_limit,(SELECT count(*) FROM xz_tenant_members WHERE tenant_id=$1) FROM xz_tenants WHERE id=$1`, tenantID).Scan(&seatLimit, &seatUsed); err != nil {
		return nil, nil, err
	}
	item := map[string]any{"id": id, "planId": planID, "planCode": planCode, "status": status, "expiresAt": formatOptionalEnterpriseTime(expiresAt), "entitlements": entitlements}
	return []map[string]any{item}, map[string]any{"planCode": planCode, "status": status, "expiresAt": formatOptionalEnterpriseTime(expiresAt), "seatLimit": seatLimit, "seatUsed": seatUsed}, nil
}

func (s *postgresStore) postgresEnterpriseTransactions(ctx context.Context, tenantID string) ([]map[string]any, map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,transaction_type,point_delta,balance_after,reference_type,reference_id,reason,coalesce(actor_user_id,''),request_id,created_at FROM xz_tenant_point_transactions WHERE tenant_id=$1 ORDER BY created_at DESC,id DESC LIMIT 500`, tenantID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, transactionType, referenceType, referenceID, reason, actorID, requestID string
		var delta, balance int64
		var createdAt time.Time
		if err := rows.Scan(&id, &transactionType, &delta, &balance, &referenceType, &referenceID, &reason, &actorID, &requestID, &createdAt); err != nil {
			return nil, nil, err
		}
		items = append(items, map[string]any{"id": id, "type": transactionType, "pointDelta": delta, "balanceAfter": balance, "referenceType": referenceType, "referenceId": referenceID, "reason": reason, "actorUserId": actorID, "requestId": requestID, "createdAt": createdAt.UTC().Format(time.RFC3339Nano)})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	var balance, frozen, cash int64
	var status string
	err = s.db.QueryRowContext(ctx, `SELECT point_balance,frozen_points,cash_balance_cents,status FROM xz_tenant_wallets WHERE tenant_id=$1`, tenantID).Scan(&balance, &frozen, &cash, &status)
	if err == sql.ErrNoRows {
		return items, map[string]any{"balance": int64(0), "frozen": int64(0), "cashBalanceCents": int64(0), "status": "ACTIVE"}, nil
	}
	return items, map[string]any{"balance": balance, "frozen": frozen, "cashBalanceCents": cash, "status": status}, err
}

func (s *postgresStore) postgresEnterpriseOrders(ctx context.Context, tenantID string) ([]map[string]any, map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT orders.id,coalesce(orders.raw->>'orderNo',''),coalesce(orders.raw->>'businessOrderType',orders.raw->>'orderType',''),
		       coalesce(orders.plan_id,''),orders.amount_cents,coalesce(orders.status,''),coalesce(orders.paid_at,''),coalesce(orders.created_at,'')
		FROM xz_orders orders
		WHERE orders.tenant_id=$1
		   OR (
			coalesce(orders.tenant_id,'')=''
			AND EXISTS (
				SELECT 1 FROM xz_tenant_members member
				WHERE member.tenant_id=$1 AND member.user_id=orders.user_id
				  AND upper(coalesce(nullif(member.member_status,''),member.status,'ACTIVE'))='ACTIVE'
			)
			AND NOT EXISTS (
				SELECT 1
				FROM xz_tenant_members other_member
				JOIN xz_tenants other_tenant ON other_tenant.id=other_member.tenant_id AND other_tenant.tenant_type='ENTERPRISE'
				WHERE other_member.user_id=orders.user_id AND other_member.tenant_id<>$1
				  AND upper(coalesce(nullif(other_member.member_status,''),other_member.status,'ACTIVE'))='ACTIVE'
			)
		   )
		ORDER BY orders.created_at DESC,orders.id DESC LIMIT 500
	`, tenantID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	var totalAmount int64
	for rows.Next() {
		var id, orderNo, orderType, planID, status, paidAt, createdAt string
		var amount int64
		if err := rows.Scan(&id, &orderNo, &orderType, &planID, &amount, &status, &paidAt, &createdAt); err != nil {
			return nil, nil, err
		}
		totalAmount += amount
		items = append(items, map[string]any{"id": id, "orderNo": orderNo, "orderType": orderType, "planId": planID, "amountCents": amount, "status": status, "paidAt": paidAt, "createdAt": createdAt})
	}
	return items, map[string]any{"orderCount": len(items), "amountCents": totalAmount}, rows.Err()
}

func (s *postgresStore) postgresEnterpriseAICapabilities(ctx context.Context, tenantID string) ([]map[string]any, map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,module_code,coalesce(model_name,''),status,limit_json,updated_at FROM tenant_module_limits WHERE tenant_id=$1 ORDER BY module_code,model_name`, tenantID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	active := 0
	for rows.Next() {
		var id, moduleCode, modelName, status string
		var limitRaw []byte
		var updatedAt time.Time
		if err := rows.Scan(&id, &moduleCode, &modelName, &status, &limitRaw, &updatedAt); err != nil {
			return nil, nil, err
		}
		limits := map[string]any{}
		_ = json.Unmarshal(limitRaw, &limits)
		if strings.EqualFold(status, "ACTIVE") {
			active++
		}
		items = append(items, map[string]any{"id": id, "moduleCode": moduleCode, "modelName": modelName, "status": status, "limits": limits, "updatedAt": updatedAt.UTC().Format(time.RFC3339Nano)})
	}
	return items, map[string]any{"enabledCount": active, "configuredCount": len(items)}, rows.Err()
}

func (s *postgresStore) postgresEnterpriseAIEmployees(ctx context.Context, tenantID string) ([]map[string]any, map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,status,version,updated_at FROM xz_ai_agents WHERE tenant_id=$1 AND deleted_at IS NULL ORDER BY updated_at DESC LIMIT 500`, tenantID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	active := 0
	for rows.Next() {
		var id, status string
		var version int64
		var updatedAt time.Time
		if err := rows.Scan(&id, &status, &version, &updatedAt); err != nil {
			return nil, nil, err
		}
		if strings.EqualFold(status, "ACTIVE") || strings.EqualFold(status, "PUBLISHED") {
			active++
		}
		items = append(items, map[string]any{"id": id, "status": status, "version": version, "updatedAt": updatedAt.UTC().Format(time.RFC3339Nano)})
	}
	return items, map[string]any{"total": len(items), "active": active, "summaryOnly": true}, rows.Err()
}

func (s *postgresStore) postgresEnterpriseKnowledgeSummary(ctx context.Context, tenantID string) (map[string]any, error) {
	var baseCount, documentCount, chunkCount, storageBytes int64
	err := s.db.QueryRowContext(ctx, `
		SELECT count(*),coalesce(sum(document_count),0),coalesce(sum(chunk_count),0),
		       coalesce(sum((metadata->>'storageBytes')::bigint) FILTER (WHERE metadata->>'storageBytes' ~ '^[0-9]+$'),0)
		FROM xz_knowledge_bases WHERE tenant_id=$1 AND deleted_at IS NULL
	`, tenantID).Scan(&baseCount, &documentCount, &chunkCount, &storageBytes)
	return map[string]any{"knowledgeBaseCount": baseCount, "documentCount": documentCount, "chunkCount": chunkCount, "storageBytes": storageBytes, "summaryOnly": true}, err
}

func (s *postgresStore) postgresEnterpriseAttribution(ctx context.Context, tenantID string) ([]map[string]any, map[string]any, error) {
	var agentID, agentName, centerID, centerName, status string
	var updatedAt time.Time
	err := s.db.QueryRowContext(ctx, `
		SELECT coalesce(tenant.source_agent_id,''),coalesce(agent_user.name,''),coalesce(tenant.operation_center_id,''),coalesce(center.name,''),tenant.status,tenant.updated_at
		FROM xz_tenants tenant
		LEFT JOIN xz_channel_agents agent ON agent.id=tenant.source_agent_id
		LEFT JOIN xz_users agent_user ON agent_user.id=agent.user_id
		LEFT JOIN xz_operation_centers center ON center.id=tenant.operation_center_id
		WHERE tenant.id=$1
	`, tenantID).Scan(&agentID, &agentName, &centerID, &centerName, &status, &updatedAt)
	if err != nil {
		return nil, nil, err
	}
	pending := []map[string]any{}
	rows, queryErr := s.db.QueryContext(ctx, `SELECT id,action_type,reason,before_snapshot,after_snapshot,status,coalesce(requested_by,''),coalesce(approved_by,''),request_id,created_at,updated_at FROM xz_admin_enterprise_change_requests WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 100`, tenantID)
	if queryErr != nil {
		return nil, nil, queryErr
	}
	defer rows.Close()
	for rows.Next() {
		var id, action, reason, statusValue, requestedBy, approvedBy, requestID string
		var beforeRaw, afterRaw []byte
		var createdAt, changedAt time.Time
		if err := rows.Scan(&id, &action, &reason, &beforeRaw, &afterRaw, &statusValue, &requestedBy, &approvedBy, &requestID, &createdAt, &changedAt); err != nil {
			return nil, nil, err
		}
		before, after := map[string]any{}, map[string]any{}
		_ = json.Unmarshal(beforeRaw, &before)
		_ = json.Unmarshal(afterRaw, &after)
		pending = append(pending, map[string]any{"id": id, "action": action, "reason": reason, "before": before, "after": after, "status": statusValue, "requestedBy": requestedBy, "approvedBy": approvedBy, "requestId": requestID, "createdAt": createdAt.UTC().Format(time.RFC3339Nano), "updatedAt": changedAt.UTC().Format(time.RFC3339Nano)})
	}
	history := []map[string]any{}
	historyRows, historyErr := s.db.QueryContext(ctx, `
		SELECT id,coalesce(source_agent_id,''),coalesce(operation_center_id,''),
		       coalesce(previous_source_agent_id,''),coalesce(previous_operation_center_id,''),
		       effective_from,effective_to,reason,coalesce(change_request_id,''),coalesce(actor_user_id,''),
		       before_value,after_value,status,created_at
		FROM xz_customer_attribution_history
		WHERE tenant_id=$1
		ORDER BY created_at DESC,id DESC
		LIMIT 200
	`, tenantID)
	if historyErr != nil {
		return nil, nil, historyErr
	}
	defer historyRows.Close()
	for historyRows.Next() {
		var id, sourceAgentID, operationCenterID, previousAgentID, previousCenterID, reason, changeRequestID, actorID, statusValue string
		var effectiveFrom, createdAt time.Time
		var effectiveTo *time.Time
		var beforeRaw, afterRaw []byte
		if err := historyRows.Scan(&id, &sourceAgentID, &operationCenterID, &previousAgentID, &previousCenterID,
			&effectiveFrom, &effectiveTo, &reason, &changeRequestID, &actorID, &beforeRaw, &afterRaw, &statusValue, &createdAt); err != nil {
			return nil, nil, err
		}
		beforeValue, afterValue := map[string]any{}, map[string]any{}
		_ = json.Unmarshal(beforeRaw, &beforeValue)
		_ = json.Unmarshal(afterRaw, &afterValue)
		history = append(history, map[string]any{
			"id": id, "recordType": "ATTRIBUTION_HISTORY", "sourceAgentId": sourceAgentID,
			"operationCenterId": operationCenterID, "previousSourceAgentId": previousAgentID,
			"previousOperationCenterId": previousCenterID, "effectiveFrom": effectiveFrom.UTC().Format(time.RFC3339Nano),
			"effectiveTo": formatOptionalEnterpriseTime(effectiveTo), "reason": reason, "changeRequestId": changeRequestID,
			"actorUserId": actorID, "before": beforeValue, "after": afterValue, "status": statusValue,
			"createdAt": createdAt.UTC().Format(time.RFC3339Nano),
		})
	}
	current := map[string]any{"sourceAgent": map[string]any{"id": agentID, "name": agentName}, "operationCenter": map[string]any{"id": centerID, "name": centerName}, "status": status, "updatedAt": updatedAt.UTC().Format(time.RFC3339Nano)}
	items := append([]map[string]any{current}, history...)
	items = append(items, pending...)
	return items, map[string]any{"sourceAgent": current["sourceAgent"], "operationCenter": current["operationCenter"], "historyCount": len(history), "changeRequestCount": len(pending)}, historyRows.Err()
}

func (s *postgresStore) postgresEnterpriseRisk(ctx context.Context, tenantID string) ([]map[string]any, map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,risk_level,action,reason,status,coalesce(actor_user_id,''),request_id,created_at,resolved_at FROM xz_admin_enterprise_risk_records WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 500`, tenantID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, level, action, reason, status, actorID, requestID string
		var createdAt time.Time
		var resolvedAt *time.Time
		if err := rows.Scan(&id, &level, &action, &reason, &status, &actorID, &requestID, &createdAt, &resolvedAt); err != nil {
			return nil, nil, err
		}
		items = append(items, map[string]any{"id": id, "riskLevel": level, "action": action, "reason": reason, "status": status, "actorUserId": actorID, "requestId": requestID, "createdAt": createdAt.UTC().Format(time.RFC3339Nano), "resolvedAt": formatOptionalEnterpriseTime(resolvedAt)})
	}
	var enterpriseStatus string
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM xz_tenants WHERE id=$1`, tenantID).Scan(&enterpriseStatus); err != nil {
		return nil, nil, err
	}
	return items, map[string]any{"enterpriseStatus": enterpriseStatus, "riskRecordCount": len(items)}, rows.Err()
}

func (s *postgresStore) postgresEnterpriseAudits(ctx context.Context, tenantID string) ([]map[string]any, map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,coalesce(actor_user_id,''),coalesce(actor_role,''),action,resource_type,coalesce(resource_id,''),coalesce(request_id,''),status,metadata,before_value,after_value,created_at FROM xz_tenant_audit_logs WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 1000`, tenantID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, actorID, actorRole, action, resourceType, resourceID, requestID, status string
		var metadataRaw, beforeRaw, afterRaw []byte
		var createdAt time.Time
		if err := rows.Scan(&id, &actorID, &actorRole, &action, &resourceType, &resourceID, &requestID, &status, &metadataRaw, &beforeRaw, &afterRaw, &createdAt); err != nil {
			return nil, nil, err
		}
		metadata := map[string]any{}
		_ = json.Unmarshal(metadataRaw, &metadata)
		beforeValue, afterValue := map[string]any{}, map[string]any{}
		_ = json.Unmarshal(beforeRaw, &beforeValue)
		_ = json.Unmarshal(afterRaw, &afterValue)
		items = append(items, map[string]any{"id": id, "actorUserId": actorID, "actorRole": actorRole, "action": action, "resourceType": resourceType, "resourceId": resourceID, "requestId": requestID, "status": status, "metadata": sanitizeEnterpriseAuditMetadata(metadata), "beforeValue": sanitizeEnterpriseAuditMetadata(beforeValue), "afterValue": sanitizeEnterpriseAuditMetadata(afterValue), "createdAt": createdAt.UTC().Format(time.RFC3339Nano)})
	}
	return items, map[string]any{"total": len(items)}, rows.Err()
}

func (s *postgresStore) MutateAdminEnterprise(actorID string, actorRole string, id string, request adminEnterpriseMutationRequest) (adminEnterpriseMutationResult, error) {
	if err := validateAdminEnterpriseMutation(request); err != nil {
		return adminEnterpriseMutationResult{}, err
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminEnterpriseMutationResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminEnterpriseMutationResult{}, err
	}
	defer tx.Rollback()
	var tenantStatus, sourceAgentID, operationCenterID, tenantName, tenantIndustry, tenantCompanySize string
	var seatLimit int
	if err := tx.QueryRowContext(ctx, `SELECT status,coalesce(source_agent_id,''),coalesce(operation_center_id,''),seat_limit,name,coalesce(industry,''),coalesce(company_size,'') FROM xz_tenants WHERE id=$1 AND tenant_type='ENTERPRISE' FOR UPDATE`, id).Scan(&tenantStatus, &sourceAgentID, &operationCenterID, &seatLimit, &tenantName, &tenantIndustry, &tenantCompanySize); err != nil {
		if err == sql.ErrNoRows {
			return adminEnterpriseMutationResult{}, errEnterpriseNotFound
		}
		return adminEnterpriseMutationResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_admin_enterprise_requests(request_id,tenant_id,action,status) VALUES($1,$2,$3,'PROCESSING')`, request.RequestID, id, request.Action); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			return adminEnterpriseMutationResult{}, fmt.Errorf("%w: duplicate requestId", errEnterpriseConflict)
		}
		return adminEnterpriseMutationResult{}, err
	}
	result := adminEnterpriseMutationResult{RequestID: request.RequestID, Action: request.Action, Status: "SUCCEEDED", Enterprise: id, Before: map[string]any{}, After: map[string]any{}, Message: "操作成功，审计记录已写入"}
	switch request.Action {
	case "profile-update":
		name := strings.TrimSpace(request.Name)
		if name == "" {
			return adminEnterpriseMutationResult{}, fmt.Errorf("%w: enterprise name is required", errEnterpriseInvalid)
		}
		result.Before = map[string]any{"name": tenantName, "industry": tenantIndustry, "companySize": tenantCompanySize}
		result.After = map[string]any{"name": name, "industry": strings.TrimSpace(request.Industry), "companySize": strings.TrimSpace(request.CompanySize)}
		if _, err := tx.ExecContext(ctx, `UPDATE xz_tenants SET name=$2,industry=$3,company_size=$4,updated_at=now() WHERE id=$1`, id, name, strings.TrimSpace(request.Industry), strings.TrimSpace(request.CompanySize)); err != nil {
			return adminEnterpriseMutationResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE xz_organizations SET name=$3,updated_at=now() WHERE tenant_id=$1 AND upper(organization_type)='ROOT' AND name=$2`, id, tenantName, name); err != nil {
			return adminEnterpriseMutationResult{}, err
		}
	case "certification-review":
		status := strings.ToUpper(strings.TrimSpace(request.Status))
		if status != "APPROVED" && status != "REJECTED" {
			return adminEnterpriseMutationResult{}, fmt.Errorf("%w: certification status must be APPROVED or REJECTED", errEnterpriseInvalid)
		}
		var certificationID, beforeStatus string
		if err := tx.QueryRowContext(ctx, `SELECT id,status FROM xz_tenant_certifications WHERE tenant_id=$1 ORDER BY updated_at DESC,id DESC LIMIT 1 FOR UPDATE`, id).Scan(&certificationID, &beforeStatus); err != nil {
			if err == sql.ErrNoRows {
				return adminEnterpriseMutationResult{}, fmt.Errorf("%w: pending certification not found", errEnterpriseInvalid)
			}
			return adminEnterpriseMutationResult{}, err
		}
		if !strings.EqualFold(beforeStatus, "PENDING") {
			return adminEnterpriseMutationResult{}, fmt.Errorf("%w: only pending certification can be reviewed", errEnterpriseConflict)
		}
		result.Before, result.After = map[string]any{"status": beforeStatus}, map[string]any{"status": status}
		if _, err := tx.ExecContext(ctx, `UPDATE xz_tenant_certifications SET status=$2,reviewed_by=nullif($3,''),reviewed_at=now(),review_comment=$4,updated_at=now() WHERE id=$1`, certificationID, status, actorID, request.ReviewComment); err != nil {
			return adminEnterpriseMutationResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE xz_tenants SET config=jsonb_set(config,'{certificationStatus}',to_jsonb($2::text),true),updated_at=now() WHERE id=$1`, id, status); err != nil {
			return adminEnterpriseMutationResult{}, err
		}
	case "package-adjust":
		var subscriptionID, beforePlanID, beforePlanCode string
		var beforeExpires *time.Time
		err := tx.QueryRowContext(ctx, `SELECT id,coalesce(plan_id,''),plan_code,trial_expires_at FROM xz_tenant_subscriptions WHERE tenant_id=$1 ORDER BY updated_at DESC,id DESC LIMIT 1 FOR UPDATE`, id).Scan(&subscriptionID, &beforePlanID, &beforePlanCode, &beforeExpires)
		if err == sql.ErrNoRows {
			subscriptionID = newEnterpriseResourceID("tenant_subscription")
			if _, err = tx.ExecContext(ctx, `INSERT INTO xz_tenant_subscriptions(id,tenant_id,plan_id,plan_code,status,trial_expires_at,entitlements) VALUES($1,$2,nullif($3,''),$4,'ACTIVE',nullif($5,'')::timestamptz,'{}'::jsonb)`, subscriptionID, id, request.PlanID, firstNonEmptyString(request.PlanCode, "enterprise_trial"), request.ExpiresAt); err != nil {
				return adminEnterpriseMutationResult{}, err
			}
		} else if err != nil {
			return adminEnterpriseMutationResult{}, err
		} else {
			if _, err = tx.ExecContext(ctx, `UPDATE xz_tenant_subscriptions SET plan_id=coalesce(nullif($2,''),plan_id),plan_code=coalesce(nullif($3,''),plan_code),status='ACTIVE',trial_expires_at=coalesce(nullif($4,'')::timestamptz,trial_expires_at),updated_at=now() WHERE id=$1`, subscriptionID, request.PlanID, request.PlanCode, request.ExpiresAt); err != nil {
				return adminEnterpriseMutationResult{}, err
			}
		}
		result.Before = map[string]any{"planId": beforePlanID, "planCode": beforePlanCode, "expiresAt": formatOptionalEnterpriseTime(beforeExpires)}
		result.After = map[string]any{"planId": firstNonEmptyString(request.PlanID, beforePlanID), "planCode": firstNonEmptyString(request.PlanCode, beforePlanCode), "expiresAt": firstNonEmptyString(request.ExpiresAt, formatOptionalEnterpriseTime(beforeExpires))}
	case "seat-adjust":
		if request.SeatLimit < 1 || request.SeatLimit > 100000 {
			return adminEnterpriseMutationResult{}, fmt.Errorf("%w: seatLimit must be between 1 and 100000", errEnterpriseInvalid)
		}
		var used int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM xz_tenant_members WHERE tenant_id=$1 AND upper(coalesce(nullif(member_status,''),status,'ACTIVE'))='ACTIVE'`, id).Scan(&used); err != nil {
			return adminEnterpriseMutationResult{}, err
		}
		if request.SeatLimit < used {
			return adminEnterpriseMutationResult{}, fmt.Errorf("%w: seatLimit cannot be lower than active member count", errEnterpriseInvalid)
		}
		result.Before, result.After = map[string]any{"seatLimit": seatLimit}, map[string]any{"seatLimit": request.SeatLimit}
		if _, err := tx.ExecContext(ctx, `UPDATE xz_tenants SET seat_limit=$2,updated_at=now() WHERE id=$1`, id, request.SeatLimit); err != nil {
			return adminEnterpriseMutationResult{}, err
		}
	case "compute-adjust", "recharge":
		baseUnits := request.PointDelta
		bonusUnits := int64(0)
		if request.Action == "recharge" {
			if request.RechargeUnits > 0 {
				baseUnits = request.RechargeUnits
			}
			bonusUnits = request.BonusUnits
		}
		totalDelta := baseUnits + bonusUnits
		if totalDelta == 0 {
			return adminEnterpriseMutationResult{}, fmt.Errorf("%w: compute unit delta must not be zero", errEnterpriseInvalid)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO xz_tenant_wallets(tenant_id,status) VALUES($1,'ACTIVE') ON CONFLICT(tenant_id) DO NOTHING`, id); err != nil {
			return adminEnterpriseMutationResult{}, err
		}
		var beforeBalance int64
		if err := tx.QueryRowContext(ctx, `SELECT point_balance FROM xz_tenant_wallets WHERE tenant_id=$1 FOR UPDATE`, id).Scan(&beforeBalance); err != nil {
			return adminEnterpriseMutationResult{}, err
		}
		if beforeBalance+totalDelta < 0 {
			return adminEnterpriseMutationResult{}, fmt.Errorf("%w: insufficient enterprise compute balance", errEnterpriseInvalid)
		}
		afterBalance := beforeBalance + totalDelta
		result.Before = map[string]any{"balance": beforeBalance, "unit": "COMPUTE_UNIT"}
		result.After = map[string]any{"balance": afterBalance, "unit": "COMPUTE_UNIT", "rechargeUnits": baseUnits, "bonusUnits": bonusUnits, "amountCents": request.AmountCents}
		if totalDelta < 0 {
			if _, err := consumeEnterpriseCreditLotsTx(ctx, tx, id, -totalDelta); err != nil {
				return adminEnterpriseMutationResult{}, err
			}
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE xz_tenant_wallets
			SET point_balance=$2,version=version+1,
			    total_recharge_units=total_recharge_units+$3,
			    total_bonus_units=total_bonus_units+$4,
			    updated_at=now()
			WHERE tenant_id=$1
		`, id, afterBalance, maxInt64(baseUnits, 0), maxInt64(bonusUnits, 0)); err != nil {
			return adminEnterpriseMutationResult{}, err
		}
		if baseUnits > 0 {
			sourceType := "MANUAL"
			if request.Action == "recharge" {
				sourceType = "RECHARGE"
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO xz_compute_credit_lots(id,tenant_id,account_id,source_type,original_units,remaining_units,amount_cents,reference_type,reference_id,idempotency_key,status,metadata)
				VALUES($1,$2,$2,$3,$4,$4,$5,'ADMIN_OPERATION',$6,$7,'ACTIVE',$8::jsonb)
			`, newEnterpriseResourceID("compute_credit"), id, sourceType, baseUnits, request.AmountCents, request.RequestID, request.RequestID+":base", jsonProjection(map[string]any{"reason": request.Reason})); err != nil {
				return adminEnterpriseMutationResult{}, err
			}
		}
		if bonusUnits > 0 {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO xz_compute_credit_lots(id,tenant_id,account_id,source_type,original_units,remaining_units,amount_cents,reference_type,reference_id,idempotency_key,status,metadata)
				VALUES($1,$2,$2,'BONUS',$3,$3,0,'ADMIN_OPERATION',$4,$5,'ACTIVE',$6::jsonb)
			`, newEnterpriseResourceID("compute_credit"), id, bonusUnits, request.RequestID, request.RequestID+":bonus", jsonProjection(map[string]any{"reason": request.Reason})); err != nil {
				return adminEnterpriseMutationResult{}, err
			}
		}
		entryType, sourceType := "CREDIT", "MANUAL"
		if totalDelta < 0 {
			entryType = "DEBIT"
		}
		if request.Action == "recharge" {
			sourceType = "RECHARGE"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO xz_compute_ledger_entries(
				id,tenant_id,account_id,entry_type,source_type,compute_unit_delta,balance_before,balance_after,
				amount_cents,reference_type,reference_id,idempotency_key,actor_user_id,request_id,before_value,after_value,status,metadata
			) VALUES($1,$2,$2,$3,$4,$5,$6,$7,$8,'ADMIN_OPERATION',$9,$9,nullif($10,''),$9,$11::jsonb,$12::jsonb,'POSTED',$13::jsonb)
		`, newEnterpriseResourceID("compute_ledger"), id, entryType, sourceType, totalDelta, beforeBalance, afterBalance,
			request.AmountCents, request.RequestID, actorID, jsonProjection(result.Before), jsonProjection(result.After), jsonProjection(map[string]any{"reason": request.Reason})); err != nil {
			return adminEnterpriseMutationResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO xz_tenant_point_transactions(id,tenant_id,transaction_type,point_delta,balance_after,reference_type,reference_id,reason,actor_user_id,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,nullif($9,''),$10)`, newEnterpriseResourceID("tenant_point_transaction"), id, strings.ToUpper(request.Action), totalDelta, afterBalance, "ADMIN_OPERATION", request.RequestID, request.Reason, actorID, request.RequestID); err != nil {
			return adminEnterpriseMutationResult{}, err
		}
	case "ai-capability-configure":
		moduleCode := strings.TrimSpace(request.ModuleCode)
		status := strings.ToUpper(strings.TrimSpace(request.Status))
		if moduleCode == "" {
			return adminEnterpriseMutationResult{}, fmt.Errorf("%w: moduleCode is required", errEnterpriseInvalid)
		}
		if status == "" {
			status = "ACTIVE"
		}
		if status != "ACTIVE" && status != "DISABLED" {
			return adminEnterpriseMutationResult{}, fmt.Errorf("%w: AI capability status must be ACTIVE or DISABLED", errEnterpriseInvalid)
		}
		var moduleExists bool
		if err := tx.QueryRowContext(ctx, `SELECT exists(SELECT 1 FROM ai_modules WHERE module_code=$1)`, moduleCode).Scan(&moduleExists); err != nil || !moduleExists {
			return adminEnterpriseMutationResult{}, fmt.Errorf("%w: AI module not found", errEnterpriseInvalid)
		}
		limits := request.Limits
		if limits == nil {
			limits = map[string]any{}
		}
		var limitID, beforeStatus string
		var beforeLimitsRaw []byte
		err := tx.QueryRowContext(ctx, `SELECT id,status,limit_json FROM tenant_module_limits WHERE tenant_id=$1 AND module_code=$2 AND coalesce(model_name,'')=$3 ORDER BY updated_at DESC,id DESC LIMIT 1 FOR UPDATE`, id, moduleCode, strings.TrimSpace(request.ModelName)).Scan(&limitID, &beforeStatus, &beforeLimitsRaw)
		if err == sql.ErrNoRows {
			limitID = newEnterpriseResourceID("tenant_module_limit")
			result.Before = map[string]any{"configured": false}
			if _, err = tx.ExecContext(ctx, `INSERT INTO tenant_module_limits(id,tenant_id,module_code,model_name,limit_json,status,created_at,updated_at) VALUES($1,$2,$3,nullif($4,''),$5::jsonb,$6,now(),now())`, limitID, id, moduleCode, strings.TrimSpace(request.ModelName), jsonProjection(limits), status); err != nil {
				return adminEnterpriseMutationResult{}, err
			}
		} else if err != nil {
			return adminEnterpriseMutationResult{}, err
		} else {
			beforeLimits := map[string]any{}
			_ = json.Unmarshal(beforeLimitsRaw, &beforeLimits)
			result.Before = map[string]any{"moduleCode": moduleCode, "modelName": strings.TrimSpace(request.ModelName), "status": beforeStatus, "limits": beforeLimits}
			if _, err = tx.ExecContext(ctx, `UPDATE tenant_module_limits SET limit_json=$2::jsonb,status=$3,updated_at=now() WHERE id=$1`, limitID, jsonProjection(limits), status); err != nil {
				return adminEnterpriseMutationResult{}, err
			}
		}
		result.After = map[string]any{"moduleCode": moduleCode, "modelName": strings.TrimSpace(request.ModelName), "status": status, "limits": limits}
	case "attribution-change":
		if request.SourceAgentID != "" {
			var exists bool
			if err := tx.QueryRowContext(ctx, `SELECT exists(SELECT 1 FROM xz_channel_agents WHERE id=$1)`, request.SourceAgentID).Scan(&exists); err != nil || !exists {
				return adminEnterpriseMutationResult{}, fmt.Errorf("%w: source agent not found", errEnterpriseInvalid)
			}
		}
		if request.OperationCenterID != "" {
			var exists bool
			if err := tx.QueryRowContext(ctx, `SELECT exists(SELECT 1 FROM xz_operation_centers WHERE id=$1)`, request.OperationCenterID).Scan(&exists); err != nil || !exists {
				return adminEnterpriseMutationResult{}, fmt.Errorf("%w: operation center not found", errEnterpriseInvalid)
			}
		}
		result.Before = map[string]any{"sourceAgentId": sourceAgentID, "operationCenterId": operationCenterID}
		result.After = map[string]any{"sourceAgentId": request.SourceAgentID, "operationCenterId": request.OperationCenterID}
		changeStatus := "APPLIED"
		approvedBy := actorID
		if !strings.EqualFold(actorRole, "SUPER_ADMIN") {
			changeStatus, approvedBy, result.Status, result.Message = "PENDING_APPROVAL", "", "PENDING_APPROVAL", "归属变更已提交审批，审批通过前不会修改当前归属"
		}
		if changeStatus == "APPLIED" {
			if _, err := tx.ExecContext(ctx, `UPDATE xz_customer_attribution_history SET status='SUPERSEDED',effective_to=now() WHERE tenant_id=$1 AND status='ACTIVE' AND effective_to IS NULL`, id); err != nil {
				return adminEnterpriseMutationResult{}, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE xz_tenants SET source_agent_id=nullif($2,''),operation_center_id=nullif($3,''),updated_at=now() WHERE id=$1`, id, request.SourceAgentID, request.OperationCenterID); err != nil {
				return adminEnterpriseMutationResult{}, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO xz_customer_attribution_history(
					id,tenant_id,source_agent_id,operation_center_id,previous_source_agent_id,previous_operation_center_id,
					reason,actor_user_id,before_value,after_value,status
				) VALUES($1,$2,nullif($3,''),nullif($4,''),nullif($5,''),nullif($6,''),$7,nullif($8,''),$9::jsonb,$10::jsonb,'ACTIVE')
			`, newEnterpriseResourceID("attribution_history"), id, request.SourceAgentID, request.OperationCenterID,
				sourceAgentID, operationCenterID, request.Reason, actorID, jsonProjection(result.Before), jsonProjection(result.After)); err != nil {
				return adminEnterpriseMutationResult{}, err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO xz_admin_enterprise_change_requests(id,tenant_id,action_type,reason,before_snapshot,after_snapshot,status,requested_by,approved_by,request_id) VALUES($1,$2,'ATTRIBUTION_CHANGE',$3,$4::jsonb,$5::jsonb,$6,nullif($7,''),nullif($8,''),$9)`, newEnterpriseResourceID("enterprise_change"), id, request.Reason, jsonProjection(result.Before), jsonProjection(result.After), changeStatus, actorID, approvedBy, request.RequestID); err != nil {
			return adminEnterpriseMutationResult{}, err
		}
	case "risk-disable", "risk-restore":
		nextStatus, riskStatus := "SUSPENDED", "ACTIVE"
		nextLifecycle := "PAUSED"
		if request.Action == "risk-restore" {
			nextStatus, riskStatus, nextLifecycle = "ACTIVE", "RESOLVED", "ACTIVE"
		}
		var beforeLifecycle string
		_ = tx.QueryRowContext(ctx, `SELECT lifecycle_state FROM xz_tenant_service_states WHERE tenant_id=$1 FOR UPDATE`, id).Scan(&beforeLifecycle)
		result.Before = map[string]any{"status": tenantStatus, "lifecycleState": beforeLifecycle}
		result.After = map[string]any{"status": nextStatus, "lifecycleState": nextLifecycle}
		if _, err := tx.ExecContext(ctx, `UPDATE xz_tenants SET status=$2,updated_at=now() WHERE id=$1`, id, nextStatus); err != nil {
			return adminEnterpriseMutationResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO xz_tenant_service_states(tenant_id,lifecycle_state,reason,changed_by,status)
			VALUES($1,$2,$3,nullif($4,''),'ACTIVE')
			ON CONFLICT(tenant_id) DO UPDATE SET lifecycle_state=excluded.lifecycle_state,reason=excluded.reason,
			  changed_by=excluded.changed_by,changed_at=now(),state_version=xz_tenant_service_states.state_version+1,
			  status='ACTIVE',updated_at=now()
		`, id, nextLifecycle, request.Reason, actorID); err != nil {
			return adminEnterpriseMutationResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO xz_admin_enterprise_risk_records(id,tenant_id,risk_level,action,reason,status,actor_user_id,request_id,resolved_at) VALUES($1,$2,$3,$4,$5,$6,nullif($7,''),$8,CASE WHEN $6='RESOLVED' THEN now() ELSE NULL END)`, newEnterpriseResourceID("enterprise_risk"), id, "HIGH", strings.ToUpper(request.Action), request.Reason, riskStatus, actorID, request.RequestID); err != nil {
			return adminEnterpriseMutationResult{}, err
		}
	case "service-state":
		nextLifecycle := strings.ToUpper(strings.TrimSpace(request.Status))
		if nextLifecycle != "ACTIVE" && nextLifecycle != "PAUSED" && nextLifecycle != "DISABLED" && nextLifecycle != "TERMINATED" {
			return adminEnterpriseMutationResult{}, fmt.Errorf("%w: service state must be ACTIVE, PAUSED, DISABLED or TERMINATED", errEnterpriseInvalid)
		}
		var beforeLifecycle string
		_ = tx.QueryRowContext(ctx, `SELECT lifecycle_state FROM xz_tenant_service_states WHERE tenant_id=$1 FOR UPDATE`, id).Scan(&beforeLifecycle)
		legacyStatus := nextLifecycle
		if nextLifecycle == "PAUSED" {
			legacyStatus = "SUSPENDED"
		}
		result.Before = map[string]any{"status": tenantStatus, "lifecycleState": beforeLifecycle}
		result.After = map[string]any{"status": legacyStatus, "lifecycleState": nextLifecycle}
		if _, err := tx.ExecContext(ctx, `UPDATE xz_tenants SET status=$2,updated_at=now() WHERE id=$1`, id, legacyStatus); err != nil {
			return adminEnterpriseMutationResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO xz_tenant_service_states(tenant_id,lifecycle_state,reason,changed_by,status)
			VALUES($1,$2,$3,nullif($4,''),'ACTIVE')
			ON CONFLICT(tenant_id) DO UPDATE SET lifecycle_state=excluded.lifecycle_state,reason=excluded.reason,
			  changed_by=excluded.changed_by,changed_at=now(),state_version=xz_tenant_service_states.state_version+1,
			  status='ACTIVE',updated_at=now()
		`, id, nextLifecycle, request.Reason, actorID); err != nil {
			return adminEnterpriseMutationResult{}, err
		}
	default:
		return adminEnterpriseMutationResult{}, fmt.Errorf("%w: unsupported enterprise action", errEnterpriseInvalid)
	}
	metadata := map[string]any{"requestId": request.RequestID, "reason": request.Reason, "before": result.Before, "after": result.After, "resultStatus": result.Status, "summary": request.Reason}
	if err := insertTenantAuditTx(ctx, tx, enterpriseAccess{UserID: actorID, TenantID: id, Role: actorRole}, "admin.enterprise."+request.Action, "tenant", id, "", metadata); err != nil {
		return adminEnterpriseMutationResult{}, err
	}
	if err := insertAuditLog(ctx, tx, actorID, actorRole, "admin.enterprise."+request.Action, "tenant", id, "POST", "/api/v1/admin/enterprises/"+id+"/actions/"+request.Action, 200, metadata); err != nil {
		return adminEnterpriseMutationResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE xz_admin_enterprise_requests SET status=$2,result=$3::jsonb WHERE request_id=$1`, request.RequestID, result.Status, jsonProjection(result)); err != nil {
		return adminEnterpriseMutationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return adminEnterpriseMutationResult{}, err
	}
	return result, nil
}

func formatOptionalEnterpriseTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
