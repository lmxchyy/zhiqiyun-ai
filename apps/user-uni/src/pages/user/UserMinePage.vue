<template>
  <view class="user-mine-page" :style="miniProgramNavigationStyle">
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
	  :hide-commerce-summary="true"
      @upgrade="openAgentCommerce()"
      @edit="openPage(miniProgramFeaturePages.userProfileEdit)"
      @recharge="openPage(miniProgramFeaturePages.userRechargePlans)"
      @role-change="handleRoleChange($event)"
      @service="handleService($event)"
      @benefit="handleBenefit($event)"
    >
      <template #commerce>
        <UserCommerceCards
          :point-balance="pointBalance"
          :point-balance-loading="balanceLoading"
          :point-balance-ready="pointBalanceReady"
          :member-active="memberActive"
          :member-expires-text="memberExpiresText"
          :agent-active="agentActive"
          :invite-code="agentInviteCode"
          :promotion-count="agentPromotionCount"
          :pending-commission-cents="agentPendingCommissionCents"
          @recharge="openPage(miniProgramFeaturePages.userRechargePlans)"
          @member="openPage(miniProgramFeaturePages.userMembershipDetail)"
          @agent="openAgentCommerce()"
        />
      </template>
      <template #extra>
        <view class="mine-test-card">
          <text class="mine-test-heading">测试</text>
          <button type="button" class="mine-test-row" hover-class="mine-test-pressed" @click="openPaymentTest('member')">
            <text class="mine-test-icon">测</text>
            <text class="mine-test-label">支付测试 · 会员 ¥1</text>
            <text class="mine-test-chevron">›</text>
          </button>
          <button type="button" class="mine-test-row" hover-class="mine-test-pressed" @click="openPaymentTest('agent')">
            <text class="mine-test-icon">测</text>
            <text class="mine-test-label">支付测试 · 代理 ¥1</text>
            <text class="mine-test-chevron">›</text>
          </button>
        </view>
      </template>
    </V531ProfilePage>

    <view v-if="loadError" class="mine-load-note" role="alert">
      <text>{{ loadError }}</text>
      <button type="button" @click="refreshProfile()">重新加载</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { onPullDownRefresh, onShow } from "@dcloudio/uni-app";
import type { MemberProfileResponse, RoleWalletResponse } from "@xianzhi/business-sdk";
import V531ProfilePage from "../../components/v531/V531ProfilePage.vue";
import UserCommerceCards from "../../components/commerce/UserCommerceCards.vue";
import { authStorage, businessSdk, setAuthToken } from "../../api/client";
import { fetchRecentWorksTask } from "../../features/assets/api";
import type { AssetItem } from "../../features/assets/types";
import {
  miniProgramCreationPages,
  miniProgramEnterprisePages,
  miniProgramFeaturePages,
  miniProgramMinePages,
  rolePage,
} from "../../config/miniProgramPages";
import { isAppRole, permissionsForRole, roleLabels } from "../../config/permissions";
import { usePageConfigStore } from "../../stores/pageConfig";
import { useAuthStore } from "../../stores/auth";
import { useUserStore } from "../../stores/user";
import type { AppRole, AuthResponse, GenerationTask } from "../../types";
import { syncCustomTabBar } from "../../utils/customTabBar";

type AnyRecord = Record<string, unknown>;
type CachedPointAccount = NonNullable<RoleWalletResponse["account"]>;

interface MineSnapshot {
  scope: string;
  storedAt: number;
  profile?: MemberProfileResponse;
  account?: CachedPointAccount;
}

const mineSnapshotStorageKey = "zhiqiyun:user-mine-snapshot:v1";

function authScope(value: AuthResponse | null) {
  return String(value?.user?.id || "").trim();
}

function readMineSnapshot(value: AuthResponse | null): MineSnapshot | null {
  const scope = authScope(value);
  if (!scope) return null;
  try {
    const snapshot = uni.getStorageSync(`${mineSnapshotStorageKey}:${encodeURIComponent(scope)}`) as Partial<MineSnapshot> | "";
    return snapshot
      && typeof snapshot === "object"
      && snapshot.scope === scope
      && Number.isFinite(snapshot.storedAt)
      ? snapshot as MineSnapshot
      : null;
  } catch {
    return null;
  }
}

