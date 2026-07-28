<template>
  <el-drawer :model-value="modelValue" title="切换默认价格方案" size="min(620px, 96vw)" destroy-on-close @close="close">
    <el-skeleton v-if="loading" :rows="8" animated />
    <template v-else>
      <el-alert type="warning" show-icon :closable="false" title="服务端将在提交事务内重新锁定并校验全部配置"
        description="当前预览只用于操作确认；提交时仅发送目标方案最新 revision 与变更原因，不会由前端修改旧默认方案。" />
      <el-alert v-if="loadError" type="error" show-icon :closable="false" title="预览数据加载失败" :description="loadError" />
      <el-descriptions v-if="preview" :column="2" border class="default-preview">
        <el-descriptions-item label="套餐">{{ plan.name }}（{{ plan.id }}）</el-descriptions-item>
        <el-descriptions-item label="分组">{{ target?.channel }} / {{ target?.environment }} / {{ target?.currency }}</el-descriptions-item>
        <el-descriptions-item label="旧默认方案">{{ preview.currentDefault?.name || preview.currentDefault?.pricePlanId || "无" }}</el-descriptions-item>
        <el-descriptions-item label="旧价格">{{ money(preview.currentDefault?.salePriceCents) }}</el-descriptions-item>
        <el-descriptions-item label="旧 bindingId">{{ preview.currentDefaultBinding?.id || "无" }}</el-descriptions-item>
        <el-descriptions-item label="旧绑定价格快照">{{ money(preview.currentDefaultBinding?.providerPriceSnapshotCents) }}</el-descriptions-item>
        <el-descriptions-item label="旧微信商品">{{ preview.currentDefaultGood?.productId || "无" }}</el-descriptions-item>
        <el-descriptions-item label="旧商品 offerId">{{ preview.currentDefaultGood?.offerId || "无" }}</el-descriptions-item>
        <el-descriptions-item label="旧微信商品价格">{{ money(preview.currentDefaultGood?.platformPriceCents) }}</el-descriptions-item>
        <el-descriptions-item label="旧支付身份">{{ preview.currentDefaultValidation?.wechatProductId || "无" }}</el-descriptions-item>
        <el-descriptions-item label="新默认方案">{{ target?.name }}（{{ target?.pricePlanId }}）</el-descriptions-item>
        <el-descriptions-item label="新价格">{{ money(target?.salePriceCents) }}</el-descriptions-item>
        <el-descriptions-item label="新微信商品">{{ preview.good?.productId || "未加载" }}</el-descriptions-item>
        <el-descriptions-item label="新商品 offerId">{{ preview.good?.offerId || "未加载" }}</el-descriptions-item>
        <el-descriptions-item label="新商品价格">{{ money(preview.good?.platformPriceCents) }}</el-descriptions-item>
        <el-descriptions-item label="新绑定价格快照">{{ money(preview.binding?.providerPriceSnapshotCents) }}</el-descriptions-item>
        <el-descriptions-item label="生效时间">{{ target?.validFrom || "立即/由服务端判断" }}</el-descriptions-item>
        <el-descriptions-item label="目标 revision">{{ target?.revision ?? "未知" }}</el-descriptions-item>
        <el-descriptions-item label="目标健康状态">{{ targetHealthFact.status }}</el-descriptions-item>
      </el-descriptions>
      <section v-if="preview?.blockers.length" class="default-preview__blockers">
        <strong>当前禁止切换</strong>
        <el-tag v-for="code in preview.blockers" :key="code" type="danger">{{ blockerLabel(code) }}</el-tag>
      </section>
      <el-alert v-if="preview?.warnings.length" type="warning" show-icon :closable="false" title="旧默认方案存在异常，但允许切换到健康新方案"
        :description="preview.warnings.map(blockerLabel).join('；')" />
      <el-form label-position="top" class="default-preview__form">
        <el-form-item label="变更原因" required><el-input v-model="changeReason" type="textarea" :rows="3" maxlength="300" show-word-limit /></el-form-item>
        <el-checkbox v-model="secondConfirmed">我已核对旧/新价格、微信商品、渠道环境，并确认执行默认切换</el-checkbox>
      </el-form>
      <el-alert v-if="mutationWarning" type="warning" show-icon :closable="false" title="切换已提交成功，但刷新失败"
        :description="`${mutationWarning.message}；请勿重复提交，先手动刷新确认服务端最终状态。`" />
      <el-alert v-if="committedStale" type="warning" show-icon :closable="false" title="服务端已确认切换，当前抽屉已锁定再次提交"
        description="请点击重新加载预览；刷新成功或关闭后重开才能继续。" />
    </template>

    <template #footer>
      <el-button @click="reload">重新加载预览</el-button>
      <el-button type="primary" :loading="saving" :disabled="!canSubmit" @click="submit">确认切换默认方案</el-button>
    </template>
  </el-drawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import {
  buildDefaultPricePlanPreview,
  defaultSwitchCanSubmit,
  defaultSwitchRefreshGate,
  formatPriceCents,
  hasPricingPermission,
  healthIssueLabel,
  pricingHealthPricePlanFact,
  pricingErrorMessage
} from "../../../domain/pricePlanAdmin.ts";
import { usePricePlanAdminStore } from "../../../stores/pricePlanAdmin.ts";
import type { BusinessPlan, PricePlan } from "../../../types/pricePlanAdmin.ts";

