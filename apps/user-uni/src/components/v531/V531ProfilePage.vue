<template>
  <view class="profile-v55-page">
    <view v-if="!isGuest && selectableRoles.length > 1" class="profile-v55-role-switcher" aria-label="角色切换">
      <button
        v-for="role in selectableRoles"
        :key="role.id"
        type="button"
        :class="['profile-v55-role-pill', { active: role.id === currentRole }]"
        :aria-label="`切换到${role.label}`"
        hover-class="profile-v55-pressed"
        @click="handleRoleSwitch(role.id)"
      >
        <text>{{ role.label }}</text>
      </button>
    </view>

    <view :class="['profile-v55-role-card', { compact: hideCommerceSummary }]">
      <button class="profile-v55-role-main" type="button" hover-class="profile-v55-pressed" @click="isGuest ? $emit('service', 'login') : $emit('edit')">
        <RemoteCover
          class="profile-v55-avatar"
          page-code="profile"
          slot-key="profile.avatar"
          :src="avatarUrl"
          :fallback="avatarFallback"
          local-fallback="/static/fallbacks/default-avatar.jpg"
          :alt="displayName"
          width="54px"
          height="54px"
          radius="50%"
          :lazy-load="false"
        />
        <view class="profile-v55-role-copy">
          <text class="profile-v55-name">{{ displayName }}</text>
          <text class="profile-v55-meta">{{ roleMeta }}</text>
          <text v-if="!hideCommerceSummary" class="profile-v55-points">{{ isGuest ? '--' : formatNumber(pointBalance) }} 点</text>
        </view>
      </button>
      <button v-if="!hideAgentCenter && !hideCommerceSummary"
        class="profile-v55-primary-cta"
        type="button"
        hover-class="profile-v55-pressed"
        @click="handleAgentCta()"
      >
        {{ hasAgentRole ? "代理商工作台" : "成为代理商" }}
      </button>
    </view>

    <slot name="commerce" />

    <view v-if="!hideRecharge && !hideCommerceSummary" class="profile-v55-section-head">
      <text class="profile-v55-section-title">我的 AI</text>
      <button type="button" class="profile-v55-section-link" @click="$emit('service', 'membership')">升级套餐 ›</button>
    </view>
    <view v-if="!hideRecharge && !hideCommerceSummary" class="profile-v55-capability-grid">
      <button
        v-for="item in aiCapabilities"
        :key="item.id"
        type="button"
        class="profile-v55-tile"
        hover-class="profile-v55-pressed"
        @click="$emit('service', item.id)"
      >
        <text class="profile-v55-tile-title">{{ item.label }}</text>
        <text :class="['profile-v55-tile-state', item.tone]">{{ item.status }}</text>
      </button>
    </view>

    <view v-if="!hideWallet && !hideCommerceSummary" class="profile-v55-section-head">
      <text class="profile-v55-section-title">钱包摘要</text>
      <button type="button" class="profile-v55-section-link" @click="$emit('service', 'wallet')">钱包中心 ›</button>
    </view>
    <view v-if="!hideWallet && !hideCommerceSummary" class="profile-v55-wallet-card">
      <button class="profile-v55-wallet-main" type="button" hover-class="profile-v55-pressed" @click="$emit('service', 'wallet')">
        <text class="profile-v55-wallet-label">点数余额</text>
        <text class="profile-v55-wallet-value">{{ isGuest ? '--' : formatNumber(pointBalance) }}</text>
        <text class="profile-v55-wallet-note">本月消耗 {{ formatNumber(monthlyPointCost) }} 点</text>
      </button>
      <button class="profile-v55-wallet-cta" type="button" hover-class="profile-v55-pressed" @click="$emit('recharge')">充值</button>
    </view>

    <view class="profile-v55-section-head">
      <text class="profile-v55-section-title">企业中心</text>
      <button type="button" class="profile-v55-section-link" @click="$emit('service', 'company')">进入 ›</button>
    </view>
    <button class="profile-v55-enterprise-card" type="button" hover-class="profile-v55-pressed" @click="$emit('service', 'company')">
      <view>
        <text class="profile-v55-enterprise-title">{{ enterpriseTitle }}</text>
        <text class="profile-v55-enterprise-copy">{{ enterpriseCopy }}</text>
      </view>
      <image class="profile-v55-chevron" src="/static/icons/chevron-right.svg" mode="aspectFit" />
    </button>

    <view class="profile-v55-section-head">
      <text class="profile-v55-section-title">我的工作台</text>
    </view>
    <view class="profile-v55-work-grid">
      <button
        v-for="item in workbenchItems"
        :key="item.id"
        type="button"
        class="profile-v55-work-item"
        hover-class="profile-v55-pressed"
        @click="$emit('service', item.id)"
      >
        <text class="profile-v55-work-icon">{{ item.icon }}</text>
        <text class="profile-v55-work-label">{{ item.label }}</text>
      </button>
    </view>

    <view class="profile-v55-section-head">
      <text class="profile-v55-section-title">我的数据</text>
      <button type="button" class="profile-v55-section-link" @click="$emit('service', 'usage')">查看明细 ›</button>
    </view>
    <view class="profile-v55-data-card">
      <button
        v-for="item in dataMetrics"
        :key="item.label"
        type="button"
        class="profile-v55-data-item"
        hover-class="profile-v55-pressed"
        @click="$emit('service', item.service)"
      >
        <text class="profile-v55-data-value">{{ formatNumber(item.value) }}</text>
        <text class="profile-v55-data-label">{{ item.label }}</text>
      </button>
    </view>

    <view class="profile-v55-section-head">
      <text class="profile-v55-section-title">常用服务</text>
    </view>
    <view class="profile-v55-service-grid">
      <button
        v-for="item in commonServices"
        :key="item.id"
        type="button"
        class="profile-v55-service-item"
        hover-class="profile-v55-pressed"
        @click="$emit('service', item.id)"
      >
        <text :class="['profile-v55-service-icon', item.tone]">{{ item.icon }}</text>
        <text class="profile-v55-service-label">{{ item.label }}</text>
      </button>
    </view>

    <view class="profile-v55-section-head">
      <text class="profile-v55-section-title">角色功能</text>
    </view>
    <view class="profile-v55-role-grid">
      <button
        v-for="item in roleFunctions"
        :key="item.id"
        type="button"
        :class="['profile-v55-role-action', { primary: item.primary }]"
        hover-class="profile-v55-pressed"
        @click="$emit('service', item.id)"
      >
        <text>{{ item.label }}</text>
      </button>
    </view>

    <slot name="extra" />

    <view class="profile-v55-settings-card">
      <button type="button" class="profile-v55-settings-row" hover-class="profile-v55-pressed" @click="$emit('service', 'settings')">
        <text class="profile-v55-settings-icon">设</text>
        <text class="profile-v55-settings-label">设置中心</text>
        <image class="profile-v55-chevron" src="/static/icons/chevron-right.svg" mode="aspectFit" />
      </button>
    </view>
    <view
      class="profile-v55-logout"
      role="button"
      aria-label="退出登录"
      hover-class="profile-v55-pressed"
      @tap.stop="handleLogout()"
    >
      <text>{{ isGuest ? guestLoginLabel : logoutLabel }}</text>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed } from "vue";
