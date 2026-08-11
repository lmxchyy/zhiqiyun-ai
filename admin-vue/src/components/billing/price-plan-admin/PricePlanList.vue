<template>
  <section class="price-plan-list">
    <header class="price-plan-list__toolbar">
      <div class="price-plan-list__filters">
        <el-input v-model="filters.keyword" clearable placeholder="搜索名称、编码或 ID" aria-label="搜索价格方案" />
        <el-select v-model="filters.status" aria-label="按生命周期筛选"><el-option label="全部状态" value="ALL" /><el-option v-for="item in statuses" :key="item" :label="statusLabel(item)" :value="item" /></el-select>
        <el-select v-model="filters.kind" aria-label="按价格方案类型筛选"><el-option label="全部类型" value="ALL" /><el-option label="正常价" value="NORMAL" /><el-option label="活动价" value="PROMOTION" /><el-option label="测试价" value="TEST" /></el-select>
        <el-select v-model="filters.environment" aria-label="按支付环境筛选"><el-option label="全部环境" value="ALL" /><el-option label="正式" value="PRODUCTION" /><el-option label="沙箱" value="SANDBOX" /></el-select>
      </div>
      <div>
        <el-button :loading="loading" @click="refresh">刷新</el-button>
        <el-button type="primary" :disabled="!canManage" @click="openCreate">新建价格方案</el-button>
      </div>
    </header>

    <el-alert v-if="runtimeAlert" :type="runtimeAlert.type" show-icon :closable="false" :title="runtimeAlert.title" :description="runtimeAlert.description" />
    <el-alert v-if="listError && rows.length" type="warning" show-icon :closable="false" title="价格方案刷新失败，当前显示缓存数据" :description="listError" />
    <el-skeleton v-if="displayState === 'LOADING'" :rows="5" animated />
    <el-result v-else-if="displayState === 'ERROR'" icon="error" title="价格方案加载失败" :sub-title="listError"><template #extra><el-button @click="refresh">重试</el-button></template></el-result>
    <el-empty v-else-if="displayState === 'EMPTY'" description="当前套餐尚无价格方案" />
    <el-table v-else :data="filteredRows" row-key="pricePlanId" stripe>
      <el-table-column label="方案" min-width="220">
        <template #default="{ row }">
          <div class="price-plan-cell"><strong>{{ row.name }}</strong><code>{{ row.code }}</code><small>{{ row.pricePlanId }}</small></div>
          <div class="price-plan-badges">
            <el-tag v-for="badge in pricePlanBadges(row)" :key="badge" :type="badge.startsWith('配置异常') ? 'danger' : 'info'" effect="plain">{{ badge }}</el-tag>
            <el-tag v-if="row.isDefault" type="success">默认</el-tag>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="类型" width="100"><template #default="{ row }">{{ kindLabel(displayFacts(row).kind) }}</template></el-table-column>
      <el-table-column label="权益版本" min-width="180"><template #default="{ row }"><code>{{ displayFacts(row).planVersionId || "未知" }}</code></template></el-table-column>
      <el-table-column label="生命周期" width="110"><template #default="{ row }"><el-tag :type="statusTag(row.status)">{{ statusLabel(row.status) }}</el-tag></template></el-table-column>
      <el-table-column label="健康" width="120"><template #default="{ row }"><el-tag :type="healthTag(row.healthStatus)">{{ healthStatusLabel(row.healthStatus) }}</el-tag></template></el-table-column>
      <el-table-column label="价格" width="150"><template #default="{ row }"><strong>{{ money(row.salePriceCents) }}</strong><small class="price-plan-muted">原价 {{ money(row.listPriceCents) }}</small></template></el-table-column>
      <el-table-column label="渠道 / 环境" width="180"><template #default="{ row }">{{ row.channel }}<small class="price-plan-muted">{{ row.environment }} · {{ row.currency }}</small></template></el-table-column>
      <el-table-column label="受众 / 开关" min-width="180"><template #default="{ row }">
        <strong>{{ displayFacts(row).audienceType || "未知" }}</strong>
        <small class="price-plan-muted">可见 {{ yesNo(displayFacts(row).isVisible) }} · 启用 {{ yesNo(displayFacts(row).isEnabled) }} · 默认 {{ yesNo(displayFacts(row).isDefault) }}</small>
      </template></el-table-column>
      <el-table-column label="有效期" min-width="210"><template #default="{ row }">
        <span>{{ displayFacts(row).validFrom || "立即" }}</span><small class="price-plan-muted">至 {{ displayFacts(row).validUntil || "长期" }}</small>
      </template></el-table-column>
      <el-table-column label="微信商品" min-width="190"><template #default="{ row }">
        <code>{{ displayFacts(row).wechatProductId || "未取得精确商品" }}</code><small class="price-plan-muted">商品价 {{ money(displayFacts(row).wechatGoodPriceCents) }}</small>
      </template></el-table-column>
      <el-table-column label="revision" width="100"><template #default="{ row }">{{ displayFacts(row).revision ?? "未知" }}</template></el-table-column>
      <el-table-column label="引用" width="120"><template #default="{ row }"><span>quote {{ row.quoteCount ?? "未知" }}</span><small class="price-plan-muted">订单 {{ row.orderCount ?? "未知" }}</small></template></el-table-column>
      <el-table-column label="校验" min-width="180"><template #default="{ row }">
        <el-tag :type="row.validationFresh ? (row.validation?.valid ? 'success' : 'danger') : 'info'">{{ row.validationFresh ? (row.validation?.valid ? "本次校验通过" : "本次校验失败") : "未取得新鲜校验" }}</el-tag>
        <small v-if="row.healthIssueCodes.length" class="price-plan-muted">{{ row.healthIssueCodes.map(healthIssueLabel).join("；") }}</small>
      </template></el-table-column>
      <el-table-column label="操作" fixed="right" width="340"><template #default="{ row }">
        <div class="price-plan-actions">
          <el-button v-if="canViewAudit" link type="primary" @click="viewPricePlanAudit(row)">查看审计</el-button>
          <el-button link type="primary" @click="openValidation(row.pricePlanId)">查看校验</el-button>
          <el-button link @click="openBindings(row)">管理支付绑定</el-button>
          <el-button link :disabled="!actions(row).canEditMetadata" @click="openEdit(row)">编辑</el-button>
          <el-button link :disabled="!actions(row).canClone" @click="openClone(row)">克隆</el-button>
          <el-button v-if="row.kind === 'TEST'" link type="primary" :disabled="!canView" @click="openWhitelist(row)">查看 / 管理白名单</el-button>
          <el-button v-if="!row.isEnabled" link type="success" :disabled="!actions(row).canEnable" @click="transition(row, 'ENABLE')">启用</el-button>
          <el-button v-else link type="warning" :disabled="!actions(row).canDisable" @click="transition(row, 'DISABLE')">停用</el-button>
          <el-button v-if="row.kind !== 'TEST'" link type="danger" :disabled="!actions(row).canMakeDefault" @click="openDefault(row)">设为默认</el-button>
        </div>
      </template></el-table-column>
    </el-table>

    <el-dialog v-model="validationDialog" title="价格方案完整校验" width="min(660px, 92vw)">
      <el-skeleton v-if="validationInspectionDisplay === 'LOADING'" :rows="4" animated />
      <el-alert v-else-if="validationInspectionDisplay === 'FRESH' && selectedValidation" :type="selectedValidation.valid ? 'success' : 'error'" show-icon :closable="false"
        :title="selectedValidation.valid ? '服务端校验通过' : '服务端返回 valid=false，已禁止启用和切换默认'"
        :description="`校验时间：${selectedValidation.checkedAt}`" />
      <el-alert v-else-if="validationInspectionDisplay === 'STALE_ERROR'" type="warning" show-icon :closable="false"
        title="本次校验加载失败，下面仅展示旧缓存" :description="validationInspection.error" />
      <el-alert v-else-if="validationInspectionDisplay === 'STALE'" type="warning" show-icon :closable="false" title="当前仅有旧缓存，不能视为本次校验" />
      <el-result v-else-if="validationInspectionDisplay === 'ERROR'" icon="error" title="本次校验加载失败" :sub-title="validationInspection.error" />
      <el-table v-if="selectedValidation && validationInspectionDisplay !== 'LOADING'" :data="selectedValidation.checks" style="margin-top: 12px">
        <el-table-column label="结果" width="90"><template #default="{ row }"><el-tag :type="row.passed ? 'success' : 'danger'">{{ row.passed ? "通过" : "阻断" }}</el-tag></template></el-table-column>
        <el-table-column label="检查项" prop="code" min-width="230" />
        <el-table-column label="说明"><template #default="{ row }">{{ row.message || healthIssueLabel(row.code) }}</template></el-table-column>
      </el-table>
    </el-dialog>

    <PaymentBindingDialog v-if="bindingDialog" v-model="bindingDialog" :plan="plan" :price-plan-id="bindingTargetId"
      :current-role="currentRole" :current-permissions="currentPermissions" @success="afterWrite" @view-audit="forwardAudit" />
    <PricePlanEditorDialog v-if="editorOpen" v-model="editorOpen" :mode="editorMode" :plan="plan" :source="editorSource"
      :versions="versions" :has-active-binding="editorHasActiveBinding" :current-role="currentRole" :current-permissions="currentPermissions"
      @success="afterWrite" />
    <DefaultPricePlanSwitchDialog v-if="defaultOpen" v-model="defaultOpen" :plan="plan" :price-plan-id="defaultTargetId"
      :current-role="currentRole" :current-permissions="currentPermissions" @success="afterWrite" />
    <PricePlanWhitelistDialog v-if="whitelistOpen" v-model="whitelistOpen" :plan="plan" :price-plan-id="whitelistTargetId"
      :current-role="currentRole" :current-permissions="currentPermissions" @view-audit="forwardAudit" />
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import {
  formatPriceCents,
  hasPricingPermission,
  healthIssueLabel,
  healthStatusLabel,
  mergePricePlanRows,
  pricePlanBadges,
  pricePlanDisplayFacts,
  pricePlanListDisplayState,
  pricePlanUIActions,
  pricingInspectionState,
  pricingErrorMessage
} from "../../../domain/pricePlanAdmin.ts";
import { usePricePlanAdminStore } from "../../../stores/pricePlanAdmin.ts";
import type { BusinessPlan, PricePlan, PricePlanValidation, PricingAuditFilters } from "../../../types/pricePlanAdmin.ts";
import DefaultPricePlanSwitchDialog from "./DefaultPricePlanSwitchDialog.vue";
import PaymentBindingDialog from "./PaymentBindingDialog.vue";
import PricePlanEditorDialog from "./PricePlanEditorDialog.vue";
import PricePlanWhitelistDialog from "./PricePlanWhitelistDialog.vue";

