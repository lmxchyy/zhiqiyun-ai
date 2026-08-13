<template>
  <view class="mine-experience">
    <template v-if="view === 'overview'">
      <MineOverviewPanel
        :display-name="displayName"
        :avatar-url="avatarUrl"
        :avatar-fallback="avatarFallback"
        :header-background="headerBackground"
        :member-background="memberBackground"
        :point-balance="pointBalance"
        :roles="roles"
        :current-role="currentRole"
        @navigate="$emit('navigate', $event)"
        @edit-profile="openFeaturePage(miniProgramFeaturePages.userProfileEdit)"
        @switch-agent="$emit('switch-agent')"
        @open-wallet="openWallet"
        @recharge="openFeaturePage(miniProgramFeaturePages.userRechargePlans)"
        @open-task-records="openTaskRecords"
        @open-orders="openFeaturePage(miniProgramFeaturePages.userOrders)"
        @open-settings="openFeaturePage(miniProgramFeaturePages.userSettings)"
      />
    </template>

    <template v-else>
      <view class="mine-detail-header">
        <button class="mine-back-button" aria-label="返回我的" @click="$emit('navigate', 'overview')">‹</button>
        <view><text>{{ detailTitle }}</text><text>{{ detailSubtitle }}</text></view>
      </view>

      <MineRechargeHistoryPanel
        v-if="view === 'recharge-history'"
        :filtered-orders="filteredOrders"
        :active-filter="orderFilter"
        :history-range-label="historyRangeLabel"
        :total-recharge-cents="totalRechargeCents"
        :total-recharge-points="totalRechargePoints"
        @invoice="$emit('invoice')"
        @update:filter="orderFilter = $event"
        @cycle-range="cycleHistoryRange"
        @open-detail="openOrderDetail"
      />

      <MineUsageDetailsPanel
        v-else-if="view === 'usage-details'"
        :monthly-point-cost="monthlyPointCost"
        :point-balance="pointBalance"
        :usage-breakdown="usageBreakdown"
        :usage-filter-label="usageFilterLabel"
        :current-month-only="usageCurrentMonthOnly"
        :filtered-usage-records="filteredUsageRecords"
        @cycle-type="cycleUsageType"
        @toggle-current-month="usageCurrentMonthOnly = !usageCurrentMonthOnly"
        @export="$emit('export-usage')"
        @open-detail="openUsageRecordDetail"
      />

      <MineRolePermissionsPanel
        v-else-if="view === 'role-permissions'"
        :current-role="currentRole"
        :current-role-label="currentRoleLabel"
        :has-agent-role="hasAgentRole"
        :role-rows="roleRows"
        :granted-permission-rows="grantedPermissionRows"
        @upgrade="$emit('navigate', 'agent-upgrade')"
        @switch-agent="$emit('switch-agent')"
      />

      <MineInvitePromotionPanel
        v-else
        :invite-code="inviteCode"
        :invite-link="inviteLink"
        :has-agent-role="hasAgentRole"
        :agent-level-label="agentLevelLabel"
        :conversion-rate="conversionRate"
        :stats="{ visits: stat('visits'), registrations: stat('registrations'), orders: stat('orders') }"
        :promotion-steps="promotionSteps"
        @copy-invite="$emit('copy-invite')"
        @poster="$emit('poster')"
        @upgrade="$emit('navigate', 'agent-upgrade')"
      />
    </template>

    <MinePurchaseSheet
      :purchase="purchase"
      :submitting="purchaseSubmitting"
      @close="$emit('close-purchase')"
      @confirm="$emit('confirm-purchase')"
      @open-agreement="openPurchaseAgreement"
    />

    <MineLogoutSheet
      :visible="logoutConfirm"
      @close="$emit('close-logout')"
      @confirm="$emit('confirm-logout')"
    />
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import type { MinePurchaseOption, MineView } from "../types";
import type { AppRole } from "../types";
import { miniProgramFeaturePages, miniProgramRolePages } from "../config/miniProgramPages";
import { roleLabels } from "../config/permissions";
import { openLegalDocument } from "../features/legal/navigation";
import MineOverviewPanel from "./mine/MineOverviewPanel.vue";
import MineRechargeHistoryPanel from "./mine/MineRechargeHistoryPanel.vue";
import MineUsageDetailsPanel from "./mine/MineUsageDetailsPanel.vue";
import MineRolePermissionsPanel from "./mine/MineRolePermissionsPanel.vue";
import MineInvitePromotionPanel from "./mine/MineInvitePromotionPanel.vue";
import MinePurchaseSheet from "./mine/MinePurchaseSheet.vue";
import MineLogoutSheet from "./mine/MineLogoutSheet.vue";

type AnyRecord = Record<string, unknown>;