import RemoteCover from "../RemoteCover.vue";
import { RoleMenuConfig, roleLabels } from "../../config/permissions";
import type { AppRole } from "../../types";
import { useAuthStore } from "../../stores/auth";
import { useUserStore } from "../../stores/user";
import { reviewModeHides } from "../../features/reviewMode";

const props = withDefaults(
  defineProps<{
    displayName: string;
    userId: string;
    roles?: AppRole[];
    currentRole?: AppRole;
    permissions?: string[];
    companyName: string;
    planName?: string;
    subscriptionExpiresAt?: string;
    pointBalance: number;
    monthlyPointCost?: number;
    monthlyGrantedPoints?: number;
    creationCount?: number;
    imageCount?: number;
    videoCount?: number;
    pptCount?: number;
    avatarUrl?: string;
    avatarFallback?: string;
    isGuest?: boolean;
		hideCommerceSummary?: boolean;
  }>(),
  {
    displayName: "当前用户",
    userId: "--",
    roles: () => ["USER"],
    currentRole: "USER",
    permissions: () => [],
    avatarUrl: "",
    avatarFallback: "",
    isGuest: false,
		hideCommerceSummary: false,
    companyName: "企业信息待完善",
    planName: "",
    subscriptionExpiresAt: "",
    pointBalance: 0,
    monthlyPointCost: 0,
    monthlyGrantedPoints: 0,
    creationCount: 0,
    imageCount: 0,
    videoCount: 0,
    pptCount: 0,
  },
);

