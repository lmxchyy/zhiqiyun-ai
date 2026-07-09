<template>
  <div v-if="open" class="ppt-create-theme-backdrop" role="presentation" @click.self="closeModal">
    <section class="ppt-create-theme-modal" role="dialog" aria-modal="true" aria-labelledby="ppt-create-theme-title">
      <div class="ppt-create-theme-editor">
        <header class="ppt-create-theme-header">
          <div>
            <button type="button" title="返回上一步" aria-label="返回上一步" @click="handleBack">
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <path d="m15 18-6-6 6-6" />
              </svg>
            </button>
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <circle cx="13.5" cy="6.5" r=".5" />
              <circle cx="17.5" cy="10.5" r=".5" />
              <circle cx="8.5" cy="7.5" r=".5" />
              <circle cx="6.5" cy="12.5" r=".5" />
              <path d="M12 3a9 9 0 1 0 0 18h2a2.5 2.5 0 0 0 0-5h-2a2 2 0 0 1 0-4h2a7 7 0 0 0 7-7 2 2 0 0 0-2-2z" />
            </svg>
            <h2 id="ppt-create-theme-title">{{ currentStepTitle }}</h2>
          </div>
          <button type="button" title="关闭主题创建" aria-label="关闭主题创建" @click="closeModal">
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="M18 6 6 18" />
              <path d="m6 6 12 12" />
            </svg>
          </button>
        </header>

        <main class="ppt-create-theme-step">
          <section v-if="currentStep === 'colors'" class="ppt-create-theme-colors">
            <div class="ppt-create-theme-step-title">
              <strong>选择颜色方向</strong>
              <span>先选基础主题，再微调主要颜色。</span>
            </div>
            <div class="ppt-create-theme-preset-grid">
              <button
                v-for="item in pptThemes"
                :key="item.value"
                type="button"
                :class="{ active: selectedBaseTheme === item.value }"
                :aria-pressed="selectedBaseTheme === item.value"
                :title="`选择基础主题：${item.label}`"
                :aria-label="`选择基础主题：${item.label}`"
                @click="selectBaseTheme(item.value)"
              >
                <span class="ppt-create-theme-swatch">
                  <i v-for="color in item.colors" :key="color" :style="{ background: color }"></i>
                </span>
                <strong>{{ item.label }}</strong>
                <small>{{ item.description }}</small>
              </button>
            </div>
            <div class="ppt-create-theme-color-grid">
              <label v-for="item in colorFields" :key="item.key">
                <span>{{ item.label }}</span>
                <b>
                  <input v-model="draftColors[item.key]" type="color" :title="`${item.label} 颜色`" :aria-label="`${item.label} 颜色`" />
                  <input v-model="draftColors[item.key]" type="text" :title="`${item.label} 色值`" :aria-label="`${item.label} 色值`" />
                </b>
              </label>
            </div>
          </section>

          <section v-else-if="currentStep === 'fonts'" class="ppt-create-theme-form">
            <div class="ppt-create-theme-step-title">
              <strong>选择字体</strong>
              <span>参考项目支持品牌字体和 URL，这里先提供常用字体组合。</span>
            </div>
            <label>
              <span>标题字体</span>
              <select v-model="headingFont" title="标题字体" aria-label="标题字体">
                <option v-for="font in fontOptions" :key="font" :value="font">{{ font }}</option>
              </select>
            </label>
            <label>
              <span>正文字体</span>
              <select v-model="bodyFont" title="正文字体" aria-label="正文字体">
                <option v-for="font in fontOptions" :key="font" :value="font">{{ font }}</option>
              </select>
            </label>
            <label>
              <span>字体 URL</span>
              <input v-model="fontUrl" type="url" title="字体 URL" aria-label="字体 URL" placeholder="https://example.com/font.woff2" />
              <small>字体上传和 URL 校验接口后续接入。</small>
            </label>
          </section>

          <section v-else-if="currentStep === 'design'" class="ppt-create-theme-design">
            <div class="ppt-create-theme-step-title">
              <strong>选择内容样式</strong>
              <span>控制卡片圆角、阴影和版式气质。</span>
            </div>
            <button
              v-for="item in designOptions"
              :key="item.value"
              type="button"
              :class="{ active: designStyle === item.value }"
              :aria-pressed="designStyle === item.value"
              :title="`选择设计样式：${item.label}`"
              :aria-label="`选择设计样式：${item.label}`"
              @click="designStyle = item.value"
            >
              <span :class="['ppt-create-theme-design-preview', `style-${item.value}`]">
                <i></i>
              </span>
              <span>
                <strong>{{ item.label }}</strong>
                <small>{{ item.description }}</small>
              </span>
            </button>
          </section>

          <section v-else class="ppt-create-theme-save">
            <div class="ppt-create-theme-step-title">
              <strong>命名主题</strong>
              <span>给当前主题副本命名，保存后会作为本地草稿保留。</span>
            </div>
            <label>
              <span>主题名称</span>
              <input v-model="themeName" type="text" title="主题名称" aria-label="主题名称" placeholder="输入主题名称" />
            </label>
            <label>
              <span>主题描述</span>
              <textarea v-model="themeDescription" title="主题描述" aria-label="主题描述" placeholder="补充这个主题适合什么场景"></textarea>
            </label>
            <div class="ppt-create-theme-mode-grid">
              <button
                type="button"
                :class="{ active: themeMode === 'light' }"
                :aria-pressed="themeMode === 'light'"
                title="选择浅色模式"
                aria-label="选择浅色模式"
                @click="themeMode = 'light'"
              >
                <b></b>
                <span>
                  <strong>浅色模式</strong>
                  <small>适合浅色背景</small>
                </span>
              </button>
              <button
                type="button"
                :class="{ active: themeMode === 'dark' }"
                :aria-pressed="themeMode === 'dark'"
                title="选择深色模式"
                aria-label="选择深色模式"
                @click="themeMode = 'dark'"
              >
                <b></b>
                <span>
                  <strong>深色模式</strong>
                  <small>适合深色背景</small>
                </span>
              </button>
            </div>
          </section>
        </main>

        <footer class="ppt-create-theme-footer">
          <nav aria-label="主题创建步骤">
            <button
              v-for="(item, index) in steps"
              :key="item.value"
              type="button"
              :class="{ active: currentStep === item.value }"
              :title="item.label"
              :aria-label="`进入主题创建步骤：${item.label}`"
              :aria-current="currentStep === item.value ? 'step' : undefined"
              @click="currentStep = item.value"
            >
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <path v-if="item.icon === 'palette'" d="M12 3a9 9 0 1 0 0 18h2a2.5 2.5 0 0 0 0-5h-2a2 2 0 0 1 0-4h2a7 7 0 0 0 7-7 2 2 0 0 0-2-2z" />
                <path v-else-if="item.icon === 'font'" d="M4 7V4h16v3M9 20h6M12 4v16" />
                <path v-else-if="item.icon === 'shape'" d="M4 7a3 3 0 0 1 3-3h2v6H7a3 3 0 0 1-3-3ZM14 4h3a3 3 0 0 1 0 6h-3zM4 17a3 3 0 0 1 3-3h10a3 3 0 1 1 0 6H7a3 3 0 0 1-3-3Z" />
                <path v-else d="m5 13 4 4L19 7" />
              </svg>
              <small v-if="index < steps.length - 1"></small>
            </button>
          </nav>
          <div class="ppt-create-theme-actions">
            <div v-if="isCustomizing && currentStep !== 'save'" class="ppt-create-theme-save-group">
              <button
                type="button"
                class="primary"
                :disabled="isSubmitting"
                :aria-busy="isSubmitting ? 'true' : undefined"
                title="保存当前主题自定义"
                aria-label="保存当前主题自定义"
                @click="saveTheme('save')"
              >
                <svg viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2Z" />
                  <path d="M17 21v-8H7v8" />
                  <path d="M7 3v5h8" />
                </svg>
                <span>保存</span>
              </button>
              <button type="button" class="primary icon" :aria-expanded="saveMenuOpen" title="更多保存选项" aria-label="更多保存选项" @click="saveMenuOpen = !saveMenuOpen">
                <svg viewBox="0 0 24 24" aria-hidden="true">
                  <path d="m6 9 6 6 6-6" />
                </svg>
              </button>
              <div v-if="saveMenuOpen" class="ppt-create-theme-save-menu" role="menu">
                <button type="button" role="menuitem" title="保存" aria-label="保存" @click="saveTheme('save')">保存</button>
                <button type="button" role="menuitem" title="保存并新建" aria-label="保存并新建" @click="saveTheme('copy')">保存并新建</button>
                <button type="button" role="menuitem" title="重置自定义" aria-label="重置自定义" @click="resetCustomization">重置自定义</button>
              </div>
            </div>
            <button
              type="button"
              class="primary"
              :disabled="isSubmitting"
              :aria-busy="isSubmitting ? 'true' : undefined"
              :title="continueLabel"
              :aria-label="continueLabel"
              @click="handleContinue"
            >
              <span>{{ continueLabel }}</span>
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <path d="m9 18 6-6-6-6" />
              </svg>
            </button>
          </div>
        </footer>
      </div>

      <aside class="ppt-create-theme-preview">
        <div class="ppt-create-theme-preview-tabs" role="tablist" aria-label="预览范围">
          <button type="button" role="tab" :aria-selected="previewTab === 'test'" title="预览测试页" aria-label="预览测试页" :class="{ active: previewTab === 'test' }" @click="previewTab = 'test'">测试页</button>
          <button type="button" role="tab" :aria-selected="previewTab === 'current'" title="预览当前幻灯片" aria-label="预览当前幻灯片" :class="{ active: previewTab === 'current' }" @click="previewTab = 'current'">当前页</button>
        </div>
        <article class="ppt-create-theme-preview-slide" :class="`style-${designStyle}`">
          <div :style="{ background: draftColors.primary }">
            <main :style="{ background: draftColors.background, color: draftColors.text, borderColor: draftColors.accent }">
              <h3 :style="{ color: draftColors.heading, fontFamily: headingFont }">{{ previewTitle }}</h3>
              <p :style="{ fontFamily: bodyFont }">{{ previewDescription }}</p>
              <ul>
                <li v-for="item in previewBullets" :key="item" :style="{ fontFamily: bodyFont }">{{ item }}</li>
              </ul>
              <i :style="{ background: draftColors.accent }"></i>
            </main>
          </div>
        </article>
      </aside>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ElMessage } from "element-plus/es/components/message/index";
