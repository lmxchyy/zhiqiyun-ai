package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const wechatWebLoginTTL = 5 * time.Minute

type wechatWebLoginSession struct {
	ID        string                   `json:"id"`
	Status    string                   `json:"status"`
	Identity  wechatMiniProgramSession `json:"identity,omitempty"`
	UserID    string                   `json:"userId,omitempty"`
	CreatedAt time.Time                `json:"createdAt"`
	ExpiresAt time.Time                `json:"expiresAt"`
}

type wechatWebBindMobileRequest struct {
	QRCodeID string `json:"qrCodeId"`
	Mobile   string `json:"mobile"`
	SMSCode  string `json:"smsCode"`
}

type wechatWebTokenResponse struct {
	OpenID      string `json:"openid"`
	UnionID     string `json:"unionid"`
	AccessToken string `json:"access_token"`
	ErrorCode   int    `json:"errcode"`
	Error       string `json:"errmsg"`
}

func (a authAPI) wechatWebQRCode(w http.ResponseWriter, r *http.Request) {
	store, ok := a.sessions.(wechatWebLoginSessionStore)
	if !ok || store == nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "WECHAT_QR_STATE_UNAVAILABLE", "微信扫码登录状态服务暂不可用")
		return
	}
	appID, _, authorizeBase, redirectURL, err := wechatWebLoginConfig()
	if err != nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "WECHAT_WEB_LOGIN_NOT_CONFIGURED", "网页微信扫码登录尚未配置")
		return
	}
	id, err := randomAuthToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	now := time.Now().UTC()
	session := wechatWebLoginSession{ID: id, Status: "PENDING", CreatedAt: now, ExpiresAt: now.Add(wechatWebLoginTTL)}
	if err := store.PutWeChatWebLogin(r.Context(), id, session, wechatWebLoginTTL); err != nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "WECHAT_QR_STATE_UNAVAILABLE", "微信扫码登录状态服务暂不可用")
		return
	}
	query := url.Values{
		"appid":         {appID},
		"redirect_uri":  {redirectURL},
		"response_type": {"code"},
		"scope":         {"snsapi_login"},
		"state":         {id},
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{
		"qrCodeId": id, "qrUrl": strings.TrimRight(authorizeBase, "/") + "/connect/qrconnect?" + query.Encode() + "#wechat_redirect",
		"status": "PENDING", "expiresInSeconds": int(wechatWebLoginTTL.Seconds()), "pollIntervalMs": 2000,
	})
}

func (a authAPI) wechatWebCallback(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if id == "" || code == "" {
		writeWeChatWebCallbackPage(w, false, "微信登录参数不完整，请返回知启云重新扫码。")
		return
	}
	store, ok := a.sessions.(wechatWebLoginSessionStore)
	if !ok || store == nil {
		writeWeChatWebCallbackPage(w, false, "登录状态服务暂不可用，请稍后重试。")
		return
	}
	session, found, err := store.WeChatWebLogin(r.Context(), id)
	if err != nil || !found || session.Status != "PENDING" || time.Now().After(session.ExpiresAt) {
		writeWeChatWebCallbackPage(w, false, "二维码已过期或已使用，请返回知启云刷新二维码。")
		return
	}
	identity, err := exchangeWeChatWebCode(r.Context(), code)
	if err != nil {
		writeWeChatWebCallbackPage(w, false, "微信身份验证失败，请返回知启云重试。")
		return
	}
	data, err := a.store.AdminData()
	if err != nil {
		writeWeChatWebCallbackPage(w, false, "登录服务暂不可用，请稍后重试。")
		return
	}
	if user, exists := findUserByWechatIdentity(data.Users, identity); exists {
		if !strings.EqualFold(user.Status, "ACTIVE") {
			writeWeChatWebCallbackPage(w, false, "当前账号暂时无法登录，请联系管理员。")
			return
		}
		session.Status = "CONFIRMED"
		session.UserID = user.ID
	} else {
		session.Status = "MOBILE_REQUIRED"
		session.Identity = identity
	}
	remaining := time.Until(session.ExpiresAt)
	if remaining <= 0 || store.PutWeChatWebLogin(r.Context(), id, session, remaining) != nil {
		writeWeChatWebCallbackPage(w, false, "二维码已过期，请返回知启云刷新二维码。")
		return
	}
	writeWeChatWebCallbackPage(w, true, "扫码确认成功，请返回知启云继续登录。")
}

