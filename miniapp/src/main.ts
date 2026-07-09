import { createSSRApp } from 'vue'
import App from './App.vue'
import { setupInterceptors } from './services/interceptor'
import { pinia } from './stores'
import './styles/index.scss'
import 'virtual:uno.css'

export function createApp() {
  const app = createSSRApp(App)
  app.use(pinia)
  setupInterceptors()
  return {
    app,
  }
}
