import { configureApiClient, toChineseApiErrorMessage } from '@xianzhi/api-client'
import { createBusinessSdk } from '@xianzhi/business-sdk'
import { createUniPlatformAdapter, type AdapterDownloadFileResponse } from '@xianzhi/platform-adapter'
import { AuthAccountMismatchError, createAuthService, createAuthStorage } from '@xianzhi/shared-auth'
import { hasAcceptedGuestBrowse } from '../features/auth/guestBrowse'

const tokenKey = 'token'
const refreshTokenKey = 'refreshToken'
const authKey = 'auth'
const requestTimeout = 600000
const rawEnv = (import.meta as unknown as { env?: Record<string, string | boolean | undefined> }).env || {}
const env = rawEnv as Record<string, string | undefined>
const adapter = createUniPlatformAdapter()
let defaultApiBaseURL = ''
// #ifdef MP-WEIXIN
defaultApiBaseURL = 'https://ai.zs-kjhn.cn'
// #endif
const apiBaseURL = String(env.VITE_API_BASE_URL || defaultApiBaseURL).replace(/\/+$/, '')
let unauthorizedRedirecting = false
let unauthorizedRefreshPromise: Promise<boolean> | null = null

export const authStorage = createAuthStorage({
  adapter,
  tokenKey,
  refreshTokenKey,
  authKey,
})

export function getApiBaseURL(): string {
  return sharedApiClient.getBaseURL()
}

export function getClientPlatform(): string {
  return adapter.getClientInfo().platform
}

export function getAuthToken(): string {
  return authStorage.getToken()
}

export function setAuthToken(token: string) {
  authStorage.setToken(token)
}

function createRequestId() {
  const cryptoRuntime = globalThis.crypto
  if (cryptoRuntime?.randomUUID) return cryptoRuntime.randomUUID()
  return `req_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 10)}`
}

function getDeviceRequestId() {
  const key = "zhiqiyunDeviceId"
  const existing = String(uni.getStorageSync(key) || "")
  if (existing) return existing
  const cryptoRuntime = globalThis.crypto
  const created = cryptoRuntime?.randomUUID
    ? cryptoRuntime.randomUUID()
    : `device_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 14)}`
  uni.setStorageSync(key, created)
  return created
}

function currentContextHeaders() {
  const auth = authStorage.getAuth()
  const headers: Record<string, string> = {}
  if (auth?.tenantId) headers['X-Tenant-Id'] = auth.tenantId
  if (auth?.organizationId) headers['X-Organization-Id'] = auth.organizationId
  return headers
}

function clientTransportHeaders(headers: Record<string, string> = {}, auth = true) {
  const clientInfo = adapter.getClientInfo()
  const result: Record<string, string> = {
    Accept: 'application/json',
    'X-Request-Id': createRequestId(),
    'X-Client-Platform': clientInfo.platform,
    'X-Device-Id': getDeviceRequestId(),
    ...currentContextHeaders(),
    ...headers,
  }
  if (clientInfo.appName) result['X-Client-Name'] = clientInfo.appName
  if (env.VITE_APP_VERSION || env.VITE_APP_BUILD_VERSION) result['X-Client-Version'] = env.VITE_APP_VERSION || env.VITE_APP_BUILD_VERSION || ''
  if (clientInfo.language) result['X-Client-Language'] = clientInfo.language
  const token = auth ? getAuthToken() : ''
  if (token && !result.Authorization) result.Authorization = `Bearer ${token}`
  return result
}

