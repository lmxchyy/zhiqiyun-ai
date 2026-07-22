<template>
  <section class="identity-panel">
    <el-skeleton v-if="loading" :rows="8" animated />
    <el-result v-else-if="forbidden" icon="warning" title="无权限查看身份与权益" sub-title="请联系超级管理员授予用户身份管理权限。" />
    <el-alert v-else-if="errorMessage" type="error" :title="errorMessage" :closable="false" show-icon>
      <template #default><el-button size="small" @click="loadAll">重新加载</el-button></template>
    </el-alert>
    <template v-else>
      <header class="identity-toolbar">
        <div><span>商业身份、会员与资金概况</span><small>身份、权限角色和会员权益相互独立</small></div>
        <div class="toolbar-actions">
          <el-button :disabled="!profile" @click="openHistory">查看身份变更记录</el-button>
          <el-dropdown v-if="canChangeIdentity || canReviewIdentity || canDowngradeIdentity" trigger="click" @command="openAction">
            <el-button type="primary">身份操作<el-icon class="el-icon--right"><ArrowDown /></el-icon></el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-if="canChangeIdentity" command="UPGRADE" :disabled="currentIdentity === 'OPERATION_CENTER'">调整身份</el-dropdown-item>
                <el-dropdown-item v-if="canChangeIdentity" command="ADJUST_PARENT_AGENT">调整上级代理商</el-dropdown-item>
                <el-dropdown-item v-if="canChangeIdentity" command="ADJUST_OPERATION_CENTER">调整运营中心</el-dropdown-item>
                <el-dropdown-item v-if="canChangeIdentity" command="FREEZE" :disabled="currentIdentity === 'USER' || currentStatus !== 'ACTIVE'" divided>冻结身份</el-dropdown-item>
                <el-dropdown-item v-if="canChangeIdentity" command="RESTORE" :disabled="currentStatus !== 'FROZEN'">恢复身份</el-dropdown-item>
                <el-dropdown-item v-if="canDowngradeIdentity" command="DOWNGRADE" :disabled="currentIdentity === 'USER'" divided>受控降级</el-dropdown-item>
                <el-dropdown-item v-if="canReviewIdentity" command="REVIEW" divided>审核套餐转换</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </header>

      <div class="identity-summary-grid">
        <el-card shadow="never" class="identity-main-card">
          <template #header><div class="card-heading"><strong>当前商业身份</strong><el-tag :type="identityTagType">{{ identityLabel(currentIdentity) }}</el-tag></div></template>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="商业身份">{{ identityLabel(currentIdentity) }}</el-descriptions-item>
            <el-descriptions-item label="身份状态"><el-tag :type="statusTagType(currentStatus)">{{ statusLabel(currentStatus) }}</el-tag></el-descriptions-item>
            <el-descriptions-item label="分润资格"><el-tag :type="currentRecord?.commissionEnabled ? 'success' : 'info'">{{ currentRecord?.commissionEnabled ? '具备' : '不具备' }}</el-tag></el-descriptions-item>
            <el-descriptions-item label="系统权限角色"><el-space wrap><el-tag v-for="role in profile?.accountRoles || []" :key="role" effect="plain">{{ role }}</el-tag><span v-if="!profile?.accountRoles?.length">{{ profile?.legacyRole || '-' }}</span></el-space></el-descriptions-item>
            <el-descriptions-item label="生效时间">{{ dateTime(currentRecord?.effectiveAt) }}</el-descriptions-item>
            <el-descriptions-item label="到期时间">{{ dateTime(currentRecord?.expiresAt) }}</el-descriptions-item>
            <el-descriptions-item label="上级代理商">{{ relationship?.parentAgentName || relationship?.parentAgentId || '未设置' }}</el-descriptions-item>
            <el-descriptions-item label="所属运营中心">{{ relationship?.operationCenterName || relationship?.operationCenterId || '未设置' }}</el-descriptions-item>
          </el-descriptions>
        </el-card>

        <el-card shadow="never">
          <template #header><strong>会员权益</strong></template>
          <el-descriptions :column="1" border>
            <el-descriptions-item label="会员等级"><el-tag effect="plain">{{ financial?.membership?.level || 'FREE' }}</el-tag></el-descriptions-item>
            <el-descriptions-item label="会员套餐">{{ financial?.membership?.planId || '未开通' }}</el-descriptions-item>
            <el-descriptions-item label="有效期至">{{ dateTime(financial?.membership?.expiresAt) }}</el-descriptions-item>
            <el-descriptions-item label="权益记录">{{ number(financial?.membership?.entitlementRecordCount) }} 条</el-descriptions-item>
          </el-descriptions>
        </el-card>
      </div>

      <div class="financial-metrics">
        <article><span>当前 Token</span><strong>{{ number(financial?.wallet?.tokenBalance ?? financial?.wallet?.pointsAvailable) }}</strong><small>冻结 {{ number(financial?.wallet?.frozenToken ?? financial?.wallet?.pointsFrozen) }}</small></article>
        <article><span>钱包余额</span><strong>{{ money(financial?.wallet?.cashBalanceCents) }}</strong><small>仅展示，不提供直接编辑</small></article>
        <article><span>待结算分润</span><strong>{{ money(pendingCommission) }}</strong><small>预计、冻结、可结算及结算中</small></article>
        <article><span>已结算分润</span><strong>{{ money(financial?.commission?.settledCents) }}</strong><small>历史记录不可覆盖</small></article>
      </div>

      <el-alert type="info" :closable="false" show-icon title="关系调整只影响生效后的新订单；冻结或终止会关闭新分润资格，不会回溯修改历史分润。" />
    </template>

    <IdentityChangeWizard v-if="profile" v-model="wizardOpen" :user-id="userId" :user-name="userName" :current-identity="currentIdentity" :relationship="relationship" :agents="agents" :centers="centers" :plans="plans" :initial-action="wizardAction" @success="loadAll" />
    <IdentityDowngradeWizard v-if="profile && currentIdentity !== 'USER'" v-model="downgradeOpen" :user-id="userId" :user-name="userName" :current-identity="currentIdentity" :agents="agents" :centers="centers" @success="loadAll" />

    <el-drawer v-model="historyOpen" title="身份变更记录" size="min(920px, 94vw)">
      <el-tabs>
        <el-tab-pane :label="`身份记录 ${history.identities.length}`">
          <el-table :data="history.identities" stripe empty-text="暂无身份记录">
            <el-table-column label="身份"><template #default="scope">{{ identityLabel(scope.row.identityType) }}</template></el-table-column>
            <el-table-column label="状态"><template #default="scope"><el-tag :type="statusTagType(scope.row.identityStatus)">{{ statusLabel(scope.row.identityStatus) }}</el-tag></template></el-table-column>
            <el-table-column label="分润资格"><template #default="scope">{{ scope.row.commissionEnabled ? '是' : '否' }}</template></el-table-column>
            <el-table-column prop="sourceType" label="来源" min-width="150" />
            <el-table-column label="生效时间" min-width="180"><template #default="scope">{{ dateTime(scope.row.effectiveAt) }}</template></el-table-column>
            <el-table-column label="结束时间" min-width="180"><template #default="scope">{{ dateTime(scope.row.endedAt) }}</template></el-table-column>
          </el-table>
        </el-tab-pane>
        <el-tab-pane :label="`变更流水 ${history.changeRecords.length}`">
          <el-table :data="history.changeRecords" stripe empty-text="暂无变更流水">
            <el-table-column prop="changeType" label="操作" min-width="140" />
            <el-table-column label="原身份"><template #default="scope">{{ identityLabel(String(scope.row.oldIdentity?.identityType || 'USER')) }}</template></el-table-column>
            <el-table-column label="新身份"><template #default="scope">{{ identityLabel(String(scope.row.newIdentity?.identityType || 'USER')) }}</template></el-table-column>
            <el-table-column prop="reason" label="原因" min-width="220" show-overflow-tooltip />
            <el-table-column prop="operatorId" label="操作管理员" min-width="150" />
            <el-table-column label="时间" min-width="180"><template #default="scope">{{ dateTime(scope.row.createdAt) }}</template></el-table-column>
          </el-table>
        </el-tab-pane>
        <el-tab-pane :label="`关系历史 ${relationshipHistory.length}`">
          <el-table :data="relationshipHistory" stripe empty-text="暂无关系历史">
            <el-table-column prop="parentAgentName" label="上级代理商" min-width="160" />
            <el-table-column prop="operationCenterName" label="运营中心" min-width="160" />
            <el-table-column prop="status" label="状态" />
            <el-table-column label="生效时间" min-width="180"><template #default="scope">{{ dateTime(scope.row.effectiveAt) }}</template></el-table-column>
            <el-table-column label="结束时间" min-width="180"><template #default="scope">{{ dateTime(scope.row.endedAt) }}</template></el-table-column>
          </el-table>
        </el-tab-pane>
        <el-tab-pane :label="`降级请求 ${downgradeRequests.length}`">
          <el-table :data="downgradeRequests" stripe empty-text="暂无降级请求或无权查看">
            <el-table-column prop="requestId" label="请求编号" min-width="210" show-overflow-tooltip />
            <el-table-column label="状态"><template #default="scope"><el-tag :type="downgradeStatusType(scope.row.status)">{{ scope.row.status }}</el-tag></template></el-table-column>
            <el-table-column label="迁移关系"><template #default="scope">{{ scope.row.migratedRelationships }}</template></el-table-column>
            <el-table-column label="生效时间" min-width="180"><template #default="scope">{{ dateTime(scope.row.effectiveAt) }}</template></el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </el-drawer>

    <el-dialog v-model="reviewOpen" title="审核套餐转换" width="560px" :close-on-click-modal="false">
      <el-form label-position="top">
        <el-form-item label="Preview 审核凭证" required><el-input v-model.trim="reviewForm.previewToken" type="textarea" :rows="2" placeholder="粘贴另一名管理员提供的审核凭证" /></el-form-item>
        <el-form-item label="审核结论"><el-radio-group v-model="reviewForm.decision"><el-radio value="APPROVED">通过</el-radio><el-radio value="REJECTED">拒绝</el-radio></el-radio-group></el-form-item>
        <el-form-item label="审核原因" required><el-input v-model.trim="reviewForm.reason" type="textarea" :rows="3" maxlength="300" show-word-limit /></el-form-item>
      </el-form>
      <template #footer><el-button @click="reviewOpen = false">取消</el-button><el-button type="primary" :loading="reviewing" :disabled="!reviewForm.previewToken || reviewForm.reason.length < 4" @click="submitReview">提交审核</el-button></template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import { ArrowDown } from "@element-plus/icons-vue";
