package httpserver

import (
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"

	"xianzhi-ai/backend-go/internal/config"
	imageprovider "xianzhi-ai/backend-go/internal/provider/image"
	videoprovider "xianzhi-ai/backend-go/internal/provider/video"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

const maxReferenceImageUploadBytes = 20 << 20

type platformStore interface {
	ListGenerationTasks() ([]generationTask, error)
	CreateGenerationTask(createGenerationTaskRequest) (generationTask, error)
	CreatePendingGenerationTask(createGenerationTaskRequest) (generationTask, error)
	CompleteGenerationTask(string, createGenerationTaskRequest) (generationTask, error)
	FailGenerationTask(string, string) (generationTask, error)
	RecordPPTGenerationUsage(pptapp.Task) (adminBillingEvent, error)
	RecordRAGUsage(context.Context, knowledgeapp.RAGBillingUsage) error
	ListAssets() ([]asset, error)
	SaveUploadedAsset(asset) (asset, error)
	UserAIState(string) (userAIState, error)
	UpdateUserAIState(string, userAIState) (userAIState, error)
	UpdateAssetThumbnails(map[string]string) (int, error)
	UpdateAssetImageInfo(map[string]assetImageInfo) (int, error)
	DeleteAssetForUser(userID string, id string) error
	PointAccount(string) (pointAccount, error)
	AdminData() (adminPlatformData, error)
	UpdateUserPassword(string, string) (adminUser, error)
	CreateAdminCustomer(adminCustomerMutation) (adminUser, error)
	UpdateAdminCustomer(string, adminCustomerMutation) (adminUser, error)
	UpdateAdminCustomerIdentity(string, adminCustomerIdentityMutation) (adminUser, error)
	CreateAdminAuthMergeRequest(adminAuthMergeRequestMutation) (adminAuthMergeRequest, error)
	ListAdminAuthMergeRequests(string) ([]adminAuthMergeRequest, error)
	UpdateAdminAuthMergeRequest(string, adminAuthMergeRequestMutation) (adminAuthMergeRequest, error)
	PreviewAdminAuthMergeRequest(string, string) (adminAuthMergeRequest, adminAuthMergePreviewResult, error)
	ExecuteAdminAuthMergeRequest(string, adminAuthMergeExecuteRequest) (adminAuthMergeRequest, adminAuthMergeExecuteResult, error)
	CreateAdminChannelAgent(adminChannelCreateMutation) (adminChannelAgent, adminUser, error)
	UpdateAdminChannelAgent(string, adminChannelMutation) (adminChannelAgent, error)
	UpdateAdminProduct(string, adminProductMutation) (adminProduct, error)
	UpdateAdminPlan(string, adminPlanMutation) (adminPlan, error)
	CreateAdminOrder(adminOrderMutation) (adminOrder, error)
	RegisterPaymentCallbackEvent(adminPaymentEvent) (bool, error)
	MarkAdminOrderPaid(string, ...map[string]any) (adminOrder, error)
	RequestOrderRefund(userID string, orderID string, reason string, remark string) (adminOrder, error)
	RenewAdminOrder(string) (adminOrder, error)
	UpdateAdminDeliveryProject(string, adminDeliveryMutation) (map[string]any, error)
	UpdateAdminSystemSettings(adminSystemMutation) (adminSystemSettings, error)
	SyncAdminCustomerNewAPI(string, adminNewAPISyncRequest) (adminUserModelRoute, error)
	CreateAdminAPIChannel(adminAPIChannelMutation) (adminAPIChannel, error)
	UpdateAdminAPIChannel(string, adminAPIChannelMutation) (adminAPIChannel, error)
	MergeAdminAPIChannelModels(string, []string) (adminAPIChannel, []string, error)
	TestAdminAPIChannel(string, adminAPIChannelTestRequest) (map[string]any, error)
	UpdateAdminAPIModel(string, adminAPIModelMutation) (adminAPIModel, error)
	CreateAdminAPIKey(adminAPIKeyMutation) (adminAPIKey, error)
	UpdateAdminAPIKey(string, adminAPIKeyMutation) (adminAPIKey, error)
	UpdateAdminCustomerGroup(string, adminCustomerGroupMutation) (adminCustomerGroup, error)
	UpdateAdminAIModule(string, adminAIModuleMutation) (adminAIModule, error)
	CreateAdminAIModel(adminAIModelMutation) (adminAIModel, error)
	UpdateAdminAIModel(string, adminAIModelMutation) (adminAIModel, error)
	UpdateAdminAIParameterSchema(string, adminAIParameterSchemaMutation) (adminAIParameterSchema, error)
	UpdateAdminTenantModuleLimit(string, adminTenantModuleLimitMutation) (adminTenantModuleLimit, error)
	UpdateAdminPlanCapabilities(string, adminPlanCapabilitiesMutation) error
	UpdateAdminBillingRule(string, adminBillingRuleMutation) (adminBillingRule, error)
	CreateAdminCommission(adminCommissionMutation) (adminCommission, error)
	ReviewAdminCommission(string, string) (adminCommission, error)
	UpdateMarketingCommissionRule(string, adminCommissionRuleMutation) (adminCommissionRule, error)
	CreateAdminWithdrawal(adminWithdrawalMutation) (adminWithdrawal, error)
	ReviewAdminWithdrawal(string, string) (adminWithdrawal, error)
	billingV1Store
}

type optimizedUserContentStore interface {
	ListGenerationTasksForUser(userID string, limit int) ([]generationTask, error)
	ListAssetsForUser(userID string, limit int) ([]asset, error)
	GetGenerationTaskForUser(userID string, id string) (generationTask, bool, error)
}

type optimizedUserAssetDetailStore interface {
	GetAssetForUser(userID string, id string) (asset, bool, error)
}

type optimizedUserContentPageStore interface {
	ListGenerationTasksPageForUser(userID string, limit int, offset int, prioritize bool) ([]generationTask, int, error)
	ListAssetsPageForUser(userID string, limit int, offset int) ([]asset, int, error)
}

type assetListSummary struct {
	Total         int   `json:"total"`
	MonthTotal    int   `json:"monthTotal"`
	FavoriteTotal int   `json:"favoriteTotal"`
	StorageBytes  int64 `json:"storageBytes"`
}

type optimizedUserContentSummaryStore interface {
	AssetListSummaryForUser(userID string, monthPrefix string) (assetListSummary, error)
}

type activeIdentityStore interface {
	GetActiveUser(userID string) (adminUser, bool, error)
	GetChannelAgentForUser(userID string) (adminChannelAgent, bool, error)
}

type authPricingPermissionStore interface {
	PricingPermissionsForRole(ctx context.Context, role string) ([]string, error)
}

type authRolePermissionStore interface {
	AuthPermissionsForRole(ctx context.Context, role string) ([]string, error)
}

type channelWorkbenchAccessStore interface {
	GetChannelWorkbenchAgentForUser(userID string) (adminChannelAgent, bool, error)
}

type operationCenterIdentityStore interface {
	GetOperationCenterForUser(userID string) (adminOperationCenter, bool, error)
}

type onlineImageSettingsStore interface {
	OnlineImageSettings() (adminPlatformData, error)
}

type optimizedUserAccountStore interface {
	UserAccountData(userID string) (adminPlatformData, error)
}

const (
	defaultUserContentListLimit = 120
	maxUserContentListLimit     = 300
	videoGenerationTimeout      = 20 * time.Minute
	otherGenerationStaleTimeout = 15 * time.Minute
)

type api struct {
	store             platformStore
	generationService generation.Service
	// connectorGenerationService is an injectable seam for connector workers.
	// Production leaves it nil and uses the same configured model routing as all users.
	connectorGenerationService *generation.Service
	pptService                 *pptapp.Service
	cfg                        config.Config
	sessions                   authSessionStore
	taskCancels                *sync.Map
	pptVisualTasks             *sync.Map
	pptVisualLocker            pptVisualDistributedLocker
	fileService                *storagecenter.Service
	contentSecurity            wechatContentSecurityChecker
	imageGenerationTimeout     time.Duration
}

type generatedImageDecorator struct{}

func (generatedImageDecorator) Decorate(ctx context.Context, images []generation.GeneratedImage) []generation.GeneratedImage {
	for i := range images {
		images[i].URL = securePublicMediaURL(images[i].URL)
		images[i].ThumbnailURL = thumbnailForImage(ctx, images[i].URL)
		if width, height, ok := imageDimensionsForImage(ctx, images[i].URL); ok {
			images[i].Width = width
			images[i].Height = height
		}
	}
	return images
}

func newAPI(store platformStore, cfg config.Config, sessions authSessionStore, fileService *storagecenter.Service) api {
	provider := imageprovider.NewDefaultRouter(cfg)
	service := generation.NewServiceWithOptions(generation.ServiceOptions{
		ImageProvider:  provider,
		VideoProvider:  videoprovider.NewMockProvider(),
		ImageDecorator: generatedImageDecorator{},
		CreateTask: func(req generation.CreateRequest) (any, error) {
			return store.CreateGenerationTask(req)
		},
	})
	pptService := pptapp.NewPersistentService(filepath.Join(filepath.Dir(cfg.DataPath), "ppt-tasks.json"))
	if pgStore, ok := store.(*postgresStore); ok {
		pptService = pptapp.NewPostgresService(pgStore.db, filepath.Join(filepath.Dir(cfg.DataPath), "ppt-tasks.json"))
	}
	imageTimeout := cfg.ImageGenerationTimeout()
	api := api{store: store, generationService: service, pptService: pptService, cfg: cfg, sessions: sessions, taskCancels: &sync.Map{}, pptVisualTasks: &sync.Map{}, fileService: fileService, contentSecurity: newWeChatContentSecurityService(cfg), imageGenerationTimeout: imageTimeout}
	go api.repairStaleGenerationTasks(imageTimeout)
	return api
}

func (a api) repairStaleGenerationTasks(maxAge time.Duration) {
	tasks, err := a.store.ListGenerationTasks()
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for _, task := range tasks {
		if !isRunningGenerationTaskStatus(task.Status) {
			continue
		}
		taskMaxAge := maxAge
		if isVideoGenerationRequest(task.Type) {
			taskMaxAge = videoGenerationTimeout
		} else if !isImageGenerationRequest(task.Type) {
			taskMaxAge = otherGenerationStaleTimeout
		}
		updatedAt := firstNonEmptyString(task.UpdatedAt, task.CreatedAt)
		updatedTime, err := time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil || now.Sub(updatedTime.UTC()) < taskMaxAge {
			continue
		}
		_, _ = a.store.FailGenerationTask(task.ID, fmt.Sprintf("任务超过 %d 分钟未完成，已自动标记为失败，请重新生成。", int(taskMaxAge.Minutes())))
	}
}

func isRunningGenerationTaskStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "PENDING", "PROCESSING", "RUNNING", "QUEUED":
		return true
	default:
		return false
	}
}

func (a api) authenticatedUser(r *http.Request) (adminPlatformData, adminUser, error) {
	data, err := a.store.AdminData()
	if err != nil {
		return adminPlatformData{}, adminUser{}, err
	}
	user, err := authAPI{store: a.store, sessions: a.sessions}.authenticatedUser(r, data)
	if err != nil {
		return adminPlatformData{}, adminUser{}, err
	}
	return data, user, nil
}

func (a api) currentUser(r *http.Request) (adminUser, error) {
	if store, ok := a.store.(activeIdentityStore); ok {
		userID, err := authenticatedUserID(r, a.sessions)
		if err != nil {
			return adminUser{}, err
		}
		user, found, err := store.GetActiveUser(userID)
		if err != nil {
			return adminUser{}, err
		}
		if !found {
			return adminUser{}, errUnauthorized
		}
		return user, nil
	}
	_, user, err := a.authenticatedUser(r)
	return user, err
}

func listLimitFromRequest(r *http.Request, key string, fallback int) int {
	limit := fallback
	if value := strings.TrimSpace(r.URL.Query().Get(key)); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			limit = parsed
		}
	}
	if limit <= 0 {
		limit = fallback
	}
	if limit > maxUserContentListLimit {
		return maxUserContentListLimit
	}
	return limit
}

func listOffsetFromRequest(r *http.Request) int {
	offset := 0
	if value := strings.TrimSpace(r.URL.Query().Get("offset")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			offset = parsed
		}
	}
	return offset
}

func pagedListRequested(r *http.Request) bool {
	value := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("paged")))
	return value == "1" || value == "true" || strings.TrimSpace(r.URL.Query().Get("offset")) != ""
}

func prioritizedTaskListRequested(r *http.Request) bool {
	value := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("priority")))
	return value == "active" || value == "1" || value == "true"
}

func (a api) generationTasksPageForUser(userID string, limit int, offset int, prioritize bool) ([]generationTask, int, error) {
	if optimized, ok := a.store.(optimizedUserContentPageStore); ok {
		return optimized.ListGenerationTasksPageForUser(userID, limit, offset, prioritize)
	}
	tasks, err := a.store.ListGenerationTasks()
	if err != nil {
		return nil, 0, err
	}
	tasks = filterGenerationTasksForUser(tasks, userID)
	sortGenerationTasksForUserList(tasks, prioritize)
	total := len(tasks)
	return pageGenerationTasks(tasks, limit, offset), total, nil
}

func (a api) assetsPageForUser(userID string, limit int, offset int) ([]asset, int, error) {
	if optimized, ok := a.store.(optimizedUserContentPageStore); ok {
		return optimized.ListAssetsPageForUser(userID, limit, offset)
	}
	assets, err := a.store.ListAssets()
	if err != nil {
		return nil, 0, err
	}
	assets = filterAssetsForUser(assets, userID)
	sortAssetsForUserList(assets)
	total := len(assets)
	return pageAssets(assets, limit, offset), total, nil
}

func (a api) assetListSummaryForUser(userID string) (assetListSummary, error) {
	monthPrefix := time.Now().UTC().Format("2006-01")
	if optimized, ok := a.store.(optimizedUserContentSummaryStore); ok {
		return optimized.AssetListSummaryForUser(userID, monthPrefix)
	}
	assets, err := a.store.ListAssets()
	if err != nil {
		return assetListSummary{}, err
	}
	assets = filterAssetsForUser(assets, userID)
	summary := assetListSummary{Total: len(assets)}
	for _, item := range assets {
		if strings.HasPrefix(item.CreatedAt, monthPrefix) {
			summary.MonthTotal++
		}
		if item.Favorite {
			summary.FavoriteTotal++
		}
		fileSize := int64Value(item.Metadata["fileSize"])
		if fileSize == 0 {
			fileSize = int64Value(item.Metadata["fileSizeBytes"])
		}
		if fileSize == 0 {
			fileSize = int64Value(item.Metadata["sizeBytes"])
		}
		summary.StorageBytes += fileSize
	}
	return summary, nil
}

func (a api) generationTasksForUser(r *http.Request, userID string, limit int) ([]generationTask, error) {
	if optimized, ok := a.store.(optimizedUserContentStore); ok {
		return optimized.ListGenerationTasksForUser(userID, limit)
	}
	tasks, err := a.store.ListGenerationTasks()
	if err != nil {
		return nil, err
	}
	tasks = filterGenerationTasksForUser(tasks, userID)
	if limit > 0 && len(tasks) > limit {
		return tasks[:limit], nil
	}
	return tasks, nil
}

func (a api) loadAssetsForUser(userID string, limit int) ([]asset, error) {
	if optimized, ok := a.store.(optimizedUserContentStore); ok {
		return optimized.ListAssetsForUser(userID, limit)
	}
	assets, err := a.store.ListAssets()
	if err != nil {
		return nil, err
	}
	assets = filterAssetsForUser(assets, userID)
	if limit > 0 && len(assets) > limit {
		assets = assets[:limit]
	}
	return assets, nil
}

func (a api) assetsForUser(r *http.Request, userID string, limit int) ([]asset, error) {
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	assets, err := a.loadAssetsForUser(userID, limit)
	if err != nil {
		return nil, err
	}
	return a.signStoredAssetURLs(ctx, userID, assets), nil
}

// assetsForUserWorkspaceList is the first-paint path for homepage / AI image /
// works center. It keeps compact thumbnails and skips serial original-URL
// signing; detail and download still sign via /assets/:id.
func (a api) assetsForUserWorkspaceList(userID string, limit int) ([]asset, error) {
	assets, err := a.loadAssetsForUser(userID, limit)
	if err != nil {
		return nil, err
	}
	return prepareWorkspaceListAssets(assets), nil
}

const maxWorkspaceListInlineBytes = 4 << 10

func prepareWorkspaceListAssets(items []asset) []asset {
	result := make([]asset, len(items))
	copy(result, items)
	for index := range result {
		result[index].ThumbnailURL = compactListInlineMediaURL(result[index].ThumbnailURL)
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(result[index].ThumbnailURL)), "storage://") {
			result[index].ThumbnailURL = ""
		}
		if workspaceAssetNeedsOriginalSigning(result[index]) || workspaceListOmitsInlineOriginal(result[index].URL) {
			// Asset grids render compact covers. The original is signed by
			// the detail/download endpoint after the user opens a work.
			result[index].URL = ""
		}
		result[index].Metadata = compactWorkspaceListMetadata(result[index].Metadata)
	}
	return result
}

func compactWorkspaceListTasks(tasks []generationTask) []generationTask {
	items := make([]generationTask, len(tasks))
	copy(items, tasks)
	for index := range items {
		compacted := compactWorkspaceListValue(items[index].Params)
		if asMap, ok := compacted.(map[string]any); ok {
			items[index].Params = asMap
		} else {
			items[index].Params = map[string]any{}
		}
		if len(items[index].Prompt) > 8<<10 {
			items[index].Prompt = items[index].Prompt[:8<<10]
		}
		if workspaceListOmitsInlineOriginal(items[index].ImageURL) {
			items[index].ImageURL = ""
		}
		if workspaceListOmitsInlineOriginal(items[index].OutputURL) {
			items[index].OutputURL = ""
		}
		if workspaceListOmitsInlineOriginal(items[index].ResultURL) {
			items[index].ResultURL = ""
		}
	}
	return items
}

func compactWorkspaceListMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	compacted, _ := compactWorkspaceListValue(metadata).(map[string]any)
	if compacted == nil {
		compacted = map[string]any{}
	}
	delete(compacted, "sourceUrl")
	delete(compacted, "thumbnailUrl")
	return compacted
}

