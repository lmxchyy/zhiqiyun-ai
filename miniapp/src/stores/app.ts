import { defineStore } from 'pinia'
import { loginByPassword, loginByWechatMiniProgram, type MiniAuthResponse } from '@/services/auth'

export const MINI_AUTH_STORAGE_KEY = 'zq_mini_auth'
export const MINI_TOKEN_STORAGE_KEY = 'zq_mini_token'

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
    },
    syncSession() {
      this.token = uni.getStorageSync(MINI_TOKEN_STORAGE_KEY) || ''
      this.auth = uni.getStorageSync(MINI_AUTH_STORAGE_KEY) || null
    },
    setSession(auth: MiniAuthResponse) {
      this.auth = auth
      this.token = auth.accessToken
      uni.setStorageSync(MINI_TOKEN_STORAGE_KEY, auth.accessToken)
      uni.setStorageSync(MINI_AUTH_STORAGE_KEY, auth)
    },
    async loginWithWechatMiniProgram() {
      this.loginLoading = true
      try {
        const auth = await loginByWechatMiniProgram()
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
        const auth = await loginByPassword(email, password)
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
      uni.removeStorageSync(MINI_TOKEN_STORAGE_KEY)
      uni.removeStorageSync(MINI_AUTH_STORAGE_KEY)
    },
  },
})
