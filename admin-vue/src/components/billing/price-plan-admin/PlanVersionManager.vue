<template>
  <section class="plan-version-manager">
    <header class="plan-version-manager__toolbar">
      <div>
        <h3>权益版本</h3>
        <p>DRAFT 可编辑；ACTIVE 与 RETIRED 为只读快照，变更权益必须克隆为新版本。</p>
      </div>
      <div class="plan-version-manager__toolbar-actions">
        <el-tag v-if="!canManage" type="info" effect="plain">只读权限</el-tag>
        <el-button :icon="Refresh" :loading="listLoading" @click="loadVersions">刷新版本</el-button>
        <el-button v-if="canManage" type="primary" :icon="Plus" @click="openCreate">新建 DRAFT</el-button>
      </div>
    </header>

    <el-alert
      v-if="!canManage"
      type="info"
      show-icon
      :closable="false"
      title="当前账号仅可查看权益版本"
      description="编辑、克隆、激活和退休需要精确权限 pricing:entitlement:manage；admin.full 不会自动获得该权限。"
    />
    <el-alert
      v-if="listError"
      type="error"
      show-icon
      :closable="false"
      title="权益版本加载失败"
      :description="listErrorDescription"
    />

    <el-skeleton v-if="listDisplayState === 'LOADING'" :rows="5" animated />
    <el-empty v-else-if="listDisplayState === 'EMPTY'" description="尚未创建权益版本" />
    <el-timeline v-else-if="listDisplayState === 'LIST'" class="plan-version-timeline">
      <el-timeline-item
        v-for="version in versions"
        :key="version.id"
        :timestamp="`更新时间 ${formatDateTime(version.updatedAt)}`"
        placement="top"
        :type="timelineType(version.status)"
      >
        <article class="plan-version-card">
          <header class="plan-version-card__heading">
            <div>
              <div class="plan-version-card__title">
                <strong>版本 v{{ version.versionNo }}</strong>
                <el-tag :type="statusTagType(version.status)" effect="plain">{{ version.status }}</el-tag>
                <el-tag effect="plain" type="info">revision {{ version.revision }}</el-tag>
              </div>
              <code>{{ version.id }}</code>
            </div>
            <div v-if="canManage || canViewAudit" class="plan-version-card__actions">
              <el-button v-if="canViewAudit" size="small" @click="viewVersionAudit(version)">查看审计</el-button>
              <el-button
                v-if="actionsFor(version).canEdit"
                size="small"
                :loading="Boolean(pricingStore.saving[`updatePlanVersion:${version.id}`])"
                @click="openEdit(version)"
              >编辑 DRAFT</el-button>
              <el-button
                v-if="actionsFor(version).canClone"
                size="small"
                @click="openClone(version)"
              >克隆为新版本</el-button>
              <el-button
                v-if="actionsFor(version).canActivate"
                size="small"
                type="success"
                plain
                :loading="Boolean(pricingStore.saving[`activatePlanVersion:${version.id}`])"
                @click="openTransition(version, 'ACTIVATE')"
              >激活</el-button>
              <el-button
                v-if="actionsFor(version).canRetire"
                size="small"
                type="danger"
                plain
                :loading="Boolean(pricingStore.saving[`retirePlanVersion:${version.id}`])"
                @click="openTransition(version, 'RETIRE')"
              >退休</el-button>
            </div>
          </header>

          <el-alert
            v-if="actionsFor(version).cloneRequired"
            type="warning"
            :closable="false"
            show-icon
            title="当前版本核心权益不可原地修改，请克隆为新版本。"
          />

          <dl class="plan-version-card__facts">
            <div><dt>{{ plan.businessType === 'MEMBER' ? '会员等级' : '代理等级' }}</dt><dd>{{ levelLabel(version) }}</dd></div>
            <div><dt>有效期</dt><dd>{{ version.durationDays }} 天</dd></div>
            <div><dt>Token</dt><dd>{{ formatInteger(version.tokenAmount) }}</dd></div>
            <div><dt>积分</dt><dd>{{ formatInteger(version.pointsAmount) }}</dd></div>
            <div><dt>分润规则版本</dt><dd>{{ version.commissionRuleVersion || '未设置' }}</dd></div>
            <div><dt>生效窗口</dt><dd>{{ effectiveWindow(version) }}</dd></div>
          </dl>

          <div class="plan-version-card__snapshots">
            <details>
              <summary>权益 JSON（{{ jsonSummary(version.rightsSnapshot) }}）</summary>
              <pre>{{ formatJSON(version.rightsSnapshot) }}</pre>
            </details>
            <details>
              <summary>分润快照（{{ jsonSummary(version.commissionSnapshot) }}）</summary>
              <pre>{{ formatJSON(version.commissionSnapshot) }}</pre>
            </details>
          </div>

          <dl class="plan-version-card__audit">
            <div><dt>创建</dt><dd>{{ actorTime(version.createdBy, version.createdAt) }}</dd></div>
            <div><dt>最后修改</dt><dd>{{ actorTime(version.updatedBy, version.updatedAt) }}</dd></div>
            <div v-if="version.activatedAt || version.activatedBy"><dt>激活</dt><dd>{{ actorTime(version.activatedBy, version.activatedAt) }}</dd></div>
            <div v-if="version.retiredAt || version.retiredBy"><dt>退休</dt><dd>{{ actorTime(version.retiredBy, version.retiredAt) }}</dd></div>
            <div><dt>最近原因</dt><dd>{{ version.changeReason || '未记录' }}</dd></div>
          </dl>
        </article>
      </el-timeline-item>
    </el-timeline>

    <el-dialog
      v-model="formVisible"
      :title="formTitle"
      width="760px"
      :close-on-click-modal="false"
      :close-on-press-escape="!formSaving"
      :show-close="!formSaving"
    >
      <el-alert
        v-if="formMode === 'CLONE'"
        type="info"
        show-icon
        :closable="false"
        title="克隆只复制权益、有效期和适用等级，不复制 ID、revision、审计、价格或微信商品。"
      />
      <el-alert v-if="formError" type="error" show-icon :closable="false" title="保存失败" :description="formError" />
      <el-alert v-if="formRefreshWarning" type="warning" show-icon :closable="false" title="写入已成功，但最新权益版本刷新失败"
        :description="`${formRefreshWarning.message}；当前表单保持锁定，请先恢复服务器状态，勿重复提交。`" />
      <div v-if="formCommittedStale" class="plan-version-manager__recovery">
        <strong>服务端已确认本次写入，关闭后重开也不会解除提交锁。</strong>
        <el-button :loading="formRecoveryLoading" @click="recoverFormMutation">重新加载服务器状态并关闭</el-button>
      </div>
      <el-form class="plan-version-form" label-position="top" :model="form">
        <div class="plan-version-form__grid">
          <el-form-item v-if="plan.businessType === 'MEMBER'" label="会员等级" required>
            <el-input v-model="form.memberLevel" maxlength="64" placeholder="例如 PRO" />
          </el-form-item>
          <el-form-item v-else label="代理等级" required>
            <el-input v-model="form.agentLevel" maxlength="64" placeholder="例如 AGENT" />
          </el-form-item>
          <el-form-item label="有效期（天）" required>
            <el-input-number v-model="form.durationDays" :min="0" :precision="0" controls-position="right" />
          </el-form-item>
          <el-form-item label="Token" required>
            <el-input-number v-model="form.tokenAmount" :min="0" :precision="0" controls-position="right" />
          </el-form-item>
          <el-form-item label="积分" required>
            <el-input-number v-model="form.pointsAmount" :min="0" :precision="0" controls-position="right" />
          </el-form-item>
          <el-form-item label="生效时间">
            <el-date-picker
              v-model="form.effectiveAt"
              type="datetime"
              value-format="YYYY-MM-DDTHH:mm:ssZ"
              format="YYYY-MM-DD HH:mm:ss"
              :clearable="false"
              :editable="false"
              placeholder="可选；设置后本阶段不支持清空"
            />
          </el-form-item>
          <el-form-item label="失效时间">
            <el-date-picker
              v-model="form.expiresAt"
              type="datetime"
              value-format="YYYY-MM-DDTHH:mm:ssZ"
              format="YYYY-MM-DD HH:mm:ss"
              :clearable="false"
              :editable="false"
              placeholder="可选；设置后本阶段不支持清空"
            />
          </el-form-item>
          <el-form-item class="is-full" label="分润规则版本">
            <el-input v-model="form.commissionRuleVersion" maxlength="128" placeholder="例如 commission-v2" />
          </el-form-item>
          <el-form-item class="is-full" label="权益 JSON 对象" required>
            <el-input v-model="form.rightsSnapshotText" type="textarea" :rows="8" spellcheck="false" />
          </el-form-item>
          <el-form-item class="is-full" label="分润快照 JSON 对象" required>
            <el-input v-model="form.commissionSnapshotText" type="textarea" :rows="6" spellcheck="false" />
          </el-form-item>
          <el-form-item class="is-full" label="变更原因" required>
            <el-input v-model="form.changeReason" type="textarea" :rows="3" maxlength="500" show-word-limit placeholder="所有写操作都必须填写；会进入领域审计。" />
          </el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button :disabled="formSaving" @click="formVisible = false">取消</el-button>
        <el-button type="primary" :loading="formSaving" :disabled="formCommittedStale" @click="submitForm">{{ formMode === 'EDIT' ? '保存 DRAFT' : '创建 DRAFT' }}</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="transitionVisible"
      :title="transitionMode === 'ACTIVATE' ? '激活权益版本' : '退休权益版本'"
      width="600px"
      :close-on-click-modal="false"
      :close-on-press-escape="!transitionSaving"
      :show-close="!transitionSaving"
    >
      <el-alert v-if="transitionError" type="error" show-icon :closable="false" title="状态变更失败" :description="transitionError" />
      <el-alert v-if="transitionRefreshWarning" type="warning" show-icon :closable="false" title="状态变更已成功，但最新权益版本刷新失败"
        :description="`${transitionRefreshWarning.message}；当前操作保持锁定，请先恢复服务器状态，勿重复提交。`" />
      <div v-if="transitionCommittedStale" class="plan-version-manager__recovery">
        <strong>服务端已确认本次状态变更，关闭后重开也不会解除提交锁。</strong>
        <el-button :loading="transitionRecoveryLoading" @click="recoverTransitionMutation">重新加载服务器状态并关闭</el-button>
      </div>
      <template v-if="transitionTarget">
        <p class="transition-copy">目标：v{{ transitionTarget.versionNo }} · {{ transitionTarget.id }} · revision {{ transitionTarget.revision }}</p>
        <el-alert
          v-if="transitionMode === 'ACTIVATE'"
          type="warning"
          show-icon
          :closable="false"
          :title="activationSummary"
          description="服务端将在同一事务中激活目标 DRAFT，并退休列表中真实的当前 ACTIVE 版本。"
        />
        <el-alert
          v-else
          type="error"
          show-icon
          :closable="false"
          title="退休当前 ACTIVE 后可能出现零个 ACTIVE 权益版本，并阻断依赖该套餐的新报价。"
          description="该操作不会提供恢复按钮；后续需激活一个新的 DRAFT 版本。"
        />
        <el-checkbox v-if="transitionMode === 'RETIRE'" v-model="retireRiskConfirmed" class="transition-risk">
          我已确认零 ACTIVE 风险，并仍要退休此版本
        </el-checkbox>
        <el-form label-position="top">
          <el-form-item label="变更原因" required>
            <el-input v-model="transitionReason" type="textarea" :rows="3" maxlength="500" show-word-limit placeholder="必填；会进入领域审计。" />
          </el-form-item>
        </el-form>
      </template>
      <template #footer>
        <el-button :disabled="transitionSaving" @click="transitionVisible = false">取消</el-button>
        <el-button
          :type="transitionMode === 'ACTIVATE' ? 'success' : 'danger'"
          :loading="transitionSaving"
          :disabled="transitionCommittedStale"
          @click="submitTransition"
        >确认{{ transitionMode === 'ACTIVATE' ? '激活' : '退休' }}</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { ElMessage } from "element-plus";
