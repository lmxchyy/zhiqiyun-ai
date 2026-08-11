<template>
  <el-drawer :model-value="modelValue" title="支付绑定与微信商品" size="min(1040px, 98vw)" destroy-on-close @close="close">
    <el-alert type="warning" show-icon :closable="false" :title="WECHAT_MANUAL_CONFIRMATION_NOTICE" description="支付绑定只关联本地微信商品记录；前端不会提交价格、productId、offerId 或任何微信密钥。" />
    <el-skeleton v-if="loading" :rows="9" animated />
    <template v-else>
      <el-alert v-if="loadError" type="error" show-icon :closable="false" title="支付决策资料加载失败" :description="loadError" />
      <el-alert v-else-if="!configurationFresh" type="warning" show-icon :closable="false" title="支付决策资料不是本轮完整新鲜数据" description="创建、换绑、启用和停用均已 fail-closed。" />

      <section class="binding-facts" aria-label="价格、渠道、环境与确认状态对照">
        <article>
          <span>价格方案</span>
          <strong>{{ latestPlan?.name || pricePlanId }}</strong>
          <dl><dt>方案售价</dt><dd>{{ money(latestPlan?.salePriceCents) }}</dd><dt>渠道</dt><dd>{{ latestPlan?.channel || "未知" }}</dd><dt>环境</dt><dd>{{ latestPlan?.environment || "未知" }}</dd><dt>revision</dt><dd>{{ latestPlan?.revision ?? "未知" }}</dd></dl>
        </article>
        <article>
          <span>支付绑定</span>
          <strong>{{ binding?.id || "未创建" }}</strong>
          <dl><dt>绑定快照价</dt><dd>{{ money(binding?.providerPriceSnapshotCents) }}</dd><dt>渠道</dt><dd>{{ binding?.channel || "未知" }}</dd><dt>环境</dt><dd>{{ binding?.environment || "未知" }}</dd><dt>状态 / revision</dt><dd>{{ binding ? `${binding.status} / ${binding.revision}` : "无" }}</dd></dl>
        </article>
        <article>
          <span>当前微信商品</span>
          <strong>{{ currentGood?.goodsName || "未绑定" }}</strong>
          <dl><dt>微信商品价</dt><dd>{{ money(currentGood?.platformPriceCents) }}</dd><dt>渠道</dt><dd>{{ currentGood?.channel || "未知" }}</dd><dt>环境</dt><dd>{{ currentGood?.environment || "未知" }}</dd><dt>人工确认</dt><dd>{{ currentGood?.verificationStatus || "未知" }}</dd><dt>确认有效至</dt><dd>{{ currentGood?.verificationExpiresAt || "未设置" }}</dd></dl>
        </article>
      </section>

      <el-alert
        :type="pricesAligned && environmentsAligned ? 'success' : 'error'"
        show-icon
        :closable="false"
        :title="pricesAligned && environmentsAligned ? '三价及三组渠道/环境一致' : '价格、渠道或环境存在漂移，操作已阻止'"
        :description="`方案售价 ${money(latestPlan?.salePriceCents)}；绑定快照价 ${money(binding?.providerPriceSnapshotCents)}；微信商品价 ${money(currentGood?.platformPriceCents)}`"
      />

      <section class="binding-validation">
        <header><strong>服务端校验</strong><el-tag :type="validation?.valid === true && validationFresh ? 'success' : 'danger'">{{ validationFresh ? (validation?.valid ? "通过" : "未通过") : "未取得新鲜结果" }}</el-tag></header>
        <el-table v-if="validation?.checks?.length" :data="validation.checks" size="small">
          <el-table-column label="结果" width="80"><template #default="{ row }"><el-tag :type="row.passed ? 'success' : 'danger'">{{ row.passed ? "通过" : "阻止" }}</el-tag></template></el-table-column>
          <el-table-column label="检查项" prop="code" min-width="230" />
          <el-table-column label="说明" prop="message" min-width="260" />
        </el-table>
      </section>

      <section class="binding-mutation">
        <header><div><strong>{{ binding ? "换绑候选商品" : "创建绑定选择商品" }}</strong><small>只提交 wechatGoodId；价格、productId 和快照均由服务端读取或生成。</small></div><el-button @click="reload">刷新全部决策资料</el-button></header>
        <el-select v-model="selectedGoodId" filterable placeholder="选择本地微信商品" class="binding-mutation__selector">
          <el-option v-for="good in selectableGoods" :key="good.id" :value="good.id" :label="`${good.goodsName} · ${good.environment} · ${money(good.platformPriceCents)} · ${good.productId}`" />
        </el-select>
        <el-descriptions v-if="candidateGood" :column="3" border>
          <el-descriptions-item label="商品 ID">{{ candidateGood.id }}</el-descriptions-item>
          <el-descriptions-item label="价格">{{ money(candidateGood.platformPriceCents) }}</el-descriptions-item>
          <el-descriptions-item label="渠道 / 环境">{{ candidateGood.channel }} / {{ candidateGood.environment }}</el-descriptions-item>
          <el-descriptions-item label="人工确认">{{ candidateGood.verificationStatus }}</el-descriptions-item>
          <el-descriptions-item label="确认有效至">{{ candidateGood.verificationExpiresAt || "未设置" }}</el-descriptions-item>
          <el-descriptions-item label="引用资料">{{ candidateReferencesFresh ? "已刷新" : "未刷新" }}</el-descriptions-item>
        </el-descriptions>
        <div v-if="candidateBlockers.length" class="binding-blockers"><el-tag v-for="code in candidateBlockers" :key="code" type="danger">{{ issue(code) }}</el-tag></div>
        <el-form label-position="top">
          <el-form-item label="变更原因" required><el-input v-model="changeReason" type="textarea" :rows="3" maxlength="300" show-word-limit /></el-form-item>
        </el-form>
        <el-alert v-if="currentPolicy.blockers.length || currentPolicy.activationBlockers.length" type="warning" show-icon :closable="false" title="当前绑定操作门禁" :description="[...currentPolicy.blockers, ...currentPolicy.activationBlockers].map(issue).join('；')" />
        <el-alert v-if="committedStale" type="warning" show-icon :closable="false" title="服务端已确认写入，但依赖刷新失败" description="当前抽屉已锁定，禁止重复提交。重新加载成功或关闭后重开才能继续。" />
        <div class="binding-mutation__actions">
          <el-button v-if="binding && canViewAudit" @click="viewBindingAudit(binding)">查看审计</el-button>
          <el-button v-if="!binding" type="primary" :loading="saving" :disabled="!canCreate" @click="createBinding">创建绑定</el-button>
          <el-button v-if="binding" type="primary" :loading="saving" :disabled="!canRebind" @click="rebind">换绑到所选商品</el-button>
          <el-button v-if="binding && !binding.enabled" type="success" :loading="saving" :disabled="!canEnable" @click="transition(true)">启用当前绑定</el-button>
          <el-button v-if="binding?.enabled" type="danger" :loading="saving" :disabled="!canDisable" @click="transition(false)">停用当前绑定</el-button>
          <el-button v-if="committedStale" @click="recoverAfterCommittedWrite">重新加载并解除锁定</el-button>
        </div>
      </section>
    </template>
  </el-drawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import {
  createLatestRequestGate,
  formatPriceCents,
  hasPricingPermission,
  healthIssueLabel,
  paymentBindingEnableReady,
  paymentBindingMutationPolicy,
  pricingErrorMessage,
  WECHAT_MANUAL_CONFIRMATION_NOTICE
} from "../../../domain/pricePlanAdmin.ts";
import { usePricePlanAdminStore } from "../../../stores/pricePlanAdmin.ts";
import type { BusinessPlan, PricePlanPaymentBinding, PricingAuditFilters, WechatVirtualGood } from "../../../types/pricePlanAdmin.ts";

