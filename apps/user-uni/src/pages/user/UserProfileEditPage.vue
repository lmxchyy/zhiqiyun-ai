<template>
  <view class="mpb-page">
    <view class="mpb-safe" />
    <view class="mpb-header">
      <button class="mpb-back" aria-label="返回" @click="backOrHome()">‹</button>
      <image class="mpb-logo" :src="loginLogo" mode="aspectFit" />
      <view class="mpb-header-copy"><text class="mpb-title">编辑资料</text><text class="mpb-subtitle">维护昵称与企业信息</text></view>
      <text class="mpb-role">普通用户</text>
    </view>
    <view class="mpb-stack">
      <view class="mpb-hero light">
        <view class="mpb-hero-top"><view><text class="mpb-hero-label">当前账户</text><text class="mpb-hero-value">{{ form.name || "当前用户" }}</text></view><text class="mpb-hero-badge success">资料完整 {{ profileCompleteness }}%</text></view>
        <text class="mpb-hero-copy">资料更新只修改当前账户基本信息，不会影响点数余额、订单和身份。</text>
        <view class="mpb-hero-metrics"><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ roleLabel }}</text><text class="mpb-hero-metric-label">身份</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ form.email ? "已填写" : "待完善" }}</text><text class="mpb-hero-metric-label">邮箱</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ statusLabel }}</text><text class="mpb-hero-metric-label">状态</text></view></view>
      </view>
      <view class="mpb-card">
        <view class="mpb-section-head"><text class="mpb-card-title">基本资料</text><text class="mpb-card-copy">真实同步</text></view>
        <view class="mpb-field"><text class="mpb-label">显示名称</text><input v-model="form.name" class="mpb-input" maxlength="60" placeholder="请输入显示名称" /></view>
        <view class="mpb-field"><text class="mpb-label">登录邮箱</text><input v-model="form.email" class="mpb-input" maxlength="120" placeholder="请输入邮箱" /></view>
      </view>
      <view class="mpb-card mpb-list">
        <view class="mpb-row"><text class="mpb-row-icon green">手</text><view class="mpb-row-main"><text class="mpb-row-title">手机号</text><text class="mpb-row-meta">后台字段尚未开放</text></view><text class="mpb-status">暂不可编辑</text></view>
        <view class="mpb-row"><text class="mpb-row-icon">企</text><view class="mpb-row-main"><text class="mpb-row-title">企业名称</text><text class="mpb-row-meta">后台字段尚未开放</text></view><text class="mpb-status">暂不可编辑</text></view>
      </view>
      <view class="mpb-note">当前仅保存名称和邮箱。未接入的字段明确禁用，避免出现界面提示成功但服务端没有数据。</view>
      <button class="mpb-button" :disabled="loading || saving" @click="save">{{ saving ? "保存中..." : "保存资料" }}</button>
      <text class="mpb-footer-note">保存后同步到桌面端和微信小程序账户</text>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import { onShow } from "@dcloudio/uni-app";
import { ApiClientError } from "@xianzhi/api-client";
import { api, authStorage, businessSdk } from "../../api/client";
import { requireAuth } from "../../features/auth/gate";
import { backOrHome } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";

const loading = ref(false);
const saving = ref(false);
const role = ref("");
const status = ref("");
const form = reactive({ name: "", email: "" });
const roleLabel = computed(() => role.value.includes("AGENT") ? "代理商" : role.value.includes("OPERATION") ? "运营中心" : "普通用户");
const statusLabel = computed(() => status.value === "ACTIVE" ? "账户正常" : status.value || "状态未知");
const profileCompleteness = computed(() => (form.name ? 50 : 0) + (form.email ? 50 : 0));
let requestingLogin = false;

async function load() {
  loading.value = true;
  try {
    const profile = await businessSdk.roleWorkbench.memberProfile();
    form.name = profile.user?.name || "";
    form.email = profile.user?.email || "";
    role.value = profile.user?.role || "";
    status.value = profile.user?.status || "";
  } catch (error) {
    if (error instanceof ApiClientError && error.statusCode === 401) {
      requestingLogin = false;
      await loadWithAuthGate();
      return;
    }
    uni.showToast({ title: error instanceof Error ? error.message : "资料加载失败", icon: "none" });
  } finally {
    loading.value = false;
  }
}

async function loadWithAuthGate() {
  if (authStorage.getToken()) {
    requestingLogin = false;
    await load();
    return;
  }
  if (requestingLogin) return;
  requestingLogin = true;
  try {
    await requireAuth({
      action: "save_work",
      route: "/pages/user/UserProfileEditPage",
      payload: { source: "profile_edit" },
      resume: load,
    });
  } finally {
    requestingLogin = false;
  }
}

async function save() {
  const name = form.name.trim();
  const email = form.email.trim().toLowerCase();
  if (!name) return void uni.showToast({ title: "请输入显示名称", icon: "none" });
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) return void uni.showToast({ title: "请输入正确邮箱", icon: "none" });
  saving.value = true;
  try {
    await api("/api/v1/member/profile", { method: "PATCH", body: JSON.stringify({ name, email }) });
    uni.showToast({ title: "资料已保存", icon: "success" });
    setTimeout(() => backOrHome("/pages/user/UserMinePage"), 400);
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "保存失败", icon: "none" });
  } finally {
    saving.value = false;
  }
}

onShow(() => { void loadWithAuthGate(); });
</script>

<style>@import "../../styles/mini-program-business.css";</style>