import { Plus, Refresh } from "@element-plus/icons-vue";
import {
  cloneEntitlementVersionDraft,
  entitlementVersionTransitionPreview,
  entitlementVersionUIActions,
  hasPricingPermission,
  parseEntitlementJSONObject,
  planVersionListDisplayState,
  pricingErrorMessage
} from "../../../domain/pricePlanAdmin.ts";
import { usePricePlanAdminStore } from "../../../stores/pricePlanAdmin.ts";
import type {
  BusinessPlan,
  PlanVersion,
  PlanVersionCreateInput,
  PlanVersionStatus,
  PlanVersionUpdateInput,
  PricingAuditFilters
} from "../../../types/pricePlanAdmin.ts";

interface Props {
  plan: BusinessPlan;
  currentRole: string;
  currentPermissions: string[];
}

type FormMode = "CREATE" | "EDIT" | "CLONE";
type TransitionMode = "ACTIVATE" | "RETIRE";

interface VersionFormState {
  memberLevel: string;
  agentLevel: string;
  durationDays: number;
  tokenAmount: number;
  pointsAmount: number;
  rightsSnapshotText: string;
  commissionRuleVersion: string;
  commissionSnapshotText: string;
  effectiveAt: string;
  expiresAt: string;
  changeReason: string;
}

