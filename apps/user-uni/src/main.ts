import { createSSRApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
import { installPermissionRouterGuard } from "./router/permissionGuard";
// styles.css is the H5/desktop shell stylesheet. Loading it in App Plus also
// applies its html/body/uni-page height and overflow rules to native pages,
// which conflicts with uni-app's single native page scroller.
// #ifdef H5
import "./styles.css";
// #endif

export function createApp() {
  const app = createSSRApp(App);
  const pinia = createPinia();
  app.use(pinia);
  installPermissionRouterGuard(pinia);
  return { app };
}