func compactWorkspaceListValue(value any) any {
	switch typed := value.(type) {
	case string:
		if workspaceListOmitsInlineOriginal(typed) {
			return nil
		}
		return typed
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			compacted := compactWorkspaceListValue(item)
			if compacted == nil {
				continue
			}
			out[key] = compacted
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			compacted := compactWorkspaceListValue(item)
			if compacted == nil {
				continue
			}
			out = append(out, compacted)
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return value
	}
}

func workspaceListOmitsInlineOriginal(value string) bool {
	text := strings.TrimSpace(value)
	return strings.HasPrefix(strings.ToLower(text), "data:") || len(text) > maxWorkspaceListInlineBytes
}

func workspaceAssetNeedsOriginalSigning(item asset) bool {
	fileID := firstNonEmptyString(
		stringValue(item.Metadata["fileId"]),
		stringValue(item.Metadata["storageFileId"]),
		storageFileIDFromRef(item.URL),
	)
	coverID := firstNonEmptyString(
		stringValue(item.Metadata["coverFileId"]),
		stringValue(item.Metadata["thumbnailFileId"]),
		storageFileIDFromRef(item.ThumbnailURL),
	)
	return fileID != "" || coverID != ""
}

func (a api) assetForUser(r *http.Request, userID string, id string) (asset, bool, error) {
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	if optimized, ok := a.store.(optimizedUserAssetDetailStore); ok {
		item, found, err := optimized.GetAssetForUser(userID, id)
		if err != nil || !found {
			return asset{}, found, err
		}
		signed := a.signStoredAssetURLs(ctx, userID, []asset{item})
		if len(signed) == 1 {
			item = signed[0]
		}
		return item, true, nil
	}
	assets, err := a.store.ListAssets()
	if err != nil {
		return asset{}, false, err
	}
	for _, item := range assets {
		if item.ID != id || item.UserID != userID || strings.TrimSpace(item.DeletedAt) != "" {
			continue
		}
		signed := a.signStoredAssetURLs(ctx, userID, []asset{item})
		if len(signed) == 1 {
			item = signed[0]
		}
		return item, true, nil
	}
	return asset{}, false, nil
}

func (a api) userAccountData(userID string) (adminPlatformData, error) {
	if optimized, ok := a.store.(optimizedUserAccountStore); ok {
		return optimized.UserAccountData(userID)
	}
	return a.store.AdminData()
}

func (a api) listGenerationTasks(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	limit := listLimitFromRequest(r, "limit", defaultUserContentListLimit)
	offset := listOffsetFromRequest(r)
	prioritize := prioritizedTaskListRequested(r)
	var tasks []generationTask
	var assets []asset
	var total int
	var tasksErr error
	var assetsErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		tasks, total, tasksErr = a.generationTasksPageForUser(user.ID, limit, offset, prioritize)
	}()
	go func() {
		defer wg.Done()
		assets, assetsErr = a.assetsForUser(r, user.ID, limit)
	}()
	wg.Wait()
	if tasksErr != nil {
		writeError(w, http.StatusInternalServerError, tasksErr)
		return
	}
	if assetsErr != nil {
		writeError(w, http.StatusInternalServerError, assetsErr)
		return
	}
	tasks = attachAssetImagesToTasks(tasks, assets)
	if pagedListRequested(r) {
		writeJSON(w, map[string]any{
			"items":   tasks,
			"total":   total,
			"limit":   limit,
			"offset":  offset,
			"hasMore": offset+len(tasks) < total,
		})
		return
	}
	writeJSON(w, tasks)
}

func (a api) getGenerationTask(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	id := strings.TrimSpace(path.Base(r.URL.Path))
	if optimized, ok := a.store.(optimizedUserContentStore); ok {
		task, found, err := optimized.GetGenerationTaskForUser(user.ID, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, errors.New("generation task not found"))
			return
		}
		assets, err := a.assetsForUser(r, user.ID, maxUserContentListLimit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		enriched := attachAssetImagesToTasks([]generationTask{task}, assets)
		if len(enriched) > 0 {
			writeJSON(w, enriched[0])
			return
		}
		writeJSON(w, task)
		return
	}
	tasks, err := a.store.ListGenerationTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	assets, err := a.assetsForUser(r, user.ID, maxUserContentListLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	assets = filterAssetsForUser(assets, user.ID)
	for _, task := range tasks {
		if task.ID == id && task.UserID == user.ID {
			enriched := attachAssetImagesToTasks([]generationTask{task}, assets)
			if len(enriched) > 0 {
				writeJSON(w, enriched[0])
				return
			}
			writeJSON(w, task)
			return
		}
	}
	writeError(w, http.StatusNotFound, fmt.Errorf("generation task not found: %s", id))
}

func (a api) createGenerationTask(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var req generation.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.ClientRequestID) == "" {
		req.ClientRequestID = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	req.UserID = user.ID
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, generation.ErrInvalidPrompt)
		return
	}
	if req.Type == "" {
		req.Type = "TEXT_TO_IMAGE"
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	req.Params["terminal"] = requestTerminal(r)
	if err := a.enforceRequiredLegalAcceptances(user.ID, stringValue(req.Params["terminal"])); err != nil {
		writeError(w, http.StatusPreconditionRequired, err)
		return
	}
	if err := a.checkMiniProgramText(r.Context(), r, user, req.Prompt); err != nil {
		writeContentSecurityError(w, err)
		return
	}
	req, err = a.prepareGenerationRequest(data, user, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := enforceMiniProgramModelCompliance(data, &req); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	markInputAudit(&req, isWeChatMiniProgramRequest(r) && a.contentSecurity != nil)
	service := a.generationService
	if compliantService, ok, err := a.generationServiceForCompliantMiniProgram(req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	} else if ok {
		service = compliantService
	} else if routeService, ok, err := a.generationServiceForUserRoute(user, req.Model); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	} else if ok {
		service = routeService
	} else if memberService, ok, err := a.generationServiceForMemberLevel(user, req.Model); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	} else if ok {
		service = memberService
	} else if providerID := selectedGenerationProvider(req.Params); providerID != "" {
		dynamicService, err := a.generationServiceForProvider(providerID, req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		service = dynamicService
	} else if configuredService, ok, err := a.generationServiceForConfiguredModel(req.Model); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	} else if ok {
		service = configuredService
	}
	if isVideoGenerationRequest(req.Type) {
		task, err := a.store.CreatePendingGenerationTask(req)
		if err != nil {
			if errors.Is(err, errGenerationConcurrencyLimit) {
				writeError(w, http.StatusTooManyRequests, err)
				return
			}
			writeError(w, http.StatusBadRequest, err)
			return
		}
		a.recordContentAudit(task.ID, "input", "generation_request", "", req)
		go a.runVideoGenerationTask(task.ID, service, cloneGenerationCreateRequest(req))
		writeJSON(w, task)
		return
	}
	if !isImageGenerationRequest(req.Type) || strings.EqualFold(strings.TrimSpace(req.Model), "mock-standard") {
		if isImageGenerationRequest(req.Type) {
			if err := auditGeneratedOutput(&req); err != nil {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
		}
		task, err := service.Create(r.Context(), req)
		if err != nil {
			if errors.Is(err, errGenerationConcurrencyLimit) {
				writeError(w, http.StatusTooManyRequests, err)
				return
			}
			if errors.Is(err, generation.ErrInvalidPrompt) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, task)
		return
	}
	task, err := a.store.CreatePendingGenerationTask(req)
	if err != nil {
		if errors.Is(err, errGenerationConcurrencyLimit) {
			writeError(w, http.StatusTooManyRequests, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	a.recordContentAudit(task.ID, "input", "generation_request", "", req)
	go a.runGenerationTask(task.ID, service, cloneGenerationCreateRequest(req))
	writeJSON(w, task)
}

func isImageGenerationRequest(taskType string) bool {
	switch strings.ToUpper(strings.TrimSpace(taskType)) {
	case "", "TEXT_TO_IMAGE", "IMAGE_TO_IMAGE":
		return true
	default:
		return false
	}
}

func isVideoGenerationRequest(taskType string) bool {
	switch strings.ToUpper(strings.TrimSpace(taskType)) {
	case "TEXT_TO_VIDEO", "IMAGE_TO_VIDEO", "VIDEO_TO_VIDEO":
		return true
	default:
		return false
	}
}

func cloneGenerationCreateRequest(req generation.CreateRequest) generation.CreateRequest {
	req.Params = cloneAnyMap(req.Params)
	if req.GeneratedImages != nil {
		req.GeneratedImages = append([]generation.GeneratedImage{}, req.GeneratedImages...)
	}
	req.VideoTask = cloneAnyValue(req.VideoTask)
	req.ChatResponse = cloneAnyValue(req.ChatResponse)
	return req
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = cloneAnyValue(value)
	}
	return output
}

func cloneAnySlice(input []any) []any {
	if input == nil {
		return nil
	}
	output := make([]any, len(input))
	for i, value := range input {
		output[i] = cloneAnyValue(value)
	}
	return output
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		return cloneAnySlice(typed)
	case []string:
		return append([]string{}, typed...)
	case []int:
		return append([]int{}, typed...)
	case []float64:
		return append([]float64{}, typed...)
	case []bool:
		return append([]bool{}, typed...)
	default:
		return value
	}
}

type generationServiceCandidate struct {
	service generation.Service
	channel adminAPIChannel
}

func (a api) runGenerationTask(taskID string, service generation.Service, req generation.CreateRequest) {
	startedAt := time.Now()
	taskTimeout := a.configuredImageGenerationTimeout()
	log.Printf("generation task started task_id=%s type=%s model=%s timeout_ms=%d", taskID, req.Type, req.Model, taskTimeout.Milliseconds())
	ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
	a.registerGenerationTaskCancel(taskID, cancel)
	defer func() {
		a.unregisterGenerationTaskCancel(taskID)
		cancel()
	}()
	prepared, err := service.PrepareImageTask(ctx, req)
	if err != nil {
		prepared, err = a.prepareImageTaskWithFallback(ctx, req, err)
		if err != nil {
			a.failImageGenerationTask(taskID, "provider", startedAt, err)
			return
		}
	}
	if err := a.auditPreparedGeneratedOutput(ctx, &prepared); err != nil {
		a.recordContentAudit(taskID, "output", "generated_image", "", prepared)
		a.failImageGenerationTask(taskID, "content_audit", startedAt, err)
		return
	}
	a.recordContentAudit(taskID, "output", "generated_image", "", prepared)
	prepared, storedFiles, err := a.persistGeneratedImages(ctx, taskID, prepared)
	if err != nil {
		a.failImageGenerationTask(taskID, "persistence", startedAt, err)
		return
	}
	completed, err := a.store.CompleteGenerationTask(taskID, prepared)
	if err != nil {
		a.cleanupGeneratedFiles(storedFiles)
		a.failImageGenerationTask(taskID, "completion", startedAt, err)
		return
	}
	if !strings.EqualFold(completed.Status, "SUCCEEDED") && !strings.EqualFold(completed.Status, "COMPLETED") {
		a.cleanupGeneratedFiles(storedFiles)
	}
	log.Printf("generation task finished task_id=%s status=%s elapsed_ms=%d", taskID, completed.Status, time.Since(startedAt).Milliseconds())
}

func (a api) configuredImageGenerationTimeout() time.Duration {
	if a.imageGenerationTimeout > 0 {
		return a.imageGenerationTimeout
	}
	return a.cfg.ImageGenerationTimeout()
}

func (a api) failImageGenerationTask(taskID string, stage string, startedAt time.Time, err error) {
	message := generationErrorMessage(err)
	log.Printf("generation task failed task_id=%s stage=%s elapsed_ms=%d error=%q", taskID, stage, time.Since(startedAt).Milliseconds(), message)
	_, _ = a.store.FailGenerationTask(taskID, message)
}

func (a api) prepareImageTaskWithFallback(ctx context.Context, req generation.CreateRequest, firstErr error) (generation.CreateRequest, error) {
	if !shouldFallbackImageGeneration(firstErr) {
		return generation.CreateRequest{}, firstErr
	}
	candidates, err := a.fallbackImageGenerationServices(req.Model)
	if err != nil || len(candidates) == 0 {
		return generation.CreateRequest{}, firstErr
	}
	providerErrs := []error{firstErr}
	for _, candidate := range candidates {
		fallbackReq := cloneGenerationCreateRequest(req)
		if fallbackReq.Params == nil {
			fallbackReq.Params = map[string]any{}
		}
		fallbackReq.Params["provider"] = candidate.channel.ID
		fallbackReq.Params["fallbackProvider"] = candidate.channel.ID
		fallbackReq.Params["fallbackReason"] = generationErrorMessage(firstErr)
		prepared, err := candidate.service.PrepareImageTask(ctx, fallbackReq)
		if err == nil {
			if prepared.Params == nil {
				prepared.Params = map[string]any{}
			}
			prepared.Params["provider"] = candidate.channel.ID
			prepared.Params["fallbackProvider"] = candidate.channel.ID
			prepared.Params["fallbackReason"] = generationErrorMessage(firstErr)
			return prepared, nil
		}
		providerErrs = append(providerErrs, fmt.Errorf("fallback provider %s failed: %w", candidate.channel.ID, err))
		if !shouldFallbackImageGeneration(err) {
			break
		}
	}
	return generation.CreateRequest{}, errors.Join(providerErrs...)
}

func (a api) fallbackImageGenerationServices(model string) ([]generationServiceCandidate, error) {
	data, err := a.store.AdminData()
	if err != nil {
		return nil, err
	}
	candidates := []generationServiceCandidate{}
	seenEndpoints := map[string]bool{}
	preferredOrigin := fallbackPreferredImageOrigin(data)
	channels := configuredGenerationChannels(data)
	sort.SliceStable(channels, func(i, j int) bool {
		leftPreferred := preferredOrigin != "" && normalizedURLOrigin(channels[i].BaseURL) == preferredOrigin
		rightPreferred := preferredOrigin != "" && normalizedURLOrigin(channels[j].BaseURL) == preferredOrigin
		if leftPreferred != rightPreferred {
			return leftPreferred
		}
		return priorityLess(channels[i], channels[j])
	})
	for _, channel := range channels {
		if strings.TrimSpace(model) != "" && !apiChannelSupportsModel(channel, model) {
			continue
		}
		endpointKey := normalizedURLOrigin(channel.BaseURL) + "|" + strings.TrimSpace(channel.ImageGenerationEndpoint)
		isPreferredOrigin := preferredOrigin != "" && normalizedURLOrigin(channel.BaseURL) == preferredOrigin
		if endpointKey != "|" && seenEndpoints[endpointKey] && !isPreferredOrigin {
			continue
		}
		service, err := a.generationServiceForChannel(data, channel)
		if err != nil {
			continue
		}
		if endpointKey != "|" {
			seenEndpoints[endpointKey] = true
		}
		candidates = append(candidates, generationServiceCandidate{service: service, channel: channel})
	}
	return candidates, nil
}

func fallbackPreferredImageOrigin(data adminPlatformData) string {
	if origin := normalizedURLOrigin(newAPISyncConfigFromSettings(data.SystemSettings).BaseURL); origin != "" {
		return origin
	}
	if origin := normalizedURLOrigin(os.Getenv("MODEL_PROVIDER_URL")); origin != "" {
		return origin
	}
	return normalizedURLOrigin(os.Getenv("OPENAI_BASE_URL"))
}

func (a api) runVideoGenerationTask(taskID string, service generation.Service, req generation.CreateRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), videoGenerationTimeout)
	a.registerGenerationTaskCancel(taskID, cancel)
	defer func() {
		a.unregisterGenerationTaskCancel(taskID)
		cancel()
	}()
	prepared, err := service.PrepareVideoTask(ctx, req)
	if err != nil {
		_, _ = a.store.FailGenerationTask(taskID, generationErrorMessage(err))
		return
	}
	if provider := providerTaskString(prepared, "provider"); provider != "" {
		prepared.Params["provider"] = provider
		prepared.Params["provider_channel"] = provider
	}
	if _, err := a.store.CompleteGenerationTask(taskID, prepared); err != nil {
		_, _ = a.store.FailGenerationTask(taskID, generationErrorMessage(err))
	}
}

func generationErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "生成超时，请稍后重试"
	}
	return compactGenerationErrorMessage(err.Error())
}

func shouldFallbackImageGeneration(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "returned 502") ||
		strings.Contains(lower, "returned 503") ||
		strings.Contains(lower, "returned 504") ||
		strings.Contains(lower, "returned 429") ||
		strings.Contains(lower, "returned 403") ||
		strings.Contains(lower, "gateway time-out") ||
		strings.Contains(lower, "gateway timeout") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "too many requests") ||
		strings.Contains(lower, "insufficient_quota") ||
		strings.Contains(lower, "quota exceeded") ||
		strings.Contains(lower, "no available image quota") ||
		strings.Contains(lower, "forbidden") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "无权访问") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "no such host") ||
		strings.Contains(lower, "network is unreachable") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "context deadline exceeded") ||
		strings.Contains(lower, "client.timeout") ||
		strings.Contains(lower, "timeout awaiting response")
}

func compactGenerationErrorMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "生成失败"
	}
	if item := regexp.MustCompile(`当前账号处未订购[^"\\\r\n]*`).FindString(message); item != "" {
		return strings.TrimSpace(item)
	}
	lower := strings.ToLower(message)
	if strings.Contains(lower, "returned 429") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "too many requests") ||
		strings.Contains(lower, "insufficient_quota") ||
		strings.Contains(lower, "quota exceeded") ||
		strings.Contains(lower, "no available image quota") {
		return "图像上游频率或额度受限，已尝试备用通道，请稍后重试或更换上游 API Key"
	}
	if item := regexp.MustCompile(`"(?:message|error|detail|reason)"\s*:\s*"([^"]+)"`).FindStringSubmatch(message); len(item) > 1 {
		if decoded, err := strconv.Unquote(`"` + item[1] + `"`); err == nil && strings.TrimSpace(decoded) != "" {
			return compactGenerationErrorMessage(decoded)
		}
	}
	if strings.Contains(lower, "returned 504") || strings.Contains(lower, "gateway time-out") || strings.Contains(lower, "gateway timeout") {
		return "图像上游网关超时，请稍后重试或切换上游通道"
	}
	if strings.Contains(lower, "returned 502") || strings.Contains(lower, "returned 503") {
		return "图像上游服务暂不可用，请稍后重试或切换上游通道"
	}
	if strings.Contains(lower, "returned 429") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "too many requests") ||
		strings.Contains(lower, "insufficient_quota") ||
		strings.Contains(lower, "quota exceeded") ||
		strings.Contains(lower, "no available image quota") {
		return "图像上游频率或额度受限，已尝试备用通道，请稍后重试或更换上游 API Key"
	}
	if strings.Contains(lower, "returned 403") ||
		strings.Contains(lower, "forbidden") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "无权访问") {
		return "图像上游权限或分组不可用，已尝试备用通道，请检查上游 API Key、分组和模型权限"
	}
	if strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "no such host") ||
		strings.Contains(lower, "network is unreachable") ||
		strings.Contains(lower, "connection reset") {
		return "图像上游网络不可达，已尝试备用通道，请检查上游地址或本地代理服务"
	}
	if strings.Contains(lower, "<html") && strings.Contains(lower, "</html>") {
		return "图像上游返回了网关错误页，请稍后重试或切换上游通道"
	}
	if strings.Contains(lower, "create_video_generation_task returned empty task id") && strings.Contains(lower, "seedance") {
		return "移动云 Seedance 创建任务失败，请检查模型资费包、API Key 和模型权限"
	}
	if localized := localizeGenerationErrorMessage(message, lower); localized != "" {
		return localized
	}
	message = strings.Join(strings.Fields(message), " ")
	const maxMessageLen = 240
	if len([]rune(message)) <= maxMessageLen {
		return message
	}
	runes := []rune(message)
	return strings.TrimSpace(string(runes[:maxMessageLen])) + "..."
}

