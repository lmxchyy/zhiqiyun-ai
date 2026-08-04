<template>
  <AdminDataTable
    title="客户 360°"
    :rows="rows"
    :columns="customerColumns"
    :column-labels="columnLabels"
    :toolbar-actions="toolbarActions"
    :row-actions="rowActions"
    :batch-actions="rowActions"
    persistence-key="customers"
    v-model:search-keyword="searchKeyword"
    v-model:status-filter="statusFilter"
    :status-filter-options="statusFilterOptions"
    :loading="saving"
    row-open-label="查看 360°"
    :is-status-column="isStatusColumn"
    :status-type="statusType"
    :status-label="statusLabel"
    :format-cell="formatCell"
    :visible-row-actions="visibleRowActions"
    :label-for-row-action="labelForRowAction"
    @run-action="(action, row) => $emit('run-action', action, row)"
    @batch-action="(action, rows) => $emit('batch-action', action, rows)"
    @open-row="openCustomer"
  />

  <el-drawer v-model="drawerOpen" size="min(980px, 92vw)" class="customer-360-drawer" destroy-on-close>
    <template #header>
      <div class="workspace-drawer-title">
        <div><span>客户 360°</span><strong>{{ detail.profile?.name || selectedCustomer?.name || '-' }}</strong></div>
        <el-tag :type="statusType(detail.profile?.status)">{{ statusLabel(detail.profile?.status) }}</el-tag>
      </div>
    </template>
    <el-skeleton v-if="loading" :rows="10" animated />
    <el-alert v-else-if="errorMessage" type="error" :title="errorMessage" :closable="false" show-icon>
      <template #default><el-button size="small" @click="reloadCustomer">重试</el-button></template>
    </el-alert>
    <section v-else class="customer-360-content">
      <div class="workspace-metrics">
        <article><span>可用积分</span><strong>{{ formatNumber(detail.wallet?.available) }}</strong><small>冻结 {{ formatNumber(detail.wallet?.frozen) }}</small></article>
        <article><span>累计订单</span><strong>{{ formatNumber(detail.summary?.orders) }}</strong><small>已支付 {{ formatNumber(detail.summary?.paidOrders) }}</small></article>
        <article><span>累计支付</span><strong>{{ formatMoney(detail.summary?.paidAmountCents) }}</strong><small>{{ detail.profile?.plan || '未配置套餐' }}</small></article>
        <article><span>生成任务</span><strong>{{ formatNumber(detail.summary?.generationTasks) }}</strong><small>积分流水 {{ formatNumber(detail.summary?.tokenRecords) }}</small></article>
      </div>
      <el-tabs v-model="activeTab">
        <el-tab-pane v-if="canAccessPoints" label="赠送积分与批次" name="personal-points">
          <CustomerPointActions
            v-if="selectedCustomer?.id"
            :user-id="String(selectedCustomer.id)"
            :user-name="String(detail.profile?.name || selectedCustomer?.name || '')"
            :role="role"
            :permissions="permissions || []"
          />
        </el-tab-pane>
        <el-tab-pane label="身份与权益" name="business-identity">
          <IdentityEntitlementsPanel
            v-if="selectedCustomer?.id"
            :user-id="String(selectedCustomer.id)"
            :user-name="String(detail.profile?.name || selectedCustomer?.name || '')"
            :permissions="permissions"
          />
        </el-tab-pane>
        <el-tab-pane label="客户概览" name="overview">
          <div class="workspace-detail-grid">
            <el-card shadow="never"><template #header><strong>基本资料</strong></template><el-descriptions :column="2" border><el-descriptions-item label="客户 ID">{{ detail.profile?.id }}</el-descriptions-item><el-descriptions-item label="角色">{{ detail.profile?.role }}</el-descriptions-item><el-descriptions-item label="邮箱">{{ detail.profile?.email }}</el-descriptions-item><el-descriptions-item label="套餐">{{ detail.profile?.plan }}</el-descriptions-item><el-descriptions-item label="订阅到期">{{ detail.profile?.subscriptionExpiresAt || '-' }}</el-descriptions-item><el-descriptions-item label="模型路由">{{ detail.profile?.modelRoute || '未绑定' }}</el-descriptions-item></el-descriptions></el-card>
            <el-card shadow="never"><template #header><strong>客户归属</strong></template><el-descriptions :column="1" border><el-descriptions-item label="健康状态">{{ detail.attribution?.item?.healthStatus || '未归属' }}</el-descriptions-item><el-descriptions-item label="直属伙伴">{{ detail.attribution?.item?.directAgent?.name || '-' }}</el-descriptions-item><el-descriptions-item label="上级伙伴">{{ detail.attribution?.item?.parentAgent?.name || '-' }}</el-descriptions-item><el-descriptions-item label="运营中心">{{ detail.attribution?.item?.operationCenter?.name || '-' }}</el-descriptions-item></el-descriptions></el-card>
          </div>
        </el-tab-pane>
        <el-tab-pane :label="`订单 ${detail.orders?.length || 0}`" name="orders"><el-table :data="detail.orders || []" height="430" stripe empty-text="暂无订单"><el-table-column prop="orderNo" label="订单号" min-width="180" /><el-table-column prop="plan" label="商品/套餐" min-width="150" /><el-table-column prop="amountCents" label="金额"><template #default="scope">{{ formatMoney(scope.row.amountCents) }}</template></el-table-column><el-table-column prop="status" label="状态"><template #default="scope"><el-tag :type="statusType(scope.row.status)">{{ statusLabel(scope.row.status) }}</el-tag></template></el-table-column><el-table-column prop="fulfillmentStatus" label="履约状态" min-width="120" /><el-table-column prop="createdAt" label="创建时间" min-width="180" /></el-table></el-tab-pane>
        <el-tab-pane label="积分与计费" name="wallet"><el-table :data="detail.tokenRecords || []" height="260" stripe empty-text="暂无积分流水"><el-table-column prop="changeType" label="变动类型" /><el-table-column prop="amount" label="变动积分" /><el-table-column prop="balanceAfter" label="变动后余额" /><el-table-column prop="remark" label="备注" min-width="200" /><el-table-column prop="createdAt" label="时间" min-width="180" /></el-table><el-table :data="detail.billingEvents || []" height="240" stripe empty-text="暂无计费事件"><el-table-column prop="moduleCode" label="业务模块" /><el-table-column prop="model" label="模型" /><el-table-column prop="pointCost" label="积分消耗" /><el-table-column prop="status" label="状态" /><el-table-column prop="occurredAt" label="时间" min-width="180" /></el-table></el-tab-pane>
        <el-tab-pane label="登录与身份" name="identity"><el-descriptions :column="2" border><el-descriptions-item label="手机号">{{ detail.identity?.mobileMasked || '未绑定' }}</el-descriptions-item><el-descriptions-item label="微信">{{ detail.identity?.wechatLinked ? detail.identity?.wechatOpenIdMasked : '未绑定' }}</el-descriptions-item><el-descriptions-item label="密码登录">{{ detail.identity?.passwordLoginEnabled ? '已启用' : '未启用' }}</el-descriptions-item><el-descriptions-item label="登录方式">{{ (detail.identity?.loginMethods || []).join('、') || '-' }}</el-descriptions-item></el-descriptions><el-table :data="detail.mergeRequests || []" height="260" stripe empty-text="暂无账号合并工单"><el-table-column prop="id" label="工单 ID" min-width="190" /><el-table-column prop="conflictCode" label="冲突类型" /><el-table-column prop="status" label="状态" /><el-table-column prop="createdAt" label="创建时间" min-width="180" /></el-table></el-tab-pane>
        <el-tab-pane label="使用活动" name="activity"><el-table :data="detail.generationTasks || []" height="430" stripe empty-text="暂无生成任务"><el-table-column prop="id" label="任务 ID" min-width="190" /><el-table-column prop="type" label="类型" /><el-table-column prop="model" label="模型" min-width="160" /><el-table-column prop="pointCost" label="积分" /><el-table-column prop="status" label="状态"><template #default="scope"><el-tag :type="statusType(scope.row.status)">{{ statusLabel(scope.row.status) }}</el-tag></template></el-table-column><el-table-column prop="createdAt" label="创建时间" min-width="180" /></el-table></el-tab-pane>
      </el-tabs>
    </section>
  </el-drawer>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import { adminWorkspaceApi, type Customer360Response, type WorkspaceRecord } from "../../api/adminWorkspaces";
