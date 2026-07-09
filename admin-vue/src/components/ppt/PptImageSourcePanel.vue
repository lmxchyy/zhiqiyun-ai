<template>
  <section class="ppt-image-editor-panel">
    <header class="ppt-image-editor-header">
      <div>
        <strong>图片编辑</strong>
        <span>{{ currentSlide ? currentSlide.title : "请先选择幻灯片" }}</span>
      </div>
      <span class="ppt-image-header-actions">
        <button
          type="button"
          :disabled="!currentSlide?.imageUrl"
          :title="currentSlide?.imageUrl ? '清除当前页图片' : '当前页没有图片可清除'"
          :aria-label="currentSlide?.imageUrl ? '清除当前页图片' : '当前页没有图片可清除'"
          @click="clearCurrentImage"
        >
          清除
        </button>
        <button type="button" title="关闭图片面板" aria-label="关闭图片面板" @click="$emit('close')">
          <svg class="ppt-image-icon" viewBox="0 0 24 24" aria-hidden="true">
            <path d="M18 6 6 18" />
            <path d="m6 6 12 12" />
          </svg>
        </button>
      </span>
    </header>

    <div class="ppt-image-preview" :class="{ empty: !currentSlide?.imageUrl }">
      <img v-if="currentSlide?.imageUrl" :src="currentSlide.imageUrl" :alt="`${currentSlide.title} 配图`" />
      <div v-else>
        <svg class="ppt-image-icon" viewBox="0 0 24 24" aria-hidden="true">
          <path d="M3 5h18v14H3z" />
          <path d="m21 16-5-5L5 19" />
          <path d="M8 10h.01" />
        </svg>
        <span>当前页暂无图片</span>
      </div>
    </div>

    <div class="ppt-image-source-tabs" role="tablist" aria-label="图片来源">
      <button
        v-for="item in imageSourceOptions"
        :key="item.value"
        type="button"
        role="tab"
        :aria-selected="imageSource === item.value"
        :title="`切换图片来源：${item.label}`"
        :aria-label="`切换图片来源：${item.label}`"
        :class="{ active: imageSource === item.value }"
        @click="$emit('update:image-source', item.value)"
      >
        <svg class="ppt-image-icon" viewBox="0 0 24 24" aria-hidden="true">
          <template v-if="item.value === 'ai'">
            <path d="M9.9 2.8 8.8 6.2 5.4 7.3l3.4 1.1 1.1 3.4 1.1-3.4 3.4-1.1-3.4-1.1z" />
            <path d="m18 10 1 3 3 1-3 1-1 3-1-3-3-1 3-1z" />
          </template>
          <template v-else-if="item.value === 'stock'">
            <path d="m21 21-4.34-4.34" />
            <circle cx="11" cy="11" r="8" />
          </template>
          <template v-else>
            <path d="M4 4h16v16H4z" />
            <path d="m8 8 8 8" />
          </template>
        </svg>
        <span>{{ item.label }}</span>
      </button>
    </div>

    <div class="ppt-image-mode-tabs" role="tablist" aria-label="图片编辑模式">
      <button
        v-for="item in modeTabs"
        :key="item.value"
        type="button"
        role="tab"
        :aria-selected="activeMode === item.value"
        :title="`切换图片编辑模式：${item.label}`"
        :aria-label="`切换图片编辑模式：${item.label}`"
        :class="{ active: activeMode === item.value }"
        @click="selectMode(item.value)"
      >
        <svg class="ppt-image-icon" viewBox="0 0 24 24" aria-hidden="true">
          <path v-if="item.icon === 'ai'" d="M9.9 2.8 8.8 6.2 5.4 7.3l3.4 1.1 1.1 3.4 1.1-3.4 3.4-1.1-3.4-1.1zM18 10l1 3 3 1-3 1-1 3-1-3-3-1 3-1z" />
          <path v-else-if="item.icon === 'upload'" d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M17 8l-5-5-5 5M12 3v12" />
          <path v-else-if="item.icon === 'spark'" d="M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8z" />
          <path v-else-if="item.icon === 'search'" d="m21 21-4.34-4.34M11 19a8 8 0 1 1 0-16 8 8 0 0 1 0 16Z" />
          <path v-else-if="item.icon === 'gif'" d="M4 5h16v14H4zM8 9h3M8 9v6M11 12H9.5M14 9v6M17 9v6M17 9h-3" />
          <path v-else d="M10 13a5 5 0 0 0 7 0l2-2a5 5 0 0 0-7-7l-1 1M14 11a5 5 0 0 0-7 0l-2 2a5 5 0 0 0 7 7l1-1" />
        </svg>
        <span>{{ item.label }}</span>
      </button>
    </div>

    <section v-if="activeMode === 'generate'" class="ppt-image-mode-panel">
      <label>
        <span>生成提示词</span>
        <textarea
          v-model="generatePrompt"
          title="当前页图片生成提示词"
          aria-label="当前页图片生成提示词"
          placeholder="描述你希望放在当前页的配图，例如：企业数字员工在协同办公场景中工作"
        ></textarea>
      </label>
      <label>
        <span>图片模型</span>
        <select
          :value="imageModel"
          title="选择图片生成模型"
          aria-label="选择图片生成模型"
          @change="$emit('update:image-model', ($event.target as HTMLSelectElement).value)"
        >
          <option v-for="model in imageModels" :key="model.value" :value="model.value">
            {{ model.label }}
          </option>
        </select>
      </label>
      <button
        class="ppt-image-primary"
        type="button"
        :disabled="generating || !currentSlide"
        :aria-busy="generating ? 'true' : undefined"
        :title="generating ? '图片正在生成' : currentSlide ? '生成当前页图片' : '请先选择幻灯片'"
        :aria-label="generating ? '图片正在生成' : currentSlide ? '生成当前页图片' : '请先选择幻灯片'"
        @click="generateCurrentImage"
      >
        <span v-if="generating" class="ppt-image-spinner"></span>
        <span>{{ generating ? "图片生成中..." : "生成当前页图片" }}</span>
      </button>
    </section>

    <section v-else-if="activeMode === 'upload'" class="ppt-image-mode-panel">
      <input ref="fileInputRef" type="file" accept="image/*" hidden aria-hidden="true" @change="handleUpload" />
      <button class="ppt-image-primary" type="button" title="上传本地图片" aria-label="上传本地图片" @click="fileInputRef?.click()">
        上传图片
      </button>
      <div v-if="uploadedImages.length" class="ppt-image-grid" role="listbox" aria-label="已上传图片">
        <button
          v-for="image in uploadedImages"
          :key="image.id"
          type="button"
          role="option"
          :class="{ active: isImageApplied(image) }"
          :aria-selected="isImageApplied(image)"
          :title="`应用上传图片：${image.title}`"
          :aria-label="`应用上传图片：${image.title}`"
          @click="applyImage(image)"
        >
          <img :src="image.url" :alt="image.title" />
          <span>{{ image.title }}</span>
        </button>
      </div>
      <p v-else>上传后的图片会先保存在当前浏览器会话里，后续可接入真实素材库。</p>
    </section>

    <section v-else-if="activeMode === 'generated'" class="ppt-image-mode-panel">
      <div v-if="generatedImages.length" class="ppt-image-grid" role="listbox" aria-label="已生成图片">
        <button
          v-for="image in generatedImages"
          :key="image.id"
          type="button"
          role="option"
          :class="{ active: isImageApplied(image) }"
          :aria-selected="isImageApplied(image)"
          :title="`应用已生成图片：${image.title}`"
          :aria-label="`应用已生成图片：${image.title}`"
          @click="applyImage(image)"
        >
          <img :src="image.url" :alt="image.title" />
          <span>{{ image.title }}</span>
        </button>
      </div>
      <p v-else>生成过的图片会显示在这里。可以先在“AI生成”里生成当前页图片。</p>
    </section>

    <section v-else-if="activeMode === 'search'" class="ppt-image-mode-panel">
      <label>
        <span>图库搜索</span>
        <div class="ppt-image-search-row">
          <input
            v-model="keyword"
            title="图片搜索关键词"
            aria-label="图片搜索关键词"
            placeholder="输入图片关键词，例如：企业增长"
            @keydown.enter.prevent="searchCurrentKeyword"
          />
          <button type="button" title="搜索图片" aria-label="搜索图片" @click="searchCurrentKeyword">搜索</button>
        </div>
      </label>
      <div v-if="results.length" class="ppt-image-grid" role="listbox" aria-label="图片搜索结果">
        <button
          v-for="image in results"
          :key="image.id"
          type="button"
          role="option"
          :class="{ active: isImageApplied(image) }"
          :aria-selected="isImageApplied(image)"
          :title="`应用搜索图片：${image.title}`"
          :aria-label="`应用搜索图片：${image.title}`"
          @click="applyImage(image)"
        >
          <img :src="image.url" :alt="image.title" />
          <span>{{ image.title }}</span>
        </button>
      </div>
      <p v-else>搜索结果会以卡片形式展示，点击即可应用到当前页。</p>
    </section>

    <section v-else-if="activeMode === 'gif'" class="ppt-image-mode-panel">
      <div class="ppt-image-grid" role="listbox" aria-label="GIF 动图">
        <button
          v-for="image in gifImages"
          :key="image.id"
          type="button"
          role="option"
          :class="{ active: isImageApplied(image) }"
          :aria-selected="isImageApplied(image)"
          :title="`应用 GIF 动图：${image.title}`"
          :aria-label="`应用 GIF 动图：${image.title}`"
          @click="applyImage(image)"
        >
          <img :src="image.url" :alt="image.title" />
          <span>{{ image.title }}</span>
        </button>
      </div>
    </section>

    <section v-else class="ppt-image-mode-panel">
      <label>
        <span>图片或媒体链接</span>
        <input
          v-model="embedUrl"
          title="图片或媒体链接"
          aria-label="图片或媒体链接"
          placeholder="粘贴图片 URL、数据图或媒体封面链接"
        />
      </label>
      <button
        class="ppt-image-primary"
        type="button"
        :disabled="!embedUrl.trim()"
        :title="embedUrl.trim() ? '应用图片或媒体链接' : '请先粘贴图片或媒体链接'"
        :aria-label="embedUrl.trim() ? '应用图片或媒体链接' : '请先粘贴图片或媒体链接'"
        @click="applyEmbedUrl"
      >
        应用链接
      </button>
      <p>当前先按图片地址应用到画布，视频/网页 iframe 后续接入媒体服务。</p>
    </section>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { ElMessage } from "element-plus/es/components/message/index";
