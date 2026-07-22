<template>
  <el-dialog v-model="visible" width="min(920px, 94vw)" :close-on-click-modal="false" destroy-on-close @closed="resetWizard">
    <template #header>
      <div class="wizard-title">
        <div><span>用户身份管理</span><strong>{{ actionLabel }}</strong></div>
        <el-tag :type="preview?.highRisk ? 'danger' : 'info'">{{ preview?.highRisk ? '高风险操作' : '受控操作' }}</el-tag>
      </div>
    </template>

    <el-steps :active="step" finish-status="success" align-center class="identity-steps">
      <el-step title="目标身份" />
      <el-step title="变更方式" />
      <el-step title="上下级关系" />
      <el-step title="订单与权益" />
      <el-step title="预览影响" />
      <el-step title="二次确认" />
    </el-steps>

    <el-alert v-if="errorMessage" type="error" :title="errorMessage" show-icon closable @close="errorMessage = ''" />

    <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="wizard-form">
      <section v-show="step === 0" class="wizard-section">
        <h3>选择目标身份</h3>
        <el-radio-group v-if="form.action === 'UPGRADE'" v-model="form.targetIdentity" class="identity-choice-grid">
          <el-radio-button value="AGENT" :disabled="currentIdentity === 'AGENT'"><strong>代理商</strong><small>推广、客户与分润权限</small></el-radio-button>
          <el-radio-button value="OPERATION_CENTER"><strong>运营中心</strong><small>使用独立运营中心工作台，仅超级管理员可升级</small></el-radio-button>
        </el-radio-group>
        <el-descriptions v-else :column="2" border>
          <el-descriptions-item label="当前身份">{{ identityLabel(currentIdentity) }}</el-descriptions-item>
          <el-descriptions-item label="本次操作">{{ actionLabel }}</el-descriptions-item>
        </el-descriptions>
      </section>

      <section v-show="step === 1" class="wizard-section">
        <h3>选择变更方式</h3>
        <el-radio-group v-model="form.method" class="method-choice-grid" :disabled="form.action !== 'UPGRADE'">
          <el-radio border value="ONLY_IDENTITY"><strong>仅调整身份</strong><small>无订单、Token 与分润</small></el-radio>
          <el-radio border value="OFFLINE_ORDER"><strong>线下升级订单</strong><small>付款凭证、真实订单及分润</small></el-radio>
          <el-radio border value="SPECIAL_GRANT"><strong>特殊授权</strong><small>不形成收入，可独立赠送 Token</small></el-radio>
          <el-radio border value="PACKAGE_CONVERSION"><strong>套餐转换</strong><small>会员转代理商，必须审核</small></el-radio>
        </el-radio-group>
      </section>

      <section v-show="step === 2" class="wizard-section">
        <h3>配置上下级关系</h3>
        <div class="form-grid">
          <el-form-item label="上级代理商">
            <el-select v-model="form.parentAgentId" clearable filterable placeholder="不设置或解除上级代理商" :disabled="form.action === 'ADJUST_OPERATION_CENTER'">
              <el-option v-for="item in activeAgents" :key="item.id" :label="optionLabel(item)" :value="item.id" :disabled="item.userId === userId" />
            </el-select>
          </el-form-item>
          <el-form-item :label="form.parentAgentId ? '所属运营中心（由上级代理商推导）' : '所属运营中心'">
            <el-select v-model="form.operationCenterId" clearable filterable placeholder="不设置或解除运营中心" :disabled="Boolean(form.parentAgentId) || form.action === 'ADJUST_PARENT_AGENT'">
              <el-option v-for="item in activeCenters" :key="item.id" :label="optionLabel(item)" :value="item.id" :disabled="item.userId === userId" />
            </el-select>
          </el-form-item>
        </div>
        <el-alert type="info" :closable="false" show-icon title="关系调整只影响确认生效后的新订单，不会回溯修改历史分润。" />
      </section>

      <section v-show="step === 3" class="wizard-section">
        <h3>配置订单、Token 和分润</h3>
        <template v-if="needsPlan">
          <div class="form-grid">
            <el-form-item label="升级套餐" prop="planId">
              <el-select v-model="form.planId" placeholder="请选择匹配的身份套餐" @change="applyPlanDefaults">
                <el-option v-for="item in matchingPlans" :key="item.id" :label="`${item.name}（${money(item.priceCents)}）`" :value="item.id" />
              </el-select>
            </el-form-item>
            <el-form-item v-if="form.method !== 'PACKAGE_CONVERSION'" label="实付金额（元）" prop="paidAmountYuan">
              <el-input-number v-model="form.paidAmountYuan" :min="0" :precision="2" :step="100" controls-position="right" />
            </el-form-item>
          </div>
        </template>
        <el-form-item v-if="form.method === 'OFFLINE_ORDER'" label="套餐 Token">
          <el-switch v-model="form.grantPackageToken" active-text="按套餐发放" inactive-text="不发放" />
        </el-form-item>
        <el-form-item v-if="form.method === 'SPECIAL_GRANT'" label="独立赠送 Token">
          <el-input-number v-model="form.giftTokenAmount" :min="0" :step="100" controls-position="right" />
        </el-form-item>
        <el-form-item v-if="form.method === 'PACKAGE_CONVERSION'" label="Token 处理方式" prop="conversionTokenPolicy">
          <el-radio-group v-model="form.conversionTokenPolicy">
            <el-radio value="KEEP_EXISTING">保留已有 Token，不重复发放</el-radio>
            <el-radio value="ADJUST_DIFFERENCE">按新旧套餐权益差额补发或扣减</el-radio>
          </el-radio-group>
        </el-form-item>
        <div v-if="requiresPaymentProof" class="form-grid">
          <el-form-item label="付款凭证编号" prop="paymentProofReference"><el-input v-model.trim="form.paymentProofReference" placeholder="银行流水号、收据号等" /></el-form-item>
          <el-form-item label="存储文件 ID"><el-input v-model.trim="form.paymentProofStorageFileId" placeholder="通过文件中心上传后填写 fileId（可选）" /></el-form-item>
          <el-form-item label="付款人" prop="paymentProofPayerName"><el-input v-model.trim="form.paymentProofPayerName" /></el-form-item>
          <el-form-item label="付款渠道" prop="paymentProofChannel"><el-select v-model="form.paymentProofChannel"><el-option label="银行转账" value="BANK_TRANSFER" /><el-option label="微信" value="WECHAT" /><el-option label="支付宝" value="ALIPAY" /><el-option label="其他" value="OTHER" /></el-select></el-form-item>
          <el-form-item label="付款时间" prop="paymentProofPaidAt"><el-date-picker v-model="form.paymentProofPaidAt" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" /></el-form-item>
          <el-form-item label="凭证备注"><el-input v-model.trim="form.paymentProofRemark" /></el-form-item>
        </div>
        <el-form-item v-if="isSpecialPrice" label="特殊价格/折扣原因" prop="discountReason"><el-input v-model.trim="form.discountReason" type="textarea" :rows="2" placeholder="金额与套餐价不一致时必填，并由另一名管理员审核" /></el-form-item>
        <el-form-item label="操作原因" prop="reason"><el-input v-model.trim="form.reason" type="textarea" :rows="3" maxlength="300" show-word-limit placeholder="必填，将写入身份历史和管理员审计日志" /></el-form-item>
        <el-form-item label="内部备注"><el-input v-model.trim="form.remark" maxlength="300" /></el-form-item>
        <el-alert type="warning" :closable="false" show-icon title="Token 和分润金额均由服务端套餐、原订单及分润规则计算，页面不提供余额或分润金额编辑。" />
      </section>

      <section v-show="step === 4" class="wizard-section preview-section" v-loading="previewing">
        <template v-if="preview">
          <div class="preview-header">
            <div><span>用户</span><strong>{{ userName || userId }}</strong><small>{{ userId }}</small></div>
            <el-tag :type="previewStatusType">{{ previewStatusLabel }}</el-tag>
          </div>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="原身份">{{ identityLabel(preview.oldIdentity) }}</el-descriptions-item>
            <el-descriptions-item label="新身份">{{ identityLabel(preview.targetIdentity) }}</el-descriptions-item>
            <el-descriptions-item label="原上级代理商">{{ relationName(preview.relationshipBefore.parentAgentId, 'agent') }}</el-descriptions-item>
            <el-descriptions-item label="新上级代理商">{{ relationName(preview.relationshipAfter.parentAgentId, 'agent') }}</el-descriptions-item>
            <el-descriptions-item label="原运营中心">{{ relationName(preview.relationshipBefore.operationCenterId, 'center') }}</el-descriptions-item>
            <el-descriptions-item label="新运营中心">{{ relationName(preview.relationshipAfter.operationCenterId, 'center') }}</el-descriptions-item>
            <el-descriptions-item label="订单实付">{{ money(preview.paidAmountCents) }}</el-descriptions-item>
            <el-descriptions-item label="原价/折扣">{{ money(preview.originalAmountCents) }} / {{ money(preview.discountAmountCents) }}</el-descriptions-item>
            <el-descriptions-item label="Token 变化"><span :class="tokenClass(preview.tokenDelta)">{{ signedNumber(preview.tokenDelta) }}</span></el-descriptions-item>
            <el-descriptions-item label="是否产生分润">{{ preview.commissionGenerated ? '是' : '否' }}</el-descriptions-item>
            <el-descriptions-item label="生效时间">确认成功后立即生效</el-descriptions-item>
            <el-descriptions-item label="操作原因" :span="2">{{ form.reason }}</el-descriptions-item>
            <el-descriptions-item label="预览凭证">用户 {{ preview.userId }} · {{ preview.action }} · {{ previewCountdown }} · {{ preview.status === 'APPROVED' ? '已审核' : '未审核' }}</el-descriptions-item>
          </el-descriptions>
          <el-table v-if="preview.estimatedCommissions.length" :data="preview.estimatedCommissions" size="small" border>
            <el-table-column prop="beneficiaryType" label="分润对象类型" />
            <el-table-column label="分润对象"><template #default="scope">{{ relationName(scope.row.beneficiaryId, scope.row.beneficiaryType === 'OPERATION_CENTER' ? 'center' : 'agent') }}</template></el-table-column>
            <el-table-column prop="ruleCode" label="规则" min-width="180" />
            <el-table-column label="预计金额"><template #default="scope">{{ money(scope.row.amountCents) }}</template></el-table-column>
          </el-table>
          <el-alert v-if="preview.blockers.length" type="error" title="当前不可执行" :closable="false" show-icon><ul><li v-for="item in preview.blockers" :key="item">{{ item }}</li></ul></el-alert>
          <el-button v-if="preview.status === 'BLOCKED' && form.method === 'PACKAGE_CONVERSION' && preview.paidAmountCents > 0" @click="applyConversionDifference">应用服务端计算差额 {{ money(preview.paidAmountCents) }}</el-button>
          <el-alert v-if="preview.riskWarnings.length" type="warning" title="风险提示" :closable="false" show-icon><ul><li v-for="item in preview.riskWarnings" :key="item">{{ item }}</li></ul></el-alert>
          <el-alert v-if="preview.reviewRequired" type="info" :closable="false" show-icon title="套餐转换需要另一名管理员审核。可复制审核凭证交由审核人处理。">
            <el-button size="small" @click="copyPreviewToken">复制审核凭证</el-button>
          </el-alert>
        </template>
        <el-empty v-else description="尚未生成预览" />
      </section>

      <section v-show="step === 5" class="wizard-section confirm-section">
        <el-result icon="warning" title="请进行最后确认" sub-title="确认后将在同一数据库事务内写入身份、关系、订单及相关流水。" />
        <el-checkbox v-model="confirmationChecked" size="large">我已核对用户、身份、关系、金额、Token、分润和风险提示</el-checkbox>
        <el-alert v-if="preview?.reviewRequired" type="warning" :closable="false" title="请确认该 Preview 已由另一名管理员审核通过，否则服务端将拒绝执行。" />
      </section>
    </el-form>

    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button v-if="step > 0" :disabled="submitting" @click="step--">上一步</el-button>
      <el-button v-if="step < 5" type="primary" :loading="previewing" :disabled="nextDisabled" @click="nextStep">下一步</el-button>
      <el-button v-else type="danger" :loading="submitting" :disabled="!confirmationChecked" @click="confirmChange">确认执行</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from "vue";
