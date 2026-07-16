<template>
  <section class="commercial-billing">
    <header class="commercial-billing__header">
      <div>
        <el-tag effect="plain" type="primary">微信虚拟支付商业账务</el-tag>
        <h2>{{ pageTitle }}</h2>
        <p>{{ pageDescription }}</p>
      </div>
      <div class="commercial-billing__actions">
        <el-button v-if="moduleId === 'billingCoupons'" type="primary" :icon="Plus" @click="openCouponDialog">新建优惠券</el-button>
        <el-button v-if="moduleId === 'billingCreditNotes'" type="primary" :icon="Plus" @click="openCreditNoteDialog">新建贷项</el-button>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
      </div>
    </header>

    <el-alert v-if="errorMessage" type="error" :closable="false" show-icon :title="errorMessage" />
    <el-alert
      v-if="moduleId === 'billingCoupons'"
      type="warning"
      :closable="false"
      show-icon
      title="微信虚拟支付优惠券仅加赠积分、图片额度或会员天数，不修改微信商品价格。"
    />
    <el-alert
      v-if="moduleId === 'billingInvoices'"
      type="info"
      :closable="false"
      show-icon
      title="本页管理交易账单与人工税票状态；本期不接自动开票。"
    />
    <el-alert
      v-if="moduleId === 'billingPaymentRequests'"
      type="info"
      :closable="false"
      show-icon
      title="催收动作仅记录人工联系或提醒台账，不会自动发送短信、邮件或绕过微信支付。"
    />

    <div class="commercial-billing__toolbar">
      <el-input v-model="keyword" clearable :prefix-icon="Search" placeholder="搜索当前页面真实账务数据" />
      <el-tag effect="plain">数据库记录 {{ filteredRows.length }} / {{ rows.length }}</el-tag>
    </div>

    <el-card shadow="never" v-loading="loading">
      <el-table v-if="moduleId === 'billingCustomers'" :data="filteredRows" height="650" stripe empty-text="暂无产生过真实账务的客户">
        <el-table-column prop="customerId" label="客户ID" min-width="160" fixed show-overflow-tooltip />
        <el-table-column prop="customerName" label="客户名称" min-width="130" />
        <el-table-column prop="email" label="邮箱" min-width="190" show-overflow-tooltip />
        <el-table-column prop="planId" label="当前套餐" min-width="160" show-overflow-tooltip />
        <el-table-column prop="subscriptionStatus" label="订阅状态" width="120"><template #default="s"><status-tag :value="s.row.subscriptionStatus" /></template></el-table-column>
        <el-table-column prop="availablePoints" label="可用点数" width="110" align="right" />
        <el-table-column prop="frozenPoints" label="冻结点数" width="110" align="right" />
        <el-table-column prop="orderCount" label="订单数" width="90" align="right" />
        <el-table-column label="已支付" width="120" align="right"><template #default="s">{{ cents(s.row.paidCents) }}</template></el-table-column>
        <el-table-column label="已退款" width="120" align="right"><template #default="s">{{ cents(s.row.refundedCents) }}</template></el-table-column>
        <el-table-column label="最近订单" min-width="180"><template #default="s">{{ dateTime(s.row.lastOrderAt) }}</template></el-table-column>
      </el-table>

      <el-table v-else-if="moduleId === 'billingProducts'" :data="filteredRows" height="650" stripe empty-text="暂无微信虚拟支付商品">
        <el-table-column prop="name" label="套餐产品" min-width="170" fixed />
        <el-table-column prop="productCode" label="服务端商品编码" min-width="180" show-overflow-tooltip />
        <el-table-column prop="planType" label="套餐类型" min-width="160" />
        <el-table-column prop="productType" label="权益类型" min-width="160" />
        <el-table-column label="价格" width="110" align="right"><template #default="s">{{ cents(s.row.priceCents) }}</template></el-table-column>
        <el-table-column prop="tokenAmount" label="Token权益" width="120" align="right" />
        <el-table-column prop="durationDays" label="有效天数" width="100" align="right" />
        <el-table-column label="微信映射" min-width="260"><template #default="s"><div class="mapping-list"><el-tag v-for="m in s.row.wechatMappings || []" :key="m.mappingId" size="small" :type="m.enabled ? 'success' : 'info'">{{ m.environment }} · {{ m.wechatProductId }}</el-tag></div></template></el-table-column>
        <el-table-column label="状态" width="100"><template #default="s"><status-tag :value="s.row.active ? 'ACTIVE' : 'INACTIVE'" /></template></el-table-column>
      </el-table>

      <el-table v-else-if="moduleId === 'billingSubscriptions'" :data="filteredRows" height="650" stripe empty-text="暂无真实订阅记录">
        <el-table-column prop="id" label="订阅ID" min-width="180" fixed show-overflow-tooltip />
        <el-table-column prop="customerName" label="客户" min-width="120" />
        <el-table-column prop="tenantId" label="租户" min-width="150" show-overflow-tooltip />
        <el-table-column prop="planName" label="套餐" min-width="160" />
        <el-table-column prop="productCode" label="商品编码" min-width="170" />
        <el-table-column prop="orderNo" label="来源订单" min-width="190" show-overflow-tooltip />
        <el-table-column label="状态" width="110"><template #default="s"><status-tag :value="s.row.status" /></template></el-table-column>
        <el-table-column label="开始时间" min-width="180"><template #default="s">{{ dateTime(s.row.startsAt) }}</template></el-table-column>
        <el-table-column label="结束时间" min-width="180"><template #default="s">{{ dateTime(s.row.endsAt) }}</template></el-table-column>
        <el-table-column label="操作" width="110" fixed="right"><template #default="s"><el-button link type="primary" @click="toggleSubscription(s.row)">{{ s.row.status === 'CANCELLED' ? '恢复' : '取消' }}</el-button></template></el-table-column>
      </el-table>

      <el-table v-else-if="moduleId === 'billingCoupons'" :data="filteredRows" height="650" stripe empty-text="暂无优惠券，点击右上角创建">
        <el-table-column prop="code" label="优惠券码" min-width="150" fixed />
        <el-table-column prop="name" label="名称" min-width="150" />
        <el-table-column prop="benefitType" label="加赠类型" min-width="190" />
        <el-table-column prop="benefitValue" label="加赠值" width="100" align="right" />
        <el-table-column label="适用商品" min-width="230"><template #default="s">{{ (s.row.applicableProductCodes || []).join('、') || '全部商品' }}</template></el-table-column>
        <el-table-column label="使用次数" width="130"><template #default="s">{{ s.row.appliedCount || 0 }} 已使用 / {{ s.row.reservedCount || 0 }} 已占用</template></el-table-column>
        <el-table-column prop="perUserLimit" label="每人上限" width="100" align="right" />
        <el-table-column label="有效期" min-width="250"><template #default="s">{{ dateTime(s.row.startsAt) }} — {{ dateTime(s.row.endsAt) }}</template></el-table-column>
        <el-table-column label="状态" width="100"><template #default="s"><status-tag :value="s.row.status" /></template></el-table-column>
        <el-table-column label="操作" width="100" fixed="right"><template #default="s"><el-button link type="primary" @click="toggleCoupon(s.row)">{{ s.row.status === 'ACTIVE' ? '停用' : '启用' }}</el-button></template></el-table-column>
      </el-table>

      <el-table v-else-if="moduleId === 'billingInvoices'" :data="filteredRows" height="650" stripe empty-text="暂无真实交易账单">
        <el-table-column prop="invoiceNo" label="账单号" min-width="180" fixed />
        <el-table-column prop="orderNo" label="订单号" min-width="190" show-overflow-tooltip />
        <el-table-column prop="customerName" label="客户" min-width="120" />
        <el-table-column prop="tenantId" label="租户" min-width="150" show-overflow-tooltip />
        <el-table-column label="账单金额" width="115" align="right"><template #default="s">{{ cents(s.row.totalCents) }}</template></el-table-column>
        <el-table-column label="已支付" width="115" align="right"><template #default="s">{{ cents(s.row.paidCents) }}</template></el-table-column>
        <el-table-column label="付款状态" width="110"><template #default="s"><status-tag :value="s.row.paymentStatus" /></template></el-table-column>
        <el-table-column label="税票状态" width="120"><template #default="s"><status-tag :value="s.row.taxInvoiceStatus" /></template></el-table-column>
        <el-table-column prop="issuedInvoiceNo" label="开票号码" min-width="160" />
        <el-table-column label="创建时间" min-width="180"><template #default="s">{{ dateTime(s.row.createdAt) }}</template></el-table-column>
        <el-table-column label="操作" width="145" fixed="right"><template #default="s"><el-button link type="primary" @click="manageInvoice(s.row)">{{ s.row.taxInvoiceStatus === 'ISSUED' ? '查看开票' : '处理开票' }}</el-button></template></el-table-column>
      </el-table>

      <el-table v-else-if="moduleId === 'billingCreditNotes'" :data="filteredRows" height="650" stripe empty-text="暂无退款或人工贷项记录">
        <el-table-column prop="creditNoteNo" label="贷项单号" min-width="180" fixed />
        <el-table-column prop="invoiceNo" label="原账单" min-width="180" />
        <el-table-column prop="orderNo" label="原订单" min-width="190" show-overflow-tooltip />
        <el-table-column label="贷项金额" width="120" align="right"><template #default="s">{{ cents(s.row.amountCents) }}</template></el-table-column>
        <el-table-column prop="reason" label="原因" min-width="220" show-overflow-tooltip />
        <el-table-column label="审核状态" width="120"><template #default="s"><status-tag :value="s.row.status" /></template></el-table-column>
        <el-table-column label="退款状态" width="120"><template #default="s"><status-tag :value="s.row.refundStatus" /></template></el-table-column>
        <el-table-column label="创建时间" min-width="180"><template #default="s">{{ dateTime(s.row.createdAt) }}</template></el-table-column>
        <el-table-column label="操作" width="140" fixed="right"><template #default="s"><template v-if="s.row.status === 'PENDING_REVIEW'"><el-button link type="success" @click="reviewCredit(s.row, 'FINALIZED')">通过</el-button><el-button link type="danger" @click="reviewCredit(s.row, 'REJECTED')">驳回</el-button></template></template></el-table-column>
      </el-table>

      <el-table v-else-if="moduleId === 'billingPaymentRequests'" :data="filteredRows" height="650" stripe empty-text="暂无付款请求">
        <el-table-column prop="requestNo" label="付款请求号" min-width="180" fixed />
        <el-table-column prop="invoiceNo" label="账单号" min-width="180" />
        <el-table-column prop="orderNo" label="订单号" min-width="190" show-overflow-tooltip />
        <el-table-column prop="customerName" label="客户" min-width="120" />
        <el-table-column prop="provider" label="支付渠道" min-width="150" />
        <el-table-column label="应付金额" width="120" align="right"><template #default="s">{{ cents(s.row.amountCents) }}</template></el-table-column>
        <el-table-column label="付款状态" width="110"><template #default="s"><status-tag :value="s.row.status" /></template></el-table-column>
        <el-table-column label="催收状态" width="120"><template #default="s"><status-tag :value="s.row.dunningStatus" /></template></el-table-column>
        <el-table-column prop="dunningAttempts" label="催收次数" width="100" align="right" />
        <el-table-column label="到期时间" min-width="180"><template #default="s">{{ dateTime(s.row.expiresAt) }}</template></el-table-column>
        <el-table-column label="操作" width="130" fixed="right"><template #default="s"><el-button v-if="s.row.status === 'PENDING'" link type="primary" @click="recordDunning(s.row)">记录催收</el-button></template></el-table-column>
      </el-table>

      <el-table v-else :data="filteredRows" height="650" stripe empty-text="暂无真实支付记录">
        <el-table-column prop="paymentNo" label="支付流水号" min-width="190" fixed />
        <el-table-column prop="orderNo" label="订单号" min-width="190" show-overflow-tooltip />
        <el-table-column prop="customerName" label="客户" min-width="120" />
        <el-table-column prop="paymentChannel" label="支付渠道" min-width="160" />
        <el-table-column label="金额" width="110" align="right"><template #default="s">{{ cents(s.row.amountCents) }}</template></el-table-column>
        <el-table-column label="状态" width="110"><template #default="s"><status-tag :value="s.row.status" /></template></el-table-column>
        <el-table-column prop="wechatOrderId" label="微信订单ID" min-width="190" show-overflow-tooltip />
        <el-table-column prop="wechatTransactionId" label="微信交易ID" min-width="190" show-overflow-tooltip />
        <el-table-column prop="invoiceNo" label="账单号" min-width="175" />
        <el-table-column prop="failureReason" label="失败原因" min-width="210" show-overflow-tooltip />
        <el-table-column label="支付时间" min-width="180"><template #default="s">{{ dateTime(s.row.paidAt) }}</template></el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="couponDialogVisible" title="新建权益加赠券" width="620px" destroy-on-close>
      <el-form label-width="120px">
        <el-form-item label="优惠券码"><el-input v-model.trim="couponDraft.code" placeholder="例如 SUMMER2026" /></el-form-item>
        <el-form-item label="名称"><el-input v-model.trim="couponDraft.name" /></el-form-item>
        <el-form-item label="加赠类型"><el-select v-model="couponDraft.benefitType" style="width:100%"><el-option label="加赠积分" value="BONUS_CREDITS" /><el-option label="加赠图片额度" value="BONUS_IMAGE_QUOTA" /><el-option label="延长会员天数" value="EXTEND_MEMBERSHIP_DAYS" /></el-select></el-form-item>
        <el-form-item label="加赠值"><el-input-number v-model="couponDraft.benefitValue" :min="1" :precision="0" /></el-form-item>
        <el-form-item label="适用商品"><el-input v-model="couponDraft.productCodes" placeholder="商品编码逗号分隔；留空表示全部" /></el-form-item>
        <el-form-item label="每人上限"><el-input-number v-model="couponDraft.perUserLimit" :min="1" :precision="0" /></el-form-item>
        <el-form-item label="状态"><el-select v-model="couponDraft.status"><el-option label="草稿" value="DRAFT" /><el-option label="启用" value="ACTIVE" /></el-select></el-form-item>
      </el-form>
      <template #footer><el-button @click="couponDialogVisible=false">取消</el-button><el-button type="primary" :loading="saving" @click="saveCoupon">保存</el-button></template>
    </el-dialog>

    <el-dialog v-model="creditDialogVisible" title="新建人工贷项" width="560px" destroy-on-close>
      <el-form label-width="110px">
        <el-form-item label="账单ID"><el-input v-model.trim="creditDraft.invoiceId" placeholder="填写账单内部 ID" /></el-form-item>
        <el-form-item label="贷项金额"><el-input-number v-model="creditDraft.amountCents" :min="1" :precision="0" /><span class="field-note">分</span></el-form-item>
        <el-form-item label="原因"><el-input v-model.trim="creditDraft.reason" type="textarea" :rows="4" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="creditDialogVisible=false">取消</el-button><el-button type="primary" :loading="saving" @click="saveCreditNote">提交审核</el-button></template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, reactive, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { Plus, Refresh, Search } from "@element-plus/icons-vue";