const props = defineProps<{
  view: MineView;
  logo: string;
  displayName: string;
  avatarUrl?: string;
  avatarFallback?: string;
  headerBackground?: string;
  memberBackground?: string;
  pointBalance: number;
  monthlyPointCost: number;
  roles: AppRole[];
  currentRole: AppRole;
  permissions: string[];
  agentLevelLabel: string;
  orders: AnyRecord[];
  usageRecords: AnyRecord[];
  inviteCode: string;
  inviteLink: string;
  channelSummary: AnyRecord;
  purchase: MinePurchaseOption | null;
  purchaseSubmitting: boolean;
  logoutConfirm: boolean;
}>();

defineEmits<{
  navigate: [view: MineView];
  "select-purchase": [purchase: MinePurchaseOption];
  "close-purchase": [];
  "confirm-purchase": [];
  "request-logout": [];
  "close-logout": [];
  "confirm-logout": [];
  "switch-agent": [];
  "copy-invite": [];
  invoice: [];
  "export-usage": [];
  poster: [];
}>();

const promotionSteps = ["分享链接", "客户注册", "完成订单", "获得分润"];
const orderFilter = ref<"all" | "success" | "refund">("all");
const historyRange = ref<30 | 90 | 0>(90);
const usageType = ref<"all" | "image" | "video" | "ppt">("all");
const usageCurrentMonthOnly = ref(true);
const hasAgentRole = computed(() => props.roles.includes("AGENT"));
const currentRoleLabel = computed(() => roleLabels[props.currentRole]);
const roleDescriptions: Partial<Record<AppRole, string>> = {
  USER: "AI 创作与个人资产管理",
  AGENT: "推广、客户管理与佣金结算",
  OPERATION: "代理、区域订单与经营数据管理",
  ENTERPRISE_ADMIN: "企业组织与成员管理",
  AI_ADMIN: "AI 能力与模型管理",
  FINANCE: "资金、结算与审核",
  CUSTOMER_SERVICE: "客户服务与工单管理",
};
const roleRows = computed(() => props.roles.map(role => ({
  id: role,
  label: roleLabels[role],
  description: roleDescriptions[role] || "已授权业务角色",
})));
const grantedPermissionRows = computed(() => props.permissions.length ? props.permissions : ["暂无当前角色权限"]);

const detailTitle = computed(() => ({
  "agent-upgrade": "成为代理商",
  "recharge-history": "充值记录",
  "usage-details": "消耗明细",
  "role-permissions": "角色与权限",
  "invite-promotion": "邀请与推广"
} as Partial<Record<MineView, string>>)[props.view] || "我的");
const detailSubtitle = computed(() => ({
  "agent-upgrade": "¥996 开通并到账 20,000 点",
  "recharge-history": "到账、订单与退款状态",
  "usage-details": "按模型、任务与日期查看",
  "role-permissions": "查看角色授权与当前权限",
  "invite-promotion": "分享链接并跟踪客户转化"
} as Partial<Record<MineView, string>>)[props.view] || "账户与钱包");
const historyRangeLabel = computed(() => historyRange.value ? `近 ${historyRange.value} 天` : "全部时间");
const filteredOrders = computed(() => props.orders.filter(order => {
  const status = rowString(order, "status").toUpperCase();
  const matchesStatus = orderFilter.value === "all"
    || (orderFilter.value === "success" && isPaidOrder(order))
    || (orderFilter.value === "refund" && ["REFUND_REQUESTED", "REFUNDED", "REFUND", "REVERSED"].includes(status));
  return matchesStatus && isWithinDays(rowDate(order), historyRange.value);
}));

function openPurchaseAgreement() {
  openLegalDocument(props.purchase?.kind === "agent" ? "agent-service-agreement" : "recharge-service-agreement");
}
const usageFilterLabel = computed(() => ({ all: "全部模型", image: "AI 生图", video: "视频生成", ppt: "PPT 文档" })[usageType.value]);
const filteredUsageRecords = computed(() => props.usageRecords.filter(record => {
  const title = usageTitle(record).toLowerCase();
  const matchesType = usageType.value === "all"
    || (usageType.value === "image" && (title.includes("image") || title.includes("生图")))
    || (usageType.value === "video" && (title.includes("video") || title.includes("视频")))
    || (usageType.value === "ppt" && title.includes("ppt"));
  return matchesType && (!usageCurrentMonthOnly.value || isCurrentMonth(rowDate(record)));
}));
const usageBreakdown = computed(() => {
  const total = Math.max(props.monthlyPointCost, 1);
  const image = props.usageRecords.filter(item => usageTitle(item).toLowerCase().includes("image") || usageTitle(item).includes("生图")).reduce((sum, item) => sum + rowPointCost(item), 0);
  const video = props.usageRecords.filter(item => usageTitle(item).includes("视频") || usageTitle(item).toLowerCase().includes("video")).reduce((sum, item) => sum + rowPointCost(item), 0);
  const ppt = props.usageRecords.filter(item => usageTitle(item).toLowerCase().includes("ppt")).reduce((sum, item) => sum + rowPointCost(item), 0);
  return [
    { label: "AI生图", percent: Math.max(8, Math.round(image / total * 100)), tone: "purple" },
    { label: "视频生成", percent: Math.max(8, Math.round(video / total * 100)), tone: "orange" },
    { label: "PPT文档", percent: Math.max(8, Math.round(ppt / total * 100)), tone: "green" }
  ];
});
const totalRechargeCents = computed(() => props.orders.reduce((sum, item) => sum + (isRechargeOrder(item) && isPaidOrder(item) ? rowNumber(item, "amountCents") || rowNumber(item, "amount") : 0), 0));
const totalRechargePoints = computed(() => props.orders.reduce((sum, item) => sum + (isRechargeOrder(item) && isPaidOrder(item) ? orderPoints(item) : 0), 0));
const conversionRate = computed(() => {
  const visits = stat("visits");
  const orders = stat("orders");
  return visits > 0 ? Math.round(orders / visits * 100) : 0;
});

