<template>
  <section class="ppt-theme-settings-panel">
    <header class="ppt-theme-settings-header">
      <div>
        <h2>自定义主题</h2>
        <p>在生成完整 PPT 前选择视觉方向和图片来源。</p>
      </div>
    </header>

    <div class="ppt-theme-settings-section">
      <div class="ppt-theme-settings-title-row">
        <span>主题与版式</span>
        <button type="button" class="ppt-theme-more-button" :disabled="disabled" title="查看更多主题" aria-label="查看更多主题" @click="openThemeModal">
          更多主题
        </button>
      </div>
      <div class="ppt-theme-visible-grid">
        <article
          v-for="item in visibleThemes"
          :key="item.value"
          class="ppt-theme-preview-card"
          :class="{ active: modelValue === item.value }"
        >
          <button
            type="button"
            class="ppt-theme-preview-main"
            :aria-pressed="modelValue === item.value"
            :title="`选择主题：${item.label}`"
            :aria-label="`选择主题：${item.label}`"
            :disabled="disabled"
            @click="$emit('update:modelValue', item.value)"
          >
            <span class="ppt-theme-preview-canvas" :style="{ background: item.colors[1] }">
              <i :style="{ background: item.colors[0], borderColor: item.colors[2] }">
                <b :style="{ color: item.colors[1] }">标题</b>
                <em :style="{ background: item.colors[2] }"></em>
                <small :style="{ color: item.colors[1] }">正文</small>
              </i>
            </span>
            <span class="ppt-theme-preview-info">
              <strong>{{ item.label }}</strong>
              <small>{{ item.description }}</small>
            </span>
            <span v-if="modelValue === item.value" class="ppt-theme-selected-mark" aria-hidden="true">
              <svg viewBox="0 0 24 24">
                <path d="m5 13 4 4L19 7" />
              </svg>
            </span>
          </button>
          <button
            v-if="modelValue === item.value"
            type="button"
            class="ppt-theme-personalize-button"
            :disabled="disabled"
            title="个性化当前主题"
            aria-label="个性化当前主题"
            @click="openCreateThemeModal(true)"
          >
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="M12 20h9" />
              <path d="M12 4h9" />
              <path d="M4 9h16" />
              <path d="M4 15h16" />
              <circle cx="7" cy="4" r="2" />
              <circle cx="17" cy="15" r="2" />
            </svg>
            <span>个性化</span>
          </button>
        </article>
      </div>
    </div>

    <div class="ppt-theme-settings-section">
      <div class="ppt-theme-settings-title-row">
        <span>图片来源</span>
      </div>
      <div ref="imageSourceSelectRef" class="ppt-image-source-select">
        <button
          type="button"
          class="ppt-image-source-trigger"
          :class="{ open: isImageSourceOpen }"
          :aria-expanded="isImageSourceOpen"
          aria-haspopup="listbox"
          title="选择图片来源"
          aria-label="选择图片来源"
          :disabled="disabled"
          @click="toggleImageSourceSelect"
        >
          <span class="ppt-image-source-trigger-value">
            {{ imageSourceLabel }}
          </span>
          <svg class="ppt-image-source-chevron" viewBox="0 0 24 24" aria-hidden="true">
            <path d="m6 9 6 6 6-6" />
          </svg>
        </button>

      </div>

      <Teleport to="body">
        <Transition name="ppt-select-pop">
          <div
            v-if="isImageSourceOpen"
            ref="imageSourceMenuRef"
            class="ppt-image-source-menu"
            :class="`is-${imageSourcePlacement}`"
            :style="imageSourceMenuStyle"
            role="listbox"
            aria-label="图片来源"
          >
            <div class="ppt-image-source-group">
              <button
                type="button"
                role="option"
                class="ppt-image-source-option"
                :class="{ selected: imageSourceSelectValue === 'automatic' }"
                :aria-selected="imageSourceSelectValue === 'automatic'"
                @click="selectAutomaticImageSource"
              >
                Automatic
              </button>
            </div>

            <div class="ppt-image-source-group">
              <span class="ppt-image-source-group-label">
                <svg viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M9.9 2.8 8.8 6.2 5.4 7.3l3.4 1.1 1.1 3.4 1.1-3.4 3.4-1.1-3.4-1.1z" />
                  <path d="m18 10 1 3 3 1-3 1-1 3-1-3-3-1 3-1z" />
                </svg>
                AI Generation
              </span>
              <button
                v-for="model in imageModels"
                :key="model.value"
                type="button"
                role="option"
                class="ppt-image-source-option"
                :class="{ selected: imageSourceSelectValue === model.value }"
                :aria-selected="imageSourceSelectValue === model.value"
                :title="`选择 AI 图片模型：${model.label}`"
                @click="selectAiModel(model.value)"
              >
                {{ model.label }}
              </button>
            </div>

            <div class="ppt-image-source-group">
              <span class="ppt-image-source-group-label">
                <svg viewBox="0 0 24 24" aria-hidden="true">
                  <rect x="4" y="5" width="16" height="14" rx="2" />
                  <circle cx="9" cy="10" r="1.5" />
                  <path d="m4 15 4-4 4 4 2-2 6 6" />
                </svg>
                Stock & Web Images
              </span>
              <button
                v-for="provider in stockImageProviders"
                :key="provider.value"
                type="button"
                role="option"
                class="ppt-image-source-option"
                :class="{ selected: imageSourceSelectValue === provider.value }"
                :aria-selected="imageSourceSelectValue === provider.value"
                @click="selectStockImageProvider(provider.value)"
              >
                {{ provider.label }}
              </button>
            </div>

            <div class="ppt-image-source-group">
              <span class="ppt-image-source-group-label">
                <svg viewBox="0 0 24 24" aria-hidden="true">
                  <rect x="4" y="5" width="16" height="14" rx="2" />
                  <path d="M8 9h3M8 9v6M11 12H9.5M14 9v6M17 9v6M17 9h-3" />
                </svg>
                Animated
              </span>
              <button
                type="button"
                role="option"
                class="ppt-image-source-option"
                :class="{ selected: imageSourceSelectValue === 'gif' }"
                :aria-selected="imageSourceSelectValue === 'gif'"
                @click="selectGifSource"
              >
                GIFs from Giphy
              </button>
            </div>

            <div class="ppt-image-source-group">
              <button
                type="button"
                role="option"
                class="ppt-image-source-option"
                :class="{ selected: imageSourceSelectValue === 'none' }"
                :aria-selected="imageSourceSelectValue === 'none'"
                @click="selectNoImageSource"
              >
                No Images
              </button>
            </div>
          </div>
        </Transition>
      </Teleport>
    </div>

    <div v-if="isThemeModalOpen" class="ppt-theme-modal-backdrop" role="presentation" @click.self="closeThemeModal">
      <section class="ppt-theme-modal" role="dialog" aria-modal="true" aria-labelledby="ppt-theme-modal-title">
        <div class="ppt-theme-modal-content">
          <header class="ppt-theme-modal-header">
            <h2 id="ppt-theme-modal-title">主题</h2>
            <button type="button" class="ppt-theme-modal-close" title="关闭更多主题" aria-label="关闭更多主题" @click="closeThemeModal">
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <path d="M18 6 6 18" />
                <path d="m6 6 12 12" />
              </svg>
            </button>
          </header>

          <div class="ppt-theme-modal-tabs" role="tablist" aria-label="主题来源">
            <button
              v-for="tab in themeModalTabs"
              :key="tab.value"
              type="button"
              role="tab"
              :aria-selected="themeModalTab === tab.value"
              :class="{ active: themeModalTab === tab.value }"
              @click="themeModalTab = tab.value"
            >
              {{ tab.label }}
            </button>
          </div>

          <div class="ppt-theme-modal-scroll">
            <div v-if="modalThemeList.length" class="ppt-theme-modal-grid" role="listbox" aria-label="主题列表">
              <button
                v-for="item in modalThemeList"
                :key="item.value"
                type="button"
                role="option"
                class="ppt-theme-modal-card"
                :class="{ active: previewTheme === item.value, applied: modelValue === item.value }"
                :aria-selected="previewTheme === item.value"
                :title="`预览主题：${item.label}`"
                :aria-label="`预览主题：${item.label}`"
                @click="previewTheme = item.value"
              >
                <span class="ppt-theme-modal-card-preview" :style="{ background: item.colors[1] }">
                  <i :style="{ background: item.colors[0], borderColor: item.colors[2] }">
                    <b :style="{ color: item.colors[1] }">标题</b>
                    <em :style="{ background: item.colors[2] }"></em>
                    <small :style="{ color: item.colors[1] }">正文链接</small>
                  </i>
                </span>
                <span class="ppt-theme-modal-card-info">
                  <strong>{{ item.label }}</strong>
                  <small>{{ item.description }}</small>
                </span>
                <span v-if="previewTheme === item.value" class="ppt-theme-modal-selected" aria-hidden="true">
                  <svg viewBox="0 0 24 24">
                    <path d="m5 13 4 4L19 7" />
                  </svg>
                </span>
                <span v-if="modelValue === item.value" class="ppt-theme-modal-applied">已应用</span>
              </button>
            </div>

            <div v-else class="ppt-theme-modal-empty">
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <path d="M12 3a9 9 0 1 0 0 18h2a2.5 2.5 0 0 0 0-5h-2a2 2 0 0 1 0-4h2a7 7 0 0 0 7-7 2 2 0 0 0-2-2z" />
              </svg>
              <strong>还没有自定义主题</strong>
              <span>后续接入主题服务后，这里会展示你保存或导入的主题。</span>
              <div>
                <button type="button" @click="openCreateThemeModal(false)">新建主题</button>
                <button type="button" @click="openCreateThemeModal(false)">导入 PPTX</button>
              </div>
            </div>
          </div>

          <footer class="ppt-theme-modal-actions">
            <button type="button" title="取消主题预览" aria-label="取消主题预览" @click="closeThemeModal">取消</button>
            <button type="button" class="primary" :disabled="!previewThemeOption" title="应用预览主题" aria-label="应用预览主题" @click="applyPreviewTheme">应用主题</button>
          </footer>
        </div>

        <aside class="ppt-theme-modal-preview">
          <header>
            <h2>预览</h2>
            <span>{{ previewThemeOption?.label || "未选择主题" }}</span>
          </header>
          <div v-if="previewThemeOption" class="ppt-theme-preview-stack" :style="previewThemeStyle">
            <article v-for="slide in modalPreviewSlides" :key="slide.index" class="ppt-theme-preview-slide">
              <span>{{ slide.index }}</span>
              <h3>{{ slide.title }}</h3>
              <p>{{ slide.body }}</p>
              <ul>
                <li v-for="point in slide.points" :key="point">{{ point }}</li>
              </ul>
            </article>
          </div>
        </aside>
      </section>
    </div>
    <PptCreateThemeModal
      v-if="isCreateThemeModalOpen"
      v-model:open="isCreateThemeModalOpen"
      :base-theme="previewTheme"
      :is-customizing="isCreateThemeCustomizing"
      @apply-theme="handleCreatedThemeApply"
      @saved="closeThemeModal"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, ref, watch } from "vue";
