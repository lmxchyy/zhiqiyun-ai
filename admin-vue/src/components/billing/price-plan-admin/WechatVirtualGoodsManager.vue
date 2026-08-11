<template>
  <section class="wechat-goods-manager">
    <el-alert
      type="warning"
      show-icon
      :closable="false"
      :title="WECHAT_MANUAL_CONFIRMATION_NOTICE"
      description="本页只维护本地商品记录与人工核验快照，不会调用微信公众平台，不会创建、发布或修改真实微信商品。"
    />

    <header class="wechat-goods-manager__toolbar">
      <div>
        <strong>微信虚拟商品本地记录</strong>
        <small>当前 V2 套餐：{{ plan.name }}（{{ plan.code }}）</small>
      </div>
      <div>
        <el-button :loading="loading" @click="refresh">刷新商品与引用</el-button>
        <el-button type="primary" :disabled="!canManage" @click="openCreate">新建本地商品</el-button>
      </div>
    </header>

    <el-alert v-if="!canView" type="error" show-icon :closable="false" title="无查看权限" description="需要 pricing:plan:view；后端 RBAC 仍是最终授权边界。" />
    <el-alert v-else-if="listError && rows.length" type="warning" show-icon :closable="false" title="刷新失败，当前显示缓存数据" :description="listError" />
    <el-skeleton v-if="loading && !loaded" :rows="6" animated />
    <el-result v-else-if="canView && !rows.length && listError" icon="error" title="微信商品加载失败" :sub-title="listError"><template #extra><el-button @click="refresh">重试</el-button></template></el-result>
    <el-empty v-else-if="canView && loaded && !rows.length" description="尚无微信虚拟商品本地记录" />

    <el-table v-else-if="canView && rows.length" :data="rows" row-key="id" stripe>
      <el-table-column label="商品" min-width="210">
        <template #default="{ row }"><div class="goods-cell"><strong>{{ row.goodsName }}</strong><code>{{ row.id }}</code><small>revision {{ row.revision }}</small></div></template>
      </el-table-column>
      <el-table-column label="微信标识（本地）" min-width="230">
        <template #default="{ row }"><div class="goods-cell"><code>productId: {{ row.productId }}</code><code>offerId: {{ row.offerId }}</code><small>mode: {{ row.mode }}</small></div></template>
      </el-table-column>
      <el-table-column label="渠道 / 环境" width="190"><template #default="{ row }"><strong>{{ row.channel }}</strong><small class="muted">{{ row.environment }}</small></template></el-table-column>
      <el-table-column label="微信商品价" width="130"><template #default="{ row }"><strong>{{ money(row.platformPriceCents) }}</strong></template></el-table-column>
      <el-table-column label="本地状态" width="160"><template #default="{ row }"><el-tag :type="statusTag(row.status)">{{ row.status }}</el-tag><small class="muted">enabled={{ yesNo(row.enabled) }} · published={{ yesNo(row.published) }}</small></template></el-table-column>
      <el-table-column label="人工确认" min-width="260">
        <template #default="{ row }">
          <div class="goods-cell">
            <el-tag :type="verificationTag(row.verificationStatus)">{{ row.verificationStatus || "UNCONFIRMED" }}</el-tag>
            <small>来源：{{ row.verificationSource || "未知" }}；实时验证：{{ row.platformRealtimeVerified === false ? "否" : "状态异常" }}</small>
            <small>操作人：{{ row.verifiedBy || "未确认" }}；时间：{{ row.verifiedAt || "未确认" }}</small>
            <small>原因：{{ row.verificationReason || "未填写" }}</small>
            <small>证据：{{ row.verificationEvidence || "未填写" }}</small>
            <small>有效至：{{ row.verificationExpiresAt || "未设置" }}</small>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="确认时快照" min-width="260"><template #default="{ row }"><pre class="snapshot">{{ snapshot(row.verificationSnapshot) }}</pre></template></el-table-column>
      <el-table-column label="引用" width="130">
        <template #default="{ row }"><strong>{{ referenceDisplay(row).count ?? "未知" }}</strong><small class="muted">{{ referenceDisplay(row).source }}</small></template>
      </el-table-column>
      <el-table-column label="操作" fixed="right" width="290">
        <template #default="{ row }">
          <div class="goods-actions">
            <el-button v-if="canViewAudit" link type="primary" @click="viewGoodAudit(row)">查看审计</el-button>
            <el-button link type="primary" @click="openReferences(row)">全局引用</el-button>
            <el-button link :disabled="!actions(row).canEdit || staleGoodIds.has(row.id)" @click="openEdit(row)">编辑</el-button>
            <el-button link type="success" :disabled="!actions(row).canConfirm || staleGoodIds.has(row.id)" @click="openConfirmation(row)">人工确认已发布</el-button>
            <el-button link type="danger" :disabled="!actions(row).canDisable || staleGoodIds.has(row.id)" @click="disableGood(row)">停用</el-button>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="editorOpen" :title="editorMode === 'CREATE' ? '新建微信商品本地记录' : '编辑未绑定商品'" width="min(680px, 94vw)" destroy-on-close>
      <el-alert type="info" show-icon :closable="false" title="仅保存本地记录" description="不会连接微信公众平台。已被任一支付绑定引用的商品不能原地修改关键字段。" />
      <el-form label-position="top" class="goods-form">
        <el-form-item label="渠道" required><el-select v-model="goodsForm.channel"><el-option label="微信虚拟支付" value="WECHAT_VIRTUAL" /></el-select></el-form-item>
        <el-form-item label="环境" required><el-select v-model="goodsForm.environment"><el-option label="正式" value="PRODUCTION" /><el-option label="沙箱" value="SANDBOX" /></el-select></el-form-item>
        <el-form-item label="offerId" required><el-input v-model="goodsForm.offerId" /></el-form-item>
        <el-form-item label="productId" required><el-input v-model="goodsForm.productId" /></el-form-item>
        <el-form-item label="商品名称" required><el-input v-model="goodsForm.goodsName" /></el-form-item>
        <el-form-item label="微信道具价格（整数分）" required><el-input-number v-model="goodsForm.platformPriceCents" :min="1" :step="1" controls-position="right" /></el-form-item>
        <el-form-item label="mode" required><el-input v-model="goodsForm.mode" /></el-form-item>
        <el-form-item label="最新 revision"><el-input :model-value="String(goodsForm.revision)" disabled /></el-form-item>
        <el-form-item label="变更原因" required><el-input v-model="goodsForm.changeReason" type="textarea" :rows="3" maxlength="300" show-word-limit /></el-form-item>
      </el-form>
      <el-alert v-if="committedStale" type="warning" show-icon :closable="false" title="服务端已确认写入，但依赖刷新失败" description="当前表单已锁定，禁止重复提交。请重新加载并核对服务端最终状态。" />
      <template #footer><el-button @click="editorOpen = false">取消</el-button><el-button v-if="committedStale" @click="recoverAfterCommittedWrite">重新加载</el-button><el-button type="primary" :loading="saving" :disabled="!editorCanSubmit" @click="submitGood">保存本地记录</el-button></template>
    </el-dialog>

    <el-dialog v-model="confirmationOpen" title="人工确认微信商品已发布" width="min(660px, 94vw)" destroy-on-close>
      <el-alert type="warning" show-icon :closable="false" :title="WECHAT_MANUAL_CONFIRMATION_NOTICE" />
      <el-descriptions v-if="confirmationGood" :column="2" border>
        <el-descriptions-item label="productId">{{ confirmationGood.productId }}</el-descriptions-item>
        <el-descriptions-item label="offerId">{{ confirmationGood.offerId }}</el-descriptions-item>
        <el-descriptions-item label="环境">{{ confirmationGood.environment }}</el-descriptions-item>
        <el-descriptions-item label="价格">{{ money(confirmationGood.platformPriceCents) }}</el-descriptions-item>
        <el-descriptions-item label="mode">{{ confirmationGood.mode }}</el-descriptions-item>
        <el-descriptions-item label="revision">{{ confirmationGood.revision }}</el-descriptions-item>
      </el-descriptions>
      <el-form label-position="top" class="confirm-form">
        <el-form-item label="人工核验原因" required><el-input v-model="confirmationForm.verificationReason" type="textarea" :rows="2" placeholder="说明如何在微信后台人工核对" /></el-form-item>
        <el-form-item label="截图或工单编号（可选）"><el-input v-model="confirmationForm.evidence" placeholder="只填写引用编号或受控证据地址，不上传密钥或凭证" /></el-form-item>
        <el-form-item label="人工确认有效期" required><el-date-picker v-model="confirmationForm.verificationExpiresAt" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" placeholder="选择确认失效时间" /></el-form-item>
        <el-form-item label="本次本地记录变更原因" required><el-input v-model="confirmationForm.changeReason" type="textarea" :rows="2" /></el-form-item>
      </el-form>
      <el-alert v-if="committedStale" type="warning" show-icon :closable="false" title="服务端已确认写入，但依赖刷新失败" description="当前表单已锁定，禁止重复提交。" />
      <template #footer><el-button @click="confirmationOpen = false">取消</el-button><el-button v-if="committedStale" @click="recoverAfterCommittedWrite">重新加载</el-button><el-button type="primary" :loading="saving" :disabled="!confirmationCanSubmit" @click="submitConfirmation">确认本地人工记录</el-button></template>
    </el-dialog>

    <el-drawer v-model="referencesOpen" title="微信商品全局引用" size="min(980px, 96vw)" destroy-on-close>
      <el-alert type="info" show-icon :closable="false" title="引用来自本地定价与订单数据" description="用于判断能否编辑、换绑或停用；不代表微信公众平台实时状态。" />
      <el-skeleton v-if="referencesLoading" :rows="5" animated />
      <el-result v-else-if="referencesError" icon="error" title="引用加载失败" :sub-title="referencesError"><template #extra><el-button @click="reloadReferenceDrawer">重试</el-button></template></el-result>
      <el-empty v-else-if="!selectedReferences.length" description="当前没有支付绑定引用" />
      <el-table v-else :data="selectedReferences" row-key="bindingId" stripe>
        <el-table-column label="套餐 / 价格方案" min-width="250"><template #default="{ row }"><strong>{{ row.planName }}</strong><code>{{ row.planId }}</code><small class="muted">{{ row.pricePlanName }} · {{ row.pricePlanCode }} · {{ row.pricePlanId }}</small></template></el-table-column>
        <el-table-column label="绑定" min-width="190"><template #default="{ row }"><code>{{ row.bindingId }}</code><small class="muted">{{ row.bindingStatus }} · enabled={{ yesNo(row.bindingEnabled) }} · default={{ yesNo(row.isDefault) }}</small></template></el-table-column>
        <el-table-column label="价格" width="190"><template #default="{ row }">方案 {{ money(row.salePriceCents) }}<small class="muted">绑定快照 {{ money(row.providerPriceSnapshotCents) }}</small></template></el-table-column>
        <el-table-column label="渠道 / 环境" width="190"><template #default="{ row }">{{ row.channel }}<small class="muted">{{ row.environment }}</small></template></el-table-column>
        <el-table-column label="历史引用" width="130"><template #default="{ row }">quote {{ row.quoteCount }}<small class="muted">订单 {{ row.orderCount }}</small></template></el-table-column>
      </el-table>
    </el-drawer>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import {
  buildWechatGoodDisableImpact,
  createLatestRequestGate,
  formatPriceCents,
  hasPricingPermission,
  mergeWechatGoodRows,
  pricingErrorMessage,
  WECHAT_MANUAL_CONFIRMATION_NOTICE,
  wechatGoodReferenceDisplay,
  wechatGoodUIActions
} from "../../../domain/pricePlanAdmin.ts";
import { usePricePlanAdminStore } from "../../../stores/pricePlanAdmin.ts";
import type { BusinessPlan, PricingAuditFilters, WechatVirtualGood } from "../../../types/pricePlanAdmin.ts";

