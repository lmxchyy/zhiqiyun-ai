<template>
  <section class="ppt-theme-panel">
    <header class="ppt-theme-panel-header">
      <button type="button" class="ppt-theme-new-button" title="新建自定义主题" aria-label="新建自定义主题" @click="handleCreateTheme">
        <svg class="ppt-theme-icon" viewBox="0 0 24 24" aria-hidden="true">
          <path d="M5 12h14" />
          <path d="M12 5v14" />
        </svg>
        <span>新建主题</span>
      </button>
      <button
        type="button"
        class="ppt-theme-favorite-toggle"
        :class="{ active: showFavoritesOnly }"
        :aria-pressed="showFavoritesOnly"
        :title="showFavoritesOnly ? '查看全部主题' : '只看收藏主题'"
        :aria-label="showFavoritesOnly ? '查看全部主题' : '只看收藏主题'"
        @click="toggleFavoritesView"
      >
        <svg class="ppt-theme-icon" viewBox="0 0 24 24" aria-hidden="true">
          <path d="M11.525 2.295a.53.53 0 0 1 .95 0l2.31 4.679a2.123 2.123 0 0 0 1.595 1.16l5.166.756a.53.53 0 0 1 .294.904l-3.736 3.638a2.123 2.123 0 0 0-.611 1.878l.882 5.14a.53.53 0 0 1-.771.56l-4.618-2.428a2.122 2.122 0 0 0-1.973 0L6.396 21.01a.53.53 0 0 1-.77-.56l.881-5.139a2.122 2.122 0 0 0-.611-1.879L2.16 9.795a.53.53 0 0 1 .294-.906l5.165-.755a2.122 2.122 0 0 0 1.597-1.16z" />
        </svg>
      </button>
      <button
        v-if="showClose"
        type="button"
        class="ppt-theme-close-button"
        title="关闭主题面板"
        aria-label="关闭主题面板"
        @click="$emit('close')"
      >
        <svg class="ppt-theme-icon" viewBox="0 0 24 24" aria-hidden="true">
          <path d="M18 6 6 18" />
          <path d="m6 6 12 12" />
        </svg>
      </button>
    </header>

    <div class="ppt-theme-tabs" role="tablist" aria-label="主题来源">
      <button
        v-for="tab in tabs"
        :key="tab.value"
        type="button"
        role="tab"
        :aria-selected="activeTab === tab.value"
        :title="`切换主题来源：${tab.label}`"
        :aria-label="`切换主题来源：${tab.label}`"
        :class="{ active: activeTab === tab.value }"
        @click="activeTab = tab.value"
      >
        {{ tab.label }}
      </button>
    </div>

    <div class="ppt-theme-filter-row">
      <button
        type="button"
        :class="{ active: filterMode === 'customize' }"
        :aria-pressed="filterMode === 'customize'"
        title="自定义当前主题"
        aria-label="自定义当前主题"
        @click="toggleFilterMode('customize')"
      >
        <svg class="ppt-theme-icon" viewBox="0 0 24 24" aria-hidden="true">
          <line x1="21" x2="14" y1="4" y2="4" />
          <line x1="10" x2="3" y1="4" y2="4" />
          <line x1="21" x2="12" y1="12" y2="12" />
          <line x1="8" x2="3" y1="12" y2="12" />
          <line x1="21" x2="16" y1="20" y2="20" />
          <line x1="12" x2="3" y1="20" y2="20" />
          <line x1="14" x2="14" y1="2" y2="6" />
          <line x1="8" x2="8" y1="10" y2="14" />
          <line x1="16" x2="16" y1="18" y2="22" />
        </svg>
        <span>自定义</span>
      </button>
      <button
        type="button"
        :class="{ active: filterMode === 'font' }"
        :aria-pressed="filterMode === 'font'"
        title="查看字体设置"
        aria-label="查看字体设置"
        @click="toggleFilterMode('font')"
      >
        <svg class="ppt-theme-icon" viewBox="0 0 24 24" aria-hidden="true">
          <path d="M4 7V4h16v3" />
          <path d="M9 20h6" />
          <path d="M12 4v16" />
        </svg>
        <span>字体</span>
      </button>
      <button type="button" title="导入 PPTX 主题" aria-label="导入 PPTX 主题" @click="handleImportTheme">
        <svg class="ppt-theme-icon" viewBox="0 0 24 24" aria-hidden="true">
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
          <path d="M17 8 12 3 7 8" />
          <path d="M12 3v12" />
        </svg>
        <span>导入</span>
      </button>
    </div>

    <label class="ppt-theme-search">
      <svg class="ppt-theme-icon" viewBox="0 0 24 24" aria-hidden="true">
        <path d="m21 21-4.34-4.34" />
        <circle cx="11" cy="11" r="8" />
      </svg>
      <input
        v-model="searchQuery"
        type="text"
        title="搜索主题、颜色或场景"
        aria-label="搜索主题、颜色或场景"
        placeholder="搜索主题、颜色或场景"
      />
      <button v-if="searchQuery" type="button" title="清空主题搜索" aria-label="清空主题搜索" @click="searchQuery = ''">
        <svg class="ppt-theme-icon" viewBox="0 0 24 24" aria-hidden="true">
          <path d="M18 6 6 18" />
          <path d="m6 6 12 12" />
        </svg>
      </button>
    </label>

    <div v-if="filterMode === 'font'" class="ppt-theme-font-state">
      <strong>字体设置</strong>
      <p>字体库和品牌字体上传接口后续接入。当前演示文稿会跟随所选主题的默认字体。</p>
      <button type="button" title="返回主题列表" aria-label="返回主题列表" @click="filterMode = 'all'">返回主题</button>
    </div>

    <div v-else-if="filteredThemes.length" class="ppt-theme-selector" role="listbox" aria-label="主题列表">
      <button
        v-for="item in filteredThemes"
        :key="item.value"
        type="button"
        role="option"
        :aria-selected="modelValue === item.value"
        :title="`应用主题：${item.label}`"
        :aria-label="`应用主题：${item.label}`"
        :class="{ active: modelValue === item.value }"
        @click="$emit('update:modelValue', item.value)"
      >
        <span v-if="modelValue === item.value" class="ppt-theme-selected-mark" aria-hidden="true">
          <svg class="ppt-theme-icon" viewBox="0 0 24 24">
            <path d="m5 13 4 4L19 7" />
          </svg>
        </span>
        <span class="ppt-theme-card-head">
          <span class="ppt-theme-swatch">
            <i v-for="color in item.colors" :key="color" :style="{ background: color }"></i>
          </span>
          <span
            class="ppt-theme-star"
            :class="{ active: isFavorite(item.value) }"
            role="button"
            tabindex="0"
            :aria-pressed="isFavorite(item.value)"
            :aria-label="isFavorite(item.value) ? '取消收藏主题' : '收藏主题'"
            :title="isFavorite(item.value) ? '取消收藏主题' : '收藏主题'"
            @click.stop="toggleFavorite(item.value)"
            @keydown.enter.stop.prevent="toggleFavorite(item.value)"
            @keydown.space.stop.prevent="toggleFavorite(item.value)"
          >
            <svg class="ppt-theme-icon" viewBox="0 0 24 24" aria-hidden="true">
              <path d="M11.525 2.295a.53.53 0 0 1 .95 0l2.31 4.679a2.123 2.123 0 0 0 1.595 1.16l5.166.756a.53.53 0 0 1 .294.904l-3.736 3.638a2.123 2.123 0 0 0-.611 1.878l.882 5.14a.53.53 0 0 1-.771.56l-4.618-2.428a2.122 2.122 0 0 0-1.973 0L6.396 21.01a.53.53 0 0 1-.77-.56l.881-5.139a2.122 2.122 0 0 0-.611-1.879L2.16 9.795a.53.53 0 0 1 .294-.906l5.165-.755a2.122 2.122 0 0 0 1.597-1.16z" />
            </svg>
          </span>
        </span>
        <strong>{{ item.label }}</strong>
        <small>{{ item.description }}</small>
        <span class="ppt-theme-card-foot">
          <span>{{ item.group }}</span>
          <span v-if="modelValue === item.value" class="ppt-theme-applied-badge">
            <svg class="ppt-theme-icon" viewBox="0 0 24 24" aria-hidden="true">
              <path d="m5 13 4 4L19 7" />
            </svg>
            已应用
          </span>
        </span>
      </button>
    </div>

    <div v-else class="ppt-theme-empty-state">
      <svg class="ppt-theme-icon" viewBox="0 0 24 24" aria-hidden="true">
        <path d="M12 3a9 9 0 1 0 0 18h2a2.5 2.5 0 0 0 0-5h-2a2 2 0 0 1 0-4h2a7 7 0 0 0 7-7 2 2 0 0 0-2-2z" />
      </svg>
      <strong>{{ emptyTitle }}</strong>
      <span>{{ emptyDescription }}</span>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { ElMessage } from "element-plus/es/components/message/index";