const props = defineProps<Props>();
const emit = defineEmits<{ (event: "view-audit", filters: PricingAuditFilters): void }>();
const pricingStore = usePricePlanAdminStore();
const formVisible = ref(false);
const formMode = ref<FormMode>("CREATE");
const formTargetId = ref("");
const formBaseRevision = ref(0);
const formOriginalEffectiveAt = ref("");
const formOriginalExpiresAt = ref("");
const formError = ref("");
const formRecoveryLoading = ref(false);
const transitionVisible = ref(false);
const transitionMode = ref<TransitionMode>("ACTIVATE");
const transitionTargetId = ref("");
const transitionBaseRevision = ref(0);
const transitionReason = ref("");
const transitionError = ref("");
const transitionRecoveryLoading = ref(false);
const retireRiskConfirmed = ref(false);
const loadedPlanId = ref("");
const form = reactive<VersionFormState>(emptyForm());

const principal = computed(() => ({ role: props.currentRole, permissions: props.currentPermissions }));
const canManage = computed(() => hasPricingPermission(principal.value, "pricing:entitlement:manage"));
const canViewAudit = computed(() => hasPricingPermission(principal.value, "pricing:audit:view"));
const versions = computed(() => [...(pricingStore.planVersionsByPlanId[props.plan.id] || [])]
  .sort((left, right) => right.versionNo - left.versionNo));
