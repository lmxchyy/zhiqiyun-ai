package httpserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/skip2/go-qrcode"

	"xianzhi-ai/backend-go/internal/config"
)

var (
	errInviteUnavailable       = errors.New("invite code is invalid or unavailable")
	errInviteExistingUnbound   = errors.New("existing user requires manual agent binding")
	errInviteAlreadyBoundOther = errors.New("user is already bound to another agent")
	errAppReleaseUnavailable   = errors.New("published app release is unavailable")
)

type agentInviteInfo struct {
	AgentID         string
	InviterUserID   string
	TenantID        string
	InviteCode      string
	DisplayName     string
	AgentStatus     string
	ActivityIntro   string
	RegistrationOK  bool
	OperationCenter string
}

type appRelease struct {
	ID                      string `json:"id"`
	Platform                string `json:"platform"`
	Channel                 string `json:"channel"`
	VersionName             string `json:"versionName"`
	VersionCode             int64  `json:"versionCode"`
	APKURL                  string `json:"apkUrl"`
	FileSize                int64  `json:"fileSize"`
	SHA256                  string `json:"sha256"`
	ReleaseNotes            string `json:"releaseNotes"`
	MinSupportedVersionCode int64  `json:"minSupportedVersionCode"`
	ForceUpdate             bool   `json:"forceUpdate"`
	Status                  string `json:"status"`
	PublishedAt             string `json:"publishedAt"`
	CreatedAt               string `json:"createdAt"`
}

type agentInviteRegistrationInput struct {
	Mobile             string
	IdempotencyKeyHash string
	RegistrationEvent  string
	Source             string
	ClientFamily       string
	PlanID             string
	PlanPoints         int
	SubscriptionExpiry string
}

type agentInviteRegistrationResult struct {
	UserID              string
	Invite              agentInviteInfo
	RegistrationEventID string
	RegistrationStatus  string
	RelationshipStatus  string
	Created             bool
}

type agentInviteFunnel struct {
	PageViews   int64 `json:"pageViews"`
	Registered  int64 `json:"registered"`
	Downloads   int64 `json:"downloads"`
	Activations int64 `json:"activations"`
}

type agentInviteDataStore interface {
	ResolveAgentInvite(context.Context, string) (agentInviteInfo, error)
	FindAgentInviteRegistration(context.Context, string) (agentInviteRegistrationResult, bool, error)
	RegisterAgentInvite(context.Context, agentInviteInfo, agentInviteRegistrationInput) (agentInviteRegistrationResult, error)
	RecordAgentInviteEvent(context.Context, agentInviteInfo, string, string, string) error
	LatestAppRelease(context.Context, string, string) (appRelease, error)
	RecordAPKDownload(context.Context, appRelease, string, string) error
	AgentInviteProfile(context.Context, string) (agentInviteInfo, agentInviteFunnel, error)
	SaveAgentInviteLanding(context.Context, agentInviteInfo, string) error
	RecordAppActivation(context.Context, string, string, string) error
}

type agentInviteAPI struct {
	store               agentInviteDataStore
	loadData            func() (adminPlatformData, error)
	auth                authAPI
	registrationEnabled bool
	downloadEnabled     bool
	activationEnabled   bool
}

func newAgentInviteAPI(store platformStore, sessions authSessionStore, configs ...config.Config) *agentInviteAPI {
	cfg := config.Load()
	if len(configs) > 0 {
		cfg = configs[0]
	}
	api := &agentInviteAPI{
		loadData: store.AdminData, auth: newAuthAPI(store, sessions, cfg),
		registrationEnabled: cfg.AgentInviteRegistrationEnabled,
		downloadEnabled:     cfg.APKDownloadEnabled,
		activationEnabled:   cfg.AppActivationReportEnabled,
	}
	if inviteStore, ok := store.(agentInviteDataStore); ok {
		api.store = inviteStore
	}
	return api
}

func (a *agentInviteAPI) invite(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "INVITE_SERVICE_UNAVAILABLE", "邀请注册服务暂不可用")
		return
	}
	code := inviteCodeFromRequest(r)
	item, err := a.store.ResolveAgentInvite(r.Context(), code)
	if err != nil {
		status := http.StatusNotFound
		if code == "" {
			status = http.StatusBadRequest
		}
		writeAuthFlowError(w, status, "INVITE_CODE_INVALID", "邀请码无效、已停用或已失效")
		return
	}
	if item.RegistrationOK {
		visitorKey := firstNonEmptyString(r.Header.Get("X-Device-Id"), r.UserAgent(), requestClientIP(r))
		requestHash := stableSecretHash(item.AgentID + "|" + visitorKey + "|" + time.Now().UTC().Format("2006-01-02"))
		_ = a.store.RecordAgentInviteEvent(r.Context(), item, "page_view", clientFamily(r), requestHash)
	}
	writeJSON(w, map[string]any{
		"valid": item.RegistrationOK, "inviteCode": item.InviteCode, "agentDisplayName": item.DisplayName,
		"agentStatus": strings.ToLower(item.AgentStatus), "activityIntro": item.ActivityIntro,
		"registrationAllowed": item.RegistrationOK && a.registrationEnabled,
	})
}

