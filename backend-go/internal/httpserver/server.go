package httpserver

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	operationcenter "xianzhi-ai/backend-go/internal/app/operationcenter"
	paymentapp "xianzhi-ai/backend-go/internal/app/payment"
	"xianzhi-ai/backend-go/internal/app/smartvideo"
	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/infra"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

func New(cfg config.Config) *http.Server {
	return newWithStoreAndSessions(cfg, newJSONStore(cfg.DataPath), defaultAuthSessions(cfg, nil))
}

func NewWithDatabase(cfg config.Config, db *sql.DB) *http.Server {
	return NewWithInfrastructure(cfg, db, nil)
}

func NewWithInfrastructure(cfg config.Config, db *sql.DB, redisClient *redis.Client) *http.Server {
	store := platformStore(newJSONStore(cfg.DataPath))
	knowledge := newMemoryKnowledgeModule(cfg)
	mediaRepo := mediaRepository(newMemoryMediaRepository())
	if db != nil {
		store = newPostgresPrimaryStore(db, cfg.DataPath)
		knowledge = newPostgresKnowledgeModule(cfg, db)
		mediaRepo = newPostgresMediaRepository(db)
	}
	return newWithStoreSessionsKnowledgeAndMedia(cfg, store, defaultAuthSessions(cfg, redisClient), knowledge, mediaRepo, redisClient)
}

func newWithStore(cfg config.Config, store platformStore) *http.Server {
	return newWithStoreAndSessions(cfg, store, defaultAuthSessions(cfg, nil))
}

func defaultAuthSessions(cfg config.Config, redisClient *redis.Client) authSessionStore {
	if sessions := newRedisAuthSessions(redisClient); sessions != nil {
		return sessions
	}
	if cfg.IsProduction() {
		return nil
	}
	return newLocalAuthSessions()
}

func newWithStoreAndSessions(cfg config.Config, store platformStore, sessions authSessionStore) *http.Server {
	return newWithStoreSessionsAndKnowledge(cfg, store, sessions, newMemoryKnowledgeModule(cfg))
}

func newWithStoreSessionsAndKnowledge(cfg config.Config, store platformStore, sessions authSessionStore, knowledge *knowledgeModule) *http.Server {
	return newWithStoreSessionsKnowledgeAndMedia(cfg, store, sessions, knowledge, newMemoryMediaRepository(), nil)
}