import { pptThemes } from "../../config/pptThemes";
import type { PptImageSource, PptModelOption, PptTheme, PptThemeOption } from "../../types/ppt";

const PptCreateThemeModal = defineAsyncComponent(() => import("./PptCreateThemeModal.vue"));

type ThemeModalTab = "builtIn" | "mine";

const props = withDefaults(defineProps<{
  modelValue: PptTheme;
  imageSource: PptImageSource;
  imageModel: string;
  imageModels: PptModelOption[];
  disabled?: boolean;
}>(), {
  disabled: false
});

const emit = defineEmits<{
  "update:modelValue": [value: PptTheme];
  "update:imageSource": [value: PptImageSource];
  "update:imageModel": [value: string];
}>();

const isThemeModalOpen = ref(false);
const isCreateThemeModalOpen = ref(false);
const isCreateThemeCustomizing = ref(true);
const isImageSourceOpen = ref(false);
const imageSourceSelectRef = ref<HTMLElement | null>(null);
const imageSourceMenuRef = ref<HTMLElement | null>(null);
const imageSourceMenuStyle = ref<Record<string, string>>({});
const imageSourcePlacement = ref<"top" | "bottom">("bottom");
const previewTheme = ref<PptTheme>(props.modelValue);
const themeModalTab = ref<ThemeModalTab>("builtIn");
const visibleThemes = computed(() => {
  const primaryThemes = pptThemes.slice(0, 9);
  if (primaryThemes.some((item) => item.value === props.modelValue)) {
    return primaryThemes;
  }
  const selectedTheme = pptThemes.find((item) => item.value === props.modelValue);
  return selectedTheme ? [selectedTheme, ...primaryThemes.slice(0, 8)] : primaryThemes;
});
const themeModalTabs = [
  { label: "内置主题", value: "builtIn" },
  { label: "我的主题", value: "mine" }
] satisfies Array<{ label: string; value: ThemeModalTab }>;
const defaultImageModel = computed(() => props.imageModels[0]?.value || "default-image");
const isDefaultImageModel = computed(() => props.imageModel === defaultImageModel.value || props.imageModel === "default-image");
const stockImageProviders = [
  { label: "Unsplash", value: "stock-unsplash" },
  { label: "Pixabay", value: "stock-pixabay" },
  { label: "Web Search", value: "stock-google" }
] as const;
const selectedStockImageProvider = computed(() => stockImageProviders.find((item) => item.value === props.imageModel));
const imageSourceSelectValue = computed(() => {
  if (props.imageSource === "none") return "none";
  if (props.imageSource === "stock") {
    if (props.imageModel === "gif") return "gif";
    return selectedStockImageProvider.value?.value || "stock-unsplash";
  }
  if (isDefaultImageModel.value) return "automatic";
  return props.imageModel || defaultImageModel.value;
});
const previewThemeOption = computed(() => pptThemes.find((item) => item.value === previewTheme.value));
const modalThemeList = computed<PptThemeOption[]>(() => themeModalTab.value === "mine" ? [] : pptThemes);
const previewThemeStyle = computed(() => {
  const colors = previewThemeOption.value?.colors || ["#f8fafc", "#1e293b", "#2563eb"];
  return {
    "--ppt-theme-preview-surface": colors[0],
    "--ppt-theme-preview-ink": colors[1],
    "--ppt-theme-preview-accent": colors[2]
  };
});
const modalPreviewSlides = [
  {
    index: "1 / 3",
    title: "业务增长方案",
    body: "用统一的视觉系统呈现目标、策略和关键数据。",
    points: ["明确目标人群", "拆解增长路径"]
  },
  {
    index: "2 / 3",
    title: "核心执行节奏",
    body: "把复杂信息压缩成适合演示的版式层级。",
    points: ["重点优先", "图文并行"]
  },
  {
    index: "3 / 3",
    title: "下一步行动",
    body: "收束结论，帮助听众快速判断方案价值。",
    points: ["确认资源", "安排负责人"]
  }
];
const imageSourceLabel = computed(() => {
  if (props.imageSource === "none") return "无图片";
  if (props.imageSource === "stock") return props.imageModel === "gif" ? "GIFs from Giphy" : selectedStockImageProvider.value?.label || "Unsplash";
  if (isDefaultImageModel.value) return "Automatic";
  const model = props.imageModels.find((item) => item.value === props.imageModel);
  return model?.label || "AI 生成";
});

