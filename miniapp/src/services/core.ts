import { configureApiClient } from '@xianzhi/api-client'
import { createBusinessSdk } from '@xianzhi/business-sdk'
import { createUniPlatformAdapter } from '@xianzhi/platform-adapter'
import { createAuthService, createAuthStorage } from '@xianzhi/shared-auth'

export const MINI_AUTH_STORAGE_KEY = 'zq_mini_auth'
export const MINI_TOKEN_STORAGE_KEY = 'zq_mini_token'
export const MINI_REFRESH_TOKEN_STORAGE_KEY = 'zq_mini_refresh_token'

const rawEnv = (import.meta as unknown as { env?: Record<string, string | boolean | undefined> }).env || {}
const env = rawEnv as Record<string, string | undefined>
const adapter = createUniPlatformAdapter()
let defaultApiBaseURL = ''
// #ifdef MP-WEIXIN
// WeChat mini programs cannot rely on the H5 dev proxy.
defaultApiBaseURL = 'http://127.0.0.1:3100'
// #endif

const apiBaseURL = String(env.VITE_API_BASE_URL || env.VITE_SERVER_BASEURL || defaultApiBaseURL).replace(/\/+$/, '')
const isDevBuild = rawEnv.DEV === true || env.MODE === 'development'
const enableMockLogin = isDevBuild && String(env.VITE_ENABLE_MOCK_LOGIN || '').toLowerCase() === 'true'

export const authStorage = createAuthStorage({
  adapter,
  tokenKey: MINI_TOKEN_STORAGE_KEY,
  refreshTokenKey: MINI_REFRESH_TOKEN_STORAGE_KEY,
  authKey: MINI_AUTH_STORAGE_KEY,
})

export const apiClient = configureApiClient({
  adapter,
  baseURL: apiBaseURL,
  clientName: 'xianzhi-user-uni',
  clientVersion: env.VITE_APP_VERSION || env.VITE_APP_BUILD_VERSION || '0.1.0',
  getToken: () => authStorage.getToken(),
  onUnauthorized: () => authStorage.clearSession(),
})

export const authService = createAuthService({
  adapter,
  api: apiClient,
  tokenKey: MINI_TOKEN_STORAGE_KEY,
  refreshTokenKey: MINI_REFRESH_TOKEN_STORAGE_KEY,
  authKey: MINI_AUTH_STORAGE_KEY,
  wechatMockCode: enableMockLogin ? 'mock-devtools-code' : undefined,
})

export const businessSdk = createBusinessSdk(apiClient)

export function getMiniApiBaseURL() {
  return apiClient.getBaseURL()
}
