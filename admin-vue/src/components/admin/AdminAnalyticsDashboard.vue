<template>
  <section class="analytics-dashboard">
    <!-- Overview Cards -->
    <div class="overview-cards">
      <div class="overview-card" v-if="overviewData">
        <h3>今日概览</h3>
        <div class="metrics-grid">
          <div class="metric-item">
            <label>日新增用户</label>
          <value>{{ overviewData.newUsersToday }}</value>
          </div>
          <div class="metric-item">
            <label>日活跃用户 (dau)</label>
          <value>{{ overviewData.dau }}</value>
          </div>
          <div class="metric-item">
            <label>周活跃用户 (wau)</label>
          <value>{{ overviewData.wau }}</value>
          </div>
          <div class="metric-item">
            <label>月活跃用户 (mau)</label>
          <value>{{ overviewData.mau }}</value>
          </div>
          <div class="metric-item">
            <label>今日AI用户</label>
          <value>{{ overviewData.aiUsersToday }}</value>
          </div>
          <div class="metric-item">
            <label>今日图片生成</label>
          <value>{{ overviewData.imagesGenerated }}</value>
          </div>
          <div class="metric-item">
            <label>今日视频生成</label>
          <value>{{ overviewData.videosGenerated }}</value>
          </div>
          <div class="metric-item">
            <label>今日积分消耗</label>
          <value>{{ overviewData.pointsConsumed }}</value>
          </div>
          <div class="metric-item">
            <label>今日Token使用</label>
          <value>{{ overviewData.tokensUsed }}</value>
          </div>
          <div class="metric-item">
            <label>今日收入</label>
          <value>{{ overviewData.revenueTodayCents / 100 }}</value>
            <unit>元</unit>
          </div>
          <div class="metric-item">
            <label>今日成本</label>
          <value>{{ overviewData.costTodayCents / 100 }}</value>
            <unit>元</unit>
          </div>
          <div class="metric-item">
            <label>今日失败任务</label>
          <value>{{ overviewData.failedTasksToday }}</value>
          </div>
          <div class="metric-item">
            <label>今日成功率</label>
          <value>{{ overviewData.successRate.toFixed(1) }}%</value>
          </div>
          <div class="metric-item">
            <label>今日平均延迟</label>
          <value>{{ overviewData.avgLatencyMs }}ms</value>
          </div>
        </div>
      </div>
    </div>

    <!-- Tabs for different sections -->
    <div class="error-state" v-if="requestError">分析数据加载失败，请稍后重试。</div>
    <div class="tabs-container" v-if="!loading && !requestError">
      <div class="tab" :class="{ active: activeTab === 'overview' }" @click="activeTab = 'overview'">概览</div>
      <div class="tab" :class="{ active: activeTab === 'trends' }" @click="activeTab = 'trends'">7日趋势</div>
      <div class="tab" :class="{ active: activeTab === 'models' }" @click="activeTab = 'models'">模型排名</div>
      <div class="tab" :class="{ active: activeTab === 'providers' }" @click="activeTab = 'providers'">供应商状态</div>
      <div class="tab" :class="{ active: activeTab === 'tokens' }" @click="activeTab = 'tokens'">Token分析</div>
      <div class="tab" :class="{ active: activeTab === 'points' }" @click="activeTab = 'points'">积分分析</div>
    </div>

    <!-- Tab Content -->
    <div class="tab-content">
      <div v-if="activeTab === 'overview'">
        <!-- Overview already shown above -->
      </div>

      <div v-else-if="activeTab === 'trends'">
        <div class="section-title">7日趋势</div>
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

      <div v-else-if="activeTab === 'models'">
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
                <th>总成本(分)</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(model, index) in modelsData" :key="index">
                <td>{{ index + 1 }}</td>
                <td>{{ model.modelCode }}</td>
                <td>{{ model.callCount }}</td>
                <td>{{ (model.successRate || 0).toFixed(1) }}%</td>
                <td>{{ model.avgLatencyMs || 0 }}</td>
                <td>{{ model.totalCostCents || 0 }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else-if="modelsData && modelsData.length === 0">
          <p class="empty-state">暂无模型数据</p>
        </div>
        <div class="loading-placeholder" v-else>
          <div class="skeleton-loader" v-for="i in 4" :key="i"></div>
        </div>
      </div>

      <div v-else-if="activeTab === 'providers'">
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
                <th>总成本(分)</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(provider, index) in providersData" :key="index">
                <td>{{ index + 1 }}</td>
                <td>{{ provider.providerCode }}</td>
                <td>{{ provider.callCount }}</td>
                <td>{{ (provider.successRate || 0).toFixed(1) }}%</td>
                <td>{{ provider.avgLatencyMs || 0 }}</td>
                <td>{{ provider.totalCostCents || 0 }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else-if="providersData && providersData.length === 0">
          <p class="empty-state">暂无供应商数据</p>
        </div>
        <div class="loading-placeholder" v-else>
          <div class="skeleton-loader" v-for="i in 4" :key="i"></div>
        </div>
      </div>

      <div v-else-if="activeTab === 'tokens'">
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
          <div class="table-container">
            <h4>Top 用户 Token 使用量</h4>
            <table class="analytics-table" v-if="tokensData.byUser && tokensData.byUser.length > 0">
              <thead>
                <tr>
                  <th>排名</th>
                  <th>用户ID</th>
                  <th>用户名</th>
                  <th>Token使用量</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(user, index) in tokensData.byUser" :key="index">
                  <td>{{ index + 1 }}</td>
                  <td>{{ user.userId }}</td>
                  <td>{{ user.userName }}</td>
                  <td>{{ user.value }}</td>
                </tr>
              </tbody>
            </table>
            <div v-else-if="tokensData.byUser && tokensData.byUser.length === 0">
              <p class="empty-state">暂无用户数据</p>
            </div>
          </div>
        </div>
        <div class="loading-placeholder" v-else>
          <div class="skeleton-loader" v-for="i in 3" :key="i"></div>
        </div>
      </div>

      <div v-else-if="activeTab === 'points'">
        <div class="section-title">积分分析</div>
        <div v-if="pointsData">
          <div class="metrics-row">
            <div class="metric-card">
              <h4>今日消耗</h4>
              <p>{{ pointsData.consumedToday }}</p>
            </div>
            <div class="metric-card">
              <h4>今日充值</h4>
              <p>{{ pointsData.rechargedToday }}</p>
            </div>
            <div class="metric-card">
              <h4>今日净变化</h4>
              <p>{{ pointsData.netChangeToday }}</p>
            </div>
            <div class="metric-card">
              <h4>可用余额</h4>
              <p>{{ pointsData.totalAvailable }}</p>
            </div>
          </div>
          <div class="table-container">
            <h4>积分趋势 (7日)</h4>
            <div class="chart-placeholder" v-if="pointsData.consumedTrend">
              <div class="chart-title">消耗趋势</div>
              <div class="chart-content">
                <div class="chart-box">
                  <div v-for="point in pointsData.consumedTrend" :key="point.date" class="trend-point">
                    <div class="point-date">{{ point.date }}</div>
                    <div class="point-value">{{ point.value }}</div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
        <div class="loading-placeholder" v-else>
          <div class="skeleton-loader" v-for="i in 3" :key="i"></div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import * as echarts from "echarts/core";
import { LineChart } from "echarts/charts";
import { GridComponent, LegendComponent, TooltipComponent } from "echarts/components";
import { SVGRenderer } from "echarts/renderers";
import { adminRequest } from "../../api/client";
import { normalizeAnalyticsModelsResponse, normalizeAnalyticsProvidersResponse } from "./analyticsContract";

echarts.use([LineChart, GridComponent, LegendComponent, TooltipComponent, SVGRenderer]);

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
  successRate: number;
  avgLatencyMs: number;
}

interface TrendData {
  newUsers: Array<{ date: string; value: number }>;
  dau: Array<{ date: string; value: number }>;
  wau: Array<{ date: string; value: number }>;
  mau: Array<{ date: string; value: number }>;
  aiUsers: Array<{ date: string; value: number }>;
  images: Array<{ date: string; value: number }>;
  videos: Array<{ date: string; value: number }>;
  points: Array<{ date: string; value: number }>;
  tokens: Array<{ date: string; value: number }>;
  revenue: Array<{ date: string; value: number }>;
  cost: Array<{ date: string; value: number }>;
  tasks: Array<{ date: string; value: number }>;
  success: Array<{ date: string; value: number }>;
  latency: Array<{ date: string; value: number }>;
  failed: Array<{ date: string; value: number }>;
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
  byUser: Array<{
    userId: string;
    userName: string;
    value: number;
  }>;
}

interface PointsData {
  consumedToday: number;
  rechargedToday: number;
  grantedToday: number;
  frozenToday: number;
  releasedToday: number;
  netChangeToday: number;
  totalAvailable: number;
  totalFrozen: number;
  consumedTrend: Array<{ date: string; value: number }>;
  rechargedTrend: Array<{ date: string; value: number }>;
  byType: Array<{
    type: string;
    count: number;
    rate: number;
  }>;
}

// Refs for data
const overviewData = ref<OverviewData | null>(null);
const trendsData = ref<TrendData | null>(null);
const modelsData = ref<ModelData[] | null>(null);
const providersData = ref<ProviderData[] | null>(null);
const tokensData = ref<TokenData | null>(null);
const pointsData = ref<PointsData | null>(null);

// Loading states
const loading = ref(true);
const requestError = ref(false);
const overviewLoading = ref(true);
const trendsLoading = ref(true);
const modelsLoading = ref(true);
const providersLoading = ref(true);
const tokensLoading = ref(true);
const pointsLoading = ref(true);

// Tab state
const activeTab = ref<'overview' | 'trends' | 'models' | 'providers' | 'tokens' | 'points'>('overview');

// Trend keys and labels for display
type TrendKey = 'newUsers' | 'dau' | 'aiUsers' | 'points' | 'tokens';
const chartKeys: TrendKey[] = [
  'newUsers',
  'dau',
  'aiUsers',
  'points',
  'tokens',
];

const chartLabels: Record<TrendKey, string> = {
  newUsers: '新增用户趋势',
  dau: 'DAU 趋势',
  aiUsers: 'AI 用户趋势',
  points: '积分消耗趋势',
  tokens: 'Token使用趋势',
};

const chartElements = new Map<TrendKey, HTMLElement>();
const chartInstances = new Map<TrendKey, echarts.ECharts>();

function setChartElement(element: unknown, key: TrendKey) {
  if (element instanceof HTMLElement) chartElements.set(key, element);
}

function renderCharts() {
  if (!trendsData.value || activeTab.value !== 'trends') return;
  for (const key of chartKeys) {
    const element = chartElements.get(key);
    if (!element) continue;
    const chart = chartInstances.get(key) || echarts.init(element, undefined, { renderer: "svg" });
    chartInstances.set(key, chart);
    const points = trendsData.value[key] || [];
    chart.setOption({
      animation: false,
      tooltip: { trigger: 'axis' },
      grid: { left: 36, right: 18, top: 18, bottom: 28 },
      xAxis: { type: 'category', data: points.map((point) => point.date) },
      yAxis: { type: 'value' },
      series: [{ type: 'line', smooth: true, data: points.map((point) => point.value), areaStyle: {} }]
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

// Fetch overview data (using store data via props would be ideal,
// but for simplicity we'll fetch it directly here as well)
async function fetchOverviewData() {
  try {
    overviewLoading.value = true;
    const response = await adminRequest<OverviewData>({
      method: "GET",
      url: "/admin/analytics/overview"
    });
    overviewData.value = response;
  } catch (error) {
    console.error("failed to fetch overview data:", error);
    requestError.value = true;
    overviewData.value = null;
  } finally {
    overviewLoading.value = false;
  }
}

// Fetch trends data
async function fetchTrendsData() {
  try {
    trendsLoading.value = true;
    const response = await adminRequest<TrendData>({
      method: "GET",
      url: "/admin/analytics/trends?days=7"
    });
    trendsData.value = response;
  } catch (error) {
    console.error("failed to fetch trends data:", error);
    requestError.value = true;
    trendsData.value = null;
  } finally {
    trendsLoading.value = false;
  }
}

// Fetch models data
async function fetchModelsData() {
  try {
    modelsLoading.value = true;
    const response = await adminRequest<{ models: ModelData[] }>({
      method: "GET",
      url: "/admin/analytics/models"
    });
    modelsData.value = normalizeAnalyticsModelsResponse(response);
  } catch (error) {
    console.error("failed to fetch models data:", error);
    requestError.value = true;
    modelsData.value = null;
  } finally {
    modelsLoading.value = false;
  }
}

// Fetch providers data
async function fetchProvidersData() {
  try {
    providersLoading.value = true;
    const response = await adminRequest<{ providers: ProviderData[] }>({
      method: "GET",
      url: "/admin/analytics/providers"
    });
    providersData.value = normalizeAnalyticsProvidersResponse(response);
  } catch (error) {
    console.error("failed to fetch providers data:", error);
    requestError.value = true;
    providersData.value = null;
  } finally {
    providersLoading.value = false;
  }
}

// Fetch tokens data
async function fetchTokensData() {
  try {
    tokensLoading.value = true;
    const response = await adminRequest<TokenData>({
      method: "GET",
      url: "/admin/analytics/tokens"
    });
    tokensData.value = response;
  } catch (error) {
    console.error("failed to fetch tokens data:", error);
    requestError.value = true;
    tokensData.value = null;
  } finally {
    tokensLoading.value = false;
  }
}

// Fetch points data
async function fetchPointsData() {
  try {
    pointsLoading.value = true;
    const response = await adminRequest<PointsData>({
      method: "GET",
      url: "/admin/analytics/points"
    });
    pointsData.value = response;
  } catch (error) {
    console.error("failed to fetch points data:", error);
    requestError.value = true;
    pointsData.value = null;
  } finally {
    pointsLoading.value = false;
  }
}

// Fetch all data
async function fetchAllData() {
  requestError.value = false;
  await Promise.all([
    fetchOverviewData(),
    fetchTrendsData(),
    fetchModelsData(),
    fetchProvidersData(),
    fetchTokensData(),
    fetchPointsData()
  ]);

  // Update overall loading state
  loading.value = !(overviewData.value &&
                   trendsData.value &&
                   modelsData.value &&
                   providersData.value &&
                   tokensData.value &&
                   pointsData.value);
}

// Lifecycle
onMounted(() => {
  fetchAllData();
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

// Refresh function
function refreshData() {
  fetchAllData();
}
</script>

<style scoped>
.analytics-dashboard {
  padding: 20px;
  background: var(--admin-panel-color);
  border-radius: 8px;
}

.overview-cards {
  margin-bottom: 24px;
}

.overview-card {
  background: white;
  border-radius: 8px;
  padding: 20px;
  box-shadow: 0 2px 4px rgba(0,0,0,0.08);
}

.overview-card h3 {
  margin: 0 0 16px 0;
  color: var(--admin-text-color);
  font-size: 18px;
  font-weight: 600;
}

.metrics-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 12px;
}

.metric-item {
  display: flex;
  flex-direction: column;
}

.metric-item label {
  font-size: 12px;
  color: var(--admin-text-color-light);
  margin-bottom: 4px;
}

.metric-item value {
  font-size: 20px;
  font-weight: 600;
  color: var(--admin-text-color);
  line-height: 1.2;
}

.metric-item unit {
  font-size: 12px;
  color: var(--admin-text-color-light);
  margin-left: 4px;
}

.tabs-container {
  display: flex;
  border-bottom: 1px solid var(--admin-border-color);
  margin-bottom: 20px;
}

.tab {
  padding: 12px 16px;
  cursor: pointer;
  color: var(--admin-text-color-light);
  border-bottom: 2px solid transparent;
  transition: all 0.2s ease;
}

.tab:hover {
  color: var(--admin-text-color);
}

.tab.active {
  color: var(--color-primary);
  border-bottom-color: var(--color-primary);
}

.tab-content {
  min-height: 300px;
}

.section-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--admin-text-color);
  margin: 0 0 16px 0;
  padding: 0 20px;
}

.charts-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 16px;
  padding: 20px;
}

.chart-placeholder {
  background: white;
  border-radius: 8px;
  padding: 16px;
  box-shadow: 0 2px 4px rgba(0,0,0,0.08);
}

.chart-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--admin-text-color);
  margin: 0 0 12px 0;
}

.chart-content {
  min-height: 120px;
}

.chart-box {
  border: 1px dashed var(--admin-border-color);
  border-radius: 4px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--admin-text-color-light);
  font-size: 12px;
}