import { commercialBillingApi, type CommercialBillingRow } from "../../api/commercialBilling";

const props = defineProps<{ moduleId: string }>();
const rows = ref<CommercialBillingRow[]>([]);
const keyword = ref("");
const loading = ref(false);
const saving = ref(false);
const errorMessage = ref("");

const titles: Record<string, [string, string]> = {
  billingCustomers: ["客户计费", "按真实订单、订阅与钱包余额聚合客户账务，不生成演示客户。"],
  billingProducts: ["套餐产品", "统一查看服务端套餐价格、权益和微信虚拟商品映射。"],
  billingSubscriptions: ["订阅管理", "订阅由已支付订单和会员权益记录生成，可追溯来源订单。"],
  billingCoupons: ["优惠券", "创建不改变微信实付金额的权益加赠券，并追踪占用与核销。"],
  billingInvoices: ["发票与交易账单", "每笔微信虚拟支付订单自动形成账单，税票采用人工申请与开具状态。"],
  billingCreditNotes: ["贷项红冲", "退款通知自动生成贷项，人工贷项需审核且不会直接发起退款。"],
  billingPaymentRequests: ["付款请求与催收", "订单创建即生成付款请求，支付、关闭和退款状态自动回写。"],
  billingPayments: ["支付记录", "查看微信签单、回调、交易号、失败原因及关联账单。"]
};
const pageTitle = computed(() => titles[props.moduleId]?.[0] || "商业计费");
const pageDescription = computed(() => titles[props.moduleId]?.[1] || "");
const query = computed(() => keyword.value.trim().toLowerCase());
const filteredRows = computed(() => rows.value.filter((row) => !query.value || JSON.stringify(row).toLowerCase().includes(query.value)));

