<template>
  <section class="price-plan-governance">
    <header class="price-plan-governance__heading">
      <div>
        <p>V2 套餐治理</p>
        <h1>套餐与价格配置</h1>
        <span>业务套餐、权益版本、多价格方案与微信商品本地记录分层管理；系统不会实时连接微信公众平台。</span>
      </div>
      <el-button :icon="Refresh" :loading="refreshing" @click="refresh">刷新配置</el-button>
    </header>

    <el-skeleton v-if="initialLoading && !pricingStore.health" :rows="3" animated />
    <section v-if="healthDisplay.showCards" class="price-plan-health-cards" aria-label="价格配置健康摘要">
      <article v-for="card in healthCards" :key="card.key" :class="`is-${card.tone}`">
        <span>{{ card.label }}</span>
        <strong>{{ card.value }}</strong>
        <small>{{ card.detail }}</small>
      </article>
    </section>
    <el-alert
      v-if="healthDisplay.showError"
      :type="healthDisplay.stale ? 'warning' : 'error'"
      show-icon
      :closable="false"
      :title="healthDisplay.stale ? '健康摘要刷新失败，当前显示缓存数据' : '健康摘要加载失败'"
      :description="healthDisplay.stale ? `${healthError}；缓存状态可能已过期，请手动重试后再执行敏感操作。` : healthError"
    />

    <BusinessPlanList
      :plans="pricingStore.businessPlans"
      :health-plans="pricingStore.health?.businessPlans || []"
      :selected-plan-id="pricingStore.selectedPlanId"
      :initial-loading="initialLoading && !pricingStore.businessPlans.length"
      :error="planListError"
      @select="selectPlan"
      @retry="refresh"
    />

    <section v-if="selectedPlan" class="price-plan-detail">
      <header class="price-plan-detail__heading">
        <div>
          <span>{{ businessTypeLabel(selectedPlan.businessType) }}</span>
          <h2>{{ selectedPlan.name }}</h2>
          <code>{{ selectedPlan.id }}</code>
        </div>
        <el-tag :type="healthTagType(selectedHealth?.status || 'UNKNOWN')">{{ healthStatusLabel(selectedHealth?.status || "UNKNOWN") }}</el-tag>
      </header>

      <el-tabs v-model="activeDetailTab" class="price-plan-detail__tabs">
        <el-tab-pane v-for="tab in visibleDetailTabs" :key="tab.id" :label="tab.label" :name="tab.id">
          <div v-if="tab.id === 'basic'" class="price-plan-basic-grid">
            <article><span>套餐 ID</span><strong>{{ selectedPlan.id }}</strong></article>
            <article><span>业务类型</span><strong>{{ businessTypeLabel(selectedPlan.businessType) }}</strong></article>
            <article class="is-code"><span>套餐编码（只读）</span><el-input :model-value="selectedPlan.code" disabled /></article>
            <article><span>编码属性</span><strong>{{ selectedPlan.legacyCode ? "历史兼容编码" : "稳定业务编码" }}</strong></article>
            <article><span>ACTIVE 权益版本</span><strong>{{ selectedHealth?.activeVersionId || selectedPlan.activeVersionId || "未配置" }}</strong></article>
            <article><span>价格方案数量</span><strong>{{ selectedHealth?.pricePlanCount ?? 0 }}</strong></article>
            <article><span>正式环境默认价</span><strong>{{ defaultPriceLabel(selectedHealth?.defaults.production || null) }}</strong></article>
            <article><span>沙箱环境默认价</span><strong>{{ defaultPriceLabel(selectedHealth?.defaults.sandbox || null) }}</strong></article>
          </div>
          <PlanVersionManager
            v-else-if="tab.id === 'entitlements'"
            :plan="selectedPlan"
            :current-role="currentRole"
            :current-permissions="currentPermissions"
            @view-audit="openAudit"
          />
          <PricePlanList
            v-else-if="tab.id === 'pricePlans'"
            :plan="selectedPlan"
            :current-role="currentRole"
            :current-permissions="currentPermissions"
            @view-audit="openAudit"
          />
          <WechatVirtualGoodsManager
            v-else-if="tab.id === 'wechatGoods'"
            :plan="selectedPlan"
            :current-role="currentRole"
            :current-permissions="currentPermissions"
            @view-audit="openAudit"
          />
          <PricePlanWhitelistManager
            v-else-if="tab.id === 'testWhitelist'"
            :plan="selectedPlan"
            :current-role="currentRole"
            :current-permissions="currentPermissions"
            @view-audit="openAudit"
          />
          <PricingAuditLog
            v-else-if="tab.id === 'audit'"
            :current-role="currentRole"
            :current-permissions="currentPermissions"
            :prefill="auditPrefill"
            :prefill-version="auditPrefillVersion"
          />
        </el-tab-pane>
      </el-tabs>
    </section>
    <el-empty v-else-if="!initialLoading && !planListError" description="请选择一个 V2 业务套餐查看详情" />
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { Refresh } from "@element-plus/icons-vue";
import BusinessPlanList from "./BusinessPlanList.vue";
import PlanVersionManager from "./PlanVersionManager.vue";
import PricePlanList from "./PricePlanList.vue";
import PricePlanWhitelistManager from "./PricePlanWhitelistManager.vue";
import PricingAuditLog from "./PricingAuditLog.vue";
import WechatVirtualGoodsManager from "./WechatVirtualGoodsManager.vue";
import { buildPricingHealthCards, PRICE_PLAN_DETAIL_TABS, pricingHealthDisplayState } from "../../../domain/pricePlanGovernance.ts";
import { formatPriceCents, hasPricingPermission, healthStatusLabel } from "../../../domain/pricePlanAdmin.ts";
import { usePricePlanAdminStore } from "../../../stores/pricePlanAdmin.ts";
import type { PricingAuditFilters, PricingHealthDefaultSummary } from "../../../types/pricePlanAdmin.ts";