const props = defineProps<{ plan: BusinessPlan; currentRole: string; currentPermissions: string[] }>();
const emit = defineEmits<{ (event: "view-audit", filters: PricingAuditFilters): void }>();
const store = usePricePlanAdminStore();
const loaded = ref(false);
const freshValidationIds = ref<Set<string>>(new Set());
const filters = reactive({ keyword: "", status: "ALL", kind: "ALL", environment: "ALL" });
const statuses = ["DRAFT", "ACTIVE", "INACTIVE", "EXPIRED"];
const validationDialog = ref(false);
const validationTargetId = ref("");
const validationInspection = reactive({ loading: false, fresh: false, error: "" });
const validationRequestSequence = ref(0);
const bindingDialog = ref(false);
const bindingTargetId = ref("");
const editorOpen = ref(false);
const editorMode = ref<"CREATE" | "EDIT" | "CLONE">("CREATE");
const editorSource = ref<PricePlan | null>(null);
const defaultOpen = ref(false);
const defaultTargetId = ref("");
const whitelistOpen = ref(false);
const whitelistTargetId = ref("");

const loading = computed(() => Boolean(store.loading[`pricePlans:${props.plan.id}`]));
const listError = computed(() => store.errors[`pricePlans:${props.plan.id}`]?.message || "");
const versions = computed(() => store.planVersionsByPlanId[props.plan.id] || []);
const rows = computed(() => mergePricePlanRows({
  plans: store.pricePlansByPlanId[props.plan.id] || [],
  healthPlans: store.health?.pricePlans || [],
  validationsByPricePlanId: store.validationByPricePlanId,
  freshValidationIds: freshValidationIds.value,
  bindingsByPricePlanId: store.bindingsByPricePlanId,
  goodsById: {
    ...Object.fromEntries(store.wechatGoods.map((good) => [good.id, good])),
    ...store.wechatGoodById
  }
}));
const filteredRows = computed(() => rows.value.filter((row) => {
  const keyword = filters.keyword.trim().toLowerCase();
  return (filters.status === "ALL" || row.status === filters.status)
    && (filters.kind === "ALL" || row.kind === filters.kind)
    && (filters.environment === "ALL" || row.environment === filters.environment)
    && (!keyword || [row.pricePlanId, row.code, row.name].some((value) => String(value || "").toLowerCase().includes(keyword)));
}));
const displayState = computed(() => pricePlanListDisplayState({ loading: loading.value, loaded: loaded.value, error: listError.value, rowCount: rows.value.length }));
const principal = computed(() => ({ role: props.currentRole, permissions: props.currentPermissions }));
const canManage = computed(() => hasPricingPermission(principal.value, "pricing:price-plan:manage"));
const canView = computed(() => hasPricingPermission(principal.value, "pricing:plan:view"));
const canViewAudit = computed(() => hasPricingPermission(principal.value, "pricing:audit:view"));
const runtimeAlert = computed(() => {
  if (!store.health) return { type: "warning" as const, title: "运行时安全状态未知", description: "未成功加载 pricing health，启用和默认切换将保持禁用。" };
  if (store.health.runtime?.v132Blocked === true) return { type: "error" as const, title: "V132 安全门禁已阻断", description: "第三阶段完成价值守恒适配前，不得启用会员或代理 V2 订单创建；后端仍是最终边界。" };
  if (store.health.runtime?.v132Blocked !== false) return { type: "warning" as const, title: "V132 安全状态未知", description: "服务端未明确返回 V132 已解除；启用和默认切换将保持禁用。" };
  return null;
});
const selectedValidation = computed<PricePlanValidation | undefined>(() => store.validationByPricePlanId[validationTargetId.value]);
const validationInspectionDisplay = computed(() => pricingInspectionState({
  ...validationInspection,
  hasCached: Boolean(selectedValidation.value)
}));
const editorHasActiveBinding = computed(() => editorSource.value ? (store.bindingsByPricePlanId[editorSource.value.pricePlanId] || [])
  .some((item) => item.enabled && item.status === "ACTIVE") : false);

