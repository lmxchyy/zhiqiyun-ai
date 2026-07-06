package httpserver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type newAPISyncConfig struct {
	Enabled         bool
	BaseURL         string
	AdminCookie     string
	DefaultGroup    string
	CreateUserPath  string
	CreateTokenPath string
	RechargePath    string
	TimeoutSeconds  int
}

type newAPISyncResult struct {
	Secret       string
	ExternalUser string
	ExternalKey  string
	Created      bool
	Updated      bool
	Raw          map[string]any
}

const (
	newAPIQuotaDisplayScale      = 10000000
	xianzhiImagePointsPer1KImage = 10
	newAPIGPTImage2Price         = 0.05
)

func (s *postgresStore) SyncAdminCustomerNewAPI(id string, req adminNewAPISyncRequest) (adminUserModelRoute, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminUserModelRoute{}, err
	}
	settings, err := s.getSystemSettings(ctx)
	if err != nil {
		return adminUserModelRoute{}, err
	}
	cfg := newAPISyncConfigFromSettings(settings)
	if !cfg.Enabled || strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.AdminCookie) == "" {
		return adminUserModelRoute{}, errors.New("请先在系统治理中配置 NewAPI 管理地址和管理员凭证")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return adminUserModelRoute{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var user adminUser
	if err := tx.QueryRowContext(ctx, `select raw from xz_users where id = $1 for update`, id).Scan(rawScanner(&user)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminUserModelRoute{}, fmt.Errorf("customer not found: %s", id)
		}
		return adminUserModelRoute{}, err
	}
	channels, err := listAPIChannelsForTx(ctx, tx)
	if err != nil {
		return adminUserModelRoute{}, err
	}
	channel := findAPIChannelForRoute(channels, req.ChannelID, "")
	if channel.ID == "" {
		channel = preferredImageBackupChannel(channels)
	}
	if channel.ID == "" {
		return adminUserModelRoute{}, errors.New("请先配置可用的生图渠道")
	}
	models := parseRouteModels(req.Models)
	if len(models) == 0 {
		models = []string{"gpt-image-2"}
	}
	quota := req.QuotaLimit
	if quota <= 0 {
		quota = 100000
	}
	group := strings.TrimSpace(req.GroupName)
	if group == "" {
		group = firstNonEmptyString(cfg.DefaultGroup, "生图备份")
	}
	existingRoute := primaryUserModelRoute(user)
	result, err := syncNewAPIUserKey(ctx, cfg, user, existingRoute, models, group, quota)
	if err != nil {
		return adminUserModelRoute{}, err
	}
	if strings.TrimSpace(result.Secret) == "" && strings.TrimSpace(existingRoute.APIKeyID) == "" {
		return adminUserModelRoute{}, errors.New("NewAPI 已响应，但没有返回可用的用户密钥")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	modelAPIKey := ""
	if newAPIUsableSecret(result.Secret) {
		modelAPIKey = result.Secret
	}
	route, err := applyCustomerModelRouteTx(ctx, tx, user, adminCustomerMutation{
		ModelChannelID:  channel.ID,
		ModelChannel:    channel.Name,
		ModelGroup:      group,
		ModelModels:     strings.Join(models, ","),
		ModelAPIKey:     modelAPIKey,
		ModelKeyStatus:  "ACTIVE",
		ModelQuotaLimit: quota,
	}, now)
	if err != nil {
		return adminUserModelRoute{}, err
	}
	if route.ID == "" {
		return adminUserModelRoute{}, errors.New("NewAPI 密钥已创建，但本地模型路由写入失败")
	}
	route.ExternalKey = firstNonEmptyString(result.ExternalKey, existingRoute.ExternalKey)
	route.ExternalUser = firstNonEmptyString(result.ExternalUser, existingRoute.ExternalUser)
	for i := range user.ModelRoutes {
		if user.ModelRoutes[i].ID == route.ID || strings.EqualFold(user.ModelRoutes[i].GroupName, route.GroupName) {
			route.QuotaUsed = user.ModelRoutes[i].QuotaUsed
			user.ModelRoutes[i] = route
			goto routeUpdated
		}
	}
	user.ModelRoutes = append(user.ModelRoutes, route)
routeUpdated:
	user.UpdatedAt = now
	if err := insertUser(ctx, tx, user); err != nil {
		return adminUserModelRoute{}, err
	}
	if result.ExternalKey != "" || result.ExternalUser != "" {
		route.Provider = firstNonEmptyString(route.Provider, "newapi")
	}
	if err := insertAuditLog(ctx, tx, user.ID, user.Role, "newapi.sync_customer", "user_model_route", route.ID, "", "", 200, map[string]any{
		"group":        group,
		"models":       models,
		"externalKey":  result.ExternalKey,
		"externalUser": result.ExternalUser,
		"created":      result.Created,
		"updated":      result.Updated,
	}); err != nil {
		return adminUserModelRoute{}, err
	}
	return route, tx.Commit()
}

func (s *jsonStore) SyncAdminCustomerNewAPI(id string, req adminNewAPISyncRequest) (adminUserModelRoute, error) {
	var route adminUserModelRoute
	err := s.updateAdmin(func(data *adminPlatformData) error {
		cfg := newAPISyncConfigFromSettings(data.SystemSettings)
		if !cfg.Enabled || strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.AdminCookie) == "" {
			return errors.New("请先在系统治理中配置 NewAPI 管理地址和管理员凭证")
		}
		index := -1
		for i := range data.Users {
			if data.Users[i].ID == id {
				index = i
				break
			}
		}
		if index < 0 {
			return fmt.Errorf("customer not found: %s", id)
		}
		models := parseRouteModels(req.Models)
		if len(models) == 0 {
			models = []string{"gpt-image-2"}
		}
		quota := req.QuotaLimit
		if quota <= 0 {
			quota = 100000
		}
		group := strings.TrimSpace(req.GroupName)
		if group == "" {
			group = firstNonEmptyString(cfg.DefaultGroup, "生图备份")
		}
		existingRoute := primaryUserModelRoute(data.Users[index])
		result, err := syncNewAPIUserKey(context.Background(), cfg, data.Users[index], existingRoute, models, group, quota)
		if err != nil {
			return err
		}
		if strings.TrimSpace(result.Secret) == "" && strings.TrimSpace(existingRoute.APIKeyID) == "" {
			return errors.New("NewAPI 已响应，但没有返回可用的用户密钥")
		}
		channel := preferredImageBackupChannel(data.APIChannels)
		if req.ChannelID != "" {
			for _, item := range data.APIChannels {
				if item.ID == req.ChannelID {
					channel = item
					break
				}
			}
		}
		key := upsertUserModelAPIKey(&data.APIKeys, data.Users[index], quota)
		if newAPIUsableSecret(result.Secret) {
			key.Secret = result.Secret
			key.Prefix = apiKeyPrefix(result.Secret, 1)
		}
		key.Models = mergeStringSet(key.Models, models)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		route = buildUserImageBackupRoute(data.Users[index], channel, key, quota, now)
		route.GroupName = group
		route.Models = models
		route.ExternalKey = firstNonEmptyString(result.ExternalKey, existingRoute.ExternalKey)
		route.ExternalUser = firstNonEmptyString(result.ExternalUser, existingRoute.ExternalUser)
		for i := range data.Users[index].ModelRoutes {
			if data.Users[index].ModelRoutes[i].ID == route.ID || strings.EqualFold(data.Users[index].ModelRoutes[i].GroupName, route.GroupName) {
				data.Users[index].ModelRoutes[i] = route
				return nil
			}
		}
		data.Users[index].ModelRoutes = append(data.Users[index].ModelRoutes, route)
		return nil
	})
	return route, err
}

