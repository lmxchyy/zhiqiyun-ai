<template>
  <view class="detail-page">
    <view class="detail-header" :style="navigationStyle">
      <button class="back-button" aria-label="返回作品列表" @click="backOrHome('/pages/user/UserAssetsPage')">‹</button>
      <text class="header-title">作品详情</text>
    </view>

    <view class="detail-content">
      <AssetSkeleton v-if="store.currentLoading && !asset" :count="2" />

      <view v-else-if="notFound" class="empty-panel">
        <text class="empty-symbol">□</text>
        <text class="empty-title">作品不存在</text>
        <text class="empty-copy">作品可能已被删除，或当前账号无权查看。</text>
        <button class="empty-action" @click="backOrHome('/pages/user/UserAssetsPage')">返回作品列表</button>
      </view>

      <AssetErrorState
        v-else-if="store.error && !asset"
        title="作品加载失败"
        :description="store.error"
        @retry="load"
      />

      <template v-else-if="asset">
        <AiGeneratedContentNotice />
        <view class="preview-card">
          <video
            v-if="asset.type === 'video' && asset.remoteUrl"
            class="video-preview"
            :src="asset.remoteUrl"
            controls
            :autoplay="autoplay"
          />

          <view v-else-if="asset.type === 'prompt'" class="prompt-preview">
            <text class="preview-kicker">提示词资产</text>
            <text>{{ asset.prompt || displayTitle }}</text>
          </view>

          <view v-else-if="asset.type === 'agent' || asset.type === 'knowledge'" :class="['entity-preview', { knowledge: asset.type === 'knowledge' }]">
            <text class="entity-symbol">{{ asset.type === "knowledge" ? "知" : "A" }}</text>
            <text class="entity-title">{{ displayTitle }}</text>
            <AssetStatusBadge :status="asset.status" />
          </view>

          <view
            v-else-if="isImagePreview"
            :class="['cover-preview', { 'has-error': previewImageError }]"
            :data-asset-id="asset.id"
            @click="preview"
          >
            <image
              v-if="previewSource && !previewImageError"
              :key="previewReloadKey"
              class="preview-image"
              :src="previewSource"
              mode="aspectFit"
              @load="handlePreviewLoad"
              @error="handlePreviewError"
            />
            <view v-if="previewImageLoading && !previewImageError" class="preview-skeleton">
              <text>作品加载中</text>
            </view>
            <view v-if="previewImageError || !previewSource" class="preview-error" @click.stop>
              <text class="preview-error-symbol">!</text>
              <text class="preview-error-title">图片加载失败</text>
              <text class="preview-error-copy">链接可能已过期，请重新获取作品地址。</text>
              <button :disabled="activeAction === 'preview'" @click.stop="reloadPreview">
                {{ activeAction === "preview" ? "重新加载中" : "重新加载" }}
              </button>
            </view>
          </view>

          <view v-else class="document-preview" :data-asset-id="asset.id" @click="preview">
            <AssetCover :asset="asset" />
            <view v-if="asset.type === 'ppt'" class="ppt-controls">
              <button :disabled="previewPage <= 1" @click.stop="previewPage--">上一页</button>
              <text>{{ previewPage }} / {{ asset.pageCount || 1 }}</text>
              <button :disabled="previewPage >= (asset.pageCount || 1)" @click.stop="previewPage++">下一页</button>
            </view>
          </view>
        </view>

        <view class="identity-section">
          <view class="asset-heading">
            <text class="asset-title">{{ displayTitle }}</text>
            <AssetStatusBadge :status="asset.status" />
          </view>
          <text class="asset-subtitle">{{ typeLabel }} · {{ asset.projectName || "未归属项目" }}</text>
        </view>

        <view class="primary-actions">
          <button class="primary-button" :disabled="Boolean(activeAction)" @click="continueEditing">
            <text class="action-icon">✎</text>
            <text>{{ activeAction === "edit" ? "正在打开" : "继续编辑" }}</text>
          </button>
          <button class="regenerate-button" :disabled="Boolean(activeAction)" @click="regenerate">
            <text class="action-icon">↻</text>
            <text>{{ activeAction === "regenerate" ? "正在打开" : "再次生成" }}</text>
          </button>
        </view>

        <view class="utility-actions">
          <button class="download-button utility-button" :disabled="Boolean(activeAction)" @click="download">
            <text class="utility-icon">↓</text>
            <text>{{ activeAction === "download" ? "保存中" : "保存到相册" }}</text>
          </button>
          <button class="utility-button" open-type="share" :disabled="Boolean(activeAction)">
            <text class="utility-icon">↗</text>
            <text>分享</text>
          </button>
          <button class="utility-button" :disabled="Boolean(activeAction)" @click="moreVisible = true">
            <text class="utility-icon more-icon">•••</text>
            <text>更多</text>
          </button>
        </view>

        <view v-if="asset.prompt" class="content-card prompt-card">
          <view class="card-head">
            <text class="card-title">提示词</text>
            <button class="copy-button" @click="copyPrompt">复制</button>
          </view>
          <text :class="['prompt-copy', { collapsed: !promptExpanded }]">{{ asset.prompt }}</text>
          <button v-if="promptCanExpand" class="expand-button" @click="promptExpanded = !promptExpanded">
            {{ promptExpanded ? "收起" : "展开" }}
          </button>
          <view v-if="asset.negativePrompt" class="negative-prompt">
            <text class="field-label">反向提示词</text>
            <text>{{ asset.negativePrompt }}</text>
          </view>
        </view>

        <view v-if="parameterRows.length || variableRows.length" class="content-card collapsible-card">
          <button class="collapse-trigger" @click="parametersExpanded = !parametersExpanded">
            <text class="card-title">生成参数</text>
            <text class="collapse-meta">{{ parameterRows.length + variableRows.length }} 项</text>
            <text :class="['chevron', { expanded: parametersExpanded }]">⌄</text>
          </button>
          <view v-if="parametersExpanded" class="compact-list">
            <view v-for="item in parameterRows" :key="item.label" class="detail-row">
              <text>{{ item.label }}</text>
              <text>{{ item.value }}</text>
            </view>
            <view v-for="item in variableRows" :key="`variable-${item.label}`" class="detail-row">
              <text>{{ item.label }}</text>
              <text>{{ item.value }}</text>
            </view>
          </view>
        </view>

        <view class="content-card collapsible-card">
          <button class="collapse-trigger" @click="assetInfoExpanded = !assetInfoExpanded">
            <text class="card-title">资产信息</text>
            <text class="collapse-meta">{{ detailRows.length }} 项</text>
            <text :class="['chevron', { expanded: assetInfoExpanded }]">⌄</text>
          </button>
          <view v-if="assetInfoExpanded" class="compact-list">
            <view v-for="item in detailRows" :key="item.label" class="detail-row">
              <text>{{ item.label }}</text>
              <text selectable>{{ item.value }}</text>
            </view>
          </view>
        </view>
      </template>
    </view>

    <AssetActionSheet
      v-if="moreVisible && asset"
      :asset="asset"
      mode="manage"
      @close="moreVisible = false"
      @action="handleManagementAction"
    />
  </view>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { onShareAppMessage, onShow } from "@dcloudio/uni-app";
