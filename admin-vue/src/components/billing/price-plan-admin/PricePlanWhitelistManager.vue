<template>
  <section class="price-plan-whitelist-manager">
    <header class="whitelist-toolbar">
      <div>
        <h3>TEST 价格白名单</h3>
        <p>仅管理当前会员或代理商套餐下的 TEST 价格方案；记录只可软停用，不提供删除或终态恢复。</p>
      </div>
      <div class="whitelist-toolbar__actions">
        <el-tag v-if="canView && !canManage" type="info" effect="plain">只读权限</el-tag>
        <el-button :loading="pricePlansLoading || listLoading" :disabled="!canView" @click="refresh">刷新</el-button>
        <el-button type="primary" :disabled="!canCreate" @click="openCreate">新增白名单</el-button>
      </div>
    </header>

    <el-alert
      type="warning"
      show-icon
      :closable="false"
      title="测试价格不会替换普通购买价格"
      description="加入白名单不会改变普通购买价格，用户仍需通过专用测试入口。"
    />
    <el-alert
      v-if="!canView"
      type="error"
      show-icon
      :closable="false"
      title="无白名单查看权限"
      description="读取需要精确权限 pricing:plan:view；admin.full 不会自动获得 pricing 权限。"
    />
    <el-alert
      v-else-if="!canManage"
      type="info"
      show-icon
      :closable="false"
      title="当前账号仅可查看测试白名单"
      description="新增、修改和停用需要精确权限 pricing:test-whitelist:manage。"
    />
    <el-alert
      v-if="fixedTargetInvalid"
      type="error"
      show-icon
      :closable="false"
      title="当前价格方案不是可管理的 TEST 方案"
      description="已阻止读取和写入；请刷新价格方案后，从 TEST 方案重新进入。"
    />
    <el-alert
      v-if="pricePlanLoadError"
      type="error"
      show-icon
      :closable="false"
      title="TEST 价格方案加载失败"
      :description="`${pricePlanLoadError}；旧缓存不会用于开启白名单读写，请刷新后重试。`"
    />

    <div v-if="canView" class="whitelist-selection">
      <el-form-item v-if="!fixedPricePlanId" label="TEST 价格方案">
        <el-select
          v-model="selectedPricePlanId"
          placeholder="请选择 TEST 价格方案"
          :loading="pricePlansLoading"
          filterable
          @change="selectPricePlan"
        >
          <el-option
            v-for="item in testPricePlans"
            :key="item.pricePlanId"
            :value="item.pricePlanId"
            :label="`${item.name} · ${item.environment} · ${formatPriceCents(item.salePriceCents)}`"
          />
        </el-select>
      </el-form-item>
      <article v-else-if="selectedPricePlan" class="whitelist-fixed-plan">
        <span>当前 TEST 方案</span>
        <strong>{{ selectedPricePlan.name }}</strong>
        <code>{{ selectedPricePlan.pricePlanId }}</code>
        <small>{{ selectedPricePlan.environment }} · {{ formatPriceCents(selectedPricePlan.salePriceCents) }} · revision {{ selectedPricePlan.revision }}</small>
      </article>
    </div>

    <template v-if="canView && selectedPricePlan">
      <div v-if="whitelistWriteLocked" class="whitelist-refresh-gate">
        <el-alert
          type="error"
          show-icon
          :closable="false"
          title="白名单写入已锁定"
          :description="`${whitelistRefreshWarning?.message || '上次写入后的最新列表尚未确认'}；关闭或重开弹窗不会解除，创建、编辑和停用均已阻断。`"
        />
        <el-button :loading="recoveryLoading" type="warning" @click="recoverPersistentWhitelistGate">精确重新加载并解除门禁</el-button>
      </div>
      <div class="whitelist-filters">
        <el-select v-model="filters.status" aria-label="按白名单状态筛选">
          <el-option label="全部状态" value="ALL" />
          <el-option v-for="status in whitelistStatuses" :key="status" :label="statusLabel(status)" :value="status" />
        </el-select>
        <el-input v-model="filters.userId" clearable placeholder="按用户 ID 精确筛选" aria-label="按用户 ID 筛选" @keyup.enter="search" />
        <el-button type="primary" plain :loading="listLoading" @click="search">查询</el-button>
        <el-button :disabled="listLoading" @click="resetFilters">重置</el-button>
      </div>

      <el-alert
        v-if="listError && entries.length"
        type="warning"
        show-icon
        :closable="false"
        title="白名单刷新失败，当前显示旧缓存"
        :description="`${listError}；请勿把当前列表视为最新状态。`"
      />
      <el-skeleton v-if="listLoading && !loadedWhitelistId" :rows="5" animated />
      <el-result v-else-if="listError && !entries.length" icon="error" title="白名单加载失败" :sub-title="listError">
        <template #extra><el-button @click="loadEntries">重试</el-button></template>
      </el-result>
      <el-empty v-else-if="!listLoading && !entries.length" description="当前筛选条件下没有白名单记录" />
      <el-table v-else :data="entries" row-key="whitelistEntryId" stripe>
        <el-table-column label="用户" min-width="180">
          <template #default="{ row }"><div class="whitelist-user"><strong>{{ row.userId }}</strong><small>{{ row.whitelistEntryId }}</small></div></template>
        </el-table-column>
        <el-table-column label="状态" width="150">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status)">{{ statusLabel(row.status) }}</el-tag>
            <small v-if="entryActions(row).requiresNewEntry" class="terminal-copy">不可恢复，请新建记录</small>
          </template>
        </el-table-column>
        <el-table-column label="有效期" min-width="220">
          <template #default="{ row }"><span>{{ formatDateTime(row.validFrom, "立即生效") }}</span><small class="muted">至 {{ formatDateTime(row.validUntil, "长期有效") }}</small></template>
        </el-table-column>
        <el-table-column label="资格原因" min-width="180" prop="reason" />
        <el-table-column label="操作记录" min-width="230">
          <template #default="{ row }">
            <span>创建 {{ actorTime(row.createdBy, row.createdAt) }}</span>
            <small class="muted">更新 {{ actorTime(row.updatedBy, row.updatedAt) }}</small>
            <small v-if="row.disabledBy || row.disabledAt" class="muted">停用 {{ actorTime(row.disabledBy, row.disabledAt) }}</small>
          </template>
        </el-table-column>
        <el-table-column label="revision" width="95" prop="revision" />
        <el-table-column label="操作" fixed="right" width="190">
          <template #default="{ row }">
            <div class="whitelist-actions">
              <el-button v-if="canViewAudit" link type="primary" @click="viewWhitelistAudit(row)">查看审计</el-button>
              <el-button v-if="entryActions(row).canEdit" link type="primary" @click="openEdit(row)">编辑</el-button>
              <el-button v-if="entryActions(row).canDisable" link type="danger" @click="openDisable(row)">停用</el-button>
              <span v-if="entryActions(row).requiresNewEntry" class="muted">终态只读</span>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        v-if="page.total > 0"
        class="whitelist-pagination"
        background
        layout="total, sizes, prev, pager, next, jumper"
        :total="page.total"
        :current-page="page.page"
        :page-size="page.pageSize"
        :page-sizes="[20, 50, 100]"
        @current-change="changePage"
        @size-change="changePageSize"
      />
    </template>
    <el-empty v-else-if="canView && !pricePlansLoading && !pricePlanLoadError && !fixedTargetInvalid" description="当前套餐没有 TEST 价格方案；普通或活动价格方案不提供白名单入口" />

    <el-dialog
      v-model="formVisible"
      append-to-body
      :title="formMode === 'CREATE' ? '新增测试白名单' : '编辑测试白名单'"
      width="min(660px, 92vw)"
      :close-on-click-modal="false"
      :close-on-press-escape="!formSaving"
      :show-close="!formSaving"
    >
      <el-alert v-if="formError" type="error" show-icon :closable="false" title="白名单保存失败" :description="formError" />
      <el-alert
        v-if="formRevisionConflict"
        type="warning"
        show-icon
        :closable="false"
        title="revision 已冲突，表单内容已保留"
        :description="formFieldConflicts.length ? '本地与服务器修改了同一字段，必须逐项明确处理；当前仍禁止提交。' : '不会自动覆盖服务器数据。请先刷新并执行三方合并。'"
      />
      <el-alert
        v-if="formWriteLocked"
        type="warning"
        show-icon
        :closable="false"
        title="服务端已确认写入，但列表刷新失败"
        description="当前表单已锁定，不能重复提交。请重新加载服务器状态。"
      />
      <section v-if="formFieldConflicts.length" class="whitelist-field-conflicts">
        <strong>字段级并发冲突</strong>
        <p>未改字段已同步服务器值；以下字段双方都已修改，请明确选择后才能继续。</p>
        <article v-for="conflict in formFieldConflicts" :key="conflict.field">
          <div>
            <el-tag type="warning">{{ conflictFieldLabel(conflict.field) }}</el-tag>
            <small>服务器值：{{ conflictValue(conflict.remote) }}</small>
            <small>当前输入：{{ conflictValue(form[conflict.field]) }}</small>
          </div>
          <div>
            <el-button size="small" @click="resolveFieldConflict(conflict.field, 'SERVER')">使用服务器值</el-button>
            <el-button size="small" type="primary" plain @click="resolveFieldConflict(conflict.field, 'LOCAL')">保留我的当前输入</el-button>
          </div>
        </article>
      </section>
      <el-form label-position="top" class="whitelist-form" :model="form" @submit.prevent>
        <el-form-item label="用户 ID" required>
          <el-input v-model="form.userId" :disabled="formMode === 'EDIT' || formWriteLocked" maxlength="128" placeholder="仅填写系统已有 userId" />
        </el-form-item>
        <el-form-item label="资格原因" required>
          <el-input v-model="form.reason" :disabled="formWriteLocked" type="textarea" :rows="2" maxlength="500" show-word-limit />
        </el-form-item>
        <div class="whitelist-form__dates">
          <el-form-item label="生效时间">
            <el-date-picker v-model="form.validFrom" :disabled="formWriteLocked" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" format="YYYY-MM-DD HH:mm:ss" clearable />
          </el-form-item>
          <el-form-item label="失效时间">
            <el-date-picker v-model="form.validUntil" :disabled="formWriteLocked" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" format="YYYY-MM-DD HH:mm:ss" clearable />
          </el-form-item>
        </div>
        <el-form-item label="变更原因" required>
          <el-input v-model="form.changeReason" :disabled="formWriteLocked" type="textarea" :rows="2" maxlength="500" show-word-limit placeholder="必填；会进入领域审计" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button :disabled="formSaving" @click="formVisible = false">取消</el-button>
        <el-button v-if="formRevisionConflict && !formFieldConflicts.length" :loading="revisionRefreshing" @click="refreshFormRevision">刷新并三方合并</el-button>
        <el-button v-if="formWriteLocked" :loading="recoveryLoading" @click="recoverFormWrite">精确重新加载并解除门禁</el-button>
        <el-button type="primary" :loading="formSaving" :disabled="!formSubmitAllowed" @click="submitForm">{{ formMode === "CREATE" ? "新增" : "保存" }}</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="disableVisible"
      append-to-body
      title="停用测试白名单"
      width="min(560px, 92vw)"
      :close-on-click-modal="false"
      :close-on-press-escape="!disableSaving"
      :show-close="!disableSaving"
    >
      <el-alert type="warning" show-icon :closable="false" title="停用后不可恢复" description="若用户以后需要重新参与测试，必须新建一条有效期记录。" />
      <el-alert v-if="disableError" type="error" show-icon :closable="false" title="停用失败" :description="disableError" />
      <el-alert v-if="disableRevisionConflict" type="warning" show-icon :closable="false" title="revision 已冲突，停用原因已保留" description="请先刷新记录并核对状态；不会自动覆盖服务器数据。" />
      <el-alert v-if="disableWriteLocked" type="warning" show-icon :closable="false" title="白名单写入门禁仍未解除" description="已锁定重复提交；关闭或重开弹窗不会解除。" />
      <p v-if="disableTarget" class="disable-target">用户 <strong>{{ disableTarget.userId }}</strong> · {{ disableTarget.status }} · revision {{ disableBaseRevision }}</p>
      <el-form label-position="top" @submit.prevent>
        <el-form-item label="停用原因" required>
          <el-input v-model="disableReason" :disabled="disableWriteLocked" type="textarea" :rows="3" maxlength="500" show-word-limit placeholder="必填；会进入领域审计" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button :disabled="disableSaving" @click="disableVisible = false">取消</el-button>
        <el-button v-if="disableRevisionConflict" :loading="revisionRefreshing" @click="refreshDisableRevision">刷新 revision，保留原因</el-button>
        <el-button v-if="disableWriteLocked" :loading="recoveryLoading" @click="recoverDisableWrite">精确重新加载并解除门禁</el-button>
        <el-button type="danger" :loading="disableSaving" :disabled="!disableSubmitAllowed" @click="submitDisable">确认停用</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { ElMessage } from "element-plus";
