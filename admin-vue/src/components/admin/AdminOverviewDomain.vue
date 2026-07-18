<template>
  <section class="role-workspace-hero">
    <div>
      <el-tag effect="dark">{{ roleWorkspace.label }}</el-tag>
      <h2>{{ roleWorkspace.title }}</h2>
      <p>{{ roleWorkspace.description }}</p>
    </div>
    <div class="role-workspace-actions">
      <button v-for="item in roleWorkspace.shortcuts" :key="item.module" type="button" @click="$emit('navigate', item.module)"><strong>{{ item.title }}</strong><small>{{ item.desc }}</small></button>
    </div>
  </section>

  <section class="operations-inbox" aria-label="运营待办与异常中心">
    <el-card shadow="never">
      <template #header><div class="panel-head"><span>待办中心</span><el-tag type="warning">{{ visibleTasks.reduce((total, item) => total + Number(item.count || 0), 0) }}</el-tag></div></template>
      <div class="operations-list">
        <button v-for="item in visibleTasks" :key="item.id" type="button" @click="$emit('navigate', item.module)"><i :class="`is-${item.severity}`"></i><span><strong>{{ item.title }}</strong><small>{{ item.description }}</small></span><em>{{ item.count || 0 }}</em></button>
        <el-empty v-if="!visibleTasks.length" description="当前角色暂无待办" :image-size="58" />
      </div>
    </el-card>
    <AdminExceptionCenter :items="visibleExceptions" />
  </section>

  <section v-if="activeModuleId === 'analysis'" class="analysis-page">
    <div class="analysis-stat-grid">
      <article v-for="stat in analysisStats" :key="stat.label" class="analysis-stat-card">
        <el-icon><component :is="stat.icon" /></el-icon>
        <div><span>{{ stat.label }}</span><strong>{{ stat.value }}</strong></div>
      </article>
    </div>
    <section class="analysis-chart-grid">
      <el-card shadow="never" class="analysis-card">
        <template #header><div class="panel-head"><span>用户访问来源</span><small>客户、渠道与推广来源</small></div></template>
        <div class="traffic-layout">
          <div class="traffic-legend">
            <div v-for="source in trafficSources" :key="source.label"><i :style="{ backgroundColor: source.color }"></i><span>{{ source.label }}</span></div>
          </div>
          <div class="donut-chart" :style="trafficDonutStyle"><span>来源</span></div>
        </div>
      </el-card>
      <el-card shadow="never" class="analysis-card">
        <template #header><div class="panel-head"><span>每周生成任务活跃量</span><small>任务提交与完成趋势</small></div></template>
        <div class="bar-chart">
          <div v-for="item in weeklyActivity" :key="item.day" class="bar-item"><span class="bar" :style="{ height: item.height + '%' }"></span><small>{{ item.day }}</small></div>
        </div>
      </el-card>
    </section>
    <el-card shadow="never" class="analysis-card analysis-line-card">
      <template #header><div class="panel-head"><span>每月销售额 / 积分消耗趋势</span><el-tag type="success">实时</el-tag></div></template>
      <svg class="trend-chart" viewBox="0 0 960 220" role="img" aria-label="每月销售额和积分消耗趋势">
        <g class="trend-grid"><line v-for="y in [40, 80, 120, 160, 200]" :key="y" x1="32" :y1="y" x2="928" :y2="y" /></g>
        <polyline class="trend-line trend-line-primary" points="32,154 112,136 192,94 272,108 352,150 432,124 512,86 592,112 672,70 752,92 832,58 928,116" />
        <polyline class="trend-line trend-line-success" points="32,112 112,150 192,142 272,78 352,70 432,92 512,84 592,36 672,108 752,174 832,132 928,118" />
      </svg>
    </el-card>
    <AdminExperienceInsights :module-labels="moduleLabels" />
  </section>

  <section v-else-if="activeModuleId === 'workbench'" class="workbench-page">
    <section class="workbench-hero">
      <div><el-tag type="success">API ONLINE</el-tag><h3>欢迎回来，{{ currentAdminName || '平台管理员' }}</h3><p>今日重点关注客户余额、上游模型连通性、待支付订单和渠道启停状态。</p></div>
      <div class="workbench-health"><span>平台健康度</span><strong>98.6%</strong><small>数据同步正常</small></div>
    </section>
    <section class="workbench-grid">
      <el-card shadow="never" class="analysis-card">
        <template #header><div class="panel-head"><span>快捷入口</span><small>高频运营动作</small></div></template>
        <div class="shortcut-grid"><button v-for="item in quickTodos" :key="item.action" type="button" @click="$emit('navigate', item.module)"><span>{{ item.title }}</span><small>{{ item.desc }}</small></button></div>
      </el-card>
      <el-card shadow="never" class="analysis-card">
        <template #header><div class="panel-head"><span>待办队列</span><el-tag type="warning">运营</el-tag></div></template>
        <div class="todo-list workbench-todos"><button v-for="task in workbenchTasks" :key="task.title" type="button" @click="$emit('navigate', task.module)"><span>{{ task.title }}</span><small>{{ task.desc }}</small></button></div>
      </el-card>
    </section>
  </section>

  <section v-else class="dashboard-grid">
    <el-card shadow="never" class="dashboard-card dashboard-card-large">
      <template #header><div class="panel-head"><span>经营概览</span><el-tag type="success">实时</el-tag></div></template>
      <div class="overview-board">
        <div v-for="metric in metrics.slice(0, 4)" :key="metric.label" class="overview-item"><span>{{ metric.label }}</span><strong>{{ metric.value }}</strong></div>
      </div>
    </el-card>
    <el-card shadow="never" class="dashboard-card">
      <template #header><div class="panel-head"><span>待办动作</span><el-tag type="warning">运营</el-tag></div></template>
      <div class="todo-list">
        <button v-for="item in quickTodos" :key="item.action" type="button" @click="$emit('navigate', item.module)"><span>{{ item.title }}</span><small>{{ item.desc }}</small></button>
      </div>
    </el-card>
  </section>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { Component, CSSProperties } from "vue";
