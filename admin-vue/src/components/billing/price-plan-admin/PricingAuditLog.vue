<template>
  <section class="pricing-audit-log">
    <el-alert
      v-if="!canView"
      type="error"
      show-icon
      :closable="false"
      title="无审计查看权限"
      description="需要精确权限 pricing:audit:view；普通 admin.full 不会替代该权限。"
    />

    <template v-else>
      <header class="pricing-audit-log__heading">
        <div>
          <h3>定价领域审计</h3>
          <p>所有筛选、分页均由服务端执行；本页只读，不提供审计记录修改能力。</p>
        </div>
        <el-button :loading="loading" @click="loadAudit">刷新</el-button>
      </header>

      <el-form class="pricing-audit-log__filters" label-position="top" @submit.prevent>
        <el-form-item label="套餐 ID"><el-input v-model="form.planId" data-audit-filter="planId" clearable /></el-form-item>
        <el-form-item label="权益版本 ID"><el-input v-model="form.planVersionId" data-audit-filter="planVersionId" clearable /></el-form-item>
        <el-form-item label="价格方案 ID"><el-input v-model="form.pricePlanId" data-audit-filter="pricePlanId" clearable /></el-form-item>
        <el-form-item label="微信商品 ID"><el-input v-model="form.wechatGoodId" data-audit-filter="wechatGoodId" clearable /></el-form-item>
        <el-form-item label="支付绑定 ID"><el-input v-model="form.bindingId" data-audit-filter="bindingId" clearable /></el-form-item>
        <el-form-item label="白名单记录 ID"><el-input v-model="form.whitelistEntryId" data-audit-filter="whitelistEntryId" clearable /></el-form-item>
        <el-form-item label="动作"><el-input v-model="form.action" data-audit-filter="action" clearable /></el-form-item>
        <el-form-item label="操作人 ID"><el-input v-model="form.operatorId" data-audit-filter="operatorId" clearable /></el-form-item>
        <el-form-item label="操作角色"><el-input v-model="form.operatorRole" data-audit-filter="operatorRole" clearable /></el-form-item>
        <el-form-item label="开始时间">
          <el-date-picker
            v-model="form.startTime"
            data-audit-filter="startTime"
            type="datetime"
            value-format="YYYY-MM-DDTHH:mm:ssZ"
            format="YYYY-MM-DD HH:mm:ss"
            clearable
          />
        </el-form-item>
        <el-form-item label="结束时间">
          <el-date-picker
            v-model="form.endTime"
            data-audit-filter="endTime"
            type="datetime"
            value-format="YYYY-MM-DDTHH:mm:ssZ"
            format="YYYY-MM-DD HH:mm:ss"
            clearable
          />
        </el-form-item>
        <el-form-item label="结果">
          <el-select v-model="form.result" data-audit-filter="result" clearable placeholder="全部结果">
            <el-option label="成功" value="SUCCEEDED" />
            <el-option label="失败" value="FAILED" />
          </el-select>
        </el-form-item>
        <div class="pricing-audit-log__filter-actions">
          <el-button @click="resetFilters">重置</el-button>
          <el-button type="primary" :loading="loading" @click="search">查询</el-button>
        </div>
      </el-form>

      <div class="pricing-audit-log__quick-filters" aria-label="常用审计动作">
        <span>快速筛选：</span>
        <el-button
          v-for="item in PRICING_AUDIT_QUICK_FILTERS"
          :key="item.id"
          size="small"
          plain
          :type="form.action === item.action ? 'primary' : undefined"
          @click="applyQuickFilter(item.action)"
        >{{ item.label }}</el-button>
      </div>

      <el-alert
        v-if="displayState === 'TABLE_STALE'"
        type="warning"
        show-icon
        :closable="false"
        title="审计日志刷新失败，当前显示同一筛选条件的缓存结果"
        :description="errorMessage"
      />
      <el-skeleton v-if="displayState === 'LOADING'" :rows="6" animated />
      <el-result v-else-if="displayState === 'ERROR'" icon="error" title="审计日志加载失败" :sub-title="errorMessage">
        <template #extra><el-button @click="loadAudit">重试</el-button></template>
      </el-result>
      <el-empty v-else-if="displayState === 'EMPTY'" description="当前筛选条件下没有审计记录" />

      <template v-else-if="displayState === 'TABLE' || displayState === 'TABLE_STALE'">
        <el-table :data="rows" row-key="auditLogId" stripe>
          <el-table-column data-audit-column="operationTime" label="操作时间" min-width="180">
            <template #default="{ row }">{{ formatDateTime(row.operationTime) }}</template>
          </el-table-column>
          <el-table-column data-audit-column="operator" label="操作人 / 角色" min-width="190">
            <template #default="{ row }"><strong>{{ row.operatorId || "未知" }}</strong><small>{{ row.operatorRole || "未知" }}</small></template>
          </el-table-column>
          <el-table-column data-audit-column="action" prop="action" label="动作" min-width="230" />
          <el-table-column data-audit-column="entity" label="实体" min-width="220">
            <template #default="{ row }"><strong>{{ row.entityType || "未知" }}</strong><code>{{ row.entityId || "未知" }}</code></template>
          </el-table-column>
          <el-table-column data-audit-column="changeReason" prop="changeReason" label="变更原因" min-width="220" show-overflow-tooltip />
          <el-table-column data-audit-column="revision" label="revision" width="130">
            <template #default="{ row }">{{ revisionLabel(row) }}</template>
          </el-table-column>
          <el-table-column data-audit-column="result" label="结果 / 错误码" min-width="180">
            <template #default="{ row }"><el-tag :type="row.result === 'SUCCEEDED' ? 'success' : 'danger'">{{ row.result }}</el-tag><small>{{ row.errorCode || "—" }}</small></template>
          </el-table-column>
          <el-table-column data-audit-column="requestId" label="requestId" min-width="210">
            <template #default="{ row }"><code>{{ row.requestId || "—" }}</code></template>
          </el-table-column>
          <el-table-column label="详情" fixed="right" width="90">
            <template #default="{ row }"><el-button link type="primary" @click="openDetail(row)">查看</el-button></template>
          </el-table-column>
        </el-table>

        <el-pagination
          class="pricing-audit-log__pagination"
          background
          layout="total, sizes, prev, pager, next, jumper"
          :current-page="form.page"
          :page-size="form.pageSize"
          :page-sizes="[25, 50, 100, 200]"
          :total="page?.total || 0"
          @current-change="changePage"
          @size-change="changePageSize"
        />
      </template>
    </template>

    <el-drawer v-model="detailOpen" title="审计详情" size="min(720px, 96vw)" destroy-on-close>
      <template v-if="selectedLog">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="审计 ID">{{ selectedLog.auditLogId }}</el-descriptions-item>
          <el-descriptions-item label="动作">{{ selectedLog.action }}</el-descriptions-item>
          <el-descriptions-item label="实体">{{ selectedLog.entityType }} / {{ selectedLog.entityId }}</el-descriptions-item>
          <el-descriptions-item label="操作人">{{ selectedLog.operatorId }} / {{ selectedLog.operatorRole }}</el-descriptions-item>
          <el-descriptions-item label="变更原因">{{ selectedLog.changeReason || "—" }}</el-descriptions-item>
          <el-descriptions-item label="结果">{{ selectedLog.result }} / {{ selectedLog.errorCode || "—" }}</el-descriptions-item>
          <el-descriptions-item label="requestId">{{ selectedLog.requestId || "—" }}</el-descriptions-item>
        </el-descriptions>

        <el-alert
          v-if="snapshotRedacted || snapshotTruncated"
          type="warning"
          show-icon
          :closable="false"
          title="详情已在客户端再次脱敏或截断"
          description="服务端脱敏仍是最终安全边界；客户端只做第二层防护与展示限额。"
        />
        <el-collapse v-model="detailSections" class="pricing-audit-log__snapshots">
          <el-collapse-item name="before" title="变更前快照">
            <pre data-audit-snapshot="before">{{ beforeSnapshot.text }}</pre>
          </el-collapse-item>
          <el-collapse-item name="after" title="变更后快照">
            <pre data-audit-snapshot="after">{{ afterSnapshot.text }}</pre>
          </el-collapse-item>
          <el-collapse-item name="metadata" title="元数据">
            <pre data-audit-snapshot="metadata">{{ metadataSnapshot.text }}</pre>
          </el-collapse-item>
        </el-collapse>
      </template>
    </el-drawer>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { buildPricingAuditQuery, normalizePricingAuditFilters } from "../../../api/pricePlanAdmin.ts";
