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
		if !rbacEnforced() {
			if user.Role != "SUPER_ADMIN" {
				c.AbortWithStatusJSON(http.StatusForbidden, map[string]string{"error": errForbidden.Error()})
				return
			}
			c.Set("actorID", user.ID)
			c.Set("actorRole", user.Role)
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
		c.Next()
	}
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
