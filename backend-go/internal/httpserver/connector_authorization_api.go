package httpserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

const connectorAuthorizationTTL = 5 * time.Minute

func (a *connectorAPI) authorizationPlatforms(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	access, ok := a.enterprise.require(w, r, "enterprise.connector.read")
	if !ok {
		return
	}
	items, err := a.platformViews(r.Context(), access.TenantID)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a *connectorAPI) createAuthorizationSession(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	access, ok := a.enterprise.require(w, r, "enterprise.connector.manage")
	if !ok {
		return
	}
	var request connectorAuthorizationCreateRequest
	if !decodeEnterpriseJSON(w, r, &request) {
		return
	}
	platform := strings.ToLower(strings.TrimSpace(request.Platform))
	if platform == "" {
		platform = "universal"
	}
	if !validAuthorizationPlatform(platform, true) {
		writeError(w, http.StatusBadRequest, errors.New("unsupported connector authorization platform"))
		return
	}
	platforms, err := a.platformViews(r.Context(), access.TenantID)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	if platform != "universal" && !platformAvailable(platforms, platform) {
		writeError(w, http.StatusConflict, errors.New("the selected platform requires configuration before QR authorization"))
		return
	}
	if platform == "universal" && !anyPlatformAvailable(platforms) {
		writeError(w, http.StatusConflict, errors.New("configure at least one supported platform before creating a QR code"))
		return
	}
	ticket, err := newAuthorizationTicket()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	now := time.Now().UTC()
	item := connectorAuthorizationSession{
		ID: newConnectorID("connector_auth"), EnterpriseID: access.TenantID, Platform: platform,
		StateHash: authorizationTicketHash(ticket), Status: "PENDING", CreatedByUserID: access.UserID,
		CreatedByRole: access.Role, OrganizationID: access.OrganizationID,
		ExpiresAt: now.Add(connectorAuthorizationTTL), CreatedAt: now, UpdatedAt: now,
	}
	if err := a.repo.createAuthorizationSession(r.Context(), item); err != nil {
		writeConnectorError(w, err)
		return
	}
	authorizationURL := a.publicConnectorBaseURL(r) + "/api/open/connectors/authorize/" + url.PathEscape(ticket)
	png, err := qrcode.Encode(authorizationURL, qrcode.Medium, 320)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	view := a.authorizationView(item)
	view.AuthorizationURL = authorizationURL
	view.QRCodeDataURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	_ = insertTenantAuditDirect(r.Context(), a.repo.db, access, "enterprise.connector.oauth.create", "connector_authorization_session", item.ID, access.UserID, map[string]any{"platform": platform})
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, view)
}

func (a *connectorAPI) getAuthorizationSession(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	access, ok := a.enterprise.require(w, r, "enterprise.connector.read")
	if !ok {
		return
	}
	item, err := a.repo.authorizationSessionByID(r.Context(), access.TenantID, strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, errors.New("authorization session not found"))
			return
		}
		writeConnectorError(w, err)
		return
	}
	writeJSON(w, a.authorizationView(item))
}