const props = defineProps<{ plan: BusinessPlan; currentRole: string; currentPermissions: string[] }>();
const emit = defineEmits<{ (event: "view-audit", filters: PricingAuditFilters): void }>();
const store = usePricePlanAdminStore();
const loaded = ref(false);
const freshReferenceIds = ref<Set<string>>(new Set());
const staleGoodIds = ref<Set<string>>(new Set());
const editorOpen = ref(false);
const editorMode = ref<"CREATE" | "EDIT">("CREATE");
const confirmationOpen = ref(false);
const confirmationGoodId = ref("");
const referencesOpen = ref(false);
const referencesGoodId = ref("");
const referencesLoading = ref(false);
const referencesError = ref("");
const committedStale = ref(false);
const lastMutationKey = ref("");
const refreshGate = createLatestRequestGate();
const referenceDrawerGate = createLatestRequestGate();
const referenceGates = new Map<string, ReturnType<typeof createLatestRequestGate>>();

const goodsForm = reactive({ id: "", revision: 0, channel: "WECHAT_VIRTUAL", environment: "PRODUCTION", offerId: "", productId: "", goodsName: "", platformPriceCents: 1, mode: "short_series_goods", changeReason: "" });
const confirmationForm = reactive({ revision: 0, verificationReason: "", evidence: "", verificationExpiresAt: "", changeReason: "" });
const principal = computed(() => ({ role: props.currentRole, permissions: props.currentPermissions }));
const canView = computed(() => hasPricingPermission(principal.value, "pricing:plan:view"));
const canManage = computed(() => canView.value && hasPricingPermission(principal.value, "pricing:wechat-good:manage"));
const canViewAudit = computed(() => hasPricingPermission(principal.value, "pricing:audit:view"));
const loading = computed(() => Boolean(store.loading.wechatGoods));
const saving = computed(() => Boolean(lastMutationKey.value && store.saving[lastMutationKey.value]));
const listError = computed(() => store.errors.wechatGoods?.message || store.errors.health?.message || "");
const rows = computed(() => mergeWechatGoodRows({ goods: store.wechatGoods, healthGoods: store.health?.wechatGoods || [] }));
const confirmationGood = computed(() => store.wechatGoodById[confirmationGoodId.value]);
const selectedReferences = computed(() => store.wechatGoodReferencesById[referencesGoodId.value] || []);
const editorCanSubmit = computed(() => canManage.value && !saving.value && !committedStale.value
  && Boolean(goodsForm.channel && goodsForm.environment && goodsForm.offerId.trim() && goodsForm.productId.trim() && goodsForm.goodsName.trim() && goodsForm.mode.trim() && goodsForm.changeReason.trim())
  && Number.isInteger(Number(goodsForm.platformPriceCents)) && Number(goodsForm.platformPriceCents) > 0);