const userStore = useUserStore();
const authStore = useAuthStore();
const pageConfigStore = usePageConfigStore();
const initialAuth = authStorage.getAuth();
const initialSnapshot = readMineSnapshot(initialAuth);
const auth = ref<AuthResponse | null>(initialAuth);
const profile = ref<MemberProfileResponse | null>(initialSnapshot?.profile || null);
const wallet = ref<RoleWalletResponse | null>(null);
const points = ref<RoleWalletResponse | null>(initialSnapshot?.account ? { account: initialSnapshot.account } : null);
const recentAssets = ref<AssetItem[]>([]);
const generationTasks = ref<GenerationTask[]>([]);
const loading = ref(false);
const balanceLoading = ref(false);
const pointBalanceReady = ref(Boolean(initialSnapshot?.account));
const loadError = ref("");
const isGuest = computed(() => !authStore.token);

const userPermissions = computed(() => userStore.currentRole === "USER"
  ? userStore.permissions
  : permissionsForRole("USER"));
const displayName = computed(() => {
  if (isGuest.value) return "\u6e38\u5ba2";
  const value = String(auth.value?.user?.name
    || profile.value?.user?.name
    || auth.value?.user?.email
    || profile.value?.user?.email
    || "当前用户").trim();
  return /^用户\s*1\d{2}\*+\d+/i.test(value) ? "知启云用户" : value;
});
const displayUserId = computed(() => rowString(auth.value?.user, "id", "userId")
  || rowString(profile.value?.user, "id", "userId")
  || "--");
const avatarUrl = computed(() => rowString(auth.value?.user, "avatarUrl", "avatar", "headImage")
  || rowString(profile.value?.user, "avatarUrl", "avatar", "headImage"));
const avatarFallback = computed(() => {
  const slot = pageConfigStore.slot("profile", "profile.avatar");
  return slot?.imageUrl || slot?.fallbackUrl || "";
});
const planName = computed(() => rowString(profile.value?.plan, "name", "planName")
  || rowString(auth.value?.user, "planName", "memberLevel", "planId")
  || "AI 创作用户");
const companyName = computed(() => rowString(profile.value?.user, "companyName", "company", "organization", "tenantName")
  || rowString(auth.value?.user, "companyName", "company", "organization", "tenantName")
  || rowString(profile.value?.operationCenter, "companyName", "name", "tenantName")
  || rowString(profile.value?.plan, "companyName", "tenantName")
  || "企业信息待完善");
const subscriptionExpiresAt = computed(() => rowString(profile.value?.user, "subscriptionExpiresAt", "expiresAt", "validUntil")
  || rowString(auth.value?.user, "subscriptionExpiresAt", "expiresAt", "validUntil")
  || rowString(profile.value?.plan, "expiresAt", "validUntil", "endedAt"));
const pointAccount = computed(() => points.value?.account || wallet.value?.account || profile.value?.account || null);
const pointBalance = computed(() => asNumber(pointAccount.value?.available));
const memberActive = computed(() => {
  const level = rowString(profile.value?.user, "memberLevel").toUpperCase();
  if (!level || level === "FREE") return false;
  const expires = new Date(subscriptionExpiresAt.value);
  return Number.isNaN(expires.getTime()) || expires.getTime() > Date.now();
});
const memberExpiresText = computed(() => {
  const expires = new Date(subscriptionExpiresAt.value);
  if (Number.isNaN(expires.getTime())) return subscriptionExpiresAt.value || "--";
  return `${expires.getFullYear()}-${String(expires.getMonth() + 1).padStart(2, "0")}-${String(expires.getDate()).padStart(2, "0")}`;
});
const agentActive = computed(() => userStore.roles.includes("AGENT") || rowString(profile.value?.user, "agentStatus").toUpperCase() === "ACTIVE");
const agentInviteCode = computed(() => rowString(profile.value?.agent, "inviteCode", "invite_code") || rowString(profile.value?.user, "inviteCode"));
const agentPromotionCount = computed(() => asNumber((profile.value?.agent as AnyRecord | undefined)?.promotionCount || (profile.value?.agent as AnyRecord | undefined)?.customerCount));
const agentPendingCommissionCents = computed(() => asNumber((profile.value?.agent as AnyRecord | undefined)?.pendingCommissionCents || (profile.value?.agent as AnyRecord | undefined)?.frozenCommissionCents));
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

function openAgentCommerce() {
  if (agentActive.value) {
    uni.reLaunch({ url: rolePage("agent", "overview") });
    return;
  }
  openPage(miniProgramFeaturePages.userAgentDetail);
}

/** Hidden TEST ¥1 quotes — whitelist enforced by backend; formal ¥996 commerce paths untouched. */
const paymentTestTargets = {
  member: {
    planId: "plan_ai_creator_996",
    pricePlanId: "price_plan_20260728212634000000000_049a91b1",
  },
  agent: {
    planId: "plan_agent_join_996",
    pricePlanId: "price_plan_20260728212634000000000_2ec1c485",
  },
} as const;