const emit = defineEmits<{
  upgrade: [];
  edit: [];
  recharge: [];
  service: [id: string];
  benefit: [id: string];
  "role-change": [role: AppRole];
}>();

const authStore = useAuthStore();
const userStore = useUserStore();

async function handleRoleSwitch(role: AppRole) {
  try {
    await userStore.switchRole(role);
    if (role === "USER") {
      uni.switchTab({
        url: "/pages/user/UserMinePage",
        fail: () => uni.reLaunch({ url: "/pages/user/UserMinePage" }),
      });
      return;
    }
    uni.reLaunch({
      url: role === "AGENT"
        ? "/pages/agent/AgentOverviewPage"
        : "/pages/operation/OperationOverviewPage",
    });
  } catch (error) {
    emit("role-change", role);
    uni.showToast({
      title: error instanceof Error ? error.message : "角色切换失败",
      icon: "none",
    });
  }
}

function handleAgentCta() {
  if (hasAgentRole.value) handleRoleSwitch("AGENT");
  else emit("upgrade");
}

function handleLogout() {
  if (props.isGuest) {
    emit("service", "login");
    return;
  }
  uni.showModal({
    title: "退出登录",
    content: "退出后需要重新登录才能继续使用知启云 AI。",
    confirmText: "退出",
    confirmColor: "#D64545",
    success: result => {
      if (!result.confirm) return;
      authStore.logout();
      uni.removeStorageSync("xianzhiMiniProgramAuth");
      uni.switchTab({ url: "/pages/user/UserHomePage" });
    },
  });
}

const supportedPageRoles: AppRole[] = ["USER", "AGENT", "OPERATION"];
const selectableRoles = computed(() => props.roles
  .filter(role => !(role === "AGENT" && reviewModeHides("hideAgentCenter")))
  .filter(role => !(role === "OPERATION" && reviewModeHides("hideOperatorCenter")))
  .filter((role, index, roles) => supportedPageRoles.includes(role) && roles.indexOf(role) === index)
  .map(role => ({ id: role, label: roleLabels[role] })));
const hasAgentRole = computed(() => props.roles.includes("AGENT"));
const grantedPermissions = computed(() => new Set(props.permissions));
const hasPermission = (permission?: string) => !permission || grantedPermissions.value.has(permission);

const aiCapabilities = [
  { id: "ai-image", label: "AI生图", status: "已开通", tone: "success" },
  { id: "ai-video", label: "AI视频", status: "已开通", tone: "success" },
  { id: "ai-ppt", label: "PPT", status: "已开通", tone: "success" },
  { id: "ai-agent", label: "AI Agent", status: "已开通", tone: "success" },
  { id: "ai-knowledge", label: "知识库", status: "试用", tone: "trial" },
  { id: "ai-infographic", label: "自由P图", status: "试用", tone: "trial" },
];

const workbenchItems = [
  { id: "projects", label: "我的项目", icon: "项" },
  { id: "assets", label: "AI资产", icon: "资" },
  { id: "recent", label: "最近创作", icon: "近" },
  { id: "tasks", label: "最近任务", icon: "任" },
  { id: "favorites", label: "收藏", icon: "藏" },
  { id: "downloads", label: "下载中心", icon: "下" },
];

const commonServices = [
  { id: "messages", label: "消息中心", icon: "消", tone: "purple" },
  { id: "knowledge", label: "知识库", icon: "知", tone: "blue" },
  { id: "ai-employees", label: "AI员工", icon: "AI", tone: "green" },
  { id: "customer-service", label: "联系客服", icon: "客", tone: "orange" },
  { id: "help", label: "帮助中心", icon: "?", tone: "violet" },
  { id: "feedback", label: "意见反馈", icon: "写", tone: "slate" },
];

const roleFunctions = computed(() => RoleMenuConfig[props.currentRole]
  .filter(item => hasPermission(item.permission))
  .filter(item => !(item.id === "wallet" && reviewModeHides("hideWallet")))
  .filter(item => !(item.id === "upgrade-agent" && reviewModeHides("hideAgentCenter")))
  .filter(item => !(item.id.includes("commission") && reviewModeHides("hideCommission")))
  .map(item => item.id === "upgrade-agent" && hasAgentRole.value
    ? { ...item, label: "代理商工作台" }
    : item));

