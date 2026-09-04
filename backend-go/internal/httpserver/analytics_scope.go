package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ScopeLevel indicates the breadth of data visibility.
type ScopeLevel string

const (
	ScopePlatform        ScopeLevel = "PLATFORM"
	ScopeOperationCenter ScopeLevel = "OPERATION_CENTER"
	ScopeAgent           ScopeLevel = "AGENT"
	ScopeTenant          ScopeLevel = "TENANT"
	ScopeFailClosed      ScopeLevel = "FAIL_CLOSED"
)

// AnalyticsScope encapsulates server-side resolved data visibility boundaries.
type AnalyticsScope struct {
	Level              ScopeLevel `json:"level"`
	IsPlatform         bool       `json:"isPlatform"`
	TenantIDs          []string   `json:"tenantIds,omitempty"`
	AgentIDs           []string   `json:"agentIds,omitempty"`
	OperationCenterIDs []string   `json:"operationCenterIds,omitempty"`
	UserID             string     `json:"userId,omitempty"`
}

// FailClosedScope returns an empty, restrictive scope that matches zero rows.
func FailClosedScope(userID string) AnalyticsScope {
	return AnalyticsScope{
		Level:      ScopeFailClosed,
		IsPlatform: false,
		UserID:     userID,
	}
}

// PlatformScope returns an unrestricted platform-wide scope.
func PlatformScope(userID string) AnalyticsScope {
	return AnalyticsScope{
		Level:      ScopePlatform,
		IsPlatform: true,
		UserID:     userID,
	}
}

// IsFailClosed returns true if the scope must not match any data.
func (s AnalyticsScope) IsFailClosed() bool {
	return s.Level == ScopeFailClosed || (!s.IsPlatform && len(s.TenantIDs) == 0 && len(s.AgentIDs) == 0 && len(s.OperationCenterIDs) == 0)
}

// ScopeSQLFilter generates SQL WHERE conditions and appends bound arguments.
// tableName must be one of: "xz_generation_tasks", "xz_billing_events", "model_call_logs".
func (s AnalyticsScope) ScopeSQLFilter(tableName string, currentArgIndex int) (clause string, args []any, nextIndex int) {
	nextIndex = currentArgIndex
	if s.IsPlatform {
		return "1=1", nil, nextIndex
	}
	if s.IsFailClosed() {
		return "1=0", nil, nextIndex
	}

	var conditions []string

	switch tableName {
	case "xz_generation_tasks":
		if len(s.TenantIDs) > 0 {
			conditions = append(conditions, fmt.Sprintf("tenant_id = ANY($%d)", nextIndex))
			args = append(args, s.TenantIDs)
			nextIndex++
		}
		if len(s.AgentIDs) > 0 {
			conditions = append(conditions, fmt.Sprintf("agent_id = ANY($%d)", nextIndex))
			args = append(args, s.AgentIDs)
			nextIndex++
		}
		if len(s.OperationCenterIDs) > 0 {
			conditions = append(conditions, fmt.Sprintf("operation_center_id = ANY($%d)", nextIndex))
			args = append(args, s.OperationCenterIDs)
			nextIndex++
		}
	case "xz_billing_events":
		if len(s.TenantIDs) > 0 {
			conditions = append(conditions, fmt.Sprintf("tenant_id = ANY($%d)", nextIndex))
			args = append(args, s.TenantIDs)
			nextIndex++
		}
		if len(s.AgentIDs) > 0 {
			conditions = append(conditions, fmt.Sprintf("agent_id = ANY($%d)", nextIndex))
			args = append(args, s.AgentIDs)
			nextIndex++
		}
		if len(s.OperationCenterIDs) > 0 {
			conditions = append(conditions, fmt.Sprintf("operation_center_id = ANY($%d)", nextIndex))
			args = append(args, s.OperationCenterIDs)
			nextIndex++
		}
	case "model_call_logs":
		// model_call_logs only has task_id referencing generation_tasks.
		// Scope-restricting model_call_logs links via task_id to xz_generation_tasks.
		var subConditions []string
		if len(s.TenantIDs) > 0 {
			subConditions = append(subConditions, fmt.Sprintf("t.tenant_id = ANY($%d)", nextIndex))
			args = append(args, s.TenantIDs)
			nextIndex++
		}
		if len(s.AgentIDs) > 0 {
			subConditions = append(subConditions, fmt.Sprintf("t.agent_id = ANY($%d)", nextIndex))
			args = append(args, s.AgentIDs)
			nextIndex++
		}
		if len(s.OperationCenterIDs) > 0 {
			subConditions = append(subConditions, fmt.Sprintf("t.operation_center_id = ANY($%d)", nextIndex))
			args = append(args, s.OperationCenterIDs)
			nextIndex++
		}
		if len(subConditions) > 0 {
			conditions = append(conditions, fmt.Sprintf("EXISTS (SELECT 1 FROM xz_generation_tasks t WHERE t.id = model_call_logs.task_id::text AND (%s))", strings.Join(subConditions, " OR ")))
		}
	default:
		return "1=0", nil, nextIndex
	}

	if len(conditions) == 0 {
		return "1=0", nil, nextIndex
	}
	return "(" + strings.Join(conditions, " OR ") + ")", args, nextIndex
}