function openPaymentTest(kind: keyof typeof paymentTestTargets) {
  const target = paymentTestTargets[kind];
  openPage(
    `${miniProgramFeaturePages.userVirtualPaymentTest}?planId=${encodeURIComponent(target.planId)}&pricePlanId=${encodeURIComponent(target.pricePlanId)}`,
  );
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
      clearProfileData();
      authStore.logout();
      uni.removeStorageSync("xianzhiMiniProgramAuth");
      uni.switchTab({ url: "/pages/user/UserHomePage" });
    },
  });
}

function handleService(payload: unknown) {
  const id = emittedValue(payload);
  const actions: Record<string, () => void> = {
    ai: () => openUserTab(rolePage("user", "create")),
    recharge: () => openPage(miniProgramFeaturePages.userRechargePlans),
    membership: () => openPage(miniProgramFeaturePages.userMembershipDetail),
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
      : openPage(miniProgramFeaturePages.userAgentDetail),
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
  if (id === "member") openPage(miniProgramFeaturePages.userMembershipDetail);
  else showCompany();
}

let refreshEpoch = 0;
let activeProfileScope = authScope(initialAuth);
let activeRecentAbort: (() => void) | null = null;
let activeProfileRefresh: { token: string; promise: Promise<void> } | null = null;

function writeMineSnapshot() {
  const scope = authScope(auth.value);
  if (!scope) return;
  const snapshot: MineSnapshot = {
    scope,
    storedAt: Date.now(),
  };
  if (profile.value) snapshot.profile = profile.value;
  if (pointAccount.value) snapshot.account = pointAccount.value;
  try {
    uni.setStorageSync(`${mineSnapshotStorageKey}:${encodeURIComponent(scope)}`, snapshot);
  } catch {
    // A full or unavailable local cache must never block live profile requests.
  }
}

function hydrateMineSnapshot(nextAuth: AuthResponse | null) {
  const nextScope = authScope(nextAuth);
  auth.value = nextAuth;
  if (nextScope === activeProfileScope) return;
  activeProfileScope = nextScope;
  const snapshot = readMineSnapshot(nextAuth);
  profile.value = snapshot?.profile || null;
  wallet.value = null;
  points.value = snapshot?.account ? { account: snapshot.account } : null;
  pointBalanceReady.value = Boolean(snapshot?.account);
  recentAssets.value = [];
  generationTasks.value = [];
}

function clearProfileData() {
  refreshEpoch += 1;
  activeProfileScope = "";
  activeRecentAbort?.();
  activeRecentAbort = null;
  auth.value = null;
  profile.value = null;
  wallet.value = null;
  points.value = null;
  recentAssets.value = [];
  generationTasks.value = [];
  loading.value = false;
  balanceLoading.value = false;
  pointBalanceReady.value = false;
  loadError.value = "";
}

function isCurrentRefresh(requestEpoch: number, token: string) {
  return requestEpoch === refreshEpoch
    && token === (authStorage.getToken() || String(uni.getStorageSync("token") || ""));
}

function acceptPointAccount(response: RoleWalletResponse, target: "wallet" | "points", requestEpoch: number, token: string) {
  if (!isCurrentRefresh(requestEpoch, token)) return;
  if (target === "points") points.value = response;
  else wallet.value = response;
  if (response.account) {
    pointBalanceReady.value = true;
    balanceLoading.value = false;
    writeMineSnapshot();
  }
}

function refreshSecondaryData(requestEpoch: number, token: string) {
  activeRecentAbort?.();
  const recentRequest = fetchRecentWorksTask(20);
  activeRecentAbort = recentRequest.abort;
  void recentRequest.promise
    .then(items => {
      if (isCurrentRefresh(requestEpoch, token)) recentAssets.value = items;
    })
    .catch(() => undefined)
    .finally(() => {
      if (activeRecentAbort === recentRequest.abort) activeRecentAbort = null;
    });

  void businessSdk.generation.listTaskPage({ limit: 20, offset: 0, prioritizeActive: true })
    .then(value => {
      if (isCurrentRefresh(requestEpoch, token)) {
        generationTasks.value = collectionOf<GenerationTask>(value);
      }
    })
    .catch(() => undefined);
  void pageConfigStore.ensure("profile").catch(() => undefined);
}

async function runProfileRefresh(token: string, requestEpoch: number) {
  loading.value = true;
  balanceLoading.value = true;
  loadError.value = "";

  try {
    setAuthToken(token);
    const legacyAuth = uni.getStorageSync("xianzhiMiniProgramAuth") as AuthResponse | "";
    hydrateMineSnapshot(authStorage.getAuth() || legacyAuth || null);
    pageConfigStore.hydrate("profile");
    refreshSecondaryData(requestEpoch, token);

    const userProfileRequest = userStore.loadProfile(true);
    const memberProfileRequest = businessSdk.roleWorkbench.memberProfile()
      .then(value => {
        if (!isCurrentRefresh(requestEpoch, token)) return value;
        profile.value = value;
        if (value.account) {
          pointBalanceReady.value = true;
          balanceLoading.value = false;
        }
        writeMineSnapshot();
        return value;
      });
    const walletRequest = businessSdk.roleWorkbench.wallet()
      .then(value => {
        acceptPointAccount(value, "wallet", requestEpoch, token);
        return value;
      });
    const pointsRequest = businessSdk.roleWorkbench.pointsAccount()
      .then(value => {
        acceptPointAccount(value, "points", requestEpoch, token);
        return value;
      });
    const results = await Promise.allSettled([
      userProfileRequest,
      memberProfileRequest,
      walletRequest,
      pointsRequest,
    ]);

    if (!isCurrentRefresh(requestEpoch, token)) return;

    if (results[0].status === "fulfilled" && userStore.currentRole !== "USER" && userStore.roles.includes("USER")) {
      try {
        await userStore.switchRole("USER");
      } catch {
        // The page remains a safe USER view even when role synchronization is temporarily unavailable.
      }
    }

    const profileFresh = results[1].status === "fulfilled";
    const accountFresh = (results[1].status === "fulfilled" && Boolean(results[1].value.account))
      || (results[2].status === "fulfilled" && Boolean(results[2].value.account))
      || (results[3].status === "fulfilled" && Boolean(results[3].value.account));
    const errors: string[] = [];
    if (!profileFresh) errors.push("登录信息同步失败");
    if (!accountFresh) errors.push("点数余额同步失败");
    if (errors.length) {
      loadError.value = `${errors.join("、")}${pointBalanceReady.value ? "，当前显示上次数据" : "，请下拉刷新重试"}`;
    }
  } catch {
    if (isCurrentRefresh(requestEpoch, token)) {
      loadError.value = `登录信息、点数余额同步失败${pointBalanceReady.value ? "，当前显示上次数据" : "，请下拉刷新重试"}`;
    }
  } finally {
    if (!isCurrentRefresh(requestEpoch, token)) return;
    loading.value = false;
    balanceLoading.value = false;
  }
}

function refreshProfile(): Promise<void> {
  const token = authStorage.getToken() || String(uni.getStorageSync("token") || "");
  if (!token) {
    clearProfileData();
    userStore.reset();
    return Promise.resolve();
  }
  if (activeProfileRefresh?.token === token) return activeProfileRefresh.promise;

  const requestEpoch = ++refreshEpoch;
  const promise = Promise.resolve()
    .then(() => runProfileRefresh(token, requestEpoch))
    .finally(() => {
      if (activeProfileRefresh?.promise === promise) activeProfileRefresh = null;
    });
  activeProfileRefresh = { token, promise };
  return promise;
}

watch(() => authStore.token, (nextToken, previousToken) => {
  if (nextToken === previousToken) return;
  if (!nextToken) {
    clearProfileData();
    return;
  }
  void refreshProfile();
});

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

.user-mine-page { min-height: 100vh; padding-top: var(--header-height, 64px); box-sizing: border-box; background: #f7f8fc; }

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

.mine-test-card {
  margin-top: 16px;
  overflow: hidden;
  border: 1px solid #edf0f7;
  border-radius: 16px;
  background: #fff;
}

.mine-test-heading {
  display: block;
  padding: 12px 16px 4px;
  color: #98a2b3;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
}

.mine-test-row {
  display: flex;
  width: 100%;
  min-height: 56px;
  margin: 0;
  padding: 0 16px !important;
  box-sizing: border-box;
  align-items: center;
  gap: 12px;
  border: 0;
  color: inherit;
  background: transparent;
  text-align: left;
  line-height: normal;
}

.mine-test-row::after { display: none; }

.mine-test-pressed { opacity: .78; transform: scale(.985); }

.mine-test-icon {
  display: grid;
  width: 30px;
  min-width: 30px;
  height: 30px;
  place-items: center;
  border-radius: 9px;
  color: #9a3412;
  background: #ffedd5;
  font-size: 11px;
  font-weight: 700;
}

.mine-test-label {
  min-width: 0;
  flex: 1;
  color: #171c29;
  font-size: 13px;
  font-weight: 700;
}

.mine-test-chevron {
  color: #98a2b3;
  font-size: 18px;
  line-height: 1;
}
</style>