func (s *postgresStore) syncExistingNewAPIRouteForCustomerUpdate(ctx context.Context, tx *sql.Tx, user adminUser, route adminUserModelRoute) (adminUserModelRoute, error) {
	if strings.TrimSpace(route.ExternalKey) == "" {
		return route, nil
	}
	settings, err := s.getSystemSettings(ctx)
	if err != nil {
		return route, err
	}
	cfg := newAPISyncConfigFromSettings(settings)
	if !cfg.Enabled || strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.AdminCookie) == "" {
		return route, nil
	}
	models := route.Models
	if len(models) == 0 {
		models = []string{"gpt-image-2"}
	}
	group := firstNonEmptyString(route.GroupName, cfg.DefaultGroup, "生图备份")
	quota := route.QuotaLimit
	if quota <= 0 {
		quota = 100000
	}
	result, err := syncNewAPIUserKey(ctx, cfg, user, route, models, group, quota)
	if err != nil {
		return route, err
	}
	route.ExternalKey = firstNonEmptyString(result.ExternalKey, route.ExternalKey)
	route.ExternalUser = firstNonEmptyString(result.ExternalUser, route.ExternalUser)
	if newAPIUsableSecret(result.Secret) && strings.TrimSpace(route.APIKeyID) != "" {
		var key adminAPIKey
		if err := tx.QueryRowContext(ctx, `select raw from xz_api_keys where id = $1 for update`, route.APIKeyID).Scan(rawScanner(&key)); err == nil {
			key.Secret = result.Secret
			key.Prefix = apiKeyPrefix(result.Secret, 1)
			key.Models = mergeStringSet(key.Models, models)
			key.Status = "ACTIVE"
			if key.QuotaLimit < quota {
				key.QuotaLimit = quota
			}
			if err := insertAPIKey(ctx, tx, key); err != nil {
				return route, err
			}
			route.KeyPrefix = key.Prefix
		} else if !errors.Is(err, sql.ErrNoRows) {
			return route, err
		}
	}
	if err := insertAuditLog(ctx, tx, user.ID, user.Role, "newapi.update_customer_route", "user_model_route", route.ID, "", "", 200, map[string]any{
		"group":        group,
		"models":       models,
		"externalKey":  route.ExternalKey,
		"externalUser": route.ExternalUser,
		"updated":      result.Updated,
	}); err != nil {
		return route, err
	}
	return route, nil
}