const confirmationCanSubmit = computed(() => canManage.value && !saving.value && !committedStale.value && Boolean(
  confirmationGood.value && confirmationForm.verificationReason.trim() && confirmationForm.verificationExpiresAt && confirmationForm.changeReason.trim()
));

function referencesFresh(goodId: string) { return freshReferenceIds.value.has(goodId); }
function referencesFor(goodId: string) { return store.wechatGoodReferencesById[goodId] || []; }
function referenceDisplay(good: { id: string; referenceCount: number | null }) {
  return wechatGoodReferenceDisplay({
    healthCount: good.referenceCount,
    exactTotal: store.wechatGoodReferencePagesById[good.id]?.total,
    exactFresh: referencesFresh(good.id)
  });
}
function actionContext(good: WechatVirtualGood) {
  const references = referencesFor(good.id);
  return {
    good,
    referencesFresh: referencesFresh(good.id),
    referenceCount: store.wechatGoodReferencePagesById[good.id]?.total ?? null,
    hasDefaultActiveDependency: references.some((item) => item.isDefault && item.bindingEnabled && item.bindingStatus === "ACTIVE")
  };
}
function actions(good: WechatVirtualGood) { return wechatGoodUIActions(actionContext(good), principal.value); }

function viewGoodAudit(row: WechatVirtualGood) {
  emit("view-audit", { wechatGoodId: row.id });
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
  const page = await request;
  if (!gate.isLatest(token) || store.requestSequences[`wechatGoodReferences:${goodId}`] !== storeSequence) return page;
  freshReferenceIds.value = new Set([...freshReferenceIds.value, goodId]);
  return page;
}