import { pptThemes } from "../../config/pptThemes";
import type { PptTheme, PptThemeOption } from "../../types/ppt";

type ThemeTab = "standard" | "mine";
type ThemeFilterMode = "all" | "customize" | "font";
type ThemeListItem = PptThemeOption & {
  group: "标准主题" | "我的主题";
  keywords: string[];
};

const props = withDefaults(defineProps<{
  modelValue: PptTheme;
  showClose?: boolean;
}>(), {
  showClose: false
});

const emit = defineEmits<{
  "update:modelValue": [value: PptTheme];
  "create-theme": [];
  "import-theme": [];
  close: [];
}>();

const favoriteStorageKey = "xianzhi_ppt_theme_favorites";
const tabs = [
  { label: "标准主题", value: "standard" },
  { label: "我的主题", value: "mine" }
] satisfies Array<{ label: string; value: ThemeTab }>;

const activeTab = ref<ThemeTab>("standard");
const showFavoritesOnly = ref(false);
const searchQuery = ref("");
const favoriteThemes = ref<PptTheme[]>([]);
const filterMode = ref<ThemeFilterMode>("all");

const allThemes = computed<ThemeListItem[]>(() =>
  pptThemes.map((item) => ({
    ...item,
    group: "标准主题" as const,
    keywords: [item.label, item.description, item.value, ...item.colors]
  }))
);

