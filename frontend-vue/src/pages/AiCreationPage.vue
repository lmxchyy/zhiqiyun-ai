<template>
  <view :class="['app-shell', { 'canvas-mode': activeModule === 'generate' || activeModule === 'assets' }]">
    <aside class="sidebar">
      <view class="brand-block">
        <text class="brand-title">先知 AI</text>
        <text class="brand-subtitle">AI Operating System</text>
      </view>
      <view class="mobile-module-bar">
        <view class="current-module-card">
          <text class="current-module-label">当前模块</text>
          <text class="current-module-title">{{ currentModule.label }}</text>
        </view>
        <button class="more-button" @click="isModuleDrawerOpen = true">更多</button>
      </view>
      <view class="nav-list">
        <button
          v-for="item in sidebarModules"
          :key="item.id"
          :class="['nav-item', { active: activeModule === item.id }]"
          @click="selectModule(item.id)"
        >
          {{ item.label }}
        </button>
      </view>
    </aside>

    <view class="workspace">
      <scroll-view v-if="activeModule === 'dashboard'" class="workspace-scroll" scroll-y>
        <view class="hero">
          <text class="eyebrow">WELCOME BACK</text>
          <text class="hero-title">让创意成为可交付成果</text>
          <text class="hero-copy">内容生产、智能体、GEO 增长、会员商业化和渠道运营已回到统一工作台。</text>
        </view>
        <view class="metric-grid">
          <view v-for="metric in metrics" :key="metric.label" class="metric-card">
            <text>{{ metric.label }}</text>
            <text class="metric-value">{{ metric.value }}</text>
          </view>
        </view>
        <view class="module-grid">
          <view v-for="item in sidebarModules.filter(module => module.id !== 'dashboard')" :key="item.id" class="module-card" @click="selectModule(item.id)">
            <text class="module-title">{{ item.label }}</text>
            <text>{{ item.description }}</text>
            <text class="module-status">{{ item.status }}</text>
          </view>
        </view>
      </scroll-view>

      <view v-else-if="activeModule === 'generate' || activeModule === 'assets'" class="creation-page">
        <view class="topbar">
          <view class="topbar-brand">
          </view>
          <view class="tabs">
            <button :class="{ active: activeModule === 'generate' }" @click="selectModule('generate')">画图</button>
            <button :class="{ active: activeModule === 'assets' }" @click="selectModule('assets')">我的作品</button>
            <button>画廊</button>
          </view>
          <view class="topbar-status">
            <text :class="['online-pill', models.length ? 'online' : 'offline']">
              {{ models.length ? "ONLINE" : "OFFLINE" }}
            </text>
            <text class="user-pill"><text class="user-badge">先</text>先知 · 普通用户</text>
            <button class="logout-button">退出</button>
          </view>
        </view>
        <template v-if="activeModule === 'generate'">
          <CreationCanvas
          :items="canvasItems"
          :history-items="allCanvasItems"
          :history-count="allCanvasItems.length"
          :showing-history="showHistory"
          :quota="quota"
          :running-count="runningTaskCount"
          @reuse="reuseTask"
          @select-history="selectHistoryItem"
          @delete="deleteAsset"
          @restore-history="restoreHistory"
          @new-canvas="newCanvas"
          @clear-canvas="clearCanvas"
          />
          <ComposerBar
          v-model:prompt="prompt"
          v-model:model="model"
          v-model:count="count"
          v-model:ratio="ratio"
          :models="models"
          @submit="submit"
          />
        </template>
        <scroll-view v-else class="creation-assets workspace-scroll" scroll-y>
          <view class="section-head">
            <text class="section-title">我的作品</text>
            <text>生成资产、参考图和可交付内容统一归档。</text>
          </view>
          <view class="asset-grid">
            <view v-for="asset in assets" :key="asset.id" class="asset-card">
              <image v-if="asset.mediaType === 'image'" :src="asset.url" mode="aspectFit" />
              <view v-else class="asset-placeholder">{{ asset.mediaType }}</view>
              <text class="asset-name">{{ asset.name }}</text>
            </view>
            <view v-if="!assets.length" class="empty-card">暂无作品，先去 AI 画布生成一张图。</view>
          </view>
        </scroll-view>
      </view>

      <scroll-view v-else class="workspace-scroll" scroll-y>
        <view class="module-detail">
          <text class="eyebrow">{{ currentModule.status }}</text>
          <text class="detail-title">{{ currentModule.label }}</text>
          <text class="detail-copy">{{ currentModule.description }}</text>
          <view class="capability-list">
            <view v-for="capability in currentModule.capabilities" :key="capability" class="capability-item">
              <text>{{ capability }}</text>
            </view>
          </view>
          <text class="detail-note">该模块入口已恢复到 uni-app 工作台，下一步会继续把旧 Node API 对应能力迁移到 Go 服务。</text>
        </view>
      </scroll-view>
    </view>

    <view v-if="isModuleDrawerOpen" class="module-drawer-layer" @click="isModuleDrawerOpen = false">
      <view class="module-drawer" @click.stop>
        <view class="drawer-handle"></view>
        <view class="drawer-head">
          <view>
            <text class="eyebrow">MODULES</text>
            <text class="drawer-title">切换工作模块</text>
          </view>
          <button class="drawer-close" @click="isModuleDrawerOpen = false">关闭</button>
        </view>
        <view class="drawer-grid">
          <button
            v-for="item in sidebarModules"
            :key="item.id"
            :class="['drawer-module', { active: activeModule === item.id }]"
            @click="selectModule(item.id)"
          >
            <text class="drawer-module-title">{{ item.label }}</text>
            <text class="drawer-module-status">{{ item.status }}</text>
          </button>
        </view>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { api } from "../api/client";
