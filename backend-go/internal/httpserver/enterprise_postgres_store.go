package httpserver

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func ensureEnterpriseCenterSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS xz_tenant_members (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
			user_id TEXT NOT NULL REFERENCES xz_users(id),
			role TEXT NOT NULL DEFAULT 'MEMBER',
			status TEXT NOT NULL DEFAULT 'ACTIVE',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (tenant_id, user_id),
			UNIQUE (tenant_id, id)
		);
		ALTER TABLE xz_tenant_members
			ADD COLUMN IF NOT EXISTS primary_organization_id TEXT,
			ADD COLUMN IF NOT EXISTS member_status TEXT NOT NULL DEFAULT 'ACTIVE',
			ADD COLUMN IF NOT EXISTS certification_status TEXT NOT NULL DEFAULT 'UNVERIFIED',
			ADD COLUMN IF NOT EXISTS data_scope TEXT NOT NULL DEFAULT 'SELF',
			ADD COLUMN IF NOT EXISTS joined_at TIMESTAMPTZ,
			ADD COLUMN IF NOT EXISTS invited_by TEXT REFERENCES xz_users(id),
			ADD COLUMN IF NOT EXISTS disabled_at TIMESTAMPTZ,
			ADD COLUMN IF NOT EXISTS disabled_by TEXT REFERENCES xz_users(id),
			ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;
		ALTER TABLE xz_user_role_context ADD COLUMN IF NOT EXISTS context_type TEXT NOT NULL DEFAULT 'PERSONAL';
		CREATE TABLE IF NOT EXISTS xz_tenant_invitations (
			id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES xz_tenants(id), invitation_code TEXT NOT NULL UNIQUE,
			invited_user_id TEXT REFERENCES xz_users(id), invited_email TEXT, default_organization_id TEXT,
			default_role TEXT NOT NULL DEFAULT 'ENTERPRISE_MEMBER', expires_at TIMESTAMPTZ NOT NULL,
			status TEXT NOT NULL DEFAULT 'PENDING', created_by TEXT NOT NULL REFERENCES xz_users(id),
			accepted_by TEXT REFERENCES xz_users(id), accepted_at TIMESTAMPTZ, metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id, id)
		);
		CREATE TABLE IF NOT EXISTS xz_tenant_join_requests (
			id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES xz_tenants(id), applicant_user_id TEXT NOT NULL REFERENCES xz_users(id),
			requested_organization_id TEXT, requested_role TEXT NOT NULL DEFAULT 'ENTERPRISE_MEMBER', reason TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'PENDING', reviewed_by TEXT REFERENCES xz_users(id), reviewed_at TIMESTAMPTZ,
			review_comment TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (tenant_id, id)
		);
		CREATE TABLE IF NOT EXISTS xz_tenant_wallets (
			tenant_id TEXT PRIMARY KEY REFERENCES xz_tenants(id), point_balance BIGINT NOT NULL DEFAULT 0,
			frozen_points BIGINT NOT NULL DEFAULT 0, cash_balance_cents BIGINT NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'ACTIVE', metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE IF NOT EXISTS xz_tenant_subscriptions (
			id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES xz_tenants(id), plan_id TEXT REFERENCES xz_plans(id),
			plan_code TEXT NOT NULL DEFAULT 'enterprise_trial', status TEXT NOT NULL DEFAULT 'TRIALING',
			trial_started_at TIMESTAMPTZ NOT NULL DEFAULT now(), trial_expires_at TIMESTAMPTZ,
			entitlements JSONB NOT NULL DEFAULT '{}'::jsonb, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id, id)
		);
		CREATE TABLE IF NOT EXISTS xz_tenant_audit_logs (
			id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES xz_tenants(id), actor_user_id TEXT REFERENCES xz_users(id),
			actor_role TEXT, organization_id TEXT, action TEXT NOT NULL, resource_type TEXT NOT NULL, resource_id TEXT,
			target_user_id TEXT REFERENCES xz_users(id), request_id TEXT, ip_address TEXT, status TEXT NOT NULL DEFAULT 'SUCCEEDED',
			metadata JSONB NOT NULL DEFAULT '{}'::jsonb, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE IF NOT EXISTS xz_tenant_certifications (
			id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES xz_tenants(id), legal_name TEXT NOT NULL,
			unified_social_credit_code TEXT NOT NULL, legal_representative_name TEXT NOT NULL DEFAULT '',
			document_urls JSONB NOT NULL DEFAULT '[]'::jsonb, status TEXT NOT NULL DEFAULT 'PENDING',
			submitted_by TEXT NOT NULL REFERENCES xz_users(id), reviewed_by TEXT REFERENCES xz_users(id),
			reviewed_at TIMESTAMPTZ, review_comment TEXT NOT NULL DEFAULT '', metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id, id)
		);
		CREATE INDEX IF NOT EXISTS idx_xz_tenant_members_scope ON xz_tenant_members(tenant_id, member_status, primary_organization_id, user_id);
		CREATE INDEX IF NOT EXISTS idx_xz_tenant_invitations_tenant_status ON xz_tenant_invitations(tenant_id, status, expires_at DESC);
		CREATE INDEX IF NOT EXISTS idx_xz_tenant_join_requests_tenant_status ON xz_tenant_join_requests(tenant_id, status, created_at DESC);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_xz_tenant_join_requests_pending_user ON xz_tenant_join_requests(tenant_id, applicant_user_id) WHERE upper(status) = 'PENDING';
		CREATE INDEX IF NOT EXISTS idx_xz_tenant_audit_logs_scope ON xz_tenant_audit_logs(tenant_id, created_at DESC, action);
		CREATE INDEX IF NOT EXISTS idx_xz_tenant_certifications_scope ON xz_tenant_certifications(tenant_id, status, updated_at DESC);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_xz_tenant_certifications_pending ON xz_tenant_certifications(tenant_id) WHERE upper(status) = 'PENDING';
	`)
	if err != nil {
		return err
	}
	for role, permissions := range rolePermissionMatrix {
		for _, permission := range permissions {
			if _, err := db.ExecContext(ctx, `INSERT INTO xz_role_permissions(role, permission) VALUES ($1,$2) ON CONFLICT DO NOTHING`, role, permission); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *postgresStore) EnterpriseContexts(userID string) ([]enterpriseContext, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	return s.enterpriseContextsContext(ctx, userID)
}

func (s *postgresStore) enterpriseContextsContext(ctx context.Context, userID string) ([]enterpriseContext, error) {
	var userName string
	if err := s.db.QueryRowContext(ctx, `SELECT coalesce(name, '') FROM xz_users WHERE id=$1 AND upper(coalesce(status,''))='ACTIVE'`, userID).Scan(&userName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errUnauthorized
		}
		return nil, err
	}
	current := enterpriseCurrentState{Type: contextPersonal, TenantID: "tenant_default", OrganizationID: defaultOrganizationID("tenant_default"), CurrentRole: roleUser}
	_ = s.db.QueryRowContext(ctx, `SELECT context_type, tenant_id, organization_id, current_role_code FROM xz_user_role_context WHERE user_id=$1`, userID).Scan(&current.Type, &current.TenantID, &current.OrganizationID, &current.CurrentRole)
	items := []enterpriseContext{{
		Type: contextPersonal, TenantID: "tenant_default", TenantName: "个人空间", OrganizationID: defaultOrganizationID("tenant_default"), OrganizationName: "个人空间",
		MemberStatus: "ACTIVE", CertificationStatus: "NOT_REQUIRED", Roles: []string{roleUser}, CurrentRole: roleUser,
		Permissions: permissionsForCurrentRole(roleUser), DataScope: dataScopeSelf, Entitlements: map[string]any{"identity": contextPersonal}, Current: current.Type == contextPersonal,
	}}
	var pointBalance, frozenPoints int64
	_ = s.db.QueryRowContext(ctx, `SELECT available, frozen FROM xz_point_accounts WHERE user_id=$1`, userID).Scan(&pointBalance, &frozenPoints)
	items[0].Wallet = enterpriseWalletSummary{PointBalance: pointBalance, FrozenPoints: frozenPoints, Status: "ACTIVE"}
	for _, identity := range []struct{ Role, Type, Name string }{{roleAgent, contextAgent, "代理商工作台"}, {roleOperation, contextOperation, "运营中心"}} {
		var exists bool
		if err := s.db.QueryRowContext(ctx, `SELECT exists(SELECT 1 FROM xz_user_roles WHERE user_id=$1 AND role=$2 AND upper(status)='ACTIVE')`, userID, identity.Role).Scan(&exists); err != nil {
			return nil, err
		}
		if exists {
			items = append(items, enterpriseContext{Type: identity.Type, TenantID: "tenant_default", TenantName: "渠道空间", OrganizationID: defaultOrganizationID("tenant_default"), OrganizationName: identity.Name, MemberStatus: "ACTIVE", CertificationStatus: "NOT_REQUIRED", Roles: []string{identity.Role}, CurrentRole: identity.Role, Permissions: permissionsForCurrentRole(identity.Role), DataScope: dataScopeSelf, Entitlements: map[string]any{"identity": identity.Type}, Current: current.Type == identity.Type})
		}
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT tenant.id, tenant.name, organization.id, organization.name,
		       member.member_status, coalesce(tenant.config->>'certificationStatus','UNVERIFIED'), member.data_scope,
		       coalesce(string_agg(DISTINCT role.role, ',' ORDER BY role.role) FILTER (WHERE upper(role.status)='ACTIVE'), ''),
		       coalesce(wallet.point_balance,0), coalesce(wallet.frozen_points,0), coalesce(wallet.cash_balance_cents,0), coalesce(wallet.status,'ACTIVE'),
		       coalesce(subscription.id,''), coalesce(subscription.plan_id,''), coalesce(subscription.plan_code,'enterprise_trial'),
		       coalesce(subscription.status,'TRIALING'), subscription.trial_expires_at, coalesce(subscription.entitlements,'{}'::jsonb)
		FROM xz_tenant_members member
		JOIN xz_tenants tenant ON tenant.id=member.tenant_id AND tenant.tenant_type='ENTERPRISE' AND upper(tenant.status)='ACTIVE'
		JOIN xz_organizations organization ON organization.tenant_id=member.tenant_id AND organization.id=member.primary_organization_id AND upper(organization.status)='ACTIVE'
		LEFT JOIN xz_user_roles role ON role.user_id=member.user_id AND role.tenant_id=member.tenant_id
		  AND role.role IN ('ENTERPRISE_ADMIN','AI_ADMIN','FINANCE','CUSTOMER_SERVICE','ENTERPRISE_MEMBER')
		LEFT JOIN xz_tenant_wallets wallet ON wallet.tenant_id=tenant.id
		LEFT JOIN LATERAL (
			SELECT item.* FROM xz_tenant_subscriptions item WHERE item.tenant_id=tenant.id ORDER BY item.updated_at DESC LIMIT 1
		) subscription ON true
		WHERE member.user_id=$1 AND upper(member.member_status)='ACTIVE'
		GROUP BY tenant.id, tenant.name, organization.id, organization.name, member.member_status, member.data_scope,
		         wallet.point_balance, wallet.frozen_points, wallet.cash_balance_cents, wallet.status,
		         subscription.id, subscription.plan_id, subscription.plan_code, subscription.status, subscription.trial_expires_at, subscription.entitlements
		ORDER BY tenant.created_at, tenant.id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item enterpriseContext
		var roleCSV, subscriptionID, planID, planCode, subscriptionStatus string
		var trialExpiresAt sql.NullTime
		var entitlementRaw []byte
		if err := rows.Scan(&item.TenantID, &item.TenantName, &item.OrganizationID, &item.OrganizationName, &item.MemberStatus, &item.CertificationStatus, &item.DataScope, &roleCSV, &item.Wallet.PointBalance, &item.Wallet.FrozenPoints, &item.Wallet.CashBalanceCents, &item.Wallet.Status, &subscriptionID, &planID, &planCode, &subscriptionStatus, &trialExpiresAt, &entitlementRaw); err != nil {
			return nil, err
		}
		item.Type = contextEnterprise
		item.Roles = normalizeEnterpriseRoles(splitCSV(roleCSV))
		if len(item.Roles) == 0 {
			item.Roles = []string{roleEnterpriseMember}
		}
		item.CurrentRole = item.Roles[0]
		item.Current = current.Type == contextEnterprise && current.TenantID == item.TenantID
		if item.Current && containsString(item.Roles, normalizeAppRole(current.CurrentRole)) {
			item.CurrentRole = normalizeAppRole(current.CurrentRole)
		}
		item.Permissions = permissionsForCurrentRole(item.CurrentRole)
		item.DataScope = normalizedDataScope(item.DataScope, item.CurrentRole)
		item.Entitlements = decodeJSONMap(entitlementRaw)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	marked := false
	for _, item := range items {
		marked = marked || item.Current
	}
	if !marked && len(items) > 0 {
		items[0].Current = true
	}
	return items, nil
}

func (s *postgresStore) CurrentEnterpriseContext(userID string) (enterpriseContext, error) {
	items, err := s.EnterpriseContexts(userID)
	if err != nil {
		return enterpriseContext{}, err
	}
	for _, item := range items {
		if item.Current {
			return item, nil
		}
	}
	return enterpriseContext{}, errForbidden
}

func (s *postgresStore) SetEnterpriseCurrentContext(userID string, request enterpriseContextSwitchRequest) (enterpriseContext, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return enterpriseContext{}, err
	}
	request.Type = strings.ToUpper(strings.TrimSpace(request.Type))
	request.TenantID = strings.TrimSpace(request.TenantID)
	request.OrganizationID = strings.TrimSpace(request.OrganizationID)
	request.Role = normalizeAppRole(request.Role)
	items, err := s.enterpriseContextsContext(ctx, userID)
	if err != nil {
		return enterpriseContext{}, err
	}
	for _, item := range items {
		if item.Type != request.Type || (request.Type == contextEnterprise && item.TenantID != request.TenantID) {
			continue
		}
		if request.Role != "" {
			if !containsString(item.Roles, request.Role) {
				return enterpriseContext{}, errForbidden
			}
			item.CurrentRole = request.Role
			item.Permissions = permissionsForCurrentRole(request.Role)
		}
		if request.OrganizationID != "" && request.OrganizationID != item.OrganizationID {
			allowed, err := s.postgresOrganizationAllowed(ctx, item.TenantID, item.OrganizationID, request.OrganizationID, item.DataScope)
			if err != nil {
				return enterpriseContext{}, err
			}
			if !allowed {
				return enterpriseContext{}, errForbidden
			}
			if err := s.db.QueryRowContext(ctx, `SELECT name FROM xz_organizations WHERE tenant_id=$1 AND id=$2 AND upper(status)='ACTIVE'`, item.TenantID, request.OrganizationID).Scan(&item.OrganizationName); err != nil {
				return enterpriseContext{}, errForbidden
			}
			item.OrganizationID = request.OrganizationID
		}
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO xz_user_role_context(user_id,tenant_id,organization_id,current_role_code,context_type,updated_at)
			VALUES($1,$2,$3,$4,$5,now())
			ON CONFLICT(user_id) DO UPDATE SET tenant_id=excluded.tenant_id, organization_id=excluded.organization_id,
			 current_role_code=excluded.current_role_code, context_type=excluded.context_type, updated_at=now()
		`, userID, item.TenantID, item.OrganizationID, item.CurrentRole, item.Type)
		if err != nil {
			return enterpriseContext{}, err
		}
		item.Current = true
		return item, nil
	}
	return enterpriseContext{}, errForbidden
}

