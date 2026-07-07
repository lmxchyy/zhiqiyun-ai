package httpserver

import (
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"xianzhi-ai/backend-go/internal/config"
)

func New(cfg config.Config) *http.Server {
	return newWithStore(cfg, newJSONStore(cfg.DataPath))
}

func NewWithDatabase(cfg config.Config, db *sql.DB) *http.Server {
	return NewWithInfrastructure(cfg, db, nil)
}

func NewWithInfrastructure(cfg config.Config, db *sql.DB, redisClient *redis.Client) *http.Server {
	store := platformStore(newJSONStore(cfg.DataPath))
	if db != nil {
		store = newPostgresPrimaryStore(db, cfg.DataPath)
	}
	return newWithStoreAndSessions(cfg, store, newRedisAuthSessions(redisClient))
}

func newWithStore(cfg config.Config, store platformStore) *http.Server {
	return newWithStoreAndSessions(cfg, store, nil)
}

func newWithStoreAndSessions(cfg config.Config, store platformStore, sessions authSessionStore) *http.Server {
	api := newAPI(store, cfg, sessions)
	admin := newAdminAPI(store)
	auth := newAuthAPI(store, sessions)
	channel := newChannelAPI(store, sessions)
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gzipMiddleware())
	if pgStore, ok := store.(*postgresStore); ok {
		router.Use(pgStore.auditMiddleware())
	}

	router.GET("/healthz", wrapF(health))
	v1 := router.Group("/api/v1")
	v1.GET("/health", wrapF(health))
	v1.POST("/auth/login", wrapF(auth.login))
	v1.POST("/auth/register", wrapF(auth.register))
	v1.GET("/auth/me", wrapF(auth.me))
	v1.POST("/auth/logout", wrapF(auth.logout))
	v1.POST("/auth/change-password", wrapF(auth.changePassword))
	v1.GET("/channel/me", wrapF(channel.me))
	v1.GET("/channel/customers", wrapF(channel.customers))
	v1.GET("/channel/customers/:id", wrapF(channel.customerDetail))
	v1.GET("/channel/orders", wrapF(channel.orders))
	v1.GET("/channel/usage", wrapF(channel.usage))
	v1.GET("/channel/commissions", wrapF(channel.commissions))
	v1.GET("/channel/withdrawals", wrapF(channel.withdrawals))
	v1.POST("/channel/withdrawals", wrapF(channel.createWithdrawal))
	v1.POST("/channel/children", wrapF(channel.createChildAgent))
	v1.GET("/models", wrapF(api.models))
	v1.GET("/module-schema", wrapF(api.moduleSchema))
	v1.GET("/plans", wrapF(api.plans))
	v1.GET("/plans/:id", wrapF(api.planDetail))
	v1.POST("/orders/create", wrapF(api.createCommerceOrder))
	v1.POST("/pay/callback", wrapF(api.payCallback))
	v1.GET("/member/profile", wrapF(api.memberProfile))
	v1.GET("/member/wallet", wrapF(api.memberWallet))
	v1.GET("/member/token-records", wrapF(api.memberTokenRecords))
	v1.GET("/agent/profile", wrapF(api.agentProfile))
	v1.POST("/agent/join-order", wrapF(api.createAgentJoinOrder))
	v1.GET("/operation-center/profile", wrapF(api.operationCenterProfile))
	v1.GET("/operation-center/agents", wrapF(api.operationCenterAgents))
	v1.GET("/operation-center/orders", wrapF(api.operationCenterOrders))
	v1.GET("/operation-center/commissions", wrapF(api.operationCenterCommissions))
	v1.POST("/operation-center/join-order", wrapF(api.createOperationCenterJoinOrder))
	v1.GET("/points/account", wrapF(api.pointAccount))
	v1.POST("/points/recharge-orders", wrapF(api.createRechargeOrder))
	v1.POST("/points/subscription-orders", wrapF(api.createSubscriptionOrder))
	v1.GET("/user/dashboard", wrapF(api.userDashboard))
	v1.GET("/user/online-image", wrapF(api.userOnlineImage))
	v1.PATCH("/user/ai-state", wrapF(api.updateUserAIState))
	v1.GET("/user/api-settings", wrapF(api.userAPISettings))
	v1.GET("/user/usage", wrapF(api.userUsage))
	v1.GET("/generation-tasks", wrapF(api.listGenerationTasks))
	v1.GET("/generation-tasks/:id", wrapF(api.getGenerationTask))
	v1.POST("/generation-tasks", wrapF(api.createGenerationTask))
	v1.POST("/ppt/generate", wrapF(api.createPPTGenerationTask))
	v1.POST("/ppt/outline/generate", wrapF(api.generatePPTOutline))
	v1.POST("/ppt/outline/save", wrapF(api.savePPTOutline))
	v1.GET("/ppt/tasks/:taskId", wrapF(api.getPPTTask))
	v1.GET("/ppt/history", wrapF(api.listPPTHistory))
	v1.DELETE("/ppt/tasks/:taskId", wrapF(api.deletePPTTask))
	v1.POST("/ppt/slides/:slideId/regenerate", wrapF(api.regeneratePPTSlide))
	v1.POST("/ppt/export/pptx", wrapF(api.exportPPT))
	v1.POST("/ppt/export/pdf", wrapF(api.exportPDF))
	v1.POST("/ppt/images/generate", wrapF(api.generatePPTImage))
	v1.GET("/ppt/images/search", wrapF(api.searchPPTImages))
	v1.GET("/ppt/models/text", wrapF(api.listPPTTextModels))
	v1.GET("/ppt/models/image", wrapF(api.listPPTImageModels))
	v1.GET("/video/download", wrapF(api.downloadVideoByURL))
	v1.GET("/assets", wrapF(api.listAssets))
	v1.POST("/assets/thumbnail-backfill", wrapF(api.backfillAssetThumbnails))
	v1.GET("/assets/:id/download", wrapF(api.downloadAsset))
	v1.DELETE("/assets/:id", wrapF(api.deleteAsset))
	v1.POST("/reference-images", wrapF(api.uploadReferenceImage))
	v1.GET("/reference-images/:name", wrapF(api.serveReferenceImage))
	v1.GET("/generated-media/:name", wrapF(api.serveGeneratedMedia))
	router.GET("/api/module-schema", wrapF(api.moduleSchema))

	pptGroup := router.Group("/api/ppt")
	pptGroup.POST("/generate", wrapF(api.createPPTGenerationTask))
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
			permission := "admin.write"
			if c.Request.Method == http.MethodGet {
				permission = "admin.read"
			}
			pgStore.rbacMiddleware(auth, permission)(c)
		})
	}
	adminGroup.GET("/overview", wrapF(admin.overview))
	adminGroup.GET("/customers", wrapF(admin.customers))
	adminGroup.POST("/customers", wrapF(admin.createCustomer))
	adminGroup.PATCH("/customers/:id", wrapF(admin.updateCustomer))
	adminGroup.POST("/customers/:id/sync-newapi", wrapF(admin.syncCustomerNewAPI))
	adminGroup.GET("/newapi/groups", wrapF(admin.newAPIGroups))
	adminGroup.GET("/channel-agents", wrapF(admin.channelAgents))
	adminGroup.POST("/channel-agents", wrapF(admin.createChannelAgent))
	adminGroup.GET("/channel-agents/tree", wrapF(admin.channelAgentTree))
	adminGroup.PATCH("/channel-agents/:id", wrapF(admin.updateChannelAgent))
	adminGroup.GET("/operation-centers", wrapF(admin.operationCenters))
	adminGroup.GET("/products", wrapF(admin.products))
	adminGroup.PATCH("/products/:id", wrapF(admin.updateProduct))
	adminGroup.GET("/plans", wrapF(admin.plans))
	adminGroup.PATCH("/plans/:id", wrapF(admin.updatePlan))
	adminGroup.GET("/orders", wrapF(admin.orders))
	adminGroup.POST("/orders", wrapF(admin.createOrder))
	adminGroup.POST("/orders/:id/mark-paid", wrapF(admin.markOrderPaid))
	adminGroup.POST("/orders/:id/renew", wrapF(admin.renewOrder))
	adminGroup.GET("/delivery-projects", wrapF(admin.deliveryProjects))
	adminGroup.PATCH("/delivery-projects/:id", wrapF(admin.updateDeliveryProject))
	adminGroup.GET("/generation-tasks", wrapF(admin.generationTasks))
	adminGroup.GET("/ai/overview", wrapF(admin.aiOverview))
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
	adminGroup.GET("/billing/invoices", wrapF(admin.billingInvoices))
	adminGroup.GET("/billing/credit-notes", wrapF(admin.billingCreditNotes))
	adminGroup.GET("/billing/payment-requests", wrapF(admin.billingPaymentRequests))
	adminGroup.GET("/billing/payments", wrapF(admin.billingPayments))

	router.GET("/v1/dashboard/billing/subscription", wrapF(admin.billingSubscription))
	router.GET("/v1/dashboard/billing/usage", wrapF(admin.billingUsage))
	registerWirelessCanvasCompatibilityRoutes(router, cfg)
	router.GET("/login", gin.WrapF(staticIndex(cfg.StaticDir)))
	router.GET("/register", gin.WrapF(staticIndex(cfg.StaticDir)))
	router.GET("/assets/*filepath", gin.WrapH(staticPrefixFiles("/assets/", filepath.Join(cfg.StaticDir, "assets"))))
	router.GET("/admin", wrapF(redirectToAdminSlash))
	router.GET("/admin/*filepath", gin.WrapH(staticPrefixFiles("/admin/", cfg.AdminStaticDir)))
	router.GET("/app", gin.WrapF(staticIndex(cfg.AdminStaticDir)))
	router.GET("/app/*filepath", gin.WrapH(staticPrefixFiles("/app/", cfg.AdminStaticDir)))
	router.GET("/agent", gin.WrapF(staticIndex(cfg.AdminStaticDir)))
	router.GET("/agent/*filepath", gin.WrapH(staticPrefixFiles("/agent/", cfg.AdminStaticDir)))
	router.GET("/user", wrapF(notFound))
	router.GET("/user/*filepath", wrapF(notFound))
	router.NoRoute(gin.WrapF(staticFiles(cfg.AdminStaticDir)))

	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}
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
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func staticIndex(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setStaticCacheHeaders(w, filepath.Join(root, "index.html"), true)
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	}
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