func localizeGenerationErrorMessage(message, lower string) string {
	switch {
	case strings.Contains(lower, "price not configured"), strings.Contains(lower, "价格未配置"):
		return "上游 NewAPI 未配置该模型价格。请在 NewAPI「系统设置 → 分组与模型定价」为 grok-imagine-video-1.5-preview 设置按次价格后重试"
	case strings.Contains(lower, "not allowed by tenant/package limit"), strings.Contains(lower, "no models are allowed by tenant/package limit"):
		return "当前套餐未开放该视频模型，请更换模型或联系管理员开通"
	case strings.Contains(lower, "input_reference") && strings.Contains(lower, "unmarshal"):
		return "视频参考图参数格式错误，请重新上传首帧图后重试"
	case strings.Contains(lower, "cannot unmarshal") && strings.Contains(lower, "seconds"):
		return "视频时长参数格式错误，请重新选择时长后重试"
	case strings.Contains(lower, "cannot unmarshal"):
		return "上游视频接口返回格式异常，请稍后重试或切换模型"
	case strings.Contains(lower, "video provider does not support parameter"):
		return "当前视频通道不支持该参数，请调整参数后重试"
	case strings.Contains(lower, "video provider requires base url and api key"):
		return "视频上游未配置，请检查通道地址和 API Key"
	case strings.Contains(lower, "video model is required"):
		return "请选择视频模型"
	case strings.Contains(lower, "does not support model"):
		return "当前视频通道不支持所选模型，请切换模型或通道"
	case strings.Contains(lower, "requires exactly one reference image"):
		return "该视频模型需要且仅支持 1 张参考图"
	case strings.Contains(lower, "supports exactly one reference image"):
		return "该视频模型仅支持 1 张参考图"
	case strings.Contains(lower, "supports at most seven reference images"):
		return "该视频模型最多支持 7 张参考图"
	case strings.Contains(lower, "video provider returned no video"), strings.Contains(lower, "no result_url"), strings.Contains(lower, "still processing"):
		return "视频仍在生成中或上游未返回结果，请稍后在历史中查看"
	case strings.Contains(lower, "video generation failed"):
		return "视频生成失败，请稍后重试"
	case strings.Contains(lower, "context deadline exceeded"), strings.Contains(lower, "client.timeout"), strings.Contains(lower, "timeout awaiting response"):
		return "生成超时，请稍后重试"
	case strings.Contains(lower, "http 401"), strings.Contains(lower, "invalid api key"), strings.Contains(lower, "incorrect api key"):
		return "上游 API Key 无效，请检查通道密钥配置"
	case strings.Contains(lower, "http 404"):
		return "上游接口不存在，请检查视频通道地址和路径"
	case strings.Contains(lower, "only http/https urls"), strings.Contains(lower, "invalid format for image_urls"), strings.Contains(lower, "asset://"):
		return "参考图地址不被上游接受，请重新上传图片后重试"
	case strings.Contains(lower, "http 400"):
		return "视频请求参数不被上游接受，请检查提示词、参考图和模型参数"
	case strings.Contains(lower, "dial tcp"), strings.Contains(lower, "i/o timeout"):
		return "无法连接视频上游服务，请检查网络或通道地址"
	case strings.Contains(lower, "storage_master_key"):
		return "对象存储密钥未配置，请检查 STORAGE_MASTER_KEY 后重试"
	case strings.Contains(lower, "resolve generated artifact storage"):
		return "生成结果入库失败，请检查对象存储配置后重试"
	case strings.Contains(lower, "unrecognized message"), strings.Contains(lower, "upstream returned unrecognized"):
		return "上游视频通道返回无法识别的结果，请稍后重试或检查 Seedance 通道配置"
	}
	if looksLikeEnglishTechnicalError(message) {
		return "生成失败，请稍后重试。若持续失败请检查模型、参数或上游通道配置"
	}
	return ""
}

func looksLikeEnglishTechnicalError(message string) bool {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return false
	}
	for _, r := range trimmed {
		if r >= 0x4e00 && r <= 0x9fff {
			return false
		}
	}
	lower := strings.ToLower(trimmed)
	markers := []string{"json:", "http", "error", "failed", "unmarshal", "invalid", "provider", "timeout", "denied", "unsupported"}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func selectedGenerationProvider(params map[string]any) string {
	if params == nil {
		return ""
	}
	raw, ok := params["provider"]
	if !ok || raw == nil {
		return ""
	}
	provider, ok := raw.(string)
	if !ok {
		return ""
	}
	provider = strings.TrimSpace(provider)
	if provider == "" || provider == "channel_runtime_env" {
		return ""
	}
	return provider
}

func (a api) generationServiceForUserRoute(user adminUser, model string) (generation.Service, bool, error) {
	if strings.EqualFold(strings.TrimSpace(model), "mock-standard") || strings.EqualFold(strings.TrimSpace(model), "mock-video") {
		return generation.Service{}, false, nil
	}
	route := primaryUserModelRoute(user)
	if route.ID == "" || !apiRouteUsableForGeneration(route) {
		return generation.Service{}, false, nil
	}
	if strings.TrimSpace(model) != "" && len(route.Models) > 0 && !stringListContains(route.Models, model) {
		return generation.Service{}, false, nil
	}
	data, err := a.onlineGenerationSettings()
	if err != nil {
		return generation.Service{}, false, err
	}
	channel, ok := findAPIChannelByRoute(data.APIChannels, route)
	if !ok {
		return generation.Service{}, false, fmt.Errorf("用户模型路由 %s 绑定的渠道不存在或不可用", route.ID)
	}
	if newAPIChannel, newAPIOK := channelForNewAPIRoute(data, route); newAPIOK {
		channel = newAPIChannel
	}
	if len(route.Models) > 0 {
		channel.Models = route.Models
	}
	service, err := a.generationServiceForChannelWithRouteKey(data, channel, route)
	return service, err == nil, err
}

func (a api) generationServiceForMemberLevel(user adminUser, model string) (generation.Service, bool, error) {
	if strings.EqualFold(strings.TrimSpace(model), "mock-standard") || strings.EqualFold(strings.TrimSpace(model), "mock-video") {
		return generation.Service{}, false, nil
	}
	data, err := a.onlineGenerationSettings()
	if err != nil {
		return generation.Service{}, false, err
	}
	channel, ok := findAPIChannelByID(data.APIChannels, "channel_newapi_gateway")
	if !ok || !apiChannelUsableForGeneration(channel) {
		return generation.Service{}, false, nil
	}
	route := adminUserModelRoute{
		ID:        "route_newapi_gateway",
		Provider:  "newapi",
		ChannelID: channel.ID,
		GroupName: memberLevelGroup(user.MemberLevel),
		Models:    channel.Models,
		APIKeyID:  "key_1786355644836388321",
		Status:    "ACTIVE",
	}
	service, err := a.generationServiceForChannelWithRouteKey(data, channel, route)
	return service, err == nil, err
}

func memberLevelGroup(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "svip":
		return "svip"
	case "vip":
		return "vip"
	default:
		return "default"
	}
}

func findAPIChannelByID(channels []adminAPIChannel, id string) (adminAPIChannel, bool) {
	for _, channel := range channels {
		if channel.ID == id && apiChannelUsableForGeneration(channel) {
			return channel, true
		}
	}
	return adminAPIChannel{}, false
}

func channelForNewAPIRoute(data adminPlatformData, route adminUserModelRoute) (adminAPIChannel, bool) {
	if !strings.EqualFold(strings.TrimSpace(route.Provider), "newapi") && strings.TrimSpace(route.ExternalKey) == "" {
		return adminAPIChannel{}, false
	}
	cfg := newAPISyncConfigFromSettings(data.SystemSettings)
	baseURL := normalizedURLOrigin(cfg.BaseURL)
	if baseURL == "" {
		return adminAPIChannel{}, false
	}
	for _, channel := range data.APIChannels {
		if !apiChannelUsableForGeneration(channel) {
			continue
		}
		if normalizedURLOrigin(channel.BaseURL) == baseURL {
			return channel, true
		}
	}
	return adminAPIChannel{}, false
}

func normalizedURLOrigin(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

func (a api) generationServiceForChannelWithRouteKey(data adminPlatformData, channel adminAPIChannel, route adminUserModelRoute) (generation.Service, error) {
	if strings.TrimSpace(route.APIKeyID) == "" {
		return generation.Service{}, fmt.Errorf("用户模型路由 %s 未绑定 API Key", route.ID)
	}
	apiKey := savedAPIKeyByID(data.APIKeys, route.APIKeyID)
	if apiKey == "" {
		return generation.Service{}, fmt.Errorf("用户模型路由 %s 的 API Key 不存在或未启用", route.ID)
	}
	return a.generationServiceForChannelWithAPIKey(channel, apiKey, route)
}

func (a api) generationServiceForConfiguredModel(model string) (generation.Service, bool, error) {
	if strings.EqualFold(strings.TrimSpace(model), "mock-standard") || strings.EqualFold(strings.TrimSpace(model), "mock-video") {
		return generation.Service{}, false, nil
	}
	data, err := a.onlineGenerationSettings()
	if err != nil {
		return generation.Service{}, false, err
	}
	channel, ok, routeErr := selectAPIChannelForConfiguredModel(data, model)
	if routeErr != nil {
		return generation.Service{}, false, routeErr
	}
	if !ok {
		return generation.Service{}, false, fmt.Errorf("请先在主控 SaaS 的 API 配置中启用支持模型 %s 的上游渠道", firstNonEmpty([]string{model, "gpt-image-2"}))
	}
	service, err := a.generationServiceForChannel(data, channel)
	if err != nil {
		return generation.Service{}, false, err
	}
	return service, true, nil
}

func selectAPIChannelForConfiguredModel(data adminPlatformData, model string) (adminAPIChannel, bool, error) {
	if configuredModel, found := findConfiguredAIModel(data, model); found && configuredModelChannelID(configuredModel) != "" {
		return selectBoundAPIChannelForConfiguredModel(data, model)
	}
	channel, ok := selectAPIChannelForModel(configuredGenerationChannels(data), model)
	return channel, ok, nil
}

func configuredModelChannelID(model adminAIModel) string {
	return firstNonEmptyString(model.ChannelID, model.ChannelIDCamel)
}

func selectBoundAPIChannelForConfiguredModel(data adminPlatformData, model string) (adminAPIChannel, bool, error) {
	configuredModel, found := findConfiguredAIModel(data, model)
	if !found {
		return adminAPIChannel{}, false, nil
	}
	channelID := configuredModelChannelID(configuredModel)
	if channelID == "" {
		return adminAPIChannel{}, false, fmt.Errorf("model %s is not bound to an api provider", model)
	}
	for _, channel := range configuredGenerationChannels(data) {
		if channel.ID != channelID {
			continue
		}
		if !apiChannelUsableForGeneration(channel) {
			return adminAPIChannel{}, false, fmt.Errorf("model %s bound api provider %s is not enabled", model, channel.Name)
		}
		if !apiChannelSupportsModel(channel, model) {
			return adminAPIChannel{}, false, fmt.Errorf("model %s bound api provider %s does not support this model", model, channel.Name)
		}
		return channel, true, nil
	}
	return adminAPIChannel{}, false, fmt.Errorf("model %s bound api provider %s was not found or has no credential", model, channelID)
}

func (a api) generationServiceForProvider(providerID string, req generation.CreateRequest) (generation.Service, error) {
	data, err := a.onlineGenerationSettings()
	if err != nil {
		return generation.Service{}, err
	}
	var channel adminAPIChannel
	for _, item := range configuredGenerationChannels(data) {
		if item.ID == providerID {
			channel = item
			break
		}
	}
	if channel.ID == "" {
		return generation.Service{}, fmt.Errorf("api provider not found: %s", providerID)
	}
	if model := strings.TrimSpace(req.Model); model != "" && !apiChannelSupportsModel(channel, model) {
		return generation.Service{}, fmt.Errorf("api provider %s does not support model %s", channel.Name, model)
	}
	return a.generationServiceForChannel(data, channel)
}

func (a api) onlineGenerationSettings() (adminPlatformData, error) {
	if optimized, ok := a.store.(onlineImageSettingsStore); ok {
		return optimized.OnlineImageSettings()
	}
	return a.store.AdminData()
}

func (a api) generationServiceForChannel(data adminPlatformData, channel adminAPIChannel) (generation.Service, error) {
	if !strings.EqualFold(channel.Status, "ACTIVE") && !strings.EqualFold(channel.Status, "CONFIGURABLE") && !strings.EqualFold(channel.Status, "ENABLED") {
		return generation.Service{}, fmt.Errorf("api provider is not enabled: %s", channel.Name)
	}
	apiKeyEnv := strings.TrimSpace(channel.APIKeyEnv)
	apiKey := ""
	if apiKeyEnv != "" {
		apiKey = strings.TrimSpace(os.Getenv(apiKeyEnv))
	}
	if apiKey == "" {
		apiKey = savedAPIKeyForChannel(data.APIKeys, channel)
	}
	if apiKey == "" {
		if apiKeyEnv != "" {
			return generation.Service{}, fmt.Errorf("api provider %s requires saved API Key or environment variable %s", channel.Name, apiKeyEnv)
		}
		return generation.Service{}, fmt.Errorf("api provider %s requires saved API Key", channel.Name)
	}
	return a.generationServiceForChannelWithAPIKey(channel, apiKey, adminUserModelRoute{})
}

func (a api) generationServiceForChannelWithAPIKey(channel adminAPIChannel, apiKey string, route adminUserModelRoute) (generation.Service, error) {
	model := firstNonEmpty(channel.Models)
	var imageProvider generation.ImageProvider
	if strings.EqualFold(strings.TrimSpace(channel.Protocol), "cloudbase-function") {
		timeoutMS := intConfigValue(os.Getenv("CLOUDBASE_IMAGE_TIMEOUT_MS"))
		if timeoutMS <= 0 {
			timeoutMS = 150000
		}
		imageProvider = imageprovider.NewCloudBaseFunction(imageprovider.CloudBaseFunctionOptions{
			Code: channel.ID, FunctionURL: channel.BaseURL, APIKey: apiKey, DefaultModel: model, Models: channel.Models,
			WatermarkText: firstNonEmptyString(strings.TrimSpace(os.Getenv("CLOUDBASE_AI_WATERMARK_TEXT")), "AI生成"), TimeoutMS: timeoutMS,
		})
	} else {
		imageProvider = imageprovider.NewOpenAICompatibleWithOptions(imageprovider.OpenAICompatibleOptions{
			Code:                    channel.ID,
			BaseURL:                 channel.BaseURL,
			APIKey:                  apiKey,
			ImageModel:              model,
			Models:                  channel.Models,
			ImageGenerationEndpoint: channel.ImageGenerationEndpoint,
			ImageEditEndpoint:       channel.ImageEditEndpoint,
			ReferenceImageDir:       a.referenceImageDir(),
			TimeoutMS:               int(a.cfg.ImageProviderTimeout() / time.Millisecond),
		})
	}
	var videoProvider generation.VideoProvider
	if !strings.EqualFold(strings.TrimSpace(channel.Protocol), "cloudbase-function") {
		videoProvider = videoprovider.NewOpenAICompatibleWithOptions(videoprovider.OpenAICompatibleOptions{
			Code: channel.ID, BaseURL: channel.BaseURL, APIKey: apiKey, Model: model, Models: channel.Models,
			Endpoint: channel.VideoGenerationEndpoint, TimeoutMS: intConfigValue(a.cfg.ModelTimeoutMS),
			OutputDir: a.generatedMediaDir(), PublicURLBase: "/api/v1/generated-media/",
		})
	}
	return generation.NewServiceWithOptions(generation.ServiceOptions{
		ImageProvider:  imageProvider,
		VideoProvider:  videoProvider,
		ImageDecorator: generatedImageDecorator{},
		CreateTask: func(req generation.CreateRequest) (any, error) {
			if req.Params == nil {
				req.Params = map[string]any{}
			}
			req.Params["provider"] = channel.ID
			req.Params["providerName"] = channel.Name
			if route.ID != "" {
				req.Params["modelRouteId"] = route.ID
				req.Params["modelGroup"] = route.GroupName
				req.Params["modelApiKeyId"] = route.APIKeyID
			}
			return a.store.CreateGenerationTask(req)
		},
	}), nil
}

func apiRouteUsableForGeneration(route adminUserModelRoute) bool {
	return route.ID != "" && (strings.EqualFold(route.Status, "ACTIVE") || strings.EqualFold(route.Status, "ENABLED"))
}

func findAPIChannelByRoute(channels []adminAPIChannel, route adminUserModelRoute) (adminAPIChannel, bool) {
	for _, channel := range channels {
		if route.ChannelID != "" && channel.ID == route.ChannelID && apiChannelUsableForGeneration(channel) {
			return channel, true
		}
	}
	for _, channel := range channels {
		if route.Channel != "" && strings.EqualFold(channel.Name, route.Channel) && apiChannelUsableForGeneration(channel) {
			return channel, true
		}
	}
	return adminAPIChannel{}, false
}

func savedAPIKeyByID(keys []adminAPIKey, id string) string {
	for _, key := range keys {
		if key.ID == id && strings.EqualFold(key.Status, "ACTIVE") && strings.TrimSpace(key.Secret) != "" {
			return strings.TrimSpace(key.Secret)
		}
	}
	return ""
}

func stringListContains(items []string, value string) bool {
	value = strings.TrimSpace(value)
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), value) {
			return true
		}
	}
	return false
}