import CreationCanvas from "../components/CreationCanvas.vue";
import ComposerBar from "../components/ComposerBar.vue";
import type { Asset, GenerationTask, ModelInfo, PointAccount } from "../types";

type ModuleId = "dashboard" | "generate" | "assets" | "ppt" | "agents" | "geo" | "enterprise" | "membership" | "channel" | "admin";

const modules: Array<{
  id: ModuleId;
  label: string;
  description: string;
  status: string;
  capabilities: string[];
}> = [
  { id: "generate", label: "AI 画布", description: "文生图、参考图、视频任务和画布式结果追踪。", status: "已接入 Go API", capabilities: ["画布生成", "模型选择", "作品删除", "配置复用"] },
  { id: "dashboard", label: "运营总览", description: "平台指标、任务、作品和商业化状态总览。", status: "已接入", capabilities: ["指标总览", "模块导航", "工作台状态"] },
  { id: "assets", label: "作品中心", description: "管理生成资产、参考文件和可下载交付物。", status: "已接入 Go API", capabilities: ["资产列表", "图片预览", "作品归档"] },
  { id: "ppt", label: "AI PPT", description: "选题、大纲、页面编辑和 PPTX/PDF 导出。", status: "待迁移 Go API", capabilities: ["PPT 项目", "页面编辑", "PPTX 导出", "PDF 导出"] },
  { id: "agents", label: "智能体", description: "智能体创建、工作流编排、发布、调用与反馈。", status: "待迁移 Go API", capabilities: ["工作流节点", "知识库绑定", "版本回滚", "调用反馈"] },
  { id: "geo", label: "GEO 优化", description: "品牌监测、竞品分析、趋势报告和优化内容生成。", status: "待迁移 Go API", capabilities: ["品牌管理", "定时监测", "周报", "内容发布跟踪"] },
  { id: "enterprise", label: "企业管理", description: "企业空间、成员、额度、计费来源和审计。", status: "待迁移 Go API", capabilities: ["企业空间", "成员额度", "额度流水", "审计记录"] },
  { id: "membership", label: "会员订单", description: "套餐、积分、订单、支付、发票和兑换码。", status: "待迁移 Go API", capabilities: ["会员套餐", "支付订单", "优惠券", "兑换码"] },
  { id: "channel", label: "代理商", description: "渠道代理、客户绑定、佣金、提现和排行榜。", status: "待迁移 Go API", capabilities: ["代理审核", "客户绑定", "佣金释放", "提现审核"] },
  { id: "admin", label: "运营后台", description: "用户、收入、模型成本、审核和监控指标。", status: "待迁移 Go API", capabilities: ["用户状态", "模型供应商", "成本报表", "Prometheus 指标"] }
];

const activeModule = ref<ModuleId>("generate");
const tasks = ref<GenerationTask[]>([]);
const assets = ref<Asset[]>([]);
const models = ref<ModelInfo[]>([]);
const pointAccount = ref<PointAccount | null>(null);
const prompt = ref("");
const model = ref("gpt-image-2");
const count = ref(1);
const ratio = ref("4:3");
const isModuleDrawerOpen = ref(false);
const canvasStartedAt = ref(new Date().toISOString());
const showHistory = ref(false);
const selectedHistoryAssetId = ref<string | null>(null);