async function refresh() {
  const token = refreshGate.begin();
  referenceDrawerGate.invalidate();
  for (const gate of referenceGates.values()) gate.invalidate();
  if (!canView.value) { if (refreshGate.isLatest(token)) loaded.value = true; return; }
  freshReferenceIds.value = new Set();
  const base = await Promise.allSettled([store.loadWechatGoods(), store.loadHealth()]);
  if (!refreshGate.isLatest(token)) return;
  const goods = store.wechatGoods;
  await Promise.allSettled(goods.map((good) => loadReferences(good.id)));
  if (!refreshGate.isLatest(token)) return;
  if (!store.errors.wechatGoods && !store.errors.health && goods.every((good) => referencesFresh(good.id))) staleGoodIds.value = new Set();
  loaded.value = true;
}

async function loadLatestActionFacts(goodId: string) {
  const goodRequest = store.loadWechatGood(goodId);
  const goodSequence = store.requestSequences[`wechatGood:${goodId}`];
  const results = await Promise.allSettled([goodRequest, loadReferences(goodId), store.loadHealth()]);
  const failure = results.find((result): result is PromiseRejectedResult => result.status === "rejected");
  if (failure) throw failure.reason;
  if (store.requestSequences[`wechatGood:${goodId}`] !== goodSequence || !referencesFresh(goodId)) {
    throw new Error("PAYMENT_BINDING_CONFIGURATION_CHANGED");
  }
  const latest = store.wechatGoodById[goodId];
  if (!latest) throw new Error("商品刷新后不存在");
  return latest;
}

