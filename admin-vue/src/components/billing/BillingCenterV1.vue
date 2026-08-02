<template>
  <section class="billing-v1">
    <header class="billing-v1__header">
      <div>
        <div class="billing-v1__eyebrow">知启云 AI · BILLING CENTER V1</div>
        <h2>{{ pageTitle }}</h2>
        <p>{{ pageDescription }}</p>
      </div>
      <el-button :icon="Refresh" :loading="loading" @click="load">刷新数据</el-button>
    </header>

    <el-alert
      v-if="errorMessage"
      class="billing-v1__alert"
      type="error"
      :title="errorMessage"
      show-icon
      :closable="false"
    />

    <template v-if="moduleId === 'billingOverview'">
      <div class="billing-v1__metrics" v-loading="loading">
        <article v-for="metric in overviewMetrics" :key="metric.label">
          <span>{{ metric.label }}</span>
          <strong>{{ metric.value }}</strong>
          <small>{{ metric.hint }}</small>
        </article>
      </div>
      <div class="billing-v1__split">
        <el-card shadow="never">
          <template #header><div class="panel-title"><strong>最近任务对账</strong><span>{{ overview.recentTasks.length }} 条</span></div></template>
          <el-table :data="overview.recentTasks" height="360" stripe empty-text="暂无任务">
            <el-table-column prop="taskId" label="taskId" min-width="190" show-overflow-tooltip />
            <el-table-column prop="modelCode" label="模型编码" min-width="170" show-overflow-tooltip />
            <el-table-column prop="taskStatus" label="任务状态" width="118"><template #default="s"><status-tag :value="s.row.taskStatus" /></template></el-table-column>
            <el-table-column prop="billingStatus" label="计费状态" width="118"><template #default="s"><status-tag :value="s.row.billingStatus" /></template></el-table-column>
            <el-table-column prop="capturedPoints" label="确认点数" width="104" align="right" />
            <el-table-column label="异常" min-width="190"><template #default="s"><span :class="s.row.anomalies.length ? 'text-danger' : 'text-success'">{{ s.row.anomalies.length ? s.row.anomalies.map(anomalyLabel).join('、') : '正常' }}</span></template></el-table-column>
          </el-table>
        </el-card>
        <el-card shadow="never">
          <template #header><div class="panel-title"><strong>最近钱包流水</strong><span>{{ overview.recentLedger.length }} 条</span></div></template>
          <el-table :data="overview.recentLedger" height="360" stripe empty-text="暂无流水">
            <el-table-column prop="entryType" label="流水类型" width="112"><template #default="s"><status-tag :value="s.row.entryType" /></template></el-table-column>
            <el-table-column prop="userId" label="userId" min-width="130" show-overflow-tooltip />
            <el-table-column prop="taskId" label="taskId" min-width="160" show-overflow-tooltip />
            <el-table-column prop="points" label="点数" width="86" align="right" />
            <el-table-column label="可用余额" min-width="120"><template #default="s">{{ number(s.row.availableBefore) }} → {{ number(s.row.availableAfter) }}</template></el-table-column>
          </el-table>
        </el-card>
      </div>
    </template>

    <template v-else-if="moduleId === 'billingRules'">
      <div class="billing-v1__filters billing-v1__filters--rules">
        <el-input v-model="keyword" clearable placeholder="搜索模型名称、编码、模块或来源" :prefix-icon="Search" />
        <el-switch v-model="showRuleHistory" active-text="显示历史版本" inactive-text="仅看当前价格" />
        <el-tag effect="plain">{{ showRuleHistory ? rules.length : currentBillingRules.length }} 条</el-tag>
      </div>
      <el-card shadow="never" v-loading="loading">
        <el-table :data="filteredRules" height="650" stripe empty-text="暂无计费规则">
          <el-table-column prop="modelName" label="模型名称" min-width="210" fixed>
            <template #default="s">
              <div class="billing-rule-name">
                <span>{{ s.row.modelName }}</span>
                <el-tag v-if="s.row.status === 'PUBLISHED'" size="small" type="success">当前生效</el-tag>
                <el-tag v-else-if="s.row.status === 'DRAFT'" size="small" type="warning">待发布草稿</el-tag>
                <el-tag v-else size="small" type="info">历史归档</el-tag>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="modelCode" label="模型编码" min-width="180" show-overflow-tooltip />
          <el-table-column prop="moduleCode" label="所属模块" min-width="130" />
          <el-table-column prop="billingUnit" label="计费单位" min-width="125" />
          <el-table-column prop="basePrice" label="基础售价" min-width="105" align="right"><template #default="s">{{ number(s.row.basePrice) }}</template></el-table-column>
          <el-table-column prop="minimumCharge" label="最低扣费" min-width="105" align="right"><template #default="s">{{ number(s.row.minimumCharge) }}</template></el-table-column>
          <el-table-column label="参数规则" min-width="230" show-overflow-tooltip><template #default="s">{{ jsonText(s.row.parameterRules) }}</template></el-table-column>
          <el-table-column prop="ruleSource" label="规则来源" min-width="145"><template #default="s"><el-tag :type="s.row.ruleSource === 'CODE_DEFAULT' ? 'warning' : s.row.ruleSource === 'DATABASE' ? 'success' : 'primary'" :effect="s.row.ruleSource === 'CODE_DEFAULT' ? 'dark' : 'light'">{{ s.row.ruleSource }}</el-tag></template></el-table-column>
          <el-table-column prop="version" label="版本" width="78"><template #default="s">v{{ s.row.version }}</template></el-table-column>
          <el-table-column prop="status" label="版本状态" width="112"><template #default="s">{{ ruleStatusLabel(s.row.status) }}</template></el-table-column>
          <el-table-column prop="updatedAt" label="更新时间" min-width="178"><template #default="s">{{ dateTime(s.row.updatedAt) }}</template></el-table-column>
          <el-table-column label="操作" width="210" fixed="right">
            <template #default="s">
              <el-button link type="primary" @click="openRuleEditor(s.row)">新建草稿</el-button>
              <el-button link type="success" :disabled="s.row.status !== 'DRAFT'" @click="validateRule(s.row)">校验</el-button>
              <el-button link type="warning" :disabled="s.row.status !== 'DRAFT'" @click="publishRule(s.row)">发布</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </template>

    <template v-else-if="moduleId === 'billingProviderCosts'">
      <div class="billing-v1__filters"><el-input v-model="keyword" clearable placeholder="搜索供应商、通道或模型" :prefix-icon="Search" /></div>
      <el-card shadow="never" v-loading="loading">
        <el-table :data="filteredCosts" height="650" stripe empty-text="暂无供应商成本">
          <el-table-column prop="provider" label="供应商" min-width="115" fixed />
          <el-table-column prop="channel" label="通道" min-width="150" show-overflow-tooltip />
          <el-table-column prop="platformModelCode" label="平台模型编码" min-width="185" show-overflow-tooltip />
          <el-table-column prop="upstreamModelName" label="上游模型名" min-width="185" show-overflow-tooltip />
          <el-table-column prop="billingUnit" label="计费单位" min-width="125" />
          <el-table-column label="参数范围" min-width="230" show-overflow-tooltip><template #default="s">{{ jsonText(s.row.parameterRange) }}</template></el-table-column>
          <el-table-column prop="unitCost" label="单位成本" min-width="110" align="right"><template #default="s">{{ money(s.row.unitCost, s.row.currency) }}</template></el-table-column>
          <el-table-column prop="currency" label="币种" width="82" />
          <el-table-column prop="effectiveFrom" label="生效时间" min-width="178"><template #default="s">{{ dateTime(s.row.effectiveFrom) }}</template></el-table-column>
          <el-table-column prop="status" label="状态" width="104"><template #default="s"><status-tag :value="s.row.status" /></template></el-table-column>
          <el-table-column label="操作" width="90" fixed="right"><template #default="s"><el-button link type="primary" @click="openCostEditor(s.row)">编辑</el-button></template></el-table-column>
        </el-table>
      </el-card>
    </template>

    <template v-else-if="moduleId === 'billingEvents'">
      <div class="billing-v1__filters"><el-input v-model="keyword" clearable placeholder="搜索任务、用户、模型、事件或幂等键" :prefix-icon="Search" /></div>
      <el-card shadow="never" v-loading="loading">
        <el-table :data="filteredEvents" height="650" stripe empty-text="暂无计费事件">
          <el-table-column prop="id" label="事件ID" min-width="190" show-overflow-tooltip />
          <el-table-column prop="taskId" label="taskId" min-width="190" show-overflow-tooltip />
          <el-table-column prop="userId" label="userId" min-width="130" show-overflow-tooltip />
          <el-table-column prop="tenantId" label="tenantId" min-width="130" show-overflow-tooltip />
          <el-table-column prop="modelCode" label="modelCode" min-width="175" show-overflow-tooltip />
          <el-table-column prop="eventType" label="事件类型" width="110"><template #default="s"><status-tag :value="s.row.eventType" /></template></el-table-column>
          <el-table-column prop="billingStatus" label="计费状态" width="120"><template #default="s"><status-tag :value="s.row.billingStatus" /></template></el-table-column>
          <el-table-column prop="points" label="点数" width="92" align="right" />
          <el-table-column prop="ruleVersionId" label="价格版本快照" min-width="185" show-overflow-tooltip />
          <el-table-column prop="providerChannel" label="供应商通道" min-width="150" show-overflow-tooltip />
          <el-table-column prop="idempotencyKey" label="幂等键" min-width="210" show-overflow-tooltip />
          <el-table-column prop="createdAt" label="发生时间" min-width="178"><template #default="s">{{ dateTime(s.row.createdAt) }}</template></el-table-column>
        </el-table>
      </el-card>
    </template>

    <template v-else-if="moduleId === 'billingReconciliation'">
      <div class="billing-v1__filters billing-v1__filters--wide">
        <el-input v-model="keyword" clearable placeholder="搜索 taskId、userId、tenantId、modelCode 或通道" :prefix-icon="Search" />
        <el-checkbox v-model="onlyAbnormal">只看异常</el-checkbox>
        <el-tag type="danger" effect="plain">异常 {{ abnormalCount }}</el-tag>
      </div>
      <el-card shadow="never" v-loading="loading">
        <el-table :data="filteredReconciliation" height="650" stripe empty-text="暂无生成任务">
          <el-table-column prop="taskId" label="taskId" min-width="190" fixed show-overflow-tooltip />
          <el-table-column prop="userId" label="userId" min-width="125" show-overflow-tooltip />
          <el-table-column prop="tenantId" label="tenantId" min-width="125" show-overflow-tooltip />
          <el-table-column prop="modelCode" label="modelCode" min-width="170" show-overflow-tooltip />
          <el-table-column prop="taskStatus" label="taskStatus" width="118"><template #default="s"><status-tag :value="s.row.taskStatus" /></template></el-table-column>
          <el-table-column prop="billingStatus" label="billingStatus" width="122"><template #default="s"><status-tag :value="s.row.billingStatus" /></template></el-table-column>
          <el-table-column prop="quotedPoints" label="quotedPoints" width="112" align="right" />
          <el-table-column prop="reservedPoints" label="reservedPoints" width="122" align="right" />
          <el-table-column prop="capturedPoints" label="capturedPoints" width="122" align="right" />
          <el-table-column prop="releasedPoints" label="releasedPoints" width="122" align="right" />
          <el-table-column prop="refundedPoints" label="refundedPoints" width="122" align="right" />
          <el-table-column prop="supplierCost" label="supplierCost" width="112" align="right"><template #default="s">{{ nullableNumber(s.row.supplierCost) }}</template></el-table-column>
          <el-table-column prop="estimatedMargin" label="estimatedMargin" width="132" align="right"><template #default="s"><span :class="Number(s.row.estimatedMargin) < 0 ? 'text-danger' : ''">{{ nullableNumber(s.row.estimatedMargin) }}</span></template></el-table-column>
          <el-table-column prop="providerChannel" label="providerChannel" min-width="155" show-overflow-tooltip />
          <el-table-column label="异常识别" min-width="260"><template #default="s"><div class="anomaly-list"><el-tag v-for="item in s.row.anomalies" :key="item" size="small" type="danger" effect="plain">{{ anomalyLabel(item) }}</el-tag><el-tag v-if="!s.row.anomalies.length" size="small" type="success" effect="plain">正常</el-tag></div></template></el-table-column>
          <el-table-column prop="createdAt" label="createdAt" min-width="178"><template #default="s">{{ dateTime(s.row.createdAt) }}</template></el-table-column>
        </el-table>
      </el-card>
    </template>

    <template v-else-if="moduleId === 'billingWalletLedger'">
      <div class="billing-v1__filters billing-v1__filters--wide">
        <el-input v-model="keyword" clearable placeholder="搜索用户、租户、任务、流水类型或幂等键" :prefix-icon="Search" />
        <el-select v-model="ledgerType" clearable placeholder="全部流水类型" style="width: 180px"><el-option v-for="item in ledgerTypes" :key="item" :label="item" :value="item" /></el-select>
      </div>
      <el-card shadow="never" v-loading="loading">
        <el-table :data="filteredLedger" height="650" stripe empty-text="暂无钱包流水">
          <el-table-column prop="id" label="流水ID" min-width="190" fixed show-overflow-tooltip />
          <el-table-column prop="entryType" label="类型" width="112"><template #default="s"><status-tag :value="s.row.entryType" /></template></el-table-column>
          <el-table-column prop="userId" label="userId" min-width="125" show-overflow-tooltip />
          <el-table-column prop="tenantId" label="tenantId" min-width="125" show-overflow-tooltip />
          <el-table-column prop="taskId" label="taskId" min-width="180" show-overflow-tooltip />
          <el-table-column prop="points" label="点数" width="92" align="right" />
          <el-table-column label="可用余额" min-width="148"><template #default="s">{{ number(s.row.availableBefore) }} → {{ number(s.row.availableAfter) }}</template></el-table-column>
          <el-table-column label="冻结余额" min-width="148"><template #default="s">{{ number(s.row.frozenBefore) }} → {{ number(s.row.frozenAfter) }}</template></el-table-column>
          <el-table-column prop="referenceType" label="关联类型" min-width="145" />
          <el-table-column prop="referenceId" label="关联ID" min-width="180" show-overflow-tooltip />
          <el-table-column prop="idempotencyKey" label="幂等键" min-width="210" show-overflow-tooltip />
          <el-table-column prop="remark" label="说明" min-width="170" show-overflow-tooltip />
          <el-table-column prop="createdAt" label="发生时间" min-width="178"><template #default="s">{{ dateTime(s.row.createdAt) }}</template></el-table-column>
        </el-table>
      </el-card>
      <el-alert class="billing-v1__footnote" type="info" :closable="false" show-icon title="RELEASE 用于确认扣费前解冻；REFUND 用于确认扣费后的返还。余额变化必须对应钱包流水。" />
    </template>

    <el-dialog v-model="ruleDialogVisible" title="创建价格草稿版本" width="620px" destroy-on-close>
      <el-form label-width="110px">
        <el-form-item label="模型"><el-input :model-value="ruleDraft.modelName" disabled /></el-form-item>
        <el-form-item label="基础售价"><el-input-number v-model="ruleDraft.basePrice" :min="0" :precision="4" :step="1" /></el-form-item>
        <el-form-item label="最低扣费"><el-input-number v-model="ruleDraft.minimumCharge" :min="0" :precision="4" :step="1" /></el-form-item>
        <el-form-item label="计费单位"><el-input :model-value="ruleDraft.billingUnit" disabled /></el-form-item>
        <el-form-item label="参数规则"><el-input v-model="ruleDraft.parameterRulesText" type="textarea" :rows="7" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="ruleDialogVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="saveRuleDraft">保存为新草稿</el-button></template>
    </el-dialog>

    <el-dialog v-model="costDialogVisible" title="编辑供应商成本" width="620px" destroy-on-close>
      <el-form label-width="120px">
        <el-form-item label="供应商"><el-input v-model="costDraft.provider" /></el-form-item>
        <el-form-item label="通道"><el-input v-model="costDraft.channel" /></el-form-item>
        <el-form-item label="平台模型编码"><el-input v-model="costDraft.platformModelCode" /></el-form-item>
        <el-form-item label="上游模型名"><el-input v-model="costDraft.upstreamModelName" /></el-form-item>
        <el-form-item label="计费单位"><el-input v-model="costDraft.billingUnit" /></el-form-item>
        <el-form-item label="单位成本"><el-input-number v-model="costDraft.unitCost" :min="0" :precision="6" :step="0.1" /></el-form-item>
        <el-form-item label="币种"><el-input v-model="costDraft.currency" /></el-form-item>
        <el-form-item label="参数范围"><el-input v-model="costDraft.parameterRangeText" type="textarea" :rows="5" /></el-form-item>
        <el-form-item label="生效时间"><el-input v-model="costDraft.effectiveFrom" /></el-form-item>
        <el-form-item label="状态"><el-select v-model="costDraft.status"><el-option label="ACTIVE" value="ACTIVE" /><el-option label="INACTIVE" value="INACTIVE" /></el-select></el-form-item>
      </el-form>
      <template #footer><el-button @click="costDialogVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="saveCost">保存成本</el-button></template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, reactive, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { Refresh, Search } from "@element-plus/icons-vue";