import { canAccessCustomerPointActions } from "../../domain/personalPointsAdmin.ts";
import AdminDataTable from "./AdminDataTable.vue";
import CustomerPointActions from "./CustomerPointActions.vue";
import IdentityEntitlementsPanel from "./IdentityEntitlementsPanel.vue";

type Action = { action: string; label: string };
const props = defineProps<{ rows: WorkspaceRecord[]; saving: boolean; toolbarActions: Action[]; rowActions: Action[]; role: string; permissions?: string[]; columnLabels: Record<string, string>; statusFilterOptions: Array<{ label: string; value: string }>; isStatusColumn: (column: string) => boolean; statusType: (value: unknown) => any; statusLabel: (value: unknown) => string; formatCell: (value: unknown, column: string) => unknown; visibleRowActions: (row: WorkspaceRecord) => Action[]; labelForRowAction: (action: Action, row: WorkspaceRecord) => string }>();
defineEmits<{ "run-action": [action: string, row?: WorkspaceRecord]; "batch-action": [action: string, rows: WorkspaceRecord[]] }>();

const customerColumns = ["name", "email", "mobile", "plan", "pointsAvailable", "sourceAgentName", "status", "createdAt"];
const searchKeyword = ref("");
const statusFilter = ref("ALL");
const drawerOpen = ref(false);
const loading = ref(false);
const errorMessage = ref("");
const activeTab = ref("overview");
const selectedCustomer = ref<WorkspaceRecord | null>(null);
const detail = reactive<Partial<Customer360Response>>({});
const canAccessPoints = computed(() => canAccessCustomerPointActions({ role: props.role, permissions: props.permissions || [] }));

