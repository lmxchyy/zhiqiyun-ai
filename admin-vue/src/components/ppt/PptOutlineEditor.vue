<template>
  <section v-if="outline" class="ppt-outline-editor">
    <header class="ppt-outline-header">
      <div>
        <h2>演示大纲</h2>
        <span>{{ isBusy ? "正在生成..." : "可编辑幻灯片顺序、内容和布局" }}</span>
      </div>
      <div class="ppt-outline-header-actions">
        <button type="button" class="ppt-outline-action-button" :disabled="isBusy" title="重新生成大纲" aria-label="重新生成大纲" @click="$emit('regenerate-all')">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="M21 12a9 9 0 0 1-15.5 6.2" />
            <path d="M3 12a9 9 0 0 1 15.5-6.2" />
            <path d="M18 3v4h-4" />
            <path d="M6 21v-4h4" />
          </svg>
          <span>重新生成</span>
        </button>
        <button type="button" class="ppt-outline-action-button" :disabled="isBusy" title="保存大纲" aria-label="保存大纲" @click="$emit('save')">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2Z" />
            <path d="M17 21v-8H7v8" />
            <path d="M7 3v5h8" />
          </svg>
          <span>保存</span>
        </button>
        <div class="ppt-outline-layouts">
          <button
            type="button"
            class="ppt-outline-layouts-button"
            :disabled="isBusy"
            title="批量选择版式"
            aria-label="批量选择版式"
            :aria-expanded="showGlobalLayoutMenu"
            aria-haspopup="menu"
            @click="showGlobalLayoutMenu = !showGlobalLayoutMenu"
          >
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <rect x="3" y="3" width="7" height="7" rx="1.5" />
              <rect x="14" y="3" width="7" height="7" rx="1.5" />
              <rect x="3" y="14" width="7" height="7" rx="1.5" />
              <rect x="14" y="14" width="7" height="7" rx="1.5" />
            </svg>
            <span>版式</span>
            <b v-if="selectedLayoutCount">{{ selectedLayoutCount }}</b>
          </button>
          <div v-if="showGlobalLayoutMenu" class="ppt-outline-global-layout-menu" role="menu">
            <button
              type="button"
              :class="{ active: selectedLayoutCount === 0 }"
              role="menuitemradio"
              :aria-checked="selectedLayoutCount === 0"
              title="全部页面使用自动版式"
              aria-label="全部页面使用自动版式"
              @click="applyGlobalLayout(null)"
            >
              <span>自动</span>
              <small>由 AI 自动匹配每页版式</small>
            </button>
            <button
              v-for="option in layoutOptions"
              :key="option.value"
              type="button"
              role="menuitemradio"
              :aria-checked="allSlidesUseLayout(option.value)"
              :title="`全部页面使用${option.label}版式`"
              :aria-label="`全部页面使用${option.label}版式：${option.description}`"
              :class="{ active: allSlidesUseLayout(option.value) }"
              @click="applyGlobalLayout(option.value)"
            >
              <span>{{ option.label }}</span>
              <small>{{ option.description }}</small>
            </button>
          </div>
        </div>
      </div>
    </header>

    <label class="ppt-outline-title-field">
      <span>演示文稿标题</span>
      <input
        :value="outline.title"
        :disabled="isBusy"
        title="演示文稿标题"
        aria-label="演示文稿标题"
        placeholder="请输入演示文稿标题"
        @input="emitOutlineTitle($event)"
      />
    </label>

    <div class="ppt-outline-list">
      <article v-for="(slide, index) in outline.slides" :key="slide.page" class="ppt-outline-card">
        <button type="button" class="ppt-outline-grip" :disabled="isBusy" title="使用右侧按钮调整顺序" aria-label="排序手柄">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="M9 5h.01M15 5h.01M9 12h.01M15 12h.01M9 19h.01M15 19h.01" />
          </svg>
        </button>
        <span class="ppt-outline-index">{{ index + 1 }}</span>

        <main class="ppt-outline-card-main">
          <div class="ppt-outline-card-top">
            <input
              :value="slide.title"
              :disabled="isBusy"
              :title="`第 ${index + 1} 页标题`"
              :aria-label="`第 ${index + 1} 页标题`"
              placeholder="输入这一页的标题"
              @input="updateSlide(index, { title: ($event.target as HTMLInputElement).value })"
            />
            <div class="ppt-outline-layout-picker">
              <button
                type="button"
                class="ppt-outline-layout-trigger"
                :disabled="isBusy"
                :title="`第 ${index + 1} 页版式：${slideLayoutLabel(slide, index)}`"
                :aria-label="`选择第 ${index + 1} 页版式`"
                :aria-expanded="openLayoutIndex === index"
                aria-haspopup="menu"
                @click="toggleLayoutMenu(index)"
              >
                <span>{{ slideLayoutLabel(slide, index) }}</span>
                <svg viewBox="0 0 24 24" aria-hidden="true">
                  <path d="m6 9 6 6 6-6" />
                </svg>
              </button>
              <div v-if="openLayoutIndex === index" class="ppt-outline-layout-menu" role="menu">
                <button
                  type="button"
                  :class="{ active: !slide.layout }"
                  role="menuitemradio"
                  :aria-checked="!slide.layout"
                  :title="`第 ${index + 1} 页使用自动版式`"
                  :aria-label="`第 ${index + 1} 页使用自动版式`"
                  @click="selectSlideLayout(index, null)"
                >
                  <span>自动</span>
                  <small>自动选择版式</small>
                </button>
                <button
                  v-for="option in layoutOptions"
                  :key="option.value"
                  type="button"
                  :class="{ active: slide.layout === option.value }"
                  role="menuitemradio"
                  :aria-checked="slide.layout === option.value"
                  :title="`第 ${index + 1} 页使用${option.label}版式`"
                  :aria-label="`第 ${index + 1} 页使用${option.label}版式：${option.description}`"
                  @click="selectSlideLayout(index, option.value)"
                >
                  <span>{{ option.label }}</span>
                  <small>{{ option.description }}</small>
                </button>
              </div>
            </div>
          </div>
          <textarea
            :value="slide.summary"
            :disabled="isBusy"
            :title="`第 ${index + 1} 页摘要`"
            :aria-label="`第 ${index + 1} 页摘要`"
            placeholder="这一页主要讲什么..."
            @input="updateSlide(index, { summary: ($event.target as HTMLTextAreaElement).value })"
          ></textarea>
          <details class="ppt-outline-bullets">
            <summary>要点列表</summary>
            <textarea
              :value="slide.bulletPoints.join('\n')"
              :disabled="isBusy"
              :title="`第 ${index + 1} 页要点列表`"
              :aria-label="`第 ${index + 1} 页要点列表`"
              placeholder="每行一个要点"
              @input="updateBullets(index, ($event.target as HTMLTextAreaElement).value)"
            ></textarea>
          </details>
        </main>

        <footer class="ppt-outline-card-actions">
          <button type="button" :disabled="isBusy || index === 0" :title="`上移第 ${index + 1} 页`" :aria-label="`上移第 ${index + 1} 页`" @click="$emit('move-slide', index, -1)">
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="m18 15-6-6-6 6" />
            </svg>
          </button>
          <button type="button" :disabled="isBusy || index === outline.slides.length - 1" :title="`下移第 ${index + 1} 页`" :aria-label="`下移第 ${index + 1} 页`" @click="$emit('move-slide', index, 1)">
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="m6 9 6 6 6-6" />
            </svg>
          </button>
          <button type="button" :disabled="isBusy" :title="`重新生成第 ${index + 1} 页`" :aria-label="`重新生成第 ${index + 1} 页`" @click="$emit('regenerate-slide', index)">
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="M21 12a9 9 0 0 1-9 9 8.6 8.6 0 0 1-6-2.4" />
              <path d="M3 12a9 9 0 0 1 15-6.7" />
              <path d="M18 3v5h-5" />
            </svg>
          </button>
          <button type="button" class="danger" :disabled="isBusy || outline.slides.length <= 1" :title="`删除第 ${index + 1} 页`" :aria-label="`删除第 ${index + 1} 页`" @click="$emit('delete-slide', index)">
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="M18 6 6 18" />
              <path d="m6 6 12 12" />
            </svg>
          </button>
        </footer>
      </article>
    </div>

    <button type="button" class="ppt-add-slide" :disabled="isBusy" title="添加幻灯片" aria-label="添加幻灯片" @click="$emit('add-slide')">
      <svg viewBox="0 0 24 24" aria-hidden="true">
        <path d="M5 12h14" />
        <path d="M12 5v14" />
      </svg>
      <span>添加幻灯片</span>
    </button>

    <div class="ppt-outline-meta">
      <span>共 {{ outline.slides.length }} 张幻灯片</span>
      <span>{{ outlineCharacterCount }}/20000</span>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import type { PptOutline, PptOutlineSlide, PptSlideLayout, PptWorkflowStatus } from "../../types/ppt";