import {
  buildWhitelistUpdateFromBaseline,
  formatPriceCents,
  hasPricingPermission,
  pricingErrorMessage,
  rebaseWhitelistEditableFields,
  resolveWhitelistFieldConflict as resolveWhitelistFieldConflictState,
  selectableWhitelistPricePlans,
  whitelistDisableResultMessage,
  whitelistEntryUIActions,
  whitelistMutationErrorState,
  whitelistRefreshGateKey,
  whitelistValidityIssue
} from "../../../domain/pricePlanAdmin.ts";
import type { WhitelistEditableField, WhitelistFieldConflict } from "../../../domain/pricePlanAdmin.ts";
import { usePricePlanAdminStore } from "../../../stores/pricePlanAdmin.ts";
import type {
  BusinessPlan,
  PricePlanWhitelistEntry,
  PricingAuditFilters,
  WhitelistFilters,
  WhitelistStatus,
  WhitelistUpdateInput
} from "../../../types/pricePlanAdmin.ts";

interface Props {
  plan: BusinessPlan;
  currentRole: string;
  currentPermissions: string[];
  fixedPricePlanId?: string;
}

type FormMode = "CREATE" | "EDIT";

interface WhitelistFormState {
  userId: string;
  reason: string;
  validFrom: string;
  validUntil: string;
  changeReason: string;
}