const listKey = computed(() => `planVersions:${props.plan.id}`);
const listLoading = computed(() => Boolean(pricingStore.loading[listKey.value]));
const listError = computed(() => pricingStore.errors[listKey.value]?.message || "");
const listDisplayState = computed(() => planVersionListDisplayState({
  loading: listLoading.value,
  loaded: loadedPlanId.value === props.plan.id,
  error: listError.value,
  versionCount: versions.value.length
}));
const listErrorDescription = computed(() => versions.value.length
  ? `${listError.value}；已加载的版本会保留，请刷新后再执行写操作。`
  : `${listError.value}；未取得有效版本列表，请重试，当前不会按空数据处理。`);
const formTitle = computed(() => formMode.value === "EDIT"
  ? "编辑 DRAFT 权益版本"
  : formMode.value === "CLONE" ? "克隆为新 DRAFT" : "新建 DRAFT 权益版本");
const formSavingKey = computed(() => formMode.value === "EDIT"
  ? `updatePlanVersion:${formTargetId.value}`
  : `createPlanVersion:${props.plan.id}`);
const formSaving = computed(() => Boolean(pricingStore.saving[formSavingKey.value]));
const formRefreshWarning = computed(() => pricingStore.refreshWarnings[formSavingKey.value]);
const formCommittedStale = computed(() => Boolean(pricingStore.planVersionRefreshGatesByMutationKey[formSavingKey.value]));
const transitionTarget = computed(() => versions.value.find((version) => version.id === transitionTargetId.value));
const transitionPreview = computed(() => transitionTarget.value
  ? entitlementVersionTransitionPreview(transitionTarget.value, versions.value)
  : null);