import {
  billingApi,
  type BillingLifecycleEvent,
  type BillingOverview,
  type BillingReconciliationItem,
  type BillingRuleVersion,
  type ProviderCost,
  type WalletLedgerEntry
} from "../../api/billing";

const props = defineProps<{ moduleId: string }>();
const loading = ref(false);
const saving = ref(false);
const errorMessage = ref("");
const keyword = ref("");
const onlyAbnormal = ref(false);
const ledgerType = ref("");
const showRuleHistory = ref(false);
const rules = ref<BillingRuleVersion[]>([]);
const costs = ref<ProviderCost[]>([]);
const events = ref<BillingLifecycleEvent[]>([]);
const reconciliation = ref<BillingReconciliationItem[]>([]);
const ledger = ref<WalletLedgerEntry[]>([]);
const emptyOverview: BillingOverview = { summary: { publishedRules: 0, draftRules: 0, providerCosts: 0, tasks: 0, abnormalTasks: 0, walletEntries: 0, estimatedMargin: 0, marginTasks: 0 }, recentTasks: [], recentLedger: [] };
const overview = reactive<BillingOverview>(structuredClone(emptyOverview));

const titles: Record<string, [string, string]> = {
  billingOverview: ["计费总览", "价格、成本、任务异常与钱包流水的统一经营视图。"],
  billingRules: ["模型计费规则", "查看版本化用户价格，修改时创建草稿，校验通过后发布。"],
  billingProviderCosts: ["供应商成本", "独立维护上游成本口径，不与用户售价混存。"],
  billingEvents: ["计费事件", "按任务追踪报价、冻结、确认、解冻与退款事件。"],
  billingReconciliation: ["任务对账", "分离任务状态与计费状态，自动识别八类计费异常。"],
  billingWalletLedger: ["用户积分钱包流水", "追踪每次用户积分余额与冻结额变化，禁止无流水改余额。"]
};
const pageTitle = computed(() => titles[props.moduleId]?.[0] || "计费中心");
const pageDescription = computed(() => titles[props.moduleId]?.[1] || "");
const overviewMetrics = computed(() => [
  { label: "正式价格版本", value: overview.summary.publishedRules, hint: `待校验草稿 ${overview.summary.draftRules}` },
  { label: "供应商成本", value: overview.summary.providerCosts, hint: "独立成本规则" },
  { label: "生成任务", value: overview.summary.tasks, hint: `异常 ${overview.summary.abnormalTasks}` },
  { label: "钱包流水", value: overview.summary.walletEntries, hint: "全量余额轨迹" },
  { label: "预估毛利", value: number(overview.summary.estimatedMargin), hint: `已核算 ${overview.summary.marginTasks} 个任务` },
  { label: "对账健康度", value: overview.summary.tasks ? `${number(((overview.summary.tasks - overview.summary.abnormalTasks) / overview.summary.tasks) * 100)}%` : "-", hint: "任务、事件、流水三方一致" }
]);