func syncNewAPIUserKey(ctx context.Context, cfg newAPISyncConfig, user adminUser, existingRoute adminUserModelRoute, models []string, group string, quota int) (newAPISyncResult, error) {
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 30
	}
	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	if strings.TrimSpace(existingRoute.ExternalKey) != "" {
		return updateNewAPITokenQuota(ctx, client, cfg, existingRoute, models, group, quota, false)
	}
	externalUser := ""
	if strings.TrimSpace(cfg.CreateUserPath) != "" {
		payload := map[string]any{
			"username":     fallback(user.Email, user.ID),
			"display_name": fallback(user.Name, user.Email),
			"email":        user.Email,
			"group":        group,
			"quota":        newAPIQuotaFromPoints(quota),
			"status":       1,
		}
		raw, err := postNewAPI(ctx, client, cfg, cfg.CreateUserPath, payload)
		if err != nil {
			return newAPISyncResult{}, err
		}
		externalUser = firstNonEmptyString(stringFromPath(raw, "data.id"), stringFromPath(raw, "id"), stringFromPath(raw, "data.user.id"), user.Email)
	}
	tokenName := "xianzhi-" + shortID(user.ID) + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	payload := map[string]any{
		"name":                 tokenName,
		"group":                group,
		"remain_quota":         newAPIQuotaFromPoints(quota),
		"unlimited_quota":      false,
		"model_limits_enabled": true,
		"model_limits":         strings.Join(models, ","),
		"models":               models,
		"status":               1,
		"expired_time":         -1,
	}
	if externalUserID, ok := newAPIIntegerID(externalUser); ok {
		payload["user_id"] = externalUserID
	}
	raw, err := postNewAPI(ctx, client, cfg, cfg.CreateTokenPath, payload)
	if err != nil {
		return newAPISyncResult{}, err
	}
	secret := newAPISecretFromRaw(raw)
	externalKey := firstNonEmptyString(stringFromPath(raw, "data.id"), stringFromPath(raw, "id"), stringFromPath(raw, "data.token_id"))
	if secret == "" {
		foundSecret, foundID, err := findNewAPITokenKeyByName(ctx, client, cfg, tokenName)
		if err != nil {
			return newAPISyncResult{}, err
		}
		secret = foundSecret
		externalKey = firstNonEmptyString(externalKey, foundID)
	}
	if secret != "" && !strings.HasPrefix(secret, "sk-") {
		secret = "sk-" + secret
	}
	return newAPISyncResult{Secret: secret, ExternalUser: externalUser, ExternalKey: externalKey, Created: true, Raw: raw}, nil
}