func (s *postgresStore) CreateEnterprise(userID string, request enterpriseCreateRequest) (enterpriseCreateResult, error) {
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || len([]rune(request.Name)) > 160 {
		return enterpriseCreateResult{}, fmt.Errorf("%w: enterprise name is required and must not exceed 160 characters", errEnterpriseInvalid)
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return enterpriseCreateResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return enterpriseCreateResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var userName string
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(name,'') FROM xz_users WHERE id=$1 AND upper(status)='ACTIVE'`, userID).Scan(&userName); err != nil {
		return enterpriseCreateResult{}, errUnauthorized
	}
	now := time.Now().UTC()
	tenantID := newEnterpriseResourceID("tenant")
	organizationID := newEnterpriseResourceID("organization")
	memberID := newEnterpriseResourceID("tenant_member")
	subscriptionID := newEnterpriseResourceID("tenant_subscription")
	invitationID := newEnterpriseResourceID("tenant_invitation")
	invitationCode := newEnterpriseInvitationCode()
	trialExpires := now.Add(14 * 24 * time.Hour)
	invitationExpires := now.Add(7 * 24 * time.Hour)
	entitlements := defaultEnterpriseTrialEntitlements()
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_tenants(id,tenant_type,enterprise_ref,owner_user_id,name,status,config,created_at,updated_at) VALUES($1,'ENTERPRISE',$1,$2,$3,'ACTIVE',$4::jsonb,$5,$5)`, tenantID, userID, request.Name, jsonProjection(map[string]any{"certificationStatus": "UNVERIFIED"}), now); err != nil {
		return enterpriseCreateResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_organizations(id,tenant_id,parent_id,organization_type,name,status,metadata,created_at,updated_at) VALUES($1,$2,NULL,'ROOT',$3,'ACTIVE','{"root":true}'::jsonb,$4,$4)`, organizationID, tenantID, request.Name, now); err != nil {
		return enterpriseCreateResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_tenant_members(id,tenant_id,user_id,role,status,primary_organization_id,member_status,certification_status,data_scope,joined_at,created_at,updated_at) VALUES($1,$2,$3,'ENTERPRISE_ADMIN','ACTIVE',$4,'ACTIVE','UNVERIFIED','TENANT_ALL',$5,$5,$5)`, memberID, tenantID, userID, organizationID, now); err != nil {
		return enterpriseCreateResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_user_roles(user_id,tenant_id,organization_id,role,status,assigned_at,updated_at) VALUES($1,$2,$3,'ENTERPRISE_ADMIN','ACTIVE',$4,$4) ON CONFLICT(user_id,tenant_id,organization_id,role) DO UPDATE SET status='ACTIVE',updated_at=excluded.updated_at`, userID, tenantID, organizationID, now); err != nil {
		return enterpriseCreateResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_user_role_context(user_id,tenant_id,organization_id,current_role_code,context_type,updated_at) VALUES($1,$2,$3,'ENTERPRISE_ADMIN','ENTERPRISE',$4) ON CONFLICT(user_id) DO UPDATE SET tenant_id=excluded.tenant_id,organization_id=excluded.organization_id,current_role_code=excluded.current_role_code,context_type='ENTERPRISE',updated_at=excluded.updated_at`, userID, tenantID, organizationID, now); err != nil {
		return enterpriseCreateResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_tenant_wallets(tenant_id,point_balance,status,created_at,updated_at) VALUES($1,1000,'ACTIVE',$2,$2)`, tenantID, now); err != nil {
		return enterpriseCreateResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_tenant_subscriptions(id,tenant_id,plan_code,status,trial_started_at,trial_expires_at,entitlements,created_at,updated_at) VALUES($1,$2,'enterprise_trial','TRIALING',$3,$4,$5::jsonb,$3,$3)`, subscriptionID, tenantID, now, trialExpires, jsonProjection(entitlements)); err != nil {
		return enterpriseCreateResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_tenant_invitations(id,tenant_id,invitation_code,default_organization_id,default_role,expires_at,status,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,'ENTERPRISE_MEMBER',$5,'PENDING',$6,$7,$7)`, invitationID, tenantID, invitationCode, organizationID, invitationExpires, userID, now); err != nil {
		return enterpriseCreateResult{}, err
	}
	access := enterpriseAccess{UserID: userID, TenantID: tenantID, OrganizationID: organizationID, Role: roleEnterpriseAdmin}
	if err := insertTenantAuditTx(ctx, tx, access, "enterprise.create", "tenant", tenantID, userID, map[string]any{"name": request.Name}); err != nil {
		return enterpriseCreateResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return enterpriseCreateResult{}, err
	}
	tenant := enterpriseTenant{ID: tenantID, Name: request.Name, OwnerUserID: userID, Status: "ACTIVE", CertificationStatus: "UNVERIFIED", Config: map[string]any{"certificationStatus": "UNVERIFIED"}, CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)}
	organization := enterpriseOrganization{ID: organizationID, TenantID: tenantID, OrganizationType: "ROOT", Name: request.Name, Status: "ACTIVE", Metadata: map[string]any{"root": true}, CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)}
	wallet := enterpriseWalletSummary{PointBalance: 1000, Status: "ACTIVE"}
	contextItem := enterpriseContext{Type: contextEnterprise, TenantID: tenantID, TenantName: request.Name, OrganizationID: organizationID, OrganizationName: request.Name, MemberStatus: "ACTIVE", CertificationStatus: "UNVERIFIED", Roles: []string{roleEnterpriseAdmin}, CurrentRole: roleEnterpriseAdmin, Permissions: permissionsForCurrentRole(roleEnterpriseAdmin), DataScope: dataScopeTenantAll, Entitlements: entitlements, Wallet: wallet, Current: true}
	invitation := enterpriseInvitation{ID: invitationID, TenantID: tenantID, InvitationCode: invitationCode, DefaultOrganizationID: organizationID, DefaultRole: roleEnterpriseMember, ExpiresAt: invitationExpires.Format(time.RFC3339Nano), Status: "PENDING", CreatedBy: userID, CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)}
	_ = userName
	return enterpriseCreateResult{Tenant: tenant, Context: contextItem, Invitation: invitation, Organization: organization}, nil
}

func (s *postgresStore) EnterpriseAccess(userID string, permission string) (enterpriseAccess, error) {
	current, err := s.CurrentEnterpriseContext(userID)
	if err != nil {
		return enterpriseAccess{}, err
	}
	if current.Type != contextEnterprise || !strings.EqualFold(current.MemberStatus, "ACTIVE") || !containsString(current.Permissions, permission) {
		return enterpriseAccess{}, errForbidden
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	var memberID string
	var dbPermission, serviceActive bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT member.id,
		       exists(
		         SELECT 1 FROM xz_user_roles role
		         JOIN xz_role_permissions permission ON permission.role=role.role
		         WHERE role.user_id=$2 AND role.tenant_id=$1 AND role.role=$3
		           AND upper(role.status)='ACTIVE' AND permission.permission=$4
		       ),
		       coalesce(service.lifecycle_state='ACTIVE' AND service.status='ACTIVE', true)
		FROM xz_tenant_members member
		JOIN xz_tenants tenant ON tenant.id=member.tenant_id AND tenant.tenant_type='ENTERPRISE' AND upper(tenant.status)='ACTIVE'
		LEFT JOIN xz_tenant_service_states service ON service.tenant_id=tenant.id
		WHERE member.tenant_id=$1 AND member.user_id=$2 AND upper(member.member_status)='ACTIVE'
	`, current.TenantID, userID, current.CurrentRole, permission).Scan(&memberID, &dbPermission, &serviceActive); err != nil {
		return enterpriseAccess{}, errForbidden
	}
	if !dbPermission || !serviceActive {
		return enterpriseAccess{}, errForbidden
	}
	return enterpriseAccess{UserID: userID, TenantID: current.TenantID, TenantName: current.TenantName, OrganizationID: current.OrganizationID, MemberID: memberID, Role: current.CurrentRole, Roles: current.Roles, Permissions: current.Permissions, DataScope: current.DataScope}, nil
}