function searchable(row: unknown) { return JSON.stringify(row).toLowerCase(); }
const query = computed(() => keyword.value.trim().toLowerCase());
const currentBillingRules = computed(() => {
  const groups = new Map<string, BillingRuleVersion[]>();
  for (const rule of rules.value) {
    const key = rule.ruleKey || `${rule.moduleCode}:${rule.modelCode}`;
    const versions = groups.get(key) || [];
    versions.push(rule);
    groups.set(key, versions);
  }
  return [...groups.values()].map((versions) => [...versions].sort((left, right) => {
    const leftCurrent = left.status === "PUBLISHED" ? 1 : 0;
    const rightCurrent = right.status === "PUBLISHED" ? 1 : 0;
    if (leftCurrent !== rightCurrent) return rightCurrent - leftCurrent;
    return right.version - left.version;
  })[0]);
});
const filteredRules = computed(() => (showRuleHistory.value ? rules.value : currentBillingRules.value).filter((row) => !query.value || searchable(row).includes(query.value)));
const filteredCosts = computed(() => costs.value.filter((row) => !query.value || searchable(row).includes(query.value)));
const filteredEvents = computed(() => events.value.filter((row) => !query.value || searchable(row).includes(query.value)));
const filteredReconciliation = computed(() => reconciliation.value.filter((row) => (!onlyAbnormal.value || row.anomalies.length > 0) && (!query.value || searchable(row).includes(query.value))));
const filteredLedger = computed(() => ledger.value.filter((row) => (!ledgerType.value || row.entryType === ledgerType.value) && (!query.value || searchable(row).includes(query.value))));
const abnormalCount = computed(() => reconciliation.value.filter((row) => row.anomalies.length > 0).length);
const ledgerTypes = ["RECHARGE", "GRANT", "RESERVE", "CAPTURE", "RELEASE", "REFUND", "ADJUSTMENT", "EXPIRE"];

