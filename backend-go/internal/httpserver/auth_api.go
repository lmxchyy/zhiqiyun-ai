package httpserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

var (
	errInvalidCredentials                  = errors.New("invalid email or password")
	errUnauthorized                        = errors.New("unauthorized")
	errForbidden                           = errors.New("forbidden")
	errAuthSessionUnavailable              = errors.New("auth session store unavailable")
	errWeChatMiniProgramLoginNotConfigured = errors.New("wechat mini program login is not configured")
)

const authSessionTTL = 24 * time.Hour
const authRefreshSessionTTL = 30 * 24 * time.Hour
const wechatMiniProgramMockCode = "mock-devtools-code"
const passwordHashBcryptPrefix = "bcrypt:"

type authSessionStore interface {
	Put(context.Context, string, string, time.Duration) error
	UserID(context.Context, string) (string, bool, error)
	Delete(context.Context, string) error
}

type localAuthSession struct {
	userID    string
	expiresAt time.Time
}

type localAuthSessions struct {
	mu       sync.Mutex
	sessions map[string]localAuthSession
}

func newLocalAuthSessions() authSessionStore {
	return &localAuthSessions{sessions: map[string]localAuthSession{}}
}

func (s *localAuthSessions) Put(_ context.Context, token string, userID string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[authSessionKey(token)] = localAuthSession{userID: userID, expiresAt: time.Now().Add(ttl)}
	return nil
}

func (s *localAuthSessions) UserID(_ context.Context, token string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := authSessionKey(token)
	session, ok := s.sessions[key]
	if !ok {
		return "", false, nil
	}
	if !session.expiresAt.IsZero() && time.Now().After(session.expiresAt) {
		delete(s.sessions, key)
		return "", false, nil
	}
	return session.userID, true, nil
}

func (s *localAuthSessions) Delete(_ context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, authSessionKey(token))
	return nil
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

func refreshSessionToken(token string) string {
	return "refresh:" + token
}

type authAPI struct {
	store    platformStore
	sessions authSessionStore
	flow     *authFlowCoordinator
}