import { pptThemes } from "../../config/pptThemes";
import type { PptSlide, PptTheme } from "../../types/ppt";

type CreateThemeStep = "colors" | "fonts" | "design" | "save";
type ThemeMode = "light" | "dark";
type DesignStyle = "standard" | "flat" | "outline" | "blocky" | "rounded";
type DraftColors = {
  primary: string;
  background: string;
  accent: string;
  text: string;
  heading: string;
};

const props = withDefaults(defineProps<{
  open: boolean;
  baseTheme: PptTheme;
  currentSlides?: PptSlide[];
  isCustomizing?: boolean;
}>(), {
  currentSlides: () => [],
  isCustomizing: true
});

const emit = defineEmits<{
  "update:open": [value: boolean];
  "apply-theme": [value: PptTheme];
  saved: [];
}>();

const currentStep = ref<CreateThemeStep>("colors");
const previewTab = ref<"test" | "current">("test");
const selectedBaseTheme = ref<PptTheme>(props.baseTheme);
const draftColors = ref<DraftColors>({
  primary: "#1e293b",
  background: "#f8fafc",
  accent: "#2563eb",
  text: "#334155",
  heading: "#0f172a"
});
const headingFont = ref("Inter, sans-serif");
const bodyFont = ref("Inter, sans-serif");
const fontUrl = ref("");
const designStyle = ref<DesignStyle>("standard");
const themeName = ref("");
const themeDescription = ref("");
const themeMode = ref<ThemeMode>("light");
const isSubmitting = ref(false);
const saveMenuOpen = ref(false);