const statusType = (value: string) => {
  const normalized = String(value || "").toUpperCase();
  if (["ACTIVE", "PUBLISHED", "SUCCEEDED", "CAPTURED", "CAPTURE", "GRANT", "RECHARGE"].includes(normalized)) return "success";
  if (["FAILED", "BILLING_FAILED", "INACTIVE", "EXPIRE"].includes(normalized)) return "danger";
  if (["DRAFT", "QUOTED", "RESERVED", "RESERVE", "QUEUED", "RUNNING"].includes(normalized)) return "warning";
  return "info";
};
const StatusTag = defineComponent({ props: { value: { type: String, default: "-" } }, setup(p) { return () => h("span", { class: `billing-status billing-status--${statusType(p.value)}` }, p.value || "-"); } });

async function load() {
  loading.value = true;
  errorMessage.value = "";
  try {
    if (props.moduleId === "billingOverview") Object.assign(overview, emptyOverview, await billingApi.overview());
    else if (props.moduleId === "billingRules") rules.value = (await billingApi.rules()).items || [];
    else if (props.moduleId === "billingProviderCosts") costs.value = (await billingApi.providerCosts()).items || [];
    else if (props.moduleId === "billingEvents") events.value = (await billingApi.events()).items || [];
    else if (props.moduleId === "billingReconciliation") reconciliation.value = (await billingApi.reconciliation()).items || [];
    else if (props.moduleId === "billingWalletLedger") ledger.value = (await billingApi.walletLedger()).items || [];
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "计费数据加载失败";
  } finally {
    loading.value = false;
  }
}