func addNewAPIQuotaForRoute(ctx context.Context, cfg newAPISyncConfig, user adminUser, route adminUserModelRoute, pointsDelta int) (newAPISyncResult, error) {
	if pointsDelta <= 0 {
		return newAPISyncResult{}, nil
	}
	models := route.Models
	if len(models) == 0 {
		models = []string{"gpt-image-2"}
	}
	group := firstNonEmptyString(route.GroupName, cfg.DefaultGroup, "生图备份")
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 30
	}
	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	if strings.TrimSpace(route.ExternalKey) != "" {
		return updateNewAPITokenQuota(ctx, client, cfg, route, models, group, pointsDelta, true)
	}
	return syncNewAPIUserKey(ctx, cfg, user, route, models, group, route.QuotaLimit)
}

func updateNewAPITokenQuota(ctx context.Context, client *http.Client, cfg newAPISyncConfig, route adminUserModelRoute, models []string, group string, points int, add bool) (newAPISyncResult, error) {
	externalKey := strings.TrimSpace(route.ExternalKey)
	tokenID, ok := newAPIIntegerID(externalKey)
	if !ok {
		return newAPISyncResult{}, fmt.Errorf("NewAPI token id 无效: %s", externalKey)
	}
	detail, err := getNewAPI(ctx, client, cfg, "/api/token/"+strconv.Itoa(tokenID))
	if err != nil {
		return newAPISyncResult{}, err
	}
	token := newAPITokenObject(detail)
	payload := map[string]any{}
	for key, value := range token {
		payload[key] = value
	}
	if len(payload) == 0 {
		payload["name"] = "xianzhi-" + shortID(route.ID)
	}
	targetQuota := newAPIQuotaFromPoints(points)
	if add {
		targetQuota += int64Value(token["remain_quota"])
	}
	payload["id"] = tokenID
	payload["group"] = group
	payload["remain_quota"] = targetQuota
	payload["unlimited_quota"] = false
	payload["model_limits_enabled"] = true
	payload["model_limits"] = strings.Join(models, ",")
	payload["models"] = models
	if _, exists := payload["status"]; !exists {
		payload["status"] = 1
	}
	if _, exists := payload["expired_time"]; !exists {
		payload["expired_time"] = -1
	}
	raw, err := putNewAPI(ctx, client, cfg, cfg.CreateTokenPath, payload)
	if err != nil {
		return newAPISyncResult{}, err
	}
	secret := firstNonEmptyString(newAPISecretFromRaw(raw), stringValue(token["key"]), stringValue(token["token"]), stringValue(token["secret"]))
	if secret != "" && !strings.HasPrefix(secret, "sk-") {
		secret = "sk-" + secret
	}
	return newAPISyncResult{Secret: secret, ExternalKey: strconv.Itoa(tokenID), Updated: true, Raw: raw}, nil
}

func newAPIQuotaFromPoints(points int) int64 {
	if points <= 0 {
		return 0
	}
	return int64(float64(points) / xianzhiImagePointsPer1KImage * newAPIGPTImage2Price * newAPIQuotaDisplayScale)
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case json.Number:
		output, _ := typed.Int64()
		return output
	case string:
		output, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return output
	default:
		return 0
	}
}

func newAPIUsableSecret(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.Contains(value, "*")
}

func newAPISecretFromRaw(raw map[string]any) string {
	return firstNonEmptyString(
		stringFromPath(raw, "data.key"),
		stringFromPath(raw, "data.token"),
		stringFromPath(raw, "data.secret"),
		stringFromPath(raw, "key"),
		stringFromPath(raw, "token"),
		stringFromPath(raw, "secret"),
	)
}

func findNewAPITokenKeyByName(ctx context.Context, client *http.Client, cfg newAPISyncConfig, name string) (string, string, error) {
	raw, err := getNewAPI(ctx, client, cfg, "/api/token/search?keyword="+url.QueryEscape(name))
	if err != nil {
		return "", "", err
	}
	for _, item := range newAPITokenItems(raw) {
		if strings.TrimSpace(stringValue(item["name"])) != name {
			continue
		}
		secret := firstNonEmptyString(stringValue(item["key"]), stringValue(item["token"]), stringValue(item["secret"]))
		id := strings.TrimSpace(fmt.Sprint(item["id"]))
		return secret, id, nil
	}
	return "", "", errors.New("NewAPI 已创建令牌，但创建接口没有返回密钥，按名称搜索也未找到新令牌")
}

