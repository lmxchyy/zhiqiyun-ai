import { defineConfig, type ViteDevServer } from "vite";
import vue from "@vitejs/plugin-vue";

function appHistoryFallback() {
  return {
    name: "xianzhi-app-history-fallback",
    configureServer(server: ViteDevServer) {
      server.middlewares.use((req, _res, next) => {
        const url = req.url || "";
        if (url === "/app" || url.startsWith("/app/") || url === "/agent" || url.startsWith("/agent/")) {
          req.url = "/admin/";
        }
        next();
      });
    }
  };
}

export default defineConfig({
  plugins: [vue(), appHistoryFallback()],
  base: "/admin/",
  server: {
    proxy: {
      "/api": "http://localhost:3100"
    }
  },
  build: {
    outDir: "dist",
    emptyOutDir: true
  }
});