function resetMutationLock() { committedStale.value = false; lastMutationKey.value = ""; }
function openCreate() {
  resetMutationLock();
  editorMode.value = "CREATE";
  Object.assign(goodsForm, { id: "", revision: 0, channel: "WECHAT_VIRTUAL", environment: "PRODUCTION", offerId: "", productId: "", goodsName: "", platformPriceCents: 1, mode: "short_series_goods", changeReason: "" });
  editorOpen.value = true;
}

async function openEdit(row: WechatVirtualGood) {
  try {
    const latest = await loadLatestActionFacts(row.id);
    if (!actions(latest).canEdit) return ElMessage.error("商品已有绑定、引用数据不新鲜或权限不足，禁止原地编辑。请新建商品后换绑。");
    resetMutationLock();
    editorMode.value = "EDIT";
    Object.assign(goodsForm, { id: latest.id, revision: latest.revision, channel: latest.channel, environment: latest.environment, offerId: latest.offerId, productId: latest.productId, goodsName: latest.goodsName, platformPriceCents: latest.platformPriceCents, mode: latest.mode, changeReason: "" });
    editorOpen.value = true;
  } catch (error) { ElMessage.error(pricingErrorMessage(error)); }
}

async function submitGood() {
  if (!editorCanSubmit.value) return;
  try {
    if (editorMode.value === "EDIT") {
      const latest = await loadLatestActionFacts(goodsForm.id);
      if (latest.revision !== goodsForm.revision) return ElMessage.error("商品 revision 已变化，请关闭后重新打开编辑器。");
      if (!actions(latest).canEdit) return ElMessage.error("最新引用状态已禁止编辑。");
      lastMutationKey.value = `updateWechatGood:${goodsForm.id}`;
      await store.updateWechatGood(goodsForm.id, { revision: goodsForm.revision, channel: goodsForm.channel as "WECHAT_VIRTUAL", environment: goodsForm.environment as "PRODUCTION" | "SANDBOX", offerId: goodsForm.offerId.trim(), productId: goodsForm.productId.trim(), goodsName: goodsForm.goodsName.trim(), platformPriceCents: Number(goodsForm.platformPriceCents), mode: goodsForm.mode.trim(), changeReason: goodsForm.changeReason.trim() });
    } else {
      lastMutationKey.value = "createWechatGood";
      await store.createWechatGood({ channel: goodsForm.channel as "WECHAT_VIRTUAL", environment: goodsForm.environment as "PRODUCTION" | "SANDBOX", offerId: goodsForm.offerId.trim(), productId: goodsForm.productId.trim(), goodsName: goodsForm.goodsName.trim(), platformPriceCents: Number(goodsForm.platformPriceCents), mode: goodsForm.mode.trim(), changeReason: goodsForm.changeReason.trim() });
    }
    if (store.refreshWarnings[lastMutationKey.value]) {
      committedStale.value = true;
      if (goodsForm.id) staleGoodIds.value = new Set([...staleGoodIds.value, goodsForm.id]);
      return ElMessage.warning("写入已成功但依赖刷新失败；表单已锁定，禁止重复提交。");
    }
    ElMessage.success("微信商品本地记录已保存");
    editorOpen.value = false;
    await refresh();
  } catch (error) { ElMessage.error(pricingErrorMessage(error)); }
}

