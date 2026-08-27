<template>
  <section class="analytics-dashboard">
    <!-- Overview Cards -->
    <div class="overview-cards">
      <div class="overview-card" v-if="overviewData">
        <h3>今日概览</h3>
        <div class="metrics-grid">
          <div class="metric-item">
            <label>日新增用户</label>
            <value>{{ overviewData.NewUsersToday }}</value>
          </div>
          <div class="metric-item">
            <label>日活跃用户 (DAU)</label>
            <value>{{ overviewData.DAU }}</value>
          </div>
          <div class="metric-item">
            <label>周活跃用户 (WAU)</label>
            <value>{{ overviewData.WAU }}</value>
          </div>
          <div class="metric-item">
            <label>月活跃用户 (MAU)</label>
            <value>{{ overviewData.MAU }}</value>
          </div>
          <div class="metric-item">
            <label>今日AI用户</label>
            <value>{{ overviewData.AIUsersToday }}</value>
          </div>
          <div class="metric-item">
            <label>今日图片生成</label>
            <value>{{ overviewData.ImagesGenerated }}</value>
          </div>
          <div class="metric-item">
            <label>今日视频生成</label>
            <value>{{ overviewData.VideosGenerated }}</value>
          </div>
          <div class="metric-item">
            <label>今日积分消耗</label>
            <value>{{ overviewData.PointsConsumed }}</value>
          </div>
          <div class="metric-item">
            <label>今日Token使用</label>
            <value>{{ overviewData.TokensUsed }}</value>
          </div>
          <div class="metric-item">
            <label>今日收入</label>
            <value>{{ overviewData.RevenueTodayCents / 100 }}</value>
            <unit>元</unit>
          </div>
          <div class="metric-item">
            <label>今日成本</label>
            <value>{{ overviewData.CostTodayCents / 100 }}</value>
            <unit>元</unit>
          </div>
          <div class="metric-item">
            <label>今日失败任务</label>
            <value>{{ overviewData.FailedTasksToday }}</value>
          </div>
          <div class="metric-item">
            <label>今日成功率</label>
            <value>{{ overviewData.SuccessRate.toFixed(1) }}%</value>
          </div>
          <div class="metric-item">
            <label>今日平均延迟</label>
            <value>{{ overviewData.AvgLatencyMs }}ms</value>
          </div>
        </div>
      </div>
    </div>

    <!-- Tabs for different sections -->
    <div class="tabs-container" v-if="!loading">
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
        <div class="charts-grid" v-if="trendsData">
          <!-- Trends charts will go here -->
          <div class="chart-placeholder" v-for="key in trendKeys" :key="key">
            <div class="chart-title">{{ trendLabels[key] }}</div>
            <div class="chart-content">
              <!-- In a real implementation, this would render ECharts -->
              <div class="chart-box">
                <template v-if="key === 'NewUsers'">
                  <div v-for="point in trendsData.NewUsers" :key="point.Date" class="trend-point">
                    <div class="point-date">{{ point.Date }}</div>
                    <div class="point-value">{{ point.Value }}</div>
                  </div>
                </template>
                <template v-else-if="key === 'DAU'">
                  <div v-for="point in trendsData.DAU" :key="point.Date" class="trend-point">
                    <div class="point-date">{{ point.Date }}</div>
                    <div class="point-value">{{ point.Value }}</div>
                  </div>
                </template>
                <template v-else-if="key === 'WAU'">
                  <div v-for="point in trendsData.WAU" :key="point.Date" class="trend-point">
                    <div class="point-date">{{ point.Date }}</div>
                    <div class="point-value">{{ point.Value }}</div>
                  </div>
                </template>
                <template v-else-if="key === 'MAU'">
                  <div v-for="point in trendsData.MAU" :key="point.Date" class="trend-point">
                    <div class="point-date">{{ point.Date }}</div>
                    <div class="point-value">{{ point.Value }}</div>
                  </div>
                </template>
                <template v-else-if="key === 'AIUsers'">
                  <div v-for="point in trendsData.AIUsers" :key="point.Date" class="trend-point">
                    <div class="point-date">{{ point.Date }}</div>
                    <div class="point-value">{{ point.Value }}</div>
                  </div>
                </template>
                <template v-else-if="key === 'Images'">
                  <div v-for="point in trendsData.Images" :key="point.Date" class="trend-point">
                    <div class="point-date">{{ point.Date }}</div>
                    <div class="point-value">{{ point.Value }}</div>
                  </div>
                </template>
                <template v-else-if="key === 'Videos'">
                  <div v-for="point in trendsData.Videos" :key="point.Date" class="trend-point">
                    <div class="point-date">{{ point.Date }}</div>
                    <div class="point-value">{{ point.Value }}</div>
                  </div>
                </template>
                <template v-else-if="key === 'Points'">
                  <div v-for="point in trendsData.Points" :key="point.Date" class="trend-point">
                    <div class="point-date">{{ point.Date }}</div>
                    <div class="point-value">{{ point.Value }}</div>
                  </div>
                </template>
                <template v-else-if="key === 'Tokens'">
                  <div v-for="point in trendsData.Tokens" :key="point.Date" class="trend-point">
                    <div class="point-date">{{ point.Date }}</div>
                    <div class="point-value">{{ point.Value }}</div>
                  </div>
                </template>
                <template v-else-if="key === 'Revenue'">
                  <div v-for="point in trendsData.Revenue" :key="point.Date" class="trend-point">
                    <div class="point-date">{{ point.Date }}</div>
                    <div class="point-value">{{ point.Value }}</div>
                  </div>
                </template>
                <template v-else-if="key === 'Cost'">
                  <div v-for="point in trendsData.Cost" :key="point.Date" class="trend-point">
                    <div class="point-date">{{ point.Date }}</div>
                    <div class="point-value">{{ point.Value }}</div>
                  </div>
                </template>
                <template v-else-if="key === 'Tasks'">
                  <div v-for="point in trendsData.Tasks" :key="point.Date" class="trend-point">
                    <div class="point-date">{{ point.Date }}</div>
                    <div class="point-value">{{ point.Value }}</div>
                  </div>
                </template>
                <template v-else-if="key === 'Success'">
                  <div v-for="point in trendsData.Success" :key="point.Date" class="trend-point">
                    <div class="point-date">{{ point.Date }}</div>
                    <div class="point-value">{{ point.Value }}</div>
                  </div>
                </template>
                <template v-else-if="key === 'Latency'">
                  <div v-for="point in trendsData.Latency" :key="point.Date" class="trend-point">
                    <div class="point-date">{{ point.Date }}</div>
                    <div class="point-value">{{ point.Value }}</div>
                  </div>
                </template>
                <template v-else-if="key === 'Failed'">
                  <div v-for="point in trendsData.Failed" :key="point.Date" class="trend-point">
                    <div class="point-date">{{ point.Date }}</div>
                    <div class="point-value">{{ point.Value }}</div>
                  </div>
                </template>
              </div>
            </div>
          </div>
        </div>
        <div class="loading-placeholder" v-else>
          <div class="skeleton-loader" v-for="i in 6" :key="i"></div>
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
                <td>{{ model.ModelCode }}</td>
                <td>{{ model.CallCount }}</td>
                <td>{{ (model.SuccessRate || 0).toFixed(1) }}%</td>
                <td>{{ model.AvgLatencyMs || 0 }}</td>
                <td>{{ model.TotalCostCents || 0 }}</td>
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
                <td>{{ provider.ProviderCode }}</td>
                <td>{{ provider.CallCount }}</td>
                <td>{{ (provider.SuccessRate || 0).toFixed(1) }}%</td>
                <td>{{ provider.AvgLatencyMs || 0 }}</td>
                <td>{{ provider.TotalCostCents || 0 }}</td>
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
              <p>{{ tokensData.TokensToday }}</p>
            </div>
            <div class="metric-card">
              <h4>7日使用量</h4>
              <p>{{ tokensData.Tokens7d }}</p>
            </div>
            <div class="metric-card">
              <h4>30日使用量</h4>
              <p>{{ tokensData.Tokens30d }}</p>
            </div>
          </div>
          <div class="table-container">
            <h4>Top 用户 Token 使用量</h4>
            <table class="analytics-table" v-if="tokensData.ByUser && tokensData.ByUser.length > 0">
              <thead>
                <tr>
                  <th>排名</th>
                  <th>用户ID</th>
                  <th>用户名</th>
                  <th>Token使用量</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(user, index) in tokensData.ByUser" :key="index">
                  <td>{{ index + 1 }}</td>
                  <td>{{ user.UserID }}</td>
                  <td>{{ user.UserName }}</td>
                  <td>{{ user.Value }}</td>
                </tr>
              </tbody>
            </table>
            <div v-else-if="tokensData.ByUser && tokensData.ByUser.length === 0">
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
              <p>{{ pointsData.ConsumedToday }}</p>
            </div>
            <div class="metric-card">
              <h4>今日充值</h4>
              <p>{{ pointsData.RechargedToday }}</p>
            </div>
            <div class="metric-card">
              <h4>今日净变化</h4>
              <p>{{ pointsData.NetChangeToday }}</p>
            </div>
            <div class="metric-card">
              <h4>可用余额</h4>
              <p>{{ pointsData.TotalAvailable }}</p>
            </div>
          </div>
          <div class="table-container">
            <h4>积分趋势 (7日)</h4>
            <div class="chart-placeholder" v-if="pointsData.ConsumedTrend">
              <div class="chart-title">消耗趋势</div>
              <div class="chart-content">
                <div class="chart-box">
                  <div v-for="point in pointsData.ConsumedTrend" :key="point.Date" class="trend-point">
                    <div class="point-date">{{ point.Date }}</div>
                    <div class="point-value">{{ point.Value }}</div>
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
import { ref, onMounted } from "vue";
import { adminRequest } from "../../api/client";