const activationSummary = computed(() => {
  const preview = transitionPreview.value;
  if (!preview?.currentActiveVersionId) return "当前版本列表中没有 ACTIVE 权益版本。";
  if (!preview.willRetireCurrentActive) return `当前 ACTIVE 为 v${preview.currentActiveVersionNo}，目标状态无需替换。`;
  return `当前 ACTIVE v${preview.currentActiveVersionNo}（${preview.currentActiveVersionId}）将在事务中退休。`;
});

function viewVersionAudit(version: PlanVersion) {
  emit("view-audit", { planId: props.plan.id, planVersionId: version.id });
}
const transitionSavingKey = computed(() => `${transitionMode.value === "ACTIVATE" ? "activate" : "retire"}PlanVersion:${transitionTargetId.value}`);
const transitionSaving = computed(() => Boolean(pricingStore.saving[transitionSavingKey.value]));
const transitionRefreshWarning = computed(() => pricingStore.refreshWarnings[transitionSavingKey.value]);
const transitionCommittedStale = computed(() => Boolean(pricingStore.planVersionRefreshGatesByMutationKey[transitionSavingKey.value]));

async function loadVersions() {
  const requestedPlanId = props.plan.id;
  try {
    await pricingStore.loadPlanVersions(requestedPlanId);
    if (props.plan.id === requestedPlanId) loadedPlanId.value = requestedPlanId;
  } catch {
    // Store keeps the last successfully loaded list and exposes the stable error.
  }
}

function emptyForm(): VersionFormState {
  return {
    memberLevel: "",
    agentLevel: "",
    durationDays: 0,
    tokenAmount: 0,
    pointsAmount: 0,
    rightsSnapshotText: "{}",
    commissionRuleVersion: "",
    commissionSnapshotText: "{}",
    effectiveAt: "",
    expiresAt: "",
    changeReason: ""
  };
}

function assignForm(input: Partial<VersionFormState>) {
  Object.assign(form, emptyForm(), input);
}

function actionsFor(version: PlanVersion) {
  return entitlementVersionUIActions(version, principal.value);
}

function openCreate() {
  if (!canManage.value) return;
  formMode.value = "CREATE";
  formTargetId.value = "";
  formBaseRevision.value = 0;
  formOriginalEffectiveAt.value = "";
  formOriginalExpiresAt.value = "";
  formError.value = "";
  assignForm({});
  formVisible.value = true;
}

function openEdit(version: PlanVersion) {
  if (!actionsFor(version).canEdit) return;
  formMode.value = "EDIT";
  formTargetId.value = version.id;
  formBaseRevision.value = version.revision;
  formOriginalEffectiveAt.value = version.effectiveAt || "";
  formOriginalExpiresAt.value = version.expiresAt || "";
  formError.value = "";
  assignForm({
    memberLevel: version.memberLevel || "",
    agentLevel: version.agentLevel || "",
    durationDays: version.durationDays,
    tokenAmount: version.tokenAmount,
    pointsAmount: version.pointsAmount,
    rightsSnapshotText: formatJSON(version.rightsSnapshot),
    commissionRuleVersion: version.commissionRuleVersion || "",
    commissionSnapshotText: formatJSON(version.commissionSnapshot),
    effectiveAt: version.effectiveAt || "",
    expiresAt: version.expiresAt || "",
    changeReason: ""
  });
  formVisible.value = true;
}

