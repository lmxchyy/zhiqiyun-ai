<template>
  <view class="inspiration-workbench">
    <view class="inspiration-header">
      <view>
        <text class="inspiration-kicker">ONLINE IMAGE STUDIO</text>
        <text class="inspiration-title">灵感画布</text>
      </view>
      <view class="canvas-module-tabs">
        <button type="button" :class="{ active: canvasTopView === 'canvas' }" @click="showCanvasView">画布</button>
        <button type="button" :class="{ active: canvasTopView === 'assets' }" @click="openAssetsView">我的作品</button>
        <button type="button" :class="{ active: canvasTopView === 'usage' }" @click="showUsageRecords">使用记录</button>
      </view>
      <view class="infinite-status-dock">
        <view class="dock-stat online">
          <text>ONLINE</text>
          <text>1</text>
        </view>
        <view class="dock-stat">
          <text>QUEUE</text>
          <text>{{ isGenerating ? count : 0 }}</text>
        </view>
        <view class="dock-stat quota">
          <text>额度</text>
          <text>{{ quota }}</text>
        </view>
        <button type="button" class="dock-manage" @click="toggleArchiveManager">管理</button>
        <button type="button" class="dock-more" @click="toggleMoreMenu">更多...</button>
        <view v-if="showMoreMenu" class="dock-more-menu">
          <button type="button" @click="openAssetsView">我的作品查看</button>
        </view>
      </view>
    </view>

    <view class="inspiration-canvas-area">
      <view class="inspiration-grid"></view>
      <view class="canvas-toolbar">
        <button type="button" class="history-button" @click="toggleCanvasHistory">
          <text class="button-icon">↺</text>
          <text>历史画布</text>
          <text class="button-count">{{ canvasSessionCards.length }}</text>
        </button>
        <button type="button" class="new-button" @click="createNewCanvas">
          <text class="button-icon">＋</text>
          <text>新增画布</text>
        </button>
        <button type="button" class="clear-all-button" @click="confirmClearAllCanvases">
          <text class="trash-icon"><text class="trash-lid"></text><text class="trash-can"></text></text>
          <text>清除所有画布</text>
        </button>
      </view>
      <view v-if="canvasTopView === 'usage'" class="canvas-usage-panel">
        <view class="usage-panel-head">
          <text>使用记录</text>
          <text>{{ usageRecordCards.length }} 条</text>
        </view>
        <view class="usage-record-list">
          <view v-for="record in usageRecordCards" :key="record.id" class="usage-record-card">
            <view>
              <text class="usage-record-title">{{ record.title }}</text>
              <text class="usage-record-meta">{{ record.meta }}</text>
            </view>
            <text :class="['usage-record-status', { failed: record.status === 'FAILED' }]">{{ record.statusText }}</text>
          </view>
          <view v-if="!usageRecordCards.length" class="usage-record-empty">暂无使用记录。</view>
        </view>
      </view>
      <view v-if="canvasTopView === 'assets'" class="canvas-assets-drawer">
        <view class="assets-drawer-head">
          <view>
            <text>我的作品</text>
            <text>{{ assets.length }} 个作品</text>
          </view>
          <button type="button" aria-label="关闭我的作品" @click="showCanvasView">×</button>
        </view>
        <view class="assets-drawer-list">
          <view v-for="asset in drawerAssets" :key="asset.id" class="assets-drawer-card">
            <button type="button" class="assets-drawer-preview" @click="openAssetActions(asset)">
              <image v-if="asset.mediaType === 'image'" :src="assetThumbnail(asset)" mode="aspectFill" lazy-load />
              <text v-else>{{ asset.mediaType }}</text>
            </button>
            <view class="assets-drawer-copy">
              <text>{{ asset.name }}</text>
              <text>{{ formatAssetTime(asset) }}</text>
              <text>{{ assetPrompt(asset) }}</text>
            </view>
            <view class="assets-drawer-actions">
              <button v-if="asset.mediaType === 'image'" type="button" @click.stop="openAssetActions(asset)">预览</button>
              <button v-if="asset.mediaType === 'image'" type="button" @click.stop="useAssetAsReference(asset)">加入编辑</button>
              <button type="button" @click.stop="downloadAsset(asset)">下载</button>
            </view>
          </view>
          <view v-if="!assets.length" class="assets-drawer-empty">暂无作品。</view>
        </view>
      </view>
      <view v-if="showCanvasHistory" class="canvas-history-panel">
        <view class="history-panel-head">
          <text>历史画布</text>
          <text>{{ activeCanvasTitle }}</text>
        </view>
        <button
          v-for="session in canvasSessionCards"
          :key="session.id"
          type="button"
          :class="['history-card', { active: session.id === activeCanvasId }]"
          @click="openCanvasSession(session.id)"
        >
          <text class="history-title">{{ session.title }}</text>
          <text class="history-meta">{{ session.assetCount }} 张图片 · {{ session.failedCount }} 条失败记录</text>
          <text class="history-meta">{{ formatCanvasTime(session.updatedAt) }}</text>
          <text :class="['history-status', { failed: session.failedCount && !session.assetCount }]">
            {{ session.id === allHistoryCanvasId ? "全部历史" : "画布" }}
          </text>
        </button>
      </view>
      <view class="inspiration-canvas-scroll">
        <view class="inspiration-board" :style="{ minHeight: `${boardHeight}px` }">
          <view v-if="!boardItems.length && !isGenerating" class="inspiration-empty">
            <text>Canvas Ready</text>
            <text>{{ activeCanvasId === allHistoryCanvasId ? "输入 Prompt 后生成图片，结果会自动进入画布和归档。" : "这是一个新的空白画布，下一次生成会归入这里。" }}</text>
          </view>

          <view v-if="isGenerating" class="inspiration-node pending-node">
            <view class="inspiration-spinner"></view>
            <text>Generating</text>
            <text>{{ estimatedPointCost }} 点 · {{ selectedRatioLabel }}</text>
          </view>

          <view
            v-for="item in boardItems"
            :key="item.key"
            :class="['inspiration-node', {
              'failed-node': item.kind === 'failed',
              selected: item.kind === 'asset' && item.asset && selectedNodeAssetId === item.asset.id
            }]"
            :style="{ top: `${item.y}px`, left: `${item.x}px` }"
          >
            <template v-if="item.kind === 'asset' && item.asset">
              <template v-if="item.asset.mediaType === 'image'">
                <button
                  type="button"
                  class="node-preview-trigger"
                  @click="openAssetActions(item.asset)"
                  @longpress.stop.prevent="selectNodeForDelete(item.asset)"
                  @contextmenu.stop.prevent="selectNodeOnly(item.asset)"
                >
                  <image :src="assetThumbnail(item.asset)" mode="aspectFill" lazy-load />
                  <text class="node-open-label">查看详情</text>
                </button>
              </template>
              <button v-else type="button" class="inspiration-file" @click="openAssetActions(item.asset)">{{ item.asset.mediaType }}</button>
              <view class="inspiration-node-body">
                <text>{{ item.asset.name }}</text>
                <text>{{ assetPrompt(item.asset) }}</text>
              </view>
              <view v-if="item.asset.mediaType === 'image' && assetResultSize(item.asset)" class="node-result-size">
                <text>{{ assetResultSize(item.asset) }}</text>
              </view>
              <view class="node-task-meta">
                <text>{{ assetRoundLabel(item.asset) }}</text>
                <text>{{ assetTaskTypeLabel(item.asset) }}</text>
                <text class="node-task-time">{{ formatAssetTime(item.asset) }}</text>
                <button type="button" @click.stop.prevent="reuseAsset(item.asset)">复用提示词</button>
              </view>
              <view v-if="item.asset.mediaType === 'image'" class="node-card-actions">
                <button type="button" @click.stop.prevent="useAssetAsReference(item.asset)">加入编辑</button>
                <button type="button" @click.stop.prevent="downloadAsset(item.asset)">下载</button>
                <button
                  v-if="selectedNodeAssetId === item.asset.id && selectedNodeMode === 'select'"
                  type="button"
                  class="node-selected-button"
                  @click.stop.prevent="clearNodeSelection"
                >
                  已选中
                </button>
                <button
                  v-else
                  type="button"
                  :class="['node-delete-button', { active: pendingDeleteAssetId === item.asset.id }]"
                  :disabled="isDeletingArchiveAssets"
                  aria-label="删除图片"
                  @click.stop.prevent="deleteNodeAsset(item.asset)"
                >
                  <text></text>
                </button>
              </view>
            </template>

            <template v-else-if="item.failed">
              <view class="failed-preview">
                <text>生成失败</text>
                <text>{{ failedPointCost(item.failed) }} 点 · {{ ratioLabelForValue(item.failed.imageRatio) }} · {{ item.failed.imageQuality }}</text>
              </view>
              <view class="inspiration-node-body">
                <text>{{ item.failed.prompt }}</text>
                <text>{{ item.failed.message }}</text>
                <button type="button" :disabled="isGenerating" @click="retryFailedGeneration(item.failed)">重新生成</button>
              </view>
              <view class="failed-card-actions">
                <button
                  type="button"
                  class="node-delete-button failed-delete-button"
                  aria-label="删除失败记录"
                  @click.stop.prevent="deleteFailedGeneration(item.failed)"
                >
                  <text></text>
                </button>
              </view>
            </template>
          </view>
        </view>
      </view>
      <view v-if="isGenerating" class="inspiration-generating-overlay">
        <view class="generating-card">
          <view class="generating-orb">
            <view class="generating-orb-core"></view>
          </view>
          <view class="generating-copy">
            <text>{{ currentGeneratingStep.title }}</text>
            <text>{{ estimatedPointCost }} 点 · {{ selectedRatioLabel }} · {{ imageQuality }}</text>
            <view class="generating-steps">
              <text
                v-for="(item, index) in generatingSteps"
                :key="item.title"
                :class="{ active: index === generatingStepIndex }"
              ></text>
            </view>
            <text>{{ currentGeneratingStep.message }}</text>
            <text>{{ promptPreview }}</text>
          </view>
        </view>
      </view>
    </view>

    <view class="inspiration-create">
      <view v-if="selectedReferences.length" class="reference-preview-grid">
        <view
          v-for="asset in selectedReferences"
          :key="asset.id"
          class="reference-preview-item"
        >
          <image :src="assetThumbnail(asset)" mode="aspectFill" lazy-load />
          <button type="button" @click.stop="removeReference(asset.id)">×</button>
        </view>
      </view>

      <view class="create-field reference-field">
        <text>Reference Images</text>
        <view class="reference-upload-zone" @click="uploadReferenceImage">
          <text>{{ selectedReferences.length ? "继续添加" : "上传参考图" }}</text>
          <text>{{ selectedReferences.length ? `${selectedReferences.length}/${maxReferenceImages}` : "可多选" }}</text>
        </view>
        <view v-if="selectedReferences.length" class="reference-upload-count">
          <text>{{ selectedReferences.length }}/{{ maxReferenceImages }}</text>
          <text>点击空白继续添加</text>
        </view>
      </view>

      <label class="create-field prompt-field">
        <text>Prompt</text>
        <view class="prompt-input-wrap">
          <textarea
            :key="promptPlaceholder"
            v-model="prompt"
            maxlength="-1"
            :placeholder="promptEditing ? '' : promptPlaceholder"
            @mousedown="promptInputSource = 'mouse'"
            @touchstart="promptInputSource = 'touch'"
            @focus="onPromptFocus"
            @keydown="promptEditing = true"
            @blur="promptEditing = false"
          />
        </view>
      </label>

      <view class="create-field model-field">
        <text>Model</text>
        <picker :range="modelNames" :value="modelIndex" @change="onModelChange">
          <view class="create-picker">{{ selectedModelName }}</view>
        </picker>
      </view>

      <view class="create-field size-field">
        <text>Size</text>
        <picker :range="ratioLabels" :value="ratioIndex" @change="onRatioChange">
          <view class="create-picker">{{ selectedRatioLabel }}</view>
        </picker>
      </view>

      <view class="create-field quality-field">
        <text>Quality</text>
        <picker :range="qualities" :value="qualityIndex" @change="onQualityChange">
          <view class="create-picker quality-picker">{{ imageQuality }}</view>
        </picker>
      </view>

      <view class="create-field point-field">
        <text>点数</text>
        <picker :range="countLabels" :value="countIndex" @change="onCountChange">
          <view class="create-picker count-picker">{{ estimatedPointCost }} 点</view>
        </picker>
      </view>

      <view class="create-field quota-field">
        <text>剩余额度</text>
        <view class="create-picker quota-picker">剩余 {{ quota }}</view>
      </view>

      <button type="button" class="generate-button" :disabled="isGenerating || !prompt.trim()" @click="generate">
        {{ isGenerating ? "Generating" : "Generate" }}
      </button>
      <text v-if="errorMessage" class="create-error">{{ errorMessage }}</text>
    </view>

    <view v-if="showArchiveManager" class="inspiration-archives">
      <view class="archive-head">
        <view>
          <text>管理</text>
          <text>{{ selectedArchiveIds.length ? `已选 ${selectedArchiveIds.length}` : "点击图片选择" }}</text>
        </view>
        <view class="archive-head-actions">
          <button type="button" @click="emit('refresh')">刷新</button>
          <button type="button" :disabled="!selectedArchiveIds.length || isDeletingArchiveAssets" @click="deleteSelectedArchiveAssets">
            删除已选
          </button>
        </view>
      </view>
      <scroll-view class="archive-list" scroll-y>
        <view
          v-for="asset in assets"
          :key="asset.id"
          :class="['archive-item', { selected: isArchiveSelected(asset.id) }]"
          @click="toggleArchiveAsset(asset)"
        >
          <view class="archive-check"><text>{{ isArchiveSelected(asset.id) ? "✓" : "" }}</text></view>
          <image v-if="asset.mediaType === 'image'" :src="assetThumbnail(asset)" mode="aspectFill" lazy-load />
          <view class="archive-item-copy">
            <text>{{ asset.name }}</text>
            <text>{{ formatAssetTime(asset) }}</text>
          </view>
          <button type="button" :disabled="isDeletingArchiveAssets" @click.stop="deleteArchiveAsset(asset)">删除</button>
        </view>
      </scroll-view>
    </view>

    <view v-if="selectedAsset" class="asset-action-layer" @click="closeAssetActions">
      <view class="asset-action-panel" @click.stop>
        <button type="button" class="asset-action-close" @click="closeAssetActions">×</button>
        <view :class="['asset-action-preview', { loaded: selectedAssetLoaded }]">
          <template v-if="selectedAsset.mediaType === 'image'">
            <image class="asset-preview-placeholder" :src="assetThumbnail(selectedAsset)" mode="aspectFit" />
            <image class="asset-preview-main" :src="selectedAsset.url" mode="aspectFit" @load="onSelectedAssetLoad" />
            <view v-if="!selectedAssetLoaded" class="asset-loading-hint">
              <text>正在加载高清图</text>
            </view>
          </template>
          <view v-else class="inspiration-file">{{ selectedAsset.mediaType }}</view>
          <view class="asset-resolution-badge">{{ selectedAssetResolution }}</view>
        </view>
        <view class="asset-action-body">
          <text>图片详情</text>
          <text>{{ selectedAsset.name }}</text>
          <text>{{ selectedAssetPrompt }}</text>
          <view class="asset-detail-grid">
            <view v-for="item in selectedAssetDetails" :key="item.label">
              <text>{{ item.label }}</text>
              <text>{{ item.value }}</text>
            </view>
          </view>
          <view class="asset-action-buttons">
            <button v-if="selectedAsset.mediaType === 'image'" type="button" @click="previewSelectedAsset">预览</button>
            <button type="button" @click="reuseSelectedAsset">复用</button>
            <button v-if="selectedAsset.mediaType === 'image'" type="button" @click="useSelectedAsReference">加入编辑</button>
            <button type="button" @click="downloadSelectedAsset">下载</button>
          </view>
        </view>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { createInspirationImage, deleteInspirationAsset } from "../api/inspirationCanvas";
