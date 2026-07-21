# 知启云AI企业级AI创作与智能体平台 V1.0 源程序鉴别材料

> 编排口径：前 30 页为用户端与管理端核心源码，后 30 页为 Go 服务端与 PostgreSQL 数据层核心源码；每页 50 个原始逻辑行，共 3,000 行。代码保持原始内容，只增加相对路径、原始行号、页码和模块说明。

## 打印说明

- 建议使用 A4 纸、单面打印、等宽字体 8—9 磅。
- 页眉统一为“知启云AI企业级AI创作与智能体平台 V1.0 源程序鉴别材料”。
- 每页保留 50 个带原始行号的代码逻辑行；Markdown 标题和说明不计入代码行数。
- 正式提交前由申请人确认代码权属、开发完成日期和版本冻结点。
- 若登记机构要求连续源程序的最前/最后 30 页，应基于冻结版本全量代码另行导出，本材料采用代表性核心代码编排。

## 前 30 页：客户端与管理端核心代码

### 第 01 页

**文件路径：** apps/user-uni/src/main.ts；apps/user-uni/src/api/client.ts  
**代码说明：** 用户端应用启动、Pinia 注册与跨端挂载。；统一 API Client、认证头、租户上下文、刷新令牌与文件传输。

```text
apps/user-uni/src/main.ts:0001 | import { createSSRApp } from "vue";
apps/user-uni/src/main.ts:0002 | import { createPinia } from "pinia";
apps/user-uni/src/main.ts:0003 | import App from "./App.vue";
apps/user-uni/src/main.ts:0004 | import { installPermissionRouterGuard } from "./router/permissionGuard";
apps/user-uni/src/main.ts:0005 | // styles.css is the H5/desktop shell stylesheet. Loading it in App Plus also
apps/user-uni/src/main.ts:0006 | // applies its html/body/uni-page height and overflow rules to native pages,
apps/user-uni/src/main.ts:0007 | // which conflicts with uni-app's single native page scroller.
apps/user-uni/src/main.ts:0008 | // #ifdef H5
apps/user-uni/src/main.ts:0009 | import "./styles.css";
apps/user-uni/src/main.ts:0010 | // #endif
apps/user-uni/src/main.ts:0011 | 
apps/user-uni/src/main.ts:0012 | export function createApp() {
apps/user-uni/src/main.ts:0013 |   const app = createSSRApp(App);
apps/user-uni/src/main.ts:0014 |   const pinia = createPinia();
apps/user-uni/src/main.ts:0015 |   app.use(pinia);
apps/user-uni/src/main.ts:0016 |   installPermissionRouterGuard(pinia);
apps/user-uni/src/main.ts:0017 |   return { app };
apps/user-uni/src/main.ts:0018 | }
apps/user-uni/src/api/client.ts:0001 | import { configureApiClient } from '@xianzhi/api-client'
apps/user-uni/src/api/client.ts:0002 | import { createBusinessSdk } from '@xianzhi/business-sdk'
apps/user-uni/src/api/client.ts:0003 | import { createUniPlatformAdapter, type AdapterDownloadFileResponse } from '@xianzhi/platform-adapter'
apps/user-uni/src/api/client.ts:0004 | import { createAuthService, createAuthStorage } from '@xianzhi/shared-auth'
apps/user-uni/src/api/client.ts:0005 | 
apps/user-uni/src/api/client.ts:0006 | const tokenKey = 'token'
apps/user-uni/src/api/client.ts:0007 | const refreshTokenKey = 'refreshToken'
apps/user-uni/src/api/client.ts:0008 | const authKey = 'auth'
apps/user-uni/src/api/client.ts:0009 | const requestTimeout = 600000
apps/user-uni/src/api/client.ts:0010 | const rawEnv = (import.meta as unknown as { env?: Record<string, string | boolean | undefined> }).env || {}
apps/user-uni/src/api/client.ts:0011 | const env = rawEnv as Record<string, string | undefined>
apps/user-uni/src/api/client.ts:0012 | const adapter = createUniPlatformAdapter()
apps/user-uni/src/api/client.ts:0013 | let defaultApiBaseURL = ''
apps/user-uni/src/api/client.ts:0014 | // #ifdef MP-WEIXIN
apps/user-uni/src/api/client.ts:0015 | defaultApiBaseURL = 'https://ai.zs-kjhn.cn'
apps/user-uni/src/api/client.ts:0016 | // #endif
apps/user-uni/src/api/client.ts:0017 | const apiBaseURL = String(env.VITE_API_BASE_URL || defaultApiBaseURL).replace(/\/+$/, '')
apps/user-uni/src/api/client.ts:0018 | let unauthorizedRedirecting = false
apps/user-uni/src/api/client.ts:0019 | let unauthorizedRefreshPromise: Promise<boolean> | null = null
apps/user-uni/src/api/client.ts:0020 | 
apps/user-uni/src/api/client.ts:0021 | export const authStorage = createAuthStorage({
apps/user-uni/src/api/client.ts:0022 |   adapter,
apps/user-uni/src/api/client.ts:0023 |   tokenKey,
apps/user-uni/src/api/client.ts:0024 |   refreshTokenKey,
apps/user-uni/src/api/client.ts:0025 |   authKey,
apps/user-uni/src/api/client.ts:0026 | })
apps/user-uni/src/api/client.ts:0027 | 
apps/user-uni/src/api/client.ts:0028 | export function getApiBaseURL(): string {
apps/user-uni/src/api/client.ts:0029 |   return sharedApiClient.getBaseURL()
apps/user-uni/src/api/client.ts:0030 | }
apps/user-uni/src/api/client.ts:0031 | 
apps/user-uni/src/api/client.ts:0032 | export function getAuthToken(): string {
```

<div style="page-break-after: always;"></div>

### 第 02 页

**文件路径：** apps/user-uni/src/api/client.ts  
**代码说明：** 统一 API Client、认证头、租户上下文、刷新令牌与文件传输。

```typescript
apps/user-uni/src/api/client.ts:0033 |   return authStorage.getToken()
apps/user-uni/src/api/client.ts:0034 | }
apps/user-uni/src/api/client.ts:0035 | 
apps/user-uni/src/api/client.ts:0036 | export function setAuthToken(token: string) {
apps/user-uni/src/api/client.ts:0037 |   authStorage.setToken(token)
apps/user-uni/src/api/client.ts:0038 | }
apps/user-uni/src/api/client.ts:0039 | 
apps/user-uni/src/api/client.ts:0040 | function createRequestId() {
apps/user-uni/src/api/client.ts:0041 |   const cryptoRuntime = globalThis.crypto
apps/user-uni/src/api/client.ts:0042 |   if (cryptoRuntime?.randomUUID) return cryptoRuntime.randomUUID()
apps/user-uni/src/api/client.ts:0043 |   return `req_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 10)}`
apps/user-uni/src/api/client.ts:0044 | }
apps/user-uni/src/api/client.ts:0045 | 
apps/user-uni/src/api/client.ts:0046 | function currentContextHeaders() {
apps/user-uni/src/api/client.ts:0047 |   const auth = authStorage.getAuth()
apps/user-uni/src/api/client.ts:0048 |   const headers: Record<string, string> = {}
apps/user-uni/src/api/client.ts:0049 |   if (auth?.tenantId) headers['X-Tenant-Id'] = auth.tenantId
apps/user-uni/src/api/client.ts:0050 |   if (auth?.organizationId) headers['X-Organization-Id'] = auth.organizationId
apps/user-uni/src/api/client.ts:0051 |   return headers
apps/user-uni/src/api/client.ts:0052 | }
apps/user-uni/src/api/client.ts:0053 | 
apps/user-uni/src/api/client.ts:0054 | function clientTransportHeaders(headers: Record<string, string> = {}, auth = true) {
apps/user-uni/src/api/client.ts:0055 |   const clientInfo = adapter.getClientInfo()
apps/user-uni/src/api/client.ts:0056 |   const result: Record<string, string> = {
apps/user-uni/src/api/client.ts:0057 |     Accept: 'application/json',
apps/user-uni/src/api/client.ts:0058 |     'X-Request-Id': createRequestId(),
apps/user-uni/src/api/client.ts:0059 |     'X-Client-Platform': clientInfo.platform,
apps/user-uni/src/api/client.ts:0060 |     ...currentContextHeaders(),
apps/user-uni/src/api/client.ts:0061 |     ...headers,
apps/user-uni/src/api/client.ts:0062 |   }
apps/user-uni/src/api/client.ts:0063 |   if (clientInfo.appName) result['X-Client-Name'] = clientInfo.appName
apps/user-uni/src/api/client.ts:0064 |   if (env.VITE_APP_VERSION || env.VITE_APP_BUILD_VERSION) result['X-Client-Version'] = env.VITE_APP_VERSION || env.VITE_APP_BUILD_VERSION || ''
apps/user-uni/src/api/client.ts:0065 |   if (clientInfo.language) result['X-Client-Language'] = clientInfo.language
apps/user-uni/src/api/client.ts:0066 |   const token = auth ? getAuthToken() : ''
apps/user-uni/src/api/client.ts:0067 |   if (token && !result.Authorization) result.Authorization = `Bearer ${token}`
apps/user-uni/src/api/client.ts:0068 |   return result
apps/user-uni/src/api/client.ts:0069 | }
apps/user-uni/src/api/client.ts:0070 | 
apps/user-uni/src/api/client.ts:0071 | function resolveApiURL(path: string) {
apps/user-uni/src/api/client.ts:0072 |   if (/^https?:\/\//i.test(path)) return path
apps/user-uni/src/api/client.ts:0073 |   const baseURL = getApiBaseURL().replace(/\/+$/, '')
apps/user-uni/src/api/client.ts:0074 |   return `${baseURL}${path.startsWith('/') ? path : `/${path}`}`
apps/user-uni/src/api/client.ts:0075 | }
apps/user-uni/src/api/client.ts:0076 | 
apps/user-uni/src/api/client.ts:0077 | function trustedApiURL(path: string) {
apps/user-uni/src/api/client.ts:0078 |   if (!/^https?:\/\//i.test(path)) return true
apps/user-uni/src/api/client.ts:0079 |   const targetOrigin = path.match(/^https?:\/\/[^/]+/i)?.[0].toLowerCase() || ''
apps/user-uni/src/api/client.ts:0080 |   const baseOrigin = getApiBaseURL().match(/^https?:\/\/[^/]+/i)?.[0].toLowerCase() || ''
apps/user-uni/src/api/client.ts:0081 |   if (baseOrigin) return targetOrigin === baseOrigin
apps/user-uni/src/api/client.ts:0082 |   if (typeof window !== 'undefined') return targetOrigin === window.location.origin.toLowerCase()
```

<div style="page-break-after: always;"></div>

### 第 03 页

**文件路径：** apps/user-uni/src/api/client.ts  
**代码说明：** 统一 API Client、认证头、租户上下文、刷新令牌与文件传输。

```typescript
apps/user-uni/src/api/client.ts:0083 |   return false
apps/user-uni/src/api/client.ts:0084 | }
apps/user-uni/src/api/client.ts:0085 | 
apps/user-uni/src/api/client.ts:0086 | function responsePayloadMessage(payload: unknown, fallback: string) {
apps/user-uni/src/api/client.ts:0087 |   if (payload && typeof payload === 'object') {
apps/user-uni/src/api/client.ts:0088 |     const record = payload as { error?: unknown; message?: unknown }
apps/user-uni/src/api/client.ts:0089 |     return String(record.error || record.message || fallback)
apps/user-uni/src/api/client.ts:0090 |   }
apps/user-uni/src/api/client.ts:0091 |   return typeof payload === 'string' && payload.trim() ? payload.trim() : fallback
apps/user-uni/src/api/client.ts:0092 | }
apps/user-uni/src/api/client.ts:0093 | 
apps/user-uni/src/api/client.ts:0094 | function unwrapTransportPayload<T>(payload: unknown) {
apps/user-uni/src/api/client.ts:0095 |   if (!payload || typeof payload !== 'object') return payload as T
apps/user-uni/src/api/client.ts:0096 |   const record = payload as { code?: unknown; data?: T; error?: unknown; message?: unknown }
apps/user-uni/src/api/client.ts:0097 |   const hasEnvelopeShape = 'code' in record || 'message' in record || 'error' in record
apps/user-uni/src/api/client.ts:0098 |   if (hasEnvelopeShape && record.code !== undefined && record.code !== 0 && record.code !== '0') {
apps/user-uni/src/api/client.ts:0099 |     throw new Error(responsePayloadMessage(record, `API code ${String(record.code)}`))
apps/user-uni/src/api/client.ts:0100 |   }
apps/user-uni/src/api/client.ts:0101 |   if (hasEnvelopeShape && Object.prototype.hasOwnProperty.call(record, 'data')) return record.data as T
apps/user-uni/src/api/client.ts:0102 |   return payload as T
apps/user-uni/src/api/client.ts:0103 | }
apps/user-uni/src/api/client.ts:0104 | 
apps/user-uni/src/api/client.ts:0105 | async function fetchErrorMessage(response: Response) {
apps/user-uni/src/api/client.ts:0106 |   const fallback = `请求失败 (${response.status})`
apps/user-uni/src/api/client.ts:0107 |   const raw = await response.text().catch(() => '')
apps/user-uni/src/api/client.ts:0108 |   if (!raw) return fallback
apps/user-uni/src/api/client.ts:0109 |   try {
apps/user-uni/src/api/client.ts:0110 |     return responsePayloadMessage(JSON.parse(raw), fallback)
apps/user-uni/src/api/client.ts:0111 |   }
apps/user-uni/src/api/client.ts:0112 |   catch {
apps/user-uni/src/api/client.ts:0113 |     return raw.trim() || fallback
apps/user-uni/src/api/client.ts:0114 |   }
apps/user-uni/src/api/client.ts:0115 | }
apps/user-uni/src/api/client.ts:0116 | 
apps/user-uni/src/api/client.ts:0117 | function normalizeHeaders(headers: RequestInit['headers']): Record<string, string> {
apps/user-uni/src/api/client.ts:0118 |   if (!headers) return {}
apps/user-uni/src/api/client.ts:0119 |   if (typeof Headers !== "undefined" && headers instanceof Headers) return Object.fromEntries(headers.entries())
apps/user-uni/src/api/client.ts:0120 |   if (Array.isArray(headers)) return Object.fromEntries(headers)
apps/user-uni/src/api/client.ts:0121 |   return headers as Record<string, string>
apps/user-uni/src/api/client.ts:0122 | }
apps/user-uni/src/api/client.ts:0123 | 
apps/user-uni/src/api/client.ts:0124 | function normalizeBody(body: RequestInit['body']) {
apps/user-uni/src/api/client.ts:0125 |   if (body === undefined || body === null) return undefined
apps/user-uni/src/api/client.ts:0126 |   if (typeof body !== 'string') return body
apps/user-uni/src/api/client.ts:0127 |   if (!body.trim()) return undefined
apps/user-uni/src/api/client.ts:0128 |   try {
apps/user-uni/src/api/client.ts:0129 |     return JSON.parse(body)
apps/user-uni/src/api/client.ts:0130 |   }
apps/user-uni/src/api/client.ts:0131 |   catch {
apps/user-uni/src/api/client.ts:0132 |     return body
```

<div style="page-break-after: always;"></div>

### 第 04 页

**文件路径：** apps/user-uni/src/api/client.ts  
**代码说明：** 统一 API Client、认证头、租户上下文、刷新令牌与文件传输。

```typescript
apps/user-uni/src/api/client.ts:0133 |   }
apps/user-uni/src/api/client.ts:0134 | }
apps/user-uni/src/api/client.ts:0135 | 
apps/user-uni/src/api/client.ts:0136 | function redirectToLogin() {
apps/user-uni/src/api/client.ts:0137 |   if (unauthorizedRedirecting) return
apps/user-uni/src/api/client.ts:0138 |   unauthorizedRedirecting = true
apps/user-uni/src/api/client.ts:0139 |   // #ifdef MP-WEIXIN || APP-PLUS
apps/user-uni/src/api/client.ts:0140 |   const pages = typeof getCurrentPages === 'function' ? getCurrentPages() : []
apps/user-uni/src/api/client.ts:0141 |   const current = pages.length ? pages[pages.length - 1] as unknown as { route?: string; options?: Record<string, unknown> } : null
apps/user-uni/src/api/client.ts:0142 |   const currentPath = current?.route ? `/${String(current.route).replace(/^\/+/, '')}` : ''
apps/user-uni/src/api/client.ts:0143 |   const query = current?.options
apps/user-uni/src/api/client.ts:0144 |     ? Object.entries(current.options).map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(String(value ?? ''))}`).join('&')
apps/user-uni/src/api/client.ts:0145 |     : ''
apps/user-uni/src/api/client.ts:0146 |   const loginQuery = currentPath && currentPath !== '/pages/WechatLoginPage'
apps/user-uni/src/api/client.ts:0147 |     ? `?redirectPath=${encodeURIComponent(currentPath)}&redirectQuery=${encodeURIComponent(query)}&sourcePage=${encodeURIComponent(currentPath)}`
apps/user-uni/src/api/client.ts:0148 |     : ''
apps/user-uni/src/api/client.ts:0149 |   uni.navigateTo({
apps/user-uni/src/api/client.ts:0150 |     url: `/pages/WechatLoginPage${loginQuery}`,
apps/user-uni/src/api/client.ts:0151 |     complete: () => { unauthorizedRedirecting = false },
apps/user-uni/src/api/client.ts:0152 |     fail: () => uni.redirectTo({ url: `/pages/WechatLoginPage${loginQuery}` }),
apps/user-uni/src/api/client.ts:0153 |   })
apps/user-uni/src/api/client.ts:0154 |   // #endif
apps/user-uni/src/api/client.ts:0155 |   // #ifdef H5
apps/user-uni/src/api/client.ts:0156 |   if (typeof window !== 'undefined' && !['/login', '/register'].includes(window.location.pathname)) {
apps/user-uni/src/api/client.ts:0157 |     window.history.pushState({ authRequired: true }, '', `/login?redirect=${encodeURIComponent(window.location.pathname + window.location.search + window.location.hash)}`)
apps/user-uni/src/api/client.ts:0158 |     window.dispatchEvent(new PopStateEvent('popstate'))
apps/user-uni/src/api/client.ts:0159 |   }
apps/user-uni/src/api/client.ts:0160 |   unauthorizedRedirecting = false
apps/user-uni/src/api/client.ts:0161 |   // #endif
apps/user-uni/src/api/client.ts:0162 | }
apps/user-uni/src/api/client.ts:0163 | 
apps/user-uni/src/api/client.ts:0164 | async function handleUnauthorized(context: { path?: string; statusCode?: number; requestId?: string; payload?: unknown; retryAttempt?: number; authMode?: "none" | "optional" | "required" } = {}) {
apps/user-uni/src/api/client.ts:0165 |   const shouldOpenLogin = context.authMode === "required"
apps/user-uni/src/api/client.ts:0166 |   if ((context.retryAttempt || 0) > 0) {
apps/user-uni/src/api/client.ts:0167 |     authStorage.clear()
apps/user-uni/src/api/client.ts:0168 |     if (shouldOpenLogin) redirectToLogin()
apps/user-uni/src/api/client.ts:0169 |     return false
apps/user-uni/src/api/client.ts:0170 |   }
apps/user-uni/src/api/client.ts:0171 |   if (!authStorage.getRefreshToken()) {
apps/user-uni/src/api/client.ts:0172 |     authStorage.clear()
apps/user-uni/src/api/client.ts:0173 |     if (shouldOpenLogin) redirectToLogin()
apps/user-uni/src/api/client.ts:0174 |     return false
apps/user-uni/src/api/client.ts:0175 |   }
apps/user-uni/src/api/client.ts:0176 |   if (!unauthorizedRefreshPromise) {
apps/user-uni/src/api/client.ts:0177 |     unauthorizedRefreshPromise = authService.refresh()
apps/user-uni/src/api/client.ts:0178 |       .then(() => true)
apps/user-uni/src/api/client.ts:0179 |       .catch(() => {
apps/user-uni/src/api/client.ts:0180 |         authStorage.clear()
apps/user-uni/src/api/client.ts:0181 |         return false
apps/user-uni/src/api/client.ts:0182 |       })
```

<div style="page-break-after: always;"></div>

### 第 05 页

**文件路径：** apps/user-uni/src/api/client.ts  
**代码说明：** 统一 API Client、认证头、租户上下文、刷新令牌与文件传输。

```typescript
apps/user-uni/src/api/client.ts:0183 |       .finally(() => {
apps/user-uni/src/api/client.ts:0184 |         unauthorizedRefreshPromise = null
apps/user-uni/src/api/client.ts:0185 |       })
apps/user-uni/src/api/client.ts:0186 |   }
apps/user-uni/src/api/client.ts:0187 |   const recovered = await unauthorizedRefreshPromise
apps/user-uni/src/api/client.ts:0188 |   if (!recovered && shouldOpenLogin) redirectToLogin()
apps/user-uni/src/api/client.ts:0189 |   return recovered
apps/user-uni/src/api/client.ts:0190 | }
apps/user-uni/src/api/client.ts:0191 | 
apps/user-uni/src/api/client.ts:0192 | const detectedClientPlatform = adapter.getClientInfo().platform
apps/user-uni/src/api/client.ts:0193 | const clientName = detectedClientPlatform === 'app-android' || detectedClientPlatform === 'app-ios'
apps/user-uni/src/api/client.ts:0194 |   ? 'xianzhi-user-app'
apps/user-uni/src/api/client.ts:0195 |   : detectedClientPlatform === 'mp-weixin'
apps/user-uni/src/api/client.ts:0196 |     ? 'xianzhi-mini-program'
apps/user-uni/src/api/client.ts:0197 |   : 'xianzhi-user-web'
apps/user-uni/src/api/client.ts:0198 | 
apps/user-uni/src/api/client.ts:0199 | const sharedApiClient = configureApiClient({
apps/user-uni/src/api/client.ts:0200 |   adapter,
apps/user-uni/src/api/client.ts:0201 |   baseURL: apiBaseURL,
apps/user-uni/src/api/client.ts:0202 |   timeout: requestTimeout,
apps/user-uni/src/api/client.ts:0203 |   clientName,
apps/user-uni/src/api/client.ts:0204 |   clientVersion: env.VITE_APP_VERSION || env.VITE_APP_BUILD_VERSION || '0.1.0',
apps/user-uni/src/api/client.ts:0205 |   getToken: getAuthToken,
apps/user-uni/src/api/client.ts:0206 |   defaultHeaders: currentContextHeaders,
apps/user-uni/src/api/client.ts:0207 |   onUnauthorized: handleUnauthorized,
apps/user-uni/src/api/client.ts:0208 | })
apps/user-uni/src/api/client.ts:0209 | 
apps/user-uni/src/api/client.ts:0210 | export const apiClient = sharedApiClient
apps/user-uni/src/api/client.ts:0211 | 
apps/user-uni/src/api/client.ts:0212 | export const authService = createAuthService({
apps/user-uni/src/api/client.ts:0213 |   adapter,
apps/user-uni/src/api/client.ts:0214 |   api: apiClient,
apps/user-uni/src/api/client.ts:0215 |   tokenKey,
apps/user-uni/src/api/client.ts:0216 |   refreshTokenKey,
apps/user-uni/src/api/client.ts:0217 |   authKey,
apps/user-uni/src/api/client.ts:0218 | })
apps/user-uni/src/api/client.ts:0219 | 
apps/user-uni/src/api/client.ts:0220 | export const businessSdk = createBusinessSdk(apiClient)
apps/user-uni/src/api/client.ts:0221 | 
apps/user-uni/src/api/client.ts:0222 | export async function apiFetchResponse(
apps/user-uni/src/api/client.ts:0223 |   path: string,
apps/user-uni/src/api/client.ts:0224 |   init: RequestInit = {},
apps/user-uni/src/api/client.ts:0225 |   options: { auth?: boolean; retriedAfterRefresh?: boolean } = {},
apps/user-uni/src/api/client.ts:0226 | ) {
apps/user-uni/src/api/client.ts:0227 |   if (typeof fetch !== 'function') throw new Error('当前运行环境不支持该请求方式')
apps/user-uni/src/api/client.ts:0228 |   const usesSession = options.auth ?? trustedApiURL(path)
apps/user-uni/src/api/client.ts:0229 |   const headers = clientTransportHeaders(normalizeHeaders(init.headers), usesSession)
apps/user-uni/src/api/client.ts:0230 |   const response = await fetch(resolveApiURL(path), { ...init, headers })
apps/user-uni/src/api/client.ts:0231 |   if (usesSession && response.status === 401) {
apps/user-uni/src/api/client.ts:0232 |     const retryAttempt = options.retriedAfterRefresh ? 1 : 0
```

<div style="page-break-after: always;"></div>

### 第 06 页

**文件路径：** apps/user-uni/src/api/client.ts  
**代码说明：** 统一 API Client、认证头、租户上下文、刷新令牌与文件传输。

```typescript
apps/user-uni/src/api/client.ts:0233 |     const recovered = await handleUnauthorized({ path, statusCode: response.status, payload: null, retryAttempt })
apps/user-uni/src/api/client.ts:0234 |     if (recovered && retryAttempt === 0) return apiFetchResponse(path, init, { ...options, retriedAfterRefresh: true })
apps/user-uni/src/api/client.ts:0235 |   }
apps/user-uni/src/api/client.ts:0236 |   if (!response.ok) throw new Error(await fetchErrorMessage(response))
apps/user-uni/src/api/client.ts:0237 |   return response
apps/user-uni/src/api/client.ts:0238 | }
apps/user-uni/src/api/client.ts:0239 | 
apps/user-uni/src/api/client.ts:0240 | export interface ApiRequestTaskHandle<T> {
apps/user-uni/src/api/client.ts:0241 |   promise: Promise<T>
apps/user-uni/src/api/client.ts:0242 |   abort: () => void
apps/user-uni/src/api/client.ts:0243 | }
apps/user-uni/src/api/client.ts:0244 | 
apps/user-uni/src/api/client.ts:0245 | export interface ApiRequestTaskOptions<TBody = unknown> {
apps/user-uni/src/api/client.ts:0246 |   method?: string
apps/user-uni/src/api/client.ts:0247 |   headers?: Record<string, string>
apps/user-uni/src/api/client.ts:0248 |   data?: TBody
apps/user-uni/src/api/client.ts:0249 |   timeout?: number
apps/user-uni/src/api/client.ts:0250 |   auth?: boolean
apps/user-uni/src/api/client.ts:0251 | }
apps/user-uni/src/api/client.ts:0252 | 
apps/user-uni/src/api/client.ts:0253 | export function apiRequestTask<T = unknown, TBody = unknown>(
apps/user-uni/src/api/client.ts:0254 |   path: string,
apps/user-uni/src/api/client.ts:0255 |   options: ApiRequestTaskOptions<TBody> = {},
apps/user-uni/src/api/client.ts:0256 | ): ApiRequestTaskHandle<T> {
apps/user-uni/src/api/client.ts:0257 |   let requestTask: UniApp.RequestTask | null = null
apps/user-uni/src/api/client.ts:0258 |   const usesSession = options.auth ?? trustedApiURL(path)
apps/user-uni/src/api/client.ts:0259 |   const promise = new Promise<T>((resolve, reject) => {
apps/user-uni/src/api/client.ts:0260 |     requestTask = uni.request({
apps/user-uni/src/api/client.ts:0261 |       url: resolveApiURL(path),
apps/user-uni/src/api/client.ts:0262 |       method: (options.method || 'GET') as UniApp.RequestOptions['method'],
apps/user-uni/src/api/client.ts:0263 |       header: clientTransportHeaders(options.headers, usesSession),
apps/user-uni/src/api/client.ts:0264 |       data: options.data as UniApp.RequestOptions['data'],
apps/user-uni/src/api/client.ts:0265 |       timeout: options.timeout || requestTimeout,
apps/user-uni/src/api/client.ts:0266 |       success(response) {
apps/user-uni/src/api/client.ts:0267 |         if (usesSession && response.statusCode === 401) {
apps/user-uni/src/api/client.ts:0268 |           void handleUnauthorized({ path, statusCode: response.statusCode, payload: response.data, retryAttempt: 0 })
apps/user-uni/src/api/client.ts:0269 |         }
apps/user-uni/src/api/client.ts:0270 |         if (response.statusCode < 200 || response.statusCode >= 300) {
apps/user-uni/src/api/client.ts:0271 |           reject(new Error(responsePayloadMessage(response.data, `请求失败 (${response.statusCode})`)))
apps/user-uni/src/api/client.ts:0272 |           return
apps/user-uni/src/api/client.ts:0273 |         }
apps/user-uni/src/api/client.ts:0274 |         try {
apps/user-uni/src/api/client.ts:0275 |           resolve(unwrapTransportPayload<T>(response.data))
apps/user-uni/src/api/client.ts:0276 |         }
apps/user-uni/src/api/client.ts:0277 |         catch (error) {
apps/user-uni/src/api/client.ts:0278 |           reject(error)
apps/user-uni/src/api/client.ts:0279 |         }
apps/user-uni/src/api/client.ts:0280 |       },
apps/user-uni/src/api/client.ts:0281 |       fail(error) {
apps/user-uni/src/api/client.ts:0282 |         reject(new Error(error.errMsg || '请求失败，请检查网络'))
```

<div style="page-break-after: always;"></div>

### 第 07 页

**文件路径：** apps/user-uni/src/api/client.ts  
**代码说明：** 统一 API Client、认证头、租户上下文、刷新令牌与文件传输。

```typescript
apps/user-uni/src/api/client.ts:0283 |       },
apps/user-uni/src/api/client.ts:0284 |     }) as UniApp.RequestTask
apps/user-uni/src/api/client.ts:0285 |   })
apps/user-uni/src/api/client.ts:0286 |   return { promise, abort: () => requestTask?.abort() }
apps/user-uni/src/api/client.ts:0287 | }
apps/user-uni/src/api/client.ts:0288 | 
apps/user-uni/src/api/client.ts:0289 | export async function uploadApiFile<T = unknown>(
apps/user-uni/src/api/client.ts:0290 |   path: string,
apps/user-uni/src/api/client.ts:0291 |   options: { filePath: string; name?: string; formData?: Record<string, unknown>; headers?: Record<string, string>; timeout?: number; auth?: boolean; retriedAfterRefresh?: boolean },
apps/user-uni/src/api/client.ts:0292 | ) {
apps/user-uni/src/api/client.ts:0293 |   if (!adapter.uploadFile) throw new Error('当前运行环境不支持文件上传')
apps/user-uni/src/api/client.ts:0294 |   const usesSession = options.auth ?? trustedApiURL(path)
apps/user-uni/src/api/client.ts:0295 |   const response = await adapter.uploadFile<T>({
apps/user-uni/src/api/client.ts:0296 |     url: resolveApiURL(path),
apps/user-uni/src/api/client.ts:0297 |     filePath: options.filePath,
apps/user-uni/src/api/client.ts:0298 |     name: options.name || 'file',
apps/user-uni/src/api/client.ts:0299 |     formData: options.formData,
apps/user-uni/src/api/client.ts:0300 |     header: clientTransportHeaders(options.headers, usesSession),
apps/user-uni/src/api/client.ts:0301 |     timeout: options.timeout || requestTimeout,
apps/user-uni/src/api/client.ts:0302 |   })
apps/user-uni/src/api/client.ts:0303 |   if (usesSession && response.statusCode === 401) {
apps/user-uni/src/api/client.ts:0304 |     const retryAttempt = options.retriedAfterRefresh ? 1 : 0
apps/user-uni/src/api/client.ts:0305 |     const recovered = await handleUnauthorized({ path, statusCode: response.statusCode, payload: response.data, retryAttempt })
apps/user-uni/src/api/client.ts:0306 |     if (recovered && retryAttempt === 0) return uploadApiFile(path, { ...options, retriedAfterRefresh: true })
apps/user-uni/src/api/client.ts:0307 |   }
apps/user-uni/src/api/client.ts:0308 |   if (response.statusCode < 200 || response.statusCode >= 300) {
apps/user-uni/src/api/client.ts:0309 |     throw new Error(responsePayloadMessage(response.data, `文件上传失败 (${response.statusCode})`))
apps/user-uni/src/api/client.ts:0310 |   }
apps/user-uni/src/api/client.ts:0311 |   return unwrapTransportPayload<T>(response.data)
apps/user-uni/src/api/client.ts:0312 | }
apps/user-uni/src/api/client.ts:0313 | 
apps/user-uni/src/api/client.ts:0314 | export async function downloadApiFile<T = unknown>(
apps/user-uni/src/api/client.ts:0315 |   path: string,
apps/user-uni/src/api/client.ts:0316 |   options: { headers?: Record<string, string>; timeout?: number; auth?: boolean; retriedAfterRefresh?: boolean } = {},
apps/user-uni/src/api/client.ts:0317 | ): Promise<AdapterDownloadFileResponse<T>> {
apps/user-uni/src/api/client.ts:0318 |   if (!adapter.downloadFile) throw new Error('当前运行环境不支持文件下载')
apps/user-uni/src/api/client.ts:0319 |   const usesSession = options.auth ?? trustedApiURL(path)
apps/user-uni/src/api/client.ts:0320 |   const response = await adapter.downloadFile<T>({
apps/user-uni/src/api/client.ts:0321 |     url: resolveApiURL(path),
apps/user-uni/src/api/client.ts:0322 |     header: clientTransportHeaders({ Accept: '*/*', ...options.headers }, usesSession),
apps/user-uni/src/api/client.ts:0323 |     timeout: options.timeout || requestTimeout,
apps/user-uni/src/api/client.ts:0324 |   })
apps/user-uni/src/api/client.ts:0325 |   if (usesSession && response.statusCode === 401) {
apps/user-uni/src/api/client.ts:0326 |     const retryAttempt = options.retriedAfterRefresh ? 1 : 0
apps/user-uni/src/api/client.ts:0327 |     const recovered = await handleUnauthorized({ path, statusCode: response.statusCode, payload: response.data, retryAttempt })
apps/user-uni/src/api/client.ts:0328 |     if (recovered && retryAttempt === 0) return downloadApiFile(path, { ...options, retriedAfterRefresh: true })
apps/user-uni/src/api/client.ts:0329 |   }
apps/user-uni/src/api/client.ts:0330 |   if (response.statusCode < 200 || response.statusCode >= 300) {
apps/user-uni/src/api/client.ts:0331 |     throw new Error(responsePayloadMessage(response.data, `文件下载失败 (${response.statusCode})`))
apps/user-uni/src/api/client.ts:0332 |   }
```

<div style="page-break-after: always;"></div>

### 第 08 页

**文件路径：** apps/user-uni/src/api/client.ts；apps/user-uni/src/features/auth/api.ts  
**代码说明：** 统一 API Client、认证头、租户上下文、刷新令牌与文件传输。；手机号、密码、微信绑定及会话安全接口。

```text
apps/user-uni/src/api/client.ts:0333 |   return response
apps/user-uni/src/api/client.ts:0334 | }
apps/user-uni/src/api/client.ts:0335 | 
apps/user-uni/src/api/client.ts:0336 | export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
apps/user-uni/src/api/client.ts:0337 |   return sharedApiClient.request<T>(path, {
apps/user-uni/src/api/client.ts:0338 |     method: init.method || 'GET',
apps/user-uni/src/api/client.ts:0339 |     headers: normalizeHeaders(init.headers),
apps/user-uni/src/api/client.ts:0340 |     body: normalizeBody(init.body),
apps/user-uni/src/api/client.ts:0341 |     timeout: requestTimeout,
apps/user-uni/src/api/client.ts:0342 |   })
apps/user-uni/src/api/client.ts:0343 | }
apps/user-uni/src/features/auth/api.ts:0001 | import { apiClient, authService } from "../../api/client";
apps/user-uni/src/features/auth/api.ts:0002 | import type {
apps/user-uni/src/features/auth/api.ts:0003 |   AccountSecurityResponse,
apps/user-uni/src/features/auth/api.ts:0004 |   AuthAttributionInput,
apps/user-uni/src/features/auth/api.ts:0005 |   AuthFlowResponse,
apps/user-uni/src/features/auth/api.ts:0006 |   BindMobileResponse,
apps/user-uni/src/features/auth/api.ts:0007 |   InviteValidationResponse,
apps/user-uni/src/features/auth/api.ts:0008 |   SmsSendResponse,
apps/user-uni/src/features/auth/api.ts:0009 | } from "./types";
apps/user-uni/src/features/auth/api.ts:0010 | 
apps/user-uni/src/features/auth/api.ts:0011 | export const loginAPI = {
apps/user-uni/src/features/auth/api.ts:0012 |   wechatPhoneLogin(input: AuthAttributionInput & { wxLoginCode: string; phoneCode: string }) {
apps/user-uni/src/features/auth/api.ts:0013 |     return apiClient.request<AuthFlowResponse>("/api/v1/auth/wechat/phone-login", {
apps/user-uni/src/features/auth/api.ts:0014 |       method: "POST",
apps/user-uni/src/features/auth/api.ts:0015 |       body: input,
apps/user-uni/src/features/auth/api.ts:0016 |       timeout: 20000,
apps/user-uni/src/features/auth/api.ts:0017 |       auth: false,
apps/user-uni/src/features/auth/api.ts:0018 |     });
apps/user-uni/src/features/auth/api.ts:0019 |   },
apps/user-uni/src/features/auth/api.ts:0020 | 
apps/user-uni/src/features/auth/api.ts:0021 |   sendSms(mobile: string, purpose = "login") {
apps/user-uni/src/features/auth/api.ts:0022 |     return apiClient.request<SmsSendResponse>("/api/v1/auth/sms/send", {
apps/user-uni/src/features/auth/api.ts:0023 |       method: "POST",
apps/user-uni/src/features/auth/api.ts:0024 |       body: { mobile, purpose },
apps/user-uni/src/features/auth/api.ts:0025 |       timeout: 15000,
apps/user-uni/src/features/auth/api.ts:0026 |       auth: false,
apps/user-uni/src/features/auth/api.ts:0027 |     });
apps/user-uni/src/features/auth/api.ts:0028 |   },
apps/user-uni/src/features/auth/api.ts:0029 | 
apps/user-uni/src/features/auth/api.ts:0030 |   smsLogin(input: AuthAttributionInput & { mobile: string; smsCode: string }) {
apps/user-uni/src/features/auth/api.ts:0031 |     return apiClient.request<AuthFlowResponse>("/api/v1/auth/sms/login", {
apps/user-uni/src/features/auth/api.ts:0032 |       method: "POST",
apps/user-uni/src/features/auth/api.ts:0033 |       body: input,
apps/user-uni/src/features/auth/api.ts:0034 |       timeout: 20000,
apps/user-uni/src/features/auth/api.ts:0035 |       auth: false,
apps/user-uni/src/features/auth/api.ts:0036 |     });
apps/user-uni/src/features/auth/api.ts:0037 |   },
apps/user-uni/src/features/auth/api.ts:0038 | 
apps/user-uni/src/features/auth/api.ts:0039 |   passwordLogin(account: string, password: string, idempotencyKey: string) {
```

<div style="page-break-after: always;"></div>

### 第 09 页

**文件路径：** apps/user-uni/src/features/auth/api.ts  
**代码说明：** 手机号、密码、微信绑定及会话安全接口。

```typescript
apps/user-uni/src/features/auth/api.ts:0040 |     return apiClient.request<AuthFlowResponse>("/api/v1/auth/login", {
apps/user-uni/src/features/auth/api.ts:0041 |       method: "POST",
apps/user-uni/src/features/auth/api.ts:0042 |       body: { account, email: account, username: account, mobile: account, password, idempotencyKey },
apps/user-uni/src/features/auth/api.ts:0043 |       timeout: 15000,
apps/user-uni/src/features/auth/api.ts:0044 |       auth: false,
apps/user-uni/src/features/auth/api.ts:0045 |     });
apps/user-uni/src/features/auth/api.ts:0046 |   },
apps/user-uni/src/features/auth/api.ts:0047 | 
apps/user-uni/src/features/auth/api.ts:0048 |   validateInvite(inviteCode: string) {
apps/user-uni/src/features/auth/api.ts:0049 |     return apiClient.request<InviteValidationResponse>(
apps/user-uni/src/features/auth/api.ts:0050 |       `/api/v1/invite/agent/resolve?inviteCode=${encodeURIComponent(inviteCode)}`,
apps/user-uni/src/features/auth/api.ts:0051 |       { timeout: 10000, auth: false },
apps/user-uni/src/features/auth/api.ts:0052 |     );
apps/user-uni/src/features/auth/api.ts:0053 |   },
apps/user-uni/src/features/auth/api.ts:0054 | 
apps/user-uni/src/features/auth/api.ts:0055 |   security() {
apps/user-uni/src/features/auth/api.ts:0056 |     return apiClient.request<AccountSecurityResponse>("/api/v1/auth/security");
apps/user-uni/src/features/auth/api.ts:0057 |   },
apps/user-uni/src/features/auth/api.ts:0058 | 
apps/user-uni/src/features/auth/api.ts:0059 |   bindMobile(mobile: string, smsCode: string) {
apps/user-uni/src/features/auth/api.ts:0060 |     return apiClient.request<BindMobileResponse>("/api/v1/auth/mobile/bind", {
apps/user-uni/src/features/auth/api.ts:0061 |       method: "POST",
apps/user-uni/src/features/auth/api.ts:0062 |       body: { mobile, smsCode },
apps/user-uni/src/features/auth/api.ts:0063 |       timeout: 15000,
apps/user-uni/src/features/auth/api.ts:0064 |     });
apps/user-uni/src/features/auth/api.ts:0065 |   },
apps/user-uni/src/features/auth/api.ts:0066 | 
apps/user-uni/src/features/auth/api.ts:0067 |   linkWechat(wxLoginCode: string) {
apps/user-uni/src/features/auth/api.ts:0068 |     return apiClient.request<{ linked: boolean; userId: string }>("/api/v1/auth/wechat-mini-program/link", {
apps/user-uni/src/features/auth/api.ts:0069 |       method: "POST",
apps/user-uni/src/features/auth/api.ts:0070 |       body: { wxLoginCode },
apps/user-uni/src/features/auth/api.ts:0071 |       timeout: 15000,
apps/user-uni/src/features/auth/api.ts:0072 |     });
apps/user-uni/src/features/auth/api.ts:0073 |   },
apps/user-uni/src/features/auth/api.ts:0074 | 
apps/user-uni/src/features/auth/api.ts:0075 |   changePassword(currentPassword: string, newPassword: string) {
apps/user-uni/src/features/auth/api.ts:0076 |     return apiClient.request<{ ok: boolean; passwordSet: boolean }>("/api/v1/auth/change-password", {
apps/user-uni/src/features/auth/api.ts:0077 |       method: "POST",
apps/user-uni/src/features/auth/api.ts:0078 |       body: { currentPassword, newPassword },
apps/user-uni/src/features/auth/api.ts:0079 |     });
apps/user-uni/src/features/auth/api.ts:0080 |   },
apps/user-uni/src/features/auth/api.ts:0081 | 
apps/user-uni/src/features/auth/api.ts:0082 |   logout() {
apps/user-uni/src/features/auth/api.ts:0083 |     return apiClient.request<{ ok: boolean }>("/api/v1/auth/logout", {
apps/user-uni/src/features/auth/api.ts:0084 |       method: "POST",
apps/user-uni/src/features/auth/api.ts:0085 |       body: { refreshToken: authService.storage.getRefreshToken() },
apps/user-uni/src/features/auth/api.ts:0086 |       retryOnUnauthorized: false,
apps/user-uni/src/features/auth/api.ts:0087 |     });
apps/user-uni/src/features/auth/api.ts:0088 |   },
apps/user-uni/src/features/auth/api.ts:0089 | 
```

<div style="page-break-after: always;"></div>

### 第 10 页

**文件路径：** apps/user-uni/src/features/auth/api.ts；apps/user-uni/src/features/auth/gate.ts  
**代码说明：** 手机号、密码、微信绑定及会话安全接口。；受保护操作的登录门禁与待执行动作恢复。

```text
apps/user-uni/src/features/auth/api.ts:0090 |   logoutAll() {
apps/user-uni/src/features/auth/api.ts:0091 |     return apiClient.request<{ ok: boolean; userId?: string; revokedSessions?: number }>("/api/v1/auth/logout-all", {
apps/user-uni/src/features/auth/api.ts:0092 |       method: "POST",
apps/user-uni/src/features/auth/api.ts:0093 |     });
apps/user-uni/src/features/auth/api.ts:0094 |   },
apps/user-uni/src/features/auth/api.ts:0095 | };
apps/user-uni/src/features/auth/gate.ts:0001 | import { createAuthGate, createPendingActionStore, type AuthStatus, type PendingActionInput } from "@xianzhi/shared-auth";
apps/user-uni/src/features/auth/gate.ts:0002 | import { createUniPlatformAdapter } from "@xianzhi/platform-adapter";
apps/user-uni/src/features/auth/gate.ts:0003 | import { authStorage } from "../../api/client";
apps/user-uni/src/features/auth/gate.ts:0004 | import { trackLogin } from "./analytics";
apps/user-uni/src/features/auth/gate.ts:0005 | 
apps/user-uni/src/features/auth/gate.ts:0006 | const adapter = createUniPlatformAdapter();
apps/user-uni/src/features/auth/gate.ts:0007 | export const pendingActions = createPendingActionStore({ adapter });
apps/user-uni/src/features/auth/gate.ts:0008 | 
apps/user-uni/src/features/auth/gate.ts:0009 | let expired = false;
apps/user-uni/src/features/auth/gate.ts:0010 | 
apps/user-uni/src/features/auth/gate.ts:0011 | export function authStatus(): AuthStatus {
apps/user-uni/src/features/auth/gate.ts:0012 |   if (authStorage.getToken()) return "authenticated";
apps/user-uni/src/features/auth/gate.ts:0013 |   return expired ? "expired" : "guest";
apps/user-uni/src/features/auth/gate.ts:0014 | }
apps/user-uni/src/features/auth/gate.ts:0015 | 
apps/user-uni/src/features/auth/gate.ts:0016 | export function initializeAuth() {
apps/user-uni/src/features/auth/gate.ts:0017 |   expired = false;
apps/user-uni/src/features/auth/gate.ts:0018 |   pendingActions.get();
apps/user-uni/src/features/auth/gate.ts:0019 |   trackLogin(authStorage.getToken() ? "authenticated_open_app" : "guest_open_app");
apps/user-uni/src/features/auth/gate.ts:0020 |   return authStatus();
apps/user-uni/src/features/auth/gate.ts:0021 | }
apps/user-uni/src/features/auth/gate.ts:0022 | 
apps/user-uni/src/features/auth/gate.ts:0023 | export function hasValidToken() {
apps/user-uni/src/features/auth/gate.ts:0024 |   return authStatus() === "authenticated";
apps/user-uni/src/features/auth/gate.ts:0025 | }
apps/user-uni/src/features/auth/gate.ts:0026 | 
apps/user-uni/src/features/auth/gate.ts:0027 | export function handleTokenExpired() {
apps/user-uni/src/features/auth/gate.ts:0028 |   expired = true;
apps/user-uni/src/features/auth/gate.ts:0029 |   authStorage.clearSession();
apps/user-uni/src/features/auth/gate.ts:0030 | }
apps/user-uni/src/features/auth/gate.ts:0031 | 
apps/user-uni/src/features/auth/gate.ts:0032 | function currentRoute() {
apps/user-uni/src/features/auth/gate.ts:0033 |   const pages = typeof getCurrentPages === "function" ? getCurrentPages() : [];
apps/user-uni/src/features/auth/gate.ts:0034 |   const current = pages[pages.length - 1] as { route?: string; options?: Record<string, unknown> } | undefined;
apps/user-uni/src/features/auth/gate.ts:0035 |   const path = current?.route ? `/${String(current.route).replace(/^\/+/, "")}` : "/pages/user/UserHomePage";
apps/user-uni/src/features/auth/gate.ts:0036 |   const query = Object.fromEntries(Object.entries(current?.options || {}).map(([key, value]) => [key, String(value ?? "")]));
apps/user-uni/src/features/auth/gate.ts:0037 |   return { path, query };
apps/user-uni/src/features/auth/gate.ts:0038 | }
apps/user-uni/src/features/auth/gate.ts:0039 | 
apps/user-uni/src/features/auth/gate.ts:0040 | function openLoginPage() {
apps/user-uni/src/features/auth/gate.ts:0041 |   return new Promise<void>(resolve => {
apps/user-uni/src/features/auth/gate.ts:0042 |     trackLogin("login_modal_show");
apps/user-uni/src/features/auth/gate.ts:0043 |     uni.showModal({
apps/user-uni/src/features/auth/gate.ts:0044 |       title: "\u767b\u5f55\u540e\u7ee7\u7eed\u4f7f\u7528",
```

<div style="page-break-after: always;"></div>

### 第 11 页

**文件路径：** apps/user-uni/src/features/auth/gate.ts  
**代码说明：** 受保护操作的登录门禁与待执行动作恢复。

```typescript
apps/user-uni/src/features/auth/gate.ts:0045 |       content: "\u767b\u5f55\u540e\u53ef\u4fdd\u5b58\u5386\u53f2\u4f5c\u54c1\u3001\u540c\u6b65\u521b\u4f5c\u8bb0\u5f55\u3001\u67e5\u770b\u8d26\u6237\u989d\u5ea6\uff0c\u5e76\u7ee7\u7eed\u521a\u624d\u7684\u521b\u4f5c\u3002",
apps/user-uni/src/features/auth/gate.ts:0046 |       confirmText: "\u7acb\u5373\u767b\u5f55",
apps/user-uni/src/features/auth/gate.ts:0047 |       cancelText: "\u6682\u4e0d\u767b\u5f55",
apps/user-uni/src/features/auth/gate.ts:0048 |       confirmColor: "#4A6BFF",
apps/user-uni/src/features/auth/gate.ts:0049 |       success(result) {
apps/user-uni/src/features/auth/gate.ts:0050 |         if (!result.confirm) {
apps/user-uni/src/features/auth/gate.ts:0051 |           trackLogin("login_cancel");
apps/user-uni/src/features/auth/gate.ts:0052 |           if (adapter.platform === "web" && typeof window !== "undefined") window.dispatchEvent(new CustomEvent("zhiqiyun:auth-cancelled"));
apps/user-uni/src/features/auth/gate.ts:0053 |           resolve();
apps/user-uni/src/features/auth/gate.ts:0054 |           return;
apps/user-uni/src/features/auth/gate.ts:0055 |         }
apps/user-uni/src/features/auth/gate.ts:0056 |         trackLogin("login_start");
apps/user-uni/src/features/auth/gate.ts:0057 |         if (adapter.platform === "web" && typeof window !== "undefined") {
apps/user-uni/src/features/auth/gate.ts:0058 |           window.history.pushState({ auth: "login" }, "", "/login");
apps/user-uni/src/features/auth/gate.ts:0059 |           window.dispatchEvent(new PopStateEvent("popstate"));
apps/user-uni/src/features/auth/gate.ts:0060 |           resolve();
apps/user-uni/src/features/auth/gate.ts:0061 |           return;
apps/user-uni/src/features/auth/gate.ts:0062 |         }
apps/user-uni/src/features/auth/gate.ts:0063 |         const route = currentRoute();
apps/user-uni/src/features/auth/gate.ts:0064 |         const query = Object.entries(route.query).map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(value)}`).join("&");
apps/user-uni/src/features/auth/gate.ts:0065 |         const url = `/pages/WechatLoginPage?redirectPath=${encodeURIComponent(route.path)}&redirectQuery=${encodeURIComponent(query)}&sourcePage=${encodeURIComponent(route.path)}&pendingAction=1`;
apps/user-uni/src/features/auth/gate.ts:0066 |         uni.navigateTo({ url, complete: () => resolve(), fail: () => uni.redirectTo({ url }) });
apps/user-uni/src/features/auth/gate.ts:0067 |       },
apps/user-uni/src/features/auth/gate.ts:0068 |       fail: () => resolve(),
apps/user-uni/src/features/auth/gate.ts:0069 |     });
apps/user-uni/src/features/auth/gate.ts:0070 |   });
apps/user-uni/src/features/auth/gate.ts:0071 | }
apps/user-uni/src/features/auth/gate.ts:0072 | 
apps/user-uni/src/features/auth/gate.ts:0073 | const gate = createAuthGate({
apps/user-uni/src/features/auth/gate.ts:0074 |   getStatus: authStatus,
apps/user-uni/src/features/auth/gate.ts:0075 |   pendingActions,
apps/user-uni/src/features/auth/gate.ts:0076 |   openLogin: openLoginPage,
apps/user-uni/src/features/auth/gate.ts:0077 | });
apps/user-uni/src/features/auth/gate.ts:0078 | 
apps/user-uni/src/features/auth/gate.ts:0079 | export function requireAuth(input: PendingActionInput) {
apps/user-uni/src/features/auth/gate.ts:0080 |   return gate.requireAuth(input);
apps/user-uni/src/features/auth/gate.ts:0081 | }
apps/user-uni/src/features/auth/gate.ts:0082 | 
apps/user-uni/src/features/auth/gate.ts:0083 | export async function resumePendingAction() {
apps/user-uni/src/features/auth/gate.ts:0084 |   try {
apps/user-uni/src/features/auth/gate.ts:0085 |     const resumed = await gate.resumePendingAction();
apps/user-uni/src/features/auth/gate.ts:0086 |     trackLogin(resumed ? "pending_action_resume_success" : "pending_action_resume_failed", { reason: resumed ? "resumed" : "missing_callback" });
apps/user-uni/src/features/auth/gate.ts:0087 |     return resumed;
apps/user-uni/src/features/auth/gate.ts:0088 |   } catch {
apps/user-uni/src/features/auth/gate.ts:0089 |     trackLogin("pending_action_resume_failed", { reason: "execution_error" });
apps/user-uni/src/features/auth/gate.ts:0090 |     return false;
apps/user-uni/src/features/auth/gate.ts:0091 |   }
apps/user-uni/src/features/auth/gate.ts:0092 | }
apps/user-uni/src/features/auth/gate.ts:0093 | 
apps/user-uni/src/features/auth/gate.ts:0094 | export function clearPendingAction() {
```

<div style="page-break-after: always;"></div>

### 第 12 页

**文件路径：** apps/user-uni/src/features/auth/gate.ts；apps/user-uni/src/pages/WechatLoginPage.vue  
**代码说明：** 受保护操作的登录门禁与待执行动作恢复。；App/小程序登录注册页面和交互状态。

```text
apps/user-uni/src/features/auth/gate.ts:0095 |   gate.clearPendingAction();
apps/user-uni/src/features/auth/gate.ts:0096 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0001 | <template>
apps/user-uni/src/pages/WechatLoginPage.vue:0002 |   <SafeAreaContainer>
apps/user-uni/src/pages/WechatLoginPage.vue:0003 |     <BrandHeader />
apps/user-uni/src/pages/WechatLoginPage.vue:0004 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0005 |     <SuccessState v-if="viewState === 'success'" :benefit-text="benefitText" @start="enterProduct()" />
apps/user-uni/src/pages/WechatLoginPage.vue:0006 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0007 |     <ErrorState
apps/user-uni/src/pages/WechatLoginPage.vue:0008 |       v-else-if="viewState === 'error'"
apps/user-uni/src/pages/WechatLoginPage.vue:0009 |       :kind="errorState"
apps/user-uni/src/pages/WechatLoginPage.vue:0010 |       :detail="errorDetail"
apps/user-uni/src/pages/WechatLoginPage.vue:0011 |       @primary="handleErrorPrimary()"
apps/user-uni/src/pages/WechatLoginPage.vue:0012 |       @secondary="returnToAvailableLogin()"
apps/user-uni/src/pages/WechatLoginPage.vue:0013 |     />
apps/user-uni/src/pages/WechatLoginPage.vue:0014 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0015 |     <!-- #ifdef MP-WEIXIN -->
apps/user-uni/src/pages/WechatLoginPage.vue:0016 |     <LoginCard
apps/user-uni/src/pages/WechatLoginPage.vue:0017 |       v-if="viewState === 'form' && mode === 'wechat'"
apps/user-uni/src/pages/WechatLoginPage.vue:0018 |       title="欢迎使用知启云AI"
apps/user-uni/src/pages/WechatLoginPage.vue:0019 |       subtitle="登录后即可继续创作与管理作品"
apps/user-uni/src/pages/WechatLoginPage.vue:0020 |       mode="wechat"
apps/user-uni/src/pages/WechatLoginPage.vue:0021 |     >
apps/user-uni/src/pages/WechatLoginPage.vue:0022 |       <PrimaryLoginButton
apps/user-uni/src/pages/WechatLoginPage.vue:0023 |         label="手机号快捷登录"
apps/user-uni/src/pages/WechatLoginPage.vue:0024 |         :disabled="busy"
apps/user-uni/src/pages/WechatLoginPage.vue:0025 |         :open-type="agreementAccepted ? 'getPhoneNumber' : ''"
apps/user-uni/src/pages/WechatLoginPage.vue:0026 |         @activate="onWechatButtonClick()"
apps/user-uni/src/pages/WechatLoginPage.vue:0027 |         @getphonenumber="onGetPhoneNumber($event)"
apps/user-uni/src/pages/WechatLoginPage.vue:0028 |       />
apps/user-uni/src/pages/WechatLoginPage.vue:0029 |       <text class="auth-auto-register">未注册手机号将自动创建账号</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0030 |       <view class="auth-divider"><view /><text>其他登录方式</text><view /></view>
apps/user-uni/src/pages/WechatLoginPage.vue:0031 |       <button class="auth-login-mode-button" hover-class="auth-login-mode-hover" @click="switchMode('sms')">
apps/user-uni/src/pages/WechatLoginPage.vue:0032 |         <text>使用手机号验证码登录</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0033 |       </button>
apps/user-uni/src/pages/WechatLoginPage.vue:0034 |       <button
apps/user-uni/src/pages/WechatLoginPage.vue:0035 |         class="auth-login-mode-button muted auth-login-mode-password"
apps/user-uni/src/pages/WechatLoginPage.vue:0036 |         hover-class="auth-login-mode-hover"
apps/user-uni/src/pages/WechatLoginPage.vue:0037 |         @click="switchMode('password')"
apps/user-uni/src/pages/WechatLoginPage.vue:0038 |       >
apps/user-uni/src/pages/WechatLoginPage.vue:0039 |         <text>账号密码登录  ›</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0040 |       </button>
apps/user-uni/src/pages/WechatLoginPage.vue:0041 |       <InviteCodeEntry class="auth-invite-spacing" :status="inviteStatus" @click="openInviteSheet()" />
apps/user-uni/src/pages/WechatLoginPage.vue:0042 |       <AgreementCheckbox
apps/user-uni/src/pages/WechatLoginPage.vue:0043 |         v-model="agreementAccepted"
apps/user-uni/src/pages/WechatLoginPage.vue:0044 |         class="auth-agreement-spacing"
apps/user-uni/src/pages/WechatLoginPage.vue:0045 |         :highlight="agreementHighlight"
apps/user-uni/src/pages/WechatLoginPage.vue:0046 |         @open="openAgreement($event)"
apps/user-uni/src/pages/WechatLoginPage.vue:0047 |       />
apps/user-uni/src/pages/WechatLoginPage.vue:0048 |       <SecondaryLoginEntry class="auth-help-spacing" label="登录遇到问题？" muted @activate="showLoginHelp()" />
```

<div style="page-break-after: always;"></div>

### 第 13 页

**文件路径：** apps/user-uni/src/pages/WechatLoginPage.vue  
**代码说明：** App/小程序登录注册页面和交互状态。

```vue
apps/user-uni/src/pages/WechatLoginPage.vue:0049 |       <SecondaryLoginEntry class="auth-browse-entry" label="暂不登录，先浏览功能" muted @activate="enterGuestBrowse()" />
apps/user-uni/src/pages/WechatLoginPage.vue:0050 |     </LoginCard>
apps/user-uni/src/pages/WechatLoginPage.vue:0051 |     <!-- #endif -->
apps/user-uni/src/pages/WechatLoginPage.vue:0052 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0053 |     <LoginCard
apps/user-uni/src/pages/WechatLoginPage.vue:0054 |       v-if="viewState === 'form' && mode === 'sms'"
apps/user-uni/src/pages/WechatLoginPage.vue:0055 |       title="手机号验证码登录"
apps/user-uni/src/pages/WechatLoginPage.vue:0056 |       subtitle="登录与注册合并，系统自动识别账号"
apps/user-uni/src/pages/WechatLoginPage.vue:0057 |       mode="sms"
apps/user-uni/src/pages/WechatLoginPage.vue:0058 |     >
apps/user-uni/src/pages/WechatLoginPage.vue:0059 |       <MobileInput v-model="mobile" :error="mobileError" />
apps/user-uni/src/pages/WechatLoginPage.vue:0060 |       <VerificationCodeInput
apps/user-uni/src/pages/WechatLoginPage.vue:0061 |         v-model="smsCode"
apps/user-uni/src/pages/WechatLoginPage.vue:0062 |         :action-label="smsActionLabel"
apps/user-uni/src/pages/WechatLoginPage.vue:0063 |         :disabled="smsSending || countdown > 0 || busy"
apps/user-uni/src/pages/WechatLoginPage.vue:0064 |         :error="smsError"
apps/user-uni/src/pages/WechatLoginPage.vue:0065 |         @send="sendSmsCode()"
apps/user-uni/src/pages/WechatLoginPage.vue:0066 |         @confirm="loginWithSms()"
apps/user-uni/src/pages/WechatLoginPage.vue:0067 |       />
apps/user-uni/src/pages/WechatLoginPage.vue:0068 |       <PrimaryLoginButton label="登录 / 注册" :disabled="busy" @activate="loginWithSms()" />
apps/user-uni/src/pages/WechatLoginPage.vue:0069 |       <SecondaryLoginEntry class="auth-mode-back" label="账号密码登录" @activate="switchMode('password')" />
apps/user-uni/src/pages/WechatLoginPage.vue:0070 |       <!-- #ifdef MP-WEIXIN -->
apps/user-uni/src/pages/WechatLoginPage.vue:0071 |       <SecondaryLoginEntry class="auth-mode-back" label="返回手机号快捷登录" @activate="switchMode('wechat')" />
apps/user-uni/src/pages/WechatLoginPage.vue:0072 |       <!-- #endif -->
apps/user-uni/src/pages/WechatLoginPage.vue:0073 |       <InviteCodeEntry :status="inviteStatus" @click="openInviteSheet()" />
apps/user-uni/src/pages/WechatLoginPage.vue:0074 |       <AgreementCheckbox
apps/user-uni/src/pages/WechatLoginPage.vue:0075 |         v-model="agreementAccepted"
apps/user-uni/src/pages/WechatLoginPage.vue:0076 |         class="auth-agreement-spacing sms"
apps/user-uni/src/pages/WechatLoginPage.vue:0077 |         :highlight="agreementHighlight"
apps/user-uni/src/pages/WechatLoginPage.vue:0078 |         @open="openAgreement($event)"
apps/user-uni/src/pages/WechatLoginPage.vue:0079 |       />
apps/user-uni/src/pages/WechatLoginPage.vue:0080 |       <SecondaryLoginEntry class="auth-help-spacing sms" label="登录遇到问题？" muted @activate="showLoginHelp()" />
apps/user-uni/src/pages/WechatLoginPage.vue:0081 |       <SecondaryLoginEntry class="auth-browse-entry" label="暂不登录，先浏览功能" muted @activate="enterGuestBrowse()" />
apps/user-uni/src/pages/WechatLoginPage.vue:0082 |     </LoginCard>
apps/user-uni/src/pages/WechatLoginPage.vue:0083 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0084 |     <LoginCard
apps/user-uni/src/pages/WechatLoginPage.vue:0085 |       v-if="viewState === 'form' && mode === 'password'"
apps/user-uni/src/pages/WechatLoginPage.vue:0086 |       title="账号密码登录"
apps/user-uni/src/pages/WechatLoginPage.vue:0087 |       subtitle="适用于企业员工及已设置密码的账号"
apps/user-uni/src/pages/WechatLoginPage.vue:0088 |       mode="password"
apps/user-uni/src/pages/WechatLoginPage.vue:0089 |     >
apps/user-uni/src/pages/WechatLoginPage.vue:0090 |       <view class="auth-field-block">
apps/user-uni/src/pages/WechatLoginPage.vue:0091 |         <text class="auth-field-label">手机号 / 账号</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0092 |         <view :class="['auth-account-shell', { error: accountError }]">
apps/user-uni/src/pages/WechatLoginPage.vue:0093 |           <input
apps/user-uni/src/pages/WechatLoginPage.vue:0094 |             id="auth-account-input"
apps/user-uni/src/pages/WechatLoginPage.vue:0095 |             v-model="account"
apps/user-uni/src/pages/WechatLoginPage.vue:0096 |             class="auth-account-input"
apps/user-uni/src/pages/WechatLoginPage.vue:0097 |             maxlength="80"
apps/user-uni/src/pages/WechatLoginPage.vue:0098 |             placeholder="请输入手机号或账号"
```

<div style="page-break-after: always;"></div>

### 第 14 页

**文件路径：** apps/user-uni/src/pages/WechatLoginPage.vue  
**代码说明：** App/小程序登录注册页面和交互状态。

```vue
apps/user-uni/src/pages/WechatLoginPage.vue:0099 |             confirm-type="next"
apps/user-uni/src/pages/WechatLoginPage.vue:0100 |             @input="accountError = ''"
apps/user-uni/src/pages/WechatLoginPage.vue:0101 |           />
apps/user-uni/src/pages/WechatLoginPage.vue:0102 |         </view>
apps/user-uni/src/pages/WechatLoginPage.vue:0103 |         <text v-if="accountError" class="auth-field-error">{{ accountError }}</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0104 |       </view>
apps/user-uni/src/pages/WechatLoginPage.vue:0105 |       <view class="auth-field-block auth-password-field">
apps/user-uni/src/pages/WechatLoginPage.vue:0106 |         <text class="auth-field-label">密码</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0107 |         <view :class="['auth-password-shell', { error: passwordError }]">
apps/user-uni/src/pages/WechatLoginPage.vue:0108 |           <input
apps/user-uni/src/pages/WechatLoginPage.vue:0109 |             id="auth-password-input"
apps/user-uni/src/pages/WechatLoginPage.vue:0110 |             v-model="password"
apps/user-uni/src/pages/WechatLoginPage.vue:0111 |             class="auth-password-input"
apps/user-uni/src/pages/WechatLoginPage.vue:0112 |             :password="!passwordVisible"
apps/user-uni/src/pages/WechatLoginPage.vue:0113 |             maxlength="64"
apps/user-uni/src/pages/WechatLoginPage.vue:0114 |             placeholder="请输入登录密码"
apps/user-uni/src/pages/WechatLoginPage.vue:0115 |             confirm-type="done"
apps/user-uni/src/pages/WechatLoginPage.vue:0116 |             @input="passwordError = ''"
apps/user-uni/src/pages/WechatLoginPage.vue:0117 |             @confirm="loginWithPassword()"
apps/user-uni/src/pages/WechatLoginPage.vue:0118 |           />
apps/user-uni/src/pages/WechatLoginPage.vue:0119 |           <button
apps/user-uni/src/pages/WechatLoginPage.vue:0120 |             class="auth-password-toggle"
apps/user-uni/src/pages/WechatLoginPage.vue:0121 |             :aria-label="passwordVisible ? '隐藏密码' : '显示密码'"
apps/user-uni/src/pages/WechatLoginPage.vue:0122 |             @click="passwordVisible = !passwordVisible"
apps/user-uni/src/pages/WechatLoginPage.vue:0123 |           >
apps/user-uni/src/pages/WechatLoginPage.vue:0124 |             <text>{{ passwordVisible ? "◉" : "◎" }}</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0125 |           </button>
apps/user-uni/src/pages/WechatLoginPage.vue:0126 |         </view>
apps/user-uni/src/pages/WechatLoginPage.vue:0127 |         <text v-if="passwordError" class="auth-field-error">{{ passwordError }}</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0128 |       </view>
apps/user-uni/src/pages/WechatLoginPage.vue:0129 |       <button class="auth-forgot" @click="forgotPassword()">忘记密码？</button>
apps/user-uni/src/pages/WechatLoginPage.vue:0130 |       <view
apps/user-uni/src/pages/WechatLoginPage.vue:0131 |         :class="['auth-password-submit', { disabled: busy }]"
apps/user-uni/src/pages/WechatLoginPage.vue:0132 |         role="button"
apps/user-uni/src/pages/WechatLoginPage.vue:0133 |         :aria-disabled="busy"
apps/user-uni/src/pages/WechatLoginPage.vue:0134 |         hover-class="auth-password-submit-pressed"
apps/user-uni/src/pages/WechatLoginPage.vue:0135 |         @tap="loginWithPassword()"
apps/user-uni/src/pages/WechatLoginPage.vue:0136 |       >
apps/user-uni/src/pages/WechatLoginPage.vue:0137 |         <text>{{ busy ? "正在登录…" : agreementAccepted ? "登录" : "同意协议并登录" }}</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0138 |       </view>
apps/user-uni/src/pages/WechatLoginPage.vue:0139 |       <SecondaryLoginEntry label="使用手机号验证码登录" @activate="switchMode('sms')" />
apps/user-uni/src/pages/WechatLoginPage.vue:0140 |       <!-- #ifdef MP-WEIXIN -->
apps/user-uni/src/pages/WechatLoginPage.vue:0141 |       <SecondaryLoginEntry label="返回手机号快捷登录" muted @activate="switchMode('wechat')" />
apps/user-uni/src/pages/WechatLoginPage.vue:0142 |       <!-- #endif -->
apps/user-uni/src/pages/WechatLoginPage.vue:0143 |       <view class="auth-password-note"><text>首次快捷登录或验证码登录后，可在账号与安全中设置密码</text></view>
apps/user-uni/src/pages/WechatLoginPage.vue:0144 |       <view :class="['auth-password-agreement', { highlight: agreementHighlight }]">
apps/user-uni/src/pages/WechatLoginPage.vue:0145 |         <view :class="['auth-password-agreement-toggle', { checked: agreementAccepted }]" @tap.stop="togglePasswordAgreement()">
apps/user-uni/src/pages/WechatLoginPage.vue:0146 |           <view :class="['auth-password-agreement-box', { checked: agreementAccepted }]">
apps/user-uni/src/pages/WechatLoginPage.vue:0147 |             <text v-if="agreementAccepted">✓</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0148 |           </view>
```

<div style="page-break-after: always;"></div>

### 第 15 页

**文件路径：** apps/user-uni/src/pages/WechatLoginPage.vue  
**代码说明：** App/小程序登录注册页面和交互状态。

```vue
apps/user-uni/src/pages/WechatLoginPage.vue:0149 |           <text>我已阅读并同意</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0150 |         </view>
apps/user-uni/src/pages/WechatLoginPage.vue:0151 |         <view class="auth-password-agreement-copy">
apps/user-uni/src/pages/WechatLoginPage.vue:0152 |           <text class="auth-password-agreement-link" @click.stop="openAgreement('user')">《用户协议》</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0153 |           <text>和</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0154 |           <text class="auth-password-agreement-link" @click.stop="openAgreement('privacy')">《隐私政策》</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0155 |         </view>
apps/user-uni/src/pages/WechatLoginPage.vue:0156 |       </view>
apps/user-uni/src/pages/WechatLoginPage.vue:0157 |       <SecondaryLoginEntry class="auth-browse-entry" label="暂不登录，先浏览功能" muted @activate="enterGuestBrowse()" />
apps/user-uni/src/pages/WechatLoginPage.vue:0158 |     </LoginCard>
apps/user-uni/src/pages/WechatLoginPage.vue:0159 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0160 |     <BottomSheet
apps/user-uni/src/pages/WechatLoginPage.vue:0161 |       :visible="inviteSheetVisible"
apps/user-uni/src/pages/WechatLoginPage.vue:0162 |       title="填写邀请码"
apps/user-uni/src/pages/WechatLoginPage.vue:0163 |       :keyboard-height="keyboardHeight"
apps/user-uni/src/pages/WechatLoginPage.vue:0164 |       @close="closeInviteSheet()"
apps/user-uni/src/pages/WechatLoginPage.vue:0165 |     >
apps/user-uni/src/pages/WechatLoginPage.vue:0166 |       <view class="auth-sheet-field">
apps/user-uni/src/pages/WechatLoginPage.vue:0167 |         <text class="auth-sheet-label">邀请码</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0168 |         <input
apps/user-uni/src/pages/WechatLoginPage.vue:0169 |           id="auth-invite-input"
apps/user-uni/src/pages/WechatLoginPage.vue:0170 |           v-model="inviteDraft"
apps/user-uni/src/pages/WechatLoginPage.vue:0171 |           class="auth-sheet-input"
apps/user-uni/src/pages/WechatLoginPage.vue:0172 |           maxlength="32"
apps/user-uni/src/pages/WechatLoginPage.vue:0173 |           placeholder="请输入邀请码"
apps/user-uni/src/pages/WechatLoginPage.vue:0174 |           confirm-type="done"
apps/user-uni/src/pages/WechatLoginPage.vue:0175 |           @input="inviteMessage = ''"
apps/user-uni/src/pages/WechatLoginPage.vue:0176 |           @confirm="confirmInvite()"
apps/user-uni/src/pages/WechatLoginPage.vue:0177 |         />
apps/user-uni/src/pages/WechatLoginPage.vue:0178 |         <text :class="['auth-invite-message', { error: inviteMessageTone === 'error', success: inviteMessageTone === 'success' }]">
apps/user-uni/src/pages/WechatLoginPage.vue:0179 |           {{ inviteMessage || "邀请码为选填，不影响正常登录注册" }}
apps/user-uni/src/pages/WechatLoginPage.vue:0180 |         </text>
apps/user-uni/src/pages/WechatLoginPage.vue:0181 |       </view>
apps/user-uni/src/pages/WechatLoginPage.vue:0182 |       <PrimaryLoginButton label="确认填写" :loading="inviteValidating" loading-text="正在校验…" @activate="confirmInvite()" />
apps/user-uni/src/pages/WechatLoginPage.vue:0183 |       <SecondaryLoginEntry label="暂不填写" muted @activate="closeInviteSheet()" />
apps/user-uni/src/pages/WechatLoginPage.vue:0184 |       <SecondaryLoginEntry v-if="pendingInviteCode" label="删除邀请码" muted @activate="removeInvite()" />
apps/user-uni/src/pages/WechatLoginPage.vue:0185 |     </BottomSheet>
apps/user-uni/src/pages/WechatLoginPage.vue:0186 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0187 |     <!-- #ifdef MP-WEIXIN -->
apps/user-uni/src/pages/WechatLoginPage.vue:0188 |     <BottomSheet :visible="authorizationSheetVisible" :closable="false" :close-on-overlay="false">
apps/user-uni/src/pages/WechatLoginPage.vue:0189 |       <view class="auth-permission-sheet">
apps/user-uni/src/pages/WechatLoginPage.vue:0190 |         <view class="auth-permission-icon">!</view>
apps/user-uni/src/pages/WechatLoginPage.vue:0191 |         <text class="auth-permission-title">未获得手机号授权</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0192 |         <text class="auth-permission-copy">你仍可以使用手机号验证码继续登录</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0193 |         <PrimaryLoginButton label="使用验证码登录" @activate="useSmsAfterAuthorizationFailure()" />
apps/user-uni/src/pages/WechatLoginPage.vue:0194 |         <PrimaryLoginButton
apps/user-uni/src/pages/WechatLoginPage.vue:0195 |           class="auth-retry-authorization"
apps/user-uni/src/pages/WechatLoginPage.vue:0196 |           label="重新授权"
apps/user-uni/src/pages/WechatLoginPage.vue:0197 |           open-type="getPhoneNumber"
apps/user-uni/src/pages/WechatLoginPage.vue:0198 |           @getphonenumber="onGetPhoneNumber($event)"
```

<div style="page-break-after: always;"></div>

### 第 16 页

**文件路径：** apps/user-uni/src/pages/WechatLoginPage.vue  
**代码说明：** App/小程序登录注册页面和交互状态。

```vue
apps/user-uni/src/pages/WechatLoginPage.vue:0199 |         />
apps/user-uni/src/pages/WechatLoginPage.vue:0200 |       </view>
apps/user-uni/src/pages/WechatLoginPage.vue:0201 |     </BottomSheet>
apps/user-uni/src/pages/WechatLoginPage.vue:0202 |     <!-- #endif -->
apps/user-uni/src/pages/WechatLoginPage.vue:0203 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0204 |     <BottomSheet :visible="agreementSheetVisible" :title="agreementSheetTitle" @close="agreementSheetVisible = false">
apps/user-uni/src/pages/WechatLoginPage.vue:0205 |       <scroll-view class="auth-agreement-document" scroll-y>
apps/user-uni/src/pages/WechatLoginPage.vue:0206 |         <text>{{ agreementSheetContent }}</text>
apps/user-uni/src/pages/WechatLoginPage.vue:0207 |       </scroll-view>
apps/user-uni/src/pages/WechatLoginPage.vue:0208 |       <PrimaryLoginButton label="我已阅读并同意" @activate="acceptAgreementFromSheet()" />
apps/user-uni/src/pages/WechatLoginPage.vue:0209 |     </BottomSheet>
apps/user-uni/src/pages/WechatLoginPage.vue:0210 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0211 |     <LoginLoading :visible="busy" :step="loadingStep" />
apps/user-uni/src/pages/WechatLoginPage.vue:0212 |     <Toast :visible="toastVisible" :message="toastMessage" :tone="toastTone" />
apps/user-uni/src/pages/WechatLoginPage.vue:0213 |   </SafeAreaContainer>
apps/user-uni/src/pages/WechatLoginPage.vue:0214 | </template>
apps/user-uni/src/pages/WechatLoginPage.vue:0215 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0216 | <script setup lang="ts">
apps/user-uni/src/pages/WechatLoginPage.vue:0217 | import { computed, reactive, ref, watch } from "vue";
apps/user-uni/src/pages/WechatLoginPage.vue:0218 | import { onLoad, onUnload } from "@dcloudio/uni-app";
apps/user-uni/src/pages/WechatLoginPage.vue:0219 | import { ApiClientError } from "@xianzhi/api-client";
apps/user-uni/src/pages/WechatLoginPage.vue:0220 | import { authStorage } from "../api/client";
apps/user-uni/src/pages/WechatLoginPage.vue:0221 | import AgreementCheckbox from "../components/auth/AgreementCheckbox.vue";
apps/user-uni/src/pages/WechatLoginPage.vue:0222 | import BottomSheet from "../components/auth/BottomSheet.vue";
apps/user-uni/src/pages/WechatLoginPage.vue:0223 | import BrandHeader from "../components/auth/BrandHeader.vue";
apps/user-uni/src/pages/WechatLoginPage.vue:0224 | import ErrorState from "../components/auth/ErrorState.vue";
apps/user-uni/src/pages/WechatLoginPage.vue:0225 | import InviteCodeEntry from "../components/auth/InviteCodeEntry.vue";
apps/user-uni/src/pages/WechatLoginPage.vue:0226 | import LoginCard from "../components/auth/LoginCard.vue";
apps/user-uni/src/pages/WechatLoginPage.vue:0227 | import LoginLoading from "../components/auth/LoginLoading.vue";
apps/user-uni/src/pages/WechatLoginPage.vue:0228 | import MobileInput from "../components/auth/MobileInput.vue";
apps/user-uni/src/pages/WechatLoginPage.vue:0229 | import PrimaryLoginButton from "../components/auth/PrimaryLoginButton.vue";
apps/user-uni/src/pages/WechatLoginPage.vue:0230 | import SafeAreaContainer from "../components/auth/SafeAreaContainer.vue";
apps/user-uni/src/pages/WechatLoginPage.vue:0231 | import SecondaryLoginEntry from "../components/auth/SecondaryLoginEntry.vue";
apps/user-uni/src/pages/WechatLoginPage.vue:0232 | import SuccessState from "../components/auth/SuccessState.vue";
apps/user-uni/src/pages/WechatLoginPage.vue:0233 | import Toast from "../components/auth/Toast.vue";
apps/user-uni/src/pages/WechatLoginPage.vue:0234 | import VerificationCodeInput from "../components/auth/VerificationCodeInput.vue";
apps/user-uni/src/pages/WechatLoginPage.vue:0235 | import { trackLogin } from "../features/auth/analytics";
apps/user-uni/src/pages/WechatLoginPage.vue:0236 | import { loginAPI } from "../features/auth/api";
apps/user-uni/src/pages/WechatLoginPage.vue:0237 | import { redirectAfterAuth } from "../features/auth/redirect";
apps/user-uni/src/pages/WechatLoginPage.vue:0238 | import { parseLoginSource, parseRedirectInfo } from "../features/auth/source";
apps/user-uni/src/pages/WechatLoginPage.vue:0239 | import type {
apps/user-uni/src/pages/WechatLoginPage.vue:0240 |   AuthFlowResponse,
apps/user-uni/src/pages/WechatLoginPage.vue:0241 |   InviteStatus,
apps/user-uni/src/pages/WechatLoginPage.vue:0242 |   LoadingStep,
apps/user-uni/src/pages/WechatLoginPage.vue:0243 |   LoginErrorState,
apps/user-uni/src/pages/WechatLoginPage.vue:0244 |   LoginMode,
apps/user-uni/src/pages/WechatLoginPage.vue:0245 |   LoginRedirectInfo,
apps/user-uni/src/pages/WechatLoginPage.vue:0246 |   LoginSourceParams,
apps/user-uni/src/pages/WechatLoginPage.vue:0247 | } from "../features/auth/types";
apps/user-uni/src/pages/WechatLoginPage.vue:0248 | import { useAuthStore } from "../stores/auth";
```

<div style="page-break-after: always;"></div>

### 第 17 页

**文件路径：** apps/user-uni/src/pages/WechatLoginPage.vue  
**代码说明：** App/小程序登录注册页面和交互状态。

```vue
apps/user-uni/src/pages/WechatLoginPage.vue:0249 | import { useUserStore } from "../stores/user";
apps/user-uni/src/pages/WechatLoginPage.vue:0250 | import type { AppRole } from "../types";
apps/user-uni/src/pages/WechatLoginPage.vue:0251 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0252 | type ViewState = "form" | "success" | "error";
apps/user-uni/src/pages/WechatLoginPage.vue:0253 | type ToastTone = "info" | "error" | "success";
apps/user-uni/src/pages/WechatLoginPage.vue:0254 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0255 | const authStore = useAuthStore();
apps/user-uni/src/pages/WechatLoginPage.vue:0256 | const userStore = useUserStore();
apps/user-uni/src/pages/WechatLoginPage.vue:0257 | const mode = ref<LoginMode>("wechat");
apps/user-uni/src/pages/WechatLoginPage.vue:0258 | // App MVP uses backend-supported SMS/password login. WeChat App OAuth is enabled only after
apps/user-uni/src/pages/WechatLoginPage.vue:0259 | // the dedicated Open Platform backend exchange is configured; the mini-program code path is not reused.
apps/user-uni/src/pages/WechatLoginPage.vue:0260 | // #ifdef APP-PLUS
apps/user-uni/src/pages/WechatLoginPage.vue:0261 | mode.value = "sms";
apps/user-uni/src/pages/WechatLoginPage.vue:0262 | // #endif
apps/user-uni/src/pages/WechatLoginPage.vue:0263 | const viewState = ref<ViewState>("form");
apps/user-uni/src/pages/WechatLoginPage.vue:0264 | const errorState = ref<LoginErrorState>("network");
apps/user-uni/src/pages/WechatLoginPage.vue:0265 | const errorDetail = ref("");
apps/user-uni/src/pages/WechatLoginPage.vue:0266 | const loadingStep = ref<LoadingStep>("authorizing");
apps/user-uni/src/pages/WechatLoginPage.vue:0267 | const busy = ref(false);
apps/user-uni/src/pages/WechatLoginPage.vue:0268 | const agreementAccepted = ref(false);
apps/user-uni/src/pages/WechatLoginPage.vue:0269 | const agreementHighlight = ref(false);
apps/user-uni/src/pages/WechatLoginPage.vue:0270 | const mobile = ref("");
apps/user-uni/src/pages/WechatLoginPage.vue:0271 | const smsCode = ref("");
apps/user-uni/src/pages/WechatLoginPage.vue:0272 | const account = ref("");
apps/user-uni/src/pages/WechatLoginPage.vue:0273 | const password = ref("");
apps/user-uni/src/pages/WechatLoginPage.vue:0274 | const passwordVisible = ref(false);
apps/user-uni/src/pages/WechatLoginPage.vue:0275 | const mobileError = ref("");
apps/user-uni/src/pages/WechatLoginPage.vue:0276 | const smsError = ref("");
apps/user-uni/src/pages/WechatLoginPage.vue:0277 | const accountError = ref("");
apps/user-uni/src/pages/WechatLoginPage.vue:0278 | const passwordError = ref("");
apps/user-uni/src/pages/WechatLoginPage.vue:0279 | const smsSending = ref(false);
apps/user-uni/src/pages/WechatLoginPage.vue:0280 | const countdown = ref(0);
apps/user-uni/src/pages/WechatLoginPage.vue:0281 | const keyboardHeight = ref(0);
apps/user-uni/src/pages/WechatLoginPage.vue:0282 | const scrollTarget = ref("");
apps/user-uni/src/pages/WechatLoginPage.vue:0283 | const inviteSheetVisible = ref(false);
apps/user-uni/src/pages/WechatLoginPage.vue:0284 | const authorizationSheetVisible = ref(false);
apps/user-uni/src/pages/WechatLoginPage.vue:0285 | const agreementSheetVisible = ref(false);
apps/user-uni/src/pages/WechatLoginPage.vue:0286 | const agreementSheetTitle = ref("");
apps/user-uni/src/pages/WechatLoginPage.vue:0287 | const agreementSheetContent = ref("");
apps/user-uni/src/pages/WechatLoginPage.vue:0288 | const pendingInviteCode = ref("");
apps/user-uni/src/pages/WechatLoginPage.vue:0289 | const inviteDraft = ref("");
apps/user-uni/src/pages/WechatLoginPage.vue:0290 | const inviteStatus = ref<InviteStatus>("empty");
apps/user-uni/src/pages/WechatLoginPage.vue:0291 | const inviteValidating = ref(false);
apps/user-uni/src/pages/WechatLoginPage.vue:0292 | const inviteMessage = ref("");
apps/user-uni/src/pages/WechatLoginPage.vue:0293 | const inviteMessageTone = ref<"info" | "error" | "success">("info");
apps/user-uni/src/pages/WechatLoginPage.vue:0294 | const benefitText = ref("新人体验权益已到账");
apps/user-uni/src/pages/WechatLoginPage.vue:0295 | const toastVisible = ref(false);
apps/user-uni/src/pages/WechatLoginPage.vue:0296 | const toastMessage = ref("");
apps/user-uni/src/pages/WechatLoginPage.vue:0297 | const toastTone = ref<ToastTone>("info");
apps/user-uni/src/pages/WechatLoginPage.vue:0298 | const idempotencyKeys = reactive<Partial<Record<LoginMode, string>>>({});
```

<div style="page-break-after: always;"></div>

### 第 18 页

**文件路径：** apps/user-uni/src/pages/WechatLoginPage.vue  
**代码说明：** App/小程序登录注册页面和交互状态。

```vue
apps/user-uni/src/pages/WechatLoginPage.vue:0299 | let countdownTimer: ReturnType<typeof setInterval> | null = null;
apps/user-uni/src/pages/WechatLoginPage.vue:0300 | let toastTimer: ReturnType<typeof setTimeout> | null = null;
apps/user-uni/src/pages/WechatLoginPage.vue:0301 | let agreementTimer: ReturnType<typeof setTimeout> | null = null;
apps/user-uni/src/pages/WechatLoginPage.vue:0302 | let requestVersion = 0;
apps/user-uni/src/pages/WechatLoginPage.vue:0303 | let destroyed = false;
apps/user-uni/src/pages/WechatLoginPage.vue:0304 | let sourceParams: LoginSourceParams = {
apps/user-uni/src/pages/WechatLoginPage.vue:0305 |   inviteCode: "", inviteSource: "none", sceneCode: "", promoterCode: "", campaignCode: "", channel: "", sourcePage: "",
apps/user-uni/src/pages/WechatLoginPage.vue:0306 | };
apps/user-uni/src/pages/WechatLoginPage.vue:0307 | let redirectInfo: LoginRedirectInfo = { path: "", query: {}, action: "", sourcePage: "" };
apps/user-uni/src/pages/WechatLoginPage.vue:0308 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0309 | const smsActionLabel = computed(() => {
apps/user-uni/src/pages/WechatLoginPage.vue:0310 |   if (smsSending.value) return "发送中";
apps/user-uni/src/pages/WechatLoginPage.vue:0311 |   if (countdown.value > 0) return `${countdown.value}秒后重新获取`;
apps/user-uni/src/pages/WechatLoginPage.vue:0312 |   return "获取验证码";
apps/user-uni/src/pages/WechatLoginPage.vue:0313 | });
apps/user-uni/src/pages/WechatLoginPage.vue:0314 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0315 | watch(mobile, () => { mobileError.value = ""; smsError.value = ""; idempotencyKeys.sms = ""; });
apps/user-uni/src/pages/WechatLoginPage.vue:0316 | watch(smsCode, () => { smsError.value = ""; });
apps/user-uni/src/pages/WechatLoginPage.vue:0317 | watch(account, () => { accountError.value = ""; idempotencyKeys.password = ""; });
apps/user-uni/src/pages/WechatLoginPage.vue:0318 | watch(password, () => { passwordError.value = ""; });
apps/user-uni/src/pages/WechatLoginPage.vue:0319 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0320 | function showToast(message: string, tone: ToastTone = "info") {
apps/user-uni/src/pages/WechatLoginPage.vue:0321 |   if (toastTimer) clearTimeout(toastTimer);
apps/user-uni/src/pages/WechatLoginPage.vue:0322 |   toastMessage.value = message;
apps/user-uni/src/pages/WechatLoginPage.vue:0323 |   toastTone.value = tone;
apps/user-uni/src/pages/WechatLoginPage.vue:0324 |   toastVisible.value = true;
apps/user-uni/src/pages/WechatLoginPage.vue:0325 |   toastTimer = setTimeout(() => { toastVisible.value = false; }, 2200);
apps/user-uni/src/pages/WechatLoginPage.vue:0326 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0327 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0328 | function ensureAgreement(): boolean {
apps/user-uni/src/pages/WechatLoginPage.vue:0329 |   if (agreementAccepted.value) return true;
apps/user-uni/src/pages/WechatLoginPage.vue:0330 |   agreementHighlight.value = true;
apps/user-uni/src/pages/WechatLoginPage.vue:0331 |   scrollTarget.value = "auth-login-card";
apps/user-uni/src/pages/WechatLoginPage.vue:0332 |   showToast("请先阅读并同意用户协议和隐私政策", "error");
apps/user-uni/src/pages/WechatLoginPage.vue:0333 |   if (agreementTimer) clearTimeout(agreementTimer);
apps/user-uni/src/pages/WechatLoginPage.vue:0334 |   agreementTimer = setTimeout(() => { agreementHighlight.value = false; }, 1600);
apps/user-uni/src/pages/WechatLoginPage.vue:0335 |   return false;
apps/user-uni/src/pages/WechatLoginPage.vue:0336 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0337 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0338 | function switchMode(next: LoginMode) {
apps/user-uni/src/pages/WechatLoginPage.vue:0339 |   if (busy.value) return;
apps/user-uni/src/pages/WechatLoginPage.vue:0340 |   viewState.value = "form";
apps/user-uni/src/pages/WechatLoginPage.vue:0341 |   mode.value = next;
apps/user-uni/src/pages/WechatLoginPage.vue:0342 |   mobileError.value = smsError.value = accountError.value = passwordError.value = "";
apps/user-uni/src/pages/WechatLoginPage.vue:0343 |   scrollTarget.value = "auth-login-card";
apps/user-uni/src/pages/WechatLoginPage.vue:0344 |   if (next === "sms") trackLogin("sms_login_click");
apps/user-uni/src/pages/WechatLoginPage.vue:0345 |   if (next === "password") trackLogin("password_login_click");
apps/user-uni/src/pages/WechatLoginPage.vue:0346 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0347 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0348 | function normalizeMobile(value: string): string {
```

<div style="page-break-after: always;"></div>

### 第 19 页

**文件路径：** apps/user-uni/src/pages/WechatLoginPage.vue  
**代码说明：** App/小程序登录注册页面和交互状态。

```vue
apps/user-uni/src/pages/WechatLoginPage.vue:0349 |   return value.replace(/\D/g, "").slice(0, 11);
apps/user-uni/src/pages/WechatLoginPage.vue:0350 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0351 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0352 | function validMobile(value: string): boolean {
apps/user-uni/src/pages/WechatLoginPage.vue:0353 |   return /^1[3-9]\d{9}$/.test(normalizeMobile(value));
apps/user-uni/src/pages/WechatLoginPage.vue:0354 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0355 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0356 | function nextIdempotencyKey(method: LoginMode): string {
apps/user-uni/src/pages/WechatLoginPage.vue:0357 |   if (!idempotencyKeys[method]) {
apps/user-uni/src/pages/WechatLoginPage.vue:0358 |     idempotencyKeys[method] = `${method}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
apps/user-uni/src/pages/WechatLoginPage.vue:0359 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0360 |   return idempotencyKeys[method] as string;
apps/user-uni/src/pages/WechatLoginPage.vue:0361 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0362 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0363 | function attribution(method: LoginMode) {
apps/user-uni/src/pages/WechatLoginPage.vue:0364 |   return {
apps/user-uni/src/pages/WechatLoginPage.vue:0365 |     inviteCode: pendingInviteCode.value || undefined,
apps/user-uni/src/pages/WechatLoginPage.vue:0366 |     scene: sourceParams.sceneCode || undefined,
apps/user-uni/src/pages/WechatLoginPage.vue:0367 |     promoterCode: sourceParams.promoterCode || undefined,
apps/user-uni/src/pages/WechatLoginPage.vue:0368 |     campaignCode: sourceParams.campaignCode || undefined,
apps/user-uni/src/pages/WechatLoginPage.vue:0369 |     redirectSource: redirectInfo.sourcePage || sourceParams.sourcePage || sourceParams.channel || undefined,
apps/user-uni/src/pages/WechatLoginPage.vue:0370 |     idempotencyKey: nextIdempotencyKey(method),
apps/user-uni/src/pages/WechatLoginPage.vue:0371 |   };
apps/user-uni/src/pages/WechatLoginPage.vue:0372 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0373 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0374 | function defaultRole(roles: AppRole[] | undefined): AppRole {
apps/user-uni/src/pages/WechatLoginPage.vue:0375 |   if (roles?.includes("OPERATION")) return "OPERATION";
apps/user-uni/src/pages/WechatLoginPage.vue:0376 |   if (roles?.includes("AGENT")) return "AGENT";
apps/user-uni/src/pages/WechatLoginPage.vue:0377 |   return "USER";
apps/user-uni/src/pages/WechatLoginPage.vue:0378 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0379 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0380 | async function completeAuth(auth: AuthFlowResponse, version: number) {
apps/user-uni/src/pages/WechatLoginPage.vue:0381 |   if (destroyed || version !== requestVersion) return;
apps/user-uni/src/pages/WechatLoginPage.vue:0382 |   if (!auth.accessToken) throw new Error("TOKEN_SAVE_FAILED");
apps/user-uni/src/pages/WechatLoginPage.vue:0383 |   try {
apps/user-uni/src/pages/WechatLoginPage.vue:0384 |     authStorage.setToken(auth.accessToken);
apps/user-uni/src/pages/WechatLoginPage.vue:0385 |     authStorage.setRefreshToken(auth.refreshToken || "");
apps/user-uni/src/pages/WechatLoginPage.vue:0386 |     authStorage.setAuth(auth);
apps/user-uni/src/pages/WechatLoginPage.vue:0387 |     authStore.applyAuth(auth);
apps/user-uni/src/pages/WechatLoginPage.vue:0388 |   } catch {
apps/user-uni/src/pages/WechatLoginPage.vue:0389 |     throw new Error("TOKEN_SAVE_FAILED");
apps/user-uni/src/pages/WechatLoginPage.vue:0390 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0391 |   const targetRole = defaultRole(auth.roles);
apps/user-uni/src/pages/WechatLoginPage.vue:0392 |   if (userStore.currentRole !== targetRole) await userStore.switchRole(targetRole);
apps/user-uni/src/pages/WechatLoginPage.vue:0393 |   pendingInviteCode.value = "";
apps/user-uni/src/pages/WechatLoginPage.vue:0394 |   inviteStatus.value = "empty";
apps/user-uni/src/pages/WechatLoginPage.vue:0395 |   uni.removeStorageSync("zhiqiyun.promotion.pending-referral.v1");
apps/user-uni/src/pages/WechatLoginPage.vue:0396 |   trackLogin(auth.isNewUser ? "register_success" : "login_success", { method: mode.value, isNewUser: Boolean(auth.isNewUser) });
apps/user-uni/src/pages/WechatLoginPage.vue:0397 |   if (auth.isNewUser) {
apps/user-uni/src/pages/WechatLoginPage.vue:0398 |     loadingStep.value = "registering";
```

<div style="page-break-after: always;"></div>

### 第 20 页

**文件路径：** apps/user-uni/src/pages/WechatLoginPage.vue  
**代码说明：** App/小程序登录注册页面和交互状态。

```vue
apps/user-uni/src/pages/WechatLoginPage.vue:0399 |     const firstBenefit = auth.newcomerBenefits?.find(item => item.title || item.description);
apps/user-uni/src/pages/WechatLoginPage.vue:0400 |     benefitText.value = firstBenefit?.title || firstBenefit?.description || "新人体验权益已到账";
apps/user-uni/src/pages/WechatLoginPage.vue:0401 |     busy.value = false;
apps/user-uni/src/pages/WechatLoginPage.vue:0402 |     viewState.value = "success";
apps/user-uni/src/pages/WechatLoginPage.vue:0403 |     return;
apps/user-uni/src/pages/WechatLoginPage.vue:0404 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0405 |   if (auth.inviteBindStatus === "ignored_existing" && sourceParams.inviteCode) {
apps/user-uni/src/pages/WechatLoginPage.vue:0406 |     showToast("当前账号已注册，邀请码仅适用于新用户");
apps/user-uni/src/pages/WechatLoginPage.vue:0407 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0408 |   loadingStep.value = "entering";
apps/user-uni/src/pages/WechatLoginPage.vue:0409 |   busy.value = false;
apps/user-uni/src/pages/WechatLoginPage.vue:0410 |   redirectAfterAuth(redirectInfo, targetRole, () => showToast("页面打开失败，请重试", "error"));
apps/user-uni/src/pages/WechatLoginPage.vue:0411 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0412 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0413 | function errorPayloadCode(error: unknown): string {
apps/user-uni/src/pages/WechatLoginPage.vue:0414 |   if (error instanceof ApiClientError) {
apps/user-uni/src/pages/WechatLoginPage.vue:0415 |     const payload = error.payload && typeof error.payload === "object" ? error.payload as Record<string, unknown> : {};
apps/user-uni/src/pages/WechatLoginPage.vue:0416 |     return String(error.apiCode || payload.code || payload.errorCode || "").toUpperCase();
apps/user-uni/src/pages/WechatLoginPage.vue:0417 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0418 |   if (error instanceof Error) return error.message.toUpperCase();
apps/user-uni/src/pages/WechatLoginPage.vue:0419 |   return "";
apps/user-uni/src/pages/WechatLoginPage.vue:0420 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0421 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0422 | function errorPayloadValue(error: unknown, key: string): string {
apps/user-uni/src/pages/WechatLoginPage.vue:0423 |   if (!(error instanceof ApiClientError) || !error.payload || typeof error.payload !== "object") return "";
apps/user-uni/src/pages/WechatLoginPage.vue:0424 |   const payload = error.payload as Record<string, unknown>;
apps/user-uni/src/pages/WechatLoginPage.vue:0425 |   return String(payload[key] || "").trim();
apps/user-uni/src/pages/WechatLoginPage.vue:0426 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0427 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0428 | function errorStatusCode(error: unknown): number {
apps/user-uni/src/pages/WechatLoginPage.vue:0429 |   return error instanceof ApiClientError ? error.statusCode : 0;
apps/user-uni/src/pages/WechatLoginPage.vue:0430 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0431 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0432 | function handleLoginError(error: unknown, method: LoginMode) {
apps/user-uni/src/pages/WechatLoginPage.vue:0433 |   const code = errorPayloadCode(error);
apps/user-uni/src/pages/WechatLoginPage.vue:0434 |   const status = errorStatusCode(error);
apps/user-uni/src/pages/WechatLoginPage.vue:0435 |   trackLogin("login_failed", { method, code: code.slice(0, 40), status });
apps/user-uni/src/pages/WechatLoginPage.vue:0436 |   if (code.includes("SMS_CODE_INVALID") || code.includes("验证码错误")) {
apps/user-uni/src/pages/WechatLoginPage.vue:0437 |     smsCode.value = "";
apps/user-uni/src/pages/WechatLoginPage.vue:0438 |     smsError.value = "验证码错误，请重新输入";
apps/user-uni/src/pages/WechatLoginPage.vue:0439 |     showToast(smsError.value, "error");
apps/user-uni/src/pages/WechatLoginPage.vue:0440 |     return;
apps/user-uni/src/pages/WechatLoginPage.vue:0441 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0442 |   if (code.includes("SMS_CODE_EXPIRED") || code.includes("验证码过期")) {
apps/user-uni/src/pages/WechatLoginPage.vue:0443 |     smsCode.value = "";
apps/user-uni/src/pages/WechatLoginPage.vue:0444 |     smsError.value = "验证码已过期，请重新获取";
apps/user-uni/src/pages/WechatLoginPage.vue:0445 |     showToast(smsError.value, "error");
apps/user-uni/src/pages/WechatLoginPage.vue:0446 |     return;
apps/user-uni/src/pages/WechatLoginPage.vue:0447 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0448 |   if (code.includes("AUTH_ACCOUNT_MERGE_REQUIRED")) {
```

<div style="page-break-after: always;"></div>

### 第 21 页

**文件路径：** apps/user-uni/src/pages/WechatLoginPage.vue  
**代码说明：** App/小程序登录注册页面和交互状态。

```vue
apps/user-uni/src/pages/WechatLoginPage.vue:0449 |     const mergeRequestId = errorPayloadValue(error, "mergeRequestId");
apps/user-uni/src/pages/WechatLoginPage.vue:0450 |     showToast(mergeRequestId ? `账号需要人工合并，工单号：${mergeRequestId}` : "账号需要人工合并，请联系客服处理", "error");
apps/user-uni/src/pages/WechatLoginPage.vue:0451 |     return;
apps/user-uni/src/pages/WechatLoginPage.vue:0452 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0453 |   if (code.includes("ACCOUNT_FROZEN") || status === 423) return showErrorState("frozen");
apps/user-uni/src/pages/WechatLoginPage.vue:0454 |   if (code.includes("ACCOUNT_DEACTIVATED")) return showErrorState("deactivated");
apps/user-uni/src/pages/WechatLoginPage.vue:0455 |   if (code.includes("SYSTEM_MAINTENANCE")) return showErrorState("maintenance");
apps/user-uni/src/pages/WechatLoginPage.vue:0456 |   if (status === 503 || code.includes("AUTH_SESSION_UNAVAILABLE")) return showErrorState("service");
apps/user-uni/src/pages/WechatLoginPage.vue:0457 |   if (method === "wechat" && status === 502 && (code.includes("WECHAT") || code.includes("CODE2SESSION"))) {
apps/user-uni/src/pages/WechatLoginPage.vue:0458 |     showToast("快捷登录凭证无效或已过期，请重新进行手机号快捷登录", "error");
apps/user-uni/src/pages/WechatLoginPage.vue:0459 |     return;
apps/user-uni/src/pages/WechatLoginPage.vue:0460 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0461 |   if (code.includes("TOKEN_SAVE_FAILED")) return showErrorState("token");
apps/user-uni/src/pages/WechatLoginPage.vue:0462 |   if (
apps/user-uni/src/pages/WechatLoginPage.vue:0463 |     (error instanceof ApiClientError && status === 0)
apps/user-uni/src/pages/WechatLoginPage.vue:0464 |     || code.includes("NETWORK")
apps/user-uni/src/pages/WechatLoginPage.vue:0465 |     || code.includes("REQUEST:FAIL")
apps/user-uni/src/pages/WechatLoginPage.vue:0466 |     || code.includes("CONNECTION")
apps/user-uni/src/pages/WechatLoginPage.vue:0467 |   ) return showErrorState("network", error instanceof Error ? error.message : "请求未能到达服务器");
apps/user-uni/src/pages/WechatLoginPage.vue:0468 |   if (code.includes("TIMEOUT")) return showErrorState("timeout");
apps/user-uni/src/pages/WechatLoginPage.vue:0469 |   if (method === "password") {
apps/user-uni/src/pages/WechatLoginPage.vue:0470 |     password.value = "";
apps/user-uni/src/pages/WechatLoginPage.vue:0471 |     passwordError.value = status === 404 ? "账号不存在，请使用验证码登录" : "账号或密码不正确，请重试";
apps/user-uni/src/pages/WechatLoginPage.vue:0472 |     showToast(passwordError.value, "error");
apps/user-uni/src/pages/WechatLoginPage.vue:0473 |     return;
apps/user-uni/src/pages/WechatLoginPage.vue:0474 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0475 |   showToast(error instanceof Error ? error.message : "登录失败，请重试", "error");
apps/user-uni/src/pages/WechatLoginPage.vue:0476 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0477 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0478 | function showErrorState(kind: LoginErrorState, detail = "") {
apps/user-uni/src/pages/WechatLoginPage.vue:0479 |   errorState.value = kind;
apps/user-uni/src/pages/WechatLoginPage.vue:0480 |   errorDetail.value = detail.slice(0, 180);
apps/user-uni/src/pages/WechatLoginPage.vue:0481 |   viewState.value = "error";
apps/user-uni/src/pages/WechatLoginPage.vue:0482 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0483 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0484 | async function runLogin(method: LoginMode, task: () => Promise<AuthFlowResponse>) {
apps/user-uni/src/pages/WechatLoginPage.vue:0485 |   if (busy.value) return;
apps/user-uni/src/pages/WechatLoginPage.vue:0486 |   const version = ++requestVersion;
apps/user-uni/src/pages/WechatLoginPage.vue:0487 |   busy.value = true;
apps/user-uni/src/pages/WechatLoginPage.vue:0488 |   loadingStep.value = method === "wechat" ? "authorizing" : "validating";
apps/user-uni/src/pages/WechatLoginPage.vue:0489 |   try {
apps/user-uni/src/pages/WechatLoginPage.vue:0490 |     const auth = await task();
apps/user-uni/src/pages/WechatLoginPage.vue:0491 |     if (destroyed || version !== requestVersion) return;
apps/user-uni/src/pages/WechatLoginPage.vue:0492 |     loadingStep.value = auth.isNewUser ? "registering" : "logging_in";
apps/user-uni/src/pages/WechatLoginPage.vue:0493 |     await completeAuth(auth, version);
apps/user-uni/src/pages/WechatLoginPage.vue:0494 |   } catch (error) {
apps/user-uni/src/pages/WechatLoginPage.vue:0495 |     if (!destroyed && version === requestVersion) handleLoginError(error, method);
apps/user-uni/src/pages/WechatLoginPage.vue:0496 |   } finally {
apps/user-uni/src/pages/WechatLoginPage.vue:0497 |     if (!destroyed && version === requestVersion) busy.value = false;
apps/user-uni/src/pages/WechatLoginPage.vue:0498 |   }
```

<div style="page-break-after: always;"></div>

### 第 22 页

**文件路径：** apps/user-uni/src/pages/WechatLoginPage.vue  
**代码说明：** App/小程序登录注册页面和交互状态。

```vue
apps/user-uni/src/pages/WechatLoginPage.vue:0499 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0500 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0501 | function requestWechatLoginCode() {
apps/user-uni/src/pages/WechatLoginPage.vue:0502 |   return new Promise<string>((resolve, reject) => {
apps/user-uni/src/pages/WechatLoginPage.vue:0503 |     uni.login({
apps/user-uni/src/pages/WechatLoginPage.vue:0504 |       provider: "weixin",
apps/user-uni/src/pages/WechatLoginPage.vue:0505 |       success: result => result.code ? resolve(result.code) : reject(new Error(`小程序登录凭证获取失败：${result.errMsg || "未返回 code"}`)),
apps/user-uni/src/pages/WechatLoginPage.vue:0506 |       fail: result => reject(new Error(`小程序登录凭证获取失败：${result.errMsg || "未知错误"}`)),
apps/user-uni/src/pages/WechatLoginPage.vue:0507 |     });
apps/user-uni/src/pages/WechatLoginPage.vue:0508 |   });
apps/user-uni/src/pages/WechatLoginPage.vue:0509 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0510 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0511 | function onWechatButtonClick() {
apps/user-uni/src/pages/WechatLoginPage.vue:0512 |   if (!ensureAgreement()) return;
apps/user-uni/src/pages/WechatLoginPage.vue:0513 |   trackLogin("wechat_login_click");
apps/user-uni/src/pages/WechatLoginPage.vue:0514 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0515 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0516 | async function onGetPhoneNumber(event: unknown) {
apps/user-uni/src/pages/WechatLoginPage.vue:0517 |   if (!ensureAgreement() || busy.value) return;
apps/user-uni/src/pages/WechatLoginPage.vue:0518 |   const detail = (event && typeof event === "object" && "detail" in event ? (event as { detail?: Record<string, unknown> }).detail : {}) || {};
apps/user-uni/src/pages/WechatLoginPage.vue:0519 |   const phoneCode = String(detail.code || "").trim();
apps/user-uni/src/pages/WechatLoginPage.vue:0520 |   const errMsg = String(detail.errMsg || "");
apps/user-uni/src/pages/WechatLoginPage.vue:0521 |   if (!phoneCode || !errMsg.toLowerCase().includes("ok")) {
apps/user-uni/src/pages/WechatLoginPage.vue:0522 |     authorizationSheetVisible.value = true;
apps/user-uni/src/pages/WechatLoginPage.vue:0523 |     trackLogin("phone_auth_cancel");
apps/user-uni/src/pages/WechatLoginPage.vue:0524 |     return;
apps/user-uni/src/pages/WechatLoginPage.vue:0525 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0526 |   authorizationSheetVisible.value = false;
apps/user-uni/src/pages/WechatLoginPage.vue:0527 |   trackLogin("phone_auth_success");
apps/user-uni/src/pages/WechatLoginPage.vue:0528 |   await runLogin("wechat", async () => {
apps/user-uni/src/pages/WechatLoginPage.vue:0529 |     loadingStep.value = "authorizing";
apps/user-uni/src/pages/WechatLoginPage.vue:0530 |     const wxLoginCode = await requestWechatLoginCode();
apps/user-uni/src/pages/WechatLoginPage.vue:0531 |     loadingStep.value = "validating";
apps/user-uni/src/pages/WechatLoginPage.vue:0532 |     return loginAPI.wechatPhoneLogin({ wxLoginCode, phoneCode, ...attribution("wechat") });
apps/user-uni/src/pages/WechatLoginPage.vue:0533 |   });
apps/user-uni/src/pages/WechatLoginPage.vue:0534 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0535 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0536 | function useSmsAfterAuthorizationFailure() {
apps/user-uni/src/pages/WechatLoginPage.vue:0537 |   authorizationSheetVisible.value = false;
apps/user-uni/src/pages/WechatLoginPage.vue:0538 |   switchMode("sms");
apps/user-uni/src/pages/WechatLoginPage.vue:0539 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0540 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0541 | async function sendSmsCode() {
apps/user-uni/src/pages/WechatLoginPage.vue:0542 |   if (smsSending.value || countdown.value > 0 || busy.value) return;
apps/user-uni/src/pages/WechatLoginPage.vue:0543 |   mobile.value = normalizeMobile(mobile.value);
apps/user-uni/src/pages/WechatLoginPage.vue:0544 |   if (!validMobile(mobile.value)) {
apps/user-uni/src/pages/WechatLoginPage.vue:0545 |     mobileError.value = mobile.value.length === 11 ? "请输入正确的手机号" : "请输入11位手机号";
apps/user-uni/src/pages/WechatLoginPage.vue:0546 |     return;
apps/user-uni/src/pages/WechatLoginPage.vue:0547 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0548 |   smsSending.value = true;
```

<div style="page-break-after: always;"></div>

### 第 23 页

**文件路径：** apps/user-uni/src/pages/WechatLoginPage.vue  
**代码说明：** App/小程序登录注册页面和交互状态。

```vue
apps/user-uni/src/pages/WechatLoginPage.vue:0549 |   try {
apps/user-uni/src/pages/WechatLoginPage.vue:0550 |     const result = await loginAPI.sendSms(mobile.value);
apps/user-uni/src/pages/WechatLoginPage.vue:0551 |     countdown.value = Math.max(1, result.retryAfterSeconds || 60);
apps/user-uni/src/pages/WechatLoginPage.vue:0552 |     startCountdown();
apps/user-uni/src/pages/WechatLoginPage.vue:0553 |     trackLogin("sms_send_success");
apps/user-uni/src/pages/WechatLoginPage.vue:0554 |     showToast("验证码已发送", "success");
apps/user-uni/src/pages/WechatLoginPage.vue:0555 |   } catch (error) {
apps/user-uni/src/pages/WechatLoginPage.vue:0556 |     const code = errorPayloadCode(error);
apps/user-uni/src/pages/WechatLoginPage.vue:0557 |     if (code.includes("TOO_FREQUENT") || errorStatusCode(error) === 429) {
apps/user-uni/src/pages/WechatLoginPage.vue:0558 |       showToast("发送过于频繁，请稍后再试", "error");
apps/user-uni/src/pages/WechatLoginPage.vue:0559 |     } else {
apps/user-uni/src/pages/WechatLoginPage.vue:0560 |       showToast("验证码发送失败，请重试", "error");
apps/user-uni/src/pages/WechatLoginPage.vue:0561 |     }
apps/user-uni/src/pages/WechatLoginPage.vue:0562 |   } finally {
apps/user-uni/src/pages/WechatLoginPage.vue:0563 |     smsSending.value = false;
apps/user-uni/src/pages/WechatLoginPage.vue:0564 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0565 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0566 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0567 | function startCountdown() {
apps/user-uni/src/pages/WechatLoginPage.vue:0568 |   if (countdownTimer) clearInterval(countdownTimer);
apps/user-uni/src/pages/WechatLoginPage.vue:0569 |   countdownTimer = setInterval(() => {
apps/user-uni/src/pages/WechatLoginPage.vue:0570 |     countdown.value = Math.max(0, countdown.value - 1);
apps/user-uni/src/pages/WechatLoginPage.vue:0571 |     if (countdown.value === 0 && countdownTimer) {
apps/user-uni/src/pages/WechatLoginPage.vue:0572 |       clearInterval(countdownTimer);
apps/user-uni/src/pages/WechatLoginPage.vue:0573 |       countdownTimer = null;
apps/user-uni/src/pages/WechatLoginPage.vue:0574 |     }
apps/user-uni/src/pages/WechatLoginPage.vue:0575 |   }, 1000);
apps/user-uni/src/pages/WechatLoginPage.vue:0576 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0577 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0578 | async function loginWithSms() {
apps/user-uni/src/pages/WechatLoginPage.vue:0579 |   if (!ensureAgreement()) return;
apps/user-uni/src/pages/WechatLoginPage.vue:0580 |   mobile.value = normalizeMobile(mobile.value);
apps/user-uni/src/pages/WechatLoginPage.vue:0581 |   if (!validMobile(mobile.value)) {
apps/user-uni/src/pages/WechatLoginPage.vue:0582 |     mobileError.value = "请输入正确的11位手机号";
apps/user-uni/src/pages/WechatLoginPage.vue:0583 |     return;
apps/user-uni/src/pages/WechatLoginPage.vue:0584 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0585 |   if (!/^\d{6}$/.test(smsCode.value)) {
apps/user-uni/src/pages/WechatLoginPage.vue:0586 |     smsError.value = "请输入6位验证码";
apps/user-uni/src/pages/WechatLoginPage.vue:0587 |     return;
apps/user-uni/src/pages/WechatLoginPage.vue:0588 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0589 |   await runLogin("sms", () => loginAPI.smsLogin({ mobile: mobile.value, smsCode: smsCode.value, ...attribution("sms") }));
apps/user-uni/src/pages/WechatLoginPage.vue:0590 |   if (viewState.value !== "error" && !smsError.value) trackLogin("sms_login_success");
apps/user-uni/src/pages/WechatLoginPage.vue:0591 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0592 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0593 | async function loginWithPassword() {
apps/user-uni/src/pages/WechatLoginPage.vue:0594 |   if (!agreementAccepted.value) {
apps/user-uni/src/pages/WechatLoginPage.vue:0595 |     agreementAccepted.value = true;
apps/user-uni/src/pages/WechatLoginPage.vue:0596 |     agreementHighlight.value = false;
apps/user-uni/src/pages/WechatLoginPage.vue:0597 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0598 |   if (!account.value.trim()) {
```

<div style="page-break-after: always;"></div>

### 第 24 页

**文件路径：** apps/user-uni/src/pages/WechatLoginPage.vue  
**代码说明：** App/小程序登录注册页面和交互状态。

```vue
apps/user-uni/src/pages/WechatLoginPage.vue:0599 |     accountError.value = "请输入手机号或账号";
apps/user-uni/src/pages/WechatLoginPage.vue:0600 |     return;
apps/user-uni/src/pages/WechatLoginPage.vue:0601 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0602 |   if (!password.value) {
apps/user-uni/src/pages/WechatLoginPage.vue:0603 |     passwordError.value = "请输入登录密码";
apps/user-uni/src/pages/WechatLoginPage.vue:0604 |     return;
apps/user-uni/src/pages/WechatLoginPage.vue:0605 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0606 |   await runLogin("password", () => loginAPI.passwordLogin(account.value.trim(), password.value, nextIdempotencyKey("password")));
apps/user-uni/src/pages/WechatLoginPage.vue:0607 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0608 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0609 | function forgotPassword() {
apps/user-uni/src/pages/WechatLoginPage.vue:0610 |   const digits = normalizeMobile(account.value);
apps/user-uni/src/pages/WechatLoginPage.vue:0611 |   if (validMobile(digits)) mobile.value = digits;
apps/user-uni/src/pages/WechatLoginPage.vue:0612 |   switchMode("sms");
apps/user-uni/src/pages/WechatLoginPage.vue:0613 |   showToast("请使用手机号验证码登录后在账号与安全中设置密码");
apps/user-uni/src/pages/WechatLoginPage.vue:0614 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0615 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0616 | function openInviteSheet() {
apps/user-uni/src/pages/WechatLoginPage.vue:0617 |   if (busy.value) return;
apps/user-uni/src/pages/WechatLoginPage.vue:0618 |   inviteDraft.value = pendingInviteCode.value;
apps/user-uni/src/pages/WechatLoginPage.vue:0619 |   inviteMessage.value = "";
apps/user-uni/src/pages/WechatLoginPage.vue:0620 |   inviteMessageTone.value = "info";
apps/user-uni/src/pages/WechatLoginPage.vue:0621 |   inviteSheetVisible.value = true;
apps/user-uni/src/pages/WechatLoginPage.vue:0622 |   trackLogin("invite_entry_click");
apps/user-uni/src/pages/WechatLoginPage.vue:0623 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0624 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0625 | function closeInviteSheet() {
apps/user-uni/src/pages/WechatLoginPage.vue:0626 |   inviteSheetVisible.value = false;
apps/user-uni/src/pages/WechatLoginPage.vue:0627 |   keyboardHeight.value = 0;
apps/user-uni/src/pages/WechatLoginPage.vue:0628 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0629 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0630 | function inviteStatusMessage(status: InviteStatus): string {
apps/user-uni/src/pages/WechatLoginPage.vue:0631 |   const messages: Partial<Record<InviteStatus, string>> = {
apps/user-uni/src/pages/WechatLoginPage.vue:0632 |     invalid: "邀请码无效，不影响正常登录注册",
apps/user-uni/src/pages/WechatLoginPage.vue:0633 |     expired: "邀请码已过期，不影响正常登录注册",
apps/user-uni/src/pages/WechatLoginPage.vue:0634 |     disabled: "邀请码已停用，不影响正常登录注册",
apps/user-uni/src/pages/WechatLoginPage.vue:0635 |     agent_frozen: "该邀请码暂不可用，不影响正常登录注册",
apps/user-uni/src/pages/WechatLoginPage.vue:0636 |   };
apps/user-uni/src/pages/WechatLoginPage.vue:0637 |   return messages[status] || "邀请码校验失败，不影响正常登录注册";
apps/user-uni/src/pages/WechatLoginPage.vue:0638 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0639 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0640 | async function validateInvite(code: string, carried: boolean) {
apps/user-uni/src/pages/WechatLoginPage.vue:0641 |   inviteValidating.value = true;
apps/user-uni/src/pages/WechatLoginPage.vue:0642 |   inviteStatus.value = "resolving";
apps/user-uni/src/pages/WechatLoginPage.vue:0643 |   try {
apps/user-uni/src/pages/WechatLoginPage.vue:0644 |     const result = await loginAPI.validateInvite(code);
apps/user-uni/src/pages/WechatLoginPage.vue:0645 |     if (result.valid) {
apps/user-uni/src/pages/WechatLoginPage.vue:0646 |       pendingInviteCode.value = code;
apps/user-uni/src/pages/WechatLoginPage.vue:0647 |       inviteStatus.value = carried ? "carried" : "filled";
apps/user-uni/src/pages/WechatLoginPage.vue:0648 |       inviteMessage.value = "邀请码有效";
```

<div style="page-break-after: always;"></div>

### 第 25 页

**文件路径：** apps/user-uni/src/pages/WechatLoginPage.vue  
**代码说明：** App/小程序登录注册页面和交互状态。

```vue
apps/user-uni/src/pages/WechatLoginPage.vue:0649 |       inviteMessageTone.value = "success";
apps/user-uni/src/pages/WechatLoginPage.vue:0650 |       trackLogin("invite_validate_success", { source: carried ? sourceParams.inviteSource : "manual" });
apps/user-uni/src/pages/WechatLoginPage.vue:0651 |       return true;
apps/user-uni/src/pages/WechatLoginPage.vue:0652 |     }
apps/user-uni/src/pages/WechatLoginPage.vue:0653 |     pendingInviteCode.value = code;
apps/user-uni/src/pages/WechatLoginPage.vue:0654 |     inviteStatus.value = result.status || "invalid";
apps/user-uni/src/pages/WechatLoginPage.vue:0655 |     inviteMessage.value = result.message || inviteStatusMessage(inviteStatus.value);
apps/user-uni/src/pages/WechatLoginPage.vue:0656 |     inviteMessageTone.value = "error";
apps/user-uni/src/pages/WechatLoginPage.vue:0657 |     trackLogin("invite_validate_failed", { status: inviteStatus.value });
apps/user-uni/src/pages/WechatLoginPage.vue:0658 |     return false;
apps/user-uni/src/pages/WechatLoginPage.vue:0659 |   } catch {
apps/user-uni/src/pages/WechatLoginPage.vue:0660 |     pendingInviteCode.value = code;
apps/user-uni/src/pages/WechatLoginPage.vue:0661 |     inviteStatus.value = "invalid";
apps/user-uni/src/pages/WechatLoginPage.vue:0662 |     inviteMessage.value = "邀请码暂时无法校验，不影响正常登录注册";
apps/user-uni/src/pages/WechatLoginPage.vue:0663 |     inviteMessageTone.value = "error";
apps/user-uni/src/pages/WechatLoginPage.vue:0664 |     return false;
apps/user-uni/src/pages/WechatLoginPage.vue:0665 |   } finally {
apps/user-uni/src/pages/WechatLoginPage.vue:0666 |     inviteValidating.value = false;
apps/user-uni/src/pages/WechatLoginPage.vue:0667 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0668 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0669 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0670 | async function confirmInvite() {
apps/user-uni/src/pages/WechatLoginPage.vue:0671 |   if (inviteValidating.value) return;
apps/user-uni/src/pages/WechatLoginPage.vue:0672 |   const code = inviteDraft.value.trim().toUpperCase().replace(/[^A-Z0-9_-]/g, "").slice(0, 32);
apps/user-uni/src/pages/WechatLoginPage.vue:0673 |   inviteDraft.value = code;
apps/user-uni/src/pages/WechatLoginPage.vue:0674 |   if (!code) return removeInvite();
apps/user-uni/src/pages/WechatLoginPage.vue:0675 |   if (await validateInvite(code, false)) closeInviteSheet();
apps/user-uni/src/pages/WechatLoginPage.vue:0676 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0677 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0678 | function removeInvite() {
apps/user-uni/src/pages/WechatLoginPage.vue:0679 |   pendingInviteCode.value = "";
apps/user-uni/src/pages/WechatLoginPage.vue:0680 |   inviteDraft.value = "";
apps/user-uni/src/pages/WechatLoginPage.vue:0681 |   inviteStatus.value = "empty";
apps/user-uni/src/pages/WechatLoginPage.vue:0682 |   sourceParams.inviteCode = "";
apps/user-uni/src/pages/WechatLoginPage.vue:0683 |   closeInviteSheet();
apps/user-uni/src/pages/WechatLoginPage.vue:0684 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0685 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0686 | function openAgreement(type: "user" | "privacy") {
apps/user-uni/src/pages/WechatLoginPage.vue:0687 |   agreementSheetTitle.value = type === "user" ? "用户协议" : "隐私政策";
apps/user-uni/src/pages/WechatLoginPage.vue:0688 |   agreementSheetContent.value = type === "user"
apps/user-uni/src/pages/WechatLoginPage.vue:0689 |     ? "请在使用知启云AI前仔细阅读用户协议。账号注册、登录、内容创作及企业功能应遵守平台规则。正式正文由运营后台发布并在小程序上线前完成审核。"
apps/user-uni/src/pages/WechatLoginPage.vue:0690 |     : "知启云AI仅在登录和提供服务所必需的范围内处理手机号、微信身份及账号信息，不会在前端保存微信 session_key，也不会在日志中输出密码、完整手机号或登录凭证。正式正文由运营后台发布并在小程序上线前完成审核。";
apps/user-uni/src/pages/WechatLoginPage.vue:0691 |   agreementSheetVisible.value = true;
apps/user-uni/src/pages/WechatLoginPage.vue:0692 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0693 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0694 | function acceptAgreementFromSheet() {
apps/user-uni/src/pages/WechatLoginPage.vue:0695 |   agreementAccepted.value = true;
apps/user-uni/src/pages/WechatLoginPage.vue:0696 |   agreementHighlight.value = false;
apps/user-uni/src/pages/WechatLoginPage.vue:0697 |   agreementSheetVisible.value = false;
apps/user-uni/src/pages/WechatLoginPage.vue:0698 | }
```

<div style="page-break-after: always;"></div>

### 第 26 页

**文件路径：** apps/user-uni/src/pages/WechatLoginPage.vue  
**代码说明：** App/小程序登录注册页面和交互状态。

```vue
apps/user-uni/src/pages/WechatLoginPage.vue:0699 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0700 | function togglePasswordAgreement() {
apps/user-uni/src/pages/WechatLoginPage.vue:0701 |   agreementAccepted.value = !agreementAccepted.value;
apps/user-uni/src/pages/WechatLoginPage.vue:0702 |   agreementHighlight.value = false;
apps/user-uni/src/pages/WechatLoginPage.vue:0703 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0704 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0705 | function showLoginHelp() {
apps/user-uni/src/pages/WechatLoginPage.vue:0706 |   uni.showModal({ title: "登录遇到问题？", content: "可切换手机号验证码登录；如账号被冻结或已注销，请联系平台客服处理。", confirmText: "联系客服", success: result => { if (result.confirm) contactService(); } });
apps/user-uni/src/pages/WechatLoginPage.vue:0707 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0708 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0709 | function contactService() {
apps/user-uni/src/pages/WechatLoginPage.vue:0710 |   uni.showModal({ title: "联系平台客服", content: "请通过知启云AI官方客服渠道反馈，并提供账号的脱敏手机号和问题发生时间。", showCancel: false });
apps/user-uni/src/pages/WechatLoginPage.vue:0711 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0712 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0713 | function enterProduct() {
apps/user-uni/src/pages/WechatLoginPage.vue:0714 |   viewState.value = "form";
apps/user-uni/src/pages/WechatLoginPage.vue:0715 |   redirectAfterAuth(redirectInfo, userStore.currentRole || "USER", () => showToast("页面打开失败，请重试", "error"));
apps/user-uni/src/pages/WechatLoginPage.vue:0716 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0717 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0718 | function handleErrorPrimary() {
apps/user-uni/src/pages/WechatLoginPage.vue:0719 |   if (errorState.value === "frozen" || errorState.value === "deactivated") {
apps/user-uni/src/pages/WechatLoginPage.vue:0720 |     contactService();
apps/user-uni/src/pages/WechatLoginPage.vue:0721 |     return;
apps/user-uni/src/pages/WechatLoginPage.vue:0722 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0723 |   viewState.value = "form";
apps/user-uni/src/pages/WechatLoginPage.vue:0724 |   showToast("请重新发起登录");
apps/user-uni/src/pages/WechatLoginPage.vue:0725 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0726 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0727 | function returnToAvailableLogin() {
apps/user-uni/src/pages/WechatLoginPage.vue:0728 |   viewState.value = "form";
apps/user-uni/src/pages/WechatLoginPage.vue:0729 |   switchMode(errorState.value === "network" || errorState.value === "timeout" ? "sms" : "password");
apps/user-uni/src/pages/WechatLoginPage.vue:0730 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0731 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0732 | function enterGuestBrowse() {
apps/user-uni/src/pages/WechatLoginPage.vue:0733 |   uni.switchTab({
apps/user-uni/src/pages/WechatLoginPage.vue:0734 |     url: "/pages/user/UserHomePage",
apps/user-uni/src/pages/WechatLoginPage.vue:0735 |     fail: () => uni.reLaunch({ url: "/pages/user/UserHomePage" }),
apps/user-uni/src/pages/WechatLoginPage.vue:0736 |   });
apps/user-uni/src/pages/WechatLoginPage.vue:0737 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0738 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0739 | const keyboardHandler = (result: { height?: number }) => {
apps/user-uni/src/pages/WechatLoginPage.vue:0740 |   keyboardHeight.value = Math.max(0, Number(result.height || 0));
apps/user-uni/src/pages/WechatLoginPage.vue:0741 |   if (keyboardHeight.value > 0) {
apps/user-uni/src/pages/WechatLoginPage.vue:0742 |     scrollTarget.value = inviteSheetVisible.value ? "auth-invite-input" : mode.value === "sms" ? "auth-code-input" : "auth-password-input";
apps/user-uni/src/pages/WechatLoginPage.vue:0743 |   } else {
apps/user-uni/src/pages/WechatLoginPage.vue:0744 |     scrollTarget.value = "";
apps/user-uni/src/pages/WechatLoginPage.vue:0745 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0746 | };
apps/user-uni/src/pages/WechatLoginPage.vue:0747 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0748 | onLoad(async options => {
```

<div style="page-break-after: always;"></div>

### 第 27 页

**文件路径：** apps/user-uni/src/pages/WechatLoginPage.vue  
**代码说明：** App/小程序登录注册页面和交互状态。

```vue
apps/user-uni/src/pages/WechatLoginPage.vue:0749 |   const query = (options || {}) as Record<string, unknown>;
apps/user-uni/src/pages/WechatLoginPage.vue:0750 |   sourceParams = parseLoginSource(query);
apps/user-uni/src/pages/WechatLoginPage.vue:0751 |   redirectInfo = parseRedirectInfo(query);
apps/user-uni/src/pages/WechatLoginPage.vue:0752 |   trackLogin("login_page_view", { hasInvite: Boolean(sourceParams.inviteCode), source: sourceParams.inviteSource });
apps/user-uni/src/pages/WechatLoginPage.vue:0753 |   if (sourceParams.inviteCode) {
apps/user-uni/src/pages/WechatLoginPage.vue:0754 |     pendingInviteCode.value = sourceParams.inviteCode;
apps/user-uni/src/pages/WechatLoginPage.vue:0755 |     await validateInvite(sourceParams.inviteCode, true);
apps/user-uni/src/pages/WechatLoginPage.vue:0756 |   }
apps/user-uni/src/pages/WechatLoginPage.vue:0757 |   if (typeof uni.onKeyboardHeightChange === "function") uni.onKeyboardHeightChange(keyboardHandler);
apps/user-uni/src/pages/WechatLoginPage.vue:0758 | });
apps/user-uni/src/pages/WechatLoginPage.vue:0759 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0760 | onUnload(() => {
apps/user-uni/src/pages/WechatLoginPage.vue:0761 |   destroyed = true;
apps/user-uni/src/pages/WechatLoginPage.vue:0762 |   requestVersion += 1;
apps/user-uni/src/pages/WechatLoginPage.vue:0763 |   if (countdownTimer) clearInterval(countdownTimer);
apps/user-uni/src/pages/WechatLoginPage.vue:0764 |   if (toastTimer) clearTimeout(toastTimer);
apps/user-uni/src/pages/WechatLoginPage.vue:0765 |   if (agreementTimer) clearTimeout(agreementTimer);
apps/user-uni/src/pages/WechatLoginPage.vue:0766 |   if (typeof uni.offKeyboardHeightChange === "function") uni.offKeyboardHeightChange(keyboardHandler);
apps/user-uni/src/pages/WechatLoginPage.vue:0767 | });
apps/user-uni/src/pages/WechatLoginPage.vue:0768 | </script>
apps/user-uni/src/pages/WechatLoginPage.vue:0769 | 
apps/user-uni/src/pages/WechatLoginPage.vue:0770 | <style scoped>
apps/user-uni/src/pages/WechatLoginPage.vue:0771 | .auth-auto-register { display: block; margin-top: 10px; color: #8c94a8; font-size: 11px; line-height: 20px; text-align: center; }
apps/user-uni/src/pages/WechatLoginPage.vue:0772 | .auth-divider { display: flex; align-items: center; gap: 7px; margin: 24px 0 2px; color: #8c94a8; font-size: 12px; line-height: 20px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0773 | .auth-divider view { height: 1px; flex: 1; background: #e3e8f5; }
apps/user-uni/src/pages/WechatLoginPage.vue:0774 | .auth-login-mode-button { width: 100%; min-height: 34px; margin: 0; padding: 5px 4px; box-sizing: border-box; border: 0; color: #4a6bff; background: transparent; font-size: 13px; line-height: 24px; font-weight: 500; }
apps/user-uni/src/pages/WechatLoginPage.vue:0775 | .auth-login-mode-button::after { display: none; }
apps/user-uni/src/pages/WechatLoginPage.vue:0776 | .auth-login-mode-button.muted { color: #697085; font-size: 12px; font-weight: 400; }
apps/user-uni/src/pages/WechatLoginPage.vue:0777 | .auth-login-mode-hover { opacity: .68; }
apps/user-uni/src/pages/WechatLoginPage.vue:0778 | .auth-invite-spacing { margin-top: 9px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0779 | .auth-agreement-spacing { margin-top: 18px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0780 | .auth-agreement-spacing.sms { margin-top: 16px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0781 | .auth-agreement-spacing.password { margin-top: 11px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0782 | .auth-help-spacing { margin-top: 22px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0783 | .auth-browse-entry { margin-top: 8px; color: #4a6bff; }
apps/user-uni/src/pages/WechatLoginPage.vue:0784 | .auth-help-spacing.sms { margin-top: 7px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0785 | .auth-mode-back { margin: 7px 0 5px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0786 | .auth-field-block { margin-bottom: 12px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0787 | .auth-field-label { display: block; margin-bottom: 6px; color: #181c28; font-size: 12px; line-height: 20px; font-weight: 500; }
apps/user-uni/src/pages/WechatLoginPage.vue:0788 | .auth-account-shell { height: 50px; box-sizing: border-box; overflow: hidden; border: 1px solid #e0e5f2; border-radius: 14px; background: #f2f7ff; }
apps/user-uni/src/pages/WechatLoginPage.vue:0789 | .auth-account-shell:focus-within { border: 1.5px solid #4a6bff; background: #fff; }
apps/user-uni/src/pages/WechatLoginPage.vue:0790 | .auth-account-shell.error { border-color: #eb404f; }
apps/user-uni/src/pages/WechatLoginPage.vue:0791 | .auth-account-input { width: 100%; height: 50px; padding: 0 15px; box-sizing: border-box; color: #181c28; font-size: 14px; line-height: 50px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0792 | .auth-field-error { display: block; margin-top: 5px; color: #eb404f; font-size: 10px; line-height: 16px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0793 | .auth-password-field { margin-bottom: 4px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0794 | .auth-password-shell { display: flex; align-items: center; height: 50px; box-sizing: border-box; overflow: hidden; border: 1px solid #e0e5f2; border-radius: 14px; background: #f2f7ff; }
apps/user-uni/src/pages/WechatLoginPage.vue:0795 | .auth-password-shell:focus-within { border: 1.5px solid #4a6bff; background: #fff; }
apps/user-uni/src/pages/WechatLoginPage.vue:0796 | .auth-password-shell.error { border-color: #eb404f; }
apps/user-uni/src/pages/WechatLoginPage.vue:0797 | .auth-password-input { min-width: 0; height: 50px; flex: 1; padding-left: 15px; color: #181c28; font-size: 14px; line-height: 50px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0798 | .auth-password-toggle { width: 52px; height: 50px; margin: 0; padding: 0; border: 0; color: #697085; background: transparent; font-size: 18px; line-height: 50px; }
```

<div style="page-break-after: always;"></div>

### 第 28 页

**文件路径：** apps/user-uni/src/pages/WechatLoginPage.vue；apps/user-uni/src/components/KnowledgeMiniChat.vue  
**代码说明：** App/小程序登录注册页面和交互状态。；知识库智能体对话、历史会话与引用展示。

```text
apps/user-uni/src/pages/WechatLoginPage.vue:0799 | .auth-password-toggle::after { display: none; }
apps/user-uni/src/pages/WechatLoginPage.vue:0800 | .auth-forgot { min-height: 34px; margin: 0; padding: 4px 0; border: 0; color: #4a6bff; background: transparent; font-size: 12px; line-height: 22px; text-align: right; }
apps/user-uni/src/pages/WechatLoginPage.vue:0801 | .auth-forgot::after { display: none; }
apps/user-uni/src/pages/WechatLoginPage.vue:0802 | .auth-password-submit { width: 100%; height: 50px; margin: 4px 0 0; padding: 0 16px; box-sizing: border-box; display: flex; align-items: center; justify-content: center; border: 0; border-radius: 15px; color: #fff; background: #4a6bff; box-shadow: 0 8px 18px rgba(46, 71, 204, 0.22); font-size: 15px; line-height: 24px; font-weight: 500; }
apps/user-uni/src/pages/WechatLoginPage.vue:0803 | .auth-password-submit.disabled { color: #9ca4b5; background: #d9deec; box-shadow: none; opacity: 1; }
apps/user-uni/src/pages/WechatLoginPage.vue:0804 | .auth-password-submit-pressed { background: #3f5be0; opacity: .96; }
apps/user-uni/src/pages/WechatLoginPage.vue:0805 | .auth-password-note { margin-top: 7px; padding: 8px 12px; border-radius: 12px; color: #697085; background: #f2f7ff; font-size: 11px; line-height: 24px; text-align: center; }
apps/user-uni/src/pages/WechatLoginPage.vue:0806 | .auth-password-agreement { display: flex; align-items: center; gap: 0; margin-top: 11px; padding: 4px 0; border-radius: 8px; transition: background 160ms ease; }
apps/user-uni/src/pages/WechatLoginPage.vue:0807 | .auth-password-agreement.highlight { padding: 5px 6px; background: #fff1f2; }
apps/user-uni/src/pages/WechatLoginPage.vue:0808 | .auth-password-agreement-toggle { min-width: 0; min-height: 36px; flex: 0 0 auto; display: flex; align-items: center; color: #697085; font-size: 11px; line-height: 20px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0809 | .auth-password-agreement-box { width: 18px; height: 18px; margin: 0 5px 0 0; box-sizing: border-box; border: 1px solid #b9c1d2; border-radius: 5px; color: #fff; background: #fff; font-size: 12px; line-height: 16px; font-weight: 700; text-align: center; }
apps/user-uni/src/pages/WechatLoginPage.vue:0810 | .auth-password-agreement-box.checked { border-color: #4a6bff; background: #4a6bff; }
apps/user-uni/src/pages/WechatLoginPage.vue:0811 | .auth-password-agreement-copy { min-width: 0; min-height: 36px; display: flex; align-items: center; flex-wrap: wrap; color: #697085; font-size: 11px; line-height: 20px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0812 | .auth-password-agreement-link { color: #4a6bff; font-weight: 500; }
apps/user-uni/src/pages/WechatLoginPage.vue:0813 | .auth-sheet-field { margin-bottom: 24px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0814 | .auth-sheet-label { display: block; margin-bottom: 7px; color: #181c28; font-size: 12px; line-height: 22px; font-weight: 500; }
apps/user-uni/src/pages/WechatLoginPage.vue:0815 | .auth-sheet-input { width: 100%; height: 50px; padding: 0 15px; box-sizing: border-box; border: 1px solid #d6dff1; border-radius: 14px; color: #181c28; background: #f5f8ff; font-size: 14px; line-height: 50px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0816 | .auth-invite-message { display: block; min-height: 20px; margin-top: 9px; color: #697085; font-size: 11px; line-height: 20px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0817 | .auth-invite-message.error { color: #eb404f; } .auth-invite-message.success { color: #18a06a; }
apps/user-uni/src/pages/WechatLoginPage.vue:0818 | .auth-permission-sheet { padding: 10px 0 0; text-align: center; }
apps/user-uni/src/pages/WechatLoginPage.vue:0819 | .auth-permission-icon { width: 72px; height: 72px; margin: 0 auto 17px; border-radius: 24px; color: #4a6bff; background: #edf2ff; font-size: 28px; line-height: 72px; font-weight: 700; }
apps/user-uni/src/pages/WechatLoginPage.vue:0820 | .auth-permission-title, .auth-permission-copy { display: block; }
apps/user-uni/src/pages/WechatLoginPage.vue:0821 | .auth-permission-title { color: #181c28; font-size: 21px; line-height: 32px; font-weight: 700; }
apps/user-uni/src/pages/WechatLoginPage.vue:0822 | .auth-permission-copy { margin: 8px 0 24px; color: #697085; font-size: 13px; line-height: 24px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0823 | .auth-retry-authorization { margin-top: 14px; color: #4a6bff !important; border: 1px solid #e0e5f2 !important; background: #fff !important; box-shadow: none !important; }
apps/user-uni/src/pages/WechatLoginPage.vue:0824 | .auth-agreement-document { max-height: 220px; margin-bottom: 22px; color: #697085; font-size: 13px; line-height: 24px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0825 | @media (max-width: 340px) {
apps/user-uni/src/pages/WechatLoginPage.vue:0826 |   .auth-auto-register { font-size: 10px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0827 |   .auth-divider { margin-top: 18px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0828 |   .auth-agreement-spacing { margin-top: 13px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0829 |   .auth-help-spacing { margin-top: 14px; }
apps/user-uni/src/pages/WechatLoginPage.vue:0830 | }
apps/user-uni/src/pages/WechatLoginPage.vue:0831 | </style>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0001 | <template>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0002 |   <view class="km-shell">
apps/user-uni/src/components/KnowledgeMiniChat.vue:0003 |     <view class="km-header">
apps/user-uni/src/components/KnowledgeMiniChat.vue:0004 |       <button v-if="embedded" class="km-back" type="button" @click="$emit('close')">←</button>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0005 |       <view class="km-title"><text>知识库问答</text><text>答案可追溯，引用可核验</text></view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0006 |       <button class="km-new" type="button" :disabled="!selectedAgentId" @click="createConversation">新对话</button>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0007 |     </view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0008 | 
apps/user-uni/src/components/KnowledgeMiniChat.vue:0009 |     <view v-if="loading" class="km-state"><text>正在同步知识库智能体…</text></view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0010 |     <view v-else-if="error && !agents.length" class="km-state error"><text>{{ error }}</text><button @click="load">重新加载</button></view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0011 |     <template v-else>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0012 |       <scroll-view class="km-agent-strip" scroll-x>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0013 |         <button v-for="agent in agents" :key="agent.id" :class="{ active: selectedAgentId === agent.id }" @click="selectAgent(agent.id)">
apps/user-uni/src/components/KnowledgeMiniChat.vue:0014 |           <text>{{ agent.name.slice(0, 1) }}</text><view><text>{{ agent.name }}</text><text>{{ agent.description || '知识库智能体' }}</text></view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0015 |         </button>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0016 |         <view v-if="!agents.length" class="km-no-agent"><text>暂无可用智能体</text><text>请先在 PC 端创建并绑定知识库</text></view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0017 |       </scroll-view>
```

<div style="page-break-after: always;"></div>

### 第 29 页

**文件路径：** apps/user-uni/src/components/KnowledgeMiniChat.vue  
**代码说明：** 知识库智能体对话、历史会话与引用展示。

```vue
apps/user-uni/src/components/KnowledgeMiniChat.vue:0018 | 
apps/user-uni/src/components/KnowledgeMiniChat.vue:0019 |       <view class="km-body">
apps/user-uni/src/components/KnowledgeMiniChat.vue:0020 |         <scroll-view class="km-history" scroll-y>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0021 |           <text class="km-section-label">历史记录</text>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0022 |           <button v-for="item in conversations" :key="item.id" :class="{ active: activeConversationId === item.id }" @click="openConversation(item.id)">
apps/user-uni/src/components/KnowledgeMiniChat.vue:0023 |             <text>{{ item.title }}</text><text>{{ formatDate(item.updatedAt) }}</text>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0024 |           </button>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0025 |           <text v-if="!conversations.length" class="km-history-empty">暂无历史对话</text>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0026 |         </scroll-view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0027 | 
apps/user-uni/src/components/KnowledgeMiniChat.vue:0028 |         <view class="km-chat">
apps/user-uni/src/components/KnowledgeMiniChat.vue:0029 |           <scroll-view class="km-messages" scroll-y :scroll-top="scrollTop">
apps/user-uni/src/components/KnowledgeMiniChat.vue:0030 |             <view v-if="!messages.length" class="km-empty">
apps/user-uni/src/components/KnowledgeMiniChat.vue:0031 |               <text class="km-empty-icon">KA</text><text class="km-empty-title">向企业知识库提问</text><text class="km-empty-copy">回答会展示引用文档、相似度和原文片段。</text>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0032 |               <view><button v-for="item in suggestions" :key="item" @click="draft = item">{{ item }}</button></view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0033 |             </view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0034 |             <view v-for="message in messages" :key="message.id" :class="['km-message', message.role]">
apps/user-uni/src/components/KnowledgeMiniChat.vue:0035 |               <text class="km-avatar">{{ message.role === 'user' ? '我' : agentInitial }}</text>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0036 |               <view><text class="km-bubble">{{ message.content || (generating ? '正在检索知识库…' : '') }}</text>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0037 |                 <view v-if="message.citations?.length" class="km-citations">
apps/user-uni/src/components/KnowledgeMiniChat.vue:0038 |                   <button v-for="citation in message.citations" :key="citation.id" @click="activeCitation = citation">[{{ citation.order }}] {{ citation.documentName }}</button>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0039 |                 </view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0040 |               </view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0041 |             </view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0042 |           </scroll-view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0043 | 
apps/user-uni/src/components/KnowledgeMiniChat.vue:0044 |           <view v-if="error" class="km-inline-error"><text>{{ error }}</text></view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0045 |           <view class="km-composer">
apps/user-uni/src/components/KnowledgeMiniChat.vue:0046 |             <textarea v-model="draft" maxlength="1000" :disabled="generating" auto-height placeholder="输入问题，答案将基于已绑定知识库生成" />
apps/user-uni/src/components/KnowledgeMiniChat.vue:0047 |             <view><text>{{ draft.length }}/1000</text><button v-if="generating" class="stop" @click="stopGeneration">停止</button><button v-else :disabled="!canSend" @click="send">发送</button></view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0048 |           </view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0049 |           <button v-if="lastQuestion && !generating" class="km-retry" type="button" @click="retryLast">重新回答上一个问题</button>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0050 |         </view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0051 |       </view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0052 |     </template>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0053 | 
apps/user-uni/src/components/KnowledgeMiniChat.vue:0054 |     <view v-if="activeCitation" class="km-mask" @click.self="activeCitation = null">
apps/user-uni/src/components/KnowledgeMiniChat.vue:0055 |       <view class="km-source-card">
apps/user-uni/src/components/KnowledgeMiniChat.vue:0056 |         <view><view><text>引用 [{{ activeCitation.order }}]</text><text>{{ activeCitation.documentName }}</text></view><button @click="activeCitation = null">×</button></view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0057 |         <text class="km-source-label">原文片段</text><text class="km-quote">{{ activeCitation.quote }}</text>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0058 |         <view class="km-source-meta"><text>相似度 {{ score(activeCitation.similarityScore) }}</text><text>Chunk {{ activeCitation.chunkId }}</text><text v-if="citationPage(activeCitation)">页码 {{ citationPage(activeCitation) }}</text></view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0059 |       </view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0060 |     </view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0061 |   </view>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0062 | </template>
apps/user-uni/src/components/KnowledgeMiniChat.vue:0063 | 
apps/user-uni/src/components/KnowledgeMiniChat.vue:0064 | <script setup lang="ts">
apps/user-uni/src/components/KnowledgeMiniChat.vue:0065 | import { computed, onMounted, ref } from "vue";
apps/user-uni/src/components/KnowledgeMiniChat.vue:0066 | import { miniKnowledgeAPI, startMiniKnowledgeRun, type KnowledgeRunHandle, type MiniKnowledgeAgent, type MiniKnowledgeCitation, type MiniKnowledgeConversation, type MiniKnowledgeMessage, type MiniKnowledgeRunResult, type MiniKnowledgeStreamEvent } from "../api/knowledge";
apps/user-uni/src/components/KnowledgeMiniChat.vue:0067 | import { authStorage } from "../api/client";
```

<div style="page-break-after: always;"></div>

### 第 30 页

**文件路径：** apps/user-uni/src/components/KnowledgeMiniChat.vue  
**代码说明：** 知识库智能体对话、历史会话与引用展示。

```vue
apps/user-uni/src/components/KnowledgeMiniChat.vue:0068 | import { requireAuth } from "../features/auth/gate";
apps/user-uni/src/components/KnowledgeMiniChat.vue:0069 | 
apps/user-uni/src/components/KnowledgeMiniChat.vue:0070 | defineProps<{ embedded?: boolean }>();
apps/user-uni/src/components/KnowledgeMiniChat.vue:0071 | defineEmits<{ close: [] }>();
apps/user-uni/src/components/KnowledgeMiniChat.vue:0072 | type ChatMessage = MiniKnowledgeMessage & { citations?: MiniKnowledgeCitation[] };
apps/user-uni/src/components/KnowledgeMiniChat.vue:0073 | 
apps/user-uni/src/components/KnowledgeMiniChat.vue:0074 | const agents = ref<MiniKnowledgeAgent[]>([]);
apps/user-uni/src/components/KnowledgeMiniChat.vue:0075 | const conversations = ref<MiniKnowledgeConversation[]>([]);
apps/user-uni/src/components/KnowledgeMiniChat.vue:0076 | const messages = ref<ChatMessage[]>([]);
apps/user-uni/src/components/KnowledgeMiniChat.vue:0077 | const selectedAgentId = ref("");
apps/user-uni/src/components/KnowledgeMiniChat.vue:0078 | const activeConversationId = ref("");
apps/user-uni/src/components/KnowledgeMiniChat.vue:0079 | const activeRunId = ref("");
apps/user-uni/src/components/KnowledgeMiniChat.vue:0080 | const activeCitation = ref<MiniKnowledgeCitation | null>(null);
apps/user-uni/src/components/KnowledgeMiniChat.vue:0081 | const draft = ref("");
apps/user-uni/src/components/KnowledgeMiniChat.vue:0082 | const lastQuestion = ref("");
apps/user-uni/src/components/KnowledgeMiniChat.vue:0083 | const loading = ref(false);
apps/user-uni/src/components/KnowledgeMiniChat.vue:0084 | const generating = ref(false);
apps/user-uni/src/components/KnowledgeMiniChat.vue:0085 | const error = ref("");
apps/user-uni/src/components/KnowledgeMiniChat.vue:0086 | const scrollTop = ref(0);
apps/user-uni/src/components/KnowledgeMiniChat.vue:0087 | let runHandle: KnowledgeRunHandle | null = null;
apps/user-uni/src/components/KnowledgeMiniChat.vue:0088 | const suggestions = ["请总结核心产品能力", "售后处理流程是什么？", "有哪些必须遵守的制度？"];
apps/user-uni/src/components/KnowledgeMiniChat.vue:0089 | const selectedAgent = computed(() => agents.value.find(item => item.id === selectedAgentId.value));
apps/user-uni/src/components/KnowledgeMiniChat.vue:0090 | const agentInitial = computed(() => selectedAgent.value?.name.slice(0, 1) || "AI");
apps/user-uni/src/components/KnowledgeMiniChat.vue:0091 | const canSend = computed(() => Boolean(draft.value.trim() && (selectedAgentId.value || !authStorage.getToken())));
apps/user-uni/src/components/KnowledgeMiniChat.vue:0092 | 
apps/user-uni/src/components/KnowledgeMiniChat.vue:0093 | onMounted(() => {
apps/user-uni/src/components/KnowledgeMiniChat.vue:0094 |   try { draft.value = String(uni.getStorageSync("zhiqiyun:web:chat-draft") || draft.value); } catch { /* optional draft */ }
apps/user-uni/src/components/KnowledgeMiniChat.vue:0095 |   if (authStorage.getToken()) void load();
apps/user-uni/src/components/KnowledgeMiniChat.vue:0096 | });
apps/user-uni/src/components/KnowledgeMiniChat.vue:0097 | 
apps/user-uni/src/components/KnowledgeMiniChat.vue:0098 | async function load() {
apps/user-uni/src/components/KnowledgeMiniChat.vue:0099 |   loading.value = true; error.value = "";
apps/user-uni/src/components/KnowledgeMiniChat.vue:0100 |   try {
apps/user-uni/src/components/KnowledgeMiniChat.vue:0101 |     agents.value = (await miniKnowledgeAPI.agents()).items || [];
apps/user-uni/src/components/KnowledgeMiniChat.vue:0102 |     if (!selectedAgentId.value && agents.value.length) selectedAgentId.value = agents.value[0].id;
apps/user-uni/src/components/KnowledgeMiniChat.vue:0103 |     await loadConversations();
apps/user-uni/src/components/KnowledgeMiniChat.vue:0104 |   } catch (reason) { error.value = errorMessage(reason); }
apps/user-uni/src/components/KnowledgeMiniChat.vue:0105 |   finally { loading.value = false; }
apps/user-uni/src/components/KnowledgeMiniChat.vue:0106 | }
apps/user-uni/src/components/KnowledgeMiniChat.vue:0107 | async function loadConversations() {
apps/user-uni/src/components/KnowledgeMiniChat.vue:0108 |   conversations.value = selectedAgentId.value ? (await miniKnowledgeAPI.conversations(selectedAgentId.value)).items || [] : [];
apps/user-uni/src/components/KnowledgeMiniChat.vue:0109 |   if (!activeConversationId.value && conversations.value.length) await openConversation(conversations.value[0].id);
apps/user-uni/src/components/KnowledgeMiniChat.vue:0110 | }
apps/user-uni/src/components/KnowledgeMiniChat.vue:0111 | async function selectAgent(id: string) { selectedAgentId.value = id; activeConversationId.value = ""; messages.value = []; await loadConversations(); }
apps/user-uni/src/components/KnowledgeMiniChat.vue:0112 | async function openConversation(id: string) {
apps/user-uni/src/components/KnowledgeMiniChat.vue:0113 |   activeConversationId.value = id; error.value = "";
apps/user-uni/src/components/KnowledgeMiniChat.vue:0114 |   try { messages.value = ((await miniKnowledgeAPI.messages(id)).items || []).map(item => ({ ...item })); scrollBottom(); }
apps/user-uni/src/components/KnowledgeMiniChat.vue:0115 |   catch (reason) { error.value = errorMessage(reason); }
apps/user-uni/src/components/KnowledgeMiniChat.vue:0116 | }
apps/user-uni/src/components/KnowledgeMiniChat.vue:0117 | async function createConversation() {
```

<div style="page-break-after: always;"></div>

## 后 30 页：服务端与数据库核心代码

### 第 31 页

**文件路径：** backend-go/cmd/api/main.go；backend-go/internal/infra/clients.go  
**代码说明：** Go 服务启动、配置加载与优雅关闭。；PostgreSQL 与 Redis 基础设施客户端初始化。

```text
backend-go/cmd/api/main.go:0001 | package main
backend-go/cmd/api/main.go:0002 | 
backend-go/cmd/api/main.go:0003 | import (
backend-go/cmd/api/main.go:0004 | 	"context"
backend-go/cmd/api/main.go:0005 | 	"log"
backend-go/cmd/api/main.go:0006 | 	"time"
backend-go/cmd/api/main.go:0007 | 
backend-go/cmd/api/main.go:0008 | 	"xianzhi-ai/backend-go/internal/config"
backend-go/cmd/api/main.go:0009 | 	"xianzhi-ai/backend-go/internal/httpserver"
backend-go/cmd/api/main.go:0010 | 	"xianzhi-ai/backend-go/internal/infra"
backend-go/cmd/api/main.go:0011 | )
backend-go/cmd/api/main.go:0012 | 
backend-go/cmd/api/main.go:0013 | func main() {
backend-go/cmd/api/main.go:0014 | 	cfg := config.Load()
backend-go/cmd/api/main.go:0015 | 	if err := cfg.ValidateProduction(); err != nil {
backend-go/cmd/api/main.go:0016 | 		log.Fatalf("invalid production config: %v", err)
backend-go/cmd/api/main.go:0017 | 	}
backend-go/cmd/api/main.go:0018 | 	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
backend-go/cmd/api/main.go:0019 | 	clients, err := infra.Open(ctx, cfg)
backend-go/cmd/api/main.go:0020 | 	cancel()
backend-go/cmd/api/main.go:0021 | 	if err != nil {
backend-go/cmd/api/main.go:0022 | 		if cfg.IsProduction() {
backend-go/cmd/api/main.go:0023 | 			log.Fatalf("production infrastructure unavailable: %v", err)
backend-go/cmd/api/main.go:0024 | 		}
backend-go/cmd/api/main.go:0025 | 		log.Printf("infrastructure clients disabled: %v", err)
backend-go/cmd/api/main.go:0026 | 	} else {
backend-go/cmd/api/main.go:0027 | 		defer func() {
backend-go/cmd/api/main.go:0028 | 			if err := clients.Close(); err != nil {
backend-go/cmd/api/main.go:0029 | 				log.Printf("close infrastructure clients: %v", err)
backend-go/cmd/api/main.go:0030 | 			}
backend-go/cmd/api/main.go:0031 | 		}()
backend-go/cmd/api/main.go:0032 | 	}
backend-go/cmd/api/main.go:0033 | 	if cfg.IsProduction() && (clients == nil || clients.DB == nil || clients.Redis == nil) {
backend-go/cmd/api/main.go:0034 | 		log.Fatal("production requires PostgreSQL and Redis infrastructure")
backend-go/cmd/api/main.go:0035 | 	}
backend-go/cmd/api/main.go:0036 | 	var server = httpserver.New(cfg)
backend-go/cmd/api/main.go:0037 | 	if clients != nil {
backend-go/cmd/api/main.go:0038 | 		server = httpserver.NewWithInfrastructure(cfg, clients.DB, clients.Redis)
backend-go/cmd/api/main.go:0039 | 	}
backend-go/cmd/api/main.go:0040 | 	log.Printf("xianzhi-ai go gin api listening on %s", cfg.Addr)
backend-go/cmd/api/main.go:0041 | 	if err := server.ListenAndServe(); err != nil {
backend-go/cmd/api/main.go:0042 | 		log.Fatal(err)
backend-go/cmd/api/main.go:0043 | 	}
backend-go/cmd/api/main.go:0044 | }
backend-go/internal/infra/clients.go:0001 | package infra
backend-go/internal/infra/clients.go:0002 | 
backend-go/internal/infra/clients.go:0003 | import (
backend-go/internal/infra/clients.go:0004 | 	"context"
backend-go/internal/infra/clients.go:0005 | 	"database/sql"
backend-go/internal/infra/clients.go:0006 | 
```

<div style="page-break-after: always;"></div>

### 第 32 页

**文件路径：** backend-go/internal/infra/clients.go  
**代码说明：** PostgreSQL 与 Redis 基础设施客户端初始化。

```go
backend-go/internal/infra/clients.go:0007 | 	_ "github.com/jackc/pgx/v5/stdlib"
backend-go/internal/infra/clients.go:0008 | 	"github.com/redis/go-redis/v9"
backend-go/internal/infra/clients.go:0009 | 
backend-go/internal/infra/clients.go:0010 | 	"xianzhi-ai/backend-go/internal/config"
backend-go/internal/infra/clients.go:0011 | )
backend-go/internal/infra/clients.go:0012 | 
backend-go/internal/infra/clients.go:0013 | type Clients struct {
backend-go/internal/infra/clients.go:0014 | 	DB    *sql.DB
backend-go/internal/infra/clients.go:0015 | 	Redis *redis.Client
backend-go/internal/infra/clients.go:0016 | }
backend-go/internal/infra/clients.go:0017 | 
backend-go/internal/infra/clients.go:0018 | func Open(ctx context.Context, cfg config.Config) (*Clients, error) {
backend-go/internal/infra/clients.go:0019 | 	clients := &Clients{}
backend-go/internal/infra/clients.go:0020 | 	if cfg.DatabaseURL != "" {
backend-go/internal/infra/clients.go:0021 | 		db, err := sql.Open("pgx", cfg.DatabaseURL)
backend-go/internal/infra/clients.go:0022 | 		if err != nil {
backend-go/internal/infra/clients.go:0023 | 			return nil, err
backend-go/internal/infra/clients.go:0024 | 		}
backend-go/internal/infra/clients.go:0025 | 		if err := db.PingContext(ctx); err != nil {
backend-go/internal/infra/clients.go:0026 | 			_ = db.Close()
backend-go/internal/infra/clients.go:0027 | 			return nil, err
backend-go/internal/infra/clients.go:0028 | 		}
backend-go/internal/infra/clients.go:0029 | 		clients.DB = db
backend-go/internal/infra/clients.go:0030 | 	}
backend-go/internal/infra/clients.go:0031 | 	if cfg.RedisURL != "" {
backend-go/internal/infra/clients.go:0032 | 		options, err := redis.ParseURL(cfg.RedisURL)
backend-go/internal/infra/clients.go:0033 | 		if err != nil {
backend-go/internal/infra/clients.go:0034 | 			_ = clients.Close()
backend-go/internal/infra/clients.go:0035 | 			return nil, err
backend-go/internal/infra/clients.go:0036 | 		}
backend-go/internal/infra/clients.go:0037 | 		clients.Redis = redis.NewClient(options)
backend-go/internal/infra/clients.go:0038 | 		if err := clients.Redis.Ping(ctx).Err(); err != nil {
backend-go/internal/infra/clients.go:0039 | 			_ = clients.Close()
backend-go/internal/infra/clients.go:0040 | 			return nil, err
backend-go/internal/infra/clients.go:0041 | 		}
backend-go/internal/infra/clients.go:0042 | 	}
backend-go/internal/infra/clients.go:0043 | 	return clients, nil
backend-go/internal/infra/clients.go:0044 | }
backend-go/internal/infra/clients.go:0045 | 
backend-go/internal/infra/clients.go:0046 | func (c *Clients) Close() error {
backend-go/internal/infra/clients.go:0047 | 	if c == nil {
backend-go/internal/infra/clients.go:0048 | 		return nil
backend-go/internal/infra/clients.go:0049 | 	}
backend-go/internal/infra/clients.go:0050 | 	if c.Redis != nil {
backend-go/internal/infra/clients.go:0051 | 		_ = c.Redis.Close()
backend-go/internal/infra/clients.go:0052 | 	}
backend-go/internal/infra/clients.go:0053 | 	if c.DB != nil {
backend-go/internal/infra/clients.go:0054 | 		return c.DB.Close()
backend-go/internal/infra/clients.go:0055 | 	}
backend-go/internal/infra/clients.go:0056 | 	return nil
```

<div style="page-break-after: always;"></div>

### 第 33 页

**文件路径：** backend-go/internal/infra/clients.go；backend-go/internal/httpserver/server.go  
**代码说明：** PostgreSQL 与 Redis 基础设施客户端初始化。；Gin 中间件、模块装配与 REST API 路由注册。

```text
backend-go/internal/infra/clients.go:0057 | }
backend-go/internal/httpserver/server.go:0001 | package httpserver
backend-go/internal/httpserver/server.go:0002 | 
backend-go/internal/httpserver/server.go:0003 | import (
backend-go/internal/httpserver/server.go:0004 | 	"compress/gzip"
backend-go/internal/httpserver/server.go:0005 | 	"crypto/rand"
backend-go/internal/httpserver/server.go:0006 | 	"database/sql"
backend-go/internal/httpserver/server.go:0007 | 	"encoding/hex"
backend-go/internal/httpserver/server.go:0008 | 	"encoding/json"
backend-go/internal/httpserver/server.go:0009 | 	"net/http"
backend-go/internal/httpserver/server.go:0010 | 	"os"
backend-go/internal/httpserver/server.go:0011 | 	"path"
backend-go/internal/httpserver/server.go:0012 | 	"path/filepath"
backend-go/internal/httpserver/server.go:0013 | 	"strconv"
backend-go/internal/httpserver/server.go:0014 | 	"strings"
backend-go/internal/httpserver/server.go:0015 | 	"time"
backend-go/internal/httpserver/server.go:0016 | 
backend-go/internal/httpserver/server.go:0017 | 	"github.com/gin-gonic/gin"
backend-go/internal/httpserver/server.go:0018 | 	"github.com/redis/go-redis/v9"
backend-go/internal/httpserver/server.go:0019 | 
backend-go/internal/httpserver/server.go:0020 | 	paymentapp "xianzhi-ai/backend-go/internal/app/payment"
backend-go/internal/httpserver/server.go:0021 | 	"xianzhi-ai/backend-go/internal/config"
backend-go/internal/httpserver/server.go:0022 | 	storagecenter "xianzhi-ai/backend-go/internal/storage"
backend-go/internal/httpserver/server.go:0023 | )
backend-go/internal/httpserver/server.go:0024 | 
backend-go/internal/httpserver/server.go:0025 | func New(cfg config.Config) *http.Server {
backend-go/internal/httpserver/server.go:0026 | 	return newWithStoreAndSessions(cfg, newJSONStore(cfg.DataPath), defaultAuthSessions(cfg, nil))
backend-go/internal/httpserver/server.go:0027 | }
backend-go/internal/httpserver/server.go:0028 | 
backend-go/internal/httpserver/server.go:0029 | func NewWithDatabase(cfg config.Config, db *sql.DB) *http.Server {
backend-go/internal/httpserver/server.go:0030 | 	return NewWithInfrastructure(cfg, db, nil)
backend-go/internal/httpserver/server.go:0031 | }
backend-go/internal/httpserver/server.go:0032 | 
backend-go/internal/httpserver/server.go:0033 | func NewWithInfrastructure(cfg config.Config, db *sql.DB, redisClient *redis.Client) *http.Server {
backend-go/internal/httpserver/server.go:0034 | 	store := platformStore(newJSONStore(cfg.DataPath))
backend-go/internal/httpserver/server.go:0035 | 	knowledge := newMemoryKnowledgeModule(cfg)
backend-go/internal/httpserver/server.go:0036 | 	mediaRepo := mediaRepository(newMemoryMediaRepository())
backend-go/internal/httpserver/server.go:0037 | 	if db != nil {
backend-go/internal/httpserver/server.go:0038 | 		store = newPostgresPrimaryStore(db, cfg.DataPath)
backend-go/internal/httpserver/server.go:0039 | 		knowledge = newPostgresKnowledgeModule(cfg, db)
backend-go/internal/httpserver/server.go:0040 | 		mediaRepo = newPostgresMediaRepository(db)
backend-go/internal/httpserver/server.go:0041 | 	}
backend-go/internal/httpserver/server.go:0042 | 	return newWithStoreSessionsKnowledgeAndMedia(cfg, store, defaultAuthSessions(cfg, redisClient), knowledge, mediaRepo, redisClient)
backend-go/internal/httpserver/server.go:0043 | }
backend-go/internal/httpserver/server.go:0044 | 
backend-go/internal/httpserver/server.go:0045 | func newWithStore(cfg config.Config, store platformStore) *http.Server {
backend-go/internal/httpserver/server.go:0046 | 	return newWithStoreAndSessions(cfg, store, defaultAuthSessions(cfg, nil))
backend-go/internal/httpserver/server.go:0047 | }
backend-go/internal/httpserver/server.go:0048 | 
backend-go/internal/httpserver/server.go:0049 | func defaultAuthSessions(cfg config.Config, redisClient *redis.Client) authSessionStore {
```

<div style="page-break-after: always;"></div>

### 第 34 页

**文件路径：** backend-go/internal/httpserver/server.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。

```go
backend-go/internal/httpserver/server.go:0050 | 	if sessions := newRedisAuthSessions(redisClient); sessions != nil {
backend-go/internal/httpserver/server.go:0051 | 		return sessions
backend-go/internal/httpserver/server.go:0052 | 	}
backend-go/internal/httpserver/server.go:0053 | 	if cfg.IsProduction() {
backend-go/internal/httpserver/server.go:0054 | 		return nil
backend-go/internal/httpserver/server.go:0055 | 	}
backend-go/internal/httpserver/server.go:0056 | 	return newLocalAuthSessions()
backend-go/internal/httpserver/server.go:0057 | }
backend-go/internal/httpserver/server.go:0058 | 
backend-go/internal/httpserver/server.go:0059 | func newWithStoreAndSessions(cfg config.Config, store platformStore, sessions authSessionStore) *http.Server {
backend-go/internal/httpserver/server.go:0060 | 	return newWithStoreSessionsAndKnowledge(cfg, store, sessions, newMemoryKnowledgeModule(cfg))
backend-go/internal/httpserver/server.go:0061 | }
backend-go/internal/httpserver/server.go:0062 | 
backend-go/internal/httpserver/server.go:0063 | func newWithStoreSessionsAndKnowledge(cfg config.Config, store platformStore, sessions authSessionStore, knowledge *knowledgeModule) *http.Server {
backend-go/internal/httpserver/server.go:0064 | 	return newWithStoreSessionsKnowledgeAndMedia(cfg, store, sessions, knowledge, newMemoryMediaRepository(), nil)
backend-go/internal/httpserver/server.go:0065 | }
backend-go/internal/httpserver/server.go:0066 | 
backend-go/internal/httpserver/server.go:0067 | func newWithStoreSessionsKnowledgeAndMedia(cfg config.Config, store platformStore, sessions authSessionStore, knowledge *knowledgeModule, mediaRepo mediaRepository, redisClient *redis.Client) *http.Server {
backend-go/internal/httpserver/server.go:0068 | 	if knowledge != nil && knowledge.rag != nil {
backend-go/internal/httpserver/server.go:0069 | 		knowledge.rag.SetBillingRecorder(store)
backend-go/internal/httpserver/server.go:0070 | 	}
backend-go/internal/httpserver/server.go:0071 | 	admin := newAdminAPI(store, sessions)
backend-go/internal/httpserver/server.go:0072 | 	adminEnterprise := newAdminEnterpriseAPI(store)
backend-go/internal/httpserver/server.go:0073 | 	auth := newAuthAPI(store, sessions)
backend-go/internal/httpserver/server.go:0074 | 	userRBAC := newUserRBACAPI(store, sessions)
backend-go/internal/httpserver/server.go:0075 | 	promotion := newPromotionAPI(store, sessions, userRBAC, cfg)
backend-go/internal/httpserver/server.go:0076 | 	enterprise := newEnterpriseAPI(store, sessions)
backend-go/internal/httpserver/server.go:0077 | 	channel := newChannelAPI(store, sessions)
backend-go/internal/httpserver/server.go:0078 | 	knowledgeAPI := newKnowledgeAPI(knowledge, sessions, store)
backend-go/internal/httpserver/server.go:0079 | 	mediaStorage, storageErr := newMediaStorage(cfg)
backend-go/internal/httpserver/server.go:0080 | 	if storageErr != nil {
backend-go/internal/httpserver/server.go:0081 | 		mediaStorage = unavailableMediaStorage{err: storageErr}
backend-go/internal/httpserver/server.go:0082 | 	}
backend-go/internal/httpserver/server.go:0083 | 	media := newMediaAPI(cfg, mediaRepo, mediaStorage, store, sessions, redisClient)
backend-go/internal/httpserver/server.go:0084 | 	var fileRepository storagecenter.Repository = storagecenter.NewMemoryRepository()
backend-go/internal/httpserver/server.go:0085 | 	if pgStore, ok := store.(*postgresStore); ok {
backend-go/internal/httpserver/server.go:0086 | 		fileRepository = storagecenter.NewPostgresRepository(pgStore.db)
backend-go/internal/httpserver/server.go:0087 | 	}
backend-go/internal/httpserver/server.go:0088 | 	fileService := storagecenter.NewService(fileRepository, storagecenter.S3ProviderFactory{AutoCreateBucket: cfg.StorageAutoCreateBucket}, fileCenterOptions(cfg))
backend-go/internal/httpserver/server.go:0089 | 	api := newAPI(store, cfg, sessions, fileService)
backend-go/internal/httpserver/server.go:0090 | 	publicCatalog := publicCatalogAPI{store: store}
backend-go/internal/httpserver/server.go:0091 | 	api.pptVisualLocker = newRedisPPTVisualLocker(redisClient)
backend-go/internal/httpserver/server.go:0092 | 	virtualPayment := newVirtualPaymentAPI(cfg, store, sessions, redisClient)
backend-go/internal/httpserver/server.go:0093 | 	paymentCenter := newPaymentCenterAPI(cfg, store, sessions, virtualPayment)
backend-go/internal/httpserver/server.go:0094 | 	connectors := newConnectorAPI(cfg, store, enterprise, api, redisClient)
backend-go/internal/httpserver/server.go:0095 | 	files := newFileCenterAPI(fileService, store, sessions)
backend-go/internal/httpserver/server.go:0096 | 	gin.SetMode(gin.ReleaseMode)
backend-go/internal/httpserver/server.go:0097 | 	router := gin.New()
backend-go/internal/httpserver/server.go:0098 | 	router.Use(gin.Recovery())
backend-go/internal/httpserver/server.go:0099 | 	router.Use(requestContextMiddleware())
```

<div style="page-break-after: always;"></div>

### 第 35 页

**文件路径：** backend-go/internal/httpserver/server.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。

```go
backend-go/internal/httpserver/server.go:0100 | 	router.Use(corsMiddleware(cfg.CORSAllowedOrigins))
backend-go/internal/httpserver/server.go:0101 | 	router.Use(gzipMiddleware())
backend-go/internal/httpserver/server.go:0102 | 	if pgStore, ok := store.(*postgresStore); ok {
backend-go/internal/httpserver/server.go:0103 | 		router.Use(pgStore.auditMiddleware())
backend-go/internal/httpserver/server.go:0104 | 	}
backend-go/internal/httpserver/server.go:0105 | 
backend-go/internal/httpserver/server.go:0106 | 	router.GET("/healthz", wrapF(health))
backend-go/internal/httpserver/server.go:0107 | 	router.POST("/api/open/connectors/feishu/events/:connectorKey", wrapF(connectors.event))
backend-go/internal/httpserver/server.go:0108 | 	router.GET("/api/open/connectors/authorize/:ticket", wrapF(connectors.authorizationLanding))
backend-go/internal/httpserver/server.go:0109 | 	router.GET("/api/open/connectors/authorize/:ticket/start", wrapF(connectors.startAuthorization))
backend-go/internal/httpserver/server.go:0110 | 	router.GET("/api/open/connectors/oauth/feishu/callback", wrapF(connectors.feishuOAuthCallback))
backend-go/internal/httpserver/server.go:0111 | 	router.GET("/api/open/connectors/oauth/wechat/callback", wrapF(connectors.wechatOAuthCallback))
backend-go/internal/httpserver/server.go:0112 | 	v1 := router.Group("/api/v1")
backend-go/internal/httpserver/server.go:0113 | 	v1.GET("/health", wrapF(health))
backend-go/internal/httpserver/server.go:0114 | 	v1.POST("/auth/login", wrapF(auth.login))
backend-go/internal/httpserver/server.go:0115 | 	v1.POST("/auth/wechat-mini-program/login", wrapF(auth.wechatMiniProgramLogin))
backend-go/internal/httpserver/server.go:0116 | 	v1.POST("/auth/wechat-mini-program/link", wrapF(auth.linkWeChatMiniProgram))
backend-go/internal/httpserver/server.go:0117 | 	v1.POST("/auth/wechat/phone-login", wrapF(auth.wechatMiniProgramLogin))
backend-go/internal/httpserver/server.go:0118 | 	v1.GET("/auth/wechat/qrcode", wrapF(auth.wechatWebQRCode))
backend-go/internal/httpserver/server.go:0119 | 	v1.GET("/auth/wechat/status", wrapF(auth.wechatWebStatus))
backend-go/internal/httpserver/server.go:0120 | 	v1.GET("/auth/wechat/callback", wrapF(auth.wechatWebCallback))
backend-go/internal/httpserver/server.go:0121 | 	v1.POST("/auth/wechat/bind-mobile", wrapF(auth.wechatWebBindMobile))
backend-go/internal/httpserver/server.go:0122 | 	v1.POST("/auth/sms/send", wrapF(auth.smsSend))
backend-go/internal/httpserver/server.go:0123 | 	v1.POST("/auth/sms/login", wrapF(auth.smsLogin))
backend-go/internal/httpserver/server.go:0124 | 	v1.POST("/auth/mobile/bind", wrapF(auth.bindMobile))
backend-go/internal/httpserver/server.go:0125 | 	v1.POST("/auth/register", wrapF(auth.register))
backend-go/internal/httpserver/server.go:0126 | 	v1.GET("/auth/me", wrapF(auth.me))
backend-go/internal/httpserver/server.go:0127 | 	v1.POST("/auth/refresh", wrapF(auth.refresh))
backend-go/internal/httpserver/server.go:0128 | 	v1.POST("/auth/logout", wrapF(auth.logout))
backend-go/internal/httpserver/server.go:0129 | 	v1.POST("/auth/logout-all", wrapF(auth.logoutAll))
backend-go/internal/httpserver/server.go:0130 | 	v1.POST("/auth/change-password", wrapF(auth.changePassword))
backend-go/internal/httpserver/server.go:0131 | 	v1.GET("/auth/security", wrapF(auth.security))
backend-go/internal/httpserver/server.go:0132 | 	v1.GET("/invite/agent/resolve", wrapF(auth.resolveInvite))
backend-go/internal/httpserver/server.go:0133 | 	v1.GET("/user/profile", wrapF(userRBAC.profile))
backend-go/internal/httpserver/server.go:0134 | 	v1.POST("/user/current-role", wrapF(userRBAC.switchCurrentRole))
backend-go/internal/httpserver/server.go:0135 | 	v1.GET("/promotion/overview", wrapF(promotion.overview))
backend-go/internal/httpserver/server.go:0136 | 	v1.GET("/promotion/profile", wrapF(promotion.profile))
backend-go/internal/httpserver/server.go:0137 | 	v1.GET("/promotion/poster-templates", wrapF(promotion.templates))
backend-go/internal/httpserver/server.go:0138 | 	v1.POST("/promotion/miniprogram-code", wrapF(promotion.miniProgramCode))
backend-go/internal/httpserver/server.go:0139 | 	v1.POST("/promotion/qrcode", wrapF(promotion.miniProgramCode))
backend-go/internal/httpserver/server.go:0140 | 	v1.POST("/promotion/poster/render", wrapF(promotion.renderConfig))
backend-go/internal/httpserver/server.go:0141 | 	v1.GET("/promotion/records", wrapF(promotion.records))
backend-go/internal/httpserver/server.go:0142 | 	v1.GET("/promotion/analytics", wrapF(promotion.analytics))
backend-go/internal/httpserver/server.go:0143 | 	v1.GET("/promotion/stats", wrapF(promotion.analytics))
backend-go/internal/httpserver/server.go:0144 | 	v1.GET("/promotion/activities", wrapF(promotion.activities))
backend-go/internal/httpserver/server.go:0145 | 	v1.GET("/promotion/share-copy", wrapF(promotion.shareCopy))
backend-go/internal/httpserver/server.go:0146 | 	v1.POST("/promotion/visit", wrapF(promotion.visit))
backend-go/internal/httpserver/server.go:0147 | 	v1.POST("/promotion/bind", wrapF(promotion.bind))
backend-go/internal/httpserver/server.go:0148 | 	v1.GET("/user/enterprise-contexts", wrapF(enterprise.contexts))
backend-go/internal/httpserver/server.go:0149 | 	v1.POST("/user/current-context", wrapF(enterprise.switchContext))
```

<div style="page-break-after: always;"></div>

### 第 36 页

**文件路径：** backend-go/internal/httpserver/server.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。

```go
backend-go/internal/httpserver/server.go:0150 | 	v1.POST("/enterprises", wrapF(enterprise.createEnterprise))
backend-go/internal/httpserver/server.go:0151 | 	v1.GET("/enterprise/overview", wrapF(enterprise.overview))
backend-go/internal/httpserver/server.go:0152 | 	v1.GET("/enterprise/members", wrapF(enterprise.members))
backend-go/internal/httpserver/server.go:0153 | 	v1.GET("/enterprise/members/:id", wrapF(enterprise.member))
backend-go/internal/httpserver/server.go:0154 | 	v1.PATCH("/enterprise/members/:id", wrapF(enterprise.updateMember))
backend-go/internal/httpserver/server.go:0155 | 	v1.POST("/enterprise/members/:id/disable", wrapF(enterprise.disableMember))
backend-go/internal/httpserver/server.go:0156 | 	v1.DELETE("/enterprise/members/:id", wrapF(enterprise.removeMember))
backend-go/internal/httpserver/server.go:0157 | 	v1.POST("/enterprise/invitations", wrapF(enterprise.createInvitation))
backend-go/internal/httpserver/server.go:0158 | 	v1.POST("/enterprise/invitations/accept", wrapF(enterprise.acceptInvitation))
backend-go/internal/httpserver/server.go:0159 | 	v1.GET("/enterprise/join-requests", wrapF(enterprise.joinRequests))
backend-go/internal/httpserver/server.go:0160 | 	v1.POST("/enterprise/join-requests", wrapF(enterprise.createJoinRequest))
backend-go/internal/httpserver/server.go:0161 | 	v1.POST("/enterprise/join-requests/:id/approve", wrapF(enterprise.approveJoinRequest))
backend-go/internal/httpserver/server.go:0162 | 	v1.POST("/enterprise/join-requests/:id/reject", wrapF(enterprise.rejectJoinRequest))
backend-go/internal/httpserver/server.go:0163 | 	v1.GET("/enterprise/organizations/tree", wrapF(enterprise.organizationTree))
backend-go/internal/httpserver/server.go:0164 | 	v1.POST("/enterprise/organizations", wrapF(enterprise.createOrganization))
backend-go/internal/httpserver/server.go:0165 | 	v1.PATCH("/enterprise/organizations/:id", wrapF(enterprise.updateOrganization))
backend-go/internal/httpserver/server.go:0166 | 	v1.POST("/enterprise/organizations/:id/move", wrapF(enterprise.moveOrganization))
backend-go/internal/httpserver/server.go:0167 | 	v1.DELETE("/enterprise/organizations/:id", wrapF(enterprise.deleteOrganization))
backend-go/internal/httpserver/server.go:0168 | 	v1.GET("/enterprise/roles", wrapF(enterprise.roles))
backend-go/internal/httpserver/server.go:0169 | 	v1.GET("/enterprise/billing/summary", wrapF(enterprise.billingSummary))
backend-go/internal/httpserver/server.go:0170 | 	v1.GET("/enterprise/compute-account", wrapF(enterprise.computeAccount))
backend-go/internal/httpserver/server.go:0171 | 	v1.POST("/enterprise/certifications", wrapF(enterprise.submitCertification))
backend-go/internal/httpserver/server.go:0172 | 	v1.GET("/enterprise/audit-logs", wrapF(enterprise.auditLogs))
backend-go/internal/httpserver/server.go:0173 | 	v1.GET("/enterprise/connectors", wrapF(connectors.list))
backend-go/internal/httpserver/server.go:0174 | 	v1.GET("/enterprise/connector-authorizations/platforms", wrapF(connectors.authorizationPlatforms))
backend-go/internal/httpserver/server.go:0175 | 	v1.POST("/enterprise/connector-authorizations", wrapF(connectors.createAuthorizationSession))
backend-go/internal/httpserver/server.go:0176 | 	v1.GET("/enterprise/connector-authorizations/:id", wrapF(connectors.getAuthorizationSession))
backend-go/internal/httpserver/server.go:0177 | 	v1.POST("/enterprise/connector-authorizations/:id/cancel", wrapF(connectors.cancelAuthorizationSession))
backend-go/internal/httpserver/server.go:0178 | 	v1.GET("/enterprise/connectors/feishu", wrapF(connectors.getFeishu))
backend-go/internal/httpserver/server.go:0179 | 	v1.POST("/enterprise/connectors/feishu", wrapF(connectors.createFeishu))
backend-go/internal/httpserver/server.go:0180 | 	v1.PUT("/enterprise/connectors/feishu", wrapF(connectors.updateFeishu))
backend-go/internal/httpserver/server.go:0181 | 	v1.POST("/enterprise/connectors/feishu/test", wrapF(connectors.testFeishu))
backend-go/internal/httpserver/server.go:0182 | 	v1.POST("/enterprise/connectors/feishu/enable", wrapF(connectors.enableFeishu))
backend-go/internal/httpserver/server.go:0183 | 	v1.POST("/enterprise/connectors/feishu/disable", wrapF(connectors.disableFeishu))
backend-go/internal/httpserver/server.go:0184 | 	v1.GET("/enterprise/connectors/feishu/users", wrapF(connectors.users))
backend-go/internal/httpserver/server.go:0185 | 	v1.PUT("/enterprise/connectors/feishu/users/:id", wrapF(connectors.updateUser))
backend-go/internal/httpserver/server.go:0186 | 	v1.GET("/enterprise/connectors/feishu/logs", wrapF(connectors.logs))
backend-go/internal/httpserver/server.go:0187 | 	v1.GET("/enterprise/connectors/feishu/tasks", wrapF(connectors.tasks))
backend-go/internal/httpserver/server.go:0188 | 	v1.POST("/enterprise/connectors/feishu/tasks/:taskId/retry-delivery", wrapF(connectors.retryDelivery))
backend-go/internal/httpserver/server.go:0189 | 	v1.GET("/channel/me", wrapF(channel.me))
backend-go/internal/httpserver/server.go:0190 | 	v1.GET("/channel/customers", wrapF(channel.customers))
backend-go/internal/httpserver/server.go:0191 | 	v1.GET("/channel/customers/:id", wrapF(channel.customerDetail))
backend-go/internal/httpserver/server.go:0192 | 	v1.GET("/channel/orders", wrapF(channel.orders))
backend-go/internal/httpserver/server.go:0193 | 	v1.GET("/channel/orders/:id", wrapF(channel.orderDetail))
backend-go/internal/httpserver/server.go:0194 | 	v1.GET("/channel/usage", wrapF(channel.usage))
backend-go/internal/httpserver/server.go:0195 | 	v1.GET("/channel/commissions", wrapF(channel.commissions))
backend-go/internal/httpserver/server.go:0196 | 	v1.GET("/channel/commissions/:id", wrapF(channel.commissionDetail))
backend-go/internal/httpserver/server.go:0197 | 	v1.GET("/channel/withdrawals", wrapF(channel.withdrawals))
backend-go/internal/httpserver/server.go:0198 | 	v1.GET("/channel/withdrawals/:id", wrapF(channel.withdrawalDetail))
backend-go/internal/httpserver/server.go:0199 | 	v1.POST("/channel/withdrawals", wrapF(channel.createWithdrawal))
```

<div style="page-break-after: always;"></div>

### 第 37 页

**文件路径：** backend-go/internal/httpserver/server.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。

```go
backend-go/internal/httpserver/server.go:0200 | 	v1.GET("/channel/children/:id", wrapF(channel.childDetail))
backend-go/internal/httpserver/server.go:0201 | 	v1.POST("/channel/children", wrapF(channel.createChildAgent))
backend-go/internal/httpserver/server.go:0202 | 	v1.GET("/channel/invite-records", wrapF(channel.inviteRecords))
backend-go/internal/httpserver/server.go:0203 | 	v1.GET("/models", wrapF(api.models))
backend-go/internal/httpserver/server.go:0204 | 	v1.GET("/public/home", wrapF(publicCatalog.home))
backend-go/internal/httpserver/server.go:0205 | 	v1.GET("/public/cases", wrapF(publicCatalog.cases))
backend-go/internal/httpserver/server.go:0206 | 	v1.GET("/public/templates", wrapF(publicCatalog.templates))
backend-go/internal/httpserver/server.go:0207 | 	v1.GET("/public/agents", wrapF(publicCatalog.agents))
backend-go/internal/httpserver/server.go:0208 | 	v1.GET("/public/models", wrapF(api.models))
backend-go/internal/httpserver/server.go:0209 | 	v1.GET("/public/legal-documents", wrapF(api.publicLegalDocuments))
backend-go/internal/httpserver/server.go:0210 | 	v1.GET("/public/terminal-capabilities", wrapF(api.publicTerminalCapabilities))
backend-go/internal/httpserver/server.go:0211 | 	v1.GET("/legal/acceptance-status", wrapF(api.legalAcceptanceStatus))
backend-go/internal/httpserver/server.go:0212 | 	v1.POST("/legal/acceptances", wrapF(api.acceptCurrentLegalDocuments))
backend-go/internal/httpserver/server.go:0213 | 	v1.GET("/public/pricing", wrapF(api.plans))
backend-go/internal/httpserver/server.go:0214 | 	v1.POST("/public/experience-events", wrapF(publicCatalog.recordGuestExperienceEvent))
backend-go/internal/httpserver/server.go:0215 | 	v1.GET("/app/review-mode", wrapF(api.reviewMode))
backend-go/internal/httpserver/server.go:0216 | 	v1.GET("/module-schema", wrapF(api.moduleSchema))
backend-go/internal/httpserver/server.go:0217 | 	v1.GET("/plans", wrapF(api.plans))
backend-go/internal/httpserver/server.go:0218 | 	v1.GET("/plans/:id", wrapF(api.planDetail))
backend-go/internal/httpserver/server.go:0219 | 	v1.POST("/orders/create", wrapF(api.createCommerceOrder))
backend-go/internal/httpserver/server.go:0220 | 	v1.POST("/pay/callback", wrapF(api.payCallback))
backend-go/internal/httpserver/server.go:0221 | 	v1.GET("/member/profile", wrapF(api.memberProfile))
backend-go/internal/httpserver/server.go:0222 | 	v1.PATCH("/member/profile", wrapF(api.updateMemberProfile))
backend-go/internal/httpserver/server.go:0223 | 	v1.GET("/member/wallet", wrapF(api.memberWallet))
backend-go/internal/httpserver/server.go:0224 | 	v1.GET("/member/orders", wrapF(api.memberOrders))
backend-go/internal/httpserver/server.go:0225 | 	v1.GET("/member/orders/:id", wrapF(api.memberOrderDetail))
backend-go/internal/httpserver/server.go:0226 | 	v1.GET("/member/invoices", wrapF(api.memberInvoices))
backend-go/internal/httpserver/server.go:0227 | 	v1.POST("/member/refund-requests", wrapF(api.createMemberRefundRequest))
backend-go/internal/httpserver/server.go:0228 | 	v1.GET("/member/token-records", wrapF(api.memberTokenRecords))
backend-go/internal/httpserver/server.go:0229 | 	v1.GET("/agent/profile", wrapF(api.agentProfile))
backend-go/internal/httpserver/server.go:0230 | 	v1.POST("/agent/join-order", wrapF(api.createAgentJoinOrder))
backend-go/internal/httpserver/server.go:0231 | 	v1.GET("/operation-center/profile", wrapF(api.operationCenterProfile))
backend-go/internal/httpserver/server.go:0232 | 	v1.GET("/operation-center/agents", wrapF(api.operationCenterAgents))
backend-go/internal/httpserver/server.go:0233 | 	v1.GET("/operation-center/agents/:id", wrapF(api.operationCenterAgentDetail))
backend-go/internal/httpserver/server.go:0234 | 	v1.GET("/operation-center/orders", wrapF(api.operationCenterOrders))
backend-go/internal/httpserver/server.go:0235 | 	v1.GET("/operation-center/orders/:id", wrapF(api.operationCenterOrderDetail))
backend-go/internal/httpserver/server.go:0236 | 	v1.GET("/operation-center/commissions", wrapF(api.operationCenterCommissions))
backend-go/internal/httpserver/server.go:0237 | 	v1.GET("/operation-center/commissions/:id", wrapF(api.operationCenterCommissionDetail))
backend-go/internal/httpserver/server.go:0238 | 	v1.POST("/operation-center/join-order", wrapF(api.createOperationCenterJoinOrder))
backend-go/internal/httpserver/server.go:0239 | 	v1.GET("/points/account", wrapF(api.pointAccount))
backend-go/internal/httpserver/server.go:0240 | 	v1.POST("/points/recharge-orders", wrapF(api.createRechargeOrder))
backend-go/internal/httpserver/server.go:0241 | 	v1.POST("/points/subscription-orders", wrapF(api.createSubscriptionOrder))
backend-go/internal/httpserver/server.go:0242 | 	v1.POST("/payment/orders", wrapF(paymentCenter.createOrder))
backend-go/internal/httpserver/server.go:0243 | 	v1.GET("/payment/products", wrapF(virtualPayment.products))
backend-go/internal/httpserver/server.go:0244 | 	v1.GET("/payment/coupons", wrapF(virtualPayment.coupons))
backend-go/internal/httpserver/server.go:0245 | 	v1.POST("/payment/wechat-virtual/orders", wrapF(virtualPayment.createOrder))
backend-go/internal/httpserver/server.go:0246 | 	v1.GET("/payment/orders/:orderNo", wrapF(paymentCenter.order))
backend-go/internal/httpserver/server.go:0247 | 	if !cfg.IsProduction() {
backend-go/internal/httpserver/server.go:0248 | 		v1.POST("/payment/mock/:orderNo/success", wrapF(paymentCenter.mockAction(paymentapp.MockSuccess, false)))
backend-go/internal/httpserver/server.go:0249 | 		v1.POST("/payment/mock/:orderNo/fail", wrapF(paymentCenter.mockAction(paymentapp.MockFailure, false)))
```

<div style="page-break-after: always;"></div>

### 第 38 页

**文件路径：** backend-go/internal/httpserver/server.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。

```go
backend-go/internal/httpserver/server.go:0250 | 		v1.POST("/payment/mock/:orderNo/duplicate-notify", wrapF(paymentCenter.mockAction(paymentapp.MockSuccess, true)))
backend-go/internal/httpserver/server.go:0251 | 		v1.POST("/payment/mock/:orderNo/amount-mismatch", wrapF(paymentCenter.mockAction(paymentapp.MockAmountMismatch, false)))
backend-go/internal/httpserver/server.go:0252 | 		v1.POST("/payment/mock/:orderNo/delayed-success", wrapF(paymentCenter.mockDelayedSuccess))
backend-go/internal/httpserver/server.go:0253 | 	}
backend-go/internal/httpserver/server.go:0254 | 	v1.GET("/payment/orders/:orderNo/status", wrapF(virtualPayment.orderStatus))
backend-go/internal/httpserver/server.go:0255 | 	v1.POST("/payment/orders/:orderNo/sync", wrapF(virtualPayment.syncOrder))
backend-go/internal/httpserver/server.go:0256 | 	v1.GET("/payment/wechat-virtual/notify", wrapF(virtualPayment.verifyNotify))
backend-go/internal/httpserver/server.go:0257 | 	v1.POST("/payment/wechat-virtual/notify", wrapF(virtualPayment.notify))
backend-go/internal/httpserver/server.go:0258 | 	v1.GET("/user/dashboard", wrapF(api.userDashboard))
backend-go/internal/httpserver/server.go:0259 | 	v1.GET("/user/online-image", wrapF(api.userOnlineImage))
backend-go/internal/httpserver/server.go:0260 | 	v1.PATCH("/user/ai-state", wrapF(api.updateUserAIState))
backend-go/internal/httpserver/server.go:0261 | 	v1.GET("/user/api-settings", wrapF(api.userAPISettings))
backend-go/internal/httpserver/server.go:0262 | 	v1.GET("/user/usage", wrapF(api.userUsage))
backend-go/internal/httpserver/server.go:0263 | 	v1.GET("/user/usage/:id", wrapF(api.userUsageDetail))
backend-go/internal/httpserver/server.go:0264 | 	v1.GET("/officecli/status", wrapF(api.officeCLIStatus))
backend-go/internal/httpserver/server.go:0265 | 	v1.POST("/officecli/documents", wrapF(api.createOfficeCLIDocument))
backend-go/internal/httpserver/server.go:0266 | 	v1.GET("/officecli/documents/:fileName/download", wrapF(api.downloadOfficeCLIDocument))
backend-go/internal/httpserver/server.go:0267 | 	v1.GET("/generation-tasks", wrapF(api.listGenerationTasks))
backend-go/internal/httpserver/server.go:0268 | 	v1.GET("/generation-tasks/:id", wrapF(api.getGenerationTask))
backend-go/internal/httpserver/server.go:0269 | 	v1.POST("/generation-tasks/:id/retry", wrapF(api.retryGenerationTask))
backend-go/internal/httpserver/server.go:0270 | 	v1.POST("/generation-tasks/:id/cancel", wrapF(api.cancelGenerationTask))
backend-go/internal/httpserver/server.go:0271 | 	v1.POST("/generation-tasks", wrapF(api.createGenerationTask))
backend-go/internal/httpserver/server.go:0272 | 	v1.POST("/ppt/generate", wrapF(api.createPPTGenerationTask))
backend-go/internal/httpserver/server.go:0273 | 	v1.POST("/ppt/estimate", wrapF(api.estimatePPTGenerationCost))
backend-go/internal/httpserver/server.go:0274 | 	v1.POST("/ppt/outline/generate", wrapF(api.generatePPTOutline))
backend-go/internal/httpserver/server.go:0275 | 	v1.POST("/ppt/outline/save", wrapF(api.savePPTOutline))
backend-go/internal/httpserver/server.go:0276 | 	v1.GET("/ppt/tasks/:taskId", wrapF(api.getPPTTask))
backend-go/internal/httpserver/server.go:0277 | 	v1.GET("/ppt/history", wrapF(api.listPPTHistory))
backend-go/internal/httpserver/server.go:0278 | 	v1.DELETE("/ppt/tasks/:taskId", wrapF(api.deletePPTTask))
backend-go/internal/httpserver/server.go:0279 | 	v1.POST("/ppt/slides/:slideId/regenerate", wrapF(api.regeneratePPTSlide))
backend-go/internal/httpserver/server.go:0280 | 	v1.POST("/presentations/:id/slides/:slideId/regenerate-visual", wrapF(api.regeneratePPTSlideVisual))
backend-go/internal/httpserver/server.go:0281 | 	v1.PATCH("/presentations/:id/slides/:slideId", wrapF(api.updatePPTSlide))
backend-go/internal/httpserver/server.go:0282 | 	v1.PATCH("/presentations/:id/slides/:slideId/visual", wrapF(api.updatePPTSlideImage))
backend-go/internal/httpserver/server.go:0283 | 	v1.DELETE("/presentations/:id/slides/:slideId/visual", wrapF(api.deletePPTSlideVisual))
backend-go/internal/httpserver/server.go:0284 | 	v1.POST("/presentations/:id/slides/:slideId/visual/restore", wrapF(api.restorePPTSlideVisual))
backend-go/internal/httpserver/server.go:0285 | 	v1.POST("/ppt/export/pptx", wrapF(api.exportPPT))
backend-go/internal/httpserver/server.go:0286 | 	v1.GET("/ppt/tasks/:taskId/export/pptx", wrapF(api.downloadPPTExport))
backend-go/internal/httpserver/server.go:0287 | 	v1.POST("/ppt/export/pdf", wrapF(api.exportPDF))
backend-go/internal/httpserver/server.go:0288 | 	v1.POST("/ppt/images/generate", wrapF(api.generatePPTImage))
backend-go/internal/httpserver/server.go:0289 | 	v1.GET("/ppt/images/search", wrapF(api.searchPPTImages))
backend-go/internal/httpserver/server.go:0290 | 	v1.GET("/ppt/models/text", wrapF(api.listPPTTextModels))
backend-go/internal/httpserver/server.go:0291 | 	v1.GET("/ppt/models/image", wrapF(api.listPPTImageModels))
backend-go/internal/httpserver/server.go:0292 | 	v1.GET("/video/download", wrapF(api.downloadVideoByURL))
backend-go/internal/httpserver/server.go:0293 | 	v1.GET("/assets", wrapF(api.listAssets))
backend-go/internal/httpserver/server.go:0294 | 	v1.GET("/assets/overview", wrapF(api.assetsOverview))
backend-go/internal/httpserver/server.go:0295 | 	v1.GET("/assets/projects", wrapF(api.assetProjects))
backend-go/internal/httpserver/server.go:0296 | 	v1.POST("/assets/batch", wrapF(api.batchAssets))
backend-go/internal/httpserver/server.go:0297 | 	v1.GET("/assets/:id", wrapF(api.assetDetail))
backend-go/internal/httpserver/server.go:0298 | 	v1.PATCH("/assets/:id", wrapF(api.updateAsset))
backend-go/internal/httpserver/server.go:0299 | 	v1.POST("/assets/:id/favorite", wrapF(api.favoriteAsset))
```

<div style="page-break-after: always;"></div>

### 第 39 页

**文件路径：** backend-go/internal/httpserver/server.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。

```go
backend-go/internal/httpserver/server.go:0300 | 	v1.DELETE("/assets/:id/favorite", wrapF(api.favoriteAsset))
backend-go/internal/httpserver/server.go:0301 | 	v1.POST("/assets/:id/archive", wrapF(api.archiveAsset))
backend-go/internal/httpserver/server.go:0302 | 	v1.POST("/assets/:id/restore", wrapF(api.restoreAsset))
backend-go/internal/httpserver/server.go:0303 | 	v1.DELETE("/assets/:id/permanent", wrapF(api.permanentlyDeleteAsset))
backend-go/internal/httpserver/server.go:0304 | 	v1.POST("/assets/:id/move-project", wrapF(api.moveAssetProject))
backend-go/internal/httpserver/server.go:0305 | 	v1.POST("/assets/thumbnail-backfill", wrapF(api.backfillAssetThumbnails))
backend-go/internal/httpserver/server.go:0306 | 	v1.GET("/assets/:id/download", wrapF(api.downloadAsset))
backend-go/internal/httpserver/server.go:0307 | 	v1.DELETE("/assets/:id", wrapF(api.deleteAsset))
backend-go/internal/httpserver/server.go:0308 | 	v1.POST("/files/upload/init", wrapF(files.initUpload))
backend-go/internal/httpserver/server.go:0309 | 	v1.POST("/files/upload/complete", wrapF(files.completeUpload))
backend-go/internal/httpserver/server.go:0310 | 	v1.GET("/files/:fileId", wrapF(files.getFile))
backend-go/internal/httpserver/server.go:0311 | 	v1.GET("/files/:fileId/access-url", wrapF(files.accessURL(false)))
backend-go/internal/httpserver/server.go:0312 | 	v1.GET("/files/:fileId/download-url", wrapF(files.accessURL(true)))
backend-go/internal/httpserver/server.go:0313 | 	v1.DELETE("/files/:fileId", wrapF(files.deleteFile))
backend-go/internal/httpserver/server.go:0314 | 	v1.POST("/files/:fileId/restore", wrapF(files.restoreFile))
backend-go/internal/httpserver/server.go:0315 | 	v1.DELETE("/files/:fileId/permanent", wrapF(files.permanentDeleteFile))
backend-go/internal/httpserver/server.go:0316 | 	v1.POST("/reference-images", wrapF(api.uploadReferenceImage))
backend-go/internal/httpserver/server.go:0317 | 	v1.GET("/reference-images/:name", wrapF(api.serveReferenceImage))
backend-go/internal/httpserver/server.go:0318 | 	v1.GET("/generated-media/:name", wrapF(api.serveGeneratedMedia))
backend-go/internal/httpserver/server.go:0319 | 	v1.GET("/knowledge/context", wrapF(knowledgeAPI.context))
backend-go/internal/httpserver/server.go:0320 | 	v1.GET("/knowledge/tags", wrapF(knowledgeAPI.listTags))
backend-go/internal/httpserver/server.go:0321 | 	v1.POST("/knowledge/tags", wrapF(knowledgeAPI.createTag))
backend-go/internal/httpserver/server.go:0322 | 	v1.GET("/knowledge/categories", wrapF(knowledgeAPI.listCategories))
backend-go/internal/httpserver/server.go:0323 | 	v1.POST("/knowledge/categories", wrapF(knowledgeAPI.createCategory))
backend-go/internal/httpserver/server.go:0324 | 	v1.GET("/knowledge/profiles/:resource", wrapF(knowledgeAPI.listProfiles))
backend-go/internal/httpserver/server.go:0325 | 	v1.GET("/knowledge-bases", wrapF(knowledgeAPI.listKnowledgeBases))
backend-go/internal/httpserver/server.go:0326 | 	v1.POST("/knowledge-bases", wrapF(knowledgeAPI.createKnowledgeBase))
backend-go/internal/httpserver/server.go:0327 | 	v1.GET("/knowledge-bases/:id", wrapF(knowledgeAPI.getKnowledgeBase))
backend-go/internal/httpserver/server.go:0328 | 	v1.PATCH("/knowledge-bases/:id", wrapF(knowledgeAPI.updateKnowledgeBase))
backend-go/internal/httpserver/server.go:0329 | 	v1.DELETE("/knowledge-bases/:id", wrapF(knowledgeAPI.deleteKnowledgeBase))
backend-go/internal/httpserver/server.go:0330 | 	v1.GET("/knowledge-bases/:id/acl", wrapF(knowledgeAPI.listKnowledgeBaseACL))
backend-go/internal/httpserver/server.go:0331 | 	v1.PUT("/knowledge-bases/:id/acl", wrapF(knowledgeAPI.replaceKnowledgeBaseACL))
backend-go/internal/httpserver/server.go:0332 | 	v1.GET("/knowledge-bases/:id/documents", wrapF(knowledgeAPI.listDocuments))
backend-go/internal/httpserver/server.go:0333 | 	v1.POST("/knowledge-bases/:id/documents:ingest", wrapF(knowledgeAPI.ingestDocument))
backend-go/internal/httpserver/server.go:0334 | 	v1.DELETE("/knowledge-documents/:id", wrapF(knowledgeAPI.deleteDocument))
backend-go/internal/httpserver/server.go:0335 | 	v1.GET("/knowledge-documents/:id", wrapF(knowledgeAPI.getDocument))
backend-go/internal/httpserver/server.go:0336 | 	v1.GET("/knowledge-chunks", wrapF(knowledgeAPI.listChunks))
backend-go/internal/httpserver/server.go:0337 | 	v1.POST("/knowledge-search", wrapF(knowledgeAPI.search))
backend-go/internal/httpserver/server.go:0338 | 	v1.GET("/knowledge-agents", wrapF(knowledgeAPI.listAgents))
backend-go/internal/httpserver/server.go:0339 | 	v1.POST("/knowledge-agents", wrapF(knowledgeAPI.createAgent))
backend-go/internal/httpserver/server.go:0340 | 	v1.GET("/knowledge-agents/:id", wrapF(knowledgeAPI.getAgent))
backend-go/internal/httpserver/server.go:0341 | 	v1.PUT("/knowledge-agents/:id/knowledge-bindings", wrapF(knowledgeAPI.replaceAgentBindings))
backend-go/internal/httpserver/server.go:0342 | 	v1.GET("/knowledge-conversations", wrapF(knowledgeAPI.listConversations))
backend-go/internal/httpserver/server.go:0343 | 	v1.POST("/knowledge-conversations", wrapF(knowledgeAPI.createConversation))
backend-go/internal/httpserver/server.go:0344 | 	v1.GET("/knowledge-conversations/:id/messages", wrapF(knowledgeAPI.listMessages))
backend-go/internal/httpserver/server.go:0345 | 	v1.POST("/knowledge-conversations/:id/runs", wrapF(knowledgeAPI.runRAG))
backend-go/internal/httpserver/server.go:0346 | 	v1.POST("/knowledge-conversations/:id/runs:stream", wrapF(knowledgeAPI.streamRAG))
backend-go/internal/httpserver/server.go:0347 | 	v1.GET("/knowledge-runs/:id", wrapF(knowledgeAPI.getRun))
backend-go/internal/httpserver/server.go:0348 | 	v1.POST("/knowledge-runs/:id/cancel", wrapF(knowledgeAPI.cancelRun))
backend-go/internal/httpserver/server.go:0349 | 	v1.POST("/knowledge-runs/:id/retry", wrapF(knowledgeAPI.retryRun))
```

<div style="page-break-after: always;"></div>

### 第 40 页

**文件路径：** backend-go/internal/httpserver/server.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。

```go
backend-go/internal/httpserver/server.go:0350 | 	v1.GET("/knowledge-runs/:id/events", wrapF(knowledgeAPI.listRunEvents))
backend-go/internal/httpserver/server.go:0351 | 	v1.GET("/knowledge-runs/:id/citations", wrapF(knowledgeAPI.listRunCitations))
backend-go/internal/httpserver/server.go:0352 | 	v1.GET("/app/pages/:pageCode", wrapF(media.publicPage))
backend-go/internal/httpserver/server.go:0353 | 	v1.GET("/app/page-config/:pageCode", wrapF(media.publicPage))
backend-go/internal/httpserver/server.go:0354 | 	v1.GET("/media/files/*filepath", wrapF(serveLocalMediaFile(mediaStorage)))
backend-go/internal/httpserver/server.go:0355 | 	router.GET("/api/module-schema", wrapF(api.moduleSchema))
backend-go/internal/httpserver/server.go:0356 | 
backend-go/internal/httpserver/server.go:0357 | 	pptGroup := router.Group("/api/ppt")
backend-go/internal/httpserver/server.go:0358 | 	pptGroup.POST("/generate", wrapF(api.createPPTGenerationTask))
backend-go/internal/httpserver/server.go:0359 | 	pptGroup.POST("/estimate", wrapF(api.estimatePPTGenerationCost))
backend-go/internal/httpserver/server.go:0360 | 	pptGroup.POST("/outline/generate", wrapF(api.generatePPTOutline))
backend-go/internal/httpserver/server.go:0361 | 	pptGroup.POST("/outline/save", wrapF(api.savePPTOutline))
backend-go/internal/httpserver/server.go:0362 | 	pptGroup.GET("/tasks/:taskId", wrapF(api.getPPTTask))
backend-go/internal/httpserver/server.go:0363 | 	pptGroup.GET("/history", wrapF(api.listPPTHistory))
backend-go/internal/httpserver/server.go:0364 | 	pptGroup.DELETE("/tasks/:taskId", wrapF(api.deletePPTTask))
backend-go/internal/httpserver/server.go:0365 | 	pptGroup.POST("/slides/:slideId/regenerate", wrapF(api.regeneratePPTSlide))
backend-go/internal/httpserver/server.go:0366 | 	pptGroup.POST("/export/pptx", wrapF(api.exportPPT))
backend-go/internal/httpserver/server.go:0367 | 	pptGroup.POST("/export/pdf", wrapF(api.exportPDF))
backend-go/internal/httpserver/server.go:0368 | 	pptGroup.POST("/images/generate", wrapF(api.generatePPTImage))
backend-go/internal/httpserver/server.go:0369 | 	pptGroup.GET("/images/search", wrapF(api.searchPPTImages))
backend-go/internal/httpserver/server.go:0370 | 	pptGroup.GET("/models/text", wrapF(api.listPPTTextModels))
backend-go/internal/httpserver/server.go:0371 | 	pptGroup.GET("/models/image", wrapF(api.listPPTImageModels))
backend-go/internal/httpserver/server.go:0372 | 
backend-go/internal/httpserver/server.go:0373 | 	adminGroup := v1.Group("/admin")
backend-go/internal/httpserver/server.go:0374 | 	if pgStore, ok := store.(*postgresStore); ok {
backend-go/internal/httpserver/server.go:0375 | 		adminGroup.Use(func(c *gin.Context) {
backend-go/internal/httpserver/server.go:0376 | 			permission := adminPermissionForRequest(c.Request)
backend-go/internal/httpserver/server.go:0377 | 			pgStore.rbacMiddleware(auth, permission)(c)
backend-go/internal/httpserver/server.go:0378 | 		})
backend-go/internal/httpserver/server.go:0379 | 	} else {
backend-go/internal/httpserver/server.go:0380 | 		adminGroup.Use(superAdminMiddleware(auth))
backend-go/internal/httpserver/server.go:0381 | 	}
backend-go/internal/httpserver/server.go:0382 | 	adminPaymentGroup := router.Group("/api/admin")
backend-go/internal/httpserver/server.go:0383 | 	if pgStore, ok := store.(*postgresStore); ok {
backend-go/internal/httpserver/server.go:0384 | 		adminPaymentGroup.Use(func(c *gin.Context) {
backend-go/internal/httpserver/server.go:0385 | 			permission := adminPermissionForRequest(c.Request)
backend-go/internal/httpserver/server.go:0386 | 			pgStore.rbacMiddleware(auth, permission)(c)
backend-go/internal/httpserver/server.go:0387 | 		})
backend-go/internal/httpserver/server.go:0388 | 	} else {
backend-go/internal/httpserver/server.go:0389 | 		adminPaymentGroup.Use(superAdminMiddleware(auth))
backend-go/internal/httpserver/server.go:0390 | 	}
backend-go/internal/httpserver/server.go:0391 | 	adminGroup.GET("/overview", wrapF(admin.overview))
backend-go/internal/httpserver/server.go:0392 | 	adminGroup.GET("/search", wrapF(admin.globalSearch))
backend-go/internal/httpserver/server.go:0393 | 	adminGroup.PATCH("/exceptions/:id", wrapF(admin.updateExceptionCase))
backend-go/internal/httpserver/server.go:0394 | 	adminGroup.POST("/experience-events", wrapF(admin.recordExperienceEvent))
backend-go/internal/httpserver/server.go:0395 | 	adminGroup.GET("/experience-analytics", wrapF(admin.experienceAnalytics))
backend-go/internal/httpserver/server.go:0396 | 	adminGroup.GET("/enterprises", wrapF(adminEnterprise.list))
backend-go/internal/httpserver/server.go:0397 | 	adminGroup.POST("/enterprises", wrapF(adminEnterprise.create))
backend-go/internal/httpserver/server.go:0398 | 	adminGroup.GET("/enterprises/export", wrapF(adminEnterprise.export))
backend-go/internal/httpserver/server.go:0399 | 	adminGroup.GET("/enterprises/certifications", wrapF(adminEnterprise.certifications))
```

<div style="page-break-after: always;"></div>

### 第 41 页

**文件路径：** backend-go/internal/httpserver/server.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。

```go
backend-go/internal/httpserver/server.go:0400 | 	adminGroup.GET("/enterprises/:enterpriseId", wrapF(adminEnterprise.detail))
backend-go/internal/httpserver/server.go:0401 | 	adminGroup.PATCH("/enterprises/:enterpriseId", wrapF(adminEnterprise.mutate("profile-update")))
backend-go/internal/httpserver/server.go:0402 | 	adminGroup.GET("/enterprises/:enterpriseId/certifications", wrapF(adminEnterprise.section("certifications")))
backend-go/internal/httpserver/server.go:0403 | 	adminGroup.GET("/enterprises/:enterpriseId/members", wrapF(adminEnterprise.section("members")))
backend-go/internal/httpserver/server.go:0404 | 	adminGroup.GET("/enterprises/:enterpriseId/package", wrapF(adminEnterprise.section("package")))
backend-go/internal/httpserver/server.go:0405 | 	adminGroup.GET("/enterprises/:enterpriseId/compute", wrapF(adminEnterprise.section("compute")))
backend-go/internal/httpserver/server.go:0406 | 	adminGroup.GET("/enterprises/:enterpriseId/transactions", wrapF(adminEnterprise.section("transactions")))
backend-go/internal/httpserver/server.go:0407 | 	adminGroup.GET("/enterprises/:enterpriseId/orders", wrapF(adminEnterprise.section("orders")))
backend-go/internal/httpserver/server.go:0408 | 	adminGroup.GET("/enterprises/:enterpriseId/ai-capabilities", wrapF(adminEnterprise.section("ai-capabilities")))
backend-go/internal/httpserver/server.go:0409 | 	adminGroup.GET("/enterprises/:enterpriseId/ai-employees", wrapF(adminEnterprise.section("ai-employees")))
backend-go/internal/httpserver/server.go:0410 | 	adminGroup.GET("/enterprises/:enterpriseId/knowledge-bases", wrapF(adminEnterprise.section("knowledge-bases")))
backend-go/internal/httpserver/server.go:0411 | 	adminGroup.GET("/enterprises/:enterpriseId/attribution", wrapF(adminEnterprise.section("attribution")))
backend-go/internal/httpserver/server.go:0412 | 	adminGroup.GET("/enterprises/:enterpriseId/relationships", wrapF(adminEnterprise.section("relationships")))
backend-go/internal/httpserver/server.go:0413 | 	adminGroup.GET("/enterprises/:enterpriseId/integrations", wrapF(adminEnterprise.section("integrations")))
backend-go/internal/httpserver/server.go:0414 | 	adminGroup.GET("/enterprises/:enterpriseId/risk", wrapF(adminEnterprise.section("risk")))
backend-go/internal/httpserver/server.go:0415 | 	adminGroup.GET("/enterprises/:enterpriseId/audit-logs", wrapF(adminEnterprise.section("audit-logs")))
backend-go/internal/httpserver/server.go:0416 | 	adminGroup.POST("/enterprises/:enterpriseId/certifications/review", wrapF(adminEnterprise.mutate("certification-review")))
backend-go/internal/httpserver/server.go:0417 | 	adminGroup.POST("/enterprises/:enterpriseId/package/adjust", wrapF(adminEnterprise.mutate("package-adjust")))
backend-go/internal/httpserver/server.go:0418 | 	adminGroup.POST("/enterprises/:enterpriseId/seats/adjust", wrapF(adminEnterprise.mutate("seat-adjust")))
backend-go/internal/httpserver/server.go:0419 | 	adminGroup.POST("/enterprises/:enterpriseId/compute/adjust", wrapF(adminEnterprise.mutate("compute-adjust")))
backend-go/internal/httpserver/server.go:0420 | 	adminGroup.POST("/enterprises/:enterpriseId/recharge", wrapF(adminEnterprise.mutate("recharge")))
backend-go/internal/httpserver/server.go:0421 | 	adminGroup.POST("/enterprises/:enterpriseId/ai-capabilities/configure", wrapF(adminEnterprise.mutate("ai-capability-configure")))
backend-go/internal/httpserver/server.go:0422 | 	adminGroup.POST("/enterprises/:enterpriseId/attribution/change", wrapF(adminEnterprise.mutate("attribution-change")))
backend-go/internal/httpserver/server.go:0423 | 	adminGroup.POST("/enterprises/:enterpriseId/risk/disable", wrapF(adminEnterprise.mutate("risk-disable")))
backend-go/internal/httpserver/server.go:0424 | 	adminGroup.POST("/enterprises/:enterpriseId/risk/restore", wrapF(adminEnterprise.mutate("risk-restore")))
backend-go/internal/httpserver/server.go:0425 | 	adminGroup.POST("/enterprises/:enterpriseId/service-state", wrapF(adminEnterprise.mutate("service-state")))
backend-go/internal/httpserver/server.go:0426 | 	adminGroup.GET("/media/assets", wrapF(media.listAssets))
backend-go/internal/httpserver/server.go:0427 | 	adminGroup.POST("/media/assets/upload", wrapF(media.uploadAsset))
backend-go/internal/httpserver/server.go:0428 | 	adminGroup.POST("/media/assets/batch-upload", wrapF(media.uploadAsset))
backend-go/internal/httpserver/server.go:0429 | 	adminGroup.GET("/media/assets/:id", wrapF(media.getAsset))
backend-go/internal/httpserver/server.go:0430 | 	adminGroup.PUT("/media/assets/:id", wrapF(media.updateAsset))
backend-go/internal/httpserver/server.go:0431 | 	adminGroup.DELETE("/media/assets/:id", wrapF(media.deleteAsset))
backend-go/internal/httpserver/server.go:0432 | 	adminGroup.POST("/media/assets/:id/enable", wrapF(media.enableAsset))
backend-go/internal/httpserver/server.go:0433 | 	adminGroup.POST("/media/assets/:id/disable", wrapF(media.disableAsset))
backend-go/internal/httpserver/server.go:0434 | 	adminGroup.GET("/media/assets/:id/usages", wrapF(media.assetUsages))
backend-go/internal/httpserver/server.go:0435 | 	adminGroup.GET("/media/categories", wrapF(media.listCategories))
backend-go/internal/httpserver/server.go:0436 | 	adminGroup.POST("/media/categories", wrapF(media.createCategory))
backend-go/internal/httpserver/server.go:0437 | 	adminGroup.PUT("/media/categories/:id", wrapF(media.updateCategory))
backend-go/internal/httpserver/server.go:0438 | 	adminGroup.DELETE("/media/categories/:id", wrapF(media.deleteCategory))
backend-go/internal/httpserver/server.go:0439 | 	adminGroup.GET("/page-configs/:pageCode", wrapF(media.getAdminPageConfig))
backend-go/internal/httpserver/server.go:0440 | 	adminGroup.PUT("/page-configs/:pageCode", wrapF(media.savePageDraft))
backend-go/internal/httpserver/server.go:0441 | 	adminGroup.POST("/page-configs/:pageCode/publish", wrapF(media.publishPage))
backend-go/internal/httpserver/server.go:0442 | 	adminGroup.GET("/page-configs/:pageCode/versions", wrapF(media.listPageVersions))
backend-go/internal/httpserver/server.go:0443 | 	adminGroup.POST("/page-configs/:pageCode/rollback/:version", wrapF(media.rollbackPage))
backend-go/internal/httpserver/server.go:0444 | 	adminGroup.GET("/page-slots/:pageCode", wrapF(media.listPageSlots))
backend-go/internal/httpserver/server.go:0445 | 	adminGroup.PUT("/page-slots/:pageCode/:slotKey", wrapF(media.updatePageSlot))
backend-go/internal/httpserver/server.go:0446 | 	adminGroup.GET("/storage/configs", wrapF(files.listConfigs))
backend-go/internal/httpserver/server.go:0447 | 	adminGroup.POST("/storage/configs", wrapF(files.saveConfig(true)))
backend-go/internal/httpserver/server.go:0448 | 	adminGroup.PUT("/storage/configs/:id", wrapF(files.saveConfig(false)))
backend-go/internal/httpserver/server.go:0449 | 	adminGroup.DELETE("/storage/configs/:id", wrapF(files.deleteConfig))
```

<div style="page-break-after: always;"></div>

### 第 42 页

**文件路径：** backend-go/internal/httpserver/server.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。

```go
backend-go/internal/httpserver/server.go:0450 | 	adminGroup.POST("/storage/configs/:id/test", wrapF(files.testConfig))
backend-go/internal/httpserver/server.go:0451 | 	adminGroup.GET("/storage/files", wrapF(files.adminListFiles))
backend-go/internal/httpserver/server.go:0452 | 	adminGroup.GET("/storage/files/:fileId", wrapF(files.adminGetFile))
backend-go/internal/httpserver/server.go:0453 | 	adminGroup.GET("/storage/files/:fileId/download-url", wrapF(files.accessURL(true)))
backend-go/internal/httpserver/server.go:0454 | 	adminGroup.DELETE("/storage/files/:fileId", wrapF(files.deleteFile))
backend-go/internal/httpserver/server.go:0455 | 	adminGroup.POST("/storage/files/:fileId/restore", wrapF(files.restoreFile))
backend-go/internal/httpserver/server.go:0456 | 	adminGroup.DELETE("/storage/files/:fileId/permanent", wrapF(files.permanentDeleteFile))
backend-go/internal/httpserver/server.go:0457 | 	adminGroup.GET("/storage/statistics/overview", wrapF(files.adminOverview))
backend-go/internal/httpserver/server.go:0458 | 	adminGroup.GET("/storage/quotas", wrapF(files.getQuota))
backend-go/internal/httpserver/server.go:0459 | 	adminGroup.PUT("/storage/quotas/:tenantId", wrapF(files.updateQuota))
backend-go/internal/httpserver/server.go:0460 | 	adminGroup.GET("/knowledge/overview", wrapF(knowledgeAPI.adminOverview))
backend-go/internal/httpserver/server.go:0461 | 	adminGroup.GET("/knowledge/:resource", wrapF(knowledgeAPI.adminRecords))
backend-go/internal/httpserver/server.go:0462 | 	adminGroup.POST("/knowledge/:resource", wrapF(knowledgeAPI.saveAdminProfile))
backend-go/internal/httpserver/server.go:0463 | 	adminGroup.PATCH("/knowledge/:resource/:id", wrapF(knowledgeAPI.saveAdminProfile))
backend-go/internal/httpserver/server.go:0464 | 	adminGroup.GET("/customer-attributions", wrapF(admin.customerAttributions))
backend-go/internal/httpserver/server.go:0465 | 	adminGroup.GET("/customers", wrapF(admin.customers))
backend-go/internal/httpserver/server.go:0466 | 	adminGroup.GET("/customers/:id/360", wrapF(admin.customer360))
backend-go/internal/httpserver/server.go:0467 | 	adminGroup.POST("/customers", wrapF(admin.createCustomer))
backend-go/internal/httpserver/server.go:0468 | 	adminGroup.PATCH("/customers/:id", wrapF(admin.updateCustomer))
backend-go/internal/httpserver/server.go:0469 | 	adminGroup.GET("/customers/:id/identities", wrapF(admin.customerIdentities))
backend-go/internal/httpserver/server.go:0470 | 	adminGroup.GET("/customers/:id/account-merge-requests", wrapF(admin.customerAuthMergeRequests))
backend-go/internal/httpserver/server.go:0471 | 	adminGroup.POST("/customers/:id/identities/mobile/unlink", wrapF(admin.unlinkCustomerMobile))
backend-go/internal/httpserver/server.go:0472 | 	adminGroup.POST("/customers/:id/identities/wechat-mini-program/unlink", wrapF(admin.unlinkCustomerWeChat))
backend-go/internal/httpserver/server.go:0473 | 	adminGroup.POST("/customers/:id/freeze-login", wrapF(admin.freezeCustomerLogin))
backend-go/internal/httpserver/server.go:0474 | 	adminGroup.POST("/customers/:id/unfreeze-login", wrapF(admin.unfreezeCustomerLogin))
backend-go/internal/httpserver/server.go:0475 | 	adminGroup.POST("/customers/:id/logout-all", wrapF(admin.forceLogoutCustomer))
backend-go/internal/httpserver/server.go:0476 | 	adminGroup.GET("/account-merge-requests", wrapF(admin.authMergeRequests))
backend-go/internal/httpserver/server.go:0477 | 	adminGroup.POST("/account-merge-requests/:id/status", wrapF(admin.updateAuthMergeRequest))
backend-go/internal/httpserver/server.go:0478 | 	adminGroup.GET("/account-merge-requests/:id/preview", wrapF(admin.previewAuthMergeRequest))
backend-go/internal/httpserver/server.go:0479 | 	adminGroup.POST("/account-merge-requests/:id/execute", wrapF(admin.executeAuthMergeRequest))
backend-go/internal/httpserver/server.go:0480 | 	adminGroup.POST("/customers/:id/sync-newapi", wrapF(admin.syncCustomerNewAPI))
backend-go/internal/httpserver/server.go:0481 | 	adminGroup.GET("/newapi/groups", wrapF(admin.newAPIGroups))
backend-go/internal/httpserver/server.go:0482 | 	adminGroup.GET("/channel-agents", wrapF(admin.channelAgents))
backend-go/internal/httpserver/server.go:0483 | 	adminGroup.POST("/channel-agents", wrapF(admin.createChannelAgent))
backend-go/internal/httpserver/server.go:0484 | 	adminGroup.GET("/channel-agents/tree", wrapF(admin.channelAgentTree))
backend-go/internal/httpserver/server.go:0485 | 	adminGroup.PATCH("/channel-agents/:id", wrapF(admin.updateChannelAgent))
backend-go/internal/httpserver/server.go:0486 | 	adminGroup.GET("/operation-centers", wrapF(admin.operationCenters))
backend-go/internal/httpserver/server.go:0487 | 	adminGroup.GET("/products", wrapF(admin.products))
backend-go/internal/httpserver/server.go:0488 | 	adminGroup.PATCH("/products/:id", wrapF(admin.updateProduct))
backend-go/internal/httpserver/server.go:0489 | 	adminGroup.GET("/plans", wrapF(admin.plans))
backend-go/internal/httpserver/server.go:0490 | 	adminGroup.PATCH("/plans/:id", wrapF(admin.updatePlan))
backend-go/internal/httpserver/server.go:0491 | 	adminGroup.GET("/plans/:id/capabilities", wrapF(admin.planCapabilities))
backend-go/internal/httpserver/server.go:0492 | 	adminGroup.PUT("/plans/:id/capabilities", wrapF(admin.updatePlanCapabilities))
backend-go/internal/httpserver/server.go:0493 | 	adminGroup.GET("/orders", wrapF(admin.orders))
backend-go/internal/httpserver/server.go:0494 | 	adminGroup.GET("/orders/:id/timeline", wrapF(admin.orderTimeline))
backend-go/internal/httpserver/server.go:0495 | 	adminGroup.POST("/orders", wrapF(admin.createOrder))
backend-go/internal/httpserver/server.go:0496 | 	adminGroup.POST("/orders/:id/mark-paid", wrapF(admin.markOrderPaid))
backend-go/internal/httpserver/server.go:0497 | 	adminGroup.POST("/orders/:id/renew", wrapF(admin.renewOrder))
backend-go/internal/httpserver/server.go:0498 | 	adminGroup.GET("/payment/virtual/overview", wrapF(virtualPayment.adminOverview))
backend-go/internal/httpserver/server.go:0499 | 	adminGroup.GET("/payment/orders", wrapF(paymentCenter.adminOrders))
```

<div style="page-break-after: always;"></div>

### 第 43 页

**文件路径：** backend-go/internal/httpserver/server.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。

```go
backend-go/internal/httpserver/server.go:0500 | 	adminGroup.GET("/payment/orders/:orderNo", wrapF(paymentCenter.adminOrder))
backend-go/internal/httpserver/server.go:0501 | 	adminGroup.GET("/payment/transactions", wrapF(paymentCenter.adminTransactions))
backend-go/internal/httpserver/server.go:0502 | 	adminGroup.GET("/payment/fulfillments", wrapF(paymentCenter.adminFulfillments))
backend-go/internal/httpserver/server.go:0503 | 	adminGroup.POST("/payment/fulfillments/:id/retry", wrapF(paymentCenter.adminRetryFulfillment))
backend-go/internal/httpserver/server.go:0504 | 	adminPaymentGroup.GET("/payment/orders", wrapF(paymentCenter.adminOrders))
backend-go/internal/httpserver/server.go:0505 | 	adminPaymentGroup.GET("/payment/orders/:orderNo", wrapF(paymentCenter.adminOrder))
backend-go/internal/httpserver/server.go:0506 | 	adminPaymentGroup.GET("/payment/transactions", wrapF(paymentCenter.adminTransactions))
backend-go/internal/httpserver/server.go:0507 | 	adminPaymentGroup.GET("/payment/fulfillments", wrapF(paymentCenter.adminFulfillments))
backend-go/internal/httpserver/server.go:0508 | 	adminPaymentGroup.POST("/payment/fulfillments/:id/retry", wrapF(paymentCenter.adminRetryFulfillment))
backend-go/internal/httpserver/server.go:0509 | 	adminGroup.GET("/payment/virtual/products", wrapF(virtualPayment.adminProducts))
backend-go/internal/httpserver/server.go:0510 | 	adminGroup.PATCH("/payment/virtual/mappings/:id", wrapF(virtualPayment.adminUpdateMapping))
backend-go/internal/httpserver/server.go:0511 | 	adminGroup.GET("/payment/virtual/orders", wrapF(virtualPayment.adminList("orders")))
backend-go/internal/httpserver/server.go:0512 | 	adminGroup.GET("/payment/virtual/records", wrapF(virtualPayment.adminList("records")))
backend-go/internal/httpserver/server.go:0513 | 	adminGroup.GET("/payment/virtual/notifications", wrapF(virtualPayment.adminList("notifications")))
backend-go/internal/httpserver/server.go:0514 | 	adminGroup.GET("/payment/virtual/memberships", wrapF(virtualPayment.adminList("memberships")))
backend-go/internal/httpserver/server.go:0515 | 	adminGroup.GET("/payment/virtual/wallet-ledger", wrapF(virtualPayment.adminList("wallet-ledger")))
backend-go/internal/httpserver/server.go:0516 | 	adminGroup.GET("/payment/virtual/refunds", wrapF(virtualPayment.adminList("refunds")))
backend-go/internal/httpserver/server.go:0517 | 	adminGroup.GET("/payment/virtual/failures", wrapF(virtualPayment.adminList("failures")))
backend-go/internal/httpserver/server.go:0518 | 	adminGroup.POST("/payment/virtual/orders/:orderNo/grant", wrapF(virtualPayment.adminGrantOrder))
backend-go/internal/httpserver/server.go:0519 | 	adminGroup.GET("/delivery-projects", wrapF(admin.deliveryProjects))
backend-go/internal/httpserver/server.go:0520 | 	adminGroup.PATCH("/delivery-projects/:id", wrapF(admin.updateDeliveryProject))
backend-go/internal/httpserver/server.go:0521 | 	adminGroup.GET("/generation-tasks", wrapF(admin.generationTasks))
backend-go/internal/httpserver/server.go:0522 | 	adminGroup.GET("/ai/overview", wrapF(admin.aiOverview))
backend-go/internal/httpserver/server.go:0523 | 	adminGroup.GET("/compliance/miniprogram-launch-check", wrapF(admin.miniProgramComplianceCheck))
backend-go/internal/httpserver/server.go:0524 | 	adminGroup.GET("/compliance/legal-documents", wrapF(admin.legalDocuments))
backend-go/internal/httpserver/server.go:0525 | 	adminGroup.PUT("/compliance/legal-documents/:code", wrapF(admin.saveLegalDocument))
backend-go/internal/httpserver/server.go:0526 | 	adminGroup.GET("/compliance/content-audits", wrapF(admin.contentAudits))
backend-go/internal/httpserver/server.go:0527 | 	adminGroup.PATCH("/compliance/content-audits/:id", wrapF(admin.reviewContentAudit))
backend-go/internal/httpserver/server.go:0528 | 	adminGroup.PATCH("/ai/modules/:code", wrapF(admin.updateAIModule))
backend-go/internal/httpserver/server.go:0529 | 	adminGroup.POST("/ai/models", wrapF(admin.createAIModel))
backend-go/internal/httpserver/server.go:0530 | 	adminGroup.PATCH("/ai/models/:id", wrapF(admin.updateAIModel))
backend-go/internal/httpserver/server.go:0531 | 	adminGroup.PATCH("/ai/parameter-schemas/:id", wrapF(admin.updateAIParameterSchema))
backend-go/internal/httpserver/server.go:0532 | 	adminGroup.PATCH("/ai/tenant-module-limits/:id", wrapF(admin.updateTenantModuleLimit))
backend-go/internal/httpserver/server.go:0533 | 	adminGroup.GET("/usage", wrapF(admin.usage))
backend-go/internal/httpserver/server.go:0534 | 	adminGroup.GET("/usage/export", wrapF(admin.exportUsage))
backend-go/internal/httpserver/server.go:0535 | 	adminGroup.GET("/token-records", wrapF(admin.tokenRecords))
backend-go/internal/httpserver/server.go:0536 | 	adminGroup.GET("/commissions", wrapF(admin.commissions))
backend-go/internal/httpserver/server.go:0537 | 	adminGroup.GET("/commission-records", wrapF(admin.commissionRecords))
backend-go/internal/httpserver/server.go:0538 | 	adminGroup.GET("/commission-rules", wrapF(admin.commissionRulesV2))
backend-go/internal/httpserver/server.go:0539 | 	adminGroup.POST("/commission-rules", wrapF(admin.createCommissionRuleV2))
backend-go/internal/httpserver/server.go:0540 | 	adminGroup.PUT("/commission-rules/:id", wrapF(admin.updateCommissionRuleV2))
backend-go/internal/httpserver/server.go:0541 | 	adminGroup.POST("/commissions", wrapF(admin.createCommission))
backend-go/internal/httpserver/server.go:0542 | 	adminGroup.POST("/commissions/:id/approve", wrapF(admin.approveCommission))
backend-go/internal/httpserver/server.go:0543 | 	adminGroup.POST("/commissions/:id/reject", wrapF(admin.rejectCommission))
backend-go/internal/httpserver/server.go:0544 | 	adminGroup.POST("/withdrawals", wrapF(admin.createWithdrawal))
backend-go/internal/httpserver/server.go:0545 | 	adminGroup.POST("/withdrawals/:id/approve", wrapF(admin.approveWithdrawal))
backend-go/internal/httpserver/server.go:0546 | 	adminGroup.POST("/withdrawals/:id/reject", wrapF(admin.rejectWithdrawal))
backend-go/internal/httpserver/server.go:0547 | 	adminGroup.GET("/system/settings", wrapF(admin.systemSettings))
backend-go/internal/httpserver/server.go:0548 | 	adminGroup.PATCH("/system/settings", wrapF(admin.updateSystemSettings))
backend-go/internal/httpserver/server.go:0549 | 	adminGroup.GET("/api/provider-channels", wrapF(admin.apiProviderChannels))
```

<div style="page-break-after: always;"></div>

### 第 44 页

**文件路径：** backend-go/internal/httpserver/server.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。

```go
backend-go/internal/httpserver/server.go:0550 | 	adminGroup.POST("/api/provider-channels", wrapF(admin.createAPIProviderChannel))
backend-go/internal/httpserver/server.go:0551 | 	adminGroup.PATCH("/api/provider-channels/:id", wrapF(admin.updateAPIProviderChannel))
backend-go/internal/httpserver/server.go:0552 | 	adminGroup.POST("/api/provider-channels/:id/test", wrapF(admin.testAPIProviderChannel))
backend-go/internal/httpserver/server.go:0553 | 	adminGroup.POST("/api/provider-channels/:id/fetch-models", wrapF(admin.fetchAPIProviderChannelModels))
backend-go/internal/httpserver/server.go:0554 | 	adminGroup.GET("/api/models", wrapF(admin.apiModels))
backend-go/internal/httpserver/server.go:0555 | 	adminGroup.PATCH("/api/models/:id", wrapF(admin.updateAPIModel))
backend-go/internal/httpserver/server.go:0556 | 	adminGroup.GET("/api/keys", wrapF(admin.apiKeys))
backend-go/internal/httpserver/server.go:0557 | 	adminGroup.POST("/api/keys", wrapF(admin.createAPIKey))
backend-go/internal/httpserver/server.go:0558 | 	adminGroup.PATCH("/api/keys/:id", wrapF(admin.updateAPIKey))
backend-go/internal/httpserver/server.go:0559 | 	adminGroup.GET("/customer-groups", wrapF(admin.customerGroups))
backend-go/internal/httpserver/server.go:0560 | 	adminGroup.PATCH("/customer-groups/:id", wrapF(admin.updateCustomerGroup))
backend-go/internal/httpserver/server.go:0561 | 	adminGroup.GET("/marketing/overview", wrapF(admin.marketingOverview))
backend-go/internal/httpserver/server.go:0562 | 	adminGroup.GET("/marketing/agent-levels", wrapF(admin.marketingAgentLevels))
backend-go/internal/httpserver/server.go:0563 | 	adminGroup.GET("/marketing/invite-records", wrapF(admin.marketingInviteRecords))
backend-go/internal/httpserver/server.go:0564 | 	adminGroup.GET("/marketing/commission-rules", wrapF(admin.marketingCommissionRules))
backend-go/internal/httpserver/server.go:0565 | 	adminGroup.PATCH("/marketing/commission-rules/:id", wrapF(admin.updateMarketingCommissionRule))
backend-go/internal/httpserver/server.go:0566 | 	adminGroup.GET("/marketing/upgrade-plans", wrapF(admin.marketingUpgradePlans))
backend-go/internal/httpserver/server.go:0567 | 	adminGroup.GET("/marketing/wallets", wrapF(admin.marketingWallets))
backend-go/internal/httpserver/server.go:0568 | 	adminGroup.GET("/marketing/wallet-records", wrapF(admin.marketingWalletRecords))
backend-go/internal/httpserver/server.go:0569 | 	adminGroup.GET("/marketing/settlement-statements", wrapF(admin.marketingSettlementStatements))
backend-go/internal/httpserver/server.go:0570 | 	adminGroup.GET("/billing/overview", wrapF(admin.billingOverview))
backend-go/internal/httpserver/server.go:0571 | 	adminGroup.GET("/billing/rules", wrapF(admin.billingRulesV1))
backend-go/internal/httpserver/server.go:0572 | 	adminGroup.GET("/billing/rules/:id", wrapF(admin.billingRuleV1))
backend-go/internal/httpserver/server.go:0573 | 	adminGroup.POST("/billing/rules/:id/validate", wrapF(admin.validateBillingRuleV1))
backend-go/internal/httpserver/server.go:0574 | 	adminGroup.POST("/billing/rules/:id/publish", wrapF(admin.publishBillingRuleV1))
backend-go/internal/httpserver/server.go:0575 | 	adminGroup.GET("/billing/provider-costs", wrapF(admin.providerCostsV1))
backend-go/internal/httpserver/server.go:0576 | 	adminGroup.PATCH("/billing/provider-costs/:id", wrapF(admin.updateProviderCostV1))
backend-go/internal/httpserver/server.go:0577 | 	adminGroup.GET("/billing/reconciliation", wrapF(admin.reconciliationV1))
backend-go/internal/httpserver/server.go:0578 | 	adminGroup.GET("/billing/wallet-ledger", wrapF(admin.walletLedgerV1))
backend-go/internal/httpserver/server.go:0579 | 	adminGroup.GET("/billing/customers", wrapF(admin.billingCustomers))
backend-go/internal/httpserver/server.go:0580 | 	adminGroup.GET("/billing/products", wrapF(admin.billingProducts))
backend-go/internal/httpserver/server.go:0581 | 	adminGroup.GET("/billing/plans", wrapF(admin.billingPlans))
backend-go/internal/httpserver/server.go:0582 | 	adminGroup.GET("/billing/subscriptions", wrapF(admin.billingSubscriptions))
backend-go/internal/httpserver/server.go:0583 | 	adminGroup.GET("/billing/events", wrapF(admin.billingEvents))
backend-go/internal/httpserver/server.go:0584 | 	adminGroup.GET("/billing/usage-summaries", wrapF(admin.billingUsageSummaries))
backend-go/internal/httpserver/server.go:0585 | 	adminGroup.GET("/billing/billable-metrics", wrapF(admin.billingBillableMetrics))
backend-go/internal/httpserver/server.go:0586 | 	adminGroup.GET("/billing/charges", wrapF(admin.billingCharges))
backend-go/internal/httpserver/server.go:0587 | 	adminGroup.PATCH("/billing/rules/:id", wrapF(admin.updateBillingRule))
backend-go/internal/httpserver/server.go:0588 | 	adminGroup.GET("/billing/fees", wrapF(admin.billingFees))
backend-go/internal/httpserver/server.go:0589 | 	adminGroup.GET("/billing/wallets", wrapF(admin.billingWallets))
backend-go/internal/httpserver/server.go:0590 | 	adminGroup.GET("/billing/coupons", wrapF(admin.billingCoupons))
backend-go/internal/httpserver/server.go:0591 | 	adminGroup.POST("/billing/coupons", wrapF(admin.createBillingCoupon))
backend-go/internal/httpserver/server.go:0592 | 	adminGroup.PATCH("/billing/coupons/:id", wrapF(admin.updateBillingCoupon))
backend-go/internal/httpserver/server.go:0593 | 	adminGroup.GET("/billing/invoices", wrapF(admin.billingInvoices))
backend-go/internal/httpserver/server.go:0594 | 	adminGroup.PATCH("/billing/invoices/:id", wrapF(admin.updateBillingInvoice))
backend-go/internal/httpserver/server.go:0595 | 	adminGroup.GET("/billing/credit-notes", wrapF(admin.billingCreditNotes))
backend-go/internal/httpserver/server.go:0596 | 	adminGroup.POST("/billing/credit-notes", wrapF(admin.createBillingCreditNote))
backend-go/internal/httpserver/server.go:0597 | 	adminGroup.PATCH("/billing/credit-notes/:id", wrapF(admin.reviewBillingCreditNote))
backend-go/internal/httpserver/server.go:0598 | 	adminGroup.GET("/billing/payment-requests", wrapF(admin.billingPaymentRequests))
backend-go/internal/httpserver/server.go:0599 | 	adminGroup.POST("/billing/payment-requests/:id/dunning", wrapF(admin.recordBillingDunning))
```

<div style="page-break-after: always;"></div>

### 第 45 页

**文件路径：** backend-go/internal/httpserver/server.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。

```go
backend-go/internal/httpserver/server.go:0600 | 	adminGroup.GET("/billing/payments", wrapF(admin.billingPayments))
backend-go/internal/httpserver/server.go:0601 | 	adminGroup.PATCH("/billing/subscriptions/:id", wrapF(admin.updateBillingSubscription))
backend-go/internal/httpserver/server.go:0602 | 
backend-go/internal/httpserver/server.go:0603 | 	router.GET("/v1/dashboard/billing/subscription", wrapF(admin.billingSubscription))
backend-go/internal/httpserver/server.go:0604 | 	router.GET("/v1/dashboard/billing/usage", wrapF(admin.billingUsage))
backend-go/internal/httpserver/server.go:0605 | 	registerWirelessCanvasCompatibilityRoutes(router, cfg)
backend-go/internal/httpserver/server.go:0606 | 	router.GET("/", gin.WrapF(staticIndex(cfg.AdminStaticDir)))
backend-go/internal/httpserver/server.go:0607 | 	router.GET("/login", gin.WrapF(staticIndex(cfg.AdminStaticDir)))
backend-go/internal/httpserver/server.go:0608 | 	router.GET("/register", gin.WrapF(staticIndex(cfg.AdminStaticDir)))
backend-go/internal/httpserver/server.go:0609 | 	router.GET("/assets/*filepath", gin.WrapH(staticPrefixFiles("/assets/", filepath.Join(cfg.StaticDir, "assets"))))
backend-go/internal/httpserver/server.go:0610 | 	router.GET("/static/*filepath", gin.WrapH(staticPrefixFiles("/static/", filepath.Join(cfg.StaticDir, "static"))))
backend-go/internal/httpserver/server.go:0611 | 	router.GET("/mobile", wrapF(notFound))
backend-go/internal/httpserver/server.go:0612 | 	router.GET("/mobile/*filepath", wrapF(notFound))
backend-go/internal/httpserver/server.go:0613 | 	router.GET("/pages/*filepath", gin.WrapH(staticPrefixFiles("/pages/", cfg.StaticDir)))
backend-go/internal/httpserver/server.go:0614 | 	router.GET("/admin", wrapF(redirectToAdminSlash))
backend-go/internal/httpserver/server.go:0615 | 	router.GET("/admin/*filepath", gin.WrapH(staticPrefixFiles("/admin/", cfg.AdminStaticDir)))
backend-go/internal/httpserver/server.go:0616 | 	router.GET("/app", wrapF(redirectToRoot))
backend-go/internal/httpserver/server.go:0617 | 	router.GET("/app/*filepath", wrapF(redirectToRoot))
backend-go/internal/httpserver/server.go:0618 | 	router.GET("/workspace", wrapF(redirectToRoot))
backend-go/internal/httpserver/server.go:0619 | 	router.GET("/workspace/*filepath", wrapF(redirectToRoot))
backend-go/internal/httpserver/server.go:0620 | 	router.GET("/agent", gin.WrapF(staticIndex(cfg.AdminStaticDir)))
backend-go/internal/httpserver/server.go:0621 | 	router.GET("/agent/*filepath", gin.WrapH(staticPrefixFiles("/agent/", cfg.AdminStaticDir)))
backend-go/internal/httpserver/server.go:0622 | 	router.GET("/user", wrapF(notFound))
backend-go/internal/httpserver/server.go:0623 | 	router.GET("/user/*filepath", wrapF(notFound))
backend-go/internal/httpserver/server.go:0624 | 	router.NoRoute(redirectUnknownWebToRoot)
backend-go/internal/httpserver/server.go:0625 | 
backend-go/internal/httpserver/server.go:0626 | 	return &http.Server{
backend-go/internal/httpserver/server.go:0627 | 		Addr:              cfg.Addr,
backend-go/internal/httpserver/server.go:0628 | 		Handler:           router,
backend-go/internal/httpserver/server.go:0629 | 		ReadHeaderTimeout: 5 * time.Second,
backend-go/internal/httpserver/server.go:0630 | 		ReadTimeout:       15 * time.Second,
backend-go/internal/httpserver/server.go:0631 | 		WriteTimeout:      15 * time.Minute,
backend-go/internal/httpserver/server.go:0632 | 		IdleTimeout:       60 * time.Second,
backend-go/internal/httpserver/server.go:0633 | 	}
backend-go/internal/httpserver/server.go:0634 | }
backend-go/internal/httpserver/server.go:0635 | 
backend-go/internal/httpserver/server.go:0636 | const requestIDHeader = "X-Request-Id"
backend-go/internal/httpserver/server.go:0637 | const corsAllowedHeaders = "Authorization, Content-Type, Idempotency-Key, X-Request-Id, X-Client-Platform, X-Client-Name, X-Client-Version, X-Client-Language, X-Tenant-Id, X-Organization-Id"
backend-go/internal/httpserver/server.go:0638 | const corsAllowedMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
backend-go/internal/httpserver/server.go:0639 | 
backend-go/internal/httpserver/server.go:0640 | func requestContextMiddleware() gin.HandlerFunc {
backend-go/internal/httpserver/server.go:0641 | 	return func(c *gin.Context) {
backend-go/internal/httpserver/server.go:0642 | 		requestID := sanitizeRequestID(c.GetHeader(requestIDHeader))
backend-go/internal/httpserver/server.go:0643 | 		if requestID == "" {
backend-go/internal/httpserver/server.go:0644 | 			requestID = newRequestID()
backend-go/internal/httpserver/server.go:0645 | 		}
backend-go/internal/httpserver/server.go:0646 | 		c.Set("request_id", requestID)
backend-go/internal/httpserver/server.go:0647 | 		c.Header(requestIDHeader, requestID)
backend-go/internal/httpserver/server.go:0648 | 		c.Header("Access-Control-Expose-Headers", requestIDHeader)
backend-go/internal/httpserver/server.go:0649 | 		c.Next()
```

<div style="page-break-after: always;"></div>

### 第 46 页

**文件路径：** backend-go/internal/httpserver/server.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。

```go
backend-go/internal/httpserver/server.go:0650 | 	}
backend-go/internal/httpserver/server.go:0651 | }
backend-go/internal/httpserver/server.go:0652 | 
backend-go/internal/httpserver/server.go:0653 | func corsMiddleware(allowedOrigins string) gin.HandlerFunc {
backend-go/internal/httpserver/server.go:0654 | 	allowed, allowAll := parseAllowedOrigins(allowedOrigins)
backend-go/internal/httpserver/server.go:0655 | 	return func(c *gin.Context) {
backend-go/internal/httpserver/server.go:0656 | 		origin := strings.TrimSpace(c.GetHeader("Origin"))
backend-go/internal/httpserver/server.go:0657 | 		if origin == "" {
backend-go/internal/httpserver/server.go:0658 | 			c.Next()
backend-go/internal/httpserver/server.go:0659 | 			return
backend-go/internal/httpserver/server.go:0660 | 		}
backend-go/internal/httpserver/server.go:0661 | 		if !allowAll {
backend-go/internal/httpserver/server.go:0662 | 			if _, ok := allowed[origin]; !ok {
backend-go/internal/httpserver/server.go:0663 | 				c.Next()
backend-go/internal/httpserver/server.go:0664 | 				return
backend-go/internal/httpserver/server.go:0665 | 			}
backend-go/internal/httpserver/server.go:0666 | 			c.Header("Access-Control-Allow-Origin", origin)
backend-go/internal/httpserver/server.go:0667 | 			c.Header("Access-Control-Allow-Credentials", "true")
backend-go/internal/httpserver/server.go:0668 | 			c.Header("Vary", "Origin")
backend-go/internal/httpserver/server.go:0669 | 		} else {
backend-go/internal/httpserver/server.go:0670 | 			c.Header("Access-Control-Allow-Origin", "*")
backend-go/internal/httpserver/server.go:0671 | 		}
backend-go/internal/httpserver/server.go:0672 | 		c.Header("Access-Control-Allow-Methods", corsAllowedMethods)
backend-go/internal/httpserver/server.go:0673 | 		c.Header("Access-Control-Allow-Headers", corsAllowedHeaders)
backend-go/internal/httpserver/server.go:0674 | 		c.Header("Access-Control-Expose-Headers", requestIDHeader)
backend-go/internal/httpserver/server.go:0675 | 		if c.Request.Method == http.MethodOptions {
backend-go/internal/httpserver/server.go:0676 | 			c.AbortWithStatus(http.StatusNoContent)
backend-go/internal/httpserver/server.go:0677 | 			return
backend-go/internal/httpserver/server.go:0678 | 		}
backend-go/internal/httpserver/server.go:0679 | 		c.Next()
backend-go/internal/httpserver/server.go:0680 | 	}
backend-go/internal/httpserver/server.go:0681 | }
backend-go/internal/httpserver/server.go:0682 | 
backend-go/internal/httpserver/server.go:0683 | func parseAllowedOrigins(value string) (map[string]struct{}, bool) {
backend-go/internal/httpserver/server.go:0684 | 	allowed := make(map[string]struct{})
backend-go/internal/httpserver/server.go:0685 | 	for _, item := range strings.Split(value, ",") {
backend-go/internal/httpserver/server.go:0686 | 		origin := strings.TrimSpace(item)
backend-go/internal/httpserver/server.go:0687 | 		if origin == "" {
backend-go/internal/httpserver/server.go:0688 | 			continue
backend-go/internal/httpserver/server.go:0689 | 		}
backend-go/internal/httpserver/server.go:0690 | 		if origin == "*" {
backend-go/internal/httpserver/server.go:0691 | 			return allowed, true
backend-go/internal/httpserver/server.go:0692 | 		}
backend-go/internal/httpserver/server.go:0693 | 		allowed[origin] = struct{}{}
backend-go/internal/httpserver/server.go:0694 | 	}
backend-go/internal/httpserver/server.go:0695 | 	return allowed, false
backend-go/internal/httpserver/server.go:0696 | }
backend-go/internal/httpserver/server.go:0697 | 
backend-go/internal/httpserver/server.go:0698 | func sanitizeRequestID(value string) string {
backend-go/internal/httpserver/server.go:0699 | 	value = strings.TrimSpace(value)
```

<div style="page-break-after: always;"></div>

### 第 47 页

**文件路径：** backend-go/internal/httpserver/server.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。

```go
backend-go/internal/httpserver/server.go:0700 | 	if len(value) == 0 || len(value) > 128 {
backend-go/internal/httpserver/server.go:0701 | 		return ""
backend-go/internal/httpserver/server.go:0702 | 	}
backend-go/internal/httpserver/server.go:0703 | 	for _, r := range value {
backend-go/internal/httpserver/server.go:0704 | 		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':' {
backend-go/internal/httpserver/server.go:0705 | 			continue
backend-go/internal/httpserver/server.go:0706 | 		}
backend-go/internal/httpserver/server.go:0707 | 		return ""
backend-go/internal/httpserver/server.go:0708 | 	}
backend-go/internal/httpserver/server.go:0709 | 	return value
backend-go/internal/httpserver/server.go:0710 | }
backend-go/internal/httpserver/server.go:0711 | 
backend-go/internal/httpserver/server.go:0712 | func newRequestID() string {
backend-go/internal/httpserver/server.go:0713 | 	var randomBytes [8]byte
backend-go/internal/httpserver/server.go:0714 | 	if _, err := rand.Read(randomBytes[:]); err != nil {
backend-go/internal/httpserver/server.go:0715 | 		return "req_" + strconv.FormatInt(time.Now().UnixNano(), 36)
backend-go/internal/httpserver/server.go:0716 | 	}
backend-go/internal/httpserver/server.go:0717 | 	return "req_" + strconv.FormatInt(time.Now().UnixNano(), 36) + "_" + hex.EncodeToString(randomBytes[:])
backend-go/internal/httpserver/server.go:0718 | }
backend-go/internal/httpserver/server.go:0719 | 
backend-go/internal/httpserver/server.go:0720 | func wrapF(handler http.HandlerFunc) gin.HandlerFunc {
backend-go/internal/httpserver/server.go:0721 | 	return func(c *gin.Context) {
backend-go/internal/httpserver/server.go:0722 | 		for _, param := range c.Params {
backend-go/internal/httpserver/server.go:0723 | 			c.Request.SetPathValue(param.Key, param.Value)
backend-go/internal/httpserver/server.go:0724 | 		}
backend-go/internal/httpserver/server.go:0725 | 		handler(c.Writer, c.Request)
backend-go/internal/httpserver/server.go:0726 | 	}
backend-go/internal/httpserver/server.go:0727 | }
backend-go/internal/httpserver/server.go:0728 | 
backend-go/internal/httpserver/server.go:0729 | func redirectToAdminSlash(w http.ResponseWriter, r *http.Request) {
backend-go/internal/httpserver/server.go:0730 | 	http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
backend-go/internal/httpserver/server.go:0731 | }
backend-go/internal/httpserver/server.go:0732 | 
backend-go/internal/httpserver/server.go:0733 | func redirectToRoot(w http.ResponseWriter, r *http.Request) {
backend-go/internal/httpserver/server.go:0734 | 	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
backend-go/internal/httpserver/server.go:0735 | }
backend-go/internal/httpserver/server.go:0736 | 
backend-go/internal/httpserver/server.go:0737 | func health(w http.ResponseWriter, _ *http.Request) {
backend-go/internal/httpserver/server.go:0738 | 	w.Header().Set("Content-Type", "application/json; charset=utf-8")
backend-go/internal/httpserver/server.go:0739 | 	_ = json.NewEncoder(w).Encode(map[string]string{
backend-go/internal/httpserver/server.go:0740 | 		"status":  "ok",
backend-go/internal/httpserver/server.go:0741 | 		"service": "xianzhi-ai-go-gin",
backend-go/internal/httpserver/server.go:0742 | 	})
backend-go/internal/httpserver/server.go:0743 | }
backend-go/internal/httpserver/server.go:0744 | 
backend-go/internal/httpserver/server.go:0745 | func notFound(w http.ResponseWriter, r *http.Request) {
backend-go/internal/httpserver/server.go:0746 | 	http.NotFound(w, r)
backend-go/internal/httpserver/server.go:0747 | }
backend-go/internal/httpserver/server.go:0748 | 
backend-go/internal/httpserver/server.go:0749 | func writeJSON(w http.ResponseWriter, value any) {
```

<div style="page-break-after: always;"></div>

### 第 48 页

**文件路径：** backend-go/internal/httpserver/server.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。

```go
backend-go/internal/httpserver/server.go:0750 | 	w.Header().Set("Content-Type", "application/json; charset=utf-8")
backend-go/internal/httpserver/server.go:0751 | 	_ = json.NewEncoder(w).Encode(value)
backend-go/internal/httpserver/server.go:0752 | }
backend-go/internal/httpserver/server.go:0753 | 
backend-go/internal/httpserver/server.go:0754 | func writeError(w http.ResponseWriter, status int, err error) {
backend-go/internal/httpserver/server.go:0755 | 	w.Header().Set("Content-Type", "application/json; charset=utf-8")
backend-go/internal/httpserver/server.go:0756 | 	w.WriteHeader(status)
backend-go/internal/httpserver/server.go:0757 | 	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
backend-go/internal/httpserver/server.go:0758 | }
backend-go/internal/httpserver/server.go:0759 | 
backend-go/internal/httpserver/server.go:0760 | func staticIndex(root string) http.HandlerFunc {
backend-go/internal/httpserver/server.go:0761 | 	return func(w http.ResponseWriter, r *http.Request) {
backend-go/internal/httpserver/server.go:0762 | 		setStaticCacheHeaders(w, filepath.Join(root, "index.html"), true)
backend-go/internal/httpserver/server.go:0763 | 		http.ServeFile(w, r, filepath.Join(root, "index.html"))
backend-go/internal/httpserver/server.go:0764 | 	}
backend-go/internal/httpserver/server.go:0765 | }
backend-go/internal/httpserver/server.go:0766 | 
backend-go/internal/httpserver/server.go:0767 | func platformIndex(mobileRoot string, desktopRoot string) http.HandlerFunc {
backend-go/internal/httpserver/server.go:0768 | 	mobileIndex := staticIndex(mobileRoot)
backend-go/internal/httpserver/server.go:0769 | 	desktopIndex := staticIndex(desktopRoot)
backend-go/internal/httpserver/server.go:0770 | 	return func(w http.ResponseWriter, r *http.Request) {
backend-go/internal/httpserver/server.go:0771 | 		if isMobileWebRequest(r) {
backend-go/internal/httpserver/server.go:0772 | 			mobileIndex(w, r)
backend-go/internal/httpserver/server.go:0773 | 			return
backend-go/internal/httpserver/server.go:0774 | 		}
backend-go/internal/httpserver/server.go:0775 | 		desktopIndex(w, r)
backend-go/internal/httpserver/server.go:0776 | 	}
backend-go/internal/httpserver/server.go:0777 | }
backend-go/internal/httpserver/server.go:0778 | 
backend-go/internal/httpserver/server.go:0779 | func isMobileWebRequest(r *http.Request) bool {
backend-go/internal/httpserver/server.go:0780 | 	userAgent := strings.ToLower(r.UserAgent())
backend-go/internal/httpserver/server.go:0781 | 	return strings.Contains(userAgent, "android") || strings.Contains(userAgent, "iphone") || strings.Contains(userAgent, "ipad") || strings.Contains(userAgent, "ipod") || strings.Contains(userAgent, "mobile")
backend-go/internal/httpserver/server.go:0782 | }
backend-go/internal/httpserver/server.go:0783 | 
backend-go/internal/httpserver/server.go:0784 | func mobileIndexOrDesktopRedirect(root string) http.HandlerFunc {
backend-go/internal/httpserver/server.go:0785 | 	mobileIndex := staticIndex(root)
backend-go/internal/httpserver/server.go:0786 | 	return func(w http.ResponseWriter, r *http.Request) {
backend-go/internal/httpserver/server.go:0787 | 		if isMobileWebRequest(r) {
backend-go/internal/httpserver/server.go:0788 | 			mobileIndex(w, r)
backend-go/internal/httpserver/server.go:0789 | 			return
backend-go/internal/httpserver/server.go:0790 | 		}
backend-go/internal/httpserver/server.go:0791 | 		redirectToRoot(w, r)
backend-go/internal/httpserver/server.go:0792 | 	}
backend-go/internal/httpserver/server.go:0793 | }
backend-go/internal/httpserver/server.go:0794 | 
backend-go/internal/httpserver/server.go:0795 | func mobilePrefixOrDesktopRedirect(prefix string, root string) http.Handler {
backend-go/internal/httpserver/server.go:0796 | 	mobileFiles := staticPrefixFiles(prefix, root)
backend-go/internal/httpserver/server.go:0797 | 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
backend-go/internal/httpserver/server.go:0798 | 		if isMobileWebRequest(r) {
backend-go/internal/httpserver/server.go:0799 | 			mobileFiles.ServeHTTP(w, r)
```

<div style="page-break-after: always;"></div>

### 第 49 页

**文件路径：** backend-go/internal/httpserver/server.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。

```go
backend-go/internal/httpserver/server.go:0800 | 			return
backend-go/internal/httpserver/server.go:0801 | 		}
backend-go/internal/httpserver/server.go:0802 | 		redirectToRoot(w, r)
backend-go/internal/httpserver/server.go:0803 | 	})
backend-go/internal/httpserver/server.go:0804 | }
backend-go/internal/httpserver/server.go:0805 | 
backend-go/internal/httpserver/server.go:0806 | func staticFiles(root string) http.HandlerFunc {
backend-go/internal/httpserver/server.go:0807 | 	fileServer := http.FileServer(http.Dir(root))
backend-go/internal/httpserver/server.go:0808 | 	return func(w http.ResponseWriter, r *http.Request) {
backend-go/internal/httpserver/server.go:0809 | 		cleanURLPath := path.Clean("/" + r.URL.Path)
backend-go/internal/httpserver/server.go:0810 | 		localPath := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(cleanURLPath, "/")))
backend-go/internal/httpserver/server.go:0811 | 		if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
backend-go/internal/httpserver/server.go:0812 | 			setStaticCacheHeaders(w, localPath, false)
backend-go/internal/httpserver/server.go:0813 | 			fileServer.ServeHTTP(w, r)
backend-go/internal/httpserver/server.go:0814 | 			return
backend-go/internal/httpserver/server.go:0815 | 		}
backend-go/internal/httpserver/server.go:0816 | 		setStaticCacheHeaders(w, filepath.Join(root, "index.html"), true)
backend-go/internal/httpserver/server.go:0817 | 		http.ServeFile(w, r, filepath.Join(root, "index.html"))
backend-go/internal/httpserver/server.go:0818 | 	}
backend-go/internal/httpserver/server.go:0819 | }
backend-go/internal/httpserver/server.go:0820 | 
backend-go/internal/httpserver/server.go:0821 | func staticOrAPINotFound(root string) gin.HandlerFunc {
backend-go/internal/httpserver/server.go:0822 | 	staticHandler := staticFiles(root)
backend-go/internal/httpserver/server.go:0823 | 	return func(c *gin.Context) {
backend-go/internal/httpserver/server.go:0824 | 		requestPath := path.Clean("/" + c.Request.URL.Path)
backend-go/internal/httpserver/server.go:0825 | 		if strings.HasPrefix(requestPath, "/api/") || strings.HasPrefix(requestPath, "/v1/") {
backend-go/internal/httpserver/server.go:0826 | 			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
backend-go/internal/httpserver/server.go:0827 | 			return
backend-go/internal/httpserver/server.go:0828 | 		}
backend-go/internal/httpserver/server.go:0829 | 		staticHandler(c.Writer, c.Request)
backend-go/internal/httpserver/server.go:0830 | 	}
backend-go/internal/httpserver/server.go:0831 | }
backend-go/internal/httpserver/server.go:0832 | 
backend-go/internal/httpserver/server.go:0833 | func platformOrAPINotFound(mobileRoot string) gin.HandlerFunc {
backend-go/internal/httpserver/server.go:0834 | 	mobileHandler := staticFiles(mobileRoot)
backend-go/internal/httpserver/server.go:0835 | 	return func(c *gin.Context) {
backend-go/internal/httpserver/server.go:0836 | 		requestPath := path.Clean("/" + c.Request.URL.Path)
backend-go/internal/httpserver/server.go:0837 | 		if strings.HasPrefix(requestPath, "/api/") || strings.HasPrefix(requestPath, "/v1/") {
backend-go/internal/httpserver/server.go:0838 | 			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
backend-go/internal/httpserver/server.go:0839 | 			return
backend-go/internal/httpserver/server.go:0840 | 		}
backend-go/internal/httpserver/server.go:0841 | 		if isMobileWebRequest(c.Request) {
backend-go/internal/httpserver/server.go:0842 | 			mobileHandler(c.Writer, c.Request)
backend-go/internal/httpserver/server.go:0843 | 			return
backend-go/internal/httpserver/server.go:0844 | 		}
backend-go/internal/httpserver/server.go:0845 | 		redirectToRoot(c.Writer, c.Request)
backend-go/internal/httpserver/server.go:0846 | 	}
backend-go/internal/httpserver/server.go:0847 | }
backend-go/internal/httpserver/server.go:0848 | 
backend-go/internal/httpserver/server.go:0849 | func redirectUnknownWebToRoot(c *gin.Context) {
```

<div style="page-break-after: always;"></div>

### 第 50 页

**文件路径：** backend-go/internal/httpserver/server.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。

```go
backend-go/internal/httpserver/server.go:0850 | 	requestPath := path.Clean("/" + c.Request.URL.Path)
backend-go/internal/httpserver/server.go:0851 | 	if strings.HasPrefix(requestPath, "/api/") || strings.HasPrefix(requestPath, "/v1/") {
backend-go/internal/httpserver/server.go:0852 | 		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
backend-go/internal/httpserver/server.go:0853 | 		return
backend-go/internal/httpserver/server.go:0854 | 	}
backend-go/internal/httpserver/server.go:0855 | 	redirectToRoot(c.Writer, c.Request)
backend-go/internal/httpserver/server.go:0856 | }
backend-go/internal/httpserver/server.go:0857 | 
backend-go/internal/httpserver/server.go:0858 | func staticPrefixFiles(prefix string, root string) http.Handler {
backend-go/internal/httpserver/server.go:0859 | 	fileServer := http.StripPrefix(strings.TrimSuffix(prefix, "/"), http.FileServer(http.Dir(root)))
backend-go/internal/httpserver/server.go:0860 | 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
backend-go/internal/httpserver/server.go:0861 | 		cleanURLPath := path.Clean("/" + strings.TrimPrefix(r.URL.Path, prefix))
backend-go/internal/httpserver/server.go:0862 | 		localPath := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(cleanURLPath, "/")))
backend-go/internal/httpserver/server.go:0863 | 		if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
backend-go/internal/httpserver/server.go:0864 | 			setStaticCacheHeaders(w, localPath, false)
backend-go/internal/httpserver/server.go:0865 | 			fileServer.ServeHTTP(w, r)
backend-go/internal/httpserver/server.go:0866 | 			return
backend-go/internal/httpserver/server.go:0867 | 		}
backend-go/internal/httpserver/server.go:0868 | 		setStaticCacheHeaders(w, filepath.Join(root, "index.html"), true)
backend-go/internal/httpserver/server.go:0869 | 		http.ServeFile(w, r, filepath.Join(root, "index.html"))
backend-go/internal/httpserver/server.go:0870 | 	})
backend-go/internal/httpserver/server.go:0871 | }
backend-go/internal/httpserver/server.go:0872 | 
backend-go/internal/httpserver/server.go:0873 | func setStaticCacheHeaders(w http.ResponseWriter, localPath string, index bool) {
backend-go/internal/httpserver/server.go:0874 | 	if index || strings.EqualFold(filepath.Ext(localPath), ".html") {
backend-go/internal/httpserver/server.go:0875 | 		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
backend-go/internal/httpserver/server.go:0876 | 		w.Header().Set("Pragma", "no-cache")
backend-go/internal/httpserver/server.go:0877 | 		return
backend-go/internal/httpserver/server.go:0878 | 	}
backend-go/internal/httpserver/server.go:0879 | 	w.Header().Set("Cache-Control", "public, max-age=604800")
backend-go/internal/httpserver/server.go:0880 | 	w.Header().Del("Pragma")
backend-go/internal/httpserver/server.go:0881 | }
backend-go/internal/httpserver/server.go:0882 | 
backend-go/internal/httpserver/server.go:0883 | func gzipMiddleware() gin.HandlerFunc {
backend-go/internal/httpserver/server.go:0884 | 	return func(c *gin.Context) {
backend-go/internal/httpserver/server.go:0885 | 		if !gzipEligibleRequest(c.Request) {
backend-go/internal/httpserver/server.go:0886 | 			c.Next()
backend-go/internal/httpserver/server.go:0887 | 			return
backend-go/internal/httpserver/server.go:0888 | 		}
backend-go/internal/httpserver/server.go:0889 | 		writer := gzip.NewWriter(c.Writer)
backend-go/internal/httpserver/server.go:0890 | 		defer writer.Close()
backend-go/internal/httpserver/server.go:0891 | 		c.Header("Content-Encoding", "gzip")
backend-go/internal/httpserver/server.go:0892 | 		c.Header("Vary", "Accept-Encoding")
backend-go/internal/httpserver/server.go:0893 | 		c.Writer.Header().Del("Content-Length")
backend-go/internal/httpserver/server.go:0894 | 		c.Writer = &gzipResponseWriter{ResponseWriter: c.Writer, writer: writer}
backend-go/internal/httpserver/server.go:0895 | 		c.Next()
backend-go/internal/httpserver/server.go:0896 | 	}
backend-go/internal/httpserver/server.go:0897 | }
backend-go/internal/httpserver/server.go:0898 | 
backend-go/internal/httpserver/server.go:0899 | func gzipEligibleRequest(r *http.Request) bool {
```

<div style="page-break-after: always;"></div>

### 第 51 页

**文件路径：** backend-go/internal/httpserver/server.go；backend-go/internal/httpserver/auth_flow_api.go  
**代码说明：** Gin 中间件、模块装配与 REST API 路由注册。；认证、注册、会话与账号安全流程。

```text
backend-go/internal/httpserver/server.go:0900 | 	if r == nil || r.Method == http.MethodHead || r.Header.Get("Range") != "" {
backend-go/internal/httpserver/server.go:0901 | 		return false
backend-go/internal/httpserver/server.go:0902 | 	}
backend-go/internal/httpserver/server.go:0903 | 	if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
backend-go/internal/httpserver/server.go:0904 | 		return false
backend-go/internal/httpserver/server.go:0905 | 	}
backend-go/internal/httpserver/server.go:0906 | 	if strings.HasSuffix(r.URL.Path, ".json") {
backend-go/internal/httpserver/server.go:0907 | 		return true
backend-go/internal/httpserver/server.go:0908 | 	}
backend-go/internal/httpserver/server.go:0909 | 	switch strings.ToLower(path.Ext(r.URL.Path)) {
backend-go/internal/httpserver/server.go:0910 | 	case ".css", ".js", ".html", ".svg", ".txt", ".md", ".map":
backend-go/internal/httpserver/server.go:0911 | 		return true
backend-go/internal/httpserver/server.go:0912 | 	default:
backend-go/internal/httpserver/server.go:0913 | 		return false
backend-go/internal/httpserver/server.go:0914 | 	}
backend-go/internal/httpserver/server.go:0915 | }
backend-go/internal/httpserver/server.go:0916 | 
backend-go/internal/httpserver/server.go:0917 | type gzipResponseWriter struct {
backend-go/internal/httpserver/server.go:0918 | 	gin.ResponseWriter
backend-go/internal/httpserver/server.go:0919 | 	writer *gzip.Writer
backend-go/internal/httpserver/server.go:0920 | }
backend-go/internal/httpserver/server.go:0921 | 
backend-go/internal/httpserver/server.go:0922 | func (w *gzipResponseWriter) Write(data []byte) (int, error) {
backend-go/internal/httpserver/server.go:0923 | 	w.Header().Del("Content-Length")
backend-go/internal/httpserver/server.go:0924 | 	return w.writer.Write(data)
backend-go/internal/httpserver/server.go:0925 | }
backend-go/internal/httpserver/server.go:0926 | 
backend-go/internal/httpserver/server.go:0927 | func (w *gzipResponseWriter) WriteString(data string) (int, error) {
backend-go/internal/httpserver/server.go:0928 | 	w.Header().Del("Content-Length")
backend-go/internal/httpserver/server.go:0929 | 	return w.writer.Write([]byte(data))
backend-go/internal/httpserver/server.go:0930 | }
backend-go/internal/httpserver/server.go:0931 | 
backend-go/internal/httpserver/server.go:0932 | func (w *gzipResponseWriter) WriteHeader(statusCode int) {
backend-go/internal/httpserver/server.go:0933 | 	w.Header().Del("Content-Length")
backend-go/internal/httpserver/server.go:0934 | 	w.ResponseWriter.WriteHeader(statusCode)
backend-go/internal/httpserver/server.go:0935 | }
backend-go/internal/httpserver/auth_flow_api.go:0001 | package httpserver
backend-go/internal/httpserver/auth_flow_api.go:0002 | 
backend-go/internal/httpserver/auth_flow_api.go:0003 | import (
backend-go/internal/httpserver/auth_flow_api.go:0004 | 	"bytes"
backend-go/internal/httpserver/auth_flow_api.go:0005 | 	"context"
backend-go/internal/httpserver/auth_flow_api.go:0006 | 	"crypto/rand"
backend-go/internal/httpserver/auth_flow_api.go:0007 | 	"crypto/sha256"
backend-go/internal/httpserver/auth_flow_api.go:0008 | 	"crypto/subtle"
backend-go/internal/httpserver/auth_flow_api.go:0009 | 	"encoding/json"
backend-go/internal/httpserver/auth_flow_api.go:0010 | 	"errors"
backend-go/internal/httpserver/auth_flow_api.go:0011 | 	"fmt"
backend-go/internal/httpserver/auth_flow_api.go:0012 | 	"math/big"
backend-go/internal/httpserver/auth_flow_api.go:0013 | 	"net/http"
backend-go/internal/httpserver/auth_flow_api.go:0014 | 	"net/url"
```

<div style="page-break-after: always;"></div>

### 第 52 页

**文件路径：** backend-go/internal/httpserver/auth_flow_api.go  
**代码说明：** 认证、注册、会话与账号安全流程。

```go
backend-go/internal/httpserver/auth_flow_api.go:0015 | 	"os"
backend-go/internal/httpserver/auth_flow_api.go:0016 | 	"strings"
backend-go/internal/httpserver/auth_flow_api.go:0017 | 	"sync"
backend-go/internal/httpserver/auth_flow_api.go:0018 | 	"time"
backend-go/internal/httpserver/auth_flow_api.go:0019 | )
backend-go/internal/httpserver/auth_flow_api.go:0020 | 
backend-go/internal/httpserver/auth_flow_api.go:0021 | const (
backend-go/internal/httpserver/auth_flow_api.go:0022 | 	smsCodeTTL      = 5 * time.Minute
backend-go/internal/httpserver/auth_flow_api.go:0023 | 	smsSendInterval = 60 * time.Second
backend-go/internal/httpserver/auth_flow_api.go:0024 | 	smsMaxAttempts  = 5
backend-go/internal/httpserver/auth_flow_api.go:0025 | )
backend-go/internal/httpserver/auth_flow_api.go:0026 | 
backend-go/internal/httpserver/auth_flow_api.go:0027 | type authFlowCoordinator struct {
backend-go/internal/httpserver/auth_flow_api.go:0028 | 	registrationMu sync.Mutex
backend-go/internal/httpserver/auth_flow_api.go:0029 | 	smsMu          sync.Mutex
backend-go/internal/httpserver/auth_flow_api.go:0030 | 	smsChallenges  map[string]smsChallenge
backend-go/internal/httpserver/auth_flow_api.go:0031 | 	smsNextSend    map[string]time.Time
backend-go/internal/httpserver/auth_flow_api.go:0032 | }
backend-go/internal/httpserver/auth_flow_api.go:0033 | 
backend-go/internal/httpserver/auth_flow_api.go:0034 | type smsChallenge struct {
backend-go/internal/httpserver/auth_flow_api.go:0035 | 	codeHash   [32]byte
backend-go/internal/httpserver/auth_flow_api.go:0036 | 	expiresAt  time.Time
backend-go/internal/httpserver/auth_flow_api.go:0037 | 	nextSendAt time.Time
backend-go/internal/httpserver/auth_flow_api.go:0038 | 	attempts   int
backend-go/internal/httpserver/auth_flow_api.go:0039 | }
backend-go/internal/httpserver/auth_flow_api.go:0040 | 
backend-go/internal/httpserver/auth_flow_api.go:0041 | type authFlowError struct {
backend-go/internal/httpserver/auth_flow_api.go:0042 | 	status  int
backend-go/internal/httpserver/auth_flow_api.go:0043 | 	code    string
backend-go/internal/httpserver/auth_flow_api.go:0044 | 	message string
backend-go/internal/httpserver/auth_flow_api.go:0045 | 	details map[string]any
backend-go/internal/httpserver/auth_flow_api.go:0046 | }
backend-go/internal/httpserver/auth_flow_api.go:0047 | 
backend-go/internal/httpserver/auth_flow_api.go:0048 | func (e *authFlowError) Error() string { return e.message }
backend-go/internal/httpserver/auth_flow_api.go:0049 | 
backend-go/internal/httpserver/auth_flow_api.go:0050 | type authRegistrationInput struct {
backend-go/internal/httpserver/auth_flow_api.go:0051 | 	InviteCode     string
backend-go/internal/httpserver/auth_flow_api.go:0052 | 	Scene          string
backend-go/internal/httpserver/auth_flow_api.go:0053 | 	PromoterCode   string
backend-go/internal/httpserver/auth_flow_api.go:0054 | 	CampaignCode   string
backend-go/internal/httpserver/auth_flow_api.go:0055 | 	RedirectSource string
backend-go/internal/httpserver/auth_flow_api.go:0056 | 	IdempotencyKey string
backend-go/internal/httpserver/auth_flow_api.go:0057 | }
backend-go/internal/httpserver/auth_flow_api.go:0058 | 
backend-go/internal/httpserver/auth_flow_api.go:0059 | type smsSendRequest struct {
backend-go/internal/httpserver/auth_flow_api.go:0060 | 	Mobile  string `json:"mobile"`
backend-go/internal/httpserver/auth_flow_api.go:0061 | 	Purpose string `json:"purpose"`
backend-go/internal/httpserver/auth_flow_api.go:0062 | }
backend-go/internal/httpserver/auth_flow_api.go:0063 | 
backend-go/internal/httpserver/auth_flow_api.go:0064 | type smsLoginRequest struct {
```

<div style="page-break-after: always;"></div>

### 第 53 页

**文件路径：** backend-go/internal/httpserver/auth_flow_api.go  
**代码说明：** 认证、注册、会话与账号安全流程。

```go
backend-go/internal/httpserver/auth_flow_api.go:0065 | 	Mobile         string `json:"mobile"`
backend-go/internal/httpserver/auth_flow_api.go:0066 | 	SMSCode        string `json:"smsCode"`
backend-go/internal/httpserver/auth_flow_api.go:0067 | 	InviteCode     string `json:"inviteCode"`
backend-go/internal/httpserver/auth_flow_api.go:0068 | 	Scene          string `json:"scene"`
backend-go/internal/httpserver/auth_flow_api.go:0069 | 	PromoterCode   string `json:"promoterCode"`
backend-go/internal/httpserver/auth_flow_api.go:0070 | 	CampaignCode   string `json:"campaignCode"`
backend-go/internal/httpserver/auth_flow_api.go:0071 | 	RedirectSource string `json:"redirectSource"`
backend-go/internal/httpserver/auth_flow_api.go:0072 | 	IdempotencyKey string `json:"idempotencyKey"`
backend-go/internal/httpserver/auth_flow_api.go:0073 | }
backend-go/internal/httpserver/auth_flow_api.go:0074 | 
backend-go/internal/httpserver/auth_flow_api.go:0075 | type mobileBindRequest struct {
backend-go/internal/httpserver/auth_flow_api.go:0076 | 	Mobile  string `json:"mobile"`
backend-go/internal/httpserver/auth_flow_api.go:0077 | 	SMSCode string `json:"smsCode"`
backend-go/internal/httpserver/auth_flow_api.go:0078 | }
backend-go/internal/httpserver/auth_flow_api.go:0079 | 
backend-go/internal/httpserver/auth_flow_api.go:0080 | func newAuthFlowCoordinator() *authFlowCoordinator {
backend-go/internal/httpserver/auth_flow_api.go:0081 | 	return &authFlowCoordinator{smsChallenges: map[string]smsChallenge{}, smsNextSend: map[string]time.Time{}}
backend-go/internal/httpserver/auth_flow_api.go:0082 | }
backend-go/internal/httpserver/auth_flow_api.go:0083 | 
backend-go/internal/httpserver/auth_flow_api.go:0084 | func appendUniqueString(values []string, value string) []string {
backend-go/internal/httpserver/auth_flow_api.go:0085 | 	value = strings.TrimSpace(value)
backend-go/internal/httpserver/auth_flow_api.go:0086 | 	if value == "" {
backend-go/internal/httpserver/auth_flow_api.go:0087 | 		return values
backend-go/internal/httpserver/auth_flow_api.go:0088 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0089 | 	for _, existing := range values {
backend-go/internal/httpserver/auth_flow_api.go:0090 | 		if strings.EqualFold(strings.TrimSpace(existing), value) {
backend-go/internal/httpserver/auth_flow_api.go:0091 | 			return values
backend-go/internal/httpserver/auth_flow_api.go:0092 | 		}
backend-go/internal/httpserver/auth_flow_api.go:0093 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0094 | 	return append(values, value)
backend-go/internal/httpserver/auth_flow_api.go:0095 | }
backend-go/internal/httpserver/auth_flow_api.go:0096 | 
backend-go/internal/httpserver/auth_flow_api.go:0097 | func cloneStringMap(values map[string]string) map[string]string {
backend-go/internal/httpserver/auth_flow_api.go:0098 | 	if len(values) == 0 {
backend-go/internal/httpserver/auth_flow_api.go:0099 | 		return nil
backend-go/internal/httpserver/auth_flow_api.go:0100 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0101 | 	cloned := make(map[string]string, len(values))
backend-go/internal/httpserver/auth_flow_api.go:0102 | 	for key, value := range values {
backend-go/internal/httpserver/auth_flow_api.go:0103 | 		if trimmed := strings.TrimSpace(value); trimmed != "" {
backend-go/internal/httpserver/auth_flow_api.go:0104 | 			cloned[key] = trimmed
backend-go/internal/httpserver/auth_flow_api.go:0105 | 		}
backend-go/internal/httpserver/auth_flow_api.go:0106 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0107 | 	if len(cloned) == 0 {
backend-go/internal/httpserver/auth_flow_api.go:0108 | 		return nil
backend-go/internal/httpserver/auth_flow_api.go:0109 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0110 | 	return cloned
backend-go/internal/httpserver/auth_flow_api.go:0111 | }
backend-go/internal/httpserver/auth_flow_api.go:0112 | 
backend-go/internal/httpserver/auth_flow_api.go:0113 | func writeAuthFlowError(w http.ResponseWriter, status int, code, message string) {
backend-go/internal/httpserver/auth_flow_api.go:0114 | 	writeAuthFlowErrorDetails(w, status, code, message, nil)
```

<div style="page-break-after: always;"></div>

### 第 54 页

**文件路径：** backend-go/internal/httpserver/auth_flow_api.go  
**代码说明：** 认证、注册、会话与账号安全流程。

```go
backend-go/internal/httpserver/auth_flow_api.go:0115 | }
backend-go/internal/httpserver/auth_flow_api.go:0116 | 
backend-go/internal/httpserver/auth_flow_api.go:0117 | func writeAuthFlowErrorDetails(w http.ResponseWriter, status int, code, message string, details map[string]any) {
backend-go/internal/httpserver/auth_flow_api.go:0118 | 	w.Header().Set("Content-Type", "application/json; charset=utf-8")
backend-go/internal/httpserver/auth_flow_api.go:0119 | 	w.WriteHeader(status)
backend-go/internal/httpserver/auth_flow_api.go:0120 | 	payload := map[string]any{"code": code, "errorCode": code, "error": message, "message": message}
backend-go/internal/httpserver/auth_flow_api.go:0121 | 	for key, value := range details {
backend-go/internal/httpserver/auth_flow_api.go:0122 | 		payload[key] = value
backend-go/internal/httpserver/auth_flow_api.go:0123 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0124 | 	_ = json.NewEncoder(w).Encode(payload)
backend-go/internal/httpserver/auth_flow_api.go:0125 | }
backend-go/internal/httpserver/auth_flow_api.go:0126 | 
backend-go/internal/httpserver/auth_flow_api.go:0127 | func writeMappedAuthFlowError(w http.ResponseWriter, err error) {
backend-go/internal/httpserver/auth_flow_api.go:0128 | 	var flowErr *authFlowError
backend-go/internal/httpserver/auth_flow_api.go:0129 | 	if errors.As(err, &flowErr) {
backend-go/internal/httpserver/auth_flow_api.go:0130 | 		writeAuthFlowErrorDetails(w, flowErr.status, flowErr.code, flowErr.message, flowErr.details)
backend-go/internal/httpserver/auth_flow_api.go:0131 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0132 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0133 | 	writeAuthFlowError(w, http.StatusInternalServerError, "AUTH_INTERNAL_ERROR", "登录服务暂时不可用")
backend-go/internal/httpserver/auth_flow_api.go:0134 | }
backend-go/internal/httpserver/auth_flow_api.go:0135 | 
backend-go/internal/httpserver/auth_flow_api.go:0136 | func normalizeMainlandMobile(value string) string {
backend-go/internal/httpserver/auth_flow_api.go:0137 | 	var builder strings.Builder
backend-go/internal/httpserver/auth_flow_api.go:0138 | 	for _, char := range strings.TrimSpace(value) {
backend-go/internal/httpserver/auth_flow_api.go:0139 | 		if char >= '0' && char <= '9' {
backend-go/internal/httpserver/auth_flow_api.go:0140 | 			builder.WriteRune(char)
backend-go/internal/httpserver/auth_flow_api.go:0141 | 		}
backend-go/internal/httpserver/auth_flow_api.go:0142 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0143 | 	mobile := builder.String()
backend-go/internal/httpserver/auth_flow_api.go:0144 | 	if strings.HasPrefix(mobile, "86") && len(mobile) == 13 {
backend-go/internal/httpserver/auth_flow_api.go:0145 | 		mobile = mobile[2:]
backend-go/internal/httpserver/auth_flow_api.go:0146 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0147 | 	return mobile
backend-go/internal/httpserver/auth_flow_api.go:0148 | }
backend-go/internal/httpserver/auth_flow_api.go:0149 | 
backend-go/internal/httpserver/auth_flow_api.go:0150 | func validMainlandMobile(value string) bool {
backend-go/internal/httpserver/auth_flow_api.go:0151 | 	mobile := normalizeMainlandMobile(value)
backend-go/internal/httpserver/auth_flow_api.go:0152 | 	return len(mobile) == 11 && mobile[0] == '1' && mobile[1] >= '3' && mobile[1] <= '9'
backend-go/internal/httpserver/auth_flow_api.go:0153 | }
backend-go/internal/httpserver/auth_flow_api.go:0154 | 
backend-go/internal/httpserver/auth_flow_api.go:0155 | func maskedMobile(value string) string {
backend-go/internal/httpserver/auth_flow_api.go:0156 | 	mobile := normalizeMainlandMobile(value)
backend-go/internal/httpserver/auth_flow_api.go:0157 | 	if len(mobile) != 11 {
backend-go/internal/httpserver/auth_flow_api.go:0158 | 		return ""
backend-go/internal/httpserver/auth_flow_api.go:0159 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0160 | 	return mobile[:3] + "****" + mobile[7:]
backend-go/internal/httpserver/auth_flow_api.go:0161 | }
backend-go/internal/httpserver/auth_flow_api.go:0162 | 
backend-go/internal/httpserver/auth_flow_api.go:0163 | func authCodeHash(mobile, code string) [32]byte {
backend-go/internal/httpserver/auth_flow_api.go:0164 | 	return sha256.Sum256([]byte(normalizeMainlandMobile(mobile) + ":" + strings.TrimSpace(code)))
```

<div style="page-break-after: always;"></div>

### 第 55 页

**文件路径：** backend-go/internal/httpserver/auth_flow_api.go  
**代码说明：** 认证、注册、会话与账号安全流程。

```go
backend-go/internal/httpserver/auth_flow_api.go:0165 | }
backend-go/internal/httpserver/auth_flow_api.go:0166 | 
backend-go/internal/httpserver/auth_flow_api.go:0167 | func randomSMSCode() (string, error) {
backend-go/internal/httpserver/auth_flow_api.go:0168 | 	value, err := rand.Int(rand.Reader, big.NewInt(1000000))
backend-go/internal/httpserver/auth_flow_api.go:0169 | 	if err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0170 | 		return "", err
backend-go/internal/httpserver/auth_flow_api.go:0171 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0172 | 	return fmt.Sprintf("%06d", value.Int64()), nil
backend-go/internal/httpserver/auth_flow_api.go:0173 | }
backend-go/internal/httpserver/auth_flow_api.go:0174 | 
backend-go/internal/httpserver/auth_flow_api.go:0175 | func developmentSMSCode() string {
backend-go/internal/httpserver/auth_flow_api.go:0176 | 	env := strings.ToLower(strings.TrimSpace(os.Getenv("XIANZHI_ENV")))
backend-go/internal/httpserver/auth_flow_api.go:0177 | 	if env == "production" || env == "prod" {
backend-go/internal/httpserver/auth_flow_api.go:0178 | 		return ""
backend-go/internal/httpserver/auth_flow_api.go:0179 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0180 | 	return strings.TrimSpace(firstNonEmptyString(os.Getenv("XIANZHI_SMS_DEV_CODE"), os.Getenv("SMS_DEV_CODE")))
backend-go/internal/httpserver/auth_flow_api.go:0181 | }
backend-go/internal/httpserver/auth_flow_api.go:0182 | 
backend-go/internal/httpserver/auth_flow_api.go:0183 | func sendSMSProvider(ctx context.Context, mobile, code string) error {
backend-go/internal/httpserver/auth_flow_api.go:0184 | 	if devCode := developmentSMSCode(); devCode != "" {
backend-go/internal/httpserver/auth_flow_api.go:0185 | 		return nil
backend-go/internal/httpserver/auth_flow_api.go:0186 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0187 | 	providerURL := strings.TrimSpace(os.Getenv("SMS_PROVIDER_URL"))
backend-go/internal/httpserver/auth_flow_api.go:0188 | 	if providerURL == "" {
backend-go/internal/httpserver/auth_flow_api.go:0189 | 		return &authFlowError{status: http.StatusServiceUnavailable, code: "SMS_NOT_CONFIGURED", message: "短信服务尚未配置"}
backend-go/internal/httpserver/auth_flow_api.go:0190 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0191 | 	payload, _ := json.Marshal(map[string]string{
backend-go/internal/httpserver/auth_flow_api.go:0192 | 		"mobile": mobile, "code": code, "templateId": strings.TrimSpace(os.Getenv("SMS_TEMPLATE_ID")),
backend-go/internal/httpserver/auth_flow_api.go:0193 | 	})
backend-go/internal/httpserver/auth_flow_api.go:0194 | 	req, err := http.NewRequestWithContext(ctx, http.MethodPost, providerURL, bytes.NewReader(payload))
backend-go/internal/httpserver/auth_flow_api.go:0195 | 	if err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0196 | 		return err
backend-go/internal/httpserver/auth_flow_api.go:0197 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0198 | 	req.Header.Set("Content-Type", "application/json")
backend-go/internal/httpserver/auth_flow_api.go:0199 | 	if token := strings.TrimSpace(os.Getenv("SMS_PROVIDER_API_KEY")); token != "" {
backend-go/internal/httpserver/auth_flow_api.go:0200 | 		req.Header.Set("Authorization", "Bearer "+token)
backend-go/internal/httpserver/auth_flow_api.go:0201 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0202 | 	response, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
backend-go/internal/httpserver/auth_flow_api.go:0203 | 	if err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0204 | 		return &authFlowError{status: http.StatusBadGateway, code: "SMS_SEND_FAILED", message: "验证码发送失败"}
backend-go/internal/httpserver/auth_flow_api.go:0205 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0206 | 	defer response.Body.Close()
backend-go/internal/httpserver/auth_flow_api.go:0207 | 	if response.StatusCode < 200 || response.StatusCode >= 300 {
backend-go/internal/httpserver/auth_flow_api.go:0208 | 		return &authFlowError{status: http.StatusBadGateway, code: "SMS_SEND_FAILED", message: "验证码发送失败"}
backend-go/internal/httpserver/auth_flow_api.go:0209 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0210 | 	return nil
backend-go/internal/httpserver/auth_flow_api.go:0211 | }
backend-go/internal/httpserver/auth_flow_api.go:0212 | 
backend-go/internal/httpserver/auth_flow_api.go:0213 | func (a authAPI) smsStore() (smsChallengeStore, bool) {
backend-go/internal/httpserver/auth_flow_api.go:0214 | 	if a.sessions == nil {
```

<div style="page-break-after: always;"></div>

### 第 56 页

**文件路径：** backend-go/internal/httpserver/auth_flow_api.go  
**代码说明：** 认证、注册、会话与账号安全流程。

```go
backend-go/internal/httpserver/auth_flow_api.go:0215 | 		return nil, false
backend-go/internal/httpserver/auth_flow_api.go:0216 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0217 | 	store, ok := a.sessions.(smsChallengeStore)
backend-go/internal/httpserver/auth_flow_api.go:0218 | 	return store, ok
backend-go/internal/httpserver/auth_flow_api.go:0219 | }
backend-go/internal/httpserver/auth_flow_api.go:0220 | 
backend-go/internal/httpserver/auth_flow_api.go:0221 | func (a authAPI) smsNextSendAt(ctx context.Context, mobile string) (time.Time, bool, error) {
backend-go/internal/httpserver/auth_flow_api.go:0222 | 	if store, ok := a.smsStore(); ok {
backend-go/internal/httpserver/auth_flow_api.go:0223 | 		return store.SMSNextSend(ctx, mobile)
backend-go/internal/httpserver/auth_flow_api.go:0224 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0225 | 	flow := a.flow
backend-go/internal/httpserver/auth_flow_api.go:0226 | 	if flow == nil {
backend-go/internal/httpserver/auth_flow_api.go:0227 | 		return time.Time{}, false, nil
backend-go/internal/httpserver/auth_flow_api.go:0228 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0229 | 	flow.smsMu.Lock()
backend-go/internal/httpserver/auth_flow_api.go:0230 | 	defer flow.smsMu.Unlock()
backend-go/internal/httpserver/auth_flow_api.go:0231 | 	nextSendAt, ok := flow.smsNextSend[mobile]
backend-go/internal/httpserver/auth_flow_api.go:0232 | 	if ok && time.Now().After(nextSendAt) {
backend-go/internal/httpserver/auth_flow_api.go:0233 | 		delete(flow.smsNextSend, mobile)
backend-go/internal/httpserver/auth_flow_api.go:0234 | 		return time.Time{}, false, nil
backend-go/internal/httpserver/auth_flow_api.go:0235 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0236 | 	return nextSendAt, ok, nil
backend-go/internal/httpserver/auth_flow_api.go:0237 | }
backend-go/internal/httpserver/auth_flow_api.go:0238 | 
backend-go/internal/httpserver/auth_flow_api.go:0239 | func (a authAPI) putSMSChallenge(ctx context.Context, mobile string, challenge smsChallenge) error {
backend-go/internal/httpserver/auth_flow_api.go:0240 | 	if store, ok := a.smsStore(); ok {
backend-go/internal/httpserver/auth_flow_api.go:0241 | 		ttl := time.Until(challenge.expiresAt)
backend-go/internal/httpserver/auth_flow_api.go:0242 | 		if ttl <= 0 {
backend-go/internal/httpserver/auth_flow_api.go:0243 | 			ttl = smsCodeTTL
backend-go/internal/httpserver/auth_flow_api.go:0244 | 		}
backend-go/internal/httpserver/auth_flow_api.go:0245 | 		return store.PutSMSChallenge(ctx, mobile, challenge, ttl)
backend-go/internal/httpserver/auth_flow_api.go:0246 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0247 | 	flow := a.flow
backend-go/internal/httpserver/auth_flow_api.go:0248 | 	if flow == nil {
backend-go/internal/httpserver/auth_flow_api.go:0249 | 		return errAuthSessionUnavailable
backend-go/internal/httpserver/auth_flow_api.go:0250 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0251 | 	flow.smsMu.Lock()
backend-go/internal/httpserver/auth_flow_api.go:0252 | 	defer flow.smsMu.Unlock()
backend-go/internal/httpserver/auth_flow_api.go:0253 | 	flow.smsChallenges[mobile] = challenge
backend-go/internal/httpserver/auth_flow_api.go:0254 | 	return nil
backend-go/internal/httpserver/auth_flow_api.go:0255 | }
backend-go/internal/httpserver/auth_flow_api.go:0256 | 
backend-go/internal/httpserver/auth_flow_api.go:0257 | func (a authAPI) putSMSNextSend(ctx context.Context, mobile string, nextSendAt time.Time) error {
backend-go/internal/httpserver/auth_flow_api.go:0258 | 	if store, ok := a.smsStore(); ok {
backend-go/internal/httpserver/auth_flow_api.go:0259 | 		ttl := time.Until(nextSendAt)
backend-go/internal/httpserver/auth_flow_api.go:0260 | 		if ttl <= 0 {
backend-go/internal/httpserver/auth_flow_api.go:0261 | 			ttl = smsSendInterval
backend-go/internal/httpserver/auth_flow_api.go:0262 | 		}
backend-go/internal/httpserver/auth_flow_api.go:0263 | 		return store.PutSMSNextSend(ctx, mobile, nextSendAt, ttl)
backend-go/internal/httpserver/auth_flow_api.go:0264 | 	}
```

<div style="page-break-after: always;"></div>

### 第 57 页

**文件路径：** backend-go/internal/httpserver/auth_flow_api.go  
**代码说明：** 认证、注册、会话与账号安全流程。

```go
backend-go/internal/httpserver/auth_flow_api.go:0265 | 	flow := a.flow
backend-go/internal/httpserver/auth_flow_api.go:0266 | 	if flow == nil {
backend-go/internal/httpserver/auth_flow_api.go:0267 | 		return errAuthSessionUnavailable
backend-go/internal/httpserver/auth_flow_api.go:0268 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0269 | 	flow.smsMu.Lock()
backend-go/internal/httpserver/auth_flow_api.go:0270 | 	defer flow.smsMu.Unlock()
backend-go/internal/httpserver/auth_flow_api.go:0271 | 	flow.smsNextSend[mobile] = nextSendAt
backend-go/internal/httpserver/auth_flow_api.go:0272 | 	return nil
backend-go/internal/httpserver/auth_flow_api.go:0273 | }
backend-go/internal/httpserver/auth_flow_api.go:0274 | 
backend-go/internal/httpserver/auth_flow_api.go:0275 | func (a authAPI) getSMSChallenge(ctx context.Context, mobile string) (smsChallenge, bool, error) {
backend-go/internal/httpserver/auth_flow_api.go:0276 | 	if store, ok := a.smsStore(); ok {
backend-go/internal/httpserver/auth_flow_api.go:0277 | 		return store.SMSChallenge(ctx, mobile)
backend-go/internal/httpserver/auth_flow_api.go:0278 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0279 | 	flow := a.flow
backend-go/internal/httpserver/auth_flow_api.go:0280 | 	if flow == nil {
backend-go/internal/httpserver/auth_flow_api.go:0281 | 		return smsChallenge{}, false, nil
backend-go/internal/httpserver/auth_flow_api.go:0282 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0283 | 	flow.smsMu.Lock()
backend-go/internal/httpserver/auth_flow_api.go:0284 | 	defer flow.smsMu.Unlock()
backend-go/internal/httpserver/auth_flow_api.go:0285 | 	challenge, ok := flow.smsChallenges[mobile]
backend-go/internal/httpserver/auth_flow_api.go:0286 | 	return challenge, ok, nil
backend-go/internal/httpserver/auth_flow_api.go:0287 | }
backend-go/internal/httpserver/auth_flow_api.go:0288 | 
backend-go/internal/httpserver/auth_flow_api.go:0289 | func (a authAPI) deleteSMSChallenge(ctx context.Context, mobile string) error {
backend-go/internal/httpserver/auth_flow_api.go:0290 | 	if store, ok := a.smsStore(); ok {
backend-go/internal/httpserver/auth_flow_api.go:0291 | 		return store.DeleteSMSChallenge(ctx, mobile)
backend-go/internal/httpserver/auth_flow_api.go:0292 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0293 | 	flow := a.flow
backend-go/internal/httpserver/auth_flow_api.go:0294 | 	if flow == nil {
backend-go/internal/httpserver/auth_flow_api.go:0295 | 		return nil
backend-go/internal/httpserver/auth_flow_api.go:0296 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0297 | 	flow.smsMu.Lock()
backend-go/internal/httpserver/auth_flow_api.go:0298 | 	defer flow.smsMu.Unlock()
backend-go/internal/httpserver/auth_flow_api.go:0299 | 	delete(flow.smsChallenges, mobile)
backend-go/internal/httpserver/auth_flow_api.go:0300 | 	return nil
backend-go/internal/httpserver/auth_flow_api.go:0301 | }
backend-go/internal/httpserver/auth_flow_api.go:0302 | 
backend-go/internal/httpserver/auth_flow_api.go:0303 | func (a authAPI) smsSend(w http.ResponseWriter, r *http.Request) {
backend-go/internal/httpserver/auth_flow_api.go:0304 | 	var req smsSendRequest
backend-go/internal/httpserver/auth_flow_api.go:0305 | 	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0306 | 		writeAuthFlowError(w, http.StatusBadRequest, "INVALID_REQUEST", "请求参数不正确")
backend-go/internal/httpserver/auth_flow_api.go:0307 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0308 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0309 | 	mobile := normalizeMainlandMobile(req.Mobile)
backend-go/internal/httpserver/auth_flow_api.go:0310 | 	if !validMainlandMobile(mobile) {
backend-go/internal/httpserver/auth_flow_api.go:0311 | 		writeAuthFlowError(w, http.StatusBadRequest, "MOBILE_INVALID", "请输入正确的11位手机号")
backend-go/internal/httpserver/auth_flow_api.go:0312 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0313 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0314 | 	now := time.Now()
```

<div style="page-break-after: always;"></div>

### 第 58 页

**文件路径：** backend-go/internal/httpserver/auth_flow_api.go  
**代码说明：** 认证、注册、会话与账号安全流程。

```go
backend-go/internal/httpserver/auth_flow_api.go:0315 | 	nextSendAt, hasNextSend, err := a.smsNextSendAt(r.Context(), mobile)
backend-go/internal/httpserver/auth_flow_api.go:0316 | 	if err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0317 | 		writeAuthFlowError(w, http.StatusServiceUnavailable, "SMS_STATE_UNAVAILABLE", "验证码服务暂时不可用")
backend-go/internal/httpserver/auth_flow_api.go:0318 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0319 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0320 | 	if hasNextSend && nextSendAt.After(now) {
backend-go/internal/httpserver/auth_flow_api.go:0321 | 		retry := int(time.Until(nextSendAt).Seconds()) + 1
backend-go/internal/httpserver/auth_flow_api.go:0322 | 		w.Header().Set("Retry-After", fmt.Sprintf("%d", retry))
backend-go/internal/httpserver/auth_flow_api.go:0323 | 		writeAuthFlowError(w, http.StatusTooManyRequests, "SMS_TOO_FREQUENT", "发送过于频繁，请稍后再试")
backend-go/internal/httpserver/auth_flow_api.go:0324 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0325 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0326 | 	code := developmentSMSCode()
backend-go/internal/httpserver/auth_flow_api.go:0327 | 	if code == "" {
backend-go/internal/httpserver/auth_flow_api.go:0328 | 		var err error
backend-go/internal/httpserver/auth_flow_api.go:0329 | 		code, err = randomSMSCode()
backend-go/internal/httpserver/auth_flow_api.go:0330 | 		if err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0331 | 			writeAuthFlowError(w, http.StatusInternalServerError, "SMS_CODE_GENERATE_FAILED", "验证码发送失败")
backend-go/internal/httpserver/auth_flow_api.go:0332 | 			return
backend-go/internal/httpserver/auth_flow_api.go:0333 | 		}
backend-go/internal/httpserver/auth_flow_api.go:0334 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0335 | 	if err := sendSMSProvider(r.Context(), mobile, code); err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0336 | 		writeMappedAuthFlowError(w, err)
backend-go/internal/httpserver/auth_flow_api.go:0337 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0338 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0339 | 	challenge := smsChallenge{
backend-go/internal/httpserver/auth_flow_api.go:0340 | 		codeHash: authCodeHash(mobile, code), expiresAt: now.Add(smsCodeTTL), nextSendAt: now.Add(smsSendInterval),
backend-go/internal/httpserver/auth_flow_api.go:0341 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0342 | 	if err := a.putSMSChallenge(r.Context(), mobile, challenge); err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0343 | 		writeAuthFlowError(w, http.StatusServiceUnavailable, "SMS_STATE_UNAVAILABLE", "验证码服务暂时不可用")
backend-go/internal/httpserver/auth_flow_api.go:0344 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0345 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0346 | 	if err := a.putSMSNextSend(r.Context(), mobile, challenge.nextSendAt); err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0347 | 		writeAuthFlowError(w, http.StatusServiceUnavailable, "SMS_STATE_UNAVAILABLE", "验证码服务暂时不可用")
backend-go/internal/httpserver/auth_flow_api.go:0348 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0349 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0350 | 	writeJSON(w, map[string]any{"sent": true, "retryAfterSeconds": int(smsSendInterval.Seconds()), "expiresInSeconds": int(smsCodeTTL.Seconds())})
backend-go/internal/httpserver/auth_flow_api.go:0351 | }
backend-go/internal/httpserver/auth_flow_api.go:0352 | 
backend-go/internal/httpserver/auth_flow_api.go:0353 | func (a authAPI) verifySMSCode(ctx context.Context, mobile, code string) error {
backend-go/internal/httpserver/auth_flow_api.go:0354 | 	challenge, ok, err := a.getSMSChallenge(ctx, mobile)
backend-go/internal/httpserver/auth_flow_api.go:0355 | 	if err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0356 | 		return &authFlowError{status: http.StatusServiceUnavailable, code: "SMS_STATE_UNAVAILABLE", message: "验证码服务暂时不可用"}
backend-go/internal/httpserver/auth_flow_api.go:0357 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0358 | 	if !ok || time.Now().After(challenge.expiresAt) {
backend-go/internal/httpserver/auth_flow_api.go:0359 | 		_ = a.deleteSMSChallenge(ctx, mobile)
backend-go/internal/httpserver/auth_flow_api.go:0360 | 		return &authFlowError{status: http.StatusUnauthorized, code: "SMS_CODE_EXPIRED", message: "验证码已过期，请重新获取"}
backend-go/internal/httpserver/auth_flow_api.go:0361 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0362 | 	if challenge.attempts >= smsMaxAttempts {
backend-go/internal/httpserver/auth_flow_api.go:0363 | 		_ = a.deleteSMSChallenge(ctx, mobile)
backend-go/internal/httpserver/auth_flow_api.go:0364 | 		return &authFlowError{status: http.StatusTooManyRequests, code: "SMS_CODE_LOCKED", message: "验证码错误次数过多，请重新获取"}
```

<div style="page-break-after: always;"></div>

### 第 59 页

**文件路径：** backend-go/internal/httpserver/auth_flow_api.go  
**代码说明：** 认证、注册、会话与账号安全流程。

```go
backend-go/internal/httpserver/auth_flow_api.go:0365 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0366 | 	wanted := challenge.codeHash
backend-go/internal/httpserver/auth_flow_api.go:0367 | 	got := authCodeHash(mobile, code)
backend-go/internal/httpserver/auth_flow_api.go:0368 | 	if subtle.ConstantTimeCompare(wanted[:], got[:]) != 1 {
backend-go/internal/httpserver/auth_flow_api.go:0369 | 		challenge.attempts++
backend-go/internal/httpserver/auth_flow_api.go:0370 | 		if err := a.putSMSChallenge(ctx, mobile, challenge); err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0371 | 			return &authFlowError{status: http.StatusServiceUnavailable, code: "SMS_STATE_UNAVAILABLE", message: "验证码服务暂时不可用"}
backend-go/internal/httpserver/auth_flow_api.go:0372 | 		}
backend-go/internal/httpserver/auth_flow_api.go:0373 | 		return &authFlowError{status: http.StatusUnauthorized, code: "SMS_CODE_INVALID", message: "验证码错误，请重新输入"}
backend-go/internal/httpserver/auth_flow_api.go:0374 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0375 | 	_ = a.deleteSMSChallenge(ctx, mobile)
backend-go/internal/httpserver/auth_flow_api.go:0376 | 	return nil
backend-go/internal/httpserver/auth_flow_api.go:0377 | }
backend-go/internal/httpserver/auth_flow_api.go:0378 | 
backend-go/internal/httpserver/auth_flow_api.go:0379 | func (a authAPI) smsLogin(w http.ResponseWriter, r *http.Request) {
backend-go/internal/httpserver/auth_flow_api.go:0380 | 	var req smsLoginRequest
backend-go/internal/httpserver/auth_flow_api.go:0381 | 	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0382 | 		writeAuthFlowError(w, http.StatusBadRequest, "INVALID_REQUEST", "请求参数不正确")
backend-go/internal/httpserver/auth_flow_api.go:0383 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0384 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0385 | 	mobile := normalizeMainlandMobile(req.Mobile)
backend-go/internal/httpserver/auth_flow_api.go:0386 | 	if !validMainlandMobile(mobile) {
backend-go/internal/httpserver/auth_flow_api.go:0387 | 		writeAuthFlowError(w, http.StatusBadRequest, "MOBILE_INVALID", "请输入正确的11位手机号")
backend-go/internal/httpserver/auth_flow_api.go:0388 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0389 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0390 | 	if err := a.verifySMSCode(r.Context(), mobile, req.SMSCode); err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0391 | 		writeMappedAuthFlowError(w, err)
backend-go/internal/httpserver/auth_flow_api.go:0392 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0393 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0394 | 	input := authRegistrationInput{
backend-go/internal/httpserver/auth_flow_api.go:0395 | 		InviteCode: req.InviteCode, Scene: req.Scene, PromoterCode: req.PromoterCode,
backend-go/internal/httpserver/auth_flow_api.go:0396 | 		CampaignCode: req.CampaignCode, RedirectSource: req.RedirectSource, IdempotencyKey: req.IdempotencyKey,
backend-go/internal/httpserver/auth_flow_api.go:0397 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0398 | 	data, user, isNewUser, inviteStatus, err := a.userForPhoneIdentity(mobile, wechatMiniProgramSession{}, input)
backend-go/internal/httpserver/auth_flow_api.go:0399 | 	if err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0400 | 		writeMappedAuthFlowError(w, err)
backend-go/internal/httpserver/auth_flow_api.go:0401 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0402 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0403 | 	response, err := a.authResponseWithToken(r.Context(), data, user)
backend-go/internal/httpserver/auth_flow_api.go:0404 | 	if err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0405 | 		writeAuthTokenError(w, err)
backend-go/internal/httpserver/auth_flow_api.go:0406 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0407 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0408 | 	response["isNewUser"] = isNewUser
backend-go/internal/httpserver/auth_flow_api.go:0409 | 	response["registrationStatus"] = map[bool]string{true: "created", false: "existing"}[isNewUser]
backend-go/internal/httpserver/auth_flow_api.go:0410 | 	response["inviteBindStatus"] = inviteStatus
backend-go/internal/httpserver/auth_flow_api.go:0411 | 	response["expiresIn"] = int(authSessionTTL.Seconds())
backend-go/internal/httpserver/auth_flow_api.go:0412 | 	if isNewUser {
backend-go/internal/httpserver/auth_flow_api.go:0413 | 		response["newcomerBenefits"] = newcomerBenefitsForPlan(configuredNewcomerPlan(data.Plans))
backend-go/internal/httpserver/auth_flow_api.go:0414 | 	}
```

<div style="page-break-after: always;"></div>

### 第 60 页

**文件路径：** backend-go/internal/httpserver/auth_flow_api.go  
**代码说明：** 认证、注册、会话与账号安全流程。

```go
backend-go/internal/httpserver/auth_flow_api.go:0415 | 	writeAuthTokenResponse(w, r, response)
backend-go/internal/httpserver/auth_flow_api.go:0416 | }
backend-go/internal/httpserver/auth_flow_api.go:0417 | 
backend-go/internal/httpserver/auth_flow_api.go:0418 | func (a authAPI) bindMobile(w http.ResponseWriter, r *http.Request) {
backend-go/internal/httpserver/auth_flow_api.go:0419 | 	var req mobileBindRequest
backend-go/internal/httpserver/auth_flow_api.go:0420 | 	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0421 | 		writeAuthFlowError(w, http.StatusBadRequest, "INVALID_REQUEST", "请求参数不正确")
backend-go/internal/httpserver/auth_flow_api.go:0422 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0423 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0424 | 	mobile := normalizeMainlandMobile(req.Mobile)
backend-go/internal/httpserver/auth_flow_api.go:0425 | 	if !validMainlandMobile(mobile) {
backend-go/internal/httpserver/auth_flow_api.go:0426 | 		writeAuthFlowError(w, http.StatusBadRequest, "MOBILE_INVALID", "请输入正确的11位手机号")
backend-go/internal/httpserver/auth_flow_api.go:0427 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0428 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0429 | 	if err := a.verifySMSCode(r.Context(), mobile, req.SMSCode); err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0430 | 		writeMappedAuthFlowError(w, err)
backend-go/internal/httpserver/auth_flow_api.go:0431 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0432 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0433 | 	data, err := a.store.AdminData()
backend-go/internal/httpserver/auth_flow_api.go:0434 | 	if err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0435 | 		writeMappedAuthFlowError(w, err)
backend-go/internal/httpserver/auth_flow_api.go:0436 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0437 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0438 | 	current, err := a.authenticatedUser(r, data)
backend-go/internal/httpserver/auth_flow_api.go:0439 | 	if err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0440 | 		writeAuthFlowError(w, http.StatusUnauthorized, "UNAUTHORIZED", "登录状态已失效")
backend-go/internal/httpserver/auth_flow_api.go:0441 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0442 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0443 | 	if existing, ok := findUserByMobile(data.Users, mobile); ok && existing.ID != current.ID {
backend-go/internal/httpserver/auth_flow_api.go:0444 | 		writeAuthFlowError(w, http.StatusConflict, "AUTH_MOBILE_ALREADY_BOUND", "该手机号已绑定其他账号")
backend-go/internal/httpserver/auth_flow_api.go:0445 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0446 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0447 | 	updated, err := a.store.UpdateAdminCustomer(current.ID, adminCustomerMutation{Mobile: mobile})
backend-go/internal/httpserver/auth_flow_api.go:0448 | 	if err != nil {
backend-go/internal/httpserver/auth_flow_api.go:0449 | 		writeMappedAuthFlowError(w, err)
backend-go/internal/httpserver/auth_flow_api.go:0450 | 		return
backend-go/internal/httpserver/auth_flow_api.go:0451 | 	}
backend-go/internal/httpserver/auth_flow_api.go:0452 | 	updatedData := dataWithUpdatedUser(data, updated)
backend-go/internal/httpserver/auth_flow_api.go:0453 | 	writeJSON(w, map[string]any{
backend-go/internal/httpserver/auth_flow_api.go:0454 | 		"bound":    true,
backend-go/internal/httpserver/auth_flow_api.go:0455 | 		"user":     userView(updated),
backend-go/internal/httpserver/auth_flow_api.go:0456 | 		"auth":     authResponse(updatedData, updated, false),
backend-go/internal/httpserver/auth_flow_api.go:0457 | 		"security": securityPayload(updated),
backend-go/internal/httpserver/auth_flow_api.go:0458 | 	})
backend-go/internal/httpserver/auth_flow_api.go:0459 | }
backend-go/internal/httpserver/auth_flow_api.go:0460 | 
backend-go/internal/httpserver/auth_flow_api.go:0461 | func registrationSource(input authRegistrationInput) map[string]string {
backend-go/internal/httpserver/auth_flow_api.go:0462 | 	return cloneStringMap(map[string]string{
backend-go/internal/httpserver/auth_flow_api.go:0463 | 		"scene": input.Scene, "promoterCode": input.PromoterCode, "campaignCode": input.CampaignCode,
backend-go/internal/httpserver/auth_flow_api.go:0464 | 		"redirectSource": input.RedirectSource,
```

<div style="page-break-after: always;"></div>