func (a *connectorAPI) cancelAuthorizationSession(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	access, ok := a.enterprise.require(w, r, "enterprise.connector.manage")
	if !ok {
		return
	}
	if err := a.repo.cancelAuthorizationSession(r.Context(), access.TenantID, strings.TrimSpace(r.PathValue("id"))); err != nil {
		writeConnectorError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *connectorAPI) authorizationLanding(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	ticket := strings.TrimSpace(r.PathValue("ticket"))
	item, err := a.repo.authorizationSessionByStateHash(r.Context(), authorizationTicketHash(ticket))
	if err != nil || (item.Status != "PENDING" && item.Status != "AUTHORIZING") {
		writeAuthorizationHTML(w, http.StatusGone, "二维码已失效", "请返回企业管理后台重新生成二维码。", "")
		return
	}
	platforms, err := a.platformViews(r.Context(), item.EnterpriseID)
	if err != nil {
		writeAuthorizationHTML(w, http.StatusInternalServerError, "暂时无法授权", "平台状态读取失败，请稍后重试。", "")
		return
	}
	if item.Platform != "universal" {
		http.Redirect(w, r, "/api/open/connectors/authorize/"+url.PathEscape(ticket)+"/start?platform="+url.QueryEscape(item.Platform), http.StatusFound)
		return
	}
	var cards strings.Builder
	for _, platform := range platforms {
		state := "暂不可用"
		action := `<span class="disabled">` + html.EscapeString(platform.Prerequisite) + `</span>`
		if platform.Available {
			state = "扫码连接"
			action = `<a href="/api/open/connectors/authorize/` + url.PathEscape(ticket) + `/start?platform=` + url.QueryEscape(platform.Key) + `">继续授权</a>`
		}
		cards.WriteString(`<section><h2>` + html.EscapeString(platform.Name) + `</h2><b>` + state + `</b><p>` + html.EscapeString(platform.Description) + `</p>` + action + `</section>`)
	}
	body := `<div class="grid">` + cards.String() + `</div>`
	writeAuthorizationHTML(w, http.StatusOK, "连接企业协作平台", "请选择刚才用于扫码的平台完成授权。二维码只在 5 分钟内有效。", body)
}

func (a *connectorAPI) startAuthorization(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	ticket := strings.TrimSpace(r.PathValue("ticket"))
	platform := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("platform")))
	if !validAuthorizationPlatform(platform, false) {
		writeAuthorizationHTML(w, http.StatusBadRequest, "不支持的平台", "请选择飞书、企业微信、钉钉或微信。", "")
		return
	}
	item, err := a.repo.beginAuthorizationSession(r.Context(), authorizationTicketHash(ticket), platform)
	if err != nil {
		writeAuthorizationHTML(w, http.StatusGone, "二维码已失效", "请返回企业管理后台重新生成二维码。", "")
		return
	}
	redirectURI := a.publicConnectorBaseURL(r) + "/api/open/connectors/oauth/" + platform + "/callback"
	switch platform {
	case "feishu":
		connectorItem, found, findErr := a.repo.connectorForEnterprise(r.Context(), item.EnterpriseID, "feishu")
		if findErr != nil || !found || connectorItem.AppID == "" || connectorItem.AppSecretEncrypted == "" {
			a.repo.failAuthorizationSession(r.Context(), item.StateHash, "FEISHU_NOT_CONFIGURED", "Feishu application credentials are not configured")
			writeAuthorizationHTML(w, http.StatusConflict, "飞书尚未配置", "请管理员先保存飞书自建应用 App ID 和 App Secret。", "")
			return
		}
		base := firstNonEmptyString(strings.TrimRight(strings.TrimSpace(a.cfg.FeishuAccountsBaseURL), "/"), "https://accounts.feishu.cn")
		values := url.Values{"client_id": {connectorItem.AppID}, "response_type": {"code"}, "redirect_uri": {redirectURI}, "state": {ticket}}
		http.Redirect(w, r, base+"/open-apis/authen/v1/authorize?"+values.Encode(), http.StatusFound)
	case "wechat":
		if strings.TrimSpace(a.cfg.WeChatOpenAppID) == "" || strings.TrimSpace(a.cfg.WeChatOpenAppSecret) == "" {
			a.repo.failAuthorizationSession(r.Context(), item.StateHash, "WECHAT_NOT_CONFIGURED", "WeChat Open Platform website application credentials are not configured")
			writeAuthorizationHTML(w, http.StatusConflict, "微信开放平台尚未配置", "请管理员先配置已审核的网站应用 AppID 与 AppSecret。", "")
			return
		}
		base := firstNonEmptyString(strings.TrimRight(strings.TrimSpace(a.cfg.WeChatOpenAuthorizeBaseURL), "/"), "https://open.weixin.qq.com")
		values := url.Values{"appid": {a.cfg.WeChatOpenAppID}, "redirect_uri": {redirectURI}, "response_type": {"code"}, "scope": {"snsapi_login"}, "state": {ticket}}
		http.Redirect(w, r, base+"/connect/qrconnect?"+values.Encode()+"#wechat_redirect", http.StatusFound)
	default:
		a.repo.failAuthorizationSession(r.Context(), item.StateHash, strings.ToUpper(platform)+"_ISV_REQUIRED", "third-party application suite is required")
		writeAuthorizationHTML(w, http.StatusConflict, "需要第三方应用资质", "跨企业扫码安装需要先在对应开放平台创建并审核第三方应用套件。", "")
	}
}

func (a *connectorAPI) feishuOAuthCallback(w http.ResponseWriter, r *http.Request) {
	a.connectorOAuthCallback(w, r, "feishu")
}

func (a *connectorAPI) wechatOAuthCallback(w http.ResponseWriter, r *http.Request) {
	a.connectorOAuthCallback(w, r, "wechat")
}