func newWithStoreSessionsKnowledgeAndMedia(cfg config.Config, store platformStore, sessions authSessionStore, knowledge *knowledgeModule, mediaRepo mediaRepository, redisClient *redis.Client) *http.Server {
	if knowledge != nil && knowledge.rag != nil {
		knowledge.rag.SetBillingRecorder(store)
	}
	admin := newAdminAPI(store, sessions)
	businessPlans := newBusinessPlanAdminAPI(store)
	pricePlans := newPricePlanAdminAPI(store)
	pricePlanTestWhitelist := newPricePlanTestWhitelistAdminAPI(store)
	pricingAudit := newPricingAuditAdminAPI(store)
	pricingHealth := newPricingHealthAdminAPI(store, cfg)
	wechatGoods := newWechatVirtualGoodsAdminAPI(store)
	operationCenterReviews := newOperationCenterReviewAPI(nil)
	operationCenterRefunds := newOperationCenterRefundAPI(nil, nil)
	var operationCenterRefundManagement *operationcenter.RefundManagementService
	var operationCenterRuntime *operationcenter.OperationCenterRuntime
	if pgStore, ok := store.(*postgresStore); ok {
		workflow, workflowErr := operationcenter.NewWorkflowService(pgStore.db, operationcenter.WorkflowOptions{})
		if workflowErr == nil {
			operationCenterReviews = newOperationCenterReviewAPI(workflow)
		}
		management, managementErr := operationcenter.NewRefundManagementService(pgStore.db)
		if managementErr == nil {
			operationCenterRefundManagement = management
			operationCenterRefunds = newOperationCenterRefundAPI(management, nil)
		}
	}
	identityQueries := newIdentityQueryAPI(store)
	identityChanges := newIdentityChangeAPI(store)
	identityDowngrades := newIdentityDowngradeAPI(store)
	adminEnterprise := newAdminEnterpriseAPI(store)
	auth := newAuthAPI(store, sessions, cfg)
	agentInvites := newAgentInviteAPI(store, sessions, cfg)
	userRBAC := newUserRBACAPI(store, sessions)
	promotion := newPromotionAPI(store, sessions, userRBAC, cfg)
	enterprise := newEnterpriseAPI(store, sessions)
	channel := newChannelAPI(store, sessions)
	knowledgeAPI := newKnowledgeAPI(knowledge, sessions, store)
	mediaStorage, storageErr := newMediaStorage(cfg)
	if storageErr != nil {
		mediaStorage = unavailableMediaStorage{err: storageErr}
	}
	media := newMediaAPI(cfg, mediaRepo, mediaStorage, store, sessions, redisClient)
	var inspirationRepo inspirationRepository = newMemoryInspirationRepository()
	if pgStore, ok := store.(*postgresStore); ok {
		inspirationRepo = postgresInspirationRepository{db: pgStore.db}
	}
	inspirations := newInspirationAPI(inspirationRepo, store, sessions)
	var fileRepository storagecenter.Repository = storagecenter.NewMemoryRepository()
	if pgStore, ok := store.(*postgresStore); ok {
		fileRepository = storagecenter.NewPostgresRepository(pgStore.db)
	}
	fileService := storagecenter.NewService(fileRepository, storagecenter.S3ProviderFactory{AutoCreateBucket: cfg.StorageAutoCreateBucket}, fileCenterOptions(cfg))
	api := newAPI(store, cfg, sessions, fileService)
	publicCatalog := publicCatalogAPI{store: store}
	api.pptVisualLocker = newRedisPPTVisualLocker(redisClient)
	virtualPayment := newVirtualPaymentAPI(cfg, store, sessions, redisClient)
	if pgStore, ok := store.(*postgresStore); ok && operationCenterRefundManagement != nil {
		var runtimeErr error
		operationCenterRuntime, runtimeErr = newOperationCenterRuntime(pgStore.db, cfg.Environment, virtualPayment.service)
		if runtimeErr == nil {
			operationCenterRefunds = newOperationCenterRefundAPI(operationCenterRefundManagement, operationCenterRuntime)
		} else {
			slog.Error("operation center runtime unavailable", "environment", cfg.Environment, "error", runtimeErr)
			if operationCenterProductionEnvironment(cfg.Environment) {
				panic(runtimeErr)
			}
		}
	}
	paymentCenter := newPaymentCenterAPI(cfg, store, sessions, virtualPayment)
	connectors := newConnectorAPI(cfg, store, enterprise, api, redisClient)
	files := newFileCenterAPI(fileService, store, sessions)
	var smartVideoRepository smartvideo.Repository = smartvideo.NewMemoryRepository()
	var smartVideoPlanService *smartvideo.PlanService
	var smartVideoExportService *smartvideo.ExportService
	if pgStore, ok := store.(*postgresStore); ok {
		pgRepo := smartvideo.NewPostgresRepository(pgStore.db)
		smartVideoRepository = pgRepo
		smartVideoPlanService = smartvideo.NewPlanService(pgRepo, pgRepo, pgRepo, nil)
		smartVideoExportService = smartvideo.NewExportService(pgRepo, pgRepo, pgRepo, newSmartVideoPointsLifecycle(store))
	} else if mem, ok := smartVideoRepository.(*smartvideo.MemoryRepository); ok {
		smartVideoPlanService = smartvideo.NewPlanService(mem, nil, mem, nil)
		smartVideoExportService = smartvideo.NewExportService(mem, mem, mem, newSmartVideoPointsLifecycle(store))
	}
	var smartVideoAnalysisQueue smartvideo.AnalysisQueue
	if redisClient != nil {
		smartVideoAnalysisQueue = infra.NewSmartVideoAnalysisQueue(redisClient, "")
	}
	var smartVideoRenderQueue smartvideo.RenderQueue
	if redisClient != nil {
		smartVideoRenderQueue = infra.NewSmartVideoRenderQueue(redisClient)
	}
	smartVideos := newSmartVideoAPI(
		smartvideo.NewService(smartVideoRepository, smartVideoFileResolver{service: fileService}).SetRenderQueue(smartVideoRenderQueue),
		smartvideo.NewAnalysisService(smartVideoRepository, smartVideoAnalysisQueue, smartVideoAnalysisOptions(cfg)),
		smartVideoPlanService,
		smartVideoExportService,
		files,
	)
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestContextMiddleware())
	router.Use(corsMiddleware(cfg.CORSAllowedOrigins))
	router.Use(gzipMiddleware())
	if pgStore, ok := store.(*postgresStore); ok {
		router.Use(pgStore.auditMiddleware())
	}

	router.GET("/healthz", wrapF(health))
	router.GET("/d/:inviteCode", wrapF(agentInviteH5Redirect))
	router.GET("/i/:inviteToken", wrapF(promotionInviteTokenH5Redirect))
	router.GET("/android/latest", wrapF(agentInvites.download))
	router.POST("/api/open/connectors/feishu/events/:connectorKey", wrapF(connectors.event))
	router.GET("/api/open/connectors/authorize/:ticket", wrapF(connectors.authorizationLanding))
	router.GET("/api/open/connectors/authorize/:ticket/start", wrapF(connectors.startAuthorization))
	router.GET("/api/open/connectors/oauth/feishu/callback", wrapF(connectors.feishuOAuthCallback))
	router.GET("/api/open/connectors/oauth/wechat/callback", wrapF(connectors.wechatOAuthCallback))
	v1 := router.Group("/api/v1")
	v1.GET("/health", wrapF(health))
	v1.POST("/auth/login", wrapF(auth.login))
	v1.POST("/auth/wechat-mini-program/login", wrapF(auth.wechatMiniProgramLogin))
	v1.POST("/auth/wechat-mini-program/link", wrapF(auth.linkWeChatMiniProgram))
	v1.POST("/auth/wechat-mini-program/session", wrapF(auth.refreshWeChatMiniProgramSession))
	v1.POST("/auth/wechat/phone-login", wrapF(auth.wechatMiniProgramLogin))
	v1.GET("/auth/wechat/qrcode", wrapF(auth.wechatWebQRCode))
	v1.GET("/auth/wechat/status", wrapF(auth.wechatWebStatus))
	v1.GET("/auth/wechat/callback", wrapF(auth.wechatWebCallback))
	v1.POST("/auth/wechat/bind-mobile", wrapF(auth.wechatWebBindMobile))
	v1.POST("/auth/sms/send", wrapF(auth.smsSend))
	v1.POST("/auth/sms/login", wrapF(auth.smsLogin))
	v1.POST("/auth/mobile/bind", wrapF(auth.bindMobile))
	v1.POST("/auth/register", wrapF(auth.register))
	v1.GET("/auth/me", wrapF(auth.me))
	v1.POST("/auth/refresh", wrapF(auth.refresh))
	v1.POST("/auth/logout", wrapF(auth.logout))
	v1.POST("/auth/logout-all", wrapF(auth.logoutAll))
	v1.POST("/auth/change-password", wrapF(auth.changePassword))
	v1.GET("/auth/security", wrapF(auth.security))
	v1.GET("/invite/agent/resolve", wrapF(auth.resolveInvite))
	v1.GET("/invite/resolve", wrapF(auth.resolvePromotionInvite))
	v1.GET("/public/invites/:inviteCode", wrapF(agentInvites.invite))
	v1.POST("/public/invites/:inviteCode/register", wrapF(agentInvites.register))
	v1.GET("/public/app/releases/latest", wrapF(agentInvites.latestRelease))
	v1.GET("/public/app/releases/android/latest/download", wrapF(agentInvites.download))
	v1.GET("/agent/invite/profile", wrapF(agentInvites.profile))
	v1.POST("/agent/invite/poster", wrapF(agentInvites.poster))
	v1.POST("/app/activation", wrapF(agentInvites.activation))
	v1.GET("/user/profile", wrapF(userRBAC.profile))
	v1.POST("/user/current-role", wrapF(userRBAC.switchCurrentRole))
	v1.GET("/promotion/overview", wrapF(promotion.overview))
	v1.GET("/promotion/profile", wrapF(promotion.profile))
	v1.GET("/promotion/poster-templates", wrapF(promotion.templates))
	v1.POST("/promotion/miniprogram-code", wrapF(promotion.miniProgramCode))
	v1.POST("/promotion/qrcode", wrapF(promotion.miniProgramCode))
	v1.POST("/promotion/poster/render", wrapF(promotion.renderConfig))
	v1.GET("/promotion/records", wrapF(promotion.records))
	v1.GET("/promotion/analytics", wrapF(promotion.analytics))
	v1.GET("/promotion/stats", wrapF(promotion.analytics))
	v1.GET("/promotion/activities", wrapF(promotion.activities))
	v1.GET("/promotion/share-copy", wrapF(promotion.shareCopy))
	v1.POST("/promotion/visit", wrapF(promotion.visit))
	v1.POST("/promotion/bind", wrapF(promotion.bind))
	v1.GET("/user/enterprise-contexts", wrapF(enterprise.contexts))
	v1.POST("/user/current-context", wrapF(enterprise.switchContext))
	v1.POST("/enterprises", wrapF(enterprise.createEnterprise))
	v1.GET("/enterprise/overview", wrapF(enterprise.overview))
	v1.GET("/enterprise/members", wrapF(enterprise.members))
	v1.GET("/enterprise/members/:id", wrapF(enterprise.member))
	v1.PATCH("/enterprise/members/:id", wrapF(enterprise.updateMember))
	v1.POST("/enterprise/members/:id/disable", wrapF(enterprise.disableMember))
	v1.DELETE("/enterprise/members/:id", wrapF(enterprise.removeMember))
	v1.POST("/enterprise/invitations", wrapF(enterprise.createInvitation))
	v1.POST("/enterprise/invitations/accept", wrapF(enterprise.acceptInvitation))
	v1.GET("/enterprise/join-requests", wrapF(enterprise.joinRequests))
	v1.POST("/enterprise/join-requests", wrapF(enterprise.createJoinRequest))
	v1.POST("/enterprise/join-requests/:id/approve", wrapF(enterprise.approveJoinRequest))
	v1.POST("/enterprise/join-requests/:id/reject", wrapF(enterprise.rejectJoinRequest))
	v1.GET("/enterprise/organizations/tree", wrapF(enterprise.organizationTree))
	v1.POST("/enterprise/organizations", wrapF(enterprise.createOrganization))
	v1.PATCH("/enterprise/organizations/:id", wrapF(enterprise.updateOrganization))
	v1.POST("/enterprise/organizations/:id/move", wrapF(enterprise.moveOrganization))
	v1.DELETE("/enterprise/organizations/:id", wrapF(enterprise.deleteOrganization))
	v1.GET("/enterprise/roles", wrapF(enterprise.roles))
	v1.GET("/enterprise/billing/summary", wrapF(enterprise.billingSummary))
	v1.GET("/enterprise/compute-account", wrapF(enterprise.computeAccount))
	v1.POST("/enterprise/certifications", wrapF(enterprise.submitCertification))
	v1.GET("/enterprise/audit-logs", wrapF(enterprise.auditLogs))
	v1.GET("/enterprise/connectors", wrapF(connectors.list))
	v1.GET("/enterprise/connector-authorizations/platforms", wrapF(connectors.authorizationPlatforms))
	v1.POST("/enterprise/connector-authorizations", wrapF(connectors.createAuthorizationSession))
	v1.GET("/enterprise/connector-authorizations/:id", wrapF(connectors.getAuthorizationSession))
	v1.POST("/enterprise/connector-authorizations/:id/cancel", wrapF(connectors.cancelAuthorizationSession))
	v1.GET("/enterprise/connectors/feishu", wrapF(connectors.getFeishu))
	v1.POST("/enterprise/connectors/feishu", wrapF(connectors.createFeishu))
	v1.PUT("/enterprise/connectors/feishu", wrapF(connectors.updateFeishu))
	v1.POST("/enterprise/connectors/feishu/test", wrapF(connectors.testFeishu))
	v1.POST("/enterprise/connectors/feishu/enable", wrapF(connectors.enableFeishu))
	v1.POST("/enterprise/connectors/feishu/disable", wrapF(connectors.disableFeishu))
	v1.GET("/enterprise/connectors/feishu/users", wrapF(connectors.users))
	v1.PUT("/enterprise/connectors/feishu/users/:id", wrapF(connectors.updateUser))
	v1.GET("/enterprise/connectors/feishu/logs", wrapF(connectors.logs))
	v1.GET("/enterprise/connectors/feishu/tasks", wrapF(connectors.tasks))
	v1.POST("/enterprise/connectors/feishu/tasks/:taskId/retry-delivery", wrapF(connectors.retryDelivery))
	v1.GET("/channel/me", wrapF(channel.me))
	v1.GET("/channel/customers", wrapF(channel.customers))
	v1.GET("/channel/customers/:id", wrapF(channel.customerDetail))
	v1.GET("/channel/orders", wrapF(channel.orders))
	v1.GET("/channel/orders/:id", wrapF(channel.orderDetail))
	v1.GET("/channel/usage", wrapF(channel.usage))
	v1.GET("/channel/commissions", wrapF(channel.commissions))
	v1.GET("/channel/commissions/:id", wrapF(channel.commissionDetail))
	v1.GET("/channel/withdrawals", wrapF(channel.withdrawals))
	v1.GET("/channel/withdrawals/:id", wrapF(channel.withdrawalDetail))
	v1.POST("/channel/withdrawals", wrapF(channel.createWithdrawal))
	v1.GET("/channel/children/:id", wrapF(channel.childDetail))
	v1.POST("/channel/children", wrapF(channel.createChildAgent))
	v1.GET("/channel/invite-records", wrapF(channel.inviteRecords))
	v1.GET("/models", wrapF(api.models))
	v1.GET("/public/home", wrapF(publicCatalog.home))
	v1.GET("/public/cases", wrapF(publicCatalog.cases))
	v1.GET("/public/templates", wrapF(publicCatalog.templates))
	v1.GET("/public/agents", wrapF(publicCatalog.agents))
	v1.GET("/public/models", wrapF(api.models))
	v1.GET("/public/module-schema", wrapF(api.publicModuleSchema))
	v1.GET("/public/legal-documents", wrapF(api.publicLegalDocuments))
	v1.GET("/public/terminal-capabilities", wrapF(api.publicTerminalCapabilities))
	v1.GET("/inspirations/categories", wrapF(inspirations.categories))
	v1.GET("/inspirations/featured", wrapF(inspirations.featured))
	v1.GET("/inspirations", wrapF(inspirations.list))
	v1.GET("/inspirations/:id", wrapF(inspirations.detail))
	v1.POST("/inspirations/:id/events", wrapF(inspirations.event))
	v1.PUT("/inspirations/:id/favorite", wrapF(inspirations.favorite(true)))
	v1.DELETE("/inspirations/:id/favorite", wrapF(inspirations.favorite(false)))
	v1.GET("/legal/acceptance-status", wrapF(api.legalAcceptanceStatus))
	v1.POST("/legal/acceptances", wrapF(api.acceptCurrentLegalDocuments))
	v1.GET("/public/pricing", wrapF(api.plans))
	v1.POST("/public/experience-events", wrapF(publicCatalog.recordGuestExperienceEvent))
	v1.GET("/app/review-mode", wrapF(api.reviewMode))
	v1.GET("/module-schema", wrapF(api.moduleSchema))
	v1.GET("/plans", wrapF(api.plans))
	v1.GET("/plans/:id", wrapF(api.planDetail))
	v1.POST("/orders/create", wrapF(api.createCommerceOrder))
	v1.POST("/pay/callback", wrapF(api.payCallback))
	v1.GET("/member/profile", wrapF(api.memberProfile))
	v1.PATCH("/member/profile", wrapF(api.updateMemberProfile))
	v1.GET("/member/wallet", wrapF(api.memberWallet))
	v1.GET("/member/orders", wrapF(api.memberOrders))
	v1.GET("/member/orders/:id", wrapF(api.memberOrderDetail))
	v1.GET("/member/invoices", wrapF(api.memberInvoices))
	v1.POST("/member/refund-requests", wrapF(api.createMemberRefundRequest))
	v1.GET("/member/token-records", wrapF(api.memberTokenRecords))
	v1.GET("/agent/profile", wrapF(api.agentProfile))
	v1.POST("/agent/join-order", wrapF(api.createAgentJoinOrder))
	v1.GET("/operation-center/profile", wrapF(api.operationCenterProfile))
	v1.GET("/operation-center/agents", wrapF(api.operationCenterAgents))
	v1.GET("/operation-center/agents/:id", wrapF(api.operationCenterAgentDetail))
	v1.GET("/operation-center/orders", wrapF(api.operationCenterOrders))
	v1.GET("/operation-center/orders/:id", wrapF(api.operationCenterOrderDetail))
	v1.GET("/operation-center/commissions", wrapF(api.operationCenterCommissions))
	v1.GET("/operation-center/commissions/:id", wrapF(api.operationCenterCommissionDetail))
	v1.POST("/operation-center/join-order", wrapF(api.createOperationCenterJoinOrder))
	v1.GET("/points/account", wrapF(api.pointAccount))
	v1.POST("/points/recharge-orders", wrapF(api.createRechargeOrder))
	v1.POST("/points/subscription-orders", wrapF(api.createSubscriptionOrder))
	v1.GET("/payment/capability", wrapF(paymentCenter.capability))
	v1.POST("/payment/orders", wrapF(paymentCenter.createOrder))
	v1.GET("/payment/products", wrapF(virtualPayment.products))
	v1.GET("/payment/coupons", wrapF(virtualPayment.coupons))
	v1.POST("/payment/price-quotes", wrapF(virtualPayment.createPublicPriceQuote))
	v1.POST("/payment/test-price-quotes", wrapF(virtualPayment.createTestPriceQuote))
	v1.POST("/payment/wechat-virtual/orders", wrapF(virtualPayment.createOrder))
	v1.GET("/payment/orders/:orderNo", wrapF(paymentCenter.order))
	if !cfg.IsProduction() {
		v1.POST("/payment/mock/:orderNo/success", wrapF(paymentCenter.mockAction(paymentapp.MockSuccess, false)))
		v1.POST("/payment/mock/:orderNo/fail", wrapF(paymentCenter.mockAction(paymentapp.MockFailure, false)))
		v1.POST("/payment/mock/:orderNo/duplicate-notify", wrapF(paymentCenter.mockAction(paymentapp.MockSuccess, true)))
		v1.POST("/payment/mock/:orderNo/amount-mismatch", wrapF(paymentCenter.mockAction(paymentapp.MockAmountMismatch, false)))
		v1.POST("/payment/mock/:orderNo/delayed-success", wrapF(paymentCenter.mockDelayedSuccess))
	}
	v1.GET("/payment/orders/:orderNo/status", wrapF(virtualPayment.orderStatus))
	v1.POST("/payment/orders/:orderNo/sync", wrapF(virtualPayment.syncOrder))
	v1.GET("/payment/wechat-virtual/notify", wrapF(virtualPayment.verifyNotify))
	v1.POST("/payment/wechat-virtual/notify", wrapF(virtualPayment.notify))
	v1.GET("/user/dashboard", wrapF(api.userDashboard))
	v1.GET("/user/online-image", wrapF(api.userOnlineImage))
	v1.PATCH("/user/ai-state", wrapF(api.updateUserAIState))
	v1.GET("/user/api-settings", wrapF(api.userAPISettings))
	v1.GET("/user/usage", wrapF(api.userUsage))
	v1.GET("/user/usage/:id", wrapF(api.userUsageDetail))
	v1.GET("/officecli/status", wrapF(api.officeCLIStatus))
	v1.POST("/officecli/documents", wrapF(api.createOfficeCLIDocument))
	v1.GET("/officecli/documents/:fileName/download", wrapF(api.downloadOfficeCLIDocument))
	v1.GET("/generation-tasks", wrapF(api.listGenerationTasks))
	v1.POST("/generation-tasks/estimate", wrapF(api.estimateVideoGenerationCost))
	v1.GET("/generation-tasks/:id", wrapF(api.getGenerationTask))
	v1.POST("/generation-tasks/:id/retry", wrapF(api.retryGenerationTask))
	v1.POST("/generation-tasks/:id/cancel", wrapF(api.cancelGenerationTask))
	v1.DELETE("/generation-tasks/:id", wrapF(api.deleteGenerationTask))
	v1.POST("/generation-tasks", wrapF(api.createGenerationTask))
	v1.POST("/ppt/generate", wrapF(api.createPPTGenerationTask))
	v1.POST("/ppt/estimate", wrapF(api.estimatePPTGenerationCost))
	v1.POST("/ppt/outline/generate", wrapF(api.generatePPTOutline))
	v1.POST("/ppt/outline/save", wrapF(api.savePPTOutline))
	v1.POST("/ppt/agent/guide", wrapF(api.guidePPTAgent))
	v1.GET("/ppt/agent/jobs/:jobId", wrapF(api.getPPTAgentState))
	v1.GET("/ppt/agent/jobs/:jobId/download", wrapF(api.downloadPPTAgentDeck))
	v1.PATCH("/ppt/agent/jobs/:jobId/outline", wrapF(api.updatePPTAgentOutline))
	v1.POST("/ppt/agent/jobs/:jobId/outline/approve", wrapF(api.approvePPTAgentOutline))
	v1.POST("/ppt/agent/jobs/:jobId/retry", wrapF(api.retryPPTAgentPlanning))
	v1.GET("/ppt/tasks/:taskId", wrapF(api.getPPTTask))
	v1.GET("/ppt/history", wrapF(api.listPPTHistory))
	v1.DELETE("/ppt/tasks/:taskId", wrapF(api.deletePPTTask))
	v1.POST("/ppt/slides/:slideId/regenerate", wrapF(api.regeneratePPTSlide))
	v1.POST("/presentations/:id/slides/:slideId/regenerate-visual", wrapF(api.regeneratePPTSlideVisual))
	v1.PATCH("/presentations/:id/slides/:slideId", wrapF(api.updatePPTSlide))
	v1.PATCH("/presentations/:id/slides/:slideId/visual", wrapF(api.updatePPTSlideImage))
	v1.DELETE("/presentations/:id/slides/:slideId/visual", wrapF(api.deletePPTSlideVisual))
	v1.POST("/presentations/:id/slides/:slideId/visual/restore", wrapF(api.restorePPTSlideVisual))
	v1.POST("/ppt/export/pptx", wrapF(api.exportPPT))
	v1.GET("/ppt/tasks/:taskId/export/pptx", wrapF(api.downloadPPTExport))
	v1.POST("/ppt/export/pdf", wrapF(api.exportPDF))
	v1.POST("/ppt/images/generate", wrapF(api.generatePPTImage))
	v1.GET("/ppt/images/search", wrapF(api.searchPPTImages))
	v1.GET("/ppt/models/text", wrapF(api.listPPTTextModels))
	v1.GET("/ppt/models/image", wrapF(api.listPPTImageModels))
	v1.GET("/video/download", wrapF(api.downloadVideoByURL))
	v1.GET("/assets", wrapF(api.listAssets))
	v1.GET("/assets/overview", wrapF(api.assetsOverview))
	v1.GET("/works/recent", wrapF(api.listRecentWorks))
	v1.GET("/assets/projects", wrapF(api.assetProjects))
	v1.POST("/assets/batch", wrapF(api.batchAssets))
	v1.GET("/assets/:id", wrapF(api.assetDetail))
	v1.PATCH("/assets/:id", wrapF(api.updateAsset))
	v1.POST("/assets/:id/favorite", wrapF(api.favoriteAsset))
	v1.DELETE("/assets/:id/favorite", wrapF(api.favoriteAsset))
	v1.POST("/assets/:id/archive", wrapF(api.archiveAsset))
	v1.POST("/assets/:id/restore", wrapF(api.restoreAsset))
	v1.DELETE("/assets/:id/permanent", wrapF(api.permanentlyDeleteAsset))
	v1.POST("/assets/:id/move-project", wrapF(api.moveAssetProject))
	v1.POST("/assets/thumbnail-backfill", wrapF(api.backfillAssetThumbnails))
	v1.GET("/assets/:id/download", wrapF(api.downloadAsset))
	v1.DELETE("/assets/:id", wrapF(api.deleteAsset))
	v1.POST("/files/upload/init", wrapF(files.initUpload))
	v1.POST("/files/upload/complete", wrapF(files.completeUpload))
	v1.POST("/files/upload/multipart/init", wrapF(files.initMultipartUpload))
	v1.POST("/files/upload/multipart/:uploadId/parts/:partNumber", wrapF(files.presignMultipartPart))
	v1.POST("/files/upload/multipart/:uploadId/complete", wrapF(files.completeMultipartUpload))
	v1.POST("/files/upload/multipart/:uploadId/abort", wrapF(files.abortMultipartUpload))
	v1.GET("/files/:fileId", wrapF(files.getFile))
	v1.GET("/files/:fileId/access-url", wrapF(files.accessURL(false)))
	v1.GET("/files/:fileId/download-url", wrapF(files.accessURL(true)))
	v1.DELETE("/files/:fileId", wrapF(files.deleteFile))
	v1.POST("/files/:fileId/restore", wrapF(files.restoreFile))
	v1.DELETE("/files/:fileId/permanent", wrapF(files.permanentDeleteFile))
	v1.GET("/video-projects", wrapF(smartVideos.projects))
	v1.POST("/video-projects", wrapF(smartVideos.projects))
	v1.GET("/video-projects/:id", wrapF(smartVideos.project))
	v1.PATCH("/video-projects/:id", wrapF(smartVideos.project))
	v1.DELETE("/video-projects/:id", wrapF(smartVideos.project))
	v1.GET("/video-projects/:id/assets", wrapF(smartVideos.assets))
	v1.POST("/video-projects/:id/assets", wrapF(smartVideos.assets))
	v1.PUT("/video-projects/:id/assets/order", wrapF(smartVideos.reorderAssets))
	v1.DELETE("/video-projects/:id/assets/:assetId", wrapF(smartVideos.deleteAsset))
	v1.POST("/video-projects/:id/analyze", wrapF(smartVideos.analyze))
	v1.GET("/video-projects/:id/analysis", wrapF(smartVideos.analysisStatus))
	v1.POST("/video-projects/:id/assets/:assetId/retry-analysis", wrapF(smartVideos.retryAnalysis))
	v1.POST("/video-projects/:id/plan-tasks", wrapF(smartVideos.planTasks))
	v1.GET("/video-projects/:id/plan-tasks/:taskId", wrapF(smartVideos.planTask))
	v1.GET("/video-projects/:id/versions", wrapF(smartVideos.versions))
	v1.GET("/video-projects/:id/versions/:versionId", wrapF(smartVideos.version))
	v1.POST("/video-projects/:id/versions/:versionId/revisions", wrapF(smartVideos.reviseVersion))
	v1.POST("/video-projects/:id/versions/:versionId/confirm", wrapF(smartVideos.confirmVersion))
	v1.GET("/video-projects/:id/versions/:versionId/render-estimate", wrapF(smartVideos.renderEstimate))
	v1.POST("/video-projects/:id/render-tasks", wrapF(smartVideos.createRenderTask))
	v1.GET("/video-projects/:id/render-tasks/:taskId", wrapF(smartVideos.renderTask))
	v1.POST("/video-projects/:id/render-tasks/:taskId/cancel", wrapF(smartVideos.cancelRenderTask))
	v1.POST("/video-projects/:id/render-tasks/:taskId/retry", wrapF(smartVideos.retryRenderTask))
	v1.POST("/reference-images", wrapF(api.uploadReferenceImage))
	v1.GET("/reference-images/:name", wrapF(api.serveReferenceImage))
	v1.GET("/generated-media/:name", wrapF(api.serveGeneratedMedia))
	v1.GET("/knowledge/context", wrapF(knowledgeAPI.context))
	v1.GET("/knowledge/tags", wrapF(knowledgeAPI.listTags))
	v1.POST("/knowledge/tags", wrapF(knowledgeAPI.createTag))
	v1.GET("/knowledge/categories", wrapF(knowledgeAPI.listCategories))
	v1.POST("/knowledge/categories", wrapF(knowledgeAPI.createCategory))
	v1.GET("/knowledge/profiles/:resource", wrapF(knowledgeAPI.listProfiles))
	v1.GET("/knowledge-bases", wrapF(knowledgeAPI.listKnowledgeBases))
	v1.POST("/knowledge-bases", wrapF(knowledgeAPI.createKnowledgeBase))
	v1.GET("/knowledge-bases/:id", wrapF(knowledgeAPI.getKnowledgeBase))
	v1.PATCH("/knowledge-bases/:id", wrapF(knowledgeAPI.updateKnowledgeBase))
	v1.DELETE("/knowledge-bases/:id", wrapF(knowledgeAPI.deleteKnowledgeBase))
	v1.GET("/knowledge-bases/:id/acl", wrapF(knowledgeAPI.listKnowledgeBaseACL))
	v1.PUT("/knowledge-bases/:id/acl", wrapF(knowledgeAPI.replaceKnowledgeBaseACL))
	v1.GET("/knowledge-bases/:id/documents", wrapF(knowledgeAPI.listDocuments))
	v1.POST("/knowledge-bases/:id/documents:ingest", wrapF(knowledgeAPI.ingestDocument))
	v1.DELETE("/knowledge-documents/:id", wrapF(knowledgeAPI.deleteDocument))
	v1.GET("/knowledge-documents/:id", wrapF(knowledgeAPI.getDocument))
	v1.GET("/knowledge-chunks", wrapF(knowledgeAPI.listChunks))
	v1.POST("/knowledge-search", wrapF(knowledgeAPI.search))
	v1.GET("/knowledge-agents", wrapF(knowledgeAPI.listAgents))
	v1.POST("/knowledge-agents", wrapF(knowledgeAPI.createAgent))
	v1.GET("/knowledge-agents/:id", wrapF(knowledgeAPI.getAgent))
	v1.PUT("/knowledge-agents/:id/knowledge-bindings", wrapF(knowledgeAPI.replaceAgentBindings))
	v1.GET("/knowledge-conversations", wrapF(knowledgeAPI.listConversations))
	v1.POST("/knowledge-conversations", wrapF(knowledgeAPI.createConversation))
	v1.GET("/knowledge-conversations/:id/messages", wrapF(knowledgeAPI.listMessages))
	v1.POST("/knowledge-conversations/:id/runs", wrapF(knowledgeAPI.runRAG))
	v1.POST("/knowledge-conversations/:id/runs:stream", wrapF(knowledgeAPI.streamRAG))
	v1.GET("/knowledge-runs/:id", wrapF(knowledgeAPI.getRun))
	v1.POST("/knowledge-runs/:id/cancel", wrapF(knowledgeAPI.cancelRun))
	v1.POST("/knowledge-runs/:id/retry", wrapF(knowledgeAPI.retryRun))
	v1.GET("/knowledge-runs/:id/events", wrapF(knowledgeAPI.listRunEvents))
	v1.GET("/knowledge-runs/:id/citations", wrapF(knowledgeAPI.listRunCitations))
	v1.GET("/app/pages/:pageCode", wrapF(media.publicPage))
	v1.GET("/app/page-config/:pageCode", wrapF(media.publicPage))
	v1.GET("/media/files/*filepath", wrapF(serveLocalMediaFile(mediaStorage)))
	router.GET("/api/module-schema", wrapF(api.moduleSchema))

	pptGroup := router.Group("/api/ppt")
	pptGroup.POST("/generate", wrapF(api.createPPTGenerationTask))
	pptGroup.POST("/estimate", wrapF(api.estimatePPTGenerationCost))
	pptGroup.POST("/outline/generate", wrapF(api.generatePPTOutline))
	pptGroup.POST("/outline/save", wrapF(api.savePPTOutline))
	pptGroup.GET("/tasks/:taskId", wrapF(api.getPPTTask))
	pptGroup.GET("/history", wrapF(api.listPPTHistory))
	pptGroup.DELETE("/tasks/:taskId", wrapF(api.deletePPTTask))
	pptGroup.POST("/slides/:slideId/regenerate", wrapF(api.regeneratePPTSlide))
	pptGroup.POST("/export/pptx", wrapF(api.exportPPT))
	pptGroup.POST("/export/pdf", wrapF(api.exportPDF))
	pptGroup.POST("/images/generate", wrapF(api.generatePPTImage))
	pptGroup.GET("/images/search", wrapF(api.searchPPTImages))
	pptGroup.GET("/models/text", wrapF(api.listPPTTextModels))
	pptGroup.GET("/models/image", wrapF(api.listPPTImageModels))

	adminGroup := v1.Group("/admin")
	if pgStore, ok := store.(*postgresStore); ok {
		adminGroup.Use(func(c *gin.Context) {
			permission := adminPermissionForRequest(c.Request)
			pgStore.rbacMiddleware(auth, permission)(c)
		})
	} else {
		adminGroup.Use(superAdminMiddleware(auth))
	}
	adminPaymentGroup := router.Group("/api/admin")
	if pgStore, ok := store.(*postgresStore); ok {
		adminPaymentGroup.Use(func(c *gin.Context) {
			permission := adminPermissionForRequest(c.Request)
			pgStore.rbacMiddleware(auth, permission)(c)
		})
	} else {
		adminPaymentGroup.Use(superAdminMiddleware(auth))
	}
	dashboardBillingGroup := router.Group("/v1/dashboard/billing")
	if pgStore, ok := store.(*postgresStore); ok {
		dashboardBillingGroup.Use(func(c *gin.Context) {
			permission := adminPermissionForRequest(c.Request)
			pgStore.rbacMiddleware(auth, permission)(c)
		})
	} else {
		dashboardBillingGroup.Use(superAdminMiddleware(auth))
	}
	adminGroup.GET("/overview", wrapF(admin.overview))
	adminGroup.GET("/channel-ecosystem/operation-centers/:id", wrapF(operationCenterReviews.status))
	adminGroup.POST("/channel-ecosystem/operation-centers/:id/approve", wrapF(operationCenterReviews.approve))
	adminGroup.POST("/channel-ecosystem/operation-centers/:id/reject", wrapF(operationCenterReviews.reject))
	adminGroup.POST("/channel-ecosystem/operation-centers/:id/refunds", wrapF(operationCenterRefunds.requestActive))
	adminGroup.GET("/channel-ecosystem/refunds/:refundTaskId", wrapF(operationCenterRefunds.get))
	adminGroup.GET("/channel-ecosystem/refunds", wrapF(operationCenterRefunds.list))
	adminGroup.POST("/channel-ecosystem/refunds/:refundTaskId/retry", wrapF(operationCenterRefunds.retry))
	adminGroup.POST("/channel-ecosystem/refunds/:refundTaskId/manual-submit", wrapF(operationCenterRefunds.manualSubmit))
	adminGroup.POST("/channel-ecosystem/refunds/:refundTaskId/manual-approve", wrapF(operationCenterRefunds.manualApprove))
	adminGroup.GET("/search", wrapF(admin.globalSearch))
	adminGroup.PATCH("/exceptions/:id", wrapF(admin.updateExceptionCase))
	adminGroup.POST("/experience-events", wrapF(admin.recordExperienceEvent))
	adminGroup.GET("/experience-analytics", wrapF(admin.experienceAnalytics))
	adminGroup.GET("/enterprises", wrapF(adminEnterprise.list))
	adminGroup.POST("/enterprises", wrapF(adminEnterprise.create))
	adminGroup.GET("/enterprises/export", wrapF(adminEnterprise.export))
	adminGroup.GET("/enterprises/certifications", wrapF(adminEnterprise.certifications))
	adminGroup.GET("/enterprises/:enterpriseId", wrapF(adminEnterprise.detail))
	adminGroup.PATCH("/enterprises/:enterpriseId", wrapF(adminEnterprise.mutate("profile-update")))
	adminGroup.GET("/enterprises/:enterpriseId/certifications", wrapF(adminEnterprise.section("certifications")))
	adminGroup.GET("/enterprises/:enterpriseId/members", wrapF(adminEnterprise.section("members")))
	adminGroup.GET("/enterprises/:enterpriseId/package", wrapF(adminEnterprise.section("package")))
	adminGroup.GET("/enterprises/:enterpriseId/compute", wrapF(adminEnterprise.section("compute")))
	adminGroup.GET("/enterprises/:enterpriseId/transactions", wrapF(adminEnterprise.section("transactions")))
	adminGroup.GET("/enterprises/:enterpriseId/orders", wrapF(adminEnterprise.section("orders")))
	adminGroup.GET("/enterprises/:enterpriseId/ai-capabilities", wrapF(adminEnterprise.section("ai-capabilities")))
	adminGroup.GET("/enterprises/:enterpriseId/ai-employees", wrapF(adminEnterprise.section("ai-employees")))
	adminGroup.GET("/enterprises/:enterpriseId/knowledge-bases", wrapF(adminEnterprise.section("knowledge-bases")))
	adminGroup.GET("/enterprises/:enterpriseId/attribution", wrapF(adminEnterprise.section("attribution")))
	adminGroup.GET("/enterprises/:enterpriseId/relationships", wrapF(adminEnterprise.section("relationships")))
	adminGroup.GET("/enterprises/:enterpriseId/integrations", wrapF(adminEnterprise.section("integrations")))
	adminGroup.GET("/enterprises/:enterpriseId/risk", wrapF(adminEnterprise.section("risk")))
	adminGroup.GET("/enterprises/:enterpriseId/audit-logs", wrapF(adminEnterprise.section("audit-logs")))
	adminGroup.POST("/enterprises/:enterpriseId/certifications/review", wrapF(adminEnterprise.mutate("certification-review")))
	adminGroup.POST("/enterprises/:enterpriseId/package/adjust", wrapF(adminEnterprise.mutate("package-adjust")))
	adminGroup.POST("/enterprises/:enterpriseId/seats/adjust", wrapF(adminEnterprise.mutate("seat-adjust")))
	adminGroup.POST("/enterprises/:enterpriseId/compute/adjust", wrapF(adminEnterprise.mutate("compute-adjust")))
	adminGroup.POST("/enterprises/:enterpriseId/recharge", wrapF(adminEnterprise.mutate("recharge")))
	adminGroup.POST("/enterprises/:enterpriseId/ai-capabilities/configure", wrapF(adminEnterprise.mutate("ai-capability-configure")))
	adminGroup.POST("/enterprises/:enterpriseId/attribution/change", wrapF(adminEnterprise.mutate("attribution-change")))
	adminGroup.POST("/enterprises/:enterpriseId/risk/disable", wrapF(adminEnterprise.mutate("risk-disable")))
	adminGroup.POST("/enterprises/:enterpriseId/risk/restore", wrapF(adminEnterprise.mutate("risk-restore")))
	adminGroup.POST("/enterprises/:enterpriseId/service-state", wrapF(adminEnterprise.mutate("service-state")))
	adminGroup.GET("/media/assets", wrapF(media.listAssets))
	adminGroup.GET("/inspirations", wrapF(inspirations.adminList))
	adminGroup.POST("/inspirations", wrapF(inspirations.adminSave(true)))
	adminGroup.GET("/inspirations/categories", wrapF(inspirations.adminCategories))
	adminGroup.POST("/inspirations/categories", wrapF(inspirations.adminSaveCategory))
	adminGroup.GET("/inspirations/statistics", wrapF(inspirations.adminStatistics))
	adminGroup.POST("/inspirations/sort", wrapF(inspirations.adminSort))
	adminGroup.POST("/inspirations/batch", wrapF(inspirations.adminBatch))
	adminGroup.GET("/inspirations/:id", wrapF(inspirations.adminGet))
	adminGroup.PUT("/inspirations/:id", wrapF(inspirations.adminSave(false)))
	adminGroup.DELETE("/inspirations/:id", wrapF(inspirations.adminDelete))
	adminGroup.POST("/inspirations/:id/copy", wrapF(inspirations.adminCopy))
	adminGroup.POST("/inspirations/:id/publish", wrapF(inspirations.adminTransition("publish")))
	adminGroup.POST("/inspirations/:id/withdraw", wrapF(inspirations.adminTransition("withdraw")))
	adminGroup.POST("/inspirations/:id/audit/approve", wrapF(inspirations.adminTransition("approve")))
	adminGroup.POST("/inspirations/:id/audit/reject", wrapF(inspirations.adminTransition("reject")))
	adminGroup.GET("/inspirations/:id/versions", wrapF(inspirations.adminVersions))
	adminGroup.POST("/inspirations/:id/rollback", wrapF(inspirations.adminRollback))
	adminGroup.POST("/media/assets/upload", wrapF(media.uploadAsset))
	adminGroup.POST("/media/assets/batch-upload", wrapF(media.uploadAsset))
	adminGroup.GET("/media/assets/:id", wrapF(media.getAsset))
	adminGroup.PUT("/media/assets/:id", wrapF(media.updateAsset))
	adminGroup.DELETE("/media/assets/:id", wrapF(media.deleteAsset))
	adminGroup.POST("/media/assets/:id/enable", wrapF(media.enableAsset))
	adminGroup.POST("/media/assets/:id/disable", wrapF(media.disableAsset))
	adminGroup.GET("/media/assets/:id/usages", wrapF(media.assetUsages))
	adminGroup.GET("/media/categories", wrapF(media.listCategories))
	adminGroup.POST("/media/categories", wrapF(media.createCategory))
	adminGroup.PUT("/media/categories/:id", wrapF(media.updateCategory))
	adminGroup.DELETE("/media/categories/:id", wrapF(media.deleteCategory))
	adminGroup.GET("/page-configs/:pageCode", wrapF(media.getAdminPageConfig))
	adminGroup.PUT("/page-configs/:pageCode", wrapF(media.savePageDraft))
	adminGroup.POST("/page-configs/:pageCode/publish", wrapF(media.publishPage))
	adminGroup.GET("/page-configs/:pageCode/versions", wrapF(media.listPageVersions))
	adminGroup.POST("/page-configs/:pageCode/rollback/:version", wrapF(media.rollbackPage))
	adminGroup.GET("/page-slots/:pageCode", wrapF(media.listPageSlots))
	adminGroup.PUT("/page-slots/:pageCode/:slotKey", wrapF(media.updatePageSlot))
	adminGroup.GET("/storage/configs", wrapF(files.listConfigs))
	adminGroup.POST("/storage/configs", wrapF(files.saveConfig(true)))
	adminGroup.PUT("/storage/configs/:id", wrapF(files.saveConfig(false)))
	adminGroup.DELETE("/storage/configs/:id", wrapF(files.deleteConfig))
	adminGroup.POST("/storage/configs/:id/test", wrapF(files.testConfig))
	adminGroup.GET("/storage/files", wrapF(files.adminListFiles))
	adminGroup.GET("/storage/files/:fileId", wrapF(files.adminGetFile))
	adminGroup.GET("/storage/files/:fileId/download-url", wrapF(files.accessURL(true)))
	adminGroup.DELETE("/storage/files/:fileId", wrapF(files.deleteFile))
	adminGroup.POST("/storage/files/:fileId/restore", wrapF(files.restoreFile))
	adminGroup.DELETE("/storage/files/:fileId/permanent", wrapF(files.permanentDeleteFile))
	adminGroup.GET("/storage/statistics/overview", wrapF(files.adminOverview))
	adminGroup.GET("/storage/quotas", wrapF(files.getQuota))
	adminGroup.PUT("/storage/quotas/:tenantId", wrapF(files.updateQuota))
	adminGroup.GET("/knowledge/overview", wrapF(knowledgeAPI.adminOverview))
	adminGroup.GET("/knowledge/:resource", wrapF(knowledgeAPI.adminRecords))
	adminGroup.POST("/knowledge/:resource", wrapF(knowledgeAPI.saveAdminProfile))
	adminGroup.PATCH("/knowledge/:resource/:id", wrapF(knowledgeAPI.saveAdminProfile))
	adminGroup.GET("/customer-attributions", wrapF(admin.customerAttributions))
	adminGroup.GET("/points/expiry-policy", wrapF(admin.pointExpiryPolicy))
	adminGroup.PUT("/points/expiry-policy", wrapF(admin.pointExpiryPolicy))
	adminGroup.GET("/customers", wrapF(admin.customers))
	adminGroup.GET("/customers/:id/360", wrapF(admin.customer360))
	adminGroup.GET("/customers/:id/identity-profile", wrapF(identityQueries.profile))
	adminGroup.GET("/customers/:id/identity-history", wrapF(identityQueries.history))
	adminGroup.GET("/customers/:id/relationship", wrapF(identityQueries.currentRelationship))
	adminGroup.GET("/customers/:id/relationship-history", wrapF(identityQueries.relationshipHistory))
	adminGroup.GET("/customers/:id/identity-financial-overview", wrapF(identityQueries.financialOverview))
	adminGroup.GET("/identity-consistency", wrapF(identityConsistencyAPI{store: store}.list))
	adminGroup.POST("/users/:id/identity-change/preview", wrapF(identityChanges.preview))
	adminGroup.POST("/users/:id/identity-change/review", wrapF(identityChanges.review))
	adminGroup.POST("/users/:id/identity-change/confirm", wrapF(identityChanges.confirm))
	adminGroup.POST("/users/:id/identity-downgrade/preview", wrapF(identityDowngrades.preview))
	adminGroup.POST("/users/:id/identity-downgrade/confirm", wrapF(identityDowngrades.confirm))
	adminGroup.GET("/users/:id/identity-downgrade/requests", wrapF(identityDowngrades.list))
	adminGroup.POST("/users/:id/identity-downgrade/requests/:requestId/recheck", wrapF(identityDowngrades.recheck))
	adminGroup.POST("/users/:id/identity-downgrade/requests/:requestId/cancel", wrapF(identityDowngrades.cancel))
	adminGroup.POST("/users/:id/identity-downgrade/requests/:requestId/reschedule", wrapF(identityDowngrades.reschedule))
	adminGroup.POST("/customers", wrapF(admin.createCustomer))
	adminGroup.PATCH("/customers/:id", wrapF(admin.updateCustomer))
	adminGroup.POST("/customers/:id/point-gifts", wrapF(admin.customerPointGift))
	adminGroup.POST("/customers/:id/point-corrections", wrapF(admin.customerPointCorrection))
	adminGroup.GET("/customers/:id/point-lots", wrapF(admin.customerPointLots))
	adminGroup.GET("/customers/:id/identities", wrapF(admin.customerIdentities))
	adminGroup.GET("/customers/:id/account-merge-requests", wrapF(admin.customerAuthMergeRequests))
	adminGroup.POST("/customers/:id/identities/mobile/unlink", wrapF(admin.unlinkCustomerMobile))
	adminGroup.POST("/customers/:id/identities/wechat-mini-program/unlink", wrapF(admin.unlinkCustomerWeChat))
	adminGroup.POST("/customers/:id/freeze-login", wrapF(admin.freezeCustomerLogin))
	adminGroup.POST("/customers/:id/unfreeze-login", wrapF(admin.unfreezeCustomerLogin))
	adminGroup.POST("/customers/:id/logout-all", wrapF(admin.forceLogoutCustomer))
	adminGroup.GET("/account-merge-requests", wrapF(admin.authMergeRequests))
	adminGroup.POST("/account-merge-requests/:id/status", wrapF(admin.updateAuthMergeRequest))
	adminGroup.GET("/account-merge-requests/:id/preview", wrapF(admin.previewAuthMergeRequest))
	adminGroup.POST("/account-merge-requests/:id/execute", wrapF(admin.executeAuthMergeRequest))
	adminGroup.POST("/customers/:id/sync-newapi", wrapF(admin.syncCustomerNewAPI))
	adminGroup.GET("/newapi/groups", wrapF(admin.newAPIGroups))
	adminGroup.GET("/channel-agents", wrapF(admin.channelAgents))
	adminGroup.POST("/channel-agents", wrapF(admin.createChannelAgent))
	adminGroup.GET("/channel-agents/tree", wrapF(admin.channelAgentTree))
	adminGroup.PATCH("/channel-agents/:id", wrapF(admin.updateChannelAgent))
	adminGroup.GET("/operation-centers", wrapF(admin.operationCenters))
	adminGroup.PATCH("/operation-centers/:id/profile", wrapF(operationCenterProfileAPI{store: store}.update))
	adminGroup.GET("/products", wrapF(admin.products))
	adminGroup.PATCH("/products/:id", wrapF(admin.updateProduct))
	adminGroup.GET("/plans", wrapF(admin.plans))
	adminGroup.PATCH("/plans/:id", wrapF(admin.updatePlan))
	adminGroup.GET("/plans/:id/capabilities", wrapF(admin.planCapabilities))
	adminGroup.PUT("/plans/:id/capabilities", wrapF(admin.updatePlanCapabilities))
	adminGroup.GET("/business-plans", wrapF(businessPlans.plans))
	adminGroup.GET("/business-plans/:planId", wrapF(businessPlans.plan))
	adminGroup.GET("/business-plans/:planId/versions", wrapF(businessPlans.versions))
	adminGroup.POST("/business-plans/:planId/versions", wrapF(businessPlans.createVersion))
	adminGroup.PATCH("/plan-versions/:versionId", wrapF(businessPlans.updateVersion))
	adminGroup.POST("/plan-versions/:versionId/activate", wrapF(businessPlans.activateVersion))
	adminGroup.POST("/plan-versions/:versionId/retire", wrapF(businessPlans.retireVersion))
	adminGroup.GET("/business-plans/:planId/price-plans", wrapF(pricePlans.plans))
	adminGroup.POST("/business-plans/:planId/price-plans", wrapF(pricePlans.createPlan))
	adminGroup.GET("/price-plans/:pricePlanId", wrapF(pricePlans.plan))
	adminGroup.PATCH("/price-plans/:pricePlanId", wrapF(pricePlans.updatePlan))
	adminGroup.POST("/price-plans/:pricePlanId/clone", wrapF(pricePlans.clonePlan))
	adminGroup.GET("/price-plans/:pricePlanId/validation", wrapF(pricePlans.validation))
	adminGroup.POST("/price-plans/:pricePlanId/enable", wrapF(pricePlans.enablePlan))
	adminGroup.POST("/price-plans/:pricePlanId/disable", wrapF(pricePlans.disablePlan))
	adminGroup.POST("/price-plans/:pricePlanId/make-default", wrapF(pricePlans.makeDefault))
	adminGroup.GET("/price-plans/:pricePlanId/whitelist", wrapF(pricePlanTestWhitelist.list))
	adminGroup.POST("/price-plans/:pricePlanId/whitelist", wrapF(pricePlanTestWhitelist.create))
	adminGroup.PATCH("/price-plans/:pricePlanId/whitelist/:entryId", wrapF(pricePlanTestWhitelist.update))
	adminGroup.POST("/price-plans/:pricePlanId/whitelist/:entryId/disable", wrapF(pricePlanTestWhitelist.disable))
	adminGroup.GET("/pricing-audit-logs", wrapF(pricingAudit.list))
	adminGroup.GET("/pricing-health", wrapF(pricingHealth.get))
	adminGroup.GET("/wechat-virtual-goods", wrapF(wechatGoods.goods))
	adminGroup.GET("/wechat-virtual-goods/:goodId", wrapF(wechatGoods.good))
	adminGroup.GET("/wechat-virtual-goods/:goodId/references", wrapF(wechatGoods.references))
	adminGroup.POST("/wechat-virtual-goods", wrapF(wechatGoods.createGood))
	adminGroup.PATCH("/wechat-virtual-goods/:goodId", wrapF(wechatGoods.updateGood))
	adminGroup.POST("/wechat-virtual-goods/:goodId/confirm-published", wrapF(wechatGoods.confirmGood))
	adminGroup.POST("/wechat-virtual-goods/:goodId/disable", wrapF(wechatGoods.disableGood))
	adminGroup.GET("/price-plans/:pricePlanId/payment-bindings", wrapF(wechatGoods.bindings))
	adminGroup.POST("/price-plans/:pricePlanId/payment-bindings", wrapF(wechatGoods.createBinding))
	adminGroup.PATCH("/payment-bindings/:bindingId", wrapF(wechatGoods.updateBinding))
	adminGroup.GET("/orders", wrapF(admin.orders))
	adminGroup.GET("/orders/:id/timeline", wrapF(admin.orderTimeline))
	adminGroup.POST("/orders", wrapF(admin.createOrder))
	adminGroup.POST("/orders/:id/mark-paid", wrapF(admin.markOrderPaid))
	adminGroup.POST("/orders/:id/renew", wrapF(admin.renewOrder))
	adminGroup.GET("/payment/virtual/overview", wrapF(virtualPayment.adminOverview))
	adminGroup.GET("/payment/orders", wrapF(paymentCenter.adminOrders))
	adminGroup.GET("/payment/orders/:orderNo", wrapF(paymentCenter.adminOrder))
	adminGroup.GET("/payment/transactions", wrapF(paymentCenter.adminTransactions))
	adminGroup.GET("/payment/fulfillments", wrapF(paymentCenter.adminFulfillments))
	adminGroup.POST("/payment/fulfillments/:id/retry", wrapF(paymentCenter.adminRetryFulfillment))
	adminPaymentGroup.GET("/payment/orders", wrapF(paymentCenter.adminOrders))
	adminPaymentGroup.GET("/payment/orders/:orderNo", wrapF(paymentCenter.adminOrder))
	adminPaymentGroup.GET("/payment/transactions", wrapF(paymentCenter.adminTransactions))
	adminPaymentGroup.GET("/payment/fulfillments", wrapF(paymentCenter.adminFulfillments))
	adminPaymentGroup.POST("/payment/fulfillments/:id/retry", wrapF(paymentCenter.adminRetryFulfillment))
	adminGroup.GET("/payment/virtual/products", wrapF(virtualPayment.adminProducts))
	adminGroup.PATCH("/payment/virtual/mappings/:id", wrapF(virtualPayment.adminUpdateMapping))
	adminGroup.GET("/payment/virtual/orders", wrapF(virtualPayment.adminList("orders")))
	adminGroup.GET("/payment/virtual/records", wrapF(virtualPayment.adminList("records")))
	adminGroup.GET("/payment/virtual/notifications", wrapF(virtualPayment.adminList("notifications")))
	adminGroup.GET("/payment/virtual/memberships", wrapF(virtualPayment.adminList("memberships")))
	adminGroup.GET("/payment/virtual/wallet-ledger", wrapF(virtualPayment.adminList("wallet-ledger")))
	adminGroup.GET("/payment/virtual/refunds", wrapF(virtualPayment.adminList("refunds")))
	adminGroup.GET("/payment/virtual/failures", wrapF(virtualPayment.adminList("failures")))
	adminGroup.POST("/payment/virtual/orders/:orderNo/grant", wrapF(virtualPayment.adminGrantOrder))
	adminGroup.POST("/payment/virtual/orders/:orderNo/notify-provide-goods", wrapF(virtualPayment.adminNotifyProvideGoods))
	adminGroup.GET("/delivery-projects", wrapF(admin.deliveryProjects))
	adminGroup.PATCH("/delivery-projects/:id", wrapF(admin.updateDeliveryProject))
	adminGroup.GET("/generation-tasks", wrapF(admin.generationTasks))
	adminGroup.GET("/ai/overview", wrapF(admin.aiOverview))
	adminGroup.GET("/compliance/miniprogram-launch-check", wrapF(admin.miniProgramComplianceCheck))
	adminGroup.GET("/compliance/legal-documents", wrapF(admin.legalDocuments))
	adminGroup.PUT("/compliance/legal-documents/:code", wrapF(admin.saveLegalDocument))
	adminGroup.GET("/compliance/content-audits", wrapF(admin.contentAudits))
	adminGroup.PATCH("/compliance/content-audits/:id", wrapF(admin.reviewContentAudit))
	adminGroup.PATCH("/ai/modules/:code", wrapF(admin.updateAIModule))
	adminGroup.POST("/ai/models", wrapF(admin.createAIModel))
	adminGroup.PATCH("/ai/models/:id", wrapF(admin.updateAIModel))
	adminGroup.PATCH("/ai/parameter-schemas/:id", wrapF(admin.updateAIParameterSchema))
	adminGroup.PATCH("/ai/tenant-module-limits/:id", wrapF(admin.updateTenantModuleLimit))
	adminGroup.GET("/usage", wrapF(admin.usage))
	adminGroup.GET("/usage/export", wrapF(admin.exportUsage))
	adminGroup.GET("/token-records", wrapF(admin.tokenRecords))
	adminGroup.GET("/commissions", wrapF(admin.commissions))
	adminGroup.GET("/commission-records", wrapF(admin.commissionRecords))
	adminGroup.GET("/commission-rules", wrapF(admin.commissionRulesV2))
	adminGroup.POST("/commission-rules", wrapF(admin.createCommissionRuleV2))
	adminGroup.PUT("/commission-rules/:id", wrapF(admin.updateCommissionRuleV2))
	adminGroup.GET("/channel-ecosystem/shadow-differences", wrapF(admin.commerceShadowDifferences))
	adminGroup.GET("/channel-ecosystem/shadow-differences/:id", wrapF(admin.commerceShadowDifference))
	adminGroup.GET("/channel-ecosystem/rollout-config", wrapF(admin.channelRolloutConfig))
	adminGroup.PUT("/channel-ecosystem/rollout-config", wrapF(admin.updateChannelRolloutConfig))
	adminGroup.POST("/commissions", wrapF(admin.createCommission))
	adminGroup.POST("/commissions/:id/approve", wrapF(admin.approveCommission))
	adminGroup.POST("/commissions/:id/reject", wrapF(admin.rejectCommission))
	adminGroup.POST("/withdrawals", wrapF(admin.createWithdrawal))
	adminGroup.POST("/withdrawals/:id/approve", wrapF(admin.approveWithdrawal))
	adminGroup.POST("/withdrawals/:id/reject", wrapF(admin.rejectWithdrawal))
	adminGroup.GET("/system/settings", wrapF(admin.systemSettings))
	adminGroup.PATCH("/system/settings", wrapF(admin.updateSystemSettings))
	adminGroup.GET("/api/provider-channels", wrapF(admin.apiProviderChannels))
	adminGroup.POST("/api/provider-channels", wrapF(admin.createAPIProviderChannel))
	adminGroup.PATCH("/api/provider-channels/:id", wrapF(admin.updateAPIProviderChannel))
	adminGroup.POST("/api/provider-channels/:id/test", wrapF(admin.testAPIProviderChannel))
	adminGroup.POST("/api/provider-channels/:id/fetch-models", wrapF(admin.fetchAPIProviderChannelModels))
	adminGroup.GET("/api/models", wrapF(admin.apiModels))
	adminGroup.PATCH("/api/models/:id", wrapF(admin.updateAPIModel))
	adminGroup.GET("/api/keys", wrapF(admin.apiKeys))
	adminGroup.POST("/api/keys", wrapF(admin.createAPIKey))
	adminGroup.PATCH("/api/keys/:id", wrapF(admin.updateAPIKey))
	adminGroup.GET("/customer-groups", wrapF(admin.customerGroups))
	adminGroup.PATCH("/customer-groups/:id", wrapF(admin.updateCustomerGroup))
	adminGroup.GET("/marketing/overview", wrapF(admin.marketingOverview))
	adminGroup.GET("/marketing/agent-levels", wrapF(admin.marketingAgentLevels))
	adminGroup.GET("/marketing/invite-records", wrapF(admin.marketingInviteRecords))
	adminGroup.GET("/marketing/commission-rules", wrapF(admin.marketingCommissionRules))
	adminGroup.PATCH("/marketing/commission-rules/:id", wrapF(admin.updateMarketingCommissionRule))
	adminGroup.GET("/marketing/upgrade-plans", wrapF(admin.marketingUpgradePlans))
	adminGroup.GET("/marketing/wallets", wrapF(admin.marketingWallets))
	adminGroup.GET("/marketing/wallet-records", wrapF(admin.marketingWalletRecords))
	adminGroup.GET("/marketing/settlement-statements", wrapF(admin.marketingSettlementStatements))
	adminGroup.GET("/billing/overview", wrapF(admin.billingOverview))
	adminGroup.GET("/billing/rules", wrapF(admin.billingRulesV1))
	adminGroup.GET("/billing/rules/:id", wrapF(admin.billingRuleV1))
	adminGroup.POST("/billing/rules/:id/validate", wrapF(admin.validateBillingRuleV1))
	adminGroup.POST("/billing/rules/:id/publish", wrapF(admin.publishBillingRuleV1))
	adminGroup.GET("/billing/provider-costs", wrapF(admin.providerCostsV1))
	adminGroup.PATCH("/billing/provider-costs/:id", wrapF(admin.updateProviderCostV1))
	adminGroup.GET("/billing/reconciliation", wrapF(admin.reconciliationV1))
	adminGroup.GET("/billing/wallet-ledger", wrapF(admin.walletLedgerV1))
	adminGroup.GET("/billing/customers", wrapF(admin.billingCustomers))
	adminGroup.GET("/billing/products", wrapF(admin.billingProducts))
	adminGroup.GET("/billing/plans", wrapF(admin.billingPlans))
	adminGroup.GET("/billing/subscriptions", wrapF(admin.billingSubscriptions))
	adminGroup.GET("/billing/events", wrapF(admin.billingEvents))
	adminGroup.GET("/billing/usage-summaries", wrapF(admin.billingUsageSummaries))
	adminGroup.GET("/billing/billable-metrics", wrapF(admin.billingBillableMetrics))
	adminGroup.GET("/billing/charges", wrapF(admin.billingCharges))
	adminGroup.PATCH("/billing/rules/:id", wrapF(admin.updateBillingRule))
	adminGroup.GET("/billing/fees", wrapF(admin.billingFees))
	adminGroup.GET("/billing/wallets", wrapF(admin.billingWallets))
	adminGroup.GET("/billing/coupons", wrapF(admin.billingCoupons))
	adminGroup.POST("/billing/coupons", wrapF(admin.createBillingCoupon))
	adminGroup.PATCH("/billing/coupons/:id", wrapF(admin.updateBillingCoupon))
	adminGroup.GET("/billing/invoices", wrapF(admin.billingInvoices))
	adminGroup.PATCH("/billing/invoices/:id", wrapF(admin.updateBillingInvoice))
	adminGroup.GET("/billing/credit-notes", wrapF(admin.billingCreditNotes))
	adminGroup.POST("/billing/credit-notes", wrapF(admin.createBillingCreditNote))
	adminGroup.PATCH("/billing/credit-notes/:id", wrapF(admin.reviewBillingCreditNote))
	adminGroup.GET("/billing/payment-requests", wrapF(admin.billingPaymentRequests))
	adminGroup.POST("/billing/payment-requests/:id/dunning", wrapF(admin.recordBillingDunning))
	adminGroup.GET("/billing/payments", wrapF(admin.billingPayments))
	adminGroup.PATCH("/billing/subscriptions/:id", wrapF(admin.updateBillingSubscription))

	dashboardBillingGroup.GET("/subscription", wrapF(admin.billingSubscription))
	dashboardBillingGroup.GET("/usage", wrapF(admin.billingUsage))
	registerWirelessCanvasCompatibilityRoutes(router, cfg)
	router.GET("/", gin.WrapF(staticIndex(cfg.AdminStaticDir)))
	router.GET("/login", gin.WrapF(staticIndex(cfg.AdminStaticDir)))
	router.GET("/register", gin.WrapF(staticIndex(cfg.AdminStaticDir)))
	router.GET("/h5", wrapF(redirectToUserH5Slash))
	router.GET("/h5/*filepath", gin.WrapH(staticPrefixFiles("/h5/", cfg.UserH5StaticDir)))
	router.GET("/assets/*filepath", gin.WrapH(staticPrefixFiles("/assets/", filepath.Join(cfg.StaticDir, "assets"))))
	router.GET("/static/*filepath", gin.WrapH(staticPrefixFiles("/static/", filepath.Join(cfg.StaticDir, "static"))))
	router.GET("/mobile", wrapF(notFound))
	router.GET("/mobile/*filepath", wrapF(notFound))
	router.GET("/pages/*filepath", gin.WrapH(staticPrefixFiles("/pages/", cfg.StaticDir)))
	router.GET("/admin", wrapF(redirectToAdminSlash))
	router.GET("/admin/*filepath", gin.WrapH(staticPrefixFiles("/admin/", cfg.AdminStaticDir)))
	router.GET("/app", wrapF(redirectToRoot))
	router.GET("/app/*filepath", wrapF(redirectToRoot))
	router.GET("/workspace", wrapF(redirectToRoot))
	router.GET("/workspace/*filepath", wrapF(redirectToRoot))
	router.GET("/agent", gin.WrapF(staticIndex(cfg.AdminStaticDir)))
	router.GET("/agent/*filepath", gin.WrapH(staticPrefixFiles("/agent/", cfg.AdminStaticDir)))
	router.GET("/user", wrapF(notFound))
	router.GET("/user/*filepath", wrapF(notFound))
	router.NoRoute(redirectUnknownWebToRoot)

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}
	if operationCenterRuntime != nil {
		schedulers, err := operationCenterRuntime.StartSchedulers(context.Background(), slog.Default())
		if err != nil {
			slog.Error("operation center scheduler startup rejected", "environment", cfg.Environment, "error", err)
			if operationCenterProductionEnvironment(cfg.Environment) {
				panic(err)
			}
		} else {
			registerWaitableShutdownHook(server, schedulers.Stop)
		}
	}
	if _, err := configurePersonalPointExpiryWorker(server, store, slog.Default()); err != nil {
		slog.Error("personal point expiry worker disabled by invalid configuration", "error", err)
	}
	return server
}

