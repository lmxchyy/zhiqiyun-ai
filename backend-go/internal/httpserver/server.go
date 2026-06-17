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
	api := newAPI(store)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("GET /api/v1/health", health)
	mux.HandleFunc("GET /api/v1/models", models)
	mux.HandleFunc("GET /api/v1/generation-tasks", api.listGenerationTasks)
	mux.HandleFunc("POST /api/v1/generation-tasks", api.createGenerationTask)
	mux.HandleFunc("GET /api/v1/assets", api.listAssets)
	mux.HandleFunc("DELETE /api/v1/assets/{id}", api.deleteAsset)
	mux.Handle("/", staticFiles(cfg.StaticDir))
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
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