func (a authAPI) wechatWebStatus(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("qrCodeId"))
	if id == "" {
		writeAuthFlowError(w, http.StatusBadRequest, "WECHAT_QR_ID_REQUIRED", "二维码标识不能为空")
		return
	}
	store, ok := a.sessions.(wechatWebLoginSessionStore)
	if !ok || store == nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "WECHAT_QR_STATE_UNAVAILABLE", "微信扫码登录状态服务暂不可用")
		return
	}
	session, found, err := store.WeChatWebLogin(r.Context(), id)
	if err != nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "WECHAT_QR_STATE_UNAVAILABLE", "微信扫码登录状态服务暂不可用")
		return
	}
	if !found || time.Now().After(session.ExpiresAt) {
		writeAuthFlowError(w, http.StatusGone, "WECHAT_QR_EXPIRED", "二维码已过期，请刷新后重试")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if session.Status != "CONFIRMED" {
		writeJSON(w, map[string]any{"qrCodeId": id, "status": session.Status, "expiresAt": session.ExpiresAt})
		return
	}
	claimed, found, err := store.TakeWeChatWebLogin(r.Context(), id)
	if err != nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "WECHAT_QR_STATE_UNAVAILABLE", "微信扫码登录状态服务暂不可用")
		return
	}
	if !found || claimed.Status != "CONFIRMED" {
		writeAuthFlowError(w, http.StatusConflict, "WECHAT_QR_ALREADY_CONSUMED", "该二维码已完成登录，请勿重复提交")
		return
	}
	session = claimed
	data, err := a.store.AdminData()
	if err != nil {
		restoreWeChatWebLogin(r.Context(), store, id, session)
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	user, exists := findAuthUserByID(data.Users, session.UserID)
	if !exists || !strings.EqualFold(user.Status, "ACTIVE") {
		writeAuthFlowError(w, http.StatusUnauthorized, "ACCOUNT_UNAVAILABLE", "当前账号暂时无法登录")
		return
	}
	response, err := a.authResponseWithToken(r.Context(), data, user)
	if err != nil {
		restoreWeChatWebLogin(r.Context(), store, id, session)
		writeAuthTokenError(w, err)
		return
	}
	response["status"] = "SUCCESS"
	writeAuthTokenResponse(w, r, response)
}

func (a authAPI) wechatWebBindMobile(w http.ResponseWriter, r *http.Request) {
	var req wechatWebBindMobileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.QRCodeID = strings.TrimSpace(req.QRCodeID)
	mobile := normalizeMainlandMobile(req.Mobile)
	if req.QRCodeID == "" || mobile == "" || strings.TrimSpace(req.SMSCode) == "" {
		writeAuthFlowError(w, http.StatusBadRequest, "WECHAT_BIND_INPUT_REQUIRED", "请填写手机号和验证码")
		return
	}
	store, ok := a.sessions.(wechatWebLoginSessionStore)
	if !ok || store == nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "WECHAT_QR_STATE_UNAVAILABLE", "微信扫码登录状态服务暂不可用")
		return
	}
	session, found, err := store.WeChatWebLogin(r.Context(), req.QRCodeID)
	if err != nil || !found || time.Now().After(session.ExpiresAt) {
		writeAuthFlowError(w, http.StatusGone, "WECHAT_QR_EXPIRED", "二维码已过期，请刷新后重试")
		return
	}
	if session.Status != "MOBILE_REQUIRED" || session.Identity.OpenID == "" {
		writeAuthFlowError(w, http.StatusConflict, "WECHAT_QR_STATE_INVALID", "当前二维码不需要绑定手机号")
		return
	}
	if err := a.verifySMSCode(r.Context(), mobile, strings.TrimSpace(req.SMSCode)); err != nil {
		writeMappedAuthFlowError(w, err)
		return
	}
	data, user, isNewUser, inviteStatus, err := a.userForPhoneIdentity(mobile, session.Identity, authRegistrationInput{RedirectSource: "wechat_web"})
	if err != nil {
		writeMappedAuthFlowError(w, err)
		return
	}
	response, err := a.authResponseWithToken(r.Context(), data, user)
	if err != nil {
		writeAuthTokenError(w, err)
		return
	}
	if err := store.DeleteWeChatWebLogin(r.Context(), req.QRCodeID); err != nil {
		writeAuthFlowError(w, http.StatusServiceUnavailable, "WECHAT_QR_STATE_UNAVAILABLE", "微信扫码登录状态服务暂不可用")
		return
	}
	response["status"] = "SUCCESS"
	response["isNewUser"] = isNewUser
	response["inviteBindStatus"] = inviteStatus
	writeAuthTokenResponse(w, r, response)
}

