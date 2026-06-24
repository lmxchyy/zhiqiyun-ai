package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var errAssetNotFound = errors.New("asset not found")

const defaultPointsAvailable = 959

type stateBackend interface {
	Read() ([]byte, error)
	Write([]byte) error
}

type fileStateBackend struct {
	path string
}

func (b fileStateBackend) Read() ([]byte, error) {
	return os.ReadFile(b.path)
}

func (b fileStateBackend) Write(content []byte) error {
	if err := os.MkdirAll(filepath.Dir(b.path), 0o755); err != nil {
		return err
	}
	return writeFileAtomically(b.path, content)
}

type postgresStateBackend struct {
	db           *sql.DB
	fallbackPath string
}

func (b postgresStateBackend) Read() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := b.ensureSchema(ctx); err != nil {
		return nil, err
	}
	var raw []byte
	err := b.db.QueryRowContext(ctx, `select state from platform_state where id = $1`, "default").Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return b.importFallback(ctx)
	}
	return raw, err
}

func (b postgresStateBackend) Write(content []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := b.ensureSchema(ctx); err != nil {
		return err
	}
	if err := b.ensureProjectionSchema(ctx); err != nil {
		return err
	}
	_, err := b.db.ExecContext(ctx, `
		insert into platform_state (id, state, version, updated_at)
		values ($1, $2::jsonb, 1, now())
		on conflict (id) do update set
			state = excluded.state,
			version = platform_state.version + 1,
			updated_at = now()
	`, "default", string(content))
	if err != nil {
		return err
	}
	return b.syncRuntimeProjections(ctx, content)
}

func (b postgresStateBackend) ensureSchema(ctx context.Context) error {
	_, err := b.db.ExecContext(ctx, `
		create table if not exists platform_state (
			id varchar(50) primary key,
			state jsonb not null,
			version bigint not null default 0,
			updated_at timestamptz not null default now()
		)
	`)
	return err
}

func (b postgresStateBackend) ensureProjectionSchema(ctx context.Context) error {
	_, err := b.db.ExecContext(ctx, runtimeProjectionSchema)
	return err
}

func (b postgresStateBackend) importFallback(ctx context.Context) ([]byte, error) {
	if b.fallbackPath == "" {
		return nil, os.ErrNotExist
	}
	raw, err := os.ReadFile(b.fallbackPath)
	if err != nil {
		return nil, err
	}
	if _, err := b.db.ExecContext(ctx, `
		insert into platform_state (id, state, version, updated_at)
		values ($1, $2::jsonb, 1, now())
		on conflict (id) do nothing
	`, "default", string(raw)); err != nil {
		return nil, err
	}
	return raw, nil
}

type jsonStore struct {
	path    string
	mu      sync.Mutex
	backend stateBackend
}

func newJSONStore(path string) *jsonStore {
	return &jsonStore{path: path, backend: fileStateBackend{path: path}}
}

func newPostgresStore(db *sql.DB, fallbackPath string) *jsonStore {
	return &jsonStore{path: fallbackPath, backend: postgresStateBackend{db: db, fallbackPath: fallbackPath}}
}

func (s *jsonStore) ListGenerationTasks() ([]generationTask, error) {
	data, err := s.load()
	if err != nil {
		return nil, err
	}
	return data.GenerationTasks, nil
}

func (s *jsonStore) ListAssets() ([]asset, error) {
	data, err := s.load()
	if err != nil {
		return nil, err
	}
	return data.Assets, nil
}

func (s *jsonStore) UserAIState(userID string) (userAIState, error) {
	data, err := s.load()
	if err != nil {
		return userAIState{}, err
	}
	return normalizeUserAIState(data.AIState, userID), nil
}

func (s *jsonStore) UpdateUserAIState(userID string, req userAIState) (userAIState, error) {
	var updated userAIState
	err := s.update(func(data *platformData) error {
		updated = normalizeUserAIState(req, userID)
		updated.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		data.AIState = updated
		return nil
	})
	return updated, err
}