func (s *postgresStore) EnterpriseOverview(access enterpriseAccess) (enterpriseOverview, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	var result enterpriseOverview
	var configRaw, entitlementRaw []byte
	var createdAt, updatedAt time.Time
	if err := s.db.QueryRowContext(ctx, `SELECT id,name,coalesce(owner_user_id,''),status,config,created_at,updated_at FROM xz_tenants WHERE id=$1 AND tenant_type='ENTERPRISE'`, access.TenantID).Scan(&result.Tenant.ID, &result.Tenant.Name, &result.Tenant.OwnerUserID, &result.Tenant.Status, &configRaw, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return result, errEnterpriseNotFound
		}
		return result, err
	}
	result.Tenant.Config = decodeJSONMap(configRaw)
	result.Tenant.CertificationStatus = stringValue(result.Tenant.Config["certificationStatus"])
	result.Tenant.CreatedAt, result.Tenant.UpdatedAt = createdAt.Format(time.RFC3339Nano), updatedAt.Format(time.RFC3339Nano)
	if err := s.db.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE upper(member_status)='ACTIVE') FROM xz_tenant_members WHERE tenant_id=$1`, access.TenantID).Scan(&result.MemberCount, &result.ActiveMembers); err != nil {
		return result, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM xz_organizations WHERE tenant_id=$1 AND upper(status)<>'DELETED'`, access.TenantID).Scan(&result.OrganizationCount); err != nil {
		return result, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM xz_tenant_join_requests WHERE tenant_id=$1 AND upper(status)='PENDING'`, access.TenantID).Scan(&result.PendingJoinRequests); err != nil {
		return result, err
	}
	_ = s.db.QueryRowContext(ctx, `SELECT point_balance,frozen_points,cash_balance_cents,status FROM xz_tenant_wallets WHERE tenant_id=$1`, access.TenantID).Scan(&result.Wallet.PointBalance, &result.Wallet.FrozenPoints, &result.Wallet.CashBalanceCents, &result.Wallet.Status)
	var trialExpires sql.NullTime
	_ = s.db.QueryRowContext(ctx, `SELECT id,coalesce(plan_id,''),plan_code,status,trial_expires_at,entitlements FROM xz_tenant_subscriptions WHERE tenant_id=$1 ORDER BY updated_at DESC LIMIT 1`, access.TenantID).Scan(&result.Subscription.ID, &result.Subscription.PlanID, &result.Subscription.PlanCode, &result.Subscription.Status, &trialExpires, &entitlementRaw)
	if trialExpires.Valid {
		result.Subscription.TrialExpiresAt = trialExpires.Time.Format(time.RFC3339Nano)
	}
	result.Subscription.Entitlements = decodeJSONMap(entitlementRaw)
	result.Current, _ = s.CurrentEnterpriseContext(access.UserID)
	return result, nil
}

func (s *postgresStore) ListEnterpriseMembers(access enterpriseAccess) ([]enterpriseMember, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	items, err := queryPostgresEnterpriseMembers(ctx, s.db, access.TenantID)
	if err != nil {
		return nil, err
	}
	allowed, err := s.postgresAllowedOrganizations(ctx, access)
	if err != nil {
		return nil, err
	}
	filtered := make([]enterpriseMember, 0, len(items))
	for _, item := range items {
		if memoryMemberVisible(access, item, allowed) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (s *postgresStore) GetEnterpriseMember(access enterpriseAccess, id string) (enterpriseMember, error) {
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

func (s *postgresStore) CreateEnterpriseInvitation(access enterpriseAccess, request enterpriseInvitationCreateRequest) (enterpriseInvitation, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	request.InvitedUserID = strings.TrimSpace(request.InvitedUserID)
	request.InvitedEmail = strings.ToLower(strings.TrimSpace(request.InvitedEmail))
	request.DefaultOrganizationID = firstNonEmptyString(strings.TrimSpace(request.DefaultOrganizationID), access.OrganizationID)
	request.DefaultRole = normalizeEnterpriseRole(request.DefaultRole)
	if request.DefaultRole == "" {
		request.DefaultRole = roleEnterpriseMember
	}
	if request.ExpiresInHours <= 0 || request.ExpiresInHours > 24*30 {
		request.ExpiresInHours = 24 * 7
	}
	var organizationExists bool
	if err := s.db.QueryRowContext(ctx, `SELECT exists(SELECT 1 FROM xz_organizations WHERE tenant_id=$1 AND id=$2 AND upper(status)='ACTIVE')`, access.TenantID, request.DefaultOrganizationID).Scan(&organizationExists); err != nil || !organizationExists {
		return enterpriseInvitation{}, errEnterpriseNotFound
	}
	if request.InvitedUserID != "" {
		var userExists bool
		if err := s.db.QueryRowContext(ctx, `SELECT exists(SELECT 1 FROM xz_users WHERE id=$1 AND upper(status)='ACTIVE')`, request.InvitedUserID).Scan(&userExists); err != nil || !userExists {
			return enterpriseInvitation{}, errEnterpriseNotFound
		}
	}
	now := time.Now().UTC()
	item := enterpriseInvitation{ID: newEnterpriseResourceID("tenant_invitation"), TenantID: access.TenantID, InvitationCode: newEnterpriseInvitationCode(), InvitedUserID: request.InvitedUserID, InvitedEmail: request.InvitedEmail, DefaultOrganizationID: request.DefaultOrganizationID, DefaultRole: request.DefaultRole, ExpiresAt: now.Add(time.Duration(request.ExpiresInHours) * time.Hour).Format(time.RFC3339Nano), Status: "PENDING", CreatedBy: access.UserID, CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)}
	expiresAt, _ := time.Parse(time.RFC3339Nano, item.ExpiresAt)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO xz_tenant_invitations(id,tenant_id,invitation_code,invited_user_id,invited_email,default_organization_id,default_role,expires_at,status,created_by,created_at,updated_at) VALUES($1,$2,$3,nullif($4,''),nullif($5,''),$6,$7,$8,'PENDING',$9,$10,$10)`, item.ID, item.TenantID, item.InvitationCode, item.InvitedUserID, item.InvitedEmail, item.DefaultOrganizationID, item.DefaultRole, expiresAt, item.CreatedBy, now); err != nil {
		return enterpriseInvitation{}, err
	}
	_ = insertTenantAuditDirect(ctx, s.db, access, "enterprise.member.invite", "invitation", item.ID, request.InvitedUserID, map[string]any{"email": request.InvitedEmail, "role": request.DefaultRole})
	return item, nil
}