import type { PptImageOption, PptImageSource, PptModelOption, PptSlide } from "../../types/ppt";

type ImageMode = "generate" | "upload" | "generated" | "search" | "gif" | "embed";

const props = defineProps<{
  imageSource: PptImageSource;
  imageModel: string;
  imageModels: PptModelOption[];
  currentSlide: PptSlide | null;
  results: PptImageOption[];
  generating: boolean;
}>();

const emit = defineEmits<{
  "update:image-source": [value: PptImageSource];
  "update:image-model": [value: string];
  "generate-image": [];
  "search-images": [keyword: string];
  "apply-image": [image: PptImageOption];
  close: [];
}>();

const imageSourceOptions = [
  { label: "AI生成", value: "ai" },
  { label: "图库", value: "stock" },
  { label: "无图", value: "none" }
] satisfies Array<{ label: string; value: PptImageSource }>;

const modeTabs = [
  { label: "AI生成", value: "generate", icon: "ai" },
  { label: "上传", value: "upload", icon: "upload" },
  { label: "已生成", value: "generated", icon: "spark" },
  { label: "搜索", value: "search", icon: "search" },
  { label: "GIF", value: "gif", icon: "gif" },
  { label: "嵌入", value: "embed", icon: "embed" }
] satisfies Array<{ label: string; value: ImageMode; icon: string }>;