func (s *jsonStore) UpdateAssetThumbnails(updates map[string]string) (int, error) {
	if len(updates) == 0 {
		return 0, nil
	}

	updated := 0
	err := s.update(func(data *platformData) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for i := range data.Assets {
			thumbnailURL := updates[data.Assets[i].ID]
			if thumbnailURL == "" || data.Assets[i].ThumbnailURL != "" {
				continue
			}
			data.Assets[i].ThumbnailURL = thumbnailURL
			data.Assets[i].UpdatedAt = now
			if data.Assets[i].Metadata == nil {
				data.Assets[i].Metadata = map[string]any{}
			}
			data.Assets[i].Metadata["thumbnailUrl"] = thumbnailURL
			updated++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return updated, nil
}

func (s *jsonStore) UpdateAssetImageInfo(updates map[string]assetImageInfo) (int, error) {
	if len(updates) == 0 {
		return 0, nil
	}

	updated := 0
	err := s.update(func(data *platformData) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for i := range data.Assets {
			info, ok := updates[data.Assets[i].ID]
			if !ok {
				continue
			}
			changed := false
			if data.Assets[i].Metadata == nil {
				data.Assets[i].Metadata = map[string]any{}
			}
			if info.ThumbnailURL != "" && data.Assets[i].ThumbnailURL == "" {
				data.Assets[i].ThumbnailURL = info.ThumbnailURL
				data.Assets[i].Metadata["thumbnailUrl"] = info.ThumbnailURL
				changed = true
			}
			if info.Width > 0 && info.Height > 0 {
				resolution := fmt.Sprintf("%dx%d", info.Width, info.Height)
				data.Assets[i].Metadata["width"] = info.Width
				data.Assets[i].Metadata["height"] = info.Height
				data.Assets[i].Metadata["resolution"] = resolution
				changed = true
			}
			if changed {
				data.Assets[i].UpdatedAt = now
				updated++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return updated, nil
}

func (s *jsonStore) PointAccount() (pointAccount, error) {
	data, err := s.load()
	if err != nil {
		return pointAccount{}, err
	}
	return pointAccount{
		ID:        "points_000001",
		UserID:    "user_000002",
		Available: pointsAvailable(data),
		Frozen:    0,
	}, nil
}

func (s *jsonStore) AdminData() (adminPlatformData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadAdminLocked()
}

func (s *jsonStore) UpdateUserPassword(userID string, passwordHash string) (adminUser, error) {
	var updated adminUser
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for i := range data.Users {
			if data.Users[i].ID != userID {
				continue
			}
			data.Users[i].PasswordHash = passwordHash
			data.Users[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			updated = data.Users[i]
			return nil
		}
		return fmt.Errorf("user not found: %s", userID)
	})
	return updated, err
}
func (s *jsonStore) CreateAdminCustomer(req adminCustomerMutation) (adminUser, error) {
	var created adminUser
	err := s.updateAdmin(func(data *adminPlatformData) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		created = adminUser{
			ID:         uniqueAdminID("user", userIDs(data.Users)),
			Email:      req.Email,
			Name:       req.Name,
			Role:       fallback(req.Role, "MEMBER"),
			Status:     fallback(req.Status, "ACTIVE"),
			PlanID:     fallback(req.PlanID, "plan_free"),
			ReferredBy: strings.TrimSpace(req.ReferredBy),
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		data.Users = append(data.Users, created)
		data.PointAccounts = append(data.PointAccounts, adminPointAccount{
			ID:        uniqueAdminID("points", pointIDs(data.PointAccounts)),
			UserID:    created.ID,
			Available: req.Available,
		})
		return nil
	})
	return created, err
}

func (s *jsonStore) UpdateAdminCustomer(id string, req adminCustomerMutation) (adminUser, error) {
	var updated adminUser
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for i := range data.Users {
			if data.Users[i].ID != id {
				continue
			}
			if req.Name != "" {
				data.Users[i].Name = req.Name
			}
			if req.Email != "" {
				data.Users[i].Email = req.Email
			}
			if req.Role != "" {
				data.Users[i].Role = req.Role
			}
			if req.Status != "" {
				data.Users[i].Status = req.Status
			}
			if req.PlanID != "" {
				data.Users[i].PlanID = req.PlanID
			}
			data.Users[i].ReferredBy = strings.TrimSpace(req.ReferredBy)
			data.Users[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			updated = data.Users[i]
			if req.Available >= 0 {
				setPointAccount(data, id, req.Available)
			}
			return nil
		}
		return fmt.Errorf("customer not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) CreateAdminChannelAgent(req adminChannelCreateMutation) (adminChannelAgent, adminUser, error) {
	var createdAgent adminChannelAgent
	var createdUser adminUser
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for _, user := range data.Users {
			if strings.EqualFold(user.Email, req.Email) {
				return fmt.Errorf("email already exists: %s", req.Email)
			}
		}
		if req.Level == 2 {
			foundParent := false
			for _, agent := range data.ChannelAgents {
				if agent.ID == req.ParentID {
					foundParent = true
					break
				}
			}
			if !foundParent {
				return fmt.Errorf("parent channel agent not found: %s", req.ParentID)
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		role := "AGENT_L1"
		if req.Level == 2 {
			role = "AGENT_L2"
		}
		createdUser = adminUser{
			ID:        uniqueAdminID("user", userIDs(data.Users)),
			Email:     req.Email,
			Name:      req.Name,
			Role:      role,
			Status:    fallback(req.Status, "ACTIVE"),
			PlanID:    "plan_free",
			CreatedAt: now,
			UpdatedAt: now,
		}
		createdAgent = adminChannelAgent{
			ID:         uniqueAdminID("channel", channelAgentIDs(data.ChannelAgents)),
			UserID:     createdUser.ID,
			ParentID:   req.ParentID,
			Level:      req.Level,
			Status:     fallback(req.Status, "ACTIVE"),
			InviteCode: req.InviteCode,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if createdAgent.InviteCode == "" {
			createdAgent.InviteCode = strings.ToUpper("AG" + fmtSix(len(data.ChannelAgents)+1))
		}
		data.Users = append(data.Users, createdUser)
		data.ChannelAgents = append(data.ChannelAgents, createdAgent)
		data.PointAccounts = append(data.PointAccounts, adminPointAccount{
			ID:        uniqueAdminID("points", pointIDs(data.PointAccounts)),
			UserID:    createdUser.ID,
			Available: req.Available,
		})
		return nil
	})
	return createdAgent, createdUser, err
}

func (s *jsonStore) UpdateAdminChannelAgent(id string, req adminChannelMutation) (adminChannelAgent, error) {
	var updated adminChannelAgent
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for i := range data.ChannelAgents {
			if data.ChannelAgents[i].ID != id {
				continue
			}
			data.ChannelAgents[i].Status = fallback(req.Status, data.ChannelAgents[i].Status)
			data.ChannelAgents[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			updated = data.ChannelAgents[i]
			return nil
		}
		return fmt.Errorf("channel agent not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) UpdateAdminProduct(id string, req adminProductMutation) (adminProduct, error) {
	var updated adminProduct
	err := s.updateAdmin(func(data *adminPlatformData) error {
		data.AdminProducts = productsWithUsage(*data)
		for i := range data.AdminProducts {
			if data.AdminProducts[i].ID != id {
				continue
			}
			if req.Name != "" {
				data.AdminProducts[i].Name = req.Name
			}
			if req.Type != "" {
				data.AdminProducts[i].Type = req.Type
			}
			if req.Status != "" {
				data.AdminProducts[i].Status = req.Status
			}
			if len(req.Entitlements) > 0 {
				data.AdminProducts[i].Entitlements = req.Entitlements
			}
			updated = data.AdminProducts[i]
			return nil
		}
		return fmt.Errorf("product not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) UpdateAdminPlan(id string, req adminPlanMutation) (adminPlan, error) {
	var updated adminPlan
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for i := range data.Plans {
			if data.Plans[i].ID != id {
				continue
			}
			if req.Name != "" {
				data.Plans[i].Name = req.Name
			}
			if req.PriceCents >= 0 {
				data.Plans[i].Price = req.PriceCents
				data.Plans[i].PriceCents = req.PriceCents
			}
			if req.GrantPoints >= 0 {
				data.Plans[i].Points = req.GrantPoints
				data.Plans[i].GrantPoints = req.GrantPoints
			}
			if req.DurationDays > 0 {
				data.Plans[i].DurationDays = req.DurationDays
			}
			if req.Concurrency > 0 {
				data.Plans[i].Concurrency = req.Concurrency
			}
			data.Plans[i].Active = req.Active
			if req.Entitlements != nil {
				data.Plans[i].Entitlements = req.Entitlements
			}
			updated = data.Plans[i]
			return nil
		}
		return fmt.Errorf("plan not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) CreateAdminOrder(req adminOrderMutation) (adminOrder, error) {
	var created adminOrder
	err := s.updateAdmin(func(data *adminPlatformData) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		created = adminOrder{
			ID:          uniqueAdminID("order", orderIDs(data.Orders)),
			UserID:      req.UserID,
			PlanID:      req.PlanID,
			Amount:      req.AmountCents,
			AmountCents: req.AmountCents,
			Status:      fallback(req.Status, "PENDING"),
			CreatedAt:   now,
		}
		data.Orders = append(data.Orders, created)
		return nil
	})
	return created, err
}

func (s *jsonStore) MarkAdminOrderPaid(id string) (adminOrder, error) {
	var updated adminOrder
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for i := range data.Orders {
			if data.Orders[i].ID != id {
				continue
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			data.Orders[i].Status = "PAID"
			data.Orders[i].PaidAt = now
			updated = data.Orders[i]
			return nil
		}
		return fmt.Errorf("order not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) RenewAdminOrder(id string) (adminOrder, error) {
	var created adminOrder
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for _, order := range data.Orders {
			if order.ID != id {
				continue
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			created = adminOrder{
				ID:          uniqueAdminID("order", orderIDs(data.Orders)),
				UserID:      order.UserID,
				PlanID:      order.PlanID,
				Amount:      orderAmount(order),
				AmountCents: orderAmount(order),
				Status:      "PENDING",
				CreatedAt:   now,
				PriceSnapshot: map[string]any{
					"renewOf": order.ID,
				},
			}
			data.Orders = append(data.Orders, created)
			return nil
		}
		return fmt.Errorf("order not found: %s", id)
	})
	return created, err
}

func (s *jsonStore) UpdateAdminDeliveryProject(id string, req adminDeliveryMutation) (map[string]any, error) {
	var updated map[string]any
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for i := range data.Presentations {
			if data.Presentations[i].ID == id {
				data.Presentations[i].Status = fallback(req.Status, data.Presentations[i].Status)
				data.Presentations[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
				updated = map[string]any{"id": id, "status": data.Presentations[i].Status, "progress": req.Progress}
				return nil
			}
		}
		for i := range data.Agents {
			if data.Agents[i].ID == id {
				data.Agents[i].Status = fallback(req.Status, data.Agents[i].Status)
				data.Agents[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
				updated = map[string]any{"id": id, "status": data.Agents[i].Status, "progress": req.Progress}
				return nil
			}
		}
		return fmt.Errorf("delivery project not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) UpdateAdminSystemSettings(req adminSystemMutation) (adminSystemSettings, error) {
	var updated adminSystemSettings
	err := s.updateAdmin(func(data *adminPlatformData) error {
		if req.Brand.Name != "" {
			data.SystemSettings.Brand = req.Brand
		}
		if len(req.Payments) > 0 {
			data.SystemSettings.Payments = req.Payments
		}
		if len(req.Permissions) > 0 {
			data.SystemSettings.Permissions = req.Permissions
		}
		updated = data.SystemSettings
		return nil
	})
	return updated, err
}

func (s *jsonStore) CreateAdminAPIChannel(req adminAPIChannelMutation) (adminAPIChannel, error) {
	var created adminAPIChannel
	err := s.updateAdmin(func(data *adminPlatformData) error {
		created = adminAPIChannel{
			ID:                      uniqueAdminID("channel_api", apiChannelIDs(data.APIChannels)),
			Name:                    fallback(req.Name, "新上游渠道"),
			BaseURL:                 fallback(req.BaseURL, "https://example.com/v1"),
			Protocol:                fallback(req.Protocol, "openai"),
			ImageRequestMode:        fallback(req.ImageRequestMode, "openai"),
			ImageGenerationEndpoint: fallback(req.ImageGenerationEndpoint, "/v1/images/generations"),
			ImageEditEndpoint:       fallback(req.ImageEditEndpoint, "/v1/images/edits"),
			FetchModelsPath:         fallback(req.FetchModelsPath, "/models"),
			APIKeyEnv:               req.APIKeyEnv,
			ComfyInstances:          req.ComfyInstances,
			Notes:                   req.Notes,
			Primary:                 req.Primary,
			Status:                  fallback(req.Status, "CONFIGURABLE"),
			Priority:                req.Priority,
			Models:                  req.Models,
		}
		if created.Priority == 0 {
			created.Priority = 100
		}
		data.APIChannels = append(data.APIChannels, created)
		return nil
	})
	return created, err
}

func (s *jsonStore) UpdateAdminAPIChannel(id string, req adminAPIChannelMutation) (adminAPIChannel, error) {
	var updated adminAPIChannel
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for i := range data.APIChannels {
			if data.APIChannels[i].ID != id {
				continue
			}
			if req.Name != "" {
				data.APIChannels[i].Name = req.Name
			}
			if req.BaseURL != "" {
				data.APIChannels[i].BaseURL = req.BaseURL
			}
			if req.Protocol != "" {
				data.APIChannels[i].Protocol = req.Protocol
			}
			if req.ImageRequestMode != "" {
				data.APIChannels[i].ImageRequestMode = req.ImageRequestMode
			}
			if req.ImageGenerationEndpoint != "" {
				data.APIChannels[i].ImageGenerationEndpoint = req.ImageGenerationEndpoint
			}
			if req.ImageEditEndpoint != "" {
				data.APIChannels[i].ImageEditEndpoint = req.ImageEditEndpoint
			}
			if req.FetchModelsPath != "" {
				data.APIChannels[i].FetchModelsPath = req.FetchModelsPath
			}
			if req.APIKeyEnv != "" {
				data.APIChannels[i].APIKeyEnv = req.APIKeyEnv
			}
			if len(req.ComfyInstances) > 0 {
				data.APIChannels[i].ComfyInstances = req.ComfyInstances
			}
			if req.Notes != "" {
				data.APIChannels[i].Notes = req.Notes
			}
			data.APIChannels[i].Primary = req.Primary
			if req.Status != "" {
				data.APIChannels[i].Status = req.Status
			}
			if req.Priority > 0 {
				data.APIChannels[i].Priority = req.Priority
			}
			if len(req.Models) > 0 {
				data.APIChannels[i].Models = req.Models
			}
			updated = data.APIChannels[i]
			return nil
		}
		return fmt.Errorf("api channel not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) TestAdminAPIChannel(id string, req adminAPIChannelTestRequest) (map[string]any, error) {
	data, err := s.AdminData()
	if err != nil {
		return nil, err
	}
	for _, item := range data.APIChannels {
		if item.ID == id {
			if strings.TrimSpace(req.APIKey) == "" {
				req.APIKey = savedAPIKeyForChannel(data.APIKeys, item)
			}
			return testAPIChannelConnection(item, req), nil
		}
	}
	return nil, fmt.Errorf("api channel not found: %s", id)
}

func (s *jsonStore) UpdateAdminAPIModel(id string, req adminAPIModelMutation) (adminAPIModel, error) {
	var updated adminAPIModel
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for i := range data.APIModels {
			if data.APIModels[i].ID != id {
				continue
			}
			if req.Name != "" {
				data.APIModels[i].Name = req.Name
			}
			if req.Capability != "" {
				data.APIModels[i].Capability = req.Capability
			}
			if req.BillingMode != "" {
				data.APIModels[i].BillingMode = req.BillingMode
			}
			if req.FixedQuota >= 0 {
				data.APIModels[i].FixedQuota = req.FixedQuota
			}
			if req.ModelRatio > 0 {
				data.APIModels[i].ModelRatio = req.ModelRatio
			}
			if req.CompletionRatio > 0 {
				data.APIModels[i].CompletionRatio = req.CompletionRatio
			}
			if req.Status != "" {
				data.APIModels[i].Status = req.Status
			}
			updated = data.APIModels[i]
			return nil
		}
		return fmt.Errorf("api model not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) CreateAdminAPIKey(req adminAPIKeyMutation) (adminAPIKey, error) {
	var created adminAPIKey
	err := s.updateAdmin(func(data *adminPlatformData) error {
		secret := strings.TrimSpace(firstNonEmptyString(req.Secret, req.APIKey))
		created = adminAPIKey{
			ID:         uniqueAdminID("key", apiKeyIDs(data.APIKeys)),
			Customer:   fallback(req.Customer, "未命名客户"),
			Prefix:     apiKeyPrefix(secret, len(data.APIKeys)+1),
			Secret:     secret,
			Status:     fallback(req.Status, "ACTIVE"),
			Models:     req.Models,
			QuotaLimit: req.QuotaLimit,
		}
		if len(created.Models) == 0 {
			created.Models = []string{"mock-standard", "gpt-image-2"}
		}
		if created.QuotaLimit == 0 {
			created.QuotaLimit = 100000
		}
		data.APIKeys = append(data.APIKeys, created)
		return nil
	})
	return created, err
}

func (s *jsonStore) UpdateAdminAPIKey(id string, req adminAPIKeyMutation) (adminAPIKey, error) {
	var updated adminAPIKey
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for i := range data.APIKeys {
			if data.APIKeys[i].ID != id {
				continue
			}
			if req.Customer != "" {
				data.APIKeys[i].Customer = req.Customer
			}
			if req.Status != "" {
				data.APIKeys[i].Status = req.Status
			}
			if secret := strings.TrimSpace(firstNonEmptyString(req.Secret, req.APIKey)); secret != "" {
				data.APIKeys[i].Secret = secret
				data.APIKeys[i].Prefix = apiKeyPrefix(secret, i+1)
			}
			if len(req.Models) > 0 {
				data.APIKeys[i].Models = req.Models
			}
			if req.QuotaLimit > 0 {
				data.APIKeys[i].QuotaLimit = req.QuotaLimit
			}
			updated = data.APIKeys[i]
			return nil
		}
		return fmt.Errorf("api key not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) UpdateAdminCustomerGroup(id string, req adminCustomerGroupMutation) (adminCustomerGroup, error) {
	var updated adminCustomerGroup
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for i := range data.CustomerGroups {
			if data.CustomerGroups[i].ID != id {
				continue
			}
			if req.Name != "" {
				data.CustomerGroups[i].Name = req.Name
			}
			if req.Ratio > 0 {
				data.CustomerGroups[i].Ratio = req.Ratio
			}
			if len(req.Models) > 0 {
				data.CustomerGroups[i].Models = req.Models
			}
			if req.Description != "" {
				data.CustomerGroups[i].Description = req.Description
			}
			updated = data.CustomerGroups[i]
			return nil
		}
		return fmt.Errorf("customer group not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) CreateAdminCommission(req adminCommissionMutation) (adminCommission, error) {
	var created adminCommission
	err := s.updateAdmin(func(data *adminPlatformData) error {
		if req.OrderID == "" || req.AgentID == "" || req.AmountCents <= 0 {
			return errors.New("orderId, agentId and positive amountCents are required")
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		created = adminCommission{
			ID:          uniqueAdminID("commission", commissionIDs(data.Commissions)),
			OrderID:     req.OrderID,
			AgentID:     req.AgentID,
			AmountCents: req.AmountCents,
			Rate:        req.Rate,
			Status:      fallback(req.Status, "PENDING"),
			RuleSnapshot: map[string]any{
				"source": "manual",
				"rate":   req.Rate,
			},
			CreatedAt: now,
		}
		data.Commissions = append(data.Commissions, created)
		return nil
	})
	return created, err
}

func (s *jsonStore) CreateAdminWithdrawal(req adminWithdrawalMutation) (adminWithdrawal, error) {
	var created adminWithdrawal
	err := s.updateAdmin(func(data *adminPlatformData) error {
		if req.AgentID == "" || req.AmountCents <= 0 {
			return errors.New("agentId and positive amountCents are required")
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		created = adminWithdrawal{
			ID:          uniqueAdminID("withdrawal", withdrawalIDs(data.Withdrawals)),
			AgentID:     req.AgentID,
			AmountCents: req.AmountCents,
			Status:      "PENDING",
			CreatedAt:   now,
		}
		data.Withdrawals = append(data.Withdrawals, created)
		return nil
	})
	return created, err
}

func (s *jsonStore) ReviewAdminWithdrawal(id string, status string) (adminWithdrawal, error) {
	var updated adminWithdrawal
	err := s.updateAdmin(func(data *adminPlatformData) error {
		status = strings.ToUpper(strings.TrimSpace(status))
		if status != "APPROVED" && status != "REJECTED" {
			return fmt.Errorf("invalid withdrawal status: %s", status)
		}
		for i := range data.Withdrawals {
			if data.Withdrawals[i].ID != id {
				continue
			}
			data.Withdrawals[i].Status = status
			data.Withdrawals[i].ReviewedAt = time.Now().UTC().Format(time.RFC3339Nano)
			updated = data.Withdrawals[i]
			return nil
		}
		return fmt.Errorf("withdrawal not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) CreateGenerationTask(req createGenerationTaskRequest) (generationTask, error) {
	var task generationTask
	if err := s.update(func(data *platformData) error {
		count := imageCount(req.Params)
		pointCost := count * modelPointCost(req.Model)
		available := pointsAvailable(*data)
		if available < pointCost {
			return fmt.Errorf("insufficient remaining points: available %d, required %d", available, pointCost)
		}
		taskID := nextID(data.Counters, "task")
		now := time.Now().UTC().Format(time.RFC3339Nano)
		resultIDs := make([]string, 0, count)
		task = generationTask{
			ID:               taskID,
			UserID:           "user_000002",
			Type:             req.Type,
			Prompt:           req.Prompt,
			Params:           req.Params,
			Model:            req.Model,
			Status:           "SUCCEEDED",
			Progress:         100,
			PointCost:        pointCost,
			ResultIDs:        resultIDs,
			CreatedAt:        now,
			UpdatedAt:        now,
			WorkerFinishedAt: now,
		}
		for i := 0; i < count; i++ {
			assetID := nextID(data.Counters, "asset")
			referenceCount := 0
			referenceImages := req.Params["referenceImages"]
			if items, ok := referenceImages.([]any); ok {
				referenceCount = len(items)
			}
			imageURL := promptPreviewImage(req.Prompt)
			contentType := "image/svg+xml"
			source := "local-prompt-preview"
			width := previewImageWidth
			height := previewImageHeight
			thumbnailURL := imageURL
			if i < len(req.GeneratedImages) && req.GeneratedImages[i].URL != "" {
				imageURL = req.GeneratedImages[i].URL
				thumbnailURL = req.GeneratedImages[i].ThumbnailURL
				contentType = req.GeneratedImages[i].ContentType
				source = req.GeneratedImages[i].Source
				width = req.GeneratedImages[i].Width
				height = req.GeneratedImages[i].Height
				if contentType == "" {
					contentType = "image/png"
				}
				if source == "" {
					source = "model-provider"
				}
				if width <= 0 || height <= 0 {
					width = previewImageWidth
					height = previewImageHeight
				}
			}
			if thumbnailURL == "" {
				thumbnailURL = imageURL
			}
			task.ResultIDs = append(task.ResultIDs, assetID)
			data.Assets = append(data.Assets, asset{
				ID:           assetID,
				UserID:       "user_000002",
				TaskID:       taskID,
				Name:         fmt.Sprintf("TEXT_TO_IMAGE-%s-%02d", taskID, i+1),
				MediaType:    "image",
				URL:          imageURL,
				ThumbnailURL: thumbnailURL,
				Favorite:     false,
				Metadata: map[string]any{
					"prompt":          req.Prompt,
					"model":           req.Model,
					"type":            req.Type,
					"sourceType":      req.Type,
					"contentType":     contentType,
					"source":          source,
					"thumbnailUrl":    thumbnailURL,
					"width":           width,
					"height":          height,
					"resolution":      fmt.Sprintf("%dx%d", width, height),
					"index":           i + 1,
					"referenceCount":  referenceCount,
					"referenceImages": referenceImages,
				},
				CreatedAt: now,
				UpdatedAt: now,
			})
		}
		data.GenerationTasks = append(data.GenerationTasks, task)
		nextAvailable := available - pointCost
		data.PointsAvailable = &nextAvailable
		setPlatformPointAccount(data, task.UserID, nextAvailable)
		return nil
	}); err != nil {
		return generationTask{}, err
	}
	return task, nil
}

func modelPointCost(model string) int {
	switch model {
	case "gpt-image-2":
		return 10
	case "mock-standard":
		return 1
	default:
		return 1
	}
}
func setPlatformPointAccount(data *platformData, userID string, available int) {
	for i := range data.PointAccounts {
		if data.PointAccounts[i].UserID == userID {
			data.PointAccounts[i].Available = available
			return
		}
	}
	data.PointAccounts = append(data.PointAccounts, adminPointAccount{
		ID:        uniqueAdminID("points", pointIDs(data.PointAccounts)),
		UserID:    userID,
		Available: available,
	})
}

func (s *jsonStore) DeleteAsset(id string) error {
	return s.update(func(data *platformData) error {
		next := data.Assets[:0]
		deleted := false
		for _, item := range data.Assets {
			if item.ID == id {
				deleted = true
				continue
			}
			next = append(next, item)
		}
		if !deleted {
			return fmt.Errorf("%w: %s", errAssetNotFound, id)
		}
		data.Assets = next
		for i := range data.GenerationTasks {
			resultIDs := data.GenerationTasks[i].ResultIDs[:0]
			for _, resultID := range data.GenerationTasks[i].ResultIDs {
				if resultID != id {
					resultIDs = append(resultIDs, resultID)
				}
			}
			data.GenerationTasks[i].ResultIDs = resultIDs
		}
		return nil
	})
}

func (s *jsonStore) load() (platformData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *jsonStore) save(data platformData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(data)
}

func (s *jsonStore) update(mutator func(*platformData) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked()
	if err != nil {
		return err
	}
	if err := mutator(&data); err != nil {
		return err
	}
	return s.saveLocked(data)
}

func (s *jsonStore) updateAdmin(mutator func(*adminPlatformData) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadAdminLocked()
	if err != nil {
		return err
	}
	if err := mutator(&data); err != nil {
		return err
	}
	return s.saveAdminLocked(data)
}

func (s *jsonStore) loadLocked() (platformData, error) {
	var data platformData
	raw, err := s.backend.Read()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			data.Counters = map[string]int{}
			return data, nil
		}
		return data, err
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return data, err
	}
	if data.Counters == nil {
		data.Counters = map[string]int{}
	}
	if data.PointsAvailable == nil {
		initial := defaultPointsAvailable
		data.PointsAvailable = &initial
	}
	return data, nil
}

func (s *jsonStore) loadAdminLocked() (adminPlatformData, error) {
	var data adminPlatformData
	raw, err := s.backend.Read()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return seedAdminData(), nil
		}
		return data, err
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return data, err
	}
	if len(data.Users) == 0 && len(data.Plans) == 0 && len(data.GenerationTasks) == 0 {
		return seedAdminData(), nil
	}
	if data.Counters == nil {
		data.Counters = map[string]int{}
	}
	if data.PointsAvailable == nil {
		initial := defaultPointsAvailable
		data.PointsAvailable = &initial
	}
	return withAdminDefaults(data), nil
}

func (s *jsonStore) saveLocked(data platformData) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return s.backend.Write(append(raw, '\n'))
}

func (s *jsonStore) saveAdminLocked(data adminPlatformData) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return s.backend.Write(append(raw, '\n'))
}

func writeFileAtomically(path string, content []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return errors.Join(err, removeErr)
		}
		if replaceErr := os.Rename(tmpPath, path); replaceErr != nil {
			return errors.Join(err, replaceErr)
		}
	}
	return nil
}

func nextID(counters map[string]int, name string) string {
	counters[name]++
	return fmt.Sprintf("%s_%06d", name, counters[name])
}

func pointsAvailable(data platformData) int {
	if data.PointsAvailable == nil {
		return defaultPointsAvailable
	}
	return *data.PointsAvailable
}

func imageCount(params map[string]any) int {
	value, ok := params["count"]
	if !ok {
		return 1
	}
	var count int
	switch typed := value.(type) {
	case float64:
		count = int(math.Floor(typed))
	case int:
		count = typed
	case string:
		_, _ = fmt.Sscanf(typed, "%d", &count)
	}
	if count < 1 {
		return 1
	}
	if count > 8 {
		return 8
	}
	return count
}

func normalizeUserAIState(state userAIState, userID string) userAIState {
	if userID == "" {
		userID = "user_000002"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	state.UserID = userID
	state.FavoriteTaskIDs = uniqueNonEmptyStrings(state.FavoriteTaskIDs)
	state.HiddenTaskIDs = uniqueNonEmptyStrings(state.HiddenTaskIDs)
	if len(state.FavoriteCollections) == 0 {
		state.FavoriteCollections = []aiFavoriteCollection{{ID: "default", Name: "默认收藏夹", TaskIDs: []string{}, CreatedAt: now, UpdatedAt: now}}
	}
	defaultCollectionExists := false
	for i := range state.FavoriteCollections {
		if strings.TrimSpace(state.FavoriteCollections[i].ID) == "" {
			state.FavoriteCollections[i].ID = fmt.Sprintf("collection_%06d", i+1)
		}
		if strings.TrimSpace(state.FavoriteCollections[i].Name) == "" {
			state.FavoriteCollections[i].Name = "未命名收藏夹"
		}
		state.FavoriteCollections[i].TaskIDs = uniqueNonEmptyStrings(state.FavoriteCollections[i].TaskIDs)
		if state.FavoriteCollections[i].CreatedAt == "" {
			state.FavoriteCollections[i].CreatedAt = now
		}
		state.FavoriteCollections[i].UpdatedAt = now
		if state.FavoriteCollections[i].ID == state.DefaultCollectionID {
			defaultCollectionExists = true
		}
	}
	if state.DefaultCollectionID == "" || !defaultCollectionExists {
		state.DefaultCollectionID = state.FavoriteCollections[0].ID
	}
	if len(state.AgentConversations) == 0 {
		state.AgentConversations = []aiAgentConversation{{
			ID:        "agent-default",
			Title:     "默认对话",
			CreatedAt: now,
			UpdatedAt: now,
			Messages:  []aiAgentMessage{{Role: "assistant", Content: "我可以根据提示词、参考图和当前参数协助规划生成任务。", CreatedAt: now}},
		}}
	}
	for i := range state.AgentConversations {
		if strings.TrimSpace(state.AgentConversations[i].ID) == "" {
			state.AgentConversations[i].ID = fmt.Sprintf("agent_%06d", i+1)
		}
		if strings.TrimSpace(state.AgentConversations[i].Title) == "" {
			state.AgentConversations[i].Title = "新对话"
		}
		if state.AgentConversations[i].CreatedAt == "" {
			state.AgentConversations[i].CreatedAt = now
		}
		state.AgentConversations[i].UpdatedAt = now
		for j := range state.AgentConversations[i].Messages {
			role := strings.ToLower(strings.TrimSpace(state.AgentConversations[i].Messages[j].Role))
			if role != "user" && role != "assistant" {
				role = "assistant"
			}
			state.AgentConversations[i].Messages[j].Role = role
			if state.AgentConversations[i].Messages[j].CreatedAt == "" {
				state.AgentConversations[i].Messages[j].CreatedAt = now
			}
		}
	}
	if state.ActiveConversationID == "" && len(state.AgentConversations) > 0 {
		state.ActiveConversationID = state.AgentConversations[0].ID
	}
	return state
}

func uniqueNonEmptyStrings(items []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}