import type { AdminExceptionCase } from "../../api/adminWorkspaces";
import AdminExceptionCenter from "./AdminExceptionCenter.vue";
import AdminExperienceInsights from "./AdminExperienceInsights.vue";

interface NavigationItem { action?: string; module: string; title: string; desc: string }
interface WorkItem { id: string; title: string; description: string; count: number; severity: string; module: string; roles?: string[] }
interface ExceptionWorkItem extends WorkItem { exceptionKey?: string; assigneeId?: string; assigneeName?: string; status?: AdminExceptionCase["status"]; slaDueAt?: string; firstDetectedAt?: string; updatedAt?: string; closedAt?: string; closeReason?: string; history?: AdminExceptionCase["history"] }

const props = defineProps<{
  activeModuleId: string;
  currentAdminName?: string;
  currentAdminRole?: string;
  analysisStats: Array<{ label: string; value: string; icon: Component }>;
  trafficSources: Array<{ label: string; color: string }>;
  trafficDonutStyle: CSSProperties;
  weeklyActivity: Array<{ day: string; height: number }>;
  metrics: Array<{ label: string; value: string }>;
  quickTodos: NavigationItem[];
  workbenchTasks: NavigationItem[];
  tasks?: WorkItem[];
  exceptions?: ExceptionWorkItem[];
  moduleLabels?: Record<string, string>;
}>();

defineEmits<{ navigate: [moduleId: string] }>();

