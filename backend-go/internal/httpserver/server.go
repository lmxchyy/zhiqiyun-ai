package httpserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

func New(cfg config.Config) *http.Server {
	return newWithStore(cfg, newJSONStore(cfg.DataPath))
}

func newWithStore(cfg config.Config, store platformStore) *http.Server {
	api := newAPI(store, cfg)
	admin := newAdminAPI(store)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("GET /api/v1/health", health)
	mux.HandleFunc("GET /api/v1/models", models)
	mux.HandleFunc("GET /api/v1/points/account", api.pointAccount)
	mux.HandleFunc("GET /api/v1/generation-tasks", api.listGenerationTasks)
	mux.HandleFunc("POST /api/v1/generation-tasks", api.createGenerationTask)
	mux.HandleFunc("GET /api/v1/assets", api.listAssets)
	mux.HandleFunc("DELETE /api/v1/assets/{id}", api.deleteAsset)
	mux.HandleFunc("GET /api/v1/admin/overview", admin.overview)
	mux.HandleFunc("GET /api/v1/admin/customers", admin.customers)
	mux.HandleFunc("POST /api/v1/admin/customers", admin.createCustomer)
	mux.HandleFunc("PATCH /api/v1/admin/customers/{id}", admin.updateCustomer)
	mux.HandleFunc("GET /api/v1/admin/channel-agents", admin.channelAgents)
	mux.HandleFunc("GET /api/v1/admin/channel-agents/tree", admin.channelAgentTree)
	mux.HandleFunc("PATCH /api/v1/admin/channel-agents/{id}", admin.updateChannelAgent)
	mux.HandleFunc("GET /api/v1/admin/products", admin.products)
	mux.HandleFunc("PATCH /api/v1/admin/products/{id}", admin.updateProduct)
	mux.HandleFunc("GET /api/v1/admin/plans", admin.plans)
	mux.HandleFunc("PATCH /api/v1/admin/plans/{id}", admin.updatePlan)
	mux.HandleFunc("GET /api/v1/admin/orders", admin.orders)
	mux.HandleFunc("POST /api/v1/admin/orders", admin.createOrder)
	mux.HandleFunc("POST /api/v1/admin/orders/{id}/mark-paid", admin.markOrderPaid)
	mux.HandleFunc("POST /api/v1/admin/orders/{id}/renew", admin.renewOrder)
	mux.HandleFunc("GET /api/v1/admin/delivery-projects", admin.deliveryProjects)
	mux.HandleFunc("PATCH /api/v1/admin/delivery-projects/{id}", admin.updateDeliveryProject)
	mux.HandleFunc("GET /api/v1/admin/usage", admin.usage)
	mux.HandleFunc("GET /api/v1/admin/usage/export", admin.exportUsage)
	mux.HandleFunc("GET /api/v1/admin/commissions", admin.commissions)
	mux.HandleFunc("POST /api/v1/admin/commissions", admin.createCommission)
	mux.HandleFunc("POST /api/v1/admin/withdrawals", admin.createWithdrawal)
	mux.HandleFunc("POST /api/v1/admin/withdrawals/{id}/approve", admin.approveWithdrawal)
	mux.HandleFunc("POST /api/v1/admin/withdrawals/{id}/reject", admin.rejectWithdrawal)
	mux.HandleFunc("GET /api/v1/admin/system/settings", admin.systemSettings)
	mux.HandleFunc("PATCH /api/v1/admin/system/settings", admin.updateSystemSettings)
	mux.HandleFunc("GET /api/v1/admin/api/provider-channels", admin.apiProviderChannels)
	mux.HandleFunc("POST /api/v1/admin/api/provider-channels", admin.createAPIProviderChannel)
	mux.HandleFunc("PATCH /api/v1/admin/api/provider-channels/{id}", admin.updateAPIProviderChannel)
	mux.HandleFunc("POST /api/v1/admin/api/provider-channels/{id}/test", admin.testAPIProviderChannel)
	mux.HandleFunc("GET /api/v1/admin/api/models", admin.apiModels)
	mux.HandleFunc("PATCH /api/v1/admin/api/models/{id}", admin.updateAPIModel)
	mux.HandleFunc("GET /api/v1/admin/api/keys", admin.apiKeys)
	mux.HandleFunc("POST /api/v1/admin/api/keys", admin.createAPIKey)
	mux.HandleFunc("PATCH /api/v1/admin/api/keys/{id}", admin.updateAPIKey)
	mux.HandleFunc("GET /api/v1/admin/customer-groups", admin.customerGroups)
	mux.HandleFunc("PATCH /api/v1/admin/customer-groups/{id}", admin.updateCustomerGroup)
	mux.HandleFunc("GET /v1/dashboard/billing/subscription", admin.billingSubscription)
	mux.HandleFunc("GET /v1/dashboard/billing/usage", admin.billingUsage)
	mux.HandleFunc("GET /admin", redirectToAdminSlash)
	mux.Handle("/admin/", staticPrefixFiles("/admin/", cfg.AdminStaticDir))
	mux.Handle("/", staticFiles(cfg.StaticDir))
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      180 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func redirectToAdminSlash(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "xianzhi-ai-go",
	})
}

func models(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, []map[string]any{
		{"code": "mock-standard", "name": "本地演示模型", "capabilities": []string{"TEXT_TO_IMAGE"}, "online": true},
		{"code": "gpt-image-2", "name": "OpenAI 图像模型", "capabilities": []string{"TEXT_TO_IMAGE", "IMAGE_TO_IMAGE"}, "online": true},
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

func staticFiles(root string) http.HandlerFunc {
	fileServer := http.FileServer(http.Dir(root))
	return func(w http.ResponseWriter, r *http.Request) {
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
		cleanURLPath := path.Clean("/" + strings.TrimPrefix(r.URL.Path, prefix))
		localPath := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(cleanURLPath, "/")))
		if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	})
}