import {
  createPricingAuditLoadExecutor,
  createLatestRequestGate,
  formatPricingAuditSnapshot,
  hasPricingPermission,
  PRICING_AUDIT_QUICK_FILTERS,
  pricingAuditDisplayState,
  pricingErrorMessage
} from "../../../domain/pricePlanAdmin.ts";
import { usePricePlanAdminStore } from "../../../stores/pricePlanAdmin.ts";
import type { PricingAuditFilters, PricingAuditLog as PricingAuditLogRecord, PricingAuditResult } from "../../../types/pricePlanAdmin.ts";

interface PricingAuditFilterForm {
  planId: string;
  planVersionId: string;
  pricePlanId: string;
  wechatGoodId: string;
  bindingId: string;
  whitelistEntryId: string;
  action: string;
  operatorId: string;
  operatorRole: string;
  startTime: string;
  endTime: string;
  result: "" | PricingAuditResult;
  page: number;
  pageSize: number;
}

const props = withDefaults(defineProps<{
  currentRole: string;
  currentPermissions: string[];
  prefill?: PricingAuditFilters;
  prefillVersion?: number;
}>(), {
  prefill: () => ({}),
  prefillVersion: 0
});

const store = usePricePlanAdminStore();
const form = reactive<PricingAuditFilterForm>(emptyForm());
const requestedSignature = ref("");
const localError = ref("");
const selectedLog = ref<PricingAuditLogRecord | null>(null);
const detailOpen = ref(false);
const detailSections = ref<string[]>([]);
const principal = computed(() => ({ role: props.currentRole, permissions: props.currentPermissions }));
const canView = computed(() => hasPricingPermission(principal.value, "pricing:audit:view"));
const executeAuditLoad = createPricingAuditLoadExecutor(
  () => principal.value,
  (filters) => store.loadAuditLogs(filters)
);
const auditRequestGate = createLatestRequestGate();
const loading = computed(() => Boolean(store.loading.audit));
const page = computed(() => store.auditPage);
const cacheSignature = computed(() => store.auditPage ? buildPricingAuditQuery(store.auditFilters) : "");
const cacheMatches = computed(() => Boolean(requestedSignature.value && requestedSignature.value === cacheSignature.value));
const rows = computed(() => cacheMatches.value ? store.auditPage?.items || [] : []);
const errorMessage = computed(() => localError.value || store.errors.audit?.message || "");
const displayState = computed(() => pricingAuditDisplayState({
  canView: canView.value,
  cacheMatches: cacheMatches.value,
  hasPage: Boolean(store.auditPage),
  loading: loading.value,
  error: errorMessage.value,
  rowCount: rows.value.length
}));
const beforeSnapshot = computed(() => formatPricingAuditSnapshot(selectedLog.value?.beforeSnapshot ?? null));
const afterSnapshot = computed(() => formatPricingAuditSnapshot(selectedLog.value?.afterSnapshot ?? null));
const metadataSnapshot = computed(() => formatPricingAuditSnapshot(selectedLog.value?.metadata ?? null));
const snapshotRedacted = computed(() => beforeSnapshot.value.redacted || afterSnapshot.value.redacted || metadataSnapshot.value.redacted);
const snapshotTruncated = computed(() => beforeSnapshot.value.truncated || afterSnapshot.value.truncated || metadataSnapshot.value.truncated);