async function openCustomer(row: WorkspaceRecord) {
  selectedCustomer.value = row;
  drawerOpen.value = true;
  activeTab.value = "overview";
  await reloadCustomer();
}
async function reloadCustomer() {
  const id = String(selectedCustomer.value?.id || "");
  if (!id) return;
  loading.value = true; errorMessage.value = "";
  try { Object.assign(detail, await adminWorkspaceApi.customer360(id)); }
  catch (error) { errorMessage.value = error instanceof Error ? error.message : "客户 360° 加载失败"; }
  finally { loading.value = false; }
}
function formatNumber(value: unknown) { return new Intl.NumberFormat("zh-CN").format(Number(value || 0)); }
function formatMoney(value: unknown) { return `¥${(Number(value || 0) / 100).toLocaleString("zh-CN", { minimumFractionDigits: 2 })}`; }
</script>

<style scoped>
.workspace-drawer-title { display: flex; align-items: center; justify-content: space-between; width: 100%; gap: 18px; }
.workspace-drawer-title > div { display: grid; gap: 4px; }.workspace-drawer-title span { color: var(--admin-muted); font-size: 12px; }.workspace-drawer-title strong { color: var(--admin-text); font-size: 20px; }
.customer-360-content { display: grid; gap: 18px; }.workspace-metrics { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }.workspace-metrics article { padding: 16px; border: 1px solid var(--admin-border); border-radius: 10px; background: var(--admin-panel); }.workspace-metrics span,.workspace-metrics small { display: block; color: var(--admin-muted); }.workspace-metrics strong { display: block; margin: 6px 0; color: var(--admin-text); font-size: 24px; }.workspace-detail-grid { display: grid; grid-template-columns: 1.4fr 1fr; gap: 14px; }
@media (max-width: 900px) { .workspace-metrics { grid-template-columns: repeat(2, 1fr); }.workspace-detail-grid { grid-template-columns: 1fr; } }
</style>
