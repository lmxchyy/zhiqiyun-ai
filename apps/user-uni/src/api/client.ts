import { configureApiClient } from '@xianzhi/api-client'
import { createBusinessSdk } from '@xianzhi/business-sdk'
import { createUniPlatformAdapter } from '@xianzhi/platform-adapter'
import { createAuthService, createAuthStorage } from '@xianzhi/shared-auth'

const tokenKey = 'token'
const refreshTokenKey = 'refreshToken'
const authKey = 'auth'
const requestTimeout = 600000
const rawEnv = (import.meta as unknown as { env?: Record<string, string | boolean | undefined> }).env || {}
const env = rawEnv as Record<string, string | undefined>
const adapter = createUniPlatformAdapter()
let defaultApiBaseURL = ''
// #ifdef MP-WEIXIN
defaultApiBaseURL = 'http://127.0.0.1:3100'
// #endif
const apiBaseURL = String(env.VITE_API_BASE_URL || defaultApiBaseURL).replace(/\/+$/, '')
const isDevBuild = rawEnv.DEV === true || env.MODE === 'development'
const enableMockLogin = isDevBuild && String(env.VITE_ENABLE_MOCK_LOGIN || '').toLowerCase() === 'true'
let unauthorizedRedirecting = false

export const authStorage = createAuthStorage({
  adapter,
  tokenKey,
  refreshTokenKey,
  authKey,
})

export function getApiBaseURL(): string {
  return sharedApiClient.getBaseURL()
}

export function getAuthToken(): string {
  return authStorage.getToken()
}

export function setAuthToken(token: string) {
  authStorage.setToken(token)
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

function handleUnauthorized() {
  authStorage.clearSession()
  if (unauthorizedRedirecting) return
  unauthorizedRedirecting = true
  // #ifdef MP-WEIXIN
  const pages = typeof getCurrentPages === 'function' ? getCurrentPages() : []
  const current = pages.length ? pages[pages.length - 1] as unknown as { route?: string; options?: Record<string, unknown> } : null
  const currentPath = current?.route ? `/${String(current.route).replace(/^\/+/, '')}` : ''
  const query = current?.options
    ? Object.entries(current.options).map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(String(value ?? ''))}`).join('&')
    : ''
  const loginQuery = currentPath && currentPath !== '/pages/WechatLoginPage'
    ? `?redirectPath=${encodeURIComponent(currentPath)}&redirectQuery=${encodeURIComponent(query)}&sourcePage=${encodeURIComponent(currentPath)}`
    : ''
  uni.reLaunch({
    url: `/pages/WechatLoginPage${loginQuery}`,
    complete: () => { unauthorizedRedirecting = false },
  })
  // #endif
  // #ifndef MP-WEIXIN
  if (typeof window !== 'undefined' && !['/login', '/register'].includes(window.location.pathname)) {
    window.location.assign('/login')
  }
  unauthorizedRedirecting = false
  // #endif
}

const sharedApiClient = configureApiClient({
  adapter,
  baseURL: apiBaseURL,
  timeout: requestTimeout,
  clientName: 'xianzhi-user-web',
  clientVersion: env.VITE_APP_VERSION || env.VITE_APP_BUILD_VERSION || '0.1.0',
  getToken: getAuthToken,
  defaultHeaders: () => {
    const auth = authStorage.getAuth()
    const headers: Record<string, string> = {}
    if (auth?.tenantId) headers['X-Tenant-Id'] = auth.tenantId
    if (auth?.organizationId) headers['X-Organization-Id'] = auth.organizationId
    return headers
  },
  onUnauthorized: handleUnauthorized,
})

export const apiClient = sharedApiClient

export const authService = createAuthService({
  adapter,
  api: apiClient,
  tokenKey,
  refreshTokenKey,
  authKey,
  wechatMockCode: enableMockLogin ? 'mock-devtools-code' : undefined,
})

export const businessSdk = createBusinessSdk(apiClient)

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  return sharedApiClient.request<T>(path, {
    method: init.method || 'GET',
    headers: normalizeHeaders(init.headers),
    body: normalizeBody(init.body),
    timeout: requestTimeout,
  })
}