func (a *connectorAPI) connectorOAuthCallback(w http.ResponseWriter, r *http.Request, platform string) {
	if !a.available(w) {
		return
	}
	ticket := strings.TrimSpace(r.URL.Query().Get("state"))
	stateHash := authorizationTicketHash(ticket)
	item, err := a.repo.authorizationSessionByStateHash(r.Context(), stateHash)
	if err != nil || item.Status != "AUTHORIZING" || item.Platform != platform {
		writeAuthorizationHTML(w, http.StatusGone, "授权会话无效", "请关闭页面并重新扫码。", "")
		return
	}
	if oauthError := strings.TrimSpace(r.URL.Query().Get("error")); oauthError != "" {
		a.repo.failAuthorizationSession(r.Context(), stateHash, "OAUTH_DENIED", oauthError)
		writeAuthorizationHTML(w, http.StatusBadRequest, "授权未完成", "你已取消授权，可以关闭页面后重试。", "")
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		a.repo.failAuthorizationSession(r.Context(), stateHash, "OAUTH_CODE_MISSING", "OAuth code is missing")
		writeAuthorizationHTML(w, http.StatusBadRequest, "授权参数不完整", "未收到平台授权码，请重新扫码。", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.FeishuHTTPTimeout())
	defer cancel()
	var identity connectorExternalIdentity
	var connectorItem enterpriseConnector
	if platform == "feishu" {
		connectorItem, _, err = a.repo.connectorForEnterprise(ctx, item.EnterpriseID, "feishu")
		if err == nil {
			identity, err = a.fetchFeishuOAuthIdentity(ctx, connectorItem, code, a.publicConnectorBaseURL(r)+"/api/open/connectors/oauth/feishu/callback")
		}
	} else {
		connectorItem, err = a.ensureWeChatIdentityConnector(ctx, item.EnterpriseID)
		if err == nil {
			identity, err = a.fetchWeChatOAuthIdentity(ctx, code)
		}
	}
	if err == nil {
		err = a.repo.completeAuthorizationSession(ctx, item, connectorItem, identity)
	}
	if err != nil {
		a.repo.failAuthorizationSession(r.Context(), stateHash, "OAUTH_BIND_FAILED", friendlyConnectorError(err))
		writeAuthorizationHTML(w, http.StatusBadGateway, "连接失败", "平台身份校验或账号绑定失败，请返回后台重试。", "")
		return
	}
	writeAuthorizationHTML(w, http.StatusOK, "连接成功", "账号已安全绑定到当前企业，可以关闭此页面。", `<div class="success">✓ 已完成授权</div>`)
}

func (a *connectorAPI) fetchFeishuOAuthIdentity(ctx context.Context, item enterpriseConnector, code, redirectURI string) (connectorExternalIdentity, error) {
	if a.cipher == nil || a.cipherErr != nil {
		return connectorExternalIdentity{}, errors.New("connector secret cipher is unavailable")
	}
	secret, err := a.cipher.Decrypt(item.AppSecretEncrypted, item.ID)
	if err != nil {
		return connectorExternalIdentity{}, err
	}
	base := firstNonEmptyString(strings.TrimRight(strings.TrimSpace(a.cfg.FeishuAPIBaseURL), "/"), "https://open.feishu.cn/open-apis")
	var tokenResponse map[string]any
	if err := connectorOAuthJSON(ctx, http.MethodPost, base+"/authen/v2/oauth/token", "", map[string]any{
		"grant_type": "authorization_code", "client_id": item.AppID, "client_secret": secret, "code": code, "redirect_uri": redirectURI,
	}, &tokenResponse); err != nil {
		return connectorExternalIdentity{}, err
	}
	accessToken := nestedString(tokenResponse, "access_token")
	if accessToken == "" {
		accessToken = nestedString(tokenResponse, "data", "access_token")
	}
	if accessToken == "" {
		return connectorExternalIdentity{}, errors.New("Feishu OAuth response did not contain an access token")
	}
	var user map[string]any
	if err := connectorOAuthJSON(ctx, http.MethodGet, base+"/authen/v1/user_info", accessToken, nil, &user); err != nil {
		return connectorExternalIdentity{}, err
	}
	data := nestedMap(user, "data")
	if len(data) == 0 {
		data = user
	}
	identity := connectorExternalIdentity{Platform: "feishu", ExternalTenantKey: nestedString(data, "tenant_key"), ExternalUserID: nestedString(data, "open_id"), ExternalUnionID: nestedString(data, "union_id"), ExternalName: nestedString(data, "name"), ExternalAvatar: nestedString(data, "avatar_url")}
	if identity.ExternalUserID == "" {
		return connectorExternalIdentity{}, errors.New("Feishu OAuth user identity is incomplete")
	}
	return identity, nil
}

func (a *connectorAPI) fetchWeChatOAuthIdentity(ctx context.Context, code string) (connectorExternalIdentity, error) {
	base := firstNonEmptyString(strings.TrimRight(strings.TrimSpace(a.cfg.WeChatOpenAPIBaseURL), "/"), "https://api.weixin.qq.com")
	values := url.Values{"appid": {a.cfg.WeChatOpenAppID}, "secret": {a.cfg.WeChatOpenAppSecret}, "code": {code}, "grant_type": {"authorization_code"}}
	var token map[string]any
	if err := connectorOAuthJSON(ctx, http.MethodGet, base+"/sns/oauth2/access_token?"+values.Encode(), "", nil, &token); err != nil {
		return connectorExternalIdentity{}, err
	}
	accessToken, openID := nestedString(token, "access_token"), nestedString(token, "openid")
	if accessToken == "" || openID == "" {
		return connectorExternalIdentity{}, errors.New("WeChat OAuth response did not contain a user identity")
	}
	var user map[string]any
	infoValues := url.Values{"access_token": {accessToken}, "openid": {openID}, "lang": {"zh_CN"}}
	if err := connectorOAuthJSON(ctx, http.MethodGet, base+"/sns/userinfo?"+infoValues.Encode(), "", nil, &user); err != nil {
		return connectorExternalIdentity{}, err
	}
	return connectorExternalIdentity{Platform: "wechat", ExternalUserID: openID, ExternalUnionID: nestedString(user, "unionid"), ExternalName: nestedString(user, "nickname"), ExternalAvatar: nestedString(user, "headimgurl")}, nil
}

func (a *connectorAPI) ensureWeChatIdentityConnector(ctx context.Context, enterpriseID string) (enterpriseConnector, error) {
	item, found, err := a.repo.connectorForEnterprise(ctx, enterpriseID, "wechat")
	if err != nil || found {
		return item, err
	}
	item, err = a.repo.createConnector(ctx, enterpriseConnector{ID: newConnectorID("connector"), EnterpriseID: enterpriseID, ConnectorType: "wechat", ConnectorName: "微信扫码身份连接", ConnectorKey: newConnectorID("wxc"), AppID: a.cfg.WeChatOpenAppID, Status: "active", Config: defaultConnectorConfig()})
	if err != nil {
		return enterpriseConnector{}, err
	}
	return a.repo.updateConnectorState(ctx, enterpriseID, item.ID, "active", "", true)
}

func (a *connectorAPI) platformViews(ctx context.Context, enterpriseID string) ([]connectorPlatformView, error) {
	feishu, found, err := a.repo.connectorForEnterprise(ctx, enterpriseID, "feishu")
	if err != nil {
		return nil, err
	}
	feishuConfigured := found && feishu.AppID != "" && feishu.AppSecretEncrypted != ""
	wechatConfigured := strings.TrimSpace(a.cfg.WeChatOpenAppID) != "" && strings.TrimSpace(a.cfg.WeChatOpenAppSecret) != ""
	wechatConnector, wechatFound, err := a.repo.connectorForEnterprise(ctx, enterpriseID, "wechat")
	if err != nil {
		return nil, err
	}
	return []connectorPlatformView{
		{Key: "feishu", Name: "飞书", Available: feishuConfigured, Configured: feishuConfigured, Connected: found && feishu.Status == "active", Mode: "oauth_user_binding", Description: "使用当前企业的飞书自建应用完成用户身份绑定。", Prerequisite: "先配置飞书 App ID 与 App Secret"},
		{Key: "wecom", Name: "企业微信", Available: false, Configured: false, Connected: false, Mode: "isv_suite_required", Description: "跨企业安装机器人需要企业微信第三方应用套件。", Prerequisite: "需创建并审核企业微信第三方应用"},
		{Key: "dingtalk", Name: "钉钉", Available: false, Configured: false, Connected: false, Mode: "isv_suite_required", Description: "跨组织安装机器人需要钉钉第三方企业应用。", Prerequisite: "需创建并审核钉钉第三方企业应用"},
		{Key: "wechat", Name: "微信", Available: wechatConfigured, Configured: wechatConfigured, Connected: wechatFound && wechatConnector.Status == "active", Mode: "website_oauth", Description: "使用微信开放平台网站应用完成个人微信身份绑定。", Prerequisite: "需配置微信开放平台网站应用"},
	}, nil
}

func (a *connectorAPI) authorizationView(item connectorAuthorizationSession) connectorAuthorizationView {
	return connectorAuthorizationView{ID: item.ID, Platform: item.Platform, Status: item.Status, ConnectorID: item.ConnectorID,
		ExternalUserName: item.ExternalUserName, Result: item.Result, ErrorCode: item.ErrorCode, ErrorMessage: item.ErrorMessage,
		ExpiresAt: connectorTime(item.ExpiresAt), CreatedAt: connectorTime(item.CreatedAt)}
}

func (a *connectorAPI) publicConnectorBaseURL(r *http.Request) string {
	if value := strings.TrimRight(strings.TrimSpace(a.cfg.ConnectorCallbackBaseURL), "/"); value != "" {
		return value
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}

func newAuthorizationTicket() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func authorizationTicketHash(ticket string) string {
	sum := sha256.Sum256([]byte(ticket))
	return hex.EncodeToString(sum[:])
}

func validAuthorizationPlatform(platform string, universal bool) bool {
	return (universal && platform == "universal") || platform == "feishu" || platform == "wecom" || platform == "dingtalk" || platform == "wechat"
}

func platformAvailable(items []connectorPlatformView, key string) bool {
	for _, item := range items {
		if item.Key == key {
			return item.Available
		}
	}
	return false
}

func anyPlatformAvailable(items []connectorPlatformView) bool {
	for _, item := range items {
		if item.Available {
			return true
		}
	}
	return false
}

func connectorOAuthJSON(ctx context.Context, method, endpoint, bearer string, body any, target any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseRaw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("OAuth provider returned HTTP %d", response.StatusCode)
	}
	if err := json.Unmarshal(responseRaw, target); err != nil {
		return errors.New("OAuth provider returned invalid JSON")
	}
	if result, ok := target.(*map[string]any); ok {
		if code := intValue((*result)["code"]); code != 0 {
			return fmt.Errorf("OAuth provider rejected request: %s", nestedString(*result, "msg"))
		}
		if errorCode := intValue((*result)["errcode"]); errorCode != 0 {
			return fmt.Errorf("OAuth provider rejected request: %s", nestedString(*result, "errmsg"))
		}
	}
	return nil
}

func nestedMap(value map[string]any, keys ...string) map[string]any {
	current := value
	for _, key := range keys {
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

func nestedString(value map[string]any, keys ...string) string {
	if len(keys) == 0 {
		return ""
	}
	if len(keys) == 1 {
		raw, ok := value[keys[0]]
		if !ok || raw == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(raw))
	}
	return nestedString(nestedMap(value, keys[:len(keys)-1]...), keys[len(keys)-1])
}

func writeAuthorizationHTML(w http.ResponseWriter, status int, title, message, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>`+html.EscapeString(title)+`</title><style>body{margin:0;background:#f4f7fb;color:#182230;font:15px/1.65 system-ui,-apple-system,"Segoe UI",sans-serif}.wrap{max-width:780px;margin:0 auto;padding:48px 20px}.panel{background:#fff;border:1px solid #e4e7ec;border-radius:22px;padding:32px;box-shadow:0 16px 48px #10182812}h1{margin:0 0 8px;font-size:28px}p{color:#667085}.grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px;margin-top:28px}section{border:1px solid #e4e7ec;border-radius:16px;padding:18px}section h2{margin:0;font-size:19px}section b{display:block;color:#175cd3;margin-top:4px}a{display:inline-block;background:#155eef;color:#fff;text-decoration:none;padding:9px 16px;border-radius:10px}.disabled{color:#98a2b3}.success{margin-top:24px;padding:18px;border-radius:14px;background:#ecfdf3;color:#067647;font-weight:700}@media(max-width:600px){.wrap{padding:18px 12px}.panel{padding:22px}.grid{grid-template-columns:1fr}}</style></head><body><div class="wrap"><main class="panel"><h1>`+html.EscapeString(title)+`</h1><p>`+html.EscapeString(message)+`</p>`+body+`</main></div></body></html>`)
}