func wechatWebLoginConfig() (string, string, string, string, error) {
	appID := strings.TrimSpace(os.Getenv("WECHAT_OPEN_APP_ID"))
	secret := strings.TrimSpace(os.Getenv("WECHAT_OPEN_APP_SECRET"))
	authorizeBase := strings.TrimSpace(os.Getenv("WECHAT_OPEN_AUTHORIZE_BASE_URL"))
	redirectURL := strings.TrimSpace(os.Getenv("WECHAT_OPEN_REDIRECT_URL"))
	if authorizeBase == "" {
		authorizeBase = "https://open.weixin.qq.com"
	}
	parsed, err := url.Parse(redirectURL)
	if appID == "" || secret == "" || err != nil || !parsed.IsAbs() || (authProductionEnvironment() && parsed.Scheme != "https") {
		return "", "", "", "", errors.New("wechat web login is not configured")
	}
	return appID, secret, authorizeBase, redirectURL, nil
}

func exchangeWeChatWebCode(ctx context.Context, code string) (wechatMiniProgramSession, error) {
	appID, secret, _, _, err := wechatWebLoginConfig()
	if err != nil {
		return wechatMiniProgramSession{}, err
	}
	apiBase := strings.TrimSpace(os.Getenv("WECHAT_OPEN_API_BASE_URL"))
	if apiBase == "" {
		apiBase = "https://api.weixin.qq.com"
	}
	query := url.Values{"appid": {appID}, "secret": {secret}, "code": {code}, "grant_type": {"authorization_code"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(apiBase, "/")+"/sns/oauth2/access_token?"+query.Encode(), nil)
	if err != nil {
		return wechatMiniProgramSession{}, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return wechatMiniProgramSession{}, err
	}
	defer response.Body.Close()
	var token wechatWebTokenResponse
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&token) != nil || token.ErrorCode != 0 || token.OpenID == "" {
		return wechatMiniProgramSession{}, fmt.Errorf("wechat web code exchange failed")
	}
	if token.UnionID == "" && token.AccessToken != "" {
		userQuery := url.Values{"access_token": {token.AccessToken}, "openid": {token.OpenID}, "lang": {"zh_CN"}}
		userRequest, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(apiBase, "/")+"/sns/userinfo?"+userQuery.Encode(), nil)
		if requestErr == nil {
			if userResponse, userErr := client.Do(userRequest); userErr == nil {
				defer userResponse.Body.Close()
				var profile wechatWebTokenResponse
				if userResponse.StatusCode == http.StatusOK && json.NewDecoder(userResponse.Body).Decode(&profile) == nil {
					token.UnionID = profile.UnionID
				}
			}
		}
	}
	return wechatMiniProgramSession{OpenID: token.OpenID, UnionID: token.UnionID}, nil
}

func writeWeChatWebCallbackPage(w http.ResponseWriter, success bool, message string) {
	color := "#d92d20"
	title := "登录未完成"
	if success {
		color = "#12b76a"
		title = "扫码成功"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title><body style="font-family:system-ui;text-align:center;padding:64px 20px;color:#101828"><div style="font-size:52px;color:%s">%s</div><h1>%s</h1><p>%s</p></body></html>`, html.EscapeString(title), color, map[bool]string{true: "✓", false: "!"}[success], html.EscapeString(title), html.EscapeString(message))
}

func findAuthUserByID(users []adminUser, userID string) (adminUser, bool) {
	for _, user := range users {
		if strings.EqualFold(strings.TrimSpace(user.ID), strings.TrimSpace(userID)) {
			return user, true
		}
	}
	return adminUser{}, false
}

func restoreWeChatWebLogin(ctx context.Context, store wechatWebLoginSessionStore, id string, session wechatWebLoginSession) {
	remaining := time.Until(session.ExpiresAt)
	if remaining > 0 {
		_ = store.PutWeChatWebLogin(ctx, id, session, remaining)
	}
}
