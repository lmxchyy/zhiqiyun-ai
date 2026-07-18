<template>
  <el-card shadow="never" class="exception-center-card">
    <template #header><div class="panel-head"><span>异常中心</span><el-tag :type="activeCount ? 'danger' : 'success'">{{ activeCount }}</el-tag></div></template>
    <div class="exception-list">
      <button v-for="item in cases" :key="item.id" type="button" @click="openCase(item)">
        <i :class="`is-${item.severity}`"></i>
        <span><strong>{{ item.title }}</strong><small>{{ item.assigneeName || '待分派' }} · {{ statusLabel(item.status) }} · {{ slaLabel(item) }}</small></span>
        <em>{{ item.count || 0 }}</em>
      </button>
      <el-empty v-if="!cases.length" description="当前没有运行异常" :image-size="58" />
    </div>
  </el-card>

  <el-drawer v-model="drawerOpen" size="min(620px, 92vw)" append-to-body>
    <template #header><div class="exception-title"><span>异常处置工单</span><strong>{{ selected?.title }}</strong></div></template>
    <el-alert v-if="selected" :type="selected.count ? 'error' : 'success'" :title="selected.description" :description="`当前影响 ${selected.count || 0} 条记录`" show-icon :closable="false" />
    <el-form v-if="selected" class="exception-form" label-position="top">
      <div class="exception-form-grid">
        <el-form-item label="负责人"><el-input v-model="draft.assigneeName" placeholder="输入负责人姓名" /></el-form-item>
        <el-form-item label="处理状态"><el-select v-model="draft.status"><el-option label="待处理" value="OPEN" /><el-option label="处理中" value="IN_PROGRESS" /><el-option label="已恢复" value="RESOLVED" /><el-option label="已关闭" value="CLOSED" /></el-select></el-form-item>
      </div>
      <el-form-item label="SLA 截止时间"><el-date-picker v-model="draft.slaDueAt" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" placeholder="选择 SLA 截止时间" /></el-form-item>
      <el-form-item label="处理备注"><el-input v-model="draft.note" type="textarea" :rows="3" placeholder="记录诊断、处理动作或交接信息" /></el-form-item>
      <el-form-item v-if="draft.status === 'CLOSED'" label="关闭原因" required><el-input v-model="draft.closeReason" type="textarea" :rows="2" placeholder="关闭工单必须填写原因" /></el-form-item>
      <el-button type="primary" :loading="saving" :disabled="draft.status === 'CLOSED' && !draft.closeReason.trim()" @click="saveCase">保存处置记录</el-button>
    </el-form>
    <el-divider>处理记录</el-divider>
    <el-timeline v-if="selected?.history?.length"><el-timeline-item v-for="(history, index) in [...(selected.history || [])].reverse()" :key="`${history.at}-${index}`" :timestamp="formatTime(history.at)" placement="top"><strong>{{ historyLabel(history.action) }}</strong><p>{{ history.actorName || history.actorId || 'system' }} · {{ history.from || '-' }} → {{ history.to || '-' }}</p><small>{{ history.note || '无补充备注' }}</small></el-timeline-item></el-timeline>
    <el-empty v-else description="暂无处理记录" :image-size="64" />
  </el-drawer>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { ElMessage } from "element-plus/es/components/message/index";
import { adminWorkspaceApi, type AdminExceptionCase } from "../../api/adminWorkspaces";