const roleMeta = computed(() => {
  const company = String(props.companyName || "").trim();
  const plan = String(props.planName || "").trim();
  return [roleLabels[props.currentRole], company && company !== "企业信息待完善" ? company : plan || "尚未加入企业"]
    .filter(Boolean)
    .join(" · ");
});

const enterpriseTitle = computed(() => {
  const company = String(props.companyName || "").trim();
  return company && company !== "企业信息待完善" ? company : "尚未加入企业";
});

const enterpriseCopy = computed(() => {
  const company = String(props.companyName || "").trim();
  return company && company !== "企业信息待完善"
    ? "企业认证、成员与团队管理"
    : "创建或加入企业后，可以使用企业知识库、共享智能体、成员协作和统一算力管理。";
});

const dataMetrics = computed(() => [
  { label: "创作", value: props.creationCount, service: "tasks" },
  { label: "图片", value: props.imageCount, service: "assets" },
  { label: "视频", value: props.videoCount, service: "assets" },
  { label: "PPT", value: props.pptCount, service: "assets" },
]);

function formatNumber(value: unknown) {
  const numberValue = Number(value);
  return Number.isFinite(numberValue) ? Math.max(0, numberValue).toLocaleString("zh-CN") : "0";
}
const hideRecharge = computed(() => reviewModeHides("hideRecharge"));
const hideWallet = computed(() => reviewModeHides("hideWallet"));
const hideAgentCenter = computed(() => reviewModeHides("hideAgentCenter"));
const guestLoginLabel = "\u7acb\u5373\u767b\u5f55";
const logoutLabel = "\u9000\u51fa\u767b\u5f55";
</script>