function emptyForm(): PricingAuditFilterForm {
  return {
    planId: "", planVersionId: "", pricePlanId: "", wechatGoodId: "", bindingId: "", whitelistEntryId: "",
    action: "", operatorId: "", operatorRole: "", startTime: "", endTime: "", result: "", page: 1, pageSize: 50
  };
}

function applyPrefill(prefill: PricingAuditFilters) {
  const normalized = normalizePricingAuditFilters({ ...prefill, page: prefill.page ?? 1, pageSize: prefill.pageSize ?? 50 });
  Object.assign(form, emptyForm(), normalized, { result: normalized.result || "" });
}

function requestFilters(): PricingAuditFilters {
  return normalizePricingAuditFilters({
    planId: form.planId,
    planVersionId: form.planVersionId,
    pricePlanId: form.pricePlanId,
    wechatGoodId: form.wechatGoodId,
    bindingId: form.bindingId,
    whitelistEntryId: form.whitelistEntryId,
    action: form.action,
    operatorId: form.operatorId,
    operatorRole: form.operatorRole,
    startTime: form.startTime,
    endTime: form.endTime,
    result: form.result || undefined,
    page: form.page,
    pageSize: form.pageSize
  });
}

async function loadAudit() {
  if (!canView.value) return;
  const token = auditRequestGate.begin();
  if (auditRequestGate.isLatest(token)) {
    localError.value = "";
    requestedSignature.value = "";
  }
  try {
    const normalized = requestFilters();
    if (auditRequestGate.isLatest(token)) requestedSignature.value = buildPricingAuditQuery(normalized);
    await executeAuditLoad(normalized);
  } catch (error) {
    if (auditRequestGate.isLatest(token)) localError.value = pricingErrorMessage(error);
  }
}

