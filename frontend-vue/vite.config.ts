import { defineConfig } from "vite";
import uniPlugin from "@dcloudio/vite-plugin-uni";

const uni = (uniPlugin as unknown as { default?: typeof uniPlugin }).default || uniPlugin;

export default defineConfig({
  plugins: [uni()],
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:3100"
    }
  }
});