const props = withDefaults(defineProps<Props>(), { fixedPricePlanId: "" });
const emit = defineEmits<{ (event: "view-audit", filters: PricingAuditFilters): void }>();
const store = usePricePlanAdminStore();
const selectedPricePlanId = ref("");
const pricePlansFreshPlanId = ref("");
const loadedWhitelistId = ref("");
const planLoadSequence = ref(0);
const whitelistLoadSequence = ref(0);
const filters = reactive<{ status: "ALL" | WhitelistStatus; userId: string; page: number; pageSize: number }>({
  status: "ALL",
  userId: "",
  page: 1,
  pageSize: 20
});
const whitelistStatuses: WhitelistStatus[] = ["PENDING", "ACTIVE", "EXPIRED", "DISABLED"];

const formVisible = ref(false);
const formMode = ref<FormMode>("CREATE");
const formTargetId = ref("");
const formBaseRevision = ref(0);
const formOriginal = ref<WhitelistFormState>(emptyForm());
const formError = ref("");
const formRevisionConflict = ref(false);
const formFieldConflicts = ref<WhitelistFieldConflict[]>([]);
const formCommittedStale = ref(false);
const formMutationKey = ref("");
const form = reactive<WhitelistFormState>(emptyForm());

const disableVisible = ref(false);
const disableTargetId = ref("");
const disableTarget = ref<PricePlanWhitelistEntry | null>(null);
const disableBaseRevision = ref(0);
const disableReason = ref("");
const disableError = ref("");
const disableRevisionConflict = ref(false);
const disableCommittedStale = ref(false);
const disableMutationKey = ref("");
const revisionRefreshing = ref(false);
const recoveryLoading = ref(false);

