package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"xianzhi-ai/backend-go/internal/connector"
)

const defaultBaseURL = "https://open.feishu.cn/open-apis"

type Config struct {
	AppID             string
	AppSecret         string
	VerificationToken string
	EncryptKey        string
	BaseURL           string
	TokenCachePrefix  string
	Timeout           time.Duration
	Retries           int
	Redis             redis.Cmdable
	HTTPClient        *http.Client
}

type cachedToken struct {
	value     string
	expiresAt time.Time
}

type Client struct {
	appID, appSecret, verificationToken, encryptKey string
	baseURL, tokenCachePrefix                       string
	retries                                         int
	httpClient                                      *http.Client
	redis                                           redis.Cmdable
	mu                                              sync.Mutex
	localToken                                      cachedToken
}

type APIError struct {
	Code       int
	Message    string
	HTTPStatus int
}

type BotInfo struct {
	OpenID  string `json:"open_id"`
	AppName string `json:"app_name"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Feishu API failed: code=%d status=%d message=%s", e.Code, e.HTTPStatus, e.Message)
}

func New(config Config) *Client {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	retries := config.Retries
	if retries < 0 {
		retries = 0
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	prefix := strings.TrimSpace(config.TokenCachePrefix)
	if prefix == "" {
		prefix = "xianzhi:connector:feishu:token:"
	}
	return &Client{
		appID: strings.TrimSpace(config.AppID), appSecret: strings.TrimSpace(config.AppSecret),
		verificationToken: strings.TrimSpace(config.VerificationToken), encryptKey: strings.TrimSpace(config.EncryptKey),
		baseURL: baseURL, tokenCachePrefix: prefix, retries: retries,
		httpClient: client, redis: config.Redis,
	}
}

func (c *Client) TestConnection(ctx context.Context) error {
	_, err := c.BotInfo(ctx)
	return err
}

func (c *Client) BotInfo(ctx context.Context) (BotInfo, error) {
	var response struct {
		Code int     `json:"code"`
		Msg  string  `json:"msg"`
		Bot  BotInfo `json:"bot"`
	}
	if err := c.doAuthorized(ctx, http.MethodGet, c.baseURL+"/bot/v3/info", nil, "application/json", &response); err != nil {
		return BotInfo{}, err
	}
	if response.Code != 0 || strings.TrimSpace(response.Bot.OpenID) == "" {
		return BotInfo{}, mapError(response.Code, firstNonEmpty(response.Msg, "Feishu bot open_id is missing"), http.StatusOK)
	}
	return response.Bot, nil
}

func (c *Client) SendText(ctx context.Context, target connector.MessageTarget, message connector.OutgoingMessage) (connector.SendResult, error) {
	content, _ := json.Marshal(map[string]string{"text": message.Text})
	return c.sendMessage(ctx, target, "text", string(content))
}

func (c *Client) SendCard(ctx context.Context, target connector.MessageTarget, message connector.OutgoingMessage) (connector.SendResult, error) {
	content, err := json.Marshal(message.Card)
	if err != nil {
		return connector.SendResult{}, err
	}
	return c.sendMessage(ctx, target, "interactive", string(content))
}

func (c *Client) SendImage(ctx context.Context, target connector.MessageTarget, message connector.OutgoingMessage) (connector.SendResult, error) {
	if message.Image == nil {
		return connector.SendResult{}, errors.New("Feishu image is required")
	}
	raw, err := io.ReadAll(io.LimitReader(message.Image, 64<<20+1))
	if err != nil {
		return connector.SendResult{}, err
	}
	if len(raw) == 0 || len(raw) > 64<<20 {
		return connector.SendResult{}, errors.New("Feishu image size is invalid")
	}
	imageKey, err := c.uploadImage(ctx, raw, message.FileName)
	if err != nil {
		return connector.SendResult{}, err
	}
	content, _ := json.Marshal(map[string]string{"image_key": imageKey})
	return c.sendMessage(ctx, target, "image", string(content))
}

func (c *Client) sendMessage(ctx context.Context, target connector.MessageTarget, messageType string, content string) (connector.SendResult, error) {
	if strings.TrimSpace(target.ChatID) == "" {
		return connector.SendResult{}, errors.New("Feishu chat id is required")
	}
	payload, _ := json.Marshal(map[string]string{"receive_id": target.ChatID, "msg_type": messageType, "content": content})
	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	endpoint := c.baseURL + "/im/v1/messages?receive_id_type=chat_id"
	if err := c.doAuthorizedJSON(ctx, http.MethodPost, endpoint, payload, &response); err != nil {
		return connector.SendResult{}, err
	}
	if response.Code != 0 {
		return connector.SendResult{}, mapError(response.Code, response.Msg, http.StatusOK)
	}
	return connector.SendResult{ExternalMessageID: response.Data.MessageID}, nil
}

func (c *Client) uploadImage(ctx context.Context, raw []byte, fileName string) (string, error) {
	if strings.TrimSpace(fileName) == "" {
		fileName = "generated.png"
	}
	buildBody := func() ([]byte, string, error) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		if err := writer.WriteField("image_type", "message"); err != nil {
			return nil, "", err
		}
		part, err := writer.CreateFormFile("image", path.Base(fileName))
		if err != nil {
			return nil, "", err
		}
		if _, err = part.Write(raw); err != nil {
			return nil, "", err
		}
		if err = writer.Close(); err != nil {
			return nil, "", err
		}
		return body.Bytes(), writer.FormDataContentType(), nil
	}
	body, contentType, err := buildBody()
	if err != nil {
		return "", err
	}
	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ImageKey string `json:"image_key"`
		} `json:"data"`
	}
	if err := c.doAuthorized(ctx, http.MethodPost, c.baseURL+"/im/v1/images", body, contentType, &response); err != nil {
		return "", err
	}
	if response.Code != 0 || response.Data.ImageKey == "" {
		return "", mapError(response.Code, response.Msg, http.StatusOK)
	}
	return response.Data.ImageKey, nil
}

func (c *Client) tenantAccessToken(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force {
		if token, ok := c.cachedToken(ctx); ok {
			return token, nil
		}
	}
	if c.appID == "" || c.appSecret == "" {
		return "", errors.New("Feishu App ID and App Secret are required")
	}
	payload, _ := json.Marshal(map[string]string{"app_id": c.appID, "app_secret": c.appSecret})
	var response struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := c.doJSON(ctx, http.MethodPost, c.baseURL+"/auth/v3/tenant_access_token/internal", payload, "application/json", "", &response); err != nil {
		return "", err
	}
	if response.Code != 0 || response.TenantAccessToken == "" {
		return "", mapError(response.Code, response.Msg, http.StatusOK)
	}
	ttl := time.Duration(response.Expire) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	if ttl > 2*time.Minute {
		ttl -= 60 * time.Second
	}
	c.localToken = cachedToken{value: response.TenantAccessToken, expiresAt: time.Now().Add(ttl)}
	if c.redis != nil {
		if err := c.redis.Set(ctx, c.tokenCachePrefix+c.appID, response.TenantAccessToken, ttl).Err(); err != nil {
			log.Printf("connector=feishu operation=cache_token result=degraded app_id=%s error=%v", redact(c.appID), err)
		}
	}
	return response.TenantAccessToken, nil
}

func (c *Client) cachedToken(ctx context.Context) (string, bool) {
	if c.redis != nil {
		token, err := c.redis.Get(ctx, c.tokenCachePrefix+c.appID).Result()
		if err == nil && token != "" {
			return token, true
		}
		if err != nil && !errors.Is(err, redis.Nil) {
			log.Printf("connector=feishu operation=read_token_cache result=degraded app_id=%s error=%v", redact(c.appID), err)
		}
	}
	if c.localToken.value != "" && time.Now().Before(c.localToken.expiresAt) {
		return c.localToken.value, true
	}
	return "", false
}

func (c *Client) invalidateToken(ctx context.Context) {
	c.mu.Lock()
	c.localToken = cachedToken{}
	c.mu.Unlock()
	if c.redis != nil {
		_ = c.redis.Del(ctx, c.tokenCachePrefix+c.appID).Err()
	}
}

func (c *Client) doAuthorizedJSON(ctx context.Context, method string, endpoint string, payload []byte, output any) error {
	return c.doAuthorized(ctx, method, endpoint, payload, "application/json", output)
}

func (c *Client) doAuthorized(ctx context.Context, method string, endpoint string, payload []byte, contentType string, output any) error {
	token, err := c.tenantAccessToken(ctx, false)
	if err != nil {
		return err
	}
	err = c.doJSON(ctx, method, endpoint, payload, contentType, token, output)
	var apiErr *APIError
	if errors.As(err, &apiErr) && isTokenError(apiErr.Code, apiErr.HTTPStatus) {
		c.invalidateToken(ctx)
		token, tokenErr := c.tenantAccessToken(ctx, true)
		if tokenErr != nil {
			return tokenErr
		}
		return c.doJSON(ctx, method, endpoint, payload, contentType, token, output)
	}
	return err
}

func (c *Client) doJSON(ctx context.Context, method string, endpoint string, payload []byte, contentType string, token string, output any) error {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", contentType)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
		} else {
			responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
			response.Body.Close()
			if readErr != nil {
				lastErr = readErr
			} else if response.StatusCode >= 200 && response.StatusCode < 300 {
				if err := json.Unmarshal(responseBody, output); err != nil {
					return fmt.Errorf("decode Feishu response: %w", err)
				}
				if code, message := responseError(output); code != 0 {
					return mapError(code, message, response.StatusCode)
				}
				return nil
			} else {
				code, message := parseErrorResponse(responseBody)
				lastErr = mapError(code, message, response.StatusCode)
				if response.StatusCode != http.StatusTooManyRequests && response.StatusCode < 500 {
					return lastErr
				}
			}
		}
		if attempt < c.retries {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 150 * time.Millisecond):
			}
		}
	}
	return lastErr
}

func parseErrorResponse(raw []byte) (int, string) {
	var value struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	_ = json.Unmarshal(raw, &value)
	return value.Code, firstNonEmpty(value.Msg, "request failed")
}

func responseError(output any) (int, string) {
	raw, _ := json.Marshal(output)
	return parseErrorResponse(raw)
}

func mapError(code int, message string, status int) error {
	return &APIError{Code: code, Message: firstNonEmpty(message, http.StatusText(status)), HTTPStatus: status}
}

func isTokenError(code int, status int) bool {
	return status == http.StatusUnauthorized || code == 99991663 || code == 99991664 || code == 99991668
}

func redact(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 6 {
		return "***"
	}
	return value[:3] + "***" + value[len(value)-3:]
}

var _ connector.PlatformConnector = (*Client)(nil)
