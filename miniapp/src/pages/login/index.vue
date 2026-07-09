<template>
  <wd-config-provider :theme-vars="wotThemeVars">
    <view class="login-page">
      <view class="login-card zq-card">
        <view class="brand-row">
          <image class="login-logo" :src="loginLogo" mode="aspectFit" />
          <view class="brand-text">
            <text class="brand-name">知启云 AI</text>
            <text class="brand-subtitle">Unified Login</text>
          </view>
        </view>

        <view class="entry-pill">
          <text>统一入口</text>
        </view>

        <text class="login-title">登录知启云 AI</text>
        <text class="login-copy">一个入口进入用户端、代理端和主控后台。</text>

        <view class="form-area">
          <view class="field">
            <text class="field-label">邮箱</text>
            <input v-model="loginEmail" class="field-input" type="text" confirm-type="next" />
          </view>

          <view class="field">
            <text class="field-label">密码</text>
            <input v-model="loginPassword" class="field-input" password placeholder="请输入密码" confirm-type="done" />
          </view>

          <view class="remember-row" @click="keepSignedIn = !keepSignedIn">
            <view :class="['remember-box', { 'remember-box--checked': keepSignedIn }]">
              <text v-if="keepSignedIn">✓</text>
            </view>
            <text>保持登录</text>
          </view>

          <wd-button
            block
            type="primary"
            :loading="appStore.loginLoading && activeLogin === 'password'"
            custom-class="password-login-button"
            @click="handlePasswordLogin"
          >
            登录
          </wd-button>

          <wd-button
            block
            plain
            :loading="appStore.loginLoading && activeLogin === 'wechat'"
            custom-class="wechat-login-button"
            @click="handleWechatLogin"
          >
            微信小程序登录
          </wd-button>

          <text class="register-link" @click="showRegisterTip">没有账号，通过邀请码注册</text>
          <text v-if="statusText" :class="['login-status', `login-status--${statusTone}`]">
            {{ statusText }}
          </text>
        </view>
      </view>
      <wd-toast />
    </view>
  </wd-config-provider>
</template>

<script setup lang="ts">
import { onShow } from '@dcloudio/uni-app'
import { ref } from 'vue'
import { wotThemeVars } from '@/constants/theme'
import loginLogo from '@/static/zhiqiyun-logo-transparent.png'
import { useAppStore } from '@/stores/app'

definePage({
  type: 'home',
  style: {
    navigationStyle: 'custom',
    navigationBarTitleText: '登录',
  },
})

const appStore = useAppStore()
const loginEmail = ref('demo@xianzhi.ai')
const loginPassword = ref('')
const keepSignedIn = ref(true)
const statusText = ref('')
const statusTone = ref<'idle' | 'error'>('idle')
const activeLogin = ref<'password' | 'wechat' | ''>('')

function goHome() {
  uni.switchTab({ url: '/pages/index/index' })
}

function getErrorMessage(error: unknown) {
  if (error instanceof Error && error.message) return error.message
  if (error && typeof error === 'object' && 'errMsg' in error) {
    const errMsg = (error as { errMsg?: unknown }).errMsg
    if (typeof errMsg === 'string' && errMsg) return errMsg
  }
  return '登录失败，请稍后重试'
}

function completeLogin() {
  uni.showToast({ title: '登录成功', icon: 'success' })
  setTimeout(goHome, 280)
}

async function handlePasswordLogin() {
  if (!loginEmail.value.trim() || !loginPassword.value.trim()) {
    statusTone.value = 'error'
    statusText.value = '请输入邮箱和密码'
    uni.showToast({ title: statusText.value, icon: 'none' })
    return
  }

  activeLogin.value = 'password'
  statusText.value = ''
  try {
    await appStore.loginWithPassword(loginEmail.value.trim(), loginPassword.value)
    completeLogin()
  }
  catch (error) {
    statusTone.value = 'error'
    statusText.value = getErrorMessage(error)
    uni.showToast({ title: statusText.value, icon: 'none' })
  }
  finally {
    activeLogin.value = ''
  }
}

async function handleWechatLogin() {
  activeLogin.value = 'wechat'
  statusText.value = ''
  try {
    await appStore.loginWithWechatMiniProgram()
    completeLogin()
  }
  catch (error) {
    statusTone.value = 'error'
    statusText.value = getErrorMessage(error)
    uni.showToast({ title: statusText.value, icon: 'none' })
  }
  finally {
    activeLogin.value = ''
  }
}