import { useAssetStore } from "../../stores/assets";
import { copyText, downloadAssetFile, previewAsset } from "../../features/assets/platform";
import { registerAssetNativeBridge } from "../../features/assets/nativeBridge";
import type { AssetItem } from "../../features/assets/types";
import AssetActionSheet from "./AssetActionSheet.vue";
import AssetCover from "./AssetCover.vue";
import AssetErrorState from "./AssetErrorState.vue";
import AssetSkeleton from "./AssetSkeleton.vue";
import AssetStatusBadge from "./AssetStatusBadge.vue";
import AiGeneratedContentNotice from "../compliance/AiGeneratedContentNotice.vue";
import { backOrHome } from "../../utils/miniProgramBusiness";

type DetailAction = "" | "preview" | "edit" | "regenerate" | "download" | "favorite" | "move" | "rename" | "archive" | "delete";
type DetailRow = { label: string; value: string };
type CreationMode = "image" | "video" | "ppt" | "agent" | "infographic";
const assetDetailDraftKey = "zhiqiyun:asset-detail:creation-draft";

const props = withDefaults(defineProps<{ assetId?: string; autoplay?: boolean }>(), {
  assetId: "",
  autoplay: false,
});

const store = useAssetStore();
const id = ref("");
const previewPage = ref(1);
const previewImageLoading = ref(true);
const previewImageError = ref(false);
const previewReloadKey = ref(0);
const notFound = ref(false);
const promptExpanded = ref(false);
const parametersExpanded = ref(false);
const assetInfoExpanded = ref(false);
const moreVisible = ref(false);
const activeAction = ref<DetailAction>("");
const navigationStyle = ref<Record<string, string>>({ paddingTop: "env(safe-area-inset-top)", paddingRight: "92px" });
let previewLoadingTimer: ReturnType<typeof setTimeout> | null = null;