const principal = computed(() => ({ role: props.currentRole, permissions: props.currentPermissions }));
const canView = computed(() => hasPricingPermission(principal.value, "pricing:plan:view"));
const canManage = computed(() => canView.value && hasPricingPermission(principal.value, "pricing:test-whitelist:manage"));
const canViewAudit = computed(() => hasPricingPermission(principal.value, "pricing:audit:view"));
const pricePlansLoading = computed(() => Boolean(store.loading[`pricePlans:${props.plan.id}`]));
const pricePlanLoadError = computed(() => store.errors[`pricePlans:${props.plan.id}`]?.message || "");
const testPricePlans = computed(() => selectableWhitelistPricePlans(store.pricePlansByPlanId[props.plan.id] || [], props.plan.id));
const selectedPricePlan = computed(() => testPricePlans.value.find((item) => item.pricePlanId === selectedPricePlanId.value));
const fixedTargetInvalid = computed(() => canView.value && Boolean(props.fixedPricePlanId)
  && pricePlansFreshPlanId.value === props.plan.id && !selectedPricePlan.value);
const listKey = computed(() => `whitelist:${selectedPricePlanId.value}`);
const listLoading = computed(() => Boolean(selectedPricePlanId.value && store.loading[listKey.value]));
const listError = computed(() => selectedPricePlanId.value ? store.errors[listKey.value]?.message || "" : "");
const entries = computed(() => selectedPricePlanId.value ? store.whitelistByPricePlanId[selectedPricePlanId.value] || [] : []);
const page = computed(() => store.whitelistPageByPricePlanId[selectedPricePlanId.value] || {
  items: entries.value,
  total: entries.value.length,
  page: filters.page,
  pageSize: filters.pageSize
});

function viewWhitelistAudit(entry: PricePlanWhitelistEntry) {
  emit("view-audit", { planId: props.plan.id, pricePlanId: selectedPricePlanId.value, whitelistEntryId: entry.whitelistEntryId });
}
const whitelistRefreshWarning = computed(() => selectedPricePlanId.value
  ? store.refreshWarnings[whitelistRefreshGateKey(selectedPricePlanId.value)]
  : undefined);