// resolveAnalyticsScope resolves the user's data visibility boundary strictly server-side.
// It never falls back to PLATFORM on errors or unknown roles.
func resolveAnalyticsScope(ctx context.Context, store platformStore, r *http.Request, sessions authSessionStore) (AnalyticsScope, error) {
	// 1. First attempt to read actor context set by rbacMiddleware
	actorID, _ := r.Context().Value(actorIDContextKey).(string)
	actorRole, _ := r.Context().Value(actorRoleContextKey).(string)

	// 2. If actor context not injected, extract authenticated user from sessions/token
	if actorID == "" || actorRole == "" {
		userID, err := authenticatedUserID(r, sessions)
		if err != nil || userID == "" {
			return FailClosedScope(""), errUnauthorized
		}
		var user adminUser
		if getter, ok := store.(interface {
			GetActiveUser(string) (adminUser, bool, error)
		}); ok {
			u, found, err := getter.GetActiveUser(userID)
			if err != nil {
				return FailClosedScope(userID), fmt.Errorf("failed to lookup user: %w", err)
			}
			if !found {
				return FailClosedScope(userID), errUnauthorized
			}
			user = u
		} else {
			return FailClosedScope(userID), errors.New("store does not support user identity lookup")
		}
		actorID = user.ID
		actorRole = user.Role
	}

	// 3. Platform Admin Check: only recognized platform admin roles get PLATFORM scope
	if isPlatformAdminRole(actorRole) {
		return PlatformScope(actorID), nil
	}

	// 4. Non-platform users: resolve entities via PostgreSQL (fail-closed if not a postgresStore)
	pgStore, ok := store.(*postgresStore)
	if !ok || pgStore.db == nil {
		// Non-postgres store cannot safely query entity graphs for non-admin roles
		return FailClosedScope(actorID), nil
	}

	return resolveEntityScopesFromDB(ctx, pgStore.db, actorID, actorRole)
}

func resolveEntityScopesFromDB(ctx context.Context, db *sql.DB, userID string, role string) (AnalyticsScope, error) {
	scope := AnalyticsScope{
		UserID:     userID,
		IsPlatform: false,
	}

	// 1. Look up Tenant Memberships (Active only)
	tenantRows, err := db.QueryContext(ctx, `
		SELECT tenant_id FROM xz_tenant_members 
		WHERE user_id = $1 AND upper(coalesce(nullif(member_status,''),status,'ACTIVE')) = 'ACTIVE'
	`, userID)
	if err == nil {
		defer tenantRows.Close()
		for tenantRows.Next() {
			var tID string
			if err := tenantRows.Scan(&tID); err == nil && tID != "" {
				scope.TenantIDs = append(scope.TenantIDs, tID)
			}
		}
	}

	// 2. Look up Channel Agent Profiles (Active only)
	agentRows, err := db.QueryContext(ctx, `
		SELECT id FROM xz_channel_agents 
		WHERE user_id = $1 AND upper(coalesce(status,'ACTIVE')) = 'ACTIVE'
	`, userID)
	if err == nil {
		defer agentRows.Close()
		for agentRows.Next() {
			var aID string
			if err := agentRows.Scan(&aID); err == nil && aID != "" {
				scope.AgentIDs = append(scope.AgentIDs, aID)
			}
		}
	}

	// 3. Look up Operation Center Profiles (Active only)
	ocRows, err := db.QueryContext(ctx, `
		SELECT id FROM xz_operation_centers 
		WHERE user_id = $1 AND upper(coalesce(status,'ACTIVE')) = 'ACTIVE'
	`, userID)
	if err == nil {
		defer ocRows.Close()
		for ocRows.Next() {
			var ocID string
			if err := ocRows.Scan(&ocID); err == nil && ocID != "" {
				scope.OperationCenterIDs = append(scope.OperationCenterIDs, ocID)
			}
		}
	}

	// Categorize Level
	if len(scope.OperationCenterIDs) > 0 {
		scope.Level = ScopeOperationCenter
	} else if len(scope.AgentIDs) > 0 {
		scope.Level = ScopeAgent
	} else if len(scope.TenantIDs) > 0 {
		scope.Level = ScopeTenant
	} else {
		// No entity memberships found: FAIL CLOSED
		scope.Level = ScopeFailClosed
	}

	if scope.IsFailClosed() {
		return FailClosedScope(userID), errors.New("user has no active tenant, agent, or operation center membership")
	}

	return scope, nil
}