let previousBodyOverflow = "";

watch(
  () => props.modelValue,
  (value) => {
    if (!isThemeModalOpen.value) previewTheme.value = value;
  }
);

watch(isThemeModalOpen, (open) => {
  if (typeof document === "undefined") return;
  if (open) {
    previousBodyOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return;
  }
  document.body.style.overflow = previousBodyOverflow;
});

watch(isImageSourceOpen, (open) => {
  if (typeof document === "undefined") return;
  if (open) {
    nextTick(updateImageSourceMenuPosition);
    document.addEventListener("pointerdown", handleImageSourceOutsidePointerDown);
    document.addEventListener("keydown", handleImageSourceKeydown);
    window.addEventListener("resize", updateImageSourceMenuPosition);
    window.addEventListener("scroll", updateImageSourceMenuPosition, true);
    return;
  }
  document.removeEventListener("pointerdown", handleImageSourceOutsidePointerDown);
  document.removeEventListener("keydown", handleImageSourceKeydown);
  window.removeEventListener("resize", updateImageSourceMenuPosition);
  window.removeEventListener("scroll", updateImageSourceMenuPosition, true);
});

onBeforeUnmount(() => {
  if (typeof document === "undefined") return;
  document.body.style.overflow = previousBodyOverflow;
  document.removeEventListener("pointerdown", handleImageSourceOutsidePointerDown);
  document.removeEventListener("keydown", handleImageSourceKeydown);
  window.removeEventListener("resize", updateImageSourceMenuPosition);
  window.removeEventListener("scroll", updateImageSourceMenuPosition, true);
});