const props = defineProps<{
  currentRole: string;
  currentPermissions: string[];
}>();

const pricingStore = usePricePlanAdminStore();
const initialized = ref(false);
const activeDetailTab = ref<(typeof PRICE_PLAN_DETAIL_TABS)[number]["id"]>("basic");
const auditPrefill = ref<PricingAuditFilters>({ page: 1, pageSize: 50 });
const auditPrefillVersion = ref(0);
const principal = computed(() => ({ role: props.currentRole, permissions: props.currentPermissions }));
const canViewAudit = computed(() => hasPricingPermission(principal.value, "pricing:audit:view"));
const visibleDetailTabs = computed(() => PRICE_PLAN_DETAIL_TABS.filter((tab) => tab.id !== "audit" || canViewAudit.value));
const refreshing = computed(() => Boolean(pricingStore.loading.businessPlans || pricingStore.loading.health));
const initialLoading = computed(() => !initialized.value && refreshing.value);
const healthCards = computed(() => pricingStore.health ? buildPricingHealthCards(pricingStore.health.summary) : []);
const planListError = computed(() => pricingStore.errors.businessPlans?.message || "");
const healthError = computed(() => pricingStore.errors.health?.message || "");
const healthDisplay = computed(() => pricingHealthDisplayState({ hasCachedHealth: healthCards.value.length > 0, error: healthError.value }));
const selectedPlan = computed(() => pricingStore.selectedBusinessPlan);
const selectedHealth = computed(() => pricingStore.health?.businessPlans.find((item) => item.planId === pricingStore.selectedPlanId));

async function refresh() {
  await Promise.allSettled([pricingStore.loadBusinessPlans(), pricingStore.loadHealth()]);
  if (!pricingStore.selectedBusinessPlan && pricingStore.businessPlans.length) {
    const planId = pricingStore.businessPlans[0].id;
    pricingStore.setSelection(planId);
    auditPrefill.value = { planId, page: 1, pageSize: 50 };
  }
  initialized.value = true;
}

function selectPlan(planId: string) {
  pricingStore.setSelection(planId);
  auditPrefill.value = { planId, page: 1, pageSize: 50 };
  auditPrefillVersion.value += 1;
  activeDetailTab.value = "basic";
}