const activeMode = ref<ImageMode>(props.imageSource === "stock" ? "search" : "generate");
const keyword = ref("");
const embedUrl = ref("");
const fileInputRef = ref<HTMLInputElement | null>(null);
const uploadedImages = ref<PptImageOption[]>([]);
const generatedImageCache = ref<PptImageOption[]>([]);

const generatePrompt = ref("");

const defaultPrompt = computed(() => {
  if (!props.currentSlide) return "";
  return `${props.currentSlide.title}，${props.currentSlide.content}`.slice(0, 140);
});

watch(
  () => props.currentSlide?.id,
  () => {
    generatePrompt.value = defaultPrompt.value;
  },
  { immediate: true }
);

watch(
  () => props.currentSlide?.imageUrl,
  (url) => {
    if (!url || !props.currentSlide) return;
    const exists = generatedImageCache.value.some((item) => item.url === url);
    if (!exists) {
      generatedImageCache.value = [{
        id: `current_${props.currentSlide.id}_${Date.now()}`,
        title: `${props.currentSlide.title} 当前图片`,
        source: props.imageSource === "ai" ? "ai" as const : "stock" as const,
        url
      }, ...generatedImageCache.value].slice(0, 8);
    }
  }
);

const generatedImages = computed(() => generatedImageCache.value.filter((item) => item.url));
const gifImages = computed<PptImageOption[]>(() => [
  { id: "gif_motion_1", title: "数据增长动效", source: "stock", url: mockPanelImageUrl("数据增长动效", ["#08111f", "#2563eb", "#38bdf8"]) },
  { id: "gif_motion_2", title: "团队协同动效", source: "stock", url: mockPanelImageUrl("团队协同动效", ["#0f172a", "#16a34a", "#86efac"]) },
  { id: "gif_motion_3", title: "产品演示动效", source: "stock", url: mockPanelImageUrl("产品演示动效", ["#111827", "#8b5cf6", "#22d3ee"]) }
]);