function viewPricePlanAudit(row: PricePlan) {
  emit("view-audit", { planId: props.plan.id, pricePlanId: row.pricePlanId });
}

function forwardAudit(filters: PricingAuditFilters) {
  emit("view-audit", filters);
}

watch(() => props.plan.id, () => refresh(), { immediate: true });

function actions(row: ReturnType<typeof mergePricePlanRows>[number]) {
  return pricePlanUIActions(row, {
    validationValid: row.validation?.valid === true,
    validationFresh: row.validationFresh === true,
    runtimeSafetyKnown: Boolean(store.health && !store.errors.health && typeof store.health.runtime?.v132Blocked === "boolean"),
    v132Blocked: store.health?.runtime?.v132Blocked,
    paymentDataComplete: row.paymentDataComplete === true,
    hasActiveBinding: Boolean(row.paymentBinding?.enabled === true && row.paymentBinding.status === "ACTIVE")
  }, principal.value);
}

async function refresh() {
  freshValidationIds.value = new Set();
  const base = await Promise.allSettled([
    store.loadPricePlans(props.plan.id), store.loadPlanVersions(props.plan.id), store.loadHealth(), store.loadWechatGoods()
  ]);
  const listed = base[0].status === "fulfilled" ? base[0].value.items : (store.pricePlansByPlanId[props.plan.id] || []);
  await Promise.allSettled(listed.map(async (item) => {
    const [validationLoaded] = await Promise.all([refreshValidation(item.pricePlanId), store.loadPaymentBindings(item.pricePlanId)]);
    if (validationLoaded) await store.loadExactPaymentGood(item.pricePlanId);
  }));
  loaded.value = true;
}