import { computed, reactive, ref, watch } from "vue";
import { ElMessage } from "element-plus";
import { identityManagementApi, type BusinessIdentity, type BusinessIdentityType, type IdentityChangeAction, type IdentityChangeRecord, type IdentityDowngradeResult, type IdentityFinancialOverview, type IdentityOption, type IdentityPlanOption, type IdentityProfile, type UserRelationship } from "../../api/identityManagement";
import IdentityChangeWizard from "./IdentityChangeWizard.vue";
import IdentityDowngradeWizard from "./IdentityDowngradeWizard.vue";

const props = defineProps<{ userId: string; userName?: string; permissions?: string[] }>();
const loading = ref(false); const errorMessage = ref(""); const forbidden = ref(false);
const profile = ref<IdentityProfile | null>(null); const relationship = ref<UserRelationship | null>(null); const financial = ref<IdentityFinancialOverview | null>(null);
const agents = ref<IdentityOption[]>([]); const centers = ref<IdentityOption[]>([]); const plans = ref<IdentityPlanOption[]>([]); const relationshipHistory = ref<UserRelationship[]>([]);
const history = reactive<{ identities: BusinessIdentity[]; changeRecords: IdentityChangeRecord[] }>({ identities: [], changeRecords: [] });
const wizardOpen = ref(false); const wizardAction = ref<IdentityChangeAction>("UPGRADE"); const historyOpen = ref(false); const reviewOpen = ref(false); const reviewing = ref(false);
const downgradeOpen = ref(false);
const downgradeRequests = ref<IdentityDowngradeResult[]>([]);
const reviewForm = reactive({ previewToken: "", decision: "APPROVED" as "APPROVED" | "REJECTED", reason: "" });

