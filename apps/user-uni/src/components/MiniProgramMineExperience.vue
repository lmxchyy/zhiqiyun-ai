<template>
  <view class="mine-experience">
    <template v-if="view === 'overview'">
      <view class="mine-v5-profile-card">
        <AppImage v-if="headerBackground" class="mine-v5-header-background" :src="headerBackground" alt="个人中心背景" width="100%" height="100%" radius="12px" :lazy-load="false" />
        <view class="mine-v5-avatar"><AppImage v-if="avatarUrl || avatarFallback" :src="avatarUrl" :fallback="avatarFallback" local-fallback="/static/fallbacks/default-avatar.jpg" :alt="displayName" width="58px" height="58px" radius="50%" :lazy-load="false" /><text v-else>{{ avatarText }}</text></view>
        <view class="mine-v5-profile-copy">
          <text class="mine-v5-profile-name">{{ displayName }}</text>
          <text class="mine-v5-profile-type">微信用户</text>
        </view>
        <button class="mine-v5-edit" @click="openFeaturePage(miniProgramFeaturePages.userProfileEdit)">编辑资料 ›</button>
      </view>

      <button class="mine-v5-agent-entry" @click="hasAgentRole ? $emit('switch-agent') : $emit('navigate', 'agent-upgrade')">
        <view class="mine-v5-agent-copy">
          <text>{{ hasAgentRole ? "代理商角色已开通" : "代理商角色未开通" }}</text>
          <text>{{ hasAgentRole ? "查看推广、客户与订单分润" : "推广客户并获得订单分润" }}</text>
        </view>
        <text class="mine-v5-agent-action">{{ hasAgentRole ? "进入工作台 ›" : "升级 ›" }}</text>
      </button>

      <view class="mine-v5-balance-card" @click="openWallet">
        <AppImage v-if="memberBackground" class="mine-v5-member-background" :src="memberBackground" alt="会员背景" width="100%" height="100%" radius="12px" />
        <text class="mine-v5-balance-title">我的剩余点数</text>
        <view class="mine-v5-balance-value"><text>{{ formatNumber(pointBalance) }}</text><text>点</text></view>
        <button class="mine-v5-recharge" @click.stop="openFeaturePage(miniProgramFeaturePages.userRechargePlans)">去充值</button>
      </view>

      <view class="mine-v5-menu-card">
        <button class="mine-v5-menu-row" @click="openFeaturePage(miniProgramFeaturePages.userRechargePlans)">
          <text class="mine-v5-menu-icon purple">充</text><text class="mine-v5-menu-label">充值中心</text><text class="mine-v5-chevron">›</text>
        </button>
        <button class="mine-v5-menu-row" @click="openTaskRecords">
          <text class="mine-v5-menu-icon blue">任</text><text class="mine-v5-menu-label">任务记录</text><text class="mine-v5-chevron">›</text>
        </button>
        <button class="mine-v5-menu-row" @click="$emit('navigate', 'usage-details')">
          <text class="mine-v5-menu-icon violet">点</text><text class="mine-v5-menu-label">点数明细</text><text class="mine-v5-chevron">›</text>
        </button>
        <button class="mine-v5-menu-row" @click="openFeaturePage(miniProgramFeaturePages.userOrders)">
          <text class="mine-v5-menu-icon orange">单</text><text class="mine-v5-menu-label">我的订单</text><text class="mine-v5-chevron">›</text>
        </button>
        <button class="mine-v5-menu-row" @click="$emit('navigate', 'invite-promotion')">
          <text class="mine-v5-menu-icon pink">邀</text><text class="mine-v5-menu-label">邀请与推广</text><text class="mine-v5-chevron">›</text>
        </button>
        <button class="mine-v5-menu-row" @click="$emit('navigate', 'role-permissions')">
          <text class="mine-v5-menu-icon blue">权</text><text class="mine-v5-menu-label">角色与权限</text><text class="mine-v5-chevron">›</text>
        </button>
        <button class="mine-v5-menu-row" @click="openFeaturePage(miniProgramFeaturePages.userSettings)">
          <text class="mine-v5-menu-icon slate">设</text><text class="mine-v5-menu-label">账户设置</text><text class="mine-v5-chevron">›</text>
        </button>
      </view>
    </template>

    <template v-else>
      <view class="mine-detail-header">
        <button class="mine-back-button" aria-label="返回我的" @click="$emit('navigate', 'overview')">‹</button>
        <view><text>{{ detailTitle }}</text><text>{{ detailSubtitle }}</text></view>
      </view>

      <view v-if="view === 'recharge-history'" class="mine-detail-stack">
        <view class="record-summary dark"><text>累计充值</text><text>{{ formatCurrency(totalRechargeCents) }}</text><text>累计到账 {{ formatNumber(totalRechargePoints) }} 点</text><button @click="$emit('invoice')">电子凭证</button></view>
        <view class="filter-strip"><button :class="{ active: orderFilter === 'all' }" @click="orderFilter = 'all'">全部</button><button :class="{ active: orderFilter === 'success' }" @click="orderFilter = 'success'">成功</button><button :class="{ active: orderFilter === 'refund' }" @click="orderFilter = 'refund'">退款</button><text @click="cycleHistoryRange">{{ historyRangeLabel }}⌄</text></view>
        <view class="record-list">
          <view v-if="filteredOrders.length" v-for="order in filteredOrders.slice(0, 8)" :key="rowKey(order)" class="record-row" @click="openOrderDetail(order)">
            <text :class="['record-icon', statusTone(order)]">{{ statusTone(order) === 'success' ? '✓' : '单' }}</text>
            <view><text>{{ orderTitle(order) }}</text><text>{{ orderMeta(order) }}</text></view>
            <text :class="['record-value', statusTone(order)]">{{ orderValue(order) }}</text>
          </view>
          <view v-else class="mine-empty"><text>暂无充值订单</text><text>完成充值后，到账与退款状态会显示在这里。</text></view>
        </view>
        <button class="secondary-action" @click="$emit('invoice')">查看电子凭证</button>
      </view>

      <view v-else-if="view === 'usage-details'" class="mine-detail-stack">
        <view class="usage-summary">
          <text>本月消耗</text><text>{{ formatNumber(monthlyPointCost) }} 点</text><text>当前余额 {{ formatNumber(pointBalance) }} 点</text>
          <view v-for="item in usageBreakdown" :key="item.label" class="usage-bar-row"><text>{{ item.label }}</text><view><view :class="item.tone" :style="{ width: `${item.percent}%` }"></view></view></view>
        </view>
        <view class="filter-strip"><button class="active" @click="cycleUsageType">{{ usageFilterLabel }}</button><button :class="{ active: usageCurrentMonthOnly }" @click="usageCurrentMonthOnly = !usageCurrentMonthOnly">{{ usageCurrentMonthOnly ? "本月" : "全部时间" }}</button><text @click="$emit('export-usage')">导出明细</text></view>
        <view class="record-list">
          <view v-if="filteredUsageRecords.length" v-for="record in filteredUsageRecords.slice(0, 8)" :key="rowKey(record)" class="record-row" @click="openUsageRecordDetail(record)">
            <text class="record-icon purple">{{ usageIcon(record) }}</text>
            <view><text>{{ usageTitle(record) }}</text><text>{{ formatDate(rowDate(record)) }}</text></view>
            <text class="record-value purple">-{{ formatNumber(rowPointCost(record)) }} 点</text>
          </view>
          <view v-else class="mine-empty"><text>暂无消耗明细</text><text>生成图片、视频或 PPT 后，将按模型记录扣点。</text></view>
        </view>
        <view class="billing-note"><text>计费规则</text><text>基础价 × 数量 × 参数倍率，最终点数向上取整。</text></view>
      </view>

      <view v-else-if="view === 'role-permissions'" class="mine-detail-stack">
        <view class="role-hero"><text>当前角色</text><text>{{ currentRoleLabel }}</text><text>页面与操作按当前角色权限动态展示</text><text class="role-status">使用中</text><button v-if="!hasAgentRole" @click="$emit('navigate', 'agent-upgrade')">代理商角色未开通　去升级 ›</button><button v-else @click="$emit('switch-agent')">切换到代理商 ›</button></view>
        <view class="mine-panel compact"><text class="mine-section-title">已授权角色</text><view v-for="role in roleRows" :key="role.id" class="role-row"><text class="mine-menu-icon green">✓</text><view><text>{{ role.label }}</text><text>{{ role.description }}</text></view><text :class="role.id === currentRole ? 'success-text' : 'muted-text'">{{ role.id === currentRole ? "当前" : "已启用" }}</text></view></view>
        <view class="mine-panel compact"><text class="mine-section-title">当前角色权限</text><view v-for="permission in grantedPermissionRows" :key="permission" class="permission-row"><text>{{ permission }}</text><text class="success-text">可用</text></view></view>
        <button v-if="!hasAgentRole" class="orange-action" @click="$emit('navigate', 'agent-upgrade')">成为代理商</button>
        <button v-else class="primary-action" @click="$emit('switch-agent')">进入代理工作台</button>
      </view>

      <view v-else class="mine-detail-stack">
        <view class="invite-hero">
          <text>我的推广码</text><text>{{ inviteCode }}</text><text class="agent-pill">{{ hasAgentRole ? agentLevelLabel : "待开通" }}</text>
          <view class="invite-link"><text>{{ inviteLink }}</text><button @click="$emit('copy-invite')">复制</button></view>
          <text>{{ hasAgentRole ? "客户通过链接注册后将自动绑定" : "成为代理商后可启用客户绑定与分润" }}</text>
        </view>
        <view class="promotion-stats"><view><text>{{ stat('visits') }}</text><text>访问</text></view><view><text>{{ stat('registrations') }}</text><text>注册</text></view><view><text>{{ stat('orders') }}</text><text>成交</text></view><view><text class="orange-text">{{ conversionRate }}%</text><text>转化率</text></view></view>
        <view class="mine-panel compact"><text class="mine-section-title">选择分享方式</text><view class="share-grid"><button :disabled="!hasAgentRole" open-type="share"><text class="green">微</text><text>微信好友</text></button><button :disabled="!hasAgentRole" open-type="share"><text class="purple">圈</text><text>朋友圈</text></button><button :disabled="!hasAgentRole" @click="$emit('poster')"><text class="orange">图</text><text>生成海报</text></button></view></view>
        <view class="mine-panel compact"><text class="mine-section-title">推广流程</text><view class="step-row four"><view v-for="(step, index) in promotionSteps" :key="step"><text :class="['step-index', { active: index === 0 }]">{{ index + 1 }}</text><text>{{ step }}</text></view></view></view>
        <button v-if="hasAgentRole" class="primary-action" open-type="share">分享专属链接</button>
        <button v-else class="orange-action" @click="$emit('navigate', 'agent-upgrade')">成为代理商后开启推广</button>
      </view>
    </template>

    <view v-if="purchase" class="mine-modal-layer" @click="$emit('close-purchase')">
      <view class="mine-bottom-sheet" @click.stop>
        <view class="sheet-handle"></view>
        <view class="sheet-title-row"><view><text>{{ purchase.kind === 'agent' ? '开通代理套餐' : `充值 ${purchase.amountCents / 100} 元` }}</text><text>{{ purchase.kind === 'agent' ? '获得代理角色、推广工具与分润资格' : `支付成功后即时到账 ${formatNumber(purchase.points)} 点` }}</text></view><text v-if="purchase.recommended" class="recommend-tag">推荐</text></view>
        <view :class="['purchase-summary', { agent: purchase.kind === 'agent' }]"><text>{{ purchase.kind === 'agent' ? '代理套餐' : '到账点数' }}</text><text>{{ purchase.kind === 'agent' ? '年度代理权益' : `${formatNumber(purchase.points)} 点` }}</text><text>{{ formatCurrency(purchase.amountCents) }}</text></view>
        <view class="payment-method"><view><text>微信支付</text><text>推荐使用当前微信账户完成支付</text></view><text>✓</text></view>
        <text class="purchase-note">{{ purchase.kind === 'agent' ? '开通即表示同意《代理商服务协议》' : '充值点数到账后不可提现，可用于平台 AI 服务' }}</text>
        <button :class="['sheet-primary', { agent: purchase.kind === 'agent' }]" :disabled="purchaseSubmitting" @click="$emit('confirm-purchase')">{{ purchaseSubmitting ? "正在创建订单..." : purchase.kind === 'agent' ? `支付 ${formatCurrency(purchase.amountCents)} 并开通` : `确认支付 ${formatCurrency(purchase.amountCents)}` }}</button>
      </view>
    </view>

    <view v-if="logoutConfirm" class="mine-modal-layer" @click="$emit('close-logout')">
      <view class="mine-bottom-sheet logout-sheet" @click.stop>
        <view class="sheet-handle"></view>
        <view class="logout-title-row"><text class="mine-menu-icon danger">退</text><view><text>退出当前账号？</text><text>退出后需要重新登录才能继续使用</text></view></view>
        <view class="logout-notice"><text>本机登录状态将被清除</text><text>创作记录与钱包数据不会删除</text></view>
        <view class="logout-actions"><button @click="$emit('close-logout')">取消</button><button class="danger-action" @click="$emit('confirm-logout')">退出登录</button></view>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import type { MinePurchaseOption, MineView } from "../types";