type publicInviteRegisterRequest struct {
	Mobile              string `json:"mobile"`
	SMSCode             string `json:"sms_code"`
	SMSCodeCamel        string `json:"smsCode"`
	AgreementAccepted   bool   `json:"agreement_accepted"`
	AgreementCamel      bool   `json:"agreementAccepted"`
	PrivacyAccepted     bool   `json:"privacy_accepted"`
	PrivacyCamel        bool   `json:"privacyAccepted"`
	IdempotencyKey      string `json:"idempotency_key"`
	IdempotencyKeyCamel string `json:"idempotencyKey"`
}

func (a *agentInviteAPI) register(w http.ResponseWriter, r *http.Request) {
	if !a.registrationEnabled {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "AGENT_INVITE_REGISTRATION_DISABLED", "代理商邀请注册正在维护，请稍后再试")
		return
	}
	if a.store == nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "INVITE_SERVICE_UNAVAILABLE", "邀请注册服务暂不可用")
		return
	}
	var req publicInviteRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthFlowError(w, http.StatusBadRequest, "INVALID_REQUEST", "请求参数不正确")
		return
	}
	if !(req.AgreementAccepted || req.AgreementCamel) || !(req.PrivacyAccepted || req.PrivacyCamel) {
		writeAuthFlowError(w, http.StatusBadRequest, "LEGAL_ACCEPTANCE_REQUIRED", "请先阅读并同意用户协议和隐私政策")
		return
	}
	mobile := normalizeMainlandMobile(req.Mobile)
	if !validMainlandMobile(mobile) {
		writeAuthFlowError(w, http.StatusBadRequest, "MOBILE_INVALID", "请输入正确的11位手机号")
		return
	}
	idempotencyKey := strings.TrimSpace(firstNonEmptyString(r.Header.Get("Idempotency-Key"), req.IdempotencyKey, req.IdempotencyKeyCamel))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 160 {
		writeAuthFlowError(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "缺少有效的幂等请求标识")
		return
	}
	idempotencyHash := stableSecretHash(inviteCodeFromRequest(r) + "|" + mobile + "|" + idempotencyKey)
	if existing, ok, err := a.store.FindAgentInviteRegistration(r.Context(), idempotencyHash); err == nil && ok {
		a.writeRegistrationResult(w, existing, true)
		return
	} else if err != nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "INVITE_STATE_UNAVAILABLE", "注册状态暂不可用")
		return
	}
	invite, err := a.store.ResolveAgentInvite(r.Context(), inviteCodeFromRequest(r))
	if err != nil || !invite.RegistrationOK {
		writeAuthFlowError(w, http.StatusConflict, "INVITE_CODE_UNAVAILABLE", "该代理商当前无法继续邀请新用户")
		return
	}
	smsCode := firstNonEmptyString(req.SMSCode, req.SMSCodeCamel)
	if err := a.auth.verifySMSCode(r.Context(), mobile, smsCode); err != nil {
		writeMappedAuthFlowError(w, err)
		return
	}
	data, err := a.loadData()
	if err != nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "REGISTRATION_UNAVAILABLE", "注册服务暂不可用")
		return
	}
	plan := configuredNewcomerPlan(data.Plans)
	result, err := a.store.RegisterAgentInvite(r.Context(), invite, agentInviteRegistrationInput{
		Mobile: mobile, IdempotencyKeyHash: idempotencyHash, RegistrationEvent: "invite_registered_" + randomOpaqueID(),
		Source: "agent_h5", ClientFamily: clientFamily(r), PlanID: plan.ID, PlanPoints: planPoints(plan),
		SubscriptionExpiry: newcomerPlanExpiresAt(plan, time.Now()),
	})
	if err != nil {
		switch {
		case errors.Is(err, errInviteExistingUnbound):
			writeAuthFlowError(w, http.StatusConflict, "EXISTING_USER_BINDING_REQUIRED", "该手机号已注册但尚未绑定代理商，请申请绑定或联系平台处理")
		case errors.Is(err, errInviteAlreadyBoundOther):
			writeAuthFlowError(w, http.StatusConflict, "AGENT_RELATION_LOCKED", "该账号已绑定其他代理商，原代理关系不会被覆盖")
		case errors.Is(err, errInviteUnavailable):
			writeAuthFlowError(w, http.StatusConflict, "INVITE_CODE_UNAVAILABLE", "该代理商当前无法继续邀请新用户")
		default:
			writeAuthFlowError(w, http.StatusInternalServerError, "INVITE_REGISTRATION_FAILED", "注册失败，请稍后重试")
		}
		return
	}
	a.writeRegistrationResult(w, result, false)
}