const props = defineProps<{
  outline: PptOutline | null;
  status?: PptWorkflowStatus;
}>();

const emit = defineEmits<{
  "update-title": [value: string];
  "update-slide": [index: number, patch: Partial<PptOutlineSlide>];
  "add-slide": [];
  "delete-slide": [index: number];
  "move-slide": [index: number, direction: -1 | 1];
  "regenerate-slide": [index: number];
  "regenerate-all": [];
  save: [];
  confirm: [];
}>();

const openLayoutIndex = ref<number | null>(null);
const showGlobalLayoutMenu = ref(false);
const layoutOptions: Array<{ label: string; value: PptSlideLayout; description: string }> = [
  { label: "封面", value: "cover", description: "适合开场和主题页" },
  { label: "章节", value: "section", description: "适合章节转场" },
  { label: "正文", value: "content", description: "适合文字说明和列表" },
  { label: "图文", value: "imageText", description: "适合配图和案例展示" },
  { label: "总结", value: "summary", description: "适合结论和行动建议" }
];
const layoutLabels = layoutOptions.reduce<Record<PptSlideLayout, string>>((acc, option) => {
  acc[option.value] = option.label;
  return acc;
}, {} as Record<PptSlideLayout, string>);
const isBusy = computed(() => ["outlining", "pending", "generating", "rendering"].includes(props.status || "idle"));
const selectedLayoutCount = computed(() => props.outline?.slides.filter(slide => Boolean(slide.layout)).length || 0);
const outlineCharacterCount = computed(() => {
  if (!props.outline) return 0;
  return props.outline.slides.reduce((total, slide) => {
    return total + slide.title.length + slide.summary.length + slide.bulletPoints.join("").length;
  }, props.outline.title.length);
});