function asRecord(value: unknown): AnyRecord { return value && typeof value === "object" ? value as AnyRecord : {}; }
function rowNumber(row: unknown, key: string) { const value = Number(asRecord(row)[key]); return Number.isFinite(value) ? value : 0; }
function rowString(row: unknown, key: string) { const value = asRecord(row)[key]; return typeof value === "string" ? value : ""; }
function rowDate(row: unknown) { return rowString(row, "createdAt") || rowString(row, "occurredAt") || rowString(row, "updatedAt") || rowString(row, "paidAt"); }
function rowKey(row: unknown) { return rowString(row, "id") || rowString(row, "orderId") || rowString(row, "transactionId") || `${rowDate(row)}-${rowPointCost(row)}`; }
function rowPointCost(row: unknown) { return Math.abs(rowNumber(row, "pointCost") || rowNumber(row, "points") || rowNumber(row, "amount") || rowNumber(row, "delta")); }
function formatNumber(value: number) { return String(Math.max(0, Math.round(value || 0))).replace(/\B(?=(\d{3})+(?!\d))/g, ","); }
function formatCurrency(cents: number) { return `¥${(Math.max(0, cents || 0) / 100).toFixed(2)}`; }
function formatDate(value: string) { if (!value) return "时间待同步"; const date = new Date(value); if (Number.isNaN(date.getTime())) return value; const pad = (part: number) => String(part).padStart(2, "0"); return `${pad(date.getMonth() + 1)}/${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`; }
function isWithinDays(value: string, days: number) { if (!days || !value) return true; const time = new Date(value).getTime(); return Number.isFinite(time) && Date.now() - time <= days * 86400000; }
function isCurrentMonth(value: string) { if (!value) return true; const date = new Date(value); const now = new Date(); return !Number.isNaN(date.getTime()) && date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth(); }
function cycleHistoryRange() { historyRange.value = historyRange.value === 90 ? 30 : historyRange.value === 30 ? 0 : 90; }
function cycleUsageType() { const types: Array<typeof usageType.value> = ["all", "image", "video", "ppt"]; usageType.value = types[(types.indexOf(usageType.value) + 1) % types.length] || "all"; }
function openFeaturePage(url: string) { uni.navigateTo({ url }); }
function openWallet() { openFeaturePage(miniProgramRolePages.user.wallet || "/pages/user/UserWalletPage"); }
function openTaskRecords() { openFeaturePage(miniProgramRolePages.user.assets || "/pages/user/UserAssetsPage"); }
function openOrderDetail(order: unknown) { const id = rowString(order, "id") || rowString(order, "orderId") || rowString(order, "orderNo"); if (id) openFeaturePage(`${miniProgramFeaturePages.userOrderDetail}?id=${encodeURIComponent(id)}`); }
function openUsageRecordDetail(record: unknown) { const id = rowString(record, "id") || rowString(record, "eventId") || rowString(record, "taskId"); if (id) openFeaturePage(`${miniProgramFeaturePages.userUsageRecordDetail}?id=${encodeURIComponent(id)}`); }
function isPaidOrder(row: unknown) { return ["PAID", "SUCCESS", "SUCCEEDED", "SETTLED", "ACTIVE"].includes(rowString(row, "status").toUpperCase()); }
function isRechargeOrder(row: unknown) { const type = `${rowString(row, "orderType")} ${rowString(row, "businessOrderType")} ${rowString(row, "planId")}`.toUpperCase(); return type.includes("RECHARGE"); }
function orderPoints(row: unknown) { const snapshot = asRecord(asRecord(row).priceSnapshot); return rowNumber(snapshot, "rechargePoints") || rowNumber(row, "tokenGrantAmount") || rowNumber(row, "tokenAmount"); }
function orderTitle(row: unknown) { const plan = rowString(row, "planId"); if (plan.includes("agent")) return "代理套餐 996 元"; const cents = rowNumber(row, "amountCents") || rowNumber(row, "amount"); return isRechargeOrder(row) ? `充值 ${Math.round(cents / 100)} 元` : rowString(row, "name") || "平台订单"; }
function orderMeta(row: unknown) { return `${formatDate(rowDate(row))} · ${rowString(row, "paymentMethod") || "微信支付"}`; }
function orderValue(row: unknown) { const points = orderPoints(row); if (isRechargeOrder(row) && points > 0) return `+${formatNumber(points)} 点`; return formatCurrency(rowNumber(row, "amountCents") || rowNumber(row, "amount")); }
function statusTone(row: unknown) { const status = rowString(row, "status").toUpperCase(); if (["PAID", "SUCCESS", "SUCCEEDED", "SETTLED", "ACTIVE"].includes(status)) return "success"; if (["FAILED", "CANCELLED", "REJECTED", "REFUNDED"].includes(status)) return "danger"; return "warning"; }
function usageTitle(row: unknown) { return rowString(row, "modelName") || rowString(row, "model") || rowString(row, "module") || rowString(row, "description") || rowString(row, "changeType") || "AI 服务"; }
function usageIcon(row: unknown) { const title = usageTitle(row).toLowerCase(); if (title.includes("video") || title.includes("视频")) return "视"; if (title.includes("ppt")) return "P"; if (title.includes("image") || title.includes("生图")) return "图"; return "AI"; }
function stat(key: string) { return rowNumber(props.channelSummary, key) || rowNumber(props.channelSummary, key === "visits" ? "clicks" : key === "registrations" ? "directCustomers" : "orderCount"); }
</script>

