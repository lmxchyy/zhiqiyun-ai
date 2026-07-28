<template>
  <section class="business-plan-list">
    <header class="business-plan-list__toolbar">
      <el-input v-model="filters.keyword" :prefix-icon="Search" clearable placeholder="搜索套餐名称、编码或 ID" />
      <el-select v-model="filters.businessType" aria-label="业务类型筛选">
        <el-option label="全部业务类型" value="ALL" />
        <el-option label="会员套餐" value="MEMBER" />
        <el-option label="代理商套餐" value="AGENT" />
      </el-select>
      <el-select v-model="filters.status" aria-label="健康状态筛选">
        <el-option label="全部健康状态" value="ALL" />
        <el-option label="正常" value="HEALTHY" />
        <el-option label="需关注" value="DEGRADED" />
        <el-option label="已阻断" value="BLOCKED" />
      </el-select>
      <span>共 {{ rows.length }} 个 V2 套餐</span>
    </header>

    <el-alert v-if="error" type="error" :closable="false" show-icon :title="error" class="business-plan-list__error">
      <template #default>
        <span>{{ rows.length ? "现有数据会继续保留；请检查权限或稍后刷新。" : "未能取得套餐数据；这不是空列表。请检查权限或稍后重试。" }}</span>
        <el-button link type="danger" @click="emit('retry')">重新加载</el-button>
      </template>
    </el-alert>

    <el-skeleton v-if="displayState === 'LOADING'" :rows="7" animated />
    <div v-else-if="displayState === 'TABLE'" class="business-plan-list__table-scroll">
      <el-table
        :data="rows"
        row-key="id"
        stripe
        highlight-current-row
        :current-row-key="selectedPlanId"
        @row-click="selectPlan"
      >
        <el-table-column label="业务套餐" min-width="210" fixed="left">
          <template #default="{ row }">
            <div class="business-plan-cell">
              <strong>{{ row.name }}</strong>
              <span>{{ row.id }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="业务类型" min-width="110">
          <template #default="{ row }"><el-tag effect="plain">{{ businessTypeLabel(row.businessType) }}</el-tag></template>
        </el-table-column>
        <el-table-column label="套餐编码" min-width="230">
          <template #default="{ row }">
            <div class="business-plan-code">
              <code>{{ row.code }}</code>
              <el-tag size="small" effect="plain" type="info">只读</el-tag>
              <el-tag v-if="row.legacyCode" size="small" effect="plain" type="warning">历史兼容编码</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="ACTIVE 权益版本" min-width="190">
          <template #default="{ row }"><code>{{ row.activeVersionId || "未配置" }}</code></template>
        </el-table-column>
        <el-table-column label="正式默认价" min-width="150">
          <template #default="{ row }">{{ defaultPriceLabel(row.productionDefault) }}</template>
        </el-table-column>
        <el-table-column label="沙箱默认价" min-width="150">
          <template #default="{ row }">{{ defaultPriceLabel(row.sandboxDefault) }}</template>
        </el-table-column>
        <el-table-column prop="pricePlanCount" label="价格方案" width="100" align="center" />
        <el-table-column label="套餐状态" width="105">
          <template #default="{ row }"><el-tag :type="row.active ? 'success' : 'info'">{{ row.active ? "启用" : "停用" }}</el-tag></template>
        </el-table-column>
        <el-table-column label="健康状态" width="110" fixed="right">
          <template #default="{ row }"><el-tag :type="healthTagType(row.healthStatus)">{{ healthStatusLabel(row.healthStatus) }}</el-tag></template>
        </el-table-column>
      </el-table>
    </div>
    <el-empty v-else-if="displayState === 'EMPTY'" description="没有符合筛选条件的 V2 会员或代理商套餐" />
  </section>
</template>

<script setup lang="ts">
import { computed, reactive } from "vue";
import { Search } from "@element-plus/icons-vue";
import { businessPlanListDisplayState, filterBusinessPlanRows, type BusinessPlanListRow } from "../../../domain/pricePlanGovernance.ts";
import { formatPriceCents, healthStatusLabel } from "../../../domain/pricePlanAdmin.ts";
import type { BusinessPlan, PricingHealthBusinessPlan, PricingHealthDefaultSummary } from "../../../types/pricePlanAdmin.ts";

const props = defineProps<{
  plans: BusinessPlan[];
  healthPlans: PricingHealthBusinessPlan[];
  selectedPlanId: string;
  initialLoading: boolean;
  error?: string;
}>();

const emit = defineEmits<{ select: [planId: string]; retry: [] }>();
const filters = reactive({ keyword: "", businessType: "ALL" as "ALL" | "MEMBER" | "AGENT", status: "ALL" as "ALL" | "HEALTHY" | "DEGRADED" | "BLOCKED" });
const rows = computed(() => filterBusinessPlanRows(props.plans, props.healthPlans, filters));
const displayState = computed(() => businessPlanListDisplayState({ initialLoading: props.initialLoading, error: props.error, rowCount: rows.value.length }));

function selectPlan(row: BusinessPlanListRow) {
  emit("select", row.id);
}

function businessTypeLabel(value: string) {
  return value === "MEMBER" ? "会员" : value === "AGENT" ? "代理商" : value;
}

function defaultPriceLabel(item: PricingHealthDefaultSummary | null) {
  if (!item) return "未配置";
  return formatPriceCents(item.salePriceCents, `${item.currency || "CNY"} `);
}

function healthTagType(value: string) {
  return ({ HEALTHY: "success", DEGRADED: "warning", BLOCKED: "danger" } as Record<string, "success" | "warning" | "danger" | "info">)[value] || "info";
}
</script>

<style scoped>
.business-plan-list { display: grid; gap: 14px; min-width: 0; padding: 18px; border: 1px solid var(--admin-border); border-radius: 12px; background: var(--admin-panel); }
.business-plan-list__toolbar { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; }
.business-plan-list__toolbar :deep(.el-input) { width: min(330px, 100%); }
.business-plan-list__toolbar :deep(.el-select) { width: 168px; }
.business-plan-list__toolbar > span { margin-left: auto; color: var(--admin-muted); font-size: 13px; }
.business-plan-list__error { align-items: flex-start; }
.business-plan-list__table-scroll { min-width: 0; overflow-x: auto; }
.business-plan-list__table-scroll :deep(.el-table) { min-width: 1280px; }
.business-plan-cell, .business-plan-code { display: grid; gap: 5px; }
.business-plan-cell span { color: var(--admin-muted); font-size: 12px; }
.business-plan-code { justify-items: start; }
.business-plan-code code, :deep(.el-table code) { color: var(--admin-text); font-size: 12px; overflow-wrap: anywhere; }
@media (max-width: 760px) {
  .business-plan-list { padding: 14px; }
  .business-plan-list__toolbar { align-items: stretch; }
  .business-plan-list__toolbar :deep(.el-input), .business-plan-list__toolbar :deep(.el-select) { width: 100%; }
  .business-plan-list__toolbar > span { margin-left: 0; }
}
</style>