function search() {
  form.page = 1;
  return loadAudit();
}

function resetFilters() {
  applyPrefill(props.prefill);
  return loadAudit();
}

function applyQuickFilter(action: string) {
  form.action = action;
  form.page = 1;
  return loadAudit();
}

function changePage(pageNumber: number) {
  form.page = pageNumber;
  return loadAudit();
}

function changePageSize(pageSize: number) {
  form.page = 1;
  form.pageSize = pageSize;
  return loadAudit();
}

function openDetail(row: PricingAuditLogRecord) {
  selectedLog.value = row;
  detailSections.value = [];
  detailOpen.value = true;
}

function formatDateTime(value: string) {
  const parsed = new Date(value);
  return Number.isFinite(parsed.getTime()) ? parsed.toLocaleString("zh-CN", { hour12: false }) : value || "—";
}

function revisionLabel(row: PricingAuditLogRecord) {
  return `${row.revisionBefore ?? "—"} → ${row.revisionAfter ?? "—"}`;
}

watch(
  [() => props.prefillVersion, canView],
  ([, allowed]) => {
    applyPrefill(props.prefill);
    if (allowed) {
      void loadAudit();
    } else {
      auditRequestGate.invalidate();
      localError.value = "";
      requestedSignature.value = "";
      detailOpen.value = false;
      selectedLog.value = null;
      detailSections.value = [];
    }
  },
  { immediate: true }
);
</script>

<style scoped>
.pricing-audit-log { display: grid; min-width: 0; gap: 14px; padding-top: 8px; }
.pricing-audit-log__heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.pricing-audit-log__heading h3 { margin: 0; }
.pricing-audit-log__heading p { margin: 6px 0 0; color: var(--admin-muted); line-height: 1.55; }
.pricing-audit-log__filters { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 0 12px; padding: 14px; border: 1px solid var(--admin-border); border-radius: 10px; background: var(--admin-panel-soft); }
.pricing-audit-log__filters :deep(.el-date-editor), .pricing-audit-log__filters :deep(.el-select) { width: 100%; }
.pricing-audit-log__filter-actions { display: flex; align-items: flex-end; justify-content: flex-end; gap: 8px; padding-bottom: 18px; }
.pricing-audit-log__quick-filters { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; color: var(--admin-muted); }
.pricing-audit-log :deep(.el-table small), .pricing-audit-log :deep(.el-table code) { display: block; margin-top: 4px; color: var(--admin-muted); overflow-wrap: anywhere; }
.pricing-audit-log__pagination { justify-content: flex-end; margin-top: 14px; }
.pricing-audit-log__snapshots { margin-top: 16px; }
.pricing-audit-log__snapshots pre { max-height: 420px; margin: 0; padding: 14px; overflow: auto; border: 1px solid var(--admin-border); border-radius: 8px; background: var(--admin-panel-soft); color: var(--admin-text); font: 12px/1.6 ui-monospace, SFMono-Regular, Consolas, monospace; white-space: pre-wrap; overflow-wrap: anywhere; }
@media (max-width: 1100px) { .pricing-audit-log__filters { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 680px) {
  .pricing-audit-log__heading { flex-direction: column; }
  .pricing-audit-log__filters { grid-template-columns: 1fr; }
  .pricing-audit-log__filter-actions { justify-content: stretch; }
}
</style>