import type { Asset, GenerationTask, ModelInfo } from "../types";

interface FailedGeneration {
  id: string;
  prompt: string;
  model: string;
  imageRatio: string;
  imageQuality: string;
  count: number;
  referenceIds: string[];
  message: string;
  createdAt: string;
}

interface BoardItem {
  kind: "asset" | "failed";
  key: string;
  asset?: Asset;
  failed?: FailedGeneration;
  createdAt: number;
  x: number;
  y: number;
}

interface CanvasSession {
  id: string;
  title: string;
  assetIds: string[];
  taskIds: string[];
  failedIds: string[];
  createdAt: string;
  updatedAt: string;
}

interface CanvasSessionCard extends CanvasSession {
  assetCount: number;
  failedCount: number;
}

interface UsageRecordCard {
  id: string;
  title: string;
  meta: string;
  status: "SUCCEEDED" | "FAILED";
  statusText: string;
  createdAt: number;
}

const props = defineProps<{
  assets: Asset[];
  models: ModelInfo[];
  quota: number;
  defaultModel: string;
}>();

const emit = defineEmits<{
  generated: [task: GenerationTask];
  refresh: [];
}>();

const ratios = [
  { label: "比例为空", value: "1:1" },
  { label: "1:1 方图", value: "1:1" },
  { label: "2:3 竖图", value: "2:3" },
  { label: "3:2 横图", value: "3:2" },
  { label: "3:4 竖图", value: "3:4" },
  { label: "4:3 横图", value: "4:3" },
  { label: "9:16 竖屏", value: "9:16" },
  { label: "16:9 宽屏", value: "16:9" }
];
const qualities = ["1K", "2K", "4K"];
const counts = [1, 2, 3, 4, 5, 6, 7, 8];
const maxReferenceImages = 9;
const failedGenerationsStorageKey = "inspiration-canvas-failed-generations";
const canvasSessionsStorageKey = "inspiration-canvas-sessions";
const activeCanvasStorageKey = "inspiration-canvas-active-id";
const allHistoryCanvasId = "all-history";
const generatingSteps = [
  { title: "正在理解提示词", message: "分析主体、风格、构图和画面重点..." },
  { title: "正在搭建画面", message: "生成基础构图，并匹配参考图与比例参数..." },
  { title: "正在细化细节", message: "增强光影、材质、边缘和海报视觉层次..." },
  { title: "正在准备出图", message: "整理生成结果，完成后会自动放到画布最下面。" }
];

