package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

var (
	errContentSecurityRejected    = errors.New("所发布内容含违规信息")
	errContentSecurityUnavailable = errors.New("内容安全检测暂不可用，请稍后重试")
)

type wechatContentSecurityChecker interface {
	CheckImage(context.Context, []byte, string, string) error
	CheckText(context.Context, string, string) error
}

type wechatContentSecurityService struct {
	appID      string
	appSecret  string
	baseURL    string
	client     *http.Client
	mu         sync.Mutex
	token      string
	tokenUntil time.Time
}

type wechatSecurityResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
	Result  struct {
		Suggest string `json:"suggest"`
		Label   int    `json:"label"`
	} `json:"result"`
}

func newWeChatContentSecurityService(cfg config.Config) wechatContentSecurityChecker {
	appID := strings.TrimSpace(cfg.WeChatMiniProgramAppID)
	secret := strings.TrimSpace(cfg.WeChatMiniProgramSecret)
	if appID == "" || secret == "" {
		return nil
	}
	return &wechatContentSecurityService{
		appID: appID, appSecret: secret, baseURL: "https://api.weixin.qq.com",
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *wechatContentSecurityService) CheckImage(ctx context.Context, raw []byte, filename string, contentType string) error {
	if s == nil || len(raw) == 0 {
		return errContentSecurityUnavailable
	}
	return s.withAccessTokenRetry(ctx, func(token string) (wechatSecurityResponse, error) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("media", firstNonEmptyString(filename, "upload.jpg"))
		if err != nil {
			return wechatSecurityResponse{}, err
		}
		if _, err := part.Write(raw); err != nil {
			return wechatSecurityResponse{}, err
		}
		if err := writer.Close(); err != nil {
			return wechatSecurityResponse{}, err
		}
		endpoint := strings.TrimRight(s.baseURL, "/") + "/wxa/img_sec_check?access_token=" + url.QueryEscape(token)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
		if err != nil {
			return wechatSecurityResponse{}, err
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		if strings.TrimSpace(contentType) != "" {
			req.Header.Set("X-Upload-Content-Type", strings.TrimSpace(contentType))
		}
		return s.doSecurityRequest(req)
	})
}

func (s *wechatContentSecurityService) CheckText(ctx context.Context, content string, openID string) error {
	content = strings.TrimSpace(content)
	openID = strings.TrimSpace(openID)
	if s == nil || content == "" || openID == "" {
		return errContentSecurityUnavailable
	}
	if len([]rune(content)) > 2500 {
		content = string([]rune(content)[:2500])
	}
	return s.withAccessTokenRetry(ctx, func(token string) (wechatSecurityResponse, error) {
		payload, err := json.Marshal(map[string]any{
			"content": content,
			"version": 2,
			"scene":   2,
			"openid":  openID,
		})
		if err != nil {
			return wechatSecurityResponse{}, err
		}
		endpoint := strings.TrimRight(s.baseURL, "/") + "/wxa/msg_sec_check?access_token=" + url.QueryEscape(token)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			return wechatSecurityResponse{}, err
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		return s.doSecurityRequest(req)
	})
}

func (s *wechatContentSecurityService) withAccessTokenRetry(ctx context.Context, call func(string) (wechatSecurityResponse, error)) error {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := s.accessToken(ctx)
		if err != nil {
			return errContentSecurityUnavailable
		}
		response, err := call(token)
		if err != nil {
			return errContentSecurityUnavailable
		}
		if response.ErrCode == 40001 || response.ErrCode == 40014 || response.ErrCode == 42001 {
			s.clearAccessToken()
			continue
		}
		return contentSecurityResult(response)
	}
	return errContentSecurityUnavailable
}

func contentSecurityResult(response wechatSecurityResponse) error {
	if response.ErrCode == 87014 {
		return errContentSecurityRejected
	}
	if response.ErrCode != 0 {
		return errContentSecurityUnavailable
	}
	switch strings.ToLower(strings.TrimSpace(response.Result.Suggest)) {
	case "risky", "review":
		return errContentSecurityRejected
	default:
		return nil
	}
}

func (s *wechatContentSecurityService) doSecurityRequest(req *http.Request) (wechatSecurityResponse, error) {
	response, err := s.client.Do(req)
	if err != nil {
		return wechatSecurityResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return wechatSecurityResponse{}, fmt.Errorf("wechat content security returned HTTP %d", response.StatusCode)
	}
	var payload wechatSecurityResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return wechatSecurityResponse{}, err
	}
	return payload, nil
}

func (s *wechatContentSecurityService) accessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && time.Now().Before(s.tokenUntil) {
		return s.token, nil
	}
	values := url.Values{
		"grant_type": {"client_credential"},
		"appid":      {s.appID},
		"secret":     {s.appSecret},
	}
	endpoint := strings.TrimRight(s.baseURL, "/") + "/cgi-bin/token?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	response, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("wechat token returned HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return "", err
	}
	if payload.ErrCode != 0 || strings.TrimSpace(payload.AccessToken) == "" {
		return "", fmt.Errorf("wechat token unavailable: %d", payload.ErrCode)
	}
	ttl := time.Duration(payload.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	if ttl > 10*time.Minute {
		ttl -= 5 * time.Minute
	}
	s.token = strings.TrimSpace(payload.AccessToken)
	s.tokenUntil = time.Now().Add(ttl)
	return s.token, nil
}

func (s *wechatContentSecurityService) clearAccessToken() {
	s.mu.Lock()
	s.token = ""
	s.tokenUntil = time.Time{}
	s.mu.Unlock()
}

func isWeChatMiniProgramRequest(r *http.Request) bool {
	platform := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Client-Platform")))
	clientName := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Client-Name")))
	return platform == "mp-weixin" || strings.Contains(clientName, "mini-program")
}

func writeContentSecurityError(w http.ResponseWriter, err error) {
	if errors.Is(err, errContentSecurityRejected) {
		writeError(w, http.StatusUnprocessableEntity, errContentSecurityRejected)
		return
	}
	writeError(w, http.StatusServiceUnavailable, errContentSecurityUnavailable)
}

func (a api) miniProgramOpenIDs(ctx context.Context, user adminUser) []string {
	openIDs := make([]string, 0, 1+len(user.WeChatOpenIDs))
	seen := map[string]bool{}
	appendOpenID := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		openIDs = append(openIDs, value)
	}
	if sessions, ok := a.sessions.(wechatMiniProgramSessionStore); ok {
		if session, found, err := sessions.WeChatSession(ctx, user.ID); err == nil && found && strings.TrimSpace(session.OpenID) != "" {
			appendOpenID(session.OpenID)
		}
	}
	for _, value := range user.WeChatOpenIDs {
		appendOpenID(value)
	}
	return openIDs
}

func (a api) checkMiniProgramText(ctx context.Context, r *http.Request, user adminUser, content string) error {
	if !isWeChatMiniProgramRequest(r) {
		return nil
	}
	if a.contentSecurity == nil {
		return errContentSecurityUnavailable
	}
	openIDs := a.miniProgramOpenIDs(ctx, user)
	if len(openIDs) == 0 {
		return errContentSecurityUnavailable
	}
	for _, openID := range openIDs {
		err := a.contentSecurity.CheckText(ctx, content, openID)
		if err == nil || errors.Is(err, errContentSecurityRejected) {
			return err
		}
	}
	return errContentSecurityUnavailable
}