const steps = [
  { value: "colors", label: "颜色", icon: "palette" },
  { value: "fonts", label: "字体", icon: "font" },
  { value: "design", label: "设计", icon: "shape" },
  { value: "save", label: "保存", icon: "check" }
] satisfies Array<{ value: CreateThemeStep; label: string; icon: string }>;
const colorFields = [
  { key: "primary", label: "主色" },
  { key: "background", label: "背景" },
  { key: "accent", label: "强调色" },
  { key: "text", label: "正文" },
  { key: "heading", label: "标题" }
] satisfies Array<{ key: keyof DraftColors; label: string }>;
const fontOptions = ["Inter, sans-serif", "Arial, sans-serif", "Georgia, serif", "PingFang SC, sans-serif", "Microsoft YaHei, sans-serif", "Source Han Sans SC, sans-serif"];
const designOptions = [
  { value: "standard", label: "标准", description: "小圆角和轻微阴影" },
  { value: "flat", label: "扁平", description: "无阴影、无圆角" },
  { value: "outline", label: "描边", description: "强调描边和结构" },
  { value: "blocky", label: "块面", description: "硬朗的立体投影" },
  { value: "rounded", label: "圆润", description: "更柔和的大圆角" }
] satisfies Array<{ value: DesignStyle; label: string; description: string }>;
const currentStepIndex = computed(() => steps.findIndex((item) => item.value === currentStep.value));
const currentStepTitle = computed(() => steps.find((item) => item.value === currentStep.value)?.label || "Colors");
const continueLabel = computed(() => {
  if (currentStep.value === "save") return props.isCustomizing ? "保存并新建" : "发布主题";
  return props.isCustomizing ? "下一步" : "继续";
});
const previewTitle = computed(() => {
  if (previewTab.value === "current" && props.currentSlides[0]) return props.currentSlides[0].title;
  return themeName.value || "主题预览";
});
const previewDescription = computed(() => {
  if (previewTab.value === "current" && props.currentSlides[0]) return props.currentSlides[0].content;
  return themeDescription.value || "用 AI 快速生成结构清晰的演示文稿。";
});
const previewBullets = computed(() => {
  if (previewTab.value === "current" && props.currentSlides[0]?.bulletPoints?.length) return props.currentSlides[0].bulletPoints.slice(0, 3);
  return ["颜色系统", "字体组合", "可复用设计样式"];
});

