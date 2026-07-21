package httpserver

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
)

var errAssetNotFound = errors.New("asset not found")

const (
	defaultPointsAvailable     = 959
	pointUnitAmountCents       = 10
	billingMetricImageGenerate = "image.generations"
	billingMetricVideoGenerate = "video.generations"
	billingMetricPPTGenerate   = "ppt.generations"
)

const (
	generationBillingReservedKey                 = "billingReserved"
	generationBillingReservedAtKey               = "billingReservedAt"
	generationBillingReservationPointCostKey     = "billingReservationPointCost"
	generationBillingReservationBalanceBeforeKey = "billingReservationBalanceBefore"
	generationBillingReservationBalanceAfterKey  = "billingReservationBalanceAfter"
	generationBillingRefundedKey                 = "billingRefunded"
	generationBillingRefundedAtKey               = "billingRefundedAt"
	generationBillingRefundBalanceBeforeKey      = "billingRefundBalanceBefore"
	generationBillingRefundBalanceAfterKey       = "billingRefundBalanceAfter"
)

func isVideoGenerationType(taskType string) bool {
	switch strings.ToUpper(strings.TrimSpace(taskType)) {
	case "TEXT_TO_VIDEO", "IMAGE_TO_VIDEO", "VIDEO_TO_VIDEO":
		return true
	default:
		return false
	}
}

func providerTaskPayload(req createGenerationTaskRequest) map[string]any {
	if req.Params != nil {
		if item, ok := req.Params["providerTask"].(map[string]any); ok {
			return item
		}
	}
	if item, ok := req.VideoTask.(map[string]any); ok {
		return item
	}
	if req.VideoTask == nil {
		return nil
	}
	raw, err := json.Marshal(req.VideoTask)
	if err != nil {
		return nil
	}
	var item map[string]any
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil
	}
	return item
}

func generationBillingReservationParams(params map[string]any, now string, pointCost int, before int, after int) map[string]any {
	next := cloneAnyMap(params)
	if next == nil {
		next = map[string]any{}
	}
	next[generationBillingReservedKey] = true
	next[generationBillingReservedAtKey] = now
	next[generationBillingReservationPointCostKey] = pointCost
	next[generationBillingReservationBalanceBeforeKey] = before
	next[generationBillingReservationBalanceAfterKey] = after
	return next
}

func generationTaskBillingReserved(task generationTask) bool {
	if task.Params == nil {
		return false
	}
	return boolValue(task.Params[generationBillingReservedKey])
}

func generationTaskBillingRefunded(task generationTask) bool {
	if task.Params == nil {
		return false
	}
	return boolValue(task.Params[generationBillingRefundedKey])
}

func generationTaskReservedAndActive(task generationTask) bool {
	return generationTaskBillingReserved(task) && !generationTaskBillingRefunded(task)
}

func generationTaskReservedPointCost(task generationTask, fallback int) int {
	if value, ok := generationTaskParamInt(task.Params, generationBillingReservationPointCostKey); ok && value > 0 {
		return value
	}
	return fallback
}

func generationTaskReservationBalances(task generationTask, currentAfter int, pointCost int) (int, int) {
	before, hasBefore := generationTaskParamInt(task.Params, generationBillingReservationBalanceBeforeKey)
	after, hasAfter := generationTaskParamInt(task.Params, generationBillingReservationBalanceAfterKey)
	if hasBefore && hasAfter {
		return before, after
	}
	return currentAfter + pointCost, currentAfter
}

func generationBillingRefundParams(params map[string]any, now string, before int, after int) map[string]any {
	next := cloneAnyMap(params)
	if next == nil {
		next = map[string]any{}
	}
	next[generationBillingRefundedKey] = true
	next[generationBillingRefundedAtKey] = now
	next[generationBillingRefundBalanceBeforeKey] = before
	next[generationBillingRefundBalanceAfterKey] = after
	return next
}

func generationTaskParamInt(params map[string]any, key string) (int, bool) {
	if params == nil {
		return 0, false
	}
	value, ok := params[key]
	if !ok {
		return 0, false
	}
	return intValue(value), true
}