async function openConfirmation(row: WechatVirtualGood) {
  try {
    const latest = await loadLatestActionFacts(row.id);
    if (!actions(latest).canConfirm) return ElMessage.error("当前商品引用状态或权限不足，不能人工确认。");
    resetMutationLock();
    confirmationGoodId.value = latest.id;
    Object.assign(confirmationForm, { revision: latest.revision, verificationReason: "", evidence: "", verificationExpiresAt: "", changeReason: "" });
    confirmationOpen.value = true;
  } catch (error) { ElMessage.error(pricingErrorMessage(error)); }
}

async function submitConfirmation() {
  if (!confirmationCanSubmit.value || !confirmationGood.value) return;
  try {
    const latest = await loadLatestActionFacts(confirmationGood.value.id);
    if (latest.revision !== confirmationForm.revision) return ElMessage.error("商品 revision 已变化，请关闭后重新核验。");
    if (!actions(latest).canConfirm) return ElMessage.error("最新引用状态已禁止人工确认。");
    lastMutationKey.value = `confirmWechatGood:${latest.id}`;
    await store.confirmWechatGood(latest.id, { revision: confirmationForm.revision, verificationReason: confirmationForm.verificationReason.trim(), evidence: confirmationForm.evidence.trim() || undefined, verificationExpiresAt: confirmationForm.verificationExpiresAt, changeReason: confirmationForm.changeReason.trim() });
    if (store.refreshWarnings[lastMutationKey.value]) {
      committedStale.value = true;
      staleGoodIds.value = new Set([...staleGoodIds.value, latest.id]);
      return ElMessage.warning("人工确认已写入，但依赖刷新失败；禁止重复提交。");
    }
    ElMessage.success("本地人工确认记录已保存；系统未实时连接微信公众平台验证。");
    confirmationOpen.value = false;
    await refresh();
  } catch (error) { ElMessage.error(pricingErrorMessage(error)); }
}

async function disableGood(row: WechatVirtualGood) {
  try {
    const latest = await loadLatestActionFacts(row.id);
    const impact = buildWechatGoodDisableImpact(referencesFor(latest.id));
    if (!impact.canDisable) {
      if (impact.unknownDependencies.length) {
        const unknown = impact.unknownDependencies
          .map((item) => `bindingId=${item.bindingId || "未知"}，pricePlanId=${item.pricePlanId || "未知"}`)
          .join("；");
        return ElMessage.error(`PAYMENT_BINDING_CONFIGURATION_CHANGED：支付绑定引用配置已变化或必要字段不完整，禁止停用商品。${unknown}`);
      }
      const dependencies = impact.defaultDependencies.map((item) => `${item.bindingId} / ${item.pricePlanId}`).join("；");
      return ElMessage.error(`商品仍支撑 ACTIVE 默认价格方案，必须先切换默认方案：${dependencies}`);
    }
    if (!actions(latest).canDisable) return ElMessage.error("商品存在默认价格依赖、引用状态未知或权限不足，禁止停用。");
    const affected = impact.affectedBindings.length
      ? impact.affectedBindings.map((item) => `bindingId=${item.bindingId}，pricePlanId=${item.pricePlanId}`).join("；")
      : "无 ACTIVE 非默认支付绑定";
    const prompt = await ElMessageBox.prompt(
      `停用本地商品记录后，后端将在同一数据库事务中级联停用以下 ACTIVE 非默认支付绑定：${affected}。该操作不会停用或修改微信公众平台商品。请输入变更原因。`,
      "停用微信商品本地记录",
      { inputType: "textarea", inputValidator: (value) => Boolean(String(value || "").trim()) || "必须填写变更原因" }
    );
    const key = `disableWechatGood:${latest.id}`;
    await store.disableWechatGood(latest.id, { revision: latest.revision, changeReason: String(prompt.value).trim() });
    if (store.refreshWarnings[key]) {
      staleGoodIds.value = new Set([...staleGoodIds.value, latest.id]);
      return ElMessage.warning("停用已写入但依赖刷新失败；该商品操作已锁定，请勿重复提交。");
    }
    ElMessage.success("微信商品本地记录已停用");
    await refresh();
  } catch (error) { if (error !== "cancel" && error !== "close") ElMessage.error(pricingErrorMessage(error)); }
}

