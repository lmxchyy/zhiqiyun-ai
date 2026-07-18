<template>
  <AdminDataTable
    title="订单履约中心"
    :rows="rows"
    :columns="orderColumns"
    :column-labels="columnLabels"
    :toolbar-actions="toolbarActions"
    :row-actions="rowActions"
    :batch-actions="rowActions"
    persistence-key="orders"
    v-model:search-keyword="searchKeyword"
    v-model:status-filter="statusFilter"
    :status-filter-options="statusFilterOptions"
    :loading="saving"
    row-open-label="履约时间轴"
    :is-status-column="isStatusColumn"
    :status-type="statusType"
    :status-label="statusLabel"
    :format-cell="formatCell"
    :visible-row-actions="visibleRowActions"
    :label-for-row-action="labelForRowAction"
    @run-action="(action, row) => $emit('run-action', action, row)"
    @batch-action="(action, rows) => $emit('batch-action', action, rows)"
    @open-row="openOrder"
  />

  <el-drawer v-model="drawerOpen" size="min(920px, 92vw)" destroy-on-close>
    <template #header>
      <div class="workspace-drawer-title">
        <div><span>订单履约时间轴</span><strong>{{ detail.item?.orderNo || selectedOrder?.orderNo || selectedOrder?.id || '-' }}</strong></div>
        <el-tag :type="statusType(detail.item?.status)">{{ statusLabel(detail.item?.status) }}</el-tag>
      </div>
    </template>
    <el-skeleton v-if="loading" :rows="10" animated />
    <el-alert v-else-if="errorMessage" type="error" :title="errorMessage" :closable="false" show-icon><template #default><el-button size="small" @click="reloadOrder">重试</el-button></template></el-alert>
    <section v-else class="order-workspace">
      <div class="order-summary">
        <article><span>客户</span><strong>{{ detail.item?.customer || detail.item?.userId || '-' }}</strong><small>{{ detail.item?.userId }}</small></article>
        <article><span>商品/套餐</span><strong>{{ detail.item?.plan || '-' }}</strong><small>{{ detail.item?.orderType || detail.item?.businessOrderType || '-' }}</small></article>
        <article><span>实付金额</span><strong>{{ formatMoney(detail.item?.amountCents) }}</strong><small>{{ detail.item?.paymentMethod || '待确认渠道' }}</small></article>
        <article><span>权益到账</span><strong>{{ formatNumber(detail.item?.tokenGrantAmount || detail.item?.tokenAmount) }}</strong><small>{{ detail.item?.fulfillmentStatus || '等待履约' }}</small></article>
      </div>

      <el-card shadow="never" class="timeline-card">
        <template #header><div class="timeline-head"><strong>履约进度</strong><small>支付、权益与分佣状态来自服务端真实记录</small></div></template>
        <ol class="fulfillment-timeline" aria-label="订单履约进度">
          <li v-for="step in detail.timeline || []" :key="step.id" :class="`is-${step.state || 'pending'}`">
            <i aria-hidden="true"></i>
            <div><strong>{{ step.title }}</strong><span>{{ step.description }}</span><small>{{ step.occurredAt || stateLabel(step.state) }}</small></div>
          </li>
        </ol>
      </el-card>

      <el-tabs v-model="activeTab">
        <el-tab-pane :label="`支付记录 ${detail.payments?.length || 0}`" name="payments"><el-table :data="detail.payments || []" height="260" stripe empty-text="暂无支付记录"><el-table-column prop="id" label="支付记录 ID" min-width="190" /><el-table-column prop="channel" label="渠道" /><el-table-column prop="amount" label="金额"><template #default="scope">{{ formatMoney(scope.row.amount) }}</template></el-table-column><el-table-column prop="status" label="状态" /><el-table-column prop="createdAt" label="时间" min-width="180" /></el-table><el-table :data="detail.paymentEvents || []" height="220" stripe empty-text="暂无支付通知"><el-table-column prop="provider" label="服务商" /><el-table-column prop="transactionId" label="交易号" min-width="180" /><el-table-column prop="verified" label="验签"><template #default="scope">{{ scope.row.verified ? '通过' : '未通过' }}</template></el-table-column><el-table-column prop="status" label="状态" /><el-table-column prop="createdAt" label="通知时间" min-width="180" /></el-table></el-tab-pane>
        <el-tab-pane :label="`权益流水 ${detail.tokenRecords?.length || 0}`" name="entitlements"><el-table :data="detail.tokenRecords || []" height="390" stripe empty-text="暂无权益流水"><el-table-column prop="changeType" label="类型" /><el-table-column prop="amount" label="变动积分" /><el-table-column prop="balanceAfter" label="变动后余额" /><el-table-column prop="remark" label="备注" min-width="220" /><el-table-column prop="createdAt" label="时间" min-width="180" /></el-table></el-tab-pane>
        <el-tab-pane :label="`分佣记录 ${detail.commissions?.length || 0}`" name="commissions"><el-table :data="detail.commissions || []" height="390" stripe empty-text="暂无分佣记录"><el-table-column prop="receiverType" label="接收方类型" /><el-table-column prop="receiverId" label="接收方" min-width="170" /><el-table-column prop="amountCents" label="分佣金额"><template #default="scope">{{ formatMoney(scope.row.amountCents) }}</template></el-table-column><el-table-column prop="status" label="状态" /><el-table-column prop="settleStatus" label="结算状态" /><el-table-column prop="createdAt" label="时间" min-width="180" /></el-table></el-tab-pane>
      </el-tabs>
    </section>
  </el-drawer>
