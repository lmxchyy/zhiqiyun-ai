import { defineConfig, type ViteDevServer } from "vite";
import vue from "@vitejs/plugin-vue";
import Components from "unplugin-vue-components/vite";
import { ElementPlusResolver } from "unplugin-vue-components/resolvers";

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
  plugins: [
    vue(),
    Components({
      dts: false,
      resolvers: [
        ElementPlusResolver({
          importStyle: "css"
        })
      ]
    }),
    appHistoryFallback()
  ],
  base: "/admin/",
  server: {
    proxy: {
      "/api": "http://localhost:3100"
    }
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks(id) {
          const normalizedId = id.replace(/\\/g, "/");
          if (normalizedId.includes("node_modules")) {
            if (normalizedId.includes("@element-plus/icons-vue")) return "vendor-icons";
            if (normalizedId.includes("node_modules/element-plus/es/components/")) {
              const componentName = normalizedId.split("node_modules/element-plus/es/components/")[1]?.split("/")[0];
              if (componentName) return `vendor-el-${componentName}`;
            }
            if (normalizedId.includes("node_modules/element-plus")) return "vendor-element-plus";
            if (normalizedId.includes("axios")) return "vendor-http";
            if (normalizedId.includes("pinia") || normalizedId.includes("@vue") || normalizedId.includes("/vue/") || normalizedId.includes("@vueuse")) {
              return "vendor-vue";
            }
            return "vendor";
          }
          if (normalizedId.includes("/src/components/ppt/") || normalizedId.includes("/src/stores/ppt") || normalizedId.includes("/src/api/ppt")) {
            return "admin-ppt";
          }
        }
      }
    }
  }
});