function emitOutlineTitle(event: Event) {
  emit("update-title", (event.target as HTMLInputElement).value);
}

function updateSlide(index: number, patch: Partial<PptOutlineSlide>) {
  emit("update-slide", index, patch);
}

function updateBullets(index: number, value: string) {
  updateSlide(index, { bulletPoints: value.split("\n").map(item => item.trim()).filter(Boolean) });
}

function inferredLayout(index: number): PptSlideLayout {
  if (!props.outline) return "content";
  if (index === 0) return "cover";
  if (index === props.outline.slides.length - 1) return "summary";
  return "content";
}

function layoutLabel(layout: PptSlideLayout) {
  return layoutLabels[layout] || "自动";
}

function slideLayoutLabel(slide: PptOutlineSlide, index: number) {
  return slide.layout ? layoutLabel(slide.layout) : `自动 · ${layoutLabel(inferredLayout(index))}`;
}

function toggleLayoutMenu(index: number) {
  if (isBusy.value) return;
  showGlobalLayoutMenu.value = false;
  openLayoutIndex.value = openLayoutIndex.value === index ? null : index;
}

function selectSlideLayout(index: number, layout: PptSlideLayout | null) {
  updateSlide(index, { layout: layout || undefined });
  openLayoutIndex.value = null;
}

function allSlidesUseLayout(layout: PptSlideLayout) {
  return Boolean(props.outline?.slides.length) && props.outline?.slides.every(slide => slide.layout === layout);
}

function applyGlobalLayout(layout: PptSlideLayout | null) {
  if (!props.outline || isBusy.value) return;
  props.outline.slides.forEach((_, index) => {
    updateSlide(index, { layout: layout || undefined });
  });
  showGlobalLayoutMenu.value = false;
}
</script>

<style scoped>
.ppt-outline-editor {
  display: grid;
  gap: 14px;
}

.ppt-outline-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
}