func providerTaskString(req createGenerationTaskRequest, key string) string {
	task := providerTaskPayload(req)
	if task == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(task[key]))
}

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
	return activeAssets(data.Assets), nil
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
			if assetDeleted(data.Assets[i]) {
				continue
			}
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
			if assetDeleted(data.Assets[i]) {
				continue
			}
			info, ok := updates[data.Assets[i].ID]
			if !ok {
				continue
			}
			changed := false
			if data.Assets[i].Metadata == nil {
				data.Assets[i].Metadata = map[string]any{}
			}
			if info.ThumbnailURL != "" && (data.Assets[i].ThumbnailURL == "" || data.Assets[i].ThumbnailURL == data.Assets[i].URL) {
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

func (s *jsonStore) PointAccount(userID string) (pointAccount, error) {
	data, err := s.load()
	if err != nil {
		return pointAccount{}, err
	}
	for _, item := range data.PointAccounts {
		if item.UserID == userID {
			return pointAccount{ID: item.ID, UserID: item.UserID, Available: item.Available, Frozen: item.Frozen, Total: totalPointsForUser(data.BillingEvents, userID, item.Available, item.Frozen)}, nil
		}
	}
	available := pointsAvailableForUser(data, userID)
	return pointAccount{
		ID:        "points_" + shortID(userID),
		UserID:    userID,
		Available: available,
		Frozen:    0,
		Total:     totalPointsForUser(data.BillingEvents, userID, available, 0),
	}, nil
}

func (s *jsonStore) AdminData() (adminPlatformData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.loadAdminLocked()
	if err != nil {
		return data, err
	}
	data.Assets = activeAssets(data.Assets)
	return data, nil
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
			ID:                    uniqueAdminID("user", userIDs(data.Users)),
			Email:                 req.Email,
			Mobile:                strings.TrimSpace(req.Mobile),
			WeChatOpenIDs:         appendUniqueString(nil, req.WeChatOpenID),
			WeChatUnionID:         strings.TrimSpace(req.WeChatUnionID),
			RegistrationSource:    cloneStringMap(req.RegistrationSource),
			Name:                  req.Name,
			Role:                  fallback(req.Role, "MEMBER"),
			Status:                fallback(req.Status, "ACTIVE"),
			PlanID:                fallback(req.PlanID, "plan_free"),
			ReferredBy:            strings.TrimSpace(req.ReferredBy),
			SubscriptionExpiresAt: strings.TrimSpace(req.SubscriptionExpiresAt),
			CreatedAt:             now,
			UpdatedAt:             now,
		}
		data.Users = append(data.Users, created)
		available := 0
		if req.Available != nil {
			available = *req.Available
		}
		return setAdminPointAccountWithLedgerV1(data, created.ID, available, "ADJUSTMENT", "ADMIN_CUSTOMER_CREATE", created.ID, "admin customer initial balance")
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
			if req.Mobile != "" {
				data.Users[i].Mobile = strings.TrimSpace(req.Mobile)
			}
			if req.WeChatOpenID != "" {
				data.Users[i].WeChatOpenIDs = appendUniqueString(data.Users[i].WeChatOpenIDs, req.WeChatOpenID)
			}
			if req.WeChatUnionID != "" {
				data.Users[i].WeChatUnionID = strings.TrimSpace(req.WeChatUnionID)
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
			if customerModelRouteRequested(req) {
				route := applyCustomerModelRoute(data, data.Users[i], req, data.Users[i].UpdatedAt)
				if route.ID != "" {
					data.Users[i].ModelRoutes = upsertUserModelRoute(data.Users[i].ModelRoutes, route)
				}
			}
			data.Users[i].ReferredBy = strings.TrimSpace(req.ReferredBy)
			data.Users[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			updated = data.Users[i]
			if req.Available != nil {
				if err := setAdminPointAccountWithLedgerV1(data, id, *req.Available, "ADJUSTMENT", "ADMIN_CUSTOMER_UPDATE", id+":"+data.Users[i].UpdatedAt, "admin customer balance adjustment"); err != nil {
					return err
				}
			}
			return nil
		}
		return fmt.Errorf("customer not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) UpdateAdminCustomerIdentity(id string, req adminCustomerIdentityMutation) (adminUser, error) {
	var updated adminUser
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for i := range data.Users {
			if data.Users[i].ID != id {
				continue
			}
			if req.ClearMobile {
				data.Users[i].Mobile = ""
			}
			if req.ClearWeChat {
				data.Users[i].WeChatOpenIDs = nil
				data.Users[i].WeChatUnionID = ""
			}
			if req.Status != "" {
				data.Users[i].Status = strings.ToUpper(strings.TrimSpace(req.Status))
			}
			data.Users[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			updated = data.Users[i]
			return nil
		}
		return fmt.Errorf("user not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) CreateAdminAuthMergeRequest(req adminAuthMergeRequestMutation) (adminAuthMergeRequest, error) {
	req = normalizeAuthMergeRequestMutation(req)
	if req.PrimaryUserID == "" || req.SecondaryUserID == "" {
		return adminAuthMergeRequest{}, errors.New("primaryUserId and secondaryUserId are required")
	}
	if strings.EqualFold(req.PrimaryUserID, req.SecondaryUserID) {
		return adminAuthMergeRequest{}, errors.New("merge request requires two different users")
	}
	var created adminAuthMergeRequest
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for _, item := range data.AuthMergeRequests {
			if sameOpenAuthMergeRequest(item, req) {
				created = item
				return nil
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		created = adminAuthMergeRequest{
			ID:              nextID(data.Counters, "auth_merge"),
			PrimaryUserID:   req.PrimaryUserID,
			SecondaryUserID: req.SecondaryUserID,
			Mobile:          req.Mobile,
			WeChatOpenID:    req.WeChatOpenID,
			WeChatUnionID:   req.WeChatUnionID,
			ConflictCode:    fallback(req.ConflictCode, "AUTH_ACCOUNT_MERGE_REQUIRED"),
			Source:          fallback(req.Source, "auth_conflict"),
			Reason:          req.Reason,
			Status:          fallback(req.Status, "PENDING"),
			CreatedAt:       now,
			UpdatedAt:       now,
			Raw:             cloneAnyMap(req.Raw),
		}
		data.AuthMergeRequests = append(data.AuthMergeRequests, created)
		return nil
	})
	return created, err
}

func (s *jsonStore) ListAdminAuthMergeRequests(userID string) ([]adminAuthMergeRequest, error) {
	data, err := s.AdminData()
	if err != nil {
		return nil, err
	}
	return filterAdminAuthMergeRequests(data.AuthMergeRequests, userID), nil
}

func (s *jsonStore) UpdateAdminAuthMergeRequest(id string, req adminAuthMergeRequestMutation) (adminAuthMergeRequest, error) {
	var updated adminAuthMergeRequest
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for i := range data.AuthMergeRequests {
			if data.AuthMergeRequests[i].ID != id {
				continue
			}
			status := normalizeAuthMergeStatus(req.Status, data.AuthMergeRequests[i].Status)
			if status == "" {
				return errors.New("status is required")
			}
			if !validAuthMergeStatus(status) {
				return errors.New("invalid merge request status")
			}
			data.AuthMergeRequests[i].Status = status
			if req.ReviewComment != "" {
				data.AuthMergeRequests[i].ReviewComment = strings.TrimSpace(req.ReviewComment)
			}
			if req.ResolvedBy != "" {
				data.AuthMergeRequests[i].ResolvedBy = strings.TrimSpace(req.ResolvedBy)
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			if authMergeClosedStatus(status) && data.AuthMergeRequests[i].ResolvedAt == "" {
				data.AuthMergeRequests[i].ResolvedAt = now
			}
			data.AuthMergeRequests[i].UpdatedAt = now
			updated = data.AuthMergeRequests[i]
			return nil
		}
		return fmt.Errorf("auth merge request not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) PreviewAdminAuthMergeRequest(id string, targetUserID string) (adminAuthMergeRequest, adminAuthMergePreviewResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.loadAdminLocked()
	if err != nil {
		return adminAuthMergeRequest{}, adminAuthMergePreviewResult{}, err
	}
	return previewAdminAuthMergeRequestOnData(&data, id, targetUserID)
}

func (s *jsonStore) ExecuteAdminAuthMergeRequest(id string, req adminAuthMergeExecuteRequest) (adminAuthMergeRequest, adminAuthMergeExecuteResult, error) {
	var updated adminAuthMergeRequest
	var result adminAuthMergeExecuteResult
	err := s.updateAdmin(func(data *adminPlatformData) error {
		var err error
		updated, result, err = executeAdminAuthMergeRequestOnData(data, id, req)
		return err
	})
	return updated, result, err
}

func previewAdminAuthMergeRequestOnData(data *adminPlatformData, id string, targetUserID string) (adminAuthMergeRequest, adminAuthMergePreviewResult, error) {
	request, target, source, targetID, sourceID, err := resolveAdminAuthMergeUsers(data, id, targetUserID)
	if err != nil {
		return adminAuthMergeRequest{}, adminAuthMergePreviewResult{}, err
	}
	result := adminAuthMergePreviewResult{
		RequestID:    request.ID,
		TargetUserID: targetID,
		SourceUserID: sourceID,
		Executable:   true,
		Moved:        previewAdminAuthMergeMoved(data, sourceID),
	}
	if authMergeClosedStatus(request.Status) {
		result.Executable = false
		result.Blockers = append(result.Blockers, "merge request is already closed")
	}
	if err := validateUsersForAdminAuthMerge(data, target, source); err != nil {
		result.Executable = false
		result.Blockers = append(result.Blockers, err.Error())
	}
	if countUserCustomerRelations(data.CustomerRelations, sourceID) > 0 {
		result.Warnings = append(result.Warnings, "memory customer relations will be reassigned; PostgreSQL customer relation projection may require a dedicated follow-up if enabled")
	}
	return request, result, nil
}

func executeAdminAuthMergeRequestOnData(data *adminPlatformData, id string, req adminAuthMergeExecuteRequest) (adminAuthMergeRequest, adminAuthMergeExecuteResult, error) {
	if data == nil {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, errors.New("admin data is required")
	}
	if !req.Confirm {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, errors.New("confirm must be true before executing account merge")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, errors.New("merge request id is required")
	}
	requestIndex := -1
	for i := range data.AuthMergeRequests {
		if data.AuthMergeRequests[i].ID == id {
			requestIndex = i
			break
		}
	}
	if requestIndex < 0 {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, fmt.Errorf("auth merge request not found: %s", id)
	}
	request := data.AuthMergeRequests[requestIndex]
	if authMergeClosedStatus(request.Status) {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, errors.New("merge request is already closed")
	}
	targetID := strings.TrimSpace(req.TargetUserID)
	if targetID == "" {
		targetID = request.PrimaryUserID
	}
	sourceID := request.SecondaryUserID
	if targetID == request.SecondaryUserID {
		sourceID = request.PrimaryUserID
	}
	if targetID == "" || sourceID == "" || targetID == sourceID {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, errors.New("merge request requires two different users")
	}
	targetIndex, sourceIndex := -1, -1
	for i := range data.Users {
		if data.Users[i].ID == targetID {
			targetIndex = i
		}
		if data.Users[i].ID == sourceID {
			sourceIndex = i
		}
	}
	if targetIndex < 0 || sourceIndex < 0 {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, errors.New("target or source user not found")
	}
	target, source := data.Users[targetIndex], data.Users[sourceIndex]
	if err := validateUsersForAdminAuthMerge(data, target, source); err != nil {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	moved := map[string]int{}
	warnings := []string{}
	mergeAdminUserIdentity(&target, source, request)
	target.UpdatedAt = now
	source.Mobile = ""
	source.WeChatOpenIDs = nil
	source.WeChatUnionID = ""
	source.ModelRoutes = nil
	source.Status = "MERGED"
	source.UpdatedAt = now
	data.Users[targetIndex], data.Users[sourceIndex] = target, source

	moved["pointAccounts"] = mergePointAccountsForUsers(data, targetID, sourceID)
	moved["tokenRecords"] = replaceUserIDInTokenRecords(data.TokenRecords, sourceID, targetID)
	moved["orders"] = replaceUserIDInOrders(data.Orders, sourceID, targetID)
	moved["channelAgents"] = replaceUserIDInChannelAgents(data.ChannelAgents, sourceID, targetID)
	moved["operationCenters"] = replaceUserIDInOperationCenters(data.OperationCenters, sourceID, targetID)
	moved["generationTasks"] = replaceUserIDInGenerationTasks(data.GenerationTasks, sourceID, targetID)
	moved["assets"] = replaceUserIDInAssets(data.Assets, sourceID, targetID)
	moved["billingEvents"] = replaceUserIDInBillingEvents(data.BillingEvents, sourceID, targetID)
	moved["presentations"] = replaceUserIDInPresentations(data.Presentations, sourceID, targetID)
	moved["agents"] = replaceOwnerIDInAgents(data.Agents, sourceID, targetID)
	moved["agentCalls"] = replaceUserIDInAgentCalls(data.AgentCalls, sourceID, targetID)
	moved["geoBrands"] = replaceOwnerIDInGeoBrands(data.GeoBrands, sourceID, targetID)
	moved["geoTasks"] = replaceOwnerIDInGeoTasks(data.GeoTasks, sourceID, targetID)
	if replaceUserIDInCustomerRelations(data.CustomerRelations, sourceID, targetID) > 0 {
		warnings = append(warnings, "memory customer relations were reassigned; PostgreSQL customer relation projection may require a dedicated follow-up if enabled")
	}
	request.Status = "RESOLVED"
	request.ResolvedBy = fallback(strings.TrimSpace(req.ResolvedBy), "admin")
	request.ReviewComment = fallback(strings.TrimSpace(req.ReviewComment), "人工确认账号合并完成")
	request.ResolvedAt = now
	request.UpdatedAt = now
	if request.Raw == nil {
		request.Raw = map[string]any{}
	}
	request.Raw["executeResult"] = map[string]any{"targetUserId": targetID, "sourceUserId": sourceID, "moved": moved}
	data.AuthMergeRequests[requestIndex] = request
	return request, adminAuthMergeExecuteResult{RequestID: request.ID, TargetUserID: targetID, SourceUserID: sourceID, Moved: moved, Warnings: warnings}, nil
}

func adminAuthMergeRequestIndex(items []adminAuthMergeRequest, id string) int {
	id = strings.TrimSpace(id)
	if id == "" {
		return -1
	}
	for i := range items {
		if items[i].ID == id {
			return i
		}
	}
	return -1
}

func resolveAdminAuthMergeUsers(data *adminPlatformData, id string, targetUserID string) (adminAuthMergeRequest, adminUser, adminUser, string, string, error) {
	if data == nil {
		return adminAuthMergeRequest{}, adminUser{}, adminUser{}, "", "", errors.New("admin data is required")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return adminAuthMergeRequest{}, adminUser{}, adminUser{}, "", "", errors.New("merge request id is required")
	}
	requestIndex := adminAuthMergeRequestIndex(data.AuthMergeRequests, id)
	if requestIndex < 0 {
		return adminAuthMergeRequest{}, adminUser{}, adminUser{}, "", "", fmt.Errorf("auth merge request not found: %s", id)
	}
	request := data.AuthMergeRequests[requestIndex]
	targetID := strings.TrimSpace(targetUserID)
	if targetID == "" {
		targetID = request.PrimaryUserID
	}
	sourceID := request.SecondaryUserID
	if targetID == request.SecondaryUserID {
		sourceID = request.PrimaryUserID
	}
	if targetID == "" || sourceID == "" || targetID == sourceID {
		return adminAuthMergeRequest{}, adminUser{}, adminUser{}, "", "", errors.New("merge request requires two different users")
	}
	targetIndex, sourceIndex := -1, -1
	for i := range data.Users {
		if data.Users[i].ID == targetID {
			targetIndex = i
		}
		if data.Users[i].ID == sourceID {
			sourceIndex = i
		}
	}
	if targetIndex < 0 || sourceIndex < 0 {
		return adminAuthMergeRequest{}, adminUser{}, adminUser{}, "", "", errors.New("target or source user not found")
	}
	return request, data.Users[targetIndex], data.Users[sourceIndex], targetID, sourceID, nil
}

func previewAdminAuthMergeMoved(data *adminPlatformData, sourceID string) map[string]int {
	moved := map[string]int{}
	moved["pointAccounts"] = countPointAccountsForUser(data.PointAccounts, sourceID)
	moved["tokenRecords"] = countTokenRecordsForUser(data.TokenRecords, sourceID)
	moved["orders"] = countOrdersForUser(data.Orders, sourceID)
	moved["channelAgents"] = countUserChannelAgents(data.ChannelAgents, sourceID)
	moved["operationCenters"] = countUserOperationCenters(data.OperationCenters, sourceID)
	moved["generationTasks"] = countGenerationTasksForUser(data.GenerationTasks, sourceID)
	moved["assets"] = countAssetsForUser(data.Assets, sourceID)
	moved["billingEvents"] = countBillingEventsForUser(data.BillingEvents, sourceID)
	moved["presentations"] = countPresentationsForUser(data.Presentations, sourceID)
	moved["agents"] = countAgentsForOwner(data.Agents, sourceID)
	moved["agentCalls"] = countAgentCallsForUser(data.AgentCalls, sourceID)
	moved["geoBrands"] = countGeoBrandsForOwner(data.GeoBrands, sourceID)
	moved["geoTasks"] = countGeoTasksForOwner(data.GeoTasks, sourceID)
	return compactPositiveCounts(moved)
}

func compactPositiveCounts(items map[string]int) map[string]int {
	result := map[string]int{}
	for key, value := range items {
		if value > 0 {
			result[key] = value
		}
	}
	return result
}

func validateUsersForAdminAuthMerge(data *adminPlatformData, target adminUser, source adminUser) error {
	if strings.EqualFold(strings.TrimSpace(target.ID), strings.TrimSpace(source.ID)) {
		return errors.New("cannot merge the same user")
	}
	targetMobile := normalizeMainlandMobile(target.Mobile)
	sourceMobile := normalizeMainlandMobile(source.Mobile)
	if targetMobile != "" && sourceMobile != "" && targetMobile != sourceMobile {
		return errors.New("cannot merge users with different bound mobiles")
	}
	if target.WeChatUnionID != "" && source.WeChatUnionID != "" && !strings.EqualFold(strings.TrimSpace(target.WeChatUnionID), strings.TrimSpace(source.WeChatUnionID)) {
		return errors.New("cannot merge users with different wechat union ids")
	}
	if countUserChannelAgents(data.ChannelAgents, target.ID) > 0 && countUserChannelAgents(data.ChannelAgents, source.ID) > 0 {
		return errors.New("both users have channel agent identities; manual asset merge is required")
	}
	if countUserOperationCenters(data.OperationCenters, target.ID) > 0 && countUserOperationCenters(data.OperationCenters, source.ID) > 0 {
		return errors.New("both users have operation center identities; manual asset merge is required")
	}
	return nil
}

func mergeAdminUserIdentity(target *adminUser, source adminUser, request adminAuthMergeRequest) {
	if normalizeMainlandMobile(target.Mobile) == "" {
		target.Mobile = firstNonEmptyString(normalizeMainlandMobile(source.Mobile), normalizeMainlandMobile(request.Mobile))
	}
	for _, openID := range source.WeChatOpenIDs {
		target.WeChatOpenIDs = appendUniqueString(target.WeChatOpenIDs, openID)
	}
	target.WeChatOpenIDs = appendUniqueString(target.WeChatOpenIDs, request.WeChatOpenID)
	if strings.TrimSpace(target.WeChatUnionID) == "" {
		target.WeChatUnionID = firstNonEmptyString(source.WeChatUnionID, request.WeChatUnionID)
	}
	if (target.MemberLevel == "" || strings.EqualFold(target.MemberLevel, "FREE")) && source.MemberLevel != "" {
		target.MemberLevel = source.MemberLevel
	}
	if (target.PlanID == "" || strings.EqualFold(target.PlanID, "plan_free")) && source.PlanID != "" {
		target.PlanID = source.PlanID
	}
	if later := laterTimeString(target.SubscriptionExpiresAt, source.SubscriptionExpiresAt); later != "" {
		target.SubscriptionExpiresAt = later
	}
	target.ModelRoutes = mergeAdminUserModelRoutes(target.ModelRoutes, source.ModelRoutes)
}

func laterTimeString(a string, b string) string {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	ta, errA := time.Parse(time.RFC3339Nano, a)
	tb, errB := time.Parse(time.RFC3339Nano, b)
	if errA == nil && errB == nil {
		if tb.After(ta) {
			return b
		}
		return a
	}
	if b > a {
		return b
	}
	return a
}

func mergeAdminUserModelRoutes(target []adminUserModelRoute, source []adminUserModelRoute) []adminUserModelRoute {
	seen := map[string]bool{}
	merged := make([]adminUserModelRoute, 0, len(target)+len(source))
	for _, route := range append(append([]adminUserModelRoute{}, target...), source...) {
		key := strings.TrimSpace(route.ID)
		if key == "" {
			key = strings.Join([]string{route.Provider, route.ChannelID, route.APIKeyID, route.GroupName, strings.Join(route.Models, ",")}, "|")
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, route)
	}
	return merged
}

func countUserChannelAgents(items []adminChannelAgent, userID string) int {
	count := 0
	for _, item := range items {
		if item.UserID == userID {
			count++
		}
	}
	return count
}

func countUserOperationCenters(items []adminOperationCenter, userID string) int {
	count := 0
	for _, item := range items {
		if item.UserID == userID {
			count++
		}
	}
	return count
}

func countUserCustomerRelations(items []adminCustomerRelation, userID string) int {
	count := 0
	for _, item := range items {
		if item.CustomerUserID == userID {
			count++
		}
	}
	return count
}

func countPointAccountsForUser(items []adminPointAccount, userID string) int {
	count := 0
	for _, item := range items {
		if item.UserID == userID {
			count++
		}
	}
	return count
}

func countTokenRecordsForUser(items []adminTokenRecord, userID string) int {
	count := 0
	for _, item := range items {
		if item.UserID == userID {
			count++
		}
	}
	return count
}

func countOrdersForUser(items []adminOrder, userID string) int {
	count := 0
	for _, item := range items {
		if item.UserID == userID || item.BuyerUserID == userID || (item.PriceSnapshot != nil && stringValue(item.PriceSnapshot["buyerUserId"]) == userID) {
			count++
		}
	}
	return count
}

func countGenerationTasksForUser(items []generationTask, userID string) int {
	count := 0
	for _, item := range items {
		if item.UserID == userID {
			count++
		}
	}
	return count
}

func countAssetsForUser(items []asset, userID string) int {
	count := 0
	for _, item := range items {
		if item.UserID == userID {
			count++
		}
	}
	return count
}

func countBillingEventsForUser(items []adminBillingEvent, userID string) int {
	count := 0
	for _, item := range items {
		if item.UserID == userID {
			count++
		}
	}
	return count
}

func countPresentationsForUser(items []adminPresentation, userID string) int {
	count := 0
	for _, item := range items {
		if item.UserID == userID {
			count++
		}
	}
	return count
}

func countAgentsForOwner(items []adminAgent, userID string) int {
	count := 0
	for _, item := range items {
		if item.OwnerID == userID {
			count++
		}
	}
	return count
}

func countAgentCallsForUser(items []adminAgentCall, userID string) int {
	count := 0
	for _, item := range items {
		if item.UserID == userID {
			count++
		}
	}
	return count
}

func countGeoBrandsForOwner(items []adminGeoBrand, userID string) int {
	count := 0
	for _, item := range items {
		if item.OwnerID == userID {
			count++
		}
	}
	return count
}

func countGeoTasksForOwner(items []adminGeoTask, userID string) int {
	count := 0
	for _, item := range items {
		if item.OwnerID == userID {
			count++
		}
	}
	return count
}

func mergePointAccountsForUsers(data *adminPlatformData, targetID string, sourceID string) int {
	if data.Counters == nil {
		data.Counters = map[string]int{}
	}
	targetIndex := -1
	for i := range data.PointAccounts {
		if data.PointAccounts[i].UserID == targetID {
			targetIndex = i
			break
		}
	}
	if targetIndex < 0 {
		targetIndex = len(data.PointAccounts)
		data.PointAccounts = append(data.PointAccounts, adminPointAccount{ID: nextID(data.Counters, "points"), UserID: targetID})
	}
	moved := 0
	kept := data.PointAccounts[:0]
	for i := range data.PointAccounts {
		item := data.PointAccounts[i]
		if item.UserID == sourceID {
			data.PointAccounts[targetIndex].Available += item.Available
			data.PointAccounts[targetIndex].Frozen += item.Frozen
			data.PointAccounts[targetIndex].TotalGranted += item.TotalGranted
			data.PointAccounts[targetIndex].TotalUsed += item.TotalUsed
			moved++
			continue
		}
		kept = append(kept, item)
	}
	data.PointAccounts = kept
	return moved
}

func replaceUserIDInTokenRecords(items []adminTokenRecord, sourceID string, targetID string) int {
	moved := 0
	for i := range items {
		if items[i].UserID == sourceID {
			items[i].UserID = targetID
			moved++
		}
	}
	return moved
}

func replaceUserIDInOrders(items []adminOrder, sourceID string, targetID string) int {
	moved := 0
	for i := range items {
		changed := false
		if items[i].UserID == sourceID {
			items[i].UserID = targetID
			changed = true
		}
		if items[i].BuyerUserID == sourceID {
			items[i].BuyerUserID = targetID
			changed = true
		}
		if items[i].PriceSnapshot != nil && stringValue(items[i].PriceSnapshot["buyerUserId"]) == sourceID {
			items[i].PriceSnapshot["buyerUserId"] = targetID
			changed = true
		}
		if changed {
			moved++
		}
	}
	return moved
}

func replaceUserIDInChannelAgents(items []adminChannelAgent, sourceID string, targetID string) int {
	moved := 0
	for i := range items {
		if items[i].UserID == sourceID {
			items[i].UserID = targetID
			items[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			moved++
		}
	}
	return moved
}

func replaceUserIDInOperationCenters(items []adminOperationCenter, sourceID string, targetID string) int {
	moved := 0
	for i := range items {
		if items[i].UserID == sourceID {
			items[i].UserID = targetID
			items[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			moved++
		}
	}
	return moved
}

func replaceUserIDInCustomerRelations(items []adminCustomerRelation, sourceID string, targetID string) int {
	moved := 0
	for i := range items {
		if items[i].CustomerUserID == sourceID {
			items[i].CustomerUserID = targetID
			items[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			moved++
		}
	}
	return moved
}

func replaceUserIDInGenerationTasks(items []generationTask, sourceID string, targetID string) int {
	moved := 0
	for i := range items {
		if items[i].UserID == sourceID {
			items[i].UserID = targetID
			if items[i].TenantID == sourceID {
				items[i].TenantID = targetID
			}
			if strings.EqualFold(items[i].BillingAccountType, "USER") && items[i].BillingAccountID == sourceID {
				items[i].BillingAccountID = targetID
			}
			items[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			moved++
		}
	}
	return moved
}

func replaceUserIDInAssets(items []asset, sourceID string, targetID string) int {
	moved := 0
	for i := range items {
		if items[i].UserID == sourceID {
			items[i].UserID = targetID
			if items[i].TenantID == sourceID {
				items[i].TenantID = targetID
			}
			items[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			moved++
		}
	}
	return moved
}

func replaceUserIDInBillingEvents(items []adminBillingEvent, sourceID string, targetID string) int {
	moved := 0
	for i := range items {
		if items[i].UserID == sourceID {
			items[i].UserID = targetID
			moved++
		}
	}
	return moved
}

func replaceUserIDInPresentations(items []adminPresentation, sourceID string, targetID string) int {
	moved := 0
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range items {
		if items[i].UserID == sourceID {
			items[i].UserID = targetID
			items[i].UpdatedAt = now
			moved++
		}
	}
	return moved
}

func replaceOwnerIDInAgents(items []adminAgent, sourceID string, targetID string) int {
	moved := 0
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range items {
		if items[i].OwnerID == sourceID {
			items[i].OwnerID = targetID
			items[i].UpdatedAt = now
			moved++
		}
	}
	return moved
}

func replaceUserIDInAgentCalls(items []adminAgentCall, sourceID string, targetID string) int {
	moved := 0
	for i := range items {
		if items[i].UserID == sourceID {
			items[i].UserID = targetID
			moved++
		}
	}
	return moved
}

func replaceOwnerIDInGeoBrands(items []adminGeoBrand, sourceID string, targetID string) int {
	moved := 0
	for i := range items {
		if items[i].OwnerID == sourceID {
			items[i].OwnerID = targetID
			moved++
		}
	}
	return moved
}

func replaceOwnerIDInGeoTasks(items []adminGeoTask, sourceID string, targetID string) int {
	moved := 0
	for i := range items {
		if items[i].OwnerID == sourceID {
			items[i].OwnerID = targetID
			moved++
		}
	}
	return moved
}

func mergeUserAIStateValues(target userAIState, source userAIState, targetID string) (userAIState, int) {
	moved := 0
	if len(source.FavoriteTaskIDs) > 0 {
		moved += len(source.FavoriteTaskIDs)
	}
	if len(source.HiddenTaskIDs) > 0 {
		moved += len(source.HiddenTaskIDs)
	}
	target.FavoriteTaskIDs = uniqueNonEmptyStrings(append(target.FavoriteTaskIDs, source.FavoriteTaskIDs...))
	target.HiddenTaskIDs = uniqueNonEmptyStrings(append(target.HiddenTaskIDs, source.HiddenTaskIDs...))

	collectionIndex := map[string]int{}
	for i := range target.FavoriteCollections {
		collectionIndex[target.FavoriteCollections[i].ID] = i
	}
	for _, collection := range source.FavoriteCollections {
		collection.ID = strings.TrimSpace(collection.ID)
		if collection.ID == "" {
			continue
		}
		if index, ok := collectionIndex[collection.ID]; ok {
			target.FavoriteCollections[index].TaskIDs = uniqueNonEmptyStrings(append(target.FavoriteCollections[index].TaskIDs, collection.TaskIDs...))
			if target.FavoriteCollections[index].Name == "" {
				target.FavoriteCollections[index].Name = collection.Name
			}
			moved++
			continue
		}
		target.FavoriteCollections = append(target.FavoriteCollections, collection)
		collectionIndex[collection.ID] = len(target.FavoriteCollections) - 1
		moved++
	}
	if target.DefaultCollectionID == "" {
		target.DefaultCollectionID = source.DefaultCollectionID
	}
	if target.ActiveCollectionID == "" {
		target.ActiveCollectionID = source.ActiveCollectionID
	}

	conversationIDs := map[string]bool{}
	for _, conversation := range target.AgentConversations {
		if conversation.ID != "" {
			conversationIDs[conversation.ID] = true
		}
	}
	conversationIDMap := map[string]string{}
	for _, conversation := range source.AgentConversations {
		conversation.ID = strings.TrimSpace(conversation.ID)
		if conversation.ID == "" {
			continue
		}
		sourceConversationID := conversation.ID
		if conversationIDs[conversation.ID] {
			conversation.ID = uniqueMergedAIConversationID(conversationIDs, targetID, conversation.ID)
		}
		target.AgentConversations = append(target.AgentConversations, conversation)
		conversationIDs[conversation.ID] = true
		conversationIDMap[sourceConversationID] = conversation.ID
		moved++
	}
	if target.ActiveConversationID == "" {
		if mappedID := conversationIDMap[source.ActiveConversationID]; mappedID != "" {
			target.ActiveConversationID = mappedID
		} else {
			target.ActiveConversationID = source.ActiveConversationID
		}
	}
	target.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return normalizeUserAIState(target, targetID), moved
}

func uniqueMergedAIConversationID(existing map[string]bool, targetID string, sourceID string) string {
	prefix := strings.TrimSpace(targetID)
	if prefix == "" {
		prefix = "merged"
	}
	base := prefix + "-" + sourceID
	next := base
	for i := 2; existing[next]; i++ {
		next = fmt.Sprintf("%s-%d", base, i)
	}
	return next
}

func normalizeAuthMergeRequestMutation(req adminAuthMergeRequestMutation) adminAuthMergeRequestMutation {
	req.PrimaryUserID = strings.TrimSpace(req.PrimaryUserID)
	req.SecondaryUserID = strings.TrimSpace(req.SecondaryUserID)
	req.Mobile = normalizeMainlandMobile(req.Mobile)
	req.WeChatOpenID = strings.TrimSpace(req.WeChatOpenID)
	req.WeChatUnionID = strings.TrimSpace(req.WeChatUnionID)
	req.ConflictCode = strings.ToUpper(strings.TrimSpace(req.ConflictCode))
	req.Source = strings.TrimSpace(req.Source)
	req.Reason = strings.TrimSpace(req.Reason)
	req.Status = normalizeAuthMergeStatus(req.Status, "PENDING")
	req.ReviewComment = strings.TrimSpace(req.ReviewComment)
	req.ResolvedBy = strings.TrimSpace(req.ResolvedBy)
	req.Raw = cloneAnyMap(req.Raw)
	return req
}

func normalizeAuthMergeStatus(status string, fallbackValue string) string {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status == "" {
		status = strings.ToUpper(strings.TrimSpace(fallbackValue))
	}
	return status
}

func validAuthMergeStatus(status string) bool {
	switch normalizeAuthMergeStatus(status, "") {
	case "PENDING", "IN_REVIEW", "RESOLVED", "CANCELLED", "REJECTED":
		return true
	default:
		return false
	}
}

func authMergeClosedStatus(status string) bool {
	switch normalizeAuthMergeStatus(status, "") {
	case "RESOLVED", "CANCELLED", "REJECTED":
		return true
	default:
		return false
	}
}

func authMergeOpenStatus(status string) bool {
	switch normalizeAuthMergeStatus(status, "") {
	case "PENDING", "IN_REVIEW":
		return true
	default:
		return false
	}
}

func sameOpenAuthMergeRequest(item adminAuthMergeRequest, req adminAuthMergeRequestMutation) bool {
	if !authMergeOpenStatus(item.Status) {
		return false
	}
	if !sameUserPair(item.PrimaryUserID, item.SecondaryUserID, req.PrimaryUserID, req.SecondaryUserID) {
		return false
	}
	if req.Mobile != "" && normalizeMainlandMobile(item.Mobile) != req.Mobile {
		return false
	}
	if req.WeChatOpenID != "" && !strings.EqualFold(strings.TrimSpace(item.WeChatOpenID), req.WeChatOpenID) {
		return false
	}
	if req.WeChatUnionID != "" && !strings.EqualFold(strings.TrimSpace(item.WeChatUnionID), req.WeChatUnionID) {
		return false
	}
	return true
}

func sameUserPair(a1, b1, a2, b2 string) bool {
	a1, b1, a2, b2 = strings.TrimSpace(a1), strings.TrimSpace(b1), strings.TrimSpace(a2), strings.TrimSpace(b2)
	return (a1 == a2 && b1 == b2) || (a1 == b2 && b1 == a2)
}

func filterAdminAuthMergeRequests(items []adminAuthMergeRequest, userID string) []adminAuthMergeRequest {
	userID = strings.TrimSpace(userID)
	filtered := make([]adminAuthMergeRequest, 0, len(items))
	for _, item := range items {
		if userID == "" || item.PrimaryUserID == userID || item.SecondaryUserID == userID {
			filtered = append(filtered, item)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt > filtered[j].CreatedAt
	})
	return filtered
}

func customerModelRouteRequested(req adminCustomerMutation) bool {
	return strings.TrimSpace(req.ModelChannelID) != "" ||
		strings.TrimSpace(req.ModelChannel) != "" ||
		strings.TrimSpace(req.ModelGroup) != "" ||
		strings.TrimSpace(req.ModelModels) != "" ||
		strings.TrimSpace(req.ModelAPIKey) != "" ||
		strings.TrimSpace(req.ModelKeyStatus) != "" ||
		req.ModelQuotaLimit > 0 ||
		req.ModelRouteEnabled != nil
}

func applyCustomerModelRoute(data *adminPlatformData, user adminUser, req adminCustomerMutation, now string) adminUserModelRoute {
	if data == nil {
		return adminUserModelRoute{}
	}
	channel := findAPIChannelForRoute(data.APIChannels, req.ModelChannelID, req.ModelChannel)
	if channel.ID == "" {
		channel = preferredImageBackupChannel(data.APIChannels)
	}
	if channel.ID == "" {
		return adminUserModelRoute{}
	}
	quota := req.ModelQuotaLimit
	if quota <= 0 {
		quota = 100000
	}
	key := upsertUserModelAPIKey(&data.APIKeys, user, quota)
	if secret := strings.TrimSpace(req.ModelAPIKey); secret != "" {
		key.Secret = secret
		key.Prefix = apiKeyPrefix(secret, 1)
		for i := range data.APIKeys {
			if data.APIKeys[i].ID == key.ID {
				data.APIKeys[i].Secret = key.Secret
				data.APIKeys[i].Prefix = key.Prefix
				break
			}
		}
	}
	models := parseRouteModels(req.ModelModels)
	if len(models) == 0 {
		models = []string{"gpt-image-2"}
	}
	status := strings.ToUpper(strings.TrimSpace(req.ModelKeyStatus))
	if status == "" {
		status = "ACTIVE"
	}
	if req.ModelRouteEnabled != nil && !*req.ModelRouteEnabled {
		status = "DISABLED"
	}
	group := strings.TrimSpace(req.ModelGroup)
	if group == "" {
		group = "生图备份"
	}
	return adminUserModelRoute{
		ID:         "route_" + user.ID + "_image_backup",
		Provider:   "newapi",
		ChannelID:  channel.ID,
		Channel:    fallback(channel.Name, req.ModelChannel),
		APIKeyID:   key.ID,
		KeyPrefix:  key.Prefix,
		GroupName:  group,
		Models:     models,
		QuotaLimit: quota,
		Status:     status,
		UpdatedAt:  now,
	}
}

func upsertUserModelRoute(routes []adminUserModelRoute, route adminUserModelRoute) []adminUserModelRoute {
	for i := range routes {
		if routes[i].ID == route.ID {
			route.QuotaUsed = routes[i].QuotaUsed
			routes[i] = route
			return routes
		}
	}
	return append(routes, route)
}

func findAPIChannelForRoute(channels []adminAPIChannel, channelID string, channelName string) adminAPIChannel {
	channelID = strings.TrimSpace(channelID)
	channelName = strings.TrimSpace(channelName)
	for _, channel := range channels {
		if channelID != "" && channel.ID == channelID {
			return channel
		}
		if channelName != "" && strings.EqualFold(channel.Name, channelName) {
			return channel
		}
	}
	return adminAPIChannel{}
}

func parseRouteModels(value string) []string {
	value = strings.ReplaceAll(value, "，", ",")
	value = strings.ReplaceAll(value, "、", ",")
	parts := strings.Split(value, ",")
	models := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		model := strings.TrimSpace(part)
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		models = append(models, model)
	}
	return models
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
		if strings.TrimSpace(req.ParentID) != "" {
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
		role := agentRoleForLevel(req.Level)
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
		return setAdminPointAccountWithLedgerV1(data, createdUser.ID, req.Available, "ADJUSTMENT", "ADMIN_CHANNEL_CREATE", createdAgent.ID, "admin channel account initial balance")
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
			item := data.ChannelAgents[i]
			if req.Level > 0 {
				item.Level = req.Level
			}
			if strings.TrimSpace(req.ParentID) != "" {
				parentID := fallback(req.ParentID, item.ParentID)
				foundParent := false
				for _, agent := range data.ChannelAgents {
					if agent.ID == parentID && agent.ID != item.ID {
						foundParent = true
						break
					}
				}
				if !foundParent {
					return fmt.Errorf("parent channel agent not found: %s", parentID)
				}
				item.ParentID = parentID
			} else if req.Level > 0 {
				item.ParentID = ""
			} else if req.ParentID != "" {
				item.ParentID = req.ParentID
			}
			if req.Status != "" {
				item.Status = req.Status
			}
			if req.InviteCode != "" {
				item.InviteCode = req.InviteCode
			}
			item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			for j := range data.Users {
				if data.Users[j].ID != item.UserID {
					continue
				}
				if req.Email != "" && !strings.EqualFold(req.Email, data.Users[j].Email) {
					for _, user := range data.Users {
						if user.ID != data.Users[j].ID && strings.EqualFold(user.Email, req.Email) {
							return fmt.Errorf("email already exists: %s", req.Email)
						}
					}
					data.Users[j].Email = req.Email
				}
				if req.Name != "" {
					data.Users[j].Name = req.Name
				}
				data.Users[j].Role = agentRoleForLevel(item.Level)
				if req.Status != "" {
					data.Users[j].Status = req.Status
				}
				data.Users[j].UpdatedAt = item.UpdatedAt
				break
			}
			if req.Available != nil {
				if err := setAdminPointAccountWithLedgerV1(data, item.UserID, *req.Available, "ADJUSTMENT", "ADMIN_CHANNEL_UPDATE", item.ID+":"+item.UpdatedAt, "admin channel account balance adjustment"); err != nil {
					return err
				}
			}
			data.ChannelAgents[i] = item
			updated = item
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
			if req.PriceCents != nil {
				data.Plans[i].Price = *req.PriceCents
				data.Plans[i].PriceCents = *req.PriceCents
			}
			if req.GrantPoints != nil {
				data.Plans[i].Points = *req.GrantPoints
				data.Plans[i].GrantPoints = *req.GrantPoints
			}
			if req.DurationDays != nil {
				data.Plans[i].DurationDays = *req.DurationDays
			}
			if req.Concurrency != nil {
				data.Plans[i].Concurrency = *req.Concurrency
			}
			if req.Active != nil {
				data.Plans[i].Active = *req.Active
			}
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
		tenantID := memoryOrderTenantForUser(*data, req.UserID)
		priceSnapshot := orderPriceSnapshot(req)
		if tenantID != "" {
			priceSnapshot["tenantId"] = tenantID
		}
		created = adminOrder{
			ID:                uniqueAdminID("order", orderIDs(data.Orders)),
			TenantID:          tenantID,
			UserID:            req.UserID,
			BuyerUserID:       req.UserID,
			PlanID:            req.PlanID,
			BusinessOrderType: businessOrderTypeForPlanID(req.PlanID),
			Amount:            req.AmountCents,
			AmountCents:       req.AmountCents,
			Status:            fallback(req.Status, "PENDING"),
			PriceSnapshot:     priceSnapshot,
			CreatedAt:         now,
		}
		data.Orders = append(data.Orders, created)
		return nil
	})
	return created, err
}

func memoryOrderTenantForUser(data adminPlatformData, userID string) string {
	current, found := data.Enterprise.CurrentContexts[userID]
	if !found || !strings.EqualFold(current.Type, contextEnterprise) {
		return ""
	}
	if _, found := memoryTenant(data.Enterprise, current.TenantID); !found {
		return ""
	}
	for _, member := range data.Enterprise.Members {
		if member.TenantID == current.TenantID && member.UserID == userID && strings.EqualFold(member.MemberStatus, "ACTIVE") {
			return current.TenantID
		}
	}
	return ""
}

func orderPriceSnapshot(req adminOrderMutation) map[string]any {
	snapshot := map[string]any{}
	paymentMethod := normalizePaymentMethod(req.PaymentMethod)
	if paymentMethod != "" {
		snapshot["paymentMethod"] = paymentMethod
	}
	if strings.TrimSpace(req.IdempotencyKey) != "" {
		snapshot["idempotencyKey"] = strings.TrimSpace(req.IdempotencyKey)
	}
	if plan, ok := planCatalogByID(req.PlanID); ok && plan.Entitlements != nil {
		planType := planBusinessType(plan)
		snapshot["buyerUserId"] = req.UserID
		snapshot["businessOrderType"] = businessOrderTypeForPlanType(planType)
		snapshot["planName"] = plan.Name
		snapshot["planCode"] = plan.Code
		snapshot["planType"] = planType
		snapshot["productType"] = planType
		snapshot["displayPrice"] = stringValue(plan.Entitlements["displayPrice"])
		snapshot["tokenGrantAmount"] = planTokenGrantAmount(plan)
		snapshot["tokenAmount"] = planTokenGrantAmount(plan)
		snapshot["tokenGrantValueCents"] = planTokenRightsValueCents(plan)
		if level := planMemberLevel(plan); level != "" {
			snapshot["memberLevel"] = level
		}
		if audience := stringValue(plan.Entitlements["audience"]); audience != "" {
			snapshot["audience"] = audience
		}
	}
	if plan, ok := planCatalogByID(req.PlanID); ok && planBusinessType(plan) == planTypeTokenRecharge {
		snapshot["orderType"] = "COMPUTE_RECHARGE"
		snapshot["rechargePoints"] = planPoints(plan)
		snapshot["amountCents"] = planPrice(plan)
	} else if plan, ok := planCatalogByID(req.PlanID); ok && planBusinessType(plan) == planTypeAgentJoinPackage {
		snapshot["orderType"] = orderTypeAgentJoin
		snapshot["grantPoints"] = planTokenGrantAmount(plan)
		snapshot["amountCents"] = planPrice(plan)
	} else if plan, ok := planCatalogByID(req.PlanID); ok && planBusinessType(plan) == planTypeOperationCenterPackage {
		snapshot["orderType"] = orderTypeOperationCenterJoin
		snapshot["grantPoints"] = planTokenGrantAmount(plan)
		snapshot["amountCents"] = planPrice(plan)
	} else if strings.Contains(strings.ToUpper(strings.TrimSpace(req.PlanID)), "RECHARGE") {
		snapshot["orderType"] = "COMPUTE_RECHARGE"
		snapshot["rechargePoints"] = rechargePointsForAmount(req.AmountCents)
	} else if strings.TrimSpace(req.PlanID) != "" {
		snapshot["orderType"] = "PLAN_ORDER"
		if plan, ok := planCatalogByID(req.PlanID); ok {
			snapshot["grantPoints"] = planPoints(plan)
			snapshot["durationDays"] = plan.DurationDays
		}
	}
	if len(snapshot) == 0 {
		return nil
	}
	return snapshot
}

func normalizePaymentMethod(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "cash", "manual", "offline":
		return "cash"
	case "wechat", "wechat_pay", "wxpay":
		return "wechat"
	case "alipay", "ali_pay":
		return "alipay"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func (s *jsonStore) RegisterPaymentCallbackEvent(event adminPaymentEvent) (bool, error) {
	event = normalizePaymentCallbackEvent(event)
	duplicate := false
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for _, existing := range data.PaymentEvents {
			if !samePaymentCallbackProvider(existing, event) {
				continue
			}
			if strings.EqualFold(existing.EventID, event.EventID) {
				if samePaymentCallbackEventTarget(existing, event) {
					duplicate = true
					return nil
				}
				return fmt.Errorf("payment event already belongs to another order: %s", event.EventID)
			}
			if event.TransactionID != "" && strings.EqualFold(existing.TransactionID, event.TransactionID) {
				if samePaymentCallbackEventTarget(existing, event) {
					duplicate = true
					return nil
				}
				return fmt.Errorf("payment transaction already belongs to another order: %s", event.TransactionID)
			}
		}
		data.PaymentEvents = append(data.PaymentEvents, event)
		return nil
	})
	return duplicate, err
}

func (s *jsonStore) MarkAdminOrderPaid(id string, metadata ...map[string]any) (adminOrder, error) {
	var updated adminOrder
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for i := range data.Orders {
			if data.Orders[i].ID != id {
				continue
			}
			mergeOrderPaymentMetadata(&data.Orders[i], metadata...)
			if strings.EqualFold(data.Orders[i].Status, "PAID") {
				now := data.Orders[i].PaidAt
				if now == "" {
					now = time.Now().UTC().Format(time.RFC3339Nano)
				}
				if err := applyCommerceOrderFulfillment(data, &data.Orders[i], now); err != nil {
					return err
				}
				updated = data.Orders[i]
				return nil
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			data.Orders[i].Status = "PAID"
			data.Orders[i].PaidAt = now
			if err := applyCommerceOrderFulfillment(data, &data.Orders[i], now); err != nil {
				return err
			}
			updated = data.Orders[i]
			return nil
		}
		return fmt.Errorf("order not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) RequestOrderRefund(userID string, orderID string, reason string, remark string) (adminOrder, error) {
	var updated adminOrder
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for i := range data.Orders {
			order := &data.Orders[i]
			if order.ID != orderID {
				continue
			}
			if order.UserID != userID && order.BuyerUserID != userID {
				return errors.New("order does not belong to current user")
			}
			if strings.EqualFold(order.Status, "REFUND_REQUESTED") {
				updated = *order
				return nil
			}
			if !isPaidStatus(order.Status) && strings.TrimSpace(order.PaidAt) == "" {
				return errors.New("only paid orders can request a refund")
			}
			if order.PriceSnapshot == nil {
				order.PriceSnapshot = map[string]any{}
			}
			order.PriceSnapshot["refundReason"] = strings.TrimSpace(reason)
			order.PriceSnapshot["refundRemark"] = strings.TrimSpace(remark)
			order.PriceSnapshot["refundRequestedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
			order.Status = "REFUND_REQUESTED"
			updated = *order
			return nil
		}
		return fmt.Errorf("order not found: %s", orderID)
	})
	return updated, err
}

func mergeOrderPaymentMetadata(order *adminOrder, metadata ...map[string]any) {
	if order == nil || len(metadata) == 0 {
		return
	}
	if order.PriceSnapshot == nil {
		order.PriceSnapshot = map[string]any{}
	}
	for _, item := range metadata {
		for key, value := range item {
			key = strings.TrimSpace(key)
			if key == "" || value == nil {
				continue
			}
			order.PriceSnapshot[key] = value
		}
	}
}

func normalizePaymentCallbackEvent(event adminPaymentEvent) adminPaymentEvent {
	event.Provider = normalizePaymentMethod(event.Provider)
	event.EventID = strings.TrimSpace(event.EventID)
	event.OrderID = strings.TrimSpace(event.OrderID)
	event.TransactionID = strings.TrimSpace(event.TransactionID)
	event.TenantID = strings.TrimSpace(event.TenantID)
	if event.IdempotencyKey == "" {
		event.IdempotencyKey = event.Provider + ":" + event.EventID
	}
	if event.Status == "" {
		event.Status = "RECEIVED"
	}
	if event.ID == "" {
		digest := sha256.Sum256([]byte(event.Provider + ":" + event.EventID))
		event.ID = fmt.Sprintf("payevt_%x", digest[:12])
	}
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.Raw == nil {
		event.Raw = map[string]any{}
	}
	return event
}

func samePaymentCallbackProvider(left adminPaymentEvent, right adminPaymentEvent) bool {
	return normalizePaymentMethod(left.Provider) == normalizePaymentMethod(right.Provider)
}

func samePaymentCallbackEventTarget(left adminPaymentEvent, right adminPaymentEvent) bool {
	if strings.TrimSpace(left.OrderID) != strings.TrimSpace(right.OrderID) {
		return false
	}
	if strings.TrimSpace(left.TransactionID) == "" || strings.TrimSpace(right.TransactionID) == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(left.TransactionID), strings.TrimSpace(right.TransactionID))
}

func paymentCallbackEventRaw(req paymentCallbackRequest, signature paymentCallbackSignature) map[string]any {
	return map[string]any{
		"orderId":               req.OrderID,
		"orderNo":               req.OrderNo,
		"provider":              req.Provider,
		"channel":               req.Channel,
		"status":                req.Status,
		"paymentStatus":         req.PaymentStatus,
		"eventType":             req.EventType,
		"paid":                  req.Paid,
		"amountCents":           paymentCallbackAmountCents(req.AmountCents, req.TotalCents, req.Amount),
		"transactionId":         req.TransactionID,
		"providerTransactionId": req.ProviderTxnID,
		"eventId":               req.EventID,
		"signatureVerified":     signature.Verified,
		"signatureProvider":     signature.Provider,
		"signatureSource":       signature.Source,
	}
}

func applyCommerceOrderFulfillment(data *adminPlatformData, order *adminOrder, now string) error {
	if order == nil {
		return nil
	}
	if order.FulfillmentStatus == "FULFILLED" || stringValue(order.PriceSnapshot["fulfillmentStatus"]) == "FULFILLED" {
		if isRechargeOrder(*order) {
			ensurePaidRechargeRoute(data, order)
		}
		return nil
	}
	if isRechargeOrder(*order) {
		applyRechargeSettlement(data, order, now)
		return nil
	}
	plan, ok := planCatalogByID(order.PlanID)
	if !ok {
		return nil
	}
	planType := planBusinessType(plan)
	switch planType {
	case planTypeMemberPackage, planTypeAgentJoinPackage, planTypeOperationCenterPackage:
	default:
		return nil
	}
	ctx := commerceContextForOrder(data, *order, plan)
	result, err := calculateCommissionSettlement(ctx)
	if err != nil {
		return err
	}
	applySettlementToOrder(order, ctx, result, planType)
	if result.TokenGrantAmount > 0 && !tokenRecordExists(data.TokenRecords, order.ID, tokenChangeTypeForPlan(planType)) {
		if err := grantTokensToUser(data, order.UserID, order.ID, tokenChangeTypeForPlan(planType), result.TokenGrantAmount, now); err != nil {
			return err
		}
	}
	for _, commission := range settlementCommissionRecords(ctx, result, now) {
		if !commissionRecordExists(data.Commissions, commission.ID) {
			data.Commissions = append(data.Commissions, commission)
		}
	}
	fulfillIdentityForOrder(data, order, plan, result, now)
	order.FulfillmentStatus = "FULFILLED"
	order.FulfilledAt = now
	order.PriceSnapshot["fulfillmentStatus"] = "FULFILLED"
	order.PriceSnapshot["fulfilledAt"] = now
	return nil
}

func commerceContextForOrder(data *adminPlatformData, order adminOrder, plan adminPlan) commissionOrderContext {
	direct, hasDirect := directActiveAgentForUser(data.Users, data.ChannelAgents, order.UserID)
	parentID := ""
	operationCenterID := ""
	if hasDirect {
		parentID = direct.ParentID
		operationCenterID = direct.OperationCenterID
		if operationCenterID == "" && parentID != "" {
			if parent := agentByIDMap(data.ChannelAgents)[parentID]; parent.ID != "" {
				operationCenterID = parent.OperationCenterID
			}
		}
	}
	orderType := orderTypeForCommerceOrder(planBusinessType(plan), hasDirect, parentID)
	directID := ""
	if hasDirect {
		directID = direct.ID
	}
	return commissionOrderContext{
		OrderID:              order.ID,
		OrderType:            orderType,
		PlanType:             planBusinessType(plan),
		AmountCents:          orderAmount(order),
		BuyerUserID:          order.UserID,
		DirectAgentID:        directID,
		ParentAgentID:        parentID,
		OperationCenterID:    operationCenterID,
		TokenGrantAmount:     planTokenGrantAmount(plan),
		TokenGrantValueCents: planTokenRightsValueCents(plan),
	}
}

func orderTypeForCommerceOrder(planType string, hasDirectAgent bool, parentAgentID string) string {
	switch normalizePlanTypeString(planType) {
	case planTypeAgentJoinPackage:
		return orderTypeAgentJoin
	case planTypeOperationCenterPackage:
		return orderTypeOperationCenterJoin
	case planTypeMemberPackage, planTypeTokenRecharge:
		if !hasDirectAgent {
			return orderTypePlatformDirectRecharge
		}
		if strings.TrimSpace(parentAgentID) != "" {
			return orderTypeUserRechargeSecondLevel
		}
		return orderTypeUserRechargeDirect
	default:
		return ""
	}
}

func grantTokensToUser(data *adminPlatformData, userID string, orderID string, changeType string, amount int, now string) error {
	account, _ := adminPointAccountV1(data, userID)
	after := account.Available + amount
	if err := setAdminPointAccountWithLedgerV1(data, userID, after, "GRANT", "COMMERCE_ORDER", orderID+":"+changeType, "commerce order grant"); err != nil {
		return err
	}
	for i := range data.PointAccounts {
		if data.PointAccounts[i].UserID == userID {
			data.PointAccounts[i].TotalGranted += amount
			break
		}
	}
	data.TokenRecords = append(data.TokenRecords, adminTokenRecord{
		ID:           "token_" + shortID(orderID+"_"+changeType),
		UserID:       userID,
		OrderID:      orderID,
		ChangeType:   changeType,
		Amount:       amount,
		BalanceAfter: after,
		Remark:       "commerce_order_grant",
		CreatedAt:    now,
	})
	return nil
}

func fulfillIdentityForOrder(data *adminPlatformData, order *adminOrder, plan adminPlan, result commissionSettlementResult, now string) {
	planType := planBusinessType(plan)
	for i := range data.Users {
		if data.Users[i].ID != order.UserID {
			continue
		}
		switch planType {
		case planTypeMemberPackage:
			data.Users[i].PlanID = order.PlanID
			data.Users[i].MemberLevel = planMemberLevel(plan)
			if data.Users[i].AgentStatus == "" {
				data.Users[i].AgentStatus = agentStatusNone
			}
			if data.Users[i].OperationCenterStatus == "" {
				data.Users[i].OperationCenterStatus = operationStatusNone
			}
			if plan.DurationDays > 0 {
				paidAt := time.Now().UTC()
				if parsed, parseErr := time.Parse(time.RFC3339Nano, now); parseErr == nil {
					paidAt = parsed.UTC()
				}
				_, expiresAt := membershipExtensionWindow(data.Users[i].SubscriptionExpiresAt, paidAt, int64(plan.DurationDays))
				data.Users[i].SubscriptionExpiresAt = expiresAt.Format(time.RFC3339Nano)
			}
		case planTypeAgentJoinPackage:
			data.Users[i].AgentStatus = agentStatusActive
			if data.Users[i].MemberLevel == "" {
				data.Users[i].MemberLevel = memberLevelFree
			}
			if strings.TrimSpace(data.Users[i].Role) == "" {
				data.Users[i].Role = "MEMBER"
			}
			ensureAgentForUser(data, data.Users[i], order, result, now)
		case planTypeOperationCenterPackage:
			data.Users[i].OperationCenterStatus = operationStatusActive
			if data.Users[i].MemberLevel == "" {
				data.Users[i].MemberLevel = memberLevelFree
			}
			ensureOperationCenterForUser(data, data.Users[i], order, now)
		}
		data.Users[i].UpdatedAt = now
		return
	}
}

func ensureAgentForUser(data *adminPlatformData, user adminUser, order *adminOrder, result commissionSettlementResult, now string) {
	for i := range data.ChannelAgents {
		if data.ChannelAgents[i].UserID == user.ID {
			data.ChannelAgents[i].Status = "ACTIVE"
			data.ChannelAgents[i].JoinOrderID = order.ID
			data.ChannelAgents[i].JoinFeeCents = orderAmount(*order)
			data.ChannelAgents[i].TokenRightsAmount = result.TokenGrantAmount
			data.ChannelAgents[i].UpdatedAt = now
			return
		}
	}
	agentID := uniqueAdminID("channel", channelAgentIDs(data.ChannelAgents))
	inviteCode := strings.ToUpper("AG" + shortID(agentID))
	data.ChannelAgents = append(data.ChannelAgents, adminChannelAgent{
		ID:                agentID,
		UserID:            user.ID,
		ParentID:          order.DirectAgentID,
		OperationCenterID: order.OperationCenterID,
		Level:             2,
		Status:            "ACTIVE",
		InviteCode:        inviteCode,
		JoinOrderID:       order.ID,
		JoinFeeCents:      orderAmount(*order),
		TokenRightsAmount: result.TokenGrantAmount,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
}

func ensureOperationCenterForUser(data *adminPlatformData, user adminUser, order *adminOrder, now string) {
	for i := range data.OperationCenters {
		if data.OperationCenters[i].UserID == user.ID {
			data.OperationCenters[i].Status = "ACTIVE"
			data.OperationCenters[i].JoinOrderID = order.ID
			data.OperationCenters[i].JoinFeeCents = orderAmount(*order)
			data.OperationCenters[i].ApprovedAt = now
			data.OperationCenters[i].UpdatedAt = now
			return
		}
	}
	centerID := uniqueOperationCenterID(data.OperationCenters)
	data.OperationCenters = append(data.OperationCenters, adminOperationCenter{
		ID:           centerID,
		UserID:       user.ID,
		Name:         user.Name + "运营中心",
		InviteCode:   strings.ToUpper("OC" + shortID(centerID)),
		Status:       "ACTIVE",
		JoinOrderID:  order.ID,
		JoinFeeCents: orderAmount(*order),
		ApprovedAt:   now,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
}

func tokenChangeTypeForPlan(planType string) string {
	switch normalizePlanTypeString(planType) {
	case planTypeAgentJoinPackage:
		return "AGENT_JOIN_GRANT"
	case planTypeOperationCenterPackage:
		return "OPERATION_CENTER_GRANT"
	case planTypeTokenRecharge:
		return "USER_RECHARGE_GRANT"
	default:
		return "MEMBER_PACKAGE_GRANT"
	}
}

func firstActiveOperationCenterID(items []adminOperationCenter) string {
	for _, item := range items {
		if strings.EqualFold(item.Status, "ACTIVE") {
			return item.ID
		}
	}
	return ""
}

func tokenRecordExists(items []adminTokenRecord, orderID string, changeType string) bool {
	for _, item := range items {
		if item.OrderID == orderID && strings.EqualFold(item.ChangeType, changeType) {
			return true
		}
	}
	return false
}

func commissionRecordExists(items []adminCommission, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func uniqueOperationCenterID(items []adminOperationCenter) string {
	ids := map[string]bool{}
	for _, item := range items {
		ids[item.ID] = true
	}
	return uniqueAdminID("operation_center", ids)
}

func ensurePaidRechargeRoute(data *adminPlatformData, order *adminOrder) {
	if data == nil || order == nil || !isRechargeOrder(*order) {
		return
	}
	if order.PriceSnapshot != nil && strings.EqualFold(strings.TrimSpace(fmt.Sprint(order.PriceSnapshot["newapiSyncStatus"])), "READY") {
		return
	}
	now := order.PaidAt
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339Nano)
	}
	points := rechargePointsForOrder(*order)
	route := ensureRechargeImageBackupRoute(data, order.UserID, points, now)
	if route.ID == "" {
		return
	}
	if order.PriceSnapshot == nil {
		order.PriceSnapshot = map[string]any{}
	}
	order.PriceSnapshot["orderType"] = "COMPUTE_RECHARGE"
	order.PriceSnapshot["rechargePoints"] = points
	order.PriceSnapshot["newapiSyncStatus"] = "READY"
	order.PriceSnapshot["newapiSyncAmountCents"] = orderAmount(*order)
	order.PriceSnapshot["modelRouteId"] = route.ID
	order.PriceSnapshot["newapiGroup"] = route.GroupName
	order.PriceSnapshot["newapiKeyId"] = route.APIKeyID
}

func applyRechargeSettlement(data *adminPlatformData, order *adminOrder, now string) {
	if order == nil || !isRechargeOrder(*order) {
		return
	}
	points := rechargePointsForOrder(*order)
	if points <= 0 {
		return
	}
	if order.PriceSnapshot == nil {
		order.PriceSnapshot = map[string]any{}
	}
	order.PriceSnapshot["orderType"] = "COMPUTE_RECHARGE"
	order.PriceSnapshot["rechargePoints"] = points
	order.PriceSnapshot["newapiSyncAmountCents"] = orderAmount(*order)
	route := ensureRechargeImageBackupRoute(data, order.UserID, points, now)
	if route.ID != "" {
		order.PriceSnapshot["newapiSyncStatus"] = "READY"
		order.PriceSnapshot["modelRouteId"] = route.ID
		order.PriceSnapshot["newapiGroup"] = route.GroupName
		order.PriceSnapshot["newapiKeyId"] = route.APIKeyID
	} else {
		order.PriceSnapshot["newapiSyncStatus"] = "PENDING"
	}
	if billingEventExists(data.BillingEvents, order.ID, "compute.recharge") {
		return
	}
	pointsByUser := pointMap(data.PointAccounts)
	before := pointsByUser[order.UserID].Available
	after := before + points
	if err := setAdminPointAccountWithLedgerV1(data, order.UserID, after, "RECHARGE", "RECHARGE_ORDER", order.ID, "recharge order credited"); err != nil {
		return
	}
	directAgent, hasDirectAgent := directActiveAgentForUser(data.Users, data.ChannelAgents, order.UserID)
	event := adminBillingEvent{
		ID:              uniqueAdminID("evt", billingEventIDs(data.BillingEvents)),
		TransactionID:   "txn_" + shortID(order.ID),
		UserID:          order.UserID,
		TaskID:          order.ID,
		MetricCode:      "compute.recharge",
		Quantity:        points,
		UnitAmountCents: 10,
		AmountCents:     orderAmount(*order),
		PointCost:       -points,
		BalanceBefore:   before,
		BalanceAfter:    after,
		Model:           "recharge",
		Status:          "SUCCEEDED",
		OccurredAt:      now,
		Metadata: map[string]any{
			"source":        "order_recharge",
			"orderId":       order.ID,
			"newapiSync":    order.PriceSnapshot["newapiSyncStatus"],
			"newapiGroup":   order.PriceSnapshot["newapiGroup"],
			"modelRouteId":  order.PriceSnapshot["modelRouteId"],
			"rechargeCents": orderAmount(*order),
		},
	}
	if hasDirectAgent {
		event.AgentID = directAgent.ID
	}
	data.Commissions = append(data.Commissions, commissionArtifactsForUser(data, order.UserID, order.ID, "COMPUTE_RECHARGE", "compute_recharge", orderAmount(*order), now)...)
	data.BillingEvents = append(data.BillingEvents, event)
}

func ensureRechargeImageBackupRoute(data *adminPlatformData, userID string, quotaLimit int, now string) adminUserModelRoute {
	if data == nil || strings.TrimSpace(userID) == "" {
		return adminUserModelRoute{}
	}
	channel := preferredImageBackupChannel(data.APIChannels)
	if channel.ID == "" {
		return adminUserModelRoute{}
	}
	userIndex := -1
	for i := range data.Users {
		if data.Users[i].ID == userID {
			userIndex = i
			break
		}
	}
	if userIndex < 0 {
		return adminUserModelRoute{}
	}
	key := upsertUserModelAPIKey(&data.APIKeys, data.Users[userIndex], quotaLimit)
	route := buildUserImageBackupRoute(data.Users[userIndex], channel, key, quotaLimit, now)
	routes := data.Users[userIndex].ModelRoutes
	replaced := false
	for i := range routes {
		if routes[i].ID == route.ID || strings.EqualFold(routes[i].GroupName, route.GroupName) {
			route.QuotaUsed = routes[i].QuotaUsed
			if route.QuotaLimit < routes[i].QuotaLimit {
				route.QuotaLimit = routes[i].QuotaLimit
			}
			routes[i] = route
			replaced = true
			break
		}
	}
	if !replaced {
		routes = append(routes, route)
	}
	data.Users[userIndex].ModelRoutes = routes
	return route
}

func preferredImageBackupChannel(channels []adminAPIChannel) adminAPIChannel {
	for _, channel := range channels {
		if !apiChannelUsableForGeneration(channel) {
			continue
		}
		text := strings.ToLower(channel.Name + " " + channel.BaseURL + " " + channel.Notes)
		if strings.Contains(text, "newapi") || strings.Contains(text, "new-api") || strings.Contains(text, "uni-api") || strings.Contains(text, "生图备份") {
			return channel
		}
	}
	for _, channel := range channels {
		if apiChannelUsableForGeneration(channel) {
			return channel
		}
	}
	return adminAPIChannel{}
}

func upsertUserModelAPIKey(keys *[]adminAPIKey, user adminUser, quotaLimit int) adminAPIKey {
	keyID := "key_" + user.ID
	secret := "sk-user-" + shortID(user.ID) + "-newapi-backup"
	customer := fallback(user.Email, user.Name)
	if customer == "" {
		customer = user.ID
	}
	if quotaLimit <= 0 {
		quotaLimit = 100000
	}
	for i := range *keys {
		if (*keys)[i].ID != keyID {
			continue
		}
		(*keys)[i].Customer = customer
		(*keys)[i].Status = "ACTIVE"
		(*keys)[i].Models = mergeStringSet((*keys)[i].Models, []string{"gpt-image-2"})
		if (*keys)[i].QuotaLimit < quotaLimit {
			(*keys)[i].QuotaLimit = quotaLimit
		}
		if (*keys)[i].Secret == "" {
			(*keys)[i].Secret = secret
			(*keys)[i].Prefix = apiKeyPrefix(secret, i+1)
		}
		return (*keys)[i]
	}
	key := adminAPIKey{
		ID:         keyID,
		Customer:   customer,
		Prefix:     apiKeyPrefix(secret, len(*keys)+1),
		Secret:     secret,
		Status:     "ACTIVE",
		Models:     []string{"gpt-image-2"},
		QuotaLimit: quotaLimit,
	}
	*keys = append(*keys, key)
	return key
}

func buildUserImageBackupRoute(user adminUser, channel adminAPIChannel, key adminAPIKey, quotaLimit int, now string) adminUserModelRoute {
	if quotaLimit <= 0 {
		quotaLimit = key.QuotaLimit
	}
	return adminUserModelRoute{
		ID:         "route_" + user.ID + "_image_backup",
		Provider:   "newapi",
		ChannelID:  channel.ID,
		Channel:    fallback(channel.Name, "NewAPI"),
		APIKeyID:   key.ID,
		KeyPrefix:  key.Prefix,
		GroupName:  "生图备份",
		Models:     []string{"gpt-image-2"},
		QuotaLimit: quotaLimit,
		Status:     "ACTIVE",
		UpdatedAt:  now,
	}
}

func mergeStringSet(base []string, extra []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(base)+len(extra))
	for _, value := range append(base, extra...) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func isRechargeOrder(order adminOrder) bool {
	if strings.Contains(strings.ToUpper(strings.TrimSpace(order.PlanID)), "RECHARGE") {
		return true
	}
	if order.PriceSnapshot == nil {
		return false
	}
	for _, key := range []string{"orderType", "type"} {
		if strings.EqualFold(strings.TrimSpace(fmt.Sprint(order.PriceSnapshot[key])), "COMPUTE_RECHARGE") ||
			strings.EqualFold(strings.TrimSpace(fmt.Sprint(order.PriceSnapshot[key])), "RECHARGE") {
			return true
		}
	}
	return false
}

func rechargePointsForOrder(order adminOrder) int {
	if order.PriceSnapshot != nil {
		if points := intValue(order.PriceSnapshot["rechargePoints"]); points > 0 {
			return points
		}
	}
	if plan, ok := rechargePackageByOrder(order); ok {
		return planPoints(plan)
	}
	return rechargePointsForAmount(orderAmount(order))
}

func rechargePointsForAmount(amountCents int) int {
	if amountCents <= 0 {
		return 0
	}
	return amountCents / 10
}

func rechargeAgentForUser(users []adminUser, agents []adminChannelAgent, userID string) (adminChannelAgent, bool) {
	usersByID := userMap(users)
	user := usersByID[userID]
	if strings.TrimSpace(user.ReferredBy) == "" {
		return adminChannelAgent{}, false
	}
	agentsByUserID := agentByUserMap(agents)
	agent, ok := agentsByUserID[user.ReferredBy]
	if !ok || !strings.EqualFold(agent.Status, "ACTIVE") {
		return adminChannelAgent{}, false
	}
	return agent, true
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
				ID:            uniqueAdminID("order", orderIDs(data.Orders)),
				TenantID:      order.TenantID,
				UserID:        order.UserID,
				PlanID:        order.PlanID,
				Amount:        orderAmount(order),
				AmountCents:   orderAmount(order),
				Status:        "PENDING",
				CreatedAt:     now,
				PriceSnapshot: map[string]any{"renewOf": order.ID, "tenantId": order.TenantID},
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
		if len(req.APIGateway) > 0 {
			data.SystemSettings.APIGateway = mergeMap(data.SystemSettings.APIGateway, req.APIGateway)
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
			VideoGenerationEndpoint: req.VideoGenerationEndpoint,
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
			data.APIChannels[i].VideoGenerationEndpoint = req.VideoGenerationEndpoint
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

func (s *jsonStore) UpdateAdminAIModule(code string, req adminAIModuleMutation) (adminAIModule, error) {
	var updated adminAIModule
	err := s.updateAdmin(func(data *adminPlatformData) error {
		*data = normalizeAICapabilityDefaults(*data)
		code = canonicalModuleCode(code)
		for i := range data.AIModules {
			if canonicalModuleCode(data.AIModules[i].ModuleCode) != code {
				continue
			}
			if req.Name != "" {
				data.AIModules[i].Name = req.Name
			}
			if req.Description != "" {
				data.AIModules[i].Description = req.Description
			}
			if req.Status != "" {
				data.AIModules[i].Status = strings.ToUpper(strings.TrimSpace(req.Status))
			}
			if req.OpenTenantIDs != nil {
				data.AIModules[i].OpenTenantIDs = req.OpenTenantIDs
			}
			if req.OpenPackageIDs != nil {
				data.AIModules[i].OpenPackageIDs = req.OpenPackageIDs
			}
			if req.BoundModels != nil {
				data.AIModules[i].BoundModels = req.BoundModels
			}
			if req.DefaultSchemaID != "" {
				data.AIModules[i].DefaultSchemaID = req.DefaultSchemaID
			}
			if req.AllowAgents != nil {
				data.AIModules[i].AllowAgents = *req.AllowAgents
			}
			if req.AllowEndUsers != nil {
				data.AIModules[i].AllowEndUsers = *req.AllowEndUsers
			}
			if req.Config != nil {
				data.AIModules[i].Config = req.Config
			}
			data.AIModules[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			updated = data.AIModules[i]
			return nil
		}
		return fmt.Errorf("ai module not found: %s", code)
	})
	return updated, err
}

func (s *jsonStore) CreateAdminAIModel(req adminAIModelMutation) (adminAIModel, error) {
	var created adminAIModel
	err := s.updateAdmin(func(data *adminPlatformData) error {
		*data = normalizeAICapabilityDefaults(*data)
		modelName := strings.TrimSpace(req.ModelName)
		if modelName == "" {
			return errors.New("model_name is required")
		}
		moduleCode := canonicalModuleCode(req.ModuleCode)
		if moduleCode == "" {
			moduleCode = moduleImageGeneration
		}
		for _, item := range data.AIModels {
			if canonicalModuleCode(item.ModuleCode) == moduleCode && strings.EqualFold(strings.TrimSpace(item.ModelName), modelName) {
				return fmt.Errorf("ai model already exists: %s", modelName)
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		fallbackModel := ""
		if req.FallbackModel != nil {
			fallbackModel = strings.TrimSpace(*req.FallbackModel)
		}
		sortWeight := req.SortWeight
		if sortWeight <= 0 {
			sortWeight = len(data.AIModels)*10 + 10
		}
		modelType := strings.TrimSpace(req.ModelType)
		if modelType == "" {
			modelType = defaultAIModelTypeForModule(moduleCode)
		}
		provider := strings.TrimSpace(req.Provider)
		if provider == "" {
			provider = "NewAPI"
		}
		status := strings.ToUpper(strings.TrimSpace(req.Status))
		if status == "" {
			status = "ACTIVE"
		}
		created = adminAIModel{
			ID:                  uniqueAdminID("ai_model", aiModelIDs(data.AIModels)),
			ModelName:           modelName,
			ModelType:           modelType,
			Provider:            provider,
			CapabilityCode:      uniqueNonEmptyStrings(req.CapabilityCode),
			ModuleCode:          moduleCode,
			Status:              status,
			FallbackModel:       fallbackModel,
			SortWeight:          sortWeight,
			AllowFallbackSwitch: req.AllowFallbackSwitch != nil && *req.AllowFallbackSwitch,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		applyAIModelComplianceMutation(&created, req)
		if err := validateAIModelMiniProgramEnable(created); err != nil {
			return err
		}
		if len(created.CapabilityCode) == 0 {
			created.CapabilityCode = defaultAICapabilitiesForModule(moduleCode)
		}
		data.AIModels = append(data.AIModels, created)
		bindAIModelToModule(data, moduleCode, modelName)
		return nil
	})
	return created, err
}

func (s *jsonStore) UpdateAdminAIModel(id string, req adminAIModelMutation) (adminAIModel, error) {
	var updated adminAIModel
	err := s.updateAdmin(func(data *adminPlatformData) error {
		*data = normalizeAICapabilityDefaults(*data)
		for i := range data.AIModels {
			if data.AIModels[i].ID != id {
				continue
			}
			oldModelName := data.AIModels[i].ModelName
			if req.ModelName != "" {
				nextModelName := strings.TrimSpace(req.ModelName)
				data.AIModels[i].ModelName = nextModelName
				for moduleIndex := range data.AIModules {
					for modelIndex, modelName := range data.AIModules[moduleIndex].BoundModels {
						if strings.EqualFold(strings.TrimSpace(modelName), oldModelName) {
							data.AIModules[moduleIndex].BoundModels[modelIndex] = nextModelName
						}
					}
				}
			}
			if req.ModelType != "" {
				data.AIModels[i].ModelType = req.ModelType
			}
			if req.Provider != "" {
				data.AIModels[i].Provider = req.Provider
			}
			if req.CapabilityCode != nil {
				data.AIModels[i].CapabilityCode = req.CapabilityCode
			}
			if req.ModuleCode != "" {
				data.AIModels[i].ModuleCode = canonicalModuleCode(req.ModuleCode)
			}
			if req.Status != "" {
				data.AIModels[i].Status = strings.ToUpper(strings.TrimSpace(req.Status))
			}
			if req.FallbackModel != nil {
				data.AIModels[i].FallbackModel = strings.TrimSpace(*req.FallbackModel)
			}
			if req.SortWeight > 0 {
				data.AIModels[i].SortWeight = req.SortWeight
			}
			if req.AllowFallbackSwitch != nil {
				data.AIModels[i].AllowFallbackSwitch = *req.AllowFallbackSwitch
			}
			applyAIModelComplianceMutation(&data.AIModels[i], req)
			if err := validateAIModelMiniProgramEnable(data.AIModels[i]); err != nil {
				return err
			}
			data.AIModels[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			updated = data.AIModels[i]
			return nil
		}
		return fmt.Errorf("ai model not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) UpdateAdminAIParameterSchema(id string, req adminAIParameterSchemaMutation) (adminAIParameterSchema, error) {
	var updated adminAIParameterSchema
	err := s.updateAdmin(func(data *adminPlatformData) error {
		*data = normalizeAICapabilityDefaults(*data)
		for i := range data.AIParameterSchemas {
			if data.AIParameterSchemas[i].ID != id {
				continue
			}
			if req.SchemaJSON.Fields != nil {
				data.AIParameterSchemas[i].SchemaJSON = req.SchemaJSON
			}
			if req.Status != "" {
				data.AIParameterSchemas[i].Status = strings.ToUpper(strings.TrimSpace(req.Status))
			}
			data.AIParameterSchemas[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			updated = data.AIParameterSchemas[i]
			return nil
		}
		return fmt.Errorf("ai parameter schema not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) UpdateAdminTenantModuleLimit(id string, req adminTenantModuleLimitMutation) (adminTenantModuleLimit, error) {
	var updated adminTenantModuleLimit
	err := s.updateAdmin(func(data *adminPlatformData) error {
		*data = normalizeAICapabilityDefaults(*data)
		for i := range data.TenantModuleLimits {
			if data.TenantModuleLimits[i].ID != id {
				continue
			}
			if req.TenantID != "" {
				data.TenantModuleLimits[i].TenantID = req.TenantID
			}
			if req.AgentID != "" {
				data.TenantModuleLimits[i].AgentID = req.AgentID
			}
			if req.PackageID != "" {
				data.TenantModuleLimits[i].PackageID = req.PackageID
			}
			if req.ModelName != "" {
				data.TenantModuleLimits[i].ModelName = req.ModelName
			}
			if req.LimitJSON != nil {
				data.TenantModuleLimits[i].LimitJSON = req.LimitJSON
			}
			if req.Status != "" {
				data.TenantModuleLimits[i].Status = strings.ToUpper(strings.TrimSpace(req.Status))
			}
			data.TenantModuleLimits[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			updated = data.TenantModuleLimits[i]
			return nil
		}
		return fmt.Errorf("tenant module limit not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) UpdateAdminPlanCapabilities(planID string, req adminPlanCapabilitiesMutation) error {
	return s.updateAdmin(func(data *adminPlatformData) error {
		return applyAdminPlanCapabilities(data, planID, req)
	})
}

func (s *jsonStore) UpdateAdminBillingRule(id string, req adminBillingRuleMutation) (adminBillingRule, error) {
	var updated adminBillingRule
	err := s.updateAdmin(func(data *adminPlatformData) error {
		draft, err := createBillingRuleDraftInData(data, id, req)
		if err != nil {
			return err
		}
		updated = billingRuleVersionProjection(draft)
		return nil
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

func (s *jsonStore) ReviewAdminCommission(id string, status string) (adminCommission, error) {
	var updated adminCommission
	err := s.updateAdmin(func(data *adminPlatformData) error {
		status = strings.ToUpper(strings.TrimSpace(status))
		if status != "APPROVED" && status != "REJECTED" {
			return fmt.Errorf("invalid commission status: %s", status)
		}
		for i := range data.Commissions {
			if data.Commissions[i].ID != id {
				continue
			}
			data.Commissions[i].Status = status
			if data.Commissions[i].RuleSnapshot == nil {
				data.Commissions[i].RuleSnapshot = map[string]any{}
			}
			data.Commissions[i].RuleSnapshot["reviewedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
			updated = data.Commissions[i]
			return nil
		}
		return fmt.Errorf("commission not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) UpdateMarketingCommissionRule(id string, req adminCommissionRuleMutation) (adminCommissionRule, error) {
	var updated adminCommissionRule
	err := s.updateAdmin(func(data *adminPlatformData) error {
		if len(data.CommissionRules) == 0 {
			data.CommissionRules = defaultCommissionRules()
		}
		for i := range data.CommissionRules {
			if data.CommissionRules[i].ID != id {
				continue
			}
			rule := applyCommissionRuleMutation(data.CommissionRules[i], req)
			data.CommissionRules[i] = rule
			updated = rule
			return nil
		}
		return fmt.Errorf("commission rule not found: %s", id)
	})
	return updated, err
}

func (s *jsonStore) CreateAdminWithdrawal(req adminWithdrawalMutation) (adminWithdrawal, error) {
	var created adminWithdrawal
	err := s.updateAdmin(func(data *adminPlatformData) error {
		if req.AgentID == "" || req.AmountCents <= 0 {
			return errors.New("agentId and positive amountCents are required")
		}
		available := availableWithdrawalCents(data.Commissions, data.Withdrawals, req.AgentID)
		if req.AmountCents > available {
			return fmt.Errorf("可提现余额不足：当前可提现 %s，申请提现 %s", moneyText(available), moneyText(req.AmountCents))
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

func availableWithdrawalCents(commissions []adminCommission, withdrawals []adminWithdrawal, agentID string) int {
	settledCommission := 0
	for _, item := range commissions {
		if item.AgentID != agentID {
			continue
		}
		switch strings.ToUpper(item.Status) {
		case "SETTLED", "PAID", "APPROVED":
			settledCommission += item.AmountCents
		}
	}
	lockedWithdrawal := 0
	for _, item := range withdrawals {
		if item.AgentID != agentID {
			continue
		}
		switch strings.ToUpper(item.Status) {
		case "APPROVED", "PAID", "SETTLED", "PENDING":
			lockedWithdrawal += item.AmountCents
		}
	}
	available := settledCommission - lockedWithdrawal
	if available < 0 {
		return 0
	}
	return available
}

func moneyText(cents int) string {
	return fmt.Sprintf("￥%.2f", float64(cents)/100)
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
		userID := strings.TrimSpace(req.UserID)
		if userID == "" {
			userID = "user_000002"
		}
		if existing, ok := findGenerationTaskByClientRequest(data.GenerationTasks, userID, req.ClientRequestID); ok {
			task = existing
			return nil
		}
		if err := enforceJSONGenerationConcurrency(*data, userID); err != nil {
			return err
		}
		adminData := adminDataFromPlatformData(*data)
		rule := billingRuleForRequest(req, adminData)
		count := imageCount(req.Params)
		pointCost := generationPointCostForRequest(req, adminData)
		available := pointsAvailableForUser(*data, userID)
		if available < pointCost {
			return fmt.Errorf("insufficient remaining points: available %d, required %d", available, pointCost)
		}
		taskID := nextID(data.Counters, "task")
		now := time.Now().UTC().Format(time.RFC3339Nano)
		resultIDs := make([]string, 0, count)
		task = generationTask{
			ID:               taskID,
			ClientRequestID:  strings.TrimSpace(req.ClientRequestID),
			UserID:           userID,
			Type:             req.Type,
			Prompt:           req.Prompt,
			Params:           req.Params,
			Model:            req.Model,
			Status:           "SUCCEEDED",
			TaskStatus:       taskStatusSucceeded,
			BillingStatus:    billingStatusCaptured,
			Progress:         100,
			PointCost:        pointCost,
			QuotedPoints:     float64(pointCost),
			ReservedPoints:   float64(pointCost),
			CapturedPoints:   float64(pointCost),
			ResultIDs:        resultIDs,
			CreatedAt:        now,
			UpdatedAt:        now,
			WorkerFinishedAt: now,
		}
		applyGenerationTaskCapabilitySnapshot(&task, req, rule)
		task.ProviderChannel = firstNonEmptyString(stringValue(req.Params["provider_channel"]), stringValue(req.Params["channel_id"]))
		applyTaskSupplierCost(&task, adminData.ProviderCosts)
		for i := 0; i < count; i++ {
			assetID := nextID(data.Counters, "asset")
			referenceCount := 0
			referenceImages := req.Params["referenceImages"]
			inputImageIds := req.Params["inputImageIds"]
			inputImagesSnapshot := req.Params["inputImagesSnapshot"]
			maskDraft := req.Params["maskDraft"]
			maskTargetImageId := req.Params["maskTargetImageId"]
			maskImageId := req.Params["maskImageId"]
			if items, ok := referenceImages.([]any); ok {
				referenceCount = len(items)
			}
			imageURL := promptPreviewImage(req.Prompt)
			contentType := "image/svg+xml"
			source := "local-prompt-preview"
			mediaType := "image"
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
			if isVideoGenerationType(req.Type) {
				if videoURL := providerTaskString(req, "videoUrl"); videoURL != "" {
					imageURL = videoURL
					mediaType = "video"
					contentType = "video/mp4"
					source = firstNonEmptyString(providerTaskString(req, "provider"), "video-provider")
					thumbnailURL = firstNonEmptyString(providerTaskString(req, "thumbnailUrl"), thumbnailURL)
				}
			}
			task.ResultIDs = append(task.ResultIDs, assetID)
			data.Assets = append(data.Assets, asset{
				ID:           assetID,
				UserID:       userID,
				TaskID:       taskID,
				Name:         generationAssetName(req.Type, taskID, i),
				MediaType:    mediaType,
				URL:          imageURL,
				ThumbnailURL: thumbnailURL,
				Favorite:     false,
				Metadata: map[string]any{
					"prompt":                req.Prompt,
					"model":                 req.Model,
					"type":                  req.Type,
					"module_code":           task.ModuleCode,
					"billing_type":          task.BillingType,
					"sourceType":            req.Type,
					"contentType":           contentType,
					"source":                source,
					"providerTaskId":        stringValue(req.Params["provider_task_id"]),
					"providerRevisedPrompt": stringValue(req.Params["provider_revised_prompt"]),
					"thumbnailUrl":          thumbnailURL,
					"width":                 width,
					"height":                height,
					"resolution":            fmt.Sprintf("%dx%d", width, height),
					"index":                 i + 1,
					"referenceCount":        referenceCount,
					"referenceImages":       referenceImages,
					"inputImageIds":         inputImageIds,
					"inputImagesSnapshot":   inputImagesSnapshot,
					"maskDraft":             maskDraft,
					"maskTargetImageId":     maskTargetImageId,
					"maskImageId":           maskImageId,
				},
				CreatedAt: now,
				UpdatedAt: now,
			})
			copyGenerationComplianceMetadata(data.Assets[len(data.Assets)-1].Metadata, req.Params, assetID, now)
		}
		appendBillingLifecycleEventJSON(data, task, "QUOTE", float64(pointCost), map[string]any{"modelCode": task.Model})
		if _, err := applyJSONWalletEntry(data, task, "RESERVE", pointCost, "生成任务冻结"); err != nil {
			return err
		}
		appendBillingLifecycleEventJSON(data, task, "RESERVE", float64(pointCost), nil)
		if _, err := applyJSONWalletEntry(data, task, "CAPTURE", pointCost, "生成任务确认扣费"); err != nil {
			return err
		}
		appendBillingLifecycleEventJSON(data, task, "CAPTURE", float64(pointCost), nil)
		data.GenerationTasks = append(data.GenerationTasks, task)
		nextAvailable := available - pointCost
		adminDefaults := withAdminDefaults(adminPlatformData{
			Users:         data.Users,
			ChannelAgents: data.ChannelAgents,
		})
		user := userMap(adminDefaults.Users)[task.UserID]
		directAgent, hasDirectAgent := directActiveAgentForUser(adminDefaults.Users, adminDefaults.ChannelAgents, task.UserID)
		event := generationBillingEvent(task, available, nextAvailable, now, user, directAgent, hasDirectAgent)
		data.BillingEvents = append(data.BillingEvents, event)
		commissionData := withAdminDefaults(adminPlatformData{
			Users:           data.Users,
			ChannelAgents:   data.ChannelAgents,
			Commissions:     data.Commissions,
			CommissionRules: data.CommissionRules,
		})
		data.Commissions = append(data.Commissions, commissionArtifactsForUser(&commissionData, task.UserID, task.ID, commissionOrderTypeForModule(task.ModuleCode), task.ModuleCode, event.AmountCents, now)...)
		return nil
	}); err != nil {
		return generationTask{}, err
	}
	return task, nil
}

func (s *jsonStore) CreatePendingGenerationTask(req createGenerationTaskRequest) (generationTask, error) {
	var task generationTask
	if err := s.update(func(data *platformData) error {
		userID := strings.TrimSpace(req.UserID)
		if userID == "" {
			userID = "user_000002"
		}
		if existing, ok := findGenerationTaskByClientRequest(data.GenerationTasks, userID, req.ClientRequestID); ok {
			task = existing
			return nil
		}
		if err := enforceJSONGenerationConcurrency(*data, userID); err != nil {
			return err
		}
		adminData := adminDataFromPlatformData(*data)
		rule := billingRuleForRequest(req, adminData)
		pointCost := generationPointCostForRequest(req, adminData)
		available := pointsAvailableForUser(*data, userID)
		if available < pointCost {
			return fmt.Errorf("insufficient remaining points: available %d, required %d", available, pointCost)
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		nextAvailable := available - pointCost
		params := generationBillingReservationParams(req.Params, now, pointCost, available, nextAvailable)
		req.Params = params
		task = generationTask{
			ID:              nextID(data.Counters, "task"),
			ClientRequestID: strings.TrimSpace(req.ClientRequestID),
			UserID:          userID,
			Type:            req.Type,
			Prompt:          req.Prompt,
			Params:          params,
			Model:           req.Model,
			Status:          "PROCESSING",
			TaskStatus:      taskStatusQueued,
			BillingStatus:   billingStatusReserved,
			Progress:        5,
			PointCost:       pointCost,
			QuotedPoints:    float64(pointCost),
			ReservedPoints:  float64(pointCost),
			ResultIDs:       []string{},
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		applyGenerationTaskCapabilitySnapshot(&task, req, rule)
		task.ProviderChannel = firstNonEmptyString(stringValue(req.Params["provider_channel"]), stringValue(req.Params["channel_id"]))
		appendBillingLifecycleEventJSON(data, task, "QUOTE", float64(pointCost), map[string]any{"modelCode": task.Model})
		if _, err := applyJSONWalletEntry(data, task, "RESERVE", pointCost, "生成任务冻结"); err != nil {
			return err
		}
		appendBillingLifecycleEventJSON(data, task, "RESERVE", float64(pointCost), nil)
		data.GenerationTasks = append(data.GenerationTasks, task)
		return nil
	}); err != nil {
		return generationTask{}, err
	}
	return task, nil
}

func (s *jsonStore) CompleteGenerationTask(id string, req createGenerationTaskRequest) (generationTask, error) {
	var task generationTask
	if err := s.update(func(data *platformData) error {
		index := -1
		for i := range data.GenerationTasks {
			if data.GenerationTasks[i].ID == id {
				index = i
				task = data.GenerationTasks[i]
				break
			}
		}
		if index < 0 {
			return fmt.Errorf("generation task not found: %s", id)
		}
		if task.Status == "SUCCEEDED" || task.Status == "FAILED" || task.Status == "CANCELLED" {
			return nil
		}
		pointCost := task.PointCost
		if pointCost <= 0 {
			pointCost = generationPointCostForRequest(req, adminDataFromPlatformData(*data))
		}
		pointCost = generationTaskReservedPointCost(task, pointCost)
		reserved := generationTaskReservedAndActive(task)
		available := pointsAvailableForUser(*data, task.UserID)
		if !reserved && available < pointCost {
			return fmt.Errorf("insufficient remaining points: available %d, required %d", available, pointCost)
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		req.UserID = task.UserID
		req.Type = firstNonEmptyString(req.Type, task.Type)
		req.Prompt = firstNonEmptyString(req.Prompt, task.Prompt)
		req.Model = firstNonEmptyString(req.Model, task.Model)
		if req.Params == nil {
			req.Params = task.Params
		}
		adminData := adminDataFromPlatformData(*data)
		rule := billingRuleForRequest(req, adminData)
		task.Status = "SUCCEEDED"
		task.TaskStatus = taskStatusSucceeded
		task.BillingStatus = billingStatusCaptured
		task.Progress = 100
		task.PointCost = pointCost
		task.CapturedPoints = float64(pointCost)
		task.Error = nil
		task.UpdatedAt = now
		task.WorkerFinishedAt = now
		task.ResultIDs = []string{}
		applyGenerationTaskCapabilitySnapshot(&task, req, rule)
		task.ProviderChannel = firstNonEmptyString(task.ProviderChannel, stringValue(req.Params["provider_channel"]), stringValue(req.Params["channel_id"]))
		applyTaskSupplierCost(&task, adminData.ProviderCosts)
		count := imageCount(req.Params)
		for i := 0; i < count; i++ {
			assetID := nextID(data.Counters, "asset")
			item := generatedAssetForRequest(req, task.UserID, task.ID, assetID, i, now)
			data.Assets = append(data.Assets, item)
			task.ResultIDs = append(task.ResultIDs, assetID)
		}
		balanceBefore := available
		balanceAfter := available - pointCost
		if reserved {
			balanceBefore, balanceAfter = generationTaskReservationBalances(task, available, pointCost)
			if _, err := applyJSONWalletEntry(data, task, "CAPTURE", pointCost, "生成任务确认扣费"); err != nil {
				return err
			}
		} else {
			task.QuotedPoints = float64(pointCost)
			task.ReservedPoints = float64(pointCost)
			appendBillingLifecycleEventJSON(data, task, "QUOTE", float64(pointCost), map[string]any{"modelCode": task.Model})
			if _, err := applyJSONWalletEntry(data, task, "RESERVE", pointCost, "生成任务冻结"); err != nil {
				return err
			}
			appendBillingLifecycleEventJSON(data, task, "RESERVE", float64(pointCost), nil)
			if _, err := applyJSONWalletEntry(data, task, "CAPTURE", pointCost, "生成任务确认扣费"); err != nil {
				return err
			}
		}
		appendBillingLifecycleEventJSON(data, task, "CAPTURE", float64(pointCost), nil)
		data.GenerationTasks[index] = task
		adminDefaults := withAdminDefaults(adminPlatformData{
			Users:         data.Users,
			ChannelAgents: data.ChannelAgents,
		})
		user := userMap(adminDefaults.Users)[task.UserID]
		directAgent, hasDirectAgent := directActiveAgentForUser(adminDefaults.Users, adminDefaults.ChannelAgents, task.UserID)
		event := generationBillingEvent(task, balanceBefore, balanceAfter, now, user, directAgent, hasDirectAgent)
		data.BillingEvents = append(data.BillingEvents, event)
		commissionData := withAdminDefaults(adminPlatformData{
			Users:           data.Users,
			ChannelAgents:   data.ChannelAgents,
			Commissions:     data.Commissions,
			CommissionRules: data.CommissionRules,
		})
		data.Commissions = append(data.Commissions, commissionArtifactsForUser(&commissionData, task.UserID, task.ID, commissionOrderTypeForModule(task.ModuleCode), task.ModuleCode, event.AmountCents, now)...)
		return nil
	}); err != nil {
		return generationTask{}, err
	}
	return task, nil
}

func (s *jsonStore) RecordPPTGenerationUsage(task pptapp.Task) (adminBillingEvent, error) {
	var event adminBillingEvent
	err := s.updateAdmin(func(data *adminPlatformData) error {
		userID := strings.TrimSpace(task.UserID)
		if userID == "" {
			userID = "user_000002"
		}
		task.UserID = userID
		for _, item := range data.BillingEvents {
			if item.TaskID == task.TaskID && strings.EqualFold(item.MetricCode, billingMetricPPTGenerate) {
				event = item
				return nil
			}
		}
		pointCost := pptPointCostWithRules(task, *data)
		available := pointsAvailableForAdminUser(*data, userID)
		if available < pointCost {
			return fmt.Errorf("insufficient remaining points: available %d, required %d", available, pointCost)
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		walletTask := generationTask{ID: task.TaskID, UserID: userID, ModuleCode: modulePPTGeneration, Model: firstNonEmptyString(task.TextModel, "ppt-text-model")}
		if _, err := applyAdminJSONWalletEntryV1(data, walletTask, "RESERVE", pointCost, "PPT generation reserve"); err != nil {
			return err
		}
		if _, err := applyAdminJSONWalletEntryV1(data, walletTask, "CAPTURE", pointCost, "PPT generation capture"); err != nil {
			return err
		}
		nextAvailable := available - pointCost

		user := userMap(data.Users)[userID]
		directAgent, hasDirectAgent := directActiveAgentForUser(data.Users, data.ChannelAgents, userID)
		event = pptBillingEvent(task, pointCost, available, nextAvailable, now, user, directAgent, hasDirectAgent)
		data.BillingEvents = append(data.BillingEvents, event)
		data.Commissions = append(data.Commissions, commissionArtifactsForUser(data, userID, task.TaskID, "PPT_GENERATION", "ppt_generation", event.AmountCents, now)...)
		return nil
	})
	return event, err
}

func (s *jsonStore) FailGenerationTask(id string, message string) (generationTask, error) {
	var task generationTask
	if err := s.update(func(data *platformData) error {
		for i := range data.GenerationTasks {
			if data.GenerationTasks[i].ID != id {
				continue
			}
			task = data.GenerationTasks[i]
			if task.Status == "SUCCEEDED" {
				return nil
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			pointCost := generationTaskReservedPointCost(task, task.PointCost)
			if generationTaskReservedAndActive(task) && pointCost > 0 {
				available := pointsAvailableForUser(*data, task.UserID)
				nextAvailable := available + pointCost
				if _, err := applyJSONWalletEntry(data, task, "RELEASE", pointCost, "生成失败解冻"); err != nil {
					return err
				}
				task.Params = generationBillingRefundParams(task.Params, now, available, nextAvailable)
				task.BillingStatus = billingStatusReleased
				task.ReleasedPoints = float64(pointCost)
				appendBillingLifecycleEventJSON(data, task, "RELEASE", float64(pointCost), nil)
			}
			if task.Status == "FAILED" || task.Status == "CANCELLED" {
				data.GenerationTasks[i] = task
				return nil
			}
			task.Status = "FAILED"
			task.TaskStatus = taskStatusFailed
			if task.BillingStatus == "" {
				task.BillingStatus = billingStatusBillingFailed
			}
			task.Progress = 100
			task.Error = map[string]any{"message": message}
			task.FailureReason = message
			task.UpdatedAt = now
			task.WorkerFinishedAt = now
			data.GenerationTasks[i] = task
			return nil
		}
		return fmt.Errorf("generation task not found: %s", id)
	}); err != nil {
		return generationTask{}, err
	}
	return task, nil
}

func generationBillingEvent(task generationTask, before int, after int, now string, user adminUser, agent adminChannelAgent, hasAgent bool) adminBillingEvent {
	amountCents := task.PointCost * pointUnitAmountCents
	agentID := ""
	if hasAgent {
		agentID = agent.ID
	}
	moduleCode := firstNonEmptyString(task.ModuleCode, stringValue(task.Params["module_code"]), moduleCodeForType(task.Type))
	billingType := firstNonEmptyString(task.BillingType, stringValue(task.Params["billing_type"]))
	quantity := int(math.Ceil(billingQuantity(billingType, createGenerationTaskRequest{Type: task.Type, Model: task.Model, Params: task.Params, ModuleCode: moduleCode})))
	if quantity < 1 {
		quantity = 1
	}
	event := adminBillingEvent{
		ID:                "evt_" + shortID(task.ID),
		TransactionID:     "txn_" + shortID(task.ID),
		UserID:            task.UserID,
		AgentID:           agentID,
		TenantID:          task.TenantID,
		OperationCenterID: task.OperationCenterID,
		ModuleCode:        moduleCode,
		TaskID:            task.ID,
		AssetIDs:          task.ResultIDs,
		MetricCode:        billingMetricForModule(moduleCode),
		Quantity:          quantity,
		UnitAmountCents:   pointUnitAmountCents,
		AmountCents:       amountCents,
		PointCost:         task.PointCost,
		BalanceBefore:     before,
		BalanceAfter:      after,
		Model:             task.Model,
		Status:            strings.ToUpper(task.Status),
		OccurredAt:        now,
		Metadata: map[string]any{
			"prompt":      task.Prompt,
			"source":      "generation_task",
			"module_code": moduleCode,
			"customer":    user.Name,
			"referredBy":  user.ReferredBy,
		},
	}
	return enrichBillingEventWithTask(event, task)
}

func pptBillingEvent(task pptapp.Task, pointCost int, before int, after int, now string, user adminUser, agent adminChannelAgent, hasAgent bool) adminBillingEvent {
	amountCents := pointCost * pointUnitAmountCents
	agentID := ""
	if hasAgent {
		agentID = agent.ID
	}
	model := strings.TrimSpace(task.TextModel)
	if model == "" {
		model = "ppt-text-model"
	}
	return adminBillingEvent{
		ID:              "evt_" + shortID(task.TaskID+"_ppt"),
		TransactionID:   "txn_" + shortID(task.TaskID+"_ppt"),
		UserID:          task.UserID,
		AgentID:         agentID,
		TenantID:        task.UserID,
		ModuleCode:      modulePPTGeneration,
		TaskID:          task.TaskID,
		AssetIDs:        []string{},
		MetricCode:      billingMetricPPTGenerate,
		Quantity:        pptSlideQuantity(task),
		UnitAmountCents: pointUnitAmountCents,
		AmountCents:     amountCents,
		PointCost:       pointCost,
		BalanceBefore:   before,
		BalanceAfter:    after,
		Model:           model,
		Status:          "SUCCEEDED",
		OccurredAt:      now,
		Metadata: map[string]any{
			"prompt":       task.Prompt,
			"title":        task.Title,
			"source":       "ppt_generation",
			"module_code":  modulePPTGeneration,
			"billing_type": "per_page",
			"customer":     user.Name,
			"referredBy":   user.ReferredBy,
			"slideCount":   pptSlideQuantity(task),
			"language":     task.Language,
			"theme":        task.Theme,
			"imageSource":  task.ImageSource,
			"imageModel":   task.ImageModel,
			"textModel":    model,
		},
	}
}

func billingMetricForModule(moduleCode string) string {
	switch canonicalModuleCode(moduleCode) {
	case moduleVideoGeneration:
		return "video.generations"
	case modulePPTGeneration:
		return billingMetricPPTGenerate
	default:
		return billingMetricImageGenerate
	}
}

func commissionOrderTypeForModule(moduleCode string) string {
	switch canonicalModuleCode(moduleCode) {
	case moduleVideoGeneration:
		return "VIDEO_GENERATION"
	case modulePPTGeneration:
		return "PPT_GENERATION"
	default:
		return "IMAGE_GENERATION"
	}
}

func pptPointCost(task pptapp.Task) int {
	return pptPointCostWithRules(task, normalizeAICapabilityDefaults(seedAdminData()))
}

func pptPointCostWithRules(task pptapp.Task, data adminPlatformData) int {
	model := strings.TrimSpace(task.TextModel)
	if model == "" {
		model = "ppt-text-model"
	}
	req := createGenerationTaskRequest{
		Type:       "PPT_GENERATION",
		ModuleCode: modulePPTGeneration,
		Model:      model,
		Params: map[string]any{
			"page_count":         pptSlideQuantity(task),
			"slideCount":         pptSlideQuantity(task),
			"with_images":        pptImagesEnabled(task.ImageSource),
			"uploaded_file":      false,
			"web_search_enabled": task.EnableWebSearch,
			"theme_style":        task.Theme,
			"language":           task.Language,
		},
	}
	return generationPointCostForRequest(req, data)
}

func pptSlideQuantity(task pptapp.Task) int {
	if task.SlideCount > 0 {
		return task.SlideCount
	}
	if len(task.Slides) > 0 {
		return len(task.Slides)
	}
	if task.Outline != nil && len(task.Outline.Slides) > 0 {
		return len(task.Outline.Slides)
	}
	return 1
}

func isUsageBillingMetric(metric string) bool {
	switch strings.TrimSpace(metric) {
	case billingMetricImageGenerate, billingMetricVideoGenerate, billingMetricPPTGenerate:
		return true
	default:
		return false
	}
}

func usageTypeForMetric(metric string) string {
	switch strings.TrimSpace(metric) {
	case billingMetricPPTGenerate:
		return "PPT_GENERATION"
	case billingMetricVideoGenerate:
		return "TEXT_TO_VIDEO"
	default:
		return "IMAGE_GENERATION"
	}
}

func usageDisplayNameForMetric(metric string) string {
	switch strings.TrimSpace(metric) {
	case billingMetricPPTGenerate:
		return "PPT 文档生成"
	case billingMetricVideoGenerate:
		return "视频生成"
	default:
		return "AI 生图"
	}
}

func directActiveAgentForUser(users []adminUser, agents []adminChannelAgent, userID string) (adminChannelAgent, bool) {
	usersByID := userMap(users)
	user := usersByID[userID]
	if strings.TrimSpace(user.ReferredBy) == "" {
		return adminChannelAgent{}, false
	}
	agent, ok := agentByUserMap(agents)[user.ReferredBy]
	if !ok || !strings.EqualFold(agent.Status, "ACTIVE") {
		return adminChannelAgent{}, false
	}
	return agent, true
}

func commissionArtifactsForUser(data *adminPlatformData, userID string, orderID string, orderType string, source string, amountCents int, now string) []adminCommission {
	if data == nil || amountCents <= 0 {
		return nil
	}
	rules := activeCommissionRules(data.CommissionRules, orderType)
	if len(rules) == 0 {
		return nil
	}
	chain := activeAgentChainForUser(data.Users, data.ChannelAgents, userID)
	if len(chain) == 0 {
		return nil
	}
	ids := commissionIDs(data.Commissions)
	type matchedCommissionRule struct {
		rule  adminCommissionRule
		agent adminChannelAgent
	}
	matchedRules := []matchedCommissionRule{}
	maxTotalRate := 0.0
	for _, rule := range rules {
		if rule.RelationDepth <= 0 || rule.RelationDepth > len(chain) {
			continue
		}
		agent := chain[rule.RelationDepth-1]
		if !commissionRuleMatchesAgent(rule, agent) {
			continue
		}
		matchedRules = append(matchedRules, matchedCommissionRule{rule: rule, agent: agent})
		if rule.MaxTotalRate > maxTotalRate {
			maxTotalRate = rule.MaxTotalRate
		}
	}
	maxTotalCents := 0
	if maxTotalRate > 0 {
		maxTotalCents = int(math.Round(float64(amountCents) * maxTotalRate))
	}
	items := []adminCommission{}
	totalCents := 0
	for _, match := range matchedRules {
		rule := match.rule
		agent := match.agent
		if commissionExistsForRule(data.Commissions, orderID, agent.ID, rule.ID) {
			continue
		}
		commissionCents := rule.FixedAmountCents
		if commissionCents <= 0 && rule.Rate > 0 {
			commissionCents = int(math.Round(float64(amountCents) * rule.Rate))
		}
		if commissionCents <= 0 {
			continue
		}
		if maxTotalCents > 0 && totalCents+commissionCents > maxTotalCents {
			commissionCents = maxTotalCents - totalCents
		}
		if commissionCents <= 0 {
			continue
		}
		id := uniqueAdminID("commission", ids)
		ids[id] = true
		totalCents += commissionCents
		items = append(items, adminCommission{
			ID:          id,
			OrderID:     orderID,
			AgentID:     agent.ID,
			AmountCents: commissionCents,
			Rate:        rule.Rate,
			Status:      "PENDING",
			RuleSnapshot: map[string]any{
				"source":           source,
				"orderType":        strings.ToUpper(strings.TrimSpace(orderType)),
				"amountCents":      amountCents,
				"rate":             rule.Rate,
				"fixedAmountCents": rule.FixedAmountCents,
				"maxTotalRate":     rule.MaxTotalRate,
				"relationDepth":    rule.RelationDepth,
				"ruleId":           rule.ID,
				"ruleName":         rule.Name,
				"settlementMode":   "RULE_ENGINE",
			},
			CreatedAt: now,
		})
	}
	return items
}

func commissionRuleMatchesAgent(rule adminCommissionRule, agent adminChannelAgent) bool {
	earnerRole := strings.ToUpper(strings.TrimSpace(rule.EarnerRole))
	if earnerRole == "" || earnerRole == "AGENT" {
		return true
	}
	return earnerRole == agentRoleForLevel(agent.Level)
}

func applyCommissionRuleMutation(rule adminCommissionRule, req adminCommissionRuleMutation) adminCommissionRule {
	if req.Name != "" {
		rule.Name = strings.TrimSpace(req.Name)
	}
	if req.OrderType != "" {
		rule.OrderType = strings.ToUpper(strings.TrimSpace(req.OrderType))
	}
	if req.EarnerRole != "" {
		rule.EarnerRole = strings.ToUpper(strings.TrimSpace(req.EarnerRole))
	}
	if req.RelationDepth > 0 {
		rule.RelationDepth = req.RelationDepth
	}
	rule.FixedAmountCents = req.FixedAmountCents
	rule.Rate = req.Rate
	rule.MaxTotalRate = req.MaxTotalRate
	if req.Status != "" {
		rule.Status = strings.ToUpper(strings.TrimSpace(req.Status))
	}
	if rule.Metadata == nil {
		rule.Metadata = map[string]any{}
	}
	rule.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return rule
}

func activeCommissionRules(rules []adminCommissionRule, orderType string) []adminCommissionRule {
	if len(rules) == 0 {
		rules = defaultCommissionRules()
	}
	orderType = strings.ToUpper(strings.TrimSpace(orderType))
	items := []adminCommissionRule{}
	for _, rule := range rules {
		if !strings.EqualFold(rule.Status, "ACTIVE") {
			continue
		}
		if !strings.EqualFold(rule.OrderType, orderType) {
			continue
		}
		items = append(items, rule)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].RelationDepth < items[j].RelationDepth
	})
	return items
}

func activeAgentChainForUser(users []adminUser, agents []adminChannelAgent, userID string) []adminChannelAgent {
	direct, ok := directActiveAgentForUser(users, agents, userID)
	if !ok {
		return nil
	}
	agentsByID := agentByIDMap(agents)
	chain := []adminChannelAgent{direct}
	current := direct
	for len(chain) < 2 && strings.TrimSpace(current.ParentID) != "" {
		parent := agentsByID[current.ParentID]
		if parent.ID == "" || !strings.EqualFold(parent.Status, "ACTIVE") {
			break
		}
		chain = append(chain, parent)
		current = parent
	}
	return chain
}

func commissionExistsForRule(items []adminCommission, orderID string, agentID string, ruleID string) bool {
	for _, item := range items {
		if item.OrderID != orderID || item.AgentID != agentID {
			continue
		}
		if ruleID == "" || stringMetadataValueFromMap(item.RuleSnapshot, "ruleId") == ruleID {
			return true
		}
	}
	return false
}

func billingEventExists(items []adminBillingEvent, taskID string, metricCode string) bool {
	for _, item := range items {
		if item.TaskID == taskID && strings.EqualFold(item.MetricCode, metricCode) {
			return true
		}
	}
	return false
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
func (s *jsonStore) DeleteAssetForUser(userID string, id string) error {
	userID = strings.TrimSpace(userID)
	return s.update(func(data *platformData) error {
		deleted := false
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for i := range data.Assets {
			if data.Assets[i].ID == id && data.Assets[i].UserID == userID && !assetDeleted(data.Assets[i]) {
				data.Assets[i].DeletedAt = now
				data.Assets[i].UpdatedAt = now
				if data.Assets[i].Metadata == nil {
					data.Assets[i].Metadata = map[string]any{}
				}
				data.Assets[i].Metadata["deletedAt"] = now
				deleted = true
				break
			}
		}
		if !deleted {
			return fmt.Errorf("%w: %s", errAssetNotFound, id)
		}
		for i := range data.GenerationTasks {
			if data.GenerationTasks[i].UserID != userID {
				continue
			}
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

func activeAssets(assets []asset) []asset {
	items := make([]asset, 0, len(assets))
	for _, item := range assets {
		if assetDeleted(item) {
			continue
		}
		items = append(items, item)
	}
	return items
}

func assetDeleted(item asset) bool {
	if strings.TrimSpace(item.DeletedAt) != "" {
		return true
	}
	if item.Metadata == nil {
		return false
	}
	return strings.TrimSpace(stringValue(item.Metadata["deletedAt"])) != ""
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

func pointsAvailableForUser(data platformData, userID string) int {
	for _, item := range data.PointAccounts {
		if item.UserID == userID {
			return item.Available
		}
	}
	if userID == "user_000002" {
		return pointsAvailable(data)
	}
	return defaultPointsAvailable
}

func pointsAvailableForAdminUser(data adminPlatformData, userID string) int {
	for _, item := range data.PointAccounts {
		if item.UserID == userID {
			return item.Available
		}
	}
	if userID == "user_000002" && data.PointsAvailable != nil {
		return *data.PointsAvailable
	}
	return defaultPointsAvailable
}

func totalPointsForUser(events []adminBillingEvent, userID string, available int, frozen int) int {
	consumed := 0
	for _, event := range events {
		if event.UserID == userID && event.PointCost > 0 {
			consumed += event.PointCost
		}
	}
	total := available + frozen + consumed
	if total < available+frozen {
		return available + frozen
	}
	return total
}

func imageCount(params map[string]any) int {
	value, ok := params["count"]
	if !ok {
		value, ok = params["n"]
	}
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

func generationAssetName(taskType string, taskID string, index int) string {
	prefix := strings.ToUpper(strings.TrimSpace(taskType))
	if prefix == "" {
		prefix = "TEXT_TO_IMAGE"
	}
	return fmt.Sprintf("%s-%s-%02d", prefix, taskID, index+1)
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