async function refreshValidation(pricePlanId: string) {
  try {
    await store.validatePricePlan(pricePlanId);
    freshValidationIds.value = new Set([...freshValidationIds.value, pricePlanId]);
    return true;
  } catch {
    freshValidationIds.value = new Set([...freshValidationIds.value].filter((item) => item !== pricePlanId));
    return false;
  }
}

async function refreshOne(pricePlanId: string) {
  const results = await Promise.allSettled([
    store.loadPricePlan(pricePlanId), store.loadPricePlans(props.plan.id), store.loadHealth(), refreshValidation(pricePlanId),
    store.loadPaymentBindings(pricePlanId), store.loadWechatGoods()
  ]);
  const failed = results.find((result) => result.status === "rejected");
  if (failed?.status === "rejected") throw failed.reason;
  if (results[3].status === "fulfilled" && results[3].value === true) await store.loadExactPaymentGood(pricePlanId);
}

function openCreate() { editorMode.value = "CREATE"; editorSource.value = null; editorOpen.value = true; }
function openEdit(row: PricePlan) { editorMode.value = "EDIT"; editorSource.value = row; editorOpen.value = true; }
function openClone(row: PricePlan) { editorMode.value = "CLONE"; editorSource.value = row; editorOpen.value = true; }
function openDefault(row: PricePlan) { defaultTargetId.value = row.pricePlanId; defaultOpen.value = true; }
function openWhitelist(row: PricePlan) {
  if (row.kind !== "TEST" || !canView.value) return;
  whitelistTargetId.value = row.pricePlanId;
  whitelistOpen.value = true;
}