import { ElMessage, type FormInstance, type FormRules } from "element-plus";
import { identityManagementApi, type BusinessIdentityType, type IdentityChangeAction, type IdentityChangeMethod, type IdentityChangePreview, type IdentityChangePreviewRequest, type IdentityOption, type IdentityPlanOption, type UserRelationship } from "../../api/identityManagement";

const props = defineProps<{ userId: string; userName?: string; currentIdentity: BusinessIdentityType; relationship: UserRelationship | null; agents: IdentityOption[]; centers: IdentityOption[]; plans: IdentityPlanOption[]; initialAction: IdentityChangeAction }>();
const emit = defineEmits<{ success: [] }>();
const visible = defineModel<boolean>({ required: true });
const step = ref(0);
const preview = ref<IdentityChangePreview | null>(null);
const previewing = ref(false);
const submitting = ref(false);
const confirmationChecked = ref(false);
const errorMessage = ref("");
const formRef = ref<FormInstance>();

const form = reactive({ action: "UPGRADE" as IdentityChangeAction, targetIdentity: "AGENT" as "AGENT" | "OPERATION_CENTER", method: "ONLY_IDENTITY" as IdentityChangeMethod, planId: "", parentAgentId: "", operationCenterId: "", paidAmountYuan: 0, grantPackageToken: false, giftTokenAmount: 0, conversionTokenPolicy: "" as "" | "KEEP_EXISTING" | "ADJUST_DIFFERENCE", paymentProofReference: "", paymentProofStorageFileId: "", paymentProofPayerName: "", paymentProofPaidAt: "", paymentProofChannel: "", paymentProofRemark: "", discountReason: "", reason: "", remark: "" });
const rules: FormRules = { planId: [{ validator: (_rule, value, callback) => needsPlan.value && !value ? callback(new Error("请选择升级套餐")) : callback(), trigger: "change" }], conversionTokenPolicy: [{ validator: (_rule, value, callback) => form.method === "PACKAGE_CONVERSION" && !value ? callback(new Error("请选择 Token 处理方式")) : callback(), trigger: "change" }], paymentProofReference: [{ validator: (_rule, value, callback) => requiresPaymentProof.value && !value ? callback(new Error("请填写付款凭证编号")) : callback(), trigger: "blur" }], paymentProofPayerName: [{ validator: (_rule, value, callback) => requiresPaymentProof.value && !value ? callback(new Error("请填写付款人")) : callback(), trigger: "blur" }], paymentProofChannel: [{ validator: (_rule, value, callback) => requiresPaymentProof.value && !value ? callback(new Error("请选择付款渠道")) : callback(), trigger: "change" }], paymentProofPaidAt: [{ validator: (_rule, value, callback) => requiresPaymentProof.value && !value ? callback(new Error("请选择付款时间")) : callback(), trigger: "change" }], discountReason: [{ validator: (_rule, value, callback) => isSpecialPrice.value && !value ? callback(new Error("特殊价格必须填写折扣原因")) : callback(), trigger: "blur" }], reason: [{ required: true, message: "请填写操作原因", trigger: "blur" }, { min: 4, message: "操作原因至少 4 个字符", trigger: "blur" }] };

