/// <reference types="@dcloudio/types" />

interface ImportMetaEnv {
  readonly VITE_APP_TITLE: string
  readonly VITE_APP_PORT: string
  readonly VITE_SERVER_BASEURL: string
  readonly VITE_APP_PROXY_ENABLE: string
  readonly VITE_APP_PROXY_PREFIX: string
  readonly VITE_APP_PUBLIC_BASE: string
  readonly VITE_USE_MOCK: string
  readonly VITE_WX_APPID: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

declare function definePage(page: Record<string, unknown>): void

declare module '*.vue' {
  import type { DefineComponent } from 'vue'

  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, unknown>
  export default component
}

declare module '*.png' {
  const src: string
  export default src
}