const whitelistWriteLocked = computed(() => Boolean(selectedPricePlanId.value
  && store.whitelistRefreshGatesByPricePlanId[selectedPricePlanId.value]));
const canCreate = computed(() => canManage.value && pricePlansFreshPlanId.value === props.plan.id
  && Boolean(selectedPricePlan.value) && !fixedTargetInvalid.value && !whitelistWriteLocked.value);
const formSaving = computed(() => Boolean(formMutationKey.value && store.saving[formMutationKey.value]));
const disableSaving = computed(() => Boolean(disableMutationKey.value && store.saving[disableMutationKey.value]));
const formWriteLocked = computed(() => formCommittedStale.value || whitelistWriteLocked.value);
const disableWriteLocked = computed(() => disableCommittedStale.value || whitelistWriteLocked.value);
const formSubmitAllowed = computed(() => canCreate.value && !formSaving.value && !formRevisionConflict.value
  && formFieldConflicts.value.length === 0 && !formWriteLocked.value);
const disableSubmitAllowed = computed(() => canManage.value && Boolean(disableTarget.value) && !disableSaving.value
  && !disableRevisionConflict.value && !disableWriteLocked.value);

watch([() => props.plan.id, () => props.fixedPricePlanId, canView], initialize, { immediate: true });

function emptyForm(): WhitelistFormState {
  return { userId: "", reason: "", validFrom: "", validUntil: "", changeReason: "" };
}

async function initialize() {
  const sequence = ++planLoadSequence.value;
  pricePlansFreshPlanId.value = "";
  loadedWhitelistId.value = "";
  selectedPricePlanId.value = "";
  disableTarget.value = null;
  if (!canView.value) return;
  try {
    await store.loadPricePlans(props.plan.id);
  } catch {
    return;
  }
  if (sequence !== planLoadSequence.value) return;
  pricePlansFreshPlanId.value = props.plan.id;
  const requested = String(props.fixedPricePlanId || "").trim();
  selectedPricePlanId.value = requested || testPricePlans.value[0]?.pricePlanId || "";
  if (selectedPricePlan.value) await loadEntries();
}

async function refresh() {
  await initialize();
}

function requestFilters(): WhitelistFilters {
  return {
    ...(filters.status !== "ALL" ? { status: filters.status } : {}),
    ...(filters.userId.trim() ? { userId: filters.userId.trim() } : {}),
    page: filters.page,
    pageSize: filters.pageSize
  };
}

async function loadEntries(): Promise<boolean> {
  const pricePlanId = selectedPricePlanId.value;
  if (!canView.value || pricePlansFreshPlanId.value !== props.plan.id
    || !testPricePlans.value.some((item) => item.pricePlanId === pricePlanId)) return false;
  const sequence = ++whitelistLoadSequence.value;
  try {
    await store.loadWhitelist(pricePlanId, requestFilters());
    if (sequence !== whitelistLoadSequence.value || selectedPricePlanId.value !== pricePlanId) return false;
    loadedWhitelistId.value = pricePlanId;
    return true;
  } catch {
    return false;
  }
}

async function selectPricePlan(value: string) {
  if (!testPricePlans.value.some((item) => item.pricePlanId === value)) {
    selectedPricePlanId.value = "";
    return;
  }
  filters.page = 1;
  loadedWhitelistId.value = "";
  await loadEntries();
}

async function search() {
  filters.page = 1;
  await loadEntries();
}

async function resetFilters() {
  Object.assign(filters, { status: "ALL", userId: "", page: 1, pageSize: 20 });
  await loadEntries();
}

async function changePage(value: number) {
  filters.page = value;
  await loadEntries();
}

async function changePageSize(value: number) {
  filters.pageSize = value;
  filters.page = 1;
  await loadEntries();
}

function entryActions(entry: PricePlanWhitelistEntry) {
  const actions = whitelistEntryUIActions(entry, principal.value);
  return whitelistWriteLocked.value
    ? { ...actions, canManage: false, canEdit: false, canDisable: false }
    : actions;
}

function resetFormState() {
  Object.assign(form, emptyForm());
  formOriginal.value = emptyForm();
  formError.value = "";
  formRevisionConflict.value = false;
  formFieldConflicts.value = [];
  formCommittedStale.value = whitelistWriteLocked.value;
  formMutationKey.value = "";
}

function openCreate() {
  if (!canCreate.value) return;
  resetFormState();
  formMode.value = "CREATE";
  formTargetId.value = "";
  formBaseRevision.value = 0;
  formMutationKey.value = `createWhitelist:${selectedPricePlanId.value}`;
  formVisible.value = true;
}

