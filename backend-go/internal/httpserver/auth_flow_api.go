package httpserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

const (
	smsCodeTTL      = 5 * time.Minute
	smsSendInterval = 60 * time.Second
	smsMaxAttempts  = 5
)

type authFlowCoordinator struct {
	registrationMu sync.Mutex
	smsMu          sync.Mutex
	smsChallenges  map[string]smsChallenge
	smsNextSend    map[string]time.Time
}

type smsChallenge struct {
	codeHash   [32]byte
	expiresAt  time.Time
	nextSendAt time.Time
	attempts   int
}

type authFlowError struct {
	status  int
	code    string
	message string
	details map[string]any
}

func (e *authFlowError) Error() string { return e.message }

type authRegistrationInput struct {
	Context        context.Context
	InviteToken    string
	InviteCode     string
	Scene          string
	PromoterCode   string
	CampaignCode   string
	RedirectSource string
	IdempotencyKey string
}

type smsSendRequest struct {
	Mobile  string `json:"mobile"`
	Purpose string `json:"purpose"`
}

type smsLoginRequest struct {
	Mobile         string `json:"mobile"`
	SMSCode        string `json:"smsCode"`
	InviteCode     string `json:"inviteCode"`
	InviteToken    string `json:"inviteToken"`
	Scene          string `json:"scene"`
	PromoterCode   string `json:"promoterCode"`
	CampaignCode   string `json:"campaignCode"`
	RedirectSource string `json:"redirectSource"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type mobileBindRequest struct {
	Mobile  string `json:"mobile"`
	SMSCode string `json:"smsCode"`
}

func newAuthFlowCoordinator() *authFlowCoordinator {
	return &authFlowCoordinator{smsChallenges: map[string]smsChallenge{}, smsNextSend: map[string]time.Time{}}
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.EqualFold(strings.TrimSpace(existing), value) {
			return values
		}
	}
	return append(values, value)
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			cloned[key] = trimmed
		}
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}

func writeAuthFlowError(w http.ResponseWriter, status int, code, message string) {
	writeAuthFlowErrorDetails(w, status, code, message, nil)
}

func writeAuthFlowErrorDetails(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	payload := map[string]any{"code": code, "errorCode": code, "error": message, "message": message}
	for key, value := range details {
		payload[key] = value
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func writeMappedAuthFlowError(w http.ResponseWriter, err error) {
	var flowErr *authFlowError
	if errors.As(err, &flowErr) {
		writeAuthFlowErrorDetails(w, flowErr.status, flowErr.code, flowErr.message, flowErr.details)
		return
	}
	writeAuthFlowError(w, http.StatusInternalServerError, "AUTH_INTERNAL_ERROR", "登录服务暂时不可用")
}

func normalizeMainlandMobile(value string) string {
	var builder strings.Builder
	for _, char := range strings.TrimSpace(value) {
		if char >= '0' && char <= '9' {
			builder.WriteRune(char)
		}
	}
	mobile := builder.String()
	if strings.HasPrefix(mobile, "86") && len(mobile) == 13 {
		mobile = mobile[2:]
	}
	return mobile
}

func validMainlandMobile(value string) bool {
	mobile := normalizeMainlandMobile(value)
	return len(mobile) == 11 && mobile[0] == '1' && mobile[1] >= '3' && mobile[1] <= '9'
}

func maskedMobile(value string) string {
	mobile := normalizeMainlandMobile(value)
	if len(mobile) != 11 {
		return ""
	}
	return mobile[:3] + "****" + mobile[7:]
}

func authCodeHash(mobile, code string) [32]byte {
	return sha256.Sum256([]byte(normalizeMainlandMobile(mobile) + ":" + strings.TrimSpace(code)))
}

func randomSMSCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}

func developmentSMSCode() string {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("XIANZHI_ENV")))
	if env == "production" || env == "prod" {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(os.Getenv("XIANZHI_SMS_DEV_CODE"), os.Getenv("SMS_DEV_CODE")))
}

func sendSMSProvider(ctx context.Context, cfg config.Config, mobile, code string) error {
	if devCode := developmentSMSCode(); devCode != "" {
		return nil
	}
	providerURL := strings.TrimSpace(cfg.SMSProviderURL)
	if providerURL == "" {
		return &authFlowError{status: http.StatusServiceUnavailable, code: "SMS_NOT_CONFIGURED", message: "短信服务尚未配置"}
	}
	payload, _ := json.Marshal(map[string]string{
		"mobile": mobile, "code": code, "templateId": strings.TrimSpace(cfg.SMSTemplateID),
		"signature": strings.TrimSpace(cfg.SMSSignature),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, providerURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(cfg.SMSProviderAPIKey); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return &authFlowError{status: http.StatusBadGateway, code: "SMS_SEND_FAILED", message: "验证码发送失败"}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &authFlowError{status: http.StatusBadGateway, code: "SMS_SEND_FAILED", message: "验证码发送失败"}
	}
	return nil
}

func (a authAPI) smsStore() (smsChallengeStore, bool) {
	if a.sessions == nil {
		return nil, false
	}
	store, ok := a.sessions.(smsChallengeStore)
	return store, ok
}

func (a authAPI) smsNextSendAt(ctx context.Context, mobile string) (time.Time, bool, error) {
	if store, ok := a.smsStore(); ok {
		return store.SMSNextSend(ctx, mobile)
	}
	flow := a.flow
	if flow == nil {
		return time.Time{}, false, nil
	}
	flow.smsMu.Lock()
	defer flow.smsMu.Unlock()
	nextSendAt, ok := flow.smsNextSend[mobile]
	if ok && time.Now().After(nextSendAt) {
		delete(flow.smsNextSend, mobile)
		return time.Time{}, false, nil
	}
	return nextSendAt, ok, nil
}

func (a authAPI) putSMSChallenge(ctx context.Context, mobile string, challenge smsChallenge) error {
	if store, ok := a.smsStore(); ok {
		ttl := time.Until(challenge.expiresAt)
		if ttl <= 0 {
			ttl = smsCodeTTL
		}
		return store.PutSMSChallenge(ctx, mobile, challenge, ttl)
	}
	flow := a.flow
	if flow == nil {
		return errAuthSessionUnavailable
	}
	flow.smsMu.Lock()
	defer flow.smsMu.Unlock()
	flow.smsChallenges[mobile] = challenge
	return nil
}

func (a authAPI) putSMSNextSend(ctx context.Context, mobile string, nextSendAt time.Time) error {
	if store, ok := a.smsStore(); ok {
		ttl := time.Until(nextSendAt)
		if ttl <= 0 {
			ttl = smsSendInterval
		}
		return store.PutSMSNextSend(ctx, mobile, nextSendAt, ttl)
	}
	flow := a.flow
	if flow == nil {
		return errAuthSessionUnavailable
	}
	flow.smsMu.Lock()
	defer flow.smsMu.Unlock()
	flow.smsNextSend[mobile] = nextSendAt
	return nil
}

func (a authAPI) getSMSChallenge(ctx context.Context, mobile string) (smsChallenge, bool, error) {
	if store, ok := a.smsStore(); ok {
		return store.SMSChallenge(ctx, mobile)
	}
	flow := a.flow
	if flow == nil {
		return smsChallenge{}, false, nil
	}
	flow.smsMu.Lock()
	defer flow.smsMu.Unlock()
	challenge, ok := flow.smsChallenges[mobile]
	return challenge, ok, nil
}

func (a authAPI) deleteSMSChallenge(ctx context.Context, mobile string) error {
	if store, ok := a.smsStore(); ok {
		return store.DeleteSMSChallenge(ctx, mobile)
	}
	flow := a.flow
	if flow == nil {
		return nil
	}
	flow.smsMu.Lock()
	defer flow.smsMu.Unlock()
	delete(flow.smsChallenges, mobile)
	return nil
}

func (a authAPI) smsSend(w http.ResponseWriter, r *http.Request) {
	var req smsSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthFlowError(w, http.StatusBadRequest, "INVALID_REQUEST", "请求参数不正确")
		return
	}
	mobile := normalizeMainlandMobile(req.Mobile)
	if !validMainlandMobile(mobile) {
		writeAuthFlowError(w, http.StatusBadRequest, "MOBILE_INVALID", "请输入正确的11位手机号")
		return
	}
	if limiter, ok := a.sessions.(smsRateLimitStore); ok {
		mobileDailyLimit, deviceDailyLimit, ipDailyLimit := a.config.SMSDailyLimits()
		namespace := strings.Trim(strings.TrimSpace(a.config.SMSRedisNamespace), ":")
		if namespace == "" {
			namespace = "zhiqiyun:development:sms"
		}
		dailyWindow := smsDailyWindow(time.Now())
		ipIdentity := smsRateIdentity(requestClientIP(r))
		deviceIdentity := smsRateIdentity(firstNonEmptyString(r.Header.Get("X-Device-Id"), r.UserAgent()))
		identities := []struct {
			key    string
			limit  int64
			window time.Duration
		}{
			{key: namespace + ":short:ip:" + ipIdentity, limit: 10, window: 10 * time.Minute},
			{key: namespace + ":short:device:" + deviceIdentity, limit: 10, window: 10 * time.Minute},
			{key: namespace + ":daily:mobile:" + smsRateIdentity(mobile), limit: mobileDailyLimit, window: dailyWindow},
			{key: namespace + ":daily:ip:" + ipIdentity, limit: ipDailyLimit, window: dailyWindow},
			{key: namespace + ":daily:device:" + deviceIdentity, limit: deviceDailyLimit, window: dailyWindow},
		}
		for _, identity := range identities {
			allowed, retryAfter, err := limiter.AllowSMSRequest(r.Context(), identity.key, identity.limit, identity.window)
			if err != nil {
				writeAuthFlowError(w, http.StatusServiceUnavailable, "SMS_STATE_UNAVAILABLE", "验证码服务暂时不可用")
				return
			}
			if !allowed {
				retry := int(retryAfter.Seconds()) + 1
				if retry < 1 {
					retry = 1
				}
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retry))
				writeAuthFlowError(w, http.StatusTooManyRequests, "SMS_RATE_LIMITED", "当前设备或网络请求过于频繁，请稍后再试")
				return
			}
		}
	}
	now := time.Now()
	nextSendAt, hasNextSend, err := a.smsNextSendAt(r.Context(), mobile)
	if err != nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "SMS_STATE_UNAVAILABLE", "验证码服务暂时不可用")
		return
	}
	if hasNextSend && nextSendAt.After(now) {
		retry := int(time.Until(nextSendAt).Seconds()) + 1
		w.Header().Set("Retry-After", fmt.Sprintf("%d", retry))
		writeAuthFlowError(w, http.StatusTooManyRequests, "SMS_TOO_FREQUENT", "发送过于频繁，请稍后再试")
		return
	}
	code := developmentSMSCode()
	if code == "" {
		var err error
		code, err = randomSMSCode()
		if err != nil {
			writeAuthFlowError(w, http.StatusInternalServerError, "SMS_CODE_GENERATE_FAILED", "验证码发送失败")
			return
		}
	}
	if err := sendSMSProvider(r.Context(), a.config, mobile, code); err != nil {
		writeMappedAuthFlowError(w, err)
		return
	}
	challenge := smsChallenge{
		codeHash: authCodeHash(mobile, code), expiresAt: now.Add(smsCodeTTL), nextSendAt: now.Add(smsSendInterval),
	}
	if err := a.putSMSChallenge(r.Context(), mobile, challenge); err != nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "SMS_STATE_UNAVAILABLE", "验证码服务暂时不可用")
		return
	}
	if err := a.putSMSNextSend(r.Context(), mobile, challenge.nextSendAt); err != nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "SMS_STATE_UNAVAILABLE", "验证码服务暂时不可用")
		return
	}
	writeJSON(w, map[string]any{"sent": true, "retryAfterSeconds": int(smsSendInterval.Seconds()), "expiresInSeconds": int(smsCodeTTL.Seconds())})
}

func requestClientIP(r *http.Request) string {
	for _, header := range []string{"CF-Connecting-IP", "X-Real-IP", "X-Forwarded-For"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if comma := strings.Index(value, ","); comma >= 0 {
			value = strings.TrimSpace(value[:comma])
		}
		if value != "" {
			return value
		}
	}
	host := strings.TrimSpace(r.RemoteAddr)
	if colon := strings.LastIndex(host, ":"); colon > 0 {
		host = host[:colon]
	}
	return host
}

func smsRateIdentity(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:12])
}

func smsDailyWindow(now time.Time) time.Duration {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("CST", 8*60*60)
	}
	localNow := now.In(location)
	nextDay := time.Date(localNow.Year(), localNow.Month(), localNow.Day()+1, 0, 0, 0, 0, location)
	window := nextDay.Sub(localNow)
	if window <= 0 {
		return 24 * time.Hour
	}
	return window
}

func (a authAPI) verifySMSCode(ctx context.Context, mobile, code string) error {
	challenge, ok, err := a.getSMSChallenge(ctx, mobile)
	if err != nil {
		return &authFlowError{status: http.StatusServiceUnavailable, code: "SMS_STATE_UNAVAILABLE", message: "验证码服务暂时不可用"}
	}
	if !ok || time.Now().After(challenge.expiresAt) {
		_ = a.deleteSMSChallenge(ctx, mobile)
		return &authFlowError{status: http.StatusUnauthorized, code: "SMS_CODE_EXPIRED", message: "验证码已过期，请重新获取"}
	}
	if challenge.attempts >= smsMaxAttempts {
		_ = a.deleteSMSChallenge(ctx, mobile)
		return &authFlowError{status: http.StatusTooManyRequests, code: "SMS_CODE_LOCKED", message: "验证码错误次数过多，请重新获取"}
	}
	wanted := challenge.codeHash
	got := authCodeHash(mobile, code)
	if subtle.ConstantTimeCompare(wanted[:], got[:]) != 1 {
		challenge.attempts++
		if err := a.putSMSChallenge(ctx, mobile, challenge); err != nil {
			return &authFlowError{status: http.StatusServiceUnavailable, code: "SMS_STATE_UNAVAILABLE", message: "验证码服务暂时不可用"}
		}
		return &authFlowError{status: http.StatusUnauthorized, code: "SMS_CODE_INVALID", message: "验证码错误，请重新输入"}
	}
	_ = a.deleteSMSChallenge(ctx, mobile)
	return nil
}

func (a authAPI) smsLogin(w http.ResponseWriter, r *http.Request) {
	var req smsLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthFlowError(w, http.StatusBadRequest, "INVALID_REQUEST", "请求参数不正确")
		return
	}
	mobile := normalizeMainlandMobile(req.Mobile)
	if !validMainlandMobile(mobile) {
		writeAuthFlowError(w, http.StatusBadRequest, "MOBILE_INVALID", "请输入正确的11位手机号")
		return
	}
	if err := a.verifySMSCode(r.Context(), mobile, req.SMSCode); err != nil {
		writeMappedAuthFlowError(w, err)
		return
	}
	input := authRegistrationInput{
		Context: r.Context(), InviteToken: req.InviteToken, InviteCode: req.InviteCode, Scene: req.Scene, PromoterCode: req.PromoterCode,
		CampaignCode: req.CampaignCode, RedirectSource: req.RedirectSource, IdempotencyKey: req.IdempotencyKey,
	}
	data, user, isNewUser, inviteStatus, err := a.userForPhoneIdentity(mobile, wechatMiniProgramSession{}, input)
	if err != nil {
		writeMappedAuthFlowError(w, err)
		return
	}
	response, err := a.authResponseWithToken(r.Context(), data, user)
	if err != nil {
		writeAuthTokenError(w, err)
		return
	}
	response["isNewUser"] = isNewUser
	response["registrationStatus"] = map[bool]string{true: "created", false: "existing"}[isNewUser]
	response["inviteBindStatus"] = inviteStatus
	response["expiresIn"] = int(authSessionTTL.Seconds())
	if isNewUser {
		response["newcomerBenefits"] = newcomerBenefitsForPlan(configuredNewcomerPlan(data.Plans))
	}
	writeAuthTokenResponse(w, r, response)
}

func (a authAPI) bindMobile(w http.ResponseWriter, r *http.Request) {
	var req mobileBindRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthFlowError(w, http.StatusBadRequest, "INVALID_REQUEST", "请求参数不正确")
		return
	}
	mobile := normalizeMainlandMobile(req.Mobile)
	if !validMainlandMobile(mobile) {
		writeAuthFlowError(w, http.StatusBadRequest, "MOBILE_INVALID", "请输入正确的11位手机号")
		return
	}
	if err := a.verifySMSCode(r.Context(), mobile, req.SMSCode); err != nil {
		writeMappedAuthFlowError(w, err)
		return
	}
	data, err := a.store.AdminData()
	if err != nil {
		writeMappedAuthFlowError(w, err)
		return
	}
	current, err := a.authenticatedUser(r, data)
	if err != nil {
		writeAuthFlowError(w, http.StatusUnauthorized, "UNAUTHORIZED", "登录状态已失效")
		return
	}
	if existing, ok := findUserByMobile(data.Users, mobile); ok && existing.ID != current.ID {
		writeAuthFlowError(w, http.StatusConflict, "AUTH_MOBILE_ALREADY_BOUND", "该手机号已绑定其他账号")
		return
	}
	updated, err := a.store.UpdateAdminCustomer(current.ID, adminCustomerMutation{Mobile: mobile})
	if err != nil {
		writeMappedAuthFlowError(w, err)
		return
	}
	updatedData := dataWithUpdatedUser(data, updated)
	writeJSON(w, map[string]any{
		"bound":    true,
		"user":     userView(updated),
		"auth":     authResponse(updatedData, updated, false),
		"security": securityPayload(updated),
	})
}

func registrationSource(input authRegistrationInput) map[string]string {
	return cloneStringMap(map[string]string{
		"scene": input.Scene, "promoterCode": input.PromoterCode, "campaignCode": input.CampaignCode,
		"redirectSource": input.RedirectSource,
	})
}

func (a authAPI) inviteBinding(data adminPlatformData, input authRegistrationInput) (string, string, error) {
	if strings.TrimSpace(input.InviteToken) != "" {
		ctx := input.Context
		if ctx == nil {
			ctx = context.Background()
		}
		invitation, err := resolvePromotionInvitation(ctx, a.store, data, input.InviteToken, "")
		if err != nil {
			return "", "invalid", &authFlowError{status: http.StatusBadRequest, code: "INVITE_TOKEN_INVALID", message: err.Error()}
		}
		return invitation.InviterUserID, "bound", nil
	}
	code := strings.ToUpper(strings.TrimSpace(input.InviteCode))
	if code == "" {
		return "", "not_provided", nil
	}
	if agent, ok := channelAgentForInviteCode(data.ChannelAgents, code); ok {
		return agent.UserID, "bound", nil
	}
	for _, center := range data.OperationCenters {
		if strings.EqualFold(strings.TrimSpace(center.InviteCode), code) && strings.EqualFold(center.Status, "ACTIVE") {
			return center.UserID, "bound", nil
		}
	}
	return "", "ignored_invalid", nil
}

func phoneSyntheticEmail(mobile string) string {
	sum := sha256.Sum256([]byte(normalizeMainlandMobile(mobile)))
	return fmt.Sprintf("phone_%x@mobile.local", sum[:12])
}

func findUserByMobile(users []adminUser, mobile string) (adminUser, bool) {
	mobile = normalizeMainlandMobile(mobile)
	for _, user := range users {
		if normalizeMainlandMobile(user.Mobile) == mobile && mobile != "" {
			return user, true
		}
	}
	return adminUser{}, false
}

func findUserByWechatIdentity(users []adminUser, session wechatMiniProgramSession) (adminUser, bool) {
	for _, user := range users {
		if session.UnionID != "" && strings.EqualFold(strings.TrimSpace(user.WeChatUnionID), strings.TrimSpace(session.UnionID)) {
			return user, true
		}
		for _, openID := range user.WeChatOpenIDs {
			if session.OpenID != "" && strings.EqualFold(strings.TrimSpace(openID), strings.TrimSpace(session.OpenID)) {
				return user, true
			}
		}
	}
	return adminUser{}, false
}

func activePhoneUser(user adminUser) error {
	switch strings.ToUpper(strings.TrimSpace(user.Status)) {
	case "ACTIVE":
		return nil
	case "FROZEN", "DISABLED", "SUSPENDED":
		return &authFlowError{status: http.StatusLocked, code: "ACCOUNT_FROZEN", message: "账号暂时无法使用"}
	case "DEACTIVATED", "DELETED", "CANCELLED":
		return &authFlowError{status: http.StatusGone, code: "ACCOUNT_DEACTIVATED", message: "账号已注销"}
	default:
		return &authFlowError{status: http.StatusUnauthorized, code: "ACCOUNT_UNAVAILABLE", message: "账号暂时无法登录"}
	}
}

func (a authAPI) userForPhoneIdentity(mobile string, session wechatMiniProgramSession, input authRegistrationInput) (adminPlatformData, adminUser, bool, string, error) {
	flow := a.flow
	if flow == nil {
		flow = newAuthFlowCoordinator()
	}
	flow.registrationMu.Lock()
	defer flow.registrationMu.Unlock()
	data, err := a.store.AdminData()
	if err != nil {
		return data, adminUser{}, false, "", err
	}
	phoneUser, hasPhoneUser := findUserByMobile(data.Users, mobile)
	wechatUser, hasWechatUser := findUserByWechatIdentity(data.Users, session)
	if hasPhoneUser && hasWechatUser && phoneUser.ID != wechatUser.ID {
		mergeRequest, mergeErr := a.store.CreateAdminAuthMergeRequest(adminAuthMergeRequestMutation{
			PrimaryUserID:   phoneUser.ID,
			SecondaryUserID: wechatUser.ID,
			Mobile:          mobile,
			WeChatOpenID:    session.OpenID,
			WeChatUnionID:   session.UnionID,
			ConflictCode:    "AUTH_ACCOUNT_MERGE_REQUIRED",
			Source:          "wechat_phone_login",
			Reason:          "手机号与微信小程序身份命中不同用户，需要人工确认后合并",
			Raw: map[string]any{
				"mobileMasked": maskedMobile(mobile),
				"scene":        input.Scene,
			},
		})
		if mergeErr != nil {
			return data, adminUser{}, false, "", mergeErr
		}
		return data, adminUser{}, false, "", &authFlowError{
			status:  http.StatusConflict,
			code:    "AUTH_ACCOUNT_MERGE_REQUIRED",
			message: "手机号与微信身份已关联不同账号，需要人工确认后合并",
			details: map[string]any{
				"accountConflict": true,
				"mergeRequestId":  mergeRequest.ID,
			},
		}
	}
	user := phoneUser
	found := hasPhoneUser
	if !found && hasWechatUser {
		user, found = wechatUser, true
	}
	if found {
		if err := activePhoneUser(user); err != nil {
			return data, adminUser{}, false, "", err
		}
		updated, updateErr := a.store.UpdateAdminCustomer(user.ID, adminCustomerMutation{
			Mobile: mobile, WeChatOpenID: session.OpenID, WeChatUnionID: session.UnionID,
		})
		if updateErr != nil {
			return data, adminUser{}, false, "", updateErr
		}
		status := "not_applicable"
		if strings.TrimSpace(input.InviteCode) != "" || strings.TrimSpace(input.InviteToken) != "" {
			status = "ignored_existing"
		}
		return dataWithUpdatedUser(data, updated), updated, false, status, nil
	}
	referredBy, inviteStatus, bindErr := a.inviteBinding(data, input)
	if bindErr != nil {
		return data, adminUser{}, false, "", bindErr
	}
	newcomerPlan := configuredNewcomerPlan(data.Plans)
	created, err := createRegisteredCustomer(a.store, adminCustomerMutation{
		Name: "用户 " + maskedMobile(mobile), Email: phoneSyntheticEmail(mobile), Mobile: mobile,
		WeChatOpenID: session.OpenID, WeChatUnionID: session.UnionID, RegistrationSource: registrationSource(input),
		Role: "MEMBER", Status: "ACTIVE", PlanID: newcomerPlan.ID, ReferredBy: referredBy,
		SubscriptionExpiresAt: newcomerPlanExpiresAt(newcomerPlan, time.Now()),
	}, planPoints(newcomerPlan))
	if err != nil {
		// A database-level mobile/UnionID unique constraint is the final guard when
		// multiple API instances race to register the same real-world identity.
		// If another request won, return that account instead of surfacing a false
		// registration failure or issuing newcomer benefits twice.
		if refreshed, refreshErr := a.store.AdminData(); refreshErr == nil {
			if existing, ok := findUserByMobile(refreshed.Users, mobile); ok {
				if statusErr := activePhoneUser(existing); statusErr != nil {
					return refreshed, adminUser{}, false, "", statusErr
				}
				updated, updateErr := a.store.UpdateAdminCustomer(existing.ID, adminCustomerMutation{
					Mobile: mobile, WeChatOpenID: session.OpenID, WeChatUnionID: session.UnionID,
				})
				if updateErr == nil {
					inviteStatus := "not_applicable"
					if strings.TrimSpace(input.InviteCode) != "" || strings.TrimSpace(input.InviteToken) != "" {
						inviteStatus = "ignored_existing"
					}
					return dataWithUpdatedUser(refreshed, updated), updated, false, inviteStatus, nil
				}
			}
		}
		return data, adminUser{}, false, "", err
	}
	return dataWithRegisteredUser(data, created), created, true, inviteStatus, nil
}

func findLoginUserByAccount(users []adminUser, account, password string) (adminUser, bool, bool, string) {
	account = strings.ToLower(strings.TrimSpace(account))
	mobile := normalizeMainlandMobile(account)
	password = strings.TrimSpace(password)
	for _, user := range users {
		matchesAccount := strings.EqualFold(strings.TrimSpace(user.Email), account) ||
			strings.EqualFold(strings.TrimSpace(user.Name), account) || strings.EqualFold(strings.TrimSpace(user.ID), account)
		if validMainlandMobile(mobile) && normalizeMainlandMobile(user.Mobile) == mobile {
			matchesAccount = true
		}
		if !matchesAccount {
			continue
		}
		if !strings.EqualFold(user.Status, "ACTIVE") {
			return adminUser{}, false, false, user.Status
		}
		matches, needsUpgrade := passwordMatches(user, password)
		if matches {
			return user, true, needsUpgrade, user.Status
		}
		return adminUser{}, false, false, user.Status
	}
	return adminUser{}, false, false, ""
}

func (a authAPI) resolveInvite(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(strings.TrimSpace(firstNonEmptyString(r.URL.Query().Get("inviteCode"), r.URL.Query().Get("invite_code"))))
	if code == "" {
		writeAuthFlowError(w, http.StatusBadRequest, "INVITE_CODE_REQUIRED", "请输入邀请码")
		return
	}
	data, err := a.store.AdminData()
	if err != nil {
		writeMappedAuthFlowError(w, err)
		return
	}
	for _, agent := range data.ChannelAgents {
		if !strings.EqualFold(strings.TrimSpace(agent.InviteCode), code) {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(agent.Status)) {
		case "ACTIVE":
			writeJSON(w, map[string]any{"valid": true, "status": "valid"})
		case "FROZEN", "SUSPENDED":
			writeJSON(w, map[string]any{"valid": false, "status": "agent_frozen", "message": "对应代理商暂不可用"})
		case "DISABLED":
			writeJSON(w, map[string]any{"valid": false, "status": "disabled", "message": "邀请码已停用"})
		default:
			writeJSON(w, map[string]any{"valid": false, "status": "invalid", "message": "邀请码无效"})
		}
		return
	}
	writeJSON(w, map[string]any{"valid": false, "status": "invalid", "message": "邀请码无效"})
}

func (a authAPI) security(w http.ResponseWriter, r *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeMappedAuthFlowError(w, err)
		return
	}
	user, err := a.authenticatedUser(r, data)
	if err != nil {
		writeAuthFlowError(w, http.StatusUnauthorized, "UNAUTHORIZED", "登录状态已失效")
		return
	}
	writeJSON(w, securityPayload(user))
}

func securityPayload(user adminUser) map[string]any {
	loginMethods := []string{}
	if strings.TrimSpace(user.Mobile) != "" {
		loginMethods = append(loginMethods, "mobile_sms")
	}
	if len(user.WeChatOpenIDs) > 0 || user.WeChatUnionID != "" {
		loginMethods = append(loginMethods, "wechat_mini_program")
	}
	if user.PasswordHash != "" {
		loginMethods = append(loginMethods, "password")
	}
	return map[string]any{
		"passwordSet":  user.PasswordHash != "",
		"mobileMasked": maskedMobile(user.Mobile),
		"mobileBound":  strings.TrimSpace(user.Mobile) != "",
		"wechatLinked": len(user.WeChatOpenIDs) > 0 || user.WeChatUnionID != "",
		"loginMethods": loginMethods,
		"status":       user.Status,
	}
}

func exchangeWeChatPhoneCode(ctx context.Context, phoneCode string) (string, error) {
	appID := strings.TrimSpace(os.Getenv("WECHAT_MINI_PROGRAM_APPID"))
	secret := strings.TrimSpace(os.Getenv("WECHAT_MINI_PROGRAM_SECRET"))
	if appID == "" || secret == "" {
		return "", errWeChatMiniProgramLoginNotConfigured
	}
	tokenURL := "https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=" + url.QueryEscape(appID) + "&secret=" + url.QueryEscape(secret)
	tokenReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	client := &http.Client{Timeout: 10 * time.Second}
	tokenResponse, err := client.Do(tokenReq)
	if err != nil {
		return "", err
	}
	defer tokenResponse.Body.Close()
	var tokenPayload struct {
		AccessToken string `json:"access_token"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.NewDecoder(tokenResponse.Body).Decode(&tokenPayload); err != nil || tokenPayload.AccessToken == "" || tokenPayload.ErrCode != 0 {
		return "", errors.New("wechat access token unavailable")
	}
	body, _ := json.Marshal(map[string]string{"code": strings.TrimSpace(phoneCode)})
	phoneURL := "https://api.weixin.qq.com/wxa/business/getuserphonenumber?access_token=" + url.QueryEscape(tokenPayload.AccessToken)
	phoneReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, phoneURL, bytes.NewReader(body))
	phoneReq.Header.Set("Content-Type", "application/json")
	phoneResponse, err := client.Do(phoneReq)
	if err != nil {
		return "", err
	}
	defer phoneResponse.Body.Close()
	var phonePayload struct {
		ErrCode   int    `json:"errcode"`
		ErrMsg    string `json:"errmsg"`
		PhoneInfo struct {
			PhoneNumber     string `json:"phoneNumber"`
			PurePhoneNumber string `json:"purePhoneNumber"`
			CountryCode     string `json:"countryCode"`
		} `json:"phone_info"`
	}
	if err := json.NewDecoder(phoneResponse.Body).Decode(&phonePayload); err != nil || phonePayload.ErrCode != 0 {
		return "", errors.New("wechat phone code exchange failed")
	}
	mobile := normalizeMainlandMobile(firstNonEmptyString(phonePayload.PhoneInfo.PurePhoneNumber, phonePayload.PhoneInfo.PhoneNumber))
	if !validMainlandMobile(mobile) {
		return "", errors.New("wechat phone response invalid")
	}
	return mobile, nil
}