watch(
  () => props.open,
  (value) => {
    if (!value) return;
    currentStep.value = "colors";
    previewTab.value = "test";
    saveMenuOpen.value = false;
    selectedBaseTheme.value = props.baseTheme;
    selectBaseTheme(props.baseTheme, false);
  }
);

watch(
  () => props.baseTheme,
  (value) => {
    if (!props.open) selectedBaseTheme.value = value;
  }
);

function selectBaseTheme(theme: PptTheme, updateName = true) {
  const option = pptThemes.find((item) => item.value === theme) || pptThemes[0];
  if (!option) return;
  selectedBaseTheme.value = option.value;
  draftColors.value = {
    background: option.colors[0],
    primary: option.colors[1],
    accent: option.colors[2],
    text: option.colors[1],
    heading: option.colors[1]
  };
  if (updateName || !themeName.value) {
    themeName.value = `${option.label} 副本`;
    themeDescription.value = option.description;
  }
}

function closeModal() {
  emit("update:open", false);
}

function handleBack() {
  if (currentStepIndex.value <= 0) {
    closeModal();
    return;
  }
  currentStep.value = steps[currentStepIndex.value - 1]?.value || "colors";
}

function handleContinue() {
  if (currentStep.value === "save") {
    void saveTheme("copy");
    return;
  }
  currentStep.value = steps[currentStepIndex.value + 1]?.value || "save";
}