function openEdit(entry: PricePlanWhitelistEntry) {
  if (!entryActions(entry).canEdit) return;
  resetFormState();
  formMode.value = "EDIT";
  formTargetId.value = entry.whitelistEntryId;
  formBaseRevision.value = entry.revision;
  const initial = {
    userId: entry.userId,
    reason: entry.reason,
    validFrom: entry.validFrom || "",
    validUntil: entry.validUntil || "",
    changeReason: ""
  };
  Object.assign(form, initial);
  formOriginal.value = { ...initial };
  formMutationKey.value = `updateWhitelist:${entry.whitelistEntryId}`;
  formVisible.value = true;
}

function validateForm() {
  if (!form.userId.trim()) return "必须填写用户 ID。";
  if (!form.reason.trim()) return "必须填写资格原因。";
  if (!form.changeReason.trim()) return "必须填写变更原因。";
  const validityIssue = whitelistValidityIssue(form);
  if (validityIssue) return validityIssue === "WHITELIST_RFC3339_OFFSET_REQUIRED"
    ? "有效期必须使用带时区偏移的 RFC3339 时间。"
    : pricingErrorMessage({ code: validityIssue });
  return "";
}

function updatePayload(): WhitelistUpdateInput {
  return buildWhitelistUpdateFromBaseline({
    revision: formBaseRevision.value,
    changeReason: form.changeReason,
    baseline: formOriginal.value,
    current: form
  });
}

async function submitForm() {
  if (!formSubmitAllowed.value || !selectedPricePlan.value) return;
  formError.value = validateForm();
  if (formError.value) return;
  try {
    let response;
    if (formMode.value === "CREATE") {
      response = await store.createWhitelistEntry(selectedPricePlan.value.pricePlanId, {
        revision: 0,
        userId: form.userId.trim(),
        reason: form.reason.trim(),
        ...(form.validFrom ? { validFrom: form.validFrom } : {}),
        ...(form.validUntil ? { validUntil: form.validUntil } : {}),
        changeReason: form.changeReason.trim()
      });
    } else {
      const payload = updatePayload();
      if (Object.keys(payload).every((key) => key === "revision" || key === "changeReason")) {
        formError.value = "没有检测到白名单字段变更。";
        return;
      }
      response = await store.updateWhitelistEntry(selectedPricePlan.value.pricePlanId, formTargetId.value, payload);
    }
    formBaseRevision.value = response.item.revision;
    if (store.refreshWarnings[formMutationKey.value]) {
      formCommittedStale.value = true;
      formError.value = "写入已成功，但列表刷新失败；禁止重复提交。";
      return;
    }
    ElMessage.success(formMode.value === "CREATE" ? "白名单已新增" : "白名单已更新");
    formVisible.value = false;
  } catch (error) {
    const state = whitelistMutationErrorState(error);
    formError.value = state.message;
    formRevisionConflict.value = state.revisionConflict;
    if (state.revisionConflict) formFieldConflicts.value = [];
  }
}

async function refreshFormRevision() {
  const pricePlanId = selectedPricePlanId.value;
  const entryId = formTargetId.value;
  const userId = formOriginal.value.userId || form.userId.trim();
  revisionRefreshing.value = true;
  try {
    if (!pricePlanId || !entryId || !userId) {
      formError.value = "缺少目标白名单身份，revision 未更新，当前表单仍保持阻断。";
      return;
    }
    const latest = await store.loadWhitelistEntryExact(pricePlanId, entryId, userId);
    if (selectedPricePlanId.value !== pricePlanId || formTargetId.value !== entryId) return;
    if (!whitelistEntryUIActions(latest, principal.value).canEdit) {
      formError.value = "记录已不存在或进入终态，请关闭表单并新建记录。";
      return;
    }
    const rebased = rebaseWhitelistEditableFields({
      original: formOriginal.value,
      local: form,
      latest: {
        reason: latest.reason,
        validFrom: latest.validFrom || "",
        validUntil: latest.validUntil || ""
      }
    });
    formBaseRevision.value = latest.revision;
    Object.assign(form, rebased.form);
    formOriginal.value = {
      userId: latest.userId,
      ...rebased.baseline,
      changeReason: ""
    };
    formFieldConflicts.value = rebased.conflicts;
    formRevisionConflict.value = rebased.conflicts.length > 0;
    formError.value = rebased.conflicts.length
      ? `已刷新到 revision ${latest.revision}，发现 ${rebased.conflicts.length} 个字段级冲突；必须逐项明确处理。`
      : `已刷新到 revision ${latest.revision}；未修改字段已同步服务器值，仅保留本地 dirty 字段。`;
  } catch (error) {
    formError.value = `未取得目标记录的最新服务器状态：${pricingErrorMessage(error)}；revision 未更新，当前表单仍保持阻断。`;
  } finally {
    revisionRefreshing.value = false;
  }
}