func (s *postgresStore) AcceptEnterpriseInvitation(userID string, request enterpriseInvitationAcceptRequest) (enterpriseContext, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return enterpriseContext{}, err
	}
	request.InvitationCode = strings.ToUpper(strings.TrimSpace(request.InvitationCode))
	if request.InvitationCode == "" {
		return enterpriseContext{}, fmt.Errorf("%w: invitationCode is required", errEnterpriseInvalid)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return enterpriseContext{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var invitation enterpriseInvitation
	var invitedUserID, invitedEmail sql.NullString
	var expiresAt time.Time
	if err := tx.QueryRowContext(ctx, `SELECT id,tenant_id,coalesce(default_organization_id,''),default_role,status,created_by,invited_user_id,invited_email,expires_at FROM xz_tenant_invitations WHERE invitation_code=$1 FOR UPDATE`, request.InvitationCode).Scan(&invitation.ID, &invitation.TenantID, &invitation.DefaultOrganizationID, &invitation.DefaultRole, &invitation.Status, &invitation.CreatedBy, &invitedUserID, &invitedEmail, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return enterpriseContext{}, errEnterpriseNotFound
		}
		return enterpriseContext{}, err
	}
	if !strings.EqualFold(invitation.Status, "PENDING") || expiresAt.Before(time.Now().UTC()) {
		return enterpriseContext{}, errEnterpriseConflict
	}
	var userName, userEmail string
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(name,''),coalesce(email,'') FROM xz_users WHERE id=$1 AND upper(status)='ACTIVE'`, userID).Scan(&userName, &userEmail); err != nil {
		return enterpriseContext{}, errUnauthorized
	}
	if invitedUserID.Valid && invitedUserID.String != "" && invitedUserID.String != userID {
		return enterpriseContext{}, errForbidden
	}
	if invitedEmail.Valid && invitedEmail.String != "" && !strings.EqualFold(invitedEmail.String, userEmail) {
		return enterpriseContext{}, errForbidden
	}
	var tenantName, tenantStatus, organizationName string
	if err := tx.QueryRowContext(ctx, `SELECT tenant.name,tenant.status,organization.name FROM xz_tenants tenant JOIN xz_organizations organization ON organization.tenant_id=tenant.id AND organization.id=$2 AND upper(organization.status)='ACTIVE' WHERE tenant.id=$1 AND tenant.tenant_type='ENTERPRISE'`, invitation.TenantID, invitation.DefaultOrganizationID).Scan(&tenantName, &tenantStatus, &organizationName); err != nil || !strings.EqualFold(tenantStatus, "ACTIVE") {
		return enterpriseContext{}, errForbidden
	}
	var activeMember bool
	if err := tx.QueryRowContext(ctx, `SELECT exists(SELECT 1 FROM xz_tenant_members WHERE tenant_id=$1 AND user_id=$2 AND upper(member_status)='ACTIVE')`, invitation.TenantID, userID).Scan(&activeMember); err != nil {
		return enterpriseContext{}, err
	}
	if activeMember {
		return enterpriseContext{}, errEnterpriseConflict
	}
	role := normalizeEnterpriseRole(invitation.DefaultRole)
	if role == "" {
		role = roleEnterpriseMember
	}
	now := time.Now().UTC()
	memberID := newEnterpriseResourceID("tenant_member")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_tenant_members(id,tenant_id,user_id,role,status,primary_organization_id,member_status,certification_status,data_scope,joined_at,invited_by,created_at,updated_at)
		VALUES($1,$2,$3,$4,'ACTIVE',$5,'ACTIVE','UNVERIFIED',$6,$7,$8,$7,$7)
		ON CONFLICT(tenant_id,user_id) DO UPDATE SET role=excluded.role,status='ACTIVE',primary_organization_id=excluded.primary_organization_id,
		 member_status='ACTIVE',data_scope=excluded.data_scope,joined_at=excluded.joined_at,invited_by=excluded.invited_by,disabled_at=NULL,disabled_by=NULL,updated_at=excluded.updated_at
	`, memberID, invitation.TenantID, userID, legacyTenantMemberRole(role), invitation.DefaultOrganizationID, defaultDataScopeForRole(role), now, invitation.CreatedBy); err != nil {
		return enterpriseContext{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_user_roles(user_id,tenant_id,organization_id,role,status,assigned_at,updated_at) VALUES($1,$2,$3,$4,'ACTIVE',$5,$5) ON CONFLICT(user_id,tenant_id,organization_id,role) DO UPDATE SET status='ACTIVE',updated_at=excluded.updated_at`, userID, invitation.TenantID, invitation.DefaultOrganizationID, role, now); err != nil {
		return enterpriseContext{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE xz_tenant_invitations SET status='ACCEPTED',accepted_by=$2,accepted_at=$3,updated_at=$3 WHERE id=$1`, invitation.ID, userID, now); err != nil {
		return enterpriseContext{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_user_role_context(user_id,tenant_id,organization_id,current_role_code,context_type,updated_at) VALUES($1,$2,$3,$4,'ENTERPRISE',$5) ON CONFLICT(user_id) DO UPDATE SET tenant_id=excluded.tenant_id,organization_id=excluded.organization_id,current_role_code=excluded.current_role_code,context_type='ENTERPRISE',updated_at=excluded.updated_at`, userID, invitation.TenantID, invitation.DefaultOrganizationID, role, now); err != nil {
		return enterpriseContext{}, err
	}
	access := enterpriseAccess{UserID: userID, TenantID: invitation.TenantID, OrganizationID: invitation.DefaultOrganizationID, Role: role}
	if err := insertTenantAuditTx(ctx, tx, access, "enterprise.invitation.accept", "member", memberID, userID, map[string]any{"invitationId": invitation.ID}); err != nil {
		return enterpriseContext{}, err
	}
	if err := tx.Commit(); err != nil {
		return enterpriseContext{}, err
	}
	items, err := s.enterpriseContextsContext(ctx, userID)
	if err != nil {
		return enterpriseContext{}, err
	}
	for _, item := range items {
		if item.Type == contextEnterprise && item.TenantID == invitation.TenantID {
			return item, nil
		}
	}
	_ = userName
	return enterpriseContext{}, errEnterpriseNotFound
}

func (s *postgresStore) CreateEnterpriseJoinRequest(userID string, request enterpriseJoinRequestCreate) (enterpriseJoinRequest, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	request.TenantID = strings.TrimSpace(request.TenantID)
	request.RequestedOrganizationID = strings.TrimSpace(request.RequestedOrganizationID)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.TenantID == "" {
		return enterpriseJoinRequest{}, fmt.Errorf("%w: tenantId is required", errEnterpriseInvalid)
	}
	var userName, userEmail string
	if err := s.db.QueryRowContext(ctx, `SELECT coalesce(name,''),coalesce(email,'') FROM xz_users WHERE id=$1 AND upper(status)='ACTIVE'`, userID).Scan(&userName, &userEmail); err != nil {
		return enterpriseJoinRequest{}, errUnauthorized
	}
	if request.RequestedOrganizationID == "" {
		_ = s.db.QueryRowContext(ctx, `SELECT id FROM xz_organizations WHERE tenant_id=$1 AND parent_id IS NULL AND upper(status)='ACTIVE' ORDER BY created_at LIMIT 1`, request.TenantID).Scan(&request.RequestedOrganizationID)
	}
	var valid bool
	if err := s.db.QueryRowContext(ctx, `SELECT exists(SELECT 1 FROM xz_tenants tenant JOIN xz_organizations organization ON organization.tenant_id=tenant.id WHERE tenant.id=$1 AND tenant.tenant_type='ENTERPRISE' AND upper(tenant.status)='ACTIVE' AND organization.id=$2 AND upper(organization.status)='ACTIVE')`, request.TenantID, request.RequestedOrganizationID).Scan(&valid); err != nil || !valid {
		return enterpriseJoinRequest{}, errEnterpriseNotFound
	}
	var duplicate bool
	if err := s.db.QueryRowContext(ctx, `SELECT exists(SELECT 1 FROM xz_tenant_members WHERE tenant_id=$1 AND user_id=$2 AND upper(member_status)='ACTIVE') OR exists(SELECT 1 FROM xz_tenant_join_requests WHERE tenant_id=$1 AND applicant_user_id=$2 AND upper(status)='PENDING')`, request.TenantID, userID).Scan(&duplicate); err != nil {
		return enterpriseJoinRequest{}, err
	}
	if duplicate {
		return enterpriseJoinRequest{}, errEnterpriseConflict
	}
	now := time.Now().UTC()
	item := enterpriseJoinRequest{ID: newEnterpriseResourceID("tenant_join_request"), TenantID: request.TenantID, ApplicantUserID: userID, ApplicantName: userName, ApplicantEmail: userEmail, RequestedOrganizationID: request.RequestedOrganizationID, RequestedRole: roleEnterpriseMember, Reason: request.Reason, Status: "PENDING", CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO xz_tenant_join_requests(id,tenant_id,applicant_user_id,requested_organization_id,requested_role,reason,status,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,'PENDING',$7,$7)`, item.ID, item.TenantID, item.ApplicantUserID, item.RequestedOrganizationID, item.RequestedRole, item.Reason, now); err != nil {
		return enterpriseJoinRequest{}, err
	}
	return item, nil
}

func (s *postgresStore) ListEnterpriseJoinRequests(access enterpriseAccess) ([]enterpriseJoinRequest, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT request.id,request.tenant_id,request.applicant_user_id,coalesce(user_account.name,''),coalesce(user_account.email,''),coalesce(request.requested_organization_id,''),request.requested_role,request.reason,request.status,coalesce(request.reviewed_by,''),request.reviewed_at,request.review_comment,request.created_at,request.updated_at FROM xz_tenant_join_requests request JOIN xz_users user_account ON user_account.id=request.applicant_user_id WHERE request.tenant_id=$1 ORDER BY request.created_at DESC`, access.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]enterpriseJoinRequest, 0)
	for rows.Next() {
		var item enterpriseJoinRequest
		var reviewedAt sql.NullTime
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.TenantID, &item.ApplicantUserID, &item.ApplicantName, &item.ApplicantEmail, &item.RequestedOrganizationID, &item.RequestedRole, &item.Reason, &item.Status, &item.ReviewedBy, &reviewedAt, &item.ReviewComment, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		item.CreatedAt, item.UpdatedAt = createdAt.Format(time.RFC3339Nano), updatedAt.Format(time.RFC3339Nano)
		if reviewedAt.Valid {
			item.ReviewedAt = reviewedAt.Time.Format(time.RFC3339Nano)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) ReviewEnterpriseJoinRequest(access enterpriseAccess, id string, approved bool, comment string) (enterpriseJoinRequest, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return enterpriseJoinRequest{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var item enterpriseJoinRequest
	var createdAt time.Time
	if err := tx.QueryRowContext(ctx, `SELECT id,tenant_id,applicant_user_id,coalesce(requested_organization_id,''),requested_role,reason,status,created_at FROM xz_tenant_join_requests WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, access.TenantID, strings.TrimSpace(id)).Scan(&item.ID, &item.TenantID, &item.ApplicantUserID, &item.RequestedOrganizationID, &item.RequestedRole, &item.Reason, &item.Status, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return item, errEnterpriseNotFound
		}
		return item, err
	}
	if !strings.EqualFold(item.Status, "PENDING") {
		return item, errEnterpriseConflict
	}
	item.CreatedAt = createdAt.Format(time.RFC3339Nano)
	now := time.Now().UTC()
	item.Status = map[bool]string{true: "APPROVED", false: "REJECTED"}[approved]
	item.ReviewedBy, item.ReviewedAt, item.ReviewComment, item.UpdatedAt = access.UserID, now.Format(time.RFC3339Nano), strings.TrimSpace(comment), now.Format(time.RFC3339Nano)
	if approved {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT exists(SELECT 1 FROM xz_tenant_members WHERE tenant_id=$1 AND user_id=$2 AND upper(member_status)='ACTIVE')`, access.TenantID, item.ApplicantUserID).Scan(&exists); err != nil {
			return item, err
		}
		if exists {
			return item, errEnterpriseConflict
		}
		memberID := newEnterpriseResourceID("tenant_member")
		if _, err := tx.ExecContext(ctx, `INSERT INTO xz_tenant_members(id,tenant_id,user_id,role,status,primary_organization_id,member_status,certification_status,data_scope,joined_at,created_at,updated_at) VALUES($1,$2,$3,'MEMBER','ACTIVE',$4,'ACTIVE','UNVERIFIED','SELF',$5,$5,$5) ON CONFLICT(tenant_id,user_id) DO UPDATE SET role='MEMBER',status='ACTIVE',primary_organization_id=excluded.primary_organization_id,member_status='ACTIVE',data_scope='SELF',joined_at=excluded.joined_at,disabled_at=NULL,disabled_by=NULL,updated_at=excluded.updated_at`, memberID, access.TenantID, item.ApplicantUserID, item.RequestedOrganizationID, now); err != nil {
			return item, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO xz_user_roles(user_id,tenant_id,organization_id,role,status,assigned_at,updated_at) VALUES($1,$2,$3,'ENTERPRISE_MEMBER','ACTIVE',$4,$4) ON CONFLICT(user_id,tenant_id,organization_id,role) DO UPDATE SET status='ACTIVE',updated_at=excluded.updated_at`, item.ApplicantUserID, access.TenantID, item.RequestedOrganizationID, now); err != nil {
			return item, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE xz_tenant_join_requests SET status=$3,reviewed_by=$4,reviewed_at=$5,review_comment=$6,updated_at=$5 WHERE tenant_id=$1 AND id=$2`, access.TenantID, item.ID, item.Status, access.UserID, now, item.ReviewComment); err != nil {
		return item, err
	}
	if err := insertTenantAuditTx(ctx, tx, access, "enterprise.join_request."+strings.ToLower(item.Status), "join_request", item.ID, item.ApplicantUserID, map[string]any{"comment": item.ReviewComment}); err != nil {
		return item, err
	}
	if err := tx.Commit(); err != nil {
		return item, err
	}
	return item, nil
}

func (s *postgresStore) UpdateEnterpriseMember(access enterpriseAccess, id string, request enterpriseMemberUpdateRequest) (enterpriseMember, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return enterpriseMember{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var userID, organizationID, dataScope, ownerUserID string
	if err := tx.QueryRowContext(ctx, `SELECT member.user_id,coalesce(member.primary_organization_id,''),member.data_scope,coalesce(tenant.owner_user_id,'') FROM xz_tenant_members member JOIN xz_tenants tenant ON tenant.id=member.tenant_id WHERE member.tenant_id=$1 AND member.id=$2 FOR UPDATE`, access.TenantID, strings.TrimSpace(id)).Scan(&userID, &organizationID, &dataScope, &ownerUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return enterpriseMember{}, errEnterpriseNotFound
		}
		return enterpriseMember{}, err
	}
	if request.PrimaryOrganizationID != "" {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT exists(SELECT 1 FROM xz_organizations WHERE tenant_id=$1 AND id=$2 AND upper(status)='ACTIVE')`, access.TenantID, request.PrimaryOrganizationID).Scan(&exists); err != nil || !exists {
			return enterpriseMember{}, errEnterpriseNotFound
		}
		organizationID = request.PrimaryOrganizationID
	}
	roles := normalizeEnterpriseRoles(request.Roles)
	if len(roles) > 0 && userID == ownerUserID && !containsString(roles, roleEnterpriseAdmin) {
		return enterpriseMember{}, errEnterpriseConflict
	}
	if scope := normalizedDataScope(request.DataScope, ""); scope != "" {
		dataScope = scope
	}
	now := time.Now().UTC()
	legacyRole := "MEMBER"
	if containsString(roles, roleEnterpriseAdmin) {
		legacyRole = "ENTERPRISE_ADMIN"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE xz_tenant_members SET primary_organization_id=$3,data_scope=$4,role=CASE WHEN $5='' THEN role ELSE $5 END,updated_at=$6 WHERE tenant_id=$1 AND id=$2`, access.TenantID, id, organizationID, dataScope, map[bool]string{true: legacyRole, false: ""}[len(roles) > 0], now); err != nil {
		return enterpriseMember{}, err
	}
	if len(roles) > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE xz_user_roles SET status='DISABLED',updated_at=$3 WHERE tenant_id=$1 AND user_id=$2 AND role IN ('ENTERPRISE_ADMIN','AI_ADMIN','FINANCE','CUSTOMER_SERVICE','ENTERPRISE_MEMBER')`, access.TenantID, userID, now); err != nil {
			return enterpriseMember{}, err
		}
		for _, role := range roles {
			if _, err := tx.ExecContext(ctx, `INSERT INTO xz_user_roles(user_id,tenant_id,organization_id,role,status,assigned_at,updated_at) VALUES($1,$2,$3,$4,'ACTIVE',$5,$5) ON CONFLICT(user_id,tenant_id,organization_id,role) DO UPDATE SET status='ACTIVE',updated_at=excluded.updated_at`, userID, access.TenantID, organizationID, role, now); err != nil {
				return enterpriseMember{}, err
			}
		}
	}
	if err := insertTenantAuditTx(ctx, tx, access, "enterprise.member.update", "member", id, userID, map[string]any{"roles": roles, "organizationId": organizationID, "dataScope": dataScope}); err != nil {
		return enterpriseMember{}, err
	}
	if err := tx.Commit(); err != nil {
		return enterpriseMember{}, err
	}
	return s.GetEnterpriseMember(enterpriseAccess{UserID: access.UserID, TenantID: access.TenantID, OrganizationID: access.OrganizationID, DataScope: dataScopeTenantAll}, id)
}