interface OverviewData {
  NewUsersToday: number;
  DAU: number;
  WAU: number;
  MAU: number;
  AIUsersToday: number;
  ImagesGenerated: number;
  VideosGenerated: number;
  PointsConsumed: number;
  TokensUsed: number;
  RevenueTodayCents: number;
  CostTodayCents: number;
  FailedTasksToday: number;
  SuccessRate: number;
  AvgLatencyMs: number;
}

interface TrendData {
  NewUsers: Array<{ Date: string; Value: number }>;
  DAU: Array<{ Date: string; Value: number }>;
  WAU: Array<{ Date: string; Value: number }>;
  MAU: Array<{ Date: string; Value: number }>;
  AIUsers: Array<{ Date: string; Value: number }>;
  Images: Array<{ Date: string; Value: number }>;
  Videos: Array<{ Date: string; Value: number }>;
  Points: Array<{ Date: string; Value: number }>;
  Tokens: Array<{ Date: string; Value: number }>;
  Revenue: Array<{ Date: string; Value: number }>;
  Cost: Array<{ Date: string; Value: number }>;
  Tasks: Array<{ Date: string; Value: number }>;
  Success: Array<{ Date: string; Value: number }>;
  Latency: Array<{ Date: string; Value: number }>;
  Failed: Array<{ Date: string; Value: number }>;
}