func (a *agentInviteAPI) writeRegistrationResult(w http.ResponseWriter, result agentInviteRegistrationResult, replay bool) {
	downloadPath := "/api/v1/public/app/releases/android/latest/download"
	if result.RegistrationEventID != "" {
		downloadPath += "?registration=" + url.QueryEscape(result.RegistrationEventID)
	}
	writeJSON(w, map[string]any{
		"registered": true, "idempotentReplay": replay, "registrationStatus": result.RegistrationStatus,
		"relationshipStatus": result.RelationshipStatus, "agentDisplayName": result.Invite.DisplayName,
		"downloadPage": map[string]any{
			"platform": "android", "channel": "official", "downloadUrl": downloadPath,
			"latestReleaseUrl": "/api/v1/public/app/releases/latest?platform=android&channel=official",
		},
	})
}

func (a *agentInviteAPI) latestRelease(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "RELEASE_SERVICE_UNAVAILABLE", "版本服务暂不可用")
		return
	}
	platform := strings.ToLower(strings.TrimSpace(firstNonEmptyString(r.URL.Query().Get("platform"), "android")))
	channel := strings.ToLower(strings.TrimSpace(firstNonEmptyString(r.URL.Query().Get("channel"), "official")))
	release, err := a.store.LatestAppRelease(r.Context(), platform, channel)
	if err != nil {
		writeAuthFlowError(w, http.StatusNotFound, "APP_RELEASE_NOT_FOUND", "暂未发布可下载版本")
		return
	}
	writeJSON(w, release)
}

func (a *agentInviteAPI) download(w http.ResponseWriter, r *http.Request) {
	if !a.downloadEnabled {
		w.Header().Set("Retry-After", "300")
		writeAuthFlowError(w, http.StatusServiceUnavailable, "APK_DOWNLOAD_DISABLED", "安卓安装包下载正在维护，请稍后再试")
		return
	}
	if a.store == nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "RELEASE_SERVICE_UNAVAILABLE", "版本服务暂不可用")
		return
	}
	release, err := a.store.LatestAppRelease(r.Context(), "android", "official")
	if err != nil {
		writeAuthFlowError(w, http.StatusNotFound, "APP_RELEASE_NOT_FOUND", "暂未发布可下载版本")
		return
	}
	if err := validateAppRelease(release); err != nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "APP_RELEASE_INVALID", "正式版本记录不完整")
		return
	}
	target, err := url.Parse(strings.TrimSpace(release.APKURL))
	if err != nil || (target.Scheme != "https" && !(target.Scheme == "http" && !authProductionEnvironment())) || target.Host == "" {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "APP_RELEASE_URL_INVALID", "下载地址配置无效")
		return
	}
	registrationEventID := strings.TrimSpace(r.URL.Query().Get("registration"))
	if err := a.store.RecordAPKDownload(r.Context(), release, registrationEventID, clientFamily(r)); err != nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "DOWNLOAD_EVENT_FAILED", "下载服务暂不可用")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func (a *agentInviteAPI) profile(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "INVITE_SERVICE_UNAVAILABLE", "邀请服务暂不可用")
		return
	}
	userID, err := authenticatedUserID(r, a.auth.sessions)
	if err != nil {
		writeAuthFlowError(w, http.StatusUnauthorized, "UNAUTHORIZED", "登录状态已失效")
		return
	}
	invite, funnel, err := a.store.AgentInviteProfile(r.Context(), userID)
	if err != nil {
		writeAuthFlowError(w, http.StatusForbidden, "AGENT_INVITE_FORBIDDEN", "当前账号没有可用的代理商邀请权限")
		return
	}
	link := agentInviteLandingURL(invite.InviteCode)
	writeJSON(w, map[string]any{
		"inviteCode": invite.InviteCode, "inviteLink": link, "agentDisplayName": invite.DisplayName,
		"agentStatus": strings.ToLower(invite.AgentStatus), "funnel": funnel,
	})
}