func appendIfMissingString(items []string, value string) []string {
	if stringListContains(items, value) {
		return items
	}
	return append(items, value)
}

func selectAPIChannelForModel(channels []adminAPIChannel, model string) (adminAPIChannel, bool) {
	model = strings.TrimSpace(model)
	var fallback adminAPIChannel
	var matched adminAPIChannel
	hasFallback := false
	hasMatched := false
	for _, channel := range channels {
		if !apiChannelUsableForGeneration(channel) {
			continue
		}
		if !hasFallback || priorityLess(channel, fallback) {
			fallback = channel
			hasFallback = true
		}
		if model != "" && apiChannelSupportsModel(channel, model) {
			if !hasMatched || priorityLess(channel, matched) {
				matched = channel
				hasMatched = true
			}
		}
	}
	if hasMatched {
		return matched, true
	}
	if model == "" && hasFallback {
		return fallback, true
	}
	return adminAPIChannel{}, false
}

func configuredGenerationChannels(data adminPlatformData) []adminAPIChannel {
	channels := annotateAPIChannelsWithKeys(data.APIChannels, data.APIKeys)
	if cloudBaseChannel, ok := runtimeCloudBaseChannelFromEnv(); ok {
		filtered := make([]adminAPIChannel, 0, len(channels)+1)
		filtered = append(filtered, cloudBaseChannel)
		for _, channel := range channels {
			if strings.EqualFold(channel.ID, cloudBaseChannel.ID) {
				continue
			}
			filtered = append(filtered, channel)
		}
		channels = filtered
	}
	if runtimeChannel, ok := runtimeGenerationChannelFromEnv(); ok {
		filtered := make([]adminAPIChannel, 0, len(channels)+1)
		filtered = append(filtered, runtimeChannel)
		for _, channel := range channels {
			if strings.EqualFold(channel.ID, runtimeChannel.ID) {
				continue
			}
			filtered = append(filtered, channel)
		}
		channels = filtered
	}
	defaultModels := configuredImageModels(data)
	result := make([]adminAPIChannel, 0, len(channels))
	for _, channel := range channels {
		if !apiChannelUsableForGeneration(channel) || !apiChannelHasCredential(data.APIKeys, channel) {
			continue
		}
		if len(nonEmptyStringItems(channel.Models...)) == 0 {
			channel.Models = defaultModels
		}
		result = append(result, channel)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return priorityLess(result[i], result[j])
	})
	return result
}

func runtimeCloudBaseChannelFromEnv() (adminAPIChannel, bool) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("CLOUDBASE_ENABLED")), "true") {
		return adminAPIChannel{}, false
	}
	apiKey := strings.TrimSpace(os.Getenv("CLOUDBASE_API_KEY"))
	functionURL := strings.TrimSpace(os.Getenv("CLOUDBASE_IMAGE_FUNCTION_URL"))
	if functionURL == "" {
		envID := strings.TrimSpace(os.Getenv("CLOUDBASE_ENV_ID"))
		functionName := strings.TrimSpace(os.Getenv("CLOUDBASE_IMAGE_FUNCTION_NAME"))
		if envID != "" && functionName != "" {
			functionURL = fmt.Sprintf("https://%s.api.tcloudbasegateway.com/v1/functions/%s", envID, url.PathEscape(functionName))
		}
	}
	if apiKey == "" || functionURL == "" {
		return adminAPIChannel{}, false
	}
	return adminAPIChannel{
		ID: "channel_cloudbase_miniprogram", Name: "CloudBase 小程序合规生图", BaseURL: functionURL,
		Protocol: "cloudbase-function", ImageRequestMode: "cloudbase-function", APIKeyEnv: "CLOUDBASE_API_KEY",
		Primary: true, Status: "ACTIVE", Priority: 1,
		Models: []string{"HY-Image-3.0-Plus-4090-Tob-v1.0", "HY-Image-v3.0-I2I-ToB-v1.0.1"}, APIKeyConfigured: true,
	}, true
}

func (a api) generationServiceForCompliantMiniProgram(req generation.CreateRequest) (generation.Service, bool, error) {
	if !strings.EqualFold(stringValue(req.Params["terminal"]), terminalMiniProgram) {
		return generation.Service{}, false, nil
	}
	data, err := a.onlineGenerationSettings()
	if err != nil {
		return generation.Service{}, false, err
	}
	channel, ok, routeErr := selectAPIChannelForConfiguredModel(data, req.Model)
	if routeErr != nil {
		return generation.Service{}, false, routeErr
	}
	if !ok {
		return generation.Service{}, false, fmt.Errorf("mini-program compliant model %s is not configured", req.Model)
	}
	service, serviceErr := a.generationServiceForChannel(data, channel)
	return service, serviceErr == nil, serviceErr
}

func runtimeGenerationChannelFromEnv() (adminAPIChannel, bool) {
	baseURL := strings.TrimSpace(os.Getenv("MODEL_PROVIDER_URL"))
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
	}
	if baseURL == "" {
		return adminAPIChannel{}, false
	}
	apiKeyEnv := "MODEL_PROVIDER_API_KEY"
	if strings.TrimSpace(os.Getenv(apiKeyEnv)) == "" && strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "" {
		apiKeyEnv = "OPENAI_API_KEY"
	}
	if strings.TrimSpace(os.Getenv(apiKeyEnv)) == "" {
		return adminAPIChannel{}, false
	}
	model := strings.TrimSpace(os.Getenv("MODEL_PROVIDER_IMAGE_MODEL"))
	if model == "" {
		model = "gpt-image-2"
	}
	models := []string{model}
	if videoModel := strings.TrimSpace(os.Getenv("MODEL_PROVIDER_VIDEO_MODEL")); videoModel != "" {
		models = appendIfMissingString(models, videoModel)
	}
	return adminAPIChannel{
		ID:                      "channel_runtime_env",
		Name:                    "当前运行上游",
		BaseURL:                 baseURL,
		Protocol:                "openai",
		ImageRequestMode:        "openai",
		ImageGenerationEndpoint: "/v1/images/generations",
		ImageEditEndpoint:       "/v1/images/edits",
		VideoGenerationEndpoint: "",
		FetchModelsPath:         "/models",
		APIKeyEnv:               apiKeyEnv,
		Primary:                 true,
		Status:                  "ACTIVE",
		Priority:                1,
		Models:                  models,
		APIKeyConfigured:        true,
	}, true
}

func nonEmptyStringItems(values ...string) []string {
	items := []string{}
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			items = append(items, text)
		}
	}
	return items
}

func configuredImageModels(data adminPlatformData) []string {
	models := []string{}
	seen := map[string]bool{}
	for _, item := range data.APIModels {
		capability := strings.ToUpper(strings.TrimSpace(item.Capability))
		status := strings.ToUpper(strings.TrimSpace(item.Status))
		if status != "" && status != "ACTIVE" && status != "ENABLED" {
			continue
		}
		if capability != "" && !strings.Contains(capability, "IMAGE") && !strings.Contains(capability, "TEXT_TO_IMAGE") {
			continue
		}
		model := strings.TrimSpace(item.Model)
		if model == "" || strings.EqualFold(model, "mock-standard") || seen[model] {
			continue
		}
		seen[model] = true
		models = append(models, model)
	}
	if len(models) == 0 {
		models = append(models, "gpt-image-2")
	}
	return models
}

func apiChannelHasCredential(keys []adminAPIKey, channel adminAPIChannel) bool {
	if strings.TrimSpace(channel.APIKeyEnv) != "" && strings.TrimSpace(os.Getenv(channel.APIKeyEnv)) != "" {
		return true
	}
	return strings.TrimSpace(savedAPIKeyForChannel(keys, channel)) != ""
}

func apiChannelUsableForGeneration(channel adminAPIChannel) bool {
	if channel.ID == "" {
		return false
	}
	return strings.EqualFold(channel.Status, "ACTIVE") || strings.EqualFold(channel.Status, "CONFIGURABLE") || strings.EqualFold(channel.Status, "ENABLED")
}

func apiChannelSupportsModel(channel adminAPIChannel, model string) bool {
	for _, item := range channel.Models {
		if strings.EqualFold(strings.TrimSpace(item), model) {
			return true
		}
	}
	return false
}

func priorityLess(left adminAPIChannel, right adminAPIChannel) bool {
	if left.Primary != right.Primary {
		return left.Primary
	}
	leftPriority := left.Priority
	if leftPriority <= 0 {
		leftPriority = 1000
	}
	rightPriority := right.Priority
	if rightPriority <= 0 {
		rightPriority = 1000
	}
	return leftPriority < rightPriority
}

func firstNonEmpty(items []string) string {
	for _, item := range items {
		if text := strings.TrimSpace(item); text != "" {
			return text
		}
	}
	return ""
}

func intConfigValue(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}

func (a api) models(w http.ResponseWriter, r *http.Request) {
	items := []map[string]any{
		{"code": "mock-standard", "name": "本地演示模型", "capabilities": []string{"TEXT_TO_IMAGE"}, "online": true, "pointCost": 1},
	}
	seen := map[string]bool{"mock-standard": true}
	data, err := a.store.AdminData()
	if err == nil {
		data = normalizeAICapabilityDefaults(data)
		miniProgram := isWeChatMiniProgramRequest(r)
		approvedModels := map[string]adminAIModel{}
		if miniProgram {
			items = items[:0]
			seen = map[string]bool{}
		}
		for _, model := range data.AIModels {
			code := strings.TrimSpace(model.ModelName)
			key := strings.ToLower(code)
			if code == "" || !isActiveLike(model.Status) {
				continue
			}
			localModel := strings.EqualFold(code, "mock-standard") || strings.EqualFold(code, "mock-video")
			_, routed, routeErr := selectAPIChannelForConfiguredModel(data, code)
			if !localModel && (!routed || routeErr != nil) {
				continue
			}
			if miniProgram {
				allowed, _ := modelAllowedForMiniProgram(model, time.Now().UTC())
				if !allowed && !miniProgramVideoComplianceBypassAllows(model) {
					continue
				}
			}
			approvedModels[key] = model
			if seen[key] {
				continue
			}
			seen[key] = true
			schema := findAIParameterSchema(data.AIParameterSchemas, model.ModuleCode, model.ModelName)
			if canonicalModuleCode(model.ModuleCode) == moduleImageGeneration {
				schema = findExactAIParameterSchema(data.AIParameterSchemas, moduleImageGeneration, model.ModelName)
				if schema.ID == "" {
					continue
				}
			}
			capabilities := publicModelCapabilities(model, schema.SchemaJSON)
			item := map[string]any{
				"code": code, "name": code, "capabilities": capabilities,
				"online": true, "pointCost": modelPointCost(code),
			}
			if isVideoAIModel(model) {
				videoCapabilities := resolveVideoModelCapabilities(model, schema.SchemaJSON)
				item["videoCapabilities"] = videoCapabilities
				item["video_capabilities"] = videoCapabilities
				attachVideoModelPublicPricing(item, data, code, videoCapabilities)
			}
			items = append(items, item)
		}
		for _, channel := range configuredGenerationChannels(data) {
			for _, model := range channel.Models {
				code := strings.TrimSpace(model)
				key := strings.ToLower(code)
				if miniProgram {
					if _, ok := approvedModels[key]; !ok {
						continue
					}
				}
				if code == "" || seen[key] {
					continue
				}
				configuredModel := findAIModel(data.AIModels, moduleImageGeneration, code)
				if configuredModel.ID == "" || !isActiveLike(configuredModel.Status) {
					continue
				}
				if findExactAIParameterSchema(data.AIParameterSchemas, moduleImageGeneration, code).ID == "" {
					continue
				}
				seen[key] = true
				items = append(items, map[string]any{
					"code":         code,
					"name":         code,
					"capabilities": []string{"TEXT_TO_IMAGE", "IMAGE_TO_IMAGE"},
					"online":       true,
					"pointCost":    10,
					"providerId":   channel.ID,
					"providerName": channel.Name,
				})
			}
		}
	}
	for _, item := range items {
		item["id"] = item["code"]
		if _, ok := item["displayName"]; !ok {
			item["displayName"] = item["name"]
		}
		if _, ok := item["description"]; !ok {
			item["description"] = ""
		}
		item["enabled"] = item["online"]
		// Public model discovery must not reveal upstream routing or vendor identity.
		delete(item, "providerId")
		delete(item, "providerName")
		delete(item, "provider")
	}
	writeJSON(w, sortPublicModelsVideoByListPrice(items))
}
func (a api) listAssets(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	query := assetCenterQueryFromRequest(r)
	limit := query.Limit
	offset := query.Offset
	assets, total, err := a.assetsForCenter(user.ID, firstNonEmptyString(user.TenantID, "tenant_default"), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	lightweight := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("lightweight")), "true") || strings.TrimSpace(r.URL.Query().Get("lightweight")) == "1"
	if lightweight {
		// Asset grids only need compact covers. The full original is signed by
		// the detail/download endpoint after the user opens a work.
		for index := range assets {
			assets[index].URL = ""
		}
	} else {
		assets = a.signStoredAssetURLs(r.Context(), user.ID, assets)
	}
	assets = secureAssetsForClient(assets)
	if pagedListRequested(r) {
		response := map[string]any{
			"items":    assets,
			"total":    total,
			"limit":    limit,
			"offset":   offset,
			"page":     offset/limit + 1,
			"pageSize": limit,
			"hasMore":  offset+len(assets) < total,
		}
		if !strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("includeSummary")), "false") {
			summary, summaryErr := a.assetListSummaryForUser(user.ID)
			if summaryErr != nil {
				writeError(w, http.StatusInternalServerError, summaryErr)
				return
			}
			summary.Total = total
			response["summary"] = summary
		}
		writeJSON(w, response)
		return
	}
	writeJSON(w, assets)
}

func (a api) uploadReferenceImage(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxReferenceImageUploadBytes+(1<<20))
	file, header, err := r.FormFile("file")
	if err != nil {
		file, header, err = r.FormFile("files")
	}
	if err != nil {
		file, header, err = r.FormFile("image[]")
	}
	if err != nil {
		file, header, err = r.FormFile("image")
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("missing image file"))
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxReferenceImageUploadBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(raw) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("empty image file"))
		return
	}
	if len(raw) > maxReferenceImageUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, errors.New("image file is too large"))
		return
	}
	contentType := detectReferenceImageContentType(raw, header.Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "image/") {
		writeError(w, http.StatusBadRequest, errors.New("unsupported image file"))
		return
	}
	if a.contentSecurity != nil {
		if err := a.contentSecurity.CheckImage(r.Context(), raw, header.Filename, contentType); err != nil {
			writeContentSecurityError(w, err)
			return
		}
	} else if isWeChatMiniProgramRequest(r) {
		writeContentSecurityError(w, errContentSecurityUnavailable)
		return
	}
	dir := a.referenceImageDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	name, err := randomReferenceImageName(referenceImageExtension(header.Filename, contentType))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	assetID := strings.TrimSuffix(name, filepath.Ext(name))
	item, err := a.store.SaveUploadedAsset(asset{
		ID: assetID, UserID: user.ID, TenantID: effectiveTenantID(user),
		Name: header.Filename, MediaType: "image", URL: "/api/v1/reference-images/" + name,
		Metadata:  map[string]any{"contentType": contentType, "size": len(raw), "sourceType": "reference_upload"},
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		_ = os.Remove(filepath.Join(dir, name))
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{
		"item": map[string]any{
			"id":          item.ID,
			"assetId":     item.ID,
			"name":        header.Filename,
			"storedName":  name,
			"url":         "/api/v1/reference-images/" + name,
			"contentType": contentType,
			"size":        len(raw),
		},
	})
}

func (a api) serveReferenceImage(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" || filepath.Base(name) != name {
		writeError(w, http.StatusBadRequest, errors.New("invalid image name"))
		return
	}
	path := filepath.Join(a.referenceImageDir(), name)
	if _, err := os.Stat(path); err != nil {
		writeError(w, http.StatusNotFound, errors.New("reference image not found"))
		return
	}
	http.ServeFile(w, r, path)
}