const prompt = ref("");
const promptEditing = ref(false);
const promptInputSource = ref<"mouse" | "touch" | "">("");
const selectedModel = ref(props.defaultModel || "gpt-image-2");
const selectedRatio = ref("1:1");
const imageQuality = ref("1K");
const count = ref(1);
const isGenerating = ref(false);
const errorMessage = ref("");
const referenceAssetIds = ref<string[]>([]);
const uploadedReferenceAssets = ref<Asset[]>([]);
const failedGenerations = ref<FailedGeneration[]>(loadFailedGenerations());
const showArchiveManager = ref(false);
const selectedAsset = ref<Asset | null>(null);
const selectedAssetSize = ref({ width: 0, height: 0 });
const selectedAssetLoaded = ref(false);
const selectedArchiveIds = ref<string[]>([]);
const isDeletingArchiveAssets = ref(false);
const pendingDeleteAssetId = ref("");
const selectedNodeAssetId = ref("");
const selectedNodeMode = ref<"" | "select" | "delete">("");
const generatingStepIndex = ref(0);
const viewportWidth = ref(typeof window === "undefined" ? 980 : window.innerWidth);
const hasSeenBoardContentSync = ref(false);
const canvasSessions = ref<CanvasSession[]>(loadCanvasSessions());
const activeCanvasId = ref(loadActiveCanvasId());
const showCanvasHistory = ref(false);
const canvasTopView = ref<"canvas" | "assets" | "usage">("canvas");
const showMoreMenu = ref(false);
let generatingTimer: ReturnType<typeof setInterval> | undefined;