const userThemes = computed<ThemeListItem[]>(() => {
  const selected = allThemes.value.find((item) => item.value === props.modelValue);
  if (!selected) return [];
  return [{
    ...selected,
    group: "我的主题" as const,
    label: `${selected.label} 副本`,
    description: "从当前演示主题保存的自定义主题占位，后续接入主题服务。",
    keywords: [...selected.keywords, "mine", "custom", "自定义"]
  }];
});

const filteredThemes = computed(() => {
  const query = normalize(searchQuery.value);
  const source = showFavoritesOnly.value
    ? allThemes.value
    : activeTab.value === "mine"
      ? userThemes.value
      : allThemes.value;
  return source.filter((item) => {
    if (showFavoritesOnly.value && !isFavorite(item.value)) return false;
    if (filterMode.value === "customize" && item.value !== props.modelValue) return false;
    if (!query) return true;
    return item.keywords.some((keyword) => normalize(keyword).includes(query));
  });
});

const emptyTitle = computed(() => {
  if (showFavoritesOnly.value) return "暂无收藏主题";
  if (searchQuery.value.trim()) return "没有匹配的主题";
  if (activeTab.value === "mine") return "还没有自定义主题";
  return "暂无主题";
});

const emptyDescription = computed(() => {
  if (showFavoritesOnly.value) return "点击主题卡片右上角星标后，会在这里集中查看。";
  if (searchQuery.value.trim()) return "尝试换一个关键词，或清空搜索后再看全部主题。";
  if (activeTab.value === "mine") return "第一版先保留自定义主题入口，后续可接入保存和导入服务。";
  return "主题配置暂不可用，请稍后再试。";
});

onMounted(() => {
  loadFavorites();
});

watch(favoriteThemes, (value) => {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(favoriteStorageKey, JSON.stringify(value));
}, { deep: true });

function normalize(value: string) {
  return value.toLowerCase().replace(/[-_/&]+/g, " ").replace(/\s+/g, " ").trim();
}