.ppt-outline-header h2 {
  margin: 0;
  color: #f4f4f5;
  font-size: 15px;
  font-weight: 760;
  letter-spacing: 0;
}

.ppt-outline-header span,
.ppt-outline-title-field span,
.ppt-outline-meta span {
  color: #a1a1aa;
  font-size: 12px;
}

.ppt-outline-header > div:first-child {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.ppt-outline-header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.ppt-outline-layouts {
  position: relative;
}

.ppt-outline-action-button,
.ppt-outline-layouts-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  min-height: 32px;
  padding: 0 10px;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
  color: #f4f4f5;
  background: #111;
  cursor: pointer;
  font-size: 12px;
  font-weight: 740;
  white-space: nowrap;
  transition: border-color 0.16s ease, background-color 0.16s ease, color 0.16s ease;
}

.ppt-outline-layouts-button {
  height: 32px;
}

.ppt-outline-layouts-button b {
  display: grid;
  place-items: center;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 999px;
  color: #111;
  background: #f4f4f5;
  font-size: 10px;
}

.ppt-outline-action-button:hover:not(:disabled),
.ppt-outline-layouts-button:hover:not(:disabled),
.ppt-outline-card-actions button:hover:not(:disabled),
.ppt-outline-layout-trigger:hover:not(:disabled) {
  border-color: #3f3f46;
  background: #18181b;
}

.ppt-outline-action-button:focus-visible,
.ppt-outline-layouts-button:focus-visible,
.ppt-outline-card-actions button:focus-visible,
.ppt-outline-layout-trigger:focus-visible,
.ppt-outline-grip:focus-visible,
.ppt-add-slide:focus-visible {
  outline: 2px solid rgba(34, 211, 238, 0.72);
  outline-offset: 2px;
}

.ppt-outline-title-field {
  display: grid;
  gap: 8px;
}

.ppt-outline-title-field input,
.ppt-outline-card input,
.ppt-outline-card textarea {
  width: 100%;
  box-sizing: border-box;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
  color: #f4f4f5;
  caret-color: #fff;
  background: #0d0d0d;
  outline: none;
}

.ppt-outline-title-field input {
  min-height: 38px;
  padding: 0 11px;
  font-weight: 760;
}

.ppt-outline-card input {
  min-height: 36px;
  padding: 0 10px;
  border-color: transparent;
  background: transparent;
  font-size: 15px;
  font-weight: 760;
}

.ppt-outline-card textarea {
  min-height: 54px;
  padding: 9px 10px;
  resize: vertical;
}

.ppt-outline-title-field input:focus,
.ppt-outline-card input:focus,
.ppt-outline-card textarea:focus {
  border-color: #3f3f46;
  background: #111;
  box-shadow: 0 0 0 3px rgba(34, 211, 238, 0.1);
}

.ppt-outline-list {
  display: grid;
  gap: 8px;
}

.ppt-outline-card {
  display: grid;
  grid-template-columns: auto 28px minmax(0, 1fr) auto;
  gap: 12px;
  align-items: start;
  padding: 14px;
  border: 1px solid #262626;
  border-radius: 8px;
  background: #111;
  transition: border-color 0.16s ease, background-color 0.16s ease, transform 0.16s ease;
}

.ppt-outline-card:hover {
  border-color: #3a3a3a;
  background: #151515;
}

.ppt-outline-grip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 34px;
  padding: 0;
  border: 0;
  color: #71717a;
  background: transparent;
  cursor: grab;
}

.ppt-outline-index {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 999px;
  color: #e5e7eb;
  background: #27272a;
  font-size: 13px;
  font-weight: 820;
}

.ppt-outline-card-main {
  display: grid;
  gap: 8px;
  min-width: 0;
}

.ppt-outline-card-top {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
  align-items: start;
}

.ppt-outline-layout-picker {
  position: relative;
}

.ppt-outline-layout-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 32px;
  max-width: 138px;
  padding: 0 9px;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
  color: #dbeafe;
  background: #0d0d0d;
  cursor: pointer;
  font-size: 12px;
  font-weight: 760;
  white-space: nowrap;
}

.ppt-outline-layout-trigger span {
  overflow: hidden;
  text-overflow: ellipsis;
}

