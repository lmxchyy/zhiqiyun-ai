<template>
  <section class="attribution-page">
    <header class="attribution-heading">
      <div>
        <p>客户资产关系</p>
        <h1>客户归属总览</h1>
        <span>统一核对普通客户与企业客户的代理商、上级代理和运营中心归属。</span>
      </div>
      <el-tooltip content="刷新归属数据" placement="bottom">
        <el-button :icon="Refresh" circle :loading="loading" aria-label="刷新归属数据" @click="loadData" />
      </el-tooltip>
    </header>

    <div class="attribution-stats">
      <article v-for="item in statItems" :key="item.key" :class="`is-${item.key}`">
        <span>{{ item.label }}</span>
        <strong>{{ item.value }}</strong>
        <small>{{ item.hint }}</small>
      </article>
    </div>

    <section class="attribution-workspace">
      <div class="attribution-toolbar">
        <el-input
          v-model="filters.keyword"
          :prefix-icon="Search"
          clearable
          placeholder="搜索客户、代理商或运营中心"
          @keyup.enter="applyFilters"
        />
        <el-select v-model="filters.customerType" clearable placeholder="客户类型">
          <el-option label="普通客户" value="PERSONAL" />
          <el-option label="企业客户" value="ENTERPRISE" />
        </el-select>
        <el-select v-model="filters.healthStatus" clearable placeholder="归属状态">
          <el-option label="归属完整" value="COMPLETE" />
          <el-option label="部分归属" value="PARTIAL" />
          <el-option label="未归属" value="UNASSIGNED" />
          <el-option label="归属异常" value="ANOMALY" />
        </el-select>
        <el-select v-model="filters.agentId" clearable filterable placeholder="代理商">
          <el-option v-for="item in options.agents" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
        <el-select v-model="filters.operationCenterId" clearable filterable placeholder="运营中心">
          <el-option v-for="item in options.operationCenters" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
        <div class="attribution-toolbar-actions">
          <el-button type="primary" :icon="Search" :loading="loading" @click="applyFilters">查询</el-button>
          <el-button @click="resetFilters">重置</el-button>
        </div>
      </div>

      <el-alert v-if="error" :title="error" type="error" show-icon :closable="false">
        <template #default><el-button link type="danger" @click="loadData">重新加载</el-button></template>
      </el-alert>

      <el-table v-loading="loading" :data="items" row-key="id" stripe empty-text="暂无符合条件的归属记录">
        <el-table-column label="客户" min-width="220" fixed="left">
          <template #default="{ row }">
            <div class="customer-cell">
              <div class="customer-cell-title">
                <strong>{{ row.customerName }}</strong>
                <el-tag size="small" effect="plain" :type="row.customerType === 'ENTERPRISE' ? 'warning' : 'info'">
                  {{ row.customerType === "ENTERPRISE" ? "企业" : "个人" }}
                </el-tag>
              </div>
              <span>{{ row.email || row.customerId }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="直接代理商" min-width="170">
          <template #default="{ row }"><RelationParty :party="row.directAgent" kind="agent" /></template>
        </el-table-column>
        <el-table-column label="上级代理商" min-width="170">
          <template #default="{ row }"><RelationParty :party="row.parentAgent" kind="agent" /></template>
        </el-table-column>
        <el-table-column label="运营中心" min-width="170">
          <template #default="{ row }"><RelationParty :party="row.operationCenter" kind="center" /></template>
        </el-table-column>
        <el-table-column label="绑定方式" min-width="140">
          <template #default="{ row }">
            <span class="bind-type">{{ bindTypeLabel(row.bindType) }}</span>
            <small class="cell-secondary">{{ formatTime(row.bindAt) }}</small>
          </template>
        </el-table-column>
        <el-table-column label="归属检查" min-width="180">
          <template #default="{ row }">
            <el-tag size="small" effect="light" :type="healthTagType(row.healthStatus)">{{ healthLabel(row.healthStatus) }}</el-tag>
            <small v-if="row.issues.length" class="cell-secondary issue-text">{{ issueLabel(row.issues) }}</small>
          </template>
        </el-table-column>
        <el-table-column label="客户状态" min-width="110">
          <template #default="{ row }"><el-tag size="small" effect="plain" :type="row.relationStatus === 'ACTIVE' ? 'success' : 'info'">{{ statusLabel(row.relationStatus) }}</el-tag></template>
        </el-table-column>
      </el-table>

      <footer class="attribution-pagination">
        <span>共 {{ total }} 条归属记录</span>
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          background
          layout="sizes, prev, pager, next"
          :page-sizes="[20, 50, 100, 200]"
          :total="total"
          @current-change="loadData"
          @size-change="handleSizeChange"
        />
      </footer>
    </section>
  </section>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref } from "vue";
