<template>
  <view class="user-mine-page">
    <V531ProfilePage
      :display-name="displayName"
      :user-id="displayUserId"
      :roles="userStore.roles"
      current-role="USER"
      :permissions="userPermissions"
      :company-name="companyName"
      :plan-name="planName"
      :subscription-expires-at="subscriptionExpiresAt"
      :point-balance="pointBalance"
      :monthly-point-cost="monthlyPointCost"
      :monthly-granted-points="monthlyGrantedPoints"
      :creation-count="generationTasks.length"
      :image-count="imageAssetCount"
      :video-count="videoAssetCount"
      :ppt-count="pptAssetCount"
      :avatar-url="avatarUrl"
      :avatar-fallback="avatarFallback"
      @upgrade="openPage(miniProgramMinePages['agent-upgrade'])"
      @edit="openPage(miniProgramFeaturePages.userProfileEdit)"
      @recharge="openPage(miniProgramFeaturePages.userRechargePlans)"
      @role-change="handleRoleChange($event)"
      @service="handleService($event)"
      @benefit="handleBenefit($event)"
    />

    <view v-if="loadError" class="mine-load-note" role="alert">
      <text>{{ loadError }}</text>
      <button type="button" @click="refreshProfile()">重新加载</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { onPullDownRefresh, onShow } from "@dcloudio/uni-app";
import type { MemberProfileResponse, RoleWalletResponse } from "@xianzhi/business-sdk";
import V531ProfilePage from "../../components/v531/V531ProfilePage.vue";
import { authStorage, businessSdk, setAuthToken } from "../../api/client";
import {
  miniProgramCreationPages,
  miniProgramEnterprisePages,
  miniProgramFeaturePages,
  miniProgramMinePages,
  rolePage,
} from "../../config/miniProgramPages";
import { isAppRole, permissionsForRole, roleLabels } from "../../config/permissions";
import { usePageConfigStore } from "../../stores/pageConfig";
import { useUserStore } from "../../stores/user";
import type { AppRole, Asset, AuthResponse, GenerationTask } from "../../types";
import { syncCustomTabBar } from "../../utils/customTabBar";

type AnyRecord = Record<string, unknown>;

const userStore = useUserStore();
const pageConfigStore = usePageConfigStore();
const auth = ref<AuthResponse | null>(authStorage.getAuth());
const profile = ref<MemberProfileResponse | null>(null);
const wallet = ref<RoleWalletResponse | null>(null);
const points = ref<RoleWalletResponse | null>(null);
const recentAssets = ref<Asset[]>([]);
const generationTasks = ref<GenerationTask[]>([]);
const loading = ref(false);
const loadError = ref("");

const userPermissions = computed(() => userStore.currentRole === "USER"
  ? userStore.permissions
  : permissionsForRole("USER"));
const displayName = computed(() => profile.value?.user?.name
  || auth.value?.user?.name
  || profile.value?.user?.email
  || auth.value?.user?.email
  || "当前用户");
const displayUserId = computed(() => rowString(profile.value?.user, "id", "userId")
  || rowString(auth.value?.user, "id", "userId")
  || "--");
const avatarUrl = computed(() => rowString(profile.value?.user, "avatarUrl", "avatar", "headImage")
  || rowString(auth.value?.user, "avatarUrl", "avatar", "headImage"));
const avatarFallback = computed(() => {
  const slot = pageConfigStore.slot("profile", "profile.avatar");
  return slot?.imageUrl || slot?.fallbackUrl || "";
});
const planName = computed(() => rowString(profile.value?.plan, "name", "planName")
  || auth.value?.defaultModule
  || "AI 创作用户");
const companyName = computed(() => rowString(profile.value?.user, "companyName", "company", "organization", "tenantName")
  || rowString(auth.value?.user, "companyName", "company", "organization", "tenantName")
  || rowString(profile.value?.operationCenter, "companyName", "name", "tenantName")
  || rowString(profile.value?.plan, "companyName", "tenantName")
  || "企业信息待完善");
const subscriptionExpiresAt = computed(() => rowString(profile.value?.user, "subscriptionExpiresAt", "expiresAt", "validUntil")
  || rowString(auth.value?.user, "subscriptionExpiresAt", "expiresAt", "validUntil")
  || rowString(profile.value?.plan, "expiresAt", "validUntil", "endedAt"));