function openClone(version: PlanVersion) {
  if (!actionsFor(version).canClone) return;
  const draft = cloneEntitlementVersionDraft(version as unknown as Record<string, unknown>);
  formMode.value = "CLONE";
  formTargetId.value = version.id;
  formBaseRevision.value = version.revision;
  formOriginalEffectiveAt.value = "";
  formOriginalExpiresAt.value = "";
  formError.value = "";
  assignForm({
    memberLevel: draft.memberLevel || "",
    agentLevel: draft.agentLevel || "",
    durationDays: draft.durationDays,
    tokenAmount: draft.tokenAmount,
    pointsAmount: draft.pointsAmount,
    rightsSnapshotText: formatJSON(draft.rightsSnapshot),
    commissionRuleVersion: draft.commissionRuleVersion,
    commissionSnapshotText: formatJSON(draft.commissionSnapshot),
    effectiveAt: draft.effectiveAt || "",
    expiresAt: draft.expiresAt || "",
    changeReason: ""
  });
  formVisible.value = true;
}

function versionFormPayload(): PlanVersionCreateInput {
  const changeReason = form.changeReason.trim();
  if (!changeReason) throw new Error("必须填写变更原因。");
  const memberLevel = form.memberLevel.trim();
  const agentLevel = form.agentLevel.trim();
  if (props.plan.businessType === "MEMBER" && !memberLevel) throw new Error("必须填写会员等级。");
  if (props.plan.businessType === "AGENT" && !agentLevel) throw new Error("必须填写代理等级。");
  const amounts = [form.durationDays, form.tokenAmount, form.pointsAmount];
  if (!amounts.every((value) => Number.isSafeInteger(value) && value >= 0)) {
    throw new Error("有效期、Token 和积分必须是非负整数。");
  }
  if (formOriginalEffectiveAt.value && !form.effectiveAt) throw new Error("当前接口不支持清空生效时间，请保留原值或克隆新版本。");
  if (formOriginalExpiresAt.value && !form.expiresAt) throw new Error("当前接口不支持清空失效时间，请保留原值或克隆新版本。");
  if (form.effectiveAt && Number.isNaN(new Date(form.effectiveAt).getTime())) throw new Error("生效时间格式无效。");
  if (form.expiresAt && Number.isNaN(new Date(form.expiresAt).getTime())) throw new Error("失效时间格式无效。");
  if (form.effectiveAt && form.expiresAt && new Date(form.expiresAt).getTime() <= new Date(form.effectiveAt).getTime()) {
    throw new Error("失效时间必须晚于生效时间。");
  }
  const payload: PlanVersionCreateInput = {
    durationDays: form.durationDays,
    tokenAmount: form.tokenAmount,
    pointsAmount: form.pointsAmount,
    rightsSnapshot: parseEntitlementJSONObject(form.rightsSnapshotText, "权益快照"),
    commissionRuleVersion: form.commissionRuleVersion.trim(),
    commissionSnapshot: parseEntitlementJSONObject(form.commissionSnapshotText, "分润快照"),
    changeReason
  };
  if (props.plan.businessType === "MEMBER") payload.memberLevel = memberLevel;
  else payload.agentLevel = agentLevel;
  if (form.effectiveAt) payload.effectiveAt = form.effectiveAt;
  if (form.expiresAt) payload.expiresAt = form.expiresAt;
  return payload;
}

async function submitForm() {
  if (!canManage.value || formSaving.value || formCommittedStale.value) return;
  formError.value = "";
  try {
    const payload = versionFormPayload();
    if (formMode.value === "EDIT") {
      const current = versions.value.find((version) => version.id === formTargetId.value);
      if (!current || current.status !== "DRAFT" || current.revision !== formBaseRevision.value) {
        throw { code: "REVISION_CONFLICT" };
      }
      await pricingStore.updatePlanVersion(current.id, { ...payload, revision: current.revision } as PlanVersionUpdateInput);
      if (formCommittedStale.value) {
        formError.value = "写入已成功，但最新权益版本尚未恢复；请先重新加载服务器状态。";
        return;
      }
      notifyMutationSuccess("DRAFT 权益版本已保存");
    } else {
      await pricingStore.createPlanVersion(props.plan.id, payload);
      if (formCommittedStale.value) {
        formError.value = "写入已成功，但最新权益版本尚未恢复；请先重新加载服务器状态。";
        return;
      }
      notifyMutationSuccess(formMode.value === "CLONE" ? "已克隆为新的 DRAFT 权益版本" : "DRAFT 权益版本已创建");
    }
    formVisible.value = false;
  } catch (error) {
    formError.value = pricingErrorMessage(error, error instanceof Error ? error.message : "权益版本保存失败，请稍后重试。");
  }
}