function openThemeModal() {
  if (props.disabled) return;
  previewTheme.value = props.modelValue;
  themeModalTab.value = "builtIn";
  isThemeModalOpen.value = true;
}

function closeThemeModal() {
  isThemeModalOpen.value = false;
}

function applyPreviewTheme() {
  emit("update:modelValue", previewTheme.value);
  closeThemeModal();
}

function openCreateThemeModal(isCustomizing: boolean) {
  if (props.disabled) return;
  previewTheme.value = props.modelValue;
  isCreateThemeCustomizing.value = isCustomizing;
  isCreateThemeModalOpen.value = true;
}

function handleCreatedThemeApply(value: PptTheme) {
  previewTheme.value = value;
  emit("update:modelValue", value);
}

function toggleImageSourceSelect() {
  if (props.disabled) return;
  if (isImageSourceOpen.value) {
    closeImageSourceSelect();
    return;
  }
  isImageSourceOpen.value = true;
}

function closeImageSourceSelect() {
  isImageSourceOpen.value = false;
}

function selectAutomaticImageSource() {
  emit("update:imageSource", "ai");
  emit("update:imageModel", defaultImageModel.value);
  closeImageSourceSelect();
}

function selectAiModel(value: string) {
  emit("update:imageSource", "ai");
  emit("update:imageModel", value);
  closeImageSourceSelect();
}

function selectStockImageProvider(value: string) {
  emit("update:imageSource", "stock");
  emit("update:imageModel", value);
  closeImageSourceSelect();
}

function selectGifSource() {
  emit("update:imageSource", "stock");
  emit("update:imageModel", "gif");
  closeImageSourceSelect();
}