const statusType = (value: string) => {
  const status = String(value || "").toUpperCase();
  if (["ACTIVE", "PAID", "SUCCEEDED", "ISSUED", "FINALIZED", "RESOLVED"].includes(status)) return "success";
  if (["FAILED", "REJECTED", "CANCELLED", "EXPIRED", "INACTIVE"].includes(status)) return "danger";
  if (["PENDING", "PENDING_REVIEW", "REQUESTED", "RESERVED", "IN_PROGRESS", "DRAFT"].includes(status)) return "warning";
  return "info";
};
const StatusTag = defineComponent({ props: { value: { type: String, default: "-" } }, setup(p) { return () => h("span", { class: `commercial-status commercial-status--${statusType(p.value)}` }, p.value || "-"); } });

function cents(value: unknown) { return `¥${(Number(value || 0) / 100).toFixed(2)}`; }
function dateTime(value: unknown) { if (!value) return "-"; const date = new Date(String(value)); return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString("zh-CN", { hour12: false }); }

async function load() {
  loading.value = true; errorMessage.value = "";
  try { rows.value = (await commercialBillingApi.list(props.moduleId)).items || []; }
  catch (error) { errorMessage.value = error instanceof Error ? error.message : "商业计费数据加载失败"; }
  finally { loading.value = false; }
}
watch(() => props.moduleId, () => { keyword.value = ""; void load(); }, { immediate: true });

