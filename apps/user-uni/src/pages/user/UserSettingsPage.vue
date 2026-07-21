<template>
  <view class="mpb-page">
    <view class="mpb-safe" />
    <view class="mpb-header">
      <button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/user/UserMinePage')">‹</button>
      <image class="mpb-logo" :src="loginLogo" mode="aspectFit" />
      <view class="mpb-header-copy">
        <text class="mpb-title">设置</text>
        <text class="mpb-subtitle">通知、安全与应用偏好</text>
      </view>
      <text class="mpb-role">普通用户</text>
    </view>

    <view class="mpb-stack">
      <view class="mpb-hero">
        <view class="mpb-hero-top">
          <view>
            <text class="mpb-hero-label">账号与安全</text>
            <text class="mpb-hero-value">{{ security.status === "ACTIVE" ? "账号状态正常" : "正在同步安全状态" }}</text>
          </view>
          <text class="mpb-hero-badge success">{{ security.status === "ACTIVE" ? "安全" : "同步中" }}</text>
        </view>
        <text class="mpb-hero-copy">手机号是主账号身份，微信身份用于快捷登录；敏感操作会进行二次确认。</text>
        <view class="mpb-hero-metrics">
          <view class="mpb-hero-metric">
            <text class="mpb-hero-metric-value">{{ loginMethodCount }}</text>
            <text class="mpb-hero-metric-label">登录方式</text>
          </view>
          <view class="mpb-hero-metric">
            <text class="mpb-hero-metric-value">{{ security.mobileMasked || "未绑定" }}</text>
            <text class="mpb-hero-metric-label">安全手机号</text>
          </view>
          <view class="mpb-hero-metric">
            <text class="mpb-hero-metric-value">{{ security.wechatLinked ? "已绑定" : "未绑定" }}</text>
            <text class="mpb-hero-metric-label">微信身份</text>
          </view>
        </view>
      </view>

      <view class="mpb-card mpb-list">
        <view class="mpb-section-head">
          <text class="mpb-card-title">登录身份</text>
          <text class="mpb-card-copy">不展示 openid、unionid 或 Token</text>
        </view>
        <view class="mpb-row">
          <text class="mpb-row-icon green">手</text>
          <view class="mpb-row-main">
            <text class="mpb-row-title">手机号</text>
            <text class="mpb-row-meta">{{ security.mobileMasked || "绑定后可使用短信验证码登录" }}</text>
          </view>
          <text class="mpb-amount">{{ security.mobileMasked ? "已绑定" : "未绑定" }}</text>
        </view>
        <view class="mpb-row">
          <text class="mpb-row-icon">微</text>
          <view class="mpb-row-main">
            <text class="mpb-row-title">微信小程序</text>
            <text class="mpb-row-meta">用于微信内快捷登录和支付身份校验</text>
          </view>
          <text class="mpb-amount">{{ security.wechatLinked ? "已绑定" : "未绑定" }}</text>
        </view>
      </view>

      <view class="mpb-card">
        <text class="mpb-card-title">{{ security.mobileMasked ? "修改手机号" : "绑定手机号" }}</text>
        <text class="mpb-card-copy">验证码只用于绑定当前账号，不会创建新账号。</text>
        <view class="mpb-field">
          <text class="mpb-label">手机号</text>
          <input v-model="mobileBind.mobile" class="mpb-input" type="number" maxlength="11" placeholder="请输入 11 位手机号" />
        </view>
        <view class="mpb-field">
          <text class="mpb-label">验证码</text>
          <input v-model="mobileBind.smsCode" class="mpb-input" type="number" maxlength="6" placeholder="请输入短信验证码" />
        </view>
        <view class="mpb-inline-actions">
          <button class="mpb-button secondary" :disabled="smsSending || countdown > 0 || bindingMobile" @click="sendBindSms">
            {{ smsSending ? "发送中..." : countdown > 0 ? `${countdown}s 后重发` : "发送验证码" }}
          </button>
          <button class="mpb-button primary" :disabled="bindingMobile" @click="bindMobile">
            {{ bindingMobile ? "绑定中..." : security.mobileMasked ? "确认修改" : "确认绑定" }}
          </button>
        </view>
      </view>

      <view class="mpb-card">
        <text class="mpb-card-title">微信绑定</text>
        <text class="mpb-card-copy">{{ security.wechatLinked ? "当前账号已绑定微信小程序身份。" : "绑定后可用微信快速登录同一个账号。" }}</text>
        <button class="mpb-button secondary" :disabled="security.wechatLinked || bindingWechat" @click="bindWechat">
          {{ bindingWechat ? "绑定中..." : security.wechatLinked ? "微信已绑定" : "绑定微信" }}
        </button>
      </view>

      <view class="mpb-card">
        <text class="mpb-card-title">{{ security.passwordSet ? "修改密码" : "设置登录密码" }}</text>
        <text class="mpb-card-copy">{{ security.passwordSet ? "修改后旧密码立即失效" : "首次使用微信或验证码登录的账号无需输入当前密码" }}</text>
        <view v-if="security.passwordSet" class="mpb-field">
          <text class="mpb-label">当前密码</text>
          <input v-model="password.current" class="mpb-input" password placeholder="请输入当前密码" />
        </view>
        <view class="mpb-field">
          <text class="mpb-label">新密码</text>
          <input v-model="password.next" class="mpb-input" password placeholder="至少 8 位" />
        </view>
        <view class="mpb-field">
          <text class="mpb-label">确认新密码</text>
          <input v-model="password.confirm" class="mpb-input" password placeholder="再次输入新密码" />
        </view>
        <button class="mpb-button secondary" :disabled="changing" @click="changePassword">{{ changing ? "提交中..." : security.passwordSet ? "更新密码" : "设置密码" }}</button>
      </view>

      <view class="mpb-card mpb-list">
        <view class="mpb-section-head">
          <text class="mpb-card-title">通用设置</text>
          <text class="mpb-card-copy">即时生效</text>
        </view>
        <view class="mpb-row" @click="toggleNotifications">
          <text class="mpb-row-icon green">铃</text>
          <view class="mpb-row-main">
            <text class="mpb-row-title">消息通知</text>
            <text class="mpb-row-meta">订单、到账和任务完成提醒</text>
          </view>
          <view :class="['mpb-switch', { on: notificationsEnabled }]" />
        </view>
        <view class="mpb-row" @click="clearCache">
          <text class="mpb-row-icon orange">清</text>
          <view class="mpb-row-main">
            <text class="mpb-row-title">清理缓存</text>
            <text class="mpb-row-meta">释放本地临时文件</text>
          </view>
          <text class="mpb-amount">清理</text>
        </view>
        <view class="mpb-row" @click="showInfo('关于知启云 AI', '知启云 AI 企业智能工作平台，小程序版本 4.0.0。')">
          <text class="mpb-row-icon">i</text>
          <view class="mpb-row-main">
            <text class="mpb-row-title">关于知启云 AI</text>
            <text class="mpb-row-meta">版本 4.0.0 · 服务协议</text>
          </view>
          <text class="mpb-amount">最新</text>
        </view>
      </view>

      <view class="mpb-inline-actions">
        <button class="mpb-button secondary" @click="showInfo('隐私政策', '隐私政策正文将由运营后台配置并在正式发布前完成审核。')">隐私政策</button>
        <button class="mpb-button secondary" @click="showInfo('用户协议', '用户协议正文将由运营后台配置并在正式发布前完成审核。')">用户协议</button>
        <button class="mpb-button secondary" @click="openComplianceCenter">协议、AI规范与投诉</button>
      </view>
      <view class="mpb-inline-actions">
        <button class="mpb-button secondary" :disabled="loggingOutAll || loggingOut" @click="logoutAllDevices">{{ loggingOutAll ? "正在退出..." : "退出全部设备" }}</button>
        <button class="mpb-button danger" :disabled="loggingOut || loggingOutAll" @click="logout">{{ loggingOut ? "正在退出..." : "退出当前账号" }}</button>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import { onShow } from "@dcloudio/uni-app";
