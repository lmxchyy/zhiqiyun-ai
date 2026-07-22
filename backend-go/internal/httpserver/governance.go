package httpserver

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type actorContextKey string

const (
	actorIDContextKey   actorContextKey = "admin-actor-id"
	actorRoleContextKey actorContextKey = "admin-actor-role"
)

func (s *postgresStore) auditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		method := c.Request.Method
		if method != http.MethodPost && method != http.MethodPatch && method != http.MethodPut && method != http.MethodDelete {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		actorID := c.GetString("actorID")
		actorRole := c.GetString("actorRole")
		_ = insertAuditDirect(ctx, s.db, actorID, actorRole, method+" "+c.FullPath(), "http_request", "", method, c.Request.URL.Path, c.Writer.Status(), map[string]any{"clientIP": c.ClientIP()})
	}
}

func (s *postgresStore) rbacMiddleware(auth authAPI, permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := authenticatedUserID(c.Request, auth.sessions)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
			return
		}
		user, found, err := s.GetActiveUser(userID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]string{"error": errUnauthorized.Error()})
			return
		}
		if !rbacEnforced() && !strings.HasPrefix(permission, "enterprise:") {
			if user.Role != "SUPER_ADMIN" {
				c.AbortWithStatusJSON(http.StatusForbidden, map[string]string{"error": errForbidden.Error()})
				return
			}
			c.Set("actorID", user.ID)
			c.Set("actorRole", user.Role)
			c.Request = c.Request.WithContext(context.WithValue(context.WithValue(c.Request.Context(), actorIDContextKey, user.ID), actorRoleContextKey, user.Role))
			c.Next()
			return
		}
		ok, err := s.roleHasPermission(c.Request.Context(), user.Role, permission)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, map[string]string{"error": errForbidden.Error()})
			return
		}
		c.Set("actorID", user.ID)
		c.Set("actorRole", user.Role)
		c.Request = c.Request.WithContext(context.WithValue(context.WithValue(c.Request.Context(), actorIDContextKey, user.ID), actorRoleContextKey, user.Role))
		c.Next()
	}
}

func superAdminMiddleware(auth authAPI) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := auth.store.AdminData()
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		user, err := auth.authenticatedUser(c.Request, data)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
			return
		}
		if user.Role != "SUPER_ADMIN" {
			c.AbortWithStatusJSON(http.StatusForbidden, map[string]string{"error": errForbidden.Error()})
			return
		}
		c.Set("actorID", user.ID)
		c.Set("actorRole", user.Role)
		c.Request = c.Request.WithContext(context.WithValue(context.WithValue(c.Request.Context(), actorIDContextKey, user.ID), actorRoleContextKey, user.Role))
		c.Next()
	}
}

func actorFromRequest(r *http.Request) (string, string) {
	actorID, _ := r.Context().Value(actorIDContextKey).(string)
	actorRole, _ := r.Context().Value(actorRoleContextKey).(string)
	return strings.TrimSpace(actorID), strings.TrimSpace(actorRole)
}