func operationCenterProductionEnvironment(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "production", "prod":
		return true
	default:
		return false
	}
}

const requestIDHeader = "X-Request-Id"
const corsAllowedHeaders = "Authorization, Content-Type, Idempotency-Key, X-Request-Id, X-Client-Platform, X-Client-Name, X-Client-Version, X-Client-Language, X-Device-Id, X-Tenant-Id, X-Organization-Id"
const corsAllowedMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"

type requestIDContextKeyType struct{}

var requestIDContextKey requestIDContextKeyType

func requestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey).(string)
	return strings.TrimSpace(requestID)
}

func requestContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := sanitizeRequestID(c.GetHeader(requestIDHeader))
		if requestID == "" {
			requestID = newRequestID()
		}
		c.Set("request_id", requestID)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestIDContextKey, requestID))
		c.Header(requestIDHeader, requestID)
		c.Header("Access-Control-Expose-Headers", requestIDHeader)
		c.Next()
	}
}

func corsMiddleware(allowedOrigins string) gin.HandlerFunc {
	allowed, allowAll := parseAllowedOrigins(allowedOrigins)
	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		if origin == "" {
			c.Next()
			return
		}
		if !allowAll {
			if _, ok := allowed[origin]; !ok {
				c.Next()
				return
			}
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
		} else {
			c.Header("Access-Control-Allow-Origin", "*")
		}
		c.Header("Access-Control-Allow-Methods", corsAllowedMethods)
		c.Header("Access-Control-Allow-Headers", corsAllowedHeaders)
		c.Header("Access-Control-Expose-Headers", requestIDHeader)
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func parseAllowedOrigins(value string) (map[string]struct{}, bool) {
	allowed := make(map[string]struct{})
	for _, item := range strings.Split(value, ",") {
		origin := strings.TrimSpace(item)
		if origin == "" {
			continue
		}
		if origin == "*" {
			return allowed, true
		}
		allowed[origin] = struct{}{}
	}
	return allowed, false
}

func sanitizeRequestID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 128 {
		return ""
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':' {
			continue
		}
		return ""
	}
	return value
}

