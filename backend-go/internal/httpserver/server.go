package httpserver

import (
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
	api := newAPI(store, cfg)
	admin := newAdminAPI(store)
	auth := newAuthAPI(store, sessions)
	channel := newChannelAPI(store, sessions)
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	if pgStore, ok := store.(*postgresStore); ok {
		router.Use(pgStore.auditMiddleware())
	}

	router.GET("/healthz", wrapF(health))
	v1 := router.Group("/api/v1")
	v1.GET("/health", wrapF(health))
	v1.POST("/auth/login", wrapF(auth.login))
	v1.GET("/auth/me", wrapF(auth.me))
	v1.POST("/auth/logout", wrapF(auth.logout))
	v1.POST("/auth/change-password", wrapF(auth.changePassword))
	v1.GET("/channel/me", wrapF(channel.me))
	v1.GET("/models", wrapF(api.models))
	v1.GET("/points/account", wrapF(api.pointAccount))
	v1.GET("/user/dashboard", wrapF(api.userDashboard))
	v1.GET("/user/online-image", wrapF(api.userOnlineImage))
	v1.PATCH("/user/ai-state", wrapF(api.updateUserAIState))
	v1.GET("/user/api-settings", wrapF(api.userAPISettings))
	v1.GET("/user/usage", wrapF(api.userUsage))
	v1.GET("/generation-tasks", wrapF(api.listGenerationTasks))
	v1.POST("/generation-tasks", wrapF(api.createGenerationTask))
	v1.GET("/assets", wrapF(api.listAssets))
	v1.POST("/assets/thumbnail-backfill", wrapF(api.backfillAssetThumbnails))
	v1.GET("/assets/:id/download", wrapF(api.downloadAsset))
	v1.DELETE("/assets/:id", wrapF(api.deleteAsset))

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
	adminGroup.GET("/channel-agents", wrapF(admin.channelAgents))
	adminGroup.POST("/channel-agents", wrapF(admin.createChannelAgent))
	adminGroup.GET("/channel-agents/tree", wrapF(admin.channelAgentTree))
	adminGroup.PATCH("/channel-agents/:id", wrapF(admin.updateChannelAgent))
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
	adminGroup.GET("/usage", wrapF(admin.usage))
	adminGroup.GET("/usage/export", wrapF(admin.exportUsage))
	adminGroup.GET("/commissions", wrapF(admin.commissions))
	adminGroup.POST("/commissions", wrapF(admin.createCommission))
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

	router.GET("/v1/dashboard/billing/subscription", wrapF(admin.billingSubscription))
	router.GET("/v1/dashboard/billing/usage", wrapF(admin.billingUsage))
	router.GET("/admin", wrapF(redirectToAdminSlash))
	router.GET("/admin/*filepath", gin.WrapH(staticPrefixFiles("/admin/", cfg.AdminStaticDir)))
	router.GET("/app", gin.WrapF(staticIndex(cfg.AdminStaticDir)))
	router.GET("/app/*filepath", gin.WrapH(staticPrefixFiles("/app/", cfg.AdminStaticDir)))
	router.GET("/agent", gin.WrapF(staticIndex(cfg.AdminStaticDir)))
	router.GET("/agent/*filepath", gin.WrapH(staticPrefixFiles("/agent/", cfg.AdminStaticDir)))
	router.GET("/user", gin.WrapF(staticIndex(cfg.AdminStaticDir)))
	router.GET("/user/*filepath", gin.WrapH(staticPrefixFiles("/user/", cfg.AdminStaticDir)))
	router.NoRoute(gin.WrapF(staticFiles(cfg.StaticDir)))

	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      180 * time.Second,
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
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	}
}
func staticFiles(root string) http.HandlerFunc {
	fileServer := http.FileServer(http.Dir(root))
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		cleanURLPath := path.Clean("/" + r.URL.Path)
		localPath := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(cleanURLPath, "/")))
		if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	}
}

func staticPrefixFiles(prefix string, root string) http.Handler {
	fileServer := http.StripPrefix(strings.TrimSuffix(prefix, "/"), http.FileServer(http.Dir(root)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		cleanURLPath := path.Clean("/" + strings.TrimPrefix(r.URL.Path, prefix))
		localPath := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(cleanURLPath, "/")))
		if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	})
}
