import { createSSRApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
// #ifndef MP-WEIXIN
import "./styles.css";
// #endif

export function createApp() {
  const app = createSSRApp(App);
  app.use(createPinia());
  return { app };
}