<style scoped>
.profile-v55-page {
  min-height: 100vh;
  padding: 0 16px calc(110px + env(safe-area-inset-bottom));
  box-sizing: border-box;
  color: var(--color-text-primary, #171c29);
  background: var(--color-bg-page, #f7f8fc);
  font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", "Noto Sans SC", sans-serif;
}

.profile-v55-page button {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
  border: 0;
  line-height: normal;
}

.profile-v55-page button::after { display: none; }
.profile-v55-pressed { opacity: .78; transform: scale(.985); }

.profile-v55-section-head,
.profile-v55-wallet-card,
.profile-v55-enterprise-card,
.profile-v55-settings-row {
  display: flex;
  align-items: center;
}

.profile-v55-role-switcher {
  display: grid;
  height: 42px;
  margin-top: 10px;
  padding: 6px 5px;
  box-sizing: border-box;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 9px;
  border-radius: 14px;
  background: var(--color-bg-card, #fff);
}

.profile-v55-role-pill {
  display: flex;
  width: 100%;
  min-width: 0;
  height: 30px;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  color: #636b80;
  background: #f2f2f7;
  font-size: 12px;
  font-weight: 600;
  line-height: 1;
  text-align: center;
}

.profile-v55-role-pill.active { color: #fff; background: var(--color-primary, #7d8df6); }
.profile-v55-role-pill.locked { color: #8b91a1; }
.profile-v55-role-pill text { width: 100%; line-height: 30px; text-align: center; }

.profile-v55-role-card {
  position: relative;
  min-height: 150px;
  margin-top: 12px;
  padding: 20px 16px;
  box-sizing: border-box;
  overflow: hidden;
  border: 1px solid rgba(226, 229, 239, .92);
  border-radius: 18px;
  background: var(--color-bg-card, #fff);
  box-shadow: 0 10px 28px rgba(56, 63, 92, .06);
}

.profile-v55-role-card.compact {
  min-height: 0;
  padding: 16px;
}

.profile-v55-role-card::before {
  position: absolute;
  top: -54px;
  right: -34px;
  width: 168px;
  height: 150px;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(125, 141, 246, .17), transparent 70%);
  content: "";
}

.profile-v55-role-main {
  position: relative;
  z-index: 1;
  display: flex;
  width: calc(100% - 108px);
  min-height: 92px;
  align-items: flex-start;
  gap: 14px;
  color: inherit;
  background: transparent;
  text-align: left;
}

.profile-v55-role-card.compact .profile-v55-role-main {
  width: 100%;
  min-height: 54px;
  align-items: center;
}

.profile-v55-avatar { flex: 0 0 54px; width: 54px !important; height: 54px !important; }
.profile-v55-role-copy { min-width: 0; flex: 1; }
.profile-v55-role-copy text { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.profile-v55-name { font-size: 18px; font-weight: 700; line-height: 24px; }
.profile-v55-meta { margin-top: 5px; color: #636b80; font-size: 11px; line-height: 16px; }
.profile-v55-points { margin-top: 8px; color: var(--color-primary-dark, #5a4db2); font-size: 18px; font-weight: 700; line-height: 24px; }

.profile-v55-primary-cta,
.profile-v55-wallet-cta {
  position: absolute;
  z-index: 2;
  top: 22px;
  right: 16px;
  display: flex;
  width: auto;
  min-width: 86px;
  height: 32px;
  padding: 0 12px !important;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  color: #fff;
  background: var(--color-accent, #ff771b);
  font-size: 11px;
  font-weight: 700;
  line-height: 1;
  text-align: center;
  white-space: nowrap;
}

.profile-v55-section-head { min-height: 24px; margin-top: 16px; justify-content: space-between; }
.profile-v55-section-title { font-size: 16px; font-weight: 700; line-height: 22px; }
.profile-v55-section-link { display: flex; width: auto; height: 24px; align-items: center; justify-content: flex-end; color: var(--color-primary-dark, #5a4db2); background: transparent; font-size: 11px; font-weight: 600; line-height: 24px; text-align: right; }

.profile-v55-capability-grid,
.profile-v55-work-grid,
.profile-v55-service-grid,
.profile-v55-role-grid {
  display: grid;
  margin-top: 10px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.profile-v55-tile,
.profile-v55-work-item,
.profile-v55-service-item {
  width: 100%;
  min-width: 0;
  border: 1px solid #edf0f7;
  border-radius: 14px;
  color: inherit;
  background: var(--color-bg-card, #fff);
  text-align: left;
  box-shadow: 0 6px 18px rgba(56, 63, 92, .035);
}

.profile-v55-tile { display: flex; height: 68px; padding: 12px 14px !important; flex-direction: column; align-items: flex-start; justify-content: center; }
.profile-v55-tile text { display: block; }
.profile-v55-tile-title { font-size: 12px; font-weight: 700; line-height: 17px; }
.profile-v55-tile-state { margin-top: 7px; font-size: 9px; font-weight: 700; }
.profile-v55-tile-state.success { color: #29a36e; }
.profile-v55-tile-state.trial { color: var(--color-accent, #ff771b); }

.profile-v55-wallet-card {
  position: relative;
  min-height: 96px;
  margin-top: 10px;
  padding: 14px 16px;
  box-sizing: border-box;
  border: 1px solid #edf0f7;
  border-radius: 16px;
  background: var(--color-bg-card, #fff);
  box-shadow: 0 8px 22px rgba(56, 63, 92, .045);
}

.profile-v55-wallet-main { display: flex; width: calc(100% - 92px); min-height: 68px; flex-direction: column; align-items: flex-start; justify-content: center; color: inherit; background: transparent; text-align: left; }
.profile-v55-wallet-main text { display: block; }
.profile-v55-wallet-label { color: #636b80; font-size: 11px; line-height: 16px; }
.profile-v55-wallet-value { margin-top: 3px; font-size: 26px; font-weight: 700; line-height: 32px; }
.profile-v55-wallet-note { margin-top: 2px; color: #8b91a1; font-size: 9px; line-height: 13px; }
.profile-v55-wallet-cta { top: 31px; min-width: 58px; }

.profile-v55-enterprise-card {
  width: 100%;
  min-height: 78px;
  margin-top: 10px !important;
  padding: 14px 16px !important;
  justify-content: space-between;
  gap: 12px;
  border: 1px solid #edf0f7 !important;
  border-radius: 16px;
  color: inherit;
  background: var(--color-bg-card, #fff);
  text-align: left;
}

.profile-v55-enterprise-card view { min-width: 0; flex: 1; }
.profile-v55-enterprise-card text { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.profile-v55-enterprise-title { font-size: 13px; font-weight: 700; line-height: 19px; }
.profile-v55-enterprise-copy { margin-top: 7px; color: #636b80; font-size: 10px; line-height: 15px; }
.profile-v55-chevron { width: 16px; height: 16px; opacity: .48; }

.profile-v55-work-item { display: flex; height: 66px; padding: 0 12px !important; align-items: center; gap: 8px; }
.profile-v55-work-icon { display: grid; width: 24px; min-width: 24px; height: 24px; place-items: center; border-radius: 8px; color: var(--color-primary-dark, #5a4db2); background: #f0f1ff; font-size: 10px; font-weight: 700; }
.profile-v55-work-label { min-width: 0; overflow: hidden; font-size: 11px; font-weight: 700; text-overflow: ellipsis; white-space: nowrap; }

.profile-v55-data-card {
  display: grid;
  min-height: 88px;
  margin-top: 10px;
  padding: 14px 8px;
  box-sizing: border-box;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  border: 1px solid #edf0f7;
  border-radius: 16px;
  background: var(--color-bg-card, #fff);
}

.profile-v55-data-item { display: grid; min-width: 0; place-content: center; gap: 6px; color: inherit; background: transparent; text-align: center; }
.profile-v55-data-value { overflow: hidden; font-size: 16px; font-weight: 700; line-height: 22px; text-overflow: ellipsis; white-space: nowrap; }
.profile-v55-data-label { color: #636b80; font-size: 9px; line-height: 13px; }

.profile-v55-service-item { display: grid; height: 62px; padding: 8px 10px !important; grid-template-columns: 28px minmax(0, 1fr); align-items: center; gap: 8px; }
.profile-v55-service-icon { display: grid; width: 28px; height: 28px; place-items: center; border-radius: 9px; font-size: 10px; font-weight: 700; }
.profile-v55-service-icon.purple { color: #5a4db2; background: #f0edff; }
.profile-v55-service-icon.blue { color: #3976d8; background: #edf5ff; }
.profile-v55-service-icon.green { color: #188b62; background: #eaf9f3; }
.profile-v55-service-icon.orange { color: #d9610f; background: #fff2e8; }
.profile-v55-service-icon.violet { color: #7c4ec7; background: #f5efff; }
.profile-v55-service-icon.slate { color: #5f6b7a; background: #eef2f6; }
.profile-v55-service-label { min-width: 0; overflow: hidden; font-size: 10px; font-weight: 700; text-overflow: ellipsis; white-space: nowrap; }

.profile-v55-role-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.profile-v55-role-action { display: flex; width: 100%; height: 58px; align-items: center; justify-content: center; border: 1px solid #edf0f7 !important; border-radius: 14px; color: #293142; background: var(--color-bg-card, #fff); font-size: 12px; font-weight: 700; line-height: 1; text-align: center; }
.profile-v55-role-action.primary { color: var(--color-accent, #ff771b); border-color: #ffe1cc !important; background: #fff8f2; }
.profile-v55-role-action text { width: 100%; line-height: 58px; text-align: center; }

.profile-v55-settings-card { margin-top: 16px; overflow: hidden; border: 1px solid #edf0f7; border-radius: 16px; background: var(--color-bg-card, #fff); }
.profile-v55-settings-row { width: 100%; min-height: 64px; padding: 0 16px !important; gap: 12px; color: inherit; background: transparent; text-align: left; }
.profile-v55-settings-icon { display: grid; width: 30px; min-width: 30px; height: 30px; place-items: center; border-radius: 9px; color: #5f6b7a; background: #eef2f6; font-size: 11px; font-weight: 700; }
.profile-v55-settings-label { min-width: 0; flex: 1; font-size: 13px; font-weight: 700; }
.profile-v55-logout { display: flex; width: 100%; height: 48px; margin-top: 12px !important; align-items: center; justify-content: center; border-radius: 14px; color: #d64545; background: #fff7f7; font-size: 12px; font-weight: 700; line-height: 48px; text-align: center; }

@media (max-width: 340px) {
  .profile-v55-capability-grid,
  .profile-v55-work-grid,
  .profile-v55-service-grid { gap: 7px; }
  .profile-v55-service-item { padding: 8px 6px !important; gap: 5px; }
  .profile-v55-primary-cta { min-width: 76px; padding: 0 9px !important; font-size: 10px; }
}
</style>