const couponDialogVisible = ref(false);
const couponDraft = reactive({ code: "", name: "", benefitType: "BONUS_CREDITS", benefitValue: 1, productCodes: "", perUserLimit: 1, status: "DRAFT" });
function openCouponDialog() { Object.assign(couponDraft, { code: "", name: "", benefitType: "BONUS_CREDITS", benefitValue: 1, productCodes: "", perUserLimit: 1, status: "DRAFT" }); couponDialogVisible.value = true; }
async function saveCoupon() {
  if (!couponDraft.code || !couponDraft.name) return ElMessage.warning("请填写优惠券码和名称");
  saving.value = true;
  try {
    await commercialBillingApi.createCoupon({ code: couponDraft.code, name: couponDraft.name, benefitType: couponDraft.benefitType, benefitValue: couponDraft.benefitValue, applicableProductCodes: couponDraft.productCodes.split(/[,，]/).map(v => v.trim()).filter(Boolean), perUserLimit: couponDraft.perUserLimit, status: couponDraft.status });
    couponDialogVisible.value = false; ElMessage.success("优惠券已保存"); await load();
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : "保存失败"); } finally { saving.value = false; }
}
async function toggleCoupon(row: CommercialBillingRow) { await commercialBillingApi.updateCoupon(String(row.id), { status: row.status === "ACTIVE" ? "INACTIVE" : "ACTIVE" }); ElMessage.success("优惠券状态已更新"); await load(); }