func newAPITokenItems(raw map[string]any) []map[string]any {
	candidates := []any{raw["data"]}
	if data, ok := raw["data"].(map[string]any); ok {
		candidates = append(candidates, data["items"], data["tokens"], data["rows"])
	}
	items := []map[string]any{}
	for _, candidate := range candidates {
		switch typed := candidate.(type) {
		case []any:
			for _, item := range typed {
				if row, ok := item.(map[string]any); ok {
					items = append(items, row)
				}
			}
		case map[string]any:
			items = append(items, typed)
		}
	}
	return items
}

func newAPITokenObject(raw map[string]any) map[string]any {
	if data, ok := raw["data"].(map[string]any); ok {
		for _, key := range []string{"token", "item", "data"} {
			if nested, nestedOK := data[key].(map[string]any); nestedOK {
				return nested
			}
		}
		return data
	}
	return raw
}

func newAPIIntegerID(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	id, err := strconv.Atoi(value)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func postNewAPI(ctx context.Context, client *http.Client, cfg newAPISyncConfig, path string, payload map[string]any) (map[string]any, error) {
	return sendNewAPI(ctx, client, cfg, http.MethodPost, path, payload)
}

func putNewAPI(ctx context.Context, client *http.Client, cfg newAPISyncConfig, path string, payload map[string]any) (map[string]any, error) {
	return sendNewAPI(ctx, client, cfg, http.MethodPut, path, payload)
}

func sendNewAPI(ctx context.Context, client *http.Client, cfg newAPISyncConfig, method string, path string, payload map[string]any) (map[string]any, error) {
	if strings.TrimSpace(path) == "" {
		path = "/api/token/"
	}
	endpoint, err := joinNewAPIURL(cfg.BaseURL, path)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	applyNewAPIAdminAuth(req, cfg.AdminCookie)
	req.Header.Set("New-Api-User", "1")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	rawBody, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	_ = json.Unmarshal(rawBody, &raw)
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("NewAPI 管理员凭证已过期或权限不足，请重新填写可用的管理 Token 或重新登录 NewAPI 后台复制最新 Cookie。原始返回 %d: %s", res.StatusCode, strings.TrimSpace(string(rawBody)))
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("NewAPI 管理接口返回 %d: %s", res.StatusCode, strings.TrimSpace(string(rawBody)))
	}
	if ok, hasOK := raw["success"].(bool); hasOK && !ok {
		return nil, fmt.Errorf("NewAPI 管理接口返回失败: %s", strings.TrimSpace(string(rawBody)))
	}
	return raw, nil
}

func fetchNewAPIGroups(ctx context.Context, cfg newAPISyncConfig) ([]string, error) {
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 30
	}
	raw, err := getNewAPI(ctx, &http.Client{Timeout: time.Duration(timeout) * time.Second}, cfg, "/api/group/")
	if err != nil {
		return nil, err
	}
	return extractNewAPIGroupNames(raw), nil
}

func getNewAPI(ctx context.Context, client *http.Client, cfg newAPISyncConfig, path string) (map[string]any, error) {
	endpoint, err := joinNewAPIURL(cfg.BaseURL, path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	applyNewAPIAdminAuth(req, cfg.AdminCookie)
	req.Header.Set("New-Api-User", "1")
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	rawBody, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	_ = json.Unmarshal(rawBody, &raw)
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("NewAPI 管理员凭证已过期或权限不足，请重新填写可用的管理 Token 或重新登录 NewAPI 后台复制最新 Cookie。原始返回 %d: %s", res.StatusCode, strings.TrimSpace(string(rawBody)))
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("NewAPI 管理接口返回 %d: %s", res.StatusCode, strings.TrimSpace(string(rawBody)))
	}
	if ok, hasOK := raw["success"].(bool); hasOK && !ok {
		return nil, fmt.Errorf("NewAPI 管理接口返回失败: %s", strings.TrimSpace(string(rawBody)))
	}
	return raw, nil
}

