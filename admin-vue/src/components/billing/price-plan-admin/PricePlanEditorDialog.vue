<template>
  <el-dialog :model-value="modelValue" :title="dialogTitle" width="min(760px, 92vw)" destroy-on-close @close="close">
    <el-alert v-if="sourceVersionUnavailable" type="warning" show-icon :closable="false"
      title="源方案绑定的权益版本已不是 ACTIVE，后端不允许克隆"
      description="请关闭克隆窗口，使用“新建价格方案”并选择当前 ACTIVE 权益版本。" />
    <el-alert v-else-if="economicBlocker" type="warning" show-icon :closable="false"
      :title="economicBlocker === 'PRICE_PLAN_ACTIVE_BINDING_REQUIRES_DISABLE' ? '请先停用 ACTIVE 支付绑定' : '经济字段必须通过克隆新方案修改'" />
    <el-alert v-else-if="mode === 'CLONE'" type="info" show-icon :closable="false" title="克隆会复制源方案配置并创建新的 DRAFT"
      description="当前接口只接收新编码、名称和变更原因，不复制 ACTIVE 支付绑定；创建后可在新 DRAFT 上编辑经济字段。" />

    <el-form label-position="top" class="price-plan-editor" @submit.prevent>
      <div class="price-plan-editor__grid">
        <el-form-item label="方案编码" required>
          <el-input v-model="form.code" :disabled="mode === 'EDIT'" placeholder="member_campaign" />
          <small>创建后只读；可含中性版本数字，但不得包含 price、yuan、rmb、amount 或明确价格编码。</small>
        </el-form-item>
        <el-form-item label="方案名称" required><el-input v-model="form.name" maxlength="80" /></el-form-item>
        <el-form-item label="权益版本" required>
          <el-select v-model="form.planVersionId" :disabled="mode !== 'CREATE' || !economicEditable" style="width: 100%">
            <el-option v-for="version in activeVersions" :key="version.id" :value="version.id"
              :label="`v${version.versionNo} · revision ${version.revision}`" />
          </el-select>
          <small v-if="mode === 'CREATE'">仅列出本套餐 ACTIVE 权益版本；创建 revision 取所选版本 revision。</small>
        </el-form-item>
        <el-form-item label="方案类型" required>
          <el-select v-model="form.kind" :disabled="!economicEditable" style="width: 100%" @change="normalizeTestScope">
            <el-option label="正常价" value="NORMAL" /><el-option label="活动价" value="PROMOTION" /><el-option label="测试价" value="TEST" />
          </el-select>
        </el-form-item>
        <el-form-item label="渠道"><el-input v-model="form.channel" :disabled="!economicEditable" /></el-form-item>
        <el-form-item label="环境">
          <el-select v-model="form.environment" :disabled="!economicEditable" style="width: 100%">
            <el-option label="正式" value="PRODUCTION" /><el-option label="沙箱" value="SANDBOX" />
          </el-select>
        </el-form-item>
        <el-form-item label="币种"><el-input v-model="form.currency" :disabled="!economicEditable" /></el-form-item>
        <el-form-item label="售价（分）" required><el-input-number v-model="form.salePriceCents" :disabled="!economicEditable" :min="1" :step="1" /></el-form-item>
        <el-form-item label="原价（分）" required><el-input-number v-model="form.listPriceCents" :disabled="!economicEditable" :min="1" :step="1" /></el-form-item>
        <el-form-item label="赠送 Token"><el-input-number v-model="form.giftTokens" :disabled="!economicEditable" :min="0" :step="1" /></el-form-item>
        <el-form-item label="赠送积分"><el-input-number v-model="form.giftPoints" :disabled="!economicEditable" :min="0" :step="1" /></el-form-item>
        <el-form-item label="可见">
          <el-switch v-model="form.isVisible" :disabled="!economicEditable || form.kind === 'TEST'" />
          <small v-if="form.kind === 'TEST'">TEST 固定隐藏、非默认且仅测试范围。</small>
        </el-form-item>
        <el-form-item label="适用范围">
          <el-select v-model="form.audienceType" :disabled="!economicEditable || form.kind === 'TEST'" style="width: 100%">
            <el-option label="公开" value="PUBLIC" /><el-option label="规则" value="RULE" /><el-option label="白名单" value="WHITELIST" /><el-option label="测试" value="TEST" />
          </el-select>
        </el-form-item>
        <el-form-item label="生效时间"><el-date-picker v-model="form.validFrom" :disabled="!economicEditable" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" /></el-form-item>
        <el-form-item label="失效时间"><el-date-picker v-model="form.validUntil" :disabled="!economicEditable" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" /></el-form-item>
      </div>
      <el-form-item label="适用规则 JSON"><el-input v-model="form.audienceRuleText" :disabled="!economicEditable" type="textarea" :rows="3" /></el-form-item>
      <el-form-item label="变更原因" required><el-input v-model="form.changeReason" type="textarea" :rows="2" maxlength="300" show-word-limit /></el-form-item>
      <el-alert v-if="mutationWarning" type="warning" show-icon :closable="false" title="写入已成功，但最新状态刷新失败"
        :description="`${mutationWarning.message}；请保留当前内容并手动刷新，勿重复提交。`" />
      <div v-if="committedStale" class="price-plan-editor__committed">
        <strong>服务端已确认本次写入，当前表单已锁定，不能再次提交。</strong>
        <el-button :loading="refreshingCommitted" @click="refreshCommittedState">重新加载服务器状态并关闭</el-button>
      </div>
    </el-form>

    <template #footer>
      <el-button @click="close">取消</el-button>
      <el-button type="primary" :loading="saving" :disabled="!submitAllowed" @click="submit">{{ committedStale ? "已写入，等待刷新" : submitLabel }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { ElMessage } from "element-plus";
import {
  activeEntitlementVersionOptions,
  hasPricingPermission,
  parseEntitlementJSONObject,
  pricePlanCodeIssue,
  pricePlanEditorEconomicFieldsEditable,
  pricePlanMutationSubmitAllowed,
  pricePlanNameIssue,
  pricePlanUIActions,
  pricingErrorMessage
} from "../../../domain/pricePlanAdmin.ts";
import { usePricePlanAdminStore } from "../../../stores/pricePlanAdmin.ts";
import type { BusinessPlan, PaymentEnvironment, PlanVersion, PricePlan, PricePlanKind } from "../../../types/pricePlanAdmin.ts";

type EditorMode = "CREATE" | "EDIT" | "CLONE";

const props = defineProps<{
  modelValue: boolean;
  mode: EditorMode;
  plan: BusinessPlan;
  source?: PricePlan | null;
  versions: PlanVersion[];
  hasActiveBinding?: boolean;
  currentRole: string;
  currentPermissions: string[];
}>();
const emit = defineEmits<{ (event: "update:modelValue", value: boolean): void; (event: "success", item: PricePlan): void }>();
const store = usePricePlanAdminStore();
const committedStale = ref(false);
const committedItem = ref<PricePlan | null>(null);
const refreshingCommitted = ref(false);

const emptyForm = () => ({
  code: "", name: "", planVersionId: "", kind: "NORMAL" as PricePlanKind,
  channel: "WECHAT_VIRTUAL", environment: "SANDBOX", currency: "CNY",
  salePriceCents: 100, listPriceCents: 100, giftPoints: 0, giftTokens: 0,
  validFrom: "", validUntil: "", audienceType: "PUBLIC", audienceRuleText: "{}", isVisible: true, changeReason: ""
});
const form = reactive(emptyForm());
const principal = computed(() => ({ role: props.currentRole, permissions: props.currentPermissions }));
const canManage = computed(() => hasPricingPermission(principal.value, "pricing:price-plan:manage"));
const activeVersions = computed(() => activeEntitlementVersionOptions(props.versions, props.plan.id));
const sourceVersionUnavailable = computed(() => props.mode === "CLONE"
  && !activeVersions.value.some((version) => version.id === props.source?.planVersionId));
const actionContext = computed(() => pricePlanUIActions(props.source || { status: "DRAFT" }, {
  validationValid: false, validationFresh: false, runtimeSafetyKnown: false, paymentDataComplete: false,
  hasActiveBinding: props.hasActiveBinding === true
}, principal.value));
const economicEditable = computed(() => pricePlanEditorEconomicFieldsEditable(props.mode, actionContext.value));
const economicBlocker = computed(() => props.mode === "EDIT" ? actionContext.value.economicBlocker : "");
const mutationKey = computed(() => props.mode === "CREATE" ? `createPricePlan:${props.plan.id}`
  : props.mode === "CLONE" ? `clonePricePlan:${props.source?.pricePlanId || ""}` : `updatePricePlan:${props.source?.pricePlanId || ""}`);
const saving = computed(() => Boolean(store.saving[mutationKey.value]));
const mutationWarning = computed(() => store.refreshWarnings[mutationKey.value]);
const submitAllowed = computed(() => pricePlanMutationSubmitAllowed({
  allowedByPolicy: canManage.value && !sourceVersionUnavailable.value,
  saving: saving.value,
  committedStale: committedStale.value
}));
const dialogTitle = computed(() => props.mode === "CREATE" ? "新建价格方案" : props.mode === "CLONE" ? "克隆为新价格方案" : "编辑价格方案");
const submitLabel = computed(() => props.mode === "CLONE" ? "创建克隆方案" : "保存");

watch(() => props.modelValue, (open) => {
  if (!open) return;
  const persistedGate = store.pricePlanRefreshGatesByMutationKey[mutationKey.value];
  committedStale.value = Boolean(persistedGate);
  committedItem.value = persistedGate ? store.pricePlanById[persistedGate.pricePlanId] || null : null;
  store.clearRefreshWarning(mutationKey.value);
  Object.assign(form, emptyForm());
  const source = props.source;
  if (source) Object.assign(form, {
    code: props.mode === "CLONE" ? "" : source.code,
    name: props.mode === "CLONE" ? "" : source.name,
    planVersionId: source.planVersionId,
    kind: source.kind,
    channel: source.channel,
    environment: source.environment,
    currency: source.currency,
    salePriceCents: source.salePriceCents,
    listPriceCents: source.listPriceCents,
    giftPoints: source.giftPoints,
    giftTokens: source.giftTokens,
    validFrom: source.validFrom || "",
    validUntil: source.validUntil || "",
    audienceType: source.audienceType,
    audienceRuleText: JSON.stringify(source.audienceRule || {}, null, 2),
    isVisible: source.isVisible,
    changeReason: ""
  });
  if (props.mode === "CREATE" && activeVersions.value.length === 1) form.planVersionId = activeVersions.value[0].id;
  normalizeTestScope();
}, { immediate: true });

function normalizeTestScope() {
  if (form.kind === "TEST") {
    form.isVisible = false;
    form.audienceType = "TEST";
  }
}

function close() { emit("update:modelValue", false); }

async function submit() {
  if (!submitAllowed.value) return;
  normalizeTestScope();
  const reason = form.changeReason.trim();
  if (!reason) return ElMessage.warning("请填写变更原因");
  const name = form.name.trim();
  if (pricePlanNameIssue(name)) return ElMessage.warning("请填写方案名称");
  if (props.mode !== "EDIT") {
    const codeIssue = pricePlanCodeIssue(form.code);
    if (codeIssue) return ElMessage.error(pricingErrorMessage({ code: codeIssue }));
  }
  if (form.listPriceCents < form.salePriceCents) return ElMessage.error("原价不能低于售价");
  try {
    let response;
    if (props.mode === "CREATE") {
      const version = activeVersions.value.find((item) => item.id === form.planVersionId);
      if (!version) return ElMessage.error("请选择当前 ACTIVE 权益版本");
      response = await store.createPricePlan(props.plan.id, {
        revision: version.revision,
        planVersionId: version.id,
        code: form.code.trim(), name, kind: form.kind,
        channel: form.channel, environment: form.environment as PaymentEnvironment, currency: form.currency,
        salePriceCents: form.salePriceCents, listPriceCents: form.listPriceCents,
        giftPoints: form.giftPoints, giftTokens: form.giftTokens,
        ...(form.validFrom ? { validFrom: form.validFrom } : {}),
        ...(form.validUntil ? { validUntil: form.validUntil } : {}),
        audienceType: form.audienceType,
        audienceRule: parseEntitlementJSONObject(form.audienceRuleText, "适用规则"),
        isVisible: form.isVisible,
        changeReason: reason
      });
    } else if (props.mode === "CLONE") {
      if (!props.source || sourceVersionUnavailable.value) return;
      response = await store.clonePricePlan(props.source.pricePlanId, {
        revision: props.source.revision, code: form.code.trim(), name, changeReason: reason
      });
    } else {
      if (!props.source) return;
      const payload: Record<string, unknown> = { revision: props.source.revision, name, changeReason: reason };
      if (economicEditable.value) Object.assign(payload, {
        planVersionId: form.planVersionId, kind: form.kind, channel: form.channel, environment: form.environment,
        currency: form.currency, salePriceCents: form.salePriceCents, listPriceCents: form.listPriceCents,
        giftPoints: form.giftPoints, giftTokens: form.giftTokens,
        audienceType: form.audienceType, audienceRule: parseEntitlementJSONObject(form.audienceRuleText, "适用规则"),
        isVisible: form.isVisible,
        ...(form.validFrom ? { validFrom: form.validFrom } : { clearValidFrom: true }),
        ...(form.validUntil ? { validUntil: form.validUntil } : { clearValidUntil: true })
      });
      response = await store.updatePricePlan(props.source.pricePlanId, payload as never);
    }
    committedItem.value = response.item;
    committedStale.value = true;
    emit("success", response.item);
    if (store.refreshWarnings[mutationKey.value]) {
      ElMessage.warning("写入成功，但刷新失败；请手动刷新后再继续操作");
      return;
    }
    ElMessage.success(props.mode === "CLONE" ? "已创建新的 DRAFT 方案，未复制支付绑定" : "价格方案已保存");
    close();
  } catch (error) {
    ElMessage.error(pricingErrorMessage(error));
  }
}

async function refreshCommittedState() {
  const persistedGate = store.pricePlanRefreshGatesByMutationKey[mutationKey.value];
  if (!persistedGate && !committedItem.value) return;
  refreshingCommitted.value = true;
  try {
    if (persistedGate) {
      await store.recoverPricePlanMutation(mutationKey.value);
    } else if (committedItem.value) {
      await store.refreshPricePlanDecisionResources(committedItem.value.planId, committedItem.value.pricePlanId);
      store.clearRefreshWarning(mutationKey.value);
    }
    committedStale.value = false;
    ElMessage.success("服务器状态已重新加载，本次表单将关闭");
    close();
  } catch (error) {
    ElMessage.error(`刷新仍失败：${pricingErrorMessage(error)}；表单继续保持锁定。`);
  } finally {
    refreshingCommitted.value = false;
  }
}
</script>

<style scoped>
.price-plan-editor { display: grid; gap: 12px; }
.price-plan-editor__grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 16px; }
.price-plan-editor small { display: block; margin-top: 5px; color: var(--admin-muted); line-height: 1.45; }
.price-plan-editor__committed { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px; border: 1px solid var(--el-color-warning-light-5); border-radius: 8px; color: var(--el-color-warning-dark-2); }
@media (max-width: 720px) { .price-plan-editor__grid { grid-template-columns: 1fr; } }
</style>