import type { AppRole } from "../types";
import { miniProgramFeaturePages, miniProgramRolePages } from "../config/miniProgramPages";
import { roleLabels } from "../config/permissions";
import AppImage from "./AppImage.vue";

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

const recharge100: MinePurchaseOption = { kind: "recharge", id: "recharge_100", amountCents: 10000, points: 10000 };
const recharge400: MinePurchaseOption = { kind: "recharge", id: "recharge_400", amountCents: 40000, points: 40000, recommended: true };
const promotionSteps = ["分享链接", "客户注册", "完成订单", "获得分润"];
const orderFilter = ref<"all" | "success" | "refund">("all");
const historyRange = ref<30 | 90 | 0>(90);
const usageType = ref<"all" | "image" | "video" | "ppt">("all");
const usageCurrentMonthOnly = ref(true);
const avatarText = computed(() => props.displayName.trim().slice(0, 1) || "知");
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
.mine-modal-layer { position: fixed; z-index: 80; inset: 0; background: rgba(15, 23, 42, .46); }.mine-bottom-sheet { position: absolute; right: 0; bottom: 0; left: 0; padding: 18px 20px calc(18px + env(safe-area-inset-bottom)); border-radius: 22px 22px 0 0; background: #fff; box-shadow: 0 -12px 36px rgba(15,23,42,.18); }.sheet-handle { width: 36px; height: 4px; margin: -8px auto 16px; border-radius: 2px; background: #d0d5dd; }.sheet-title-row { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }.sheet-title-row view text:first-child { font-size: 18px; font-weight: 700; }.sheet-title-row view text + text { margin-top: 5px; color: #697386; font-size: 10px; }.recommend-tag { padding: 5px 10px; border: 1px solid #ffb27a; border-radius: 999px; color: #ff6b1a; background: #fff2e8; font-size: 10px; font-weight: 600; }.purchase-summary { position: relative; margin-top: 16px; padding: 13px; border-radius: 12px; background: #f4f3ff; }.purchase-summary.agent { background: #fff2e8; }.purchase-summary text { display: block; }.purchase-summary text:first-child { color: #697386; font-size: 10px; }.purchase-summary text:nth-child(2) { margin-top: 6px; font-size: 18px; font-weight: 700; }.purchase-summary text:last-child { position: absolute; right: 13px; bottom: 13px; color: #5b55d6; font-size: 16px; font-weight: 700; }.purchase-summary.agent text:last-child { color: #ff6b1a; }.payment-method { display: flex; margin-top: 14px; padding: 10px 12px; align-items: center; justify-content: space-between; border: 1px solid #e5eaf6; border-radius: 10px; }.payment-method view text { font-size: 11px; font-weight: 600; }.payment-method view text + text { margin-top: 3px; color: #697386; font-size: 10px; font-weight: 500; }.payment-method > text { display: inline-flex; width: 22px; height: 22px; align-items: center; justify-content: center; border-radius: 50%; color: #fff; background: #079455; font-size: 11px; }.purchase-note { display: block; margin: 13px 0; color: #697386; font-size: 10px; }.sheet-primary { width: 100%; height: 48px; margin: 0; border-radius: 12px; color: #fff; background: #5b55d6; font-size: 13px; font-weight: 700; }.sheet-primary.agent { background: #ff6b1a; }.sheet-primary:disabled { opacity: .58; }
.logout-title-row { display: flex; align-items: center; gap: 12px; }.logout-title-row view text:first-child { font-size: 18px; font-weight: 700; }.logout-title-row view text + text { margin-top: 4px; color: #697386; font-size: 10px; }.logout-notice { margin-top: 16px; padding: 12px; border-radius: 10px; background: #f7f8fc; }.logout-notice text { font-size: 11px; font-weight: 600; }.logout-notice text + text { margin-top: 4px; color: #697386; font-size: 10px; font-weight: 500; }.logout-actions { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 12px; margin-top: 16px; }.logout-actions button { height: 46px; margin: 0; border: 1px solid #e5eaf6; border-radius: 12px; color: #111827; background: #fff; font-size: 13px; font-weight: 700; }.logout-actions button.danger-action { color: #fff; border-color: #e73b3b; background: #e73b3b; }
@media (max-width: 340px) { .mine-recharge-row { flex-wrap: wrap; }.mine-recharge-row button { min-width: 92px; }.step-row { grid-template-columns: 1fr; }.step-row.four { grid-template-columns: repeat(2,minmax(0,1fr)); } }
</style>