const props = defineProps<{ items: AdminExceptionCase[] }>();
const emit = defineEmits<{ navigate: [moduleId: string]; updated: [item: AdminExceptionCase] }>();
const cases = ref<AdminExceptionCase[]>([]);
const selected = ref<AdminExceptionCase | null>(null);
const drawerOpen = ref(false);
const saving = ref(false);
const draft = reactive({ assigneeName: "", status: "OPEN", slaDueAt: "", note: "", closeReason: "" });
watch(() => props.items, (items) => { cases.value = (items || []).map((item) => ({ ...item, history: [...(item.history || [])] })); }, { immediate: true, deep: true });
const activeCount = computed(() => cases.value.filter((item) => !["RESOLVED", "CLOSED"].includes(item.status)).reduce((sum, item) => sum + Number(item.count || 0), 0));
function openCase(item: AdminExceptionCase) { selected.value = item; Object.assign(draft, { assigneeName: item.assigneeName || "", status: item.status || "OPEN", slaDueAt: item.slaDueAt || "", note: "", closeReason: item.closeReason || "" }); drawerOpen.value = true; }
async function saveCase() {
  if (!selected.value) return;
  saving.value = true;
  try {
    const previousStatus = selected.value.status;
    const response = await adminWorkspaceApi.updateException(selected.value.id, { assigneeName: draft.assigneeName, status: draft.status as AdminExceptionCase["status"], slaDueAt: draft.slaDueAt, note: draft.note, closeReason: draft.closeReason });
    const index = cases.value.findIndex((item) => item.id === response.item.id); if (index >= 0) cases.value[index] = response.item; selected.value = response.item; emit("updated", response.item);
    if (previousStatus !== "IN_PROGRESS" && response.item.status === "IN_PROGRESS") void adminWorkspaceApi.recordExperience("TASK_STARTED", response.item.module, response.item.id, { source: "exception_center" }).catch(() => undefined);
    if (!["RESOLVED", "CLOSED"].includes(previousStatus) && ["RESOLVED", "CLOSED"].includes(response.item.status)) void adminWorkspaceApi.recordExperience("TASK_COMPLETED", response.item.module, response.item.id, { source: "exception_center" }).catch(() => undefined);
    ElMessage.success("异常处置记录已保存");
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : "异常处置保存失败"); }
  finally { saving.value = false; }
}
function statusLabel(status: string) { return ({ OPEN: "待处理", IN_PROGRESS: "处理中", RESOLVED: "已恢复", CLOSED: "已关闭" } as Record<string, string>)[status] || status; }
function slaLabel(item: AdminExceptionCase) { if (!item.slaDueAt) return "未设置 SLA"; const ms = new Date(item.slaDueAt).getTime() - Date.now(); if (ms < 0 && !["RESOLVED", "CLOSED"].includes(item.status)) return "SLA 已超时"; return `${Math.max(0, Math.ceil(ms / 3600000))} 小时内处理`; }
function formatTime(value: string) { const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN"); }
function historyLabel(action: string) { return ({ DETECTED: "检测到异常", UPDATED: "更新处置", AUTO_RESOLVED: "系统检测恢复", REOPENED: "异常再次发生" } as Record<string, string>)[action] || action; }
</script>

<style scoped>
.panel-head { display: flex; align-items: center; justify-content: space-between; }.exception-list { display: grid; gap: 8px; }.exception-list button { display: grid; grid-template-columns: 8px minmax(0,1fr) auto; align-items: center; gap: 10px; width: 100%; padding: 11px; border: 1px solid var(--admin-border); border-radius: 8px; background: transparent; cursor: pointer; text-align: left; }.exception-list button:hover,.exception-list button:focus-visible { border-color: var(--color-border-active); background: var(--color-primary-light); outline: none; }.exception-list i { width: 8px; height: 32px; border-radius: 8px; background: #94a3b8; }.exception-list i.is-danger { background: #ef4444; }.exception-list i.is-warning { background: #f59e0b; }.exception-list span { display: grid; gap: 3px; min-width: 0; }.exception-list small { overflow: hidden; color: var(--admin-muted); text-overflow: ellipsis; white-space: nowrap; }.exception-list em { font-size: 18px; font-style: normal; font-weight: 700; }.exception-title { display: grid; gap: 4px; }.exception-title span { color: var(--admin-muted); font-size: 12px; }.exception-title strong { font-size: 20px; }.exception-form { margin-top: 18px; }.exception-form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }.exception-form :deep(.el-select),.exception-form :deep(.el-date-editor) { width: 100%; }.exception-form p { margin: 4px 0; }.exception-form small { color: var(--admin-muted); }
@media (max-width: 620px) { .exception-form-grid { grid-template-columns: 1fr; } }
</style>