const props = defineProps<{ modelValue: boolean; plan: BusinessPlan; pricePlanId: string; currentRole: string; currentPermissions: string[] }>();
const emit = defineEmits<{
  (event: "update:modelValue", value: boolean): void;
  (event: "success"): void;
  (event: "view-audit", filters: PricingAuditFilters): void;
}>();
const store = usePricePlanAdminStore();
const loading = ref(false);
const loadError = ref("");
const configurationFresh = ref(false);
const validationFresh = ref(false);
const freshReferenceIds = ref<Set<string>>(new Set());
const selectedGoodId = ref("");
const changeReason = ref("");
const committedStale = ref(false);
const lastMutationKey = ref("");
const reloadGate = createLatestRequestGate();
const selectedGoodGate = createLatestRequestGate();
const referenceGates = new Map<string, ReturnType<typeof createLatestRequestGate>>();

const principal = computed(() => ({ role: props.currentRole, permissions: props.currentPermissions }));
const canManage = computed(() => hasPricingPermission(principal.value, "pricing:plan:view") && hasPricingPermission(principal.value, "pricing:price-plan:manage"));
const canViewAudit = computed(() => hasPricingPermission(principal.value, "pricing:audit:view"));
const latestPlan = computed(() => store.pricePlanById[props.pricePlanId] || (store.pricePlansByPlanId[props.plan.id] || []).find((item) => item.pricePlanId === props.pricePlanId));
const bindings = computed(() => store.bindingsByPricePlanId[props.pricePlanId] || []);
const validation = computed(() => store.validationByPricePlanId[props.pricePlanId]);
const binding = computed<PricePlanPaymentBinding | undefined>(() => {
  const exactId = validation.value?.paymentBindingId;
  if (exactId) return bindings.value.find((item) => item.id === exactId);
  return bindings.value.find((item) => item.enabled && item.status === "ACTIVE") || bindings.value[0];
});

