import { createSSRApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
import { installPermissionRouterGuard } from "./router/permissionGuard";
import { miniProgramNavigationStyle, syncMiniProgramNavigation } from "./composables/useMiniProgramNavigation";
// styles.css is the H5/desktop shell stylesheet. Loading it in App Plus also
// applies its html/body/uni-page height and overflow rules to native pages,
// which conflicts with uni-app's single native page scroller.
// #ifdef H5
import "./styles.css";
// #endif

export function createApp() {
  syncMiniProgramNavigation();
  const app = createSSRApp(App);
  const pinia = createPinia();
  app.use(pinia);
  app.mixin({
    computed: {
      miniProgramNavigationStyle() {
        return miniProgramNavigationStyle.value;
      },
    },
  });
  installPermissionRouterGuard(pinia);
  return { app };
}