async function saveTheme(kind: "save" | "copy") {
  if (!themeName.value.trim()) {
    ElMessage.warning("请先填写主题名称");
    currentStep.value = "save";
    return;
  }
  isSubmitting.value = true;
  await wait(180);
  const item = {
    id: `local_theme_${Date.now()}`,
    baseTheme: selectedBaseTheme.value,
    name: themeName.value.trim(),
    description: themeDescription.value.trim(),
    colors: draftColors.value,
    headingFont: headingFont.value,
    bodyFont: bodyFont.value,
    fontUrl: fontUrl.value.trim(),
    designStyle: designStyle.value,
    mode: themeMode.value,
    createdAt: new Date().toISOString()
  };
  try {
    if (typeof window !== "undefined") {
      const key = "xianzhi_ppt_local_theme_drafts";
      const oldValue = window.localStorage.getItem(key);
      const parsed = oldValue ? JSON.parse(oldValue) : [];
      const list = Array.isArray(parsed) ? parsed : [];
      window.localStorage.setItem(key, JSON.stringify([item, ...list].slice(0, 20)));
    }
  } catch {
    // 本地存储不可用时仍保持当前会话可用。
  }
  emit("apply-theme", selectedBaseTheme.value);
  emit("saved");
  ElMessage.success(kind === "copy" ? "已保存主题副本并应用基础主题" : "已保存当前主题自定义");
  isSubmitting.value = false;
  closeModal();
}

function resetCustomization() {
  selectBaseTheme(props.baseTheme);
  designStyle.value = "standard";
  headingFont.value = "Inter, sans-serif";
  bodyFont.value = "Inter, sans-serif";
  fontUrl.value = "";
  saveMenuOpen.value = false;
  ElMessage.success("已重置主题自定义");
}

function wait(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}
</script>

<style scoped>
.ppt-create-theme-backdrop {
  position: fixed;
  inset: 0;
  z-index: 120;
  background: rgba(0, 0, 0, 0.72);
}

.ppt-create-theme-modal {
  display: grid;
  grid-template-columns: minmax(420px, 1fr) minmax(420px, 1fr);
  width: 100vw;
  height: 100vh;
  color: #f4f4f5;
  background: #080808;
}

.ppt-create-theme-editor {
  display: grid;
  grid-template-rows: 70px minmax(0, 1fr) 76px;
  min-width: 0;
  border-right: 1px solid #202020;
}

.ppt-create-theme-header,
.ppt-create-theme-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 16px;
  border-color: #202020;
}

.ppt-create-theme-header {
  border-bottom: 1px solid #202020;
}

.ppt-create-theme-footer {
  border-top: 1px solid #202020;
}

.ppt-create-theme-header > div {
  display: flex;
  align-items: center;
  gap: 12px;
}

.ppt-create-theme-header h2 {
  margin: 0;
  font-size: 18px;
  letter-spacing: 0;
}

.ppt-create-theme-header button,
.ppt-create-theme-footer button,
.ppt-create-theme-preset-grid button,
.ppt-create-theme-design button,
.ppt-create-theme-mode-grid button,
.ppt-create-theme-preview-tabs button {
  border: 1px solid #2b2b2b;
  border-radius: 9px;
  color: #f4f4f5;
  background: #111;
  cursor: pointer;
  transition: border-color 0.16s ease, background-color 0.16s ease, transform 0.16s ease;
}

.ppt-create-theme-header button {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
}

.ppt-create-theme-step {
  min-height: 0;
  overflow: auto;
  padding: 24px;
}

.ppt-create-theme-step-title {
  display: grid;
  gap: 7px;
  margin-bottom: 18px;
  text-align: center;
}

.ppt-create-theme-step-title strong {
  font-size: 21px;
}

.ppt-create-theme-step-title span,
.ppt-create-theme-form small,
.ppt-create-theme-preset-grid small,
.ppt-create-theme-design small,
.ppt-create-theme-mode-grid small {
  color: #a1a1aa;
  font-size: 13px;
  line-height: 1.5;
}

.ppt-create-theme-preset-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.ppt-create-theme-preset-grid button {
  display: grid;
  gap: 8px;
  min-height: 132px;
  padding: 12px;
  text-align: left;
}

.ppt-create-theme-preset-grid button:hover,
.ppt-create-theme-preset-grid button.active,
.ppt-create-theme-design button:hover,
.ppt-create-theme-design button.active,
.ppt-create-theme-mode-grid button:hover,
.ppt-create-theme-mode-grid button.active,
.ppt-create-theme-preview-tabs button:hover,
.ppt-create-theme-preview-tabs button.active {
  border-color: #60a5fa;
  background: rgba(37, 99, 235, 0.14);
}

.ppt-create-theme-swatch {
  display: flex;
  height: 32px;
  overflow: hidden;
  border-radius: 7px;
}

.ppt-create-theme-swatch i {
  flex: 1;
}