function selectNoImageSource() {
  emit("update:imageSource", "none");
  closeImageSourceSelect();
}

function handleImageSourceOutsidePointerDown(event: PointerEvent) {
  const target = event.target as Node | null;
  if (!target || imageSourceSelectRef.value?.contains(target) || imageSourceMenuRef.value?.contains(target)) return;
  closeImageSourceSelect();
}

function handleImageSourceKeydown(event: KeyboardEvent) {
  if (event.key === "Escape") closeImageSourceSelect();
}

function updateImageSourceMenuPosition() {
  if (typeof window === "undefined") return;
  const trigger = imageSourceSelectRef.value?.querySelector(".ppt-image-source-trigger") as HTMLElement | null;
  if (!trigger) return;

  const viewportGap = 8;
  const popoverGap = 6;
  const rect = trigger.getBoundingClientRect();
  const viewportWidth = window.innerWidth || document.documentElement.clientWidth;
  const viewportHeight = window.innerHeight || document.documentElement.clientHeight;
  const menuHeight = imageSourceMenuRef.value?.offsetHeight || 320;
  const spaceBelow = viewportHeight - rect.bottom - viewportGap;
  const spaceAbove = rect.top - viewportGap;
  const shouldOpenTop = spaceBelow < Math.min(menuHeight, 260) && spaceAbove > spaceBelow;
  const availableHeight = Math.max(160, Math.min(320, (shouldOpenTop ? spaceAbove : spaceBelow) - popoverGap));
  const width = Math.min(Math.max(rect.width, 180), viewportWidth - viewportGap * 2);
  const left = Math.min(Math.max(viewportGap, rect.left), viewportWidth - width - viewportGap);
  const visibleMenuHeight = Math.min(menuHeight, availableHeight);
  const top = shouldOpenTop
    ? Math.max(viewportGap, rect.top - visibleMenuHeight - popoverGap)
    : Math.min(rect.bottom + popoverGap, viewportHeight - visibleMenuHeight - viewportGap);

  imageSourcePlacement.value = shouldOpenTop ? "top" : "bottom";
  imageSourceMenuStyle.value = {
    left: `${Math.round(left)}px`,
    top: `${Math.round(top)}px`,
    width: `${Math.round(width)}px`,
    maxHeight: `${Math.round(availableHeight)}px`
  };
}
</script>

<style scoped>
.ppt-theme-settings-panel {
  display: grid;
  gap: 20px;
  padding: 20px;
  border: 1px solid #202020;
  border-radius: 14px;
  background: rgba(12, 12, 12, 0.82);
}

.ppt-theme-settings-header h2 {
  margin: 0;
  color: #f4f4f5;
  font-size: 20px;
  line-height: 1.2;
  letter-spacing: 0;
}

.ppt-theme-settings-header p,
.ppt-theme-modal-preview p {
  margin: 6px 0 0;
  color: #a1a1aa;
  font-size: 13px;
  line-height: 1.55;
}

.ppt-theme-settings-section {
  display: grid;
  gap: 12px;
}

.ppt-theme-settings-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.ppt-theme-settings-title-row > span {
  color: #0f172a;
  font-size: 15px;
  font-weight: 760;
  line-height: 1.3;
}

.ppt-theme-settings-title-row small {
  color: #64748b;
  font-size: 12px;
}

.ppt-theme-more-button {
  border: 0;
  color: #93c5fd;
  background: transparent;
  cursor: pointer;
  font-weight: 780;
}

.ppt-theme-more-button:hover {
  color: #bfdbfe;
}

.ppt-theme-visible-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.ppt-theme-preview-card {
  position: relative;
  display: grid;
  gap: 9px;
  min-width: 0;
  min-height: 212px;
  padding: 0;
  border: 2px solid #242424;
  border-radius: 10px;
  color: #f4f4f5;
  background: #0d0d0d;
  text-align: left;
  transition: border-color 0.16s ease, transform 0.16s ease, background-color 0.16s ease;
}

.ppt-theme-preview-card:hover,
.ppt-theme-preview-card.active {
  border-color: #737373;
  background: #151515;
  transform: translateY(-1px);
}

.ppt-theme-preview-main {
  position: relative;
  display: grid;
  gap: 9px;
  width: 100%;
  min-height: 176px;
  padding: 10px;
  border: 0;
  color: inherit;
  background: transparent;
  cursor: pointer;
  text-align: left;
}

.ppt-theme-personalize-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  min-height: 34px;
  margin: 0 10px 10px;
  border: 0;
  border-radius: 999px;
  color: #fff;
  background: #7c3aed;
  cursor: pointer;
  font-size: 12px;
  font-weight: 780;
  transition: background-color 0.16s ease, transform 0.16s ease;
}