func extractNewAPIGroupNames(raw map[string]any) []string {
	seen := map[string]bool{}
	groups := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		groups = append(groups, value)
	}
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case string:
			add(typed)
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case map[string]any:
			for _, key := range []string{"name", "group", "id", "key"} {
				if text, ok := typed[key].(string); ok {
					add(text)
					return
				}
			}
		}
	}
	walk(raw["data"])
	return groups
}

func applyNewAPIAdminAuth(req *http.Request, credential string) {
	credential = strings.TrimSpace(credential)
	lower := strings.ToLower(credential)
	if strings.HasPrefix(lower, "bearer ") {
		req.Header.Set("Authorization", credential)
		return
	}
	if strings.HasPrefix(lower, "cookie:") {
		req.Header.Set("Cookie", strings.TrimSpace(credential[len("cookie:"):]))
		return
	}
	if strings.Contains(credential, "=") || strings.Contains(credential, ";") {
		req.Header.Set("Cookie", credential)
		return
	}
	req.Header.Set("Authorization", "Bearer "+credential)
}

func joinNewAPIURL(baseURL string, apiPath string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiPath = strings.TrimSpace(apiPath)
	if baseURL == "" {
		return "", errors.New("NewAPI Base URL is required")
	}
	if apiPath == "" {
		apiPath = "/api/token/"
	}
	if strings.HasPrefix(apiPath, "http://") || strings.HasPrefix(apiPath, "https://") {
		return apiPath, nil
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(apiPath, "/") {
		apiPath = "/" + apiPath
	}
	if strings.HasSuffix(parsed.Path, "/v1") && strings.HasPrefix(apiPath, "/api/") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/v1")
	}
	return strings.TrimRight(parsed.String(), "/") + apiPath, nil
}

func newAPISyncConfigFromSettings(settings adminSystemSettings) newAPISyncConfig {
	raw, _ := settings.APIGateway["newapi"].(map[string]any)
	cfg := newAPISyncConfig{
		Enabled:         boolValue(raw["enabled"]),
		BaseURL:         stringValue(raw["baseUrl"]),
		AdminCookie:     firstNonEmptyString(stringValue(raw["adminCookie"]), stringValue(raw["adminToken"])),
		DefaultGroup:    firstNonEmptyString(stringValue(raw["defaultGroup"]), "生图备份"),
		CreateUserPath:  stringValue(raw["createUserPath"]),
		CreateTokenPath: firstNonEmptyString(stringValue(raw["createTokenPath"]), "/api/token/"),
		RechargePath:    stringValue(raw["rechargePath"]),
		TimeoutSeconds:  intValue(raw["timeoutSeconds"]),
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = stringValue(settings.APIGateway["newapiBaseUrl"])
	}
	if cfg.AdminCookie == "" {
		cfg.AdminCookie = firstNonEmptyString(stringValue(settings.APIGateway["newapiAdminCookie"]), stringValue(settings.APIGateway["newapiAdminToken"]))
	}
	return cfg
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true") || strings.EqualFold(strings.TrimSpace(typed), "enabled") || strings.EqualFold(strings.TrimSpace(typed), "active")
	default:
		return false
	}
}

func mergeSystemSettings(base adminSystemSettings, extra adminSystemSettings) adminSystemSettings {
	if extra.Brand.Name != "" {
		base.Brand = extra.Brand
	}
	if len(extra.Payments) > 0 {
		base.Payments = extra.Payments
	}
	if len(extra.Permissions) > 0 {
		base.Permissions = extra.Permissions
	}
	if len(extra.APIGateway) > 0 {
		base.APIGateway = mergeMap(base.APIGateway, extra.APIGateway)
	}
	return base
}

func mergeMap(base map[string]any, extra map[string]any) map[string]any {
	if base == nil {
		base = map[string]any{}
	}
	for key, value := range extra {
		base[key] = value
	}
	return base
}

func stringFromPath(raw map[string]any, path string) string {
	var current any = raw
	for _, part := range strings.Split(path, ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = m[part]
	}
	if current == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(current))
	if text == "<nil>" {
		return ""
	}
	return text
}