function openAudit(filters: PricingAuditFilters) {
  if (!canViewAudit.value) return;
  auditPrefill.value = { ...filters, page: 1, pageSize: 50 };
  auditPrefillVersion.value += 1;
  activeDetailTab.value = "audit";
}

watch(canViewAudit, (allowed) => {
  if (!allowed && activeDetailTab.value === "audit") activeDetailTab.value = "basic";
});

function businessTypeLabel(value: string) {
  return value === "MEMBER" ? "会员套餐" : value === "AGENT" ? "代理商套餐" : value;
}

function defaultPriceLabel(item: PricingHealthDefaultSummary | null) {
  if (!item) return "未配置";
  return formatPriceCents(item.salePriceCents, `${item.currency || "CNY"} `);
}

function healthTagType(value: string) {
  return ({ HEALTHY: "success", DEGRADED: "warning", BLOCKED: "danger" } as Record<string, "success" | "warning" | "danger" | "info">)[value] || "info";
}

onMounted(refresh);
</script>

<style scoped>
.price-plan-governance { display: grid; min-width: 0; gap: 16px; color: var(--admin-text); }
.price-plan-governance__heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 20px; }
.price-plan-governance__heading p { margin: 0 0 5px; color: var(--admin-primary-strong); font-size: 13px; font-weight: 700; }
.price-plan-governance__heading h1 { margin: 0; font-size: 25px; }
.price-plan-governance__heading span { display: block; margin-top: 7px; color: var(--admin-muted); line-height: 1.55; }
.price-plan-health-cards { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
.price-plan-health-cards article { display: grid; gap: 7px; min-height: 120px; padding: 18px; border: 1px solid var(--admin-border); border-radius: 12px; background: var(--admin-panel); }
.price-plan-health-cards article span, .price-plan-health-cards article small { color: var(--admin-muted); }
.price-plan-health-cards article strong { color: var(--admin-text); font-size: 28px; }
.price-plan-health-cards article.is-primary { box-shadow: inset 3px 0 var(--admin-primary); }
.price-plan-health-cards article.is-success { box-shadow: inset 3px 0 var(--admin-success); }
.price-plan-health-cards article.is-info { box-shadow: inset 3px 0 #64748b; }
.price-plan-health-cards article.is-danger { box-shadow: inset 3px 0 #ef4444; }
.price-plan-detail { min-width: 0; padding: 18px; border: 1px solid var(--admin-border); border-radius: 12px; background: var(--admin-panel); }
.price-plan-detail__heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; margin-bottom: 8px; }
.price-plan-detail__heading > div { display: grid; gap: 4px; }
.price-plan-detail__heading span { color: var(--admin-primary-strong); font-size: 12px; font-weight: 700; }
.price-plan-detail__heading h2 { margin: 0; font-size: 20px; }
.price-plan-detail__heading code { color: var(--admin-muted); }
.price-plan-detail__tabs { min-width: 0; }
.price-plan-detail__tabs :deep(.el-tabs__nav-wrap) { overflow: hidden; }
.price-plan-detail__tabs :deep(.el-tabs__nav-scroll) { overflow-x: auto; scrollbar-width: thin; }
.price-plan-detail__tabs :deep(.el-tabs__nav) { float: none; width: max-content; white-space: nowrap; }
.price-plan-basic-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; padding-top: 8px; }
.price-plan-basic-grid article { display: grid; gap: 7px; min-width: 0; padding: 14px; border: 1px solid var(--admin-border); border-radius: 10px; background: var(--admin-panel-soft); }
.price-plan-basic-grid article span { color: var(--admin-muted); font-size: 12px; }
.price-plan-basic-grid article strong { overflow-wrap: anywhere; }
.price-plan-basic-grid article.is-code { grid-column: 1 / -1; }
@media (max-width: 1120px) {
  .price-plan-health-cards { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
@media (max-width: 640px) {
  .price-plan-governance__heading { align-items: stretch; flex-direction: column; }
  .price-plan-health-cards, .price-plan-basic-grid { grid-template-columns: 1fr; }
  .price-plan-basic-grid article.is-code { grid-column: auto; }
}
</style>