func newRequestID() string {
	var randomBytes [8]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		return "req_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return "req_" + strconv.FormatInt(time.Now().UnixNano(), 36) + "_" + hex.EncodeToString(randomBytes[:])
}

func wrapF(handler http.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, param := range c.Params {
			c.Request.SetPathValue(param.Key, param.Value)
		}
		handler(c.Writer, c.Request)
	}
}

func redirectToAdminSlash(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
}

func redirectToUserH5Slash(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/h5/", http.StatusMovedPermanently)
}

func redirectToRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "xianzhi-ai-go-gin",
	})
}

func notFound(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	payload := map[string]any{"error": err.Error()}
	if coded, ok := err.(interface{ BusinessCode() string }); ok {
		payload["code"] = coded.BusinessCode()
		payload["message"] = err.Error()
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func staticIndex(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setStaticCacheHeaders(w, filepath.Join(root, "index.html"), true)
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	}
}

func platformIndex(mobileRoot string, desktopRoot string) http.HandlerFunc {
	mobileIndex := staticIndex(mobileRoot)
	desktopIndex := staticIndex(desktopRoot)
	return func(w http.ResponseWriter, r *http.Request) {
		if isMobileWebRequest(r) {
			mobileIndex(w, r)
			return
		}
		desktopIndex(w, r)
	}
}

func isMobileWebRequest(r *http.Request) bool {
	userAgent := strings.ToLower(r.UserAgent())
	return strings.Contains(userAgent, "android") || strings.Contains(userAgent, "iphone") || strings.Contains(userAgent, "ipad") || strings.Contains(userAgent, "ipod") || strings.Contains(userAgent, "mobile")
}

func mobileIndexOrDesktopRedirect(root string) http.HandlerFunc {
	mobileIndex := staticIndex(root)
	return func(w http.ResponseWriter, r *http.Request) {
		if isMobileWebRequest(r) {
			mobileIndex(w, r)
			return
		}
		redirectToRoot(w, r)
	}
}

func mobilePrefixOrDesktopRedirect(prefix string, root string) http.Handler {
	mobileFiles := staticPrefixFiles(prefix, root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isMobileWebRequest(r) {
			mobileFiles.ServeHTTP(w, r)
			return
		}
		redirectToRoot(w, r)
	})
}

