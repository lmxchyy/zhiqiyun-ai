<template>
  <section class="customer-point-actions">
    <el-alert v-if="!hasAnyAccess" title="暂无积分管理权限" type="warning" :closable="false" show-icon />
    <template v-else>
      <header class="point-actions-head">
        <div><strong>会员与个人积分管理</strong><small>{{ userName || userId }}</small></div>
        <button v-if="actions.canViewLots" type="button" :disabled="store.loading[lotKey]" @click="reloadLots">{{ store.loading[lotKey] ? "加载中…" : "刷新批次" }}</button>
      </header>

      <el-alert
        v-if="canGrantMembership"
        type="info"
        :closable="false"
        show-icon
        title="手动会员开通不走支付，也不会自动发放套餐默认 40,000 积分；积分请在下方单独赠送。"
      />

      <div v-if="canGrantMembership || actions.canGift || actions.canCorrect" class="point-operation-grid">
        <form v-if="canGrantMembership" class="point-operation-card membership-card" @submit.prevent="submitMembership">
          <strong>手动开通正式会员</strong>
          <p>仅超级管理员可执行。默认开通知启云 AI Pro 年度会员，可独立设置有效期。</p>
          <label>
            <span>会员套餐</span>
            <select data-testid="membership-plan" v-model="membershipForm.planId">
              <option value="plan_ai_creator_996">知启云AI Pro年度会员（996 元标准套餐）</option>
            </select>
          </label>
          <label><span>有效期（天）</span><input data-testid="membership-days" v-model.number="membershipForm.durationDays" type="number" min="1" max="3650" step="1" /></label>
          <label><span>操作原因</span><textarea data-testid="membership-reason" v-model.trim="membershipForm.reason" rows="2" maxlength="500" placeholder="例如：合作客户赠送年度会员"></textarea></label>
          <label class="confirm-row"><input data-testid="membership-confirm" v-model="membershipForm.confirmed" type="checkbox" /><span>确认直接调整该客户会员权益，不创建支付订单</span></label>
          <button data-testid="membership-submit" type="submit" :disabled="!membershipForm.confirmed || membershipSaving">{{ membershipSaving ? "提交中…" : "确认开通会员" }}</button>
        </form>

        <form v-if="actions.canGift" class="point-operation-card" @submit.prevent="submitGift">
          <strong>赠送积分</strong>
          <p>可为本次赠送单独指定有效期；填 0 时继续使用服务端全局赠送积分到期策略。</p>
          <label><span>赠送积分</span><input data-testid="gift-points" v-model.number="giftForm.points" type="number" min="1" step="1" /></label>
          <label><span>有效期（天）</span><input data-testid="gift-validity-days" v-model.number="giftForm.validityDays" type="number" min="0" max="3650" step="1" /></label>
          <label><span>原因</span><textarea data-testid="gift-reason" v-model.trim="giftForm.reason" rows="2" maxlength="500"></textarea></label>
          <label class="confirm-row"><input data-testid="gift-confirm" v-model="giftForm.confirmed" type="checkbox" /><span>确认向该客户赠送积分</span></label>
          <button data-testid="gift-submit" type="submit" :disabled="!giftForm.confirmed || store.saving.gift">{{ store.saving.gift ? "提交中…" : "确认赠送" }}</button>
          <p v-if="store.errors.gift" class="error-message">{{ store.errors.gift.message }}</p>
        </form>

        <form v-if="actions.canCorrect" class="point-operation-card correction-card" @submit.prevent="submitCorrection">
          <strong>余额纠正</strong>
          <p>独立永久纠正，可输入正数或负数；不是赠送，也不会直接写绝对余额。</p>
          <label><span>纠正增减值</span><input data-testid="correction-points" v-model.number="correctionForm.points" type="number" step="1" /></label>
          <label><span>原因</span><textarea data-testid="correction-reason" v-model.trim="correctionForm.reason" rows="2" maxlength="500"></textarea></label>
          <label class="confirm-row"><input data-testid="correction-confirm" v-model="correctionForm.confirmed" type="checkbox" /><span>确认按增减值执行永久余额纠正</span></label>
          <button data-testid="correction-submit" type="submit" :disabled="!correctionForm.confirmed || store.saving.correction">{{ store.saving.correction ? "提交中…" : "确认纠正" }}</button>
          <p v-if="store.errors.correction" class="error-message">{{ store.errors.correction.message }}</p>
        </form>
      </div>

      <section v-if="actions.canViewLots" class="point-history">
        <div class="point-history-head"><div><strong>积分批次历史</strong><small>来源、授予、到期、余额拆分与状态</small></div></div>
        <el-alert v-if="store.errors[lotKey]" :title="store.errors[lotKey].message" :type="store.errors[lotKey].forbidden ? 'warning' : 'error'" :closable="false" show-icon />
        <div class="table-wrap">
          <table>
            <thead><tr><th>来源</th><th>原始积分</th><th>可用 / 预留</th><th>已消费 / 已过期 / 已反转</th><th>授予时间</th><th>到期时间</th><th>状态</th></tr></thead>
            <tbody>
              <tr v-for="lot in lots" :key="lot.id">
                <td><strong>{{ lot.source_type }}</strong><small>{{ lot.reference_type }} / {{ lot.reference_id }}</small></td>
                <td>{{ lot.original_points }}</td><td>{{ lot.available_points }} / {{ lot.reserved_points }}</td>
                <td>{{ lot.consumed_points }} / {{ lot.expired_points }} / {{ lot.reversed_points }}</td>
                <td>{{ formatDate(lot.granted_at) }}</td><td>{{ lot.expires_at ? formatDate(lot.expires_at) : "永久" }}</td><td>{{ lot.status }}</td>
              </tr>
              <tr v-if="!lots.length && !store.loading[lotKey]"><td colspan="7" class="empty-cell">暂无积分批次</td></tr>
            </tbody>
          </table>
        </div>

        <div class="point-summary-head">
          <strong>批次变动摘要（非独立账本流水）</strong>
          <small>以下 GRANT/EXPIRE 仅由批次授予值和已过期值汇总展示；expired_points = 0 时不生成 EXPIRE，不代表独立账本事件。</small>
        </div>
        <ol class="point-summary-list">
          <li v-for="item in summaries" :key="item.id">
            <b :class="item.type === 'EXPIRE' ? 'is-expire' : 'is-grant'">{{ item.type }}</b>
            <span>{{ item.points }} 积分 · {{ item.sourceType }} · {{ formatDate(item.occurredAt) }}</span>
          </li>
          <li v-if="!summaries.length" class="empty-cell">暂无批次变动摘要</li>
        </ol>
      </section>
    </template>
    <p v-if="successMessage" class="success-message">{{ successMessage }}</p>
    <p v-if="localError" class="error-message">{{ localError }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { buildPointLotSummaries, buildPointMutationPayload, canAccessCustomerPointActions, pointAdminActions } from "../../domain/personalPointsAdmin.ts";
import { personalPointsAdminApi } from "../../api/personalPointsAdmin.ts";
import { usePersonalPointsAdminStore } from "../../stores/personalPointsAdmin.ts";

const props = defineProps<{ userId: string; userName?: string; role: string; permissions: string[] }>();
const store = usePersonalPointsAdminStore();
const actions = computed(() => pointAdminActions({ role: props.role, permissions: props.permissions }));
const canGrantMembership = computed(() => String(props.role || "").trim().toUpperCase() === "SUPER_ADMIN");
const hasAnyAccess = computed(() => canGrantMembership.value || canAccessCustomerPointActions({ role: props.role, permissions: props.permissions }));
const lotKey = computed(() => `lots:${props.userId}`);
const lots = computed(() => store.lotsByUser[props.userId] || []);
const summaries = computed(() => buildPointLotSummaries(lots.value));
const giftForm = reactive({ points: 0, validityDays: 0, reason: "", confirmed: false, idempotencyKey: createIdempotencyKey("gift") });
const correctionForm = reactive({ points: 0, reason: "", confirmed: false, idempotencyKey: createIdempotencyKey("correction") });
const membershipForm = reactive({ planId: "plan_ai_creator_996", durationDays: 365, reason: "", confirmed: false, idempotencyKey: createIdempotencyKey("membership") });
const membershipSaving = ref(false);
const successMessage = ref("");
const localError = ref("");

onMounted(() => {
  if (actions.value.canViewLots) void reloadLots();
});

async function reloadLots() {
  await store.loadLots(props.userId, { limit: 100, offset: 0 }).catch(() => undefined);
}

async function submitMembership() {
  if (!canGrantMembership.value || !membershipForm.confirmed || membershipSaving.value) return;
  localError.value = ""; successMessage.value = ""; membershipSaving.value = true;
  try {
    const response = await personalPointsAdminApi.grantMembership(props.userId, {
      planId: membershipForm.planId,
      durationDays: membershipForm.durationDays,
      reason: membershipForm.reason,
      idempotencyKey: membershipForm.idempotencyKey
    });
    successMessage.value = response.idempotent ? "该会员开通请求已处理，本次未重复变更。" : `会员已开通，有效期至 ${formatDate(response.membership.expiresAt)}。`;
    membershipForm.reason = ""; membershipForm.confirmed = false; membershipForm.idempotencyKey = createIdempotencyKey("membership");
  } catch (error) {
    localError.value = error instanceof Error ? error.message : "手动开通会员失败";
  } finally {
    membershipSaving.value = false;
  }
}

async function submitGift() {
  if (!actions.value.canGift || !giftForm.confirmed) return;
  localError.value = ""; successMessage.value = "";
  try {
    const payload = buildPointMutationPayload(giftForm, "GIFT");
    const result = await store.grantGift(props.userId, payload);
    successMessage.value = result.idempotent ? "该赠送请求已处理，本次未重复增加积分。" : "赠送积分已提交。";
    giftForm.points = 0; giftForm.validityDays = 0; giftForm.reason = ""; giftForm.confirmed = false; giftForm.idempotencyKey = createIdempotencyKey("gift");
    if (actions.value.canViewLots) void reloadLots();
  } catch (error) {
    localError.value = error instanceof Error ? error.message : "赠送积分失败";
  }
}

async function submitCorrection() {
  if (!actions.value.canCorrect || !correctionForm.confirmed) return;
  localError.value = ""; successMessage.value = "";
  try {
    const payload = buildPointMutationPayload(correctionForm, "CORRECTION");
    const result = await store.correctBalance(props.userId, payload);
    successMessage.value = result.idempotent ? "该余额纠正已处理，本次未重复变更。" : "余额纠正已提交。";
    correctionForm.points = 0; correctionForm.reason = ""; correctionForm.confirmed = false; correctionForm.idempotencyKey = createIdempotencyKey("correction");
  } catch (error) {
    localError.value = error instanceof Error ? error.message : "余额纠正失败";
  }
}

function createIdempotencyKey(kind: string) {
  const random = typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  return `admin-${kind}-${random}`;
}
function formatDate(value: string) {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "medium", timeZone: "Asia/Shanghai" }).format(date);
}
</script>

