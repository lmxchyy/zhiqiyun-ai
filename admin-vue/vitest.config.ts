import path from "node:path";
import vue from "@vitejs/plugin-vue";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      "@xianzhi/api-client": path.resolve(process.cwd(), "../packages/api-client/src"),
      "@xianzhi/platform-adapter": path.resolve(process.cwd(), "../packages/platform-adapter/src"),
      "@xianzhi/shared-auth": path.resolve(process.cwd(), "../packages/shared-auth/src"),
      "@xianzhi/shared-image-utils": path.resolve(process.cwd(), "../packages/shared-image-utils/src"),
      "@xianzhi/shared-types": path.resolve(process.cwd(), "../packages/shared-types/src")
    }
  },
  test: {
    environment: "happy-dom",
    include: ["tests/**/*.spec.ts"],
    clearMocks: true
  }
});