.ppt-theme-personalize-button:hover:not(:disabled) {
  background: #6d28d9;
  transform: translateY(-1px);
}

.ppt-theme-preview-canvas {
  display: block;
  height: 96px;
  padding: 10px;
  overflow: hidden;
  border-radius: 8px;
}

.ppt-theme-preview-canvas i {
  display: grid;
  align-content: center;
  gap: 7px;
  height: 100%;
  padding: 12px;
  border: 1px solid;
  border-radius: 8px;
  font-style: normal;
}

.ppt-theme-preview-canvas b {
  font-size: 16px;
}

.ppt-theme-preview-canvas em {
  width: 48px;
  height: 5px;
  border-radius: 999px;
}

.ppt-theme-preview-canvas small {
  opacity: 0.76;
}

.ppt-theme-preview-info {
  display: grid;
  gap: 3px;
  min-width: 0;
}

.ppt-theme-preview-info strong {
  font-size: 13px;
}

.ppt-theme-preview-info small {
  display: -webkit-box;
  overflow: hidden;
  color: #9ca3af;
  font-size: 12px;
  line-height: 1.42;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.ppt-theme-selected-mark {
  position: absolute;
  right: 9px;
  bottom: 9px;
  display: grid;
  place-items: center;
  width: 22px;
  height: 22px;
  border-radius: 999px;
  color: #111;
  background: #f4f4f5;
}

.ppt-image-source-select {
  position: relative;
  width: 100%;
}

.ppt-image-source-trigger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
  min-height: 42px;
  padding: 0 12px;
  border: 1px solid #d9dde3;
  border-radius: 7px;
  color: #111827;
  background: #fff;
  cursor: pointer;
  font-size: 14px;
  text-align: left;
  transition: border-color 0.16s ease, box-shadow 0.16s ease, background-color 0.16s ease;
}

.ppt-image-source-trigger:hover,
.ppt-image-source-trigger.open {
  border-color: #b7beca;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.08);
}

.ppt-image-source-trigger:disabled {
  cursor: not-allowed;
  opacity: 0.58;
}

.ppt-image-source-trigger-value {
  overflow: hidden;
  min-width: 0;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-image-source-chevron {
  flex: 0 0 auto;
  width: 16px;
  height: 16px;
  color: #6b7280;
  transition: transform 0.16s ease;
}

.ppt-image-source-trigger.open .ppt-image-source-chevron {
  transform: rotate(180deg);
}

.ppt-image-source-menu {
  position: fixed;
  z-index: 2600;
  display: grid;
  box-sizing: border-box;
  overflow: auto;
  padding: 5px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  color: #111827;
  background: #fff;
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.16), 0 2px 8px rgba(15, 23, 42, 0.08);
  transform-origin: top center;
}

.ppt-image-source-menu.is-top {
  transform-origin: bottom center;
}

.ppt-select-pop-enter-active,
.ppt-select-pop-leave-active {
  transition: opacity 0.14s ease, transform 0.14s ease;
}

.ppt-select-pop-enter-from,
.ppt-select-pop-leave-to {
  opacity: 0;
  transform: translateY(-4px) scale(0.98);
}

.ppt-select-pop-enter-from.is-top,
.ppt-select-pop-leave-to.is-top {
  transform: translateY(4px) scale(0.98);
}

.ppt-image-source-group {
  display: grid;
  gap: 2px;
  padding: 2px 0;
}

.ppt-image-source-group + .ppt-image-source-group {
  border-top: 1px solid #f1f5f9;
  padding-top: 5px;
}

.ppt-image-source-group-label {
  display: flex;
  align-items: center;
  gap: 5px;
  min-height: 26px;
  padding: 4px 8px 2px;
  color: #3b82f6;
  font-size: 11px;
  font-weight: 650;
}

.ppt-image-source-group-label svg {
  width: 12px;
  height: 12px;
}

.ppt-image-source-option {
  display: flex;
  align-items: center;
  width: 100%;
  min-height: 34px;
  padding: 6px 8px;
  border: 0;
  border-radius: 6px;
  color: #111827;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  line-height: 1.35;
  text-align: left;
  transition: background-color 0.14s ease, color 0.14s ease;
}

.ppt-image-source-option:hover,
.ppt-image-source-option.selected {
  background: #f3f4f6;
}

.ppt-image-source-option.selected {
  font-weight: 650;
}

.ppt-theme-modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 90;
  display: grid;
  place-items: center;
  padding: 8px;
  background: rgba(15, 23, 42, 0.48);
  backdrop-filter: blur(10px);
}

.ppt-theme-modal {
  display: flex;
  width: min(calc(100vw - 16px), 72rem);
  height: min(92dvh, 56rem);
  max-height: calc(100dvh - 16px);
  overflow: hidden;
  border: 1px solid rgba(226, 232, 240, 0.92);
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 28px 90px rgba(15, 23, 42, 0.28);
}