async function toggleSubscription(row: CommercialBillingRow) {
  const status = row.status === "CANCELLED" ? "ACTIVE" : "CANCELLED";
  await ElMessageBox.confirm(`确认将订阅设为 ${status}？此操作不会直接改写钱包余额。`, "订阅状态", { type: "warning" });
  await commercialBillingApi.updateSubscription(String(row.id), { status }); ElMessage.success("订阅状态已更新"); await load();
}

async function manageInvoice(row: CommercialBillingRow) {
  if (row.taxInvoiceStatus === "ISSUED") return ElMessageBox.alert(`开票号码：${row.issuedInvoiceNo || "-"}\n开票时间：${dateTime(row.issuedAt)}`, "开票信息");
  const result = await ElMessageBox.prompt("输入已开具的发票号码；留空将状态设为已申请", "处理税票", { confirmButtonText: "保存", cancelButtonText: "取消", inputPlaceholder: "发票号码" });
  const issuedInvoiceNo = String(result.value || "").trim();
  await commercialBillingApi.updateInvoice(String(row.id), { taxInvoiceStatus: issuedInvoiceNo ? "ISSUED" : "REQUESTED", issuedInvoiceNo, taxTitle: row.taxTitle || "", taxNumber: row.taxNumber || "", taxEmail: row.taxEmail || "" });
  ElMessage.success("税票状态已更新"); await load();
}