const autoplay = computed(() => props.autoplay);
const asset = computed(() => store.currentAsset);
const isImagePreview = computed(() => asset.value?.type === "image" || asset.value?.type === "infographic");
const previewSource = computed(() => asset.value?.thumbnailUrl || asset.value?.remoteUrl || "");
const typeLabel = computed(() => ({
  image: "AI 图片",
  video: "AI 视频",
  ppt: "PPT",
  document: "文档",
  agent: "Agent",
  infographic: "信息图",
  knowledge: "知识库",
  prompt: "提示词",
  template: "模板",
} as Record<string, string>)[asset.value?.type || ""] || "数字资产");
const displayTitle = computed(() => friendlyAssetName(asset.value));
const promptCanExpand = computed(() => {
  const value = asset.value?.prompt || "";
  return value.length > 110 || value.split(/\r?\n/).length > 4;
});

const parameterRows = computed<DetailRow[]>(() => {
  const item = asset.value;
  if (!item) return [];
  const metadata = item.metadata || {};
  const resolution = item.width && item.height
    ? `${item.width} × ${item.height}`
    : stringMetadata(metadata, "resolution", "size");
  const ratio = item.aspectRatio || ratioFromResolution(item.width, item.height) || stringMetadata(metadata, "aspectRatio", "aspect_ratio", "ratio");
  return compactRows([
    { label: "模型", value: item.model || stringMetadata(metadata, "model", "modelName") || "默认" },
    { label: "图片比例", value: ratio },
    { label: "分辨率", value: resolution },
    { label: "生成数量", value: stringMetadata(metadata, "count", "imageCount", "generationCount") },
    { label: "图片质量", value: stringMetadata(metadata, "quality", "imageQuality") },
    { label: "风格", value: stringMetadata(metadata, "style", "stylePreset") },
    { label: "随机种子", value: item.seed === undefined ? "" : String(item.seed) },
    { label: "创建方式", value: creationMethodLabel(stringMetadata(metadata, "sourceType", "type", "creationMethod")) },
  ]);
});

const variableRows = computed<DetailRow[]>(() => Object.entries(asset.value?.variables || {})
  .map(([label, value]) => ({ label, value: typeof value === "string" ? value : JSON.stringify(value) }))
  .filter(item => Boolean(item.value)));

const detailRows = computed<DetailRow[]>(() => {
  const item = asset.value;
  if (!item) return [];
  const metadata = item.metadata || {};
  const resolution = item.width && item.height
    ? `${item.width} × ${item.height}`
    : stringMetadata(metadata, "resolution");
  return compactRows([
    { label: "创建时间", value: formatDate(item.createdAt) },
    { label: "更新时间", value: formatDate(item.updatedAt) },
    { label: "文件大小", value: formatBytes(item.fileSize) },
    { label: "分辨率", value: resolution },
    { label: "任务 ID", value: item.taskId || "" },
    { label: "所属项目", value: item.projectName || "未归属项目" },
    { label: "文件格式", value: fileFormat(item) },
    { label: "标签", value: item.tags.join("、") },
  ]);
});

function compactRows(rows: DetailRow[]) {
  return rows.filter(row => Boolean(String(row.value || "").trim()));
}

function stringMetadata(metadata: Record<string, unknown>, ...keys: string[]) {
  for (const key of keys) {
    const value = metadata[key];
    if (value !== undefined && value !== null && value !== "") return String(value);
  }
  return "";
}

function stringArrayMetadata(metadata: Record<string, unknown>, ...keys: string[]) {
  for (const key of keys) {
    const value = metadata[key];
    if (Array.isArray(value)) {
      return value.map(item => {
        if (typeof item === "string") return item.trim();
        if (item && typeof item === "object") {
          const row = item as Record<string, unknown>;
          return String(row.url || row.remoteUrl || row.sourceUrl || row.fileUrl || "").trim();
        }
        return "";
      }).filter(Boolean);
    }
    if (typeof value === "string" && value.trim()) return value.split(/[,，]/).map(item => item.trim()).filter(Boolean);
  }
  return [];
}

function friendlyAssetName(item: AssetItem | null) {
  if (!item) return "作品详情";
  const current = String(item.name || "").trim();
  const technicalName = /^(TEXT_TO_IMAGE|IMAGE_TO_IMAGE|TEXT_TO_VIDEO|IMAGE_TO_VIDEO|PPT|INFOGRAPHIC)[-_]task[_-]/i.test(current)
    || /^task[_-]\d+/i.test(current);
  if (current && !technicalName && current !== "未命名作品") return current;
  const prompt = String(item.prompt || "").trim();
  if (/水果/.test(prompt) && /电商|商品|详情/.test(prompt)) return "水果电商详情图";
  if (/iphone\s*17/i.test(prompt) && /电商|商品|主图/.test(prompt)) return "iPhone 17 电商主图";
  const promptTitle = prompt.replace(/^(请|帮我|生成|制作|创建|设计)+/g, "").replace(/[，。,.!！?？].*$/, "").trim();
  if (promptTitle) return promptTitle.slice(0, 22);
  return `${typeLabel.value}作品`;
}

