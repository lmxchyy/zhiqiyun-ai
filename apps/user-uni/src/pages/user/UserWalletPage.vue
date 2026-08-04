<template>
  <view class="wallet-page" :style="navigationStyle">
    <view class="wallet-nav">
      <button type="button" class="back-button" aria-label="返回" @click="backOrHome">‹</button>
      <view>
        <text class="page-title">钱包与点数</text>
        <text class="page-copy">余额、充值与点数明细</text>
      </view>
    </view>

    <view v-if="contextLoading" class="state-card">
      <text class="state-title">正在确认钱包上下文...</text>
    </view>
    <view v-else-if="contextType !== 'PERSONAL'" class="state-card">
      <text class="state-title">个人钱包未显示</text>
      <text class="state-copy">{{ contextError || "当前不是个人上下文，个人点数和到期信息不会读取或展示。" }}</text>
      <button v-if="contextError" type="button" class="outline-button" @click="refreshWallet">重试</button>
    </view>
    <template v-else>
      <view v-if="walletReady" class="balance-card">
        <text class="balance-label">钱包余额</text>
        <text class="balance-value">{{ formatNumber(account?.available) }}</text>
        <text class="balance-copy">永久余额 {{ formatNumber(account?.permanentAvailable) }} 点 · 冻结 {{ formatNumber(account?.frozen) }} 点</text>
        <template v-if="expirySummary">
          <text class="balance-copy gift-balance">赠送余额 {{ formatNumber(expirySummary.expiringPoints) }} 点</text>
          <text v-if="expirySummary.nextExpiryAt" class="balance-copy">其中 {{ formatNumber(expirySummary.nextExpiryPoints) }} 点将于 {{ formatDate(expirySummary.nextExpiryAt) }} 到期</text>
        </template>
        <text v-if="walletStale" class="stale-warning">当前显示上次成功数据，数据可能已过期。</text>
        <button v-if="walletStale" type="button" class="sync-button" @click="refreshWallet">重新同步</button>
      </view>
      <view v-else class="state-card">
        <text class="state-title">{{ walletLoading ? "正在加载点数余额..." : "点数余额暂不可用" }}</text>
        <text v-if="walletError" class="state-copy">{{ walletError }}</text>
        <button v-if="!walletLoading" type="button" class="outline-button" @click="refreshWallet">重试</button>
      </view>

      <view class="action-row">
        <button type="button" class="primary-button" @click="openPage(miniProgramFeaturePages.userRechargePlans)">全部充值方案</button>
        <button type="button" class="outline-button compact" @click="openPage(miniProgramFeaturePages.userOrders)">我的订单</button>
      </view>

      <view class="section-card">
        <view class="section-head"><text class="section-title">点数充值</text><text class="section-tag">微信支付</text></view>
        <view v-if="rechargePackages.length" class="recharge-list">
          <button v-for="item in rechargePackages" :key="rowString(item, 'id')" type="button" class="recharge-item" @click="openPage(miniProgramFeaturePages.userRechargePlans)">
            <text>{{ formatNumber(rowNumber(item, 'grantPoints') || rowNumber(item, 'points') || rowNumber(item, 'tokenAmount')) }} 点</text>
            <text>{{ formatCurrency(rowNumber(item, 'priceCents') || rowNumber(item, 'amountCents')) }}</text>
          </button>
        </view>
        <text v-else class="empty-copy">暂无可用充值方案。</text>
      </view>

      <view class="section-card">
        <view class="section-head"><text class="section-title">点数明细</text><text class="section-tag">{{ walletRecords.length }} 条</text></view>
        <view v-if="walletRecords.length" class="record-list">
          <view v-for="record in walletRecords.slice(0, 20)" :key="rowKey(record)" class="record-row">
            <view><text class="record-title">{{ recordTitle(record) }}</text><text class="record-date">{{ formatDate(rowDate(record)) }}</text></view>
            <text :class="['record-amount', { grant: personalPointEntryKind(record) === 'GRANT' }]">{{ recordAmount(record) }}</text>
          </view>
        </view>
        <text v-else class="empty-copy">暂无点数明细。</text>
      </view>
    </template>
  </view>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { onHide, onPullDownRefresh, onShow, onUnload } from "@dcloudio/uni-app";
import type { RoleWalletResponse } from "@xianzhi/business-sdk";
import { api, businessSdk } from "../../api/client";
import { useMiniProgramNavigation } from "../../composables/useMiniProgramNavigation";
import { usePersonalPointsWallet } from "../../composables/usePersonalPointsWallet";
import { miniProgramFeaturePages } from "../../config/miniProgramPages";
import {
  createPersonalWalletPageRefreshCoordinator,
  personalPointEntryKind,
  personalPointsExpirySummary,
  personalWalletRuntimeScopeFingerprint,
  type PersonalPointsWalletRuntimeScope,
  type PersonalWalletContextType,
} from "../../features/wallet/personalPointsWallet";
import { useUserStore } from "../../stores/user";
import { useAuthStore } from "../../stores/auth";