function openTransition(version: PlanVersion, mode: TransitionMode) {
  const actions = actionsFor(version);
  if ((mode === "ACTIVATE" && !actions.canActivate) || (mode === "RETIRE" && !actions.canRetire)) return;
  transitionMode.value = mode;
  transitionTargetId.value = version.id;
  transitionBaseRevision.value = version.revision;
  transitionReason.value = "";
  transitionError.value = "";
  retireRiskConfirmed.value = false;
  transitionVisible.value = true;
}

async function submitTransition() {
  if (!canManage.value || transitionSaving.value || transitionCommittedStale.value) return;
  transitionError.value = "";
  try {
    const reason = transitionReason.value.trim();
    if (!reason) throw new Error("必须填写变更原因。");
    if (transitionMode.value === "RETIRE" && !retireRiskConfirmed.value) throw new Error("请先确认零 ACTIVE 风险。");
    const current = versions.value.find((version) => version.id === transitionTargetId.value);
    const expectedStatus: PlanVersionStatus = transitionMode.value === "ACTIVATE" ? "DRAFT" : "ACTIVE";
    if (!current || current.status !== expectedStatus || current.revision !== transitionBaseRevision.value) {
      throw { code: "REVISION_CONFLICT" };
    }
    const input = { revision: current.revision, changeReason: reason };
    if (transitionMode.value === "ACTIVATE") await pricingStore.activatePlanVersion(current.id, input);
    else await pricingStore.retirePlanVersion(current.id, input);
    if (transitionCommittedStale.value) {
      transitionError.value = "状态变更已成功，但最新权益版本尚未恢复；请先重新加载服务器状态。";
      return;
    }
    notifyMutationSuccess(transitionMode.value === "ACTIVATE" ? "权益版本已激活" : "权益版本已退休");
    transitionVisible.value = false;
  } catch (error) {
    transitionError.value = pricingErrorMessage(error, error instanceof Error ? error.message : "权益版本状态变更失败，请稍后重试。");
  }
}

async function recoverFormMutation() {
  if (!formCommittedStale.value) return;
  formRecoveryLoading.value = true;
  try {
    await pricingStore.recoverPlanVersionMutation(formSavingKey.value);
    formError.value = "";
    ElMessage.success("服务器权益版本状态已恢复，本次表单将关闭");
    formVisible.value = false;
  } catch (error) {
    formError.value = pricingErrorMessage(error, "权益版本刷新仍失败，表单继续保持锁定。");
  } finally {
    formRecoveryLoading.value = false;
  }
}

async function recoverTransitionMutation() {
  if (!transitionCommittedStale.value) return;
  transitionRecoveryLoading.value = true;
  try {
    await pricingStore.recoverPlanVersionMutation(transitionSavingKey.value);
    transitionError.value = "";
    ElMessage.success("服务器权益版本状态已恢复，本次状态操作将关闭");
    transitionVisible.value = false;
  } catch (error) {
    transitionError.value = pricingErrorMessage(error, "权益版本刷新仍失败，操作继续保持锁定。");
  } finally {
    transitionRecoveryLoading.value = false;
  }
}

function levelLabel(version: PlanVersion) {
  return props.plan.businessType === "MEMBER" ? (version.memberLevel || "未设置") : (version.agentLevel || "未设置");
}

function notifyMutationSuccess(message: string) {
  if (listError.value) {
    ElMessage.warning(`${message}，但版本列表刷新失败；当前仍显示缓存数据，请手动刷新确认。`);
    return;
  }
  ElMessage.success(message);
}

function formatInteger(value: number) {
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(Number(value || 0));
}

function formatDateTime(value?: string) {
  if (!value) return "未记录";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "medium", hour12: false }).format(date);
}

