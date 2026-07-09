import { createApp } from "vue";
import { createPinia } from "pinia";
import { ElLoading } from "element-plus/es/components/loading/index";
import "element-plus/es/components/loading/style/css";
import "element-plus/es/components/message/style/css";
import "element-plus/es/components/message-box/style/css";
import "../../packages/design-token/src/tokens.css";
import App from "./App.vue";
import "./styles.css";

createApp(App).use(createPinia()).use(ElLoading).mount("#app");