type AnyRecord = Record<string, unknown>;

const { navigationStyle } = useMiniProgramNavigation();
const userStore = useUserStore();
const authStore = useAuthStore();
const contextType = ref<PersonalWalletContextType>("");
const contextLoading = ref(false);
const contextError = ref("");
const wallet = ref<RoleWalletResponse | null>(null);
const rechargePackages = ref<AnyRecord[]>([]);
const runtimeScope = (): PersonalPointsWalletRuntimeScope => ({
  sessionKey: authStore.token,
  userId: userStore.userId,
  contextType: userStore.currentContext?.type || "",
  tenantId: userStore.currentContext?.tenantId || "",
});
const personalWallet = usePersonalPointsWallet(runtimeScope);
const pageRefresh = createPersonalWalletPageRefreshCoordinator({
  onInvalidate: clearWalletContent,
  onLoadingChange: loading => { contextLoading.value = loading; },
});

const account = personalWallet.account;
const walletReady = personalWallet.ready;
const walletLoading = personalWallet.loading;
const walletStale = personalWallet.stale;
const walletError = personalWallet.error;
const expirySummary = computed(() => personalPointsExpirySummary(account.value));
const walletRecords = computed(() => [
  ...listOf(personalWallet.payload.value?.transactions),
  ...listOf(wallet.value?.tokenRecords),
].sort((a, b) => dateValue(rowDate(b)) - dateValue(rowDate(a))));

function listOf(value: unknown): AnyRecord[] {
  return Array.isArray(value) ? value.filter((item): item is AnyRecord => Boolean(item) && typeof item === "object") : [];
}

function rowString(row: unknown, ...keys: string[]) {
  if (!row || typeof row !== "object") return "";
  for (const key of keys) {
    const value = (row as AnyRecord)[key];
    if (typeof value === "string" && value.trim()) return value.trim();
  }
  return "";
}

function rowNumber(row: unknown, ...keys: string[]) {
  if (!row || typeof row !== "object") return 0;
  for (const key of keys) {
    const value = Number((row as AnyRecord)[key]);
    if (Number.isFinite(value) && value !== 0) return value;
  }
  return 0;
}

function rowDate(row: unknown) {
  return rowString(row, "createdAt", "occurredAt", "updatedAt", "paidAt");
}

function dateValue(value: string) {
  const timestamp = new Date(value).getTime();
  return Number.isFinite(timestamp) ? timestamp : 0;
}

function rowKey(row: unknown) {
  return rowString(row, "id", "transactionId", "taskId") || JSON.stringify(row);
}

function recordTitle(row: unknown) {
  const kind = personalPointEntryKind(row);
  if (kind === "GRANT") return "赠送点数";
  if (kind === "EXPIRE") return "点数过期";
  return rowString(row, "title", "metricCode", "modelName", "model", "type") || "AI 创作消耗";
}

function recordAmount(row: unknown) {
  const kind = personalPointEntryKind(row);
  const delta = row && typeof row === "object" ? Number((row as AnyRecord).delta) : Number.NaN;
  const points = Math.abs(rowNumber(row, "points", "amount", "delta", "pointCost"));
  if (kind === "GRANT") return `+${formatNumber(points)}`;
  if (kind === "EXPIRE") return `-${formatNumber(points)}`;
  if (Number.isFinite(delta) && delta !== 0) return `${delta > 0 ? "+" : "-"}${formatNumber(Math.abs(delta))}`;
  return rowNumber(row, "pointCost") ? `-${formatNumber(points)}` : formatNumber(points);
}

function formatNumber(value: unknown) {
  const numberValue = Number(value);
  return Number.isFinite(numberValue) ? numberValue.toLocaleString("zh-CN") : "--";
}

function formatCurrency(value: unknown) {
  const cents = Number(value);
  return `¥${(Number.isFinite(cents) ? cents / 100 : 0).toFixed(2)}`;
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value || "--";
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}

function openPage(url: string) {
  uni.navigateTo({ url });
}

function backOrHome() {
  if (getCurrentPages().length > 1) uni.navigateBack({ delta: 1 });
  else uni.reLaunch({ url: "/pages/user/UserHomePage" });
}