func (a *agentInviteAPI) poster(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "INVITE_SERVICE_UNAVAILABLE", "邀请服务暂不可用")
		return
	}
	userID, err := authenticatedUserID(r, a.auth.sessions)
	if err != nil {
		writeAuthFlowError(w, http.StatusUnauthorized, "UNAUTHORIZED", "登录状态已失效")
		return
	}
	invite, funnel, err := a.store.AgentInviteProfile(r.Context(), userID)
	if err != nil {
		writeAuthFlowError(w, http.StatusForbidden, "AGENT_INVITE_FORBIDDEN", "当前账号没有可用的代理商邀请权限")
		return
	}
	link := agentInviteLandingURL(invite.InviteCode)
	png, err := qrcode.Encode(link, qrcode.Medium, 768)
	if err != nil {
		writeAuthFlowError(w, http.StatusInternalServerError, "POSTER_QR_FAILED", "邀请二维码生成失败")
		return
	}
	if err := a.store.SaveAgentInviteLanding(r.Context(), invite, link); err != nil {
		writeAuthFlowError(w, http.StatusInternalServerError, "POSTER_SAVE_FAILED", "邀请海报信息保存失败")
		return
	}
	writeJSON(w, map[string]any{
		"inviteCode": invite.InviteCode, "inviteLink": link,
		"qrCodeDataUrl": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		"poster": map[string]any{
			"title": "知启云AI", "subtitle": invite.DisplayName + " 邀请您体验企业级AI创作平台",
			"width": 1080, "height": 1440, "format": "png",
		},
		"funnel": funnel,
	})
}