const modelNames = computed(() => props.models.length ? props.models.map(item => item.name || item.code) : [selectedModel.value]);
const modelIndex = computed(() => Math.max(0, props.models.findIndex(item => item.code === selectedModel.value)));
const selectedModelName = computed(() => props.models.find(item => item.code === selectedModel.value)?.name || selectedModel.value);
const selectedModelProviderId = computed(() => props.models.find(item => item.code === selectedModel.value)?.providerId || "");
const ratioLabels = computed(() => ratios.map(item => item.label));
const ratioIndex = computed(() => Math.max(0, ratios.findIndex(item => item.value === selectedRatio.value)));
const qualityIndex = computed(() => Math.max(0, qualities.findIndex(item => item === imageQuality.value)));
const selectedModelPointCost = computed(() => modelPointCost(selectedModel.value));
const estimatedPointCost = computed(() => count.value * selectedModelPointCost.value);
const countLabels = computed(() => counts.map(item => `${item * selectedModelPointCost.value} 点`));
const countIndex = computed(() => Math.max(0, counts.findIndex(item => item === count.value)));
const selectedRatioLabel = computed(() => ratios[ratioIndex.value]?.label || ratios[0].label);
const promptPreview = computed(() => {
  const value = prompt.value.trim();
  return value.length > 38 ? `${value.slice(0, 38)}...` : value || "等待提示词";
});
const promptPlaceholder = computed(() => referenceAssetIds.value.length
  ? "描述你希望如何修改参考图"
  : "输入你想要生成的画面，也可直接粘贴图片"
);
const currentGeneratingStep = computed(() => generatingSteps[generatingStepIndex.value] || generatingSteps[0]);
const selectedAssetPrompt = computed(() => selectedAsset.value ? assetPrompt(selectedAsset.value) : "");
const selectedAssetResolution = computed(() => {
  const resolution = selectedAsset.value ? stringMetadata(selectedAsset.value, "resolution") : "";
  if (resolution) return resolution;
  const width = selectedAssetSize.value.width || numericMetadata(selectedAsset.value, "width");
  const height = selectedAssetSize.value.height || numericMetadata(selectedAsset.value, "height");
  return width && height ? `${width} x ${height}` : "0 x 0";
});
const selectedAssetDetails = computed(() => {
  if (!selectedAsset.value) return [];
  return [
    { label: "类型", value: selectedAsset.value.mediaType },
    { label: "分辨率", value: selectedAssetResolution.value },
    { label: "海报生成时间", value: formatAssetTime(selectedAsset.value) },
    { label: "模型", value: stringMetadata(selectedAsset.value, "model") || selectedModelName.value },
    { label: "任务", value: selectedAsset.value.taskId || "本地资产" }
  ];
});
const selectedReferences = computed(() => referenceAssetIds.value
  .map(id => referenceAssetById(id))
  .filter((asset): asset is Asset => Boolean(asset))
);
const sortedBoardAssets = computed(() => [...props.assets].sort((a, b) => assetTimeMs(a) - assetTimeMs(b)));
const drawerAssets = computed(() => [...props.assets].sort((a, b) => assetTimeMs(b) - assetTimeMs(a)));
const sortedFailedGenerations = computed(() => [...failedGenerations.value].sort((a, b) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime()));
const activeCanvasSession = computed(() => canvasSessions.value.find(session => session.id === activeCanvasId.value));
const visibleBoardAssets = computed(() => {
  const session = activeCanvasSession.value;
  if (activeCanvasId.value === allHistoryCanvasId || !session) return sortedBoardAssets.value;
  return sortedBoardAssets.value.filter(asset => canvasSessionIncludesAsset(session, asset));
});
const visibleFailedGenerations = computed(() => {
  const session = activeCanvasSession.value;
  if (activeCanvasId.value === allHistoryCanvasId || !session) return sortedFailedGenerations.value;
  return sortedFailedGenerations.value.filter(item => session.failedIds.includes(item.id));
});
const canvasSessionCards = computed<CanvasSessionCard[]>(() => {
  const allHistoryCard: CanvasSessionCard = {
    id: allHistoryCanvasId,
    title: "全部历史",
    assetIds: props.assets.map(asset => asset.id),
    taskIds: [],
    failedIds: failedGenerations.value.map(item => item.id),
    createdAt: firstCanvasTime(),
    updatedAt: latestCanvasTime(),
    assetCount: props.assets.length,
    failedCount: failedGenerations.value.length
  };
  const savedCards = canvasSessions.value.map(session => ({
    ...session,
    assetCount: props.assets.filter(asset => canvasSessionIncludesAsset(session, asset)).length,
    failedCount: failedGenerations.value.filter(item => session.failedIds.includes(item.id)).length
  }));
  return [allHistoryCard, ...savedCards].sort((a, b) => {
    if (a.id === allHistoryCanvasId) return -1;
    if (b.id === allHistoryCanvasId) return 1;
    return new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime();
  });
});
const activeCanvasTitle = computed(() => canvasSessionCards.value.find(session => session.id === activeCanvasId.value)?.title || "全部历史");
const usageRecordCards = computed<UsageRecordCard[]>(() => {
  const assetRecords = sortedBoardAssets.value.map(asset => {
    const model = stringMetadata(asset, "model") || selectedModelName.value;
    const pointCost = modelPointCost(model);
    return {
      id: `asset_${asset.id}`,
      title: assetPrompt(asset),
      meta: `${formatAssetTime(asset)} · ${assetTaskTypeLabel(asset)} · ${model} · ${pointCost} 点`,
      status: "SUCCEEDED" as const,
      statusText: "成功",
      createdAt: assetTimeMs(asset)
    };
  });
  const failedRecords = sortedFailedGenerations.value.map(item => ({
    id: `failed_${item.id}`,
    title: item.prompt,
    meta: `${formatFailedTime(item)} · ${ratioLabelForValue(item.imageRatio)} · ${failedPointCost(item)} 点`,
    status: "FAILED" as const,
    statusText: "失败",
    createdAt: new Date(item.createdAt).getTime()
  }));
  return [...assetRecords, ...failedRecords]
    .sort((a, b) => b.createdAt - a.createdAt)
    .slice(0, 50);
});
const selectedArchiveAssets = computed(() => props.assets.filter(asset => selectedArchiveIds.value.includes(asset.id)));
const isMobileCanvas = computed(() => viewportWidth.value <= 980);
const boardHeight = computed(() => {
  const itemCount = Math.max(boardItems.value.length, isGenerating.value ? 1 : 0);
  const columns = isMobileCanvas.value ? 1 : 2;
  const rows = Math.max(1, Math.ceil(itemCount / columns));
  return Math.max(isMobileCanvas.value ? 1120 : 1280, 80 + rows * 388);
});