watch(() => props.moduleId, () => { keyword.value = ""; void load(); }, { immediate: true });

const ruleDialogVisible = ref(false);
const selectedRule = ref<BillingRuleVersion | null>(null);
const ruleDraft = reactive({ modelName: "", billingUnit: "", basePrice: 0, minimumCharge: 0, parameterRulesText: "{}" });
function openRuleEditor(row: BillingRuleVersion) {
  selectedRule.value = row;
  Object.assign(ruleDraft, { modelName: row.modelName, billingUnit: row.billingUnit, basePrice: row.basePrice, minimumCharge: row.minimumCharge, parameterRulesText: JSON.stringify(row.parameterRules || {}, null, 2) });
  ruleDialogVisible.value = true;
}
async function saveRuleDraft() {
  if (!selectedRule.value) return;
  try {
    const parameterRules = JSON.parse(ruleDraft.parameterRulesText || "{}");
    saving.value = true;
    await billingApi.createRuleDraft(selectedRule.value.id, { billing_type: selectedRule.value.billingUnit, base_price: ruleDraft.basePrice, minimum_charge: ruleDraft.minimumCharge, parameter_multiplier: parameterRules, status: "DRAFT" });
    showRuleHistory.value = true;
    ruleDialogVisible.value = false;
    ElMessage.success("已创建新草稿，当前正式版本未被覆盖");
    await load();
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : "草稿创建失败"); } finally { saving.value = false; }
}
async function validateRule(row: BillingRuleVersion) {
  try {
    const { validation } = await billingApi.validateRule(row.id);
    if (validation.valid) ElMessage.success("规则校验通过，可以发布");
    else await ElMessageBox.alert(validation.issues.map((item) => `${item.severity} · ${item.message}`).join("\n"), "规则校验未通过", { type: "warning" });
    await load();
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : "规则校验失败"); }
}
async function publishRule(row: BillingRuleVersion) {
  try {
    await ElMessageBox.confirm(`发布 ${row.modelName} v${row.version}？历史任务仍保留原价格快照。`, "发布价格版本", { type: "warning" });
    await billingApi.publishRule(row.id);
    ElMessage.success("价格版本已发布");
    await load();
  } catch (error) { if (error !== "cancel") ElMessage.error(error instanceof Error ? error.message : "发布失败"); }
}