func (a *agentInviteAPI) activation(w http.ResponseWriter, r *http.Request) {
	if !a.activationEnabled {
		writeJSON(w, map[string]any{"activated": false, "reportingEnabled": false})
		return
	}
	if a.store == nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "INVITE_SERVICE_UNAVAILABLE", "激活记录服务暂不可用")
		return
	}
	userID, err := authenticatedUserID(r, a.auth.sessions)
	if err != nil {
		writeAuthFlowError(w, http.StatusUnauthorized, "UNAUTHORIZED", "登录状态已失效")
		return
	}
	var req struct {
		InstallationID string `json:"installationId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(strings.TrimSpace(req.InstallationID)) < 12 {
		writeAuthFlowError(w, http.StatusBadRequest, "INSTALLATION_ID_INVALID", "设备安装标识无效")
		return
	}
	requestHash := stableSecretHash(userID + "|" + strings.TrimSpace(req.InstallationID))
	if err := a.store.RecordAppActivation(r.Context(), userID, requestHash, clientFamily(r)); err != nil {
		writeAuthFlowError(w, http.StatusInternalServerError, "APP_ACTIVATION_FAILED", "激活状态记录失败")
		return
	}
	writeJSON(w, map[string]any{"activated": true})
}

func inviteCodeFromRequest(r *http.Request) string {
	return strings.ToUpper(strings.TrimSpace(firstNonEmptyString(r.PathValue("inviteCode"), r.URL.Query().Get("inviteCode"), r.URL.Query().Get("invite_code"))))
}

func agentInviteLandingURL(code string) string {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("AGENT_INVITE_LANDING_BASE_URL")), "/")
	if base == "" {
		base = "https://ai.zs-kjhn.cn"
	}
	return base + "/d/" + url.PathEscape(strings.ToUpper(strings.TrimSpace(code)))
}

func clientFamily(r *http.Request) string {
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	switch {
	case strings.Contains(ua, "micromessenger"):
		return "wechat"
	case strings.Contains(ua, "android"):
		return "android"
	case strings.Contains(ua, "iphone"), strings.Contains(ua, "ipad"), strings.Contains(ua, "ios"):
		return "ios"
	default:
		return "web"
	}
}

func stableSecretHash(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func sanitizeAgentInviteDisplayName(value string) string {
	value = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value))
	if value == "" {
		return "知启云AI合作代理商"
	}
	runes := []rune(value)
	if len(runes) > 40 {
		value = string(runes[:40])
	}
	return value
}

func randomOpaqueID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func secureAgentInviteCode() string {
	value := randomOpaqueID()
	if len(value) < 12 {
		value = stableSecretHash(value)
	}
	return strings.ToUpper(value[:12])
}

func (s *postgresStore) ResolveAgentInvite(ctx context.Context, code string) (agentInviteInfo, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return agentInviteInfo{}, errInviteUnavailable
	}
	var item agentInviteInfo
	err := s.db.QueryRowContext(ctx, `
		SELECT agent.id, agent.user_id, coalesce(nullif(users.raw->>'tenantId', ''), 'tenant_default'),
		       upper(btrim(agent.invite_code)), coalesce(nullif(btrim(agent.raw->>'inviteDisplayName'), ''), '知启云AI合作代理商'),
		       upper(coalesce(agent.status, '')), coalesce(codes.activity_intro, ''),
		       coalesce(agent.operation_center_id, '')
		FROM xz_channel_agents agent
		JOIN xz_users users ON users.id=agent.user_id
		LEFT JOIN xz_marketing_invite_codes codes ON codes.agent_id=agent.id
		WHERE upper(btrim(agent.invite_code))=$1
		  AND upper(coalesce(users.status, ''))='ACTIVE'
		LIMIT 1
	`, code).Scan(&item.AgentID, &item.InviterUserID, &item.TenantID, &item.InviteCode, &item.DisplayName, &item.AgentStatus, &item.ActivityIntro, &item.OperationCenter)
	if err != nil {
		return agentInviteInfo{}, errInviteUnavailable
	}
	item.DisplayName = sanitizeAgentInviteDisplayName(item.DisplayName)
	item.RegistrationOK = item.AgentStatus == "ACTIVE"
	if item.ActivityIntro == "" {
		item.ActivityIntro = "注册知启云AI，体验企业级AI创作、智能体、图片、视频与PPT能力"
	}
	return item, nil
}

func (s *postgresStore) FindAgentInviteRegistration(ctx context.Context, keyHash string) (agentInviteRegistrationResult, bool, error) {
	var result agentInviteRegistrationResult
	err := s.db.QueryRowContext(ctx, `
		SELECT event.user_id, event.id, agent.id, agent.user_id,
		       coalesce(nullif(users.raw->>'tenantId', ''), 'tenant_default'), upper(btrim(agent.invite_code)),
		       coalesce(nullif(btrim(agent.raw->>'inviteDisplayName'), ''), '知启云AI合作代理商'), upper(coalesce(agent.status, '')),
		       coalesce(codes.activity_intro, ''), coalesce(agent.operation_center_id, '')
		FROM xz_agent_invite_events event
		JOIN xz_channel_agents agent ON agent.id=event.agent_id
		JOIN xz_users users ON users.id=event.user_id
		LEFT JOIN xz_marketing_invite_codes codes ON codes.agent_id=agent.id
		WHERE event.event_type='registered' AND event.request_key_hash=$1
		LIMIT 1
	`, keyHash).Scan(
		&result.UserID, &result.RegistrationEventID, &result.Invite.AgentID, &result.Invite.InviterUserID,
		&result.Invite.TenantID, &result.Invite.InviteCode, &result.Invite.DisplayName, &result.Invite.AgentStatus,
		&result.Invite.ActivityIntro, &result.Invite.OperationCenter,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return agentInviteRegistrationResult{}, false, nil
	}
	if err != nil {
		return agentInviteRegistrationResult{}, false, err
	}
	result.Invite.DisplayName = sanitizeAgentInviteDisplayName(result.Invite.DisplayName)
	result.Invite.RegistrationOK = true
	result.RegistrationStatus = "created"
	result.RelationshipStatus = "locked"
	return result, true, nil
}

func (s *postgresStore) RegisterAgentInvite(ctx context.Context, invite agentInviteInfo, input agentInviteRegistrationInput) (agentInviteRegistrationResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return agentInviteRegistrationResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, normalizeMainlandMobile(input.Mobile)); err != nil {
		return agentInviteRegistrationResult{}, err
	}
	var locked agentInviteInfo
	err = tx.QueryRowContext(ctx, `
		SELECT agent.id, agent.user_id, coalesce(nullif(users.raw->>'tenantId', ''), 'tenant_default'),
		       upper(btrim(agent.invite_code)), coalesce(nullif(btrim(agent.raw->>'inviteDisplayName'), ''), '知启云AI合作代理商'),
		       upper(coalesce(agent.status, '')), coalesce(agent.operation_center_id, '')
		FROM xz_channel_agents agent
		JOIN xz_users users ON users.id=agent.user_id
		WHERE agent.id=$1 AND upper(btrim(agent.invite_code))=$2
		  AND upper(coalesce(agent.status, ''))='ACTIVE'
		  AND upper(coalesce(users.status, ''))='ACTIVE'
		FOR UPDATE OF agent
	`, invite.AgentID, invite.InviteCode).Scan(
		&locked.AgentID, &locked.InviterUserID, &locked.TenantID, &locked.InviteCode,
		&locked.DisplayName, &locked.AgentStatus, &locked.OperationCenter,
	)
	if err != nil {
		return agentInviteRegistrationResult{}, errInviteUnavailable
	}
	locked.DisplayName = sanitizeAgentInviteDisplayName(locked.DisplayName)
	locked.ActivityIntro = invite.ActivityIntro
	locked.RegistrationOK = true

	var user adminUser
	err = tx.QueryRowContext(ctx, `SELECT raw FROM xz_users WHERE mobile=$1 FOR UPDATE`, normalizeMainlandMobile(input.Mobile)).Scan(rawScanner(&user))
	if err == nil {
		var existingAgentID string
		relationErr := tx.QueryRowContext(ctx, `
			SELECT coalesce(parent_agent_id, '') FROM xz_user_relationships
			WHERE tenant_id=$1 AND user_id=$2 AND status='ACTIVE' AND ended_at IS NULL
			LIMIT 1
		`, locked.TenantID, user.ID).Scan(&existingAgentID)
		switch {
		case relationErr == nil && existingAgentID == locked.AgentID:
			eventID := input.RegistrationEvent
			if _, insertErr := tx.ExecContext(ctx, `
				INSERT INTO xz_agent_invite_events(
				  id, tenant_id, inviter_user_id, agent_id, invite_code, user_id, event_type,
				  source, request_key_hash, client_family, metadata
				) VALUES($1,$2,$3,$4,$5,$6,'registered',$7,$8,$9,'{"existingRelation":true}'::jsonb)
				ON CONFLICT DO NOTHING
			`, eventID, locked.TenantID, locked.InviterUserID, locked.AgentID, locked.InviteCode,
				user.ID, input.Source, input.IdempotencyKeyHash, input.ClientFamily); insertErr != nil {
				return agentInviteRegistrationResult{}, insertErr
			}
			if lookupErr := tx.QueryRowContext(ctx, `
				SELECT id FROM xz_agent_invite_events
				WHERE event_type='registered' AND request_key_hash=$1 LIMIT 1
			`, input.IdempotencyKeyHash).Scan(&eventID); lookupErr != nil {
				return agentInviteRegistrationResult{}, lookupErr
			}
			return agentInviteRegistrationResult{
				UserID: user.ID, Invite: locked, RegistrationEventID: eventID,
				RegistrationStatus: "existing", RelationshipStatus: "locked", Created: false,
			}, tx.Commit()
		case relationErr == nil:
			return agentInviteRegistrationResult{}, errInviteAlreadyBoundOther
		case !errors.Is(relationErr, sql.ErrNoRows):
			return agentInviteRegistrationResult{}, relationErr
		default:
			return agentInviteRegistrationResult{}, errInviteExistingUnbound
		}
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return agentInviteRegistrationResult{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	userID := "user_inv_" + randomOpaqueID()
	user = adminUser{
		ID: userID, Email: phoneSyntheticEmail(input.Mobile), Mobile: normalizeMainlandMobile(input.Mobile),
		Name: "用户 " + maskedMobile(input.Mobile), Role: "MEMBER", Status: "ACTIVE",
		PlanID: fallback(input.PlanID, "plan_free"), ReferredBy: locked.InviterUserID,
		RegistrationSource:    map[string]string{"scene": "agent_invite_h5", "inviteCode": locked.InviteCode},
		SubscriptionExpiresAt: input.SubscriptionExpiry, CreatedAt: now, UpdatedAt: now,
	}
	if err := insertUser(ctx, tx, user); err != nil {
		return agentInviteRegistrationResult{}, err
	}
	if err := upsertPointAccountByUser(ctx, tx, user.ID, input.PlanPoints); err != nil {
		return agentInviteRegistrationResult{}, err
	}
	relationID := "user_relationship_invite_" + shortStableHash(user.ID+"|"+locked.AgentID, 20)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_user_relationships(
		  id, tenant_id, user_id, parent_agent_id, operation_center_id,
		  effective_at, status, source_type, created_by
		) VALUES($1,$2,$3,$4,nullif($5,''),now(),'ACTIVE','AGENT_INVITE','agent_invite_api')
	`, relationID, locked.TenantID, user.ID, locked.AgentID, locked.OperationCenter); err != nil {
		return agentInviteRegistrationResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_user_business_identities(
		  id, tenant_id, user_id, identity_type, identity_status, commission_enabled,
		  source_type, effective_at, created_by
		) VALUES($1,$2,$3,'USER','ACTIVE',FALSE,'AGENT_INVITE',now(),'agent_invite_api')
		ON CONFLICT DO NOTHING
	`, "business_identity_user_"+shortStableHash(user.ID, 20), locked.TenantID, user.ID); err != nil {
		return agentInviteRegistrationResult{}, err
	}
	masked := maskedMobile(input.Mobile)
	recordID := promotionBoundRecordID(locked.InviterUserID, user.ID)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_marketing_invite_records(
		  id, tenant_id, inviter_user_id, invitee_user_id, visitor_id, visitor_name, masked_mobile,
		  invite_code, source, register_status, recharge_status, upgrade_status, status,
		  template_id, visit_time, register_time, metadata, created_at, updated_at
		) VALUES($1,$2,$3,$4,$4,$5,$6,$7,$8,'REGISTERED','PENDING','PENDING','registered',
		         'poster.brand.simple',now(),now(),'{}'::jsonb,now(),now())
		ON CONFLICT (id) DO UPDATE SET invitee_user_id=excluded.invitee_user_id,
		  register_status='REGISTERED', status='registered', register_time=coalesce(xz_marketing_invite_records.register_time, now()),
		  updated_at=now()
	`, recordID, locked.TenantID, locked.InviterUserID, user.ID, user.Name, masked, locked.InviteCode, input.Source); err != nil {
		return agentInviteRegistrationResult{}, err
	}
	for _, eventType := range []string{"sms_verified", "registered"} {
		eventID := "invite_event_" + randomOpaqueID()
		requestHash := ""
		if eventType == "registered" {
			eventID = input.RegistrationEvent
			requestHash = input.IdempotencyKeyHash
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO xz_agent_invite_events(
			  id, tenant_id, inviter_user_id, agent_id, invite_code, user_id, event_type,
			  source, request_key_hash, client_family, metadata
			) VALUES($1,$2,$3,$4,$5,$6,$7,$8,nullif($9,''),$10,'{}'::jsonb)
			ON CONFLICT DO NOTHING
		`, eventID, locked.TenantID, locked.InviterUserID, locked.AgentID, locked.InviteCode, user.ID,
			eventType, input.Source, requestHash, input.ClientFamily); err != nil {
			return agentInviteRegistrationResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return agentInviteRegistrationResult{}, err
	}
	return agentInviteRegistrationResult{
		UserID: user.ID, Invite: locked, RegistrationEventID: input.RegistrationEvent,
		RegistrationStatus: "created", RelationshipStatus: "locked", Created: true,
	}, nil
}

func (s *postgresStore) RecordAgentInviteEvent(ctx context.Context, invite agentInviteInfo, eventType, family, requestKeyHash string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO xz_agent_invite_events(
		  id, tenant_id, inviter_user_id, agent_id, invite_code, event_type,
		  source, request_key_hash, client_family, metadata
		) VALUES($1,$2,$3,$4,$5,$6,'agent_h5',nullif($7,''),$8,'{}'::jsonb)
		ON CONFLICT DO NOTHING
	`, "invite_event_"+randomOpaqueID(), invite.TenantID, invite.InviterUserID, invite.AgentID,
		invite.InviteCode, eventType, requestKeyHash, family)
	return err
}