const currentModule = computed(() => modules.find(item => item.id === activeModule.value) || modules[0]);
const sidebarModules = computed(() => modules.filter(item => item.id !== "assets"));
const quota = computed(() => pointAccount.value?.available || 0);
const runningTaskCount = computed(() => tasks.value.filter(item => ["QUEUED", "PROCESSING", "RETRYING"].includes(item.status)).length);
const metrics = computed(() => [
  { label: "生成任务", value: tasks.value.length },
  { label: "作品资产", value: assets.value.length },
  { label: "可用模型", value: models.value.length },
  { label: "工作台模块", value: sidebarModules.value.length - 1 }
]);


const assetById = computed(() => new Map(assets.value.map(asset => [asset.id, asset])));

const allCanvasItems = computed(() => {
  const sorted = [...tasks.value].sort((a, b) => {
    return new Date(a.createdAt || 0).getTime() - new Date(b.createdAt || 0).getTime();
  });
  return sorted
    .flatMap(task => {
      return (task.resultIds || []).map(assetId => ({
        task,
        asset: assetById.value.get(assetId)
      }));
    })
    .filter((item): item is { task: GenerationTask; asset: Asset } => item.asset?.mediaType === "image");
});

const canvasItems = computed(() => {
  if (selectedHistoryAssetId.value) {
    return allCanvasItems.value.filter(item => item.asset.id === selectedHistoryAssetId.value);
  }
  if (showHistory.value) return allCanvasItems.value;
  const startedAt = new Date(canvasStartedAt.value).getTime();
  return allCanvasItems.value.filter(item => new Date(item.task.createdAt || 0).getTime() >= startedAt);
});

onMounted(refresh);

async function refresh() {
  const [taskItems, assetItems, modelItems, points] = await Promise.all([
    api<GenerationTask[]>("/api/v1/generation-tasks"),
    api<Asset[]>("/api/v1/assets"),
    api<ModelInfo[]>("/api/v1/models"),
    api<{ account: PointAccount }>("/api/v1/points/account")
  ]);
  tasks.value = taskItems;
  assets.value = assetItems;
  models.value = modelItems;
  pointAccount.value = points.account;
  model.value = models.value.find(item => item.code === "gpt-image-2")?.code || models.value[0]?.code || model.value;
}

function reuseTask(task: GenerationTask) {
  selectModule("generate");
  selectedHistoryAssetId.value = null;
  showHistory.value = false;
  prompt.value = task.prompt;
  model.value = task.model || model.value;
  ratio.value = String(task.params?.imageRatio || ratio.value);
}

function restoreHistory() {
  showHistory.value = true;
}

function selectHistoryItem(item: { task: GenerationTask; asset: Asset }) {
  selectModule("generate");
  selectedHistoryAssetId.value = item.asset.id;
  showHistory.value = false;
}

function newCanvas() {
  canvasStartedAt.value = new Date().toISOString();
  showHistory.value = false;
  selectedHistoryAssetId.value = null;
  prompt.value = "";
}

function clearCanvas() {
  newCanvas();
}

function selectModule(id: ModuleId) {
  activeModule.value = id;
  isModuleDrawerOpen.value = false;
}

async function deleteAsset(asset: Asset) {
  const confirmed = await new Promise<boolean>(resolve => {
    uni.showModal({
      title: "删除图片",
      content: "确定删除这张图片吗？",
      success: (result: UniApp.ShowModalRes) => resolve(result.confirm),
      fail: () => resolve(false)
    });
  });
  if (!confirmed) return;
  await api(`/api/v1/assets/${asset.id}`, { method: "DELETE" });
  await refresh();
}

async function submit() {
  if (!prompt.value.trim()) return;
  await api("/api/v1/generation-tasks", {
    method: "POST",
    body: JSON.stringify({
      type: "TEXT_TO_IMAGE",
      prompt: prompt.value,
      model: model.value,
      params: { count: count.value, imageRatio: ratio.value },
      idempotencyKey: crypto.randomUUID()
    })
  });
  if (!showHistory.value) {
    selectedHistoryAssetId.value = null;
    canvasStartedAt.value = new Date(Date.now() - 1000).toISOString();
  }
  await refresh();
}
</script>