.ppt-outline-bullets {
  display: grid;
  gap: 8px;
}

.ppt-outline-bullets summary {
  width: fit-content;
  color: #a1a1aa;
  cursor: pointer;
  font-size: 12px;
  font-weight: 740;
}

.ppt-outline-bullets[open] summary {
  color: #f4f4f5;
}

.ppt-outline-card-actions {
  display: grid;
  grid-template-columns: repeat(2, 30px);
  gap: 6px;
  opacity: 0;
  transition: opacity 0.16s ease;
}

.ppt-outline-card:hover .ppt-outline-card-actions,
.ppt-outline-card:focus-within .ppt-outline-card-actions {
  opacity: 1;
}

.ppt-outline-card-actions button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  padding: 0;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
  color: #d4d4d8;
  background: #0d0d0d;
  cursor: pointer;
}

.ppt-outline-card-actions button.danger {
  color: #fecaca;
}

.ppt-outline-action-button svg,
.ppt-outline-layouts-button svg,
.ppt-outline-layout-trigger svg,
.ppt-outline-grip svg,
.ppt-outline-card-actions svg,
.ppt-add-slide svg {
  width: 16px;
  height: 16px;
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.ppt-outline-global-layout-menu,
.ppt-outline-layout-menu {
  position: absolute;
  z-index: 20;
  display: grid;
  gap: 4px;
  width: 250px;
  padding: 6px;
  border: 1px solid #303030;
  border-radius: 10px;
  background: #111;
  box-shadow: 0 18px 40px rgba(0, 0, 0, 0.38);
}

.ppt-outline-global-layout-menu {
  top: calc(100% + 6px);
  right: 0;
}

.ppt-outline-layout-menu {
  top: calc(100% + 6px);
  right: 0;
}

.ppt-outline-global-layout-menu button,
.ppt-outline-layout-menu button {
  display: grid;
  justify-items: start;
  gap: 2px;
  min-height: 44px;
  padding: 7px 9px;
  border: 0;
  border-radius: 8px;
  color: #f4f4f5;
  text-align: left;
  background: transparent;
  cursor: pointer;
}

.ppt-outline-global-layout-menu button:hover,
.ppt-outline-global-layout-menu button.active,
.ppt-outline-layout-menu button:hover,
.ppt-outline-layout-menu button.active {
  background: #242424;
}

.ppt-outline-global-layout-menu small,
.ppt-outline-layout-menu small {
  color: #8b8b94;
  font-size: 11px;
}

.ppt-add-slide {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  width: 100%;
  min-height: 42px;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
  color: #a1a1aa;
  background: rgba(39, 39, 42, 0.55);
  cursor: pointer;
  font-weight: 760;
  transition: background-color 0.16s ease, color 0.16s ease, border-color 0.16s ease;
}

.ppt-add-slide:hover:not(:disabled) {
  border-color: #3f3f46;
  color: #f4f4f5;
  background: #242424;
}

.ppt-outline-meta {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 0 2px;
}

.ppt-outline-action-button:disabled,
.ppt-outline-layouts-button:disabled,
.ppt-outline-layout-trigger:disabled,
.ppt-outline-card-actions button:disabled,
.ppt-outline-grip:disabled,
.ppt-add-slide:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

@media (max-width: 980px) {
  .ppt-outline-card {
    grid-template-columns: auto 28px minmax(0, 1fr);
  }

  .ppt-outline-card-actions {
    grid-column: 3;
    grid-template-columns: repeat(4, 30px);
    opacity: 1;
  }

  .ppt-outline-card-top {
    grid-template-columns: 1fr;
  }

  .ppt-outline-layout-menu {
    right: auto;
    left: 0;
  }
}

@media (max-width: 640px) {
  .ppt-outline-header {
    display: grid;
  }

  .ppt-outline-header-actions {
    justify-content: start;
    overflow-x: auto;
    padding-bottom: 2px;
  }

  .ppt-outline-card {
    grid-template-columns: 1fr;
  }

  .ppt-outline-grip,
  .ppt-outline-index {
    display: none;
  }

  .ppt-outline-card-actions {
    grid-column: auto;
  }

  .ppt-outline-global-layout-menu {
    right: auto;
    left: 0;
  }
}
</style>