const pointAccount = computed(() => wallet.value?.account || points.value?.account || profile.value?.account || null);
const pointBalance = computed(() => asNumber(pointAccount.value?.available));
const walletRecords = computed(() => [
  ...recordList(wallet.value?.transactions),
  ...recordList(points.value?.transactions),
  ...recordList(wallet.value?.tokenRecords),
]);
const monthlyPointCost = computed(() => walletRecords.value
  .filter(item => isCurrentMonth(rowDate(item)))
  .reduce((total, item) => {
    const delta = rowNumber(item, "delta");
    const cost = rowNumber(item, "pointCost") || rowNumber(item, "points") || rowNumber(item, "amount");
    return total + (delta < 0 ? Math.abs(delta) : Math.max(0, cost));
  }, 0));
const monthlyGrantedPoints = computed(() => walletRecords.value.reduce((total, item) => {
  if (!isCurrentMonth(rowDate(item))) return total;
  const changeType = rowString(item, "changeType", "type").toUpperCase();
  if (!changeType.includes("GRANT") && !changeType.includes("BONUS") && !changeType.includes("GIFT")) return total;
  return total + Math.abs(rowNumber(item, "amount") || rowNumber(item, "points") || rowNumber(item, "delta"));
}, 0));
const imageAssetCount = computed(() => recentAssets.value.filter(item => {
  const type = rowString(item, "mediaType", "type", "assetType").toLowerCase();
  return !type || type.includes("image");
}).length);
const videoAssetCount = computed(() => recentAssets.value.filter(item => rowString(item, "mediaType", "type", "assetType").toLowerCase().includes("video")).length);
const pptAssetCount = computed(() => recentAssets.value.filter(item => {
  const type = rowString(item, "mediaType", "type", "assetType").toLowerCase();
  return type.includes("ppt") || type.includes("document") || type.includes("presentation");
}).length);

function asNumber(value: unknown) {
  const numberValue = Number(value);
  return Number.isFinite(numberValue) ? numberValue : 0;
}

function rowString(row: unknown, ...keys: string[]) {
  if (!row || typeof row !== "object") return "";
  for (const key of keys) {
    const value = (row as AnyRecord)[key];
    if (typeof value === "string" && value.trim()) return value.trim();
  }
  return "";
}

function rowNumber(row: unknown, key: string) {
  return row && typeof row === "object" ? asNumber((row as AnyRecord)[key]) : 0;
}

function rowDate(row: unknown) {
  return rowString(row, "createdAt", "occurredAt", "updatedAt", "paidAt");
}

function isCurrentMonth(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return false;
  const now = new Date();
  return date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth();
}

function recordList(value: unknown): AnyRecord[] {
  return Array.isArray(value)
    ? value.filter((item): item is AnyRecord => Boolean(item) && typeof item === "object")
    : [];
}

function collectionOf<T>(value: unknown): T[] {
  if (Array.isArray(value)) return value as T[];
  if (!value || typeof value !== "object") return [];
  const record = value as AnyRecord;
  for (const key of ["items", "rows", "data"] as const) {
    if (Array.isArray(record[key])) return record[key] as T[];
  }
  return [];
}

function emittedValue(payload: unknown): string {
  if (typeof payload === "string") return payload;
  if (Array.isArray(payload)) {
    for (const item of payload) {
      const value = emittedValue(item);
      if (value) return value;
    }
    return "";
  }
  if (!payload || typeof payload !== "object") return "";
  const record = payload as AnyRecord;
  const directValue = rowString(record, "serviceId", "service-id", "id", "role");
  if (directValue) return directValue;
  const argsValue = emittedValue(record.__args__);
  if (argsValue) return argsValue;
  const detailValue = emittedValue(record.detail);
  if (detailValue) return detailValue;
  const currentTarget = record.currentTarget as AnyRecord | undefined;
  const target = record.target as AnyRecord | undefined;
  return emittedValue(currentTarget?.dataset) || emittedValue(target?.dataset);
}

function openPage(url: string) {
  uni.navigateTo({
    url,
    fail: () => uni.redirectTo({
      url,
      fail: () => uni.reLaunch({ url }),
    }),
  });
}

