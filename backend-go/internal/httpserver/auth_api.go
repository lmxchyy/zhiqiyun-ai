package httpserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	errInvalidCredentials = errors.New("invalid email or password")
	errUnauthorized       = errors.New("unauthorized")
	errForbidden          = errors.New("forbidden")
)

const authSessionTTL = 24 * time.Hour

type authSessionStore interface {
	Put(context.Context, string, string, time.Duration) error
	UserID(context.Context, string) (string, bool, error)
	Delete(context.Context, string) error
}

type redisAuthSessions struct {
	client *redis.Client
}

func newRedisAuthSessions(client *redis.Client) authSessionStore {
	if client == nil {
		return nil
	}
	return redisAuthSessions{client: client}
}

func (s redisAuthSessions) Put(ctx context.Context, token string, userID string, ttl time.Duration) error {
	return s.client.Set(ctx, authSessionKey(token), userID, ttl).Err()
}

func (s redisAuthSessions) UserID(ctx context.Context, token string) (string, bool, error) {
	userID, err := s.client.Get(ctx, authSessionKey(token)).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return userID, true, nil
}

func (s redisAuthSessions) Delete(ctx context.Context, token string) error {
	return s.client.Del(ctx, authSessionKey(token)).Err()
}

func authSessionKey(token string) string {
	return "auth:session:" + token
}

type authAPI struct {
	store    platformStore
	sessions authSessionStore
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type authTokenPayload struct {
	UserID string `json:"userId"`
	Issued int64  `json:"issued"`
}

func newAuthAPI(store platformStore, sessions authSessionStore) authAPI {
	return authAPI{store: store, sessions: sessions}
}

func (a authAPI) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	user, ok := findLoginUser(data.Users, req.Email, req.Password)
	if !ok {
		writeError(w, http.StatusUnauthorized, errInvalidCredentials)
		return
	}
	response := authResponse(data, user, false)
	token := encodeAuthToken(user.ID)
	if a.sessions != nil {
		token, err = randomAuthToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if err := a.sessions.Put(r.Context(), token, user.ID, authSessionTTL); err != nil {
			writeError(w, http.StatusServiceUnavailable, err)
			return
		}
	}
	response["accessToken"] = token
	writeJSON(w, response)
}

func (a authAPI) me(w http.ResponseWriter, r *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	user, err := a.authenticatedUser(r, data)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, authResponse(data, user, false))
}

func (a authAPI) logout(w http.ResponseWriter, r *http.Request) {
	if a.sessions != nil {
		token := bearerToken(r)
		if token != "" {
			if err := a.sessions.Delete(r.Context(), token); err != nil {
				writeError(w, http.StatusServiceUnavailable, err)
				return
			}
		}
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a authAPI) changePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.CurrentPassword = strings.TrimSpace(req.CurrentPassword)
	req.NewPassword = strings.TrimSpace(req.NewPassword)
	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, errors.New("new password must be at least 8 characters"))
		return
	}
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	user, err := a.authenticatedUser(r, data)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if !passwordMatches(user, req.CurrentPassword) {
		writeError(w, http.StatusUnauthorized, errInvalidCredentials)
		return
	}
	updated, err := a.store.UpdateUserPassword(user.ID, hashPassword(req.NewPassword))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "user": userView(updated)})
}

func (a authAPI) authenticatedUser(r *http.Request, data adminPlatformData) (adminUser, error) {
	token := bearerToken(r)
	if token == "" {
		return adminUser{}, errUnauthorized
	}
	userID := ""
	if a.sessions != nil {
		cachedUserID, ok, err := a.sessions.UserID(r.Context(), token)
		if err != nil || !ok {
			return adminUser{}, errUnauthorized
		}
		userID = cachedUserID
	} else {
		payload, err := decodeAuthToken(token)
		if err != nil || payload.UserID == "" {
			return adminUser{}, errUnauthorized
		}
		userID = payload.UserID
	}
	for _, user := range data.Users {
		if user.ID == userID && strings.EqualFold(user.Status, "ACTIVE") {
			return user, nil
		}
	}
	return adminUser{}, errUnauthorized
}

