package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func ensureUserRBACSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS xz_tenants (
			id TEXT PRIMARY KEY,
			tenant_type TEXT NOT NULL CHECK (tenant_type IN ('PLATFORM', 'ENTERPRISE', 'PERSONAL')),
			enterprise_ref TEXT,
			owner_user_id TEXT REFERENCES xz_users(id),
			name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'ACTIVE',
			config JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (id, tenant_type)
		);
		CREATE TABLE IF NOT EXISTS xz_organizations (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
			parent_id TEXT,
			organization_type TEXT NOT NULL DEFAULT 'DEPARTMENT',
			name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'ACTIVE',
			metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (tenant_id, id),
			FOREIGN KEY (tenant_id, parent_id) REFERENCES xz_organizations(tenant_id, id)
		);
		CREATE TABLE IF NOT EXISTS xz_user_roles (
			user_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
			organization_id TEXT NOT NULL REFERENCES xz_organizations(id),
			role TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'ACTIVE',
			assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (user_id, tenant_id, organization_id, role)
		);
		CREATE TABLE IF NOT EXISTS xz_user_role_context (
			user_id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
			organization_id TEXT NOT NULL REFERENCES xz_organizations(id),
			current_role_code TEXT NOT NULL DEFAULT 'USER',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE IF NOT EXISTS xz_role_permissions (
			role TEXT NOT NULL,
			permission TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (role, permission)
		);
		CREATE INDEX IF NOT EXISTS idx_xz_user_roles_user ON xz_user_roles(user_id, status);
		CREATE INDEX IF NOT EXISTS idx_xz_user_roles_scope ON xz_user_roles(tenant_id, organization_id, role, status);
		CREATE INDEX IF NOT EXISTS idx_xz_organizations_rbac_scope ON xz_organizations(tenant_id, status);
	`)
	return err
}

func syncUserRBACProjection(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_tenants (id, tenant_type, name)
		VALUES ('tenant_default', 'PLATFORM', '知启云默认租户')
		ON CONFLICT (id) DO NOTHING;
		INSERT INTO xz_organizations (id, tenant_id, organization_type, name)
		SELECT 'organization_default_' || substr(md5(id), 1, 16), id, 'DEPARTMENT', '默认组织'
		FROM xz_tenants
		ON CONFLICT (id) DO NOTHING;
	`); err != nil {
		return err
	}
	if err := seedRuntimeRolePermissions(ctx, db); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `
		WITH user_scope AS (
			SELECT u.id AS user_id, 'tenant_default'::text AS tenant_id
			FROM xz_users u
		)
		INSERT INTO xz_user_roles (user_id, tenant_id, organization_id, role)
		SELECT user_id, tenant_id, 'organization_default_' || substr(md5(tenant_id), 1, 16), 'USER'
		FROM user_scope
		ON CONFLICT (user_id, tenant_id, organization_id, role)
		DO UPDATE SET status = 'ACTIVE', updated_at = now();

		INSERT INTO xz_user_roles (user_id, tenant_id, organization_id, role)
		SELECT agent.user_id, scope.tenant_id, scope.organization_id, 'AGENT'
		FROM xz_channel_agents agent
		JOIN xz_user_roles scope ON scope.user_id = agent.user_id AND scope.role = 'USER' AND upper(scope.status) = 'ACTIVE'
		WHERE upper(coalesce(agent.status, 'ACTIVE')) = 'ACTIVE'
		ON CONFLICT (user_id, tenant_id, organization_id, role)
		DO UPDATE SET status = 'ACTIVE', updated_at = now();

		INSERT INTO xz_user_roles (user_id, tenant_id, organization_id, role)
		SELECT center.user_id, scope.tenant_id, scope.organization_id, 'OPERATION'
		FROM xz_operation_centers center
		JOIN xz_user_roles scope ON scope.user_id = center.user_id AND scope.role = 'USER' AND upper(scope.status) = 'ACTIVE'
		WHERE upper(coalesce(center.status, '')) = 'ACTIVE'
		ON CONFLICT (user_id, tenant_id, organization_id, role)
		DO UPDATE SET status = 'ACTIVE', updated_at = now();

		INSERT INTO xz_user_roles (user_id, tenant_id, organization_id, role)
		SELECT users.id, scope.tenant_id, scope.organization_id, role_code
		FROM xz_users users
		JOIN xz_user_roles scope ON scope.user_id = users.id AND scope.role = 'USER' AND upper(scope.status) = 'ACTIVE'
		CROSS JOIN LATERAL (
			VALUES ('ENTERPRISE_ADMIN'), ('AI_ADMIN'), ('FINANCE'), ('CUSTOMER_SERVICE')
		) AS extra(role_code)
		WHERE upper(coalesce(users.raw->>'role', '')) = 'SUPER_ADMIN'
		ON CONFLICT (user_id, tenant_id, organization_id, role)
		DO UPDATE SET status = 'ACTIVE', updated_at = now();

		INSERT INTO xz_user_role_context (user_id, tenant_id, organization_id, current_role_code, context_type)
		SELECT user_id, tenant_id, organization_id, 'USER', 'PERSONAL'
		FROM xz_user_roles
		WHERE role = 'USER' AND upper(status) = 'ACTIVE'
		ON CONFLICT (user_id) DO NOTHING;
	`)
	return err
}