function loadFavorites() {
  if (typeof window === "undefined") return;
  try {
    const value = window.localStorage.getItem(favoriteStorageKey);
    if (!value) return;
    const parsed = JSON.parse(value);
    if (!Array.isArray(parsed)) return;
    favoriteThemes.value = parsed.filter((item): item is PptTheme =>
      typeof item === "string" && pptThemes.some((theme) => theme.value === item)
    );
  } catch {
    favoriteThemes.value = [];
  }
}

function isFavorite(value: PptTheme) {
  return favoriteThemes.value.includes(value);
}

function toggleFavorite(value: PptTheme) {
  if (isFavorite(value)) {
    favoriteThemes.value = favoriteThemes.value.filter((item) => item !== value);
    ElMessage.success("已取消收藏主题");
    return;
  }
  favoriteThemes.value = [value, ...favoriteThemes.value];
  ElMessage.success("已收藏主题");
}

function toggleFavoritesView() {
  showFavoritesOnly.value = !showFavoritesOnly.value;
}

function toggleFilterMode(value: ThemeFilterMode) {
  filterMode.value = filterMode.value === value ? "all" : value;
  if (value === "customize") {
    activeTab.value = "standard";
  }
}

function handleCreateTheme() {
  emit("create-theme");
  activeTab.value = "mine";
  filterMode.value = "customize";
  ElMessage.info("新建主题接口已预留，当前先展示当前主题副本");
}

function handleImportTheme() {
  emit("import-theme");
  ElMessage.info("导入 PPTX 主题接口已预留");
}
</script>

<style scoped>
.ppt-theme-panel {
  display: grid;
  gap: 12px;
  min-width: 0;
  min-height: 0;
}

.ppt-theme-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding-bottom: 10px;
  border-bottom: 1px solid #262626;
}

.ppt-theme-new-button,
.ppt-theme-favorite-toggle,
.ppt-theme-close-button,
.ppt-theme-filter-row button,
.ppt-theme-search button,
.ppt-theme-font-state button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 36px;
  border: 1px solid #2f2f2f;
  border-radius: 8px;
  color: #f4f4f5;
  background: #121212;
  cursor: pointer;
  font-weight: 760;
  transition: border-color 0.16s ease, background-color 0.16s ease, color 0.16s ease;
}

.ppt-theme-new-button {
  flex: 1;
  color: #111;
  border-color: #f4f4f5;
  background: #f4f4f5;
}

.ppt-theme-favorite-toggle {
  width: 36px;
  padding: 0;
}

.ppt-theme-close-button {
  width: 36px;
  padding: 0;
}

.ppt-theme-favorite-toggle.active,
.ppt-theme-star.active {
  color: #facc15;
  border-color: rgba(250, 204, 21, 0.42);
  background: rgba(250, 204, 21, 0.1);
}

.ppt-theme-favorite-toggle.active .ppt-theme-icon,
.ppt-theme-star.active .ppt-theme-icon {
  fill: currentColor;
}

.ppt-theme-tabs {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 4px;
  padding: 4px;
  border: 1px solid #262626;
  border-radius: 10px;
  background: #0b0b0b;
}

.ppt-theme-tabs button {
  min-height: 34px;
  border: 0;
  border-radius: 7px;
  color: #a1a1aa;
  background: transparent;
  cursor: pointer;
  font-weight: 780;
}

.ppt-theme-tabs button:hover,
.ppt-theme-tabs button.active {
  color: #f4f4f5;
  background: #1f1f1f;
}

.ppt-theme-filter-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  gap: 8px;
}

.ppt-theme-filter-row button {
  min-width: 0;
  padding: 0 10px;
  white-space: nowrap;
}

.ppt-theme-filter-row button:hover,
.ppt-theme-filter-row button.active {
  border-color: #525252;
  background: #1b1b1b;
}

.ppt-theme-search {
  display: grid;
  grid-template-columns: 18px minmax(0, 1fr) 28px;
  align-items: center;
  gap: 8px;
  min-height: 38px;
  padding: 0 8px 0 11px;
  border: 1px solid #2f2f2f;
  border-radius: 8px;
  background: #0b0b0b;
}

