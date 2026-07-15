package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var adminEnterpriseRolePermissionMatrix = map[string][]string{
	"SUPER_ADMIN": adminEnterprisePermissions,
	"ENTERPRISE_OPERATOR": {
		permissionEnterpriseList, permissionEnterpriseDetail, permissionEnterpriseCreate, permissionEnterpriseUpdate,
		permissionEnterpriseMemberView, permissionEnterprisePackageView, permissionEnterprisePackageAdjust,
		permissionEnterpriseSeatAdjust, permissionEnterpriseComputeView, permissionEnterpriseComputeAdjust,
		permissionEnterpriseTransactionView, permissionEnterpriseOrderView, permissionEnterpriseAIView,
		permissionEnterpriseAIConfigure, permissionEnterpriseEmployeeView, permissionEnterpriseKnowledgeView,
		permissionEnterpriseAttributionView, permissionEnterpriseAttributionChange, permissionEnterpriseRiskView,
		permissionEnterpriseAuditView, permissionEnterpriseExport,
	},
	"CERTIFICATION_REVIEWER": {
		permissionEnterpriseList, permissionEnterpriseDetail, permissionEnterpriseCertificationReview,
		permissionEnterpriseAuditView, permissionEnterpriseExport,
	},
	"FINANCE": {
		permissionEnterpriseList, permissionEnterpriseDetail, permissionEnterprisePackageView,
		permissionEnterpriseComputeView, permissionEnterpriseComputeAdjust, permissionEnterpriseTransactionView,
		permissionEnterpriseOrderView, permissionEnterpriseAuditView, permissionEnterpriseExport,
	},
	"RISK_MANAGER": {
		permissionEnterpriseList, permissionEnterpriseDetail, permissionEnterpriseRiskView,
		permissionEnterpriseRiskDisable, permissionEnterpriseRiskRestore, permissionEnterpriseServiceTransition, permissionEnterpriseAuditView,
		permissionEnterpriseExport,
	},
	"CUSTOMER_SERVICE": {
		permissionEnterpriseList, permissionEnterpriseDetail, permissionEnterpriseMemberView,
		permissionEnterprisePackageView, permissionEnterpriseComputeView, permissionEnterpriseOrderView,
		permissionEnterpriseAIView, permissionEnterpriseEmployeeView, permissionEnterpriseKnowledgeView,
		permissionEnterpriseAttributionView, permissionEnterpriseRiskView,
	},
}

func ensureAdminEnterpriseSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		ALTER TABLE xz_tenants
			ADD COLUMN IF NOT EXISTS enterprise_code TEXT,
			ADD COLUMN IF NOT EXISTS source_agent_id TEXT,
			ADD COLUMN IF NOT EXISTS operation_center_id TEXT,
			ADD COLUMN IF NOT EXISTS seat_limit INT NOT NULL DEFAULT 20,
			ADD COLUMN IF NOT EXISTS industry TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS company_size TEXT NOT NULL DEFAULT '';
		UPDATE xz_tenants
		SET enterprise_code = 'ENT-' || upper(substr(md5(id), 1, 12))
		WHERE tenant_type='ENTERPRISE' AND coalesce(enterprise_code, '')='';
		CREATE UNIQUE INDEX IF NOT EXISTS idx_xz_tenants_enterprise_code
			ON xz_tenants(enterprise_code) WHERE tenant_type='ENTERPRISE';
		CREATE INDEX IF NOT EXISTS idx_xz_tenants_enterprise_admin_filter
			ON xz_tenants(tenant_type, status, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_xz_tenants_enterprise_relation
			ON xz_tenants(source_agent_id, operation_center_id) WHERE tenant_type='ENTERPRISE';
		ALTER TABLE xz_orders ADD COLUMN IF NOT EXISTS tenant_id TEXT;
		CREATE INDEX IF NOT EXISTS idx_xz_orders_tenant ON xz_orders(tenant_id, created_at DESC);
		WITH unique_enterprise_membership AS (
			SELECT member.user_id,min(member.tenant_id) AS tenant_id
			FROM xz_tenant_members member
			JOIN xz_tenants tenant ON tenant.id=member.tenant_id AND tenant.tenant_type='ENTERPRISE'
			WHERE upper(coalesce(nullif(member.member_status,''),member.status,'ACTIVE'))='ACTIVE'
			GROUP BY member.user_id
			HAVING count(DISTINCT member.tenant_id)=1
		)
		UPDATE xz_orders orders
		SET tenant_id=membership.tenant_id,
			price_snapshot=jsonb_set(coalesce(orders.price_snapshot,'{}'::jsonb),'{tenantId}',to_jsonb(membership.tenant_id),true),
			raw=jsonb_set(coalesce(orders.raw,'{}'::jsonb),'{tenantId}',to_jsonb(membership.tenant_id),true)
		FROM unique_enterprise_membership membership
		WHERE coalesce(orders.tenant_id,'')='' AND membership.user_id=orders.user_id;
		CREATE TABLE IF NOT EXISTS xz_tenant_point_transactions (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
			transaction_type TEXT NOT NULL,
			point_delta BIGINT NOT NULL,
			balance_after BIGINT NOT NULL,
			reference_type TEXT NOT NULL DEFAULT '',
			reference_id TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL,
			actor_user_id TEXT,
			request_id TEXT NOT NULL UNIQUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_xz_tenant_point_transactions_scope ON xz_tenant_point_transactions(tenant_id, created_at DESC);
		CREATE TABLE IF NOT EXISTS xz_admin_enterprise_change_requests (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
			action_type TEXT NOT NULL,
			reason TEXT NOT NULL,
			before_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
			after_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
			status TEXT NOT NULL DEFAULT 'PENDING_APPROVAL',
			requested_by TEXT,
			approved_by TEXT,
			request_id TEXT NOT NULL UNIQUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_xz_admin_enterprise_change_requests_scope ON xz_admin_enterprise_change_requests(tenant_id, status, created_at DESC);
		CREATE TABLE IF NOT EXISTS xz_admin_enterprise_requests (
			request_id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
			action TEXT NOT NULL,
			status TEXT NOT NULL,
			result JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE IF NOT EXISTS xz_admin_enterprise_risk_records (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
			risk_level TEXT NOT NULL DEFAULT 'MEDIUM',
			action TEXT NOT NULL,
			reason TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'ACTIVE',
			actor_user_id TEXT,
			request_id TEXT NOT NULL UNIQUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			resolved_at TIMESTAMPTZ
		);
		CREATE INDEX IF NOT EXISTS idx_xz_admin_enterprise_risk_records_scope ON xz_admin_enterprise_risk_records(tenant_id, created_at DESC);
	`); err != nil {
		return err
	}
	for role, permissions := range adminEnterpriseRolePermissionMatrix {
		for _, permission := range permissions {
			if _, err := db.ExecContext(ctx, `INSERT INTO xz_role_permissions(role,permission) VALUES($1,$2) ON CONFLICT DO NOTHING`, role, permission); err != nil {
				return err
			}
		}
	}
	return nil
}

const adminEnterpriseSelectSQL = `
	SELECT tenant.id,
	       coalesce(nullif(tenant.enterprise_code,''), tenant.id),
	       tenant.name,
	       coalesce(certification.status, nullif(tenant.config->>'certificationStatus',''), 'UNVERIFIED'),
	       coalesce(subscription.plan_id,''), coalesce(subscription.plan_code,'enterprise_trial'),
	       coalesce(plan.name, nullif(subscription.plan_code,''), '试用版'), coalesce(subscription.status,'TRIALING'), subscription.trial_expires_at,
	       coalesce(member_summary.member_count,0), coalesce(member_summary.active_member_count,0), greatest(tenant.seat_limit,1),
	       coalesce(wallet.point_balance,0), coalesce(wallet.frozen_points,0),
	       coalesce(agent.id,''), coalesce(agent_user.name,''),
	       coalesce(center.id,''), coalesce(center.name,''),
	       tenant.status, tenant.created_at, tenant.updated_at
	FROM xz_tenants tenant
	LEFT JOIN LATERAL (
		SELECT item.plan_id,item.plan_code,item.status,item.trial_expires_at
		FROM xz_tenant_subscriptions item WHERE item.tenant_id=tenant.id
		ORDER BY item.updated_at DESC,item.id DESC LIMIT 1
	) subscription ON true
	LEFT JOIN xz_plans plan ON plan.id=subscription.plan_id OR (subscription.plan_id IS NULL AND plan.code=subscription.plan_code)
	LEFT JOIN LATERAL (
		SELECT count(*)::int member_count,
		       count(*) FILTER(WHERE upper(coalesce(member.member_status,member.status,'ACTIVE'))='ACTIVE')::int active_member_count
		FROM xz_tenant_members member WHERE member.tenant_id=tenant.id
	) member_summary ON true
	LEFT JOIN xz_tenant_wallets wallet ON wallet.tenant_id=tenant.id
	LEFT JOIN xz_channel_agents agent ON agent.id=tenant.source_agent_id
	LEFT JOIN xz_users agent_user ON agent_user.id=agent.user_id
	LEFT JOIN xz_operation_centers center ON center.id=tenant.operation_center_id
	LEFT JOIN LATERAL (
		SELECT item.status FROM xz_tenant_certifications item WHERE item.tenant_id=tenant.id
		ORDER BY item.updated_at DESC,item.id DESC LIMIT 1
	) certification ON true
`

func (s *postgresStore) ListAdminEnterprises(query adminEnterpriseListQuery) (adminEnterpriseListResult, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminEnterpriseListResult{}, err
	}
	keyword := "%" + strings.ToLower(query.Keyword) + "%"
	var createdFrom any
	var createdTo any
	if query.CreatedFrom != nil {
		createdFrom = *query.CreatedFrom
	}
	if query.CreatedTo != nil {
		createdTo = *query.CreatedTo
	}
	filterSQL := `
		WHERE tenant.tenant_type='ENTERPRISE'
		  AND ($1='' OR lower(tenant.name) LIKE $2 OR lower(tenant.id) LIKE $2 OR lower(coalesce(tenant.enterprise_code,'')) LIKE $2)
		  AND ($3='' OR upper(coalesce(certification.status, nullif(tenant.config->>'certificationStatus',''), 'UNVERIFIED'))=$3)
		  AND ($4='' OR coalesce(subscription.plan_code,'enterprise_trial')=$4)
		  AND ($5='' OR upper(tenant.status)=$5)
		  AND ($6='' OR coalesce(tenant.source_agent_id,'')=$6)
		  AND ($7='' OR coalesce(tenant.operation_center_id,'')=$7)
		  AND ($8::timestamptz IS NULL OR tenant.created_at >= $8::timestamptz)
		  AND ($9::timestamptz IS NULL OR tenant.created_at <= $9::timestamptz)
	`
	args := []any{strings.ToLower(query.Keyword), keyword, query.Certification, query.PlanCode, query.Status, query.SourceAgentID, query.OperationCenterID, createdFrom, createdTo}
	var total int
	countSQL := `SELECT count(*) FROM (` + adminEnterpriseSelectSQL + filterSQL + `) filtered`
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return adminEnterpriseListResult{}, err
	}
	listSQL := adminEnterpriseSelectSQL + filterSQL + ` ORDER BY tenant.created_at DESC,tenant.id DESC LIMIT $10 OFFSET $11`
	rows, err := s.db.QueryContext(ctx, listSQL, append(args, query.PageSize, (query.Page-1)*query.PageSize)...)
	if err != nil {
		return adminEnterpriseListResult{}, err
	}
	defer rows.Close()
	items := make([]adminEnterpriseListItem, 0, query.PageSize)
	for rows.Next() {
		item, err := scanAdminEnterpriseListItem(rows)
		if err != nil {
			return adminEnterpriseListResult{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return adminEnterpriseListResult{}, err
	}
	stats, err := s.adminEnterpriseStats(ctx)
	if err != nil {
		return adminEnterpriseListResult{}, err
	}
	filters, err := s.adminEnterpriseFilters(ctx)
	if err != nil {
		return adminEnterpriseListResult{}, err
	}
	return adminEnterpriseListResult{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, Stats: stats, Filters: filters}, nil
}

func (s *postgresStore) GetAdminEnterprise(id string) (adminEnterpriseDetail, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminEnterpriseDetail{}, err
	}
	return s.getAdminEnterpriseContext(ctx, strings.TrimSpace(id))
}

func (s *postgresStore) getAdminEnterpriseContext(ctx context.Context, id string) (adminEnterpriseDetail, error) {
	row := s.db.QueryRowContext(ctx, adminEnterpriseSelectSQL+` WHERE tenant.tenant_type='ENTERPRISE' AND tenant.id=$1`, id)
	item, err := scanAdminEnterpriseListItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return adminEnterpriseDetail{}, errEnterpriseNotFound
	}
	if err != nil {
		return adminEnterpriseDetail{}, err
	}
	detail := adminEnterpriseDetail{Enterprise: item, Privacy: adminEnterprisePrivacyBoundary()}
	_ = s.db.QueryRowContext(ctx, `
		SELECT coalesce(certification.legal_name,''),coalesce(certification.unified_social_credit_code,''),
		       coalesce(certification.legal_representative_name,''),tenant.industry,tenant.company_size,coalesce(tenant.owner_user_id,''),
		       (SELECT count(*) FROM xz_organizations organization WHERE organization.tenant_id=tenant.id AND upper(organization.status)<>'DELETED')
		FROM xz_tenants tenant
		LEFT JOIN LATERAL (
			SELECT item.legal_name,item.unified_social_credit_code,item.legal_representative_name
			FROM xz_tenant_certifications item WHERE item.tenant_id=tenant.id ORDER BY item.updated_at DESC,item.id DESC LIMIT 1
		) certification ON true
		WHERE tenant.id=$1 AND tenant.tenant_type='ENTERPRISE'
	`, id).Scan(&detail.Profile.LegalName, &detail.Profile.UnifiedSocialCreditCode, &detail.Profile.LegalRepresentativeName, &detail.Profile.Industry, &detail.Profile.CompanySize, &detail.Profile.OwnerUserID, &detail.OrganizationCount)
	operationRows, err := s.db.QueryContext(ctx, `
		SELECT id,coalesce(actor_role,actor_user_id,''),action,coalesce(metadata->>'summary',''),created_at
		FROM xz_tenant_audit_logs WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 5
	`, id)
	if err == nil {
		defer operationRows.Close()
		for operationRows.Next() {
			var operation adminEnterpriseRecentOperation
			var createdAt time.Time
			if scanErr := operationRows.Scan(&operation.ID, &operation.Actor, &operation.Action, &operation.Summary, &createdAt); scanErr != nil {
				return adminEnterpriseDetail{}, scanErr
			}
			operation.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
			detail.RecentOperations = append(detail.RecentOperations, operation)
		}
	}
	return detail, nil
}

func (s *postgresStore) CreateAdminEnterprise(actorID string, actorRole string, request adminEnterpriseCreateRequest) (adminEnterpriseDetail, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminEnterpriseDetail{}, err
	}
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
	if request.PlanCode == "" {
		request.PlanCode = "enterprise_trial"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminEnterpriseDetail{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := validateAdminEnterpriseRelations(ctx, tx, request); err != nil {
		return adminEnterpriseDetail{}, err
	}
	tenantID := newEnterpriseResourceID("tenant")
	organizationID := newEnterpriseResourceID("organization")
	if request.EnterpriseCode == "" {
		request.EnterpriseCode = strings.ToUpper("ENT-" + strings.TrimPrefix(tenantID, "tenant_"))
	}
	var duplicated bool
	if err := tx.QueryRowContext(ctx, `SELECT exists(SELECT 1 FROM xz_tenants WHERE tenant_type='ENTERPRISE' AND (lower(name)=lower($1) OR enterprise_code=$2))`, request.Name, request.EnterpriseCode).Scan(&duplicated); err != nil {
		return adminEnterpriseDetail{}, err
	}
	if duplicated {
		return adminEnterpriseDetail{}, fmt.Errorf("%w: enterprise name or code already exists", errEnterpriseConflict)
	}
	now := time.Now().UTC()
	config := map[string]any{"certificationStatus": "UNVERIFIED"}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO xz_tenants(id,tenant_type,enterprise_ref,enterprise_code,owner_user_id,name,status,config,source_agent_id,operation_center_id,seat_limit,industry,company_size,created_at,updated_at)
		VALUES($1,'ENTERPRISE',$1,$2,nullif($3,''),$4,'ACTIVE',$5::jsonb,nullif($6,''),nullif($7,''),$8,$9,$10,$11,$11)
	`, tenantID, request.EnterpriseCode, request.OwnerUserID, request.Name, jsonProjection(config), request.SourceAgentID, request.OperationCenterID, request.SeatLimit, strings.TrimSpace(request.Industry), strings.TrimSpace(request.CompanySize), now)
	if err != nil {
		return adminEnterpriseDetail{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_organizations(id,tenant_id,parent_id,organization_type,name,status,metadata,created_at,updated_at) VALUES($1,$2,NULL,'ROOT',$3,'ACTIVE','{"root":true}'::jsonb,$4,$4)`, organizationID, tenantID, request.Name, now); err != nil {
		return adminEnterpriseDetail{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_tenant_wallets(tenant_id,point_balance,frozen_points,cash_balance_cents,status,created_at,updated_at) VALUES($1,0,0,0,'ACTIVE',$2,$2)`, tenantID, now); err != nil {
		return adminEnterpriseDetail{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_tenant_subscriptions(id,tenant_id,plan_id,plan_code,status,trial_started_at,trial_expires_at,entitlements,created_at,updated_at) VALUES($1,$2,nullif($3,''),$4,'TRIALING',$5,$6,$7::jsonb,$5,$5)`, newEnterpriseResourceID("tenant_subscription"), tenantID, request.PlanID, request.PlanCode, now, now.Add(14*24*time.Hour), jsonProjection(defaultEnterpriseTrialEntitlements())); err != nil {
		return adminEnterpriseDetail{}, err
	}
	if request.OwnerUserID != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO xz_tenant_members(id,tenant_id,user_id,role,status,primary_organization_id,member_status,certification_status,data_scope,joined_at,created_at,updated_at) VALUES($1,$2,$3,'ENTERPRISE_ADMIN','ACTIVE',$4,'ACTIVE','UNVERIFIED','TENANT_ALL',$5,$5,$5)`, newEnterpriseResourceID("tenant_member"), tenantID, request.OwnerUserID, organizationID, now); err != nil {
			return adminEnterpriseDetail{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO xz_user_roles(user_id,tenant_id,organization_id,role,status,assigned_at,updated_at) VALUES($1,$2,$3,'ENTERPRISE_ADMIN','ACTIVE',$4,$4) ON CONFLICT(user_id,tenant_id,organization_id,role) DO UPDATE SET status='ACTIVE',updated_at=excluded.updated_at`, request.OwnerUserID, tenantID, organizationID, now); err != nil {
			return adminEnterpriseDetail{}, err
		}
	}
	access := enterpriseAccess{UserID: actorID, TenantID: tenantID, OrganizationID: organizationID, Role: actorRole}
	if err := insertTenantAuditTx(ctx, tx, access, "admin.enterprise.create", "tenant", tenantID, request.OwnerUserID, map[string]any{"name": request.Name, "enterpriseCode": request.EnterpriseCode, "summary": "创建企业"}); err != nil {
		return adminEnterpriseDetail{}, err
	}
	if err := insertAuditLog(ctx, tx, actorID, actorRole, "admin.enterprise.create", "tenant", tenantID, "POST", "/api/v1/admin/enterprises", httpStatusCreated, map[string]any{"enterpriseCode": request.EnterpriseCode}); err != nil {
		return adminEnterpriseDetail{}, err
	}
	if err := tx.Commit(); err != nil {
		return adminEnterpriseDetail{}, err
	}
	return s.getAdminEnterpriseContext(ctx, tenantID)
}

const httpStatusCreated = 201

func validateAdminEnterpriseRelations(ctx context.Context, tx *sql.Tx, request adminEnterpriseCreateRequest) error {
	checks := []struct {
		value string
		query string
		name  string
	}{
		{request.OwnerUserID, `SELECT exists(SELECT 1 FROM xz_users WHERE id=$1 AND upper(coalesce(status,'ACTIVE'))='ACTIVE')`, "owner user"},
		{request.SourceAgentID, `SELECT exists(SELECT 1 FROM xz_channel_agents WHERE id=$1)`, "source agent"},
		{request.OperationCenterID, `SELECT exists(SELECT 1 FROM xz_operation_centers WHERE id=$1)`, "operation center"},
		{request.PlanID, `SELECT exists(SELECT 1 FROM xz_plans WHERE id=$1 AND active=true)`, "plan"},
	}
	for _, check := range checks {
		if check.value == "" {
			continue
		}
		var found bool
		if err := tx.QueryRowContext(ctx, check.query, check.value).Scan(&found); err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%w: %s not found", errEnterpriseInvalid, check.name)
		}
	}
	return nil
}

func scanAdminEnterpriseListItem(scanner sqlRowScanner) (adminEnterpriseListItem, error) {
	var item adminEnterpriseListItem
	var planExpires sql.NullTime
	var createdAt, updatedAt time.Time
	err := scanner.Scan(
		&item.ID, &item.EnterpriseCode, &item.Name, &item.CertificationStatus,
		&item.Plan.ID, &item.Plan.Code, &item.Plan.Name, &item.Plan.Status, &planExpires,
		&item.MemberCount, &item.ActiveMemberCount, &item.SeatLimit,
		&item.Compute.Balance, &item.Compute.Frozen,
		&item.SourceAgent.ID, &item.SourceAgent.Name, &item.OperationCenter.ID, &item.OperationCenter.Name,
		&item.Status, &createdAt, &updatedAt,
	)
	if err != nil {
		return adminEnterpriseListItem{}, err
	}
	item.Compute.Unit = "POINT"
	if planExpires.Valid {
		item.Plan.ExpiresAt = planExpires.Time.UTC().Format(time.RFC3339Nano)
	}
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	return item, nil
}

func (s *postgresStore) adminEnterpriseStats(ctx context.Context) (adminEnterpriseStats, error) {
	var result adminEnterpriseStats
	err := s.db.QueryRowContext(ctx, `
		SELECT count(*)::int,
		       count(*) FILTER(WHERE upper(coalesce(certification.status,nullif(tenant.config->>'certificationStatus',''),'UNVERIFIED')) IN ('APPROVED','CERTIFIED'))::int,
		       count(*) FILTER(WHERE tenant.created_at >= date_trunc('month',now()))::int,
		       count(*) FILTER(WHERE upper(tenant.status)<>'ACTIVE')::int
		FROM xz_tenants tenant
		LEFT JOIN LATERAL (SELECT item.status FROM xz_tenant_certifications item WHERE item.tenant_id=tenant.id ORDER BY item.updated_at DESC LIMIT 1) certification ON true
		WHERE tenant.tenant_type='ENTERPRISE'
	`).Scan(&result.Total, &result.Certified, &result.CreatedThisMonth, &result.Abnormal)
	return result, err
}

func (s *postgresStore) adminEnterpriseFilters(ctx context.Context) (adminEnterpriseFilters, error) {
	result := adminEnterpriseFilters{Plans: []adminEnterpriseFilterOption{}, SourceAgents: []adminEnterpriseFilterOption{}, OperationCenters: []adminEnterpriseFilterOption{}}
	planRows, err := s.db.QueryContext(ctx, `SELECT code,name FROM xz_plans WHERE active=true ORDER BY name,code`)
	if err != nil {
		return result, err
	}
	for planRows.Next() {
		var option adminEnterpriseFilterOption
		if err := planRows.Scan(&option.Value, &option.Label); err != nil {
			_ = planRows.Close()
			return result, err
		}
		result.Plans = append(result.Plans, option)
	}
	_ = planRows.Close()
	agentRows, err := s.db.QueryContext(ctx, `SELECT agent.id,coalesce(user_account.name,agent.id) FROM xz_channel_agents agent LEFT JOIN xz_users user_account ON user_account.id=agent.user_id WHERE upper(coalesce(agent.status,'ACTIVE'))<>'DISABLED' ORDER BY 2,1`)
	if err != nil {
		return result, err
	}
	for agentRows.Next() {
		var option adminEnterpriseFilterOption
		if err := agentRows.Scan(&option.Value, &option.Label); err != nil {
			_ = agentRows.Close()
			return result, err
		}
		result.SourceAgents = append(result.SourceAgents, option)
	}
	_ = agentRows.Close()
	centerRows, err := s.db.QueryContext(ctx, `SELECT id,coalesce(name,id) FROM xz_operation_centers WHERE upper(coalesce(status,'ACTIVE'))<>'DISABLED' ORDER BY 2,1`)
	if err != nil {
		return result, err
	}
	for centerRows.Next() {
		var option adminEnterpriseFilterOption
		if err := centerRows.Scan(&option.Value, &option.Label); err != nil {
			_ = centerRows.Close()
			return result, err
		}
		result.OperationCenters = append(result.OperationCenters, option)
	}
	_ = centerRows.Close()
	return result, nil
}
