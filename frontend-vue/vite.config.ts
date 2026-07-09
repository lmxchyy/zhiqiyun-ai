import path from "node:path";
import { defineConfig } from "vite";
import uniPlugin from "@dcloudio/vite-plugin-uni";

const uni = (uniPlugin as unknown as { default?: typeof uniPlugin }).default || uniPlugin;

export default defineConfig({
  plugins: [uni()],
  resolve: {
    alias: {
      "@xianzhi/api-client": path.resolve(process.cwd(), "../packages/api-client/src"),
      "@xianzhi/business-sdk": path.resolve(process.cwd(), "../packages/business-sdk/src"),
      "@xianzhi/design-token": path.resolve(process.cwd(), "../packages/design-token/src"),
      "@xianzhi/platform-adapter": path.resolve(process.cwd(), "../packages/platform-adapter/src"),
      "@xianzhi/shared-auth": path.resolve(process.cwd(), "../packages/shared-auth/src"),
      "@xianzhi/shared-types": path.resolve(process.cwd(), "../packages/shared-types/src")
    }
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:3100"
    }
  }
});
