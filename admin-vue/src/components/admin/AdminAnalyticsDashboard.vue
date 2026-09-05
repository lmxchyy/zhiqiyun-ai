<template>
  <section class="analytics-dashboard">
    <header class="dashboard-header">
      <div class="header-titles">
        <h2>{{ dashboardTitle }}</h2>
        <span class="header-sub">{{ dashboardSubTitle }}</span>
      </div>
      <div class="header-actions">
        <button type="button" class="btn-refresh" :disabled="loading" @click="refreshAll">
          {{ loading ? '刷新中...' : '刷新数据' }}
        </button>
      </div>
    </header>

    <div v-if="requestError" class="error-banner">
      部分统计数据加载遇到异常，已自动降级展示可用指标。
    </div>

    <!-- 1. Top 8 KPI Cards Grid -->
    <section class="kpi-section">
      <div class="kpi-section__heading">今日经营概览</div>
      <div class="kpi-grid">
      <!-- 1: New Users / Customers / Members -->
      <div class="kpi-card">
        <div class="kpi-card__title">{{ kpi1Title }}</div>
        <div class="kpi-card__val"><strong>{{ overviewData?.newUsersToday ?? 0 }}</strong><small>人</small></div>
        <div class="kpi-card__foot">昨日: {{ yesterdayNewUsers ?? '-' }}</div>
      </div>
      <!-- 2: DAU / Active Members -->
      <div class="kpi-card">
        <div class="kpi-card__title">{{ kpi2Title }}</div>
        <div class="kpi-card__val"><strong>{{ overviewData?.dau ?? 0 }}</strong><small>人</small></div>
        <div class="kpi-card__foot">昨日: {{ yesterdayDau ?? '-' }}</div>
      </div>
      <!-- 3: Total AI Tasks -->
      <div class="kpi-card">
        <div class="kpi-card__title">今日生图与视频生成量</div>
        <div class="kpi-card__val"><strong>{{ totalAITasksToday }}</strong><small>次</small></div>
        <div class="kpi-card__foot">生图 {{ overviewData?.imagesGenerated ?? 0 }} · 视频 {{ overviewData?.videosGenerated ?? 0 }}</div>
      </div>
      <!-- 4: Success Rate -->
      <div class="kpi-card">
        <div class="kpi-card__title">今日整体成功率</div>
        <div class="kpi-card__val"><strong>{{ (overviewData?.successRate ?? 0).toFixed(1) }}%</strong></div>
        <div class="kpi-card__foot">失败任务: {{ overviewData?.failedTasksToday ?? 0 }} 次</div>
      </div>
      <!-- 5: Points Consumed -->
      <div class="kpi-card">
        <div class="kpi-card__title">今日积分消耗</div>
        <div class="kpi-card__val"><strong>{{ overviewData?.pointsConsumed ?? 0 }}</strong><small>点</small></div>
        <div class="kpi-card__foot">昨日: {{ yesterdayPoints ?? '-' }}</div>
      </div>
      <!-- 6: Revenue / Available Points -->
      <div class="kpi-card">
        <div class="kpi-card__title">{{ kpi6Title }}</div>
        <div class="kpi-card__val">
          <strong v-if="capabilities.showRevenue">{{ (revenueYuan).toFixed(2) }}<small>元</small></strong>
          <strong v-else>{{ pointsData?.totalAvailable ?? 0 }}<small>点</small></strong>
        </div>
        <div class="kpi-card__foot">{{ kpi6Foot }}</div>
      </div>
      <!-- 7: Processing Tasks / Scope Tasks -->
      <div class="kpi-card">
        <div class="kpi-card__title">{{ kpi7Title }}</div>
        <div class="kpi-card__val"><strong>{{ overviewData?.processingTasks ?? 0 }}</strong><small>个</small></div>
        <div class="kpi-card__foot">含 PROCESSING/PENDING/QUEUED</div>
      </div>
      <!-- 8: Exception Cases / Frozen Points / Scope Node -->
      <div class="kpi-card" :class="{ 'is-alert': capabilities.canViewExceptionCenter && (overviewData?.exceptionCount ?? 0) > 0 }">
        <div class="kpi-card__title">{{ kpi8Title }}</div>
        <div class="kpi-card__val">
          <strong v-if="capabilities.canViewExceptionCenter" class="danger-text">{{ overviewData?.exceptionCount ?? 0 }}<small>条</small></strong>
          <strong v-else-if="scopeMode === 'TENANT'">{{ pointsData?.totalFrozen ?? 0 }}<small>点</small></strong>
          <strong v-else class="scope-badge">{{ scopeName }}</strong>
        </div>
        <div class="kpi-card__foot">{{ kpi8Foot }}</div>
      </div>
      </div>
    </section>

    <!-- Detail analysis controls: the range applies below the today KPI overview. -->
    <div class="analysis-toolbar">
      <!-- Tabs Container to keep 100% test contract compatibility -->
      <div class="tabs-container">
        <div class="tab" :class="{ active: activeTab === 'overview' }" @click="activeTab = 'overview'">运营驾驶舱</div>
        <div class="tab" :class="{ active: activeTab === 'trends' }" @click="activeTab = 'trends'">{{ trendTitle }}</div>
        <div class="tab" :class="{ active: activeTab === 'models' }" @click="activeTab = 'models'">模型排名</div>
        <div v-if="capabilities.canViewProviders" class="tab" :class="{ active: activeTab === 'providers' }" @click="activeTab = 'providers'">供应商状态</div>
        <div v-if="capabilities.canViewTokens" class="tab" :class="{ active: activeTab === 'tokens' }" @click="activeTab = 'tokens'">Token分析</div>
        <div class="tab" :class="{ active: activeTab === 'points' }" @click="activeTab = 'points'">积分分析</div>
      </div>
      <div class="analysis-toolbar__range">
        <span class="analysis-toolbar__label">分析周期</span>
        <div class="time-range-segmented">
          <button type="button" :class="{ active: selectedDays === 7 }" @click="selectDays(7)">最近 7 天</button>
          <button type="button" :class="{ active: selectedDays === 30 }" @click="selectDays(30)">最近 30 天</button>
        </div>
      </div>
    </div>

    <!-- Tab: Overview (Main Cockpit View) -->
    <div v-if="activeTab === 'overview'" class="cockpit-content">
      <!-- AI Types & Financial Summary -->
      <div class="split-row">
        <!-- AI Applications breakdown -->
        <div class="panel-box">
          <div class="panel-box__head">
            <strong>AI 应用类型分布</strong>
            <small>今日调用量与占比</small>
          </div>
          <div class="type-list" v-if="generationData?.byType?.length">
            <div v-for="t in generationData.byType" :key="t.type" class="type-item">
              <div class="type-item__label">
                <span>{{ formatTypeName(t.type) }}</span>
                <span>{{ t.count }} 次 ({{ (t.rate || 0).toFixed(1) }}%)</span>
              </div>
              <div class="progress-track"><div class="progress-bar" :style="{ width: Math.min(100, Math.round(t.rate || 0)) + '%' }"></div></div>
            </div>
          </div>
          <div v-else class="empty-state">今日暂无生成任务分类数据</div>
        </div>

        <!-- Points & Revenue Summary -->
        <div class="panel-box">
          <div class="panel-box__head">
            <strong>财务与积分水位</strong>
            <small>可用与冻结资产</small>
          </div>
          <div class="points-grid">
            <div class="point-col" v-if="capabilities.showRevenue">
              <label>今日充值总额</label>
              <strong>{{ ((pointsData?.rechargedToday ?? 0) / 100).toFixed(2) }} 元</strong>
            </div>
            <div class="point-col">
              <label>今日积分消耗</label>
              <strong>{{ pointsData?.consumedToday ?? 0 }} 点</strong>
            </div>
            <div class="point-col">
              <label>{{ scopeMode === 'TENANT' ? '企业可用积分' : '全平台可用余额' }}</label>
              <strong>{{ pointsData?.totalAvailable ?? 0 }} 点</strong>
            </div>
            <div class="point-col">
              <label>{{ scopeMode === 'TENANT' ? '企业冻结积分' : '全平台冻结余额' }}</label>
              <strong>{{ pointsData?.totalFrozen ?? 0 }} 点</strong>
            </div>
          </div>
        </div>
      </div>

      <!-- Model & Provider Status Tables -->
      <div class="split-row">
        <!-- Models -->
        <div class="panel-box" :class="{ 'full-width': !capabilities.canViewProviders }">
          <div class="panel-box__head">
            <strong>模型调用排行 (Top)</strong>
            <small>按调用量排序</small>
          </div>
          <table class="analytics-table" v-if="modelsData && modelsData.length > 0">
            <thead>
              <tr>
                <th>排名</th>
                <th>模型</th>
                <th>调用次数</th>
                <th>成功率</th>
                <th>平均延迟</th>
                <th v-if="capabilities.canViewProviderCost">总成本</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(model, index) in modelsData" :key="index">
                <td>{{ index + 1 }}</td>
                <td>{{ model.modelCode }}</td>
                <td>{{ model.callCount }}</td>
                <td>{{ (model.successRate || 0).toFixed(1) }}%</td>
                <td>{{ model.avgLatencyMs || 0 }}ms</td>
                <td v-if="capabilities.canViewProviderCost">{{ (model.totalCostCents || 0) }}分</td>
              </tr>
            </tbody>
          </table>
          <div v-else class="empty-state">暂无模型调用数据</div>
        </div>

        <!-- Providers (Platform only) -->
        <div class="panel-box" v-if="capabilities.canViewProviders">
          <div class="panel-box__head">
            <strong>供应商状态</strong>
            <small>上游可用性监控</small>
          </div>
          <table class="analytics-table" v-if="providersData && providersData.length > 0">
            <thead>
              <tr>
                <th>排名</th>
                <th>供应商</th>
                <th>调用次数</th>
                <th>成功率</th>
                <th>平均延迟</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(provider, index) in providersData" :key="index">
                <td>{{ index + 1 }}</td>
                <td>{{ provider.providerCode }}</td>
                <td>{{ provider.callCount }}</td>
                <td>{{ (provider.successRate || 0).toFixed(1) }}%</td>
                <td>{{ provider.avgLatencyMs || 0 }}ms</td>
              </tr>
            </tbody>
          </table>
          <div v-else class="empty-state">暂无供应商数据</div>
        </div>
      </div>

      <!-- Runtime Operational Panel (Platform only) -->
      <div class="panel-box runtime-box" v-if="capabilities.canViewRuntimeMetrics">
        <div class="panel-box__head">
          <strong>任务运行态透视</strong>
          <small>数据库排队/处理中任务与工单统计（仅平台管理员可见）</small>
        </div>
        <div class="runtime-items">
          <div class="runtime-item">
            <span>排队/处理中任务</span>
            <strong>{{ overviewData?.processingTasks ?? 0 }} 个</strong>
          </div>
          <div class="runtime-item">
            <span>异常处置工单</span>
            <strong :class="{ 'danger-text': (overviewData?.exceptionCount ?? 0) > 0 }">{{ overviewData?.exceptionCount ?? 0 }} 条</strong>
          </div>
          <div class="runtime-item">
            <span>今日失败任务数</span>
            <strong>{{ overviewData?.failedTasksToday ?? 0 }} 次</strong>
          </div>
          <div class="runtime-item">
            <span>今日平均延迟 (AVG)</span>
            <strong>{{ overviewData?.avgLatencyMs ?? 0 }} ms</strong>
          </div>
        </div>
      </div>

      <!-- Exception Cases Integration (Platform only) -->
      <div class="panel-box exception-box" v-if="capabilities.canViewExceptionCenter && exceptionCases.length > 0">
        <AdminExceptionCenter :items="exceptionCases" @navigate="handleExceptionNavigate" @updated="handleExceptionUpdated" />
      </div>
    </div>

    <!-- Tab: Trends -->
    <div v-else-if="activeTab === 'trends'" class="tab-content-panel">
      <div class="section-title">{{ trendTitle }}</div>
      <div class="charts-grid" v-if="trendsData && hasTrendData()">
        <div class="chart-placeholder" v-for="key in chartKeys" :key="key">
          <div class="chart-title">{{ chartLabels[key] }}</div>
          <div class="chart-content" :ref="(element) => setChartElement(element, key)"></div>
        </div>
      </div>
      <div class="empty-state" v-else-if="!trendsLoading">暂无趋势数据</div>
      <div class="loading-placeholder" v-else>
        <div class="skeleton-loader" v-for="i in 5" :key="i"></div>
      </div>
    </div>

    <!-- Tab: Models -->
    <div v-else-if="activeTab === 'models'" class="tab-content-panel">
      <div class="section-title">模型排名</div>
      <div class="table-container" v-if="modelsData && modelsData.length > 0">
        <table class="analytics-table">
          <thead>
            <tr>
              <th>排名</th>
              <th>模型</th>
              <th>调用次数</th>
              <th>成功率</th>
              <th>平均延迟(ms)</th>
              <th v-if="capabilities.canViewProviderCost">总成本(分)</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(model, index) in modelsData" :key="index">
              <td>{{ index + 1 }}</td>
              <td>{{ model.modelCode }}</td>
              <td>{{ model.callCount }}</td>
              <td>{{ (model.successRate || 0).toFixed(1) }}%</td>
              <td>{{ model.avgLatencyMs || 0 }}</td>
              <td v-if="capabilities.canViewProviderCost">{{ model.totalCostCents || 0 }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else class="empty-state">暂无模型数据</div>
    </div>

    <!-- Tab: Providers (Platform only) -->
    <div v-else-if="activeTab === 'providers' && capabilities.canViewProviders" class="tab-content-panel">
      <div class="section-title">供应商状态</div>
      <div class="table-container" v-if="providersData && providersData.length > 0">
        <table class="analytics-table">
          <thead>
            <tr>
              <th>排名</th>
              <th>供应商</th>
              <th>调用次数</th>
              <th>成功率</th>
              <th>平均延迟(ms)</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(provider, index) in providersData" :key="index">
              <td>{{ index + 1 }}</td>
              <td>{{ provider.providerCode }}</td>
              <td>{{ provider.callCount }}</td>
              <td>{{ (provider.successRate || 0).toFixed(1) }}%</td>
              <td>{{ provider.avgLatencyMs || 0 }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else class="empty-state">暂无供应商数据</div>
    </div>

    <!-- Tab: Tokens (Platform only) -->
    <div v-else-if="activeTab === 'tokens' && capabilities.canViewTokens" class="tab-content-panel">
      <div class="section-title">Token分析</div>
      <div v-if="tokensData">
        <div class="metrics-row">
          <div class="metric-card">
            <h4>今日使用量</h4>
            <p>{{ tokensData.tokensToday }}</p>
          </div>
          <div class="metric-card">
            <h4>7日使用量</h4>
            <p>{{ tokensData.tokens7d }}</p>
          </div>
          <div class="metric-card">
            <h4>30日使用量</h4>
            <p>{{ tokensData.tokens30d }}</p>
          </div>
        </div>
      </div>
      <div v-else class="empty-state">暂无 Token 数据</div>
    </div>

    <!-- Tab: Points -->
    <div v-else-if="activeTab === 'points'" class="tab-content-panel">
      <div class="section-title">积分分析</div>
      <div v-if="pointsData">
        <div class="metrics-row">
          <div class="metric-card">
            <h4>今日消耗</h4>
            <p>{{ pointsData.consumedToday }}</p>
          </div>
          <div class="metric-card" v-if="capabilities.showRevenue">
            <h4>今日充值</h4>
            <p>{{ pointsData.rechargedToday }}</p>
          </div>
          <div class="metric-card" v-if="capabilities.showRevenue">
            <h4>今日净变化</h4>
            <p>{{ pointsData.netChangeToday }}</p>
          </div>
          <div class="metric-card">
            <h4>可用余额</h4>
            <p>{{ pointsData.totalAvailable }}</p>
          </div>
        </div>
      </div>
      <div v-else class="empty-state">暂无积分数据</div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import * as echarts from "echarts/core";
import { LineChart } from "echarts/charts";
import { GridComponent, LegendComponent, TooltipComponent } from "echarts/components";
import { SVGRenderer } from "echarts/renderers";
import { adminRequest } from "../../api/client";
import { normalizeAnalyticsModelsResponse, normalizeAnalyticsProvidersResponse } from "./analyticsContract";
import AdminExceptionCenter from "./AdminExceptionCenter.vue";
import type { AdminExceptionCase } from "../../api/adminWorkspaces";

echarts.use([LineChart, GridComponent, LegendComponent, TooltipComponent, SVGRenderer]);

export type DashboardScopeMode = "PLATFORM" | "OPERATION_CENTER" | "AGENT" | "TENANT";

export interface DashboardCapabilities {
  canViewPlatformRevenue: boolean;
  canViewProviderCost: boolean;
  canViewRuntimeMetrics: boolean;
  canViewExceptionCenter: boolean;
  canViewTokens: boolean;
  canViewProviders: boolean;
  showRevenue: boolean;
  showCustomerRanking: boolean;
  showMemberRanking: boolean;
}

export interface AnalyticsScopeInfo {
  level: DashboardScopeMode;
  scopeName: string;
  capabilities: DashboardCapabilities;
}

interface OverviewData {
  newUsersToday: number;
  dau: number;
  wau: number;
  mau: number;
  aiUsersToday: number;
  imagesGenerated: number;
  videosGenerated: number;
  pointsConsumed: number;
  tokensUsed: number;
  revenueTodayCents: number;
  costTodayCents: number;
  failedTasksToday: number;
  processingTasks?: number;
  exceptionCount?: number;
  successRate: number;
  avgLatencyMs: number;
}

interface TrendData {
  newUsers: Array<{ date: string; value: number }>;
  dau: Array<{ date: string; value: number }>;
  aiUsers: Array<{ date: string; value: number }>;
  points: Array<{ date: string; value: number }>;
  tokens: Array<{ date: string; value: number }>;
  revenue?: Array<{ date: string; value: number }>;
  images?: Array<{ date: string; value: number }>;
  videos?: Array<{ date: string; value: number }>;
  success?: Array<{ date: string; value: number }>;
}

interface GenerationData {
  imagesToday: number;
  videosToday: number;
  totalTasksToday: number;
  successRate: number;
  avgLatencyMs: number;
  failedTasks: number;
  byType?: Array<{ type: string; count: number; rate: number }>;
}

interface ModelData {
  modelCode: string;
  callCount: number;
  successCount: number;
  successRate: number;
  avgLatencyMs: number;
  totalCostCents: number;
}

interface ProviderData {
  providerCode: string;
  callCount: number;
  successCount: number;
  successRate: number;
  avgLatencyMs: number;
  totalCostCents: number;
}

interface TokenData {
  tokensToday: number;
  tokens7d: number;
  tokens30d: number;
  byUser: Array<{ userId: string; userName: string; value: number }>;
}

interface PointsData {
  consumedToday: number;
  rechargedToday: number;
  grantedToday?: number;
  frozenToday?: number;
  releasedToday?: number;
  netChangeToday?: number;
  totalAvailable: number;
  totalFrozen: number;
  consumedTrend?: Array<{ date: string; value: number }>;
  rechargedTrend?: Array<{ date: string; value: number }>;
  byType?: Array<{ type: string; count: number; rate: number }>;
}

const scopeMode = ref<DashboardScopeMode>("PLATFORM");
const scopeName = ref<string>("全平台");
const capabilities = ref<DashboardCapabilities>({
  canViewPlatformRevenue: true,
  canViewProviderCost: true,
  canViewRuntimeMetrics: true,
  canViewExceptionCenter: true,
  canViewTokens: true,
  canViewProviders: true,
  showRevenue: true,
  showCustomerRanking: true,
  showMemberRanking: false,
});

const selectedDays = ref<number>(7);
const trendTitle = computed(() => `近 ${selectedDays.value} 日趋势分析`);
const loading = ref(false);
const requestError = ref(false);
const activeTab = ref<"overview" | "trends" | "models" | "providers" | "tokens" | "points">("overview");

const overviewLoading = ref(true);
const trendsLoading = ref(true);
const generationLoading = ref(true);
const modelsLoading = ref(true);
const providersLoading = ref(true);
const tokensLoading = ref(true);
const pointsLoading = ref(true);

const overviewData = ref<OverviewData | null>(null);
const trendsData = ref<TrendData | null>(null);
const generationData = ref<GenerationData | null>(null);
const modelsData = ref<ModelData[] | null>(null);
const providersData = ref<ProviderData[] | null>(null);
const tokensData = ref<TokenData | null>(null);
const pointsData = ref<PointsData | null>(null);
const exceptionCases = ref<AdminExceptionCase[]>([]);

const dashboardTitle = computed(() => {
  switch (scopeMode.value) {
    case "OPERATION_CENTER":
      return "运营中心驾驶舱";
    case "AGENT":
      return "代理经营驾驶舱";
    case "TENANT":
      return "企业使用驾驶舱";
    default:
      return "平台运营驾驶舱";
  }
});

const dashboardSubTitle = computed(() => {
  switch (scopeMode.value) {
    case "OPERATION_CENTER":
      return `区域客户拓客与服务运行看板 · ${scopeName.value}`;
    case "AGENT":
      return `直属客户与生成业务经营看板 · ${scopeName.value}`;
    case "TENANT":
      return `企业成员与 AI 资产消耗看板 · ${scopeName.value}`;
    default:
      return "全平台经营与系统运行状态看板";
  }
});

const kpi1Title = computed(() => {
  switch (scopeMode.value) {
    case "OPERATION_CENTER":
      return "今日新增关联用户";
    case "AGENT":
      return "今日新增直属客户";
    case "TENANT":
      return "企业成员数";
    default:
      return "今日新增用户";
  }
});

const kpi2Title = computed(() => {
  return scopeMode.value === "TENANT" ? "今日活跃成员" : "今日活跃用户 (DAU)";
});

const kpi6Title = computed(() => {
  if (capabilities.value.showRevenue) {
    return scopeMode.value === "PLATFORM" ? "今日充值收入" : "今日客户充值贡献";
  }
  return "企业可用积分";
});

const kpi6Foot = computed(() => {
  if (capabilities.value.showRevenue) {
    return yesterdayRevenueYuan.value !== undefined ? `昨日: ${yesterdayRevenueYuan.value.toFixed(2)} 元` : "-";
  }
  return "企业资产配额";
});

const kpi7Title = computed(() => {
  return scopeMode.value === "AGENT" ? "直属客户生成任务" : "当前排队/处理中任务";
});

const kpi8Title = computed(() => {
  if (capabilities.value.canViewExceptionCenter) {
    return "当前异常风险任务";
  }
  if (scopeMode.value === "TENANT") {
    return "企业冻结积分";
  }
  return scopeMode.value === "OPERATION_CENTER" ? "区域服务中心" : "渠道代理身份";
});

const kpi8Foot = computed(() => {
  if (capabilities.value.canViewExceptionCenter) {
    return "待处置/处理中工单";
  }
  if (scopeMode.value === "TENANT") {
    return "已锁定等待结算点数";
  }
  return "有效授权业务节点";
});

const totalAITasksToday = computed(() => {
  return (overviewData.value?.imagesGenerated ?? 0) + (overviewData.value?.videosGenerated ?? 0);
});

const revenueYuan = computed(() => {
  return (overviewData.value?.revenueTodayCents ?? 0) / 100;
});

function getYesterdayValue(series?: Array<{ date: string; value: number }>): number | undefined {
  if (!series || series.length < 2) return undefined;
  return series[series.length - 2].value;
}

const yesterdayNewUsers = computed(() => getYesterdayValue(trendsData.value?.newUsers));
const yesterdayDau = computed(() => getYesterdayValue(trendsData.value?.dau));
const yesterdayPoints = computed(() => getYesterdayValue(trendsData.value?.points));
const yesterdayRevenueYuan = computed(() => {
  const cents = getYesterdayValue(trendsData.value?.revenue);
  return cents !== undefined ? cents / 100 : undefined;
});

type TrendKey = "newUsers" | "dau" | "aiUsers" | "points" | "tokens";
const chartKeys: TrendKey[] = ["newUsers", "dau", "aiUsers", "points", "tokens"];
const chartLabels: Record<TrendKey, string> = {
  newUsers: "新增用户趋势",
  dau: "DAU 趋势",
  aiUsers: "AI 用户趋势",
  points: "积分消耗趋势",
  tokens: "Token使用趋势",
};

const chartElements = new Map<TrendKey, HTMLElement>();
const chartInstances = new Map<TrendKey, echarts.ECharts>();

function setChartElement(element: unknown, key: TrendKey) {
  if (element instanceof HTMLElement) chartElements.set(key, element);
}

function renderCharts() {
  if (!trendsData.value || activeTab.value !== "trends") return;
  for (const key of chartKeys) {
    const element = chartElements.get(key);
    if (!element) continue;
    const chart = chartInstances.get(key) || echarts.init(element, undefined, { renderer: "svg" });
    chartInstances.set(key, chart);
    const points = trendsData.value[key] || [];
    chart.setOption({
      animation: false,
      tooltip: { trigger: "axis" },
      grid: { left: 36, right: 18, top: 18, bottom: 28 },
      xAxis: { type: "category", data: points.map((point) => point.date) },
      yAxis: { type: "value" },
      series: [{ type: "line", smooth: true, data: points.map((point) => point.value), areaStyle: {} }],
    });
  }
}

function hasTrendData() {
  return Boolean(trendsData.value && chartKeys.some((key) => trendsData.value?.[key]?.length));
}

function resizeCharts() {
  chartInstances.forEach((chart) => chart.resize());
}

function disposeCharts() {
  chartInstances.forEach((chart) => chart.dispose());
  chartInstances.clear();
  chartElements.clear();
}

function formatTypeName(raw: string): string {
  const map: Record<string, string> = {
    TEXT_TO_IMAGE: "文生图",
    IMAGE_TO_IMAGE: "图生图",
    TEXT_TO_VIDEO: "文生视频",
    IMAGE_TO_VIDEO: "图生视频",
    PPT: "AI 演示文稿 (PPT)",
    AGENT: "AI 智能体",
    KNOWLEDGE: "知识库问答",
  };
  return map[raw] || raw;
}

async function fetchScope() {
  try {
    const res = await adminRequest<AnalyticsScopeInfo>({
      method: "GET",
      url: "/admin/analytics/scope",
    });
    if (res && res.level) {
      scopeMode.value = res.level;
      scopeName.value = res.scopeName || "";
      if (res.capabilities) {
        capabilities.value = res.capabilities;
      }
    }
  } catch {
    // If scope endpoint is unavailable in mock tests, retain default platform scope
  }
}

async function fetchOverview() {
  try {
    overviewLoading.value = true;
    overviewData.value = await adminRequest<OverviewData>({
      method: "GET",
      url: "/admin/analytics/overview",
    });
  } catch {
    requestError.value = true;
  } finally {
    overviewLoading.value = false;
  }
}

async function fetchTrends() {
  try {
    trendsLoading.value = true;
    trendsData.value = await adminRequest<TrendData>({
      method: "GET",
      url: `/admin/analytics/trends?days=${selectedDays.value}`,
    });
  } catch {
    requestError.value = true;
  } finally {
    trendsLoading.value = false;
  }
}

async function fetchGeneration() {
  try {
    generationLoading.value = true;
    generationData.value = await adminRequest<GenerationData>({
      method: "GET",
      url: `/admin/analytics/generation?days=${selectedDays.value}`,
    });
  } catch {
    requestError.value = true;
  } finally {
    generationLoading.value = false;
  }
}

async function fetchModels() {
  try {
    modelsLoading.value = true;
    const res = await adminRequest<{ models: ModelData[] }>({
      method: "GET",
      url: `/admin/analytics/models?days=${selectedDays.value}`,
    });
    modelsData.value = normalizeAnalyticsModelsResponse(res);
  } catch {
    requestError.value = true;
  } finally {
    modelsLoading.value = false;
  }
}

async function fetchProviders() {
  if (!capabilities.value.canViewProviders) {
    providersData.value = [];
    providersLoading.value = false;
    return;
  }
  try {
    providersLoading.value = true;
    const res = await adminRequest<{ providers: ProviderData[] }>({
      method: "GET",
      url: `/admin/analytics/providers?days=${selectedDays.value}`,
    });
    providersData.value = normalizeAnalyticsProvidersResponse(res);
  } catch {
    requestError.value = true;
  } finally {
    providersLoading.value = false;
  }
}

async function fetchTokens() {
  if (!capabilities.value.canViewTokens) {
    tokensData.value = null;
    tokensLoading.value = false;
    return;
  }
  try {
    tokensLoading.value = true;
    tokensData.value = await adminRequest<TokenData>({
      method: "GET",
      url: "/admin/analytics/tokens",
    });
  } catch {
    requestError.value = true;
  } finally {
    tokensLoading.value = false;
  }
}

async function fetchPoints() {
  try {
    pointsLoading.value = true;
    pointsData.value = await adminRequest<PointsData>({
      method: "GET",
      url: `/admin/analytics/points?days=${selectedDays.value}`,
    });
  } catch {
    requestError.value = true;
  } finally {
    pointsLoading.value = false;
  }
}

async function fetchExceptions() {
  if (!capabilities.value.canViewExceptionCenter) {
    exceptionCases.value = [];
    return;
  }
  try {
    const res = await adminRequest<{ adminExceptionCases?: AdminExceptionCase[] }>({
      method: "GET",
      url: "/admin/overview",
    });
    exceptionCases.value = res.adminExceptionCases || [];
  } catch {
    // Non-critical background fetch
  }
}

async function refreshAll() {
  loading.value = true;
  requestError.value = false;
  await fetchScope();
  await Promise.allSettled([
    fetchOverview(),
    fetchTrends(),
    fetchGeneration(),
    fetchModels(),
    fetchProviders(),
    fetchTokens(),
    fetchPoints(),
    fetchExceptions(),
  ]);
  loading.value = false;
  await nextTick();
  renderCharts();
}

function selectDays(days: number) {
  selectedDays.value = days;
  refreshAll();
}

function handleExceptionNavigate(_moduleId: string) {}
function handleExceptionUpdated(updatedCase: AdminExceptionCase) {
  const idx = exceptionCases.value.findIndex((c) => c.id === updatedCase.id);
  if (idx >= 0) exceptionCases.value[idx] = updatedCase;
}

onMounted(() => {
  refreshAll();
  window.addEventListener("resize", resizeCharts);
});

watch([activeTab, trendsData], async () => {
  if (activeTab.value === "trends") {
    await nextTick();
    renderCharts();
  } else {
    disposeCharts();
  }
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", resizeCharts);
  disposeCharts();
});
</script>

<style scoped>
.analytics-dashboard {
  padding: 20px;
  background: var(--admin-panel-color, #f4f6f9);
  border-radius: 8px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.dashboard-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}

.header-titles h2 {
  margin: 0 0 4px 0;
  font-size: 20px;
  font-weight: 700;
  color: var(--admin-text-color, #1e293b);
}

.header-sub {
  font-size: 13px;
  color: var(--admin-text-color-light, #64748b);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.time-range-segmented {
  display: inline-flex;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  overflow: hidden;
}

.time-range-segmented button {
  padding: 6px 12px;
  font-size: 12px;
  border: none;
  background: #ffffff;
  cursor: pointer;
  color: #64748b;
}

.time-range-segmented button.active {
  background: #0284c7;
  color: #ffffff;
  font-weight: 600;
}

.btn-refresh {
  padding: 6px 14px;
  font-size: 12px;
  border-radius: 6px;
  border: none;
  background: #0284c7;
  color: #ffffff;
  cursor: pointer;
}

.error-banner {
  padding: 8px 12px;
  background: #fffbeb;
  border: 1px solid #fef3c7;
  border-radius: 6px;
  font-size: 13px;
  color: #b45309;
}

/* 8 KPI Cards Grid */
.kpi-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.kpi-section__heading {
  font-size: 14px;
  font-weight: 600;
  color: var(--admin-text-color, #1e293b);
}

.kpi-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 12px;
}

.kpi-card {
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
}

.kpi-card.is-alert {
  border-color: #fecaca;
  background: #fff5f5;
}

.kpi-card__title {
  font-size: 12px;
  color: #64748b;
  margin-bottom: 6px;
}

.kpi-card__val {
  font-size: 22px;
  font-weight: 700;
  color: #0f172a;
  display: flex;
  align-items: baseline;
  gap: 4px;
  line-height: 1.1;
}

.kpi-card__val small {
  font-size: 12px;
  font-weight: normal;
  color: #64748b;
}

.kpi-card__foot {
  font-size: 11px;
  color: #94a3b8;
  margin-top: 8px;
  border-top: 1px dashed #f1f5f9;
  padding-top: 6px;
}

.danger-text {
  color: #ef4444;
}

.scope-badge {
  font-size: 14px;
  color: #0284c7;
  font-weight: 600;
}

/* Detail analysis tabs and range */
.analysis-toolbar {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  margin-top: 8px;
}

.analysis-toolbar__range {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-bottom: 7px;
}

.analysis-toolbar__label {
  font-size: 12px;
  color: #64748b;
  white-space: nowrap;
}

.tabs-container {
  display: flex;
  flex: 1;
  border-bottom: 1px solid #e2e8f0;
}

.tab {
  padding: 10px 16px;
  cursor: pointer;
  color: #64748b;
  font-size: 14px;
  border-bottom: 2px solid transparent;
}

.tab.active {
  color: #0284c7;
  border-bottom-color: #0284c7;
  font-weight: 600;
}

/* Cockpit View Layout */
.cockpit-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.split-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

@media (max-width: 900px) {
  .analysis-toolbar {
    align-items: stretch;
    flex-direction: column;
    gap: 8px;
  }

  .analysis-toolbar__range {
    justify-content: flex-end;
    padding-bottom: 0;
  }

  .split-row {
    grid-template-columns: 1fr;
  }
}

.panel-box {
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 16px;
}

.panel-box.full-width {
  grid-column: 1 / -1;
}

.panel-box__head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.panel-box__head strong {
  font-size: 15px;
  color: #0f172a;
}

.panel-box__head small {
  font-size: 12px;
  color: #64748b;
}

/* AI Type List */
.type-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.type-item__label {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  color: #334155;
  margin-bottom: 4px;
}

.progress-track {
  width: 100%;
  height: 6px;
  background: #f1f5f9;
  border-radius: 3px;
  overflow: hidden;
}

.progress-bar {
  height: 100%;
  background: #0284c7;
  border-radius: 3px;
}

/* Points Summary */
.points-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.point-col {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px;
  background: #f8fafc;
  border-radius: 6px;
}

.point-col label {
  font-size: 12px;
  color: #64748b;
}

.point-col strong {
  font-size: 18px;
  color: #0f172a;
}

/* Tables */
.analytics-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

.analytics-table th,
.analytics-table td {
  padding: 8px 12px;
  text-align: left;
  border-bottom: 1px solid #f1f5f9;
}

.analytics-table th {
  background-color: #f8fafc;
  color: #475569;
  font-weight: 600;
}

/* Runtime Items */
.runtime-items {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
}

.runtime-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px;
  background: #f8fafc;
  border-radius: 6px;
}

.runtime-item span {
  font-size: 12px;
  color: #64748b;
}

.runtime-item strong {
  font-size: 18px;
  color: #0f172a;
}

.empty-state {
  text-align: center;
  padding: 24px 0;
  color: #94a3b8;
  font-size: 13px;
}

/* Tab Panels */
.tab-content-panel {
  min-height: 280px;
}

.section-title {
  font-size: 16px;
  font-weight: 600;
  color: #0f172a;
  margin-bottom: 12px;
}

.charts-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 16px;
}

.chart-placeholder {
  background: #ffffff;
  border-radius: 8px;
  padding: 14px;
  border: 1px solid #e2e8f0;
}

.chart-title {
  font-size: 13px;
  font-weight: 600;
  color: #475569;
  margin-bottom: 8px;
}

.chart-content {
  height: 140px;
  width: 100%;
}

.metrics-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 16px;
}

.metric-card {
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 16px;
  text-align: center;
}

.metric-card h4 {
  margin: 0 0 6px 0;
  font-size: 13px;
  color: #64748b;
}

.metric-card p {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  color: #0f172a;
}
</style>