func adminPermissionForRequest(r *http.Request) string {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/admin")
	if strings.HasPrefix(path, "/users/") && strings.Contains(path, "/identity-change/") {
		switch {
		case strings.HasSuffix(path, "/preview"):
			return "identity:change:preview"
		case strings.HasSuffix(path, "/review"):
			return "identity:change:review"
		default:
			return "identity:change:confirm"
		}
	}
	if strings.HasPrefix(path, "/users/") && strings.Contains(path, "/identity-downgrade/") {
		if r.Method == http.MethodGet {
			return "identity:downgrade:view"
		}
		if strings.HasSuffix(path, "/preview") {
			return "identity:downgrade:preview"
		}
		return "identity:downgrade:confirm"
	}
	if strings.HasPrefix(path, "/inspirations") {
		if r.Method == http.MethodGet {
			return "content:inspiration:view"
		}
		if path == "/inspirations/batch" {
			return "content:inspiration:publish"
		}
		if strings.Contains(path, "/audit/") {
			return "content:inspiration:audit"
		}
		if strings.HasSuffix(path, "/publish") || strings.HasSuffix(path, "/withdraw") {
			return "content:inspiration:publish"
		}
		return "content:inspiration:manage"
	}
	if path == "/commission-rules" && r.Method == http.MethodGet {
		return "finance:commission-rule:view"
	}
	if path == "/commission-rules" && r.Method == http.MethodPost {
		return "finance:commission-rule:manage"
	}
	if strings.HasPrefix(path, "/commission-rules/") && r.Method == http.MethodPut {
		return "finance:commission-rule:manage"
	}
	if strings.HasPrefix(path, "/storage/") {
		switch {
		case strings.HasPrefix(path, "/storage/configs") && strings.HasSuffix(path, "/test"):
			return "storage:config:test"
		case path == "/storage/configs" && r.Method == http.MethodGet:
			return "storage:config:view"
		case path == "/storage/configs" && r.Method == http.MethodPost:
			return "storage:config:create"
		case strings.HasPrefix(path, "/storage/configs/") && r.Method == http.MethodPut:
			return "storage:config:update"
		case strings.HasPrefix(path, "/storage/configs/") && r.Method == http.MethodDelete:
			return "storage:config:delete"
		case strings.HasPrefix(path, "/storage/files/") && strings.HasSuffix(path, "/download-url"):
			return "storage:file:download"
		case strings.HasPrefix(path, "/storage/files/") && strings.HasSuffix(path, "/restore"):
			return "storage:file:restore"
		case strings.HasPrefix(path, "/storage/files") && r.Method == http.MethodDelete:
			return "storage:file:delete"
		case strings.HasPrefix(path, "/storage/files"):
			return "storage:file:view"
		case strings.HasPrefix(path, "/storage/quotas") && r.Method == http.MethodPut:
			return "storage:quota:update"
		case strings.HasPrefix(path, "/storage/quotas"):
			return "storage:quota:view"
		case strings.HasPrefix(path, "/storage/jobs") && r.Method != http.MethodGet:
			return "storage:job:retry"
		default:
			return "storage:view"
		}
	}
	if path == "/enterprises/export" && r.Method == http.MethodGet {
		return permissionEnterpriseExport
	}
	if path == "/enterprises/certifications" && r.Method == http.MethodGet {
		return permissionEnterpriseCertificationReview
	}
	if path == "/enterprises" {
		if r.Method == http.MethodGet {
			return permissionEnterpriseList
		}
		if r.Method == http.MethodPost {
			return permissionEnterpriseCreate
		}
	}
	if strings.HasPrefix(path, "/enterprises/") {
		if r.Method != http.MethodGet {
			switch {
			case r.Method == http.MethodPatch && strings.Count(strings.Trim(path, "/"), "/") == 1:
				return permissionEnterpriseUpdate
			case strings.HasSuffix(path, "/certifications/review"):
				return permissionEnterpriseCertificationReview
			case strings.HasSuffix(path, "/package/adjust"):
				return permissionEnterprisePackageAdjust
			case strings.HasSuffix(path, "/seats/adjust"):
				return permissionEnterpriseSeatAdjust
			case strings.HasSuffix(path, "/compute/adjust"), strings.HasSuffix(path, "/recharge"):
				return permissionEnterpriseComputeAdjust
			case strings.HasSuffix(path, "/ai-capabilities/configure"):
				return permissionEnterpriseAIConfigure
			case strings.HasSuffix(path, "/attribution/change"):
				return permissionEnterpriseAttributionChange
			case strings.HasSuffix(path, "/risk/disable"):
				return permissionEnterpriseRiskDisable
			case strings.HasSuffix(path, "/risk/restore"):
				return permissionEnterpriseRiskRestore
			case strings.HasSuffix(path, "/service-state"):
				return permissionEnterpriseServiceTransition
			default:
				return permissionEnterpriseUpdate
			}
		}
		switch {
		case strings.HasSuffix(path, "/certifications"):
			return permissionEnterpriseCertificationReview
		case strings.HasSuffix(path, "/members"):
			return permissionEnterpriseMemberView
		case strings.HasSuffix(path, "/package"):
			return permissionEnterprisePackageView
		case strings.HasSuffix(path, "/compute"):
			return permissionEnterpriseComputeView
		case strings.HasSuffix(path, "/transactions"):
			return permissionEnterpriseTransactionView
		case strings.HasSuffix(path, "/orders"):
			return permissionEnterpriseOrderView
		case strings.HasSuffix(path, "/ai-capabilities"):
			return permissionEnterpriseAIView
		case strings.HasSuffix(path, "/ai-employees"):
			return permissionEnterpriseEmployeeView
		case strings.HasSuffix(path, "/knowledge-bases"):
			return permissionEnterpriseKnowledgeView
		case strings.HasSuffix(path, "/attribution"), strings.HasSuffix(path, "/relationships"):
			return permissionEnterpriseAttributionView
		case strings.HasSuffix(path, "/integrations"):
			return permissionEnterpriseConnectorView
		case strings.HasSuffix(path, "/risk"):
			return permissionEnterpriseRiskView
		case strings.HasSuffix(path, "/audit-logs"):
			return permissionEnterpriseAuditView
		default:
			return permissionEnterpriseDetail
		}
	}
	if r.Method == http.MethodGet {
		return "admin.read"
	}
	return "admin.write"
}

func rbacEnforced() bool {
	return strings.EqualFold(os.Getenv("XIANZHI_ENFORCE_RBAC"), "true")
}

func (s *postgresStore) roleHasPermission(ctx context.Context, role string, permission string) (bool, error) {
	if role == "SUPER_ADMIN" {
		return true, nil
	}
	var ok bool
	err := s.db.QueryRowContext(ctx, `
		select exists (
			select 1 from xz_role_permissions
			where role = $1 and (permission = $2 or permission = 'admin.full')
		)
	`, role, permission).Scan(&ok)
	return ok, err
}

func insertAuditDirect(ctx context.Context, db *sql.DB, actorID string, actorRole string, action string, resource string, resourceID string, method string, path string, status int, metadata map[string]any) error {
	id := newAuditID()
	_, err := db.ExecContext(ctx, `
		insert into xz_audit_logs (id, actor_id, actor_role, action, resource, resource_id, method, path, status, metadata)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb)
		on conflict (id) do nothing
	`, id, actorID, actorRole, action, resource, resourceID, method, path, status, jsonProjection(metadata))
	return err
}

func newAuditID() string {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err == nil {
		return "audit_" + time.Now().UTC().Format("20060102150405000000000") + "_" + hex.EncodeToString(suffix[:])
	}
	return "audit_" + time.Now().UTC().Format("20060102150405000000000") + "_" + strconv.FormatInt(time.Now().UnixNano(), 10)
}
