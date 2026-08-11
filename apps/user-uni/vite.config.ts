import path from "node:path";
import { defineConfig } from "vite";
import uniPlugin from "@dcloudio/vite-plugin-uni";

const uni = (uniPlugin as unknown as { default?: typeof uniPlugin }).default || uniPlugin;

export default defineConfig({
  base: process.env.UNI_PLATFORM === "h5" ? "/h5/" : undefined,
  plugins: [uni()],
  resolve: {
    alias: {
      "@xianzhi/api-client": path.resolve(process.cwd(), "../../packages/api-client/src"),
      "@xianzhi/business-sdk": path.resolve(process.cwd(), "../../packages/business-sdk/src"),
      "@xianzhi/design-token": path.resolve(process.cwd(), "../../packages/design-token/src"),
      "@xianzhi/platform-adapter": path.resolve(process.cwd(), "../../packages/platform-adapter/src"),
      "@xianzhi/shared-auth": path.resolve(process.cwd(), "../../packages/shared-auth/src"),
      "@xianzhi/shared-types": path.resolve(process.cwd(), "../../packages/shared-types/src")
    }
  },
  server: {
    host: "127.0.0.1",
    port: (() => {
      const argvPort = process.argv.find((arg, i, arr) => arg === "--port" && arr[i + 1]) 
        ? Number(process.argv[process.argv.indexOf("--port") + 1])
        : Number(process.argv.find(arg => /^--port=\d+$/.test(arg))?.split("=")[1] || NaN);
      return Number(process.env.USER_UNI_DEV_PORT || process.env.PORT || argvPort || 5173);
    })(),
    strictPort: true,
    proxy: {
      "/api": "http://127.0.0.1:3100",
      "/h5/api": {
        target: "http://127.0.0.1:3100",
        rewrite: (p) => p.replace(/^\/h5/, ""),
      },
    },
  }
});
