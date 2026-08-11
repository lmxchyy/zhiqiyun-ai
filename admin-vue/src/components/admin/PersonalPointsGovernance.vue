<template>
  <section class="points-policy-page">
    <header class="points-policy-hero">
      <div>
        <el-tag effect="dark" type="primary">个人赠送积分</el-tag>
        <h2>赠送积分到期策略</h2>
        <p>仅影响注册赠送、活动赠送和后台赠送；充值、付费权益与余额纠正保持永久。</p>
      </div>
      <el-button v-if="actions.canViewPolicy" :loading="store.loading.policy" @click="refreshPolicy">刷新策略</el-button>
    </header>

    <el-alert v-if="!actions.canViewPolicy" title="暂无赠送积分策略查看权限" type="warning" :closable="false" show-icon />
    <template v-else>
      <el-alert
        v-if="errorState"
        :title="errorState.message"
        :type="errorState.conflict ? 'warning' : 'error'"
        :closable="false"
        show-icon
      >
        <template #default>
          <span v-if="errorState.forbidden">需要权限 points:gift-policy:view / points:gift-policy:manage。</span>
          <el-button v-if="errorState.conflict" size="small" @click="refreshPolicy">刷新最新 revision</el-button>
        </template>
      </el-alert>

      <el-skeleton v-if="store.loading.policy && !store.policy" :rows="7" animated />
      <div v-else class="points-policy-grid">
        <el-card shadow="never" class="points-policy-card">
          <template #header><strong>当前生效策略</strong></template>
          <dl class="policy-facts">
            <div><dt>状态</dt><dd>{{ store.policy?.enabled ? "已启用到期" : "已停用到期" }}</dd></div>
            <div><dt>版本 / revision</dt><dd>v{{ store.policy?.version || "-" }} / {{ store.policy?.revision || "-" }}</dd></div>
            <div><dt>周期语义</dt><dd>CALENDAR_MONTH</dd></div>
            <div><dt>业务时区</dt><dd>Asia/Shanghai</dd></div>
            <div><dt>适用来源</dt><dd>REGISTRATION_GIFT、ACTIVITY_GIFT、ADMIN_GIFT</dd></div>
          </dl>
          <el-alert title="按入账时的上海本地时间增加自然月；目标月份没有对应日期时按月末 clamp。" type="info" :closable="false" show-icon />
        </el-card>

        <el-card shadow="never" class="points-policy-card">
          <template #header><strong>发布新策略版本</strong></template>
          <form class="policy-form" @submit.prevent="submitPolicy">
            <label class="switch-row">
              <span>启用赠送积分到期</span>
              <input data-testid="policy-enabled" v-model="form.enabled" type="checkbox" :disabled="!actions.canManagePolicy || store.saving.policy" />
            </label>
            <label>
              <span>自然月数</span>
              <input data-testid="policy-duration" v-model.number="form.durationValue" type="number" min="1" step="1" :disabled="!actions.canManagePolicy || store.saving.policy" />
              <small>默认 3 个月；单位固定为 CALENDAR_MONTH，时区固定为 Asia/Shanghai。</small>
            </label>
            <label>
              <span>变更原因</span>
              <textarea data-testid="policy-reason" v-model.trim="form.changeReason" rows="3" maxlength="500" placeholder="必填，说明为什么调整策略" :disabled="!actions.canManagePolicy || store.saving.policy"></textarea>
            </label>
            <div class="policy-actions">
              <button data-testid="policy-preview" type="button" :disabled="!actions.canManagePolicy || store.saving.policy" @click="previewPolicy">预览变更</button>
            </div>

            <section v-if="preview" class="policy-preview" aria-live="polite">
              <strong>变更预览</strong>
              <p>revision {{ store.policy?.revision || "-" }} → {{ Number(store.policy?.revision || 0) + 1 }}</p>
              <p>{{ preview.enabled ? "启用" : "停用" }}赠送积分到期；启用时按 {{ preview.durationValue }} 个自然月、Asia/Shanghai、月末 clamp 计算。</p>
              <p>原因：{{ preview.changeReason }}</p>
              <label class="confirm-row">
                <input data-testid="policy-confirm" v-model="confirmed" type="checkbox" />
                <span>我确认发布会生成新策略版本，并只影响发布后新入账的赠送批次。</span>
              </label>
            </section>

            <button data-testid="policy-submit" class="primary-button" type="submit" :disabled="!canSubmitPolicy">
              {{ store.saving.policy ? "发布中…" : "确认发布" }}
            </button>
            <p v-if="successMessage" class="success-message">{{ successMessage }}</p>
            <p v-if="localError" class="error-message">{{ localError }}</p>
            <p v-if="!actions.canManagePolicy" class="permission-note">当前账号只有查看权限，不能发布策略。</p>
          </form>
        </el-card>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { buildPolicyMutationPayload, pointAdminActions } from "../../domain/personalPointsAdmin.ts";