async function openReferences(row: WechatVirtualGood) {
  referencesGoodId.value = row.id;
  referencesOpen.value = true;
  await reloadReferenceDrawer();
}
async function reloadReferenceDrawer() {
  const token = referenceDrawerGate.begin();
  referencesLoading.value = true;
  referencesError.value = "";
  const goodId = referencesGoodId.value;
  try {
    await loadReferences(goodId);
    if (!referenceDrawerGate.isLatest(token) || referencesGoodId.value !== goodId || !referencesFresh(goodId)) return;
  } catch (error) {
    if (referenceDrawerGate.isLatest(token) && referencesGoodId.value === goodId) referencesError.value = pricingErrorMessage(error);
  } finally {
    if (referenceDrawerGate.isLatest(token) && referencesGoodId.value === goodId) referencesLoading.value = false;
  }
}
async function recoverAfterCommittedWrite() {
  await refresh();
  if (listError.value) return;
  if (lastMutationKey.value) store.clearRefreshWarning(lastMutationKey.value);
  resetMutationLock();
  editorOpen.value = false;
  confirmationOpen.value = false;
  ElMessage.success("已重新加载服务端状态，请重新发起需要的操作。");
}

function money(value: unknown) { return formatPriceCents(value); }
function yesNo(value: boolean) { return value ? "是" : "否"; }
function snapshot(value: Record<string, unknown>) { try { return JSON.stringify(value || {}, null, 2); } catch { return "无法展示"; } }
function statusTag(value: string) { return ({ PUBLISHED: "success", DRAFT: "info", DISABLED: "danger" } as Record<string, "success" | "info" | "danger">)[value] || "warning"; }
function verificationTag(value: string) { return ({ MANUALLY_CONFIRMED_PUBLISHED: "success", UNCONFIRMED: "info", PRICE_MISMATCH: "danger", VERIFICATION_EXPIRED: "warning", DISABLED: "danger" } as Record<string, "success" | "info" | "danger" | "warning">)[value] || "info"; }

watch(() => props.plan.id, refresh);
onMounted(refresh);
</script>

<style scoped>
.wechat-goods-manager { display: grid; gap: 14px; padding-top: 8px; }
.wechat-goods-manager__toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.wechat-goods-manager__toolbar > div:first-child { display: grid; gap: 4px; }
.wechat-goods-manager__toolbar small,.muted,.goods-cell small { display: block; color: var(--admin-muted); }
.goods-cell { display: grid; gap: 4px; min-width: 0; }
.goods-cell code { overflow-wrap: anywhere; }
.goods-actions { display: flex; flex-wrap: wrap; gap: 4px; }
.snapshot { max-height: 120px; margin: 0; overflow: auto; color: var(--admin-muted); font-size: 11px; white-space: pre-wrap; overflow-wrap: anywhere; }
.goods-form { display: grid; grid-template-columns: 1fr 1fr; gap: 0 14px; margin-top: 14px; }
.goods-form :deep(.el-form-item:nth-last-child(-n+2)) { grid-column: 1 / -1; }
.confirm-form { margin-top: 14px; }
@media (max-width: 720px) { .wechat-goods-manager__toolbar { align-items: stretch; flex-direction: column; }.goods-form { grid-template-columns: 1fr; }.goods-form :deep(.el-form-item) { grid-column: auto; } }
</style>