const actionLabel = computed(() => ({ UPGRADE: "调整身份", ADJUST_PARENT_AGENT: "调整上级代理商", ADJUST_OPERATION_CENTER: "调整运营中心", FREEZE: "冻结身份", RESTORE: "恢复身份", TERMINATE: "终止身份" }[form.action]));
const needsPlan = computed(() => form.method === "OFFLINE_ORDER" || form.method === "PACKAGE_CONVERSION");
const requiresPaymentProof = computed(() => form.method === "OFFLINE_ORDER" || (form.method === "PACKAGE_CONVERSION" && form.paidAmountYuan > 0));
const selectedPlan = computed(() => props.plans.find((item) => item.id === form.planId));
const isSpecialPrice = computed(() => form.method === "OFFLINE_ORDER" && Boolean(selectedPlan.value) && Math.round(form.paidAmountYuan * 100) !== selectedPlan.value!.priceCents);
const nowTick = ref(Date.now()); let countdownTimer = 0;
const previewCountdown = computed(() => { if (!preview.value?.expiresAt) return "-"; const seconds=Math.max(0,Math.floor((new Date(preview.value.expiresAt).getTime()-nowTick.value)/1000)); return `${Math.floor(seconds/60)}分${seconds%60}秒后过期`; });
const activeAgents = computed(() => props.agents.filter((item) => !item.status || String(item.status).toUpperCase() === "ACTIVE"));
const activeCenters = computed(() => props.centers.filter((item) => !item.status || String(item.status).toUpperCase() === "ACTIVE"));
const matchingPlans = computed(() => props.plans.filter((item) => {
  const planType = String(item.entitlements?.planType || "").toUpperCase();
  return item.active && (form.targetIdentity === "AGENT" ? planType === "AGENT_JOIN_PACKAGE" || item.id.includes("agent") : planType === "OPERATION_CENTER_PACKAGE" || item.id.includes("operation_center"));
}));
const nextDisabled = computed(() => step.value === 4 && (!preview.value || preview.value.blockers.length > 0));
const previewStatusType = computed(() => preview.value?.status === "BLOCKED" || preview.value?.status === "REJECTED" ? "danger" : preview.value?.reviewRequired ? "warning" : "success");
const previewStatusLabel = computed(() => ({ READY: "可执行", BLOCKED: "已阻断", REVIEW_REQUIRED: "待审核", APPROVED: "已审核", REJECTED: "已拒绝", CONSUMED: "已执行" }[preview.value?.status || "READY"]));