const currentIdentity = computed<BusinessIdentityType>(() => profile.value?.primaryIdentity || "USER");
const currentRecord = computed(() => profile.value?.identities.find((item) => item.identityType === currentIdentity.value) || profile.value?.identities[0]);
const currentStatus = computed(() => currentRecord.value?.identityStatus || "ACTIVE");
const identityTagType = computed(() => currentIdentity.value === "OPERATION_CENTER" ? "danger" : currentIdentity.value === "AGENT" ? "warning" : "info");
const pendingCommission = computed(() => ["expectedCents", "frozenCents", "availableCents", "settlingCents"].reduce((sum, key) => sum + Number(financial.value?.commission?.[key] || 0), 0));
const hasPermission = (permission: string) => Boolean(props.permissions?.includes("admin.full") || props.permissions?.includes(permission));
const canChangeIdentity = computed(() => hasPermission("identity:change:preview") && hasPermission("identity:change:confirm"));
const canReviewIdentity = computed(() => hasPermission("identity:change:review"));
const canDowngradeIdentity = computed(() => hasPermission("identity:downgrade:preview") && hasPermission("identity:downgrade:confirm"));

watch(() => props.userId, () => loadAll(), { immediate: true });

async function loadAll() {
  if (!props.userId) return;
  loading.value = true; errorMessage.value = ""; forbidden.value = false;
  try {
    const [profileValue, relationshipValue, financialValue, historyValue, relationHistoryValue, agentValues, centerValues, planValues] = await Promise.all([
      identityManagementApi.profile(props.userId), identityManagementApi.relationship(props.userId), identityManagementApi.financialOverview(props.userId), identityManagementApi.history(props.userId), identityManagementApi.relationshipHistory(props.userId), identityManagementApi.agents(), identityManagementApi.operationCenters(), identityManagementApi.plans()
    ]);
    profile.value = profileValue; relationship.value = relationshipValue; financial.value = financialValue; history.identities = historyValue.identities || []; history.changeRecords = historyValue.changeRecords || []; relationshipHistory.value = relationHistoryValue || []; agents.value = agentValues || []; centers.value = centerValues || []; plans.value = planValues || [];
  } catch (error) {
    const message = error instanceof Error ? error.message : "身份与权益加载失败";
    forbidden.value = /forbidden|permission denied|无权限/i.test(message); errorMessage.value = message;
  } finally { loading.value = false; }
}
function openAction(command: IdentityChangeAction | "REVIEW" | "DOWNGRADE") { if (command === "REVIEW") { reviewOpen.value = true; return; } if (command === "DOWNGRADE") { downgradeOpen.value = true; return; } wizardAction.value = command; wizardOpen.value = true; }
async function openHistory() { historyOpen.value = true; try { downgradeRequests.value = await identityManagementApi.downgradeRequests(props.userId); } catch { downgradeRequests.value = []; } }
async function submitReview() {
  if (reviewing.value) return; reviewing.value = true;
  try { await identityManagementApi.review(props.userId, reviewForm.previewToken, reviewForm.decision, reviewForm.reason); ElMessage.success("审核结果已提交"); reviewOpen.value = false; Object.assign(reviewForm, { previewToken: "", decision: "APPROVED", reason: "" }); }
  catch (error) { ElMessage.error(error instanceof Error ? error.message : "审核失败"); }
  finally { reviewing.value = false; }
}
function identityLabel(value: string) { return ({ USER: "普通用户", AGENT: "代理商", OPERATION_CENTER: "运营中心" } as Record<string, string>)[value] || value || "普通用户"; }
function statusLabel(value: string) { return ({ PENDING: "待生效", ACTIVE: "有效", FROZEN: "已冻结", TERMINATED: "已终止" } as Record<string, string>)[value] || value || "-"; }
function statusTagType(value: string) { return value === "ACTIVE" ? "success" : value === "FROZEN" ? "warning" : value === "TERMINATED" ? "danger" : "info"; }
function downgradeStatusType(value: string) { return value === "SUCCEEDED" ? "success" : value === "FAILED" || value === "CANCELLED" ? "danger" : value === "WAITING" || value === "SCHEDULED" ? "warning" : "info"; }
function dateTime(value: unknown) { if (!value) return "-"; const date = new Date(String(value)); return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString("zh-CN", { hour12: false }); }
function number(value: unknown) { return Number(value || 0).toLocaleString("zh-CN"); }
function money(value: unknown) { return `¥${(Number(value || 0) / 100).toLocaleString("zh-CN", { minimumFractionDigits: 2 })}`; }
</script>

<style scoped>
.identity-panel{display:grid;gap:16px}.identity-toolbar,.card-heading{display:flex;align-items:center;justify-content:space-between;gap:14px}.identity-toolbar>div:first-child{display:grid;gap:4px}.identity-toolbar span{font-weight:700;color:var(--admin-text)}.identity-toolbar small{color:var(--admin-muted)}.toolbar-actions{display:flex;gap:10px}.identity-summary-grid{display:grid;grid-template-columns:minmax(0,1.45fr) minmax(280px,.75fr);gap:14px}.financial-metrics{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px}.financial-metrics article{padding:16px;border:1px solid var(--admin-border);border-radius:10px;background:var(--admin-panel)}.financial-metrics span,.financial-metrics small{display:block;color:var(--admin-muted)}.financial-metrics strong{display:block;margin:7px 0;color:var(--admin-text);font-size:22px}@media(max-width:900px){.identity-summary-grid{grid-template-columns:1fr}.financial-metrics{grid-template-columns:repeat(2,1fr)}.identity-toolbar{align-items:flex-start;flex-direction:column}}@media(max-width:560px){.financial-metrics{grid-template-columns:1fr}.toolbar-actions{width:100%;flex-wrap:wrap}}
</style>