watch(() => props.models, modelItems => {
  if (!modelItems.length || modelItems.some(item => item.code === selectedModel.value)) return;
  selectedModel.value = modelItems[0].code;
}, { immediate: true });
const boardItems = computed<BoardItem[]>(() => {
  const items: Array<Omit<BoardItem, "x" | "y">> = [
    ...visibleBoardAssets.value.map(asset => ({
    kind: "asset" as const,
    key: asset.id,
    asset,
    createdAt: assetTimeMs(asset)
    })),
    ...visibleFailedGenerations.value.map(failed => ({
    kind: "failed" as const,
    key: failed.id,
    failed,
    createdAt: new Date(failed.createdAt).getTime()
    }))
  ];
  return items.sort((a, b) => a.createdAt - b.createdAt).map((item, index) => {
    const columns = isMobileCanvas.value ? 1 : 2;
    const nodeWidth = Math.min(268, Math.max(0, viewportWidth.value - 32));
    const x = isMobileCanvas.value
      ? Math.max(16, Math.floor((viewportWidth.value - nodeWidth) / 2))
      : 28 + (index % columns) * 478;
    return {
      ...item,
      x,
      y: 28 + Math.floor(index / columns) * 388
    };
  });
});

onMounted(() => {
  updateViewportWidth();
  window.addEventListener("resize", updateViewportWidth);
});

watch(() => props.defaultModel, value => {
  if (value) selectedModel.value = value;
});

watch(prompt, value => {
  if (!value.trim()) {
    promptEditing.value = false;
  }
});

watch(() => props.assets.map(asset => asset.id).join("|"), () => {
  selectedArchiveIds.value = selectedArchiveIds.value.filter(id => props.assets.some(asset => asset.id === id));
});

watch(failedGenerations, value => {
  saveFailedGenerations(value);
}, { deep: true });

watch(canvasSessions, value => {
  saveCanvasSessions(value);
}, { deep: true });

watch(activeCanvasId, value => {
  uni.setStorageSync(activeCanvasStorageKey, value);
});

watch(() => [
  props.assets.map(asset => `${asset.id}:${asset.createdAt || asset.updatedAt || ""}`).join("|"),
  failedGenerations.value.map(item => `${item.id}:${item.createdAt}`).join("|")
].join("::"), () => {
  if (!hasSeenBoardContentSync.value) {
    hasSeenBoardContentSync.value = true;
    return;
  }
  void scrollCanvasToLatest();
});

onBeforeUnmount(() => {
  stopGeneratingMotion();
  if (typeof window !== "undefined") {
    window.removeEventListener("resize", updateViewportWidth);
  }
});

function updateViewportWidth() {
  if (typeof window !== "undefined") {
    viewportWidth.value = window.innerWidth;
  }
}

function onModelChange(event: { detail: { value: number } }) {
  const next = props.models[event.detail.value];
  if (next) selectedModel.value = next.code;
}

function onRatioChange(event: { detail: { value: number } }) {
  selectedRatio.value = ratios[event.detail.value]?.value || "1:1";
}

function onQualityChange(event: { detail: { value: number } }) {
  imageQuality.value = qualities[event.detail.value] || "1K";
}

function onCountChange(event: { detail: { value: number } }) {
  count.value = counts[event.detail.value] || 1;
}

function onPromptFocus() {
  promptEditing.value = promptInputSource.value === "touch";
}

function loadFailedGenerations() {
  try {
    const raw = uni.getStorageSync(failedGenerationsStorageKey);
    if (!raw || typeof raw !== "string") return [];
    const parsed = JSON.parse(raw) as FailedGeneration[];
    return Array.isArray(parsed) ? parsed.filter(isFailedGeneration).slice(-20) : [];
  } catch {
    return [];
  }
}

function loadCanvasSessions() {
  try {
    const raw = uni.getStorageSync(canvasSessionsStorageKey);
    if (!raw || typeof raw !== "string") return [];
    const parsed = JSON.parse(raw) as CanvasSession[];
    return Array.isArray(parsed) ? parsed.filter(isCanvasSession).slice(-40) : [];
  } catch {
    return [];
  }
}

function loadActiveCanvasId() {
  try {
    const raw = uni.getStorageSync(activeCanvasStorageKey);
    return typeof raw === "string" && raw.trim() ? raw : allHistoryCanvasId;
  } catch {
    return allHistoryCanvasId;
  }
}

function saveFailedGenerations(items: FailedGeneration[]) {
  uni.setStorageSync(failedGenerationsStorageKey, JSON.stringify(items.slice(-20)));
}

function saveCanvasSessions(items: CanvasSession[]) {
  uni.setStorageSync(canvasSessionsStorageKey, JSON.stringify(items.slice(-40)));
}

function isFailedGeneration(value: unknown): value is FailedGeneration {
  if (!value || typeof value !== "object") return false;
  const item = value as Partial<FailedGeneration>;
  return typeof item.id === "string"
    && typeof item.prompt === "string"
    && typeof item.model === "string"
    && typeof item.imageRatio === "string"
    && typeof item.imageQuality === "string"
    && typeof item.count === "number"
    && Array.isArray(item.referenceIds)
    && typeof item.message === "string"
    && typeof item.createdAt === "string";
}

function isCanvasSession(value: unknown): value is CanvasSession {
  if (!value || typeof value !== "object") return false;
  const item = value as Partial<CanvasSession>;
  return typeof item.id === "string"
    && typeof item.title === "string"
    && Array.isArray(item.assetIds)
    && Array.isArray(item.taskIds)
    && Array.isArray(item.failedIds)
    && typeof item.createdAt === "string"
    && typeof item.updatedAt === "string";
}

function ratioLabelForValue(value: string) {
  return ratios.find(item => item.value === value)?.label || value;
}

function modelPointCost(modelCode: string) {
  const model = props.models.find(item => item.code === modelCode);
  const directCost = model?.pointCost;
  if (typeof directCost === "number" && Number.isFinite(directCost) && directCost > 0) return directCost;
  const fixedQuota = model?.fixedQuota;
  if (typeof fixedQuota === "number" && Number.isFinite(fixedQuota) && fixedQuota > 0) return fixedQuota;
  return modelCode === "gpt-image-2" ? 10 : 1;
}