function selectMode(mode: ImageMode) {
  activeMode.value = mode;
  if (mode === "generate") emit("update:image-source", "ai");
  if (mode === "search" || mode === "gif" || mode === "embed" || mode === "upload") emit("update:image-source", "stock");
}

function generateCurrentImage() {
  emit("update:image-source", "ai");
  emit("generate-image");
}

function searchCurrentKeyword() {
  const nextKeyword = keyword.value.trim() || props.currentSlide?.title || "PPT";
  keyword.value = nextKeyword;
  emit("update:image-source", "stock");
  emit("search-images", nextKeyword);
}

function applyImage(image: PptImageOption) {
  emit("apply-image", image);
  ElMessage.success("已应用图片到当前页");
}

function isImageApplied(image: PptImageOption) {
  return Boolean(image.url && props.currentSlide?.imageUrl === image.url);
}

function clearCurrentImage() {
  emit("apply-image", { id: "clear_image", title: "清除图片", source: "stock", url: "" });
  ElMessage.success("已清除当前页图片");
}

function applyEmbedUrl() {
  const url = embedUrl.value.trim();
  if (!url) return;
  emit("update:image-source", "stock");
  applyImage({ id: `embed_${Date.now()}`, title: "嵌入图片链接", source: "stock", url });
}

function handleUpload(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0];
  if (!file) return;
  const url = URL.createObjectURL(file);
  const image: PptImageOption = {
    id: `upload_${Date.now()}`,
    title: file.name,
    source: "stock",
    url
  };
  uploadedImages.value = [image, ...uploadedImages.value].slice(0, 8);
  applyImage(image);
  if (fileInputRef.value) fileInputRef.value.value = "";
}

onBeforeUnmount(() => {
  uploadedImages.value.forEach((image) => {
    if (image.url.startsWith("blob:")) URL.revokeObjectURL(image.url);
  });
});