function viewBindingAudit(binding: PricePlanPaymentBinding) {
  emit("view-audit", { planId: props.plan.id, pricePlanId: props.pricePlanId, bindingId: binding.id });
}
const currentGood = computed(() => binding.value ? store.wechatGoodById[binding.value.wechatGoodId] : undefined);
const currentReferences = computed(() => currentGood.value ? store.wechatGoodReferencesById[currentGood.value.id] || [] : []);
const candidateGood = computed(() => store.wechatGoodById[selectedGoodId.value] || store.wechatGoods.find((item) => item.id === selectedGoodId.value));
const candidateReferencesFresh = computed(() => Boolean(candidateGood.value && freshReferenceIds.value.has(candidateGood.value.id)));
const selectableGoods = computed(() => store.wechatGoods.filter((good) => good.enabled && good.status !== "DISABLED"));
const currentPolicy = computed(() => paymentBindingMutationPolicy({
  plan: latestPlan.value,
  binding: binding.value,
  good: currentGood.value || candidateGood.value,
  references: binding.value ? currentReferences.value : (candidateGood.value ? store.wechatGoodReferencesById[candidateGood.value.id] || [] : []),
  configurationFresh: configurationFresh.value,
  referencesFresh: binding.value ? Boolean(currentGood.value && freshReferenceIds.value.has(currentGood.value.id)) : candidateReferencesFresh.value
}, principal.value));
const candidateBlockers = computed(() => candidateGoodSafety(candidateGood.value));
const pricesAligned = computed(() => Boolean(latestPlan.value && binding.value && currentGood.value
  && latestPlan.value.salePriceCents === binding.value.providerPriceSnapshotCents
  && binding.value.providerPriceSnapshotCents === currentGood.value.platformPriceCents));
const environmentsAligned = computed(() => Boolean(latestPlan.value && binding.value && currentGood.value
  && latestPlan.value.channel === binding.value.channel && binding.value.channel === currentGood.value.channel
  && latestPlan.value.environment === binding.value.environment && binding.value.environment === currentGood.value.environment));
const saving = computed(() => Boolean(lastMutationKey.value && store.saving[lastMutationKey.value]));
const reasonReady = computed(() => Boolean(changeReason.value.trim()) && canManage.value && !committedStale.value && !saving.value);
const canCreate = computed(() => reasonReady.value && !binding.value && currentPolicy.value.canCreate && candidateBlockers.value.length === 0);
const canRebind = computed(() => reasonReady.value && Boolean(binding.value) && selectedGoodId.value !== binding.value?.wechatGoodId
  && currentPolicy.value.canRebind && candidateReferencesFresh.value && candidateBlockers.value.length === 0);
const canEnable = computed(() => paymentBindingEnableReady({
  reasonReady: reasonReady.value,
  selectedCurrentGood: selectedGoodId.value === binding.value?.wechatGoodId,
  policyCanEnable: currentPolicy.value.canEnable,
  validationFresh: validationFresh.value,
  validationValid: validation.value?.valid === true
}));
const canDisable = computed(() => reasonReady.value && selectedGoodId.value === binding.value?.wechatGoodId && currentPolicy.value.canDisable);

function candidateGoodSafety(good: WechatVirtualGood | undefined) {
  const blockers: string[] = [];
  const plan = latestPlan.value;
  if (!plan || !good || !configurationFresh.value || !candidateReferencesFresh.value) blockers.push("PAYMENT_BINDING_CONFIGURATION_CHANGED");
  if (plan && good && plan.salePriceCents !== good.platformPriceCents) blockers.push("PRICE_PLAN_WECHAT_PRICE_MISMATCH");
  if (plan && good && (plan.channel !== good.channel || plan.environment !== good.environment)) blockers.push("PRICE_PLAN_PAYMENT_ENV_MISMATCH");
  const expiry = good?.verificationExpiresAt ? new Date(good.verificationExpiresAt).getTime() : Number.NaN;
  if (good?.verificationStatus === "VERIFICATION_EXPIRED" || (Number.isFinite(expiry) && expiry <= Date.now())) blockers.push("WECHAT_GOOD_VERIFICATION_EXPIRED");
  if (!good?.enabled || !good?.published || good?.status !== "PUBLISHED" || good?.verificationStatus !== "MANUALLY_CONFIRMED_PUBLISHED" || good?.platformRealtimeVerified !== false) blockers.push("WECHAT_GOOD_NOT_CONFIRMED");
  return [...new Set(blockers)];
}