func (s *postgresStore) DisableEnterpriseMember(access enterpriseAccess, id string) (enterpriseMember, error) {
	return s.setPostgresEnterpriseMemberStatus(access, id, "DISABLED")
}

func (s *postgresStore) RemoveEnterpriseMember(access enterpriseAccess, id string) error {
	_, err := s.setPostgresEnterpriseMemberStatus(access, id, "REMOVED")
	return err
}

func (s *postgresStore) setPostgresEnterpriseMemberStatus(access enterpriseAccess, id string, status string) (enterpriseMember, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return enterpriseMember{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var userID, ownerUserID string
	if err := tx.QueryRowContext(ctx, `SELECT member.user_id,coalesce(tenant.owner_user_id,'') FROM xz_tenant_members member JOIN xz_tenants tenant ON tenant.id=member.tenant_id WHERE member.tenant_id=$1 AND member.id=$2 FOR UPDATE`, access.TenantID, strings.TrimSpace(id)).Scan(&userID, &ownerUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return enterpriseMember{}, errEnterpriseNotFound
		}
		return enterpriseMember{}, err
	}
	if userID == access.UserID || userID == ownerUserID {
		return enterpriseMember{}, errEnterpriseConflict
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE xz_tenant_members SET member_status=$3,status=$3,disabled_at=$4,disabled_by=$5,updated_at=$4 WHERE tenant_id=$1 AND id=$2`, access.TenantID, id, status, now, access.UserID); err != nil {
		return enterpriseMember{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE xz_user_roles SET status='DISABLED',updated_at=$3 WHERE tenant_id=$1 AND user_id=$2`, access.TenantID, userID, now); err != nil {
		return enterpriseMember{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE xz_user_role_context SET tenant_id='tenant_default',organization_id=$2,current_role_code='USER',context_type='PERSONAL',updated_at=$3 WHERE user_id=$1 AND tenant_id=$4`, userID, defaultOrganizationID("tenant_default"), now, access.TenantID); err != nil {
		return enterpriseMember{}, err
	}
	action := "enterprise.member.disable"
	if status == "REMOVED" {
		action = "enterprise.member.remove"
	}
	if err := insertTenantAuditTx(ctx, tx, access, action, "member", id, userID, nil); err != nil {
		return enterpriseMember{}, err
	}
	if err := tx.Commit(); err != nil {
		return enterpriseMember{}, err
	}
	if status == "REMOVED" {
		return enterpriseMember{}, nil
	}
	return s.GetEnterpriseMember(enterpriseAccess{UserID: access.UserID, TenantID: access.TenantID, OrganizationID: access.OrganizationID, DataScope: dataScopeTenantAll}, id)
}

func (s *postgresStore) EnterpriseOrganizationTree(access enterpriseAccess) ([]enterpriseOrganization, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	allowed, err := s.postgresAllowedOrganizations(ctx, access)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT organization.id,organization.tenant_id,coalesce(organization.parent_id,''),organization.organization_type,organization.name,organization.status,organization.metadata,organization.created_at,organization.updated_at,count(member.id) FILTER(WHERE upper(member.member_status)='ACTIVE') FROM xz_organizations organization LEFT JOIN xz_tenant_members member ON member.tenant_id=organization.tenant_id AND member.primary_organization_id=organization.id WHERE organization.tenant_id=$1 AND upper(organization.status)<>'DELETED' GROUP BY organization.id ORDER BY organization.created_at,organization.id`, access.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	flat := make([]enterpriseOrganization, 0)
	for rows.Next() {
		var item enterpriseOrganization
		var metadataRaw []byte
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.TenantID, &item.ParentID, &item.OrganizationType, &item.Name, &item.Status, &metadataRaw, &createdAt, &updatedAt, &item.MemberCount); err != nil {
			return nil, err
		}
		if !allowed[item.ID] {
			continue
		}
		item.Metadata = decodeJSONMap(metadataRaw)
		item.CreatedAt, item.UpdatedAt = createdAt.Format(time.RFC3339Nano), updatedAt.Format(time.RFC3339Nano)
		flat = append(flat, item)
	}
	return buildEnterpriseOrganizationTree(flat), rows.Err()
}

func (s *postgresStore) CreateEnterpriseOrganization(access enterpriseAccess, request enterpriseOrganizationCreateRequest) (enterpriseOrganization, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	request.Name = strings.TrimSpace(request.Name)
	request.ParentID = firstNonEmptyString(strings.TrimSpace(request.ParentID), access.OrganizationID)
	request.OrganizationType = strings.ToUpper(strings.TrimSpace(request.OrganizationType))
	if request.Name == "" {
		return enterpriseOrganization{}, fmt.Errorf("%w: organization name is required", errEnterpriseInvalid)
	}
	if request.OrganizationType == "" {
		request.OrganizationType = "DEPARTMENT"
	}
	var parentExists bool
	if err := s.db.QueryRowContext(ctx, `SELECT exists(SELECT 1 FROM xz_organizations WHERE tenant_id=$1 AND id=$2 AND upper(status)='ACTIVE')`, access.TenantID, request.ParentID).Scan(&parentExists); err != nil || !parentExists {
		return enterpriseOrganization{}, errEnterpriseNotFound
	}
	now := time.Now().UTC()
	item := enterpriseOrganization{ID: newEnterpriseResourceID("organization"), TenantID: access.TenantID, ParentID: request.ParentID, OrganizationType: request.OrganizationType, Name: request.Name, Status: "ACTIVE", Metadata: request.Metadata, CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO xz_organizations(id,tenant_id,parent_id,organization_type,name,status,metadata,created_at,updated_at) VALUES($1,$2,$3,$4,$5,'ACTIVE',$6::jsonb,$7,$7)`, item.ID, item.TenantID, item.ParentID, item.OrganizationType, item.Name, jsonProjection(item.Metadata), now); err != nil {
		return enterpriseOrganization{}, err
	}
	_ = insertTenantAuditDirect(ctx, s.db, access, "enterprise.organization.create", "organization", item.ID, "", map[string]any{"parentId": item.ParentID, "name": item.Name})
	return item, nil
}

func (s *postgresStore) UpdateEnterpriseOrganization(access enterpriseAccess, id string, request enterpriseOrganizationUpdateRequest) (enterpriseOrganization, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	var item enterpriseOrganization
	var metadataRaw []byte
	var createdAt, updatedAt time.Time
	if err := s.db.QueryRowContext(ctx, `SELECT id,tenant_id,coalesce(parent_id,''),organization_type,name,status,metadata,created_at,updated_at FROM xz_organizations WHERE tenant_id=$1 AND id=$2 AND upper(status)<>'DELETED'`, access.TenantID, strings.TrimSpace(id)).Scan(&item.ID, &item.TenantID, &item.ParentID, &item.OrganizationType, &item.Name, &item.Status, &metadataRaw, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return item, errEnterpriseNotFound
		}
		return item, err
	}
	if strings.TrimSpace(request.Name) != "" {
		item.Name = strings.TrimSpace(request.Name)
	}
	if value := strings.ToUpper(strings.TrimSpace(request.OrganizationType)); value != "" {
		item.OrganizationType = value
	}
	if value := strings.ToUpper(strings.TrimSpace(request.Status)); value != "" {
		item.Status = value
	}
	item.Metadata = decodeJSONMap(metadataRaw)
	if request.Metadata != nil {
		item.Metadata = request.Metadata
	}
	updatedAt = time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `UPDATE xz_organizations SET organization_type=$3,name=$4,status=$5,metadata=$6::jsonb,updated_at=$7 WHERE tenant_id=$1 AND id=$2`, access.TenantID, item.ID, item.OrganizationType, item.Name, item.Status, jsonProjection(item.Metadata), updatedAt); err != nil {
		return item, err
	}
	item.CreatedAt, item.UpdatedAt = createdAt.Format(time.RFC3339Nano), updatedAt.Format(time.RFC3339Nano)
	_ = insertTenantAuditDirect(ctx, s.db, access, "enterprise.organization.update", "organization", item.ID, "", map[string]any{"name": item.Name, "status": item.Status})
	return item, nil
}

func (s *postgresStore) MoveEnterpriseOrganization(access enterpriseAccess, id string, request enterpriseOrganizationMoveRequest) (enterpriseOrganization, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	id, request.ParentID = strings.TrimSpace(id), strings.TrimSpace(request.ParentID)
	if request.ParentID == "" || request.ParentID == id {
		return enterpriseOrganization{}, fmt.Errorf("%w: a different parentId is required", errEnterpriseInvalid)
	}
	var parentID string
	if err := s.db.QueryRowContext(ctx, `SELECT coalesce(parent_id,'') FROM xz_organizations WHERE tenant_id=$1 AND id=$2 AND upper(status)<>'DELETED'`, access.TenantID, id).Scan(&parentID); err != nil {
		return enterpriseOrganization{}, errEnterpriseNotFound
	}
	if parentID == "" {
		return enterpriseOrganization{}, errEnterpriseConflict
	}
	var cycle bool
	if err := s.db.QueryRowContext(ctx, `WITH RECURSIVE descendants AS (SELECT id FROM xz_organizations WHERE tenant_id=$1 AND id=$2 UNION ALL SELECT child.id FROM xz_organizations child JOIN descendants parent ON child.parent_id=parent.id WHERE child.tenant_id=$1) SELECT exists(SELECT 1 FROM descendants WHERE id=$3)`, access.TenantID, id, request.ParentID).Scan(&cycle); err != nil {
		return enterpriseOrganization{}, err
	}
	if cycle {
		return enterpriseOrganization{}, errEnterpriseConflict
	}
	var parentExists bool
	if err := s.db.QueryRowContext(ctx, `SELECT exists(SELECT 1 FROM xz_organizations WHERE tenant_id=$1 AND id=$2 AND upper(status)='ACTIVE')`, access.TenantID, request.ParentID).Scan(&parentExists); err != nil || !parentExists {
		return enterpriseOrganization{}, errEnterpriseNotFound
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE xz_organizations SET parent_id=$3,updated_at=now() WHERE tenant_id=$1 AND id=$2`, access.TenantID, id, request.ParentID); err != nil {
		return enterpriseOrganization{}, err
	}
	_ = insertTenantAuditDirect(ctx, s.db, access, "enterprise.organization.move", "organization", id, "", map[string]any{"parentId": request.ParentID})
	return s.postgresOrganizationByID(ctx, access.TenantID, id)
}

func (s *postgresStore) DeleteEnterpriseOrganization(access enterpriseAccess, id string) error {
	ctx, cancel := s.withTimeout()
	defer cancel()
	id = strings.TrimSpace(id)
	var parentID string
	if err := s.db.QueryRowContext(ctx, `SELECT coalesce(parent_id,'') FROM xz_organizations WHERE tenant_id=$1 AND id=$2 AND upper(status)<>'DELETED'`, access.TenantID, id).Scan(&parentID); err != nil {
		return errEnterpriseNotFound
	}
	if parentID == "" {
		return errEnterpriseConflict
	}
	var blocked bool
	if err := s.db.QueryRowContext(ctx, `SELECT exists(SELECT 1 FROM xz_organizations WHERE tenant_id=$1 AND parent_id=$2 AND upper(status)<>'DELETED') OR exists(SELECT 1 FROM xz_tenant_members WHERE tenant_id=$1 AND primary_organization_id=$2 AND upper(member_status)='ACTIVE')`, access.TenantID, id).Scan(&blocked); err != nil {
		return err
	}
	if blocked {
		return errEnterpriseConflict
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE xz_organizations SET status='DELETED',updated_at=now() WHERE tenant_id=$1 AND id=$2`, access.TenantID, id); err != nil {
		return err
	}
	return insertTenantAuditDirect(ctx, s.db, access, "enterprise.organization.delete", "organization", id, "", nil)
}

func (s *postgresStore) EnterpriseAuditLogs(access enterpriseAccess, limit int) ([]enterpriseAuditLog, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,tenant_id,coalesce(actor_user_id,''),coalesce(actor_role,''),coalesce(organization_id,''),action,resource_type,coalesce(resource_id,''),coalesce(target_user_id,''),coalesce(request_id,''),status,metadata,before_value,after_value,created_at FROM xz_tenant_audit_logs WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT $2`, access.TenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]enterpriseAuditLog, 0)
	for rows.Next() {
		var item enterpriseAuditLog
		var metadataRaw, beforeRaw, afterRaw []byte
		var createdAt time.Time
		if err := rows.Scan(&item.ID, &item.TenantID, &item.ActorUserID, &item.ActorRole, &item.OrganizationID, &item.Action, &item.ResourceType, &item.ResourceID, &item.TargetUserID, &item.RequestID, &item.Status, &metadataRaw, &beforeRaw, &afterRaw, &createdAt); err != nil {
			return nil, err
		}
		item.Metadata = decodeJSONMap(metadataRaw)
		item.BeforeValue = decodeJSONMap(beforeRaw)
		item.AfterValue = decodeJSONMap(afterRaw)
		item.CreatedAt = createdAt.Format(time.RFC3339Nano)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) SubmitEnterpriseCertification(access enterpriseAccess, request enterpriseCertificationSubmitRequest) (enterpriseCertification, error) {
	request.LegalName = strings.TrimSpace(request.LegalName)
	request.UnifiedSocialCreditCode = strings.ToUpper(strings.TrimSpace(request.UnifiedSocialCreditCode))
	request.LegalRepresentativeName = strings.TrimSpace(request.LegalRepresentativeName)
	if request.LegalName == "" || request.UnifiedSocialCreditCode == "" || len(request.DocumentURLs) == 0 {
		return enterpriseCertification{}, fmt.Errorf("%w: legal name, unified social credit code and document URLs are required", errEnterpriseInvalid)
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return enterpriseCertification{}, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	item := enterpriseCertification{
		ID: newEnterpriseResourceID("tenant_certification"), TenantID: access.TenantID,
		LegalName: request.LegalName, UnifiedSocialCreditCode: request.UnifiedSocialCreditCode,
		LegalRepresentativeName: request.LegalRepresentativeName, DocumentURLs: request.DocumentURLs,
		Status: "PENDING", SubmittedBy: access.UserID, Metadata: request.Metadata,
		CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano),
	}
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO xz_tenant_certifications(
			id,tenant_id,legal_name,unified_social_credit_code,legal_representative_name,document_urls,status,submitted_by,metadata,created_at,updated_at
		) VALUES($1,$2,$3,$4,$5,$6::jsonb,'PENDING',$7,$8::jsonb,$9,$9)
		ON CONFLICT (tenant_id) WHERE upper(status) = 'PENDING' DO UPDATE SET
			legal_name=excluded.legal_name,unified_social_credit_code=excluded.unified_social_credit_code,
			legal_representative_name=excluded.legal_representative_name,document_urls=excluded.document_urls,
			submitted_by=excluded.submitted_by,metadata=excluded.metadata,updated_at=excluded.updated_at
		RETURNING id,created_at,updated_at
	`, item.ID, item.TenantID, item.LegalName, item.UnifiedSocialCreditCode, item.LegalRepresentativeName, jsonProjection(item.DocumentURLs), item.SubmittedBy, jsonProjection(item.Metadata), now).Scan(&item.ID, &createdAt, &now)
	if err != nil {
		return enterpriseCertification{}, err
	}
	item.CreatedAt, item.UpdatedAt = createdAt.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE xz_tenants SET config=jsonb_set(coalesce(config,'{}'::jsonb),'{certificationStatus}','"PENDING"'::jsonb,true),updated_at=$2 WHERE id=$1 AND tenant_type='ENTERPRISE'`, access.TenantID, now); err != nil {
		return enterpriseCertification{}, err
	}
	if err := insertTenantAuditTx(ctx, tx, access, "enterprise.certification.submit", "tenant_certification", item.ID, "", map[string]any{"status": "PENDING"}); err != nil {
		return enterpriseCertification{}, err
	}
	if err := tx.Commit(); err != nil {
		return enterpriseCertification{}, err
	}
	return item, nil
}

type enterpriseSQLQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func queryPostgresEnterpriseMembers(ctx context.Context, queryer enterpriseSQLQueryer, tenantID string) ([]enterpriseMember, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT member.id,member.tenant_id,member.user_id,coalesce(user_account.name,''),coalesce(user_account.email,''),
		       coalesce(member.primary_organization_id,''),coalesce(organization.name,''),member.member_status,
		       member.certification_status,member.data_scope,
		       coalesce(string_agg(DISTINCT role.role, ',' ORDER BY role.role) FILTER(WHERE upper(role.status)='ACTIVE'),''),
		       member.joined_at,coalesce(member.invited_by,''),member.created_at,member.updated_at
		FROM xz_tenant_members member
		JOIN xz_users user_account ON user_account.id=member.user_id
		LEFT JOIN xz_organizations organization ON organization.tenant_id=member.tenant_id AND organization.id=member.primary_organization_id
		LEFT JOIN xz_user_roles role ON role.user_id=member.user_id AND role.tenant_id=member.tenant_id
		 AND role.role IN ('ENTERPRISE_ADMIN','AI_ADMIN','FINANCE','CUSTOMER_SERVICE','ENTERPRISE_MEMBER')
		WHERE member.tenant_id=$1
		GROUP BY member.id,user_account.name,user_account.email,organization.name
		ORDER BY member.joined_at DESC NULLS LAST,member.created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]enterpriseMember, 0)
	for rows.Next() {
		var item enterpriseMember
		var roleCSV string
		var joinedAt sql.NullTime
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.TenantID, &item.UserID, &item.Name, &item.Email, &item.PrimaryOrganizationID, &item.OrganizationName, &item.MemberStatus, &item.CertificationStatus, &item.DataScope, &roleCSV, &joinedAt, &item.InvitedBy, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		item.Roles = normalizeEnterpriseRoles(splitCSV(roleCSV))
		if joinedAt.Valid {
			item.JoinedAt = joinedAt.Time.Format(time.RFC3339Nano)
		}
		item.CreatedAt, item.UpdatedAt = createdAt.Format(time.RFC3339Nano), updatedAt.Format(time.RFC3339Nano)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) postgresAllowedOrganizations(ctx context.Context, access enterpriseAccess) (map[string]bool, error) {
	allowed := map[string]bool{}
	query := `SELECT id FROM xz_organizations WHERE tenant_id=$1 AND upper(status)<>'DELETED'`
	args := []any{access.TenantID}
	switch access.DataScope {
	case dataScopeTenantAll:
	case dataScopeOrgAndChildren:
		query = `WITH RECURSIVE scoped AS (SELECT id FROM xz_organizations WHERE tenant_id=$1 AND id=$2 AND upper(status)<>'DELETED' UNION ALL SELECT child.id FROM xz_organizations child JOIN scoped parent ON child.parent_id=parent.id WHERE child.tenant_id=$1 AND upper(child.status)<>'DELETED') SELECT id FROM scoped`
		args = append(args, access.OrganizationID)
	default:
		allowed[access.OrganizationID] = true
		return allowed, nil
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		allowed[id] = true
	}
	return allowed, rows.Err()
}