async function refreshWallet() {
  const token = pageRefresh.beginRefresh();
  contextError.value = "";
  try {
    const contexts = await userStore.fetchEnterpriseContexts();
    const nextContext = contexts.current || contexts.contexts?.find(item => item.current) || null;
    const nextScope = {
      sessionKey: authStore.token,
      userId: userStore.userId,
      contextType: nextContext?.type || "",
      tenantId: nextContext?.tenantId || "",
    };
    const committed = pageRefresh.commitOwnedScope(
      token,
      personalWalletRuntimeScopeFingerprint(nextScope),
      () => { userStore.applyEnterpriseContexts(contexts); },
    );
    if (!committed) return;
    contextType.value = nextScope.contextType as PersonalWalletContextType;

    if (contextType.value !== "PERSONAL") return;

    const [walletResult, pointsResult, plansResult] = await Promise.allSettled([
      businessSdk.roleWorkbench.wallet(),
      personalWallet.refresh(),
      api<AnyRecord[] | { items?: AnyRecord[] }>("/api/v1/plans?planType=recharge"),
    ]);
    if (!pageRefresh.isCurrent(token)) return;
    wallet.value = walletResult.status === "fulfilled" ? walletResult.value : null;
    if (pointsResult.status === "rejected") personalWallet.hide();
    rechargePackages.value = plansResult.status === "fulfilled"
      ? (Array.isArray(plansResult.value) ? plansResult.value : listOf(plansResult.value.items))
      : [];
  } catch {
    if (!pageRefresh.isCurrent(token)) return;
    contextType.value = "";
    clearWalletContent();
    contextError.value = "无法确认当前是否为个人上下文，已停止读取个人钱包。";
  } finally {
    pageRefresh.finishRefresh(token);
  }
}

function clearWalletContent() {
  wallet.value = null;
  rechargePackages.value = [];
  personalWallet.hide();
}

function invalidateWalletView() {
  pageRefresh.cancel();
  contextType.value = "";
}

watch(
  () => [
    authStore.token,
    userStore.userId,
    userStore.currentContext?.type || "",
    userStore.currentContext?.tenantId || "",
  ],
  () => {
    pageRefresh.scopeChanged(personalWalletRuntimeScopeFingerprint(runtimeScope()));
    contextType.value = userStore.currentContext?.type || "";
  },
  { flush: "sync" },
);

onShow(() => { void refreshWallet(); });
onPullDownRefresh(() => { void refreshWallet().finally(() => uni.stopPullDownRefresh()); });
onHide(invalidateWalletView);
onUnload(invalidateWalletView);
</script>

<style>
page { background: #f7f8fc; }
.wallet-page { min-height: 100vh; padding: 18px 16px 40px; box-sizing: border-box; background: #f7f8fc; }
.wallet-nav { display: flex; align-items: center; gap: 12px; margin-bottom: 18px; }
.back-button { width: 38px; height: 38px; margin: 0; padding: 0; border-radius: 12px; color: #111827; background: #fff; font-size: 26px; line-height: 36px; }
.page-title, .page-copy, .balance-label, .balance-value, .balance-copy, .state-title, .state-copy, .record-title, .record-date, .empty-copy { display: block; }
.page-title { color: #111827; font-size: 22px; font-weight: 700; }
.page-copy { margin-top: 3px; color: #6b7280; font-size: 13px; }
.balance-card, .state-card, .section-card { margin-bottom: 14px; padding: 18px; border-radius: 18px; box-sizing: border-box; }
.balance-card { color: #fff; background: #111827; }
.balance-label, .balance-copy { color: rgba(255,255,255,.72); }
.balance-value { margin: 7px 0 5px; font-size: 36px; font-weight: 700; }
.balance-copy { margin-top: 4px; font-size: 13px; }
.gift-balance, .stale-warning { margin-top: 12px; color: #fde68a; }
.gift-balance { font-weight: 600; }
.stale-warning { display: block; font-size: 12px; }
.sync-button { width: auto; min-height: 34px; margin: 10px 0 0; padding: 0 14px; border-radius: 999px; color: #111827; background: #fff; font-size: 12px; }
.state-card, .section-card { background: #fff; border: 1px solid #e5e7eb; }
.state-title { color: #111827; font-weight: 600; }
.state-copy, .empty-copy { margin-top: 7px; color: #6b7280; font-size: 13px; line-height: 1.6; }
.action-row { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin-bottom: 14px; }
.primary-button, .outline-button { min-height: 44px; margin: 0; border-radius: 13px; font-size: 14px; font-weight: 600; }
.primary-button { color: #fff; background: #7d8df6; }
.outline-button { margin-top: 14px; color: #5a4db2; background: #fff; border: 1px solid #c7d2fe; }
.outline-button.compact { margin-top: 0; }
.section-head, .recharge-item, .record-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.section-title { color: #111827; font-size: 16px; font-weight: 700; }
.section-tag { padding: 4px 9px; border-radius: 999px; color: #5a4db2; background: #eef2ff; font-size: 11px; }
.recharge-list, .record-list { margin-top: 12px; }
.recharge-item { width: 100%; margin-top: 8px; padding: 13px; color: #111827; background: #f9fafb; border-radius: 12px; font-size: 14px; }
.record-row { padding: 12px 0; border-top: 1px solid #f3f4f6; }
.record-title { color: #111827; font-size: 14px; }
.record-date { margin-top: 3px; color: #9ca3af; font-size: 11px; }
.record-amount { color: #dc2626; font-size: 14px; font-weight: 700; }
.record-amount.grant { color: #059669; }
</style>