func staticFiles(root string) http.HandlerFunc {
	fileServer := http.FileServer(http.Dir(root))
	return func(w http.ResponseWriter, r *http.Request) {
		cleanURLPath := path.Clean("/" + r.URL.Path)
		localPath := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(cleanURLPath, "/")))
		if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
			setStaticCacheHeaders(w, localPath, false)
			fileServer.ServeHTTP(w, r)
			return
		}
		setStaticCacheHeaders(w, filepath.Join(root, "index.html"), true)
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	}
}

func staticOrAPINotFound(root string) gin.HandlerFunc {
	staticHandler := staticFiles(root)
	return func(c *gin.Context) {
		requestPath := path.Clean("/" + c.Request.URL.Path)
		if strings.HasPrefix(requestPath, "/api/") || strings.HasPrefix(requestPath, "/v1/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		staticHandler(c.Writer, c.Request)
	}
}

func platformOrAPINotFound(mobileRoot string) gin.HandlerFunc {
	mobileHandler := staticFiles(mobileRoot)
	return func(c *gin.Context) {
		requestPath := path.Clean("/" + c.Request.URL.Path)
		if strings.HasPrefix(requestPath, "/api/") || strings.HasPrefix(requestPath, "/v1/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if isMobileWebRequest(c.Request) {
			mobileHandler(c.Writer, c.Request)
			return
		}
		redirectToRoot(c.Writer, c.Request)
	}
}

func redirectUnknownWebToRoot(c *gin.Context) {
	requestPath := path.Clean("/" + c.Request.URL.Path)
	if strings.HasPrefix(requestPath, "/api/") || strings.HasPrefix(requestPath, "/v1/") {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	redirectToRoot(c.Writer, c.Request)
}

func staticPrefixFiles(prefix string, root string) http.Handler {
	fileServer := http.StripPrefix(strings.TrimSuffix(prefix, "/"), http.FileServer(http.Dir(root)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanURLPath := path.Clean("/" + strings.TrimPrefix(r.URL.Path, prefix))
		localPath := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(cleanURLPath, "/")))
		if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
			setStaticCacheHeaders(w, localPath, false)
			fileServer.ServeHTTP(w, r)
			return
		}
		setStaticCacheHeaders(w, filepath.Join(root, "index.html"), true)
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	})
}

