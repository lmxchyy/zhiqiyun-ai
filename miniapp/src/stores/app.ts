import { defineStore } from 'pinia'
import { authService, authStorage, MINI_AUTH_STORAGE_KEY, MINI_REFRESH_TOKEN_STORAGE_KEY, MINI_TOKEN_STORAGE_KEY } from '@/services/core'
import type { AuthResponse } from '@xianzhi/shared-types'

export { MINI_AUTH_STORAGE_KEY, MINI_REFRESH_TOKEN_STORAGE_KEY, MINI_TOKEN_STORAGE_KEY }
export type MiniAuthResponse = AuthResponse

export const useAppStore = defineStore('app', {
  state: () => ({
    bootstrapped: false,
    token: '',
    auth: null as MiniAuthResponse | null,
    loginLoading: false,
  }),
  getters: {
    isLoggedIn: state => Boolean(state.token),
  },
  actions: {
    bootstrap() {
      this.bootstrapped = true
      this.syncSession()
      void this.restoreSession()
    },
    syncSession() {
      this.token = authStorage.getToken()
      this.auth = authStorage.getAuth()
    },
    setSession(auth: MiniAuthResponse) {
      this.auth = auth
      this.token = auth.accessToken || ''
      authStorage.setToken(this.token)
      if (Object.prototype.hasOwnProperty.call(auth, 'refreshToken')) {
        authStorage.setRefreshToken(auth.refreshToken || '')
      }
      authStorage.setAuth(auth)
    },
    async restoreSession() {
      const auth = await authService.restore()
      if (auth) this.setSession(auth)
      else this.syncSession()
      return auth
    },
    async loginWithWechatMiniProgram() {
      this.loginLoading = true
      try {
        const auth = await authService.loginByWechatMiniProgram()
        this.setSession(auth)
        return auth
      }
      finally {
        this.loginLoading = false
      }
    },
    async loginWithPassword(email: string, password: string) {
      this.loginLoading = true
      try {
        const auth = await authService.loginByPassword(email, password)
        this.setSession(auth)
        return auth
      }
      finally {
        this.loginLoading = false
      }
    },
    ensureAuthenticated() {
      this.syncSession()
      if (this.isLoggedIn) return true

      const pages = getCurrentPages()
      const currentRoute = pages[pages.length - 1]?.route
      if (currentRoute && currentRoute !== 'pages/login/index') {
        uni.reLaunch({ url: '/pages/login/index' })
      }
      return false
    },
    logout() {
      this.token = ''
      this.auth = null
      authStorage.clear()
    },
  },
})