import { authStorage } from "../../api/client";
import { loginAPI } from "../../features/auth/api";
import { useAuthStore } from "../../stores/auth";
import type { AccountSecurityResponse } from "../../features/auth/types";
import { backOrHome } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";

const changing = ref(false);
const authStore = useAuthStore();
const loggingOut = ref(false);
const loggingOutAll = ref(false);
const smsSending = ref(false);
const bindingMobile = ref(false);
const bindingWechat = ref(false);
const countdown = ref(0);
let countdownTimer: ReturnType<typeof setInterval> | null = null;

const notificationsEnabled = ref(uni.getStorageSync("miniProgramNotificationsEnabled") !== false);
const password = reactive({ current: "", next: "", confirm: "" });
const mobileBind = reactive({ mobile: "", smsCode: "" });
const security = reactive<AccountSecurityResponse>({ passwordSet: false, mobileMasked: "", mobileBound: false, wechatLinked: false, loginMethods: [], status: "" });
const loginMethodCount = computed(() => Math.max(1, security.loginMethods?.length || Number(Boolean(security.mobileMasked)) + Number(security.wechatLinked) + Number(security.passwordSet)));

function showInfo(title: string, content: string) {
  uni.showModal({ title, content, showCancel: false });
}

function openComplianceCenter() {
  uni.navigateTo({ url: "/pages/user/ComplianceCenterPage" });
}

function normalizeMobile(value: string) {
  return value.replace(/\D/g, "").replace(/^86(?=\d{11}$)/, "");
}

function isValidMobile(value: string) {
  return /^1[3-9]\d{9}$/.test(normalizeMobile(value));
}

function startCountdown(seconds: number) {
  countdown.value = Math.max(1, seconds || 60);
  if (countdownTimer) clearInterval(countdownTimer);
  countdownTimer = setInterval(() => {
    countdown.value -= 1;
    if (countdown.value <= 0 && countdownTimer) {
      clearInterval(countdownTimer);
      countdownTimer = null;
    }
  }, 1000);
}