interface ModelData {
  ModelCode: string;
  CallCount: number;
  SuccessCount: number;
  SuccessRate: number;
  AvgLatencyMs: number;
  TotalCostCents: number;
}

interface ProviderData {
  ProviderCode: string;
  CallCount: number;
  SuccessCount: number;
  SuccessRate: number;
  AvgLatencyMs: number;
  TotalCostCents: number;
}

interface TokenData {
  TokensToday: number;
  Tokens7d: number;
  Tokens30d: number;
  ByUser: Array<{
    UserID: string;
    UserName: string;
    Value: number;
  }>;
}

interface PointsData {
  ConsumedToday: number;
  RechargedToday: number;
  GrantedToday: number;
  FrozenToday: number;
  ReleasedToday: number;
  NetChangeToday: number;
  TotalAvailable: number;
  TotalFrozen: number;
  ConsumedTrend: Array<{ Date: string; Value: number }>;
  RechargedTrend: Array<{ Date: string; Value: number }>;
  ByType: Array<{
    Type: string;
    Count: number;
    Rate: number;
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
const overviewLoading = ref(true);
const trendsLoading = ref(true);
const modelsLoading = ref(true);
const providersLoading = ref(true);
const tokensLoading = ref(true);
const pointsLoading = ref(true);

// Tab state
const activeTab = ref<'overview' | 'trends' | 'models' | 'providers' | 'tokens' | 'points'>('overview');

// Trend keys and labels for display
type TrendKey = 'NewUsers' | 'DAU' | 'WAU' | 'MAU' | 'AIUsers' | 'Images' | 'Videos' | 'Points' | 'Tokens' | 'Revenue' | 'Cost' | 'Tasks' | 'Success' | 'Latency' | 'Failed';
const trendKeys: TrendKey[] = [
  'NewUsers',
  'DAU',
  'WAU',
  'MAU',
  'AIUsers',
  'Images',
  'Videos',
  'Points',
  'Tokens',
  'Revenue',
  'Cost',
  'Tasks',
  'Success',
  'Latency',
  'Failed'
];

const trendLabels: Record<TrendKey, string> = {
  NewUsers: '新增用户趋势',
  DAU: 'DAU趋势',
  WAU: 'WAU趨勢',
  MAU: 'MAU趋勢',
  AIUsers: 'AI用戶趋势',
  Images: '圖片生成趋势',
  Videos: '視頻生成趋势',
  Points: '積分消耗趋势',
  Tokens: 'Token使用趋势',
  Revenue: '收入趋势',
  Cost: '成本趋势',
  Tasks: '任务量趋势',
  Success: '成功率趋势',
  Latency: '平均延迟趋势',
  Failed: '失败任务趋势'
};

// Fetch overview data (using store data via props would be ideal,
// but for simplicity we'll fetch it directly here as well)
async function fetchOverviewData() {
  try {
    overviewLoading.value = true;
    const response = await adminRequest<OverviewData>({
      method: "GET",
      url: "/api/admin/analytics/overview"
    });
    overviewData.value = response;
  } catch (error) {
    console.error("Failed to fetch overview data:", error);
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
      url: "/api/admin/analytics/trends?days=7"
    });
    trendsData.value = response;
  } catch (error) {
    console.error("Failed to fetch trends data:", error);
    trendsData.value = null;
  } finally {
    trendsLoading.value = false;
  }
}

// Fetch models data
async function fetchModelsData() {
  try {
    modelsLoading.value = true;
    const response = await adminRequest<ModelData[]>({
      method: "GET",
      url: "/api/admin/analytics/models"
    });
    modelsData.value = response;
  } catch (error) {
    console.error("Failed to fetch models data:", error);
    modelsData.value = null;
  } finally {
    modelsLoading.value = false;
  }
}

// Fetch providers data
async function fetchProvidersData() {
  try {
    providersLoading.value = true;
    const response = await adminRequest<ProviderData[]>({
      method: "GET",
      url: "/api/admin/analytics/providers"
    });
    providersData.value = response;
  } catch (error) {
    console.error("Failed to fetch providers data:", error);
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
      url: "/api/admin/analytics/tokens"
    });
    tokensData.value = response;
  } catch (error) {
    console.error("Failed to fetch tokens data:", error);
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
      url: "/api/admin/analytics/points"
    });
    pointsData.value = response;
  } catch (error) {
    console.error("Failed to fetch points data:", error);
    pointsData.value = null;
  } finally {
    pointsLoading.value = false;
  }
}

// Fetch all data
async function fetchAllData() {
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