type loginRequest struct {
	Account  string `json:"account"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Mobile   string `json:"mobile"`
	Password string `json:"password"`
}

type wechatMiniProgramLoginRequest struct {
	Code           string `json:"code"`
	WxLoginCode    string `json:"wxLoginCode"`
	PhoneCode      string `json:"phoneCode"`
	InviteCode     string `json:"inviteCode"`
	Scene          string `json:"scene"`
	PromoterCode   string `json:"promoterCode"`
	CampaignCode   string `json:"campaignCode"`
	RedirectSource string `json:"redirectSource"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type refreshTokenRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type registerRequest struct {
	Username        string `json:"username"`
	Name            string `json:"name"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirmPassword"`
	InviteCode      string `json:"inviteCode"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type authTokenPayload struct {
	UserID string `json:"userId"`
	Issued int64  `json:"issued"`
}

type wechatMiniProgramSession struct {
	OpenID  string
	UnionID string
}

func newAuthAPI(store platformStore, sessions authSessionStore) authAPI {
	return authAPI{store: store, sessions: sessions, flow: newAuthFlowCoordinator()}
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
	account := firstNonEmptyString(req.Account, req.Mobile, req.Username, req.Email)
	user, ok, needsPasswordUpgrade, accountStatus := findLoginUserByAccount(data.Users, account, req.Password)
	if !ok {
		if strings.EqualFold(accountStatus, "FROZEN") || strings.EqualFold(accountStatus, "DISABLED") {
			writeAuthFlowError(w, http.StatusLocked, "ACCOUNT_FROZEN", "账号暂时无法使用")
			return
		}
		if strings.EqualFold(accountStatus, "DEACTIVATED") || strings.EqualFold(accountStatus, "DELETED") {
			writeAuthFlowError(w, http.StatusGone, "ACCOUNT_DEACTIVATED", "账号已注销")
			return
		}
		writeError(w, http.StatusUnauthorized, errInvalidCredentials)
		return
	}
	if needsPasswordUpgrade {
		updated, err := a.updatePasswordHash(user.ID, req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		user = updated
		data = dataWithUpdatedUser(data, updated)
	}
	response, err := a.authResponseWithToken(r.Context(), data, user)
	if err != nil {
		writeAuthTokenError(w, err)
		return
	}
	writeJSON(w, response)
}

func (a authAPI) wechatMiniProgramLogin(w http.ResponseWriter, r *http.Request) {
	var req wechatMiniProgramLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	code := strings.TrimSpace(firstNonEmptyString(req.WxLoginCode, req.Code))
	if code == "" {
		writeError(w, http.StatusBadRequest, errors.New("wechat mini program code is required"))
		return
	}
	if isWeChatMiniProgramMockCodeValue(code) && !wechatMiniProgramMockLoginEnabled() {
		writeError(w, http.StatusForbidden, errors.New("wechat mini program mock login is disabled"))
		return
	}
	mockLogin := isWeChatMiniProgramMockCode(code)
	log.Printf("wechat mini program login request mock=%t", mockLogin)

	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var user adminUser
	isNewUser := false
	inviteBindStatus := "not_applicable"
	if mockLogin {
		var ok bool
		user, ok = findActiveUserByEmail(data.Users, "demo@xianzhi.ai")
		if !ok {
			log.Printf("wechat mini program mock login failed: demo user inactive or missing")
			writeError(w, http.StatusUnauthorized, errUnauthorized)
			return
		}
	} else {
		session, err := exchangeWeChatMiniProgramCode(r.Context(), code)
		if err != nil {
			log.Printf("wechat mini program code exchange failed: %v", err)
			if errors.Is(err, errWeChatMiniProgramLoginNotConfigured) {
				writeError(w, http.StatusNotImplemented, err)
				return
			}
			writeError(w, http.StatusBadGateway, err)
			return
		}
		if strings.TrimSpace(req.PhoneCode) == "" {
			writeAuthFlowError(w, http.StatusBadRequest, "WECHAT_PHONE_CODE_REQUIRED", "wechat phone authorization code is required")
			return
		}
		mobile, err := exchangeWeChatPhoneCode(r.Context(), req.PhoneCode)
		if err != nil {
			writeAuthFlowError(w, http.StatusBadGateway, "WECHAT_PHONE_AUTH_FAILED", "未能验证微信手机号授权")
			return
		}
		data, user, isNewUser, inviteBindStatus, err = a.userForPhoneIdentity(mobile, session, authRegistrationInput{
			InviteCode: req.InviteCode, Scene: req.Scene, PromoterCode: req.PromoterCode,
			CampaignCode: req.CampaignCode, RedirectSource: req.RedirectSource, IdempotencyKey: req.IdempotencyKey,
		})
		if err != nil {
			writeMappedAuthFlowError(w, err)
			return
		}
	}

	response, err := a.authResponseWithToken(r.Context(), data, user)
	if err != nil {
		log.Printf("wechat mini program login token issue failed: %v", err)
		writeAuthTokenError(w, err)
		return
	}
	response["isNewUser"] = isNewUser
	response["registrationStatus"] = map[bool]string{true: "created", false: "existing"}[isNewUser]
	response["inviteBindStatus"] = inviteBindStatus
	response["expiresIn"] = int(authSessionTTL.Seconds())
	if isNewUser {
		response["newcomerBenefits"] = []map[string]string{{"title": "新人体验权益已到账", "status": "granted"}}
	}
	log.Printf("wechat mini program login succeeded user=%s mock=%t", user.ID, mockLogin)
	writeJSON(w, response)
}

func (a authAPI) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Username = strings.TrimSpace(firstNonEmpty([]string{req.Username, req.Name}))
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Password = strings.TrimSpace(req.Password)
	req.ConfirmPassword = strings.TrimSpace(req.ConfirmPassword)
	req.InviteCode = strings.ToUpper(strings.TrimSpace(req.InviteCode))
	if req.Username == "" || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, errors.New("username, email and password are required"))
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, errors.New("password must be at least 8 characters"))
		return
	}
	if req.ConfirmPassword != "" && req.ConfirmPassword != req.Password {
		writeError(w, http.StatusBadRequest, errors.New("password confirmation does not match"))
		return
	}
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, user := range data.Users {
		if strings.EqualFold(user.Email, req.Email) {
			writeError(w, http.StatusBadRequest, errors.New("email already exists"))
			return
		}
	}
	referredBy := ""
	if req.InviteCode != "" {
		agent, ok := channelAgentForInviteCode(data.ChannelAgents, req.InviteCode)
		if ok {
			referredBy = agent.UserID
		}
	}
	created, err := a.store.CreateAdminCustomer(adminCustomerMutation{
		Name:       req.Username,
		Email:      req.Email,
		Role:       "MEMBER",
		Status:     "ACTIVE",
		PlanID:     "plan_free",
		ReferredBy: referredBy,
		Available:  100,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	passwordHash, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	created, err = a.store.UpdateUserPassword(created.ID, passwordHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	response, err := a.authResponseWithToken(r.Context(), dataWithRegisteredUser(data, created), created)
	if err != nil {
		writeAuthTokenError(w, err)
		return
	}
	writeJSON(w, response)
}

func (a authAPI) me(w http.ResponseWriter, r *http.Request) {
	if store, ok := a.store.(activeIdentityStore); ok {
		userID, err := authenticatedUserID(r, a.sessions)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
		}
		user, found, err := store.GetActiveUser(userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if !found {
			writeError(w, http.StatusUnauthorized, errUnauthorized)
			return
		}
		identityData := adminPlatformData{Users: []adminUser{user}}
		agent, hasAgent, err := store.GetChannelAgentForUser(user.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if hasAgent {
			identityData.ChannelAgents = []adminChannelAgent{agent}
		}
		if operationStore, ok := a.store.(operationCenterIdentityStore); ok {
			center, hasCenter, err := operationStore.GetOperationCenterForUser(user.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			if hasCenter {
				identityData.OperationCenters = []adminOperationCenter{center}
			}
		}
		writeJSON(w, authResponse(identityData, user, false))
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
	writeJSON(w, authResponse(data, user, false))
}

func (a authAPI) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	refreshToken := strings.TrimSpace(req.RefreshToken)
	if refreshToken == "" {
		writeError(w, http.StatusBadRequest, errors.New("refresh token is required"))
		return
	}
	if a.sessions == nil {
		writeError(w, http.StatusServiceUnavailable, errAuthSessionUnavailable)
		return
	}
	userID, ok, err := a.sessions.UserID(r.Context(), refreshSessionToken(refreshToken))
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, errUnauthorized)
		return
	}
	if err := a.sessions.Delete(r.Context(), refreshSessionToken(refreshToken)); err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, user := range data.Users {
		if user.ID == userID && strings.EqualFold(user.Status, "ACTIVE") {
			response, err := a.authResponseWithToken(r.Context(), data, user)
			if err != nil {
				writeAuthTokenError(w, err)
				return
			}
			writeJSON(w, response)
			return
		}
	}
	writeError(w, http.StatusUnauthorized, errUnauthorized)
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
	matches := user.PasswordHash == ""
	if !matches {
		matches, _ = passwordMatches(user, req.CurrentPassword)
	}
	if !matches {
		writeError(w, http.StatusUnauthorized, errInvalidCredentials)
		return
	}
	updated, err := a.updatePasswordHash(user.ID, req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "passwordSet": true, "user": userView(updated)})
}

func (a authAPI) authenticatedUser(r *http.Request, data adminPlatformData) (adminUser, error) {
	userID, err := authenticatedUserID(r, a.sessions)
	if err != nil {
		return adminUser{}, err
	}
	for _, user := range data.Users {
		if user.ID == userID && strings.EqualFold(user.Status, "ACTIVE") {
			return user, nil
		}
	}
	return adminUser{}, errUnauthorized
}

func (a authAPI) authResponseWithToken(ctx context.Context, data adminPlatformData, user adminUser) (map[string]any, error) {
	response := authResponse(data, user, false)
	if a.sessions == nil {
		if !devAuthFallbackEnabled() {
			return nil, errAuthSessionUnavailable
		}
		response["accessToken"] = encodeAuthToken(user.ID)
		return response, nil
	}
	token, err := randomAuthToken()
	if err != nil {
		return nil, err
	}
	refreshToken, err := randomAuthToken()
	if err != nil {
		return nil, err
	}
	if err := a.sessions.Put(ctx, token, user.ID, authSessionTTL); err != nil {
		return nil, fmt.Errorf("%w: %v", errAuthSessionUnavailable, err)
	}
	if err := a.sessions.Put(ctx, refreshSessionToken(refreshToken), user.ID, authRefreshSessionTTL); err != nil {
		return nil, fmt.Errorf("%w: %v", errAuthSessionUnavailable, err)
	}
	response["refreshToken"] = refreshToken
	response["accessToken"] = token
	return response, nil
}

func (a authAPI) updatePasswordHash(userID string, password string) (adminUser, error) {
	passwordHash, err := hashPassword(password)
	if err != nil {
		return adminUser{}, err
	}
	return a.store.UpdateUserPassword(userID, passwordHash)
}

func writeAuthTokenError(w http.ResponseWriter, err error) {
	if errors.Is(err, errAuthSessionUnavailable) {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}

func (a authAPI) userForWeChatMiniProgramSession(data adminPlatformData, session wechatMiniProgramSession) (adminPlatformData, adminUser, error) {
	email := wechatMiniProgramSyntheticEmail(session.OpenID)
	if user, ok := findUserByEmail(data.Users, email); ok {
		if !strings.EqualFold(user.Status, "ACTIVE") {
			return data, adminUser{}, errUnauthorized
		}
		return data, user, nil
	}

	created, err := a.store.CreateAdminCustomer(adminCustomerMutation{
		Name:      "WeChat User",
		Email:     email,
		Role:      "MEMBER",
		Status:    "ACTIVE",
		PlanID:    "plan_free",
		Available: 100,
	})
	if err != nil {
		return data, adminUser{}, err
	}
	passwordHash, err := hashPassword("wechat-mini-program:" + session.OpenID)
	if err != nil {
		return data, adminUser{}, err
	}
	created, err = a.store.UpdateUserPassword(created.ID, passwordHash)
	if err != nil {
		return data, adminUser{}, err
	}
	return dataWithRegisteredUser(data, created), created, nil
}

func authenticatedUserID(r *http.Request, sessions authSessionStore) (string, error) {
	token := bearerToken(r)
	if token == "" {
		return "", errUnauthorized
	}
	if sessions != nil {
		userID, ok, err := sessions.UserID(r.Context(), token)
		if err != nil || !ok || userID == "" {
			return "", errUnauthorized
		}
		return userID, nil
	}
	if !devAuthFallbackEnabled() {
		return "", errUnauthorized
	}
	payload, err := decodeAuthToken(token)
	if err != nil || payload.UserID == "" {
		return "", errUnauthorized
	}
	return payload.UserID, nil
}

func bearerToken(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

func exchangeWeChatMiniProgramCode(ctx context.Context, code string) (wechatMiniProgramSession, error) {
	appID := strings.TrimSpace(os.Getenv("WECHAT_MINI_PROGRAM_APPID"))
	secret := strings.TrimSpace(os.Getenv("WECHAT_MINI_PROGRAM_SECRET"))
	if appID == "" || secret == "" {
		return wechatMiniProgramSession{}, errWeChatMiniProgramLoginNotConfigured
	}

	values := url.Values{}
	values.Set("appid", appID)
	values.Set("secret", secret)
	values.Set("js_code", code)
	values.Set("grant_type", "authorization_code")
	endpoint := "https://api.weixin.qq.com/sns/jscode2session?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return wechatMiniProgramSession{}, err
	}
	client := http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return wechatMiniProgramSession{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return wechatMiniProgramSession{}, fmt.Errorf("wechat code2session status %d", resp.StatusCode)
	}

	var payload struct {
		OpenID     string `json:"openid"`
		UnionID    string `json:"unionid"`
		SessionKey string `json:"session_key"`
		ErrCode    int    `json:"errcode"`
		ErrMsg     string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return wechatMiniProgramSession{}, err
	}
	if payload.ErrCode != 0 {
		message := strings.TrimSpace(payload.ErrMsg)
		if message == "" {
			message = fmt.Sprintf("errcode %d", payload.ErrCode)
		}
		return wechatMiniProgramSession{}, fmt.Errorf("wechat code2session failed: %s", message)
	}
	if strings.TrimSpace(payload.OpenID) == "" {
		return wechatMiniProgramSession{}, errors.New("wechat code2session did not return openid")
	}
	return wechatMiniProgramSession{OpenID: strings.TrimSpace(payload.OpenID), UnionID: strings.TrimSpace(payload.UnionID)}, nil
}

func isWeChatMiniProgramMockCode(code string) bool {
	return isWeChatMiniProgramMockCodeValue(code) && wechatMiniProgramMockLoginEnabled()
}

func isWeChatMiniProgramMockCodeValue(code string) bool {
	code = strings.TrimSpace(strings.ToLower(code))
	return code == wechatMiniProgramMockCode || strings.HasPrefix(code, "mock-")
}

func findUserByEmail(users []adminUser, email string) (adminUser, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, user := range users {
		if strings.ToLower(user.Email) == email {
			return user, true
		}
	}
	return adminUser{}, false
}

func findActiveUserByEmail(users []adminUser, email string) (adminUser, bool) {
	user, ok := findUserByEmail(users, email)
	if !ok || !strings.EqualFold(user.Status, "ACTIVE") {
		return adminUser{}, false
	}
	return user, true
}

func wechatMiniProgramSyntheticEmail(openID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(openID)))
	return fmt.Sprintf("wx_%x@wechat.local", sum[:12])
}

func findLoginUser(users []adminUser, email string, password string) (adminUser, bool, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	password = strings.TrimSpace(password)
	for _, user := range users {
		if strings.ToLower(user.Email) != email || !strings.EqualFold(user.Status, "ACTIVE") {
			continue
		}
		matches, needsUpgrade := passwordMatches(user, password)
		if matches {
			return user, true, needsUpgrade
		}
	}
	return adminUser{}, false, false
}

func channelAgentForInviteCode(agents []adminChannelAgent, inviteCode string) (adminChannelAgent, bool) {
	inviteCode = strings.ToUpper(strings.TrimSpace(inviteCode))
	for _, agent := range agents {
		if strings.EqualFold(agent.InviteCode, inviteCode) && strings.EqualFold(agent.Status, "ACTIVE") {
			return agent, true
		}
	}
	return adminChannelAgent{}, false
}

func dataWithRegisteredUser(data adminPlatformData, user adminUser) adminPlatformData {
	data.Users = append(data.Users, user)
	return data
}

func dataWithUpdatedUser(data adminPlatformData, user adminUser) adminPlatformData {
	for i := range data.Users {
		if data.Users[i].ID == user.ID {
			data.Users[i] = user
			return data
		}
	}
	return dataWithRegisteredUser(data, user)
}

func passwordMatches(user adminUser, password string) (bool, bool) {
	if user.PasswordHash != "" {
		return verifyPasswordHash(user.PasswordHash, password)
	}
	if strings.TrimSpace(user.Mobile) != "" {
		return false, false
	}
	return demoPasswordForRole(user.Role, user.Email) == password, true
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return passwordHashBcryptPrefix + string(hash), nil
}

func legacySHA256PasswordHash(password string) string {
	sum := sha256.Sum256([]byte(password))
	return fmt.Sprintf("sha256:%x", sum)
}

func verifyPasswordHash(storedHash string, password string) (bool, bool) {
	storedHash = strings.TrimSpace(storedHash)
	switch {
	case strings.HasPrefix(storedHash, passwordHashBcryptPrefix):
		err := bcrypt.CompareHashAndPassword([]byte(strings.TrimPrefix(storedHash, passwordHashBcryptPrefix)), []byte(password))
		return err == nil, false
	case strings.HasPrefix(storedHash, "$2a$"), strings.HasPrefix(storedHash, "$2b$"), strings.HasPrefix(storedHash, "$2y$"):
		err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password))
		return err == nil, false
	case strings.HasPrefix(storedHash, "sha256:"):
		return storedHash == legacySHA256PasswordHash(password), true
	default:
		return false, false
	}
}

func devAuthFallbackEnabled() bool {
	return (boolEnvAuth(os.Getenv("XIANZHI_DEV_AUTH_FALLBACK")) || boolEnvAuth(os.Getenv("XIANZHI_ALLOW_INSECURE_AUTH_TOKEN"))) && !authProductionEnvironment()
}

func wechatMiniProgramMockLoginEnabled() bool {
	return (boolEnvAuth(os.Getenv("XIANZHI_ENABLE_MOCK_LOGIN")) || boolEnvAuth(os.Getenv("XIANZHI_ALLOW_WECHAT_MOCK_LOGIN"))) && !authProductionEnvironment()
}

func authProductionEnvironment() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("XIANZHI_ENV")))
	return value == "production" || value == "prod"
}

func boolEnvAuth(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
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
	agent, hasAgent := channelAgentForUser(data.ChannelAgents, user.ID)
	operationCenter, hasOperationCenter := activeOperationCenterForUser(data.OperationCenters, user.ID)
	roles := rolesForUser(data, user)
	loginPermissions := append([]string{}, permissionsForIdentity(user.Role, hasAgent, hasOperationCenter)...)
	loginPermissions = appendUnique(loginPermissions, permissionsForCurrentRole(roleUser)...)
	response := map[string]any{
		"user":           userView(user),
		"tenantId":       firstNonEmptyString(user.TenantID, "tenant_default"),
		"organizationId": firstNonEmptyString(user.OrganizationID, "organization_default"),
		"roles":          roles,
		"currentRole":    roleUser,
		"permissions":    loginPermissions,
		"defaultModule":  defaultModuleForIdentity(user.Role, hasOperationCenter),
		"workspace":      workspaceForRole(user.Role),
		"defaultRoute":   defaultRouteForIdentity(user.Role, hasOperationCenter),
	}
	if includeToken {
		if devAuthFallbackEnabled() {
			response["accessToken"] = encodeAuthToken(user.ID)
		}
	}
	if hasAgent {
		response["agent"] = channelAgentView(agent, user)
	}
	if hasOperationCenter {
		response["operationCenter"] = operationCenter
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
		"id":                    user.ID,
		"tenantId":              firstNonEmptyString(user.TenantID, "tenant_default"),
		"organizationId":        firstNonEmptyString(user.OrganizationID, "organization_default"),
		"email":                 user.Email,
		"mobileMasked":          maskedMobile(user.Mobile),
		"passwordSet":           user.PasswordHash != "",
		"wechatLinked":          len(user.WeChatOpenIDs) > 0 || user.WeChatUnionID != "",
		"name":                  user.Name,
		"role":                  user.Role,
		"memberLevel":           user.MemberLevel,
		"agentStatus":           user.AgentStatus,
		"operationCenterStatus": user.OperationCenterStatus,
		"status":                user.Status,
		"planId":                user.PlanID,
		"createdAt":             user.CreatedAt,
		"updatedAt":             user.UpdatedAt,
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

func activeOperationCenterForUser(centers []adminOperationCenter, userID string) (adminOperationCenter, bool) {
	center, ok := operationCenterForUser(centers, userID)
	if !ok || !strings.EqualFold(center.Status, "ACTIVE") {
		return adminOperationCenter{}, false
	}
	return center, true
}

func permissionsForRole(role string) []string {
	switch {
	case role == "SUPER_ADMIN":
		return append([]string{"admin.full", "channel.dashboard", "channel.customers.read", "channel.commissions.read", "channel.withdrawals.create", "operation_center.dashboard", "operation_center.agents.read", "operation_center.orders.read", "operation_center.commissions.read"}, adminEnterprisePermissions...)
	case role == "ENTERPRISE_OPERATOR" || role == "CERTIFICATION_REVIEWER" || role == "FINANCE" || role == "RISK_MANAGER" || role == "CUSTOMER_SERVICE":
		return append([]string{}, adminEnterpriseRolePermissionMatrix[role]...)
	case strings.HasPrefix(role, "AGENT"):
		return []string{"workspace.use", "generation.create", "assets.read", "channel.dashboard", "channel.customers.read", "channel.commissions.read", "channel.withdrawals.create"}
	default:
		return []string{"workspace.use", "generation.create", "assets.read"}
	}
}

func permissionsForIdentity(role string, hasAgent bool, hasOperationCenter bool) []string {
	permissions := append([]string{}, permissionsForRole(role)...)
	if hasAgent && !stringSliceContains(permissions, "channel.dashboard") {
		permissions = append(permissions, "channel.dashboard", "channel.customers.read", "channel.commissions.read", "channel.withdrawals.create")
	}
	if hasOperationCenter && !stringSliceContains(permissions, "operation_center.dashboard") {
		permissions = append(permissions, "operation_center.dashboard", "operation_center.agents.read", "operation_center.orders.read", "operation_center.commissions.read")
	}
	return permissions
}

func stringSliceContains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func defaultModuleForRole(role string) string {
	if isPlatformAdminRole(role) {
		return "admin"
	}
	return "dashboard"
}

func defaultModuleForIdentity(role string, hasOperationCenter bool) string {
	if isPlatformAdminRole(role) {
		return "admin"
	}
	if hasOperationCenter {
		return "operationCenterDashboard"
	}
	return "dashboard"
}

func workspaceForRole(role string) string {
	if isPlatformAdminRole(role) {
		return "admin"
	}
	return "user"
}

func defaultRouteForIdentity(role string, hasOperationCenter bool) string {
	if isPlatformAdminRole(role) {
		return "/admin/"
	}
	if hasOperationCenter {
		return "/app/operation-center"
	}
	return defaultRouteForRole(role)
}

func isPlatformAdminRole(role string) bool {
	role = strings.ToUpper(strings.TrimSpace(role))
	return role == "SUPER_ADMIN" || role == "ENTERPRISE_OPERATOR" || role == "CERTIFICATION_REVIEWER" || role == "FINANCE" || role == "RISK_MANAGER" || role == "CUSTOMER_SERVICE"
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