function openUserTab(url: string) {
  uni.switchTab({ url, fail: () => uni.reLaunch({ url }) });
}

async function handleRoleChange(payload: unknown) {
  const roleValue = emittedValue(payload);
  if (!isAppRole(roleValue)) {
    uni.showToast({ title: "角色切换参数无效", icon: "none" });
    return;
  }
  const role: AppRole = roleValue;
  if (!userStore.roles.includes(role)) {
    uni.showToast({ title: `当前账号未开通${roleLabels[role]}`, icon: "none" });
    return;
  }
  try {
    await userStore.switchRole(role);
    if (role === "USER") openUserTab(rolePage("user", "mine"));
    else if (role === "AGENT") uni.reLaunch({ url: rolePage("agent", "overview") });
    else if (role === "OPERATION") uni.reLaunch({ url: rolePage("operation", "overview") });
    else uni.showToast({ title: `${roleLabels[role]}工作台即将开放`, icon: "none" });
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "角色切换失败", icon: "none" });
  }
}

function showCompany() {
  const hasCompany = companyName.value !== "企业信息待完善";
  uni.showModal({
    title: hasCompany ? "企业中心" : "尚未加入企业",
    content: hasCompany
      ? `当前企业：${companyName.value}`
      : "创建或加入企业后，可以使用企业知识库、共享智能体、成员协作和统一算力管理。",
    showCancel: false,
  });
}

function showHelp() {
  uni.showModal({
    title: "帮助与客服",
    content: "如遇账号、点数或创作问题，请联系知启云 AI 客服处理。",
    showCancel: false,
  });
}

function confirmLogout() {
  uni.showModal({
    title: "退出登录",
    content: "退出后需要重新登录才能继续使用知启云 AI。",
    confirmText: "退出",
    confirmColor: "#D64545",
    success: result => {
      if (!result.confirm) return;
      authStorage.clear();
      userStore.reset();
      uni.removeStorageSync("xianzhiMiniProgramAuth");
      uni.reLaunch({ url: "/pages/WechatLoginPage" });
    },
  });
}

function handleService(payload: unknown) {
  const id = emittedValue(payload);
  const actions: Record<string, () => void> = {
    ai: () => openUserTab(rolePage("user", "create")),
    recharge: () => openPage(miniProgramFeaturePages.userRechargePlans),
    membership: () => openPage(miniProgramFeaturePages.userRechargePlans),
    wallet: () => openPage(rolePage("user", "wallet")),
    assets: () => openUserTab(rolePage("user", "assets")),
    recent: () => openUserTab(rolePage("user", "assets")),
    projects: () => openPage(`${miniProgramFeaturePages.userAssetsList}?view=projects`),
    tasks: () => openPage(miniProgramFeaturePages.userTasksList),
    favorites: () => openPage(`${miniProgramFeaturePages.userAssetsList}?filter=favorite`),
    downloads: () => openPage(`${miniProgramFeaturePages.userAssetsList}?filter=download`),
    points: () => openPage(miniProgramMinePages["usage-details"]),
    usage: () => openPage(miniProgramMinePages["usage-details"]),
    orders: () => openPage(miniProgramFeaturePages.userOrders),
    invoices: () => openPage(miniProgramFeaturePages.userInvoices),
    invite: () => openPage(miniProgramMinePages["invite-promotion"]),
    roles: () => openPage(miniProgramMinePages["role-permissions"]),
    team: () => openPage(miniProgramMinePages["role-permissions"]),
    company: () => openPage(miniProgramEnterprisePages.entry),
    "enterprise-overview": () => openPage(miniProgramEnterprisePages.overview),
    "enterprise-members": () => openPage(miniProgramEnterprisePages.members),
    "enterprise-organizations": () => openPage(miniProgramEnterprisePages.organizations),
    "enterprise-ai-employees": () => openPage(miniProgramEnterprisePages.aiEmployees),
    "enterprise-billing": () => openPage(miniProgramEnterprisePages.billing),
    "enterprise-roles": () => openPage(miniProgramEnterprisePages.roles),
    "enterprise-settings": () => openPage(miniProgramEnterprisePages.settings),
    messages: () => uni.showToast({ title: "暂无新消息", icon: "none" }),
    knowledge: () => openPage(miniProgramCreationPages.agent),
    "ai-employees": () => openPage(miniProgramCreationPages.agent),
    "customer-service": showHelp,
    help: showHelp,
    feedback: showHelp,
    "ai-image": () => openPage(miniProgramCreationPages.image),
    "ai-video": () => openPage(miniProgramCreationPages.video),
    "ai-ppt": () => openPage(miniProgramCreationPages.ppt),
    "ai-agent": () => openPage(miniProgramCreationPages.agent),
    "ai-knowledge": () => openPage(miniProgramCreationPages.agent),
    "ai-infographic": () => openPage(miniProgramCreationPages.infographic),
    "upgrade-agent": () => userStore.roles.includes("AGENT")
      ? void handleRoleChange("AGENT")
      : openPage(miniProgramMinePages["agent-upgrade"]),
    coupons: () => uni.showToast({ title: "暂无可用优惠券", icon: "none" }),
    settings: () => openPage(miniProgramFeaturePages.userSettings),
    logout: confirmLogout,
  };
  const action = actions[id];
  if (action) action();
  else uni.showToast({ title: "服务入口即将开放", icon: "none" });
}