func setStaticCacheHeaders(w http.ResponseWriter, localPath string, index bool) {
	if index || strings.EqualFold(filepath.Ext(localPath), ".html") {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=604800")
	w.Header().Del("Pragma")
}

func gzipMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !gzipEligibleRequest(c.Request) {
			c.Next()
			return
		}
		writer := gzip.NewWriter(c.Writer)
		defer writer.Close()
		c.Header("Content-Encoding", "gzip")
		c.Header("Vary", "Accept-Encoding")
		c.Writer.Header().Del("Content-Length")
		c.Writer = &gzipResponseWriter{ResponseWriter: c.Writer, writer: writer}
		c.Next()
	}
}

func gzipEligibleRequest(r *http.Request) bool {
	if r == nil || r.Method == http.MethodHead || r.Header.Get("Range") != "" {
		return false
	}
	if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		return false
	}
	if strings.HasSuffix(r.URL.Path, ".json") {
		return true
	}
	switch strings.ToLower(path.Ext(r.URL.Path)) {
	case ".css", ".js", ".html", ".svg", ".txt", ".md", ".map":
		return true
	default:
		return false
	}
}

type gzipResponseWriter struct {
	gin.ResponseWriter
	writer *gzip.Writer
}

func (w *gzipResponseWriter) Write(data []byte) (int, error) {
	w.Header().Del("Content-Length")
	return w.writer.Write(data)
}

func (w *gzipResponseWriter) WriteString(data string) (int, error) {
	w.Header().Del("Content-Length")
	return w.writer.Write([]byte(data))
}

func (w *gzipResponseWriter) WriteHeader(statusCode int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(statusCode)
}