function resolveApiURL(path: string) {
  if (/^https?:\/\//i.test(path)) return path
  const baseURL = getApiBaseURL().replace(/\/+$/, '')
  return `${baseURL}${path.startsWith('/') ? path : `/${path}`}`
}

function trustedApiURL(path: string) {
  if (!/^https?:\/\//i.test(path)) return true
  const targetOrigin = path.match(/^https?:\/\/[^/]+/i)?.[0].toLowerCase() || ''
  const baseOrigin = getApiBaseURL().match(/^https?:\/\/[^/]+/i)?.[0].toLowerCase() || ''
  if (baseOrigin) return targetOrigin === baseOrigin
  if (typeof window !== 'undefined') return targetOrigin === window.location.origin.toLowerCase()
  return false
}

function responsePayloadMessage(payload: unknown, fallback: string, statusCode = 0) {
  if (payload && typeof payload === 'object') {
    const record = payload as { code?: unknown; error?: unknown; message?: unknown }
    return toChineseApiErrorMessage(record.error || record.message, { statusCode, apiCode: record.code, fallback })
  }
  return toChineseApiErrorMessage(payload, { statusCode, fallback })
}

function unwrapTransportPayload<T>(payload: unknown) {
  if (!payload || typeof payload !== 'object') return payload as T
  const record = payload as { code?: unknown; data?: T; error?: unknown; message?: unknown }
  const hasEnvelopeShape = 'code' in record || 'message' in record || 'error' in record
  if (hasEnvelopeShape && record.code !== undefined && record.code !== 0 && record.code !== '0') {
    throw new Error(responsePayloadMessage(record, "请求失败，请稍后重试"))
  }
  if (hasEnvelopeShape && Object.prototype.hasOwnProperty.call(record, 'data')) return record.data as T
  return payload as T
}

async function fetchErrorMessage(response: Response) {
  const fallback = `请求失败 (${response.status})`
  const raw = await response.text().catch(() => '')
  if (!raw) return fallback
  try {
    return responsePayloadMessage(JSON.parse(raw), fallback, response.status)
  }
  catch {
    return toChineseApiErrorMessage(raw, { statusCode: response.status, fallback })
  }
}

function normalizeHeaders(headers: RequestInit['headers']): Record<string, string> {
  if (!headers) return {}
  if (typeof Headers !== "undefined" && headers instanceof Headers) return Object.fromEntries(headers.entries())
  if (Array.isArray(headers)) return Object.fromEntries(headers)
  return headers as Record<string, string>
}

function normalizeBody(body: RequestInit['body']) {
  if (body === undefined || body === null) return undefined
  if (typeof body !== 'string') return body
  if (!body.trim()) return undefined
  try {
    return JSON.parse(body)
  }
  catch {
    return body
  }
}

function redirectToLogin() {
  if (unauthorizedRedirecting) return
  if (!authStorage.getToken() && hasAcceptedGuestBrowse()) return
  unauthorizedRedirecting = true
  // #ifdef MP-WEIXIN || APP-PLUS
  const pages = typeof getCurrentPages === 'function' ? getCurrentPages() : []
  const current = pages.length ? pages[pages.length - 1] as unknown as { route?: string; options?: Record<string, unknown> } : null
  const currentPath = current?.route ? `/${String(current.route).replace(/^\/+/, '')}` : ''
  const query = current?.options
    ? Object.entries(current.options).map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(String(value ?? ''))}`).join('&')
    : ''
  const loginQuery = currentPath && currentPath !== '/pages/WechatLoginPage'
    ? `?redirectPath=${encodeURIComponent(currentPath)}&redirectQuery=${encodeURIComponent(query)}&sourcePage=${encodeURIComponent(currentPath)}`
    : ''
  uni.navigateTo({
    url: `/pages/WechatLoginPage${loginQuery}`,
    complete: () => { unauthorizedRedirecting = false },
    fail: () => uni.redirectTo({ url: `/pages/WechatLoginPage${loginQuery}` }),
  })
  // #endif
  // #ifdef H5
  if (typeof window !== 'undefined' && !['/login', '/register'].includes(window.location.pathname)) {
    window.history.pushState({ authRequired: true }, '', `/login?redirect=${encodeURIComponent(window.location.pathname + window.location.search + window.location.hash)}`)
    window.dispatchEvent(new PopStateEvent('popstate'))
  }
  unauthorizedRedirecting = false
  // #endif
}

async function handleUnauthorized(context: { path?: string; statusCode?: number; requestId?: string; payload?: unknown; retryAttempt?: number; authMode?: "none" | "optional" | "required" } = {}) {
  const guestBrowse = !authStorage.getToken() && hasAcceptedGuestBrowse()
  const shouldOpenLogin = context.authMode === "required" && !guestBrowse
  if ((context.retryAttempt || 0) > 0) {
    authStorage.clear()
    if (shouldOpenLogin) redirectToLogin()
    return false
  }
  if (!authStorage.getRefreshToken()) {
    authStorage.clear()
    if (shouldOpenLogin) redirectToLogin()
    return false
  }
  if (!unauthorizedRefreshPromise) {
    unauthorizedRefreshPromise = authService.refresh()
      .then(() => true)
      .catch(error => {
        authStorage.clear()
        if (error instanceof AuthAccountMismatchError || (error as { code?: unknown })?.code === 'AUTH_ACCOUNT_MISMATCH') {
          uni.removeStorageSync('xianzhiMiniProgramAuth')
          redirectToLogin()
        }
        return false
      })
      .finally(() => {
        unauthorizedRefreshPromise = null
      })
  }
  const recovered = await unauthorizedRefreshPromise
  if (!recovered && shouldOpenLogin) redirectToLogin()
  return recovered
}

const detectedClientPlatform = adapter.getClientInfo().platform
const clientName = detectedClientPlatform === 'app-android' || detectedClientPlatform === 'app-ios'
  ? 'xianzhi-user-app'
  : detectedClientPlatform === 'mp-weixin'
    ? 'xianzhi-mini-program'
  : 'xianzhi-user-web'

const sharedApiClient = configureApiClient({
  adapter,
  baseURL: apiBaseURL,
  timeout: requestTimeout,
  clientName,
  clientVersion: env.VITE_APP_VERSION || env.VITE_APP_BUILD_VERSION || '0.1.0',
  getToken: getAuthToken,
  defaultHeaders: currentContextHeaders,
  onUnauthorized: handleUnauthorized,
})

export const apiClient = sharedApiClient

export const authService = createAuthService({
  adapter,
  api: apiClient,
  tokenKey,
  refreshTokenKey,
  authKey,
})

export const businessSdk = createBusinessSdk(apiClient)

export async function apiFetchResponse(
  path: string,
  init: RequestInit = {},
  options: { auth?: boolean; retriedAfterRefresh?: boolean } = {},
) {
  if (typeof fetch !== 'function') throw new Error('当前运行环境不支持该请求方式')
  const usesSession = options.auth ?? trustedApiURL(path)
  const headers = clientTransportHeaders(normalizeHeaders(init.headers), usesSession)
  const response = await fetch(resolveApiURL(path), { ...init, headers })
  if (usesSession && response.status === 401) {
    const retryAttempt = options.retriedAfterRefresh ? 1 : 0
    const recovered = await handleUnauthorized({ path, statusCode: response.status, payload: null, retryAttempt })
    if (recovered && retryAttempt === 0) return apiFetchResponse(path, init, { ...options, retriedAfterRefresh: true })
  }
  if (!response.ok) throw new Error(await fetchErrorMessage(response))
  return response
}

export interface ApiRequestTaskHandle<T> {
  promise: Promise<T>
  abort: () => void
}

export interface ApiRequestTaskOptions<TBody = unknown> {
  method?: string
  headers?: Record<string, string>
  data?: TBody
  timeout?: number
  auth?: boolean
}

export function apiRequestTask<T = unknown, TBody = unknown>(
  path: string,
  options: ApiRequestTaskOptions<TBody> = {},
): ApiRequestTaskHandle<T> {
  let requestTask: UniApp.RequestTask | null = null
  const usesSession = options.auth ?? trustedApiURL(path)
  const promise = new Promise<T>((resolve, reject) => {
    requestTask = uni.request({
      url: resolveApiURL(path),
      method: (options.method || 'GET') as UniApp.RequestOptions['method'],
      header: clientTransportHeaders(options.headers, usesSession),
      data: options.data as UniApp.RequestOptions['data'],
      timeout: options.timeout || requestTimeout,
      success(response) {
        if (usesSession && response.statusCode === 401) {
          void handleUnauthorized({ path, statusCode: response.statusCode, payload: response.data, retryAttempt: 0 })
        }
        if (response.statusCode < 200 || response.statusCode >= 300) {
          reject(new Error(responsePayloadMessage(response.data, `请求失败 (${response.statusCode})`, response.statusCode)))
          return
        }
        try {
          resolve(unwrapTransportPayload<T>(response.data))
        }
        catch (error) {
          reject(error)
        }
      },
      fail(error) {
        reject(new Error(toChineseApiErrorMessage(error.errMsg, { fallback: '请求失败，请检查网络' })))
      },
    }) as UniApp.RequestTask
  })
  return { promise, abort: () => requestTask?.abort() }
}

export function uploadApiFile<T = unknown>(
  path: string,
  options: { filePath: string; name?: string; formData?: Record<string, unknown>; headers?: Record<string, string>; timeout?: number; auth?: boolean; retriedAfterRefresh?: boolean },
): Promise<T> {
  if (!adapter.uploadFile) return Promise.reject(new Error('当前运行环境不支持文件上传'))
  const usesSession = options.auth ?? trustedApiURL(path)
  return adapter.uploadFile<T>({
    url: resolveApiURL(path),
    filePath: options.filePath,
    name: options.name || 'file',
    formData: options.formData,
    header: clientTransportHeaders(options.headers, usesSession),
    timeout: options.timeout || requestTimeout,
  }).then(response => {
    if (usesSession && response.statusCode === 401) {
      const retryAttempt = options.retriedAfterRefresh ? 1 : 0
      return handleUnauthorized({ path, statusCode: response.statusCode, payload: response.data, retryAttempt })
        .then(recovered => {
          if (recovered && retryAttempt === 0) return uploadApiFile<T>(path, { ...options, retriedAfterRefresh: true })
          if (response.statusCode < 200 || response.statusCode >= 300) {
            throw new Error(responsePayloadMessage(response.data, `文件上传失败 (${response.statusCode})`, response.statusCode))
          }
          return unwrapTransportPayload<T>(response.data)
        })
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw new Error(responsePayloadMessage(response.data, `文件上传失败 (${response.statusCode})`, response.statusCode))
    }
    return unwrapTransportPayload<T>(response.data)
  })
}

export async function downloadApiFile<T = unknown>(
  path: string,
  options: { headers?: Record<string, string>; timeout?: number; auth?: boolean; retriedAfterRefresh?: boolean } = {},
): Promise<AdapterDownloadFileResponse<T>> {
  if (!adapter.downloadFile) throw new Error('当前运行环境不支持文件下载')
  const usesSession = options.auth ?? trustedApiURL(path)
  const response = await adapter.downloadFile<T>({
    url: resolveApiURL(path),
    header: clientTransportHeaders({ Accept: '*/*', ...options.headers }, usesSession),
    timeout: options.timeout || requestTimeout,
  })
  if (usesSession && response.statusCode === 401) {
    const retryAttempt = options.retriedAfterRefresh ? 1 : 0
    const recovered = await handleUnauthorized({ path, statusCode: response.statusCode, payload: response.data, retryAttempt })
    if (recovered && retryAttempt === 0) return downloadApiFile(path, { ...options, retriedAfterRefresh: true })
  }
  if (response.statusCode < 200 || response.statusCode >= 300) {
    throw new Error(responsePayloadMessage(response.data, `文件下载失败 (${response.statusCode})`, response.statusCode))
  }
  return response
}

export function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  return sharedApiClient.request<T>(path, {
    method: init.method || 'GET',
    headers: normalizeHeaders(init.headers),
    body: normalizeBody(init.body),
    timeout: requestTimeout,
  })
}