.ppt-theme-search input {
  min-width: 0;
  border: 0;
  outline: 0;
  color: #f8fafc;
  caret-color: #fff;
  background: transparent;
  font-size: 13px;
}

.ppt-theme-search input::placeholder {
  color: #71717a;
}

.ppt-theme-search button {
  min-height: 28px;
  width: 28px;
  padding: 0;
  border: 0;
  background: transparent;
}

.ppt-theme-selector {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  max-height: min(540px, calc(100vh - 260px));
  overflow: auto;
  padding-right: 2px;
}

.ppt-theme-selector > button {
  position: relative;
  display: grid;
  gap: 8px;
  min-height: 138px;
  padding: 11px;
  border: 1px solid #262626;
  border-radius: 8px;
  color: #f4f4f5;
  background: #0e0e0e;
  text-align: left;
  cursor: pointer;
  transition: border-color 0.16s ease, transform 0.16s ease, background-color 0.16s ease, box-shadow 0.16s ease;
}

.ppt-theme-selector > button:hover,
.ppt-theme-selector > button.active {
  border-color: #5f5f66;
  background: #171717;
  transform: translateY(-1px);
}

.ppt-theme-selector > button:focus-visible {
  outline: 2px solid #22d3ee;
  outline-offset: 2px;
}

.ppt-theme-selector > button.active {
  border-color: #f4f4f5;
  box-shadow: inset 0 0 0 1px rgba(244, 244, 245, 0.68), 0 10px 24px rgba(255, 255, 255, 0.06);
}

.ppt-theme-selected-mark {
  position: absolute;
  right: 9px;
  bottom: 9px;
  z-index: 2;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 999px;
  color: #111;
  background: #f4f4f5;
  box-shadow: 0 6px 18px rgba(255, 255, 255, 0.16);
}

.ppt-theme-selected-mark .ppt-theme-icon {
  width: 13px;
  height: 13px;
  stroke-width: 2.4;
}

.ppt-theme-card-head {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 30px;
  gap: 8px;
  align-items: center;
}

.ppt-theme-swatch {
  display: flex;
  height: 30px;
  overflow: hidden;
  border-radius: 6px;
}

.ppt-theme-swatch i {
  flex: 1;
}

.ppt-theme-star {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: 1px solid transparent;
  border-radius: 8px;
  color: #a1a1aa;
  background: rgba(255, 255, 255, 0.04);
  transition: color 0.16s ease, border-color 0.16s ease, background-color 0.16s ease, transform 0.16s ease;
}

.ppt-theme-star:hover,
.ppt-theme-star:focus-visible {
  color: #f4f4f5;
  border-color: #3f3f46;
  background: rgba(255, 255, 255, 0.08);
  outline: none;
  transform: translateY(-1px);
}

.ppt-theme-selector strong {
  font-size: 14px;
}

.ppt-theme-selector small {
  color: #9b9b9f;
  line-height: 1.45;
}

.ppt-theme-card-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  color: #71717a;
  font-size: 11px;
  font-weight: 760;
}

.ppt-theme-applied-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #86efac;
  padding-right: 28px;
}

.ppt-theme-applied-badge .ppt-theme-icon {
  width: 13px;
  height: 13px;
  stroke-width: 2.3;
}

.ppt-theme-font-state,
.ppt-theme-empty-state {
  display: grid;
  justify-items: center;
  gap: 10px;
  padding: 28px 16px;
  border: 1px dashed #333;
  border-radius: 10px;
  color: #a1a1aa;
  background: #0b0b0b;
  text-align: center;
}

.ppt-theme-font-state strong,
.ppt-theme-empty-state strong {
  color: #f4f4f5;
}

.ppt-theme-font-state p,
.ppt-theme-empty-state span {
  margin: 0;
  line-height: 1.6;
  font-size: 13px;
}

.ppt-theme-icon {
  width: 16px;
  height: 16px;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
}

@media (max-width: 900px) {
  .ppt-theme-selector,
  .ppt-theme-filter-row {
    grid-template-columns: 1fr;
  }
}
</style>