func bearerToken(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

func findLoginUser(users []adminUser, email string, password string) (adminUser, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	password = strings.TrimSpace(password)
	for _, user := range users {
		if strings.ToLower(user.Email) != email || !strings.EqualFold(user.Status, "ACTIVE") {
			continue
		}
		if passwordMatches(user, password) {
			return user, true
		}
	}
	return adminUser{}, false
}

func passwordMatches(user adminUser, password string) bool {
	if user.PasswordHash != "" {
		return user.PasswordHash == hashPassword(password)
	}
	return demoPasswordForRole(user.Role, user.Email) == password
}

func hashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return fmt.Sprintf("sha256:%x", sum)
}

func demoPasswordForRole(role string, email string) string {
	switch {
	case strings.EqualFold(email, "admin@xianzhi.ai"), role == "SUPER_ADMIN":
		return "Admin123!"
	case strings.HasPrefix(role, "AGENT"):
		return "Agent123!"
	default:
		return "Demo123!"
	}
}

func authResponse(data adminPlatformData, user adminUser, includeToken bool) map[string]any {
	response := map[string]any{
		"user":          userView(user),
		"permissions":   permissionsForRole(user.Role),
		"defaultModule": defaultModuleForRole(user.Role),
		"workspace":     workspaceForRole(user.Role),
		"defaultRoute":  defaultRouteForRole(user.Role),
	}
	if includeToken {
		response["accessToken"] = encodeAuthToken(user.ID)
	}
	if agent, ok := channelAgentForUser(data.ChannelAgents, user.ID); ok {
		response["agent"] = channelAgentView(agent, user)
	}
	return response
}

func randomAuthToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func encodeAuthToken(userID string) string {
	raw, _ := json.Marshal(authTokenPayload{UserID: userID, Issued: time.Now().UTC().Unix()})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeAuthToken(token string) (authTokenPayload, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return authTokenPayload{}, err
	}
	var payload authTokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return authTokenPayload{}, err
	}
	return payload, nil
}

func userView(user adminUser) map[string]any {
	return map[string]any{
		"id":        user.ID,
		"email":     user.Email,
		"name":      user.Name,
		"role":      user.Role,
		"status":    user.Status,
		"planId":    user.PlanID,
		"createdAt": user.CreatedAt,
		"updatedAt": user.UpdatedAt,
	}
}

func channelAgentForUser(agents []adminChannelAgent, userID string) (adminChannelAgent, bool) {
	for _, agent := range agents {
		if agent.UserID == userID {
			return agent, true
		}
	}
	return adminChannelAgent{}, false
}

func permissionsForRole(role string) []string {
	switch {
	case role == "SUPER_ADMIN":
		return []string{"admin.full", "channel.dashboard", "channel.customers.read", "channel.commissions.read", "channel.withdrawals.create"}
	case strings.HasPrefix(role, "AGENT"):
		return []string{"channel.dashboard", "channel.customers.read", "channel.commissions.read", "channel.withdrawals.create"}
	default:
		return []string{"workspace.use", "generation.create", "assets.read"}
	}
}

func defaultModuleForRole(role string) string {
	if strings.HasPrefix(role, "AGENT") {
		return "agentHome"
	}
	if role == "SUPER_ADMIN" {
		return "admin"
	}
	return "dashboard"
}

func workspaceForRole(role string) string {
	if strings.HasPrefix(role, "AGENT") {
		return "agent"
	}
	if role == "SUPER_ADMIN" {
		return "admin"
	}
	return "user"
}

func defaultRouteForRole(role string) string {
	switch workspaceForRole(role) {
	case "agent":
		return "/agent"
	case "admin":
		return "/admin/"
	default:
		return "/app"
	}
}