.ppt-theme-modal-content {
  display: flex;
  flex: 0 0 40%;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
  border-right: 1px solid #e5e7eb;
  background: #fff;
}

.ppt-theme-modal-header {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  min-height: 60px;
  padding: 0 18px;
  border-bottom: 1px solid #e5e7eb;
}

.ppt-theme-modal-header h2,
.ppt-theme-modal-preview h2 {
  margin: 0;
  color: #0f172a;
  font-size: 18px;
  font-weight: 780;
  letter-spacing: 0;
}

.ppt-theme-modal-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border: 0;
  border-radius: 9px;
  color: #475569;
  background: transparent;
  cursor: pointer;
  transition: background-color 0.16s ease, color 0.16s ease;
}

.ppt-theme-modal-close:hover {
  color: #0f172a;
  background: #f1f5f9;
}

.ppt-theme-modal-tabs {
  display: flex;
  flex: 0 0 auto;
  gap: 24px;
  padding: 10px 18px 0;
  border-bottom: 1px solid #e5e7eb;
}

.ppt-theme-modal-tabs button {
  position: relative;
  min-height: 38px;
  padding: 0;
  border: 0;
  color: #64748b;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  font-weight: 760;
}

.ppt-theme-modal-tabs button::after {
  position: absolute;
  left: 0;
  right: 0;
  bottom: -1px;
  height: 2px;
  border-radius: 999px;
  background: transparent;
  content: "";
}

.ppt-theme-modal-tabs button:hover,
.ppt-theme-modal-tabs button.active {
  color: #111827;
}

.ppt-theme-modal-tabs button.active::after {
  background: #111827;
}

.ppt-theme-modal-scroll {
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
  padding: 16px;
}

.ppt-theme-modal-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.ppt-theme-modal-card {
  position: relative;
  display: grid;
  gap: 9px;
  min-width: 0;
  min-height: 176px;
  padding: 10px;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  color: #0f172a;
  background: #fff;
  cursor: pointer;
  text-align: left;
  transition: border-color 0.16s ease, transform 0.16s ease, box-shadow 0.16s ease;
}

.ppt-theme-modal-card:hover,
.ppt-theme-modal-card.active {
  border-color: #111827;
  transform: translateY(-1px);
  box-shadow: 0 14px 32px rgba(15, 23, 42, 0.1);
}

.ppt-theme-modal-card-preview {
  display: block;
  height: 94px;
  overflow: hidden;
  padding: 10px;
  border-radius: 8px;
}

.ppt-theme-modal-card-preview i {
  display: grid;
  align-content: center;
  gap: 6px;
  height: 100%;
  padding: 12px;
  border: 1px solid;
  border-radius: 9px;
  font-style: normal;
}

.ppt-theme-modal-card-preview b {
  font-size: 15px;
  line-height: 1;
}

.ppt-theme-modal-card-preview em {
  display: block;
  width: 46px;
  height: 5px;
  border-radius: 999px;
}

.ppt-theme-modal-card-preview small {
  opacity: 0.72;
  font-size: 11px;
}