function showRegisterTip() {
  uni.showToast({ title: '注册入口已预留', icon: 'none' })
}

onShow(() => {
  appStore.syncSession()
  if (appStore.isLoggedIn) {
    goHome()
  }
})
</script>

<style scoped lang="scss">
.login-page {
  display: flex;
  min-height: 100vh;
  align-items: center;
  justify-content: center;
  padding: 56rpx 32rpx;
  background:
    radial-gradient(circle at 12% 4%, rgba(125, 141, 246, 0.2), transparent 36%),
    radial-gradient(circle at 94% 18%, rgba(255, 119, 27, 0.12), transparent 30%),
    var(--color-bg-page);
}

.login-card {
  width: 100%;
  max-width: 660rpx;
  padding: 54rpx 56rpx 52rpx;
}

.brand-row {
  display: flex;
  align-items: center;
  gap: 22rpx;
}

.login-logo {
  width: 92rpx;
  height: 92rpx;
  flex: 0 0 92rpx;
}

.brand-text {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 6rpx;
}

.brand-name {
  color: var(--color-text-primary);
  font-size: 34rpx;
  font-weight: 900;
  line-height: 1.15;
}

.brand-subtitle {
  color: var(--color-text-secondary);
  font-size: 22rpx;
  font-weight: 800;
}

.entry-pill {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 48rpx;
  margin-top: 34rpx;
  border-radius: var(--radius-sm);
  background: #14B8A6;
  color: #fff;
  font-size: 24rpx;
  font-weight: 900;
}

.login-title {
  display: block;
  margin-top: 24rpx;
  color: var(--color-text-primary);
  font-size: 50rpx;
  font-weight: 900;
  line-height: 1.12;
}

.login-copy {
  display: block;
  margin-top: 18rpx;
  color: #475569;
  font-size: 26rpx;
  font-weight: 700;
  line-height: 1.5;
}

.form-area {
  margin-top: 42rpx;
}

.field + .field {
  margin-top: 26rpx;
}

.field-label {
  display: block;
  color: var(--color-text-primary);
  font-size: 25rpx;
  font-weight: 800;
}

.field-input {
  width: 100%;
  height: 84rpx;
  margin-top: 14rpx;
  padding: 0 22rpx;
  border: 1rpx solid #CBD5E1;
  border-radius: var(--radius-sm);
  background: #fff;
  color: var(--color-text-primary);
  font-size: 27rpx;
  font-weight: 800;
}

.remember-row {
  display: flex;
  align-items: center;
  gap: 12rpx;
  margin-top: 28rpx;
  color: var(--color-text-primary);
  font-size: 26rpx;
  font-weight: 700;
}

.remember-box {
  display: flex;
  width: 30rpx;
  height: 30rpx;
  align-items: center;
  justify-content: center;
  border: 2rpx solid #CBD5E1;
  border-radius: 4rpx;
  color: #fff;
  font-size: 22rpx;
  font-weight: 900;
}

.remember-box--checked {
  border-color: #1677FF;
  background: #1677FF;
}

:deep(.password-login-button),
:deep(.wechat-login-button) {
  height: 88rpx !important;
  border-radius: var(--radius-sm) !important;
  font-size: 28rpx !important;
  font-weight: 900 !important;
}

:deep(.password-login-button) {
  margin-top: 30rpx !important;
  border-color: var(--color-primary-dark) !important;
  background: linear-gradient(135deg, var(--color-primary), var(--color-primary-dark)) !important;
  color: #fff !important;
}

:deep(.wechat-login-button) {
  margin-top: 26rpx !important;
  border-color: #CBD5E1 !important;
  background: #fff !important;
  color: #059669 !important;
}

.register-link {
  display: block;
  margin-top: 28rpx;
  color: var(--color-primary-dark);
  font-size: 25rpx;
  font-weight: 800;
  text-align: center;
}

.login-status {
  display: block;
  margin-top: 18rpx;
  color: var(--color-text-muted);
  font-size: 24rpx;
  line-height: 1.5;
  text-align: center;
}

.login-status--error {
  color: #DC2626;
}
</style>