const costDialogVisible = ref(false);
const selectedCost = ref<ProviderCost | null>(null);
const costDraft = reactive({ provider: "", channel: "", platformModelCode: "", upstreamModelName: "", billingUnit: "", parameterRangeText: "{}", unitCost: 0, currency: "CNY", effectiveFrom: "", effectiveTo: "", status: "ACTIVE" });
function openCostEditor(row: ProviderCost) {
  selectedCost.value = row;
  Object.assign(costDraft, row, { parameterRangeText: JSON.stringify(row.parameterRange || {}, null, 2), effectiveTo: row.effectiveTo || "" });
  costDialogVisible.value = true;
}
async function saveCost() {
  if (!selectedCost.value) return;
  try {
    saving.value = true;
    await billingApi.updateProviderCost(selectedCost.value.id, { provider: costDraft.provider, channel: costDraft.channel, platformModelCode: costDraft.platformModelCode, upstreamModelName: costDraft.upstreamModelName, billingUnit: costDraft.billingUnit, parameterRange: JSON.parse(costDraft.parameterRangeText || "{}"), unitCost: costDraft.unitCost, currency: costDraft.currency, effectiveFrom: costDraft.effectiveFrom, effectiveTo: costDraft.effectiveTo, status: costDraft.status });
    costDialogVisible.value = false;
    ElMessage.success("供应商成本已更新，用户价格未变更");
    await load();
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : "成本保存失败"); } finally { saving.value = false; }
}