function ratioFromResolution(width?: number, height?: number) {
  if (!width || !height) return "";
  const divisor = greatestCommonDivisor(width, height);
  return `${Math.round(width / divisor)}:${Math.round(height / divisor)}`;
}

function greatestCommonDivisor(left: number, right: number): number {
  let a = Math.abs(Math.round(left));
  let b = Math.abs(Math.round(right));
  while (b) [a, b] = [b, a % b];
  return a || 1;
}

function creationMethodLabel(value: string) {
  const normalized = value.toUpperCase();
  if (normalized.includes("IMAGE_TO_IMAGE")) return "参考图生成";
  if (normalized.includes("TEXT_TO_IMAGE")) return "文本生成图片";
  if (normalized.includes("IMAGE_TO_VIDEO")) return "图片生成视频";
  if (normalized.includes("TEXT_TO_VIDEO")) return "文本生成视频";
  return value;
}

function fileFormat(item: AssetItem) {
  const contentType = stringMetadata(item.metadata || {}, "contentType", "mimeType");
  if (contentType.includes("/")) return contentType.split("/").pop()?.toUpperCase() || "";
  const source = item.remoteUrl || item.thumbnailUrl;
  const match = source.match(/\.([a-z0-9]{2,6})(?:[?#]|$)/i);
  return match?.[1]?.toUpperCase() || "";
}

async function load() {
  if (!id.value || store.currentLoading) return null;
  notFound.value = false;
  previewImageLoading.value = true;
  previewImageError.value = false;
  try {
    const result = await store.loadCurrentAsset(id.value);
    if (result) cacheCreationDraft(result);
    armPreviewLoadingTimeout();
    showPreviewTipOnce(result?.type);
    return result;
  } catch (error) {
    const status = Number((error as { status?: number; statusCode?: number })?.status || (error as { statusCode?: number })?.statusCode || 0);
    notFound.value = status === 404 || /not found|不存在|无权/i.test(error instanceof Error ? error.message : String(error));
    return null;
  }
}

function showPreviewTipOnce(type?: string) {
  if (type !== "image" && type !== "infographic") return;
  const key = "zhiqiyun:asset-detail:preview-tip-v1";
  if (uni.getStorageSync(key)) return;
  uni.setStorageSync(key, true);
  setTimeout(() => uni.showToast({ title: "点击作品可全屏查看", icon: "none", duration: 1600 }), 350);
}

function handlePreviewLoad() {
  clearPreviewLoadingTimer();
  previewImageLoading.value = false;
  previewImageError.value = false;
}

function handlePreviewError() {
  clearPreviewLoadingTimer();
  previewImageLoading.value = false;
  previewImageError.value = true;
}

async function reloadPreview() {
  if (activeAction.value) return;
  activeAction.value = "preview";
  try {
    await store.loadCurrentAsset(id.value);
    previewReloadKey.value += 1;
    previewImageLoading.value = true;
    previewImageError.value = false;
    armPreviewLoadingTimeout();
  } catch (error) {
    uni.showToast({ title: errorMessage(error, "重新加载失败"), icon: "none" });
  } finally {
    activeAction.value = "";
  }
}

function preview() {
  if (!asset.value || previewImageError.value) return;
  previewAsset(asset.value);
}

async function download() {
  if (!asset.value || activeAction.value) return;
  activeAction.value = "download";
  try {
    await downloadAssetFile(asset.value);
  } catch (error) {
    const message = errorMessage(error, "保存失败");
    if (!/取消/.test(message)) uni.showToast({ title: message, icon: "none" });
  } finally {
    activeAction.value = "";
  }
}

function copyPrompt() {
  if (!asset.value?.prompt) return;
  copyText(asset.value.prompt, "提示词已复制");
}

function creationMode(item: AssetItem): CreationMode {
  if (item.type === "video") return "video";
  if (item.type === "ppt") return "ppt";
  if (item.type === "agent" || item.type === "knowledge") return "agent";
  if (item.type === "infographic") return "infographic";
  return "image";
}

function creationPage(mode: CreationMode) {
  return ({
    image: "UserImageCreationPage",
    video: "UserVideoCreationPage",
    ppt: "UserPptCreationPage",
    agent: "UserAgentCreationPage",
    infographic: "UserInfographicCreationPage",
  } as Record<CreationMode, string>)[mode];
}

function buildCreationDraft(item: AssetItem, intent: "edit" | "regenerate") {
  const metadata = item.metadata || {};
  const mode = creationMode(item);
  const originalReferences = stringArrayMetadata(metadata, "referenceImages", "inputImagesSnapshot", "inputImages", "reference_urls");
  const editableOutput = intent === "edit" && ["image", "infographic"].includes(mode)
    ? item.remoteUrl || item.thumbnailUrl
    : "";
  const referencePaths = [editableOutput, ...originalReferences]
    .filter((value, index, values) => Boolean(value) && values.indexOf(value) === index)
    .slice(0, 3);
  return {
    mode,
    intent,
    sourceAssetId: item.id,
    sourceTaskId: item.taskId || "",
    prompt: item.prompt || "",
    referencePaths,
    model: item.model || stringMetadata(metadata, "model", "modelName"),
    aspectRatio: item.aspectRatio || stringMetadata(metadata, "aspectRatio", "aspect_ratio", "ratio"),
    size: stringMetadata(metadata, "size"),
    resolution: item.width && item.height ? `${item.width}x${item.height}` : stringMetadata(metadata, "resolution"),
    quality: stringMetadata(metadata, "quality", "imageQuality"),
    style: stringMetadata(metadata, "style", "stylePreset"),
    seed: item.seed ?? metadata.seed ?? "",
    count: stringMetadata(metadata, "count", "imageCount", "generationCount"),
  };
}

function cacheCreationDraft(item: AssetItem) {
  uni.setStorageSync(assetDetailDraftKey, buildCreationDraft(item, "edit"));
}

function persistCreationDraft(item: AssetItem, intent: "edit" | "regenerate") {
  const draft = buildCreationDraft(item, intent);
  uni.setStorageSync(assetDetailDraftKey, draft);
  uni.setStorageSync("v531-creation-prompt", draft.prompt);
  uni.setStorageSync("v532-studio-draft", draft);
}

function openCreation(intent: "edit" | "regenerate") {
  if (!asset.value || activeAction.value) return;
  if (intent === "regenerate" && !asset.value.prompt) {
    uni.showToast({ title: "作品缺少原提示词，暂时无法再次生成", icon: "none" });
    return;
  }
  const item = asset.value;
  const mode = creationMode(item);
  persistCreationDraft(item, intent);
  activeAction.value = intent;
  uni.navigateTo({
    url: `/pages/user/${creationPage(mode)}?assetId=${encodeURIComponent(item.id)}&intent=${intent}`,
    fail: () => uni.showToast({ title: "创作页面打开失败", icon: "none" }),
    complete: () => { activeAction.value = ""; },
  });
}

function continueEditing() {
  openCreation("edit");
}

function regenerate() {
  openCreation("regenerate");
}

async function toggleFavorite() {
  if (!asset.value || activeAction.value) return;
  activeAction.value = "favorite";
  try {
    await store.toggleFavorite(asset.value.id);
  } catch {
    // The store restores optimistic state and displays the API error.
  } finally {
    activeAction.value = "";
  }
}

async function move() {
  if (!asset.value || activeAction.value) return;
  activeAction.value = "move";
  try {
    await store.loadProjects();
    const canRemove = Boolean(asset.value.projectId || asset.value.projectName);
    const options = [...(canRemove ? ["移出当前项目"] : []), ...store.projects.map(item => item.name)];
    if (!options.length) {
      uni.showToast({ title: "暂无可移动的项目", icon: "none" });
      return;
    }
    uni.showActionSheet({
      itemList: options,
      success: result => {
        const removeOffset = canRemove ? 1 : 0;
        if (canRemove && result.tapIndex === 0) {
          void moveToProject("", "");
          return;
        }
        const project = store.projects[result.tapIndex - removeOffset];
        if (project) void moveToProject(project.id, project.name);
      },
    });
  } catch (error) {
    uni.showToast({ title: errorMessage(error, "项目加载失败"), icon: "none" });
  } finally {
    activeAction.value = "";
  }
}

async function moveToProject(projectId: string, projectName: string) {
  if (!asset.value || activeAction.value) return;
  activeAction.value = "move";
  try {
    await store.moveToProject(asset.value.id, projectId, projectName);
    uni.showToast({ title: projectId ? "已移动到项目" : "已移出项目", icon: "success" });
  } catch (error) {
    uni.showToast({ title: errorMessage(error, "移动失败"), icon: "none" });
  } finally {
    activeAction.value = "";
  }
}

function rename() {
  if (!asset.value || activeAction.value) return;
  uni.showModal({
    title: "重命名作品",
    editable: true,
    content: displayTitle.value,
    placeholderText: "请输入作品名称",
    success: result => {
      const name = String(result.content || "").trim();
      if (result.confirm && name) void confirmRename(name);
    },
  });
}

async function confirmRename(name: string) {
  if (!asset.value || activeAction.value) return;
  activeAction.value = "rename";
  try {
    await store.renameAsset(asset.value.id, name);
    uni.showToast({ title: "作品名称已更新", icon: "success" });
  } catch (error) {
    uni.showToast({ title: errorMessage(error, "重命名失败"), icon: "none" });
  } finally {
    activeAction.value = "";
  }
}

async function archive() {
  if (!asset.value || activeAction.value) return;
  activeAction.value = "archive";
  try {
    await store.archiveAsset(asset.value.id);
    uni.showToast({ title: "作品已归档", icon: "success" });
  } catch (error) {
    uni.showToast({ title: errorMessage(error, "归档失败"), icon: "none" });
  } finally {
    activeAction.value = "";
  }
}

function remove() {
  if (!asset.value || activeAction.value) return;
  uni.showModal({
    title: "删除作品",
    content: "作品将移入回收站并从当前列表移除，是否继续？",
    confirmText: "删除",
    confirmColor: "#d64545",
    success: result => { if (result.confirm) void confirmRemove(); },
  });
}

async function confirmRemove() {
  if (!asset.value || activeAction.value) return;
  activeAction.value = "delete";
  try {
    await store.deleteAsset(asset.value.id);
    uni.showToast({ title: "作品已删除", icon: "success" });
    setTimeout(() => backOrHome("/pages/user/UserAssetsPage"), 450);
  } catch (error) {
    uni.showToast({ title: errorMessage(error, "删除失败"), icon: "none" });
  } finally {
    activeAction.value = "";
  }
}

function handleManagementAction(action: string) {
  moreVisible.value = false;
  if (action === "favorite") void toggleFavorite();
  else if (action === "move") void move();
  else if (action === "rename") rename();
  else if (action === "archive") void archive();
  else if (action === "delete") remove();
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function formatBytes(value: number) {
  if (!value) return "";
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB`;
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${value} B`;
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}

function clearPreviewLoadingTimer() {
  if (previewLoadingTimer) clearTimeout(previewLoadingTimer);
  previewLoadingTimer = null;
}

function armPreviewLoadingTimeout() {
  clearPreviewLoadingTimer();
  previewLoadingTimer = setTimeout(() => {
    if (!previewImageError.value) previewImageLoading.value = false;
    previewLoadingTimer = null;
  }, 1800);
}

function syncNavigationInsets() {
  try {
    const system = uni.getSystemInfoSync() as { statusBarHeight?: number; windowWidth?: number };
    const capsule = typeof uni.getMenuButtonBoundingClientRect === "function" ? uni.getMenuButtonBoundingClientRect() : null;
    const statusBarHeight = Math.max(0, Number(system.statusBarHeight || 0));
    const rightInset = capsule && system.windowWidth ? Math.max(88, system.windowWidth - capsule.left + 8) : 92;
    navigationStyle.value = { paddingTop: `${statusBarHeight}px`, paddingRight: `${rightInset}px` };
  } catch {
    navigationStyle.value = { paddingTop: "env(safe-area-inset-top)", paddingRight: "92px" };
  }
}

let disposeNativeBridge = () => {};

function installNativeBridge() {
  disposeNativeBridge();
  disposeNativeBridge = registerAssetNativeBridge({
    previewCurrentAsset: preview,
    downloadCurrentAsset: () => { void download(); },
    editCurrentAsset: continueEditing,
    regenerateCurrentAsset: regenerate,
    copyCurrentPrompt: copyPrompt,
  });
}

installNativeBridge();
onShow(() => {
  activeAction.value = "";
  moreVisible.value = false;
  installNativeBridge();
});
onMounted(syncNavigationInsets);
onBeforeUnmount(() => {
  clearPreviewLoadingTimer();
  disposeNativeBridge();
});

watch(() => props.assetId, value => {
  const nextId = String(value || "").trim();
  if (!nextId || nextId === id.value) return;
  id.value = nextId;
  previewPage.value = 1;
  promptExpanded.value = false;
  parametersExpanded.value = false;
  assetInfoExpanded.value = false;
  void load();
}, { immediate: true });

onShareAppMessage(() => ({
  title: displayTitle.value || "知启云 AI 作品",
  path: `/pages/user/UserAssetDetailPage?id=${encodeURIComponent(id.value)}`,
  imageUrl: asset.value?.thumbnailUrl || asset.value?.remoteUrl || undefined,
}));
</script>

<style scoped>
.detail-page { min-height: 100vh; box-sizing: border-box; color: #182033; background: #f5f7fc; }
.detail-header { position: sticky; z-index: 30; top: 0; display: flex; min-height: 48px; box-sizing: content-box; padding-left: 16px; align-items: center; gap: 12px; border-bottom: 1px solid rgba(224,228,240,.8); background: rgba(248,250,255,.96); }
.header-title { overflow: hidden; font-size: 18px; font-weight: 650; letter-spacing: 0; text-overflow: ellipsis; white-space: nowrap; }
.detail-content { padding: 0 16px calc(28px + env(safe-area-inset-bottom)); }
.back-button,.preview-error button,.ppt-controls button,.primary-actions button,.utility-actions button,.card-head button,.collapse-trigger,.expand-button,.empty-action { margin: 0; border: 0; }
.back-button::after,.preview-error button::after,.ppt-controls button::after,.primary-actions button::after,.utility-actions button::after,.card-head button::after,.collapse-trigger::after,.expand-button::after,.empty-action::after { display: none; }
.back-button { display: grid; width: 36px; height: 36px; flex: 0 0 36px; padding: 0; place-items: center; border-radius: 12px; color: #4f5fd6; background: #edf0ff; font-size: 28px; line-height: 34px; }
.preview-card { margin-top: 12px; padding: 8px; border: 1px solid #e2e6f0; border-radius: 16px; background: #fff; }
.cover-preview { position: relative; width: 100%; aspect-ratio: 1 / 1; overflow: hidden; border-radius: 16px; background: #eef1f6; }
.preview-image { display: block; width: 100%; height: 100%; background: #eef1f6; }
.preview-skeleton,.preview-error { position: absolute; z-index: 2; inset: 0; display: flex; align-items: center; justify-content: center; }
.preview-skeleton { color: #8b93a8; background: linear-gradient(90deg,#eef1f6 25%,#f9faff 50%,#eef1f6 75%); background-size: 200% 100%; font-size: 12px; animation: preview-loading 1.25s infinite; }
.preview-error { padding: 24px; box-sizing: border-box; flex-direction: column; text-align: center; background: #f1f3f8; }
.preview-error-symbol { display: grid; width: 38px; height: 38px; place-items: center; border-radius: 12px; color: #6b74c9; background: #e4e8ff; font-size: 18px; font-weight: 700; }
.preview-error-title { margin-top: 12px; color: #2c3448; font-size: 15px; font-weight: 650; }
.preview-error-copy { max-width: 240px; margin-top: 6px; color: #858da1; font-size: 12px; line-height: 18px; }
.preview-error button { width: auto; height: 36px; margin-top: 15px; padding: 0 16px; border-radius: 12px; color: #fff; background: #5b6ee1; font-size: 12px; }
.video-preview { display: block; width: 100%; aspect-ratio: 16 / 9; border-radius: 16px; background: #10131d; }
.document-preview { position: relative; width: 100%; min-height: 250px; padding: 8px; box-sizing: border-box; overflow: hidden; border-radius: 16px; background: #eef1f6; }
.document-preview :deep(.asset-cover-shell) { height: 230px; }
.prompt-preview,.entity-preview { display: flex; min-height: 250px; padding: 24px; box-sizing: border-box; flex-direction: column; align-items: flex-start; justify-content: center; border-radius: 16px; background: #f1f2ff; }
.preview-kicker { color: #5b6ee1; font-size: 12px; font-weight: 650; }
.prompt-preview > text:last-child { margin-top: 12px; color: #343b50; font-size: 14px; line-height: 22px; }
.entity-preview { align-items: center; text-align: center; }
.entity-preview.knowledge { background: #edf8f4; }
.entity-symbol { display: grid; width: 60px; height: 60px; place-items: center; border-radius: 16px; color: #fff; background: #6577e8; font-size: 24px; font-weight: 700; }
.knowledge .entity-symbol { background: #249477; }
.entity-title { max-width: 100%; margin: 13px 0 8px; overflow: hidden; font-size: 17px; font-weight: 650; text-overflow: ellipsis; white-space: nowrap; }
.ppt-controls { display: flex; margin-top: 9px; align-items: center; justify-content: center; gap: 14px; }
.ppt-controls button { width: auto; height: 30px; padding: 0 10px; border-radius: 10px; color: #4f5fd6; background: #edf0ff; font-size: 11px; }
.ppt-controls text { color: #767f94; font-size: 11px; }
.identity-section { padding: 16px 2px 2px; }
.asset-heading { display: flex; align-items: flex-start; gap: 10px; }
.asset-title { display: -webkit-box; min-width: 0; flex: 1; overflow: hidden; color: #151b2b; font-size: 19px; font-weight: 680; line-height: 26px; -webkit-box-orient: vertical; -webkit-line-clamp: 2; overflow-wrap: anywhere; }
.asset-subtitle { display: block; margin-top: 6px; color: #858da1; font-size: 12px; line-height: 18px; }
.primary-actions { display: grid; margin-top: 14px; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 10px; }
.primary-actions button { display: flex; height: 46px; padding: 0 12px; align-items: center; justify-content: center; gap: 7px; border-radius: 14px; font-size: 14px; font-weight: 650; }
.primary-button { color: #fff; background: #5b6ee1; box-shadow: 0 7px 16px rgba(91,110,225,.18); }
.regenerate-button { border: 1px solid #cfd5ff !important; color: #4f5fd6; background: #f0f2ff; }
.primary-actions button[disabled],.utility-actions button[disabled] { opacity: .55; }
.action-icon { font-size: 17px; font-weight: 500; }
.utility-actions { display: grid; margin-top: 10px; grid-template-columns: repeat(3,minmax(0,1fr)); gap: 8px; }
.utility-button { display: flex; height: 42px; padding: 0 5px; align-items: center; justify-content: center; gap: 6px; border: 1px solid #e0e4ef !important; border-radius: 13px; color: #5d667b; background: #fff; font-size: 12px; }
.utility-icon { color: #6676dc; font-size: 16px; line-height: 1; }
.more-icon { font-size: 13px; letter-spacing: 0; }
.content-card { margin-top: 12px; padding: 16px; border: 1px solid #e1e5ef; border-radius: 16px; background: #fff; box-shadow: 0 3px 12px rgba(41,51,82,.025); }
.card-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.card-title { color: #252d40; font-size: 15px; font-weight: 650; line-height: 22px; }
.copy-button { width: auto; height: 30px; padding: 0 11px; border-radius: 10px; color: #4f5fd6; background: #eef1ff; font-size: 12px; }
.prompt-copy { display: block; margin-top: 11px; color: #4f586d; font-size: 13px; line-height: 21px; white-space: pre-wrap; overflow-wrap: anywhere; }
.prompt-copy.collapsed { display: -webkit-box; overflow: hidden; -webkit-box-orient: vertical; -webkit-line-clamp: 4; }
.expand-button { width: auto; height: 28px; margin-top: 6px; padding: 0; color: #5b6ee1; background: transparent; font-size: 12px; line-height: 28px; }
.negative-prompt { margin-top: 12px; padding-top: 12px; border-top: 1px solid #edf0f5; }
.negative-prompt text { display: block; color: #747d91; font-size: 12px; line-height: 19px; }
.negative-prompt .field-label { margin-bottom: 5px; color: #3c4559; font-weight: 650; }
.collapsible-card { padding: 0 16px; }
.collapse-trigger { display: grid; width: 100%; min-height: 52px; padding: 0; grid-template-columns: minmax(0,1fr) auto 18px; align-items: center; gap: 8px; text-align: left; background: transparent; }
.collapse-meta { color: #99a0b1; font-size: 11px; }
.chevron { color: #7b8498; font-size: 17px; line-height: 1; text-align: center; transform: rotate(0); transition: transform .18s ease; }
.chevron.expanded { transform: rotate(180deg); }
.compact-list { padding-bottom: 8px; border-top: 1px solid #edf0f5; }
.detail-row { display: flex; min-height: 42px; padding: 4px 0; box-sizing: border-box; align-items: center; justify-content: space-between; gap: 16px; border-bottom: 1px solid #f0f2f6; font-size: 12px; }
.detail-row:last-child { border-bottom: 0; }
.detail-row text:first-child { flex: 0 0 auto; color: #8991a4; }
.detail-row text:last-child { min-width: 0; max-width: 68%; color: #384156; text-align: right; overflow-wrap: anywhere; }
.empty-panel { display: flex; min-height: 340px; margin-top: 12px; padding: 28px; box-sizing: border-box; flex-direction: column; align-items: center; justify-content: center; border: 1px solid #e2e6f0; border-radius: 16px; background: #fff; text-align: center; }
.empty-symbol { display: grid; width: 48px; height: 48px; place-items: center; border-radius: 15px; color: #6577e8; background: #eef1ff; font-size: 23px; }
.empty-title { margin-top: 14px; color: #252d40; font-size: 16px; font-weight: 650; }
.empty-copy { max-width: 250px; margin-top: 7px; color: #858da1; font-size: 12px; line-height: 19px; }
.empty-action { width: auto; height: 38px; margin-top: 16px; padding: 0 17px; border-radius: 13px; color: #fff; background: #5b6ee1; font-size: 12px; }
@keyframes preview-loading { to { background-position: -200% 0; } }
</style>