func (s *postgresStore) postgresOrganizationAllowed(ctx context.Context, tenantID string, baseOrganizationID string, targetOrganizationID string, dataScope string) (bool, error) {
	access := enterpriseAccess{TenantID: tenantID, OrganizationID: baseOrganizationID, DataScope: dataScope}
	allowed, err := s.postgresAllowedOrganizations(ctx, access)
	return allowed[targetOrganizationID], err
}

func (s *postgresStore) postgresOrganizationByID(ctx context.Context, tenantID string, id string) (enterpriseOrganization, error) {
	var item enterpriseOrganization
	var metadataRaw []byte
	var createdAt, updatedAt time.Time
	if err := s.db.QueryRowContext(ctx, `SELECT id,tenant_id,coalesce(parent_id,''),organization_type,name,status,metadata,created_at,updated_at FROM xz_organizations WHERE tenant_id=$1 AND id=$2 AND upper(status)<>'DELETED'`, tenantID, id).Scan(&item.ID, &item.TenantID, &item.ParentID, &item.OrganizationType, &item.Name, &item.Status, &metadataRaw, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return item, errEnterpriseNotFound
		}
		return item, err
	}
	item.Metadata = decodeJSONMap(metadataRaw)
	item.CreatedAt, item.UpdatedAt = createdAt.Format(time.RFC3339Nano), updatedAt.Format(time.RFC3339Nano)
	return item, nil
}