watch(visible, (open) => { if (open) initializeForm(); });
watch(() => form.method, () => { preview.value = null; if (form.method === "ONLY_IDENTITY") { form.planId = ""; form.paidAmountYuan = 0; form.giftTokenAmount = 0; } });
watch(() => form.parentAgentId, (id) => { if (!id) return; form.operationCenterId = props.agents.find((item) => item.id === id)?.operationCenterId || ""; });
onMounted(()=>{countdownTimer=window.setInterval(()=>{nowTick.value=Date.now()},1000)});onUnmounted(()=>window.clearInterval(countdownTimer));

function initializeForm() {
  resetWizard();
  form.action = props.initialAction;
  form.targetIdentity = props.currentIdentity === "AGENT" || props.currentIdentity === "OPERATION_CENTER" ? "OPERATION_CENTER" : "AGENT";
  form.method = form.action === "UPGRADE" ? "ONLY_IDENTITY" : "ONLY_IDENTITY";
  form.parentAgentId = props.relationship?.parentAgentId || "";
  form.operationCenterId = props.relationship?.operationCenterId || "";
}
function resetWizard() {
  step.value = 0; preview.value = null; previewing.value = false; submitting.value = false; confirmationChecked.value = false; errorMessage.value = "";
  Object.assign(form, { action: props.initialAction, targetIdentity: "AGENT", method: "ONLY_IDENTITY", planId: "", parentAgentId: "", operationCenterId: "", paidAmountYuan: 0, grantPackageToken: false, giftTokenAmount: 0, conversionTokenPolicy: "", paymentProofReference: "", paymentProofStorageFileId: "", paymentProofPayerName: "", paymentProofPaidAt: "", paymentProofChannel: "", paymentProofRemark: "", discountReason: "", reason: "", remark: "" });
}
async function nextStep() {
  errorMessage.value = "";
  if (step.value === 0 && form.action === "UPGRADE" && !form.targetIdentity) return;
  if (step.value === 3) {
    const valid = await formRef.value?.validate().catch(() => false);
    if (!valid) return;
    await createPreview();
    step.value = 4;
    return;
  }
  if (step.value === 4 && preview.value && !preview.value.blockers.length) { step.value = 5; return; }
  step.value++;
}
async function createPreview() {
  previewing.value = true;
  try { preview.value = await identityManagementApi.preview(props.userId, requestPayload()); }
  catch (error) { errorMessage.value = error instanceof Error ? error.message : "身份变更预检查失败"; }
  finally { previewing.value = false; }
}
function requestPayload(): IdentityChangePreviewRequest {
  return { action: form.action, method: form.method, targetIdentity: form.action === "UPGRADE" ? form.targetIdentity : undefined, planId: form.planId || undefined, parentAgentId: form.parentAgentId, operationCenterId: form.operationCenterId, paidAmountCents: Math.round(Number(form.paidAmountYuan || 0) * 100), grantPackageToken: form.grantPackageToken, giftTokenAmount: Number(form.giftTokenAmount || 0), conversionTokenPolicy: form.conversionTokenPolicy || undefined, paymentProof: { reference: form.paymentProofReference, storageFileId: form.paymentProofStorageFileId, payerName: form.paymentProofPayerName, paidAt: form.paymentProofPaidAt, paymentChannel: form.paymentProofChannel, remark: form.paymentProofRemark }, discountReason: form.discountReason, reason: form.reason, remark: form.remark };
}
async function confirmChange() {
  if (!preview.value || submitting.value) return;
  submitting.value = true; errorMessage.value = "";
  try {
    await identityManagementApi.confirm(props.userId, preview.value.previewToken, preview.value.highRisk);
    ElMessage.success("身份变更已完成"); visible.value = false; emit("success");
  } catch (error) { errorMessage.value = error instanceof Error ? error.message : "身份变更确认失败"; }
  finally { submitting.value = false; }
}
function applyPlanDefaults() { const plan = props.plans.find((item) => item.id === form.planId); if (plan && form.method === "OFFLINE_ORDER") form.paidAmountYuan = plan.priceCents / 100; }
function applyConversionDifference() { if (!preview.value) return; form.paidAmountYuan = preview.value.paidAmountCents / 100; preview.value = null; step.value = 3; }
async function copyPreviewToken() { if (!preview.value?.previewToken) return; await navigator.clipboard.writeText(preview.value.previewToken); ElMessage.success("审核凭证已复制"); }
function relationName(id: string | undefined, type: "agent" | "center") { if (!id) return "未设置"; const source = type === "agent" ? props.agents : props.centers; const item = source.find((option) => option.id === id); return item ? optionLabel(item) : id; }
function optionLabel(item: IdentityOption) { return item.name || item.owner || item.userId || item.id; }
function identityLabel(value: string) { return ({ USER: "普通用户", AGENT: "代理商", OPERATION_CENTER: "运营中心" } as Record<string, string>)[value] || value || "普通用户"; }
function money(cents: number) { return `¥${(Number(cents || 0) / 100).toLocaleString("zh-CN", { minimumFractionDigits: 2 })}`; }
function signedNumber(value: number) { return value > 0 ? `+${value.toLocaleString()}` : value.toLocaleString(); }
function tokenClass(value: number) { return value > 0 ? "positive" : value < 0 ? "negative" : ""; }
</script>

