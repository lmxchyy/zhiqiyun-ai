import { createSSRApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
import { installPermissionRouterGuard } from "./router/permissionGuard";
// #ifndef MP-WEIXIN
import "./styles.css";
// #endif

export function createApp() {
  const app = createSSRApp(App);
  const pinia = createPinia();
  app.use(pinia);
  installPermissionRouterGuard(pinia);
  return { app };
}