func seedRuntimeRolePermissions(ctx context.Context, db *sql.DB) error {
	for role, permissions := range rolePermissionMatrix {
		for _, permission := range permissions {
			if _, err := db.ExecContext(ctx, `
				INSERT INTO xz_role_permissions (role, permission)
				VALUES ($1, $2)
				ON CONFLICT (role, permission) DO NOTHING
			`, role, permission); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *postgresStore) GetUserRoleAccess(userID string) (userRoleAccess, bool, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return userRoleAccess{}, false, err
	}
	if err := syncUserRBACForUser(ctx, s.db, userID); err != nil {
		return userRoleAccess{}, false, err
	}
	access := userRoleAccess{UserID: userID}
	var contextType string
	err := s.db.QueryRowContext(ctx, `
		SELECT tenant_id, organization_id, current_role_code, context_type
		FROM xz_user_role_context
		WHERE user_id = $1
	`, userID).Scan(&access.TenantID, &access.OrganizationID, &access.CurrentRole, &contextType)
	if errors.Is(err, sql.ErrNoRows) {
		return userRoleAccess{}, false, nil
	}
	if err != nil {
		return userRoleAccess{}, false, err
	}
	if contextType == contextEnterprise {
		var valid bool
		if err := s.db.QueryRowContext(ctx, `
			SELECT exists(
				SELECT 1 FROM xz_tenant_members member
				JOIN xz_tenants tenant ON tenant.id=member.tenant_id
				JOIN xz_organizations organization ON organization.tenant_id=member.tenant_id AND organization.id=$3
				WHERE member.user_id=$1 AND member.tenant_id=$2 AND upper(member.member_status)='ACTIVE'
				  AND tenant.tenant_type='ENTERPRISE' AND upper(tenant.status)='ACTIVE' AND upper(organization.status)='ACTIVE'
			)
		`, userID, access.TenantID, access.OrganizationID).Scan(&valid); err != nil {
			return userRoleAccess{}, false, err
		}
		if !valid {
			access.TenantID = "tenant_default"
			access.OrganizationID = defaultOrganizationID("tenant_default")
			access.CurrentRole = roleUser
			contextType = contextPersonal
			if _, err := s.db.ExecContext(ctx, `UPDATE xz_user_role_context SET tenant_id=$2,organization_id=$3,current_role_code='USER',context_type='PERSONAL',updated_at=now() WHERE user_id=$1`, userID, access.TenantID, access.OrganizationID); err != nil {
				return userRoleAccess{}, false, err
			}
		}
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT role
		FROM (
		  SELECT DISTINCT role_binding.role
		  FROM xz_user_roles role_binding
		  JOIN xz_tenants tenant ON tenant.id = role_binding.tenant_id
		  LEFT JOIN xz_tenant_members member
		    ON member.tenant_id = role_binding.tenant_id AND member.user_id = role_binding.user_id
		  WHERE role_binding.user_id = $1
		    AND upper(role_binding.status) = 'ACTIVE'
		    AND (
		      role_binding.tenant_id = 'tenant_default'
		      OR (
		        tenant.tenant_type = 'ENTERPRISE'
		        AND upper(tenant.status) = 'ACTIVE'
		        AND upper(member.member_status) = 'ACTIVE'
		      )
		    )
		) available_roles
		ORDER BY CASE role WHEN 'USER' THEN 0 WHEN 'AGENT' THEN 10 WHEN 'OPERATION' THEN 20 ELSE 100 END, role
	`, userID)
	if err != nil {
		return userRoleAccess{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return userRoleAccess{}, false, err
		}
		access.Roles = append(access.Roles, role)
	}
	if err := rows.Err(); err != nil {
		return userRoleAccess{}, false, err
	}
	if !containsString(access.Roles, normalizeAppRole(access.CurrentRole)) {
		access.CurrentRole = roleUser
		if _, err := s.db.ExecContext(ctx, `UPDATE xz_user_role_context SET current_role_code = 'USER', updated_at = now() WHERE user_id = $1`, userID); err != nil {
			return userRoleAccess{}, false, err
		}
	}
	access.Permissions, err = runtimePermissionsForRole(ctx, s.db, access.CurrentRole)
	if err != nil {
		return userRoleAccess{}, false, err
	}
	return access, true, nil
}

func (s *postgresStore) SetUserCurrentRole(userID string, role string) (userRoleAccess, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return userRoleAccess{}, err
	}
	role = normalizeAppRole(role)
	if role == "" {
		return userRoleAccess{}, errForbidden
	}
	var tenantID, organizationID string
	err := s.db.QueryRowContext(ctx, `
		SELECT role_binding.tenant_id, role_binding.organization_id
		FROM xz_user_roles role_binding
		JOIN xz_tenants tenant ON tenant.id=role_binding.tenant_id
		LEFT JOIN xz_tenant_members member ON member.tenant_id=role_binding.tenant_id AND member.user_id=role_binding.user_id
		WHERE role_binding.user_id = $1 AND role_binding.role = $2 AND upper(role_binding.status) = 'ACTIVE'
		  AND (
		    ($2 IN ('USER','AGENT','OPERATION') AND role_binding.tenant_id='tenant_default')
		    OR
		    ($2 IN ('ENTERPRISE_ADMIN','AI_ADMIN','FINANCE','CUSTOMER_SERVICE','ENTERPRISE_MEMBER')
		      AND tenant.tenant_type='ENTERPRISE' AND upper(tenant.status)='ACTIVE' AND upper(member.member_status)='ACTIVE')
		  )
		ORDER BY role_binding.assigned_at DESC
		LIMIT 1
	`, userID, role).Scan(&tenantID, &organizationID)
	if errors.Is(err, sql.ErrNoRows) {
		return userRoleAccess{}, errForbidden
	}
	if err != nil {
		return userRoleAccess{}, err
	}
	contextType := contextEnterprise
	if role == roleUser {
		contextType = contextPersonal
	} else if role == roleAgent {
		contextType = contextAgent
	} else if role == roleOperation {
		contextType = contextOperation
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO xz_user_role_context (user_id, tenant_id, organization_id, current_role_code, context_type, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (user_id) DO UPDATE
		SET tenant_id = EXCLUDED.tenant_id,
		    organization_id = EXCLUDED.organization_id,
		    current_role_code = EXCLUDED.current_role_code,
		    context_type = EXCLUDED.context_type,
		    updated_at = now()
	`, userID, tenantID, organizationID, role, contextType)
	if err != nil {
		return userRoleAccess{}, err
	}
	access, found, err := s.GetUserRoleAccess(userID)
	if err != nil {
		return userRoleAccess{}, err
	}
	if !found {
		return userRoleAccess{}, fmt.Errorf("user role access not found")
	}
	return access, nil
}

func syncUserRBACForUser(ctx context.Context, db *sql.DB, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return errUnauthorized
	}
	queries := []string{
		`
		INSERT INTO xz_organizations (id, tenant_id, organization_type, name)
		SELECT 'organization_default_' || substr(md5(t.id), 1, 16), t.id, 'DEPARTMENT', '默认组织'
		FROM xz_tenants t
		WHERE t.id = 'tenant_default' AND $1::text <> ''
		ON CONFLICT (id) DO NOTHING
		`, `
		WITH user_scope AS (
			SELECT u.id AS user_id, 'tenant_default'::text AS tenant_id
			FROM xz_users u WHERE u.id = $1
		)
		INSERT INTO xz_user_roles (user_id, tenant_id, organization_id, role)
		SELECT user_id, tenant_id, 'organization_default_' || substr(md5(tenant_id), 1, 16), 'USER'
		FROM user_scope
		ON CONFLICT (user_id, tenant_id, organization_id, role)
		DO UPDATE SET status = 'ACTIVE', updated_at = now()
		`, `
		INSERT INTO xz_user_roles (user_id, tenant_id, organization_id, role)
		SELECT agent.user_id, scope.tenant_id, scope.organization_id, 'AGENT'
		FROM xz_channel_agents agent
		JOIN xz_user_roles scope ON scope.user_id = agent.user_id AND scope.role = 'USER' AND upper(scope.status) = 'ACTIVE'
		WHERE agent.user_id = $1 AND upper(coalesce(agent.status, 'ACTIVE')) = 'ACTIVE'
		ON CONFLICT (user_id, tenant_id, organization_id, role)
		DO UPDATE SET status = 'ACTIVE', updated_at = now()
		`, `
		INSERT INTO xz_user_roles (user_id, tenant_id, organization_id, role)
		SELECT center.user_id, scope.tenant_id, scope.organization_id, 'OPERATION'
		FROM xz_operation_centers center
		JOIN xz_user_roles scope ON scope.user_id = center.user_id AND scope.role = 'USER' AND upper(scope.status) = 'ACTIVE'
		WHERE center.user_id = $1 AND upper(coalesce(center.status, '')) = 'ACTIVE'
		ON CONFLICT (user_id, tenant_id, organization_id, role)
		DO UPDATE SET status = 'ACTIVE', updated_at = now()
		`, `
		INSERT INTO xz_user_role_context (user_id, tenant_id, organization_id, current_role_code, context_type)
		SELECT user_id, tenant_id, organization_id, 'USER', 'PERSONAL'
		FROM xz_user_roles
		WHERE user_id = $1 AND role = 'USER' AND upper(status) = 'ACTIVE'
		ON CONFLICT (user_id) DO NOTHING
		`,
	}
	for _, query := range queries {
		if _, err := db.ExecContext(ctx, query, userID); err != nil {
			return err
		}
	}
	return nil
}

func runtimePermissionsForRole(ctx context.Context, db *sql.DB, currentRole string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT permission
		FROM xz_role_permissions
		WHERE role = 'USER' OR role = $1
		ORDER BY permission
	`, currentRole)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	permissions := make([]string, 0)
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}
	return permissions, rows.Err()
}