const props = defineProps<{
  modelValue: boolean;
  plan: BusinessPlan;
  pricePlanId: string;
  currentRole: string;
  currentPermissions: string[];
}>();
const emit = defineEmits<{ (event: "update:modelValue", value: boolean): void; (event: "success"): void }>();
const store = usePricePlanAdminStore();
const loading = ref(false);
const loadError = ref("");
const validationFresh = ref(false);
const runtimeSafetyKnown = ref(false);
const currentDefaultValidationFresh = ref(false);
const refreshComplete = ref(false);
const changeReason = ref("");
const secondConfirmed = ref(false);
const committedStale = ref(false);

const target = computed(() => store.pricePlanById[props.pricePlanId]
  || store.pricePlansByPlanId[props.plan.id]?.find((item) => item.pricePlanId === props.pricePlanId));
const validation = computed(() => store.validationByPricePlanId[props.pricePlanId]);
const bindings = computed(() => store.bindingsByPricePlanId[props.pricePlanId] || []);
const binding = computed(() => {
  const exactId = validation.value?.paymentBindingId;
  return exactId ? bindings.value.find((item) => item.id === exactId) : undefined;
});
const good = computed(() => {
  const id = validation.value?.wechatGoodId;
  return id && binding.value?.wechatGoodId === id ? store.wechatGoodById[id] : undefined;
});
const currentDefault = computed(() => {
  if (!target.value) return undefined;
  return (store.pricePlansByPlanId[props.plan.id] || []).find((item) => item.isDefault
    && item.planId === target.value?.planId
    && item.channel === target.value?.channel
    && item.environment === target.value?.environment
    && item.currency === target.value?.currency);
});
const currentDefaultValidation = computed(() => currentDefault.value?.pricePlanId === props.pricePlanId
  ? validation.value
  : currentDefault.value ? store.validationByPricePlanId[currentDefault.value.pricePlanId] : undefined);
const currentDefaultBindings = computed(() => currentDefault.value?.pricePlanId === props.pricePlanId
  ? bindings.value
  : currentDefault.value ? store.bindingsByPricePlanId[currentDefault.value.pricePlanId] || [] : []);
const currentDefaultBinding = computed(() => {
  const exactId = currentDefaultValidation.value?.paymentBindingId;
  return exactId ? currentDefaultBindings.value.find((item) => item.id === exactId) : undefined;
});
const currentDefaultGood = computed(() => {
  const id = currentDefaultValidation.value?.wechatGoodId;
  return id && currentDefaultBinding.value?.wechatGoodId === id ? store.wechatGoodById[id] : undefined;
});
const targetHealthFact = computed(() => pricingHealthPricePlanFact(store.health, props.pricePlanId));
const targetHealthAvailable = computed(() => targetHealthFact.value.available);
const preview = computed(() => target.value ? buildDefaultPricePlanPreview({
  target: target.value as unknown as Record<string, unknown>,
  plans: (store.pricePlansByPlanId[props.plan.id] || []) as unknown as Array<Record<string, unknown>>,
  validation: validation.value as unknown as Record<string, unknown> | undefined,
  binding: binding.value as unknown as Record<string, unknown> | undefined,
  good: good.value as unknown as Record<string, unknown> | undefined,
  currentDefaultValidation: currentDefaultValidation.value as unknown as Record<string, unknown> | undefined,
  currentDefaultBinding: currentDefaultBinding.value as unknown as Record<string, unknown> | undefined,
  currentDefaultGood: currentDefaultGood.value as unknown as Record<string, unknown> | undefined,
  currentDefaultValidationFresh: currentDefaultValidationFresh.value,
  validationFresh: validationFresh.value,
  runtimeSafetyKnown: runtimeSafetyKnown.value,
  v132Blocked: store.health?.runtime?.v132Blocked,
  targetHealthAvailable: targetHealthAvailable.value
}) : null);
const principal = computed(() => ({ role: props.currentRole, permissions: props.currentPermissions }));
const canSwitchDefault = computed(() => hasPricingPermission(principal.value, "pricing:price-plan:default"));
const savingKey = computed(() => `makeDefaultPricePlan:${props.pricePlanId}`);
const saving = computed(() => Boolean(store.saving[savingKey.value]));
const mutationWarning = computed(() => store.refreshWarnings[savingKey.value]);
const canSubmit = computed(() => defaultSwitchCanSubmit({
  permission: canSwitchDefault.value,
  previewReady: preview.value?.ready === true,
  secondConfirmed: secondConfirmed.value,
  hasReason: Boolean(changeReason.value.trim()),
  loading: loading.value,
  refreshComplete: refreshComplete.value,
  loadError: loadError.value,
  committedStale: committedStale.value
}));