function failedPointCost(item: FailedGeneration) {
  return item.count * modelPointCost(item.model);
}

function canvasSessionIncludesAsset(session: CanvasSession, asset: Asset) {
  return session.assetIds.includes(asset.id) || Boolean(asset.taskId && session.taskIds.includes(asset.taskId));
}

function firstCanvasTime() {
  const times = [
    ...props.assets.map(assetTimeMs),
    ...failedGenerations.value.map(item => new Date(item.createdAt).getTime())
  ].filter(time => Number.isFinite(time) && time > 0);
  return new Date(times.length ? Math.min(...times) : Date.now()).toISOString();
}

function latestCanvasTime() {
  const times = [
    ...props.assets.map(assetTimeMs),
    ...failedGenerations.value.map(item => new Date(item.createdAt).getTime())
  ].filter(time => Number.isFinite(time) && time > 0);
  return new Date(times.length ? Math.max(...times) : Date.now()).toISOString();
}

function formatCanvasTime(raw: string) {
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return "未知时间";
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  });
}

function formatFailedTime(item: FailedGeneration) {
  return formatCanvasTime(item.createdAt);
}

function toggleCanvasHistory() {
  showCanvasHistory.value = !showCanvasHistory.value;
  if (showCanvasHistory.value) {
    canvasTopView.value = "canvas";
  }
  showMoreMenu.value = false;
}

function showCanvasView() {
  canvasTopView.value = "canvas";
  showCanvasHistory.value = false;
  showMoreMenu.value = false;
}

function showUsageRecords() {
  canvasTopView.value = "usage";
  showCanvasHistory.value = false;
  showMoreMenu.value = false;
}

function openCanvasSession(id: string) {
  activeCanvasId.value = id;
  showCanvasView();
  clearNodeSelection();
  void scrollCanvasToLatest();
}

function createNewCanvas() {
  const now = new Date().toISOString();
  const nextSession: CanvasSession = {
    id: `canvas_${Date.now()}`,
    title: `新画布 ${canvasSessions.value.length + 1}`,
    assetIds: [],
    taskIds: [],
    failedIds: [],
    createdAt: now,
    updatedAt: now
  };
  canvasSessions.value = [nextSession, ...canvasSessions.value].slice(0, 40);
  activeCanvasId.value = nextSession.id;
  showCanvasView();
  selectedArchiveIds.value = [];
  clearNodeSelection();
  uni.showToast({ title: "已新增画布", icon: "success" });
  void scrollCanvasToLatest();
}

function confirmClearAllCanvases() {
  uni.showModal({
    title: "清除所有画布",
    content: "只清除本地画布分组记录，不删除作品中心图片。确定继续？",
    confirmText: "清除",
    confirmColor: "#dc2626",
    success: result => {
      if (!result.confirm) return;
      canvasSessions.value = [];
      activeCanvasId.value = allHistoryCanvasId;
      showCanvasView();
      selectedArchiveIds.value = [];
      clearNodeSelection();
      uni.showToast({ title: "已清除画布", icon: "success" });
    }
  });
}

function attachTaskToActiveCanvas(task: GenerationTask) {
  if (activeCanvasId.value === allHistoryCanvasId) return;
  const session = activeCanvasSession.value;
  if (!session) return;
  const resultIds = Array.isArray(task.resultIds) ? task.resultIds.filter(Boolean) : [];
  updateCanvasSession(session.id, {
    taskIds: uniqueStrings([...session.taskIds, task.id]),
    assetIds: uniqueStrings([...session.assetIds, ...resultIds]),
    updatedAt: new Date().toISOString()
  });
}

function attachFailedToActiveCanvas(id: string) {
  if (activeCanvasId.value === allHistoryCanvasId) return;
  const session = activeCanvasSession.value;
  if (!session) return;
  updateCanvasSession(session.id, {
    failedIds: uniqueStrings([...session.failedIds, id]),
    updatedAt: new Date().toISOString()
  });
}

function updateCanvasSession(id: string, patch: Partial<CanvasSession>) {
  canvasSessions.value = canvasSessions.value.map(session => session.id === id ? { ...session, ...patch } : session);
}

function uniqueStrings(items: string[]) {
  return Array.from(new Set(items.filter(Boolean)));
}

function assetPrompt(asset: Asset) {
  const value = asset.metadata?.prompt;
  return typeof value === "string" && value.trim() ? value : "Prompt";
}

function assetThumbnail(asset: Asset) {
  const metadataValue = asset.metadata?.thumbnailUrl;
  return asset.thumbnailUrl || (typeof metadataValue === "string" ? metadataValue : "") || asset.url;
}

function referenceAssetById(id: string) {
  return props.assets.find(asset => asset.id === id) || uploadedReferenceAssets.value.find(asset => asset.id === id);
}

function stringMetadata(asset: Asset, key: string) {
  const value = asset.metadata?.[key];
  return typeof value === "string" && value.trim() ? value : "";
}

function numericMetadata(asset: Asset | null, key: string) {
  const value = asset?.metadata?.[key];
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function assetResultSize(asset: Asset) {
  const resolution = stringMetadata(asset, "resolution");
  if (resolution) return `结果 ${resolution}`;
  const width = numericMetadata(asset, "width");
  const height = numericMetadata(asset, "height");
  return width && height ? `结果 ${width} x ${height}` : "";
}

function assetRoundLabel(asset: Asset) {
  const taskId = asset.taskId || stringMetadata(asset, "taskId");
  const match = taskId.match(/(\d+)/);
  if (match) return `第 ${Number(match[1])} 轮`;
  return "任务";
}

function assetTaskTypeLabel(asset: Asset) {
  const type = stringMetadata(asset, "type") || stringMetadata(asset, "sourceType");
  if (type === "IMAGE_TO_IMAGE") return "图生图";
  return "文生图";
}

function assetTimeMs(asset: Asset) {
  const raw = asset.createdAt || asset.updatedAt || stringMetadata(asset, "generatedAt") || stringMetadata(asset, "createdAt") || stringMetadata(asset, "created_at");
  const time = raw ? new Date(raw).getTime() : Number.NaN;
  return Number.isFinite(time) ? time : 0;
}

async function scrollCanvasToLatest() {
  await nextTick();
  const canvas = document.querySelector(".inspiration-canvas-scroll");
  if (canvas instanceof HTMLElement) {
    const hasCanvasContent = boardItems.value.length > 0 || isGenerating.value;
    canvas.scrollTop = hasCanvasContent ? Math.max(0, canvas.scrollHeight - canvas.clientHeight) : 0;
  }
}

function formatAssetTime(asset: Asset) {
  const raw = asset.createdAt || stringMetadata(asset, "createdAt") || stringMetadata(asset, "generatedAt") || stringMetadata(asset, "created_at");
  if (!raw) return "未知";
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw;
  return date.toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  });
}