function mockPanelImageUrl(label: string, palette: string[]) {
  const [background, accent, highlight] = palette;
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="960" height="540" viewBox="0 0 960 540">
    <defs>
      <linearGradient id="bg" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0%" stop-color="${background}"/>
        <stop offset="60%" stop-color="${accent}"/>
        <stop offset="100%" stop-color="${highlight}"/>
      </linearGradient>
    </defs>
    <rect width="960" height="540" rx="36" fill="url(#bg)"/>
    <circle cx="760" cy="126" r="120" fill="#fff" opacity=".15"/>
    <rect x="96" y="132" width="430" height="256" rx="28" fill="#fff" opacity=".18"/>
    <path d="M132 334 248 238l82 72 68-54 102 78Z" fill="#fff" opacity=".58"/>
    <text x="96" y="458" fill="#fff" font-family="Arial, sans-serif" font-size="36" font-weight="700">${escapeSvg(label.slice(0, 24))}</text>
  </svg>`;
  return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
}

function escapeSvg(value: string) {
  return value.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}
</script>

<style scoped>
.ppt-image-editor-panel {
  display: grid;
  gap: 12px;
  min-width: 0;
}

.ppt-image-editor-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
}

.ppt-image-editor-header div {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.ppt-image-editor-header strong {
  color: #f4f4f5;
}

.ppt-image-editor-header span,
.ppt-image-mode-panel p {
  color: #a1a1aa;
  font-size: 12px;
  line-height: 1.5;
}

.ppt-image-header-actions {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.ppt-image-editor-header button,
.ppt-image-source-tabs button,
.ppt-image-mode-tabs button,
.ppt-image-mode-panel button,
.ppt-image-search-row button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  min-height: 34px;
  border: 1px solid #333;
  border-radius: 8px;
  color: #f4f4f5;
  background: #151515;
  cursor: pointer;
  font-weight: 760;
}

.ppt-image-editor-header button {
  padding: 0 10px;
}

.ppt-image-header-actions button:last-child {
  width: 34px;
  padding: 0;
  border-radius: 50%;
}

.ppt-image-editor-header button:disabled,
.ppt-image-mode-panel button:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.ppt-image-preview {
  display: grid;
  place-items: center;
  min-height: 148px;
  overflow: hidden;
  border: 1px solid #2b2b2b;
  border-radius: 10px;
  background: #0b0b0b;
}

.ppt-image-preview img {
  width: 100%;
  height: 100%;
  min-height: 148px;
  object-fit: cover;
}

.ppt-image-preview.empty div {
  display: grid;
  justify-items: center;
  gap: 8px;
  color: #71717a;
}

.ppt-image-source-tabs,
.ppt-image-mode-tabs {
  display: grid;
  gap: 6px;
}

.ppt-image-source-tabs {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.ppt-image-mode-tabs {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.ppt-image-source-tabs button,
.ppt-image-mode-tabs button {
  min-width: 0;
  padding: 0 8px;
  font-size: 12px;
}

.ppt-image-source-tabs button:hover,
.ppt-image-source-tabs button.active,
.ppt-image-mode-tabs button:hover,
.ppt-image-mode-tabs button.active {
  border-color: #555;
  background: #202020;
}

.ppt-image-mode-panel {
  display: grid;
  gap: 11px;
}

.ppt-image-mode-panel label {
  display: grid;
  gap: 6px;
}

.ppt-image-mode-panel label span {
  color: #d4d4d8;
  font-size: 12px;
  font-weight: 780;
}

.ppt-image-mode-panel textarea,
.ppt-image-mode-panel input,
.ppt-image-mode-panel select {
  width: 100%;
  min-height: 36px;
  padding: 8px 10px;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
  color: #f4f4f5;
  caret-color: #fff;
  background: #0d0d0d;
  outline: 0;
}

.ppt-image-mode-panel textarea {
  min-height: 82px;
  resize: vertical;
}

.ppt-image-mode-panel textarea:focus,
.ppt-image-mode-panel input:focus,
.ppt-image-mode-panel select:focus {
  border-color: #525252;
  box-shadow: 0 0 0 3px rgba(255, 255, 255, 0.08);
}

.ppt-image-search-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
}

.ppt-image-primary {
  min-height: 38px;
  color: #111 !important;
  border-color: #f4f4f5 !important;
  background: #f4f4f5 !important;
}

.ppt-image-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 9px;
}

.ppt-image-grid button {
  display: grid;
  justify-items: start;
  gap: 7px;
  min-height: auto;
  padding: 8px;
  text-align: left;
}

.ppt-image-grid button:hover,
.ppt-image-grid button.active {
  border-color: #6b7280;
  background: #202020;
}

.ppt-image-grid img {
  width: 100%;
  aspect-ratio: 16 / 10;
  object-fit: cover;
  border-radius: 6px;
}

.ppt-image-grid span {
  overflow: hidden;
  max-width: 100%;
  color: #d4d4d8;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-image-icon {
  width: 16px;
  height: 16px;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.ppt-image-spinner {
  width: 14px;
  height: 14px;
  border: 2px solid rgba(17, 17, 17, 0.28);
  border-top-color: #111;
  border-radius: 999px;
  animation: ppt-image-spin 0.8s linear infinite;
}

@keyframes ppt-image-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 900px) {
  .ppt-image-grid,
  .ppt-image-mode-tabs {
    grid-template-columns: 1fr;
  }
}
</style>