.ppt-create-theme-color-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin-top: 18px;
}

.ppt-create-theme-color-grid label,
.ppt-create-theme-form label,
.ppt-create-theme-save label {
  display: grid;
  gap: 8px;
}

.ppt-create-theme-color-grid label > span,
.ppt-create-theme-form label > span,
.ppt-create-theme-save label > span {
  color: #d4d4d8;
  font-size: 13px;
  font-weight: 780;
}

.ppt-create-theme-color-grid b {
  display: grid;
  grid-template-columns: 44px minmax(0, 1fr);
  gap: 8px;
}

.ppt-create-theme-color-grid input,
.ppt-create-theme-form input,
.ppt-create-theme-form select,
.ppt-create-theme-save input,
.ppt-create-theme-save textarea {
  width: 100%;
  min-height: 40px;
  box-sizing: border-box;
  border: 1px solid #303030;
  border-radius: 8px;
  color: #f8fafc;
  caret-color: #fff;
  background: #0d0d0d;
  outline: none;
}

.ppt-create-theme-color-grid input[type="color"] {
  padding: 3px;
}

.ppt-create-theme-color-grid input[type="text"],
.ppt-create-theme-form input,
.ppt-create-theme-form select,
.ppt-create-theme-save input,
.ppt-create-theme-save textarea {
  padding: 0 11px;
}

.ppt-create-theme-save textarea {
  min-height: 96px;
  padding: 10px 11px;
  resize: vertical;
}

.ppt-create-theme-form,
.ppt-create-theme-save {
  display: grid;
  gap: 18px;
  max-width: 620px;
  margin: 0 auto;
}

.ppt-create-theme-design {
  display: grid;
  gap: 12px;
}

.ppt-create-theme-design button {
  display: flex;
  align-items: center;
  gap: 22px;
  padding: 18px;
  text-align: left;
}

.ppt-create-theme-design-preview {
  display: grid;
  place-items: center;
  flex: 0 0 96px;
  width: 96px;
  height: 64px;
  border: 1px solid #303030;
  background: #171717;
}

.ppt-create-theme-design-preview i {
  width: 48px;
  height: 30px;
  background: #3f3f46;
}

.ppt-create-theme-design-preview.style-standard {
  border-radius: 10px;
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.28);
}

.ppt-create-theme-design-preview.style-standard i {
  border-radius: 6px;
}

.ppt-create-theme-design-preview.style-flat,
.ppt-create-theme-design-preview.style-flat i {
  border-radius: 0;
  box-shadow: none;
}

.ppt-create-theme-design-preview.style-outline,
.ppt-create-theme-design-preview.style-outline i {
  border-radius: 0;
  box-shadow: 0 0 0 2px #60a5fa;
}

.ppt-create-theme-design-preview.style-blocky,
.ppt-create-theme-design-preview.style-blocky i {
  border-radius: 0;
  box-shadow: 5px 5px 0 rgba(0, 0, 0, 0.34);
}

.ppt-create-theme-design-preview.style-rounded {
  border-radius: 22px;
}

.ppt-create-theme-design-preview.style-rounded i {
  border-radius: 999px;
}

.ppt-create-theme-mode-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.ppt-create-theme-mode-grid button {
  display: flex;
  gap: 12px;
  min-height: 92px;
  padding: 18px;
  text-align: left;
}

.ppt-create-theme-mode-grid button > b {
  width: 20px;
  height: 20px;
  border: 2px solid #52525b;
  border-radius: 999px;
}

.ppt-create-theme-mode-grid button.active > b {
  border-color: #60a5fa;
  background: radial-gradient(circle at center, #60a5fa 0 42%, transparent 45%);
}

.ppt-create-theme-footer nav {
  display: flex;
  align-items: center;
  gap: 12px;
}

.ppt-create-theme-footer nav button {
  position: relative;
  display: grid;
  place-items: center;
  width: 40px;
  height: 40px;
  border-radius: 999px;
}

.ppt-create-theme-footer nav button.active {
  border-color: #2563eb;
  background: #2563eb;
  transform: scale(1.08);
}

.ppt-create-theme-footer nav small {
  position: absolute;
  left: calc(100% + 4px);
  width: 16px;
  height: 2px;
  background: #303030;
}

.ppt-create-theme-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.ppt-create-theme-actions button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 38px;
  padding: 0 14px;
  font-weight: 780;
}