</template>

<script setup lang="ts">
import { reactive, ref } from "vue";
import { adminWorkspaceApi, type OrderTimelineResponse, type WorkspaceRecord } from "../../api/adminWorkspaces";
import AdminDataTable from "./AdminDataTable.vue";

type Action = { action: string; label: string };
defineProps<{ rows: WorkspaceRecord[]; saving: boolean; toolbarActions: Action[]; rowActions: Action[]; columnLabels: Record<string, string>; statusFilterOptions: Array<{ label: string; value: string }>; isStatusColumn: (column: string) => boolean; statusType: (value: unknown) => any; statusLabel: (value: unknown) => string; formatCell: (value: unknown, column: string) => unknown; visibleRowActions: (row: WorkspaceRecord) => Action[]; labelForRowAction: (action: Action, row: WorkspaceRecord) => string }>();
defineEmits<{ "run-action": [action: string, row?: WorkspaceRecord]; "batch-action": [action: string, rows: WorkspaceRecord[]] }>();

const orderColumns = ["orderNo", "customer", "plan", "orderType", "amountCents", "status", "fulfillmentStatus", "createdAt"];
const searchKeyword = ref("");
const statusFilter = ref("ALL");
const drawerOpen = ref(false);
const loading = ref(false);
const errorMessage = ref("");
const activeTab = ref("payments");
const selectedOrder = ref<WorkspaceRecord | null>(null);
const detail = reactive<Partial<OrderTimelineResponse>>({});

async function openOrder(row: WorkspaceRecord) { selectedOrder.value = row; drawerOpen.value = true; activeTab.value = "payments"; await reloadOrder(); }
async function reloadOrder() {
  const id = String(selectedOrder.value?.id || selectedOrder.value?.orderNo || "");
  if (!id) return;
  loading.value = true; errorMessage.value = "";
  try { Object.assign(detail, await adminWorkspaceApi.orderTimeline(id)); }
  catch (error) { errorMessage.value = error instanceof Error ? error.message : "订单履约时间轴加载失败"; }
  finally { loading.value = false; }
}
function formatNumber(value: unknown) { return new Intl.NumberFormat("zh-CN").format(Number(value || 0)); }
function formatMoney(value: unknown) { return `¥${(Number(value || 0) / 100).toLocaleString("zh-CN", { minimumFractionDigits: 2 })}`; }
function stateLabel(value: unknown) { return ({ complete: "已完成", current: "处理中", error: "异常", pending: "等待处理" } as Record<string, string>)[String(value || "pending")] || "等待处理"; }
</script>

<style scoped>
.workspace-drawer-title { display: flex; align-items: center; justify-content: space-between; width: 100%; gap: 18px; }.workspace-drawer-title > div { display: grid; gap: 4px; }.workspace-drawer-title span { color: var(--admin-muted); font-size: 12px; }.workspace-drawer-title strong { color: var(--admin-text); font-size: 20px; }
.order-workspace { display: grid; gap: 18px; }.order-summary { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }.order-summary article { padding: 15px; border: 1px solid var(--admin-border); border-radius: 10px; background: var(--admin-panel); }.order-summary span,.order-summary small { display: block; color: var(--admin-muted); }.order-summary strong { display: block; overflow: hidden; margin: 6px 0; color: var(--admin-text); font-size: 18px; text-overflow: ellipsis; white-space: nowrap; }.timeline-head { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }.timeline-head small { color: var(--admin-muted); }.fulfillment-timeline { display: grid; grid-template-columns: repeat(5, 1fr); margin: 0; padding: 12px 0 0; list-style: none; }.fulfillment-timeline li { position: relative; min-width: 0; padding: 0 12px; text-align: center; }.fulfillment-timeline li::before { position: absolute; top: 8px; right: 50%; left: -50%; height: 2px; background: var(--admin-border); content: ""; }.fulfillment-timeline li:first-child::before { display: none; }.fulfillment-timeline i { position: relative; z-index: 1; display: block; width: 18px; height: 18px; margin: 0 auto 12px; border: 4px solid #cbd5e1; border-radius: 50%; background: white; }.fulfillment-timeline div { display: grid; gap: 4px; }.fulfillment-timeline strong { font-size: 13px; }.fulfillment-timeline span,.fulfillment-timeline small { color: var(--admin-muted); font-size: 11px; line-height: 1.5; }.fulfillment-timeline .is-complete i { border-color: #22c55e; }.fulfillment-timeline .is-complete::before { background: #86efac; }.fulfillment-timeline .is-current i { border-color: var(--color-primary); box-shadow: 0 0 0 4px var(--color-primary-light); }.fulfillment-timeline .is-error i { border-color: #ef4444; }.fulfillment-timeline .is-error strong { color: #dc2626; }
@media (max-width: 900px) { .order-summary { grid-template-columns: repeat(2, 1fr); }.fulfillment-timeline { grid-template-columns: 1fr; gap: 0; }.fulfillment-timeline li { display: grid; grid-template-columns: 24px 1fr; padding: 10px 0; text-align: left; }.fulfillment-timeline li::before { top: -50%; right: auto; bottom: 50%; left: 8px; width: 2px; height: auto; }.fulfillment-timeline i { margin: 0; }.fulfillment-timeline div { padding-left: 10px; } }
</style>