func (s *postgresStore) LatestAppRelease(ctx context.Context, platform, channel string) (appRelease, error) {
	var item appRelease
	var publishedAt, createdAt time.Time
	err := s.db.QueryRowContext(ctx, `
		SELECT id, platform, channel, version_name, version_code, apk_url, file_size, sha256,
		       release_notes, min_supported_version_code, force_update, status, published_at, created_at
		FROM xz_app_releases
		WHERE platform=$1 AND channel=$2 AND status='published'
		ORDER BY published_at DESC, version_code DESC LIMIT 1
	`, strings.ToLower(strings.TrimSpace(platform)), strings.ToLower(strings.TrimSpace(channel))).Scan(
		&item.ID, &item.Platform, &item.Channel, &item.VersionName, &item.VersionCode,
		&item.APKURL, &item.FileSize, &item.SHA256, &item.ReleaseNotes,
		&item.MinSupportedVersionCode, &item.ForceUpdate, &item.Status, &publishedAt, &createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return appRelease{}, errAppReleaseUnavailable
	}
	if err != nil {
		return appRelease{}, err
	}
	item.PublishedAt = publishedAt.UTC().Format(time.RFC3339)
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	return item, nil
}

func (s *postgresStore) RecordAPKDownload(ctx context.Context, release appRelease, registrationEventID, family string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var agentID, userID string
	if registrationEventID != "" {
		if err := tx.QueryRowContext(ctx, `
			SELECT agent_id, coalesce(user_id, '') FROM xz_agent_invite_events
			WHERE id=$1 AND event_type='registered'
		`, registrationEventID).Scan(&agentID, &userID); err != nil {
			registrationEventID = ""
			agentID = ""
			userID = ""
		}
	}
	downloadID := "apk_download_" + randomOpaqueID()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_apk_download_events(id, release_id, agent_id, user_id, invite_event_id, channel, client_family)
		VALUES($1,$2,nullif($3,''),nullif($4,''),nullif($5,''),$6,$7)
	`, downloadID, release.ID, agentID, userID, registrationEventID, release.Channel, family); err != nil {
		return err
	}
	if agentID != "" {
		var invite agentInviteInfo
		if err := tx.QueryRowContext(ctx, `
			SELECT agent.user_id, coalesce(nullif(users.raw->>'tenantId', ''), 'tenant_default'), upper(btrim(agent.invite_code))
			FROM xz_channel_agents agent JOIN xz_users users ON users.id=agent.user_id WHERE agent.id=$1
		`, agentID).Scan(&invite.InviterUserID, &invite.TenantID, &invite.InviteCode); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO xz_agent_invite_events(
			  id, tenant_id, inviter_user_id, agent_id, invite_code, user_id, event_type,
			  source, release_id, client_family, metadata
			) VALUES($1,$2,$3,$4,$5,nullif($6,''),'apk_downloaded','agent_h5',$7,$8,'{}'::jsonb)
		`, "invite_event_"+randomOpaqueID(), invite.TenantID, invite.InviterUserID, agentID,
			invite.InviteCode, userID, release.ID, family); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *postgresStore) AgentInviteProfile(ctx context.Context, userID string) (agentInviteInfo, agentInviteFunnel, error) {
	var invite agentInviteInfo
	err := s.db.QueryRowContext(ctx, `
		SELECT agent.id, agent.user_id, coalesce(nullif(users.raw->>'tenantId', ''), 'tenant_default'),
		       upper(btrim(agent.invite_code)), coalesce(nullif(btrim(agent.raw->>'inviteDisplayName'), ''), '知启云AI合作代理商'),
		       upper(coalesce(agent.status, '')), coalesce(codes.activity_intro, ''),
		       coalesce(agent.operation_center_id, '')
		FROM xz_channel_agents agent
		JOIN xz_users users ON users.id=agent.user_id
		LEFT JOIN xz_marketing_invite_codes codes ON codes.agent_id=agent.id
		WHERE agent.user_id=$1 AND upper(coalesce(agent.status, ''))='ACTIVE'
		ORDER BY agent.level DESC, agent.created_at ASC LIMIT 1
	`, userID).Scan(&invite.AgentID, &invite.InviterUserID, &invite.TenantID, &invite.InviteCode,
		&invite.DisplayName, &invite.AgentStatus, &invite.ActivityIntro, &invite.OperationCenter)
	if err != nil {
		return agentInviteInfo{}, agentInviteFunnel{}, err
	}
	invite.DisplayName = sanitizeAgentInviteDisplayName(invite.DisplayName)
	invite.RegistrationOK = true
	var funnel agentInviteFunnel
	err = s.db.QueryRowContext(ctx, `
		SELECT
		  count(*) FILTER (WHERE event_type='page_view'),
		  count(DISTINCT user_id) FILTER (WHERE event_type='registered'),
		  count(DISTINCT coalesce(user_id, id)) FILTER (WHERE event_type='apk_downloaded'),
		  count(DISTINCT user_id) FILTER (WHERE event_type='app_activated')
		FROM xz_agent_invite_events WHERE agent_id=$1
	`, invite.AgentID).Scan(&funnel.PageViews, &funnel.Registered, &funnel.Downloads, &funnel.Activations)
	return invite, funnel, err
}