async function recoverFormWrite() {
  recoveryLoading.value = true;
  try {
    if (!await recoverWhitelistGateExact()) return;
    store.clearRefreshWarning(formMutationKey.value);
    formCommittedStale.value = false;
    formVisible.value = false;
    ElMessage.success("服务器白名单状态已重新加载");
  } catch (error) {
    formError.value = pricingErrorMessage(error);
  } finally {
    recoveryLoading.value = false;
  }
}

function openDisable(entry: PricePlanWhitelistEntry) {
  if (!entryActions(entry).canDisable) return;
  disableTargetId.value = entry.whitelistEntryId;
  disableTarget.value = { ...entry };
  disableBaseRevision.value = entry.revision;
  disableReason.value = "";
  disableError.value = "";
  disableRevisionConflict.value = false;
  disableCommittedStale.value = whitelistWriteLocked.value;
  disableMutationKey.value = `disableWhitelist:${entry.whitelistEntryId}`;
  disableVisible.value = true;
}

async function submitDisable() {
  if (!disableSubmitAllowed.value || !selectedPricePlan.value || !disableTarget.value) return;
  const reason = disableReason.value.trim();
  if (!reason) {
    disableError.value = "必须填写停用原因。";
    return;
  }
  try {
    const response = await store.disableWhitelistEntry(selectedPricePlan.value.pricePlanId, disableTarget.value.whitelistEntryId, {
      revision: disableBaseRevision.value,
      changeReason: reason
    });
    if (store.refreshWarnings[disableMutationKey.value]) {
      disableCommittedStale.value = true;
      disableError.value = `${whitelistDisableResultMessage(response)}，但列表刷新失败；禁止重复提交。`;
      return;
    }
    ElMessage.success(whitelistDisableResultMessage(response));
    disableVisible.value = false;
  } catch (error) {
    const state = whitelistMutationErrorState(error);
    disableError.value = state.message;
    disableRevisionConflict.value = state.revisionConflict;
  }
}

async function refreshDisableRevision() {
  const pricePlanId = selectedPricePlanId.value;
  const target = disableTarget.value;
  revisionRefreshing.value = true;
  try {
    if (!pricePlanId || !target) {
      disableError.value = "缺少目标白名单身份，revision 未更新，停用操作仍保持阻断。";
      return;
    }
    const latest = await store.loadWhitelistEntryExact(pricePlanId, target.whitelistEntryId, target.userId);
    if (selectedPricePlanId.value !== pricePlanId || disableTargetId.value !== target.whitelistEntryId) return;
    if (!whitelistEntryUIActions(latest, principal.value).canDisable) {
      disableError.value = "记录已不存在或进入终态，不能再次停用；后续资格需新建记录。";
      return;
    }
    disableTarget.value = { ...latest };
    disableBaseRevision.value = latest.revision;
    disableRevisionConflict.value = false;
    disableError.value = `已刷新到 revision ${latest.revision}；停用原因仍保留，请核对后再提交。`;
  } catch (error) {
    disableError.value = `未取得目标记录的最新服务器状态：${pricingErrorMessage(error)}；revision 未更新，停用操作仍保持阻断。`;
  } finally {
    revisionRefreshing.value = false;
  }
}

async function recoverDisableWrite() {
  recoveryLoading.value = true;
  try {
    if (!await recoverWhitelistGateExact()) return;
    store.clearRefreshWarning(disableMutationKey.value);
    disableCommittedStale.value = false;
    disableVisible.value = false;
    ElMessage.success("服务器白名单状态已重新加载");
  } catch (error) {
    disableError.value = pricingErrorMessage(error);
  } finally {
    recoveryLoading.value = false;
  }
}

function resolveFieldConflict(field: WhitelistEditableField, resolution: "SERVER" | "LOCAL") {
  const resolved = resolveWhitelistFieldConflictState({
    form,
    baseline: formOriginal.value,
    conflicts: formFieldConflicts.value
  }, field, resolution);
  Object.assign(form, resolved.form);
  formFieldConflicts.value = resolved.conflicts;
  if (formFieldConflicts.value.length === 0) {
    formRevisionConflict.value = false;
    formError.value = "字段冲突已明确处理；请复核表单和变更原因后再提交。";
  }
}

