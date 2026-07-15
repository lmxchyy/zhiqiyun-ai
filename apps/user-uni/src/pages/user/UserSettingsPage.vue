<template>
  <view class="mpb-page">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/user/UserMinePage')">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">设置</text><text class="mpb-subtitle">通知、安全与应用偏好</text></view><text class="mpb-role">普通用户</text></view>
    <view class="mpb-stack">
      <view class="mpb-hero"><view class="mpb-hero-top"><view><text class="mpb-hero-label">账号与安全</text><text class="mpb-hero-value">{{ security.status === "ACTIVE" ? "账号状态正常" : "正在同步安全状态" }}</text></view><text class="mpb-hero-badge success">{{ security.status === "ACTIVE" ? "安全" : "同步中" }}</text></view><text class="mpb-hero-copy">手机号是主账号身份，微信身份用于快捷登录；敏感操作仍需二次确认。</text><view class="mpb-hero-metrics"><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ loginMethodCount }}</text><text class="mpb-hero-metric-label">登录方式</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ security.mobileMasked || "未绑定" }}</text><text class="mpb-hero-metric-label">安全手机号</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ security.passwordSet ? "已设置" : "未设置" }}</text><text class="mpb-hero-metric-label">账号密码</text></view></view></view>
      <view class="mpb-card mpb-list">
        <view class="mpb-section-head"><text class="mpb-card-title">通用设置</text><text class="mpb-card-copy">即时生效</text></view>
        <view class="mpb-row" @click="toggleNotifications"><text class="mpb-row-icon green">铃</text><view class="mpb-row-main"><text class="mpb-row-title">消息通知</text><text class="mpb-row-meta">订单、到账和任务完成提醒</text></view><view :class="['mpb-switch', { on: notificationsEnabled }]" /></view>
        <view class="mpb-row" @click="showInfo('隐私与安全', '可在此管理密码、登录设备和微信授权。')"><text class="mpb-row-icon">盾</text><view class="mpb-row-main"><text class="mpb-row-title">隐私与安全</text><text class="mpb-row-meta">登录设备、密码与授权</text></view><text class="mpb-amount">查看</text></view>
        <view class="mpb-row" @click="clearCache"><text class="mpb-row-icon orange">清</text><view class="mpb-row-main"><text class="mpb-row-title">清理缓存</text><text class="mpb-row-meta">释放本地临时文件</text></view><text class="mpb-amount">清理</text></view>
        <view class="mpb-row" @click="showInfo('关于知启云 AI', '知启云 AI 企业智能工作平台，小程序版本 4.0.0。')"><text class="mpb-row-icon">i</text><view class="mpb-row-main"><text class="mpb-row-title">关于知启云</text><text class="mpb-row-meta">版本 4.0.0 · 服务协议</text></view><text class="mpb-amount">最新</text></view>
      </view>
      <view class="mpb-card">
        <text class="mpb-card-title">{{ security.passwordSet ? "修改密码" : "设置登录密码" }}</text>
        <text class="mpb-card-copy">{{ security.passwordSet ? "修改后旧密码立即失效" : "首次使用微信或验证码登录的账号无需输入当前密码" }}</text>
        <view v-if="security.passwordSet" class="mpb-field"><text class="mpb-label">当前密码</text><input v-model="password.current" class="mpb-input" password placeholder="请输入当前密码" /></view>
        <view class="mpb-field"><text class="mpb-label">新密码</text><input v-model="password.next" class="mpb-input" password placeholder="至少 8 位" /></view>
        <view class="mpb-field"><text class="mpb-label">确认新密码</text><input v-model="password.confirm" class="mpb-input" password placeholder="再次输入新密码" /></view>
        <button class="mpb-button secondary" :disabled="changing" @click="changePassword">{{ changing ? "提交中..." : security.passwordSet ? "更新密码" : "设置密码" }}</button>
      </view>
      <view class="mpb-inline-actions"><button class="mpb-button secondary" @click="showInfo('隐私政策', '隐私政策正文将由运营后台配置并在正式发布前完成审核。')">隐私政策</button><button class="mpb-button secondary" @click="showInfo('用户协议', '用户协议正文将由运营后台配置并在正式发布前完成审核。')">用户协议</button></view>
      <button class="mpb-button danger" :disabled="loggingOut" @click="logout">{{ loggingOut ? "正在退出..." : "退出当前账号" }}</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import { onShow } from "@dcloudio/uni-app";
import { api, authStorage } from "../../api/client";
import { loginAPI } from "../../features/auth/api";
import type { AccountSecurityResponse } from "../../features/auth/types";
import { backOrHome } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";

const changing = ref(false);
const loggingOut = ref(false);
const notificationsEnabled = ref(uni.getStorageSync("miniProgramNotificationsEnabled") !== false);
const password = reactive({ current: "", next: "", confirm: "" });
const security = reactive<AccountSecurityResponse>({ passwordSet: false, mobileMasked: "", wechatLinked: false, status: "" });
const loginMethodCount = computed(() => 1 + Number(security.wechatLinked) + Number(security.passwordSet));

function showInfo(title: string, content: string) { uni.showModal({ title, content, showCancel: false }); }
function toggleNotifications() { notificationsEnabled.value = !notificationsEnabled.value; uni.setStorageSync("miniProgramNotificationsEnabled", notificationsEnabled.value); }

function clearCache() {
  ["miniProgramSearchHistory", "miniProgramOrderFilter", "miniProgramCreationDrafts"].forEach(key => uni.removeStorageSync(key));
  uni.showToast({ title: "缓存已清除", icon: "success" });
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
  } finally { changing.value = false; }
}

async function loadSecurity() {
  try { Object.assign(security, await loginAPI.security()); }
  catch { /* 页面其余设置仍可使用 */ }
}

function logout() {
  uni.showModal({ title: "退出登录", content: "退出后需要重新登录，云端作品和订单不会删除。", success: async result => {
    if (!result.confirm) return;
    loggingOut.value = true;
    try { await api("/api/v1/auth/logout", { method: "POST" }); } catch { /* 本地会话仍需清理 */ }
    authStorage.clear();
    uni.removeStorageSync("xianzhiMiniProgramAuth");
    uni.reLaunch({ url: "/pages/WechatLoginPage" });
  }});
}

onShow(() => void loadSecurity());
</script>

<style>@import "../../styles/mini-program-business.css";</style>