func (a api) downloadAsset(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	id := r.PathValue("id")
	assets, err := a.assetsForUser(r, user.ID, maxUserContentListLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, item := range assets {
		if item.ID != id {
			continue
		}
		if item.UserID != user.ID {
			writeError(w, http.StatusNotFound, errAssetNotFound)
			return
		}
		if strings.TrimSpace(item.URL) == "" && assetStorageFileID(item) == "" {
			writeError(w, http.StatusNotFound, errAssetNotFound)
			return
		}
		a.writeAssetDownload(w, r, item)
		return
	}
	writeError(w, http.StatusNotFound, errAssetNotFound)
}

func (a api) downloadVideoByURL(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if rawURL == "" {
		writeError(w, http.StatusBadRequest, errors.New("video url is required"))
		return
	}
	tasks, err := a.generationTasksForUser(r, user.ID, maxUserContentListLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	assets, err := a.assetsForUser(r, user.ID, maxUserContentListLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	userAssets := filterAssetsForUser(assets, user.ID)
	if !videoURLBelongsToUser(rawURL, filterGenerationTasksForUser(attachAssetImagesToTasks(tasks, userAssets), user.ID), userAssets) {
		writeError(w, http.StatusNotFound, errors.New("video not found"))
		return
	}
	filename := sanitizeVideoDownloadFilename(r.URL.Query().Get("filename"))
	if filename == "" {
		filename = "video.mp4"
	}
	if _, ok := generatedMediaNameFromURL(rawURL); ok {
		a.writeGeneratedMediaDownload(w, r.Context(), rawURL, filename)
		return
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		writeError(w, http.StatusBadRequest, errors.New("invalid video url"))
		return
	}
	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		writeError(w, http.StatusBadRequest, errors.New("unsupported video url scheme"))
		return
	}
	a.writeNormalizedVideoDownload(w, r, rawURL, filename)
}

func videoURLBelongsToUser(rawURL string, tasks []generationTask, assets []asset) bool {
	for _, item := range assets {
		if videoURLsEqual(item.URL, rawURL) && strings.EqualFold(item.MediaType, "video") {
			return true
		}
	}
	for _, task := range tasks {
		if videoURLsEqual(task.OutputURL, rawURL) || videoURLsEqual(task.ResultURL, rawURL) || videoURLsEqual(task.ImageURL, rawURL) {
			return true
		}
	}
	return false
}

func videoURLsEqual(left string, right string) bool {
	if left == right {
		return true
	}
	leftName, leftOK := generatedMediaNameFromURL(left)
	rightName, rightOK := generatedMediaNameFromURL(right)
	return leftOK && rightOK && leftName == rightName
}

func (a api) writeAssetDownload(w http.ResponseWriter, r *http.Request, item asset) {
	if isVideoAsset(item) {
		filename := sanitizeVideoDownloadFilename(downloadAssetName(item, "video/mp4"))
		if a.writeCompliantVideoDownload(w, r, item, filename) {
			return
		}
		if raw, _, ok := a.readStoredAssetBytes(r.Context(), item); ok {
			a.writeShareableVideoBytes(w, r.Context(), raw, item.URL, filename, false)
			return
		}
		if _, ok := generatedMediaNameFromURL(item.URL); ok {
			a.writeGeneratedMediaDownload(w, r.Context(), item.URL, filename)
			return
		}
		if strings.TrimSpace(item.URL) == "" {
			writeError(w, http.StatusNotFound, errAssetNotFound)
			return
		}
		a.writeNormalizedVideoDownload(w, r, item.URL, filename)
		return
	}
	if a.writeCompliantAssetDownload(w, r, item) {
		return
	}
	if a.writeStoredAssetDownload(w, r, item) {
		return
	}
	contentType := stringMetadataValue(item, "contentType")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if _, ok := generatedMediaNameFromURL(item.URL); ok {
		a.writeGeneratedMediaDownload(w, r.Context(), item.URL, downloadAssetName(item, "video/mp4"))
		return
	}
	if strings.HasPrefix(item.URL, "data:") {
		comma := strings.IndexByte(item.URL, ',')
		if comma < 0 {
			writeError(w, http.StatusBadRequest, errors.New("invalid data URL"))
			return
		}
		header := item.URL[:comma]
		if mediaType := strings.TrimPrefix(strings.Split(header, ";")[0], "data:"); mediaType != "" {
			contentType = mediaType
		}
		raw := []byte(item.URL[comma+1:])
		if strings.Contains(header, ";base64") {
			decoded, err := base64.StdEncoding.DecodeString(string(raw))
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			raw = decoded
		}
		writeAttachmentHeaders(w, contentType, downloadAssetName(item, contentType))
		_, _ = w.Write(raw)
		return
	}
	if strings.TrimSpace(item.URL) == "" {
		writeError(w, http.StatusNotFound, errAssetNotFound)
		return
	}
	a.writeRemoteDownload(w, r, item.URL, contentType, downloadAssetName(item, contentType))
}

func assetStorageFileID(item asset) string {
	return firstNonEmptyString(stringValue(item.Metadata["fileId"]), stringValue(item.Metadata["storageFileId"]))
}

func (a api) writeStoredAssetDownload(w http.ResponseWriter, r *http.Request, item asset) bool {
	raw, contentType, ok := a.readStoredAssetBytes(r.Context(), item)
	if !ok {
		return false
	}
	if contentType == "" {
		contentType = firstNonEmptyString(stringMetadataValue(item, "contentType"), "application/octet-stream")
	}
	writeAttachmentHeaders(w, contentType, downloadAssetName(item, contentType))
	_, _ = w.Write(raw)
	return true
}

func (a api) readStoredAssetBytes(ctx context.Context, item asset) ([]byte, string, bool) {
	if a.fileService == nil {
		return nil, "", false
	}
	fileID := firstNonEmptyString(stringValue(item.Metadata["fileId"]), stringValue(item.Metadata["storageFileId"]))
	if fileID == "" {
		return nil, "", false
	}
	tenantID := firstNonEmptyString(item.TenantID, stringValue(item.Metadata["storageTenantId"]), "tenant_default")
	userID := firstNonEmptyString(item.UserID)
	file, stream, err := a.fileService.OpenObject(ctx, storagecenter.AccessContext{
		TenantID: tenantID,
		UserID:   userID,
	}, fileID)
	if err != nil {
		return nil, "", false
	}
	defer stream.Close()
	raw, err := io.ReadAll(io.LimitReader(stream, 512<<20))
	if err != nil || len(raw) == 0 {
		return nil, "", false
	}
	contentType := firstNonEmptyString(file.MIMEType, stringMetadataValue(item, "contentType"), item.MediaType)
	return raw, contentType, true
}

func (a api) serveGeneratedMedia(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" || filepath.Base(name) != name || !strings.HasSuffix(strings.ToLower(name), ".mp4") {
		writeError(w, http.StatusBadRequest, errors.New("invalid generated media name"))
		return
	}
	path := filepath.Join(a.generatedMediaDir(), name)
	if _, err := os.Stat(path); err != nil {
		writeError(w, http.StatusNotFound, errors.New("generated media not found"))
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	http.ServeFile(w, r, path)
}

func (a api) writeGeneratedMediaDownload(w http.ResponseWriter, ctx context.Context, rawURL string, filename string) {
	name, ok := generatedMediaNameFromURL(rawURL)
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("invalid generated media url"))
		return
	}
	file, err := os.Open(filepath.Join(a.generatedMediaDir(), name))
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("generated media not found"))
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxShareVideoNormalizeBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxShareVideoNormalizeBytes {
		writeError(w, http.StatusBadGateway, errors.New("generated media read failed"))
		return
	}
	a.writeShareableVideoBytes(w, ctx, raw, rawURL, filename, false)
}

func (a api) writeNormalizedVideoDownload(w http.ResponseWriter, r *http.Request, rawURL string, filename string) {
	remoteURL, err := validateRemoteDownloadURL(rawURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, remoteURL.String(), nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	res, err := remoteDownloadHTTPClient().Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, fmt.Errorf("asset download returned %d", res.StatusCode))
		return
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxShareVideoNormalizeBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxShareVideoNormalizeBytes {
		writeError(w, http.StatusBadGateway, errors.New("video download payload invalid"))
		return
	}
	a.writeShareableVideoBytes(w, r.Context(), raw, rawURL, filename, false)
}

func (a api) writeShareableVideoBytes(w http.ResponseWriter, ctx context.Context, raw []byte, sourceURL string, filename string, aiGenerated bool) {
	filename = sanitizeVideoDownloadFilename(filename)
	normalized, err := normalizeVideoBytesForShare(ctx, raw, sourceURL)
	if err != nil || len(normalized) == 0 {
		normalized = raw
	}
	writeAttachmentHeaders(w, "video/mp4", filename)
	w.Header().Set("Content-Length", strconv.Itoa(len(normalized)))
	w.Header().Set("X-Video-Share-Format", "mp4")
	if aiGenerated {
		w.Header().Set("X-AI-Generated", "true")
	}
	_, _ = w.Write(normalized)
}

func (a api) writeCompliantVideoDownload(w http.ResponseWriter, r *http.Request, item asset, filename string) bool {
	if !boolValue(item.Metadata["ai_generated"]) {
		return false
	}
	if !strings.EqualFold(stringMetadataValue(item, "output_audit_status"), auditApproved) {
		writeError(w, http.StatusUnprocessableEntity, errOutputAuditRejected)
		return true
	}
	if markedURL := strings.TrimSpace(stringMetadataValue(item, "download_marked_url")); markedURL != "" {
		a.writeNormalizedVideoDownload(w, r, markedURL, filename)
		return true
	}
	raw, _, err := func() ([]byte, string, error) {
		if stored, storedType, ok := a.readStoredAssetBytes(r.Context(), item); ok {
			return stored, firstNonEmptyString(storedType, item.MediaType), nil
		}
		payload, mediaType, _, readErr := readGeneratedArtifact(r.Context(), item.URL, item.MediaType)
		return payload, mediaType, readErr
	}()
	if err == nil && len(raw) > 0 {
		a.writeShareableVideoBytes(w, r.Context(), raw, item.URL, filename, true)
		return true
	}
	writeError(w, http.StatusServiceUnavailable, errors.New("带AI标识的下载文件尚未生成，请稍后重试"))
	return true
}

func (a api) writeRemoteDownload(w http.ResponseWriter, r *http.Request, rawURL string, contentType string, filename string) {
	remoteURL, err := validateRemoteDownloadURL(rawURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, remoteURL.String(), nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	res, err := remoteDownloadHTTPClient().Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, fmt.Errorf("asset download returned %d", res.StatusCode))
		return
	}
	if res.Header.Get("Content-Type") != "" {
		contentType = res.Header.Get("Content-Type")
	}
	writeAttachmentHeaders(w, contentType, filename)
	_, _ = io.Copy(w, io.LimitReader(res.Body, 512<<20))
}

func validateRemoteDownloadURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("invalid remote download url")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return nil, errors.New("unsupported remote download url scheme")
	}
	if parsed.User != nil {
		return nil, errors.New("remote download url userinfo is not allowed")
	}
	if err := validateRemoteDownloadHost(parsed.Hostname()); err != nil {
		return nil, err
	}
	return parsed, nil
}

func validateRemoteDownloadHost(host string) error {
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return errors.New("remote download host is required")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return errors.New("remote download host is not public")
	}
	if ip := net.ParseIP(host); ip != nil && !isPublicRemoteDownloadIP(ip) {
		return errors.New("remote download address is not public")
	}
	return nil
}

func remoteDownloadHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, item := range ips {
			if !isPublicRemoteDownloadIP(item.IP) {
				return nil, fmt.Errorf("remote download host resolves to non-public address: %s", host)
			}
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
	}
	return &http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many remote download redirects")
			}
			_, err := validateRemoteDownloadURL(req.URL.String())
			return err
		},
	}
}

func isPublicRemoteDownloadIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return !ip.IsLoopback() &&
		!ip.IsPrivate() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() &&
		!ip.IsUnspecified() &&
		!ip.IsMulticast()
}

func (a api) referenceImageDir() string {
	base := filepath.Dir(a.cfg.DataPath)
	if base == "." || base == "" {
		base = "data"
	}
	return filepath.Join(base, "reference-images")
}

func (a api) generatedMediaDir() string {
	base := filepath.Dir(a.cfg.DataPath)
	if base == "." || base == "" {
		base = "data"
	}
	return filepath.Join(base, "generated-media")
}

func generatedMediaNameFromURL(rawURL string) (string, bool) {
	const prefix = "/api/v1/generated-media/"
	text := strings.TrimSpace(rawURL)
	if text == "" {
		return "", false
	}
	pathValue := text
	if parsed, err := url.Parse(text); err == nil {
		if parsed.Path != "" {
			pathValue = parsed.Path
		}
	}
	if !strings.HasPrefix(pathValue, prefix) {
		return "", false
	}
	name, err := url.PathUnescape(path.Base(pathValue))
	if err != nil || name == "" || filepath.Base(name) != name || !strings.HasSuffix(strings.ToLower(name), ".mp4") {
		return "", false
	}
	return name, true
}

func detectReferenceImageContentType(raw []byte, declared string) string {
	contentType := ""
	if len(raw) > 0 {
		limit := len(raw)
		if limit > 512 {
			limit = 512
		}
		contentType = http.DetectContentType(raw[:limit])
	}
	if !strings.HasPrefix(contentType, "image/") && strings.HasPrefix(declared, "image/") {
		contentType = strings.Split(declared, ";")[0]
	}
	return contentType
}