async function recoverWhitelistGateExact(): Promise<boolean> {
  const pricePlanId = selectedPricePlanId.value;
  if (!pricePlanId || !whitelistWriteLocked.value) return false;
  try {
    await store.recoverWhitelistRefreshGate(pricePlanId, requestFilters());
    if (selectedPricePlanId.value !== pricePlanId) return false;
    loadedWhitelistId.value = pricePlanId;
    formCommittedStale.value = false;
    disableCommittedStale.value = false;
    return true;
  } catch (error) {
    const message = pricingErrorMessage(error);
    formError.value = `服务器状态仍未成功加载：${message}；写入门禁继续保留。`;
    disableError.value = `服务器状态仍未成功加载：${message}；写入门禁继续保留。`;
    return false;
  }
}

async function recoverPersistentWhitelistGate() {
  recoveryLoading.value = true;
  try {
    if (await recoverWhitelistGateExact()) ElMessage.success("已精确加载当前白名单，写入门禁已解除");
    else if (whitelistWriteLocked.value) ElMessage.error("最新白名单仍未确认，写入门禁继续保留");
  } finally {
    recoveryLoading.value = false;
  }
}

function conflictFieldLabel(field: WhitelistEditableField) {
  return ({ reason: "资格原因", validFrom: "生效时间", validUntil: "失效时间" } as Record<WhitelistEditableField, string>)[field];
}

function conflictValue(value: string) {
  return value || "（空）";
}

function statusLabel(value: string) {
  return ({ PENDING: "未生效", ACTIVE: "有效", EXPIRED: "已过期", DISABLED: "已停用" } as Record<string, string>)[value] || value;
}

function statusTag(value: string) {
  return ({ PENDING: "warning", ACTIVE: "success", EXPIRED: "info", DISABLED: "danger" } as Record<string, "warning" | "success" | "info" | "danger">)[value] || "info";
}

function formatDateTime(value?: string, fallback = "未记录") {
  if (!value) return fallback;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN", { hour12: false });
}

function actorTime(actor?: string, time?: string) {
  return `${actor || "未记录"} · ${formatDateTime(time)}`;
}
</script>

<style scoped>
.price-plan-whitelist-manager { display: grid; min-width: 0; gap: 14px; padding-top: 8px; }
.whitelist-toolbar { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.whitelist-toolbar h3,.whitelist-toolbar p { margin: 0; }.whitelist-toolbar p { margin-top: 5px; color: var(--admin-muted); line-height: 1.5; }
.whitelist-toolbar__actions,.whitelist-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; }
.whitelist-selection { max-width: 620px; }.whitelist-selection :deep(.el-form-item) { margin-bottom: 0; }.whitelist-selection :deep(.el-select) { width: 100%; }
.whitelist-fixed-plan,.whitelist-user { display: grid; gap: 4px; }.whitelist-fixed-plan { padding: 12px; border: 1px solid var(--admin-border); border-radius: 10px; background: var(--admin-panel-soft); }
.whitelist-fixed-plan span,.whitelist-fixed-plan small,.whitelist-user small,.muted { color: var(--admin-muted); }.whitelist-fixed-plan code { overflow-wrap: anywhere; }
.whitelist-refresh-gate { display: flex; align-items: center; gap: 10px; }.whitelist-refresh-gate :deep(.el-alert) { flex: 1; }
.whitelist-filters { display: grid; grid-template-columns: 160px minmax(220px, 1fr) auto auto; gap: 8px; }
.terminal-copy { display: block; margin-top: 5px; color: var(--el-color-danger); line-height: 1.35; }
.whitelist-pagination { justify-content: flex-end; overflow-x: auto; padding-bottom: 4px; }
.whitelist-form { margin-top: 14px; }.whitelist-form__dates { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }.whitelist-form__dates :deep(.el-date-editor) { width: 100%; }
.whitelist-field-conflicts { display: grid; gap: 8px; margin-top: 12px; padding: 12px; border: 1px solid var(--el-color-warning-light-5); border-radius: 8px; background: var(--el-color-warning-light-9); }
.whitelist-field-conflicts > p { margin: 0; color: var(--admin-muted); }.whitelist-field-conflicts article { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding-top: 8px; border-top: 1px solid var(--admin-border); }
.whitelist-field-conflicts article > div { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; }.whitelist-field-conflicts small { color: var(--admin-muted); overflow-wrap: anywhere; }
.disable-target { padding: 10px 0; overflow-wrap: anywhere; }
@media (max-width: 720px) {
  .whitelist-toolbar,.whitelist-refresh-gate,.whitelist-field-conflicts article { align-items: stretch; flex-direction: column; }.whitelist-filters,.whitelist-form__dates { grid-template-columns: 1fr; }
}
</style>