<style scoped>
.wizard-title,.preview-header{display:flex;align-items:center;justify-content:space-between;gap:16px}.wizard-title>div,.preview-header>div{display:grid;gap:3px}.wizard-title span,.preview-header span,.preview-header small{color:var(--admin-muted);font-size:12px}.wizard-title strong{font-size:19px}.identity-steps{margin:4px 0 24px}.wizard-form{min-height:430px}.wizard-section{display:grid;gap:18px}.wizard-section h3{margin:0;color:var(--admin-text)}.identity-choice-grid,.method-choice-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px;width:100%}.identity-choice-grid :deep(.el-radio-button__inner),.method-choice-grid :deep(.el-radio){width:100%;height:auto;min-height:74px;display:grid;justify-items:start;gap:5px;padding:16px}.identity-choice-grid small,.method-choice-grid small{display:block;color:var(--admin-muted)}.form-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px}.form-grid :deep(.el-select),.form-grid :deep(.el-input-number){width:100%}.preview-section :deep(.el-alert__content) ul{margin:8px 0 0;padding-left:18px}.positive{color:var(--el-color-success);font-weight:700}.negative{color:var(--el-color-danger);font-weight:700}.confirm-section{justify-items:center}.confirm-section .el-alert{width:100%}@media(max-width:760px){.identity-choice-grid,.method-choice-grid,.form-grid{grid-template-columns:1fr}.wizard-form{min-height:520px}}
</style>