import { Refresh, Search } from "@element-plus/icons-vue";
import { ElTag } from "element-plus";
import {
  fetchCustomerAttributions,
  type CustomerAttributionItem,
  type CustomerAttributionOption,
  type CustomerAttributionParty
} from "../../api/customerAttribution";

const RelationParty = defineComponent({
  props: {
    party: { type: Object as () => CustomerAttributionParty, required: true },
    kind: { type: String as () => "agent" | "center", required: true }
  },
  setup(props) {
    return () => {
      if (!props.party?.id) return h("span", { class: "empty-relation" }, "未归属");
      return h("div", { class: "relation-party" }, [
        h("strong", props.party.name || props.party.id),
        props.kind === "agent" && props.party.level
          ? h(ElTag, { size: "small", effect: "plain", type: "primary" }, () => `L${props.party.level}`)
          : null,
        h("small", props.party.id)
      ]);
    };
  }
});

const loading = ref(false);
const error = ref("");
const items = ref<CustomerAttributionItem[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
const stats = reactive({ total: 0, complete: 0, partial: 0, unassigned: 0, anomaly: 0 });
const options = reactive<{ agents: CustomerAttributionOption[]; operationCenters: CustomerAttributionOption[] }>({ agents: [], operationCenters: [] });
const filters = reactive({ keyword: "", customerType: "", healthStatus: "", agentId: "", operationCenterId: "" });
let requestSequence = 0;

const statItems = computed(() => [
  { key: "total", label: "全部客户", value: stats.total, hint: "个人与企业统一口径" },
  { key: "complete", label: "归属完整", value: stats.complete, hint: "代理商与运营中心齐全" },
  { key: "partial", label: "部分归属", value: stats.partial, hint: "仍有关系字段待补齐" },
  { key: "unassigned", label: "未归属", value: stats.unassigned, hint: "平台直客或尚未绑定" },
  { key: "anomaly", label: "归属异常", value: stats.anomaly, hint: "存在失效或冲突关系" }
]);

async function loadData() {
  const sequence = ++requestSequence;
  loading.value = true;
  error.value = "";
  try {
    const result = await fetchCustomerAttributions({ page: page.value, pageSize: pageSize.value, ...filters });
    if (sequence !== requestSequence) return;
    items.value = result.items || [];
    total.value = result.total || 0;
    Object.assign(stats, result.stats || {});
    options.agents = result.filters?.agents || [];
    options.operationCenters = result.filters?.operationCenters || [];
  } catch (reason) {
    if (sequence !== requestSequence) return;
    error.value = reason instanceof Error ? reason.message : "客户归属数据加载失败";
    items.value = [];
  } finally {
    if (sequence === requestSequence) loading.value = false;
  }
}

function applyFilters() {
  page.value = 1;
  void loadData();
}

function resetFilters() {
  Object.assign(filters, { keyword: "", customerType: "", healthStatus: "", agentId: "", operationCenterId: "" });
  applyFilters();
}

function handleSizeChange() {
  page.value = 1;
  void loadData();
}

function formatTime(value?: string) {
  if (!value) return "--";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const parts = new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false }).formatToParts(date);
  const values = Object.fromEntries(parts.map((part) => [part.type, part.value]));
  return `${values.year}-${values.month}-${values.day} ${values.hour}:${values.minute}`;
}

function bindTypeLabel(value: string) {
  return ({ CUSTOMER_RELATION: "关系表绑定", REFERRAL: "推荐注册", PLATFORM_DIRECT: "平台直客", ENTERPRISE_ATTRIBUTION: "企业归属" } as Record<string, string>)[value] || value || "--";
}

function healthLabel(value: string) {
  return ({ COMPLETE: "归属完整", PARTIAL: "部分归属", UNASSIGNED: "未归属", ANOMALY: "归属异常" } as Record<string, string>)[value] || value;
}

function healthTagType(value: string) {
  return ({ COMPLETE: "success", PARTIAL: "warning", UNASSIGNED: "info", ANOMALY: "danger" } as Record<string, "success" | "warning" | "info" | "danger">)[value] || "info";
}