<style scoped>
.mine-experience { color: #111827; }
.mine-v5-edit::after, .mine-v5-agent-entry::after, .mine-v5-recharge::after, .mine-v5-menu-row::after { display: none; }
.mine-v5-profile-card { position: relative; display: flex; height: 98px; padding: 17px 16px; box-sizing: border-box; overflow: hidden; align-items: center; gap: 16px; border: 1px solid #e5e7eb; border-radius: 18px; background: #fff; }
.mine-v5-header-background, .mine-v5-member-background { position: absolute; z-index: 0; inset: 0; opacity: .22; }
.mine-v5-avatar { position: relative; z-index: 1; display: flex; width: 58px; min-width: 58px; height: 58px; align-items: center; justify-content: center; border-radius: 50%; color: #5a4db2; background: #ece9ff; font-size: 17px; font-weight: 700; }
.mine-v5-profile-copy { position: relative; z-index: 1; min-width: 0; flex: 1; }
.mine-v5-profile-copy text { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.mine-v5-profile-name { color: #111827; font-size: 14px; font-weight: 600; line-height: 20px; }
.mine-v5-profile-type { margin-top: 5px; color: #6b7280; font-size: 10px; line-height: 15px; }
.mine-v5-edit { position: relative; z-index: 1; width: auto; min-width: 76px; height: 34px; margin: 0; padding: 0; color: #6b7280; background: transparent; font-size: 10px; line-height: 34px; text-align: right; }
.mine-v5-agent-entry { display: flex; width: 100%; height: 48px; margin: 12px 0 0; padding: 7px 12px; box-sizing: border-box; align-items: center; gap: 10px; border: 1px solid #ffc48c; border-radius: 10px; color: #111827; background: #fff8f0; text-align: left; }
.mine-v5-agent-copy { min-width: 0; flex: 1; }
.mine-v5-agent-copy text { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.mine-v5-agent-copy text:first-child { font-size: 11px; font-weight: 600; line-height: 15px; }
.mine-v5-agent-copy text:last-child { margin-top: 2px; color: #8a5a32; font-size: 10px; line-height: 13px; }
.mine-v5-agent-action { color: #ff771b; font-size: 11px; font-weight: 600; white-space: nowrap; }
.mine-v5-balance-card { position: relative; height: 116px; margin-top: 18px; padding: 15px 18px; box-sizing: border-box; overflow: hidden; border-radius: 18px; color: #fff; background: linear-gradient(100deg, #5463ff 0%, #744ee8 100%); }
.mine-v5-balance-title { position: relative; z-index: 1; display: block; font-size: 11px; font-weight: 700; line-height: 16px; }
.mine-v5-balance-value { position: relative; z-index: 1; display: flex; margin-top: 6px; align-items: baseline; gap: 10px; }
.mine-v5-balance-value text:first-child { font-size: 24px; font-weight: 700; line-height: 34px; }
.mine-v5-balance-value text:last-child { color: #e2e8ff; font-size: 10px; font-weight: 700; }
.mine-v5-recharge { position: absolute; z-index: 1; top: 31px; right: 19px; width: 82px; height: 42px; margin: 0; padding: 0; border-radius: 14px; color: #5a4db2; background: #fff; font-size: 12px; font-weight: 600; line-height: 42px; }
.mine-v5-menu-card { margin-top: 18px; padding: 0 12px; overflow: hidden; border: 1px solid #e5e7eb; border-radius: 18px; background: #fff; }
.mine-v5-menu-row { display: flex; width: 100%; height: 54px; margin: 0; padding: 0; box-sizing: border-box; align-items: center; gap: 14px; border-radius: 0; border-bottom: 1px solid #e5e7eb; background: transparent; text-align: left; }
.mine-v5-menu-row:last-child { border-bottom: 0; }
.mine-v5-menu-icon { display: inline-flex; width: 34px; min-width: 34px; height: 34px; align-items: center; justify-content: center; border-radius: 9px; color: #fff; font-size: 12px; font-weight: 700; }
.mine-v5-menu-icon.purple { background: #6366f1; }
.mine-v5-menu-icon.blue { background: #3b82f6; }
.mine-v5-menu-icon.violet { background: #8b5cf6; }
.mine-v5-menu-icon.orange { background: #ff7e2e; }
.mine-v5-menu-icon.pink { background: #f4638d; }
.mine-v5-menu-icon.slate { background: #64748b; }
.mine-v5-menu-label { min-width: 0; flex: 1; color: #111827; font-size: 12px; font-weight: 600; line-height: 18px; }
.mine-v5-chevron { width: 14px; color: #94a3b8; font-size: 18px; font-weight: 400; line-height: 24px; text-align: center; }
.mine-profile-card, .mine-panel, .mine-menu-panel, .mine-logout-row, .record-summary, .filter-strip, .record-list, .usage-summary, .role-hero, .invite-hero, .promotion-stats { box-sizing: border-box; border: 1px solid #e5eaf6; border-radius: 12px; background: #fff; box-shadow: 0 8px 20px rgba(23, 28, 56, .06); }
.mine-profile-card { display: grid; grid-template-columns: 54px minmax(0, 1fr); gap: 12px; padding: 15px; align-items: center; color: #fff; background: #15192d; }
.mine-avatar { width: 54px; height: 54px; border-radius: 14px; background: #eef2ff; }
.mine-profile-copy text, .mine-logout-row view text, .mine-menu-panel button view text, .mine-detail-header view text, .benefit-list view text, .record-row view text, .role-row view text, .sheet-title-row view text, .payment-method view text, .logout-title-row view text, .logout-notice text, .share-grid button text { display: block; }
.mine-profile-name { font-size: 16px; font-weight: 700; }.mine-profile-meta { margin-top: 4px; color: #cdd5f5; font-size: 11px; }
.mine-upgrade-button { grid-column: 2; width: 96px; height: 28px; margin: 0; border-radius: 8px; color: #ff6b1a; background: #fff2e8; font-size: 11px; line-height: 28px; }
.mine-panel, .mine-menu-panel { margin-top: 14px; padding: 15px; }.mine-panel.compact { margin-top: 0; }
.mine-section-title { display: block; margin-bottom: 12px; color: #111827; font-size: 15px; font-weight: 700; }
.mine-wallet-grid { display: grid; grid-template-columns: repeat(2, minmax(0,1fr)); gap: 12px; }.mine-wallet-metric { padding: 12px; border: 1px solid #d8d5ff; border-radius: 8px; background: #f4f3ff; }.mine-wallet-metric.orange { border-color: #ffe2cc; background: #fff7ed; }.mine-wallet-metric text { display: block; color: #5b55d6; font-size: 16px; font-weight: 700; }.mine-wallet-metric.orange text { color: #ff6b1a; }.mine-wallet-metric text + text { margin-top: 4px; color: #697386; font-size: 10px; font-weight: 500; }
.mine-recharge-row { display: flex; gap: 10px; margin-top: 10px; }.mine-recharge-row button { min-width: 0; height: 32px; margin: 0; padding: 0 8px; flex: 1; border: 1px solid #e3e8f2; border-radius: 8px; color: #475467; background: #f5f7fb; font-size: 10px; line-height: 30px; }.mine-recharge-row button.primary { color: #5b55d6; border-color: #c9d2ff; background: #eef2ff; }.mine-recharge-row button.orange { color: #ff6b1a; border-color: #ffd0b3; background: #fff7ed; }
.mine-menu-panel { display: flex; flex-direction: column; gap: 10px; padding: 11px; }.mine-menu-panel > button, .mine-logout-row { display: flex; width: 100%; min-height: 54px; margin: 0; padding: 10px; align-items: center; gap: 10px; border: 1px solid #e5eaf6; border-radius: 8px; background: #fff; text-align: left; }.mine-menu-panel button view, .mine-logout-row view { min-width: 0; flex: 1; }.mine-menu-panel button view text, .mine-logout-row view text { color: #111827; font-size: 12px; font-weight: 600; }.mine-menu-panel button view text + text, .mine-logout-row view text + text { margin-top: 3px; color: #697386; font-size: 10px; font-weight: 500; }
.mine-menu-icon { display: inline-flex; width: 36px; height: 36px; flex: 0 0 36px; align-items: center; justify-content: center; border: 1px solid #d8d5ff; border-radius: 8px; color: #5b55d6; background: #f4f3ff; font-size: 12px; font-weight: 700; }.mine-menu-icon.orange { color: #ff6b1a; border-color: #ffe2cc; background: #fff7ed; }.mine-menu-icon.green { color: #079455; border-color: #cbf5df; background: #ecfdf5; }.mine-menu-icon.danger { color: #e73b3b; border-color: #fecaca; background: #fff1f2; }.mine-chevron { color: #98a2b3; font-size: 20px; }.mine-logout-row { margin-top: 10px; border-color: #fecaca; }.mine-logout-row view text:first-child { color: #dc2626; }
.mine-detail-header { display: flex; min-height: 52px; align-items: center; gap: 10px; }.mine-back-button { display: grid; width: 40px; min-width: 40px; height: 40px; margin: 0; padding: 0; place-items: center; border: 1px solid #dfe5f2; border-radius: 10px; color: #5b55d6; background: #fff; font-size: 27px; line-height: 1; }.mine-detail-header view text:first-child { font-size: 15px; font-weight: 700; }.mine-detail-header view text + text { margin-top: 2px; color: #697386; font-size: 10px; }.mine-detail-stack { display: flex; margin-top: 14px; padding-bottom: 18px; flex-direction: column; gap: 14px; }
.agent-hero, .role-hero, .invite-hero { position: relative; padding: 16px; border-radius: 12px; color: #fff; background: #15192d; }.agent-hero text, .role-hero > text, .invite-hero > text { display: block; }.agent-kicker { color: #aaa6ff; font-size: 11px; font-weight: 600; }.agent-title { margin-top: 8px; font-size: 20px; font-weight: 700; line-height: 28px; }.agent-copy { margin-top: 4px; color: #cdd5f5; font-size: 11px; }.agent-pill { display: inline-flex !important; width: auto; margin-top: 16px; padding: 5px 10px; border-radius: 999px; color: #ff6b1a; background: #fff2e8; font-size: 10px; font-weight: 600; }
.step-row { display: grid; grid-template-columns: repeat(3, minmax(0,1fr)); gap: 6px; }.step-row.four { grid-template-columns: repeat(4,minmax(0,1fr)); }.step-row > view { display: flex; align-items: center; gap: 5px; color: #697386; font-size: 10px; }.step-row.four > view { flex-direction: column; text-align: center; }.step-index { display: inline-flex; width: 26px; height: 26px; flex: 0 0 26px; align-items: center; justify-content: center; border-radius: 50%; color: #5b55d6; background: #f4f3ff; font-weight: 600; }.step-index.active { color: #fff; background: #5b55d6; }
.benefit-list { display: flex; flex-direction: column; gap: 12px; }.benefit-list > view { display: flex; align-items: center; gap: 10px; }.benefit-icon { display: inline-flex; width: 30px; height: 30px; align-items: center; justify-content: center; border-radius: 9px; color: #5b55d6; background: #f4f3ff; font-size: 12px; font-weight: 700; }.benefit-list view view text { font-size: 11px; font-weight: 600; }.benefit-list view view text + text { margin-top: 3px; color: #697386; font-size: 10px; font-weight: 500; }
.detail-bottom-action { padding: 14px; border-top: 1px solid #e5eaf6; background: #fff; text-align: center; }.detail-bottom-action > text { display: block; margin-top: 8px; color: #697386; font-size: 10px; }.orange-action, .primary-action, .secondary-action { width: 100%; height: 46px; margin: 0; border-radius: 12px; color: #fff; background: #ff6b1a; font-size: 13px; font-weight: 600; }.primary-action { background: #5b55d6; }.secondary-action { color: #5b55d6; border: 1px solid #d8d5ff; background: #fff; }
.record-summary { position: relative; padding: 16px; }.record-summary.dark { color: #fff; background: #15192d; }.record-summary > text { display: block; }.record-summary > text:first-child { color: #cdd5f5; font-size: 10px; }.record-summary > text:nth-child(2) { margin-top: 7px; font-size: 24px; font-weight: 700; }.record-summary > text:nth-child(3) { margin-top: 5px; color: #aaa6ff; font-size: 10px; }.record-summary button { position: absolute; top: 16px; right: 16px; width: auto; height: 26px; margin: 0; padding: 0 13px; border-radius: 13px; color: #5b55d6; background: #f4f3ff; font-size: 10px; line-height: 26px; }
.filter-strip { display: flex; padding: 9px; align-items: center; gap: 8px; }.filter-strip button { width: auto; min-width: 58px; height: 28px; margin: 0; padding: 0 12px; border: 1px solid #e5eaf6; border-radius: 14px; color: #697386; background: #fff; font-size: 10px; line-height: 26px; }.filter-strip button.active { color: #5b55d6; border-color: #c9d2ff; background: #eef2ff; }.filter-strip > text { margin-left: auto; color: #5b55d6; font-size: 10px; font-weight: 600; }
.record-list { padding: 11px; }.record-row { display: flex; min-height: 62px; padding: 10px; box-sizing: border-box; align-items: center; gap: 10px; border: 1px solid #e5eaf6; border-radius: 10px; }.record-row + .record-row { margin-top: 9px; }.record-icon { display: inline-flex; width: 34px; height: 34px; flex: 0 0 34px; align-items: center; justify-content: center; border-radius: 10px; color: #5b55d6; background: #f4f3ff; font-size: 10px; font-weight: 700; }.record-icon.success { color: #079455; background: #ecfdf5; }.record-icon.danger { color: #dc2626; background: #fff1f2; }.record-icon.warning { color: #ff6b1a; background: #fff7ed; }.record-row view { min-width: 0; flex: 1; }.record-row view text { overflow: hidden; font-size: 11px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }.record-row view text + text { margin-top: 4px; color: #697386; font-size: 10px; font-weight: 500; }.record-value { font-size: 10px; font-weight: 600; }.record-value.success { color: #079455; }.record-value.danger { color: #dc2626; }.record-value.warning, .record-value.purple { color: #5b55d6; }
.mine-empty { display: flex; min-height: 160px; flex-direction: column; align-items: center; justify-content: center; color: #697386; text-align: center; }.mine-empty text:first-child { font-size: 13px; font-weight: 600; }.mine-empty text + text { max-width: 240px; margin-top: 7px; font-size: 10px; line-height: 17px; }
.usage-summary { padding: 16px; }.usage-summary > text { display: block; }.usage-summary > text:first-child { color: #697386; font-size: 10px; }.usage-summary > text:nth-child(2) { margin-top: 5px; font-size: 24px; font-weight: 700; }.usage-summary > text:nth-child(3) { margin: 5px 0 13px; color: #5b55d6; font-size: 10px; font-weight: 600; }.usage-bar-row { display: grid; grid-template-columns: 64px minmax(0,1fr); gap: 8px; margin-top: 7px; align-items: center; }.usage-bar-row > text { color: #697386; font-size: 10px; }.usage-bar-row > view { height: 6px; overflow: hidden; border-radius: 3px; background: #eef0f6; }.usage-bar-row > view > view { height: 100%; border-radius: 3px; }.usage-bar-row .purple { background: #5b55d6; }.usage-bar-row .orange { background: #ff6b1a; }.usage-bar-row .green { background: #079455; }.billing-note { padding: 12px; border-radius: 10px; background: #f4f3ff; }.billing-note text { display: block; color: #5b55d6; font-size: 10px; font-weight: 600; }.billing-note text + text { margin-top: 4px; color: #697386; font-size: 10px; font-weight: 500; }
.role-hero > text:first-child, .invite-hero > text:first-child { color: #cdd5f5; font-size: 10px; }.role-hero > text:nth-child(2), .invite-hero > text:nth-child(2) { margin-top: 7px; font-size: 22px; font-weight: 700; }.role-hero > text:nth-child(3) { margin-top: 5px; color: #cdd5f5; font-size: 10px; }.role-status { position: absolute; top: 16px; right: 16px; padding: 5px 11px; border-radius: 999px; color: #079455 !important; background: #ecfdf5; font-size: 10px !important; font-weight: 600; }.role-hero button { width: 100%; height: 34px; margin: 16px 0 0; color: #ffb27a; border-radius: 8px; background: rgba(255,255,255,.06); font-size: 10px; text-align: left; }
.role-row { display: flex; min-height: 62px; align-items: center; gap: 10px; }.role-row + .role-row { border-top: 1px solid #e5eaf6; }.role-row > view { min-width: 0; flex: 1; }.role-row view text { display: block; font-size: 11px; font-weight: 600; }.role-row view text + text { margin-top: 3px; color: #697386; font-size: 10px; font-weight: 500; }.success-text { color: #079455; font-size: 10px; font-weight: 600; }.warning-text, .orange-text { color: #ff6b1a; font-size: 10px; font-weight: 600; }.muted-text { color: #98a2b3; font-size: 10px; }.permission-row { display: flex; min-height: 34px; align-items: center; justify-content: space-between; border-top: 1px solid #eef0f6; }.permission-row text:first-child { font-size: 11px; }
.invite-hero .agent-pill { position: absolute; top: 0; right: 16px; }.invite-link { display: flex; margin-top: 18px; padding: 9px 10px; align-items: center; gap: 8px; border-radius: 10px; background: rgba(255,255,255,.07); }.invite-link > text { min-width: 0; flex: 1; overflow: hidden; color: #cdd5f5; font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }.invite-link button { width: 64px; height: 30px; margin: 0; border-radius: 9px; color: #fff; background: #5b55d6; font-size: 10px; }.invite-hero > text:last-child { margin-top: 10px; color: #cdd5f5; font-size: 10px; }.promotion-stats { display: grid; grid-template-columns: repeat(4,minmax(0,1fr)); padding: 16px 8px; }.promotion-stats view { text-align: center; }.promotion-stats view + view { border-left: 1px solid #e5eaf6; }.promotion-stats text { display: block; font-size: 17px; font-weight: 700; }.promotion-stats text + text { margin-top: 5px; color: #697386; font-size: 10px; font-weight: 500; }.share-grid { display: grid; grid-template-columns: repeat(3,minmax(0,1fr)); gap: 10px; }.share-grid button { min-width: 0; height: 76px; margin: 0; padding: 8px; border: 1px solid #e5eaf6; border-radius: 10px; background: #fff; }.share-grid button:disabled { opacity: .45; }.share-grid button text:first-child { display: inline-flex; width: 30px; height: 30px; margin: 0 auto; align-items: center; justify-content: center; border-radius: 9px; font-size: 10px; font-weight: 700; }.share-grid button text + text { margin-top: 6px; font-size: 10px; font-weight: 600; }.share-grid .green { color: #079455; background: #ecfdf5; }.share-grid .purple { color: #5b55d6; background: #f4f3ff; }.share-grid .orange { color: #ff6b1a; background: #fff7ed; }
.mine-modal-layer { position: fixed; z-index: 80; inset: 0; background: rgba(15, 23, 42, .46); }.mine-bottom-sheet { position: absolute; right: 0; bottom: 0; left: 0; padding: 18px 20px calc(18px + env(safe-area-inset-bottom)); border-radius: 22px 22px 0 0; background: #fff; box-shadow: 0 -12px 36px rgba(15,23,42,.18); }.sheet-handle { width: 36px; height: 4px; margin: -8px auto 16px; border-radius: 2px; background: #d0d5dd; }.sheet-title-row { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }.sheet-title-row view text:first-child { font-size: 18px; font-weight: 700; }.sheet-title-row view text + text { margin-top: 5px; color: #697386; font-size: 10px; }.recommend-tag { padding: 5px 10px; border: 1px solid #ffb27a; border-radius: 999px; color: #ff6b1a; background: #fff2e8; font-size: 10px; font-weight: 600; }.purchase-summary { position: relative; margin-top: 16px; padding: 13px; border-radius: 12px; background: #f4f3ff; }.purchase-summary.agent { background: #fff2e8; }.purchase-summary text { display: block; }.purchase-summary text:first-child { color: #697386; font-size: 10px; }.purchase-summary text:nth-child(2) { margin-top: 6px; font-size: 18px; font-weight: 700; }.purchase-summary text:last-child { position: absolute; right: 13px; bottom: 13px; color: #5b55d6; font-size: 16px; font-weight: 700; }.purchase-summary.agent text:last-child { color: #ff6b1a; }.payment-method { display: flex; margin-top: 14px; padding: 10px 12px; align-items: center; justify-content: space-between; border: 1px solid #e5eaf6; border-radius: 10px; }.payment-method view text { font-size: 11px; font-weight: 600; }.payment-method view text + text { margin-top: 3px; color: #697386; font-size: 10px; font-weight: 500; }.payment-method > text { display: inline-flex; width: 22px; height: 22px; align-items: center; justify-content: center; border-radius: 50%; color: #fff; background: #079455; font-size: 11px; }.purchase-note { display: flex; margin: 13px 0; color: #697386; font-size: 10px; }.purchase-note-link { color: #5b55d6; }.sheet-primary { width: 100%; height: 48px; margin: 0; border-radius: 12px; color: #fff; background: #5b55d6; font-size: 13px; font-weight: 700; }.sheet-primary.agent { background: #ff6b1a; }.sheet-primary:disabled { opacity: .58; }
.logout-title-row { display: flex; align-items: center; gap: 12px; }.logout-title-row view text:first-child { font-size: 18px; font-weight: 700; }.logout-title-row view text + text { margin-top: 4px; color: #697386; font-size: 10px; }.logout-notice { margin-top: 16px; padding: 12px; border-radius: 10px; background: #f7f8fc; }.logout-notice text { font-size: 11px; font-weight: 600; }.logout-notice text + text { margin-top: 4px; color: #697386; font-size: 10px; font-weight: 500; }.logout-actions { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 12px; margin-top: 16px; }.logout-actions button { height: 46px; margin: 0; border: 1px solid #e5eaf6; border-radius: 12px; color: #111827; background: #fff; font-size: 13px; font-weight: 700; }.logout-actions button.danger-action { color: #fff; border-color: #e73b3b; background: #e73b3b; }
@media (max-width: 340px) { .mine-recharge-row { flex-wrap: wrap; }.mine-recharge-row button { min-width: 92px; }.step-row { grid-template-columns: 1fr; }.step-row.four { grid-template-columns: repeat(2,minmax(0,1fr)); } }
</style>