async function openValidation(pricePlanId: string) {
  const requestSequence = ++validationRequestSequence.value;
  validationTargetId.value = pricePlanId;
  validationDialog.value = true;
  Object.assign(validationInspection, { loading: true, fresh: false, error: "" });
  freshValidationIds.value = new Set([...freshValidationIds.value].filter((item) => item !== pricePlanId));
  try {
    await store.validatePricePlan(pricePlanId);
    if (requestSequence !== validationRequestSequence.value || validationTargetId.value !== pricePlanId) return;
    freshValidationIds.value = new Set([...freshValidationIds.value, pricePlanId]);
    validationInspection.fresh = true;
  } catch (error) {
    if (requestSequence !== validationRequestSequence.value || validationTargetId.value !== pricePlanId) return;
    validationInspection.error = pricingErrorMessage(error);
  } finally {
    if (requestSequence === validationRequestSequence.value && validationTargetId.value === pricePlanId) validationInspection.loading = false;
  }
}
function openBindings(row: PricePlan) {
  bindingTargetId.value = row.pricePlanId;
  bindingDialog.value = true;
}

async function transition(row: PricePlan, action: "ENABLE" | "DISABLE") {
  try {
    await refreshOne(row.pricePlanId);
    const latest = rows.value.find((item) => item.pricePlanId === row.pricePlanId);
    if (!latest) throw new Error("价格方案刷新后不存在");
    const permitted = actions(latest);
    if (action === "ENABLE" && !permitted.canEnable) {
      return ElMessage.error("新鲜校验、支付数据、V132 或 giftPoints 门禁未全部通过，已禁止启用");
    }
    if (action === "DISABLE" && !permitted.canDisable) return ElMessage.error("当前方案不能停用；默认方案必须先切换默认");
    const prompt = await ElMessageBox.prompt(action === "ENABLE" ? "请输入启用原因" : "请输入停用原因", action === "ENABLE" ? "启用价格方案" : "停用价格方案", {
      inputType: "textarea", inputValidator: (value) => Boolean(String(value || "").trim()) || "必须填写变更原因"
    });
    const input = { revision: Number(latest.revision), changeReason: String(prompt.value).trim() };
    if (action === "ENABLE") await store.enablePricePlan(row.pricePlanId, input);
    else await store.disablePricePlan(row.pricePlanId, input);
    const warning = store.refreshWarnings[`${action === "ENABLE" ? "enable" : "disable"}PricePlan:${row.pricePlanId}`];
    if (warning) ElMessage.warning("写入已成功但刷新失败，请勿重复提交；请手动刷新确认状态");
    else ElMessage.success(action === "ENABLE" ? "价格方案已启用" : "价格方案已停用");
    await refresh();
  } catch (error) {
    if (error === "cancel" || error === "close") return;
    ElMessage.error(pricingErrorMessage(error));
  }
}

async function afterWrite() { await refresh(); }
function displayFacts(row: ReturnType<typeof mergePricePlanRows>[number]) { return pricePlanDisplayFacts(row as unknown as Record<string, unknown>); }
function money(value: unknown) { return formatPriceCents(value); }
function yesNo(value: boolean | null) { return value === true ? "是" : value === false ? "否" : "未知"; }
function kindLabel(value: string) { return ({ NORMAL: "正常价", PROMOTION: "活动价", TEST: "测试价" } as Record<string, string>)[value] || value || "未知"; }
function statusLabel(value: string) { return ({ DRAFT: "草稿", ACTIVE: "启用", INACTIVE: "停用", EXPIRED: "已过期" } as Record<string, string>)[value] || value; }
function statusTag(value: string) { return ({ DRAFT: "info", ACTIVE: "success", INACTIVE: "warning", EXPIRED: "danger" } as Record<string, "info" | "success" | "warning" | "danger">)[value] || "info"; }
function healthTag(value: string) { return ({ HEALTHY: "success", DEGRADED: "warning", BLOCKED: "danger" } as Record<string, "success" | "warning" | "danger" | "info">)[value] || "info"; }
</script>

<style scoped>
.price-plan-list { display: grid; gap: 14px; padding-top: 8px; }
.price-plan-list__toolbar { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.price-plan-list__filters { display: grid; grid-template-columns: minmax(220px, 1fr) repeat(3, 150px); gap: 8px; flex: 1; }
.price-plan-cell { display: grid; gap: 3px; }.price-plan-cell code,.price-plan-cell small,.price-plan-muted { display: block; color: var(--admin-muted); }
.price-plan-badges,.price-plan-actions { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 6px; }
@media (max-width: 980px) { .price-plan-list__toolbar { flex-direction: column; }.price-plan-list__filters { grid-template-columns: 1fr 1fr; width: 100%; } }
</style>