const anomalyNames: Record<string, string> = {
  TASK_SUCCEEDED_NOT_CAPTURED: "任务成功未确认扣费", TASK_FAILED_NOT_RELEASED: "任务失败未解冻",
  CAPTURED_NOT_EQUAL_QUOTED: "实际扣费与报价不一致", MISSING_PROVIDER_COST: "缺少供应商成本",
  MISSING_BILLING_EVENT: "缺少计费事件", MISSING_WALLET_LEDGER: "缺少钱包流水",
  DUPLICATE_CAPTURE: "重复扣费", NEGATIVE_MARGIN: "负毛利"
};
function anomalyLabel(value: string) { return anomalyNames[value] || value; }
function number(value: unknown) { const n = Number(value); return Number.isFinite(n) ? new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 4 }).format(n) : "-"; }
function nullableNumber(value: unknown) { return value === null || value === undefined ? "-" : number(value); }
function money(value: unknown, currency = "CNY") { const n = Number(value); return Number.isFinite(n) ? `${number(n)} ${currency}` : "-"; }
function dateTime(value?: string) { if (!value) return "-"; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN", { hour12: false }); }
function jsonText(value: unknown) { return JSON.stringify(value || {}); }
function ruleStatusLabel(value: string) { return value === "PUBLISHED" ? "当前生效" : value === "DRAFT" ? "待发布草稿" : "历史归档"; }
</script>

<style scoped>
.billing-v1 { min-width: 0; padding: 4px 2px 28px; color: #1d2939; }
.billing-v1__header { display: flex; align-items: flex-start; justify-content: space-between; gap: 24px; margin-bottom: 18px; padding: 22px 24px; border: 1px solid #e6eaf0; border-radius: 18px; background: linear-gradient(135deg, #fff 0%, #f6f8ff 60%, #edf2ff 100%); }
.billing-v1__eyebrow { margin-bottom: 8px; color: #5b65d8; font-size: 11px; font-weight: 800; letter-spacing: .12em; }
.billing-v1__header h2 { margin: 0; font-size: 24px; }
.billing-v1__header p { margin: 8px 0 0; color: #667085; }
.billing-v1__alert { margin-bottom: 16px; }
.billing-v1__metrics { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 12px; margin-bottom: 16px; }
.billing-v1__metrics article { min-height: 116px; padding: 18px; border: 1px solid #e7eaf2; border-radius: 16px; background: #fff; box-shadow: 0 8px 30px rgba(16, 24, 40, .035); }
.billing-v1__metrics span, .billing-v1__metrics small { display: block; color: #667085; }
.billing-v1__metrics strong { display: block; margin: 10px 0 6px; font-size: 26px; color: #111827; }
.billing-v1__metrics small { font-size: 12px; }
.billing-v1__split { display: grid; grid-template-columns: 1.15fr 1fr; gap: 16px; }
.panel-title { display: flex; justify-content: space-between; align-items: center; }
.panel-title span { color: #98a2b3; font-size: 12px; }
.billing-v1__filters { display: flex; justify-content: flex-end; gap: 14px; align-items: center; margin-bottom: 12px; }
.billing-v1__filters .el-input { width: min(460px, 100%); }
.billing-v1__filters--wide { justify-content: flex-start; }
.billing-v1__filters--rules { flex-wrap: wrap; }
.billing-rule-name { display: flex; align-items: center; gap: 8px; min-width: 0; }
.billing-rule-name > span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.billing-status { display: inline-flex; align-items: center; min-height: 24px; padding: 2px 9px; border-radius: 999px; font-size: 12px; font-weight: 700; white-space: nowrap; }
.billing-status--success { color: #067647; background: #ecfdf3; }
.billing-status--warning { color: #b54708; background: #fffaeb; }
.billing-status--danger { color: #b42318; background: #fef3f2; }
.billing-status--info { color: #344054; background: #f2f4f7; }
.text-danger { color: #d92d20; }
.text-success { color: #079455; }
.anomaly-list { display: flex; flex-wrap: wrap; gap: 4px; }
.billing-v1__footnote { margin-top: 14px; }
@media (max-width: 1300px) { .billing-v1__metrics { grid-template-columns: repeat(3, 1fr); } .billing-v1__split { grid-template-columns: 1fr; } }
@media (max-width: 760px) { .billing-v1__header { flex-direction: column; } .billing-v1__metrics { grid-template-columns: 1fr 1fr; } }
</style>