.ppt-theme-modal-card-info {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.ppt-theme-modal-card-info strong {
  font-size: 14px;
  line-height: 1.2;
}

.ppt-theme-modal-card-info small {
  display: -webkit-box;
  overflow: hidden;
  color: #64748b;
  font-size: 12px;
  line-height: 1.45;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.ppt-theme-modal-selected {
  position: absolute;
  right: 10px;
  bottom: 10px;
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border-radius: 999px;
  color: #fff;
  background: #7c3aed;
  box-shadow: 0 8px 20px rgba(124, 58, 237, 0.24);
}

.ppt-theme-modal-applied {
  position: absolute;
  top: 10px;
  right: 10px;
  padding: 4px 8px;
  border-radius: 999px;
  color: #0f172a;
  background: rgba(255, 255, 255, 0.86);
  font-size: 11px;
  font-weight: 780;
}

.ppt-theme-modal-empty {
  display: grid;
  justify-items: center;
  gap: 12px;
  min-height: 280px;
  padding: 34px 18px;
  border: 1px dashed #cbd5e1;
  border-radius: 12px;
  color: #64748b;
  background: #f8fafc;
  text-align: center;
}

.ppt-theme-modal-empty > svg {
  width: 32px;
  height: 32px;
  color: #94a3b8;
}

.ppt-theme-modal-empty strong {
  color: #0f172a;
  font-size: 15px;
}

.ppt-theme-modal-empty span {
  max-width: 280px;
  font-size: 13px;
  line-height: 1.6;
}

.ppt-theme-modal-empty div {
  display: flex;
  gap: 10px;
}

.ppt-theme-modal-empty button,
.ppt-theme-modal-actions button {
  min-height: 36px;
  padding: 0 14px;
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  color: #0f172a;
  background: #fff;
  cursor: pointer;
  font-weight: 780;
  transition: border-color 0.16s ease, background-color 0.16s ease, color 0.16s ease;
}

.ppt-theme-modal-empty button:hover,
.ppt-theme-modal-actions button:hover:not(:disabled) {
  border-color: #94a3b8;
  background: #f8fafc;
}

.ppt-theme-modal-actions button.primary {
  color: #fff;
  border-color: #111827;
  background: #111827;
}

.ppt-theme-modal-actions button.primary:hover:not(:disabled) {
  border-color: #020617;
  background: #020617;
}

.ppt-theme-modal-actions button:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.ppt-theme-modal-actions {
  display: flex;
  flex: 0 0 auto;
  justify-content: flex-end;
  gap: 10px;
  padding: 14px 16px calc(14px + env(safe-area-inset-bottom));
  border-top: 1px solid #e5e7eb;
  background: rgba(255, 255, 255, 0.92);
  backdrop-filter: blur(12px);
}

.ppt-theme-modal-preview {
  display: flex;
  flex: 1 1 60%;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  background: #f8fafc;
}

.ppt-theme-modal-preview header {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 60px;
  padding: 0 18px;
  border-bottom: 1px solid #e5e7eb;
  background: rgba(248, 250, 252, 0.88);
  backdrop-filter: blur(10px);
}

.ppt-theme-modal-preview header span {
  color: #64748b;
  font-size: 13px;
  font-weight: 760;
}

.ppt-theme-preview-stack {
  --ppt-theme-preview-surface: #f8fafc;
  --ppt-theme-preview-ink: #1e293b;
  --ppt-theme-preview-accent: #2563eb;
  display: grid;
  justify-items: center;
  gap: 18px;
  min-height: 0;
  overflow: auto;
  padding: 22px;
}

.ppt-theme-preview-slide {
  display: grid;
  gap: 10px;
  width: min(520px, 100%);
  aspect-ratio: 16 / 9;
  min-height: 250px;
  padding: 30px 34px;
  border: 1px solid color-mix(in srgb, var(--ppt-theme-preview-accent), transparent 40%);
  border-radius: 12px;
  color: var(--ppt-theme-preview-ink);
  background:
    radial-gradient(circle at 88% 12%, color-mix(in srgb, var(--ppt-theme-preview-accent), transparent 72%), transparent 34%),
    var(--ppt-theme-preview-surface);
  box-shadow: 0 18px 44px rgba(15, 23, 42, 0.12);
}

.ppt-theme-preview-slide > span {
  color: var(--ppt-theme-preview-accent);
  font-size: 13px;
  font-weight: 860;
}

.ppt-theme-preview-slide h3 {
  margin: 0;
  color: var(--ppt-theme-preview-ink);
  font-size: 30px;
  line-height: 1.12;
  letter-spacing: 0;
}

.ppt-theme-preview-slide p {
  margin: 0;
  max-width: 420px;
  color: color-mix(in srgb, var(--ppt-theme-preview-ink), transparent 18%);
  font-size: 15px;
  line-height: 1.7;
}

.ppt-theme-preview-slide ul {
  display: grid;
  gap: 8px;
  margin: 4px 0 0;
  padding-left: 20px;
  color: var(--ppt-theme-preview-ink);
  font-size: 14px;
  line-height: 1.5;
}

.ppt-theme-preview-slide li::marker {
  color: var(--ppt-theme-preview-accent);
}

.ppt-theme-settings-panel svg {
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.9;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.ppt-theme-settings-panel button:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

@media (max-width: 980px) {
  .ppt-theme-modal {
    flex-direction: column;
    overflow: auto;
  }

  .ppt-theme-modal-content {
    flex-basis: auto;
    border-right: 0;
    border-bottom: 1px solid #e5e7eb;
  }

  .ppt-theme-modal-preview {
    flex-basis: auto;
    min-height: 420px;
  }

  .ppt-theme-preview-slide {
    min-height: 220px;
  }

  .ppt-theme-visible-grid,
  .ppt-theme-modal-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .ppt-theme-settings-panel {
    padding: 16px;
  }

  .ppt-theme-settings-title-row {
    align-items: flex-start;
    flex-direction: column;
  }

  .ppt-theme-modal-backdrop {
    align-items: stretch;
    padding: 0;
  }

  .ppt-theme-modal {
    width: 100%;
    height: 100dvh;
    max-height: none;
    border-radius: 0;
  }

  .ppt-theme-modal-tabs {
    gap: 18px;
  }

  .ppt-theme-modal-preview {
    display: none;
  }
}
</style>