function effectiveWindow(version: PlanVersion) {
  return `${version.effectiveAt ? formatDateTime(version.effectiveAt) : "立即"} → ${version.expiresAt ? formatDateTime(version.expiresAt) : "长期"}`;
}

function actorTime(actor?: string, value?: string) {
  return `${actor || "未知操作人"} · ${formatDateTime(value)}`;
}

function formatJSON(value: Record<string, unknown>) {
  return JSON.stringify(value || {}, null, 2);
}

function jsonSummary(value: Record<string, unknown>) {
  const count = Object.keys(value || {}).length;
  return count ? `${count} 个顶层字段` : "空对象";
}

function statusTagType(status: PlanVersionStatus) {
  return ({ DRAFT: "warning", ACTIVE: "success", RETIRED: "info" } as const)[status];
}

function timelineType(status: PlanVersionStatus) {
  return ({ DRAFT: "warning", ACTIVE: "success", RETIRED: "info" } as const)[status];
}

watch(
  () => props.plan.id,
  () => {
    loadedPlanId.value = "";
    formVisible.value = false;
    transitionVisible.value = false;
    formError.value = "";
    transitionError.value = "";
    void loadVersions();
  },
  { immediate: true }
);
</script>

<style scoped>
.plan-version-manager { display: grid; gap: 14px; padding-top: 8px; }
.plan-version-manager__toolbar { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; }
.plan-version-manager__toolbar h3 { margin: 0; font-size: 18px; }
.plan-version-manager__toolbar p { margin: 6px 0 0; color: var(--admin-muted); line-height: 1.55; }
.plan-version-manager__toolbar-actions { display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: 8px; }
.plan-version-timeline { padding: 8px 0 0 8px; }
.plan-version-card { display: grid; gap: 14px; min-width: 0; padding: 16px; border: 1px solid var(--admin-border); border-radius: 12px; background: var(--admin-panel-soft); }
.plan-version-card__heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.plan-version-card__heading > div:first-child { display: grid; min-width: 0; gap: 7px; }
.plan-version-card__title { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.plan-version-card__title strong { font-size: 17px; }
.plan-version-card__heading code { overflow-wrap: anywhere; color: var(--admin-muted); }
.plan-version-card__actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 6px; }
.plan-version-card__facts, .plan-version-card__audit { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; margin: 0; }
.plan-version-card__facts div, .plan-version-card__audit div { display: grid; min-width: 0; gap: 5px; padding: 10px; border: 1px solid var(--admin-border); border-radius: 8px; background: var(--admin-panel); }
.plan-version-card dt { color: var(--admin-muted); font-size: 12px; }
.plan-version-card dd { margin: 0; overflow-wrap: anywhere; }
.plan-version-card__audit { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.plan-version-card__snapshots { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
.plan-version-card__snapshots details { min-width: 0; padding: 11px; border: 1px solid var(--admin-border); border-radius: 8px; background: var(--admin-panel); }
.plan-version-card__snapshots summary { cursor: pointer; color: var(--admin-primary-strong); font-weight: 600; }
.plan-version-card__snapshots pre { max-height: 300px; margin: 12px 0 0; overflow: auto; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 12px; line-height: 1.55; }
.plan-version-form { margin-top: 14px; }
.plan-version-form__grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 16px; }
.plan-version-form__grid .is-full { grid-column: 1 / -1; }
.plan-version-form :deep(.el-input-number), .plan-version-form :deep(.el-date-editor) { width: 100%; }
.transition-copy { overflow-wrap: anywhere; color: var(--admin-muted); }
.transition-risk { margin: 14px 0; white-space: normal; }
@media (max-width: 860px) {
  .plan-version-manager__toolbar, .plan-version-card__heading { align-items: stretch; flex-direction: column; }
  .plan-version-manager__toolbar-actions, .plan-version-card__actions { justify-content: flex-start; }
  .plan-version-card__facts, .plan-version-card__audit, .plan-version-card__snapshots, .plan-version-form__grid { grid-template-columns: 1fr; }
  .plan-version-form__grid .is-full { grid-column: auto; }
}
</style>