.trend-point {
  display: flex;
  flex-direction: column;
  align-items: center;
  margin: 4px 0;
}

.point-date {
  font-size: 10px;
  color: var(--admin-text-color-light);
}

.point-value {
  font-size: 12px;
  font-weight: 600;
  color: var(--admin-text-color);
}

.table-container {
  background: white;
  border-radius: 8px;
  overflow: hidden;
  box-shadow: 0 2px 4px rgba(0,0,0,0.08);
  margin: 0 20px;
}

.analytics-table {
  width: 100%;
  border-collapse: collapse;
}

.analytics-table th,
.analytics-table td {
  padding: 12px 16px;
  text-align: left;
  border-bottom: 1px solid var(--admin-border-color);
}

.analytics-table th {
  background-color: var(--admin-background-color-light);
  font-weight: 600;
  font-size: 14px;
  color: var(--admin-text-color);
}

.analytics-table td {
  font-size: 13px;
  color: var(--admin-text-color);
}

.analytics-table tr:hover {
  background-color: var(--admin-background-color-light);
}

.empty-state {
  text-align: center;
  padding: 32px 0;
  color: var(--admin-text-color-light);
  font-style: italic;
}

.metrics-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}

.metric-card {
  background: white;
  border-radius: 8px;
  padding: 16px;
  text-align: center;
  box-shadow: 0 2px 4px rgba(0,0,0,0.08);
}

.metric-card h4 {
  margin: 0 0 8px 0;
  font-size: 13px;
  color: var(--admin-text-color-light);
  font-weight: 600;
}

.metric-card p {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: var(--admin-text-color);
}

.loading-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 200px;
}

.skeleton-loader {
  height: 4px;
  background: var(--admin-border-color);
  margin: 4px 0;
  border-radius: 2px;
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0% {
    opacity: 0.6;
  }
  50% {
    opacity: 1;
  }
  100% {
    opacity: 0.6;
  }
}
</style>