function reuseAsset(asset: Asset) {
  prompt.value = assetPrompt(asset);
  referenceAssetIds.value = [...referenceAssetIds.value.filter(id => id !== asset.id), asset.id].slice(0, maxReferenceImages);
}

function useAssetAsReference(asset: Asset) {
  referenceAssetIds.value = [...referenceAssetIds.value.filter(id => id !== asset.id), asset.id].slice(0, maxReferenceImages);
}

function removeReference(id: string) {
  referenceAssetIds.value = referenceAssetIds.value.filter(item => item !== id);
  uploadedReferenceAssets.value = uploadedReferenceAssets.value.filter(item => item.id !== id);
}

function uploadReferenceImage() {
  const remainingCount = maxReferenceImages - referenceAssetIds.value.length;
  if (remainingCount <= 0) {
    uni.showToast({ title: "参考图已满", icon: "none" });
    return;
  }
  uni.chooseImage({
    count: Math.max(1, remainingCount),
    sizeType: ["compressed"],
    sourceType: ["album", "camera"],
    success: result => {
      const files = Array.isArray(result.tempFiles) ? result.tempFiles : result.tempFiles ? [result.tempFiles] : [];
      const paths = Array.isArray(result.tempFilePaths) ? result.tempFilePaths : result.tempFilePaths ? [result.tempFilePaths] : [];
      const items = paths.slice(0, remainingCount);
      if (!items.length) return;
      void Promise.all(items.map(async (path: string, index: number) => {
        const file = files[index] as { name?: string; path?: string; size?: number; file?: File } | undefined;
        const referenceURL = await referenceFileURL(file?.file, path);
        const id = `uploaded_reference_${Date.now()}_${index}`;
        const asset: Asset = {
          id,
          name: file?.name || `参考图-${uploadedReferenceAssets.value.length + index + 1}`,
          url: referenceURL,
          thumbnailUrl: referenceURL,
          mediaType: "image",
          metadata: {
            source: "local-upload",
            size: file?.size
          },
          createdAt: new Date().toISOString()
        };
        return asset;
      })).then(nextAssets => {
        uploadedReferenceAssets.value = [...uploadedReferenceAssets.value, ...nextAssets].slice(-maxReferenceImages);
        referenceAssetIds.value = [...referenceAssetIds.value, ...nextAssets.map(asset => asset.id)].slice(0, maxReferenceImages);
      });
    },
    fail: () => {
      uni.showToast({ title: "未选择图片", icon: "none" });
    }
  });
}

function referenceFileURL(file: File | undefined, fallback: string): Promise<string> {
  if (!file || typeof FileReader === "undefined") return Promise.resolve(fallback);
  return new Promise(resolve => {
    const reader = new FileReader();
    reader.onload = () => resolve(typeof reader.result === "string" ? reader.result : fallback);
    reader.onerror = () => resolve(fallback);
    reader.readAsDataURL(file);
  });
}

function openAssetActions(asset: Asset) {
  selectedAsset.value = asset;
  selectedAssetLoaded.value = false;
  selectedAssetSize.value = {
    width: numericMetadata(asset, "width"),
    height: numericMetadata(asset, "height")
  };
}

function closeAssetActions() {
  selectedAsset.value = null;
  selectedAssetLoaded.value = false;
  selectedAssetSize.value = { width: 0, height: 0 };
}

function onSelectedAssetLoad(event: Event) {
  selectedAssetLoaded.value = true;
  const detail = (event as Event & { detail?: { width?: number; height?: number } }).detail;
  selectedAssetSize.value = {
    width: Math.round(detail?.width || selectedAssetSize.value.width || 0),
    height: Math.round(detail?.height || selectedAssetSize.value.height || 0)
  };
}

function toggleArchiveManager() {
  showMoreMenu.value = false;
  showArchiveManager.value = !showArchiveManager.value;
  if (showArchiveManager.value) emit("refresh");
}

function toggleMoreMenu() {
  showMoreMenu.value = !showMoreMenu.value;
}

function openAssetsView() {
  canvasTopView.value = "assets";
  showMoreMenu.value = false;
  showCanvasHistory.value = false;
  showArchiveManager.value = false;
}

function isArchiveSelected(id: string) {
  return selectedArchiveIds.value.includes(id);
}

function toggleArchiveAsset(asset: Asset) {
  if (isArchiveSelected(asset.id)) {
    selectedArchiveIds.value = selectedArchiveIds.value.filter(id => id !== asset.id);
    return;
  }
  selectedArchiveIds.value = [...selectedArchiveIds.value, asset.id];
}

async function deleteSelectedArchiveAssets() {
  if (!selectedArchiveAssets.value.length || isDeletingArchiveAssets.value) return;
  const confirmed = await confirmArchiveDelete(selectedArchiveAssets.value.length);
  if (!confirmed) return;
  await deleteArchiveAssets(selectedArchiveAssets.value);
}

async function deleteArchiveAsset(asset: Asset) {
  if (isDeletingArchiveAssets.value) return;
  const confirmed = await confirmArchiveDelete(1);
  if (!confirmed) return;
  await deleteArchiveAssets([asset]);
}

async function deleteNodeAsset(asset: Asset) {
  if (isDeletingArchiveAssets.value) return;
  pendingDeleteAssetId.value = asset.id;
  selectedNodeAssetId.value = asset.id;
  selectedNodeMode.value = "delete";
  const confirmed = await confirmArchiveDelete(1);
  if (!confirmed) {
    clearNodeSelection();
    return;
  }
  await deleteArchiveAssets([asset]);
}

async function selectNodeForDelete(asset: Asset) {
  if (isDeletingArchiveAssets.value) return;
  selectedNodeAssetId.value = asset.id;
  selectedNodeMode.value = "delete";
  const confirmed = await confirmArchiveDelete(1, "已选中图片，是否删除？");
  if (!confirmed) {
    clearNodeSelection();
    return;
  }
  pendingDeleteAssetId.value = asset.id;
  await deleteArchiveAssets([asset]);
}

function selectNodeOnly(asset: Asset) {
  if (isDeletingArchiveAssets.value) return;
  pendingDeleteAssetId.value = "";
  selectedNodeAssetId.value = asset.id;
  selectedNodeMode.value = "select";
}

function clearNodeSelection() {
  pendingDeleteAssetId.value = "";
  selectedNodeAssetId.value = "";
  selectedNodeMode.value = "";
}