function handleBenefit(payload: unknown) {
  const id = emittedValue(payload);
  if (id === "member") openPage(miniProgramFeaturePages.userRechargePlans);
  else showCompany();
}

async function refreshProfile() {
  if (loading.value) return;
  const token = authStorage.getToken() || String(uni.getStorageSync("token") || "");
  if (!token) {
    uni.reLaunch({ url: "/pages/WechatLoginPage" });
    return;
  }

  loading.value = true;
  loadError.value = "";
  setAuthToken(token);
  const legacyAuth = uni.getStorageSync("xianzhiMiniProgramAuth") as AuthResponse | "";
  auth.value = authStorage.getAuth() || legacyAuth || null;
  pageConfigStore.hydrate("profile");

  try {
    const results = await Promise.allSettled([
      userStore.loadProfile(true),
      businessSdk.roleWorkbench.memberProfile(),
      businessSdk.roleWorkbench.wallet(),
      businessSdk.roleWorkbench.pointsAccount(),
      businessSdk.assets.listPage({ limit: 40, offset: 0 }),
      businessSdk.generation.listTaskPage({ limit: 20, offset: 0, prioritizeActive: true }),
      pageConfigStore.ensure("profile"),
    ]);

    if (results[0].status === "fulfilled" && userStore.currentRole !== "USER" && userStore.roles.includes("USER")) {
      try {
        await userStore.switchRole("USER");
      } catch {
        // The page remains a safe USER view even when role synchronization is temporarily unavailable.
      }
    }
    if (results[1].status === "fulfilled") profile.value = results[1].value;
    if (results[2].status === "fulfilled") wallet.value = results[2].value;
    if (results[3].status === "fulfilled") points.value = results[3].value;
    if (results[4].status === "fulfilled") recentAssets.value = collectionOf<Asset>(results[4].value);
    if (results[5].status === "fulfilled") generationTasks.value = collectionOf<GenerationTask>(results[5].value);

    const businessResults = results.slice(0, 6);
    if (businessResults.every(result => result.status === "rejected")) {
      loadError.value = "数据暂时未同步，已显示本地个人中心。";
    }
  } finally {
    loading.value = false;
  }
}

onShow(() => {
  syncCustomTabBar(3);
  void refreshProfile();
});

onPullDownRefresh(() => {
  void refreshProfile().finally(() => uni.stopPullDownRefresh());
});
</script>

<style>
page { background: #f7f8fc; }

.user-mine-page { min-height: 100vh; background: #f7f8fc; }

.mine-load-note {
  display: flex;
  margin: -92px 16px 110px;
  padding: 12px 14px;
  box-sizing: border-box;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  border: 1px solid #ffe1cc;
  border-radius: 12px;
  color: #8a4b22;
  background: #fff8f2;
  font-size: 12px;
  line-height: 18px;
}

.mine-load-note text { min-width: 0; flex: 1; }

.mine-load-note button {
  display: flex;
  width: auto;
  height: 30px;
  margin: 0;
  padding: 0 12px;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 999px;
  color: #fff;
  background: #7d8df6;
  font-size: 12px;
  line-height: 30px;
}

.mine-load-note button::after { display: none; }
</style>