async function loadReferences(goodId: string) {
  let gate = referenceGates.get(goodId);
  if (!gate) {
    gate = createLatestRequestGate();
    referenceGates.set(goodId, gate);
  }
  const token = gate.begin();
  freshReferenceIds.value = new Set([...freshReferenceIds.value].filter((id) => id !== goodId));
  const request = store.loadWechatGoodReferences(goodId);
  const storeSequence = store.requestSequences[`wechatGoodReferences:${goodId}`];
  await request;
  if (!gate.isLatest(token) || store.requestSequences[`wechatGoodReferences:${goodId}`] !== storeSequence) return false;
  freshReferenceIds.value = new Set([...freshReferenceIds.value, goodId]);
  return true;
}

async function reload() {
  if (!props.modelValue) return;
  const token = reloadGate.begin();
  selectedGoodGate.invalidate();
  for (const gate of referenceGates.values()) gate.invalidate();
  loading.value = true;
  loadError.value = "";
  configurationFresh.value = false;
  validationFresh.value = false;
  freshReferenceIds.value = new Set();
  const requestedGoodId = selectedGoodId.value;
  try {
    const base = await Promise.allSettled([
      store.loadPricePlan(props.pricePlanId),
      store.loadPricePlans(props.plan.id),
      store.loadPaymentBindings(props.pricePlanId),
      store.loadWechatGoods(),
      store.validatePricePlan(props.pricePlanId),
      store.loadHealth()
    ]);
    if (!reloadGate.isLatest(token)) return;
    const failure = base.find((result): result is PromiseRejectedResult => result.status === "rejected");
    if (failure) throw failure.reason;
    validationFresh.value = true;
    const currentId = binding.value?.wechatGoodId || "";
    const firstCandidate = store.wechatGoods.find((good) => good.channel === latestPlan.value?.channel && good.environment === latestPlan.value?.environment && good.platformPriceCents === latestPlan.value?.salePriceCents)?.id || store.wechatGoods[0]?.id || "";
    selectedGoodId.value = requestedGoodId && store.wechatGoods.some((good) => good.id === requestedGoodId) ? requestedGoodId : currentId || firstCandidate;
    const ids = [...new Set([currentId, selectedGoodId.value].filter(Boolean))];
    const details = await Promise.allSettled(ids.flatMap((id) => [store.loadWechatGood(id), loadReferences(id)]));
    if (!reloadGate.isLatest(token)) return;
    const detailsFailure = details.find((result): result is PromiseRejectedResult => result.status === "rejected");
    if (detailsFailure) throw detailsFailure.reason;
    if (ids.some((id) => !freshReferenceIds.value.has(id))) throw new Error("PAYMENT_BINDING_CONFIGURATION_CHANGED");
    configurationFresh.value = true;
  } catch (error) {
    if (reloadGate.isLatest(token)) loadError.value = pricingErrorMessage(error);
  } finally {
    if (reloadGate.isLatest(token)) loading.value = false;
  }
}

async function ensureSelectedGoodFresh() {
  const goodId = selectedGoodId.value;
  if (!goodId) return;
  const token = selectedGoodGate.begin();
  try {
    await Promise.all([store.loadWechatGood(goodId), loadReferences(goodId)]);
    if (!selectedGoodGate.isLatest(token) || selectedGoodId.value !== goodId || !freshReferenceIds.value.has(goodId)) return;
  } catch (error) {
    if (!selectedGoodGate.isLatest(token) || selectedGoodId.value !== goodId) return;
    configurationFresh.value = false;
    loadError.value = pricingErrorMessage(error);
  }
}

async function createBinding() {
  if (!canCreate.value || !candidateGood.value) return;
  await submitMutation("CREATE", async () => {
    lastMutationKey.value = `createBinding:${props.pricePlanId}`;
    return store.createPaymentBinding(props.pricePlanId, { wechatGoodId: candidateGood.value!.id, changeReason: changeReason.value.trim() });
  });
}

async function rebind() {
  if (!canRebind.value || !binding.value || !candidateGood.value) return;
  await submitMutation("REBIND", async () => {
    lastMutationKey.value = `rebindBinding:${binding.value!.id}`;
    return store.rebindPaymentBinding(binding.value!.id, { revision: binding.value!.revision, wechatGoodId: candidateGood.value!.id, changeReason: changeReason.value.trim() });
  });
}