function toggleNotifications() {
  notificationsEnabled.value = !notificationsEnabled.value;
  uni.setStorageSync("miniProgramNotificationsEnabled", notificationsEnabled.value);
}

function clearCache() {
  ["miniProgramSearchHistory", "miniProgramOrderFilter", "miniProgramCreationDrafts"].forEach(key => uni.removeStorageSync(key));
  uni.showToast({ title: "缓存已清除", icon: "success" });
}

async function sendBindSms() {
  const mobile = normalizeMobile(mobileBind.mobile);
  if (!isValidMobile(mobile)) return void uni.showToast({ title: "请输入正确的手机号", icon: "none" });
  if (smsSending.value || countdown.value > 0) return;
  smsSending.value = true;
  try {
    const result = await loginAPI.sendSms(mobile, "bind_mobile");
    mobileBind.mobile = mobile;
    startCountdown(result.retryAfterSeconds || 60);
    uni.showToast({ title: "验证码已发送", icon: "success" });
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "验证码发送失败", icon: "none" });
  } finally {
    smsSending.value = false;
  }
}

async function bindMobile() {
  const mobile = normalizeMobile(mobileBind.mobile);
  if (!isValidMobile(mobile)) return void uni.showToast({ title: "请输入正确的手机号", icon: "none" });
  if (!/^\d{4,6}$/.test(mobileBind.smsCode.trim())) return void uni.showToast({ title: "请输入验证码", icon: "none" });
  bindingMobile.value = true;
  try {
    const result = await loginAPI.bindMobile(mobile, mobileBind.smsCode.trim());
    if (result.auth) {
      const current = authStorage.getAuth();
      authStorage.setAuth(current ? { ...current, ...result.auth, accessToken: current.accessToken, refreshToken: current.refreshToken } : result.auth);
    }
    if (result.security) Object.assign(security, result.security);
    else await loadSecurity();
    mobileBind.smsCode = "";
    uni.showToast({ title: "手机号已绑定", icon: "success" });
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "手机号绑定失败", icon: "none" });
  } finally {
    bindingMobile.value = false;
  }
}

async function bindWechat() {
  if (security.wechatLinked || bindingWechat.value) return;
  bindingWechat.value = true;
  try {
    const loginResult = await new Promise<UniApp.LoginRes>((resolve, reject) => {
      uni.login({ provider: "weixin", success: resolve, fail: reject });
    });
    const wxLoginCode = String(loginResult.code || "");
    if (!wxLoginCode) throw new Error("未获取到微信登录凭证");
    await loginAPI.linkWechat(wxLoginCode);
    await loadSecurity();
    uni.showToast({ title: "微信已绑定", icon: "success" });
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "微信绑定失败", icon: "none" });
  } finally {
    bindingWechat.value = false;
  }
}

async function changePassword() {
  if (security.passwordSet && !password.current) return void uni.showToast({ title: "请输入当前密码", icon: "none" });
  if (password.next.length < 8) return void uni.showToast({ title: "新密码至少 8 位", icon: "none" });
  if (password.next !== password.confirm) return void uni.showToast({ title: "两次密码不一致", icon: "none" });
  changing.value = true;
  try {
    await loginAPI.changePassword(password.current, password.next);
    password.current = password.next = password.confirm = "";
    security.passwordSet = true;
    uni.showToast({ title: "密码已安全保存", icon: "success" });
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "密码更新失败", icon: "none" });
  } finally {
    changing.value = false;
  }
}

async function loadSecurity() {
  try {
    Object.assign(security, await loginAPI.security());
  } catch {
    // 页面其余设置仍可使用。
  }
}

function clearLocalAuthAndReturnLogin() {
  authStore.logout();
  uni.removeStorageSync("xianzhiMiniProgramAuth");
  uni.switchTab({ url: "/pages/user/UserHomePage" });
}

function logout() {
  uni.showModal({
    title: "退出登录",
    content: "退出后需要重新登录，云端作品和订单不会删除。",
    success: async result => {
      if (!result.confirm) return;
      loggingOut.value = true;
      try { await loginAPI.logout(); } catch { /* 本地会话仍需清理 */ }
      clearLocalAuthAndReturnLogin();
    },
  });
}

function logoutAllDevices() {
  uni.showModal({
    title: "退出全部设备",
    content: "确认后当前账号在其他设备上的登录状态也会失效，需要重新登录。",
    success: async result => {
      if (!result.confirm) return;
      loggingOutAll.value = true;
      try {
        await loginAPI.logoutAll();
        uni.showToast({ title: "已退出全部设备", icon: "success" });
      } catch (error) {
        uni.showToast({ title: error instanceof Error ? error.message : "退出全部设备失败", icon: "none" });
      } finally {
        clearLocalAuthAndReturnLogin();
      }
    },
  });
}

onShow(() => void loadSecurity());
</script>

<style>@import "../../styles/mini-program-business.css";</style>