function confirmArchiveDelete(count: number, content?: string) {
  return new Promise<boolean>(resolve => {
    uni.showModal({
      title: "删除图片",
      content: content || `确定删除选中的 ${count} 张图片？删除后会从画布和任务结果里移除。`,
      confirmText: "删除",
      confirmColor: "#dc2626",
      success: result => resolve(Boolean(result.confirm)),
      fail: () => resolve(false)
    });
  });
}

async function deleteArchiveAssets(items: Asset[]) {
  isDeletingArchiveAssets.value = true;
  try {
    for (const asset of items) {
      await deleteInspirationAsset(asset.id);
    }
    const deletedIds = new Set(items.map(asset => asset.id));
    selectedArchiveIds.value = selectedArchiveIds.value.filter(id => !deletedIds.has(id));
    referenceAssetIds.value = referenceAssetIds.value.filter(id => !deletedIds.has(id));
    canvasSessions.value = canvasSessions.value.map(session => ({
      ...session,
      assetIds: session.assetIds.filter(id => !deletedIds.has(id))
    }));
    if (selectedAsset.value && deletedIds.has(selectedAsset.value.id)) {
      closeAssetActions();
    }
    emit("refresh");
    uni.showToast({ title: "已删除", icon: "success" });
  } catch (error) {
    const message = error instanceof Error ? error.message : "删除失败";
    uni.showToast({ title: message, icon: "none" });
  } finally {
    isDeletingArchiveAssets.value = false;
    clearNodeSelection();
  }
}

function reuseSelectedAsset() {
  if (!selectedAsset.value) return;
  reuseAsset(selectedAsset.value);
  closeAssetActions();
}

function useSelectedAsReference() {
  if (!selectedAsset.value) return;
  referenceAssetIds.value = [...referenceAssetIds.value.filter(id => id !== selectedAsset.value?.id), selectedAsset.value.id].slice(0, maxReferenceImages);
  closeAssetActions();
}

async function retryFailedGeneration(item: FailedGeneration) {
  prompt.value = item.prompt;
  selectedModel.value = item.model;
  selectedRatio.value = item.imageRatio;
  imageQuality.value = item.imageQuality;
  count.value = item.count;
  referenceAssetIds.value = item.referenceIds.filter(id => referenceAssetById(id)).slice(0, maxReferenceImages);
  failedGenerations.value = failedGenerations.value.filter(failed => failed.id !== item.id);
  await nextTick();
  await generate();
}

function deleteFailedGeneration(item: FailedGeneration) {
  uni.showModal({
    title: "删除失败记录",
    content: "确定删除这条生成失败的记录吗？",
    confirmText: "删除",
    confirmColor: "#dc2626",
    success: result => {
      if (!result.confirm) return;
      failedGenerations.value = failedGenerations.value.filter(failed => failed.id !== item.id);
      uni.showToast({ title: "已删除", icon: "success" });
    }
  });
}

function previewSelectedAsset() {
  if (!selectedAsset.value) return;
  uni.previewImage({
    current: selectedAsset.value.url,
    urls: [selectedAsset.value.url]
  });
}

function downloadSelectedAsset() {
  if (!selectedAsset.value) return;
  void downloadAsset(selectedAsset.value);
}

async function downloadAsset(asset: Asset) {
  const downloadUrl = asset.id.startsWith("uploaded_reference_")
    ? asset.url
    : `/api/v1/assets/${encodeURIComponent(asset.id)}/download`;
  const fileName = downloadFileName(asset);
  let objectURL = "";
  const link = document.createElement("a");
  try {
    const token = uni.getStorageSync("token");
    const headers = token && !asset.id.startsWith("uploaded_reference_")
      ? { Authorization: `Bearer ${token}` }
      : undefined;
    const response = await fetch(downloadUrl, { headers });
    if (!response.ok) throw new Error(`download failed: ${response.status}`);
    const blob = await response.blob();
    objectURL = URL.createObjectURL(blob);
    link.href = objectURL;
    link.download = fileName;
    link.rel = "noopener";
    link.style.display = "none";
    document.body.appendChild(link);
    link.click();
    link.remove();
    uni.showToast({ title: "已开始下载", icon: "success" });
  } catch {
    uni.showToast({ title: "下载失败，请稍后重试", icon: "none" });
  } finally {
    if (objectURL) {
      window.setTimeout(() => URL.revokeObjectURL(objectURL), 1000);
    }
  }
}

function downloadFileName(asset: Asset) {
  const rawName = (asset.name || `inspiration-${Date.now()}`).replace(/[\\/:*?"<>|]+/g, "-");
  if (/\.(png|jpe?g|webp|gif|svg)$/i.test(rawName)) return rawName;
  const contentType = stringMetadata(asset, "contentType");
  if (contentType.includes("svg")) return `${rawName}.svg`;
  if (contentType.includes("jpeg") || contentType.includes("jpg")) return `${rawName}.jpg`;
  if (contentType.includes("webp")) return `${rawName}.webp`;
  return `${rawName}.png`;
}

function startGeneratingMotion() {
  stopGeneratingMotion();
  generatingStepIndex.value = 0;
  generatingTimer = setInterval(() => {
    generatingStepIndex.value = (generatingStepIndex.value + 1) % generatingSteps.length;
  }, 1700);
}

function stopGeneratingMotion() {
  if (!generatingTimer) return;
  clearInterval(generatingTimer);
  generatingTimer = undefined;
}

async function generate() {
  const cleanPrompt = prompt.value.trim();
  if (!cleanPrompt || isGenerating.value) return;
  const requestSnapshot = {
    prompt: cleanPrompt,
    model: selectedModel.value,
    imageRatio: selectedRatio.value,
    imageQuality: imageQuality.value,
    count: count.value,
    referenceIds: [...referenceAssetIds.value]
  };
  isGenerating.value = true;
  startGeneratingMotion();
  errorMessage.value = "";
  try {
    const task = await createInspirationImage({
      prompt: cleanPrompt,
      model: selectedModel.value,
      provider: selectedModelProviderId.value,
      imageRatio: selectedRatio.value,
      imageQuality: imageQuality.value,
      count: count.value,
      references: selectedReferences.value
    });
    attachTaskToActiveCanvas(task);
    emit("generated", task);
    emit("refresh");
    uni.showToast({ title: "已生成", icon: "success" });
  } catch (error) {
    const message = error instanceof Error ? error.message : "生成失败";
    const failedId = `failed_generation_${Date.now()}`;
    errorMessage.value = message;
    failedGenerations.value = [...failedGenerations.value, {
      id: failedId,
      ...requestSnapshot,
      message,
      createdAt: new Date().toISOString()
    }];
    attachFailedToActiveCanvas(failedId);
  } finally {
    isGenerating.value = false;
    stopGeneratingMotion();
  }
}
</script>