func insertTenantAuditDirect(ctx context.Context, db *sql.DB, access enterpriseAccess, action string, resourceType string, resourceID string, targetUserID string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	beforeValue, afterValue, requestID := tenantAuditSnapshots(metadata)
	_, err := db.ExecContext(ctx, `INSERT INTO xz_tenant_audit_logs(id,tenant_id,actor_user_id,actor_role,organization_id,action,resource_type,resource_id,target_user_id,request_id,status,metadata,before_value,after_value,idempotency_key) VALUES($1,$2,nullif($3,''),$4,nullif($5,''),$6,$7,nullif($8,''),nullif($9,''),nullif($10,''),'SUCCEEDED',$11::jsonb,$12::jsonb,$13::jsonb,nullif($14,''))`, newEnterpriseResourceID("tenant_audit"), access.TenantID, access.UserID, access.Role, access.OrganizationID, action, resourceType, resourceID, targetUserID, requestID, jsonProjection(metadata), jsonProjection(beforeValue), jsonProjection(afterValue), firstNonEmptyString(requestID, action+":"+resourceID))
	return err
}

func insertTenantAuditTx(ctx context.Context, tx *sql.Tx, access enterpriseAccess, action string, resourceType string, resourceID string, targetUserID string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	beforeValue, afterValue, requestID := tenantAuditSnapshots(metadata)
	_, err := tx.ExecContext(ctx, `INSERT INTO xz_tenant_audit_logs(id,tenant_id,actor_user_id,actor_role,organization_id,action,resource_type,resource_id,target_user_id,request_id,status,metadata,before_value,after_value,idempotency_key) VALUES($1,$2,nullif($3,''),$4,nullif($5,''),$6,$7,nullif($8,''),nullif($9,''),nullif($10,''),'SUCCEEDED',$11::jsonb,$12::jsonb,$13::jsonb,nullif($14,''))`, newEnterpriseResourceID("tenant_audit"), access.TenantID, access.UserID, access.Role, access.OrganizationID, action, resourceType, resourceID, targetUserID, requestID, jsonProjection(metadata), jsonProjection(beforeValue), jsonProjection(afterValue), firstNonEmptyString(requestID, action+":"+resourceID))
	return err
}

func tenantAuditSnapshots(metadata map[string]any) (map[string]any, map[string]any, string) {
	beforeValue, _ := mapValue(metadata["before"])
	afterValue, _ := mapValue(metadata["after"])
	if beforeValue == nil {
		beforeValue = map[string]any{}
	}
	if afterValue == nil {
		afterValue = map[string]any{}
	}
	return beforeValue, afterValue, strings.TrimSpace(stringValue(metadata["requestId"]))
}

func defaultOrganizationID(tenantID string) string {
	if tenantID == "tenant_default" {
		digest := md5.Sum([]byte(tenantID))
		return "organization_default_" + fmt.Sprintf("%x", digest)[:16]
	}
	return "organization_default_" + shortID(tenantID)
}

func decodeJSONMap(raw []byte) map[string]any {
	result := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &result)
	}
	return result
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts
}

func legacyTenantMemberRole(role string) string {
	if role == roleEnterpriseAdmin {
		return "ENTERPRISE_ADMIN"
	}
	return "MEMBER"
}

var _ enterpriseStore = (*postgresStore)(nil)