<style scoped>
.customer-point-actions { display: grid; gap: 16px; }.point-actions-head,.point-history-head,.point-summary-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }.point-actions-head div,.point-history-head div,.point-summary-head { display: grid; gap: 4px; }.point-actions-head small,.point-history-head small,.point-summary-head small,.point-operation-card p,.point-operation-card label span,td small { color: var(--admin-muted); }.point-actions-head button,.point-operation-card button { padding: 8px 14px; border: 0; border-radius: 8px; color: #fff; background: var(--el-color-primary); cursor: pointer; }.point-actions-head button:disabled,.point-operation-card button:disabled { opacity: .5; cursor: not-allowed; }.point-operation-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; }.point-operation-card { display: grid; gap: 11px; padding: 16px; border: 1px solid var(--admin-border); border-radius: 10px; background: var(--admin-panel); }.point-operation-card label:not(.confirm-row) { display: grid; gap: 6px; }.point-operation-card input[type="number"],.point-operation-card textarea,.point-operation-card select { box-sizing: border-box; width: 100%; padding: 9px 11px; border: 1px solid var(--admin-border); border-radius: 8px; color: var(--admin-text); background: var(--admin-panel); }.membership-card { border-color: var(--el-color-primary-light-5); }.correction-card { border-color: var(--el-color-warning-light-5); }.confirm-row { display: flex; align-items: center; gap: 8px; }.point-history { display: grid; gap: 13px; }.table-wrap { overflow-x: auto; }table { width: 100%; border-collapse: collapse; min-width: 960px; }th,td { padding: 10px; border-bottom: 1px solid var(--admin-border); text-align: left; color: var(--admin-text); }th { color: var(--admin-muted); font-size: 12px; }td small { display: block; margin-top: 4px; }.point-summary-list { display: grid; gap: 8px; margin: 0; padding: 0; list-style: none; }.point-summary-list li { display: flex; align-items: center; gap: 10px; padding: 9px 11px; border: 1px solid var(--admin-border); border-radius: 8px; }.point-summary-list b { min-width: 58px; }.is-grant { color: var(--el-color-success); }.is-expire { color: var(--el-color-danger); }.empty-cell { color: var(--admin-muted); text-align: center; }.success-message { color: var(--el-color-success); }.error-message { color: var(--el-color-danger); }
@media (max-width: 1200px) { .point-operation-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 900px) { .point-operation-grid { grid-template-columns: 1fr; } }
</style>