function issueLabel(issues: string[]) {
  const labels: Record<string, string> = {
    ATTRIBUTION_UNASSIGNED: "代理商和运营中心均未绑定",
    DIRECT_AGENT_UNASSIGNED: "缺少直接代理商",
    OPERATION_CENTER_UNASSIGNED: "缺少运营中心",
    DIRECT_AGENT_NOT_FOUND: "直接代理商已失效",
    PARENT_AGENT_NOT_FOUND: "上级代理商已失效",
    OPERATION_CENTER_NOT_FOUND: "运营中心已失效",
    OPERATION_CENTER_MISMATCH: "运营中心与代理关系冲突"
  };
  return issues.map((item) => labels[item] || item).join("；");
}

function statusLabel(value: string) {
  return ({ ACTIVE: "正常", DISABLED: "已停用", PAUSED: "已暂停", TERMINATED: "已终止" } as Record<string, string>)[value] || value || "--";
}

onMounted(loadData);
</script>

<style scoped>
.attribution-page {
  display: grid;
  gap: 16px;
  color: var(--color-text-main);
}

.attribution-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
}

.attribution-heading p {
  margin: 0 0 6px;
  color: var(--color-primary);
  font-size: 13px;
  font-weight: 600;
}

.attribution-heading h1 {
  margin: 0;
  font-size: 24px;
  line-height: 1.35;
  letter-spacing: 0;
}

.attribution-heading span {
  display: block;
  margin-top: 6px;
  color: var(--color-text-sub);
  font-size: 14px;
}

.attribution-stats {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: 8px;
  background: #fff;
}

.attribution-stats article {
  position: relative;
  min-width: 0;
  padding: 16px 18px;
  border-right: 1px solid var(--color-border);
}

.attribution-stats article:last-child { border-right: 0; }
.attribution-stats article::before { content: ""; position: absolute; inset: 0 auto 0 0; width: 3px; background: #c7d2fe; }
.attribution-stats .is-complete::before { background: #34c989; }
.attribution-stats .is-partial::before { background: #f7b84b; }
.attribution-stats .is-unassigned::before { background: #94a3b8; }
.attribution-stats .is-anomaly::before { background: #ef6464; }
.attribution-stats span, .attribution-stats small { display: block; color: var(--color-text-sub); font-size: 12px; }
.attribution-stats strong { display: block; margin: 4px 0 3px; font-size: 24px; line-height: 1.2; }
.attribution-stats small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.attribution-workspace {
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: 8px;
  background: #fff;
}

.attribution-toolbar {
  display: grid;
  grid-template-columns: minmax(220px, 1.5fr) repeat(4, minmax(150px, 1fr)) auto;
  gap: 10px;
  padding: 16px;
  border-bottom: 1px solid var(--color-border);
}

.attribution-toolbar-actions { display: flex; gap: 8px; }
.attribution-workspace :deep(.el-alert) { margin: 12px 16px 0; width: auto; }
.attribution-workspace :deep(.el-table) { --el-table-header-bg-color: #f7f8fc; --el-table-row-hover-bg-color: rgba(125, 141, 246, 0.06); }
.attribution-workspace :deep(.el-table th.el-table__cell) { color: #475569; font-size: 13px; font-weight: 600; }
.attribution-workspace :deep(.el-table .cell) { padding: 10px 14px; }

.customer-cell, .relation-party { display: grid; gap: 4px; min-width: 0; }
.customer-cell-title { display: flex; align-items: center; gap: 8px; min-width: 0; }
.customer-cell strong, .relation-party strong { overflow: hidden; color: var(--color-text-main); font-size: 14px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.customer-cell span, .relation-party small, .cell-secondary { overflow: hidden; color: var(--color-text-muted); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.relation-party { grid-template-columns: minmax(0, auto) max-content; align-items: center; }
.relation-party small { grid-column: 1 / -1; }
.empty-relation { color: var(--color-text-muted); font-size: 13px; }
.bind-type { display: block; color: var(--color-text-main); font-size: 13px; }
.cell-secondary { display: block; margin-top: 4px; }
.issue-text { max-width: 170px; color: #c24141; white-space: normal; }

.attribution-pagination {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 16px;
  border-top: 1px solid var(--color-border);
}

.attribution-pagination > span { color: var(--color-text-sub); font-size: 13px; }

@media (max-width: 1280px) {
  .attribution-toolbar { grid-template-columns: repeat(3, minmax(180px, 1fr)); }
  .attribution-toolbar-actions { justify-content: flex-end; }
}

@media (max-width: 900px) {
  .attribution-stats { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .attribution-stats article { border-bottom: 1px solid var(--color-border); }
  .attribution-toolbar { grid-template-columns: 1fr 1fr; }
}
</style>