func referenceImageExtension(filename string, contentType string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		return ext
	}
	switch strings.ToLower(strings.Split(contentType, ";")[0]) {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

func randomReferenceImageName(ext string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x%s", raw, ext), nil
}

func writeAttachmentHeaders(w http.ResponseWriter, contentType string, filename string) {
	w.Header().Set("Content-Type", contentType)
	asciiFilename := regexp.MustCompile(`[^\x20-\x7E]+`).ReplaceAllString(filename, "_")
	asciiFilename = strings.ReplaceAll(asciiFilename, `"`, "-")
	if strings.TrimSpace(asciiFilename) == "" {
		asciiFilename = "download"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, asciiFilename, url.PathEscape(filename)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func downloadAssetName(item asset, contentType string) string {
	name := regexp.MustCompile(`[\\/:*?"<>|]+`).ReplaceAllString(item.Name, "-")
	if name == "" {
		name = item.ID
	}
	if strings.Contains(strings.ToLower(contentType), "video") || strings.EqualFold(item.MediaType, "video") {
		return sanitizeVideoDownloadFilename(name)
	}
	if regexp.MustCompile(`(?i)\.(png|jpe?g|webp|gif|svg|mp4)$`).MatchString(name) {
		return name
	}
	switch {
	case strings.Contains(contentType, "svg"):
		return name + ".svg"
	case strings.Contains(contentType, "jpeg"), strings.Contains(contentType, "jpg"):
		return name + ".jpg"
	case strings.Contains(contentType, "webp"):
		return name + ".webp"
	default:
		return name + ".png"
	}
}

func stringMetadataValue(item asset, key string) string {
	value, ok := item.Metadata[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func (a api) backfillAssetThumbnails(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	assets, err := a.assetsForUser(r, user.ID, maxUserContentListLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	thumbnailUpdates := map[string]string{}
	infoUpdates := map[string]assetImageInfo{}
	missing := 0
	for _, item := range assets {
		if item.UserID != user.ID || item.MediaType != "image" || item.URL == "" {
			continue
		}
		info := assetImageInfo{}
		needsThumbnail := item.ThumbnailURL == "" || item.ThumbnailURL == item.URL
		if needsThumbnail {
			missing++
			if thumbnailURL, width, height, ok := thumbnailAndDimensionsForImage(r.Context(), item.URL); ok {
				info.ThumbnailURL = thumbnailURL
				thumbnailUpdates[item.ID] = thumbnailURL
				info.Width = width
				info.Height = height
			}
		} else if width, height, ok := imageDimensionsForImage(r.Context(), item.URL); ok {
			info.Width = width
			info.Height = height
		}
		if info.ThumbnailURL != "" || info.Width > 0 || info.Height > 0 {
			infoUpdates[item.ID] = info
		}
	}

	updated, err := a.store.UpdateAssetThumbnails(thumbnailUpdates)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	infoUpdated, err := a.store.UpdateAssetImageInfo(infoUpdates)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{
		"ok":          true,
		"missing":     missing,
		"updated":     updated,
		"infoUpdated": infoUpdated,
	})
}

func (a api) pointAccount(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	account, err := a.store.PointAccount(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	pointService, err := personalPointServiceForStore(a.store)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	summary, err := pointService.Summary(r.Context(), account.ID, user.ID, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// account.Total is lifetime (available + frozen + consumed). summary.Total is only
	// available+frozen and would make the sidebar show identical available/total values.
	totalUsed := account.Total - int(summary.Available) - int(summary.Frozen)
	if totalUsed < 0 {
		totalUsed = 0
	}
	accountView := map[string]any{
		"id": account.ID, "userId": user.ID,
		"available": summary.Available, "frozen": summary.Frozen, "total": account.Total,
		"totalUsed": totalUsed, "totalGranted": account.Total,
		"permanentAvailable": summary.PermanentAvailable, "expiringAvailable": summary.ExpiringAvailable,
		"nextExpiryPoints": summary.NextExpiryPoints,
	}
	if !summary.NextExpiryAt.IsZero() {
		accountView["nextExpiryAt"] = summary.NextExpiryAt.UTC().Format(time.RFC3339Nano)
	}
	data, err := a.userAccountData(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	orders := userMembershipOrders(data, user.ID)
	writeJSON(w, map[string]any{
		"account":      accountView,
		"orders":       orders,
		"transactions": userPointTransactions(data.BillingEvents, user.ID),
	})
}

func (a api) plans(w http.ResponseWriter, r *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	planType := normalizePlanTypeString(r.URL.Query().Get("planType"))
	if planType == "" {
		planType = normalizePlanTypeString(r.URL.Query().Get("type"))
	}
	items := make([]map[string]any, 0, len(data.Plans))
	for _, plan := range data.Plans {
		if !commercePlanVisible(plan) {
			continue
		}
		if planType != "" && planBusinessType(plan) != planType {
			continue
		}
		items = append(items, commercePlanView(plan))
	}
	sort.SliceStable(items, func(i, j int) bool {
		return intValue(items[i]["sort"]) < intValue(items[j]["sort"])
	})
	writeJSON(w, map[string]any{"items": items})
}

func (a api) planDetail(w http.ResponseWriter, r *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	plan, ok := commercePlanByID(data, r.PathValue("id"))
	if !ok || !commercePlanVisible(plan) {
		writeError(w, http.StatusNotFound, errors.New("plan not found"))
		return
	}
	writeJSON(w, map[string]any{"item": commercePlanView(plan)})
}

func (a api) createCommerceOrder(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var req struct {
		PlanID         string `json:"planId"`
		AmountCents    int    `json:"amountCents"`
		PaymentMethod  string `json:"paymentMethod"`
		IdempotencyKey string `json:"idempotencyKey"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	if err := a.rejectManagedV2LegacyOrder(r.Context(), req.PlanID); err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	plan, ok := commercePlanByID(data, req.PlanID)
	if !ok || !commercePlanVisible(plan) {
		writeError(w, http.StatusBadRequest, errors.New("valid planId is required"))
		return
	}
	order, err := a.createOrderForPlan(user, plan, req.AmountCents, req.PaymentMethod, req.IdempotencyKey)
	if err != nil {
		writeCommerceOrderCreationError(w, err)
		return
	}
	writeJSON(w, commerceOrderResponse(order, plan))
}

func (a api) createAgentJoinOrder(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if agent, ok := channelAgentForUser(data.ChannelAgents, user.ID); ok && strings.EqualFold(agent.Status, "ACTIVE") {
		writeError(w, http.StatusConflict, errors.New("agent identity is already active"))
		return
	}
	var req struct {
		PlanID         string `json:"planId"`
		PaymentMethod  string `json:"paymentMethod"`
		IdempotencyKey string `json:"idempotencyKey"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	planID := firstNonEmptyString(req.PlanID, "plan_agent_join_996")
	if err := a.rejectManagedV2LegacyOrder(r.Context(), planID); err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	plan, ok := commercePlanByID(data, planID)
	if !ok || planBusinessType(plan) != planTypeAgentJoinPackage {
		writeError(w, http.StatusBadRequest, errors.New("valid agent join plan is required"))
		return
	}
	order, err := a.createOrderForPlan(user, plan, 0, req.PaymentMethod, req.IdempotencyKey)
	if err != nil {
		writeCommerceOrderCreationError(w, err)
		return
	}
	writeJSON(w, commerceOrderResponse(order, plan))
}

func (a api) rejectManagedV2LegacyOrder(ctx context.Context, planRef string) error {
	postgres, ok := a.store.(*postgresStore)
	if !ok || postgres == nil || postgres.db == nil || strings.TrimSpace(planRef) == "" {
		return nil
	}
	service := virtualPaymentService{db: postgres.db, cfg: virtualPaymentConfigFromApp(a.cfg)}
	managed, err := service.isManagedMemberAgentPlanRef(ctx, planRef)
	if err != nil {
		return err
	}
	if !managed {
		return nil
	}
	return newBusinessPlanAdminError(http.StatusConflict, "MANAGED_PLAN_REQUIRES_PRICE_QUOTE", "V2 managed member and agent plans must be ordered with a server-issued price quote")
}

func writeCommerceOrderCreationError(w http.ResponseWriter, err error) {
	var businessErr *businessPlanAdminError
	if errors.As(err, &businessErr) {
		writeBusinessPlanAdminError(w, businessErr)
		return
	}
	writeError(w, http.StatusBadRequest, err)
}

func (a api) createOperationCenterJoinOrder(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if center, ok := operationCenterForUser(data.OperationCenters, user.ID); ok && strings.EqualFold(center.Status, "ACTIVE") {
		writeError(w, http.StatusConflict, errors.New("operation center identity is already active"))
		return
	}
	var req struct {
		PlanID         string `json:"planId"`
		PaymentMethod  string `json:"paymentMethod"`
		IdempotencyKey string `json:"idempotencyKey"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	planID := firstNonEmptyString(req.PlanID, "plan_operation_center_5000")
	plan, ok := commercePlanByID(data, planID)
	if !ok || planBusinessType(plan) != planTypeOperationCenterPackage {
		writeError(w, http.StatusBadRequest, errors.New("valid operation center plan is required"))
		return
	}
	order, err := a.createOrderForPlan(user, plan, 0, req.PaymentMethod, req.IdempotencyKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, commerceOrderResponse(order, plan))
}

func (a api) payCallback(w http.ResponseWriter, r *http.Request) {
	if secret := strings.TrimSpace(a.cfg.PaymentCallbackSecret); secret != "" {
		headerSecret := strings.TrimSpace(firstNonEmptyString(r.Header.Get("X-Xianzhi-Payment-Secret"), r.Header.Get("X-Payment-Secret")))
		if headerSecret != secret {
			writeError(w, http.StatusUnauthorized, errors.New("invalid payment callback secret"))
			return
		}
	}
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req, signature, err := parsePaymentCallbackRequest(r, rawBody, a.cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	orderID := strings.TrimSpace(firstNonEmptyString(req.OrderID, req.OrderNo))
	if orderID == "" {
		writeError(w, http.StatusBadRequest, errors.New("orderId is required"))
		return
	}
	status := strings.ToUpper(strings.TrimSpace(firstNonEmptyString(req.Status, req.PaymentStatus, req.EventType)))
	if !req.Paid && status != "" && !isPaidStatus(status) && status != "PAYMENT_SUCCEEDED" {
		writeJSON(w, map[string]any{"ok": true, "ignored": true, "status": status})
		return
	}
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	order, ok := findPaymentCallbackOrder(data.Orders, req.OrderID, req.OrderNo)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Errorf("order not found: %s", orderID))
		return
	}
	if err := validatePaymentCallbackProvider(req, order); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	provider := paymentCallbackProvider(req, order)
	if err := a.requirePaymentCallbackSignature(provider, signature); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	amount := paymentCallbackAmountCents(req.AmountCents, req.TotalCents, req.Amount)
	if amount <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("payment amountCents is required"))
		return
	}
	if amount != orderAmount(order) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("payment amount mismatch: got %d, want %d", amount, orderAmount(order)))
		return
	}
	eventID := strings.TrimSpace(req.EventID)
	if eventID == "" {
		writeError(w, http.StatusBadRequest, errors.New("payment eventId is required"))
		return
	}
	providerTransactionID := strings.TrimSpace(firstNonEmptyString(req.ProviderTxnID, req.TransactionID))
	if providerTransactionID == "" {
		writeError(w, http.StatusBadRequest, errors.New("payment providerTransactionId is required"))
		return
	}
	duplicateEvent, err := a.store.RegisterPaymentCallbackEvent(adminPaymentEvent{
		Provider:      provider,
		EventID:       eventID,
		OrderID:       order.ID,
		TransactionID: providerTransactionID,
		AmountCents:   amount,
		Verified:      signature.Verified,
		Raw:           paymentCallbackEventRaw(req, signature),
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	callbackMetadata := paymentCallbackMetadata(eventID, providerTransactionID, status, amount)
	order, err = a.store.MarkAdminOrderPaid(order.ID, callbackMetadata)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{
		"ok":                    true,
		"item":                  order,
		"duplicateEvent":        duplicateEvent,
		"eventId":               eventID,
		"providerTransactionId": providerTransactionID,
	})
}

type paymentCallbackRequest struct {
	OrderID       string `json:"orderId"`
	OrderNo       string `json:"orderNo"`
	Provider      string `json:"provider"`
	Channel       string `json:"channel"`
	Status        string `json:"status"`
	PaymentStatus string `json:"paymentStatus"`
	EventType     string `json:"eventType"`
	Paid          bool   `json:"paid"`
	AmountCents   int    `json:"amountCents"`
	Amount        int    `json:"amount"`
	TotalCents    int    `json:"totalCents"`
	TransactionID string `json:"transactionId"`
	ProviderTxnID string `json:"providerTransactionId"`
	EventID       string `json:"eventId"`
}

type paymentCallbackSignature struct {
	Provider string
	Verified bool
	Source   string
}

func parsePaymentCallbackRequest(r *http.Request, rawBody []byte, cfg config.Config) (paymentCallbackRequest, paymentCallbackSignature, error) {
	if hasWeChatPaySignatureHeaders(r.Header) {
		return parseWeChatPayCallback(r, rawBody, cfg)
	}
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		return parseAlipayCallback(rawBody, cfg)
	}
	var req paymentCallbackRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		return paymentCallbackRequest{}, paymentCallbackSignature{}, err
	}
	return req, paymentCallbackSignature{}, nil
}

func paymentCallbackProvider(req paymentCallbackRequest, order adminOrder) string {
	provider := normalizePaymentMethod(firstNonEmptyString(req.Provider, req.Channel, stringValue(order.PriceSnapshot["paymentMethod"])))
	if provider == "" {
		return "manual"
	}
	return provider
}

func validatePaymentCallbackProvider(req paymentCallbackRequest, order adminOrder) error {
	reqProvider := normalizePaymentMethod(firstNonEmptyString(req.Provider, req.Channel))
	orderProvider := normalizePaymentMethod(stringValue(order.PriceSnapshot["paymentMethod"]))
	if reqProvider == "" || orderProvider == "" || reqProvider == orderProvider {
		return nil
	}
	return fmt.Errorf("payment callback provider mismatch: got %s, want %s", reqProvider, orderProvider)
}

func (a api) requirePaymentCallbackSignature(provider string, signature paymentCallbackSignature) error {
	if !a.cfg.IsProduction() {
		return nil
	}
	provider = normalizePaymentMethod(provider)
	if provider != "wechat" && provider != "alipay" {
		return nil
	}
	if signature.Verified && signature.Provider == provider {
		return nil
	}
	return fmt.Errorf("%s payment callback requires official signature verification in production", provider)
}

func hasWeChatPaySignatureHeaders(header http.Header) bool {
	return strings.TrimSpace(header.Get("Wechatpay-Signature")) != "" ||
		strings.TrimSpace(header.Get("Wechatpay-Timestamp")) != "" ||
		strings.TrimSpace(header.Get("Wechatpay-Nonce")) != ""
}

func parseWeChatPayCallback(r *http.Request, rawBody []byte, cfg config.Config) (paymentCallbackRequest, paymentCallbackSignature, error) {
	if err := verifyWeChatPaySignature(r.Header, rawBody, cfg); err != nil {
		return paymentCallbackRequest{}, paymentCallbackSignature{}, err
	}
	var envelope struct {
		ID           string `json:"id"`
		EventType    string `json:"event_type"`
		ResourceType string `json:"resource_type"`
		Summary      string `json:"summary"`
		Resource     struct {
			Algorithm      string `json:"algorithm"`
			Ciphertext     string `json:"ciphertext"`
			AssociatedData string `json:"associated_data"`
			Nonce          string `json:"nonce"`
			OriginalType   string `json:"original_type"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(rawBody, &envelope); err != nil {
		return paymentCallbackRequest{}, paymentCallbackSignature{}, err
	}
	plain, err := decryptWeChatPayResource(envelope.Resource.Ciphertext, envelope.Resource.Nonce, envelope.Resource.AssociatedData, cfg.WeChatPayAPIv3Key)
	if err != nil {
		return paymentCallbackRequest{}, paymentCallbackSignature{}, err
	}
	var payload struct {
		OutTradeNo    string `json:"out_trade_no"`
		TransactionID string `json:"transaction_id"`
		TradeState    string `json:"trade_state"`
		Amount        struct {
			Total int `json:"total"`
		} `json:"amount"`
	}
	if err := json.Unmarshal(plain, &payload); err != nil {
		return paymentCallbackRequest{}, paymentCallbackSignature{}, err
	}
	status := firstNonEmptyString(payload.TradeState, envelope.EventType)
	req := paymentCallbackRequest{
		OrderNo:       strings.TrimSpace(payload.OutTradeNo),
		Provider:      "wechat",
		Status:        status,
		PaymentStatus: status,
		EventType:     envelope.EventType,
		Paid:          isPaidStatus(status),
		TotalCents:    payload.Amount.Total,
		TransactionID: strings.TrimSpace(payload.TransactionID),
		ProviderTxnID: strings.TrimSpace(payload.TransactionID),
		EventID:       strings.TrimSpace(envelope.ID),
	}
	return req, paymentCallbackSignature{Provider: "wechat", Verified: true, Source: "wechatpay-v3"}, nil
}

func verifyWeChatPaySignature(header http.Header, rawBody []byte, cfg config.Config) error {
	timestamp := strings.TrimSpace(header.Get("Wechatpay-Timestamp"))
	nonce := strings.TrimSpace(header.Get("Wechatpay-Nonce"))
	signature := strings.TrimSpace(header.Get("Wechatpay-Signature"))
	if timestamp == "" || nonce == "" || signature == "" {
		return errors.New("wechat pay callback missing signature headers")
	}
	publicKey, err := rsaPublicKeyFromConfig(cfg.WeChatPayPlatformKey, cfg.WeChatPayPlatformPath)
	if err != nil {
		return fmt.Errorf("wechat pay platform public key: %w", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("wechat pay signature decode: %w", err)
	}
	message := timestamp + "\n" + nonce + "\n" + string(rawBody) + "\n"
	hash := sha256.Sum256([]byte(message))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hash[:], decoded); err != nil {
		return fmt.Errorf("wechat pay signature verification failed: %w", err)
	}
	return nil
}

func decryptWeChatPayResource(ciphertext string, nonce string, associatedData string, apiV3Key string) ([]byte, error) {
	apiV3Key = strings.TrimSpace(apiV3Key)
	if len(apiV3Key) != 32 {
		return nil, errors.New("wechat pay api v3 key must be 32 bytes")
	}
	rawCiphertext, err := base64.StdEncoding.DecodeString(strings.TrimSpace(ciphertext))
	if err != nil {
		return nil, fmt.Errorf("wechat pay resource ciphertext decode: %w", err)
	}
	block, err := aes.NewCipher([]byte(apiV3Key))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, []byte(nonce), rawCiphertext, []byte(associatedData))
}

func parseAlipayCallback(rawBody []byte, cfg config.Config) (paymentCallbackRequest, paymentCallbackSignature, error) {
	values, err := url.ParseQuery(string(rawBody))
	if err != nil {
		return paymentCallbackRequest{}, paymentCallbackSignature{}, err
	}
	if err := verifyAlipaySignature(values, cfg); err != nil {
		return paymentCallbackRequest{}, paymentCallbackSignature{}, err
	}
	amountCents, err := decimalAmountCents(values.Get("total_amount"))
	if err != nil {
		return paymentCallbackRequest{}, paymentCallbackSignature{}, err
	}
	status := strings.TrimSpace(values.Get("trade_status"))
	req := paymentCallbackRequest{
		OrderNo:       strings.TrimSpace(values.Get("out_trade_no")),
		Provider:      "alipay",
		Status:        status,
		PaymentStatus: status,
		EventType:     strings.TrimSpace(values.Get("notify_type")),
		Paid:          isPaidStatus(status) || status == "TRADE_SUCCESS" || status == "TRADE_FINISHED",
		TotalCents:    amountCents,
		TransactionID: strings.TrimSpace(values.Get("trade_no")),
		ProviderTxnID: strings.TrimSpace(values.Get("trade_no")),
		EventID:       strings.TrimSpace(values.Get("notify_id")),
	}
	return req, paymentCallbackSignature{Provider: "alipay", Verified: true, Source: "alipay-rsa2"}, nil
}

func verifyAlipaySignature(values url.Values, cfg config.Config) error {
	signature := strings.TrimSpace(values.Get("sign"))
	if signature == "" {
		return errors.New("alipay callback missing sign")
	}
	signType := strings.ToUpper(strings.TrimSpace(values.Get("sign_type")))
	if signType != "" && signType != "RSA2" {
		return fmt.Errorf("unsupported alipay sign_type: %s", signType)
	}
	publicKey, err := rsaPublicKeyFromConfig(cfg.AlipayPublicKey, cfg.AlipayPublicKeyPath)
	if err != nil {
		return fmt.Errorf("alipay public key: %w", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("alipay signature decode: %w", err)
	}
	content := alipaySignContent(values)
	hash := sha256.Sum256([]byte(content))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hash[:], decoded); err != nil {
		return fmt.Errorf("alipay signature verification failed: %w", err)
	}
	return nil
}

func alipaySignContent(values url.Values) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key == "sign" || key == "sign_type" {
			continue
		}
		if strings.TrimSpace(values.Get(key)) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values.Get(key))
	}
	return strings.Join(parts, "&")
}

func rsaPublicKeyFromConfig(inline string, filePath string) (*rsa.PublicKey, error) {
	material, err := keyMaterialFromConfig(inline, filePath)
	if err != nil {
		return nil, err
	}
	return parseRSAPublicKey(material)
}

func keyMaterialFromConfig(inline string, filePath string) (string, error) {
	inline = strings.TrimSpace(strings.ReplaceAll(inline, `\n`, "\n"))
	if inline != "" {
		return inline, nil
	}
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return "", errors.New("key material is not configured")
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func parseRSAPublicKey(material string) (*rsa.PublicKey, error) {
	material = strings.TrimSpace(strings.ReplaceAll(material, `\n`, "\n"))
	if block, _ := pem.Decode([]byte(material)); block != nil {
		switch block.Type {
		case "CERTIFICATE":
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, err
			}
			key, ok := cert.PublicKey.(*rsa.PublicKey)
			if !ok {
				return nil, errors.New("certificate public key is not RSA")
			}
			return key, nil
		case "PUBLIC KEY":
			key, err := x509.ParsePKIXPublicKey(block.Bytes)
			if err != nil {
				return nil, err
			}
			rsaKey, ok := key.(*rsa.PublicKey)
			if !ok {
				return nil, errors.New("public key is not RSA")
			}
			return rsaKey, nil
		case "RSA PUBLIC KEY":
			return x509.ParsePKCS1PublicKey(block.Bytes)
		default:
			return nil, fmt.Errorf("unsupported public key PEM type: %s", block.Type)
		}
	}
	der, err := base64.StdEncoding.DecodeString(material)
	if err != nil {
		return nil, errors.New("public key must be PEM or base64 DER")
	}
	key, err := x509.ParsePKIXPublicKey(der)
	if err == nil {
		if rsaKey, ok := key.(*rsa.PublicKey); ok {
			return rsaKey, nil
		}
		return nil, errors.New("public key is not RSA")
	}
	return x509.ParsePKCS1PublicKey(der)
}

func decimalAmountCents(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	negative := strings.HasPrefix(value, "-")
	if negative {
		value = strings.TrimPrefix(value, "-")
	}
	parts := strings.SplitN(value, ".", 3)
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid amount: %s", value)
	}
	yuan, err := strconv.Atoi(firstNonEmptyString(parts[0], "0"))
	if err != nil {
		return 0, err
	}
	cents := 0
	if len(parts) == 2 {
		fraction := parts[1]
		if len(fraction) > 2 {
			fraction = fraction[:2]
		}
		for len(fraction) < 2 {
			fraction += "0"
		}
		cents, err = strconv.Atoi(fraction)
		if err != nil {
			return 0, err
		}
	}
	total := yuan*100 + cents
	if negative {
		return -total, nil
	}
	return total, nil
}

func findPaymentCallbackOrder(orders []adminOrder, orderID string, orderNo string) (adminOrder, bool) {
	orderID = strings.TrimSpace(orderID)
	orderNo = strings.TrimSpace(orderNo)
	for _, order := range orders {
		idMatches := orderID == "" || order.ID == orderID || order.OrderNo == orderID
		noMatches := orderNo == "" || order.ID == orderNo || order.OrderNo == orderNo
		if idMatches && noMatches {
			return order, true
		}
	}
	return adminOrder{}, false
}

func paymentCallbackAmountCents(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func paymentCallbackMetadata(eventID string, providerTransactionID string, status string, amountCents int) map[string]any {
	metadata := map[string]any{
		"callbackReceivedAt": time.Now().UTC().Format(time.RFC3339Nano),
	}
	if eventID = strings.TrimSpace(eventID); eventID != "" {
		metadata["eventId"] = eventID
	}
	if providerTransactionID = strings.TrimSpace(providerTransactionID); providerTransactionID != "" {
		metadata["providerTransactionId"] = providerTransactionID
	}
	if status = strings.TrimSpace(status); status != "" {
		metadata["paymentCallbackStatus"] = status
	}
	if amountCents > 0 {
		metadata["paidAmountCents"] = amountCents
	}
	return metadata
}

func (a api) memberProfile(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	account, err := a.store.PointAccount(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	response := map[string]any{
		"user":            memberUserView(user),
		"account":         account,
		"plan":            commercePlanView(planMap(data.Plans)[user.PlanID]),
		"agent":           nil,
		"operationCenter": nil,
	}
	if agent, ok := channelAgentForUser(data.ChannelAgents, user.ID); ok {
		response["agent"] = channelAgentView(agent, user)
	}
	if center, ok := operationCenterForUser(data.OperationCenters, user.ID); ok {
		response["operationCenter"] = center
	}
	writeJSON(w, response)
}

func (a api) updateMemberProfile(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var req struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Name == "" || req.Email == "" {
		writeError(w, http.StatusBadRequest, errors.New("name and email are required"))
		return
	}
	if !regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`).MatchString(req.Email) {
		writeError(w, http.StatusBadRequest, errors.New("invalid email"))
		return
	}
	for _, item := range data.Users {
		if item.ID != user.ID && strings.EqualFold(item.Email, req.Email) {
			writeError(w, http.StatusConflict, errors.New("email already exists"))
			return
		}
	}
	account, err := a.store.PointAccount(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	updated, err := a.store.UpdateAdminCustomer(user.ID, adminCustomerMutation{
		Name:       req.Name,
		Email:      req.Email,
		Role:       user.Role,
		Status:     user.Status,
		PlanID:     user.PlanID,
		ReferredBy: user.ReferredBy,
		Available:  pointBalancePointer(account.Available),
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"user": memberUserView(updated)})
}

func (a api) memberWallet(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	account, err := a.store.PointAccount(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	data, err := a.userAccountData(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{
		"account":      account,
		"tokenRecords": userTokenRecords(data.TokenRecords, user.ID),
		"orders":       userMembershipOrders(data, user.ID),
		"transactions": userPointTransactions(data.BillingEvents, user.ID),
	})
}

func (a api) memberInvoices(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	data, err := a.userAccountData(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	plans := planMap(data.Plans)
	items := []map[string]any{}
	for _, order := range data.Orders {
		if order.UserID != user.ID && order.BuyerUserID != user.ID {
			continue
		}
		status := "PENDING_PAYMENT"
		invoiceNo := ""
		if isPaidStatus(order.Status) || strings.TrimSpace(order.PaidAt) != "" {
			status = "AVAILABLE"
			invoiceNo = "XZ-" + strings.ToUpper(shortID(order.ID))
		}
		items = append(items, map[string]any{
			"id":          "invoice_" + shortID(order.ID),
			"invoiceNo":   invoiceNo,
			"orderId":     order.ID,
			"planId":      order.PlanID,
			"planName":    planName(plans[order.PlanID]),
			"amountCents": orderAmount(order),
			"status":      status,
			"paymentStatus": map[bool]string{
				true:  "PAID",
				false: strings.ToUpper(strings.TrimSpace(order.Status)),
			}[isPaidStatus(order.Status)],
			"paidAt":    order.PaidAt,
			"createdAt": order.CreatedAt,
		})
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a api) createMemberRefundRequest(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var req struct {
		OrderID string `json:"orderId"`
		Reason  string `json:"reason"`
		Remark  string `json:"remark"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.OrderID = strings.TrimSpace(req.OrderID)
	req.Reason = strings.TrimSpace(req.Reason)
	req.Remark = strings.TrimSpace(req.Remark)
	if req.OrderID == "" || req.Reason == "" {
		writeError(w, http.StatusBadRequest, errors.New("orderId and reason are required"))
		return
	}
	if len([]rune(req.Remark)) > 200 {
		writeError(w, http.StatusBadRequest, errors.New("remark must not exceed 200 characters"))
		return
	}
	order, err := a.store.RequestOrderRefund(user.ID, req.OrderID, req.Reason, req.Remark)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": order, "message": "退款申请已提交，等待人工审核"})
}

func (a api) memberTokenRecords(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	data, err := a.userAccountData(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": userTokenRecords(data.TokenRecords, user.ID)})
}

func (a api) agentProfile(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	response := map[string]any{
		"user":     memberUserView(user),
		"agent":    nil,
		"joinPlan": commercePlanView(planMap(data.Plans)["plan_agent_join_996"]),
	}
	if agent, ok := channelAgentForUser(data.ChannelAgents, user.ID); ok {
		response["agent"] = channelAgentView(agent, user)
		response["summary"] = channelSummary(
			channelCustomers(data.Users, data.Plans, data.PointAccounts, channelVisibleCustomerIDs(data.Users, data.ChannelAgents, user.ID, agent.ID)),
			channelCommissions(data.Commissions, agent.ID),
			channelWithdrawals(data.Withdrawals, agent.ID),
			channelChildren(data.ChannelAgents, data.Users, agent.ID),
		)
	}
	writeJSON(w, response)
}

func (a api) operationCenterProfile(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	response := map[string]any{
		"user":            memberUserView(user),
		"operationCenter": nil,
		"joinPlan":        commercePlanView(planMap(data.Plans)["plan_operation_center_5000"]),
	}
	if center, ok := operationCenterForUser(data.OperationCenters, user.ID); ok {
		response["operationCenter"] = center
		response["summary"] = operationCenterSummary(data, center.ID)
	}
	writeJSON(w, response)
}

func (a api) operationCenterAgents(w http.ResponseWriter, r *http.Request) {
	data, _, center, ok := a.authenticatedOperationCenter(r)
	if !ok {
		writeError(w, http.StatusForbidden, errForbidden)
		return
	}
	users := userMap(data.Users)
	items := []map[string]any{}
	for _, agent := range data.ChannelAgents {
		if agent.OperationCenterID != center.ID {
			continue
		}
		items = append(items, channelAgentView(agent, users[agent.UserID]))
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a api) operationCenterOrders(w http.ResponseWriter, r *http.Request) {
	data, _, center, ok := a.authenticatedOperationCenter(r)
	if !ok {
		writeError(w, http.StatusForbidden, errForbidden)
		return
	}
	users := userMap(data.Users)
	plans := planMap(data.Plans)
	items := []map[string]any{}
	for _, order := range data.Orders {
		if orderOperationCenterID(order) != center.ID {
			continue
		}
		items = append(items, adminOrderView(order, users, plans))
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a api) operationCenterCommissions(w http.ResponseWriter, r *http.Request) {
	data, _, center, ok := a.authenticatedOperationCenter(r)
	if !ok {
		writeError(w, http.StatusForbidden, errForbidden)
		return
	}
	items := []adminCommission{}
	for _, commission := range data.Commissions {
		if commission.ReceiverType == receiverTypeOperationCenter && commission.ReceiverID == center.ID {
			items = append(items, commission)
		}
	}
	writeJSON(w, map[string]any{"summary": operationCenterCommissionSummary(items), "items": items})
}

func (a api) createRechargeOrder(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if !userHasCompletedPurchase(data.Orders, user.ID) {
		writeError(w, http.StatusConflict, errors.New("首次充值仅可选择 996 元代理商开通包"))
		return
	}
	var req struct {
		AmountCents       int    `json:"amountCents"`
		RechargePackageID string `json:"rechargePackageId"`
		PaymentMethod     string `json:"paymentMethod"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	planID := strings.TrimSpace(req.RechargePackageID)
	if planID == "" {
		planID = rechargePackageIDForAmount(req.AmountCents)
	}
	if plan, ok := planCatalogByID(planID); ok && strings.EqualFold(stringValue(plan.Entitlements["planType"]), "recharge") {
		req.AmountCents = planPrice(plan)
	} else {
		planID = fmt.Sprintf("recharge_%d", req.AmountCents/100)
	}
	if req.AmountCents == 0 {
		planID = "recharge_standard"
		if plan, ok := planCatalogByID(planID); ok {
			req.AmountCents = planPrice(plan)
		}
	}
	if req.AmountCents < 100 {
		writeError(w, http.StatusBadRequest, errors.New("amountCents must be at least 100"))
		return
	}
	order, err := a.store.CreateAdminOrder(adminOrderMutation{
		UserID:        user.ID,
		PlanID:        planID,
		AmountCents:   req.AmountCents,
		Status:        "PENDING",
		PaymentMethod: req.PaymentMethod,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{
		"item":           order,
		"rechargePoints": rechargePointsForOrder(order),
		"message":        "充值订单已创建，请等待主控确认收款",
	})
}

func userHasCompletedPurchase(orders []adminOrder, userID string) bool {
	for _, order := range orders {
		if order.UserID != userID && order.BuyerUserID != userID {
			continue
		}
		if strings.TrimSpace(order.PaidAt) != "" || strings.TrimSpace(order.FulfilledAt) != "" {
			return true
		}
		switch strings.ToUpper(strings.TrimSpace(order.Status)) {
		case "PAID", "SUCCESS", "SUCCEEDED", "COMPLETED", "SETTLED", "ACTIVE", "REFUNDED":
			return true
		}
	}
	return false
}

func (a api) createSubscriptionOrder(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var req struct {
		PlanID        string `json:"planId"`
		AmountCents   int    `json:"amountCents"`
		PaymentMethod string `json:"paymentMethod"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.PlanID = strings.TrimSpace(req.PlanID)
	if req.PlanID == "" || req.AmountCents <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("planId and amountCents are required"))
		return
	}
	if plan, ok := planCatalogByID(req.PlanID); ok && strings.EqualFold(stringValue(plan.Entitlements["planType"]), "subscription") && planPrice(plan) > 0 {
		req.AmountCents = planPrice(plan)
	}
	order, err := a.store.CreateAdminOrder(adminOrderMutation{
		UserID:        user.ID,
		PlanID:        req.PlanID,
		AmountCents:   req.AmountCents,
		Status:        "PENDING",
		PaymentMethod: req.PaymentMethod,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{
		"item":    order,
		"message": "订阅订单已创建，请等待主控确认收款",
	})
}

func userMembershipOrders(data adminPlatformData, userID string) []map[string]any {
	plans := planMap(data.Plans)
	items := []map[string]any{}
	for _, order := range data.Orders {
		if order.UserID != userID {
			continue
		}
		item := adminOrderView(order, map[string]adminUser{}, plans)
		item["rechargePoints"] = intValue(order.PriceSnapshot["rechargePoints"])
		items = append(items, item)
	}
	return items
}

func userPointTransactions(events []adminBillingEvent, userID string) []map[string]any {
	items := []map[string]any{}
	for _, event := range events {
		if event.UserID != userID {
			continue
		}
		items = append(items, map[string]any{
			"id":              event.ID,
			"transactionId":   event.TransactionID,
			"taskId":          event.TaskID,
			"metricCode":      event.MetricCode,
			"modelName":       usageDisplayNameForMetric(event.MetricCode),
			"title":           usageDisplayNameForMetric(event.MetricCode),
			"model":           event.Model,
			"type":            usageTypeForMetric(event.MetricCode),
			"quantity":        event.Quantity,
			"unitAmountCents": event.UnitAmountCents,
			"pointCost":       event.PointCost,
			"amountCents":     event.AmountCents,
			"balanceBefore":   event.BalanceBefore,
			"balanceAfter":    event.BalanceAfter,
			"status":          event.Status,
			"occurredAt":      event.OccurredAt,
			"createdAt":       event.OccurredAt,
		})
	}
	return items
}

func (a api) createOrderForPlan(user adminUser, plan adminPlan, requestedAmountCents int, paymentMethod string, idempotencyKey string) (adminOrder, error) {
	if user.ID == "" {
		return adminOrder{}, errors.New("user is required")
	}
	if plan.ID == "" || !commercePlanVisible(plan) {
		return adminOrder{}, errors.New("valid plan is required")
	}
	amountCents := planPrice(plan)
	if amountCents <= 0 {
		amountCents = requestedAmountCents
	} else if requestedAmountCents > 0 && requestedAmountCents != amountCents {
		return adminOrder{}, fmt.Errorf("amountCents must match plan price: %d", amountCents)
	}
	if amountCents < 0 {
		return adminOrder{}, errors.New("amountCents must be greater than or equal to 0")
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey != "" {
		data, err := a.store.AdminData()
		if err != nil {
			return adminOrder{}, err
		}
		for _, order := range data.Orders {
			if order.UserID == user.ID && order.PlanID == plan.ID && stringValue(order.PriceSnapshot["idempotencyKey"]) == idempotencyKey {
				return order, nil
			}
		}
	}
	return a.store.CreateAdminOrder(adminOrderMutation{
		UserID:             user.ID,
		PlanID:             plan.ID,
		AmountCents:        amountCents,
		Status:             "PENDING",
		PaymentMethod:      paymentMethod,
		IdempotencyKey:     idempotencyKey,
		PaymentEnvironment: map[int]string{0: "PRODUCTION", 1: "SANDBOX"}[virtualPaymentConfigFromApp(a.cfg).Env],
	})
}

func commercePlanByID(data adminPlatformData, id string) (adminPlan, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return adminPlan{}, false
	}
	for _, plan := range data.Plans {
		if plan.ID == id {
			return plan, true
		}
	}
	return planCatalogByID(id)
}

func commercePlanVisible(plan adminPlan) bool {
	return plan.ID != "" && planBusinessType(plan) != ""
}

func commercePlanView(plan adminPlan) map[string]any {
	if plan.ID == "" {
		return nil
	}
	entitlements := planEntitlements(plan)
	planType := planBusinessType(plan)
	sortValue := intValue(entitlements["sort"])
	if sortValue == 0 {
		sortValue = 1000
	}
	return map[string]any{
		"id":                    plan.ID,
		"code":                  fallback(plan.Code, plan.ID),
		"name":                  plan.Name,
		"planType":              planType,
		"priceCents":            planPrice(plan),
		"grantPoints":           planPoints(plan),
		"tokenGrantAmount":      planTokenGrantAmount(plan),
		"tokenRightsValueCents": planTokenRightsValueCents(plan),
		"memberLevel":           planMemberLevel(plan),
		"agentLevel":            plan.AgentLevel,
		"durationDays":          plan.DurationDays,
		"concurrency":           plan.Concurrency,
		"displayPrice":          stringValue(entitlements["displayPrice"]),
		"businessDescription":   stringValue(entitlements["businessDescription"]),
		"active":                plan.Active || plan.ID != "",
		"sort":                  sortValue,
		"entitlements":          entitlements,
	}
}

func commerceOrderResponse(order adminOrder, plan adminPlan) map[string]any {
	buyerUserID := firstNonEmptyString(order.BuyerUserID, order.UserID, stringValue(order.PriceSnapshot["buyerUserId"]))
	return map[string]any{
		"item": order,
		"plan": commercePlanView(plan),
		"checkout": map[string]any{
			"orderId":           order.ID,
			"orderNo":           firstNonEmptyString(order.OrderNo, order.ID),
			"buyerUserId":       buyerUserID,
			"businessOrderType": firstNonEmptyString(order.BusinessOrderType, businessOrderTypeForPlanType(planBusinessType(plan))),
			"amountCents":       orderAmount(order),
			"tokenAmount":       firstNonEmptyInt(order.TokenAmount, order.TokenGrantAmount, planTokenGrantAmount(plan)),
			"status":            order.Status,
		},
		"message": "order created",
	}
}

func memberUserView(user adminUser) map[string]any {
	view := userView(user)
	view["memberLevel"] = user.MemberLevel
	view["agentStatus"] = user.AgentStatus
	view["operationCenterStatus"] = user.OperationCenterStatus
	view["referredBy"] = user.ReferredBy
	view["subscriptionExpiresAt"] = user.SubscriptionExpiresAt
	return view
}

func userTokenRecords(records []adminTokenRecord, userID string) []adminTokenRecord {
	items := []adminTokenRecord{}
	for _, record := range records {
		if record.UserID == userID {
			items = append(items, record)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
	return items
}

func operationCenterForUser(centers []adminOperationCenter, userID string) (adminOperationCenter, bool) {
	for _, center := range centers {
		if center.UserID == userID {
			return center, true
		}
	}
	return adminOperationCenter{}, false
}

func (a api) authenticatedOperationCenter(r *http.Request) (adminPlatformData, adminUser, adminOperationCenter, bool) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		return adminPlatformData{}, adminUser{}, adminOperationCenter{}, false
	}
	center, ok := operationCenterForUser(data.OperationCenters, user.ID)
	if !ok || !strings.EqualFold(center.Status, "ACTIVE") {
		return data, user, adminOperationCenter{}, false
	}
	return data, user, center, true
}

func operationCenterSummary(data adminPlatformData, centerID string) map[string]any {
	agentCount := 0
	orderCount := 0
	paidOrderAmount := 0
	for _, agent := range data.ChannelAgents {
		if agent.OperationCenterID == centerID && strings.EqualFold(agent.Status, "ACTIVE") {
			agentCount++
		}
	}
	for _, order := range data.Orders {
		if orderOperationCenterID(order) == centerID && isPaidStatus(order.Status) {
			orderCount++
			paidOrderAmount += orderAmount(order)
		}
	}
	commissions := []adminCommission{}
	for _, item := range data.Commissions {
		if item.ReceiverType == receiverTypeOperationCenter && item.ReceiverID == centerID {
			commissions = append(commissions, item)
		}
	}
	summary := operationCenterCommissionSummary(commissions)
	summary["agents"] = agentCount
	summary["paidOrderAmountCents"] = paidOrderAmount
	summary["orders"] = orderCount
	return summary
}

func operationCenterCommissionSummary(items []adminCommission) map[string]any {
	total := 0
	settled := 0
	pending := 0
	for _, item := range items {
		total += item.AmountCents
		if strings.EqualFold(item.SettleStatus, "SETTLED") || strings.EqualFold(item.Status, "SETTLED") || strings.EqualFold(item.Status, "APPROVED") {
			settled += item.AmountCents
		} else {
			pending += item.AmountCents
		}
	}
	return map[string]any{
		"totalCents":   total,
		"settledCents": settled,
		"pendingCents": pending,
		"records":      len(items),
	}
}

func orderOperationCenterID(order adminOrder) string {
	if strings.TrimSpace(order.OperationCenterID) != "" {
		return strings.TrimSpace(order.OperationCenterID)
	}
	if order.PriceSnapshot != nil {
		return stringValue(order.PriceSnapshot["operationCenterId"])
	}
	return ""
}

func adminOrderView(order adminOrder, users map[string]adminUser, plans map[string]adminPlan) map[string]any {
	orderType := order.OrderType
	if orderType == "" && order.PriceSnapshot != nil {
		orderType = stringValue(order.PriceSnapshot["orderType"])
	}
	fulfillmentStatus := order.FulfillmentStatus
	if fulfillmentStatus == "" && order.PriceSnapshot != nil {
		fulfillmentStatus = stringValue(order.PriceSnapshot["fulfillmentStatus"])
	}
	directAgentID := firstNonEmptyString(order.DirectAgentID, stringValue(order.PriceSnapshot["directAgentId"]))
	parentAgentID := firstNonEmptyString(order.ParentAgentID, stringValue(order.PriceSnapshot["parentAgentId"]))
	operationCenterID := firstNonEmptyString(order.OperationCenterID, stringValue(order.PriceSnapshot["operationCenterId"]))
	tokenGrantAmount := order.TokenGrantAmount
	if tokenGrantAmount == 0 && order.PriceSnapshot != nil {
		tokenGrantAmount = intValue(order.PriceSnapshot["tokenGrantAmount"])
	}
	tokenAmount := firstNonEmptyInt(order.TokenAmount, tokenGrantAmount, intValue(order.PriceSnapshot["tokenAmount"]))
	platformIncomeCents := order.PlatformIncomeCents
	if platformIncomeCents == 0 && order.PriceSnapshot != nil {
		platformIncomeCents = intValue(order.PriceSnapshot["platformIncomeCents"])
	}
	rewardSnapshot := order.RewardSnapshot
	if rewardSnapshot == nil && order.PriceSnapshot != nil {
		rewardSnapshot, _ = mapValue(order.PriceSnapshot["rewardSnapshot"])
	}
	buyerUserID := firstNonEmptyString(order.BuyerUserID, order.UserID, stringValue(order.PriceSnapshot["buyerUserId"]))
	return map[string]any{
		"id":                  order.ID,
		"orderNo":             firstNonEmptyString(order.OrderNo, order.ID),
		"userId":              order.UserID,
		"buyerUserId":         buyerUserID,
		"customer":            users[order.UserID].Name,
		"planId":              order.PlanID,
		"plan":                planName(plans[order.PlanID]),
		"orderType":           orderType,
		"businessOrderType":   firstNonEmptyString(order.BusinessOrderType, stringValue(order.PriceSnapshot["businessOrderType"]), businessOrderTypeForPlanType(stringValue(order.PriceSnapshot["planType"]))),
		"paymentMethod":       stringValue(order.PriceSnapshot["paymentMethod"]),
		"amountCents":         orderAmount(order),
		"status":              order.Status,
		"directAgentId":       directAgentID,
		"parentAgentId":       parentAgentID,
		"operationCenterId":   operationCenterID,
		"tokenGrantAmount":    tokenGrantAmount,
		"tokenAmount":         tokenAmount,
		"platformIncomeCents": platformIncomeCents,
		"rewardSnapshot":      rewardSnapshot,
		"fulfillmentStatus":   fulfillmentStatus,
		"fulfilledAt":         firstNonEmptyString(order.FulfilledAt, stringValue(order.PriceSnapshot["fulfilledAt"])),
		"paidAt":              order.PaidAt,
		"createdAt":           order.CreatedAt,
	}
}

func (a api) userDashboard(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	// Homepage only renders ~30 recent items; avoid signing hundreds of asset URLs.
	taskLimit := listLimitFromRequest(r, "taskLimit", 30)
	assetLimit := listLimitFromRequest(r, "assetLimit", 30)
	var tasks []generationTask
	var assets []asset
	var points pointAccount
	var firstErr error
	var errMu sync.Mutex
	recordErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		defer errMu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		items, err := a.generationTasksForUser(r, user.ID, taskLimit)
		if err != nil {
			recordErr(err)
			return
		}
		tasks = items
	}()
	go func() {
		defer wg.Done()
		items, err := a.assetsForUserWorkspaceList(user.ID, assetLimit)
		if err != nil {
			recordErr(err)
			return
		}
		assets = items
	}()
	go func() {
		defer wg.Done()
		item, err := a.store.PointAccount(user.ID)
		if err != nil {
			recordErr(err)
			return
		}
		points = item
	}()
	wg.Wait()
	if firstErr != nil {
		writeError(w, http.StatusInternalServerError, firstErr)
		return
	}
	tasks = compactWorkspaceListTasks(attachAssetImagesToTasks(tasks, assets))
	succeeded := 0
	totalPointCost := 0
	for _, task := range tasks {
		if strings.EqualFold(task.Status, "SUCCEEDED") {
			succeeded++
		}
		totalPointCost += task.PointCost
	}
	writeJSON(w, map[string]any{
		"summary": map[string]any{
			"availablePoints":      points.Available,
			"totalPoints":          points.Total,
			"todayGenerations":     len(tasks),
			"succeededGenerations": succeeded,
			"assets":               len(assets),
			"totalPointCost":       totalPointCost,
		},
		"metrics": []map[string]any{
			{"label": "可用点数", "value": points.Available},
			{"label": "今日生成", "value": len(tasks)},
			{"label": "作品数量", "value": len(assets)},
			{"label": "点数消耗", "value": totalPointCost},
		},
		"recentTasks":  limitGenerationTasks(tasks, 30),
		"recentAssets": limitAssets(assets, 30),
	})
}

func (a api) userOnlineImage(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	// Workspace first paint only needs a short recent list; clients can raise the limit.
	taskLimit := listLimitFromRequest(r, "taskLimit", 40)
	assetLimit := listLimitFromRequest(r, "assetLimit", 40)
	var tasks []generationTask
	var assets []asset
	var points pointAccount
	var settings adminPlatformData
	var aiState userAIState
	var firstErr error
	var errMu sync.Mutex
	recordErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		defer errMu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}
	var wg sync.WaitGroup
	wg.Add(5)
	go func() {
		defer wg.Done()
		items, err := a.generationTasksForUser(r, user.ID, taskLimit)
		if err != nil {
			recordErr(err)
			return
		}
		tasks = items
	}()
	go func() {
		defer wg.Done()
		items, err := a.assetsForUserWorkspaceList(user.ID, assetLimit)
		if err != nil {
			recordErr(err)
			return
		}
		assets = items
	}()
	go func() {
		defer wg.Done()
		item, err := a.store.PointAccount(user.ID)
		if err != nil {
			recordErr(err)
			return
		}
		points = item
	}()
	go func() {
		defer wg.Done()
		var item adminPlatformData
		var err error
		if store, ok := a.store.(onlineImageSettingsStore); ok {
			item, err = store.OnlineImageSettings()
		} else {
			item, err = a.store.AdminData()
		}
		if err != nil {
			recordErr(err)
			return
		}
		settings = item
	}()
	go func() {
		defer wg.Done()
		item, err := a.store.UserAIState(user.ID)
		if err != nil {
			recordErr(err)
			return
		}
		aiState = item
	}()
	wg.Wait()
	if firstErr != nil {
		writeError(w, http.StatusInternalServerError, firstErr)
		return
	}
	tasks = compactWorkspaceListTasks(attachAssetImagesToTasks(tasks, assets))
	queued := 0
	running := 0
	completed := 0
	failed := 0
	totalPointCost := 0
	for _, task := range tasks {
		status := strings.ToUpper(task.Status)
		switch status {
		case "PENDING", "QUEUED":
			queued++
		case "RUNNING", "PROCESSING":
			running++
		case "SUCCEEDED", "COMPLETED":
			completed++
		case "FAILED", "ERROR":
			failed++
		}
		totalPointCost += task.PointCost
	}
	generationChannels := configuredGenerationChannels(settings)
	providers := make([]map[string]any, 0, len(generationChannels))
	for index, channel := range generationChannels {
		providers = append(providers, map[string]any{
			"id":               channel.ID,
			"name":             channel.Name,
			"baseUrl":          channel.BaseURL,
			"status":           channel.Status,
			"primary":          channel.Primary,
			"priority":         channel.Priority,
			"models":           channel.Models,
			"apiKeyConfigured": channel.APIKeyConfigured,
			"latencyMs":        120 + index*35,
			"quota":            100000 - index*12000,
		})
	}
	writeJSON(w, map[string]any{
		"summary": map[string]any{
			"availablePoints":  points.Available,
			"totalPoints":      points.Total,
			"todayGenerations": len(tasks),
			"queueTasks":       queued + running,
			"apiPlatforms":     len(providers),
			"totalPointCost":   totalPointCost,
		},
		"metrics": []map[string]any{
			{"label": "可用点数", "value": points.Available},
			{"label": "今日生成", "value": len(tasks)},
			{"label": "队列任务", "value": queued + running},
			{"label": "可用 API 平台", "value": len(providers)},
		},
		"queue":       map[string]any{"queued": queued, "running": running, "completed": completed, "failed": failed},
		"providers":   providers,
		"models":      settings.APIModels,
		"recentTasks": tasks,
		"assets":      assets,
		"aiState":     aiState,
	})
}

func (a api) updateUserAIState(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var req userAIState
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	state, err := a.store.UpdateUserAIState(user.ID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, state)
}

func (a api) userAPISettings(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var data adminPlatformData
	if store, ok := a.store.(onlineImageSettingsStore); ok {
		data, err = store.OnlineImageSettings()
	} else {
		data, err = a.store.AdminData()
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	models := userVisibleAPIModels(data, user)
	capabilities := apiModelCapabilities(models)
	defaultModel := ""
	if len(models) > 0 {
		defaultModel = models[0].Model
	}
	account, accountErr := a.store.PointAccount(user.ID)
	quota := map[string]any{}
	if accountErr == nil {
		quota = map[string]any{
			"available": account.Available,
			"frozen":    account.Frozen,
			"total":     account.Total,
		}
	}
	writeJSON(w, map[string]any{
		"summary": map[string]any{
			"models":       len(models),
			"capabilities": len(capabilities),
			"apiKeyCount":  len(data.APIKeys),
		},
		"models":       models,
		"apiModels":    models,
		"capabilities": capabilities,
		"defaultModel": defaultModel,
		"userGroup":    billingCustomerGroupName(data, user),
		"quota":        quota,
	})
}

func userVisibleAPIModels(data adminPlatformData, user adminUser) []adminAPIModel {
	groupName := billingCustomerGroupName(data, user)
	groupModels := map[string]bool{}
	for _, group := range data.CustomerGroups {
		if strings.EqualFold(strings.TrimSpace(group.Name), strings.TrimSpace(groupName)) || strings.EqualFold(strings.TrimSpace(group.ID), strings.TrimSpace(groupName)) {
			for _, model := range group.Models {
				model = strings.ToLower(strings.TrimSpace(model))
				if model != "" {
					groupModels[model] = true
				}
			}
			break
		}
	}
	items := make([]adminAPIModel, 0, len(data.APIModels))
	for _, item := range data.APIModels {
		if !strings.EqualFold(strings.TrimSpace(item.Status), "ACTIVE") {
			continue
		}
		if len(groupModels) > 0 && !apiModelAllowedForGroup(item, groupModels) {
			continue
		}
		items = append(items, item)
	}
	return items
}

func apiModelAllowedForGroup(item adminAPIModel, groupModels map[string]bool) bool {
	for _, value := range []string{item.ID, item.Model, item.Name} {
		if groupModels[strings.ToLower(strings.TrimSpace(value))] {
			return true
		}
	}
	return false
}

func apiModelCapabilities(models []adminAPIModel) []string {
	seen := map[string]bool{}
	items := []string{}
	for _, model := range models {
		capability := strings.TrimSpace(model.Capability)
		if capability == "" || seen[capability] {
			continue
		}
		seen[capability] = true
		items = append(items, capability)
	}
	sort.Strings(items)
	return items
}

func (a api) userUsage(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	data, err := a.userAccountData(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items := make([]map[string]any, 0, len(data.BillingEvents))
	totalPointCost := 0
	totalAmountCents := 0
	succeeded := 0
	for _, event := range data.BillingEvents {
		if event.UserID != user.ID || event.PointCost <= 0 || !isUsageBillingMetric(event.MetricCode) {
			continue
		}
		totalPointCost += event.PointCost
		totalAmountCents += event.AmountCents
		if strings.EqualFold(event.Status, "SUCCEEDED") {
			succeeded++
		}
		items = append(items, map[string]any{
			"id":              event.ID,
			"transactionId":   event.TransactionID,
			"taskId":          event.TaskID,
			"metricCode":      event.MetricCode,
			"modelName":       usageDisplayNameForMetric(event.MetricCode),
			"title":           usageDisplayNameForMetric(event.MetricCode),
			"model":           event.Model,
			"type":            usageTypeForMetric(event.MetricCode),
			"quantity":        event.Quantity,
			"unitAmountCents": event.UnitAmountCents,
			"amountCents":     event.AmountCents,
			"pointCost":       event.PointCost,
			"balanceBefore":   event.BalanceBefore,
			"balanceAfter":    event.BalanceAfter,
			"status":          event.Status,
			"occurredAt":      event.OccurredAt,
			"createdAt":       event.OccurredAt,
		})
	}
	writeJSON(w, map[string]any{
		"summary": map[string]any{"records": len(items), "totalPointCost": totalPointCost, "totalAmountCents": totalAmountCents, "succeeded": succeeded},
		"items":   items,
	})
}

func (a api) deleteAsset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	assets, err := a.assetsForUser(r, user.ID, maxUserContentListLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	found := false
	for _, item := range assets {
		if item.ID == id && item.UserID == user.ID {
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, errAssetNotFound)
		return
	}
	if err := a.store.DeleteAssetForUser(user.ID, id); err != nil {
		if errors.Is(err, errAssetNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func filterGenerationTasksForUser(tasks []generationTask, userID string) []generationTask {
	items := make([]generationTask, 0, len(tasks))
	for _, task := range tasks {
		if task.UserID == userID {
			items = append(items, task)
		}
	}
	return items
}

func filterAssetsForUser(assets []asset, userID string) []asset {
	items := make([]asset, 0, len(assets))
	for _, item := range assets {
		if item.UserID == userID && !assetDeleted(item) {
			items = append(items, item)
		}
	}
	return items
}

func limitGenerationTasks(tasks []generationTask, limit int) []generationTask {
	if limit <= 0 || len(tasks) <= limit {
		return tasks
	}
	return tasks[:limit]
}

func limitAssets(assets []asset, limit int) []asset {
	if limit <= 0 || len(assets) <= limit {
		return assets
	}
	return assets[:limit]
}

func sortGenerationTasksForUserList(tasks []generationTask, prioritize bool) {
	sort.SliceStable(tasks, func(i, j int) bool {
		if prioritize {
			leftPriority := generationTaskListPriority(tasks[i].Status)
			rightPriority := generationTaskListPriority(tasks[j].Status)
			if leftPriority != rightPriority {
				return leftPriority < rightPriority
			}
		}
		if tasks[i].CreatedAt != tasks[j].CreatedAt {
			return tasks[i].CreatedAt > tasks[j].CreatedAt
		}
		return tasks[i].ID > tasks[j].ID
	})
}

func generationTaskListPriority(status string) int {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "PENDING", "QUEUED", "RUNNING", "PROCESSING", "RETRYING", "FAILED", "ERROR":
		return 0
	default:
		return 1
	}
}

func sortAssetsForUserList(assets []asset) {
	sort.SliceStable(assets, func(i, j int) bool {
		if assets[i].CreatedAt != assets[j].CreatedAt {
			return assets[i].CreatedAt > assets[j].CreatedAt
		}
		return assets[i].ID > assets[j].ID
	})
}

func pageGenerationTasks(tasks []generationTask, limit int, offset int) []generationTask {
	if offset >= len(tasks) {
		return []generationTask{}
	}
	end := len(tasks)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return tasks[offset:end]
}

func pageAssets(assets []asset, limit int, offset int) []asset {
	if offset >= len(assets) {
		return []asset{}
	}
	end := len(assets)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return assets[offset:end]
}

func attachAssetImagesToTasks(tasks []generationTask, assets []asset) []generationTask {
	if len(tasks) == 0 || len(assets) == 0 {
		return tasks
	}
	assetByID := make(map[string]asset, len(assets))
	assetByTaskID := make(map[string]asset, len(assets))
	for _, item := range assets {
		item = secureAssetForClient(item)
		if item.ID != "" {
			assetByID[item.ID] = item
		}
		if item.TaskID != "" {
			if _, exists := assetByTaskID[item.TaskID]; !exists {
				assetByTaskID[item.TaskID] = item
			}
		}
	}
	items := make([]generationTask, 0, len(tasks))
	for _, task := range tasks {
		if item, ok := firstAssetForTask(task, assetByID, assetByTaskID); ok {
			if task.ThumbnailURL == "" {
				task.ThumbnailURL = item.ThumbnailURL
			}
			if task.ImageURL == "" {
				task.ImageURL = item.URL
			}
			if task.OutputURL == "" {
				task.OutputURL = item.URL
			}
			if task.ResultURL == "" {
				task.ResultURL = item.URL
			}
		}
		task.ImageURL = securePublicMediaURL(task.ImageURL)
		task.OutputURL = securePublicMediaURL(task.OutputURL)
		task.ResultURL = securePublicMediaURL(task.ResultURL)
		task.ThumbnailURL = securePublicMediaURL(task.ThumbnailURL)
		items = append(items, task)
	}
	return items
}

func secureAssetsForClient(items []asset) []asset {
	secured := make([]asset, len(items))
	for i, item := range items {
		secured[i] = secureAssetForClient(item)
	}
	return secured
}

func secureAssetForClient(item asset) asset {
	item.URL = securePublicMediaURL(item.URL)
	item.ThumbnailURL = securePublicMediaURL(item.ThumbnailURL)
	if item.Metadata != nil {
		metadata := make(map[string]any, len(item.Metadata))
		for key, value := range item.Metadata {
			if key == "thumbnailUrl" {
				continue
			}
			metadata[key] = value
		}
		item.Metadata = metadata
	}
	return item
}

func securePublicMediaURL(rawURL string) string {
	value := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, "http") || parsed.Hostname() == "" {
		return rawURL
	}
	host := strings.ToLower(strings.Trim(parsed.Hostname(), "[]"))
	// Keep internal docker / loopback endpoints untouched; upgrading them to https
	// makes browser detail previews fail harder (e.g. https://minio:9000/...).
	if host == "localhost" || host == "minio" || !strings.Contains(host, ".") {
		return rawURL
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
		return rawURL
	}
	parsed.Scheme = "https"
	return parsed.String()
}

func firstAssetForTask(task generationTask, assetByID map[string]asset, assetByTaskID map[string]asset) (asset, bool) {
	for _, id := range task.ResultIDs {
		if item, ok := assetByID[id]; ok {
			return item, true
		}
	}
	if item, ok := assetByTaskID[task.ID]; ok {
		return item, true
	}
	return asset{}, false
}