import { usePersonalPointsAdminStore } from "../../stores/personalPointsAdmin.ts";
import type { UpdatePointExpiryPolicyRequest } from "../../types/personalPointsAdmin.ts";

const props = defineProps<{ role: string; permissions: string[] }>();
const store = usePersonalPointsAdminStore();
const actions = computed(() => pointAdminActions({ role: props.role, permissions: props.permissions }));
const form = reactive({ enabled: true, durationValue: 3, changeReason: "" });
const preview = ref<UpdatePointExpiryPolicyRequest | null>(null);
const confirmed = ref(false);
const localError = ref("");
const successMessage = ref("");
const errorState = computed(() => store.errors.policy || null);
const canSubmitPolicy = computed(() => actions.value.canManagePolicy
  && Boolean(preview.value)
  && confirmed.value
  && !store.saving.policy);

watch(() => store.policy, (policy) => {
  if (!policy) return;
  form.enabled = policy.enabled;
  form.durationValue = Number(policy.duration_value || 3);
  form.changeReason = "";
}, { immediate: true });
watch(() => [form.enabled, form.durationValue, form.changeReason], () => {
  preview.value = null;
  confirmed.value = false;
  successMessage.value = "";
});

onMounted(() => {
  if (actions.value.canViewPolicy) void refreshPolicy();
});

async function refreshPolicy() {
  localError.value = "";
  await store.loadPolicy().catch(() => undefined);
}

function previewPolicy() {
  localError.value = "";
  try {
    preview.value = buildPolicyMutationPayload({
      revision: Number(store.policy?.revision || 0),
      enabled: form.enabled,
      durationValue: form.durationValue,
      changeReason: form.changeReason
    });
  } catch (error) {
    localError.value = error instanceof Error ? error.message : "策略表单校验失败";
  }
}

async function submitPolicy() {
  if (!preview.value || !confirmed.value || !actions.value.canManagePolicy) return;
  localError.value = "";
  successMessage.value = "";
  try {
    const policy = await store.publishPolicy(preview.value);
    preview.value = null;
    confirmed.value = false;
    successMessage.value = `策略 v${policy.version} 已发布，revision ${policy.revision}。`;
  } catch (error) {
    localError.value = error instanceof Error ? error.message : "策略发布失败";
  }
}
</script>

<style scoped>
.points-policy-page { display: grid; gap: 18px; }.points-policy-hero { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; padding: 22px; border: 1px solid var(--admin-border); border-radius: 14px; background: var(--admin-panel); }.points-policy-hero h2 { margin: 12px 0 8px; color: var(--admin-text); }.points-policy-hero p { margin: 0; color: var(--admin-muted); }.points-policy-grid { display: grid; grid-template-columns: minmax(0, .9fr) minmax(0, 1.1fr); gap: 16px; }.policy-facts { display: grid; gap: 10px; margin: 0 0 16px; }.policy-facts div { display: grid; grid-template-columns: 130px 1fr; gap: 12px; padding: 10px 0; border-bottom: 1px solid var(--admin-border); }.policy-facts dt { color: var(--admin-muted); }.policy-facts dd { margin: 0; color: var(--admin-text); font-weight: 600; overflow-wrap: anywhere; }.policy-form { display: grid; gap: 14px; }.policy-form label:not(.switch-row):not(.confirm-row) { display: grid; gap: 7px; }.policy-form input[type="number"],.policy-form textarea { box-sizing: border-box; width: 100%; padding: 10px 12px; border: 1px solid var(--admin-border); border-radius: 8px; color: var(--admin-text); background: var(--admin-panel); }.policy-form small,.permission-note { color: var(--admin-muted); }.switch-row,.confirm-row { display: flex; align-items: center; gap: 10px; }.policy-actions { display: flex; justify-content: flex-end; }.policy-actions button,.primary-button { padding: 9px 16px; border: 0; border-radius: 8px; cursor: pointer; }.policy-actions button { color: var(--el-color-primary); background: var(--el-color-primary-light-9); }.primary-button { color: #fff; background: var(--el-color-primary); }.policy-actions button:disabled,.primary-button:disabled { cursor: not-allowed; opacity: .5; }.policy-preview { padding: 14px; border: 1px solid var(--el-color-warning-light-5); border-radius: 10px; background: var(--el-color-warning-light-9); }.policy-preview p { margin: 7px 0; }.success-message { color: var(--el-color-success); }.error-message { color: var(--el-color-danger); }
@media (max-width: 980px) { .points-policy-grid { grid-template-columns: 1fr; } }
</style>