func (s *postgresStore) SaveAgentInviteLanding(ctx context.Context, invite agentInviteInfo, landingURL string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO xz_marketing_invite_codes(
		  id, owner_user_id, agent_id, code, landing_url, status, created_at, updated_at
		) VALUES($1,$2,$3,$4,$5,'ACTIVE',now(),now())
		ON CONFLICT (code) DO UPDATE SET
		  owner_user_id=excluded.owner_user_id, agent_id=excluded.agent_id,
		  landing_url=excluded.landing_url, status='ACTIVE', updated_at=now()
	`, "marketing_invite_"+shortStableHash(invite.AgentID, 20), invite.InviterUserID,
		invite.AgentID, invite.InviteCode, landingURL)
	return err
}

func (s *postgresStore) RecordAppActivation(ctx context.Context, userID, requestHash, family string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO xz_agent_invite_events(
		  id, tenant_id, inviter_user_id, agent_id, invite_code, user_id, event_type,
		  source, request_key_hash, client_family, metadata
		)
		SELECT $1, relation.tenant_id, agent.user_id, agent.id, upper(btrim(agent.invite_code)),
		       relation.user_id, 'app_activated', 'android_app', $2, $3, '{}'::jsonb
		FROM xz_user_relationships relation
		JOIN xz_channel_agents agent ON agent.id=relation.parent_agent_id
		WHERE relation.user_id=$4 AND relation.status='ACTIVE' AND relation.ended_at IS NULL
		  AND upper(coalesce(agent.status, ''))='ACTIVE'
		ON CONFLICT DO NOTHING
	`, "invite_event_"+randomOpaqueID(), requestHash, family, userID)
	return err
}

func agentInviteH5Redirect(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(strings.TrimSpace(r.PathValue("inviteCode")))
	if code == "" {
		http.NotFound(w, r)
		return
	}
	target := "/h5/#/pages/invite/InviteRegisterPage?inviteCode=" + url.QueryEscape(code)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusFound)
}

func validateSHA256(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validateAppRelease(release appRelease) error {
	if release.Platform == "" || release.Channel == "" || release.VersionName == "" || release.VersionCode <= 0 {
		return errors.New("release identity is incomplete")
	}
	if !validateSHA256(release.SHA256) {
		return errors.New("release sha256 is invalid")
	}
	if strings.EqualFold(release.Platform, "android") &&
		(!strings.Contains(strings.ToLower(release.APKURL), ".apk") || !strings.Contains(release.APKURL, release.VersionName)) {
		return fmt.Errorf("android release URL must target a versioned APK")
	}
	return nil
}