const creditDialogVisible = ref(false);
const creditDraft = reactive({ invoiceId: "", amountCents: 1, reason: "" });
function openCreditNoteDialog() { Object.assign(creditDraft, { invoiceId: "", amountCents: 1, reason: "" }); creditDialogVisible.value = true; }
async function saveCreditNote() {
  if (!creditDraft.invoiceId || !creditDraft.reason) return ElMessage.warning("请填写账单ID和原因");
  saving.value = true;
  try { await commercialBillingApi.createCreditNote({ ...creditDraft }); creditDialogVisible.value = false; ElMessage.success("贷项已提交审核，不会自动退款"); await load(); }
  catch (error) { ElMessage.error(error instanceof Error ? error.message : "提交失败"); } finally { saving.value = false; }
}
async function reviewCredit(row: CommercialBillingRow, status: "FINALIZED" | "REJECTED") { await commercialBillingApi.reviewCreditNote(String(row.id), { status }); ElMessage.success(status === "FINALIZED" ? "贷项已确认；退款仍需走微信退款流程" : "贷项已驳回"); await load(); }

async function recordDunning(row: CommercialBillingRow) {
  const result = await ElMessageBox.prompt("记录本次人工联系说明。系统只留痕，不自动发送消息。", "记录催收", { inputType: "textarea", confirmButtonText: "记录", cancelButtonText: "取消" });
  await commercialBillingApi.recordDunning(String(row.id), { action: "MANUAL_CONTACT", channel: "MANUAL", note: String(result.value || "") });
  ElMessage.success("催收动作已记录"); await load();
}
</script>

<style scoped>
.commercial-billing { display: grid; gap: 16px; }
.commercial-billing__header { display: flex; justify-content: space-between; gap: 24px; align-items: flex-start; padding: 20px 22px; border: 1px solid #e6eaf2; border-radius: 16px; background: linear-gradient(135deg, #fff, #f6f9ff); }
.commercial-billing__header h2 { margin: 10px 0 5px; font-size: 24px; color: #15233d; }
.commercial-billing__header p { margin: 0; color: #68758a; }
.commercial-billing__actions { display: flex; gap: 10px; flex-wrap: wrap; justify-content: flex-end; }
.commercial-billing__toolbar { display: flex; gap: 12px; align-items: center; }
.commercial-billing__toolbar .el-input { max-width: 520px; }
.mapping-list { display: flex; flex-wrap: wrap; gap: 5px; }
.field-note { margin-left: 8px; color: #758198; }
.commercial-status { display: inline-flex; padding: 3px 9px; border-radius: 999px; font-size: 12px; font-weight: 700; background: #eef2f7; color: #56647a; }
.commercial-status--success { background: #e9f8ef; color: #20834c; }
.commercial-status--warning { background: #fff4dc; color: #a76a00; }
.commercial-status--danger { background: #ffebed; color: #bf3441; }
@media (max-width: 900px) { .commercial-billing__header, .commercial-billing__toolbar { flex-direction: column; align-items: stretch; } .commercial-billing__actions { justify-content: flex-start; } }
</style>