async function transition(enabled: boolean) {
  if ((enabled && !canEnable.value) || (!enabled && !canDisable.value) || !binding.value) return;
  const confirmed = await ElMessageBox.confirm(enabled ? "确认启用当前支付绑定？服务端会再次校验价格、环境和人工确认状态。" : "确认停用当前支付绑定？默认价格依赖会由服务端再次阻止。", enabled ? "启用支付绑定" : "停用支付绑定", { type: "warning", confirmButtonText: "确认", cancelButtonText: "取消" }).catch(() => false);
  if (!confirmed) return;
  await submitMutation(enabled ? "ENABLE" : "DISABLE", async () => {
    lastMutationKey.value = `transitionBinding:${binding.value!.id}`;
    return store.transitionPaymentBinding(binding.value!.id, { revision: binding.value!.revision, enabled, changeReason: changeReason.value.trim() });
  });
}

async function submitMutation(action: string, mutation: () => Promise<unknown>) {
  if (committedStale.value) return;
  await reload();
  if (!configurationFresh.value) return ElMessage.error("最新支付决策资料加载失败，已禁止提交。");
  if (action === "CREATE" && !canCreate.value) return ElMessage.error("最新配置不允许创建绑定。");
  if (action === "REBIND" && !canRebind.value) return ElMessage.error("最新配置、历史引用或候选商品不允许换绑。");
  if (action === "ENABLE" && !canEnable.value) return ElMessage.error("最新价格、环境、人工确认或服务端校验不允许启用。");
  if (action === "DISABLE" && !canDisable.value) return ElMessage.error("最新引用或默认价格依赖不允许停用。");
  try {
    await mutation();
    if (store.refreshWarnings[lastMutationKey.value]) {
      committedStale.value = true;
      return ElMessage.warning("服务端已写入，但完整依赖刷新失败；抽屉已锁定，禁止重复提交。");
    }
    ElMessage.success(action === "CREATE" ? "支付绑定已创建" : action === "REBIND" ? "支付绑定已换绑" : action === "ENABLE" ? "支付绑定已启用" : "支付绑定已停用");
    changeReason.value = "";
    await reload();
    emit("success");
  } catch (error) { ElMessage.error(pricingErrorMessage(error)); }
}

async function recoverAfterCommittedWrite() {
  await reload();
  if (!configurationFresh.value) return;
  if (lastMutationKey.value) store.clearRefreshWarning(lastMutationKey.value);
  committedStale.value = false;
  lastMutationKey.value = "";
  changeReason.value = "";
  ElMessage.success("已重新加载服务端最终状态，操作锁已解除。");
}

function close() {
  reloadGate.invalidate();
  selectedGoodGate.invalidate();
  for (const gate of referenceGates.values()) gate.invalidate();
  loading.value = false;
  configurationFresh.value = false;
  validationFresh.value = false;
  emit("update:modelValue", false);
}
function money(value: unknown) { return formatPriceCents(value); }
function issue(code: string) { return healthIssueLabel(code); }

watch(() => props.modelValue, (open) => { if (open) { committedStale.value = false; lastMutationKey.value = ""; changeReason.value = ""; reload(); } });
watch(selectedGoodId, (next, previous) => { if (props.modelValue && next && next !== previous && !loading.value) ensureSelectedGoodFresh(); });
</script>

<style scoped>
.binding-facts { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; margin: 14px 0; }
.binding-facts article { min-width: 0; padding: 14px; border: 1px solid var(--admin-border); border-radius: 10px; background: var(--admin-panel-soft); }
.binding-facts article > span,.binding-mutation small { display: block; color: var(--admin-muted); font-size: 12px; }
.binding-facts article > strong { display: block; margin: 5px 0 10px; overflow-wrap: anywhere; }
.binding-facts dl { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 6px 10px; margin: 0; font-size: 12px; }
.binding-facts dt { color: var(--admin-muted); }.binding-facts dd { margin: 0; overflow-wrap: anywhere; }
.binding-validation,.binding-mutation { display: grid; gap: 12px; margin-top: 14px; padding: 14px; border: 1px solid var(--admin-border); border-radius: 10px; }
.binding-validation > header,.binding-mutation > header { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.binding-mutation > header > div { display: grid; gap: 3px; }
.binding-mutation__selector { width: 100%; }
.binding-mutation__actions,.binding-blockers { display: flex; flex-wrap: wrap; gap: 8px; }
@media (max-width: 840px) { .binding-facts { grid-template-columns: 1fr; } }
</style>