watch(() => props.modelValue, (open) => {
  if (!open) return;
  changeReason.value = "";
  secondConfirmed.value = false;
  validationFresh.value = false;
  runtimeSafetyKnown.value = false;
  currentDefaultValidationFresh.value = false;
  refreshComplete.value = false;
  committedStale.value = false;
  store.clearRefreshWarning(savingKey.value);
  reload();
}, { immediate: true });

function close() { emit("update:modelValue", false); }
function money(value: unknown) {
  return formatPriceCents(value);
}
function blockerLabel(code: string) {
  const local: Record<string, string> = {
    VALIDATION_NOT_FRESH: "缺少本次打开后取得的新鲜校验",
    RUNTIME_SAFETY_UNKNOWN: "运行时安全状态未知",
    PRICE_PLAN_CONFIGURATION_CHANGED: "配置校验未通过",
    CURRENT_DEFAULT_VALIDATION_NOT_FRESH: "旧默认方案校验未在本轮成功刷新",
    CURRENT_DEFAULT_CONFIGURATION_INVALID: "旧默认方案当前校验异常"
  };
  return local[code] || healthIssueLabel(code);
}

async function reload() {
  if (!props.pricePlanId) return;
  const reloadAfterCommittedWrite = committedStale.value;
  loading.value = true;
  loadError.value = "";
  validationFresh.value = false;
  runtimeSafetyKnown.value = false;
  currentDefaultValidationFresh.value = false;
  refreshComplete.value = false;
  try {
    const results: PromiseSettledResult<unknown>[] = await Promise.allSettled([
      store.loadPricePlan(props.pricePlanId),
      store.loadPricePlans(props.plan.id),
      store.loadHealth(),
      store.validatePricePlan(props.pricePlanId),
      store.loadPaymentBindings(props.pricePlanId),
      store.loadWechatGoods()
    ]);
    let failed = results.find((result) => result.status === "rejected");
    if (!failed) {
      const targetGoodResults = await Promise.allSettled([store.loadExactPaymentGood(props.pricePlanId)]);
      results.push(...targetGoodResults);
      failed = targetGoodResults.find((result) => result.status === "rejected");
    }
    if (!failed && results[0].status === "fulfilled" && results[1].status === "fulfilled") {
      const refreshedTarget = results[0].value as PricePlan;
      const refreshedPlans = (results[1].value as { items: PricePlan[] }).items;
      const oldDefault = refreshedPlans.find((item) => item.isDefault
        && item.planId === refreshedTarget.planId
        && item.channel === refreshedTarget.channel
        && item.environment === refreshedTarget.environment
        && item.currency === refreshedTarget.currency);
      if (oldDefault && oldDefault.pricePlanId !== refreshedTarget.pricePlanId) {
        const oldResults = await Promise.allSettled([
          store.validatePricePlan(oldDefault.pricePlanId),
          store.loadPaymentBindings(oldDefault.pricePlanId)
        ]);
        results.push(...oldResults);
        failed = oldResults.find((result) => result.status === "rejected");
        if (!failed) {
          const oldGoodResults = await Promise.allSettled([store.loadExactPaymentGood(oldDefault.pricePlanId)]);
          results.push(...oldGoodResults);
          failed = oldGoodResults.find((result) => result.status === "rejected");
        }
      }
    }
    const gate = defaultSwitchRefreshGate(results);
    refreshComplete.value = gate.complete;
    validationFresh.value = gate.validationFresh;
    runtimeSafetyKnown.value = gate.runtimeSafetyKnown;
    currentDefaultValidationFresh.value = gate.complete;
    if (failed?.status === "rejected") {
      validationFresh.value = false;
      runtimeSafetyKnown.value = false;
      currentDefaultValidationFresh.value = false;
      refreshComplete.value = false;
      loadError.value = pricingErrorMessage(failed.reason);
    } else if (gate.complete && reloadAfterCommittedWrite) {
      committedStale.value = false;
      store.clearRefreshWarning(savingKey.value);
      ElMessage.success("服务器最终状态已重新加载");
    }
  } finally {
    loading.value = false;
  }
}

async function submit() {
  if (!canSubmit.value || !target.value) return;
  try {
    await ElMessageBox.confirm("服务端会在同一事务内重新校验并切换默认方案。确定继续？", "二次确认", { type: "warning" });
    const response = await store.makeDefaultPricePlan(target.value.pricePlanId, {
      revision: target.value.revision,
      changeReason: changeReason.value.trim()
    });
    committedStale.value = true;
    if (response.alreadyDefault) ElMessage.info("该方案已经是默认方案，服务端按幂等请求处理，未制造重复审计");
    else ElMessage.success("默认价格方案已由服务端事务切换");
    emit("success");
    if (store.refreshWarnings[savingKey.value]) return;
    close();
  } catch (error) {
    if (error === "cancel" || error === "close") return;
    ElMessage.error(pricingErrorMessage(error));
  }
}
</script>

<style scoped>
.default-preview { margin: 16px 0; }
.default-preview__blockers { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; margin: 14px 0; padding: 12px; border: 1px solid var(--el-color-danger-light-5); border-radius: 8px; }
.default-preview__blockers strong { width: 100%; color: var(--el-color-danger); }
.default-preview__form { margin-top: 16px; }
</style>