.ppt-create-theme-actions button.primary {
  border-color: #2563eb;
  background: #2563eb;
}

.ppt-create-theme-actions button.icon {
  width: 38px;
  padding: 0;
}

.ppt-create-theme-save-group {
  position: relative;
  display: flex;
  gap: 1px;
}

.ppt-create-theme-save-menu {
  position: absolute;
  right: 0;
  bottom: calc(100% + 8px);
  z-index: 10;
  display: grid;
  gap: 4px;
  width: 190px;
  padding: 6px;
  border: 1px solid #303030;
  border-radius: 10px;
  background: #111;
  box-shadow: 0 18px 40px rgba(0, 0, 0, 0.45);
}

.ppt-create-theme-save-menu button {
  justify-content: flex-start;
  min-height: 34px;
  border: 0;
  background: transparent;
}

.ppt-create-theme-save-menu button:hover {
  background: #242424;
}

.ppt-create-theme-preview {
  display: grid;
  align-content: center;
  gap: 18px;
  min-width: 0;
  padding: 32px;
}

.ppt-create-theme-preview-tabs {
  display: flex;
  gap: 8px;
}

.ppt-create-theme-preview-tabs button {
  min-height: 34px;
  padding: 0 12px;
}

.ppt-create-theme-preview-slide {
  max-width: 760px;
  overflow: hidden;
  border: 1px solid #262626;
  background: #111;
}

.ppt-create-theme-preview-slide,
.ppt-create-theme-preview-slide > div,
.ppt-create-theme-preview-slide main {
  border-radius: 14px;
}

.ppt-create-theme-preview-slide > div {
  aspect-ratio: 16 / 10;
  padding: 26px;
}

.ppt-create-theme-preview-slide main {
  display: grid;
  align-content: center;
  gap: 14px;
  height: 100%;
  padding: 34px;
  border: 1px solid;
}

.ppt-create-theme-preview-slide h3 {
  margin: 0;
  font-size: 38px;
  letter-spacing: 0;
}

.ppt-create-theme-preview-slide p {
  max-width: 520px;
  margin: 0;
  line-height: 1.65;
}

.ppt-create-theme-preview-slide ul {
  display: grid;
  gap: 7px;
  margin: 0;
  padding-left: 18px;
}

.ppt-create-theme-preview-slide i {
  width: 90px;
  height: 7px;
  border-radius: 999px;
}

.ppt-create-theme-preview-slide.style-flat,
.ppt-create-theme-preview-slide.style-flat > div,
.ppt-create-theme-preview-slide.style-flat main {
  border-radius: 0;
  box-shadow: none;
}

.ppt-create-theme-preview-slide.style-outline main {
  box-shadow: 0 0 0 2px currentColor;
}

.ppt-create-theme-preview-slide.style-blocky main {
  box-shadow: 8px 8px 0 rgba(0, 0, 0, 0.24);
}

.ppt-create-theme-preview-slide.style-rounded,
.ppt-create-theme-preview-slide.style-rounded > div,
.ppt-create-theme-preview-slide.style-rounded main {
  border-radius: 28px;
}

.ppt-create-theme-modal svg {
  width: 18px;
  height: 18px;
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.ppt-create-theme-modal button:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

@media (max-width: 980px) {
  .ppt-create-theme-modal {
    grid-template-columns: 1fr;
    overflow: auto;
  }

  .ppt-create-theme-editor {
    min-height: 720px;
    border-right: 0;
    border-bottom: 1px solid #202020;
  }
}

@media (max-width: 720px) {
  .ppt-create-theme-preset-grid,
  .ppt-create-theme-color-grid,
  .ppt-create-theme-mode-grid {
    grid-template-columns: 1fr;
  }

  .ppt-create-theme-footer {
    align-items: stretch;
    flex-direction: column;
  }

  .ppt-create-theme-footer nav {
    justify-content: center;
  }

  .ppt-create-theme-actions {
    justify-content: flex-end;
  }
}
</style>