const roleKey = computed(() => String(props.currentAdminRole || "SUPER_ADMIN").toUpperCase());
const roleWorkspace = computed(() => {
  const profiles: Record<string, { label: string; title: string; description: string; shortcuts: NavigationItem[] }> = {
    FINANCE: { label: "财务工作台", title: "资金与履约风险优先", description: "集中处理支付、权益发放、对账和渠道结算。", shortcuts: [{ module: "orders", title: "订单履约", desc: "支付与权益时间轴" }, { module: "billingReconciliation", title: "计费对账", desc: "定位异常计费任务" }, { module: "marketingSettlementStatements", title: "结算审核", desc: "处理渠道结算单" }] },
    AI_OPERATOR: { label: "AI 运营工作台", title: "模型质量与调用稳定性优先", description: "集中查看模型、通道、限制和失败调用。", shortcuts: [{ module: "aiCapabilityModels", title: "模型管理", desc: "模型状态与能力" }, { module: "aiCapabilityChannels", title: "上游通道", desc: "连通性与路由" }, { module: "aiCapabilityLogs", title: "调用日志", desc: "失败任务定位" }] },
    CUSTOMER_SERVICE: { label: "客户运营工作台", title: "客户问题与账号安全优先", description: "通过客户 360° 处理身份、套餐、订单与归属问题。", shortcuts: [{ module: "customers", title: "客户 360°", desc: "客户统一视图" }, { module: "orders", title: "订单履约", desc: "追踪支付与到账" }, { module: "customerAttributions", title: "归属关系", desc: "渠道归属健康度" }] },
    ENTERPRISE_ADMIN: { label: "企业运营工作台", title: "企业交付与风险优先", description: "集中处理认证、成员、套餐和企业集成。", shortcuts: [{ module: "enterpriseList", title: "企业中心", desc: "企业统一管理" }, { module: "enterpriseCertifications", title: "认证审核", desc: "处理待审企业" }, { module: "enterpriseIntegrations", title: "集成中心", desc: "Connector 运行状态" }] },
    SUPER_ADMIN: { label: "平台管理工作台", title: "全局经营与系统风险总览", description: "跨客户、商业化、AI 与渠道领域处理关键事项。", shortcuts: props.quickTodos.slice(0, 3) }
  };
  return profiles[roleKey.value] || profiles.SUPER_ADMIN;
});
function visibleForRole(item: WorkItem) { return roleKey.value === "SUPER_ADMIN" || !item.roles?.length || item.roles.includes(roleKey.value); }
const visibleTasks = computed(() => (props.tasks || []).filter(visibleForRole));
const visibleExceptions = computed<AdminExceptionCase[]>(() => (props.exceptions || []).filter(visibleForRole).map((item) => ({ ...item, exceptionKey: item.exceptionKey || item.id, status: item.status || "OPEN", firstDetectedAt: item.firstDetectedAt || item.updatedAt || "", updatedAt: item.updatedAt || item.firstDetectedAt || "" })));
</script>

<style scoped>
.role-workspace-hero { display: flex; align-items: center; justify-content: space-between; gap: 24px; padding: 22px; border: 1px solid var(--admin-border); border-radius: 12px; background: linear-gradient(135deg, var(--admin-panel), var(--color-primary-light)); }.role-workspace-hero h2 { margin: 10px 0 6px; color: var(--admin-text); font-size: 24px; }.role-workspace-hero p { margin: 0; color: var(--admin-muted); }.role-workspace-actions { display: grid; grid-template-columns: repeat(3, minmax(130px, 1fr)); gap: 10px; }.role-workspace-actions button { padding: 12px; border: 1px solid var(--admin-border); border-radius: 9px; background: var(--admin-panel); color: var(--admin-text); cursor: pointer; text-align: left; }.role-workspace-actions button:hover,.role-workspace-actions button:focus-visible { border-color: var(--color-border-active); outline: none; }.role-workspace-actions strong,.role-workspace-actions small { display: block; }.role-workspace-actions small { margin-top: 4px; color: var(--admin-muted); }
.operations-inbox { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }.operations-list { display: grid; gap: 8px; }.operations-list button { display: grid; grid-template-columns: 8px 1fr auto; align-items: center; gap: 10px; width: 100%; padding: 11px; border: 1px solid var(--admin-border); border-radius: 8px; background: transparent; cursor: pointer; text-align: left; }.operations-list button:hover,.operations-list button:focus-visible { border-color: var(--color-border-active); background: var(--color-primary-light); outline: none; }.operations-list i { width: 8px; height: 32px; border-radius: 8px; background: #94a3b8; }.operations-list i.is-warning { background: #f59e0b; }.operations-list i.is-danger { background: #ef4444; }.operations-list i.is-success { background: #22c55e; }.operations-list span { display: grid; gap: 3px; }.operations-list small { color: var(--admin-muted); }.operations-list em { min-width: 30px; color: var(--admin-text); font-size: 18px; font-style: normal; font-weight: 700; text-align: right; }
@media (max-width: 1050px) { .role-workspace-hero { align-items: stretch; flex-direction: column; }.operations-inbox { grid-template-columns: 1fr; } }
@media (max-width: 700px) { .role-workspace-actions { grid-template-columns: 1fr; } }
</style>
